package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	tapp "github.com/tilman-schieber/tlog/internal/app"
	"github.com/tilman-schieber/tlog/internal/dates"
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
	files []tapp.AttachView
	// refs is the (( menu: blocks to point at. Accepting one gives that block
	// a durable name, which is a write to another file, so it is kept here
	// rather than reduced to a string like the other candidates.
	refs []tapp.BlockRef
	// dates is the quick pick for a date, as the argument of /date and
	// /deadline. Accepting one writes its ISO form, whatever was typed to
	// find it.
	dates []tapp.DateChoice
	// mentions is the @ menu: people, days and pages, each written as a link.
	mentions []tapp.Mention
	arg      string
	start    int // rune offset where the "/" sits
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
	if m.slashCompletion() || m.refCompletion() || m.mentionCompletion() {
		return
	}
	prefix, ok := linkPrefix(m.ed.textBefore())
	if !ok {
		m.comp = nil
		return
	}
	l := m.snapshot()
	if l == nil {
		m.comp = nil
		return
	}
	// A # after the page name starts a tag: [[Andreas #person]] says what
	// Andreas is, at the moment you first mention them.
	if i := strings.LastIndexAny(prefix, " \t"); i >= 0 && strings.HasPrefix(prefix[i+1:], "#") {
		tagPrefix := prefix[i+2:]
		m.comp = &completion{
			active: true,
			prefix: tagPrefix,
			items:  l.Tags(tagPrefix, completionLimit),
		}
		return
	}
	// Candidates include pages that are linked to but not yet created, so that
	// a name you have used before completes before the file exists.
	//
	// An empty candidate list still opens the popup, showing a hint instead of
	// nothing: on a fresh notes directory there is nothing to suggest, and
	// silence there is indistinguishable from the feature not existing.
	items := l.Pages(prefix, completionLimit)
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

// refCompletion opens the block-reference menu when (( has been typed. It
// reports whether it took over.
//
// (( is free in the dialect and is what Logseq uses, so the habit carries. The
// candidates are the same search the palette runs: what you can find, you can
// point at.
func (m *Model) refCompletion() bool {
	prefix, ok := refPrefix(m.ed.textBefore())
	if !ok {
		return false
	}
	l := m.snapshot()
	if l == nil {
		return false
	}
	refs := l.Blocks(prefix, completionLimit)
	items := make([]string, 0, len(refs))
	for _, r := range refs {
		items = append(items, oneLine(r.Text)+"  "+styleMuted.Render(r.Page))
	}
	sel := 0
	if m.comp != nil && m.comp.refs != nil && m.comp.sel < len(items) {
		sel = m.comp.sel
	}
	m.comp = &completion{active: true, prefix: prefix, items: items, refs: refs, sel: sel}
	return true
}

// mentionCompletion opens the @ menu when an @ starts a word: the people you
// write about, then the days the typing could mean, then other pages, and a
// new person when nobody matches. Whatever is chosen is written as an ordinary
// link. When nothing could be meant the menu closes and the @ is just a
// character, so an address like a@b.ch is never interrupted.
func (m *Model) mentionCompletion() bool {
	prefix, ok := atPrefix(m.ed.textBefore())
	if !ok {
		return false
	}
	l := m.snapshot()
	if l == nil {
		return false
	}
	found := l.Mentions(prefix, completionLimit)
	if len(found) == 0 {
		return false
	}
	sel := 0
	if m.comp != nil && m.comp.mentions != nil && m.comp.moved && m.comp.sel < len(found) {
		sel = m.comp.sel
	}
	m.comp = &completion{active: true, prefix: prefix, items: mentionLabels(found), mentions: found, sel: sel, moved: m.comp != nil && m.comp.moved}
	return true
}

// mentionLabels marks each entry with what it is, so a person, a day and a
// page are told apart before a word of them is read.
func mentionLabels(found []tapp.Mention) []string {
	items := make([]string, 0, len(found))
	for _, f := range found {
		mark := map[string]string{"person": "", "date": "󰃭", "page": "󰈙", "new": "󰐕"}[f.Kind]
		if f.Kind == "new" {
			items = append(items, mark+" new person: "+f.Name)
			continue
		}
		items = append(items, fmt.Sprintf("%s %-24s %s", mark, f.Name, styleMuted.Render(f.Detail)))
	}
	return items
}

func dateLabels(choices []tapp.DateChoice) []string {
	items := make([]string, 0, len(choices))
	for _, c := range choices {
		items = append(items, fmt.Sprintf("%-14s %s  %s", c.Phrase, c.Short, styleMuted.Render(c.Label)))
	}
	return items
}

// atPrefix is what has been typed after an @ that starts a word, on this line.
func atPrefix(before string) (string, bool) {
	r := []rune(before)
	i := len(r) - 1
	for ; i >= 0 && r[i] != '@'; i-- {
		if r[i] == '\n' || len(r)-i > 24 {
			return "", false
		}
	}
	if i < 0 || (i > 0 && !strings.ContainsRune(" \t\n(", r[i-1])) {
		return "", false
	}
	return string(r[i+1:]), true
}

func (m *Model) acceptMention() {
	if m.comp.sel >= len(m.comp.mentions) {
		m.comp = nil
		return
	}
	f := m.comp.mentions[m.comp.sel]
	m.ed.replaceBefore(len([]rune(m.comp.prefix))+1, f.Insert)
	m.comp = nil
	if f.Kind == "new" {
		m.status = f.Name + " is a person now — their page lists every mention"
	}
}

