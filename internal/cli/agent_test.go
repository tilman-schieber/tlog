package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The surface a script drives. What is being tested is the contract an agent
// depends on: a read hands back an address, that address works as the argument
// to a write, and a write hands back the next one. And that when it says no,
// the exit code says which kind of no.

func jsonOut[T any](t *testing.T, out string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	return v
}

type hit struct {
	Addr string `json:"addr"`
	Page string `json:"page"`
	Text string `json:"text"`
}

func seed(t *testing.T, dir string) {
	t.Helper()
	if code, _, e := runCLI(t, dir, "add", "plan the lecture"); code != 0 {
		t.Fatal(e)
	}
	if code, _, e := runCLI(t, dir, "add", "-p", "Timetable", "the lecture is at nine"); code != 0 {
		t.Fatal(e)
	}
}

func firstAddr(t *testing.T, dir, query string) string {
	t.Helper()
	code, out, errs := runCLI(t, dir, "search", query, "-json")
	if code != 0 {
		t.Fatalf("search: %d %s", code, errs)
	}
	hits := jsonOut[[]hit](t, out)
	if len(hits) == 0 {
		t.Fatalf("nothing matched %q", query)
	}
	return hits[0].Addr
}

func TestSearchHandsBackAnAddressThatWorks(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir)

	addr := firstAddr(t, dir, "plan the lecture")
	if !strings.Contains(addr, ".md:") || !strings.Contains(addr, "@") {
		t.Fatalf("not an address: %q", addr)
	}

	code, out, errs := runCLI(t, dir, "block", addr, "task")
	if code != 0 {
		t.Fatalf("block task: %d %s", code, errs)
	}
	// The write hands back where the block now is, so the next call can be
	// written from this one without reading again.
	next := strings.TrimSpace(out)
	if next == addr {
		t.Fatal("the address did not move after a write that changed the file")
	}
	if code, _, errs := runCLI(t, dir, "block", next, "prop", "Deadline", "2026-10-01"); code != 0 {
		t.Fatalf("chaining onto the returned address failed: %d %s", code, errs)
	}

	body := read(t, dir, "journals")
	if !strings.Contains(body, "- [ ] plan the lecture") || !strings.Contains(body, "Deadline:: 2026-10-01") {
		t.Fatalf("got %q", body)
	}
}

// The reason this is safe to hand to something that is not a person.
func TestAWriteFromAStaleReadIsRefusedWithItsOwnExitCode(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir)
	addr := firstAddr(t, dir, "plan the lecture")

	if code, _, errs := runCLI(t, dir, "block", addr, "text", "first"); code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	code, _, errs := runCLI(t, dir, "block", addr, "text", "second")
	if code != exitStale {
		t.Fatalf("a stale write returned %d, wanted %d (%s)", code, exitStale, errs)
	}
	if body := read(t, dir, "journals"); !strings.Contains(body, "first") {
		t.Fatalf("the refused write landed anyway: %q", body)
	}
}

func TestAMoveThatDoesNotExistHasItsOwnExitCode(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir)
	addr := firstAddr(t, dir, "plan the lecture")

	// Outdenting a top-level block is not a failure, and not something a
	// caller should retry. Those are different answers and they need
	// different codes.
	code, _, _ := runCLI(t, dir, "block", addr, "outdent")
	if code != exitRefused {
		t.Fatalf("got %d, wanted %d", code, exitRefused)
	}
}

func TestFlagsWorkOnEitherSideOfTheQuery(t *testing.T) {
	// Go's flag package stops at the first non-flag argument, so `search
	// lecture -json` silently searched for "lecture -json" and printed
	// nothing. A surface meant for scripts cannot have that in it.
	dir := t.TempDir()
	seed(t, dir)

	_, before, _ := runCLI(t, dir, "search", "-json", "lecture")
	_, after, _ := runCLI(t, dir, "search", "lecture", "-json")
	if before != after {
		t.Fatalf("flag position changed the answer:\nbefore=%s\nafter=%s", before, after)
	}
	if len(jsonOut[[]hit](t, after)) != 2 {
		t.Fatalf("got %s", after)
	}
}

func TestAWordBeginningWithADashIsStillAQuery(t *testing.T) {
	dir := t.TempDir()
	if code, _, e := runCLI(t, dir, "add", "the -v flag"); code != 0 {
		t.Fatal(e)
	}
	code, out, errs := runCLI(t, dir, "search", "-json", "--", "-v")
	if code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	if len(jsonOut[[]hit](t, out)) != 1 {
		t.Fatalf("got %s", out)
	}
}

