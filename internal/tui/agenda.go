package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	tapp "github.com/tilman-schieber/tlog/internal/app"
	"github.com/tilman-schieber/tlog/internal/dates"
)

// Everything with a deadline, soonest first — the same list `tlog due` prints
// and the window shows, from the same core call. It was the one thing the
// window had that the outliner did not, which meant knowing what was due
// required leaving the outliner.
//
// It is a view, not a page: nothing is written by looking at it, and enter on
// a line goes to the block itself.

type agenda struct {
	items []tapp.DueItem
	sel   int
	all   bool // include what is already done
}

func (m *Model) openAgenda() {
	items, err := m.svc.Due(false)
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
		m.mode = modeNormal
		m.agenda = nil
	case "up", "k":
		a.sel = max(0, a.sel-1)
	case "down", "j":
		a.sel = min(a.sel+1, len(a.items)-1)
	case "a":
		// Everything, including what is finished — the same -all the command
		// line takes.
		a.all = !a.all
		items, err := m.svc.Due(a.all)
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
			items, err := m.svc.Due(a.all)
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
			m.mode = modeNormal
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
		b.WriteString(styleMuted.Render("Nothing is due. /deadline on a block puts it here.\n"))
	}

	for i, it := range a.items {
		box := "☐"
		if it.Done {
			box = "☑"
		}
		when := styleMuted.Render(padRight(it.Label, 20))
		switch it.State {
		case string(dates.Overdue):
			when = styleError.Render(padRight(it.Label, 20))
		case string(dates.Today):
			when = styleTodo.Render(padRight(it.Label, 20))
		}
		line := box + " " + padRight(it.Due, 12) + when +
			truncate(it.Text, max(20, m.width-58)) + "  " + styleMuted.Render(it.Page)
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
