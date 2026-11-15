# What tlog is

tlog is a place to write things down, in your terminal.

You open it and you are in today's page, ready to type. Each line is a
**block**; blocks nest under each other, so a thought and its details stay
together. Mention a person, a project or a day and it becomes a link — and the
page for that person or project collects, by itself, everything you ever wrote
about them.

Underneath there is nothing but markdown files in a folder:

```text
~/notes/
├── journals/
│   ├── 2026-11-14.md
│   └── 2026-11-15.md
├── pages/
│   ├── Ada Lovelace.md
│   └── Grant proposal.md
└── assets/
    └── budget.pdf
```

No database, no account, no background process. You can read your notes with
`cat`, search them with `rg`, edit them in any editor, and tlog notices. The
folder is a git repository, and tlog commits as you write, so nothing you
change is ever really gone.

## The whole idea in one minute

You only need to remember four things, and three of them are single characters:

| You type | It means | Example |
|---|---|---|
| plain text and `enter` | write a block, start the next one | `Met the dean about the budget` |
| **`@`** | **find** a page, a person or a day and link it | `@ada` · `@fri` |
| **`#`** | **file** this block under a topic | `#grant` |
| **`/`** | **change** the block you are in | `/todo` · `/deadline fri` |

Everything else in this book is detail on those four.

## Where to go from here

- **New to tlog?** Follow the [tutorial](tutorial/index.md). It takes about
  twenty minutes and walks through a working day of notes, introducing each
  idea when you need it.
- **Want to understand how it thinks?** The *Concepts* chapters explain one
  idea each — start with [Notes are files](concepts/files.md) or
  [Blocks and the outline](concepts/blocks.md).
- **Looking something up?** The *Reference* chapters list every
  [sign](reference/signs.md), [slash command](reference/slash-commands.md),
  [key](reference/keys.md) and [command](reference/cli.md).

Inside tlog, `F1` shows a short version of all this at any time.
