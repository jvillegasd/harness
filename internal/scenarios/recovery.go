package scenarios

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/jvillegasd/harness/internal/models"
)

// The routes a recovery experiment can take: two fresh client sessions for a checkpoint continuation, and one dispatched worker observed through the headless CLI or an Orca worker terminal.
const (
	routeFresh    = "fresh-session"
	routeHeadless = "headless"
	routeOrca     = "orca"
)

// Continuation outcomes. Only a fresh session that recovered both values from the branch checkpoint, never from its prompt or a shared conversation and never from another branch, is accepted.
const (
	continuationRecovered          = "recovered"
	continuationRepeatedInPrompt   = "repeated_in_prompt"
	continuationSharedConversation = "shared_conversation"
	continuationUnrelatedSupplied  = "unrelated_supplied"
	continuationMissingRequirement = "missing_requirement"
	continuationMissingRationale   = "missing_rationale"
	continuationMemoryNotIsolated  = "memory_not_isolated"
	continuationSourceOnDisk       = "source_readable_on_disk"
	continuationUnverified         = "unverified"
)

// Worker outcomes. A proven stop or an uncertain outcome that started no fallback is accepted; a post-stop edit or a fallback beside a possibly live writer is rejected.
const (
	workerProvenStop          = "proven_stop"
	workerPostStopEdit        = "post_stop_edit"
	workerUncertainNoFallback = "uncertain_no_fallback"
	workerUncertainFallback   = "uncertain_with_fallback"
	workerUnverified          = "unverified"
)

// continuationFacts are the retained observations a fresh-session continuation produced.
type continuationFacts struct {
	Prompt           string   // the continuation session's prompt, which must not carry either value
	Recovered        string   // the continuation session's recovered checkpoint text
	Requirement      string   // the deferred requirement session one recorded
	Rationale        string   // its rationale, durable nowhere else
	Unrelated        []string // values only an unrelated branch's checkpoint holds
	MemoryReadable   bool     // the runner could read the isolated store
	MemoryIsolated   bool     // the checkpoint was found in the isolated test store
	SharedTranscript bool     // the sessions shared one conversation
	RouteAvailable   bool     // the client route and memory service answered
	HandoffDeclared  bool     // the scenario declared a temporary document the worker composed the handoff into
	DocumentRemoved  bool     // the runner removed that document before the fresh parent ran
	Residue          []string // workspace paths, or git history or index, where a declared value was still readable after the removal
	ProjectCleared   bool     // the runner cleared the disposable test project before the first session ran
}

// continuationVerdict is the classification of one continuation observation.
type continuationVerdict struct {
	Outcome string
	Reasons []string
}

// continuationAccepted reports whether a continuation verdict proves recovery from the branch checkpoint.
func continuationAccepted(outcome string) bool { return outcome == continuationRecovered }

