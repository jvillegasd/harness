package agents

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var outputNames = []string{
	"claude/code-search.md", "claude/executor.md", "claude/reviewer.md",
	"codex/code-search.toml", "codex/executor.toml", "codex/reviewer.toml",
	"opencode/code-search.md", "opencode/executor.md", "opencode/reviewer.md",
}

// temp returns a resolved temporary directory, because macOS temp paths pass through a symlink and the renderer refuses a symlinked output ancestor.
func temp(t *testing.T) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func repoRoot() string {
	abs, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		panic(err)
	}
	return abs
}

func configPath() string { return filepath.Join(repoRoot(), "clients", "assignments.json") }

// runCommand calls Generate, or Check when args lead with --check, with the config and output that follow.
func runCommand(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	run := Generate
	if args[0] == "--check" {
		run, args = Check, args[1:]
	}
	code := run(args[0], args[1], nil, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func readConfig(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	return data
}

// configCopy copies the canonical assignments and each role definition into dir, pointing the copy at the copied definitions, then applies edit.
func configCopy(t *testing.T, dir string, edit func(map[string]any)) string {
	t.Helper()
	data := readConfig(t, configPath())
	roles := data["roles"].(map[string]any)
	if err := os.MkdirAll(filepath.Join(dir, "roles"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, raw := range roles {
		role := raw.(map[string]any)
		def, ok := role["definition"].(string)
		if !ok {
			continue
		}
		source := filepath.Join(filepath.Dir(configPath()), def)
		rawBytes, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		copyPath := filepath.Join(dir, "roles", name+".md")
		if err := os.WriteFile(copyPath, rawBytes, 0o644); err != nil {
			t.Fatal(err)
		}
		role["definition"] = copyPath
	}
	if edit != nil {
		edit(data)
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "assignments.json")
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func treeOf(t *testing.T, root string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		raw, _ := os.ReadFile(path)
		tree[filepath.ToSlash(rel)] = string(raw)
		return nil
	})
	return tree
}

func head(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(string(raw), "---\n", 3)
	if len(parts) < 2 {
		t.Fatalf("%s has no frontmatter", path)
	}
	return strings.Split(parts[1], "\n")
}

func TestOneDefinitionPerRolePerClient(t *testing.T) {
	out := temp(t)
	code, _, stderr := runCommand(t, configPath(), out)
	if code != 0 {
		t.Fatalf("generate exited %d: %s", code, stderr)
	}
	if len(treeOf(t, out)) != len(outputNames) {
		t.Fatalf("generated %v", treeOf(t, out))
	}
	for _, name := range outputNames {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Errorf("missing %s", name)
		}
	}
}

func TestGenerationIsIdempotent(t *testing.T) {
	out := temp(t)
	if code, stdout, stderr := runCommand(t, configPath(), out); code != 0 || !strings.Contains(stdout, "output: changed") {
		t.Fatalf("first generate exited %d with %q: %s", code, stdout, stderr)
	}
	first := treeOf(t, out)
	code, stdout, stderr := runCommand(t, configPath(), out)
	if code != 0 {
		t.Fatalf("second generate exited %d, want 0: %s", code, stderr)
	}
	if !strings.Contains(stdout, "output: unchanged") {
		t.Fatalf("second generate did not report its output as unchanged: %s", stdout)
	}
	if len(treeOf(t, out)) != len(first) {
		t.Fatal("idempotent generation changed the tree")
	}
}

func TestEachClientGetsItsControls(t *testing.T) {
	out := temp(t)
	if code, _, stderr := runCommand(t, configPath(), out); code != 0 {
		t.Fatalf("generate exited %d: %s", code, stderr)
	}
	for _, role := range []string{"reviewer"} {
		claude := readFile(t, filepath.Join(out, "claude", role+".md"))
		opencode := readFile(t, filepath.Join(out, "opencode", role+".md"))
		codex := readFile(t, filepath.Join(out, "codex", role+".toml"))
		if !strings.HasPrefix(codex, "name = \""+role+"\"\n") || !strings.HasSuffix(codex, "\n'''\n\n[features]\napps = false\nplugins = false\n") {
			t.Errorf("%s codex controls are wrong:\n%s", role, codex)
		}
		if !strings.Contains(claude, "\nname: "+role+"\n") || !strings.Contains(claude, "\ntools: Read, Grep, Glob, Bash\n") {
			t.Errorf("%s claude frontmatter is wrong:\n%s", role, claude)
		}
		if !strings.Contains(opencode, "\nmode: all\npermission:\n  \"*\": deny\n") || !strings.Contains(opencode, "\n    \"*.env\": deny\n") {
			t.Errorf("%s opencode controls are wrong:\n%s", role, opencode)
		}
	}
}

func TestExecutorKeepsImplementationAccess(t *testing.T) {
	out := temp(t)
	if code, _, stderr := runCommand(t, configPath(), out); code != 0 {
		t.Fatalf("generate exited %d: %s", code, stderr)
	}
	claude := readFile(t, filepath.Join(out, "claude", "executor.md"))
	opencode := readFile(t, filepath.Join(out, "opencode", "executor.md"))
	if !strings.Contains(claude, "\nname: executor\n") || strings.Contains(strings.Join(head(t, filepath.Join(out, "claude", "executor.md")), "\n"), "tools:") {
		t.Errorf("executor claude frontmatter is wrong:\n%s", claude)
	}
	if !strings.Contains(claude, "\ndisallowedTools: Agent\n") {
		t.Error("executor claude does not disable delegation")
	}
	if !strings.Contains(opencode, "\nmode: all\npermission:\n  task: deny\n  bash:\n") {
		t.Errorf("executor opencode controls are wrong:\n%s", opencode)
	}
	for _, pattern := range []string{"git commit*", "git push*", "git worktree remove*", "wt remove*", "wt merge*", "wt step commit*", "gh *"} {
		if !strings.Contains(opencode, "\n    \""+pattern+"\": deny\n") {
			t.Errorf("executor opencode does not deny %s", pattern)
		}
	}
	if strings.Contains(opencode, "\"*\": deny") {
		t.Error("executor opencode denies everything")
	}
	if codex := readFile(t, filepath.Join(out, "codex", "executor.toml")); !strings.HasSuffix(codex, "\n'''\n") {
		t.Errorf("executor codex carries controls:\n%s", codex)
	}
	if code, _, stderr := runCommand(t, "--check", configPath(), out); code != 0 {
		t.Fatalf("check exited %d: %s", code, stderr)
	}
}

func TestOnlyTheExecutorMayImplement(t *testing.T) {
	cases := []struct {
		name, target, access string
		want                 int
	}{
		{"reviewer", "reviewer", "implement", 1},
		{"reviewer", "reviewer", "read-search", 0},
		{"executor", "executor", "read-search", 1},
	}
	for _, c := range cases {
		t.Run(c.name+"-"+c.access, func(t *testing.T) {
			dir := temp(t)
			config := configCopy(t, dir, nil)
			rolePath := filepath.Join(dir, "roles", c.target+".md")
			raw, err := os.ReadFile(rolePath)
			if err != nil {
				t.Fatal(err)
			}
			text := string(raw)
			if c.target == "reviewer" {
				text = strings.Replace(text, "access: read-search", "access: "+c.access, 1)
			} else {
				text = strings.Replace(text, "access: implement", "access: "+c.access, 1)
			}
			if err := os.WriteFile(rolePath, []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
			code, _, stderr := runCommand(t, config, filepath.Join(dir, "out"))
			if code != c.want {
				t.Fatalf("exit %d, want %d: %s", code, c.want, stderr)
			}
			if c.want != 0 && !strings.Contains(stderr, c.target+".md: access must be") {
				t.Errorf("stderr = %q", stderr)
			}
		})
	}
}

func TestCheckDetectsWidening(t *testing.T) {
	mutations := []struct{ path, old, new string }{
		{"claude/reviewer.md", "permissionMode: dontAsk\n", "permissionMode: bypassPermissions\n"},
		{"claude/reviewer.md", "tools: Read, Grep, Glob, Bash\n", ""},
		{"opencode/reviewer.md", "  \"*\": deny", "  \"*\": allow"},
		{"opencode/reviewer.md", "  list: allow\n", "  list: allow\n  bash: allow\n"},
		{"opencode/reviewer.md", "mode: all\n", "mode: all\ntools:\n  bash: true\n"},
		{"claude/code-search.md", "tools: Read, Grep, Glob, Bash\n", "tools: Read, Grep, Glob, Edit\n"},
		{"opencode/code-search.md", "  grep: allow\n", "  grep: allow\n  task: allow\n"},
		{"claude/executor.md", "disallowedTools: Agent\n", ""},
		{"opencode/executor.md", "  task: deny\n", "  task: allow\n"},
		{"codex/reviewer.toml", "plugins = false\n", "plugins = true\n"},
		{"codex/reviewer.toml", "[features]\napps = false\n", "[features]\n"},
		{"codex/code-search.toml", "developer_instructions = '''\n", "sandbox_mode = \"danger-full-access\"\ndeveloper_instructions = '''\n"},
		{"codex/executor.toml", "\n'''\n", "\n'''\n\n[features]\nmulti_agent = true\n"},
	}
	for _, mutation := range mutations {
		t.Run(mutation.path+mutation.new, func(t *testing.T) {
			out := temp(t)
			if code, _, stderr := runCommand(t, configPath(), out); code != 0 {
				t.Fatalf("generate exited %d: %s", code, stderr)
			}
			target := filepath.Join(out, filepath.FromSlash(mutation.path))
			text := readFile(t, target)
			if !strings.Contains(text, mutation.old) {
				t.Fatalf("%s does not contain %q", mutation.path, mutation.old)
			}
			if err := os.WriteFile(target, []byte(strings.Replace(text, mutation.old, mutation.new, 1)), 0o644); err != nil {
				t.Fatal(err)
			}
			code, stdout, _ := runCommand(t, "--check", configPath(), out)
			if code != 1 || !strings.Contains(stdout, "widened: "+mutation.path) {
				t.Fatalf("check = %d, %q; want widened %s", code, stdout, mutation.path)
			}
		})
	}
}

func TestCheckDetectsMissingAndDrifted(t *testing.T) {
	out := temp(t)
	if code, _, _ := runCommand(t, configPath(), out); code != 0 {
		t.Fatal("generate failed")
	}
	target := filepath.Join(out, "claude", "reviewer.md")
	raw := readFile(t, target)
	if err := os.WriteFile(target, []byte(raw+"\nExtra instruction.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, _ := runCommand(t, "--check", configPath(), out)
	if code != 1 || !strings.Contains(stdout, "changed: claude/reviewer.md") {
		t.Fatalf("check = %d, %q", code, stdout)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	code, stdout, _ = runCommand(t, "--check", configPath(), out)
	if code != 1 || !strings.Contains(stdout, "missing: claude/reviewer.md") {
		t.Fatalf("check = %d, %q", code, stdout)
	}
}

func TestGenerationRemovesStaleOutputs(t *testing.T) {
	out := temp(t)
	if err := os.MkdirAll(filepath.Join(out, "gone"), 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(out, "gone", "retired.md")
	if err := os.WriteFile(stale, []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runCommand(t, configPath(), out); code != 0 {
		t.Fatalf("generate exited %d: %s", code, stderr)
	}
	if _, err := os.Stat(stale); err == nil {
		t.Fatal("the stale output survived")
	}
	if len(treeOf(t, out)) != len(outputNames) {
		t.Fatalf("tree = %v", treeOf(t, out))
	}
}

func TestSymlinkedOutputPathsAreRefusedWithoutWrites(t *testing.T) {
	for _, kind := range []string{"file", "client", "output", "ancestor", "dangling"} {
		t.Run(kind, func(t *testing.T) {
			base := temp(t)
			output := filepath.Join(base, "parent", "out")
			if code, _, stderr := runCommand(t, configPath(), output); code != 0 {
				t.Fatalf("generate exited %d: %s", code, stderr)
			}
			var link string
			switch kind {
			case "file":
				link = filepath.Join(output, "claude", "reviewer.md")
			case "client":
				link = filepath.Join(output, "claude")
			case "output":
				link = output
			case "ancestor":
				link = filepath.Dir(output)
			case "dangling":
				link = filepath.Join(output, "claude", "reviewer.md")
			}
			external := filepath.Join(base, "external")
			if err := os.Rename(link, external); err != nil {
				t.Fatal(err)
			}
			if info, err := os.Stat(external); err == nil && !info.IsDir() {
				if err := os.WriteFile(external, []byte("private agent\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			target := external
			if kind == "dangling" {
				target = filepath.Join(base, "missing")
			}
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			before := treeOf(t, base)
			for _, flags := range [][]string{{}, {"--check"}} {
				args := append(append([]string{}, flags...), configPath(), output)
				code, _, stderr := runCommand(t, args...)
				if code == 0 || !strings.Contains(stderr, "symlink") {
					t.Fatalf("flags %v: exit %d, stderr %q; want a symlink refusal", flags, code, stderr)
				}
				if !deepEqualStrings(treeOf(t, base), before) {
					t.Fatalf("flags %v changed the tree", flags)
				}
				if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
					t.Fatalf("flags %v moved the link", flags)
				}
			}
			if _, err := os.Stat(filepath.Join(base, "missing")); err == nil {
				t.Fatal("the refusal created the dangling target")
			}
		})
	}
}

func TestMissingDroppedAndMisnamedRolesPreserveOutputs(t *testing.T) {
	out := temp(t)
	if code, _, _ := runCommand(t, configPath(), out); code != 0 {
		t.Fatal("generate failed")
	}
	before := treeOf(t, out)
	for _, kind := range []string{"missing", "dropped", "misnamed"} {
		t.Run(kind, func(t *testing.T) {
			dir := temp(t)
			var edit func(map[string]any)
			if kind == "dropped" {
				edit = func(data map[string]any) { delete(data["roles"].(map[string]any), "reviewer") }
			}
			config := configCopy(t, dir, edit)
			role := filepath.Join(dir, "roles", "reviewer.md")
			if kind == "missing" {
				if err := os.Remove(role); err != nil {
					t.Fatal(err)
				}
			} else if kind == "misnamed" {
				raw := readFile(t, role)
				if err := os.WriteFile(role, []byte(strings.Replace(raw, "name: reviewer", "name: other", 1)), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			for _, flags := range [][]string{{}, {"--check"}} {
				args := append(append([]string{}, flags...), config, out)
				code, _, stderr := runCommand(t, args...)
				if code != 1 || !strings.Contains(stderr, "reviewer") {
					t.Fatalf("flags %v: exit %d, stderr %q", flags, code, stderr)
				}
				if !deepEqualStrings(treeOf(t, out), before) {
					t.Fatalf("flags %v changed the outputs", flags)
				}
			}
		})
	}
}

func TestIncompleteOrWidenedRoleRefused(t *testing.T) {
	edits := map[string][2]string{
		"description": {"description: ", "summary: "},
		"access":      {"access: read-search", "access: read-write"},
		"implement":   {"access: read-search", "access: implement"},
		"access line": {access, "Read anything."},
	}
	for label, edit := range edits {
		t.Run(label, func(t *testing.T) {
			dir := temp(t)
			config := configCopy(t, dir, nil)
			role := filepath.Join(dir, "roles", "reviewer.md")
			raw := readFile(t, role)
			if err := os.WriteFile(role, []byte(strings.Replace(raw, edit[0], edit[1], 1)), 0o644); err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(dir, "out")
			code, _, stderr := runCommand(t, config, output)
			if code != 1 || !strings.Contains(stderr, "reviewer.md") {
				t.Fatalf("exit %d, stderr %q", code, stderr)
			}
			if _, err := os.Stat(output); err == nil {
				t.Fatal("an invalid assignment wrote output")
			}
		})
	}
}

func TestAssignments(t *testing.T) {
	t.Run("inherit is explicit and translated per client", func(t *testing.T) {
		out := temp(t)
		_, stdout, _ := runCommand(t, configPath(), out)
		if !containsLine(head(t, filepath.Join(out, "claude", "reviewer.md")), "model: inherit") {
			t.Error("claude reviewer does not inherit the model")
		}
		for _, line := range head(t, filepath.Join(out, "claude", "reviewer.md")) {
			if strings.HasPrefix(line, "effort:") {
				t.Error("claude reviewer writes an effort it inherits")
			}
		}
		for _, line := range head(t, filepath.Join(out, "opencode", "reviewer.md")) {
			if strings.HasPrefix(line, "model:") || strings.HasPrefix(line, "reasoningEffort:") {
				t.Error("opencode reviewer writes a setting it inherits")
			}
		}
		if got := codexSettings(t, filepath.Join(out, "codex", "reviewer.toml")); len(got) != 0 {
			t.Errorf("codex reviewer writes settings it inherits: %v", got)
		}
		if !strings.Contains(stdout, "assignment: claude reviewer model=inherit (client default) effort=inherit (client default)") {
			t.Errorf("report = %s", stdout)
		}
	})

	t.Run("role overrides win over client defaults per client", func(t *testing.T) {
		dir := temp(t)
		config := configCopy(t, dir, func(data map[string]any) {
			data["clients"] = map[string]any{
				"claude":   map[string]any{"model": "opus", "effort": "medium"},
				"codex":    map[string]any{"model": "gpt-6-luna", "effort": "inherit"},
				"opencode": map[string]any{"model": "openai/gpt-5.5", "effort": "low"},
			}
			roles := data["roles"].(map[string]any)
			roles["reviewer"].(map[string]any)["claude"] = map[string]any{"model": "sonnet"}
			roles["reviewer"].(map[string]any)["codex"] = map[string]any{"model": "gpt-6.1-sol", "effort": "high"}
			roles["executor"].(map[string]any)["opencode"] = map[string]any{"effort": "high"}
			roles["code-search"].(map[string]any)["claude"] = map[string]any{"model": "haiku", "effort": "low"}
		})
		out := filepath.Join(dir, "out")
		_, stdout, _ := runCommand(t, config, out)
		assertHead(t, filepath.Join(out, "claude", "reviewer.md"), []string{"model: sonnet", "effort: medium"})
		assertHead(t, filepath.Join(out, "claude", "executor.md"), []string{"model: opus", "effort: medium"})
		assertHead(t, filepath.Join(out, "opencode", "executor.md"), []string{"model: openai/gpt-5.5", "reasoningEffort: high"})
		assertHead(t, filepath.Join(out, "opencode", "reviewer.md"), []string{"model: openai/gpt-5.5", "reasoningEffort: low"})
		assertHead(t, filepath.Join(out, "claude", "code-search.md"), []string{"model: haiku", "effort: low"})
		if got := codexSettings(t, filepath.Join(out, "codex", "reviewer.toml")); strings.Join(got, "|") != `model = "gpt-6.1-sol"|model_reasoning_effort = "high"` {
			t.Errorf("codex reviewer settings = %v", got)
		}
		if got := codexSettings(t, filepath.Join(out, "codex", "executor.toml")); strings.Join(got, "|") != `model = "gpt-6-luna"` {
			t.Errorf("codex executor settings = %v", got)
		}
		if code, _, stderr := runCommand(t, "--check", config, out); code != 0 {
			t.Fatalf("check exited %d: %s", code, stderr)
		}
		if !strings.Contains(stdout, "assignment: claude reviewer model=sonnet (role) effort=medium (client default)") ||
			!strings.Contains(stdout, "assignment: opencode executor model=openai/gpt-5.5 (client default) effort=high (role)") {
			t.Errorf("report = %s", stdout)
		}
	})

	t.Run("invalid assignments are reported before writing", func(t *testing.T) {
		out := temp(t)
		if code, _, _ := runCommand(t, configPath(), out); code != 0 {
			t.Fatal("generate failed")
		}
		before := treeOf(t, out)
		cases := map[string]func(map[string]any){
			"gpt-5": func(data map[string]any) {
				data["roles"].(map[string]any)["reviewer"].(map[string]any)["claude"] = map[string]any{"model": "gpt-5"}
			},
			"extreme": func(data map[string]any) {
				data["clients"].(map[string]any)["claude"].(map[string]any)["effort"] = "extreme"
			},
			"gpt-5.5": func(data map[string]any) {
				data["clients"].(map[string]any)["opencode"].(map[string]any)["model"] = "gpt-5.5"
			},
			"openai/": func(data map[string]any) {
				data["clients"].(map[string]any)["opencode"].(map[string]any)["effort"] = "high"
			},
			"anthropic/claude-opus-5": func(data map[string]any) {
				data["clients"].(map[string]any)["opencode"] = map[string]any{"model": "anthropic/claude-opus-5", "effort": "high"}
			},
			"gemini": func(data map[string]any) {
				data["roles"].(map[string]any)["reviewer"].(map[string]any)["gemini"] = map[string]any{"model": "gpt-5"}
			},
			"codex effort 'none'": func(data map[string]any) {
				data["clients"].(map[string]any)["codex"].(map[string]any)["effort"] = "none"
			},
			"codex model 'openai/gpt-5.5'": func(data map[string]any) {
				data["roles"].(map[string]any)["reviewer"].(map[string]any)["codex"] = map[string]any{"model": "openai/gpt-5.5"}
			},
			"temperature": func(data map[string]any) {
				data["roles"].(map[string]any)["reviewer"].(map[string]any)["claude"] = map[string]any{"temperature": 0}
			},
			"effort": func(data map[string]any) {
				delete(data["clients"].(map[string]any)["claude"].(map[string]any), "effort")
			},
			"opencode": func(data map[string]any) {
				delete(data["clients"].(map[string]any), "opencode")
			},
			"code-search": func(data map[string]any) {
				data["roles"].(map[string]any)["code-search"].(map[string]any)["claude"] = map[string]any{"model": "gpt-5"}
			},
			"../x": func(data map[string]any) {
				data["roles"].(map[string]any)["../x"] = map[string]any{}
			},
			"roles.reviewer.definition": func(data map[string]any) {
				data["roles"].(map[string]any)["reviewer"].(map[string]any)["definition"] = 5
			},
			"roles.executor.definition": func(data map[string]any) {
				data["roles"].(map[string]any)["executor"].(map[string]any)["definition"] = nil
			},
		}
		for expected, edit := range cases {
			t.Run(expected, func(t *testing.T) {
				dir := temp(t)
				config := configCopy(t, dir, edit)
				for _, flags := range [][]string{{}, {"--check"}} {
					args := append(append([]string{}, flags...), config, out)
					code, _, stderr := runCommand(t, args...)
					if code != 1 || !strings.Contains(stderr, expected) {
						t.Fatalf("flags %v: exit %d, stderr %q; want %q", flags, code, stderr, expected)
					}
					if !deepEqualStrings(treeOf(t, out), before) {
						t.Fatalf("flags %v changed the outputs", flags)
					}
				}
			})
		}
	})

	t.Run("duplicate keys are refused", func(t *testing.T) {
		dir := temp(t)
		config := configCopy(t, dir, nil)
		raw := readFile(t, config)
		if err := os.WriteFile(config, []byte(strings.Replace(raw, `"roles":{`, `"roles":{"reviewer":{},`, 1)), 0o644); err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(dir, "out")
		code, _, stderr := runCommand(t, config, out)
		if code != 1 || !strings.Contains(stderr, "duplicate") {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
		if _, err := os.Stat(out); err == nil {
			t.Fatal("a duplicate-key config wrote output")
		}
	})

	t.Run("assignment change reaches generated output", func(t *testing.T) {
		dir := temp(t)
		config := configCopy(t, dir, nil)
		out := filepath.Join(dir, "out")
		if code, _, _ := runCommand(t, config, out); code != 0 {
			t.Fatal("generate failed")
		}
		data := readConfig(t, config)
		data["roles"].(map[string]any)["reviewer"].(map[string]any)["claude"] = map[string]any{"model": "sonnet"}
		encoded, _ := json.Marshal(data)
		if err := os.WriteFile(config, encoded, 0o644); err != nil {
			t.Fatal(err)
		}
		code, stdout, _ := runCommand(t, "--check", config, out)
		if code != 1 || !strings.Contains(stdout, "changed: claude/reviewer.md") {
			t.Fatalf("check = %d, %q", code, stdout)
		}
		if code, _, stderr := runCommand(t, config, out); code != 0 {
			t.Fatalf("regenerate exited %d: %s", code, stderr)
		}
		assertHead(t, filepath.Join(out, "claude", "reviewer.md"), []string{"model: sonnet"})
	})

	t.Run("assigned role without definition generates nothing", func(t *testing.T) {
		dir := temp(t)
		config := configCopy(t, dir, func(data map[string]any) {
			data["roles"].(map[string]any)["future-role"] = map[string]any{}
		})
		out := filepath.Join(dir, "out")
		_, stdout, _ := runCommand(t, config, out)
		if len(treeOf(t, out)) != len(outputNames) {
			t.Fatalf("tree = %v", treeOf(t, out))
		}
		if !strings.Contains(stdout, "pending: future-role") {
			t.Errorf("report = %s", stdout)
		}
	})

	t.Run("the reviewer is required", func(t *testing.T) {
		dir := temp(t)
		config := configCopy(t, dir, func(data map[string]any) {
			delete(data["roles"].(map[string]any)["reviewer"].(map[string]any), "definition")
		})
		code, _, stderr := runCommand(t, config, filepath.Join(dir, "out"))
		if code != 1 || !strings.Contains(stderr, "reviewer must be assigned with a definition") {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
	})

	t.Run("model availability is reported unverified", func(t *testing.T) {
		out := temp(t)
		_, stdout, _ := runCommand(t, configPath(), out)
		if strings.Contains(stdout, "to this account") {
			t.Error("inherit assignments reported an availability warning")
		}
		dir := temp(t)
		config := configCopy(t, dir, func(data map[string]any) {
			data["roles"].(map[string]any)["reviewer"].(map[string]any)["claude"] = map[string]any{"model": "sonnet", "effort": "max"}
		})
		_, stdout, _ = runCommand(t, config, filepath.Join(dir, "out"))
		if !strings.Contains(stdout, "unverified: claude reviewer model sonnet is available to this account") ||
			!strings.Contains(stdout, "unverified: claude reviewer effort max is supported by its model") {
			t.Errorf("report = %s", stdout)
		}
	})

	t.Run("generated frontmatter carries only agent settings", func(t *testing.T) {
		dir := temp(t)
		config := configCopy(t, dir, func(data map[string]any) {
			data["clients"] = map[string]any{
				"claude":   map[string]any{"model": "claude-opus-5", "effort": "high"},
				"codex":    map[string]any{"model": "gpt-6-luna", "effort": "high"},
				"opencode": map[string]any{"model": "openai/gpt-5.5", "effort": "high"},
			}
		})
		out := filepath.Join(dir, "out")
		if code, _, stderr := runCommand(t, config, out); code != 0 {
			t.Fatalf("generate exited %d: %s", code, stderr)
		}
		allowed := map[string]map[string]bool{
			"claude":   {"name": true, "description": true, "tools": true, "disallowedTools": true, "permissionMode": true, "hooks": true, "model": true, "effort": true},
			"opencode": {"description": true, "mode": true, "permission": true, "model": true, "reasoningEffort": true},
		}
		for relative := range treeOf(t, out) {
			client := strings.Split(relative, "/")[0]
			if client == "codex" {
				// TOML, whose keys RestrictionsMatch already limits.
				continue
			}
			for _, line := range head(t, filepath.Join(out, filepath.FromSlash(relative))) {
				if line == "" || strings.HasPrefix(line, " ") {
					continue
				}
				key := strings.SplitN(line, ":", 2)[0]
				if !allowed[client][key] {
					t.Errorf("%s carries unexpected frontmatter key %q", relative, key)
				}
			}
		}
	})
}

func TestCheckDetectsWideningBesideAssignments(t *testing.T) {
	dir := temp(t)
	config := configCopy(t, dir, func(data map[string]any) {
		data["clients"].(map[string]any)["opencode"] = map[string]any{"model": "openai/gpt-5.5", "effort": "high"}
	})
	out := filepath.Join(dir, "out")
	if code, _, stderr := runCommand(t, config, out); code != 0 {
		t.Fatalf("generate exited %d: %s", code, stderr)
	}
	target := filepath.Join(out, "opencode", "reviewer.md")
	text := readFile(t, target)
	if err := os.WriteFile(target, []byte(strings.Replace(text, "  \"*\": deny", "  \"*\": allow", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, _ := runCommand(t, "--check", config, out)
	if code != 1 || !strings.Contains(stdout, "widened: opencode/reviewer.md") {
		t.Fatalf("check = %d, %q", code, stdout)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func containsLine(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}

func assertHead(t *testing.T, path string, want []string) {
	t.Helper()
	var got []string
	for _, line := range head(t, path) {
		if strings.HasPrefix(line, "model:") || strings.HasPrefix(line, "effort:") || strings.HasPrefix(line, "reasoningEffort:") {
			got = append(got, line)
		}
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("%s settings = %v, want %v", path, got, want)
	}
}

// codexSettings lists a Codex agent's model and effort lines.
func codexSettings(t *testing.T, path string) []string {
	t.Helper()
	var got []string
	for _, line := range strings.Split(readFile(t, path), "\n") {
		if strings.HasPrefix(line, "model = ") || strings.HasPrefix(line, "model_reasoning_effort = ") {
			got = append(got, line)
		}
	}
	return got
}

func deepEqualStrings(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}
