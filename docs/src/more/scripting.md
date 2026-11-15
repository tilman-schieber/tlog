# Scripts and agents

Everything the outliner can do is also a command, so a shell script, a status
bar or an AI assistant can work with your notes the same way you do — and with
the same safety.

This chapter is the overview. The complete reference for agents lives in the
repository under
[`skills/tlog/`](https://github.com/tilman-schieber/tlog/tree/main/skills/tlog).

## The idea: read an address, write with it

Commands that read hand back an **address** for each block. Commands that
write take one.

```text
pages/Grant proposal.md:42@9f3a2b1c8d4e5f60
└──── the file ───────┘ │  └── a fingerprint of the file as it was read
                        └ where the block starts
```

The fingerprint is the point. If the file has changed since you read it — you
typed something in the outliner meanwhile — a write with the old address is
**refused**. It is never applied to whatever now happens to sit at that
position. A script cannot silently damage a note.

Every write prints the block's *new* address, so one step chains into the next:

```sh
addr=$(tlog search "plan the lecture" -json | jq -r '.[0].addr')
addr=$(tlog block "$addr" open)                  # make it a task
addr=$(tlog block "$addr" deadline 2026-12-01)   # now it is on the agenda
        tlog block "$addr" after -child "book the room"
```

## Reading

| Command | Gives you |
|---|---|
| `tlog search <words>` | matching blocks, each with an address |
| `tlog view <page> -addr` | a page, with an address per block |
| `tlog view -day yesterday` | a journal by day: `today`, `-1`, `fri`, `2026-11-03` |
| `tlog todo` · `tlog due` | open tasks, addressable |
| `tlog block <addr>` | one block: text, properties, deadline |
| `tlog links <addr>` | where a block's links go |
| `tlog index` | every journal, page and tag |
| `tlog places <query>` | the same, ranked against a query |
| `tlog mentions <letters>` | who or what a few letters could mean — what `@` would offer |

Add `-json` to any of them. An empty result is `[]`.

## Writing

| Command | Does |
|---|---|
| `tlog add [-p page] <text>` | a new block on today's journal or a page |
| `tlog append <file> [-if-match hash] <text>` | a block at the end of a file |
| `tlog block <addr> text <text>` | replace a block's text |
| `tlog block <addr> after [-child] <text>` | a block below it, or as its first child |
| `tlog block <addr> before <text>` | a block above it |
| `tlog block <addr> indent` · `outdent` · `up` · `down` | restructure |
| `tlog block <addr> open` · `done` | make it an open or a finished task |
| `tlog block <addr> deadline <date>` | set a deadline; `none` removes it |
| `tlog block <addr> prop <key> [value]` | set or remove a property |
| `tlog block <addr> cmd <name> [arg]` | run any [slash command](../reference/slash-commands.md) |
| `tlog block <addr> ref` | give it a permanent marker and print a link to it |
| `tlog block <addr> delete` | remove it and everything under it |

`tlog block help` lists them all; `tlog commands` lists the slash commands.

Prefer `open` and `done` over `task` (which toggles): if a script is run twice,
they land in the same place both times.

## Exit codes

The exit code says which kind of "no" it was, so a script knows whether trying
again makes sense.

| Code | Meaning | Try again? |
|---|---|---|
| `0` | done | |
| `1` | something went wrong | |
| `2` | not a command, an operation or an address | no — fix the call |
| `3` | that does not exist: a missing page, an unreadable date | no |
| `4` | the file changed since you read it | **yes** — read again, then retry |

## Examples

A capture key for your launcher:

```sh
tlog add -todo "$(wofi --dmenu --prompt 'todo')"
```

What is due today, for a status bar:

```sh
tlog due -json | jq '[.[] | select(.state == "today" or .state == "overdue")] | length'
```

Append the output of a command to a project page:

```sh
git log --oneline -5 | tlog add -p "tlog" -
```

## AI assistants

Because notes are plain markdown and the command line is complete, an
assistant such as Claude Code can read and maintain your notes without any
special integration. The repository ships a *skill* that teaches it the
read-then-write loop, the exit codes, and the habits that keep notes intact —
check a date with `tlog date` before writing it, ask `tlog mentions` before
writing a name, never edit the files directly.

```sh
just skill-install     # links the skill into ~/.claude/skills
```

After that, "add a task to my notes to call the supplier by Friday" does what
you would expect.

## The Omarchy bar

If you use [Omarchy](https://omarchy.org), the repository contains a bar widget
built on exactly these commands: a count of open tasks, a list to tick them
off, and a capture field. See
[`omarchy/README.md`](https://github.com/tilman-schieber/tlog/blob/main/omarchy/README.md).
