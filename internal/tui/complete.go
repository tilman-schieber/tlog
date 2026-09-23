package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tilman-schieber/tlog/internal/graph"
)

// completion is the [[page]] autocomplete. It is driven from the same graph the
// CLI would use, so a completion source in any editor can be made to return
// exactly these candidates.
type completion struct {
	active bool
	prefix string
	items  []string
	sel    int
	// closing is appended when a candidate is accepted: page names finish the
	// link, tags do not, because you may want to add another.
	closing string
	// moved records that the selection was chosen deliberately with the arrow
	// keys. Until then, narrowing the query re-selects the best match, the way
	// every other completion menu behaves.
	moved bool
}

const completionLimit = 8

// updateCompletion re-evaluates the trigger after every keystroke. The trigger
// is an unclosed [[ to the left of the cursor.
func (m *Model) updateCompletion() {
	prefix, ok := linkPrefix(m.ed.textBefore())
	if !ok {
		m.comp = nil
		return
	}
	if m.g == nil {
		m.rebuildGraph()
	}
	if m.g == nil {
		m.rebuildGraph()
	}
	// A # after the page name starts a tag: [[Andreas #person]] says what
	// Andreas is, at the moment you first mention them.
	if i := strings.LastIndexAny(prefix, " \t"); i >= 0 && strings.HasPrefix(prefix[i+1:], "#") {
		tagPrefix := prefix[i+2:]
		m.comp = &completion{
			active: true,
			prefix: tagPrefix,
			items:  graph.FuzzyRank(m.g.TagNames(), tagPrefix, completionLimit),
		}
		return
	}
	// Candidates include pages that are linked to but not yet created, so that
	// a name you have used before completes before the file exists.
	//
	// An empty candidate list still opens the popup, showing a hint instead of
	// nothing: on a fresh notes directory there is nothing to suggest, and
	// silence there is indistinguishable from the feature not existing.
	items := graph.FuzzyRank(m.g.LinkTargets(), prefix, completionLimit)
	sel, moved := 0, false
	if m.comp != nil && m.comp.active && m.comp.moved {
		for i, it := range items {
			if it == m.currentCompletion() {
				sel, moved = i, true
				break
			}
		}
	}
	m.comp = &completion{active: true, prefix: prefix, items: items, sel: sel, moved: moved, closing: "]]"}
}

func (m *Model) currentCompletion() string {
	if m.comp == nil || m.comp.sel >= len(m.comp.items) {
		return ""
	}
	return m.comp.items[m.comp.sel]
}

// linkPrefix finds the text typed after the most recent unclosed [[.
func linkPrefix(before string) (string, bool) {
	open := strings.LastIndex(before, "[[")
	if open < 0 {
		return "", false
	}
	rest := before[open+2:]
	if strings.Contains(rest, "]]") || strings.Contains(rest, "\n") {
		return "", false
	}
	return rest, true
}

// completionKey handles the keys the popup owns while it is open. It returns
// false for everything else so typing continues to reach the editor.
//
// With no candidates the popup is only a hint, so it claims nothing but esc:
// enter must still start a new block rather than being swallowed by a menu
// that has nothing to offer.
func (m *Model) completionKey(k string) (bool, tea.Cmd) {
	if k == "esc" {
		m.comp = nil
		return true, nil
	}
	if len(m.comp.items) == 0 {
		return false, nil
	}
	switch k {
	case "up", "ctrl+p":
		if m.comp.sel > 0 {
			m.comp.sel--
			m.comp.moved = true
		}
		return true, nil
	case "down", "ctrl+n":
		if m.comp.sel < len(m.comp.items)-1 {
			m.comp.sel++
			m.comp.moved = true
		}
		return true, nil
	case "tab", "enter":
		m.acceptCompletion()
		return true, nil
	}
	return false, nil
}

func (m *Model) acceptCompletion() {
	name := m.currentCompletion()
	if name == "" {
		m.comp = nil
		return
	}
	m.ed.replaceBefore(len([]rune(m.comp.prefix)), name+m.comp.closing)
	m.comp = nil
}
