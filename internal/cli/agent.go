package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tilman-schieber/tlog/internal/app"
	"github.com/tilman-schieber/tlog/internal/store"
)

// The surface a script or an agent works through. It is the same core the
// outliner uses — these commands hold no logic, and every one of
// them is a call into app.Service.
//
// Two things make it safe to hand to something that is not a person. Every
// block carries an address that includes the hash of the file it was read
// from, so a write from a stale read is refused rather than landing in the
// wrong place. And the exit code says which kind of no it was, so a caller
// knows whether re-reading would help.

// Exit codes. 0 and 1 are the usual; the other three exist so that a caller
// can tell "that does not exist" from "you are behind" without matching on the
// text of a message.
const (
	exitOK      = 0
	exitError   = 1
	exitUsage   = 2
	exitRefused = 3 // the move does not exist — retrying will not help
	exitStale   = 4 // the file moved since the address was read — re-read
)

// fail maps an error to the exit code that describes it.
func fail(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "tlog: %v\n", err)
	var conflict *store.ErrConflict
	switch {
	case app.Refused(err):
		return exitRefused
	case app.Stale(err), errors.As(err, &conflict):
		return exitStale
	case errors.Is(err, store.ErrOutside), errors.Is(err, store.ErrBadPageName):
		return exitUsage
	}
	return exitError
}

// parseFlags parses a flag set without caring where the flags were written.
//
// Go's flag package stops at the first non-flag argument, so `tlog search
// lecture -json` would search for "lecture -json" and print no JSON. People
// and agents both write the flag last about as often as first, and a surface
// meant to be driven by a script should not have a trap in it that silently
// changes the query. Recognised flags are lifted to the front; everything
// after a literal -- is left exactly as it is.
func parseFlags(fs *flag.FlagSet, args []string) error {
	var flags, rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			rest = append(rest, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			rest = append(rest, a)
			continue
		}
		name := strings.TrimLeft(a, "-")
		value := ""
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			name, value = name[:eq], name[eq:]
		}
		f := fs.Lookup(name)
		if f == nil {
			rest = append(rest, a) // not ours: an ordinary word that begins with -
			continue
		}
		flags = append(flags, a)
		// A flag that takes a value swallows the next argument, unless the
		// value was already attached with =.
		if value == "" && !isBoolFlag(f) && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	// The -- goes back in so that flag.Parse stops there too: everything left
	// is a positional, including a word that begins with a dash.
	return fs.Parse(append(append(flags, "--"), rest...))
}

func isBoolFlag(f *flag.Flag) bool {
	b, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && b.IsBoolFlag()
}

