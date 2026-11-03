package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveLinkOpensOnlyWhatALinkShouldBe(t *testing.T) {
	s := newSvc(t)
	file := filepath.Join(t.TempDir(), "plan.png")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(s.Store.Root, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(s.Store.Root, "assets", "a.pdf")
	if err := os.WriteFile(local, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct{ target, want string }{
		{"https://go.dev", "https://go.dev"},
		{"mailto:a@b.c", "mailto:a@b.c"},
		{"file://" + file, file},
		{file, file},
		{"../assets/a.pdf", local}, // relative to the note, like Logseq writes them
	} {
		got, err := s.ResolveLink("pages/Plan.md", c.target)
		if err != nil || got != c.want {
			t.Errorf("%q: got %q, %v; want %q", c.target, got, err, c.want)
		}
	}
	for _, bad := range []string{"javascript:alert(1)", "file:///nowhere/at/all", "missing.png", ""} {
		if _, err := s.ResolveLink("pages/Plan.md", bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestOpenLinkHandsTheTargetOver(t *testing.T) {
	s := newSvc(t)
	var opened []string
	s.Launch = func(target string) error { opened = append(opened, target); return nil }
	if err := s.OpenLink("journals/x.md", "https://go.dev"); err != nil {
		t.Fatal(err)
	}
	if len(opened) != 1 || opened[0] != "https://go.dev" {
		t.Fatalf("got %v", opened)
	}
}

func TestDroppedFilesReadsWhatTerminalsType(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "plan.png")
	spaced := filepath.Join(dir, "my notes.pdf")
	for _, p := range []string{plain, spaced} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	escaped := strings.ReplaceAll(spaced, " ", `\ `)

	for _, drop := range []string{
		plain + " " + escaped,              // kitty, ghostty: shell-escaped
		"'" + plain + "' '" + spaced + "'", // quoted
		"file://" + plain + "\nfile://" + strings.ReplaceAll(spaced, " ", "%20"), // a uri-list
		plain + "\n" + spaced + "\n", // one plain path per line, spaces and all
		plain + "\r\n" + spaced,      // ...with Windows line ends
	} {
		got := DroppedFiles(drop)
		if len(got) != 2 || got[0] != plain || got[1] != spaced {
			t.Errorf("%q: got %q", drop, got)
		}
	}
	for _, text := range []string{
		"just some text",
		plain + " and some words", // a path in prose is not a drop
		dir,                       // a directory is not a file to attach
		filepath.Join(dir, "gone.png"),
		"",
	} {
		if got := DroppedFiles(text); got != nil {
			t.Errorf("%q was taken for a drop: %q", text, got)
		}
	}
}

func TestADeadlineTypedByHandIsPinnedWhenWritten(t *testing.T) {
	s := newSvc(t)
	rel := s.TodayRel()
	// As if typed in nvim: a relative deadline and a German one.
	body := "- call the printer\n  due:: tomorrow\n\n- send the slides\n  Deadline:: 24.12.2026\n"
	if err := s.Store.Write(rel, []byte(body), ""); err != nil {
		t.Fatal(err)
	}
	d, err := s.Load(rel)
	if err != nil {
		t.Fatal(err)
	}
	// Read strictly, "tomorrow" is not a fixed day until it is written down.
	if _, ok := Deadline(d.Doc.Blocks[0]); ok {
		t.Fatal("a relative deadline in a file was read as a fixed day")
	}
	if got, ok := Deadline(d.Doc.Blocks[1]); !ok || got.Format("2006-01-02") != "2026-12-24" {
		t.Fatalf("24.12.2026 read as %v %v", got, ok)
	}

	// Any write through tlog normalizes the file.
	if _, err := s.Add(rel, "a third block"); err != nil {
		t.Fatal(err)
	}
	f, err := s.Store.Read(rel)
	if err != nil {
		t.Fatal(err)
	}
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	if !strings.Contains(string(f.Data), "due:: "+tomorrow) || !strings.Contains(string(f.Data), "Deadline:: 2026-12-24") {
		t.Fatalf("not normalized: %q", f.Data)
	}
}

func TestDateChoicesSayWhatTheyMean(t *testing.T) {
	got := DateChoices("", 8)
	if len(got) < 3 || got[0].Phrase != "today" || got[0].ISO != time.Now().Format("2006-01-02") || got[0].Label != "today" {
		t.Fatalf("got %+v", got)
	}
	if got := DateChoices("in 2", 8); len(got) != 3 || got[1].Phrase != "in 2 weeks" {
		t.Fatalf("got %+v", got)
	}
}

func TestLinkedTextReadsOnlyPlainTextFiles(t *testing.T) {
	s := newSvc(t)
	assets := filepath.Join(s.Store.Root, "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name string, data []byte) {
		if err := os.WriteFile(filepath.Join(assets, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("Regeln Übersicht.md", []byte("\xef\xbb\xbf# Regeln\r\nText\r\n"))
	write("plot.png", []byte("\x89PNG\r\n\x1a\n\x00\x00"))
	write("latin1.txt", []byte("Gr\xfc\xdfe"))
	write("big.txt", []byte(strings.Repeat("x", maxTextFile+1)))

	f, err := s.LinkedText("pages/NOA.md", "../assets/Regeln%20%C3%9Cbersicht.md")
	if err != nil || f == nil {
		t.Fatalf("the markdown file was not read: %v %v", f, err)
	}
	if f.Name != "Regeln Übersicht.md" || f.Lang != "markdown" || f.Text != "# Regeln\nText\n" {
		t.Fatalf("got %+v", f)
	}
	for _, other := range []string{"../assets/plot.png", "../assets/latin1.txt", "../assets/big.txt", "https://go.dev"} {
		if f, err := s.LinkedText("pages/NOA.md", other); err != nil || f != nil {
			t.Errorf("%s: got %v, %v; want nothing to show and no error", other, f, err)
		}
	}
	if _, err := s.LinkedText("pages/NOA.md", "../assets/gone.md"); err == nil {
		t.Error("a link to nothing was not an error")
	}
}
