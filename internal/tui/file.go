package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/tilman-schieber/tlog/internal/app"
)

// Following a link to a plain-text file — an attached markdown document, a
// script — shows it here, read-only, instead of handing it to the desktop,
// which often has nothing that opens markdown and so would do nothing at all.
// o still hands it over, for when an editor is what you want.

type fileView struct {
	file *app.TextFile
	top  int
}

func (m *Model) showFile(f *app.TextFile) {
	m.file = &fileView{file: f}
	m.mode = modeFile
	m.errMsg = ""
	m.status = ""
}

// fileLines is the file as drawn: coloured by what it is, and wrapped to the
// window so nothing runs off the edge.
func fileLines(f *app.TextFile, width int) []string {
	w := max(20, width-1)
	if f.Lang == "markdown" {
		return markdownLines(f.Text, w)
	}
	raw := strings.Split(strings.TrimRight(highlightCode(f.Lang, f.Text), "\n"), "\n")
	var out []string
	for _, l := range raw {
		out = append(out, strings.Split(ansi.Wrap(l, w, ""), "\n")...)
	}
	return out
}

func (m *Model) fileViewString() string {
	f := m.file.file
	lines := fileLines(f, m.width)
	h := max(1, m.height-2) // a title line above, a footer below
	m.file.top = max(0, min(m.file.top, len(lines)-h))

	var b strings.Builder
	b.WriteString(pad(styleTitle.Render(f.Name), styleMuted.Render(tildeHome(f.Path)), m.width) + "\n")
	shown := 0
	for i := m.file.top; i < len(lines) && shown < h; i++ {
		b.WriteString(lines[i] + "\n")
		shown++
	}
	for ; shown < h; shown++ {
		b.WriteByte('\n')
	}
	where := "all of it"
	if len(lines) > h {
		where = fmt.Sprintf("%d%%", 100*min(m.file.top+h, len(lines))/len(lines))
	}
	foot := "↑↓ scroll · o open outside tlog · esc back · " + where
	if m.errMsg != "" {
		foot = styleError.Render(m.errMsg)
	}
	b.WriteString(styleMuted.Render(foot))
	return b.String()
}

func (m *Model) fileKey(k string) (tea.Model, tea.Cmd) {
	page := max(1, m.height-3)
	switch k {
	case "esc", "q", "ctrl+c", "b", "enter":
		m.mode = modeOutline
		m.file = nil
		m.errMsg = ""
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
