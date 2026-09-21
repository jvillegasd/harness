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
	"handoff_recovered_after_document_removed", "handoff_source_on_disk_rejected",
	"handoff_foreign_branch_excluded", "handoff_checks_invalidated_on_changed_revision",
	"handoff_unavailable_is_unverified",
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
		ProjectCleared: true,
	}
	cases := []struct {
		name string
		edit func(*continuationFacts)
		want string
	}{
		{"compliant recovery", func(*continuationFacts) {}, continuationRecovered},
		{"continuation project not cleared", func(f *continuationFacts) { f.ProjectCleared = false }, continuationUnverified},
		{"requirement repeated in prompt", func(f *continuationFacts) { f.Prompt = "recover wire /ship continuation retrieval" }, continuationRepeatedInPrompt},
		{"rationale repeated in prompt", func(f *continuationFacts) { f.Prompt = "recover the branch scopes recovery" }, continuationRepeatedInPrompt},
		{"shared conversation", func(f *continuationFacts) { f.SharedTranscript = true }, continuationSharedConversation},
		{"unrelated value supplied", func(f *continuationFacts) { f.Recovered += " a different next action" }, continuationUnrelatedSupplied},
		{"memory not isolated", func(f *continuationFacts) { f.MemoryIsolated = false }, continuationMemoryNotIsolated},
		{"route unavailable", func(f *continuationFacts) { f.RouteAvailable = false }, continuationUnverified},
		{"store unreadable", func(f *continuationFacts) { f.MemoryReadable = false }, continuationUnverified},
		{"missing rationale", func(f *continuationFacts) { f.Recovered = "next_action wire /ship continuation retrieval" }, continuationMissingRationale},
		{"missing requirement", func(f *continuationFacts) { f.Recovered = "rationale the branch scopes recovery" }, continuationMissingRequirement},
		{"handoff document removed", func(f *continuationFacts) {
			f.HandoffDeclared = true
			f.ProjectCleared = true
			f.DocumentRemoved = true
		}, continuationRecovered},
		{"test project not cleared", func(f *continuationFacts) {
			f.HandoffDeclared = true
			f.ProjectCleared = false
			f.DocumentRemoved = true
		}, continuationUnverified},
		{"value still readable on disk", func(f *continuationFacts) {
			f.HandoffDeclared = true
			f.ProjectCleared = true
			f.DocumentRemoved = true
			f.Residue = []string{"notes/leftover.md"}
		}, continuationSourceOnDisk},
		{"residue outranks the unrelated value", func(f *continuationFacts) {
			f.HandoffDeclared = true
			f.ProjectCleared = true
			f.DocumentRemoved = true
			f.Residue = []string{"notes/leftover.md"}
			f.Recovered += " a different next action"
		}, continuationSourceOnDisk},
		{"handoff document never composed", func(f *continuationFacts) { f.HandoffDeclared = true; f.ProjectCleared = true }, continuationUnverified},
		{"residue without a declared handoff is ignored", func(f *continuationFacts) { f.Residue = []string{"notes/leftover.md"} }, continuationRecovered},
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
		{"handoff without a document", live(map[string]any{
			"sessions":     []any{map[string]any{"id": "a", "prompt": "p"}, map[string]any{"id": "b", "prompt": "p"}},
			"continuation": map[string]any{"branch": "b", "requirement": "r", "rationale": "w", "unrelated_value": "u"},
			"handoff":      map[string]any{},
		}), "handoff.document"},
		{"handoff document outside the repository", live(map[string]any{
			"sessions":     []any{map[string]any{"id": "a", "prompt": "p"}, map[string]any{"id": "b", "prompt": "p"}},
			"continuation": map[string]any{"branch": "b", "requirement": "r", "rationale": "w", "unrelated_value": "u"},
			"handoff":      map[string]any{"document": "../escape.md"},
		}), "handoff.document must be a path inside the repository"},
		{"handoff with three sessions", live(map[string]any{
			"sessions":     []any{map[string]any{"id": "a", "prompt": "p"}, map[string]any{"id": "b", "prompt": "p"}, map[string]any{"id": "c", "prompt": "p"}},
			"continuation": map[string]any{"branch": "b", "requirement": "r", "rationale": "w", "unrelated_value": "u"},
			"handoff":      map[string]any{"document": "handoff/composing.md"},
		}), "exactly two sessions"},
		{"handoff without sessions", live(map[string]any{
			"handoff": map[string]any{"document": "handoff/composing.md"},
		}), "declaring sessions"},
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
	writeStub(t, stubDir, "engram", "#!/bin/sh\ncase \"$1\" in\nsearch) echo 'wire /ship continuation retrieval' ;;\ndelete) echo \"$@\" > "+stubDir+"/deleted ;;\n*) exit 1 ;;\nesac\n")
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
	if deleted, _ := readText(filepath.Join(stubDir, "deleted")); strings.TrimSpace(deleted) != "delete project scenario-continuation-live --hard" {
		t.Fatalf("the test project was cleared with %q", deleted)
	}
	for _, want := range []string{"\"outcome\": \"recovered\"", "\"identities\"", "\"revision\":", "\"project_cleared\": true"} {
		if !strings.Contains(recovery, want) {
			t.Fatalf("the retained classification lacks %q:\n%s", want, recovery)
		}
	}
}

