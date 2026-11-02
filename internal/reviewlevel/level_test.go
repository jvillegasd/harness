package reviewlevel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLevelDefaultsToMedium(t *testing.T) {
	level, source, err := LoadLevel(filepath.Join(t.TempDir(), "state.json"))
	if err != nil || level != "medium" || source != "default" {
		t.Fatalf("got %q %q %v", level, source, err)
	}
}

func TestSaveLevelKeepsOtherKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"models": {"preset": "economy"}, "version": 1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveLevel(path, "high"); err != nil {
		t.Fatal(err)
	}
	if level, source, err := LoadLevel(path); err != nil || level != "high" || source != "global" {
		t.Fatalf("got %q %q %v", level, source, err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"preset": "economy"`) || !strings.Contains(string(data), `"version": 1`) {
		t.Errorf("SaveLevel dropped another key:\n%s", data)
	}
}

func TestLevelRejectsWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	seed := `{"review": {"level": "high"}}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveLevel(path, "max"); err == nil {
		t.Error("SaveLevel accepted max")
	}
	if data, _ := os.ReadFile(path); string(data) != seed {
		t.Errorf("a rejected level rewrote the file:\n%s", data)
	}
	for _, stored := range []string{`{"review": {"level": "max"}}`, `{"review": "high"}`} {
		if err := os.WriteFile(path, []byte(stored), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := LoadLevel(path); err == nil || !strings.Contains(err.Error(), "harness review level medium") {
			t.Errorf("%s: want an error naming the repair, got %v", stored, err)
		}
	}
}

func TestSaveLevelOverANullState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("null"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveLevel(path, "off"); err != nil {
		t.Fatal(err)
	}
	if level, _, err := LoadLevel(path); err != nil || level != "off" {
		t.Fatalf("got %q %v", level, err)
	}
}
