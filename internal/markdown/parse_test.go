package markdown

import (
	"strings"
	"testing"
)

func parseOne(t *testing.T, src string) *Document {
	t.Helper()
	return Parse([]byte(src))
}

func TestParseNesting(t *testing.T) {
	doc := parseOne(t, `- a
  - b
    - c
  - d
- e
`)
	if len(doc.Blocks) != 2 {
		t.Fatalf("top level: got %d, want 2", len(doc.Blocks))
	}
	a := doc.Blocks[0]
	if a.Text != "a" || len(a.Children) != 2 {
		t.Fatalf("a: text=%q children=%d", a.Text, len(a.Children))
	}
	b := a.Children[0]
	if b.Text != "b" || len(b.Children) != 1 || b.Children[0].Text != "c" {
		t.Fatalf("b subtree wrong: %+v", b)
	}
	if b.Depth != 1 || b.Children[0].Depth != 2 {
		t.Fatalf("depths wrong: b=%d c=%d", b.Depth, b.Children[0].Depth)
	}
	if a.Children[1].Text != "d" {
		t.Fatalf("d misplaced: %q", a.Children[1].Text)
	}
}

func TestParseMultilineBlock(t *testing.T) {
	doc := parseOne(t, `- first line
  second line
  third line
- next
`)
	if got, want := doc.Blocks[0].Text, "first line\nsecond line\nthird line"; got != want {
		t.Fatalf("multiline text:\ngot  %q\nwant %q", got, want)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("expected 2 top-level blocks, got %d", len(doc.Blocks))
	}
}

func TestParseCodeFence(t *testing.T) {
	src := "- run this\n  ```sh\n  - not a bullet\n    indented\n  ```\n- after\n"
	doc := parseOne(t, src)
	if len(doc.Blocks) != 2 {
		t.Fatalf("fence swallowed structure: got %d top-level blocks", len(doc.Blocks))
	}
	want := "run this\n```sh\n- not a bullet\n  indented\n```"
	if got := doc.Blocks[0].Text; got != want {
		t.Fatalf("fence text:\ngot  %q\nwant %q", got, want)
	}
	if len(doc.Blocks[0].Children) != 0 {
		t.Fatalf("fence content became children")
	}
}

func TestParseProperties(t *testing.T) {
	doc := parseOne(t, `- a task
  status:: doing
  due:: 2026-09-20
  trailing text
`)
	b := doc.Blocks[0]
	if b.Text != "a task\ntrailing text" {
		t.Fatalf("text kept properties: %q", b.Text)
	}
	if len(b.Props) != 2 || b.Props[0].Key != "status" || b.Props[0].Value != "doing" {
		t.Fatalf("props wrong: %+v", b.Props)
	}
	if v, ok := b.Prop("due"); !ok || v != "2026-09-20" {
		t.Fatalf("due lookup: %q %v", v, ok)
	}
}

func TestPropertiesNotParsedInFence(t *testing.T) {
	src := "- code\n  ```go\n  key:: value\n  ```\n"
	doc := parseOne(t, src)
	if len(doc.Blocks[0].Props) != 0 {
		t.Fatalf("property parsed inside fence: %+v", doc.Blocks[0].Props)
	}
}

func TestParseAnchor(t *testing.T) {
	doc := parseOne(t, "- an idea worth naming ^k3f9q2\n")
	b := doc.Blocks[0]
	if b.Anchor != "k3f9q2" {
		t.Fatalf("anchor: %q", b.Anchor)
	}
	if b.Text != "an idea worth naming" {
		t.Fatalf("anchor left in text: %q", b.Text)
	}
}

func TestParseFrontmatter(t *testing.T) {
	doc := parseOne(t, `---
title: Project Foo
created: 2026-09-17
---

- content
`)
	if len(doc.Frontmatter) != 2 || doc.Frontmatter[0].Key != "title" {
		t.Fatalf("frontmatter: %+v", doc.Frontmatter)
	}
	if len(doc.Blocks) != 1 || doc.Blocks[0].Text != "content" {
		t.Fatalf("body after frontmatter: %+v", doc.Blocks)
	}
}

