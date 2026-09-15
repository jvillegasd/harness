package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// statuslineEnv builds the throwaway home and repository the recorded payloads render against: one effort level in the tracked settings, a repository named `project` on a known branch with one uncommitted file, and shims that keep the macOS keychain out of the run.
func statuslineEnv(t *testing.T) (base, home, work string) {
	t.Helper()
	base = t.TempDir()
	home = filepath.Join(base, "home")
	work = filepath.Join(base, "work")
	writeMode(t, filepath.Join(home, ".claude", "settings.json"), `{"effortLevel": "high"}`+"\n", 0o644)
	project := filepath.Join(work, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, project, "init", "-q", "-b", "statusline-branch")
	git(t, project, "commit", "-q", "--allow-empty", "-m", "init")
	writeMode(t, filepath.Join(project, "dirty.txt"), "uncommitted\n", 0o644)
	shims := filepath.Join(base, "shims")
	writeMode(t, filepath.Join(shims, "security"), "#!/bin/sh\nexit 1\n", 0o755)
	writeMode(t, filepath.Join(shims, "secret-tool"), "#!/bin/sh\nexit 1\n", 0o755)
	if err := os.MkdirAll(filepath.Join(base, "tmp"), 0o755); err != nil {
		t.Fatal(err)
	}
	return base, home, work
}

// TestStatuslineFixtures drives the binary the way Claude Code does: a recorded payload on stdin, the rendered line on stdout, compared byte for byte against the output captured from the shell renderer before it was deleted.
func TestStatuslineFixtures(t *testing.T) {
	base, home, work := statuslineEnv(t)
	fixtures, err := filepath.Glob(filepath.Join("testdata", "statusline", "fixtures", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) == 0 {
		t.Fatal("no recorded status payloads")
	}
	for _, fixture := range fixtures {
		name := strings.TrimSuffix(filepath.Base(fixture), ".json")
		t.Run(name, func(t *testing.T) {
			payload, err := os.ReadFile(fixture)
			if err != nil {
				t.Fatal(err)
			}
			// A recorded cache beside the payload seeds the rate-limit bars; its fresh mtime is what keeps the renderer from fetching.
			cachePath := filepath.Join(base, "tmp", "claude", "statusline-usage-cache.json")
			if cache, err := os.ReadFile(filepath.Join("testdata", "statusline", name+"-cache.json")); err == nil {
				writeMode(t, cachePath, string(cache), 0o644)
			} else if err := os.Remove(cachePath); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			cmd := exec.Command(binary, "statusline")
			cmd.Dir = work
			cmd.Env = []string{
				"HOME=" + home,
				"PATH=" + filepath.Join(base, "shims") + ":/usr/bin:/bin",
				"TMPDIR=" + filepath.Join(base, "tmp"),
				"TZ=UTC",
				"GIT_CONFIG_NOSYSTEM=1",
				"GIT_CONFIG_GLOBAL=" + filepath.Join(base, "gitconfig"),
			}
			cmd.Stdin = bytes.NewReader(payload)
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("%s: %v", fixture, err)
			}
			want, err := os.ReadFile(filepath.Join("testdata", "statusline", name+".golden"))
			if err != nil {
				t.Fatal(err)
			}
			if string(out) != string(want) {
				t.Errorf("%s:\n got %q\nwant %q", fixture, out, want)
			}
		})
	}
}

// TestStatuslineHelpAndBadArguments keeps the subcommand's own surface small and scriptable like the other commands'.
func TestStatuslineHelpAndBadArguments(t *testing.T) {
	if r := ok(t, harness(t, t.TempDir(), "", "statusline", "--help"), "statusline --help"); !strings.Contains(r.stdout, "statusline") {
		t.Errorf("--help printed %q", r.stdout)
	}
	r := harness(t, t.TempDir(), "", "statusline", "--bogus")
	if r.code == 0 || strings.Count(r.stderr, "\n") != 1 {
		t.Errorf("a bad argument exited %d with stderr %q, want non-zero and one line", r.code, r.stderr)
	}
}
