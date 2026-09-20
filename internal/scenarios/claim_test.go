package scenarios

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func read(c, state string) claimEvent {
	return claimReadEvent(c, state)
}

func write(c string) claimEvent {
	return claimWriteEvent(c)
}

func implement(c string) claimEvent {
	return claimImplementEvent(c)
}

// TestClaimOracleSequences holds the ownership expectation against a compliant sequence and every way it breaks.
func TestClaimOracleSequences(t *testing.T) {
	contenders := []string{"a", "b"}
	cases := []struct {
		name         string
		contenders   []string
		synchronized bool
		events       []claimEvent
		want         string
		wantWinners  []string
	}{
		{
			name: "synchronized single winner", contenders: contenders, synchronized: true, want: claimSingleWinner, wantWinners: []string{"a"},
			events: []claimEvent{read("a", "ready"), read("b", "ready"), write("a"), write("b"), implement("a")},
		},
		{
			name: "serialized single winner", contenders: contenders, want: claimSingleWinner, wantWinners: []string{"a"},
			events: []claimEvent{read("a", "ready"), write("a"), implement("a"), read("b", "claimed")},
		},
		{
			name: "duplicate winners", contenders: contenders, synchronized: true, want: claimDuplicateWinners, wantWinners: []string{"a", "b"},
			events: []claimEvent{read("a", "ready"), read("b", "ready"), write("a"), write("b"), implement("a"), implement("b")},
		},
		{
			name: "loser interference", contenders: contenders, synchronized: true, want: claimLoserInterference,
			events: []claimEvent{read("a", "ready"), read("b", "ready"), write("a"), implement("a"), write("b"), {Contender: "b", Kind: claimInterfere, Workspace: "work/b", Detail: "work/a"}},
		},
		{
			name: "unsynchronized reads fail the barrier", contenders: contenders, synchronized: true, want: claimBarrierFailed,
			events: []claimEvent{read("a", "ready"), write("a"), implement("a"), read("b", "ready")},
		},
		{
			name: "barrier timeout is a failed barrier", contenders: contenders, synchronized: true, want: claimBarrierFailed,
			events: []claimEvent{read("a", "ready"), {Contender: "a", Kind: claimBarrier, Detail: "timeout"}, read("b", "ready")},
		},
		{
			name: "absent contender", contenders: contenders, want: claimAbsentContender,
			events: []claimEvent{read("a", "ready"), write("a"), implement("a")},
		},
		{
			name: "unavailable client", contenders: contenders, want: claimUnavailableClient,
			events: []claimEvent{read("a", "ready"), write("a"), {Contender: "b", Kind: claimUnavailable, Detail: "model_not_found"}},
		},
		{
			name: "missing prototype confirmation", contenders: contenders, synchronized: true, want: claimConfirmationMissing,
			events: []claimEvent{read("a", "ready"), read("b", "ready"), write("a"), implement("a"), {Contender: "b", Kind: claimAbort, Detail: "no_confirmation"}},
		},
		{
			name: "no implementation is unverified", contenders: contenders, synchronized: true, want: claimNoImplementation,
			events: []claimEvent{read("a", "ready"), read("b", "ready"), write("a"), write("b")},
		},
		{
			name: "fewer than two contenders", contenders: []string{"a"}, want: claimAbsentContender,
			events: []claimEvent{read("a", "ready"), implement("a")},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			verdict := claimOracle(c.contenders, c.events, c.synchronized)
			if verdict.Outcome != c.want {
				t.Fatalf("outcome = %q, want %q (%v)", verdict.Outcome, c.want, verdict.Reasons)
			}
			if len(c.wantWinners) == 0 {
				if len(verdict.Winners) != 0 {
					t.Fatalf("winners = %v, want none", verdict.Winners)
				}
				return
			}
			if !equalStrings(verdict.Winners, c.wantWinners) {
				t.Fatalf("winners = %v, want %v", verdict.Winners, c.wantWinners)
			}
		})
	}
}

