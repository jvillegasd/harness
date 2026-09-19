package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// dispatchBase is a models sandbox with a prompt file, a worktree directory, a claude shim that prints its own output, and an OpenCode agent rendered for the balanced executor.
func dispatchBase(t *testing.T) (base, prompt, worktree string) {
	t.Helper()
	base = modelsBase(t)
	prompt = filepath.Join(base, "tmp", "prompt.md")
	writeMode(t, prompt, "review the diff", 0o644)
	worktree = filepath.Join(base, "wt")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatal(err)
	}
	writeShim(t, base, "claude", `pwd > "`+filepath.Join(base, "claude.pwd")+`"; echo "claude said"`)
	renderAgent(t, base, "opencode", "executor", "opencode-go/glm-5.3", "")
	return base, prompt, worktree
}

// renderAgent writes the frontmatter `harness install` renders for a role agent on one client.
func renderAgent(t *testing.T, base, client, role, model, effort string) {
	t.Helper()
	dir, effortKey := filepath.Join(base, "home", ".claude", "agents"), "effort: "
	if client == "opencode" {
		dir, effortKey = filepath.Join(base, "home", ".config", "opencode", "agents"), "reasoningEffort: "
	}
	head := "---\ndescription: \"x\"\nmodel: " + model + "\n"
	if effort != "" {
		head += effortKey + effort + "\n"
	}
	writeMode(t, filepath.Join(dir, role+".md"), head+"---\n\nbody\n", 0o644)
}

// orcaEnv pins the Orca executable to the shim, since Linux outside an Orca terminal resolves to orca-ide.
func orcaEnv(session string) []string { return []string{session, "ORCA_CLI_COMMAND=orca"} }

// orcaShim fakes an Orca runtime with no bound Run, whose worker-show reports the status and worker state written to base/status.
func orcaShim(t *testing.T, base string) {
	t.Helper()
	status := filepath.Join(base, "status")
	writeMode(t, status, "completed ready\n", 0o644)
	writeShim(t, base, "orca", `read status state < "`+status+`"
case "$1 $2" in
"worktree current") echo '{"ok":true}' ;;
"orchestration run-current") echo '{"ok":true,"result":{"run":null}}' ;;
"orchestration run-create") echo '{"ok":true,"result":{"run":{"id":"r-1"}}}' ;;
"terminal create") echo '{"ok":true,"result":{"terminal":{"handle":"term-7"}}}' ;;
"orchestration worker-start") echo '{"ok":true,"result":{"dispatchId":"d-1","state":"ready"}}' ;;
"orchestration worker-show") echo '{"ok":true,"result":{"dispatch":{"id":"d-1","status":"'"$status"'"},"worker":{"state":"'"$state"'"}}}' ;;
"orchestration worker-read") echo "worker said" ;;
"orchestration worker-release") echo '{"ok":true}' ;;
*) exit 1 ;;
esac`)
}

func runDispatch(t *testing.T, base string, env []string, args ...string) result {
	t.Helper()
	return runHarness(t, base, env, append([]string{"dispatch"}, args...)...)
}

// startDispatch starts dispatch in the background, for a test that signals it.
func startDispatch(t *testing.T, base string, env []string, args ...string) (*exec.Cmd, *strings.Builder) {
	t.Helper()
	cmd := exec.Command(binary, append([]string{"dispatch"}, args...)...)
	cmd.Env = append([]string{"HOME=" + filepath.Join(base, "home"), "PATH=" + filepath.Join(base, "shims") + ":/usr/bin:/bin"}, env...)
	var stdout strings.Builder
	cmd.Stdout = &stdout
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	return cmd, &stdout
}

// waitFor polls until done reports true, failing the test after ten seconds.
func waitFor(t *testing.T, what string, done func() bool) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); !done(); time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

func shimLog(t *testing.T, base string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(base, "shims.log"))
	if err != nil {
		return ""
	}
	return string(data)
}

func reason(t *testing.T, r result) string {
	t.Helper()
	var out map[string]string
	if err := json.Unmarshal([]byte(r.stdout), &out); err != nil {
		t.Fatalf("failure output is not JSON: %v\n%s", err, r.stdout)
	}
	if out["reason"] == "" {
		t.Fatalf("failure output has no reason: %s", r.stdout)
	}
	return out["reason"]
}

