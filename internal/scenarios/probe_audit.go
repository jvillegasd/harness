package scenarios

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var auditKeys = []string{
	"sources",
	"verdict_requires",
	"finding_rule",
	"unavailable_state",
	"transcript_mode",
	"transcript_signals",
	"effects",
	"owned_by_setup_harness",
}

const transcriptScanCap = 200

var auditCommandPath = filepath.Join("commands", "harness-audit.md")

func probeAuditBlock(root string) (map[string]any, error) {
	text, err := readText(joinPath(root, auditCommandPath))
	if err != nil {
		return nil, err
	}
	return findBlock(text, auditKeys)
}

func auditDigest(root string, skipGit bool) string {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return "missing"
	}
	var files []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		files = append(files, path)
		return nil
	})
	sort.Strings(files)
	var parts []string
	for _, path := range files {
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if skipGit {
			skip := false
			for _, part := range strings.Split(rel, "/") {
				if part == ".git" {
					skip = true
				}
			}
			if skip {
				continue
			}
		}
		parts = append(parts, rel+":"+fileDigest(path))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}

func auditGitState(repo string) string {
	cmd := exec.Command("git", "-C", repo, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return "not-a-repository"
	}
	return string(out)
}

// auditBuildFixture materializes mixed evidence: four present, one unreadable, one empty, one missing.
func auditBuildFixture(fixture string) error {
	mkdir := func(parts ...string) error {
		return os.MkdirAll(filepath.Join(append([]string{fixture}, parts...)...), 0o755)
	}
	write := func(rel, text string) error {
		return os.WriteFile(filepath.Join(fixture, filepath.FromSlash(rel)), []byte(text), 0o644)
	}
	for _, dir := range []string{"worktrees", "engram_checkpoints", "pull_request_bodies", "commits", "tracker_state", "session_evidence"} {
		if err := mkdir(dir); err != nil {
			return err
		}
	}
	if err := write("worktrees/state.json", "{\"branch\": \"feat/78\"}\n"); err != nil {
		return err
	}
	if err := write("engram_checkpoints/record.json", "{\"state\": \"active\"}\n"); err != nil {
		return err
	}
	if err := write("pull_request_bodies/body.md", "Closes #78\n\n## Contract\n\n## Checks\n"); err != nil {
		return err
	}
	if err := write("commits/log.txt", "feat(harness): add audit\n"); err != nil {
		return err
	}
	return os.Symlink(filepath.Join(fixture, "tracker_state", "absent"), filepath.Join(fixture, "tracker_state", "dangling"))
}

func auditScan(block map[string]any, fixture string) map[string]string {
	states := map[string]string{}
	for _, raw := range list(block, "sources") {
		source, _ := raw.(string)
		directory := filepath.Join(fixture, source)
		info, err := os.Stat(directory)
		if err != nil || !info.IsDir() {
			states[source] = "missing"
			continue
		}
		entries, err := os.ReadDir(directory)
		if err != nil {
			states[source] = "unreadable"
			continue
		}
		dangling := false
		for _, e := range entries {
			if e.Type()&fs.ModeSymlink != 0 {
				if _, err := os.Stat(filepath.Join(directory, e.Name())); err != nil {
					dangling = true
				}
			}
		}
		switch {
		case dangling:
			states[source] = "unreadable"
		case len(entries) == 0:
			states[source] = "empty"
		default:
			states[source] = "present"
		}
	}
	return states
}

func auditCoverage(block map[string]any, states map[string]string) map[string]string {
	unknown := getString(block, "unavailable_state")
	report := map[string]string{}
	for _, raw := range list(block, "sources") {
		source, _ := raw.(string)
		if states[source] == "present" {
			report[source] = "present"
		} else {
			report[source] = unknown
		}
	}
	return report
}

func auditVerdict(block map[string]any, states map[string]string) *string {
	if getString(block, "verdict_requires") != "workflow_bearing_evidence" {
		return nil
	}
	for _, raw := range list(block, "sources") {
		if states[raw.(string)] == "present" {
			verdict := "reported"
			return &verdict
		}
	}
	return nil
}

