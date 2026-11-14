package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	tapp "github.com/tilman-schieber/tlog/internal/app"
	"github.com/tilman-schieber/tlog/internal/dates"
)

// Every open task: what has a deadline first, soonest first, then the rest
// under "No date" — the same list `tlog todo` prints, from the same core call,
// so knowing what is left to do does not mean leaving the outliner. `a` adds
// what is done among the dated ones, as `tlog due -all` does.
//
// It is a view, not a page: nothing is written by looking at it, and enter on
// a line goes to the block itself.

type agenda struct {
	items []tapp.DueItem
	sel   int
	all   bool // include what is already done
}

// agendaItems is the open tasks, dated first; with all, the dated part also
// keeps what is done.
func (m *Model) agendaItems(all bool) ([]tapp.DueItem, error) {
	todos, err := m.svc.Todos()
	if err != nil || !all {
		return todos, err
	}
	items, err := m.svc.Due(true)
	if err != nil {
		return nil, err
	}
	for _, it := range todos {
		if it.Due == "" {
			items = append(items, it)
		}
	}
	return items, nil
}

func (m *Model) openAgenda() {
	items, err := m.agendaItems(false)
	if err != nil {
		m.errMsg = err.Error()
		return
	}
	m.agenda = &agenda{items: items}
	m.mode = modeAgenda
}

func (m *Model) agendaKey(k string) (tea.Model, tea.Cmd) {
	a := m.agenda
	switch k {
	case "esc", "q", "ctrl+c", "A":
		m.mode = modeOutline
		m.agenda = nil
	case "up", "k":
		a.sel = max(0, a.sel-1)
	case "down", "j":
		a.sel = min(a.sel+1, len(a.items)-1)
	case "a":
		// Everything, including what is finished — the same -all the command
		// line takes.
		a.all = !a.all
		items, err := m.agendaItems(a.all)
		if err != nil {
			m.errMsg = err.Error()
			return m, nil
		}
		a.items, a.sel = items, 0
	case " ":
		// Ticking a deadline off from the agenda is the point of having it on
		// one screen; the list is rebuilt because the item may now be gone.
		if a.sel < len(a.items) {
			it := a.items[a.sel]
			if _, err := m.svc.ToggleTask(it.Addr); err != nil {
				m.fail(err)
				return m, nil
			}
			items, err := m.agendaItems(a.all)
			if err == nil {
				a.items = items
				a.sel = min(a.sel, max(0, len(items)-1))
			}
			// The page being edited may be the one that just changed.
			if err := m.reloadFromDisk(); err != nil {
				m.errMsg = err.Error()
			}
		}
	case "enter":
		if a.sel < len(a.items) {
			it := a.items[a.sel]
			m.mode = modeOutline
			m.agenda = nil
			m.goTo(it.Rel)
			m.focusOffset(it.Offset)
		}
	}
	return m, nil
}

func (m *Model) agendaView() string {
	a := m.agenda
	var b strings.Builder

	title := "Agenda"
	if a.all {
		title = "Agenda — everything"
	}
	b.WriteString(styleTitle.Render(title))
	b.WriteString("  " + styleMuted.Render(plural(len(a.items), "item")) + "\n\n")

	if len(a.items) == 0 {
		b.WriteString(styleMuted.Render("Nothing to do. /todo or /deadline on a block puts it here.\n"))
	}

	textW := max(20, m.width-58)
	for i, it := range a.items {
		if it.Due == "" && (i == 0 || a.items[i-1].Due != "") {
			if i > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(" " + styleMuted.Bold(true).Render("No date") + "\n")
		}
		line := agendaLine(it, textW)
		if i == a.sel {
			b.WriteString(styleSelected.Render(" " + stripANSI(line)))
		} else {
			b.WriteString(" " + line)
		}
		b.WriteByte('\n')
	}

	b.WriteString("\n")
	b.WriteString(styleMuted.Render("↑↓ choose · enter go there · space tick off · a all · esc back"))
	return b.String()
}

// plural keeps "1 items" out of the interface.
func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

// agendaLine draws one task. How soon it is due sets the colour of its date,
// so the eye finds what is pressing without reading the labels: overdue in the
// error colour, today in the task colour, the coming week as plain text and
// anything later faded. The text is drawn as it is in the outline — links,
// tags, emphasis — and something done is faded as a whole.
func agendaLine(it tapp.DueItem, textW int) string {
	plain := padRight(it.Due, 12) + padRight(it.Label, 20)
	if it.Due == "" {
		plain = "" // under "No date", a blank date column is only a gap
	}
	date := plain
	switch it.State {
	case string(dates.Overdue):
		date = styleError.Bold(true).Render(date)
	case string(dates.Today):
		date = styleTodo.Bold(true).Render(date)
	case string(dates.Soon):
	default:
		date = styleMuted.Render(date)
	}
	text, _ := inlineWithLinks(it.Text, &refCursor{}, 0)
	if lipgloss.Width(text) > textW {
		text = ansi.Truncate(text, textW, "…")
	}
	if it.Done {
		return styleDone.Render("☑ " + plain + stripANSI(text) + "  " + it.Page)
	}
	return styleTodo.Render("☐") + " " + date + text + "  " + styleMuted.Render(it.Page)
}
