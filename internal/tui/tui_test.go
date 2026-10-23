package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/3794/todo-cli/internal/todomd"
)

const sample = "# TODO\n" +
	"\n" +
	"## Auth\n" +
	"- [ ] Handle token expiry #auth due:2026-10-24\n" +
	"  - [ ] Call the refresh API\n" +
	"- [x] Login screen done:2026-10-22\n" +
	"\n" +
	"## Deploy\n" +
	"- [ ] Set up CI cache\n"

func setup(t *testing.T, content string) (*Model, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "TODO.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	m.now = func() time.Time { return time.Date(2026, 10, 23, 9, 0, 0, 0, time.Local) }
	return m, path
}

func keyMsg(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	}
	r := []rune(k)
	if len(r) != 1 {
		panic("unknown key " + k)
	}
	return tea.KeyPressMsg{Code: r[0], Text: k}
}

func press(m *Model, keys ...string) tea.Cmd {
	var cmd tea.Cmd
	for _, k := range keys {
		_, cmd = m.Update(keyMsg(k))
	}
	return cmd
}

func typeText(m *Model, s string) {
	for _, r := range s {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

// moveTo moves the cursor with j/k until it is on the line raw.
func moveTo(t *testing.T, m *Model, raw string) {
	t.Helper()
	for range len(m.rows) {
		press(m, "k")
	}
	for range len(m.rows) {
		if r, ok := m.current(); ok && r.line.Raw == raw {
			return
		}
		press(m, "j")
	}
	t.Fatalf("no row %q on screen:\n%s", raw, screen(m))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// writeExternal simulates another program (e.g. Claude) editing the file.
func writeExternal(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	// Make sure the poller sees a new mtime even on coarse filesystems.
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
}

func screen(m *Model) string { return ansi.Strip(m.View().Content) }

func wantFile(t *testing.T, path, want string) {
	t.Helper()
	if got := readFile(t, path); got != want {
		t.Fatalf("file content:\n got %q\nwant %q", got, want)
	}
}

func TestAddBelowItem(t *testing.T) {
	m, path := setup(t, sample)
	moveTo(t, m, "- [ ] Handle token expiry #auth due:2026-10-24")
	press(m, "a")
	typeText(m, "New task")
	press(m, "enter")
	wantFile(t, path, strings.Replace(sample,
		"  - [ ] Call the refresh API\n",
		"  - [ ] Call the refresh API\n- [ ] New task\n", 1))
	if r, _ := m.current(); r.line.Raw != "- [ ] New task" {
		t.Fatalf("cursor on %q, want the new item", r.line.Raw)
	}
}

func TestAddBelowHeading(t *testing.T) {
	m, path := setup(t, sample)
	moveTo(t, m, "## Deploy")
	press(m, "a")
	typeText(m, "Prepare the deploy")
	press(m, "enter")
	wantFile(t, path, strings.Replace(sample, "## Deploy\n", "## Deploy\n- [ ] Prepare the deploy\n", 1))
}

func TestAddToEmptyFile(t *testing.T) {
	m, path := setup(t, "# TODO\n")
	if !strings.Contains(screen(m), "(no items — press a to add)") {
		t.Fatalf("empty screen:\n%s", screen(m))
	}
	press(m, "a")
	typeText(m, "First item")
	press(m, "enter")
	wantFile(t, path, "# TODO\n- [ ] First item\n")
	if !strings.Contains(screen(m), "[ ] First item") {
		t.Fatalf("screen:\n%s", screen(m))
	}
}

func TestEditKeepsIndentAndState(t *testing.T) {
	m, path := setup(t, sample)
	moveTo(t, m, "  - [ ] Call the refresh API")
	press(m, "e")
	if got := m.input.Value(); got != "Call the refresh API" {
		t.Fatalf("input prefilled with %q", got)
	}
	typeText(m, " with retries")
	press(m, "enter")
	wantFile(t, path, strings.Replace(sample, "  - [ ] Call the refresh API\n", "  - [ ] Call the refresh API with retries\n", 1))

	moveTo(t, m, "- [x] Login screen done:2026-10-22")
	press(m, "enter") // Enter also edits
	for range len("done:2026-10-22") {
		press(m, "backspace")
	}
	typeText(m, "v2")
	press(m, "enter")
	if got := readFile(t, path); !strings.Contains(got, "\n- [x] Login screen v2\n") {
		t.Fatalf("file:\n%s", got)
	}
}

func TestToggleRoundTrip(t *testing.T) {
	m, path := setup(t, sample)
	moveTo(t, m, "- [ ] Set up CI cache")
	press(m, "space")
	wantFile(t, path, strings.Replace(sample, "- [ ] Set up CI cache\n", "- [x] Set up CI cache done:2026-10-23\n", 1))
	press(m, "space") // cursor stays on the toggled item
	wantFile(t, path, sample)
}

func TestDeleteWithConfirm(t *testing.T) {
	m, path := setup(t, sample)
	moveTo(t, m, "- [ ] Handle token expiry #auth due:2026-10-24")

	press(m, "d", "n")
	wantFile(t, path, sample)
	press(m, "d", "esc")
	wantFile(t, path, sample)

	press(m, "d")
	if !strings.Contains(screen(m), "(y/n)") {
		t.Fatalf("no confirmation shown:\n%s", screen(m))
	}
	press(m, "y")
	wantFile(t, path, strings.Replace(sample,
		"- [ ] Handle token expiry #auth due:2026-10-24\n  - [ ] Call the refresh API\n", "", 1))
	if r, _ := m.current(); r.line.Raw != "- [x] Login screen done:2026-10-22" {
		t.Fatalf("cursor on %q after delete", r.line.Raw)
	}
}

func TestHeadingIgnoresItemKeys(t *testing.T) {
	m, path := setup(t, sample)
	moveTo(t, m, "## Auth")
	press(m, "e", "space", "d", "enter")
	if m.mode != modeNormal {
		t.Fatalf("mode = %v", m.mode)
	}
	wantFile(t, path, sample)
}

func TestEmptyInputAndEscDoNotSave(t *testing.T) {
	m, path := setup(t, sample)
	moveTo(t, m, "- [ ] Set up CI cache")
	press(m, "a")
	typeText(m, "   ")
	press(m, "enter")
	wantFile(t, path, sample)

	press(m, "a")
	typeText(m, "Item to cancel")
	press(m, "esc")
	wantFile(t, path, sample)

	press(m, "e")
	for range len([]rune("Set up CI cache")) {
		press(m, "backspace")
	}
	press(m, "enter")
	wantFile(t, path, sample)
	if m.mode != modeNormal {
		t.Fatalf("mode = %v", m.mode)
	}
}

func TestPasteWithNewlines(t *testing.T) {
	m, path := setup(t, "# TODO\n")
	press(m, "a")
	m.Update(tea.PasteMsg{Content: "two\nlines"})
	press(m, "enter")
	got := readFile(t, path)
	if strings.Count(got, "\n") != 2 || !strings.HasPrefix(got, "# TODO\n- [ ] two") {
		t.Fatalf("file: %q", got)
	}
}

func TestExternalLinesSurviveTUIEdits(t *testing.T) {
	m, path := setup(t, sample)
	moveTo(t, m, "- [ ] Set up CI cache")

	// Claude edits the file; the TUI hasn't polled yet and still shows the
	// old content with old line numbers.
	external := strings.Replace(sample, "# TODO\n", "# TODO\n\n> Claude's note\n", 1)
	external = strings.Replace(external, "## Deploy\n", "## Deploy\n- [ ] Item added by Claude\n", 1)
	writeExternal(t, path, external)

	press(m, "space")
	afterToggle := strings.Replace(external, "- [ ] Set up CI cache\n", "- [x] Set up CI cache done:2026-10-23\n", 1)
	wantFile(t, path, afterToggle)

	press(m, "a")
	typeText(m, "TUI item")
	press(m, "enter")
	wantFile(t, path, afterToggle+"- [ ] TUI item\n")

	if !strings.Contains(screen(m), "Item added by Claude") {
		t.Fatalf("screen doesn't show external item:\n%s", screen(m))
	}

	// External change again, then delete an item that moved.
	external2 := strings.Replace(readFile(t, path), "## Auth\n", "## Auth\n- [ ] Another external item\n", 1)
	writeExternal(t, path, external2)
	moveTo(t, m, "- [x] Login screen done:2026-10-22")
	press(m, "d", "y")
	wantFile(t, path, strings.Replace(external2, "- [x] Login screen done:2026-10-22\n", "", 1))
}

func TestEditCancelledWhenItemDeletedExternally(t *testing.T) {
	m, path := setup(t, sample)
	moveTo(t, m, "- [ ] Set up CI cache")
	press(m, "e")
	typeText(m, " being edited")

	external := strings.Replace(sample, "- [ ] Set up CI cache\n", "", 1) + "- [ ] External item\n"
	writeExternal(t, path, external)

	// The poll refreshes the screen but keeps the input.
	m.Update(tickMsg{})
	if m.mode != modeEdit || m.input.Value() != "Set up CI cache being edited" {
		t.Fatalf("input lost: mode=%v value=%q", m.mode, m.input.Value())
	}
	if !strings.Contains(screen(m), "External item") {
		t.Fatalf("screen not refreshed:\n%s", screen(m))
	}

	// Enter: nothing is written, the typed text stays, a notice is shown.
	press(m, "enter")
	wantFile(t, path, external)
	if m.mode != modeEdit || m.input.Value() != "Set up CI cache being edited" {
		t.Fatalf("typed text lost: mode=%v value=%q", m.mode, m.input.Value())
	}
	if !strings.Contains(screen(m), "Item removed elsewhere — not saved. Esc to close") {
		t.Fatalf("notice not shown:\n%s", screen(m))
	}
	press(m, "enter") // still nothing to save onto
	wantFile(t, path, external)
	press(m, "esc")
	if m.mode != modeNormal {
		t.Fatalf("mode = %v after esc", m.mode)
	}
	wantFile(t, path, external)
}

func TestEditCancelledWhenItemChangedExternally(t *testing.T) {
	m, path := setup(t, sample)
	moveTo(t, m, "- [ ] Set up CI cache")
	press(m, "d")
	external := strings.Replace(sample, "- [ ] Set up CI cache\n", "- [x] Set up CI cache done:2026-10-23\n", 1)
	writeExternal(t, path, external)
	press(m, "y")
	wantFile(t, path, external)
}

func TestRefreshShowsExternalChange(t *testing.T) {
	m, path := setup(t, sample)
	moveTo(t, m, "- [ ] Set up CI cache")

	m.Update(tickMsg{}) // no change: nothing happens
	external := strings.Replace(sample, "## Auth\n", "## Auth\n- [ ] Added externally\n- [ ] One more\n", 1)
	writeExternal(t, path, external)
	if strings.Contains(screen(m), "Added externally") {
		t.Fatal("changed before polling")
	}

	_, cmd := m.Update(tickMsg{})
	if cmd == nil {
		t.Fatal("polling stopped")
	}
	if !strings.Contains(screen(m), "Added externally") {
		t.Fatalf("screen not refreshed:\n%s", screen(m))
	}
	if r, _ := m.current(); r.line.Raw != "- [ ] Set up CI cache" {
		t.Fatalf("cursor moved to %q", r.line.Raw)
	}

	// The item under the cursor disappears: cursor stays near.
	writeExternal(t, path, strings.Replace(external, "- [ ] Set up CI cache\n", "", 1))
	m.Update(tickMsg{})
	if _, ok := m.current(); !ok {
		t.Fatal("no cursor row")
	}
	if strings.Contains(screen(m), "Set up CI cache") {
		t.Fatal("deleted item still shown")
	}
}

func TestMissingFileWhilePolling(t *testing.T) {
	m, path := setup(t, sample)
	os.Remove(path)
	m.Update(tickMsg{})
	if !strings.Contains(screen(m), "File not found: "+path) {
		t.Fatalf("screen:\n%s", screen(m))
	}
	writeExternal(t, path, "# TODO\n- [ ] Back again\n")
	m.Update(tickMsg{})
	if !strings.Contains(screen(m), "Back again") {
		t.Fatalf("screen:\n%s", screen(m))
	}
}

func TestViewShowsTextLiterally(t *testing.T) {
	m, _ := setup(t, "# TODO\n## Section\n- [ ] [link](x) *star* `code` #tag due:2026-10-24\n  - [/] in progress\n- [ ] danger\x1b[31mtext\nHidden paragraph\n")
	s := screen(m)
	for _, want := range []string{"Section", "[ ] [link](x) *star* `code` #tag due:2026-10-24", "    [/] in progress", "danger�[31mtext"} {
		if !strings.Contains(s, want) {
			t.Errorf("screen missing %q:\n%s", want, s)
		}
	}
	for _, hidden := range []string{"# TODO", "Hidden paragraph"} {
		if strings.Contains(s, hidden) {
			t.Errorf("screen shows %q:\n%s", hidden, s)
		}
	}
	if strings.Contains(m.View().Content, "\x1b[31mtext") {
		t.Error("raw escape sequence from file reached the terminal")
	}
}

func TestCursorMovementAndQuit(t *testing.T) {
	m, _ := setup(t, sample)
	press(m, "k")
	if m.cursor != 0 {
		t.Fatalf("cursor = %d", m.cursor)
	}
	press(m, "j", "down", "j", "j", "j", "j", "j", "j")
	if m.cursor != len(m.rows)-1 {
		t.Fatalf("cursor = %d, want last (%d)", m.cursor, len(m.rows)-1)
	}
	press(m, "up")
	if m.cursor != len(m.rows)-2 {
		t.Fatalf("cursor = %d", m.cursor)
	}
	cmd := press(m, "q")
	if cmd == nil {
		t.Fatal("q returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q did not quit")
	}

	// q inside the input is just a letter.
	press(m, "a", "q")
	if m.input.Value() != "q" {
		t.Fatalf("input = %q", m.input.Value())
	}
}

func TestScrollKeepsCursorVisible(t *testing.T) {
	var b strings.Builder
	b.WriteString("# TODO\n")
	for i := range 50 {
		b.WriteString("- [ ] item " + string(rune('A'+i%26)) + "\n")
	}
	m, _ := setup(t, b.String())
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	for range 30 {
		press(m, "j")
	}
	lines := strings.Split(screen(m), "\n")
	if len(lines) > 10 {
		t.Fatalf("%d lines for height 10", len(lines))
	}
	if !strings.Contains(screen(m), "›") {
		t.Fatalf("cursor not visible:\n%s", screen(m))
	}
}

func TestWriteAtomicRefusesChangedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "TODO.md")
	os.WriteFile(path, []byte("new content\n"), 0o600)
	if err := writeAtomic(path, []byte("my version\n"), []byte("old content\n")); err != errChanged {
		t.Fatalf("err = %v", err)
	}
	wantFile(t, path, "new content\n")

	if err := writeAtomic(path, []byte("my version\n"), []byte("new content\n")); err != nil {
		t.Fatal(err)
	}
	wantFile(t, path, "my version\n")
	fi, _ := os.Stat(path)
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", fi.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("temp files left behind: %v", entries)
	}
}

func TestWriteAtomicFollowsSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.md")
	link := filepath.Join(dir, "TODO.md")
	os.WriteFile(real, []byte("a\n"), 0o644)
	if err := os.Symlink(real, link); err != nil {
		t.Skip(err)
	}
	if err := writeAtomic(link, []byte("b\n"), []byte("a\n")); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink replaced by a regular file")
	}
	wantFile(t, real, "b\n")
}

func TestAddAnchorCheckedOffElsewhereStillAddsBelowIt(t *testing.T) {
	m, path := setup(t, sample)
	moveTo(t, m, "- [ ] Handle token expiry #auth due:2026-10-24")
	press(m, "a")
	typeText(m, "New task")
	external := strings.Replace(sample, "- [ ] Handle token expiry #auth due:2026-10-24\n",
		"- [x] Handle token expiry #auth due:2026-10-24 done:2026-10-23\n", 1)
	writeExternal(t, path, external)
	press(m, "enter")
	wantFile(t, path, strings.Replace(external, "  - [ ] Call the refresh API\n", "  - [ ] Call the refresh API\n- [ ] New task\n", 1))
	if m.mode != modeNormal || m.status != "" {
		t.Fatalf("mode=%v status=%q", m.mode, m.status)
	}
}

func TestAddAnchorGoneAppendsAtEnd(t *testing.T) {
	m, path := setup(t, sample)
	moveTo(t, m, "- [ ] Handle token expiry #auth due:2026-10-24")
	press(m, "a")
	typeText(m, "New task")
	external := strings.Replace(sample, "- [ ] Handle token expiry #auth due:2026-10-24\n  - [ ] Call the refresh API\n", "", 1)
	writeExternal(t, path, external)
	press(m, "enter")
	wantFile(t, path, external+"- [ ] New task\n")
	if !strings.Contains(screen(m), "Anchor line gone — added at end of file") {
		t.Fatalf("no notice:\n%s", screen(m))
	}
	if r, _ := m.current(); r.line.Raw != "- [ ] New task" {
		t.Fatalf("cursor on %q", r.line.Raw)
	}
}

