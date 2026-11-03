package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// What the audit in TODO.md asked for: that a script can do everything a
// person can, safely. Each test here is one line of that list.

type writeOut struct {
	Addr    string `json:"addr"`
	Rel     string `json:"rel"`
	Offset  int    `json:"offset"`
	Hash    string `json:"hash"`
	Warning string `json:"warning"`
}

// --- safety -------------------------------------------------------------------

func TestAnAddressWithoutAHashIsRefused(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir)
	addr := firstAddr(t, dir, "plan the lecture")
	bare := addr[:strings.LastIndex(addr, "@")+1] // "journals/X.md:0@"
	before := read(t, dir, "journals")

	code, _, errs := runCLI(t, dir, "block", bare, "text", "overwritten")
	if code != exitUsage || !strings.Contains(errs, "hash") {
		t.Fatalf("code=%d stderr=%q", code, errs)
	}
	if read(t, dir, "journals") != before {
		t.Fatal("a write without a hash landed")
	}
}

func TestAPathCannotLeaveTheNotesDirectory(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "notes")
	outside := filepath.Join(parent, "out.md")
	if err := os.WriteFile(outside, []byte("- secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"view", "../out.md"},
		{"view", outside},
		{"append", "../out.md", "x"},
		{"append", outside, "x"},
	} {
		code, out, _ := runCLI(t, dir, args...)
		if code == 0 || strings.Contains(out, "secret") {
			t.Errorf("%v: code=%d out=%q", args, code, out)
		}
	}
	if data, _ := os.ReadFile(outside); string(data) != "- secret\n" {
		t.Fatalf("the file outside was changed: %q", data)
	}
}

func TestFlagsAfterTheTextAreFlags(t *testing.T) {
	dir := t.TempDir()
	if code, out, errs := runCLI(t, dir, "add", "buy milk", "-todo", "-json"); code != 0 {
		t.Fatalf("%d %s", code, errs)
	} else if !strings.Contains(out, `"addr"`) {
		t.Fatalf("no JSON: %q", out)
	}
	if body := read(t, dir, "journals"); !strings.Contains(body, "- [ ] buy milk\n") || strings.Contains(body, "-todo") {
		t.Fatalf("got %q", body)
	}

	addr := firstAddr(t, dir, "buy milk")
	code, out, errs := runCLI(t, dir, "block", addr, "text", "new words", "-json")
	if code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	w := jsonOut[writeOut](t, out)
	if w.Addr == "" || w.Rel == "" || w.Hash == "" {
		t.Fatalf("incomplete: %+v", w)
	}
	if body := read(t, dir, "journals"); !strings.Contains(body, "new words\n") || strings.Contains(body, "-json") {
		t.Fatalf("got %q", body)
	}
}

// --- the commands that were missing --------------------------------------------

func TestSlashCommandsFromAScript(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir)
	addr := firstAddr(t, dir, "plan the lecture")

	next := func(args ...string) string {
		t.Helper()
		code, out, errs := runCLI(t, dir, append([]string{"block", addr}, args...)...)
		if code != 0 {
			t.Fatalf("block %v: %d %s", args, code, errs)
		}
		addr = strings.TrimSpace(out)
		return read(t, dir, "journals")
	}
	if body := next("done"); !strings.Contains(body, "- [x] plan the lecture") {
		t.Fatalf("done: %q", body)
	}
	if body := next("done"); !strings.Contains(body, "- [x] plan the lecture") {
		t.Fatalf("done twice must stay done: %q", body)
	}
	if body := next("open"); !strings.Contains(body, "- [ ] plan the lecture") {
		t.Fatalf("open: %q", body)
	}
	if body := next("deadline", "2026-11-12"); !strings.Contains(body, "Deadline:: 2026-11-12") {
		t.Fatalf("deadline: %q", body)
	}
	if body := next("quote"); !strings.Contains(body, "> ") {
		t.Fatalf("quote: %q", body)
	}
	if body := next("cmd", "prop", "status", "open"); !strings.Contains(body, "status:: open") {
		t.Fatalf("cmd prop: %q", body)
	}
	if body := next("cmd", "date", "2026-12-24"); !strings.Contains(body, "[[2026-12-24]]") {
		t.Fatalf("cmd date: %q", body)
	}

	// A date nobody can read is refused with its own code, and nothing is
	// written.
	before := read(t, dir, "journals")
	if code, _, _ := runCLI(t, dir, "block", addr, "deadline", "someday"); code != exitRefused {
		t.Fatalf("a bad date exited %d", code)
	}
	if read(t, dir, "journals") != before {
		t.Fatal("a refused deadline changed the file")
	}
	if code, _, _ := runCLI(t, dir, "block", addr, "cmd", "frobnicate"); code != exitRefused {
		t.Fatalf("an unknown command exited %d", code)
	}
}

