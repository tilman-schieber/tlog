package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/tilman-schieber/tlog/internal/markdown"
	"github.com/tilman-schieber/tlog/internal/store"
)

func newSvc(t *testing.T) *Service {
	t.Helper()
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func body(t *testing.T, s *Service, rel string) string {
	t.Helper()
	f, err := s.Store.Read(rel)
	if err != nil {
		t.Fatal(err)
	}
	return string(f.Data)
}

func TestAddCreatesAndAppends(t *testing.T) {
	s := newSvc(t)
	rel, err := s.AddToday("first thought")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddToday("second thought"); err != nil {
		t.Fatal(err)
	}
	got := body(t, s, rel)
	if got != "- first thought\n\n- second thought\n" {
		t.Fatalf("got %q", got)
	}
}

func TestAddKeepsMultilineTextInOneBlock(t *testing.T) {
	s := newSvc(t)
	rel, err := s.AddToday("run this\n```sh\necho hi\n```")
	if err != nil {
		t.Fatal(err)
	}
	got := body(t, s, rel)
	want := "- run this\n  ```sh\n  echo hi\n  ```\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	d, err := s.Load(rel)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Doc.Blocks) != 1 {
		t.Fatalf("a pasted snippet should be one block, got %d", len(d.Doc.Blocks))
	}
}

func TestAddToPageCreatesThePage(t *testing.T) {
	s := newSvc(t)
	rel, err := s.AddToPage("Project Foo", "implement the parser")
	if err != nil {
		t.Fatal(err)
	}
	if rel != "pages/Project Foo.md" {
		t.Fatalf("got %q", rel)
	}
	if !strings.Contains(body(t, s, rel), "implement the parser") {
		t.Fatal("content missing")
	}
}

func TestSaveRefusesToClobberAnExternalEdit(t *testing.T) {
	s := newSvc(t)
	rel, err := s.AddToday("mine")
	if err != nil {
		t.Fatal(err)
	}
	d, err := s.Load(rel)
	if err != nil {
		t.Fatal(err)
	}

	// nvim writes while the document is open.
	if err := s.Store.Write(rel, []byte("- theirs\n"), ""); err != nil {
		t.Fatal(err)
	}

	d.Doc.Blocks[0].Text = "mine, edited"
	err = s.Save(d)
	var conflict *store.ErrConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("expected a conflict, got %v", err)
	}
	if got := body(t, s, rel); got != "- theirs\n" {
		t.Fatalf("the external edit was destroyed: %q", got)
	}
}

func TestSaveOfUnchangedDocumentDoesNothing(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToday("x")
	d, _ := s.Load(rel)
	before := d.Hash
	if err := s.Save(d); err != nil {
		t.Fatal(err)
	}
	if d.Hash != before {
		t.Fatal("a no-op save rewrote the file")
	}
}

func TestOpenPageIsIdempotentAndPreservesCasing(t *testing.T) {
	s := newSvc(t)
	rel, err := s.OpenPage("Project Foo")
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.OpenPage("project foo")
	if err != nil {
		t.Fatal(err)
	}
	if again != rel {
		t.Fatalf("a differently-cased link made a second page: %q vs %q", again, rel)
	}
}

func TestAddrRoundTripAndResolve(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToday("find me")
	d, _ := s.Load(rel)
	a := Addr{Rel: rel, Offset: d.Doc.Blocks[0].Start, Hash: d.Hash}

	parsed, err := ParseAddr(a.String())
	if err != nil {
		t.Fatal(err)
	}
	if parsed != a {
		t.Fatalf("round trip: %+v vs %+v", parsed, a)
	}

	_, b, err := s.Resolve(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if b.Text != "find me" {
		t.Fatalf("resolved the wrong block: %q", b.Text)
	}
}

func TestResolveRefusesAStaleAddress(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToday("first")
	d, _ := s.Load(rel)
	a := Addr{Rel: rel, Offset: d.Doc.Blocks[0].Start, Hash: d.Hash}

	if _, err := s.AddToday("second"); err != nil {
		t.Fatal(err)
	}

	_, _, err := s.Resolve(a)
	var conflict *store.ErrConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("a stale address must not resolve, got %v", err)
	}
}

