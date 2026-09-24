// Package tui is the outliner. It is an adapter: every change it makes goes
// through app.Service, and it holds no logic the CLI could not also reach.
package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tilman-schieber/tlog/internal/app"
	"github.com/tilman-schieber/tlog/internal/markdown"
	"github.com/tilman-schieber/tlog/internal/store"
)

type mode int

const (
	modeNormal mode = iota
	modeInsert
	modePalette
	modeSearch
	modeLinks
	modeBacklinks
	modeHelp
	modeSettings
)

type rowKind int

const (
	rowBlock  rowKind = iota // a block of this page, editable
	rowHeader                // a section heading below the outline
	rowLink                  // a page name to jump to
	rowRef                   // a block on another page that refers to this one
	rowEmbed                 // a block on another page, brought in by a reference
)

type row struct {
	kind  rowKind
	block *markdown.Block
	depth int
	path  string

	text   string // label, for rows that are not blocks of this page
	rel    string // navigation target
	offset int    // block offset within that target
}

// Model is the outliner state.
type Model struct {
	svc *app.Service

	doc  *app.Doc
	page *app.PageView // what the rest of the notes say about this file
	rows []row
	cur  int
	top  int

	width, height int

	mode    mode
	ed      *editor
	edBlock *markdown.Block

	collapsed map[string]bool

	pick *picker
	comp *completion

	settings *settings

	// embeds resolves each block's references by offset, so that a reference
	// can be drawn as the block it points at rather than as a page name. It is
	// rebuilt with the rows, from the same read model the desktop app uses.
	embeds map[int][]app.EmbedView

	// look is a snapshot held only while a picker is open, so that filtering as
	// you type does not rebuild the graph on every keystroke.
	look *app.Lookup

	history []string
	status  string
	errMsg  string
	stale   bool

	// graphDirty says a file other than this one changed while typing, so the
	// sections below the outline are out of date. Redrawing them mid-word
	// would move what is on screen under the caret, so it waits for esc.
	graphDirty bool

	// changes is the stream of external edits, nil when watching is off. A nil
	// channel blocks forever, which is exactly the old behaviour.
	changes <-chan app.Change

	pendingG bool
	pendingD bool
	pendingR bool

	quitting bool
}

