package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func attachFile(t *testing.T, m *Model, name, body string) {
	t.Helper()
	dir := filepath.Join(m.svc.Store.Root, "assets")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFollowingALinkToATextFileShowsIt(t *testing.T) {
	m := treeModel(t, "- rules: [Regeln.md](../assets/Regeln.md)\n\n- below\n")
	attachFile(t, m, "Regeln.md", "# Bewertungsregeln\n\nEine Note unter 4 ist ungenügend.\n")
	got := opened(m)
	send(t, m, k(tea.KeyEsc), runes("g"))

	if m.mode != modeFile {
		t.Fatalf("mode %v, status %q, error %q", m.mode, m.status, m.errMsg)
	}
	if len(*got) != 0 {
		t.Fatalf("a text file was handed to the desktop: %v", *got)
	}
	frame := stripANSI(m.View())
	for _, want := range []string{"Regeln.md", "# Bewertungsregeln", "Eine Note unter 4 ist ungenügend."} {
		if !strings.Contains(frame, want) {
			t.Fatalf("%q is not on screen:\n%s", want, frame)
		}
	}

	send(t, m, runes("o")) // still possible to open it outside
	if len(*got) != 1 || !strings.HasSuffix((*got)[0], "Regeln.md") {
		t.Fatalf("o opened %v", *got)
	}
	send(t, m, k(tea.KeyEsc))
	if m.mode != modeOutline || m.file != nil || m.doc.Rel == "" {
		t.Fatalf("esc did not go back: mode %v", m.mode)
	}
}

func TestALinkToAPictureStillGoesToTheDesktop(t *testing.T) {
	m := treeModel(t, "- ![plot](../assets/plot.png)\n\n- below\n")
	attachFile(t, m, "plot.png", "\x89PNG\r\n\x1a\n\x00\x00")
	got := opened(m)
	send(t, m, k(tea.KeyEsc), runes("g"))
	if m.mode == modeFile || len(*got) != 1 {
		t.Fatalf("mode %v, opened %v", m.mode, *got)
	}
}

func TestATextFileIsWrappedAndScrolls(t *testing.T) {
	m := treeModel(t, "- [log](../assets/log.txt)\n")
	var body strings.Builder
	for i := range 200 {
		body.WriteString(strings.Repeat("w", 150) + " line " + string(rune('a'+i%26)) + "\n")
	}
	attachFile(t, m, "log.txt", body.String())
	send(t, m, k(tea.KeyEsc), runes("g"))
	m.width, m.height = 80, 20
	for _, l := range strings.Split(stripANSI(m.View()), "\n") {
		if len([]rune(l)) > 80 {
			t.Fatalf("a line runs off the window: %d columns", len([]rune(l)))
		}
	}
	send(t, m, runes("G"))
	if !strings.Contains(stripANSI(m.View()), "100%") {
		t.Fatalf("G did not reach the end:\n%s", stripANSI(m.View()))
	}
}

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
