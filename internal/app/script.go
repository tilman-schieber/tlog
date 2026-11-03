package app

import (
	"fmt"
	"strings"

	"github.com/tilman-schieber/tlog/internal/markdown"
	"github.com/tilman-schieber/tlog/internal/store"
)

// What a script needs that a person at a keyboard does not: the links in a
// block resolved to where they go, and an undo that works across processes.

// LinkView is one thing a block points at, resolved as far as it can be.
type LinkView struct {
	Kind   string `json:"kind"`   // "page", "block", "url" or "file"
	Target string `json:"target"` // as written: the page name, or the address

	// For a page or a block: the page's file, whether it exists, and — for a
	// block — where the block is and what it says.
	Rel    string `json:"rel,omitempty"`
	Exists bool   `json:"exists,omitempty"`
	Anchor string `json:"anchor,omitempty"`
	Addr   *Addr  `json:"addr,omitempty"`
	Text   string `json:"text,omitempty"`

	// For a url or a file: what opening it would open, or why it cannot be.
	Resolved string `json:"resolved,omitempty"`
	Error    string `json:"error,omitempty"`
	// Plain says a file is plain text, which tlog shows itself instead of
	// handing it to the desktop.
	Plain bool `json:"plain,omitempty"`
}

// Links resolves everything a block points at: [[pages]] to their files,
// [[Page#^anchor]] to the block behind it, and web and file links to what
// OpenLink would open. A link that cannot be followed is reported in place
// rather than failing the whole list — the other links are still answers.
func (s *Service) Links(a Addr) ([]LinkView, error) {
	d, b, err := s.Resolve(a)
	if err != nil {
		return nil, err
	}
	out := []LinkView{}
	for _, l := range b.Links() {
		v := LinkView{Kind: "page", Target: l.Page, Anchor: l.Anchor}
		rel, exists, rerr := s.ResolvePage(l.Page)
		if rerr != nil {
			v.Error = rerr.Error()
			out = append(out, v)
			continue
		}
		v.Rel, v.Exists = rel, exists
		if l.Anchor != "" {
			v.Kind = "block"
			if target, terr := s.Load(rel); terr == nil {
				if tb := target.Doc.FindByAnchor(l.Anchor); tb != nil {
					addr := Addr{Rel: rel, Offset: tb.Start, Hash: target.Hash}
					v.Addr, v.Text = &addr, tb.Text
				} else {
					v.Error = fmt.Sprintf("no block ^%s in %s", l.Anchor, l.Page)
				}
			}
		}
		out = append(out, v)
	}
	for _, u := range b.URLLinks() {
		v := LinkView{Kind: "url", Target: u.URL}
		resolved, rerr := s.ResolveLink(d.Rel, u.URL)
		if rerr != nil {
			v.Error = rerr.Error()
		} else {
			v.Resolved = resolved
			if !strings.Contains(resolved, "://") && !strings.HasPrefix(resolved, "mailto:") {
				v.Kind = "file"
				v.Plain = readText(resolved) != nil
			}
		}
		out = append(out, v)
	}
	return out, nil
}

// UndoCommitted takes back the last thing tlog committed to a file — or, with
// no file named, the last commit tlog made — for a caller that has no session
// to keep an undo history in. It is a write like any other: each file must
// still be as that commit left it, or the undo is refused rather than put over
// somebody's later change. A commit made by hand is left alone; git is the
// tool for that.
func (s *Service) UndoCommitted(rel string) (*Undone, error) {
	if rel != "" {
		if err := s.Store.Check(rel); err != nil {
			return nil, err
		}
	}
	hash, subject, ok, err := s.Git.LastCommit(rel)
	if err != nil {
		return nil, err
	}
	what := "by tlog"
	if rel != "" {
		what = "to " + rel
	}
	if !ok {
		return nil, refuse("nothing to undo: no commit %s", what)
	}
	if !strings.HasPrefix(subject, store.CommitPrefix) {
		return nil, refuse("the last change %s was not made by tlog (%q); undo it with git", what, subject)
	}

	files := []string{rel}
	if rel == "" {
		if files, err = s.Git.FilesIn(hash); err != nil {
			return nil, err
		}
	}
	var notes []string
	for _, f := range files {
		if strings.HasSuffix(f, ".md") {
			notes = append(notes, f)
		}
	}
	if len(notes) == 0 {
		return nil, refuse("nothing to undo in %s", subject)
	}

	// Every file is checked before any is written, so a refusal leaves the
	// notes as they were rather than half undone.
	type step struct {
		rel        string
		cur, after string
		before     []byte
	}
	var steps []step
	for _, f := range notes {
		cur, err := s.Store.Read(f)
		if err != nil {
			return nil, err
		}
		at, exists, err := s.Git.FileAt(hash, f)
		if err != nil {
			return nil, err
		}
		want := markdown.Hash(nil)
		if exists {
			want = markdown.Hash(at)
		}
		if cur.Hash != want {
			return nil, &store.ErrConflict{Rel: f, Expected: want, Actual: cur.Hash}
		}
		before, _, err := s.Git.FileAt(hash+"^", f)
		if err != nil {
			return nil, err
		}
		steps = append(steps, step{rel: f, cur: cur.Hash, before: before})
	}
	for _, st := range steps {
		if err := s.Store.Write(st.rel, st.before, st.cur); err != nil {
			return nil, err
		}
		s.Git.Touch(st.rel)
	}
	if err := s.Commit(); err != nil {
		return nil, err
	}
	return &Undone{Rels: notes, Rel: notes[len(notes)-1]}, nil
}
