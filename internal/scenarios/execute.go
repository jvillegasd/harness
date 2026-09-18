package scenarios

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// installForgeGuard writes a disposable `gh` that logs and refuses, so a scenario cannot reach a forge. Under a publication fixture it is a substitute instead: it logs and succeeds, answering `pr create` with a placeholder URL, so the workflow proceeds and the log is what gets judged. A shim that cannot be written fails the run rather than leaving the real forge reachable.
func installForgeGuard(workspace string, substitute bool) (shim, log string, err error) {
	shim = filepath.Join(workspace, "shim")
	if err := os.MkdirAll(shim, 0o755); err != nil {
		return "", "", err
	}
	log = filepath.Join(workspace, "forge.log")
	script := "#!/bin/sh\nprintf \"%s\\n\" \"$*\" >> \"" + log + "\"\nexit 1\n"
	if substitute {
		script = "#!/bin/sh\nprintf \"%s\\n\" \"$*\" >> \"" + log + "\"\n" +
			"if [ \"$1 $2\" = \"pr create\" ]; then echo \"https://forge.invalid/pull/$(grep -c '^pr create' \"" + log + "\")\"; fi\n"
	}
	if err := os.WriteFile(filepath.Join(shim, "gh"), []byte(script), 0o755); err != nil {
		return "", "", err
	}
	return shim, log, nil
}

// forgeEnv isolates a scenario from real forges and remotes. Local is set only under a publication fixture, and admits the file transport the seeded origin needs.
func forgeEnv(shim string, local bool) []string {
	env := os.Environ()
	path := shim + string(os.PathListSeparator) + os.Getenv("PATH")
	set := map[string]string{
		"PATH":                path,
		"GH_CONFIG_DIR":       filepath.Join(filepath.Dir(shim), "gh-config"),
		"GH_TOKEN":            "",
		"GITHUB_TOKEN":        "",
		"GIT_TERMINAL_PROMPT": "0",
		"GIT_ASKPASS":         "/bin/false",
		"SSH_ASKPASS":         "/bin/false",
		"GIT_SSH_COMMAND":     "/bin/false",
		"GIT_ALLOW_PROTOCOL":  "none",
	}
	if local {
		set["GIT_ALLOW_PROTOCOL"] = "file"
	}
	var out []string
	seen := map[string]bool{}
	for _, item := range env {
		key, _, _ := strings.Cut(item, "=")
		if _, ok := set[key]; ok {
			out = append(out, key+"="+set[key])
			seen[key] = true
			continue
		}
		out = append(out, item)
	}
	for key, value := range set {
		if !seen[key] {
			out = append(out, key+"="+value)
		}
	}
	return out
}

func gitRemotes(repo string) []string {
	cmd := exec.Command("git", "-C", repo, "remote")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var remotes []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) != "" {
			remotes = append(remotes, line)
		}
	}
	return remotes
}

func jsonl(items []any) string {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	for _, item := range items {
		_ = encoder.Encode(item)
	}
	return out.String()
}

// runCheck runs a scenario's check command, or its internal probe, recording the same events a live tool call would.
func runCheck(scenario map[string]any, repo, root string, env []string, events, transcript *[]any) int {
	command := getString(scenario, "check")
	*events = append(*events, map[string]any{"type": "command", "command": command})
	var stdout, stderr string
	code := 0
	if name, ok := strings.CutPrefix(command, "probe:"); ok {
		if err := runProbe(name, repo, root); err != nil {
			stderr = err.Error()
			code = 1
		}
	} else {
		cmd := exec.Command("/bin/sh", "-c", command)
		cmd.Dir, cmd.Env = repo, env
		var outBuf, errBuf bytes.Buffer
		cmd.Stdout, cmd.Stderr = &outBuf, &errBuf
		err := cmd.Run()
		stdout, stderr = outBuf.String(), errBuf.String()
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else if err != nil {
			stderr = err.Error()
			code = 1
		}
	}
	*events = append(*events, map[string]any{"type": "exit", "code": code})
	*events = append(*events, map[string]any{"type": "tool_result", "content": stdout + stderr})
	*transcript = append(*transcript, map[string]any{"stream": "stdout", "text": stdout})
	*transcript = append(*transcript, map[string]any{"stream": "stderr", "text": stderr})
	return code
}

func replay(fixture, repo string, events, transcript *[]any) error {
	source := filepath.Join(fixture, "repo")
	if info, err := os.Stat(source); err == nil && info.IsDir() {
		if err := copyTree(source, repo); err != nil {
			return err
		}
	}
	for _, pair := range []struct {
		name string
		sink *[]any
	}{{"events.jsonl", events}, {"transcript.jsonl", transcript}} {
		raw, err := os.ReadFile(filepath.Join(fixture, pair.name))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			decoded, err := decodeJSON([]byte(line))
			if err != nil {
				continue
			}
			*pair.sink = append(*pair.sink, decoded)
		}
	}
	return nil
}

