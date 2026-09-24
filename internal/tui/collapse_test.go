package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Collapse is remembered by position, so anything that renumbers siblings used
// to hand the fold to whichever block moved into that slot. These drive the
// operations that renumber and check the fold is still on the block it was put
// on — and, just as importantly, not on its neighbour.

func treeModel(t *testing.T, body string) *Model {
	t.Helper()
	m := newModel(t)
	if err := m.svc.Store.Write(m.doc.Rel, []byte(body), ""); err != nil {
		t.Fatal(err)
	}
	if err := m.load(m.doc.Rel); err != nil {
		t.Fatal(err)
	}
	return m
}

// folded returns the text of every block currently drawn as collapsed.
func folded(m *Model) []string {
	var out []string
	for _, r := range m.rows {
		if r.kind == rowBlock && m.collapsed[r.path] {
			out = append(out, r.block.Text)
		}
	}
	return out
}

func only(t *testing.T, m *Model, want string) {
	t.Helper()
	got := folded(m)
	if len(got) != 1 || got[0] != want {
		t.Fatalf("folded %v, wanted exactly [%s]", got, want)
	}
}

const threeTop = "- Alpha\n  - a1\n\n- Beta\n  - b1\n  - b2\n\n- Gamma\n"

func collapseBlock(t *testing.T, m *Model, row int) {
	t.Helper()
	m.cur = row
	send(t, m, k(tea.KeyLeft))
}

func TestAFoldFollowsItsBlockWhenItMoves(t *testing.T) {
	m := treeModel(t, threeTop)
	collapseBlock(t, m, 2) // Beta, whose children are now hidden
	only(t, m, "Beta")

	m.cur = 2
	send(t, m, altKey(tea.KeyUp)) // Beta above Alpha
	only(t, m, "Beta")

	send(t, m, altKey(tea.KeyDown)) // and back
	only(t, m, "Beta")
}

func TestDeletingABlockAboveDoesNotMoveTheFold(t *testing.T) {
	m := treeModel(t, threeTop)
	collapseBlock(t, m, 2)
	only(t, m, "Beta")

	m.cur = 0 // Alpha
	typeText(t, m, "dd")
	only(t, m, "Beta")
}

func TestIndentingCarriesTheFold(t *testing.T) {
	m := treeModel(t, threeTop)
	collapseBlock(t, m, 2)
	m.cur = 2
	send(t, m, k(tea.KeyTab)) // Beta becomes a child of Alpha
	only(t, m, "Beta")
}

func TestAFoldIsDroppedWhenItsBlockGoes(t *testing.T) {
	m := treeModel(t, threeTop)
	collapseBlock(t, m, 2)
	m.cur = 2
	typeText(t, m, "dd") // delete Beta itself
	if got := folded(m); len(got) != 0 {
		t.Fatalf("still folded: %v", got)
	}
	// And the entry is not left behind to be inherited later.
	if len(m.collapsed) != 0 {
		t.Fatalf("stale entries kept: %v", m.collapsed)
	}
}

func TestAFoldDoesNotLeakToAnotherPage(t *testing.T) {
	// The map is keyed by position alone, so without this "the second block"
	// was folded on every page at once.
	m := treeModel(t, threeTop)
	collapseBlock(t, m, 2)
	only(t, m, "Beta")

	if _, err := m.svc.AddToPage("Elsewhere", "one"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.svc.AppendBlock("pages/Elsewhere.md", "", "two"); err != nil {
		t.Fatal(err)
	}
	m.goTo("pages/Elsewhere.md")
	if got := folded(m); len(got) != 0 {
		t.Fatalf("the fold came along to another page: %v", got)
	}

	// Going back is a fresh arrival, which starts expanded rather than
	// remembering something that may no longer be true.
	m.goTo(m.svc.TodayRel())
	if got := folded(m); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestAnEditFromAnotherEditorDoesNotMoveTheFold(t *testing.T) {
	m := treeModel(t, threeTop)
	collapseBlock(t, m, 2)
	only(t, m, "Beta")

	// Someone inserts a block above Beta in nvim, renumbering everything after.
	externalWrite(t, m, m.doc.Rel, "- Alpha\n  - a1\n\n- Inserted\n\n- Beta\n  - b1\n  - b2\n\n- Gamma\n")
	m.Update(changed(t, m, m.doc.Rel))

	only(t, m, "Beta")
}

func TestTwoBlocksWithTheSameTextKeepTheirOwnFolds(t *testing.T) {
	// Matching is by text, so identical siblings have to pair up in order
	// rather than both matching the first one.
	m := treeModel(t, "- same\n  - x\n\n- same\n  - y\n\n- last\n")
	collapseBlock(t, m, 2) // the second "same"
	only(t, m, "same")
	first := m.rows[0]
	if m.collapsed[first.path] {
		t.Fatal("the fold was applied to both blocks with that text")
	}

	m.cur = 2
	send(t, m, altKey(tea.KeyUp))
	// Still exactly one fold, and the other "same" is still open.
	if got := folded(m); len(got) != 1 {
		t.Fatalf("folded %v", got)
	}
}

func TestEditingABlockDoesNotUnfoldItsChildren(t *testing.T) {
	// A block is matched on its own text, not its parent's, so changing a
	// parent costs that parent's fold and nothing below it.
	m := treeModel(t, "- parent\n  - mid\n    - leaf\n")
	m.cur = 1 // mid, which has a child
	send(t, m, k(tea.KeyLeft))
	only(t, m, "mid")

	m.cur = 0
	send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	typeText(t, m, " renamed")
	send(t, m, k(tea.KeyEsc))

	only(t, m, "mid")
}
