package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func touch(t *testing.T, path string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// Every way a terminal writes a dropped path is read back to the same file.
func TestDroppedFilesReadsWhatTerminalsPaste(t *testing.T) {
	dir := t.TempDir()
	shot := touch(t, filepath.Join(dir, "Screenshot 2026-11-02 at 17.03.png"))
	plain := touch(t, filepath.Join(dir, "plain.pdf"))
	quote := touch(t, filepath.Join(dir, "it's.png"))

	esc := strings.ReplaceAll(shot, " ", `\ `)
	for _, c := range []struct {
		name, text string
		want       []string
	}{
		{"escaped with backslashes", esc, []string{shot}},
		{"in single quotes", "'" + shot + "'", []string{shot}},
		{"in double quotes", `"` + shot + `"`, []string{shot}},
		{"as a file URL", "file://" + strings.ReplaceAll(shot, " ", "%20"), []string{shot}},
		{"plainly, with spaces", shot, []string{shot}},
		{"with a trailing space", esc + " ", []string{shot}},
		{"several, escaped", esc + " " + plain, []string{shot, plain}},
		{"several, a line each", shot + "\n" + plain, []string{shot, plain}},
		{"a quote escaped", strings.ReplaceAll(quote, "'", `\'`), []string{quote}},
	} {
		got := DroppedFiles(c.text)
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("%s: %q gave %q", c.name, c.text, got)
		}
	}
}

// Anything that is not entirely existing files is ordinary pasted text.
func TestDroppedFilesLeavesTextAlone(t *testing.T) {
	dir := t.TempDir()
	real := touch(t, filepath.Join(dir, "real.png"))
	for _, text := range []string{
		"", "   ", "hello world", "see " + real,
		real + " " + filepath.Join(dir, "missing.png"),
		dir,        // a directory is not a file to attach
		"real.png", // relative: depends on where tlog was started
		"'" + real, // unbalanced
		"https://example.com/a.png",
	} {
		if got := DroppedFiles(text); got != nil {
			t.Errorf("%q was taken for files: %q", text, got)
		}
	}
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestAttachImagePutsAScreenshotOnTheShelf(t *testing.T) {
	t.Setenv("ATT_DIR", t.TempDir())
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 2, 17, 3, 12, 0, time.Local)
	v, err := s.AttachImage(pngBytes(t), at)
	if err != nil {
		t.Fatal(err)
	}
	if v.Name != "screenshot-2026-11-02-170312.png" {
		t.Fatalf("name: %q", v.Name)
	}
	if !strings.HasPrefix(v.Link, "![screenshot-2026-11-02-170312.png](file://") {
		t.Fatalf("an image should be linked as one: %q", v.Link)
	}
	if !strings.HasPrefix(v.Path, s.AttachDir()) {
		t.Fatalf("not on the shelf: %s", v.Path)
	}
	if data, _ := os.ReadFile(v.Path); !bytes.Equal(data, pngBytes(t)) {
		t.Fatal("the picture was not stored as it came")
	}

	// A second one in the same second does not overwrite the first.
	v2, err := s.AttachImage(pngBytes(t), at)
	if err != nil {
		t.Fatal(err)
	}
	if v2.Path == v.Path {
		t.Fatal("two pastes in one second landed on one file")
	}
}

func TestAttachImageRefusesWhatIsNotAPicture(t *testing.T) {
	t.Setenv("ATT_DIR", t.TempDir())
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{nil, []byte("just some text"), []byte("<html><script>")} {
		if _, err := s.AttachImage(data, time.Now()); err == nil {
			t.Errorf("%q was stored as a picture", data)
		}
	}
	if entries, _ := os.ReadDir(s.AttachDir()); len(entries) != 0 {
		t.Fatal("a refused paste left something on the shelf")
	}
}