// Run starts the outliner on a file, defaulting to today's journal.
func Run(svc *app.Service, rel string) error {
	if rel == "" {
		rel = svc.StartupRel()
	}
	m := &Model{svc: svc, collapsed: map[string]bool{}}
	if err := m.load(rel); err != nil {
		return err
	}

	// Watching is what makes nvim and the outliner usable on the same file at
	// the same time. It stops when the outliner does; a watcher that will not
	// start leaves WatchErr set and a nil channel, which is the old behaviour.
	if svc.Cfg.Watch.Enabled {
		ctx, stop := context.WithCancel(context.Background())
		defer stop()
		m.changes = svc.Watch(ctx)
		if svc.WatchErr != nil {
			m.status = "not watching for outside edits: " + svc.WatchErr.Error()
		}
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	if cerr := svc.Commit(); cerr != nil && err == nil {
		return cerr
	}
	return err
}

func (m *Model) Init() tea.Cmd { return watchCmd(m.changes) }

// load reads a file for editing together with what the rest of the notes say
// about it. Both come from the core in one graph build.
func (m *Model) load(rel string) error {
	d, page, err := m.svc.Open(rel)
	if err != nil {
		return err
	}
	// Collapse is remembered by position, and a position means a different
	// block on a different file — "the second block at the top level" is not
	// the same thing on two pages. Arriving somewhere new starts expanded;
	// re-reading the same file keeps what was folded.
	if m.doc == nil || m.doc.Rel != rel {
		m.collapsed = map[string]bool{}
	} else {
		m.collapsed = carryCollapse(m.collapsed, m.doc.Doc, d.Doc)
	}
	m.doc, m.page = d, page
	m.stale = false
	if len(m.doc.Doc.Blocks) == 0 {
		m.doc.Doc.AppendChild(nil, &markdown.Block{})
	}
	m.cur, m.top = 0, 0
	m.buildRows()
	return nil
}

// refresh re-reads what the notes say about the current file, after something
// changed the links or tags in it.
func (m *Model) refresh() {
	page, err := m.svc.View(m.doc.Rel)
	if err != nil {
		m.errMsg = err.Error()
		return
	}
	m.page = page
}

func (m *Model) buildRows() {
	m.rows = m.rows[:0]

	// What the core resolved about this page, by offset: which references
	// point where, and which blocks are nothing but a reference.
	views := map[int]*app.BlockView{}
	m.embeds = map[int][]app.EmbedView{}
	if m.page != nil {
		for i := range m.page.Blocks {
			bv := &m.page.Blocks[i]
			views[bv.Offset] = bv
			if len(bv.Embeds) > 0 {
				m.embeds[bv.Offset] = bv.Embeds
			}
		}
	}

	var walk func(bs []*markdown.Block, depth int, prefix string)
	walk = func(bs []*markdown.Block, depth int, prefix string) {
		for i, b := range bs {
			path := blockPath(prefix, i)
			m.rows = append(m.rows, row{block: b, depth: depth, path: path})

			// A block that is nothing but a reference brings the subtree with
			// it. Those rows belong to another file, so they are shown and not
			// edited — current() returns nil for anything that is not a block
			// of this page, which is what keeps them read-only.
			if bv := views[b.Start]; bv != nil && bv.IsEmbed && !m.collapsed[path] {
				for _, kid := range bv.EmbedKids {
					m.rows = append(m.rows, row{
						kind:   rowEmbed,
						text:   kid.Text,
						rel:    kid.Rel,
						offset: kid.Addr.Offset,
						depth:  depth + kid.Depth,
					})
				}
			}

			if len(b.Children) > 0 && !m.collapsed[path] {
				walk(b.Children, depth+1, path)
			}
		}
	}
	walk(m.doc.Doc.Blocks, 0, "")
	m.buildSections()
	if m.cur >= len(m.rows) {
		m.cur = max(0, len(m.rows)-1)
	}
}

// buildSections appends what the rest of the notes directory says about this
// page: the pages it groups as a tag, and the blocks elsewhere that mention it.
// Seeing them on the page is the whole point of having a graph — going to a
// person and finding every mention already there beats remembering to search.
func (m *Model) buildSections() {
	if m.page == nil {
		return
	}
	if tagged := m.page.Tagged; len(tagged) > 0 {
		m.rows = append(m.rows, row{kind: rowHeader,
			text: fmt.Sprintf("%d pages tagged #%s", len(tagged), m.page.Title)})
		for _, p := range tagged {
			m.rows = append(m.rows, row{kind: rowLink, text: p, depth: 0})
		}
	}

	refs := m.page.Refs
	if len(refs) == 0 {
		return
	}
	heads := 0
	for _, r := range refs {
		if r.Head {
			heads++
		}
	}
	m.rows = append(m.rows, row{kind: rowHeader, text: fmt.Sprintf("%d linked references", heads)})
	last := ""
	for _, r := range refs {
		if r.Head && r.Page != last {
			last = r.Page
			m.rows = append(m.rows, row{kind: rowLink, text: r.Page, rel: r.Rel, depth: 0})
		}
		m.rows = append(m.rows, row{
			kind: rowRef, text: r.Text, rel: r.Rel, offset: r.Offset, depth: r.Depth + 1,
		})
	}
}

func (m *Model) rowKind() rowKind {
	if m.cur < 0 || m.cur >= len(m.rows) {
		return rowBlock
	}
	return m.rows[m.cur].kind
}

// activateRow follows whatever the cursor is sitting on in the sections below
// the outline. It reports false for ordinary blocks so the caller can fall
// through to editing.
func (m *Model) activateRow() bool {
	if m.cur < 0 || m.cur >= len(m.rows) {
		return false
	}
	r := m.rows[m.cur]
	switch r.kind {
	case rowHeader:
		return true
	case rowLink:
		if r.rel != "" {
			m.goTo(r.rel)
		} else {
			m.openLink(markdown.Link{Page: r.text})
		}
		return true
	case rowRef, rowEmbed:
		m.goTo(r.rel)
		m.focusOffset(r.offset)
		return true
	}
	return false
}

// current is the block under the cursor, or nil when the cursor is in the
// sections below the outline, which are not part of this file and must never be
// edited as though they were.
func (m *Model) current() *markdown.Block {
	if m.cur < 0 || m.cur >= len(m.rows) || m.rows[m.cur].kind != rowBlock {
		return nil
	}
	return m.rows[m.cur].block
}

func (m *Model) currentPath() string {
	if m.cur < 0 || m.cur >= len(m.rows) {
		return ""
	}
	return m.rows[m.cur].path
}

// save writes the document, turning a lost race into a visible message rather
// than a silent overwrite.
func (m *Model) save() {
	if err := m.svc.Save(m.doc); err != nil {
		var conflict *store.ErrConflict
		if asConflict(err, &conflict) {
			m.stale = true
			m.errMsg = "file changed on disk — press R to reload (your edit is not saved)"
			return
		}
		m.errMsg = err.Error()
		return
	}
	m.errMsg = ""
	// A push that failed happened in the background, so it has to be said
	// somewhere or it is the same as not happening at all.
	if sync := m.svc.Sync(); sync.LastErr != "" {
		m.status = "not pushed: " + sync.LastErr
	}
	// Links and tags just changed, and the sections below the outline are
	// derived from them.
	m.refresh()
}

func asConflict(err error, target **store.ErrConflict) bool {
	c, ok := err.(*store.ErrConflict)
	if ok {
		*target = c
	}
	return ok
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.key(msg)
	case changedMsg:
		m.changed(msg)
		// Wait for the next one. Each change schedules the next wait, so the
		// stream stays a stream.
		return m, watchCmd(m.changes)
	}
	return m, nil
}

