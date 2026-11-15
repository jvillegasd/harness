package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupCommand(t *testing.T) {
	dir, _, run := prRepo(t)
	git(t, dir, "remote", "set-url", "origin", "https://github.com/o/r.git")
	git(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	log := filepath.Join(t.TempDir(), "gh.log")
	forge := `"auth status") ;;
"label list") echo '[]';;
"label create") echo "$*" >> '` + log + `';;
"api repos/{owner}/{repo}/branches/main") echo '{"protected":true}';;
`
	r := run(forge, "setup", "probe")
	var e struct {
		GitHubRemote  bool   `json:"github_remote"`
		DefaultBranch string `json:"default_branch"`
		Protected     *bool  `json:"protected"`
	}
	if r.code != 0 || json.Unmarshal([]byte(r.stdout), &e) != nil || !e.GitHubRemote || e.DefaultBranch != "main" || e.Protected == nil || !*e.Protected {
		t.Fatalf("probe exited %d: %s %s", r.code, r.stdout, r.stderr)
	}
	r = run(forge, "setup", "tracker", "--mode", "github")
	if r.code != 0 || !strings.Contains(r.stdout, "created docs/agents/harness.json") || !strings.Contains(r.stdout, "created .github/workflows/agent-lifecycle.yml") {
		t.Fatalf("tracker exited %d: %s %s", r.code, r.stdout, r.stderr)
	}
	if raw, _ := os.ReadFile(log); strings.Count(string(raw), "label create") != 4 {
		t.Errorf("label creates: %q", raw)
	}
	if again := run(strings.Replace(forge, `echo '[]'`, `echo '[{"name":"blocked"},{"name":"ready-for-agent"},{"name":"in-progress"},{"name":"ready-for-human"}]'`, 1), "setup", "tracker", "--mode", "github"); again.code != 0 || again.stdout != "" {
		t.Errorf("second run exited %d and printed %q: %s", again.code, again.stdout, again.stderr)
	}
}
