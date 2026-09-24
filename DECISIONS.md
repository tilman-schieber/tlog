# Decisions

`coding-brief.txt` is the original design brief. This file records where the
implementation departs from it, and why. Where the two disagree, this file wins.

## Premise

No Electron, markdown as the source of truth, and agents that read the files
directly. The sharp version of that last claim is not "a faster API than
Logseq's MCP" — that already exists and is already enabled — it is **zero round
trips**: an agent `rg`s the corpus.

## Storage and identity

**Files are truth, but tlog owns the format.** The brief (§9) forbade
reformatting and asked for a source-preserving patcher. That buys little — these
files are written by you and by tlog, not by a hostile third party — and costs
the one invariant that makes patching safe. tlog normalises a file on its first
write and re-renders whole files after that. Because the file is already
canonical, a full re-render *is* a minimal diff. Files tlog is not otherwise
modifying are never touched.

**Addressing is position plus checksum, not ids.** §7 (lazy ids) and §13 (agents
mutate by id obtained from search) contradicted each other: with lazy ids, search
would have to write ids into files just to answer a read. Resolved by making the
address `file:offset@hash` and every mutation a compare-and-swap. An ephemeral
address is safe to mutate through precisely because the write is conditional.

**Anchors are lazy and Obsidian-style.** `^k3f9q2` at the end of a line, six
base36 characters, unique per page since every reference is page-qualified. The
brief's `<!-- id: ULID -->` was rejected: no other tool understands it, and it is
a full ULID of clutter in every diff. An anchor is materialised only when
something asks to name the block — never as a side effect of reading.

**Page name is the filename.** `[[Foo]]` is `pages/Foo.md`, matched
case-insensitively, created when you follow the link rather than when you write
it. Duplicate page names are therefore impossible by construction, which deletes
`ResolvePage` ambiguity from the domain model and the duplicate-name case from
the test plan (§23) rather than handling them.

**Durability is git, not backup files.** §22 asked for "no data loss" and
"backups on risky writes" without a mechanism. `~/notes` is a git repository and
tlog auto-commits ~30s after the last write (immediately, for one-shot CLI
commands). Structural edits rewrite whole files, so the only honest protection
against tlog confidently writing the *wrong* thing is a history you can walk
back. This replaces the tempfile/backup machinery and gives undo for free.

## Concurrency

**No daemon, no locks, optimistic concurrency.** §14's daemon plus the standalone
fallback created *two* independent writers with no conflict story anywhere in the
brief. Instead: every write verifies the file still hashes to what was read, then
lands via temp file and atomic rename. Conflicts are surfaced, never merged. The
outliner writes on leaving a block rather than on quitting, so the window in
which anything can be lost is one block wide.

The daemon's only remaining job would have been avoiding a re-parse per CLI call.
At these corpus sizes that is a few milliseconds. Deleted, along with the Unix
socket.

**SQLite is not built.** §16 scheduled it for V0.3; it is not in the roadmap at
all now. The in-memory graph is rebuilt from scratch on demand, including after
every block edit, because the sections shown on a page are derived from it.

Measured on the real corpus (43 files, 517 blocks): **7.9 ms** per full rebuild
(`BenchmarkBuildRealCorpus`, run with `REAL_DIR=...`). That is well inside a
frame and not worth optimising. It grows linearly, so revisit the index decision
when a rebuild passes roughly 50 ms — around a thousand files — and not before.

## Dialect

**Multi-line blocks and code fences are in V0.1.** §6 deferred both. The real
Logseq corpus already contains multi-line blocks and an embedded heading inside a
bullet, so a single-line parser could not import the user's own notes. The cost
is real and was accepted: the outliner needs an inline multi-line editor
(`internal/tui/editor.go`), which is the work §10 was trying to avoid.

The import reads the Logseq graph and never writes to it, and refuses if the
destination overlaps the source. "One-way" refers to the conversion, not to any
risk to the original data: there is no converter back.

