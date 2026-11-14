package setup

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// repo is an empty repository, optionally with a GitHub origin whose HEAD is main.
func repo(t *testing.T, github bool) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
	dir := t.TempDir()
	run(t, dir, "init", "-q", "-b", "main")
	if github {
		run(t, dir, "remote", "add", "origin", "https://github.com/o/r.git")
		run(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	}
	return dir
}

// fake is a forge with labels; it logs every call and fails auth when noAuth is set.
type fake struct {
	calls     []string
	labels    []string
	noAuth    bool
	protected bool
}

func (f *fake) run(args ...string) ([]byte, error) {
	call := strings.Join(args, " ")
	f.calls = append(f.calls, call)
	switch {
	case call == "auth status":
		if f.noAuth {
			return nil, errors.New("not logged in")
		}
		return nil, nil
	case strings.HasPrefix(call, "label list"):
		var list []map[string]string
		for _, l := range f.labels {
			list = append(list, map[string]string{"name": l})
		}
		return json.Marshal(list)
	case strings.HasPrefix(call, "label create "):
		f.labels = append(f.labels, args[2])
		return nil, nil
	case call == "api repos/{owner}/{repo}/branches/main":
		return json.Marshal(map[string]bool{"protected": f.protected})
	}
	return nil, errors.New("unexpected gh " + call)
}

func (f *fake) writes() []string {
	var w []string
	for _, c := range f.calls {
		if strings.HasPrefix(c, "label create") {
			w = append(w, c)
		}
	}
	return w
}

func read(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// snapshot reads every file under dir, so a run that writes anything is caught.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && !strings.Contains(path, string(filepath.Separator)+".git"+string(filepath.Separator)) {
			files[path] = read(t, path)
		}
		return nil
	})
	return files
}

func TestProbeReportsEvidenceAndWritesNothing(t *testing.T) {
	local := repo(t, false)
	if err := os.MkdirAll(filepath.Join(local, "docs", "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	f := &fake{}
	before := snapshot(t, local)
	e := Probe(local, f.run)
	if e.GitHubRemote || e.Protected != nil || len(f.calls) != 0 || strings.Join(e.LocalSpecTrees, ",") != filepath.Join("docs", "specs") {
		t.Errorf("local probe = %+v, calls %v", e, f.calls)
	}

	hosted := repo(t, true)
	if err := os.MkdirAll(filepath.Join(hosted, "docs", "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hosted, ConfigPath), []byte(`{"tracker":"github","spec_home":"github-issues"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	f = &fake{protected: true}
	e = Probe(hosted, f.run)
	if !e.GitHubRemote || e.RemoteURL != "https://github.com/o/r.git" || e.DefaultBranch != "main" || e.Protected == nil || !*e.Protected || e.ConfiguredMode != "github" || len(e.LocalSpecTrees) != 0 {
		t.Errorf("hosted probe = %+v", e)
	}
	if after := snapshot(t, local); len(after) != len(before) {
		t.Error("probe wrote a file")
	}
}

func TestTrackerGitHubWritesTheStableShapeLabelsAndAutomation(t *testing.T) {
	dir := repo(t, true)
	if err := os.MkdirAll(filepath.Join(dir, ".github", "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{supersededPath, filepath.Join(".github", "workflows", "ci.yml")} {
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, p), []byte("old\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f := &fake{}
	done, err := Tracker(dir, Options{Mode: "github"}, f.run)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "tracker": "github",
  "spec_home": "github-issues",
  "labels": {
    "blocked": "blocked",
    "ready_for_agent": "ready-for-agent",
    "in_progress": "in-progress",
    "ready_for_human": "ready-for-human"
  }
}
`
	if got := read(t, filepath.Join(dir, ConfigPath)); got != want {
		t.Errorf("config:\n%s", got)
	}
	if len(f.writes()) != 4 {
		t.Errorf("label writes = %v", f.writes())
	}
	if f.calls[0] != "auth status" {
		t.Errorf("credentials were not checked first: %v", f.calls)
	}
	for path, harnessCopy := range map[string]string{WorkflowPath: workflowAsset, ReconcilerPath: reconcilerAsset} {
		if read(t, filepath.Join(dir, path)) != read(t, filepath.Join("..", "..", harnessCopy)) {
			t.Errorf("%s differs from the harness copy", path)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, supersededPath)); !os.IsNotExist(err) {
		t.Error("the superseded script survived")
	}
	if read(t, filepath.Join(dir, ".github", "workflows", "ci.yml")) != "old\n" {
		t.Error("an unrelated workflow changed")
	}
	if !strings.Contains(strings.Join(done, "\n"), "created "+ConfigPath) || !strings.Contains(strings.Join(done, "\n"), "removed "+supersededPath) {
		t.Errorf("report = %v", done)
	}

	before := snapshot(t, dir)
	f.calls = nil
	again, err := Tracker(dir, Options{Mode: "github"}, f.run)
	if err != nil || len(again) != 0 || len(f.writes()) != 0 {
		t.Errorf("second run reported %v, label writes %v, err %v", again, f.writes(), err)
	}
	for path, body := range snapshot(t, dir) {
		if before[path] != body {
			t.Errorf("second run changed %s", path)
		}
	}
}

func TestTrackerKeepsCustomLabelsCriticalPathsAndUnknownKeys(t *testing.T) {
	dir := repo(t, false)
	if err := os.MkdirAll(filepath.Join(dir, "docs", "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ConfigPath), []byte(`{"tracker":"github","labels":{"blocked":"on-hold","ready_for_agent":"agent-go"},"criticalPaths":["db/**"],"team":{"owner":"x"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &fake{}
	if _, err := Tracker(dir, Options{Mode: "local-markdown"}, f.run); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Tracker       string            `json:"tracker"`
		SpecHome      string            `json:"spec_home"`
		Labels        map[string]string `json:"labels"`
		CriticalPaths []string          `json:"criticalPaths"`
		Team          map[string]string `json:"team"`
	}
	if err := json.Unmarshal([]byte(read(t, filepath.Join(dir, ConfigPath))), &got); err != nil {
		t.Fatal(err)
	}
	if got.Tracker != "local-markdown" || got.SpecHome != "docs/specs" || got.Labels["blocked"] != "on-hold" || got.Labels["ready_for_agent"] != "agent-go" || got.Labels["in_progress"] != "in-progress" || strings.Join(got.CriticalPaths, ",") != "db/**" || got.Team["owner"] != "x" {
		t.Errorf("config = %+v", got)
	}
	if len(f.calls) != 0 {
		t.Errorf("local-markdown reached the forge: %v", f.calls)
	}
	if _, err := os.Stat(filepath.Join(dir, ".github")); !os.IsNotExist(err) {
		t.Error("local-markdown installed GitHub automation")
	}
}

func TestTrackerRefusesMissingCredentialsBeforeAnyWrite(t *testing.T) {
	dir := repo(t, true)
	f := &fake{noAuth: true}
	if _, err := Tracker(dir, Options{Mode: "github"}, f.run); err == nil {
		t.Fatal("missing credentials were accepted")
	}
	if len(snapshot(t, dir)) != 0 || len(f.calls) != 1 {
		t.Errorf("a refused run wrote files or called %v", f.calls)
	}
}
