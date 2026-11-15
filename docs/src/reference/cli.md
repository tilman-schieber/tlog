# The command line

`tlog` with no arguments opens the outliner. With a command, it does one thing
and exits — which makes it easy to use from a shell, a launcher, a status bar
or a script.

Every command accepts `-dir <path>` *before* it, to use a notes folder other
than `$TLOG_DIR` or `~/notes`:

```sh
tlog -dir ~/work-notes todo
```

`tlog help` prints the full list.

## Opening

| Command | What it does |
|---|---|
| `tlog` | today's journal, in the outliner |
| `tlog edit "Ada Lovelace"` | a page in the outliner |
| `tlog edit 2026-11-03` | a day in the outliner |
| `tlog today` | print the path of today's journal file |
| `tlog open "Ada Lovelace"` | print the path of a page's file, creating it if needed |

The last two exist so that other programs can work on the files:

```sh
nvim "$(tlog today)"
```

## Capturing

| Command | What it does |
|---|---|
| `tlog add "buy milk"` | a new block on today's journal |
| `tlog add -p "Grant proposal" "check the budget"` | …on a page |
| `tlog add -todo "call the supplier"` | an open task |
| `tlog add -todo -due fri "send the table"` | a task with a deadline |
| `echo "from a pipe" \| tlog add` | text from standard input |

Bind `tlog add` to a key in your launcher and you can capture a thought without
opening anything.

## What is due

| Command | What it does |
|---|---|
| `tlog todo` | every open task: dated first, soonest on top, then undated |
| `tlog due` | only what has a deadline |
| `tlog due -all` | including what is already done |
| `tlog date "next friday"` | what a date phrase means, without writing anything |

## Files

| Command | What it does |
|---|---|
| `tlog attach report.pdf` | copy into `assets/`, link on today's journal |
| `tlog attach -p "Grant proposal" report.pdf` | …link on a page |
| `tlog attach -link-only report.pdf` | copy, print the link, write no note |
| `tlog attach -clipboard` | the picture on the clipboard |
| `tlog attach -note` | the text on the clipboard, as a linked `.md` file |
| `tlog attach -note - < minutes.txt` | …or text from standard input |
| `tlog files [query]` | what is attached, newest first |

## Pages and tags

| Command | What it does |
|---|---|
| `tlog alias "Ada Lovelace" Ada` | give a page another name |
| `tlog tag person` | list every page tagged `person` |
| `tlog tag person "Ada Lovelace"` | tag a page |
| `tlog untag person "Ada Lovelace"` | untag it |
| `tlog people` | short for `tlog tag person` |

## Sync

| Command | What it does |
|---|---|
| `tlog push` | send the notes to their remote |
| `tlog push -auto on` · `off` | push after every commit, or stop |
| `tlog pull` | bring in notes pushed from another machine |
| `tlog sync` | show where the notes stand with their remote |
| `tlog undo [file]` | take back the last thing tlog committed |

See [History, undo and pushing](../concepts/git.md).

## Settings and setup

| Command | What it does |
|---|---|
| `tlog config` | every setting and its value |
| `tlog config <key>` | read one |
| `tlog config <key> <value>` | change one |
| `tlog import` | import a Logseq graph — see [Coming from Logseq](../more/logseq.md) |
| `tlog version` | the version |

## Reading and changing notes from a script

`tlog search`, `tlog view`, `tlog block`, `tlog index` and friends let a script
do everything the outliner can. They are described in
[Scripts and agents](../more/scripting.md).

## Good to know

- Flags may come before or after the arguments; anything after `--` is never a
  flag.
- Where a command takes text, `-` reads it from standard input.
- Every command has `-json`.
- Dates are accepted in [every form the outliner accepts](../concepts/dates.md).
