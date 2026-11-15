package review

import (
	"bytes"
	"fmt"
	"os/exec"
	"slices"
	"strings"
)

func ChangedSince(root, baseRef, reviewed, head string, touched []string) ([]string, error) {
	before, err := Names(root, baseRef+"..."+reviewed)
	if err != nil {
		return nil, err
	}
	differing, err := Names(root, reviewed, head)
	if err != nil {
		return nil, err
	}
	var changed []string
	for _, f := range differing {
		if !slices.Contains(touched, f) && !slices.Contains(before, f) {
			continue
		}
		was, err := PatchID(root, baseRef+"..."+reviewed, f)
		if err != nil {
			return nil, err
		}
		now, err := PatchID(root, baseRef+"..."+head, f)
		if err != nil {
			return nil, err
		}
		if was != now {
			changed = append(changed, f)
		}
	}
	return changed, nil
}

func Names(root string, revs ...string) ([]string, error) {
	out, err := git(root, append([]string{"diff", "--name-only", "--no-renames", "-z"}, revs...)...)
	if err != nil {
		return nil, err
	}
	return strings.FieldsFunc(out, func(r rune) bool { return r == 0 }), nil
}

func PatchID(root, revs, file string) (string, error) {
	diff := exec.Command("git", "diff", "-U0", "--no-renames", "--full-index", revs, "--", file)
	diff.Dir = root
	patch, err := diff.Output()
	if err != nil {
		return "", fmt.Errorf("git diff %s -- %s: %w", revs, file, err)
	}
	id := exec.Command("git", "patch-id", "--verbatim")
	id.Dir = root
	id.Stdin = bytes.NewReader(patch)
	out, err := id.Output()
	if err != nil {
		return "", fmt.Errorf("git patch-id: %w", err)
	}
	if len(bytes.TrimSpace(out)) == 0 {
		return string(patch), nil
	}
	return string(out), nil
}
