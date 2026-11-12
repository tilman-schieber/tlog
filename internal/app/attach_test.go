package app

import (
	"bytes"
	"image"
	"image/png"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tilman-schieber/tlog/internal/clipboard"
)

func attachSvc(t *testing.T) *Service {
	t.Helper()
	return newSvc(t)
}

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAPastedScreenshotLandsInAssetsAsAFile(t *testing.T) {
	s := attachSvc(t)
	at := time.Date(2026, 11, 3, 14, 5, 9, 0, time.Local)

	v, err := s.AttachImage(tinyPNG(t), at)
	if err != nil {
		t.Fatal(err)
	}
	if v.Name != "screenshot-2026-11-03-140509.png" {
		t.Fatalf("name: %q", v.Name)
	}
	if filepath.Dir(v.Path) != filepath.Join(s.Store.Root, "assets") || s.AttachDir() != filepath.Dir(v.Path) {
		t.Fatalf("not in the notes' assets/: %s", v.Path)
	}
	if _, err := os.Stat(v.Path); err != nil {
		t.Fatal(err)
	}
	if v.Link != "![screenshot-2026-11-03-140509.png](../assets/screenshot-2026-11-03-140509.png)" {
		t.Fatalf("an image should be linked as an embed: %s", v.Link)
	}

	// The same second again: a second file, never the first one overwritten.
	w, err := s.AttachImage(tinyPNG(t), at)
	if err != nil {
		t.Fatal(err)
	}
	if w.Path == v.Path || w.Name != "screenshot-2026-11-03-140509-2.png" {
		t.Fatalf("second paste: %+v", w)
	}
}

func TestWhatIsNotAPictureIsRefused(t *testing.T) {
	s := attachSvc(t)
	for name, data := range map[string][]byte{
		"nothing": nil,
		"text":    []byte("hello, this is not a picture"),
		"html":    []byte("<html><body>nope</body></html>"),
	} {
		if _, err := s.AttachImage(data, time.Now()); err == nil {
			t.Errorf("%s was accepted as a picture", name)
		}
	}
	entries, _ := os.ReadDir(s.AttachDir())
	if len(entries) != 0 {
		t.Fatalf("a refused paste left something on the shelf: %v", entries)
	}
}

func TestAnOversizedPictureIsRefused(t *testing.T) {
	s := attachSvc(t)
	big := append(tinyPNG(t), make([]byte, MaxImage)...)
	if _, err := s.AttachImage(big, time.Now()); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("got %v", err)
	}
}

// An attachment is part of the notes: it is committed with them, so it is
// pushed with them and is there on the next machine.
func TestAnAttachmentIsCommittedWithTheNote(t *testing.T) {
	s := attachSvc(t)
	if !s.Git.Enabled() {
		t.Skip("no git")
	}
	src := filepath.Join(t.TempDir(), "plan.pdf")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	rel := s.TodayRel()
	if _, err := s.AttachTo(rel, src); err != nil {
		t.Fatal(err)
	}
	if err := s.Commit(); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("git", "-C", s.Store.Root, "ls-files").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"assets/plan.pdf", filepath.ToSlash(rel)} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("%s is not committed; tracked:\n%s", want, out)
		}
	}
	// The link resolves from the note it is written in.
	if p, err := s.ResolveLink(rel, "../assets/plan.pdf"); err != nil || p != filepath.Join(s.Store.Root, "assets", "plan.pdf") {
		t.Fatalf("resolve: %q %v", p, err)
	}
}

