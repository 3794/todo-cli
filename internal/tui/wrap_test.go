package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func plainLines(lines [][]cell) []string {
	var out []string
	for _, l := range lines {
		var b strings.Builder
		for _, c := range l {
			b.WriteString(c.s)
		}
		out = append(out, b.String())
	}
	return out
}

func TestWrapCells(t *testing.T) {
	tests := []struct {
		text  string
		width int
		want  []string
	}{
		{"", 10, []string{""}},
		{"short", 10, []string{"short"}},
		{"exactly ten", 11, []string{"exactly ten"}},
		{"one two three four", 9, []string{"one two", "three", "four"}},
		{"one  two", 4, []string{"one", "two"}}, // spaces at a break are dropped
		{"  lead spaces stay", 20, []string{"  lead spaces stay"}},
		{"abcdefghij", 4, []string{"abcd", "efgh", "ij"}},
		{"go abcdefghij", 4, []string{"go", "abcd", "efgh", "ij"}},
		// Hangul syllables are 2 cells wide: widths count cells, not runes.
		{"한글 문장을 나눈다", 6, []string{"한글", "문장을", "나눈다"}},
		{"한글문장을나눈다", 5, []string{"한글", "문장", "을나", "눈다"}},
		{"a한b 글c", 4, []string{"a한b", "글c"}},
		{"한", 1, []string{"한"}}, // wider than the line: on its own
		{"x y", 0, []string{"x", "y"}},
	}
	for _, tt := range tests {
		got := plainLines(wrapCells(toCells(tt.text, plain), tt.width))
		if strings.Join(got, "|") != strings.Join(tt.want, "|") {
			t.Errorf("wrap(%q, %d) = %q, want %q", tt.text, tt.width, got, tt.want)
		}
	}
}

// Property: no line is wider than the width (unless one cell is), no text is
// lost apart from spaces at breaks, and continuation lines never start with
// a space.
func TestWrapCellsKeepsText(t *testing.T) {
	texts := []string{
		"Handle token expiry #auth due:2026-10-24 and refresh the session cache",
		"https://example.com/a/very/long/url/without/any/spaces/at/all?x=1",
		"섞인 mixed 문장 with 한글 and English 단어들", // 2-cell Hangul mixed with ASCII
		"   leading   and   repeated   spaces   ",
	}
	for _, text := range texts {
		for width := 1; width <= 40; width++ {
			lines := wrapCells(toCells(text, plain), width)
			for i, l := range lines {
				if w := cellsWidth(l); w > width && len(l) > 1 {
					t.Fatalf("wrap(%q, %d): line %d is %d wide", text, width, i, w)
				}
				if i > 0 && len(l) > 0 && l[0].space() {
					t.Fatalf("wrap(%q, %d): line %d starts with a space", text, width, i)
				}
			}
			joined := strings.Join(plainLines(lines), "")
			if strings.ReplaceAll(joined, " ", "") != strings.ReplaceAll(text, " ", "") {
				t.Fatalf("wrap(%q, %d) lost text: %q", text, width, plainLines(lines))
			}
		}
	}
}

// checkScreen verifies the whole frame fits the window.
func checkScreen(t *testing.T, m *Model) []string {
	t.Helper()
	lines := strings.Split(screen(m), "\n")
	if m.height > 0 && len(lines) > m.height {
		t.Fatalf("%d lines for height %d:\n%s", len(lines), m.height, screen(m))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); m.width > 0 && w > m.width {
			t.Fatalf("line %d is %d wide (width %d): %q", i, w, m.width, l)
		}
	}
	return lines
}

const longItems = "# TODO\n" +
	"## A heading that is much longer than the narrow screen\n" +
	"- [ ] Handle token expiry #auth due:2026-10-24 and refresh the session cache\n" +
	"  - [ ] A long sub-item wraps too and its lines stay indented under its text\n" +
	"- [x] A finished item that is long enough to wrap onto more lines done:2026-10-22\n" +
	"- [ ] short\n"

func TestLongItemsWrapWithAlignedIndent(t *testing.T) {
	m, _ := setup(t, longItems)
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 40})
	lines := checkScreen(t, m)
	s := strings.Join(lines, "\n")

	want := []string{
		"› A heading that is much",       // heading: text after the 2-cell mark
		"  longer than the narrow",       // ... continues 2 cells in
		"  [ ] Handle token expiry",      // item text starts at 6
		"      #auth due:2026-10-24 and", // continuation lines up with it
		"    [ ] A long sub-item wraps",  // sub-item text starts at 8
		"        too and its lines stay", // continuation at 8
		"  [x] A finished item that is",
		"      long enough to wrap onto",
		"  [ ] short",
	}
	for _, w := range want {
		if !strings.Contains(s, w+"\n") && !strings.HasSuffix(s, w) {
			t.Errorf("screen lacks line %q:\n%s", w, s)
		}
	}
	// Nothing is cut: every word of every item is on screen.
	for _, word := range []string{"cache", "text", "done:2026-10-22", "screen"} {
		if !strings.Contains(s, word) {
			t.Errorf("screen lacks %q:\n%s", word, s)
		}
	}
}

