# Settings

tlog has few settings, on purpose. Change them in the outliner — `p`, then
type `settings` — or from a shell:

```sh
tlog config                             # everything, and where each value came from
tlog config attachments.sanitize        # read one
tlog config attachments.sanitize true   # change one
```

They are kept in `~/.config/tlog/config.toml`, which is created the first time
you change something. The file is rewritten whole each time, with an
explanation of every setting in it, so reading it is a fine way to see what
there is. Your values are kept; comments you add by hand are not.

## The settings

| Key | What it decides |
|---|---|
| `notes` | where the notes live. `$TLOG_DIR` overrides it. Takes effect at the next start. |
| `startup` | what tlog opens on: today's journal, or whatever was written last |
| `attachments.sanitize` | tidy file names on the way in: `meeting notes.pdf` → `meeting-notes.pdf`. Off by default. Umlauts are kept either way. |
| `attachments.lowercase` | …and lowercase them |
| `git.autocommit` | commit as you write |
| `git.debounce` | how long writing must pause before a commit |
| `git.autopush` | push after every commit ¹ |
| `git.remote` | where pushing goes ¹ |
| `watch.enabled` | notice edits made in another editor. Takes effect at the next start. |
| `watch.interval` | how often the notes folder is checked |
| `format.blank_lines` | a blank line between top-level blocks in the files ² |
| `deadline.property` | whether a deadline is written as `Deadline::` or `due::` |
| `dates.end_of_week` | what `eow` means: friday or sunday |

¹ **Kept with the notes, not with tlog.** Whether these notes are pushed, and
where to, is stored in the notes folder's own git configuration. A copy of your
notes carries the answer with it, and a brand-new notes folder never pushes by
surprise. You still read and change both through `tlog config`. Clearing the
remote turns pushing off as well.

² Changing this reformats each file the next time that file is written. The
content is untouched, but the change in git will look large.

## Which value wins

A flag on the command line beats an environment variable, which beats the
config file, which beats the default.

| | Flag | Environment |
|---|---|---|
| notes folder | `-dir <path>` | `TLOG_DIR` |
| picture drawing | | `TLOG_IMAGES=kitty\|sixel\|iterm\|none` |

`TLOG_IMAGES` is only needed if tlog guesses wrong about whether your terminal
can draw pictures.

## What is not a setting

Some things are decisions rather than preferences, and have no switch:

- dates are always stored as `2026-11-20` ([why](../concepts/dates.md));
- the files are the truth, and tlog never overwrites a file that changed under
  it ([why](../concepts/files.md));
- attachments live in the notes' `assets/` folder
  ([why](../concepts/attachments.md));
- the [markdown dialect](dialect.md) is small and stays small.

A setting for any of these would only be a supported way to break it.

## Colours

tlog has no theme of its own. It uses your terminal's sixteen colours, so it
matches whatever theme the terminal has — change the terminal's theme and tlog
follows.
