# 5. Topics

`@` links to a *thing* — a person, a project, a day. Sometimes you just want to
say what a note is *about*, the way you would put a label on a folder. That is
what `#` is for.

## File a block under a topic

Add a block to today's journal:

```text
Second quote for the lab equipment is 4,200 #budget
```

Nothing visible happens — a tag is complete the moment you type it. But there
is now a page called **budget**, and this block is listed on it.

Go and look: `esc`, then `p`, type `budget`, `enter`. There is your block.
Press `b` to come back.

The next time you type `#b`, a menu offers `#budget`. Press `tab` to take it.
`enter` keeps its usual meaning — it ends the block — so a brand-new tag at the
end of a line is never swapped for an old one by accident.

Several tags at the end of a line read like hashtags:

```text
Second quote for the lab equipment is 4,200 #budget #lab
```

## A tag is just a page

`#budget` and `[[budget]]` point at the same page. There is no separate "tag
system" to keep tidy: the budget page can hold notes of its own, be linked
with `@`, and it shows both the blocks tagged `#budget` and the ones that
mention it.

So when do you use which?

| Use | When | Example |
|---|---|---|
| `@` / `[[ ]]` | the block *talks about* this thing | `Talked to [[Ada Lovelace]]` |
| `#` | the block *belongs under* this heading | `…is 4,200 #budget` |

If you are unsure, it does not matter much. Both end up on the page.

## Saying what a page *is*

There is one more use for tags, and it is worth learning early: saying what
kind of thing a page is.

Go to Ada's page (`p`, `ada`, `enter`), start writing in the empty block, and
type:

```text
/tagpage person
```

Press `enter`. The command disappears; the page now carries the tag `person`.
From now on the `@` menu shows *person* next to Ada's name — useful the day
you also know an Ada who is a programming language — and the **person** page
lists everyone you have marked this way.

You can also say it in passing, the first time you mention someone, by putting
the tag inside the brackets:

```text
Met [[Charles Babbage #person]] at the workshop
```

That reads "Charles Babbage, who is a person". You never have to visit his
page to declare it.

---

**What you just met**

- Topics, kinds, and how they differ: [Tags](../concepts/tags.md)
- `/tagpage` is a slash command — there are more: [Slash commands](../reference/slash-commands.md)

Next: [Tasks and deadlines](06-tasks.md)
