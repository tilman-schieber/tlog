package tui

import (
	"net/url"
	"path"
	"regexp"
	"sort"
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
	// A heading: hashes, a space, the text.
	viewHeadingRe = regexp.MustCompile(`^\s*#{1,6}\s+(.*)$`)
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
	if m.mode == modeFile {
		return m.fileViewString()
	}
	if m.mode == modeSettings {
		return m.settingsView()
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
	m.bodyTop = top

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
	// nf-md-notebook, the mark the Omarchy bar widget wears too.
	left := styleTitle.Render("󰠮 "+title) + " " + styleMuted.Render(kind)
	if m.page != nil {
		for _, t := range m.page.Tags {
			left += " " + styleTag.Render("#"+t)
		}
		// The other names this page answers to, so a [[link]] that worked is
		// not a mystery when you arrive.
		if a := m.page.Aliases; len(a) > 0 {
			left += " " + styleMuted.Render("auch: "+strings.Join(a, ", "))
		}
	}
	right := ""
	if m.mode == modeMove {
		right = styleTodo.Render("MOVE")
	}
	if m.stale {
		right = strings.TrimSpace(styleError.Render("STALE") + " " + right)
	}
	return pad(left, right, m.width)
}

func (m *Model) footer() string {
	switch {
	case m.errMsg != "":
		return styleError.Render(m.errMsg)
	case m.status != "":
		return styleMuted.Render(m.status)
	case m.hover != nil:
		return styleURL.Render(m.hover.target()) + styleMuted.Render("  click to open")
	case m.rowKind() != rowBlock:
		return styleMuted.Render("enter open · ↑↓ move · these live on other pages and are not edited here")
	case m.mode == modeMove:
		return styleMuted.Render("enter write · j k move · p go to · / search · t today · b back · g follow · [ ] days · x task · dd delete · z fold · ? keys · q quit")
	default:
		// Typing needs no reminding; getting somewhere does.
		return styleMuted.Render("esc to move around · ctrl+p go to · ctrl+f search · F1 keys")
	}
}

// bodyLines renders every visible block into screen lines and reports which
// screen line the cursor is on.
func (m *Model) bodyLines() ([]string, int) {
	var out []string
	cursorLine := 0
	m.lineRow, m.lineHits, m.lineFirst = m.lineRow[:0], m.lineHits[:0], m.lineFirst[:0]

	for i, r := range m.rows {
		editing := m.ed != nil && m.edBlock == r.block
		start := len(out)
		lines, hits := m.renderRowHits(r, i == m.cur, editing)
		out = append(out, lines...)
		for j := range lines {
			m.lineRow = append(m.lineRow, i)
			m.lineHits = append(m.lineHits, hits[j])
			m.lineFirst = append(m.lineFirst, start)
		}
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
		m.lineRow, m.lineHits, m.lineFirst = append(m.lineRow, -1), append(m.lineHits, nil), append(m.lineFirst, 0)
	}
	return out, cursorLine
}

func (m *Model) renderRow(r row, selected, editing bool) []string {
	lines, _ := m.renderRowHits(r, selected, editing)
	return lines
}

// renderRowHits draws a row and says, line by line, where its links landed.
func (m *Model) renderRowHits(r row, selected, editing bool) ([]string, [][]linkHit) {
	if r.kind != rowBlock {
		// A heading starts with a blank line; each screen line is its own
		// entry, or the mouse would be a line out below it.
		lines := strings.Split(m.renderSectionRow(r, selected), "\n")
		return lines, make([][]linkHit, len(lines))
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

	// Fenced regions are drawn from the core's prepared form, the one
	// `tlog view -json` carries too: highlighted, or laid out as a table when they are ```csv.
	if !editing && !selected && strings.Contains(text, "```") {
		if out, hits := m.renderFenced(r, text, indent, bullet); out != nil {
			return out, hits
		}
	}

	lines := strings.Split(text, "\n")

	var out []string
	var hits [][]linkHit
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
		}
		var lineHits []linkHit
		if !inFence && !quoted && !strings.HasPrefix(strings.TrimSpace(l), "```") {
			body, lineHits = m.highlight(l, r)
		}
		if selected && !editing {
			// The highlight is drawn over what is shown, links collapsed, so
			// the block does not change shape when the cursor reaches it.
			body = styleSelected.Render(stripANSI(body))
		}
		if editing {
			body = m.renderEditLine(l, i)
			lineHits = nil
		}
		shift := lipgloss.Width(prefix)
		for j := range lineHits {
			lineHits[j].from += shift
			lineHits[j].to += shift
		}
		out = append(out, prefix+body)
		hits = append(hits, lineHits)
	}

	if r.block.Anchor != "" && !editing {
		out[0] += " " + styleMuted.Render("^"+r.block.Anchor)
	}
	for _, p := range r.block.Props {
		if tapp.IsDeadlineKey(p.Key) {
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
	for len(hits) < len(out) {
		hits = append(hits, nil) // properties and menus carry no links
	}
	return out, hits
}

// renderFenced draws a block containing fenced regions, replacing each fence
// with coloured code or an aligned table. It returns nil when the block turns
// out to have no fence after all.
func (m *Model) renderFenced(r row, text, indent, bullet string) ([]string, [][]linkHit) {
	fences := markdown.Fences(text)
	if len(fences) == 0 {
		return nil, nil
	}
	cont := indent + "  "

	var out []string
	var hits [][]linkHit
	lines := strings.Split(text, "\n")
	fi := 0
	first := true
	pushHits := func(s string, h []linkHit) {
		prefix := cont
		if first {
			prefix = indent + styleBullet.Render(bullet) + " "
			first = false
		}
		shift := lipgloss.Width(prefix)
		for j := range h {
			h[j].from += shift
			h[j].to += shift
		}
		out = append(out, prefix+s)
		hits = append(hits, h)
	}
	push := func(s string) { pushHits(s, nil) }

	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(strings.TrimLeft(lines[i], " \t"), "```") {
			pushHits(m.highlight(lines[i], r))
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
		hits = append(hits, nil)
	}
	return out, hits
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
		hint := "no page by that name yet — finish typing and press ctrl+g to create it"
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
// block reference in the line to the block it points at. It also says where on
// the line each link landed, so a mouse can find it.
func (m *Model) highlight(s string, r row) (string, []linkHit) {
	if s == "" {
		return s, nil
	}
	// References are numbered across the whole block but drawn a line at a
	// time, so the count is shared. Lines before this one have already used up
	// theirs.
	var embeds []tapp.EmbedView
	if r.kind == rowBlock && r.block != nil {
		embeds = m.embeds[r.block.Start]
	}
	cursor := &refCursor{embeds: embeds}
	cursor.skip(r, s)

	if mark := viewCheckRe.FindString(s); mark != "" {
		rest := s[len(mark):]
		if strings.HasPrefix(mark, "[ ]") {
			box := styleTodo.Render("☐ ")
			body, hits := inlineWithLinks(rest, cursor, lipgloss.Width(box))
			return box + body, hits
		}
		// Done work is drawn faded as a whole, links included, but they are
		// still collapsed and still clickable.
		body, hits := inlineWithLinks(rest, cursor, 2)
		return styleDone.Render("☑ " + stripANSI(body)), hits
	}
	// A markdown heading is drawn bold, without its hashes. Nothing about the file changes; the hashes are there while
	// writing.
	if h := viewHeadingRe.FindStringSubmatch(s); h != nil {
		body, hits := inlineWithLinks(h[1], cursor, 0)
		return boldThroughout(body), hits
	}
	return inlineWithLinks(s, cursor, 0)
}

// boldThroughout makes a line bold although it is already coloured in places:
// every reset inside it would end the bold, so bold is put back after each.
// When the terminal takes no styling at all the text comes back as it was.
func boldThroughout(s string) string { return rebold(styleTitle.Render(s)) }

func rebold(out string) string {
	const on, off = "\x1b[1m", "\x1b[0m"
	if !strings.HasPrefix(out, on) || !strings.HasSuffix(out, off) {
		return out
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(out, on), off)
	return on + strings.ReplaceAll(inner, off, off+on) + off
}

// linkHit is where a link was drawn: columns on one screen line, and what it
// points at — an address or a page.
type linkHit struct {
	from, to int // columns, from inclusive, to exclusive
	url      string
	page     markdown.Link
}

func (h linkHit) target() string {
	if h.url != "" {
		return h.url
	}
	return h.page.String()
}

// inlineWithLinks draws a line piece by piece — text, then a link, then text
// — so that the column each link starts at is known exactly rather than
// searched for afterwards. Standard markdown links are drawn as their label
// alone; the address is what hovering shows. col is where the line's text
// starts on screen.
func inlineWithLinks(s string, cursor *refCursor, col int) (string, []linkHit) {
	type span struct {
		start, end int
		url        *markdown.URLLink
		page       *markdown.Link
	}
	var spans []span
	urls := markdown.URLLinks(s)
	for i := range urls {
		spans = append(spans, span{start: urls[i].Start, end: urls[i].End, url: &urls[i]})
	}
	for _, w := range viewLinkRe.FindAllStringIndex(s, -1) {
		inside := false
		for _, sp := range spans {
			if w[0] < sp.end && sp.start < w[1] {
				inside = true
			}
		}
		if !inside {
			l := markdown.ParseLinkTarget(s[w[0]+2 : w[1]-2])
			spans = append(spans, span{start: w[0], end: w[1], page: &l})
		}
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })

	var b strings.Builder
	var hits []linkHit
	emit := func(text string) {
		out := highlightInline(text, cursor)
		b.WriteString(out)
		col += lipgloss.Width(out)
	}
	last := 0
	for _, sp := range spans {
		emit(s[last:sp.start])
		start := col
		if sp.url != nil {
			label := linkLabel(*sp.url)
			b.WriteString(styleURL.Render(label))
			col += lipgloss.Width(label)
			hits = append(hits, linkHit{from: start, to: col, url: sp.url.URL})
		} else {
			emit(s[sp.start:sp.end]) // a [[link]] keeps its own drawing
			hits = append(hits, linkHit{from: start, to: col, page: *sp.page})
		}
		last = sp.end
	}
	emit(s[last:])
	return b.String(), hits
}

// linkLabel is what a link is drawn as: its text, never its address. A bare
// address has no text, so it is shortened to what identifies it — a file's
// name, a site's host.
func linkLabel(l markdown.URLLink) string {
	label := l.Label
	if l.Bare || label == "" {
		label = shortAddress(l.URL)
	}
	if l.Image {
		return "▣ " + label
	}
	return label
}

func shortAddress(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return path.Base(raw)
	}
	switch u.Scheme {
	case "file":
		return path.Base(u.Path)
	case "mailto":
		return u.Opaque
	}
	if p := strings.Trim(u.Path, "/"); p != "" || u.RawQuery != "" {
		return u.Host + "/…"
	}
	return u.Host
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
			// [[Max #person]] says once what Max is; drawn every time, it would
			// set him apart from every person who was introduced elsewhere.
			// The link is drawn as [[Max]], and hovering shows all of it.
			if l := markdown.ParseLinkTarget(match[2 : len(match)-2]); len(l.Tags) > 0 {
				return styleLink.Render("[[" + l.Page + "]]")
			}
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
	m.pickTop = top
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
	hint := "enter open · esc cancel"
	if m.pick.creates {
		hint = "enter open · a name that matches nothing makes that page · esc cancel"
	}
	b.WriteString(styleMuted.Render(hint))
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
