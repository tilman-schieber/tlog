// Package cli is a thin adapter over the core. It parses arguments, calls
// app.Service and prints; it decides nothing on its own.
package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/tilman-schieber/tlog/internal/app"
	"github.com/tilman-schieber/tlog/internal/config"
	"github.com/tilman-schieber/tlog/internal/importer"
	"github.com/tilman-schieber/tlog/internal/tui"
)

// Version is set at build time.
var Version = "0.1.0-dev"

const usage = `tlog — a markdown-first knowledge outliner

usage:
  tlog                        open today's journal in the outliner
  tlog edit <page>            open a page, or a day like 2026-11-03, in the outliner
  tlog today                  print the path of today's journal
  tlog open <page>            print the path of a page, creating it if needed
  tlog add [-p page] <text>   append a block to today's journal, or to a page
  tlog add -todo [-due d] <t> add an open task, with a deadline; -p for a page
  tlog todo                   every open task, dated first, then the undated ones
  tlog due [-all]             what is dated and still open, soonest first
  tlog attach [-p page] FILE  copy files into the notes' assets/ and link them here
  tlog attach -link-only FILE copy a file into assets/ and print its link, no note
  tlog attach -migrate        bring files linked on the old ~/.att shelf into assets/
  tlog files [query] [-links] what is attached, newest first
  tlog alias <page> <name>    give a page another name
  tlog config                 the settings; <key> reads one, <key> <value> sets it
  tlog push [-auto on|off]    push the notes now, or set pushing on every commit
  tlog sync                   where the notes stand with their remote
  tlog import [-from dir]     import a Logseq graph into the notes directory
  tlog version                print the version

for scripts and agents (every one takes -json; writes print the new address):
  tlog search <query> [-n N]  find blocks, each with an address
  tlog view [page] [-day d]   print a page, or a day: -day yesterday, -day -1
  tlog view ... -addr         ...with an address per block
  tlog block <addr>           print one block
  tlog block <addr> <op> ...  change one block — see: tlog block help
  tlog append <rel> [-if-match hash] <text>
                              add a block at the end of a file
  tlog links <addr>           where a block's links go
  tlog index                  every journal, page and tag; -json gives paths
  tlog places [query]         the same, ranked against a query
  tlog date [shorthand]       what a typed date means before writing it
  tlog tag <tag>              every page tagged <tag>; tlog people is tag person
  tlog tag <tag> <page>…      put a tag in pages' frontmatter
  tlog untag <tag> <page>…    take it out again
  tlog mentions <query>       who or what a few letters could mean
  tlog commands               the slash commands, for: tlog block <addr> cmd
  tlog undo [rel]             take back the last thing tlog committed

An address is pages/Note.md:42@9f3a2b1c8d4e5f60 — the file, where the block
starts, and the hash of the file it was read from. A write from a stale read
is refused rather than landing in the wrong place. Text may be "-" to read it
from standard input. Flags may come before or after the arguments; what
follows "--" is never a flag.

exit codes: 0 done · 1 something went wrong · 2 not a command, operation or
address · 3 that does not exist, retrying will not help · 4 the file moved
since you read it, read again and retry

global flags, before the command:
  -dir <path>   notes directory (default $TLOG_DIR, else ~/notes)

The notes directory is plain markdown and a git repository. Nothing here owns
your data: read it with cat, search it with rg, edit it with nvim.
`

