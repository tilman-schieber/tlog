// Package clipboard reads what is on the system clipboard: a picture if there
// is one, otherwise text — and puts text there. A terminal can only paste text, so a screenshot on
// the clipboard never reaches a terminal program through the terminal; this
// asks the system for it instead, through the tools every desktop already has.
//
// Linux: wl-paste on Wayland, xclip on X11. macOS: osascript for a picture,
// pbpaste for text. Nothing is linked against; a missing tool is an error that
// says which one to install.
package clipboard

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

// Content is what the clipboard held: a picture's bytes, or text, or neither.
type Content struct {
	Image []byte
	Type  string // the picture's MIME type, as the clipboard said
	Text  string
}

// ErrEmpty says the clipboard holds neither a picture nor text.
var ErrEmpty = errors.New("the clipboard is empty")

// imageTypes are tried in order of preference: a lossless screenshot first.
var imageTypes = []string{"image/png", "image/jpeg", "image/webp", "image/gif"}

// Read takes what is on the clipboard, a picture in preference to text.
func Read() (Content, error) {
	return read(runtime.GOOS, os.Getenv, run)
}

// runner runs a tool and returns what it printed; a seam for tests.
type runner func(name string, args ...string) ([]byte, error)

func run(name string, args ...string) ([]byte, error) {
	if _, err := exec.LookPath(name); err != nil {
		return nil, fmt.Errorf("reading the clipboard needs %s, which is not installed", name)
	}
	var out, errb bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s: %v %s", name, err, strings.TrimSpace(errb.String()))
	}
	return out.Bytes(), nil
}

func read(goos string, getenv func(string) string, run runner) (Content, error) {
	switch {
	case goos == "darwin":
		return readMac(run)
	case getenv("WAYLAND_DISPLAY") != "":
		return readWith(run, "wl-paste", []string{"--list-types"},
			func(t string) []string { return []string{"--no-newline", "--type", t} })
	case getenv("DISPLAY") != "":
		return readWith(run, "xclip", []string{"-selection", "clipboard", "-t", "TARGETS", "-o"},
			func(t string) []string { return []string{"-selection", "clipboard", "-t", t, "-o"} })
	}
	return Content{}, errors.New("no clipboard to read: neither Wayland nor X11 is running")
}

// readWith asks a Linux clipboard tool what types it holds, then takes the
// best picture, or else the text.
func readWith(run runner, tool string, listArgs []string, get func(string) []string) (Content, error) {
	listed, err := run(tool, listArgs...)
	if err != nil {
		return Content{}, err
	}
	types := map[string]bool{}
	for _, t := range strings.Fields(string(listed)) {
		types[t] = true
	}
	for _, t := range imageTypes {
		if types[t] {
			data, err := run(tool, get(t)...)
			if err != nil {
				return Content{}, err
			}
			if len(data) > 0 {
				return Content{Image: data, Type: t}, nil
			}
		}
	}
	// Files copied in a file manager are a list of file:// addresses, offered
	// before the plain text that names them.
	for _, t := range []string{"text/uri-list", "text/plain;charset=utf-8", "UTF8_STRING", "text/plain", "STRING", "TEXT"} {
		if types[t] {
			data, err := run(tool, get(t)...)
			if err != nil {
				return Content{}, err
			}
			if len(data) > 0 {
				return Content{Text: string(data)}, nil
			}
		}
	}
	return Content{}, ErrEmpty
}

// macPicture is how AppleScript prints a picture: «data PNGf89504E47…».
var macPicture = regexp.MustCompile(`«data PNGf([0-9A-Fa-f]+)»`)

func readMac(run runner) (Content, error) {
	if out, err := run("osascript", "-e", "get the clipboard as «class PNGf»"); err == nil {
		if m := macPicture.FindSubmatch(out); m != nil {
			if data, err := hex.DecodeString(string(m[1])); err == nil && len(data) > 0 {
				return Content{Image: data, Type: "image/png"}, nil
			}
		}
	}
	out, err := run("pbpaste")
	if err != nil {
		return Content{}, err
	}
	if len(out) == 0 {
		return Content{}, ErrEmpty
	}
	return Content{Text: string(out)}, nil
}

// Write puts text on the clipboard, with the same tools Read uses.
func Write(text string) error {
	return write(runtime.GOOS, os.Getenv, text, feed)
}

// feeder hands text to a tool on its standard input; a seam for tests.
type feeder func(text, name string, args ...string) error

// feed runs a tool with text on its input. Its output is not captured: wl-copy
// and xclip leave a process behind that serves the clipboard, and waiting for
// that process to close a pipe would wait forever.
func feed(text, name string, args ...string) error {
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("copying needs %s, which is not installed", name)
	}
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %v", name, err)
	}
	return nil
}

func write(goos string, getenv func(string) string, text string, feed feeder) error {
	switch {
	case goos == "darwin":
		return feed(text, "pbcopy")
	case getenv("WAYLAND_DISPLAY") != "":
		return feed(text, "wl-copy")
	case getenv("DISPLAY") != "":
		return feed(text, "xclip", "-selection", "clipboard")
	}
	return errors.New("no clipboard to copy to: neither Wayland nor X11 is running")
}
