// Package scenarios runs artifact-based scenarios for critical harness behavior, replacing the Python runner.
//
// Each scenario declares the artifacts and assertions it expects before any model session runs, executes in a disposable repository with a forge shim on PATH, and retains one attempt directory holding prompts, input provenance, tool events, artifact results, and transcript evidence. An attempt passes only when its artifact checks pass and a transcript review record exists. Changed scenario or harness inputs mark an older attempt stale.
//
// Paid model execution is off unless --allow-paid is passed with a retained per-batch session cap. Local tooling success is reported separately from behavioral acceptance.
package scenarios

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

var requiredEvidence = []string{
	"provenance.json",
	"prompts.txt",
	"events.jsonl",
	"artifacts.json",
	"transcript.jsonl",
	"metrics.json",
}

var artifactKeys = []string{"tree_unchanged", "fixtures_unchanged", "present", "absent", "contains"}

var stateOrder = []string{"pass", "fail", "interrupted", "stale", "unreviewed", "incomplete", "unavailable"}

const batchFile = "batch.json"

const usageText = `Usage: harness scenarios <command> [flags]

Commands:
  validate   Reject the matrix before any session runs
  run        Execute the declared scenarios and retain one attempt each
  report     Report local tooling and behavioral acceptance separately
  compare    Compare two retained batches, pass rates before efficiency

Flags:
  --matrix <path>   Matrix declaration (default <root>/harness/scenarios/matrix.json)
  --out <path>      Attempt directory (default <root>/harness/scenarios/attempts)
  --root <path>     Harness checkout the harness_inputs resolve against (default .)
  run:     --scenario <id> --fixtures <path> --allow-paid --max-sessions <n>
  compare: --before <dir> --after <dir>
`

// Command runs `harness scenarios` and returns the exit code.
func Command(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usageText)
		return 2
	}
	if args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprint(stdout, usageText)
		return 0
	}
	sub, rest := args[0], args[1:]
	flags := newFlags()
	matrix := flags.String("matrix", "", "")
	out := flags.String("out", "", "")
	rootFlag := flags.String("root", "", "")
	var scenario, fixtures, before, after *string
	var allowPaid *bool
	var maxSessions *int
	switch sub {
	case "validate", "report":
	case "run":
		scenario = flags.String("scenario", "", "")
		fixtures = flags.String("fixtures", "", "")
		allowPaid = flags.Bool("allow-paid", false, "")
		maxSessions = flags.Int("max-sessions", 0, "")
	case "compare":
		before = flags.String("before", "", "")
		after = flags.String("after", "", "")
	default:
		fmt.Fprintf(stderr, "harness scenarios: unknown command %q; run harness scenarios --help\n", sub)
		return 2
	}
	if err := flags.Parse(rest); err != nil {
		fmt.Fprintf(stderr, "harness scenarios: %v; run harness scenarios --help\n", err)
		return 2
	}
	root := *rootFlag
	if root == "" {
		root, _ = os.Getwd()
	}
	if *matrix == "" {
		*matrix = filepath.Join(root, "harness", "scenarios", "matrix.json")
	}
	if *out == "" {
		*out = filepath.Join(root, "harness", "scenarios", "attempts")
	}
	data, err := os.ReadFile(*matrix)
	if err != nil {
		fmt.Fprintf(stderr, "harness scenarios: %v\n", err)
		return 1
	}
	decoded, err := decodeJSON(data)
	if err != nil {
		fmt.Fprintf(stderr, "harness scenarios: %s: %v\n", *matrix, err)
		return 1
	}
	declaration, _ := decoded.(map[string]any)
	switch sub {
	case "validate":
		errors := validateMatrix(declaration)
		for _, problem := range errors {
			fmt.Fprintln(stderr, problem)
		}
		if len(errors) > 0 {
			fmt.Fprintln(stdout, "local-tooling: fail")
			return 1
		}
		fmt.Fprintln(stdout, "local-tooling: pass")
		return 0
	case "report":
		fmt.Fprintln(stdout, report(declaration, *out, root))
		return 0
	case "compare":
		if *before == "" || *after == "" {
			fmt.Fprintln(stderr, "harness scenarios: compare needs --before and --after")
			return 2
		}
		fmt.Fprintln(stdout, compare(*before, *after, declaration, root))
		return 0
	case "run":
		fixturesPath := *fixtures
		if fixturesPath == "" {
			fixturesPath = filepath.Join(root, "harness", "scenarios", "fixtures")
		}
		var cap *int
		if isFlagSet(flags, "max-sessions") {
			cap = maxSessions
		}
		errors := validateMatrix(declaration)
		if len(errors) > 0 {
			for _, problem := range errors {
				fmt.Fprintln(stderr, problem)
			}
			return 1
		}
		exitCode := 0
		for _, raw := range list(declaration, "scenarios") {
			entry, _ := raw.(map[string]any)
			if *scenario != "" && getString(entry, "id") != *scenario {
				continue
			}
			state, where, err := runOne(entry, root, *out, fixturesPath, *allowPaid, cap)
			if err != nil {
				fmt.Fprintf(stderr, "%s: %v\n", getString(entry, "id"), err)
				exitCode = 1
				continue
			}
			fmt.Fprintf(stdout, "%s: %s -> %s\n", getString(entry, "id"), state, where)
			if state == "fail" || state == "not-run" || state == "interrupted" {
				exitCode = 1
			}
		}
		fmt.Fprintln(stdout, report(declaration, *out, root))
		return exitCode
	}
	return 2
}

