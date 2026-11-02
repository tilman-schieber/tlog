package app

import (
	"sort"

	"github.com/tilman-schieber/tlog/internal/store"
)

// The journals page is every day, newest first, each one still its own file.
// It is a way of looking, not a file: nothing about storage changes, every
// block on it is addressed in the day it belongs to, and so every edit made
// there is an edit to that one day — with that day's hash check. A day's edge
// is therefore a wall for free: indent, move and merge are single-file
// operations, and the core refuses them at the first or last block of a file
// exactly as it does on the day's own page.

// JournalsRel names the journals page wherever a page is named by path — the
// recent list, history, the picker. It is the journals directory itself, so it
// can never collide with a note.
const JournalsRel = store.JournalsDir

// Day is one day on the journals page: the document to edit, and what the
// rest of the notes say about it.
type Day struct {
	Doc  *Doc
	View *PageView
}

// JournalDays lists the days on the journals page, newest first: every day
// that has a file, and today whether it has one or not, so that writing today's
// first note does not need a detour.
func (s *Service) JournalDays() ([]string, error) {
	g, err := s.Graph()
	if err != nil {
		return nil, err
	}
	today := s.TodayRel()
	rels := []string{today}
	for _, p := range g.Journals() {
		if p.Rel != today {
			rels = append(rels, p.Rel)
		}
	}
	// ISO dates sort as text. A day written ahead — a plan for next week —
	// sorts above today, as it would in any calendar.
	sort.Slice(rels, func(i, j int) bool { return rels[i] > rels[j] })
	return rels, nil
}

// OpenDays opens several days at once with one graph build, which is what
// keeps the journals page as cheap as one page.
func (s *Service) OpenDays(rels []string) ([]Day, error) {
	g, err := s.Graph()
	if err != nil {
		return nil, err
	}
	out := make([]Day, 0, len(rels))
	for _, rel := range rels {
		d, err := s.Load(rel)
		if err != nil {
			return nil, err
		}
		out = append(out, Day{Doc: d, View: s.viewWith(g, d)})
	}
	return out, nil
}
