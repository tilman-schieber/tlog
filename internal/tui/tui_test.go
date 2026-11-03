package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	tapp "github.com/tilman-schieber/tlog/internal/app"
)

// The outliner is driven headlessly here: Update takes key messages and the
// assertions are made against what actually lands on disk. Every mutation the
// TUI performs goes through the core, so these are end-to-end tests of the
// whole write path.

func newModel(t *testing.T) *Model {
	t.Helper()
	svc, err := tapp.New(t.TempDir())
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

// startWriting picks the caret back up after esc, at the end of the block.
func startWriting(t *testing.T, m *Model) {
	t.Helper()
	send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if m.mode != modeOutline || m.ed == nil {
		t.Fatal("i did not start writing")
	}
}

// command runs something from the palette by name, the way a person would:
// ctrl+p, a few letters, enter.
func command(t *testing.T, m *Model, name string) {
	t.Helper()
	send(t, m, k(tea.KeyCtrlP))
	typeText(t, m, name)
	if it, ok := m.pick.selected(); !ok || it.run == nil {
		t.Fatalf("no command matching %q in the palette", name)
	}
	send(t, m, k(tea.KeyEnter))
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
	typeText(t, m, "hello world")
	send(t, m, k(tea.KeyEsc))

	if got := onDisk(t, m); got != "- hello world\n" {
		t.Fatalf("got %q", got)
	}
	if m.mode != modeMove || m.ed != nil {
		t.Fatal("esc did not put the caret down")
	}
}

func TestEnterStartsANewBlock(t *testing.T) {
	m := newModel(t)
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
	typeText(t, m, "a")
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "b")
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "c")
	send(t, m, k(tea.KeyTab), k(tea.KeyEsc))
	// a / b / c indented under b.

	m.cur = 1
	startWriting(t, m)
	send(t, m, k(tea.KeyHome), k(tea.KeyBackspace))
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
	typeText(t, m, "keep")
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "drop")
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "drop child")
	send(t, m, k(tea.KeyTab), k(tea.KeyEsc))

	m.cur = 1
	command(t, m, "delete block")

	if got := onDisk(t, m); got != "- keep\n" {
		t.Fatalf("got %q", got)
	}
}

func TestCtrlDTogglesTheTaskCheckbox(t *testing.T) {
	m := newModel(t)
	typeText(t, m, "buy milk")

	// Mid-sentence, unsaved: the text goes to disk with the checkbox, and the
	// caret stays after the word it was after.
	send(t, m, k(tea.KeyCtrlD))
	if got := onDisk(t, m); got != "- [ ] buy milk\n" {
		t.Fatalf("first toggle: %q", got)
	}
	typeText(t, m, "!")
	if got := m.ed.String(); got != "[ ] buy milk!" {
		t.Fatalf("the caret moved: %q", got)
	}
	send(t, m, k(tea.KeyBackspace))
	send(t, m, k(tea.KeyCtrlD))
	if got := onDisk(t, m); got != "- [x] buy milk\n" {
		t.Fatalf("second toggle: %q", got)
	}
}

func TestMoveBlockCarriesItsChildren(t *testing.T) {
	m := newModel(t)
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
	typeText(t, m, "parent")
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "child")
	send(t, m, k(tea.KeyTab), k(tea.KeyEsc))

	before := onDisk(t, m)
	m.cur = 0
	send(t, m, k(tea.KeyCtrlUp))
	if len(m.rows) != 1 {
		t.Fatalf("collapse did not hide the child: %d rows", len(m.rows))
	}
	send(t, m, k(tea.KeyCtrlDown))
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
	typeText(t, m, "[[X]]")
	if m.comp != nil && m.comp.active {
		t.Fatal("completion should close once ]] is typed")
	}
	send(t, m, k(tea.KeyEsc))
}

func TestFollowingALinkCreatesThePageAndBackNavigates(t *testing.T) {
	m := newModel(t)
	typeText(t, m, "about [[Rust]]")
	send(t, m, k(tea.KeyEsc))

	journal := m.doc.Rel
	send(t, m, k(tea.KeyCtrlG))
	if m.errMsg != "" {
		t.Fatal(m.errMsg)
	}
	if m.doc.Rel != "pages/Rust.md" {
		t.Fatalf("did not follow the link, still on %q", m.doc.Rel)
	}

	send(t, m, k(tea.KeyCtrlO))
	if m.doc.Rel != journal {
		t.Fatalf("back did not return to the journal, on %q", m.doc.Rel)
	}
}

func TestBacklinksListsTheReferringBlock(t *testing.T) {
	m := newModel(t)
	typeText(t, m, "about [[Rust]]")
	send(t, m, k(tea.KeyEsc))

	send(t, m, k(tea.KeyCtrlG))
	command(t, m, "backlinks")

	if m.mode != modeBacklinks {
		t.Fatalf("backlinks did not open, status %q", m.status)
	}
	if len(m.pick.filtered) != 1 || !strings.Contains(m.pick.filtered[0].detail, "about") {
		t.Fatalf("wrong backlinks: %+v", m.pick.filtered)
	}
}

func TestAnExternalEditIsRefusedRatherThanClobbered(t *testing.T) {
	m := newModel(t)
	typeText(t, m, "mine")
	send(t, m, k(tea.KeyEsc))

	// Someone edits the file in nvim while the outliner has it open.
	if err := m.svc.Store.Write(m.doc.Rel, []byte("- theirs\n"), ""); err != nil {
		t.Fatal(err)
	}

	startWriting(t, m)
	typeText(t, m, " edited")
	m.leave()

	if !m.stale {
		t.Fatal("the outliner did not notice the file had changed")
	}
	if !strings.Contains(m.errMsg, "reload") {
		t.Fatalf("unhelpful message: %q", m.errMsg)
	}
	if got := onDisk(t, m); got != "- theirs\n" {
		t.Fatalf("the external edit was clobbered: %q", got)
	}

	// ctrl+r reloads — but the edit it would throw away is still only in
	// memory, so the first press asks rather than discarding it silently.
	r := k(tea.KeyCtrlR)
	m.Update(r)
	if m.rows[0].block.Text == "theirs" {
		t.Fatal("ctrl+r discarded an unsaved edit without asking")
	}
	if !strings.Contains(m.status, "again") {
		t.Fatalf("ctrl+r gave no warning: %q", m.status)
	}

	m.Update(r)
	if m.stale || m.rows[0].block.Text != "theirs" {
		t.Fatalf("reload failed: stale=%v text=%q", m.stale, m.rows[0].block.Text)
	}
}

func TestMovingBetweenDaysCreatesNoFiles(t *testing.T) {
	m := newModel(t)
	send(t, m, altKey(tea.KeyLeft), altKey(tea.KeyLeft), altKey(tea.KeyRight))

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
	send(t, m, k(tea.KeyCtrlF))
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
	typeText(t, m, "a block with [[a link]] and #atag")
	send(t, m, altKey(tea.KeyEnter))
	typeText(t, m, "```go")
	send(t, m, altKey(tea.KeyEnter))
	typeText(t, m, "x := 1")

	for _, mode := range []mode{modeOutline, modeHelp} {
		m.mode = mode
		if out := m.View(); out == "" {
			t.Fatalf("empty view in mode %v", mode)
		}
	}
	m.mode = modeOutline
	m.width, m.height = 20, 6
	if out := m.View(); out == "" {
		t.Fatal("empty view at a small size")
	}
}

