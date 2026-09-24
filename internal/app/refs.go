package app

import "strings"

// Block references. The syntax and the graph have been here from the start —
// [[Page#^anchor]] parses, resolves and shows up in backlinks — with no way to
// make one. This is that way.
//
// The anchor is materialised at the moment a reference is made and not before,
// which is why Anchor writes and reading never does: a corpus that has never
// been referred to has no ^anchors in it at all.

// BlockRef is a block that could be referred to: what it says, so a chooser can
// show it, and where it is, so a link to it can be asked for.
type BlockRef struct {
	Text   string `json:"text"`
	Page   string `json:"page"`
	Rel    string `json:"rel"`
	Offset int    `json:"offset"`
	Hash   string `json:"hash"`
}

// Addr is where this block is, for asking Anchor or RefTo about it.
func (r BlockRef) Addr() Addr { return Addr{Rel: r.Rel, Offset: r.Offset, Hash: r.Hash} }

// Blocks finds blocks to refer to. It is the same search the palette uses, so
// what you can find you can refer to, with empty blocks left out because a
// reference to one says nothing.
func (l *Lookup) Blocks(query string, limit int) []BlockRef {
	var out []BlockRef
	for _, h := range l.g.Search(query, 0) {
		if strings.TrimSpace(h.Block.Text) == "" {
			continue
		}
		out = append(out, BlockRef{
			Text:   h.Block.Text,
			Page:   h.Page.Name,
			Rel:    h.Page.Rel,
			Offset: h.Offset,
			Hash:   h.Hash,
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

// Blocks is Lookup.Blocks for a caller that wants one answer and no snapshot.
func (s *Service) Blocks(query string, limit int) ([]BlockRef, error) {
	l, err := s.Lookup()
	if err != nil {
		return nil, err
	}
	return l.Blocks(query, limit), nil
}

// RefTo gives a block a durable name and returns the link text that points at
// it. Calling it twice on the same block returns the same link and writes once,
// because EnsureAnchor keeps the anchor a block already has.
func (s *Service) RefTo(a Addr) (string, error) {
	l, err := s.Anchor(a)
	if err != nil {
		return "", err
	}
	return l.String(), nil
}