// Main runs the command line and returns a process exit code.
func Main(args []string, stdout, stderr io.Writer) int {
	dir := ""
	rest := args
	// -dir may appear before the subcommand.
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		switch {
		case rest[0] == "-dir" && len(rest) > 1:
			dir, rest = rest[1], rest[2:]
		case strings.HasPrefix(rest[0], "-dir="):
			dir, rest = strings.TrimPrefix(rest[0], "-dir="), rest[1:]
		case rest[0] == "-h" || rest[0] == "--help" || rest[0] == "-help":
			fmt.Fprint(stdout, usage)
			return exitOK
		case rest[0] == "-v" || rest[0] == "--version":
			fmt.Fprintln(stdout, Version)
			return exitOK
		default:
			fmt.Fprintf(stderr, "tlog: unknown flag %s\n\n%s", rest[0], usage)
			return exitUsage
		}
	}

	cmd := ""
	if len(rest) > 0 {
		cmd, rest = rest[0], rest[1:]
	}

	svc, err := app.New(dir)
	if err != nil {
		fmt.Fprintf(stderr, "tlog: %v\n", err)
		return exitError
	}

	commands := map[string]func([]string, io.Writer, io.Writer, *app.Service) int{
		"todo": cmdTodo, "add": cmdAdd, "import": cmdImport, "push": cmdPush,
		"due": cmdDue, "attach": cmdAttach, "config": cmdConfig, "files": cmdFiles,
		"search": cmdSearch, "view": cmdView, "index": cmdIndex, "block": cmdBlock,
		"alias": cmdAlias, "commands": cmdCommands, "date": cmdDate, "people": cmdPeople,
		"mentions": cmdMentions, "tag": cmdTag, "untag": cmdUntag, "links": cmdLinks, "undo": cmdUndo, "append": cmdAppend,
		"sync": cmdSync, "places": cmdPlaces,
	}

	switch cmd {
	case "":
		return run(stderr, svc, func() error { return tui.Run(svc, "") })
	case "edit":
		if len(rest) == 0 {
			fmt.Fprintln(stderr, "tlog edit: which page?")
			return exitUsage
		}
		return run(stderr, svc, func() error {
			rel, err := svc.OpenPage(strings.Join(rest, " "))
			if err != nil {
				return err
			}
			return tui.Run(svc, rel)
		})
	case "today":
		fmt.Fprintln(stdout, svc.Store.Abs(svc.TodayRel()))
		return exitOK
	case "open":
		if len(rest) == 0 {
			fmt.Fprintln(stderr, "tlog open: which page?")
			return exitUsage
		}
		return run(stderr, svc, func() error {
			rel, err := svc.OpenPage(strings.Join(rest, " "))
			if err != nil {
				return err
			}
			fmt.Fprintln(stdout, svc.Store.Abs(rel))
			return nil
		})
	case "version":
		fmt.Fprintln(stdout, Version)
		return exitOK
	case "help":
		fmt.Fprint(stdout, usage)
		return exitOK
	}
	if fn, ok := commands[cmd]; ok {
		return fn(rest, stdout, stderr, svc)
	}
	fmt.Fprintf(stderr, "tlog: unknown command %q\n\n%s", cmd, usage)
	return exitUsage
}

// run executes an operation and flushes the auto-commit, so that a one-shot
// command never leaves uncommitted changes behind.
func run(stderr io.Writer, svc *app.Service, fn func() error) int {
	err := fn()
	if cerr := svc.Commit(); cerr != nil && err == nil {
		fmt.Fprintf(stderr, "tlog: %v\n", cerr)
	}
	if err != nil {
		return fail(stderr, err)
	}
	return exitOK
}

func cmdAdd(args []string, stdout, stderr io.Writer, svc *app.Service) int {
	fs := flags("add", stderr)
	page := fs.String("p", "", "append to this page instead of today's journal")
	fs.StringVar(page, "page", "", "append to this page instead of today's journal")
	todo := fs.Bool("todo", false, "make it an open task")
	due := fs.String("due", "", "a deadline for the task: fri, in 3 days, 12.11, 2026-11-12")
	asJSON := fs.Bool("json", false, "print the new block's address as JSON")
	if err := parseFlags(fs, args); err != nil {
		return exitUsage
	}

	text, err := textFrom(fs.Args())
	if err != nil {
		fmt.Fprintf(stderr, "tlog add: %v\n", err)
		return exitError
	}
	if strings.TrimSpace(text) == "" && fs.NArg() == 0 {
		// No argument at all: the block comes from stdin, so that piping works.
		if text, err = textFrom([]string{"-"}); err != nil {
			fmt.Fprintf(stderr, "tlog add: %v\n", err)
			return exitError
		}
	}
	if strings.TrimSpace(text) == "" {
		fmt.Fprintln(stderr, "tlog add: nothing to add")
		return exitUsage
	}

	rel := svc.TodayRel()
	if *page != "" {
		r, _, err := svc.ResolvePage(*page)
		if err != nil {
			return fail(stderr, err)
		}
		rel = r
	}
	var res *app.Result
	if *todo || *due != "" {
		res, err = svc.AddTodoTo(rel, text, *due)
	} else {
		res, err = svc.Add(rel, text)
	}
	if err != nil {
		settle(stderr, svc)
		return fail(stderr, err)
	}
	if *asJSON {
		return report(stdout, stderr, svc, res, true)
	}
	settle(stderr, svc)
	fmt.Fprintln(stdout, svc.Store.Abs(res.Rel))
	return exitOK
}

