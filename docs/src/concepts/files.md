# Notes are files

The most important thing about tlog is something it does *not* have: a
database. Your notes are markdown files in a folder, and that folder is the
whole truth.

```text
~/notes/
├── journals/     one file per day:  2026-11-15.md
├── pages/        one file per page: Ada Lovelace.md
└── assets/       pictures and files you attached
```

## What that buys you

**You are never locked in.** If tlog disappeared tomorrow, you would still have
a folder of readable text files. Any editor opens them; any tool searches them.

```sh
cat ~/notes/journals/2026-11-15.md
rg 'lab equipment' ~/notes
nvim "$(tlog open 'Ada Lovelace')"
```

**Other programs can join in.** A script can append to today's journal. An AI
assistant can read your notes without learning a special format. Your backup
tool sees ordinary files.

**Nothing runs in the background.** There is no server and no sync daemon. When
tlog is closed, nothing of it is running.

## What a note looks like inside

```markdown
- Budget meeting with the dean
  - We need a second quote for the lab equipment #budget
  - [ ] Ask for a second quote
    Deadline:: 2026-11-20
- Talked to [[Ada Lovelace]] about the grant proposal
```

A block is a list item. A child is an indented list item. A task has a
checkbox. A link is a name in double brackets. The one thing that is not
everyday markdown is the `Key:: value` line under a block, which is how a block
carries a property such as its deadline. The full description is in
[The markdown dialect](../reference/dialect.md).

## Editing elsewhere, safely

You can change a note in another editor while tlog is open. tlog checks the
folder regularly and shows what changed.

The dangerous case is two programs changing the *same* file at the *same*
moment. tlog handles it the careful way: before it writes a file, it checks
that the file is still what it read. If something else got there first, tlog
**refuses to write** and says so, instead of overwriting. You reload
(`ctrl+r`), see the other change, and carry on.

A notes tool silently losing a paragraph is the worst thing it can do, so this
rule has no exceptions.

## tlog tidies what it writes

The first time tlog writes to a file, it puts it into one consistent form —
consistent indentation, one way of writing a checkbox. After that, a change you
make touches only the lines you changed. That keeps the history readable: the
[git log](git.md) of your notes shows what you did, not reformatting noise.

## Where the folder is

`~/notes` unless you say otherwise. `TLOG_DIR` in your environment, the `notes`
[setting](../reference/settings.md), or `-dir` on any command changes it.

---

See it in the tutorial: [Your first note](../tutorial/01-first-note.md) ·
[Keeping notes safe and in sync](../tutorial/09-sync.md)
