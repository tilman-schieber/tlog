package app

import (
	"strings"
	"testing"
)

func TestRefToNamesABlockAndTheLinkResolves(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToPage("Timetable", "the lecture is at nine")

	link, err := s.RefTo(addrOf(t, s, rel, 0))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(link, "[[Timetable#^") || !strings.HasSuffix(link, "]]") {
		t.Fatalf("not a block reference: %q", link)
	}
	if !strings.Contains(body(t, s, rel), "^") {
		t.Fatalf("the anchor was not written: %q", body(t, s, rel))
	}

	// Asking again is the same link and no second anchor.
	again, err := s.RefTo(addrOf(t, s, rel, 0))
	if err != nil {
		t.Fatal(err)
	}
	if again != link {
		t.Fatalf("a block got two names: %q then %q", link, again)
	}
	if strings.Count(body(t, s, rel), "^") != 1 {
		t.Fatalf("two anchors: %q", body(t, s, rel))
	}
}

func TestBlocksOffersWhatCanBeFoundAndNothingEmpty(t *testing.T) {
	s := newSvc(t)
	if _, err := s.AddToPage("Timetable", "the lecture is at nine"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddToPage("Other", "unrelated"); err != nil {
		t.Fatal(err)
	}

	got, err := s.Blocks("lecture", 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Page != "Timetable" {
		t.Fatalf("got %+v", got)
	}
	// The candidate carries everything needed to ask for a reference to it.
	if _, err := s.RefTo(got[0].Addr()); err != nil {
		t.Fatalf("a candidate could not be referred to: %v", err)
	}
}

func TestAnEmptyBlockIsNotWorthReferringTo(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToPage("Notes", "a real block")
	if _, err := s.AppendBlock(rel, "", ""); err != nil {
		t.Fatal(err)
	}
	for _, b := range mustBlocks(t, s, "a") {
		if strings.TrimSpace(b.Text) == "" {
			t.Fatalf("an empty block was offered: %+v", b)
		}
	}
}

func mustBlocks(t *testing.T, s *Service, q string) []BlockRef {
	t.Helper()
	got, err := s.Blocks(q, 8)
	if err != nil {
		t.Fatal(err)
	}
	return got
}
