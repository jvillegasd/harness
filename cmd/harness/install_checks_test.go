package main

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

var agentOutputs = []string{"claude/code-search.md", "claude/executor.md", "claude/reviewer.md", "claude/review-bugs.md", "claude/review-risk.md", "claude/review-spec.md", "codex/code-search.toml", "codex/executor.toml", "codex/reviewer.toml", "codex/review-bugs.toml", "codex/review-risk.toml", "codex/review-spec.toml", "opencode/code-search.md", "opencode/executor.md", "opencode/reviewer.md", "opencode/review-bugs.md", "opencode/review-risk.md", "opencode/review-spec.md"}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func symlink(t *testing.T, target, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}

func isSymlink(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&fs.ModeSymlink != 0
}

func readFileOr(path string) string {
	data, _ := os.ReadFile(path)
	return string(data)
}

func readDir(dir string) []string {
	list, _ := os.ReadDir(dir)
	var out []string
	for _, e := range list {
		out = append(out, e.Name())
	}
	return out
}

func TestGuardedSkillsAreInstalled(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	ok(t, runInstaller(t, base, "skills"), "skip-packages install")
	bodies, err := filepath.Glob(filepath.Join(base, "checkout", "skills", "*", "*", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	guarded := 0
	for _, body := range bodies {
		if !strings.Contains(readFile(t, body), "disable-model-invocation: true") {
			continue
		}
		guarded++
		name := filepath.Base(filepath.Dir(body))
		if _, err := os.Stat(filepath.Join(base, "home", ".agents", "skills", name, "SKILL.md")); err != nil {
			t.Errorf("guarded skill %s is not installed in the shared skill namespace: %v", name, err)
		}
	}
	if guarded == 0 {
		t.Fatal("found no guarded skill")
	}
}

func TestInstallRefusesAnUnguardedLocalSkill(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	writeMode(t, filepath.Join(base, "checkout", "skills", "workflow", "zz-bare", "SKILL.md"), "---\nname: zz-bare\ndisable-model-invocation: true\n---\nbody\n", 0o644)
	var before strings.Builder
	tree(t, &before, filepath.Join(base, "home"), true)
	r := runInstaller(t, base)
	if r.code == 0 || !strings.Contains(r.stderr, "zz-bare") || !strings.Contains(r.stderr, "opencode/autoinvoke") {
		t.Errorf("exit %d, stderr %q; want a refusal naming the skill and the key", r.code, r.stderr)
	}
	var after strings.Builder
	tree(t, &after, filepath.Join(base, "home"), true)
	if before.String() != after.String() {
		t.Error("the refused install wrote to HOME")
	}
}

func TestInstallExplainsAMissingLock(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	if err := os.Remove(filepath.Join(base, "checkout", "skills-lock.json")); err != nil {
		t.Fatal(err)
	}
	if r := runInstaller(t, base); r.code == 0 || !strings.Contains(r.stderr, "separates local skills from locked ones") {
		t.Errorf("exit %d, stderr %q; want the lock's purpose named", r.code, r.stderr)
	}
}

func TestAgentInstallAndUninstallTouchOnlyRecordedCopies(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	home := filepath.Join(base, "home")
	roots := map[string]string{"claude": filepath.Join(home, ".claude", "agents"), "codex": filepath.Join(home, ".codex", "agents"), "opencode": filepath.Join(home, ".config", "opencode", "agents")}
	private := filepath.Join(roots["claude"], "mine.md")
	writeMode(t, private, "mine\n", 0o644)
	foreign := filepath.Join(roots["claude"], "foreign.md")
	symlink(t, "/elsewhere/other-checkout/.generated/agents/claude/foreign.md", foreign)
	secrets := map[string]string{
		filepath.Join(home, ".local", "share", "opencode", "auth.json"): `{"openai": "private-secret"}` + "\n",
		filepath.Join(home, ".claude", ".credentials.json"):             `{"token": "private-secret"}` + "\n",
	}
	for path, text := range secrets {
		writeMode(t, path, text, 0o644)
	}
	opencode := filepath.Join(home, ".config", "opencode", "opencode.json")
	writeMode(t, opencode, `{"agent": {"mine": {"model": "openai/gpt-5.5"}}}`+"\n", 0o644)
	untouched := func(stage string) {
		t.Helper()
		if got := readFile(t, private); got != "mine\n" {
			t.Errorf("%s changed the private agent: %q", stage, got)
		}
		if !isSymlink(foreign) {
			t.Errorf("%s removed a link it did not write", stage)
		}
		for path, text := range secrets {
			if got := readFile(t, path); got != text {
				t.Errorf("%s changed %s: %q", stage, path, got)
			}
		}
		if !strings.Contains(readFile(t, opencode), `"mine"`) {
			t.Errorf("%s dropped the private OpenCode agent", stage)
		}
	}

	ok(t, runInstaller(t, base, "agents", "settings"), "install")
	for _, rel := range agentOutputs {
		client, name, _ := strings.Cut(rel, "/")
		path := filepath.Join(roots[client], name)
		if isSymlink(path) || strings.Contains(readFileOr(path), "private-secret") || readFileOr(path) == "" {
			t.Errorf("%s is not a clean copy", path)
		}
	}
	untouched("the install")

	ok(t, runUninstaller(t, base, "agents", "settings", "--yes"), "uninstall")
	for _, root := range roots {
		for _, entry := range readDir(root) {
			if entry != "mine.md" && entry != "foreign.md" {
				t.Errorf("the uninstall left %s in %s", entry, root)
			}
		}
	}
	untouched("the uninstall")
}

func TestAgentInstallArchivesUnrecordedCollisions(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"file", "foreign-link", "root-link"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			base := sandbox(t)
			home := filepath.Join(base, "home")
			root := filepath.Join(home, ".config", "opencode", "agents")
			private := filepath.Join(base, "private", "reviewer.md")
			writeMode(t, private, "private agent\n", 0o644)
			switch kind {
			case "file":
				writeMode(t, filepath.Join(root, "reviewer.md"), "private agent\n", 0o644)
			case "foreign-link":
				symlink(t, private, filepath.Join(root, "reviewer.md"))
			case "root-link":
				symlink(t, filepath.Dir(private), root)
			}
			ok(t, runInstaller(t, base, "agents"), "install")
			if got := readFile(t, private); got != "private agent\n" {
				t.Errorf("the install wrote through to %s: %q", private, got)
			}
			if installed := filepath.Join(root, "reviewer.md"); isSymlink(installed) || readFileOr(installed) == "private agent\n" {
				t.Errorf("%s is not the harness copy", installed)
			}
			archived, _ := filepath.Glob(filepath.Join(home, ".dotfiles-backups", "harness", "*", ".config", "opencode", "agents*"))
			if len(archived) == 0 {
				t.Error("the collision was not archived")
			}
		})
	}
}

func TestUninstallKeepsAnEditedCopy(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	ok(t, runInstaller(t, base, "skills"), "install")
	edited := filepath.Join(base, "home", ".claude", "skills", "open-pr", "SKILL.md")
	writeMode(t, edited, "my open-pr\n", 0o644)
	r := runUninstaller(t, base, "--yes")
	ok(t, r, "uninstall")
	if got := readFileOr(edited); got != "my open-pr\n" {
		t.Errorf("the uninstall removed or changed the edited copy: %q", got)
	}
	if !strings.Contains(r.stderr, "changed since the harness wrote it") {
		t.Errorf("the kept copy was not reported: %q", r.stderr)
	}
	if _, err := os.Stat(filepath.Join(base, "home", ".harness")); err == nil {
		t.Error("a full uninstall left ~/.harness")
	}
}

func TestUninstallDeclinePreservesInstallation(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	ok(t, runInstaller(t, base, "agents"), "install")
	copy := filepath.Join(base, "home", ".claude", "agents", "reviewer.md")
	before := readFile(t, copy)
	r := runUninstallerInput(t, base, "n\n")
	ok(t, r, "declined uninstall")
	if got := readFileOr(copy); got != before {
		t.Errorf("declined uninstall changed the installed copy: %q", got)
	}
}

