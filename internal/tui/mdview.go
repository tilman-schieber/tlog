package tui

import (
	"html"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/tilman-schieber/tlog/internal/markdown"
)

// An attached markdown document is drawn the way glow draws one: headings
// stand out, paragraphs are reflowed to the window, lists get bullets and
// hanging indents, quotes a bar, tables aligned columns, code is highlighted,
// and emphasis, inline code and links are styled instead of shown as
// asterisks and brackets. It is a reader's view, not CommonMark: a document
// written for GitHub reads right, and anything it does not recognise is shown
// as the text it is, never dropped. Colours are the same sixteen-colour roles
// as everywhere else.
//
// Every drawn line carries the links on it and the columns they landed in, as
// the outline's lines do, so a click can follow one; nothing is searched for
// in the rendered text afterwards.

// mdLine is one line as drawn, with its links and, on a heading's first line,
// the anchor a #link to it uses.
type mdLine struct {
	s    string
	hits []linkHit
	slug string
}

func plainLines(ss ...string) []mdLine {
	out := make([]mdLine, len(ss))
	for i, s := range ss {
		out[i] = mdLine{s: s}
	}
	return out
}

// indented puts first before the first line and cont before the others,
// moving their links along. An empty line stays empty.
func indented(ls []mdLine, first, cont string) []mdLine {
	for i := range ls {
		p := cont
		if i == 0 {
			p = first
		}
		if ls[i].s == "" && i > 0 {
			continue
		}
		ls[i] = shifted(ls[i], p)
	}
	return ls
}

func shifted(l mdLine, prefix string) mdLine {
	w := ansi.StringWidth(prefix)
	hits := make([]linkHit, len(l.hits))
	for j, h := range l.hits {
		h.from += w
		h.to += w
		hits[j] = h
	}
	return mdLine{s: prefix + l.s, hits: hits, slug: l.slug}
}

// clipHits drops what of a line's links lies at or past column w.
func clipHits(hits []linkHit, w int) []linkHit {
	var out []linkHit
	for _, h := range hits {
		if h.from < w {
			h.to = min(h.to, w)
			out = append(out, h)
		}
	}
	return out
}

var (
	mdH1     = lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Bold(true).Reverse(true)
	mdCode   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	mdBullet = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))

	mdAlerts = map[string]lipgloss.Style{
		"NOTE":      lipgloss.NewStyle().Foreground(lipgloss.Color("4")),
		"TIP":       lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
		"IMPORTANT": lipgloss.NewStyle().Foreground(lipgloss.Color("5")),
		"WARNING":   lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		"CAUTION":   lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
	}
)

var (
	mdHeadingRe = regexp.MustCompile(`^ {0,3}(#{1,6})(?:[ \t]+(.*?))?(?:[ \t]+#+)?[ \t]*$`)
	mdRuleRe    = regexp.MustCompile(`^ {0,3}(?:(?:-[ \t]*){3,}|(?:\*[ \t]*){3,}|(?:_[ \t]*){3,})$`)
	mdFenceRe   = regexp.MustCompile("^( {0,3})(`{3,}|~{3,})[ \t]*([^`]*)$")
	mdListRe    = regexp.MustCompile(`^( {0,3})([-*+]|\d{1,9}[.)])( +|$)`)
	mdQuoteRe   = regexp.MustCompile(`^ {0,3}> ?`)
	mdSetext1Re = regexp.MustCompile(`^ {0,3}=+[ \t]*$`)
	mdSetext2Re = regexp.MustCompile(`^ {0,3}-+[ \t]*$`)
	mdTableSep  = regexp.MustCompile(`^ {0,3}\|?[ \t]*:?-+:?[ \t]*(\|[ \t]*:?-+:?[ \t]*)*\|?[ \t]*$`)
	mdTaskRe    = regexp.MustCompile(`^\[([ xX])\][ \t]+`)
	mdAlertRe   = regexp.MustCompile(`^\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\][ \t]*$`)
)

