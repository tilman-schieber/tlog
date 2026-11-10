package store

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// DebounceDefault is how long the committer waits after the last write before
// making a commit. Long enough that a burst of typing lands in one commit,
// short enough that walking away from the terminal still leaves your notes
// safe.
const DebounceDefault = 30 * time.Second

// PushTimeout bounds a push, so that being offline costs a moment rather than
// hanging a command that is supposed to feel instant.
const PushTimeout = 20 * time.Second

// AutoPushKey is the repository-local setting that turns pushing on. It lives
// in the notes repository rather than in tlog, because whether these notes go
// anywhere is a property of this directory: a freshly created one never pushes
// by surprise.
const AutoPushKey = "tlog.autopush"

// Git is the durability layer. Structural edits rewrite whole files, so the
// only honest protection against tlog confidently writing the wrong thing is a
// history you can walk back. It replaces backup files entirely.
type Git struct {
	root    string
	enabled bool

	mu         sync.Mutex
	touched    map[string]bool
	timer      *time.Timer
	wait       time.Duration
	autopush   bool
	autocommit bool
	lastErr    error

	// op keeps committing, pulling and pushing from running over each other:
	// the debounced commit fires on a timer, and a pull rewrites the branch.
	op sync.Mutex
}

// NewGit prepares the notes directory for versioning, initialising a
// repository if there is not one already. When git is unavailable the
// committer degrades to doing nothing rather than failing writes.
func NewGit(root string) *Git {
	g := &Git{root: root, touched: map[string]bool{}, wait: DebounceDefault, autocommit: true}
	if _, err := exec.LookPath("git"); err != nil {
		return g
	}
	if !isRepo(root) {
		if err := run(root, "init", "-q"); err != nil {
			return g
		}
		writeIfAbsent(filepath.Join(root, ".gitignore"), ".tlog-*.tmp\n.DS_Store\n")
	}
	ensureIdentity(root)
	g.enabled = true
	g.autopush = configBool(root, AutoPushKey)
	return g
}

// AutoPush reports whether commits are pushed as they are made.
func (g *Git) AutoPush() bool { return g != nil && g.enabled && g.autopush }

// SetAutoPush turns pushing on or off for this notes directory.
func (g *Git) SetAutoPush(on bool) error {
	if !g.Enabled() {
		return fmt.Errorf("no git repository in the notes directory")
	}
	if on && !g.HasRemote() {
		return fmt.Errorf("no remote configured; add one with `git -C %s remote add origin <url>`", g.root)
	}
	if err := run(g.root, "config", AutoPushKey, boolStr(on)); err != nil {
		return err
	}
	g.mu.Lock()
	g.autopush = on
	g.mu.Unlock()
	return nil
}

// Remote is where the notes are pushed, or empty when nowhere.
func (g *Git) Remote() string {
	if !g.Enabled() {
		return ""
	}
	out, err := output(g.root, "remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// SetRemote points the notes at a remote, adding or replacing origin. An empty
// url removes it, and takes pushing down with it rather than leaving a setting
// that cannot do anything.
func (g *Git) SetRemote(url string) error {
	if !g.Enabled() {
		return fmt.Errorf("no git repository in the notes directory")
	}
	url = strings.TrimSpace(url)
	if url == "" {
		if g.Remote() != "" {
			_ = run(g.root, "remote", "remove", "origin")
		}
		return g.SetAutoPush(false)
	}
	if g.Remote() == "" {
		return run(g.root, "remote", "add", "origin", url)
	}
	return run(g.root, "remote", "set-url", "origin", url)
}

// AutoCommit reports whether writing is committed as it happens.
func (g *Git) AutoCommit() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.autocommit
}

// SetAutoCommit turns the debounced commit on or off. With it off nothing is
// committed until something asks — `tlog push`, or quitting the outliner.
func (g *Git) SetAutoCommit(on bool) {
	g.mu.Lock()
	g.autocommit = on
	if !on && g.timer != nil {
		g.timer.Stop()
		g.timer = nil
	}
	g.mu.Unlock()
}

// HasRemote reports whether there is anywhere to push to.
func (g *Git) HasRemote() bool {
	out, err := output(g.root, "remote")
	return err == nil && strings.TrimSpace(out) != ""
}

// LastPushError is the most recent push failure, or nil. A push that fails must
// never block writing a note, so the error is kept here for an adapter to show
// rather than returned from the write path.
func (g *Git) LastPushError() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.lastErr
}

// Push sends the current branch to its remote. It never forces. When the
// remote has moved — another machine pushed — it pulls first, replaying this
// copy's commits on top of what arrived, and tries once more; that only
// succeeds when the two touched different things. When they changed the same
// lines nothing is merged and the push is refused, with both copies intact.
func (g *Git) Push() error {
	g.op.Lock()
	defer g.op.Unlock()
	return g.push()
}

