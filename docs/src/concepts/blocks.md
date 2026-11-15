# Blocks and the outline

Everything you write in tlog is a **block**: one item in a list. A page is
nothing more than blocks, some of them nested under others.

```text
▾ Budget meeting with the dean
  ▾ We need a second quote for the lab equipment
    • Supplier A quoted 4,800
  • Next meeting in two weeks
• Talked to Ada about the grant
```

On screen every block has a bullet in front: `•` for a block on its own, `▾`
for one with blocks under it, and `▸` when those are folded away.

## Why blocks and not paragraphs

Because a block is a unit you can do things with. You can

- **move** it, with everything under it (`alt+↑` `alt+↓`),
- **nest** it under another (`tab`),
- **fold** it away (`ctrl+↑`),
- make it a **task** (`/todo`) and give it a **deadline**,
- **link** to it from somewhere else (`((`),
- and it shows up as a unit on the pages it mentions.

That last point is the one that shapes how you write.

## Parents and children

When a block mentions a page — `Talked to [[Ada Lovelace]]` — that block
appears on Ada's page, *together with its children*. So the natural shape of a
note is:

```text
▾ Call with [[Ada Lovelace]]            ← what it is, who it is with
  • Reviewer 2 wants more on methods    ← what was said
  • She sends the draft by Friday
```

One line to say what this is, the substance underneath. Later, on Ada's page,
you see the call and everything in it, without having filed anything.

A useful rule of thumb: **if you would say "about that…" before a line, it is a
child.**

## A block can be more than one line

`enter` starts a new block. `alt+enter` (or `ctrl+j`) makes a new line *inside*
the current one. That is how a block holds an address, a short list, a quote
or a piece of code:

````text
Fix for the import script
```sh
tlog import -dry-run
```
````

Inside a block, ordinary markdown works and is drawn properly: **bold**,
*italic*, `code`, numbered lists, quotations (`/quote`), code blocks (`/code`)
and tables (`/table`).

## Properties

A block can carry small named facts, written under it as `key:: value`:

```markdown
- [ ] Ask for a second quote
  Deadline:: 2026-11-20
  status:: waiting
```

The deadline is the one tlog itself understands. Others are yours to use —
`/prop status waiting` sets one, `/prop status` with no value removes it. They
stay out of the way on screen and are easy to search for.

## Folding is a view, not a fact

Folding a block hides its children on your screen. It is not saved in the
file, so your notes never contain "this was collapsed" markers, and a folded
block looks the same as any other to every other tool.

---

See it in the tutorial: [Shaping an outline](../tutorial/02-outline.md) ·
Reference: [Keys](../reference/keys.md)
