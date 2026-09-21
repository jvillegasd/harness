package scenarios

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
)

var executorRolePath = filepath.Join("roles", "executor.md")

// handoffSchema is the canonical worker-handoff field set, read from the checkpoint skill.
func handoffSchema(protocol map[string]any) map[string]any {
	return dict(protocol, "handoff_fields")
}

// handoffKey resolves the branch-scoped handoff key, distinct from the checkpoint topic key.
func handoffKey(protocol map[string]any, branch string) string {
	return strings.Replace(getString(protocol, "handoff_key"), "<branch>", branch, 1)
}

// handoffCompose builds a handoff with exactly the canonical handoff fields.
func handoffCompose(schema map[string]any, slice, revision string, changed, pending, checks, unresolved, artifacts []string) (map[string]any, error) {
	record := map[string]any{
		"slice":         slice,
		"revision":      revision,
		"changed_files": toAnyList(changed),
		"pending_files": toAnyList(pending),
		"checks":        toAnyList(checks),
		"unresolved":    toAnyList(unresolved),
		"artifacts":     toAnyList(artifacts),
	}
	if len(record) != len(schema) || !hasAll(record, sortedKeys(schema)) {
		return nil, errors.New("handoff does not match the canonical handoff_fields")
	}
	return record, nil
}

// handoffStore is the isolated memory store the probe drives; failWrite models an unavailable service and tamper models a write the read-back cannot trust.
type handoffStore struct {
	records   map[string]string
	failWrite bool
	tamper    bool
}

func newHandoffStore() *handoffStore {
	return &handoffStore{records: map[string]string{}}
}

