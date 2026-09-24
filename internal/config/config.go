// Package config is the handful of choices that are genuinely personal.
//
// Most of tlog's behaviour is not configurable on purpose — ISO dates, files as
// truth, compare-and-swap writes and the small dialect are decisions, not
// preferences, and a setting for each would only be a way to break them. What
// is here is where two reasonable people would want different things.
//
// Precedence is: a command-line flag, then the environment, then this file,
// then the default. Nothing is written until something asks for it to be.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Config is the whole of it.
type Config struct {
	Notes       string      `toml:"notes"`
	Startup     string      `toml:"startup"`
	Attachments Attachments `toml:"attachments"`
	Git         Git         `toml:"git"`
	Format      Format      `toml:"format"`
	Deadline    Deadline    `toml:"deadline"`
	Dates       Dates       `toml:"dates"`
}

// Attachments is the shelf shared with att.
type Attachments struct {
	Dir string `toml:"dir"`
	// Sanitize renames a file on the way in: "meeting notes.pdf" becomes
	// "meeting-notes.pdf". Off by default, because att's rule is that files
	// keep their names and both tools write to this directory.
	Sanitize bool `toml:"sanitize"`
	// Lowercase goes further, and only applies when Sanitize is on.
	Lowercase bool `toml:"lowercase"`
}

// Git is how eagerly the notes are committed.
type Git struct {
	// AutoCommit commits as you write. With it off nothing is committed until
	// something asks — `tlog push`, or quitting the outliner.
	AutoCommit bool `toml:"autocommit"`
	// Debounce is how long writing must be idle before a commit.
	//
	// Pushing and the remote are not here. They belong to the notes directory
	// and live in its own git config, so that a copy of the notes carries the
	// answer with it and there is never a second place saying otherwise. Both
	// are still shown and set through tlog's settings.
	Debounce string `toml:"debounce"`
}

// Format is the canonical shape of a file tlog writes.
type Format struct {
	// BlankLines puts a blank line between top-level blocks. Changing it
	// reformats each file the next time that file is written.
	BlankLines bool `toml:"blank_lines"`
}

// Deadline is where a due date is stored.
type Deadline struct {
	// Property is the block property a deadline is written to. Reading accepts
	// any casing and "due" as well, whatever this says.
	Property string `toml:"property"`
}

// Dates is how typed shorthand is understood.
type Dates struct {
	// EndOfWeek is what "eow" means. A week ends when the work does, for most
	// people, which is why the default is friday rather than sunday.
	EndOfWeek string `toml:"end_of_week"`
}

// Default is what tlog does when nothing says otherwise.
func Default() Config {
	return Config{
		Notes:       "~/notes",
		Startup:     "today",
		Attachments: Attachments{Dir: "~/.att"},
		Git:         Git{AutoCommit: true, Debounce: "30s"},
		Format:      Format{BlankLines: true},
		Deadline:    Deadline{Property: "Deadline"},
		Dates:       Dates{EndOfWeek: "friday"},
	}
}

// Path is where the file lives: $TLOG_CONFIG, then $XDG_CONFIG_HOME, then
// ~/.config.
//
// Deliberately ~/.config on macOS too, rather than Library/Application Support.
// Everything else here already lives there — nvim, fish, kitty, aerospace,
// starship — and the same dotfiles are stowed onto an Arch machine, where a
// second location would be one more thing to keep in step.
func Path() string {
	if p := os.Getenv("TLOG_CONFIG"); p != "" {
		return p
	}
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(".config", "tlog", "config.toml")
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "tlog", "config.toml")
}

// Load reads the file, filling anything it does not mention from the defaults.
// A missing file is not an error: it means every answer is the default.
func Load() (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(Path())
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Default(), fmt.Errorf("%s: %w", Path(), err)
	}
	return cfg, nil
}

// Exists reports whether a file has been written yet.
func Exists() bool {
	_, err := os.Stat(Path())
	return err == nil
}

// NotesDir is the notes directory, after the environment has had its say.
func (c Config) NotesDir() string {
	if d := os.Getenv("TLOG_DIR"); d != "" {
		return d
	}
	return expand(c.Notes)
}

