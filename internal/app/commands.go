package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/tilman-schieber/tlog/internal/dates"
	"github.com/tilman-schieber/tlog/internal/markdown"
)

// Slash commands, named after Logseq's where they mean the same thing, so that
// muscle memory carries over and there is one place to add the next one.
//
// A command is typed into a block and then disappears: what stays in the file
// is its effect. `/todo` leaves a checkbox, `/deadline fr` leaves a property,
// and neither leaves a slash behind. The list lives here rather than in either
// adapter so the outliner and the app cannot drift apart.

// Command is one entry in the slash menu.
type Command struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Hint    string `json:"hint"`
	Arg     string `json:"arg,omitempty"` // "date" when it takes one
	aliases []string
}

// TakesDate reports whether the command wants a date typed after it.
func (c Command) TakesDate() bool { return c.Arg == "date" }

// TakesAttachment reports whether the command wants a file chosen after it.
func (c Command) TakesAttachment() bool { return c.Arg == "attachment" }

// DeadlineProp is the property a deadline is written to. Capitalised because
// that is what is already in these notes; reading is case-insensitive.
const DeadlineProp = "Deadline"

var commands = []Command{
	{Name: "todo", Title: "TODO", Hint: "Aufgabe, offen", aliases: []string{"task", "aufgabe"}},
	{Name: "done", Title: "DONE", Hint: "Aufgabe, erledigt", aliases: []string{"erledigt"}},
	{Name: "deadline", Title: "Deadline", Hint: "Fällig am …", Arg: "date", aliases: []string{"due", "faellig", "fällig", "dl"}},
	{Name: "date", Title: "Datum", Hint: "Datum einfügen, als Link", Arg: "date", aliases: []string{"datum"}},
	{Name: "quote", Title: "Zitat", Hint: "Block als Zitat", aliases: []string{"zitat"}},
	{Name: "code", Title: "Code", Hint: "Codeblock einfügen"},
	{Name: "table", Title: "Tabelle", Hint: "Tabelle als csv einfügen", aliases: []string{"tabelle", "csv"}},
	{Name: "page", Title: "Seite", Hint: "Link auf eine Seite", aliases: []string{"link", "seite"}},
	{Name: "tag", Title: "Tag", Hint: "Tag einfügen"},
	{Name: "file", Title: "Anhang", Hint: "Datei aus ~/.att einfügen", Arg: "attachment", aliases: []string{"anhang", "att", "attach"}},
}

// Commands returns the whole menu, in the order it is shown.
func Commands() []Command { return append([]Command(nil), commands...) }

// MatchCommands filters the menu by what has been typed after the slash. An
// empty prefix returns everything, so `/` alone shows what there is.
func MatchCommands(prefix string) []Command {
	p := strings.ToLower(strings.TrimSpace(prefix))
	if p == "" {
		return Commands()
	}
	var exact, prefixed, loose []Command
	for _, c := range commands {
		switch {
		case c.Name == p:
			exact = append(exact, c)
		case strings.HasPrefix(c.Name, p):
			prefixed = append(prefixed, c)
		case matchesAlias(c, p):
			prefixed = append(prefixed, c)
		case strings.Contains(c.Name, p):
			loose = append(loose, c)
		}
	}
	return append(append(exact, prefixed...), loose...)
}

func matchesAlias(c Command, p string) bool {
	for _, a := range c.aliases {
		if strings.HasPrefix(a, p) {
			return true
		}
	}
	return false
}

// FindCommand looks a command up by name or alias.
func FindCommand(name string) (Command, bool) {
	n := strings.ToLower(strings.TrimSpace(name))
	for _, c := range commands {
		if c.Name == n {
			return c, true
		}
		for _, a := range c.aliases {
			if a == n {
				return c, true
			}
		}
	}
	return Command{}, false
}

// PreviewDate resolves what a typed date argument means, for showing in the
// menu before anything is committed. The second result is false when the text
// is not a date yet, which is the normal state while it is being typed.
func PreviewDate(arg string) (time.Time, bool) {
	return dates.Parse(arg, time.Now())
}

