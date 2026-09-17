package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"
)

var update = flag.Bool("update", false, "rewrite testdata/install goldens from the installer under test")

// installScenarios are the runs the goldens record. Each starts from a fresh sandbox, and the golden holds every run's exit, the external commands in order, and the resulting HOME.
var installScenarios = []struct {
	name       string
	darwinOnly bool
	seed       func(*testing.T, string)
	runs       [][]string
	stdin      string
}{
	{name: "skip-packages", seed: seedLived, runs: [][]string{{"--skip-packages"}}},
	{name: "skip-packages-no-backup", seed: seedLived, runs: [][]string{{"--skip-packages", "--no-backup"}}},
	// Linux adds --with-deps to the browser download, so this golden is macOS's.
	{name: "full", darwinOnly: true, seed: seedFull, runs: [][]string{nil}},
	{name: "uninstall", seed: seedLived, runs: [][]string{{"--skip-packages"}, {"--uninstall", "--yes"}}},
	// OpenCode alone: its copies and registrations go, and ~/.agents/skills stays because Claude Code still reads guarded bodies there.
	{name: "uninstall-partial", seed: seedLived, runs: [][]string{{"--skip-packages"}, {"--uninstall", "--client", "opencode", "--yes"}}},
	// Declining the confirmation removes nothing.
	{name: "uninstall-declined", seed: seedLived, runs: [][]string{{"--skip-packages"}, {"--uninstall"}}, stdin: "n\n"},
	{name: "clear-backups", seed: seedBackups, runs: [][]string{{"--clear-backups"}}},
	{name: "rejected-flags", runs: [][]string{{"--uninstall", "--skip-packages"}, {"--clear-backups", "--uninstall"}, {"--skip-packages", "--clear-backups"}, {"--bogus"}}},
}

func TestInstallerGolden(t *testing.T) {
	for _, sc := range installScenarios {
		t.Run(sc.name, func(t *testing.T) {
			if sc.darwinOnly && runtime.GOOS != "darwin" {
				t.Skip("recorded on macOS")
			}
			base := sandbox(t)
			if sc.seed != nil {
				sc.seed(t, base)
			}
			var b strings.Builder
			for _, args := range sc.runs {
				r := runInstallerInput(t, base, sc.stdin, args...)
				fmt.Fprintf(&b, "run [%s]: exit %d\n", strings.Join(args, " "), r.code)
				if r.code != 0 {
					b.WriteString(ansi.ReplaceAllString(r.stderr, ""))
				}
			}
			log, err := os.ReadFile(filepath.Join(base, "shims.log"))
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				t.Fatal(err)
			}
			b.WriteString("== commands\n")
			b.Write(log)
			b.WriteString("== home\n")
			tree(t, &b, filepath.Join(base, "home"), true)
			golden(t, sc.name, normalize(base, b.String()))
		})
	}
}

// runInstaller runs `harness install` from the sandbox checkout.
func runInstaller(t *testing.T, base string, args ...string) result {
	t.Helper()
	return runInstallerInput(t, base, "", args...)
}

// runInstallerInput runs `harness install` with stdin, and TMPDIR inside the sandbox so the uninstall's cache removal never reaches the real one.
func runInstallerInput(t *testing.T, base, stdin string, args ...string) result {
	t.Helper()
	return runHarnessInput(t, base, stdin, []string{"TMPDIR=" + filepath.Join(base, "tmp"), "HARNESS_ASSETS=" + filepath.Join(base, "checkout")}, append([]string{"install"}, args...)...)
}

// runHarness runs the built binary from the sandbox checkout, with only the shims and the system directories on PATH, plus any extra environment.
func runHarness(t *testing.T, base string, extra []string, args ...string) result {
	t.Helper()
	return runHarnessInput(t, base, "", extra, args...)
}

func runHarnessInput(t *testing.T, base, stdin string, extra []string, args ...string) result {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Dir = filepath.Join(base, "checkout")
	cmd.Env = append([]string{"HOME=" + filepath.Join(base, "home"), "PATH=" + filepath.Join(base, "shims") + ":/usr/bin:/bin", "SHELL=/bin/zsh"}, extra...)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return result{stdout.String(), stderr.String(), code}
}