func TestParseAddrRejectsGarbage(t *testing.T) {
	for _, bad := range []string{"", "no-at-sign", "missing-offset@abc", "pages/A.md:notanumber@abc"} {
		if _, err := ParseAddr(bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestAnchorIsLazyAndWritesOnlyWhenAsked(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToday("worth naming")
	if strings.Contains(body(t, s, rel), "^") {
		t.Fatal("an anchor appeared without anyone asking for one")
	}

	d, _ := s.Load(rel)
	link, err := s.Anchor(Addr{Rel: rel, Offset: d.Doc.Blocks[0].Start, Hash: d.Hash})
	if err != nil {
		t.Fatal(err)
	}
	if link.Anchor == "" || link.Page == "" {
		t.Fatalf("bad link: %+v", link)
	}
	if !strings.Contains(body(t, s, rel), "^"+link.Anchor) {
		t.Fatalf("anchor not written: %q", body(t, s, rel))
	}

	// Asking again must return the same anchor rather than a second one.
	d2, _ := s.Load(rel)
	link2, err := s.Anchor(Addr{Rel: rel, Offset: d2.Doc.Blocks[0].Start, Hash: d2.Hash})
	if err != nil {
		t.Fatal(err)
	}
	if link2.Anchor != link.Anchor {
		t.Fatalf("anchor changed: %q then %q", link.Anchor, link2.Anchor)
	}
}

func TestGraphBacklinks(t *testing.T) {
	s := newSvc(t)
	if _, err := s.AddToday("talking about [[Project Foo]] today"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddToPage("Project Foo", "the page itself"); err != nil {
		t.Fatal(err)
	}

	g, err := s.Graph()
	if err != nil {
		t.Fatal(err)
	}
	refs := g.Backlinks("Project Foo")
	if len(refs) != 1 {
		t.Fatalf("backlinks: %d", len(refs))
	}
	if !strings.Contains(refs[0].Block.Text, "talking about") {
		t.Fatalf("wrong backlink: %q", refs[0].Block.Text)
	}
	if !refs[0].From.IsJournal {
		t.Fatal("backlink should come from the journal")
	}
}

func TestGraphSearchReturnsUsableAddresses(t *testing.T) {
	s := newSvc(t)
	if _, err := s.AddToday("use sqlite as a disposable cache"); err != nil {
		t.Fatal(err)
	}
	g, err := s.Graph()
	if err != nil {
		t.Fatal(err)
	}
	hits := g.Search("sqlite cache", 10)
	if len(hits) != 1 {
		t.Fatalf("hits: %d", len(hits))
	}

	// The whole point of the address: search, then mutate exactly that block.
	d, b, err := s.Resolve(Addr{Rel: hits[0].Page.Rel, Offset: hits[0].Offset, Hash: hits[0].Hash})
	if err != nil {
		t.Fatal(err)
	}
	b.Text = "use sqlite as a disposable cache, decided"
	if err := s.Save(d); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body(t, s, hits[0].Page.Rel), "decided") {
		t.Fatal("mutation by address did not land")
	}
}

func TestEmptyDocumentRendersToNothing(t *testing.T) {
	s := newSvc(t)
	d := &Doc{Rel: "pages/Blank.md", Doc: &markdown.Document{}, Hash: markdown.Hash(nil)}
	if err := s.Save(d); err != nil {
		t.Fatal(err)
	}
	if got := body(t, s, "pages/Blank.md"); got != "" {
		t.Fatalf("an empty document created a file: %q", got)
	}
}

// --- block operations -------------------------------------------------------

func addrOf(t *testing.T, s *Service, rel string, index int) Addr {
	t.Helper()
	d, err := s.Load(rel)
	if err != nil {
		t.Fatal(err)
	}
	flat := d.Doc.Flatten()
	if index >= len(flat) {
		t.Fatalf("block %d of %d", index, len(flat))
	}
	return Addr{Rel: rel, Offset: flat[index].Start, Hash: d.Hash}
}

func TestBlockOperationsRewriteTheFile(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToday("parent")
	if _, err := s.AppendBlock(rel, "", "child"); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Indent(addrOf(t, s, rel, 1)); err != nil {
		t.Fatal(err)
	}
	if got := body(t, s, rel); got != "- parent\n  - child\n" {
		t.Fatalf("indent: %q", got)
	}

	if _, err := s.Outdent(addrOf(t, s, rel, 1)); err != nil {
		t.Fatal(err)
	}
	if got := body(t, s, rel); got != "- parent\n\n- child\n" {
		t.Fatalf("outdent: %q", got)
	}

	if _, err := s.Move(addrOf(t, s, rel, 1), -1); err != nil {
		t.Fatal(err)
	}
	if got := body(t, s, rel); got != "- child\n\n- parent\n" {
		t.Fatalf("move: %q", got)
	}

	if _, err := s.DeleteBlock(addrOf(t, s, rel, 0)); err != nil {
		t.Fatal(err)
	}
	if got := body(t, s, rel); got != "- parent\n" {
		t.Fatalf("delete: %q", got)
	}
}

// A mutation returns where the block ended up, because rewriting the file moves
// every offset after it. Without this a caller cannot address the same block
// twice in a row.
func TestResultCarriesAUsableAddress(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToday("first")
	if _, err := s.AppendBlock(rel, "", "second"); err != nil {
		t.Fatal(err)
	}

	res, err := s.SetText(addrOf(t, s, rel, 1), "second, edited")
	if err != nil {
		t.Fatal(err)
	}
	// Use exactly what came back, with no reload in between.
	res2, err := s.ToggleTask(Addr{Rel: res.Rel, Offset: res.Offset, Hash: res.Hash})
	if err != nil {
		t.Fatal(err)
	}
	if got := body(t, s, rel); got != "- first\n\n- [ ] second, edited\n" {
		t.Fatalf("got %q", got)
	}
	if res2.Offset == 0 {
		t.Fatal("the second block cannot start at offset zero")
	}
}

func TestMutationsRefuseAStaleAddress(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToday("mine")
	a := addrOf(t, s, rel, 0)

	if err := s.Store.Write(rel, []byte("- theirs\n"), ""); err != nil {
		t.Fatal(err)
	}

	for name, fn := range map[string]func() error{
		"SetText":  func() error { _, err := s.SetText(a, "x"); return err },
		"Indent":   func() error { _, err := s.Indent(a); return err },
		"Delete":   func() error { _, err := s.DeleteBlock(a); return err },
		"Insert":   func() error { _, err := s.InsertAfter(a, "x", true); return err },
		"Toggle":   func() error { _, err := s.ToggleTask(a); return err },
		"Property": func() error { _, err := s.SetProperty(a, "k", "v"); return err },
	} {
		if err := fn(); err == nil {
			t.Fatalf("%s accepted a stale address", name)
		}
	}
	if got := body(t, s, rel); got != "- theirs\n" {
		t.Fatalf("the other writer's work was destroyed: %q", got)
	}
}

func TestAppendBlockHonoursTheHash(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToday("one")
	d, _ := s.Load(rel)
	stale := d.Hash

	if _, err := s.AppendBlock(rel, "", "two"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendBlock(rel, stale, "three"); err == nil {
		t.Fatal("a stale hash was accepted")
	}
}

func TestRefusedOperationsSayWhy(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToday("only")

	if _, err := s.Indent(addrOf(t, s, rel, 0)); err == nil || !strings.Contains(err.Error(), "nothing above") {
		t.Fatalf("indent: %v", err)
	}
	if _, err := s.Outdent(addrOf(t, s, rel, 0)); err == nil || !strings.Contains(err.Error(), "top level") {
		t.Fatalf("outdent: %v", err)
	}
	if _, err := s.Move(addrOf(t, s, rel, 0), 1); err == nil || !strings.Contains(err.Error(), "no sibling") {
		t.Fatalf("move: %v", err)
	}
}

func TestInsertAfterNestsUnderABlockWithChildren(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToday("parent")
	if _, err := s.AppendBlock(rel, "", "child"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Indent(addrOf(t, s, rel, 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.InsertAfter(addrOf(t, s, rel, 0), "new first child", true); err != nil {
		t.Fatal(err)
	}
	if got := body(t, s, rel); got != "- parent\n  - new first child\n  - child\n" {
		t.Fatalf("got %q", got)
	}
}

// --- the read model ---------------------------------------------------------

func TestViewDescribesThePage(t *testing.T) {
	s := newSvc(t)
	if _, err := s.AddToday("met [[Ada Lovelace #person]] about the plan"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddToPage("Ada Lovelace", "his page"); err != nil {
		t.Fatal(err)
	}

	v, err := s.ViewPage("Ada Lovelace")
	if err != nil {
		t.Fatal(err)
	}
	if v.Title != "Ada Lovelace" || v.Hash == "" {
		t.Fatalf("view: %+v", v)
	}
	if len(v.Tags) != 1 || v.Tags[0] != "person" {
		t.Fatalf("tags: %v", v.Tags)
	}
	if len(v.Blocks) != 1 || v.Blocks[0].Text != "his page" {
		t.Fatalf("blocks: %+v", v.Blocks)
	}
	if len(v.Refs) != 1 || !v.Refs[0].Head || !strings.Contains(v.Refs[0].Text, "the plan") {
		t.Fatalf("refs: %+v", v.Refs)
	}

	person, err := s.ViewPage("person")
	if err != nil {
		t.Fatal(err)
	}
	if len(person.Tagged) != 1 || person.Tagged[0] != "Ada Lovelace" {
		t.Fatalf("tagged: %v", person.Tagged)
	}
}

func TestViewCarriesTaskStateAndDepth(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToday("parent")
	if _, err := s.AppendBlock(rel, "", "[ ] a task"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Indent(addrOf(t, s, rel, 1)); err != nil {
		t.Fatal(err)
	}

	v, err := s.View(rel)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Blocks[0].HasChildren || v.Blocks[1].Depth != 1 {
		t.Fatalf("structure: %+v", v.Blocks)
	}
	if v.Blocks[1].Task != "open" {
		t.Fatalf("task: %q", v.Blocks[1].Task)
	}
}

func TestViewRefsIncludeChildren(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToday("[[Ada Lovelace]]")
	if _, err := s.AppendBlock(rel, "", "agreed the plan"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Indent(addrOf(t, s, rel, 1)); err != nil {
		t.Fatal(err)
	}

	v, err := s.ViewPage("Ada Lovelace")
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Refs) != 2 || v.Refs[1].Text != "agreed the plan" || v.Refs[1].Head {
		t.Fatalf("refs: %+v", v.Refs)
	}
}

func TestIndexAndCompletion(t *testing.T) {
	s := newSvc(t)
	if _, err := s.AddToday("[[Project Foo #project]]"); err != nil {
		t.Fatal(err)
	}
	idx, err := s.Index()
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Journals) != 1 {
		t.Fatalf("journals: %v", idx.Journals)
	}
	if len(idx.Tags) != 1 || idx.Tags[0] != "project" {
		t.Fatalf("tags: %v", idx.Tags)
	}

	pages, err := s.CompletePages("Pro", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) == 0 || pages[0] != "Project Foo" {
		t.Fatalf("page completion: %v", pages)
	}
	tags, err := s.CompleteTags("pro", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || tags[0] != "project" {
		t.Fatalf("tag completion: %v", tags)
	}
}

func TestSearchViaTheCore(t *testing.T) {
	s := newSvc(t)
	if _, err := s.AddToPage("Notes", "use sqlite as a disposable cache"); err != nil {
		t.Fatal(err)
	}
	hits, err := s.Search("sqlite cache", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits: %+v", hits)
	}
	// The hit must be directly usable as an address.
	if _, err := s.SetText(Addr{Rel: hits[0].Rel, Offset: hits[0].Offset, Hash: hits[0].Hash}, "decided"); err != nil {
		t.Fatal(err)
	}
}
