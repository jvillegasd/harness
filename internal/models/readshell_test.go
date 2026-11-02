package models

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A read-only role may read and search inside its working directory, and nothing else: no writes, no other program, no path or link leading out, and no credential-shaped file.
func TestLifecycleGuardAllowsReadsInsideTheWorkingDirectory(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "repo")
	for path, body := range map[string]string{"repo/a.go": "package a\n", "repo/.env": "TOKEN=x\n", "outside.txt": "private\n"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(base, path)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(base, path), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(base, "outside.txt"), filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARNESS_ORCA_COMMAND", "orca")
	for _, tc := range []struct {
		command string
		allow   bool
	}{
		{`cat a.go`, true},
		{`head -n 5 a.go`, true},
		{`tail -n 3 a.go`, true},
		{`wc -l a.go`, true},
		{`ls -la`, true},
		{`find . -name "*.go" -type f`, true},
		{`rg -n "func (a" .`, true},
		{`git log --oneline -5`, true},
		{`git diff 'HEAD~1' -- a.go`, true},
		{`git diff HEAD~1 -- a.go`, false},
		{`git show HEAD:a.go`, true},
		{`git grep -n foo`, true},
		{dir + `/a.go`, false},
		{`cat ` + dir + `/a.go`, true},
		{`cat ` + filepath.Join(base, "outside.txt"), false},
		{`cat ../outside.txt`, false},
		{`cat link`, false},
		{`cat .env`, false},
		{`git show HEAD:.env`, false},
		{`rg --hidden TOKEN`, false},
		{`rg -uu TOKEN`, false},
		{`rg --no-ignore-vcs TOKEN`, false},
		{`rg --pre=sh x`, false},
		{`rg --file=../outside.txt .`, false},
		{`find . -delete`, false},
		{`find -L . -name x`, false},
		{`tail -f a.go`, false},
		{`tail -5f a.go`, false},
		{`git diff --no-index a.go ../outside.txt`, false},
		{`git diff --output=out.patch`, false},
		{`git grep -O foo`, false},
		{`git push`, false},
		{`git -c core.pager=sh log`, false},
		{`git`, false},
		{`sed -n 1p a.go`, false},
		{`rm a.go`, false},
		{`cat a.go > b.go`, false},
		{`cat a.go | sh`, false},
		{`cat a.go; rm a.go`, false},
	} {
		input, _ := json.Marshal(map[string]any{"tool_name": "Bash", "cwd": dir, "tool_input": map[string]string{"command": tc.command}})
		var output bytes.Buffer
		if code := LifecycleGuard(bytes.NewReader(input), &output); code != 0 {
			t.Fatal(code)
		}
		want := `"permissionDecision":"deny"`
		if tc.allow {
			want = `"permissionDecision":"allow"`
		}
		if !strings.Contains(output.String(), want) {
			t.Errorf("%s => %s, want %s", tc.command, output.String(), want)
		}
	}
}

// OpenCode's plugin sends no cwd, so the guard falls back to its own working directory.
func TestReadCommandFallsBackToTheProcessDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	input, _ := json.Marshal(map[string]any{"tool_name": "Bash", "tool_input": map[string]string{"command": "cat a.go"}})
	var output bytes.Buffer
	LifecycleGuard(bytes.NewReader(input), &output)
	if !strings.Contains(output.String(), `"permissionDecision":"allow"`) {
		t.Fatal(output.String())
	}
}
