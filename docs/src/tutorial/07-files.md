# 7. Files, pictures and long text

Not everything worth keeping is a line of text. The quote arrives as a PDF,
somebody sends a screenshot, the minutes come in a long email.

All of these go into a folder called `assets/` *inside* your notes, and a block
links to them. They travel with your notes wherever the notes go.

## A screenshot

Take a screenshot so that it is on your clipboard. In tlog, with the caret
where you want it, press `ctrl+v`.

The picture is saved into `assets/` and a link to it appears at the caret.
Press `esc`, move onto the block and press `g`: in a terminal that can draw
pictures (kitty, ghostty, foot, WezTerm, iTerm2 and others) it is shown right
there; anywhere else it opens in your image viewer.

> It has to be plain `ctrl+v`. `ctrl+shift+v` is your terminal's own paste,
> which can only deliver text.

## A file

Drag a file from your file manager onto the terminal window. It is copied into
`assets/` and linked where you are. The original stays where it was.

From a shell, the same thing is:

```sh
tlog attach quote.pdf                      # linked on today's journal
tlog attach -p "Grant proposal" quote.pdf  # …or on a page
```

To link something that is *already* attached, type `/file` and a space in a
block: the menu lists what is in `assets/`, newest first. Type a few letters to
narrow it.

## A long text

Sometimes you want to keep a whole email or a set of minutes, but not as forty
blocks in the middle of your day. Copy the text, and in a block type:

```text
Minutes from the board /paste
```

The text becomes a markdown file of its own in `assets/`, and the block links
to it by its first line:

```text
• Minutes from the board [Minutes, 15 November](../assets/2026-11-15-minutes-15-november.md)
```

Following the link (`g`) shows the file inside tlog, nicely rendered. `e` there
opens it in your editor.

## Ordinary links

A web address pasted into a block is a link; so is `[a label](https://…)`,
which is drawn as just its label. Hover over a link to see where it goes, click
it or press `g` to open it.

---

**What you just met**

- Where attached files live and why: [Attachments](../concepts/attachments.md)
- `/file` and `/paste`: [Slash commands](../reference/slash-commands.md#attach)

Next: [Finding things again](08-finding.md)
