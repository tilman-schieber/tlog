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

## Known gaps

- A name that parses as an ISO date resolves to that day's journal rather than
  to a page, so `[[2026-09-17]]` and `tlog open 2026-09-17` go where you mean.
  A page cannot therefore be named after a date; nothing else claims a name.
- Block references have a syntax and a graph, but no UI for creating them.
- The outliner does not watch the filesystem; an external edit is caught by the
  checksum on the next write, and `R` reloads. A watcher is V0.2.
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