func (m *Model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()

	// Completion intercepts navigation keys while it is open.
	if m.mode == modeInsert && m.comp != nil && m.comp.active {
		if handled, cmd := m.completionKey(k); handled {
			return m, cmd
		}
	}

	switch m.mode {
	case modeInsert:
		return m.insertKey(msg, k)
	case modePalette, modeSearch, modeLinks, modeBacklinks:
		return m.pickerKey(msg, k)
	case modeSettings:
		return m.settingsKey(msg, k)
	case modeHelp:
		m.mode = modeNormal
		return m, nil
	default:
		return m.normalKey(msg, k)
	}
}

// --- NORMAL -----------------------------------------------------------------

func (m *Model) normalKey(msg tea.KeyMsg, k string) (tea.Model, tea.Cmd) {
	if m.pendingG {
		m.pendingG = false
		switch k {
		case "g":
			m.cur = 0
			return m, nil
		case "f":
			if m.activateRow() {
				return m, nil
			}
			return m, m.followLink()
		case "b":
			m.openBacklinks()
			return m, nil
		}
		return m, nil
	}
	if m.pendingR && k != "R" {
		m.pendingR = false
	}
	if m.pendingD {
		m.pendingD = false
		if k == "d" {
			m.deleteBlock()
			return m, nil
		}
		return m, nil
	}

	switch k {
	case "ctrl+c", "q":
		m.quitting = true
		return m, tea.Quit
	case "?":
		m.mode = modeHelp
		return m, nil
	case ",":
		m.openSettings()
		return m, nil
	case "R":
		// Reloading throws away whatever is not on disk, so it asks first —
		// the same two-key confirmation as dd.
		if m.dirty() && !m.pendingR {
			m.pendingR = true
			m.status = "your edit is not saved — R again to discard it"
			return m, nil
		}
		m.pendingR = false
		if err := m.load(m.doc.Rel); err != nil {
			m.errMsg = err.Error()
		} else {
			m.errMsg = ""
			m.status = "reloaded from disk"
		}
		return m, nil

	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "g":
		m.pendingG = true
	case "G":
		m.cur = max(0, len(m.rows)-1)
	case "ctrl+u":
		m.moveCursor(-m.viewHeight() / 2)
	case "ctrl+d":
		m.moveCursor(m.viewHeight() / 2)

	case "left", "h":
		m.collapseOrParent()
	case "right", "l":
		m.expandOrChild()

	case "enter":
		if m.activateRow() {
			return m, nil
		}
		m.newSibling(false)
	case "o":
		m.newSibling(false)
	case "O":
		m.newSibling(true)
	case "i", "e", "f2":
		m.startInsert(true)
	case "a":
		m.startInsert(true)
	case "I":
		m.startInsert(false)

	case "tab":
		m.structural(m.svc.Indent)
	case "shift+tab":
		m.structural(m.svc.Outdent)
	case "alt+up", "K":
		m.structural(func(a app.Addr) (*app.Result, error) { return m.svc.Move(a, -1) })
	case "alt+down", "J":
		m.structural(func(a app.Addr) (*app.Result, error) { return m.svc.Move(a, 1) })

	case "d":
		m.pendingD = true
	case " ":
		m.toggleTask()

	case "ctrl+p":
		m.openPalette()
	case "/":
		m.openSearch()
	case "t":
		m.goTo(m.svc.TodayRel())
	case "[":
		m.shiftDay(-1)
	case "]":
		m.shiftDay(1)
	case "backspace", "ctrl+o":
		m.back()
	}
	return m, nil
}

