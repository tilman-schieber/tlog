# Links and backlinks

A link is a page's name in double brackets:

```text
• Talked to [[Ada Lovelace]] about the [[Grant proposal]]
```

That is all it is — text. But it does two things at once.

## A link goes both ways

Following the link (`g`, or a click) takes you from the block to the page.

And without you doing anything, the page gets a **backlink**: the block is
listed on Ada's page, and on the grant's page. A link you write once can be
followed in both directions.

This is what makes writing in the [journal](journals-and-pages.md) work. You
never copy a note onto the pages it concerns. You mention them, and each page
gathers its own material.

## What a page shows

Below a page's own outline is a list of every block that mentions it. Each
comes **with its children**, because the mention itself is usually just a name
and the content is underneath:

```text
▾ Call with [[Ada Lovelace]]
  • Reviewer 2 wants more on methods
  • She sends the draft by Friday
```

Those rows belong to the day they were written on. They are shown, not copied:
move onto one and press `enter` to go to the original and edit it there.

## Three ways to type a link

They produce exactly the same thing in the file. Use whichever is quickest.

| Type | Best for |
|---|---|
| `@` | finding someone or something fast — it ranks by how recently and often you write about them, shows each page's tags, understands initials (`@al`), and also finds [days](dates.md) |
| `[[` | when you know the name and think in brackets; the `]]` closes itself |
| by hand | `[[Any Name]]` typed out works too — it is only text |

The `@` disappears when you choose; it is a way of *finding*, not something
that is stored.

## Links to pages that do not exist

Linking to a name nobody has used yet is not an error. The link works, the page
exists as "everything that mentions it", and it gets a file the first time you
write on it. You can sprinkle `[[names]]` freely and decide later which ones
deserve notes of their own.

## Other names

If a page has an [alias](journals-and-pages.md#more-than-one-name), linking the
alias reaches the same page and lands in the same backlinks.

## Links to other things

- `[[2026-11-20]]` — a day. Type `@fri`.
- `#budget` — a topic; see [Tags](tags.md).
- `[[Page#^k3f9q2]]` — one particular block; see
  [Pointing at one block](block-references.md).
- `[label](https://example.org)` or a bare web address — the outside world.
- `[report.pdf](../assets/report.pdf)` — an [attachment](attachments.md).

---

See it in the tutorial: [People, pages and days](../tutorial/04-linking.md) ·
[Finding things again](../tutorial/08-finding.md) ·
Reference: [The four signs](../reference/signs.md)
