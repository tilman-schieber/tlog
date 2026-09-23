package graph

import (
	"sort"
	"strings"

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

// FuzzyRank orders candidates by how well they match a subsequence query, the
// way a command palette does. Empty queries keep the original order.
func FuzzyRank(candidates []string, query string, limit int) []string {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		if limit > 0 && len(candidates) > limit {
			return candidates[:limit]
		}
		return candidates
	}
	type scored struct {
		s string
		n int
	}
	var out []scored
	for _, c := range candidates {
		if n, ok := fuzzyScore(strings.ToLower(c), q); ok {
			out = append(out, scored{c, n})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].n != out[j].n {
			return out[i].n > out[j].n
		}
		return len(out[i].s) < len(out[j].s)
	})
	res := make([]string, 0, len(out))
	for _, s := range out {
		res = append(res, s.s)
	}
	if limit > 0 && len(res) > limit {
		res = res[:limit]
	}
	return res
}

// FuzzyMatch scores a single candidate against a subsequence query. An empty
// query matches everything with a neutral score.
func FuzzyMatch(candidate, query string) (int, bool) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return 0, true
	}
	return fuzzyScore(strings.ToLower(candidate), q)
}

// fuzzyScore rewards contiguous runs, matches at word boundaries and matches
// near the start of the candidate.
func fuzzyScore(s, q string) (int, bool) {
	score := 0
	si := 0
	prevMatch := -2
	for _, r := range q {
		i := strings.IndexRune(s[si:], r)
		if i < 0 {
			return 0, false
		}
		at := si + i
		switch {
		case at == prevMatch+1:
			score += 8
		case at == 0 || s[at-1] == ' ' || s[at-1] == '-' || s[at-1] == '/':
			score += 6
		default:
			score += 1
		}
		if at < 8 {
			score += 2
		}
		prevMatch = at
		si = at + 1
	}
	return score, true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