// markdownLines draws a markdown document for a window width columns wide.
// Every line it returns fits.
func markdownLines(text string, width int) []mdLine {
	text = strings.ReplaceAll(strings.TrimRight(text, "\n"), "\t", "    ")
	lines := strings.Split(text, "\n")
	var out []mdLine
	if fm, rest, ok := mdFrontMatter(lines); ok {
		out = append(out, mdCodeBlock("yaml", fm, width-2)...)
		out = append(out, mdLine{})
		lines = rest
	}
	out = append(out, mdBlocks(lines, width-2, false)...)
	seen := map[string]int{}
	for i, l := range out {
		if l.s != "" {
			out[i] = shifted(l, "  ")
		}
		if l.slug != "" { // a second heading of the same name is #name-1, as on GitHub
			if n := seen[l.slug]; n > 0 {
				out[i].slug = l.slug + "-" + strconv.Itoa(n)
			}
			seen[l.slug]++
		}
	}
	return out
}

// mdSlug is the anchor GitHub gives a heading: lower case, spaces as hyphens,
// punctuation dropped.
func mdSlug(segs []mdSeg) string {
	var b strings.Builder
	for _, sg := range segs {
		for _, r := range strings.ToLower(sg.text) {
			switch {
			case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_':
				b.WriteRune(r)
			case r == ' ':
				b.WriteByte('-')
			}
		}
	}
	return b.String()
}

// mdFrontMatter splits off a YAML header, which glow hides; here it is shown
// faded, because hiding part of a file is how a reader gets misled.
func mdFrontMatter(lines []string) (fm string, rest []string, ok bool) {
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		return "", lines, false
	}
	for i := 1; i < len(lines); i++ {
		if t := strings.TrimSpace(lines[i]); t == "---" || t == "..." {
			return strings.Join(lines[1:i], "\n"), lines[i+1:], true
		}
	}
	return "", lines, false
}

// mdBlocks draws a run of block-level markdown. It calls itself for what a
// list item or a quote contains, with the width that is left; tight drops the
// blank line between blocks, as in a list whose items have none.
func mdBlocks(lines []string, width int, tight bool) []mdLine {
	width = max(8, width)
	var out []mdLine
	add := func(block []mdLine) {
		if len(block) == 0 {
			return
		}
		if len(out) > 0 && !tight {
			out = append(out, mdLine{})
		}
		out = append(out, block...)
	}

	for i := 0; i < len(lines); {
		l := lines[i]
		t := strings.TrimSpace(l)
		switch {
		case t == "":
			i++

		case strings.HasPrefix(t, "<!--"):
			for i < len(lines) && !strings.Contains(lines[i], "-->") {
				i++
			}
			i++

		case mdFenceRe.MatchString(l):
			m := mdFenceRe.FindStringSubmatch(l)
			indent, fence, lang := len(m[1]), m[2], strings.Fields(m[3]+" ")
			var code []string
			i++
			for i < len(lines) {
				c := strings.TrimSpace(lines[i])
				if strings.HasPrefix(c, fence) && strings.Trim(c, fence[:1]) == "" {
					i++
					break
				}
				code = append(code, mdTrimIndent(lines[i], indent))
				i++
			}
			name := ""
			if len(lang) > 0 {
				name = lang[0]
			}
			add(mdCodeBlock(name, strings.Join(code, "\n"), width))

		case mdHeadingRe.MatchString(l):
			m := mdHeadingRe.FindStringSubmatch(l)
			add(mdHeading(len(m[1]), m[2], width))
			i++

		case mdRuleRe.MatchString(l):
			add(plainLines(styleMuted.Render(strings.Repeat("─", width))))
			i++

		case mdQuoteRe.MatchString(l):
			var inner []string
			for i < len(lines) {
				q := lines[i]
				if loc := mdQuoteRe.FindStringIndex(q); loc != nil {
					inner = append(inner, q[loc[1]:])
				} else if strings.TrimSpace(q) != "" && len(inner) > 0 && strings.TrimSpace(inner[len(inner)-1]) != "" && !mdStartsBlock(q) {
					inner = append(inner, q) // a lazy continuation of the paragraph
				} else {
					break
				}
				i++
			}
			add(mdQuote(inner, width))

		case mdListRe.MatchString(l):
			n, block := mdList(lines[i:], width)
			add(block)
			i += n

		case mdIsIndentedCode(l) && (i == 0 || strings.TrimSpace(lines[i-1]) == ""):
			var code []string
			for i < len(lines) && (mdIsIndentedCode(lines[i]) || strings.TrimSpace(lines[i]) == "") {
				code = append(code, mdTrimIndent(lines[i], 4))
				i++
			}
			for len(code) > 0 && strings.TrimSpace(code[len(code)-1]) == "" {
				code = code[:len(code)-1]
			}
			add(mdCodeBlock("", strings.Join(code, "\n"), width))

		case i+1 < len(lines) && strings.Contains(l, "|") && mdTableSep.MatchString(lines[i+1]) && strings.Contains(lines[i+1], "-"):
			rows := [][]string{mdTableCells(l)}
			align := mdTableAlign(lines[i+1])
			i += 2
			for i < len(lines) && strings.Contains(lines[i], "|") && strings.TrimSpace(lines[i]) != "" {
				rows = append(rows, mdTableCells(lines[i]))
				i++
			}
			add(mdTable(rows, align, width))

		default:
			var para []string
			level := 0
			for i < len(lines) {
				p := lines[i]
				if strings.TrimSpace(p) == "" {
					break
				}
				if len(para) > 0 {
					if mdSetext1Re.MatchString(p) {
						level, i = 1, i+1
						break
					}
					if mdSetext2Re.MatchString(p) {
						level, i = 2, i+1
						break
					}
					if mdStartsBlock(p) {
						break
					}
				}
				para = append(para, p)
				i++
			}
			if level > 0 {
				add(mdHeading(level, strings.TrimSpace(strings.Join(para, " ")), width))
			} else {
				add(mdParagraph(para, width))
			}
		}
	}
	return out
}