func (m *Model) moveCursor(delta int) {
	m.cur = max(0, min(m.cur+delta, len(m.rows)-1))
}

func (m *Model) collapseOrParent() {
	b := m.current()
	if b == nil {
		return
	}
	path := m.currentPath()
	if len(b.Children) > 0 && !m.collapsed[path] {
		m.collapsed[path] = true
		m.buildRows()
		return
	}
	// Already collapsed or a leaf: step out to the parent.
	for i := m.cur - 1; i >= 0; i-- {
		if m.rows[i].depth < m.rows[m.cur].depth {
			m.cur = i
			return
		}
	}
}

func (m *Model) expandOrChild() {
	b := m.current()
	if b == nil || len(b.Children) == 0 {
		return
	}
	path := m.currentPath()
	if m.collapsed[path] {
		delete(m.collapsed, path)
		m.buildRows()
		return
	}
	if m.cur+1 < len(m.rows) {
		m.cur++
	}
}

// --- the core ---------------------------------------------------------------

// addrOf addresses the block under the cursor for the core: where it starts in
// the file, and the hash of the file that offset was computed against. The core
// refuses an address computed against a file that has since moved, which is how
// the outliner cannot clobber an edit made in another editor.
//
// It is only meaningful while memory and disk agree about the *shape* of the
// file. They always do: every structural change goes through the core, which
// writes and hands back a fresh offset. The text being typed is the one thing
// that is allowed to differ, and text does not move the block it is in.
func (m *Model) addrOf(b *markdown.Block) app.Addr {
	return app.Addr{Rel: m.doc.Rel, Offset: b.Start, Hash: m.doc.Hash}
}

// ensureOnDisk makes the address of a block mean something before one is taken.
//
// Browsing to a day with no file must create nothing, so load shows a blank
// bullet that exists only in memory — and the core addresses a block by where
// it is *in the file*. The first real operation on such a page has to put it
// there. Writing is skipped when the file already says exactly this, which is
// the case every other time.
func (m *Model) ensureOnDisk() bool {
	if markdown.Hash(markdown.Render(m.doc.Doc)) == m.doc.Hash {
		return true
	}
	m.save()
	return m.errMsg == ""
}

// addrFrom turns the result of one mutation into the address for the next, so
// two steps can be chained without a round trip through the rows.
func addrFrom(res *app.Result) app.Addr {
	return app.Addr{Rel: res.Rel, Offset: res.Offset, Hash: res.Hash}
}

