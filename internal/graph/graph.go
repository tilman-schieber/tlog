// Package graph is the interpreted view of the notes directory: pages, the
// links between them and the backlinks that fall out of those links. It holds
// no truth of its own — everything here is derived from markdown and can be
// thrown away and rebuilt.
package graph

import (
	"sort"
	"strings"
	"time"

	"github.com/tilman-schieber/tlog/internal/markdown"
	"github.com/tilman-schieber/tlog/internal/store"
)

// Page is one file, parsed.
type Page struct {
	Rel       string
	Name      string
	Doc       *markdown.Document
	Hash      string
	IsJournal bool
	Day       time.Time
}

// Ref is one link from a block to a page, or to a block within a page.
type Ref struct {
	From  *Page
	Block *markdown.Block
	Link  markdown.Link
}

// Graph is the whole notes directory in memory. At the corpus sizes this tool
// targets, building it from scratch costs single-digit milliseconds, which is
// why there is no daemon and no persistent index.
type Graph struct {
	Pages map[string]*Page // keyed by lowercased page name
	Order []*Page

	backlinks map[string][]Ref // keyed by lowercased page name
	anchorRef map[string][]Ref // keyed by "page\x00anchor", lowercased page

	// A tag is just a page: #person and [[person]] name the same thing, which
	// is what lets the person page list every person without a second
	// namespace to keep in step.
	pageTags map[string]map[string]bool // lowercased page -> tags asserted about it
	tagPages map[string]map[string]bool // lowercased tag -> page names it was asserted on

	// An alias is another name for the same page: aliases: Julia on
	// "Julia Rausenberger" makes [[Julia]] mean her. It is resolved when a
	// reference is recorded and when one is looked up, so a page has exactly
	// one set of backlinks however it was named.
	aliases map[string]string // lowercased alias -> canonical page name
}

// Build parses every file in the store.
func Build(s *store.Store) (*Graph, error) {
	rels, err := s.List()
	if err != nil {
		return nil, err
	}
	g := &Graph{
		Pages:     make(map[string]*Page, len(rels)),
		backlinks: map[string][]Ref{},
		anchorRef: map[string][]Ref{},
		pageTags:  map[string]map[string]bool{},
		tagPages:  map[string]map[string]bool{},
		aliases:   map[string]string{},
	}
	for _, rel := range rels {
		f, err := s.Read(rel)
		if err != nil {
			continue
		}
		p := &Page{
			Rel:  rel,
			Name: store.PageName(rel),
			Doc:  markdown.Parse(f.Data),
			Hash: f.Hash,
		}
		if day, ok := store.JournalDay(rel); ok {
			p.IsJournal = true
			p.Day = day
		}
		g.Pages[strings.ToLower(p.Name)] = p
		g.Order = append(g.Order, p)
	}
	g.readAliases()
	g.link()
	return g, nil
}

// readAliases runs before link, because every reference is recorded under the
// canonical name and that has to be known first.
//
// An alias that collides with a real page loses: a file on disk is a stronger
// claim to a name than a line of frontmatter, and silently shadowing a page
// would make it unreachable.
func (g *Graph) readAliases() {
	for _, p := range g.Order {
		for _, fm := range p.Doc.Frontmatter {
			if !strings.EqualFold(fm.Key, "aliases") && !strings.EqualFold(fm.Key, "alias") {
				continue
			}
			for _, a := range splitList(fm.Value) {
				k := strings.ToLower(strings.TrimSpace(a))
				if k == "" {
					continue
				}
				if _, taken := g.Pages[k]; taken {
					continue
				}
				if _, dup := g.aliases[k]; dup {
					continue // first one wins, deterministically by file order
				}
				g.aliases[k] = p.Name
			}
		}
	}
}

// canon is the key a page's references live under, whichever of its names was
// used to write them.
func (g *Graph) canon(name string) string {
	k := strings.ToLower(strings.TrimSpace(name))
	if real, ok := g.aliases[k]; ok {
		return strings.ToLower(real)
	}
	return k
}

// Canonical is the real name of a page, given any of its names. The second
// result says whether the name given was an alias.
func (g *Graph) Canonical(name string) (string, bool) {
	real, ok := g.aliases[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return strings.TrimSpace(name), false
	}
	return real, true
}

// Aliases returns the other names a page answers to, sorted.
func (g *Graph) Aliases(name string) []string {
	lower := strings.ToLower(strings.TrimSpace(name))
	var out []string
	for a, real := range g.aliases {
		if strings.ToLower(real) == lower {
			out = append(out, a)
		}
	}
	sort.Strings(out)
	return out
}

func (g *Graph) link() {
	for _, p := range g.Order {
		// tags: a, b in a page's frontmatter is an equal way of saying what the
		// page is, for anyone who prefers declaring it on the page itself.
		for _, fm := range p.Doc.Frontmatter {
			if strings.EqualFold(fm.Key, "tags") {
				for _, t := range splitList(fm.Value) {
					g.assertTag(p.Name, t)
				}
			}
		}

		p.Doc.Walk(func(b *markdown.Block) bool {
			for _, l := range b.Links() {
				key := g.canon(l.Page)
				ref := Ref{From: p, Block: b, Link: l}
				g.backlinks[key] = append(g.backlinks[key], ref)
				if l.Anchor != "" {
					g.anchorRef[key+"\x00"+l.Anchor] = append(g.anchorRef[key+"\x00"+l.Anchor], ref)
				}
				// A tag inside the brackets describes the page being linked to,
				// so it is an assertion about that page rather than a mention
				// of the tag here.
				for _, t := range l.Tags {
					g.assertTag(l.Page, t)
				}
			}
			// A bare #tag in the text is an ordinary mention, so it becomes a
			// linked reference on the tag's own page.
			for _, t := range b.Tags() {
				key := g.canon(t)
				g.backlinks[key] = append(g.backlinks[key], Ref{From: p, Block: b, Link: markdown.Link{Page: t}})
			}
			return true
		})
	}
}

