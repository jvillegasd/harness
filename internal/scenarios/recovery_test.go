package scenarios

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var recoveryOutcomeKeys = []string{
	"continuation_recovers_requirement", "continuation_recovers_rationale",
	"continuation_not_repeated_in_prompt", "continuation_not_shared_conversation",
	"continuation_unrelated_excluded", "continuation_memory_isolated",
	"continuation_missing_is_rejected", "continuation_missing_requirement_rejected",
	"continuation_unavailable_is_unverified", "continuation_store_unavailable_is_unverified",
	"headless_proven_stop", "post_stop_edit_rejected", "uncertain_no_fallback",
	"uncertain_fallback_rejected", "worker_identities_required", "worker_unavailable_is_unverified",
}

func TestContinuationOracle(t *testing.T) {
	base := continuationFacts{
		Prompt:         "recover the branch checkpoint",
		Recovered:      "next_action wire /ship continuation retrieval | rationale the branch scopes recovery",
		Requirement:    "wire /ship continuation retrieval",
		Rationale:      "the branch scopes recovery",
		Unrelated:      []string{"a different next action"},
		MemoryReadable: true,
		MemoryIsolated: true,
		RouteAvailable: true,
	}
	cases := []struct {
		name string
		edit func(*continuationFacts)
		want string
	}{
		{"compliant recovery", func(*continuationFacts) {}, continuationRecovered},
		{"requirement repeated in prompt", func(f *continuationFacts) { f.Prompt = "recover wire /ship continuation retrieval" }, continuationRepeatedInPrompt},
		{"rationale repeated in prompt", func(f *continuationFacts) { f.Prompt = "recover the branch scopes recovery" }, continuationRepeatedInPrompt},
		{"shared conversation", func(f *continuationFacts) { f.SharedTranscript = true }, continuationSharedConversation},
		{"unrelated value supplied", func(f *continuationFacts) { f.Recovered += " a different next action" }, continuationUnrelatedSupplied},
		{"memory not isolated", func(f *continuationFacts) { f.MemoryIsolated = false }, continuationMemoryNotIsolated},
		{"route unavailable", func(f *continuationFacts) { f.RouteAvailable = false }, continuationUnverified},
		{"store unreadable", func(f *continuationFacts) { f.MemoryReadable = false }, continuationUnverified},
		{"missing rationale", func(f *continuationFacts) { f.Recovered = "next_action wire /ship continuation retrieval" }, continuationMissingRationale},
		{"missing requirement", func(f *continuationFacts) { f.Recovered = "rationale the branch scopes recovery" }, continuationMissingRequirement},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			facts := base
			c.edit(&facts)
			got := continuationOracle(facts)
			if got.Outcome != c.want {
				t.Fatalf("outcome = %q, want %q (%v)", got.Outcome, c.want, got.Reasons)
			}
			if continuationAccepted(got.Outcome) != (got.Outcome == continuationRecovered) {
				t.Fatalf("accepted disagrees with the outcome %q", got.Outcome)
			}
		})
	}
}

func TestWorkerRecoveryOracle(t *testing.T) {
	proven := workerFacts{Identities: true, Available: true, ProvenStopped: true}
	uncertain := workerFacts{Identities: true, Available: true, Surviving: []string{"pid 9"}}
	cases := []struct {
		name    string
		edit    func(*workerFacts)
		want    string
		accepts bool
	}{
		{"proven headless stop", func(*workerFacts) {}, workerProvenStop, true},
		{"post-stop edit", func(f *workerFacts) { f.PostStopEdits = []string{"impl/late.txt"} }, workerPostStopEdit, false},
		{"surviving worker", func(f *workerFacts) { f.ProvenStopped = false; f.Surviving = []string{"pid 9"} }, workerUncertainNoFallback, true},
		{"uncertain no fallback", func(f *workerFacts) { *f = uncertain }, workerUncertainNoFallback, true},
		{"uncertain with fallback", func(f *workerFacts) { *f = uncertain; f.FallbackStarted = true }, workerUncertainFallback, false},
		{"unrecorded identity", func(f *workerFacts) { f.Identities = false }, workerUnverified, false},
		{"route unavailable", func(f *workerFacts) { f.Available = false }, workerUnverified, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			facts := proven
			c.edit(&facts)
			got := workerRecoveryOracle(facts)
			if got.Outcome != c.want {
				t.Fatalf("outcome = %q, want %q (%v)", got.Outcome, c.want, got.Reasons)
			}
			if workerAccepted(got.Outcome) != c.accepts {
				t.Fatalf("accepted = %v, want %v for %q", !c.accepts, c.accepts, got.Outcome)
			}
		})
	}
}