func TestAddHeadingAnchorRenamedAppendsAtEnd(t *testing.T) {
	m, path := setup(t, sample)
	moveTo(t, m, "## Deploy")
	press(m, "a")
	typeText(m, "New task")
	external := strings.Replace(sample, "## Deploy\n", "## Release\n", 1)
	writeExternal(t, path, external)
	press(m, "enter")
	wantFile(t, path, external+"- [ ] New task\n")
}

func TestEditConflictRetargetsAndSavesOnSecondEnter(t *testing.T) {
	m, path := setup(t, sample)
	moveTo(t, m, "- [ ] Set up CI cache")
	press(m, "e")
	typeText(m, " tidied")
	external := strings.Replace(sample, "- [ ] Set up CI cache\n", "- [x] Set up CI cache done:2026-10-23\n", 1)
	writeExternal(t, path, external)

	press(m, "enter")
	wantFile(t, path, external)
	if m.mode != modeEdit || !strings.Contains(screen(m), "Item changed elsewhere — not saved. Enter: save to updated item · Esc: cancel") {
		t.Fatalf("mode=%v screen:\n%s", m.mode, screen(m))
	}
	if r, _ := m.current(); r.line.Raw != "- [x] Set up CI cache done:2026-10-23" {
		t.Fatalf("cursor not on the changed item: %q", r.line.Raw)
	}

	press(m, "enter") // save onto the item as it is now, keeping its check state
	wantFile(t, path, strings.Replace(external, "- [x] Set up CI cache done:2026-10-23\n", "- [x] Set up CI cache tidied done:2026-10-23\n", 1))
	if m.mode != modeNormal {
		t.Fatalf("mode = %v", m.mode)
	}
}