func TestUninstallOpenCodeLeavesClaudeCopies(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	ok(t, runInstaller(t, base, "agents"), "install")
	claude := filepath.Join(base, "home", ".claude", "agents", "reviewer.md")
	ok(t, runUninstaller(t, base, "agents", "--client", "opencode", "--yes"), "OpenCode uninstall")
	if readFileOr(claude) == "" {
		t.Error("OpenCode uninstall removed the Claude role copy")
	}
	if _, err := os.Stat(filepath.Join(base, "home", ".config", "opencode", "agents", "executor.md")); err == nil {
		t.Error("OpenCode uninstall kept its role copy")
	}
}

func TestCodexInstallsIntoHomeCodexAndUninstallsAlone(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	elsewhere := filepath.Join(base, "launcher-codex-home")
	extra := []string{"TMPDIR=" + filepath.Join(base, "tmp"), "HARNESS_ASSETS=" + filepath.Join(base, "checkout"), "CODEX_HOME=" + elsewhere}
	ok(t, runHarness(t, base, extra, "install"), "install under another CODEX_HOME")
	codexHome := filepath.Join(base, "home", ".codex")
	if readFile(t, filepath.Join(codexHome, "AGENTS.md")) != readFile(t, filepath.Join(base, "checkout", "FLOOR.md")) {
		t.Error("~/.codex/AGENTS.md is not the floor")
	}
	if !strings.Contains(readFile(t, filepath.Join(codexHome, "agents", "executor.toml")), "\nmodel = \"gpt-6.1-terra\"\n") {
		t.Error("the Codex executor lacks the balanced standard-tier model")
	}
	if _, err := os.Stat(elsewhere); err == nil {
		t.Error("the install wrote into CODEX_HOME")
	}
	log := readFile(t, filepath.Join(base, "shims.log"))
	for _, want := range []string{"codex mcp add agent-browser -- " + filepath.Join(base, "shims", "agent-browser") + " mcp", "engram setup codex"} {
		if !strings.Contains(log, want) {
			t.Errorf("install skipped %q:\n%s", want, log)
		}
	}
	if strings.Contains(log, "CODEX_HOME="+elsewhere) || !strings.Contains(log, "CODEX_HOME="+codexHome) {
		t.Errorf("codex ran outside ~/.codex:\n%s", log)
	}

	for _, name := range []string{"engram-instructions.md", "engram-compact-prompt.md"} {
		writeMode(t, filepath.Join(codexHome, name), "engram\n", 0o644)
	}
	ok(t, runHarness(t, base, extra, "uninstall", "--client", "codex", "--yes"), "Codex uninstall")
	for _, gone := range []string{filepath.Join(codexHome, "AGENTS.md"), filepath.Join(codexHome, "agents", "executor.toml"), filepath.Join(codexHome, "engram-instructions.md")} {
		if _, err := os.Stat(gone); err == nil {
			t.Errorf("Codex uninstall kept %s", gone)
		}
	}
	for _, kept := range []string{filepath.Join(base, "home", ".agents", "skills", "land", "agents", "openai.yaml"), filepath.Join(base, "home", ".claude", "agents", "reviewer.md"), filepath.Join(base, "home", ".config", "opencode", "AGENTS.md")} {
		if readFileOr(kept) == "" {
			t.Errorf("Codex uninstall removed %s", kept)
		}
	}
	log = readFile(t, filepath.Join(base, "shims.log"))
	for _, want := range []string{"codex mcp remove agent-browser", "codex mcp remove engram", "codex plugin remove engram@engram", "codex plugin marketplace remove engram"} {
		if !strings.Contains(log, want) {
			t.Errorf("uninstall skipped %q:\n%s", want, log)
		}
	}
}

func TestNoBackupReplacesCollisionWithoutArchiving(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	path := filepath.Join(base, "home", ".claude", "agents", "reviewer.md")
	writeMode(t, path, "private agent\n", 0o644)
	ok(t, runInstaller(t, base, "agents", "--no-backup"), "no-backup install")
	if got := readFile(t, path); got == "private agent\n" {
		t.Error("no-backup left the colliding file in place")
	}
	if backups, _ := filepath.Glob(filepath.Join(base, "home", ".dotfiles-backups", "harness", "*")); len(backups) != 0 {
		t.Errorf("no-backup archived the collision: %v", backups)
	}
}

func TestInstallInstallsTheEmbeddedHarnessFromAnywhere(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	seedFull(t, base)
	home := filepath.Join(base, "home")
	cmd := exec.Command(binary, "install")
	cmd.Dir = base
	cmd.Env = []string{"HOME=" + home, "PATH=" + filepath.Join(base, "shims") + ":/usr/bin:/bin", "SHELL=/bin/zsh", "TMPDIR=" + filepath.Join(base, "tmp")}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	if help, err := exec.Command(filepath.Join(home, ".local", "bin", "harness"), "--help").Output(); err != nil || !strings.Contains(string(help), "install") {
		t.Errorf("the installed harness does not run: %v %s", err, help)
	}
	if got, want := readFileOr(filepath.Join(home, ".claude", "CLAUDE.md")), readFile(t, filepath.Join("..", "..", "FLOOR.md")); got != want {
		t.Error("install did not install the embedded floor at ~/.claude/CLAUDE.md")
	}
}

func TestClearBackupsRunsOutsideTheCheckout(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	seedBackups(t, base)
	home := filepath.Join(base, "home")
	cmd := exec.Command(binary, "uninstall", "--clear-backups")
	cmd.Dir, cmd.Env = home, []string{"HOME=" + home, "PATH=/usr/bin:/bin"}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("clear-backups outside the checkout: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(home, ".dotfiles-backups", "harness")); err == nil {
		t.Error("the harness backups survived")
	}
}

