package markdown

import (
	"crypto/rand"
	"errors"
	"math/big"
)

// ErrNotFound is returned when a block is not part of the document it is being
// edited in.
var ErrNotFound = errors.New("block not in document")

const anchorAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"

// NewAnchor returns a short random block anchor. Anchors only need to be unique
// within their page, because every reference to one is page-qualified
// ([[Page#^anchor]]), so six characters is ample.
func NewAnchor() string {
	b := make([]byte, 6)
	max := big.NewInt(int64(len(anchorAlphabet)))
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			// crypto/rand does not fail in practice; degrade rather than panic
			// in the middle of someone's note.
			b[i] = anchorAlphabet[i%len(anchorAlphabet)]
			continue
		}
		b[i] = anchorAlphabet[n.Int64()]
	}
	return string(b)
}

// EnsureAnchor gives a block a stable anchor if it does not already have one,
// and returns it. This is the lazy-identity rule: anchors are materialised only
// when something needs to name the block permanently, never as a side effect of
// reading.
func (d *Document) EnsureAnchor(b *Block) string {
	if b.Anchor != "" {
		return b.Anchor
	}
	taken := map[string]bool{}
	d.Walk(func(x *Block) bool {
		if x.Anchor != "" {
			taken[x.Anchor] = true
		}
		return true
	})
	for {
		a := NewAnchor()
		if !taken[a] {
			b.Anchor = a
			return a
		}
	}
}

// Reindex recomputes Parent and Depth for the whole tree. Every structural
// operation ends with it. Source offsets are deliberately not recomputed: they
// belong to the bytes the document was parsed from and are stale the moment the
// tree changes, so callers re-parse after writing.
func (d *Document) Reindex() {
	var rec func(bs []*Block, parent *Block, depth int)
	rec = func(bs []*Block, parent *Block, depth int) {
		for _, b := range bs {
			b.Parent = parent
			b.Depth = depth
			rec(b.Children, b, depth+1)
		}
	}
	rec(d.Blocks, nil, 0)
}

func (d *Document) listFor(parent *Block) *[]*Block {
	if parent == nil {
		return &d.Blocks
	}
	return &parent.Children
}

func removeFrom(list *[]*Block, b *Block) bool {
	for i, x := range *list {
		if x == b {
			*list = append((*list)[:i:i], (*list)[i+1:]...)
			return true
		}
	}
	return false
}

func indexOf(list []*Block, b *Block) int {
	for i, x := range list {
		if x == b {
			return i
		}
	}
	return -1
}

// detach removes a block (with its subtree) from its current position.
func (d *Document) detach(b *Block) error {
	if !removeFrom(d.listFor(b.Parent), b) {
		return ErrNotFound
	}
	return nil
}

// InsertAfter places nb as the next sibling of ref.
func (d *Document) InsertAfter(ref, nb *Block) error {
	list := d.listFor(ref.Parent)
	i := indexOf(*list, ref)
	if i < 0 {
		return ErrNotFound
	}
	nb.Parent = ref.Parent
	*list = append((*list)[:i+1], append([]*Block{nb}, (*list)[i+1:]...)...)
	d.Reindex()
	return nil
}

// InsertBefore places nb as the previous sibling of ref.
func (d *Document) InsertBefore(ref, nb *Block) error {
	list := d.listFor(ref.Parent)
	i := indexOf(*list, ref)
	if i < 0 {
		return ErrNotFound
	}
	nb.Parent = ref.Parent
	*list = append((*list)[:i], append([]*Block{nb}, (*list)[i:]...)...)
	d.Reindex()
	return nil
}

// AppendChild places nb as the last child of parent, or as the last top-level
// block when parent is nil.
func (d *Document) AppendChild(parent, nb *Block) {
	list := d.listFor(parent)
	nb.Parent = parent
	*list = append(*list, nb)
	d.Reindex()
}

// Remove deletes a block and everything under it.
func (d *Document) Remove(b *Block) error {
	if err := d.detach(b); err != nil {
		return err
	}
	d.Reindex()
	return nil
}

// Indent makes a block the last child of its previous sibling. It is a no-op
// when there is no previous sibling, because there is nothing to nest under.
func (d *Document) Indent(b *Block) bool {
	list := *d.listFor(b.Parent)
	i := indexOf(list, b)
	if i <= 0 {
		return false
	}
	prev := list[i-1]
	_ = d.detach(b)
	b.Parent = prev
	prev.Children = append(prev.Children, b)
	d.Reindex()
	return true
}

// Outdent makes a block the next sibling of its parent, carrying its own
// children with it. Blocks that followed it stay where they were: predictable
// beats clever, and nothing silently changes parent behind your back.
func (d *Document) Outdent(b *Block) bool {
	parent := b.Parent
	if parent == nil {
		return false
	}
	_ = d.detach(b)
	grand := parent.Parent
	list := d.listFor(grand)
	i := indexOf(*list, parent)
	if i < 0 {
		// Should not happen; put it back rather than lose it.
		parent.Children = append(parent.Children, b)
		d.Reindex()
		return false
	}
	b.Parent = grand
	*list = append((*list)[:i+1], append([]*Block{b}, (*list)[i+1:]...)...)
	d.Reindex()
	return true
}

// MoveUp swaps a block with its previous sibling, subtree included.
func (d *Document) MoveUp(b *Block) bool {
	list := d.listFor(b.Parent)
	i := indexOf(*list, b)
	if i <= 0 {
		return false
	}
	(*list)[i-1], (*list)[i] = (*list)[i], (*list)[i-1]
	d.Reindex()
	return true
}

// MoveDown swaps a block with its next sibling, subtree included.
func (d *Document) MoveDown(b *Block) bool {
	list := d.listFor(b.Parent)
	i := indexOf(*list, b)
	if i < 0 || i >= len(*list)-1 {
		return false
	}
	(*list)[i], (*list)[i+1] = (*list)[i+1], (*list)[i]
	d.Reindex()
	return true
}

// Flatten returns every block in document order.
func (d *Document) Flatten() []*Block {
	var out []*Block
	d.Walk(func(b *Block) bool {
		out = append(out, b)
		return true
	})
	return out
}

// SetFrontmatter sets or appends a frontmatter entry.
func (d *Document) SetFrontmatter(key, value string) {
	for i := range d.Frontmatter {
		if d.Frontmatter[i].Key == key {
			d.Frontmatter[i].Value = value
			return
		}
	}
	d.Frontmatter = append(d.Frontmatter, Prop{Key: key, Value: value})
}
