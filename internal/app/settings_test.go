package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func cfgSvc(t *testing.T) *Service {
	t.Helper()
	t.Setenv("TLOG_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	return newSvc(t)
}

func get(t *testing.T, s *Service, key string) Setting {
	t.Helper()
	for _, it := range s.Settings() {
		if it.Key == key {
			return it
		}
	}
	t.Fatalf("no setting %q", key)
	return Setting{}
}

// Pushing and the remote belong to the notes directory, so that a copy of the
// notes carries them along and no second file can disagree.
func TestSomeSettingsLiveInTheNotesRepoNotInTlog(t *testing.T) {
	s := cfgSvc(t)
	for _, key := range []string{"git.autopush", "git.remote"} {
		if got := get(t, s, key).Source; got != "notes" {
			t.Errorf("%s says %q", key, got)
		}
	}
	for _, key := range []string{"notes", "git.autocommit", "git.debounce"} {
		if got := get(t, s, key).Source; got != "config" {
			t.Errorf("%s says %q", key, got)
		}
	}
}

func TestSettingTheRemoteReachesGit(t *testing.T) {
	s := cfgSvc(t)
	if !s.Git.Enabled() {
		t.Skip("git is not available")
	}
	bare := t.TempDir()

	if got := get(t, s, "git.remote").Value; got != "" {
		t.Fatalf("a fresh notes directory has no remote, got %q", got)
	}
	if _, err := s.SetSetting("git.remote", bare); err != nil {
		t.Fatal(err)
	}
	if got := s.Git.Remote(); got != bare {
		t.Fatalf("git does not know about it: %q", got)
	}
	// And it is really in the notes repository, not in tlog's config.
	data, _ := os.ReadFile(filepath.Join(s.Store.Root, ".git", "config"))
	if !strings.Contains(string(data), bare) {
		t.Fatalf("not in the notes repo:\n%s", data)
	}
	cfgData, _ := os.ReadFile(os.Getenv("TLOG_CONFIG"))
	if strings.Contains(string(cfgData), bare) {
		t.Fatal("the remote leaked into tlog's config")
	}
}

// Clearing the remote takes pushing down with it, rather than leaving a switch
// that cannot do anything.
func TestClearingTheRemoteTurnsPushingOff(t *testing.T) {
	s := cfgSvc(t)
	if !s.Git.Enabled() {
		t.Skip("git is not available")
	}
	if _, err := s.SetSetting("git.remote", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetSetting("git.autopush", "true"); err != nil {
		t.Fatal(err)
	}
	if !s.Git.AutoPush() {
		t.Fatal("pushing did not turn on")
	}

	if _, err := s.SetSetting("git.remote", ""); err != nil {
		t.Fatal(err)
	}
	if s.Git.AutoPush() {
		t.Fatal("pushing was left on with nowhere to push")
	}
}

func TestAutoCommitCanBeTurnedOff(t *testing.T) {
	s := cfgSvc(t)
	if !s.Git.Enabled() {
		t.Skip("git is not available")
	}
	if _, err := s.SetSetting("git.autocommit", "false"); err != nil {
		t.Fatal(err)
	}
	if s.Git.AutoCommit() {
		t.Fatal("still on")
	}

	// Writing still happens, and is still remembered for a later commit.
	if _, err := s.AddToday("eine Notiz"); err != nil {
		t.Fatal(err)
	}
	out, _ := gitOut(s.Store.Root, "status", "--porcelain")
	if strings.TrimSpace(out) == "" {
		t.Fatal("with autocommit off the note should still be uncommitted")
	}
	if err := s.Commit(); err != nil {
		t.Fatal(err)
	}
	out, _ = gitOut(s.Store.Root, "status", "--porcelain")
	if strings.TrimSpace(out) != "" {
		t.Fatalf("an explicit commit should pick it up:\n%s", out)
	}
}

func TestStartupOpensTodayOrTheLastThingWritten(t *testing.T) {
	s := cfgSvc(t)
	if got := s.StartupRel(); got != s.TodayRel() {
		t.Fatalf("today is the default, got %q", got)
	}

	if _, err := s.AddToPage("Zuletzt", "etwas"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetSetting("startup", "last"); err != nil {
		t.Fatal(err)
	}
	if got := s.StartupRel(); got != "pages/Zuletzt.md" {
		t.Fatalf("got %q", got)
	}
	// An empty notes directory still opens on something.
	empty := cfgSvc(t)
	if _, err := empty.SetSetting("startup", "last"); err != nil {
		t.Fatal(err)
	}
	if got := empty.StartupRel(); got != empty.TodayRel() {
		t.Fatalf("got %q", got)
	}
}

func TestBadValuesAreRefusedAndChangeNothing(t *testing.T) {
	s := cfgSvc(t)
	before := s.Cfg
	for _, c := range [][2]string{
		{"startup", "gestern"}, {"git.debounce", "banana"},
		{"git.autocommit", "vielleicht"}, {"dates.end_of_week", "montag"},
		{"deadline.property", ""}, {"nope", "x"},
	} {
		if _, err := s.SetSetting(c[0], c[1]); err == nil {
			t.Errorf("%s = %q was accepted", c[0], c[1])
		}
	}
	if s.Cfg != before {
		t.Fatal("a refused value changed the configuration anyway")
	}
}

func TestAConsequenceIsAnnouncedRatherThanDiscovered(t *testing.T) {
	s := cfgSvc(t)
	note, err := s.SetSetting("notes", "~/zettel")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(note, "nächsten Start") {
		t.Fatalf("got %q", note)
	}
	if note, _ = s.SetSetting("format.blank_lines", "false"); !strings.Contains(note, "neu formatiert") {
		t.Fatalf("got %q", note)
	}
	if note, _ = s.SetSetting("deadline.property", "due"); note != "" {
		t.Fatalf("this one takes effect at once, got %q", note)
	}
}

func TestToggle(t *testing.T) {
	s := cfgSvc(t)
	if _, err := s.ToggleSetting("attachments.sanitize"); err != nil {
		t.Fatal(err)
	}
	if !s.Cfg.Attachments.Sanitize {
		t.Fatal("not toggled")
	}
	if _, err := s.ToggleSetting("notes"); err == nil {
		t.Fatal("text is not something to toggle")
	}
}

// Every setting must say what it is, or a menu is a list of riddles.
func TestEverySettingIsDescribed(t *testing.T) {
	s := cfgSvc(t)
	for _, it := range s.Settings() {
		if it.Key == "" || it.Kind == "" || it.Hint == "" || it.Source == "" {
			t.Errorf("incomplete: %+v", it)
		}
		if it.Kind != "bool" && it.Kind != "text" && it.Kind != "duration" {
			t.Errorf("%s has kind %q", it.Key, it.Kind)
		}
	}
}