// TestClaimExperiment runs the concurrent contenders against the shared disposable tracker: a serialized launch leaves one winner, a synchronized launch lets both observe ready and duplicates, and an unconfirmed prototype run is unverified rather than an exclusion.
func TestClaimExperiment(t *testing.T) {
	contenders := []claimContender{
		{ID: "ship-a", Workspace: "a", Driver: claimDriverScript},
		{ID: "ship-b", Workspace: "b", Driver: claimDriverScript},
	}
	cases := []struct {
		name                  string
		synchronized          bool
		requiresConf, confirm bool
		want                  string
		wantWinners           []string
	}{
		{name: "serialized experiment", want: claimSingleWinner, wantWinners: []string{"ship-a"}},
		{name: "synchronized experiment duplicates", synchronized: true, want: claimDuplicateWinners, wantWinners: []string{"ship-a", "ship-b"}},
		{name: "proto without confirmation is unverified", synchronized: true, requiresConf: true, want: claimConfirmationMissing},
		{name: "proto with confirmation is judged", synchronized: true, requiresConf: true, confirm: true, want: claimDuplicateWinners, wantWinners: []string{"ship-a", "ship-b"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			setup, err := prepareClaim(t.TempDir(), contenders, "ready-for-agent", "in-progress", c.synchronized, 30)
			if err != nil {
				t.Fatal(err)
			}
			events, verdict := runClaim(setup, c.synchronized, c.requiresConf, c.confirm, driverExecutor())
			if verdict.Outcome != c.want {
				t.Fatalf("outcome = %q, want %q (%v); events %v", verdict.Outcome, c.want, verdict.Reasons, events)
			}
			if len(c.wantWinners) > 0 && !equalStrings(verdict.Winners, c.wantWinners) {
				t.Fatalf("winners = %v, want %v", verdict.Winners, c.wantWinners)
			}
			if len(events) == 0 {
				t.Fatal("the experiment recorded no ordered events")
			}
		})
	}
}

// TestClaimExperimentUsesIndependentWorkspaces proves each contender acts in its own workspace and that a synchronized duplicate is only claimed after both initial reads.
func TestClaimExperimentUsesIndependentWorkspaces(t *testing.T) {
	contenders := []claimContender{
		{ID: "ship-a", Workspace: "a", Driver: claimDriverScript},
		{ID: "ship-b", Workspace: "b", Driver: claimDriverScript},
	}
	setup, err := prepareClaim(t.TempDir(), contenders, "ready-for-agent", "in-progress", true, 30)
	if err != nil {
		t.Fatal(err)
	}
	if setup.Repo["ship-a"] == setup.Repo["ship-b"] {
		t.Fatal("contenders share one workspace")
	}
	events, verdict := runClaim(setup, true, false, false, driverExecutor())
	if verdict.Outcome != claimDuplicateWinners {
		t.Fatalf("outcome = %q, want duplicate", verdict.Outcome)
	}
	firstWrite := len(events)
	for index, event := range events {
		if event.Kind == claimWrite {
			firstWrite = index
			break
		}
	}
	if firstWrite == len(events) {
		t.Fatalf("no claim write was recorded: %v", events)
	}
	for index, event := range events {
		if event.Kind == claimRead && index > firstWrite {
			t.Fatalf("a read followed the first write: %v", events)
		}
	}
}

// TestClaimVerdictPassOnlyForASingleWinner keeps "rejected" and "incomplete" separate from a pass.
func TestClaimVerdictPassOnlyForASingleWinner(t *testing.T) {
	if !claimAccepted(claimSingleWinner) {
		t.Fatal("a single winner was not accepted")
	}
	for _, outcome := range []string{claimDuplicateWinners, claimLoserInterference, claimBarrierFailed, claimAbsentContender, claimUnavailableClient, claimConfirmationMissing, claimNoImplementation} {
		if claimAccepted(outcome) {
			t.Fatalf("%s was accepted as a pass", outcome)
		}
	}
}

var claimOutcomeKeys = []string{
	"serialized_single_winner", "synchronized_duplicate_exposed", "independent_workspaces",
	"reads_preceded_writes", "proto_unconfirmed_unverified", "proto_confirmed_judged",
	"interference_rejected", "barrier_failure_incomplete", "absent_contender_incomplete",
	"unavailable_client_incomplete", "reads_observed_ready", "serialized_loser_abstained",
	"proto_abort_recorded", "proto_reread_observed", "ship_read_before_write",
	"ship_aborts_when_claimed", "proto_rereads_before_write", "proto_requires_confirmation",
}

