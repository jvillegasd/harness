package models

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"time"
)

const dispatchUsage = "usage: harness dispatch <role> --prompt-file <path> [--worktree <path>]"

// executorDenies carries the executor's parent-owned actions into a Claude run, where a CLI deny rule can name a command; the rendered agent can only state them.
var executorDenies = []string{"Bash(git commit:*)", "Bash(git push:*)", "Bash(git merge:*)", "Bash(git worktree remove:*)", "Bash(wt remove:*)", "Bash(wt merge:*)", "Bash(wt step commit:*)", "Bash(gh:*)"}

// pollEvery is how often an Orca worker is checked for settlement, and workerTimeout how long any dispatched run may take.
const pollEvery, workerTimeout = 5 * time.Second, time.Hour

// terminalCreateTimeout is how long Orca may take to open the agent terminal; a variable so a test can shorten it.
var terminalCreateTimeout = time.Minute

// failure is a dispatch that could not run, printed as JSON so the calling workflow can act on its reason. A reason never authorizes another backend: it decides between retrying this dispatch, repairing the install, doing permitted work inline, and escalating.
type failure struct {
	Reason string `json:"reason"`
	Detail string `json:"detail"`
}

func (f failure) Error() string { return f.Reason + ": " + f.Detail }

// Dispatch runs `harness dispatch`: it launches a role as its own named agent in an Orca worker and prints that worker's output. Orca is the only runtime, so a role assigned to the session's own client takes the same path as one assigned to the other.
func Dispatch(args []string, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, workerTimeout)
	defer cancel()
	out, err := dispatch(ctx, args)
	if err != nil {
		var f failure
		if !errors.As(err, &f) {
			f = failure{"run_failed", err.Error()}
		}
		_ = json.NewEncoder(stdout).Encode(f)
		fmt.Fprintf(stderr, "harness dispatch: %v\n", f)
		return 1
	}
	_, _ = stdout.Write(out)
	return 0
}

func dispatch(ctx context.Context, args []string) ([]byte, error) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return nil, failure{"bad_input", dispatchUsage}
	}
	role := args[0]
	fs := flag.NewFlagSet("dispatch", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	promptFile := fs.String("prompt-file", "", "")
	worktree := fs.String("worktree", ".", "")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() > 0 || *promptFile == "" {
		return nil, failure{"bad_input", dispatchUsage}
	}
	if err := knownRole(role); err != nil {
		return nil, failure{"bad_input", err.Error()}
	}
	prompt, err := os.ReadFile(*promptFile)
	if err != nil {
		return nil, failure{"bad_input", err.Error()}
	}
	dir, err := filepath.Abs(*worktree)
	if err != nil {
		return nil, failure{"bad_input", err.Error()}
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return nil, failure{"bad_input", "worktree " + dir + " is not a directory"}
	}
	if err := requireOrca(ctx, dir); err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	choice, err := Load(StatePath(home))
	if err != nil {
		return nil, err
	}
	a := choice.Resolve()[role].Assignment
	if _, err := exec.LookPath(a.Client); err != nil {
		return nil, failure{"client_missing", a.Client + " is not on PATH"}
	}
	if a.Client == "opencode" {
		listed, err := OpenCodeModels()
		if err != nil || !slices.Contains(listed, a.Model) {
			return nil, failure{"model_unavailable", "opencode does not list " + a.Model}
		}
	}
	if err := renderedMatches(home, role, a); err != nil {
		return nil, err
	}
	return viaOrca(ctx, role, a, string(prompt), dir)
}

// requireOrca proves the only runtime can take this worktree before anything is launched, so an unavailable Orca names what to do instead of failing part way through a launch that leaves a terminal behind. Which recovery a caller owes is the role's: a workflow may do an executor's slice inline and disclose it, while a required review escalates rather than becoming inline.
func requireOrca(ctx context.Context, dir string) error {
	cli := orcaCLI()
	if _, err := exec.LookPath(cli); err != nil {
		return failure{"orca_unavailable", cli + " is not on PATH, and Orca is the only runtime for a delegated role; install or start Orca, or set ORCA_CLI_COMMAND, then retry"}
	}
	probe := exec.CommandContext(ctx, cli, "worktree", "current", "--json")
	probe.Dir = dir
	if err := probe.Run(); err != nil {
		return failure{"orca_unavailable", dir + " is not an Orca worktree, and Orca is the only runtime for a delegated role; create or open this branch's worktree in Orca, then retry"}
	}
	return nil
}

