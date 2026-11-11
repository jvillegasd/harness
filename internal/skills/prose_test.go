package skills

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jvillegasd/harness/internal/checkout"
)

// TestShippedProseHasNoEmDash holds the floor's prose rule where a file check can enforce it exactly: no Markdown the harness authors (the floor, the role definitions, and every local skill) carries an em-dash. Locked skill bodies are upstream content and are exempt.
func TestShippedProseHasNoEmDash(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	c := checkout.Checkout{Root: root}
	lf, err := readLock(c.Lock())
	if err != nil {
		t.Fatal(err)
	}
	dirs := []string{filepath.Join(root, "roles")}
	skills, err := c.SkillDirs()
	if err != nil {
		t.Fatal(err)
	}
	for _, sk := range skills {
		if !lf.skills.has(sk.Name) {
			dirs = append(dirs, sk.Dir)
		}
	}
	check := func(path string) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if strings.ContainsRune(line, '—') {
				rel, _ := filepath.Rel(root, path)
				t.Errorf("%s:%d carries an em-dash", rel, i+1)
			}
		}
	}
	check(c.Floor())
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(path, ".md") {
				check(path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
