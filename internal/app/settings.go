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
}

// Settings describes the current configuration.
func Settings(c config.Config) []Setting {
	return []Setting{
		{"notes", c.Notes, "text", "Wo die Notizen liegen", "$TLOG_DIR sticht das aus"},
		{"attachments.dir", c.Attachments.Dir, "text", "Das Regal, geteilt mit att", "$ATT_DIR sticht das aus"},
		{"attachments.sanitize", boolStr(c.Attachments.Sanitize), "bool",
			"Dateinamen beim Ablegen aufräumen", "att lässt Namen wie sie sind — beide schreiben hierhin"},
		{"attachments.lowercase", boolStr(c.Attachments.Lowercase), "bool",
			"…und klein schreiben", "nur wirksam, wenn sanitize an ist"},
		{"git.debounce", c.Git.Debounce, "duration", "Ruhe vor einem Commit", ""},
		{"format.blank_lines", boolStr(c.Format.BlankLines), "bool",
			"Leerzeile zwischen Blöcken", "formatiert jede Datei beim nächsten Schreiben neu"},
		{"deadline.property", c.Deadline.Property, "text", "Property für Fälligkeiten",
			"gelesen wird jede Schreibweise, auch due"},
		{"dates.end_of_week", c.Dates.EndOfWeek, "text", "Was „eow“ bedeutet", "friday oder sunday"},
	}
}

// SetSetting applies one typed value, returning the configuration it would
// make. An unknown key or an unusable value is refused rather than ignored.
func SetSetting(c config.Config, key, value string) (config.Config, error) {
	v := strings.TrimSpace(value)
	switch key {
	case "notes":
		c.Notes = v
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
	case "git.debounce":
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return c, fmt.Errorf("%q is not a delay; try 30s or 2m", v)
		}
		c.Git.Debounce = v
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

// ToggleSetting flips a boolean, for a menu where enter means "the other one".
func ToggleSetting(c config.Config, key string) (config.Config, error) {
	for _, s := range Settings(c) {
		if s.Key == key && s.Kind == "bool" {
			return SetSetting(c, key, boolStr(s.Value != "true"))
		}
	}
	return c, fmt.Errorf("%s is not something to toggle", key)
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
