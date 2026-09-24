package tui

import (
	"fmt"

	"github.com/tilman-schieber/tlog/internal/markdown"
)

// Which rows are collapsed is remembered by position — "the second block at the
// top level" — because a block has no durable identity to remember it by. The
// alternatives were all worse: pointers and byte offsets are replaced on every
// write now that writing reloads, and materialising an anchor would mean
// collapsing a row writes to the file, which is the one thing a view action
// must never do.
//
// Positions renumber. Move a block up and "the second block" is a different
// block, so the state has to be carried across every time the tree changes or
// it lands on whatever moved into that slot.

// blockPath is the key for a block at index i under prefix. One definition,
// because a remap that disagreed with the walk that draws the rows would be
// invisible until it was wrong.
func blockPath(prefix string, i int) string { return fmt.Sprintf("%s%d.", prefix, i) }

// walkPaths returns every block in document order with its path, including the
// ones inside collapsed subtrees — they have positions too, and their own
// collapsed state to carry.
func walkPaths(d *markdown.Document) (paths []string, blocks []*markdown.Block) {
	var walk func(bs []*markdown.Block, prefix string)
	walk = func(bs []*markdown.Block, prefix string) {
		for i, b := range bs {
			p := blockPath(prefix, i)
			paths = append(paths, p)
			blocks = append(blocks, b)
			walk(b.Children, p)
		}
	}
	walk(d.Blocks, "")
	return paths, blocks
}

// carryCollapse moves the collapsed set from one shape of the tree to another.
//
// Blocks are matched by their text in document order, which is exact for every
// operation that only rearranges them — indent, outdent, move, delete — and is
// the best available guess for an edit that arrived from another editor. A
// block whose text changed loses its own state and keeps its children's, since
// each is matched on its own text rather than on its parent's.
//
// Entries for blocks that no longer exist, or that no longer have children to
// hide, are dropped rather than kept forever.
func carryCollapse(old map[string]bool, before, after *markdown.Document) map[string]bool {
	if len(old) == 0 {
		return map[string]bool{}
	}
	oldPaths, oldBlocks := walkPaths(before)
	newPaths, newBlocks := walkPaths(after)

	byText := map[string][]int{}
	for i, b := range newBlocks {
		byText[b.Text] = append(byText[b.Text], i)
	}

	out := map[string]bool{}
	// Every old block is walked, not only the collapsed ones, so that blocks
	// with identical text pair up in the order they appear rather than all
	// matching the first one.
	taken := map[string]int{}
	for i, ob := range oldBlocks {
		n := taken[ob.Text]
		cands := byText[ob.Text]
		if n >= len(cands) {
			continue // nothing left with this text: the block is gone
		}
		taken[ob.Text] = n + 1
		j := cands[n]
		if old[oldPaths[i]] && len(newBlocks[j].Children) > 0 {
			out[newPaths[j]] = true
		}
	}
	return out
}