// CommandResult is what an adapter needs after running a command: where the
// block now is, and where to put the caret inside it.
type CommandResult struct {
	*Result
	Caret int `json:"caret"` // rune offset into the block's new text
}

// RunCommand applies a slash command to a block and removes the command itself
// from the text, in one write.
//
// text is the block's current text and from/to are the rune offsets of the
// "/command argument" run within it. The core does the cutting so that both
// adapters cut identically.
func (s *Service) RunCommand(a Addr, name, arg, text string, from, to int) (*CommandResult, error) {
	cmd, ok := FindCommand(name)
	if !ok {
		return nil, fmt.Errorf("no such command: /%s", name)
	}

	runes := []rune(text)
	if from < 0 || to > len(runes) || from > to {
		return nil, fmt.Errorf("the command is not where it was said to be")
	}
	before, after := string(runes[:from]), string(runes[to:])

	// A date command is refused rather than guessed at: silently setting the
	// wrong deadline is worse than saying the word was not understood.
	var due time.Time
	if cmd.TakesDate() {
		d, ok := dates.Parse(arg, time.Now())
		if !ok {
			return nil, fmt.Errorf("%q is not a date I understand", arg)
		}
		due = d
	}

	insert := ""
	if cmd.TakesAttachment() {
		found, err := s.Attachments(arg, 1)
		if err != nil {
			return nil, err
		}
		if len(found) == 0 {
			return nil, fmt.Errorf("no attachment matches %q — `att drop` puts one there", arg)
		}
		insert = found[0].Link
	}
	switch cmd.Name {
	case "date":
		insert = "[[" + dates.Format(due) + "]]"
	case "code":
		insert = "\n```\n\n```"
	case "table":
		insert = "\n```csv\n\n```"
	case "page":
		insert = "[[]]"
	case "tag":
		insert = "#"
	}

	newText := before + insert + after
	caret := from + len([]rune(insert))
	switch cmd.Name {
	case "page":
		caret = from + 2 // between the brackets, where completion takes over
	case "code", "table":
		caret = from + len([]rune(insert)) - 4 // on the empty line inside the fence
	}

	res, err := s.mutate(a, func(_ *markdown.Document, b *markdown.Block) error {
		b.Text = newText
		switch cmd.Name {
		case "todo":
			if b.Task() == markdown.NotATask {
				b.MakeTask()
				caret += 4 // "[ ] " went in front
			} else if b.Task() == markdown.TaskDone {
				b.ToggleTask()
			}
		case "done":
			if b.Task() == markdown.NotATask {
				b.MakeTask()
				caret += 4
			}
			if b.Task() == markdown.TaskOpen {
				b.ToggleTask()
			}
		case "deadline":
			setDeadline(b, due, s.Cfg.DeadlineProperty())
		case "quote":
			if !b.Quote() {
				b.Text = "> " + b.Text
				caret += 2
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if caret < 0 {
		caret = 0
	}
	return &CommandResult{Result: res, Caret: caret}, nil
}

// setDeadline writes the date, replacing whatever spelling of the property was
// already there so a block never ends up with two.
func setDeadline(b *markdown.Block, due time.Time, prop string) {
	for _, p := range b.Props {
		if isDeadlineKey(p.Key) {
			b.DelProp(p.Key)
		}
	}
	b.SetProp(prop, dates.Format(due))
}

// isDeadlineKey accepts every spelling, whatever the setting says to write, so
// that changing the setting never orphans what is already in the notes.
func isDeadlineKey(k string) bool {
	return strings.EqualFold(k, DeadlineProp) || strings.EqualFold(k, "due") ||
		strings.EqualFold(k, "deadline") || strings.EqualFold(k, "fällig")
}

// Deadline reads a block's deadline, whatever case the property was written in.
func Deadline(b *markdown.Block) (time.Time, bool) {
	for _, p := range b.Props {
		if isDeadlineKey(p.Key) {
			if d, ok := dates.Parse(p.Value, time.Now()); ok {
				return d, true
			}
		}
	}
	return time.Time{}, false
}
