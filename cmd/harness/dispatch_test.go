package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// dispatchBase is a models sandbox with a prompt file, a worktree directory, a claude shim for the client lookup, and an OpenCode agent rendered for the balanced executor.
func dispatchBase(t *testing.T) (base, prompt, worktree string) {
	t.Helper()
	base = modelsBase(t)
	prompt = filepath.Join(base, "tmp", "prompt.md")
	writeMode(t, prompt, "review the diff", 0o644)
	worktree = filepath.Join(base, "wt")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatal(err)
	}
	// Orca runs the client inside the terminal it opens, so dispatch only looks this one up; a client dispatch ran itself would leave a line in the shim log.
	writeShim(t, base, "claude", "")
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
	if client == "opencode" {
		head += "mode: all\n"
	}
	if effort != "" {
		head += effortKey + effort + "\n"
	}
	writeMode(t, filepath.Join(dir, role+".md"), head+"---\n\nbody\n", 0o644)
}

// orcaEnv pins the Orca executable to the shim, since Linux outside an Orca terminal resolves to orca-ide. It names no session client: every role takes the same route whatever the session runs.
func orcaEnv(extra ...string) []string { return append([]string{"ORCA_CLI_COMMAND=orca"}, extra...) }

// ranAClient reports the shim log lines left by a client dispatch started itself, rather than quoted into the command Orca's terminal runs. `opencode models` is the model listing, not a launch.
func ranAClient(log string) []string {
	var lines []string
	for _, line := range strings.Split(log, "\n") {
		if strings.HasPrefix(line, "claude ") || (strings.HasPrefix(line, "opencode ") && !strings.HasPrefix(line, "opencode models")) {
			lines = append(lines, line)
		}
	}
	return lines
}

