package scenarios

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// evaluateArtifacts applies a scenario's declared artifact checks to the repository it ran in. Effects is nil unless the scenario seeded a publication fixture; then forge calls are judged by its declared forge rules, and refused outright when it declares none.
func evaluateArtifacts(scenario map[string]any, before, after map[string]string, repo string, forgeCalls []string, fixturesUnchanged bool, effects *sideEffects) []map[string]any {
	artifacts := dict(scenario, "artifacts")
	var entries []map[string]any
	if value, ok := artifacts["tree_unchanged"]; ok {
		expected := truthy(value)
		entries = append(entries, entry("tree_unchanged", equalTrees(before, after) == expected, ""))
	}
	if value, ok := artifacts["fixtures_unchanged"]; ok {
		expected := truthy(value)
		entries = append(entries, entry("fixtures_unchanged", fixturesUnchanged == expected, ""))
	}
	for _, relative := range asList(artifacts["present"]) {
		path := filepath.Join(repo, relative.(string))
		entries = append(entries, entry("present:"+relative.(string), pathExists(path), ""))
	}
	for _, relative := range asList(artifacts["absent"]) {
		path := filepath.Join(repo, relative.(string))
		entries = append(entries, entry("absent:"+relative.(string), !pathExists(path), ""))
	}
	base, _ := filepath.Abs(repo)
	base = resolveExisting(base)
	for _, item := range asList(artifacts["contains"]) {
		declaration, _ := item.(map[string]any)
		relative := getString(declaration, "path")
		text := getString(declaration, "text")
		target := resolveExisting(filepath.Join(repo, filepath.FromSlash(relative)))
		inside := isWithin(base, target)
		found := false
		if inside {
			if raw, err := os.ReadFile(target); err == nil {
				found = strings.Contains(string(raw), text)
			}
		}
		entries = append(entries, entry(fmt.Sprintf("contains:%s:%s", relative, text), found, ""))
	}
	if effects != nil {
		entries = append(entries, sideEffectEntries(artifacts, effects, forgeCalls)...)
		_, allowed := artifacts["forge_allowed"]
		_, forbidden := artifacts["forge_forbidden"]
		if allowed || forbidden {
			return entries
		}
	}
	return append(entries, entry("no-external-forge", len(forgeCalls) == 0, strings.Join(forgeCalls, "; ")))
}

func equalTrees(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// resolveExisting follows symlinks where it can, matching pathlib's non-strict resolve.
func resolveExisting(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return abs
}

func isWithin(root, path string) bool {
	root = strings.TrimSuffix(root, string(filepath.Separator))
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}