func TestInstallReplacesAnUnusableStatuslineBinary(t *testing.T) {
	t.Parallel()
	installed := func(base string) string { return filepath.Join(base, "home", ".local", "bin", "harness") }
	cases := []struct {
		name string
		seed func(t *testing.T, base string)
	}{
		{"missing", func(t *testing.T, base string) {
			if err := os.Remove(installed(base)); err != nil {
				t.Fatal(err)
			}
		}},
		{"not executable", func(t *testing.T, base string) {
			if err := os.Chmod(installed(base), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"pre-port", func(t *testing.T, base string) {
			writeMode(t, installed(base), "#!/bin/sh\n[ \"$1\" = statusline ] && exit 64\nexit 0\n", 0o755)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			base := sandbox(t)
			c.seed(t, base)
			ok(t, runInstaller(t, base, "floor"), "install")
			if err := exec.Command(installed(base), "statusline", "--help").Run(); err != nil {
				t.Fatalf("the installed harness has no working statusline: %v", err)
			}
		})
	}
}

func TestAgentBrowserRefusesANonObjectOpenCodeMCP(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	config := filepath.Join(base, "home", ".config", "opencode", "opencode.json")
	const text = `{"mcp": ["x"]}` + "\n"
	writeMode(t, config, text, 0o644)
	r := runInstaller(t, base, "mcps")
	if r.code == 0 || !strings.Contains(r.stderr, "mcp is not a JSON object") {
		t.Fatalf("exit %d, stderr %q; want the mcp refusal", r.code, r.stderr)
	}
	if got := readFile(t, config); got != text {
		t.Errorf("the refused config changed: %q", got)
	}
	log := readFileOr(filepath.Join(base, "shims.log"))
	for _, early := range []string{"claude mcp add", "codex mcp add", "engram setup"} {
		if strings.Contains(log, early) {
			t.Errorf("%q ran before the refusal:\n%s", early, log)
		}
	}
}

func TestCodexUninstallWithoutCodex(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	ok(t, runInstaller(t, base, "mcps"), "install")
	codexHome := filepath.Join(base, "home", ".codex")
	for _, name := range []string{"engram-instructions.md", "engram-compact-prompt.md"} {
		writeMode(t, filepath.Join(codexHome, name), "engram\n", 0o644)
	}
	if err := os.Remove(filepath.Join(base, "shims", "codex")); err != nil {
		t.Fatal(err)
	}
	r := ok(t, runUninstaller(t, base, "--client", "codex", "--yes"), "Codex uninstall without Codex")
	if !strings.Contains(r.stderr, "Codex is not installed") {
		t.Errorf("no warning about the entries left behind:\n%s", r.stderr)
	}
	if _, err := os.Stat(filepath.Join(codexHome, "engram-instructions.md")); err == nil {
		t.Error("Engram's instructions file survived")
	}
}

func TestInstallRefusesANonUserScopeEngramMCP(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	seedLived(t, base)
	writeShim(t, base, "claude", `case "$*" in
"plugin list --json") echo '[{"id": "engram@engram"}]' ;;
"mcp get agent-browser") echo 'Scope: User config' ;;
"mcp get engram") echo 'Scope: Project config' ;;
esac`)
	r := runInstaller(t, base, "mcps")
	if r.code == 0 || !strings.Contains(r.stderr, "engram is not registered in the Claude Code user scope") {
		t.Fatalf("exit %d, stderr %q; want the engram scope refusal", r.code, r.stderr)
	}
}

func TestUninstallRefusesASurvivingEngramMCP(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	seedLived(t, base)
	ok(t, runInstaller(t, base, "mcps"), "install")
	writeMode(t, filepath.Join(base, "home", ".claude.json"), `{"mcpServers": {"engram": {}}}`+"\n", 0o644)
	r := runUninstaller(t, base, "mcps", "--yes")
	if r.code == 0 || !strings.Contains(r.stderr, "The engram MCP survived the uninstall") {
		t.Fatalf("exit %d, stderr %q; want the engram survivor refusal", r.code, r.stderr)
	}
}

func missingTools(t *testing.T, base string, names ...string) {
	t.Helper()
	shims, saved := filepath.Join(base, "shims"), filepath.Join(base, "saved")
	for _, name := range names {
		writeMode(t, filepath.Join(saved, name), readFile(t, filepath.Join(shims, name)), 0o755)
		if err := os.Remove(filepath.Join(shims, name)); err != nil {
			t.Fatal(err)
		}
	}
	restore := `for arg in "$@"; do n=$(basename "$arg"); n=${n%-v2}; [ -e '` + saved + `'/"$n" ] && cp '` + saved + `'/"$n" '` + shims + `'/"$n"; done`
	writeShim(t, base, "brew", `if [ "$1" = install ]; then `+restore+`; fi`)
	writeShim(t, base, "npm", `if [ "$1" = install ]; then `+restore+`; fi`)
}

func recordedTools(t *testing.T, base string) map[string]string {
	t.Helper()
	var s struct {
		Tools     map[string]string   `json:"tools"`
		Installed map[string][]string `json:"installed"`
	}
	if err := json.Unmarshal([]byte(readFile(t, stateFile(base))), &s); err != nil {
		t.Fatal(err)
	}
	return s.Tools
}

func TestFullInstallProvisionsMissingOnly(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	seedFull(t, base)
	missingTools(t, base, "uv", "agent-browser", "engram")
	ok(t, runInstaller(t, base), "full install")
	log := readFile(t, filepath.Join(base, "shims.log"))
	for _, absent := range []string{"brew list", "brew upgrade", "brew install node", "brew install gh", "brew install anomalyco/tap/opencode", "brew tap anomalyco/tap", "brew install --cask codex", "npm install"} {
		if strings.Contains(log, absent) {
			t.Errorf("full install unexpectedly invoked %q:\n%s", absent, log)
		}
	}
	for _, want := range []string{"brew install uv", "brew install agent-browser", "brew tap gentleman-programming/tap", "brew install gentleman-programming/tap/engram", "agent-browser install"} {
		if !strings.Contains(log, want) {
			t.Errorf("full install skipped the missing %q:\n%s", want, log)
		}
	}
	for _, want := range []string{"engram setup claude-code", "engram setup codex", "engram setup opencode", "claude mcp add --scope user agent-browser", "claude mcp add --scope user engram", "codex mcp add agent-browser"} {
		if !strings.Contains(log, want) {
			t.Errorf("full install skipped reconciliation though the tools exist, missing %q", want)
		}
	}
	want := map[string]string{"uv": "brew:uv", "agent-browser": "brew:agent-browser", "engram": "brew:gentleman-programming/tap/engram"}
	if got := recordedTools(t, base); !reflect.DeepEqual(got, want) {
		t.Errorf("recorded tools %v, want %v", got, want)
	}
}

func TestInstallWithEveryToolPresentCallsNoPackageManager(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	seedLived(t, base)
	ok(t, runInstaller(t, base), "install")
	log := readFile(t, filepath.Join(base, "shims.log"))
	for _, absent := range []string{"brew ", "agent-browser install", "npm "} {
		if strings.Contains(log, absent) {
			t.Errorf("an install with every tool present ran %q:\n%s", absent, log)
		}
	}
	for _, want := range []string{"engram setup claude-code", "claude mcp add --scope user agent-browser", "claude mcp add --scope user engram"} {
		if !strings.Contains(log, want) {
			t.Errorf("the install skipped reconciliation, missing %q", want)
		}
	}
	if got, want := readFileOr(filepath.Join(base, "home", ".claude", "CLAUDE.md")), readFile(t, filepath.Join(base, "checkout", "FLOOR.md")); got != want {
		t.Error("the install did not install the floor")
	}
	if got := recordedTools(t, base); len(got) != 0 {
		t.Errorf("tools present before the install were recorded as its own: %v", got)
	}
}

func TestNamedComponentInstallsOnlyItself(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	home := filepath.Join(base, "home")
	r := ok(t, runInstaller(t, base, "skills"), "skills install")
	if readFileOr(filepath.Join(home, ".claude", "skills", "open-pr", "SKILL.md")) == "" {
		t.Error("the skills install did not copy the skills")
	}
	for _, absent := range []string{".claude/CLAUDE.md", ".claude/agents/reviewer.md", ".claude/settings.json", ".harness/aliases.sh", ".zshrc"} {
		if _, err := os.Stat(filepath.Join(home, absent)); err == nil {
			t.Errorf("the skills install wrote %s", absent)
		}
	}
	if log := readFileOr(filepath.Join(base, "shims.log")); strings.Contains(log, "engram setup") || strings.Contains(log, "mcp add") {
		t.Errorf("the skills install reconciled the MCPs:\n%s", log)
	}
	if !strings.Contains(r.stdout, "installed: skills for claude, codex, opencode") {
		t.Errorf("the summary does not name what was installed:\n%s", r.stdout)
	}
	if got := readFile(t, stateFile(base)); !strings.Contains(got, `"claude": [
      "skills"
    ]`) {
		t.Errorf("the record does not hold skills alone:\n%s", got)
	}

	ok(t, runInstaller(t, base, "floor"), "floor install")
	ok(t, runUninstaller(t, base, "skills", "--yes"), "skills uninstall")
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "open-pr")); err == nil {
		t.Error("the skills uninstall left a skill copy, so the floor install dropped its record")
	}
	if readFileOr(filepath.Join(home, ".claude", "CLAUDE.md")) == "" {
		t.Error("the skills uninstall removed the floor")
	}
}

func TestClientFlagLimitsTheInstall(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	home := filepath.Join(base, "home")
	ok(t, runInstaller(t, base, "--client", "claude"), "Claude Code install")
	if readFileOr(filepath.Join(home, ".claude", "CLAUDE.md")) == "" {
		t.Error("the Claude Code install did not install its floor")
	}
	for _, absent := range []string{".codex/AGENTS.md", ".codex/agents/executor.toml", ".config/opencode/AGENTS.md", ".config/opencode/agents/executor.md"} {
		if _, err := os.Stat(filepath.Join(home, absent)); err == nil {
			t.Errorf("the Claude Code install wrote %s", absent)
		}
	}
	log := readFile(t, filepath.Join(base, "shims.log"))
	for _, absent := range []string{"codex mcp add", "engram setup codex", "engram setup opencode"} {
		if strings.Contains(log, absent) {
			t.Errorf("the Claude Code install ran %q:\n%s", absent, log)
		}
	}
}