// sandbox lays out a resolved temporary base holding a copy of this checkout's harness, an empty HOME with the status line binary a prior install would have left, and the recording shims. The base is resolved because the installer compares resolved link targets against its own directory, and macOS temp paths pass through a symlink.
func sandbox(t *testing.T) string {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("..", "..")
	out, err := exec.Command("git", "-C", root, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", "POLICY.md", "aliases.sh", "bench", "clients", "commands", "roles", "scenarios", "skills", "cmd", "internal", "go.mod", "go.sum").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00") {
		info, err := os.Stat(filepath.Join(root, rel))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		writeMode(t, filepath.Join(base, "checkout", rel), string(data), info.Mode().Perm())
	}
	for _, dir := range []string{"home", "tmp"} {
		if err := os.MkdirAll(filepath.Join(base, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// The status line binary a prior `harness install` left in place, which every install mode now probes.
	writeMode(t, filepath.Join(base, "home", ".local/bin/harness"), "#!/bin/sh\n", 0o755)
	shims, log := filepath.Join(base, "shims"), filepath.Join(base, "shims.log")
	for name, body := range shimScripts(shims) {
		script := "#!/bin/sh\ninput=$(cat)\nprintf '%s\\n' \"" + name + " $* <$input>\" >> '" + log + "'\n" + body + "\nexit 0\n"
		writeMode(t, filepath.Join(shims, name), script, 0o755)
	}
	return base
}

// shimScripts fake every external tool the installer calls. Each records its arguments and stdin, and prints only what the installer parses.
func shimScripts(shims string) map[string]string {
	return map[string]string{
		"agent-browser": "",
		// The formulae `brew list --formula` reports as installed: a full run adds only what this leaves out, so the golden shows missing-only provisioning without a per-formula probe.
		"brew":   `if [ "$1" = "list" ]; then printf '%s\n' node gh opencode; fi`,
		"curl":   "",
		"engram": "",
		"gh":     "",
		"npx":    "",
		"claude": `case "$*" in
"plugin list --json") echo '[{"id": "engram@engram"}]' ;;
"mcp get agent-browser") echo 'Scope: User config' ;;
esac`,
		"opencode": `if [ "$*" = "debug config" ]; then
echo '{"plugin": ["engram"], "mcp": {"engram": {}, "agent-browser": {"type": "local", "enabled": true, "command": ["` + filepath.Join(shims, "agent-browser") + `", "mcp"]}}}'
fi`,
		"wt": `case "$*" in
"switch --help") echo '--no-cd' ;;
"list --help") echo '--config-set' ;;
esac`,
	}
}

// seedLived is a machine with history: private entries beside managed ones, collisions, stale and foreign links, hand-written aliases, and the client config Engram writes.
func seedLived(t *testing.T, base string) {
	home, checkoutRoot := filepath.Join(base, "home"), filepath.Join(base, "checkout")
	for rel, text := range map[string]string{
		".claude/settings.json": `{
  "env": {
    "KEEP": "café"
  },
  "model": "sonnet",
  "permissions": {
    "allow": [
      "mcp__plugin_engram_engram__mem_save",
      "Bash(cd ~/src/engram && cargo test)"
    ],
    "deny": [
      "Read(stale)"
    ],
    "defaultMode": "plan"
  },
  "enabledPlugins": {
    "engram@engram": true,
    "other@market": true
  },
  "extraKnownMarketplaces": {
    "engram": {},
    "market": {}
  }
}
`,
		".claude.json":                              `{"mcpServers": {"other": {}}}` + "\n",
		".claude/mcp/engram.json":                   "{}\n",
		".claude/CLAUDE.md":                         "a hand-written policy\n",
		".claude/agents/mine.md":                    "private agent\n",
		".claude/skills/mine/SKILL.md":              "private skill\n",
		".agents/skills/grilling/SKILL.md":          "shadowing copy\n",
		".config/opencode/skills/grilling/SKILL.md": "duplicate copy\n",
		".config/opencode/opencode.json":            `{"$schema": "https://opencode.ai/config.json", "agent": {"mine": {"model": "openai/gpt-5.5"}}, "mcp": {"engram": {"type": "local"}, "other": {"type": "remote"}}, "plugin": ["/x/plugins/engram.ts", "other-plugin"]}` + "\n",
		".config/opencode/tui.json":                 `{"plugin": ["opencode-subagent-statusline", "keep"]}` + "\n",
		".config/opencode/plugins/engram.ts":        "export {}\n",
		".local/bin/claude-seo":                     "#!/bin/sh\n",
		".engram/engram.db":                         "memories\n",
		".agent-browser/browsers/chrome":            "browser\n",
		".local/share/opencode/auth.json":           `{"openai": "private-secret"}` + "\n",
		".zshrc":                                    "export KEEP=1\nalias ccyolo='claude'\nalias ll='ls -l'\n\n\n",
		".dotfiles-backups/other/keep.txt":          "not the harness's\n",
	} {
		writeMode(t, filepath.Join(home, rel), text, 0o644)
	}
	elsewhere := filepath.Join(base, "elsewhere")
	if err := os.MkdirAll(elsewhere, 0o755); err != nil {
		t.Fatal(err)
	}
	for rel, target := range map[string]string{
		".claude/commands/retired.md": checkoutRoot + "/commands/retired.md",
		".claude/commands/renamed.md": checkoutRoot + "/commands/ship.md",
		// A link an install from before the copies left at a managed path: archived, then replaced by a copy.
		".claude/commands/ship.md":    checkoutRoot + "/commands/ship.md",
		".claude/commands/foreign.md": "/nonexistent/foreign.md",
		".claude/agents/retired.md":   checkoutRoot + "/.generated/agents/claude/retired.md",
		".config/opencode/command":    elsewhere,
	} {
		path := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
	}
}

// seedFull holds only what a real `engram setup` would have written, which the full run's tool check reads.
func seedFull(t *testing.T, base string) {
	writeMode(t, filepath.Join(base, "home", ".claude/mcp/engram.json"), "{}\n", 0o644)
}

func seedBackups(t *testing.T, base string) {
	home := filepath.Join(base, "home")
	writeMode(t, filepath.Join(home, ".dotfiles-backups/harness/old-a/.zshrc"), "a\n", 0o644)
	writeMode(t, filepath.Join(home, ".dotfiles-backups/harness/old-b/.claude/CLAUDE.md"), "b\n", 0o644)
	writeMode(t, filepath.Join(home, ".dotfiles-backups/other/keep.txt"), "kept\n", 0o644)
}

func writeMode(t *testing.T, path, text string, mode fs.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), mode); err != nil {
		t.Fatal(err)
	}
}

// tree writes one line per entry under root: links with their text, directories, and files with their content when contents is set.
func tree(t *testing.T, b *strings.Builder, root string, contents bool) {
	t.Helper()
	copies := map[string]bool{}
	if data, err := os.ReadFile(filepath.Join(root, ".harness", "state.json")); err == nil {
		var s struct{ Files []struct{ Path string } }
		if err := json.Unmarshal(data, &s); err != nil {
			t.Fatal(err)
		}
		for _, f := range s.Files {
			copies[f.Path] = true
		}
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == root && errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		rel, _ := filepath.Rel(root, path)
		switch {
		case rel == ".":
		case copies[path]:
			// A managed copy of a checkout asset: the state file records its digest, so the golden names it rather than repeating the asset.
			fmt.Fprintf(b, "C %s\n", rel)
			if d.IsDir() {
				return filepath.SkipDir
			}
		case rel == "Library" && d.IsDir():
			// macOS's system Python writes its bytecode cache here, stamped differently on every run.
			return filepath.SkipDir
		case d.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			fmt.Fprintf(b, "L %s -> %s\n", rel, target)
		case d.IsDir():
			fmt.Fprintf(b, "D %s\n", rel)
		case !contents:
			fmt.Fprintf(b, "F %s\n", rel)
		default:
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			fmt.Fprintf(b, "F %s\n", rel)
			if !utf8.Valid(data) {
				// The harness binary an install copies into place.
				b.WriteString("  |<binary>\n")
				return nil
			}
			for _, line := range strings.Split(string(data), "\n") {
				fmt.Fprintf(b, "  |%s\n", line)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

var (
	ansi  = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	stamp = regexp.MustCompile(`\d{8}-\d{6}(-\d+)?`)
)

// normalize replaces the sandbox paths and the backup timestamps, which differ on every run.
var (
	digestText = regexp.MustCompile(`sha256:[0-9a-f]{64}`)
	assetsDir  = regexp.MustCompile(`harness-assets-[0-9]+`)
)

func normalize(base, s string) string {
	s = strings.NewReplacer(base+"/checkout", "$CHECKOUT", base+"/home", "$HOME", base+"/shims", "$SHIMS", base, "$BASE").Replace(s)
	s = digestText.ReplaceAllString(s, "sha256:<digest>")
	s = assetsDir.ReplaceAllString(s, "harness-assets-<n>")
	return stamp.ReplaceAllString(s, "<stamp>")
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "install", name+".golden")
	if *update {
		writeMode(t, path, got, 0o644)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got == string(want) {
		return
	}
	gotPath := filepath.Join(t.TempDir(), name)
	writeMode(t, gotPath, got, 0o644)
	diff, _ := exec.Command("diff", "-u", path, gotPath).CombinedOutput()
	t.Errorf("installer output differs from %s; rerun with -update only when the change is intended:\n%s", path, diff)
}
