package changemap

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeConfig(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, "docs", "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "harness.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadConfigMissingFileMeansNoCriticalPaths(t *testing.T) {
	cfg, err := LoadConfig(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.CriticalPaths) != 0 {
		t.Fatalf("CriticalPaths = %v, want none", cfg.CriticalPaths)
	}
}

func TestLoadConfigMissingFieldMeansNoCriticalPaths(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, `{"version": 1, "tracker": "github", "labels": {"blocked": "blocked"}}`)
	cfg, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.CriticalPaths) != 0 {
		t.Fatalf("CriticalPaths = %v, want none", cfg.CriticalPaths)
	}
}

func TestLoadConfigReadsCriticalPathsAndIgnoresOtherKeys(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, `{"version": 1, "tracker": "github", "criticalPaths": ["POLICY.md", "clients/**"], "extra": [1, 2]}`)
	cfg, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"POLICY.md", "clients/**"}; !reflect.DeepEqual(cfg.CriticalPaths, want) {
		t.Fatalf("CriticalPaths = %v, want %v", cfg.CriticalPaths, want)
	}
}

func TestLoadConfigRejectsBadCriticalPaths(t *testing.T) {
	cases := []struct {
		name, body string
		// mention are fragments the error must carry: the field and the bad entry.
		mention []string
	}{
		{"not an array", `{"criticalPaths": "clients/**"}`, []string{"criticalPaths"}},
		{"object", `{"criticalPaths": {"a": 1}}`, []string{"criticalPaths"}},
		{"null", `{"criticalPaths": null}`, []string{"criticalPaths"}},
		{"non-string entry", `{"criticalPaths": ["ok", 7]}`, []string{"criticalPaths", "7"}},
		{"empty entry", `{"criticalPaths": ["ok", ""]}`, []string{"criticalPaths", `""`}},
		{"absolute", `{"criticalPaths": ["/etc/passwd"]}`, []string{"criticalPaths", "/etc/passwd"}},
		{"parent segment", `{"criticalPaths": ["a/../b"]}`, []string{"criticalPaths", "a/../b"}},
		{"bad pattern", `{"criticalPaths": ["a/[b"]}`, []string{"criticalPaths", "a/[b"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			writeConfig(t, root, c.body)
			_, err := LoadConfig(root)
			if err == nil {
				t.Fatal("LoadConfig accepted the file")
			}
			for _, fragment := range c.mention {
				if !contains(err.Error(), fragment) {
					t.Errorf("error %q does not mention %q", err, fragment)
				}
			}
		})
	}
}

func TestLoadConfigRejectsMalformedJSON(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, `{"criticalPaths": [`)
	if _, err := LoadConfig(root); err == nil {
		t.Fatal("LoadConfig accepted malformed JSON")
	}
}
