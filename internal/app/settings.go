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
	remoteNote := "im Repository der Notizen, nicht in tlog"
	if remote == "" {
		remoteNote = "noch keins — ohne Remote kann nichts gepusht werden"
	}
	return []Setting{
		{"notes", c.Notes, "text",
			"Wo die Notizen liegen", "$TLOG_DIR sticht das aus", "config"},
		{"startup", c.Startup, "text",
			"Womit tlog öffnet", "today oder last", "config"},

		{"attachments.dir", c.Attachments.Dir, "text",
			"Das Regal, geteilt mit att", "$ATT_DIR sticht das aus", "config"},
		{"attachments.sanitize", boolStr(c.Attachments.Sanitize), "bool",
			"Dateinamen beim Ablegen aufräumen",
			"att lässt Namen wie sie sind — beide schreiben hierhin", "config"},
		{"attachments.lowercase", boolStr(c.Attachments.Lowercase), "bool",
			"…und klein schreiben", "nur wirksam, wenn sanitize an ist", "config"},

		{"git.autocommit", boolStr(c.Git.AutoCommit), "bool",
			"Beim Schreiben committen",
			"aus: erst bei tlog push oder beim Beenden", "config"},
		{"git.debounce", c.Git.Debounce, "duration",
			"Ruhe vor einem Commit", "", "config"},
		{"git.autopush", boolStr(s.Git.AutoPush()), "bool",
			"Nach jedem Commit pushen",
			"im Repository der Notizen, nicht in tlog", "notes"},
		{"git.remote", remote, "text",
			"Wohin gepusht wird", remoteNote, "notes"},

		{"watch.enabled", boolStr(c.Watch.Enabled), "bool",
			"Änderungen von aussen übernehmen",
			"Getipptes wird nie verworfen — nur gemeldet", "config"},
		{"watch.interval", c.Watch.Interval, "duration",
			"Wie oft nachgesehen wird", "", "config"},

		{"format.blank_lines", boolStr(c.Format.BlankLines), "bool",
			"Leerzeile zwischen Blöcken",
			"formatiert jede Datei beim nächsten Schreiben neu", "config"},
		{"deadline.property", c.Deadline.Property, "text",
			"Property für Fälligkeiten",
			"gelesen wird jede Schreibweise, auch due", "config"},
		{"dates.end_of_week", c.Dates.EndOfWeek, "text",
			"Was „eow“ bedeutet", "friday oder sunday", "config"},
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
		return "wirkt beim nächsten Start"
	case after.Format.BlankLines != before.Format.BlankLines:
		return "Dateien werden beim nächsten Schreiben neu formatiert"
	case after.Watch != before.Watch:
		return "wirkt beim nächsten Start"
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
