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
