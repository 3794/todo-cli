package todomd

import (
	"errors"
	"slices"
	"testing"
)

const sample = "# TODO\n" +
	"\n" +
	"Note: paragraphs outside the format are kept as is.\n" +
	"\n" +
	"## Auth\n" +
	"- [ ] Handle token expiry #auth due:2026-10-24\n" +
	"  - [ ] Call the refresh API\n" +
	"    a note\n" +
	"- [x] Login screen done:2026-10-22\n" +
	"\n" +
	"## Deploy\n" +
	"- [ ] Set up CI cache\n"

func TestRoundTripIsByteIdentical(t *testing.T) {
	cases := map[string]string{
		"empty":               "",
		"only newline":        "\n",
		"trailing newline":    sample,
		"no trailing":         sample[:len(sample)-1],
		"double trailing":     sample + "\n",
		"markdown and others": "# Tasks\n\n> quote *star* [link](x)\n- [/] in progress\n\t- [ ] tab indent\n- [ ]nospace\n",
		// Hangul is multi-byte UTF-8 (3 bytes per syllable); it must survive byte for byte.
		"multi-byte utf-8":   "# TODO\n- [ ] 한글 항목\n",
		"crlf":               "# TODO\r\n- [ ] one\r\n- [x] two done:2026-01-01\r\n",
		"whitespace lines":   "  \n\t\n- [ ] a\n   ",
		"no heading no item": "just a sentence",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if got := Split(in).String(); got != in {
				t.Fatalf("round trip changed content:\n got %q\nwant %q", got, in)
			}
			// Parsing and reassembling every line must also be lossless.
			d := Split(in)
			for i, l := range d.Parse() {
				if l.String() != d.Lines[i] {
					t.Fatalf("line %d: reassembled %q, want %q", i, l.String(), d.Lines[i])
				}
			}
		})
	}
}

func TestParseLine(t *testing.T) {
	tests := []struct {
		in     string
		kind   Kind
		indent string
		mark   string
		text   string
		done   bool
	}{
		{"## Auth", Heading, "", "", "Auth", false},
		{"### sub", Other, "", "", "", false},
		{"# TODO", Other, "", "", "", false},
		{"- [ ] task #tag due:2026-10-24", Item, "", " ", "task #tag due:2026-10-24", false},
		{"- [x] finished", Item, "", "x", "finished", true},
		{"- [X] finished", Item, "", "X", "finished", true},
		{"- [/] in progress", Item, "", "/", "in progress", false},
		{"  - [ ] sub", Item, "  ", " ", "sub", false},
		{"\t- [ ] tab", Item, "\t", " ", "tab", false},
		{"- [ ] ", Item, "", " ", "", false},
		{"- [ ]", Other, "", "", "", false},
		{"* [ ] star list", Other, "", "", "", false},
		{"- [ ] [link] *emphasis*", Item, "", " ", "[link] *emphasis*", false},
	}
	for _, tt := range tests {
		l := ParseLine(tt.in)
		if l.Kind != tt.kind || l.Indent != tt.indent || l.Mark != tt.mark || l.Text != tt.text || l.Done() != tt.done {
			t.Errorf("ParseLine(%q) = %+v (done=%v)", tt.in, l, l.Done())
		}
	}
}

// assertOnlyChanged checks that after equals before with lines[at:at+del]
// replaced by ins.
func assertOnlyChanged(t *testing.T, before, after Doc, at, del int, ins ...string) {
	t.Helper()
	want := slices.Concat(before.Lines[:at], ins, before.Lines[at+del:])
	if !slices.Equal(after.Lines, want) {
		t.Fatalf("lines differ:\n got %q\nwant %q", after.Lines, want)
	}
	if after.TrailingNewline != before.TrailingNewline {
		t.Fatalf("trailing newline changed: %v -> %v", before.TrailingNewline, after.TrailingNewline)
	}
}

func lineIndex(t *testing.T, d Doc, s string) int {
	t.Helper()
	i := slices.Index(d.Lines, s)
	if i < 0 {
		t.Fatalf("line %q not found", s)
	}
	return i
}

