package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tilman-schieber/tlog/internal/store"
)

func writeSrc(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, s *store.Store, rel string) string {
	t.Helper()
	f, err := s.Read(rel)
	if err != nil {
		t.Fatal(err)
	}
	return string(f.Data)
}

func setup(t *testing.T) (src string, dst *store.Store) {
	t.Helper()
	src = t.TempDir()
	d, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return src, d
}

func TestImportRenamesJournalsToISO(t *testing.T) {
	src, dst := setup(t)
	writeSrc(t, src, "journals/2026_09_15.md", "id:: 00000001-2026-0915-0000-000000000000\n\n- a note\n")

	rep, err := Run(dst, Options{Source: src})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Journals != 1 {
		t.Fatalf("journals: %d", rep.Journals)
	}
	got := read(t, dst, "journals/2026-09-15.md")
	if got != "- a note\n" {
		t.Fatalf("got %q", got)
	}
	if strings.Contains(got, "id::") {
		t.Fatal("page header id leaked into the file")
	}
}

func TestImportConvertsTaskMarkers(t *testing.T) {
	src, dst := setup(t)
	writeSrc(t, src, "journals/2026_09_16.md", strings.Join([]string{
		"- TODO buy milk",
		"- LATER think",
		"- DONE shipped",
		"- CANCELED dropped",
		"- TODOSHOP not a marker",
		"",
	}, "\n"))

	rep, err := Run(dst, Options{Source: src})
	if err != nil {
		t.Fatal(err)
	}
	got := read(t, dst, "journals/2026-09-16.md")
	for _, want := range []string{"- [ ] buy milk", "- [ ] think", "- [x] shipped", "- [x] dropped", "- TODOSHOP not a marker"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
	if rep.Tasks != 4 {
		t.Fatalf("tasks converted: %d, want 4", rep.Tasks)
	}
}

func TestImportRewiresBlockReferences(t *testing.T) {
	src, dst := setup(t)
	writeSrc(t, src, "pages/Target.md", "- the referenced thought\n  id:: aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee\n")
	writeSrc(t, src, "journals/2026_09_17.md", "- see [[aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee]] and ((aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee))\n- dangling [[11111111-2222-3333-4444-555555555555]]\n")

	rep, err := Run(dst, Options{Source: src})
	if err != nil {
		t.Fatal(err)
	}
	target := read(t, dst, "pages/Target.md")
	if !strings.Contains(target, "^") {
		t.Fatalf("no anchor materialised:\n%s", target)
	}
	if strings.Contains(target, "id::") {
		t.Fatalf("logseq id survived:\n%s", target)
	}
	anchor := strings.TrimSpace(strings.SplitN(target, "^", 2)[1])

	journal := read(t, dst, "journals/2026-09-17.md")
	want := "[[Target#^" + anchor + "]]"
	if strings.Count(journal, want) != 2 {
		t.Fatalf("expected both reference forms rewired to %q:\n%s", want, journal)
	}
	if !strings.Contains(journal, "[[11111111-2222-3333-4444-555555555555]]") {
		t.Fatal("dangling reference should be left exactly as written")
	}
	if rep.RefsRewired != 2 || rep.RefsDangling != 1 {
		t.Fatalf("report: rewired=%d dangling=%d", rep.RefsRewired, rep.RefsDangling)
	}
}

func TestImportNormalisesPropertiesAndDates(t *testing.T) {
	src, dst := setup(t)
	writeSrc(t, src, "journals/2026_09_15.md", strings.Join([]string{
		"- TODO Stunden vormerken",
		"  * Deadline:: Sep 16th, 2026 00:00",
		"  collapsed:: true",
		"  logseq.order-list-type:: number",
		"",
	}, "\n"))

	if _, err := Run(dst, Options{Source: src}); err != nil {
		t.Fatal(err)
	}
	got := read(t, dst, "journals/2026-09-15.md")
	if !strings.Contains(got, "Deadline:: 2026-09-16") {
		t.Fatalf("date not normalised:\n%s", got)
	}
	if strings.Contains(got, "collapsed") || strings.Contains(got, "logseq.") {
		t.Fatalf("UI state survived the import:\n%s", got)
	}
	if strings.Contains(got, "* Deadline") {
		t.Fatalf("starred property not unwrapped:\n%s", got)
	}
}

func TestImportKeepsNestingAndWikilinks(t *testing.T) {
	src, dst := setup(t)
	writeSrc(t, src, "journals/2026_07_15.md", strings.Join([]string{
		"- [[Grace Hopper]]",
		"\t- Projekt 1.9.26 - 31.8.2027",
		"\t\t- Digital Competencies für [[Difference Engine]]",
		"",
	}, "\n"))

	if _, err := Run(dst, Options{Source: src}); err != nil {
		t.Fatal(err)
	}
	want := "- [[Grace Hopper]]\n  - Projekt 1.9.26 - 31.8.2027\n    - Digital Competencies für [[Difference Engine]]\n"
	if got := read(t, dst, "journals/2026-07-15.md"); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestImportDryRunWritesNothing(t *testing.T) {
	src, dst := setup(t)
	writeSrc(t, src, "journals/2026_09_15.md", "- a\n")

	rep, err := Run(dst, Options{Source: src, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Written) != 1 {
		t.Fatalf("dry run should still report what it would write: %+v", rep.Written)
	}
	if got := read(t, dst, "journals/2026-09-15.md"); got != "" {
		t.Fatalf("dry run wrote %q", got)
	}
}

func TestImportFindsMirrorUnderGraphRoot(t *testing.T) {
	src, dst := setup(t)
	writeSrc(t, src, "graphs/ts/mirror/markdown/journals/2026_09_15.md", "- found me\n")

	if _, err := Run(dst, Options{Source: src}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dst, "journals/2026-09-15.md"); !strings.Contains(got, "found me") {
		t.Fatalf("mirror not located: %q", got)
	}
}

func TestImportSkipsUnparseableNamesWithoutFailing(t *testing.T) {
	src, dst := setup(t)
	writeSrc(t, src, "journals/not-a-date.md", "- x\n")
	writeSrc(t, src, "journals/2026_09_15.md", "- ok\n")

	rep, err := Run(dst, Options{Source: src})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Skipped) != 1 {
		t.Fatalf("skipped: %+v", rep.Skipped)
	}
	if got := read(t, dst, "journals/2026-09-15.md"); !strings.Contains(got, "ok") {
		t.Fatal("a bad filename stopped the rest of the import")
	}
}

func TestImportRefusesToWriteIntoTheSourceGraph(t *testing.T) {
	src := t.TempDir()
	writeSrc(t, src, "journals/2026_09_15.md", "- a\n")

	// Destination is the graph itself.
	same, err := store.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Run(same, Options{Source: src}); err == nil || !strings.Contains(err.Error(), "into itself") {
		t.Fatalf("expected a refusal, got %v", err)
	}

	// Destination sits inside the graph.
	inner, err := store.Open(filepath.Join(src, "inner"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Run(inner, Options{Source: src}); err == nil || !strings.Contains(err.Error(), "inside the Logseq graph") {
		t.Fatalf("expected a refusal, got %v", err)
	}

	// The graph sits inside the destination.
	outer, err := store.Open(filepath.Dir(src))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Run(outer, Options{Source: src}); err == nil || !strings.Contains(err.Error(), "is inside the destination") {
		t.Fatalf("expected a refusal, got %v", err)
	}

	// The source graph must be unchanged by all of that.
	data, err := os.ReadFile(filepath.Join(src, "journals", "2026_09_15.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "- a\n" {
		t.Fatalf("the source graph was modified: %q", data)
	}
}
