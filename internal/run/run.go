// Package run holds the git and gh process helpers shared across the CLI.
package run

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Forge runs a forge CLI command and returns its stdout.
type Forge func(args ...string) ([]byte, error)

// Git runs git in dir and returns trimmed stdout.
func Git(dir string, args ...string) (string, error) {
	out, err := GitRaw(dir, args...)
	return strings.TrimSpace(string(out)), err
}

// GitRaw runs git in dir and returns untrimmed stdout.
func GitRaw(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// Gh returns a Forge that runs gh in dir.
func Gh(dir string) Forge {
	return func(args ...string) ([]byte, error) {
		cmd := exec.Command("gh", args...)
		cmd.Dir = dir
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			return stdout.Bytes(), fmt.Errorf("gh %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
		}
		return stdout.Bytes(), nil
	}
}
