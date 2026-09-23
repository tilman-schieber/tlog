package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runCLI(t *testing.T, dir string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	full := append([]string{"-dir", dir}, args...)
	code = Main(full, &out, &errb)
	return code, out.String(), errb.String()
}

func TestAddAndToday(t *testing.T) {
	dir := t.TempDir()

	code, out, errs := runCLI(t, dir, "today")
	if code != 0 {
		t.Fatalf("today failed: %d %s", code, errs)
	}
	path := strings.TrimSpace(out)
	if !strings.HasSuffix(path, ".md") || !strings.Contains(path, "journals") {
		t.Fatalf("unexpected path %q", path)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("printing today's path should not create the file")
	}

	if code, _, errs = runCLI(t, dir, "add", "buy", "milk"); code != 0 {
		t.Fatalf("add failed: %d %s", code, errs)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "- buy milk\n" {
		t.Fatalf("got %q", data)
	}
}

func TestAddToPage(t *testing.T) {
	dir := t.TempDir()
	code, out, errs := runCLI(t, dir, "add", "-p", "Project Foo", "implement the parser")
	if code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	data, err := os.ReadFile(strings.TrimSpace(out))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "implement the parser") {
		t.Fatalf("got %q", data)
	}
	if !strings.HasSuffix(strings.TrimSpace(out), filepath.Join("pages", "Project Foo.md")) {
		t.Fatalf("wrong destination %q", out)
	}
}

func TestOpenCreatesThePage(t *testing.T) {
	dir := t.TempDir()
	code, out, errs := runCLI(t, dir, "open", "Rust")
	if code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	if _, err := os.Stat(strings.TrimSpace(out)); err != nil {
		t.Fatalf("open should create the page: %v", err)
	}
}

func TestEverythingIsCommitted(t *testing.T) {
	dir := t.TempDir()
	if code, _, errs := runCLI(t, dir, "add", "a note"); code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Skipf("git not available: %v", err)
	}
	// A one-shot command must leave nothing uncommitted behind it.
	if out, err := gitOutput(dir, "status", "--porcelain"); err != nil {
		t.Fatal(err)
	} else if strings.TrimSpace(out) != "" {
		t.Fatalf("uncommitted changes after a CLI write:\n%s", out)
	}
}

func TestUnknownCommandAndFlagExitTwo(t *testing.T) {
	dir := t.TempDir()
	if code, _, errs := runCLI(t, dir, "frobnicate"); code != 2 || !strings.Contains(errs, "unknown command") {
		t.Fatalf("code=%d stderr=%q", code, errs)
	}
	if code, _, errs := runCLI(t, dir, "-nope"); code != 2 || !strings.Contains(errs, "unknown flag") {
		t.Fatalf("code=%d stderr=%q", code, errs)
	}
}

func TestHelpAndVersion(t *testing.T) {
	dir := t.TempDir()
	if code, out, _ := runCLI(t, dir, "help"); code != 0 || !strings.Contains(out, "usage:") {
		t.Fatalf("code=%d out=%q", code, out)
	}
	if code, out, _ := runCLI(t, dir, "version"); code != 0 || strings.TrimSpace(out) != Version {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestAddWithNothingToAddIsRejected(t *testing.T) {
	dir := t.TempDir()
	// Point stdin at an empty file so the command does not block.
	old := os.Stdin
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = f
	defer func() { os.Stdin = old; f.Close() }()

	if code, _, errs := runCLI(t, dir, "add"); code != 2 || !strings.Contains(errs, "nothing to add") {
		t.Fatalf("code=%d stderr=%q", code, errs)
	}
}

func TestImportDryRunReportsWithoutWriting(t *testing.T) {
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "journals"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "journals", "2026_09_15.md"), []byte("- TODO a task\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	code, out, errs := runCLI(t, dir, "import", "-from", src, "-dry-run")
	if code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	if !strings.Contains(out, "dry run") || !strings.Contains(out, "1 task markers") {
		t.Fatalf("unhelpful report: %q", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "journals", "2026-09-15.md")); !os.IsNotExist(err) {
		t.Fatal("dry run wrote a file")
	}

	if code, out, errs = runCLI(t, dir, "import", "-from", src); code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	data, err := os.ReadFile(filepath.Join(dir, "journals", "2026-09-15.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "- [ ] a task\n" {
		t.Fatalf("got %q", data)
	}
}

func TestImportOfANonGraphFailsClearly(t *testing.T) {
	dir := t.TempDir()
	code, _, errs := runCLI(t, dir, "import", "-from", t.TempDir())
	if code != 1 || !strings.Contains(errs, "no journals/ or pages/") {
		t.Fatalf("code=%d stderr=%q", code, errs)
	}
}
