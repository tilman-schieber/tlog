package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tilman-schieber/tlog/internal/app"
)

// The outliner is driven headlessly here: Update takes key messages and the
// assertions are made against what actually lands on disk. Every mutation the
// TUI performs goes through the core, so these are end-to-end tests of the
// whole write path.

func newModel(t *testing.T) *Model {
	t.Helper()
	svc, err := app.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := &Model{svc: svc, collapsed: map[string]bool{}}
	if err := m.load(svc.TodayRel()); err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 80, 24
	return m
}

func send(t *testing.T, m *Model, msgs ...tea.KeyMsg) {
	t.Helper()
	for _, msg := range msgs {
		m.Update(msg)
	}
	if m.errMsg != "" {
		t.Fatalf("unexpected error from the outliner: %s", m.errMsg)
	}
}

func k(typ tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: typ} }

func altKey(typ tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: typ, Alt: true} }

func typeText(t *testing.T, m *Model, s string) {
	t.Helper()
	for _, r := range s {
		if r == ' ' {
			send(t, m, tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
			continue
		}
		send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func onDisk(t *testing.T, m *Model) string {
	t.Helper()
	f, err := m.svc.Store.Read(m.doc.Rel)
	if err != nil {
		t.Fatal(err)
	}
	return string(f.Data)
}

func TestTypingWritesToDisk(t *testing.T) {
	m := newModel(t)
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "hello world")
	send(t, m, k(tea.KeyEsc))

	if got := onDisk(t, m); got != "- hello world\n" {
		t.Fatalf("got %q", got)
	}
	if m.mode != modeNormal {
		t.Fatal("esc did not leave insert mode")
	}
}

func TestEnterStartsANewBlock(t *testing.T) {
	m := newModel(t)
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "one")
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "two")
	send(t, m, k(tea.KeyEsc))

	if got := onDisk(t, m); got != "- one\n\n- two\n" {
		t.Fatalf("got %q", got)
	}
}

func TestAltEnterStaysInsideTheBlock(t *testing.T) {
	m := newModel(t)
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "first")
	send(t, m, altKey(tea.KeyEnter))
	typeText(t, m, "second")
	send(t, m, k(tea.KeyEsc))

	if got := onDisk(t, m); got != "- first\n  second\n" {
		t.Fatalf("got %q", got)
	}
	if len(m.rows) != 1 {
		t.Fatalf("expected one block, got %d", len(m.rows))
	}
}

func TestEnterSplitsAtTheCursor(t *testing.T) {
	m := newModel(t)
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "onetwo")
	for i := 0; i < 3; i++ {
		send(t, m, k(tea.KeyLeft))
	}
	send(t, m, k(tea.KeyEnter), k(tea.KeyEsc))

	if got := onDisk(t, m); got != "- one\n\n- two\n" {
		t.Fatalf("got %q", got)
	}
}

func TestTabIndentsAndShiftTabOutdents(t *testing.T) {
	m := newModel(t)
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "parent")
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "child")
	send(t, m, k(tea.KeyTab), k(tea.KeyEsc))

	if got := onDisk(t, m); got != "- parent\n  - child\n" {
		t.Fatalf("after indent: %q", got)
	}

	send(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if got := onDisk(t, m); got != "- parent\n\n- child\n" {
		t.Fatalf("after outdent: %q", got)
	}
}

func TestBackspaceAtStartMergesWithTheBlockAbove(t *testing.T) {
	m := newModel(t)
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "one")
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "two")
	send(t, m, k(tea.KeyHome), k(tea.KeyBackspace), k(tea.KeyEsc))

	if got := onDisk(t, m); got != "- onetwo\n" {
		t.Fatalf("got %q", got)
	}
}

func TestMergeRefusesWhenTheBlockHasChildren(t *testing.T) {
	m := newModel(t)
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "a")
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "b")
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "c")
	send(t, m, k(tea.KeyTab), k(tea.KeyEsc))
	// a / b / c indented under b.

	m.cur = 1
	m.startInsert(false)
	send(t, m, k(tea.KeyBackspace))
	if !strings.Contains(m.status, "outdent") {
		t.Fatalf("expected a refusal explaining why, got %q", m.status)
	}
	send(t, m, k(tea.KeyEsc))
	if got := onDisk(t, m); got != "- a\n\n- b\n  - c\n" {
		t.Fatalf("the tree was damaged: %q", got)
	}
}