func copyTree(source, dest string) error {
	return filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, raw, 0o644)
	})
}

func runLive(scenario map[string]any, repo string, env []string, events, transcript *[]any) (int, error) {
	client := getString(scenario, "client")
	if client == "" {
		client = "claude"
	}
	if client != "claude" {
		return 0, fmt.Errorf("unsupported client: %s", client)
	}
	*events = append(*events, map[string]any{"type": "session-start", "client": client})
	cmd := exec.Command("claude", "-p", "--output-format", "stream-json", "--verbose", getString(scenario, "prompt"))
	cmd.Dir, cmd.Env = repo, env
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &outBuf, &errBuf
	err := cmd.Run()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		return 0, err
	}
	*events = append(*events, map[string]any{"type": "session-end", "code": code})
	for _, line := range strings.Split(outBuf.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		decoded, err := decodeJSON([]byte(line))
		if err != nil {
			*events = append(*events, map[string]any{"type": "output", "text": line})
			continue
		}
		*events = append(*events, decoded)
	}
	*transcript = append(*transcript, map[string]any{"stream": "stdout", "text": outBuf.String()})
	*transcript = append(*transcript, map[string]any{"stream": "stderr", "text": errBuf.String()})
	return code, nil
}

// defaultCap bounds paid sessions in a batch that never set --cap.
const defaultCap = 32

// claimSession refuses a paid session once the batch's recorded paid attempts reach its cap. The cap is retained in the batch on first use, so it cannot be raised mid-batch; a new --out directory is a new batch and a new budget. The count comes from the attempts themselves: failed and interrupted sessions count, model-unavailable ones do not, and nothing is ever deleted to free budget.
func claimSession(outRoot string, cap *int) error {
	path := filepath.Join(outRoot, batchFile)
	data, err := readObject(path)
	if err != nil {
		data = map[string]any{}
	}
	switch {
	case data["cap"] == nil:
		data["cap"] = defaultCap
		if cap != nil {
			data["cap"] = *cap
		}
		if err := os.MkdirAll(outRoot, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(indentJSON(map[string]any{"cap": data["cap"]})), 0o644); err != nil {
			return err
		}
	case cap != nil && floatOf(data["cap"]) != float64(*cap):
		return fmt.Errorf("batch cap is retained at %s; start a new batch to change it", formatNumber(data["cap"]))
	}
	used := paidSessions(outRoot)
	if float64(used) >= floatOf(data["cap"]) {
		return fmt.Errorf("per-batch session cap reached (%d/%s)", used, formatNumber(data["cap"]))
	}
	return nil
}

// paidSessions counts the batch's live attempts that reached a model.
func paidSessions(outRoot string) int {
	provenances, _ := filepath.Glob(filepath.Join(outRoot, "*", "*", "provenance.json"))
	count := 0
	for _, path := range provenances {
		provenance, err := readObject(path)
		if err != nil || !strings.HasPrefix(getString(provenance, "executor"), "live:") {
			continue
		}
		if getString(readMetrics(filepath.Dir(path)), "terminal_status") != "model-unavailable" {
			count++
		}
	}
	return count
}

// sessionStatus is terminalStatus, except that a failed session whose client reported the model as not found is model-unavailable, which spends no budget.
func sessionStatus(code *int, events []any) string {
	status := terminalStatus(code)
	if status != "failed" {
		return status
	}
	for _, raw := range events {
		event, _ := raw.(map[string]any)
		if getString(event, "error") == "model_not_found" {
			return "model-unavailable"
		}
	}
	return status
}

func attemptPaths(outRoot, scenarioID string) []string {
	root := filepath.Join(outRoot, scenarioID)
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() {
			paths = append(paths, filepath.Join(root, entry.Name()))
		}
	}
	sort.Strings(paths)
	return paths
}