func (g *Graph) assertTag(page, tag string) {
	page, tag = strings.TrimSpace(page), strings.TrimSpace(tag)
	if page == "" || tag == "" {
		return
	}
	pk, tk := strings.ToLower(page), strings.ToLower(tag)
	if g.pageTags[pk] == nil {
		g.pageTags[pk] = map[string]bool{}
	}
	g.pageTags[pk][tag] = true
	if g.tagPages[tk] == nil {
		g.tagPages[tk] = map[string]bool{}
	}
	g.tagPages[tk][page] = true
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' }) {
		if p := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(part), "#")); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// PageTags returns what a page has been declared to be, gathered from every
// [[Page #tag]] anywhere and from its own frontmatter.
func (g *Graph) PageTags(name string) []string {
	set := g.pageTags[strings.ToLower(strings.TrimSpace(name))]
	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}

// PagesWithTag returns every page asserted to carry a tag. This is what makes
// the person page a list of people.
func (g *Graph) PagesWithTag(tag string) []string {
	set := g.tagPages[strings.ToLower(strings.TrimSpace(tag))]
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}

// TagNames returns every tag in use, for completion.
func (g *Graph) TagNames() []string {
	seen := map[string]string{}
	for _, set := range g.pageTags {
		for t := range set {
			seen[strings.ToLower(t)] = t
		}
	}
	for _, p := range g.Order {
		p.Doc.Walk(func(b *markdown.Block) bool {
			for _, t := range b.Tags() {
				seen[strings.ToLower(t)] = t
			}
			return true
		})
	}
	out := make([]string, 0, len(seen))
	for _, t := range seen {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}

// Page looks a page up by name, case-insensitively.
func (g *Graph) Page(name string) (*Page, bool) {
	p, ok := g.Pages[g.canon(name)]
	return p, ok
}

// Backlinks returns every reference pointing at a page, excluding references
// that originate on the page itself.
func (g *Graph) Backlinks(name string) []Ref {
	var out []Ref
	lower := g.canon(name)
	for _, r := range g.backlinks[lower] {
		if strings.ToLower(r.From.Name) == lower {
			continue
		}
		out = append(out, r)
	}
	sortRefs(out)
	return out
}

// BacklinksToBlock returns references pointing at one specific block.
func (g *Graph) BacklinksToBlock(page, anchor string) []Ref {
	out := append([]Ref(nil), g.anchorRef[g.canon(page)+"\x00"+anchor]...)
	sortRefs(out)
	return out
}

// UnresolvedLinks returns links that point at pages which do not exist yet.
// Pages are created when you follow a link, not when you write one, so these
// are normal rather than errors.
func (g *Graph) UnresolvedLinks() []Ref {
	var out []Ref
	for key, refs := range g.backlinks {
		if _, ok := g.Pages[key]; ok {
			continue
		}
		if _, ok := g.aliases[key]; ok {
			continue // it does exist; it is just called something else
		}
		out = append(out, refs...)
	}
	sortRefs(out)
	return out
}

// PageNames returns every existing page name, journals excluded, sorted.
func (g *Graph) PageNames() []string {
	var out []string
	for _, p := range g.Order {
		if !p.IsJournal {
			out = append(out, p.Name)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}

// LinkTargets returns every page name that is linked to anywhere, whether or
// not the page exists. Used for completion, so that a page you have referred to
// but not yet created still autocompletes.
func (g *Graph) LinkTargets() []string {
	seen := map[string]string{}
	for _, refs := range g.backlinks {
		for _, r := range refs {
			seen[strings.ToLower(r.Link.Page)] = r.Link.Page
		}
	}
	for _, p := range g.Order {
		if !p.IsJournal {
			seen[strings.ToLower(p.Name)] = p.Name
		}
	}
	// Both names complete, because either one works and you may remember
	// either one.
	for a := range g.aliases {
		seen[a] = a
	}
	out := make([]string, 0, len(seen))
	for _, v := range seen {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}

// Journals returns journal pages, newest first.
func (g *Graph) Journals() []*Page {
	var out []*Page
	for _, p := range g.Order {
		if p.IsJournal {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Day.After(out[j].Day) })
	return out
}

func sortRefs(refs []Ref) {
	sort.SliceStable(refs, func(i, j int) bool {
		if refs[i].From.IsJournal != refs[j].From.IsJournal {
			return refs[i].From.IsJournal // journals first, most recent context
		}
		if refs[i].From.IsJournal {
			return refs[i].From.Day.After(refs[j].From.Day)
		}
		return strings.ToLower(refs[i].From.Name) < strings.ToLower(refs[j].From.Name)
	})
}
