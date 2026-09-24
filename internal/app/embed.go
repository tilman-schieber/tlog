package app

import (
	"strings"

	"github.com/tilman-schieber/tlog/internal/graph"
	"github.com/tilman-schieber/tlog/internal/markdown"
)

// Showing what a reference points at.
//
// [[Page#^anchor]] parsed, resolved and produced backlinks from the start, but
// it drew as the page name — you pointed at one block out of a hundred and were
// shown the word "Timetable". A reference nobody can read is a reference nobody
// makes, which is why this and the (( that creates them are the same feature
// arriving in two parts.
//
// The resolution happens here rather than in either adapter, because finding
// the block behind an anchor means having the whole graph, and an adapter that
// had the graph would be deciding things.

// embedChildBudget caps how much of an embedded subtree is carried, so that
// embedding a deep block cannot bury the page doing the embedding.
const embedChildBudget = 24

// EmbedView is a block referred to from another one, resolved so an adapter can
// show what it says instead of only where it is.
type EmbedView struct {
	Anchor string `json:"anchor"`
	Page   string `json:"page"`
	Rel    string `json:"rel"`
	Addr   Addr   `json:"addr"`
	Text   string `json:"text"`
	Depth  int    `json:"depth,omitempty"`

	// Missing is a reference whose target is gone — the page was deleted, or
	// the anchor was edited away. It is shown as a reference that has lost its
	// block rather than silently as nothing, because a dangling pointer the
	// reader cannot see is worse than an ugly one they can.
	Missing bool `json:"missing,omitempty"`
}

// embedsFor resolves every block reference in a block's text, in the order they
// appear, so that an adapter can substitute each one where it stands.
func embedsFor(g *graph.Graph, b *markdown.Block) []EmbedView {
	var out []EmbedView
	for _, l := range b.Links() {
		if l.Anchor == "" {
			continue
		}
		out = append(out, resolveEmbed(g, l))
	}
	return out
}

func resolveEmbed(g *graph.Graph, l markdown.Link) EmbedView {
	e := EmbedView{Anchor: l.Anchor, Page: l.Page, Missing: true}
	p, ok := g.Page(l.Page)
	if !ok {
		return e
	}
	e.Page, e.Rel = p.Name, p.Rel
	target := p.Doc.FindByAnchor(l.Anchor)
	if target == nil {
		return e
	}
	e.Missing = false
	e.Text = target.Text
	e.Addr = Addr{Rel: p.Rel, Offset: target.Start, Hash: p.Hash}
	return e
}

// isEmbed reports whether a block is nothing but a reference. Such a block is
// drawn as the thing it points at, children and all — an embed, with no syntax
// of its own to learn.
//
// The rule is deliberately strict: one reference and nothing else. A block that
// says anything besides the pointer is prose that happens to cite something,
// and inlining a subtree into the middle of a sentence would be nonsense.
func isEmbed(text string, embeds []EmbedView) bool {
	if len(embeds) != 1 || embeds[0].Missing {
		return false
	}
	t := strings.TrimSpace(text)
	return t == "[["+embeds[0].Page+"#^"+embeds[0].Anchor+"]]"
}

// embedChildren flattens what is under an embedded block, so that embedding a
// block brings its subtree with it — which is the whole reason to embed rather
// than to refer.
func embedChildren(g *graph.Graph, e EmbedView) []EmbedView {
	p, ok := g.Page(e.Page)
	if !ok {
		return nil
	}
	target := p.Doc.FindByAnchor(e.Anchor)
	if target == nil {
		return nil
	}
	var out []EmbedView
	budget := embedChildBudget
	var walk func(bs []*markdown.Block, depth int)
	walk = func(bs []*markdown.Block, depth int) {
		for _, c := range bs {
			if budget <= 0 {
				return
			}
			budget--
			out = append(out, EmbedView{
				Anchor: c.Anchor,
				Page:   p.Name,
				Rel:    p.Rel,
				Addr:   Addr{Rel: p.Rel, Offset: c.Start, Hash: p.Hash},
				Text:   c.Text,
				Depth:  depth,
			})
			walk(c.Children, depth+1)
		}
	}
	walk(target.Children, 1)
	return out
}
