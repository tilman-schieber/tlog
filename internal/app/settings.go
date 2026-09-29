package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/tilman-schieber/tlog/internal/config"
)

// One description of every setting, shared by `tlog config`, the outliner's
// menu and the app's panel — so that the three cannot offer different things
// or disagree about what a value means.

// Setting is one configurable choice, as shown in a menu.
type Setting struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Kind  string `json:"kind"` // "bool", "text" or "duration"
	Hint  string `json:"hint"`
	Note  string `json:"note,omitempty"` // a consequence worth knowing

	// Source says where the answer is kept. Pushing and the remote belong to
	// the notes directory rather than to tlog, so that a copy of the notes
	// carries them along and no second file can disagree — but they are shown
	// and changed here like everything else.
	Source string `json:"source"` // "config" or "notes"
}

// Settings describes the current configuration, from both places it lives.
func (s *Service) Settings() []Setting {
	c := s.Cfg
	remote := s.Git.Remote()
	remoteNote := "kept in the notes repository, not in tlog"
	if remote == "" {
		remoteNote = "none yet — without a remote nothing can be pushed"
	}
	return []Setting{
		{"notes", c.Notes, "text",
			"Where the notes live", "$TLOG_DIR overrides this", "config"},
		{"startup", c.Startup, "text",
			"What tlog opens on", "today or last", "config"},

		{"attachments.dir", c.Attachments.Dir, "text",
			"The shelf, shared with att", "$ATT_DIR overrides this", "config"},
		{"attachments.sanitize", boolStr(c.Attachments.Sanitize), "bool",
			"Tidy filenames on the way in",
			"att leaves names as they are — both write here", "config"},
		{"attachments.lowercase", boolStr(c.Attachments.Lowercase), "bool",
			"…and lowercase them", "only has an effect when sanitize is on", "config"},

		{"git.autocommit", boolStr(c.Git.AutoCommit), "bool",
			"Commit as you write",
			"off: not until tlog push, or quitting", "config"},
		{"git.debounce", c.Git.Debounce, "duration",
			"How long writing must be idle before a commit", "", "config"},
		{"git.autopush", boolStr(s.Git.AutoPush()), "bool",
			"Push after every commit",
			"kept in the notes repository, not in tlog", "notes"},
		{"git.remote", remote, "text",
			"Where pushing goes", remoteNote, "notes"},

		{"watch.enabled", boolStr(c.Watch.Enabled), "bool",
			"Pick up edits made elsewhere",
			"what you are typing is never discarded, only reported", "config"},
		{"watch.interval", c.Watch.Interval, "duration",
			"How often the notes are checked", "", "config"},

		{"format.blank_lines", boolStr(c.Format.BlankLines), "bool",
			"A blank line between top-level blocks",
			"reformats each file the next time it is written", "config"},
		{"deadline.property", c.Deadline.Property, "text",
			"The property a deadline is written to",
			"reading accepts any casing, and due as well", "config"},
		{"dates.end_of_week", c.Dates.EndOfWeek, "text",
			"What “eow” means", "friday or sunday", "config"},
	}
}

// SetSetting applies one typed value and makes it stick, wherever that value
// lives. An unknown key or an unusable value is refused rather than ignored,
// and nothing is written until the value has been understood.
//
// It returns a note when the change cannot take effect yet, so that a setting
// never looks applied when it is not.
func (s *Service) SetSetting(key, value string) (string, error) {
	v := strings.TrimSpace(value)

	// The two that belong to the notes directory rather than to tlog.
	switch key {
	case "git.autopush":
		b, err := parseBool(v)
		if err != nil {
			return "", err
		}
		return "", s.Git.SetAutoPush(b)
	case "git.remote":
		return "", s.Git.SetRemote(v)
	}

	before := s.Cfg
	cfg, err := setConfigValue(before, key, v)
	if err != nil {
		return "", err
	}
	if err := s.Reconfigure(cfg); err != nil {
		return "", err
	}
	return consequence(before, cfg), nil
}

// ToggleSetting flips a boolean, for a menu where enter means "the other one".
func (s *Service) ToggleSetting(key string) (string, error) {
	for _, it := range s.Settings() {
		if it.Key == key && it.Kind == "bool" {
			return s.SetSetting(key, boolStr(it.Value != "true"))
		}
	}
	return "", fmt.Errorf("%s is not something to toggle", key)
}

// consequence says what a change does not do immediately.
func consequence(before, after config.Config) string {
	switch {
	case after.Notes != before.Notes || after.Attachments.Dir != before.Attachments.Dir:
		return "takes effect at the next start"
	case after.Format.BlankLines != before.Format.BlankLines:
		return "files are reformatted the next time they are written"
	case after.Watch != before.Watch:
		return "takes effect at the next start"
	}
	return ""
}

func setConfigValue(c config.Config, key, v string) (config.Config, error) {
	switch key {
	case "notes":
		c.Notes = v
	case "startup":
		l := strings.ToLower(v)
		if l != "today" && l != "last" {
			return c, fmt.Errorf("%q is not today or last", v)
		}
		c.Startup = l
	case "attachments.dir":
		c.Attachments.Dir = v
	case "attachments.sanitize":
		b, err := parseBool(v)
		if err != nil {
			return c, err
		}
		c.Attachments.Sanitize = b
	case "attachments.lowercase":
		b, err := parseBool(v)
		if err != nil {
			return c, err
		}
		c.Attachments.Lowercase = b
	case "git.autocommit":
		b, err := parseBool(v)
		if err != nil {
			return c, err
		}
		c.Git.AutoCommit = b
	case "git.debounce":
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return c, fmt.Errorf("%q is not a delay; try 30s or 2m", v)
		}
		c.Git.Debounce = v
	case "watch.enabled":
		b, err := parseBool(v)
		if err != nil {
			return c, err
		}
		c.Watch.Enabled = b
	case "watch.interval":
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return c, fmt.Errorf("%q is not an interval; try 1s or 500ms", v)
		}
		c.Watch.Interval = v
	case "format.blank_lines":
		b, err := parseBool(v)
		if err != nil {
			return c, err
		}
		c.Format.BlankLines = b
	case "deadline.property":
		if v == "" {
			return c, fmt.Errorf("a property needs a name")
		}
		c.Deadline.Property = v
	case "dates.end_of_week":
		l := strings.ToLower(v)
		if l != "friday" && l != "sunday" {
			return c, fmt.Errorf("%q is not friday or sunday", v)
		}
		c.Dates.EndOfWeek = l
	default:
		return c, fmt.Errorf("no such setting: %s", key)
	}
	return c, nil
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func parseBool(v string) (bool, error) {
	b, err := strconv.ParseBool(strings.ToLower(v))
	if err != nil {
		return false, fmt.Errorf("%q is not true or false", v)
	}
	return b, nil
}