func TestCommandsAreListed(t *testing.T) {
	dir := t.TempDir()
	code, out, _ := runCLI(t, dir, "commands", "-json")
	if code != 0 || !strings.Contains(out, `"name": "deadline"`) {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestAliasFromTheCommandLine(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir)
	if code, _, errs := runCLI(t, dir, "alias", "Timetable", "Stundenplan"); code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	if body := read(t, dir, "pages"); !strings.Contains(body, "aliases: Stundenplan") {
		t.Fatalf("got %q", body)
	}
	// The alias resolves the way a link would.
	if code, out, _ := runCLI(t, dir, "view", "Stundenplan"); code != 0 || !strings.Contains(out, "Timetable") {
		t.Fatalf("code=%d out=%q", code, out)
	}
	if code, _, _ := runCLI(t, dir, "alias", "Nowhere", "x"); code != exitRefused {
		t.Fatalf("aliasing a page that does not exist exited %d", code)
	}
}

func TestAddPrintsAnAddressAndTakesAPageForATask(t *testing.T) {
	dir := t.TempDir()
	code, out, errs := runCLI(t, dir, "add", "-json", "-p", "Projekt", "-todo", "-due", "2026-11-20", "write it up")
	if code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	w := jsonOut[writeOut](t, out)
	if !strings.HasSuffix(w.Rel, filepath.Join("pages", "Projekt.md")) || w.Addr == "" {
		t.Fatalf("got %+v", w)
	}
	body := read(t, dir, "pages")
	if !strings.Contains(body, "- [ ] write it up") || !strings.Contains(body, "Deadline:: 2026-11-20") {
		t.Fatalf("got %q", body)
	}
	// The address works.
	if code, _, errs := runCLI(t, dir, "block", w.Addr, "done"); code != 0 {
		t.Fatalf("the address from add did not work: %d %s", code, errs)
	}
}

func TestATaskHonoursTheDeadlinePropertySetting(t *testing.T) {
	t.Setenv("TLOG_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	dir := t.TempDir()
	if code, _, errs := runCLI(t, dir, "config", "deadline.property", "due"); code != 0 {
		t.Fatal(errs)
	}
	if code, _, errs := runCLI(t, dir, "add", "-todo", "-due", "2026-11-20", "x"); code != 0 {
		t.Fatal(errs)
	}
	if body := read(t, dir, "journals"); !strings.Contains(body, "due:: 2026-11-20") || strings.Contains(body, "Deadline::") {
		t.Fatalf("got %q", body)
	}
}

func TestDateSaysWhatAShorthandMeans(t *testing.T) {
	dir := t.TempDir()
	code, out, _ := runCLI(t, dir, "date", "2026-11-06", "-json")
	if code != 0 || !strings.Contains(out, `"iso": "2026-11-06"`) {
		t.Fatalf("code=%d out=%q", code, out)
	}
	if code, out, _ = runCLI(t, dir, "date"); code != 0 || !strings.Contains(out, "today") {
		t.Fatalf("with nothing typed: code=%d out=%q", code, out)
	}
	if code, _, _ = runCLI(t, dir, "date", "someday maybe"); code != exitRefused {
		t.Fatalf("nonsense exited %d", code)
	}
}

func TestPeopleAndMentions(t *testing.T) {
	dir := t.TempDir()
	if code, _, errs := runCLI(t, dir, "add", "met [[Julia Rausenberger #person]] about the grant"); code != 0 {
		t.Fatal(errs)
	}
	code, out, _ := runCLI(t, dir, "people", "-json")
	if code != 0 || !strings.Contains(out, "Julia Rausenberger") {
		t.Fatalf("code=%d out=%q", code, out)
	}
	code, out, _ = runCLI(t, dir, "mentions", "jr", "-json")
	if code != 0 || !strings.Contains(out, `"insert": "[[Julia Rausenberger]]"`) {
		t.Fatalf("code=%d out=%q", code, out)
	}
	// Nobody: an empty list, not null.
	code, out, _ = runCLI(t, t.TempDir(), "people", "-json")
	if code != 0 || strings.TrimSpace(out) != "[]" {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestLinksAreResolved(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir)
	target := firstAddr(t, dir, "the lecture is at nine")
	code, out, errs := runCLI(t, dir, "block", target, "ref")
	if code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	ref := strings.TrimSpace(out)
	if code, _, errs := runCLI(t, dir, "add", "see "+ref+", [[Timetable]], [[Nowhere]], https://go.dev/doc and [gone](../assets/gone.png)"); code != 0 {
		t.Fatal(errs)
	}
	addr := firstAddr(t, dir, "see")
	code, out, errs = runCLI(t, dir, "links", addr, "-json")
	if code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	type link struct {
		Kind, Target, Rel, Addr, Text, Resolved, Error string
		Exists                                         bool
	}
	links := jsonOut[[]link](t, out)
	byTarget := map[string]link{}
	for _, l := range links {
		byTarget[l.Target] = l
	}
	if l := byTarget["Timetable"]; l.Kind != "page" || !l.Exists || !strings.HasSuffix(l.Rel, "Timetable.md") {
		t.Errorf("page link: %+v", l)
	}
	if l := byTarget["Nowhere"]; l.Kind != "page" || l.Exists {
		t.Errorf("a link to no page: %+v", l)
	}
	blockLink := byTarget["Timetable"]
	for _, l := range links {
		if l.Kind == "block" {
			blockLink = l
		}
	}
	if blockLink.Kind != "block" || blockLink.Addr == "" || blockLink.Text != "the lecture is at nine" {
		t.Errorf("block reference: %+v", blockLink)
	}
	if l := byTarget["https://go.dev/doc"]; l.Kind != "url" || l.Resolved == "" || l.Error != "" {
		t.Errorf("web link: %+v", l)
	}
	if l := byTarget["../assets/gone.png"]; l.Error == "" || l.Resolved != "" {
		t.Errorf("a file that is not there was not refused: %+v", l)
	}
}

func TestLinksSayWhichFilesArePlainText(t *testing.T) {
	dir := t.TempDir()
	if code, _, errs := runCLI(t, dir, "add", "see [rules](../assets/rules.md) and ![plot](../assets/plot.png)"); code != 0 {
		t.Fatal(errs)
	}
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"rules.md": "# Regeln\n", "plot.png": "\x89PNG\x00"} {
		if err := os.WriteFile(filepath.Join(dir, "assets", name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	code, out, errs := runCLI(t, dir, "links", firstAddr(t, dir, "see"), "-json")
	if code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	type link struct {
		Kind, Target string
		Plain        bool
	}
	got := map[string]bool{}
	for _, l := range jsonOut[[]link](t, out) {
		if l.Kind != "file" {
			t.Errorf("not a file: %+v", l)
		}
		got[l.Target] = l.Plain
	}
	if !got["../assets/rules.md"] || got["../assets/plot.png"] {
		t.Fatalf("plain: %v", got)
	}
}

func TestUndoFromAScriptGoesThroughGit(t *testing.T) {
	dir := t.TempDir()
	if code, _, errs := runCLI(t, dir, "add", "first"); code != 0 {
		t.Fatal(errs)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Skipf("git not available: %v", err)
	}
	if code, _, errs := runCLI(t, dir, "add", "second"); code != 0 {
		t.Fatal(errs)
	}
	if body := read(t, dir, "journals"); !strings.Contains(body, "second") {
		t.Fatalf("got %q", body)
	}

	code, out, errs := runCLI(t, dir, "undo")
	if code != 0 {
		t.Fatalf("undo: %d %s", code, errs)
	}
	if body := read(t, dir, "journals"); strings.Contains(body, "second") || !strings.Contains(body, "first") {
		t.Fatalf("after undo: %q", body)
	}
	if !strings.Contains(out, "undone") {
		t.Fatalf("out=%q", out)
	}
	// The undo is itself committed: nothing is left dangling.
	if st, _ := gitOutput(dir, "status", "--porcelain"); strings.TrimSpace(st) != "" {
		t.Fatalf("uncommitted after undo:\n%s", st)
	}

	// A file changed by hand since the commit is not undone over.
	rel := "journals"
	entries, _ := os.ReadDir(filepath.Join(dir, rel))
	path := filepath.Join(dir, rel, entries[0].Name())
	if err := os.WriteFile(path, []byte("- first\n\n- by hand\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := runCLI(t, dir, "undo", filepath.Join(rel, entries[0].Name())); code != exitStale {
		t.Fatalf("undo over a hand edit exited %d", code)
	}
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), "by hand") {
		t.Fatal("the hand edit was overwritten")
	}
}

func TestSplitAsChildAppendAndStdin(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir)
	addr := firstAddr(t, dir, "plan the lecture")
	if code, _, errs := runCLI(t, dir, "block", addr, "split", "-child", "plan", "the lecture"); code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	if body := read(t, dir, "journals"); !strings.Contains(body, "- plan\n  - the lecture") {
		t.Fatalf("got %q", body)
	}

	old := stdin
	stdin = strings.NewReader("line one\nline two\n")
	defer func() { stdin = old }()
	code, out, errs := runCLI(t, dir, "add", "-p", "Timetable", "-json", "-")
	if code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	w := jsonOut[writeOut](t, out)
	if body := read(t, dir, "pages"); !strings.Contains(body, "- line one\n  line two\n") {
		t.Fatalf("got %q", body)
	}

	code, out, errs = runCLI(t, dir, "append", w.Rel, "-if-match", w.Hash, "the end", "-json")
	if code != 0 {
		t.Fatalf("append: %d %s", code, errs)
	}
	if body := read(t, dir, "pages"); !strings.HasSuffix(body, "- the end\n") {
		t.Fatalf("got %q", body)
	}
	// The hash it was given is stale now.
	if code, _, _ = runCLI(t, dir, "append", w.Rel, "-if-match", w.Hash, "again"); code != exitStale {
		t.Fatalf("a stale append exited %d", code)
	}
}

func TestReadOnlyGaps(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir)

	if code, out, _ := runCLI(t, dir, "sync", "-json"); code != 0 || !strings.Contains(out, `"remote": false`) {
		t.Fatalf("sync: code=%d out=%q", code, out)
	}
	if code, out, _ := runCLI(t, dir, "places", "time", "-json"); code != 0 || !strings.Contains(out, `"rel": "pages/Timetable.md"`) {
		t.Fatalf("places: code=%d out=%q", code, out)
	}
	if code, out, _ := runCLI(t, dir, "index", "-json"); code != 0 || !strings.Contains(out, `"kind": "journal"`) || !strings.Contains(out, `"rel":`) {
		t.Fatalf("index -json has no paths: code=%d out=%q", code, out)
	}
	if code, out, _ := runCLI(t, dir, "view", "-day", "today"); code != 0 || !strings.Contains(out, "plan the lecture") {
		t.Fatalf("view -day today: code=%d out=%q", code, out)
	}
	if code, out, _ := runCLI(t, dir, "view", "-day", "-1"); code != 0 || strings.Contains(out, "plan the lecture") {
		t.Fatalf("view -day -1 should be an empty day: code=%d out=%q", code, out)
	}
	if code, _, _ := runCLI(t, dir, "view", "-day", "someday"); code != exitRefused {
		t.Fatalf("a day nobody can read exited %d", code)
	}

	addr := firstAddr(t, dir, "plan the lecture")
	if code, _, errs := runCLI(t, dir, "block", addr, "prop", "status", "open"); code != 0 {
		t.Fatal(errs)
	}
	addr = firstAddr(t, dir, "plan the lecture")
	code, out, _ := runCLI(t, dir, "block", addr)
	if code != 0 || !strings.Contains(out, "plan the lecture") || !strings.Contains(out, "status:: open") {
		t.Fatalf("reading one block: code=%d out=%q", code, out)
	}
	if code, out, _ = runCLI(t, dir, "block", addr, "-json"); code != 0 || !strings.Contains(out, `"text": "plan the lecture"`) {
		t.Fatalf("code=%d out=%q", code, out)
	}
	// Plain view shows the property too.
	if code, out, _ = runCLI(t, dir, "view"); code != 0 || !strings.Contains(out, "status:: open") {
		t.Fatalf("plain view drops properties: code=%d out=%q", code, out)
	}
}

// --- exit codes and shapes -------------------------------------------------------

func TestTheWrongOffsetIsNotAFailure(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir)
	addr := firstAddr(t, dir, "plan the lecture")
	hash := addr[strings.LastIndex(addr, "@")+1:]
	rel := addr[:strings.LastIndex(addr, ":")]
	code, _, _ := runCLI(t, dir, "block", rel+":9999@"+hash, "task")
	if code != exitRefused {
		t.Fatalf("a right hash with a wrong offset exited %d, wanted %d", code, exitRefused)
	}
}

func TestAMissingPageIsTheSameAnswerByNameOrByPath(t *testing.T) {
	dir := t.TempDir()
	byName, _, _ := runCLI(t, dir, "view", "Missing")
	byPath, _, _ := runCLI(t, dir, "view", "pages/Missing.md")
	if byName != exitRefused || byPath != exitRefused {
		t.Fatalf("by name %d, by path %d", byName, byPath)
	}
}

func TestEmptyListsAreEmptyLists(t *testing.T) {
	dir := t.TempDir()
	// A page with nothing on it and nothing pointing at it: lists, not nulls.
	if code, out, _ := runCLI(t, dir, "view", "-json"); code != 0 || strings.Contains(out, "null") {
		t.Errorf("view -json of an empty day holds a null: code=%d out=%s", code, out)
	}
	for _, args := range [][]string{
		{"search", "nothing", "-json"}, {"due", "-json"}, {"todo", "-json"},
		{"places", "-json"}, {"index", "-json"}, {"people", "-json"}, {"files", "-json"},
	} {
		code, out, errs := runCLI(t, dir, args...)
		if code != 0 || strings.TrimSpace(out) != "[]" {
			t.Errorf("%v: code=%d out=%q err=%q", args, code, out, errs)
		}
	}
}

func TestSearchCarriesTheWholeBlock(t *testing.T) {
	dir := t.TempDir()
	old := stdin
	stdin = strings.NewReader("first line\nsecond line\n")
	defer func() { stdin = old }()
	if code, _, errs := runCLI(t, dir, "add", "-"); code != 0 {
		t.Fatal(errs)
	}
	code, out, _ := runCLI(t, dir, "search", "first", "-json")
	if code != 0 {
		t.Fatal(out)
	}
	hits := jsonOut[[]struct{ Text, FirstLine string }](t, out)
	if len(hits) != 1 || hits[0].Text != "first line\nsecond line" || hits[0].FirstLine != "first line" {
		t.Fatalf("got %+v", hits)
	}
}

func TestRefSaysWhereTheBlockIsAfterwards(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir)
	addr := firstAddr(t, dir, "the lecture is at nine")
	code, out, errs := runCLI(t, dir, "block", addr, "ref", "-json")
	if code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	res := jsonOut[struct {
		Link string
		writeOut
	}](t, out)
	if !strings.HasPrefix(res.Link, "[[Timetable#^") || res.Addr == "" || res.Addr == addr {
		t.Fatalf("got %+v", res)
	}
	if code, _, errs := runCLI(t, dir, "block", res.Addr, "task"); code != 0 {
		t.Fatalf("the address after ref did not work: %d %s", code, errs)
	}
}

func TestAfterWithNoTextIsAMistake(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir)
	addr := firstAddr(t, dir, "plan the lecture")
	before := read(t, dir, "journals")
	if code, _, _ := runCLI(t, dir, "block", addr, "after"); code != exitUsage {
		t.Fatalf("exited %d", code)
	}
	if read(t, dir, "journals") != before {
		t.Fatal("an empty block was inserted")
	}
}

func TestTheUsageNamesEveryCommand(t *testing.T) {
	dir := t.TempDir()
	_, out, _ := runCLI(t, dir, "help")
	for _, want := range []string{"attach -link-only", "-links", "due [-all]", "exit codes", "-dir", "block <addr>", "undo", "links", "date", "people", "mentions", "append", "sync", "places", "alias", "commands"} {
		if !strings.Contains(out, want) {
			t.Errorf("the usage does not mention %q", want)
		}
	}
	_, out, _ = runCLI(t, dir, "block", "help")
	if !strings.Contains(out, "open / done") || !strings.Contains(out, "done -> open") {
		t.Fatalf("block help: %q", out)
	}
}
