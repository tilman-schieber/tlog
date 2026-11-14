package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func drawMD(text string, width int) []string {
	var out []string
	for _, l := range markdownLines(text, width) {
		out = append(out, strings.TrimRight(stripANSI(l.s), " "))
	}
	return out
}

func TestMarkdownIsDrawnNotShownAsSource(t *testing.T) {
	got := strings.Join(drawMD(strings.Join([]string{
		"# Title",
		"",
		"Some **bold**, *italic*, `code` and a [link](https://x.org/a).",
		"snake_case_name stays.",
		"",
		"## Section",
		"",
		"- one",
		"- [ ] open",
		"- [x] done",
		"  - nested",
		"",
		"1. first",
		"2. second",
		"",
		"> quoted",
		"",
		"```go",
		"x := 1",
		"```",
		"",
		"| a | b |",
		"|---|--:|",
		"| 1 | 22 |",
		"",
		"---",
	}, "\n"), 60), "\n")
	for _, want := range []string{
		"   Title",
		"  Some bold, italic, code and a link https://x.org/a.",
		"  snake_case_name stays.",
		"  Section",
		"  • one",
		"  ☐ open",
		"  ☑ done",
		"    • nested",
		"  1. first",
		"  │ quoted",
		"  ▏ x := 1",
		"  a │  b",
		"  1 │ 22",
		"  " + strings.Repeat("─", 58),
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, gone := range []string{"**", "](", "```", "|---"} {
		if strings.Contains(got, gone) {
			t.Errorf("%q is still drawn:\n%s", gone, got)
		}
	}
}

func TestMarkdownParagraphsReflowAndListsHang(t *testing.T) {
	got := drawMD("one\ntwo three\n\n- "+strings.Repeat("word ", 12)+"\n", 30)
	if got[0] != "  one two three" {
		t.Fatalf("soft breaks not joined: %q", got[0])
	}
	if !strings.HasPrefix(got[2], "  • word") || !strings.HasPrefix(got[3], "    word") {
		t.Fatalf("no hanging indent: %q", got[2:])
	}
}

func TestMarkdownFitsAnyWidth(t *testing.T) {
	doc := "# " + strings.Repeat("long heading ", 8) + "\n\n" +
		strings.Repeat("x", 200) + "\n\n" +
		"> - " + strings.Repeat("quoted list ", 10) + "\n\n" +
		"| " + strings.Repeat("wide cell ", 10) + " | b |\n|---|---|\n| c | d |\n\n" +
		"```\n" + strings.Repeat("code ", 30) + "\n```\n"
	for _, w := range []int{20, 40, 79} {
		for _, l := range markdownLines(doc, w) {
			if n := ansi.StringWidth(l.s); n > w {
				t.Fatalf("width %d: a line of %d: %q", w, n, stripANSI(l.s))
			}
		}
	}
}

func TestMarkdownKeepsWhatItDoesNotUnderstand(t *testing.T) {
	got := strings.Join(drawMD("a * b and **unclosed and [x] (y) and <b>html</b>\n", 80), "\n")
	if !strings.Contains(got, "a * b and **unclosed and [x] (y) and <b>html</b>") {
		t.Fatalf("got %q", got)
	}
}

func TestTheOutlineStylesInlineMarkdownLikeTheFileViewer(t *testing.T) {
	m := treeModel(t, "- some **bold** and `#notatag` with #tag and [[a_b_c]]\n- # Title\n- below\n")
	send(t, m, k(tea.KeyDown), k(tea.KeyDown))
	frame := stripANSI(m.View())
	for _, want := range []string{"• some bold and #notatag with #tag and [[a_b_c]]", "•  Title "} {
		if !strings.Contains(frame, want) {
			t.Fatalf("%q not drawn:\n%s", want, frame)
		}
	}
	if got := colourInline("`#x` **[[P]]**", &refCursor{}, false); stripANSI(got) != "#x [[P]]" {
		t.Fatalf("got %q", stripANSI(got))
	}
}