// renderedMatches refuses a role whose installed agent was rendered for another model or effort. The agent's frontmatter is the only place either client takes them from, since the policy forbids a launch-time model override, so a `harness models set` without a reinstall would otherwise run the old assignment.
func renderedMatches(home, role string, a Assignment) error {
	path, effortKey := filepath.Join(home, ".claude", "agents", role+".md"), "effort: "
	if a.Client == "opencode" {
		config := os.Getenv("XDG_CONFIG_HOME")
		if config == "" {
			config = filepath.Join(home, ".config")
		}
		path, effortKey = filepath.Join(config, "opencode", "agents", role+".md"), "reasoningEffort: "
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return failure{"agent_missing", "no rendered " + a.Client + " agent for " + role + "; run harness install"}
	}
	head := ""
	if parts := strings.SplitN(string(data), "---\n", 3); strings.HasPrefix(string(data), "---\n") && len(parts) == 3 {
		head = parts[1]
	}
	lines := strings.Split(head, "\n")
	effort := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, effortKey) })
	rendered := ""
	if effort >= 0 {
		rendered = strings.TrimPrefix(lines[effort], effortKey)
	}
	if !slices.Contains(lines, "model: "+a.Model) || rendered != a.Effort {
		return failure{"stale_install", "the installed " + a.Client + " " + role + " agent was not rendered for " + a.Model + " at effort " + effortOrInherit(a.Effort) + "; run harness install"}
	}
	// A mode: subagent agent launched by --agent runs as OpenCode's default agent with none of its controls.
	if a.Client == "opencode" && !slices.Contains(lines, "mode: all") {
		return failure{"stale_install", "the installed opencode " + role + " agent is not mode: all; run harness install"}
	}
	return nil
}

// launchArgs is the argv that starts the role as its own named agent on its client, inside the terminal Orca opens. The executor gets edits and Bash, minus the actions its parent owns.
func launchArgs(role string, a Assignment) []string {
	if a.Client == "opencode" {
		return []string{"opencode", "-m", a.Model, "--agent", role}
	}
	// No --model or --effort: they would override the agent's frontmatter, which renderedMatches proved current. Whether a main-thread --agent session applies the frontmatter's effort, not only its model, is unverified.
	argv := []string{"claude", "--agent", role}
	if role == "executor" {
		argv = append(append(argv, "--permission-mode", "acceptEdits", "--allowedTools", "Bash", "--disallowedTools"), executorDenies...)
	}
	return argv
}

// orcaCLI is the Orca executable, chosen the way the Orca orchestration skill says: Linux's bare `orca` outside an Orca terminal is the GNOME screen reader.
func orcaCLI() string {
	switch {
	case os.Getenv("ORCA_CLI_COMMAND") != "":
		return os.Getenv("ORCA_CLI_COMMAND")
	case os.Getenv("ORCA_DEV_REPO_ROOT") != "":
		return "orca-dev"
	case runtime.GOOS == "linux" && os.Getenv("ORCA_PANE_KEY") == "":
		return "orca-ide"
	}
	return "orca"
}

