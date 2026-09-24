package main

import (
	"strings"
	"testing"
	"time"

	tapp "github.com/tilman-schieber/tlog/internal/app"
)

// The API is a bridge and nothing more, so these tests check the bridging:
// that an edit comes back with a page the frontend can draw and an offset it
// can address next, and that a stale hash is refused rather than applied.

func newAPI(t *testing.T) *API {
	t.Helper()
	svc, err := tapp.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &API{svc: svc}
}

func TestTodayStartsEmptyAndTakesText(t *testing.T) {
	a := newAPI(t)
	p, err := a.Today()
	if err != nil {
		t.Fatal(err)
	}
	if !p.IsJournal || len(p.Blocks) != 0 {
		t.Fatalf("a fresh journal should be empty: %+v", p)
	}

	e, err := a.AppendBlock(p.Rel, p.Hash, "first thought")
	if err != nil {
		t.Fatal(err)
	}
	if len(e.Page.Blocks) != 1 || e.Page.Blocks[0].Text != "first thought" {
		t.Fatalf("blocks: %+v", e.Page.Blocks)
	}
	if e.Page.Hash == p.Hash {
		t.Fatal("the hash must move when the file changes")
	}
}

// The frontend chains operations using only what came back from the last one.
// If the returned offset or hash were wrong, the second call would fail.
func TestEditsChainWithoutReloading(t *testing.T) {
	a := newAPI(t)
	p, _ := a.Today()

	e, err := a.AppendBlock(p.Rel, p.Hash, "parent")
	if err != nil {
		t.Fatal(err)
	}
	e, err = a.NewBlock(e.Page.Rel, e.Offset, e.Page.Hash, "child")
	if err != nil {
		t.Fatal(err)
	}
	e, err = a.Indent(e.Page.Rel, e.Offset, e.Page.Hash)
	if err != nil {
		t.Fatal(err)
	}
	e, err = a.SetText(e.Page.Rel, e.Offset, e.Page.Hash, "child, edited")
	if err != nil {
		t.Fatal(err)
	}
	e, err = a.ToggleTask(e.Page.Rel, e.Offset, e.Page.Hash)
	if err != nil {
		t.Fatal(err)
	}

	if len(e.Page.Blocks) != 2 {
		t.Fatalf("blocks: %+v", e.Page.Blocks)
	}
	b := e.Page.Blocks[1]
	if b.Depth != 1 || b.Task != "open" || !strings.Contains(b.Text, "child, edited") {
		t.Fatalf("second block: %+v", b)
	}
	if !e.Page.Blocks[0].HasChildren {
		t.Fatal("the parent should report children")
	}
}

func TestStaleHashIsRefused(t *testing.T) {
	a := newAPI(t)
	p, _ := a.Today()
	e, _ := a.AppendBlock(p.Rel, p.Hash, "mine")

	// Something else writes: nvim, the TUI, an agent.
	if err := a.svc.Store.Write(e.Page.Rel, []byte("- theirs\n"), ""); err != nil {
		t.Fatal(err)
	}

	if _, err := a.SetText(e.Page.Rel, e.Offset, e.Page.Hash, "clobbered"); err == nil {
		t.Fatal("a stale edit was accepted")
	}
	f, _ := a.svc.Store.Read(e.Page.Rel)
	if string(f.Data) != "- theirs\n" {
		t.Fatalf("the other writer's work was destroyed: %q", f.Data)
	}
}

func TestFollowingALinkAndTheTagPage(t *testing.T) {
	a := newAPI(t)
	p, _ := a.Today()
	if _, err := a.AppendBlock(p.Rel, p.Hash, "met [[Ada Lovelace #person]] about the plan"); err != nil {
		t.Fatal(err)
	}

	ada, err := a.OpenPage("Ada Lovelace")
	if err != nil {
		t.Fatal(err)
	}
	if len(ada.Tags) != 1 || ada.Tags[0] != "person" {
		t.Fatalf("tags: %v", ada.Tags)
	}
	if len(ada.Refs) != 1 || !strings.Contains(ada.Refs[0].Text, "the plan") {
		t.Fatalf("refs: %+v", ada.Refs)
	}

	person, err := a.OpenPage("person")
	if err != nil {
		t.Fatal(err)
	}
	if len(person.Tagged) != 1 || person.Tagged[0] != "Ada Lovelace" {
		t.Fatalf("tagged: %v", person.Tagged)
	}
}

func TestBrowsingDaysCreatesNothing(t *testing.T) {
	a := newAPI(t)
	p, _ := a.Today()
	for i := 0; i < 3; i++ {
		var err error
		p, err = a.Journal(p.Rel, -1)
		if err != nil {
			t.Fatal(err)
		}
	}
	files, err := a.svc.Store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("browsing wrote files: %v", files)
	}
}

func TestIndexSearchAndCompletion(t *testing.T) {
	a := newAPI(t)
	p, _ := a.Today()
	if _, err := a.AppendBlock(p.Rel, p.Hash, "use sqlite as a cache for [[Project Foo #project]]"); err != nil {
		t.Fatal(err)
	}

	idx, err := a.Index()
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Journals) != 1 || len(idx.Tags) != 1 {
		t.Fatalf("index: %+v", idx)
	}

	hits, err := a.Search("sqlite cache")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Offset != 0 && hits[0].Hash == "" {
		t.Fatalf("hits: %+v", hits)
	}

	pages, err := a.CompletePages("Pro")
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) == 0 || pages[0] != "Project Foo" {
		t.Fatalf("completion: %v", pages)
	}
	tags, err := a.CompleteTags("pro")
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || tags[0] != "project" {
		t.Fatalf("tag completion: %v", tags)
	}
}

