package main

import (
	"net/http"
	"path/filepath"
	"strings"
)

// The window is served from its own origin, and WebKit will not load a
// file:// image into it — so an attachment linked the way att writes it,
// ![name](file:///…/store/name.png), drew as a broken image. The frontend
// asks for /file/<that path> instead and this answers, for files on the shelf
// and nothing else. The note keeps its file:// link: att reads the same notes.

const filePrefix = "/file/"

// shelfFiles serves files from the shelf, whose directory dir names. It is a
// function so a change of attachments.dir needs no restart to be honoured.
func shelfFiles(dir func() string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rest, ok := strings.CutPrefix(r.URL.Path, filePrefix)
		if !ok || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
			http.NotFound(w, r)
			return
		}
		path, ok := onShelf(dir(), "/"+rest)
		if !ok {
			http.Error(w, "not on the shelf", http.StatusForbidden)
			return
		}
		// Shown as a picture and nothing else: an SVG on the shelf is drawn,
		// never run.
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self' data:; style-src 'unsafe-inline'")
		http.ServeFile(w, r, path)
	})
}

// onShelf resolves a requested path and reports whether it is a file inside
// the shelf. Links are followed on both sides before comparing, so neither a
// ../ nor a symlink out of the shelf reaches anything else on the disk.
func onShelf(shelf, path string) (string, bool) {
	root, err := filepath.EvalSymlinks(shelf)
	if err != nil {
		return "", false
	}
	real, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(root, real)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return real, true
}