func TestDeleteBlockRemovesTheSubtree(t *testing.T) {
	m := newModel(t)
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "keep")
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "drop")
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "drop child")
	send(t, m, k(tea.KeyTab), k(tea.KeyEsc))

	m.cur = 1
	typeText(t, m, "dd")

	if got := onDisk(t, m); got != "- keep\n" {
		t.Fatalf("got %q", got)
	}
}

func TestSpaceTogglesTheTaskCheckbox(t *testing.T) {
	m := newModel(t)
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "buy milk")
	send(t, m, k(tea.KeyEsc))

	send(t, m, tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	if got := onDisk(t, m); got != "- [ ] buy milk\n" {
		t.Fatalf("first toggle: %q", got)
	}
	send(t, m, tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	if got := onDisk(t, m); got != "- [x] buy milk\n" {
		t.Fatalf("second toggle: %q", got)
	}
}

func TestMoveBlockCarriesItsChildren(t *testing.T) {
	m := newModel(t)
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "a")
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "b")
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "b1")
	send(t, m, k(tea.KeyTab), k(tea.KeyEsc))

	m.cur = 1 // b
	send(t, m, altKey(tea.KeyUp))

	if got := onDisk(t, m); got != "- b\n  - b1\n\n- a\n" {
		t.Fatalf("got %q", got)
	}
}

func TestCollapseHidesChildrenWithoutTouchingTheFile(t *testing.T) {
	m := newModel(t)
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "parent")
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "child")
	send(t, m, k(tea.KeyTab), k(tea.KeyEsc))

	before := onDisk(t, m)
	m.cur = 0
	send(t, m, k(tea.KeyLeft))
	if len(m.rows) != 1 {
		t.Fatalf("collapse did not hide the child: %d rows", len(m.rows))
	}
	send(t, m, k(tea.KeyRight))
	if len(m.rows) != 2 {
		t.Fatalf("expand did not restore the child: %d rows", len(m.rows))
	}
	if onDisk(t, m) != before {
		t.Fatal("collapsing wrote to the file")
	}
}

func TestPageCompletionOffersLinkedAndExistingPages(t *testing.T) {
	m := newModel(t)
	if _, err := m.svc.AddToPage("Project Foo", "the page"); err != nil {
		t.Fatal(err)
	}
	if err := m.load(m.doc.Rel); err != nil {
		t.Fatal(err)
	}

	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "see [[Pro")
	if m.comp == nil || !m.comp.active {
		t.Fatal("completion did not open after [[")
	}
	if m.comp.items[0] != "Project Foo" {
		t.Fatalf("wrong candidate: %+v", m.comp.items)
	}

	send(t, m, k(tea.KeyTab))
	if m.comp != nil {
		t.Fatal("completion stayed open after accepting")
	}
	send(t, m, k(tea.KeyEsc))
	if got := onDisk(t, m); got != "- see [[Project Foo]]\n" {
		t.Fatalf("got %q", got)
	}
}

func TestCompletionClosesOnceTheLinkIsClosed(t *testing.T) {
	m := newModel(t)
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "[[X]]")
	if m.comp != nil && m.comp.active {
		t.Fatal("completion should close once ]] is typed")
	}
	send(t, m, k(tea.KeyEsc))
}

func TestFollowingALinkCreatesThePageAndBackNavigates(t *testing.T) {
	m := newModel(t)
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "about [[Rust]]")
	send(t, m, k(tea.KeyEsc))

	journal := m.doc.Rel
	typeText(t, m, "gf")
	if m.errMsg != "" {
		t.Fatal(m.errMsg)
	}
	if m.doc.Rel != "pages/Rust.md" {
		t.Fatalf("did not follow the link, still on %q", m.doc.Rel)
	}

	send(t, m, k(tea.KeyBackspace))
	if m.doc.Rel != journal {
		t.Fatalf("back did not return to the journal, on %q", m.doc.Rel)
	}
}

