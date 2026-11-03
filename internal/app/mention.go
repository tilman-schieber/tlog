package app

import (
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/tilman-schieber/tlog/internal/fuzzy"
)

// Mention is one entry in the @ menu: a page, a date, or a page that does not
// exist yet. Whatever it is, accepting it writes an ordinary link. No kind of
// page is special in how it is found: everything is ranked by how recently and
// how often it is mentioned, and its tags are shown, so a person and a project
// with similar names are told apart in the menu rather than by being sorted
// into different sections.
type Mention struct {
	Kind   string   `json:"kind"`   // "page", "date" or "new"
	Name   string   `json:"name"`   // the page, or what the date is called
	Insert string   `json:"insert"` // what accepting writes in place of the @…
	Detail string   `json:"detail"` // why it is here: "person · 12 mentions", "Fr 07.11.  in 4 days"
	Tags   []string `json:"tags,omitempty"`
	ISO    string   `json:"iso,omitempty"`
	State  string   `json:"state,omitempty"`
}

// usage is how much a page is talked about: how often, and when last — the
// day of the newest journal that mentions it.
type usage struct {
	count int
	last  time.Time
}

func (l *Lookup) usage(name string) usage {
	var u usage
	for _, r := range l.g.Backlinks(name) {
		u.count++
		if r.From.IsJournal && r.From.Day.After(u.last) {
			u.last = r.From.Day
		}
	}
	return u
}

// byUse orders names by who you are writing about: most recently mentioned
// first, then most often.
func (l *Lookup) byUse(names []string) []string {
	us := make(map[string]usage, len(names))
	for _, n := range names {
		us[n] = l.usage(n)
	}
	out := append([]string(nil), names...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := us[out[i]], us[out[j]]
		if !a.last.Equal(b.last) {
			return a.last.After(b.last)
		}
		return a.count > b.count
	})
	return out
}

// match is how well a name answers a query. A name that starts with it — or
// one of its words does, or its initials are it, as "jr" is Julia
// Rausenberger's — is a strong match; any other subsequence is a loose one.
type match struct {
	name   string
	exact  bool // the name, or an alias, is what was typed
	strong bool
	score  int
}

func (l *Lookup) matchName(name, query string) (match, bool) {
	q := strings.ToLower(strings.TrimSpace(query))
	m := match{name: name}
	ok := false
	for _, n := range append([]string{name}, l.g.Aliases(name)...) {
		s, hit := fuzzy.Match(n, q)
		if !hit {
			continue
		}
		ok = true
		if strings.EqualFold(n, q) {
			m.exact = true
		}
		if s > m.score {
			m.score = s
		}
		if strongMatch(strings.ToLower(n), q) {
			m.strong = true
		}
	}
	return m, ok
}

func strongMatch(name, q string) bool {
	if strings.HasPrefix(name, q) {
		return true
	}
	var initials strings.Builder
	for _, w := range strings.Fields(name) {
		if strings.HasPrefix(w, q) {
			return true
		}
		initials.WriteString(string([]rune(w)[:1]))
	}
	return strings.HasPrefix(initials.String(), q) && len([]rune(q)) > 1
}

// rankNames orders names against a query: strong matches first, then by score,
// ties broken by use, so of two Julias the one you write about wins. With no
// query it is use alone.
func (l *Lookup) rankNames(names []string, query string) []match {
	ordered := l.byUse(names)
	if strings.TrimSpace(query) == "" {
		out := make([]match, len(ordered))
		for i, n := range ordered {
			out[i] = match{name: n, strong: true}
		}
		return out
	}
	var hits []match
	for _, n := range ordered {
		if m, ok := l.matchName(n, query); ok {
			hits = append(hits, m)
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].exact != hits[j].exact {
			return hits[i].exact
		}
		if hits[i].strong != hits[j].strong {
			return hits[i].strong
		}
		return hits[i].score > hits[j].score
	})
	return hits
}

// Tagged is every page a tag has been asserted on, by its frontmatter or by a
// [[Page #tag]] anywhere.
func (l *Lookup) Tagged(tag string) []string {
	return l.g.PagesWithTag(strings.TrimPrefix(strings.TrimSpace(tag), "#"))
}

// People is every page tagged person: the same as Tagged("person"), kept for
// the callers that ask for exactly that.
func (l *Lookup) People() []string { return l.Tagged("person") }

