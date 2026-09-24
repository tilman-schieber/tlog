package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tilman-schieber/tlog/internal/dates"
)

// run types a command into a block and applies it, the way an adapter does:
// the text still contains the "/cmd arg" run, and the core cuts it out.
func run(t *testing.T, s *Service, rel string, index int, text, name, arg string, from, to int) *CommandResult {
	t.Helper()
	a := addrOf(t, s, rel, index)
	res, err := s.RunCommand(a, name, arg, text, from, to)
	if err != nil {
		t.Fatalf("/%s %s: %v", name, arg, err)
	}
	return res
}

func TestSlashCommandsLeaveNoSlashBehind(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToday("Bericht schreiben /todo")

	run(t, s, rel, 0, "Bericht schreiben /todo", "todo", "", 18, 23)

	got := body(t, s, rel)
	if got != "- [ ] Bericht schreiben \n" && got != "- [ ] Bericht schreiben\n" {
		t.Fatalf("got %q", got)
	}
	if strings.Contains(got, "/todo") {
		t.Fatal("the command survived into the file")
	}
}

func TestTodoAndDone(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToday("x /todo")
	run(t, s, rel, 0, "x /todo", "todo", "", 2, 7)
	if got := body(t, s, rel); !strings.HasPrefix(got, "- [ ] x") {
		t.Fatalf("todo: %q", got)
	}

	// /done on an open task closes it rather than adding a second marker.
	run(t, s, rel, 0, "[ ] x ", "done", "", 6, 6)
	if got := body(t, s, rel); !strings.HasPrefix(got, "- [x] x") {
		t.Fatalf("done: %q", got)
	}
}

func TestDeadlineWritesAPropertyAndNotText(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToday("Bot testen /deadline fr")

	run(t, s, rel, 0, "Bot testen /deadline fr", "deadline", "fr", 11, 23)

	got := body(t, s, rel)
	if !strings.Contains(got, "Deadline:: ") {
		t.Fatalf("no property written: %q", got)
	}
	if strings.Contains(got, "/deadline") || strings.Contains(got, " fr") {
		t.Fatalf("the shorthand survived: %q", got)
	}
	// Whatever was typed, what is stored is ISO.
	d, _ := s.Load(rel)
	due, ok := Deadline(d.Doc.Blocks[0])
	if !ok {
		t.Fatal("the deadline does not read back")
	}
	if due.Weekday() != time.Friday {
		t.Fatalf("got %s, a %s", dates.Format(due), due.Weekday())
	}
}

func TestDeadlineReplacesRatherThanDuplicating(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToday("x")
	run(t, s, rel, 0, "x", "deadline", "fr", 1, 1)
	run(t, s, rel, 0, "x", "deadline", "mo", 1, 1)

	if n := strings.Count(body(t, s, rel), "Deadline::"); n != 1 {
		t.Fatalf("expected one deadline, found %d:\n%s", n, body(t, s, rel))
	}
}

func TestDeadlineIsRefusedRatherThanGuessed(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToday("x")
	a := addrOf(t, s, rel, 0)

	if _, err := s.RunCommand(a, "deadline", "banana", "x", 1, 1); err == nil {
		t.Fatal("an unparseable date should be refused")
	}
	// And the block is untouched, not half-changed.
	if got := body(t, s, rel); got != "- x\n" {
		t.Fatalf("the block was modified anyway: %q", got)
	}
}

func TestDateInsertsALinkInline(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToday("Protokoll vom /date gestern")
	run(t, s, rel, 0, "Protokoll vom /date gestern", "date", "gestern", 14, 27)

	got := body(t, s, rel)
	if !strings.Contains(got, "[[") || strings.Contains(got, "Deadline") {
		t.Fatalf("a date should be an inline link, not a deadline: %q", got)
	}
}

func TestQuoteCodeAndTable(t *testing.T) {
	s := newSvc(t)

	rel, _ := s.AddToday("gesagt /quote")
	run(t, s, rel, 0, "gesagt /quote", "quote", "", 7, 13)
	if got := body(t, s, rel); !strings.Contains(got, "- > gesagt") {
		t.Fatalf("quote: %q", got)
	}

	rel2, _ := s.AddToPage("P", "x /code")
	run(t, s, rel2, 0, "x /code", "code", "", 2, 7)
	if got := body(t, s, rel2); !strings.Contains(got, "```") {
		t.Fatalf("code: %q", got)
	}

	rel3, _ := s.AddToPage("Q", "x /table")
	run(t, s, rel3, 0, "x /table", "table", "", 2, 8)
	if got := body(t, s, rel3); !strings.Contains(got, "```csv") {
		t.Fatalf("table: %q", got)
	}
}