func TestBacklinksListsTheReferringBlock(t *testing.T) {
	m := newModel(t)
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "about [[Rust]]")
	send(t, m, k(tea.KeyEsc))

	typeText(t, m, "gf")
	m.rebuildGraph()
	typeText(t, m, "gb")

	if m.mode != modeBacklinks {
		t.Fatalf("backlinks did not open, status %q", m.status)
	}
	if len(m.pick.filtered) != 1 || !strings.Contains(m.pick.filtered[0].detail, "about") {
		t.Fatalf("wrong backlinks: %+v", m.pick.filtered)
	}
}

func TestAnExternalEditIsRefusedRatherThanClobbered(t *testing.T) {
	m := newModel(t)
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "mine")
	send(t, m, k(tea.KeyEsc))

	// Someone edits the file in nvim while the outliner has it open.
	if err := m.svc.Store.Write(m.doc.Rel, []byte("- theirs\n"), ""); err != nil {
		t.Fatal(err)
	}

	m.startInsert(true)
	typeText(t, m, " edited")
	m.commitInsert()

	if !m.stale {
		t.Fatal("the outliner did not notice the file had changed")
	}
	if !strings.Contains(m.errMsg, "reload") {
		t.Fatalf("unhelpful message: %q", m.errMsg)
	}
	if got := onDisk(t, m); got != "- theirs\n" {
		t.Fatalf("the external edit was clobbered: %q", got)
	}

	// R reloads and clears the condition.
	send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	if m.stale || m.rows[0].block.Text != "theirs" {
		t.Fatalf("reload failed: stale=%v text=%q", m.stale, m.rows[0].block.Text)
	}
}

func TestMovingBetweenDaysCreatesNoFiles(t *testing.T) {
	m := newModel(t)
	send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})

	files, err := m.svc.Store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("browsing wrote files: %v", files)
	}
}

func TestSearchFindsBlocksAcrossFiles(t *testing.T) {
	m := newModel(t)
	if _, err := m.svc.AddToPage("Notes", "use sqlite as a disposable cache"); err != nil {
		t.Fatal(err)
	}
	m.rebuildGraph()

	send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	typeText(t, m, "sqlite")
	if len(m.pick.filtered) != 1 {
		t.Fatalf("search results: %+v", m.pick.filtered)
	}
	send(t, m, k(tea.KeyEnter))
	if m.doc.Rel != "pages/Notes.md" {
		t.Fatalf("selecting a result did not navigate, on %q", m.doc.Rel)
	}
}

func TestViewRendersWithoutPanicking(t *testing.T) {
	m := newModel(t)
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "a block with [[a link]] and #atag")
	send(t, m, altKey(tea.KeyEnter))
	typeText(t, m, "```go")
	send(t, m, altKey(tea.KeyEnter))
	typeText(t, m, "x := 1")

	for _, mode := range []mode{modeInsert, modeNormal, modeHelp} {
		m.mode = mode
		if out := m.View(); out == "" {
			t.Fatalf("empty view in mode %v", mode)
		}
	}
	m.mode = modeNormal
	m.width, m.height = 20, 6
	if out := m.View(); out == "" {
		t.Fatal("empty view at a small size")
	}
}

func TestCompletionShowsItselfEvenWithNothingToSuggest(t *testing.T) {
	m := newModel(t) // a fresh notes directory: no pages at all
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "see [[")

	if m.comp == nil || !m.comp.active {
		t.Fatal("the popup must open even with no candidates, or the feature is invisible")
	}
	frame := m.View()
	if !strings.Contains(frame, "no pages yet") {
		t.Fatalf("no hint rendered:\n%s", frame)
	}

	// With nothing to offer, the popup must not swallow enter.
	send(t, m, k(tea.KeyEnter))
	if len(m.rows) != 2 {
		t.Fatalf("enter was eaten by an empty menu: %d rows", len(m.rows))
	}
}

func TestCompletionHintWhenNothingMatchesThePrefix(t *testing.T) {
	m := newModel(t)
	if _, err := m.svc.AddToPage("Project Foo", "x"); err != nil {
		t.Fatal(err)
	}
	if err := m.load(m.doc.Rel); err != nil {
		t.Fatal(err)
	}
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "[[Zzz")
	if !strings.Contains(m.View(), "no page by that name yet") {
		t.Fatalf("expected a hint for an unmatched prefix:\n%s", m.View())
	}
	// Tab must not be swallowed either; it should indent.
	send(t, m, k(tea.KeyTab))
	send(t, m, k(tea.KeyEsc))
}

