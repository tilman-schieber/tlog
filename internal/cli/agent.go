package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/tilman-schieber/tlog/internal/app"
	"github.com/tilman-schieber/tlog/internal/store"
)

// The surface a script or an agent works through. It is the same core the
// outliner and the window use — these commands hold no logic, and every one of
// them is a call into app.Service.
//
// Two things make it safe to hand to something that is not a person. Every
// block carries an address that includes the hash of the file it was read
// from, so a write from a stale read is refused rather than landing in the
// wrong place. And the exit code says which kind of no it was, so a caller
// knows whether re-reading would help.

// Exit codes. 0 and 1 are the usual; the other three exist so that a caller
// can tell "that move does not exist" from "you are behind" without matching
// on the text of a message.
const (
	exitOK      = 0
	exitError   = 1
	exitUsage   = 2
	exitRefused = 3 // the move does not exist — retrying will not help
	exitStale   = 4 // the file moved since the address was read — re-read
)

// fail maps an error to the exit code that describes it.
func fail(stderr io.Writer, err error) int {
	switch {
	case app.Refused(err):
		fmt.Fprintf(stderr, "tlog: %v\n", err)
		return exitRefused
	case app.Stale(err):
		fmt.Fprintf(stderr, "tlog: %v\n", err)
		return exitStale
	}
	var conflict *store.ErrConflict
	if ok := asConflict(err, &conflict); ok {
		fmt.Fprintf(stderr, "tlog: %v\n", err)
		return exitStale
	}
	fmt.Fprintf(stderr, "tlog: %v\n", err)
	return exitError
}

func asConflict(err error, target **store.ErrConflict) bool {
	c, ok := err.(*store.ErrConflict)
	if ok {
		*target = c
	}
	return ok
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

// emit writes a value as JSON. Indented, because the first reader of any of
// this is a person finding out what the shape is.
func emit(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// --- reading -----------------------------------------------------------------

func cmdSearch(args []string, stdout, stderr io.Writer, svc *app.Service) int {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.SetOutput(stderr)
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
		fmt.Fprintf(stdout, "%-28s %s\n", truncate(h.Page, 28), truncate(h.Text, 90))
	}
	return exitOK
}

func cmdIndex(args []string, stdout, stderr io.Writer, svc *app.Service) int {
	fs := flag.NewFlagSet("index", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "print as JSON")
	if err := parseFlags(fs, args); err != nil {
		return exitUsage
	}
	idx, err := svc.Index()
	if err != nil {
		return fail(stderr, err)
	}
	if *asJSON {
		if err := emit(stdout, idx); err != nil {
			return fail(stderr, err)
		}
		return exitOK
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

// cmdView prints a page. Unlike `tlog open` it creates nothing: reading is not
// a reason to write a file, and an agent surveying the notes must not leave
// empty pages behind everywhere it looked.
func cmdView(args []string, stdout, stderr io.Writer, svc *app.Service) int {
	fs := flag.NewFlagSet("view", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "print the whole page as JSON, addresses included")
	withAddr := fs.Bool("addr", false, "show each block's address")
	if err := parseFlags(fs, args); err != nil {
		return exitUsage
	}
	name := strings.Join(fs.Args(), " ")
	if strings.TrimSpace(name) == "" {
		name = svc.TodayRel()
	}

	rel, err := resolveForReading(svc, name)
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
		line := strings.Repeat("  ", b.Depth) + "- " + oneLine(b.Text)
		if *withAddr {
			line = b.Addr.String() + "  " + line
		}
		fmt.Fprintln(stdout, line)
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

// resolveForReading turns what was typed into a file, without creating one.
// A path is taken as a path; anything else is a page name, which may be an
// alias for a page actually called something else.
func resolveForReading(svc *app.Service, name string) (string, error) {
	if strings.HasSuffix(name, ".md") || strings.Contains(name, "/") {
		return name, nil
	}
	rel, exists, err := svc.ResolvePage(name)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("no page called %q — `tlog open %s` would create it", name, name)
	}
	return rel, nil
}

// oneLine flattens a block for a line-per-block listing.
func oneLine(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
}
