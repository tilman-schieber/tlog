# tlog — notes for agents

Markdown-first knowledge outliner. Go, single binary, macOS + Linux.
Read `DECISIONS.md` before changing anything architectural; it records why the
design departs from `coding-brief.txt`. `PLAN.md` holds the dialect additions
that are designed but not built.

## Build and test

```sh
just test      # go vet, go test, the Omarchy widget's test
just ci        # the same with -race and gofmt, as CI runs it
just build
just install   # GOBIN=~/.local/bin go install ./cmd/tlog
```

`just test` is not only Go: `omarchy/tests/model_test.js` runs too, with node.

Everything is testable without a terminal. The outliner is driven headlessly by
sending `tea.KeyMsg` values to `Model.Update` and asserting on what lands on
disk (`internal/tui/tui_test.go`). If you cannot test a change that way, the
logic is in the wrong layer.

## Layout

```
cmd/tlog/            main — the CLI and the outliner
internal/markdown/   the dialect: parse, render, structural edits
internal/store/      the notes directory: paths, CAS writes, git
internal/attach/     attachments: the notes' assets/ folder
internal/config/     ~/.config/tlog/config.toml
internal/dates/      typed shorthand in, ISO out
internal/fuzzy/      ranking strings against a typed prefix; knows nothing of notes
internal/termimg/    drawing a picture in a terminal: kitty, sixel, iTerm2; knows nothing of notes
internal/graph/      derived view: pages, links, backlinks, search
internal/watch/      noticing edits made elsewhere: a stat sweep, not fsnotify
internal/app/        the core — every semantic operation
internal/importer/   one-way Logseq conversion
internal/cli/        adapter, and the surface scripts and agents drive
internal/tui/        adapter
omarchy/             the Omarchy bar widget (Quickshell), over `tlog todo -json`
skills/tlog/         the skill that teaches an agent to use the command line
```

## The rules that matter

**The core is the product.** `internal/app` holds every semantic operation —
including the block mutations (`mutate.go`) and the read model (`view.go`). The
TUI and the CLI are adapters. If something can be done in one
of them but not through `app.Service`, that is a bug — it is what makes the tool scriptable and
agent-addressable, which is the whole point.

**The command line is the agent surface.** `internal/cli` is how a script or
an agent reaches the core, so it keeps a contract: every read that can be acted
on carries an address (`rel:offset@hash`), every write takes one and prints the
one the block ended up at, `-json` exists on everything, empty lists are `[]`,
flags work on either side of the arguments, `-` reads text from stdin, and the
exit code says which kind of no it was — 2 not a command or address, 3 that does
not exist, 4 the file moved since it was read. A new core operation gets a CLI
form and a test in `internal/cli/revamp_test.go`; `tlog block … cmd <name>`
already covers a new slash command. `skills/tlog/` documents the contract for
an agent using tlog; change it when the contract changes.

**Never write without a hash.** `store.Write` takes an `ifMatch` checksum and
refuses if the file changed. Pass an empty one only for files nothing else could
be holding (the importer writing into a fresh directory). A silent overwrite of
someone's notes is the worst bug this program can have.

**Render, do not patch.** Files are canonical after their first write, so
`markdown.Render` of the whole document is already a minimal diff. Do not
reintroduce span patching.

**Reading never writes.** Anchors are materialised only by an explicit request
to name a block (`Service.Anchor`). If a read path ever writes, the lazy-identity
guarantee is gone.

**Terminal colours only.** Semantic roles map onto the sixteen ANSI colours in
`internal/tui/theme.go`. No RGB anywhere — the tool inherits the terminal theme.

**Slash commands live in one table.** `internal/app/commands.go` holds the menu
and does the cutting of `/name arg` out of the text. Adding one is a table entry
plus a case; do not teach an adapter about a command it can look up.

**A setting is described once.** `internal/app/settings.go` holds every
configurable key with its kind, hint and consequence; `tlog config`, the
outliner and the app all render that list. Adding a setting means adding it
there and in `internal/config`, never in an adapter.

**Do not add a setting for a decision.** ISO dates, files as truth,
compare-and-swap writes and the dialect are settled; `DECISIONS.md` says why. A
setting for one of them is a supported way to break it.

**Attachments live in the notes.** Pasted pictures and dropped files are
copied into `assets/` inside the notes directory, committed with the note that
links them, and linked relatively as `../assets/name`. Their location is not a
setting, and nothing may write a link to a file outside the notes. The store
skips `assets/` when listing, so an attached `.md` is never a page.
`TestLinkFormat` pins the link.

**Dates are stored as ISO, always.** `internal/dates` is forgiving on input and
strict on output. Anything that cannot be parsed is refused rather than guessed
at — a silently wrong deadline is worse than an error.

**The dialect stays small.** It is not CommonMark and will not become CommonMark.
Add syntax only when the product actually needs it, and prefer whatever standard
markdown already has. Rendering is not the dialect: quotes, lists, tables, code
and `csv` fences are all ordinary markdown inside a block's text, and nothing
about how a file is stored changed to support any of them.

**Highlighting must not lose a character.** `markdown.Highlight` tokens have to
reassemble into exactly the input, for every language and for half-pasted code.
That is the first test in `highlight_test.go`; a highlighter that drops a
character silently corrupts what the reader sees.

## Testing conventions

- Parser and writer changes need a round-trip case in
  `internal/markdown/render_test.go`: canonical input must survive parse/render
  byte for byte, and normalisation must be idempotent.
- Structural operations are golden-tested through `Render` in `edit_test.go`.
- Importer changes should be checked against a real graph with
  `tlog import -dry-run -from ~/logseq` as well as the unit tests.

## Things deliberately absent

No daemon, no socket, no SQLite, no fsnotify, no Neovim plugin, no MCP server.
Each was considered and cut; `DECISIONS.md` says why. Do not add them back
without a measured reason. `internal/watch` is not the file watcher that was
cut: it is a stat sweep that runs in process, only while the outliner is open,
and nothing runs when it is closed. The command line is the
agent surface; an MCP server would sit on exactly that and is not built until
something needs it.

There is no desktop app. A Wails window existed until November 2026 and was
retired because the outliner had caught up with it; its code is at the tag
`desktop-app-final`, and DECISIONS.md says why. Do not bring a GUI back as a
second place for logic: anything one needs belongs in `app.Service` first.