func auditClassify(block map[string]any, observations []any) map[string][]string {
	rule := dict(block, "finding_rule")
	findings, noted := []string{}, []string{}
	for _, raw := range observations {
		observation, _ := raw.(map[string]any)
		count := len(asList(observation["occurrences"]))
		cost := truthy(observation["concrete_cost"])
		if count >= int(floatOf(rule["pattern_minimum"])) || (count == 1 && getString(rule, "single_requires") == "concrete_cost" && cost) {
			findings = append(findings, getString(observation, "id"))
		} else {
			noted = append(noted, getString(observation, "id"))
		}
	}
	return map[string][]string{"findings": findings, "observations": noted}
}

var auditHexRun = regexp.MustCompile(`(?i)\b[0-9a-f]{7,40}\b`)

// auditCommitSignal is the one canonical signal RE2 cannot compile, because it uses a digit lookahead; the fallback is keyed on this exact pattern so any other unparseable signal fails loudly.
const auditCommitSignal = `\b(?=[0-9a-f]*[0-9])[0-9a-f]{7,40}\b`

func auditSignalMatch(pattern, text string) (bool, error) {
	compiled, err := regexp.Compile("(?i)" + pattern)
	if err == nil {
		return compiled.MatchString(text), nil
	}
	if pattern != auditCommitSignal {
		return false, fmt.Errorf("unparseable transcript signal %q", pattern)
	}
	// RE2 has no lookahead, so the commit signal is matched as a hex run that carries a digit.
	for _, match := range auditHexRun.FindAllString(text, -1) {
		if hasDigit(match) {
			return true, nil
		}
	}
	return false, nil
}

func auditTranscriptSignals(block map[string]any, lines []string) (map[string]bool, error) {
	markers := dict(block, "transcript_signals")
	capped := lines
	if len(capped) > transcriptScanCap {
		capped = capped[:transcriptScanCap]
	}
	text := strings.Join(capped, "\n")
	out := map[string]bool{}
	for _, name := range sortedKeys(markers) {
		pattern, _ := markers[name].(string)
		matched, err := auditSignalMatch(pattern, text)
		if err != nil {
			return nil, err
		}
		out[name] = matched
	}
	return out, nil
}

