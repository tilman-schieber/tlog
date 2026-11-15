# 6. Tasks and deadlines

Notes turn into work. In tlog a task is not a separate list somewhere — it is a
block, right where the thought came up, with a checkbox in front.

## Make a task

On today's journal, type:

```text
/todo Ask for a second quote
```

When you type `/`, a menu of commands opens. You can pick one with the arrows,
or just keep typing. Press `enter`:

```text
• ☐ Ask for a second quote
```

The `/todo` is gone and what you typed after it stayed. A **slash command**
does something to the block you are in and then disappears — what is left in
the file is its effect, never the slash.

You can also add it afterwards. Type a normal line, then ` /todo` at the end,
`enter`. Same result.

## Give it a deadline

Put the caret at the end of that block and type:

```text
 /deadline 
```

After the space, the menu turns into a list of days: today, tomorrow, the rest
of the week, next week. Pick one with the arrows, or type the way you would say
it — `fri`, `in 3 days`, `24.12.` — and press `enter`.

The block now shows its date beside it:

```text
• ☐ Ask for a second quote ⏰ Fr 20.11.
```

In the file it is a small property underneath the block:

```markdown
- [ ] Ask for a second quote
  Deadline:: 2026-11-20
```

Whatever you typed, the date is stored in one unambiguous form, so it still
means the same thing next year.

A deadline also makes the block a task, if it was not one already. A date on
something that can never be ticked off would stay on your list forever.

## Tick it off

Two ways:

- while writing: `ctrl+d`
- while moving (`esc` first): `x`

Or type `/done`. A finished task is never overdue, however long ago it was due.

## See everything that is open

Press `esc`, then `a`. This is the **agenda**: every open task in all your
notes. Tasks with a deadline come first, soonest on top — overdue in red, today
in yellow — and the undated ones follow under *No date*.

In the agenda:

| Key | What it does |
|---|---|
| `enter` | go to the block, in its page |
| `space` | tick it off without leaving |
| `a` | also show the dated tasks already done |
| `esc` | close |

The same list is available in a shell, which is handy for a status bar or a
morning routine:

```sh
tlog todo     # every open task
tlog due      # only the dated ones
```

## Changing your mind

| You want to | Type |
|---|---|
| take the deadline off but keep the task | `/deadline none` |
| make it an ordinary block again | `/plain` |
| reopen a finished task | `x` again, or `/todo` |

---

**What you just met**

- How tasks and dates fit together: [Tasks, deadlines and the agenda](../concepts/tasks.md)
- Every phrase a date understands: [Dates](../concepts/dates.md)
- The whole menu: [Slash commands](../reference/slash-commands.md)

Next: [Files, pictures and long text](07-files.md)
