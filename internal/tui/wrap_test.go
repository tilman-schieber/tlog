package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestWrapRangesBreakAtWordsAndLoseNothing(t *testing.T) {
	for _, c := range []struct {
		text string
		w    int
		want []string
	}{
		{"the quick brown fox", 10, []string{"the quick ", "brown fox"}},
		{"the quick", 9, []string{"the quick"}},                 // fits exactly
		{"aaaa bbbb", 4, []string{"aaaa", " ", "bbbb"}},         // no room even for the space
		{"one two three", 7, []string{"one ", "two ", "three"}}, // no line starts with a space
		{"abcdefghij", 4, []string{"abcd", "efgh", "ij"}},       // a word wider than a line
		{"go to https://example.com/a/b", 12, []string{"go to ", "https://exam", "ple.com/a/b"}},
	} {
		var got []string
		r := []rune(c.text)
		for _, sg := range wrapRanges(c.text, c.w) {
			got = append(got, string(r[sg[0]:sg[1]]))
		}
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("%q at %d: got %q, want %q", c.text, c.w, got, c.want)
		}
		if strings.Join(got, "") != c.text {
			t.Errorf("%q: something was lost: %q", c.text, got)
		}
	}
}

// screen draws the outliner and returns its lines, colours taken out.
func screen(m *Model) []string { return strings.Split(stripANSI(m.View()), "\n") }

// body is the screen without the header and the footer: the lines that hold
// the page. The footer is a hint, and a hint may be cut short; text may not.
func body(lines []string) []string { return lines[1 : len(lines)-1] }

const long = "this is a long line of text that goes well past the edge of a narrow window and keeps going"

func TestALongLineIsWrappedNotCutOff(t *testing.T) {
	m := treeModel(t, "- "+long+"\n\n- below\n")
	m.width, m.height = 30, 20
	send(t, m, k(tea.KeyDown)) // off the block, so it is drawn rather than edited
	lines := screen(m)
	var text []string
	for _, l := range body(lines) {
		if w := lipgloss.Width(l); w > 30 {
			t.Fatalf("a line is %d wide in a 30-wide window: %q", w, l)
		}
		if strings.HasPrefix(l, "• ") || strings.HasPrefix(l, "  ") {
			text = append(text, strings.TrimPrefix(strings.TrimPrefix(l, "• "), "  "))
		}
	}
	if got := strings.Join(text, ""); !strings.Contains(got, long) {
		t.Fatalf("the text is not all on screen:\n%s", strings.Join(lines, "\n"))
	}
}

func TestTheCaretStaysOnScreenPastTheEdge(t *testing.T) {
	m := newModel(t)
	m.width, m.height = 30, 20
	typeText(t, m, long)
	frame := m.View()
	if !strings.Contains(frame, styleCursor.Render(" ")) {
		t.Fatalf("the caret at the end of a long line is not drawn:\n%s", stripANSI(frame))
	}
	for _, l := range body(strings.Split(stripANSI(frame), "\n")) {
		if w := lipgloss.Width(l); w > 30 {
			t.Fatalf("a line is %d wide while typing: %q", w, l)
		}
	}
}

func TestUpAndDownMoveByScreenLineInALongLine(t *testing.T) {
	m := treeModel(t, "- first\n\n- "+long+"\n\n- last\n")
	m.width, m.height = 30, 20
	send(t, m, k(tea.KeyDown)) // the long block, caret at its end
	m.View()
	end := m.ed.cur
	send(t, m, k(tea.KeyUp))
	if m.cur != 1 || m.ed.cur >= end || end-m.ed.cur > 30 {
		t.Fatalf("up should go one screen line up within the block: row %d, caret %d → %d", m.cur, end, m.ed.cur)
	}
	m.View()
	send(t, m, k(tea.KeyDown))
	if m.cur != 1 || m.ed.cur != end {
		t.Fatalf("down should come back: row %d caret %d, want %d", m.cur, m.ed.cur, end)
	}
	// At the top screen line of the block, up leaves it.
	for i := 0; i < 10 && m.cur == 1; i++ {
		m.View()
		send(t, m, k(tea.KeyUp))
	}
	if m.cur != 0 {
		t.Fatalf("up from the first screen line should reach the block above, on row %d", m.cur)
	}
}

func TestAClickOnAWrappedLineLandsWhereItWasClicked(t *testing.T) {
	m := treeModel(t, "- "+long+"\n\n- below\n")
	m.width, m.height = 30, 20
	send(t, m, k(tea.KeyEsc))
	lines := screen(m)
	// The block's second screen line is screen line 2: the header is line 0.
	second := strings.TrimPrefix(lines[2], "  ")
	m.Update(clickAt(0, 1)) // highlight the block (moving: a click highlights)
	m.View()
	m.Update(clickAt(2+4, 2)) // the fifth character of its second screen line
	if m.ed == nil {
		t.Fatal("the click did not start writing")
	}
	if got := string([]rune(m.ed.String())[m.ed.cur:])[:1]; got != second[4:5] {
		t.Fatalf("the caret is before %q, the click was on %q (%q)", got, second[4:5], second)
	}
}

func TestTheBlockBeingEditedShowsExactlyWhatIsTyped(t *testing.T) {
	m := treeModel(t, "- met [[Max Muster #Person]] today\n")
	frame := stripANSI(m.View())
	if !strings.Contains(frame, "met [[Max Muster #Person]] today") {
		t.Fatalf("the block under the caret hides part of its text:\n%s", frame)
	}
}
