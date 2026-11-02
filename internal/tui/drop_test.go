package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// A terminal hands a dropped file over as its path, pasted.
func paste(text string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text), Paste: true}
}

func screenshot(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "Screenshot 2026-11-02 at 17.03.png")
	if err := os.WriteFile(p, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDroppingAFileOnAFreshDayWritesItsLink(t *testing.T) {
	t.Setenv("ATT_DIR", t.TempDir())
	m := newModel(t)
	shot := screenshot(t)

	send(t, m, paste(strings.ReplaceAll(shot, " ", `\ `)))

	got := onDisk(t, m)
	if !strings.HasPrefix(got, "- ![Screenshot 2026-11-02 at 17.03.png](file://") {
		t.Fatalf("the blank bullet should hold the image link: %q", got)
	}
	if strings.Count(got, "\n- ") != 0 {
		t.Fatalf("an empty bullet was left behind: %q", got)
	}
	if !strings.Contains(m.status, "attached 1 file") {
		t.Fatalf("status: %q", m.status)
	}
	entries, _ := os.ReadDir(m.svc.AttachDir())
	if len(entries) != 1 {
		t.Fatalf("the shelf has %d files", len(entries))
	}
	if _, err := os.Stat(shot); err != nil {
		t.Fatal("the original was moved rather than copied")
	}
}

func TestDroppingAFileBelowABlockMakesItsOwn(t *testing.T) {
	t.Setenv("ATT_DIR", t.TempDir())
	m := newModel(t)
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "meeting")
	send(t, m, k(tea.KeyEsc))

	send(t, m, paste("'"+screenshot(t)+"'"))

	got := onDisk(t, m)
	if !strings.HasPrefix(got, "- meeting\n") || !strings.Contains(got, "\n- ![Screenshot") {
		t.Fatalf("got %q", got)
	}
}

func TestDroppingAFileWhileTypingPutsTheLinkAtTheCaret(t *testing.T) {
	t.Setenv("ATT_DIR", t.TempDir())
	m := newModel(t)
	send(t, m, k(tea.KeyEnter))
	typeText(t, m, "see ")
	send(t, m, paste(screenshot(t)))
	typeText(t, m, " here")
	send(t, m, k(tea.KeyEsc))

	got := onDisk(t, m)
	if !strings.HasPrefix(got, "- see ![Screenshot") || !strings.HasSuffix(got, ".png) here\n") {
		t.Fatalf("got %q", got)
	}
}

// Pasting text is still pasting text, even text that mentions a file.
func TestPastingTextIsNotADrop(t *testing.T) {
	t.Setenv("ATT_DIR", t.TempDir())
	m := newModel(t)
	shot := screenshot(t)
	send(t, m, k(tea.KeyEnter))
	send(t, m, paste("look at "+shot))
	send(t, m, k(tea.KeyEsc))

	if got := onDisk(t, m); got != "- look at "+shot+"\n" {
		t.Fatalf("got %q", got)
	}
	if entries, _ := os.ReadDir(m.svc.AttachDir()); len(entries) != 0 {
		t.Fatal("pasted text put a file on the shelf")
	}
}

// A terminal that does not bracket the paste still delivers the path in one
// burst, which no amount of typing does. Without this, in NORMAL mode the
// path's letters would have been read as commands.
func TestAnUnbracketedDropIsStillADrop(t *testing.T) {
	t.Setenv("ATT_DIR", t.TempDir())
	m := newModel(t)
	shot := screenshot(t)
	send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(strings.ReplaceAll(shot, " ", `\ `))})

	if got := onDisk(t, m); !strings.HasPrefix(got, "- ![Screenshot") {
		t.Fatalf("got %q", got)
	}
}

// The path macOS gives a screenshot dragged from its thumbnail, with the
// German name and the dots in the time.
func TestAScreenshotThumbnailDrop(t *testing.T) {
	t.Setenv("ATT_DIR", t.TempDir())
	m := newModel(t)
	dir := filepath.Join(t.TempDir(), "TemporaryItems", "NSIRD_screencaptureui_Bl2XwK")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	shot := filepath.Join(dir, "Bildschirmfoto 2026-11-02 um 17.38.34.jpg")
	if err := os.WriteFile(shot, []byte("jpg"), 0o644); err != nil {
		t.Fatal(err)
	}
	send(t, m, paste(shot))
	if got := onDisk(t, m); !strings.HasPrefix(got, "- ![Bildschirmfoto 2026-11-02 um 17.38.34.jpg](file://") {
		t.Fatalf("got %q", got)
	}
}
