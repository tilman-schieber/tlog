# The four signs: @ [[ # ((

Four things you can type in a block open a menu and help you point at
something. Each has one job. (The fifth, `/`, does not point at anything — it
[changes the block](slash-commands.md).)

| Sign | Job | What ends up in the file |
|---|---|---|
| `@` | find a page or a day | `[[Ada Lovelace]]` · `[[2026-11-20]]` |
| `[[` | find a page | `[[Ada Lovelace]]` |
| `#` | file the block under a topic | `#budget` |
| `((` | point at one block | `[[Page#^k3f9q2]]` |

In every menu: `↑` `↓` choose, `enter` or `tab` accepts, `esc` closes the menu
and leaves what you typed as plain text.

## @

*Find a person, a page or a day, and link it.*

- Opens when `@` starts a word. An email address such as `a@b.ch` never
  triggers it.
- With nothing typed: the pages you have written about most recently.
- As you type: pages whose name — or [alias](../concepts/journals-and-pages.md#more-than-one-name)
  — starts with it come first; initials work (`@al` → Ada Lovelace); then
  [days](../concepts/dates.md) the typing could mean (`@fri`, `@in 3 days`);
  then looser matches.
- Each page shows its [tags](../concepts/tags.md) and how often it is
  mentioned.
- A capitalised name that nothing answers to is offered as **new page**.

The `@` itself is not stored. Accepting replaces `@…` with an ordinary link.

→ Concept: [Links and backlinks](../concepts/links.md) · Tutorial:
[People, pages and days](../tutorial/04-linking.md)

## [[

*The same pages, for when you think in brackets.*

- Typing `[[` adds the closing `]]` by itself; typing `]` over it steps past.
- Matching is the same as `@`, without the days.
- Inside the brackets, a space and `#` starts a tag that says what the page
  *is*: `[[Ada Lovelace #person]]`. Existing tags are offered.
- A name that does not exist yet is fine — finish typing and close the
  brackets.

## #

*File this block under a topic.*

- Opens when `#` starts a word and offers the topics already in use.
- `tab` takes the highlighted one. **`enter` does not** — it ends the block as
  usual — unless you chose a tag with the arrows first. A tag is complete as
  typed, so a new one never needs confirming.
- A tag may contain letters, digits, `-`, `_` and `/`, and starts with a
  letter: `#lab-move`, `#teaching/ws26`.
- `# ` with a space after it, at the start of a block, is a heading and not a
  tag.

→ Concept: [Tags](../concepts/tags.md) · Tutorial:
[Topics](../tutorial/05-topics.md)

## ((

*Point at one particular block.*

- Opens on `((` and searches every block in your notes for what you type.
- Accepting gives the chosen block a permanent marker (`^k3f9q2`) and writes a
  link to it. This is the one menu whose choice changes *another* file.
- The reference is drawn as the text of the block it points at; a block
  containing only a reference shows the original with its children.

→ Concept: [Pointing at one block](../concepts/block-references.md) ·
Tutorial: [Finding things again](../tutorial/08-finding.md)

## At a glance

| I want to… | Type |
|---|---|
| mention a person or project | `@name` |
| mention a day | `@fri` |
| label a block with a topic | `#topic` |
| say what a page is | `[[Name #kind]]`, or [`/tagpage kind`](slash-commands.md#tagpage) on it |
| quote or reuse one block | `((words from it` |
| link a file I attached | [`/file`](slash-commands.md#file) |
| link a website | paste the address |
