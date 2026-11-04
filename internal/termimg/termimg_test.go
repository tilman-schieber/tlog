package termimg

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func env(kv ...string) func(string) string {
	m := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	return func(k string) string { return m[k] }
}

func TestDetectReadsTheTerminalFromItsEnvironment(t *testing.T) {
	for _, c := range []struct {
		name string
		env  func(string) string
		want Protocol
	}{
		{"foot", env("TERM", "foot"), Sixel},
		{"foot-extra", env("TERM", "foot-extra"), Sixel},
		{"kitty", env("TERM", "xterm-kitty"), Kitty},
		{"kitty by window id", env("TERM", "xterm-256color", "KITTY_WINDOW_ID", "1"), Kitty},
		{"ghostty", env("TERM", "xterm-ghostty", "TERM_PROGRAM", "ghostty"), Kitty},
		{"iTerm2", env("TERM", "xterm-256color", "TERM_PROGRAM", "iTerm.app"), ITerm},
		{"WezTerm", env("TERM", "xterm-256color", "TERM_PROGRAM", "WezTerm"), ITerm},
		{"Konsole", env("TERM", "xterm-256color", "KONSOLE_VERSION", "240202"), Sixel},
		{"alacritty: ask it", env("TERM", "alacritty"), Ask},
		{"foot as xterm-256color: ask it", env("TERM", "xterm-256color", "COLORTERM", "truecolor"), Ask},
		{"kitty inside tmux", env("TERM", "tmux-256color", "TMUX", "/tmp/x", "KITTY_WINDOW_ID", "1"), None},
		{"forced", env("TERM", "alacritty", "TLOG_IMAGES", "sixel"), Sixel},
		{"forced off", env("TERM", "xterm-kitty", "TLOG_IMAGES", "none"), None},
		{"forced through tmux", env("TMUX", "/tmp/x", "TLOG_IMAGES", "kitty"), Kitty},
	} {
		if got := Detect(c.env); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestAnswerReadsWhatTheTerminalSaid(t *testing.T) {
	for _, c := range []struct {
		name, reply string
		want        Protocol
		done        bool
	}{
		{"foot", "\x1bP>|foot(1.21.0)\x1b\\\x1b[?62;4;22c", Sixel, true},
		{"kitty", "\x1bP>|kitty(0.39.1)\x1b\\\x1b_Gi=31;OK\x1b\\\x1b[?62;c", Kitty, true},
		{"ghostty", "\x1bP>|ghostty 1.2.0\x1b\\\x1b_Gi=31;OK\x1b\\\x1b[?62;22c", Kitty, true},
		{"WezTerm", "\x1bP>|WezTerm 20240203\x1b\\\x1b_Gi=31;OK\x1b\\\x1b[?65;4;6;18;22c", ITerm, true},
		{"iTerm2", "\x1bP>|iTerm2 3.5.0\x1b\\\x1b[?62;4c", ITerm, true},
		{"alacritty", "\x1b[?6c", None, true},
		{"not finished", "\x1bP>|foot(1.21.0)\x1b\\", None, false},
	} {
		got, done := Answer(c.reply)
		if got != c.want || done != c.done {
			t.Errorf("%s: got %v, %v; want %v, %v", c.name, got, done, c.want, c.done)
		}
	}
}

func TestFitKeepsProportionsAndNeverEnlarges(t *testing.T) {
	b := Box{Cols: 80, Rows: 22, CellW: 10, CellH: 20} // 800×440 px
	for _, c := range []struct{ w, h, pw, ph, cols, rows int }{
		{1600, 800, 800, 400, 80, 20}, // wide: limited by width
		{400, 1600, 110, 440, 11, 22}, // tall: limited by height
		{100, 50, 100, 50, 10, 3},     // small: left as it is
	} {
		pw, ph, cols, rows := Fit(c.w, c.h, b)
		if pw != c.pw || ph != c.ph || cols != c.cols || rows != c.rows {
			t.Errorf("%dx%d: got %dx%d px in %dx%d cells, want %dx%d px in %dx%d", c.w, c.h, pw, ph, cols, rows, c.pw, c.ph, c.cols, c.rows)
		}
	}
	// No cell size from the terminal: a guess, and it still fits the cells.
	_, _, cols, rows := Fit(4000, 3000, Box{Cols: 50, Rows: 10})
	if cols > 50 || rows > 10 {
		t.Errorf("overflowed the box: %dx%d", cols, rows)
	}
}

func picture(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.NRGBA{uint8(x * 7), uint8(y * 5), uint8(x ^ y), 255})
		}
	}
	return img
}

