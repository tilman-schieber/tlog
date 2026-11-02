package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestShelfFilesServeTheShelfAndNothingElse(t *testing.T) {
	shelf := t.TempDir()
	outside := t.TempDir()
	pic := filepath.Join(shelf, "Screenshot 1.png")
	if err := os.WriteFile(pic, []byte("\x89PNG\r\n\x1a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("no"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(shelf, "link.txt")); err != nil {
		t.Fatal(err)
	}
	h := shelfFiles(func() string { return shelf })

	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		// The frontend asks for what the link says, percent-encoded.
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, (&url.URL{Path: path}).EscapedPath(), nil))
		return w
	}

	if w := get("/file" + pic); w.Code != http.StatusOK || w.Body.String() != "\x89PNG\r\n\x1a\n" {
		t.Fatalf("a picture on the shelf: %d %q", w.Code, w.Body.String())
	}
	if w := get("/file" + pic); w.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("served without a policy")
	}
	for _, path := range []string{
		"/file" + secret,
		"/file" + shelf + "/../" + filepath.Base(outside) + "/secret.txt",
		"/file" + filepath.Join(shelf, "link.txt"),
		"/file" + shelf,
		"/file" + filepath.Join(shelf, "missing.png"),
		"/other" + pic,
	} {
		if w := get(path); w.Code == http.StatusOK {
			t.Errorf("%s was served", path)
		}
	}
}
