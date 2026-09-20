package scenarios

import "strings"

// probeRecoveryRun exercises both recovery oracles over compliant, violating, and unverified offline cases. The continuation schema is read from the workflow-checkpoint skill, so a changed protocol changes this probe. The live experiments that supply real facts are declared in scenarios/matrix.recovery.json and stay budget-gated.
func probeRecoveryRun(root string) (map[string]any, error) {
	if _, err := probeCheckpointProtocol(root); err != nil {
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

	return map[string]any{
		"continuation_recovers_requirement":            continuationOracle(recovered).Outcome == continuationRecovered && strings.Contains(recovered.Recovered, recovered.Requirement),
		"continuation_recovers_rationale":              continuationOracle(recovered).Outcome == continuationRecovered && strings.Contains(recovered.Recovered, recovered.Rationale),
		"continuation_not_repeated_in_prompt":          continuationOracle(repeated).Outcome == continuationRepeatedInPrompt,
		"continuation_not_shared_conversation":         continuationOracle(shared).Outcome == continuationSharedConversation,
		"continuation_unrelated_excluded":              continuationOracle(unrelated).Outcome == continuationUnrelatedSupplied,
		"continuation_memory_isolated":                 continuationOracle(leaked).Outcome == continuationMemoryNotIsolated,
		"continuation_missing_is_rejected":             continuationOracle(missingRationale).Outcome == continuationMissingRationale,
		"continuation_missing_requirement_rejected":    continuationOracle(missingRequirement).Outcome == continuationMissingRequirement,
		"continuation_unavailable_is_unverified":       continuationOracle(unavailable).Outcome == continuationUnverified,
		"continuation_store_unavailable_is_unverified": continuationOracle(storeDown).Outcome == continuationUnverified,
		"headless_proven_stop":                         workerVerdictAccepted(provenStop) && workerRecoveryOracle(provenStop).Outcome == workerProvenStop,
		"post_stop_edit_rejected":                      !workerAccepted(workerRecoveryOracle(postStop).Outcome) && workerRecoveryOracle(postStop).Outcome == workerPostStopEdit,
		"uncertain_no_fallback":                        workerVerdictAccepted(uncertain) && workerRecoveryOracle(uncertain).Outcome == workerUncertainNoFallback,
		"uncertain_fallback_rejected":                  !workerAccepted(workerRecoveryOracle(uncertainFallback).Outcome) && workerRecoveryOracle(uncertainFallback).Outcome == workerUncertainFallback,
		"worker_identities_required":                   workerRecoveryOracle(anonymous).Outcome == workerUnverified,
		"worker_unavailable_is_unverified":             workerRecoveryOracle(workerUnavailable).Outcome == workerUnverified,
	}, nil
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
