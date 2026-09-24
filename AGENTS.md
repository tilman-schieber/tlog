# tlog — notes for agents

Markdown-first knowledge outliner. Go, single binary, macOS + Linux.
Read `DECISIONS.md` before changing anything architectural; it records why the
design departs from `coding-brief.txt`. `PLAN.md` holds the dialect additions
that are designed but not built.

## Build and test

```sh
just test      # go vet ./... && go test ./...
just build
just install   # GOBIN=~/.local/bin go install ./cmd/tlog
```

Everything is testable without a terminal. The outliner is driven headlessly by
sending `tea.KeyMsg` values to `Model.Update` and asserting on what lands on
disk (`internal/tui/tui_test.go`). If you cannot test a change that way, the
logic is in the wrong layer.

## Layout

```
cmd/tlog/            main — the CLI and the outliner
app/                 the desktop app (Wails): main.go, api.go, frontend/
internal/markdown/   the dialect: parse, render, structural edits
internal/store/      the notes directory: paths, CAS writes, git
internal/dates/      typed shorthand in, ISO out
internal/graph/      derived view: pages, links, backlinks, search
internal/app/        the core — every semantic operation
internal/importer/   one-way Logseq conversion
internal/cli/        adapter
internal/tui/        adapter
```

## The rules that matter

**The core is the product.** `internal/app` holds every semantic operation —
including the block mutations (`mutate.go`) and the read model (`view.go`). The
TUI, the CLI and the desktop app are adapters. If something can be done in one
of them but not through `app.Service`, that is a bug — it is what makes the tool scriptable and
agent-addressable, which is the whole point.

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
- The desktop frontend has tests too: `node app/frontend/app_test.js`, run by
  `just test`. They lift the pure helpers out of `app.js` so it can stay a plain
  script the WebView loads with no module system.

## Things deliberately absent

No daemon, no socket, no SQLite, no file watcher, no Neovim plugin, no MCP
server yet. Each was considered and cut; `DECISIONS.md` says why. Do not add
them back without a measured reason.

The desktop app deliberately has no build step: the frontend is plain
HTML/CSS/JS served from `app/frontend`, so there is no npm in a Go repo.
Swapping in Svelte and Vite later is a `frontend:build` line in
`app/wails.json`.
