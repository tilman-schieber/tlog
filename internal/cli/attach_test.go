package cli

import (
	"net/url"
	"os"
	"path/filepath"
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
