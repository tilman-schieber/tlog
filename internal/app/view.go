package app

import (
	"strings"

	"github.com/tilman-schieber/tlog/internal/graph"
	"github.com/tilman-schieber/tlog/internal/markdown"
	"github.com/tilman-schieber/tlog/internal/store"
)

// The read model. Adapters render these; they do not walk the graph themselves,
// so what the outliner shows and what an agent reads over JSON stay the same
// thing described once.

// FenceView is a fenced region of a block, prepared for drawing: highlighted
// when the language is known, and parsed into rows when it is tabular. Both
// adapters render from this, so code is coloured the same way everywhere and a
// JSON caller gets the same thing.
type FenceView struct {
	Lang   string           `json:"lang,omitempty"`
	Code   string           `json:"code"`
	Tokens []markdown.Token `json:"tokens,omitempty"`
	Rows   [][]string       `json:"rows,omitempty"`
}

// BlockView is one block, flattened into document order with its depth.
type BlockView struct {
	Offset      int             `json:"offset"`
	Depth       int             `json:"depth"`
	Text        string          `json:"text"`
	Anchor      string          `json:"anchor,omitempty"`
	Props       []markdown.Prop `json:"props,omitempty"`
	Task        string          `json:"task,omitempty"` // "", "open" or "done"
	HasChildren bool            `json:"hasChildren"`
	Fences      []FenceView     `json:"fences,omitempty"`
}

// RefView is a block elsewhere that mentions this page, or one of its children.
type RefView struct {
	Page   string `json:"page"`
	Rel    string `json:"rel"`
	Offset int    `json:"offset"`
	Text   string `json:"text"`
	Depth  int    `json:"depth"`
	Head   bool   `json:"head"` // the mention itself, rather than a child of it
}

// PageView is everything needed to draw a page: its own blocks, what it is,
// what it groups and what mentions it.
type PageView struct {
	Rel       string      `json:"rel"`
	Title     string      `json:"title"`
	Hash      string      `json:"hash"`
	IsJournal bool        `json:"isJournal"`
	Tags      []string    `json:"tags"`
	Blocks    []BlockView `json:"blocks"`
	Tagged    []string    `json:"tagged"`
	Refs      []RefView   `json:"refs"`
}

// SearchHit is one search result, addressable for a later mutation.
type SearchHit struct {
	Rel    string `json:"rel"`
	Page   string `json:"page"`
	Offset int    `json:"offset"`
	Hash   string `json:"hash"`
	Text   string `json:"text"`
}

// Index is the list of everything, for a sidebar or a palette.
type Index struct {
	Journals []string `json:"journals"`
	Pages    []string `json:"pages"`
	Tags     []string `json:"tags"`
}

// refChildBudget caps how much of a mention's subtree is carried, so one deep
// day cannot bury the rest of the list.
const refChildBudget = 12

// View builds the read model for a file, rebuilding the graph so that the
// sections reflect what is on disk right now.
func (s *Service) View(rel string) (*PageView, error) {
	d, err := s.Load(rel)
	if err != nil {
		return nil, err
	}
	g, err := s.Graph()
	if err != nil {
		return nil, err
	}
	return s.viewWith(g, d), nil
}

// ViewPage is View by page name, creating the page if following a link there
// for the first time.
func (s *Service) ViewPage(name string) (*PageView, error) {
	rel, err := s.OpenPage(name)
	if err != nil {
		return nil, err
	}
	return s.View(rel)
}

// ViewToday is View for today's journal, which need not exist yet.
func (s *Service) ViewToday() (*PageView, error) { return s.View(s.TodayRel()) }

