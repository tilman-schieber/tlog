package app

import (
	"strings"
	"testing"
)

func TestTagAndUntagAPage(t *testing.T) {
	s := newSvc(t)
	rel := "pages/tlog.md"
	if err := s.Store.Write(rel, []byte("---\naliases: tl\n---\n\n- the outliner\n"), ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Tag(rel, "#repo"); err != nil {
		t.Fatal(err)
	}
	if err := s.Tag(rel, "projekt"); err != nil {
		t.Fatal(err)
	}
	if err := s.Tag(rel, "Repo"); err != nil { // again, in another case: nothing changes
		t.Fatal(err)
	}
	want := "---\naliases: tl\ntags: repo, projekt\n---\n\n- the outliner\n"
	if got := body(t, s, rel); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	pages, err := s.Tagged("repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || pages[0].Name != "tlog" || pages[0].Rel != rel || !pages[0].Exists || len(pages[0].Tags) != 2 {
		t.Fatalf("tagged: %+v", pages)
	}

	if err := s.Untag(rel, "repo"); err != nil {
		t.Fatal(err)
	}
	if err := s.Untag(rel, "projekt"); err != nil {
		t.Fatal(err)
	}
	// The last tag takes its line with it; the rest of the frontmatter stays.
	if got := body(t, s, rel); got != "---\naliases: tl\n---\n\n- the outliner\n" {
		t.Fatalf("got:\n%s", got)
	}
	// Taking off what is not there is refused, not pretended.
	if err := s.Untag(rel, "repo"); err == nil || !strings.Contains(err.Error(), "does not carry") {
		t.Fatalf("expected a refusal, got %v", err)
	}
}

// Saying what a page is is a reason to have one: a page only ever linked gets
// a file holding its tag.
func TestTaggingAPageThatOnlyExistsAsALinkMakesIt(t *testing.T) {
	s := newSvc(t)
	if _, err := s.AddToday("met [[Noah Müller]]"); err != nil {
		t.Fatal(err)
	}
	rel, _, err := s.ResolvePage("Noah Müller")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Tag(rel, "person"); err != nil {
		t.Fatal(err)
	}
	if got := body(t, s, rel); !strings.HasPrefix(got, "---\ntags: person\n---") {
		t.Fatalf("got %q", got)
	}
	people, _ := s.People()
	if len(people) != 1 || people[0] != "Noah Müller" {
		t.Fatalf("people: %v", people)
	}
}

func TestATagMustFitAFrontmatterList(t *testing.T) {
	s := newSvc(t)
	for _, bad := range []string{"", "#", "a, b", "x]"} {
		if err := s.Tag("pages/X.md", bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

// A tag said by a link elsewhere counts as much as one in the frontmatter.
func TestTaggedIncludesTagsSaidInLinks(t *testing.T) {
	s := newSvc(t)
	if _, err := s.AddToday("call [[Ada Lovelace #person]]"); err != nil {
		t.Fatal(err)
	}
	pages, err := s.Tagged("Person")
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || pages[0].Name != "Ada Lovelace" || pages[0].Exists || pages[0].Rel != "pages/Ada Lovelace.md" {
		t.Fatalf("tagged: %+v", pages)
	}
}