func TestCaretLandsSomewhereUseful(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToPage("P", "x /code")
	res := run(t, s, rel, 0, "x /code", "code", "", 2, 7)

	d, _ := s.Load(rel)
	text := []rune(d.Doc.Blocks[0].Text)
	if res.Caret < 0 || res.Caret > len(text) {
		t.Fatalf("caret %d is outside the text of %d runes", res.Caret, len(text))
	}
	// Inside the fence, on the empty line, not after the closing backticks.
	if strings.Contains(string(text[res.Caret:]), "```") == false {
		t.Fatalf("caret should sit inside the fence: %q", string(text))
	}
}

func TestMenuMatching(t *testing.T) {
	if len(MatchCommands("")) != len(Commands()) {
		t.Fatal("a bare slash should offer everything")
	}
	if got := MatchCommands("dl"); len(got) == 0 || got[0].Name != "deadline" {
		t.Fatalf("an alias should match: %+v", got)
	}
	if got := MatchCommands("tod"); len(got) == 0 || got[0].Name != "todo" {
		t.Fatalf("a prefix should match: %+v", got)
	}
	if got := MatchCommands("todo"); got[0].Name != "todo" {
		t.Fatal("an exact name should come first")
	}
	if got := MatchCommands("zzz"); len(got) != 0 {
		t.Fatalf("nonsense should match nothing: %+v", got)
	}
	if _, ok := FindCommand("fällig"); !ok {
		t.Fatal("German aliases should resolve")
	}
	if _, ok := FindCommand("nope"); ok {
		t.Fatal("an unknown command should not resolve")
	}
}

func TestUnknownCommandIsRefused(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToday("x")
	if _, err := s.RunCommand(addrOf(t, s, rel, 0), "frobnicate", "", "x", 1, 1); err == nil {
		t.Fatal("expected a refusal")
	}
}

func TestOutOfRangeSpanIsRefused(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToday("x")
	if _, err := s.RunCommand(addrOf(t, s, rel, 0), "todo", "", "x", 5, 99); err == nil {
		t.Fatal("a span outside the text should be refused, not clamped")
	}
}

func TestSlashFileInsertsTheLink(t *testing.T) {
	shelf := t.TempDir()
	t.Setenv("ATT_DIR", shelf)
	src := filepath.Join(t.TempDir(), "Protokoll.pdf")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := newSvc(t)
	if _, err := s.Attach(src); err != nil {
		t.Fatal(err)
	}
	rel, _ := s.AddToday("siehe /file protokoll")
	run(t, s, rel, 0, "siehe /file protokoll", "file", "protokoll", 6, 21)

	got := body(t, s, rel)
	if !strings.Contains(got, "[Protokoll.pdf](file://") {
		t.Fatalf("no link: %q", got)
	}
	if strings.Contains(got, "/file") {
		t.Fatalf("the command survived: %q", got)
	}
}

func TestSlashFileWithNothingOnTheShelfSaysWhereToPutOne(t *testing.T) {
	t.Setenv("ATT_DIR", t.TempDir())
	s := newSvc(t)
	rel, _ := s.AddToday("x")
	_, err := s.RunCommand(addrOf(t, s, rel, 0), "file", "nope", "x", 1, 1)
	if err == nil || !strings.Contains(err.Error(), "att drop") {
		t.Fatalf("expected a message naming how to put one there, got %v", err)
	}
	if got := body(t, s, rel); got != "- x\n" {
		t.Fatalf("the block was modified anyway: %q", got)
	}
}

func TestPropCommandSetsAndRemovesAProperty(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToday("the meeting")

	text := "the meeting /prop status offen"
	res, err := s.RunCommand(addrOf(t, s, rel, 0), "prop", "status offen", text, 12, 30)
	if err != nil {
		t.Fatal(err)
	}
	if got := body(t, s, rel); got != "- the meeting \n  status:: offen\n" {
		t.Fatalf("got %q", got)
	}
	// The command itself is gone from the text, like every other one.
	d, _ := s.Load(rel)
	if b := d.Doc.FindByOffset(res.Offset); b == nil || b.Text != "the meeting " {
		t.Fatalf("the slash stayed behind: %+v", b)
	}

	// No value takes it off again, which is the only way to.
	if _, err := s.RunCommand(addrOf(t, s, rel, 0), "prop", "status", "the meeting ", 12, 12); err != nil {
		t.Fatal(err)
	}
	if got := body(t, s, rel); got != "- the meeting \n" {
		t.Fatalf("the property was not removed: %q", got)
	}
}

func TestPropWithoutAKeyIsRefused(t *testing.T) {
	s := newSvc(t)
	rel, _ := s.AddToday("note")
	if _, err := s.RunCommand(addrOf(t, s, rel, 0), "prop", "", "note", 4, 4); err == nil {
		t.Fatal("a property with no name was accepted")
	}
	if got := body(t, s, rel); got != "- note\n" {
		t.Fatalf("a refused command wrote anyway: %q", got)
	}
}