// mdStartsBlock reports whether a line begins something other than more of the
// paragraph above it.
func mdStartsBlock(l string) bool {
	return mdHeadingRe.MatchString(l) || mdRuleRe.MatchString(l) || mdFenceRe.MatchString(l) ||
		mdQuoteRe.MatchString(l) || mdListRe.MatchString(l) || strings.HasPrefix(strings.TrimSpace(l), "<!--")
}

func mdIsIndentedCode(l string) bool {
	return strings.HasPrefix(l, "    ") && strings.TrimSpace(l) != ""
}

// mdTrimIndent removes up to n leading spaces.
func mdTrimIndent(l string, n int) string {
	i := 0
	for i < n && i < len(l) && l[i] == ' ' {
		i++
	}
	return l[i:]
}

func mdHeading(level int, text string, width int) []mdLine {
	segs := mdInline(text, 0)
	var lines []mdLine
	if level == 1 {
		lines = mdWrap(segs, width-2)
		for i, l := range lines {
			l = shifted(l, " ")
			l.s = throughout(mdH1, l.s+" ")
			lines[i] = l
		}
	} else {
		// Below the title a heading is bold, as it is in a note: the outline
		// and the file viewer draw one thing one way.
		lines = mdWrap(segs, width)
		for i := range lines {
			lines[i].s = boldThroughout(lines[i].s)
		}
	}
	lines[0].slug = mdSlug(segs)
	return lines
}

// mdParagraph reflows lines into the width, keeping the breaks markdown calls
// hard: two spaces or a backslash at the end of a line.
func mdParagraph(lines []string, width int) []mdLine {
	var out []mdLine
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			out = append(out, mdWrap(mdInline(strings.Join(cur, " "), 0), width)...)
			cur = nil
		}
	}
	for _, l := range lines {
		hard := strings.HasSuffix(l, "  ") || strings.HasSuffix(l, "\\")
		l = strings.TrimSpace(l)
		if hard {
			l = strings.TrimSuffix(l, "\\")
		}
		cur = append(cur, l)
		if hard {
			flush()
		}
	}
	flush()
	return out
}

func mdCodeBlock(lang, code string, width int) []mdLine {
	var out []mdLine
	if lang != "" {
		out = append(out, mdLine{s: styleMuted.Render(lang)})
	}
	bar := styleMuted.Render("▏") + " "
	for _, l := range strings.Split(highlightCode(lang, code), "\n") {
		for _, piece := range strings.Split(ansi.Hardwrap(l, max(4, width-2), true), "\n") {
			out = append(out, mdLine{s: bar + piece})
		}
	}
	return out
}