func TestMissingClientIsRefusedOnlyWhenNamed(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	home := filepath.Join(base, "home")
	if err := os.Remove(filepath.Join(base, "shims", "opencode")); err != nil {
		t.Fatal(err)
	}
	before := homeTree(t, home)
	r := runInstaller(t, base, "skills", "--client", "opencode")
	if r.code == 0 || !strings.Contains(r.stderr, "harness install clients --client opencode") {
		t.Fatalf("exit %d, stderr %q; want the refusal naming the client install", r.code, r.stderr)
	}
	if got := homeTree(t, home); got != before {
		t.Errorf("the refused run wrote into the home tree:\n%s", got)
	}
	ok(t, runInstaller(t, base, "floor"), "floor install without OpenCode")
	if _, err := os.Stat(filepath.Join(home, ".config", "opencode", "AGENTS.md")); err == nil {
		t.Error("the install configured OpenCode, which is not installed")
	}
	if readFileOr(filepath.Join(home, ".codex", "AGENTS.md")) == "" {
		t.Error("the install skipped Codex, which is installed")
	}
}

func TestMCPInstallReconcilesSettings(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	writeShim(t, base, "engram", `if [ "$*" = "setup claude-code" ]; then
mkdir -p "$HOME/.claude"
printf '%s\n' '{"env":{"ENGRAM_FIXTURE":"preserved"}}' > "$HOME/.claude/settings.json"
fi`)
	r := ok(t, runInstaller(t, base, "mcps"), "mcps install")
	if !strings.Contains(r.stdout, "also reconciling settings") {
		t.Errorf("the mcps install did not say it reconciles settings:\n%s", r.stdout)
	}
	if !strings.Contains(readFileOr(filepath.Join(base, "home", ".claude", "settings.json")), "harness") {
		t.Error("the mcps install left the tracked settings unreconciled")
	}
	if !strings.Contains(readFileOr(filepath.Join(base, "home", ".claude", "settings.json")), `"ENGRAM_FIXTURE": "preserved"`) {
		t.Error("the settings reconcile lost the value written by Engram setup")
	}
}

func TestMCPInstallReadsOpenCode2Config(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		disabled, plugin, fails bool
	}{
		"registered":        {false, true, false},
		"agent-browser off": {true, true, true},
		"no engram plugin":  {false, false, true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			base := sandbox(t)
			if c.plugin {
				writeMode(t, filepath.Join(base, "home", ".config", "opencode", "plugins", "engram.ts"), "", 0o644)
			}
			writeShim(t, base, "opencode", `if [ "$*" = "debug config" ]; then
[ -f /dev/stdout ] || exit 1
printf '%s' '`+strings.Repeat(" ", 70_000)+`'
echo '[{"type": "document", "path": "opencode.json", "info": {"mcp": {"servers": {"agent-browser": {"type": "local", "disabled": `+strconv.FormatBool(c.disabled)+`, "command": ["`+filepath.Join(base, "shims", "agent-browser")+`", "mcp"]}}}}}, {"type": "document", "path": "opencode.jsonc", "info": {"mcp": {"servers": {"engram": {"type": "local"}}}}}, {"type": "directory", "path": "opencode", "info": {}}]'
fi`)
			r := runInstaller(t, base, "mcps")
			if !c.fails {
				ok(t, r, "install")
			} else if r.code == 0 || !strings.Contains(r.stderr, "OpenCode is missing an expected plugin or MCP server") {
				t.Errorf("install exited %d: %s", r.code, r.stderr)
			}
		})
	}
}

func TestBareUninstallKeepsToolsAndTheirRecord(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	seedFull(t, base)
	missingTools(t, base, "uv", "engram")
	ok(t, runInstaller(t, base), "install")
	r := ok(t, runUninstaller(t, base, "--yes"), "bare uninstall")
	if log := readFile(t, filepath.Join(base, "shims.log")); strings.Contains(log, "brew uninstall") {
		t.Errorf("a bare uninstall removed a tool:\n%s", log)
	}
	if _, err := os.Stat(filepath.Join(base, "home", ".claude", "CLAUDE.md")); err == nil {
		t.Error("a bare uninstall left the floor")
	}
	if got := recordedTools(t, base); got["uv"] == "" || got["engram"] == "" {
		t.Errorf("a bare uninstall dropped the tool records: %v", got)
	}
	if strings.Contains(r.stdout, "This also removes ~/.harness") {
		t.Error("a bare uninstall that keeps tool records announced removing ~/.harness")
	}
}

func TestUninstallAllRemovesOnlyRecordedTools(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	home := filepath.Join(base, "home")
	seedFull(t, base)
	writeMode(t, filepath.Join(home, ".engram", "engram.db"), "memories\n", 0o644)
	missingTools(t, base, "uv", "engram")
	ok(t, runInstaller(t, base), "install")
	r := ok(t, runUninstaller(t, base, "--all", "--yes"), "uninstall --all")
	log := readFile(t, filepath.Join(base, "shims.log"))
	for _, want := range []string{"brew uninstall uv", "brew uninstall gentleman-programming/tap/engram"} {
		if !strings.Contains(log, want) {
			t.Errorf("uninstall --all skipped %q:\n%s", want, log)
		}
	}
	for _, absent := range []string{"brew uninstall node", "brew uninstall gh", "brew uninstall agent-browser", "brew uninstall --cask", "npm uninstall"} {
		if strings.Contains(log, absent) {
			t.Errorf("uninstall --all removed a tool present before the harness: %q", absent)
		}
	}
	if !strings.Contains(r.stdout, "node: present before the harness, left installed") {
		t.Errorf("the tools left installed are not named:\n%s", r.stdout)
	}
	if got := readFileOr(filepath.Join(home, ".engram", "engram.db")); got != "memories\n" {
		t.Error("uninstall --all deleted Engram's data")
	}
	for _, gone := range []string{".harness", ".local/bin/harness"} {
		if _, err := os.Stat(filepath.Join(home, gone)); err == nil {
			t.Errorf("uninstall --all left %s", gone)
		}
	}
}

func TestNamedToolUninstallKeepsSharedTools(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	seedFull(t, base)
	missingTools(t, base, "codex", "engram")
	ok(t, runInstaller(t, base), "install")
	r := ok(t, runUninstaller(t, base, "mcps", "clients", "--client", "codex", "--yes"), "Codex mcps and client uninstall")
	log := readFile(t, filepath.Join(base, "shims.log"))
	if !strings.Contains(log, "npm uninstall --global @openai/codex") && !strings.Contains(log, "brew uninstall --cask codex") {
		t.Errorf("the Codex uninstall kept the Codex the harness installed:\n%s", log)
	}
	if strings.Contains(log, "brew uninstall gentleman-programming/tap/engram") {
		t.Errorf("the Codex uninstall removed Engram, which other clients still use:\n%s", log)
	}
	if !strings.Contains(r.stdout, "engram: still used by claude, codex, opencode, left installed") {
		t.Errorf("the kept shared tool is not named:\n%s", r.stdout)
	}
}

func TestUninstallFindsANativeClaudeInLocalBin(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	home := filepath.Join(base, "home")
	if err := os.Remove(filepath.Join(base, "shims", "claude")); err != nil {
		t.Fatal(err)
	}
	writeMode(t, stateFile(base), `{"version":1,"installed":{},"files":[],"tools":{"claude":"native:claude"}}`+"\n", 0o644)
	writeMode(t, filepath.Join(home, ".local", "bin", "claude"), "#!/bin/sh\n", 0o755)
	writeMode(t, filepath.Join(home, ".local", "share", "claude", "versions", "1"), "binary\n", 0o755)
	r := ok(t, runUninstaller(t, base, "clients", "--client", "claude", "--yes"), "Claude Code client uninstall")
	if strings.Contains(r.stdout, "already gone") {
		t.Errorf("the uninstall took the Claude Code in ~/.local/bin for gone:\n%s", r.stdout)
	}
	for _, gone := range []string{".local/bin/claude", ".local/share/claude", ".harness"} {
		if _, err := os.Stat(filepath.Join(home, gone)); err == nil {
			t.Errorf("the Claude Code uninstall left %s", gone)
		}
	}
}

