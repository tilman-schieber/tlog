// Package app is the core. Every semantic operation lives here; the TUI and
// the CLI are adapters over it and hold no logic of their own. If a thing can
// be done in the TUI and not from here, that is a bug.
package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/tilman-schieber/tlog/internal/config"
	"github.com/tilman-schieber/tlog/internal/graph"
	"github.com/tilman-schieber/tlog/internal/importer"
	"github.com/tilman-schieber/tlog/internal/markdown"
	"github.com/tilman-schieber/tlog/internal/store"
)

// Service is the application core.
type Service struct {
	Store *store.Store
	Git   *store.Git
	Cfg   config.Config

	// CfgErr is a broken config file, kept rather than returned: a typo in a
	// setting must not stop someone writing a note.
	CfgErr error
}

// New opens the notes directory, creating it and its git repository if needed.
// An empty root means the configured one, which means the default one.
func New(root string) (*Service, error) {
	cfg, cfgErr := config.Load()
	if root == "" {
		root = cfg.NotesDir()
	}
	s, err := store.Open(root)
	if err != nil {
		return nil, err
	}
	g := store.NewGit(s.Root)
	g.SetDebounce(cfg.DebounceDuration())
	markdown.SetBlankLines(cfg.Format.BlankLines)
	return &Service{Store: s, Git: g, Cfg: cfg, CfgErr: cfgErr}, nil
}

// Reconfigure applies changed settings and writes them to the config file.
// Everything that can take effect without a restart does.
func (s *Service) Reconfigure(cfg config.Config) error {
	if err := cfg.Save(); err != nil {
		return err
	}
	s.Cfg = cfg
	s.CfgErr = nil
	s.Git.SetDebounce(cfg.DebounceDuration())
	markdown.SetBlankLines(cfg.Format.BlankLines)
	return nil
}

// ConfigPath is where the settings live, shown so it is never a mystery.
func (s *Service) ConfigPath() string { return config.Path() }

// Graph builds a fresh in-memory graph of the whole notes directory.
func (s *Service) Graph() (*graph.Graph, error) {
	return graph.Build(s.Store)
}

// TodayRel is the relative path of today's journal, whether or not it exists.
func (s *Service) TodayRel() string { return s.Store.JournalRel(time.Now()) }

// JournalRel is the relative path of a given day's journal.
func (s *Service) JournalRel(t time.Time) string { return s.Store.JournalRel(t) }

// Doc is a loaded file plus the hash any write against it must match.
type Doc struct {
	Rel  string
	Doc  *markdown.Document
	Hash string
}

// Load reads and parses a file. A file that does not exist loads as an empty
// document, so creating and editing are one path.
func (s *Service) Load(rel string) (*Doc, error) {
	d, hash, err := s.Store.Parse(rel)
	if err != nil {
		return nil, err
	}
	return &Doc{Rel: rel, Doc: d, Hash: hash}, nil
}

// Save writes a document back in canonical form, but only if the file still
// matches the hash it was loaded with. On success the document's hash is
// updated so the caller can keep editing.
//
// A conflict is never merged: the file changed underneath, so the caller
// re-reads and decides what to do.
func (s *Service) Save(d *Doc) error {
	out := markdown.Render(d.Doc)
	if markdown.Hash(out) == d.Hash {
		return nil // the file on disk already says exactly this
	}
	if err := s.Store.Write(d.Rel, out, d.Hash); err != nil {
		return err
	}
	d.Hash = markdown.Hash(out)
	s.Git.Touch(d.Rel)
	return nil
}

// Commit flushes any pending auto-commit immediately, pushing too when this
// notes directory is set to.
func (s *Service) Commit() error { return s.Git.Flush() }

// Push sends the notes to their remote. Nothing pushes unless the notes
// directory has been told to: writing a note and publishing it are different
// acts, and only one of them is easy to take back.
func (s *Service) Push() error { return s.Git.Push() }

// SetAutoPush turns pushing-on-commit on or off for this notes directory.
func (s *Service) SetAutoPush(on bool) error { return s.Git.SetAutoPush(on) }

// SyncStatus describes where the notes stand with their remote.
type SyncStatus struct {
	Remote   bool   `json:"remote"`
	AutoPush bool   `json:"autoPush"`
	LastErr  string `json:"lastError,omitempty"`
}

// Sync reports the state of pushing, for an adapter to show.
func (s *Service) Sync() SyncStatus {
	st := SyncStatus{Remote: s.Git.HasRemote(), AutoPush: s.Git.AutoPush()}
	if err := s.Git.LastPushError(); err != nil {
		st.LastErr = err.Error()
	}
	return st
}

// ResolvePage returns the relative path of a page by name and whether it
// already exists. The page name is the filename, so there is never more than
// one page with a given name.
func (s *Service) ResolvePage(name string) (rel string, exists bool, err error) {
	return s.Store.ResolvePage(name)
}