func mdQuote(inner []string, width int) []mdLine {
	bar := styleMuted
	var title []mdLine
	if len(inner) > 0 {
		if m := mdAlertRe.FindStringSubmatch(strings.TrimSpace(inner[0])); m != nil {
			bar = mdAlerts[m[1]]
			name := m[1][:1] + strings.ToLower(m[1][1:])
			title = plainLines(bar.Bold(true).Render(name))
			inner = inner[1:]
		}
	}
	body := append(title, mdBlocks(inner, width-2, false)...)
	for i, l := range body {
		body[i] = shifted(l, bar.Render("│")+" ")
	}
	return body
}

// mdList draws the list starting at lines[0] and says how many lines it took.
// Each item's content is drawn as blocks of its own, so nested lists, code and
// quotes inside an item come out right.
func mdList(lines []string, width int) (int, []mdLine) {
	first := mdListRe.FindStringSubmatch(lines[0])
	ordered := len(first[2]) > 1 || unicode.IsDigit(rune(first[2][0]))
	kind := first[2][len(first[2])-1]

	type item struct {
		marker string
		body   []string
		blank  bool // a blank line came before it
	}
	var items []item
	i := 0
	blankBefore := false
	for i < len(lines) {
		m := mdListRe.FindStringSubmatch(lines[i])
		if m == nil || m[2][len(m[2])-1] != kind || len(m[1]) > 3 {
			break
		}
		col := len(m[0])
		if len(m[3]) > 4 { // the content is indented code; its marker takes one space
			col = len(m[1]) + len(m[2]) + 1
		}
		it := item{marker: m[2], body: []string{lines[i][min(col, len(lines[i])):]}, blank: blankBefore}
		blankBefore = false
		i++
		for i < len(lines) {
			l := lines[i]
			if strings.TrimSpace(l) == "" {
				// A blank line keeps the item open only if what follows is
				// still indented under it.
				j := i
				for j < len(lines) && strings.TrimSpace(lines[j]) == "" {
					j++
				}
				if j < len(lines) && mdIndentOf(lines[j]) >= col {
					for ; i < j; i++ {
						it.body = append(it.body, "")
					}
					continue
				}
				blankBefore = j < len(lines) && mdListRe.MatchString(lines[j])
				i = j
				break
			}
			if mdIndentOf(l) >= col {
				it.body = append(it.body, l[col:])
				i++
				continue
			}
			last := strings.TrimSpace(it.body[len(it.body)-1])
			if !mdStartsBlock(l) && last != "" {
				it.body = append(it.body, strings.TrimSpace(l)) // lazy continuation
				i++
				continue
			}
			break
		}
		items = append(items, it)
		if blankBefore && (i >= len(lines) || !mdListRe.MatchString(lines[i])) {
			break
		}
	}

	markerWidth := 2
	if ordered {
		for _, it := range items {
			markerWidth = max(markerWidth, len(it.marker)+1)
		}
	}
	var out []mdLine
	for _, it := range items {
		body := it.body
		box := ""
		if m := mdTaskRe.FindString(body[0]); m != "" {
			if strings.TrimSpace(m) == "[ ]" {
				box = styleTodo.Render("☐")
			} else {
				box = styleDone.Render("☑")
			}
			body = append([]string{body[0][len(m):]}, body[1:]...)
		}
		mark := mdBullet.Render("•")
		if ordered {
			mark = mdBullet.Render(strings.TrimRight(it.marker, ".)") + ".")
		}
		if box != "" {
			mark = box
		}
		pad := markerWidth - lipgloss.Width(mark)
		if !ordered {
			pad = 1
		}
		first := mark + strings.Repeat(" ", max(1, pad))
		w := lipgloss.Width(first)

		tight := true
		for _, l := range body {
			if strings.TrimSpace(l) == "" {
				tight = false
			}
		}
		drawn := mdBlocks(body, width-w, tight)
		if len(drawn) == 0 {
			drawn = []mdLine{{}}
		}
		if it.blank && len(out) > 0 {
			out = append(out, mdLine{})
		}
		out = append(out, indented(drawn, first, strings.Repeat(" ", w))...)
	}
	return i, out
}

