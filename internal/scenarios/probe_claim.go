package scenarios

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

var (
	claimShipPath   = filepath.Join("commands", "ship.md")
	claimProtoPath  = filepath.Join("commands", "proto.md")
	claimLabelsPath = filepath.Join("docs", "agents", "harness.json")
)

// probeClaimBlock reads the canonical claim block from the document that owns it, so a changed claim rule changes the probe's behavior.
func probeClaimBlock(root, path string) (map[string]any, error) {
	text, err := readText(joinPath(root, path))
	if err != nil {
		return nil, err
	}
	block, err := findBlock(text, []string{"claim"})
	if err != nil {
		return nil, errors.New(path + " carries no canonical claim block")
	}
	return dict(block, "claim"), nil
}

// probeClaimLabels resolves the ready and claimed states from the tracker lifecycle mapping, so a remapped label flips the probe.
func probeClaimLabels(root string) (string, string, error) {
	raw, err := readText(joinPath(root, claimLabelsPath))
	if err != nil {
		return "", "", err
	}
	decoded, err := decodeJSON([]byte(raw))
	if err != nil {
		return "", "", err
	}
	object, _ := decoded.(map[string]any)
	labels := dict(object, "labels")
	ready, claimed := getString(labels, "ready_for_agent"), getString(labels, "in_progress")
	if ready == "" || claimed == "" {
		return "", "", errors.New("harness.json declares no ready_for_agent or in_progress label")
	}
	return ready, claimed, nil
}

// claimCase is one retained experiment: its verdict and the facts the oracle cross-checks.
type claimCase struct {
	verdict     claimVerdict
	independent bool
	readsFirst  bool
	readsReady  bool
}

// runClaimCase creates one disposable tracker shared by independent workspaces, runs the contenders, and returns the judged order.
func runClaimCase(contenders []claimContender, ready, claimed string, synchronized, requiresConfirmation, confirmed bool) (claimCase, []claimEvent, error) {
	workspace, err := os.MkdirTemp("", "claim-experiment-")
	if err != nil {
		return claimCase{}, nil, err
	}
	defer os.RemoveAll(workspace)
	setup, err := prepareClaim(workspace, contenders, ready, claimed, synchronized, claimProbeBarrierSeconds)
	if err != nil {
		return claimCase{}, nil, err
	}
	events, verdict := runClaim(setup, synchronized, requiresConfirmation, confirmed, driverExecutor())
	result := claimCase{verdict: verdict}
	result.independent = claimIndependentWorkspaces(contenders, events)
	result.readsFirst = claimReadsPrecedeWrites(contenders, events)
	result.readsReady = claimReadsObservedReady(contenders, events, ready)
	return result, events, nil
}

// claimIndependentWorkspaces proves from the recorded events, not the setup, that each contender acted in its own workspace.
func claimIndependentWorkspaces(contenders []claimContender, events []claimEvent) bool {
	workspaces := map[string]string{}
	for _, event := range events {
		if event.Contender != "" && event.Workspace != "" {
			workspaces[event.Contender] = event.Workspace
		}
	}
	if len(workspaces) != len(contenders) {
		return false
	}
	distinct := map[string]bool{}
	for _, workspace := range workspaces {
		distinct[workspace] = true
	}
	return len(distinct) == len(contenders)
}

// claimProbeBarrierSeconds bounds the offline barrier wait; the scripted contenders read immediately, so only a broken shim reaches it.
const claimProbeBarrierSeconds = 30

// claimLoserAbstained reports whether a contender read the ticket and then neither wrote nor implemented, the recorded form of `/ship`'s abort-when-claimed rule.
func claimLoserAbstained(contenders []claimContender, events []claimEvent) bool {
	read, wrote, implemented := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, event := range events {
		switch event.Kind {
		case claimRead:
			read[event.Contender] = true
		case claimWrite:
			wrote[event.Contender] = true
		case claimImplement:
			implemented[event.Contender] = true
		}
	}
	for _, contender := range contenders {
		if read[contender.ID] && !wrote[contender.ID] && !implemented[contender.ID] {
			return true
		}
	}
	return false
}

