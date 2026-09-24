package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/tilman-schieber/tlog/internal/app"
)

// `tlog block <addr> <op> [args]` is every mutation the core has, reachable
// from a script. The address comes from `tlog search -json`, `tlog view -json`
// or from the result of the previous write — each one prints the address the
// block ended up at, because rewriting a file moves every offset after it.
//
// The hash inside the address is the whole safety story. A write computed
// against a file that has since changed is refused with exit code 4, never
// merged and never applied to whatever now happens to be at that offset.

const blockUsage = `tlog block <addr> <operation>

  text <new text>      replace the block's text
  after [-child] <t>   add a block below it, or as its first child
  before <text>        add a block above it
  split <before> <after>   end the block at a point and start the next one
  merge                join it onto the block above
  indent               nest it under the block above
  outdent              move it out to its parent's level
  up / down            move it among its siblings
  task                 plain -> open -> done -> plain
  prop <key> [value]   set a block property; no value removes it
  ref                  give it a durable name and print a link to it
  delete               remove it and everything under it

An address is what search and view print: pages/Note.md:42@9f3a2b1c8d4e5f60
It carries the hash of the file it was read from, so a write from a stale
read is refused (exit 4) rather than landing in the wrong place.

flags:
  -json   print the result as JSON instead of the new address
`

func cmdBlock(args []string, stdout, stderr io.Writer, svc *app.Service) int {
	fs := flag.NewFlagSet("block", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "print the result as JSON")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	rest := fs.Args()
	if len(rest) == 0 || rest[0] == "help" {
		fmt.Fprint(stdout, blockUsage)
		return exitOK
	}
	if len(rest) < 2 {
		fmt.Fprintf(stderr, "tlog block: which operation?\n\n%s", blockUsage)
		return exitUsage
	}

	a, err := app.ParseAddr(rest[0])
	if err != nil {
		fmt.Fprintf(stderr, "tlog block: %v\n", err)
		return exitUsage
	}
	op, opArgs := rest[1], rest[2:]

	// `ref` is the one operation whose answer is not an address, so it is
	// handled before the table.
	if op == "ref" {
		link, err := svc.RefTo(a)
		if err != nil {
			return fail(stderr, err)
		}
		if cerr := svc.Commit(); cerr != nil {
			fmt.Fprintf(stderr, "tlog: %v\n", cerr)
		}
		if *asJSON {
			if err := emit(stdout, map[string]string{"link": link}); err != nil {
				return fail(stderr, err)
			}
			return exitOK
		}
		fmt.Fprintln(stdout, link)
		return exitOK
	}

	res, code := applyBlockOp(svc, a, op, opArgs, stdout, stderr)
	if code != exitOK {
		return code
	}
	// A one-shot command never leaves an uncommitted change behind.
	if cerr := svc.Commit(); cerr != nil {
		fmt.Fprintf(stderr, "tlog: %v\n", cerr)
	}

	next := app.Addr{Rel: res.Rel, Offset: res.Offset, Hash: res.Hash}
	if *asJSON {
		if err := emit(stdout, map[string]any{
			"addr":   next,
			"rel":    res.Rel,
			"offset": res.Offset,
			"hash":   res.Hash,
		}); err != nil {
			return fail(stderr, err)
		}
		return exitOK
	}
	// The new address, so the next command can be written from this one.
	fmt.Fprintln(stdout, next.String())
	return exitOK
}

func applyBlockOp(svc *app.Service, a app.Addr, op string, args []string, stdout, stderr io.Writer) (*app.Result, int) {
	need := func(n int, form string) bool {
		if len(args) < n {
			fmt.Fprintf(stderr, "tlog block: %s needs %s\n", op, form)
			return false
		}
		return true
	}

	var res *app.Result
	var err error
	switch op {
	case "text":
		if !need(1, "the new text") {
			return nil, exitUsage
		}
		res, err = svc.SetText(a, strings.Join(args, " "))
	case "after":
		asChild := false
		if len(args) > 0 && (args[0] == "-child" || args[0] == "--child") {
			asChild, args = true, args[1:]
		}
		res, err = svc.InsertAfter(a, strings.Join(args, " "), asChild)
	case "before":
		res, err = svc.InsertBefore(a, strings.Join(args, " "))
	case "split":
		if !need(2, "the text before and the text after") {
			return nil, exitUsage
		}
		res, err = svc.SplitBlock(a, args[0], args[1], false)
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
	case "delete":
		res, err = svc.DeleteBlock(a)
	case "prop":
		if !need(1, "a property name") {
			return nil, exitUsage
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