// cmdAppend adds a block at the end of a file, guarded by the file's hash when
// one is given — the form a script uses when it has read the page and wants
// to be sure nothing moved in between.
func cmdAppend(args []string, stdout, stderr io.Writer, svc *app.Service) int {
	fs := flags("append", stderr)
	ifMatch := fs.String("if-match", "", "the file's hash as last read; refused if it moved")
	asJSON := fs.Bool("json", false, "print the new block's address as JSON")
	if err := parseFlags(fs, args); err != nil {
		return exitUsage
	}
	if fs.NArg() < 2 {
		fmt.Fprintln(stderr, "tlog append: which file, and what? tlog append journals/2026-11-03.md \"the text\"")
		return exitUsage
	}
	text, err := textFrom(fs.Args()[1:])
	if err != nil {
		fmt.Fprintf(stderr, "tlog append: %v\n", err)
		return exitError
	}
	if strings.TrimSpace(text) == "" {
		fmt.Fprintln(stderr, "tlog append: nothing to add")
		return exitUsage
	}
	res, err := svc.AppendBlock(fs.Arg(0), *ifMatch, text)
	if err != nil {
		return fail(stderr, err)
	}
	return report(stdout, stderr, svc, res, *asJSON)
}

// cmdAlias gives a page another name, the way /alias does from inside it.
func cmdAlias(args []string, stdout, stderr io.Writer, svc *app.Service) int {
	fs := flags("alias", stderr)
	if err := parseFlags(fs, args); err != nil {
		return exitUsage
	}
	if fs.NArg() < 2 {
		fmt.Fprintln(stderr, "tlog alias: which page, and what name? tlog alias \"Ada Lovelace\" Ada")
		return exitUsage
	}
	rel, err := svc.FindPage(fs.Arg(0))
	if err != nil {
		return fail(stderr, err)
	}
	alias := strings.Join(fs.Args()[1:], " ")
	if err := svc.AddAlias(rel, alias); err != nil {
		return fail(stderr, err)
	}
	settle(stderr, svc)
	fmt.Fprintf(stdout, "%s is also %s\n", fs.Arg(0), alias)
	return exitOK
}

// cmdConfig shows the settings, reads one, or changes one. The list is the
// core's: the same one the outliner and the window show.
func cmdConfig(args []string, stdout, stderr io.Writer, svc *app.Service) int {
	fs := flags("config", stderr)
	asJSON := fs.Bool("json", false, "print the settings as JSON")
	if err := parseFlags(fs, args); err != nil {
		return exitUsage
	}
	rest := fs.Args()
	if len(rest) == 0 {
		if *asJSON {
			if err := emit(stdout, svc.Settings()); err != nil {
				return fail(stderr, err)
			}
			return exitOK
		}
		fmt.Fprintf(stdout, "# %s", svc.ConfigPath())
		if !config.Exists() {
			fmt.Fprint(stdout, "  (not written yet — these are the defaults)")
		}
		fmt.Fprintln(stdout)
		for _, st := range svc.Settings() {
			where := ""
			if st.Source == "notes" {
				where = "  [notes repo]"
			}
			value := st.Value
			if value == "" {
				value = "—"
			}
			fmt.Fprintf(stdout, "%-24s %s%s\n", st.Key, value, where)
			fmt.Fprintf(stdout, "%-24s %s\n", "", st.Hint)
		}
		if svc.CfgErr != nil {
			fmt.Fprintf(stderr, "\ntlog: %v\n", svc.CfgErr)
			return exitError
		}
		return exitOK
	}
	if len(rest) == 1 {
		for _, st := range svc.Settings() {
			if st.Key == rest[0] {
				if *asJSON {
					if err := emit(stdout, st); err != nil {
						return fail(stderr, err)
					}
					return exitOK
				}
				fmt.Fprintln(stdout, st.Value)
				return exitOK
			}
		}
		fmt.Fprintf(stderr, "tlog: no such setting: %s\n", rest[0])
		return exitUsage
	}

	value := strings.Join(rest[1:], " ")
	note, err := svc.SetSetting(rest[0], value)
	if err != nil {
		fmt.Fprintf(stderr, "tlog: %v\n", err)
		return exitUsage
	}
	if *asJSON {
		if err := emit(stdout, map[string]any{"settings": svc.Settings(), "note": note}); err != nil {
			return fail(stderr, err)
		}
		return exitOK
	}
	fmt.Fprintf(stdout, "%s is now %s\n", rest[0], value)
	if note != "" {
		fmt.Fprintf(stdout, "(%s)\n", note)
	}
	return exitOK
}