func TestRecoveryProbe(t *testing.T) {
	outcomes, err := probeRecoveryRun(testRepoRoot())
	if err != nil {
		t.Fatal(err)
	}
	haveKeys(t, outcomes, recoveryOutcomeKeys)
	allTrue(t, outcomes)
}

// TestRecoveryProbeReadsTheCheckpointSchema repoints the protocol at a document without it and expects the probe to refuse.
func TestRecoveryProbeReadsTheCheckpointSchema(t *testing.T) {
	saved := checkpointSkillPath
	t.Cleanup(func() { checkpointSkillPath = saved })
	checkpointSkillPath = writeTemp(t, "# no protocol block\n")
	if _, err := probeRecoveryRun(testRepoRoot()); err == nil {
		t.Fatal("the probe accepted a skill without a canonical protocol block")
	}
	checkpointSkillPath = saved
	if _, err := probeRecoveryRun(testRepoRoot()); err != nil {
		t.Fatal(err)
	}
}

// TestRecoveryMatrixIsTheBudgetedLiveHome keeps the live experiments declared and budget-gated in their own matrix, while the default matrix runs the offline oracle.
func TestRecoveryMatrixIsTheBudgetedLiveHome(t *testing.T) {
	recovery := loadTestMatrixAt(t, filepath.Join(testRepoRoot(), "scenarios", "matrix.recovery.json"))
	if errors := validateMatrix(recovery); len(errors) != 0 {
		t.Fatalf("recovery matrix errors: %v", errors)
	}
	declared := list(recovery, "scenarios")
	if len(declared) < 3 {
		t.Fatalf("recovery matrix declares %d scenarios, want at least three", len(declared))
	}
	routes := map[string]bool{}
	for _, raw := range declared {
		scenario, _ := raw.(map[string]any)
		if mode(scenario) != "live" {
			t.Fatalf("%s is not a live scenario", getString(scenario, "id"))
		}
		if !truthy(scenario["sessions"]) && !truthy(scenario["worker"]) {
			t.Fatalf("%s declares neither sessions nor a worker", getString(scenario, "id"))
		}
		if worker, ok := scenario["worker"].(map[string]any); ok {
			routes[getString(worker, "route")] = true
		}
	}
	if !routes[routeHeadless] || !routes[routeOrca] {
		t.Fatalf("the recovery matrix does not cover both worker routes: %v", routes)
	}
	for _, id := range []string{"checkpoint-continuation", "worker-recovery"} {
		scenario := findScenario(loadTestMatrix(t), id)
		if scenario == nil || mode(scenario) != "offline" || !truthy(scenario["check"]) {
			t.Fatalf("the default matrix does not run %s offline", id)
		}
	}
}

