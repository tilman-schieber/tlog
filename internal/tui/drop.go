package tui

import (
	"fmt"
	"strings"

	"github.com/tilman-schieber/tlog/internal/app"
)

// dropFiles attaches files dropped on the terminal. They go on the shelf, and
// their links go where the drop happened: at the caret while typing, or as a
// block of their own below the cursor. An image is linked as one, so the
// desktop app shows the picture; here it is the line that says where it is.
func (m *Model) dropFiles(paths []string) {
	// Checked before anything is copied, so a drop with nowhere to go does not
	// leave a file on the shelf that no note mentions.
	b := m.current()
	if m.mode != modeInsert && m.feed != nil && b == nil {
		m.status = "put the cursor on a day first, then drop the file there"
		return
	}

	added, err := m.svc.Attach(paths...)
	if err != nil {
		m.fail(err)
		return
	}
	links := make([]string, 0, len(added))
	for _, a := range added {
		links = append(links, a.Link)
	}
	text := strings.Join(links, "\n")
	defer func() {
		if m.errMsg == "" {
			m.status = fmt.Sprintf("attached %s — on the shelf in %s", plural(len(added), "file"), m.svc.AttachDir())
		}
	}()

	if m.mode == modeInsert && m.ed != nil {
		m.ed.insert(text)
		return
	}

	if !m.ensureOnDisk() {
		return
	}
	var res *app.Result
	switch {
	case b == nil:
		d := m.docOf(nil)
		res, err = m.svc.AppendBlock(d.Rel, d.Hash, text)
	case b.Text == "" && len(b.Children) == 0 && len(b.Props) == 0:
		// The blank bullet of a fresh day is the place for it.
		res, err = m.svc.SetText(m.addrOf(b), text)
	default:
		res, err = m.svc.InsertAfter(m.addrOf(b), text, m.hasVisibleChildren(b))
	}
	m.apply(res, err)
}