func TestCompletionShowsItselfEvenWithNothingToSuggest(t *testing.T) {
	m := newModel(t) // a fresh notes directory: no pages at all
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
	send(t, m, k(tea.KeyCtrlG)) // follow to the person's page

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
	typeText(t, m, "[[Ada Lovelace]]")
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "agreed the plan")
	send(t, m, k(tea.KeyTab), k(tea.KeyEsc))

	send(t, m, k(tea.KeyCtrlHome), k(tea.KeyCtrlG))
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
	send(t, m, k(tea.KeyCtrlG))

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
	send(t, m, k(tea.KeyCtrlG))
	if m.doc.Rel != "pages/Ada Lovelace.md" {
		t.Fatalf("on %q", m.doc.Rel)
	}
	// The tag is drawn where it describes something: in the header of the
	// page it is about, not in every line that mentions her.
	if header := stripANSI(strings.SplitN(m.View(), "\n", 2)[0]); !strings.Contains(header, "#person") {
		t.Fatalf("the page does not show what it is: %q", header)
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
	// A tag is not closed off: the caret stays inside the brackets, which
	// closed themselves when [[ was typed, so a second tag can follow.
	if got := m.ed.String(); got != "[[Ada #person]]" || m.ed.cur != len("[[Ada #person") {
		t.Fatalf("accepted tag: %q, caret %d", got, m.ed.cur)
	}
	// Typing the closing brackets out of habit steps over them.
	typeText(t, m, "]]")
	if got := m.ed.String(); got != "[[Ada #person]]" || m.ed.cur != len(got) {
		t.Fatalf("after ]]: %q, caret %d", got, m.ed.cur)
	}
	send(t, m, k(tea.KeyEsc))
}

func TestQuotedBlockIsMarkedAsAQuotation(t *testing.T) {
	m := newModel(t)
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
	m.mode = modeOutline
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
	if _, err := m.svc.AddToday("after the table"); err != nil {
		t.Fatal(err)
	}
	if err := m.load(m.doc.Rel); err != nil {
		t.Fatal(err)
	}
	// Off the block: the one under the caret shows what is typed, not a table.
	send(t, m, k(tea.KeyCtrlEnd))
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

func TestSlashTodoMakesATask(t *testing.T) {
	m := newModel(t)
	typeText(t, m, "Bericht schreiben /todo")
	if m.comp == nil || m.comp.cmds == nil {
		t.Fatal("the slash menu did not open")
	}
	if m.comp.cmds[0].Name != "todo" {
		t.Fatalf("wrong command offered: %+v", m.comp.cmds)
	}
	send(t, m, k(tea.KeyEnter))
	send(t, m, k(tea.KeyEsc))

	got := onDisk(t, m)
	if !strings.Contains(got, "- [ ] Bericht schreiben") {
		t.Fatalf("got %q", got)
	}
	if strings.Contains(got, "/todo") {
		t.Fatalf("the command survived into the file: %q", got)
	}
}

func TestSlashDeadlineWritesAProperty(t *testing.T) {
	m := newModel(t)
	typeText(t, m, "Bot testen /deadline morgen")
	if m.comp == nil || m.comp.cmds == nil || m.comp.cmds[0].Name != "deadline" {
		t.Fatalf("deadline not offered: %+v", m.comp)
	}
	// The menu shows what the shorthand resolved to, before anything is written.
	if !strings.Contains(m.comp.items[0], "tomorrow") {
		t.Fatalf("no preview of the date: %q", m.comp.items[0])
	}
	send(t, m, k(tea.KeyEnter))
	send(t, m, k(tea.KeyEsc))

	got := onDisk(t, m)
	if !strings.Contains(got, "Deadline:: ") {
		t.Fatalf("no deadline written: %q", got)
	}
	if strings.Contains(got, "/deadline") || strings.Contains(got, "morgen") {
		t.Fatalf("the shorthand survived: %q", got)
	}
	want := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	if !strings.Contains(got, want) {
		t.Fatalf("expected %s in %q", want, got)
	}
	// A date on a block that can never be ticked off is a date that never
	// leaves the agenda, so a deadline makes it a task.
	if !strings.Contains(got, "- [ ] Bot testen") {
		t.Fatalf("a deadline did not make it a task: %q", got)
	}
}

func TestASlashInProseIsNotACommand(t *testing.T) {
	m := newModel(t)
	typeText(t, m, "siehe http://x.test/pfad")
	if m.comp != nil && m.comp.cmds != nil {
		t.Fatal("a slash inside a URL opened the command menu")
	}
	typeText(t, m, " und/oder")
	if m.comp != nil && m.comp.cmds != nil {
		t.Fatal("a slash inside a word opened the command menu")
	}
	send(t, m, k(tea.KeyEsc))
	if got := onDisk(t, m); !strings.Contains(got, "und/oder") {
		t.Fatalf("the text was mangled: %q", got)
	}
}

func TestAnUnknownSlashWordIsLeftAlone(t *testing.T) {
	m := newModel(t)
	typeText(t, m, "/frobnicate")
	if m.comp != nil && m.comp.cmds != nil {
		t.Fatal("an unknown command should not hold the menu open")
	}
	send(t, m, k(tea.KeyEsc))
	if got := onDisk(t, m); !strings.Contains(got, "/frobnicate") {
		t.Fatalf("the text was eaten: %q", got)
	}
}

func TestSlashMenuCanBeDismissed(t *testing.T) {
	m := newModel(t)
	typeText(t, m, "x /todo")
	send(t, m, k(tea.KeyEsc)) // dismiss the menu
	if m.comp != nil {
		t.Fatal("esc should close the menu")
	}
	send(t, m, k(tea.KeyEsc)) // leave insert
	if got := onDisk(t, m); !strings.Contains(got, "/todo") {
		t.Fatalf("dismissing should leave the literal text: %q", got)
	}
}

func TestSlashFileOffersTheAttachments(t *testing.T) {
	src := filepath.Join(t.TempDir(), "Quartalsbericht.pdf")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := newModel(t)
	if _, err := m.svc.Attach(src); err != nil {
		t.Fatal(err)
	}

	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "siehe /file quartal")
	if m.comp == nil || m.comp.files == nil {
		t.Fatalf("the attachments were not offered: %+v", m.comp)
	}
	if !strings.Contains(m.comp.items[0], "Quartalsbericht.pdf") {
		t.Fatalf("wrong candidate: %q", m.comp.items[0])
	}

	send(t, m, k(tea.KeyEnter))
	send(t, m, k(tea.KeyEsc))

	got := onDisk(t, m)
	if !strings.Contains(got, "[Quartalsbericht.pdf](../assets/Quartalsbericht.pdf)") {
		t.Fatalf("no link written: %q", got)
	}
	if strings.Contains(got, "/file") {
		t.Fatalf("the command survived: %q", got)
	}
}