func TestEditConflictRetargetDoesNotWriteIfChangedAgain(t *testing.T) {
	m, path := setup(t, sample)
	moveTo(t, m, "- [ ] Set up CI cache")
	press(m, "e")
	typeText(m, " tidied")
	writeExternal(t, path, strings.Replace(sample, "- [ ] Set up CI cache\n", "- [x] Set up CI cache done:2026-10-23\n", 1))
	press(m, "enter")
	external := strings.Replace(sample, "- [ ] Set up CI cache\n", "- [ ] Set up CI cache changed\n", 1)
	writeExternal(t, path, external)
	press(m, "enter")
	wantFile(t, path, external)
	if m.mode != modeEdit || m.input.Value() != "Set up CI cache tidied" {
		t.Fatalf("mode=%v value=%q", m.mode, m.input.Value())
	}
}

func TestWriteFailureKeepsTypedText(t *testing.T) {
	m, path := setup(t, sample)
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	moveTo(t, m, "- [ ] Set up CI cache")
	press(m, "a")
	typeText(m, "Unsaved text")
	press(m, "enter")
	if m.mode != modeAdd || m.input.Value() != "Unsaved text" || !strings.Contains(screen(m), "Save failed: ") {
		t.Fatalf("mode=%v value=%q screen:\n%s", m.mode, m.input.Value(), screen(m))
	}
	wantFile(t, path, sample)

	os.Chmod(dir, 0o755)
	press(m, "enter")
	wantFile(t, path, sample+"- [ ] Unsaved text\n")
}