// continuationOracle classifies whether a fresh session recovered the deferred requirement and its rationale from the branch checkpoint or handoff. It refuses a value repeated in the prompt, a shared conversation, a value still readable in the workspace, an unrelated branch's value, a non-isolated store, and an unavailable route before it accepts a recovery.
func continuationOracle(f continuationFacts) continuationVerdict {
	if !f.RouteAvailable || !f.MemoryReadable {
		return continuationVerdict{continuationUnverified, []string{"the client route, model, or memory service did not answer"}}
	}
	if !f.ProjectCleared {
		return continuationVerdict{continuationUnverified, []string{"the disposable test project could not be cleared, so an earlier run's records could answer this one"}}
	}
	if !f.MemoryIsolated {
		if f.HandoffDeclared {
			return continuationVerdict{continuationMemoryNotIsolated, []string{"both values were not held under the branch handoff key in the isolated test store, or another branch's handoff held one"}}
		}
		return continuationVerdict{continuationMemoryNotIsolated, []string{"the checkpoint was not found in the isolated test store"}}
	}
	if f.SharedTranscript {
		return continuationVerdict{continuationSharedConversation, []string{"the sessions shared one conversation"}}
	}
	if f.Requirement == "" || f.Rationale == "" {
		return continuationVerdict{continuationUnverified, []string{"the experiment declared no requirement or rationale to recover"}}
	}
	if f.HandoffDeclared && !f.DocumentRemoved {
		return continuationVerdict{continuationUnverified, []string{"the handoff document was not present when the runner removed it, so recovery after its removal was not exercised"}}
	}
	if strings.Contains(f.Prompt, f.Requirement) || strings.Contains(f.Prompt, f.Rationale) {
		return continuationVerdict{continuationRepeatedInPrompt, []string{"the continuation prompt repeated a value it had to recover"}}
	}
	// A value still readable without Engram would fake a recovery, so residue is refused before the recovered text is trusted.
	if f.HandoffDeclared && len(f.Residue) > 0 {
		return continuationVerdict{continuationSourceOnDisk, []string{"a deferred value was still readable outside Engram: " + strings.Join(f.Residue, ", ")}}
	}
	for _, value := range f.Unrelated {
		if value != "" && value != f.Requirement && value != f.Rationale && strings.Contains(f.Recovered, value) {
			return continuationVerdict{continuationUnrelatedSupplied, []string{"an unrelated branch's checkpoint supplied a value"}}
		}
	}
	if !strings.Contains(f.Recovered, f.Requirement) {
		return continuationVerdict{continuationMissingRequirement, []string{"the deferred requirement was not recovered"}}
	}
	if !strings.Contains(f.Recovered, f.Rationale) {
		return continuationVerdict{continuationMissingRationale, []string{"the deferred rationale was not recovered"}}
	}
	return continuationVerdict{continuationRecovered, nil}
}

// workerFacts are the retained observations of one dispatched worker and its stop.
type workerFacts struct {
	Identities      bool     // the dispatch and worker identities were recorded
	Available       bool     // the route, model, and budget answered
	ProvenStopped   bool     // the worker is proven to have stopped
	Surviving       []string // surviving worker processes
	PostStopEdits   []string // files changed after the recorded stop
	FallbackStarted bool     // a fallback was started beside the worker
}

// workerVerdict is the classification of one worker observation.
type workerVerdict struct {
	Outcome string
	Reasons []string
}

// workerAccepted reports whether a worker verdict is the conservative, safe result: a proven stop, or an uncertain outcome that started no fallback.
func workerAccepted(outcome string) bool {
	return outcome == workerProvenStop || outcome == workerUncertainNoFallback
}

// workerRecoveryOracle classifies a dispatched worker's outcome. A post-stop edit or a fallback started beside a possibly live writer is rejected; a proven stop with nothing surviving is accepted; anything else is an accepted no-fallback wait, or unverified when the route never answered or the identities were not recorded.
func workerRecoveryOracle(f workerFacts) workerVerdict {
	if !f.Available {
		return workerVerdict{workerUnverified, []string{"the dispatch route, model, memory service, or budget did not answer"}}
	}
	if !f.Identities {
		return workerVerdict{workerUnverified, []string{"the dispatch or worker identity was not recorded"}}
	}
	if len(f.PostStopEdits) > 0 {
		return workerVerdict{workerPostStopEdit, []string{"the workspace changed after the recorded stop: " + strings.Join(f.PostStopEdits, ", ")}}
	}
	if f.ProvenStopped && len(f.Surviving) == 0 {
		return workerVerdict{workerProvenStop, nil}
	}
	if f.FallbackStarted {
		return workerVerdict{workerUncertainFallback, []string{"a fallback started beside a worker whose liveness is unproven: " + strings.Join(f.Surviving, ", ")}}
	}
	return workerVerdict{workerUncertainNoFallback, nil}
}

// ---- the live experiments ----

