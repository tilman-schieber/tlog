package tui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/tilman-schieber/tlog/internal/markdown"
	"github.com/tilman-schieber/tlog/internal/store"
)

var (
	viewLinkRe  = regexp.MustCompile(`\[\[[^\[\]]+\]\]`)
	viewTagRe   = regexp.MustCompile(`(^|\s)(#[A-Za-z][A-Za-z0-9_/\-]*)`)
	viewCheckRe = regexp.MustCompile(`^\[( |x|X)\] `)
)

func (m *Model) View() string {
	if m.quitting {
		return ""
	}
	if m.mode == modeHelp {
		return m.helpView()
	}

	var b strings.Builder
	b.WriteString(m.header())
	b.WriteByte('\n')

	lines, cursorLine := m.bodyLines()
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

	for i := top; i < len(lines) && i < top+h; i++ {
		b.WriteString(lines[i])
		b.WriteByte('\n')
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

func (m *Model) header() string {
	title := store.Title(m.doc.Rel)
	kind := "page"
	if store.IsJournal(m.doc.Rel) {
		kind = "journal"
	}
	left := styleTitle.Render(title) + " " + styleMuted.Render(kind)
	if m.g != nil {
		for _, t := range m.g.PageTags(title) {
			left += " " + styleTag.Render("#"+t)
		}
	}
	right := ""
	switch m.mode {
	case modeInsert:
		right = styleTodo.Render("INSERT")
	default:
		right = styleMuted.Render("NORMAL")
	}
	if m.stale {
		right = styleError.Render("STALE") + " " + right
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
		if m.rowKind() != rowBlock {
			return styleMuted.Render("enter open · ↑↓ move · these live on other pages and are not edited here")
		}
		return styleMuted.Render("? help · enter new block · i edit · tab/shift+tab indent · ctrl+p open · / search · gf follow · q quit")
	}
}

// bodyLines renders every visible block into screen lines and reports which
// screen line the cursor is on.
func (m *Model) bodyLines() ([]string, int) {
	var out []string
	cursorLine := 0

	for i, r := range m.rows {
		editing := m.mode == modeInsert && m.edBlock == r.block
		start := len(out)
		out = append(out, m.renderRow(r, i == m.cur, editing)...)
		if i == m.cur {
			cursorLine = start
			if editing {
				line, _ := m.ed.cursorPos()
				cursorLine = start + line
			}
		}
	}
	if len(out) == 0 {
		out = append(out, styleMuted.Render("  (empty — press enter to write something)"))
	}
	return out, cursorLine
}

func (m *Model) renderRow(r row, selected, editing bool) []string {
	if r.kind != rowBlock {
		return []string{m.renderSectionRow(r, selected)}
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
			body = highlight(l)
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
		out = append(out, indent+"  "+styleProp.Render(p.Key+":: "+p.Value))
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
			push(highlight(lines[i]))
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
		body := highlight(text)
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
		return highlight(l)
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
	return highlight(before) + styleCursor.Render(at) + highlight(after)
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

// highlight applies the semantic colour roles to inline syntax.
func highlight(s string) string {
	if s == "" {
		return s
	}
	out := s
	if m := viewCheckRe.FindString(out); m != "" {
		rest := out[len(m):]
		if strings.HasPrefix(m, "[ ]") {
			return styleTodo.Render("☐ ") + highlightInline(rest)
		}
		return styleDone.Render("☑ " + rest)
	}
	return highlightInline(out)
}

func highlightInline(s string) string {
	s = viewLinkRe.ReplaceAllStringFunc(s, func(m string) string {
		return styleLink.Render(m)
	})
	s = viewTagRe.ReplaceAllStringFunc(s, func(m string) string {
		i := strings.Index(m, "#")
		return m[:i] + styleTag.Render(m[i:])
	})
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
		{"gf", "follow the [[link]] in this block"},
		{"gb", "what links here, as a list"},
		{"enter", "on a reference below the outline: jump to it"},
		{"ctrl+p", "open a journal or page"},
		{"/", "search every block"},
		{"t", "today's journal"},
		{"[ / ]", "previous · next day"},
		{"backspace", "back to where you came from"},
		{"R", "reload from disk after an external edit"},
		{"g g / G", "first · last block"},
		{"q", "quit"},
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
