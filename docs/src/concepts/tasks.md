# Tasks, deadlines and the agenda

A task in tlog is a block with a checkbox. It lives where you wrote it — in the
meeting notes, under the phone call — and not in a separate list. The list is
something tlog builds *from* your notes whenever you ask.

## Three states of a block

| State | Looks like | In the file |
|---|---|---|
| plain | `• Ask for a second quote` | `- Ask for a second quote` |
| open task | `• ☐ Ask for a second quote` | `- [ ] Ask for a second quote` |
| done | `• ☑ Ask for a second quote` | `- [x] Ask for a second quote` |

Moving between them:

| To | Type | or press |
|---|---|---|
| make it a task | `/todo` | `x` while moving, `ctrl+d` while writing |
| finish it | `/done` | `x` · `ctrl+d` |
| reopen it | `/todo` | `x` · `ctrl+d` |
| make it plain again | `/plain` | |

## Deadlines

`/deadline fri` gives a block a due date. You can type the date
[any way you would say it](dates.md); it is always written as `2026-11-20`.

Three things follow from a deadline:

- **It makes the block a task.** A date on something that can never be ticked
  off would sit on your list forever, so a dated block always has a checkbox.
- **It shows beside the block**, coloured by how soon it is: red when overdue,
  yellow when it is today.
- **It puts the block on the agenda**, sorted by date.

`/deadline` again replaces the date. `/deadline none` removes it and leaves the
task. `/plain` removes both.

A finished task is never overdue.

## The agenda

`a` while moving (or **Agenda** in `p`) shows every open task across all your
notes:

1. the dated ones, soonest first,
2. then the rest, under *No date*.

`enter` goes to a task where it was written, so you see its context; `space`
ticks it off on the spot; `a` again adds the dated tasks that are already done.

From a shell:

```sh
tlog todo        # the same list
tlog due         # only what has a date
tlog due -all    # including what is done
```

## Why tasks stay in place

It is tempting to want one tidy to-do list. But a task torn from its context
loses most of its meaning: "ask for a second quote" — for what, agreed with
whom, by when?

Left where it was written, the task keeps its parent ("Budget meeting with the
dean"), its siblings and its links. The agenda gives you the tidy list; `enter`
gives you the context back.

## Deadline or a date in the text?

They look similar and do different things.

| | What it is | Use it for |
|---|---|---|
| `/deadline fri` | a property of the block; makes it a task; appears on the agenda | something *you* must have done by then |
| `@fri` | a link to Friday's page, in the text | something that *happens* then, or that you want to see again that day |

"Send the budget table" gets a deadline. "Ada is away from @fri" gets a date.

---

See it in the tutorial: [Tasks and deadlines](../tutorial/06-tasks.md) ·
Reference: [Slash commands](../reference/slash-commands.md#block)
