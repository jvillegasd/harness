package changemap

import (
	"strings"
	"testing"
)

func contains(s, sub string) bool { return strings.Contains(s, sub) }

func TestTierOrder(t *testing.T) {
	want := []Tier{Mechanical, TestsDocs, Structural, Behavioral, Critical}
	if len(Tiers) != len(want) {
		t.Fatalf("Tiers = %v, want %v", Tiers, want)
	}
	for i, tier := range want {
		if Tiers[i] != tier {
			t.Fatalf("Tiers[%d] = %q, want %q", i, Tiers[i], tier)
		}
		if i > 0 && Tiers[i].Rank() <= Tiers[i-1].Rank() {
			t.Errorf("%q does not rank above %q", Tiers[i], Tiers[i-1])
		}
	}
}

func TestClassify(t *testing.T) {
	cfg := Config{CriticalPaths: []string{"POLICY.md", "clients/**", "internal/install/**"}}
	cases := []struct {
		path string
		want Tier
		// reason is a fragment the reason must contain.
		reason string
	}{
		// Configured critical globs name themselves.
		{"POLICY.md", Critical, "POLICY.md"},
		{"clients/claude/settings.json", Critical, "clients/**"},
		{"internal/install/install.go", Critical, "internal/install/**"},
		// Built-in critical paths.
		{".github/workflows/ci.yml", Critical, ".github/workflows/**"},
		{"db/migrations/001_init.go", Critical, "**/migrations/**"},
		{"schema/up.sql", Critical, "**/*.sql"},
		{"Dockerfile", Critical, "**/Dockerfile"},
		{"svc/api/Dockerfile", Critical, "**/Dockerfile"},
		{"infra/main.tf", Critical, "**/*.tf"},
		// Lockfiles and generated files are mechanical.
		{"go.sum", Mechanical, "lockfile"},
		{"web/package-lock.json", Mechanical, "lockfile"},
		{"yarn.lock", Mechanical, "lockfile"},
		{"pnpm-lock.yaml", Mechanical, "lockfile"},
		{"Cargo.lock", Mechanical, "lockfile"},
		{"poetry.lock", Mechanical, "lockfile"},
		{"uv.lock", Mechanical, "lockfile"},
		{"Gemfile.lock", Mechanical, "lockfile"},
		{"composer.lock", Mechanical, "lockfile"},
		{"api/service.pb.go", Mechanical, "generated"},
		{"x/models_gen.go", Mechanical, "generated"},
		{"x/schema.gen.ts", Mechanical, "generated"},
		{"x/types_generated.go", Mechanical, "generated"},
		// Tests and docs.
		{"internal/a/a_test.go", TestsDocs, "test"},
		{"web/app.test.ts", TestsDocs, "test"},
		{"web/app.spec.js", TestsDocs, "test"},
		{"pkg/test/helper.go", TestsDocs, "test"},
		{"pkg/tests/helper.go", TestsDocs, "test"},
		{"web/__tests__/a.js", TestsDocs, "test"},
		{"cmd/harness/testdata/a.golden", TestsDocs, "test"},
		{"README.md", TestsDocs, "doc"},
		{"notes/a.mdx", TestsDocs, "doc"},
		{"notes/a.rst", TestsDocs, "doc"},
		{"notes/a.txt", TestsDocs, "doc"},
		{"docs/agents/tasks/x.json", TestsDocs, "doc"},
		// Skill and role prose is executable agent instruction; skill code takes the ordinary rules.
		{"skills/x/SKILL.md", Structural, "agent instruction"},
		{"skills/x/references/notes.md", Structural, "agent instruction"},
		{"skills/x/scripts/run_test.go", TestsDocs, "test"},
		{"skills/x/scripts/test/cli.test.ts", TestsDocs, "test"},
		{"skills/x/scripts/run.go", Structural, "no deterministic signal"},
		{"skills/x/assets/workflow.yml", Structural, "no deterministic signal"},
		{"skills-lock.json", Critical, "skills-lock.json"},
		{"roles/executor.md", Structural, "agent instruction"},
		{"elsewhere/SKILL.md", Structural, "agent instruction"},
		{"docs/agents/harness.json", Critical, "docs/agents/harness.json"},
		{"AGENTS.md", Structural, "agent instruction"},
		{"sub/AGENTS.md", Structural, "agent instruction"},
		{"CLAUDE.md", Structural, "agent instruction"},
		// Anything else is judged by a human or a model.
		{"internal/a/a.go", Structural, "no deterministic signal; judge whether behavior changes"},
		{"main.go", Structural, "no deterministic signal; judge whether behavior changes"},
		// skills/ is a top-level prefix only.
		{"internal/skills/guards_test.go", TestsDocs, "test"},
	}
	for _, c := range cases {
		got := Classify(c.path, cfg)
		if got.Tier != c.want || !contains(got.Reason, c.reason) {
			t.Errorf("Classify(%q) = {%s, %q}, want tier %s with reason containing %q", c.path, got.Tier, got.Reason, c.want, c.reason)
		}
	}
}

