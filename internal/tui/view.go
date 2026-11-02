package tui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	tapp "github.com/tilman-schieber/tlog/internal/app"
	"github.com/tilman-schieber/tlog/internal/dates"
	"github.com/tilman-schieber/tlog/internal/markdown"
	"github.com/tilman-schieber/tlog/internal/store"
)

var (
	viewLinkRe   = regexp.MustCompile(`\[\[[^\[\]]+\]\]`)
	viewAnchorRe = regexp.MustCompile(`\[\[[^\[\]]*#\^[^\[\]]+\]\]`)
	viewTagRe    = regexp.MustCompile(`(^|\s)(#[A-Za-z][A-Za-z0-9_/\-]*)`)
	viewCheckRe  = regexp.MustCompile(`^\[( |x|X)\] `)
)

func (m *Model) View() string {
	if m.quitting {
		return ""
	}
	if m.mode == modeHelp {
		return m.helpView()
	}
	if m.mode == modeAgenda {
		return m.agendaView()
	}
	if m.mode == modeLog {
		return m.logView()
	}
	if m.mode == modeSettings {
		return m.settingsView()
	}

	var b strings.Builder
	b.WriteString(m.header())
	b.WriteByte('\n')

	lines, owners, cursorLine := m.bodyLines()
	h := m.viewHeight()
	top := m.top
	if cursorLine < top {
		top = cursorLine
	}
	if cursorLine >= top+h {
		top = cursorLine - h + 1
	}
	if top > len(lines)-h {
		top = len(lines) - h
	}
	if top < 0 {
		top = 0
	}
	m.top = top

	// What is drawn is also what a click is measured against: one entry per
	// screen line.
	m.hits = m.hits[:0]
	for i := top; i < len(lines) && i < top+h; i++ {
		b.WriteString(lines[i])
		b.WriteByte('\n')
		m.hits = append(m.hits, owners[i])
	}
	for i := len(lines) - top; i < h; i++ {
		b.WriteByte('\n')
	}

	if m.pick != nil {
		return m.pickerView()
	}
	b.WriteString(m.footer())
	return b.String()
}

// header is the recent pages on the left, the current one in the title colour,
// and what the current page is on the right. When the line is too narrow the
// aliases go first, then the tags, then the oldest of the recent pages; the
// current page and the mode are never dropped.
func (m *Model) header() string {
	kind := "page"
	switch {
	case m.feed != nil:
		kind = "every day"
	case store.IsJournal(m.doc.Rel):
		kind = "journal"
	}
	var tags, aka string
	if m.page != nil {
		for _, t := range m.page.Tags {
			tags += " " + styleTag.Render("#"+t)
		}
		// The other names this page answers to, so a [[link]] that worked is
		// not a mystery when you arrive.
		if a := m.page.Aliases; len(a) > 0 {
			aka = " " + styleMuted.Render("auch: "+strings.Join(a, ", "))
		}
	}
	mode := styleMuted.Render("NORMAL")
	if m.mode == modeInsert {
		mode = styleTodo.Render("INSERT")
	}
	if m.stale {
		mode = styleError.Render("STALE") + " " + mode
	}

	type fit struct {
		keep      int
		tags, aka bool
	}
	tries := []fit{{tapp.RecentMax, true, true}, {tapp.RecentMax, true, false}}
	for k := tapp.RecentMax; k >= 1; k-- {
		tries = append(tries, fit{k, false, false})
	}
	var left, right string
	for _, f := range tries {
		left, m.headerHits = m.recentStrip(f.keep)
		right = styleMuted.Render(kind)
		if f.tags {
			right += tags
		}
		if f.aka {
			right += aka
		}
		right += "  " + mode
		if m.width == 0 || lipgloss.Width(left)+lipgloss.Width(right)+1 <= m.width {
			break
		}
	}
	return pad(left, right, m.width)
}

