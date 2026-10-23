package tui

import (
	"os"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// The README shows a screenshot of the TUI as text. It must be what the TUI
// really draws for the README's own example file, so a change to the screen
// fails this test until the README is updated.

const (
	readmeWidth  = 62
	readmeHeight = 30 // more than enough: the frame is only as tall as its content
)

func readReadme(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// readmeBlock returns the body of the fenced block that follows heading.
func readmeBlock(t *testing.T, readme []byte, heading, lang string) string {
	t.Helper()
	b := readme
	re := regexp.MustCompile("(?s)\n" + regexp.QuoteMeta(heading) + "\n.*?\n```" + lang + "\n(.*?)\n```\n")
	m := re.FindSubmatch(b)
	if m == nil {
		t.Fatalf("README.md has no ```%s block under %q", lang, heading)
	}
	return string(m[1])
}

// renderReadmeExample draws the File format example as `todo` shows it, with
// the cursor on the first item.
func renderReadmeExample(t *testing.T, readme []byte) string {
	t.Helper()
	content := readmeBlock(t, readme, "## File format", "markdown") + "\n"
	dir := t.TempDir()
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(wd) })

	if err := os.WriteFile("TODO.md", []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := New("TODO.md") // relative, so the title reads "TODO.md"
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: readmeWidth, Height: readmeHeight})
	press(m, "j") // from the first heading to the first item
	return screen(m)
}

// normalize drops trailing spaces on each line, which Markdown editors tend
// to strip, and trailing blank lines.
func normalize(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

func TestReadmeScreenMatchesTUI(t *testing.T) {
	readme := readReadme(t)
	want := normalize(readmeBlock(t, readme, "## What it looks like", "text"))
	got := normalize(renderReadmeExample(t, readme))
	if got != want {
		t.Fatalf("README screenshot is out of date. The TUI draws:\n\n%s\n\nREADME has:\n\n%s", got, want)
	}
}
