package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/tilman-schieber/tlog/internal/app"
	"github.com/tilman-schieber/tlog/internal/store"
)

// The header names the last few pages you were on, so going back to one is a
// keypress or a click rather than a search. The core remembers the list across
// restarts (app.Service.Visited); this file is how it is drawn and stepped
// through.

const (
	recentName = 24 // cells per name before it is shortened

	// keyRecent steps to the next older recent page, wrapping. It is a key
	// every terminal sends as it is, so it needs no terminal configuration;
	// ctrl+tab was tried and cannot reach a program without it.
	keyRecent = "ctrl+n"
)

// recentSpan is where one recent page's name was drawn in the header.
type recentSpan struct {
	start, end int // cells, end exclusive
	rel        string
}

// touch puts a page at the front of the recent list, and the core remembers it.
func (m *Model) touch(rel string) {
	list, err := m.svc.Visited(rel)
	m.recent = list
	if err != nil {
		m.status = "could not remember the recent pages: " + err.Error()
	}
}

// cycleRecent steps through the recent pages. The order holds still while you
// keep stepping — otherwise every step would put the page you landed on first
// and the next step could only ever go back — and the page you stop on moves
// to the front once you do something else (settleRecent). The first step from
// a settled list is therefore the page you were on before this one.
func (m *Model) cycleRecent() {
	if len(m.recent) < 2 {
		m.status = "no other recent page yet"
		return
	}
	if !m.cycling {
		m.cycling = true
		m.cycleAt = max(0, m.recentIndex(m.doc.Rel))
	}
	m.cycleAt = (m.cycleAt + 1) % len(m.recent)
	m.goTo(m.recent[m.cycleAt])
}

// settleRecent ends a run of cycling: wherever it stopped is now most recent.
func (m *Model) settleRecent() {
	if !m.cycling {
		return
	}
	m.cycling = false
	if m.doc != nil {
		m.touch(m.doc.Rel)
	}
}

func (m *Model) recentIndex(rel string) int {
	for i, r := range m.recent {
		if r == rel {
			return i
		}
	}
	return -1
}

// recentStrip draws the recent pages, the current one as the title and the
// rest muted, keeping at most keep of them. It also records where each name
// landed, so the header can be clicked.
func (m *Model) recentStrip(keep int) (string, []recentSpan) {
	cur := ""
	if m.doc != nil {
		cur = m.doc.Rel
	}
	rels := m.recent
	if len(rels) == 0 && cur != "" {
		rels = []string{cur}
	}

	sep := styleMuted.Render(" · ")
	sepW := lipgloss.Width(sep)

	var b strings.Builder
	var spans []recentSpan
	w := 0
	shown := 0
	for _, rel := range rels {
		if shown >= keep && rel != cur {
			continue
		}
		name := truncate(store.Title(rel), recentName)
		if rel == app.JournalsRel {
			name = "Journals"
		}
		if shown > 0 {
			b.WriteString(sep)
			w += sepW
		}
		if rel == cur {
			b.WriteString(styleTitle.Render(name))
		} else {
			b.WriteString(styleMuted.Render(name))
		}
		nw := lipgloss.Width(name)
		spans = append(spans, recentSpan{start: w, end: w + nw, rel: rel})
		w += nw
		shown++
	}
	return b.String(), spans
}

// clickHeader goes to the recent page whose name is under the pointer.
func (m *Model) clickHeader(x int) {
	for _, s := range m.headerHits {
		if x < s.start || x >= s.end {
			continue
		}
		if m.doc != nil && s.rel == m.doc.Rel {
			return
		}
		if m.mode == modeInsert {
			m.commitInsert()
			if m.mode != modeNormal {
				return
			}
		}
		m.goTo(s.rel)
		return
	}
}