// probeAuditRun scans a disposable fixture holding mixed evidence and applies the audit's coverage, verdict, finding-threshold, and bounded-transcript rules.
func probeAuditRun(root string, repo *string) (map[string]any, error) {
	block, err := probeAuditBlock(root)
	if err != nil {
		return nil, err
	}
	var owned []string
	defer func() {
		for _, path := range owned {
			_ = os.RemoveAll(path)
		}
	}()
	if repo == nil {
		created, err := os.MkdirTemp("", "harness-audit-repo-")
		if err != nil {
			return nil, err
		}
		owned = append(owned, created)
		init := exec.Command("git", "init", "-q")
		init.Dir = created
		_ = init.Run()
		repo = &created
	}
	repoPath := *repo
	fixture, err := os.MkdirTemp("", "harness-audit-fixture-")
	if err != nil {
		return nil, err
	}
	owned = append(owned, fixture)
	if err := auditBuildFixture(fixture); err != nil {
		return nil, err
	}

	fixtureBefore := auditDigest(fixture, false)
	repoBefore := auditDigest(repoPath, true)
	statusBefore := auditGitState(repoPath)
	states := auditScan(block, fixture)
	reportOut := auditCoverage(block, states)
	result := auditVerdict(block, states)
	fixtureAfter := auditDigest(fixture, false)
	repoAfter := auditDigest(repoPath, true)
	statusAfter := auditGitState(repoPath)

	present := map[string]bool{"worktrees": true, "engram_checkpoints": true, "pull_request_bodies": true, "commits": true}
	expected := map[string]string{}
	for _, raw := range list(block, "sources") {
		source, _ := raw.(string)
		if present[source] {
			expected[source] = "present"
		} else {
			expected[source] = getString(block, "unavailable_state")
		}
	}

	within := []string{"## Contract\n## Checks\nworktree checkpoint commit 6cee0bf ready-for-agent benchmark"}
	var cappedLines []string
	for n := 0; n < transcriptScanCap; n++ {
		cappedLines = append(cappedLines, "line "+formatNumber(float64(n)))
	}
	cappedLines = append(cappedLines, "## Contract ## Checks worktree checkpoint benchmark")
	withinSignals, err := auditTranscriptSignals(block, within)
	if err != nil {
		return nil, err
	}
	cappedSignals, err := auditTranscriptSignals(block, cappedLines)
	if err != nil {
		return nil, err
	}

	empty := map[string]string{}
	for _, raw := range list(block, "sources") {
		empty[raw.(string)] = "missing"
	}
	soloUncosted := []any{map[string]any{"id": "solo-uncosted", "occurrences": []any{"a"}, "concrete_cost": false}}
	soloCosted := []any{map[string]any{"id": "solo-costed", "occurrences": []any{"a"}, "concrete_cost": true}}
	patternCase := []any{map[string]any{"id": "pattern", "occurrences": []any{"a", "b"}, "concrete_cost": false}}

	unavailable := getString(block, "unavailable_state")
	effects := dict(block, "effects")
	allEffectsReadOnly := true
	for _, value := range effects {
		if truthy(value) {
			allEffectsReadOnly = false
		}
	}
	ownedBySetup := stringList(block["owned_by_setup_harness"])
	setupBoundary := len(ownedBySetup) == 2 &&
		containsString(ownedBySetup, "tracker_configuration") &&
		containsString(ownedBySetup, "lifecycle_configuration") &&
		containsString(stringList(block["sources"]), "tracker_state")
	cappedAny := false
	for _, value := range cappedSignals {
		if value {
			cappedAny = true
		}
	}

	return map[string]any{
		"all_sources_reported":               equalStringMaps(reportOut, expected),
		"present_is_recorded":                reportOut["worktrees"] == "present",
		"absent_is_unknown":                  reportOut["benchmark_results"] == unavailable && reportOut["session_evidence"] == unavailable,
		"unreadable_is_unknown":              reportOut["tracker_state"] == unavailable,
		"no_evidence_no_verdict":             auditVerdict(block, empty) == nil,
		"evidence_yields_verdict":            result != nil && *result == "reported",
		"single_without_cost_not_finding":    len(auditClassify(block, soloUncosted)["findings"]) == 0,
		"single_without_cost_observed":       equalStrings(auditClassify(block, soloUncosted)["observations"], []string{"solo-uncosted"}),
		"single_with_cost_is_finding":        equalStrings(auditClassify(block, soloCosted)["findings"], []string{"solo-costed"}),
		"pattern_two_occurrences_is_finding": equalStrings(auditClassify(block, patternCase)["findings"], []string{"pattern"}),
		"transcript_is_bounded": getString(block, "transcript_mode") == "bounded_named_signals" &&
			withinSignals["contract_section"] && !cappedSignals["contract_section"],
		"transcript_scan_is_capped":      !cappedAny,
		"transcript_finds_named_signals": withinSignals["contract_section"] && withinSignals["checks_section"],
		"effects_are_read_only":          allEffectsReadOnly,
		"setup_harness_boundary":         setupBoundary,
		"fixture_files_unchanged":        fixtureBefore == fixtureAfter,
		"audit_engine_writes_nothing":    repoBefore == repoAfter,
		"git_state_unchanged":            statusBefore == statusAfter,
	}, nil
}

func equalStringMaps(a, b map[string]string) bool {
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

func probeHarnessAudit(repo, root string) error {
	outcomes, err := probeAuditRun(root, &repo)
	if err != nil {
		return err
	}
	return writeArtifact(repo, "audit.json", outcomes)
}