func TestPackageStaysWhileAToolInstalledThroughItIsRecorded(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	shims := filepath.Join(base, "shims")
	writeMode(t, stateFile(base), `{"version":1,"installed":{},"files":[],"tools":{"node":"brew:node","codex":"npm:@openai/codex"}}`+"\n", 0o644)
	writeShim(t, base, "brew", `if [ "$*" = "uninstall node" ]; then rm -f '`+shims+`/node' '`+shims+`/npm'; fi`)
	writeShim(t, base, "npm", `if [ "$*" = "uninstall --global @openai/codex" ]; then rm -f '`+shims+`/codex'; fi`)

	r := ok(t, runUninstaller(t, base, "packages", "--yes"), "packages uninstall with Codex recorded")
	if log := readFileOr(filepath.Join(base, "shims.log")); strings.Contains(log, "brew uninstall node") {
		t.Fatalf("the packages uninstall removed node while the Codex installed through npm is recorded:\n%s", log)
	}
	if !strings.Contains(r.stdout, "node: codex was installed through it, left installed") {
		t.Errorf("the kept package does not say why:\n%s", r.stdout)
	}
	if got := recordedTools(t, base); got["node"] == "" {
		t.Errorf("the packages uninstall dropped node's record: %v", got)
	}

	ok(t, runUninstaller(t, base, "clients", "--yes"), "clients uninstall")
	if log := readFile(t, filepath.Join(base, "shims.log")); !strings.Contains(log, "npm uninstall --global @openai/codex") {
		t.Fatalf("the clients uninstall did not remove Codex through npm:\n%s", log)
	}
	if got := recordedTools(t, base); got["codex"] != "" {
		t.Errorf("the clients uninstall kept Codex's record: %v", got)
	}

	ok(t, runUninstaller(t, base, "packages", "--yes"), "packages uninstall after Codex")
	if log := readFile(t, filepath.Join(base, "shims.log")); !strings.Contains(log, "brew uninstall node") {
		t.Errorf("node stayed after the Codex installed through it was removed:\n%s", log)
	}
	if _, err := os.Stat(filepath.Join(base, "home", ".harness")); err == nil {
		t.Error("the last tool removal left ~/.harness")
	}
}

func TestOldComponentNamesAreMigrated(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	ok(t, runInstaller(t, base), "install")
	old := strings.NewReplacer(`"floor"`, `"policy"`, `"mcps"`, `"mcp"`).Replace(readFile(t, stateFile(base)))
	writeMode(t, stateFile(base), old, 0o644)
	ok(t, runUninstaller(t, base, "floor", "--client", "claude", "--yes"), "floor uninstall over old names")
	if _, err := os.Stat(filepath.Join(base, "home", ".claude", "CLAUDE.md")); err == nil {
		t.Error("the floor recorded as policy survived its uninstall")
	}
	if got := readFile(t, stateFile(base)); strings.Contains(got, `"policy"`) || strings.Contains(got, `"mcp"`) {
		t.Errorf("the state kept the old names:\n%s", got)
	}
}

func TestInstallAddsClaudeOnlyWhenAbsent(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	seedFull(t, base)
	shims, log := filepath.Join(base, "shims"), filepath.Join(base, "shims.log")
	claude := filepath.Join(shims, "claude")
	saved := filepath.Join(base, "claude.saved")
	writeMode(t, saved, readFile(t, claude), 0o755)
	if err := os.Remove(claude); err != nil {
		t.Fatal(err)
	}

	writeMode(t, filepath.Join(shims, "bash"), "#!/bin/sh\nprintf '%s\\n' \"bash $*\" >> '"+log+"'\ncp '"+saved+"' '"+claude+"'\nchmod +x '"+claude+"'\nexit 0\n", 0o755)

	ok(t, runInstaller(t, base), "full install without claude")

	logged := readFile(t, log)
	if got := strings.Count(logged, "claude.ai/install.sh"); got != 1 {
		t.Errorf("claude absent ran the native installer %d times, want exactly 1:\n%s", got, logged)
	}
	if _, err := os.Stat(claude); err != nil {
		t.Errorf("the native installer left claude off PATH: %v", err)
	}
}

func TestInstallRefusesMissingOrRelativeLocations(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		env  []string
	}{
		{"missing-home", []string{"HOME="}},
		{"relative-home", []string{"HOME=relative"}},
		{"relative-config", []string{"XDG_CONFIG_HOME=relative"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			base := sandbox(t)
			home := filepath.Join(base, "home")
			before := homeTree(t, home)
			r := runHarness(t, base, c.env, "install")
			if r.code == 0 || !strings.Contains(r.stderr, "must be absolute") {
				t.Fatalf("exit %d, stderr %q; want a refusal naming the absolute-path requirement", r.code, r.stderr)
			}
			if got := homeTree(t, home); got != before {
				t.Errorf("a refused run wrote into the home tree:\n%s", got)
			}
			if raw, err := os.ReadFile(filepath.Join(base, "shims.log")); err == nil && strings.TrimSpace(string(raw)) != "" {
				t.Errorf("a refused run spawned a subprocess:\n%s", raw)
			}
		})
	}
}

func homeTree(t *testing.T, home string) string {
	t.Helper()
	var b strings.Builder
	tree(t, &b, home, true)
	return b.String()
}

func TestReinstallRewritesNothingUnchanged(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	ok(t, runInstaller(t, base), "install")
	home := filepath.Join(base, "home")
	before := homeTree(t, home)
	r := runInstaller(t, base)
	ok(t, r, "reinstall")
	if after := homeTree(t, home); after != before {
		t.Errorf("the reinstall changed the home tree:\n--- before\n%s\n--- after\n%s", before, after)
	}
	for _, noise := range []string{"Installed ", "Backed up ", "Removed "} {
		if strings.Contains(r.stdout, noise) {
			t.Errorf("the reinstall logged %q:\n%s", noise, r.stdout)
		}
	}
}

func TestInstallWithSymlinkedTmpdir(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	link := filepath.Join(base, "tmp-link")
	symlink(t, filepath.Join(base, "tmp"), link)
	r := runHarnessInput(t, base, "", []string{"TMPDIR=" + link, "HARNESS_ASSETS=" + filepath.Join(base, "checkout")}, "install")
	ok(t, r, "install")
}

func TestRetiredAssetsPreserveEditsAndPrivateWorktrees(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	home := filepath.Join(base, "home")
	source := filepath.Join(base, "checkout", "skills", "ops", "retired")
	guards := "disable-model-invocation: true\nmetadata:\n  opencode/autoinvoke: \"false\"\n"
	sidecar := "policy:\n  allow_implicit_invocation: false\n"
	writeMode(t, filepath.Join(source, "SKILL.md"), "---\ndescription: retired\n"+guards+"---\n", 0o644)
	writeMode(t, filepath.Join(source, "agents", "openai.yaml"), sidecar, 0o644)
	linkedSource := filepath.Join(base, "checkout", "skills", "ops", "retired-linked")
	writeMode(t, filepath.Join(linkedSource, "SKILL.md"), "---\ndescription: retired too\n"+guards+"---\n", 0o644)
	writeMode(t, filepath.Join(linkedSource, "agents", "openai.yaml"), sidecar, 0o644)
	ok(t, runInstaller(t, base, "skills", "aliases"), "install asset")
	edited := filepath.Join(home, ".claude", "skills", "retired", "SKILL.md")
	writeMode(t, edited, "my edited skill\n", 0o644)
	main := filepath.Join(home, "repositories", "main")
	writeMode(t, filepath.Join(main, "tracked.txt"), "existing repository\n", 0o644)
	git(t, main, "init", "-q", "-b", "main")
	git(t, main, "add", "tracked.txt")
	git(t, main, "commit", "-qm", "existing work")
	worktree := filepath.Join(home, "worktrees", "mine")
	git(t, main, "worktree", "add", "-b", "user-work", worktree)
	beforeWorktrees := git(t, main, "worktree", "list", "--porcelain")
	uncommitted := filepath.Join(home, "worktrees", "mine", "uncommitted.txt")
	writeMode(t, uncommitted, "user work\n", 0o644)
	writeMode(t, filepath.Join(home, ".zshrc"), readFile(t, filepath.Join(home, ".zshrc"))+"# private worktrunk integration\n", 0o644)
	replaced := filepath.Join(home, ".claude", "skills", "retired-linked")
	if err := os.RemoveAll(replaced); err != nil {
		t.Fatal(err)
	}
	symlink(t, uncommitted, replaced)
	if err := os.RemoveAll(source); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(linkedSource); err != nil {
		t.Fatal(err)
	}
	ok(t, runInstaller(t, base, "skills", "aliases"), "retire asset")
	if _, err := os.Lstat(filepath.Join(home, ".agents", "skills", "retired")); !os.IsNotExist(err) {
		t.Fatalf("unchanged retired copy remains: %v", err)
	}
	if got := readFile(t, edited); got != "my edited skill\n" {
		t.Fatalf("edited asset = %q", got)
	}
	if !strings.Contains(readFile(t, filepath.Join(home, ".zshrc")), "# private worktrunk integration\n") {
		t.Fatal("private shell integration removed")
	}
	if after := git(t, main, "worktree", "list", "--porcelain"); after != beforeWorktrees {
		t.Fatal("existing worktrees changed")
	}
	if !isSymlink(replaced) {
		t.Fatal("a recorded copy the user replaced with a symlink was removed")
	}
	if got := readFile(t, replaced); got != "user work\n" {
		t.Fatalf("retirement wrote through the user's symlink: %q", got)
	}
}