// OpenPage resolves a page, creating an empty file for it if it does not exist.
// This is what following a [[link]] does: nothing is written until you actually
// go there.
func (s *Service) OpenPage(name string) (string, error) {
	rel, exists, err := s.ResolvePage(name)
	if err != nil {
		return "", err
	}
	if !exists {
		d := &Doc{Rel: rel, Doc: &markdown.Document{}, Hash: markdown.Hash(nil)}
		d.Doc.AppendChild(nil, &markdown.Block{})
		if err := s.Save(d); err != nil {
			return "", err
		}
	}
	return rel, nil
}

// Add appends a block to a file, creating the file if necessary. Nested text is
// supported: a multi-line string becomes one multi-line block, which is what
// pasting a snippet into a journal should do.
func (s *Service) Add(rel, text string) (*markdown.Block, error) {
	d, err := s.Load(rel)
	if err != nil {
		return nil, err
	}
	b := &markdown.Block{Text: strings.TrimRight(text, "\n")}
	// An untouched new file starts with a single empty block; reuse it rather
	// than leaving a blank bullet above the first real note.
	if len(d.Doc.Blocks) == 1 && d.Doc.Blocks[0].Text == "" && len(d.Doc.Blocks[0].Children) == 0 {
		d.Doc.Blocks[0].Text = b.Text
		b = d.Doc.Blocks[0]
	} else {
		d.Doc.AppendChild(nil, b)
	}
	if err := s.Save(d); err != nil {
		return nil, err
	}
	return b, nil
}

// AddToPage appends a block to a named page, creating the page if needed.
func (s *Service) AddToPage(name, text string) (string, error) {
	rel, _, err := s.ResolvePage(name)
	if err != nil {
		return "", err
	}
	if _, err := s.Add(rel, text); err != nil {
		return "", err
	}
	return rel, nil
}

// AddToday appends a block to today's journal.
func (s *Service) AddToday(text string) (string, error) {
	rel := s.TodayRel()
	if _, err := s.Add(rel, text); err != nil {
		return "", err
	}
	return rel, nil
}

// Import converts a Logseq graph into the notes directory and puts the result
// under version control. Importing is the one moment where having every file in
// git matters most, so it happens here rather than being left to the caller.
func (s *Service) Import(opt importer.Options) (*importer.Report, error) {
	rep, err := importer.Run(s.Store, opt)
	if err != nil {
		return rep, err
	}
	if !opt.DryRun {
		for _, rel := range rep.Written {
			s.Git.Touch(rel)
		}
	}
	return rep, nil
}

// Addr is a block address: where the block starts, in which file, and the hash
// of that file at the time the address was handed out. The hash is what makes
// the address safe to mutate through — if the file moved on, the write is
// refused instead of landing somewhere else.
type Addr struct {
	Rel    string
	Offset int
	Hash   string
}

// String renders an address in the form the CLI accepts.
func (a Addr) String() string { return fmt.Sprintf("%s:%d@%s", a.Rel, a.Offset, a.Hash) }

// ParseAddr reads the form produced by Addr.String.
func ParseAddr(s string) (Addr, error) {
	at := strings.LastIndex(s, "@")
	if at < 0 {
		return Addr{}, fmt.Errorf("address %q has no @hash; take it from search output", s)
	}
	rest, hash := s[:at], s[at+1:]
	colon := strings.LastIndex(rest, ":")
	if colon < 0 {
		return Addr{}, fmt.Errorf("address %q has no :offset", s)
	}
	var off int
	if _, err := fmt.Sscanf(rest[colon+1:], "%d", &off); err != nil {
		return Addr{}, fmt.Errorf("address %q has a bad offset", s)
	}
	return Addr{Rel: rest[:colon], Offset: off, Hash: hash}, nil
}

// Resolve turns an address back into a live block, refusing if the file has
// changed since the address was issued.
func (s *Service) Resolve(a Addr) (*Doc, *markdown.Block, error) {
	d, err := s.Load(a.Rel)
	if err != nil {
		return nil, nil, err
	}
	if a.Hash != "" && d.Hash != a.Hash {
		return nil, nil, &store.ErrConflict{Rel: a.Rel, Expected: a.Hash, Actual: d.Hash}
	}
	b := d.Doc.FindByOffset(a.Offset)
	if b == nil {
		return nil, nil, fmt.Errorf("no block at offset %d in %s", a.Offset, a.Rel)
	}
	return d, b, nil
}

// Anchor materialises a stable anchor for a block and returns a permanent
// reference to it. This is the only thing that ever writes an id into a file,
// and it happens because something asked to name the block, never because
// something read it.
func (s *Service) Anchor(a Addr) (markdown.Link, error) {
	d, b, err := s.Resolve(a)
	if err != nil {
		return markdown.Link{}, err
	}
	anchor := d.Doc.EnsureAnchor(b)
	if err := s.Save(d); err != nil {
		return markdown.Link{}, err
	}
	return markdown.Link{Page: store.PageName(d.Rel), Anchor: anchor}, nil
}
