package termimg

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"fmt"
	"image"
	"image/color/palette"
	"image/png"
	"strings"

	"golang.org/x/image/draw"
)

// Box is where a picture may go: a number of cells, and how many pixels a cell
// is. A terminal that does not say its cell size gets a guess, which only
// changes how sharp the picture is, never whether it fits.
type Box struct {
	Cols, Rows   int
	CellW, CellH int
}

// guessCell is a cell's size in pixels when the terminal does not report it:
// a common size for a 10–11 pt font. Being wrong makes a picture a little
// soft or a little large in pixels; the cell count still fits the box.
const guessCellW, guessCellH = 10, 20

func (b Box) cell() (int, int) {
	if b.CellW <= 0 || b.CellH <= 0 {
		return guessCellW, guessCellH
	}
	return b.CellW, b.CellH
}

// Fit scales a picture of w×h pixels to fit the box, keeping its proportions
// and never enlarging it, and says how many pixels and how many cells that is.
func Fit(w, h int, b Box) (pw, ph, cols, rows int) {
	cw, ch := b.cell()
	maxW, maxH := max(1, b.Cols*cw), max(1, b.Rows*ch)
	pw, ph = w, h
	if pw > maxW {
		pw, ph = maxW, max(1, h*maxW/w)
	}
	if ph > maxH {
		pw, ph = max(1, w*maxH/h), maxH
	}
	cols = min(b.Cols, (pw+cw-1)/cw)
	rows = min(b.Rows, (ph+ch-1)/ch)
	return pw, ph, max(1, cols), max(1, rows)
}

// Encode is the bytes that draw img at the cursor, scaled into the box, and
// the number of columns it takes, so a caller can centre it.
func Encode(p Protocol, img image.Image, b Box) ([]byte, int, error) {
	bounds := img.Bounds()
	pw, ph, cols, rows := Fit(bounds.Dx(), bounds.Dy(), b)
	scaled := scale(img, pw, ph)
	switch p {
	case Kitty:
		return kitty(scaled, cols, rows), cols, nil
	case ITerm:
		out, err := iterm(scaled, cols, rows)
		return out, cols, err
	case Sixel:
		return sixel(scaled), cols, nil
	}
	return nil, 0, fmt.Errorf("this terminal cannot show pictures")
}

// scale resizes to exactly w×h. Pictures are only ever made smaller here, and
// Catmull-Rom is what keeps text in a screenshot legible when they are.
func scale(img image.Image, w, h int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	if img.Bounds().Dx() == w && img.Bounds().Dy() == h {
		draw.Draw(dst, dst.Bounds(), img, img.Bounds().Min, draw.Src)
		return dst
	}
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Src, nil)
	return dst
}

// kittyChunk is the most base64 a single kitty escape may carry.
const kittyChunk = 4096

// kitty sends raw RGBA, zlib-compressed, in chunks. q=2 asks the terminal not
// to answer: an answer would arrive as input and be read as keys.
func kitty(img *image.NRGBA, cols, rows int) []byte {
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	_, _ = zw.Write(img.Pix)
	_ = zw.Close()
	data := base64.StdEncoding.EncodeToString(z.Bytes())

	var out bytes.Buffer
	b := img.Bounds()
	for i := 0; i < len(data) || i == 0; i += kittyChunk {
		end := min(i+kittyChunk, len(data))
		more := 0
		if end < len(data) {
			more = 1
		}
		if i == 0 {
			fmt.Fprintf(&out, "\x1b_Ga=T,f=32,o=z,q=2,s=%d,v=%d,c=%d,r=%d,m=%d;%s\x1b\\",
				b.Dx(), b.Dy(), cols, rows, more, data[i:end])
		} else {
			fmt.Fprintf(&out, "\x1b_Gm=%d;%s\x1b\\", more, data[i:end])
		}
	}
	return out.Bytes()
}

// KittyClear removes every picture kitty is showing, so none is left behind
// when the outliner redraws.
const KittyClear = "\x1b_Ga=d,q=2\x1b\\"

// iterm sends a PNG and lets the terminal place it in the cells given.
func iterm(img *image.NRGBA, cols, rows int) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return []byte(fmt.Sprintf("\x1b]1337;File=inline=1;size=%d;width=%d;height=%d;preserveAspectRatio=1:%s\a",
		buf.Len(), cols, rows, base64.StdEncoding.EncodeToString(buf.Bytes()))), nil
}

// sixel draws in six-pixel-high bands, one colour at a time, from a 256-colour
// palette with error diffusion. Pixels that are mostly transparent are left
// unset, so the terminal's background shows through them.
func sixel(img *image.NRGBA) []byte {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	pal := image.NewPaletted(b, palette.Plan9)
	draw.FloydSteinberg.Draw(pal, b, img, b.Min)

	var out strings.Builder
	// P2=1: unset pixels stay as they are. Raster attributes give the size,
	// with square pixels.
	fmt.Fprintf(&out, "\x1bP0;1;0q\"1;1;%d;%d", w, h)
	used := map[uint8]bool{}
	for i, px := range pal.Pix {
		if img.Pix[i*4+3] >= 128 {
			used[px] = true
		}
	}
	for i := range palette.Plan9 {
		if !used[uint8(i)] {
			continue
		}
		r, g, bl, _ := palette.Plan9[i].RGBA()
		fmt.Fprintf(&out, "#%d;2;%d;%d;%d", i, r*100/0xffff, g*100/0xffff, bl*100/0xffff)
	}

	rows := map[uint8][]byte{}
	var order []uint8
	for y0 := 0; y0 < h; y0 += 6 {
		clear(rows)
		order = order[:0]
		for k := 0; k < 6 && y0+k < h; k++ {
			for x := 0; x < w; x++ {
				i := (y0+k)*w + x
				if img.Pix[i*4+3] < 128 {
					continue
				}
				c := pal.Pix[i]
				row, ok := rows[c]
				if !ok {
					row = make([]byte, w)
					rows[c] = row
					order = append(order, c)
				}
				row[x] |= 1 << k
			}
		}
		for n, c := range order {
			if n > 0 {
				out.WriteByte('$') // back to the start of the band
			}
			fmt.Fprintf(&out, "#%d", c)
			writeRuns(&out, rows[c])
		}
		out.WriteByte('-') // the next band
	}
	out.WriteString("\x1b\\")
	return []byte(out.String())
}

// writeRuns writes one colour's row of a band, run-length encoded.
func writeRuns(out *strings.Builder, row []byte) {
	for x := 0; x < len(row); {
		n := 1
		for x+n < len(row) && row[x+n] == row[x] {
			n++
		}
		ch := byte(63 + row[x])
		if n > 3 {
			fmt.Fprintf(out, "!%d%c", n, ch)
		} else {
			for range n {
				out.WriteByte(ch)
			}
		}
		x += n
	}
}