func TestScreenTextIsEnglish(t *testing.T) {
	m, path := setup(t, sample)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if !strings.Contains(screen(m), "j/k move · a add · e edit · space done · d delete · q quit") {
		t.Fatalf("help:\n%s", screen(m))
	}
	moveTo(t, m, "- [ ] Set up CI cache")
	press(m, "d")
	for _, want := range []string{"Delete this item and its sub-items? (y/n)", "y delete · n/Esc cancel"} {
		if !strings.Contains(screen(m), want) {
			t.Fatalf("missing %q:\n%s", want, screen(m))
		}
	}
	press(m, "n", "a")
	if !strings.Contains(screen(m), "Add › ") || !strings.Contains(screen(m), "Enter save · Esc cancel") {
		t.Fatalf("add mode:\n%s", screen(m))
	}
	press(m, "enter")
	if !strings.Contains(screen(m), "Empty — not saved") {
		t.Fatalf("empty input:\n%s", screen(m))
	}
	press(m, "e")
	if !strings.Contains(screen(m), "Edit › Set up CI cache") {
		t.Fatalf("edit mode:\n%s", screen(m))
	}
	press(m, "esc")
	writeExternal(t, path, strings.Replace(sample, "- [ ] Set up CI cache\n", "", 1))
	press(m, "space")
	if !strings.Contains(screen(m), "Item changed or removed elsewhere — cancelled") {
		t.Fatalf("toggle conflict:\n%s", screen(m))
	}

	// Only the user's own text may be non-ASCII; every UI string is English.
	ui := screen(m)
	for _, r := range m.doc.Lines {
		ui = strings.ReplaceAll(ui, todomd.ParseLine(r).Text, "")
	}
	for _, r := range ui {
		if r >= 0xAC00 && r <= 0xD7A3 {
			t.Fatalf("Hangul in UI text:\n%s", ui)
		}
	}
}

