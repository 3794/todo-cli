// Package tui is the Bubble Tea front end for a TODO.md file.
//
// Every edit re-reads the file from disk, finds its target line by content,
// changes only that line (see package todomd) and replaces the file
// atomically, so lines written by other programs in the meantime survive.
package tui

import (
	"errors"
	"os"
	"regexp"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/3794/todo-cli/internal/todomd"
)

// PollInterval is how often the file is checked for outside changes.
const PollInterval = 500 * time.Millisecond

type mode int

const (
	modeNormal mode = iota
	modeAdd
	modeEdit
	modeConfirmDelete
)

// row is a visible line: a heading or an item.
type row struct {
	idx  int // index into doc.Lines
	line todomd.Line
}

// target identifies the line an operation applies to, as it was when the
// operation started.
type target struct {
	ok      bool // false: no line (add at end of file)
	content string
	hint    int
}

type tickMsg struct{}

// Model is the Bubble Tea model.
type Model struct {
	path string
	doc  todomd.Doc
	rows []row
	stat fileStat

	cursor  int
	offset  int
	hscroll int // first visible rune of the input when it is wider than the screen
	width   int
	height  int

	mode       mode
	target     target
	editOrig   string
	retargeted bool // edit target was re-found loosely after a conflict
	input      textinput.Model
	status     string

	now func() time.Time
}

// New loads path and returns a model for it. The file must exist.
func New(path string) (*Model, error) {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 0
	// The input line is drawn by renderInput and the real terminal cursor is
	// placed there, because IMEs draw the text being composed (e.g. Hangul)
	// at the real cursor.
	ti.SetVirtualCursor(false)
	m := &Model{path: path, input: ti, now: time.Now}
	if err := m.reload(); err != nil {
		return nil, err
	}
	return m, nil
}

// Init starts polling the file.
func (m *Model) Init() tea.Cmd { return tick() }

func tick() tea.Cmd {
	return tea.Tick(PollInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

// reload reads the file from disk and redraws, keeping the cursor on the
// same line content when possible.
func (m *Model) reload() error {
	st := statFile(m.path) // before reading, so a write in between triggers another reload
	data, err := os.ReadFile(m.path)
	if err != nil {
		m.stat = st
		return err
	}
	m.stat = st
	m.setDoc(todomd.Split(string(data)))
	return nil
}

func (m *Model) setDoc(d todomd.Doc) {
	prev, hadPrev := m.current()
	m.doc = d
	m.rows = m.rows[:0]
	for i, l := range d.Parse() {
		if l.Kind == todomd.Heading || l.Kind == todomd.Item {
			m.rows = append(m.rows, row{idx: i, line: l})
		}
	}
	if hadPrev {
		if i := m.findRow(prev.line.Raw, prev.idx); i >= 0 {
			m.cursor = i
		}
	}
	m.clamp()
}

// findRow returns the row whose line equals content, closest to line index
// hint, or -1.
func (m *Model) findRow(content string, hint int) int {
	best := -1
	for i, r := range m.rows {
		if r.line.Raw == content && (best < 0 || absInt(m.rows[i].idx-hint) < absInt(m.rows[best].idx-hint)) {
			best = i
		}
	}
	return best
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func (m *Model) selectLine(idx int) {
	for i, r := range m.rows {
		if r.idx == idx {
			m.cursor = i
			return
		}
	}
}

func (m *Model) current() (row, bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return row{}, false
	}
	return m.rows[m.cursor], true
}

func (m *Model) clamp() {
	m.cursor = max(0, min(m.cursor, len(m.rows)-1))
}

func (m *Model) poll() {
	st := statFile(m.path)
	if st.missing {
		if !m.stat.missing {
			m.status = "File not found: " + m.path
		}
		m.stat = st
		return
	}
	if !m.stat.missing && st.size == m.stat.size && st.modTime.Equal(m.stat.modTime) {
		return
	}
	if err := m.reload(); err != nil {
		m.status = "Reload failed: " + err.Error()
	}
}

// Update handles messages.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tickMsg:
		m.poll()
		cmd = tick()
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		switch m.mode {
		case modeNormal:
			cmd = m.updateNormal(msg)
		case modeAdd, modeEdit:
			cmd = m.updateInput(msg)
		case modeConfirmDelete:
			m.updateConfirm(msg)
		}
	default:
		if m.mode == modeAdd || m.mode == modeEdit {
			m.input, cmd = m.input.Update(msg) // paste, cursor blink, ...
		}
	}
	m.scroll()
	m.fitInput()
	return m, cmd
}

