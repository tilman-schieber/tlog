# Start here

This tutorial follows one imaginary working day. You will take notes in a
meeting, link the people in it, turn a few lines into tasks with deadlines,
attach a file, and find everything again a week later.

Each step is short and builds on the one before. When a step introduces an idea
that has more to it, there is a link to the chapter that explains it — follow it
now or come back later; the tutorial does not depend on it.

## Before you start

You need tlog installed. With [Go](https://go.dev) and
[just](https://github.com/casey/just) on your machine:

```sh
git clone https://github.com/tilman-schieber/tlog
cd tlog
just install        # puts tlog into ~/.local/bin
```

or, without cloning:

```sh
go install github.com/tilman-schieber/tlog/cmd/tlog@latest
```

Check that it worked:

```sh
tlog version
```

Your notes will live in `~/notes`. If you would rather keep them elsewhere, set
`TLOG_DIR` first, or see [Settings](../reference/settings.md).

> **Trying it out without touching your real notes.** Every command takes
> `-dir`, so `tlog -dir /tmp/tryout` gives you a scratch notebook you can throw
> away afterwards.

## The steps

1. [Your first note](01-first-note.md) — open tlog and write
2. [Shaping an outline](02-outline.md) — nesting, moving and joining blocks
3. [Moving around](03-moving.md) — the two modes, days, and going to a page
4. [People, pages and days](04-linking.md) — `@` and `[[`
5. [Topics](05-topics.md) — `#`
6. [Tasks and deadlines](06-tasks.md) — `/todo`, `/deadline` and the agenda
7. [Files, pictures and long text](07-files.md) — paste, drop, `/file`, `/paste`
8. [Finding things again](08-finding.md) — search, backlinks, block references
9. [Keeping notes safe and in sync](09-sync.md) — undo, git, other editors