// structural sends the block under the cursor through a core mutation. Every
// structural change in the outliner goes through here, so the outliner and the
// desktop app cannot come to disagree about what indent means.
func (m *Model) structural(fn func(app.Addr) (*app.Result, error)) {
	b := m.current()
	if b == nil || !m.ensureOnDisk() {
		return
	}
	m.apply(fn(m.addrOf(b)))
}

// apply takes the outcome of a core mutation: it re-reads the file the core has
// just rewritten and puts the cursor back on the block the core says it moved.
func (m *Model) apply(res *app.Result, err error) bool {
	if err != nil {
		m.fail(err)
		return false
	}
	if err := m.reloadFromDisk(); err != nil {
		m.errMsg = err.Error()
		return false
	}
	m.errMsg = ""
	m.focusOffset(res.Offset)
	// A push that failed happened in the background, so it has to be said
	// somewhere or it is the same as not happening at all.
	if sync := m.svc.Sync(); sync.LastErr != "" {
		m.status = "not pushed: " + sync.LastErr
	}
	return true
}

// fail separates the three things that can go wrong, because they want three
// different reactions. A move that does not exist — outdenting a top-level
// block — is not an error: nothing broke and nothing is lost, so it belongs in
// the status line. A file that moved underneath is the one worth shouting
// about. Anything else is a real failure.
func (m *Model) fail(err error) {
	if app.Refused(err) {
		m.status = err.Error()
		return
	}
	var conflict *store.ErrConflict
	if asConflict(err, &conflict) || app.Stale(err) {
		m.stale = true
		m.errMsg = "file changed on disk — press R to reload (your edit is not saved)"
		return
	}
	m.errMsg = err.Error()
}

// reloadFromDisk re-reads the file after the core rewrote it, keeping the view
// where it is. load is for arriving at a page; this is for staying on one.
func (m *Model) reloadFromDisk() error {
	d, page, err := m.svc.Open(m.doc.Rel)
	if err != nil {
		return err
	}
	// The tree has just changed shape, so what was folded has to be carried
	// onto the blocks it belonged to rather than left on their old positions.
	m.collapsed = carryCollapse(m.collapsed, m.doc.Doc, d.Doc)
	m.doc, m.page = d, page
	m.stale = false
	if len(m.doc.Doc.Blocks) == 0 {
		m.doc.Doc.AppendChild(nil, &markdown.Block{})
	}
	m.buildRows()
	return nil
}

// focusOffset puts the cursor on the block starting at an offset. Reloading
// replaces every block pointer, so after a write a block is found by where it
// is rather than by which object it was.
func (m *Model) focusOffset(off int) {
	for i, r := range m.rows {
		if r.kind == rowBlock && r.block.Start == off {
			m.cur = i
			return
		}
	}
	// The block exists but is hidden under a collapsed parent, or the file is
	// now empty. Either way, stay somewhere valid.
	m.cur = max(0, min(m.cur, len(m.rows)-1))
}

// focus puts the cursor on a particular block, for the callers that have the
// object rather than an offset.
func (m *Model) focus(b *markdown.Block) {
	for i, r := range m.rows {
		if r.block == b {
			m.cur = i
			return
		}
	}
}

func (m *Model) deleteBlock() {
	b := m.current()
	if b == nil || !m.ensureOnDisk() {
		return
	}
	// Deleting the last block leaves an empty file, and load puts the blank
	// bullet back on screen — the same state a page is in before anything has
	// been written to it.
	if !m.apply(m.svc.DeleteBlock(m.addrOf(b))) {
		return
	}
	m.status = "block deleted — git has the previous version"
}

func (m *Model) toggleTask() {
	b := m.current()
	if b == nil || !m.ensureOnDisk() {
		return
	}
	m.apply(m.svc.ToggleTask(m.addrOf(b)))
}

// --- editing ----------------------------------------------------------------

func (m *Model) startInsert(atEnd bool) {
	b := m.current()
	if b == nil {
		return
	}
	m.ed = newEditor(b.Text)
	if !atEnd {
		m.ed.cur = 0
	}
	m.edBlock = b
	m.mode = modeInsert
	m.comp = nil
}