func (s *Service) viewWith(g *graph.Graph, d *Doc) *PageView {
	title := store.PageName(d.Rel)
	v := &PageView{
		Rel:       d.Rel,
		Title:     title,
		Hash:      d.Hash,
		IsJournal: store.IsJournal(d.Rel),
		Tags:      g.PageTags(title),
		Tagged:    g.PagesWithTag(title),
	}

	d.Doc.Walk(func(b *markdown.Block) bool {
		v.Blocks = append(v.Blocks, BlockView{
			Offset:      b.Start,
			Depth:       b.Depth,
			Text:        b.Text,
			Anchor:      b.Anchor,
			Props:       b.Props,
			Task:        taskName(b.Task()),
			HasChildren: len(b.Children) > 0,
			Fences:      fenceViews(b.Text),
		})
		return true
	})

	for _, r := range g.Backlinks(title) {
		v.Refs = append(v.Refs, refViews(r)...)
	}
	return v
}

// fenceViews prepares every fenced region of a block for drawing.
func fenceViews(text string) []FenceView {
	fences := markdown.Fences(text)
	if len(fences) == 0 {
		return nil
	}
	out := make([]FenceView, 0, len(fences))
	for _, f := range fences {
		v := FenceView{Lang: f.Lang, Code: f.Body}
		if markdown.IsCSVLang(f.Lang) {
			// A table that does not parse yet falls back to code, because a
			// half-typed table is still worth seeing.
			if rows, ok := markdown.CSVTable(f.Lang, f.Body); ok {
				v.Rows = rows
			}
		}
		if v.Rows == nil {
			v.Tokens = markdown.Highlight(f.Lang, f.Body)
		}
		out = append(out, v)
	}
	return out
}

func taskName(t markdown.TaskState) string {
	switch t {
	case markdown.TaskOpen:
		return "open"
	case markdown.TaskDone:
		return "done"
	}
	return ""
}

// refViews flattens a mention and its children. The children are the point: a
// mention is usually a bare name with the substance nested under it.
func refViews(r graph.Ref) []RefView {
	out := []RefView{{
		Page:   r.From.Name,
		Rel:    r.From.Rel,
		Offset: r.Block.Start,
		Text:   strings.TrimSpace(r.Block.FirstLine()),
		Depth:  0,
		Head:   true,
	}}
	budget := refChildBudget
	var walk func(bs []*markdown.Block, d int)
	walk = func(bs []*markdown.Block, d int) {
		for _, c := range bs {
			if budget <= 0 {
				return
			}
			budget--
			out = append(out, RefView{
				Page:   r.From.Name,
				Rel:    r.From.Rel,
				Offset: c.Start,
				Text:   strings.TrimSpace(c.FirstLine()),
				Depth:  d,
			})
			walk(c.Children, d+1)
		}
	}
	walk(r.Block.Children, 1)
	return out
}

// Index lists journals newest first, then pages, then the tags in use.
func (s *Service) Index() (*Index, error) {
	g, err := s.Graph()
	if err != nil {
		return nil, err
	}
	idx := &Index{Pages: g.PageNames(), Tags: g.TagNames()}
	for _, p := range g.Journals() {
		idx.Journals = append(idx.Journals, p.Name)
	}
	return idx, nil
}

// Search finds blocks matching every term, with addresses safe to mutate
// through.
func (s *Service) Search(query string, limit int) ([]SearchHit, error) {
	g, err := s.Graph()
	if err != nil {
		return nil, err
	}
	var out []SearchHit
	for _, h := range g.Search(query, limit) {
		out = append(out, SearchHit{
			Rel:    h.Page.Rel,
			Page:   h.Page.Name,
			Offset: h.Offset,
			Hash:   h.Hash,
			Text:   strings.TrimSpace(h.Block.FirstLine()),
		})
	}
	return out, nil
}

// CompletePages ranks page names for a [[link]] prefix, including pages that
// have been linked to but not yet created.
func (s *Service) CompletePages(prefix string, limit int) ([]string, error) {
	g, err := s.Graph()
	if err != nil {
		return nil, err
	}
	return graph.FuzzyRank(g.LinkTargets(), prefix, limit), nil
}

// CompleteTags ranks the tags in use for a #tag prefix.
func (s *Service) CompleteTags(prefix string, limit int) ([]string, error) {
	g, err := s.Graph()
	if err != nil {
		return nil, err
	}
	return graph.FuzzyRank(g.TagNames(), prefix, limit), nil
}
