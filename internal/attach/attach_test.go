package attach

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	return Open(t.TempDir())
}

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAddKeepsTheNameAndLeavesTheOriginal(t *testing.T) {
	s := newStore(t)
	src := t.TempDir()
	orig := write(t, src, "report.pdf", "one")

	e, err := s.Add(orig)
	if err != nil {
		t.Fatal(err)
	}
	if e.Name != "report.pdf" {
		t.Fatalf("name: %q", e.Name)
	}
	// Attaching a file must not move it out from under whatever else refers to it.
	if _, err := os.Stat(orig); err != nil {
		t.Fatalf("the original was moved: %v", err)
	}
	if got, _ := os.ReadFile(e.Path); string(got) != "one" {
		t.Fatalf("contents: %q", got)
	}
}

// att's rule, restated here because att's packages cannot be imported: a clash
// gets a numeric suffix and nothing is ever overwritten.
func TestClashesAreSuffixedAndNothingIsOverwritten(t *testing.T) {
	s := newStore(t)
	src := t.TempDir()

	a, _ := s.Add(write(t, src, "report.pdf", "first"))
	b, _ := s.Add(write(t, filepath.Join(src, "sub"), "report.pdf", "second"))
	c, _ := s.Add(write(t, filepath.Join(src, "sub2"), "report.pdf", "third"))

	if a.Name != "report.pdf" || b.Name != "report-2.pdf" || c.Name != "report-3.pdf" {
		t.Fatalf("names: %q %q %q", a.Name, b.Name, c.Name)
	}
	for path, want := range map[string]string{a.Path: "first", b.Path: "second", c.Path: "third"} {
		if got, _ := os.ReadFile(path); string(got) != want {
			t.Fatalf("%s holds %q, want %q", filepath.Base(path), got, want)
		}
	}
}

func TestNameWithoutExtension(t *testing.T) {
	s := newStore(t)
	src := t.TempDir()
	_, _ = s.Add(write(t, src, "NOTES", "a"))
	e, _ := s.Add(write(t, filepath.Join(src, "x"), "NOTES", "b"))
	if e.Name != "NOTES-2" {
		t.Fatalf("got %q", e.Name)
	}
}

// The link is what lands in a note, so it has to match att byte for byte.
func TestLinkFormat(t *testing.T) {
	cases := map[string]string{
		"/a/report.pdf":       "[report.pdf](file:///a/report.pdf)",
		"/a/shot.png":         "![shot.png](file:///a/shot.png)",
		"/a/Shot.JPG":         "![Shot.JPG](file:///a/Shot.JPG)",
		"/a/meeting notes.md": "[meeting notes.md](file:///a/meeting%20notes.md)",
		"/a/b[c].txt":         `[b\[c\].txt](file:///a/b%5Bc%5D.txt)`,
	}
	for path, want := range cases {
		got := Link(Entry{Name: filepath.Base(path), Path: path})
		if got != want {
			t.Errorf("%s\n  got  %s\n  want %s", path, got, want)
		}
	}
}

func TestListIsNewestFirst(t *testing.T) {
	s := newStore(t)
	src := t.TempDir()
	for _, n := range []string{"a.txt", "b.txt", "c.txt"} {
		if _, err := s.Add(write(t, src, n, n)); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	got, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Name != "c.txt" || got[2].Name != "a.txt" {
		t.Fatalf("order: %+v", got)
	}
	// Every listed entry carries the link that would be written.
	if !strings.HasPrefix(got[0].Link, "[c.txt](file://") {
		t.Fatalf("link: %q", got[0].Link)
	}
}

func TestAnEmptyStoreIsNotAnError(t *testing.T) {
	got, err := Open(t.TempDir()).List()
	if err != nil || len(got) != 0 {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestMatchPrefersContainsThenFallsBackToFuzzy(t *testing.T) {
	entries := []Entry{
		{Name: "meeting notes.pdf"}, {Name: "meeting notes-2.pdf"}, {Name: "budget.xlsx"},
	}
	if got := Match(entries, "meeting"); len(got) != 2 {
		t.Fatalf("contains: %+v", got)
	}
	// att's fuzzy fallback: the letters in order.
	if got := Match(entries, "mtgnts2"); len(got) != 1 || got[0].Name != "meeting notes-2.pdf" {
		t.Fatalf("fuzzy: %+v", got)
	}
	if got := Match(entries, ""); len(got) != 3 {
		t.Fatalf("an empty query should match everything: %+v", got)
	}
	if got := Match(entries, "zzz"); len(got) != 0 {
		t.Fatalf("nonsense: %+v", got)
	}
}

func TestDirectoriesAndJunkAreNotAttachments(t *testing.T) {
	s := newStore(t)
	_ = os.MkdirAll(filepath.Join(s.Dir(), "a-folder"), 0o755)
	write(t, s.Dir(), ".DS_Store", "junk")
	write(t, s.Dir(), "real.txt", "yes")

	got, _ := s.List()
	if len(got) != 1 || got[0].Name != "real.txt" {
		t.Fatalf("got %+v", got)
	}
}

func TestAddRefusesWhatIsNotAFile(t *testing.T) {
	s := newStore(t)
	if _, err := s.Add(t.TempDir()); err == nil {
		t.Fatal("a directory is not an attachment")
	}
	if _, err := s.Add(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("a missing file should be an error")
	}
}

func TestHuman(t *testing.T) {
	cases := map[int64]string{0: "0 B", 512: "512 B", 2048: "2.0 KB", 5 << 20: "5.0 MB"}
	for n, want := range cases {
		if got := Human(n); got != want {
			t.Errorf("%d → %q, want %q", n, got, want)
		}
	}
}
