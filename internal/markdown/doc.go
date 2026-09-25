// Package markdown implements tlog's deliberately small markdown dialect:
// a nested bullet outline, optional YAML frontmatter, GFM task markers,
// [[wikilinks]], Obsidian-style ^anchors and key:: value block properties.
//
// It is not CommonMark and does not try to be. It parses what tlog writes and
// what the Logseq importer produces, and nothing else.
package markdown

import (
	"crypto/sha256"
	"encoding/hex"
)

// Prop is a single key:: value block property, or a key: value frontmatter
// entry. Order is preserved because these files are read by humans.
// The tags are not decoration: Prop travels in the read model like everything
// else, and without them it went over the wire as Key/Value while every other
// type used lowercase. The window reads p.key, got undefined, and threw while
// drawing — so no page with a block property could be drawn at all.
type Prop struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Block is the unit of editing. Its Text may span several lines: everything
// indented under the bullet that is not itself a bullet belongs to the block,
// code fences included.
type Block struct {
	// Text is the block's own content with the bullet marker stripped and
	// continuation lines dedented to column zero. It excludes property lines
	// and the trailing ^anchor.
	Text string

	// Anchor is the stable per-page block id, without the leading caret.
	// Empty until something needs to address this block permanently.
	Anchor string

	Props []Prop

	Children []*Block
	Parent   *Block

	// Depth is 0 for top-level blocks.
	Depth int

	// Source spans are byte offsets into the file this block was parsed from.
	// They are only meaningful for a Block obtained from Parse, and only until
	// the file changes underneath — which is what the file hash guards.
	Start   int // start of the bullet line, including its indentation
	SelfEnd int // just past the block's own last line, before the first child
	End     int // just past the last line of the last descendant
}

// Document is one parsed markdown file.
type Document struct {
	Frontmatter []Prop
	FrontEnd    int // byte offset just past the frontmatter, 0 if there is none
	Blocks      []*Block
	Source      []byte
}

// Hash is the checksum used for compare-and-swap writes. Every mutation
// carries the hash of the file it was computed against; if the file changed in
// the meantime the write is refused rather than merged.
func Hash(src []byte) string {
	sum := sha256.Sum256(src)
	return hex.EncodeToString(sum[:8])
}

// Prop returns the value of a block property and whether it was present.
func (b *Block) Prop(key string) (string, bool) {
	for _, p := range b.Props {
		if p.Key == key {
			return p.Value, true
		}
	}
	return "", false
}

// SetProp sets or appends a block property, preserving the order of existing
// ones.
func (b *Block) SetProp(key, value string) {
	for i := range b.Props {
		if b.Props[i].Key == key {
			b.Props[i].Value = value
			return
		}
	}
	b.Props = append(b.Props, Prop{Key: key, Value: value})
}

// DelProp removes a block property if present.
func (b *Block) DelProp(key string) {
	out := b.Props[:0]
	for _, p := range b.Props {
		if p.Key != key {
			out = append(out, p)
		}
	}
	b.Props = out
}

// Walk calls fn for every block in document order, depth first. Returning
// false from fn stops the walk.
func (d *Document) Walk(fn func(*Block) bool) {
	var rec func(bs []*Block) bool
	rec = func(bs []*Block) bool {
		for _, b := range bs {
			if !fn(b) {
				return false
			}
			if !rec(b.Children) {
				return false
			}
		}
		return true
	}
	rec(d.Blocks)
}

// Walk calls fn for this block and every descendant, depth first.
func (b *Block) Walk(fn func(*Block) bool) {
	if !fn(b) {
		return
	}
	for _, c := range b.Children {
		c.Walk(fn)
	}
}

// Siblings returns the slice the block lives in, and its index within it.
func (d *Document) Siblings(b *Block) ([]*Block, int) {
	list := d.Blocks
	if b.Parent != nil {
		list = b.Parent.Children
	}
	for i, s := range list {
		if s == b {
			return list, i
		}
	}
	return list, -1
}

// FindByAnchor returns the block carrying the given anchor.
func (d *Document) FindByAnchor(anchor string) *Block {
	var found *Block
	d.Walk(func(b *Block) bool {
		if b.Anchor == anchor {
			found = b
			return false
		}
		return true
	})
	return found
}

// FindByOffset returns the block whose bullet line starts at the given byte
// offset. This is the positional half of a block address; the file hash is the
// other half.
func (d *Document) FindByOffset(off int) *Block {
	var found *Block
	d.Walk(func(b *Block) bool {
		if b.Start == off {
			found = b
			return false
		}
		return true
	})
	return found
}