func TestRecoveryDeclarationValidation(t *testing.T) {
	live := func(extra map[string]any) map[string]any {
		base := map[string]any{"id": "x", "mode": "live"}
		for key, value := range extra {
			base[key] = value
		}
		return testScenario(base)
	}
	cases := []struct {
		name     string
		scenario map[string]any
		want     string
	}{
		{"one session", live(map[string]any{
			"sessions":     []any{map[string]any{"id": "a", "prompt": "p"}},
			"continuation": map[string]any{"branch": "b", "requirement": "r", "rationale": "w", "unrelated_value": "u"},
		}), "at least two sessions"},
		{"duplicate session", live(map[string]any{
			"sessions":     []any{map[string]any{"id": "a", "prompt": "p"}, map[string]any{"id": "a", "prompt": "p"}},
			"continuation": map[string]any{"branch": "b", "requirement": "r", "rationale": "w", "unrelated_value": "u"},
		}), "duplicate session id"},
		{"missing continuation values", live(map[string]any{
			"sessions": []any{map[string]any{"id": "a", "prompt": "p"}, map[string]any{"id": "b", "prompt": "p"}},
		}), "continuation.branch"},
		{"bad worker route", live(map[string]any{
			"worker": map[string]any{"route": "socket", "role": "executor", "session": "claude"},
		}), "worker.route"},
		{"worker without role", live(map[string]any{
			"worker": map[string]any{"route": routeHeadless, "session": "claude"},
		}), "worker.role"},
		{"bad settle", live(map[string]any{
			"worker": map[string]any{"route": routeHeadless, "role": "executor", "session": "claude", "settle_seconds": 0},
		}), "settle_seconds"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if errors := joinErrors(validateMatrix(map[string]any{"scenarios": []any{c.scenario}})); !contains(errors, c.want) {
				t.Fatalf("validateMatrix = %v, want an error naming %q", errors, c.want)
			}
		})
	}
}