// orcaShim fakes an Orca runtime with no bound Run, whose worker-show reports the status and worker state written to base/status, and whose worker-abandon refuses when a third field there says refuse, and whose liveness verdict is the fourth field.
func orcaShim(t *testing.T, base string) {
	t.Helper()
	status := filepath.Join(base, "status")
	writeMode(t, status, "completed ready\n", 0o644)
	writeShim(t, base, "orca", `read status state abandon verdict < "`+status+`"
case "$1 $2" in
"worktree current") echo '{"ok":true}' ;;
"orchestration run-current") echo '{"ok":true,"result":{"run":null}}' ;;
"orchestration run-create") echo '{"ok":true,"result":{"run":{"id":"r-1"}}}' ;;
"terminal create") echo '{"ok":true,"result":{"terminal":{"handle":"term-7"}}}' ;;
"orchestration worker-start") echo '{"ok":true,"result":{"dispatchId":"d-1","state":"ready"}}' ;;
"orchestration worker-show") echo '{"ok":true,"result":{"dispatch":{"id":"d-1","status":"'"$status"'"},"worker":{"state":"'"$state"'"},"projection":{"liveness":{"verdict":"'"$verdict"'"}}}}' ;;
"orchestration worker-read") echo "worker said" ;;
"orchestration worker-release") echo '{"ok":true}' ;;
"orchestration worker-abandon") [ "$abandon" = refuse ] && exit 1; echo '{"ok":true}' ;;
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

// TestDispatchRunsEveryRoleThroughOrca proves the single route: each role starts as its own named agent in the terminal Orca opens and a supervised worker adopts, a role assigned to the session's own client included, and no client is ever started directly.
func TestDispatchRunsEveryRoleThroughOrca(t *testing.T) {
	base, prompt, wt := dispatchBase(t)
	orcaShim(t, base)
	ok(t, runModels(t, base, "set", "--preset", "diversity"), "set diversity")
	renderAgent(t, base, "claude", "executor", "sonnet", "")

	r := ok(t, runDispatch(t, base, orcaEnv(), "executor", "--prompt-file", prompt, "--worktree", wt), "claude executor")
	if r.stdout != "worker said\n" {
		t.Errorf("stdout = %q", r.stdout)
	}
	log := shimLog(t, base)
	for _, want := range []string{
		"orca orchestration run-create --objective harness dispatch executor --json",
		"orca terminal create --worktree path:" + wt + " --title harness dispatch executor --command 'claude' '--agent' 'executor' '--permission-mode' 'acceptEdits' '--allowedTools' 'Bash' '--disallowedTools' 'Bash(git commit:*)' 'Bash(git push:*)' 'Bash(git merge:*)' 'Bash(git worktree remove:*)' 'Bash(wt remove:*)' 'Bash(wt merge:*)' 'Bash(wt step commit:*)' 'Bash(gh:*)' --json",
		"orca orchestration worker-start --run r-1 --spec review the diff --worktree path:" + wt + " --terminal term-7 --json",
		"orca orchestration worker-read --dispatch d-1 --source auto",
		"orca orchestration worker-release --dispatch d-1 --json",
	} {
		if !strings.Contains(log, want) {
			t.Errorf("log lacks %q:\n%s", want, log)
		}
	}

	// A role on the client this session itself runs was refused as native; it now takes the same route as any other, and its rendered agent is still what carries the model.
	ok(t, withAgent(t, base, "claude", "reviewer-a", "inherit", "", func() result {
		return runDispatch(t, base, orcaEnv(), "reviewer-a", "--prompt-file", prompt, "--worktree", wt)
	}), "same-client role")
	if want := "orca terminal create --worktree path:" + wt + " --title harness dispatch reviewer-a --command 'claude' '--agent' 'reviewer-a' --json"; !strings.Contains(shimLog(t, base), want) {
		t.Errorf("log lacks %q:\n%s", want, shimLog(t, base))
	}

	renderAgent(t, base, "opencode", "reviewer-b", "openai/gpt-5.6-sol", "")
	ok(t, runDispatch(t, base, orcaEnv(), "reviewer-b", "--prompt-file", prompt, "--worktree", wt), "opencode role")
	if want := "orca terminal create --worktree path:" + wt + " --title harness dispatch reviewer-b --command 'opencode' '-m' 'openai/gpt-5.6-sol' '--agent' 'reviewer-b' --json"; !strings.Contains(shimLog(t, base), want) {
		t.Errorf("log lacks %q:\n%s", want, shimLog(t, base))
	}
	if ran := ranAClient(shimLog(t, base)); ran != nil {
		t.Errorf("a role ran outside its own agent terminal:\n%s", strings.Join(ran, "\n"))
	}
}

// TestDispatchWithoutOrcaLaunchesNothing proves the only runtime is proven before a worker launch: a missing Orca executable, a probe that exits nonzero, and a refusal Orca reports at exit 0 each fail with orca_unavailable carrying Orca's own answer and the recovery, and none of them opens a terminal, starts a worker, or reaches a client.
func TestDispatchWithoutOrcaLaunchesNothing(t *testing.T) {
	base, prompt, wt := dispatchBase(t)
	args := []string{"executor", "--prompt-file", prompt, "--worktree", wt}
	for _, c := range []struct {
		name, shim, want, recovery string
		env                        []string
	}{
		{"no Orca executable", "", "is not on PATH", "install or start Orca", []string{"ORCA_CLI_COMMAND=orca-absent"}},
		{"a probe that exits nonzero", `echo "no worktree here" >&2; exit 1`, "no worktree here", "create or open this branch's worktree in Orca", orcaEnv()},
		// Orca can refuse with ok:false at exit 0, which a gate reading only the exit code would pass through to run-create.
		{"a refusal at exit 0", `echo '{"ok":false,"error":{"code":"not_a_worktree"}}'`, "not_a_worktree", "create or open this branch's worktree in Orca", orcaEnv()},
	} {
		writeShim(t, base, "orca", c.shim)
		r := runDispatch(t, base, c.env, args...)
		// The recovery is asserted beside the evidence, so deleting the clause that says what to do fails here.
		if r.code == 0 || reason(t, r) != "orca_unavailable" || !strings.Contains(r.stdout, c.want) || !strings.Contains(r.stdout, c.recovery) {
			t.Errorf("%s: exit %d, %s", c.name, r.code, r.stdout)
		}
	}
	log := shimLog(t, base)
	if strings.Contains(log, "terminal create") || strings.Contains(log, "worker-start") {
		t.Errorf("an unavailable Orca still launched a worker:\n%s", log)
	}
	if ran := ranAClient(log); ran != nil {
		t.Errorf("an unavailable Orca still reached a client:\n%s", strings.Join(ran, "\n"))
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

// TestDispatchInsideOrcaSettlesOnlyOnProof maps each Dispatch status to its outcome: a failure or a proven stop fails the run, and anything else, including a signal while the worker runs, is reported as unknown so the workflow escalates instead of editing beside a live worker.
func TestDispatchInsideOrcaSettlesOnlyOnProof(t *testing.T) {
	base, prompt, wt := dispatchBase(t)
	orcaShim(t, base)
	ok(t, runModels(t, base, "set", "--preset", "performance"), "set performance")
	renderAgent(t, base, "claude", "executor", "opus", "high")
	args := []string{"executor", "--prompt-file", prompt, "--worktree", wt}
	// A settled failure frees its terminal; a proven stop of an unsettled Dispatch is fenced with worker-abandon, never released, and its terminal closed; an unknown outcome touches nothing.
	for status, want := range map[string]struct {
		reason                     string
		releases, abandons, closes int
	}{
		"failed ready":               {"run_failed", 1, 0, 0},
		"circuit_broken ready":       {"run_failed", 1, 0, 0},
		"dispatched stopped":         {"run_failed", 0, 1, 1},
		"dispatched abandoned":       {"run_failed", 0, 0, 1},
		"dispatched ready ok exited": {"run_failed", 0, 1, 1},
		"dispatched stopped refuse":  {"outcome_unknown", 0, 1, 0},
		"archived ready":             {"outcome_unknown", 0, 0, 0},
	} {
		writeMode(t, filepath.Join(base, "status"), status+"\n", 0o644)
		count := func() [3]int {
			log := shimLog(t, base)
			return [3]int{strings.Count(log, "worker-release"), strings.Count(log, "worker-abandon"), strings.Count(log, "terminal close")}
		}
		before := count()
		if r := runDispatch(t, base, orcaEnv(), args...); r.code == 0 || reason(t, r) != want.reason {
			t.Errorf("%s: exit %d, %s", status, r.code, r.stdout)
		}
		after := count()
		if got := [3]int{after[0] - before[0], after[1] - before[1], after[2] - before[2]}; got != [3]int{want.releases, want.abandons, want.closes} {
			t.Errorf("%s: releases, abandons, closes %v, want %d %d %d", status, got, want.releases, want.abandons, want.closes)
		}
	}

	writeMode(t, filepath.Join(base, "status"), "dispatched ready\n", 0o644)
	earlier := len(shimLog(t, base))
	before := strings.Count(shimLog(t, base), "worker-show")
	cmd, stdout := startDispatch(t, base, orcaEnv(), args...)
	waitFor(t, "the first poll", func() bool { return strings.Count(shimLog(t, base), "worker-show") > before })
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	if !strings.Contains(stdout.String(), `"reason":"outcome_unknown"`) {
		t.Errorf("signalled while dispatched: %q", stdout.String())
	}
	if log := shimLog(t, base)[earlier:]; strings.Contains(log, "worker-stop") || strings.Contains(log, "worker-abandon") || strings.Contains(log, "terminal close") {
		t.Errorf("dispatch stopped a worker without positive proof:\n%s", log)
	}
}

// TestDispatchWorkerStartReceipts closes the idle agent terminal on a clean refusal, releases a failed start, and reports a receipt that proves neither ready nor refused as unknown.
func TestDispatchWorkerStartReceipts(t *testing.T) {
	base, prompt, wt := dispatchBase(t)
	ok(t, runModels(t, base, "set", "--preset", "performance"), "set performance")
	renderAgent(t, base, "claude", "executor", "opus", "high")
	for receipt, want := range map[string]struct{ reason, identity string }{
		`{"ok":false,"error":{"code":"runtime_error"}}`:                       {"orca_refused", ""},
		`{"ok":true,"result":{"dispatchId":"d-1","state":"failed"}}`:          {"orca_refused", ""},
		`{"ok":true,"result":{"dispatchId":"d-1","state":"outcome_unknown"}}`: {"outcome_unknown", "orca dispatch d-1:"},
		`{"ok":true,"result":{"state":"ready"}}`:                              {"outcome_unknown", "terminal term-7"},
		`not json`:                                                            {"outcome_unknown", "terminal term-7"},
	} {
		writeShim(t, base, "orca", `case "$1 $2" in
"worktree current") echo '{"ok":true}' ;;
"orchestration run-current") echo '{"ok":true,"result":{"run":{"id":"r-0"}}}' ;;
"terminal create") echo '{"ok":true,"result":{"terminal":{"handle":"term-7"}}}' ;;
"orchestration worker-start") echo '`+receipt+`'; exit 1 ;;
esac`)
		r := runDispatch(t, base, orcaEnv(), "executor", "--prompt-file", prompt, "--worktree", wt)
		if r.code == 0 || reason(t, r) != want.reason {
			t.Errorf("%s: exit %d, %s", receipt, r.code, r.stdout)
		}
		if want.identity != "" && !strings.Contains(r.stdout, want.identity) {
			t.Errorf("%s: an unknown outcome named nothing a recovery can address: %s", receipt, r.stdout)
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
	r := ok(t, runDispatch(t, base, []string{"ORCA_CLI_COMMAND=orca-custom"}, "executor", "--prompt-file", prompt, "--worktree", wt), "dispatch through orca-custom")
	if r.stdout != "worker said\n" {
		t.Errorf("dispatch ignored ORCA_CLI_COMMAND: stdout %q\n%s", r.stdout, shimLog(t, base))
	}
}

func TestDispatchFailuresExitWithAReason(t *testing.T) {
	base, prompt, wt := dispatchBase(t)
	orcaShim(t, base)
	args := []string{"executor", "--prompt-file", prompt, "--worktree", wt}

	renderAgent(t, base, "opencode", "executor", "opencode-go/deepseek-v4-flash", "")
	if r := runDispatch(t, base, orcaEnv(), args...); r.code == 0 || reason(t, r) != "stale_install" {
		t.Errorf("agent rendered for another model: exit %d, %s", r.code, r.stdout)
	}
	renderAgent(t, base, "opencode", "executor", "opencode-go/glm-5.3", "high")
	if r := runDispatch(t, base, orcaEnv(), args...); r.code == 0 || reason(t, r) != "stale_install" {
		t.Errorf("agent rendered for another effort: exit %d, %s", r.code, r.stdout)
	}
	// An agent rendered before mode: all would run as OpenCode's default agent.
	writeMode(t, filepath.Join(base, "home", ".config", "opencode", "agents", "executor.md"), "---\ndescription: \"x\"\nmode: subagent\nmodel: opencode-go/glm-5.3\n---\n\nbody\n", 0o644)
	if r := runDispatch(t, base, orcaEnv(), args...); r.code == 0 || reason(t, r) != "stale_install" {
		t.Errorf("agent rendered as a subagent: exit %d, %s", r.code, r.stdout)
	}

	// OpenCode lists no model, so the balanced executor is unavailable.
	writeShim(t, base, "opencode", `case "$1" in models) echo 'openai/gpt-5.6-sol' ;; esac`)
	if r := runDispatch(t, base, orcaEnv(), args...); r.code == 0 || reason(t, r) != "model_unavailable" {
		t.Errorf("unlisted model: exit %d, %s", r.code, r.stdout)
	}

	if err := os.Remove(filepath.Join(base, "shims", "opencode")); err != nil {
		t.Fatal(err)
	}
	if r := runDispatch(t, base, orcaEnv(), args...); r.code == 0 || reason(t, r) != "client_missing" {
		t.Errorf("missing opencode: exit %d, %s", r.code, r.stdout)
	}

	// --session only ever chose between routes, and there is one route now.
	if r := runDispatch(t, base, orcaEnv(), "executor", "--prompt-file", prompt, "--worktree", wt, "--session", "claude"); r.code == 0 || reason(t, r) != "bad_input" {
		t.Errorf("--session: exit %d, %s", r.code, r.stdout)
	}
}
