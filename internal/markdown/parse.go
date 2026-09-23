package markdown

import (
	"regexp"
	"strings"
)

var (
	// A bullet line: indentation, a dash, then either a space and content or
	// nothing at all (an empty block).
	bulletRe = regexp.MustCompile(`^([ \t]*)-(?:[ \t]+(.*))?$`)

	// A block property on its own continuation line: key:: value.
	propRe = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_.\-]*)::[ \t]*(.*)$`)

	// A trailing Obsidian-style block anchor.
	anchorRe = regexp.MustCompile(`[ \t]+\^([A-Za-z0-9]{4,32})$`)

	// A frontmatter entry. Flat only in V0.1; nested YAML is not part of the
	// dialect.
	frontRe = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_.\-]*):[ \t]*(.*)$`)

	fenceRe = regexp.MustCompile("^(```|~~~)")
)

type srcLine struct {
	start int // byte offset of the first character
	nlEnd int // byte offset just past the trailing newline, or past EOF
	text  string
}

func splitLines(src []byte) []srcLine {
	var out []srcLine
	i := 0
	for i <= len(src) {
		if i == len(src) {
			if i == 0 || src[i-1] == '\n' {
				break // no trailing empty line after a final newline
			}
		}
		j := i
		for j < len(src) && src[j] != '\n' {
			j++
		}
		text := string(src[i:j])
		text = strings.TrimSuffix(text, "\r")
		nlEnd := j
		if j < len(src) {
			nlEnd = j + 1
		}
		out = append(out, srcLine{start: i, nlEnd: nlEnd, text: text})
		if j >= len(src) {
			break
		}
		i = nlEnd
	}
	return out
}

// indentCols measures leading whitespace. A tab counts as four columns, a
// space as one. Mixing them within a file is the user's problem; consistent
// files of either kind nest correctly.
func indentCols(s string) int {
	n := 0
	for _, r := range s {
		switch r {
		case '\t':
			n += 4
		case ' ':
			n++
		default:
			return n
		}
	}
	return n
}

// contLine is a raw continuation line held until the block is finalised, at
// which point the whole group is dedented together so that relative
// indentation inside code fences survives.
type contLine struct {
	text    string
	indent  int
	inFence bool
	nlEnd   int
	blank   bool
}

type parser struct {
	src   []byte
	lines []srcLine

	roots []*Block

	// stack holds the open blocks by nesting level, together with the
	// indentation column of their bullet.
	stack []stackEntry

	cur      *Block
	curCont  []contLine
	curFence bool
	pending  []contLine // blank lines not yet known to belong to the block
}

type stackEntry struct {
	col   int
	block *Block
}

// Parse reads a file into a Document. It never fails: input that does not fit
// the dialect is preserved as block text rather than rejected, because the
// alternative is refusing to open the user's own notes.
func Parse(src []byte) *Document {
	p := &parser{src: src, lines: splitLines(src)}
	doc := &Document{Source: src}

	i := 0
	if fm, front, n := parseFrontmatter(p.lines); n > 0 {
		doc.Frontmatter = fm
		doc.FrontEnd = front
		i = n
	}

	for ; i < len(p.lines); i++ {
		ln := p.lines[i]
		trimmed := strings.TrimSpace(ln.text)

		if trimmed == "" && !p.curFence {
			if p.cur != nil {
				p.pending = append(p.pending, contLine{blank: true, nlEnd: ln.nlEnd})
			}
			continue
		}

		if m := bulletRe.FindStringSubmatch(ln.text); m != nil && !p.curFence {
			p.startBlock(indentCols(m[1]), m[2], ln)
			continue
		}

		if p.cur == nil {
			// Content before any bullet: a stray heading or paragraph. Treat it
			// as an implicit top-level block so nothing is ever dropped.
			p.startBlock(indentCols(ln.text), strings.TrimLeft(ln.text, " \t"), ln)
			continue
		}

		// A continuation line of the current block.
		p.curCont = append(p.curCont, p.pending...)
		p.pending = nil
		inFence := p.curFence
		if fenceRe.MatchString(strings.TrimLeft(ln.text, " \t")) {
			p.curFence = !p.curFence
			inFence = true // the fence marker itself is verbatim
		}
		p.curCont = append(p.curCont, contLine{
			text:    ln.text,
			indent:  indentCols(ln.text),
			inFence: inFence,
			nlEnd:   ln.nlEnd,
		})
	}

	p.finishBlock()
	p.closeTo(-1)
	doc.Blocks = p.roots
	return doc
}

