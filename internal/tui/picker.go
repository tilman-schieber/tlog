package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/tilman-schieber/tlog/internal/fuzzy"
	"github.com/tilman-schieber/tlog/internal/markdown"
)

// pickKind is what a row in the palette is, which is what it is drawn as. The
// other lists hold one kind of thing each and leave it plain.
type pickKind int

const (
	pickPlain   pickKind = iota
	pickPage             // somewhere to go
	pickDay              // a journal
	pickCommand          // something to do
	pickDanger           // something to do that throws something away
)

// pickItem is one row in the palette, the search results, the backlink list or
// the follow-link chooser. They are all the same widget.
type pickItem struct {
	label  string
	detail string
	rel    string
	offset int
	link   markdown.Link
	run    func(*Model) // a command rather than a place
	url    string       // an address to hand to the desktop

	kind  pickKind
	tags  []string // what a page has been said to be
	today bool
	group string // the heading the row sits under while nothing is typed
}

// mark is what stands before a row's name. Colour alone would not say what a
// row is in a terminal whose theme has little of it, and says nothing at all
// on the row that is selected, which is drawn in reverse.
func (it pickItem) mark() string {
	if it.kind == pickCommand || it.kind == pickDanger {
		return "> "
	}
	return ""
}

// tagText is a page's tags as they are written, to draw and to match against.
func (it pickItem) tagText() string {
	if len(it.tags) == 0 {
		return ""
	}
	return "#" + strings.Join(it.tags, " #")
}

// plain is the whole row without colour: what the selected row shows.
func (it pickItem) plain() string {
	s := it.mark() + it.label
	if t := it.tagText(); t != "" {
		s += "  " + t
	}
	if it.detail != "" {
		s += "  " + it.detail
	}
	return s
}

// styled is the row as it is drawn when it is not the selected one.
func (it pickItem) styled() string {
	name := lipgloss.NewStyle()
	switch it.kind {
	case pickDay:
		name = styleDay
	case pickCommand:
		name = styleCommand
	case pickDanger:
		name = styleDanger
	}
	if it.today {
		name = name.Bold(true)
	}
	label := name.Render(it.mark() + it.label)
	if it.label == "" {
		label = styleMuted.Render("(empty block)")
	}
	if t := it.tagText(); t != "" {
		label += "  " + styleTag.Render(t)
	}
	if it.detail != "" {
		label += "  " + styleMuted.Render(it.detail)
	}
	return label
}

type picker struct {
	title    string
	query    *editor
	items    []pickItem
	filtered []pickItem
	sel      int
	live     bool // results come from a query rather than from filtering
	creates  bool // enter on a name that matches nothing makes that page
}

func newPicker(title string, items []pickItem) *picker {
	return &picker{
		title:    title,
		query:    newEditor(""),
		items:    items,
		filtered: items,
	}
}

func (p *picker) filter() {
	q := p.query.String()
	if q == "" {
		p.filtered = p.items
		p.sel = 0
		return
	}
	type scored struct {
		item  pickItem
		score int
	}
	var hits []scored
	for _, it := range p.items {
		best, ok := fuzzy.Match(it.label, q)
		if s, ok2 := fuzzy.Match(it.detail, q); ok2 && s-4 > best {
			best, ok = s-4, true
		}
		// A page answers to what it is, so "person" lists the people.
		if s, ok2 := fuzzy.Match(it.tagText(), q); ok2 && s-4 > best {
			best, ok = s-4, true
		}
		if ok {
			hits = append(hits, scored{it, best})
		}
	}
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0 && hits[j].score > hits[j-1].score; j-- {
			hits[j], hits[j-1] = hits[j-1], hits[j]
		}
	}
	// A fresh slice: p.filtered may still alias p.items.
	out := make([]pickItem, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.item)
	}
	p.filtered = out
	p.sel = 0
}

func (p *picker) move(d int) {
	if len(p.filtered) == 0 {
		return
	}
	p.sel = max(0, min(p.sel+d, len(p.filtered)-1))
}

func (p *picker) selected() (pickItem, bool) {
	if p.sel < 0 || p.sel >= len(p.filtered) {
		return pickItem{}, false
	}
	return p.filtered[p.sel], true
}

// pickRow is one line of a list as it is drawn: a row to choose, or the
// heading above a group of them.
type pickRow struct {
	head string
	item int // an index into filtered; -1 for a heading
}

// rows lays the list out. Headings appear only while nothing is typed: after
// that the list is ranked by how well each row matches, the groups are mixed,
// and a row says what it is by its colour and its mark alone.
func (p *picker) rows() []pickRow {
	grouped := p.query.String() == "" && !p.live
	rows := make([]pickRow, 0, len(p.filtered)+4)
	last := ""
	for i, it := range p.filtered {
		if grouped && it.group != "" && it.group != last {
			rows = append(rows, pickRow{head: it.group, item: -1})
		}
		last = it.group
		rows = append(rows, pickRow{item: i})
	}
	return rows
}

// window is the part of the list that fits in height lines, scrolled so that
// the selected row is in it. What is drawn and what a click lands on are both
// read from it, so they cannot disagree.
func (p *picker) window(height int) []pickRow {
	rows := p.rows()
	at := 0
	for i, r := range rows {
		if r.item == p.sel {
			at = i
			break
		}
	}
	top := 0
	if at >= height {
		top = at - height + 1
	}
	return rows[top:min(len(rows), top+height)]
}
