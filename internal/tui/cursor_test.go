package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// The real terminal cursor must sit where the next character goes, measured
// in screen cells, because IMEs draw the text being composed there.

// labelWidth is the display width of the current input label, measured
// from the label itself so the tests don't depend on its wording.
func labelWidth(m *Model) int { return ansi.StringWidth(m.inputLabel()) }

// checkCursor verifies the cursor against the rendered screen alone: it is
// on the input line, inside the screen, the visible text left of it is the
// end of the text before the cursor, and the text right of it is the start
// of the text after the cursor. It returns the cursor.
func checkCursor(t *testing.T, m *Model) *tea.Cursor {
	t.Helper()
	cur := m.View().Cursor
	if cur == nil {
		t.Fatal("no real cursor while typing")
	}
	lines := strings.Split(screen(m), "\n")
	if cur.Y < 0 || cur.Y >= len(lines) {
		t.Fatalf("cursor row %d outside %d lines", cur.Y, len(lines))
	}
	line := lines[cur.Y]
	if !strings.HasPrefix(line, m.inputLabel()) {
		t.Fatalf("cursor row %d is not the input line: %q\n%s", cur.Y, line, screen(m))
	}
	if m.width > 0 && cur.X >= m.width {
		t.Fatalf("cursor column %d outside width %d", cur.X, m.width)
	}
	value := []rune(m.input.Value())
	pos := m.input.Position()
	before, after := string(value[:pos]), string(value[pos:])
	lw := labelWidth(m)
	left := ansi.Cut(line, lw, cur.X)
	right := ansi.Cut(line, cur.X, ansi.StringWidth(line))
	if !strings.HasSuffix(before, left) || ansi.StringWidth(left) != cur.X-lw {
		t.Fatalf("left of cursor %q is not the end of %q (x=%d)", left, before, cur.X)
	}
	if !strings.HasPrefix(after, right) {
		t.Fatalf("right of cursor %q is not the start of %q", right, after)
	}
	fits := m.width == 0 || lw+ansi.StringWidth(string(value))+1 <= m.width
	if fits && left != before {
		t.Fatalf("whole input fits but is scrolled: left=%q before=%q", left, before)
	}
	return cur
}

// wantCursorX checks that the cursor is dx cells right of the label.
func wantCursorX(t *testing.T, m *Model, dx int) {
	t.Helper()
	if cur := checkCursor(t, m); cur.X != labelWidth(m)+dx {
		t.Fatalf("cursor x = %d, want label %d + %d (input %q pos %d)", cur.X, labelWidth(m), dx, m.input.Value(), m.input.Position())
	}
}

func TestInputLabels(t *testing.T) {
	m, _ := setup(t, sample)
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	press(m, "a")
	if got := m.inputLabel(); got != "Add › " {
		t.Fatalf("add label %q", got)
	}
	wantCursorX(t, m, 0)
	if x := m.View().Cursor.X; x != 6 {
		t.Fatalf("empty add input: cursor x = %d, want 6", x)
	}
	press(m, "esc")
	moveTo(t, m, "- [ ] Set up CI cache")
	press(m, "e")
	if got := m.inputLabel(); got != "Edit › " {
		t.Fatalf("edit label %q", got)
	}
	if x := m.View().Cursor.X; x != 7+ansi.StringWidth("Set up CI cache") {
		t.Fatalf("edit input: cursor x = %d", x)
	}
}

func TestCursorHiddenOutsideInput(t *testing.T) {
	m, _ := setup(t, sample)
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	if m.View().Cursor != nil {
		t.Fatal("cursor shown in normal mode")
	}
	press(m, "a")
	checkCursor(t, m)
	press(m, "esc")
	if m.View().Cursor != nil {
		t.Fatal("cursor shown after esc")
	}
	moveTo(t, m, "- [ ] Set up CI cache")
	press(m, "d")
	if m.View().Cursor != nil {
		t.Fatal("cursor shown while confirming delete")
	}
	press(m, "n", "e")
	checkCursor(t, m)
	press(m, "enter") // unchanged: closes
	if m.View().Cursor != nil {
		t.Fatal("cursor shown after saving")
	}
}

