package main

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
)

// The window is served from an origin of its own, and a note links its
// pictures relative to itself — ![name](../assets/name.png) — which means
// nothing to a page that is not a file on disk. The frontend asks for
// /assets/<name> instead, and this answers for files inside the notes'
// assets/ and nothing else. The note keeps its relative link.

const assetsPrefix = "/assets/"

// assetFiles serves the notes' assets directory, whose path dir names.
func assetFiles(dir func() string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rest, ok := strings.CutPrefix(r.URL.Path, assetsPrefix)
		if !ok || rest == "" || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
			http.NotFound(w, r)
			return
		}
		if unescaped, err := url.PathUnescape(rest); err == nil {
			rest = unescaped
		}
		path, ok := inside(dir(), filepath.Join(dir(), filepath.FromSlash(rest)))
		if !ok {
			http.Error(w, "not an attachment", http.StatusForbidden)
			return
		}
		// Shown as a picture and nothing more: an attached SVG is drawn, never
		// run, and nothing served here can reach the API.
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self' data:; style-src 'unsafe-inline'; sandbox")
		http.ServeFile(w, r, path)
	})
}

// inside resolves a path and reports whether it is a file within dir. Links
// are followed on both sides before comparing, so neither a ../ nor a symlink
// out of assets/ reaches anything else on the disk — the notes included.
func inside(dir, path string) (string, bool) {
	root, err := filepath.EvalSymlinks(dir)
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