func newFlags() *flagSet {
	return &flagSet{values: map[string]*string{}, bools: map[string]*bool{}, ints: map[string]*int{}, set: map[string]bool{}}
}

// flagSet is a minimal parser for the subcommand flags, which all take a value or are boolean.
type flagSet struct {
	values map[string]*string
	bools  map[string]*bool
	ints   map[string]*int
	set    map[string]bool
}

func (f *flagSet) String(name, def, _ string) *string {
	value := def
	f.values[name] = &value
	return &value
}

func (f *flagSet) Bool(name string, def bool, _ string) *bool {
	value := def
	f.bools[name] = &value
	return &value
}

func (f *flagSet) Int(name string, def int, _ string) *int {
	value := def
	f.ints[name] = &value
	return &value
}

func (f *flagSet) Parse(args []string) error {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			return fmt.Errorf("unexpected argument %q", arg)
		}
		name, value, hasValue := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		if b, ok := f.bools[name]; ok {
			if hasValue {
				parsed, err := strconv.ParseBool(value)
				if err != nil {
					return fmt.Errorf("--%s: %v", name, err)
				}
				*b = parsed
			} else {
				*b = true
			}
			f.set[name] = true
			continue
		}
		if !hasValue {
			if i+1 >= len(args) {
				return fmt.Errorf("--%s needs a value", name)
			}
			i++
			value = args[i]
		}
		if target, ok := f.values[name]; ok {
			*target = value
			f.set[name] = true
			continue
		}
		if target, ok := f.ints[name]; ok {
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("--%s: %v", name, err)
			}
			*target = parsed
			f.set[name] = true
			continue
		}
		return fmt.Errorf("unknown flag --%s", name)
	}
	return nil
}

func isFlagSet(f *flagSet, name string) bool { return f.set[name] }

// ---- matrix accessors ----

func getString(m map[string]any, key string) string {
	value, _ := m[key].(string)
	return value
}

func list(m map[string]any, key string) []any {
	value, _ := m[key].([]any)
	return value
}

func dict(m map[string]any, key string) map[string]any {
	value, _ := m[key].(map[string]any)
	return value
}

func truthy(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case bool:
		return v
	case string:
		return v != ""
	case []any:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	case json.Number:
		f, err := v.Float64()
		return err == nil && f != 0
	case float64:
		return v != 0
	default:
		return true
	}
}

// mode reports a scenario's execution mode, defaulting by whether it declares a check.
func mode(scenario map[string]any) string {
	if value := getString(scenario, "mode"); value != "" {
		return value
	}
	if truthy(scenario["check"]) {
		return "offline"
	}
	return "live"
}

