package markdown

import "testing"

// edit operates on the tree, renders, and returns the resulting file. Every
// structural test is therefore also a golden test of the writer.
func edit(t *testing.T, src string, fn func(d *Document)) string {
	t.Helper()
	d := Parse([]byte(src))
	fn(d)
	return string(Render(d))
}

func find(t *testing.T, d *Document, text string) *Block {
	t.Helper()
	var b *Block
	d.Walk(func(x *Block) bool {
		if x.FirstLine() == text {
			b = x
			return false
		}
		return true
	})
	if b == nil {
		t.Fatalf("no block with text %q", text)
	}
	return b
}

func TestIndent(t *testing.T) {
	got := edit(t, "- a\n\n- b\n  - b1\n", func(d *Document) {
		if !d.Indent(find(t, d, "b")) {
			t.Fatal("indent refused")
		}
	})
	want := "- a\n  - b\n    - b1\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestIndentFirstSiblingIsNoOp(t *testing.T) {
	src := "- a\n  - b\n"
	got := edit(t, src, func(d *Document) {
		if d.Indent(find(t, d, "a")) {
			t.Fatal("indent of first sibling should be refused")
		}
	})
	if got != src {
		t.Fatalf("no-op changed the file: %q", got)
	}
}

func TestOutdentCarriesChildrenAndLeavesSiblings(t *testing.T) {
	src := "- p\n  - a\n    - a1\n  - b\n"
	got := edit(t, src, func(d *Document) {
		if !d.Outdent(find(t, d, "a")) {
			t.Fatal("outdent refused")
		}
	})
	want := "- p\n  - b\n\n- a\n  - a1\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestOutdentTopLevelIsNoOp(t *testing.T) {
	src := "- a\n"
	got := edit(t, src, func(d *Document) {
		if d.Outdent(find(t, d, "a")) {
			t.Fatal("top-level outdent should be refused")
		}
	})
	if got != src {
		t.Fatalf("no-op changed the file: %q", got)
	}
}

func TestMoveUpDownTakesSubtree(t *testing.T) {
	src := "- a\n\n- b\n  - b1\n\n- c\n"
	got := edit(t, src, func(d *Document) {
		if !d.MoveUp(find(t, d, "b")) {
			t.Fatal("move up refused")
		}
	})
	want := "- b\n  - b1\n\n- a\n\n- c\n"
	if got != want {
		t.Fatalf("up: got %q want %q", got, want)
	}

	got = edit(t, src, func(d *Document) {
		if !d.MoveDown(find(t, d, "b")) {
			t.Fatal("move down refused")
		}
	})
	want = "- a\n\n- c\n\n- b\n  - b1\n"
	if got != want {
		t.Fatalf("down: got %q want %q", got, want)
	}
}

func TestMoveAtEdgesIsNoOp(t *testing.T) {
	src := "- a\n\n- b\n"
	edit(t, src, func(d *Document) {
		if d.MoveUp(find(t, d, "a")) {
			t.Fatal("moving the first block up should be refused")
		}
		if d.MoveDown(find(t, d, "b")) {
			t.Fatal("moving the last block down should be refused")
		}
	})
}

func TestRemoveDeletesSubtree(t *testing.T) {
	got := edit(t, "- a\n  - a1\n    - a2\n\n- b\n", func(d *Document) {
		if err := d.Remove(find(t, d, "a")); err != nil {
			t.Fatal(err)
		}
	})
	if got != "- b\n" {
		t.Fatalf("got %q", got)
	}
}

func TestInsertAfterBeforeAndChild(t *testing.T) {
	got := edit(t, "- a\n", func(d *Document) {
		if err := d.InsertAfter(find(t, d, "a"), &Block{Text: "after"}); err != nil {
			t.Fatal(err)
		}
		if err := d.InsertBefore(find(t, d, "a"), &Block{Text: "before"}); err != nil {
			t.Fatal(err)
		}
		d.AppendChild(find(t, d, "a"), &Block{Text: "child"})
	})
	want := "- before\n\n- a\n  - child\n\n- after\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestAppendChildAtRoot(t *testing.T) {
	got := edit(t, "- a\n", func(d *Document) {
		d.AppendChild(nil, &Block{Text: "b"})
	})
	if got != "- a\n\n- b\n" {
		t.Fatalf("got %q", got)
	}
}

func TestEnsureAnchorIsLazyAndStable(t *testing.T) {
	d := Parse([]byte("- a\n\n- b ^fixed1\n"))
	a := find(t, d, "a")
	if a.Anchor != "" {
		t.Fatal("anchor materialised during parse")
	}
	first := d.EnsureAnchor(a)
	if first == "" || len(first) != 6 {
		t.Fatalf("bad anchor %q", first)
	}
	if again := d.EnsureAnchor(a); again != first {
		t.Fatalf("anchor changed on second call: %q then %q", first, again)
	}
	if d.FindByAnchor(first) != a {
		t.Fatal("FindByAnchor failed")
	}
	if d.EnsureAnchor(find(t, d, "b")) != "fixed1" {
		t.Fatal("existing anchor overwritten")
	}
}

func TestEditingTextDoesNotDisturbNeighbours(t *testing.T) {
	src := "- untouched one\n  with a continuation\n\n- target\n  status:: doing\n\n- untouched two\n  ```sh\n  echo hi\n  ```\n"
	got := edit(t, src, func(d *Document) {
		find(t, d, "target").Text = "target edited"
	})
	want := "- untouched one\n  with a continuation\n\n- target edited\n  status:: doing\n\n- untouched two\n  ```sh\n  echo hi\n  ```\n"
	if got != want {
		t.Fatalf("neighbours disturbed\ngot  %q\nwant %q", got, want)
	}
}

func TestTaskToggle(t *testing.T) {
	got := edit(t, "- [ ] a\n", func(d *Document) {
		if !d.Blocks[0].ToggleTask() {
			t.Fatal("toggle refused")
		}
	})
	if got != "- [x] a\n" {
		t.Fatalf("got %q", got)
	}
	got = edit(t, "- plain\n", func(d *Document) {
		d.Blocks[0].MakeTask()
	})
	if got != "- [ ] plain\n" {
		t.Fatalf("got %q", got)
	}
}

func TestPropertyEditing(t *testing.T) {
	got := edit(t, "- a\n  x:: 1\n  y:: 2\n", func(d *Document) {
		b := d.Blocks[0]
		b.SetProp("x", "9")
		b.SetProp("z", "3")
		b.DelProp("y")
	})
	want := "- a\n  x:: 9\n  z:: 3\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestHashChangesWithContent(t *testing.T) {
	if Hash([]byte("a")) == Hash([]byte("b")) {
		t.Fatal("hash collision on trivial input")
	}
	if Hash([]byte("a")) != Hash([]byte("a")) {
		t.Fatal("hash not deterministic")
	}
}
