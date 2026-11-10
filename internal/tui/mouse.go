package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tilman-schieber/tlog/internal/app"
	"github.com/tilman-schieber/tlog/internal/markdown"
	"github.com/tilman-schieber/tlog/internal/termimg"
)

// The mouse, and things dropped on the window.
//
// A terminal reports the mouse as a cell, so everything here works from what
// the last frame drew: bodyLines records, for every line on screen, which row
// it belongs to and the columns its links landed in. Nothing is searched for
// in the rendered text afterwards.

func (m *Model) mouse(msg tea.MouseMsg) {
	switch m.mode {
	case modeOutline, modeMove:
	case modePalette, modeSearch, modeLinks, modeBacklinks:
		m.pickerMouse(msg)
		return
	default:
		// The agenda, the log, settings: lists that move with the arrows.
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.key(tea.KeyMsg{Type: tea.KeyUp})
		case tea.MouseButtonWheelDown:
			m.key(tea.KeyMsg{Type: tea.KeyDown})
		}
		return
	}

	line := m.bodyTop + msg.Y - 1 // the header is the first screen line
	if msg.Y < 1 || msg.Y > m.viewHeight() {
		line = -1
	}
	switch {
	case msg.Button == tea.MouseButtonWheelUp:
		m.moveCursor(-3)
	case msg.Button == tea.MouseButtonWheelDown:
		m.moveCursor(3)
	case msg.Action == tea.MouseActionMotion:
		m.hover = m.hitAt(line, msg.X)
	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
		m.click(line, msg.X)
	}
}

// hitAt is the link drawn at a cell, or nil.
func (m *Model) hitAt(line, x int) *linkHit {
	if line < 0 || line >= len(m.lineHits) {
		return nil
	}
	for i := range m.lineHits[line] {
		h := m.lineHits[line][i]
		if x >= h.from && x < h.to {
			return &h
		}
	}
	return nil
}

// click does what pointing at something means: a link opens, a bullet folds,
// a row below the outline is followed, and anywhere in a block puts the caret
// there — or, while moving, highlights it, and a second click starts writing.
func (m *Model) click(line, x int) {
	if line < 0 || line >= len(m.lineRow) || m.lineRow[line] < 0 {
		return
	}
	if h := m.hitAt(line, x); h != nil {
		m.openHit(*h)
		return
	}
	ri := m.lineRow[line]
	r := m.rows[ri]
	if r.kind != rowBlock {
		m.leave()
		m.cur = ri
		m.activateRow()
		return
	}

	bulletCol := r.depth * 2
	if line == m.lineFirst[line] && x >= bulletCol && x <= bulletCol+1 && len(r.block.Children) > 0 {
		m.moveCursor(ri - m.cur)
		if m.collapsed[r.path] {
			m.expand()
		} else {
			m.collapse()
		}
		return
	}

	// The box of a task sits after the bullet: two cells as drawn, three as
	// the raw "[ ]" of the block being written.
	box := 2
	if m.ed != nil && m.edBlock == r.block {
		box = 3
	}
	if line == m.lineFirst[line] && r.block.Task() != markdown.NotATask &&
		x >= bulletCol+2 && x < bulletCol+2+box {
		m.moveCursor(ri - m.cur)
		if m.ed != nil {
			m.toggleTaskWhileTyping()
		} else {
			m.toggleTask()
		}
		return
	}

	if m.mode == modeMove && ri != m.cur {
		m.moveCursor(ri - m.cur)
		return
	}
	m.mode = modeOutline
	if ri != m.cur {
		m.moveCursor(ri - m.cur)
	}
	m.syncEditor()
	if m.ed != nil {
		// Columns count from where the text starts, after the bullet. A long
		// line is wrapped over several screen lines, so the screen line is
		// mapped back to the line of text and the column it starts at. On a
		// block that was drawn with its links collapsed this is close rather
		// than exact, which is as good as a click on text that is about to
		// change shape can be.
		logical, col0 := line-m.lineFirst[line], 0
		if line < len(m.lineInfo) && m.lineInfo[line].logical >= 0 {
			logical, col0 = m.lineInfo[line].logical, m.lineInfo[line].col0
		}
		m.ed.setCaret(logical, col0+x-bulletCol-2)
	}
}

// openHit follows a link the way ctrl+g would.
func (m *Model) openHit(h linkHit) {
	if h.url != "" {
		m.openURL(h.url)
		return
	}
	m.leave()
	m.openLink(h.page)
}

// openURL follows an address or a file. A plain-text file is shown here;
// anything else goes to the desktop: xdg-open, or open on a Mac. The core
// decides what may be opened and what a relative path means.
func (m *Model) openURL(target string) {
	f, err := m.svc.LinkedText(m.doc.Rel, target)
	if err != nil {
		m.errMsg = err.Error()
		return
	}
	if f != nil {
		m.leave()
		m.showFile(f)
		return
	}
	if m.images != termimg.None {
		if p, err := m.svc.LinkedPicture(m.doc.Rel, target); err == nil && p != nil {
			m.leave()
			m.showPicture(p, target)
			return
		}
	}
	if err := m.svc.OpenLink(m.doc.Rel, target); err != nil {
		m.errMsg = err.Error()
		return
	}
	m.errMsg = ""
	m.status = "opened " + shortAddress(target)
}