func mdIndentOf(l string) int { return len(l) - len(strings.TrimLeft(l, " ")) }

// mdTableCells splits a table row on the pipes that are not escaped or inside
// inline code.
func mdTableCells(l string) []string {
	l = strings.TrimSpace(l)
	l = strings.TrimPrefix(l, "|")
	if strings.HasSuffix(l, "|") && !strings.HasSuffix(l, `\|`) {
		l = l[:len(l)-1]
	}
	var cells []string
	var cur strings.Builder
	code := false
	for i := 0; i < len(l); i++ {
		c := l[i]
		switch {
		case c == '\\' && i+1 < len(l) && l[i+1] == '|':
			cur.WriteByte('|')
			i++
			continue
		case c == '`':
			code = !code
		case c == '|' && !code:
			cells = append(cells, strings.TrimSpace(cur.String()))
			cur.Reset()
			continue
		}
		cur.WriteByte(c)
	}
	return append(cells, strings.TrimSpace(cur.String()))
}

func mdTableAlign(sep string) []byte {
	var out []byte
	for _, c := range mdTableCells(sep) {
		switch {
		case strings.HasPrefix(c, ":") && strings.HasSuffix(c, ":"):
			out = append(out, 'c')
		case strings.HasSuffix(c, ":"):
			out = append(out, 'r')
		default:
			out = append(out, 'l')
		}
	}
	return out
}

// mdTable lays rows out in aligned columns, narrowing the widest column until
// the table fits; a cell that still does not fit is cut with an ellipsis.
func mdTable(rows [][]string, align []byte, width int) []mdLine {
	cols := len(rows[0])
	drawn := make([][]mdLine, len(rows))
	widths := make([]int, cols)
	for r, row := range rows {
		drawn[r] = make([]mdLine, cols)
		for c := range cols {
			var cell mdLine
			if c < len(row) {
				cell = mdWrap(mdInline(row[c], 0), 1<<20)[0]
			}
			if r == 0 {
				cell.s = boldThroughout(cell.s)
			}
			drawn[r][c] = cell
			widths[c] = max(widths[c], lipgloss.Width(cell.s))
		}
	}
	sepW := 3 * (cols - 1)
	for {
		total := sepW
		widest := 0
		for c, w := range widths {
			total += w
			if w > widths[widest] {
				widest = c
			}
		}
		if total <= width || widths[widest] <= 3 {
			break
		}
		widths[widest]--
	}
	line := func(cells []mdLine) mdLine {
		var b strings.Builder
		var hits []linkHit
		col := 0
		for c, cell := range cells {
			if c > 0 {
				b.WriteString(styleMuted.Render(" │ "))
				col += 3
			}
			w := widths[c]
			text, cellHits := cell.s, cell.hits
			if lipgloss.Width(text) > w {
				text = ansi.Truncate(text, w, "…")
				cellHits = clipHits(cellHits, w)
			}
			gap := w - lipgloss.Width(text)
			a := byte('l')
			if c < len(align) {
				a = align[c]
			}
			left := 0
			switch a {
			case 'r':
				left = gap
			case 'c':
				left = gap / 2
			}
			b.WriteString(strings.Repeat(" ", left) + text + strings.Repeat(" ", gap-left))
			for _, h := range cellHits {
				h.from += col + left
				h.to += col + left
				hits = append(hits, h)
			}
			col += w
		}
		return mdLine{s: b.String(), hits: hits}
	}
	out := []mdLine{line(drawn[0])}
	var rule []string
	for _, w := range widths {
		rule = append(rule, strings.Repeat("─", w))
	}
	out = append(out, mdLine{s: styleMuted.Render(strings.Join(rule, "─┼─"))})
	for _, r := range drawn[1:] {
		out = append(out, line(r))
	}
	for i, l := range out { // a table wider than the window even at its narrowest
		if lipgloss.Width(l.s) > width {
			out[i] = mdLine{s: ansi.Truncate(l.s, width, "…"), hits: clipHits(l.hits, width-1)}
		}
	}
	return out
}

// Inline markdown is parsed into runs of text that share a style, so that a
// paragraph can be wrapped by words and still keep each word's style.

type mdFlag uint8

