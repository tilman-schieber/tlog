// Package store owns the notes directory on disk: where journals and pages
// live, how files are read and written, and the compare-and-swap rule that
// makes concurrent editing safe without a daemon or a lock.
package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tilman-schieber/tlog/internal/markdown"
)

const (
	JournalsDir = "journals"
	PagesDir    = "pages"
	JournalDate = "2006-01-02"
)

// ErrConflict is returned when a write is attempted against a file that has
// changed since it was read. It is never resolved by merging: the caller
// re-reads and decides.
type ErrConflict struct {
	Rel      string
	Expected string
	Actual   string
}

func (e *ErrConflict) Error() string {
	return fmt.Sprintf("%s changed on disk (expected %s, found %s); re-read and retry", e.Rel, e.Expected, e.Actual)
}

// ErrBadPageName is returned for page names that cannot be filenames.
var ErrBadPageName = errors.New("invalid page name")

// Store is a notes directory.
type Store struct {
	Root string

	// What tlog itself last wrote, per file. A watcher comparing the bytes on
	// disk against this can tell an edit made in nvim from tlog's own save,
	// and from the rewrite git does on a checkout. It belongs here rather than
	// in the watcher because every write goes through Write — the importer and
	// the anchor writer included — and none of them should have to remember.
	mu    sync.Mutex
	wrote map[string]string
}

// Wrote reports the hash tlog last wrote to a file, and whether it wrote one
// at all in this process.
func (s *Store) Wrote(rel string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.wrote[rel]
	return h, ok
}

func (s *Store) noteWrite(rel, hash string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.wrote == nil {
		s.wrote = map[string]string{}
	}
	s.wrote[rel] = hash
}

// DefaultRoot is $TLOG_DIR, or ~/notes.
func DefaultRoot() string {
	if d := os.Getenv("TLOG_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "notes"
	}
	return filepath.Join(home, "notes")
}