// flags makes a flag set that reports its mistakes on stderr.
func flags(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

// emit writes a value as JSON. Indented, because the first reader of any of
// this is a person finding out what the shape is.
func emit(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// stdin is where a "-" argument reads from; tests point it elsewhere.
var stdin io.Reader = os.Stdin

// textFrom is the text an operation was given: the arguments joined, or
// standard input when the one argument is "-", for a block of several lines.
func textFrom(args []string) (string, error) {
	if len(args) == 1 && args[0] == "-" {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return "", err
		}
		return strings.TrimRight(string(data), "\n"), nil
	}
	return strings.Join(args, " "), nil
}

// written is what a write prints with -json: the address to carry on from,
// and a warning when the note was saved but the commit or push after it
// failed. The write succeeded, so the exit code is 0; the warning is there so
// that it is not only on stderr.
type written struct {
	Addr    app.Addr `json:"addr"`
	Rel     string   `json:"rel"`
	Offset  int      `json:"offset"`
	Hash    string   `json:"hash"`
	Warning string   `json:"warning,omitempty"`
}

// settle commits what a one-shot command wrote and says how it went. A
// one-shot command never leaves an uncommitted change behind; a commit that
// fails is reported, never swallowed, but does not undo the save.
func settle(stderr io.Writer, svc *app.Service) string {
	if err := svc.Commit(); err != nil {
		fmt.Fprintf(stderr, "tlog: %v\n", err)
		return err.Error()
	}
	return ""
}

// report prints the outcome of a write: its address, or the whole result as
// JSON when asked.
func report(stdout, stderr io.Writer, svc *app.Service, res *app.Result, asJSON bool) int {
	warning := settle(stderr, svc)
	if asJSON {
		if err := emit(stdout, written{Addr: res.Addr, Rel: res.Rel, Offset: res.Offset, Hash: res.Hash, Warning: warning}); err != nil {
			return fail(stderr, err)
		}
		return exitOK
	}
	fmt.Fprintln(stdout, res.Addr.String())
	return exitOK
}

// --- reading -----------------------------------------------------------------

func cmdSearch(args []string, stdout, stderr io.Writer, svc *app.Service) int {
	fs := flags("search", stderr)
	asJSON := fs.Bool("json", false, "print the hits as JSON, with an address for each")
	limit := fs.Int("n", 50, "how many hits at most")
	if err := parseFlags(fs, args); err != nil {
		return exitUsage
	}
	query := strings.Join(fs.Args(), " ")
	if strings.TrimSpace(query) == "" {
		fmt.Fprintln(stderr, "tlog search: search for what?")
		return exitUsage
	}

	hits, err := svc.Search(query, *limit)
	if err != nil {
		return fail(stderr, err)
	}
	if *asJSON {
		if err := emit(stdout, hits); err != nil {
			return fail(stderr, err)
		}
		return exitOK
	}
	for _, h := range hits {
		fmt.Fprintf(stdout, "%-28s %s\n", truncate(h.Page, 28), truncate(h.FirstLine, 90))
	}
	return exitOK
}

// cmdIndex lists everything. With -json each entry is a place — kind, name and
// path — so that a caller can open one without assembling a path itself.
func cmdIndex(args []string, stdout, stderr io.Writer, svc *app.Service) int {
	fs := flags("index", stderr)
	asJSON := fs.Bool("json", false, "print as JSON: kind, name and path of every place")
	if err := parseFlags(fs, args); err != nil {
		return exitUsage
	}
	if *asJSON {
		places, err := svc.Places("", 0)
		if err != nil {
			return fail(stderr, err)
		}
		if err := emit(stdout, places); err != nil {
			return fail(stderr, err)
		}
		return exitOK
	}
	idx, err := svc.Index()
	if err != nil {
		return fail(stderr, err)
	}
	for _, j := range idx.Journals {
		fmt.Fprintln(stdout, "journal  "+j)
	}
	for _, p := range idx.Pages {
		fmt.Fprintln(stdout, "page     "+p)
	}
	for _, t := range idx.Tags {
		fmt.Fprintln(stdout, "tag      #"+t)
	}
	return exitOK
}

// cmdPlaces ranks everywhere you could go against a query — the palette's
// list, for a script.
func cmdPlaces(args []string, stdout, stderr io.Writer, svc *app.Service) int {
	fs := flags("places", stderr)
	asJSON := fs.Bool("json", false, "print as JSON")
	limit := fs.Int("n", 0, "how many at most")
	if err := parseFlags(fs, args); err != nil {
		return exitUsage
	}
	places, err := svc.Places(strings.Join(fs.Args(), " "), *limit)
	if err != nil {
		return fail(stderr, err)
	}
	if *asJSON {
		if err := emit(stdout, places); err != nil {
			return fail(stderr, err)
		}
		return exitOK
	}
	for _, p := range places {
		fmt.Fprintf(stdout, "%-8s %-30s %s\n", p.Kind, truncate(p.Name, 30), p.Rel)
	}
	return exitOK
}

// cmdView prints a page. Unlike `tlog open` it creates nothing: reading is not
// a reason to write a file, and an agent surveying the notes must not leave
// empty pages behind everywhere it looked.
func cmdView(args []string, stdout, stderr io.Writer, svc *app.Service) int {
	fs := flags("view", stderr)
	asJSON := fs.Bool("json", false, "print the whole page as JSON, addresses included")
	withAddr := fs.Bool("addr", false, "show each block's address")
	day := fs.String("day", "", "a journal: today, yesterday, -1, fri, 2026-11-03")
	if err := parseFlags(fs, args); err != nil {
		return exitUsage
	}
	name := strings.Join(fs.Args(), " ")
	if *day != "" && strings.TrimSpace(name) != "" {
		fmt.Fprintln(stderr, "tlog view: a page or a day, not both")
		return exitUsage
	}

	var rel string
	var err error
	switch {
	case *day != "":
		rel, err = svc.DayRel(*day)
	case strings.TrimSpace(name) == "":
		rel = svc.TodayRel()
	default:
		rel, err = svc.FindPage(name)
	}
	if err != nil {
		return fail(stderr, err)
	}
	v, err := svc.View(rel)
	if err != nil {
		return fail(stderr, err)
	}
	if *asJSON {
		if err := emit(stdout, v); err != nil {
			return fail(stderr, err)
		}
		return exitOK
	}

	head := v.Title
	if v.IsJournal {
		head += "  journal"
	}
	for _, t := range v.Tags {
		head += "  #" + t
	}
	if len(v.Aliases) > 0 {
		head += "  (also " + strings.Join(v.Aliases, ", ") + ")"
	}
	fmt.Fprintln(stdout, head)
	for _, b := range v.Blocks {
		indent := strings.Repeat("  ", b.Depth)
		line := indent + "- " + oneLine(b.Text)
		if *withAddr {
			line = b.Addr.String() + "  " + line
		}
		fmt.Fprintln(stdout, line)
		// What the file holds beside the text: its properties, and the
		// deadline with how it stands. -json has always had them; a reader of
		// the plain form should not have to guess.
		for _, p := range b.Props {
			if app.IsDeadlineKey(p.Key) {
				continue
			}
			fmt.Fprintf(stdout, "%s  %s:: %s\n", indent, p.Key, p.Value)
		}
		if b.Due != "" {
			state := b.DueLabel
			if b.Task == "done" {
				state = "done"
			}
			fmt.Fprintf(stdout, "%s  %s:: %s  (%s)\n", indent, app.DeadlineProp, b.Due, state)
		}
	}
	if len(v.Refs) > 0 {
		heads := 0
		for _, r := range v.Refs {
			if r.Head {
				heads++
			}
		}
		fmt.Fprintf(stdout, "\n%d linked references\n", heads)
		for _, r := range v.Refs {
			line := strings.Repeat("  ", r.Depth) + "- " + oneLine(r.Text)
			if *withAddr {
				line = r.Addr.String() + "  " + line
			}
			fmt.Fprintf(stdout, "%s  %s\n", line, r.Page)
		}
	}
	return exitOK
}

// cmdLinks resolves what a block points at.
func cmdLinks(args []string, stdout, stderr io.Writer, svc *app.Service) int {
	fs := flags("links", stderr)
	asJSON := fs.Bool("json", false, "print as JSON")
	if err := parseFlags(fs, args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "tlog links: which block? give its address")
		return exitUsage
	}
	a, err := app.ParseAddr(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "tlog links: %v\n", err)
		return exitUsage
	}
	links, err := svc.Links(a)
	if err != nil {
		return fail(stderr, err)
	}
	if *asJSON {
		if err := emit(stdout, links); err != nil {
			return fail(stderr, err)
		}
		return exitOK
	}
	for _, l := range links {
		where := l.Resolved
		switch {
		case l.Error != "":
			where = "refused: " + l.Error
		case l.Addr != nil:
			where = l.Addr.String()
		case l.Kind == "page":
			where = l.Rel
			if !l.Exists {
				where += "  (not created yet)"
			}
		}
		fmt.Fprintf(stdout, "%-6s %-30s %s\n", l.Kind, truncate(l.Target, 30), where)
	}
	return exitOK
}