const (
	fBold mdFlag = 1 << iota
	fItalic
	fStrike
	fCode
	fLink
	fFaint
)

type mdSeg struct {
	text  string
	flags mdFlag
	to    *linkHit // what the run links to, its columns unset; nil for text
}

var mdStyles = map[mdFlag]lipgloss.Style{}

func mdSegStyle(f mdFlag) lipgloss.Style {
	if st, ok := mdStyles[f]; ok {
		return st
	}
	st := lipgloss.NewStyle()
	switch {
	case f&fCode != 0:
		st = mdCode
	case f&fLink != 0:
		st = styleURL
	}
	if f&fFaint != 0 {
		st = st.Faint(true)
	}
	if f&fBold != 0 {
		st = st.Bold(true)
	}
	if f&fItalic != 0 {
		st = st.Italic(true)
	}
	if f&fStrike != 0 {
		st = st.Strikethrough(true)
	}
	mdStyles[f] = st
	return st
}

var (
	mdBareURLRe = regexp.MustCompile(`^https?://[^\s<>]+`)
	mdAutoRe    = regexp.MustCompile(`^<((?:https?|mailto|file):[^\s<>]+)>`)
)

// mdInline turns inline markdown into styled runs. What does not parse as
// markup is kept as the characters it is.
func mdInline(s string, flags mdFlag) []mdSeg {
	var out []mdSeg
	var lit strings.Builder
	flushLit := func() {
		if lit.Len() > 0 {
			out = append(out, mdSeg{text: html.UnescapeString(lit.String()), flags: flags})
			lit.Reset()
		}
	}
	emit := func(segs ...mdSeg) {
		flushLit()
		out = append(out, segs...)
	}
	for i := 0; i < len(s); {
		c := s[i]
		rest := s[i:]
		switch {
		case c == '\\' && i+1 < len(s) && strings.ContainsRune("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~", rune(s[i+1])):
			lit.WriteByte(s[i+1])
			i += 2
			continue

		case c == '`':
			n := len(rest) - len(strings.TrimLeft(rest, "`"))
			ticks := rest[:n]
			if end := closingTicks(rest[n:], ticks); end >= 0 {
				code := rest[n : n+end]
				if len(code) > 1 && code[0] == ' ' && code[len(code)-1] == ' ' && strings.TrimSpace(code) != "" {
					code = code[1 : len(code)-1]
				}
				emit(mdSeg{text: code, flags: flags | fCode})
				i += n + end + n
				continue
			}
			lit.WriteString(ticks)
			i += n
			continue

		case strings.HasPrefix(rest, "[["):
			if end := strings.Index(rest, "]]"); end > 2 {
				page := markdown.ParseLinkTarget(rest[2:end])
				emit(mdSeg{text: rest[:end+2], flags: flags | fLink, to: &linkHit{page: page}})
				i += end + 2
				continue
			}

		case c == '[' || (c == '!' && strings.HasPrefix(rest, "![")):
			image := c == '!'
			start := 0
			if image {
				start = 1
			}
			if label, url, n, ok := inlineLink(rest[start:]); ok {
				to := &linkHit{url: url}
				var segs []mdSeg
				if image {
					segs = append(segs, mdSeg{text: "▣ ", flags: flags | fLink})
				}
				if label == "" {
					label = url
				}
				segs = append(segs, mdInline(label, flags|fLink)...)
				if url != "" && url != label && !strings.HasPrefix(url, "#") {
					segs = append(segs, mdSeg{text: " ", flags: flags}, mdSeg{text: url, flags: flags | fFaint})
				}
				for k := range segs {
					if url != "" && segs[k].text != " " {
						segs[k].to = to
					}
				}
				emit(segs...)
				i += start + n
				continue
			}

		case c == '<':
			if m := mdAutoRe.FindStringSubmatch(rest); m != nil {
				emit(mdSeg{text: strings.TrimPrefix(m[1], "mailto:"), flags: flags | fLink, to: &linkHit{url: m[1]}})
				i += len(m[0])
				continue
			}

		case c == 'h' && (i == 0 || !isWordByte(s[i-1])):
			if m := mdBareURLRe.FindString(rest); m != "" {
				m = strings.TrimRight(m, ".,;:!?)'\"")
				emit(mdSeg{text: m, flags: flags | fLink, to: &linkHit{url: m}})
				i += len(m)
				continue
			}

		case c == '*' || c == '_' || (c == '~' && strings.HasPrefix(rest, "~~")):
			n := len(rest) - len(strings.TrimLeft(rest, string(c)))
			run := rest[:n]
			if c == '~' {
				run = "~~"
			} else if n > 3 {
				run = rest[:3]
			}
			if inner, ok := emphasis(s, i, run); ok {
				f := flags
				switch {
				case c == '~':
					f |= fStrike
				case len(run) == 1:
					f |= fItalic
				case len(run) == 2:
					f |= fBold
				default:
					f |= fBold | fItalic
				}
				emit(mdInline(inner, f)...)
				i += len(run) + len(inner) + len(run)
				continue
			}
			lit.WriteString(rest[:n])
			i += n
			continue
		}
		_, size := utf8.DecodeRuneInString(rest)
		lit.WriteString(rest[:size])
		i += size
	}
	flushLit()
	return out
}

