// Package cli is a thin adapter over the core. It parses arguments, calls
// app.Service and prints; it decides nothing on its own.
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tilman-schieber/tlog/internal/app"
	"github.com/tilman-schieber/tlog/internal/importer"
	"github.com/tilman-schieber/tlog/internal/tui"
)

// Version is set at build time.
var Version = "0.1.0-dev"

const usage = `tlog — a markdown-first knowledge outliner

usage:
  tlog                      open today's journal in the outliner
  tlog today                print the path of today's journal
  tlog open <page>          print the path of a page, creating it if needed
  tlog add [-p page] text   append a block to today's journal, or to a page
  tlog import [-from dir]   import a Logseq graph into the notes directory
  tlog push [-auto on|off]  push the notes now, or set pushing on every commit
  tlog version              print the version

global flags:
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
			return 0
		case rest[0] == "-v" || rest[0] == "--version":
			fmt.Fprintln(stdout, Version)
			return 0
		default:
			fmt.Fprintf(stderr, "tlog: unknown flag %s\n\n%s", rest[0], usage)
			return 2
		}
	}

	cmd := ""
	if len(rest) > 0 {
		cmd, rest = rest[0], rest[1:]
	}

	svc, err := app.New(dir)
	if err != nil {
		fmt.Fprintf(stderr, "tlog: %v\n", err)
		return 1
	}

	switch cmd {
	case "":
		return run(stderr, svc, func() error { return tui.Run(svc, "") })
	case "today":
		return run(stderr, svc, func() error {
			rel := svc.TodayRel()
			fmt.Fprintln(stdout, svc.Store.Abs(rel))
			return nil
		})
	case "open":
		if len(rest) == 0 {
			fmt.Fprintln(stderr, "tlog open: which page?")
			return 2
		}
		return run(stderr, svc, func() error {
			rel, err := svc.OpenPage(strings.Join(rest, " "))
			if err != nil {
				return err
			}
			fmt.Fprintln(stdout, svc.Store.Abs(rel))
			return nil
		})
	case "add":
		return cmdAdd(rest, stdout, stderr, svc)
	case "import":
		return cmdImport(rest, stdout, stderr, svc)
	case "push":
		return cmdPush(rest, stdout, stderr, svc)
	case "version":
		fmt.Fprintln(stdout, Version)
		return 0
	case "help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "tlog: unknown command %q\n\n%s", cmd, usage)
		return 2
	}
}

// run executes an operation and flushes the auto-commit, so that a one-shot
// command never leaves uncommitted changes behind.
func run(stderr io.Writer, svc *app.Service, fn func() error) int {
	err := fn()
	if cerr := svc.Commit(); cerr != nil && err == nil {
		fmt.Fprintf(stderr, "tlog: %v\n", cerr)
	}
	if err != nil {
		fmt.Fprintf(stderr, "tlog: %v\n", err)
		return 1
	}
	return 0
}

func cmdAdd(args []string, stdout, stderr io.Writer, svc *app.Service) int {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(stderr)
	page := fs.String("p", "", "append to this page instead of today's journal")
	fs.StringVar(page, "page", "", "append to this page instead of today's journal")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	text := strings.Join(fs.Args(), " ")
	if strings.TrimSpace(text) == "" {
		// No argument: read the block from stdin, so that piping works.
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(stderr, "tlog add: %v\n", err)
			return 1
		}
		text = strings.TrimRight(string(data), "\n")
	}
	if strings.TrimSpace(text) == "" {
		fmt.Fprintln(stderr, "tlog add: nothing to add")
		return 2
	}

	return run(stderr, svc, func() error {
		var rel string
		var err error
		if *page != "" {
			rel, err = svc.AddToPage(*page, text)
		} else {
			rel, err = svc.AddToday(text)
		}
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, svc.Store.Abs(rel))
		return nil
	})
}

func cmdPush(args []string, stdout, stderr io.Writer, svc *app.Service) int {
	fs := flag.NewFlagSet("push", flag.ContinueOnError)
	fs.SetOutput(stderr)
	auto := fs.String("auto", "", "turn pushing on every commit on or off")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *auto != "" {
		on := *auto == "on" || *auto == "true" || *auto == "yes"
		if !on && *auto != "off" && *auto != "false" && *auto != "no" {
			fmt.Fprintf(stderr, "tlog push: -auto takes on or off, not %q\n", *auto)
			return 2
		}
		if err := svc.SetAutoPush(on); err != nil {
			fmt.Fprintf(stderr, "tlog: %v\n", err)
			return 1
		}
		if on {
			fmt.Fprintf(stdout, "pushing on every commit, to %s\n", svc.Store.Root)
		} else {
			fmt.Fprintln(stdout, "pushing is now manual")
		}
		return 0
	}

	st := svc.Sync()
	if !st.Remote {
		fmt.Fprintf(stderr, "tlog: no remote; add one with `git -C %s remote add origin <url>`\n", svc.Store.Root)
		return 1
	}
	if err := svc.Commit(); err != nil {
		fmt.Fprintf(stderr, "tlog: %v\n", err)
		return 1
	}
	if err := svc.Push(); err != nil {
		fmt.Fprintf(stderr, "tlog: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, "pushed")
	return 0
}

func cmdImport(args []string, stdout, stderr io.Writer, svc *app.Service) int {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	from := fs.String("from", "", "Logseq graph directory (default ~/logseq)")
	dry := fs.Bool("dry-run", false, "report what would happen without writing")
	if err := fs.Parse(args); err != nil {
		return 2
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
