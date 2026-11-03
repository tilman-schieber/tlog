package app

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tilman-schieber/tlog/internal/attach"
	"github.com/tilman-schieber/tlog/internal/markdown"
)

// Attachments used to live outside the notes, on att's shelf in ~/.att/store,
// and notes linked them by absolute file:// address. Those links break on any
// other machine, and the files never travelled with the notes. MigrateShelf
// brings them in: every file a note links on the old shelf is copied into
// assets/ and the link is rewritten to the relative one tlog writes now.

// Migration is what MigrateShelf did, or would do on a dry run.
type Migration struct {
	From    string          `json:"from"`
	DryRun  bool            `json:"dryRun"`
	Copied  []MigratedFile  `json:"copied"`  // each file once, however many links name it
	Notes   []string        `json:"notes"`   // the notes whose links were rewritten
	Links   int             `json:"links"`   // links rewritten, across all notes
	Missing []MissingTarget `json:"missing"` // links into the shelf whose file is not here
}

// MigratedFile is one file brought from the shelf into assets/.
type MigratedFile struct {
	From string `json:"from"`
	Rel  string `json:"rel"` // empty on a dry run, where nothing is copied
}

// MissingTarget is a link that points into the shelf at a file this machine
// does not have. It is left exactly as it was.
type MissingTarget struct {
	Note   string `json:"note"`
	Target string `json:"target"`
}

// DefaultShelf is where att kept its files.
func DefaultShelf() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".att", "store")
}

// MigrateShelf copies every shelf file a note links into assets/ and rewrites
// those links. Links elsewhere — a file:// address outside the shelf, a web
// address — are not touched, and a link to a shelf file that is not on this
// machine is reported and left alone, so running it where the files are not
// is harmless, and running it twice does nothing the second time.
//
// Each note is written through the same compare-and-swap as any edit: a note
// that changed while this ran is refused, not overwritten.
func (s *Service) MigrateShelf(from string, dryRun bool) (*Migration, error) {
	if from == "" {
		from = DefaultShelf()
	}
	from, err := filepath.Abs(from)
	if err != nil {
		return nil, err
	}
	m := &Migration{From: from, DryRun: dryRun, Copied: []MigratedFile{}, Notes: []string{}, Missing: []MissingTarget{}}

	rels, err := s.Store.List()
	if err != nil {
		return m, err
	}
	store := s.assets()
	copied := map[string]attach.Entry{} // source path → where it went
	seen := map[string]bool{}           // source paths counted in Copied

	for _, rel := range rels {
		d, err := s.Load(rel)
		if err != nil {
			return m, err
		}
		changed := 0
		var walkErr error
		d.Doc.Walk(func(b *markdown.Block) bool {
			links := b.URLLinks()
			// Back to front, so each splice leaves the spans before it valid.
			sort.Slice(links, func(i, j int) bool { return links[i].Start > links[j].Start })
			for _, l := range links {
				src, ok := shelfPath(l.URL, from)
				if !ok {
					continue
				}
				if info, err := os.Stat(src); err != nil || !info.Mode().IsRegular() {
					m.Missing = append(m.Missing, MissingTarget{Note: rel, Target: l.URL})
					continue
				}
				if !seen[src] {
					seen[src] = true
					m.Copied = append(m.Copied, MigratedFile{From: src})
				}
				changed++
				if dryRun {
					continue
				}
				e, ok := copied[src]
				if !ok {
					e, err = store.Add(src)
					if err != nil {
						walkErr = fmt.Errorf("%s: %w", src, err)
						return false
					}
					copied[src] = e
					s.Git.Touch(e.Rel)
					for i := range m.Copied {
						if m.Copied[i].From == src {
							m.Copied[i].Rel = e.Rel
						}
					}
				}
				b.Text = b.Text[:l.Start] + relink(b.Text[l.Start:l.End], l, e) + b.Text[l.End:]
			}
			return true
		})
		if walkErr != nil {
			return m, walkErr
		}
		if changed == 0 {
			continue
		}
		m.Notes = append(m.Notes, rel)
		m.Links += changed
		if !dryRun {
			if err := s.Save(d); err != nil {
				return m, fmt.Errorf("%s: %w", rel, err)
			}
		}
	}
	return m, nil
}

// shelfPath reports the local path a file:// link names, when it is inside the
// shelf.
func shelfPath(raw, shelf string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "file") {
		return "", false
	}
	p := filepath.Clean(filepath.FromSlash(u.Path))
	if !strings.HasPrefix(p, shelf+string(filepath.Separator)) {
		return "", false
	}
	return p, true
}

// relink rewrites one link to point into assets/. A [label](address) keeps its
// label and whether it was an image; only the address changes. A bare address
// becomes the link tlog would have written.
func relink(span string, l markdown.URLLink, e attach.Entry) string {
	if l.Bare {
		return attach.Link(e)
	}
	i := strings.LastIndex(span, l.URL)
	if i < 0 {
		return attach.Link(e)
	}
	return span[:i] + attach.Target(e) + span[i+len(l.URL):]
}