func (m *Model) updateNormal(msg tea.KeyPressMsg) tea.Cmd {
	m.status = ""
	r, ok := m.current()
	isItem := ok && r.line.Kind == todomd.Item
	switch msg.String() {
	case "q":
		return tea.Quit
	case "j", "down":
		if m.cursor < len(m.rows)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "a":
		m.target = target{}
		if ok {
			m.target = target{ok: true, content: r.line.Raw, hint: r.idx}
		}
		m.mode = modeAdd
		m.input.Reset()
		return m.input.Focus()
	case "e", "enter":
		if !isItem {
			return nil
		}
		m.target = target{ok: true, content: r.line.Raw, hint: r.idx}
		m.editOrig = r.line.Text
		m.retargeted = false
		m.mode = modeEdit
		m.input.SetValue(r.line.Text)
		m.input.CursorEnd()
		return m.input.Focus()
	case "space":
		if !isItem {
			return nil
		}
		today := m.now().Format("2006-01-02")
		t := target{ok: true, content: r.line.Raw, hint: r.idx}
		m.apply(m.findExact(t), func(d todomd.Doc, i int) (todomd.Doc, int, error) {
			nd, err := todomd.ToggleItem(d, i, today)
			return nd, i, err
		})
	case "d":
		if !isItem {
			return nil
		}
		m.target = target{ok: true, content: r.line.Raw, hint: r.idx}
		m.mode = modeConfirmDelete
	}
	return nil
}

func (m *Model) updateInput(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.status = ""
		m.closeInput()
		return nil
	case "enter":
		m.status = ""
		value := m.input.Value()
		text := strings.TrimSpace(todomd.CleanText(value))
		switch {
		case text == "":
			m.closeInput()
			m.status = "Empty — not saved"
		case m.mode == modeEdit && value == m.editOrig:
			m.closeInput() // unchanged
		case m.mode == modeEdit:
			if m.saveEdit(text) {
				m.closeInput()
			}
		default:
			if m.saveAdd(text) {
				m.closeInput()
			}
		}
		// On failure the input stays open with the typed text (rule 6).
		return nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return cmd
}

func (m *Model) closeInput() {
	m.mode = modeNormal
	m.input.Blur()
	m.input.Reset()
}

// saveAdd adds text below the anchor captured when 'a' was pressed. The
// anchor is only a position hint: if it changed it is found loosely, and if
// it is gone the item goes to the end of the file. Adding is never cancelled.
func (m *Model) saveAdd(text string) bool {
	t := m.target
	atEnd := false
	ok := m.apply(func(lines []string) (int, bool) {
		atEnd = false
		if !t.ok {
			return -1, true
		}
		i := todomd.FindLineLoose(lines, t.content, t.hint)
		atEnd = i < 0
		return i, true
	}, func(d todomd.Doc, i int) (todomd.Doc, int, error) {
		return todomd.AddItem(d, i, text)
	})
	if ok && atEnd {
		m.status = "Anchor line gone — added at end of file"
	}
	return ok
}

// saveEdit replaces the body of the edit target. If the target changed
// elsewhere nothing is written; when a loose match exists it becomes the new
// target, so pressing Enter again saves onto it.
func (m *Model) saveEdit(text string) bool {
	t := m.target
	if m.retargeted {
		// Keep the check state the line has now.
		text = todomd.WithDoneOf(text, todomd.ParseLine(t.content).Text)
	}
	return m.apply(func(lines []string) (int, bool) {
		if i := todomd.FindLine(lines, t.content, t.hint); i >= 0 {
			return i, true
		}
		if j := todomd.FindLineLoose(lines, t.content, t.hint); j >= 0 {
			m.target = target{ok: true, content: lines[j], hint: j}
			m.retargeted = true
			m.selectLine(j)
			m.status = "Item changed elsewhere — not saved. Enter: save to updated item · Esc: cancel"
		} else {
			m.status = "Item removed elsewhere — not saved. Esc to close"
		}
		return 0, false
	}, func(d todomd.Doc, i int) (todomd.Doc, int, error) {
		nd, err := todomd.EditItem(d, i, text)
		return nd, i, err
	})
}

// findExact finds t's line exactly, or cancels the operation.
func (m *Model) findExact(t target) func([]string) (int, bool) {
	return func(lines []string) (int, bool) {
		i := todomd.FindLine(lines, t.content, t.hint)
		if i < 0 {
			m.status = "Item changed or removed elsewhere — cancelled"
			return 0, false
		}
		return i, true
	}
}