func (m *Model) footer() string {
	switch {
	case m.errMsg != "":
		return styleError.Render(m.errMsg)
	case m.status != "":
		return styleMuted.Render(m.status)
	case m.mode == modeInsert:
		return styleMuted.Render("esc done · enter new block · alt+enter newline · tab indent · [[ links")
	default:
		if m.rowKind() == rowDay {
			return styleMuted.Render("enter open this day on its own · [ ] older · newer day · ↑↓ move")
		}
		if m.rowKind() != rowBlock {
			return styleMuted.Render("enter open · ↑↓ move · these live on other pages and are not edited here")
		}
		return styleMuted.Render("? help · enter new block · i edit · tab/shift+tab indent · ctrl+p open · / search · A agenda · q quit")
	}
}

// bodyLines renders every visible block into screen lines, says which row and
// which line of its text each one belongs to, and reports which screen line the
// cursor is on.
func (m *Model) bodyLines() ([]string, []hit, int) {
	var out []string
	var owners []hit
	cursorLine := 0

	for i, r := range m.rows {
		editing := m.mode == modeInsert && m.edBlock == r.block
		start := len(out)
		out = append(out, m.renderRow(r, i == m.cur, editing)...)
		textLines := 0
		if r.kind == rowBlock {
			text := r.block.Text
			if editing {
				text = m.ed.String()
			}
			textLines = strings.Count(text, "\n") + 1
		}
		for j := start; j < len(out); j++ {
			line := j - start
			if line >= textLines {
				line = -1
			}
			owner := i
			if r.kind != rowBlock && j < len(out)-1 {
				owner = -1 // the blank line above a heading is not the heading
			}
			owners = append(owners, hit{row: owner, line: line})
		}
		if i == m.cur {
			cursorLine = start
			if r.kind != rowBlock {
				cursorLine = len(out) - 1 // the heading, not the blank above it
			}
			if editing {
				line, _ := m.ed.cursorPos()
				cursorLine = start + line
			}
		}
	}
	if len(out) == 0 {
		out = append(out, styleMuted.Render("  (empty — press enter or click to write something)"))
		owners = append(owners, hit{row: -1, line: -1})
	}
	return out, owners, cursorLine
}

func (m *Model) renderRow(r row, selected, editing bool) []string {
	if r.kind != rowBlock {
		// A heading brings its own blank line above it. Each screen line is
		// its own entry, so that the window is measured in screen lines and
		// never drawn taller than the terminal — which would push the header
		// off the top.
		return strings.Split(m.renderSectionRow(r, selected), "\n")
	}
	indent := strings.Repeat("  ", r.depth)
	bullet := "•"
	quoted := r.block.Quote()
	if quoted {
		bullet = "❝"
	}
	if len(r.block.Children) > 0 {
		if m.collapsed[r.path] {
			bullet = "▸"
		} else {
			bullet = "▾"
		}
	}

	text := r.block.Text
	if editing {
		text = m.ed.String()
	}

	// Fenced regions are drawn from the same prepared form the desktop app
	// uses: highlighted, or laid out as a table when they are ```csv.
	if !editing && !selected && strings.Contains(text, "```") {
		if out := m.renderFenced(r, text, indent, bullet); out != nil {
			return out
		}
	}

	lines := strings.Split(text, "\n")

	var out []string
	inFence := false
	for i, l := range lines {
		var prefix string
		if i == 0 {
			prefix = indent + styleBullet.Render(bullet) + " "
		} else {
			prefix = indent + "  "
		}
		body := l
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			inFence = !inFence
			body = styleFence.Render(l)
		} else if inFence {
			body = styleFence.Render(l)
		} else if quoted {
			// A quotation is somebody else's words; the gutter says so without
			// needing a colour of its own.
			body = styleMuted.Render("│ " + strings.TrimPrefix(strings.TrimSpace(l), "> "))
		} else {
			body = m.highlight(l, r)
		}
		if selected && !editing {
			body = styleSelected.Render(stripANSI(l))
		}
		if editing {
			body = m.renderEditLine(l, i)
		}
		out = append(out, prefix+body)
	}

	if r.block.Anchor != "" && !editing {
		out[0] += " " + styleMuted.Render("^"+r.block.Anchor)
	}
	for _, p := range r.block.Props {
		if strings.EqualFold(p.Key, tapp.DeadlineProp) {
			continue // shown beside the text instead, with how it stands
		}
		out = append(out, indent+"  "+styleProp.Render(p.Key+":: "+p.Value))
	}
	if due, ok := tapp.Deadline(r.block); ok && !editing {
		style := styleMuted
		if r.block.Task() != markdown.TaskDone {
			switch dates.Status(due, time.Now()) {
			case dates.Overdue:
				style = styleError
			case dates.Today:
				style = styleTodo
			}
		}
		out[0] += " " + style.Render("⏰ "+dates.Short(due))
	}

	if editing && m.comp != nil && m.comp.active {
		out = append(out, m.completionLines(indent+"  ")...)
	}
	return out
}

