package scenarios

import (
	"errors"
	"strings"
)

// probeRecoveryRun exercises both recovery oracles over compliant, violating, and unverified offline cases. The continuation schema is read from the workflow-checkpoint skill, so a changed protocol changes this probe. The live experiments that supply real facts are declared in scenarios/matrix.recovery.json and stay budget-gated.
func probeRecoveryRun(root string) (map[string]any, error) {
	protocol, err := probeCheckpointProtocol(root)
	if err != nil {
		return nil, err
	}

	recovered := continuationFacts{
		Prompt:         "recover the branch checkpoint",
		Recovered:      `{"next_action":"wire /ship continuation retrieval","rationale":"the branch scopes recovery across sessions"}`,
		Requirement:    "wire /ship continuation retrieval",
		Rationale:      "the branch scopes recovery across sessions",
		Unrelated:      []string{"a different next action"},
		MemoryReadable: true,
		MemoryIsolated: true,
		RouteAvailable: true,
		ProjectCleared: true,
	}
	repeated := recovered
	repeated.Prompt = "recover wire /ship continuation retrieval from the branch checkpoint"
	shared := recovered
	shared.SharedTranscript = true
	unrelated := recovered
	unrelated.Recovered = recovered.Recovered + " a different next action"
	leaked := recovered
	leaked.MemoryIsolated = false
	unavailable := recovered
	unavailable.RouteAvailable = false
	storeDown := recovered
	storeDown.MemoryReadable = false
	missingRationale := recovered
	missingRationale.Recovered = `{"next_action":"wire /ship continuation retrieval"}`
	missingRequirement := recovered
	missingRequirement.Recovered = `{"rationale":"the branch scopes recovery across sessions"}`

	provenStop := workerFacts{
		Identities: true, Available: true, ProvenStopped: true,
	}
	postStop := provenStop
	postStop.PostStopEdits = []string{"impl/late.txt"}
	uncertain := workerFacts{
		Identities: true, Available: true, Surviving: []string{"pid 4242"},
	}
	uncertainFallback := uncertain
	uncertainFallback.FallbackStarted = true
	anonymous := provenStop
	anonymous.Identities = false
	workerUnavailable := provenStop
	workerUnavailable.Available = false

	// The handoff leg: a fresh parent recovers the worker's deferred requirement and rationale after the composing document is gone.
	handoffRecovered := recovered
	handoffRecovered.HandoffDeclared = true
	handoffRecovered.ProjectCleared = true
	handoffRecovered.DocumentRemoved = true
	handoffRecovered.Recovered = `{"unresolved":["wire /ship continuation retrieval","the branch scopes recovery across sessions"]}`
	neverComposed := handoffRecovered
	neverComposed.DocumentRemoved = false
	onDisk := handoffRecovered
	onDisk.Residue = []string{"handoff/composing.md"}
	foreignHandoff := handoffRecovered
	foreignHandoff.Recovered = `{"unresolved":["a different next action"]}`
	handoffUnavailable := handoffRecovered
	handoffUnavailable.RouteAvailable = false

	invalidated, err := handoffChecksInvalidated(protocol)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"continuation_recovers_requirement":              continuationOracle(recovered).Outcome == continuationRecovered && strings.Contains(recovered.Recovered, recovered.Requirement),
		"continuation_recovers_rationale":                continuationOracle(recovered).Outcome == continuationRecovered && strings.Contains(recovered.Recovered, recovered.Rationale),
		"continuation_not_repeated_in_prompt":            continuationOracle(repeated).Outcome == continuationRepeatedInPrompt,
		"continuation_not_shared_conversation":           continuationOracle(shared).Outcome == continuationSharedConversation,
		"continuation_unrelated_excluded":                continuationOracle(unrelated).Outcome == continuationUnrelatedSupplied,
		"continuation_memory_isolated":                   continuationOracle(leaked).Outcome == continuationMemoryNotIsolated,
		"continuation_missing_is_rejected":               continuationOracle(missingRationale).Outcome == continuationMissingRationale,
		"continuation_missing_requirement_rejected":      continuationOracle(missingRequirement).Outcome == continuationMissingRequirement,
		"continuation_unavailable_is_unverified":         continuationOracle(unavailable).Outcome == continuationUnverified,
		"continuation_store_unavailable_is_unverified":   continuationOracle(storeDown).Outcome == continuationUnverified,
		"headless_proven_stop":                           workerVerdictAccepted(provenStop) && workerRecoveryOracle(provenStop).Outcome == workerProvenStop,
		"post_stop_edit_rejected":                        !workerAccepted(workerRecoveryOracle(postStop).Outcome) && workerRecoveryOracle(postStop).Outcome == workerPostStopEdit,
		"uncertain_no_fallback":                          workerVerdictAccepted(uncertain) && workerRecoveryOracle(uncertain).Outcome == workerUncertainNoFallback,
		"uncertain_fallback_rejected":                    !workerAccepted(workerRecoveryOracle(uncertainFallback).Outcome) && workerRecoveryOracle(uncertainFallback).Outcome == workerUncertainFallback,
		"worker_identities_required":                     workerRecoveryOracle(anonymous).Outcome == workerUnverified,
		"worker_unavailable_is_unverified":               workerRecoveryOracle(workerUnavailable).Outcome == workerUnverified,
		"handoff_recovered_after_document_removed":       continuationOracle(handoffRecovered).Outcome == continuationRecovered && continuationOracle(neverComposed).Outcome == continuationUnverified,
		"handoff_source_on_disk_rejected":                continuationOracle(onDisk).Outcome == continuationSourceOnDisk && !continuationAccepted(continuationOracle(onDisk).Outcome),
		"handoff_foreign_branch_excluded":                continuationOracle(foreignHandoff).Outcome == continuationUnrelatedSupplied,
		"handoff_checks_invalidated_on_changed_revision": invalidated,
		"handoff_unavailable_is_unverified":              continuationOracle(handoffUnavailable).Outcome == continuationUnverified,
	}, nil
}