func (m *Model) updateConfirm(msg tea.KeyPressMsg) {
	switch msg.String() {
	case "y", "Y":
		m.mode = modeNormal
		pos := m.cursor
		if m.apply(m.findExact(m.target), func(d todomd.Doc, i int) (todomd.Doc, int, error) {
			nd, err := todomd.DeleteItem(d, i)
			return nd, -1, err
		}) {
			m.cursor = pos
			m.clamp()
		}
	case "n", "N", "esc":
		m.mode = modeNormal
	}
}

// apply runs op on a fresh read of the file and writes the result back
// atomically. find locates the target line in the fresh lines; when it
// reports false (after setting the status) the file is left alone. op
// receives that index and returns the new document and the line to select.
// Reports success.
func (m *Model) apply(find func([]string) (int, bool), op func(todomd.Doc, int) (todomd.Doc, int, error)) bool {
	for range 3 {
		data, err := os.ReadFile(m.path)
		if err != nil {
			m.status = "Read failed: " + err.Error()
			return false
		}
		doc := todomd.Split(string(data))
		m.setDoc(doc) // show what is on disk even if we stop here
		idx, ok := find(doc.Lines)
		if !ok {
			return false
		}
		nd, sel, err := op(doc, idx)
		if err != nil {
			m.status = "Cannot do that: " + err.Error()
			return false
		}
		err = writeAtomic(m.path, []byte(nd.String()), data)
		if errors.Is(err, errChanged) {
			continue // someone wrote in between; redo on their version
		}
		if err != nil {
			m.status = "Save failed: " + err.Error()
			return false
		}
		m.setDoc(nd)
		m.selectLine(sel)
		return true
	}
	m.status = "File keeps changing — not saved. Try again"
	_ = m.reload()
	return false
}

// listHeight is the number of screen lines available for the list.
func (m *Model) listHeight() int {
	if m.height <= 0 {
		return 1 << 30 // unknown size: show everything
	}
	return max(1, m.height-1-len(m.footer())) // title + footer
}

// scroll picks the first visible row so that every line of the selected row
// is on screen (from its first line when it is taller than the list), and
// the list fills the screen when it can.
func (m *Model) scroll() {
	if m.height <= 0 || len(m.rows) == 0 {
		m.offset = 0
		return
	}
	h := m.listHeight()
	heights := make([]int, len(m.rows))
	for i, r := range m.rows {
		heights[i] = len(m.rowLines(r, false))
	}
	m.offset = max(0, min(m.offset, m.cursor))
	sum := func(from, to int) int { // lines in rows [from, to]
		n := 0
		for i := from; i <= to; i++ {
			n += heights[i]
		}
		return n
	}
	for m.offset < m.cursor && sum(m.offset, m.cursor) > h {
		m.offset++
	}
	for m.offset > 0 && sum(m.offset-1, len(m.rows)-1) <= h {
		m.offset--
	}
}

var (
	titleStyle   = lipgloss.NewStyle().Faint(true)
	headingStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("4"))
	cursorStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5"))
	doneStyle    = lipgloss.NewStyle().Faint(true)
	tagStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	dueStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	statusStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	helpStyle    = lipgloss.NewStyle().Faint(true)

	tokenRe = regexp.MustCompile(`[^ ]+| +`)
)

// clean makes text safe to print as is: tabs become spaces and control
// characters are replaced, so file content can't inject terminal escapes.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t':
			return ' '
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0):
			return '�'
		}
		return r
	}, s)
}

// rowLines renders a heading or item as one or more screen lines. Text that
// doesn't fit the screen width wraps; continuation lines line up with where
// the text starts (after the cursor mark, the indent and the checkbox).
func (m *Model) rowLines(r row, selected bool) []string {
	mark := "  "
	if selected {
		mark = cursorStyle.Render("› ")
	}
	l := r.line
	var lead []cell // indent and checkbox, first line only
	var body []cell
	if l.Kind == todomd.Heading {
		body = toCells(clean(l.Text), heading)
	} else {
		indent := todomd.IndentWidth(l.Indent)
		if m.width > 0 {
			// On a narrow screen give up indent before the text gets
			// narrower than 2 cells.
			indent = max(0, min(indent, m.width-2-4-2))
		}
		boxStyle := plain
		if l.Done() {
			boxStyle = done
		}
		lead = append(toCells(strings.Repeat(" ", indent), plain), toCells("["+clean(l.Mark)+"] ", boxStyle)...)
		body = itemCells(clean(l.Text), l.Done())
	}

	width := 1 << 30
	if m.width > 0 {
		width = m.width - 2 - cellsWidth(lead)
	}
	pad := strings.Repeat(" ", 2+cellsWidth(lead))
	var out []string
	for i, line := range wrapCells(body, width) {
		if i == 0 {
			out = append(out, mark+renderCells(lead)+renderCells(line))
		} else {
			out = append(out, pad+renderCells(line))
		}
	}
	return out
}

