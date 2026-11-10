// Package tui is the outliner. It is an adapter: every change it makes goes
// through app.Service, and it holds no logic the CLI could not also reach.
package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tilman-schieber/tlog/internal/app"
	"github.com/tilman-schieber/tlog/internal/markdown"
	"github.com/tilman-schieber/tlog/internal/store"
	"github.com/tilman-schieber/tlog/internal/termimg"
)

type mode int

const (
	// modeOutline is the page itself, being written: the block under the
	// cursor is the one being typed into. It is where the outliner opens.
	modeOutline mode = iota
	// modeMove is the same page after esc: no caret, the block under the
	// cursor highlighted, and single letters that go places. Everything below
	// is something laid over the page for a moment.
	modeMove
	modePalette
	modeSearch
	modeLinks
	modeBacklinks
	modeHelp
	modeSettings
	modeAgenda
	modeLog
	modeFile
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
	agenda   *agenda
	log      *logView
	file     *fileView

	// images is how this terminal draws a picture, if it can: read from the
	// environment when the outliner starts, None in tests and anywhere it
	// cannot tell.
	images termimg.Protocol
	// queued is a command something deep in a key or a click wants run —
	// showing a picture hands the terminal over — returned with the update.
	queued tea.Cmd

	// embeds resolves each block's references by offset, so that a reference
	// can be drawn as the block it points at rather than as a page name. It is
	// rebuilt with the rows, from the same read model `tlog view` uses.
	embeds map[int][]app.EmbedView

	// look is a snapshot held only while a picker is open, so that filtering as
	// you type does not rebuild the graph on every keystroke.
	look *app.Lookup

	history []string
	// recent is every page visited this session, most recent first, so the
	// palette can open on the places you actually go.
	recent []string
	status string
	errMsg string
	stale  bool

	// notices is everything said so far, so that a message replaced by the
	// next one is not simply gone.
	notices []notice

	// graphDirty says a file other than this one changed while typing, so the
	// sections below the outline are out of date. Redrawing them mid-word
	// would move what is on screen under the caret, so it waits for esc.
	graphDirty bool

	// changes is the stream of external edits, nil when watching is off. A nil
	// channel blocks forever, which is exactly the old behaviour.
	changes <-chan app.Change

	pendingR bool
	pendingD bool

	// helpTop is how far the help screen is scrolled.
	helpTop int

	// What the last frame drew, line by line, for the mouse to find things
	// in: which row each body line belongs to, the line that row starts on,
	// and where its links are. bodyTop is the first body line on screen.
	lineRow   []int
	lineFirst []int
	lineHits  [][]linkHit
	// lineInfo maps a screen line back to the text it shows, since a long
	// line is wrapped over several; layout is how the block being edited was
	// wrapped, for moving by screen line, and edCursorVis the screen line of
	// that block the caret is on.
	lineInfo    []lineInfo
	layout      edLayout
	edCursorVis int
	bodyTop     int
	pickTop     int
	// hover is the link under the mouse pointer, whose address the footer
	// shows: links are drawn as their label, and this is how to see where
	// one goes before clicking.
	hover *linkHit

	quitting bool
}

// Run starts the outliner on a file, defaulting to today's journal.
func Run(svc *app.Service, rel string) error {
	if rel == "" {
		rel = svc.StartupRel()
	}
	m := &Model{svc: svc, collapsed: map[string]bool{}, images: termimg.Detect(os.Getenv)}
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

	// All motion, not only clicks: links are drawn as their label, and
	// hovering is how to see the address behind one.
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseAllMotion())
	_, err := p.Run()
	if cerr := svc.Commit(); cerr != nil && err == nil {
		return cerr
	}
	return err
}

func (m *Model) Init() tea.Cmd {
	if m.svc.Sync().AutoPush {
		return tea.Batch(watchCmd(m.changes), pullCmd(m.svc))
	}
	return watchCmd(m.changes)
}

// pulledMsg is the end of the pull made on opening, when these notes are
// pushed somewhere: another machine may have written since.
type pulledMsg struct{ err error }

