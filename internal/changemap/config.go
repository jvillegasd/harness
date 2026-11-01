package changemap

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ConfigPath is the repository-relative file shared with the tools that own the tracker and label fields. This package reads only criticalPaths from it and ignores every other key.
const ConfigPath = "docs/agents/harness.json"

// Config is what a repository adds to the built-in rules.
type Config struct {
	// CriticalPaths are repo-relative, slash-separated globs for files that always need a human.
	CriticalPaths []string
}

// LoadConfig reads criticalPaths from the repository root's ConfigPath. A missing file or field means no configured paths; a field of the wrong shape, or an entry that is not a valid glob, is an error that names the field and the entry, because silently dropping a critical path would lower the floor.
func LoadConfig(root string) (Config, error) {
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ConfigPath)))
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	return parseConfig(raw, ConfigPath)
}

// EffectiveConfig is the floor a change is classified against: the union of the criticalPaths in the working tree, at the merge base, and at the merge base with the default branch. Reading only the working tree would let a branch lower its own floor by deleting an entry, and reading only the chosen base would let a caller pick a commit that already dropped it, so every one of those entries counts.
func EffectiveConfig(root, mergeBase string) (Config, error) {
	current, err := LoadConfig(root)
	if err != nil {
		return Config{}, err
	}
	revs := []string{mergeBase}
	if def, err := git(root, "merge-base", "HEAD", DefaultBase(root)); err == nil {
		revs = append(revs, strings.TrimSpace(def))
	}
	cfg := Config{}
	seen := map[string]bool{}
	add := func(globs []string) {
		for _, g := range globs {
			if !seen[g] {
				seen[g] = true
				cfg.CriticalPaths = append(cfg.CriticalPaths, g)
			}
		}
	}
	for _, rev := range revs {
		if rev == "" || seen["rev:"+rev] {
			continue
		}
		seen["rev:"+rev] = true
		base, err := configAt(root, rev)
		if err != nil {
			return Config{}, err
		}
		add(base.CriticalPaths)
	}
	add(current.CriticalPaths)
	return cfg, nil
}

// configAt reads criticalPaths from ConfigPath as committed at rev; a revision without the file adds nothing.
func configAt(root, rev string) (Config, error) {
	if _, err := git(root, "cat-file", "-e", rev+":"+ConfigPath); err != nil {
		return Config{}, nil
	}
	raw, err := git(root, "show", rev+":"+ConfigPath)
	if err != nil {
		return Config{}, err
	}
	cfg, err := parseConfig([]byte(raw), ConfigPath+" at "+shortRev(rev))
	if err != nil {
		return Config{}, fmt.Errorf("%v; fix it on the default branch, because the committed base config still sets the floor", err)
	}
	return cfg, nil
}

func shortRev(rev string) string {
	if len(rev) > 12 {
		return rev[:12]
	}
	return rev
}

// parseConfig reads criticalPaths from raw, naming source in every error.
func parseConfig(raw []byte, source string) (Config, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Config{}, fmt.Errorf("%s: %v", source, err)
	}
	field, ok := doc["criticalPaths"]
	if !ok {
		return Config{}, nil
	}
	if bytes.Equal(bytes.TrimSpace(field), []byte("null")) {
		return Config{}, fmt.Errorf("%s: criticalPaths must be an array of glob strings, got null", source)
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(field, &entries); err != nil {
		return Config{}, fmt.Errorf("%s: criticalPaths must be an array of glob strings: %v", source, err)
	}
	var cfg Config
	for i, entry := range entries {
		var glob string
		if err := json.Unmarshal(entry, &glob); err != nil {
			return Config{}, fmt.Errorf("%s: criticalPaths[%d] %s is not a string", source, i, entry)
		}
		if err := ValidateGlob(glob); err != nil {
			return Config{}, fmt.Errorf("%s: criticalPaths[%d] %q: %v", source, i, glob, err)
		}
		cfg.CriticalPaths = append(cfg.CriticalPaths, glob)
	}
	return cfg, nil
}
