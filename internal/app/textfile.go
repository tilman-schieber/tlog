package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// A link to a plain-text file — an attached markdown document, a script, a
// CSV — is shown by tlog itself rather than handed to the desktop. The desktop
// often has nothing registered for text/markdown, so xdg-open would start and
// quietly do nothing, and reading a text file is something a terminal is
// already good at.

// TextFile is a linked file that is plain text, read for showing.
type TextFile struct {
	Path string `json:"path"` // absolute
	// Rel is Path from the notes directory, ../ and all when the file lies
	// outside it: what links inside the file are relative to.
	Rel  string `json:"rel"`
	Name string `json:"name"`
	Lang string `json:"lang"` // from the extension: "markdown", "go", …; empty if unknown
	Text string `json:"text"`
}

// maxTextFile bounds what is read for showing. A text file larger than this is
// a log or a data dump, which belongs in a pager or an editor, not in tlog.
const maxTextFile = 2 << 20

// LinkedText reads what a link points at when it is a local plain-text file.
// It answers nil, with no error, for anything tlog does not show itself: a web
// address, a picture, a PDF, a file too large to be read whole. An error means
// the link cannot be followed at all, as with OpenLink.
func (s *Service) LinkedText(fromRel, target string) (*TextFile, error) {
	resolved, err := s.ResolveLink(fromRel, target)
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(resolved) {
		return nil, nil // an address, not a file
	}
	f := readText(resolved)
	if f != nil {
		if rel, err := filepath.Rel(s.Store.Root, resolved); err == nil {
			f.Rel = filepath.ToSlash(rel)
		}
	}
	return f, nil
}

// readText is the file at p when it is plain text, otherwise nil. Plain text
// means valid UTF-8 with no NUL byte, whatever the extension says: an attached
// .md is text, and so is a README with none.
func readText(p string) *TextFile {
	info, err := os.Stat(p)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxTextFile {
		return nil
	}
	data, err := os.ReadFile(p)
	if err != nil || bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		return nil
	}
	text := strings.ReplaceAll(string(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))), "\r\n", "\n")
	return &TextFile{Path: p, Name: filepath.Base(p), Lang: langFor(p), Text: text}
}

// langFor names the language of a file by its extension, in the names the
// highlighter and code fences use.
func langFor(p string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(p), "."))
	switch ext {
	case "md", "markdown":
		return "markdown"
	case "":
		return ""
	}
	return ext
}

// Reread reads a shown file again after something else changed it: an editor
// it was handed to. A change inside the notes directory is committed with the
// rest, so an attached document edited by hand travels like a note does. It is
// an error for the file to have gone, or to have stopped being text.
func (s *Service) Reread(f *TextFile) (*TextFile, bool, error) {
	again := readText(f.Path)
	if again == nil {
		return f, false, fmt.Errorf("%s can no longer be shown here", f.Name)
	}
	again.Rel = f.Rel
	changed := again.Text != f.Text
	if changed && f.Rel != "" && !strings.HasPrefix(f.Rel, "../") && f.Rel != ".." {
		s.Git.Touch(f.Rel)
	}
	return again, changed, nil
}
