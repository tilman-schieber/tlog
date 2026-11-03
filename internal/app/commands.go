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

// TakesText reports whether the command wants free text typed after it, which
// the menu shows verbatim because there is nothing to resolve or preview.
func (c Command) TakesText() bool { return c.Arg == "text" }

// DeadlineProp is the property a deadline is written to. Capitalised because
// that is what is already in these notes; reading is case-insensitive.
const DeadlineProp = "Deadline"

var commands = []Command{
	{Name: "todo", Title: "TODO", Hint: "Task, open", aliases: []string{"task", "aufgabe"}},
	{Name: "done", Title: "DONE", Hint: "Task, done", aliases: []string{"erledigt"}},
	{Name: "deadline", Title: "Deadline", Hint: "Due on …", Arg: "date", aliases: []string{"due", "faellig", "fällig", "dl"}},
	{Name: "date", Title: "Date", Hint: "Insert a date, as a link", Arg: "date", aliases: []string{"datum"}},
	{Name: "quote", Title: "Quote", Hint: "Make the block a quotation", aliases: []string{"zitat"}},
	{Name: "code", Title: "Code", Hint: "Insert a code block"},
	{Name: "table", Title: "Table", Hint: "Insert a table as csv", aliases: []string{"tabelle", "csv"}},
	{Name: "page", Title: "Page", Hint: "Link to a page", aliases: []string{"link", "seite"}},
	{Name: "tag", Title: "Tag", Hint: "Insert a tag"},
	{Name: "file", Title: "Attachment", Hint: "Link a file from the notes' assets", Arg: "attachment", aliases: []string{"anhang", "att", "attach"}},
	{Name: "prop", Title: "Property", Hint: "Set a property: /prop status open", Arg: "text", aliases: []string{"property", "eigenschaft"}},
	{Name: "alias", Title: "Alias", Hint: "Another name for this page: /alias Ada", Arg: "text", aliases: []string{"aka", "zweitname"}},
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
			return nil, refuse("%q is not a date I understand", arg)
		}
		due = d
	}

	// A property is "key value": the first word names it, the rest is the
	// value. An empty value removes the property, which is what SetProperty
	// already means and the only way to take one off again.
	var propKey, propVal string
	if cmd.Name == "prop" {
		propKey, propVal, _ = strings.Cut(strings.TrimSpace(arg), " ")
		propVal = strings.TrimSpace(propVal)
		if propKey == "" {
			return nil, fmt.Errorf("which property? try /prop status offen")
		}
	}

	// An alias is a property of the page rather than of a block, so it goes in
	// the frontmatter — the same place tags: goes, read the same way.
	var alias string
	if cmd.Name == "alias" {
		alias = strings.TrimSpace(arg)
		if err := checkAlias(alias); err != nil {
			return nil, err
		}
	}

	insert := ""
	if cmd.TakesAttachment() {
		found, err := s.Attachments(arg, 1)
		if err != nil {
			return nil, err
		}
		if len(found) == 0 {
			return nil, fmt.Errorf("no attachment matches %q — `tlog attach` or a drop puts one there", arg)
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

	// A command that leaves nothing behind takes the space before it too, so
	// "slides /deadline fr" becomes "slides", not "slides ". A space is kept
	// where text follows and needs separating from what came before.
	if insert == "" {
		before = strings.TrimRight(before, " \t")
		if before != "" && after != "" && !strings.HasPrefix(after, " ") && !strings.HasPrefix(after, "\n") {
			before += " "
		}
	}
	newText := before + insert + after
	caret := from + len([]rune(insert))
	switch cmd.Name {
	case "page":
		caret = from + 2 // between the brackets, where completion takes over
	case "code", "table":
		caret = from + len([]rune(insert)) - 4 // on the empty line inside the fence
	}

	res, err := s.mutate(a, func(d *markdown.Document, b *markdown.Block) error {
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
			// A deadline makes it a task. A date on a block that can never be
			// ticked off is a date that never leaves the agenda, and "what is
			// due" only means anything if something can become not-due.
			if b.Task() == markdown.NotATask {
				b.MakeTask()
				caret += 4 // "[ ] " went in front
			}
			setDeadline(b, due, s.Cfg.DeadlineProperty())
		case "quote":
			if !b.Quote() {
				b.Text = "> " + b.Text
				caret += 2
			}
		case "prop":
			if propVal == "" {
				b.DelProp(propKey)
			} else {
				b.SetProp(propKey, propVal)
			}
		case "alias":
			addAlias(d, alias)
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

// checkAlias is what may be another name for a page.
func checkAlias(alias string) error {
	if alias == "" {
		return fmt.Errorf("which name? try /alias Ada")
	}
	if strings.ContainsAny(alias, "[]#\n") {
		return fmt.Errorf("%q cannot be a page name", alias)
	}
	return nil
}

// ApplyCommand runs a slash command on a block from outside an editor, where
// there is no "/name arg" typed into the text to cut out. What a command would
// insert — a date, a file link — goes at the end of the block's text; what it
// changes about the block happens as it would from the menu.
func (s *Service) ApplyCommand(a Addr, name, arg string) (*CommandResult, error) {
	if _, ok := FindCommand(name); !ok {
		return nil, refuse("no such command: /%s", name)
	}
	_, b, err := s.Resolve(a)
	if err != nil {
		return nil, err
	}
	end := len([]rune(b.Text))
	return s.RunCommand(a, name, arg, b.Text, end, end)
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

// IsDeadlineKey accepts every spelling, whatever the setting says to write, so
// that changing the setting never orphans what is already in the notes. An
// adapter that draws a deadline in its own way hides the same set.
func IsDeadlineKey(k string) bool { return isDeadlineKey(k) }

func isDeadlineKey(k string) bool {
	return strings.EqualFold(k, DeadlineProp) || strings.EqualFold(k, "due") ||
		strings.EqualFold(k, "deadline") || strings.EqualFold(k, "fällig")
}

// Deadline reads a block's deadline, whatever case the property was written in.
//
// It reads strictly: a relative value typed by hand — "due:: fri" — would mean
// a different day every time it was read, so it is not a deadline until tlog
// next writes the file and normalizeDates pins it to the day it meant then.
func Deadline(b *markdown.Block) (time.Time, bool) {
	for _, p := range b.Props {
		if isDeadlineKey(p.Key) {
			if d, ok := dates.ParseStored(p.Value); ok {
				return d, true
			}
		}
	}
	return time.Time{}, false
}

// normalizeDates writes every deadline in the one form a file holds: ISO.
// However it was typed — by hand in nvim, "24.9.", "fri", "7. November" — it is
// converted on the way to disk, and a relative one is resolved against the
// moment of writing, which is when it was meant. What is not a date is left as
// it was typed.
func normalizeDates(d *markdown.Document, now time.Time) {
	d.Walk(func(b *markdown.Block) bool {
		for i := range b.Props {
			if !isDeadlineKey(b.Props[i].Key) {
				continue
			}
			if iso, ok := dates.NormalizeStored(b.Props[i].Value, now); ok {
				b.Props[i].Value = iso
			}
		}
		return true
	})
}

// DateChoice is one entry in a date quick pick, with everything an adapter
// needs to draw it.
type DateChoice struct {
	Phrase string `json:"phrase"` // what it is called: "friday", "in 3 days"
	ISO    string `json:"iso"`    // what is written
	Short  string `json:"short"`  // "Fr 07.11."
	Label  string `json:"label"`  // how far away: "in 4 days"
	State  string `json:"state"`  // overdue, today, soon, later
}

// DateChoices is the quick pick for a date being typed: the usual days when
// nothing is typed yet, narrowing as it is. The outliner's @ menu, the slash
// menus for /date and /deadline, and the window all draw this one list.
func DateChoices(input string, limit int) []DateChoice {
	now := time.Now()
	var out []DateChoice
	for _, s := range dates.Suggest(input, now, limit) {
		out = append(out, DateChoice{
			Phrase: s.Phrase,
			ISO:    dates.Format(s.Day),
			Short:  dates.Short(s.Day),
			Label:  dates.Describe(s.Day, now),
			State:  string(dates.Status(s.Day, now)),
		})
	}
	return out
}

// addAlias appends a name to the page's aliases, leaving the ones already there
// alone — a second alias must not take the first one's place.
func addAlias(d *markdown.Document, alias string) {
	var names []string
	for _, fm := range d.Frontmatter {
		if strings.EqualFold(fm.Key, "aliases") || strings.EqualFold(fm.Key, "alias") {
			for _, n := range strings.Split(fm.Value, ",") {
				if n = strings.TrimSpace(n); n != "" {
					names = append(names, n)
				}
			}
		}
	}
	for _, n := range names {
		if strings.EqualFold(n, alias) {
			return // already one of this page's names
		}
	}
	d.SetFrontmatter("aliases", strings.Join(append(names, alias), ", "))
}
