package changemap

import (
	"path"
	"strings"
)

// Tier is how much human attention a changed file needs, from nothing to read to read first. The command assigns only a floor: a model may raise a tier but never lower it.
type Tier string

const (
	Mechanical Tier = "mechanical" // lockfiles and generated output; no judgment to review
	TestsDocs  Tier = "tests-docs" // tests and documentation; read for intent, not behavior
	Structural Tier = "structural" // the default: code or instructions whose effect a reviewer must judge
	Behavioral Tier = "behavioral" // reserved for a model to assign; no deterministic rule produces it
	Critical   Tier = "critical"   // paths where a mistake is costly: infrastructure, migrations, configured critical paths
)

// Tiers lists every tier from lowest to highest.
var Tiers = []Tier{Mechanical, TestsDocs, Structural, Behavioral, Critical}

// Rank orders tiers; a higher rank needs more attention.
func (t Tier) Rank() int {
	for i, tier := range Tiers {
		if tier == t {
			return i
		}
	}
	return -1
}

// Result is a floor tier with the short reason a reader can check. Results sharing a Group are reported as one entry, so a vendored body does not bury the files a human reads.
type Result struct {
	Tier   Tier
	Reason string
	Group  string
}

// BuiltinCritical are the paths critical in any repository: automation that runs with repository credentials, and schema or infrastructure definitions. A repository adds its own through criticalPaths.
var BuiltinCritical = []string{
	ConfigPath,    // it decides every other file's floor
	SkillLockPath, // it decides which vendored bodies collapse, and names their upstream source
	".github/workflows/**",
	"**/migrations/**",
	"**/*.sql",
	"**/Dockerfile",
	"**/*.tf",
}

// lockfiles are matched by base name. Their content is derived from a manifest, so the manifest is what a human reads.
var lockfiles = []string{
	"go.sum", "package-lock.json", "yarn.lock", "pnpm-lock.yaml", "Cargo.lock",
	"poetry.lock", "uv.lock", "Gemfile.lock", "composer.lock",
}

// generatedPatterns are base-name globs for generated output.
var generatedPatterns = []string{"*.pb.go", "*_gen.go", "*.gen.*", "*_generated.*"}

// testFilePatterns and docFilePatterns are base-name globs; testDirs are path segments.
var (
	testFilePatterns = []string{"*_test.go", "*.test.*", "*.spec.*"}
	testDirs         = []string{"test", "tests", "__tests__", "testdata"}
	docFilePatterns  = []string{"*.md", "*.mdx", "*.rst", "*.txt"}
)

// Classify assigns the floor tier for one repo-relative, slash-separated path. The first rule that matches wins, in this order: critical, the skill lock and locked bodies, mechanical, the agent-instruction exception, tests and docs, then the structural default. Mechanical comes only from locked skill bodies and the lockfile and generated rules, so a path no rule names is never skipped as noise.
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

	// A locked body collapses only while it still hashes to its lock entry, so the critical lock change that names its upstream source is what a human reads instead. A body that no longer matches has been edited in place, which is exactly what must not hide.
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
		// A path the hash never covered, such as a metadata.json or a symlink leading out of the folder, takes the ordinary rules.
		break
	}

	base := path.Base(file)
	for _, name := range lockfiles {
		if base == name {
			return Result{Tier: Mechanical, Reason: "lockfile"}
		}
	}
	if matchesAny(generatedPatterns, base) {
		return Result{Tier: Mechanical, Reason: "generated file"}
	}

	// The instruction files clients load (SKILL.md, AGENTS.md, CLAUDE.md) and the prose under skills/ and roles/ are executable agent instructions even when they look like docs or tests, so they never drop to tests-docs. Code under those prefixes, such as a skill's scripts and their tests, falls through to the ordinary rules. The prefix is top-level only: internal/skills is Go code about skills, not a skill.
	if base == "SKILL.md" || base == "AGENTS.md" || base == "CLAUDE.md" ||
		((strings.HasPrefix(file, "skills/") || strings.HasPrefix(file, "roles/")) && matchesAny(docFilePatterns, base)) {
		return Result{Tier: Structural, Reason: "agent instruction; read as executable"}
	}

	segments := strings.Split(file, "/")
	for _, segment := range segments[:len(segments)-1] {
		for _, dir := range testDirs {
			if segment == dir {
				return Result{Tier: TestsDocs, Reason: "test directory " + dir}
			}
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