func TestCompactionHooksKeepTheUsersOwn(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	home := filepath.Join(base, "home")
	settings := filepath.Join(home, ".claude", "settings.json")
	user := `{"matcher": "startup", "hooks": [{"type": "command", "command": "echo mine"}]}`
	writeMode(t, settings, `{"hooks": {"SessionStart": [`+user+`, {"matcher": "compact", "hooks": [{"type": "command", "command": "\"$HOME/.local/bin/harness\" retired"}]}], "Stop": [`+user+`]}}`+"\n", 0o644)
	ok(t, runInstaller(t, base, "settings"), "install")

	var got struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	decode := func() {
		got.Hooks = nil
		if err := json.Unmarshal([]byte(readFile(t, settings)), &got); err != nil {
			t.Fatal(err)
		}
	}
	decode()
	var commands []string
	for _, group := range got.Hooks["SessionStart"] {
		commands = append(commands, group.Matcher+": "+group.Hooks[0].Command)
	}
	if want := []string{"startup: echo mine", `compact: "$HOME/.local/bin/harness" resume`}; !slices.Equal(commands, want) {
		t.Errorf("SessionStart after install = %q, want %q", commands, want)
	}
	if len(got.Hooks["Stop"]) != 1 {
		t.Errorf("the user's Stop hook changed: %+v", got.Hooks["Stop"])
	}
	for _, path := range []string{filepath.Join(home, ".codex", "hooks.json"), filepath.Join(home, ".config", "opencode", "plugins", "harness-resume.ts")} {
		if !strings.Contains(readFileOr(path), "harness") {
			t.Errorf("%s is missing its compaction hook", path)
		}
	}

	ok(t, runUninstaller(t, base, "settings", "--yes"), "uninstall")
	decode()
	if groups := got.Hooks["SessionStart"]; len(groups) != 1 || groups[0].Matcher != "startup" || len(got.Hooks["Stop"]) != 1 {
		t.Errorf("the uninstall did not leave exactly the user's hooks: %+v", got.Hooks)
	}
	for _, path := range []string{filepath.Join(home, ".codex", "hooks.json"), filepath.Join(home, ".config", "opencode", "plugins", "harness-resume.ts")} {
		if _, err := os.Stat(path); err == nil {
			t.Errorf("the uninstall kept %s", path)
		}
	}
}

func TestCodexHooksKeepTheUsersOwn(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	path := filepath.Join(base, "home", ".codex", "hooks.json")
	writeMode(t, path, `{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "echo mine"}]}]}}`+"\n", 0o644)
	ok(t, runInstaller(t, base, "settings"), "install")
	text := readFileOr(path)
	if !strings.Contains(text, "echo mine") || !strings.Contains(text, `harness\" resume`) {
		t.Fatalf("after install, hooks.json lacks the user's hook or the harness hook:\n%s", text)
	}
	ok(t, runUninstaller(t, base, "settings", "--yes"), "uninstall")
	text = readFileOr(path)
	if !strings.Contains(text, "echo mine") || strings.Contains(text, "resume") {
		t.Errorf("after uninstall, hooks.json should hold only the user's hook:\n%s", text)
	}
}

func TestClaudeAllowKeepsTheUsersRules(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	settings := filepath.Join(base, "home", ".claude", "settings.json")
	writeMode(t, settings, `{"permissions": {"allow": ["Bash(make test)"], "defaultMode": "plan"}}`+"\n", 0o644)
	allow := func() []string {
		var got struct {
			Permissions struct {
				Allow []string `json:"allow"`
			} `json:"permissions"`
		}
		if err := json.Unmarshal([]byte(readFile(t, settings)), &got); err != nil {
			t.Fatal(err)
		}
		return got.Permissions.Allow
	}
	ok(t, runInstaller(t, base, "settings"), "install")
	if got, want := allow(), []string{"Bash(make test)", "Read(~/.claude/skills/**)"}; !slices.Equal(got, want) {
		t.Errorf("allow after install = %q, want %q", got, want)
	}
	ok(t, runInstaller(t, base, "settings"), "reinstall")
	if got := allow(); len(got) != 2 {
		t.Errorf("a reinstall duplicated the tracked rule: %q", got)
	}
	ok(t, runUninstaller(t, base, "settings", "--yes"), "uninstall")
	if got, want := allow(), []string{"Bash(make test)"}; !slices.Equal(got, want) {
		t.Errorf("allow after uninstall = %q, want %q", got, want)
	}
}

