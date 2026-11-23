package changemap

import (
	"path"
	"slices"
	"strings"
)

type Tier string

const (
	Mechanical Tier = "mechanical"
	TestsDocs  Tier = "tests-docs"
	Structural Tier = "structural"
	Behavioral Tier = "behavioral"
	Critical   Tier = "critical"
)

var Tiers = []Tier{Mechanical, TestsDocs, Structural, Behavioral, Critical}

func (t Tier) Rank() int {
	return slices.Index(Tiers, t)
}

type Result struct {
	Tier   Tier
	Reason string
	Group  string
}

var BuiltinCritical = []string{
	ConfigPath,
	SkillLockPath,
	".github/workflows/**",
	"**/migrations/**",
	"**/*.sql",
	"**/Dockerfile",
	"**/*.tf",
}

var lockfiles = []string{
	"go.sum", "package-lock.json", "yarn.lock", "pnpm-lock.yaml", "Cargo.lock",
	"poetry.lock", "uv.lock", "Gemfile.lock", "composer.lock",
}

var generatedPatterns = []string{"*.pb.go", "*_gen.go", "*.gen.*", "*_generated.*"}

var (
	testFilePatterns = []string{"*_test.go", "*.test.*", "*.spec.*"}
	testDirs         = []string{"test", "tests", "__tests__", "testdata"}
	docFilePatterns  = []string{"*.md", "*.mdx", "*.rst", "*.txt"}
)

func Classify(file string, cfg Config) Result {
	for _, glob := range cfg.CriticalPaths {
		if Match(glob, file) {
			return Result{Tier: Critical, Reason: "configured critical path " + glob}
		}
	}
	for _, glob := range BuiltinCritical {
		if Match(glob, file) {
			return Result{Tier: Critical, Reason: "built-in critical path " + glob}
		}
	}

	for _, lock := range cfg.Locked {
		if !strings.HasPrefix(file, lock.Dir+"/") {
			continue
		}
		if !lock.Verified {
			return Result{Tier: Structural, Reason: "locked body " + lock.Name + " does not match its lock hash; read the edit"}
		}
		if lock.Hashed[strings.TrimPrefix(file, lock.Dir+"/")] {
			return Result{Tier: Mechanical, Reason: "locked body " + lock.Name + " from " + lock.Source + ", matching its lock hash", Group: lock.Dir + "/"}
		}

		break
	}

	base := path.Base(file)
	if slices.Contains(lockfiles, base) {
		return Result{Tier: Mechanical, Reason: "lockfile"}
	}
	if matchesAny(generatedPatterns, base) {
		return Result{Tier: Mechanical, Reason: "generated file"}
	}

	if base == "SKILL.md" || base == "AGENTS.md" || base == "CLAUDE.md" ||
		((strings.HasPrefix(file, "skills/") || strings.HasPrefix(file, "roles/")) && matchesAny(docFilePatterns, base)) {
		return Result{Tier: Structural, Reason: "agent instruction; read as executable"}
	}

	segments := strings.Split(file, "/")
	for _, segment := range segments[:len(segments)-1] {
		if slices.Contains(testDirs, segment) {
			return Result{Tier: TestsDocs, Reason: "test directory " + segment}
		}
	}
	if matchesAny(testFilePatterns, base) {
		return Result{Tier: TestsDocs, Reason: "test file"}
	}
	if segments[0] == "docs" && len(segments) > 1 {
		return Result{Tier: TestsDocs, Reason: "documentation under docs/"}
	}
	if matchesAny(docFilePatterns, base) {
		return Result{Tier: TestsDocs, Reason: "documentation"}
	}

	return Result{Tier: Structural, Reason: "no deterministic signal; judge whether behavior changes"}
}

func matchesAny(patterns []string, name string) bool {
	for _, pattern := range patterns {
		if ok, _ := path.Match(pattern, name); ok {
			return true
		}
	}
	return false
}
