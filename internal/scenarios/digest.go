package scenarios

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func canonical(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func fileDigest(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "missing"
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func treeDigest(root string) string {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return "missing"
	}
	var files []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		files = append(files, path)
		return nil
	})
	sort.Strings(files)
	var parts []string
	for _, path := range files {
		rel, _ := filepath.Rel(root, path)
		parts = append(parts, filepath.ToSlash(rel)+":"+fileDigest(path))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}

// scenarioDigest hashes the declaration, its harness inputs, and its recording, so a change marks an older attempt stale.
func scenarioDigest(scenario map[string]any, root, fixturesRoot string) string {
	parts := []string{canonical(scenario)}
	for _, raw := range list(scenario, "harness_inputs") {
		relative, _ := raw.(string)
		parts = append(parts, relative+":"+fileDigest(filepath.Join(root, filepath.FromSlash(relative))))
	}
	parts = append(parts, "fixture:"+treeDigest(filepath.Join(fixturesRoot, getString(scenario, "id"))))
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}

func snapshot(root string) map[string]string {
	tree := map[string]string{}
	if !pathExists(root) {
		return tree
	}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		parts := strings.Split(filepath.ToSlash(rel), "/")
		for _, part := range parts {
			if part == ".git" {
				return nil
			}
		}
		tree[filepath.ToSlash(rel)] = fileDigest(path)
		return nil
	})
	return tree
}