// viaOrca runs the role as its own agent in an Orca terminal in dir, which a supervised worker adopts, and returns the worker's output once its Dispatch settles. worker-start --agent launches only a known TUI agent and cannot name a role agent, so both clients take Orca's documented terminal-then-worker path.
func viaOrca(ctx context.Context, role string, a Assignment, prompt, dir string) ([]byte, error) {
	where := "path:" + dir
	var bound struct {
		Result struct {
			Run *struct{ ID string }
		}
	}
	if err := orcaJSON(ctx, &bound, "orchestration", "run-current", "--json"); err != nil {
		return nil, err
	}
	if bound.Result.Run == nil {
		if err := orcaJSON(ctx, &bound, "orchestration", "run-create", "--objective", "harness dispatch "+role, "--json"); err != nil {
			return nil, err
		}
	}
	if bound.Result.Run == nil || bound.Result.Run.ID == "" {
		return nil, failure{"orca_refused", "orca reported no Run id"}
	}
	var quoted []string
	for _, arg := range launchArgs(role, a) {
		quoted = append(quoted, "'"+strings.ReplaceAll(arg, "'", `'\''`)+"'")
	}
	var created struct {
		Result struct {
			Terminal struct{ Handle string }
		}
	}
	// Immune to signals, so a signal cannot lose the handle of a terminal Orca already opened; the guard below closes it. Bounded, so a hung Orca cannot hold dispatch forever. The title is Orca's low-level topology recipe, and what makes a terminal whose handle the timeout lost findable: Orca's recovery guide says inspect a lost mutation, never replay it blind.
	title := "harness dispatch " + role
	createCtx, cancelCreate := context.WithTimeout(context.Background(), terminalCreateTimeout)
	defer cancelCreate()
	if err := orcaJSON(createCtx, &created, "terminal", "create", "--worktree", where, "--title", title, "--command", strings.Join(quoted, " "), "--json"); err != nil {
		if createCtx.Err() != nil {
			return nil, failure{"timeout", fmt.Sprintf("orca terminal create did not answer within %s; a %q terminal may be open, so inspect `%s terminal list --worktree %s --json` and close it before retrying", terminalCreateTimeout, title, orcaCLI(), where)}
		}
		return nil, err
	}
	handle := created.Result.Terminal.Handle
	closeTerminal := func() { _ = exec.Command(orcaCLI(), "terminal", "close", "--terminal", handle, "--json").Run() }
	if ctx.Err() != nil {
		closeTerminal()
		return nil, failure{"timeout", "dispatch was stopped before a worker started"}
	}
	// ponytail: the spec is one argument, so Linux caps it at 128 KiB; pass a file when a prompt outgrows that.
	out, err := exec.CommandContext(ctx, orcaCLI(), "orchestration", "worker-start", "--run", bound.Result.Run.ID, "--spec", prompt, "--worktree", where, "--terminal", handle, "--json").Output()
	var started struct {
		Ok     *bool
		Result struct{ DispatchId, State string }
	}
	_ = json.Unmarshal(out, &started)
	id := started.Result.DispatchId
	switch {
	case err == nil && started.Ok != nil && *started.Ok && started.Result.State == "ready" && id != "":
	case started.Ok != nil && !*started.Ok && id == "":
		// A clean refusal is preflight: no Dispatch exists, so only the idle agent terminal needs closing.
		closeTerminal()
		return nil, failure{"orca_refused", "worker-start: " + strings.TrimSpace(string(out))}
	case started.Result.State == "failed" && id != "":
		// A start that failed before its agent was ready still owns the terminal; its receipt names worker-release.
		release(id)
		return nil, failure{"orca_refused", "worker-start failed: " + strings.TrimSpace(string(out))}
	default:
		return nil, failure{"outcome_unknown", "worker-start did not prove a ready or refused worker; one may be running: " + strings.TrimSpace(string(out))}
	}
	for {
		var shown struct {
			Result struct {
				Dispatch   struct{ Status string }
				Worker     struct{ State string }
				Projection struct {
					Liveness struct{ Verdict string }
				}
			}
		}
		// Once a worker may exist, every unproven outcome is unknown, never a reason to fall back beside it.
		if err := orcaJSON(ctx, &shown, "orchestration", "worker-show", "--dispatch", id, "--json"); err != nil {
			return nil, failure{"outcome_unknown", "orca dispatch " + id + " could not be observed; the worker may still be running: " + err.Error()}
		}
		r := shown.Result
		switch r.Dispatch.Status {
		case "completed":
			return readAndRelease(ctx, id)
		case "failed", "circuit_broken":
			release(id)
			return nil, failure{"run_failed", "orca dispatch " + id + " " + r.Dispatch.Status}
		case "pending", "dispatched":
			// Only positive proof the agent stopped ends the wait early: Orca's guide never reads a stall from absence. An unsettled Dispatch is never released; it is fenced with worker-abandon so a late worker_done cannot land beside the fallback.
			if slices.Contains([]string{"failed", "stopped", "abandoned"}, r.Worker.State) || r.Projection.Liveness.Verdict == "exited" {
				if r.Worker.State != "abandoned" && orcaJSON(ctx, &struct{}{}, "orchestration", "worker-abandon", "--dispatch", id, "--json") != nil {
					return nil, failure{"outcome_unknown", "orca worker " + id + " stopped without reporting worker_done and could not be abandoned; the worker may still be running"}
				}
				// Fenced and proven stopped, so its terminal can no longer settle; close it so each fallback does not leave an agent TUI in the worktree.
				closeTerminal()
				return nil, failure{"run_failed", "orca worker " + id + " stopped without reporting worker_done and was abandoned"}
			}
		default:
			return nil, failure{"outcome_unknown", fmt.Sprintf("orca dispatch %s reports status %q; the worker may still be running", id, r.Dispatch.Status)}
		}
		select {
		case <-ctx.Done():
			// Absence never authorizes a stop, so the worker keeps running and the workflow escalates rather than falling back beside it.
			return nil, failure{"outcome_unknown", "orca dispatch " + id + " had not settled when dispatch was stopped; the worker may still be running"}
		case <-time.After(pollEvery):
		}
	}
}

// readAndRelease returns a completed worker's output, then releases its terminal.
func readAndRelease(ctx context.Context, id string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, orcaCLI(), "orchestration", "worker-read", "--dispatch", id, "--source", "auto").Output()
	if err != nil {
		// Unreleased, so the completed output stays readable with this worker-read and the worker is released by hand.
		return nil, failure{"read_failed", "orca dispatch " + id + " completed but worker-read failed (" + err.Error() + "); read it with `orca orchestration worker-read --dispatch " + id + " --source auto`, then release it"}
	}
	release(id)
	return out, nil
}

// release frees a settled worker's terminal. A failed release leaves only that settled terminal open, which worker-list reports as reclaimable.
func release(id string) {
	_ = exec.Command(orcaCLI(), "orchestration", "worker-release", "--dispatch", id, "--json").Run()
}

// orcaJSON runs one orca command and decodes its JSON envelope; a nonzero exit or ok:false is Orca refusing.
func orcaJSON(ctx context.Context, v any, args ...string) error {
	cmd := exec.CommandContext(ctx, orcaCLI(), args...)
	// A child that keeps stdout open would otherwise hold Output past the kill.
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	var envelope struct{ Ok bool }
	if err != nil || json.Unmarshal(out, &envelope) != nil || !envelope.Ok {
		return failure{"orca_refused", fmt.Sprintf("orca %s %s: %s", args[0], args[1], strings.TrimSpace(string(out)))}
	}
	return json.Unmarshal(out, v)
}
