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
func TestReadGuardAllowsReadsInsideTheWorkingDirectory(t *testing.T) {
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
		{`git log --oneline -5 --no-ext-diff --no-textconv`, true},
		{`git diff --no-ext-diff --no-textconv 'HEAD~1' -- a.go`, true},
		{`git diff --no-ext-diff --no-textconv HEAD~1 -- a.go`, false},
		{`git diff 'HEAD~1' -- a.go`, false},
		{`git diff --no-ext-diff 'HEAD~1'`, false},
		{`git log -p --no-ext-diff --no-textconv`, true},
		{`git blame --no-textconv a.go`, true},
		{`git blame a.go`, false},
		{`rg -f` + filepath.Join(base, "outside.txt") + ` .`, false},
		{`rg -nf../outside.txt .`, false},
		{`rg -f.env .`, false},
		{`rg -flink x`, false},
		{`rg -nflink x`, false},
		{`rg -n5 x .`, true},
		{`git show --no-ext-diff --no-textconv HEAD:a.go`, true},
		{`git grep -n foo`, true},
		{dir + `/a.go`, false},
		{`cat ` + dir + `/a.go`, true},
		{`cat ` + filepath.Join(base, "outside.txt"), false},
		{`cat ../outside.txt`, false},
		{`cat link`, false},
		{`cat .env`, false},
		{`cat .env.local`, true},
		{`cat .env.example`, true},
		{`cat .env.development.local`, true},
		{`cat .env.test`, false},
		{`cat .ENV`, false},
		{`cat .env.development`, false},
		{`cat .env.production`, false},
		{`cat .env.local.bak`, false},
		{`git show --no-ext-diff --no-textconv HEAD:.env`, false},
		{`rg --hidden TOKEN`, false},
		{`rg -uu TOKEN`, false},
		{`rg --no-ignore-vcs TOKEN`, false},
		{`rg --pre=sh x`, false},
		{`rg --file=../outside.txt .`, false},
		{`find . -delete`, false},
		{`find -L . -name x`, false},
		{`tail -f a.go`, false},
		{`tail -5f a.go`, false},
		{`git diff --no-ext-diff --no-textconv --no-index a.go ../outside.txt`, false},
		{`git diff --no-ext-diff --no-textconv --output=out.patch`, false},
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
		if code := ReadGuard(bytes.NewReader(input), &output); code != 0 {
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

// A hook request without a cwd falls back to the guard's own working directory.
func TestReadCommandFallsBackToTheProcessDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	input, _ := json.Marshal(map[string]any{"tool_name": "Bash", "tool_input": map[string]string{"command": "cat a.go"}})
	var output bytes.Buffer
	ReadGuard(bytes.NewReader(input), &output)
	if !strings.Contains(output.String(), `"permissionDecision":"allow"`) {
		t.Fatal(output.String())
	}
}

// Anything that is not a well-formed Bash request is denied.
func TestReadGuardDeniesMalformedInput(t *testing.T) {
	for _, input := range []string{"malformed", `{"tool_name":"Write","tool_input":{"command":"cat a.go"}}`} {
		var output bytes.Buffer
		if code := ReadGuard(strings.NewReader(input), &output); code != 0 || !strings.Contains(output.String(), `"permissionDecision":"deny"`) {
			t.Fatalf("%s => %d %s", input, code, output.String())
		}
	}
}

// Every Claude tool is refused a secret env file by name, in any directory, while the example template and explicitly local files stay open; a call naming none gets no decision, so the client's own rules still apply.
func TestSecretGuardDeniesSecretEnvFilesOnEveryTool(t *testing.T) {
	for _, tc := range []struct {
		tool  string
		input map[string]any
		deny  bool
	}{
		{"Read", map[string]any{"file_path": "/elsewhere/repo/.env"}, true},
		{"Read", map[string]any{"file_path": "/repo/.env.production"}, true},
		{"Read", map[string]any{"file_path": "/repo/.env.test"}, true},
		{"Read", map[string]any{"file_path": "/repo/.env.local.bak"}, true},
		{"Read", map[string]any{"file_path": "/repo/.env.example"}, false},
		{"Read", map[string]any{"file_path": "/repo/.env.local"}, false},
		{"Read", map[string]any{"file_path": "/repo/.env.development.local"}, false},
		{"Read", map[string]any{"file_path": "/repo/app/env.py"}, false},
		{"Read", map[string]any{"file_path": "/repo/.ENV"}, true},
		{"Read", map[string]any{"file_path": "/repo/.Env.Production"}, true},
		{"Read", map[string]any{"file_path": "/repo/.ENV.EXAMPLE"}, false},
		{"Edit", map[string]any{"file_path": "/repo/.env", "old_string": "a", "new_string": "b"}, true},
		{"Edit", map[string]any{"file_path": "/repo/.env.local", "old_string": "a", "new_string": "b"}, false},
		{"Write", map[string]any{"file_path": "/repo/.env.staging", "content": "x"}, true},
		{"Write", map[string]any{"file_path": "/repo/notes.md", "content": "mentions .env freely"}, false},
		{"Write", map[string]any{"file_path": "/repo/notes.md", "content": strings.Repeat("x", 1<<20+1)}, false},
		{"Grep", map[string]any{"pattern": "TOKEN", "path": "/repo/.env"}, true},
		{"Grep", map[string]any{"pattern": "TOKEN", "glob": ".env*"}, true},
		{"Grep", map[string]any{"pattern": `\.env`, "path": "/repo"}, false},
		{"Glob", map[string]any{"pattern": "**/.env*"}, true},
		{"Glob", map[string]any{"pattern": "**/.env.example"}, false},
		{"Bash", map[string]any{"command": "cat .env"}, true},
		{"Bash", map[string]any{"command": "sed -n 1p ../other/.env.production"}, true},
		{"Bash", map[string]any{"command": "git show HEAD:.env"}, true},
		{"Bash", map[string]any{"command": "git diff -- .env.prod"}, true},
		{"Bash", map[string]any{"command": "grep TOKEN .env*"}, true},
		{"Bash", map[string]any{"command": "printf x > .env"}, true},
		{"Bash", map[string]any{"command": `python3 -c "print(open('.env').read())"`}, true},
		{"Bash", map[string]any{"command": "cat .Env.Production"}, true},
		{"Bash", map[string]any{"command": "cat .en''v"}, true},
		{"Bash", map[string]any{"command": `cat ".e"nv`}, true},
		{"Bash", map[string]any{"command": `cat .e\nv`}, true},
		{"Bash", map[string]any{"command": "cat .env.example .env.local"}, false},
		{"Bash", map[string]any{"command": "git diff .env.example"}, false},
		{"Bash", map[string]any{"command": "uv run pytest -q"}, false},
		{"mcp__other__tool", map[string]any{"path": "/repo/.env"}, true},
		{"mcp__other__tool", map[string]any{"query": ".env"}, false},
	} {
		input, _ := json.Marshal(map[string]any{"tool_name": tc.tool, "tool_input": tc.input})
		var output bytes.Buffer
		if code := SecretGuard(bytes.NewReader(input), &output); code != 0 {
			t.Fatal(code)
		}
		if denied := strings.Contains(output.String(), `"permissionDecision":"deny"`); denied != tc.deny || (!tc.deny && output.Len() != 0) {
			t.Errorf("%s %v => %q, want deny %v", tc.tool, tc.input, output.String(), tc.deny)
		}
	}
}

// A request the guard cannot read is denied rather than waved through.
func TestSecretGuardDeniesMalformedInput(t *testing.T) {
	for _, input := range []string{"malformed", `{"tool_input":{"file_path":"a.go"}}`} {
		var output bytes.Buffer
		if code := SecretGuard(strings.NewReader(input), &output); code != 0 || !strings.Contains(output.String(), `"permissionDecision":"deny"`) {
			t.Fatalf("%s => %d %s", input, code, output.String())
		}
	}
}
