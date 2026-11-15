# Coming from Logseq

tlog will feel familiar: daily journals, an outline of blocks, `[[links]]`,
backlinks, `((block references))`, slash commands. It differs mainly in being
a terminal program over plain files, and in being much smaller.

## Importing your graph

```sh
tlog import -dry-run -from ~/logseq    # report what would happen, write nothing
tlog import -from ~/logseq             # write into the notes folder, then commit
```

**Your Logseq graph is only read.** tlog cannot change, move or delete it, and
the import refuses to run if the destination overlaps the source. You can
repeat it as often as you like and keep using Logseq until you are sure.

The conversion is one-way: notes you then write in tlog do not flow back.

## What is translated

| In Logseq | In tlog |
|---|---|
| `journals/2026_11_15.md` | `journals/2026-11-15.md` |
| `TODO` `LATER` `DOING` | `- [ ]` |
| `DONE` `CANCELED` | `- [x]` |
| page properties at the top | the page header (`tags:`, `aliases:`…) |
| block ids | `^markers` |
| `((uuid))` | `[[Page#^marker]]` |
| `Nov 15th, 2026` | `2026-11-15` |
| `collapsed::` and other interface state | dropped |
| pages containing only an id | not created |
| page classes (DB version) | `tags:` in the page header |

References whose target no longer exists are left exactly as written and listed
in the report, not guessed at.

> **Page classes need `sqlite3`.** Logseq's database version keeps what kind of
> thing a page is — person, project — only in its database, not in the
> markdown it exports. tlog reads them from a copy of `db.sqlite`, so a running
> Logseq is not disturbed. Without `sqlite3` installed, the import says so and
> continues without tags.

## What is different

| | Logseq | tlog |
|---|---|---|
| link a page | `[[` | `@` or `[[` |
| insert a date | `/date` | `@fri` |
| tag | `#tag` | `#tag` — the same |
| task | `TODO` keyword | a checkbox: `/todo` |
| deadline | `DEADLINE: <…>` | `/deadline fri` → `Deadline:: 2026-11-20` |
| block reference | `((uuid))` | `((` then search → an ordinary link |
| embed | `{{embed ((…))}}` | a block containing only a reference |
| queries | `{{query …}}` | none — backlinks, tag pages, the agenda and search |
| plugins, themes, graph view | yes | none |
| where the truth is | a database (DB version) | the markdown files |

If you rely on queries and plugins, tlog will feel bare. If what you use every
day is the journal, links and tasks, it has what you need and starts in an
instant.
