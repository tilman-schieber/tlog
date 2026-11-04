package tui

import (
	"errors"
	"fmt"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tilman-schieber/tlog/internal/app"
	"github.com/tilman-schieber/tlog/internal/termimg"
)

// Following a link to a picture draws it full screen, in a terminal that can
// show one — kitty and ghostty, foot and the other sixel terminals, iTerm2 and
// WezTerm. The outliner lets go of the terminal while it is up, so nothing
// redraws over the picture. Anywhere else, or when the picture cannot be
// decoded, it goes to the system's opener exactly as before.

type pictureClosedMsg struct {
	viewer *termimg.Viewer
	err    error
	rel    string // the note the link is in
	target string // the link as written
}

func (m *Model) showPicture(p *app.Picture, target string) {
	v := &termimg.Viewer{
		Path:     p.Path,
		Title:    p.Name,
		Subtitle: fmt.Sprintf("%d×%d · %s", p.Width, p.Height, tildeHome(filepath.Dir(p.Path))),
		Protocol: m.images,
	}
	rel := m.doc.Rel
	m.queued = tea.Exec(v, func(err error) tea.Msg {
		return pictureClosedMsg{viewer: v, err: err, rel: rel, target: target}
	})
}

// pictureClosed is the outliner taking the terminal back. A picture that
// could not be drawn goes to the opener, which is what following it did
// before there was a viewer.
func (m *Model) pictureClosed(msg pictureClosedMsg) {
	// What the terminal said when asked is kept, so it is asked once.
	if m.images == termimg.Ask && msg.viewer.Protocol != termimg.Ask {
		m.images = msg.viewer.Protocol
	}
	switch {
	case errors.Is(msg.err, termimg.ErrCannotShow):
		if err := m.svc.OpenLink(msg.rel, msg.target); err != nil {
			m.errMsg = err.Error()
			return
		}
		m.status = "opened " + shortAddress(msg.target)
	case msg.err != nil:
		m.errMsg = msg.err.Error()
	case msg.viewer.Opened:
		if err := m.svc.OpenLink("", msg.viewer.Path); err != nil {
			m.errMsg = err.Error()
			return
		}
		m.status = "opened " + msg.viewer.Title
	}
}
