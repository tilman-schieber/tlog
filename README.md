# tlog

A markdown-first knowledge outliner for the terminal, with a desktop app. Journals, nested blocks,
`[[page links]]` and backlinks — in plain files you can `cat`, `rg`, `nvim` and
`git` without tlog running at all.

```
~/notes/
├── journals/
│   ├── 2026-09-16.md
│   └── 2026-09-17.md
└── pages/
    ├── Project Foo.md
    └── Rust.md
```

No Electron, no database, no daemon, no background process. The notes directory
is a git repository and tlog commits to it as you work, so every structural edit
is one `git checkout` away from being undone. Add a remote and
`tlog push -auto on`, and it pushes too.

Three ways in, one core: a terminal outliner, a CLI, and a desktop app that
renders in the system WebView and weighs about 8 MB.

## Install

```sh
just install      # the CLI and the outliner, into ~/.local/bin
just app-setup    # once: the Wails CLI, also into ~/.local/bin
just app          # the desktop app — macOS: app/build/bin/tlog.app
```

On Arch the desktop app also wants `gtk3` and `webkit2gtk-4.1`.

## Use

```sh
tlog                          # today's journal, in the outliner
tlog today                    # print the path of today's journal
tlog open "Project Foo"       # print a page's path, creating it if needed
tlog add "buy milk"           # append a block to today's journal
tlog add -p "Project Foo" "implement the parser"
echo "note" | tlog add        # append from a pipe
tlog attach report.pdf        # onto the shelf, and linked in today's journal
tlog files                    # what is on the shelf, newest first
tlog due                      # what is dated and still open, soonest first
tlog import                   # one-time import from a Logseq graph
tlog push                     # send the notes to their remote
tlog help                     # all of it, and `tlog version`

nvim "$(tlog today)"          # the point: it is just a file
rg '#research' ~/notes
```

The notes directory is `$TLOG_DIR`, or `~/notes`. Any command takes `-dir`.

## Keys

| | |
|---|---|
| `↑ ↓` / `j k` | move between blocks |
| `← →` / `h l` | collapse · expand, or step out and in |
| `enter` | new block below, and start typing |
| `i` / `e` | edit this block |
| `alt+enter` | newline **inside** a block (also `ctrl+j`) |
| `tab` / `shift+tab` | indent · outdent |
| `alt+↑` `alt+↓` | move the block and its children |
| `space` | toggle the task checkbox |
| `dd` | delete the block and its children |
| `[[` | page autocomplete, while typing |
| `/` | commands: `/todo`, `/deadline fr`, `/table`, `/code` … |
| `gf` / `gb` | follow the link · what links here, as a list |
| `enter` | on a reference below the outline: jump to it |
| `ctrl+p` / `/` | open a page · search every block |
| `t` / `[` `]` | today · previous, next day |
| `backspace` | back to where you came from |
| `R` | reload after an external edit |
| `?` / `q` | help · quit |

Arrow keys work everywhere; the vim keys are an alternative, not the foundation.

`shift+enter` is not distinguishable from `enter` in most terminals, so the
newline-inside-a-block key is `alt+enter` or `ctrl+j`. In kitty you can map
`shift+enter` to `\x1b\r` to get it as well.

## The dialect

Small on purpose. This is not CommonMark and does not try to be.

```markdown
---
title: optional page metadata as YAML frontmatter
---

- a block
  - a child block
  - a block can span several lines,
    including a fenced code block:
    ```go
    x := 1
    ```
- [ ] an open task, [x] a done one
- a block with properties
  status:: doing
  due:: 2026-09-20
- a link to [[Some Page]], to a block [[Some Page#^k3f9q2]], and a #tag
- met [[Ada Lovelace #person]] about [[Analytical Engine #project]]
- a block that something points at ^k3f9q2

- > a quotation, which is just a block whose text starts with >

- a block can hold ordinary markdown:
  1. a numbered list
  2. a second item

- tables are easier to type as csv than as pipes:
  ```csv
  Kürzel, Studiengang, ECTS
  BZ, Bioanalytik und Zellbiologie, 180
  ```
```

Everything here except `key:: value` is standard markdown, because the point is
that an agent reading your notes needs no knowledge of anyone's dialect. Block
properties survive untranslated only because markdown offers no equivalent.

## The desktop app

The same notes in a window, for when an outline is easier to read than to type:
sidebar of journals, pages and tags; the outline, editable in place; and the
tagged pages and linked references below it. `⌘F` searches, `⌘T` jumps to today.

Markdown is rendered while you read and raw while you write: the block under the
caret shows the characters that are actually in the file, everything else is
drawn. Rendered are links (wiki, markdown, bare URLs and the `file://` ones
`att` writes), **bold**, *italics*, `inline code`, ~~strikethrough~~, fenced
code as a highlighted monospace block, `>` quotations, numbered and bulleted
lists, tables, headings, rules and images. Clicking a page name follows it, a tag opens the tag
page, and a web or file link opens in the system.
Enter makes a new block, alt+Enter a newline, Tab and shift+Tab indent, alt+↑↓
move a block with its children, and `[[` completes — the same keys as the TUI.