// renderFenced draws a block containing fenced regions, replacing each fence
// with coloured code or an aligned table. It returns nil when the block turns
// out to have no fence after all.
func (m *Model) renderFenced(r row, text, indent, bullet string) []string {
	fences := markdown.Fences(text)
	if len(fences) == 0 {
		return nil
	}
	cont := indent + "  "

	var out []string
	lines := strings.Split(text, "\n")
	fi := 0
	first := true
	push := func(s string) {
		if first {
			out = append(out, indent+styleBullet.Render(bullet)+" "+s)
			first = false
			return
		}
		out = append(out, cont+s)
	}

	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(strings.TrimLeft(lines[i], " \t"), "```") {
			push(m.highlight(lines[i], r))
			continue
		}
		f := fences[min(fi, len(fences)-1)]
		fi++
		i++
		for i < len(lines) && !strings.HasPrefix(strings.TrimLeft(lines[i], " \t"), "```") {
			i++
		}
		if rows, ok := markdown.CSVTable(f.Lang, f.Body); ok && markdown.IsCSVLang(f.Lang) {
			for _, l := range csvGrid(rows) {
				push(l)
			}
			continue
		}
		if f.Lang != "" {
			push(styleMuted.Render(f.Lang))
		}
		for _, l := range strings.Split(highlightCode(f.Lang, f.Body), "\n") {
			push(styleMuted.Render("▏") + " " + l)
		}
	}

	if r.block.Anchor != "" && len(out) > 0 {
		out[0] += " " + styleMuted.Render("^"+r.block.Anchor)
	}
	for _, p := range r.block.Props {
		out = append(out, cont+styleProp.Render(p.Key+":: "+p.Value))
	}
	return out
}

// renderSectionRow draws the material below the outline: what this page groups
// as a tag, and what mentions it. These rows belong to other files, so they are
// shown but never edited here.
func (m *Model) renderSectionRow(r row, selected bool) string {
	switch r.kind {
	case rowDay:
		return m.dayHeading(r, selected)
	case rowHeader:
		rule := strings.Repeat("─", max(1, min(m.width, 72)-lipgloss.Width(r.text)-3))
		return "\n" + styleMuted.Render("── "+r.text+" "+rule)
	case rowLink:
		body := styleLink.Render(r.text)
		if selected {
			body = styleSelected.Render(r.text)
		}
		return "  " + body
	default:
		indent := strings.Repeat("  ", r.depth+1)
		text := truncate(r.text, max(10, min(m.width, 100)-len(indent)-4))
		body := highlightRaw(text)
		if selected {
			body = styleSelected.Render(text)
		}
		return indent + styleMuted.Render("·") + " " + body
	}
}

