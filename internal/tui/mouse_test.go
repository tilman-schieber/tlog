package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// The mouse works from what the last frame drew, so every test here draws one
// first. Screen line 0 is the header; the first block is on line 1, and its
// text starts after "• ", at column 2.

func mouseAt(x, y int, action tea.MouseAction, button tea.MouseButton) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: action, Button: button}
}

func hoverAt(x, y int) tea.MouseMsg {
	return mouseAt(x, y, tea.MouseActionMotion, tea.MouseButtonNone)
}

func clickAt(x, y int) tea.MouseMsg {
	return mouseAt(x, y, tea.MouseActionPress, tea.MouseButtonLeft)
}

// opened records what was handed to the desktop, instead of opening it.
func opened(m *Model) *[]string {
	var got []string
	m.svc.Launch = func(target string) error { got = append(got, target); return nil }
	return &got
}

func TestALinkIsDrawnAsItsLabelAndHoverShowsTheAddress(t *testing.T) {
	m := treeModel(t, "- see [the docs](https://go.dev/doc) and ![plan](https://x.example/p.png)\n\n- below\n")
	send(t, m, k(tea.KeyDown)) // off the block, so it is drawn rather than edited

	frame := stripANSI(m.View())
	if strings.Contains(frame, "go.dev/doc") || !strings.Contains(frame, "see the docs and ▣ plan") {
		t.Fatalf("the link was not collapsed to its label:\n%s", frame)
	}

	// "• see " is six columns, so the label runs from column 6.
	m.Update(hoverAt(7, 1))
	if footer := stripANSI(m.View()); !strings.Contains(footer, "https://go.dev/doc") {
		t.Fatalf("hovering did not show where the link goes:\n%s", footer)
	}
	m.Update(hoverAt(2, 1)) // on "see", which is not a link
	if strings.Contains(stripANSI(m.View()), "https://go.dev/doc") {
		t.Fatal("the address stayed up after the pointer left the link")
	}
}

func TestClickingALinkOpensIt(t *testing.T) {
	m := treeModel(t, "- see [the docs](https://go.dev/doc)\n\n- below\n")
	got := opened(m)
	send(t, m, k(tea.KeyDown))
	m.View()

	m.Update(clickAt(8, 1))
	if len(*got) != 1 || (*got)[0] != "https://go.dev/doc" {
		t.Fatalf("opened %v", *got)
	}
	// Clicking a link is not clicking into the block.
	if m.cur != 1 {
		t.Fatalf("the cursor moved to row %d", m.cur)
	}
}

func TestClickingAPageLinkGoesThere(t *testing.T) {
	m := treeModel(t, "- met [[Ada Lovelace]] today\n\n- below\n")
	send(t, m, k(tea.KeyDown))
	m.View()
	m.Update(clickAt(10, 1)) // inside [[Ada Lovelace]]
	if m.doc.Rel != "pages/Ada Lovelace.md" {
		t.Fatalf("on %q", m.doc.Rel)
	}
}

func TestClickingABlockPutsTheCaretThere(t *testing.T) {
	m := treeModel(t, "- first block\n\n- second block\n")
	send(t, m, k(tea.KeyEsc)) // moving: a click highlights, it does not write
	m.View()

	m.Update(clickAt(4, 2)) // "se|cond"
	if m.cur != 1 || m.mode != modeMove || m.ed != nil {
		t.Fatalf("a click while moving should only highlight: row %d mode %v", m.cur, m.mode)
	}
	m.View()
	m.Update(clickAt(4, 2)) // the highlighted block again: write here
	if m.mode != modeOutline || m.ed == nil || m.ed.cur != 2 {
		t.Fatalf("a second click should start writing at the click: mode %v", m.mode)
	}
	typeText(t, m, "!")
	send(t, m, k(tea.KeyEsc))
	if got := onDisk(t, m); got != "- first block\n\n- se!cond block\n" {
		t.Fatalf("got %q", got)
	}
}

func TestClickingABulletFolds(t *testing.T) {
	m := treeModel(t, "- parent\n  - child\n\n- other\n")
	send(t, m, k(tea.KeyEsc))
	m.View()
	m.Update(clickAt(0, 1)) // the ▾ of "parent"
	if len(m.rows) != 2 {
		t.Fatalf("clicking the bullet did not fold: %d rows", len(m.rows))
	}
	m.View()
	m.Update(clickAt(0, 1))
	if len(m.rows) != 3 {
		t.Fatalf("clicking it again did not unfold: %d rows", len(m.rows))
	}
}

