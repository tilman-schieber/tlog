package tui

import (
	"github.com/charmbracelet/x/ansi"
)

// Soft wrapping. A line of a block longer than the window used to be drawn as
// one line, and the terminal cut it off at the edge: what was typed past it
// was in the file but not on the screen, and the caret went with it. Now a
// long line is wrapped onto as many screen lines as it needs, at the last
// space that fits, or mid-word when a word is wider than the window. Only the
// screen is wrapped; the file still holds one line.
//
// Wrapping works on what is drawn, after colouring, so it measures exactly
// what the terminal will show. Every character drawn lands on exactly one
// screen line — nothing is dropped at a break — so the caret always has a
// cell to stand on and a column on screen always maps back to one in the
// text.

// wrapRanges splits drawn text into screen lines no wider than w cells,
// returned as [start, end) column ranges. It breaks after the last space that
// fits, so a word is never split unless it is wider than a whole line.
func wrapRanges(plain string, w int) [][2]int {
	r := []rune(plain)
	col := make([]int, len(r)+1)
	for i, c := range r {
		col[i+1] = col[i] + ansi.StringWidth(string(c))
	}
	if w <= 0 {
		return [][2]int{{0, col[len(r)]}}
	}
	var out [][2]int
	s := 0
	for col[len(r)]-col[s] > w {
		e := s
		for e < len(r) && col[e+1]-col[s] <= w {
			e++
		}
		if e == s {
			e = s + 1 // one character wider than the line: it gets a line of its own
		}
		// Back up to just after a space. A word that fits only without the
		// space after it goes down too, or the next line would start with
		// that space.
		if e < len(r) {
			b := e
			for b > s && r[b-1] != ' ' {
				b--
			}
			if b > s {
				e = b
			}
		}
		out = append(out, [2]int{col[s], col[e]})
		s = e
	}
	return append(out, [2]int{col[s], col[len(r)]})
}

// lineInfo says where a screen line's text came from: which line of the
// block's text, and the column of that line it starts at. -1 marks a screen
// line that is not text of the block — a property, a menu, a code sample.
type lineInfo struct{ logical, col0 int }

// edSeg is one screen line of the block being edited.
type edSeg struct {
	logical    int // which line of the block's text
	start, end int // columns of that line, [start, end)
	last       bool
}

// edLayout is how the block being edited was last drawn, kept so that ↑ and ↓
// can move by screen line. It is only trusted for the text and width it was
// drawn with.
type edLayout struct {
	segs  []edSeg
	text  string
	width int
}

// cellsTo is how many columns the first n runes of s take on screen.
func cellsTo(s []rune, n int) int {
	c := 0
	for _, r := range s[:min(n, len(s))] {
		c += ansi.StringWidth(string(r))
	}
	return c
}

// moveVisual moves the caret one screen line up or down within the block
// being edited, keeping its column, and reports false when there is no screen
// line in that direction — at the top or bottom of the block — or when the
// block has not been drawn as it now is.
func (m *Model) moveVisual(d int) bool {
	L := m.layout
	if m.ed == nil || len(L.segs) == 0 || L.text != m.ed.String() || L.width != m.width {
		return false
	}
	cl, cc := m.ed.cursorPos()
	line := []rune(m.ed.lineText(cl))
	cell := cellsTo(line, cc)
	v := -1
	for i, s := range L.segs {
		if s.logical == cl && cell >= s.start && (cell < s.end || s.last) {
			v = i
			break
		}
	}
	t := v + d
	if v < 0 || t < 0 || t >= len(L.segs) {
		return false
	}
	target := L.segs[t].start + (cell - L.segs[v].start)
	limit := L.segs[t].end
	if !L.segs[t].last {
		limit-- // the last column belongs to this screen line, the end to the next
	}
	m.ed.setCaret(L.segs[t].logical, min(target, limit))
	return true
}
