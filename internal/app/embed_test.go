package app

import (
	"strings"
	"testing"
)

// A reference that draws as the page name tells the reader nothing about the
// block it points at. These check that what comes back is the block's own text,
// and that a block which is nothing but a pointer brings the subtree with it.

func withRef(t *testing.T) (*Service, string, string) {
	t.Helper()
	s := newSvc(t)
	target, _ := s.AddToPage("Timetable", "the lecture is at nine")
	if _, err := s.AppendBlock(target, "", "room B103"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Indent(addrOf(t, s, target, 1)); err != nil {
		t.Fatal(err)
	}
	link, err := s.RefTo(addrOf(t, s, target, 0))
	if err != nil {
		t.Fatal(err)
	}
	here, _ := s.AddToday("remember: " + link)
	return s, here, link
}

func TestAReferenceResolvesToWhatItPointsAt(t *testing.T) {
	s, here, _ := withRef(t)

	v, err := s.View(here)
	if err != nil {
		t.Fatal(err)
	}
	b := v.Blocks[0]
	if len(b.Embeds) != 1 {
		t.Fatalf("nothing resolved: %+v", b)
	}
	e := b.Embeds[0]
	if e.Text != "the lecture is at nine" {
		t.Fatalf("got %q — the reader needs the block, not the page name", e.Text)
	}
	if e.Page != "Timetable" || e.Missing {
		t.Fatalf("got %+v", e)
	}
	// And it is addressable, so following it is a read away.
	if _, _, err := s.Resolve(e.Addr); err != nil {
		t.Fatalf("the resolved reference is not addressable: %v", err)
	}
}

func TestABlockThatIsOnlyAReferenceIsAnEmbed(t *testing.T) {
	s, _, link := withRef(t)
	rel, _ := s.AddToPage("Notes", link)

	v, err := s.View(rel)
	if err != nil {
		t.Fatal(err)
	}
	b := v.Blocks[0]
	if !b.IsEmbed {
		t.Fatalf("a block that is nothing but a pointer should embed: %+v", b)
	}
	// The subtree comes with it — that is the reason to embed rather than refer.
	if len(b.EmbedKids) != 1 || b.EmbedKids[0].Text != "room B103" {
		t.Fatalf("children: %+v", b.EmbedKids)
	}
}

func TestProseAroundAReferenceIsNotAnEmbed(t *testing.T) {
	// Inlining a subtree into the middle of a sentence would be nonsense, so
	// the rule is strict: one reference and nothing else.
	s, here, _ := withRef(t)
	v, err := s.View(here)
	if err != nil {
		t.Fatal(err)
	}
	if v.Blocks[0].IsEmbed {
		t.Fatalf("%q embedded a subtree mid-sentence", v.Blocks[0].Text)
	}
	if len(v.Blocks[0].EmbedKids) != 0 {
		t.Fatal("children were carried for something that is not an embed")
	}
}

func TestAReferenceToAVanishedBlockSaysSo(t *testing.T) {
	// A dangling pointer the reader cannot see is worse than an ugly one.
	s := newSvc(t)
	rel, _ := s.AddToday("see [[Timetable#^gone42]]")
	v, err := s.View(rel)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Blocks[0].Embeds) != 1 || !v.Blocks[0].Embeds[0].Missing {
		t.Fatalf("got %+v", v.Blocks[0].Embeds)
	}
	if v.Blocks[0].IsEmbed {
		t.Fatal("a reference with nothing behind it must not embed")
	}
}

func TestAPlainPageLinkIsNotAReference(t *testing.T) {
	s := newSvc(t)
	if _, err := s.AddToPage("Timetable", "the lecture is at nine"); err != nil {
		t.Fatal(err)
	}
	rel, _ := s.AddToday("see [[Timetable]]")
	v, err := s.View(rel)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Blocks[0].Embeds) != 0 {
		t.Fatalf("a page link resolved as a block reference: %+v", v.Blocks[0].Embeds)
	}
}

func TestSeveralReferencesResolveInOrder(t *testing.T) {
	// An adapter substitutes each one where it stands, so the order is the
	// contract.
	s := newSvc(t)
	rel, _ := s.AddToPage("P", "first")
	if _, err := s.AppendBlock(rel, "", "second"); err != nil {
		t.Fatal(err)
	}
	one, err := s.RefTo(addrOf(t, s, rel, 0))
	if err != nil {
		t.Fatal(err)
	}
	two, err := s.RefTo(addrOf(t, s, rel, 1))
	if err != nil {
		t.Fatal(err)
	}
	here, _ := s.AddToday("compare " + two + " with " + one)

	v, err := s.View(here)
	if err != nil {
		t.Fatal(err)
	}
	got := v.Blocks[0].Embeds
	if len(got) != 2 || got[0].Text != "second" || got[1].Text != "first" {
		t.Fatalf("got %+v", got)
	}
}

func TestAnEmbeddedSubtreeIsBounded(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToPage("Deep", "root")
	for i := 0; i < 40; i++ {
		if _, err := s.AppendBlock(rel, "", "child"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Indent(addrOf(t, s, rel, i+1)); err != nil {
			t.Fatal(err)
		}
	}
	link, err := s.RefTo(addrOf(t, s, rel, 0))
	if err != nil {
		t.Fatal(err)
	}
	here, _ := s.AddToPage("Notes", link)
	v, err := s.View(here)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(v.Blocks[0].EmbedKids); n == 0 || n > embedChildBudget {
		t.Fatalf("carried %d children, budget is %d", n, embedChildBudget)
	}
}

func TestAReferenceStillCountsAsABacklink(t *testing.T) {
	// Resolving it for display must not change what it is.
	s, _, _ := withRef(t)
	v, err := s.View("pages/Timetable.md")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range v.Refs {
		if strings.Contains(r.Text, "remember") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the reference is not in the target's backlinks: %+v", v.Refs)
	}
}
