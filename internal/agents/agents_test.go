package agents

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var outputNames = []string{
	"claude/code-search.md", "claude/executor.md", "claude/reviewer.md", "claude/review-bugs.md", "claude/review-risk.md", "claude/review-spec.md", "claude/worker.md",
	"codex/code-search.toml", "codex/executor.toml", "codex/reviewer.toml", "codex/review-bugs.toml", "codex/review-risk.toml", "codex/review-spec.toml", "codex/worker.toml",
	"opencode/code-search.md", "opencode/executor.md", "opencode/reviewer.md", "opencode/review-bugs.md", "opencode/review-risk.md", "opencode/review-spec.md", "opencode/worker.md",
}

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

// runCommand renders every role as generic, so each renders on every client
// and a test can read each output's restrictions.
func runCommand(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	generic := slices.Clone(Roles)
	for i := range generic {
		generic[i].Generic = true
	}
	return runRoles(t, nil, generic, args...)
}

func runWith(t *testing.T, assigned map[string]Assigned, args ...string) (int, string, string) {
	t.Helper()
	return runRoles(t, assigned, Roles, args...)
}

func runRoles(t *testing.T, assigned map[string]Assigned, roles []Role, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if args[0] == "--check" {
		return checkRoles(args[1], args[2], assigned, roles, &stdout, &stderr), stdout.String(), stderr.String()
	}
	code := generateRoles(args[0], args[1], assigned, roles, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func rootCopy(t *testing.T, dir string) string {
	t.Helper()
	for _, role := range Roles {
		raw, err := os.ReadFile(filepath.Join(repoRoot(), filepath.FromSlash(role.Definition)))
		if err != nil {
			t.Fatal(err)
		}
		path := definition(dir, role.Name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func definition(root, name string) string {
	for _, role := range Roles {
		if role.Name == name {
			return filepath.Join(root, filepath.FromSlash(role.Definition))
		}
	}
	panic("unknown role " + name)
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
	code, _, stderr := runCommand(t, repoRoot(), out)
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
	if code, stdout, stderr := runCommand(t, repoRoot(), out); code != 0 || !strings.Contains(stdout, "output: changed") {
		t.Fatalf("first generate exited %d with %q: %s", code, stdout, stderr)
	}
	first := treeOf(t, out)
	code, stdout, stderr := runCommand(t, repoRoot(), out)
	if code != 0 {
		t.Fatalf("second generate exited %d, want 0: %s", code, stderr)
	}
	if !strings.Contains(stdout, "output: unchanged") {
		t.Fatalf("second generate did not report its output as unchanged: %s", stdout)
	}
	if !deepEqualStrings(treeOf(t, out), first) {
		t.Fatal("idempotent generation changed the tree")
	}
}

func TestEachClientGetsItsControls(t *testing.T) {
	out := temp(t)
	if code, _, stderr := runCommand(t, repoRoot(), out); code != 0 {
		t.Fatalf("generate exited %d: %s", code, stderr)
	}
	for _, role := range []string{"reviewer"} {
		claude := readFile(t, filepath.Join(out, "claude", role+".md"))
		opencode := readFile(t, filepath.Join(out, "opencode", role+".md"))
		codex := readFile(t, filepath.Join(out, "codex", role+".toml"))
		if !strings.HasPrefix(codex, "name = \""+role+"\"\n") || !strings.HasSuffix(codex, "\n'''\n\n[features]\napps = false\nplugins = false\n") {
			t.Errorf("%s codex controls are wrong:\n%s", role, codex)
		}
		if !strings.Contains(claude, "\nname: "+role+"\n") || !strings.Contains(claude, "\ntools: Read, Grep, Glob, Bash, Skill\n") {
			t.Errorf("%s claude frontmatter is wrong:\n%s", role, claude)
		}
		if !strings.Contains(opencode, "\nmode: all\npermission:\n  \"*\": deny\n") || !strings.Contains(opencode, "\n    \"*.env\": deny\n") {
			t.Errorf("%s opencode controls are wrong:\n%s", role, opencode)
		}
	}
}

func TestExecutorKeepsImplementationAccess(t *testing.T) {
	out := temp(t)
	if code, _, stderr := runCommand(t, repoRoot(), out); code != 0 {
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
	if code, _, stderr := runCommand(t, "--check", repoRoot(), out); code != 0 {
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
			config := rootCopy(t, dir)
			rolePath := definition(dir, c.target)
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
		{"claude/reviewer.md", "tools: Read, Grep, Glob, Bash, Skill\n", ""},
		{"opencode/reviewer.md", "  \"*\": deny", "  \"*\": allow"},
		{"opencode/reviewer.md", "  list: allow\n", "  list: allow\n  bash: allow\n"},
		{"opencode/reviewer.md", "mode: all\n", "mode: all\ntools:\n  bash: true\n"},
		{"claude/code-search.md", "tools: Read, Grep, Glob, Bash, Skill\n", "tools: Read, Grep, Glob, Edit\n"},
		{"opencode/code-search.md", "  grep: allow\n", "  grep: allow\n  task: allow\n"},
		{"claude/executor.md", "disallowedTools: Agent\n", ""},
		{"opencode/executor.md", "  task: deny\n", "  task: allow\n"},
		{"codex/reviewer.toml", "plugins = false\n", "plugins = true\n"},
		{"codex/reviewer.toml", "[features]\napps = false\n", "[features]\n"},
		{"codex/code-search.toml", "developer_instructions = '''\n", "sandbox_mode = \"danger-full-access\"\ndeveloper_instructions = '''\n"},
		{"codex/executor.toml", "\n'''\n", "\n'''\n\n[features]\nmulti_agent = true\n"},
		{"codex/executor.toml", "\n'''\n", "'''\nsandbox_mode = \"danger-full-access\"\nnote = '''\nx\n'''\n"},
	}
	for _, mutation := range mutations {
		t.Run(mutation.path+mutation.new, func(t *testing.T) {
			out := temp(t)
			if code, _, stderr := runCommand(t, repoRoot(), out); code != 0 {
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
			code, stdout, _ := runCommand(t, "--check", repoRoot(), out)
			if code != 1 || !strings.Contains(stdout, "widened: "+mutation.path) {
				t.Fatalf("check = %d, %q; want widened %s", code, stdout, mutation.path)
			}
		})
	}
}

func TestControlCharactersInADefinitionAreRefused(t *testing.T) {
	dir := temp(t)
	config := rootCopy(t, dir)
	path := definition(dir, "reviewer")
	if err := os.WriteFile(path, []byte(strings.Replace(readFile(t, path), "# ", "# \x1b[1m", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runCommand(t, config, filepath.Join(dir, "out"))
	if code != 1 || !strings.Contains(stderr, "control characters") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}

func TestCheckDetectsMissingAndDrifted(t *testing.T) {
	out := temp(t)
	if code, _, _ := runCommand(t, repoRoot(), out); code != 0 {
		t.Fatal("generate failed")
	}
	target := filepath.Join(out, "claude", "reviewer.md")
	raw := readFile(t, target)
	if err := os.WriteFile(target, []byte(raw+"\nExtra instruction.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, _ := runCommand(t, "--check", repoRoot(), out)
	if code != 1 || !strings.Contains(stdout, "changed: claude/reviewer.md") {
		t.Fatalf("check = %d, %q", code, stdout)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	code, stdout, _ = runCommand(t, "--check", repoRoot(), out)
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
	if code, _, stderr := runCommand(t, repoRoot(), out); code != 0 {
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
			if code, _, stderr := runCommand(t, repoRoot(), output); code != 0 {
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
				args := append(append([]string{}, flags...), repoRoot(), output)
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

func TestMissingAndMisnamedRolesPreserveOutputs(t *testing.T) {
	out := temp(t)
	if code, _, _ := runCommand(t, repoRoot(), out); code != 0 {
		t.Fatal("generate failed")
	}
	before := treeOf(t, out)
	for _, kind := range []string{"missing", "misnamed"} {
		t.Run(kind, func(t *testing.T) {
			dir := temp(t)
			config := rootCopy(t, dir)
			role := definition(dir, "reviewer")
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
			config := rootCopy(t, dir)
			role := definition(dir, "reviewer")
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
	t.Run("a generic role inherits on every client", func(t *testing.T) {
		out := temp(t)
		_, stdout, _ := runCommand(t, repoRoot(), out)
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
		if !strings.Contains(stdout, "assignment: claude reviewer model=inherit (chosen at launch) effort=inherit (chosen at launch)") {
			t.Errorf("report = %s", stdout)
		}
		if strings.Contains(stdout, "to this account") {
			t.Error("inherit assignments reported an availability warning")
		}
	})

	t.Run("an assignment reaches only its client", func(t *testing.T) {
		out := temp(t)
		assigned := map[string]Assigned{
			"reviewer":    {Client: "codex", Model: "gpt-6.1-sol", Effort: "high", Source: "preset:diversity"},
			"executor":    {Client: "opencode", Model: "openai/gpt-5.5", Effort: "high", Source: "override"},
			"code-search": {Client: "claude", Model: "haiku", Source: "preset:diversity"},
			"review-bugs": {Client: "claude", Model: "sonnet", Effort: "max", Source: "reviewer"},
		}
		code, stdout, stderr := runWith(t, assigned, repoRoot(), out)
		if code != 0 {
			t.Fatalf("generate exited %d: %s", code, stderr)
		}
		if got := codexSettings(t, filepath.Join(out, "codex", "reviewer.toml")); strings.Join(got, "|") != `model = "gpt-6.1-sol"|model_reasoning_effort = "high"` {
			t.Errorf("codex reviewer settings = %v", got)
		}
		assertHead(t, filepath.Join(out, "opencode", "executor.md"), []string{"model: openai/gpt-5.5", "reasoningEffort: high"})
		// A client a role is not assigned to gets no file, so a launch there fails
		// instead of running on the launching session's model.
		for _, absent := range []string{"claude/reviewer.md", "opencode/reviewer.md", "claude/executor.md", "codex/executor.toml", "codex/code-search.toml", "codex/review-bugs.toml"} {
			if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(absent))); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("%s was rendered for a client its role is not assigned to", absent)
			}
		}
		// A role with no runnable model gets no file anywhere; the worker, whose
		// tier is chosen at launch, keeps one on every client.
		for _, absent := range []string{"claude/review-spec.md", "codex/review-spec.toml", "opencode/review-spec.md"} {
			if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(absent))); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("%s was rendered for a role with no assigned model", absent)
			}
		}
		assertHead(t, filepath.Join(out, "claude", "worker.md"), []string{"model: inherit"})
		assertHead(t, filepath.Join(out, "claude", "code-search.md"), []string{"model: haiku"})
		assertHead(t, filepath.Join(out, "claude", "review-bugs.md"), []string{"model: sonnet", "effort: max"})
		for _, want := range []string{
			"assignment: codex reviewer model=gpt-6.1-sol (preset:diversity) effort=high (preset:diversity)",
			"assignment: opencode executor model=openai/gpt-5.5 (override) effort=high (override)",
			"assignment: claude code-search model=haiku (preset:diversity) effort=inherit (preset:diversity)",
			"unverified: claude review-bugs model sonnet is available to this account",
			"unverified: claude review-bugs effort max is supported by its model",
		} {
			if !strings.Contains(stdout, want) {
				t.Errorf("report lacks %q:\n%s", want, stdout)
			}
		}
		if code, stdout, _ := runWith(t, assigned, "--check", repoRoot(), out); code != 0 {
			t.Fatalf("check exited %d: %s", code, stdout)
		}
		if code, stdout, _ := runCommand(t, "--check", repoRoot(), out); code != 1 || !strings.Contains(stdout, "changed: codex/reviewer.toml") {
			t.Fatalf("check without the assignment = %d, %q", code, stdout)
		}
	})

	t.Run("invalid assignments are reported before writing", func(t *testing.T) {
		out := temp(t)
		if code, _, _ := runCommand(t, repoRoot(), out); code != 0 {
			t.Fatal("generate failed")
		}
		before := treeOf(t, out)
		cases := map[string]Assigned{
			"gpt-5":                   {Client: "claude", Model: "gpt-5"},
			"extreme":                 {Client: "claude", Model: "opus", Effort: "extreme"},
			"gpt-5.5":                 {Client: "opencode", Model: "gpt-5.5"},
			"anthropic/claude-opus-5": {Client: "opencode", Model: "anthropic/claude-opus-5", Effort: "high"},
			"codex effort 'none'":     {Client: "codex", Model: "gpt-6-luna", Effort: "none"},
			"'openai/gpt-5.5'":        {Client: "codex", Model: "openai/gpt-5.5"},
			"newline":                 {Client: "opencode", Model: "openai/x\nmode: x"},
		}
		for expected, a := range cases {
			t.Run(expected, func(t *testing.T) {
				assigned := map[string]Assigned{"reviewer": a}
				for _, flags := range [][]string{{}, {"--check"}} {
					args := append(append([]string{}, flags...), repoRoot(), out)
					code, _, stderr := runWith(t, assigned, args...)
					if code != 1 || !strings.Contains(stderr, "reviewer") {
						t.Fatalf("flags %v: exit %d, stderr %q", flags, code, stderr)
					}
					if !deepEqualStrings(treeOf(t, out), before) {
						t.Fatalf("flags %v changed the outputs", flags)
					}
				}
			})
		}
	})

	t.Run("generated frontmatter carries only agent settings", func(t *testing.T) {
		out := temp(t)
		assigned := map[string]Assigned{}
		for i, role := range Roles {
			assigned[role.Name] = []Assigned{
				{Client: "claude", Model: "claude-opus-5", Effort: "high"},
				{Client: "codex", Model: "gpt-6-luna", Effort: "high"},
				{Client: "opencode", Model: "openai/gpt-5.5", Effort: "high"},
			}[i%3]
		}
		if code, _, stderr := runWith(t, assigned, repoRoot(), out); code != 0 {
			t.Fatalf("generate exited %d: %s", code, stderr)
		}
		allowed := map[string]map[string]bool{
			"claude":   {"name": true, "description": true, "tools": true, "disallowedTools": true, "permissionMode": true, "hooks": true, "model": true, "effort": true},
			"opencode": {"description": true, "mode": true, "permission": true, "model": true, "reasoningEffort": true},
		}
		for relative := range treeOf(t, out) {
			client := strings.Split(relative, "/")[0]
			if client == "codex" {
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
	out := temp(t)
	assigned := map[string]Assigned{"reviewer": {Client: "opencode", Model: "openai/gpt-5.5", Effort: "high"}}
	if code, _, stderr := runWith(t, assigned, repoRoot(), out); code != 0 {
		t.Fatalf("generate exited %d: %s", code, stderr)
	}
	target := filepath.Join(out, "opencode", "reviewer.md")
	text := readFile(t, target)
	if err := os.WriteFile(target, []byte(strings.Replace(text, "  \"*\": deny", "  \"*\": allow", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, _ := runWith(t, assigned, "--check", repoRoot(), out)
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

func TestLensExtendsTheReviewer(t *testing.T) {
	out := temp(t)
	if code, _, stderr := runCommand(t, repoRoot(), out); code != 0 {
		t.Fatalf("generate exited %d: %s", code, stderr)
	}
	lens := readFile(t, filepath.Join(out, "claude", "review-spec.md"))
	body := func(name string) string {
		t.Helper()
		parts := strings.SplitN(readFile(t, filepath.Join(repoRoot(), "skills", "delivery", "review", "roles", name)), "---\n", 3)
		if len(parts) != 3 {
			t.Fatalf("%s has no frontmatter", name)
		}
		return strings.TrimSpace(parts[2])
	}
	core, own := strings.Index(lens, body("reviewer.md")), strings.Index(lens, body("review-spec.md"))
	if core < 0 || own < core {
		t.Errorf("review-spec does not carry the reviewer text followed by its lens:\n%s", lens)
	}
	for _, want := range []string{"name: review-spec\n", "permissionMode: dontAsk\n", "models read-guard"} {
		if !strings.Contains(lens, want) {
			t.Errorf("review-spec lacks %q", want)
		}
	}
}

func TestExtendsIsOneLevelDeep(t *testing.T) {
	dir := temp(t)
	config := rootCopy(t, dir)
	base := definition(dir, "reviewer")
	raw := readFile(t, base)
	if err := os.WriteFile(base, []byte(strings.Replace(raw, "access: read-search\n", "access: read-search\nextends: review-spec.md\n", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runCommand(t, config, filepath.Join(dir, "out")); code != 1 || !strings.Contains(stderr, "extends another definition itself") {
		t.Errorf("exit %d, stderr %q", code, stderr)
	}
}

func TestExtendsRequiresTheSameAccess(t *testing.T) {
	dir := temp(t)
	config := rootCopy(t, dir)
	base := readFile(t, definition(dir, "reviewer"))
	writable := strings.Replace(strings.Replace(base, "access: read-search", "access: implement", 1), "name: reviewer", "name: unassigned", 1)
	lens := definition(dir, "review-bugs")
	if err := os.WriteFile(filepath.Join(filepath.Dir(lens), "writable-base.md"), []byte(writable), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lens, []byte(strings.Replace(readFile(t, lens), "extends: reviewer.md", "extends: writable-base.md", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runCommand(t, config, filepath.Join(dir, "out")); code != 1 || !strings.Contains(stderr, "the access of the definition extending it") {
		t.Errorf("exit %d, stderr %q", code, stderr)
	}
}
