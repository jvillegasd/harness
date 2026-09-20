package scenarios

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// concurrentContenders reads the rival declarations from a concurrent scenario's `concurrent.contenders`.
func concurrentContenders(declaration map[string]any) []claimContender {
	var contenders []claimContender
	for index, raw := range list(declaration, "contenders") {
		entry, _ := raw.(map[string]any)
		id := getString(entry, "id")
		if id == "" {
			id = fmt.Sprintf("contender-%d", index+1)
		}
		workspace := getString(entry, "workspace")
		if workspace == "" {
			workspace = id
		}
		contenders = append(contenders, claimContender{
			ID: id, Workspace: workspace, Prompt: getString(entry, "prompt"), Client: getString(entry, "client"),
		})
	}
	return contenders
}

// claimConcurrentSessions charges one concurrent attempt for every session it launches, since the batch cap counts model sessions, not attempts.
func claimConcurrentSessions(outRoot string, limit *int, sessions int) error {
	if err := claimSession(outRoot, limit); err != nil {
		return err
	}
	data, err := readObject(filepath.Join(outRoot, batchFile))
	if err != nil {
		return err
	}
	cap := intOf(data["cap"])
	if paidSessions(outRoot)+sessions > cap {
		return fmt.Errorf("per-batch session cap reached (%d+%d/%d)", paidSessions(outRoot), sessions, cap)
	}
	return nil
}

// runConcurrent executes a simultaneous-claim scenario: two or more independent model sessions race for one disposable ticket, and the recorded order decides ownership rather than either session's report. Only the offline probe is run in the default matrix; this path is reachable only under --allow-paid.
func runConcurrent(scenario map[string]any, declaration map[string]any, root, outRoot string, allowPaid bool, limit *int) (string, string, error) {
	id := getString(scenario, "id")
	contenders := concurrentContenders(declaration)
	if len(contenders) < 2 {
		return "", "", fmt.Errorf("%s: a concurrent scenario needs at least two contenders", id)
	}
	if !allowPaid {
		return "not-run", "live scenario: pass --allow-paid or provide a recording", nil
	}
	clients := map[string]string{}
	for _, contender := range contenders {
		client := contender.Client
		if client == "" {
			client = "claude"
		}
		if _, err := exec.LookPath(client); err != nil {
			return "", "", err
		}
		clients[contender.ID] = client
	}
	ready, claimed, err := probeClaimLabels(root)
	if err != nil {
		return "", "", err
	}
	synchronized := truthy(declaration["synchronized"])
	if err := claimConcurrentSessions(outRoot, limit, len(contenders)); err != nil {
		return "", "", err
	}
	workspace, err := os.MkdirTemp("", "scenario-"+id+"-")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(workspace)
	setup, err := prepareClaim(workspace, contenders, ready, claimed, synchronized, intOf(declaration["barrier_seconds"]))
	if err != nil {
		return "", "", err
	}
	shared := setup.Shared
	digest := scenarioDigest(scenario, root, filepath.Join(root, "scenarios", "fixtures"))
	now := time.Now().UTC()
	stamp := now.Format("20060102T150405") + fmt.Sprintf("%06dZ", now.Nanosecond()/1000)
	attempt := filepath.Join(outRoot, id, stamp+"-"+digest[:8])
	// A paid attempt is recorded before any session starts, so a session killed mid-run still counts toward the cap.
	if err := writeClaimFile(attempt, "provenance.json", indentJSON(map[string]any{
		"scenario": id, "input_digest": digest, "recorded_at": now.Format("2006-01-02T15:04:05.000000"),
		"executor": "live:" + clients[contenders[0].ID], "sessions": len(contenders),
	})); err != nil {
		return "", "", err
	}
	before := snapshot(shared)
	var mu sync.Mutex
	sessionEvents := map[string][]any{}
	sessionTranscript := map[string][]any{}
	sessionStatuses := map[string]string{}
	started := time.Now()
	launch := func(c claimContender, repo string, env []string) error {
		client := clients[c.ID]
		cmd := exec.Command(client, "-p", "--output-format", "stream-json", "--verbose", c.Prompt)
		cmd.Dir, cmd.Env = repo, env
		var outBuf, errBuf bytes.Buffer
		cmd.Stdout, cmd.Stderr = &outBuf, &errBuf
		runErr := cmd.Run()
		code := 0
		if exit, ok := runErr.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else if runErr != nil {
			code = -1
		}
		var events []any
		events = append(events, map[string]any{"type": "session-start", "client": client, "contender": c.ID})
		for _, line := range strings.Split(outBuf.String(), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			decoded, err := decodeJSON([]byte(line))
			if err != nil {
				events = append(events, map[string]any{"type": "output", "text": line, "contender": c.ID})
				continue
			}
			events = append(events, decoded)
		}
		events = append(events, map[string]any{"type": "session-end", "code": code, "contender": c.ID})
		// Only a session that failed without reaching a model is unavailable; any other nonzero exit is a failure, not an exclusion.
		status := sessionStatus(&code, events)
		if status == "model-unavailable" {
			_ = os.WriteFile(filepath.Join(repo, "unavailable"), []byte(c.ID+"\n"), 0o644)
		}
		mu.Lock()
		sessionEvents[c.ID] = events
		sessionStatuses[c.ID] = status
		sessionTranscript[c.ID] = []any{
			map[string]any{"stream": "stdout", "contender": c.ID, "text": outBuf.String()},
			map[string]any{"stream": "stderr", "contender": c.ID, "text": errBuf.String()},
		}
		mu.Unlock()
		return nil
	}
	events, verdict := runClaim(setup, synchronized, truthy(declaration["requires_confirmation"]), truthy(declaration["confirmed"]), launch)
	wallSeconds := time.Since(started).Seconds()
	after := snapshot(shared)
	if err := writeArtifact(shared, "claim.json", map[string]any{
		"outcome":            verdict.Outcome,
		"single_winner":      claimAccepted(verdict.Outcome),
		"duplicate_winners":  verdict.Outcome == claimDuplicateWinners,
		"loser_interference": verdict.Outcome == claimLoserInterference,
		"winners":            toAnyList(verdict.Winners),
	}); err != nil {
		return "", "", err
	}
	forgeCalls := readForgeLog(shared)
	entries := evaluateArtifacts(scenario, before, after, shared, forgeCalls, true, nil)
	// A race that never reached a judgeable state is incomplete or unverified, not a behavioral rejection, but its side effects are still checked.
	incomplete := ""
	if judgeable := claimJudgeable(verdict.Outcome); judgeable {
		entries = append(entries, entry("ownership_expectation", claimAccepted(verdict.Outcome), verdict.Outcome+": "+strings.Join(verdict.Reasons, "; ")))
	} else {
		incomplete = verdict.Outcome + ": " + strings.Join(verdict.Reasons, "; ")
	}
	if err := writeClaimAttempt(attempt, contenders, sessionEvents, sessionTranscript, sessionStatuses, events, entries, incomplete, forgeCalls, scenario, wallSeconds); err != nil {
		return "", "", err
	}
	state, reasons := evaluate(attempt, scenario, digest)
	if err := writeClaimFile(attempt, "status.json", indentJSON(map[string]any{
		"state": state, "reasons": reasons, "evidence_digest": evidenceDigest(attempt),
	})); err != nil {
		return "", "", err
	}
	return state, attempt, nil
}