func TestKittyIsChunkedAndAsksForNoReply(t *testing.T) {
	out, cols, err := Encode(Kitty, picture(300, 200), Box{Cols: 40, Rows: 20, CellW: 10, CellH: 20})
	if err != nil || cols != 30 {
		t.Fatalf("cols %d, %v", cols, err)
	}
	chunks := regexp.MustCompile("\x1b_G([^;]*);([^\x1b]*)\x1b\\\\").FindAllStringSubmatch(string(out), -1)
	if len(chunks) < 2 {
		t.Fatalf("expected the picture in several chunks, got %d", len(chunks))
	}
	first := chunks[0][1]
	for _, want := range []string{"a=T", "f=32", "q=2", "s=300", "v=200", "c=30", "r=10", "m=1"} {
		if !strings.Contains(first, want) {
			t.Errorf("first chunk %q lacks %s", first, want)
		}
	}
	for i, c := range chunks {
		if len(c[2]) > kittyChunk {
			t.Errorf("chunk %d carries %d bytes", i, len(c[2]))
		}
		last := i == len(chunks)-1
		if strings.Contains(c[1], "m=0") != last {
			t.Errorf("chunk %d of %d says %q", i, len(chunks), c[1])
		}
	}
}

func TestITermGetsAPNGInTheCellsGiven(t *testing.T) {
	out, _, err := Encode(ITerm, picture(50, 50), Box{Cols: 40, Rows: 20, CellW: 10, CellH: 20})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.HasPrefix(s, "\x1b]1337;File=inline=1;") || !strings.HasSuffix(s, "\a") ||
		!strings.Contains(s, "width=5;height=3;preserveAspectRatio=1:iVBOR") {
		t.Fatalf("got %.120q", s)
	}
}

func TestSixelDrawsBandsAndLeavesTransparencyUnset(t *testing.T) {
	out, _, err := Encode(Sixel, picture(20, 13), Box{Cols: 40, Rows: 20, CellW: 10, CellH: 20})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.HasPrefix(s, "\x1bP0;1;0q\"1;1;20;13#") || !strings.HasSuffix(s, "\x1b\\") {
		t.Fatalf("got %.80q … %q", s, s[len(s)-10:])
	}
	if bands := strings.Count(s, "-"); bands != 3 { // 13 rows: three bands of six
		t.Errorf("%d bands", bands)
	}

	clear := image.NewNRGBA(image.Rect(0, 0, 8, 8)) // all transparent
	out, _, _ = Encode(Sixel, clear, Box{Cols: 40, Rows: 20, CellW: 10, CellH: 20})
	if strings.Contains(string(out), "#") {
		t.Errorf("a transparent picture set colours: %q", out)
	}
}

func TestTheViewerGivesUpBeforeTouchingTheScreen(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "plot.png")
	var buf bytes.Buffer
	if err := png.Encode(&buf, picture(4, 4)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(good, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(dir, "broken.png")
	if err := os.WriteFile(broken, []byte("\x89PNG not really"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		name string
		v    *Viewer
	}{
		{"no protocol", &Viewer{Path: good, Protocol: None}},
		{"not a picture", &Viewer{Path: broken, Protocol: Kitty}},
		{"not a terminal", &Viewer{Path: good, Protocol: Kitty}},
	} {
		var out bytes.Buffer
		c.v.SetStdin(strings.NewReader("q"))
		c.v.SetStdout(&out)
		if err := c.v.Run(); !errors.Is(err, ErrCannotShow) {
			t.Errorf("%s: got %v", c.name, err)
		}
		if out.Len() != 0 {
			t.Errorf("%s: wrote %q before giving up", c.name, out.String())
		}
	}
}