// refPrefix is an unclosed (( to the left of the caret.
func refPrefix(before string) (string, bool) {
	open := strings.LastIndex(before, "((")
	if open < 0 {
		return "", false
	}
	rest := before[open+2:]
	if strings.ContainsAny(rest, "\n") || strings.Contains(rest, "))") {
		return "", false
	}
	return rest, true
}

// oneLine flattens a block for a menu row: a reference candidate may be a
// multi-line block, and a menu is one line per candidate.
func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len([]rune(s)) > 60 {
		s = string([]rune(s)[:57]) + "…"
	}
	return s
}

// acceptRef gives the chosen block a durable name and types a link to it. The
// anchor is written now and not before, which is the whole point of lazy
// anchors: a corpus nobody has referred to has none.
func (m *Model) acceptRef() {
	if m.comp.sel >= len(m.comp.refs) {
		m.comp = nil
		return
	}
	r := m.comp.refs[m.comp.sel]
	link, err := m.svc.RefTo(r.Addr)
	if err != nil {
		m.fail(err)
		m.comp = nil
		return
	}
	// The (( goes too: what stays in the file is an ordinary link.
	m.ed.replaceBefore(len([]rune(m.comp.prefix))+2, link)
	m.comp = nil
	m.status = "referring to a block on " + r.Page
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
		label := c.Title + "  " + c.Hint
		if c.TakesDate() {
			if d, ok := tapp.PreviewDate(arg); ok {
				label = c.Title + "  → " + dates.Short(d) + "  " + dates.Describe(d, time.Now())
			}
		}
		items = append(items, label)
	}

	// A date command shows the days themselves, the same quick pick as @:
	// /deadline alone offers today, tomorrow, Friday…, and "/deadline in 3"
	// narrows it.
	if len(cmds) == 1 && cmds[0].TakesDate() {
		if choices := tapp.DateChoices(arg, completionLimit); len(choices) > 0 {
			sel := 0
			if m.comp != nil && m.comp.dates != nil && m.comp.sel < len(choices) {
				sel = m.comp.sel
			}
			m.comp = &completion{
				active: true, items: dateLabels(choices), cmds: cmds, dates: choices,
				arg: arg, start: start, sel: sel,
			}
			return true
		}
	}

	// A file command shows the attachments themselves: what you are choosing between is
	// the attachments, not a menu entry called "attachment".
	if len(cmds) == 1 && cmds[0].TakesAttachment() {
		found, err := m.svc.Attachments(arg, 8)
		if err == nil && len(found) > 0 {
			items = items[:0]
			for _, f := range found {
				items = append(items, f.Name+"  "+styleMuted.Render(f.Size+"  "+f.When))
			}
			sel := 0
			if m.comp != nil && m.comp.files != nil && m.comp.sel < len(items) {
				sel = m.comp.sel
			}
			m.comp = &completion{
				active: true, items: items, cmds: cmds, files: found,
				arg: arg, start: start, sel: sel,
			}
			return true
		}
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
		switch {
		case m.comp.cmds != nil:
			m.runCommand()
		case m.comp.mentions != nil:
			m.acceptMention()
		case m.comp.refs != nil:
			m.acceptRef()
		default:
			m.acceptCompletion()
		}
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
	closing := m.comp.closing
	if closing != "" && strings.HasPrefix(m.ed.textAfter(), closing) {
		// The brackets closed themselves when [[ was typed: step past them
		// rather than adding another pair.
		m.ed.replaceBefore(len([]rune(m.comp.prefix)), name)
		m.ed.cur += len([]rune(closing))
	} else {
		m.ed.replaceBefore(len([]rune(m.comp.prefix)), name+closing)
	}
	m.comp = nil
}

// runCommand applies the selected slash command to the block being edited. The
// command and its argument are cut out by the core, so both adapters cut
// identically.
func (m *Model) runCommand() {
	// The selection indexes the rows shown, which are commands, files on the
	// attachments or days — not always the commands, of which there may be one.
	if m.comp == nil || len(m.comp.cmds) == 0 || m.edBlock == nil {
		return
	}
	if rows := max(len(m.comp.items), len(m.comp.cmds)); m.comp.sel >= rows {
		return
	}
	cmd := m.comp.cmds[0]
	arg := m.comp.arg
	if m.comp.files != nil {
		// The menu was the attachments, so the selection names the file.
		if m.comp.sel < len(m.comp.files) {
			arg = m.comp.files[m.comp.sel].Name
		}
	} else if m.comp.dates != nil {
		// The menu was the days, so the selection is the date.
		if m.comp.sel < len(m.comp.dates) {
			arg = m.comp.dates[m.comp.sel].ISO
		}
	} else {
		cmd = m.comp.cmds[m.comp.sel]
	}

	text := m.ed.String()
	from := m.comp.start
	to := len([]rune(m.ed.textBefore()))

	// The command runs against the file, so what has been typed goes in first.
	a, ok := m.commitText()
	m.comp = nil
	if !ok {
		return
	}

	res, err := m.svc.RunCommand(a, cmd.Name, arg, text, from, to)
	if err != nil {
		m.fail(err)
		return
	}
	if !m.apply(res.Result, nil) {
		return
	}
	m.attach(res.Caret)
	m.status = "/" + cmd.Name
}
