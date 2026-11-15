# 3. Moving around

So far every key you pressed typed something. To get around quickly, tlog has a
second mode where letters are commands instead.

## Writing and moving

Press `esc`. The block you were in is highlighted and the header says **MOVE**.
Nothing is lost — leaving a block saves it.

Now single letters do things:

| Key | What it does |
|---|---|
| `j` `k` or `↓` `↑` | go from block to block |
| `enter` or `i` | start writing in this block again |
| `x` | tick a task off (more in [step 6](06-tasks.md)) |
| `dd` | delete the block and everything under it |
| `z` | fold or unfold |
| `?` | the help page |
| `q` | quit |

Press `enter` and you are writing again. That is the rhythm: `esc` to put the
caret down, a letter or two to get somewhere, `enter` to pick it back up.

> If you never want to leave writing mode, you do not have to. Everything
> important also has a `ctrl` key that works while typing — they are listed
> under [Keys](../reference/keys.md).

## From day to day

While moving:

| Key | Goes to |
|---|---|
| `t` | today |
| `[` | the day before |
| `]` | the day after |
| `b` | back to wherever you just came from |

Try `[` — yesterday's journal is empty, and that is fine. A day's file is only
created once you write something in it. Press `t` to return.

## Going to a page

Press `p`. A list opens: the pages you visited recently, then today, a handful
of commands, and every journal and page you have. Type a few letters to narrow
it and press `enter`.

Type a name that does not exist yet — say `Grant proposal` — and `enter` takes
you to a new, empty page with that name. That is how pages are made: by going
to them, or, more often, by [linking to them](04-linking.md).

Press `b` to go back to today.

`p` is also where the less common things live: **Agenda**, **Backlinks**,
**Settings**, **Messages**, **Help**. Type the first letters of the name.

---

**What you just met**

- Daily pages and named pages: [Journals and pages](../concepts/journals-and-pages.md)
- The full list: [Keys](../reference/keys.md)

Next: [People, pages and days](04-linking.md)