func TestSlashFileWithNoMatchSaysSo(t *testing.T) {
	m := newModel(t)
	typeText(t, m, "/file")
	// Nothing attached, so the command is offered but has nothing to insert.
	m.comp = &completion{active: true, cmds: []tapp.Command{{Name: "file", Arg: "attachment"}}, sel: 0, start: 0}
	m.runCommand()
	if m.errMsg == "" || !strings.Contains(m.errMsg, "tlog attach") {
		t.Fatalf("expected a message naming how to put one there, got %q", m.errMsg)
	}
}

func TestSettingsScreenTogglesAndPersists(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("TLOG_CONFIG", cfgPath)

	m := newModel(t)
	command(t, m, "settings")
	if m.mode != modeSettings {
		t.Fatal("the settings screen did not open")
	}

	// Walk to the sanitize toggle and turn it on.
	for m.settings.items[m.settings.sel].Key != "attachments.sanitize" {
		send(t, m, k(tea.KeyDown))
	}
	send(t, m, k(tea.KeyEnter))

	if !m.svc.Cfg.Attachments.Sanitize {
		t.Fatal("the setting did not take effect")
	}
	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("nothing was written: %v", err)
	}
	// And it is really in the file, not just in memory.
	data, _ := os.ReadFile(cfgPath)
	if !strings.Contains(string(data), "sanitize = true") {
		t.Fatalf("file says:\n%s", data)
	}

	send(t, m, k(tea.KeyEsc))
	if m.mode != modeOutline {
		t.Fatal("esc should leave the settings screen")
	}
}

func TestSettingsScreenEditsAValueAndRefusesNonsense(t *testing.T) {
	t.Setenv("TLOG_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	m := newModel(t)
	command(t, m, "settings")

	for m.settings.items[m.settings.sel].Key != "git.debounce" {
		send(t, m, k(tea.KeyDown))
	}
	send(t, m, k(tea.KeyEnter)) // start typing
	for i := 0; i < 3; i++ {
		send(t, m, k(tea.KeyBackspace))
	}
	typeText(t, m, "banana")
	send(t, m, k(tea.KeyEnter))

	if !strings.Contains(m.settings.msg, "not a delay") {
		t.Fatalf("nonsense should be refused with a reason: %q", m.settings.msg)
	}
	if m.svc.Cfg.Git.Debounce != "30s" {
		t.Fatalf("the bad value was kept: %q", m.svc.Cfg.Git.Debounce)
	}

	send(t, m, k(tea.KeyEnter))
	for i := 0; i < 3; i++ {
		send(t, m, k(tea.KeyBackspace))
	}
	typeText(t, m, "5s")
	send(t, m, k(tea.KeyEnter))
	if m.svc.Cfg.Git.Debounce != "5s" {
		t.Fatalf("got %q", m.svc.Cfg.Git.Debounce)
	}
}

func TestSettingsSayWhatWillNotTakeEffectYet(t *testing.T) {
	t.Setenv("TLOG_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	m := newModel(t)
	command(t, m, "settings")

	m.applySetting("notes", "~/zettel")
	if !strings.Contains(m.settings.msg, "next start") {
		t.Fatalf("a setting that needs a restart should say so: %q", m.settings.msg)
	}
	m.applySetting("format.blank_lines", "false")
	if !strings.Contains(m.settings.msg, "reformat") {
		t.Fatalf("a reformat should be announced: %q", m.settings.msg)
	}
}

func TestAnImpossibleMoveIsSaidQuietly(t *testing.T) {
	// Outdenting a top-level block is not a failure: nothing broke and nothing
	// was lost. It belongs in the status line, not in the error line.
	m := newModel(t)
	typeText(t, m, "top")
	send(t, m, k(tea.KeyEsc))
	before := onDisk(t, m)

	send(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.errMsg != "" {
		t.Fatalf("a move that does not exist was raised as an error: %q", m.errMsg)
	}
	if !strings.Contains(m.status, "top level") {
		t.Fatalf("it was not said at all: status %q", m.status)
	}
	if onDisk(t, m) != before {
		t.Fatal("a refused move wrote to the file")
	}
}

func TestEnterUnderCollapsedChildrenMakesASibling(t *testing.T) {
	// The core used to decide this from whether the block *had* children, so
	// the outliner and the app disagreed about a block whose children were
	// hidden. The adapter that can see the screen decides now.
	m := newModel(t)
	typeText(t, m, "parent")
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "child")
	send(t, m, k(tea.KeyTab), k(tea.KeyEsc))

	m.cur = 0
	send(t, m, k(tea.KeyCtrlUp)) // collapse: the child is off screen
	if len(m.rows) != 1 {
		t.Fatalf("collapse did not hide the child: %d rows", len(m.rows))
	}

	// Enter picks the caret up at the end of "parent"; the second makes the block.
	send(t, m, k(tea.KeyEnter), k(tea.KeyEnter))
	typeText(t, m, "next")
	send(t, m, k(tea.KeyEsc))

	if got := onDisk(t, m); got != "- parent\n  - child\n\n- next\n" {
		t.Fatalf("the new block went where it could not be seen: %q", got)
	}
}

func TestTabWhileTypingKeepsTheCaretWhereItWas(t *testing.T) {
	m := newModel(t)
	typeText(t, m, "parent")
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "child")
	send(t, m, k(tea.KeyLeft), k(tea.KeyLeft), k(tea.KeyTab))

	if m.ed == nil || m.edBlock != m.current() {
		t.Fatal("tab left the block")
	}
	if m.ed.cur != 3 {
		t.Fatalf("the caret moved: %d", m.ed.cur)
	}
	typeText(t, m, "!")
	send(t, m, k(tea.KeyEsc))
	if got := onDisk(t, m); got != "- parent\n  - chi!ld\n" {
		t.Fatalf("got %q", got)
	}
}

// --- noticing an edit made somewhere else ------------------------------------
//
// changedMsg is delivered straight to Update, so every branch is driven with no
// filesystem watcher, no goroutine and no clock.

func changed(t *testing.T, m *Model, rel string) changedMsg {
	t.Helper()
	f, err := m.svc.Store.Read(rel)
	if err != nil {
		t.Fatal(err)
	}
	return changedMsg{Rel: rel, Hash: f.Hash, Exists: len(f.Data) > 0}
}

func externalWrite(t *testing.T, m *Model, rel, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(m.svc.Store.Abs(rel)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.svc.Store.Abs(rel), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAnEditMadeElsewhereAppearsWithoutPressingR(t *testing.T) {
	m := newModel(t)
	typeText(t, m, "mine")
	send(t, m, k(tea.KeyEsc))

	externalWrite(t, m, m.doc.Rel, "- mine\n\n- from nvim\n")
	m.Update(changed(t, m, m.doc.Rel))

	if len(m.rows) != 2 || m.rows[1].block.Text != "from nvim" {
		t.Fatalf("the outliner did not pick it up: %d rows", len(m.rows))
	}
	if m.errMsg != "" {
		t.Fatalf("a reload is not an error: %q", m.errMsg)
	}
}

func TestTypingSurvivesAChangeUnderneath(t *testing.T) {
	m := newModel(t)
	typeText(t, m, "mine")
	send(t, m, k(tea.KeyEsc))

	// Mid-word, with the text not yet folded back into the block.
	startWriting(t, m)
	typeText(t, m, " half-typed")
	externalWrite(t, m, m.doc.Rel, "- theirs\n")
	m.Update(changed(t, m, m.doc.Rel))

	if m.ed.String() != "mine half-typed" {
		t.Fatalf("typing was discarded: %q", m.ed.String())
	}
	if !m.stale || m.status == "" {
		t.Fatalf("nothing was said about it: stale=%v status=%q", m.stale, m.status)
	}
}

func TestOurOwnWriteIsNotAnExternalEdit(t *testing.T) {
	// The loop has to terminate. Reloading on our own save would fight the
	// caret on every keystroke.
	m := newModel(t)
	typeText(t, m, "mine")
	send(t, m, k(tea.KeyEsc))

	before := m.doc
	m.Update(changed(t, m, m.doc.Rel))
	if m.doc != before {
		t.Fatal("the outliner reloaded on its own write")
	}
	if m.status == "reloaded — changed on disk" {
		t.Fatal("and said so")
	}
}

func TestAChangeToAnotherFileRefreshesTheSections(t *testing.T) {
	m := newModel(t)
	typeText(t, m, "a note")
	send(t, m, k(tea.KeyEsc))

	// Someone links to today's journal from a page tlog has never loaded.
	name := strings.TrimSuffix(filepath.Base(m.doc.Rel), ".md")
	externalWrite(t, m, "pages/Elsewhere.md", "- see [["+name+"]]\n")
	m.Update(changed(t, m, "pages/Elsewhere.md"))

	found := false
	for _, r := range m.rows {
		if r.kind == rowRef && strings.Contains(r.text, "see") {
			found = true
		}
	}
	if !found {
		t.Fatal("the new backlink did not appear")
	}
}

func TestAChangeElsewhereWaitsUntilYouStopTyping(t *testing.T) {
	m := newModel(t)
	typeText(t, m, "a note")
	send(t, m, k(tea.KeyEsc))
	rows := len(m.rows)

	startWriting(t, m)
	typeText(t, m, " and more") // mid-word: unsaved typing in the block
	name := strings.TrimSuffix(filepath.Base(m.doc.Rel), ".md")
	externalWrite(t, m, "pages/Elsewhere.md", "- see [["+name+"]]\n")
	m.Update(changed(t, m, "pages/Elsewhere.md"))

	if len(m.rows) != rows {
		t.Fatal("the screen moved under the caret mid-word")
	}
	if !m.graphDirty {
		t.Fatal("the refresh was dropped rather than deferred")
	}

	send(t, m, k(tea.KeyEsc))
	if len(m.rows) <= rows {
		t.Fatal("saving the block did not catch up")
	}
	if m.graphDirty {
		t.Fatal("still marked dirty after catching up")
	}
}

func TestSlashPropSetsAProperty(t *testing.T) {
	m := newModel(t)
	typeText(t, m, "the meeting /prop status offen")
	send(t, m, k(tea.KeyEnter)) // the menu is open; enter runs the command
	send(t, m, k(tea.KeyEsc))

	if got := onDisk(t, m); got != "- the meeting\n  status:: offen\n" {
		t.Fatalf("got %q", got)
	}
}

func TestBlockReferenceIsMadeWithDoubleParens(t *testing.T) {
	m := newModel(t)
	if _, err := m.svc.AddToPage("Timetable", "the lecture is at nine"); err != nil {
		t.Fatal(err)
	}
	if err := m.load(m.doc.Rel); err != nil {
		t.Fatal(err)
	}

	typeText(t, m, "see ((lecture")
	if m.comp == nil || len(m.comp.refs) != 1 {
		t.Fatalf("no block was offered: %+v", m.comp)
	}
	send(t, m, k(tea.KeyEnter), k(tea.KeyEsc))

	// An anchor was written on the target, and a link to it typed here.
	target, err := m.svc.Store.Read("pages/Timetable.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(target.Data), " ^") {
		t.Fatalf("no anchor on the target: %q", target.Data)
	}
	here := onDisk(t, m)
	if !strings.Contains(here, "see [[Timetable#^") || strings.Contains(here, "((") {
		t.Fatalf("got %q", here)
	}
}

func TestABlockReferenceShowsUpAsABacklink(t *testing.T) {
	m := newModel(t)
	if _, err := m.svc.AddToPage("Timetable", "the lecture is at nine"); err != nil {
		t.Fatal(err)
	}
	if err := m.load(m.doc.Rel); err != nil {
		t.Fatal(err)
	}
	typeText(t, m, "see ((lecture")
	send(t, m, k(tea.KeyEnter), k(tea.KeyEsc))

	// The whole point of a reference is that the other page knows about it.
	m.goTo("pages/Timetable.md")
	found := false
	for _, r := range m.rows {
		if r.kind == rowRef && strings.Contains(r.text, "see") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the reference is not visible from the other end: %+v", m.rows)
	}
}

func TestParensInProseAreNotAReference(t *testing.T) {
	m := newModel(t)
	typeText(t, m, "a function call f((x))")
	if m.comp != nil && m.comp.refs != nil {
		t.Fatal("closed parens opened the reference menu")
	}
	send(t, m, k(tea.KeyEsc))
	if got := onDisk(t, m); got != "- a function call f((x))\n" {
		t.Fatalf("got %q", got)
	}
}

func TestALinkByAliasOpensThePageItNames(t *testing.T) {
	m := newModel(t)
	if _, err := m.svc.AddToPage("Ada Lovelace", "the first programmer"); err != nil {
		t.Fatal(err)
	}
	if err := m.svc.Store.Write("pages/Ada Lovelace.md",
		[]byte("---\naliases: Ada\n---\n\n- the first programmer\n"), ""); err != nil {
		t.Fatal(err)
	}
	if err := m.load(m.doc.Rel); err != nil {
		t.Fatal(err)
	}

	typeText(t, m, "lunch with [[Ada]]")

	// ctrl+g follows the link in the block, saved or not.
	send(t, m, k(tea.KeyCtrlG))
	if m.doc.Rel != "pages/Ada Lovelace.md" {
		t.Fatalf("[[Ada]] went to %q", m.doc.Rel)
	}
	// And the mention is hers, listed under her real name.
	found := false
	for _, r := range m.rows {
		if r.kind == rowRef && strings.Contains(r.text, "lunch") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the aliased mention is not in her backlinks: %+v", m.rows)
	}
	// The header says what else she is called.
	if !strings.Contains(m.header(), "Ada") {
		t.Fatalf("the header does not mention the alias: %q", m.header())
	}
	files, _ := m.svc.Store.List()
	for _, f := range files {
		if f == "pages/Ada.md" {
			t.Fatal("following the alias created a second page")
		}
	}
}

// --- block references and embeds ---------------------------------------------

// refModel sets up a target block with a child on one page, a reference to it
// in today's journal, and returns the link text.
func refModel(t *testing.T) (*Model, string) {
	t.Helper()
	m := newModel(t)
	target, err := m.svc.AddToPage("Timetable", "the lecture is at nine")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.svc.AppendBlock(target, "", "room B103"); err != nil {
		t.Fatal(err)
	}
	d, _ := m.svc.Load(target)
	if _, err := m.svc.Indent(tapp.Addr{Rel: target, Offset: d.Doc.Flatten()[1].Start, Hash: d.Hash}); err != nil {
		t.Fatal(err)
	}
	d, _ = m.svc.Load(target)
	link, err := m.svc.RefTo(tapp.Addr{Rel: target, Offset: d.Doc.Flatten()[0].Start, Hash: d.Hash})
	if err != nil {
		t.Fatal(err)
	}
	return m, link
}

func TestAReferenceDrawsTheBlockItPointsAt(t *testing.T) {
	m, link := refModel(t)
	if _, err := m.svc.AddToday("remember: " + link); err != nil {
		t.Fatal(err)
	}
	if err := m.load(m.doc.Rel); err != nil {
		t.Fatal(err)
	}

	out := strings.Join(m.renderRow(m.rows[0], false, false), "\n")
	if !strings.Contains(stripANSI(out), "the lecture is at nine") {
		t.Fatalf("the reference did not draw the block: %q", stripANSI(out))
	}
	if strings.Contains(stripANSI(out), "#^") {
		t.Fatalf("the machinery is still on screen: %q", stripANSI(out))
	}
}

func TestTheBlockUnderTheCaretStillShowsTheReferenceItself(t *testing.T) {
	// What you type is what is stored, so editing must show the characters
	// that are actually in the file.
	m, link := refModel(t)
	if _, err := m.svc.AddToday("remember: " + link); err != nil {
		t.Fatal(err)
	}
	if err := m.load(m.doc.Rel); err != nil {
		t.Fatal(err)
	}
	m.cur = 0
	m.attach(-1)

	out := stripANSI(strings.Join(m.renderRow(m.rows[0], true, true), "\n"))
	if !strings.Contains(out, "#^") {
		t.Fatalf("editing hid the syntax being edited: %q", out)
	}
}

func TestABlockThatIsOnlyAReferenceBringsTheSubtree(t *testing.T) {
	m, link := refModel(t)
	if _, err := m.svc.AddToPage("Notes", link); err != nil {
		t.Fatal(err)
	}
	m.goTo("pages/Notes.md")

	var embedded []string
	for _, r := range m.rows {
		if r.kind == rowEmbed {
			embedded = append(embedded, r.text)
		}
	}
	if len(embedded) != 1 || embedded[0] != "room B103" {
		t.Fatalf("the subtree did not come with it: %v", embedded)
	}
}

func TestAnEmbeddedRowIsNotEditable(t *testing.T) {
	// Those rows belong to another file. Editing one here would be editing a
	// file this page does not have open.
	m, link := refModel(t)
	if _, err := m.svc.AddToPage("Notes", link); err != nil {
		t.Fatal(err)
	}
	m.goTo("pages/Notes.md")

	for i, r := range m.rows {
		if r.kind != rowEmbed {
			continue
		}
		m.cur = i
		if m.current() != nil {
			t.Fatal("an embedded row is being treated as a block of this page")
		}
		before := onDisk(t, m)
		send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
		typeText(t, m, "xyz")
		send(t, m, k(tea.KeyEsc))
		if onDisk(t, m) != before {
			t.Fatal("typing on an embedded row changed this page")
		}
	}
}

func TestFollowingAnEmbeddedRowGoesToItsOwnPage(t *testing.T) {
	m, link := refModel(t)
	if _, err := m.svc.AddToPage("Notes", link); err != nil {
		t.Fatal(err)
	}
	m.goTo("pages/Notes.md")

	for i, r := range m.rows {
		if r.kind == rowEmbed {
			m.cur = i
			break
		}
	}
	if !m.activateRow() {
		t.Fatal("an embedded row did not follow")
	}
	if m.doc.Rel != "pages/Timetable.md" {
		t.Fatalf("went to %q", m.doc.Rel)
	}
}

func TestAReferenceToAVanishedBlockIsVisiblyBroken(t *testing.T) {
	m := newModel(t)
	if _, err := m.svc.AddToPage("Timetable", "something"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.svc.AddToday("see [[Timetable#^gone42]]"); err != nil {
		t.Fatal(err)
	}
	if err := m.load(m.doc.Rel); err != nil {
		t.Fatal(err)
	}
	out := stripANSI(strings.Join(m.renderRow(m.rows[0], false, false), "\n"))
	if !strings.Contains(out, "gone42") {
		t.Fatalf("a dangling reference was drawn as though it worked: %q", out)
	}
}

func TestTheWholeRoundTrip(t *testing.T) {
	// Type (( in one page, pick a block, and read it back as the block's text.
	m := newModel(t)
	if _, err := m.svc.AddToPage("Timetable", "the lecture is at nine"); err != nil {
		t.Fatal(err)
	}
	if err := m.load(m.doc.Rel); err != nil {
		t.Fatal(err)
	}
	typeText(t, m, "see ((lecture")
	send(t, m, k(tea.KeyEnter), k(tea.KeyEsc))

	if err := m.load(m.doc.Rel); err != nil {
		t.Fatal(err)
	}
	out := stripANSI(strings.Join(m.renderRow(m.rows[0], false, false), "\n"))
	if !strings.Contains(out, "see") || !strings.Contains(out, "the lecture is at nine") {
		t.Fatalf("got %q", out)
	}
	// And the file still holds the reference, not the referenced text.
	if !strings.Contains(onDisk(t, m), "#^") {
		t.Fatalf("the file lost the reference: %q", onDisk(t, m))
	}
}

// --- the agenda and the message log ------------------------------------------
//
// The window had both and the outliner had neither: knowing what was due meant
// leaving the outliner for `tlog due`, and a message was replaced by the next
// thing that happened with nothing to go back to.

func TestAgendaListsWhatIsDueAndGoesThere(t *testing.T) {
	m := newModel(t)
	rel, err := m.svc.AddToPage("Timetable", "book the room")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := m.svc.Load(rel)
	a := tapp.Addr{Rel: rel, Offset: d.Doc.Flatten()[0].Start, Hash: d.Hash}
	if _, err := m.svc.RunCommand(a, "deadline", "tomorrow", "book the room", 13, 13); err != nil {
		t.Fatal(err)
	}
	if err := m.load(m.doc.Rel); err != nil {
		t.Fatal(err)
	}

	command(t, m, "agenda")
	if m.mode != modeAgenda {
		t.Fatal("the palette did not open the agenda")
	}
	if len(m.agenda.items) != 1 || !strings.Contains(m.agenda.items[0].Text, "book the room") {
		t.Fatalf("got %+v", m.agenda.items)
	}
	out := stripANSI(m.agendaView())
	if !strings.Contains(out, "book the room") || !strings.Contains(out, "tomorrow") {
		t.Fatalf("agenda reads %q", out)
	}

	// Enter goes to the block itself.
	send(t, m, k(tea.KeyEnter))
	if m.mode != modeOutline {
		t.Fatal("enter did not leave the agenda")
	}
	if m.doc.Rel != rel {
		t.Fatalf("went to %q", m.doc.Rel)
	}
}

func TestAgendaTicksOffWithoutLeaving(t *testing.T) {
	m := newModel(t)
	rel, _ := m.svc.AddToPage("Timetable", "book the room")
	d, _ := m.svc.Load(rel)
	a := tapp.Addr{Rel: rel, Offset: d.Doc.Flatten()[0].Start, Hash: d.Hash}
	if _, err := m.svc.RunCommand(a, "deadline", "tomorrow", "book the room", 13, 13); err != nil {
		t.Fatal(err)
	}
	if err := m.load(m.doc.Rel); err != nil {
		t.Fatal(err)
	}

	command(t, m, "agenda")
	send(t, m, tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})

	f, err := m.svc.Store.Read(rel)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(f.Data), "- [x] book the room") {
		t.Fatalf("space did not tick it off: %q", f.Data)
	}
	// Done work leaves the agenda, which is what an agenda is for.
	if len(m.agenda.items) != 0 {
		t.Fatalf("still listed: %+v", m.agenda.items)
	}
	// And `a` brings it back.
	send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if len(m.agenda.items) != 1 {
		t.Fatalf("all did not include what is done: %+v", m.agenda.items)
	}
}

func TestMessagesAreKeptRatherThanReplaced(t *testing.T) {
	m := newModel(t)
	typeText(t, m, "top")
	send(t, m, k(tea.KeyEsc))

	// Two things worth saying, the second replacing the first on screen.
	send(t, m, tea.KeyMsg{Type: tea.KeyShiftTab}) // refused: already at the top
	first := m.status
	if !strings.Contains(first, "top level") {
		t.Fatalf("nothing was said: %q", first)
	}
	command(t, m, "delete block")
	if m.status == first {
		t.Fatal("the second message did not replace the first on screen")
	}

	command(t, m, "messages")
	if m.mode != modeLog {
		t.Fatal("the palette did not open the log")
	}
	out := stripANSI(m.logView())
	if !strings.Contains(out, "top level") {
		t.Fatalf("the replaced message is gone: %q", out)
	}
	if !strings.Contains(out, "git has the previous version") {
		t.Fatalf("the newer message is missing: %q", out)
	}
	// Newest first: what you came to read is the last thing said.
	if strings.Index(out, "git has") > strings.Index(out, "top level") {
		t.Fatalf("oldest first: %q", out)
	}
}

// --- one mode -----------------------------------------------------------------
//
// The block under the cursor is always the one being typed into. These pin the
// promises that makes: typing needs no key first, moving off a block keeps what
// was typed, and moving through one that was not touched writes nothing.

func TestMovingOffABlockKeepsWhatWasTyped(t *testing.T) {
	m := treeModel(t, "- one\n\n- two\n")
	typeText(t, m, "!")
	send(t, m, k(tea.KeyDown))
	if got := onDisk(t, m); got != "- one!\n\n- two\n" {
		t.Fatalf("got %q", got)
	}
	if m.edBlock != m.current() || m.ed.String() != "two" {
		t.Fatalf("the editor did not follow the cursor: %q", m.ed.String())
	}
}

func TestPassingThroughBlocksWritesNothing(t *testing.T) {
	m := newModel(t)
	send(t, m, k(tea.KeyDown), k(tea.KeyUp), k(tea.KeyEsc), k(tea.KeyLeft), k(tea.KeyRight))
	files, err := m.svc.Store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("looking at an empty day wrote files: %v", files)
	}
}

func TestArrowsRunOnIntoTheNextBlock(t *testing.T) {
	m := treeModel(t, "- one\n\n- two\n")
	// The caret starts at the end of "one"; right crosses into "two".
	send(t, m, k(tea.KeyRight))
	if m.cur != 1 || m.ed.cur != 0 {
		t.Fatalf("right did not cross into the next block: row %d caret %d", m.cur, m.ed.cur)
	}
	send(t, m, k(tea.KeyLeft))
	if m.cur != 0 || m.ed.cur != len("one") {
		t.Fatalf("left did not cross back to the end: row %d caret %d", m.cur, m.ed.cur)
	}
}

func TestBackspaceOnAnEmptyBlockRemovesIt(t *testing.T) {
	m := treeModel(t, "- one\n\n- two\n")
	send(t, m, k(tea.KeyEnter)) // a fresh empty block after "one"
	send(t, m, k(tea.KeyBackspace))
	if got := onDisk(t, m); got != "- one\n\n- two\n" {
		t.Fatalf("got %q", got)
	}
	if m.cur != 0 || m.ed.cur != len("one") {
		t.Fatalf("the caret should be back at the end of one: row %d caret %d", m.cur, m.ed.cur)
	}

	// The first block has nothing to join onto, so an empty one just goes.
	m = treeModel(t, "-\n\n- two\n")
	send(t, m, k(tea.KeyBackspace))
	if got := onDisk(t, m); got != "- two\n" {
		t.Fatalf("got %q", got)
	}
}

func TestOpeningSomethingOverThePageWritesTheBlockFirst(t *testing.T) {
	m := newModel(t)
	typeText(t, m, "half a thought")
	send(t, m, k(tea.KeyCtrlP))
	if got := onDisk(t, m); got != "- half a thought\n" {
		t.Fatalf("the palette opened over unsaved typing: %q", got)
	}
	send(t, m, k(tea.KeyEsc))
	if m.mode != modeOutline || m.ed == nil {
		t.Fatal("closing the palette did not put the editor back")
	}
}

func TestThePaletteOpensOnWhereYouHaveBeen(t *testing.T) {
	m := newModel(t)
	for _, p := range []string{"Alpha", "Beta", "Gamma"} {
		if _, err := m.svc.AddToPage(p, "x"); err != nil {
			t.Fatal(err)
		}
	}
	m.goTo("pages/Gamma.md")
	m.goTo("pages/Alpha.md")

	send(t, m, k(tea.KeyCtrlP))
	got := []string{m.pick.filtered[0].label, m.pick.filtered[1].label}
	if got[0] != "Gamma" || m.pick.filtered[0].detail != "recent" {
		t.Fatalf("the last page visited should come first, got %v", got)
	}
	// The page you are on is not somewhere to go.
	for _, it := range m.pick.filtered {
		if it.rel == "pages/Alpha.md" {
			t.Fatal("the current page is offered")
		}
	}
	send(t, m, k(tea.KeyEnter))
	if m.doc.Rel != "pages/Gamma.md" {
		t.Fatalf("ctrl+p enter did not go back: on %q", m.doc.Rel)
	}
}

func TestThePaletteMakesAPageThatDoesNotExist(t *testing.T) {
	m := newModel(t)
	send(t, m, k(tea.KeyCtrlP))
	typeText(t, m, "Zebra Crossing Notes")
	if len(m.pick.filtered) != 0 {
		t.Fatalf("expected nothing to match: %+v", m.pick.filtered)
	}
	send(t, m, k(tea.KeyEnter))
	if m.doc.Rel != "pages/Zebra Crossing Notes.md" {
		t.Fatalf("on %q", m.doc.Rel)
	}
	typeText(t, m, "first line")
	send(t, m, k(tea.KeyEsc))
	if got := onDisk(t, m); got != "- first line\n" {
		t.Fatalf("got %q", got)
	}
}

// --- moving -------------------------------------------------------------------
//
// Writing is where the outliner opens; esc puts the caret down and turns the
// letters into commands. These pin that the two never leak into each other.

func runeKey(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func TestEscSavesAndLettersThenMoveRatherThanType(t *testing.T) {
	m := treeModel(t, "- one\n\n- two\n\n- three\n")
	typeText(t, m, "!")
	send(t, m, k(tea.KeyEsc))
	if got := onDisk(t, m); got != "- one!\n\n- two\n\n- three\n" {
		t.Fatalf("esc did not save: %q", got)
	}

	send(t, m, runeKey('j'), runeKey('j'), runeKey('k'))
	if m.cur != 1 {
		t.Fatalf("j j k should land on the second block, on row %d", m.cur)
	}
	if got := onDisk(t, m); got != "- one!\n\n- two\n\n- three\n" {
		t.Fatalf("moving typed into the file: %q", got)
	}

	send(t, m, runeKey('x'))
	if got := onDisk(t, m); got != "- one!\n\n- [ ] two\n\n- three\n" {
		t.Fatalf("x did not make a task of the highlighted block: %q", got)
	}

	// Typed quickly, both d's can arrive as one key.
	send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("dd")})
	if got := onDisk(t, m); got != "- one!\n\n- three\n" {
		t.Fatalf("dd did not delete it: %q", got)
	}

	// Enter picks the caret back up, at the end of whatever is highlighted.
	send(t, m, k(tea.KeyEnter))
	if m.mode != modeOutline || m.ed == nil {
		t.Fatal("enter did not start writing")
	}
	typeText(t, m, "?")
	send(t, m, k(tea.KeyEsc))
	if got := onDisk(t, m); !strings.Contains(got, "- three?\n") {
		t.Fatalf("got %q", got)
	}
}

