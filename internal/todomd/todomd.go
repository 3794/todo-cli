// Package todomd parses TODO.md files and edits them line by line.
//
// All functions are pure: they never modify their inputs and return new
// values instead. Lines that are not touched by an edit are kept byte for
// byte, including the presence or absence of the final newline.
package todomd

import (
	"errors"
	"regexp"
	"strings"
)

// Kind is the kind of a line in a TODO.md file.
type Kind int

const (
	// Other is any line that is neither a heading nor an item. It is never
	// shown on screen and is always preserved.
	Other Kind = iota
	// Heading is a section heading ("## name").
	Heading
	// Item is a checklist item ("- [ ] text").
	Item
)

var (
	// ErrOutOfRange is returned when a line index is outside the document.
	ErrOutOfRange = errors.New("todomd: line index out of range")
	// ErrNotItem is returned when an item operation targets a non-item line.
	ErrNotItem = errors.New("todomd: line is not an item")
	// ErrNotAnchor is returned when AddItem is anchored on a line that is
	// neither a heading nor an item.
	ErrNotAnchor = errors.New("todomd: line is not a heading or an item")
)

var itemRe = regexp.MustCompile(`^(\s*)- \[(.)\] (.*)$`)

// Line is a parsed line.
type Line struct {
	Kind   Kind
	Raw    string // the original line, unchanged
	Indent string // leading whitespace (items only)
	Mark   string // character between the brackets (items only)
	Text   string // item body, or heading name
	CR     string // "\r" when the line ended with CRLF, otherwise ""
}

// ParseLine parses a single line (without its "\n").
func ParseLine(s string) Line {
	l := Line{Kind: Other, Raw: s}
	body := s
	if strings.HasSuffix(body, "\r") {
		body = body[:len(body)-1]
		l.CR = "\r"
	}
	if name, ok := strings.CutPrefix(body, "## "); ok {
		l.Kind = Heading
		l.Text = name
		return l
	}
	if m := itemRe.FindStringSubmatch(body); m != nil {
		l.Kind = Item
		l.Indent, l.Mark, l.Text = m[1], m[2], m[3]
	}
	return l
}

// Done reports whether the item is checked ("[x]" or "[X]").
// Any other mark, such as "[/]", counts as not done.
func (l Line) Done() bool {
	return l.Kind == Item && (l.Mark == "x" || l.Mark == "X")
}

// String reassembles the line. Lines that are not items or headings are
// returned unchanged.
func (l Line) String() string {
	switch l.Kind {
	case Item:
		return l.Indent + "- [" + l.Mark + "] " + l.Text + l.CR
	case Heading:
		return "## " + l.Text + l.CR
	default:
		return l.Raw
	}
}

// Doc is a file split into lines.
type Doc struct {
	Lines           []string
	TrailingNewline bool
}

// Split splits file content into lines. Join(Split(s)) == s for any s.
func Split(content string) Doc {
	if content == "" {
		return Doc{Lines: []string{}}
	}
	d := Doc{TrailingNewline: strings.HasSuffix(content, "\n")}
	if d.TrailingNewline {
		content = content[:len(content)-1]
	}
	d.Lines = strings.Split(content, "\n")
	return d
}

// String joins the lines back into file content.
func (d Doc) String() string {
	s := strings.Join(d.Lines, "\n")
	if d.TrailingNewline && len(d.Lines) > 0 {
		s += "\n"
	}
	return s
}

// Parse parses every line of the document.
func (d Doc) Parse() []Line {
	out := make([]Line, len(d.Lines))
	for i, s := range d.Lines {
		out[i] = ParseLine(s)
	}
	return out
}

// IndentWidth returns the visual width of leading whitespace.
// A tab counts as 4 columns.
func IndentWidth(indent string) int {
	w := 0
	for _, r := range indent {
		switch r {
		case '\t':
			w += 4
		case ' ':
			w++
		default:
			return w
		}
	}
	return w
}

func leadingSpace(s string) string {
	return s[:len(s)-len(strings.TrimLeft(s, " \t"))]
}

// SubtreeEnd returns the index just past item i and the lines directly below
// it that are indented deeper (its sub-items and notes). Blank lines end the
// subtree.
func SubtreeEnd(lines []string, i int) int {
	base := IndentWidth(ParseLine(lines[i]).Indent)
	j := i + 1
	for j < len(lines) {
		s := strings.TrimSuffix(lines[j], "\r")
		if strings.TrimSpace(s) == "" || IndentWidth(leadingSpace(s)) <= base {
			break
		}
		j++
	}
	return j
}

// FindLine returns the index of the line equal to target that is closest to
// hint, or -1 if there is none. On a tie the earlier line wins.
func FindLine(lines []string, target string, hint int) int {
	best := -1
	for i, s := range lines {
		if s != target {
			continue
		}
		if best < 0 || abs(i-hint) < abs(best-hint) {
			best = i
		}
	}
	return best
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// CleanText turns user input into a single line: line breaks become spaces.
func CleanText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.ReplaceAll(s, "\r", " ")
}

// splice returns a copy of d whose lines are lines[:at] + insert + lines[end:].
func (d Doc) splice(at, end int, insert ...string) Doc {
	lines := make([]string, 0, len(d.Lines)-(end-at)+len(insert))
	lines = append(lines, d.Lines[:at]...)
	lines = append(lines, insert...)
	lines = append(lines, d.Lines[end:]...)
	return Doc{Lines: lines, TrailingNewline: d.TrailingNewline}
}

