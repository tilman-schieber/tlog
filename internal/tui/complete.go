package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	tapp "github.com/tilman-schieber/tlog/internal/app"
	"github.com/tilman-schieber/tlog/internal/dates"
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

	// A slash command is a different kind of completion: it has a menu of its
	// own, and what it inserts is an effect rather than text.
	cmds  []tapp.Command
	arg   string
	start int // rune offset where the "/" sits
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
	if m.slashCompletion() {
		return
	}
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

// slashCompletion opens the command menu when a "/" has been typed at the start
// of a word. It reports whether it took over.
func (m *Model) slashCompletion() bool {
	before := m.ed.textBefore()
	start, name, arg, ok := slashAt(before)
	if !ok {
		return false
	}
	cmds := tapp.MatchCommands(name)
	if len(cmds) == 0 {
		m.comp = nil
		return false // let it be ordinary text: not every slash is a command
	}
	items := make([]string, 0, len(cmds))
	for _, c := range cmds {
		label := c.Title
		if c.TakesDate() {
			if d, ok := tapp.PreviewDate(arg); ok {
				label = c.Title + "  → " + dates.Short(d) + "  " + dates.Describe(d, time.Now())
			} else {
				label = c.Title + "  " + c.Hint
			}
		} else {
			label = c.Title + "  " + c.Hint
		}
		items = append(items, label)
	}
	sel := 0
	if m.comp != nil && m.comp.cmds != nil && m.comp.sel < len(items) {
		sel = m.comp.sel
	}
	m.comp = &completion{active: true, items: items, cmds: cmds, arg: arg, start: start, sel: sel}
	return true
}

// slashAt finds a command being typed to the left of the caret: a "/" at the
// start of a word, the command name, and anything after the first space as its
// argument.
func slashAt(before string) (start int, name, arg string, ok bool) {
	r := []rune(before)
	i := len(r) - 1
	for ; i >= 0; i-- {
		if r[i] == '/' {
			break
		}
		if r[i] == '\n' {
			return 0, "", "", false
		}
	}
	if i < 0 {
		return 0, "", "", false
	}
	// A slash only starts a command at the start of a word, so a URL or a path
	// never opens the menu.
	if i > 0 && r[i-1] != ' ' && r[i-1] != '\t' {
		return 0, "", "", false
	}
	rest := string(r[i+1:])
	if strings.ContainsAny(rest, "/\\") {
		return 0, "", "", false
	}
	name, arg, _ = strings.Cut(rest, " ")
	if name == "" && arg == "" && rest != "" {
		return 0, "", "", false
	}
	return i, name, arg, true
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
		if m.comp.cmds != nil {
			m.runCommand()
			return true, nil
		}
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

// runCommand applies the selected slash command to the block being edited. The
// command and its argument are cut out by the core, so both adapters cut
// identically.
func (m *Model) runCommand() {
	if m.comp == nil || m.comp.sel >= len(m.comp.cmds) || m.edBlock == nil {
		return
	}
	cmd := m.comp.cmds[m.comp.sel]

	text := m.ed.String()
	from := m.comp.start
	to := len([]rune(m.ed.textBefore()))

	// The core addresses a block by where it is in the file, so the block has
	// to be in the file first: a block just created by enter is only in memory,
	// and its offset is whatever it was when the document was last parsed.
	index := -1
	for i, b := range m.doc.Doc.Flatten() {
		if b == m.edBlock {
			index = i
			break
		}
	}
	m.edBlock.Text = text
	m.save()
	if m.errMsg != "" {
		m.comp = nil
		return
	}
	if err := m.load(m.doc.Rel); err != nil {
		m.errMsg = err.Error()
		m.comp = nil
		return
	}
	flat := m.doc.Doc.Flatten()
	if index < 0 || index >= len(flat) {
		m.errMsg = "lost track of the block the command was typed in"
		m.comp = nil
		return
	}

	res, err := m.svc.RunCommand(
		tapp.Addr{Rel: m.doc.Rel, Offset: flat[index].Start, Hash: m.doc.Hash},
		cmd.Name, m.comp.arg, text, from, to,
	)
	if err != nil {
		m.errMsg = err.Error()
		m.comp = nil
		return
	}

	m.comp = nil
	if lerr := m.load(res.Rel); lerr != nil {
		m.errMsg = lerr.Error()
		return
	}
	if b := m.doc.Doc.FindByOffset(res.Offset); b != nil {
		m.focus(b)
		m.startInsert(true)
		m.ed.cur = min(res.Caret, len(m.ed.runes))
	}
	m.status = "/" + cmd.Name
}