func TestTheWheelMoves(t *testing.T) {
	m := treeModel(t, "- a\n\n- b\n\n- c\n\n- d\n\n- e\n")
	send(t, m, k(tea.KeyEsc))
	m.Update(mouseAt(5, 3, tea.MouseActionPress, tea.MouseButtonWheelDown))
	if m.cur != 3 {
		t.Fatalf("on row %d", m.cur)
	}
	m.Update(mouseAt(5, 3, tea.MouseActionPress, tea.MouseButtonWheelUp))
	if m.cur != 0 {
		t.Fatalf("on row %d", m.cur)
	}
}

func TestClickingInThePaletteGoesThere(t *testing.T) {
	m := newModel(t)
	if _, err := m.svc.AddToPage("Alpha", "x"); err != nil {
		t.Fatal(err)
	}
	m.goTo("pages/Alpha.md")
	m.back()
	send(t, m, k(tea.KeyCtrlP))
	m.View()
	m.Update(clickAt(4, 2)) // the first item, under the title and the rule
	if m.doc.Rel != "pages/Alpha.md" {
		t.Fatalf("on %q", m.doc.Rel)
	}
}

// --- dropping files ------------------------------------------------------------

func dropped(t *testing.T, names ...string) []string {
	t.Helper()
	dir := t.TempDir()
	var paths []string
	for _, n := range names {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	return paths
}

func paste(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s), Paste: true}
}

func TestDroppingAFileWhileWritingPutsItsLinkAtTheCaret(t *testing.T) {
	paths := dropped(t, "plan.png", "my notes.pdf")
	m := newModel(t)
	typeText(t, m, "see")
	// What kitty types for a drop: the paths, shell-escaped.
	send(t, m, paste(paths[0]+" "+strings.ReplaceAll(paths[1], " ", `\ `)))
	send(t, m, k(tea.KeyEsc))

	got := onDisk(t, m)
	if got != "- see ![plan.png](../assets/plan.png) [my notes.pdf](../assets/my%20notes.pdf)\n" {
		t.Fatalf("got %q", got)
	}
	// The files are in the notes, not merely linked where they lay.
	assets := m.svc.AttachDir()
	for _, n := range []string{"plan.png", "my notes.pdf"} {
		if _, err := os.Stat(filepath.Join(assets, n)); err != nil {
			t.Fatalf("%s is not in assets/: %v", n, err)
		}
	}
	if strings.Contains(got, filepath.Dir(paths[0])) {
		t.Fatalf("the link points at where the file was dropped from: %q", got)
	}
}

func TestDroppingAFileWhileMovingMakesABlock(t *testing.T) {
	paths := dropped(t, "plan.png")
	m := treeModel(t, "- one\n\n- two\n")
	send(t, m, k(tea.KeyEsc))
	send(t, m, paste(paths[0]))
	got := onDisk(t, m)
	if got != "- one\n\n- ![plan.png](../assets/plan.png)\n\n- two\n" {
		t.Fatalf("got %q", got)
	}
}

func TestPastingTextIsNotADrop(t *testing.T) {
	m := newModel(t)
	send(t, m, paste("just some words, /etc/hostname among them"))
	send(t, m, k(tea.KeyEsc))
	if got := onDisk(t, m); got != "- just some words, /etc/hostname among them\n" {
		t.Fatalf("got %q", got)
	}
}

func TestGOpensAnOrdinaryLink(t *testing.T) {
	m := treeModel(t, "- read https://go.dev/doc tonight\n")
	got := opened(m)
	send(t, m, k(tea.KeyEsc), runeKey('g'))
	if len(*got) != 1 || (*got)[0] != "https://go.dev/doc" {
		t.Fatalf("opened %v", *got)
	}
}

func TestSlashFileCanPickAnyAttachedFile(t *testing.T) {
	paths := dropped(t, "alpha.pdf", "beta.pdf")
	m := newModel(t)
	if _, err := m.svc.Attach(paths...); err != nil {
		t.Fatal(err)
	}
	typeText(t, m, "/file ")
	if m.comp == nil || len(m.comp.files) != 2 {
		t.Fatalf("the shelf was not offered: %+v", m.comp)
	}
	second := m.comp.files[1].Name
	send(t, m, k(tea.KeyDown), k(tea.KeyEnter), k(tea.KeyEsc))
	if got := onDisk(t, m); !strings.Contains(got, "["+second+"]") {
		t.Fatalf("picking the second file did nothing: %q", got)
	}
}

