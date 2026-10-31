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
		// Skills and roles are executable agent instructions.
		{"skills/x/SKILL.md", Structural, "agent instruction"},
		{"skills/x/references/notes.md", Structural, "agent instruction"},
		{"skills/x/scripts/run_test.go", Structural, "agent instruction"},
		{"roles/executor.md", Structural, "agent instruction"},
		{"elsewhere/SKILL.md", Structural, "agent instruction"},
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

// TestClassifyPrecedence pins first-match-wins: critical beats mechanical beats tests-docs beats the structural exception.
func TestClassifyPrecedence(t *testing.T) {
	cfg := Config{CriticalPaths: []string{"go.sum", "docs/**", "skills/land/**", "**/*_test.go"}}
	cases := []struct {
		path string
		want Tier
	}{
		{"go.sum", Critical},                   // configured beats lockfile
		{"docs/readme.md", Critical},           // configured beats docs
		{"skills/land/SKILL.md", Critical},     // configured beats the skills exception
		{"internal/a/a_test.go", Critical},     // configured beats test file
		{"db/migrations/a_test.go", Critical},  // built-in beats test file
		{"db/migrations/go.sum", Critical},     // built-in beats lockfile
		{"queries/schema_gen.sql", Critical},   // built-in beats generated
		{"test/api.pb.go", Mechanical},         // generated beats a test directory
		{"docs/package-lock.json", Critical},   // configured docs/** is checked first
		{"tests/yarn.lock", Mechanical},        // lockfile beats a test directory
		{"skills/x/yarn.lock", Mechanical},     // lockfile beats the skills exception
		{"skills/x/api.pb.go", Mechanical},     // generated beats the skills exception
		{"skills/x/notes.md", Structural},      // exception beats docs
		{"roles/a/tests/case.txt", Structural}, // exception beats tests
	}
	for _, c := range cases {
		if got := Classify(c.path, cfg).Tier; got != c.want {
			t.Errorf("Classify(%q) = %s, want %s", c.path, got, c.want)
		}
	}
}

// TestMechanicalOnlyFromRules guards the floor contract: no path the rules do not name is ever mechanical.
func TestMechanicalOnlyFromRules(t *testing.T) {
	for _, path := range []string{"a.go", "a.lock", "lock.json", "go.mod", "gen.go", "a.pb.gox", "x/generated/a.go", "README.md", "a.sum"} {
		if got := Classify(path, Config{}).Tier; got == Mechanical {
			t.Errorf("Classify(%q) = mechanical without a deterministic rule", path)
		}
	}
}

func TestBuiltinCriticalIsDocumented(t *testing.T) {
	want := []string{".github/workflows/**", "**/migrations/**", "**/*.sql", "**/Dockerfile", "**/*.tf"}
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
