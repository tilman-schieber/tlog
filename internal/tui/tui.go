// Package tui is the outliner. It is an adapter: every change it makes goes
// through app.Service, and it holds no logic the CLI could not also reach.
package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tilman-schieber/tlog/internal/app"
	"github.com/tilman-schieber/tlog/internal/graph"
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
)

type rowKind int

const (
	rowBlock  rowKind = iota // a block of this page, editable
	rowHeader                // a section heading below the outline
	rowLink                  // a page name to jump to
	rowRef                   // a block on another page that refers to this one
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
	g   *graph.Graph

	doc  *app.Doc
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

	backlinks []graph.Ref

	history []string
	status  string
	errMsg  string
	stale   bool

	pendingG bool
	pendingD bool

	quitting bool
}

// Run starts the outliner on a file, defaulting to today's journal.
func Run(svc *app.Service, rel string) error {
	if rel == "" {
		rel = svc.TodayRel()
	}
	m := &Model{svc: svc, collapsed: map[string]bool{}}
	if err := m.load(rel); err != nil {
		return err
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	if cerr := svc.Commit(); cerr != nil && err == nil {
		return cerr
	}
	return err
}

func (m *Model) Init() tea.Cmd { return nil }

// load reads a file and rebuilds the graph. At the corpus sizes this targets,
// reparsing everything is cheaper than keeping an index honest.
func (m *Model) load(rel string) error {
	d, err := m.svc.Load(rel)
	if err != nil {
		return err
	}
	m.doc = d
	m.stale = false
	if len(m.doc.Doc.Blocks) == 0 {
		m.doc.Doc.AppendChild(nil, &markdown.Block{})
	}
	m.cur, m.top = 0, 0
	m.rebuildGraph()
	m.buildRows()
	return nil
}

func (m *Model) rebuildGraph() {
	g, err := m.svc.Graph()
	if err != nil {
		m.errMsg = err.Error()
		return
	}
	m.g = g
}

func (m *Model) buildRows() {
	m.rows = m.rows[:0]
	var walk func(bs []*markdown.Block, depth int, prefix string)
	walk = func(bs []*markdown.Block, depth int, prefix string) {
		for i, b := range bs {
			path := fmt.Sprintf("%s%d.", prefix, i)
			m.rows = append(m.rows, row{block: b, depth: depth, path: path})
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
	if m.g == nil {
		return
	}
	name := store.PageName(m.doc.Rel)

	if tagged := m.g.PagesWithTag(name); len(tagged) > 0 {
		m.rows = append(m.rows, row{kind: rowHeader, text: fmt.Sprintf("%d pages tagged #%s", len(tagged), name)})
		for _, p := range tagged {
			m.rows = append(m.rows, row{kind: rowLink, text: p, depth: 0})
		}
	}

	refs := m.g.Backlinks(name)
	if len(refs) == 0 {
		return
	}
	m.rows = append(m.rows, row{kind: rowHeader, text: fmt.Sprintf("%d linked references", len(refs))})
	last := ""
	for _, r := range refs {
		if r.From.Name != last {
			last = r.From.Name
			m.rows = append(m.rows, row{kind: rowLink, text: r.From.Name, rel: r.From.Rel, depth: 0})
		}
		m.rows = append(m.rows, refRows(r, 1)...)
	}
}

// refRowBudget caps how much of one reference's subtree is shown, so that a
// deeply nested day does not bury the rest of the list.
const refRowBudget = 12

// refRows renders a referring block together with its children. The children
// are the point: a mention is usually the parent bullet of a name, and showing
// only that line says nothing but the name you already navigated to.
func refRows(r graph.Ref, depth int) []row {
	out := []row{{
		kind:   rowRef,
		text:   strings.TrimSpace(r.Block.FirstLine()),
		rel:    r.From.Rel,
		offset: r.Block.Start,
		depth:  depth,
	}}
	budget := refRowBudget
	var walk func(bs []*markdown.Block, d int)
	walk = func(bs []*markdown.Block, d int) {
		for _, c := range bs {
			if budget <= 0 {
				return
			}
			budget--
			out = append(out, row{
				kind:   rowRef,
				text:   strings.TrimSpace(c.FirstLine()),
				rel:    r.From.Rel,
				offset: c.Start,
				depth:  d,
			})
			walk(c.Children, d+1)
		}
	}
	walk(r.Block.Children, depth+1)
	if budget <= 0 {
		out = append(out, row{kind: rowRef, text: "…", rel: r.From.Rel, offset: r.Block.Start, depth: depth + 1})
	}
	return out
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
	case rowRef:
		m.goTo(r.rel)
		if b := m.doc.Doc.FindByOffset(r.offset); b != nil {
			m.focus(b)
		}
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
	// Links and tags just changed, and the sections below the outline are
	// derived from them.
	m.rebuildGraph()
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
	case "R":
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
		m.structural(func(d *markdown.Document, b *markdown.Block) bool { return d.Indent(b) })
	case "shift+tab":
		m.structural(func(d *markdown.Document, b *markdown.Block) bool { return d.Outdent(b) })
	case "alt+up", "K":
		m.structural(func(d *markdown.Document, b *markdown.Block) bool { return d.MoveUp(b) })
	case "alt+down", "J":
		m.structural(func(d *markdown.Document, b *markdown.Block) bool { return d.MoveDown(b) })

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

// structural applies a tree operation and saves. Structural changes rewrite the
// file, which is safe because it is already canonical.
func (m *Model) structural(fn func(*markdown.Document, *markdown.Block) bool) {
	b := m.current()
	if b == nil {
		return
	}
	if !fn(m.doc.Doc, b) {
		return
	}
	m.save()
	m.buildRows()
	m.focus(b)
}

// focus puts the cursor back on a block after the rows were rebuilt.
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
	if b == nil {
		return
	}
	if len(m.rows) == 1 {
		b.Text = ""
		b.Props = nil
		b.Anchor = ""
		m.save()
		m.buildRows()
		return
	}
	if err := m.doc.Doc.Remove(b); err != nil {
		m.errMsg = err.Error()
		return
	}
	m.save()
	m.buildRows()
	m.cur = min(m.cur, len(m.rows)-1)
	m.status = "block deleted — git has the previous version"
}

func (m *Model) toggleTask() {
	b := m.current()
	if b == nil {
		return
	}
	if !b.ToggleTask() {
		b.MakeTask()
	}
	m.save()
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
	nb := &markdown.Block{}
	var err error
	switch {
	case b == nil:
		m.doc.Doc.AppendChild(nil, nb)
	case above:
		err = m.doc.Doc.InsertBefore(b, nb)
	default:
		// A new block under a block with visible children becomes its first
		// child, which is what an outliner does and what Enter should feel like.
		if len(b.Children) > 0 && !m.collapsed[m.currentPath()] {
			b.Children = append([]*markdown.Block{nb}, b.Children...)
			nb.Parent = b
			m.doc.Doc.Reindex()
		} else {
			err = m.doc.Doc.InsertAfter(b, nb)
		}
	}
	if err != nil {
		m.errMsg = err.Error()
		return
	}
	m.buildRows()
	m.focus(nb)
	m.ed = newEditor("")
	m.edBlock = nb
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
		// costs one keypress.
		before, after := m.ed.split()
		m.edBlock.Text = before
		m.save()
		nb := &markdown.Block{Text: after}
		if len(m.edBlock.Children) > 0 && !m.collapsed[m.currentPath()] {
			m.edBlock.Children = append([]*markdown.Block{nb}, m.edBlock.Children...)
			nb.Parent = m.edBlock
			m.doc.Doc.Reindex()
		} else if err := m.doc.Doc.InsertAfter(m.edBlock, nb); err != nil {
			m.errMsg = err.Error()
			return m, nil
		}
		m.save()
		m.buildRows()
		m.focus(nb)
		m.edBlock = nb
		m.ed = newEditor(after)
		m.ed.cur = 0
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
		m.commitInsertKeepingPlace(func(b *markdown.Block) { m.doc.Doc.Indent(b) })
		return m, nil
	case "shift+tab":
		m.commitInsertKeepingPlace(func(b *markdown.Block) { m.doc.Doc.Outdent(b) })
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

// commitInsertKeepingPlace applies a structural change without leaving INSERT,
// so Tab still indents while typing.
func (m *Model) commitInsertKeepingPlace(fn func(*markdown.Block)) {
	b := m.edBlock
	if b == nil {
		return
	}
	b.Text = m.ed.String()
	cur := m.ed.cur
	fn(b)
	m.save()
	m.buildRows()
	m.focus(b)
	m.ed = newEditor(b.Text)
	m.ed.cur = min(cur, len(m.ed.runes))
}

// mergeIntoPrevious is what backspace at the very start of a block does: join
// it onto the block above, the way every outliner behaves.
func (m *Model) mergeIntoPrevious() {
	if m.cur == 0 || m.edBlock == nil {
		return
	}
	prev := m.rows[m.cur-1].block
	if len(m.edBlock.Children) > 0 {
		m.status = "cannot merge a block that has children — outdent them first"
		return
	}
	text := m.ed.String()
	at := len([]rune(prev.Text))
	prev.Text += text
	if err := m.doc.Doc.Remove(m.edBlock); err != nil {
		m.errMsg = err.Error()
		return
	}
	m.save()
	m.buildRows()
	m.focus(prev)
	m.edBlock = prev
	m.ed = newEditor(prev.Text)
	m.ed.cur = at
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
	name := store.PageName(m.doc.Rel)
	m.backlinks = m.g.Backlinks(name)
	if len(m.backlinks) == 0 {
		m.status = "nothing links to " + name
		return
	}
	items := make([]pickItem, 0, len(m.backlinks))
	for _, r := range m.backlinks {
		items = append(items, pickItem{
			label:  r.From.Name,
			detail: strings.TrimSpace(r.Block.FirstLine()),
			rel:    r.From.Rel,
			offset: r.Block.Start,
		})
	}
	m.pick = newPicker("backlinks to "+name, items)
	m.mode = modeBacklinks
}

func (m *Model) openPalette() {
	if m.g == nil {
		m.rebuildGraph()
	}
	var items []pickItem
	for _, p := range m.g.Journals() {
		items = append(items, pickItem{label: p.Name, detail: "journal", rel: p.Rel})
	}
	for _, name := range m.g.PageNames() {
		if p, ok := m.g.Page(name); ok {
			items = append(items, pickItem{label: p.Name, detail: "page", rel: p.Rel})
		}
	}
	m.pick = newPicker("open", items)
	m.mode = modePalette
}

func (m *Model) openSearch() {
	if m.g == nil {
		m.rebuildGraph()
	}
	m.pick = newPicker("search", nil)
	m.pick.live = true
	m.mode = modeSearch
}

func (m *Model) refreshSearch() {
	q := m.pick.query.String()
	hits := m.g.Search(q, 200)
	items := make([]pickItem, 0, len(hits))
	for _, h := range hits {
		items = append(items, pickItem{
			label:  strings.TrimSpace(h.Block.FirstLine()),
			detail: h.Page.Name,
			rel:    h.Page.Rel,
			offset: h.Offset,
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
		m.pick = nil
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
		m.pick = nil
		if !ok {
			return m, nil
		}
		if item.link.Page != "" {
			m.openLink(item.link)
			return m, nil
		}
		m.goTo(item.rel)
		if item.offset > 0 {
			if b := m.doc.Doc.FindByOffset(item.offset); b != nil {
				m.focus(b)
			}
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
