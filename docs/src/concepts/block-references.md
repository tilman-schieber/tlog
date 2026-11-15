# Pointing at one block

A link to a page says "see Ada". Sometimes you need "see *that sentence*": the
decision that was taken, the figure that was agreed, the definition you wrote
down once and want to reuse.

## Making a reference

Type `((` and a word or two from the block you mean:

```text
As agreed: ((second quote
```

A menu lists matching blocks from all your notes — it is the same search as
`/` while moving, so anything you can find, you can point at. Choose one.

## What you see, and what is stored

On screen, the reference is drawn as **the text of the block it points at**:

```text
• As agreed: Ask for a second quote
```

In the file it is an ordinary link with a short code at the end:

```markdown
As agreed: [[2026-11-15#^k3f9q2]]
```

and the block you pointed at has received that code as a small marker:

```markdown
- [ ] Ask for a second quote ^k3f9q2
```

That marker is the block's permanent name. It is written at the moment you
first refer to the block, and not before — notes nobody has pointed into
contain no such markers at all. Once it is there, the reference keeps working
if the block is moved, edited or re-nested.

When your caret is in the referring block you see the stored form, like every
other piece of syntax: what you edit is what is in the file.

## Embeds

A block that contains *nothing but* a reference shows the whole original in its
place — the block **and its children**:

```text
▾ Decisions so far
  • ((budget meeting         ← alone in its block
```

draws as

```text
▾ Decisions so far
  ▾ Budget meeting with the dean
    • We need a second quote for the lab equipment
    • Next meeting in two weeks
```

There is no extra syntax for this. The rule is simply: a block that says
nothing else is showing you what it points at. The embedded rows belong to the
other page — click one or press `enter` on it to go there and edit.

## It is still a link

A block reference is a link to the page the block lives on, so it also appears
in that page's backlinks. Following it (`g`) goes to the exact block.

## When to use it

- Collecting decisions, quotes or open questions on a summary page without
  copying them.
- Answering an earlier note: "re: ((the question".
- Reusing a definition or an address in several places and keeping one
  original.

If you find yourself referring to the same block often, it may want to be a
page instead.

---

See it in the tutorial: [Finding things again](../tutorial/08-finding.md) ·
Reference: [The four signs](../reference/signs.md)
