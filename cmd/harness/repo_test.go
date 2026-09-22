package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// repoRoot is the checkout this test binary was built from.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// readRepo reads a repository file by its checkout-relative path.
func readRepo(t *testing.T, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestDocumentedRepositoryPathsResolve(t *testing.T) {
	root := repoRoot(t)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	top := map[string]bool{}
	for _, entry := range entries {
		top[entry.Name()] = true
	}
	tokens := regexp.MustCompile(`[A-Za-z0-9._][A-Za-z0-9._/-]*`).FindAllString(readRepo(t, "AGENTS.md"), -1)
	paths := map[string]bool{}
	for _, token := range tokens {
		token = strings.TrimPrefix(token, "./")
		if !top[strings.SplitN(token, "/", 2)[0]] {
			continue
		}
		paths[token] = true
	}
	if len(paths) <= 5 {
		t.Fatalf("AGENTS.md path parsing found only %d paths", len(paths))
	}
	for path := range paths {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Errorf("AGENTS.md documents %s, which does not exist", path)
		}
	}
}
