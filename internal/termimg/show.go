package termimg

import (
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"time"

	// The formats a pasted or dropped picture comes in.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"

	"github.com/charmbracelet/x/term"
	"golang.org/x/sys/unix"
)

// ErrCannotShow says the picture was not drawn and nothing was written to the
// terminal: it could not be decoded, or the terminal cannot show it. The
// caller hands the file to the system's opener instead.
var ErrCannotShow = errors.New("cannot show this picture in the terminal")

// Viewer shows one picture full screen until a key is pressed. It is a
// bubbletea ExecCommand: the outliner lets go of the terminal while it runs,
// so nothing redraws over the picture and no key is read twice.
type Viewer struct {
	Path     string
	Title    string // the first line: the file's name
	Subtitle string // after it, dimmed: where the file is
	Protocol Protocol

	// Opened is set when the key pressed was o: open it outside after all.
	Opened bool

	in       io.Reader
	out, err io.Writer
}

func (v *Viewer) SetStdin(r io.Reader)  { v.in = r }
func (v *Viewer) SetStdout(w io.Writer) { v.out = w }
func (v *Viewer) SetStderr(w io.Writer) { v.err = w }

// Run draws the picture and waits for a key. Everything that can fail is
// tried before the screen is touched, so a failure leaves no flash behind and
// returns ErrCannotShow.
func (v *Viewer) Run() error {
	if v.Protocol == None {
		return ErrCannotShow
	}
	img, err := decode(v.Path)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCannotShow, err)
	}
	in, out := v.in, v.out
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = os.Stdout
	}
	inFile, ok := in.(*os.File)
	if !ok {
		return ErrCannotShow
	}
	cols, rows, cw, ch := size(inFile, out)
	if cols < 10 || rows < 4 {
		return ErrCannotShow
	}
	state, err := term.MakeRaw(inFile.Fd())
	if err != nil {
		return ErrCannotShow
	}
	defer func() { _ = term.Restore(inFile.Fd(), state) }()

	if v.Protocol == Ask {
		v.Protocol = probe(inFile, out)
		if v.Protocol == None {
			return ErrCannotShow
		}
	}
	pic, used, err := Encode(v.Protocol, img, Box{Cols: cols, Rows: rows - 2, CellW: cw, CellH: ch})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCannotShow, err)
	}

	// A screen of its own, no cursor, no mouse reports: a moving pointer
	// would otherwise arrive as input and count as the key that closes it.
	fmt.Fprint(out, "\x1b[?1049h\x1b[?25l\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[2J\x1b[H")
	title := clip(v.Title, cols)
	fmt.Fprintf(out, "\x1b[1m%s\x1b[0m", title)
	if rest := cols - len([]rune(title)) - 2; rest > 3 && v.Subtitle != "" {
		fmt.Fprintf(out, "  \x1b[2m%s\x1b[0m", clip(v.Subtitle, rest))
	}
	fmt.Fprintf(out, "\x1b[2;%dH", 1+(cols-used)/2)
	_, _ = out.Write(pic)
	fmt.Fprintf(out, "\x1b[%d;1H\x1b[2m%s\x1b[0m", rows, clip("o open outside tlog · any other key back", cols))

	buf := make([]byte, 64)
	n, _ := in.Read(buf)
	v.Opened = n > 0 && buf[0] == 'o'

	if v.Protocol == Kitty {
		fmt.Fprint(out, KittyClear)
	}
	fmt.Fprint(out, "\x1b[2J\x1b[?25h\x1b[?1049l")
	return nil
}

// probeWait is how long a terminal has to answer. A local one answers in
// microseconds; this is for ssh.
const probeWait = 600 * time.Millisecond

// probe asks the terminal which protocol it speaks and reads the answer,
// without a goroutine: one left blocked on the input would eat the outliner's
// next key. A terminal that does not answer in time cannot show pictures as
// far as tlog is concerned.
func probe(in *os.File, out io.Writer) Protocol {
	if _, err := io.WriteString(out, Questions); err != nil {
		return None
	}
	deadline := time.Now().Add(probeWait)
	var reply []byte
	buf := make([]byte, 256)
	for {
		left := time.Until(deadline)
		if left <= 0 || !readable(in, left) {
			return None
		}
		n, err := in.Read(buf)
		if err != nil {
			return None
		}
		reply = append(reply, buf[:n]...)
		if p, done := Answer(string(reply)); done {
			return p
		}
	}
}

// readable waits until the input has something to read, or the time is up.
// select rather than poll, because poll does not work on a tty on macOS.
func readable(f *os.File, wait time.Duration) bool {
	fd := int(f.Fd())
	var set unix.FdSet
	set.Set(fd)
	tv := unix.NsecToTimeval(wait.Nanoseconds())
	n, err := unix.Select(fd+1, &set, nil, nil, &tv)
	return err == nil && n > 0
}

func decode(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

// size is the terminal in cells and, where it says, a cell in pixels.
func size(in *os.File, out io.Writer) (cols, rows, cw, ch int) {
	fds := []uintptr{in.Fd()}
	if f, ok := out.(*os.File); ok {
		fds = append(fds, f.Fd())
	}
	for _, fd := range fds {
		ws, err := unix.IoctlGetWinsize(int(fd), unix.TIOCGWINSZ)
		if err != nil || ws.Col == 0 || ws.Row == 0 {
			continue
		}
		cols, rows = int(ws.Col), int(ws.Row)
		if ws.Xpixel > 0 && ws.Ypixel > 0 {
			cw, ch = int(ws.Xpixel)/cols, int(ws.Ypixel)/rows
		}
		return cols, rows, cw, ch
	}
	return 0, 0, 0, 0
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n < 2 {
		return string(r[:max(0, n)])
	}
	return string(r[:n-1]) + "…"
}
