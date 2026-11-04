package tui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The help screen: what tlog is, in a few sentences, and then each thing it
// does with the keys that do it, side by side. A list of keys alone tells you
// what to press but not why you would, and the why is short.
//
// It is longer than a small terminal, so it scrolls; only the keys that close
// it close it, so scrolling is not a way to lose it.

// helpPart is a heading, a paragraph, or a key and what it does. A paragraph
// is wrapped to the window; a key's description is too, under itself.
type helpPart struct {
	heading string
	text    string
	key     string
}

func heading(s string) helpPart   { return helpPart{heading: s} }
func para(s string) helpPart      { return helpPart{text: s} }
func key(k, does string) helpPart { return helpPart{key: k, text: does} }

func (m *Model) helpParts() []helpPart {
	return []helpPart{
		para("Notes as an outline. Every line is a block, and blocks nest under each other. " +
			"Each day has a journal page, which is where tlog opens; other pages come into being " +
			"when you link to them. Everything is plain markdown in " + tildeHome(m.svc.Store.Root) +
			", committed to git as you go — edit it in any editor, and tlog notices."),

		heading("Writing"),
		para("Where tlog opens. Typing types into the block under the caret, and leaving a block saves it."),
		key("enter", "a new block, splitting this one at the caret"),
		key("alt+enter", "a new line inside the block (also ctrl+j)"),
		key("tab  shift+tab", "indent, outdent"),
		key("alt+↑  alt+↓", "move the block, with everything under it"),
		key("backspace", "at the start of a block, join it to the one above; an empty block goes"),
		key("↑ ↓ ← →", "move the caret; they run on from one block into the next"),
		key("ctrl+z  ctrl+y", "undo, redo: typing a word at a time, then each thing written"),
		key("esc", "stop writing and move around"),

		heading("Moving"),
		para("After esc. The block is highlighted and the header says MOVE; single letters go " +
			"places or act on the block. Enter starts writing again."),
		key("j k  ↑ ↓", "from block to block"),
		key("enter  i", "write here"),
		key("p", "go to: recent pages first, then commands, journals and pages by name — a name that does not exist yet makes the page"),
		key("/", "search every block"),
		key("t  [  ]", "today, the day before, the day after"),
		key("b", "back to where you came from"),
		key("g  o", "follow the link in this block: a page, a website, a file — a text file, such as an attached .md, opens right here, and so does a picture in a terminal that can draw one"),
		key("x", "tick the task off, or make the block one"),
		key("dd", "delete the block and everything under it — git keeps the old version"),
		key("z  h l", "fold and unfold"),
		key("a", "the agenda: everything with a deadline"),
		key("u  U", "undo, redo — a change on another page takes you there"),
		key("q", "quit"),

		heading("In either mode"),
		key("ctrl+p  ctrl+f", "go to, search"),
		key("ctrl+t  ctrl+o", "today, back"),
		key("alt+←  alt+→", "the day before, the day after"),
		key("ctrl+g  ctrl+d", "follow a link, tick a task"),
		key("ctrl+↑  ctrl+↓", "fold, unfold"),
		key("ctrl+r", "reload after the file was changed elsewhere"),
		key("F1  ctrl+q", "this page, quit"),

		heading("Mentioning"),
		para("Mention someone or something and it is linked. Everything that mentions a page is listed " +
			"under it, so going to a person shows every note about them."),
		key("@", "the pages you write about most recently, with their tags, then days. "+
			"@jr finds Julia Rausenberger, @fri Friday. A name nothing answers to makes a new page"),
		key("[[", "link any page; the ]] closes itself, and the match is fuzzy"),
		key("[[Ada #person]]", "link Ada and say she is a person — the tag shows beside her in the @ menu"),
		key("#moodle", "a topic: the block is listed on the moodle page; several at the end of a line read like hashtags"),
		key("((", "point at one block anywhere, found by a word in it; it is shown as that block's text"),

		heading("Tasks and dates"),
		para("Dates are typed the way you would say them — fri, morgen, in 3 days, next week, " +
			"7 nov, 24.12. — and always written down as 2026-11-07, so they sort and mean the same in a year."),
		key("/todo", "make the block a task; x or ctrl+d ticks it off"),
		key("/deadline fri", "give it a deadline; the menu offers the usual days before you type"),
		key("@fri", "a date in the text, picked from the same list, as a link to that day's journal"),
		key("a", "everything due, soonest first; tlog todo and tlog due print it in a shell"),

		heading("Links and files"),
		para("A [label](address) is drawn as its label alone; point at it with the mouse to see the address."),
		key("g  click", "open it: a website in the browser, a file in whatever opens it"),
		key("drop a file", "drag it onto the terminal: it is copied into the notes' assets/ and linked where you are"),
		key("/file", "link something already attached"),

		heading("The mouse"),
		key("click", "a block to write there, a link to open it, ▸ ▾ to fold, ☐ to tick"),
		key("wheel", "move through the page"),
		key("shift+drag", "select text, as in any program that uses the mouse"),

		heading("Commands"),
		para("Type / in a block for the commands that change it: /todo, /deadline, /date, /quote, " +
			"/code, /table, /file, /prop, /alias. The rest — agenda, backlinks, settings, messages — " +
			"are in p by name."),
	}
}