// pullCmd pulls in the background, so a slow network never delays the first
// keystroke. What arrives reaches the screen the way any edit made elsewhere
// does, through the watch.
func pullCmd(svc *app.Service) tea.Cmd {
	return func() tea.Msg { return pulledMsg{svc.Pull()} }
}

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
	m.visited(rel)
	m.syncEditor()
	return nil
}

func (m *Model) visited(rel string) {
	out := []string{rel}
	for _, r := range m.recent {
		if r != rel && len(out) < 30 {
			out = append(out, r)
		}
	}
	m.recent = out
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
			m.errMsg = "file changed on disk — ctrl+r reloads it (your edit is not saved)"
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
	// derived from them. So is what completion offers: a person mentioned for
	// the first time a moment ago is someone to complete now.
	m.look = nil
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
	// Everything the outliner says lands in one of two lines and is replaced by
	// the next thing that happens. Recording the change here rather than at
	// thirty assignment sites means nothing can be said without being kept.
	before, beforeErr, beforeMode := m.status, m.errMsg, m.mode
	// Each key and each click is one thing a person did: whatever it writes
	// is undone together, and apart from whatever the next one writes.
	switch msg.(type) {
	case tea.KeyMsg, tea.MouseMsg:
		m.svc.Checkpoint()
	}
	// Synced on the way in too, so a key always lands in the block under the
	// cursor even if the cursor was put there by something other than a key.
	m.syncEditor()
	model, cmd := m.update(msg)
	// Anything laid over the page may change files — the agenda ticks tasks
	// off, a command deletes blocks — so the block being typed is written
	// before it opens, and the editor is put back on the cursor when it closes.
	if beforeMode == modeOutline && m.mode != modeOutline {
		m.leave()
	}
	m.syncEditor()
	if m.status != before {
		m.record(m.status, false)
	}
	if m.errMsg != beforeErr {
		m.record(m.errMsg, true)
	}
	if m.queued != nil {
		cmd = tea.Batch(cmd, m.queued)
		m.queued = nil
	}
	return model, cmd
}

func (m *Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		m.hover = nil // the screen is about to change under the pointer
		return m.key(msg)
	case tea.MouseMsg:
		m.mouse(msg)
		return m, nil
	case pictureClosedMsg:
		m.pictureClosed(msg)
		return m, nil
	case pulledMsg:
		if msg.err != nil {
			m.status = "not pulled: " + msg.err.Error()
		}
		return m, nil
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
	if m.mode == modeOutline && m.ed != nil && m.comp != nil && m.comp.active {
		if handled, cmd := m.completionKey(k); handled {
			return m, cmd
		}
	}

	switch m.mode {
	case modePalette, modeSearch, modeLinks, modeBacklinks:
		return m.pickerKey(msg, k)
	case modeSettings:
		return m.settingsKey(msg, k)
	case modeAgenda:
		return m.agendaKey(k)
	case modeLog:
		return m.logKey(k)
	case modeFile:
		return m.fileKey(k)
	case modeHelp:
		return m.helpKey(k)
	default:
		return m.outlineKey(msg, k)
	}
}

// --- moving -----------------------------------------------------------------
//
// Writing is what the outliner opens on, because taking notes is mostly
// typing. Getting around is the other half, and chords for it were clumsy; so
// esc puts the caret down and the letters become commands, the way Logseq's
// block selection works. Enter picks the caret back up.