func TestAddItem(t *testing.T) {
	d := Split(sample)
	orig := slices.Clone(d.Lines)

	t.Run("below item and its sub-items", func(t *testing.T) {
		i := lineIndex(t, d, "- [ ] Handle token expiry #auth due:2026-10-24")
		nd, at, err := AddItem(d, i, "new item")
		if err != nil {
			t.Fatal(err)
		}
		want := lineIndex(t, d, "- [x] Login screen done:2026-10-22")
		if at != want {
			t.Fatalf("inserted at %d, want %d", at, want)
		}
		assertOnlyChanged(t, d, nd, at, 0, "- [ ] new item")
	})

	t.Run("below sub-item keeps indent", func(t *testing.T) {
		i := lineIndex(t, d, "  - [ ] Call the refresh API")
		nd, at, err := AddItem(d, i, "sub added")
		if err != nil {
			t.Fatal(err)
		}
		if at != i+2 { // after the deeper-indented note
			t.Fatalf("inserted at %d, want %d", at, i+2)
		}
		assertOnlyChanged(t, d, nd, at, 0, "  - [ ] sub added")
	})

	t.Run("below heading", func(t *testing.T) {
		i := lineIndex(t, d, "## Deploy")
		nd, at, err := AddItem(d, i, "Deploy item")
		if err != nil {
			t.Fatal(err)
		}
		if at != i+1 {
			t.Fatalf("inserted at %d, want %d", at, i+1)
		}
		assertOnlyChanged(t, d, nd, at, 0, "- [ ] Deploy item")
	})

	t.Run("end of file", func(t *testing.T) {
		nd, at, err := AddItem(d, -1, "at the end")
		if err != nil {
			t.Fatal(err)
		}
		assertOnlyChanged(t, d, nd, len(d.Lines), 0, "- [ ] at the end")
		if at != len(d.Lines) {
			t.Fatalf("at = %d", at)
		}
	})

	t.Run("empty file", func(t *testing.T) {
		nd, _, err := AddItem(Split(""), -1, "first item")
		if err != nil {
			t.Fatal(err)
		}
		if got := nd.String(); got != "- [ ] first item\n" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("new file with title only", func(t *testing.T) {
		nd, _, err := AddItem(Split("# TODO\n"), -1, "first item")
		if err != nil {
			t.Fatal(err)
		}
		if got := nd.String(); got != "# TODO\n- [ ] first item\n" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("no trailing newline stays without one", func(t *testing.T) {
		nd, _, err := AddItem(Split("# TODO\n- [ ] a"), -1, "b")
		if err != nil {
			t.Fatal(err)
		}
		if got := nd.String(); got != "# TODO\n- [ ] a\n- [ ] b" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("newlines in input become spaces", func(t *testing.T) {
		nd, at, err := AddItem(d, -1, "one\nline\r\nonly")
		if err != nil {
			t.Fatal(err)
		}
		if nd.Lines[at] != "- [ ] one line only" {
			t.Fatalf("got %q", nd.Lines[at])
		}
	})

	t.Run("crlf file", func(t *testing.T) {
		in := "## A\r\n- [ ] a\r\n"
		nd, _, err := AddItem(Split(in), 1, "b")
		if err != nil {
			t.Fatal(err)
		}
		if got := nd.String(); got != "## A\r\n- [ ] a\r\n- [ ] b\r\n" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("anchor on other line", func(t *testing.T) {
		if _, _, err := AddItem(d, 0, "x"); !errors.Is(err, ErrNotAnchor) {
			t.Fatalf("err = %v", err)
		}
		if _, _, err := AddItem(d, 999, "x"); !errors.Is(err, ErrOutOfRange) {
			t.Fatalf("err = %v", err)
		}
	})

	if !slices.Equal(d.Lines, orig) {
		t.Fatal("input slice was modified")
	}
}

func TestEditItem(t *testing.T) {
	d := Split(sample)
	orig := slices.Clone(d.Lines)
	tests := []struct{ line, text, want string }{
		{"  - [ ] Call the refresh API", "Retry the refresh API", "  - [ ] Retry the refresh API"},
		{"- [x] Login screen done:2026-10-22", "Login screen v2 done:2026-10-22", "- [x] Login screen v2 done:2026-10-22"},
		{"- [ ] Set up CI cache", "two\nlines", "- [ ] two lines"},
	}
	for _, tt := range tests {
		i := lineIndex(t, d, tt.line)
		nd, err := EditItem(d, i, tt.text)
		if err != nil {
			t.Fatal(err)
		}
		assertOnlyChanged(t, d, nd, i, 1, tt.want)
	}
	if _, err := EditItem(d, lineIndex(t, d, "## Auth"), "x"); !errors.Is(err, ErrNotItem) {
		t.Fatalf("editing heading: err = %v", err)
	}
	if _, err := EditItem(d, -1, "x"); !errors.Is(err, ErrOutOfRange) {
		t.Fatalf("err = %v", err)
	}
	if !slices.Equal(d.Lines, orig) {
		t.Fatal("input slice was modified")
	}
}

func TestToggleItemRoundTrip(t *testing.T) {
	d := Split(sample)
	orig := slices.Clone(d.Lines)
	i := lineIndex(t, d, "  - [ ] Call the refresh API")

	done, err := ToggleItem(d, i, "2026-10-23")
	if err != nil {
		t.Fatal(err)
	}
	assertOnlyChanged(t, d, done, i, 1, "  - [x] Call the refresh API done:2026-10-23")

	undone, err := ToggleItem(done, i, "2026-10-23")
	if err != nil {
		t.Fatal(err)
	}
	if undone.String() != sample {
		t.Fatalf("round trip differs:\n%q", undone.String())
	}
	if !slices.Equal(d.Lines, orig) {
		t.Fatal("input slice was modified")
	}
}

func TestToggleItemCases(t *testing.T) {
	tests := []struct{ in, want string }{
		{"- [x] Login screen done:2026-10-22", "- [ ] Login screen"},
		{"- [X] uppercase done:2026-10-22 #tag", "- [ ] uppercase #tag"},
		{"- [x] no date", "- [ ] no date"},
		{"- [x] done:2026-10-22 in front", "- [ ] in front"},
		{"- [/] in progress", "- [x] in progress done:2026-10-23"},
		{"- [ ] stale done:2026-01-01 token", "- [x] stale token done:2026-10-23"},
		{"- [ ] ", "- [x] done:2026-10-23"},
		{"- [ ] undone:keep", "- [x] undone:keep done:2026-10-23"},
		{"- [ ] crlf\r", "- [x] crlf done:2026-10-23\r"},
	}
	for _, tt := range tests {
		nd, err := ToggleItem(Doc{Lines: []string{tt.in}}, 0, "2026-10-23")
		if err != nil {
			t.Fatal(err)
		}
		if nd.Lines[0] != tt.want {
			t.Errorf("toggle %q = %q, want %q", tt.in, nd.Lines[0], tt.want)
		}
	}
}

func TestDeleteItem(t *testing.T) {
	d := Split(sample)
	orig := slices.Clone(d.Lines)

	t.Run("with sub-items, siblings kept", func(t *testing.T) {
		i := lineIndex(t, d, "- [ ] Handle token expiry #auth due:2026-10-24")
		nd, err := DeleteItem(d, i)
		if err != nil {
			t.Fatal(err)
		}
		assertOnlyChanged(t, d, nd, i, 3) // item + sub-item + note
	})

	t.Run("sub-item only", func(t *testing.T) {
		i := lineIndex(t, d, "  - [ ] Call the refresh API")
		nd, err := DeleteItem(d, i)
		if err != nil {
			t.Fatal(err)
		}
		assertOnlyChanged(t, d, nd, i, 2)
	})

	t.Run("sibling at same indent is kept", func(t *testing.T) {
		in := Split("- [ ] a\n  - [ ] a1\n  - [ ] a2\n- [ ] b\n")
		nd, err := DeleteItem(in, 1)
		if err != nil {
			t.Fatal(err)
		}
		if got := nd.String(); got != "- [ ] a\n  - [ ] a2\n- [ ] b\n" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("blank line ends subtree", func(t *testing.T) {
		in := Split("- [ ] a\n\n  indented paragraph\n")
		nd, err := DeleteItem(in, 0)
		if err != nil {
			t.Fatal(err)
		}
		if got := nd.String(); got != "\n  indented paragraph\n" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("last line without trailing newline", func(t *testing.T) {
		nd, err := DeleteItem(Split("# T\n- [ ] a"), 1)
		if err != nil {
			t.Fatal(err)
		}
		if got := nd.String(); got != "# T" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("heading is not deletable", func(t *testing.T) {
		if _, err := DeleteItem(d, lineIndex(t, d, "## Deploy")); !errors.Is(err, ErrNotItem) {
			t.Fatalf("err = %v", err)
		}
	})

	if !slices.Equal(d.Lines, orig) {
		t.Fatal("input slice was modified")
	}
}

func TestFindLine(t *testing.T) {
	lines := []string{"a", "- [ ] same", "b", "c", "- [ ] same", "d", "e", "f", "- [ ] same"}
	tests := []struct {
		target string
		hint   int
		want   int
	}{
		{"- [ ] same", 0, 1},
		{"- [ ] same", 3, 4},
		{"- [ ] same", 7, 8},
		{"- [ ] same", 100, 8},
		{"- [ ] same", 6, 4}, // tie between 4 and 8 -> earlier
		{"b", 7, 2},
		{"- [ ] missing", 1, -1},
		{"- [ ] sam", 1, -1}, // must match the whole line
	}
	for _, tt := range tests {
		if got := FindLine(lines, tt.target, tt.hint); got != tt.want {
			t.Errorf("FindLine(%q, %d) = %d, want %d", tt.target, tt.hint, got, tt.want)
		}
	}
	if got := FindLine(nil, "x", 0); got != -1 {
		t.Errorf("empty: got %d", got)
	}
}

func TestSubtreeEnd(t *testing.T) {
	lines := Split("- [ ] a\n  - [ ] a1\n\t- [ ] a2\n    note\n- [ ] b\n  - [ ] b1\n## H\n").Lines
	if got := SubtreeEnd(lines, 0); got != 4 {
		t.Errorf("a: got %d", got)
	}
	// A tab counts as 4 columns, so "\t- [ ] a2" is deeper than a1 (2).
	if got := SubtreeEnd(lines, 1); got != 4 {
		t.Errorf("a1: got %d", got)
	}
	if got := SubtreeEnd(lines, 4); got != 6 {
		t.Errorf("b: got %d", got)
	}
}

func TestRemoveDoneOnlyDates(t *testing.T) {
	tests := []struct{ in, want string }{
		{"check done: meeting", "check done: meeting"},
		{"done:later task", "done:later task"},
		{"work done:2026-10-22", "work"},
		{"done:2026-10-22 in front", "in front"},
		{"a done:2026-01-01 done:2026-01-02 b", "a b"},
		{"a  done:2026-01-01\tb", "a\tb"},
		{"undone:2026-01-01 keep", "undone:2026-01-01 keep"},
		{"done:2026-10-22", ""},
	}
	for _, tt := range tests {
		if got := RemoveDone(tt.in); got != tt.want {
			t.Errorf("RemoveDone(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestWithDoneOf(t *testing.T) {
	tests := []struct{ text, from, want string }{
		{"Tidy the cache setup", "Set up CI cache done:2026-10-23", "Tidy the cache setup done:2026-10-23"},
		{"Tidy done:2026-01-01", "Set up CI cache", "Tidy"},
		{"Tidy done:2026-01-01 ", "x done:2026-10-23", "Tidy done:2026-10-23"},
		{"", "done:2026-10-23", "done:2026-10-23"},
	}
	for _, tt := range tests {
		if got := WithDoneOf(tt.text, tt.from); got != tt.want {
			t.Errorf("WithDoneOf(%q, %q) = %q, want %q", tt.text, tt.from, got, tt.want)
		}
	}
}

func TestFindLineLoose(t *testing.T) {
	lines := []string{
		"## Deploy",
		"- [x] Set up CI cache done:2026-10-23",
		"  - [ ] Set up CI cache",
		"- [/] other task",
		"- [ ] other task done:2026-01-01",
	}
	tests := []struct {
		target string
		hint   int
		want   int
	}{
		{"## Deploy", 3, 0},                               // exact
		{"- [ ] Set up CI cache", 2, 1},                   // checked off elsewhere; indent must match
		{"  - [x] Set up CI cache done:2026-10-22", 0, 2}, // unchecked elsewhere
		{"- [ ] other task", 0, 3},                        // nearest of two loose matches
		{"- [ ] other task", 9, 4},
		{"- [ ] Set up CI", 1, -1}, // body differs
		{"## Auth", 0, -1},         // headings only match exactly
		{"just a paragraph", 0, -1},
	}
	for _, tt := range tests {
		if got := FindLineLoose(lines, tt.target, tt.hint); got != tt.want {
			t.Errorf("FindLineLoose(%q, %d) = %d, want %d", tt.target, tt.hint, got, tt.want)
		}
	}
}
