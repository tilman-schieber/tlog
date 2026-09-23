package importer

import (
	"strings"
	"testing"
)

// datom builds one transit-encoded datom vector.
func datom(e int, attr, val string, tx int) string {
	return "[" + itoa(e) + "," + attr + "," + val + "," + itoa(tx) + "]"
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

func TestTransitCacheIsReplayed(t *testing.T) {
	// The first use of an attribute spells it out; later uses are "^N" back
	// references, and the index depends on every cacheable string seen so far.
	// Getting that wrong makes attribute names unreadable, so it is the one
	// thing worth testing directly.
	doc := "[" + strings.Join([]string{
		datom(1, `"~:block/title"`, `"Person"`, 5),
		datom(1, `"~:block/name"`, `"person"`, 5),
		datom(2, `"^0"`, `"Ada Lovelace"`, 6), // ^0 -> ~:block/title
		datom(2, `"^1"`, `"ada lovelace"`, 6), // ^1 -> ~:block/name
		datom(2, `"~:block/tags"`, `1`, 6),
	}, ",") + "]"

	got := parseDatoms([]byte(doc))
	if len(got) != 1 || len(got["Ada Lovelace"]) != 1 || got["Ada Lovelace"][0] != "Person" {
		t.Fatalf("cache not replayed correctly: %+v", got)
	}
}

func TestBuiltInClassesAreNotCarriedOver(t *testing.T) {
	// Logseq's own classes describe machinery. Tagging every page with "Page"
	// would be noise, and the ident is what distinguishes them.
	doc := "[" + strings.Join([]string{
		datom(1, `"~:block/title"`, `"Page"`, 5),
		datom(1, `"~:db/ident"`, `"~:logseq.class/Page"`, 5),
		datom(3, `"~:block/title"`, `"Person"`, 5),
		datom(2, `"~:block/title"`, `"Ada Lovelace"`, 6),
		datom(2, `"~:block/name"`, `"ada lovelace"`, 6),
		datom(2, `"~:block/tags"`, `1`, 6),
		datom(2, `"~:block/tags"`, `3`, 6),
	}, ",") + "]"

	got := parseDatoms([]byte(doc))
	if len(got["Ada Lovelace"]) != 1 || got["Ada Lovelace"][0] != "Person" {
		t.Fatalf("built-in class leaked through: %+v", got)
	}
}

func TestOnlyPagesAreTagged(t *testing.T) {
	// An ordinary block can carry a class too (a task, a quote). Those are not
	// pages and have no file to describe.
	doc := "[" + strings.Join([]string{
		datom(1, `"~:block/title"`, `"Task"`, 5),
		datom(2, `"~:block/title"`, `"buy milk"`, 6),
		datom(2, `"~:block/tags"`, `1`, 6),
	}, ",") + "]"

	if got := parseDatoms([]byte(doc)); len(got) != 0 {
		t.Fatalf("a block was treated as a page: %+v", got)
	}
}

func TestDuplicateTitlesCollapse(t *testing.T) {
	// Logseq can hold two entities with the same title; a page name is a
	// filename here, so there is exactly one page.
	doc := "[" + strings.Join([]string{
		datom(1, `"~:block/title"`, `"Person"`, 5),
		datom(2, `"~:block/title"`, `"Barbara Liskov"`, 6),
		datom(2, `"~:block/name"`, `"barbara liskov"`, 6),
		datom(2, `"~:block/tags"`, `1`, 6),
		datom(3, `"~:block/title"`, `"Barbara Liskov"`, 7),
		datom(3, `"~:block/name"`, `"barbara liskov"`, 7),
		datom(3, `"~:block/tags"`, `1`, 7),
	}, ",") + "]"

	got := parseDatoms([]byte(doc))
	if len(got) != 1 || len(got["Barbara Liskov"]) != 1 {
		t.Fatalf("duplicates not collapsed: %+v", got)
	}
}

func TestGarbageIsSurvived(t *testing.T) {
	// It is Logseq's private format and may change. Anything unreadable must
	// leave the import working without tags, never fail it.
	for _, in := range []string{"", "not json", "[1,2,3]", `[[1,"~:block/tags","nope",2]]`, "null"} {
		if got := parseDatoms([]byte(in)); len(got) != 0 {
			t.Fatalf("%q produced %+v", in, got)
		}
	}
}

func TestCacheIndexDecoding(t *testing.T) {
	for in, want := range map[string]int{"^0": 0, "^1": 1, "^Z": 42, "^10": 44, "^11": 45} {
		got, ok := cacheIndex(in)
		if !ok || got != want {
			t.Fatalf("%q -> %d (%v), want %d", in, got, ok, want)
		}
	}
	if _, ok := cacheIndex("^"); ok {
		t.Fatal("a bare caret is not an index")
	}
}

func TestImportWithoutADatabaseStillWorks(t *testing.T) {
	src, dst := setup(t)
	writeSrc(t, src, "journals/2026_09_15.md", "- a note\n")

	rep, err := Run(dst, Options{Source: src})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Tagged != 0 {
		t.Fatalf("tagged without a database: %d", rep.Tagged)
	}
	if got := read(t, dst, "journals/2026-09-15.md"); got != "- a note\n" {
		t.Fatalf("got %q", got)
	}
}
