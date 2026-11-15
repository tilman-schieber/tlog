# Attachments

Pictures, PDFs and other files you attach become **part of your notes**. They
are copied into the `assets/` folder inside the notes directory, and a block
links to them.

```text
~/notes/
├── journals/
├── pages/
└── assets/
    ├── quote.pdf
    └── screenshot-2026-11-15-1432.png
```

## Why inside the notes

Because then they go wherever the notes go. They are committed with the note
that links them, pushed and pulled with everything else, and a screenshot
pasted on one machine is there on the next. A link to a file somewhere else on
your disk would break the first time you opened your notes on another
computer.

For the same reason tlog never writes a link to a file *outside* the notes.

## Four ways in

| You have | Do | Result |
|---|---|---|
| a screenshot or picture on the clipboard | `ctrl+v` | saved as `screenshot-<date>-<time>.png`, linked at the caret |
| a file in your file manager | drag it onto the terminal (or copy it and `ctrl+v`) | copied in, linked at the caret |
| a file, from a shell | `tlog attach quote.pdf` | copied in, linked on today's journal (`-p Page` for a page) |
| a long text on the clipboard | `/paste` | kept as a `.md` file of its own, linked by its first line |

And to link something that is already there: `/file` and a few letters of its
name.

## What happens to the original

Nothing. The file is **copied**, never moved, so attaching it does not take it
away from wherever else it is used.

If a file of the same name is already in `assets/`, the new one gets a number
— `report.pdf`, then `report-2.pdf`. Nothing is ever overwritten.

## Looking at an attachment

Follow the link with `g` or a click.

- **A picture** is drawn in the terminal if it can draw pictures (kitty,
  ghostty, foot, Konsole, WezTerm, iTerm2…); otherwise it opens in your image
  viewer.
- **A text file** — markdown, a script, CSV — is shown inside tlog. Markdown is
  rendered with headings, lists and working links. `e` opens it in your editor,
  `o` hands it to the desktop, `esc` comes back.
- **Anything else** opens in whatever your system uses for it.

## Long text: `/paste`

The minutes of a meeting or a long email are worth keeping, but forty lines in
the middle of a journal page bury everything around them. `/paste` takes the
text on your clipboard, saves it as its own markdown file, and leaves one link
in the block.

Such a file is an attachment, not a [page](journals-and-pages.md): it has no
backlinks and does not appear when you go to a page. It is a document you
filed, and the block that links it is your note about it.

## Listing what is attached

```sh
tlog files            # everything, newest first
tlog files budget     # narrowed
```

---

See it in the tutorial: [Files, pictures and long text](../tutorial/07-files.md) ·
Reference: [`/file` and `/paste`](../reference/slash-commands.md#attach)
