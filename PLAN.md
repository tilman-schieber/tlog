# Plan: what is not built yet

`DECISIONS.md` records what is settled. This file is what remains, and what was
looked at and left alone.

## Already built

Quotes, ordered and bulleted lists inside a block, fenced and inline code as
monospace, syntax highlighting, `csv` fences drawn as tables, pipe tables,
headings, horizontal rules, images, and linking for bare URLs and the `file://`
paths `att` writes.

Every one of those is **rendering only**. Nothing about how a file is stored
changed, so the round-trip invariant never moved and no existing note was
touched. The counts below are from the real corpus, rendered through the actual
parser: 550 blocks, nothing thrown.

| construct | blocks |
|---|---|
| links (wiki, markdown, bare, `file://`) | 109 |
| headings | 34 |
| horizontal rules | 12 |
| tables | 9 |
| ordered lists | 6 |
| quotes | 5 |
| fenced code | 1 |

Two of those numbers began as wrong guesses, which is the argument for measuring:
"0 headings" was a bad regex — they are written as `- # Heading`, the bullet's
own text, so `^\s*#` never matched. And "68 horizontal rules" was 56 frontmatter
delimiters plus 12 real ones.

## Syntax highlighting — done

`internal/markdown/highlight.go` is a generic scanner driven by a per-language
table: line and block comments, string quotes, keywords and type words. Adding a
language is a table entry, not code. It covers go, js/ts, python, sh, sql, json,
yaml and toml, and an unknown language yields one plain token, which renders as
monospace — exactly what happened before.

It lives in Go rather than JavaScript so that the outliner, the desktop app and
a future `--json` caller all colour code the same way from one implementation.
`chroma` was not used: two hundred languages against one code fence in the
corpus would roughly double the 8.4 MB bundle.

The invariant worth keeping: **the tokens must reassemble into exactly the input**,
whatever the language and however malformed the snippet. A highlighter that
drops a character silently corrupts what the reader sees, and notes are full of
half-pasted code. That is the first test in the file.

## `csv` fences — done

Pipe tables are miserable to type and worse to edit. A fence is not:

    ```csv
    Kürzel, Studiengang, ECTS
    BZ, Bioanalytik und Zellbiologie, 180
    ```

The separator is detected rather than assumed, so a spreadsheet exported on a
German-locale machine works without anyone caring. `tsv` and `psv` are honoured
explicitly, quoted fields keep their commas, and ragged rows are padded, because
a table being typed is ragged for most of its life. A body that will not parse
falls back to a code block rather than disappearing.

It is still ordinary markdown: anything that does not know about tlog shows a
code block, which is a readable thing to show.

---

## The one remaining item: numbered outline blocks

A block that is itself numbered rather than bulleted:

```markdown
1. first step
2. second step
   - a note about it
```

Numbered lines *inside* a block already render as an ordered list, which covers
every numbered item in the corpus. This is only for wanting the outline bullet
itself to be a number.

- `internal/markdown/doc.go` — `Block.Ordered bool`.
- `internal/markdown/parse.go` — accept `N.` and `N)` as bullet markers. The
  marker is three or more characters rather than two, so the continuation-line
  dedent must key off the *content* column instead of assuming a fixed width.
  **This is the one genuinely delicate change in this document**, because that
  dedent is what multi-line blocks and code fences both depend on.
- `internal/markdown/render.go` — emit `N.` from the position within the run of
  consecutive ordered siblings, so inserting in the middle renumbers correctly.
  A hand-written `3.` at the head of a run normalises to `1.`, consistent with
  tlog owning the format, but it does mean the first write changes such a file.
- `internal/app/mutate.go` — `SetOrdered(addr, bool)`.
- Both adapters — a key to toggle it, and `1.` drawn in place of the bullet.
- `internal/markdown/render_test.go` — round-trip cases for ordered blocks,
  mixed ordered and bulleted siblings, and ordered blocks with children.

**Cost:** two days, most of it parser and tests. **Risk:** moderate — the first
change to the bullet grammar since V0.1, and the only item here that changes how
an existing file is stored.

---

## Considered and left alone

Measured rather than guessed:

- **Footnotes, definition lists, admonitions.** Zero occurrences, and each wants
  its own syntax. The dialect stays small on purpose.
- **`==highlight==`.** Zero occurrences, and an Obsidian-ism rather than
  markdown.
- **Nested lists inside a block's text.** The outline already nests. A second
  nesting mechanism would be two ways to say one thing, and they would disagree.
- **Transclusion and embeds.** Real value, but block references still have no UI
  for creating them, so there is nothing to embed by hand yet.
- **A `/query` command.** Logseq has one and it is the door to a whole query
  language. `tlog due` covers the one query these notes actually want; the rest
  is `rg`.
- **Recurring deadlines.** Nothing in the corpus repeats, and a repeat rule is a
  syntax, a scheduler and a "what does it mean to tick off one instance" problem
  all at once.
- **Auto-pull.** Pushing is automatic; pulling is not, and should stay that way.
  A pull can conflict, and resolving a conflict in someone's notes behind their
  back is the same mistake as merging a lost write instead of refusing it.
- **A WYSIWYG editor.** The block under the caret shows raw text deliberately:
  the caret has to land where the characters are, and what you type is what is
  stored. Hiding the syntax while editing it is how round-trip guarantees die.
