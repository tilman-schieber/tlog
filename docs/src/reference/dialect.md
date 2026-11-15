# The markdown dialect

This page describes what a tlog file looks like inside. You never *need* to
know it — the outliner writes it for you — but it is the thing to read before
editing notes by hand or writing a script that produces them.

The dialect is small on purpose. Almost all of it is ordinary markdown, so any
person or program that reads markdown can read your notes.

## A whole file

````markdown
---
tags: person, colleague
aliases: Ada, Countess Lovelace
---

- a block
  - a child block
  - a block can span several lines,
    including a fenced code block:
    ```go
    x := 1
    ```
- [ ] an open task
- [x] a done one
- a block with properties
  status:: waiting
  Deadline:: 2026-11-20
- a link to [[Some Page]], a day [[2026-11-20]], and a #tag
- met [[Ada Lovelace #person]] about [[Analytical Engine #project]]
- a block that something points at ^k3f9q2
- a reference to it: [[Some Page#^k3f9q2]]
- > a quotation
- a picture ![plan](../assets/plan.png) and a file [report](../assets/report.pdf)
````

## The pieces

| Piece | Written as | Notes |
|---|---|---|
| block | `- text` | one list item |
| child | indented two spaces under its parent | |
| line inside a block | continuation line, indented to the text | |
| open task · done task | `- [ ] ` · `- [x] ` | |
| property | `key:: value` on a line under the block | the one piece that is not standard markdown |
| deadline | `Deadline:: 2026-11-20` | `due::` is read too; always an ISO date |
| page link | `[[Page Name]]` | |
| day link | `[[2026-11-20]]` | a link to that day's journal |
| tag on a block | `#topic` | letters, digits, `-`, `_`, `/`; starts with a letter |
| tag on a linked page | `[[Page #kind]]` | says what the *page* is |
| block marker | ` ^k3f9q2` at the end of the block | written only when something refers to the block |
| block reference | `[[Page#^k3f9q2]]` | |
| quotation | block text starting with `> ` | |
| heading | block text starting with `# ` | drawn bold |
| page header | `---` … `---` at the top | `tags:`, `aliases:`, `title:` |

## Inside a block

A block's text is ordinary markdown, and tlog draws it: `**bold**`,
`*italic*`, `` `code` ``, `~~struck~~`, numbered and bulleted lists, links
`[label](address)`, images `![alt](address)`, fenced code, and tables.

A fence marked `csv` (or `tsv`, `psv`) is drawn as a table, which is much less
work to type and edit than a pipe table:

````markdown
- course list
  ```csv
  Course, Semester, Credits
  Bioanalytics, 3, 6
  ```
````

Pipe tables are drawn as well.

## File names

| Kind | Path |
|---|---|
| a day | `journals/2026-11-15.md` |
| a page | `pages/<Page Name>.md` |
| an attachment | `assets/<file name>` — linked as `../assets/<file name>` |

A markdown file inside `assets/` is an attachment, not a page.

## What tlog does to a file it writes

- **It normalises once.** The first write puts the file into canonical form:
  two-space indentation, one spelling of a checkbox, properties under their
  block. After that, a change touches only what changed.
- **It makes dates ISO.** A deadline typed by hand as `due:: fri` becomes
  `due:: 2026-11-20`, pinned to the Friday it meant when the file was written.
- **It gives dated blocks a checkbox.** A block with a deadline is a task.
- **It never adds markers on its own.** A `^marker` appears only when you refer
  to a block.

## Editing by hand

Go ahead. Keep to the shapes above and tlog will read the file as if it had
written it. If tlog is open, it notices the change; if you both changed the
same file, tlog refuses to overwrite yours — see
[Notes are files](../concepts/files.md#editing-elsewhere-safely).