func TestUnterminatedFrontmatterIsContent(t *testing.T) {
	doc := parseOne(t, "---\ntitle: nope\n- a\n")
	if len(doc.Frontmatter) != 0 {
		t.Fatalf("accepted unterminated frontmatter: %+v", doc.Frontmatter)
	}
	if len(doc.Blocks) == 0 {
		t.Fatal("content dropped")
	}
}

func TestParseLinksTagsTasks(t *testing.T) {
	doc := parseOne(t, "- see [[Project Foo]] and [[Rust#^a1b2c3]] about #research\n- [ ] open task\n- [x] done task\n")
	links := doc.Blocks[0].Links()
	if len(links) != 2 || links[0].Page != "Project Foo" || links[1].Anchor != "a1b2c3" {
		t.Fatalf("links: %+v", links)
	}
	if tags := doc.Blocks[0].Tags(); len(tags) != 1 || tags[0] != "research" {
		t.Fatalf("tags: %+v", tags)
	}
	if doc.Blocks[1].Task() != TaskOpen || doc.Blocks[2].Task() != TaskDone {
		t.Fatalf("task states: %v %v", doc.Blocks[1].Task(), doc.Blocks[2].Task())
	}
	if doc.Blocks[0].Task() != NotATask {
		t.Fatal("plain block reported as task")
	}
}

func TestLinksIgnoredInsideFence(t *testing.T) {
	src := "- x\n  ```\n  [[Not A Link]]\n  ```\n"
	doc := parseOne(t, src)
	if got := doc.Blocks[0].Links(); len(got) != 0 {
		t.Fatalf("link parsed inside fence: %+v", got)
	}
}

func TestTabIndentation(t *testing.T) {
	doc := parseOne(t, "- a\n\t- b\n\t\t- c\n")
	if len(doc.Blocks) != 1 {
		t.Fatalf("tabs broke nesting: %d top-level", len(doc.Blocks))
	}
	if len(doc.Blocks[0].Children) != 1 || len(doc.Blocks[0].Children[0].Children) != 1 {
		t.Fatal("tab nesting wrong")
	}
}

func TestContentBeforeFirstBulletIsKept(t *testing.T) {
	doc := parseOne(t, "# A heading\n\n- a bullet\n")
	if len(doc.Blocks) != 2 || !strings.Contains(doc.Blocks[0].Text, "A heading") {
		t.Fatalf("stray content dropped: %+v", doc.Blocks)
	}
}

func TestEmptyBullet(t *testing.T) {
	doc := parseOne(t, "-\n- x\n")
	if len(doc.Blocks) != 2 || doc.Blocks[0].Text != "" {
		t.Fatalf("empty bullet: %+v", doc.Blocks)
	}
}

func TestEmptyAndWhitespaceInput(t *testing.T) {
	for _, src := range []string{"", "\n", "   \n\n"} {
		doc := parseOne(t, src)
		if len(doc.Blocks) != 0 {
			t.Fatalf("input %q produced %d blocks", src, len(doc.Blocks))
		}
		if out := Render(doc); len(out) != 0 {
			t.Fatalf("input %q rendered %q", src, out)
		}
	}
}

func TestSourceSpans(t *testing.T) {
	src := "- a\n  cont\n  - b\n- c\n"
	doc := parseOne(t, src)
	a := doc.Blocks[0]
	if got := string([]byte(src)[a.Start:a.SelfEnd]); got != "- a\n  cont\n" {
		t.Fatalf("self span: %q", got)
	}
	if got := string([]byte(src)[a.Start:a.End]); got != "- a\n  cont\n  - b\n" {
		t.Fatalf("subtree span: %q", got)
	}
	if doc.FindByOffset(a.Start) != a {
		t.Fatal("FindByOffset failed")
	}
}

