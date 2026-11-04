// Package app is the core. Every semantic operation lives here; the TUI and
// the CLI are adapters over it and hold no logic of their own. If a thing can
// be done in the TUI and not from here, that is a bug.
package app

import (
	"encoding/json"
	"fmt"
	"github.com/tilman-schieber/tlog/internal/clipboard"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tilman-schieber/tlog/internal/config"
	"github.com/tilman-schieber/tlog/internal/dates"
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

	// WatchErr is a file watcher that would not start, kept for the same
	// reason: not noticing someone else's edits is a smaller problem than
	// refusing to open.
	WatchErr error

	// Launch hands a link to the desktop. Nil means the system's own opener;
	// tests put a recorder here so that nothing actually opens.
	Launch func(target string) error

	// Clipboard reads the system clipboard. Nil means the real one; tests put
	// a fixed picture or text here.
	Clipboard func() (clipboard.Content, error)

	hist history // what Undo takes back; see undo.go
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
	g.SetAutoCommit(cfg.Git.AutoCommit)
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
	s.Git.SetAutoCommit(cfg.Git.AutoCommit)
	markdown.SetBlankLines(cfg.Format.BlankLines)
	return nil
}

// StartupRel is what to open: today's journal, or whatever was written last.
// "last" needs no state file — the most recently changed note is the answer.
func (s *Service) StartupRel() string {
	if !strings.EqualFold(s.Cfg.Startup, "last") {
		return s.TodayRel()
	}
	rels, err := s.Store.List()
	if err != nil {
		return s.TodayRel()
	}
	newest, best := "", time.Time{}
	for _, rel := range rels {
		st, err := os.Stat(s.Store.Abs(rel))
		if err != nil {
			continue
		}
		if st.ModTime().After(best) {
			newest, best = rel, st.ModTime()
		}
	}
	if newest == "" {
		return s.TodayRel()
	}
	return newest
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
	normalizeDates(d.Doc, time.Now())
	out := markdown.Render(d.Doc)
	if markdown.Hash(out) == d.Hash {
		return nil // the file on disk already says exactly this
	}
	// What the file says now, for undo. It is checked against the hash by the
	// write itself, so a file that moved is still refused rather than recorded.
	before, err := s.Store.Read(d.Rel)
	if err != nil {
		return err
	}
	if err := s.Store.Write(d.Rel, out, d.Hash); err != nil {
		return err
	}
	if before.Hash == d.Hash {
		s.record(d.Rel, before.Data, out)
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
	rel, exists, err = s.Store.ResolvePage(name)
	if err != nil || exists {
		return rel, exists, err
	}
	// No file by that name. Before deciding to create one, ask whether an
	// existing page answers to it: aliases: Ada on "Ada Lovelace" means
	// [[Ada]] is her, not a new page.
	//
	// The graph is only built on this branch, so the ordinary case — following
	// a link to a page that exists — costs nothing.
	g, gerr := graph.Build(s.Store)
	if gerr != nil {
		return rel, exists, nil // no aliases available; the name stands
	}
	if real, was := g.Canonical(name); was {
		return s.Store.ResolvePage(real)
	}
	return rel, exists, nil
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
// pasting a snippet into a journal should do. It returns where the block
// landed, so that the next operation can be addressed to it.
func (s *Service) Add(rel, text string) (*Result, error) {
	return s.addWith(rel, text, nil)
}

// addWith is Add with a chance to shape the block — make it a task, give it a
// deadline — before the one write.
func (s *Service) addWith(rel, text string, shape func(*markdown.Block)) (*Result, error) {
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
	if shape != nil {
		shape(b)
	}
	return s.commit(d, b)
}

// AddToPage appends a block to a named page, creating the page if needed, and
// returns the page's file. Add is the form that returns the block's address.
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

// AddTodo appends an open task to today's journal, with a deadline when one is
// given, and returns the journal's file. AddTodoTo returns the block's address.
func (s *Service) AddTodo(text, due string) (string, error) {
	rel := s.TodayRel()
	if _, err := s.AddTodoTo(rel, text, due); err != nil {
		return "", err
	}
	return rel, nil
}

// AddTodoTo appends an open task to a file, with a deadline when one is given.
// The deadline is typed shorthand — "fri", "in 3 days", "12.11" — and anything
// that does not parse is refused before anything is written: a silently wrong
// deadline is worse than an error. It is written to the configured property,
// the same one /deadline writes.
func (s *Service) AddTodoTo(rel, text, due string) (*Result, error) {
	var deadline time.Time
	if strings.TrimSpace(due) != "" {
		d, ok := dates.Parse(due, time.Now())
		if !ok {
			return nil, refuse("not a date: %q", due)
		}
		deadline = d
	}
	return s.addWith(rel, text, func(b *markdown.Block) {
		b.MakeTask()
		if !deadline.IsZero() {
			setDeadline(b, deadline, s.Cfg.DeadlineProperty())
		}
	})
}

// AddToday appends a block to today's journal and returns the journal's file.
func (s *Service) AddToday(text string) (string, error) {
	rel := s.TodayRel()
	if _, err := s.Add(rel, text); err != nil {
		return "", err
	}
	return rel, nil
}

// FindPage turns what was typed into a file, without creating one. A path is
// taken as a path; anything else is a page name, which may be an alias for a
// page actually called something else. A page that does not exist is refused
// rather than made, because reading is not a reason to write.
func (s *Service) FindPage(name string) (string, error) {
	name = strings.TrimSpace(name)
	if strings.HasSuffix(name, ".md") || strings.Contains(name, "/") {
		if err := s.Store.Check(name); err != nil {
			return "", err
		}
		if _, err := os.Stat(s.Store.Abs(name)); err != nil {
			return "", refuse("no file %s in the notes", name)
		}
		return name, nil
	}
	rel, exists, err := s.ResolvePage(name)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", refuse("no page called %q — `tlog open %s` would create it", name, name)
	}
	return rel, nil
}

// DayRel is the journal a day means, typed as a date, a shorthand — today,
// yesterday, fri, 12.11 — or a count of days from today: -1, +2.
func (s *Service) DayRel(spec string) (string, error) {
	spec = strings.TrimSpace(spec)
	now := time.Now()
	if spec == "" {
		return s.TodayRel(), nil
	}
	if n, err := strconv.Atoi(spec); err == nil {
		return s.JournalRel(now.AddDate(0, 0, n)), nil
	}
	day, ok := dates.Parse(spec, now)
	if !ok {
		return "", refuse("not a day: %q", spec)
	}
	return s.JournalRel(day), nil
}

// PushNow commits whatever is outstanding and pushes it, which is what every
// adapter means by "push".
func (s *Service) PushNow() error {
	if err := s.Commit(); err != nil {
		return err
	}
	return s.Push()
}

// AddAlias gives a page another name, in its frontmatter, the way /alias does.
func (s *Service) AddAlias(rel, alias string) error {
	alias = strings.TrimSpace(alias)
	if err := checkAlias(alias); err != nil {
		return err
	}
	d, err := s.Load(rel)
	if err != nil {
		return err
	}
	addAlias(d.Doc, alias)
	return s.Save(d)
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

// An address crosses the wire as the single string it prints as, rather than
// as three fields a caller would have to reassemble in the right order. A
// script or an agent copies what it was given straight into the next command,
// which is the only way an address is ever used.
func (a Addr) MarshalJSON() ([]byte, error) { return json.Marshal(a.String()) }

func (a *Addr) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	parsed, err := ParseAddr(s)
	if err != nil {
		return err
	}
	*a = parsed
	return nil
}

// ParseAddr reads the form produced by Addr.String. An address without its
// hash is refused: the hash is the whole safety story, and an address that
// skipped the check would be a way to overwrite whatever is at an offset now.
func ParseAddr(s string) (Addr, error) {
	at := strings.LastIndex(s, "@")
	if at < 0 {
		return Addr{}, fmt.Errorf("address %q has no @hash; take it from search output", s)
	}
	rest, hash := s[:at], s[at+1:]
	if strings.TrimSpace(hash) == "" {
		return Addr{}, fmt.Errorf("address %q has an empty hash; take it from search output", s)
	}
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
	if a.Hash == "" {
		return nil, nil, fmt.Errorf("%s has no hash; an address is only safe with the hash of the file it came from", a)
	}
	d, err := s.Load(a.Rel)
	if err != nil {
		return nil, nil, err
	}
	if d.Hash != a.Hash {
		return nil, nil, &store.ErrConflict{Rel: a.Rel, Expected: a.Hash, Actual: d.Hash}
	}
	b := d.Doc.FindByOffset(a.Offset)
	if b == nil {
		// The file is as it was read, so the offset was never a block: a
		// place that does not exist, not a race.
		return nil, nil, refuse("no block at offset %d in %s", a.Offset, a.Rel)
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