func (m *Model) moveKey(msg tea.KeyMsg, k string) (tea.Model, tea.Cmd) {
	if m.pendingD {
		m.pendingD = false
		if k == "d" {
			m.deleteBlock()
		}
		return m, nil
	}
	switch k {
	case "enter":
		if !m.activateRow() && m.current() != nil {
			m.mode = modeOutline
		}
	case "i", "e":
		if m.current() != nil {
			m.mode = modeOutline
		}
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "left", "h":
		m.collapse()
	case "right", "l":
		m.expand()
	case "z":
		if b := m.current(); b != nil && m.collapsed[m.currentPath()] {
			m.expand()
		} else {
			m.collapse()
		}
	case "p":
		m.openPalette()
	case "/":
		m.openSearch()
	case "t":
		m.goTo(m.svc.TodayRel())
	case "b", "backspace":
		m.back()
	case "g", "o":
		if !m.activateRow() {
			m.followLink()
		}
	case "[":
		m.shiftDay(-1)
	case "]":
		m.shiftDay(1)
	case "a":
		m.openAgenda()
	case "x", " ":
		m.toggleTask()
	case "d":
		if m.current() != nil {
			m.pendingD = true
			m.status = "d again deletes the block and everything in it"
		}
	case "tab":
		m.structural(m.svc.Indent)
	case "shift+tab":
		m.structural(m.svc.Outdent)
	case "alt+up":
		m.structural(func(a app.Addr) (*app.Result, error) { return m.svc.Move(a, -1) })
	case "alt+down":
		m.structural(func(a app.Addr) (*app.Result, error) { return m.svc.Move(a, 1) })
	case "u":
		m.undoFile(false)
	case "U":
		m.undoFile(true)
	case "?":
		m.mode = modeHelp
	case "q":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		m.status = ""
	default:
		if msg.Type == tea.KeyRunes {
			m.status = "moving — enter writes here · ? lists the keys"
		}
	}
	return m, nil
}

// --- the outline ------------------------------------------------------------
//
// There is one mode. The block under the cursor is always the one being typed
// into, so writing never starts with a keystroke that says you are about to
// write. That rules out bare letters as commands, so everything that is not
// text takes a modifier — and the few things done often enough to deserve a
// key of their own have one, while the rest are found by name with ctrl+p.
//
// The keys a terminal cannot tell apart shaped the table: ctrl+i is tab, ctrl+m
// is enter, ctrl+h is backspace, ctrl+[ is esc, and alt+[ is the start of
// every arrow-key sequence. ctrl+enter and shift+enter arrive as plain enter in
// most terminals, which is why a task is toggled with ctrl+d instead.

func (m *Model) outlineKey(msg tea.KeyMsg, k string) (tea.Model, tea.Cmd) {
	if k != "ctrl+r" {
		m.pendingR = false
	}
	// A drop arrives as a paste of file paths, in either mode.
	if m.paste(msg) {
		return m, nil
	}

	// Going somewhere, and everything else that is not typing.
	switch k {
	case "ctrl+c", "ctrl+q":
		m.leave()
		m.quitting = true
		return m, tea.Quit
	case "f1":
		m.mode = modeHelp
		return m, nil
	case "ctrl+p":
		m.openPalette()
		return m, nil
	case "ctrl+f":
		m.openSearch()
		return m, nil
	case "ctrl+t":
		m.goTo(m.svc.TodayRel())
		return m, nil
	case "ctrl+o":
		m.back()
		return m, nil
	case "alt+left":
		m.shiftDay(-1)
		return m, nil
	case "alt+right":
		m.shiftDay(1)
		return m, nil
	case "ctrl+r":
		m.reload()
		return m, nil
	case "ctrl+z":
		// Typing not yet written is undone in the block, a word at a time;
		// once there is none, it is the last thing written that goes.
		if m.mode == modeOutline && m.ed != nil && m.ed.undo() {
			m.updateCompletion()
			return m, nil
		}
		m.undoFile(false)
		return m, nil
	case "ctrl+y":
		if m.mode == modeOutline && m.ed != nil && m.ed.redo() {
			m.updateCompletion()
			return m, nil
		}
		m.undoFile(true)
		return m, nil
	case "ctrl+v":
		m.pasteClipboard()
		return m, nil
	case "ctrl+g":
		if m.activateRow() {
			return m, nil
		}
		m.followLinkHere()
		return m, nil
	case "ctrl+up":
		m.collapse()
		return m, nil
	case "ctrl+down":
		m.expand()
		return m, nil
	case "pgup":
		m.moveCursor(-m.viewHeight() / 2)
		return m, nil
	case "pgdown":
		m.moveCursor(m.viewHeight() / 2)
		return m, nil
	case "ctrl+home":
		m.moveCursor(-len(m.rows))
		return m, nil
	case "ctrl+end":
		m.moveCursor(len(m.rows))
		return m, nil
	}

	if m.mode == modeMove {
		// Letters typed quickly can arrive in one read, as one key holding
		// several runes; "dd" would then be a key called "dd" that matches
		// nothing. Each is a command of its own, so they are taken apart.
		if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 && !msg.Paste {
			var cmd tea.Cmd
			for _, r := range msg.Runes {
				one := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
				_, c := m.key(one)
				cmd = tea.Batch(cmd, c)
			}
			return m, cmd
		}
		return m.moveKey(msg, k)
	}

	// Below the outline the rows belong to other pages: they can be walked
	// through and opened, never typed into.
	if m.ed == nil {
		switch k {
		case "up":
			m.moveCursor(-1)
		case "down":
			m.moveCursor(1)
		case "enter":
			m.activateRow()
		case "esc":
			m.mode = modeMove
		default:
			if msg.Type == tea.KeyRunes {
				m.status = "this lives on another page — enter opens it"
			}
		}
		return m, nil
	}
	return m.typingKey(msg, k)
}

