package skills

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestFolderHashMatchesSkillsSh pins the hash to the value skills.sh 1.8.1's computeSkillFolderHash gave for this exact tree, so verify and change-map agree with the computedHash skills.sh writes into a version 2 lock.
func TestFolderHashMatchesSkillsSh(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "skill")
	for rel, data := range map[string]string{
		"SKILL.md":                "---\nname: x\n---\nbody\n",
		"a.md":                    "lower\n",
		"B.md":                    "upper\n",
		"ref/crlf.md":             "one\r\ntwo\r\n",
		"ref/bin.dat":             "\x00\r\n",
		"metadata.json":           "{}\n",
		".git/config":             "x\n",
		"__pycache__/m.pyc":       "x\n",
		"ref/__pypackages__/p.py": "x\n",
		"node_modules/x.js":       "added\n",
	} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(base, "outside.txt"), []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for link, target := range map[string]string{"link.md": "a.md", "out.md": "../outside.txt", "loop": ".", "refdir": "ref"} {
		if err := os.Symlink(target, filepath.Join(root, link)); err != nil {
			t.Fatal(err)
		}
	}

	hash, files, err := FolderHash(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := "799a23f9a43b275bdd10401cc86fd0cd04dfcb86e2730482f770e19eb325faf0"; hash != want {
		t.Errorf("FolderHash = %s, want skills.sh's %s", hash, want)
	}
	want := []string{"B.md", "SKILL.md", "a.md", "link.md", "node_modules/x.js", "ref/bin.dat", "ref/crlf.md", "refdir/bin.dat", "refdir/crlf.md"}
	if !slices.Equal(files, want) {
		t.Errorf("covered files = %q, want %q", files, want)
	}

	t.Chdir(base)
	if rel, _, err := FolderHash("skill"); err != nil || rel != hash {
		t.Errorf("FolderHash(relative) = %s, %v; want %s", rel, err, hash)
	}
}
