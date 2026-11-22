# The outliner: an assessment, 2026-11-22

How this was done: read all of `internal/tui` (about 10,000 lines, 164 tests),
ran the test suite with coverage, drove the built binary in tmux against a
scratch notes directory (journal, pages, tags, a task with a deadline, nested
blocks, code, tables, quotes, an outside edit mid-typing, a 50-column window),
and timed rendering headlessly on generated pages of 400, 2,000 and 6,000 rows.
Every finding marked **(seen)** was reproduced, not just read in the code.

## Verdict

The outliner is in good shape for daily notes on pages of normal size. The
layering holds: every write goes through `app.Service`, compare-and-swap is
used everywhere, nothing reading writes, and the headless test harness is a
real strength (83.8 % coverage, tests assert on disk). Help, footer hints and
messages are clear and consistent in tone.

The weak spots are about **getting around and recovering**, not about storage:
leaving an overlay forgets which mode you were in, so letters typed next land
in the note; "go to" cannot reliably reach a page that is only linked or
create a new one; a conflict leaves you with no way to keep what you typed;
and rendering cost grows linearly with page size on every keystroke and
every mouse move.

## What works well

- **Safety model.** CAS writes, saving on leaving a block, refusing a stale
  undo, never reloading over unsaved typing (`watch.go`). In the conflict test
  nothing on disk was overwritten.
- **Two modes, the right way round.** Opening ready to type, and `esc` for
  single-letter navigation, works well. The `MOVE` / `STALE` header markers
  are easy to read.
- **Typing comfort.** Arrows run from one block into the next, wrapped lines
  move by screen line, `[[` closes itself, `@` completion ranks by recency,
  undo goes a word at a time before it undoes writes. Wide runes are measured
  with `ansi.StringWidth`.
- **The graph on the page.** Linked references with their children, tag
  listings and embedded block references below the outline. This is the
  product's best idea and it is visible without a keypress.
- **The help screen** explains concepts as well as keys, and scrolls.
- **Tests.** Driving `Model.Update` with `tea.KeyMsg` and checking files is
  exactly right. Adding a regression test for each finding below is cheap.

## Bugs and defects

### 1. Leaving any overlay drops you into writing mode (seen, twice)

Agenda, help, messages, settings, the file viewer and every picker set
`m.mode = modeOutline` on `esc` (`agenda.go:62`, `help.go:218`, `log.go:44`,
`settings.go:55`, `file.go:115`, `tui.go:1586`). Starting from MOVE, `p` then
`esc` puts you in writing, and the next letter you type as a command goes into
the block. In testing, `a` → `esc` → `p` typed a "p" into the journal, and
`?` → `esc` → `t` typed a "t" into a quote, which was then saved. Undo
recovers it, but this is the one failure the two-mode design exists to
prevent.

**Fix:** remember the mode an overlay was opened from and return to it
(`m.returnTo` set in one `openOverlay` helper). Even better, keep a small
overlay stack (see Code health). Add a test: from MOVE, open each overlay,
`esc`, type `x`, and assert the file is unchanged.

### 2. "Go to" cannot reach pages that are only linked, and rarely creates new ones (seen)

- `Lookup.Places` lists files, so a page that exists only as `[[Ada
  Lovelace]]` is missing from the palette. Tags are skipped explicitly
  (`tui.go`, "a tag has no file to go to from here").
- A new page is made only when **nothing** matches (`pickerKey`, `!ok`). The
  fuzzy match also searches the `detail` text and command names, so almost any
  short query matches something. Typing `ada` and pressing enter opened the
  **Agenda** (subsequence a‑d‑a in "Agenda").

**Fix:** have the core offer linked-but-unwritten pages and tag pages as
places (their kind is already known to the graph). Always add a final
"New page ‘<query>’" row when no row's label matches exactly, so enter on an
exact name never runs a command. Rank commands below pages unless the query
matches the command's name at the start.

### 3. A conflict while typing has no way out that keeps the typing (seen)