func TestParseLinkTargetTags(t *testing.T) {
	cases := []struct {
		in     string
		page   string
		anchor string
		tags   []string
	}{
		{"Ada", "Ada", "", nil},
		{"Ada #person", "Ada", "", []string{"person"}},
		{"Ada Lovelace #person #colleague", "Ada Lovelace", "", []string{"person", "colleague"}},
		{"engine #project", "engine", "", []string{"project"}},
		// An anchor is never mistaken for a tag.
		{"Rust#^a1b2c3", "Rust", "a1b2c3", nil},
		{"Rust#^a1b2c3 #lang", "Rust", "a1b2c3", []string{"lang"}},
		// A trailing token that is not a usable tag stays part of the name.
		{"Issue #42", "Issue #42", "", nil},
		{"C# stuff", "C# stuff", "", nil},
	}
	for _, c := range cases {
		got := ParseLinkTarget(c.in)
		if got.Page != c.page || got.Anchor != c.anchor {
			t.Fatalf("%q: page=%q anchor=%q", c.in, got.Page, got.Anchor)
		}
		if len(got.Tags) != len(c.tags) {
			t.Fatalf("%q: tags=%v want %v", c.in, got.Tags, c.tags)
		}
		for i := range c.tags {
			if got.Tags[i] != c.tags[i] {
				t.Fatalf("%q: tags=%v want %v", c.in, got.Tags, c.tags)
			}
		}
		if round := ParseLinkTarget(strings.TrimSuffix(strings.TrimPrefix(got.String(), "[["), "]]")); round.Page != got.Page {
			t.Fatalf("%q did not survive a round trip through String: %q", c.in, got.String())
		}
	}
}

func TestBracketTagsAreNotBlockTags(t *testing.T) {
	doc := parseOne(t, "- met [[Ada #person]] about #research\n")
	b := doc.Blocks[0]
	// The bracket tag describes Ada, not this block.
	if tags := b.Tags(); len(tags) != 1 || tags[0] != "research" {
		t.Fatalf("block tags: %v", tags)
	}
	links := b.Links()
	if len(links) != 1 || links[0].Page != "Ada" || len(links[0].Tags) != 1 || links[0].Tags[0] != "person" {
		t.Fatalf("link: %+v", links)
	}
}

func TestTaggedLinkSurvivesRoundTrip(t *testing.T) {
	src := "- met [[Ada Lovelace #person]] about [[Analytical Engine #project]]\n"
	if got := string(Render(Parse([]byte(src)))); got != src {
		t.Fatalf("got %q want %q", got, src)
	}
}

func TestQuoteIsOrdinaryBlockText(t *testing.T) {
	src := "- > Dear all,\n  > the meeting is moved.\n\n- not a quote\n"
	doc := parseOne(t, src)

	q := doc.Blocks[0]
	if !q.Quote() {
		t.Fatal("a block starting with > should read as a quotation")
	}
	if got := q.QuoteBody(); got != "Dear all,\nthe meeting is moved." {
		t.Fatalf("quote body: %q", got)
	}
	if doc.Blocks[1].Quote() {
		t.Fatal("a plain block reported as a quote")
	}

	// Nothing is stored differently, so the file must survive untouched.
	if out := string(Render(doc)); out != src {
		t.Fatalf("round trip changed the file:\ngot  %q\nwant %q", out, src)
	}
}

func TestQuoteKeepsUnmarkedLines(t *testing.T) {
	// Pasted mail rarely marks every line.
	doc := parseOne(t, "- > Dear all,\n  the rest was not marked\n")
	if !doc.Blocks[0].Quote() {
		t.Fatal("should still be a quote")
	}
	if got := doc.Blocks[0].QuoteBody(); got != "Dear all,\nthe rest was not marked" {
		t.Fatalf("body: %q", got)
	}
}
