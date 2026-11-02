// Package statefile reads and writes ~/.harness/state.json, which the installer, `harness models`, and `harness review level` share. Each owner reads the whole document and writes back only its own top-level key, so no command drops another's.
package statefile

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Path is ~/.harness/state.json under home.
func Path(home string) string { return filepath.Join(home, ".harness", "state.json") }

// Read returns the document's top-level keys, empty when the file does not exist.
func Read(path string) (map[string]json.RawMessage, error) {
	doc := map[string]json.RawMessage{}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return doc, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return doc, nil
}

// Write stores doc as indented JSON, creating the directory when needed.
func Write(path string, doc map[string]json.RawMessage) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return Replace(path, append(data, '\n'))
}

// Replace writes data to a fresh temporary file beside path and renames it over path, so a reader never sees half a file.
func Replace(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), "state-*.json")
	if err != nil {
		return err
	}
	_, werr := f.Write(data)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(f.Name(), 0o644)
	}
	if werr != nil {
		_ = os.Remove(f.Name())
		return werr
	}
	return os.Rename(f.Name(), path)
}