// pickerMouse lets the palette and the other lists be scrolled and clicked.
func (m *Model) pickerMouse(msg tea.MouseMsg) {
	switch {
	case msg.Button == tea.MouseButtonWheelUp:
		m.pick.move(-1)
	case msg.Button == tea.MouseButtonWheelDown:
		m.pick.move(1)
	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
		i := m.pickTop + msg.Y - 2 // a title line and a rule above the list
		if msg.Y < 2 || i >= len(m.pick.filtered) {
			return
		}
		m.pick.sel = i
		m.key(tea.KeyMsg{Type: tea.KeyEnter})
	}
}

// --- dropping files -----------------------------------------------------------

// dropFiles is what dragging files onto the terminal means. The terminal
// types their paths; the files are copied into the notes' assets/, and their links
// go where you are — at the caret while writing, as a block of their own after
// the highlighted one while moving.
func (m *Model) dropFiles(paths []string) {
	added, err := m.svc.Attach(paths...)
	if err != nil {
		m.fail(err)
		if len(added) == 0 {
			return
		}
	}
	links := make([]string, 0, len(added))
	for _, a := range added {
		links = append(links, a.Link)
	}
	m.place(strings.Join(links, " "), true)
	if m.errMsg == "" {
		m.status = fmt.Sprintf("attached %s — copied into %s", plural(len(added), "file"), m.svc.AttachDir())
	}
}

// place puts text where you are: at the caret while writing — after a space,
// for a link, when it would otherwise run into the word before — and as a
// block of its own after the highlighted one while moving.
func (m *Model) place(text string, spaced bool) {
	if m.mode == modeOutline && m.ed != nil {
		if before := m.ed.textBefore(); spaced && before != "" && !strings.HasSuffix(before, " ") && !strings.HasSuffix(before, "\n") {
			text = " " + text
		}
		m.ed.insert(text)
	} else if b := m.current(); b != nil && m.ensureOnDisk() {
		m.apply(m.svc.InsertAfter(m.addrOf(b), text, false))
	} else if b == nil {
		m.apply(m.svc.AppendBlock(m.doc.Rel, m.doc.Hash, text))
	}
}

// pasteClipboard is ctrl+v. The terminal's own paste (ctrl+shift+v, cmd+v)
// only ever delivers text, so a screenshot on the clipboard never arrives
// through it; plain ctrl+v reaches tlog as a key, and tlog reads the clipboard
// itself. A picture is saved into assets/ and linked where you are; files
// copied in a file manager attach like a drop; text is pasted as text.
func (m *Model) pasteClipboard() {
	c, err := m.svc.ReadClipboard()
	if err != nil {
		m.errMsg = err.Error()
		return
	}
	if c.Image != nil {
		a, err := m.svc.AttachImage(c.Image, time.Now())
		if err != nil {
			m.errMsg = err.Error()
			return
		}
		m.place(a.Link, true)
		if m.errMsg == "" {
			m.status = "pasted " + a.Name + " into " + m.svc.AttachDir()
		}
		return
	}
	if files := app.DroppedFiles(c.Text); files != nil {
		m.dropFiles(files)
		return
	}
	m.place(strings.TrimRight(c.Text, "\n"), false)
}

// paste decides whether a paste is a drop. It reports whether it dealt with
// the paste, which it does for files, and for anything pasted while moving,
// where there is no caret to put it at.
func (m *Model) paste(msg tea.KeyMsg) bool {
	// Not every terminal brackets a drop. Typing delivers one character at a
	// time, so several runes in one message are a paste whatever it is called
	// — and in moving mode a path's letters would otherwise be read as keys.
	burst := msg.Type == tea.KeyRunes && len(msg.Runes) > 1
	if !msg.Paste && !burst {
		return false
	}
	if files := app.DroppedFiles(string(msg.Runes)); files != nil {
		m.dropFiles(files)
		return true
	}
	if !msg.Paste {
		return false // quick typing, not a paste: each rune is a key
	}
	if m.mode == modeMove {
		m.status = "enter to write, then paste"
		return true
	}
	return false
}

// --- following ---------------------------------------------------------------

// blockLinks is everything in a block that can be followed: [[pages]] and
// ordinary links, in that order.
func blockLinks(b *markdown.Block) []pickItem {
	var items []pickItem
	for _, l := range b.Links() {
		items = append(items, pickItem{label: l.Page, detail: l.Anchor, link: l})
	}
	for _, u := range b.URLLinks() {
		items = append(items, pickItem{label: linkLabel(u), detail: u.URL, url: u.URL})
	}
	return items
}