func (p *parser) startBlock(col int, content string, ln srcLine) {
	p.finishBlock()

	b := &Block{Start: ln.start, SelfEnd: ln.nlEnd, End: ln.nlEnd}

	if m := anchorRe.FindStringSubmatch(content); m != nil {
		b.Anchor = m[1]
		content = content[:len(content)-len(m[0])]
	}
	b.Text = content

	p.closeTo(col)

	if len(p.stack) > 0 {
		parent := p.stack[len(p.stack)-1].block
		b.Parent = parent
		b.Depth = parent.Depth + 1
		parent.Children = append(parent.Children, b)
	} else {
		p.roots = append(p.roots, b)
	}

	p.stack = append(p.stack, stackEntry{col: col, block: b})
	p.cur = b
	p.curCont = nil
	p.curFence = false
	p.pending = nil
}

// closeTo pops the stack until the top entry is a strict ancestor of a bullet
// at column col, extending each popped block's End as it goes.
func (p *parser) closeTo(col int) {
	for len(p.stack) > 0 && p.stack[len(p.stack)-1].col >= col {
		p.stack = p.stack[:len(p.stack)-1]
	}
}

// finishBlock folds the buffered continuation lines into the current block and
// propagates its extent to its ancestors.
func (p *parser) finishBlock() {
	if p.cur == nil {
		return
	}
	b := p.cur

	if n := len(p.curCont); n > 0 {
		b.SelfEnd = p.curCont[n-1].nlEnd
	}

	// Dedent the group by the smallest indentation of any non-blank line, so
	// that indentation inside a code fence is preserved relative to the fence.
	min := -1
	for _, c := range p.curCont {
		if c.blank {
			continue
		}
		if min < 0 || c.indent < min {
			min = c.indent
		}
	}

	var textLines []string
	for _, c := range p.curCont {
		if c.blank {
			textLines = append(textLines, "")
			continue
		}
		line := dedent(c.text, min)
		if !c.inFence {
			if m := propRe.FindStringSubmatch(line); m != nil {
				b.Props = append(b.Props, Prop{Key: m[1], Value: strings.TrimSpace(m[2])})
				continue
			}
		}
		textLines = append(textLines, line)
	}

	for len(textLines) > 0 && textLines[len(textLines)-1] == "" {
		textLines = textLines[:len(textLines)-1]
	}
	if len(textLines) > 0 {
		if b.Text != "" {
			b.Text += "\n"
		}
		b.Text += strings.Join(textLines, "\n")
	}

	for _, e := range p.stack {
		if e.block.End < b.SelfEnd {
			e.block.End = b.SelfEnd
		}
	}
	if b.End < b.SelfEnd {
		b.End = b.SelfEnd
	}

	p.cur = nil
	p.curCont = nil
	p.curFence = false
}

// dedent removes up to n columns of leading whitespace.
func dedent(s string, n int) string {
	if n <= 0 {
		return s
	}
	col := 0
	i := 0
	for i < len(s) && col < n {
		switch s[i] {
		case '\t':
			col += 4
		case ' ':
			col++
		default:
			return s[i:]
		}
		i++
	}
	return s[i:]
}

func parseFrontmatter(lines []srcLine) (props []Prop, end int, consumed int) {
	if len(lines) == 0 || strings.TrimSpace(lines[0].text) != "---" {
		return nil, 0, 0
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i].text) == "---" {
			return props, lines[i].nlEnd, i + 1
		}
		if m := frontRe.FindStringSubmatch(lines[i].text); m != nil {
			props = append(props, Prop{Key: m[1], Value: strings.TrimSpace(m[2])})
		}
	}
	// Unterminated frontmatter is not frontmatter.
	return nil, 0, 0
}
