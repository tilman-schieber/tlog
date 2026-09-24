package app

import (
	"strings"
	"testing"
)

// Enter is the most frequent key in an outliner, and it used to be SetText
// followed by InsertAfter — two writes, two commits, and a window in which the
// file could move between them and leave half the split behind.

func TestSplitBlockIsOneWrite(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToday("onetwo")

	res, err := s.SplitBlock(addrOf(t, s, rel, 0), "one", "two", false)
	if err != nil {
		t.Fatal(err)
	}
	if got := body(t, s, rel); got != "- one\n\n- two\n" {
		t.Fatalf("got %q", got)
	}
	// The result addresses the new block, so a caller can keep typing in it.
	d, _ := s.Load(rel)
	b := d.Doc.FindByOffset(res.Offset)
	if b == nil || b.Text != "two" {
		t.Fatalf("result points at %+v", b)
	}
}

func TestSplitBlockNestsOnlyWhenTheAdapterSaysSo(t *testing.T) {
	// The collapsed-children bug: the core used to decide this itself, and the
	// two adapters disagreed about a block whose children were hidden.
	s := newSvc(t)
	rel, _ := s.AddToday("parent")
	if _, err := s.AppendBlock(rel, "", "child"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Indent(addrOf(t, s, rel, 1)); err != nil {
		t.Fatal(err)
	}

	if _, err := s.SplitBlock(addrOf(t, s, rel, 0), "par", "ent", true); err != nil {
		t.Fatal(err)
	}
	if got := body(t, s, rel); got != "- par\n  - ent\n  - child\n" {
		t.Fatalf("asChild: %q", got)
	}

	s2 := newSvc(t)
	rel2, _ := s2.AddToday("parent")
	if _, err := s2.AppendBlock(rel2, "", "child"); err != nil {
		t.Fatal(err)
	}
	if _, err := s2.Indent(addrOf(t, s2, rel2, 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := s2.SplitBlock(addrOf(t, s2, rel2, 0), "par", "ent", false); err != nil {
		t.Fatal(err)
	}
	if got := body(t, s2, rel2); got != "- par\n  - child\n\n- ent\n" {
		t.Fatalf("as sibling: %q", got)
	}
}

func TestSplitKeepsTheChildrenWithTheOriginal(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToday("onetwo")
	if _, err := s.AppendBlock(rel, "", "child"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Indent(addrOf(t, s, rel, 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SplitBlock(addrOf(t, s, rel, 0), "one", "two", false); err != nil {
		t.Fatal(err)
	}
	// Splitting a block does not rehome what was under it.
	if got := body(t, s, rel); got != "- one\n  - child\n\n- two\n" {
		t.Fatalf("got %q", got)
	}
}

func TestMergeIntoPrevious(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToday("one")
	if _, err := s.AppendBlock(rel, "", "two"); err != nil {
		t.Fatal(err)
	}

	res, err := s.MergeIntoPrevious(addrOf(t, s, rel, 1))
	if err != nil {
		t.Fatal(err)
	}
	if got := body(t, s, rel); got != "- onetwo\n" {
		t.Fatalf("got %q", got)
	}
	d, _ := s.Load(rel)
	if b := d.Doc.FindByOffset(res.Offset); b == nil || b.Text != "onetwo" {
		t.Fatalf("the result should address the surviving block: %+v", b)
	}
}

func TestMergeIsRefusedWhenItWouldOrphanChildren(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToday("one")
	if _, err := s.AppendBlock(rel, "", "two"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendBlock(rel, "", "child"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Indent(addrOf(t, s, rel, 2)); err != nil {
		t.Fatal(err)
	}
	before := body(t, s, rel)

	_, err := s.MergeIntoPrevious(addrOf(t, s, rel, 1))
	if err == nil || !strings.Contains(err.Error(), "outdent") {
		t.Fatalf("expected a refusal naming the way out, got %v", err)
	}
	if body(t, s, rel) != before {
		t.Fatal("a refused merge changed the file anyway")
	}
}

func TestMergeAtTheTopIsRefused(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToday("only")
	if _, err := s.MergeIntoPrevious(addrOf(t, s, rel, 0)); err == nil {
		t.Fatal("there is nothing above the first block")
	}
}

func TestSplitAndMergeRefuseAStaleAddress(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToday("mine")
	a := addrOf(t, s, rel, 0)
	if err := s.Store.Write(rel, []byte("- theirs\n"), ""); err != nil {
		t.Fatal(err)
	}

	if _, err := s.SplitBlock(a, "mi", "ne", false); err == nil {
		t.Fatal("split accepted a stale address")
	}
	if _, err := s.MergeIntoPrevious(a); err == nil {
		t.Fatal("merge accepted a stale address")
	}
	if got := body(t, s, rel); got != "- theirs\n" {
		t.Fatalf("the other writer's work was destroyed: %q", got)
	}
}

func TestAsChildNestsEvenWhenThereAreNoChildrenYet(t *testing.T) {
	// It used to be ignored on a childless block, which made it a request the
	// core could quietly decline. Both outliners only ask when children are
	// already there, so nothing noticed until a script asked plainly.
	s := newSvc(t)
	rel, _ := s.AddToday("parent")

	if _, err := s.InsertAfter(addrOf(t, s, rel, 0), "child", true); err != nil {
		t.Fatal(err)
	}
	if got := body(t, s, rel); got != "- parent\n  - child\n" {
		t.Fatalf("asking for a child got %q", got)
	}
}

func TestSplitAsChildNestsEvenWhenThereAreNoChildrenYet(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToday("onetwo")
	if _, err := s.SplitBlock(addrOf(t, s, rel, 0), "one", "two", true); err != nil {
		t.Fatal(err)
	}
	if got := body(t, s, rel); got != "- one\n  - two\n" {
		t.Fatalf("got %q", got)
	}
}