func isWordByte(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= 0x80
}

// mdEmphasis draws the inline styling of a line in a note — **bold**,
// *italic*, `code`, ~~struck~~ — with the delimiters hidden, and hands every
// other run of text to plain. It is the outliner's half of mdInline: the same
// delimiter rules and styles, with links left to the caller, which draws them
// itself so it knows where they landed. [[links]] are passed through whole,
// and nothing inside inline code is coloured as a tag or a link.
func mdEmphasis(s string, plain func(string) string) string {
	var b, lit strings.Builder
	flush := func() {
		if lit.Len() > 0 {
			b.WriteString(plain(lit.String()))
			lit.Reset()
		}
	}
	for i := 0; i < len(s); {
		c := s[i]
		rest := s[i:]
		switch {
		case c == '\\' && i+1 < len(s):
			lit.WriteString(rest[:2])
			i += 2
			continue
		case strings.HasPrefix(rest, "[["):
			if end := strings.Index(rest, "]]"); end > 2 {
				lit.WriteString(rest[:end+2])
				i += end + 2
				continue
			}
		case c == '`':
			n := len(rest) - len(strings.TrimLeft(rest, "`"))
			if end := closingTicks(rest[n:], rest[:n]); end >= 0 {
				flush()
				b.WriteString(mdSegStyle(fCode).Render(rest[n : n+end]))
				i += n + end + n
				continue
			}
			lit.WriteString(rest[:n])
			i += n
			continue
		case c == '*' || c == '_' || (c == '~' && strings.HasPrefix(rest, "~~")):
			n := len(rest) - len(strings.TrimLeft(rest, string(c)))
			run := rest[:min(n, 3)]
			if c == '~' {
				run = "~~"
			}
			if inner, ok := emphasis(s, i, run); ok {
				f := fBold | fItalic
				switch {
				case c == '~':
					f = fStrike
				case len(run) == 1:
					f = fItalic
				case len(run) == 2:
					f = fBold
				}
				flush()
				b.WriteString(throughout(mdSegStyle(f), mdEmphasis(inner, plain)))
				i += 2*len(run) + len(inner)
				continue
			}
			lit.WriteString(rest[:n])
			i += n
			continue
		}
		_, size := utf8.DecodeRuneInString(rest)
		lit.WriteString(rest[:size])
		i += size
	}
	flush()
	return b.String()
}

// closingTicks finds a run of exactly the opening backticks in s.
func closingTicks(s, ticks string) int {
	for off := 0; ; {
		j := strings.Index(s[off:], ticks)
		if j < 0 {
			return -1
		}
		j += off
		k := j + len(ticks)
		if (j == 0 || s[j-1] != '`') && (k >= len(s) || s[k] != '`') {
			return j
		}
		off = k
		for off < len(s) && s[off] == '`' {
			off++
		}
	}
}

