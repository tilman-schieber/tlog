package cli

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestAttachMigrate(t *testing.T) {
	dir := t.TempDir()
	shelf := filepath.Join(t.TempDir(), "store")
	if err := os.MkdirAll(shelf, 0o755); err != nil {
		t.Fatal(err)
	}
	pdf := filepath.Join(shelf, "Antrag.pdf")
	if err := os.WriteFile(pdf, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := (&url.URL{Scheme: "file", Path: filepath.ToSlash(pdf)}).String()
	if code, _, errs := runCLI(t, dir, "add", "-p", "Projekt", "[Antrag]("+link+")"); code != 0 {
		t.Fatal(errs)
	}

	// The migration flags are refused anywhere else, and files are refused with it.
	if code, _, _ := runCLI(t, dir, "attach", "-dry-run", pdf); code != exitUsage {
		t.Fatalf("-dry-run without -migrate: exit %d", code)
	}
	if code, _, _ := runCLI(t, dir, "attach", "-migrate", pdf); code != exitUsage {
		t.Fatalf("-migrate with a file: exit %d", code)
	}

	code, out, errs := runCLI(t, dir, "attach", "-migrate", "-from", shelf, "-dry-run")
	if code != 0 || !strings.Contains(out, "would rewrite 1 links in 1 notes") {
		t.Fatalf("dry run: %d %q %q", code, out, errs)
	}
	code, out, errs = runCLI(t, dir, "attach", "-migrate", "-from", shelf)
	if code != 0 || !strings.Contains(out, "→ assets/Antrag.pdf") {
		t.Fatalf("migrate: %d %q %q", code, out, errs)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "pages", "Projekt.md"))
	if !strings.Contains(string(got), "[Antrag](../assets/Antrag.pdf)") {
		t.Fatalf("page: %q", got)
	}
}

func TestTagCommands(t *testing.T) {
	dir := t.TempDir()
	if code, _, errs := runCLI(t, dir, "add", "-p", "tlog", "the outliner"); code != 0 {
		t.Fatal(errs)
	}
	if code, out, errs := runCLI(t, dir, "tag", "repo", "tlog"); code != 0 || !strings.Contains(out, "tlog is tagged repo") {
		t.Fatalf("tag: %d %q %q", code, out, errs)
	}
	if code, out, _ := runCLI(t, dir, "tag", "repo"); code != 0 || strings.TrimSpace(out) != "tlog" {
		t.Fatalf("list: %d %q", code, out)
	}
	code, out, _ := runCLI(t, dir, "tag", "repo", "-json")
	if code != 0 || !strings.Contains(out, `"rel": "pages/tlog.md"`) {
		t.Fatalf("json: %d %q", code, out)
	}
	if code, _, _ := runCLI(t, dir, "untag", "repo", "tlog"); code != 0 {
		t.Fatal("untag failed")
	}
	if code, _, _ := runCLI(t, dir, "untag", "repo", "tlog"); code != exitRefused {
		t.Fatalf("untagging what is not there: exit %d", code)
	}
	if code, _, _ := runCLI(t, dir, "tag"); code != exitUsage {
		t.Fatal("no tag should be a usage error")
	}
}

// fakeClipboard puts a wl-paste on PATH that holds one picture, or nothing.
func fakeClipboard(t *testing.T, png string) {
	t.Helper()
	bin := t.TempDir()
	if png != "" {
		if err := os.WriteFile(filepath.Join(bin, "shot.png"), []byte(png), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	script := `#!/bin/sh
case "$1" in
  --list-types) [ -f "` + bin + `/shot.png" ] && echo image/png; exit 0 ;;
  *) cat "` + bin + `/shot.png" ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "wl-paste"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WAYLAND_DISPLAY", "wayland-test")
}

func TestAttachTheScreenshotOnTheClipboard(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the fake clipboard is wl-paste")
	}
	dir := t.TempDir()
	fakeClipboard(t, onePixelPNG)
	code, out, errs := runCLI(t, dir, "attach", "-clipboard", "-link-only")
	if code != 0 || !regexp.MustCompile(`^!\[screenshot-[0-9-]+\.png\]\(\.\./assets/screenshot-[0-9-]+\.png\)\n$`).MatchString(out) {
		t.Fatalf("code %d out %q err %q", code, out, errs)
	}
	if code, _, errs := runCLI(t, dir, "attach", "-clipboard", "-p", "Grant"); code != 0 {
		t.Fatalf("onto a page: %d %s", code, errs)
	}
	page, _ := os.ReadFile(filepath.Join(dir, "pages", "Grant.md"))
	if !strings.Contains(string(page), "../assets/screenshot-") {
		t.Fatalf("not linked: %q", page)
	}

	fakeClipboard(t, "")
	if code, _, errs := runCLI(t, dir, "attach", "-clipboard"); code == 0 || errs == "" {
		t.Fatalf("an empty clipboard was not refused: %d %q", code, errs)
	}
	if code, _, _ := runCLI(t, dir, "attach", "-clipboard", "x.png"); code != 2 {
		t.Fatalf("a file with -clipboard: exit %d", code)
	}
}