// footer returns the lines below the list: status, input or confirmation,
// and key help.
func (m *Model) footer() []string {
	lines, _, _ := m.footerWithCursor()
	return lines
}

// footerWithCursor is footer plus the input cursor: its row within the
// footer and its screen column, or row -1 when not typing.
func (m *Model) footerWithCursor() (lines []string, row, col int) {
	status := ""
	if m.status != "" {
		status = statusStyle.Render(m.status)
	}
	switch m.mode {
	case modeAdd, modeEdit:
		if status != "" {
			lines = append(lines, status)
		}
		text, col := m.renderInput()
		row = len(lines)
		return append(lines, text, helpStyle.Render("Enter save · Esc cancel")), row, col
	case modeConfirmDelete:
		return []string{
			statusStyle.Render("Delete this item and its sub-items? (y/n)"),
			helpStyle.Render("y delete · n/Esc cancel"),
		}, -1, 0
	default:
		return []string{status, helpStyle.Render("j/k move · a add · e edit · space done · d delete · q quit")}, -1, 0
	}
}

func (m *Model) inputLabel() string {
	if m.mode == modeEdit {
		return "Edit › "
	}
	return "Add › "
}

// inputRunes returns the input as displayed and the cursor's rune index.
func (m *Model) inputRunes() ([]rune, int) {
	r := []rune(clean(m.input.Value())) // clean maps rune to rune, so indexes match
	return r, min(m.input.Position(), len(r))
}

// fitInput scrolls the input horizontally so that the text before the cursor
// plus one cell for the cursor fits on screen. Widths are display widths
// (a Hangul syllable takes 2 cells).
func (m *Model) fitInput() {
	if (m.mode != modeAdd && m.mode != modeEdit) || m.width <= 0 {
		m.hscroll = 0
		return
	}
	r, pos := m.inputRunes()
	room := max(1, m.width-ansi.StringWidth(m.inputLabel())-1)
	m.hscroll = min(m.hscroll, pos)
	for m.hscroll < pos && ansi.StringWidth(string(r[m.hscroll:pos])) > room {
		m.hscroll++
	}
	// Scroll back when there is space again (e.g. after deleting).
	for m.hscroll > 0 && ansi.StringWidth(string(r[m.hscroll-1:pos])) <= room &&
		ansi.StringWidth(string(r[m.hscroll-1:])) <= room {
		m.hscroll--
	}
}

// renderInput draws the input line and returns the cursor's screen column.
func (m *Model) renderInput() (string, int) {
	label := m.inputLabel()
	lw := ansi.StringWidth(label)
	r, pos := m.inputRunes()
	start := min(m.hscroll, pos)
	text := string(r[start:])
	if m.width > 0 {
		text = ansi.Truncate(text, max(0, m.width-lw), "")
	}
	return label + text, lw + ansi.StringWidth(string(r[start:pos]))
}

// render returns the screen and the real terminal cursor (nil: hidden).
func (m *Model) render() (string, *tea.Cursor) {
	// Title, status and help lines are cut to the width; list lines are
	// already wrapped to it (the cut is only a guard for tiny screens).
	cut := func(s, tail string) string {
		if m.width > 0 {
			return ansi.Truncate(s, m.width, tail)
		}
		return s
	}
	lines := []string{cut(titleStyle.Render(clean(m.path)), "…")}
	if len(m.rows) == 0 {
		lines = append(lines, cut(helpStyle.Render("  (no items — press a to add)"), "…"))
	}
	room := m.listHeight()
	for i := m.offset; i < len(m.rows) && room > 0; i++ {
		for _, l := range m.rowLines(m.rows[i], i == m.cursor) {
			if room == 0 {
				break
			}
			lines = append(lines, cut(l, ""))
			room--
		}
	}
	footer, crow, ccol := m.footerWithCursor()
	var cur *tea.Cursor
	if crow >= 0 {
		cur = tea.NewCursor(ccol, len(lines)+crow)
	}
	for _, l := range footer {
		lines = append(lines, cut(l, "…"))
	}
	return strings.Join(lines, "\n"), cur
}

// View renders the screen.
func (m *Model) View() tea.View {
	content, cur := m.render()
	v := tea.NewView(content)
	v.AltScreen = true
	v.Cursor = cur
	return v
}