func TestCompletionSelectsTheBestMatchWhileTyping(t *testing.T) {
	m := newModel(t)
	for _, p := range []string{"Project Foo", "Project Bar", "Prototype"} {
		if _, err := m.svc.AddToPage(p, "x"); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.load(m.doc.Rel); err != nil {
		t.Fatal(err)
	}

	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "[[Pro")
	if m.comp.sel != 0 {
		t.Fatalf("narrowing the query must re-select the best match, sel=%d in %v", m.comp.sel, m.comp.items)
	}

	// Once the arrow keys are used, the choice is deliberate and must survive
	// further typing as long as it still matches.
	chosen := m.comp.items[1]
	send(t, m, k(tea.KeyDown))
	typeText(t, m, "j")
	if m.currentCompletion() != chosen {
		t.Fatalf("a deliberate choice was lost: wanted %q, selected %q of %v", chosen, m.currentCompletion(), m.comp.items)
	}
	send(t, m, k(tea.KeyEsc))
}

func TestLinkedReferencesAppearOnThePage(t *testing.T) {
	m := newModel(t)
	if _, err := m.svc.AddToday("met [[Ada Lovelace]] about the plan"); err != nil {
		t.Fatal(err)
	}
	if err := m.load(m.doc.Rel); err != nil {
		t.Fatal(err)
	}
	typeText(t, m, "gf") // follow to the person's page

	if m.doc.Rel != "pages/Ada Lovelace.md" {
		t.Fatalf("on %q", m.doc.Rel)
	}
	frame := m.View()
	if !strings.Contains(frame, "1 linked references") {
		t.Fatalf("no linked references section:\n%s", frame)
	}
	if !strings.Contains(frame, "about the plan") {
		t.Fatalf("the mention is not shown:\n%s", frame)
	}
}

func TestLinkedReferencesShowTheirChildren(t *testing.T) {
	m := newModel(t)
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "[[Ada Lovelace]]")
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "agreed the plan")
	send(t, m, k(tea.KeyTab), k(tea.KeyEsc))

	typeText(t, m, "gg")
	typeText(t, m, "gf")
	frame := m.View()
	// The mention is a bare name with the substance underneath it; showing only
	// the parent would say nothing.
	if !strings.Contains(frame, "agreed the plan") {
		t.Fatalf("children of the referring block are missing:\n%s", frame)
	}
}

func TestReferenceRowsNavigateAndAreNotEditable(t *testing.T) {
	m := newModel(t)
	if _, err := m.svc.AddToday("met [[Ada Lovelace]] about the plan"); err != nil {
		t.Fatal(err)
	}
	if err := m.load(m.doc.Rel); err != nil {
		t.Fatal(err)
	}
	journal := m.doc.Rel
	typeText(t, m, "gf")

	// Walk down into the sections.
	before := onDisk(t, m)
	for m.rowKind() == rowBlock && m.cur < len(m.rows)-1 {
		send(t, m, k(tea.KeyDown))
	}
	if m.rowKind() == rowBlock {
		t.Fatal("never reached a section row")
	}
	// Editing keys must do nothing here: these rows live in another file.
	send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	send(t, m, k(tea.KeyTab))
	typeText(t, m, "dd")
	if onDisk(t, m) != before {
		t.Fatalf("a section row was edited as though it were a block:\n%s", onDisk(t, m))
	}

	// Walk to an actual reference and open it.
	for m.cur < len(m.rows)-1 && m.rows[m.cur].kind != rowRef {
		send(t, m, k(tea.KeyDown))
	}
	send(t, m, k(tea.KeyEnter))
	if m.doc.Rel != journal {
		t.Fatalf("enter on a reference did not jump to it, on %q", m.doc.Rel)
	}
}

func TestTagPageListsEveryPageTypedWithIt(t *testing.T) {
	m := newModel(t)
	if _, err := m.svc.AddToday("[[Ada Lovelace #person]] and [[Alan Turing #person]]"); err != nil {
		t.Fatal(err)
	}
	if err := m.load(m.doc.Rel); err != nil {
		t.Fatal(err)
	}
	rel, err := m.svc.OpenPage("person")
	if err != nil {
		t.Fatal(err)
	}
	m.goTo(rel)

	frame := m.View()
	if !strings.Contains(frame, "2 pages tagged #person") {
		t.Fatalf("tag page does not list its pages:\n%s", frame)
	}
	for _, want := range []string{"Ada Lovelace", "Alan Turing"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("%s missing from the tag page:\n%s", want, frame)
		}
	}
}