// claimJudgeable reports whether a verdict is a statement about ownership at all. A failed barrier, an absent contender, an unavailable client, a missing confirmation, or no implementation is unverified, never a rejection.
func claimJudgeable(outcome string) bool {
	switch outcome {
	case claimSingleWinner, claimDuplicateWinners, claimLoserInterference:
		return true
	}
	return false
}

// readForgeLog returns the `gh` calls the claim shim refused, so the attempt's no-external-forge check judges what the session actually tried.
func readForgeLog(shared string) []string {
	raw, err := os.ReadFile(filepath.Join(shared, "forge.log"))
	if err != nil {
		return nil
	}
	var calls []string
	for _, line := range splitLines(string(raw)) {
		if strings.TrimSpace(line) != "" {
			calls = append(calls, line)
		}
	}
	return calls
}

// writeClaimAttempt retains the same evidence a single-session attempt does, with every event and transcript tagged by the contender that produced it. An unjudgeable race records its reason as incomplete rather than as a failed check.
func writeClaimAttempt(attempt string, contenders []claimContender, sessionEvents, sessionTranscript map[string][]any, sessionStatuses map[string]string, events []claimEvent, entries []map[string]any, incomplete string, forgeCalls []string, scenario map[string]any, wallSeconds float64) error {
	var allEvents, transcript []any
	var prompts []string
	for _, contender := range contenders {
		allEvents = append(allEvents, sessionEvents[contender.ID]...)
		transcript = append(transcript, sessionTranscript[contender.ID]...)
		prompts = append(prompts, contender.ID+": "+contender.Prompt)
	}
	for _, event := range events {
		allEvents = append(allEvents, map[string]any{"type": "claim-" + event.Kind, "contender": event.Contender, "workspace": event.Workspace, "detail": event.Detail})
	}
	ok := true
	results := make([]any, 0, len(entries))
	for _, item := range entries {
		if !truthy(item["ok"]) {
			ok = false
		}
		results = append(results, item)
	}
	artifacts := map[string]any{"ok": ok, "results": results}
	if incomplete != "" {
		artifacts["incomplete_reason"] = incomplete
	}
	for name, text := range map[string]string{
		"prompts.txt":      strings.Join(prompts, "\n") + "\n",
		"events.jsonl":     jsonl(allEvents),
		"artifacts.json":   indentJSON(artifacts),
		"transcript.jsonl": jsonl(transcript),
		"metrics.json":     indentJSON(buildMetrics(scenario, allEvents, wallSeconds, concurrentStatus(sessionStatuses))),
		"forge.log":        strings.Join(append(append([]string{}, forgeCalls...), ""), "\n"),
	} {
		if err := writeClaimFile(attempt, name, text); err != nil {
			return err
		}
	}
	return nil
}

// concurrentStatus aggregates the per-session statuses: interrupted when any session was killed, failed when any session failed, model-unavailable only when every session failed without reaching a model, and ok otherwise.
func concurrentStatus(statuses map[string]string) string {
	interrupted, failed, unavailable, ok := false, false, false, false
	for _, status := range statuses {
		switch status {
		case "interrupted":
			interrupted = true
		case "model-unavailable":
			unavailable = true
		case "failed":
			failed = true
		case "ok":
			ok = true
		}
	}
	switch {
	case len(statuses) == 0:
		return "unknown"
	case interrupted:
		return "interrupted"
	case failed:
		return "failed"
	case unavailable && !ok:
		return "model-unavailable"
	}
	return "ok"
}

func writeClaimFile(attempt, name, text string) error {
	if err := os.MkdirAll(attempt, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(attempt, name), []byte(text), 0o644)
}
