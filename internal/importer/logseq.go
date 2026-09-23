// Package importer converts a Logseq graph into a tlog notes directory.
//
// The Logseq graph is strictly read-only here: this package reads it and writes
// somewhere else. Nothing in tlog can modify, move or delete Logseq data, and
// importing as often as you like is harmless.
//
// The conversion itself is one-way, in the sense that there is no converter
// back: Logseq-only syntax is translated to the nearest thing standard markdown
// already has, so that the result renders correctly in GitHub, glow and
// render-markdown.nvim, and so that an agent reading the files needs no
// knowledge of anyone's dialect. What survives untranslated is block
// properties, because markdown has no equivalent.
package importer

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/tilman-schieber/tlog/internal/markdown"
	"github.com/tilman-schieber/tlog/internal/store"
)

var (
	uuidRe = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

	// Logseq writes some block properties as a bulleted child line.
	starPropRe = regexp.MustCompile(`(?m)^(\s*)\*[ \t]+([A-Za-z][A-Za-z0-9_.\-]*)::`)

	// A property line at column zero at the top of a file is Logseq's page
	// header, not content.
	headerPropRe = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_.\-]*)::[ \t]*(.*)$`)

	openMarkers = []string{"TODO", "LATER", "NOW", "DOING", "IN-PROGRESS", "WAITING", "WAIT"}
	doneMarkers = []string{"DONE", "CANCELED", "CANCELLED"}

	// Logseq's human date format, e.g. "Sep 16th, 2026 00:00".
	logseqDateRe = regexp.MustCompile(`^([A-Z][a-z]{2}) (\d{1,2})(?:st|nd|rd|th), (\d{4})(?: \d{2}:\d{2})?$`)

	journalNameRe = regexp.MustCompile(`^(\d{4})[_-](\d{2})[_-](\d{2})$`)

	// Properties that are Logseq UI state or DB bookkeeping, not user content.
	dropProps = map[string]bool{
		"id":               true,
		"collapsed":        true,
		"heading":          true,
		"background-color": true,
	}
)

// Report describes what an import did, or would do.
type Report struct {
	Files        int
	Journals     int
	Pages        int
	Blocks       int
	Tasks        int
	Anchors      int
	RefsRewired  int
	RefsDangling int
	Empty        int
	Tagged       int
	Skipped      []string
	Written      []string
}

// String renders a human summary.
func (r *Report) String() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d files (%d journals, %d pages), %d blocks\n", r.Files, r.Journals, r.Pages, r.Blocks)
	fmt.Fprintf(&sb, "%d task markers converted to GFM checkboxes\n", r.Tasks)
	fmt.Fprintf(&sb, "%d block ids converted to ^anchors, %d references rewired\n", r.Anchors, r.RefsRewired)
	if r.Tagged > 0 {
		fmt.Fprintf(&sb, "%d pages carried their Logseq tags across\n", r.Tagged)
	}
	if r.Empty > 0 {
		fmt.Fprintf(&sb, "%d files held nothing but a Logseq page id and were not created\n", r.Empty)
	}
	if r.RefsDangling > 0 {
		fmt.Fprintf(&sb, "%d references pointed at blocks that no longer exist and were left as written\n", r.RefsDangling)
	}
	for _, s := range r.Skipped {
		fmt.Fprintf(&sb, "skipped: %s\n", s)
	}
	return sb.String()
}

// Options configure an import.
type Options struct {
	Source string // a Logseq graph directory, or its markdown mirror
	DryRun bool
}

type pending struct {
	rel string
	doc *markdown.Document
}

// Run imports a Logseq graph into the store.
func Run(dst *store.Store, opt Options) (*Report, error) {
	src, err := locate(opt.Source)
	if err != nil {
		return nil, err
	}
	// The importer only ever reads the source, but a mistyped -dir could still
	// aim the destination at the Logseq graph. Refuse rather than write a
	// single byte into someone's other tool's data.
	if err := refuseOverlap(src, dst.Root); err != nil {
		return nil, err
	}

	rep := &Report{}
	var docs []*pending

	// The mirror carries no page tags, so the typing lives only in Logseq's
	// database. Without this, every person, project and topic you classified
	// arrives untyped and the loss is invisible.
	classes, cerr := classesFor(src)
	if cerr != nil {
		rep.Skipped = append(rep.Skipped, "page tags: "+cerr.Error())
	}

	// uuid -> "Page#^anchor", built while parsing so that references can be
	// rewired once every block that might be referenced has an anchor.
	targets := map[string]markdown.Link{}

	for _, dir := range []string{store.JournalsDir, store.PagesDir} {
		entries, err := os.ReadDir(filepath.Join(src, dir))
		if err != nil {
			continue
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)

		for _, name := range names {
			raw, err := os.ReadFile(filepath.Join(src, dir, name))
			if err != nil {
				rep.Skipped = append(rep.Skipped, fmt.Sprintf("%s/%s: %v", dir, name, err))
				continue
			}
			rel, title, err := destPath(dir, name)
			if err != nil {
				rep.Skipped = append(rep.Skipped, fmt.Sprintf("%s/%s: %v", dir, name, err))
				continue
			}

			body, front, pageID := splitHeader(raw)
			doc := markdown.Parse(body)
			doc.Frontmatter = front

			// A [[uuid]] can point at a page as well as at a block, so page ids
			// are resolvable targets too.
			if pageID != "" {
				targets[strings.ToLower(pageID)] = markdown.Link{Page: title}
			}

			convert(doc, title, targets, rep)

			rep.Files++
			if dir == store.JournalsDir {
				rep.Journals++
			} else {
				rep.Pages++
			}
			if tags := classes[title]; len(tags) > 0 {
				doc.SetFrontmatter("tags", strings.Join(tags, ", "))
				rep.Tagged++
				delete(classes, title)
			}
			docs = append(docs, &pending{rel: rel, doc: doc})
		}
	}

	// A page can be typed in Logseq without ever having had a file of its own.
	// It still carries meaning, so it gets one here.
	for title, tags := range classes {
		rel, _, err := destPath(store.PagesDir, title+".md")
		if err != nil {
			continue
		}
		doc := &markdown.Document{}
		doc.SetFrontmatter("tags", strings.Join(tags, ", "))
		rep.Tagged++
		docs = append(docs, &pending{rel: rel, doc: doc})
	}

	for _, p := range docs {
		rewireRefs(p.doc, targets, rep)
	}

	for _, p := range docs {
		out := markdown.Render(p.doc)
		if len(out) == 0 {
			rep.Empty++
			continue
		}
		rep.Written = append(rep.Written, p.rel)
		if opt.DryRun {
			continue
		}
		if err := dst.Write(p.rel, out, ""); err != nil {
			return rep, fmt.Errorf("writing %s: %w", p.rel, err)
		}
	}
	return rep, nil
}

// locate accepts a graph root, a mirror directory or the markdown directory
// itself, so that the command works with whichever path is to hand.
func locate(src string) (string, error) {
	if src == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		src = filepath.Join(home, "logseq")
	}
	candidates := []string{
		src,
		filepath.Join(src, "mirror", "markdown"),
	}
	if entries, err := os.ReadDir(filepath.Join(src, "graphs")); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				candidates = append(candidates,
					filepath.Join(src, "graphs", e.Name(), "mirror", "markdown"),
					filepath.Join(src, "graphs", e.Name()))
			}
		}
	}
	for _, c := range candidates {
		if hasGraph(c) {
			return c, nil
		}
	}
	return "", fmt.Errorf("no journals/ or pages/ directory found under %s", src)
}

func hasGraph(dir string) bool {
	for _, d := range []string{store.JournalsDir, store.PagesDir} {
		if st, err := os.Stat(filepath.Join(dir, d)); err == nil && st.IsDir() {
			return true
		}
	}
	return false
}

// destPath maps a Logseq filename to its tlog equivalent, renaming journals to
// ISO dates along the way.
func destPath(dir, name string) (rel, title string, err error) {
	base := strings.TrimSuffix(name, ".md")
	if dir == store.JournalsDir {
		m := journalNameRe.FindStringSubmatch(base)
		if m == nil {
			return "", "", fmt.Errorf("unrecognised journal filename")
		}
		iso := fmt.Sprintf("%s-%s-%s", m[1], m[2], m[3])
		if _, perr := time.Parse(store.JournalDate, iso); perr != nil {
			return "", "", fmt.Errorf("unparseable journal date %q", base)
		}
		return filepath.Join(store.JournalsDir, iso+".md"), iso, nil
	}
	// Logseq URL-encodes characters that are awkward in filenames.
	clean := strings.ReplaceAll(base, "%2F", "-")
	clean = strings.ReplaceAll(clean, "%3A", "-")
	clean = strings.ReplaceAll(clean, "/", "-")
	if strings.TrimSpace(clean) == "" {
		return "", "", fmt.Errorf("empty page name")
	}
	return filepath.Join(store.PagesDir, clean+".md"), clean, nil
}

// splitHeader lifts Logseq's page-header properties off the top of a file and
// returns them as frontmatter, dropping the ones that are DB bookkeeping.
func splitHeader(raw []byte) ([]byte, []markdown.Prop, string) {
	lines := strings.Split(string(raw), "\n")
	var front []markdown.Prop
	var pageID string
	i := 0
	for ; i < len(lines); i++ {
		l := lines[i]
		if strings.TrimSpace(l) == "" {
			continue
		}
		m := headerPropRe.FindStringSubmatch(l)
		if m == nil {
			break
		}
		if strings.EqualFold(m[1], "id") {
			pageID = strings.TrimSpace(m[2])
		} else if !dropProps[strings.ToLower(m[1])] && !strings.HasPrefix(m[1], "logseq.") {
			front = append(front, markdown.Prop{Key: m[1], Value: normaliseValue(strings.TrimSpace(m[2]))})
		}
	}
	body := strings.Join(lines[i:], "\n")
	body = starPropRe.ReplaceAllString(body, "$1$2::")
	return []byte(body), front, pageID
}

// convert applies the per-block transformations and registers anchors for
// blocks that other blocks refer to by uuid.
func convert(doc *markdown.Document, page string, targets map[string]markdown.Link, rep *Report) {
	doc.Walk(func(b *markdown.Block) bool {
		rep.Blocks++

		if uuid, ok := b.Prop("id"); ok {
			anchor := doc.EnsureAnchor(b)
			targets[strings.ToLower(uuid)] = markdown.Link{Page: page, Anchor: anchor}
			rep.Anchors++
		}

		var kept []markdown.Prop
		for _, p := range b.Props {
			if dropProps[strings.ToLower(p.Key)] || strings.HasPrefix(p.Key, "logseq.") {
				continue
			}
			kept = append(kept, markdown.Prop{Key: p.Key, Value: normaliseValue(p.Value)})
		}
		b.Props = kept

		if convertMarker(b) {
			rep.Tasks++
		}
		return true
	})
}

// convertMarker turns a Logseq leading keyword into a GFM checkbox.
func convertMarker(b *markdown.Block) bool {
	for _, m := range openMarkers {
		if rest, ok := cutMarker(b.Text, m); ok {
			b.Text = "[ ] " + rest
			return true
		}
	}
	for _, m := range doneMarkers {
		if rest, ok := cutMarker(b.Text, m); ok {
			b.Text = "[x] " + rest
			return true
		}
	}
	return false
}

func cutMarker(text, marker string) (string, bool) {
	if !strings.HasPrefix(text, marker) {
		return "", false
	}
	rest := text[len(marker):]
	if rest == "" {
		return "", true
	}
	if rest[0] != ' ' && rest[0] != '\t' && rest[0] != '\n' {
		return "", false
	}
	return strings.TrimLeft(rest, " \t"), true
}

// rewireRefs turns [[uuid]] and ((uuid)) references into [[Page#^anchor]].
func rewireRefs(doc *markdown.Document, targets map[string]markdown.Link, rep *Report) {
	doc.Walk(func(b *markdown.Block) bool {
		b.Text = replaceRefs(b.Text, targets, rep)
		for i := range b.Props {
			b.Props[i].Value = replaceRefs(b.Props[i].Value, targets, rep)
		}
		return true
	})
}

var bracketRefRe = regexp.MustCompile(`\[\[\s*(` + uuidRe.String() + `)\s*\]\]|\(\(\s*(` + uuidRe.String() + `)\s*\)\)`)

func replaceRefs(text string, targets map[string]markdown.Link, rep *Report) string {
	return bracketRefRe.ReplaceAllStringFunc(text, func(m string) string {
		sub := bracketRefRe.FindStringSubmatch(m)
		uuid := sub[1]
		if uuid == "" {
			uuid = sub[2]
		}
		if link, ok := targets[strings.ToLower(uuid)]; ok {
			rep.RefsRewired++
			return link.String()
		}
		rep.RefsDangling++
		return m
	})
}

// normaliseValue rewrites Logseq's human dates to ISO so that property values
// sort and compare.
func normaliseValue(v string) string {
	m := logseqDateRe.FindStringSubmatch(v)
	if m == nil {
		return v
	}
	t, err := time.Parse("Jan 2 2006", fmt.Sprintf("%s %s %s", m[1], m[2], m[3]))
	if err != nil {
		return v
	}
	return t.Format(store.JournalDate)
}

// refuseOverlap rejects an import whose destination is the source, or sits
// inside it, or contains it. Importing is the only operation that reads one
// notes tree and writes another, so it is the only place the two can be
// confused — and the cost of getting it wrong is someone else's data.
func refuseOverlap(src, dst string) error {
	a, err := resolve(src)
	if err != nil {
		return err
	}
	b, err := resolve(dst)
	if err != nil {
		return err
	}
	if a == b {
		return fmt.Errorf("refusing to import %s into itself", src)
	}
	if isInside(b, a) {
		return fmt.Errorf("refusing to import: the destination %s is inside the Logseq graph at %s", dst, src)
	}
	if isInside(a, b) {
		return fmt.Errorf("refusing to import: the Logseq graph at %s is inside the destination %s", src, dst)
	}
	return nil
}

func resolve(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real, nil
	}
	return abs, nil
}

// isInside reports whether child is at or below parent.
func isInside(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
