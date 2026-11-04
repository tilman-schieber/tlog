package app

import (
	"image"
	"os"
	"path/filepath"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// Picture is a linked file that is a picture tlog can decode, for an adapter
// that can draw one. Whether it can be drawn — whether the terminal speaks an
// image protocol — is the adapter's question; this only says what the file is.
type Picture struct {
	Path   string `json:"path"` // absolute
	Name   string `json:"name"`
	Format string `json:"format"` // png, jpeg, gif, webp, bmp, tiff
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// LinkedPicture reads what a link points at when it is a local picture. Like
// LinkedText it answers nil, with no error, for anything else, and an error
// only when the link cannot be followed at all. The file is recognised by its
// bytes, not its name: a screenshot saved without an extension is still one.
func (s *Service) LinkedPicture(fromRel, target string) (*Picture, error) {
	resolved, err := s.ResolveLink(fromRel, target)
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(resolved) {
		return nil, nil
	}
	return readPicture(resolved), nil
}

func readPicture(p string) *Picture {
	info, err := os.Stat(p)
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer f.Close()
	cfg, format, err := image.DecodeConfig(f)
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return nil
	}
	return &Picture{Path: p, Name: filepath.Base(p), Format: format, Width: cfg.Width, Height: cfg.Height}
}