func (g *Git) push() error {
	if !g.Enabled() {
		return fmt.Errorf("no git repository in the notes directory")
	}
	if !g.HasRemote() {
		return fmt.Errorf("no remote configured; add one with `git -C %s remote add origin <url>`", g.root)
	}
	branch, err := g.branch()
	if err != nil {
		return err
	}

	err = runTimeout(g.root, PushTimeout, "push", "--set-upstream", "origin", branch)
	if err != nil && rejected(err) {
		if err = g.pull(); err == nil {
			err = runTimeout(g.root, PushTimeout, "push", "--set-upstream", "origin", branch)
		}
	}
	g.setErr(err)
	if err != nil && rejected(err) {
		return fmt.Errorf("push rejected: the remote has commits this copy does not. "+
			"Run `git -C %s pull --rebase` and look at what comes back", g.root)
	}
	return err
}

func rejected(err error) bool {
	return strings.Contains(err.Error(), "rejected") || strings.Contains(err.Error(), "non-fast-forward")
}

// Pull brings in what the remote has and this copy does not: another machine's
// notes. This copy's own commits that were never pushed are replayed on top of
// them, and anything not yet committed is set aside and put back. When both
// changed the same lines nothing is merged — the rebase is undone, everything
// is as it was, and the error names the files — because which version of a
// note is right is a decision, not something to make behind someone's back.
func (g *Git) Pull() error {
	if !g.Enabled() {
		return fmt.Errorf("no git repository in the notes directory")
	}
	if !g.HasRemote() {
		return fmt.Errorf("no remote configured; add one with `git -C %s remote add origin <url>`", g.root)
	}
	g.op.Lock()
	defer g.op.Unlock()
	err := g.pull()
	if err != nil {
		g.setErr(err)
	}
	return err
}

func (g *Git) pull() error {
	if g.rebasing() {
		return fmt.Errorf("a rebase is already in progress in %s; finish it with git first", g.root)
	}
	branch, err := g.branch()
	if err != nil {
		return err
	}
	if err := runTimeout(g.root, PushTimeout, "fetch", "-q", "origin"); err != nil {
		return err
	}
	theirs := "refs/remotes/origin/" + branch
	if run(g.root, "rev-parse", "-q", "--verify", theirs) != nil {
		return nil // the remote has no such branch yet: nothing to bring in
	}
	if run(g.root, "merge-base", "--is-ancestor", theirs, "HEAD") == nil {
		return nil // nothing new
	}
	if err := run(g.root, "-c", "core.editor=true", "rebase", "-q", "--autostash", theirs); err != nil {
		files, _ := output(g.root, "-c", "core.quotePath=false", "diff", "--name-only", "--diff-filter=U")
		_ = run(g.root, "rebase", "--abort")
		if files = strings.TrimSpace(files); files != "" {
			return fmt.Errorf("this copy and the remote both changed %s; nothing was merged. "+
				"Run `git -C %s pull --rebase` and choose", strings.ReplaceAll(files, "\n", ", "), g.root)
		}
		return err
	}
	return nil
}

