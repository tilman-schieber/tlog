package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Clicks are answered from the last drawn frame, so every helper draws first,
// exactly as the terminal would have before anyone could aim at anything.

func withPage(t *testing.T, body string) *Model {
	t.Helper()
	m := newModel(t)
	abs := m.svc.Store.Abs(m.doc.Rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.load(m.doc.Rel); err != nil {
		t.Fatal(err)
	}
	m.View()
	return m
}

func mouseAt(t *testing.T, m *Model, x, y int, b tea.MouseButton) {
	t.Helper()
	m.Update(tea.MouseMsg{X: x, Y: y, Button: b, Action: tea.MouseActionPress})
	if m.errMsg != "" {
		t.Fatalf("unexpected error from the outliner: %s", m.errMsg)
	}
	m.View()
}

func click(t *testing.T, m *Model, x, y int) {
	t.Helper()
	mouseAt(t, m, x, y, tea.MouseButtonLeft)
}

func TestClickSelectsThenEditsAtThePointer(t *testing.T) {
	m := withPage(t, "- first\n\n- second\n")

	// Line 0 is the header; "second" is drawn on line 2, its text from column 2.
	click(t, m, 4, 2)
	if m.cur != 1 || m.mode != modeNormal {
		t.Fatalf("first click should only select: cur=%d mode=%d", m.cur, m.mode)
	}

	click(t, m, 4, 2) // between "se" and "cond"
	if m.mode != modeInsert {
		t.Fatal("second click on the selected block should start editing")
	}
	typeText(t, m, "X")
	send(t, m, k(tea.KeyEsc))
	if got := onDisk(t, m); got != "- first\n\n- seXcond\n" {
		t.Fatalf("got %q", got)
	}
}

func TestClickElsewhereSavesTheBlockBeingEdited(t *testing.T) {
	m := withPage(t, "- first\n\n- second\n")
	click(t, m, 4, 1)
	click(t, m, 20, 1) // past the end: caret at the end of the line
	typeText(t, m, "!")
	m.View()

	click(t, m, 4, 2)
	if m.mode != modeNormal || m.cur != 1 {
		t.Fatalf("click on another block should leave insert and select it: cur=%d mode=%d", m.cur, m.mode)
	}
	if got := onDisk(t, m); got != "- first!\n\n- second\n" {
		t.Fatalf("got %q", got)
	}
}

func TestClickOnALinkInTheSelectedBlockFollowsIt(t *testing.T) {
	m := withPage(t, "- top\n\n- see [[Ada]] today\n")
	from := m.doc.Rel

	click(t, m, 3, 2) // selects; links are not followed by a stray first click
	if m.doc.Rel != from {
		t.Fatal("the first click navigated away")
	}
	// "see " is four cells, so the link starts at column 2+4.
	click(t, m, 8, 2)
	if !strings.EqualFold(m.doc.Rel, "pages/Ada.md") {
		t.Fatalf("expected to be on Ada, got %s", m.doc.Rel)
	}
}

func TestClickOnTheBulletFolds(t *testing.T) {
	m := withPage(t, "- parent\n  - child\n\n- next\n")
	if len(m.rows) != 3 {
		t.Fatalf("rows: %d", len(m.rows))
	}
	click(t, m, 0, 1)
	if len(m.rows) != 2 {
		t.Fatalf("clicking ▾ should fold the children away, rows: %d", len(m.rows))
	}
	click(t, m, 0, 1)
	if len(m.rows) != 3 {
		t.Fatalf("clicking ▸ should unfold, rows: %d", len(m.rows))
	}
	if got := onDisk(t, m); got != "- parent\n  - child\n\n- next\n" {
		t.Fatalf("folding wrote to the file: %q", got)
	}
}

func TestClickOnTheCheckboxTicksIt(t *testing.T) {
	m := withPage(t, "- [ ] buy milk\n\n- other\n")
	click(t, m, 4, 2) // select something else, so the box is drawn as ☐
	click(t, m, 2, 1)
	if got := onDisk(t, m); got != "- [x] buy milk\n\n- other\n" {
		t.Fatalf("got %q", got)
	}
	if m.mode != modeNormal {
		t.Fatal("ticking a box should not open the editor")
	}
}

func TestWheelMovesTheSelection(t *testing.T) {
	m := withPage(t, "- a\n\n- b\n\n- c\n")
	mouseAt(t, m, 0, 0, tea.MouseButtonWheelDown)
	mouseAt(t, m, 0, 0, tea.MouseButtonWheelDown)
	if m.cur != 2 {
		t.Fatalf("cur=%d", m.cur)
	}
	mouseAt(t, m, 0, 0, tea.MouseButtonWheelUp)
	if m.cur != 1 {
		t.Fatalf("cur=%d", m.cur)
	}
}

func TestClickOnAnEmptyPageStartsWriting(t *testing.T) {
	m := newModel(t)
	m.View()
	click(t, m, 4, 1)
	if m.mode != modeInsert {
		t.Fatal("clicking the empty page should open a block")
	}
	typeText(t, m, "hi")
	send(t, m, k(tea.KeyEsc))
	if got := onDisk(t, m); got != "- hi\n" {
		t.Fatalf("got %q", got)
	}
}

func TestClickBetweenTheKeysOfDDDoesNotDelete(t *testing.T) {
	m := withPage(t, "- a\n\n- b\n")
	send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	click(t, m, 3, 1)
	send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if got := onDisk(t, m); got != "- a\n\n- b\n" {
		t.Fatalf("got %q", got)
	}
}

func TestCaretAtCountsCellsNotBytes(t *testing.T) {
	// 日 is two cells wide: a click on either cell lands before it.
	if got := caretAt("ab\n日x", 1, 1); got != 3 {
		t.Fatalf("got %d", got)
	}
	if got := caretAt("ab\n日x", 1, 2); got != 4 {
		t.Fatalf("got %d", got)
	}
	if got := caretAt("ab\n日x", 1, 99); got != 5 {
		t.Fatalf("got %d", got)
	}
}
