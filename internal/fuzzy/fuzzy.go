// Package fuzzy ranks strings against a typed prefix, the way a command palette
// does: contiguous runs, word boundaries and matches near the start all count
// for more.
//
// It lives on its own because ranking is not knowledge about the notes. The
// graph uses it to order search results and completion candidates; the adapters
// use it to filter a picker. Neither has to know about the other.
package fuzzy

import (
	"sort"
	"strings"
)

// Rank orders candidates by how well they match a subsequence query, the
// way a command palette does. Empty queries keep the original order.
func Rank(candidates []string, query string, limit int) []string {
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
		if n, ok := score(strings.ToLower(c), q); ok {
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

// Match scores a single candidate against a subsequence query. An empty
// query matches everything with a neutral score.
func Match(candidate, query string) (int, bool) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return 0, true
	}
	return score(strings.ToLower(candidate), q)
}

// score rewards contiguous runs, matches at word boundaries and matches
// near the start of the candidate.
func score(s, q string) (int, bool) {
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