// validateMatrix rejects a declaration before any session runs.
func validateMatrix(matrix map[string]any) []string {
	var errors []string
	scenarios := list(matrix, "scenarios")
	if len(scenarios) == 0 {
		return []string{"matrix declares no scenarios"}
	}
	seen := map[string]bool{}
	for index, raw := range scenarios {
		scenario, _ := raw.(map[string]any)
		where := getString(scenario, "id")
		if where == "" {
			where = fmt.Sprintf("scenario[%d]", index)
		}
		for _, field := range []string{"id", "command", "prompt"} {
			if !truthy(scenario[field]) {
				errors = append(errors, fmt.Sprintf("%s: missing %s", where, field))
			}
		}
		if seen[where] {
			errors = append(errors, fmt.Sprintf("%s: duplicate id", where))
		}
		seen[where] = true
		artifacts := dict(scenario, "artifacts")
		hasKey := false
		for _, key := range artifactKeys {
			if _, ok := artifacts[key]; ok {
				hasKey = true
				break
			}
		}
		if artifacts == nil || !hasKey {
			errors = append(errors, fmt.Sprintf("%s: declares no artifacts or assertions", where))
		} else {
			contains := artifacts["contains"]
			if contains != nil {
				if _, ok := contains.([]any); !ok {
					errors = append(errors, fmt.Sprintf("%s: contains must be a list", where))
				}
			}
			for _, item := range asList(contains) {
				entry, ok := item.(map[string]any)
				if !ok || !isString(entry["path"]) || !isString(entry["text"]) {
					errors = append(errors, fmt.Sprintf("%s: contains entry needs string path and text", where))
				}
			}
		}
		if mode(scenario) == "offline" && !truthy(scenario["check"]) {
			errors = append(errors, fmt.Sprintf("%s: offline scenario needs a check command", where))
		}
	}
	return errors
}

func asList(value any) []any {
	items, _ := value.([]any)
	return items
}

func isString(value any) bool {
	_, ok := value.(string)
	return ok
}

func entry(name string, ok bool, detail string) map[string]any {
	return map[string]any{"name": name, "ok": ok, "detail": detail}
}

// evaluateArtifacts applies a scenario's declared artifact checks to the repository it ran in.
func evaluateArtifacts(scenario map[string]any, before, after map[string]string, repo string, forgeCalls []string, fixturesUnchanged bool) []map[string]any {
	artifacts := dict(scenario, "artifacts")
	var entries []map[string]any
	if value, ok := artifacts["tree_unchanged"]; ok {
		expected := truthy(value)
		entries = append(entries, entry("tree_unchanged", equalTrees(before, after) == expected, ""))
	}
	if value, ok := artifacts["fixtures_unchanged"]; ok {
		expected := truthy(value)
		entries = append(entries, entry("fixtures_unchanged", fixturesUnchanged == expected, ""))
	}
	for _, relative := range asList(artifacts["present"]) {
		path := filepath.Join(repo, relative.(string))
		entries = append(entries, entry("present:"+relative.(string), pathExists(path), ""))
	}
	for _, relative := range asList(artifacts["absent"]) {
		path := filepath.Join(repo, relative.(string))
		entries = append(entries, entry("absent:"+relative.(string), !pathExists(path), ""))
	}
	base, _ := filepath.Abs(repo)
	base = resolveExisting(base)
	for _, item := range asList(artifacts["contains"]) {
		declaration, _ := item.(map[string]any)
		relative := getString(declaration, "path")
		text := getString(declaration, "text")
		target := resolveExisting(filepath.Join(repo, filepath.FromSlash(relative)))
		inside := isWithin(base, target)
		found := false
		if inside {
			if raw, err := os.ReadFile(target); err == nil {
				found = strings.Contains(string(raw), text)
			}
		}
		entries = append(entries, entry(fmt.Sprintf("contains:%s:%s", relative, text), found, ""))
	}
	entries = append(entries, entry("no-external-forge", len(forgeCalls) == 0, strings.Join(forgeCalls, "; ")))
	return entries
}