// handoffScenario declares the live handoff leg: a worker session composes the handoff into a temporary document, and a fresh parent recovers it after the runner removes that document.
func handoffScenario() map[string]any {
	return testScenario(map[string]any{
		"id": "handoff-recovery-live", "mode": "live", "client": "claude",
		"sessions": []any{
			map[string]any{"id": "worker", "prompt": "compose {{requirement}} and {{rationale}} for {{branch}}"},
			map[string]any{"id": "parent", "prompt": "recover the handoff for {{branch}}"},
		},
		"continuation": map[string]any{
			"branch": "docs/76", "requirement": "wire /ship continuation retrieval",
			"rationale": "the branch scopes recovery", "unrelated_branch": "feat/99",
			"unrelated_value": "a different next action",
		},
		"handoff":   map[string]any{"document": "handoff/composing.md"},
		"artifacts": map[string]any{"contains": []any{map[string]any{"path": "artifacts/recovery.json", "text": "\"accepted\": true"}}},
	})
}

// handoffComposes is the compliant worker: it composes the handoff into the declared temporary document.
const handoffComposes = "mkdir -p handoff\nprintf '%s\\n' 'requirement: wire /ship continuation retrieval rationale: the branch scopes recovery' > handoff/composing.md\n"

// handoffBoth is a record holding both deferred values.
const handoffBoth = "unresolved: wire /ship continuation retrieval; the branch scopes recovery"

// handoffExport is an Engram export of the isolated test project: the branch checkpoint naming the handoff key, beside the given topic key and content pairs.
func handoffExport(pairs ...string) string {
	records := []string{`{"project":"scenario-handoff-recovery-live","topic_key":"checkpoint/docs/76","content":"handoff: handoff/docs/76"}`}
	for index := 0; index+1 < len(pairs); index += 2 {
		records = append(records, `{"project":"scenario-handoff-recovery-live","topic_key":"`+pairs[index]+`","content":"`+pairs[index+1]+`"}`)
	}
	return `{"observations":[` + strings.Join(records, ",") + `]}`
}

// handoffStubs installs a worker session that runs the given shell on its first launch, a parent session that recovers the values on its second, and an Engram store whose export holds the given observations. It returns the stub directory, where the store records a project delete in `deleted` and refuses one while `delete-fails` exists.
func handoffStubs(t *testing.T, worker string, observations ...string) string {
	t.Helper()
	stubDir := t.TempDir()
	writeStub(t, stubDir, "claude", `#!/bin/sh
if [ -f `+stubDir+`/composed ]; then
  mkdir -p recovery
  printf '%s\n' '{"unresolved":["wire /ship continuation retrieval","the branch scopes recovery"]}' > recovery/session-two.json
else
  : > `+stubDir+`/composed
`+worker+`
fi
echo '{"type":"result","model":"stub"}'
`)
	export := handoffExport("handoff/docs/76", handoffBoth)
	if len(observations) > 0 {
		export = observations[0]
	}
	writeStub(t, stubDir, "engram", `#!/bin/sh
case "$1" in
export) printf '%s' '`+export+`' > "$2" ;;
delete) [ -f `+stubDir+`/delete-fails ] && exit 1; [ -f `+stubDir+`/delete-missing ] && { echo "project not found: \"$3\"" >&2; exit 1; }; echo "$@" > `+stubDir+`/deleted ;;
*) exit 1 ;;
esac
`)
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return stubDir
}