func (d Doc) item(i int) (Line, error) {
	if i < 0 || i >= len(d.Lines) {
		return Line{}, ErrOutOfRange
	}
	l := ParseLine(d.Lines[i])
	if l.Kind != Item {
		return Line{}, ErrNotItem
	}
	return l, nil
}

// AddItem inserts "- [ ] text" and returns the new document and the index of
// the new line.
//
//   - anchor < 0: at the end of the file
//   - anchor is a heading: right below the heading
//   - anchor is an item: below the item and its sub-items, with the same indent
func AddItem(d Doc, anchor int, text string) (Doc, int, error) {
	text = CleanText(text)
	if anchor < 0 {
		cr := ""
		if n := len(d.Lines); n > 0 {
			cr = ParseLine(d.Lines[n-1]).CR
		}
		nd := d.splice(len(d.Lines), len(d.Lines), "- [ ] "+text+cr)
		if len(d.Lines) == 0 {
			nd.TrailingNewline = true
		}
		return nd, len(d.Lines), nil
	}
	if anchor >= len(d.Lines) {
		return Doc{}, 0, ErrOutOfRange
	}
	a := ParseLine(d.Lines[anchor])
	switch a.Kind {
	case Heading:
		return d.splice(anchor+1, anchor+1, "- [ ] "+text+a.CR), anchor + 1, nil
	case Item:
		at := SubtreeEnd(d.Lines, anchor)
		return d.splice(at, at, a.Indent+"- [ ] "+text+a.CR), at, nil
	default:
		return Doc{}, 0, ErrNotAnchor
	}
}

// EditItem replaces the body of item i, keeping its indent and mark.
func EditItem(d Doc, i int, text string) (Doc, error) {
	l, err := d.item(i)
	if err != nil {
		return Doc{}, err
	}
	l.Text = CleanText(text)
	return d.splice(i, i+1, l.String()), nil
}

var (
	doneTokenRe = regexp.MustCompile(`^done:\d{4}-\d{2}-\d{2}$`)
	wordRe      = regexp.MustCompile(`[^ \t]+|[ \t]+`)
)

func isBlank(tok string) bool { return strings.Trim(tok, " \t") == "" }

// RemoveDone removes every "done:YYYY-MM-DD" word from an item body, together
// with the whitespace in front of it. Other words starting with "done:" stay.
func RemoveDone(text string) string {
	var out []string
	dropSpace := false // a done token was the first word: drop the space after it
	for _, tok := range wordRe.FindAllString(text, -1) {
		switch {
		case doneTokenRe.MatchString(tok):
			if n := len(out); n > 0 && isBlank(out[n-1]) {
				out = out[:n-1]
			} else if n == 0 {
				dropSpace = true
			}
		case dropSpace && isBlank(tok):
			dropSpace = false
		default:
			dropSpace = false
			out = append(out, tok)
		}
	}
	return strings.Join(out, "")
}

// WithDoneOf returns text with its done: dates replaced by those in from.
// It keeps an edit consistent with the check state of a line that changed
// state in the meantime.
func WithDoneOf(text, from string) string {
	body := strings.TrimRight(RemoveDone(text), " \t")
	for _, w := range strings.Fields(from) {
		if doneTokenRe.MatchString(w) {
			if body != "" {
				body += " "
			}
			body += w
		}
	}
	return body
}

// looseKey identifies an item regardless of its check mark and done: dates.
func looseKey(s string) (string, bool) {
	l := ParseLine(s)
	if l.Kind != Item {
		return "", false
	}
	return l.Indent + "\x00" + strings.TrimRight(RemoveDone(l.Text), " \t"), true
}

// FindLineLoose is FindLine, but when no line equals target exactly and target
// is an item, it falls back to the closest item with the same indent and the
// same body once the check mark and done: dates are ignored. Use it only
// where a near miss is harmless (e.g. as a position hint).
func FindLineLoose(lines []string, target string, hint int) int {
	if i := FindLine(lines, target, hint); i >= 0 {
		return i
	}
	key, ok := looseKey(target)
	if !ok {
		return -1
	}
	best := -1
	for i, s := range lines {
		if k, ok := looseKey(s); ok && k == key && (best < 0 || abs(i-hint) < abs(best-hint)) {
			best = i
		}
	}
	return best
}

// ToggleItem flips item i between done and not done. Marking it done sets
// "[x]" and appends " done:<today>" (replacing any old done: date); marking it
// not done sets "[ ]" and removes the done: dates.
func ToggleItem(d Doc, i int, today string) (Doc, error) {
	l, err := d.item(i)
	if err != nil {
		return Doc{}, err
	}
	if l.Done() {
		l.Mark = " "
		l.Text = RemoveDone(l.Text)
	} else {
		l.Mark = "x"
		l.Text = WithDoneOf(l.Text, "done:"+today)
	}
	return d.splice(i, i+1, l.String()), nil
}

// DeleteItem removes item i together with its sub-items.
func DeleteItem(d Doc, i int) (Doc, error) {
	if _, err := d.item(i); err != nil {
		return Doc{}, err
	}
	return d.splice(i, SubtreeEnd(d.Lines, i)), nil
}