func cmdAttach(args []string, stdout, stderr io.Writer, svc *app.Service) int {
	fs := flags("attach", stderr)
	page := fs.String("p", "", "attach to this page instead of today's journal")
	only := fs.Bool("link-only", false, "copy the file into assets/ and print the link, writing no note")
	asJSON := fs.Bool("json", false, "print what was attached as JSON")
	migrate := fs.Bool("migrate", false, "copy every file notes link on the old att shelf into assets/ and relink it")
	from := fs.String("from", "", "with -migrate: the old shelf (default ~/.att/store)")
	dry := fs.Bool("dry-run", false, "with -migrate: say what would happen, change nothing")
	if err := parseFlags(fs, args); err != nil {
		return exitUsage
	}
	if *migrate {
		if fs.NArg() > 0 {
			fmt.Fprintln(stderr, "tlog attach -migrate: takes no files; it finds them in the notes")
			return exitUsage
		}
		return migrateShelf(stdout, stderr, svc, *from, *dry, *asJSON)
	}
	if *from != "" || *dry {
		fmt.Fprintln(stderr, "tlog attach: -from and -dry-run only go with -migrate")
		return exitUsage
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(stderr, "tlog attach: which file?")
		return exitUsage
	}

	rel := svc.TodayRel()
	if *page != "" && !*only {
		r, _, err := svc.ResolvePage(*page)
		if err != nil {
			return fail(stderr, err)
		}
		rel = r
	}
	var added []app.AttachView
	var err error
	if *only {
		added, err = svc.Attach(fs.Args()...)
	} else {
		added, err = svc.AttachTo(rel, fs.Args()...)
	}
	settle(stderr, svc)
	if *asJSON {
		if added == nil {
			added = []app.AttachView{}
		}
		if eerr := emit(stdout, added); eerr != nil {
			return fail(stderr, eerr)
		}
		if err != nil {
			return fail(stderr, err)
		}
		return exitOK
	}
	for _, a := range added {
		if *only {
			fmt.Fprintln(stdout, a.Link)
		} else {
			fmt.Fprintf(stdout, "%s  %s\n", a.Name, a.Size)
		}
	}
	if err != nil {
		return fail(stderr, err)
	}
	if !*only && len(added) > 0 {
		fmt.Fprintln(stdout, svc.Store.Abs(rel))
	}
	return exitOK
}

func migrateShelf(stdout, stderr io.Writer, svc *app.Service, from string, dry, asJSON bool) int {
	m, err := svc.MigrateShelf(from, dry)
	if !dry {
		settle(stderr, svc)
	}
	if asJSON && m != nil {
		if eerr := emit(stdout, m); eerr != nil {
			return fail(stderr, eerr)
		}
		if err != nil {
			return fail(stderr, err)
		}
		return exitOK
	}
	if m != nil {
		verb, files := "rewrote", "copied"
		if dry {
			verb, files = "would rewrite", "would copy"
		}
		for _, c := range m.Copied {
			if c.Rel != "" {
				fmt.Fprintf(stdout, "%s → %s\n", c.From, c.Rel)
			} else {
				fmt.Fprintf(stdout, "%s\n", c.From)
			}
		}
		for _, x := range m.Missing {
			fmt.Fprintf(stdout, "not on this machine, left as it is: %s  (%s)\n", x.Target, x.Note)
		}
		fmt.Fprintf(stdout, "%s %d files from %s; %s %d links in %d notes\n",
			files, len(m.Copied), m.From, verb, m.Links, len(m.Notes))
	}
	if err != nil {
		return fail(stderr, err)
	}
	return exitOK
}

func cmdFiles(args []string, stdout, stderr io.Writer, svc *app.Service) int {
	fs := flags("files", stderr)
	links := fs.Bool("links", false, "print the Markdown links instead of a table")
	asJSON := fs.Bool("json", false, "print as JSON")
	if err := parseFlags(fs, args); err != nil {
		return exitUsage
	}

	items, err := svc.Attachments(strings.Join(fs.Args(), " "), 0)
	if err != nil {
		return fail(stderr, err)
	}
	if *asJSON {
		if err := emit(stdout, items); err != nil {
			return fail(stderr, err)
		}
		return exitOK
	}
	if len(items) == 0 {
		fmt.Fprintf(stdout, "nothing attached yet in %s\n", svc.AttachDir())
		return exitOK
	}
	for _, a := range items {
		if *links {
			fmt.Fprintln(stdout, a.Link)
			continue
		}
		fmt.Fprintf(stdout, "%s  %9s  %s\n", a.When, a.Size, a.Name)
	}
	return exitOK
}

