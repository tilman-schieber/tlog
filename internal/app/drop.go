package app

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Two ways a picture arrives that are not a file chosen from the shelf: a file
// dropped on a terminal, which the terminal hands over as pasted text, and a
// screenshot on the clipboard, which is bytes with no file at all. Both end on
// the shelf, as a file with a name, exactly like anything else attached.

// DroppedFiles reads what a terminal pastes when files are dropped on it, and
// returns the files, or nil when the text is not that.
//
// Terminals disagree about how to write a path: escaped with backslashes
// (`Screenshot\ 1.png`), in quotes, as a file:// URL, or plainly, one per
// line. Each way is tried, and an answer counts only when every part of it is
// an absolute path to a file that exists. Anything else — prose, a directory,
// a path that does not exist — is ordinary pasted text.
func DroppedFiles(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	for _, parts := range [][]string{shellWords(text), strings.Split(text, "\n")} {
		if files := existingFiles(parts); files != nil {
			return files
		}
	}
	return nil
}

func existingFiles(parts []string) []string {
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "file://") {
			u, err := url.Parse(p)
			if err != nil {
				return nil
			}
			p = u.Path
		}
		if !filepath.IsAbs(p) {
			return nil
		}
		st, err := os.Stat(p)
		if err != nil || !st.Mode().IsRegular() {
			return nil
		}
		out = append(out, p)
	}
	return out
}

// shellWords splits text the way a shell would read it: whitespace separates,
// quotes group, and a backslash takes the next character as it is.
func shellWords(text string) []string {
	var words []string
	var cur strings.Builder
	inWord := false
	var quote rune
	escaped := false
	for _, r := range text {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case quote != 0:
			if r == quote {
				quote = 0
			} else if r == '\\' && quote == '"' {
				escaped = true
			} else {
				cur.WriteRune(r)
			}
		case r == '\\':
			escaped, inWord = true, true
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if quote != 0 || escaped {
		return nil // unbalanced: not something a terminal wrote
	}
	if inWord {
		words = append(words, cur.String())
	}
	return words
}

// MaxImage is the largest picture taken from the clipboard. A screenshot of a
// large display is a few megabytes; this is room to spare, and a limit on what
// a stray paste can put on the shelf.
const MaxImage = 50 << 20

var imageTypes = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// AttachImage puts a picture that has no file — a screenshot on the clipboard —
// on the shelf, named for the moment it arrived. What it is is read from the
// bytes rather than taken on trust, so a paste of anything else is refused.
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

	// The shelf copies from a path, so the bytes become a file first, under
	// the name it should have; the copy on the shelf is the one that stays.
	tmp, err := os.MkdirTemp("", "tlog-paste-")
	if err != nil {
		return AttachView{}, err
	}
	defer os.RemoveAll(tmp)
	src := filepath.Join(tmp, "screenshot-"+now.Format("2006-01-02-150405")+ext)
	if err := os.WriteFile(src, data, 0o644); err != nil {
		return AttachView{}, err
	}
	added, err := s.Attach(src)
	if err != nil {
		return AttachView{}, err
	}
	return added[0], nil
}
