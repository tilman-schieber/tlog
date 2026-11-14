package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/tilman-schieber/tlog/internal/app"
	"github.com/tilman-schieber/tlog/internal/termimg"
)

// Following a link to a plain-text file — an attached markdown document, a
// script — shows it here, read-only, instead of handing it to the desktop,
// which often has nothing that opens markdown and so would do nothing at all.
// It is read-only until e hands it to $VISUAL or $EDITOR, in this terminal;
// when the editor quits the file is read again and drawn as it now is. o still
// hands it to the desktop.
//
// The links in a markdown file can be clicked like the ones in a note. One to
// another text file shows that file, and esc comes back; one to a #heading
// scrolls there; a [[page]] leaves for the outline; anything else goes where a
// link in a note would.

type fileView struct {
	file *app.TextFile
	top  int
	back *fileView // the file this one was reached from

	lines []mdLine // as last drawn, for the mouse
	width int
}

func (m *Model) showFile(f *app.TextFile) {
	m.file = &fileView{file: f}
	m.mode = modeFile
	m.errMsg = ""
	m.status = ""
}

// fileLines is the file as drawn: coloured by what it is, and wrapped to the
// window so nothing runs off the edge.
func fileLines(f *app.TextFile, width int) []mdLine {
	w := max(20, width-1)
	if f.Lang == "markdown" {
		return markdownLines(f.Text, w)
	}
	raw := strings.Split(strings.TrimRight(highlightCode(f.Lang, f.Text), "\n"), "\n")
	var out []mdLine
	for _, l := range raw {
		out = append(out, plainLines(strings.Split(ansi.Wrap(l, w, ""), "\n")...)...)
	}
	return out
}

func (fv *fileView) drawn(width int) []mdLine {
	if fv.lines == nil || fv.width != width {
		fv.lines, fv.width = fileLines(fv.file, width), width
	}
	return fv.lines
}

// fileRows is how many lines of the file fit: a title line above, a footer
// below.
func (m *Model) fileRows() int { return max(1, m.height-2) }

func (m *Model) fileViewString() string {
	f := m.file.file
	lines := m.file.drawn(m.width)
	h := m.fileRows()
	m.file.top = max(0, min(m.file.top, len(lines)-h))

	var b strings.Builder
	b.WriteString(pad(styleTitle.Render(f.Name), styleMuted.Render(tildeHome(f.Path)), m.width) + "\n")
	shown := 0
	for i := m.file.top; i < len(lines) && shown < h; i++ {
		b.WriteString(lines[i].s + "\n")
		shown++
	}
	for ; shown < h; shown++ {
		b.WriteByte('\n')
	}
	where := "all of it"
	if len(lines) > h {
		where = fmt.Sprintf("%d%%", 100*min(m.file.top+h, len(lines))/len(lines))
	}
	back := "esc back"
	if m.file.back != nil {
		back = "esc back to " + m.file.back.file.Name
	}
	foot := styleMuted.Render("↑↓ scroll · e edit · o open outside tlog · " + back + " · " + where)
	switch {
	case m.errMsg != "":
		foot = styleError.Render(m.errMsg)
	case m.hover != nil:
		foot = styleURL.Render(m.hover.target()) + styleMuted.Render("  click to open")
	}
	b.WriteString(foot)
	return b.String()
}

func (m *Model) fileKey(k string) (tea.Model, tea.Cmd) {
	page := max(1, m.height-3)
	switch k {
	case "esc", "q", "ctrl+c", "b", "enter":
		m.errMsg = ""
		if m.file.back != nil {
			m.file = m.file.back
			return m, nil
		}
		m.mode = modeOutline
		m.file = nil
	case "e":
		cmd := editorCommand(m.file.file.Path)
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return fileEditedMsg{err: err} })
	case "o":
		if err := m.svc.OpenLink("", m.file.file.Path); err != nil {
			m.errMsg = err.Error()
		} else {
			m.errMsg = ""
			m.status = "opened " + m.file.file.Name
		}
	case "down", "j":
		m.file.top++
	case "up", "k":
		m.file.top = max(0, m.file.top-1)
	case "pgdown", " ", "ctrl+d":
		m.file.top += page
	case "pgup", "ctrl+u":
		m.file.top = max(0, m.file.top-page)
	case "home", "g":
		m.file.top = 0
	case "end", "G":
		m.file.top = 1 << 20 // clamped when drawn
	}
	return m, nil
}

