# tlog commands

`tlog help` and `tlog block help` print the authoritative short form. This is
the long form, grouped by what you are trying to do. Every command below takes
`-json`; flags may be written before or after the arguments; a word after `--`
is never a flag. `-dir <path>` before the command picks a notes directory
other than `$TLOG_DIR` / `~/notes`.

## Finding things

| command | what it does | prints |
|---|---|---|
| `tlog search <query> [-n N]` | blocks whose text has every word of the query, best first | page and first line; `-json` the hits with `addr` |
| `tlog view [page]` | a page by name, alias or path, or today's journal | title, bullets; `-addr` prefixes each with its address; `-json` the whole page |
| `tlog view -day <d>` | a journal: `today`, `yesterday`, `-1`, `+2`, `fri`, `2026-11-03` | as above |
| `tlog block <addr>` | one block | text, properties, deadline; `-json` the block |
| `tlog todo` | every open task, dated first (soonest), then undated (newest journal first) | lines; `-json` items with `addr`, `due`, `state`, `label` |
| `tlog due [-all]` | everything with a deadline that is open; `-all` includes done | as `todo` |
| `tlog links <addr>` | what a block points at, resolved | page links with their file, block refs with the target's address and text, urls and files with what would open, or why not |
| `tlog index` | every journal, page and tag | lines; `-json` places: `kind`, `name`, `rel` |
| `tlog places [query]` | the same, ranked against a query like the palette does | as `index -json` |
| `tlog tag <tag>` | every page carrying a tag, by frontmatter or by `[[Page #tag]]` anywhere; `tlog people` is `tag person` | names; `-json` `name`, `rel`, `exists`, `tags` |
| `tlog tag <tag> <page>…` | puts the tag into each page's `tags:` frontmatter (a page only ever linked gets a file); twice is harmless | a line per page |
| `tlog untag <tag> <page>…` | takes it out; refused (3) when the frontmatter does not carry it | a line per page |
| `tlog mentions <query>` | the `@` menu: every page ranked by use, with its tags, then days, then a "new page" when a name nothing answers to was typed | `kind`, `name`, `insert`, `detail` |
| `tlog date [shorthand]` | what typed dates mean: `fri`, `in 3 days`, `12.11`, `eow`; nothing typed gives the usual days | `phrase`, `iso`, `short`, `label`, `state` |
| `tlog commands` | the slash commands `block … cmd` can run | `name`, `title`, `hint`, `arg` |
| `tlog files [query] [-links]` | what is attached (the notes' `assets/`), newest first | name, size, date; `-links` the markdown links; `-json` adds `rel` |
| `tlog config [key]` | every setting, or one | key, value, hint; `-json` the list |
| `tlog sync` | remote, auto-push, last push error | one line; `-json` `remote`, `autoPush`, `lastError` |
| `tlog today` | the path of today's journal (does not create it) | a path |

Reads create nothing. A page that does not exist is exit 3. `tlog open <page>`
is the one read-like command that creates the page and prints its path.

## Adding

| command | what it does | prints |
|---|---|---|
| `tlog add <text>` | a block at the end of today's journal | the file's path; `-json` the block's `addr` |
| `tlog add -p <page> <text>` | …at the end of a page, created if needed | as above |
| `tlog add -todo [-due <d>] [-p page] <text>` | …as an open task, with a deadline when given; an unreadable date is exit 3 and writes nothing | as above |
| `tlog append <rel> [-if-match <hash>] <text>` | a block at the end of a file by path, refused (4) if the file's hash is not `hash` | the new `addr` |
| `tlog attach [-p page] FILE…` | copies files into the notes' `assets/` (committed with them), links them as `../assets/name` in one block in today's journal or a page | names and sizes, then the file's path |
| `tlog attach -link-only FILE…` | copies into `assets/` and prints the links, writes no note | links |
| `tlog attach -clipboard [-p page] [-link-only]` | saves the picture on the system clipboard (a screenshot) into `assets/` as `screenshot-<date>-<time>.png` and links it like a file; exit 3 if the clipboard holds no picture | as for a file |
| `tlog attach -migrate [-from dir] [-dry-run]` | copies files that notes link on the old `~/.att/store` shelf into `assets/` and rewrites those links; files missing here are reported and left alone | one line per file, then a summary; `-json` `copied`, `notes`, `links`, `missing` |
| `tlog alias <page> <name>` | another name for a page, in its frontmatter | a line |

`<text>` may be `-` to read it from standard input; several lines become one
multi-line block. With no text at all, `add` reads standard input too.

## Changing one block: `tlog block <addr> <op>`

| op | effect | note |
|---|---|---|
| `text <t>` | replace the text; children stay | |
| `after [-child] <t>` | a new sibling below, or a first child | empty text is a usage error |
| `before <t>` | a new sibling above | |
| `split [-child] <before> <after>` | end the block at a point and start the next one | one write, not two |
| `merge` | join onto the block above and remove | refused (3) when it has children |
| `indent` / `outdent` | nest under the previous sibling / out to the parent's level | refused (3) at the edge |
| `up` / `down` | move among siblings, children along | refused (3) at the edge |
| `task` | toggle: plain → open → done → open | avoid in scripts; a retry undoes it |
| `open` / `done` | make it an open or a done task | idempotent |
| `deadline <d>` | due on a day; makes it a task | writes the configured property, replacing other spellings |
| `quote` | make it a quotation (`> `) | |
| `prop <key> [value]` | set a `Key:: value` property; no value removes it | |
| `cmd <name> [arg]` | any slash command by name: `todo`, `done`, `deadline`, `date`, `quote`, `code`, `table`, `page`, `tag`, `file`, `prop`, `alias` | what a command would insert goes at the end of the text |
| `ref` | give the block a durable `^anchor` and print `[[Page#^anchor]]` | the only read-shaped op that writes; `-json` also gives the new `addr` |
| `delete` | remove it and everything under it | |

Every op prints the block's new address, or with `-json`:

```json
{"addr": "pages/Grant.md:0@ff893538f9b3e69d", "rel": "pages/Grant.md", "offset": 0, "hash": "ff893538f9b3e69d"}
```

plus `"warning"` when the note was saved but the commit or push after it
failed (still exit 0: the write happened).

## Taking back

| command | what it does |
|---|---|
| `tlog undo` | reverts the last commit tlog made, every note in it |
| `tlog undo <rel>` | reverts the last tlog commit to one file |

Refused (3) when the last change was not tlog's; a conflict (4) when the file
has changed since that commit, which means a person edited it by hand. The
undo is itself committed.

## Pushing and settings

| command | what it does |
|---|---|
| `tlog push` | commit what is outstanding and push; needs a remote. When another machine pushed first, its notes are pulled in and the push retried |
| `tlog pull` | bring in what another machine pushed; this copy's unpushed commits go on top, a clash is refused and nothing is merged |
| `tlog push -auto on\|off` | push on every commit, or not — kept in the notes repository |
| `tlog config git.remote <url>` | where pushing goes |
| `tlog config <key> <value>` | change a setting; a bad value is exit 2 and changes nothing |

## Exit codes

0 done · 1 something went wrong · 2 not a command, operation or address ·
3 does not exist (page, move, date, command) — retrying will not help ·
4 the file moved since the address was read — read again and retry.