func TestDispatchHeadlessRunsTheOtherClientInTheWorktree(t *testing.T) {
	base, prompt, wt := dispatchBase(t)
	writeShim(t, base, "opencode", `case "$1" in models) echo 'opencode-go/glm-5.3' ;; run) pwd > "`+filepath.Join(base, "opencode.pwd")+`"; echo "opencode said" ;; esac`)
	r := ok(t, runDispatch(t, base, []string{"CLAUDECODE=1"}, "executor", "--prompt-file", prompt, "--worktree", wt), "dispatch executor")
	if r.stdout != "opencode said\n" {
		t.Errorf("stdout = %q", r.stdout)
	}
	if !strings.Contains(shimLog(t, base), "opencode run -m opencode-go/glm-5.3 --agent executor -- review the diff <>") {
		t.Errorf("opencode argv wrong:\n%s", shimLog(t, base))
	}
	if got := strings.TrimSpace(readFile(t, filepath.Join(base, "opencode.pwd"))); got != wt {
		t.Errorf("opencode ran in %q, want %q", got, wt)
	}

	ok(t, runModels(t, base, "set", "--preset", "performance"), "set performance")
	r = ok(t, withAgent(t, base, "claude", "reviewer-a", "fable", "high", func() result {
		return runDispatch(t, base, []string{"OPENCODE=1"}, "reviewer-a", "--prompt-file", prompt, "--worktree", wt)
	}), "dispatch reviewer-a")
	if r.stdout != "claude said\n" {
		t.Errorf("stdout = %q", r.stdout)
	}
	// The rendered agent carries the model and effort; a launch-time --model would override it.
	if !strings.Contains(shimLog(t, base), "claude -p --agent reviewer-a <review the diff>") {
		t.Errorf("claude argv wrong:\n%s", shimLog(t, base))
	}
	if got := strings.TrimSpace(readFile(t, filepath.Join(base, "claude.pwd"))); got != wt {
		t.Errorf("claude ran in %q, want %q", got, wt)
	}
}

// withAgent checks that dispatch refuses a missing and then a stale rendered agent before it renders the matching one and runs.
func withAgent(t *testing.T, base, client, role, model, effort string, run func() result) result {
	t.Helper()
	if r := run(); r.code == 0 || reason(t, r) != "agent_missing" {
		t.Errorf("%s %s without its rendered agent: exit %d, %s", client, role, r.code, r.stdout)
	}
	renderAgent(t, base, client, role, model+"-old", effort)
	if r := run(); r.code == 0 || reason(t, r) != "stale_install" {
		t.Errorf("%s %s rendered for another model: exit %d, %s", client, role, r.code, r.stdout)
	}
	renderAgent(t, base, client, role, model, "")
	if effort != "" {
		if r := run(); r.code == 0 || reason(t, r) != "stale_install" {
			t.Errorf("%s %s rendered for another effort: exit %d, %s", client, role, r.code, r.stdout)
		}
	}
	renderAgent(t, base, client, role, model, effort)
	return run()
}

// TestDispatchHeadlessClaudeExecutorKeepsItsLimits lets a headless Claude executor edit and run checks while denying the commit, push, merge, forge, and worktree actions its parent owns.
func TestDispatchHeadlessClaudeExecutorKeepsItsLimits(t *testing.T) {
	base, prompt, wt := dispatchBase(t)
	ok(t, runModels(t, base, "set", "--preset", "performance"), "set performance")
	renderAgent(t, base, "claude", "executor", "opus", "high")
	ok(t, runDispatch(t, base, []string{"OPENCODE=1"}, "executor", "--prompt-file", prompt, "--worktree", wt), "dispatch executor")
	want := "claude -p --agent executor --permission-mode acceptEdits --allowedTools Bash --disallowedTools Bash(git commit:*) Bash(git push:*) Bash(git merge:*) Bash(git worktree remove:*) Bash(wt remove:*) Bash(wt merge:*) Bash(wt step commit:*) Bash(gh:*) <review the diff>"
	if !strings.Contains(shimLog(t, base), want) {
		t.Errorf("headless executor argv wrong:\n%s", shimLog(t, base))
	}
}

