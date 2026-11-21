package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDraft(t *testing.T, dir, feature, name, title, blocked string) {
	t.Helper()
	path := filepath.Join(dir, "docs", "agents", "tickets", feature, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "# " + title + "\n\n## What to build\n\nA thing.\n\n## Acceptance criteria\n\n- It works.\n\n## Seam\n\nThe package tests.\n\n## Blocked by\n\n" + blocked + "\n\n## Out of scope\n\nNothing.\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTicketsCheckCommand(t *testing.T) {
	dir := repo(t)
	ok(t, harness(t, dir, "tickets", "check"), "check with no drafts")
	writeDraft(t, dir, "good", "01-a.md", "01 A", "none")
	writeDraft(t, dir, "bad", "01-a.md", "01 A", "02")
	if r := ok(t, harness(t, dir, "tickets", "check", "good"), "check good"); r.stdout != "" {
		t.Errorf("clean feature printed %q", r.stdout)
	}
	r := harness(t, filepath.Join(dir, "docs"), "tickets", "check")
	want := filepath.Join("docs", "agents", "tickets", "bad", "01-a.md") + ":17: blocked by 02, which is not a ticket of this feature\n"
	if r.code != 1 || r.stdout != want {
		t.Errorf("check every feature exited %d with %q, want 1 with %q", r.code, r.stdout, want)
	}
	if !strings.HasPrefix(r.stdout, "docs/") {
		t.Errorf("paths are not repository-relative from a subdirectory: %q", r.stdout)
	}
}

func TestTicketsFrontierCommand(t *testing.T) {
	dir := repo(t)
	writeDraft(t, dir, "demo", "01-a.md", "01 A", "none")
	writeDraft(t, dir, "demo", "02-b.md", "02 B", "none")
	writeDraft(t, dir, "demo", "03-c.md", "03 C", "01, 02")
	path := filepath.Join(dir, "docs", "agents", "tickets", "demo", "02-b.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(raw), "\n## What", "\nStatus: done\n\n## What", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	r := ok(t, harness(t, dir, "tickets", "frontier", "demo", "--done", "1"), "frontier")
	if got := strings.Join(strings.Fields(r.stdout), ""); got != `{"feature":"demo","ready":[3],"done":[1,2],"blocked":[]}` {
		t.Errorf("frontier printed %s", r.stdout)
	}
	if r := harness(t, dir, "tickets", "frontier", "demo", "--done", "9"); r.code != 2 {
		t.Errorf("unknown --done exited %d: %s", r.code, r.stderr)
	}
	writeDraft(t, dir, "demo", "04-d.md", "04 D", "07")
	if r := harness(t, dir, "tickets", "frontier", "demo"); r.code != 1 || !strings.Contains(r.stderr, "04-d.md:17: blocked by 07") || r.stdout != "" {
		t.Errorf("a failing feature exited %d with %q, %q", r.code, r.stdout, r.stderr)
	}
}

func TestTicketsPublishCommand(t *testing.T) {
	dir, _, run := prRepo(t)
	writeDraft(t, dir, "demo", "01-a.md", "01 Alpha", "none")
	config := filepath.Join(dir, "docs", "agents", "harness.json")
	log := filepath.Join(t.TempDir(), "gh.log")
	forge := `"issue create") printf '%s\n' "$*" >> '` + log + `'; echo https://github.com/o/r/issues/9;;
"issue view") echo '{"title":"Alpha","body":"edited"}';;
`
	if err := os.WriteFile(config, []byte(`{"tracker":"jira"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := run(forge, "tickets", "publish", "demo"); r.code != 1 || !strings.Contains(r.stderr, "jira") {
		t.Errorf("another tracker exited %d: %s", r.code, r.stderr)
	}
	if err := os.WriteFile(config, []byte(`{"tracker":"github"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := run(forge, "tickets", "publish", "demo", "--dry-run"); r.code != 0 || r.stdout != "harness tickets publish  dry run\nACTION  TICKET  DETAIL\ncreate  01      Alpha\n" {
		t.Errorf("dry run exited %d: %q %s", r.code, r.stdout, r.stderr)
	}
	if _, err := os.Stat(log); err == nil {
		t.Fatal("a refused or dry run reached gh")
	}
	r := run(forge, "tickets", "publish", "demo")
	if r.code != 1 || r.stdout != "https://github.com/o/r/issues/9\n" || !strings.Contains(r.stderr, "body differs") {
		t.Errorf("publish exited %d: %q %s", r.code, r.stdout, r.stderr)
	}
	if raw, _ := os.ReadFile(log); !strings.Contains(string(raw), "--title Alpha --body ## What to build") {
		t.Errorf("gh create got %q", raw)
	}
	if raw, _ := os.ReadFile(filepath.Join(dir, "docs", "agents", "tickets", "demo", "01-a.md")); !strings.Contains(string(raw), "Issue: https://github.com/o/r/issues/9") {
		t.Errorf("the kept draft does not record its issue:\n%s", raw)
	}
}