func TestViewCreatesNothing(t *testing.T) {
	// An agent surveying the notes must not leave an empty page everywhere it
	// looked. `tlog open` creates; `tlog view` does not.
	dir := t.TempDir()
	seed(t, dir)

	code, _, errs := runCLI(t, dir, "view", "Nonexistent")
	if code == 0 {
		t.Fatal("viewing a page that does not exist succeeded")
	}
	if !strings.Contains(errs, "tlog open") {
		t.Fatalf("the error should say how to create it: %q", errs)
	}
	if _, err := os.Stat(filepath.Join(dir, "pages", "Nonexistent.md")); !os.IsNotExist(err) {
		t.Fatal("viewing created the page")
	}
}

func TestViewCarriesAnAddressForEveryBlock(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir)

	code, out, errs := runCLI(t, dir, "view", "Timetable", "-json")
	if code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	var page struct {
		Title  string `json:"title"`
		Blocks []struct {
			Addr string `json:"addr"`
			Text string `json:"text"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal([]byte(out), &page); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if page.Title != "Timetable" || len(page.Blocks) != 1 {
		t.Fatalf("got %+v", page)
	}
	// Not just present — usable.
	if code, _, errs := runCLI(t, dir, "block", page.Blocks[0].Addr, "task"); code != 0 {
		t.Fatalf("an address from view did not work: %d %s", code, errs)
	}
}

func TestBlockRefFromAScript(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir)
	addr := firstAddr(t, dir, "the lecture is at nine")

	code, out, errs := runCLI(t, dir, "block", addr, "ref")
	if code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	link := strings.TrimSpace(out)
	if !strings.HasPrefix(link, "[[Timetable#^") {
		t.Fatalf("got %q", link)
	}
	if !strings.Contains(read(t, dir, "pages"), "^") {
		t.Fatal("no anchor was written")
	}
}

func TestEveryBlockOperationIsReachable(t *testing.T) {
	// The point of this surface is that nothing the core can do is missing
	// from it. If an operation is added and not wired up, this fails.
	dir := t.TempDir()
	for _, op := range [][]string{
		{"text", "changed"},
		{"after", "sibling"},
		{"after", "-child", "a child"},
		{"before", "above"},
		{"split", "left", "right"},
		{"indent"},
		{"task"},
		{"prop", "status", "open"},
		{"up"},
		{"down"},
		{"outdent"},
		{"merge"},
		{"delete"},
	} {
		sub := filepath.Join(dir, op[0]+strings.Join(op[1:], "-"))
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		if code, _, e := runCLI(t, sub, "add", "one"); code != 0 {
			t.Fatal(e)
		}
		if code, _, e := runCLI(t, sub, "add", "two"); code != 0 {
			t.Fatal(e)
		}
		addr := firstAddr(t, sub, "two")
		code, _, errs := runCLI(t, sub, append([]string{"block", addr}, op...)...)
		// A refusal still proves the operation reached the core and the core
		// answered — `down` on the last block and `outdent` on a top-level one
		// have nowhere to go in this fixture. A usage error is the failure:
		// that means the operation is not wired up at all.
		if code != exitOK && code != exitRefused {
			t.Errorf("block %v: exit %d %s", op, code, errs)
		}
	}
}

func TestAnUnknownOperationSaysSoRatherThanDoingSomething(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir)
	addr := firstAddr(t, dir, "plan the lecture")
	before := read(t, dir, "journals")

	code, _, errs := runCLI(t, dir, "block", addr, "frobnicate")
	if code != exitUsage {
		t.Fatalf("got %d", code)
	}
	if !strings.Contains(errs, "frobnicate") {
		t.Fatalf("unhelpful: %q", errs)
	}
	if read(t, dir, "journals") != before {
		t.Fatal("an unknown operation changed the file")
	}
}

func TestABadAddressIsAUsageErrorNotACrash(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir)
	for _, bad := range []string{"nonsense", "pages/X.md:notanumber@abc", "pages/X.md:1"} {
		code, _, errs := runCLI(t, dir, "block", bad, "task")
		if code != exitUsage {
			t.Errorf("%q gave exit %d (%s)", bad, code, errs)
		}
	}
}

// read returns the single markdown file under a subdirectory of the store.
func read(t *testing.T, dir, sub string) string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, sub))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			b, err := os.ReadFile(filepath.Join(dir, sub, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			return string(b)
		}
	}
	t.Fatalf("no markdown file in %s", sub)
	return ""
}
