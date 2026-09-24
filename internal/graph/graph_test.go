package graph

import (
	"strings"
	"testing"

	"github.com/tilman-schieber/tlog/internal/store"
)

func build(t *testing.T, files map[string]string) *Graph {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for rel, body := range files {
		if err := s.Write(rel, []byte(body), ""); err != nil {
			t.Fatal(err)
		}
	}
	g, err := Build(s)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestBacklinksExcludeSelfReferences(t *testing.T) {
	g := build(t, map[string]string{
		"pages/Rust.md":          "- [[Rust]] talking about itself\n",
		"journals/2026-09-17.md": "- learning [[Rust]]\n",
		"journals/2026-09-15.md": "- more [[Rust]]\n",
		"pages/Unrelated.md":     "- nothing here\n",
	})
	refs := g.Backlinks("Rust")
	if len(refs) != 2 {
		t.Fatalf("expected 2 backlinks, got %d", len(refs))
	}
	// Journals sort newest first, because that is where recent context is.
	if refs[0].From.Name != "2026-09-17" {
		t.Fatalf("wrong order: %s then %s", refs[0].From.Name, refs[1].From.Name)
	}
}

func TestBacklinksToASpecificBlock(t *testing.T) {
	g := build(t, map[string]string{
		"pages/Target.md":        "- the thought ^abc123\n",
		"journals/2026-09-17.md": "- see [[Target#^abc123]]\n- see [[Target]]\n",
	})
	if got := len(g.BacklinksToBlock("Target", "abc123")); got != 1 {
		t.Fatalf("block backlinks: %d", got)
	}
	if got := len(g.Backlinks("Target")); got != 2 {
		t.Fatalf("page backlinks should include both forms: %d", got)
	}
}

func TestUnresolvedLinksAreNormalNotErrors(t *testing.T) {
	g := build(t, map[string]string{
		"journals/2026-09-17.md": "- about [[Never Created]] and [[Exists]]\n",
		"pages/Exists.md":        "- here\n",
	})
	un := g.UnresolvedLinks()
	if len(un) != 1 || un[0].Link.Page != "Never Created" {
		t.Fatalf("unresolved: %+v", un)
	}
}

func TestLinkTargetsIncludePagesThatDoNotExistYet(t *testing.T) {
	g := build(t, map[string]string{
		"journals/2026-09-17.md": "- about [[Not Yet]]\n",
		"pages/Exists.md":        "- here\n",
	})
	targets := g.LinkTargets()
	var hasNotYet, hasExists bool
	for _, t2 := range targets {
		if t2 == "Not Yet" {
			hasNotYet = true
		}
		if t2 == "Exists" {
			hasExists = true
		}
	}
	if !hasNotYet || !hasExists {
		t.Fatalf("completion candidates incomplete: %v", targets)
	}
	// Journals are not completion targets; you link to pages, not to dates.
	for _, t2 := range g.PageNames() {
		if strings.HasPrefix(t2, "2026-") {
			t.Fatalf("journal leaked into page names: %v", g.PageNames())
		}
	}
}

func TestPageLookupIsCaseInsensitive(t *testing.T) {
	g := build(t, map[string]string{"pages/Project Foo.md": "- x\n"})
	if _, ok := g.Page("project FOO"); !ok {
		t.Fatal("case-insensitive lookup failed")
	}
	if _, ok := g.Page("nope"); ok {
		t.Fatal("found a page that does not exist")
	}
}

func TestSearchRequiresEveryTerm(t *testing.T) {
	g := build(t, map[string]string{
		"pages/A.md": "- use sqlite as a cache\n- use postgres as a database\n",
	})
	if got := len(g.Search("sqlite cache", 10)); got != 1 {
		t.Fatalf("both terms must match the same block: %d", got)
	}
	if got := len(g.Search("sqlite database", 10)); got != 0 {
		t.Fatalf("terms from different blocks must not match: %d", got)
	}
	if got := len(g.Search("", 10)); got != 0 {
		t.Fatalf("an empty query should match nothing: %d", got)
	}
}

func TestSearchHitCarriesAUsableAddress(t *testing.T) {
	g := build(t, map[string]string{"pages/A.md": "- first\n\n- findme\n"})
	hits := g.Search("findme", 10)
	if len(hits) != 1 {
		t.Fatalf("hits: %d", len(hits))
	}
	h := hits[0]
	if h.Hash != h.Page.Hash || h.Offset != h.Block.Start {
		t.Fatal("a hit must carry the offset and the hash it was computed against")
	}
	if h.Offset == 0 {
		t.Fatal("the second block cannot start at offset zero")
	}
}

func TestSearchRespectsTheLimit(t *testing.T) {
	g := build(t, map[string]string{"pages/A.md": "- x one\n\n- x two\n\n- x three\n"})
	if got := len(g.Search("x", 2)); got != 2 {
		t.Fatalf("limit ignored: %d", got)
	}
}

func TestFuzzyRankPrefersWordStartsAndRuns(t *testing.T) {
	got := FuzzyRank([]string{"Miscellaneous Project Notes", "Project Foo", "Prototype"}, "profo", 10)
	if len(got) == 0 || got[0] != "Project Foo" {
		t.Fatalf("ranking: %v", got)
	}
	if got := FuzzyRank([]string{"abc"}, "xyz", 10); len(got) != 0 {
		t.Fatalf("non-matching candidate returned: %v", got)
	}
	all := []string{"a", "b", "c"}
	if got := FuzzyRank(all, "", 2); len(got) != 2 {
		t.Fatalf("empty query should keep order and respect the limit: %v", got)
	}
}

func TestJournalsAreOrderedNewestFirst(t *testing.T) {
	g := build(t, map[string]string{
		"journals/2026-09-15.md": "- a\n",
		"journals/2026-09-17.md": "- b\n",
		"pages/A.md":             "- c\n",
	})
	js := g.Journals()
	if len(js) != 2 || js[0].Name != "2026-09-17" {
		t.Fatalf("journals: %+v", js)
	}
}

func TestTagPagesCollectEveryPageTypedAnywhere(t *testing.T) {
	g := build(t, map[string]string{
		"journals/2026-09-17.md": "- met [[Ada Lovelace #person]] about [[Analytical Engine #project]]\n",
		"journals/2026-09-15.md": "- called [[Alan Turing #person]]\n",
	})
	people := g.PagesWithTag("person")
	if len(people) != 2 || people[0] != "Ada Lovelace" || people[1] != "Alan Turing" {
		t.Fatalf("people: %v", people)
	}
	if projects := g.PagesWithTag("project"); len(projects) != 1 || projects[0] != "Analytical Engine" {
		t.Fatalf("projects: %v", projects)
	}
	// Neither page needs to exist as a file for this to work.
	if _, ok := g.Page("Ada Lovelace"); ok {
		t.Fatal("test assumes the page file does not exist")
	}
}

func TestPageTagsAreVisibleFromThePage(t *testing.T) {
	g := build(t, map[string]string{
		"journals/2026-09-17.md": "- [[Ada Lovelace #person]] and again [[Ada Lovelace #colleague]]\n",
	})
	tags := g.PageTags("ada lovelace")
	if len(tags) != 2 || tags[0] != "colleague" || tags[1] != "person" {
		t.Fatalf("tags: %v", tags)
	}
}

func TestFrontmatterTagsAreEquivalent(t *testing.T) {
	g := build(t, map[string]string{
		"pages/Ada Lovelace.md": "---\ntags: person, colleague\n---\n\n- x\n",
	})
	if tags := g.PageTags("Ada Lovelace"); len(tags) != 2 {
		t.Fatalf("frontmatter tags ignored: %v", tags)
	}
	if people := g.PagesWithTag("person"); len(people) != 1 || people[0] != "Ada Lovelace" {
		t.Fatalf("people: %v", people)
	}
}

func TestBareTagIsALinkToItsPage(t *testing.T) {
	g := build(t, map[string]string{
		"journals/2026-09-17.md": "- some thinking about #research\n",
		"pages/research.md":      "- the topic\n",
	})
	refs := g.Backlinks("research")
	if len(refs) != 1 || !strings.Contains(refs[0].Block.Text, "some thinking") {
		t.Fatalf("a #tag should be a link to its own page: %+v", refs)
	}
}

func TestTypingAPageDoesNotClutterTheTagPage(t *testing.T) {
	g := build(t, map[string]string{
		"journals/2026-09-17.md": "- [[Ada Lovelace #person]]\n- [[Alan Turing #person]]\n",
	})
	if refs := g.Backlinks("person"); len(refs) != 0 {
		t.Fatalf("typing pages should not become linked references on the tag page: %+v", refs)
	}
	if len(g.PagesWithTag("person")) != 2 {
		t.Fatal("but they must still be listed as pages tagged person")
	}
}

func TestTagNamesForCompletion(t *testing.T) {
	g := build(t, map[string]string{
		"journals/2026-09-17.md": "- [[A #person]] and bare #research\n",
	})
	names := g.TagNames()
	var hasPerson, hasResearch bool
	for _, n := range names {
		hasPerson = hasPerson || n == "person"
		hasResearch = hasResearch || n == "research"
	}
	if !hasPerson || !hasResearch {
		t.Fatalf("tag names: %v", names)
	}
}

// --- aliases -----------------------------------------------------------------
//
// An alias is another name for the same page, so that [[Ada]] finds Ada
// Lovelace. It is resolved when a reference is recorded and when one is looked
// up, which is what gives a page one set of backlinks however it was named.

func aliasGraph(t *testing.T) *Graph {
	t.Helper()
	return build(t, map[string]string{
		"pages/Ada Lovelace.md":  "---\naliases: Ada, Countess Lovelace\n---\n\n- the first programmer\n",
		"journals/2026-09-24.md": "- lunch with [[Ada]]\n- and later [[Ada Lovelace]]\n",
	})
}

func TestAnAliasResolvesToThePage(t *testing.T) {
	g := aliasGraph(t)
	p, ok := g.Page("Ada")
	if !ok || p.Name != "Ada Lovelace" {
		t.Fatalf("got %+v, %v", p, ok)
	}
	if real, was := g.Canonical("countess lovelace"); !was || real != "Ada Lovelace" {
		t.Fatalf("got %q, %v", real, was)
	}
	if real, was := g.Canonical("Ada Lovelace"); was || real != "Ada Lovelace" {
		t.Fatalf("a real name is not an alias: %q, %v", real, was)
	}
}

func TestBothNamesReachTheSameBacklinks(t *testing.T) {
	g := aliasGraph(t)
	byReal := g.Backlinks("Ada Lovelace")
	if len(byReal) != 2 {
		t.Fatalf("a mention by either name belongs to her: %d", len(byReal))
	}
	if len(g.Backlinks("Ada")) != len(byReal) {
		t.Fatal("asking by the alias gave a different answer")
	}
}

func TestAnAliasedMentionIsNotAnUnresolvedLink(t *testing.T) {
	g := aliasGraph(t)
	for _, r := range g.UnresolvedLinks() {
		if strings.EqualFold(r.Link.Page, "Ada") {
			t.Fatal("[[Ada]] was reported as a page that does not exist")
		}
	}
}

func TestBothNamesComplete(t *testing.T) {
	g := aliasGraph(t)
	var hasReal, hasAlias bool
	for _, n := range g.LinkTargets() {
		switch strings.ToLower(n) {
		case "ada lovelace":
			hasReal = true
		case "ada":
			hasAlias = true
		}
	}
	if !hasReal || !hasAlias {
		t.Fatalf("completion offers real=%v alias=%v", hasReal, hasAlias)
	}
}

func TestAPageAlwaysBeatsAnAliasForItsOwnName(t *testing.T) {
	// Otherwise a line of frontmatter could make a real file unreachable.
	g := build(t, map[string]string{
		"pages/Ada Lovelace.md":  "---\naliases: Grace\n---\n\n- the first programmer\n",
		"pages/Grace.md":         "- Grace Hopper\n",
		"journals/2026-09-24.md": "- [[Grace]]\n",
	})
	p, ok := g.Page("Grace")
	if !ok || p.Name != "Grace" {
		t.Fatalf("the alias shadowed a real page: %+v", p)
	}
	if len(g.Backlinks("Grace")) != 1 {
		t.Fatalf("the mention went to the wrong page: %+v", g.Backlinks("Grace"))
	}
	if len(g.Backlinks("Ada Lovelace")) != 0 {
		t.Fatal("the mention was stolen by the alias")
	}
}

func TestAliasesListsTheOtherNames(t *testing.T) {
	g := aliasGraph(t)
	got := g.Aliases("Ada Lovelace")
	if len(got) != 2 || got[0] != "ada" || got[1] != "countess lovelace" {
		t.Fatalf("got %+v", got)
	}
}