It renders in the system WebView — WKWebView on macOS, WebKitGTK on Linux — so
the whole bundle is about **8 MB** with nothing embedded. There is no Electron
and no Chromium. It is a third adapter over the same core, which is why it can
be that small: `app/api.go` is 100 lines of bridging and holds no logic.

Because writes are compare-and-swap, the app, the outliner and nvim can all have
the same file open at once. A lost race is refused and reported, never merged.

On Hyprland with an NVIDIA card, WebKitGTK's DMA-BUF renderer can produce a
blank window; Wails detects the driver and disables it. Build natively rather
than shipping an AppImage — a bundled, stale `libwayland-client` is the usual
cause of an empty window on Wayland.

## Tasks and deadlines

`/` in a block opens a command menu, named after Logseq's so the muscle memory
carries over. The command is typed and then disappears: what stays in the file
is its effect, never a slash.

| | |
|---|---|
| `/todo` `/done` | a checkbox, open or ticked |
| `/deadline fr` | `Deadline:: 2026-09-25` on the block |
| `/date morgen` | `[[2026-09-25]]` inline, a link to that day's journal |
| `/quote` `/code` `/table` | a quotation, a code fence, a `csv` table |
| `/file report` | a link to an attachment, chosen from the shelf |
| `/page` `/tag` | a `[[link]]` or a `#tag`, where completion takes over |

The date argument is typed, not picked. A calendar appears beside the menu
showing what the shorthand resolved to — confirmation, so a mistyped date is
caught before it is written, rather than the way you are meant to enter one.

```
/deadline fr          Freitag
/deadline 26.9.       the German form you would type anyway
/deadline morgen      heute · morgen · übermorgen
/deadline +3d         +3d  +2w  +1m  +1j   (and 3t for Tage)
/deadline eow         end of week · eom · eoy
/deadline 2026-10-11  ISO
```

German and English both work, and whatever you type is stored as ISO — so the
files sort, grep and still mean the same thing in a year. Nonsense is refused
rather than guessed at: a silently wrong deadline is worse than being told the
word was not understood.

A dated block shows its date beside it, red when overdue, yellow when today.
Finished work is never overdue, however long ago it was due.

```sh
tlog due          # everything open and dated, soonest first
tlog due -all     # including what is done
```

## Attachments