func TestSingleLettersGoPlaces(t *testing.T) {
	m := newModel(t)
	if _, err := m.svc.AddToPage("Ada Lovelace", "the first programmer"); err != nil {
		t.Fatal(err)
	}
	today := m.doc.Rel
	m.goTo("pages/Ada Lovelace.md")
	send(t, m, k(tea.KeyEsc))

	send(t, m, runeKey('t'))
	if m.doc.Rel != today {
		t.Fatalf("t did not go to today: %q", m.doc.Rel)
	}
	send(t, m, runeKey('b'))
	if m.doc.Rel != "pages/Ada Lovelace.md" {
		t.Fatalf("b did not go back: %q", m.doc.Rel)
	}
	send(t, m, runeKey('p'))
	if m.mode != modePalette || m.pick.filtered[0].rel != today {
		t.Fatalf("p did not open the palette on the recent page: %+v", m.pick)
	}
	send(t, m, k(tea.KeyEnter))
	if m.doc.Rel != today {
		t.Fatalf("p enter did not go there: %q", m.doc.Rel)
	}
	// Arriving somewhere by the palette is arriving to write.
	if m.mode != modeOutline {
		t.Fatal("the palette did not return to writing")
	}
}

// --- dates ----------------------------------------------------------------------

func TestAtOffersDaysAndWritesTheDate(t *testing.T) {
	m := newModel(t) // nobody in the notes yet, so the days come first
	typeText(t, m, "call the printer @")
	if m.comp == nil || len(m.comp.mentions) == 0 || m.comp.mentions[0].Name != "today" {
		t.Fatalf("@ did not offer the usual days: %+v", m.comp)
	}
	typeText(t, m, "in 3")
	if len(m.comp.mentions) != 3 || m.comp.mentions[0].Name != "in 3 days" {
		t.Fatalf("@in 3 offered %+v", m.comp.mentions)
	}
	send(t, m, k(tea.KeyEnter))
	want := "[[" + time.Now().AddDate(0, 0, 3).Format("2006-01-02") + "]]"
	if got := m.ed.String(); got != "call the printer "+want {
		t.Fatalf("got %q", got)
	}
}