// Mentions is the @ menu for what has been typed after the @.
//
// Every page is a candidate, ranked by how recently and how often it is
// mentioned: the people and projects written about most come first because
// they are written about most, not because of what they are. A name that
// starts with what was typed beats a loose match, an exact name or alias beats
// both, and the days the typing could mean sit between the strong and the loose
// matches, so @fri still works. When nothing answers to a whole name, the last
// entry makes a new page; what it is can be said with a tag inside the link,
// [[Name #person]], now or whenever.
func (l *Lookup) Mentions(query string, limit int) []Mention {
	q := strings.TrimSpace(query)

	var hits []match
	if q == "" {
		hits = take(l.rankNames(l.targets(), ""), 5)
	} else {
		hits = take(l.rankNames(l.targets(), q), 8)
	}

	page := func(m match) Mention {
		tags := l.g.PageTags(m.name)
		detail := plural(l.usage(m.name).count, "mention")
		if len(tags) > 0 {
			detail = strings.Join(tags, ", ") + " · " + detail
		}
		return Mention{Kind: "page", Name: m.name, Insert: "[[" + m.name + "]]", Detail: detail, Tags: tags}
	}

	var out []Mention
	for _, m := range hits {
		if m.strong || m.exact {
			out = append(out, page(m))
		}
	}
	for _, d := range DateChoices(q, 3) {
		out = append(out, Mention{
			Kind: "date", Name: d.Phrase, Insert: "[[" + d.ISO + "]]",
			Detail: d.Short + "  " + d.Label, ISO: d.ISO, State: d.State,
		})
	}
	for _, m := range hits {
		if !m.strong && !m.exact {
			out = append(out, page(m))
		}
	}
	out = take(out, limit)

	strong := len(hits) > 0 && (hits[0].strong || hits[0].exact)
	if q != "" && !l.exists(q) && looksLikeAName(q) && (!strong || strings.Contains(q, " ")) {
		if len(out) >= limit && limit > 0 {
			out = out[:limit-1] // the way to make a page is never crowded out
		}
		out = append(out, Mention{Kind: "new", Name: q, Insert: "[[" + q + "]]", Detail: "new page"})
	}
	return out
}

// targets is every page a link could point at, each once, under its real
// name. An alias is not a page of its own: matching looks at a page's aliases,
// so typing one finds the page it belongs to.
func (l *Lookup) targets() []string {
	seen := map[string]bool{}
	var out []string
	for _, n := range l.g.LinkTargets() {
		real, _ := l.g.Canonical(n)
		if k := strings.ToLower(real); !seen[k] {
			seen[k] = true
			out = append(out, real)
		}
	}
	return out
}

// exists reports whether a page or alias already answers to a name.
func (l *Lookup) exists(name string) bool {
	_, ok := l.g.Canonical(name)
	if ok {
		return true
	}
	_, ok = l.g.Page(name)
	return ok
}

// looksLikeAName keeps "@fri", "@in 3 days" and a stray "@here" from being
// offered as new pages: a name is capitalised or has a space in it, the way
// names are written, and what reads as a day is a day — unless it is a
// capitalised name with a space, as "Mai Tran" is somebody. A topic in lower
// case is still a page; it is written #moodle or [[moodle]].
func looksLikeAName(s string) bool {
	r := []rune(s)
	if len(r) < 2 || !(unicode.IsUpper(r[0]) || strings.Contains(s, " ")) {
		return false
	}
	if len(DateChoices(s, 1)) == 0 {
		return true
	}
	return strings.Contains(s, " ") && unicode.IsUpper(r[0])
}

func take[T any](xs []T, n int) []T {
	if len(xs) > n {
		return xs[:n]
	}
	return xs
}

func plural(n int, unit string) string {
	switch n {
	case 0:
		return "no " + unit + "s yet"
	case 1:
		return "1 " + unit
	}
	return strconv.Itoa(n) + " " + unit + "s"
}

// rankPages is the [[ completion: the same matching as @, over every name a
// link could point at, so the pages you write about come first.
func (l *Lookup) rankPages(prefix string, limit int) []string {
	hits := take(l.rankNames(l.targets(), prefix), limitOr(limit))
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.name
	}
	return out
}

func limitOr(n int) int {
	if n <= 0 {
		return 1 << 30
	}
	return n
}