// recoverySession is one declared fresh session. Its prompt is expanded with the declared checkpoint values, except that a continuation prompt deliberately excludes them.
type recoverySession struct {
	ID     string
	Prompt string
}

// continuationSpec is the declared branch checkpoint a continuation experiment saves and recovers.
type continuationSpec struct {
	Branch          string
	Requirement     string
	Rationale       string
	UnrelatedBranch string
	UnrelatedValue  string
}

func declaredSessions(scenario map[string]any) []recoverySession {
	var out []recoverySession
	for index, raw := range asList(scenario["sessions"]) {
		entry, _ := raw.(map[string]any)
		id := getString(entry, "id")
		if id == "" {
			id = fmt.Sprintf("session-%d", index+1)
		}
		out = append(out, recoverySession{ID: id, Prompt: getString(entry, "prompt")})
	}
	return out
}

func continuationSpecFrom(scenario map[string]any) continuationSpec {
	block := dict(scenario, "continuation")
	return continuationSpec{
		Branch:          getString(block, "branch"),
		Requirement:     getString(block, "requirement"),
		Rationale:       getString(block, "rationale"),
		UnrelatedBranch: getString(block, "unrelated_branch"),
		UnrelatedValue:  getString(block, "unrelated_value"),
	}
}

// expandContinuationPrompt substitutes the declared values a session is allowed to see. The continuation prompt names only the branch topic key, never the requirement or its rationale.
func expandContinuationPrompt(prompt string, spec continuationSpec, index int) string {
	replacements := map[string]string{
		"{{branch}}":           spec.Branch,
		"{{unrelated_branch}}": spec.UnrelatedBranch,
	}
	if index == 0 {
		replacements["{{requirement}}"] = spec.Requirement
		replacements["{{rationale}}"] = spec.Rationale
		replacements["{{unrelated_value}}"] = spec.UnrelatedValue
	}
	for token, value := range replacements {
		prompt = strings.ReplaceAll(prompt, token, value)
	}
	return prompt
}

// sessionClient names the client a continuation experiment launches.
func sessionClient(scenario map[string]any) string {
	if client := getString(scenario, "client"); client != "" {
		return client
	}
	return "claude"
}

// sessionIdentity is the retained identity of one launched session.
type sessionIdentity struct {
	ID        string  `json:"id"`
	Client    string  `json:"client"`
	Model     string  `json:"model"`
	PID       int     `json:"pid"`
	Exit      int     `json:"exit"`
	StartedAt string  `json:"started_at"`
	Seconds   float64 `json:"seconds"`
}

// recoveryEnv isolates the experiment: a disposable practice project namespaces the checkpoint under a test project, so personal memory and live work are untouched. HOME is left intact because the client authenticates from it.
func recoveryEnv(base []string, scenario map[string]any) []string {
	return append(base, "ENGRAM_PROJECT="+recoveryProject(scenario))
}

// hasRecovery reports whether a scenario is one of the declared live recovery experiments.
func hasRecovery(scenario map[string]any) bool {
	return truthy(scenario["sessions"]) || truthy(scenario["worker"])
}

// liveSessionCount is how many model sessions a live scenario spends against the batch budget.
func liveSessionCount(scenario map[string]any) int {
	if hasPublicationGate(scenario) {
		return 2
	}
	if truthy(scenario["sessions"]) {
		return len(declaredSessions(scenario))
	}
	return 1
}

// runRecoveryExperiment dispatches to the declared live experiment and writes its classification artifact. It returns the reason an unverified outcome must be reported as incomplete rather than as a failed check. It is the only live path these scenarios take; there is no general scheduler.
func runRecoveryExperiment(scenario map[string]any, repo, root string, env []string, events, transcript *[]any) (map[string]any, string, error) {
	var artifact map[string]any
	var err error
	if truthy(scenario["sessions"]) {
		artifact, err = runContinuationExperiment(scenario, repo, root, env, events, transcript)
	} else {
		artifact, err = runWorkerExperiment(scenario, repo, root, env, events, transcript)
	}
	if err != nil {
		return nil, "", err
	}
	if err := writeArtifact(repo, "recovery.json", artifact); err != nil {
		return nil, "", err
	}
	// Retain the classification with its recorded identities, revision, and route in the attempt transcript, so the evidence a review binds to carries what actually ran.
	*transcript = append(*transcript, map[string]any{"stream": "recovery", "text": indentJSON(artifact)})
	return artifact, unverifiedReason(artifact), nil
}

