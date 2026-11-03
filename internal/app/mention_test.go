package app

import (
	"strings"
	"testing"
)

func mentionNames(ms []Mention) []string {
	var out []string
	for _, m := range ms {
		out = append(out, m.Kind+":"+m.Name)
	}
	return out
}

func peopleService(t *testing.T) *Lookup {
	t.Helper()
	s := newSvc(t)
	for _, p := range []string{"Julia Rausenberger", "Juliane Reck", "Oliver Mülken"} {
		if err := s.Store.Write("pages/"+p+".md", []byte("---\ntags: Person\n---\n\n- about them\n"), ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Store.Write("pages/Elke.md", []byte("---\naliases: EH\n---\n\n- x\n"), ""); err != nil {
		t.Fatal(err)
	}
	// Julia is written about most, so she comes first among the Jul… people.
	for _, line := range []string{"met [[Julia Rausenberger]]", "and [[Julia Rausenberger]] again", "[[Juliane Reck]] too", "see [[NOA]]"} {
		if _, err := s.AddToday(line); err != nil {
			t.Fatal(err)
		}
	}
	l, err := s.Lookup()
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestMentionsRankEveryPageByUse(t *testing.T) {
	l := peopleService(t)
	got := mentionNames(l.Mentions("", 8))
	// Julia is written about most and most recently; nothing is first for
	// being a person.
	if len(got) < 4 || got[0] != "page:Julia Rausenberger" || got[1] != "page:Juliane Reck" {
		t.Fatalf("got %v", got)
	}
	if !strings.HasPrefix(got[len(got)-1], "date:") {
		t.Fatalf("the usual days should follow the pages: %v", got)
	}
	m := l.Mentions("", 8)[0]
	if m.Insert != "[[Julia Rausenberger]]" || m.Detail != "Person · 2 mentions" || len(m.Tags) != 1 {
		t.Fatalf("a page's tags are shown beside it: %+v", m)
	}
}

func TestMentionsMatchInitialsAndFirstNames(t *testing.T) {
	l := peopleService(t)
	for q, want := range map[string]string{"jr": "page:Julia Rausenberger", "oli": "page:Oliver Mülken", "julia": "page:Julia Rausenberger"} {
		if got := mentionNames(l.Mentions(q, 8)); len(got) == 0 || got[0] != want {
			t.Errorf("@%s: %v", q, got)
		}
	}
	// A page by its alias, and a page by its exact name before anything loose.
	if got := mentionNames(l.Mentions("EH", 8)); len(got) == 0 || got[0] != "page:Elke" {
		t.Errorf("@EH: %v", got)
	}
	if got := mentionNames(l.Mentions("noa", 8)); len(got) == 0 || got[0] != "page:NOA" {
		t.Errorf("@noa: %v", got)
	}
}

// A project mentioned more than a person comes first: what something is shows
// beside it, it does not decide where it is.
func TestAProjectMentionedMoreOutranksAPerson(t *testing.T) {
	s := newSvc(t)
	if err := s.Store.Write("pages/Tilda Meyer.md", []byte("---\ntags: person\n---\n\n- x\n"), ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.Write("pages/tlog.md", []byte("---\ntags: repo\n---\n\n- x\n"), ""); err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"[[Tilda Meyer]]", "[[tlog]]", "[[tlog]] again"} {
		if _, err := s.AddToday(line); err != nil {
			t.Fatal(err)
		}
	}
	l, err := s.Lookup()
	if err != nil {
		t.Fatal(err)
	}
	got := l.Mentions("t", 8)
	if len(got) < 2 || got[0].Name != "tlog" || got[0].Detail != "repo · 2 mentions" || got[1].Name != "Tilda Meyer" {
		t.Fatalf("got %+v", got)
	}
}

func TestMentionsStillPickDates(t *testing.T) {
	got := peopleService(t).Mentions("fri", 8)
	if len(got) != 1 || got[0].Kind != "date" || !strings.HasPrefix(got[0].Insert, "[[20") {
		t.Fatalf("got %+v", got)
	}
}

func TestANewPageIsOfferedOnlyWhenNothingIsMeant(t *testing.T) {
	l := peopleService(t)
	// Halfway through typing Julia, nothing new is meant.
	for _, m := range l.Mentions("ju", 8) {
		if m.Kind == "new" {
			t.Fatalf("@ju offered a new page: %+v", m)
		}
	}
	// A whole new name is offered as a plain link: what it is can be said
	// with a tag inside it, now or later.
	got := l.Mentions("Maria Muster", 8)
	last := got[len(got)-1]
	if last.Kind != "new" || last.Insert != "[[Maria Muster]]" || last.Detail != "new page" {
		t.Fatalf("got %+v", got)
	}
	// What exists is not offered again as new, and a lower-case word or a
	// day is not a name.
	for _, q := range []string{"Oliver Mülken", "here", "fri", "in 3 days"} {
		for _, m := range l.Mentions(q, 8) {
			if m.Kind == "new" {
				t.Fatalf("@%s offered a new page: %+v", q, m)
			}
		}
	}
	// A capitalised name is a name even when it starts like a month.
	got = l.Mentions("Mai Tran", 8)
	if got[len(got)-1].Kind != "new" {
		t.Fatalf("@Mai Tran: %+v", got)
	}
}
