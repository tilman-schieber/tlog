package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// editor is a minimal multi-line text buffer for one block. Blocks are the
// editing unit, so this is deliberately not a general text editor: it knows
// about lines and words and nothing else. It exists because a block can hold a
// code fence, and a single-line input cannot.
type editor struct {
	runes   []rune
	cur     int
	goalCol int

	// The block's own undo, for typing that has not been written yet: a
	// snapshot before each change, with a run of letters kept as one, so
	// undo takes back a word rather than a character. Once the block is
	// left, what was typed is a write, and the core's undo has it.
	undos, redos []edSnap
	lastKind     string
}

type edSnap struct {
	runes []rune
	cur   int
}

// remember takes a snapshot before a change. Typing letter after letter, or
// deleting one after another, continues the step already begun; anything
// else starts a new one.
func (e *editor) remember(kind string) {
	e.redos = nil
	if kind == e.lastKind && (kind == "type" || kind == "delete") && len(e.undos) > 0 {
		return
	}
	e.lastKind = kind
	e.undos = append(e.undos, edSnap{runes: append([]rune(nil), e.runes...), cur: e.cur})
	if len(e.undos) > 200 {
		e.undos = e.undos[1:]
	}
}

// undo takes back the last step of typing, reporting false when there is
// none left in this block.
func (e *editor) undo() bool {
	if len(e.undos) == 0 {
		return false
	}
	last := e.undos[len(e.undos)-1]
	e.undos = e.undos[:len(e.undos)-1]
	e.redos = append(e.redos, edSnap{runes: append([]rune(nil), e.runes...), cur: e.cur})
	e.runes, e.cur, e.lastKind, e.goalCol = last.runes, last.cur, "", -1
	return true
}

func (e *editor) redo() bool {
	if len(e.redos) == 0 {
		return false
	}
	next := e.redos[len(e.redos)-1]
	e.redos = e.redos[:len(e.redos)-1]
	e.undos = append(e.undos, edSnap{runes: append([]rune(nil), e.runes...), cur: e.cur})
	e.runes, e.cur, e.lastKind, e.goalCol = next.runes, next.cur, "", -1
	return true
}

func newEditor(s string) *editor {
	r := []rune(s)
	return &editor{runes: r, cur: len(r)}
}

func (e *editor) String() string { return string(e.runes) }

func (e *editor) insert(s string) {
	if strings.TrimSpace(s) == "" {
		e.remember("space")
	} else {
		e.remember("insert")
	}
	e.put(s)
}

func (e *editor) put(s string) {
	r := []rune(s)
	e.runes = append(e.runes[:e.cur], append(r, e.runes[e.cur:]...)...)
	e.cur += len(r)
	e.goalCol = -1
}

// textAfter is everything from the caret on.
func (e *editor) textAfter() string { return string(e.runes[e.cur:]) }

// typeRune is typing one character, with brackets made to pair: "[[" closes
// itself, so a link is typed as [[name and the ]] is already there; typing "]"
// in front of a "]" steps over it, so typing the closing brackets out of habit
// does not double them.
func (e *editor) typeRune(s string) {
	switch {
	case s == "]" && strings.HasPrefix(e.textAfter(), "]"):
		e.right()
	case s == "[" && strings.HasSuffix(e.textBefore(), "[") && !strings.HasPrefix(e.textAfter(), "]]"):
		e.remember("type")
		e.put("[]]")
		e.cur -= 2
	case strings.TrimSpace(s) == "":
		e.remember("space")
		e.put(s)
	default:
		e.remember("type")
		e.put(s)
	}
}

// backspace deletes the rune before the cursor, reporting false when there is
// nothing to delete — which the caller turns into "merge with the block above".
func (e *editor) backspace() bool {
	if e.cur == 0 {
		return false
	}
	e.remember("delete")
	// Inside an empty pair, both halves go: [[|]] backs out to [|] and then to
	// nothing, rather than leaving a stray ]] behind.
	if e.runes[e.cur-1] == '[' && e.cur < len(e.runes) && e.runes[e.cur] == ']' {
		e.runes = append(e.runes[:e.cur-1], e.runes[e.cur+1:]...)
	} else {
		e.runes = append(e.runes[:e.cur-1], e.runes[e.cur:]...)
	}
	e.cur--
	e.goalCol = -1
	return true
}

func (e *editor) del() {
	if e.cur >= len(e.runes) {
		return
	}
	e.remember("delete")
	e.runes = append(e.runes[:e.cur], e.runes[e.cur+1:]...)
	e.goalCol = -1
}

// deleteWord removes the word before the cursor.
func (e *editor) deleteWord() {
	e.remember("cut")
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
	e.remember("cut")
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

// setCaret puts the caret at a line and a screen column of it, clamped to
// what is there. A click and a move by screen line both know columns rather
// than characters, and a wide character takes two.
func (e *editor) setCaret(line, col int) {
	i := 0
	for ; line > 0 && i < len(e.runes); i++ {
		if e.runes[i] == '\n' {
			line--
		}
	}
	end := e.lineEnd(i)
	c := 0
	for i < end && c < col {
		c += ansi.StringWidth(string(e.runes[i]))
		i++
	}
	e.cur = i
	e.goalCol = 0
}

// lineText is the text of one line of the block.
func (e *editor) lineText(n int) string {
	i := 0
	for ; n > 0 && i < len(e.runes); i++ {
		if e.runes[i] == '\n' {
			n--
		}
	}
	return string(e.runes[i:e.lineEnd(i)])
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
	e.remember("replace")
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