// unverifiedReason reports an unavailable route, model, memory service, or budget as unverified, never as a failed behavioral check.
func unverifiedReason(artifact map[string]any) string {
	if getString(artifact, "outcome") != continuationUnverified {
		return ""
	}
	return "unverified: " + strings.Join(stringList(artifact["reasons"]), "; ")
}

// launchSession runs one fresh client process in the repository and retains its events and transcript tagged by session id.
func launchSession(client, repo string, env []string, prompt, id string, events, transcript *[]any) (sessionIdentity, error) {
	identity := sessionIdentity{ID: id, Client: client}
	started := time.Now()
	*events = append(*events, map[string]any{"type": "session-start", "client": client, "session": id})
	cmd := exec.Command(client, "-p", "--output-format", "stream-json", "--verbose", prompt)
	cmd.Dir, cmd.Env = repo, env
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &outBuf, &errBuf
	runErr := cmd.Run()
	if cmd.Process != nil {
		identity.PID = cmd.Process.Pid
	}
	identity.Seconds = round6(time.Since(started).Seconds())
	identity.StartedAt = started.UTC().Format("2006-01-02T15:04:05.000000")
	code := 0
	if exit, ok := runErr.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if runErr != nil {
		code = -1
	}
	identity.Exit = code
	end := map[string]any{"type": "session-end", "client": client, "session": id, "code": code}
	if runErr != nil {
		end["error"] = runErr.Error()
	}
	*events = append(*events, end)
	var local []any
	for _, line := range strings.Split(outBuf.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		decoded, err := decodeJSON([]byte(line))
		if err != nil {
			*events = append(*events, map[string]any{"type": "output", "session": id, "text": line})
			continue
		}
		*events = append(*events, decoded)
		local = append(local, decoded)
	}
	if model := modelFromEvents(local); model != nil {
		identity.Model = *model
	}
	*transcript = append(*transcript, map[string]any{"stream": "stdout", "session": id, "text": outBuf.String()})
	*transcript = append(*transcript, map[string]any{"stream": "stderr", "session": id, "text": errBuf.String()})
	return identity, runErr
}