func TestAnAtThatIsNotADateIsJustText(t *testing.T) {
	m := newModel(t)
	typeText(t, m, "mail a@b.ch and @zzz")
	if m.comp != nil && m.comp.active {
		t.Fatalf("a menu opened: %+v", m.comp)
	}
	send(t, m, k(tea.KeyEnter)) // a new block, not an accepted suggestion
	send(t, m, k(tea.KeyEsc))
	if got := onDisk(t, m); !strings.HasPrefix(got, "- mail a@b.ch and @zzz\n") {
		t.Fatalf("got %q", got)
	}
}

func TestDeadlineOffersTheSameDays(t *testing.T) {
	m := newModel(t)
	typeText(t, m, "send the slides /deadline ")
	if m.comp == nil || m.comp.dates == nil {
		t.Fatalf("/deadline did not offer days: %+v", m.comp)
	}
	send(t, m, k(tea.KeyDown), k(tea.KeyEnter)) // tomorrow
	send(t, m, k(tea.KeyEsc))
	want := "Deadline:: " + time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	if got := onDisk(t, m); !strings.Contains(got, want) || strings.Contains(got, "/deadline") {
		t.Fatalf("got %q", got)
	}
}

func TestHelpScrollsAndOnlyClosesWhenAsked(t *testing.T) {
	m := newModel(t)
	m.width, m.height = 60, 15
	send(t, m, k(tea.KeyF1))
	if m.mode != modeHelp {
		t.Fatal("F1 did not open the help")
	}
	first := stripANSI(m.View())
	// A long path is wrapped, and broken where it has to be, so it is looked
	// for with the line breaks and the indentation taken out.
	squash := func(s string) string { return strings.Join(strings.Fields(s), "") }
	if !strings.Contains(squash(first), squash(tildeHome(m.svc.Store.Root))) {
		t.Fatalf("the help does not say where the notes are:\n%s", first)
	}
	for _, line := range strings.Split(first, "\n") {
		if w := lipgloss.Width(line); w > 60 {
			t.Fatalf("a line is %d wide in a 60-wide window: %q", w, line)
		}
	}

	send(t, m, k(tea.KeyPgDown), runeKey('j'), k(tea.KeyDown))
	if m.mode != modeHelp {
		t.Fatal("scrolling closed the help")
	}
	if stripANSI(m.View()) == first {
		t.Fatal("scrolling did not move the page")
	}
	send(t, m, k(tea.KeyEsc))
	if m.mode != modeOutline {
		t.Fatal("esc did not close the help")
	}
}

