package changemap

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Config{}, fmt.Errorf("%s: %v", ConfigPath, err)
	}
	field, ok := doc["criticalPaths"]
	if !ok {
		return Config{}, nil
	}
	if bytes.Equal(bytes.TrimSpace(field), []byte("null")) {
		return Config{}, fmt.Errorf("%s: criticalPaths must be an array of glob strings, got null", ConfigPath)
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(field, &entries); err != nil {
		return Config{}, fmt.Errorf("%s: criticalPaths must be an array of glob strings: %v", ConfigPath, err)
	}
	var cfg Config
	for i, entry := range entries {
		var glob string
		if err := json.Unmarshal(entry, &glob); err != nil {
			return Config{}, fmt.Errorf("%s: criticalPaths[%d] %s is not a string", ConfigPath, i, entry)
		}
		if err := ValidateGlob(glob); err != nil {
			return Config{}, fmt.Errorf("%s: criticalPaths[%d] %q: %v", ConfigPath, i, glob, err)
		}
		cfg.CriticalPaths = append(cfg.CriticalPaths, glob)
	}
	return cfg, nil
}
