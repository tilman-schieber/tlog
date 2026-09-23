package cli

import (
	"bytes"
	"os/exec"
)

// gitOutput runs a git command in a directory, for tests that assert the
// working tree is clean.
func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	return out.String(), err
}
