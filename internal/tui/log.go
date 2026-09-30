package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// What the outliner has said. There is one status line and one error line, and
// both are replaced by the next thing that happens — so a push that failed
// while you were typing was gone before you looked up, with nothing to go back
// to. The window keeps a log; this is the same.

type notice struct {
	text string
	bad  bool
	at   time.Time
}

type logView struct{ top int }

func (m *Model) record(text string, bad bool) {
	if text == "" {
		return
	}
	if n := len(m.notices); n > 0 && m.notices[n-1].text == text {
		return // the same thing twice running is once
	}
	m.notices = append(m.notices, notice{text: text, bad: bad, at: time.Now()})
	if len(m.notices) > 200 {
		m.notices = m.notices[len(m.notices)-200:]
	}
}

func (m *Model) openLog() {
	m.log = &logView{}
	m.mode = modeLog
}

func (m *Model) logKey(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "esc", "q", "ctrl+c", "M":
		m.mode = modeNormal
		m.log = nil
	case "up", "k":
		m.log.top = max(0, m.log.top-1)
	case "down", "j":
		m.log.top = min(m.log.top+1, max(0, len(m.notices)-1))
	case "g":
		m.log.top = 0
	case "G":
		m.log.top = max(0, len(m.notices)-1)
	}
	return m, nil
}

func (m *Model) logView() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("Messages"))
	b.WriteString("  " + styleMuted.Render(plural(len(m.notices), "line")) + "\n\n")

	if len(m.notices) == 0 {
		b.WriteString(styleMuted.Render("Nothing has gone wrong yet.\n"))
	}

	// Newest first: the thing you came here to read is the last thing said.
	lines := make([]notice, 0, len(m.notices))
	for i := len(m.notices) - 1; i >= 0; i-- {
		lines = append(lines, m.notices[i])
	}
	h := max(1, m.height-6)
	for i := m.log.top; i < len(lines) && i < m.log.top+h; i++ {
		n := lines[i]
		when := styleMuted.Render(n.at.Format("15:04"))
		text := n.text
		if n.bad {
			text = styleError.Render(text)
		}
		b.WriteString(" " + when + "  " + text + "\n")
	}

	b.WriteString("\n")
	b.WriteString(styleMuted.Render("↑↓ scroll · esc back"))
	return b.String()
}