// emphasis finds what the delimiter run at s[i:] encloses. An opening run is
// not followed by a space, a closing one not preceded by one, and underscores
// only count at the edges of words, so snake_case stays as it is.
func emphasis(s string, i int, run string) (string, bool) {
	open := i + len(run)
	if open >= len(s) || s[open] == ' ' {
		return "", false
	}
	if run[0] == '_' && i > 0 && isWordByte(s[i-1]) {
		return "", false
	}
	for off := open + 1; off <= len(s)-len(run); off++ {
		j := strings.Index(s[off:], run)
		if j < 0 {
			return "", false
		}
		j += off
		end := j + len(run)
		if s[j-1] == ' ' || (end < len(s) && s[end] == run[0]) {
			off = j
			continue
		}
		if run[0] == '_' && end < len(s) && isWordByte(s[end]) {
			off = j
			continue
		}
		if strings.Count(s[open:j], "`")%2 != 0 { // the closer is inside inline code
			off = j
			continue
		}
		return s[open:j], true
	}
	return "", false
}

// inlineLink parses [label](url "title") at the start of s.
func inlineLink(s string) (label, url string, n int, ok bool) {
	depth := 0
	closeB := -1
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				closeB = i
			}
		}
		if closeB >= 0 {
			break
		}
	}
	if closeB < 0 || closeB+1 >= len(s) || s[closeB+1] != '(' {
		return "", "", 0, false
	}
	depth = 0
	for j := closeB + 1; j < len(s); j++ {
		switch s[j] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				dest := strings.TrimSpace(s[closeB+2 : j])
				if k := strings.IndexAny(dest, " \t"); k >= 0 {
					dest = dest[:k] // drop a "title"
				}
				dest = strings.TrimSuffix(strings.TrimPrefix(dest, "<"), ">")
				return s[1:closeB], dest, j + 1, true
			}
		}
	}
	return "", "", 0, false
}

// mdWrap breaks styled runs into lines at most width columns wide, at
// spaces where it can and inside a word only when the word alone is wider. It
// notes the columns every link lands in; the words of one link, and the space
// between them, make one stretch to click.
func mdWrap(segs []mdSeg, width int) []mdLine {
	width = max(1, width)
	type word []mdSeg
	var words []word
	var cur word
	for _, sg := range segs {
		parts := strings.Split(sg.text, " ")
		for k, p := range parts {
			if k > 0 {
				if len(cur) > 0 {
					words = append(words, cur)
				}
				cur = nil
			}
			if p != "" {
				cur = append(cur, mdSeg{text: p, flags: sg.flags, to: sg.to})
			}
		}
	}
	if len(cur) > 0 {
		words = append(words, cur)
	}

	var lines []mdLine
	var line strings.Builder
	var hits []linkHit
	lw := 0
	newline := func() {
		lines = append(lines, mdLine{s: line.String(), hits: hits})
		line.Reset()
		hits = nil
		lw = 0
	}
	hit := func(to *linkHit, from, w int) {
		if to == nil || w == 0 {
			return
		}
		if n := len(hits); n > 0 && hits[n-1].target() == to.target() && from-hits[n-1].to <= 1 {
			hits[n-1].to = from + w
			return
		}
		h := *to
		h.from, h.to = from, from+w
		hits = append(hits, h)
	}
	for _, w := range words {
		ww := 0
		for _, sg := range w {
			ww += ansi.StringWidth(sg.text)
		}
		if lw > 0 && lw+1+ww > width {
			newline()
		}
		if ww > width { // longer than a line: cut it, each piece linking where the word did
			var full strings.Builder
			var to *linkHit
			for _, sg := range w {
				full.WriteString(mdRender(sg))
				if to == nil {
					to = sg.to
				}
			}
			for _, piece := range strings.Split(ansi.Hardwrap(full.String(), width, false), "\n") {
				if lw > 0 {
					newline()
				}
				line.WriteString(piece)
				lw = ansi.StringWidth(piece)
				hit(to, 0, lw)
			}
			continue
		}
		if lw > 0 {
			line.WriteByte(' ')
			lw++
		}
		for _, sg := range w {
			sw := ansi.StringWidth(sg.text)
			hit(sg.to, lw, sw)
			line.WriteString(mdRender(sg))
			lw += sw
		}
	}
	if lw > 0 || len(lines) == 0 {
		newline()
	}
	return lines
}

func mdRender(sg mdSeg) string {
	if sg.flags == 0 {
		return sg.text
	}
	return mdSegStyle(sg.flags).Render(sg.text)
}