// typingKey is everything that happens inside the block under the cursor.
func (m *Model) typingKey(msg tea.KeyMsg, k string) (tea.Model, tea.Cmd) {
	switch k {
	case "esc":
		// Put the caret down and go somewhere. Leaving writing writes the
		// block, so nothing typed is ever behind a mode.
		m.mode = modeMove
		return m, nil

	case "enter":
		// Enter ends this block and starts the next one: the dominant action
		// costs one keypress, and one write — splitting is a single core
		// operation precisely because this is the key people hold down.
		before, after := m.ed.split()
		asChild := m.hasVisibleChildren(m.edBlock)
		a, ok := m.commitText()
		if !ok {
			return m, nil
		}
		if m.apply(m.svc.SplitBlock(a, before, after, asChild)) {
			m.attach(0)
		}
		return m, nil

	case "alt+enter", "ctrl+j", "shift+enter":
		m.ed.insert("\n")
		m.updateCompletion()
		return m, nil

	case "tab":
		m.structuralWhileTyping(m.svc.Indent)
		return m, nil
	case "shift+tab":
		m.structuralWhileTyping(m.svc.Outdent)
		return m, nil
	case "alt+up":
		m.structuralWhileTyping(func(a app.Addr) (*app.Result, error) { return m.svc.Move(a, -1) })
		return m, nil
	case "alt+down":
		m.structuralWhileTyping(func(a app.Addr) (*app.Result, error) { return m.svc.Move(a, 1) })
		return m, nil
	case "ctrl+d":
		m.toggleTaskWhileTyping()
		return m, nil

	case "backspace":
		if !m.ed.backspace() {
			m.backspaceAtStart()
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

	// The arrows run off the end of a block into the next one, so the whole
	// page reads as one piece of text.
	case "left":
		if m.ed.atStart() && m.cur > 0 {
			m.moveCursor(-1)
			return m, nil // lands at the end of the block above
		}
		m.ed.left()
		return m, nil
	case "right":
		if m.ed.atEnd() && m.cur+1 < len(m.rows) {
			m.moveCursor(1)
			if m.ed != nil {
				m.ed.cur = 0
			}
			return m, nil
		}
		m.ed.right()
		return m, nil
	case "home", "ctrl+a":
		m.ed.home()
		return m, nil
	case "end", "ctrl+e":
		m.ed.end()
		return m, nil
	case "up":
		// A long line is wrapped on screen, so ↑ goes to the screen line
		// above, which may still be the same line of text.
		if !m.moveVisual(-1) && !m.ed.up() {
			m.moveCursor(-1)
		}
		return m, nil
	case "down":
		if !m.moveVisual(1) && !m.ed.down() {
			m.moveCursor(1)
		}
		return m, nil
	}

	if msg.Type == tea.KeyRunes {
		if msg.Paste {
			m.ed.insert(string(msg.Runes))
		} else {
			// Typed quickly, several characters can arrive as one key; each
			// is typed on its own so "[[" still closes itself.
			for _, r := range msg.Runes {
				m.ed.typeRune(string(r))
			}
		}
		m.updateCompletion()
		return m, nil
	}
	if k == " " {
		m.ed.insert(" ")
		m.updateCompletion()
	}
	return m, nil
}

// moveCursor goes to another row, writing the block being left first if it
// was changed. The editor follows the cursor: see syncEditor.
func (m *Model) moveCursor(delta int) {
	m.leave()
	m.cur = max(0, min(m.cur+delta, len(m.rows)-1))
	m.syncEditor()
}

// undoFile takes back the last thing written, or puts back the last thing
// taken back. If it was on another page, that page is where you end up, so the
// change is seen happening rather than made out of sight.
func (m *Model) undoFile(redo bool) {
	m.leave()
	word, did := "undo", "undone"
	run := m.svc.Undo
	if redo {
		word, did, run = "redo", "redone", m.svc.Redo
	}
	done, err := run()
	if err != nil {
		var conflict *store.ErrConflict
		if asConflict(err, &conflict) {
			m.errMsg = fmt.Sprintf("cannot %s: %s was changed elsewhere since", word, store.Title(conflict.Rel))
			return
		}
		m.fail(err)
		return
	}
	m.look = nil
	m.errMsg = ""
	if done.Rel != m.doc.Rel {
		m.goTo(done.Rel)
	} else {
		row := m.cur
		if err := m.reloadFromDisk(); err != nil {
			m.errMsg = err.Error()
			return
		}
		m.cur = max(0, min(row, len(m.rows)-1))
	}
	m.syncEditor()
	where := store.Title(done.Rel)
	if len(done.Rels) > 1 {
		where += fmt.Sprintf(" and %d more", len(done.Rels)-1)
	}
	m.status = did + " — " + where
}

// reload throws away what is not on disk, so it asks first: a second ctrl+r.
func (m *Model) reload() {
	if m.dirty() && !m.pendingR {
		m.pendingR = true
		m.status = "your edit is not saved — ctrl+r again to discard it"
		return
	}
	m.pendingR = false
	m.ed, m.edBlock, m.comp = nil, nil, nil
	if err := m.reloadFromDisk(); err != nil {
		m.errMsg = err.Error()
		return
	}
	m.errMsg = ""
	m.status = "reloaded from disk"
}

// collapse folds the block under the cursor. On a block with nothing to fold
// it folds the parent instead and goes there, so ctrl+↑ from deep inside a
// list closes the list you are in.
func (m *Model) collapse() {
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
	for i := m.cur - 1; i >= 0; i-- {
		if m.rows[i].depth < m.rows[m.cur].depth {
			m.moveCursor(i - m.cur)
			m.collapsed[m.currentPath()] = true
			m.buildRows()
			return
		}
	}
}

func (m *Model) expand() {
	b := m.current()
	if b == nil || len(b.Children) == 0 {
		return
	}
	delete(m.collapsed, m.currentPath())
	m.buildRows()
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
// command line cannot come to disagree about what indent means.
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
		m.errMsg = "file changed on disk — ctrl+r reloads it (your edit is not saved)"
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

// attach puts the editor on the block under the cursor, with the caret at a
// rune offset; a negative one means the end. Every write reloads the file, so
// the block to type into is whichever one the core put the cursor on, not the
// one we were holding.
func (m *Model) attach(caret int) {
	b := m.current()
	if b == nil {
		m.ed, m.edBlock, m.comp = nil, nil, nil
		return
	}
	m.edBlock = b
	m.ed = newEditor(b.Text)
	if caret >= 0 {
		m.ed.cur = max(0, min(caret, len(m.ed.runes)))
	}
	m.comp = nil
}

// syncEditor keeps the one invariant the outliner has: in the outline, the
// block under the cursor is the block being edited. It runs after every
// message, so a path that moves the cursor cannot leave the editor behind.
func (m *Model) syncEditor() {
	if m.mode != modeOutline || m.doc == nil {
		return
	}
	b := m.current()
	if m.ed != nil && m.edBlock == b {
		return
	}
	// The editor is on a block the cursor has left. Every move goes through
	// leave, so this is a path that forgot to; the text is written rather
	// than dropped, as long as the block is still part of this file.
	if m.ed != nil && m.edBlock != nil && m.ed.String() != m.edBlock.Text && m.holds(m.edBlock) {
		m.leave()
	}
	m.attach(-1)
}

// holds reports whether a block belongs to the document on screen. A reload
// replaces every block, and writing into one from before it would be writing
// into nothing.
func (m *Model) holds(b *markdown.Block) bool {
	for _, x := range m.doc.Doc.Flatten() {
		if x == b {
			return true
		}
	}
	return false
}

// leave folds the editor back into its block and writes the file if the text
// changed. The write happens on leaving the block, not on quitting, so the
// window in which a crash or an external edit can cost anything stays one block
// wide. Passing through a block without changing it writes nothing, which is
// what keeps browsing to a day with no file from creating one.
func (m *Model) leave() {
	ed, b := m.ed, m.edBlock
	m.ed, m.edBlock, m.comp = nil, nil, nil
	if ed != nil && b != nil && ed.String() != b.Text {
		b.Text = ed.String()
		m.save()
		// A longer or shorter block moves every block after it in the file,
		// and the core addresses blocks by where they start. Re-reading puts
		// the offsets right, so the next command on a later block is not
		// refused as stale. A refused save is not re-read: that would throw
		// the typing away.
		if !m.stale && m.errMsg == "" {
			if err := m.reloadFromDisk(); err != nil {
				m.errMsg = err.Error()
			}
		} else {
			m.buildRows()
		}
	}
	if m.graphDirty {
		m.graphDirty = false
		m.refresh()
		m.buildRows()
	}
}

// hasVisibleChildren answers the one question the core is not allowed to ask,
// because collapse is view state and only the view knows it.
func (m *Model) hasVisibleChildren(b *markdown.Block) bool {
	return len(b.Children) > 0 && !m.collapsed[m.currentPath()]
}

// structuralWhileTyping applies a core mutation without leaving the block, so
// Tab still indents mid-sentence. The text has to reach the file first: the core
// addresses a block by where it is, and half-typed text lives only here.
func (m *Model) structuralWhileTyping(fn func(app.Addr) (*app.Result, error)) {
	caret := m.ed.cur
	a, ok := m.commitText()
	if !ok {
		return
	}
	if m.apply(fn(a)) {
		m.attach(caret)
	}
}

// toggleTaskWhileTyping ticks the block off and keeps the caret on the same
// character, which has moved by however much the checkbox added or took away.
func (m *Model) toggleTaskWhileTyping() {
	caret, before := m.ed.cur, len(m.ed.runes)
	a, ok := m.commitText()
	if !ok {
		return
	}
	if !m.apply(m.svc.ToggleTask(a)) {
		return
	}
	after := 0
	if b := m.current(); b != nil {
		after = len([]rune(b.Text))
	}
	m.attach(caret + after - before)
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
	// What was typed is a step of its own, so undoing the indent or the
	// split that follows leaves the typing alone.
	m.svc.Checkpoint()
	return m.addrOf(m.edBlock), true
}

// backspaceAtStart joins the block onto the one above, the way every outliner
// behaves — which also makes it how an empty block is deleted. The first block
// has nothing above it, so there an empty one is simply removed.
func (m *Model) backspaceAtStart() {
	b := m.edBlock
	if m.cur == 0 && m.ed.String() == "" && len(b.Children) == 0 && len(m.doc.Doc.Blocks) > 1 {
		if !m.ensureOnDisk() {
			return
		}
		if m.apply(m.svc.DeleteBlock(m.addrOf(b))) {
			m.attach(0)
		}
		return
	}
	m.mergeIntoPrevious()
}

// mergeIntoPrevious is what backspace at the very start of a block does: join
// it onto the block above.
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
		m.attach(at)
	}
}

