package tui

import "strings"

// editor is a minimal multi-line text buffer for one block. Blocks are the
// editing unit, so this is deliberately not a general text editor: it knows
// about lines and words and nothing else. It exists because a block can hold a
// code fence, and a single-line input cannot.
type editor struct {
	runes   []rune
	cur     int
	goalCol int
}

func newEditor(s string) *editor {
	r := []rune(s)
	return &editor{runes: r, cur: len(r)}
}

func (e *editor) String() string { return string(e.runes) }

func (e *editor) insert(s string) {
	r := []rune(s)
	e.runes = append(e.runes[:e.cur], append(r, e.runes[e.cur:]...)...)
	e.cur += len(r)
	e.goalCol = -1
}

// backspace deletes the rune before the cursor, reporting false when there is
// nothing to delete — which the caller turns into "merge with the block above".
func (e *editor) backspace() bool {
	if e.cur == 0 {
		return false
	}
	e.runes = append(e.runes[:e.cur-1], e.runes[e.cur:]...)
	e.cur--
	e.goalCol = -1
	return true
}

func (e *editor) del() {
	if e.cur >= len(e.runes) {
		return
	}
	e.runes = append(e.runes[:e.cur], e.runes[e.cur+1:]...)
	e.goalCol = -1
}

// deleteWord removes the word before the cursor.
func (e *editor) deleteWord() {
	i := e.cur
	for i > 0 && isSpace(e.runes[i-1]) {
		i--
	}
	for i > 0 && !isSpace(e.runes[i-1]) {
		i--
	}
	e.runes = append(e.runes[:i], e.runes[e.cur:]...)
	e.cur = i
	e.goalCol = -1
}

// deleteToLineStart removes everything from the start of the current line.
func (e *editor) deleteToLineStart() {
	start := e.lineStart(e.cur)
	e.runes = append(e.runes[:start], e.runes[e.cur:]...)
	e.cur = start
	e.goalCol = -1
}

func (e *editor) left() {
	if e.cur > 0 {
		e.cur--
	}
	e.goalCol = -1
}

func (e *editor) right() {
	if e.cur < len(e.runes) {
		e.cur++
	}
	e.goalCol = -1
}

func (e *editor) home() {
	e.cur = e.lineStart(e.cur)
	e.goalCol = -1
}

func (e *editor) end() {
	e.cur = e.lineEnd(e.cur)
	e.goalCol = -1
}

// up moves to the previous line, keeping the column the cursor was aiming for.
// It reports false at the first line so the caller can move to the block above.
func (e *editor) up() bool {
	start := e.lineStart(e.cur)
	if start == 0 {
		return false
	}
	col := e.column()
	prevStart := e.lineStart(start - 1)
	prevEnd := start - 1
	e.cur = min(prevStart+col, prevEnd)
	e.goalCol = col
	return true
}

// down moves to the next line, reporting false at the last line.
func (e *editor) down() bool {
	end := e.lineEnd(e.cur)
	if end >= len(e.runes) {
		return false
	}
	col := e.column()
	nextStart := end + 1
	nextEnd := e.lineEnd(nextStart)
	e.cur = min(nextStart+col, nextEnd)
	e.goalCol = col
	return true
}

// column is the cursor's column, or the column it has been aiming for while
// moving vertically past shorter lines.
func (e *editor) column() int {
	c := e.cur - e.lineStart(e.cur)
	if e.goalCol > c {
		return e.goalCol
	}
	return c
}

func (e *editor) lineStart(i int) int {
	for i > 0 && e.runes[i-1] != '\n' {
		i--
	}
	return i
}

func (e *editor) lineEnd(i int) int {
	for i < len(e.runes) && e.runes[i] != '\n' {
		i++
	}
	return i
}

func (e *editor) atStart() bool { return e.cur == 0 }
func (e *editor) atEnd() bool   { return e.cur == len(e.runes) }

func (e *editor) empty() bool { return len(strings.TrimSpace(string(e.runes))) == 0 }

// cursorPos returns the zero-based line and column of the cursor, for drawing.
func (e *editor) cursorPos() (line, col int) {
	for i := 0; i < e.cur; i++ {
		if e.runes[i] == '\n' {
			line++
			col = 0
			continue
		}
		col++
	}
	return line, col
}

// split cuts the buffer at the cursor and returns both halves. Enter uses it to
// break a block in two.
func (e *editor) split() (before, after string) {
	return string(e.runes[:e.cur]), string(e.runes[e.cur:])
}

// textBefore returns the text to the left of the cursor, which is what
// completion triggers are detected in.
func (e *editor) textBefore() string { return string(e.runes[:e.cur]) }

// replaceBefore swaps the n runes before the cursor for s.
func (e *editor) replaceBefore(n int, s string) {
	if n > e.cur {
		n = e.cur
	}
	r := []rune(s)
	e.runes = append(e.runes[:e.cur-n], append(r, e.runes[e.cur:]...)...)
	e.cur = e.cur - n + len(r)
	e.goalCol = -1
}

func isSpace(r rune) bool { return r == ' ' || r == '\t' || r == '\n' }

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
