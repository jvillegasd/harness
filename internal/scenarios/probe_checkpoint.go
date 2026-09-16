package scenarios

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
)

var checkpointSkillPath = filepath.Join("skills", "workflow-checkpoint", "SKILL.md")

// probeCheckpointProtocol reads the canonical protocol block from the workflow-checkpoint skill.
func probeCheckpointProtocol(root string) (map[string]any, error) {
	text, err := readText(joinPath(root, checkpointSkillPath))
	if err != nil {
		return nil, err
	}
	for _, block := range jsonBlocks(text) {
		if _, ok := block["fields"].(map[string]any); ok {
			if _, ok := block["states"]; ok {
				return block, nil
			}
		}
	}
	return nil, errors.New("workflow-checkpoint skill carries no canonical protocol block")
}

func checkpointTopicKey(protocol map[string]any, branch string) string {
	return strings.Replace(getString(protocol, "topic_key"), "<branch>", branch, 1)
}

func checkpointSave(protocol map[string]any, store map[string]string, branch, tip, state string, contract, checks []string, rationale, nextAction string) (map[string]any, error) {
	record := map[string]any{
		"state":       state,
		"branch_tip":  tip,
		"contract":    toAnyList(contract),
		"checks":      toAnyList(checks),
		"rationale":   rationale,
		"next_action": nextAction,
	}
	fields := dict(protocol, "fields")
	if len(record) != len(fields) || !hasAll(record, sortedKeys(fields)) {
		return nil, errors.New("probe record does not match the skill schema")
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	store[checkpointTopicKey(protocol, branch)] = string(encoded)
	return record, nil
}

func toAnyList(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func checkpointLoad(protocol map[string]any, store map[string]string, branch string) (map[string]any, bool) {
	raw, ok := store[checkpointTopicKey(protocol, branch)]
	if !ok {
		return nil, false
	}
	decoded, err := decodeJSON([]byte(raw))
	if err != nil {
		return nil, false
	}
	record, ok := decoded.(map[string]any)
	return record, ok
}

func checkpointMutate(protocol map[string]any, store map[string]string, branch string, changes map[string]any) {
	record, ok := checkpointLoad(protocol, store, branch)
	if !ok {
		return
	}
	for key, value := range changes {
		record[key] = value
	}
	encoded, _ := json.Marshal(record)
	store[checkpointTopicKey(protocol, branch)] = string(encoded)
}

func checkpointValidShapes(protocol map[string]any, record map[string]any) bool {
	shapes := dict(protocol, "fields")
	for field, rawShape := range shapes {
		shape, _ := rawShape.(string)
		value, ok := record[field]
		if !ok || !matchesShape(value, shape) {
			return false
		}
	}
	return true
}

func matchesShape(value any, shape string) bool {
	switch shape {
	case "string":
		_, ok := value.(string)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "number":
		_, ok := toNumber(value)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	}
	return false
}

// checkpointRecover validates a record against the protocol and drops what a moved tip invalidates.
func checkpointRecover(protocol map[string]any, store map[string]string, branch, currentTip string, resolve func(string) bool) (map[string]any, []string) {
	record, ok := checkpointLoad(protocol, store, branch)
	if !ok {
		return nil, []string{"no checkpoint for branch"}
	}
	state := getString(record, "state")
	if containsString(stringList(protocol["terminal_states"]), state) {
		return nil, []string{"terminal state " + state + " is excluded"}
	}
	if !containsString(stringList(protocol["states"]), state) {
		return nil, []string{"unknown state " + state + " is excluded"}
	}
	if !checkpointValidShapes(protocol, record) {
		return nil, []string{"a field is missing or has the wrong shape"}
	}
	for _, field := range stringList(protocol["require_non_empty"]) {
		if !truthy(record[field]) {
			return nil, []string{"required field " + field + " is empty"}
		}
	}
	contract := stringList(record["contract"])
	var resolved []any
	for _, ref := range contract {
		if resolve(ref) {
			resolved = append(resolved, ref)
		}
	}
	if len(resolved) == 0 {
		return nil, []string{"no contract reference resolves"}
	}
	usable := map[string]any{}
	for key, value := range record {
		usable[key] = value
	}
	usable["contract"] = resolved
	var notes []string
	for _, ref := range contract {
		if !containsString(stringList(resolved), ref) {
			notes = append(notes, "contract reference "+ref+" did not resolve")
		}
	}
	if getString(record, "branch_tip") != currentTip {
		shapes := dict(protocol, "fields")
		for _, field := range stringList(protocol["stale_invalidates"]) {
			if getString(shapes, field) == "array" {
				usable[field] = []any{}
			} else {
				usable[field] = ""
			}
		}
		notes = append(notes, "stale tip; "+strings.Join(stringList(protocol["stale_invalidates"]), ", ")+" not reused")
	}
	return usable, notes
}

func checkpointReadBack(protocol map[string]any, store map[string]string, branch string, written map[string]any) bool {
	loaded, ok := checkpointLoad(protocol, store, branch)
	if !ok {
		return false
	}
	return deepEqual(loaded, written)
}

func deepEqual(a, b any) bool {
	encodedA, errA := json.Marshal(a)
	encodedB, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return string(encodedA) == string(encodedB)
}

// probeCheckpointRun drives one serialized store through recovery, with the rules, not a parallel copy, deciding each outcome.
func probeCheckpointRun(root string) (map[string]any, error) {
	protocol, err := probeCheckpointProtocol(root)
	if err != nil {
		return nil, errors.New("workflow-checkpoint skill carries no canonical protocol block")
	}
	store := map[string]string{}
	branch := "feat/83-centralize-checkpoint-protocol"
	tip := "abc123"
	contract := []string{"https://github.com/jvillegasd/dotfiles/issues/83"}
	resolve := func(ref string) bool { return strings.HasPrefix(ref, "https://") }
	active := getString(protocol, "active_state")

	written, err := checkpointSave(protocol, store, branch, tip, active, contract, []string{"typecheck: pass", "lint: pass"}, "the skill is the single schema source", "wire the commands at their return boundaries (#77)")
	if err != nil {
		return nil, err
	}
	snapshotRaw, _ := json.Marshal(written)
	snapshotDecoded, _ := decodeJSON(snapshotRaw)
	snapshot, _ := snapshotDecoded.(map[string]any)
	readBackOK := checkpointReadBack(protocol, store, branch, snapshot)

	corrupted := map[string]any{}
	for key, value := range snapshot {
		corrupted[key] = value
	}
	corrupted["branch_tip"] = "corrupted"
	encoded, _ := json.Marshal(corrupted)
	store[checkpointTopicKey(protocol, branch)] = string(encoded)
	detectsCorruption := !checkpointReadBack(protocol, store, branch, snapshot)
	if _, err := checkpointSave(protocol, store, branch, tip, active, contract, []string{"typecheck: pass"}, "why", "wire (#77)"); err != nil {
		return nil, err
	}

	other, otherReasons := checkpointRecover(protocol, store, "feat/99-other", tip, resolve)
	isolation := other == nil && equalStrings(otherReasons, []string{"no checkpoint for branch"})

	stale, staleNotes := checkpointRecover(protocol, store, branch, "def456", resolve)
	staleChecksDropped := stale != nil &&
		len(asList(stale["checks"])) == 0 &&
		getString(stale, "next_action") == "" &&
		equalStrings(stringList(stale["contract"]), contract) &&
		anyContains(staleNotes, "stale tip")

	checkpointMutate(protocol, store, branch, map[string]any{"state": "paused"})
	unknown, unknownReasons := checkpointRecover(protocol, store, branch, tip, resolve)
	invalidStateRejected := unknown == nil && equalStrings(unknownReasons, []string{"unknown state paused is excluded"})
	checkpointMutate(protocol, store, branch, map[string]any{"state": active})

	terminalExcluded := true
	for _, state := range stringList(protocol["terminal_states"]) {
		checkpointMutate(protocol, store, branch, map[string]any{"state": state})
		record, reasons := checkpointRecover(protocol, store, branch, tip, resolve)
		terminalExcluded = terminalExcluded && record == nil && equalStrings(reasons, []string{"terminal state " + state + " is excluded"})
	}
	checkpointMutate(protocol, store, branch, map[string]any{"state": active})

	checkpointMutate(protocol, store, branch, map[string]any{"contract": []any{}})
	empty, emptyReasons := checkpointRecover(protocol, store, branch, tip, resolve)
	emptyContractRejected := empty == nil && equalStrings(emptyReasons, []string{"required field contract is empty"})

	checkpointMutate(protocol, store, branch, map[string]any{"contract": "https://github.com/jvillegasd/dotfiles/issues/83"})
	badShape, shapeReasons := checkpointRecover(protocol, store, branch, tip, resolve)
	shapeRejected := badShape == nil && equalStrings(shapeReasons, []string{"a field is missing or has the wrong shape"})

	checkpointMutate(protocol, store, branch, map[string]any{"contract": toAnyList(contract)})
	expired, expiredReasons := checkpointRecover(protocol, store, branch, tip, func(string) bool { return false })
	contractRejected := expired == nil && equalStrings(expiredReasons, []string{"no contract reference resolves"})

	checkpointMutate(protocol, store, branch, map[string]any{"contract": toAnyList([]string{"https://good", "https://gone"})})
	partial, partialNotes := checkpointRecover(protocol, store, branch, tip, func(ref string) bool { return ref == "https://good" })
	contractPartial := partial != nil &&
		equalStrings(stringList(partial["contract"]), []string{"https://good"}) &&
		anyContains(partialNotes, "did not resolve")

	return map[string]any{
		"isolation":                    isolation,
		"stale_checks_dropped":         staleChecksDropped,
		"invalid_state_rejected":       invalidStateRejected,
		"terminal_excluded":            terminalExcluded,
		"empty_contract_rejected":      emptyContractRejected,
		"shape_rejected":               shapeRejected,
		"contract_rejected":            contractRejected,
		"contract_partial":             contractPartial,
		"read_back":                    readBackOK,
		"read_back_detects_corruption": detectsCorruption,
	}, nil
}

func anyContains(items []string, sub string) bool {
	for _, item := range items {
		if strings.Contains(item, sub) {
			return true
		}
	}
	return false
}

func probeCheckpoint(repo, root string) error {
	outcomes, err := probeCheckpointRun(root)
	if err != nil {
		return err
	}
	return writeArtifact(repo, "checkpoint.json", outcomes)
}
