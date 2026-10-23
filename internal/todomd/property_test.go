package todomd

import (
	"math/rand"
	"slices"
	"strings"
	"testing"
)

// Property test: random documents, random edits,
// and every line outside the edit must survive byte for byte.

var pool = []string{
	"# TODO", "", "   ", "## Auth", "## Deploy", "just a paragraph", "> quote",
	"- [ ] task #tag due:2026-10-24", "- [x] finished task done:2026-10-22",
	"  - [ ] sub-item", "\t- [X] tab sub", "  - note line", "    deeper note",
	"- [/] in progress @claude", "- [?] question", "- [ ] `code` [link](x) *emphasis*",
	"- [ ]", "* [ ] other bullet", "## ", "- [ ] same task", "- [ ] same task",
}

func randomDoc(r *rand.Rand) Doc {
	n := r.Intn(14)
	lines := make([]string, n)
	crlf := r.Intn(4) == 0
	for i := range lines {
		lines[i] = pool[r.Intn(len(pool))]
		if crlf {
			lines[i] += "\r"
		}
	}
	return Doc{Lines: lines, TrailingNewline: r.Intn(2) == 0}
}

func itemIdx(d Doc) []int {
	var out []int
	for i, l := range d.Parse() {
		if l.Kind == Item {
			out = append(out, i)
		}
	}
	return out
}

func TestRandomEditsPreserveOtherLines(t *testing.T) {
	r := rand.New(rand.NewSource(20261023))
	for iter := 0; iter < 20000; iter++ {
		d := randomDoc(r)
		orig := slices.Clone(d.Lines)
		s := d.String()
		if Split(s).String() != s {
			t.Fatalf("round trip broke: %q", s)
		}
		items := itemIdx(d)
		switch op := r.Intn(4); op {
		case 0: // toggle
			if len(items) == 0 {
				continue
			}
			i := items[r.Intn(len(items))]
			nd, err := ToggleItem(d, i, "2026-10-23")
			if err != nil {
				t.Fatal(err)
			}
			mustOnlyLineChanged(t, orig, nd, i)
			back, _ := ToggleItem(nd, i, "2026-10-23")
			if pl := ParseLine(back.Lines[i]); pl.Kind != Item || pl.Indent != ParseLine(orig[i]).Indent || pl.CR != ParseLine(orig[i]).CR {
				t.Fatalf("toggle twice changed indent/CR: %q -> %q", orig[i], back.Lines[i])
			}
		case 1: // edit
			if len(items) == 0 {
				continue
			}
			i := items[r.Intn(len(items))]
			nd, err := EditItem(d, i, "new text\ntwo lines")
			if err != nil {
				t.Fatal(err)
			}
			mustOnlyLineChanged(t, orig, nd, i)
			pl := ParseLine(nd.Lines[i])
			op := ParseLine(orig[i])
			if pl.Text != "new text two lines" || pl.Mark != op.Mark || pl.Indent != op.Indent || pl.CR != op.CR {
				t.Fatalf("edit result wrong: %q -> %q", orig[i], nd.Lines[i])
			}
		case 2: // delete
			if len(items) == 0 {
				continue
			}
			i := items[r.Intn(len(items))]
			end := SubtreeEnd(orig, i)
			nd, err := DeleteItem(d, i)
			if err != nil {
				t.Fatal(err)
			}
			want := append(slices.Clone(orig[:i]), orig[end:]...)
			if !slices.Equal(nd.Lines, want) || nd.TrailingNewline != d.TrailingNewline {
				t.Fatalf("delete %d of %q: got %q", i, orig, nd.Lines)
			}
			base := IndentWidth(ParseLine(orig[i]).Indent)
			for _, s := range orig[i+1 : end] {
				s = strings.TrimSuffix(s, "\r")
				if strings.TrimSpace(s) == "" || IndentWidth(leadingSpace(s)) <= base {
					t.Fatalf("delete removed a non-child line %q from %q", s, orig)
				}
			}
		case 3: // add
			anchor := -1
			var anchors []int
			for i, l := range d.Parse() {
				if l.Kind != Other {
					anchors = append(anchors, i)
				}
			}
			if len(anchors) > 0 && r.Intn(4) != 0 {
				anchor = anchors[r.Intn(len(anchors))]
			}
			nd, at, err := AddItem(d, anchor, "added task")
			if err != nil {
				t.Fatal(err)
			}
			if len(nd.Lines) != len(orig)+1 {
				t.Fatalf("add changed line count wrongly")
			}
			rest := append(slices.Clone(nd.Lines[:at]), nd.Lines[at+1:]...)
			if !slices.Equal(rest, orig) {
				t.Fatalf("add disturbed other lines: %q -> %q", orig, nd.Lines)
			}
			pl := ParseLine(nd.Lines[at])
			if pl.Kind != Item || pl.Text != "added task" || pl.Mark != " " {
				t.Fatalf("added line is not a fresh item: %q", nd.Lines[at])
			}
			// The added line must show up as an item when the file is re-read.
			re := Split(nd.String())
			if ParseLine(re.Lines[at]).Kind != Item {
				t.Fatalf("added item not an item after write+read: %q", nd.String())
			}
		}
		if !slices.Equal(d.Lines, orig) {
			t.Fatalf("input slice was mutated")
		}
	}
}

func mustOnlyLineChanged(t *testing.T, orig []string, nd Doc, i int) {
	t.Helper()
	if len(nd.Lines) != len(orig) {
		t.Fatalf("line count changed")
	}
	for j := range orig {
		if j != i && nd.Lines[j] != orig[j] {
			t.Fatalf("line %d changed: %q -> %q", j, orig[j], nd.Lines[j])
		}
	}
}