With text typed and the file changed by another writer, the header shows
`STALE` and the status says *"esc saves, ctrl+r discards"*. `esc` does not
save: the write is refused, and the footer then says *"ctrl+r reloads it
(your edit is not saved)"*. The only way forward is `ctrl+r` twice, which
throws the words away. This is safe, but the user loses their work and the
first message is wrong.

**Fix, smallest first:**
1. Correct the message in `watch.go` ("esc saves" is not true).
2. Offer **"keep mine as a new block"**: reload, then insert the unsaved text
   as a block after the block's old position (by row), through
   `InsertAfter`. Nothing is overwritten, nothing is lost.
3. Offer a single-block rebase: if the block's original text is still at the
   same place in the new file, replace just that block through the normal
   CAS path. This is a core operation (`app.RebaseBlock`), so the CLI gets it
   too.
4. At the least, `y` in STALE copies the unsaved text to the clipboard.

### 4. A wrapped conflict from undo is not recognised, and a partial undo leaves the screen stale

`asConflict` (`tui.go`, below `save`) uses a type assertion. `undo.go:166`
wraps the error with `%w` when a multi-file undo stops partway, so the
outliner shows the raw error instead of "cannot undo: … changed elsewhere".
This is the same bug `TODO.md` §3 fixed in the CLI with `errors.As`. Also,
`undoFile` returns as soon as there is an error and ignores the `done` that
came back, so files that *were* undone are not re-read. If the current file
was one of them, the next save is refused as stale.

**Fix:** `errors.As`, and reload when `done != nil` even if there is an error.

### 5. Every frame renders the whole page (measured)

`View` → `bodyLines` renders, highlights and wraps every row, then keeps only
the visible window. Measured per frame (100×40, rows with links and bold):

| rows on the page | View | keystroke + View |
|---|---|---|
| 400 | 4.8 ms | 4.6 ms |
| 2,000 | 26 ms | 25 ms |
| 6,000 | 69 ms | 72 ms |

The outliner asks for **all mouse motion**, so every pointer movement also
redraws everything. A long journal, or a person page with hundreds of
backlinks, will feel slow and use CPU while the pointer simply moves.

**Fix:** cache each row's rendered lines, keyed by text, width, selection and
editing state, and rebuild only the row that changed. Hit-testing data
(`lineRow`, `lineHits`) can come from the same cache. Skip the redraw on a
motion event when the hovered link did not change. Add a benchmark next to
`BenchmarkBuildRealCorpus` so this stays measured.

## Rendering gaps and inconsistencies (seen)

- **Pipe tables are not drawn in the outline.** `PLAN.md` lists them as
  built, but only `csv` fences become a grid (`view.go:386`). Pipe tables are
  drawn only in the file viewer (`mdview.go`). `markdown` already parses
  them, so the outline can reuse the grid.
- **`---` in a block is shown as text**, not as a rule.
- **A quote is marked twice:** `❝` as the bullet and `│` before the text.
  One is enough.
- **A heading leaves two spaces** after the bullet (`•  A heading`).
- **Language is mixed:** weekdays are English (`Fr`), the short date is
  German (`27.11.`), and the header says `auch:` for aliases while
  everything else is in English. Choose one: English, or follow `LANG`.

## Usability gaps

Getting around:
- **Picker input only supports typing and backspace.** No ←/→, `ctrl+w`,
  `ctrl+u` or `alt+backspace`, although `editor` already supports them.
  Reusing the editor's key handling fixes this.
- **Search results** show only the first line, without the match
  highlighted and without the page's path or context. No `n`/`N` to step
  through hits once you are on the page.
- **No search within the page.** `/` searches everything. A page-local
  filter that narrows the outline as you type is what outliners usually offer
  here.
- **No zoom (hoist).** Showing one block's subtree as the whole screen, with
  a breadcrumb, is the standard way to work inside a large page.
- **Nothing shows where you are** in a deep tree or a long page. Add a
  breadcrumb of ancestors in the header and a position indicator (`42%`) as
  the help screen already has.

