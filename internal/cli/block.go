package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/tilman-schieber/tlog/internal/app"
)

// `tlog block <addr> <op>` is every mutation the core has, reachable from a
// script. The address comes from `tlog search -json`, `tlog view -json` or
// from the result of the previous write — each one prints the address the
// block ended up at, because rewriting a file moves every offset after it.
//
// The hash inside the address is the whole safety story. A write computed
// against a file that has since changed is refused with exit code 4, never
// merged and never applied to whatever now happens to be at that offset.

const blockUsage = `tlog block <addr> [operation]

  (none)                   print the block: its text, properties and deadline
  text <text>              replace the block's text
  after [-child] <text>    add a block below it, or as its first child
  before <text>            add a block above it
  split [-child] <before> <after>
                           end the block at a point and start the next one
  merge                    join it onto the block above
  indent                   nest it under the block above
  outdent                  move it out to its parent's level
  up / down                move it among its siblings
  task                     toggle: plain -> open -> done -> open
  open / done              make it an open task, or a done one; never toggles
  deadline <date>          due on a date, as a task: fri, in 3 days, 2026-11-12;
                           none takes the deadline off
  quote                    make it a quotation
  prop <key> [value]       set a block property; no value removes it
  cmd <name> [arg]         any slash command by name — see: tlog commands
  ref                      give it a durable name and print a link to it
  delete                   remove it and everything under it

Text may be "-" to read it from standard input, for a block of several lines.

An address is what search and view print: pages/Note.md:42@9f3a2b1c8d4e5f60
It carries the hash of the file it was read from, so a write from a stale
read is refused (exit 4) rather than landing in the wrong place. Every write
prints the block's new address, to carry on from.

flags:
  -json   print the result as JSON instead of the new address
`

func cmdBlock(args []string, stdout, stderr io.Writer, svc *app.Service) int {
	fs := flags("block", stderr)
	asJSON := fs.Bool("json", false, "print the result as JSON")
	if err := parseFlags(fs, args); err != nil {
		return exitUsage
	}
	rest := fs.Args()
	if len(rest) == 0 || rest[0] == "help" {
		fmt.Fprint(stdout, blockUsage)
		return exitOK
	}

	a, err := app.ParseAddr(rest[0])
	if err != nil {
		fmt.Fprintf(stderr, "tlog block: %v\n", err)
		return exitUsage
	}
	if len(rest) == 1 {
		return readBlock(stdout, stderr, svc, a, *asJSON)
	}
	op, opArgs := rest[1], rest[2:]

	// `ref` is the one operation whose answer is a link as well as an address.
	if op == "ref" {
		link, res, err := svc.Name(a)
		if err != nil {
			return fail(stderr, err)
		}
		warning := settle(stderr, svc)
		if *asJSON {
			if err := emit(stdout, struct {
				Link string `json:"link"`
				written
			}{link, written{Addr: res.Addr, Rel: res.Rel, Offset: res.Offset, Hash: res.Hash, Warning: warning}}); err != nil {
				return fail(stderr, err)
			}
			return exitOK
		}
		fmt.Fprintln(stdout, link)
		return exitOK
	}

	res, code := applyBlockOp(svc, a, op, opArgs, stderr)
	if code != exitOK {
		return code
	}
	return report(stdout, stderr, svc, res, *asJSON)
}

// readBlock prints one block, which is what an address with no operation
// means: a look before a write.
func readBlock(stdout, stderr io.Writer, svc *app.Service, a app.Addr, asJSON bool) int {
	b, err := svc.Block(a)
	if err != nil {
		return fail(stderr, err)
	}
	if asJSON {
		if err := emit(stdout, b); err != nil {
			return fail(stderr, err)
		}
		return exitOK
	}
	fmt.Fprintln(stdout, b.Text)
	for _, p := range b.Props {
		if app.IsDeadlineKey(p.Key) {
			continue
		}
		fmt.Fprintf(stdout, "%s:: %s\n", p.Key, p.Value)
	}
	if b.Due != "" {
		fmt.Fprintf(stdout, "%s:: %s  (%s)\n", app.DeadlineProp, b.Due, b.DueLabel)
	}
	return exitOK
}

func applyBlockOp(svc *app.Service, a app.Addr, op string, args []string, stderr io.Writer) (*app.Result, int) {
	usage := func(format string, v ...any) (*app.Result, int) {
		fmt.Fprintf(stderr, "tlog block: "+format+"\n", v...)
		return nil, exitUsage
	}
	// -child may be written before the text of after and split.
	asChild := false
	if len(args) > 0 && (args[0] == "-child" || args[0] == "--child") {
		asChild, args = true, args[1:]
	}
	// The text of an operation that takes one, with "-" standing for stdin.
	// An operation that writes a block refuses to write an empty one by
	// accident: a missing argument is a mistake, not a request.
	text := func(what string) (string, bool) {
		t, err := textFrom(args)
		if err != nil {
			fmt.Fprintf(stderr, "tlog block: %v\n", err)
			return "", false
		}
		if strings.TrimSpace(t) == "" {
			usage("%s needs %s", op, what)
			return "", false
		}
		return t, true
	}

	var res *app.Result
	var err error
	switch op {
	case "text":
		t, ok := text("the new text")
		if !ok {
			return nil, exitUsage
		}
		res, err = svc.SetText(a, t)
	case "after":
		t, ok := text("the text of the new block")
		if !ok {
			return nil, exitUsage
		}
		res, err = svc.InsertAfter(a, t, asChild)
	case "before":
		t, ok := text("the text of the new block")
		if !ok {
			return nil, exitUsage
		}
		res, err = svc.InsertBefore(a, t)
	case "split":
		if len(args) < 2 {
			return usage("split needs the text before and the text after")
		}
		res, err = svc.SplitBlock(a, args[0], args[1], asChild)
	case "merge":
		res, err = svc.MergeIntoPrevious(a)
	case "indent":
		res, err = svc.Indent(a)
	case "outdent":
		res, err = svc.Outdent(a)
	case "up":
		res, err = svc.Move(a, -1)
	case "down":
		res, err = svc.Move(a, 1)
	case "task":
		res, err = svc.ToggleTask(a)
	case "open":
		res, err = command(svc, a, "todo", "")
	case "done":
		res, err = command(svc, a, "done", "")
	case "quote":
		res, err = command(svc, a, "quote", "")
	case "deadline":
		if len(args) == 0 {
			return usage("deadline needs a date: fri, in 3 days, 2026-11-12")
		}
		res, err = command(svc, a, "deadline", strings.Join(args, " "))
	case "cmd":
		if len(args) == 0 {
			return usage("cmd needs the command's name — see: tlog commands")
		}
		res, err = command(svc, a, args[0], strings.Join(args[1:], " "))
	case "delete":
		res, err = svc.DeleteBlock(a)
	case "prop":
		if len(args) == 0 {
			return usage("prop needs a property name")
		}
		res, err = svc.SetProperty(a, args[0], strings.Join(args[1:], " "))
	default:
		fmt.Fprintf(stderr, "tlog block: no operation %q\n\n%s", op, blockUsage)
		return nil, exitUsage
	}

	if err != nil {
		return nil, fail(stderr, err)
	}
	return res, exitOK
}

// command runs a slash command by name, which is how open, done, deadline and
// quote reach the same code the menus run.
func command(svc *app.Service, a app.Addr, name, arg string) (*app.Result, error) {
	r, err := svc.ApplyCommand(a, name, arg)
	if err != nil {
		return nil, err
	}
	return r.Result, nil
}