// runContinuationExperiment launches each declared session as a fresh process against one isolated memory store, then classifies what the continuation recovered.
func runContinuationExperiment(scenario map[string]any, repo, root string, env []string, events, transcript *[]any) (map[string]any, error) {
	sessions := declaredSessions(scenario)
	if len(sessions) < 2 {
		return nil, fmt.Errorf("a continuation experiment needs at least two sessions")
	}
	spec := continuationSpecFrom(scenario)
	client := sessionClient(scenario)
	env = recoveryEnv(env, scenario)
	document := getString(dict(scenario, "handoff"), "document")
	key, prefix := "", ""
	if document != "" {
		// The key comes from the checkpoint skill's canonical protocol, so a changed protocol changes what this experiment reads back.
		protocol, err := probeCheckpointProtocol(root)
		if err != nil {
			return nil, err
		}
		key = handoffKey(protocol, spec.Branch)
		prefix, _, _ = strings.Cut(getString(protocol, "handoff_key"), "<branch>")
	}
	project := recoveryProject(scenario)
	cleared := clearTestProject(project)
	*events = append(*events, map[string]any{"type": "test-project-cleared", "project": project, "cleared": cleared})
	identities := make([]any, 0, len(sessions))
	available := true
	removed := false
	var residue []string
	for index, session := range sessions {
		// The composing document is removed before the fresh parent launches, so what the parent recovers came from the store rather than from a file it could reread.
		if document != "" && index == len(sessions)-1 {
			removed, residue = removeHandoffDocument(repo, document, spec)
			*events = append(*events, map[string]any{"type": "handoff-document-removed", "document": document, "removed": removed, "residue": toAnyList(residue)})
		}
		prompt := expandContinuationPrompt(session.Prompt, spec, index)
		identity, err := launchSession(client, repo, env, prompt, session.ID, events, transcript)
		if err != nil {
			available = false
		}
		identities = append(identities, identity)
	}
	recovered, _ := readText(filepath.Join(repo, filepath.FromSlash("recovery/session-two.json")))
	// Isolation is read back from the store by the runner, not taken from the session's own file, so a value recovered from personal memory is visible rather than assumed away.
	var isolated, readable bool
	if document != "" {
		isolated, readable = storeHoldsHandoff(project, key, prefix, spec.Requirement, spec.Rationale)
	} else {
		isolated, readable = storeHoldsCheckpoint(project, spec.Requirement)
	}
	facts := continuationFacts{
		Prompt:          expandContinuationPrompt(sessions[1].Prompt, spec, 1),
		Recovered:       recovered,
		Requirement:     spec.Requirement,
		Rationale:       spec.Rationale,
		Unrelated:       []string{spec.UnrelatedValue},
		MemoryReadable:  readable,
		MemoryIsolated:  isolated,
		RouteAvailable:  available && recovered != "",
		HandoffDeclared: document != "",
		DocumentRemoved: removed,
		ProjectCleared:  cleared,
		Residue:         residue,
	}
	verdict := continuationOracle(facts)
	artifact := map[string]any{
		"outcome":    verdict.Outcome,
		"accepted":   continuationAccepted(verdict.Outcome),
		"reasons":    toAnyList(verdict.Reasons),
		"route":      routeFresh,
		"client":     client,
		"identities": identities,
		"revision":   revisionOf(root),
		"memory":     map[string]any{"readable": readable, "isolated": isolated, "project": project, "project_cleared": cleared},
	}
	if document != "" {
		artifact["handoff"] = map[string]any{
			"key": key, "document": document, "document_removed": removed, "residue": toAnyList(residue),
			// This experiment never starts a worker beside a possibly live one, so an uncertain outcome is reported rather than replaced.
			"fallback_started": false,
		}
	}
	return artifact, nil
}