func TestWrapWordsBreaksWhatDoesNotFit(t *testing.T) {
	got := wrapWords("in /a/very/long/path/that/does/not/fit, committed", 12)
	for _, l := range got {
		if lipgloss.Width(l) > 12 {
			t.Fatalf("%q is wider than 12: %q", l, got)
		}
	}
	if strings.Join(got, "") != strings.ReplaceAll("in /a/very/long/path/that/does/not/fit, committed", " ", "") {
		t.Fatalf("something was lost: %q", got)
	}
}

// --- mentioning people -------------------------------------------------------

func TestAtFindsAPersonByFirstNameOrInitials(t *testing.T) {
	m := newModel(t)
	if err := m.svc.Store.Write("pages/Julia Rausenberger.md", []byte("---\ntags: Person\n---\n\n- x\n"), ""); err != nil {
		t.Fatal(err)
	}
	if err := m.load(m.doc.Rel); err != nil {
		t.Fatal(err)
	}
	typeText(t, m, "lunch with @jr")
	if m.comp == nil || len(m.comp.mentions) == 0 || m.comp.mentions[0].Name != "Julia Rausenberger" {
		t.Fatalf("@jr offered %+v", m.comp)
	}
	send(t, m, k(tea.KeyEnter))
	if got := m.ed.String(); got != "lunch with [[Julia Rausenberger]]" {
		t.Fatalf("got %q", got)
	}
}