func TestPageHeaderShowsWhatThePageIs(t *testing.T) {
	m := newModel(t)
	if _, err := m.svc.AddToday("[[Ada Lovelace #person]]"); err != nil {
		t.Fatal(err)
	}
	if err := m.load(m.doc.Rel); err != nil {
		t.Fatal(err)
	}
	typeText(t, m, "gf")
	if !strings.Contains(m.View(), "#person") {
		t.Fatalf("the page does not show what it is:\n%s", m.View())
	}
}

func TestTagCompletionInsideBrackets(t *testing.T) {
	m := newModel(t)
	if _, err := m.svc.AddToday("[[Someone #person]] and [[Thing #project]]"); err != nil {
		t.Fatal(err)
	}
	if err := m.load(m.doc.Rel); err != nil {
		t.Fatal(err)
	}

	m.cur = len(m.rows) - 1
	for m.rowKind() != rowBlock && m.cur > 0 {
		m.cur--
	}
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "[[Ada #per")
	if m.comp == nil || !m.comp.active {
		t.Fatal("no completion after # inside the brackets")
	}
	if m.comp.items[0] != "person" {
		t.Fatalf("expected tag candidates, got %v", m.comp.items)
	}

	send(t, m, k(tea.KeyTab))
	// A tag is not closed off: you may want a second one.
	if got := m.ed.String(); got != "[[Ada #person" {
		t.Fatalf("accepted tag: %q", got)
	}
	typeText(t, m, "]]")
	send(t, m, k(tea.KeyEsc))
}

func TestQuotedBlockIsMarkedAsAQuotation(t *testing.T) {
	m := newModel(t)
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "> Dear all,")
	send(t, m, k(tea.KeyEsc))

	// Nothing is stored differently: a quote is a block whose text starts with >.
	if got := onDisk(t, m); got != "- > Dear all,\n" {
		t.Fatalf("got %q", got)
	}
	m.cur = 1 // off the quoted block, so it is not drawn as the selection
	m.cur = 0
	frame := m.View()
	if !strings.Contains(frame, "❝") {
		t.Fatalf("a quote should be marked as one:\n%s", frame)
	}
}

func TestFencedCodeIsHighlightedInTheOutliner(t *testing.T) {
	m := newModel(t)
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "build")
	send(t, m, altKey(tea.KeyEnter))
	typeText(t, m, "```go")
	send(t, m, altKey(tea.KeyEnter))
	typeText(t, m, "func main() { }")
	send(t, m, altKey(tea.KeyEnter))
	typeText(t, m, "```")
	send(t, m, k(tea.KeyEsc))

	if got := onDisk(t, m); !strings.Contains(got, "```go\n  func main() { }") {
		t.Fatalf("stored wrongly: %q", got)
	}
	m.cur = 1 // move the selection off the block so it is drawn, not inverted
	m.cur = 0
	m.mode = modeNormal
	frame := m.View()
	if !strings.Contains(frame, "func main()") {
		t.Fatalf("code missing from the frame:\n%s", frame)
	}
	if !strings.Contains(frame, "go") {
		t.Fatalf("language not labelled:\n%s", frame)
	}
}

func TestCSVFenceBecomesATableInTheOutliner(t *testing.T) {
	m := newModel(t)
	if _, err := m.svc.AddToday("Fachgebiete\n```csv\nKürzel,Fachgebiet\nBZ,Zahlentheorie\nCH,Topologie\n```"); err != nil {
		t.Fatal(err)
	}
	if err := m.load(m.doc.Rel); err != nil {
		t.Fatal(err)
	}
	m.cur = 1 // off the block, so it renders rather than inverting
	frame := m.View()

	for _, want := range []string{"Kürzel", "Fachgebiet", "BZ", "Zahlentheorie", "│", "┼"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("%q missing from the table:\n%s", want, frame)
		}
	}
	// A csv fence is ordinary markdown; nothing is stored differently.
	if got := onDisk(t, m); !strings.Contains(got, "```csv") {
		t.Fatalf("stored wrongly: %q", got)
	}
}