**Standard markdown wherever one exists.** Logseq task markers become GFM
checkboxes, page metadata becomes YAML frontmatter, block references become
`[[Page#^anchor]]`. Only `key:: value` block properties survive, because markdown
has no equivalent. The result renders correctly in GitHub, glow and
render-markdown.nvim.

**A tag is a page, and pages are typed where they are mentioned.** The brief
(§6, §10) treated tags as a separate kind of thing with their own autocomplete
and colour. Two namespaces is the part of Logseq that felt overcomplicated, and
the corpus showed why: zero `#tags` in three months, because declaring one meant
navigating to a page to do it.

So `#person` and `[[person]]` name the same page, and `[[Andreas #person]]`
asserts *Andreas is a person* at the moment you first mention him. A page's own
`tags:` frontmatter says the same thing and is read equally. The tag's page then
lists everything tagged with it — which is what makes it worth having.

Tags inside brackets describe the linked page; a bare `#tag` in prose is a
mention of the tag itself. Only the latter becomes a linked reference, or every
tag page would carry one identical line per page ever typed, beside the list that
already says it better.

**Linked references are shown on the page, with their children.** Backlinks
behind a keystroke are backlinks nobody looks at. A mention is usually a bare
name with the substance nested under it, so showing only the referring line says
nothing you did not already know; the subtree is shown, capped per reference.

## Interfaces

**Enter makes a new block; alt+enter makes a newline.** The most-pressed key in
the app, undecided in §10. This is what Logseq, Obsidian, Notion and Roam all do,
so the muscle memory already exists. Arrow keys are first-class and vim keys
layer on top, per the note added to §10.

**No Neovim plugin; an LSP server instead (V0.2).** §11 wants completion in the
TUI, Neovim, Helix, Zed and VS Code — five of which speak LSP natively. An LSP
server additionally yields go-to-definition, backlinks as find-references and
rename-updates-all-links, all of which are on the roadmap anyway. The completion
logic already lives in the core (`graph.LinkTargets`, `graph.FuzzyRank`) so the
server and a `tlog complete --json` are both thin adapters over one function.

**Theming is the terminal's.** Semantic roles map onto the sixteen ANSI colours;
no RGB is hardcoded. Omarchy themes the terminal, so tlog inherits it, and it
stays legible over ssh and in tmux.

## The desktop app

**Wails, not Electron, SwiftUI, Gio or Tauri.** The core is a Go package, so the
question was which host can call it *in process*, with no protocol to invent.
Wails binds Go methods straight into the WebView, which makes the adapter 100
lines of bridging. The result is an 8 MB bundle against Logseq's 456 MB and
Obsidian's 515 MB, because the WebView is the system's and nothing is embedded.

SwiftUI would be more genuinely native and was the runner-up. It costs a ~15 GB
Xcode install, a second language, a cgo boundary, and it abandons Linux — and
the Omarchy machine is half of where this runs. Gio and Fyne draw their own
widgets, so text editing, IME and menus would all be rebuilt; an outliner is
almost entirely text editing. Tauri is the same WebView idea with a Rust host,
which would demote the Go core to a sidecar with an IPC protocol to maintain.

**No frontend build step.** Plain HTML, CSS and JS, served from `app/frontend`,
so a Go repository does not acquire npm and the binary stays self-contained.
Svelte and Vite remain one config line away if the UI outgrows it.

**The block mutations moved into the core to make this possible.** The TUI had
been reaching into `markdown.Document` directly; a second adapter doing the same
would have been duplication of exactly the kind `§24` forbids. `app/mutate.go`
now holds them, compare-and-swap guarded, and `app/view.go` holds the read model
both the app and a future `--json` CLI render. That the GUI needed no new
concepts is the clearest evidence the adapter boundary was drawn in the right
place.

This entry was written before it was true. The mutations moved into the core
*for the GUI*; the outliner went on reaching into `markdown.Document` and
`graph.Graph` for another day, which left the read model and the CAS layer
reachable from exactly one of the three adapters — the opposite of the claim.
Two adapters drifting apart is what that costs: Enter on a block with collapsed
children made a sibling in the outliner and a first child in the app. The
outliner was migrated afterwards, and the paragraph is kept with its correction
rather than quietly fixed, because a design note that no longer describes the
code is worse than none.