// runHandoff runs the live handoff scenario through the stubs and returns its state and retained classification.
func runHandoff(t *testing.T) (string, string) {
	t.Helper()
	state, where, err := runOne(handoffScenario(), testRepoRoot(), t.TempDir(), filepath.Join(testRepoRoot(), "scenarios", "fixtures"), true, nil)
	if err != nil {
		t.Fatal(err)
	}
	return state, retainedRecovery(t, where)
}

func wantRetained(t *testing.T, recovery string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(recovery, want) {
			t.Fatalf("the retained classification lacks %q:\n%s", want, recovery)
		}
	}
}

// TestHandoffRecoveryLiveWithStubSessions proves the fresh parent recovers the deferred requirement and rationale through the branch handoff after the runner removed the composing document and found no residue.
func TestHandoffRecoveryLiveWithStubSessions(t *testing.T) {
	stubDir := handoffStubs(t, handoffComposes)
	state, recovery := runHandoff(t)
	if state != "unreviewed" {
		t.Fatalf("state = %q, want unreviewed:\n%s", state, recovery)
	}
	wantRetained(t, recovery, "\"outcome\": \"recovered\"", "\"key\": \"handoff/docs/76\"", "\"document_removed\": true", "\"residue\": []", "\"project_cleared\": true", "\"fallback_started\": false")
	if deleted, _ := readText(filepath.Join(stubDir, "deleted")); strings.TrimSpace(deleted) != "delete project scenario-handoff-recovery-live --hard" {
		t.Fatalf("the test project was cleared with %q", deleted)
	}
}

