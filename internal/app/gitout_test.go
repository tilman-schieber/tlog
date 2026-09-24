package app

import (
	"bytes"
	"os/exec"
)

// gitOut runs git in a directory, for tests that assert on repository state.
func gitOut(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	return out.String(), err
}
