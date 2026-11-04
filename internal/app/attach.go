package app

import (
	"fmt"
	"github.com/tilman-schieber/tlog/internal/clipboard"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tilman-schieber/tlog/internal/attach"
)

// Attachments live inside the notes, in assets/, so that a picture pasted on
// one machine is there on the next: they are committed with the note that
// links them and pushed with everything else.

// AttachView is one stored file, ready to show and to insert.
type AttachView struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Rel  string `json:"rel"`
	Size string `json:"size"`
	Link string `json:"link"`
	When string `json:"when"`
}

func attachView(e attach.Entry) AttachView {
	return AttachView{
		Name: e.Name,
		Path: e.Path,
		Rel:  e.Rel,
		Size: attach.Human(e.Size),
		Link: e.Link,
		When: e.ModTime.Format("2006-01-02"),
	}
}

// AttachDir is where attachments are kept, shown so it is never a mystery.
func (s *Service) AttachDir() string { return s.assets().Dir() }

func (s *Service) assets() *attach.Store { return attach.Open(s.Store.Root) }

// Attachments lists what is attached, newest first, narrowed by a query.
func (s *Service) Attachments(query string, limit int) ([]AttachView, error) {
	all, err := s.assets().List()
	if err != nil {
		return nil, err
	}
	matched := attach.Match(all, query)
	if limit > 0 && len(matched) > limit {
		matched = matched[:limit]
	}
	out := make([]AttachView, 0, len(matched))
	for _, e := range matched {
		out = append(out, attachView(e))
	}
	return out, nil
}

// Attach copies files into the notes and returns them. The originals are left
// where they are. Each one is committed like a note, so it travels with them.
func (s *Service) Attach(paths ...string) ([]AttachView, error) {
	store := s.assets()
	opt := attach.Options{
		Sanitize:  s.Cfg.Attachments.Sanitize,
		Lowercase: s.Cfg.Attachments.Lowercase,
	}
	var out []AttachView
	for _, p := range paths {
		e, err := store.AddWith(p, opt)
		if err != nil {
			return out, fmt.Errorf("%s: %w", p, err)
		}
		s.Git.Touch(e.Rel)
		out = append(out, attachView(e))
	}
	return out, nil
}

// AttachTo copies files into the notes and writes their links into a file as
// one block, which is the whole point: the note and the file arrive together.
func (s *Service) AttachTo(rel string, paths ...string) ([]AttachView, error) {
	added, err := s.Attach(paths...)
	if err != nil {
		return added, err
	}
	if len(added) == 0 {
		return added, nil
	}
	links := make([]string, 0, len(added))
	for _, a := range added {
		links = append(links, a.Link)
	}
	if _, err := s.Add(rel, strings.Join(links, "\n")); err != nil {
		return added, err
	}
	return added, nil
}

// MaxImage is the largest picture taken from the clipboard. A screenshot of a
// large display is a few megabytes; this leaves room to spare while bounding
// what a stray paste can put into the notes.
const MaxImage = 50 << 20

// imageTypes is what a pasted picture may be, keyed by what its bytes say.
var imageTypes = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// ReadClipboard is what is on the system clipboard: a picture in preference to
// text. A terminal pastes only text, so this is how a screenshot reaches tlog.
func (s *Service) ReadClipboard() (clipboard.Content, error) {
	if s.Clipboard != nil {
		return s.Clipboard()
	}
	return clipboard.Read()
}

// AttachClipboard puts the picture on the clipboard into the notes, like
// AttachImage, and links it at the end of rel; with rel empty it only attaches,
// for a caller that puts the link somewhere itself.
func (s *Service) AttachClipboard(rel string, now time.Time) (AttachView, error) {
	c, err := s.ReadClipboard()
	if err != nil {
		return AttachView{}, err
	}
	if c.Image == nil {
		return AttachView{}, refuse("the clipboard holds no picture")
	}
	a, err := s.AttachImage(c.Image, now)
	if err != nil || rel == "" {
		return a, err
	}
	if _, err := s.Add(rel, a.Link); err != nil {
		return a, err
	}
	return a, nil
}

// AttachImage puts a picture that is not a file yet — a screenshot on the
// clipboard — into the notes, named for the moment it arrived, and returns it
// like anything else attached. What it is is read from the bytes rather than
// taken on trust, so a paste of anything else is refused; two in one second
// get two names, because nothing is ever overwritten.
func (s *Service) AttachImage(data []byte, now time.Time) (AttachView, error) {
	if len(data) == 0 {
		return AttachView{}, fmt.Errorf("the clipboard holds no picture")
	}
	if len(data) > MaxImage {
		return AttachView{}, fmt.Errorf("the picture is larger than %d MB", MaxImage>>20)
	}
	kind := http.DetectContentType(data)
	ext, ok := imageTypes[kind]
	if !ok {
		return AttachView{}, fmt.Errorf("not a picture tlog can show (%s)", kind)
	}

	// Attaching copies from a path, so the bytes become a file first, under
	// the name they should have; the copy in assets/ is the one that stays.
	tmp, err := os.MkdirTemp("", "tlog-paste-")
	if err != nil {
		return AttachView{}, err
	}
	defer os.RemoveAll(tmp)
	src := filepath.Join(tmp, "screenshot-"+now.Format("2006-01-02-150405")+ext)
	if err := os.WriteFile(src, data, 0o600); err != nil {
		return AttachView{}, err
	}
	added, err := s.Attach(src)
	if err != nil {
		return AttachView{}, err
	}
	return added[0], nil
}
