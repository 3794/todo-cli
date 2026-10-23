package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

// Long headings and items are wrapped onto several screen lines. Text is
// split into cells (grapheme clusters) that carry their own style, so a style
// such as a tag color or the faint look of a done item continues on the next
// line even when a word is split in the middle.

type cellStyle int

const (
	plain cellStyle = iota
	tag
	due
	done
	heading
)

var cellStyles = map[cellStyle]lipgloss.Style{
	tag:     tagStyle,
	due:     dueStyle,
	done:    doneStyle,
	heading: headingStyle,
}

type cell struct {
	s     string
	w     int // display width (a Hangul syllable is 2)
	style cellStyle
}

func (c cell) space() bool { return c.s == " " }

// toCells splits text into cells of one style.
func toCells(text string, style cellStyle) []cell {
	var out []cell
	g := uniseg.NewGraphemes(text)
	for g.Next() {
		s := g.Str()
		out = append(out, cell{s: s, w: ansi.StringWidth(s), style: style})
	}
	return out
}

// itemCells styles an item body: #tags and due: dates are highlighted, and a
// done item is faint throughout.
func itemCells(text string, isDone bool) []cell {
	if isDone {
		return toCells(text, done)
	}
	var out []cell
	for _, tok := range tokenRe.FindAllString(text, -1) {
		style := plain
		switch {
		case len(tok) > 1 && tok[0] == '#':
			style = tag
		case strings.HasPrefix(tok, "due:"):
			style = due
		}
		out = append(out, toCells(tok, style)...)
	}
	return out
}

func cellsWidth(cs []cell) int {
	w := 0
	for _, c := range cs {
		w += c.w
	}
	return w
}

// wrapCells breaks cells into lines at most width wide, breaking at spaces
// where possible and inside a word only when the word alone is too wide.
// Spaces at a break are dropped. It always returns at least one line. A
// single cell wider than width gets a line of its own.
func wrapCells(cs []cell, width int) [][]cell {
	width = max(1, width)
	var lines [][]cell
	var cur []cell
	curW := 0
	flush := func() {
		n := len(cur)
		for n > 0 && cur[n-1].space() {
			n-- // spaces at a break are dropped
		}
		lines = append(lines, cur[:n])
		cur, curW = nil, 0
	}
	for i := 0; i < len(cs); {
		// The next run: spaces, or a word.
		j := i + 1
		for j < len(cs) && cs[j].space() == cs[i].space() {
			j++
		}
		run, runW := cs[i:j], cellsWidth(cs[i:j])
		i = j

		if run[0].space() {
			if len(cur) == 0 && len(lines) > 0 {
				continue // no leading spaces on a continuation line
			}
			if curW+runW > width {
				if len(cur) > 0 {
					flush()
				}
				continue
			}
			cur, curW = append(cur, run...), curW+runW
			continue
		}

		switch {
		case curW+runW <= width:
			cur, curW = append(cur, run...), curW+runW
		case runW <= width:
			flush()
			cur, curW = append(cur, run...), runW
		default: // a word longer than a whole line: split it
			if len(cur) > 0 && !onlySpaces(cur) {
				flush()
			}
			for _, c := range run {
				if curW+c.w > width && len(cur) > 0 {
					flush()
				}
				cur, curW = append(cur, c), curW+c.w
			}
		}
	}
	if len(cur) > 0 || len(lines) == 0 {
		flush()
	}
	return lines
}

func onlySpaces(cs []cell) bool {
	for _, c := range cs {
		if !c.space() {
			return false
		}
	}
	return true
}

// renderCells draws cells, grouping neighbors with the same style.
func renderCells(cs []cell) string {
	var b strings.Builder
	for i := 0; i < len(cs); {
		j := i
		var run strings.Builder
		for j < len(cs) && cs[j].style == cs[i].style {
			run.WriteString(cs[j].s)
			j++
		}
		if st, ok := cellStyles[cs[i].style]; ok {
			b.WriteString(st.Render(run.String()))
		} else {
			b.WriteString(run.String())
		}
		i = j
	}
	return b.String()
}
