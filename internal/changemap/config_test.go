package changemap

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jvillegasd/harness/internal/skills"
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

// TestLoadLocksReadsSkillFolders maps each lock entry to its skill folder and refuses an entry whose path could match outside skills.
func TestLoadLocksReadsSkillFolders(t *testing.T) {
	root := t.TempDir()
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, SkillLockPath), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if locks, err := LoadLocks(root); err != nil || locks != nil {
		t.Fatalf("missing lock = %v, %v; want none", locks, err)
	}
	body := filepath.Join(root, "skills", "a")
	if err := os.MkdirAll(body, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(body, "SKILL.md"), []byte("upstream\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hash, _, err := skills.FolderHash(body)
	if err != nil {
		t.Fatal(err)
	}
	write(`{"version": 1, "skills": {"b": {"source": "o/b", "skillPath": "skills/b/SKILL.md", "computedHash": "` + hash + `"}, "a": {"source": "o/a", "skillPath": "skills/a/SKILL.md", "computedHash": "` + hash + `"}, "c": {"source": "o/c", "skillPath": "internal/c/SKILL.md", "computedHash": "` + hash + `"}}}`)
	locks, err := LoadLocks(root)
	if err != nil || len(locks) != 2 || locks[0].Dir != "skills/a" || !locks[0].Verified || !locks[0].Hashed["SKILL.md"] || locks[1].Dir != "skills/b" || locks[1].Verified {
		t.Fatalf("LoadLocks = %+v, %v; want a verified, b unverified (no body), c outside skills/ ignored", locks, err)
	}
	if err := os.MkdirAll(filepath.Join(body, "__pycache__"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(body, "__pycache__", "x.pyc"), []byte("added\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if locks, _ := LoadLocks(root); !locks[0].Verified || locks[0].Hashed["__pycache__/x.pyc"] {
		t.Fatalf("a file the hash skips changed verification or was listed as covered: %+v", locks[0])
	}
	if err := os.WriteFile(filepath.Join(body, "SKILL.md"), []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if locks, _ := LoadLocks(root); locks[0].Verified {
		t.Fatalf("an edited body still verified: %+v", locks)
	}
	for _, bad := range []string{"", "SKILL.md", "../x/SKILL.md"} {
		write(`{"skills": {"x": {"source": "o/x", "skillPath": "` + bad + `"}}}`)
		if _, err := LoadLocks(root); err == nil || !strings.Contains(err.Error(), `"x"`) {
			t.Errorf("skillPath %q: err = %v, want a refusal naming the skill", bad, err)
		}
	}
}