// cmdDate says what a typed shorthand means before anything is written with
// it — the reason dates are refused rather than guessed at.
func cmdDate(args []string, stdout, stderr io.Writer, _ *app.Service) int {
	fs := flags("date", stderr)
	asJSON := fs.Bool("json", false, "print as JSON")
	limit := fs.Int("n", 8, "how many to offer")
	if err := parseFlags(fs, args); err != nil {
		return exitUsage
	}
	input := strings.Join(fs.Args(), " ")
	choices := app.DateChoices(input, *limit)
	if len(choices) == 0 {
		fmt.Fprintf(stderr, "tlog: %q is not a date I understand\n", input)
		return exitRefused
	}
	if *asJSON {
		if err := emit(stdout, choices); err != nil {
			return fail(stderr, err)
		}
		return exitOK
	}
	for _, c := range choices {
		fmt.Fprintf(stdout, "%-16s %s  %-10s %s\n", c.Phrase, c.ISO, c.Short, c.Label)
	}
	return exitOK
}

// cmdPeople lists every page tagged person. It is `tlog tag person` under the
// name people look for, and keeps its JSON a plain list of names.
func cmdPeople(args []string, stdout, stderr io.Writer, svc *app.Service) int {
	fs := flags("people", stderr)
	asJSON := fs.Bool("json", false, "print as JSON")
	if err := parseFlags(fs, args); err != nil {
		return exitUsage
	}
	people, err := svc.People()
	if err != nil {
		return fail(stderr, err)
	}
	if *asJSON {
		if err := emit(stdout, people); err != nil {
			return fail(stderr, err)
		}
		return exitOK
	}
	for _, p := range people {
		fmt.Fprintln(stdout, p)
	}
	return exitOK
}

