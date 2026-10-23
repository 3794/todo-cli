package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Scenario tests: the user works in the TUI while Claude edits
// the same TODO.md. writeExternal is used without waiting for a poll, so the
// TUI always acts on a stale screen.

// A realistic interleaving: every external edit and every TUI edit must
// survive in the final file.
func TestInterleavedWithClaude(t *testing.T) {
	m, path := setup(t, sample)

	// Claude adds an item at the end of Deploy.
	writeExternal(t, path, readFile(t, path)+"- [ ] Added by Claude\n")
	// User (stale screen) completes "Set up CI cache".
	moveTo(t, m, "- [ ] Set up CI cache")
	press(m, "space")
	// Claude completes the sub-item.
	writeExternal(t, path, strings.Replace(readFile(t, path),
		"  - [ ] Call the refresh API\n", "  - [x] Call the refresh API done:2026-10-23\n", 1))
	// User (stale screen again) adds an item under the Auth heading.
	moveTo(t, m, "## Auth")
	press(m, "a")
	typeText(m, "Added by the user")
	press(m, "enter")
	// Claude inserts a note line at the top.
	writeExternal(t, path, strings.Replace(readFile(t, path), "# TODO\n", "# TODO\nNote: a paragraph Claude wrote\n", 1))
	// User deletes the finished Login item.
	moveTo(t, m, "- [x] Login screen done:2026-10-22")
	press(m, "d", "y")

	want := "# TODO\n" +
		"Note: a paragraph Claude wrote\n" +
		"\n" +
		"## Auth\n" +
		"- [ ] Added by the user\n" +
		"- [ ] Handle token expiry #auth due:2026-10-24\n" +
		"  - [x] Call the refresh API done:2026-10-23\n" +
		"\n" +
		"## Deploy\n" +
		"- [x] Set up CI cache done:2026-10-23\n" +
		"- [ ] Added by Claude\n"
	wantFile(t, path, want)
	assertNoTempFiles(t, path)
}

// The user is typing a new item below an item; meanwhile Claude checks that
// item off. What happens to the text the user typed?
func TestAddWhileClaudeChangesAnchor(t *testing.T) {
	m, path := setup(t, sample)
	moveTo(t, m, "- [ ] Set up CI cache")
	press(m, "a")
	typeText(m, "My new task")
	writeExternal(t, path, strings.Replace(readFile(t, path),
		"- [ ] Set up CI cache\n", "- [x] Set up CI cache done:2026-10-23\n", 1))
	press(m, "enter")
	got := readFile(t, path)
	if !strings.Contains(got, "My new task") {
		t.Errorf("user's new item was dropped because the anchor line changed.\nstatus: %q\nfile:\n%s", m.status, got)
	}
}

// Same for an edit whose target Claude changed: the edit is correctly not
// applied, but is the typed text still recoverable?
func TestEditConflictKeepsTypedText(t *testing.T) {
	m, path := setup(t, sample)
	moveTo(t, m, "- [ ] Set up CI cache")
	press(m, "e")
	typeText(m, " (clean up cache keys)")
	writeExternal(t, path, strings.Replace(readFile(t, path),
		"- [ ] Set up CI cache\n", "- [x] Set up CI cache done:2026-10-23\n", 1))
	press(m, "enter")
	if strings.Contains(readFile(t, path), "clean up cache keys") {
		t.Fatalf("conflicting edit was applied")
	}
	if m.mode != modeEdit || !strings.Contains(m.input.Value(), "clean up cache keys") {
		t.Errorf("typed text lost on conflict (mode=%d, input=%q, status=%q)", m.mode, m.input.Value(), m.status)
	}
}

// CRLF file edited by both sides keeps CRLF everywhere.
func TestCRLFStaysCRLF(t *testing.T) {
	crlf := strings.ReplaceAll(sample, "\n", "\r\n")
	m, path := setup(t, crlf)
	moveTo(t, m, "- [ ] Set up CI cache\r")
	press(m, "space")
	press(m, "a")
	typeText(m, "New task")
	press(m, "enter")
	got := readFile(t, path)
	if strings.Count(got, "\n") != strings.Count(got, "\r\n") {
		t.Fatalf("mixed line endings:\n%q", got)
	}
}

func assertNoTempFiles(t *testing.T, path string) {
	t.Helper()
	ents, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if e.Name() != filepath.Base(path) {
			t.Errorf("leftover file: %s", e.Name())
		}
	}
}
