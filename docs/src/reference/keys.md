# Keys

tlog opens ready to write. `esc` puts the caret down so that single letters
become commands; `enter` picks it back up. The header shows **MOVE** while you
are moving.

`F1` shows a summary of this page inside tlog.

## While writing

| Key | What it does |
|---|---|
| `enter` | new block — in the middle of a line it splits the block |
| `alt+enter` · `ctrl+j` | new line inside the block |
| `tab` · `shift+tab` | indent · outdent |
| `alt+↑` · `alt+↓` | move the block, with everything under it |
| `backspace` at the start | join onto the block above; an empty block goes |
| `↑` `↓` `←` `→` | move the caret; they run on from one block into the next |
| `home` · `end` (`ctrl+a` · `ctrl+e`) | start · end of the line |
| `ctrl+w` · `ctrl+u` | delete the word before the caret · back to the start of the line |
| `ctrl+d` | tick the task off, or make the block one |
| `ctrl+v` | paste: a picture or copied files are attached, text is text |
| `ctrl+z` · `ctrl+y` | undo · redo |
| `@` `[[` `#` `((` | [point at something](signs.md) |
| `/` | [change the block](slash-commands.md) |
| `esc` | stop writing and move around |

> `shift+enter` cannot be told apart from `enter` in most terminals, which is
> why the line break is `alt+enter`. In kitty you can map `shift+enter` to send
> `\x1b\r` and have it too.

## While moving

| Key | What it does |
|---|---|
| `enter` · `i` | write here |
| `j` `k` · `↓` `↑` | from block to block |
| `h` `l` · `z` | fold, unfold · toggle |
| `tab` · `shift+tab` | indent · outdent |
| `alt+↑` · `alt+↓` | move the block |
| `x` · `space` | tick the task off, or make the block one |
| `dd` | delete the block and everything under it |
| `g` · `o` | follow the link in this block |
| `b` · `backspace` | back to where you came from |
| `t` · `[` · `]` | today · the day before · the day after |
| `p` | go to a page, or run a command by name |
| `/` | search every block |
| `a` | the agenda |
| `y` · `Y` | copy the block's text · copy a link in it |
| `u` · `U` | undo · redo |
| `?` | help |
| `q` | quit |

## In either mode

| Key | What it does |
|---|---|
| `ctrl+p` | go to |
| `ctrl+f` | search |
| `ctrl+t` | today |
| `alt+←` · `alt+→` | the day before · the day after |
| `ctrl+o` | back |
| `ctrl+g` | follow the link at the caret |
| `ctrl+↑` · `ctrl+↓` | fold · unfold |
| `pgup` · `pgdn` | half a screen up · down |
| `ctrl+home` · `ctrl+end` | top · bottom of the page |
| `ctrl+r` | reload after the file was changed elsewhere |
| `F1` | help |
| `ctrl+q` | quit |

## In a menu or list

| Key | What it does |
|---|---|
| `↑` `↓` (`ctrl+p` `ctrl+n`) | choose |
| `enter` · `tab` | accept |
| `esc` | close |

## In the agenda

| Key | What it does |
|---|---|
| `j` `k` · `↓` `↑` | move |
| `enter` | go to the task in its page |
| `space` | tick it off |
| `a` | also show dated tasks that are done |
| `esc` · `q` | close |

## When a file or picture is shown

| Key | What it does |
|---|---|
| `e` | edit it in `$VISUAL` or `$EDITOR` |
| `o` | hand it to the desktop |
| `esc` | back |

## The mouse

| Action | What it does |
|---|---|
| click a block | write there |
| click a link | open it |
| click `▸` `▾` | fold, unfold |
| click `☐` | tick the task |
| right-click a link | copy where it points |
| hover a link | show its address in the footer |
| wheel | scroll |
| drop a file on the window | attach it |
| `shift`+drag | select text, as in any terminal program |

## The `p` list

`p` (or `ctrl+p`) shows, in order: pages you visited recently, today, the
commands below, then every journal and page. Typing narrows all of it. A name
that matches nothing makes a new page.

| Command | What it does |
|---|---|
| Agenda | every open task |
| Backlinks | what links to this page, as a list |
| Delete block | the block under the cursor and everything in it |
| Reload from disk | re-read the file |
| Settings | change tlog's [settings](settings.md) |
| Messages | everything tlog has told you this session |
| Help | the help page |