func TestDeleteAndMove(t *testing.T) {
	a := newAPI(t)
	p, _ := a.Today()
	e, _ := a.AppendBlock(p.Rel, p.Hash, "one")
	e, _ = a.NewBlock(e.Page.Rel, e.Offset, e.Page.Hash, "two")

	e, err := a.Move(e.Page.Rel, e.Offset, e.Page.Hash, -1)
	if err != nil {
		t.Fatal(err)
	}
	if e.Page.Blocks[0].Text != "two" {
		t.Fatalf("move: %+v", e.Page.Blocks)
	}

	e, err = a.DeleteBlock(e.Page.Rel, e.Offset, e.Page.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if len(e.Page.Blocks) != 1 || e.Page.Blocks[0].Text != "one" {
		t.Fatalf("delete: %+v", e.Page.Blocks)
	}
}

// Clicking a journal in the sidebar, or following [[2026-09-17]], used to
// resolve into pages/ and show an empty file while creating junk.
func TestJournalsOpenByNameNotAsPages(t *testing.T) {
	a := newAPI(t)
	p, _ := a.Today()
	e, err := a.AppendBlock(p.Rel, p.Hash, "the real day")
	if err != nil {
		t.Fatal(err)
	}
	name := e.Page.Title

	got, err := a.OpenPage(name)
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsJournal || len(got.Blocks) != 1 || got.Blocks[0].Text != "the real day" {
		t.Fatalf("opening %q gave %q with %d blocks", name, got.Rel, len(got.Blocks))
	}

	files, err := a.svc.Store.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if !strings.HasPrefix(f, "journals/") {
			t.Fatalf("a junk page was created: %q", f)
		}
	}
}

// --- slash commands ---------------------------------------------------------

func TestCommandMenuAndDatePreview(t *testing.T) {
	a := newAPI(t)

	if len(a.Commands("")) == 0 {
		t.Fatal("a bare slash should offer the whole menu")
	}
	if got := a.Commands("dl"); len(got) == 0 || got[0].Name != "deadline" {
		t.Fatalf("alias not matched: %+v", got)
	}

	// The menu shows what a shorthand means before anything is written.
	p := a.DatePreview("morgen")
	if p.ISO != time.Now().AddDate(0, 0, 1).Format("2006-01-02") {
		t.Fatalf("preview: %+v", p)
	}
	if p.Label == "" || p.Short == "" {
		t.Fatalf("preview should be readable: %+v", p)
	}
	// Halfway through typing, there is simply no date yet.
	if got := a.DatePreview("mor"); got.ISO != "" {
		t.Fatalf("half-typed text should not resolve: %+v", got)
	}
}

func TestRunCommandFromTheApp(t *testing.T) {
	a := newAPI(t)
	p, _ := a.Today()
	e, err := a.AppendBlock(p.Rel, p.Hash, "Bot testen /deadline morgen")
	if err != nil {
		t.Fatal(err)
	}

	// The frontend passes the rune offsets of the "/name arg" run.
	text := "Bot testen /deadline morgen"
	e2, err := a.RunCommand(e.Page.Rel, e.Offset, e.Page.Hash,
		"deadline", "morgen", text, 11, len([]rune(text)))
	if err != nil {
		t.Fatal(err)
	}

	b := e2.Page.Blocks[0]
	if b.Due != time.Now().AddDate(0, 0, 1).Format("2006-01-02") {
		t.Fatalf("due: %+v", b)
	}
	if b.DueState == "" || b.DueLabel == "" {
		t.Fatalf("the app needs the resolved state to colour it: %+v", b)
	}
	if strings.Contains(b.Text, "/deadline") || strings.Contains(b.Text, "morgen") {
		t.Fatalf("the command survived: %q", b.Text)
	}
}

func TestDueListsWhatIsOpen(t *testing.T) {
	a := newAPI(t)
	p, _ := a.Today()

	e, _ := a.AppendBlock(p.Rel, p.Hash, "[ ] offen")
	e, _ = a.RunCommand(e.Page.Rel, e.Offset, e.Page.Hash, "deadline", "morgen", "[ ] offen", 9, 9)
	e2, _ := a.AppendBlock(e.Page.Rel, e.Page.Hash, "[x] erledigt")
	_, _ = a.RunCommand(e2.Page.Rel, e2.Offset, e2.Page.Hash, "deadline", "morgen", "[x] erledigt", 12, 12)

	open, err := a.Due(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 1 || open[0].Text != "offen" {
		t.Fatalf("open: %+v", open)
	}
	all, _ := a.Due(true)
	if len(all) != 2 {
		t.Fatalf("all: %+v", all)
	}
	// Finished work is never overdue, however long ago it was due.
	for _, it := range all {
		if it.Done && it.State != "" {
			t.Fatalf("a done item was given a state: %+v", it)
		}
	}
}

func TestMonthIsLaidOutMondayFirst(t *testing.T) {
	a := newAPI(t)
	cal := a.Month("2026-09-24")
	if cal.Title != "September 2026" {
		t.Fatalf("title: %q", cal.Title)
	}
	// September 2026 starts on a Tuesday, so exactly one blank leads it.
	if cal.Days[0].Day != 0 || cal.Days[1].Day != 1 {
		t.Fatalf("leading blanks wrong: %+v", cal.Days[:4])
	}
	var sel, days int
	for _, d := range cal.Days {
		if d.Day > 0 {
			days++
		}
		if d.Sel {
			sel = d.Day
		}
	}
	if days != 30 || sel != 24 {
		t.Fatalf("days=%d selected=%d", days, sel)
	}
	// An unparseable date still yields a usable month rather than nothing.
	if got := a.Month("nonsense"); len(got.Days) == 0 {
		t.Fatal("a bad date should still draw a calendar")
	}
}