func equalTrees(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// resolveExisting follows symlinks where it can, matching pathlib's non-strict resolve.
func resolveExisting(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return abs
}

func isWithin(root, path string) bool {
	root = strings.TrimSuffix(root, string(filepath.Separator))
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}

// reviewed reports whether the attempt carries a transcript review record.
func reviewed(attempt string) bool {
	raw, err := os.ReadFile(filepath.Join(attempt, "review.json"))
	if err != nil {
		return false
	}
	data, err := decodeJSON(raw)
	if err != nil {
		return false
	}
	object, ok := data.(map[string]any)
	if !ok {
		return false
	}
	return truthy(object["reviewed"])
}

// evaluate returns the state of one retained attempt and the reasons it did not pass.
func evaluate(attempt string, scenario map[string]any, currentDigest string) (string, []string) {
	var missing []string
	for _, name := range requiredEvidence {
		if !pathExists(filepath.Join(attempt, name)) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return "incomplete", []string{"missing evidence: " + strings.Join(missing, ", ")}
	}
	provenance, err := readObject(filepath.Join(attempt, "provenance.json"))
	if err != nil {
		return "incomplete", []string{"unreadable evidence: " + err.Error()}
	}
	artifacts, err := readObject(filepath.Join(attempt, "artifacts.json"))
	if err != nil {
		return "incomplete", []string{"unreadable evidence: " + err.Error()}
	}
	metrics, err := readObject(filepath.Join(attempt, "metrics.json"))
	if err != nil {
		return "incomplete", []string{"unreadable evidence: metrics is not an object"}
	}
	if getString(provenance, "input_digest") != currentDigest {
		return "stale", []string{"inputs changed since " + getString(provenance, "recorded_at")}
	}
	if getString(metrics, "terminal_status") == "interrupted" {
		return "interrupted", []string{"session interrupted"}
	}
	if !truthy(artifacts["ok"]) {
		var failed []string
		for _, raw := range asList(artifacts["results"]) {
			result, _ := raw.(map[string]any)
			if !truthy(result["ok"]) {
				failed = append(failed, getString(result, "name"))
			}
		}
		detail := "none recorded"
		if len(failed) > 0 {
			detail = strings.Join(failed, ", ")
		}
		return "fail", []string{"artifact checks failed: " + detail}
	}
	if !reviewed(attempt) {
		return "unreviewed", []string{"no transcript review record"}
	}
	return "pass", nil
}

func readObject(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decoded, err := decodeJSON(raw)
	if err != nil {
		return nil, err
	}
	object, ok := decoded.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s is not an object", filepath.Base(path))
	}
	return object, nil
}

func recordedDigest(attempt string) (string, bool) {
	provenance, err := readObject(filepath.Join(attempt, "provenance.json"))
	if err != nil {
		return "", false
	}
	value, ok := provenance["input_digest"].(string)
	return value, ok
}

func readMetrics(attempt string) map[string]any {
	metrics, err := readObject(filepath.Join(attempt, "metrics.json"))
	if err != nil {
		return map[string]any{}
	}
	return metrics
}

func canonical(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func fileDigest(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "missing"
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func treeDigest(root string) string {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return "missing"
	}
	var files []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		files = append(files, path)
		return nil
	})
	sort.Strings(files)
	var parts []string
	for _, path := range files {
		rel, _ := filepath.Rel(root, path)
		parts = append(parts, filepath.ToSlash(rel)+":"+fileDigest(path))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}

// scenarioDigest hashes the declaration, its harness inputs, and its recording, so a change marks an older attempt stale.
func scenarioDigest(scenario map[string]any, root, fixturesRoot string) string {
	parts := []string{canonical(scenario)}
	for _, raw := range list(scenario, "harness_inputs") {
		relative, _ := raw.(string)
		parts = append(parts, relative+":"+fileDigest(filepath.Join(root, filepath.FromSlash(relative))))
	}
	parts = append(parts, "fixture:"+treeDigest(filepath.Join(fixturesRoot, getString(scenario, "id"))))
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}

func snapshot(root string) map[string]string {
	tree := map[string]string{}
	if !pathExists(root) {
		return tree
	}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		parts := strings.Split(filepath.ToSlash(rel), "/")
		for _, part := range parts {
			if part == ".git" {
				return nil
			}
		}
		tree[filepath.ToSlash(rel)] = fileDigest(path)
		return nil
	})
	return tree
}

// installForgeGuard writes a disposable `gh` that logs and refuses, so a scenario cannot reach a forge. A shim that cannot be written fails the run rather than leaving the real forge reachable.
func installForgeGuard(workspace string) (shim, log string, err error) {
	shim = filepath.Join(workspace, "shim")
	if err := os.MkdirAll(shim, 0o755); err != nil {
		return "", "", err
	}
	log = filepath.Join(workspace, "forge.log")
	script := "#!/bin/sh\nprintf \"%s\\n\" \"$*\" >> \"" + log + "\"\nexit 1\n"
	if err := os.WriteFile(filepath.Join(shim, "gh"), []byte(script), 0o755); err != nil {
		return "", "", err
	}
	return shim, log, nil
}

