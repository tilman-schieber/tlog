package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	tapp "github.com/tilman-schieber/tlog/internal/app"
	"github.com/tilman-schieber/tlog/internal/config"
)

// The settings screen. It edits the same list `tlog config` prints and the
// desktop app shows, so the three cannot drift apart or disagree about what a
// value means.

type settings struct {
	items []tapp.Setting
	sel   int
	ed    *editor // non-nil while a text value is being typed
	msg   string
}

func (m *Model) openSettings() {
	m.settings = &settings{items: tapp.Settings(m.svc.Cfg)}
	m.mode = modeSettings
	if m.svc.CfgErr != nil {
		m.settings.msg = m.svc.CfgErr.Error()
	}
}

func (m *Model) settingsKey(msg tea.KeyMsg, k string) (tea.Model, tea.Cmd) {
	s := m.settings

	// While typing a value, the keys belong to the little editor.
	if s.ed != nil {
		switch k {
		case "esc":
			s.ed = nil
		case "enter":
			m.applySetting(s.items[s.sel].Key, s.ed.String())
			s.ed = nil
		case "backspace":
			s.ed.backspace()
		default:
			if msg.Type == tea.KeyRunes {
				s.ed.insert(string(msg.Runes))
			} else if k == " " {
				s.ed.insert(" ")
			}
		}
		return m, nil
	}

	switch k {
	case "esc", "q", "ctrl+c":
		m.mode = modeNormal
		m.settings = nil
	case "up", "k":
		s.sel = max(0, s.sel-1)
		s.msg = ""
	case "down", "j":
		s.sel = min(s.sel+1, len(s.items)-1)
		s.msg = ""
	case "enter", " ":
		it := s.items[s.sel]
		if it.Kind == "bool" {
			cfg, err := tapp.ToggleSetting(m.svc.Cfg, it.Key)
			if err != nil {
				s.msg = err.Error()
				return m, nil
			}
			m.saveSettings(cfg)
			return m, nil
		}
		// Anything else is typed, starting from what it is now.
		s.ed = newEditor(it.Value)
	}
	return m, nil
}

func (m *Model) applySetting(key, value string) {
	cfg, err := tapp.SetSetting(m.svc.Cfg, key, value)
	if err != nil {
		m.settings.msg = err.Error()
		return
	}
	m.saveSettings(cfg)
}

// saveSettings writes the file and says what will not take effect until later,
// rather than letting a setting look applied when it is not.
func (m *Model) saveSettings(cfg config.Config) {
	before := m.svc.Cfg
	if err := m.svc.Reconfigure(cfg); err != nil {
		m.settings.msg = err.Error()
		return
	}
	m.settings.items = tapp.Settings(m.svc.Cfg)
	m.settings.msg = "gespeichert in " + m.svc.ConfigPath()
	if cfg.Notes != before.Notes || cfg.Attachments.Dir != before.Attachments.Dir {
		m.settings.msg = "gespeichert — wirkt beim nächsten Start"
	}
	if cfg.Format.BlankLines != before.Format.BlankLines {
		m.settings.msg = "gespeichert — Dateien werden beim nächsten Schreiben neu formatiert"
	}
}

func (m *Model) settingsView() string {
	s := m.settings
	var b strings.Builder

	b.WriteString(styleTitle.Render("Einstellungen"))
	b.WriteString("  " + styleMuted.Render(m.svc.ConfigPath()) + "\n\n")

	for i, it := range s.items {
		value := it.Value
		if s.ed != nil && i == s.sel {
			value = s.ed.String() + styleCursor.Render(" ")
		} else if it.Kind == "bool" {
			if it.Value == "true" {
				value = styleDone.Render("an")
			} else {
				value = styleMuted.Render("aus")
			}
		}

		line := padRight(it.Key, 22) + value
		if i == s.sel {
			b.WriteString(styleSelected.Render(" "+padRight(stripANSI(line), 46)) + "\n")
			b.WriteString("   " + styleMuted.Render(it.Hint) + "\n")
			if it.Note != "" {
				b.WriteString("   " + styleMuted.Render("— "+it.Note) + "\n")
			}
			continue
		}
		b.WriteString(" " + line + "\n")
	}

	b.WriteString("\n")
	if s.msg != "" {
		b.WriteString(styleMuted.Render(s.msg) + "\n")
	}
	if s.ed != nil {
		b.WriteString(styleMuted.Render("enter übernehmen · esc abbrechen"))
	} else {
		b.WriteString(styleMuted.Render("↑↓ wählen · enter ändern · esc zurück"))
	}
	return b.String()
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}
