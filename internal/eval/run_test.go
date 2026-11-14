package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubs puts stand-in claude, codex, and opencode programs first on PATH. Each records its arguments, working directory, and stdin under log, writes a Codex rollout naming its directory, and exits 3 in a directory holding stop.txt.
func stubs(t *testing.T, home string) string {
	t.Helper()
	bin, log := t.TempDir(), t.TempDir()
	script := `#!/bin/sh
out="` + log + `/$(basename "$PWD")"
pwd > "$out.cwd"
for a in "$@"; do printf '%s\n--\n' "$a"; done > "$out.args"
cat > "$out.stdin"
day="` + home + `/.codex/sessions/2026/11/14"
mkdir -p "$day"
printf '{"type":"session_meta","payload":{"cwd":"%s"}}\n' "$PWD" > "$day/rollout-$(basename "$PWD").jsonl"
echo "worked in $(basename "$PWD")"
[ -e stop.txt ] && exit 3
exit 0
`
	for _, name := range []string{"claude", "codex", "opencode"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	t.Setenv("CODEX_HOME", "")
	return log
}

func prompt(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ask.md")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunStartsOneSessionPerDirectoryAndModel(t *testing.T) {
	f := newFixture(t)
	log := stubs(t, f.home)
	m, _ := f.plant(t, "codex", 2)
	text := "Tidy the docs folder.\n'quoted' $HOME\n"
	sessions, err := Run(f.home, m.Run, RunOptions{PromptFile: prompt(t, text), Models: []string{"m-one", "m-two"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, key, err := ReadRun(f.home, m.Run)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 8 {
		t.Fatalf("started %d sessions", len(sessions))
	}
	arms := map[string]map[string]bool{}
	dirs := map[string]bool{}
	for _, s := range sessions {
		if arms[s.Model] == nil {
			arms[s.Model] = map[string]bool{}
		}
		arms[s.Model][key.Labels[s.Label]] = true
		dirs[s.Dir] = true
		name := filepath.Base(s.Dir)
		if got := strings.TrimSpace(read(t, filepath.Join(log, name+".cwd"))); got != s.Dir {
			t.Errorf("%s ran in %s", s.Label, got)
		}
		if read(t, filepath.Join(log, name+".stdin")) != text {
			t.Errorf("%s did not get the prompt's exact text", s.Label)
		}
		args := read(t, filepath.Join(log, name+".args"))
		for _, want := range []string{"exec\n", "--sandbox\n--\nworkspace-write\n", "--model\n--\n" + s.Model + "\n"} {
			if !strings.Contains(args, want) {
				t.Errorf("%s args lack %q:\n%s", s.Label, want, args)
			}
		}
		if s.Status != "ok" || read(t, s.Output) != "worked in "+name+"\n" {
			t.Errorf("%s: status %s, output %q", s.Label, s.Status, s.Output)
		}
		if filepath.Base(s.Output) != s.Label+".out" || strings.HasPrefix(s.Output, f.root) {
			t.Errorf("%s output saved at %s", s.Label, s.Output)
		}
		if s.Record != filepath.Join(f.home, ".codex", "sessions", "2026", "11", "14", "rollout-"+name+".jsonl") {
			t.Errorf("%s record = %q", s.Label, s.Record)
		}
	}
	if len(dirs) != 8 {
		t.Errorf("sessions shared directories: %v", dirs)
	}
	for model, seen := range arms {
		if !seen[Variant] || !seen[Baseline] {
			t.Errorf("%s ran only in %v", model, seen)
		}
	}
	if _, err := Clean(f.home, m.Run); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(f.root); len(entries) != 0 {
		t.Errorf("clean left the copies: %v", entries)
	}
}

func TestRunRecordsAFailureAndRunsTheRest(t *testing.T) {
	f := newFixture(t)
	stubs(t, f.home)
	m, _ := f.plant(t, "claude", 1)
	stop := m.Dirs[0]
	if err := os.WriteFile(filepath.Join(stop.Dir, "stop.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	sessions, err := Run(f.home, m.Run, RunOptions{PromptFile: prompt(t, "go\n"), Models: []string{"sonnet"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range sessions {
		failed := s.Label == stop.Label
		if (s.Status == "failed") != failed || failed && s.Exit != 3 {
			t.Errorf("%s: status %s exit %d", s.Label, s.Status, s.Exit)
		}
	}
	raw := read(t, filepath.Join(MetaDir(f.home, m.Run), sessionsFile))
	var recorded []Session
	if err := json.Unmarshal([]byte(raw), &recorded); err != nil || len(recorded) != 2 {
		t.Errorf("sessions record: %v %s", err, raw)
	}
	if _, err := Run(f.home, m.Run, RunOptions{PromptFile: prompt(t, "go\n"), Models: []string{"sonnet"}}, nil); err == nil {
		t.Error("a second run of the same id was accepted")
	}
}

func TestDryRunPrintsEachCommandAndStartsNothing(t *testing.T) {
	f := newFixture(t)
	log := stubs(t, f.home)
	for client, want := range map[string]string{
		"claude":   "claude -p --permission-mode acceptEdits --model sonnet --session-id ",
		"codex":    "codex exec --sandbox workspace-write --skip-git-repo-check --model sonnet - < ",
		"opencode": `opencode run --auto --model sonnet -- "$(cat `,
	} {
		m, _ := f.plant(t, client, 1)
		var out strings.Builder
		ask := prompt(t, "go\n")
		if _, err := Run(f.home, m.Run, RunOptions{PromptFile: ask, Models: []string{"sonnet"}, DryRun: true}, &out); err != nil {
			t.Fatal(err)
		}
		for _, p := range m.Dirs {
			if !strings.Contains(out.String(), "cd "+p.Dir+" && "+want) {
				t.Errorf("%s dry run lacks %q for %s:\n%s", client, want, p.Label, out.String())
			}
		}
		if _, err := os.Stat(filepath.Join(MetaDir(f.home, m.Run), sessionsFile)); err == nil {
			t.Errorf("%s dry run wrote a sessions record", client)
		}
	}
	if entries, _ := os.ReadDir(log); len(entries) != 0 {
		t.Errorf("a dry run started %v", entries)
	}
}

func TestRunRefusesAnUnknownRunOrMissingPrompt(t *testing.T) {
	f := newFixture(t)
	log := stubs(t, f.home)
	m, _ := f.plant(t, "claude", 1)
	if _, err := Run(f.home, "20990101-000000-abcdef", RunOptions{PromptFile: prompt(t, "go"), Models: []string{"x"}}, nil); err == nil {
		t.Error("an unknown run was accepted")
	}
	if _, err := Run(f.home, m.Run, RunOptions{PromptFile: filepath.Join(t.TempDir(), "none.md"), Models: []string{"x"}}, nil); err == nil {
		t.Error("a missing prompt file was accepted")
	}
	if entries, _ := os.ReadDir(log); len(entries) != 0 {
		t.Errorf("a refused run started %v", entries)
	}
}

func TestOpenCodeTakesADashLedPromptAsTheMessage(t *testing.T) {
	cmd, _, _ := command("opencode", "m", "- fix the list")
	if len(cmd) < 2 || cmd[len(cmd)-2] != "--" || cmd[len(cmd)-1] != "- fix the list" {
		t.Fatalf("opencode argv %q does not end the flags before the prompt", cmd)
	}
}