Editing:
- **No word motion.** `alt+←/→` are days; `ctrl+←/→` and `alt+b/f` are not
  bound. Bind `ctrl+←/→` and `alt+b/f` to word left and right, and
  `ctrl+delete` / `alt+d` to delete a word forward.
- **No multi-block selection.** You cannot indent, move, delete, copy or tick
  off several blocks at once. In MOVE, `v` plus `j`/`k` (or shift+↑/↓) and
  then the existing letters, with each action running once through one core
  operation per range, is the natural extension.
- **No "new block above"** (`O` in MOVE) or **"new child"** key; both are
  common enough to deserve a letter now that letters are free in MOVE.

Tasks and time:
- **The agenda is one flat list.** Group it into *overdue / today / this
  week / later / no date*. Allow rescheduling in place (`d` then a date
  shorthand through `internal/dates`, the same path as `/deadline`). Filter
  by page or tag.
- **The journal does not show today's tasks.** A collapsed "Due today /
  overdue" section under today's journal, built like the linked references,
  would bring the agenda to where you already are.

Feedback:
- **The messages screen exists but is easy to miss.** Show a small marker in
  the header when an error was replaced before you could read it.
- **Background pull and push** show only when they fail. A quiet
  `↑`/`↓`/`✓` marker in the header says that sync is working.

## Code health

- **`tui.go` is 1,650 lines and `Model` has about 40 fields.** Overlays
  (picker, agenda, settings, log, file, help) each keep state in `Model` and
  set `m.mode` themselves, which is where bug 1 comes from. Move to an
  **overlay interface** (`Update(msg) (done bool, cmd)`, `View() string`) on
  a stack: closing one returns to whatever is underneath. That fixes bug 1
  structurally and makes each screen testable alone.
- **`internal/fuzzy` has 0 % test coverage**, but it decides what the
  palette, the `@` menu and go-to offer. It needs table tests, including the
  `ada` → Agenda case.
- **The picker sorts with an insertion sort** over every hit. This is fine
  for hundreds of items, but `sort.SliceStable` is shorter and does not
  degrade.
- **`ensureOnDisk` and `dirty` do `Render` + `Hash` in the adapter.** Each is
  correct, but "does memory differ from disk" is a question the core could
  answer (`Doc.Dirty()`), and `dirty` hard-codes the blank-bullet exception.
- **`holds` flattens the whole document** to find one block. It is cheap
  today, but it runs on the sync path; a set built once in `buildRows` would
  cost nothing.

## Ideas, in the order I would do them

**Quick (each under an hour, with a test):**
1. Return to the previous mode when an overlay closes (bug 1).
2. `errors.As` in `asConflict`, and reload after a partial undo (bug 4).
3. Correct the STALE message, and `y` copies unsaved text while STALE (bug 3, steps 1 and 4).
4. Always show a "New page ‘…’" row in go-to (part of bug 2).
5. Picker input reuses the editor's keys; `ctrl+←/→` word motion.
6. Quote marker, heading spacing, `---` as a rule, one language.

**Medium (a day or two each):**
7. Row render cache and skipping redraws on unchanged hover (bug 5), plus a benchmark.
8. Linked-only pages and tags as places in the core (bug 2).
9. "Keep mine as a new block" on conflict, as a core operation (bug 3, step 2).
10. Overlay stack refactor of `tui.go`.
11. Pipe tables in the outline, reusing `mdview`'s grid.
12. Agenda grouped by when, with reschedule in place.

**Larger (worth a `DECISIONS.md` entry first):**
13. Zoom/hoist with a breadcrumb.
14. Multi-block selection in MOVE.
15. Page-local filter (`f` in MOVE), and `n`/`N` after a search jump.
16. "Due today" section on today's journal.
17. Single-block rebase on conflict (`app.RebaseBlock`, with a CLI form and a `revamp_test.go` case per `AGENTS.md`).

Items 13–17 add behaviour and touch the core. 1–12 are fixes and polish
inside the rules as they stand.
