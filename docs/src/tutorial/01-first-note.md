# 1. Your first note

Open a terminal and run:

```sh
tlog
```

You are looking at **today's journal** — a page named after today's date. The
caret is already blinking in an empty block. There is nothing to click and no
"new note" button: tlog opens ready to write.

Type a line:

```text
Budget meeting with the dean
```

Press `enter`. That line is now a **block**, and you are in a new, empty one
below it. tlog draws a small bullet in front of every block, so you can always
see where one ends and the next begins:

```text
• Budget meeting with the dean
• 
```

What comes next is not a new subject, though — it is what was said *in* the
meeting. So before typing, press `tab`. The empty block moves in, under the
meeting. Now type, pressing `enter` after each line:

```text
▾ Budget meeting with the dean
  • We need a second quote for the lab equipment
  • Next meeting in two weeks
```

Two things to notice. The third line stayed indented by itself: `enter` gives
you a new block at the same level as the one you were in. And the meeting's
bullet turned into a `▾`, which is how tlog marks a block that has others
under it.

That is the whole basic loop: a line for what this is, `enter`, `tab`, and then
the details underneath — type, `enter`, type, `enter`.

When the meeting is over and the next thing comes along, press `shift+tab` in
an empty block to come back out to the left edge.

## Where did it go?

It is already saved. Leaving a block writes it to disk, and you can see the
result from another terminal:

```sh
cat "$(tlog today)"
```

```markdown
- Budget meeting with the dean
  - We need a second quote for the lab equipment
  - Next meeting in two weeks
```

A plain markdown list, nested the way you nested it, in a file named after the
day. That file *is* your note;
tlog is one way of looking at it.

## Leaving

Press `ctrl+q` to quit. Run `tlog` again and you are back where you were.

## A line break inside a block

`enter` always starts a new block. When you want a second line *inside* the
same block — an address, a short poem, a code snippet — press `alt+enter`
(or `ctrl+j`).

---

**What you just met**

- Each day has its own page: [Journals and pages](../concepts/journals-and-pages.md)
- A line is a block, and blocks nest: [Blocks and the outline](../concepts/blocks.md)
- It is all plain files: [Notes are files](../concepts/files.md)

Next: [Shaping an outline](02-outline.md)