func TestWrappedLinesKeepStyle(t *testing.T) {
	sgr := func(st interface{ Render(...string) string }) string {
		return strings.SplitN(st.Render("x"), "x", 2)[0]
	}
	faint, tagColor := sgr(doneStyle), sgr(tagStyle)
	if faint == "" || tagColor == "" {
		t.Skip("styles render without escape codes here")
	}

	m, _ := setup(t, "# TODO\n- [x] finished item with several words done:2026-10-22\n- [ ] see #averyveryverylongtagname now\n")
	m.Update(tea.WindowSizeMsg{Width: 20, Height: 40})
	raw := strings.Split(m.View().Content, "\n")
	plain := strings.Split(screen(m), "\n")

	var doneLines, tagLines []string
	for i, p := range plain {
		switch {
		case strings.Contains(p, "finished") || strings.Contains(p, "several") || strings.Contains(p, "done:"):
			doneLines = append(doneLines, raw[i])
		case strings.Contains(p, "#avery") || strings.Contains(p, "tagname") || strings.Contains(p, "verylong"):
			tagLines = append(tagLines, raw[i])
		}
	}
	if len(doneLines) < 2 || len(tagLines) < 2 {
		t.Fatalf("expected wrapped lines, got done=%d tag=%d:\n%s", len(doneLines), len(tagLines), screen(m))
	}
	for _, l := range doneLines {
		if !strings.Contains(l, faint) {
			t.Errorf("done item line not faint: %q", l)
		}
	}
	for _, l := range tagLines {
		if !strings.Contains(l, tagColor) {
			t.Errorf("tag split across lines lost its color: %q", l)
		}
	}
}

func TestScrollShowsAllLinesOfSelectedItem(t *testing.T) {
	var b strings.Builder
	b.WriteString("# TODO\n")
	for i := range 12 {
		b.WriteString("- [ ] item " + string(rune('A'+i)) + "\n")
	}
	b.WriteString("- [ ] LAST-START this item is long enough to need several lines on a narrow screen LAST-END\n")
	m, _ := setup(t, b.String())
	m.Update(tea.WindowSizeMsg{Width: 24, Height: 10})
	for range 20 {
		press(m, "j")
	}
	lines := checkScreen(t, m)
	s := strings.Join(lines, "\n")
	if !strings.Contains(s, "LAST-START") || !strings.Contains(s, "LAST-END") {
		t.Fatalf("selected item not fully visible:\n%s", s)
	}
	if !strings.Contains(s, "› [ ] LAST-START") {
		t.Fatalf("cursor mark not on the selected item:\n%s", s)
	}

	// Moving back up shows the earlier items again.
	for range 20 {
		press(m, "k")
	}
	if s := strings.Join(checkScreen(t, m), "\n"); !strings.Contains(s, "› [ ] item A") {
		t.Fatalf("top not shown after moving up:\n%s", s)
	}
}

func TestItemTallerThanListShowsItsStart(t *testing.T) {
	m, _ := setup(t, "# TODO\n- [ ] before\n- [ ] START "+strings.Repeat("word ", 40)+"END\n- [ ] after\n")
	m.Update(tea.WindowSizeMsg{Width: 20, Height: 8})
	moveTo(t, m, "- [ ] START "+strings.Repeat("word ", 40)+"END")
	s := strings.Join(checkScreen(t, m), "\n")
	if !strings.Contains(s, "› [ ] START") {
		t.Fatalf("tall selected item doesn't start at its first line:\n%s", s)
	}
}

func TestNarrowScreen(t *testing.T) {
	// A deep Hangul sub-item: at width 10 its text gets 2 cells, exactly one
	// 2-cell syllable per line.
	m, _ := setup(t, longItems+"      - [ ] 깊게 들여쓴 하위 항목\n")
	for _, w := range []int{10, 8, 6, 3} {
		m.Update(tea.WindowSizeMsg{Width: w, Height: 12})
		for range 8 {
			press(m, "j")
			checkScreen(t, m)
		}
		for range 8 {
			press(m, "k")
			checkScreen(t, m)
		}
	}
	m.Update(tea.WindowSizeMsg{Width: 10, Height: 200})
	s := strings.Join(checkScreen(t, m), "\n")
	// The deep sub-item gives up indent and wraps 2 cells wide: one
	// syllable per line.
	if !strings.Contains(s, "깊\n") || !strings.Contains(s, "게\n") || !strings.Contains(s, "[ ] shor\n") {
		t.Fatalf("text lost at width 10:\n%s", s)
	}
}

func TestResizeRewraps(t *testing.T) {
	m, _ := setup(t, longItems)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	wide := len(checkScreen(t, m))
	m.Update(tea.WindowSizeMsg{Width: 24, Height: 20})
	narrow := len(checkScreen(t, m))
	if narrow <= wide {
		t.Fatalf("narrowing didn't add lines: %d -> %d", wide, narrow)
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	if back := len(checkScreen(t, m)); back != wide {
		t.Fatalf("widening again: %d lines, want %d", back, wide)
	}
	// Shrinking the height keeps the selected item visible.
	moveTo(t, m, "- [ ] short")
	m.Update(tea.WindowSizeMsg{Width: 24, Height: 6})
	if s := strings.Join(checkScreen(t, m), "\n"); !strings.Contains(s, "› [ ] short") {
		t.Fatalf("selected item scrolled away after resize:\n%s", s)
	}
}

func TestInputCursorWithWrappedList(t *testing.T) {
	m, _ := setup(t, longItems)
	m.Update(tea.WindowSizeMsg{Width: 24, Height: 9})
	moveTo(t, m, "- [ ] short")
	press(m, "e")
	typeText(m, " and a much longer text that scrolls sideways")
	checkScreen(t, m)
	checkCursor(t, m)
	press(m, "esc")
	moveTo(t, m, "- [ ] Handle token expiry #auth due:2026-10-24 and refresh the session cache")
	press(m, "a")
	typeText(m, "New task")
	checkScreen(t, m)
	checkCursor(t, m)
}
