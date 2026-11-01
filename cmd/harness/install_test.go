package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

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
	out, err := exec.Command("git", "-C", root, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", "POLICY.md", "aliases.sh", "bench", "clients", "roles", "skills", "skills-lock.json", "cmd", "internal", "go.mod", "go.sum").Output()
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
	shims := filepath.Join(base, "shims")
	for name, body := range shimScripts(shims) {
		writeShim(t, base, name, body)
	}
	return base
}

// writeShim writes one fake tool with the recording wrapper every shim shares, so a test that overrides a body still logs its invocations.
func writeShim(t *testing.T, base, name, body string) {
	t.Helper()
	log := filepath.Join(base, "shims.log")
	script := "#!/bin/sh\ninput=$(cat)\nprintf '%s\\n' \"" + name + " $* <$input>\" >> '" + log + "'\n" + body + "\nexit 0\n"
	writeMode(t, filepath.Join(base, "shims", name), script, 0o755)
}

// shimScripts fake every external tool the installer calls. Each records its arguments and stdin, and prints only what the installer parses.
func shimScripts(shims string) map[string]string {
	return map[string]string{
		"agent-browser": "",
		// The formulae `brew list --formula` reports as installed let the full run add only what this leaves out, without a per-formula probe.
		"brew":   `if [ "$1" = "list" ]; then printf '%s\n' node gh opencode; fi`,
		"curl":   "",
		"engram": "",
		"gh":     "",
		"npx":    "",
		"claude": `case "$*" in
"plugin list --json") echo '[{"id": "engram@engram"}]' ;;
"mcp get agent-browser"|"mcp get engram") echo 'Scope: User config' ;;
esac`,
		// The catalog lists the preset models; agent-browser's registration lives in a marker file so get answers what add and remove left. Each call logs the CODEX_HOME it ran under.
		"codex": `printf 'CODEX_HOME=%s\n' "$CODEX_HOME" >> '` + filepath.Join(filepath.Dir(shims), "shims.log") + `'
case "$*" in
"debug models") echo '{"models":[{"slug":"gpt-6-luna","visibility":"list","supported_reasoning_levels":[{"effort":"low"},{"effort":"high"}]},{"slug":"gpt-5.6-luna","visibility":"list","supported_reasoning_levels":[{"effort":"low"}]},{"slug":"gpt-6.1-sol","visibility":"list","supported_reasoning_levels":[{"effort":"low"},{"effort":"high"}]}]}' ;;
"mcp add agent-browser"*) touch '` + filepath.Join(shims, ".codex-agent-browser") + `' ;;
"mcp remove agent-browser") rm -f '` + filepath.Join(shims, ".codex-agent-browser") + `' ;;
"mcp get agent-browser") [ -e '` + filepath.Join(shims, ".codex-agent-browser") + `' ] || exit 1 ;;
"mcp get engram") exit 1 ;;
esac`,
		"opencode": `if [ "$*" = "debug config" ]; then
echo '{"plugin": ["engram"], "mcp": {"engram": {}, "agent-browser": {"type": "local", "enabled": true, "command": ["` + filepath.Join(shims, "agent-browser") + `", "mcp"]}}}'
fi`,
		"wt": "",
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
	for rel, target := range map[string]string{
		".claude/skills/retired": checkoutRoot + "/skills/retired",
		".claude/skills/renamed": checkoutRoot + "/skills/open-pr",
		// A link an install from before the copies left at a managed path: archived, then replaced by a copy.
		".claude/skills/open-pr":    checkoutRoot + "/skills/open-pr",
		".claude/skills/foreign":    "/nonexistent/foreign",
		".claude/agents/retired.md": checkoutRoot + "/.generated/agents/claude/retired.md",
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
			// A managed copy of a checkout asset: the state file records its digest, so report the path without repeating the asset.
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