// removeHandoffDocument deletes the temporary document the worker composed the handoff into, then reports every workspace file, and git's history or index, where the requirement or the rationale is still readable. Both are the runner's own observations, never the session's report, because a value left on disk would let a fresh parent fake a recovery.
func removeHandoffDocument(repo, document string, spec continuationSpec) (removed bool, residue []string) {
	removed = os.Remove(filepath.Join(repo, filepath.FromSlash(document))) == nil
	for rel := range snapshot(repo) {
		text, err := readText(filepath.Join(repo, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		if strings.Contains(text, spec.Requirement) || strings.Contains(text, spec.Rationale) {
			residue = append(residue, rel)
		}
	}
	sort.Strings(residue)
	// A committed or staged document survives its removal from the tree, and the reflog keeps one committed then reset or amended, so git counts as residue too. A failing git, such as a repository with no commit, reads as empty.
	for _, source := range []struct {
		label string
		args  []string
	}{
		{"git history", []string{"log", "--all", "--reflog", "-p", "--no-color"}},
		{"git index", []string{"diff", "--cached", "--no-color"}},
	} {
		out, _ := git(repo, source.args...)
		if strings.Contains(out, spec.Requirement) || strings.Contains(out, spec.Rationale) {
			residue = append(residue, source.label)
		}
	}
	return removed, residue
}

// storeHoldsCheckpoint reads the isolated test project back through the Engram CLI, so isolation is an observation the runner makes rather than the session's own claim. An unreadable store is reported as such, never as a pass.
func storeHoldsCheckpoint(project, query string) (found, readable bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "engram", "search", query, "--project", project, "--limit", "5").Output()
	if err != nil {
		return false, false
	}
	return strings.Contains(string(out), query), true
}

// storeHoldsHandoff reads the isolated test project back through an Engram export, because search prints no topic keys. Each value must sit under the branch handoff key, and no other key under the protocol's handoff prefix may hold either, which is what makes a recovery attributable to the handoff. Every other record is ignored, because Engram tells each session to save and summarize proactively, so those legitimately hold the values. The export holds the machine's whole memory store, so only the two booleans leave this function.
func storeHoldsHandoff(project, key, prefix string, values ...string) (found, readable bool) {
	dir, err := os.MkdirTemp("", "handoff-export-")
	if err != nil {
		return false, false
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "export.json")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "engram", "export", path).Run(); err != nil {
		return false, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, false
	}
	var export struct {
		Observations []struct {
			Project  string `json:"project"`
			TopicKey string `json:"topic_key"`
			Content  string `json:"content"`
		} `json:"observations"`
	}
	if err := json.Unmarshal(raw, &export); err != nil {
		return false, false
	}
	held := map[string]bool{}
	for _, observation := range export.Observations {
		if observation.Project != project || !strings.HasPrefix(observation.TopicKey, prefix) {
			continue
		}
		for _, value := range values {
			if !strings.Contains(observation.Content, value) {
				continue
			}
			if observation.TopicKey != key {
				return false, true
			}
			held[value] = true
		}
	}
	return len(held) == len(values), true
}

// clearTestProject hard-deletes the disposable test project, so an earlier run's records can neither satisfy nor fail this one. It refuses every name outside the scenario namespace, because a hard delete of a real project cannot be undone.
func clearTestProject(project string) bool {
	if !strings.HasPrefix(project, "scenario-") || project == "scenario-" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "engram", "delete", "project", project, "--hard").CombinedOutput()
	// A first run has no project yet, and the CLI (engram v1.20.0) answers `project not found` with exit 1 and never prompts; nothing to clear is already clear.
	return err == nil || strings.Contains(string(out), "project not found")
}

// recoveryProject names the disposable test project the experiment namespaces its checkpoints under.
func recoveryProject(scenario map[string]any) string {
	return "scenario-" + getString(scenario, "id")
}

// workerObservation is what one dispatched worker left behind.
type workerObservation struct {
	Reason    string
	Available bool
	Uncertain bool
	Output    string
}

// dispatchWorker runs the real supported route: `harness dispatch` launches the role as its own agent, through the Orca worker terminal inside an Orca worktree and through the other client's headless CLI otherwise. After interruptAfter it signals the dispatch process, so dispatch cancels its own worker group and reports its outcome; the runner never starts a fallback.
func dispatchWorker(role, session, repo, promptFile string, env []string, interruptAfter time.Duration) workerObservation {
	observation := workerObservation{}
	cmd := exec.Command("harness", "dispatch", role, "--prompt-file", promptFile, "--worktree", repo, "--session", session)
	cmd.Dir, cmd.Env = repo, env
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	// A surviving grandchild that inherited stdout must not hold Wait forever, or a real stop could hang the runner.
	cmd.WaitDelay = 10 * time.Second
	if err := cmd.Start(); err != nil {
		return observation
	}
	finished := make(chan error, 1)
	go func() { finished <- cmd.Wait() }()
	var runErr error
	if interruptAfter > 0 {
		select {
		case runErr = <-finished:
		case <-time.After(interruptAfter):
			_ = cmd.Process.Signal(syscall.SIGTERM)
			// A dispatch that ignores the signal must not wedge the run, so a second bound escalates to a kill.
			select {
			case runErr = <-finished:
			case <-time.After(30 * time.Second):
				_ = cmd.Process.Kill()
				runErr = <-finished
			}
		}
	} else {
		runErr = <-finished
	}
	observation.Output = outBuf.String()
	observation.Available = runErr == nil
	if runErr != nil {
		// A nonzero dispatch prints a JSON reason. A timeout or a failure is the route answering; an outcome_unknown one may still have a live worker, so no fallback may start beside it.
		var failure struct {
			Reason string `json:"reason"`
		}
		_ = json.Unmarshal(outBuf.Bytes(), &failure)
		observation.Reason = failure.Reason
		switch failure.Reason {
		case "timeout", "run_failed":
			observation.Available = true
		case "outcome_unknown":
			observation.Available = true
			observation.Uncertain = true
		default:
			observation.Available = false
		}
	}
	return observation
}

