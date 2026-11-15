# 2. Shaping an outline

You already made an outline in the first step, by pressing `tab` once. This
step is about why that matters, and how to reshape one after the fact.

## Nesting

```text
▾ Budget meeting with the dean
  • We need a second quote for the lab equipment
  • Next meeting in two weeks
```

The two indented lines sit *under* the meeting. They are its children: they
move with it, fold away with it, and when the meeting block shows up somewhere
else later (it will, in [step 8](08-finding.md)), they come along.

`tab` and `shift+tab` work on a block at any time, not only on an empty one,
and you can go as deep as you like. Put the caret at the end of the "second
quote" line, press `enter`, then `tab`, and type:

```text
▾ Budget meeting with the dean
  ▾ We need a second quote for the lab equipment
    • Supplier A quoted 4,800
  • Next meeting in two weeks
```

A detail about a detail. `shift+tab` would move it back out one level.

A good habit: **one block for the thing, children for what was said about it.**
A meeting, a phone call, a paper you are reading — each is a block, and the
details go underneath.

## Rearranging

| Key | What it does |
|---|---|
| `tab` / `shift+tab` | indent, outdent |
| `alt+↑` / `alt+↓` | move the block up or down, children included |
| `enter` in the middle of a line | split the block in two at the caret |
| `backspace` at the very start of a block | join it onto the block above |

Try it: put the caret on "Next meeting in two weeks" and press `alt+↑`. It
moves above the "second quote" block — and stays inside the meeting. Press
`alt+↓` to put it back.

An empty block disappears when you press `backspace` in it, so there is no
"delete line" to learn for the common case.

## Folding

The `▾` in front of a block with children is also a handle. `ctrl+↑` folds the
block to a single line, `ctrl+↓` opens it again; clicking the triangle does the
same.

```text
▸ Budget meeting with the dean
```

`▸` says there is more inside.

Folding is only how *you* see the page right now. Nothing about it is written
into the file.

## Made a mistake?

`ctrl+z` undoes, `ctrl+y` redoes — first your typing, a word at a time, and
then each change that was written, including indents and moves.

---

**What you just met**

- Why nesting matters: [Blocks and the outline](../concepts/blocks.md)
- Every key for writing: [Keys](../reference/keys.md)

Next: [Moving around](03-moving.md)
