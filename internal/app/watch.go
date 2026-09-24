package app

import (
	"context"

	"github.com/tilman-schieber/tlog/internal/watch"
)

// Watching the notes directory is opt-in: an adapter that wants to redraw when
// a file changes underneath asks for a channel, and one that does not pays
// nothing. A watcher that will not start is recorded rather than returned — a
// failure to notice other people's edits is not a reason to refuse to write a
// note — and a nil channel blocks forever, which is exactly today's behaviour.

// Change is a file in the notes directory that is no longer what it was.
type Change = watch.Change

// Watch starts watching the notes directory until the context is cancelled.
// It returns nil if the watcher could not be started, leaving WatchErr set.
func (s *Service) Watch(ctx context.Context) <-chan Change {
	w, err := watch.New(s.Store, s.Cfg.WatchInterval())
	if err != nil {
		s.WatchErr = err
		return nil
	}
	go w.Start(ctx)
	return w.Changes()
}

// Ours reports whether a change is one tlog itself wrote, which an adapter must
// not treat as an external edit — reloading its own save would be a loop.
func (s *Service) Ours(c Change) bool { return watch.Ours(s.Store, c) }