func (m *Model) fileMouse(msg tea.MouseMsg) {
	line := -1
	if msg.Y >= 1 && msg.Y <= m.fileRows() {
		line = m.file.top + msg.Y - 1 // the title is the first screen line
	}
	switch {
	case msg.Button == tea.MouseButtonWheelUp:
		m.file.top = max(0, m.file.top-3)
	case msg.Button == tea.MouseButtonWheelDown:
		m.file.top += 3 // clamped when drawn
	case msg.Action == tea.MouseActionMotion:
		m.hover = m.fileHitAt(line, msg.X)
	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
		if h := m.fileHitAt(line, msg.X); h != nil {
			m.hover = nil
			m.followFromFile(*h)
		}
	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonRight:
		if h := m.fileHitAt(line, msg.X); h != nil {
			m.copyItem(pickItem{url: h.url, link: h.page})
		}
	}
}

func (m *Model) fileHitAt(line, x int) *linkHit {
	lines := m.file.drawn(m.width)
	if line < 0 || line >= len(lines) {
		return nil
	}
	for _, h := range lines[line].hits {
		if x >= h.from && x < h.to {
			return &h
		}
	}
	return nil
}

// followFromFile follows a link drawn in the file being shown. A relative
// address is relative to that file, which the core resolves; everything after
// works from the absolute path, so a picture's fallback to the desktop opens
// the same file the click meant.
func (m *Model) followFromFile(h linkHit) {
	if h.url == "" {
		m.mode = modeOutline
		m.file = nil
		m.openLink(h.page)
		return
	}
	target, anchor := h.url, ""
	if i := strings.Index(target, "#"); i >= 0 && !strings.Contains(target[:i], ":") {
		target, anchor = target[:i], target[i+1:]
	}
	if target == "" {
		m.scrollToHeading(anchor)
		return
	}
	resolved, err := m.svc.ResolveLink(m.file.file.Rel, target)
	if err != nil {
		m.errMsg = err.Error()
		return
	}
	if filepath.IsAbs(resolved) {
		if f, err := m.svc.LinkedText("", resolved); err == nil && f != nil {
			m.file = &fileView{file: f, back: m.file}
			m.errMsg = ""
			if anchor != "" {
				m.scrollToHeading(anchor)
			}
			return
		}
		if m.images != termimg.None {
			if p, err := m.svc.LinkedPicture("", resolved); err == nil && p != nil {
				m.showPicture(p, resolved)
				return
			}
		}
	}
	if err := m.svc.OpenLink("", resolved); err != nil {
		m.errMsg = err.Error()
		return
	}
	m.errMsg = ""
	m.status = "opened " + shortAddress(resolved)
}

// scrollToHeading brings the heading a #anchor names to the top.
func (m *Model) scrollToHeading(anchor string) {
	anchor = strings.ToLower(anchor)
	for i, l := range m.file.drawn(m.width) {
		if l.slug != "" && l.slug == anchor {
			m.file.top = i
			m.errMsg = ""
			return
		}
	}
	m.errMsg = "no heading #" + anchor + " in " + m.file.file.Name
}

// fileEditedMsg says the editor a shown file was handed to has quit.
type fileEditedMsg struct{ err error }

func (m *Model) fileEdited(msg fileEditedMsg) {
	if m.file == nil {
		return
	}
	f, changed, err := m.svc.Reread(m.file.file)
	m.file.file, m.file.lines = f, nil
	switch {
	case msg.err != nil:
		m.errMsg = "the editor: " + msg.err.Error()
	case err != nil:
		m.errMsg = err.Error()
	case changed:
		m.errMsg, m.status = "", "saved "+f.Name
	default:
		m.errMsg, m.status = "", f.Name+" is unchanged"
	}
}

// editorCommand is the editor the environment names, which may carry
// arguments of its own ("code --wait"), or vi, which every system has.
func editorCommand(path string) *exec.Cmd {
	for _, v := range []string{"VISUAL", "EDITOR"} {
		if words := strings.Fields(os.Getenv(v)); len(words) > 0 {
			return exec.Command(words[0], append(words[1:], path)...)
		}
	}
	return exec.Command("vi", path)
}
