package app

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/tilman-schieber/tlog/internal/store"
)

// The pages you were last on are remembered across restarts, but not in the
// notes: the notes directory is committed and pushed, and where you happened to
// be looking is not something to commit. It goes to the XDG state directory
// instead, one list per notes directory, so pointing tlog at another directory
// does not show pages that are not there.
//
// This is a convenience, not data. It is written without a hash — the worst a
// race between two outliners can do is reorder a few names — and anything wrong
// with the file reads as an empty list rather than an error.

// The recent list is not simply the last few places: journal days would crowd
// out the pages, since every day is a new one. It holds the last RecentPages
// pages, the last journal day, and the journals page — newest first, so one
// step back is always the place you were just on. The last two are always
// there, the day falling back to today, because they are where you go most.
const (
	RecentPages = 4
	RecentMax   = RecentPages + 2
)

// recentPath is $TLOG_STATE, else $XDG_STATE_HOME/tlog/recent.json, else
// ~/.local/state/tlog/recent.json.
func recentPath() (string, error) {
	if p := os.Getenv("TLOG_STATE"); p != "" {
		return p, nil
	}
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "tlog", "recent.json"), nil
}

// readRecent is every notes directory's list. A missing or broken file is an
// empty one.
func readRecent(path string) map[string][]string {
	all := map[string][]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		return all
	}
	if json.Unmarshal(data, &all) != nil {
		return map[string][]string{}
	}
	return all
}

// Recent is the pages most recently arrived at in this notes directory, newest
// first. Pages deleted or renamed since are left out; a journal day is kept
// even without a file, since opening it is how it gets one.
func (s *Service) Recent() []string {
	path, err := recentPath()
	if err != nil {
		return s.withFixed(nil)
	}
	return s.withFixed(pickRecent(s.stillThere(readRecent(path)[s.Store.Root])))
}

// Visited puts a page at the front of the recent list and remembers it. The
// list is returned even when it could not be written, so the session still has
// it.
func (s *Service) Visited(rel string) ([]string, error) {
	path, err := recentPath()
	if err != nil {
		return s.withFixed([]string{rel}), err
	}
	all := readRecent(path)
	list := pickRecent(append([]string{rel}, s.stillThere(all[s.Store.Root])...))
	all[s.Store.Root] = list

	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return s.withFixed(list), err
	}
	return s.withFixed(list), writeAtomic(path, data)
}

// pickRecent keeps, in order, the first RecentPages pages, the first journal
// day and the journals page, dropping repeats.
func pickRecent(list []string) []string {
	var out []string
	pages, days, feed := 0, 0, 0
	seen := map[string]bool{}
	for _, rel := range list {
		if seen[rel] {
			continue
		}
		seen[rel] = true
		switch {
		case rel == JournalsRel:
			if feed++; feed > 1 {
				continue
			}
		case store.IsJournal(rel):
			if days++; days > 1 {
				continue
			}
		default:
			if pages++; pages > RecentPages {
				continue
			}
		}
		out = append(out, rel)
	}
	return out
}

// withFixed adds the journals page and a journal day when the list has not got
// them yet, at the end: they are there to be reached, not because they were
// recent.
func (s *Service) withFixed(list []string) []string {
	hasFeed, hasDay := false, false
	for _, rel := range list {
		hasFeed = hasFeed || rel == JournalsRel
		hasDay = hasDay || store.IsJournal(rel)
	}
	out := append([]string(nil), list...)
	if !hasDay {
		out = append(out, s.TodayRel())
	}
	if !hasFeed {
		out = append(out, JournalsRel)
	}
	return out
}

func (s *Service) stillThere(rels []string) []string {
	var out []string
	for _, rel := range rels {
		if store.IsJournal(rel) {
			out = append(out, rel)
			continue
		}
		if _, err := os.Stat(s.Store.Abs(rel)); !errors.Is(err, fs.ErrNotExist) {
			out = append(out, rel)
		}
	}
	return out
}

// writeAtomic replaces a file by renaming a complete one over it, so a reader
// never sees half a list.
func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".recent-*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