func forgeEnv(shim string) []string {
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

// claimSession retains a per-batch paid-session cap and refuses once it is reached.
func claimSession(outRoot string, maxSessions *int) error {
	path := filepath.Join(outRoot, batchFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data := map[string]any{"cap": nil, "sessions": 0}
	if raw, err := os.ReadFile(path); err == nil {
		if decoded, err := decodeJSON(raw); err == nil {
			if object, ok := decoded.(map[string]any); ok {
				data = object
			}
		}
	}
	capValue, hasCap := data["cap"]
	if !hasCap || capValue == nil {
		if maxSessions == nil {
			return fmt.Errorf("paid execution needs --max-sessions to set the cap")
		}
		data["cap"] = *maxSessions
	} else if maxSessions != nil && floatOf(capValue) != float64(*maxSessions) {
		return fmt.Errorf("batch cap is retained at %v; start a new batch to change it", formatNumber(capValue))
	}
	if floatOf(data["sessions"]) >= floatOf(data["cap"]) {
		return fmt.Errorf("per-batch session cap reached (%s/%s)", formatNumber(data["sessions"]), formatNumber(data["cap"]))
	}
	data["sessions"] = intOf(data["sessions"]) + 1
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(encoded, '\n'), 0o644)
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

func runOne(scenario map[string]any, root, outRoot, fixturesRoot string, allowPaid bool, maxSessions *int) (string, string, error) {
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
	shim, forgeLog, err := installForgeGuard(workspace)
	if err != nil {
		return "", "", err
	}
	env := forgeEnv(shim)
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
		if err := claimSession(outRoot, maxSessions); err != nil {
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
	entries := evaluateArtifacts(scenario, before, after, repo, forgeCalls, fixturesBefore == treeDigest(fixture))
	if checkCode != nil {
		entries = append(entries, entry("check-exit", *checkCode == 0, "exit "+strconv.Itoa(*checkCode)))
	}
	if liveCode != nil {
		entries = append(entries, entry("session-exit", *liveCode == 0, "exit "+strconv.Itoa(*liveCode)))
	}
	remotes := gitRemotes(repo)
	entries = append(entries, entry("no-git-remote", len(remotes) == 0, strings.Join(remotes, "; ")))

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
	status := terminalStatus(exitCode)
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

func report(matrix map[string]any, outRoot, root string) string {
	errors := validateMatrix(matrix)
	var lines []string
	if len(errors) == 0 {
		lines = append(lines, "local-tooling: pass")
	} else {
		lines = append(lines, "local-tooling: fail")
	}
	for _, problem := range errors {
		lines = append(lines, "  "+problem)
	}
	lines = append(lines, "behavioral-acceptance:")
	for _, raw := range list(matrix, "scenarios") {
		scenario, _ := raw.(map[string]any)
		attempts := attemptPaths(outRoot, getString(scenario, "id"))
		if len(attempts) == 0 {
			lines = append(lines, "  "+getString(scenario, "id")+": incomplete (no attempt)")
			continue
		}
		state, reasons := evaluate(attempts[len(attempts)-1], scenario, scenarioDigest(scenario, root, filepath.Join(root, "harness", "scenarios", "fixtures")))
		detail := ""
		if len(reasons) > 0 {
			detail = " (" + strings.Join(reasons, "; ") + ")"
		}
		lines = append(lines, "  "+getString(scenario, "id")+": "+state+detail)
	}
	return strings.Join(lines, "\n")
}

var efficiencyFields = []string{"input_tokens", "output_tokens", "cached_input", "tool_calls", "command_output_bytes", "wall_seconds"}

// batchCounts tallies a batch by its own recorded states, not the current checkout.
func batchCounts(root string, scenario map[string]any, digest string) (map[string]int, map[string][]map[string]any) {
	attempts := attemptPaths(root, getString(scenario, "id"))
	counts := map[string]int{}
	passing := map[string][]map[string]any{}
	if len(attempts) == 0 {
		return map[string]int{"unavailable": 1}, passing
	}
	recorded := make([]string, len(attempts))
	tally := map[string]int{}
	for i, attempt := range attempts {
		value, ok := recordedDigest(attempt)
		if ok {
			recorded[i] = value
			tally[value]++
		}
	}
	reference := digest
	if _, ok := tally[digest]; ok {
		reference = digest
	} else if len(tally) > 0 {
		best, bestCount := "", -1
		for value, count := range tally {
			if count > bestCount || (count == bestCount && value < best) {
				best, bestCount = value, count
			}
		}
		reference = best
	}
	for i, attempt := range attempts {
		value := recorded[i]
		if value != "" && value != reference {
			counts["stale"]++
			continue
		}
		current := value
		if current == "" {
			current = digest
		}
		state, _ := evaluate(attempt, scenario, current)
		counts[state]++
		if state == "pass" {
			metrics := readMetrics(attempt)
			if model, ok := metrics["model"].(string); ok {
				passing[model] = append(passing[model], metrics)
			}
		}
	}
	return counts, passing
}

func rate(counts map[string]int) string {
	if len(counts) == 1 {
		if _, ok := counts["unavailable"]; ok {
			return "unavailable"
		}
	}
	total := 0
	for _, count := range counts {
		total += count
	}
	var details []string
	for _, state := range stateOrder {
		if counts[state] > 0 {
			details = append(details, fmt.Sprintf("%s=%d", state, counts[state]))
		}
	}
	return fmt.Sprintf("pass %d/%d (%s)", counts["pass"], total, strings.Join(details, " "))
}

func median(records []map[string]any, field string) (any, bool) {
	var values []float64
	for _, record := range records {
		if value, ok := toNumber(record[field]); ok {
			values = append(values, value)
		}
	}
	if len(values) == 0 {
		return nil, false
	}
	sort.Float64s(values)
	middle := len(values) / 2
	if len(values)%2 == 1 {
		return values[middle], true
	}
	return (values[middle-1] + values[middle]) / 2, true
}

func formatValue(value any) string {
	number, ok := toNumber(value)
	if !ok {
		return "-"
	}
	if number == math.Trunc(number) {
		return strconv.FormatInt(int64(number), 10)
	}
	return fmt.Sprintf("%.3f", number)
}

// compare puts two retained batches side by side, pass rates before efficiency.
func compare(beforeRoot, afterRoot string, matrix map[string]any, root string) string {
	lines := []string{"behavioral pass rates"}
	type pair struct {
		scenario, model string
		before, after   []map[string]any
	}
	var pairs []pair
	for _, raw := range list(matrix, "scenarios") {
		scenario, _ := raw.(map[string]any)
		digest := scenarioDigest(scenario, root, filepath.Join(root, "harness", "scenarios", "fixtures"))
		beforeCounts, beforePass := batchCounts(beforeRoot, scenario, digest)
		afterCounts, afterPass := batchCounts(afterRoot, scenario, digest)
		lines = append(lines, "  "+getString(scenario, "id"))
		lines = append(lines, "    before: "+rate(beforeCounts))
		lines = append(lines, "    after:  "+rate(afterCounts))
		for model := range beforePass {
			if _, ok := afterPass[model]; !ok {
				continue
			}
			pairs = append(pairs, pair{getString(scenario, "id"), model, beforePass[model], afterPass[model]})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].scenario != pairs[j].scenario {
			return pairs[i].scenario < pairs[j].scenario
		}
		return pairs[i].model < pairs[j].model
	})
	lines = append(lines, "", "efficiency (passing runs only)")
	if len(pairs) == 0 {
		lines = append(lines, "  no comparable pair")
		return strings.Join(lines, "\n")
	}
	if len(pairs) == 1 {
		lines = append(lines, fmt.Sprintf("  smoke: one successful pair (%s model=%s), report as smoke rather than a savings claim", pairs[0].scenario, pairs[0].model))
		return strings.Join(lines, "\n")
	}
	for _, item := range pairs {
		lines = append(lines, fmt.Sprintf("  %s model=%s", item.scenario, item.model))
		lines = append(lines, fmt.Sprintf("    %-20s %10s %10s %10s", "metric", "before", "after", "delta"))
		for _, field := range efficiencyFields {
			before, beforeOK := median(item.before, field)
			after, afterOK := median(item.after, field)
			delta := any(nil)
			if beforeOK && afterOK {
				delta = after.(float64) - before.(float64)
			}
			if !beforeOK {
				before = nil
			}
			if !afterOK {
				after = nil
			}
			lines = append(lines, fmt.Sprintf("    %-20s %10s %10s %10s", field, formatValue(before), formatValue(after), formatValue(delta)))
		}
	}
	return strings.Join(lines, "\n")
}

func decodeJSON(data []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("trailing data after the JSON document")
	}
	return value, nil
}
