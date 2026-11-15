package review

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type coverRepo struct {
	t    *testing.T
	dir  string
	base string
}

func newCoverRepo(t *testing.T) *coverRepo {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
	r := &coverRepo{t: t, dir: t.TempDir()}
	r.git("init", "-q", "-b", "main")
	var lines []string
	for i := 1; i <= 20; i++ {
		lines = append(lines, "line")
	}
	r.commit(map[string]string{"a.go": strings.Join(lines, "\n") + "\n", "b.go": "b\n"})
	r.base = r.git("rev-parse", "HEAD")
	r.git("switch", "-q", "-c", "feat")
	r.commit(map[string]string{"a.go": "changed\n" + strings.Join(lines[1:], "\n") + "\n"})
	return r
}

func (r *coverRepo) git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = r.dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (r *coverRepo) commit(files map[string]string) string {
	r.t.Helper()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(r.dir, name), []byte(body), 0o644); err != nil {
			r.t.Fatal(err)
		}
	}
	r.git("add", "-A")
	r.git("commit", "-q", "-m", "change")
	return r.git("rev-parse", "HEAD")
}