// Korean jamo are not shortcuts: a Korean IME holds a jamo while composing
// and sends it with the next key, so a mapping would act a key late.
func TestJamoAreNotShortcuts(t *testing.T) {
	m, path := setup(t, sample)
	moveTo(t, m, "- [ ] Set up CI cache")
	before := m.cursor
	for _, k := range []string{"ㅓ", "ㅏ", "ㅁ", "ㄷ", "ㅇ", "ㅂ", "ㅛ", "ㅜ", "한"} {
		if cmd := press(m, k); cmd != nil {
			if _, ok := cmd().(tea.QuitMsg); ok {
				t.Fatalf("%s quit", k)
			}
		}
		if m.mode != modeNormal || m.cursor != before {
			t.Fatalf("%s: mode=%v cursor=%d", k, m.mode, m.cursor)
		}
	}
	wantFile(t, path, sample)

	// In the delete prompt they are ignored too.
	press(m, "d", "ㅛ", "ㅜ")
	if m.mode != modeConfirmDelete {
		t.Fatalf("mode = %v", m.mode)
	}
	press(m, "n")
	wantFile(t, path, sample)
}

// A Korean IME delivers composed Hangul syllables as key text, 3 bytes each
// in UTF-8; they must be stored unchanged.
func TestTypeHangul(t *testing.T) {
	m, path := setup(t, "# TODO\n")
	press(m, "a")
	typeText(m, "한글 입력")
	press(m, "enter")
	wantFile(t, path, "# TODO\n- [ ] 한글 입력\n")
}
