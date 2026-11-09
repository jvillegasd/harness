package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// agentOutputs are the role agents this checkout renders, per client.
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

// readFileOr is the file's text, or "" for anything unreadable.
func readFileOr(path string) string {
	data, _ := os.ReadFile(path)
	return string(data)
}

// readDir names a directory's entries, or none for a missing one.
func readDir(dir string) []string {
	list, _ := os.ReadDir(dir)
	var out []string
	for _, e := range list {
		out = append(out, e.Name())
	}
	return out
}

// Every guarded skill is installed in the shared namespace used by both clients.
func TestGuardedSkillsAreInstalled(t *testing.T) {
	base := sandbox(t)
	ok(t, runInstaller(t, base, "--skip-packages"), "skip-packages install")
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

// TestInstallRefusesAnUnguardedLocalSkill stops the install before it writes anything when a skill absent from the lock lacks an invocation guard. Which frontmatter shapes count as guarded is settled at the parser, in internal/skills.
func TestInstallRefusesAnUnguardedLocalSkill(t *testing.T) {
	base := sandbox(t)
	writeMode(t, filepath.Join(base, "checkout", "skills", "workflow", "zz-bare", "SKILL.md"), "---\nname: zz-bare\ndisable-model-invocation: true\n---\nbody\n", 0o644)
	var before strings.Builder
	tree(t, &before, filepath.Join(base, "home"), true)
	r := runInstaller(t, base, "--skip-packages")
	if r.code == 0 || !strings.Contains(r.stderr, "zz-bare") || !strings.Contains(r.stderr, "opencode/autoinvoke") {
		t.Errorf("exit %d, stderr %q; want a refusal naming the skill and the key", r.code, r.stderr)
	}
	var after strings.Builder
	tree(t, &after, filepath.Join(base, "home"), true)
	if before.String() != after.String() {
		t.Error("the refused install wrote to HOME")
	}
}

// TestInstallExplainsAMissingLock names why the lock is needed when a HARNESS_ASSETS tree lacks it.
func TestInstallExplainsAMissingLock(t *testing.T) {
	base := sandbox(t)
	if err := os.Remove(filepath.Join(base, "checkout", "skills-lock.json")); err != nil {
		t.Fatal(err)
	}
	if r := runInstaller(t, base, "--skip-packages"); r.code == 0 || !strings.Contains(r.stderr, "separates local skills from locked ones") {
		t.Errorf("exit %d, stderr %q; want the lock's purpose named", r.code, r.stderr)
	}
}

// Role agents are copied into each client beside private entries, and the uninstall removes only the recorded copies.
func TestAgentInstallAndUninstallTouchOnlyRecordedCopies(t *testing.T) {
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

	ok(t, runInstaller(t, base, "--skip-packages"), "install")
	for _, rel := range agentOutputs {
		client, name, _ := strings.Cut(rel, "/")
		path := filepath.Join(roots[client], name)
		if isSymlink(path) || strings.Contains(readFileOr(path), "private-secret") || readFileOr(path) == "" {
			t.Errorf("%s is not a clean copy", path)
		}
	}
	untouched("the install")

	ok(t, runInstaller(t, base, "--uninstall", "--yes"), "uninstall")
	for _, root := range roots {
		for _, entry := range readDir(root) {
			if entry != "mine.md" && entry != "foreign.md" {
				t.Errorf("the uninstall left %s in %s", entry, root)
			}
		}
	}
	untouched("the uninstall")
}

// An unrecorded entry at a role agent path is archived before the copy lands, never overwritten in place.
func TestAgentInstallArchivesUnrecordedCollisions(t *testing.T) {
	for _, kind := range []string{"file", "foreign-link", "root-link"} {
		t.Run(kind, func(t *testing.T) {
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
			ok(t, runInstaller(t, base, "--skip-packages"), "install")
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

// A copy edited after the install is the user's: the uninstall keeps it and says so.
func TestUninstallKeepsAnEditedCopy(t *testing.T) {
	base := sandbox(t)
	ok(t, runInstaller(t, base, "--skip-packages"), "install")
	edited := filepath.Join(base, "home", ".claude", "skills", "open-pr", "SKILL.md")
	writeMode(t, edited, "my open-pr\n", 0o644)
	r := runInstaller(t, base, "--uninstall", "--yes")
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
	base := sandbox(t)
	ok(t, runInstaller(t, base, "--skip-packages"), "install")
	copy := filepath.Join(base, "home", ".claude", "agents", "reviewer.md")
	before := readFile(t, copy)
	r := runInstallerInput(t, base, "n\n", "--uninstall")
	ok(t, r, "declined uninstall")
	if got := readFileOr(copy); got != before {
		t.Errorf("declined uninstall changed the installed copy: %q", got)
	}
}

func TestUninstallOpenCodeLeavesClaudeCopies(t *testing.T) {
	base := sandbox(t)
	ok(t, runInstaller(t, base, "--skip-packages"), "install")
	claude := filepath.Join(base, "home", ".claude", "agents", "reviewer.md")
	ok(t, runInstaller(t, base, "--uninstall", "--client", "opencode", "--yes"), "OpenCode uninstall")
	if readFileOr(claude) == "" {
		t.Error("OpenCode uninstall removed the Claude role copy")
	}
	if _, err := os.Stat(filepath.Join(base, "home", ".config", "opencode", "agents", "executor.md")); err == nil {
		t.Error("OpenCode uninstall kept its role copy")
	}
}

// Codex is configured in ~/.codex even when the terminal points CODEX_HOME elsewhere, as a launcher may, and its uninstall leaves the copies the other clients still use.
func TestCodexInstallsIntoHomeCodexAndUninstallsAlone(t *testing.T) {
	base := sandbox(t)
	elsewhere := filepath.Join(base, "launcher-codex-home")
	extra := []string{"TMPDIR=" + filepath.Join(base, "tmp"), "HARNESS_ASSETS=" + filepath.Join(base, "checkout"), "CODEX_HOME=" + elsewhere}
	ok(t, runHarness(t, base, extra, "install", "--skip-packages"), "install under another CODEX_HOME")
	codexHome := filepath.Join(base, "home", ".codex")
	if readFile(t, filepath.Join(codexHome, "AGENTS.md")) != readFile(t, filepath.Join(base, "checkout", "POLICY.md")) {
		t.Error("~/.codex/AGENTS.md is not the policy")
	}
	if !strings.Contains(readFile(t, filepath.Join(codexHome, "agents", "executor.toml")), "\nmodel = \"gpt-6-luna\"\n") {
		t.Error("the Codex executor lacks the balanced model")
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
	ok(t, runHarness(t, base, extra, "install", "--uninstall", "--client", "codex", "--yes"), "Codex uninstall")
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
	base := sandbox(t)
	path := filepath.Join(base, "home", ".claude", "agents", "reviewer.md")
	writeMode(t, path, "private agent\n", 0o644)
	ok(t, runInstaller(t, base, "--skip-packages", "--no-backup"), "no-backup install")
	if got := readFile(t, path); got == "private agent\n" {
		t.Error("no-backup left the colliding file in place")
	}
	if backups, _ := filepath.Glob(filepath.Join(base, "home", ".dotfiles-backups", "harness", "*")); len(backups) != 0 {
		t.Errorf("no-backup archived the collision: %v", backups)
	}
}

// TestInstallInstallsTheEmbeddedHarnessFromAnywhere runs outside any checkout with no asset override, so what lands is what the binary carries.
func TestInstallInstallsTheEmbeddedHarnessFromAnywhere(t *testing.T) {
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
	if got, want := readFileOr(filepath.Join(home, ".claude", "CLAUDE.md")), readFile(t, filepath.Join("..", "..", "POLICY.md")); got != want {
		t.Error("install did not install the embedded policy at ~/.claude/CLAUDE.md")
	}
}

// --clear-backups reads only $HOME, so it runs from outside any checkout.
func TestClearBackupsRunsOutsideTheCheckout(t *testing.T) {
	base := sandbox(t)
	seedBackups(t, base)
	home := filepath.Join(base, "home")
	cmd := exec.Command(binary, "install", "--clear-backups")
	cmd.Dir, cmd.Env = home, []string{"HOME=" + home, "PATH=/usr/bin:/bin"}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("clear-backups outside the checkout: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(home, ".dotfiles-backups", "harness")); err == nil {
		t.Error("the harness backups survived")
	}
}

// TestInstallReplacesAnUnusableStatuslineBinary keeps the tracked statusLine honest: a missing binary, one without the exec bit, or one predating the subcommand is replaced by the running harness rather than left for Claude Code to draw an empty status line with.
func TestInstallReplacesAnUnusableStatuslineBinary(t *testing.T) {
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
			base := sandbox(t)
			c.seed(t, base)
			ok(t, runInstaller(t, base, "--skip-packages"), "install")
			if err := exec.Command(installed(base), "statusline", "--help").Run(); err != nil {
				t.Fatalf("the installed harness has no working statusline: %v", err)
			}
		})
	}
}

// A non-object mcp in opencode.json stops the run before either client is registered, and leaves the file as it was.
func TestAgentBrowserRefusesANonObjectOpenCodeMCP(t *testing.T) {
	base := sandbox(t)
	config := filepath.Join(base, "home", ".config", "opencode", "opencode.json")
	const text = `{"mcp": ["x"]}` + "\n"
	writeMode(t, config, text, 0o644)
	r := runInstaller(t, base, "--skip-packages")
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

// With Codex gone, its uninstall still removes Engram's files beside config.toml and succeeds, leaving the config entries with a warning.
func TestCodexUninstallWithoutCodex(t *testing.T) {
	base := sandbox(t)
	ok(t, runInstaller(t, base, "--skip-packages"), "install")
	codexHome := filepath.Join(base, "home", ".codex")
	for _, name := range []string{"engram-instructions.md", "engram-compact-prompt.md"} {
		writeMode(t, filepath.Join(codexHome, name), "engram\n", 0o644)
	}
	if err := os.Remove(filepath.Join(base, "shims", "codex")); err != nil {
		t.Fatal(err)
	}
	r := ok(t, runInstaller(t, base, "--uninstall", "--client", "codex", "--yes"), "Codex uninstall without Codex")
	if !strings.Contains(r.stderr, "Codex is not installed") {
		t.Errorf("no warning about the entries left behind:\n%s", r.stderr)
	}
	if _, err := os.Stat(filepath.Join(codexHome, "engram-instructions.md")); err == nil {
		t.Error("Engram's instructions file survived")
	}
}

// A registration that landed outside the user scope fails the run, so the scope check cannot pass on a server Claude Code would not load for every project. Only a full run reaches checkTools.
func TestInstallRefusesANonUserScopeEngramMCP(t *testing.T) {
	base := sandbox(t)
	seedLived(t, base)
	writeShim(t, base, "claude", `case "$*" in
"plugin list --json") echo '[{"id": "engram@engram"}]' ;;
"mcp get agent-browser") echo 'Scope: User config' ;;
"mcp get engram") echo 'Scope: Project config' ;;
esac`)
	r := runInstaller(t, base)
	if r.code == 0 || !strings.Contains(r.stderr, "engram is not registered in the Claude Code user scope") {
		t.Fatalf("exit %d, stderr %q; want the engram scope refusal", r.code, r.stderr)
	}
}

// A surviving engram entry fails the uninstall's own sweep, so the removal at removeClaudeMCP cannot pass vacuously.
func TestUninstallRefusesASurvivingEngramMCP(t *testing.T) {
	base := sandbox(t)
	seedLived(t, base)
	ok(t, runInstaller(t, base, "--skip-packages"), "install")
	writeMode(t, filepath.Join(base, "home", ".claude.json"), `{"mcpServers": {"engram": {}}}`+"\n", 0o644)
	r := runInstaller(t, base, "--uninstall", "--yes")
	if r.code == 0 || !strings.Contains(r.stderr, "The engram MCP survived the uninstall") {
		t.Fatalf("exit %d, stderr %q; want the engram survivor refusal", r.code, r.stderr)
	}
}

// TestFullInstallProvisionsMissingOnly locks the provisioning contract: one inventory per full run, no upgrade command, an install only for a formula the inventory lacks, and MCP reconciliation that still runs though the executables already exist.
func TestFullInstallProvisionsMissingOnly(t *testing.T) {
	base := sandbox(t)
	seedFull(t, base)
	ok(t, runInstaller(t, base), "full install")
	log := readFile(t, filepath.Join(base, "shims.log"))
	if got := strings.Count(log, "brew list --formula"); got != 1 {
		t.Errorf("full install inventoried formulae %d times, want 1:\n%s", got, log)
	}
	if strings.Contains(log, "brew upgrade") {
		t.Errorf("full install issued an upgrade:\n%s", log)
	}
	for _, absent := range []string{"brew install worktrunk", "wt ", "brew install node", "brew install gh", "brew install anomalyco/tap/opencode"} {
		if strings.Contains(log, absent) {
			t.Errorf("full install unexpectedly invoked %q:\n%s", absent, log)
		}
	}
	for _, want := range []string{"brew install uv", "brew install agent-browser", "brew install gentleman-programming/tap/engram"} {
		if !strings.Contains(log, want) {
			t.Errorf("full install skipped the missing %q:\n%s", want, log)
		}
	}
	if strings.Contains(log, "brew install --cask codex") {
		t.Errorf("full install reinstalled Codex, which is on PATH:\n%s", log)
	}
	for _, want := range []string{"engram setup claude-code", "engram setup codex", "engram setup opencode", "claude mcp add --scope user agent-browser", "claude mcp add --scope user engram", "codex mcp add agent-browser"} {
		if !strings.Contains(log, want) {
			t.Errorf("full install skipped reconciliation though the tools exist, missing %q", want)
		}
	}
}

// TestSkipPackagesCallsNoPackageManager locks that the option named for skipping packages makes no package-manager call while still reconciling the links and the MCP registrations.
func TestSkipPackagesCallsNoPackageManager(t *testing.T) {
	base := sandbox(t)
	seedLived(t, base)
	ok(t, runInstaller(t, base, "--skip-packages"), "skip-packages install")
	log := readFile(t, filepath.Join(base, "shims.log"))
	if strings.Contains(log, "brew ") {
		t.Errorf("--skip-packages called the package manager:\n%s", log)
	}
	for _, want := range []string{"engram setup claude-code", "claude mcp add --scope user agent-browser", "claude mcp add --scope user engram"} {
		if !strings.Contains(log, want) {
			t.Errorf("--skip-packages skipped reconciliation, missing %q", want)
		}
	}
	link := filepath.Join(base, "home", ".claude", "CLAUDE.md")
	if got, want := readFileOr(link), readFile(t, filepath.Join(base, "checkout", "POLICY.md")); got != want {
		t.Errorf("--skip-packages did not install the policy at %s", link)
	}
}

// Every other sandbox has the claude shim; this run stands in for Anthropic's installer and proves it is reached exactly once.
func TestInstallAddsClaudeOnlyWhenAbsent(t *testing.T) {
	base := sandbox(t)
	seedFull(t, base)
	shims, log := filepath.Join(base, "shims"), filepath.Join(base, "shims.log")
	claude := filepath.Join(shims, "claude")
	saved := filepath.Join(base, "claude.saved")
	writeMode(t, saved, readFile(t, claude), 0o755)
	if err := os.Remove(claude); err != nil {
		t.Fatal(err)
	}
	// bash stands in for the native installer: it records the call and writes the claude binary the tool check and the agent-browser registration then look for.
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

// TestInstallRefusesMissingOrRelativeLocations proves the installer validates HOME and the config location before any write, so a relative one can never turn managed paths into writes inside the invocation directory.
func TestInstallRefusesMissingOrRelativeLocations(t *testing.T) {
	for _, c := range []struct {
		name string
		env  []string
	}{
		{"missing-home", []string{"HOME="}},
		{"relative-home", []string{"HOME=relative"}},
		{"relative-config", []string{"XDG_CONFIG_HOME=relative"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			base := sandbox(t)
			home := filepath.Join(base, "home")
			before := homeTree(t, home)
			r := runHarness(t, base, c.env, "install", "--skip-packages")
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

// homeTree renders the home tree so a test can assert a refused run left it unchanged.
func homeTree(t *testing.T, home string) string {
	t.Helper()
	var b strings.Builder
	tree(t, &b, home, true)
	return b.String()
}

// A second install over unchanged copies rewrites and archives nothing.
func TestReinstallRewritesNothingUnchanged(t *testing.T) {
	base := sandbox(t)
	ok(t, runInstaller(t, base, "--skip-packages"), "install")
	r := runInstaller(t, base, "--skip-packages")
	ok(t, r, "reinstall")
	for _, noise := range []string{"Installed ", "Backed up ", "Removed "} {
		if strings.Contains(r.stdout, noise) {
			t.Errorf("the reinstall logged %q:\n%s", noise, r.stdout)
		}
	}
}

// A TMPDIR reached through a symlink, as macOS /var is, still extracts assets the role agent renderer accepts.
func TestInstallWithSymlinkedTmpdir(t *testing.T) {
	base := sandbox(t)
	link := filepath.Join(base, "tmp-link")
	symlink(t, filepath.Join(base, "tmp"), link)
	r := runHarnessInput(t, base, "", []string{"TMPDIR=" + link, "HARNESS_ASSETS=" + filepath.Join(base, "checkout")}, "install", "--skip-packages")
	ok(t, r, "install")
}

func TestRetiredAssetsPreserveEditsAndPrivateWorktrees(t *testing.T) {
	base := sandbox(t)
	home := filepath.Join(base, "home")
	source := filepath.Join(base, "checkout", "skills", "ops", "retired")
	writeMode(t, filepath.Join(source, "SKILL.md"), "---\ndescription: retired\n---\n", 0o644)
	linkedSource := filepath.Join(base, "checkout", "skills", "ops", "retired-linked")
	writeMode(t, filepath.Join(linkedSource, "SKILL.md"), "---\ndescription: retired too\n---\n", 0o644)
	ok(t, runInstaller(t, base, "--skip-packages"), "install asset")
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
	ok(t, runInstaller(t, base, "--skip-packages"), "retire asset")
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

// The compaction hooks reach every client, and Claude Code's merge replaces only the harness's own hook group: the user's hooks under the same event and under other events survive both the install and the uninstall, and a stale harness group is replaced rather than duplicated.
func TestCompactionHooksKeepTheUsersOwn(t *testing.T) {
	base := sandbox(t)
	home := filepath.Join(base, "home")
	settings := filepath.Join(home, ".claude", "settings.json")
	user := `{"matcher": "startup", "hooks": [{"type": "command", "command": "echo mine"}]}`
	writeMode(t, settings, `{"hooks": {"SessionStart": [`+user+`, {"matcher": "compact", "hooks": [{"type": "command", "command": "\"$HOME/.local/bin/harness\" retired"}]}], "Stop": [`+user+`]}}`+"\n", 0o644)
	ok(t, runInstaller(t, base, "--skip-packages"), "install")

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

	ok(t, runInstaller(t, base, "--uninstall", "--yes"), "uninstall")
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

// A user's own ~/.codex/hooks.json keeps its hooks: the install adds the harness's compaction group beside them, and the uninstall removes only that group.
func TestCodexHooksKeepTheUsersOwn(t *testing.T) {
	base := sandbox(t)
	path := filepath.Join(base, "home", ".codex", "hooks.json")
	writeMode(t, path, `{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "echo mine"}]}]}}`+"\n", 0o644)
	ok(t, runInstaller(t, base, "--skip-packages"), "install")
	text := readFileOr(path)
	if !strings.Contains(text, "echo mine") || !strings.Contains(text, `harness\" resume`) {
		t.Fatalf("after install, hooks.json lacks the user's hook or the harness hook:\n%s", text)
	}
	ok(t, runInstaller(t, base, "--uninstall", "--yes"), "uninstall")
	text = readFileOr(path)
	if !strings.Contains(text, "echo mine") || strings.Contains(text, "resume") {
		t.Errorf("after uninstall, hooks.json should hold only the user's hook:\n%s", text)
	}
}

// The tracked allow rules join the user's own instead of replacing them: the install keeps every live rule and adds the harness's, and the uninstall removes only the harness's.
func TestClaudeAllowKeepsTheUsersRules(t *testing.T) {
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
	ok(t, runInstaller(t, base, "--skip-packages"), "install")
	if got, want := allow(), []string{"Bash(make test)", "Read(~/.claude/skills/**)"}; !slices.Equal(got, want) {
		t.Errorf("allow after install = %q, want %q", got, want)
	}
	ok(t, runInstaller(t, base, "--skip-packages"), "reinstall")
	if got := allow(); len(got) != 2 {
		t.Errorf("a reinstall duplicated the tracked rule: %q", got)
	}
	ok(t, runInstaller(t, base, "--uninstall", "--yes"), "uninstall")
	if got, want := allow(), []string{"Bash(make test)"}; !slices.Equal(got, want) {
		t.Errorf("allow after uninstall = %q, want %q", got, want)
	}
}
