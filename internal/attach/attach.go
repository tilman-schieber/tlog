// Package attach keeps files inside the notes: pasted pictures, dropped
// documents, anything a note links to. They live in assets/ beside journals/
// and pages/, so they are committed, pushed and pulled with the notes that
// mention them, and a copy of the notes directory is a copy of everything.
//
// The rules are few and do not move: a file keeps its name, a clash gets a
// numeric suffix, nothing is ever overwritten, and the link is relative —
// ../assets/name — which is right from every note, because notes live exactly
// one directory down, and which any markdown reader follows.
package attach

import (
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/tilman-schieber/tlog/internal/store"
)

// Dir is the directory inside the notes that holds attachments. The store
// owns the name, because it is the store that must not mistake one for a note.
const Dir = store.AssetsDir

// Entry is one stored file.
type Entry struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Rel     string    `json:"rel"` // relative to the notes directory, for git
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
	Link    string    `json:"link"`
}

// Store is the assets directory of one notes directory.
type Store struct{ Root string }

// Open returns the attachments of the notes directory at root. It does not
// create anything: a missing directory simply means there are none yet.
func Open(root string) *Store { return &Store{Root: root} }

// Dir is where the files themselves live.
func (s *Store) Dir() string { return filepath.Join(s.Root, Dir) }

// List returns every attachment, newest first, because the thing you
// want to link is almost always the thing you just put there.
func (s *Store) List() ([]Entry, error) {
	entries, err := os.ReadDir(s.Dir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Entry
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, newEntry(filepath.Join(s.Dir(), e.Name()), info))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	return out, nil
}

func newEntry(path string, info fs.FileInfo) Entry {
	name := filepath.Base(path)
	e := Entry{Name: name, Path: path, Rel: Dir + "/" + name, Size: info.Size(), ModTime: info.ModTime()}
	e.Link = Link(e)
	return e
}

// Add copies a file into the notes and returns it. The original is left alone:
// attaching a file to a note should not move it out from under whatever else
// refers to it.
func (s *Store) Add(src string) (Entry, error) { return s.add(src, filepath.Base(src)) }

func (s *Store) add(src, name string) (Entry, error) {
	info, err := os.Stat(src)
	if err != nil {
		return Entry{}, err
	}
	if !info.Mode().IsRegular() {
		return Entry{}, fmt.Errorf("%s: not a regular file", src)
	}
	if err := os.MkdirAll(s.Dir(), 0o755); err != nil {
		return Entry{}, err
	}

	f, dst, err := reserve(s.Dir(), name)
	if err != nil {
		return Entry{}, err
	}
	if err := copyInto(f, src); err != nil {
		f.Close()
		os.Remove(dst)
		return Entry{}, err
	}
	if err := f.Close(); err != nil {
		os.Remove(dst)
		return Entry{}, err
	}

	// Stamp with the moment it arrived, so the newest is the one just added
	// rather than whenever the original happened to be written.
	now := time.Now()
	_ = os.Chtimes(dst, now, now)

	st, err := os.Stat(dst)
	if err != nil {
		return Entry{}, err
	}
	return newEntry(dst, st), nil
}

// reserve claims a free name, creating the file exclusively so that two writers
// can never pick the same one.
func reserve(dir, name string) (*os.File, string, error) {
	for n := 1; ; n++ {
		path := filepath.Join(dir, candidate(name, n))
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if os.IsExist(err) {
			continue
		}
		return f, path, err
	}
}

// candidate is the naming rule: report.pdf, then report-2.pdf, report-3.pdf.
func candidate(name string, n int) string {
	if n <= 1 {
		return name
	}
	ext := filepath.Ext(name)
	return fmt.Sprintf("%s-%d%s", strings.TrimSuffix(name, ext), n, ext)
}

func copyInto(dst *os.File, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	_, err = io.Copy(dst, in)
	return err
}

var imageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".svg": true,
}

var labelEscaper = strings.NewReplacer(`\`, `\\`, `[`, `\[`, `]`, `\]`)

// Link renders the Markdown link a note uses: an image becomes an embed, the
// path is relative to the note — every note is one directory below the notes
// root — and percent-encoded, so a space or a bracket in a filename does not
// break the link.
func Link(e Entry) string {
	prefix := ""
	if imageExts[strings.ToLower(filepath.Ext(e.Name))] {
		prefix = "!"
	}
	return fmt.Sprintf("%s[%s](%s)", prefix, labelEscaper.Replace(e.Name), Target(e))
}

// Target is the address part of the link: ../assets/name, percent-encoded.
func Target(e Entry) string {
	u := url.URL{Path: "../" + Dir + "/" + e.Name}
	return u.String()
}

// Match finds attachments by name: everything containing the query, and if
// nothing does, everything whose letters appear in order.
func Match(entries []Entry, query string) []Entry {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return entries
	}
	var contains, fuzzy []Entry
	for _, e := range entries {
		name := strings.ToLower(e.Name)
		switch {
		case strings.Contains(name, q):
			contains = append(contains, e)
		case subsequence(name, q):
			fuzzy = append(fuzzy, e)
		}
	}
	if len(contains) > 0 {
		return contains
	}
	return fuzzy
}

func subsequence(s, q string) bool {
	i := 0
	for _, r := range s {
		if i < len(q) && rune(q[i]) == r {
			i++
		}
	}
	return i == len(q)
}

// Human renders a size the way a person reads one.
func Human(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for n/div >= unit && exp < 3 {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}

// Options change how a file is taken in.
type Options struct {
	// Sanitize tidies the name: "meeting notes (final).pdf" becomes
	// "meeting-notes-final.pdf". Off unless asked for: a file keeps the name
	// it was given.
	Sanitize bool
	// Lowercase goes further, and only applies with Sanitize.
	Lowercase bool
}

// AddWith is Add with the name tidied on the way in.
func (s *Store) AddWith(src string, opt Options) (Entry, error) {
	if !opt.Sanitize {
		return s.Add(src)
	}
	return s.add(src, Sanitize(filepath.Base(src), opt.Lowercase))
}

// Sanitize makes a filename pleasant to type, link and shell-quote: spaces and
// punctuation become single hyphens, and the extension is left alone.
//
// Letters are kept as they are, umlauts included. They are valid in filenames,
// they survive percent-encoding in a link, and mangling a German word to make
// it look like an English one helps nobody.
func Sanitize(name string, lower bool) string {
	ext := filepath.Ext(name)
	if ext == "." {
		ext = "" // a name that is only dots has no extension worth keeping
	}
	stem := strings.TrimSuffix(name, ext)

	var b strings.Builder
	dash := false
	for _, r := range stem {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			dash = false
		case r == '.' || r == '_' || r == '+':
			b.WriteRune(r)
			dash = false
		default:
			// Everything else — spaces, brackets, slashes, punctuation — is one
			// hyphen, however many of them there were.
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-._")
	if out == "" {
		out = "datei"
	}
	if lower {
		out = strings.ToLower(out)
		ext = strings.ToLower(ext)
	}
	return out + ext
}
