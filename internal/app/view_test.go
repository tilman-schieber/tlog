package app

import "testing"

func TestPlacesRanksEverywhereYouCouldGo(t *testing.T) {
	s := newSvc(t)
	if _, err := s.AddToPage("Ada Lovelace", "the first programmer [[engine #project]]"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddToPage("Grace Hopper", "the first compiler"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddToday("a note"); err != nil {
		t.Fatal(err)
	}

	all, err := s.Places("", 0)
	if err != nil {
		t.Fatal(err)
	}
	var kinds = map[string]int{}
	for _, p := range all {
		kinds[p.Kind]++
	}
	if kinds["journal"] == 0 || kinds["page"] == 0 || kinds["tag"] == 0 {
		t.Fatalf("not everywhere: %+v", kinds)
	}
	// A journal is what is most often being looked for, so it comes first.
	if all[0].Kind != "journal" {
		t.Fatalf("got %+v", all[0])
	}

	// The same fuzzy ranking the [[link]] completion uses, so a name that
	// completes one way is found the other way too.
	hits, err := s.Places("lov", 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 || hits[0].Name != "Ada Lovelace" {
		t.Fatalf("got %+v", hits)
	}
	if hits[0].Rel != "pages/Ada Lovelace.md" {
		t.Fatalf("a page must carry its path so opening it creates nothing: %+v", hits[0])
	}

	// A tag may have no file of its own, and that is not an error.
	for _, p := range all {
		if p.Kind == "tag" && p.Rel != "" {
			t.Fatalf("a tag should carry no path: %+v", p)
		}
	}
}
