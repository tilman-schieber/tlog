# 9. Keeping notes safe and in sync

You have been trusting tlog with your notes for eight steps. Here is why that
is reasonable.

## Every change is kept

Your notes folder is a [git](https://git-scm.com) repository. tlog commits to
it by itself, a few seconds after you stop writing. You never have to think
about it — but it means every version of every note is still there.

Day to day, you only need undo:

| Where | Undo | Redo |
|---|---|---|
| while writing | `ctrl+z` | `ctrl+y` |
| while moving | `u` | `U` |

Undo first takes back your typing, a word at a time, then each change that was
written. If the last change was on another page, undo takes you there so you
can see what it did.

Even `dd`, which deletes a block and everything under it, is only an undo away
— and if you notice a week later, the old version is still in git.

## A second machine

Give your notes a place to go — any git remote you can push to:

```sh
tlog config git.remote git@github.com:you/notes.git
tlog push
```

On another machine, clone that repository as `~/notes` and you have the same
notebook there. Then:

```sh
tlog pull     # bring in what the other machine wrote
tlog push     # send what this one wrote
tlog sync     # just tell me where things stand
```

If you would rather not think about it:

```sh
tlog push -auto on
```

Now every commit is pushed in the background. If a push ever fails — you are
on a train — your notes are still saved and committed; tlog tells you "not
pushed" in the status line, and the next push sends it along.

> Use a **private** repository. Your notes are yours.

## Other editors are welcome

Because a note is a markdown file, you can open it in anything:

```sh
nvim "$(tlog today)"
```

If tlog is open at the same time, it notices the change and shows it. And if
you and another program change the same file at the same moment, tlog refuses
to overwrite and tells you, instead of silently keeping one version. Press
`ctrl+r` to reload.

## Capturing without opening tlog

A thought while you are in the middle of something else:

```sh
tlog add "Ask Ada about the reviewer comments"
tlog add -todo -due fri "Send the budget table"
```

Both land on today's journal. Bind that to a key in your launcher and you have
a capture box.

## You are done

That is all of tlog's everyday use. To recap the whole thing:

- **Write** in today's journal; `enter` for the next block, `tab` to nest.
- **`@`** finds a person, page or day and links it.
- **`#`** files a block under a topic.
- **`/`** changes the block: `/todo`, `/deadline fri`, `/paste`…
- **`esc`** to move around, `p` to go somewhere, `a` for what is due.

From here, the *Concepts* chapters go deeper on each idea, and the *Reference*
is there when you need to look something up.

---

**What you just met**

- What git does for you here: [History, undo and pushing](../concepts/git.md)
- Why other editors are safe: [Notes are files](../concepts/files.md)
- Everything the shell can do: [The command line](../reference/cli.md)
