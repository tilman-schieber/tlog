# Slash commands

A slash command **does something to the block you are in**, or to the page it
is on, and then disappears. What stays in your file is the effect, never the
slash.

Type `/` at the start of a word to open the menu. It is shown in four groups:

| Group | Commands | What they do |
|---|---|---|
| [Block](#block) | `/todo` `/done` `/plain` `/deadline` `/quote` `/prop` | change what the block is |
| [Insert](#insert) | `/code` `/table` | put a structure into it |
| [Attach](#attach) | `/file` `/paste` | bring something in from outside |
| [Page](#page) | `/alias` `/tagpage` | say something about the page |

## How the menu behaves

- **Keep typing to narrow it.** `/de` is enough for `/deadline`. `↑` `↓`
  choose; `enter` or `tab` runs the highlighted command; `esc` closes the menu
  and leaves the text as you typed it.
- **A command can go anywhere in the block.** `Call the supplier /todo` and
  `/todo Call the supplier` do the same.
- **What you type after a command stays.** `/todo call Ada` gives a task that
  says "call Ada".
- **Commands that need something ask for it.** Choosing *Deadline* from the
  menu completes it to `/deadline ` and shows the days; it does not run empty.
- **Once you type a space, the name must be exact.** `/dl fri` works because
  `dl` is another name for `deadline`. `/d fri` is just text. A slash inside a
  word or a web address is never a command.
- **Nothing is guessed.** A date tlog cannot read, or a file that is not there,
  gives a message and changes nothing.

Other names for a command are listed with it. The German ones are there
because the notes may be.

---

## Block

### /todo

Makes the block an open task: `☐`. On a finished task, reopens it.

Also: `/task`, `/aufgabe` · Keys: `x` while moving, `ctrl+d` while writing

### /done

Makes the block a finished task: `☑`.

Also: `/erledigt`

### /plain

Not a task any more. The checkbox goes, and so does the deadline — a dated
block is always a task, so the two leave together.

Also: `/untodo`, `/notask`, `/normal`

### /deadline

`/deadline fri` — the block is due on that day. It becomes a task if it was not
one. The date can be typed [in many ways](../concepts/dates.md) and is stored
as `Deadline:: 2026-11-20` under the block.

- Typing `/deadline ` shows the usual days to pick from.
- A second `/deadline` replaces the first.
- **`/deadline none`** takes the date off and leaves the task. (`off`, `-`,
  `kein`, `weg` work too.)

Also: `/due`, `/dl`, `/fällig`

### /quote

Makes the block a quotation: its text gets a `>` in front and is drawn as a
quote.

Also: `/zitat`

### /prop

`/prop status waiting` writes `status:: waiting` under the block. The first
word is the property's name, the rest its value. `/prop status` with no value
removes it.

Also: `/property`, `/eigenschaft`

→ Concepts: [Tasks, deadlines and the agenda](../concepts/tasks.md) ·
[Blocks and the outline](../concepts/blocks.md#properties)

---

## Insert

### /code

Puts a code block into the block, with the caret inside. Text typed after the
command goes into it: `/code ls -la`.

Write a language after the opening fence — ` ```go ` — to have it coloured.
Known: go, js/ts, python, sh, sql, json, yaml, toml.

### /table

Puts a `csv` block into the block. Type rows with commas and tlog draws a
table:

````text
```csv
Course, Semester, Credits
Bioanalytics, 3, 6
Chemistry, 1, 9
```
````

No pipes to line up. Semicolons and tabs work as separators too, so a pasted
spreadsheet export usually just works.

Also: `/tabelle`, `/csv`

---

## Attach

### /file

`/file ` and a few letters lists what is in the notes' `assets/`, newest first;
choosing one links it here. For files not yet attached, drop them on the
terminal or use `ctrl+v` — see [Attachments](../concepts/attachments.md).

Also: `/attach`, `/att`, `/anhang`

### /paste

Takes the **text on the clipboard** and keeps it as a markdown file of its own
in `assets/`, linked here by its first line. For a long email, minutes, an
answer you want to keep — anything too long to be a block.

(To paste text *into* the block, use your terminal's paste or `ctrl+v`.)

Also: `/note`, `/notiz`

---

## Page

These change the page the block is on, not the block.

### /alias

`/alias Ada` — this page also answers to "Ada". `[[Ada]]` and `@ada` now reach
it, and mentions under either name land in the same backlinks. Use it again to
add another name.

Also: `/aka`, `/zweitname`

### /tagpage

`/tagpage person` — says what this page is, by writing `tags: person` into the
page's header. The kind shows next to the page in the `@` menu, and the page is
listed on the tag's own page.

The same can be said from anywhere with `[[Ada Lovelace #person]]`.

Also: `/pagetag`, `/kind`

→ Concepts: [Journals and pages](../concepts/journals-and-pages.md#more-than-one-name) ·
[Tags](../concepts/tags.md#job-2-what-a-page-is)

---

## Not slash commands

Some things you might look for here are done another way:

| Looking for | Use |
|---|---|
| link a page | `@` or `[[` — [The four signs](signs.md) |
| insert a date | `@fri` |
| add a tag | `#topic` |
| agenda, backlinks, settings, help | `p` while moving, then type the name |
| delete a block | `dd` while moving |

From a script, every slash command is available as
`tlog block <address> cmd <name> [argument]`, and `tlog commands` lists them.
See [Scripts and agents](../more/scripting.md).