func TestAtMakesANewPersonAndRemembersThem(t *testing.T) {
	m := newModel(t)
	typeText(t, m, "met @Maria Muster")
	last := m.comp.mentions[len(m.comp.mentions)-1]
	if last.Kind != "new" {
		t.Fatalf("no new person offered: %+v", m.comp.mentions)
	}
	m.comp.sel = len(m.comp.mentions) - 1
	send(t, m, k(tea.KeyEnter), k(tea.KeyEnter)) // accept, then a new block
	// The next @ already knows her: the save dropped the old snapshot.
	typeText(t, m, "@mar")
	if m.comp == nil || len(m.comp.mentions) == 0 || m.comp.mentions[0].Name != "Maria Muster" || m.comp.mentions[0].Kind != "person" {
		t.Fatalf("the new person was not offered next time: %+v", m.comp)
	}
	send(t, m, k(tea.KeyEsc), k(tea.KeyEsc))
	if got := onDisk(t, m); !strings.HasPrefix(got, "- met [[Maria Muster #person]]\n") {
		t.Fatalf("got %q", got)
	}
}

func TestDoubleBracketClosesItself(t *testing.T) {
	m := newModel(t)
	if _, err := m.svc.AddToPage("Project Foo", "x"); err != nil {
		t.Fatal(err)
	}
	if err := m.load(m.doc.Rel); err != nil {
		t.Fatal(err)
	}
	typeText(t, m, "see [[")
	if got := m.ed.String(); got != "see [[]]" || m.ed.cur != len("see [[") {
		t.Fatalf("[[ did not close itself: %q caret %d", got, m.ed.cur)
	}
	typeText(t, m, "proj")
	send(t, m, k(tea.KeyEnter)) // accept the completion: no second ]]
	typeText(t, m, " now")
	if got := m.ed.String(); got != "see [[Project Foo]] now" {
		t.Fatalf("got %q", got)
	}

	// Backspace inside an empty pair takes both halves.
	m2 := newModel(t)
	typeText(t, m2, "x [[")
	send(t, m2, k(tea.KeyBackspace), k(tea.KeyBackspace))
	if got := m2.ed.String(); got != "x " {
		t.Fatalf("got %q", got)
	}
}

func TestBracketsPairEvenWhenTypedFast(t *testing.T) {
	m := newModel(t)
	send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("see [[")})
	if got := m.ed.String(); got != "see [[]]" {
		t.Fatalf("got %q", got)
	}
	// A paste is taken as it is.
	m2 := newModel(t)
	send(t, m2, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[[x"), Paste: true})
	if got := m2.ed.String(); got != "[[x" {
		t.Fatalf("a paste was changed: %q", got)
	}
}

// --- undo ----------------------------------------------------------------------

func TestUndoTypingAWordAtATime(t *testing.T) {
	m := newModel(t)
	typeText(t, m, "hello world")
	send(t, m, k(tea.KeyCtrlZ))
	if got := m.ed.String(); got != "hello " {
		t.Fatalf("one undo should take back a word: %q", got)
	}
	send(t, m, k(tea.KeyCtrlZ), k(tea.KeyCtrlZ))
	if got := m.ed.String(); got != "" {
		t.Fatalf("got %q", got)
	}
	send(t, m, k(tea.KeyCtrlY))
	if got := m.ed.String(); got != "hello" {
		t.Fatalf("redo: %q", got)
	}
}

func TestUndoAnEditToABlockLeftBehind(t *testing.T) {
	m := treeModel(t, "- one\n\n- two\n")
	typeText(t, m, " more")
	send(t, m, k(tea.KeyDown)) // leaving the block writes it
	if got := onDisk(t, m); got != "- one more\n\n- two\n" {
		t.Fatalf("got %q", got)
	}
	send(t, m, k(tea.KeyCtrlZ)) // nothing typed in "two", so the file goes back
	if got := onDisk(t, m); got != "- one\n\n- two\n" {
		t.Fatalf("after undo: %q", got)
	}
	if !strings.Contains(m.status, "undone") {
		t.Fatalf("nothing was said: %q", m.status)
	}
	send(t, m, k(tea.KeyCtrlY))
	if got := onDisk(t, m); got != "- one more\n\n- two\n" {
		t.Fatalf("after redo: %q", got)
	}
}

func TestUndoAfterTabOrEnterKeepsTheTyping(t *testing.T) {
	m := newModel(t)
	typeText(t, m, "parent")
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "child")
	send(t, m, k(tea.KeyTab))
	if got := onDisk(t, m); got != "- parent\n  - child\n" {
		t.Fatalf("got %q", got)
	}
	send(t, m, k(tea.KeyCtrlZ)) // the indent, not the word
	if got := onDisk(t, m); got != "- parent\n\n- child\n" {
		t.Fatalf("after undoing the indent: %q", got)
	}
	send(t, m, k(tea.KeyCtrlZ)) // the word "child"
	send(t, m, k(tea.KeyCtrlZ)) // the new block, but "parent" stays
	if got := onDisk(t, m); got != "- parent\n" {
		t.Fatalf("after undoing the split: %q", got)
	}
}

func TestUndoGoesToThePageItChanges(t *testing.T) {
	m := newModel(t)
	if _, err := m.svc.AddToPage("Plans", "x"); err != nil {
		t.Fatal(err)
	}
	m.goTo("pages/Plans.md")
	typeText(t, m, "y")
	send(t, m, k(tea.KeyEsc)) // written
	m.goTo(m.svc.TodayRel())
	send(t, m, runeKey('u'))
	if m.doc.Rel != "pages/Plans.md" || onDisk(t, m) != "- x\n" {
		t.Fatalf("on %q with %q", m.doc.Rel, onDisk(t, m))
	}
	send(t, m, runeKey('U'))
	if got := onDisk(t, m); got != "- xy\n" {
		t.Fatalf("U did not redo: %q", got)
	}
}

func TestUndoRefusesToOverwriteAnEditMadeElsewhere(t *testing.T) {
	m := treeModel(t, "- one\n")
	typeText(t, m, "!")
	send(t, m, k(tea.KeyEsc))
	externalWrite(t, m, m.doc.Rel, "- theirs\n")
	m.Update(changed(t, m, m.doc.Rel))
	m.Update(runeKey('u'))
	if got := onDisk(t, m); got != "- theirs\n" {
		t.Fatalf("undo overwrote their edit: %q", got)
	}
	if !strings.Contains(m.errMsg, "changed elsewhere") {
		t.Fatalf("got %q", m.errMsg)
	}
}