// An attached markdown file is not a page.
func TestAnAttachedMarkdownFileIsNotAPage(t *testing.T) {
	s := attachSvc(t)
	src := filepath.Join(t.TempDir(), "minutes.md")
	if err := os.WriteFile(src, []byte("- hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Attach(src); err != nil {
		t.Fatal(err)
	}
	rels, err := s.Store.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rels {
		if strings.HasPrefix(filepath.ToSlash(r), "assets/") {
			t.Fatalf("listed as a note: %s", r)
		}
	}
}

func shelfFile(t *testing.T, shelf, name, body string) string {
	t.Helper()
	if err := os.MkdirAll(shelf, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(shelf, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func fileURL(p string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(p)}).String()
}

func TestMigrateShelfBringsLinkedFilesIntoTheNotes(t *testing.T) {
	s := attachSvc(t)
	shelf := filepath.Join(t.TempDir(), ".att", "store")
	pdf := shelfFile(t, shelf, "Projekt antrag.pdf", "pdf")
	png := shelfFile(t, shelf, "plan.png", "png")
	elsewhere := shelfFile(t, t.TempDir(), "mine.txt", "keep")

	text := "- [Antrag](" + fileURL(pdf) + ") und ![plan.png](" + fileURL(png) + ")\n" +
		"  - nochmal [Antrag](" + fileURL(pdf) + ")\n\n" +
		"- bare " + fileURL(png) + " and [mine](" + fileURL(elsewhere) + ")\n\n" +
		"- [gone](" + fileURL(filepath.Join(shelf, "gone.pdf")) + ") and [web](https://example.com)\n"
	rel := "pages/Projekt.md"
	if err := s.Store.Write(rel, []byte(text), ""); err != nil {
		t.Fatal(err)
	}

	// A dry run writes nothing and copies nothing, but says what it would do.
	dry, err := s.MigrateShelf(shelf, true)
	if err != nil {
		t.Fatal(err)
	}
	if dry.Links != 4 || len(dry.Copied) != 2 || len(dry.Missing) != 1 || len(dry.Notes) != 1 {
		t.Fatalf("dry run: %+v", dry)
	}
	if got := body(t, s, rel); got != text {
		t.Fatalf("a dry run wrote:\n%s", got)
	}
	if _, err := os.Stat(s.AttachDir()); !os.IsNotExist(err) {
		t.Fatalf("a dry run copied files: %v", err)
	}

	m, err := s.MigrateShelf(shelf, false)
	if err != nil {
		t.Fatal(err)
	}
	want := "- [Antrag](../assets/Projekt%20antrag.pdf) und ![plan.png](../assets/plan.png)\n" +
		"  - nochmal [Antrag](../assets/Projekt%20antrag.pdf)\n\n" +
		"- bare ![plan.png](../assets/plan.png) and [mine](" + fileURL(elsewhere) + ")\n\n" +
		"- [gone](" + fileURL(filepath.Join(shelf, "gone.pdf")) + ") and [web](https://example.com)\n"
	if got := body(t, s, rel); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	// Each file once, however many links named it.
	entries, _ := os.ReadDir(s.AttachDir())
	if len(entries) != 2 || len(m.Copied) != 2 || m.Copied[0].Rel == "" {
		t.Fatalf("copied: %+v, assets holds %d", m.Copied, len(entries))
	}
	// The shelf is left as it was: migrating copies.
	if _, err := os.Stat(pdf); err != nil {
		t.Fatal("the original was moved")
	}

	// Twice is harmless: nothing left to bring in, nothing rewritten.
	again, err := s.MigrateShelf(shelf, false)
	if err != nil {
		t.Fatal(err)
	}
	if again.Links != 0 || len(again.Copied) != 0 {
		t.Fatalf("second run: %+v", again)
	}
}

// Text kept as a note is a file of its own, called by its title, linked by it.
func TestAttachNoteKeepsTextAsALinkedFile(t *testing.T) {
	s := newSvc(t)
	now := time.Date(2026, 11, 12, 9, 0, 0, 0, time.Local)
	text := "# Rückmeldung zum Protokoll\n\nHallo Tilman,\ndanke für das Protokoll.\n"
	a, err := s.AttachNote(text, "", now)
	if err != nil {
		t.Fatal(err)
	}
	if a.Name != "2026-11-12-rückmeldung-zum-protokoll.md" {
		t.Fatalf("named %q", a.Name)
	}
	if !strings.HasPrefix(a.Link, "[Rückmeldung zum Protokoll](../assets/2026-11-12-") || !strings.HasSuffix(a.Link, ".md)") {
		t.Fatalf("linked as %q", a.Link)
	}
	got, err := os.ReadFile(a.Path)
	if err != nil || string(got) != strings.TrimSpace(text)+"\n" {
		t.Fatalf("kept %q, %v", got, err)
	}

	// The same title twice is two notes; nothing is overwritten.
	b, err := s.AttachNote("# Rückmeldung zum Protokoll\n\nanother one", "", now)
	if err != nil || b.Name == a.Name {
		t.Fatalf("second note %q, %v", b.Name, err)
	}
	if got, _ := os.ReadFile(a.Path); !strings.Contains(string(got), "Hallo Tilman") {
		t.Fatal("the first note was overwritten")
	}

	if _, err := s.AttachNote("  \n ", "", now); err == nil {
		t.Fatal("empty text should be refused")
	}
	if c, _ := s.AttachNote("whatever", "Minutes, 12 Nov", now); !strings.HasPrefix(c.Link, "[Minutes, 12 Nov](") {
		t.Fatalf("a given title should win: %q", c.Link)
	}
}

func TestSlashNotePastesTheClipboardAsANote(t *testing.T) {
	s := newSvc(t)
	s.Clipboard = func() (clipboard.Content, error) {
		return clipboard.Content{Text: "Minutes of the board\n\n- item one\n- item two\n"}, nil
	}
	rel, _ := s.AddToday("from the board ")
	text := "from the board /note"
	if _, err := s.RunCommand(addrOf(t, s, rel, 0), "note", "", text, 15, len(text)); err != nil {
		t.Fatal(err)
	}
	got := body(t, s, rel)
	if !strings.Contains(got, "from the board [Minutes of the board](../assets/") || strings.Contains(got, "/note") {
		t.Fatalf("the block is %q", got)
	}

	s.Clipboard = func() (clipboard.Content, error) { return clipboard.Content{Image: []byte{1}}, nil }
	if _, err := s.RunCommand(addrOf(t, s, rel, 0), "note", "", "x /note", 2, 7); err == nil {
		t.Fatal("a picture on the clipboard is not text to keep")
	}
}