func TestFloorRulesReachCodexAndOpenCode(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	home := filepath.Join(base, "home")
	rules := filepath.Join(home, ".codex", "rules", "harness.rules")
	config := filepath.Join(home, ".config", "opencode", "opencode.json")
	writeMode(t, config, `{"theme": "mine", "permission": {"bash": "ask", "read": {"*.md": "allow"}, "webfetch": "deny"}}`+"\n", 0o644)
	type permission struct {
		Bash, Read, Edit map[string]string
		Webfetch         string
	}
	read := func() (permission, map[string][]string) {
		data := []byte(readFile(t, config))
		var doc struct {
			Theme      string     `json:"theme"`
			Permission permission `json:"permission"`
		}
		if err := json.Unmarshal(data, &doc); err != nil || doc.Theme != "mine" {
			t.Fatalf("opencode.json lost the user's keys (%v):\n%s", err, data)
		}
		var raw struct {
			Permission map[string]json.RawMessage `json:"permission"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Fatal(err)
		}
		order := map[string][]string{}
		for class, value := range raw.Permission {
			dec := json.NewDecoder(strings.NewReader(string(value)))
			if tok, _ := dec.Token(); tok != json.Delim('{') {
				continue
			}
			for dec.More() {
				key, _ := dec.Token()
				var skip json.RawMessage
				if err := dec.Decode(&skip); err != nil {
					t.Fatal(err)
				}
				order[class] = append(order[class], key.(string))
			}
		}
		return doc.Permission, order
	}

	ok(t, runInstaller(t, base, "settings"), "install")
	if text := readFileOr(rules); !strings.Contains(text, `"git", "push", ["--force", "-f"]`) {
		t.Errorf("the Codex rules file is missing or lacks the force-push rule:\n%s", text)
	}
	got, order := read()
	if got.Bash["*"] != "ask" || order["bash"][0] != "*" || got.Bash["git push --force *"] != "deny" || got.Bash["gh pr merge *"] != "ask" {
		t.Errorf("bash after install = %v in order %q, want the user's action first and the harness's rules after it", got.Bash, order["bash"])
	}
	if order["read"][0] != "*.md" || got.Read["*/.env*"] != "deny" || got.Edit["*/.ssh/*"] != "deny" || got.Webfetch != "deny" {
		t.Errorf("read, edit, or webfetch after install = %v, %v, %q", order["read"], got.Edit, got.Webfetch)
	}
	tracked := len(order["bash"])

	text := readFile(t, config)
	last := `"git worktree remove -f *": "ask"`
	if !strings.Contains(text, last) {
		t.Fatalf("opencode.json lacks the last harness rule %s:\n%s", last, text)
	}
	writeMode(t, config, strings.Replace(text, last, last+`, "git *": "allow"`, 1), 0o644)
	ok(t, runInstaller(t, base, "settings"), "reinstall")
	_, order = read()
	if len(order["bash"]) != tracked+1 || order["bash"][1] != "git *" || order["bash"][len(order["bash"])-1] != "git worktree remove -f *" {
		t.Errorf("bash after reinstall = %q, want the user's rules first and each harness rule once, last", order["bash"])
	}

	ok(t, runUninstaller(t, base, "settings", "--yes"), "uninstall")
	got, _ = read()
	if len(got.Bash) != 2 || got.Bash["git *"] != "allow" || len(got.Read) != 1 || got.Edit != nil || got.Webfetch != "deny" {
		t.Errorf("permission after uninstall = %+v, want only the user's rules", got)
	}
	if _, err := os.Stat(rules); err == nil {
		t.Errorf("the uninstall kept %s", rules)
	}
}

func TestOpenCodeKeepsAStricterUserRule(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	config := filepath.Join(base, "home", ".config", "opencode", "opencode.json")
	writeMode(t, config, `{"permission": {"bash": {"gh pr merge *": "deny"}}}`+"\n", 0o644)
	ok(t, runInstaller(t, base, "settings"), "install")
	if text := readFile(t, config); !strings.Contains(text, `"gh pr merge *": "deny"`) {
		t.Errorf("the install weakened the user's deny:\n%s", text)
	}
	ok(t, runUninstaller(t, base, "settings", "--yes"), "uninstall")
	if text := readFile(t, config); !strings.Contains(text, `"gh pr merge *": "deny"`) || strings.Contains(text, "git push") {
		t.Errorf("the uninstall should leave only the user's deny:\n%s", text)
	}
}

func TestInstallsOpenCodeV2(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	seedFull(t, base)
	missingTools(t, base, "opencode")
	ok(t, runInstaller(t, base), "full install")
	if got := recordedTools(t, base)["opencode"]; got != "brew:anomalyco/tap/opencode-v2" {
		t.Errorf("recorded %q for OpenCode, want the v2 formula, the first that honors the skill guard", got)
	}
}

func TestOpenCodeAsksBeforeAHarnessMerge(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	ok(t, runInstaller(t, base, "settings"), "install")
	if text := readFile(t, filepath.Join(base, "home", ".config", "opencode", "opencode.json")); !strings.Contains(text, `"harness land merge *": "ask"`) {
		t.Errorf("harness land merge runs without asking:\n%s", text)
	}
}

func TestOpenCodeAsksOutsideTheProject(t *testing.T) {
	t.Parallel()
	const ask, skills = `"*": "ask"`, `"~/.agents/skills/*": "allow"`
	for name, c := range map[string]struct {
		user    string
		inOrder []string
		absent  string
	}{
		"no setting":              {`{}`, []string{ask, skills}, ""},
		"the user's path wins":    {`{"permission": {"external_directory": {"~/work/*": "allow"}}}`, []string{ask, skills, `"~/work/*": "allow"`}, ""},
		"the user's skill rule":   {`{"permission": {"external_directory": {"~/.agents/skills/*": "deny"}}}`, []string{ask, `"~/.agents/skills/*": "deny"`}, skills},
		"the user allows all":     {`{"permission": {"external_directory": "allow"}}`, []string{`"external_directory": "allow"`}, skills},
		"the user denies all":     {`{"permission": {"external_directory": {"*": "deny"}}}`, []string{`"*": "deny"`}, skills},
		"the user denies any use": {`{"permission": "deny"}`, []string{`"*": "deny"`}, skills},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			base := sandbox(t)
			config := filepath.Join(base, "home", ".config", "opencode", "opencode.json")
			writeMode(t, config, c.user+"\n", 0o644)
			ok(t, runInstaller(t, base, "settings"), "install")
			installed := readFile(t, config)
			rest := installed
			if i := strings.Index(rest, `"external_directory"`); i >= 0 {
				rest = rest[i:]
			}
			for _, want := range c.inOrder {
				i := strings.Index(rest, want)
				if i < 0 {
					t.Fatalf("want %q in that order:\n%s", c.inOrder, installed)
				}
				rest = rest[i+len(want):]
			}
			if c.absent != "" && strings.Contains(installed, c.absent) {
				t.Errorf("the install widened the user's setting with %s:\n%s", c.absent, installed)
			}
			ok(t, runInstaller(t, base, "settings"), "reinstall")
			if again := readFile(t, config); again != installed {
				t.Errorf("a reinstall changed the config:\n%s", again)
			}
			ok(t, runUninstaller(t, base, "settings", "--yes"), "uninstall")
			left := readFileOr(config)
			if strings.Contains(left, skills) {
				t.Errorf("the uninstall left a harness rule:\n%s", left)
			}
			if last := c.inOrder[len(c.inOrder)-1]; last != skills && !strings.Contains(left, last) {
				t.Errorf("the uninstall dropped the user's %s:\n%s", last, left)
			}
		})
	}
}

func externalDirectory(t *testing.T, config string) string {
	t.Helper()
	var doc struct {
		Permission json.RawMessage `json:"permission"`
	}
	if err := json.Unmarshal([]byte(readFileOr(config)), &doc); err != nil {
		return ""
	}
	var classes map[string]json.RawMessage
	if json.Unmarshal(doc.Permission, &classes) != nil {
		return ""
	}
	var out bytes.Buffer
	if raw, ok := classes["external_directory"]; ok {
		_ = json.Compact(&out, raw)
	}
	return out.String()
}

func TestOpenCodeKeepsTheUsersExternalRules(t *testing.T) {
	t.Parallel()
	const skills = `"~/.agents/skills/*":"allow"`
	for name, c := range map[string]struct {
		user               string
		edit               func(string) string
		install, uninstall string
		order              []string
	}{
		"user rules keep their order after every added rule": {
			user:      `{"permission":{"external_directory":{"~/.agents/*":"allow","~/.agents/skills/*":"deny"}}}`,
			order:     []string{`"/private/var/folders/*/T/opencode/*":"allow"`, `"~/.agents/*":"allow"`, `"~/.agents/skills/*":"deny"`},
			uninstall: `{"~/.agents/*":"allow","~/.agents/skills/*":"deny"}`,
		},
		"the user's ask survives": {
			user:    `{"permission":{"external_directory":"ask"}}`,
			install: `"ask"`, uninstall: `"ask"`,
		},
		"the user's catch-all key survives": {
			user:    `{"permission":{"external_directory":{"*":"allow"}}}`,
			install: `{"*":"allow"}`, uninstall: `{"*":"allow"}`,
		},
		"a top-level ask is the user's catch-all": {
			user:    `{"permission":{"*":"ask"}}`,
			install: ``, uninstall: ``,
		},
		"an edited generated rule survives": {
			user:      `{}`,
			edit:      func(s string) string { return strings.Replace(s, `"*": "ask"`, `"*": "deny"`, 1) },
			order:     []string{`"*":"deny"`, skills},
			uninstall: `{"*":"deny"}`,
		},
		"a user's rule equal to a tracked one survives": {
			user:      `{"permission":{"external_directory":{"~/.agents/skills/*":"allow"}}}`,
			order:     []string{`"*":"ask"`, skills},
			uninstall: `{"~/.agents/skills/*":"allow"}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			base := sandbox(t)
			config := filepath.Join(base, "home", ".config", "opencode", "opencode.json")
			writeMode(t, config, c.user+"\n", 0o644)
			ok(t, runInstaller(t, base, "settings", "--client", "opencode"), "install")
			if c.edit != nil {
				writeMode(t, config, c.edit(readFile(t, config)), 0o644)
			}
			installed := externalDirectory(t, config)
			if c.install != "" || len(c.order) == 0 {
				if installed != c.install {
					t.Errorf("the install left external_directory as %s, want %s", installed, c.install)
				}
			}
			rest := installed
			for _, want := range c.order {
				i := strings.Index(rest, want)
				if i < 0 {
					t.Fatalf("want %s in that order:\n%s", c.order, installed)
				}
				rest = rest[i+len(want):]
			}
			whole := readFile(t, config)
			ok(t, runInstaller(t, base, "settings", "--client", "opencode"), "reinstall")
			if readFile(t, config) != whole {
				t.Errorf("a reinstall changed the config:\n%s", readFile(t, config))
			}
			ok(t, runUninstaller(t, base, "settings", "--client", "opencode", "--yes"), "uninstall")
			if left := externalDirectory(t, config); left != c.uninstall {
				t.Errorf("the uninstall left external_directory as %s, want %s", left, c.uninstall)
			}
		})
	}
}

