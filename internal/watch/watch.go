// Package watch notices when a file in the notes directory changes underneath
// tlog, so that an edit made in nvim shows up without anyone pressing R.
//
// It polls. On macOS fsnotify is kqueue: one file descriptor per watched file,
// no recursion, and re-registration after every atomic rename — and a notes
// directory is written by atomic rename. It would also need hand-written
// exclusions for .git, .tlog-*.tmp, editor swap files and nvim's 4913 probe,
// all of which store.List already gives for free. A stat sweep of a corpus this
// size costs tens of microseconds against the graph rebuild a save already
// pays, so the simpler thing is also the faster one here.
//
// The debounce is by content rather than by time: an atomic rename, an in-place
// write, a swap file and a git checkout all become the same question — are the
// bytes at this path different from the last time we looked? A burst of writes
// collapses into one change for free, and a write that restores identical bytes
// emits nothing at all.
package watch

import (
	"context"
	"time"

	"github.com/tilman-schieber/tlog/internal/markdown"
	"github.com/tilman-schieber/tlog/internal/store"
)

// Change is one file whose contents are no longer what they were.
type Change struct {
	Rel    string
	Hash   string
	Exists bool
}

// Watcher reports changes to the markdown files in a store.
type Watcher struct {
	store    *store.Store
	interval time.Duration
	seen     map[string]string // rel -> hash, as of the last sweep
	ch       chan Change
}

// DefaultInterval is a compromise: fast enough that switching from nvim to the
// outliner feels live, slow enough to be invisible in a profile.
const DefaultInterval = time.Second

// New returns a watcher primed with what is on disk now, so that the files
// already there are not reported as changes the moment it starts.
func New(s *store.Store, interval time.Duration) (*Watcher, error) {
	if interval <= 0 {
		interval = DefaultInterval
	}
	w := &Watcher{
		store:    s,
		interval: interval,
		seen:     map[string]string{},
		ch:       make(chan Change, 64),
	}
	seen, err := w.scan()
	if err != nil {
		return nil, err
	}
	w.seen = seen
	return w, nil
}

// Changes is the stream. It is buffered, and a full buffer drops the oldest
// change rather than blocking the sweep: a reader that far behind is going to
// reload anyway, and a stalled watcher is worse than a coarse one.
func (w *Watcher) Changes() <-chan Change { return w.ch }

// Start sweeps until the context is cancelled.
func (w *Watcher) Start(ctx context.Context) {
	t := time.NewTicker(w.interval)
	defer t.Stop()
	defer close(w.ch)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			for _, c := range w.sweep() {
				w.emit(c)
			}
		}
	}
}

func (w *Watcher) emit(c Change) {
	select {
	case w.ch <- c:
	default:
		select {
		case <-w.ch:
		default:
		}
		select {
		case w.ch <- c:
		default:
		}
	}
}

// sweep compares the store against what the last sweep saw. It is unexported
// and returns its changes, so tests can drive it directly with no goroutine,
// no ticker and no sleeping.
func (w *Watcher) sweep() []Change {
	now, err := w.scan()
	if err != nil {
		// A directory that cannot be listed right now is not a change; the next
		// sweep will say what is true then.
		return nil
	}
	var out []Change
	for rel, h := range now {
		if old, ok := w.seen[rel]; !ok || old != h {
			out = append(out, Change{Rel: rel, Hash: h, Exists: true})
		}
	}
	for rel := range w.seen {
		if _, ok := now[rel]; !ok {
			// The hash of emptiness, which is what the store reports for a file
			// that is not there — so Ours needs no special case for a deletion.
			out = append(out, Change{Rel: rel, Hash: markdown.Hash(nil), Exists: false})
		}
	}
	w.seen = now
	return out
}

// scan hashes every file the store knows about. What the store does not list —
// .git, temp files, editor droppings — is not part of the notes and cannot be
// a change to them.
func (w *Watcher) scan() (map[string]string, error) {
	rels, err := w.store.List()
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rels))
	for _, rel := range rels {
		f, err := w.store.Read(rel)
		if err != nil {
			// Read between a rename and its target existing. Leaving it out
			// means the next sweep reports it, which is the right answer one
			// interval later rather than a wrong one now.
			continue
		}
		out[rel] = f.Hash
	}
	return out, nil
}

// Ours reports whether a change is one tlog itself made, by comparing it with
// the last thing the store wrote to that path. This is what keeps a save from
// looping back as an external edit — and what makes the loop provably
// terminate, since tlog reloading its own write would write nothing new.
func Ours(s *store.Store, c Change) bool {
	h, ok := s.Wrote(c.Rel)
	return ok && h == c.Hash
}
