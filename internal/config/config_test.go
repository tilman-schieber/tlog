package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func at(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("TLOG_CONFIG", p)
	return p
}

func TestAMissingFileIsEveryDefault(t *testing.T) {
	at(t)
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got != Default() {
		t.Fatalf("got %+v", got)
	}
	if Exists() {
		t.Fatal("nothing should have been written")
	}
}

func TestWhatIsNotMentionedStaysDefault(t *testing.T) {
	p := at(t)
	// A file that sets one thing must not blank out everything else.
	if err := os.WriteFile(p, []byte("[attachments]\nsanitize = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !got.Attachments.Sanitize {
		t.Fatal("the setting was not read")
	}
	if got.Notes != Default().Notes || got.Git.Debounce != Default().Git.Debounce {
		t.Fatalf("defaults were lost: %+v", got)
	}
}

func TestRoundTrip(t *testing.T) {
	at(t)
	want := Default()
	want.Notes = "~/zettel"
	want.Attachments.Sanitize = true
	want.Attachments.Lowercase = true
	want.Git.Debounce = "5s"
	want.Format.BlankLines = false
	want.Deadline.Property = "due"
	want.Dates.EndOfWeek = "sunday"

	if err := want.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

// The file is how you find out what can be configured, so it has to explain
// itself rather than being a list of bare keys.
func TestTheWrittenFileExplainsItself(t *testing.T) {
	at(t)
	if err := Default().Save(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(Path())
	text := string(data)
	for _, must := range []string{
		"$TLOG_DIR", "$ATT_DIR", "tlog.autopush", "att's rule is that files keep",
		"reformats each file", "end_of_week",
	} {
		if !strings.Contains(text, must) {
			t.Errorf("the file should mention %q", must)
		}
	}
	if strings.Count(text, "#") < 10 {
		t.Error("the file should be commented")
	}
}

func TestTheEnvironmentBeatsTheFile(t *testing.T) {
	at(t)
	c := Default()
	c.Notes = "/from/file"
	c.Attachments.Dir = "/shelf/from/file"

	if got := c.NotesDir(); got != "/from/file" {
		t.Fatalf("got %q", got)
	}
	t.Setenv("TLOG_DIR", "/from/env")
	t.Setenv("ATT_DIR", "/shelf/from/env")
	if got := c.NotesDir(); got != "/from/env" {
		t.Fatalf("notes: %q", got)
	}
	if got := c.AttachDir(); got != "/shelf/from/env" {
		t.Fatalf("attachments: %q", got)
	}
}

func TestTildeIsExpanded(t *testing.T) {
	at(t)
	home, _ := os.UserHomeDir()
	c := Default()
	if got := c.NotesDir(); got != filepath.Join(home, "notes") {
		t.Fatalf("got %q", got)
	}
	c.Attachments.Dir = "~"
	if got := c.AttachDir(); got != home {
		t.Fatalf("got %q", got)
	}
	c.Notes = "/absolute/stays"
	if got := c.NotesDir(); got != "/absolute/stays" {
		t.Fatalf("got %q", got)
	}
}

// A value nobody can parse must fall back rather than break writing notes.
func TestNonsenseValuesFallBack(t *testing.T) {
	at(t)
	c := Default()
	c.Git.Debounce = "banana"
	if got := c.DebounceDuration(); got != 30*time.Second {
		t.Fatalf("got %s", got)
	}
	c.Git.Debounce = "-5s"
	if got := c.DebounceDuration(); got != 30*time.Second {
		t.Fatalf("a negative delay should fall back, got %s", got)
	}
	c.Deadline.Property = "   "
	if got := c.DeadlineProperty(); got != "Deadline" {
		t.Fatalf("got %q", got)
	}
}

func TestBrokenTomlIsReportedNotSwallowed(t *testing.T) {
	p := at(t)
	if err := os.WriteFile(p, []byte("this is not toml ["), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err == nil {
		t.Fatal("a broken file should be reported")
	}
	if !strings.Contains(err.Error(), p) {
		t.Fatalf("the error should name the file: %v", err)
	}
	// And the caller still gets something usable rather than a zero value.
	if got != Default() {
		t.Fatalf("got %+v", got)
	}
}

func TestSaveIsAtomicAndLeavesNoTempFiles(t *testing.T) {
	p := at(t)
	if err := Default().Save(); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Dir(p))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tlog-config-") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

// The same dotfiles are stowed onto an Arch machine, so the config belongs
// where everything else already is rather than in a macOS-only location.
func TestTheConfigLivesInDotConfig(t *testing.T) {
	t.Setenv("TLOG_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".config", "tlog", "config.toml")
	if got := Path(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if got := Path(); got != "/xdg/tlog/config.toml" {
		t.Fatalf("XDG_CONFIG_HOME ignored: %q", got)
	}
	t.Setenv("TLOG_CONFIG", "/explicit/tlog.toml")
	if got := Path(); got != "/explicit/tlog.toml" {
		t.Fatalf("TLOG_CONFIG ignored: %q", got)
	}
}