func (s *handoffStore) save(key string, record map[string]any) error {
	if s.failWrite {
		return errors.New("memory service unavailable")
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if s.tamper {
		encoded = []byte(`{"slice":"tampered"}`)
	}
	s.records[key] = string(encoded)
	return nil
}

func (s *handoffStore) load(key string) (map[string]any, bool) {
	raw, ok := s.records[key]
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

// persistHandoff writes under the branch key and reads it back; a failed write or a mismatched read-back is an incomplete durable handoff.
func persistHandoff(store *handoffStore, protocol map[string]any, branch string, record map[string]any) (readBack bool, incomplete bool) {
	if err := store.save(handoffKey(protocol, branch), record); err != nil {
		return false, true
	}
	if !handoffReadBack(store, protocol, branch, record) {
		return false, true
	}
	return true, false
}

// handoffReadBack confirms the stored handoff matches what was written.
func handoffReadBack(store *handoffStore, protocol map[string]any, branch string, record map[string]any) bool {
	loaded, ok := store.load(handoffKey(protocol, branch))
	return ok && deepEqual(loaded, record)
}

// recoverHandoff validates the branch checkpoint, reads its handoff reference, loads the handoff at that key, and validates it against the canonical schema, the current tip, and the handoff's own pending files.
func recoverHandoff(protocol map[string]any, store *handoffStore, branch, currentTip string, resolve func(string) bool) (map[string]any, []string) {
	checkpoint, reasons := checkpointRecover(protocol, store.records, branch, currentTip, resolve)
	if checkpoint == nil {
		return nil, reasons
	}
	key := getString(checkpoint, "handoff")
	if key == "" {
		return nil, []string{"the checkpoint names no handoff"}
	}
	if key != handoffKey(protocol, branch) {
		return nil, []string{"the checkpoint names another handoff"}
	}
	record, ok := store.load(key)
	if !ok {
		return nil, []string{"no handoff for branch"}
	}
	for field, rawShape := range handoffSchema(protocol) {
		shape, _ := rawShape.(string)
		value, ok := record[field]
		if !ok || !matchesShape(value, shape) {
			return nil, []string{"the handoff is missing a field or has the wrong shape"}
		}
	}
	if getString(record, "revision") != currentTip || len(asList(record["pending_files"])) > 0 {
		record["checks"] = []any{}
		return record, []string{"the revision moved or pending files remain; checks invalidated"}
	}
	return record, nil
}

// handoffIsReviewerInput reports whether the review skill's permitted-input block names the handoff among an input or a standard.
func handoffIsReviewerInput(root string) (bool, error) {
	text, err := readText(joinPath(root, filepath.Join("skills", "adversarial-review", "SKILL.md")))
	if err != nil {
		return false, err
	}
	for _, block := range jsonBlocks(text) {
		for _, group := range []string{"inputs", "standards"} {
			groups, ok := block[group].(map[string]any)
			if !ok {
				continue
			}
			for _, raw := range groups {
				for _, item := range asList(raw) {
					if name, ok := item.(string); ok && strings.Contains(strings.ToLower(name), "handoff") {
						return true, nil
					}
				}
			}
		}
	}
	return false, nil
}

// probeWorkerHandoffRun exercises the durable worker handoff over an isolated store and the shared checkpoint protocol.
func probeWorkerHandoffRun(root string) (map[string]any, error) {
	protocol, err := probeCheckpointProtocol(root)
	if err != nil {
		return nil, err
	}
	schema := handoffSchema(protocol)
	if len(schema) == 0 {
		return nil, errors.New("workflow-checkpoint skill carries no canonical handoff_fields")
	}
	branch := "feat/75-worker-handoff-persistence"
	tip := "abc1234"
	contract := []string{"https://github.com/jvillegasd/harness/issues/75"}
	active := getString(protocol, "active_state")
	resolve := func(ref string) bool { return strings.HasPrefix(ref, "https://") }

	checkpointText, err := readText(joinPath(root, checkpointSkillPath))
	if err != nil {
		return nil, err
	}

	record, err := handoffCompose(schema, "persist worker handoffs through Engram", tip,
		[]string{"skills/workflow-checkpoint/SKILL.md", "roles/executor.md"},
		nil,
		[]string{"go test ./...: pass"},
		[]string{"none"},
		[]string{"handoff/<branch>"})
	if err != nil {
		return nil, err
	}
	composition := len(record) == len(schema) && hasAll(record, sortedKeys(schema)) &&
		strings.Contains(checkpointText, "with `scope: project`")

	executor, err := readText(joinPath(root, executorRolePath))
	if err != nil {
		return nil, err
	}
	executorNamesFields := true
	for field := range schema {
		if !strings.Contains(executor, "`"+field+"`") {
			executorNamesFields = false
		}
	}
	neverGrantsApproval := truthy(protocol["handoff_never_grants_approval"]) &&
		!hasAll(record, []string{"approved", "approval"}) &&
		strings.Contains(executor, "never grants approval")

	// Valid recovery: the parent persists, records the key in the checkpoint, reads back, and a fresh parent recovers it only through a valid checkpoint link.
	store := newHandoffStore()
	readBack, incomplete := persistHandoff(store, protocol, branch, record)
	if _, err := checkpointSave(protocol, store.records, branch, tip, active, contract, []string{"go test ./...: pass"}, "", handoffKey(protocol, branch), "human review"); err != nil {
		return nil, err
	}
	recovered, notes := recoverHandoff(protocol, store, branch, tip, resolve)
	recoveredOK := readBack && !incomplete && recovered != nil &&
		getString(recovered, "slice") == getString(record, "slice") &&
		getString(recovered, "revision") == tip && len(notes) == 0

	// Linkage: a handoff is reused only through a checkpoint that validates and names this branch's key.
	unlinked := newHandoffStore()
	if err := unlinked.save(handoffKey(protocol, branch), record); err != nil {
		return nil, err
	}
	_, unlinkedNotes := recoverHandoff(protocol, unlinked, branch, tip, resolve)

	misdirected := newHandoffStore()
	if _, err := checkpointSave(protocol, misdirected.records, branch, tip, active, contract, nil, "", handoffKey(protocol, "feat/99-other"), "human review"); err != nil {
		return nil, err
	}
	if err := misdirected.save(handoffKey(protocol, branch), record); err != nil {
		return nil, err
	}
	_, misdirectedNotes := recoverHandoff(protocol, misdirected, branch, tip, resolve)

	terminalStore := newHandoffStore()
	if _, err := checkpointSave(protocol, terminalStore.records, branch, tip, stringList(protocol["terminal_states"])[0], contract, nil, "", handoffKey(protocol, branch), "human review"); err != nil {
		return nil, err
	}
	if err := terminalStore.save(handoffKey(protocol, branch), record); err != nil {
		return nil, err
	}
	_, terminalNotes := recoverHandoff(protocol, terminalStore, branch, tip, resolve)

	expiredStore := newHandoffStore()
	if _, err := checkpointSave(protocol, expiredStore.records, branch, tip, active, contract, nil, "", handoffKey(protocol, branch), "human review"); err != nil {
		return nil, err
	}
	if err := expiredStore.save(handoffKey(protocol, branch), record); err != nil {
		return nil, err
	}
	_, expiredNotes := recoverHandoff(protocol, expiredStore, branch, tip, func(string) bool { return false })

	linkage := equalStrings(unlinkedNotes, []string{"no checkpoint for branch"}) &&
		equalStrings(misdirectedNotes, []string{"the checkpoint names another handoff"}) &&
		anyContains(terminalNotes, "terminal state") &&
		equalStrings(expiredNotes, []string{"no contract reference resolves"})
	validRecovery := recoveredOK && linkage

	// A checkpoint written before the handoff field existed reads it as empty rather than rejecting the record.
	legacyStore := newHandoffStore()
	legacy, err := json.Marshal(map[string]any{
		"state":       active,
		"branch_tip":  tip,
		"contract":    toAnyList(contract),
		"checks":      []any{},
		"rationale":   "",
		"next_action": "human review",
	})
	if err != nil {
		return nil, err
	}
	legacyStore.records[checkpointTopicKey(protocol, branch)] = string(legacy)
	legacyUsable, _ := checkpointRecover(protocol, legacyStore.records, branch, tip, resolve)
	legacyTolerated := legacyUsable != nil && getString(legacyUsable, "handoff") == ""

	// Missing read-back: a write whose read-back does not match is unverified.
	tampered := newHandoffStore()
	tampered.tamper = true
	tamperedReadBack, tamperedIncomplete := persistHandoff(tampered, protocol, branch, record)
	missingReadBack := !tamperedReadBack && tamperedIncomplete && readBack

	// Foreign branch key: another branch's valid handoff is never recovered for this branch.
	foreignStore := newHandoffStore()
	if _, err := checkpointSave(protocol, foreignStore.records, "feat/99-unrelated", tip, active, contract, nil, "", handoffKey(protocol, "feat/99-unrelated"), "human review"); err != nil {
		return nil, err
	}
	foreignRecord, err := handoffCompose(schema, "another slice", tip, nil, nil, nil, nil, nil)
	if err != nil {
		return nil, err
	}
	if err := foreignStore.save(handoffKey(protocol, "feat/99-unrelated"), foreignRecord); err != nil {
		return nil, err
	}
	theirs, theirsNotes := recoverHandoff(protocol, foreignStore, "feat/99-unrelated", tip, resolve)
	mine, mineNotes := recoverHandoff(protocol, foreignStore, branch, tip, resolve)
	foreignBranchKey := theirs != nil && len(theirsNotes) == 0 &&
		mine == nil && equalStrings(mineNotes, []string{"no checkpoint for branch"})

	// Changed tip or the handoff's own pending files: the reported checks are invalidated.
	pendingRecord, err := handoffCompose(schema, "pending slice", tip,
		[]string{"roles/executor.md"}, []string{"roles/executor.md"},
		[]string{"go test ./...: pass"}, nil, []string{"handoff/<branch>"})
	if err != nil {
		return nil, err
	}
	pendingStore := newHandoffStore()
	if _, err := checkpointSave(protocol, pendingStore.records, branch, tip, active, contract, []string{"go test ./...: pass"}, "", handoffKey(protocol, branch), "human review"); err != nil {
		return nil, err
	}
	_, _ = persistHandoff(pendingStore, protocol, branch, pendingRecord)
	moved, movedNotes := recoverHandoff(protocol, pendingStore, branch, "def5678", resolve)
	pendingOnly, pendingNotes := recoverHandoff(protocol, pendingStore, branch, tip, resolve)
	changedTip := moved != nil && len(asList(moved["checks"])) == 0 && anyContains(movedNotes, "invalidated") &&
		pendingOnly != nil && len(asList(pendingOnly["checks"])) == 0 && anyContains(pendingNotes, "invalidated")

	// Failed persistence: reported incomplete, and the previously stored handoff survives.
	down := newHandoffStore()
	if _, err := checkpointSave(protocol, down.records, branch, tip, active, contract, nil, "", handoffKey(protocol, branch), "human review"); err != nil {
		return nil, err
	}
	if err := down.save(handoffKey(protocol, branch), record); err != nil {
		return nil, err
	}
	previous, _ := down.load(handoffKey(protocol, branch))
	down.failWrite = true
	failedReadBack, failedIncomplete := persistHandoff(down, protocol, branch, foreignRecord)
	survived, _ := down.load(handoffKey(protocol, branch))
	failedPersistence := !failedReadBack && failedIncomplete && deepEqual(survived, previous)

	// Checkpoint non-overwrite: both records share one store, and persisting a handoff leaves the checkpoint unchanged.
	before, _ := checkpointLoad(protocol, store.records, branch)
	_, _ = persistHandoff(store, protocol, branch, record)
	after, _ := checkpointLoad(protocol, store.records, branch)
	nonOverwrite := handoffKey(protocol, branch) != checkpointTopicKey(protocol, branch) && deepEqual(before, after)

	// Reviewer boundary: the handoff is not a permitted reviewer input or standard.
	reviewerInput, err := handoffIsReviewerInput(root)
	if err != nil {
		return nil, err
	}
	reviewersExcluded := !reviewerInput && strings.Contains(checkpointText, "Reviewers never receive the handoff")

	return map[string]any{
		"composition_fields":          composition,
		"executor_names_fields":       executorNamesFields,
		"never_grants_approval":       neverGrantsApproval,
		"valid_recovery":              validRecovery,
		"legacy_checkpoint_tolerated": legacyTolerated,
		"missing_read_back":           missingReadBack,
		"foreign_branch_key":          foreignBranchKey,
		"changed_tip_pending_edits":   changedTip,
		"failed_persistence":          failedPersistence,
		"checkpoint_non_overwrite":    nonOverwrite,
		"reviewers_excluded":          reviewersExcluded,
	}, nil
}

func probeWorkerHandoff(repo, root string) error {
	outcomes, err := probeWorkerHandoffRun(root)
	if err != nil {
		return err
	}
	return writeArtifact(repo, "handoff.json", outcomes)
}