// helpLines lays the help out for a width: headings, wrapped paragraphs, and
// keys in a column with their descriptions wrapped beside them.
func (m *Model) helpLines(width int) []string {
	width = max(30, min(width, 90))
	const keyCol = 18
	var out []string
	for _, p := range m.helpParts() {
		switch {
		case p.heading != "":
			out = append(out, "", styleTitle.Render(p.heading))
		case p.key != "":
			wrapped := wrapWords(p.text, width-keyCol-4)
			for i, l := range wrapped {
				k := ""
				if i == 0 {
					k = p.key
				}
				out = append(out, "  "+styleLink.Render(fmt.Sprintf("%-*s", keyCol, k))+styleMuted.Render(l))
			}
		default:
			for _, l := range wrapWords(p.text, width-2) {
				out = append(out, "  "+l)
			}
		}
	}
	return out
}

// tildeHome writes a path under the home directory the way it is typed.
func tildeHome(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(p, home+"/") {
		return "~" + p[len(home):]
	}
	return p
}

// wrapWords breaks text into lines no wider than w, at spaces. A word longer
// than a line — a path, an address — is broken where it has to be rather than
// run off the edge of the window.
func wrapWords(text string, w int) []string {
	var lines []string
	line := ""
	var words []string
	for _, word := range strings.Fields(text) {
		for lipgloss.Width(word) > w {
			r := []rune(word)
			words = append(words, string(r[:w]))
			word = string(r[w:])
		}
		words = append(words, word)
	}
	for _, word := range words {
		switch {
		case line == "":
			line = word
		case lipgloss.Width(line)+1+lipgloss.Width(word) <= w:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

func (m *Model) helpView() string {
	lines := m.helpLines(m.width)
	h := max(1, m.height-2) // a title line above, a footer below
	m.helpTop = max(0, min(m.helpTop, len(lines)-h))

	var b strings.Builder
	b.WriteString(pad(styleTitle.Render("󰠮 tlog"), styleMuted.Render("help"), min(m.width, 90)) + "\n")
	shown := 0
	for i := m.helpTop; i < len(lines) && shown < h; i++ {
		b.WriteString(lines[i] + "\n")
		shown++
	}
	for ; shown < h; shown++ {
		b.WriteByte('\n')
	}
	where := "all of it"
	if len(lines) > h {
		where = fmt.Sprintf("%d%%", 100*min(m.helpTop+h, len(lines))/len(lines))
	}
	b.WriteString(styleMuted.Render("↑↓ scroll · esc close · " + where))
	return b.String()
}

// helpKey scrolls the help, and closes it only on the keys that mean close.
func (m *Model) helpKey(k string) (tea.Model, tea.Cmd) {
	page := max(1, m.height-3)
	switch k {
	case "esc", "q", "?", "f1", "ctrl+c", "enter":
		m.mode = modeOutline
		m.helpTop = 0
	case "down", "j":
		m.helpTop++
	case "up", "k":
		m.helpTop = max(0, m.helpTop-1)
	case "pgdown", " ", "ctrl+d":
		m.helpTop += page
	case "pgup", "ctrl+u":
		m.helpTop = max(0, m.helpTop-page)
	case "home", "g":
		m.helpTop = 0
	case "end", "G":
		m.helpTop = 1 << 20 // clamped when drawn
	}
	return m, nil
}