// cmdTag lists the pages carrying a tag, or, given pages, puts the tag on them.
func cmdTag(args []string, stdout, stderr io.Writer, svc *app.Service) int {
	fs := flags("tag", stderr)
	asJSON := fs.Bool("json", false, "print as JSON")
	if err := parseFlags(fs, args); err != nil {
		return exitUsage
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(stderr, "tlog tag: which tag? tlog tag person, or tlog tag repo tlog to tag a page")
		return exitUsage
	}
	tag := fs.Arg(0)
	if fs.NArg() == 1 {
		pages, err := svc.Tagged(tag)
		if err != nil {
			return fail(stderr, err)
		}
		if *asJSON {
			if err := emit(stdout, pages); err != nil {
				return fail(stderr, err)
			}
			return exitOK
		}
		for _, p := range pages {
			fmt.Fprintln(stdout, p.Name)
		}
		return exitOK
	}
	return tagPages(stdout, stderr, svc, tag, fs.Args()[1:], svc.Tag, "is tagged")
}

// cmdUntag takes a tag out of pages' frontmatter.
func cmdUntag(args []string, stdout, stderr io.Writer, svc *app.Service) int {
	fs := flags("untag", stderr)
	if err := parseFlags(fs, args); err != nil {
		return exitUsage
	}
	if fs.NArg() < 2 {
		fmt.Fprintln(stderr, "tlog untag: which tag, from which pages? tlog untag repo tlog")
		return exitUsage
	}
	return tagPages(stdout, stderr, svc, fs.Arg(0), fs.Args()[1:], svc.Untag, "is no longer tagged")
}

// tagPages applies a tag operation to each named page. A page is found the way
// a link finds it, aliases included; one that has never been written is made,
// because saying what a page is is a reason to have one.
func tagPages(stdout, stderr io.Writer, svc *app.Service, tag string, pages []string,
	apply func(rel, tag string) error, done string) int {
	for _, name := range pages {
		rel, _, err := svc.ResolvePage(name)
		if err != nil {
			return fail(stderr, err)
		}
		if err := apply(rel, tag); err != nil {
			settle(stderr, svc)
			return fail(stderr, err)
		}
		fmt.Fprintf(stdout, "%s %s %s\n", name, done, strings.TrimPrefix(tag, "#"))
	}
	settle(stderr, svc)
	return exitOK
}