// rebasing reports whether a rebase was left half done, by hand or by a crash.
func (g *Git) rebasing() bool {
	for _, d := range []string{"rebase-merge", "rebase-apply"} {
		p, err := output(g.root, "rev-parse", "--git-path", d)
		if err != nil {
			continue
		}
		p = strings.TrimSpace(p)
		if !filepath.IsAbs(p) {
			p = filepath.Join(g.root, p)
		}
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

func (g *Git) branch() (string, error) {
	b, err := output(g.root, "rev-parse", "--abbrev-ref", "HEAD")
	return strings.TrimSpace(b), err
}

func (g *Git) setErr(err error) {
	g.mu.Lock()
	g.lastErr = err
	g.mu.Unlock()
}

// ensureIdentity sets a repository-local committer only when the user has none
// configured. A configured identity always wins.
func ensureIdentity(root string) {
	if hasConfig(root, "user.email") && hasConfig(root, "user.name") {
		return
	}
	_ = run(root, "config", "user.name", "tlog")
	_ = run(root, "config", "user.email", "tlog@localhost")
}

func hasConfig(root, key string) bool {
	out, err := output(root, "config", "--get", key)
	return err == nil && strings.TrimSpace(out) != ""
}

func configBool(root, key string) bool {
	out, err := output(root, "config", "--bool", "--get", key)
	return err == nil && strings.TrimSpace(out) == "true"
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func output(root string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	return string(out), err
}

// Enabled reports whether commits will actually happen.
func (g *Git) Enabled() bool { return g != nil && g.enabled }

// SetDebounce overrides the idle delay before a commit.
func (g *Git) SetDebounce(d time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.wait = d
}

// Touch records that a file changed and schedules a commit once writing has
// been idle for the debounce period.
func (g *Git) Touch(rel string) {
	if !g.Enabled() {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.touched[rel] = true
	if !g.autocommit {
		// Still remembered, so an explicit commit later picks it up.
		return
	}
	if g.timer != nil {
		g.timer.Stop()
	}
	g.timer = time.AfterFunc(g.wait, func() { _ = g.Flush() })
}

// Flush commits everything outstanding immediately. One-shot CLI commands call
// it directly; the TUI calls it on quit.
func (g *Git) Flush() error {
	if !g.Enabled() {
		return nil
	}
	g.mu.Lock()
	if g.timer != nil {
		g.timer.Stop()
		g.timer = nil
	}
	files := make([]string, 0, len(g.touched))
	for f := range g.touched {
		files = append(files, f)
	}
	g.touched = map[string]bool{}
	g.mu.Unlock()

	if len(files) == 0 {
		return nil
	}
	sort.Strings(files)

	g.op.Lock()
	defer g.op.Unlock()
	if err := run(g.root, "add", "-A"); err != nil {
		return err
	}
	if clean(g.root) {
		return nil
	}
	if err := run(g.root, "commit", "-q", "-m", commitMessage(files)); err != nil {
		return err
	}
	// The commit is what protects the notes; the push is a convenience on top.
	// A failed push must never look like a failed save.
	if g.AutoPush() {
		if err := g.push(); err != nil {
			return fmt.Errorf("committed, but not pushed: %w", err)
		}
	}
	return nil
}

func commitMessage(files []string) string {
	switch len(files) {
	case 1:
		return "tlog: " + filepath.ToSlash(files[0])
	case 2, 3:
		parts := make([]string, len(files))
		for i, f := range files {
			parts[i] = filepath.ToSlash(f)
		}
		return "tlog: " + strings.Join(parts, ", ")
	default:
		return fmt.Sprintf("tlog: %d files", len(files))
	}
}

// CommitPrefix starts every commit tlog makes, which is how one of tlog's own
// can be told from one made by hand.
const CommitPrefix = "tlog: "

// LastCommit is the newest commit touching rel, or the newest commit tlog made
// when rel is empty: its hash and subject. ok is false when there is none.
func (g *Git) LastCommit(rel string) (hash, subject string, ok bool, err error) {
	if !g.Enabled() {
		return "", "", false, fmt.Errorf("no git repository in the notes directory")
	}
	args := []string{"log", "-1", "--format=%H%x00%s"}
	if rel == "" {
		args = append(args, "-E", "--grep=^"+CommitPrefix)
	} else {
		args = append(args, "--", rel)
	}
	out, err := output(g.root, args...)
	if err != nil || strings.TrimSpace(out) == "" {
		return "", "", false, nil
	}
	hash, subject, _ = strings.Cut(strings.TrimSpace(out), "\x00")
	return hash, subject, true, nil
}

// FilesIn lists the files a commit changed.
func (g *Git) FilesIn(hash string) ([]string, error) {
	out, err := output(g.root, "show", "--name-only", "--format=", hash)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			files = append(files, filepath.FromSlash(l))
		}
	}
	return files, nil
}

// FileAt is a file as a commit has it. exists is false when the commit does
// not have the file at all; "HEAD^" and the like are accepted as revisions.
func (g *Git) FileAt(rev, rel string) (data []byte, exists bool, err error) {
	cmd := exec.Command("git", "cat-file", "-e", rev+":"+filepath.ToSlash(rel))
	cmd.Dir = g.root
	if cmd.Run() != nil {
		return nil, false, nil
	}
	out, err := output(g.root, "show", rev+":"+filepath.ToSlash(rel))
	if err != nil {
		return nil, false, err
	}
	return []byte(out), true, nil
}

// Revert restores the notes directory to the last commit, discarding
// uncommitted changes. This is the undo of last resort.
func (g *Git) Revert() error {
	if !g.Enabled() {
		return fmt.Errorf("no git repository in the notes directory")
	}
	return run(g.root, "checkout", "--", ".")
}

func isRepo(root string) bool {
	st, err := os.Stat(filepath.Join(root, ".git"))
	return err == nil && (st.IsDir() || st.Mode().IsRegular())
}

func clean(root string) bool {
	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = root
	return cmd.Run() == nil
}

func run(root string, args ...string) error { return runTimeout(root, 0, args...) }

// runTimeout runs git, optionally bounded. Without a bound a push on a dead
// network can hang for minutes.
func runTimeout(root string, timeout time.Duration, args ...string) error {
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("git %s timed out after %s — offline?", args[0], timeout)
		}
		return fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func writeIfAbsent(path, content string) {
	if _, err := os.Stat(path); err == nil {
		return
	}
	_ = os.WriteFile(path, []byte(content), 0o644)
}