func TestConcurrentClaimProbe(t *testing.T) {
	outcomes, events, err := probeClaimRun(testRepoRoot(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	haveKeys(t, outcomes, claimOutcomeKeys)
	allTrue(t, outcomes)
	if !contains(events, "\"kind\":\"read\"") || !contains(events, "\"kind\":\"write\"") || !contains(events, "\"kind\":\"implement\"") {
		t.Fatalf("the retained ordered events are incomplete:\n%s", events)
	}
}

// TestClaimProbeReadsRulesFromTheCommands repoints each owning document and expects the probe's behavior to follow it.
func TestClaimProbeReadsRulesFromTheCommands(t *testing.T) {
	savedShip, savedProto := claimShipPath, claimProtoPath
	t.Cleanup(func() { claimShipPath, claimProtoPath = savedShip, savedProto })

	claimShipPath = writeTemp(t, patch(t, filepath.Join("commands", "ship.md"),
		`"abort_when_claimed": true`, `"abort_when_claimed": false`))
	ship, err := probeClaimBlock(testRepoRoot(), claimShipPath)
	if err != nil {
		t.Fatal(err)
	}
	if truthy(ship["abort_when_claimed"]) {
		t.Fatal("the probe did not read the repointed abort_when_claimed rule")
	}

	claimProtoPath = writeTemp(t, patch(t, filepath.Join("commands", "proto.md"),
		`"requires_confirmation": true`, `"requires_confirmation": false`))
	proto, err := probeClaimBlock(testRepoRoot(), claimProtoPath)
	if err != nil {
		t.Fatal(err)
	}
	if truthy(proto["requires_confirmation"]) {
		t.Fatal("the probe did not read the repointed requires_confirmation rule")
	}

	claimShipPath, claimProtoPath = savedShip, savedProto
	if _, err := probeClaimBlock(testRepoRoot(), filepath.Join("commands", "ship.md")); err != nil {
		t.Fatal(err)
	}
}

func TestClaimLabelsComeFromTheLifecycleMapping(t *testing.T) {
	ready, claimed, err := probeClaimLabels(testRepoRoot())
	if err != nil {
		t.Fatal(err)
	}
	if ready != "ready-for-agent" || claimed != "in-progress" {
		t.Fatalf("labels = %q, %q", ready, claimed)
	}
}

func TestClaimReadsPrecedeWrites(t *testing.T) {
	contenders := []claimContender{{ID: "a"}, {ID: "b"}}
	if !claimReadsPrecedeWrites(contenders, []claimEvent{claimReadEvent("a", "r"), claimReadEvent("b", "r"), claimWriteEvent("a")}) {
		t.Fatal("reads before the first write were not accepted")
	}
	if claimReadsPrecedeWrites(contenders, []claimEvent{claimReadEvent("a", "r"), claimWriteEvent("a"), claimReadEvent("b", "r")}) {
		t.Fatal("a read after the first write was accepted")
	}
}

func TestConcurrentDeclarationValidation(t *testing.T) {
	valid := map[string]any{"scenarios": []any{testScenario(map[string]any{
		"id": "c", "mode": "live", "concurrent": map[string]any{"contenders": []any{
			map[string]any{"id": "a", "prompt": "p"}, map[string]any{"id": "b", "prompt": "p"},
		}},
	})}}
	if errors := validateMatrix(valid); len(errors) != 0 {
		t.Fatalf("a valid concurrent declaration was rejected: %v", errors)
	}
	tooFew := map[string]any{"scenarios": []any{testScenario(map[string]any{
		"id": "c", "mode": "live", "concurrent": map[string]any{"contenders": []any{map[string]any{"id": "a", "prompt": "p"}}},
	})}}
	if !contains(joinErrors(validateMatrix(tooFew)), "at least two contenders") {
		t.Fatalf("one contender was not rejected: %v", validateMatrix(tooFew))
	}
	duplicate := map[string]any{"scenarios": []any{testScenario(map[string]any{
		"id": "c", "mode": "live", "concurrent": map[string]any{"contenders": []any{
			map[string]any{"id": "a", "prompt": "p"}, map[string]any{"id": "a", "prompt": "p"},
		}},
	})}}
	if !contains(joinErrors(validateMatrix(duplicate)), "duplicate contender id") {
		t.Fatalf("duplicate contender ids were not rejected: %v", validateMatrix(duplicate))
	}
	noPrompt := map[string]any{"scenarios": []any{testScenario(map[string]any{
		"id": "c", "mode": "live", "concurrent": map[string]any{"contenders": []any{
			map[string]any{"id": "a"}, map[string]any{"id": "b", "prompt": "p"},
		}},
	})}}
	if !contains(joinErrors(validateMatrix(noPrompt)), "needs a prompt") {
		t.Fatalf("a live contender without a prompt was not rejected: %v", validateMatrix(noPrompt))
	}
	badBarrier := map[string]any{"scenarios": []any{testScenario(map[string]any{
		"id": "c", "mode": "live", "concurrent": map[string]any{"barrier_seconds": 0, "contenders": []any{
			map[string]any{"id": "a", "prompt": "p"}, map[string]any{"id": "b", "prompt": "p"},
		}},
	})}}
	if !contains(joinErrors(validateMatrix(badBarrier)), "barrier_seconds") {
		t.Fatalf("a zero barrier_seconds was not rejected: %v", validateMatrix(badBarrier))
	}
}

// TestConcurrentWithoutBudgetDoesNotRun keeps the live path honest offline: no attempt is written without --allow-paid.
func TestConcurrentWithoutBudgetDoesNotRun(t *testing.T) {
	scenario := testScenario(map[string]any{
		"id": "claim-race", "mode": "live",
		"concurrent": map[string]any{"synchronized": true, "contenders": []any{
			map[string]any{"id": "a", "prompt": "p"}, map[string]any{"id": "b", "prompt": "p"},
		}},
	})
	out := t.TempDir()
	state, _, err := runOne(scenario, testRepoRoot(), out, filepath.Join(testRepoRoot(), "scenarios", "fixtures"), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if state != "not-run" {
		t.Fatalf("state = %q, want not-run", state)
	}
	if attempts := attemptPaths(out, "claim-race"); len(attempts) != 0 {
		t.Fatalf("a not-run concurrent scenario wrote an attempt: %v", attempts)
	}
}

func joinErrors(errors []string) string {
	out := ""
	for _, err := range errors {
		out += err + "\n"
	}
	return out
}

// TestClaimsMatrixIsTheBudgetedLiveHome keeps the model-session experiment declared and budget-gated, while the default matrix runs the offline oracle.
func TestClaimsMatrixIsTheBudgetedLiveHome(t *testing.T) {
	claims := loadTestMatrixAt(t, filepath.Join(testRepoRoot(), "scenarios", "matrix.claims.json"))
	if errors := validateMatrix(claims); len(errors) != 0 {
		t.Fatalf("claims matrix errors: %v", errors)
	}
	declared := list(claims, "scenarios")
	if len(declared) < 2 {
		t.Fatalf("claims matrix declares %d scenarios, want at least two", len(declared))
	}
	for _, raw := range declared {
		scenario, _ := raw.(map[string]any)
		declaration, ok := scenario["concurrent"].(map[string]any)
		if !ok {
			t.Fatalf("%s declares no concurrent block", getString(scenario, "id"))
		}
		if mode(scenario) != "live" {
			t.Fatalf("%s is not a live scenario", getString(scenario, "id"))
		}
		if len(concurrentContenders(declaration)) < 2 {
			t.Fatalf("%s declares fewer than two contenders", getString(scenario, "id"))
		}
	}

	offline := findScenario(loadTestMatrix(t), "concurrent-claim")
	if offline == nil || mode(offline) != "offline" || !truthy(offline["check"]) {
		t.Fatal("the default matrix does not run the concurrent-claim oracle offline")
	}
}

func TestConcurrentStatus(t *testing.T) {
	cases := []struct {
		name     string
		statuses map[string]string
		want     string
	}{
		{"both ok", map[string]string{"a": "ok", "b": "ok"}, "ok"},
		{"one failed", map[string]string{"a": "ok", "b": "failed"}, "failed"},
		{"interrupted wins", map[string]string{"a": "ok", "b": "interrupted"}, "interrupted"},
		{"all model-unavailable", map[string]string{"a": "model-unavailable", "b": "model-unavailable"}, "model-unavailable"},
		{"mixed unavailable and ok", map[string]string{"a": "ok", "b": "model-unavailable"}, "ok"},
		{"no sessions", map[string]string{}, "unknown"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := concurrentStatus(c.statuses); got != c.want {
				t.Fatalf("concurrentStatus = %q, want %q", got, c.want)
			}
		})
	}
}

func TestClaimIndependentWorkspaces(t *testing.T) {
	contenders := []claimContender{{ID: "a"}, {ID: "b"}}
	if !claimIndependentWorkspaces(contenders, []claimEvent{claimReadEvent("a", "r"), claimReadEvent("b", "r")}) {
		t.Fatal("distinct workspaces were not proven independent")
	}
	shared := []claimEvent{{Contender: "a", Workspace: "x", Kind: claimRead}, {Contender: "b", Workspace: "x", Kind: claimRead}}
	if claimIndependentWorkspaces(contenders, shared) {
		t.Fatal("a shared workspace was accepted as independent")
	}
	if claimIndependentWorkspaces(contenders, []claimEvent{claimReadEvent("a", "r")}) {
		t.Fatal("a contender without a workspace was accepted")
	}
}

// TestConcurrentAttemptWithStubSession exercises the budgeted path end to end with a stub client: the synchronized race duplicates, so the ownership expectation fails visibly, and the attempt retains two sessions' evidence tagged by contender.
func TestConcurrentAttemptWithStubSession(t *testing.T) {
	stubDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stubDir, "claude"), []byte(claimDriverScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	scenario := testScenario(map[string]any{
		"id": "claim-race", "mode": "live",
		"concurrent": map[string]any{"synchronized": true, "contenders": []any{
			map[string]any{"id": "a", "workspace": "a", "client": "claude", "prompt": "claim a"},
			map[string]any{"id": "b", "workspace": "b", "client": "claude", "prompt": "claim b"},
		}},
		"artifacts": map[string]any{"contains": []any{map[string]any{"path": "artifacts/claim.json", "text": "\"single_winner\": true"}}},
	})
	out := t.TempDir()
	state, where, err := runOne(scenario, testRepoRoot(), out, filepath.Join(testRepoRoot(), "scenarios", "fixtures"), true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if state != "fail" {
		t.Fatalf("state = %q at %s, want fail because a synchronized race duplicates", state, where)
	}
	provenance, err := readObject(filepath.Join(where, "provenance.json"))
	if err != nil {
		t.Fatal(err)
	}
	if intOf(provenance["sessions"]) != 2 {
		t.Fatalf("provenance sessions = %v, want 2", provenance["sessions"])
	}
	events, err := readText(filepath.Join(where, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"\"type\":\"claim-implement\"", "\"contender\":\"a\"", "\"contender\":\"b\""} {
		if !contains(events, want) {
			t.Fatalf("retained events are missing %q:\n%s", want, events)
		}
	}
}

// TestClaimExperimentRecordsRivalInterference proves a loser that removes the winner's workspace is recorded as interference, not scored as a single winner.
func TestClaimExperimentRecordsRivalInterference(t *testing.T) {
	contenders := []claimContender{
		{ID: "ship-a", Workspace: "a", Driver: claimDriverScript},
		{ID: "ship-b", Workspace: "b", Driver: claimDriverScript},
	}
	setup, err := prepareClaim(t.TempDir(), contenders, "ready-for-agent", "in-progress", false, 30)
	if err != nil {
		t.Fatal(err)
	}
	run := func(c claimContender, repo string, env []string) error {
		cmd := exec.Command("/bin/sh", filepath.Join(repo, "driver.sh"))
		cmd.Dir, cmd.Env = repo, env
		if err := cmd.Run(); err != nil {
			return err
		}
		if c.ID == "ship-b" {
			return os.Remove(filepath.Join(setup.Repo["ship-a"], "impl", "ship-a.txt"))
		}
		return nil
	}
	events, verdict := runClaim(setup, false, false, false, run)
	if verdict.Outcome != claimLoserInterference {
		t.Fatalf("outcome = %q, want loser_interference; events %v", verdict.Outcome, events)
	}
}

func TestClaimReadsObservedReady(t *testing.T) {
	contenders := []claimContender{{ID: "a"}, {ID: "b"}}
	if !claimReadsObservedReady(contenders, []claimEvent{claimReadEvent("a", "ready"), claimReadEvent("b", "ready"), claimWriteEvent("a")}, "ready") {
		t.Fatal("two ready observations were not accepted")
	}
	if claimReadsObservedReady(contenders, []claimEvent{claimReadEvent("a", "ready"), claimReadEvent("b", "claimed")}, "ready") {
		t.Fatal("a read that missed the ready state was accepted")
	}
	if claimReadsObservedReady(contenders, []claimEvent{claimReadEvent("a", "ready")}, "ready") {
		t.Fatal("a contender that never read was accepted")
	}
}

// TestUnjudgeableRaceIsIncomplete keeps a failed barrier, absent contender, unavailable client, or missing confirmation out of the fail state.
func TestUnjudgeableRaceIsIncomplete(t *testing.T) {
	declaration := testScenario(nil)
	attempt := filepath.Join(t.TempDir(), "attempt")
	writeAttempt(t, attempt, "abc", false, false, nil)
	writeJSON(t, filepath.Join(attempt, "artifacts.json"), map[string]any{
		"ok": false, "results": []any{}, "incomplete_reason": "barrier_failed: the read barrier did not complete",
	})
	if got, reasons := evaluate(attempt, declaration, "abc"); got != "incomplete" {
		t.Fatalf("evaluate = %q (%v), want incomplete", got, reasons)
	}
}

func TestClaimJudgeable(t *testing.T) {
	for _, outcome := range []string{claimSingleWinner, claimDuplicateWinners, claimLoserInterference} {
		if !claimJudgeable(outcome) {
			t.Fatalf("%s is judgeable", outcome)
		}
	}
	for _, outcome := range []string{claimBarrierFailed, claimAbsentContender, claimUnavailableClient, claimConfirmationMissing, claimNoImplementation} {
		if claimJudgeable(outcome) {
			t.Fatalf("%s must not be judgeable", outcome)
		}
	}
}

// TestWriteClaimAttemptRecordsIncomplete covers the run path that decides an unjudgeable race is incomplete, not just evaluate's reading of the key.
func TestWriteClaimAttemptRecordsIncomplete(t *testing.T) {
	attempt := filepath.Join(t.TempDir(), "attempt")
	contenders := []claimContender{{ID: "a", Prompt: "p"}, {ID: "b", Prompt: "p"}}
	events := []claimEvent{claimReadEvent("a", "r"), claimReadEvent("b", "r")}
	if err := writeClaimFile(attempt, "provenance.json", indentJSON(map[string]any{"input_digest": "abc", "recorded_at": "now"})); err != nil {
		t.Fatal(err)
	}
	if err := writeClaimAttempt(attempt, contenders, map[string][]any{}, map[string][]any{}, map[string]string{"a": "ok", "b": "ok"}, events, nil, "barrier_failed: the read barrier did not complete", nil, testScenario(map[string]any{"id": "c"}), 1.0); err != nil {
		t.Fatal(err)
	}
	artifacts, err := readObject(filepath.Join(attempt, "artifacts.json"))
	if err != nil {
		t.Fatal(err)
	}
	if getString(artifacts, "incomplete_reason") == "" {
		t.Fatalf("the attempt records no incomplete reason: %v", artifacts)
	}
	if got, reasons := evaluate(attempt, testScenario(nil), "abc"); got != "incomplete" {
		t.Fatalf("evaluate = %q (%v), want incomplete", got, reasons)
	}
}

// TestClaimShimRefusesAndLogsPublishing proves the instrumented tracker refuses a real-forge write and records it, so the attempt's no-external-forge check is not vacuous.
func TestClaimShimRefusesAndLogsPublishing(t *testing.T) {
	contenders := []claimContender{{ID: "a", Workspace: "a"}, {ID: "b", Workspace: "b"}}
	setup, err := prepareClaim(t.TempDir(), contenders, "ready-for-agent", "in-progress", false, 30)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(filepath.Join(filepath.Dir(setup.Repo["a"]), "shim", "gh"), "pr", "create", "--title", "t")
	cmd.Env = setup.Env["a"]
	if err := cmd.Run(); err == nil {
		t.Fatal("the claim shim did not refuse a publishing call")
	}
	calls := readForgeLog(setup.Shared)
	if len(calls) != 1 || !contains(calls[0], "pr create") {
		t.Fatalf("forge log = %v", calls)
	}
}