// observeWorkerStop reads the worker's own pid marker and any change made to the workspace after the dispatch stopped, so a proven stop is separated from a still-live writer. The comparison is against the tree as it stood at the stop, not before the run, so files the worker legitimately wrote are not mistaken for post-stop edits.
func observeWorkerStop(repo string, atStop map[string]string, settle time.Duration) (surviving, edits []string) {
	for _, line := range splitLines(readOrEmpty(filepath.Join(repo, "worker.pid"))) {
		pid := strings.TrimSpace(line)
		if pid == "" {
			continue
		}
		if processAlive(pid) {
			surviving = append(surviving, "pid "+pid)
		}
	}
	if settle > 0 {
		time.Sleep(settle)
	}
	after := snapshot(repo)
	changed := map[string]bool{}
	for rel := range atStop {
		if after[rel] != atStop[rel] {
			changed[rel] = true
		}
	}
	for rel := range after {
		if atStop[rel] != after[rel] {
			changed[rel] = true
		}
	}
	for rel := range changed {
		edits = append(edits, rel)
	}
	return surviving, edits
}

func processAlive(pid string) bool {
	var value int
	if _, err := fmt.Sscanf(pid, "%d", &value); err != nil || value <= 0 {
		return false
	}
	return syscall.Kill(value, 0) == nil
}

func readOrEmpty(path string) string {
	text, _ := readText(path)
	return text
}

// runWorkerExperiment dispatches one worker through the declared route, observes its stop, and classifies the outcome. It never starts a fallback, which is what makes an uncertain Orca outcome safe rather than a second writer.
func runWorkerExperiment(scenario map[string]any, repo, root string, env []string, events, transcript *[]any) (map[string]any, error) {
	worker := dict(scenario, "worker")
	route := getString(worker, "route")
	role := getString(worker, "role")
	session := getString(worker, "session")
	if route == "" || role == "" || session == "" {
		return nil, fmt.Errorf("a worker experiment needs a route, role, and session")
	}
	// The real assignment, not a declared one, so the experiment records the client and model that actually ran.
	workerClient, workerModel, resolved := resolveWorkerAssignment(role, session)
	observed := ""
	if resolved {
		observed = detectRecoveryRoute(session, workerClient, repo)
	}
	identities := map[string]any{
		"route": route, "observed_route": observed, "role": role, "session": session,
		"client": workerClient, "model": workerModel, "agent": role, "agent_digest": agentDigest(workerClient, role),
	}
	*events = append(*events, map[string]any{"type": "worker-dispatch", "route": route, "observed_route": observed, "role": role, "session": session, "client": workerClient, "model": workerModel})
	// A declared route the environment would not take dispatches nothing: launching a real worker to discard it is the fallback this runner must not do.
	if !resolved || observed != route {
		verdict := workerRecoveryOracle(workerFacts{Identities: resolved, Available: false})
		return workerArtifact(verdict, route, workerClient, root, identities, nil, nil), nil
	}
	promptFile := filepath.Join(repo, "worker-prompt.md")
	if err := os.WriteFile(promptFile, []byte(getString(scenario, "prompt")), 0o644); err != nil {
		return nil, err
	}
	started := time.Now()
	observation := dispatchWorker(role, session, repo, promptFile, env, workerInterrupt(scenario))
	atStop := snapshot(repo)
	identities["reason"] = observation.Reason
	*events = append(*events, map[string]any{"type": "worker-settled", "route": route, "reason": observation.Reason, "uncertain": observation.Uncertain, "seconds": round6(time.Since(started).Seconds())})
	surviving, edits := observeWorkerStop(repo, atStop, workerSettle(scenario))
	facts := workerFacts{
		Identities:      resolved,
		Available:       observation.Available,
		ProvenStopped:   observation.Available && !observation.Uncertain,
		Surviving:       surviving,
		PostStopEdits:   edits,
		FallbackStarted: false,
	}
	verdict := workerRecoveryOracle(facts)
	*transcript = append(*transcript, map[string]any{"stream": "stderr", "text": strings.Join(surviving, "\n")})
	return workerArtifact(verdict, route, workerClient, root, identities, surviving, edits), nil
}