// TestClassifyPrecedence pins first-match-wins: critical beats a locked body beats mechanical beats tests-docs beats the structural exception.
func TestClassifyPrecedence(t *testing.T) {
	cfg := Config{
		CriticalPaths: []string{"go.sum", "docs/**", "skills/land/**", "**/*_test.go"},
		Locked:        []Lock{{"skills/vendored", "vendored", "o/r", true}, {"skills/land", "land", "o/r", true}},
	}
	cases := []struct {
		path string
		want Tier
	}{
		{"go.sum", Critical},                           // configured beats lockfile
		{"docs/readme.md", Critical},                   // configured beats docs
		{"skills/land/SKILL.md", Critical},             // configured beats the skills exception
		{"internal/a/a_test.go", Critical},             // configured beats test file
		{"db/migrations/a_test.go", Critical},          // built-in beats test file
		{"db/migrations/go.sum", Critical},             // built-in beats lockfile
		{"queries/schema_gen.sql", Critical},           // built-in beats generated
		{"test/api.pb.go", Mechanical},                 // generated beats a test directory
		{"docs/package-lock.json", Critical},           // configured docs/** is checked first
		{"tests/yarn.lock", Mechanical},                // lockfile beats a test directory
		{"skills/x/yarn.lock", Mechanical},             // lockfile beats the skills exception
		{"skills/x/api.pb.go", Mechanical},             // generated beats the skills exception
		{"skills/x/notes.md", Structural},              // exception beats docs
		{"roles/a/tests/case.txt", Structural},         // exception beats tests
		{"skills/vendored/SKILL.md", Mechanical},       // a locked body beats the skills exception
		{"skills/vendored/scripts/run.go", Mechanical}, // and beats the structural default
		{"skills/land/scripts/a.go", Critical},         // configured beats a locked body
	}
	for _, c := range cases {
		if got := Classify(c.path, cfg).Tier; got != c.want {
			t.Errorf("Classify(%q) = %s, want %s", c.path, got, c.want)
		}
	}
}

// TestMechanicalOnlyFromRules guards the floor contract: no path the rules do not name is ever mechanical, including inside a locked folder whose body no longer matches its lock.
func TestMechanicalOnlyFromRules(t *testing.T) {
	cfg := Config{Locked: []Lock{{"skills/edited", "edited", "o/r", false}}}
	for _, path := range []string{"a.go", "a.lock", "lock.json", "go.mod", "gen.go", "a.pb.gox", "x/generated/a.go", "README.md", "a.sum", "skills/edited/SKILL.md", "skills/edited/run.go"} {
		if got := Classify(path, cfg).Tier; got == Mechanical {
			t.Errorf("Classify(%q) = mechanical without a deterministic rule", path)
		}
	}
}

func TestBuiltinCriticalIsDocumented(t *testing.T) {
	want := []string{ConfigPath, SkillLockPath, ".github/workflows/**", "**/migrations/**", "**/*.sql", "**/Dockerfile", "**/*.tf"}
	if len(BuiltinCritical) != len(want) {
		t.Fatalf("BuiltinCritical = %v, want %v", BuiltinCritical, want)
	}
	for i, glob := range want {
		if BuiltinCritical[i] != glob {
			t.Errorf("BuiltinCritical[%d] = %q, want %q", i, BuiltinCritical[i], glob)
		}
		if err := ValidateGlob(glob); err != nil {
			t.Errorf("built-in %q is not a valid glob: %v", glob, err)
		}
	}
}

// TestLockedBodiesCollapse reports a locked body as one mechanical entry that names its source and file count, without hiding the lock file or a configured critical path inside it.
func TestLockedBodiesCollapse(t *testing.T) {
	cfg := Config{
		CriticalPaths: []string{"skills/vendored/hook.sh"},
		Locked:        []Lock{{"skills/vendored", "vendored", "owner/repo", true}, {"skills/edited", "edited", "owner/repo", false}},
	}
	changes := Changes{Files: []string{"skills/vendored/SKILL.md", "skills/vendored/a/b.ts", "skills/vendored/hook.sh", "skills/vendored-other/x.md", "skills-lock.json", "main.go", "skills/edited/a_test.go"}}
	report := Build(changes, cfg)
	byPath := map[string]File{}
	for _, f := range report.Files {
		byPath[f.Path] = f
	}
	group, ok := byPath["skills/vendored/"]
	if !ok || group.Tier != Mechanical || group.Files != 2 || !contains(group.Reason, "owner/repo") {
		t.Fatalf("locked group = %+v, want one mechanical entry for 2 files naming owner/repo; report %+v", group, report.Files)
	}
	if byPath["skills/vendored/hook.sh"].Tier != Critical {
		t.Errorf("configured critical file inside a locked body was not critical: %+v", report.Files)
	}
	if byPath["skills/vendored-other/x.md"].Tier != Structural || byPath["skills-lock.json"].Tier != Critical {
		t.Errorf("a sibling prefix was collapsed or the lock file was not critical: %+v", report.Files)
	}
	if edited := byPath["skills/edited/a_test.go"]; edited.Tier != Structural || !contains(edited.Reason, "does not match its lock hash") {
		t.Errorf("an edited locked body was not raised and named: %+v", edited)
	}
	if report.Totals[Mechanical] != 1 || len(report.Files) != 6 {
		t.Errorf("totals %v over %d entries, want one mechanical entry among 6", report.Totals, len(report.Files))
	}
	if !contains(report.Text(), "2 files: locked body vendored") {
		t.Errorf("text output does not name the file count:\n%s", report.Text())
	}
}
