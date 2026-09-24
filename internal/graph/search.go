package graph

import (
	"sort"
	"strings"

	"github.com/tilman-schieber/tlog/internal/fuzzy"
	"github.com/tilman-schieber/tlog/internal/markdown"
)

// Hit is one search result. It carries the byte offset of the block so that a
// caller can address it for a later mutation, together with the file hash the
// offset was computed against — the two halves of a block address.
type Hit struct {
	Page   *Page
	Block  *markdown.Block
	Offset int
	Hash   string
	Score  int
}

// Search finds blocks whose text contains every whitespace-separated term,
// case-insensitively. Ranking favours matches near the start of a block and
// blocks in recent journals, which is where a half-remembered note usually is.
func (g *Graph) Search(query string, limit int) []Hit {
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return nil
	}
	var hits []Hit
	for _, p := range g.Order {
		p.Doc.Walk(func(b *markdown.Block) bool {
			lower := strings.ToLower(b.Text)
			score := 0
			for _, t := range terms {
				i := strings.Index(lower, t)
				if i < 0 {
					return true
				}
				score += 100 - min(i, 90)
			}
			if p.IsJournal {
				score += 10
			}
			score -= b.Depth
			hits = append(hits, Hit{Page: p, Block: b, Offset: b.Start, Hash: p.Hash, Score: score})
			return true
		})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		if hits[i].Page.IsJournal && hits[j].Page.IsJournal {
			return hits[i].Page.Day.After(hits[j].Page.Day)
		}
		return hits[i].Page.Name < hits[j].Page.Name
	})
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

// FuzzyRank and FuzzyMatch are the ranking helpers, kept here as the names the
// graph's own callers use. The implementation lives in internal/fuzzy, because
// ordering strings is not knowledge about the notes.
func FuzzyRank(candidates []string, query string, limit int) []string {
	return fuzzy.Rank(candidates, query, limit)
}

// FuzzyMatch scores a single candidate against a subsequence query.
func FuzzyMatch(candidate, query string) (int, bool) { return fuzzy.Match(candidate, query) }
