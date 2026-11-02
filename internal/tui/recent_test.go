package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tilman-schieber/tlog/internal/store"
)

// visit opens pages by name in order, the way following links would.
func visit(t *testing.T, m *Model, names ...string) map[string]string {
	t.Helper()
	rels := map[string]string{}
	for _, n := range names {
		rel, err := m.svc.OpenPage(n)
		if err != nil {
			t.Fatal(err)
		}
		m.goTo(rel)
		rels[n] = rel
	}
	m.View()
	return rels
}

func titles(m *Model) string {
	var out []string
	for _, r := range m.recent {
		out = append(out, strings.TrimSuffix(r[strings.LastIndex(r, "/")+1:], ".md"))
	}
	return strings.Join(out, " ")
}

var ctrlN = tea.KeyMsg{Type: tea.KeyCtrlN}

func TestRecentPagesAreNewestFirstAndCapped(t *testing.T) {
	m := newModel(t)
	visit(t, m, "A", "B", "C", "A", "D", "E", "F")
	// Four pages, then the journal day the outliner started on, then the
	// journals page, which is always there.
	today := store.Title(m.svc.TodayRel())
	if got := titles(m); got != "F E D A "+today+" journals" {
		t.Fatalf("recent: %s", got)
	}
	head := stripANSI(m.header())
	if !strings.HasPrefix(head, "F · E · D · A · "+today+" · Journals") {
		t.Fatalf("header: %q", head)
	}
}

func TestCyclingHoldsTheOrderUntilYouSettle(t *testing.T) {
	m := newModel(t)
	visit(t, m, "A", "B", "C") // recent: C B A journal

	send(t, m, ctrlN)
	if !strings.HasSuffix(m.doc.Rel, "B.md") {
		t.Fatalf("the first step should be the page before this one, on %s", m.doc.Rel)
	}
	send(t, m, ctrlN)
	if !strings.HasSuffix(m.doc.Rel, "A.md") {
		t.Fatalf("two steps from C should reach A, on %s", m.doc.Rel)
	}
	if got := titles(m); !strings.HasPrefix(got, "C B A") {
		t.Fatalf("the order moved while cycling: %s", got)
	}

	// Anything else ends the cycle, and where it stopped is now newest.
	send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if got := titles(m); !strings.HasPrefix(got, "A C B") {
		t.Fatalf("recent after settling: %s", got)
	}
	// So one press now goes back to C, like alt-tab.
	send(t, m, ctrlN)
	if !strings.HasSuffix(m.doc.Rel, "C.md") {
		t.Fatalf("on %s", m.doc.Rel)
	}
}

func TestCyclingWraps(t *testing.T) {
	m := newModel(t)
	visit(t, m, "A", "B") // recent: B A today Journals
	send(t, m, ctrlN, ctrlN, ctrlN, ctrlN)
	if !strings.HasSuffix(m.doc.Rel, "B.md") {
		t.Fatalf("four steps through four places should come back round, on %s", m.doc.Rel)
	}
}

func TestCyclingWhileTypingSavesFirst(t *testing.T) {
	m := newModel(t)
	rels := visit(t, m, "A")
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "kept")
	send(t, m, ctrlN)
	if m.mode != modeNormal || !strings.HasPrefix(m.doc.Rel, "journals/") {
		t.Fatalf("mode=%d on %s", m.mode, m.doc.Rel)
	}
	if got := fileOf(t, m, rels["A"]); got != "- kept\n" {
		t.Fatalf("got %q", got)
	}
}

func TestClickingARecentPageGoesThere(t *testing.T) {
	m := newModel(t)
	visit(t, m, "Alpha", "Beta")
	var x = -1
	for _, s := range m.headerHits {
		if strings.HasSuffix(s.rel, "Alpha.md") {
			x = s.start + 1
		}
	}
	if x < 0 {
		t.Fatalf("Alpha not in the header: %+v", m.headerHits)
	}
	click(t, m, x, 0)
	if !strings.HasSuffix(m.doc.Rel, "Alpha.md") {
		t.Fatalf("on %s", m.doc.Rel)
	}
	if got := titles(m); !strings.HasPrefix(got, "Alpha Beta") {
		t.Fatalf("recent: %s", got)
	}
}

func TestNarrowHeaderDropsTheOldestButNeverTheCurrent(t *testing.T) {
	m := newModel(t)
	visit(t, m, "Aaaaaaaaaa", "Bbbbbbbbbb", "Cccccccccc", "Dddddddddd")
	m.width = 40
	head := stripANSI(m.header())
	if !strings.HasPrefix(head, "Dddddddddd") || strings.Contains(head, "Aaaaaaaaaa") {
		t.Fatalf("header: %q", head)
	}
	if !strings.Contains(head, "NORMAL") {
		t.Fatalf("the mode was dropped: %q", head)
	}
}

func TestRecentPagesSurviveARestart(t *testing.T) {
	m := newModel(t)
	visit(t, m, "A", "B")

	again := &Model{svc: m.svc, collapsed: map[string]bool{}, width: 80, height: 24}
	again.recent = m.svc.Recent()
	if err := again.load(m.svc.TodayRel()); err != nil {
		t.Fatal(err)
	}
	if got := titles(again); !strings.HasSuffix(got, " B A journals") {
		t.Fatalf("recent after restart: %s", got)
	}
	send(t, again, ctrlN)
	if !strings.HasSuffix(again.doc.Rel, "B.md") {
		t.Fatalf("ctrl+n after a restart should reach the last page, on %s", again.doc.Rel)
	}
}