// renderEditLine draws one line of the block being edited, with the text cursor
// shown as an inverted cell.
func (m *Model) renderEditLine(l string, idx int) string {
	cline, ccol := m.ed.cursorPos()
	if idx != cline {
		return highlightRaw(l)
	}
	runes := []rune(l)
	if ccol > len(runes) {
		ccol = len(runes)
	}
	before := string(runes[:ccol])
	at := " "
	after := ""
	if ccol < len(runes) {
		at = string(runes[ccol])
		after = string(runes[ccol+1:])
	}
	return highlightRaw(before) + styleCursor.Render(at) + highlightRaw(after)
}

func (m *Model) completionLines(indent string) []string {
	if len(m.comp.items) == 0 {
		hint := "no page by that name yet — finish typing and press gf to create it"
		if m.comp.prefix == "" {
			hint = "type a page name · no pages yet"
		}
		return []string{indent + "  " + styleMuted.Render(hint)}
	}
	var out []string
	for i, it := range m.comp.items {
		line := indent + "  " + it
		if i == m.comp.sel {
			line = indent + "› " + styleSelected.Render(it)
		} else {
			line = indent + "  " + styleMuted.Render(it)
		}
		out = append(out, line)
	}
	return out
}

// highlight applies the semantic colour roles to inline syntax, resolving any
// block reference in the line to the block it points at.
func (m *Model) highlight(s string, r row) string {
	if s == "" {
		return s
	}
	// References are numbered across the whole block but drawn a line at a
	// time, so the count is shared. Lines before this one have already used up
	// theirs.
	var embeds []tapp.EmbedView
	if r.kind == rowBlock && r.block != nil {
		embeds = m.embeds[r.block]
	}
	cursor := &refCursor{embeds: embeds}
	cursor.skip(r, s)

	out := s
	if mark := viewCheckRe.FindString(out); mark != "" {
		rest := out[len(mark):]
		if strings.HasPrefix(mark, "[ ]") {
			return styleTodo.Render("☐ ") + highlightInline(rest, cursor)
		}
		return styleDone.Render("☑ " + rest)
	}
	return highlightInline(out, cursor)
}

// refCursor hands out a block's resolved references in the order they appear.
type refCursor struct {
	embeds []tapp.EmbedView
	i      int
}

func (c *refCursor) next() (tapp.EmbedView, bool) {
	if c.i >= len(c.embeds) {
		c.i++
		return tapp.EmbedView{}, false
	}
	e := c.embeds[c.i]
	c.i++
	return e, true
}

// skip advances past the references on earlier lines of the same block, so that
// the second line of a two-line block does not redraw the first line's.
func (c *refCursor) skip(r row, line string) {
	if r.kind != rowBlock || r.block == nil {
		return
	}
	for _, l := range strings.Split(r.block.Text, "\n") {
		if l == line {
			return
		}
		c.i += len(viewAnchorRe.FindAllString(l, -1))
	}
}

func highlightInline(s string, cursor *refCursor) string {
	s = viewLinkRe.ReplaceAllStringFunc(s, func(match string) string {
		if !viewAnchorRe.MatchString(match) {
			return styleLink.Render(match)
		}
		// A block reference shows the block. Showing the page name told the
		// reader nothing: you point at one block out of a hundred and are
		// shown the word "Timetable".
		e, ok := cursor.next()
		switch {
		case !ok:
			return styleLink.Render(match) // nothing was resolved for this row
		case e.Missing:
			return styleError.Render(match)
		}
		return styleLink.Render(oneLineRef(e.Text))
	})
	s = viewTagRe.ReplaceAllStringFunc(s, func(match string) string {
		i := strings.Index(match, "#")
		return match[:i] + styleTag.Render(match[i:])
	})
	return s
}

// highlightRaw colours a line without resolving anything, for the places that
// must show what is actually in the file: the block under the caret, and the
// section rows below the outline, which belong to other pages.
func highlightRaw(s string) string { return highlightInline(s, &refCursor{}) }

// oneLineRef flattens a referenced block for use inside a line of prose.
func oneLineRef(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len([]rune(s)) > 80 {
		s = string([]rune(s)[:77]) + "…"
	}
	return s
}

