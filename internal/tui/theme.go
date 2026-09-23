package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/tilman-schieber/tlog/internal/markdown"
)

// Semantic roles are mapped onto the sixteen ANSI colours and nothing else. No
// RGB is hardcoded anywhere, so tlog inherits whatever theme the terminal is
// wearing — Omarchy's on Linux, the terminal profile on macOS — and stays
// legible over ssh and inside tmux.
var (
	styleLink     = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	styleTag      = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	styleProp     = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	styleTodo     = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleDone     = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Faint(true)
	styleError    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styleMuted    = lipgloss.NewStyle().Faint(true)
	styleBullet   = lipgloss.NewStyle().Faint(true)
	styleTitle    = lipgloss.NewStyle().Bold(true)
	styleSelected = lipgloss.NewStyle().Reverse(true)
	styleMatch    = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
	styleCursor   = lipgloss.NewStyle().Reverse(true)
	styleFence    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	// Code uses the same semantic roles as everything else, so a snippet in a
	// note still inherits the terminal's theme.
	codeStyles = map[markdown.TokenKind]lipgloss.Style{
		markdown.TokKeyword: lipgloss.NewStyle().Foreground(lipgloss.Color("5")),
		markdown.TokType:    lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		markdown.TokString:  lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
		markdown.TokNumber:  lipgloss.NewStyle().Foreground(lipgloss.Color("6")),
		markdown.TokComment: lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true),
	}
)

// highlightCode colours a fenced snippet, leaving an unknown language plain.
func highlightCode(lang, code string) string {
	var sb strings.Builder
	for _, t := range markdown.Highlight(lang, code) {
		if st, ok := codeStyles[t.Kind]; ok {
			sb.WriteString(st.Render(t.Text))
			continue
		}
		sb.WriteString(t.Text)
	}
	return sb.String()
}

// csvGrid lays parsed rows out in aligned columns.
func csvGrid(rows [][]string) []string {
	if len(rows) == 0 {
		return nil
	}
	width := make([]int, len(rows[0]))
	for _, r := range rows {
		for i, c := range r {
			if i < len(width) && lipgloss.Width(c) > width[i] {
				width[i] = lipgloss.Width(c)
			}
		}
	}
	line := func(r []string, style lipgloss.Style) string {
		var cells []string
		for i, c := range r {
			if i >= len(width) {
				break
			}
			cells = append(cells, style.Render(c+strings.Repeat(" ", width[i]-lipgloss.Width(c))))
		}
		return strings.Join(cells, styleMuted.Render(" │ "))
	}
	out := []string{line(rows[0], styleTitle)}
	var rule []string
	for _, w := range width {
		rule = append(rule, strings.Repeat("─", w))
	}
	out = append(out, styleMuted.Render(strings.Join(rule, "─┼─")))
	for _, r := range rows[1:] {
		out = append(out, line(r, lipgloss.NewStyle()))
	}
	return out
}