func TestCursorColumnUsesDisplayWidth(t *testing.T) {
	tests := []struct {
		typed string
		left  int // presses of ←
		dx    int // cells right of the label
	}{
		{"", 0, 0},
		{"abc", 0, 3},
		// Hangul syllables are 2 cells wide; the cursor column counts cells.
		{"한글", 0, 4},
		{"a한b", 0, 4},
		{"한글ab", 2, 4},
		{"한글ab", 3, 2},
		{"a한b글", 4, 0},
	}
	for _, tt := range tests {
		m, _ := setup(t, sample)
		m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
		press(m, "a")
		typeText(m, tt.typed)
		for range tt.left {
			press(m, "left")
		}
		wantCursorX(t, m, tt.dx)
	}
}

func TestCursorRowFollowsLayout(t *testing.T) {
	m, path := setup(t, sample)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	moveTo(t, m, "- [ ] Set up CI cache")
	press(m, "e")
	y := checkCursor(t, m).Y
	wantCursorX(t, m, ansi.StringWidth("Set up CI cache"))

	// A conflict notice appears above the input: the cursor moves down one row.
	writeExternal(t, path, strings.Replace(sample, "- [ ] Set up CI cache\n", "- [x] Set up CI cache done:2026-10-23\n", 1))
	typeText(m, "!")
	press(m, "enter")
	if m.status == "" {
		t.Fatal("no conflict notice")
	}
	if got := checkCursor(t, m).Y; got != y+1 {
		t.Fatalf("cursor row %d, want %d", got, y+1)
	}
}

func TestCursorRowWithScrolledList(t *testing.T) {
	var b strings.Builder
	b.WriteString("# TODO\n")
	for i := range 40 {
		b.WriteString("- [ ] item " + strings.Repeat("x", i%5+1) + "\n")
	}
	m, _ := setup(t, b.String())
	m.Update(tea.WindowSizeMsg{Width: 50, Height: 12})
	for range 25 {
		press(m, "j")
	}
	press(m, "a")
	typeText(m, "New task")
	cur := checkCursor(t, m)
	if n := len(strings.Split(screen(m), "\n")); n > 12 || cur.Y >= 12 {
		t.Fatalf("%d lines, cursor row %d for height 12", n, cur.Y)
	}
}

func TestCursorLongInputScrollsHorizontally(t *testing.T) {
	m, _ := setup(t, sample)
	const width = 24
	m.Update(tea.WindowSizeMsg{Width: width, Height: 20})
	press(m, "a")
	// Hangul syllables are 2 cells wide, so scrolling must count cells, not runes.
	long := strings.Repeat("가나다라마바사", 5) // 35 syllables, 70 cells
	typeText(m, long)
	cur := checkCursor(t, m)
	line := strings.Split(screen(m), "\n")[cur.Y]
	if ansi.StringWidth(line) > width {
		t.Fatalf("input line wider than screen: %q", line)
	}
	if left := ansi.Cut(line, labelWidth(m), cur.X); !strings.HasSuffix(left, "바사") {
		t.Fatalf("last typed text not just left of the cursor: %q", left)
	}

	// Typing ASCII at the end keeps the cursor inside and right after it.
	typeText(m, "xyz")
	cur = checkCursor(t, m)
	line = strings.Split(screen(m), "\n")[cur.Y]
	if left := ansi.Cut(line, labelWidth(m), cur.X); !strings.HasSuffix(left, "사xyz") {
		t.Fatalf("left of cursor = %q", left)
	}

	// Moving left through the whole text scrolls back to the start.
	for range len([]rune(long)) + 3 {
		press(m, "left")
		checkCursor(t, m)
	}
	wantCursorX(t, m, 0)
	line = strings.Split(screen(m), "\n")[checkCursor(t, m).Y]
	if !strings.HasPrefix(line, "Add › 가나다") {
		t.Fatalf("start not shown: %q", line)
	}

	// Back to the end, then delete most of it: the view scrolls back.
	for range len([]rune(long)) + 3 {
		press(m, "right")
		checkCursor(t, m)
	}
	for range len([]rune(long)) {
		press(m, "backspace")
		checkCursor(t, m)
	}
	if m.input.Value() != "가나다" {
		t.Fatalf("value = %q", m.input.Value())
	}
	wantCursorX(t, m, 6)
}

func TestCursorResizeKeepsCursorInside(t *testing.T) {
	m, _ := setup(t, sample)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	press(m, "a")
	typeText(m, strings.Repeat("한글", 20)) // 2-cell syllables at tiny widths
	checkCursor(t, m)
	m.Update(tea.WindowSizeMsg{Width: 20, Height: 20})
	checkCursor(t, m)
	m.Update(tea.WindowSizeMsg{Width: 9, Height: 20}) // barely room for the label
	checkCursor(t, m)
}