**Splitting a block is one operation, not two.** Enter is the most frequent key
in an outliner, and writing it as set-the-text then insert-after is two writes,
two commits, and a window in which the file can move between them and leave half
the split on disk. `SplitBlock` and `MergeIntoPrevious` take one load and one
save, so the pair is atomic or it is refused.

**Collapse is view state, so the core does not read it.** `InsertAfter` and
`SplitBlock` take an explicit `asChild bool`: the adapter has seen whether the
children are on screen and decides; the core never guesses from whether a block
*has* children. It used to guess, which is why this is a parameter.

## Rendering

**Rendering is not the dialect.** Quotes, ordered and bulleted lists, tables,
headings, rules, images and fenced code are all ordinary markdown living inside
a block's text. Every one of them was added by teaching the adapters to draw,
and not one changed how a file is stored — so the round-trip invariant never
moved and no existing note was touched.

**Markdown is rendered while reading and raw while writing.** The block under
the caret shows the characters that are actually in the file, because the caret
has to land where they are and what you type is what is stored. Hiding syntax
while editing it is how round-trip guarantees die.

**Measure the corpus before adding syntax.** Two of the guesses that drove the
first pass were wrong: "zero headings" was a bad regex (they are written as
`- # Heading`, the bullet's own text, so `^\s*#` never matched — there are 34),
and "68 horizontal rules" was 56 frontmatter delimiters plus 12 real ones.
Tables turned out to be used in five files. The habit is cheap and it has paid
twice.

**`csv` fences instead of pipe tables.** A pipe table must be re-aligned by hand
every time a cell changes; rows of values need not be. The separator is detected
rather than assumed, because a spreadsheet exported on a German-locale machine
uses semicolons and nobody should have to care. It stays ordinary markdown:
anything that does not know about tlog shows a code block, which is a readable
thing to show.

**Highlighting lives in Go, not JavaScript.** A JS library would serve only the
desktop app, need a build step, and leave the outliner plain. A Go tokenizer
serves both and a `--json` caller too. `chroma` was rejected: two hundred
languages against one code fence in the corpus would roughly double the 8.4 MB
bundle. The invariant is that tokens must reassemble into exactly the input, for
every language and for half-pasted code — notes are full of half-pasted code,
and a highlighter that drops a character silently corrupts what you read.

## Commands, tasks and deadlines

**Slash commands, named after Logseq's.** One extensible surface rather than a
trigger per feature, and the muscle memory carries over. An `@` trigger for
dates was designed first and dropped: it would have spent a character on one
feature and left the next one homeless.

**A command disappears when it runs.** What stays in the file is its effect —
`/todo` leaves a checkbox, `/deadline fr` leaves a property, neither leaves a
slash. The menu and the text-cutting both live in `internal/app`, so the two
adapters cannot cut differently.

**A deadline is a property, not an inline date link.** `[[2026-09-25]]` reads
better and would give the day's journal its backlinks for free, but

    - [ ] Rückmeldung zum Protokoll von [[2026-09-20]]

is a past date that is emphatically not a deadline, and an agenda built on "task
plus date link" would scream about it. Implicit is wrong here. The property is
spelled `Deadline` to match what the corpus already had; reading accepts any
casing and `due` as well.

**Dates are typed, and the calendar only confirms.** `/dl fr` is eight
keystrokes and no picker beats that, so the calendar shows what the shorthand
resolved to rather than being the way in. German and English are both accepted
because these notes are both, and everything is stored as ISO so the files sort,
grep and mean the same thing in a year. Unparseable input is refused rather than
guessed at: a silently wrong deadline is worse than being told the word was not
understood.

**"nächsten Freitag" is defined, not inferred.** It is always one week after
plain "Freitag". The phrase is genuinely ambiguous in speech, and a rule that is
written down beats one that is clever.

**Finished work is never overdue**, however long ago it was due.

**`tlog due` is why deadlines are stored at all.** A date nobody can ask about
is just text that looks like a date. Run against the real corpus for the first
time it surfaced something seven days overdue, which is the whole argument.

## Settings

**Almost nothing is configurable, on purpose.** ISO dates, files as the source
of truth, compare-and-swap writes and the small dialect are decisions, not
preferences; a setting for each would only be a supported way to break them.
What is configurable is the handful of places where two reasonable people would
genuinely want different things.

**One description, three surfaces.** `internal/app/settings.go` holds the list
with each value's kind, its explanation and its consequence. `tlog config`, the
outliner's `,` screen and the app's panel all render that, so they cannot offer
different things or disagree about what a value means.

**TOML in `~/.config`, on macOS too.** Not `Library/Application Support`:
everything else here already lives in `~/.config` — nvim, fish, kitty,
aerospace, starship, mise — and the same dotfiles are stowed onto an Arch
machine, where a second location would be one more thing to keep in step.

**The file is rendered whole, with its own documentation in it.** Reading the
file is how you find out what can be configured, so saving regenerates the
comments. Hand-written ones are replaced; values never are.

**A broken config must not stop someone writing a note.** A parse error is kept
on the service and shown, not returned from startup, and an unusable value falls
back to the default rather than failing.

**Some settings are not tlog's to keep.** Pushing and the remote live in the
notes repository's own git config, not in `config.toml`. Where your notes go is
a property of that directory: a copy of them carries the answer along, and there
is never a second file to disagree. They are still shown and changed through
tlog's settings, marked with where they are kept — display and storage are
different questions, and answering them separately is what avoids two sources of
truth for one fact.

**A setting that cannot take effect yet says so.** Changing the notes directory
waits for a restart; changing `blank_lines` reformats files on their next write.
Both are announced rather than left to be discovered.

## Attachments

**Share att's directory rather than inventing one.** `att` already owns the
attachment store, it is already live in these notes, and it owns the half of the
workflow tlog has no business in — a drop folder and a watcher. tlog does the
other half: shelving a file from inside a note, and finding one again while
writing.

**The rules are restated, not imported.** att's packages are internal to its
module, so Go forbids importing them. The handful that matter — a file keeps its
name, a clash gets a numeric suffix, nothing is overwritten, an image becomes an
embed — are small, unlikely to move, and pinned by tests that were checked
against att's real output byte for byte, escaped brackets and percent-encoded
spaces included.

**The original is never moved.** att's `add` copies and so does tlog's:
attaching a file to a note should not take it away from whatever else refers to
it.

**A window can be dropped onto, and a terminal cannot.** File drop is the one
thing the desktop app does that the outliner has no equivalent for, and it is
the reason to have the app open while writing.

## Pushing

**Pushing is opt in, per notes directory.** The setting lives in the notes
repository as `tlog.autopush`, not in tlog: whether a directory's contents leave
the machine is a property of that directory, so a freshly created one never
pushes by surprise, and turning it on without a remote is refused rather than
quietly ignored.

**A failed push is never a failed save.** The commit protects the notes; the
push sits on top. Failure reads "committed, but not pushed" and the commit
stands. A push that failed in the background is surfaced in both adapters,
because a failure nobody sees is the same as no backup at all.

**It never forces and never merges**, and it is bounded at twenty seconds. A
rejected push means the remote moved; resolving that is a decision, and the same
rule applies as to a lost write — refuse and report rather than merge behind
someone's back. Auto-pull is not built for exactly that reason.

## Scope

V0.1 is the smallest thing worth using daily: import, journals, nested multi-line
blocks, create/edit/delete/indent/outdent/move, `[[links]]` and following them,
backlinks, search, page completion, `tlog today`, `tlog add`, `tlog open`.

Deferred to V0.2: CLI mutation commands, the JSON output contract, the LSP
server, `rebuild`/`doctor`, a file watcher. The parser should survive contact
with real notes before anything is built on top of a JSON schema.

Deferred further: MCP (a thin transport over the CLI contract, once that
contract is stable), block-reference UI, SQLite, a web UI.

Not built, per §20: Electron, native GUI, sync, CRDT, collaboration, a plugin
system, full CommonMark, Datalog, a theme engine.

## Naming

`noto` collides with Google's Noto font family: permanently un-googleable, and it
reads as a typo. The tool is `tlog`.

## Watching the files

**Poll, do not subscribe.** On macOS fsnotify is kqueue: one file descriptor per
watched file, no recursion, and re-registration after every atomic rename — and
every write here *is* an atomic rename. It would also need a hand-written list
of things to ignore: `.git`, `.tlog-*.tmp`, editor swap files, nvim's `4913`
probe. `store.List` already answers "what is a note" for the whole program, so
the watcher asks it instead of keeping a second opinion. A stat sweep of the
corpus costs tens of microseconds against the 8 ms graph rebuild a save already
pays, so the simpler thing is also the faster one at this size.

**Debounce by content, not by time.** An atomic rename, an in-place write, a
swap file and a `git checkout` then collapse into one question: are the bytes at
this path different from last time? A burst of writes becomes one change for
free, and a write that restores identical bytes emits nothing. The cost is up to
one interval of latency, which nobody can feel.

**tlog recognises its own writes,** because `store.Write` records the hash it
wrote. That is what makes the loop provably terminate: without it every save
would come back as an external edit and move the page under the caret. It lives
in the store rather than the watcher so that the importer and the anchor writer
get it without remembering to.

**Typing is never discarded.** When the file changes while a block is being
edited, both surfaces say so and reload nothing. The compare-and-swap write was
already the protection; this only adds the warning. `R` in the outliner used to
discard an unsaved edit silently, and now asks first.

The risk worth writing down rather than engineering around: a poll can read a
file mid-write from an editor that does not write atomically, and briefly show a
truncated version. It corrects itself within one interval, and the CAS write
protects the file itself. fsnotify has the same problem, sooner.

## Aliases

**An alias is a name, not a page.** It resolves to the canonical name when a
reference is *recorded* and when one is *looked up*, so a page has exactly one
set of backlinks however it was written. Merging two sets at read time would
have been the other way to do it, and would have left `[[Ada]]` and
`[[Ada Lovelace]]` as two things that mostly agree.

**A real page always wins its own name.** If `Ada Lovelace` claims the alias
`Grace` and a `Grace` page exists, the alias is dropped. A line of frontmatter
must not be able to make a file on disk unreachable.

**Resolution only builds the graph when it has to.** `Service.ResolvePage`
answers from the filesystem first; only a name with no file behind it asks
whether some page answers to it. Following a link to a page that exists — the
overwhelmingly common case — costs nothing. That the check lives in the service
and not in `store` is deliberate: the store owns bytes and filenames, and what a
name *means* is interpretation.

## Known gaps

- A name that parses as an ISO date resolves to that day's journal rather than
  to a page, so `[[2026-09-17]]` and `tlog open 2026-09-17` go where you mean.
  A page cannot therefore be named after a date; nothing else claims a name.
- `Outdent` moves a block to just after its parent and leaves its following
  siblings where they were. Some outliners instead adopt those siblings as
  children. Predictable was preferred over clever.
- The Logseq markdown mirror carries no block-level ids, so `[[uuid]]` block
  references in that corpus cannot be resolved and are left verbatim. The
  importer reports how many.
- Page classes are read straight out of Logseq's `db.sqlite`, whose kvs table
  holds transit-encoded datascript datoms. That is an internal, undocumented,
  version-specific format, and `internal/importer/logseqdb.go` is the only place
  that knows about it. When it cannot be read the import continues untagged and
  says so. The mirror does not carry this information in any form, so the choice
  was between reading the database and losing the typing entirely: on the real
  graph it is 28 pages, which is not an acceptable thing to drop silently.
