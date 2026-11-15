# 4. People, pages and days

This is the step that turns a diary into a notebook that knows things.

## Mention someone

Go back to today (`t`) and start a new block. If it is still indented under the
meeting, `shift+tab` brings it out to the left edge. Type:

```text
Talked to @Ada Lovelace
```

As soon as you type `@`, a small menu opens under the block. Nobody called Ada
exists in your notes yet, so it offers **new page: Ada Lovelace**. Press
`enter`.

The `@` is gone and the line now reads:

```text
• Talked to [[Ada Lovelace]]
```

Those double brackets are a **link**. `@` was only the way to find what to link
— what is written in your file is always the plain `[[name]]`.

Finish the thought:

```text
• Talked to [[Ada Lovelace]] about the grant proposal
```

Tomorrow, when you type `@ad`, Ada will be at the top of the menu. The menu
ranks pages by how recently and how often you write about them, so the people
you actually deal with are always one or two letters away. `@al` finds her too
— initials work.

## Visit her page

Press `esc`, move onto that block, and press `g` (for *go*). You are on the
page **Ada Lovelace**.

You never created this page, and its own outline is empty — but below it there
is a section listing the block you just wrote on today's journal. These are
**backlinks**: everything, anywhere in your notes, that mentions this page.

This is the central idea of tlog:

> You write in the journal, in the order things happen. Pages assemble
> themselves from what you wrote.

You do not file notes under "Ada". You mention Ada, and her page fills up.

Press `b` to go back.

## Mention a day

`@` finds days as well as pages. Type:

```text
She will send the draft @fri
```

The menu shows Friday with its date. `enter` writes it as a link to that day's
journal:

```text
• She will send the draft [[2026-11-20]]
```

When Friday comes and you open that day's page, this line is waiting in its
backlinks. A note to your future self needs nothing more than `@` and a day.

You can type days the way you would say them: `@tomorrow`, `@next week`,
`@in 3 days`, `@24.12.`, and German works too (`@morgen`).

## If you think in brackets

Typing `[[` opens the same list of pages, and the closing `]]` appears by
itself. Use whichever your fingers prefer; the result in the file is identical.

---

**What you just met**

- Why pages fill themselves: [Links and backlinks](../concepts/links.md)
- Every way to type a day: [Dates](../concepts/dates.md)
- `@` and `[[` side by side: [The four signs](../reference/signs.md)

Next: [Topics](05-topics.md)
