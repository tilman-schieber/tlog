package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/tilman-schieber/tlog/internal/app"
	"github.com/tilman-schieber/tlog/internal/markdown"
)

// Noticing an edit made elsewhere. nvim, the outliner and the desktop app all
// hold the same files, and until now an external edit only surfaced as a
// refused write and a manual R.
//
// The change arrives as an ordinary message through a tea.Cmd rather than by
// pushing into the program, so the headless test harness can drive every branch
// below with m.Update(changedMsg{...}) — no filesystem, no goroutine, no clock.

type changedMsg app.Change

// watchCmd waits for the next change. Each one schedules the next wait, which
// is how a channel becomes a stream of messages in this architecture.
func watchCmd(ch <-chan app.Change) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		c, ok := <-ch
		if !ok {
			return nil
		}
		return changedMsg(c)
	}
}

// dirty reports whether there is work in memory that the file does not have.
// It is the one thing that must never be thrown away for a reload — by the
// watcher, and equally by someone pressing R.
//
// Two ways to be dirty: half-typed text that has not been folded back into the
// block yet, and an edit whose save was refused because the file had moved.
func (m *Model) dirty() bool {
	if m.mode == modeInsert && m.edBlock != nil && m.ed.String() != m.edBlock.Text {
		return true
	}
	// The blank bullet shown for a page that has no file yet is not work, and
	// it never matches what is on disk, so it has to be excluded by name.
	if bs := m.doc.Doc.Blocks; len(bs) == 1 && bs[0].Text == "" &&
		len(bs[0].Children) == 0 && len(bs[0].Props) == 0 {
		return false
	}
	return markdown.Hash(markdown.Render(m.doc.Doc)) != m.doc.Hash
}

// changed decides what an external edit means for what is on screen.
func (m *Model) changed(c changedMsg) {
	// Our own save, coming back round. Ignoring it is what makes this loop
	// terminate: a reload here would write nothing, but it would fight the
	// caret on every keystroke.
	if m.svc.Ours(app.Change(c)) {
		return
	}

	if c.Rel != m.doc.Rel {
		// Another file: the links and tags into this page may have moved, so
		// the sections below the outline are stale. Redrawing them mid-word
		// would be its own kind of rude, so it waits.
		if m.mode == modeInsert {
			m.graphDirty = true
			return
		}
		m.refresh()
		m.buildRows()
		return
	}

	if c.Hash == m.doc.Hash {
		return // the file says what we already think it says
	}

	if m.dirty() {
		// Never discard typing. Say so instead, and let the next save refuse
		// on the hash — which is the same protection, now with warning.
		m.stale = true
		m.status = "changed on disk while you were typing — esc saves, R discards"
		return
	}

	// The row the cursor is on is kept rather than the byte offset: an edit
	// made elsewhere in the file moves every offset after it, so an offset
	// would point at a different block, while the row is roughly where you
	// were looking.
	caret := 0
	if m.mode == modeInsert && m.ed != nil {
		caret = m.ed.cur
	}
	if err := m.reloadFromDisk(); err != nil {
		m.errMsg = err.Error()
		return
	}
	if m.mode == modeInsert {
		m.editHere(caret)
	}
	m.status = "reloaded — changed on disk"
}