func (m *Model) pickerView() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render(m.pick.title))
	b.WriteString(" ")
	b.WriteString(m.pick.query.String())
	b.WriteString(styleCursor.Render(" "))
	b.WriteByte('\n')
	b.WriteString(styleMuted.Render(strings.Repeat("─", max(1, min(m.width, 80)))))
	b.WriteByte('\n')

	h := m.height - 4
	if h < 1 {
		h = 1
	}
	top := 0
	if m.pick.sel >= h {
		top = m.pick.sel - h + 1
	}
	shown := 0
	for i := top; i < len(m.pick.filtered) && shown < h; i++ {
		it := m.pick.filtered[i]
		label := it.label
		if label == "" {
			label = styleMuted.Render("(empty block)")
		}
		line := label
		if it.detail != "" {
			line += "  " + styleMuted.Render(it.detail)
		}
		if i == m.pick.sel {
			line = styleSelected.Render(truncate(it.label+"  "+it.detail, m.width-2))
		}
		b.WriteString("  " + line + "\n")
		shown++
	}
	if len(m.pick.filtered) == 0 {
		b.WriteString(styleMuted.Render("  no matches") + "\n")
	}
	for ; shown < h; shown++ {
		b.WriteByte('\n')
	}
	b.WriteString(styleMuted.Render("enter open · esc cancel"))
	return b.String()
}

func (m *Model) helpView() string {
	rows := [][2]string{
		{"↑ ↓ / j k", "move between blocks"},
		{"← → / h l", "collapse · expand, or step out and in"},
		{"enter", "new block below, and start typing"},
		{"i / e", "edit this block"},
		{"alt+enter", "newline inside a block (also ctrl+j)"},
		{"tab / shift+tab", "indent · outdent"},
		{"alt+↑ / alt+↓", "move the block and its children"},
		{"space", "toggle the task checkbox"},
		{"dd", "delete the block and its children"},
		{"[[", "page autocomplete, while typing"},
		{"/", "commands: /todo, /deadline fr, /table, /code …"},
		{"gf", "follow the [[link]] in this block"},
		{"gb", "what links here, as a list"},
		{"gj", "journals: every day, newest first, editable"},
		{"enter", "on a reference below the outline: jump to it"},
		{"ctrl+p", "open a journal or page"},
		{"/", "search every block"},
		{"t", "today's journal"},
		{"[ / ]", "previous · next day"},
		{"backspace", "back to where you came from"},
		{"ctrl+n", "next of the recent pages in the header"},
		{"R", "reload from disk after an external edit"},
		{"g g / G", "first · last block"},
		{"A", "agenda: everything with a deadline"},
		{"M", "messages: everything tlog has said"},
		{",", "settings"},
		{"q", "quit"},
		{"click", "select · again: edit there, or open the [[link]]"},
		{"click ▸ ▾ ☐", "fold · unfold · tick the task"},
		{"wheel", "move between blocks, or through a list"},
	}
	var b strings.Builder
	b.WriteString(styleTitle.Render("tlog") + "\n\n")
	for _, r := range rows {
		b.WriteString(fmt.Sprintf("  %-16s %s\n", r[0], styleMuted.Render(r[1])))
	}
	b.WriteString("\n  " + styleMuted.Render("Type a page as you mention it: [[Ada #person]], [[engine #project]].") + "\n")
	b.WriteString("  " + styleMuted.Render("The #person page then lists every person. A tag is just a page.") + "\n")
	b.WriteString("\n" + styleMuted.Render("  Your notes are plain markdown in "+m.svc.Store.Root+"\n"))
	b.WriteString(styleMuted.Render("  Every change is committed to git there. Press any key.\n"))
	return b.String()
}

func pad(left, right string, width int) string {
	lw := lipgloss.Width(left)
	rw := lipgloss.Width(right)
	gap := width - lw - rw
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w <= 1 {
		return string(r[:w])
	}
	return string(r[:w-1]) + "…"
}

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }
