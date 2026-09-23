package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestMissingFileReadsAsEmpty(t *testing.T) {
	s := newStore(t)
	f, err := s.Read("pages/Nope.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Data) != 0 {
		t.Fatalf("expected empty, got %q", f.Data)
	}
	// Creating and updating must be one path: writing against the hash of
	// emptiness has to succeed.
	if err := s.Write("pages/Nope.md", []byte("- hi\n"), f.Hash); err != nil {
		t.Fatal(err)
	}
}

func TestWriteRefusesStaleHash(t *testing.T) {
	s := newStore(t)
	if err := s.Write("pages/A.md", []byte("- one\n"), ""); err != nil {
		t.Fatal(err)
	}
	f, _ := s.Read("pages/A.md")
	stale := f.Hash

	// Someone else writes — nvim, another terminal, an agent.
	if err := s.Write("pages/A.md", []byte("- one\n- two\n"), ""); err != nil {
		t.Fatal(err)
	}

	err := s.Write("pages/A.md", []byte("- clobbered\n"), stale)
	var conflict *ErrConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("expected a conflict, got %v", err)
	}
	if got, _ := s.Read("pages/A.md"); string(got.Data) != "- one\n- two\n" {
		t.Fatalf("the other writer's work was destroyed: %q", got.Data)
	}
	if !strings.Contains(conflict.Error(), "changed on disk") {
		t.Fatalf("unhelpful conflict message: %s", conflict)
	}
}

func TestWriteIsAtomicAndLeavesNoTempFiles(t *testing.T) {
	s := newStore(t)
	if err := s.Write("pages/A.md", []byte("- one\n"), ""); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(s.Root, PagesDir))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tlog-") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one file, got %d", len(entries))
	}
}

func TestWritingNothingRemovesTheFile(t *testing.T) {
	s := newStore(t)
	if err := s.Write("pages/A.md", []byte("- one\n"), ""); err != nil {
		t.Fatal(err)
	}
	f, _ := s.Read("pages/A.md")
	if err := s.Write("pages/A.md", nil, f.Hash); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.Abs("pages/A.md")); !os.IsNotExist(err) {
		t.Fatal("emptied file should be removed, not left as a husk")
	}
}

func TestPageNamesAreFilenames(t *testing.T) {
	s := newStore(t)
	rel, err := s.PageRel("Project Foo")
	if err != nil {
		t.Fatal(err)
	}
	if rel != filepath.Join(PagesDir, "Project Foo.md") {
		t.Fatalf("got %q", rel)
	}
	for _, bad := range []string{"", "  ", "a/b", `a\b`, ".", ".."} {
		if _, err := s.PageRel(bad); !errors.Is(err, ErrBadPageName) {
			t.Fatalf("accepted bad page name %q", bad)
		}
	}
}

func TestResolvePageIsCaseInsensitive(t *testing.T) {
	s := newStore(t)
	if err := s.Write(filepath.Join(PagesDir, "Project Foo.md"), []byte("- x\n"), ""); err != nil {
		t.Fatal(err)
	}
	rel, exists, err := s.ResolvePage("project foo")
	if err != nil || !exists {
		t.Fatalf("not resolved: %v %v", exists, err)
	}
	if PageName(rel) != "Project Foo" {
		t.Fatalf("resolved to %q — original casing should win", PageName(rel))
	}

	rel, exists, err = s.ResolvePage("Brand New")
	if err != nil || exists {
		t.Fatalf("unexpectedly exists: %v %v", exists, err)
	}
	if rel != filepath.Join(PagesDir, "Brand New.md") {
		t.Fatalf("wrong destination for a new page: %q", rel)
	}
}

func TestJournalPathsAreISO(t *testing.T) {
	s := newStore(t)
	day := time.Date(2026, 9, 17, 13, 0, 0, 0, time.UTC)
	rel := s.JournalRel(day)
	if rel != filepath.Join(JournalsDir, "2026-09-17.md") {
		t.Fatalf("got %q", rel)
	}
	if !IsJournal(rel) {
		t.Fatal("not recognised as a journal")
	}
	got, ok := JournalDay(rel)
	if !ok || !got.Equal(time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("round trip: %v %v", got, ok)
	}
	if IsJournal(filepath.Join(PagesDir, "Rust.md")) {
		t.Fatal("a page was called a journal")
	}
}