// commitInsert folds the editor back into the block and writes the file. The
// write happens on leaving the block, not on quitting, so the window in which a
// crash or an external edit can cost anything stays one block wide.
func (m *Model) commitInsert() {
	if m.edBlock == nil {
		return
	}
	m.edBlock.Text = m.ed.String()
	// Always save, even when the text did not change: a block created and left
	// empty still exists on screen, and what is on screen must be on disk.
	m.save()
	m.mode = modeNormal
	m.ed = nil
	m.edBlock = nil
	m.comp = nil
	if m.graphDirty {
		m.graphDirty = false
		m.refresh()
	}
	m.buildRows()
}

func (m *Model) newSibling(above bool) {
	if len(m.rows) > 0 && m.rowKind() != rowBlock {
		return
	}
	b := m.current()
	// An empty block is already the blank line you were about to make. Opening
	// a fresh journal and pressing enter should start typing, not leave an
	// orphaned bullet above the first note.
	if b != nil && !above && b.Text == "" && len(b.Children) == 0 && len(b.Props) == 0 {
		m.startInsert(true)
		return
	}
	if !m.ensureOnDisk() {
		return
	}
	var res *app.Result
	var err error
	switch {
	case b == nil:
		res, err = m.svc.AppendBlock(m.doc.Rel, m.doc.Hash, "")
	case above:
		res, err = m.svc.InsertBefore(m.addrOf(b), "")
	default:
		// A new block under a block with *visible* children becomes its first
		// child, which is what an outliner does and what Enter should feel
		// like. Whether they are visible is the outliner's business, so it
		// answers that question rather than letting the core guess.
		res, err = m.svc.InsertAfter(m.addrOf(b), "", m.hasVisibleChildren(b))
	}
	if !m.apply(res, err) {
		return
	}
	m.editHere(0)
}

// hasVisibleChildren answers the one question the core is not allowed to ask,
// because collapse is view state and only the view knows it.
func (m *Model) hasVisibleChildren(b *markdown.Block) bool {
	return len(b.Children) > 0 && !m.collapsed[m.currentPath()]
}

// editHere opens the editor on the block under the cursor, with the caret at a
// rune offset. Every write reloads the file, so the block to type into is
// whichever one the core put the cursor on, not the one we were holding.
func (m *Model) editHere(caret int) {
	b := m.current()
	if b == nil {
		m.mode = modeNormal
		m.ed, m.edBlock, m.comp = nil, nil, nil
		return
	}
	m.edBlock = b
	m.ed = newEditor(b.Text)
	m.ed.cur = max(0, min(caret, len(m.ed.runes)))
	m.mode = modeInsert
	m.comp = nil
}

func (m *Model) insertKey(msg tea.KeyMsg, k string) (tea.Model, tea.Cmd) {
	switch k {
	case "esc":
		m.commitInsert()
		return m, nil
	case "ctrl+c":
		m.commitInsert()
		m.quitting = true
		return m, tea.Quit

	case "enter":
		// Enter ends this block and starts the next one: the dominant action
		// costs one keypress, and one write — splitting is a single core
		// operation precisely because this is the key people hold down.
		before, after := m.ed.split()
		asChild := m.hasVisibleChildren(m.edBlock)
		if !m.ensureOnDisk() {
			return m, nil
		}
		if m.apply(m.svc.SplitBlock(m.addrOf(m.edBlock), before, after, asChild)) {
			m.editHere(0)
		}
		return m, nil

	case "alt+enter", "ctrl+j", "shift+enter":
		m.ed.insert("\n")
		m.updateCompletion()
		return m, nil

	case "backspace":
		if !m.ed.backspace() {
			m.mergeIntoPrevious()
			return m, nil
		}
		m.updateCompletion()
		return m, nil
	case "delete":
		m.ed.del()
		return m, nil
	case "ctrl+w", "alt+backspace":
		m.ed.deleteWord()
		m.updateCompletion()
		return m, nil
	case "ctrl+u":
		m.ed.deleteToLineStart()
		return m, nil

	case "left":
		m.ed.left()
		return m, nil
	case "right":
		m.ed.right()
		return m, nil
	case "home", "ctrl+a":
		m.ed.home()
		return m, nil
	case "end", "ctrl+e":
		m.ed.end()
		return m, nil
	case "up":
		if !m.ed.up() {
			m.commitInsert()
			m.moveCursor(-1)
			m.startInsert(true)
		}
		return m, nil
	case "down":
		if !m.ed.down() {
			m.commitInsert()
			if m.cur+1 < len(m.rows) {
				m.moveCursor(1)
				m.startInsert(true)
			}
		}
		return m, nil

	case "tab":
		m.structuralWhileTyping(m.svc.Indent)
		return m, nil
	case "shift+tab":
		m.structuralWhileTyping(m.svc.Outdent)
		return m, nil
	}

	if msg.Type == tea.KeyRunes {
		m.ed.insert(string(msg.Runes))
		m.updateCompletion()
		return m, nil
	}
	if k == " " {
		m.ed.insert(" ")
		m.updateCompletion()
	}
	return m, nil
}