func writeStub(t *testing.T, dir, name, script string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

// retainedRecovery returns the classification the live runner retained in the attempt transcript, so a review binds to what actually ran.
func retainedRecovery(t *testing.T, where string) string {
	t.Helper()
	raw, err := readText(filepath.Join(where, "transcript.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range splitLines(raw) {
		decoded, err := decodeJSON([]byte(line))
		if err != nil {
			continue
		}
		entry, _ := decoded.(map[string]any)
		if getString(entry, "stream") == "recovery" {
			return getString(entry, "text")
		}
	}
	t.Fatal("the attempt retained no recovery classification")
	return ""
}

// TestContinuationLiveWithStubSessions exercises the live path end to end: two fresh stub processes supply the recovered values and the oracle accepts a checkpoint recovery, with the identities retained in the attempt transcript.
func TestContinuationLiveWithStubSessions(t *testing.T) {
	stubDir := t.TempDir()
	writeStub(t, stubDir, "claude", `#!/bin/sh
mkdir -p recovery
printf '%s\n' '{"next_action":"wire /ship continuation retrieval","rationale":"the branch scopes recovery","project":"scenario-continuation-live"}' > recovery/session-two.json
echo '{"type":"result","model":"stub"}'
`)
	writeStub(t, stubDir, "engram", "#!/bin/sh\n[ \"$1\" = search ] || exit 1\necho 'wire /ship continuation retrieval'\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	scenario := testScenario(map[string]any{
		"id": "continuation-live", "mode": "live", "client": "claude",
		"sessions": []any{
			map[string]any{"id": "record", "prompt": "record {{requirement}} and {{rationale}} for {{branch}}"},
			map[string]any{"id": "recover", "prompt": "recover {{branch}}"},
		},
		"continuation": map[string]any{
			"branch": "docs/69", "requirement": "wire /ship continuation retrieval",
			"rationale": "the branch scopes recovery", "unrelated_branch": "feat/99",
			"unrelated_value": "a different next action",
		},
		"artifacts": map[string]any{"contains": []any{map[string]any{"path": "artifacts/recovery.json", "text": "\"accepted\": true"}}},
	})
	state, where, err := runOne(scenario, testRepoRoot(), t.TempDir(), filepath.Join(testRepoRoot(), "scenarios", "fixtures"), true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if state != "unreviewed" {
		t.Fatalf("state = %q at %s, want unreviewed", state, where)
	}
	recovery := retainedRecovery(t, where)
	for _, want := range []string{"\"outcome\": \"recovered\"", "\"identities\"", "\"revision\":"} {
		if !strings.Contains(recovery, want) {
			t.Fatalf("the retained classification lacks %q:\n%s", want, recovery)
		}
	}
}

func workerStub(t *testing.T, dispatchBehavior string) {
	t.Helper()
	stubDir := t.TempDir()
	writeStub(t, stubDir, "claude", "#!/bin/sh\necho '{\"type\":\"result\"}'\n")
	writeStub(t, stubDir, "orca", "#!/bin/sh\nexit 1\n")
	writeStub(t, stubDir, "harness", `#!/bin/sh
case "$1" in
models) echo '{"role":"executor","client":"opencode","model":"opencode-go/glm-5.3","effort":"","source":"preset:balanced","route":"headless"}' ;;
dispatch) `+dispatchBehavior+` ;;
esac
`)
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestWorkerLiveProvenStop drives the real dispatch route with a stub that reports a timeout from a stop that left nothing behind, so the oracle proves the stop.
func TestWorkerLiveProvenStop(t *testing.T) {
	workerStub(t, `echo '{"reason":"timeout","detail":"stopped"}'; exit 1`)
	scenario := testScenario(map[string]any{
		"id": "worker-live", "mode": "live", "client": "claude",
		"worker":    map[string]any{"route": routeHeadless, "role": "executor", "session": "claude"},
		"artifacts": map[string]any{"contains": []any{map[string]any{"path": "artifacts/recovery.json", "text": "\"accepted\": true"}}},
	})
	state, where, err := runOne(scenario, testRepoRoot(), t.TempDir(), filepath.Join(testRepoRoot(), "scenarios", "fixtures"), true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if state != "unreviewed" {
		t.Fatalf("state = %q at %s, want unreviewed", state, where)
	}
	if recovery := retainedRecovery(t, where); !strings.Contains(recovery, "\"outcome\": \"proven_stop\"") {
		t.Fatalf("the worker was not classified as a proven stop:\n%s", recovery)
	}
}

// TestWorkerLiveUncertainStartsNoFallback keeps an uncertain outcome conservative: the runner starts no fallback, so the oracle accepts waiting rather than a second writer.
func TestWorkerLiveUncertainStartsNoFallback(t *testing.T) {
	workerStub(t, `echo '{"reason":"outcome_unknown","detail":"may still be running"}'; exit 1`)
	scenario := testScenario(map[string]any{
		"id": "worker-live", "mode": "live", "client": "claude",
		"worker":    map[string]any{"route": routeHeadless, "role": "executor", "session": "claude"},
		"artifacts": map[string]any{"contains": []any{map[string]any{"path": "artifacts/recovery.json", "text": "\"accepted\": true"}}},
	})
	state, where, err := runOne(scenario, testRepoRoot(), t.TempDir(), filepath.Join(testRepoRoot(), "scenarios", "fixtures"), true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if state != "unreviewed" {
		t.Fatalf("state = %q at %s, want unreviewed from the conservative no-fallback wait", state, where)
	}
	if recovery := retainedRecovery(t, where); !strings.Contains(recovery, "\"outcome\": \"uncertain_no_fallback\"") {
		t.Fatalf("an uncertain outcome was not classified conservatively:\n%s", recovery)
	}
}

// TestWorkerLiveUnreachableRouteIsUnverified keeps a declared route that the environment cannot reach out of behavioral acceptance.
func TestWorkerLiveUnreachableRouteIsUnverified(t *testing.T) {
	workerStub(t, `echo '{"reason":"timeout","detail":"stopped"}'; exit 1`)
	scenario := testScenario(map[string]any{
		"id": "worker-live", "mode": "live", "client": "claude",
		"worker":    map[string]any{"route": routeOrca, "role": "executor", "session": "claude"},
		"artifacts": map[string]any{"contains": []any{map[string]any{"path": "artifacts/recovery.json", "text": "\"accepted\": true"}}},
	})
	// The declared Orca route is unreachable from a disposable repository, so the observed route is headless and the experiment is unverified rather than a false pass.
	state, where, err := runOne(scenario, testRepoRoot(), t.TempDir(), filepath.Join(testRepoRoot(), "scenarios", "fixtures"), true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if state != "incomplete" {
		t.Fatalf("state = %q at %s, want incomplete from the unreachable declared route", state, where)
	}
	if recovery := retainedRecovery(t, where); !strings.Contains(recovery, "\"outcome\": \"unverified\"") {
		t.Fatalf("an unreachable route was not unverified:\n%s", recovery)
	}
}

// TestWorkerLivePostStopEditIsRejected proves the runner observes a workspace change made after the recorded stop and refuses to treat it as a proven stop.
func TestWorkerLivePostStopEditIsRejected(t *testing.T) {
	workerStub(t, `( sleep 0.2; mkdir -p impl; echo late > impl/late.txt ) >/dev/null 2>&1 & echo '{"reason":"timeout","detail":"stopped"}'; exit 1`)
	scenario := testScenario(map[string]any{
		"id": "worker-live", "mode": "live", "client": "claude",
		"worker":    map[string]any{"route": routeHeadless, "role": "executor", "session": "claude", "settle_seconds": 1},
		"artifacts": map[string]any{"contains": []any{map[string]any{"path": "artifacts/recovery.json", "text": "\"accepted\": true"}}},
	})
	state, where, err := runOne(scenario, testRepoRoot(), t.TempDir(), filepath.Join(testRepoRoot(), "scenarios", "fixtures"), true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if state != "fail" {
		t.Fatalf("state = %q at %s, want fail", state, where)
	}
	if recovery := retainedRecovery(t, where); !strings.Contains(recovery, "\"outcome\": \"post_stop_edit\"") {
		t.Fatalf("the post-stop edit was not classified:\n%s", recovery)
	}
}

// TestRecoveryWithoutBudgetDoesNotRun keeps the live path honest offline: no attempt is written without --allow-paid.
func TestRecoveryWithoutBudgetDoesNotRun(t *testing.T) {
	scenario := testScenario(map[string]any{
		"id": "continuation-live", "mode": "live", "client": "claude",
		"sessions":     []any{map[string]any{"id": "a", "prompt": "p"}, map[string]any{"id": "b", "prompt": "p"}},
		"continuation": map[string]any{"branch": "b", "requirement": "r", "rationale": "w", "unrelated_value": "u"},
	})
	out := t.TempDir()
	state, _, err := runOne(scenario, testRepoRoot(), out, filepath.Join(testRepoRoot(), "scenarios", "fixtures"), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if state != "not-run" {
		t.Fatalf("state = %q, want not-run", state)
	}
	if attempts := attemptPaths(out, "continuation-live"); len(attempts) != 0 {
		t.Fatalf("a not-run recovery scenario wrote an attempt: %v", attempts)
	}
}

// TestRecoveryReportNamesRoutes records which concrete routes ran and which did not, so an unapproved budget reads as unverified rather than a pass.
func TestRecoveryReportNamesRoutes(t *testing.T) {
	matrix := loadTestMatrixAt(t, filepath.Join(testRepoRoot(), "scenarios", "matrix.recovery.json"))
	text := report(matrix, t.TempDir(), testRepoRoot())
	for _, want := range []string{"live coverage:", "route=fresh-session client=claude", "route=headless client=claude", "route=orca client=claude", "unverified (no attempt)"} {
		if !contains(text, want) {
			t.Fatalf("report missing %q:\n%s", want, text)
		}
	}
}

// TestReportSeparatesLiveFromOffline keeps an offline simulation and a live result visibly distinct even before any attempt exists.
func TestReportSeparatesLiveFromOffline(t *testing.T) {
	matrix := loadTestMatrix(t)
	text := report(matrix, t.TempDir(), testRepoRoot())
	for _, want := range []string{"ask-read-only: incomplete (no attempt) [live:claude]", "read-only-execution: incomplete (no attempt) [offline]", "live coverage:", "ask-read-only: unverified (no attempt) route=direct client=claude"} {
		if !contains(text, want) {
			t.Fatalf("report missing %q:\n%s", want, text)
		}
	}
}
