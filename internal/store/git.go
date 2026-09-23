package store

import (
	"bytes"
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

// Git is the durability layer. Structural edits rewrite whole files, so the
// only honest protection against tlog confidently writing the wrong thing is a
// history you can walk back. It replaces backup files entirely.
type Git struct {
	root    string
	enabled bool

	mu      sync.Mutex
	touched map[string]bool
	timer   *time.Timer
	wait    time.Duration
}

// NewGit prepares the notes directory for versioning, initialising a
// repository if there is not one already. When git is unavailable the
// committer degrades to doing nothing rather than failing writes.
func NewGit(root string) *Git {
	g := &Git{root: root, touched: map[string]bool{}, wait: DebounceDefault}
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
	return g
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
	cmd := exec.Command("git", "config", "--get", key)
	cmd.Dir = root
	out, err := cmd.Output()
	return err == nil && len(bytes.TrimSpace(out)) > 0
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

	if err := run(g.root, "add", "-A"); err != nil {
		return err
	}
	if clean(g.root) {
		return nil
	}
	return run(g.root, "commit", "-q", "-m", commitMessage(files))
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

func run(root string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
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
