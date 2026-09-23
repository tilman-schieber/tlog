package markdown

import (
	"regexp"
	"strings"
)

var (
	linkRe = regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)

	// Tags are parsed so that imported notes keep their meaning and links stay
	// clickable, but tlog builds no UI on top of them.
	tagRe = regexp.MustCompile(`(^|\s)#([A-Za-z][A-Za-z0-9_/\-]*)`)

	taskRe = regexp.MustCompile(`^\[( |x|X)\][ \t]+`)

	quoteRe = regexp.MustCompile(`^[ \t]*>[ \t]?`)

	// Fenced regions are excluded from inline parsing so that a [[link]] inside
	// a code sample is not treated as a reference.
	fenceBlockRe = regexp.MustCompile("(?s)```.*?```|~~~.*?~~~")
)

// Link is a [[Page]], [[Page#^anchor]] or [[Page #tag]] reference found in
// block text.
//
// Tags written inside the brackets say something about the page being linked
// to, not about the block doing the linking: [[Andreas #person]] means "Andreas
// is a person", asserted at the moment you first mention them. That is what
// makes a tag usable without ever navigating to the page to declare it.
type Link struct {
	Page   string
	Anchor string   // empty for a page-level link
	Tags   []string // types asserted about the target page, without the #
}

// String renders the link back to its source form.
func (l Link) String() string {
	var sb strings.Builder
	sb.WriteString("[[")
	sb.WriteString(l.Page)
	if l.Anchor != "" {
		sb.WriteString("#^")
		sb.WriteString(l.Anchor)
	}
	for _, t := range l.Tags {
		sb.WriteString(" #")
		sb.WriteString(t)
	}
	sb.WriteString("]]")
	return sb.String()
}

// stripLinks blanks out [[...]] spans, keeping the offsets of everything else.
func stripLinks(s string) string {
	return linkRe.ReplaceAllStringFunc(s, func(m string) string {
		return strings.Repeat(" ", len(m))
	})
}

func stripFences(s string) string {
	return fenceBlockRe.ReplaceAllStringFunc(s, func(m string) string {
		return strings.Repeat(" ", len(m))
	})
}

// Links returns every wikilink in the block's own text, in order, with
// duplicates preserved.
func (b *Block) Links() []Link {
	var out []Link
	for _, m := range linkRe.FindAllStringSubmatch(stripFences(b.Text), -1) {
		out = append(out, ParseLinkTarget(m[1]))
	}
	return out
}

// ParseLinkTarget splits the inside of a wikilink into page, anchor and the
// tags asserted about that page. Trailing #tokens are tags; #^ is an anchor and
// is never mistaken for one.
func ParseLinkTarget(s string) Link {
	s = strings.TrimSpace(s)

	var tags []string
	for {
		i := strings.LastIndexAny(s, " \t")
		if i < 0 {
			break
		}
		tok := s[i+1:]
		if len(tok) < 2 || tok[0] != '#' || !isTagName(tok[1:]) {
			break
		}
		tags = append([]string{tok[1:]}, tags...)
		s = strings.TrimRight(s[:i], " \t")
	}

	l := Link{Tags: tags}
	if i := strings.Index(s, "#^"); i >= 0 {
		l.Page = strings.TrimSpace(s[:i])
		l.Anchor = strings.TrimSpace(s[i+2:])
		return l
	}
	l.Page = s
	return l
}

// isTagName reports whether s is a usable tag: a letter followed by word
// characters. It deliberately rejects "^abc" so that an anchor is never read as
// a tag.
func isTagName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case i > 0 && (r >= '0' && r <= '9' || r == '_' || r == '-' || r == '/'):
		default:
			return false
		}
	}
	return true
}

// Tags returns the bare #tags in the block's own text: mentions of a tag by
// this block.
//
// Tags written inside [[brackets]] are excluded, because those say what the
// linked page is rather than what this block is about. Counting them here would
// fill a tag's page with one identical reference per page ever typed, next to
// the list that already says it better.
func (b *Block) Tags() []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range tagRe.FindAllStringSubmatch(stripLinks(stripFences(b.Text)), -1) {
		if !seen[m[2]] {
			seen[m[2]] = true
			out = append(out, m[2])
		}
	}
	return out
}

// Quote reports whether the block reads as a quotation: its first line begins
// with a > marker. Nothing is stored differently — a quote is an ordinary block
// whose text happens to start that way, so it needs no dialect of its own.
func (b *Block) Quote() bool {
	return quoteRe.MatchString(b.Text)
}

// QuoteBody returns the block's text with the > markers stripped from every
// line that has one. Pasted mail rarely marks every line, so lines without a
// marker are kept as they are.
func (b *Block) QuoteBody() string {
	lines := strings.Split(b.Text, "\n")
	for i, l := range lines {
		lines[i] = quoteRe.ReplaceAllString(l, "")
	}
	return strings.Join(lines, "\n")
}

// TaskState describes the GFM checkbox at the start of a block, if any.
type TaskState int

const (
	NotATask TaskState = iota
	TaskOpen
	TaskDone
)

// Task reports whether the block starts with a GFM task marker and its state.
func (b *Block) Task() TaskState {
	m := taskRe.FindStringSubmatch(b.Text)
	if m == nil {
		return NotATask
	}
	if m[1] == " " {
		return TaskOpen
	}
	return TaskDone
}

// TaskBody returns the block text with the task marker removed.
func (b *Block) TaskBody() string {
	return taskRe.ReplaceAllString(b.Text, "")
}

// ToggleTask flips an open task to done and back. It does nothing to blocks
// that are not tasks.
func (b *Block) ToggleTask() bool {
	switch b.Task() {
	case TaskOpen:
		b.Text = taskRe.ReplaceAllString(b.Text, "[x] ")
		return true
	case TaskDone:
		b.Text = taskRe.ReplaceAllString(b.Text, "[ ] ")
		return true
	}
	return false
}

// MakeTask turns a plain block into an open task.
func (b *Block) MakeTask() {
	if b.Task() == NotATask {
		b.Text = "[ ] " + b.Text
	}
}

// FirstLine returns the block's first line, which is what list views show.
func (b *Block) FirstLine() string {
	if i := strings.IndexByte(b.Text, '\n'); i >= 0 {
		return b.Text[:i]
	}
	return b.Text
}
