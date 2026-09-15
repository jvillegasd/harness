package scenarios

import (
	"encoding/json"
	"strings"
)

const (
	twoSessionBranch      = "docs/77-engram-workflow-checkpoints"
	twoSessionOther       = "feat/99-unrelated-work"
	twoSessionRequirement = "wire /ship continuation retrieval at its worktree step"
	twoSessionRationale   = "the branch, not the ticket, scopes recovery across sessions"
	twoSessionTicket      = "https://github.com/jvillegasd/dotfiles/issues/77"
)

func cloneStore(store map[string]string) map[string]string {
	raw, _ := json.Marshal(store)
	decoded, _ := decodeJSON(raw)
	object, _ := decoded.(map[string]any)
	out := map[string]string{}
	for key, value := range object {
		out[key] = value.(string)
	}
	return out
}

// probeCheckpointTwoSessionRun recovers a branch checkpoint in a later session through its topic key, not recent memory.
func probeCheckpointTwoSessionRun(root string) (map[string]any, error) {
	protocol, err := probeCheckpointProtocol(root)
	if err != nil {
		return nil, err
	}
	resolves := func(ref string) bool { return strings.HasPrefix(ref, "https://") }
	store := map[string]string{}

	// Session one: record the deferred requirement with the rationale that is not yet durable anywhere else, then persist the store across the reset.
	if _, err := checkpointSave(protocol, store, twoSessionBranch, "abc1234", "active", []string{twoSessionTicket}, []string{"tests: pass"}, twoSessionRationale, twoSessionRequirement); err != nil {
		return nil, err
	}
	persisted := cloneStore(store)

	// Session two: retrieve by the branch topic key, never from recent memory.
	record, _ := checkpointRecover(protocol, persisted, twoSessionBranch, "abc1234", resolves)
	recoversRequirement := record != nil && getString(record, "next_action") == twoSessionRequirement
	recoversRationale := record != nil && getString(record, "rationale") == twoSessionRationale
	recoversContract := record != nil && equalStrings(stringList(record["contract"]), []string{twoSessionTicket})

	// Two branches hold separate checkpoints; neither reads the other's.
	if _, err := checkpointSave(protocol, persisted, twoSessionOther, "abc1234", "active", []string{twoSessionTicket}, nil, "", "a different next action"); err != nil {
		return nil, err
	}
	mine, _ := checkpointRecover(protocol, persisted, twoSessionBranch, "abc1234", resolves)
	theirs, _ := checkpointRecover(protocol, persisted, twoSessionOther, "abc1234", resolves)
	isolation := mine != nil && theirs != nil &&
		getString(mine, "next_action") == twoSessionRequirement &&
		getString(theirs, "next_action") == "a different next action" &&
		getString(mine, "next_action") != getString(theirs, "next_action")

	// The topic key is stable: a later save upserts the one branch record.
	if _, err := checkpointSave(protocol, store, twoSessionBranch, "def5678", "active", []string{twoSessionTicket}, []string{"lint: pass"}, twoSessionRationale, "next"); err != nil {
		return nil, err
	}
	saved, _ := checkpointLoad(protocol, store, twoSessionBranch)
	stableKey := len(store) == 1 && saved != nil && getString(saved, "branch_tip") == "def5678"

	// A moved tip drops the resolved checks and the next action but keeps the rationale and the contract reference.
	stale, staleNotes := checkpointRecover(protocol, persisted, twoSessionBranch, "def5678", resolves)
	staleTip := stale != nil &&
		len(asList(stale["checks"])) == 0 &&
		getString(stale, "next_action") == "" &&
		getString(stale, "rationale") == twoSessionRationale &&
		equalStrings(stringList(stale["contract"]), []string{twoSessionTicket}) &&
		anyContains(staleNotes, "stale tip")

	// Terminal records are excluded from ordinary continuation.
	terminalExcluded := true
	for _, state := range stringList(protocol["terminal_states"]) {
		checkpointMutate(protocol, persisted, twoSessionBranch, map[string]any{"state": state})
		record, reasons := checkpointRecover(protocol, persisted, twoSessionBranch, "abc1234", resolves)
		terminalExcluded = terminalExcluded && record == nil && equalStrings(reasons, []string{"terminal state " + state + " is excluded"})
	}

	return map[string]any{
		"two_session_recovers_requirement": recoversRequirement,
		"two_session_recovers_rationale":   recoversRationale,
		"two_session_recovers_contract":    recoversContract,
		"branch_isolation":                 isolation,
		"stable_topic_key_upserts":         stableKey,
		"stale_tip_keeps_rationale":        staleTip,
		"terminal_excluded":                terminalExcluded,
	}, nil
}

func probeCheckpointTwoSession(repo, root string) error {
	outcomes, err := probeCheckpointTwoSessionRun(root)
	if err != nil {
		return err
	}
	return writeArtifact(repo, "checkpoint.json", outcomes)
}