func TestListOrdersJournalsNewestFirst(t *testing.T) {
	s := newStore(t)
	for _, rel := range []string{
		filepath.Join(JournalsDir, "2026-09-15.md"),
		filepath.Join(JournalsDir, "2026-09-17.md"),
		filepath.Join(PagesDir, "zeta.md"),
		filepath.Join(PagesDir, "Alpha.md"),
	} {
		if err := s.Write(rel, []byte("- x\n"), ""); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.Join(JournalsDir, "2026-09-17.md"),
		filepath.Join(JournalsDir, "2026-09-15.md"),
		filepath.Join(PagesDir, "Alpha.md"),
		filepath.Join(PagesDir, "zeta.md"),
	}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("position %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestGitCommitsAndReverts(t *testing.T) {
	s := newStore(t)
	g := NewGit(s.Root)
	if !g.Enabled() {
		t.Skip("git is not available")
	}
	if err := s.Write("journals/2026-09-17.md", []byte("- first\n"), ""); err != nil {
		t.Fatal(err)
	}
	g.Touch("journals/2026-09-17.md")
	if err := g.Flush(); err != nil {
		t.Fatal(err)
	}

	// A second flush with nothing outstanding must not fail or make an empty
	// commit.
	if err := g.Flush(); err != nil {
		t.Fatal(err)
	}

	if err := s.Write("journals/2026-09-17.md", []byte("- oops\n"), ""); err != nil {
		t.Fatal(err)
	}
	if err := g.Revert(); err != nil {
		t.Fatal(err)
	}
	f, _ := s.Read("journals/2026-09-17.md")
	if string(f.Data) != "- first\n" {
		t.Fatalf("revert did not restore the committed version: %q", f.Data)
	}
}

func TestDateNamesResolveToJournalsNotPages(t *testing.T) {
	s := newStore(t)
	journal := filepath.Join(JournalsDir, "2026-09-17.md")
	if err := s.Write(journal, []byte("- the real day\n"), ""); err != nil {
		t.Fatal(err)
	}

	// Following [[2026-09-17]], clicking it in a sidebar, or `tlog open` must
	// all land on the journal. Resolving it as a page would show an empty file
	// and create pages/2026-09-17.md as junk.
	rel, exists, err := s.ResolvePage("2026-09-17")
	if err != nil {
		t.Fatal(err)
	}
	if rel != journal || !exists {
		t.Fatalf("got %q exists=%v, want %q true", rel, exists, journal)
	}

	// A day with no file yet resolves to where it would go, and is not created.
	rel, exists, err = s.ResolvePage("2026-01-02")
	if err != nil {
		t.Fatal(err)
	}
	if rel != filepath.Join(JournalsDir, "2026-01-02.md") || exists {
		t.Fatalf("got %q exists=%v", rel, exists)
	}

	// Names that merely look date-ish are still pages.
	for _, name := range []string{"2026-09", "2026-13-01", "Q3 2026", "2026-09-17 review"} {
		rel, _, err := s.ResolvePage(name)
		if err != nil {
			t.Fatal(err)
		}
		if IsJournal(rel) {
			t.Fatalf("%q was treated as a journal: %q", name, rel)
		}
	}
}

// --- pushing ----------------------------------------------------------------

// bareRemote gives the store somewhere to push to, so the whole path can be
// exercised without a network.
func bareRemote(t *testing.T, s *Store) string {
	t.Helper()
	dir := t.TempDir()
	if err := run(dir, "init", "-q", "--bare"); err != nil {
		t.Fatal(err)
	}
	if err := run(s.Root, "remote", "add", "origin", dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

func commitSomething(t *testing.T, s *Store, g *Git, rel, body string) {
	t.Helper()
	if err := s.Write(rel, []byte(body), ""); err != nil {
		t.Fatal(err)
	}
	g.Touch(rel)
	if err := g.Flush(); err != nil {
		t.Fatal(err)
	}
}

func TestNothingPushesUnlessAsked(t *testing.T) {
	s := newStore(t)
	g := NewGit(s.Root)
	if !g.Enabled() {
		t.Skip("git is not available")
	}
	remote := bareRemote(t, s)

	// Writing a note and publishing it are different acts.
	if g.AutoPush() {
		t.Fatal("auto-push should be off until the directory is told otherwise")
	}
	commitSomething(t, s, g, "journals/2026-09-17.md", "- private\n")

	out, err := output(remote, "log", "--oneline")
	if err == nil && strings.TrimSpace(out) != "" {
		t.Fatalf("something was pushed without being asked: %q", out)
	}
}

func TestAutoPushSendsEachCommit(t *testing.T) {
	s := newStore(t)
	g := NewGit(s.Root)
	if !g.Enabled() {
		t.Skip("git is not available")
	}
	remote := bareRemote(t, s)

	if err := g.SetAutoPush(true); err != nil {
		t.Fatal(err)
	}
	commitSomething(t, s, g, "journals/2026-09-17.md", "- first\n")
	commitSomething(t, s, g, "journals/2026-09-18.md", "- second\n")

	out, _ := output(remote, "log", "--oneline")
	if n := len(strings.Fields(strings.TrimSpace(out))); n == 0 {
		t.Fatal("nothing reached the remote")
	}
	if !strings.Contains(out, "2026-09-18") {
		t.Fatalf("the second commit did not arrive:\n%s", out)
	}

	// The setting lives in the repository, so a later run remembers it.
	if !NewGit(s.Root).AutoPush() {
		t.Fatal("auto-push did not persist")
	}
}

func TestAFailedPushStillLeavesTheCommit(t *testing.T) {
	s := newStore(t)
	g := NewGit(s.Root)
	if !g.Enabled() {
		t.Skip("git is not available")
	}
	// A remote that cannot work: the commit must survive regardless.
	if err := run(s.Root, "remote", "add", "origin", filepath.Join(t.TempDir(), "nope")); err != nil {
		t.Fatal(err)
	}
	if err := g.SetAutoPush(true); err != nil {
		t.Fatal(err)
	}

	if err := s.Write("journals/2026-09-17.md", []byte("- kept\n"), ""); err != nil {
		t.Fatal(err)
	}
	g.Touch("journals/2026-09-17.md")
	err := g.Flush()
	if err == nil {
		t.Fatal("a failed push should be reported")
	}
	if !strings.Contains(err.Error(), "committed, but not pushed") {
		t.Fatalf("a failed push must not read as a failed save: %v", err)
	}

	out, _ := output(s.Root, "log", "--oneline")
	if !strings.Contains(out, "2026-09-17") {
		t.Fatalf("the commit was lost: %q", out)
	}
	if g.LastPushError() == nil {
		t.Fatal("the failure should be remembered for an adapter to show")
	}
}

func TestPushIsNeverForcedWhenTheRemoteMoved(t *testing.T) {
	s := newStore(t)
	g := NewGit(s.Root)
	if !g.Enabled() {
		t.Skip("git is not available")
	}
	remote := bareRemote(t, s)
	commitSomething(t, s, g, "journals/2026-09-17.md", "- ours\n")
	if err := g.Push(); err != nil {
		t.Fatal(err)
	}

	// Another machine pushes something this copy has never seen.
	other := t.TempDir()
	if err := run(other, "clone", "-q", remote, other+"/c"); err != nil {
		t.Fatal(err)
	}
	c := other + "/c"
	_ = run(c, "config", "user.name", "other")
	_ = run(c, "config", "user.email", "other@localhost")
	if err := os.WriteFile(filepath.Join(c, "elsewhere.md"), []byte("- theirs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = run(c, "add", "-A")
	if err := run(c, "commit", "-q", "-m", "from elsewhere"); err != nil {
		t.Fatal(err)
	}
	if err := run(c, "push", "-q"); err != nil {
		t.Fatal(err)
	}

	commitSomething(t, s, g, "journals/2026-09-19.md", "- ours again\n")
	err := g.Push()
	if err == nil {
		t.Fatal("pushing over a moved remote should be refused, not forced")
	}
	if !strings.Contains(err.Error(), "rejected") || !strings.Contains(err.Error(), "pull --rebase") {
		t.Fatalf("the message should say what happened and what to do: %v", err)
	}

	// And the other machine's work is still there.
	out, _ := output(remote, "log", "--oneline")
	if !strings.Contains(out, "from elsewhere") {
		t.Fatalf("the other machine's commit was destroyed:\n%s", out)
	}
}

func TestAutoPushRefusedWithoutARemote(t *testing.T) {
	s := newStore(t)
	g := NewGit(s.Root)
	if !g.Enabled() {
		t.Skip("git is not available")
	}
	if err := g.SetAutoPush(true); err == nil || !strings.Contains(err.Error(), "no remote") {
		t.Fatalf("expected a refusal naming the missing remote, got %v", err)
	}
	if g.AutoPush() {
		t.Fatal("auto-push turned on with nowhere to push")
	}
}
