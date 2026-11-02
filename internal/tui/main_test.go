package tui

import (
	"os"
	"path/filepath"
	"testing"
)

// Every navigation in these tests would otherwise be remembered in the real
// ~/.local/state/tlog/recent.json.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "tlog-state-")
	if err != nil {
		panic(err)
	}
	os.Setenv("TLOG_STATE", filepath.Join(dir, "recent.json"))
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