// Open returns a store rooted at dir, creating the directory layout if it is
// missing. A fresh notes directory is a valid notes directory.
func Open(dir string) (*Store, error) {
	if dir == "" {
		dir = DefaultRoot()
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	for _, d := range []string{abs, filepath.Join(abs, JournalsDir), filepath.Join(abs, PagesDir)} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	return &Store{Root: abs}, nil
}

// Abs turns a store-relative path into an absolute one.
func (s *Store) Abs(rel string) string { return filepath.Join(s.Root, rel) }

// JournalRel is the relative path of a day's journal.
func (s *Store) JournalRel(t time.Time) string {
	return filepath.Join(JournalsDir, t.Format(JournalDate)+".md")
}

// PageRel is the relative path of a named page. The page name is the filename:
// that is what makes duplicate page names impossible rather than merely
// handled.
func (s *Store) PageRel(name string) (string, error) {
	n := strings.TrimSpace(name)
	if n == "" || strings.ContainsAny(n, `/\`) || n == "." || n == ".." {
		return "", fmt.Errorf("%w: %q", ErrBadPageName, name)
	}
	if len(n) > 200 {
		return "", fmt.Errorf("%w: too long", ErrBadPageName)
	}
	return filepath.Join(PagesDir, n+".md"), nil
}

// PageName is the inverse of PageRel.
func PageName(rel string) string {
	return strings.TrimSuffix(filepath.Base(rel), ".md")
}

// IsJournal reports whether a relative path is a journal file.
func IsJournal(rel string) bool {
	return strings.HasPrefix(filepath.ToSlash(rel), JournalsDir+"/")
}

// ParseJournalName reports whether a bare name is a journal date.
func ParseJournalName(name string) (time.Time, bool) {
	t, err := time.Parse(JournalDate, strings.TrimSpace(name))
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// JournalDay parses the date out of a journal path.
func JournalDay(rel string) (time.Time, bool) {
	if !IsJournal(rel) {
		return time.Time{}, false
	}
	t, err := time.Parse(JournalDate, PageName(rel))
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// ResolvePage finds an existing page whose name matches case-insensitively,
// returning its relative path. The second result reports whether the page
// exists on disk; when it does not, the returned path is where it would go.
func (s *Store) ResolvePage(name string) (string, bool, error) {
	// A name that is a date is that day's journal, not a page that happens to
	// look like one. Without this, following [[2026-09-17]] or clicking a
	// journal in a sidebar silently creates pages/2026-09-17.md and shows it
	// empty, while the real day sits in journals/.
	if day, ok := ParseJournalName(name); ok {
		rel := s.JournalRel(day)
		_, err := os.Stat(s.Abs(rel))
		return rel, err == nil, nil
	}

	want, err := s.PageRel(name)
	if err != nil {
		return "", false, err
	}
	// The directory scan comes first even when a file of that exact name seems
	// to exist: on a case-insensitive filesystem os.Stat happily matches a
	// differently-cased file, and returning the requested casing rather than
	// the one on disk would let page names drift apart from the [[links]] that
	// point at them.
	entries, err := os.ReadDir(filepath.Join(s.Root, PagesDir))
	if err != nil {
		return want, false, nil
	}
	lower := strings.ToLower(strings.TrimSpace(name))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		if strings.ToLower(PageName(e.Name())) == lower {
			return filepath.Join(PagesDir, e.Name()), true, nil
		}
	}
	return want, false, nil
}

// File is a file on disk together with the hash that a later write must match.
type File struct {
	Rel  string
	Data []byte
	Hash string
}

// Read loads a file. A file that does not exist reads as empty, with the hash
// of emptiness, so that creating and updating are the same code path.
func (s *Store) Read(rel string) (*File, error) {
	data, err := os.ReadFile(s.Abs(rel))
	if err != nil {
		if os.IsNotExist(err) {
			return &File{Rel: rel, Data: nil, Hash: markdown.Hash(nil)}, nil
		}
		return nil, err
	}
	return &File{Rel: rel, Data: data, Hash: markdown.Hash(data)}, nil
}

// Parse reads and parses a file in one step.
func (s *Store) Parse(rel string) (*markdown.Document, string, error) {
	f, err := s.Read(rel)
	if err != nil {
		return nil, "", err
	}
	return markdown.Parse(f.Data), f.Hash, nil
}

// Write replaces a file, but only if its current contents still hash to
// ifMatch. Pass an empty ifMatch to write unconditionally, which is only
// correct for files nothing else could be holding.
//
// The write itself is a temp file plus a rename, so a crash mid-write leaves
// either the old file or the new one, never half of either.
func (s *Store) Write(rel string, data []byte, ifMatch string) error {
	abs := s.Abs(rel)

	if ifMatch != "" {
		cur, err := s.Read(rel)
		if err != nil {
			return err
		}
		if cur.Hash != ifMatch {
			return &ErrConflict{Rel: rel, Expected: ifMatch, Actual: cur.Hash}
		}
	}

	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}

	if len(data) == 0 {
		// An emptied file is removed rather than left as a husk, so the page
		// list never fills up with blanks.
		if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
			return err
		}
		s.noteWrite(rel, markdown.Hash(nil))
		return nil
	}

	tmp, err := os.CreateTemp(filepath.Dir(abs), ".tlog-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmpName, abs); err != nil {
		return err
	}
	s.noteWrite(rel, markdown.Hash(data))
	return nil
}

// List returns every markdown file in the store, journals first (newest to
// oldest), then pages alphabetically.
func (s *Store) List() ([]string, error) {
	var journals, pages []string
	err := filepath.WalkDir(s.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && path != s.Root {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		rel, rerr := filepath.Rel(s.Root, path)
		if rerr != nil {
			return nil
		}
		if IsJournal(rel) {
			journals = append(journals, rel)
		} else {
			pages = append(pages, rel)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(journals)))
	sort.Slice(pages, func(i, j int) bool {
		return strings.ToLower(pages[i]) < strings.ToLower(pages[j])
	})
	return append(journals, pages...), nil
}

// Title is the human name of a file: the date for a journal, the page name
// otherwise.
func Title(rel string) string { return PageName(rel) }
