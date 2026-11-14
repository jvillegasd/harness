package reviewledger

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Snapshot saves the tracked and untracked changes under owned as one binary patch against base.
// It builds an isolated index, so the repository's real index is never read or written.
func Snapshot(root, base, branchKey string, owned []string) (path, digest string, err error) {
	if len(owned) == 0 {
		return "", "", fmt.Errorf("at least one owned path is required")
	}
	resolvedBase, err := git(root, "rev-parse", "--verify", "--end-of-options", base+"^{commit}")
	if err != nil {
		return "", "", fmt.Errorf("base %q does not name a commit", base)
	}

	indexFile, err := os.CreateTemp("", "harness-review-index-*")
	if err != nil {
		return "", "", err
	}
	indexPath := indexFile.Name()
	if err := indexFile.Close(); err != nil {
		os.Remove(indexPath)
		return "", "", err
	}
	defer os.Remove(indexPath)

	run := func(args ...string) ([]byte, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_INDEX_FILE="+indexPath)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
		return out, nil
	}
	if _, err := run("read-tree", resolvedBase); err != nil {
		return "", "", err
	}
	addArgs := append([]string{"add", "-A", "--"}, owned...)
	if _, err := run(addArgs...); err != nil {
		return "", "", err
	}
	diffArgs := append([]string{"diff", "--cached", "--binary", "--full-index", "--no-renames", resolvedBase, "--"}, owned...)
	patch, err := run(diffArgs...)
	if err != nil {
		return "", "", err
	}
	if len(patch) == 0 {
		return "", "", errors.New("no changes under the owned paths")
	}
	hash := sha256.Sum256(patch)
	digest = hex.EncodeToString(hash[:])
	ledgerFile, err := File(root, branchKey)
	if err != nil {
		return "", "", err
	}
	dir := filepath.Dir(ledgerFile)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	path = filepath.Join(dir, "snapshot-"+digest[:12]+".patch")
	if err := os.WriteFile(path, patch, 0o644); err != nil {
		return "", "", err
	}
	return path, digest, nil
}