func TestOpenCodeDefaultsOutliveLaterEdits(t *testing.T) {
	t.Parallel()
	settings := []string{"settings", "--client", "opencode"}
	const ask, skill = `"*": "ask",`, `"~/.agents/skills/*": "allow",`
	for name, c := range map[string]struct {
		edit      func(string) string
		reinstall bool
		uninstall string
	}{
		"deleted catch-all": {func(s string) string { return strings.Replace(s, ask, "", 1) }, true, ""},
		"deleted allow":     {func(s string) string { return strings.Replace(s, skill, "", 1) }, true, ""},
		"user rule first":   {func(s string) string { return strings.Replace(s, ask, `"~/work/*": "allow", `+ask, 1) }, true, `{"~/work/*":"allow"}`},
		"deleted catch-all under a top-level deny": {func(s string) string {
			return strings.Replace(strings.Replace(s, ask, "", 1), `"permission": {`, `"permission": {"*": "deny",`, 1)
		}, false, ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			base := sandbox(t)
			config := filepath.Join(base, "home", ".config", "opencode", "opencode.json")
			ok(t, runInstaller(t, base, settings...), "install")
			writeMode(t, config, c.edit(readFile(t, config)), 0o644)
			if c.reinstall {
				ok(t, runInstaller(t, base, settings...), "reinstall")
				installed := externalDirectory(t, config)
				askAt, allowAt := strings.Index(installed, `"*":"ask"`), strings.Index(installed, `"~/.agents/skills/*":"allow"`)
				if askAt < 0 || (allowAt >= 0 && allowAt < askAt) {
					t.Errorf("the reinstall left %s, want the ask ahead of any allow it covers", installed)
				}
			}
			ok(t, runUninstaller(t, base, append(settings, "--yes")...), "uninstall")
			if left := externalDirectory(t, config); left != c.uninstall {
				t.Errorf("the uninstall left external_directory as %q, want %q", left, c.uninstall)
			}
		})
	}
}

func TestCodexUninstallKeepsTheOpenCodeRecord(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	home := filepath.Join(base, "home")
	ok(t, runInstaller(t, base, "settings"), "install")
	if err := os.Remove(filepath.Join(home, ".codex", "hooks.json")); err != nil {
		t.Fatal(err)
	}
	ok(t, runUninstaller(t, base, "settings", "--client", "codex", "--yes"), "codex uninstall")
	ok(t, runUninstaller(t, base, "settings", "--client", "opencode", "--yes"), "opencode uninstall")
	if left := externalDirectory(t, filepath.Join(home, ".config", "opencode", "opencode.json")); left != "" {
		t.Errorf("uninstalling Codex lost the OpenCode record, so this stayed: %s", left)
	}
}

func runWithShell(t *testing.T, base, shell string, args ...string) result {
	t.Helper()
	return runHarness(t, base, []string{"TMPDIR=" + filepath.Join(base, "tmp"), "HARNESS_ASSETS=" + filepath.Join(base, "checkout"), "SHELL=" + shell}, args...)
}

func TestAliasesGoToTheLoginShellsStartupFile(t *testing.T) {
	t.Parallel()
	bash := ".bashrc"
	if runtime.GOOS == "darwin" {
		bash = ".bash_profile"
	}
	for shell, rc := range map[string]string{"/bin/bash": bash, "/bin/zsh": ".zshrc"} {
		t.Run(filepath.Base(shell), func(t *testing.T) {
			t.Parallel()
			base := sandbox(t)
			home := filepath.Join(base, "home")
			ok(t, runWithShell(t, base, shell, "install", "aliases"), "aliases install")
			line := `[ -f "` + filepath.Join(home, ".harness", "aliases.sh") + `" ] && . "`
			if got := readFileOr(filepath.Join(home, rc)); !strings.Contains(got, "# portable-harness aliases\n"+line) {
				t.Errorf("%s lacks the managed line:\n%s", rc, got)
			}
			for _, other := range []string{".zshrc", ".bashrc", ".bash_profile", ".profile"} {
				if _, err := os.Stat(filepath.Join(home, other)); other != rc && err == nil {
					t.Errorf("the install wrote %s, which a %s login shell does not read", other, filepath.Base(shell))
				}
			}
			ok(t, runWithShell(t, base, shell, "uninstall", "aliases", "--yes"), "aliases uninstall")
			if strings.Contains(readFileOr(filepath.Join(home, rc)), "portable-harness") {
				t.Errorf("the uninstall left the managed line in %s", rc)
			}
		})
	}
}

func TestAliasesWarnForAnotherShell(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	home := filepath.Join(base, "home")
	r := ok(t, runWithShell(t, base, "/usr/bin/fish", "install", "aliases"), "aliases install under fish")
	if !strings.Contains(r.stderr, filepath.Join(home, ".harness", "aliases.sh")) || strings.Contains(r.stderr, "harness-assets") {
		t.Errorf("the warning does not name the installed aliases:\n%s", r.stderr)
	}
	for _, rc := range []string{".zshrc", ".bashrc", ".bash_profile", ".profile"} {
		if _, err := os.Stat(filepath.Join(home, rc)); err == nil {
			t.Errorf("the install wrote %s for a fish login shell", rc)
		}
	}
}

func TestUninstallKeepsAliasesItNeverWrote(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	home := filepath.Join(base, "home")
	mine := "alias ccyolo='claude --my-flags'\n"
	writeMode(t, filepath.Join(home, ".bashrc"), mine, 0o644)
	ok(t, runWithShell(t, base, "/bin/zsh", "install", "aliases"), "aliases install")
	ok(t, runWithShell(t, base, "/bin/zsh", "uninstall", "aliases", "--yes"), "aliases uninstall")
	if got := readFile(t, filepath.Join(home, ".bashrc")); got != mine {
		t.Errorf("the uninstall changed a startup file the harness never wrote: %q", got)
	}
}

func TestMacBashUsesTheExistingProfile(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "darwin" {
		t.Skip("bash login shells read ~/.bashrc on Linux")
	}
	base := sandbox(t)
	home := filepath.Join(base, "home")
	writeMode(t, filepath.Join(home, ".profile"), "export KEEP=1\n", 0o644)
	ok(t, runWithShell(t, base, "/bin/bash", "install", "aliases"), "aliases install")
	if !strings.Contains(readFile(t, filepath.Join(home, ".profile")), "portable-harness") {
		t.Error("the managed line did not join ~/.profile")
	}
	if _, err := os.Stat(filepath.Join(home, ".bash_profile")); err == nil {
		t.Error("the install created ~/.bash_profile, which hides ~/.profile")
	}
}

func TestSkillOverridesKeepTheUsersOwn(t *testing.T) {
	t.Parallel()
	base := sandbox(t)
	settings := filepath.Join(base, "home", ".claude", "settings.json")
	writeMode(t, settings, `{"skillOverrides": {"mine": "off", "tdd": "on"}}`+"\n", 0o644)
	overrides := func() map[string]string {
		var got struct {
			SkillOverrides map[string]string `json:"skillOverrides"`
		}
		if err := json.Unmarshal([]byte(readFile(t, settings)), &got); err != nil {
			t.Fatal(err)
		}
		return got.SkillOverrides
	}
	ok(t, runInstaller(t, base, "settings"), "install")
	got := overrides()
	if got["mine"] != "off" || got["tdd"] != "on" || got["impeccable"] != "user-invocable-only" || got["grilling"] != "name-only" {
		t.Errorf("skillOverrides after install = %v", got)
	}
	ok(t, runInstaller(t, base, "settings"), "reinstall")
	ok(t, runUninstaller(t, base, "settings", "--yes"), "uninstall")
	if got := overrides(); len(got) != 2 || got["mine"] != "off" || got["tdd"] != "on" {
		t.Errorf("skillOverrides after uninstall = %v, want only the user's own", got)
	}
}