// workerArtifact is the retained classification of one worker observation.
func workerArtifact(verdict workerVerdict, route, client, root string, identities map[string]any, surviving, edits []string) map[string]any {
	return map[string]any{
		"outcome":          verdict.Outcome,
		"accepted":         workerAccepted(verdict.Outcome),
		"reasons":          toAnyList(verdict.Reasons),
		"route":            route,
		"client":           client,
		"identities":       identities,
		"revision":         revisionOf(root),
		"surviving":        toAnyList(surviving),
		"post_stop_edits":  toAnyList(edits),
		"fallback_started": false,
	}
}

// workerInterrupt bounds how long a live dispatch may run before the runner signals it, so a real headless interruption exercises the stop path. A declaration of zero leaves dispatch to its own settlement.
func workerInterrupt(scenario map[string]any) time.Duration {
	seconds := intOf(dict(scenario, "worker")["interrupt_after_seconds"])
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

// workerSettle is how long the runner waits after a stop before checking for a post-stop edit.
func workerSettle(scenario map[string]any) time.Duration {
	seconds := intOf(dict(scenario, "worker")["settle_seconds"])
	if seconds <= 0 {
		return time.Second
	}
	return time.Duration(seconds) * time.Second
}

// resolveWorkerAssignment reads the role's actual client and model for this session, so the experiment records the assignment that ran rather than one it declared.
func resolveWorkerAssignment(role, session string) (client, model string, ok bool) {
	out, err := exec.Command("harness", "models", "resolve", role, "--session", session, "--json").Output()
	if err != nil {
		return "", "", false
	}
	decoded, err := decodeJSON(out)
	if err != nil {
		return "", "", false
	}
	object, _ := decoded.(map[string]any)
	client, model = getString(object, "client"), getString(object, "model")
	return client, model, client != ""
}

// detectRecoveryRoute reports where the real dispatch would place the role, using the same resolver dispatch uses so a genuine Orca run is not misread as headless.
func detectRecoveryRoute(session, workerClient, repo string) string {
	return models.Route(models.Assignment{Client: workerClient}, session, repo)
}

// revisionOf records the repository revision the experiment ran against.
func revisionOf(root string) string {
	out, err := git(root, "rev-parse", "HEAD")
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(out)
}

// agentDigest records the installed agent definition the worker ran as, reading only the client it actually ran on, so a Claude definition is never bound to an OpenCode run.
func agentDigest(client, role string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "unknown"
	}
	path := filepath.Join(home, ".claude", "agents", role+".md")
	if client == "opencode" {
		config := os.Getenv("XDG_CONFIG_HOME")
		if config == "" {
			config = filepath.Join(home, ".config")
		}
		path = filepath.Join(config, "opencode", "agents", role+".md")
	}
	if pathExists(path) {
		return fileDigest(path)
	}
	return "missing"
}
