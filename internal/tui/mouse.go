package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/tilman-schieber/tlog/internal/markdown"
)

// The mouse is an alternative to the keys, never the only way to do something:
// every click below ends in a function a key already calls. A click is answered
// from the frame that was last drawn — the frame the user actually aimed at —
// which View records line by line in m.hits.

// hit is what one screen line of the outline belongs to.
type hit struct {
	row  int // index into m.rows; -1 for the empty-page placeholder
	line int // line of the block's own text, or -1 for properties and the like
}

func (m *Model) mouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		return m.wheel(-1)
	case tea.MouseButtonWheelDown:
		return m.wheel(1)
	}
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	// A click between the two keys of dd must not arm the second one.
	m.pendingG, m.pendingD, m.pendingR = false, false, false

	switch msg.Button {
	case tea.MouseButtonLeft:
		if m.mode == modeNormal || m.mode == modeInsert {
			m.click(msg.X, msg.Y)
		}
	case tea.MouseButtonBackward:
		if m.mode == modeNormal {
			m.back()
		}
	}
	return m, nil
}

// wheel moves the selection, which is what the keys move too; scrolling the
// view on its own would only be undone by the next draw, which keeps the
// cursor on screen.
func (m *Model) wheel(delta int) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeNormal:
		m.moveCursor(delta)
		return m, nil
	case modeInsert, modeHelp:
		// Scrolling away from a half-typed block, or out of help, is not
		// what anyone means by turning the wheel.
		return m, nil
	}
	key := tea.KeyMsg{Type: tea.KeyDown}
	if delta < 0 {
		key = tea.KeyMsg{Type: tea.KeyUp}
	}
	return m.key(key)
}

// click acts on the outline. On a block that is not selected it selects it; on
// the selected one it opens the [[link]] under the pointer or starts editing
// with the caret where the click landed — the second click is the commitment,
// so a stray first one never navigates away or opens an editor.
func (m *Model) click(x, y int) {
	head := lipgloss.Height(m.header())
	if y < head {
		m.clickHeader(x)
		return
	}
	i := y - head
	if i >= len(m.hits) {
		return
	}
	h := m.hits[i]
	if h.row < 0 {
		if m.mode == modeNormal && len(m.rows) == 0 {
			m.newSibling(false)
		}
		return
	}
	if h.row >= len(m.rows) {
		return
	}
	r := m.rows[h.row]
	col := x - textColumn(r)

	selected := m.mode == modeNormal && m.cur == h.row
	if m.mode == modeInsert {
		if r.kind == rowBlock && r.block == m.edBlock {
			if h.line >= 0 {
				m.ed.cur = caretAt(m.ed.String(), h.line, col)
				m.ed.goalCol = -1
				m.updateCompletion()
			}
			return
		}
		// Leaving a block by clicking elsewhere saves it, exactly as esc does.
		n := len(m.rows)
		m.commitInsert()
		if m.mode != modeNormal || len(m.rows) != n {
			// The save changed the shape of the page, so the frame that was
			// clicked no longer says which row is where.
			return
		}
	}

	switch r.kind {
	case rowLink, rowRef, rowEmbed, rowDay:
		m.cur = h.row
		m.activateRow()
		return
	case rowHeader:
		m.cur = h.row
		return
	}

	m.cur = h.row
	b := r.block
	if col == -2 && len(b.Children) > 0 {
		// The bullet is the fold triangle.
		if m.collapsed[r.path] {
			delete(m.collapsed, r.path)
		} else {
			m.collapsed[r.path] = true
		}
		m.buildRows()
		return
	}
	if h.line == 0 && col >= 0 && viewCheckRe.MatchString(b.Text) {
		// "☐ " is two cells as drawn, "[ ] " is three when selected and raw.
		width := 2
		if selected {
			width = 3
		}
		if col < width {
			m.toggleTask()
			return
		}
	}
	if !selected {
		return
	}

	lines := strings.Split(b.Text, "\n")
	if h.line >= 0 && h.line < len(lines) {
		if target, ok := linkAt(lines[h.line], col); ok {
			m.openLink(markdown.ParseLinkTarget(target))
			return
		}
	}
	m.startInsert(true)
	if m.mode == modeInsert && h.line >= 0 {
		m.ed.cur = caretAt(m.ed.String(), h.line, col)
	}
}

// textColumn is the screen column where a row's text begins: two cells per
// level of depth, then the bullet and a space. It mirrors renderRow.
func textColumn(r row) int {
	return 2*r.depth + 2
}

// caretAt turns a click on a line of text into a rune offset in the whole
// text, clamped to that line. Columns are cells, so wide runes count twice.
func caretAt(text string, line, col int) int {
	lines := strings.Split(text, "\n")
	if line >= len(lines) {
		return len([]rune(text))
	}
	off := 0
	for _, l := range lines[:line] {
		off += len([]rune(l)) + 1
	}
	return off + runeAtCell(lines[line], col)
}

// runeAtCell is the index of the rune covering cell col, or the line's length
// if col is past its end.
func runeAtCell(s string, col int) int {
	if col <= 0 {
		return 0
	}
	w := 0
	for i, r := range []rune(s) {
		w += runewidth.RuneWidth(r)
		if w > col {
			return i
		}
	}
	return len([]rune(s))
}

// linkAt reports the inside of the [[link]] covering cell col of a raw line.
func linkAt(s string, col int) (string, bool) {
	for _, loc := range viewLinkRe.FindAllStringIndex(s, -1) {
		start := runewidth.StringWidth(s[:loc[0]])
		end := start + runewidth.StringWidth(s[loc[0]:loc[1]])
		if col >= start && col < end {
			return s[loc[0]+2 : loc[1]-2], true
		}
	}
	return "", false
}