// --- navigation -------------------------------------------------------------

func (m *Model) goTo(rel string) {
	if m.doc != nil && rel == m.doc.Rel {
		return
	}
	m.leave()
	if m.doc != nil {
		m.history = append(m.history, m.doc.Rel)
	}
	if err := m.load(rel); err != nil {
		m.errMsg = err.Error()
	}
}

func (m *Model) back() {
	if len(m.history) == 0 {
		m.status = "nowhere to go back to"
		return
	}
	m.leave()
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

// followLinkHere follows a link in the block being typed, which may be one that
// was typed a moment ago and is not on disk yet.
func (m *Model) followLinkHere() {
	caret, rel := 0, m.doc.Rel
	if m.ed != nil {
		caret = m.ed.cur
	}
	m.leave()
	m.followLink()
	if m.doc.Rel == rel && m.mode == modeOutline {
		m.attach(caret)
	}
}

// followLink opens the page a link points at, creating it if this is the first
// time anyone has gone there.
func (m *Model) followLink() tea.Cmd {
	b := m.current()
	if b == nil {
		return nil
	}
	items := blockLinks(b)
	switch len(items) {
	case 0:
		m.status = "no link in this block"
		return nil
	case 1:
		if items[0].url != "" {
			m.openURL(items[0].url)
		} else {
			m.openLink(items[0].link)
		}
		return nil
	default:
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

// openPalette is the one place to go from: where you have just been, then
// today, then every command by name, then every journal and page. Typing
// narrows all of it at once, so reaching a page you went to five minutes ago
// is ctrl+p and enter, and a command you use once a month needs no key of its
// own.
func (m *Model) openPalette() {
	l := m.snapshot()
	if l == nil {
		return
	}
	seen := map[string]bool{m.doc.Rel: true}
	var items []pickItem
	add := func(it pickItem) {
		if !seen[it.rel] {
			seen[it.rel] = true
			items = append(items, it)
		}
	}
	for _, rel := range m.recent {
		add(pickItem{label: store.Title(rel), detail: "recent", rel: rel})
	}
	if today := m.svc.TodayRel(); !seen[today] {
		add(pickItem{label: store.Title(today), detail: "today", rel: today})
	}
	items = append(items, m.commands()...)
	// The core's list of places, journals newest first, so the palette, the
	// window and `tlog places` offer the same things in the same order.
	for _, p := range l.Places("", 0) {
		if p.Rel == "" {
			continue // a tag has no file to go to from here
		}
		add(pickItem{label: p.Name, detail: p.Kind, rel: p.Rel})
	}
	m.pick = newPicker("go to", items)
	m.pick.creates = true
	m.mode = modePalette
}

// commands are what the palette can do besides going somewhere. They are the
// outliner's own screens and the actions too rare or too destructive to sit on
// a key; slash commands, which change a block, stay in the block.
func (m *Model) commands() []pickItem {
	cmd := func(label, detail string, run func(*Model)) pickItem {
		return pickItem{label: label, detail: detail, run: run}
	}
	return []pickItem{
		cmd("Agenda", "everything with a deadline", (*Model).openAgenda),
		cmd("Backlinks", "what links to this page", (*Model).openBacklinks),
		cmd("Delete block", "the block under the cursor and everything in it", (*Model).deleteBlock),
		cmd("Reload from disk", "ctrl+r · discards what is not saved", (*Model).reload),
		cmd("Settings", "", (*Model).openSettings),
		cmd("Messages", "everything tlog has said", (*Model).openLog),
		cmd("Help", "F1 · how tlog works, and every key", func(m *Model) { m.mode = modeHelp }),
	}
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
			label: h.FirstLine, detail: h.Page, rel: h.Rel, offset: h.Offset,
		})
	}
	m.pick.items = items
	m.pick.filtered = items
	m.pick.sel = 0
}

func (m *Model) pickerKey(msg tea.KeyMsg, k string) (tea.Model, tea.Cmd) {
	switch k {
	case "esc", "ctrl+c":
		m.mode = modeOutline
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
		creates, query := m.pick.creates, strings.TrimSpace(m.pick.query.String())
		m.mode = modeOutline
		m.pick, m.look = nil, nil
		if !ok {
			// Nothing by that name: going there is how a page is made, the
			// same as following a [[link]] to it for the first time.
			if creates && query != "" {
				m.openLink(markdown.Link{Page: query})
			}
			return m, nil
		}
		if item.run != nil {
			item.run(m)
			return m, nil
		}
		if item.url != "" {
			m.openURL(item.url)
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
