package tui

import (
	"github.com/tilman-schieber/tlog/internal/fuzzy"
	"github.com/tilman-schieber/tlog/internal/markdown"
)

// pickItem is one row in the palette, the search results, the backlink list or
// the follow-link chooser. They are all the same widget.
type pickItem struct {
	label  string
	detail string
	rel    string
	offset int
	link   markdown.Link
}

type picker struct {
	title    string
	query    *editor
	items    []pickItem
	filtered []pickItem
	sel      int
	live     bool // results come from a query rather than from filtering
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