// claimAbortRecorded reports whether a contender recorded the named abort itself.
func claimAbortRecorded(events []claimEvent, detail string) bool {
	for _, event := range events {
		if event.Kind == claimAbort && event.Detail == detail {
			return true
		}
	}
	return false
}

// claimRereadBeforeWrite reports whether every contender read twice and its second read preceded its first write, the recorded form of `/proto`'s re-read before the claim write.
func claimRereadBeforeWrite(contenders []claimContender, events []claimEvent) bool {
	reads := map[string]int{}
	firstWrite := map[string]int{}
	for index, event := range events {
		switch event.Kind {
		case claimRead:
			reads[event.Contender]++
		case claimWrite:
			if _, ok := firstWrite[event.Contender]; !ok {
				firstWrite[event.Contender] = index
			}
		}
	}
	for _, contender := range contenders {
		write, ok := firstWrite[contender.ID]
		if !ok {
			return false
		}
		second := -1
		seen := 0
		for index, event := range events {
			if event.Kind == claimRead && event.Contender == contender.ID {
				seen++
				if seen == 2 {
					second = index
					break
				}
			}
		}
		if reads[contender.ID] < 2 || second < 0 || second > write {
			return false
		}
	}
	return true
}

// claimReadsObservedReady reports whether every contender's first read observed the ready state, so a synchronized duplicate is only claimed from a genuine ready observation.
func claimReadsObservedReady(contenders []claimContender, events []claimEvent, ready string) bool {
	seen := map[string]bool{}
	for _, event := range events {
		if event.Kind == claimRead && !seen[event.Contender] {
			if event.Detail != ready {
				return false
			}
			seen[event.Contender] = true
		}
	}
	return len(seen) == len(contenders)
}

// claimReadsPrecedeWrites reports whether every contender read before the first competing write, the fact a synchronized duplicate depends on.
func claimReadsPrecedeWrites(contenders []claimContender, events []claimEvent) bool {
	firstRead := map[string]int{}
	firstWrite := -1
	for index, event := range events {
		if _, ok := firstRead[event.Contender]; !ok && event.Kind == claimRead {
			firstRead[event.Contender] = index
		}
		if event.Kind == claimWrite && (firstWrite < 0 || index < firstWrite) {
			firstWrite = index
		}
	}
	if firstWrite < 0 {
		return false
	}
	for _, contender := range contenders {
		at, ok := firstRead[contender.ID]
		if !ok || at > firstWrite {
			return false
		}
	}
	return true
}

