package app

import (
	"errors"
	"fmt"

	"github.com/tilman-schieber/tlog/internal/markdown"
)

// RefusedError is a move that does not exist: outdenting a block that is
// already at the top level, merging the first block into nothing. It is not a
// failure — nothing broke and nothing was lost, the key simply had nowhere to
// go — and an adapter should say so quietly rather than raise an alarm.
//
// The distinction belongs here because the core is what knows it. Left to the
// adapters, each would have to guess from the wording of a message.
type RefusedError struct{ Msg string }

func (e *RefusedError) Error() string { return e.Msg }

func refuse(format string, a ...any) error {
	return &RefusedError{Msg: fmt.Sprintf(format, a...)}
}

// Refused reports whether a mutation was declined because the move does not
// exist, as opposed to having gone wrong.
func Refused(err error) bool {
	var r *RefusedError
	return errors.As(err, &r)
}

// Stale reports whether a mutation was refused because the file had moved on
// since the address was computed — the one failure that means "re-read".
func Stale(err error) bool {
	var e *staleErr
	return errors.As(err, &e)
}

// Block mutations live here rather than in any adapter. Each one takes an
// address carrying the hash of the file it was computed against, so a caller
// that has fallen behind is refused rather than allowed to overwrite. Each
// returns where the block ended up, because rewriting the file moves every
// offset after it.

// Result says what a mutation did: the file's new hash, and where the block it
// touched now starts. Both are needed to address the same block again.
type Result struct {
	Rel    string
	Hash   string
	Offset int
}

// mutate resolves an address, applies a change to the tree, writes the file and
// reports where the block landed.
func (s *Service) mutate(a Addr, fn func(*markdown.Document, *markdown.Block) error) (*Result, error) {
	d, b, err := s.Resolve(a)
	if err != nil {
		return nil, err
	}
	if err := fn(d.Doc, b); err != nil {
		return nil, err
	}
	return s.commit(d, b)
}

// commit saves and re-reads, translating a block identity into a fresh offset.
func (s *Service) commit(d *Doc, b *markdown.Block) (*Result, error) {
	idx := -1
	for i, x := range d.Doc.Flatten() {
		if x == b {
			idx = i
			break
		}
	}
	if err := s.Save(d); err != nil {
		return nil, err
	}
	fresh, err := s.Load(d.Rel)
	if err != nil {
		return nil, err
	}
	res := &Result{Rel: d.Rel, Hash: fresh.Hash}
	flat := fresh.Doc.Flatten()
	if idx >= 0 && idx < len(flat) {
		res.Offset = flat[idx].Start
	} else if len(flat) > 0 {
		res.Offset = flat[len(flat)-1].Start
	}
	return res, nil
}

// SetText replaces a block's own text, leaving its children alone.
func (s *Service) SetText(a Addr, text string) (*Result, error) {
	return s.mutate(a, func(_ *markdown.Document, b *markdown.Block) error {
		b.Text = text
		return nil
	})
}

// ToggleTask turns a plain block into an open task, an open task into a done
// one, and back.
func (s *Service) ToggleTask(a Addr) (*Result, error) {
	return s.mutate(a, func(_ *markdown.Document, b *markdown.Block) error {
		if !b.ToggleTask() {
			b.MakeTask()
		}
		return nil
	})
}

// SetProperty sets a block property; an empty value removes it.
func (s *Service) SetProperty(a Addr, key, value string) (*Result, error) {
	return s.mutate(a, func(_ *markdown.Document, b *markdown.Block) error {
		if key == "" {
			return fmt.Errorf("a property needs a name")
		}
		if value == "" {
			b.DelProp(key)
			return nil
		}
		b.SetProp(key, value)
		return nil
	})
}

// Indent makes a block the last child of its previous sibling.
func (s *Service) Indent(a Addr) (*Result, error) {
	return s.mutate(a, func(d *markdown.Document, b *markdown.Block) error {
		if !d.Indent(b) {
			return refuse("nothing above to nest under")
		}
		return nil
	})
}

// Outdent makes a block the next sibling of its parent.
func (s *Service) Outdent(a Addr) (*Result, error) {
	return s.mutate(a, func(d *markdown.Document, b *markdown.Block) error {
		if !d.Outdent(b) {
			return refuse("already at the top level")
		}
		return nil
	})
}

// Move shifts a block among its siblings, carrying its children.
func (s *Service) Move(a Addr, delta int) (*Result, error) {
	return s.mutate(a, func(d *markdown.Document, b *markdown.Block) error {
		ok := false
		switch {
		case delta < 0:
			ok = d.MoveUp(b)
		case delta > 0:
			ok = d.MoveDown(b)
		default:
			return nil
		}
		if !ok {
			return refuse("no sibling in that direction")
		}
		return nil
	})
}

