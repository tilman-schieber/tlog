# Dates

tlog has one rule about dates, with two halves:

> **Type them however you like. They are stored one way.**

Whatever you type — `fri`, `morgen`, `24.12.` — what lands in your file is
`2026-11-20`: year, month, day. That form sorts correctly, is searchable, and
means the same thing in five years. "Friday" does not.

## Where you type a date

| Where | Result |
|---|---|
| `@fri` in a block | a link to that day's journal: `[[2026-11-20]]` |
| `/deadline fri` | a deadline on the block |

Both open the same quick pick. With nothing typed yet it offers the usual
suspects — today, tomorrow, the rest of this week, next week, the end of the
month — and it narrows as you type. `enter` takes the highlighted day.

## What it understands

| Type | Means |
|---|---|
| `today` `tomorrow` `yesterday` | just that — also `tom` |
| `fri` `mo` `tu` `we` `th` `sa` `su` | the next such day |
| `next week` · `next month` | its Monday · its 1st |
| `in 3 days` · `in 2 weeks` · `in 1 month` | counted from today |
| `+3d` `+2w` `+1m` `+1y` | the same, shorter |
| `7 nov` · `nov 7` | a day in a named month |
| `24.12.` · `26.9.` | day and month, the German way |
| `2026-12-24` | the stored form itself |
| `eow` | end of the week — Friday or Sunday, [your choice](../reference/settings.md) |

German works everywhere a date is read: `heute`, `morgen`, `übermorgen`,
`nächsten freitag`, `3t`, `+1j`.

To check what a phrase means before using it:

```sh
tlog date "next friday"
```

## Nonsense is refused

If tlog cannot tell what day you mean, it says so and changes nothing. It never
guesses. A deadline that is silently wrong is worse than being asked to type it
again.

## Dates typed in another editor

If you write `Deadline:: fri` by hand in another editor, tlog converts it to
the stored form the next time it writes that file — pinned to the Friday it
meant on that day. Until then it is not treated as a date, because a relative
word sitting in a file would mean a different day every time it was read.

## Days are pages

Because a date in the text is a link to that day's journal, days collect
backlinks like any other page. Open next Friday's page and everything you
pointed at it is listed there. See
[Journals and pages](journals-and-pages.md#a-day-is-a-page-too).

---

See it in the tutorial: [People, pages and days](../tutorial/04-linking.md) ·
[Tasks and deadlines](../tutorial/06-tasks.md)
