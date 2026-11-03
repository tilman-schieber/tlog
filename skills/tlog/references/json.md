# tlog JSON shapes

Everything an address appears in prints it as one string, `rel:offset@hash`.
Copy it whole. Lists are always lists: an empty result is `[]`, never `null`.

## A write (`add -json`, `append`, every `block … <op>`)

```json
{
  "addr": "pages/Grant.md:0@39ad22fa87fdf960",
  "rel": "pages/Grant.md",
  "offset": 0,
  "hash": "39ad22fa87fdf960",
  "warning": "committed, but not pushed: …"      // only when the commit or push failed
}
```

`block … ref -json` adds `"link": "[[Grant#^k3f9q2]]"`.

## `search -json`

```json
[
  {
    "addr": "pages/Grant.md:0@39ad22fa87fdf960",
    "rel": "pages/Grant.md",
    "page": "Grant",
    "offset": 0,
    "hash": "39ad22fa87fdf960",
    "text": "[ ] write the budget\nwith the new rates",   // the whole block
    "firstLine": "[ ] write the budget"
  }
]
```

`text` is the raw block, task marker included. `firstLine` is for listings.

## `view -json`

```json
{
  "rel": "pages/Grant.md",
  "title": "Grant",
  "hash": "39ad22fa87fdf960",
  "isJournal": false,
  "tags": [],                 // what the page is: ["person"], ["project"]
  "aliases": ["Förderung"],   // only when it has some
  "blocks": [
    {
      "addr": "pages/Grant.md:0@39ad22fa87fdf960",
      "offset": 0,
      "depth": 0,                       // nesting; order is document order
      "text": "[ ] write the budget",   // raw
      "body": "write the budget",       // without the task marker
      "props": [{"key": "Deadline", "value": "2026-11-20"}],
      "task": "open",                   // "", "open" or "done"
      "hasChildren": false,
      "due": "2026-11-20",              // resolved from whichever spelling the file uses
      "dueState": "later",              // overdue, today, soon, later; empty when done
      "dueLabel": "in 2 weeks",
      "anchor": "k3f9q2",               // only when the block has a durable name
      "embeds": [ ... ],                // resolved [[Page#^anchor]] references, in order
      "fences": [ ... ]                 // fenced code, highlighted; csv as rows
    }
  ],
  "tagged": [],               // pages tagged with this page's name
  "refs": [                   // blocks elsewhere that mention this page, with their children
    {"addr": "journals/2026-11-03.md:0@…", "page": "2026-11-03", "rel": "journals/2026-11-03.md",
     "offset": 0, "text": "met [[Grant]] people", "body": "…", "task": "", "depth": 0, "head": true}
  ]
}
```

`blocks[i].addr` is what you pass to `block`. The page-level `hash` is what
`append -if-match` takes.

## `block <addr> -json`

One element of `blocks` above.

## `todo -json` and `due -json`

```json
[
  {
    "addr": "pages/Grant.md:0@39ad22fa87fdf960",
    "rel": "pages/Grant.md",
    "page": "Grant",
    "offset": 0,
    "hash": "39ad22fa87fdf960",
    "text": "write the budget",   // without the task marker
    "due": "2026-11-20",          // empty for an undated task in todo
    "state": "later",             // overdue, today, soon, later; empty when undated or done
    "label": "in 2 weeks",
    "done": false
  }
]
```

## `links <addr> -json`

```json
[
  {"kind": "page",  "target": "Grant", "rel": "pages/Grant.md", "exists": true},
  {"kind": "page",  "target": "Nowhere", "rel": "pages/Nowhere.md"},            // exists omitted = false
  {"kind": "block", "target": "Timetable", "anchor": "k3f9q2", "rel": "pages/Timetable.md",
   "exists": true, "addr": "pages/Timetable.md:0@…", "text": "the lecture is at nine"},
  {"kind": "url",   "target": "https://go.dev/doc", "resolved": "https://go.dev/doc"},
  {"kind": "file",  "target": "../assets/plan.png", "resolved": "/Users/…/assets/plan.png"},
  {"kind": "file",  "target": "../assets/rules.md", "resolved": "/Users/…/assets/rules.md",
   "plain": true},                                                              // plain text: read it with cat
  {"kind": "url",   "target": "ftp://x", "error": "…"}                           // cannot be opened
]
```

## `date -json`

```json
[{"phrase": "friday", "iso": "2026-11-06", "short": "Fr 06.11.", "label": "in 3 days", "state": "soon"}]
```

## `mentions -json`

```json
[
  {"kind": "page", "name": "Julia Rausenberger", "insert": "[[Julia Rausenberger]]", "detail": "person · 12 mentions", "tags": ["person"]},
  {"kind": "date", "name": "friday", "insert": "[[2026-11-06]]", "detail": "Fr 06.11.  in 3 days", "iso": "2026-11-06", "state": "soon"},
  {"kind": "page", "name": "Jour fixe", "insert": "[[Jour fixe]]", "detail": "1 mention"},
  {"kind": "new",  "name": "Jr Neu", "insert": "[[Jr Neu]]", "detail": "new page"}
]
```

Write `insert` as given. `tags` says what a page is; it is absent when the page
has none.

## `tag <tag> -json`

```json
[
  {"name": "tlog", "rel": "pages/tlog.md", "exists": true, "tags": ["repo", "projekt"]},
  {"name": "Ada Lovelace", "rel": "pages/Ada Lovelace.md", "exists": false, "tags": ["person"]}
]
```

`exists` is false for a page that is only linked, as `[[Ada Lovelace #person]]`.

## `index -json` and `places -json`

```json
[
  {"kind": "journal", "name": "2026-11-03", "rel": "journals/2026-11-03.md"},
  {"kind": "page",    "name": "Grant",      "rel": "pages/Grant.md"},
  {"kind": "tag",     "name": "project"}                                     // a tag has no file
]
```

## `commands -json`

```json
[{"name": "deadline", "title": "Deadline", "hint": "Due on …", "arg": "date"}]
```

`arg` is `date`, `attachment`, `text`, or absent when the command takes none.

## `undo -json`

```json
{"rels": ["pages/Grant.md"], "rel": "pages/Grant.md"}
```

## `config -json`

```json
[{"key": "deadline.property", "value": "Deadline", "kind": "text", "hint": "The property a deadline is written to", "note": "…", "source": "config"}]
```

## `sync -json`

```json
{"remote": true, "autoPush": false, "lastError": ""}
```
