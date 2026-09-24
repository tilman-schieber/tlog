package watch

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tilman-schieber/tlog/internal/store"
)

// sweep is driven directly here: no goroutine, no ticker, no sleeping. What is
// being tested is what counts as a change, and that is a pure comparison.

func newWatcher(t *testing.T) (*Watcher, *store.Store) {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w, err := New(s, 0)
	if err != nil {
		t.Fatal(err)
	}
	return w, s
}

func writeFile(t *testing.T, s *store.Store, rel, body string) {
	t.Helper()
	abs := s.Abs(rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestANewFileIsAChangeAndThenIsNot(t *testing.T) {
	w, s := newWatcher(t)
	writeFile(t, s, "journals/2026-09-24.md", "- from nvim\n")

	got := w.sweep()
	if len(got) != 1 || got[0].Rel != "journals/2026-09-24.md" || !got[0].Exists {
		t.Fatalf("got %+v", got)
	}
	if again := w.sweep(); len(again) != 0 {
		t.Fatalf("the same file was reported twice: %+v", again)
	}
}

func TestIdenticalBytesAreNotAChange(t *testing.T) {
	w, s := newWatcher(t)
	writeFile(t, s, "pages/Note.md", "- one\n")
	w.sweep()

	// A save that restores exactly what was there — a git checkout of an
	// unchanged file, or an editor writing the buffer back untouched.
	writeFile(t, s, "pages/Note.md", "- one\n")
	if got := w.sweep(); len(got) != 0 {
		t.Fatalf("rewriting the same bytes was reported: %+v", got)
	}
}

func TestABurstOfWritesCollapsesIntoOneChange(t *testing.T) {
	w, s := newWatcher(t)
	w.sweep()
	for _, body := range []string{"- a\n", "- ab\n", "- abc\n"} {
		writeFile(t, s, "pages/Note.md", body)
	}
	got := w.sweep()
	if len(got) != 1 {
		t.Fatalf("three writes between sweeps should be one change: %+v", got)
	}
	if got[0].Hash == "" {
		t.Fatal("a change should carry the hash it changed to")
	}
}

func TestADeletionIsAChange(t *testing.T) {
	w, s := newWatcher(t)
	writeFile(t, s, "pages/Gone.md", "- here\n")
	w.sweep()

	if err := os.Remove(s.Abs("pages/Gone.md")); err != nil {
		t.Fatal(err)
	}
	got := w.sweep()
	if len(got) != 1 || got[0].Exists {
		t.Fatalf("got %+v", got)
	}
}

func TestTlogsOwnWritesAreRecognised(t *testing.T) {
	// The loop has to terminate: a save must not come back as an external edit
	// and cause a reload, which would be indistinguishable from an edit war.
	w, s := newWatcher(t)
	if err := s.Write("pages/Note.md", []byte("- ours\n"), ""); err != nil {
		t.Fatal(err)
	}
	got := w.sweep()
	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
	if !Ours(s, got[0]) {
		t.Fatal("tlog did not recognise its own write")
	}

	writeFile(t, s, "pages/Note.md", "- theirs\n")
	got = w.sweep()
	if len(got) != 1 || Ours(s, got[0]) {
		t.Fatalf("someone else's write was taken for ours: %+v", got)
	}
}

func TestADeletionByTlogIsRecognisedToo(t *testing.T) {
	w, s := newWatcher(t)
	if err := s.Write("pages/Note.md", []byte("- ours\n"), ""); err != nil {
		t.Fatal(err)
	}
	w.sweep()
	// Emptying a file removes it, which is still tlog's own doing.
	if err := s.Write("pages/Note.md", nil, ""); err != nil {
		t.Fatal(err)
	}
	got := w.sweep()
	if len(got) != 1 || got[0].Exists {
		t.Fatalf("got %+v", got)
	}
	if !Ours(s, got[0]) {
		t.Fatal("tlog did not recognise its own deletion")
	}
}

func TestTheStoresOwnDroppingsAreNotChanges(t *testing.T) {
	// .git, temp files and editor swap files are not notes. The store decides
	// what a note is, and the watcher asks it rather than keeping its own list.
	w, s := newWatcher(t)
	writeFile(t, s, ".git/HEAD", "ref: refs/heads/main\n")
	writeFile(t, s, "pages/.tlog-123.tmp", "half a file")
	writeFile(t, s, "pages/.Note.md.swp", "vim")
	if got := w.sweep(); len(got) != 0 {
		t.Fatalf("something that is not a note was reported: %+v", got)
	}
}

func TestANewWatcherDoesNotReportWhatIsAlreadyThere(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, s, "pages/Old.md", "- from before\n")

	w, err := New(s, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := w.sweep(); len(got) != 0 {
		t.Fatalf("starting up reported the existing corpus: %+v", got)
	}
}