// DeleteBlock removes a block and everything under it.
func (s *Service) DeleteBlock(a Addr) (*Result, error) {
	d, b, err := s.Resolve(a)
	if err != nil {
		return nil, err
	}
	idx := -1
	for i, x := range d.Doc.Flatten() {
		if x == b {
			idx = i
			break
		}
	}
	if err := d.Doc.Remove(b); err != nil {
		return nil, err
	}
	if err := s.Save(d); err != nil {
		return nil, err
	}
	fresh, err := s.Load(a.Rel)
	if err != nil {
		return nil, err
	}
	res := &Result{Rel: a.Rel, Hash: fresh.Hash}
	flat := fresh.Doc.Flatten()
	if len(flat) > 0 {
		// Land on whatever took its place, or the block before it.
		if idx >= len(flat) {
			idx = len(flat) - 1
		}
		if idx < 0 {
			idx = 0
		}
		res.Offset = flat[idx].Start
	}
	return res, nil
}

// InsertAfter adds a block below another one.
//
// asChild makes it the first child instead, which is what an outliner does when
// the block above has children you can see. The adapter decides rather than the
// core, because the answer turns on whether those children are *visible* —
// collapse is view state, and a core that guessed would disagree with whichever
// adapter guessed differently. It used to, which is why this is a parameter.
func (s *Service) InsertAfter(a Addr, text string, asChild bool) (*Result, error) {
	d, b, err := s.Resolve(a)
	if err != nil {
		return nil, err
	}
	nb := &markdown.Block{Text: text}
	if asChild && len(b.Children) > 0 {
		b.Children = append([]*markdown.Block{nb}, b.Children...)
		nb.Parent = b
		d.Doc.Reindex()
	} else if err := d.Doc.InsertAfter(b, nb); err != nil {
		return nil, err
	}
	return s.commit(d, nb)
}

// InsertBefore adds a sibling above a block.
func (s *Service) InsertBefore(a Addr, text string) (*Result, error) {
	d, b, err := s.Resolve(a)
	if err != nil {
		return nil, err
	}
	nb := &markdown.Block{Text: text}
	if err := d.Doc.InsertBefore(b, nb); err != nil {
		return nil, err
	}
	return s.commit(d, nb)
}

// AppendBlock adds a block at the end of a file, guarded by the file's hash.
// Passing an empty hash writes unconditionally, which only a caller that knows
// nothing else can be holding the file should do.
func (s *Service) AppendBlock(rel, ifMatch, text string) (*Result, error) {
	d, err := s.Load(rel)
	if err != nil {
		return nil, err
	}
	if ifMatch != "" && d.Hash != ifMatch {
		return nil, &staleErr{rel: rel, expected: ifMatch, actual: d.Hash}
	}
	nb := &markdown.Block{Text: text}
	if len(d.Doc.Blocks) == 1 && d.Doc.Blocks[0].Text == "" && len(d.Doc.Blocks[0].Children) == 0 {
		d.Doc.Blocks[0].Text = text
		nb = d.Doc.Blocks[0]
	} else {
		d.Doc.AppendChild(nil, nb)
	}
	return s.commit(d, nb)
}

type staleErr struct {
	rel, expected, actual string
}

func (e *staleErr) Error() string {
	return fmt.Sprintf("%s changed on disk (expected %s, found %s); re-read and retry", e.rel, e.expected, e.actual)
}

// SplitBlock ends a block at the caret and starts the next one with the rest.
//
// It is one method rather than SetText followed by InsertAfter because enter is
// the most frequent key in an outliner: two calls would be two writes, two
// commits, and a window in which the file can move between them, leaving half
// the split behind.
//
// asChild says where the new block goes when the old one has children. The
// adapter decides, because the answer depends on whether those children are
// *visible* — collapse is view state and the core has never been told about it.
func (s *Service) SplitBlock(a Addr, before, after string, asChild bool) (*Result, error) {
	d, b, err := s.Resolve(a)
	if err != nil {
		return nil, err
	}
	b.Text = before
	nb := &markdown.Block{Text: after}
	if asChild && len(b.Children) > 0 {
		b.Children = append([]*markdown.Block{nb}, b.Children...)
		nb.Parent = b
		d.Doc.Reindex()
	} else if err := d.Doc.InsertAfter(b, nb); err != nil {
		return nil, err
	}
	return s.commit(d, nb)
}

// MergeIntoPrevious joins a block onto the one above it in the file and removes
// it — what backspace at the start of a block means in an outliner.
//
// Refused when the block has children: they would have nowhere to go, and
// picking a place for them silently is worse than saying so.
func (s *Service) MergeIntoPrevious(a Addr) (*Result, error) {
	d, b, err := s.Resolve(a)
	if err != nil {
		return nil, err
	}
	if len(b.Children) > 0 {
		return nil, refuse("cannot merge a block that has children — outdent them first")
	}

	flat := d.Doc.Flatten()
	idx := -1
	for i, x := range flat {
		if x == b {
			idx = i
			break
		}
	}
	if idx <= 0 {
		return nil, refuse("nothing above to merge into")
	}
	prev := flat[idx-1]

	prev.Text += b.Text
	if err := d.Doc.Remove(b); err != nil {
		return nil, err
	}
	return s.commit(d, prev)
}