// cmdMentions is the @ menu: who or what a few letters could mean.
func cmdMentions(args []string, stdout, stderr io.Writer, svc *app.Service) int {
	fs := flags("mentions", stderr)
	asJSON := fs.Bool("json", false, "print as JSON")
	limit := fs.Int("n", 8, "how many at most")
	if err := parseFlags(fs, args); err != nil {
		return exitUsage
	}
	items, err := svc.Mentions(strings.Join(fs.Args(), " "), *limit)
	if err != nil {
		return fail(stderr, err)
	}
	if *asJSON {
		if err := emit(stdout, items); err != nil {
			return fail(stderr, err)
		}
		return exitOK
	}
	for _, m := range items {
		fmt.Fprintf(stdout, "%-7s %-30s %-24s %s\n", m.Kind, truncate(m.Name, 30), m.Insert, m.Detail)
	}
	return exitOK
}

// cmdCommands lists the slash commands, for `tlog block <addr> cmd`.
func cmdCommands(args []string, stdout, stderr io.Writer, _ *app.Service) int {
	fs := flags("commands", stderr)
	asJSON := fs.Bool("json", false, "print as JSON")
	if err := parseFlags(fs, args); err != nil {
		return exitUsage
	}
	cmds := app.Commands()
	if *asJSON {
		if err := emit(stdout, cmds); err != nil {
			return fail(stderr, err)
		}
		return exitOK
	}
	for _, c := range cmds {
		arg := ""
		if c.Arg != "" {
			arg = "<" + c.Arg + ">"
		}
		fmt.Fprintf(stdout, "%-10s %-12s %s\n", c.Name, arg, c.Hint)
	}
	return exitOK
}

// cmdSync reports where the notes stand with their remote.
func cmdSync(args []string, stdout, stderr io.Writer, svc *app.Service) int {
	fs := flags("sync", stderr)
	asJSON := fs.Bool("json", false, "print as JSON")
	if err := parseFlags(fs, args); err != nil {
		return exitUsage
	}
	st := svc.Sync()
	if *asJSON {
		if err := emit(stdout, st); err != nil {
			return fail(stderr, err)
		}
		return exitOK
	}
	switch {
	case !st.Remote:
		fmt.Fprintln(stdout, "no remote: nothing is pushed")
	case st.AutoPush:
		fmt.Fprintln(stdout, "pushing on every commit")
	default:
		fmt.Fprintln(stdout, "pushing is manual: tlog push")
	}
	if st.LastErr != "" {
		fmt.Fprintf(stdout, "last push failed: %s\n", st.LastErr)
	}
	return exitOK
}

// cmdUndo takes back the last thing tlog committed, for a script that has no
// session to keep an undo history in.
func cmdUndo(args []string, stdout, stderr io.Writer, svc *app.Service) int {
	fs := flags("undo", stderr)
	asJSON := fs.Bool("json", false, "print as JSON")
	if err := parseFlags(fs, args); err != nil {
		return exitUsage
	}
	if fs.NArg() > 1 {
		fmt.Fprintln(stderr, "tlog undo: one file, or none for the last thing tlog committed")
		return exitUsage
	}
	done, err := svc.UndoCommitted(fs.Arg(0))
	if err != nil {
		return fail(stderr, err)
	}
	if *asJSON {
		if err := emit(stdout, done); err != nil {
			return fail(stderr, err)
		}
		return exitOK
	}
	for _, rel := range done.Rels {
		fmt.Fprintf(stdout, "undone  %s\n", rel)
	}
	return exitOK
}

// oneLine flattens a block for a line-per-block listing.
func oneLine(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
}