// TestHandoffRecoveryFailedClearIsUnverified keeps a run whose disposable project could not be cleared out of acceptance, because an earlier run's records could have answered it.
func TestHandoffRecoveryFailedClearIsUnverified(t *testing.T) {
	stubDir := handoffStubs(t, handoffComposes)
	if err := os.WriteFile(filepath.Join(stubDir, "delete-fails"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	state, recovery := runHandoff(t)
	if state != "incomplete" {
		t.Fatalf("state = %q, want incomplete:\n%s", state, recovery)
	}
	wantRetained(t, recovery, "\"outcome\": \"unverified\"", "\"project_cleared\": false", "could not be cleared")
}

// TestHandoffRecoveryClearsAMissingProject treats a test project that does not exist yet as cleared, because a first run has nothing to clear and must not read as unverified.
func TestHandoffRecoveryClearsAMissingProject(t *testing.T) {
	stubDir := handoffStubs(t, handoffComposes)
	if err := os.WriteFile(filepath.Join(stubDir, "delete-missing"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	state, recovery := runHandoff(t)
	if state != "unreviewed" {
		t.Fatalf("state = %q, want unreviewed:\n%s", state, recovery)
	}
	wantRetained(t, recovery, "\"outcome\": \"recovered\"", "\"project_cleared\": true")
}

// TestClearTestProjectRefusesOtherProjects keeps the hard delete inside the disposable scenario namespace, because deleting a real project cannot be undone.
func TestClearTestProjectRefusesOtherProjects(t *testing.T) {
	stubDir := handoffStubs(t, handoffComposes)
	for _, project := range []string{"harness", "scenario-", "my-scenario-x"} {
		if clearTestProject(project) {
			t.Fatalf("clearTestProject(%q) reported a clear", project)
		}
	}
	if pathExists(filepath.Join(stubDir, "deleted")) {
		t.Fatal("a project outside the scenario namespace reached engram delete")
	}
	if !clearTestProject("scenario-handoff-recovery-live") {
		t.Fatal("the disposable scenario project was not cleared")
	}
}

// TestHandoffRecoveryRejectsValueReadableOnDisk keeps a value the parent could have reread from the workspace out of acceptance, because that recovery proves nothing about the durable handoff.
func TestHandoffRecoveryRejectsValueReadableOnDisk(t *testing.T) {
	handoffStubs(t, handoffComposes+"mkdir -p notes\nprintf '%s\\n' 'wire /ship continuation retrieval' > notes/leftover.md\n")
	state, recovery := runHandoff(t)
	if state != "fail" {
		t.Fatalf("state = %q, want fail:\n%s", state, recovery)
	}
	wantRetained(t, recovery, "\"outcome\": \"source_readable_on_disk\"", "\"accepted\": false", "still readable outside Engram: notes/leftover.md")
}

// TestHandoffRecoveryRejectsValueReadableThroughGit refuses a composing document the worker committed or staged, because removing it from the tree leaves both values recoverable through git.
func TestHandoffRecoveryRejectsValueReadableThroughGit(t *testing.T) {
	for label, record := range map[string]string{
		"git history": "git add handoff/composing.md\ngit -c user.name=stub -c user.email=stub@example.com commit -qm handoff\n",
		"git index":   "git add handoff/composing.md\n",
		// A commit dropped by a reset is reachable only through the reflog; the document stays in the tree, unstaged, so the runner still removes it.
		"reset commit": "git -c user.name=stub -c user.email=stub@example.com commit -q --allow-empty -m base\ngit add handoff/composing.md\ngit -c user.name=stub -c user.email=stub@example.com commit -qm handoff\ngit reset -q --soft HEAD~1\ngit rm -q --cached handoff/composing.md\n",
	} {
		t.Run(label, func(t *testing.T) {
			handoffStubs(t, handoffComposes+record)
			state, recovery := runHandoff(t)
			if state != "fail" {
				t.Fatalf("state = %q, want fail:\n%s", state, recovery)
			}
			want := label
			if label == "reset commit" {
				want = "git history"
			}
			wantRetained(t, recovery, "\"outcome\": \"source_readable_on_disk\"", "\""+want+"\"")
		})
	}
}

// TestHandoffRecoveryWithoutDocumentIsUnverified keeps a run whose worker never composed the document out of acceptance, because recovery after its removal was never exercised.
func TestHandoffRecoveryWithoutDocumentIsUnverified(t *testing.T) {
	handoffStubs(t, ":\n")
	state, recovery := runHandoff(t)
	if state != "incomplete" {
		t.Fatalf("state = %q, want incomplete:\n%s", state, recovery)
	}
	wantRetained(t, recovery, "\"outcome\": \"unverified\"", "\"accepted\": false", "\"document_removed\": false", "the handoff document was not present when the runner removed it")
}

// TestHandoffRecoveryAttribution accepts a recovery only when both values sit under the branch handoff key and no other branch's handoff holds either. Session summaries and other saves may hold them too, because Engram tells every session to save proactively.
func TestHandoffRecoveryAttribution(t *testing.T) {
	cases := []struct {
		name, export, state, outcome string
	}{
		{"session summary beside the branch handoff", handoffExport("handoff/docs/76", handoffBoth, "", handoffBoth, "session-summary/scenario", handoffBoth), "unreviewed", continuationRecovered},
		{"foreign key only", handoffExport("handoff/feat/99", handoffBoth), "fail", continuationMemoryNotIsolated},
		{"foreign handoff holds the rationale", handoffExport("handoff/docs/76", handoffBoth, "handoff/feat/99", "the branch scopes recovery"), "fail", continuationMemoryNotIsolated},
		{"rationale missing from the branch key", handoffExport("handoff/docs/76", "unresolved: wire /ship continuation retrieval", "", handoffBoth), "fail", continuationMemoryNotIsolated},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			handoffStubs(t, handoffComposes, c.export)
			state, recovery := runHandoff(t)
			if state != c.state {
				t.Fatalf("state = %q, want %s:\n%s", state, c.state, recovery)
			}
			wantRetained(t, recovery, "\"outcome\": \""+c.outcome+"\"")
		})
	}
}

// TestHandoffRecoveryWithoutBudgetDoesNotRun keeps the live handoff leg behind the approved budget.
func TestHandoffRecoveryWithoutBudgetDoesNotRun(t *testing.T) {
	out := t.TempDir()
	state, _, err := runOne(handoffScenario(), testRepoRoot(), out, filepath.Join(testRepoRoot(), "scenarios", "fixtures"), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if state != "not-run" {
		t.Fatalf("state = %q, want not-run", state)
	}
	if attempts := attemptPaths(out, "handoff-recovery-live"); len(attempts) != 0 {
		t.Fatalf("a not-run handoff scenario wrote an attempt: %v", attempts)
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