Files live in `~/.att`, shared with [att](https://github.com/tilman-schieber/att)
rather than in a second place of tlog's own: the same flat store with original
names, the same collision rule, and byte-for-byte the same Markdown link. So
`att drop` and `tlog attach` fill one shelf, and a link written by either is
understood by both.

```sh
tlog attach report.pdf slides.pptx   # onto the shelf, linked in today's journal
tlog attach -p "Projekt" report.pdf  # ...or on a page
tlog attach -link-only report.pdf    # shelved, link printed, no note written
tlog files                           # the shelf, newest first
tlog files budget                    # narrowed, fuzzily if nothing contains it
```

In a note, `/file` offers the shelf. **In the desktop app, drop files on the
window** — they land on the shelf and their links land in the page you are
reading. Clicking one opens it in the Finder.

The original file is never moved: attaching it to a note should not take it
away from whatever else refers to it. Images become `![embeds]`, everything
else a plain link, and nothing on the shelf is ever overwritten — a clash gets
a numeric suffix, `report.pdf` then `report-2.pdf`.

att is not required. It owns the workflow tlog does not have — a drop folder
and a watcher — and if it is installed the two simply share a directory.

## Pushing

`tlog push` sends the notes to their remote. `tlog push -auto on` does it after
every commit instead, and the setting is stored in the notes repository itself
(`tlog.autopush`), not in tlog: whether these notes leave the machine is a
property of this directory, so a freshly created one never pushes by surprise.

Three rules it does not bend, because writing a note and publishing it are
different acts and only one of them is easy to take back:

- **A failed push is never a failed save.** The commit is what protects the
  notes; the push is a convenience on top. If it fails you are told "committed,
  but not pushed", and the commit is still there.
- **It never forces and never merges.** A rejected push means the remote has
  commits this copy does not — probably another machine. You get told to run
  `git pull --rebase` and look at what comes back. Resolving that is a decision.
- **It is bounded.** A push gets 20 seconds; being offline costs a moment rather
  than hanging a command that is supposed to feel instant.

A push that failed in the background is shown in the outliner's status line and
in the app, because a failure nobody sees is the same as no backup at all.

## Pages, tags and backlinks

**Every page shows what the rest of your notes say about it.** Below its own
outline, a page lists the blocks elsewhere that mention it — with their children,
because a mention is usually a bare name with the substance underneath it. Move
onto one and press `enter` to jump there. Those rows belong to other files and
are not editable in place.

**A tag is just a page.** `#person` and `[[person]]` name the same thing, so
there is no second namespace to keep in step. The `person` page shows everything
tagged `#person` and everything that mentions it, and can hold notes of its own.

**Type a page at the moment you mention it:**

```markdown
- met [[Ada Lovelace #person]] about [[Analytical Engine #project]]
```

That says *Ada Lovelace is a person* and *Analytical Engine is a project*. Now the
`person` page lists every person you have ever named, without you having gone to
a single one of their pages to declare it. Typing `#` after the page name inside
the brackets completes existing tags.

The same thing can be said on the page itself, if you prefer:

```markdown
---
tags: person, colleague
---
```

Both are read; neither is required. Tags inside brackets describe the page being
linked to, a bare `#tag` in prose is a mention of the tag itself, and only the
latter shows up as a linked reference — otherwise a tag's page would fill with
one identical line per page ever typed.

### Tables without the pain

A pipe table has to be re-aligned by hand every time a cell changes. A `csv`
fence does not — type rows, and both the outliner and the app draw a table:

```csv
Kürzel, Studiengang, ECTS
BZ, Bioanalytik und Zellbiologie, 180
CH, Chemie, 180
```

The separator is detected, so a semicolon-separated export from a German-locale
spreadsheet works unchanged; `tsv` and `psv` are honoured too. Quoted fields keep
their commas and a half-typed row is padded rather than rejected. Pipe tables
still render, so nothing you already wrote changes.

Fenced code is highlighted for go, js/ts, python, sh, sql, json, yaml and toml.
An unknown language stays plain monospace.

## Design

The core is the product. The outliner, the CLI and the desktop app are adapters
over `internal/app` and hold no logic of their own — which is why adding the
third one needed no new concepts.

**Markdown is the truth, and tlog owns the formatting of files it writes.** The
first write normalises a file to canonical form; from then on, re-rendering the
whole file changes only what actually changed, so diffs stay small without a
source-preserving patcher.

**A block is addressed by position plus a file checksum**, not by an id. Search
hands out both; a mutation passes the checksum back and is refused if the file
moved on. That is what makes concurrent editing safe with no daemon and no
locks: `nvim`, the outliner and a CLI call can all write, and a lost race
produces a visible refusal rather than a silent overwrite.

**Ids are lazy.** A block gets a written `^anchor` only when something needs to
name it permanently. Reading never writes.

**The index is in memory and thrown away.** At the sizes this targets, parsing
the whole directory costs single-digit milliseconds, which is cheaper than
keeping a cache honest.

`DECISIONS.md` has the reasoning behind all of that, including what was
deliberately not built. `PLAN.md` has what is still unbuilt, and what was looked
at and left alone.

## Importing from Logseq

```sh
tlog import -dry-run     # report, write nothing
tlog import              # write, then commit
```

Reads a Logseq graph (or its markdown mirror) and translates it:

- `journals/2026_09_15.md` → `journals/2026-09-15.md`
- `TODO` / `LATER` / `DOING` → `- [ ]`, `DONE` / `CANCELED` → `- [x]`
- Logseq's page header properties → YAML frontmatter
- block ids → `^anchors`, and `[[uuid]]` / `((uuid))` → `[[Page#^anchor]]`
- `Sep 16th, 2026 00:00` → `2026-09-16`
- `collapsed::` and other UI state → dropped
- pages holding nothing but a Logseq id → not created
- **page classes → `tags:` frontmatter**, read out of Logseq's own database

References whose target no longer exists are left exactly as written and
reported rather than guessed at.

**Page tags come from the database, not the mirror.** Logseq's DB version keeps
the truth in SQLite and writes the markdown mirror as a lossy export: it carries
no page classes at all, so importing from the files alone silently drops every
person, project and topic you have classified. tlog reads the classes back out
of `db.sqlite` — on a snapshot copy, so a running Logseq is never disturbed — and
writes them as `tags:` frontmatter. It needs `sqlite3` on PATH; without it the
import says so and proceeds untagged.

**Your Logseq graph is only ever read.** tlog cannot modify, move or delete it,
so importing is safe to repeat as often as you like, and the import refuses
outright if the destination overlaps the source. What is one-way is the
*conversion*: there is no converter back, so notes you write in tlog will not
appear in Logseq. Keep using Logseq until you decide to stop.

## Development

```sh
just test    # go vet, go test, and the frontend's own tests
```

Everything is testable without a terminal or a window. The outliner is driven
headlessly by sending key messages to its model and asserting on what lands on
disk (`internal/tui/tui_test.go`); the desktop app's bindings chain edits using
only what the previous call returned (`app/api_test.go`), which is exactly what
its frontend does; and the frontend's pure helpers are lifted out of `app.js`
and tested in node (`app/frontend/app_test.js`). If a change cannot be tested
that way, the logic is in the wrong layer.

`AGENTS.md` has the rules that matter for changing any of it.

## Licence

MIT — see `LICENSE`.

tlog itself depends on Bubble Tea and Lip Gloss for the terminal and Wails for
the desktop app, all MIT. Everything else it uses is the Go standard library.
Their own transitive dependencies are a mix of MIT, BSD-3-Clause, ISC and
Apache-2.0; `go list -m all` is the authoritative list.