func TestATagInALinkIsShownOnHover(t *testing.T) {
	m := treeModel(t, "- met [[Max Muster #Person]] today\n\n- below\n")
	send(t, m, k(tea.KeyDown))
	frame := stripANSI(m.View())
	if !strings.Contains(frame, "met [[Max Muster]] today") || strings.Contains(frame, "#Person") {
		t.Fatalf("the tag is still drawn:\n%s", frame)
	}
	m.Update(hoverAt(10, 1)) // inside [[Max Muster]]
	if footer := stripANSI(m.View()); !strings.Contains(footer, "[[Max Muster #Person]]") {
		t.Fatalf("hovering did not show the tag:\n%s", footer)
	}
	// Clicking still goes to Max, not to a page called "Max Muster #Person".
	m.Update(clickAt(10, 1))
	if m.doc.Rel != "pages/Max Muster.md" {
		t.Fatalf("on %q", m.doc.Rel)
	}
	// And in the block being edited it is all there, as it is in the file.
	m.back()
	if got := m.ed.String(); got != "met [[Max Muster #Person]] today" {
		t.Fatalf("editing shows %q", got)
	}
}

func TestADropNobodyBracketedIsStillADrop(t *testing.T) {
	paths := dropped(t, "plan.png")
	m := treeModel(t, "- one\n\n- two\n")
	send(t, m, k(tea.KeyEsc))
	// A terminal that does not bracket a paste delivers the path as one burst
	// of runes; typed by hand it would have come one rune at a time.
	send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(paths[0])})
	got := onDisk(t, m)
	if got != "- one\n\n- ![plan.png](../assets/plan.png)\n\n- two\n" {
		t.Fatalf("got %q", got)
	}
}

func TestABurstOfLettersIsTypedNotAttached(t *testing.T) {
	m := newModel(t)
	send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ab")})
	send(t, m, k(tea.KeyEsc))
	if got := onDisk(t, m); got != "- ab\n" {
		t.Fatalf("got %q", got)
	}
}

func TestClickingTheBoxTicksATask(t *testing.T) {
	m := treeModel(t, "- [ ] buy milk\n\n- other\n")
	send(t, m, k(tea.KeyDown)) // off the task, so its box is drawn rather than raw
	m.View()

	// Line 1 is the task; "• " is two columns, then the box.
	m.Update(clickAt(2, 1))
	if got := onDisk(t, m); !strings.HasPrefix(got, "- [x] buy milk") {
		t.Fatalf("the click did not tick the task: %q", got)
	}
	// A done task is drawn the same shape, so the same click unticks it.
	m.View()
	m.Update(clickAt(3, 1))
	if got := onDisk(t, m); !strings.HasPrefix(got, "- [ ] buy milk") {
		t.Fatalf("the second click did not untick it: %q", got)
	}

	// A click on the words is a click to write there, not a tick.
	m.View()
	m.Update(clickAt(8, 1))
	if got := onDisk(t, m); !strings.HasPrefix(got, "- [ ] buy milk") {
		t.Fatalf("clicking the text changed the task: %q", got)
	}
	if m.ed == nil || m.edBlock == nil || m.edBlock.Text != "[ ] buy milk" {
		t.Fatal("clicking the text did not start writing in the task")
	}
}

func TestTheBoxTicksWhileMovingToo(t *testing.T) {
	m := treeModel(t, "- [ ] buy milk\n")
	send(t, m, k(tea.KeyEsc))
	m.View()
	m.Update(clickAt(2, 1))
	if got := onDisk(t, m); got != "- [x] buy milk\n" {
		t.Fatalf("got %q", got)
	}
}

// --- headings ----------------------------------------------------------------

func TestAHeadingIsDrawnWithoutItsHashesUnlessBeingWritten(t *testing.T) {
	m := treeModel(t, "- # Plans for [[Alpha]]\n\n- below\n")
	send(t, m, k(tea.KeyDown))
	frame := stripANSI(m.View())
	if strings.Contains(frame, "# Plans") || !strings.Contains(frame, "• Plans for [[Alpha]]") {
		t.Fatalf("the heading kept its hashes:\n%s", frame)
	}
	// Its links are where they are drawn, not where the hashes would have
	// put them: "• Plans for " is twelve columns, so the link starts there.
	m.Update(clickAt(12, 1))
	if m.doc.Rel != "pages/Alpha.md" {
		t.Fatalf("clicking the link in a heading went to %q", m.doc.Rel)
	}

	m = treeModel(t, "- # Plans\n")
	if frame := stripANSI(m.View()); !strings.Contains(frame, "# Plans") {
		t.Fatalf("while writing, the hashes must be there to edit:\n%s", frame)
	}
	if got := onDisk(t, m); got != "- # Plans\n" {
		t.Fatalf("drawing changed the file: %q", got)
	}
}

func TestBoldSurvivesTheColoursInsideAHeading(t *testing.T) {
	in := "\x1b[1mfoo \x1b[34mbar\x1b[0m baz\x1b[0m"
	want := "\x1b[1mfoo \x1b[34mbar\x1b[0m\x1b[1m baz\x1b[0m"
	if got := rebold(in); got != want {
		t.Fatalf("got %q", got)
	}
	if got := rebold("plain"); got != "plain" {
		t.Fatalf("a terminal without styling gets its text back, got %q", got)
	}
}
