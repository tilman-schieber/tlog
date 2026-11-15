# History, undo and pushing

Your notes folder is a git repository, and tlog commits to it for you. You do
not need to know git to benefit — but it explains why tlog can afford to be
relaxed about deleting and rearranging.

## Saving, committing, pushing

Three things happen to what you write, each a little later than the one before.

| Step | When | What it protects against |
|---|---|---|
| **Saved** to the file | as soon as you leave a block | closing the terminal |
| **Committed** to git | once you have stopped writing for a moment | your own mistakes — every earlier version is kept |
| **Pushed** to a remote | when you ask, or after every commit | losing the machine |

The first two happen by themselves. The third is yours to switch on.

## Undo

`ctrl+z` while writing, `u` while moving.

Undo works in two layers: first the typing in the current block, a word at a
time; then each written change — an indent, a move, a deleted block, a ticked
task. Redo is `ctrl+y` or `U`.

From a shell, `tlog undo` takes back the last thing tlog committed. It refuses
if the file has been changed by hand since, so it cannot undo over somebody
else's edit.

And beyond undo, the whole history is there:

```sh
cd ~/notes
git log --oneline
git log -p -- "pages/Ada Lovelace.md"
```

## Pushing

Pushing means sending your notes to another git repository — on GitHub, GitLab,
or your own server.

```sh
tlog config git.remote git@gitlab.com:you/notes.git
tlog push                 # now
tlog push -auto on        # from now on, after every commit
tlog pull                 # bring in what another machine pushed
tlog sync                 # where do I stand?
```

Whether notes leave the machine is remembered *in the notes folder itself*, not
in tlog's settings. A fresh notes folder never pushes by surprise, and a copy
of an existing one keeps its answer.

### Three promises about pushing

Writing a note and publishing it are different acts, and only one is easy to
take back. So:

- **A failed push is never a failed save.** If you are offline you are told
  "committed, but not pushed". The commit is there and goes out with the next
  push.
- **It never forces and never merges.** If the remote has changes this copy
  does not — you wrote on another machine — the push is declined and you are
  told to pull. tlog does not decide for you which version wins.
- **It never hangs.** A push gets twenty seconds. Being offline costs a moment.

## Two machines

The comfortable routine is: `tlog pull` when you sit down, auto-push on. If you
forget and both machines have new notes, `tlog pull` brings the other side in
and puts your own on top. Only if both changed the *same lines* does it stop:
the pull is declined, nothing is merged for you, and you resolve it with git.

## Turning it off

If you manage the repository yourself, or do not want one, `git.autocommit` in
the [settings](../reference/settings.md) switches the automatic commits off.
Everything else in tlog works the same.

---

See it in the tutorial: [Keeping notes safe and in sync](../tutorial/09-sync.md) ·
Related: [Notes are files](files.md)
