package tui

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tilman-schieber/tlog/internal/clipboard"
	"github.com/tilman-schieber/tlog/internal/termimg"
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
	for _, want := range []string{"Regeln.md", "Bewertungsregeln", "Eine Note unter 4 ist ungenügend."} {
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
	got := opened(m)
	send(t, m, runes("o")) // any other text file can still go to the desktop
	if len(*got) != 1 || !strings.HasSuffix((*got)[0], "log.txt") {
		t.Fatalf("o opened %v", *got)
	}
}

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func pngBytes(t *testing.T) string {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 4, 3))); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestAPictureIsShownInATerminalThatCan(t *testing.T) {
	m := treeModel(t, "- ![plot](../assets/plot.png)\n\n- below\n")
	attachFile(t, m, "plot.png", pngBytes(t))
	m.images = termimg.Kitty
	got := opened(m)
	send(t, m, k(tea.KeyEsc))
	_, cmd := m.Update(runes("g"))
	if cmd == nil || len(*got) != 0 {
		t.Fatalf("the picture was not handed to the viewer: cmd %v, opened %v, error %q", cmd != nil, *got, m.errMsg)
	}

	// The viewer could not draw it after all: the opener, as before.
	m.Update(pictureClosedMsg{viewer: &termimg.Viewer{}, err: termimg.ErrCannotShow, rel: m.doc.Rel, target: "../assets/plot.png"})
	if len(*got) != 1 || !strings.HasSuffix((*got)[0], "plot.png") {
		t.Fatalf("no fallback to the opener: %v", *got)
	}
	// o in the viewer: open it outside.
	m.Update(pictureClosedMsg{viewer: &termimg.Viewer{Path: (*got)[0], Title: "plot.png", Opened: true}})
	if len(*got) != 2 {
		t.Fatalf("o did not open it: %v", *got)
	}
	// Any other key: back, nothing opened.
	m.Update(pictureClosedMsg{viewer: &termimg.Viewer{Protocol: termimg.Kitty}})
	if len(*got) != 2 || m.errMsg != "" {
		t.Fatalf("closing opened something: %v %q", *got, m.errMsg)
	}
}

func TestWhatTheTerminalSaidIsKept(t *testing.T) {
	m := treeModel(t, "- x\n")
	m.images = termimg.Ask
	m.Update(pictureClosedMsg{viewer: &termimg.Viewer{Protocol: termimg.Sixel}})
	if m.images != termimg.Sixel {
		t.Fatalf("still %v", m.images)
	}
}

func TestATextFileIsNotMistakenForAPicture(t *testing.T) {
	m := treeModel(t, "- [r](../assets/r.md)\n")
	attachFile(t, m, "r.md", "# R\n")
	m.images = termimg.Sixel
	send(t, m, k(tea.KeyEsc), runes("g"))
	if m.mode != modeFile {
		t.Fatalf("mode %v", m.mode)
	}
}

func clip(m *Model, c clipboard.Content) {
	m.svc.Clipboard = func() (clipboard.Content, error) { return c, nil }
}

func TestCtrlVPastesAScreenshotAtTheCaret(t *testing.T) {
	m := treeModel(t, "- see\n")
	clip(m, clipboard.Content{Image: []byte(pngBytes(t)), Type: "image/png"})
	send(t, m, k(tea.KeyEnd), k(tea.KeyCtrlV), k(tea.KeyEsc))
	body := readNote(t, m)
	if !regexp.MustCompile(`- see !\[screenshot-[0-9-]+\.png\]\(\.\./assets/screenshot-[0-9-]+\.png\)`).MatchString(body) {
		t.Fatalf("got %q", body)
	}
	entries, _ := os.ReadDir(filepath.Join(m.svc.Store.Root, "assets"))
	if len(entries) != 1 {
		t.Fatalf("assets: %v", entries)
	}
}