func cmdDue(args []string, stdout, stderr io.Writer, svc *app.Service) int {
	fs := flags("due", stderr)
	all := fs.Bool("all", false, "include what is already done")
	asJSON := fs.Bool("json", false, "print as JSON, with an address for each")
	if err := parseFlags(fs, args); err != nil {
		return exitUsage
	}

	items, err := svc.Due(*all)
	if err != nil {
		return fail(stderr, err)
	}
	if *asJSON {
		if err := emit(stdout, items); err != nil {
			return fail(stderr, err)
		}
		return exitOK
	}
	if len(items) == 0 {
		fmt.Fprintln(stdout, "nothing dated is open")
		return exitOK
	}
	for _, it := range items {
		mark := " "
		if it.Done {
			mark = "x"
		}
		fmt.Fprintf(stdout, "%s [%s] %-11s %-18s %s  (%s)\n",
			urgency(it.State), mark, it.Due, truncate(it.Label, 18), truncate(it.Text, 60), it.Page)
	}
	return exitOK
}

// cmdTodo lists every open task. With -json each carries an address, which is
// what a bar widget needs to tick one off with `tlog block <addr> done`.
func cmdTodo(args []string, stdout, stderr io.Writer, svc *app.Service) int {
	fs := flags("todo", stderr)
	asJSON := fs.Bool("json", false, "print as JSON, with an address for each")
	if err := parseFlags(fs, args); err != nil {
		return exitUsage
	}
	items, err := svc.Todos()
	if err != nil {
		return fail(stderr, err)
	}
	if *asJSON {
		if err := emit(stdout, items); err != nil {
			return fail(stderr, err)
		}
		return exitOK
	}
	if len(items) == 0 {
		fmt.Fprintln(stdout, "nothing to do")
		return exitOK
	}
	for _, it := range items {
		fmt.Fprintf(stdout, "%s [ ] %-11s %-18s %s  (%s)\n",
			urgency(it.State), it.Due, truncate(it.Label, 18), truncate(it.Text, 60), it.Page)
	}
	return exitOK
}

// urgency is the one-character flag in front of a dated line.
func urgency(state string) string {
	switch state {
	case "overdue":
		return "!"
	case "today":
		return "*"
	}
	return " "
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func cmdPush(args []string, stdout, stderr io.Writer, svc *app.Service) int {
	fs := flags("push", stderr)
	auto := fs.String("auto", "", "turn pushing on every commit on or off")
	if err := parseFlags(fs, args); err != nil {
		return exitUsage
	}

	if *auto != "" {
		on, err := app.ParseBool(*auto)
		if err != nil {
			fmt.Fprintf(stderr, "tlog push: -auto takes on or off, not %q\n", *auto)
			return exitUsage
		}
		if err := svc.SetAutoPush(on); err != nil {
			return fail(stderr, err)
		}
		if on {
			fmt.Fprintf(stdout, "pushing on every commit, to %s\n", svc.Git.Remote())
		} else {
			fmt.Fprintln(stdout, "pushing is now manual")
		}
		return exitOK
	}

	if !svc.Sync().Remote {
		fmt.Fprintln(stderr, "tlog: no remote; set one with `tlog config git.remote <url>`")
		return exitError
	}
	if err := svc.PushNow(); err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintln(stdout, "pushed")
	return exitOK
}

func cmdImport(args []string, stdout, stderr io.Writer, svc *app.Service) int {
	fs := flags("import", stderr)
	from := fs.String("from", "", "Logseq graph directory (default ~/logseq)")
	dry := fs.Bool("dry-run", false, "report what would happen without writing")
	if err := parseFlags(fs, args); err != nil {
		return exitUsage
	}

	return run(stderr, svc, func() error {
		rep, err := svc.Import(importer.Options{Source: *from, DryRun: *dry})
		if err != nil {
			return err
		}
		if *dry {
			fmt.Fprintf(stdout, "dry run — nothing was written\n")
		}
		fmt.Fprint(stdout, rep.String())
		if !*dry {
			fmt.Fprintf(stdout, "wrote %d files into %s\n", len(rep.Written), svc.Store.Root)
		}
		return nil
	})
}
