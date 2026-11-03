package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// The handler is the one hole in the window's origin, so what it refuses
// matters more than what it serves.
func TestOnlyAssetsAreServed(t *testing.T) {
	notes := t.TempDir()
	assets := filepath.Join(notes, "assets")
	if err := os.MkdirAll(filepath.Join(notes, "pages"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(p, s string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(assets, "plan.png"), "PNG")
	write(filepath.Join(assets, "my notes.pdf"), "PDF")
	write(filepath.Join(notes, "pages", "Secret.md"), "no")
	if err := os.Symlink(filepath.Join(notes, "pages", "Secret.md"), filepath.Join(assets, "escape.txt")); err != nil {
		t.Fatal(err)
	}

	h := assetFiles(func() string { return assets })
	get := func(method, path string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		return rec
	}

	if rec := get("GET", assetsPrefix+"plan.png"); rec.Code != 200 || rec.Body.String() != "PNG" {
		t.Fatalf("an attachment: %d %q", rec.Code, rec.Body.String())
	}
	if rec := get("GET", assetsPrefix+"my%20notes.pdf"); rec.Code != 200 || rec.Body.String() != "PDF" {
		t.Fatalf("a name with a space, percent-encoded as notes link it: %d", rec.Code)
	}
	if csp := get("GET", assetsPrefix+"plan.png").Header().Get("Content-Security-Policy"); csp == "" {
		t.Fatal("served without a policy: an attached SVG could run")
	}

	for name, path := range map[string]string{
		"a note beside assets/":       assetsPrefix + "../pages/Secret.md",
		"an encoded dot-dot":          assetsPrefix + "..%2Fpages%2FSecret.md",
		"a symlink out of it":         assetsPrefix + "escape.txt",
		"the directory itself":        assetsPrefix,
		"something that is not there": assetsPrefix + "gone.png",
		"a path outside the prefix":   "/pages/Secret.md",
		"an absolute file path":       "/file" + assets + "/plan.png",
	} {
		if rec := get("GET", path); rec.Code == 200 {
			t.Errorf("%s was served (%s)", name, path)
		}
	}
	if rec := get("POST", assetsPrefix+"plan.png"); rec.Code == 200 {
		t.Error("a POST was answered")
	}
}