func TestCtrlVPastesTextAndCopiedFiles(t *testing.T) {
	m := treeModel(t, "- a\n")
	clip(m, clipboard.Content{Text: "hello\n"})
	send(t, m, k(tea.KeyEnd), k(tea.KeyCtrlV), k(tea.KeyEsc))
	if body := readNote(t, m); !strings.Contains(body, "- ahello") {
		t.Fatalf("text: %q", body)
	}

	src := filepath.Join(t.TempDir(), "my report.pdf")
	if err := os.WriteFile(src, []byte("%PDF"), 0o644); err != nil {
		t.Fatal(err)
	}
	clip(m, clipboard.Content{Text: "file://" + strings.ReplaceAll(src, " ", "%20") + "\r\n"})
	send(t, m, k(tea.KeyCtrlV)) // while moving: a block of its own
	if body := readNote(t, m); !strings.Contains(body, "[my report.pdf](../assets/my%20report.pdf)") {
		t.Fatalf("copied file: %q", body)
	}
}

func TestCtrlVSaysWhyNothingHappened(t *testing.T) {
	m := treeModel(t, "- a\n")
	m.svc.Clipboard = func() (clipboard.Content, error) { return clipboard.Content{}, clipboard.ErrEmpty }
	m.Update(k(tea.KeyCtrlV))
	if !strings.Contains(m.errMsg, "empty") {
		t.Fatalf("error %q", m.errMsg)
	}
}