// structuralWhileTyping applies a core mutation without leaving INSERT, so Tab
// still indents mid-sentence. The text has to reach the file first: the core
// addresses a block by where it is, and half-typed text lives only here.
func (m *Model) structuralWhileTyping(fn func(app.Addr) (*app.Result, error)) {
	caret := m.ed.cur
	a, ok := m.commitText()
	if !ok {
		return
	}
	if m.apply(fn(a)) {
		m.editHere(caret)
	}
}

// commitText writes what is being typed and returns the block's address in the
// file as it now stands, for a second operation to be addressed against.
func (m *Model) commitText() (app.Addr, bool) {
	if m.edBlock == nil {
		return app.Addr{}, false
	}
	m.edBlock.Text = m.ed.String()
	if !m.ensureOnDisk() {
		return app.Addr{}, false
	}
	return m.addrOf(m.edBlock), true
}

// mergeIntoPrevious is what backspace at the very start of a block does: join
// it onto the block above, the way every outliner behaves.
func (m *Model) mergeIntoPrevious() {
	if m.cur == 0 || m.edBlock == nil {
		return
	}
	// Where the caret should land afterwards: the seam between the two texts.
	at := 0
	if prev := m.rows[m.cur-1]; prev.kind == rowBlock {
		at = len([]rune(prev.block.Text))
	}
	a, ok := m.commitText()
	if !ok {
		return
	}
	if m.apply(m.svc.MergeIntoPrevious(a)) {
		m.editHere(at)
	}
}

// --- navigation -------------------------------------------------------------

func (m *Model) goTo(rel string) {
	if m.doc != nil && rel == m.doc.Rel {
		return
	}
	if m.doc != nil {
		m.history = append(m.history, m.doc.Rel)
	}
	if err := m.load(rel); err != nil {
		m.errMsg = err.Error()
	}
}

func (m *Model) back() {
	if len(m.history) == 0 {
		return
	}
	rel := m.history[len(m.history)-1]
	m.history = m.history[:len(m.history)-1]
	if err := m.load(rel); err != nil {
		m.errMsg = err.Error()
	}
}

func (m *Model) shiftDay(delta int) {
	day, ok := store.JournalDay(m.doc.Rel)
	if !ok {
		day = time.Now()
	}
	m.goTo(m.svc.JournalRel(day.AddDate(0, 0, delta)))
}

// followLink opens the page a link points at, creating it if this is the first
// time anyone has gone there.
func (m *Model) followLink() tea.Cmd {
	b := m.current()
	if b == nil {
		return nil
	}
	links := b.Links()
	switch len(links) {
	case 0:
		m.status = "no [[link]] in this block"
		return nil
	case 1:
		m.openLink(links[0])
		return nil
	default:
		items := make([]pickItem, 0, len(links))
		for _, l := range links {
			items = append(items, pickItem{label: l.Page, detail: l.Anchor, link: l})
		}
		m.pick = newPicker("follow link", items)
		m.mode = modeLinks
		return nil
	}
}

