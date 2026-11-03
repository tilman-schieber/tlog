---
name: tlog
description: Read and change a person's notes kept in tlog, the markdown-first knowledge outliner, through its command line — find blocks, add notes and tasks, set deadlines, tick things off, attach files, follow links — without ever editing the markdown files directly. Use this whenever the user mentions tlog, their notes or journal, a page in their notes, a todo or deadline they keep in tlog, or asks you to write something down, look something up, or change a note, even if they do not say "tlog"; also when a notes directory holds journals/ and pages/ of markdown with "- " bullets and "Key:: value" properties. Use it before reaching for cat, rg, sed or your own edits on those files.
---

# Working in someone's notes with tlog

tlog keeps notes as plain markdown in a git repository, usually `~/notes`:
`journals/2026-11-03.md` for days, `pages/Name.md` for everything else, each a
tree of `- ` bullets. The person may have the outliner or the desktop window
open on the same files while you work. The command line is the one way in that
cannot collide with them, so use it for every read that leads to a write and
for every write. Do not edit the files with your own tools: a plain rewrite
skips the concurrency check, breaks the outline shape, leaves dates in forms
tlog would otherwise normalise, and leaves nothing committed. Reading a file
with `cat` to get the gist is fine; acting on what you read goes through tlog.

## The one idea: addresses

Every block you can act on has an address that tlog prints and you copy:

```
pages/Grant.md:0@39ad22fa87fdf960
└── file ───────┘ │ └── hash of the file when you read it
                  └── where the block starts
```

Reads hand addresses out (`search`, `view`, `todo`, `due`, `add -json`,
every write). Writes take one and print the block's **new** address, because
rewriting a file moves every offset after it. So the loop is always:

1. read with `-json` and take an `addr`;
2. write with it;
3. carry on from the address the write printed, never the one you started with.

Never assemble an address yourself, never reuse one after a write to that
file, and never strip the hash: the hash is what makes tlog refuse to write
into a file that changed under you.

## What the exit code tells you

| code | meaning | what to do |
|---|---|---|
| 0 | done | carry on from the printed address |
| 1 | something went wrong | read stderr |
| 2 | not a command, operation or address | fix the call; nothing was written |
| 3 | that does not exist: a page nobody made, a move with nowhere to go, a date nobody can read | do not retry; nothing was written |
| 4 | the file moved since you read it | read again, take the fresh address, redo the write |

A 4 is normal when the person is typing in the same file. It is never a
reason to force anything; it is a reason to read again.

## Reading

```sh
tlog search "budget" -json          # blocks, each with addr, page, text, firstLine
tlog view "Grant" -json             # a page: blocks with addr/depth/task/due, backlinks
tlog view -day yesterday -json      # a journal: today, -1, fri, 2026-11-03
tlog block <addr>                   # one block, with its properties and deadline
tlog todo -json                     # every open task, dated first
tlog due -json                      # what is dated and open, soonest first
tlog links <addr> -json             # where a block's [[links]] and urls go
tlog index -json / tlog places q    # every journal, page and tag, with paths
```

`view` and `search` create nothing and are safe to run anywhere. A page that
does not exist is exit 3, not an empty page: `tlog open <name>` is the only
read that creates.

## Writing

```sh
tlog add "a thought" -json                       # today's journal
tlog add -p "Grant" "a thought" -json            # a page, created if needed
tlog add -todo -due fri "write the budget" -json # an open task with a deadline
tlog block <addr> text "new words"
tlog block <addr> after "a sibling below"        # add -child for a first child
tlog block <addr> before "a sibling above"
tlog block <addr> done | open                    # idempotent; prefer over `task`
tlog block <addr> deadline 2026-11-20            # makes it a task if it is not one
tlog block <addr> prop status open               # a Key:: value; no value removes
tlog block <addr> indent | outdent | up | down | merge | delete
tlog block <addr> cmd <name> [arg]               # any slash command: tlog commands
tlog block <addr> ref                            # a durable [[Page#^anchor]] link
tlog attach -p "Grant" ./report.pdf              # into the notes' assets/, linked in the page
tlog undo [rel]                                  # the last thing tlog committed
```

Every write is committed to git by tlog itself. Text may be `-` to read it
from standard input, which is how a block of several lines goes in. Flags go
before or after the arguments; put `--` before text that starts with a dash.

## Before you write a date or a name

Dates are refused rather than guessed, so check what a shorthand means first:

```sh
tlog date fri -json        # [{"phrase":"friday","iso":"2026-11-06",...}]
```

Names make pages. A misspelt `[[Julia Rausenberg]]` is a new, empty person.
Ask who is meant before writing a mention:

```sh
tlog mentions jr -json     # [{"kind":"person","insert":"[[Julia Rausenberger]]",...}]
tlog people -json          # everyone tagged #person
```

Use the `insert` text as given. A deadline is written as a property the person
has configured (`tlog config deadline.property`), and `block … deadline` and
`add -due` already do that; do not write `Deadline::` by hand.

## Habits that keep the notes intact

- One operation per write, from the newest address. Do not plan five edits
  against one read; each write invalidates the addresses after it.
- Use `done` and `open`, not `task`: `task` toggles, so a retry undoes it.
- Prefer `tlog block … cmd` over typing `/todo` into text: the slash is a menu
  gesture, not syntax, and the command leaves nothing behind.
- Do not write `[x]`, `#tag`, `Key::` forms yourself when an operation exists
  for them; the operation also keeps the file canonical.
- Journals are days. Write to today unless the person names a page or a day.
- A 4 on `undo` means someone edited the file by hand since; leave it.

## References

- `references/commands.md` — every command and operation, with its flags and
  what it prints. Read it when you need something not shown above.
- `references/json.md` — the JSON shapes, field by field, with an example of
  each. Read it when you parse output or build a tool over tlog.