func readNote(t *testing.T, m *Model) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(m.svc.Store.Root, m.doc.Rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSlashNoteLinksTheClipboardsTextAsAFile(t *testing.T) {
	m := newModel(t)
	m.svc.Clipboard = func() (clipboard.Content, error) {
		return clipboard.Content{Text: "Protokoll der Sitzung\n\nAnwesend: alle\n"}, nil
	}
	typeText(t, m, "Sitzung /note")
	if m.comp == nil || len(m.comp.cmds) == 0 || m.comp.cmds[0].Name != "note" {
		t.Fatal("/note was not offered")
	}
	send(t, m, k(tea.KeyEnter))
	send(t, m, k(tea.KeyEsc))
	if got := onDisk(t, m); !strings.Contains(got, "Sitzung [Protokoll der Sitzung](../assets/") {
		t.Fatalf("got %q", got)
	}
}

// clickText clicks the first screen cell of text in the current frame.
func clickText(t *testing.T, m *Model, text string) {
	t.Helper()
	for y, l := range strings.Split(stripANSI(m.View()), "\n") {
		if x := strings.Index(l, text); x >= 0 {
			m.Update(clickAt(len([]rune(l[:x])), y))
			return
		}
	}
	t.Fatalf("%q is not on screen:\n%s", text, stripANSI(m.View()))
}

func TestTheLinksInAMarkdownFileCanBeClicked(t *testing.T) {
	m := treeModel(t, "- [guide](../assets/guide.md)\n")
	attachFile(t, m, "guide.md", "# Guide\n\nSee [the rules](rules.md#grades), [[Alpha]],\n"+
		"[below](#the-end) and [the site](https://example.org).\n\n"+strings.Repeat("filler\n\n", 40)+"## The End\n\nbye\n\n"+strings.Repeat("more\n\n", 20))
	attachFile(t, m, "rules.md", "# Rules\n\n"+strings.Repeat("x\n\n", 40)+"## Grades\n\nA to F\n\n"+strings.Repeat("y\n\n", 20))
	got := opened(m)
	send(t, m, k(tea.KeyEsc), runes("g"))
	m.width, m.height = 80, 20
	if m.mode != modeFile {
		t.Fatalf("mode %v", m.mode)
	}

	// Hovering says where a link goes.
	for y, l := range strings.Split(stripANSI(m.View()), "\n") {
		if x := strings.Index(l, "the site"); x >= 0 {
			m.Update(mouseAt(len([]rune(l[:x])), y, tea.MouseActionMotion, tea.MouseButtonNone))
		}
	}
	if !strings.Contains(stripANSI(m.View()), "https://example.org  click to open") {
		t.Fatalf("no hover:\n%s", stripANSI(m.View()))
	}

	clickText(t, m, "the site")
	if len(*got) != 1 || (*got)[0] != "https://example.org" {
		t.Fatalf("opened %v", *got)
	}

	clickText(t, m, "below") // a #heading in the same file
	if frame := stripANSI(m.View()); !strings.Contains(strings.Split(frame, "\n")[1], "The End") {
		t.Fatalf("did not scroll to the heading:\n%s", frame)
	}

	send(t, m, runes("g"))
	clickText(t, m, "the rules") // another file, relative to this one, at a heading
	if m.mode != modeFile || m.file.file.Name != "rules.md" {
		t.Fatalf("showing %v", m.file)
	}
	if frame := stripANSI(m.View()); !strings.Contains(strings.Split(frame, "\n")[1], "Grades") ||
		!strings.Contains(frame, "esc back to guide.md") {
		t.Fatalf("not at the heading:\n%s", frame)
	}
	send(t, m, k(tea.KeyEsc))
	if m.mode != modeFile || m.file.file.Name != "guide.md" {
		t.Fatalf("esc did not come back to the guide: %v", m.mode)
	}

	clickText(t, m, "[[Alpha]]") // a page leaves for the outline
	if m.mode != modeOutline || m.doc.Rel != "pages/Alpha.md" {
		t.Fatalf("mode %v, at %q, error %q", m.mode, m.doc.Rel, m.errMsg)
	}
}

func TestLinksKeepTheirColumnsThroughListsQuotesAndTables(t *testing.T) {
	doc := "- item [a](x.md)\n\n> quoted [b](y.md)\n\n| k | v |\n|---|---|\n| 1 | [c](z.md) |\n"
	for _, l := range markdownLines(doc, 60) {
		plain := []rune(stripANSI(l.s))
		for _, h := range l.hits {
			label := string(plain[h.from:min(h.to, len(plain))])
			if !strings.HasPrefix(label, string(rune('a'+strings.Index("xyz", h.url[:1])))) {
				t.Fatalf("the link to %s lands on %q in %q", h.url, label, string(plain))
			}
		}
	}
}

func TestEHandsTheFileToTheEditorAndShowsWhatItWrote(t *testing.T) {
	m := treeModel(t, "- [r](../assets/r.md)\n")
	attachFile(t, m, "r.md", "# Before\n")
	send(t, m, k(tea.KeyEsc), runes("g"))
	m.width, m.height = 80, 20
	if _, cmd := m.Update(runes("e")); cmd == nil {
		t.Fatal("e did not start the editor")
	}

	// What the editor did, then its quitting.
	attachFile(t, m, "r.md", "# After\n\nedited **here**\n")
	m.Update(fileEditedMsg{})
	frame := stripANSI(m.View())
	if m.mode != modeFile || !strings.Contains(frame, "After") || !strings.Contains(frame, "edited here") {
		t.Fatalf("not re-read:\n%s", frame)
	}
	if m.status != "saved r.md" {
		t.Fatalf("status %q", m.status)
	}
}

func TestTheEditorIsTheOneTheEnvironmentNames(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "omarchy-launch-editor --inline")
	if got := editorCommand("/n/a.md").Args; strings.Join(got, " ") != "omarchy-launch-editor --inline /n/a.md" {
		t.Fatalf("got %q", got)
	}
	t.Setenv("VISUAL", "code --wait")
	if got := editorCommand("/n/a.md").Args; strings.Join(got, " ") != "code --wait /n/a.md" {
		t.Fatalf("got %q", got)
	}
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	if got := editorCommand("/n/a.md").Args; strings.Join(got, " ") != "vi /n/a.md" {
		t.Fatalf("got %q", got)
	}
}