func (m *Model) openLink(l markdown.Link) {
	rel, err := m.svc.OpenPage(l.Page)
	if err != nil {
		m.errMsg = err.Error()
		return
	}
	m.goTo(rel)
	if l.Anchor != "" {
		if b := m.doc.Doc.FindByAnchor(l.Anchor); b != nil {
			m.focus(b)
		}
	}
}

func (m *Model) openBacklinks() {
	name := m.page.Title
	var items []pickItem
	for _, r := range m.page.Refs {
		if !r.Head {
			continue // the list is of mentions, not of every line under one
		}
		items = append(items, pickItem{
			label: r.Page, detail: r.Text, rel: r.Rel, offset: r.Offset,
		})
	}
	if len(items) == 0 {
		m.status = "nothing links to " + name
		return
	}
	m.pick = newPicker("backlinks to "+name, items)
	m.mode = modeBacklinks
}

// snapshot takes the lookup a picker filters against, so that typing does not
// rebuild the graph on every character. It is dropped when the picker closes.
func (m *Model) snapshot() *app.Lookup {
	if m.look != nil {
		return m.look
	}
	l, err := m.svc.Lookup()
	if err != nil {
		m.errMsg = err.Error()
		return nil
	}
	m.look = l
	return l
}

func (m *Model) openPalette() {
	l := m.snapshot()
	if l == nil {
		return
	}
	idx := l.Index()
	var items []pickItem
	for _, name := range idx.Journals {
		items = append(items, pickItem{label: name, detail: "journal",
			rel: filepath.Join(store.JournalsDir, name+".md")})
	}
	for _, name := range idx.Pages {
		items = append(items, pickItem{label: name, detail: "page",
			rel: filepath.Join(store.PagesDir, name+".md")})
	}
	m.pick = newPicker("open", items)
	m.mode = modePalette
}

func (m *Model) openSearch() {
	if m.snapshot() == nil {
		return
	}
	m.pick = newPicker("search", nil)
	m.pick.live = true
	m.mode = modeSearch
}

func (m *Model) refreshSearch() {
	l := m.snapshot()
	if l == nil {
		return
	}
	hits := l.Search(m.pick.query.String(), 200)
	items := make([]pickItem, 0, len(hits))
	for _, h := range hits {
		items = append(items, pickItem{
			label: h.Text, detail: h.Page, rel: h.Rel, offset: h.Offset,
		})
	}
	m.pick.items = items
	m.pick.filtered = items
	m.pick.sel = 0
}

func (m *Model) pickerKey(msg tea.KeyMsg, k string) (tea.Model, tea.Cmd) {
	switch k {
	case "esc", "ctrl+c":
		m.mode = modeNormal
		m.pick, m.look = nil, nil
		return m, nil
	case "up", "ctrl+p":
		m.pick.move(-1)
		return m, nil
	case "down", "ctrl+n":
		m.pick.move(1)
		return m, nil
	case "enter":
		item, ok := m.pick.selected()
		m.mode = modeNormal
		m.pick, m.look = nil, nil
		if !ok {
			return m, nil
		}
		if item.link.Page != "" {
			m.openLink(item.link)
			return m, nil
		}
		m.goTo(item.rel)
		if item.offset > 0 {
			m.focusOffset(item.offset)
		}
		return m, nil
	case "backspace":
		m.pick.query.backspace()
	default:
		if msg.Type == tea.KeyRunes {
			m.pick.query.insert(string(msg.Runes))
		} else if k == " " {
			m.pick.query.insert(" ")
		} else {
			return m, nil
		}
	}
	if m.mode == modeSearch {
		m.refreshSearch()
	} else {
		m.pick.filter()
	}
	return m, nil
}

func (m *Model) viewHeight() int {
	h := m.height - 3
	if h < 3 {
		return 3
	}
	return h
}