// TestDispatchKillsTheWholeHeadlessRun proves a signal leaves no orphan behind: a grandchild the client started dies with it, so a fallback never edits beside it.
func TestDispatchKillsTheWholeHeadlessRun(t *testing.T) {
	base, prompt, wt := dispatchBase(t)
	ok(t, runModels(t, base, "set", "--preset", "performance"), "set performance")
	renderAgent(t, base, "claude", "reviewer-a", "fable", "high")
	pidFile := filepath.Join(base, "grandchild.pid")
	writeShim(t, base, "claude", `sleep 60 & echo $! > "`+pidFile+`"; wait`)
	cmd, stdout := startDispatch(t, base, []string{"OPENCODE=1"}, "reviewer-a", "--prompt-file", prompt, "--worktree", wt)
	var pid int
	waitFor(t, "the grandchild", func() bool {
		data, _ := os.ReadFile(pidFile)
		pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
		return pid != 0
	})
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err == nil {
		t.Error("a signalled dispatch exited 0")
	}
	if !strings.Contains(stdout.String(), `"reason":"timeout"`) {
		t.Errorf("stdout = %q", stdout.String())
	}
	waitFor(t, "the grandchild to die", func() bool { return syscall.Kill(pid, 0) != nil })
}

func TestDispatchInsideOrca(t *testing.T) {
	base, prompt, wt := dispatchBase(t)
	orcaShim(t, base)
	ok(t, runModels(t, base, "set", "--preset", "diversity"), "set diversity")
	renderAgent(t, base, "claude", "executor", "sonnet", "")

	r := ok(t, runDispatch(t, base, orcaEnv("OPENCODE=1"), "executor", "--prompt-file", prompt, "--worktree", wt), "claude role in orca")
	if r.stdout != "worker said\n" {
		t.Errorf("stdout = %q", r.stdout)
	}
	log := shimLog(t, base)
	for _, want := range []string{
		"orca orchestration run-create --objective harness dispatch executor --json",
		"orca terminal create --worktree path:" + wt + " --command 'claude' '--agent' 'executor' '--permission-mode' 'acceptEdits' '--allowedTools' 'Bash' '--disallowedTools' 'Bash(git commit:*)' 'Bash(git push:*)' 'Bash(git merge:*)' 'Bash(git worktree remove:*)' 'Bash(wt remove:*)' 'Bash(wt merge:*)' 'Bash(wt step commit:*)' 'Bash(gh:*)' --json",
		"orca orchestration worker-start --run r-1 --spec review the diff --worktree path:" + wt + " --terminal term-7 --json",
		"orca orchestration worker-read --dispatch d-1 --source auto",
		"orca orchestration worker-release --dispatch d-1 --json",
	} {
		if !strings.Contains(log, want) {
			t.Errorf("log lacks %q:\n%s", want, log)
		}
	}

	renderAgent(t, base, "opencode", "reviewer-b", "openai/gpt-5.6-sol", "")
	ok(t, runDispatch(t, base, orcaEnv("CLAUDECODE=1"), "reviewer-b", "--prompt-file", prompt, "--worktree", wt), "opencode role in orca")
	if want := "orca terminal create --worktree path:" + wt + " --command 'opencode' '-m' 'openai/gpt-5.6-sol' '--agent' 'reviewer-b' --json"; !strings.Contains(shimLog(t, base), want) {
		t.Errorf("log lacks %q:\n%s", want, shimLog(t, base))
	}
	if log := shimLog(t, base); strings.Contains(log, "claude -p") || strings.Contains(log, "opencode run") || strings.Contains(log, "--agent claude") {
		t.Errorf("inside Orca a role ran outside its own agent terminal:\n%s", log)
	}
}