// handoffChecksInvalidated drives the stored handoff recovery once per trigger, each over its own record, so every trigger is proven on its own: a moved tip and a listed pending file each invalidate the checks with a note naming it, and a control with neither keeps its checks.
func handoffChecksInvalidated(protocol map[string]any) (bool, error) {
	branch := "test/76-worker-handoff-recovery"
	tip := "abc1234"
	contract := []string{"https://github.com/jvillegasd/harness/issues/76"}
	resolve := func(ref string) bool { return strings.HasPrefix(ref, "https://") }
	recoverWith := func(pending []string, currentTip string) (map[string]any, []string, error) {
		record, err := handoffCompose(handoffSchema(protocol), "the delegated slice", tip,
			[]string{"internal/scenarios/recovery.go"}, pending,
			[]string{"go test ./...: pass"},
			[]string{"wire /ship continuation retrieval", "the branch scopes recovery across sessions"},
			[]string{"artifacts/recovery.json"})
		if err != nil {
			return nil, nil, err
		}
		store := newHandoffStore()
		if _, err := checkpointSave(protocol, store.records, branch, tip, getString(protocol, "active_state"), contract, []string{"go test ./...: pass"}, "", handoffKey(protocol, branch), "human review"); err != nil {
			return nil, nil, err
		}
		if _, incomplete := persistHandoff(store, protocol, branch, record); incomplete {
			return nil, nil, errors.New("the handoff did not persist")
		}
		recovered, notes := recoverHandoff(protocol, store, branch, currentTip, resolve)
		return recovered, notes, nil
	}
	invalidatedBy := func(recovered map[string]any, notes []string, note string) bool {
		return recovered != nil && len(asList(recovered["checks"])) == 0 && equalStrings(notes, []string{note})
	}
	moved, movedNotes, err := recoverWith(nil, "def5678")
	if err != nil {
		return false, err
	}
	pending, pendingNotes, err := recoverWith([]string{"internal/scenarios/recovery.go"}, tip)
	if err != nil {
		return false, err
	}
	control, controlNotes, err := recoverWith(nil, tip)
	if err != nil {
		return false, err
	}
	return invalidatedBy(moved, movedNotes, "the revision moved; checks invalidated") &&
		invalidatedBy(pending, pendingNotes, "the handoff lists pending files; checks invalidated") &&
		control != nil && len(asList(control["checks"])) == 1 && len(controlNotes) == 0, nil
}

// workerVerdictAccepted reports whether an observation classifies as the conservative accepted result.
func workerVerdictAccepted(f workerFacts) bool {
	return workerAccepted(workerRecoveryOracle(f).Outcome)
}

// probeRecovery writes the classification artifact a declared scenario asserts against.
func probeRecovery(repo, root string) error {
	outcomes, err := probeRecoveryRun(root)
	if err != nil {
		return err
	}
	return writeArtifact(repo, "recovery.json", outcomes)
}
