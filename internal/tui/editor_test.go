package tui

import "testing"

func TestEditorInsertAndDelete(t *testing.T) {
	e := newEditor("")
	e.insert("hello")
	if e.String() != "hello" || e.cur != 5 {
		t.Fatalf("%q cur=%d", e.String(), e.cur)
	}
	e.backspace()
	if e.String() != "hell" {
		t.Fatalf("%q", e.String())
	}
	e.home()
	e.del()
	if e.String() != "ell" {
		t.Fatalf("%q", e.String())
	}
	if e.backspace() {
		t.Fatal("backspace at position zero should report nothing to delete")
	}
}

func TestEditorVerticalMovementKeepsTheGoalColumn(t *testing.T) {
	e := newEditor("aaaaa\nb\nccccc")
	e.end() // end of the last line, column 5
	if !e.up() {
		t.Fatal("up refused")
	}
	if _, col := e.cursorPos(); col != 1 {
		t.Fatalf("short line should clamp to its end, col=%d", col)
	}
	if !e.up() {
		t.Fatal("up refused")
	}
	// The goal column survives the short line in between.
	if _, col := e.cursorPos(); col != 5 {
		t.Fatalf("goal column lost, col=%d", col)
	}
	if e.up() {
		t.Fatal("up from the first line should report the edge")
	}
	e.end()
	e.down()
	e.down()
	if e.down() {
		t.Fatal("down from the last line should report the edge")
	}
}

func TestEditorCursorPos(t *testing.T) {
	e := newEditor("ab\ncd")
	e.home()
	if l, c := e.cursorPos(); l != 1 || c != 0 {
		t.Fatalf("line=%d col=%d", l, c)
	}
	e.cur = 0
	if l, c := e.cursorPos(); l != 0 || c != 0 {
		t.Fatalf("line=%d col=%d", l, c)
	}
}

func TestEditorSplit(t *testing.T) {
	e := newEditor("onetwo")
	e.cur = 3
	before, after := e.split()
	if before != "one" || after != "two" {
		t.Fatalf("%q %q", before, after)
	}
}

func TestEditorDeleteWord(t *testing.T) {
	e := newEditor("one two  three")
	e.deleteWord()
	if e.String() != "one two  " {
		t.Fatalf("%q", e.String())
	}
	e.deleteWord()
	if e.String() != "one " {
		t.Fatalf("%q", e.String())
	}
}

func TestEditorReplaceBefore(t *testing.T) {
	e := newEditor("see [[Pro")
	e.replaceBefore(3, "Project Foo]]")
	if e.String() != "see [[Project Foo]]" {
		t.Fatalf("%q", e.String())
	}
	if !e.atEnd() {
		t.Fatal("cursor should follow the inserted text")
	}
}

func TestEditorHandlesMultibyteText(t *testing.T) {
	e := newEditor("Grüße 日本")
	e.end()
	e.backspace()
	if e.String() != "Grüße 日" {
		t.Fatalf("%q", e.String())
	}
	e.home()
	e.right()
	e.right()
	if _, col := e.cursorPos(); col != 2 {
		t.Fatalf("col=%d — columns must count runes, not bytes", col)
	}
}

func TestLinkPrefixDetection(t *testing.T) {
	cases := map[string]struct {
		want string
		ok   bool
	}{
		"see [[Pro":        {"Pro", true},
		"[[":               {"", true},
		"see [[Done]] and": {"", false},
		"no brackets":      {"", false},
		"[[multi\nline":    {"", false},
	}
	for in, want := range cases {
		got, ok := linkPrefix(in)
		if got != want.want || ok != want.ok {
			t.Fatalf("%q: got (%q,%v) want (%q,%v)", in, got, ok, want.want, want.ok)
		}
	}
}