// probeClaimRun proves the ownership expectation over recorded ordered events: one winner is accepted, duplicates and loser interference are rejected, and a failed barrier, absent contender, unavailable client, or missing prototype confirmation stays incomplete. The experiment is launched as independent processes against one disposable tracker; It returns the synchronized run's ordered events so the caller can retain them with the attempt; model-session runs stay budget-gated and are declared in the claims matrix.
func probeClaimRun(root, repo string) (map[string]any, string, error) {
	ship, err := probeClaimBlock(root, claimShipPath)
	if err != nil {
		return nil, "", err
	}
	proto, err := probeClaimBlock(root, claimProtoPath)
	if err != nil {
		return nil, "", err
	}
	ready, claimed, err := probeClaimLabels(root)
	if err != nil {
		return nil, "", err
	}
	contenders := []claimContender{
		{ID: "ship-a", Workspace: "a", Driver: claimDriverScript},
		{ID: "ship-b", Workspace: "b", Driver: claimDriverScript},
	}
	serialized, serializedEvents, err := runClaimCase(contenders, ready, claimed, false, false, false)
	if err != nil {
		return nil, "", err
	}
	synchronized, events, err := runClaimCase(contenders, ready, claimed, true, false, false)
	if err != nil {
		return nil, "", err
	}
	if err := os.MkdirAll(filepath.Join(repo, "artifacts"), 0o755); err != nil {
		return nil, "", err
	}
	retained := claimEventLines(events)
	if err := os.WriteFile(filepath.Join(repo, "artifacts", "claim-events.jsonl"), []byte(retained), 0o644); err != nil {
		return nil, "", err
	}
	unconfirmed, unconfirmedEvents, err := runClaimCase(contenders, ready, claimed, true, true, false)
	if err != nil {
		return nil, "", err
	}
	confirmedProto, protoEvents, err := runClaimCase(contenders, ready, claimed, true, true, true)
	if err != nil {
		return nil, "", err
	}
	outcomes := map[string]any{
		"serialized_single_winner":       serialized.verdict.Outcome == claimSingleWinner,
		"synchronized_duplicate_exposed": synchronized.verdict.Outcome == claimDuplicateWinners,
		"independent_workspaces":         serialized.independent && synchronized.independent,
		"reads_preceded_writes":          synchronized.readsFirst,
		"proto_unconfirmed_unverified":   unconfirmed.verdict.Outcome == claimConfirmationMissing,
		"proto_confirmed_judged":         confirmedProto.verdict.Outcome == claimDuplicateWinners,
		"interference_rejected":          claimOracle([]string{"a", "b"}, []claimEvent{claimReadEvent("a", ready), claimReadEvent("b", ready), claimWriteEvent("a"), claimImplementEvent("a"), {Contender: "b", Kind: claimInterfere, Workspace: "work/b", Detail: "work/a"}}, true).Outcome == claimLoserInterference,
		"barrier_failure_incomplete":     claimOracle([]string{"a", "b"}, []claimEvent{claimReadEvent("a", ready), claimWriteEvent("a"), claimImplementEvent("a"), claimReadEvent("b", ready)}, true).Outcome == claimBarrierFailed,
		"absent_contender_incomplete":    claimOracle([]string{"a", "b"}, []claimEvent{claimReadEvent("a", ready), claimWriteEvent("a"), claimImplementEvent("a")}, false).Outcome == claimAbsentContender,
		"unavailable_client_incomplete":  claimOracle([]string{"a", "b"}, []claimEvent{claimReadEvent("a", ready), claimWriteEvent("a"), {Contender: "b", Kind: claimUnavailable, Detail: "model_not_found"}}, false).Outcome == claimUnavailableClient,
		"reads_observed_ready":           synchronized.readsReady,
		"serialized_loser_abstained":     claimLoserAbstained(contenders, serializedEvents),
		"proto_abort_recorded":           claimAbortRecorded(unconfirmedEvents, "no_confirmation"),
		"proto_reread_observed":          claimRereadBeforeWrite(contenders, protoEvents),
		"ship_read_before_write":         truthy(ship["read_before_write"]),
		"ship_aborts_when_claimed":       truthy(ship["abort_when_claimed"]),
		"proto_rereads_before_write":     truthy(proto["reread_before_write"]),
		"proto_requires_confirmation":    truthy(proto["requires_confirmation"]),
	}
	return outcomes, retained, nil
}

// claimEventLines renders the retained events, one ordered JSON object per line.
func claimEventLines(events []claimEvent) string {
	items := make([]any, 0, len(events))
	for _, event := range events {
		items = append(items, map[string]any{
			"contender": event.Contender, "workspace": event.Workspace, "kind": event.Kind, "detail": event.Detail,
		})
	}
	return jsonl(items)
}

func probeConcurrentClaim(repo, root string, out io.Writer) error {
	outcomes, retained, err := probeClaimRun(root, repo)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(out, retained); err != nil {
		return err
	}
	return writeArtifact(repo, "concurrent-claim.json", outcomes)
}