// AttachDir is the shared shelf, after the environment has had its say.
func (c Config) AttachDir() string {
	if d := os.Getenv("ATT_DIR"); d != "" {
		return d
	}
	return expand(c.Attachments.Dir)
}

// DebounceDuration is the commit delay, falling back to the default rather than
// failing on a value nobody can parse.
func (c Config) DebounceDuration() time.Duration {
	d, err := time.ParseDuration(c.Git.Debounce)
	if err != nil || d <= 0 {
		return 30 * time.Second
	}
	return d
}

// DeadlineProperty is the property name, never empty.
func (c Config) DeadlineProperty() string {
	if p := strings.TrimSpace(c.Deadline.Property); p != "" {
		return p
	}
	return "Deadline"
}

func expand(p string) string {
	p = strings.TrimSpace(p)
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/"))
		}
	}
	return p
}

// Save writes the file, creating the directory if needed.
//
// The whole file is rendered from the settings, with the explanations below, so
// that reading it is how you find out what can be configured. Comments added by
// hand are replaced; the values are not.
func (c Config) Save() error {
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tlog-config-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(c.render()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func (c Config) render() string {
	var b strings.Builder
	b.WriteString(`# tlog
#
# Most of what tlog does is not configurable on purpose: ISO dates, files as
# the source of truth, and the small markdown dialect are decisions rather
# than preferences. What is here is where two reasonable people would want
# different things.
#
# A command-line flag beats the environment, which beats this file.

# Where the notes live. $TLOG_DIR overrides it.
`)
	fmt.Fprintf(&b, "notes = %q\n\n", c.Notes)

	b.WriteString("# What opens on start: today's journal, or whatever was written last.\n")
	fmt.Fprintf(&b, "startup = %q\n\n", c.Startup)

	b.WriteString(`[attachments]
# The shelf, shared with att. $ATT_DIR overrides it.
`)
	fmt.Fprintf(&b, "dir = %q\n", c.Attachments.Dir)
	b.WriteString(`
# Rename a file on the way in: "meeting notes.pdf" becomes
# "meeting-notes.pdf". Off by default, because att's rule is that files keep
# their names and att writes to this same directory — turning this on means
# the two tools name things differently.
`)
	fmt.Fprintf(&b, "sanitize = %v\n", c.Attachments.Sanitize)
	b.WriteString("\n# Go further and lowercase the name. Only applies when sanitize is on.\n")
	fmt.Fprintf(&b, "lowercase = %v\n\n", c.Attachments.Lowercase)

	b.WriteString(`[git]
# Commit as you write. With this off nothing is committed until something
# asks: tlog push, or quitting the outliner.
`)
	fmt.Fprintf(&b, "autocommit = %v\n", c.Git.AutoCommit)
	b.WriteString(`
# How long writing must be idle before a commit.
`)
	fmt.Fprintf(&b, "debounce = %q\n", c.Git.Debounce)
	b.WriteString(`
# Pushing and the remote are deliberately not here. They belong to the notes
# directory and live in its own git config — tlog.autopush and remote.origin —
# so that a copy of the notes carries the answer with it and there is never a
# second place saying otherwise. Both are shown and set through tlog's
# settings, or with: tlog config git.remote <url>
`)
	b.WriteString("\n")

	b.WriteString(`[format]
# A blank line between top-level blocks. Changing this reformats each file
# the next time that file is written — the content is untouched, but the
# diff will be large.
`)
	fmt.Fprintf(&b, "blank_lines = %v\n\n", c.Format.BlankLines)

	b.WriteString(`[deadline]
# The block property a deadline is written to. Reading accepts any casing,
# and "due" as well, whatever this says.
`)
	fmt.Fprintf(&b, "property = %q\n\n", c.Deadline.Property)

	b.WriteString(`[dates]
# What "eow" means when typed after /deadline. A week ends when the work
# does, for most people, which is why this is friday rather than sunday.
`)
	fmt.Fprintf(&b, "end_of_week = %q\n", c.Dates.EndOfWeek)
	return b.String()
}