// TestDispatchInsideOrcaSettlesOnlyOnProof maps each Dispatch status to its outcome: a failure or a proven stop fails the run, and anything else, including a signal while the worker runs, is reported as unknown so the workflow escalates instead of editing beside a live worker.
func TestDispatchInsideOrcaSettlesOnlyOnProof(t *testing.T) {
	base, prompt, wt := dispatchBase(t)
	orcaShim(t, base)
	ok(t, runModels(t, base, "set", "--preset", "performance"), "set performance")
	renderAgent(t, base, "claude", "executor", "opus", "high")
	args := []string{"executor", "--prompt-file", prompt, "--worktree", wt}
	for status, want := range map[string]string{
		"failed ready":         "run_failed",
		"circuit_broken ready": "run_failed",
		"dispatched stopped":   "run_failed",
		"archived ready":       "outcome_unknown",
	} {
		writeMode(t, filepath.Join(base, "status"), status+"\n", 0o644)
		released := strings.Count(shimLog(t, base), "worker-release")
		if r := runDispatch(t, base, orcaEnv("OPENCODE=1"), args...); r.code == 0 || reason(t, r) != want {
			t.Errorf("%s: exit %d, %s", status, r.code, r.stdout)
		}
		// A settled failure frees its terminal; an unknown outcome touches nothing.
		if got, wantReleased := strings.Count(shimLog(t, base), "worker-release")-released, map[bool]int{true: 1, false: 0}[want == "run_failed"]; got != wantReleased {
			t.Errorf("%s: %d releases, want %d", status, got, wantReleased)
		}
	}

	writeMode(t, filepath.Join(base, "status"), "dispatched ready\n", 0o644)
	before := strings.Count(shimLog(t, base), "worker-show")
	cmd, stdout := startDispatch(t, base, orcaEnv("OPENCODE=1"), args...)
	waitFor(t, "the first poll", func() bool { return strings.Count(shimLog(t, base), "worker-show") > before })
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	if !strings.Contains(stdout.String(), `"reason":"outcome_unknown"`) {
		t.Errorf("signalled while dispatched: %q", stdout.String())
	}
	if log := shimLog(t, base); strings.Contains(log, "worker-stop") || strings.Contains(log, "terminal close") {
		t.Errorf("dispatch stopped a worker without positive proof:\n%s", log)
	}
}

// TestDispatchWorkerStartReceipts closes the idle agent terminal on a clean refusal, releases a failed start, and reports a receipt that proves neither ready nor refused as unknown.
func TestDispatchWorkerStartReceipts(t *testing.T) {
	base, prompt, wt := dispatchBase(t)
	ok(t, runModels(t, base, "set", "--preset", "performance"), "set performance")
	renderAgent(t, base, "claude", "executor", "opus", "high")
	for receipt, want := range map[string]string{
		`{"ok":false,"error":{"code":"runtime_error"}}`:                       "orca_refused",
		`{"ok":true,"result":{"dispatchId":"d-1","state":"failed"}}`:          "orca_refused",
		`{"ok":true,"result":{"dispatchId":"d-1","state":"outcome_unknown"}}`: "outcome_unknown",
		`{"ok":true,"result":{"state":"ready"}}`:                              "outcome_unknown",
		`not json`:                                                            "outcome_unknown",
	} {
		writeShim(t, base, "orca", `case "$1 $2" in
"worktree current") echo '{"ok":true}' ;;
"orchestration run-current") echo '{"ok":true,"result":{"run":{"id":"r-0"}}}' ;;
"terminal create") echo '{"ok":true,"result":{"terminal":{"handle":"term-7"}}}' ;;
"orchestration worker-start") echo '`+receipt+`'; exit 1 ;;
esac`)
		if r := runDispatch(t, base, orcaEnv("OPENCODE=1"), "executor", "--prompt-file", prompt, "--worktree", wt); r.code == 0 || reason(t, r) != want {
			t.Errorf("%s: exit %d, %s", receipt, r.code, r.stdout)
		}
	}
	log := shimLog(t, base)
	if strings.Contains(log, "run-create") || !strings.Contains(log, "worker-start --run r-0") {
		t.Errorf("dispatch did not reuse the bound Run:\n%s", log)
	}
	if strings.Count(log, "orca terminal close --terminal term-7 --json") != 1 || strings.Count(log, "orca orchestration worker-release --dispatch d-1 --json") != 1 {
		t.Errorf("want one closed terminal for the refusal and one released worker for the failed start:\n%s", log)
	}
}

