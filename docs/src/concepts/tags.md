# Tags

A tag is a word with `#` in front: `#budget`. It does one of two jobs, depending
on where you write it. Keeping the two apart clears up most confusion about
tags.

## Job 1: what a block is about

A `#tag` in the text of a block files that block under a topic.

```text
Second quote for the lab equipment is 4,200 #budget
```

The block is now listed on the **budget** page. That is the whole effect.

Typing `#` and a letter or two offers the topics you already use; `tab` takes
the highlighted one. A new tag needs nothing — you just type it.

## Job 2: what a page is

A tag *on a page* says what kind of thing the page is: a person, a project, a
paper, a repository.

There are three ways to say it, all equivalent:

| Where | How |
|---|---|
| on the page itself | `/tagpage person` |
| in passing, while linking | `[[Ada Lovelace #person]]` |
| in the file's header | `tags: person, colleague` |

The second form is the handy one. The first time you mention someone you can
say what they are in the same breath, without leaving the line you are writing:

```text
Met [[Charles Babbage #person]] about [[Analytical Engine #project]]
```

What this gives you:

- The `@` menu shows the kind next to each name, so two pages with similar
  names are easy to tell apart.
- The **person** page lists every person; the **project** page every project.

## A tag is a page

There is no separate world of tags. `#budget` and `[[budget]]` are the same
page. It can have notes of its own, an alias, even a kind (`/tagpage topic`).

So the difference between `#budget` and `[[budget]]` in a block is only one of
tone: a tag reads as a label at the end of a line, a link reads as part of the
sentence. Both put the block on the page.

## Which sign when

| You mean | Write |
|---|---|
| this block mentions Ada | `@ada` → `[[Ada Lovelace]]` |
| this block belongs under "budget" | `#budget` |
| Ada is a person | `/tagpage person` on her page, or `[[Ada Lovelace #person]]` |

## Good tags

A few habits that keep tags useful:

- **Few and broad** for kinds: `person`, `project`, `paper`, `idea`.
- **Lower case, one word**, with `-` or `/` if you must: `#lab-move`,
  `#teaching/ws26`.
- If you are about to invent a tag you will use once, a link is probably what
  you want.

---

See it in the tutorial: [Topics](../tutorial/05-topics.md) ·
Reference: [The four signs](../reference/signs.md) ·
[`/tagpage`](../reference/slash-commands.md#tagpage)
