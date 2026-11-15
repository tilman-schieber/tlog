# 8. Finding things again

A week has passed. You remember there was something about a quote for lab
equipment, and that Ada was going to send a draft. Here are the four ways back.

## 1. Go to the page

If you know *who* or *what* it was about, go there: `p`, a few letters,
`enter`.

Ada's page lists every block that mentions her, each with its children — which
is why it pays to put details *under* the block that names the person. A
mention is usually short; the substance is underneath.

Move onto one of those blocks and press `enter` to jump to it in the journal
where you wrote it.

## 2. Search

Press `/` while moving (or `ctrl+f` at any time) and type a word. Every block
in your notes that contains it is listed as you type; `enter` goes there.

> While *writing*, `/` opens the command menu instead. While *moving* it
> searches. If in doubt, `ctrl+f` always searches.

## 3. Look at the topic

Everything you tagged `#budget` is on the **budget** page. `p`, `budget`,
`enter`.

## 4. Look at the day

If you remember *when*, walk there: `[` and `]` step through days, and `p`
accepts a date. A day's page shows what you wrote that day and, below it,
everything from other days that pointed at it with `@fri` or a similar phrase.

## Pointing at one particular block

Sometimes a page is too coarse: you want to refer to *that one line* — the
decision, the number, the phone number.

In a block, type `((` and a word from the block you mean:

```text
As agreed: ((second quote
```

A menu shows matching blocks from all your notes. Pick one and the `((…`
becomes a link to exactly that block. On screen it is drawn as *the text of the
block it points at*, so the line reads naturally and stays up to date if the
original is edited.

A block that contains nothing but such a reference shows the whole original,
children and all, in place — an **embed**.

## Back and forth

`b` (or `ctrl+o`) goes back to where you came from, as often as you like. With
`g` to follow links and `b` to return, you can wander through your notes the
way you click through a website.

---

**What you just met**

- Why a page knows what links to it: [Links and backlinks](../concepts/links.md)
- References and embeds: [Pointing at one block](../concepts/block-references.md)

Next: [Keeping notes safe and in sync](09-sync.md)