// TestDispatchResolvesTheOrcaExecutable honors ORCA_CLI_COMMAND ahead of the bare name.
func TestDispatchResolvesTheOrcaExecutable(t *testing.T) {
	base, prompt, wt := dispatchBase(t)
	orcaShim(t, base)
	if err := os.Rename(filepath.Join(base, "shims", "orca"), filepath.Join(base, "shims", "orca-custom")); err != nil {
		t.Fatal(err)
	}
	ok(t, runModels(t, base, "set", "--preset", "performance"), "set performance")
	renderAgent(t, base, "claude", "executor", "opus", "high")
	// No `orca` is on PATH any more, so the worker's output proves the Orca route ran through orca-custom.
	r := ok(t, runDispatch(t, base, []string{"OPENCODE=1", "ORCA_CLI_COMMAND=orca-custom"}, "executor", "--prompt-file", prompt, "--worktree", wt), "dispatch through orca-custom")
	if r.stdout != "worker said\n" {
		t.Errorf("dispatch ignored ORCA_CLI_COMMAND: stdout %q\n%s", r.stdout, shimLog(t, base))
	}
}

func TestDispatchFailuresExitWithAReason(t *testing.T) {
	base, prompt, wt := dispatchBase(t)
	args := []string{"executor", "--prompt-file", prompt, "--worktree", wt}

	renderAgent(t, base, "opencode", "executor", "opencode-go/deepseek-v4-flash", "")
	if r := runDispatch(t, base, []string{"CLAUDECODE=1"}, args...); r.code == 0 || reason(t, r) != "stale_install" {
		t.Errorf("agent rendered for another model: exit %d, %s", r.code, r.stdout)
	}
	renderAgent(t, base, "opencode", "executor", "opencode-go/glm-5.3", "high")
	if r := runDispatch(t, base, []string{"CLAUDECODE=1"}, args...); r.code == 0 || reason(t, r) != "stale_install" {
		t.Errorf("agent rendered for another effort: exit %d, %s", r.code, r.stdout)
	}

	// OpenCode lists no model, so the balanced executor is unavailable.
	writeShim(t, base, "opencode", `case "$1" in models) echo 'openai/gpt-5.6-sol' ;; esac`)
	if r := runDispatch(t, base, []string{"CLAUDECODE=1"}, args...); r.code == 0 || reason(t, r) != "model_unavailable" {
		t.Errorf("unlisted model: exit %d, %s", r.code, r.stdout)
	}

	if err := os.Remove(filepath.Join(base, "shims", "opencode")); err != nil {
		t.Fatal(err)
	}
	if r := runDispatch(t, base, []string{"CLAUDECODE=1"}, args...); r.code == 0 || reason(t, r) != "client_missing" {
		t.Errorf("missing opencode: exit %d, %s", r.code, r.stdout)
	}

	if r := runDispatch(t, base, []string{"CLAUDECODE=1"}, "reviewer-a", "--prompt-file", prompt, "--worktree", wt); r.code == 0 || reason(t, r) != "native_route" {
		t.Errorf("same-client role: exit %d, %s", r.code, r.stdout)
	}
}

func TestDispatchNeedsASession(t *testing.T) {
	base, prompt, wt := dispatchBase(t)
	for name, env := range map[string][]string{"none": nil, "both": {"CLAUDECODE=1", "OPENCODE=1"}} {
		r := runDispatch(t, base, env, "executor", "--prompt-file", prompt, "--worktree", wt)
		if r.code == 0 || reason(t, r) != "session_unknown" || !strings.Contains(r.stdout, "--session") {
			t.Errorf("%s: exit %d, %s", name, r.code, r.stdout)
		}
	}
	writeShim(t, base, "opencode", `case "$1" in models) echo 'opencode-go/glm-5.3' ;; run) echo ran ;; esac`)
	ok(t, runDispatch(t, base, nil, "executor", "--prompt-file", prompt, "--worktree", wt, "--session", "claude"), "explicit session")
}