func runOne(scenario map[string]any, root, outRoot, fixturesRoot string, allowPaid bool, cap *int) (string, string, error) {
	digest := scenarioDigest(scenario, root, fixturesRoot)
	fixture := filepath.Join(fixturesRoot, getString(scenario, "id"))
	fixtureInfo, fixtureErr := os.Stat(fixture)
	hasFixture := fixtureErr == nil && fixtureInfo.IsDir()
	if mode(scenario) == "live" && !hasFixture && !allowPaid {
		return "not-run", "live scenario: pass --allow-paid or provide a recording", nil
	}
	workspace, err := os.MkdirTemp("", "scenario-"+getString(scenario, "id")+"-")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(workspace)
	repo := filepath.Join(workspace, "repo")
	if err := os.Mkdir(repo, 0o755); err != nil {
		return "", "", err
	}
	init := exec.Command("git", "init", "-q")
	init.Dir = repo
	_ = init.Run()
	var seeded *publication
	if truthy(scenario["publication"]) {
		if seeded, err = seedPublication(workspace, repo); err != nil {
			return "", "", err
		}
	}
	shim, forgeLog, err := installForgeGuard(workspace, seeded != nil)
	if err != nil {
		return "", "", err
	}
	env := forgeEnv(shim, seeded != nil)
	env = append(env, "HARNESS_ROOT="+root, "SCENARIO_REPO="+repo)
	before := snapshot(repo)
	fixturesBefore := treeDigest(fixture)
	var events, transcript []any
	var checkCode, liveCode *int

	started := time.Now()
	switch {
	case mode(scenario) == "offline":
		code := runCheck(scenario, repo, root, env, &events, &transcript)
		checkCode = &code
	case hasFixture:
		if err := replay(fixture, repo, &events, &transcript); err != nil {
			return "", "", err
		}
	default:
		if err := claimSession(outRoot, cap); err != nil {
			return "", "", err
		}
		code, err := runLive(scenario, repo, env, &events, &transcript)
		if err != nil {
			return "", "", err
		}
		liveCode = &code
	}
	wallSeconds := time.Since(started).Seconds()

	after := snapshot(repo)
	var forgeCalls []string
	if raw, err := os.ReadFile(forgeLog); err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.TrimSpace(line) != "" {
				forgeCalls = append(forgeCalls, line)
			}
		}
	}
	var effects *sideEffects
	if seeded != nil {
		if effects, err = seeded.measure(repo); err != nil {
			return "", "", err
		}
	}
	entries := evaluateArtifacts(scenario, before, after, repo, forgeCalls, fixturesBefore == treeDigest(fixture), effects)
	if checkCode != nil {
		entries = append(entries, entry("check-exit", *checkCode == 0, "exit "+strconv.Itoa(*checkCode)))
	}
	if liveCode != nil {
		entries = append(entries, entry("session-exit", *liveCode == 0, "exit "+strconv.Itoa(*liveCode)))
	}
	if seeded != nil {
		foreign := seeded.foreignRemotes(repo)
		entries = append(entries, entry("local-origin-only", len(foreign) == 0, strings.Join(foreign, "; ")))
	} else {
		remotes := gitRemotes(repo)
		entries = append(entries, entry("no-git-remote", len(remotes) == 0, strings.Join(remotes, "; ")))
	}

	now := time.Now().UTC()
	stamp := now.Format("20060102T150405") + fmt.Sprintf("%06dZ", now.Nanosecond()/1000)
	attempt := filepath.Join(outRoot, getString(scenario, "id"), stamp+"-"+digest[:8])
	if err := os.MkdirAll(attempt, 0o755); err != nil {
		return "", "", err
	}
	ok := true
	results := make([]any, 0, len(entries))
	for _, item := range entries {
		if !truthy(item["ok"]) {
			ok = false
		}
		results = append(results, item)
	}
	write := func(name, text string) error {
		return os.WriteFile(filepath.Join(attempt, name), []byte(text), 0o644)
	}
	if err := write("prompts.txt", strings.TrimRight(getString(scenario, "prompt"), " \t\r\n\f\v")+"\n"); err != nil {
		return "", "", err
	}
	executor := "offline"
	switch {
	case mode(scenario) == "offline":
		executor = "offline"
	case hasFixture:
		executor = "recording"
	default:
		client := getString(scenario, "client")
		if client == "" {
			client = "claude"
		}
		executor = "live:" + client
	}
	provenance := map[string]any{
		"scenario":     getString(scenario, "id"),
		"input_digest": digest,
		"recorded_at":  time.Now().UTC().Format("2006-01-02T15:04:05.000000"),
		"executor":     executor,
	}
	if err := write("provenance.json", indentJSON(provenance)); err != nil {
		return "", "", err
	}
	if seeded != nil {
		if err := write("forge.log", strings.Join(append(forgeCalls, ""), "\n")); err != nil {
			return "", "", err
		}
	}
	if err := write("events.jsonl", jsonl(events)); err != nil {
		return "", "", err
	}
	if err := write("artifacts.json", indentJSON(map[string]any{"ok": ok, "results": results})); err != nil {
		return "", "", err
	}
	if err := write("transcript.jsonl", jsonl(transcript)); err != nil {
		return "", "", err
	}
	var exitCode *int
	if checkCode != nil {
		exitCode = checkCode
	} else {
		exitCode = liveCode
	}
	status := sessionStatus(exitCode, events)
	if err := write("metrics.json", indentJSON(buildMetrics(scenario, events, wallSeconds, status))); err != nil {
		return "", "", err
	}
	state, reasons := evaluate(attempt, scenario, digest)
	if err := write("status.json", indentJSON(map[string]any{"state": state, "reasons": reasons})); err != nil {
		return "", "", err
	}
	return state, attempt, nil
}

func indentJSON(value any) string {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return ""
	}
	return string(encoded) + "\n"
}
