package main

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// lockRun is a throwaway checkout with the lock tool's paths, an isolated HOME, and a shim directory.
type lockRun struct {
	root, shims, home string
}

func newLockRun(t *testing.T) lockRun {
	t.Helper()
	base := t.TempDir()
	lr := lockRun{
		root:  filepath.Join(base, "checkout"),
		shims: filepath.Join(base, "shims"),
		home:  filepath.Join(base, "home"),
	}
	for _, dir := range []string{"skills", ".claude/skills"} {
		if err := os.MkdirAll(filepath.Join(lr.root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, dir := range []string{lr.shims, lr.home} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeMode(t, filepath.Join(lr.root, "POLICY.md"), "# policy\n", 0o644)
	writeMode(t, filepath.Join(lr.root, "go.mod"), "module github.com/jvillegasd/harness\n", 0o644)
	writeMode(t, filepath.Join(lr.root, "skills-lock.json"), "{\n  \"version\": 1,\n  \"skills\": {}\n}\n", 0o644)
	return lr
}

// setNpx writes the shim at PATH: it records every call, with the directory it ran in, in the checkout's npx.log and, when install is set, performs what skills.sh does for an add in that directory: the body under .agents/skills and the lock beside it.
func (lr lockRun) setNpx(t *testing.T, install bool) {
	t.Helper()
	script := `#!/bin/sh
root="$PWD"
echo "npx $*" >> "` + lr.root + `/npx.log"
echo "$PWD" >> "` + lr.root + `/npx.dirs"
[ "$1" = "skills@1" ] || exit 0
[ "$2" = "add" ] || exit 0
[ "` + boolFlag(install) + `" = "yes" ] || exit 0
shift 2
name=""
while [ $# -gt 0 ]; do
  case "$1" in
    --skill) name="$2"; shift 2 ;;
    *) shift ;;
  esac
done
mkdir -p "$root/.agents/skills/$name"
echo "# $name" > "$root/.agents/skills/$name/SKILL.md"
cat > "$root/skills-lock.json" <<EOF
{
  "version": 1,
  "skills": {
    "$name": {
      "source": "owner/repo",
      "sourceType": "github",
      "skillPath": "skills/$name/SKILL.md",
      "computedHash": "0000000000000000000000000000000000000000000000000000000000000000"
    }
  }
}
EOF
`
	writeMode(t, filepath.Join(lr.shims, "npx"), script, 0o755)
}

func boolFlag(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func (lr lockRun) harness(t *testing.T, args ...string) result {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = lr.root
	cmd.Env = []string{"HOME=" + lr.home, "PATH=" + lr.shims + ":/usr/bin:/bin", "SHELL=/bin/sh"}
	var stdout, stderr bytes.Buffer
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

func (lr lockRun) npxLog(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(lr.root, "npx.log"))
	if err != nil {
		return ""
	}
	return string(data)
}

func (lr lockRun) locked(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(lr.root, "skills-lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	var lf struct {
		Skills map[string]json.RawMessage `json:"skills"`
	}
	if err := json.Unmarshal(data, &lf); err != nil {
		t.Fatal(err)
	}
	return lf.Skills
}

func TestSkillsAddDrivesNpxAndLinks(t *testing.T) {
	lr := newLockRun(t)
	lr.setNpx(t, true)
	r := ok(t, lr.harness(t, "skills", "add", "owner/repo", "myskill", "g"), "skills add")
	if got := lr.npxLog(t); !strings.Contains(got, "npx skills@1 add owner/repo --skill myskill --agent universal -y") {
		t.Fatalf("npx log = %q", got)
	}
	lr.ranOutsideCheckout(t)
	if _, err := os.Stat(filepath.Join(lr.root, "skills/g/myskill/SKILL.md")); err != nil {
		t.Fatalf("skill body not installed in its group: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(lr.root, "skills/myskill")); !os.IsNotExist(err) {
		t.Fatalf("skill body landed outside a group: %v", err)
	}
	if target, err := os.Readlink(filepath.Join(lr.root, ".agents/skills/myskill")); err != nil || target != "../../skills/g/myskill" {
		t.Fatalf("agents link = %q, %v", target, err)
	}
	target, err := os.Readlink(filepath.Join(lr.root, ".claude/skills/myskill"))
	if err != nil || target != "../../.agents/skills/myskill" {
		t.Fatalf("claude link = %q, %v", target, err)
	}
	if _, ok := lr.locked(t)["myskill"]; !ok {
		t.Fatal("myskill missing from the lock")
	}
	if !strings.Contains(r.stdout, "commit only the approved change") {
		t.Fatalf("add did not print the review hint: %q", r.stdout)
	}
}

func TestSkillsAddRefusesBeforeNpx(t *testing.T) {
	cases := []struct {
		name string
		args []string
		seed func(t *testing.T, lr lockRun)
		want string
	}{
		{"source without slash", []string{"skills", "add", "owner", "myskill", "g"}, nil, "source must be owner/repo"},
		{"invalid owner", []string{"skills", "add", "-bad/repo", "myskill", "g"}, nil, "invalid GitHub owner"},
		{"invalid repo", []string{"skills", "add", "owner/..", "myskill", "g"}, nil, "invalid GitHub repository"},
		{"invalid name", []string{"skills", "add", "owner/repo", "Bad_Name", "g"}, nil, "invalid skill name"},
		{"invalid group", []string{"skills", "add", "owner/repo", "myskill", "../g"}, nil, "invalid skill name"},
		{"wrong count", []string{"skills", "add", "owner/repo", "myskill"}, nil, "add expects 3"},
		{"already locked", []string{"skills", "add", "owner/repo", "myskill", "g"}, func(t *testing.T, lr lockRun) {
			writeMode(t, filepath.Join(lr.root, "skills-lock.json"), "{\n  \"version\": 1,\n  \"skills\": {\"myskill\": {}}\n}\n", 0o644)
		}, "already locked"},
		{"existing skill path in another group", []string{"skills", "add", "owner/repo", "myskill", "g"}, func(t *testing.T, lr lockRun) {
			os.MkdirAll(filepath.Join(lr.root, "skills/other/myskill"), 0o755)
		}, "skill path already exists"},
		{"existing agents path", []string{"skills", "add", "owner/repo", "myskill", "g"}, func(t *testing.T, lr lockRun) {
			writeMode(t, filepath.Join(lr.root, ".agents/skills/myskill"), "x\n", 0o644)
		}, "claude path already exists"},
		{"existing claude path", []string{"skills", "add", "owner/repo", "myskill", "g"}, func(t *testing.T, lr lockRun) {
			writeMode(t, filepath.Join(lr.root, ".claude/skills/myskill"), "x\n", 0o644)
		}, "claude path already exists"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lr := newLockRun(t)
			if c.seed != nil {
				c.seed(t, lr)
			}
			r := lr.harness(t, c.args...)
			if r.code == 0 || !strings.Contains(r.stderr, c.want) {
				t.Fatalf("%v exited %d with stderr %q, want %q", c.args, r.code, r.stderr, c.want)
			}
			if log := lr.npxLog(t); log != "" {
				t.Fatalf("npx ran for a rejected input: %q", log)
			}
		})
	}
}

func TestSkillsAddFailsWhenSkillsShDoesNotInstall(t *testing.T) {
	lr := newLockRun(t)
	lr.setNpx(t, false)
	r := lr.harness(t, "skills", "add", "owner/repo", "myskill", "g")
	if r.code == 0 || !strings.Contains(r.stderr, "did not install") {
		t.Fatalf("exited %d with stderr %q", r.code, r.stderr)
	}
}

func TestSkillsUpdateDrivesNpx(t *testing.T) {
	lr := newLockRun(t)
	lr.setNpx(t, false)
	if r := lr.harness(t, "skills", "update", "extra"); r.code == 0 {
		t.Fatal("update with an argument should fail")
	}
	ok(t, lr.harness(t, "skills", "update"), "skills update")
	if got := lr.npxLog(t); !strings.Contains(got, "npx skills@1 update --project --yes") {
		t.Fatalf("npx log = %q", got)
	}
	lr.ranOutsideCheckout(t)
}

// TestSkillsUpdateReplacesTheBodyInItsGroup has skills.sh rewrite a body and the lock in its scratch project, and expects the new body in the group that held the old one, with no stale file left and the lock copied back.
func TestSkillsUpdateReplacesTheBodyInItsGroup(t *testing.T) {
	lr := newLockRun(t)
	seedLockedSkill(t, lr, "myskill")
	writeMode(t, filepath.Join(lr.root, "skills/g/myskill/stale.md"), "old\n", 0o644)
	writeMode(t, filepath.Join(lr.shims, "npx"), `#!/bin/sh
mkdir -p .agents/skills/myskill
echo "# new" > .agents/skills/myskill/SKILL.md
sed 's/0000000000000000000000000000000000000000000000000000000000000000/1111111111111111111111111111111111111111111111111111111111111111/' skills-lock.json > lock.tmp && mv lock.tmp skills-lock.json
`, 0o755)
	ok(t, lr.harness(t, "skills", "update"), "skills update")
	if got := readFile(t, filepath.Join(lr.root, "skills/g/myskill/SKILL.md")); got != "# new\n" {
		t.Fatalf("grouped body = %q", got)
	}
	if _, err := os.Stat(filepath.Join(lr.root, "skills/g/myskill/stale.md")); !os.IsNotExist(err) {
		t.Fatalf("stale file survived the update: %v", err)
	}
	if got := readFile(t, filepath.Join(lr.root, "skills-lock.json")); !strings.Contains(got, strings.Repeat("1", 64)) {
		t.Fatalf("lock was not copied back:\n%s", got)
	}
}

// ranOutsideCheckout fails when skills.sh ran inside the checkout, where it would write a flat body beside the groups.
func (lr lockRun) ranOutsideCheckout(t *testing.T) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(lr.root, "npx.dirs"))
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range strings.Fields(string(data)) {
		if dir == lr.root || strings.HasPrefix(dir, lr.root+"/") {
			t.Fatalf("skills.sh ran inside the checkout at %s", dir)
		}
	}
}

func TestSkillsRemoveDeletesEntryBodyAndLink(t *testing.T) {
	lr := newLockRun(t)
	seedLockedSkill(t, lr, "myskill")
	ok(t, lr.harness(t, "skills", "remove", "myskill"), "skills remove")
	for _, path := range []string{"skills/g/myskill", ".agents/skills/myskill", ".claude/skills/myskill"} {
		if _, err := os.Lstat(filepath.Join(lr.root, path)); !os.IsNotExist(err) {
			t.Fatalf("%s survived: %v", path, err)
		}
	}
	if _, ok := lr.locked(t)["myskill"]; ok {
		t.Fatal("lock entry survived")
	}
}

func TestSkillsRemovePreservesUnknownTopLevelKeys(t *testing.T) {
	lr := newLockRun(t)
	writeMode(t, filepath.Join(lr.root, "skills/g/myskill/SKILL.md"), "# x\n", 0o644)
	writeMode(t, filepath.Join(lr.root, "skills-lock.json"), `{
  "version": 1,
  "future": {"keep": true},
  "skills": {
    "myskill": {"computedHash": "`+strings.Repeat("0", 64)+`"},
    "other": {"computedHash": "`+strings.Repeat("1", 64)+`"}
  }
}
`, 0o644)
	ok(t, lr.harness(t, "skills", "remove", "myskill"), "skills remove")
	data, err := os.ReadFile(filepath.Join(lr.root, "skills-lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"version"`, `"future"`, `"other"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("remove dropped %s:\n%s", want, data)
		}
	}
	if strings.Contains(string(data), "myskill") {
		t.Fatalf("remove kept myskill:\n%s", data)
	}
}

func TestSkillsRemoveKeepsEntryOrder(t *testing.T) {
	lr := newLockRun(t)
	writeMode(t, filepath.Join(lr.root, "skills/g/aaa/SKILL.md"), "# x\n", 0o644)
	writeMode(t, filepath.Join(lr.root, "skills-lock.json"), `{
  "version": 1,
  "skills": {
    "zzz": {"computedHash": "`+strings.Repeat("0", 64)+`"},
    "aaa": {"computedHash": "`+strings.Repeat("0", 64)+`"},
    "mmm": {"computedHash": "`+strings.Repeat("0", 64)+`"}
  }
}
`, 0o644)
	ok(t, lr.harness(t, "skills", "remove", "aaa"), "skills remove")
	data, err := os.ReadFile(filepath.Join(lr.root, "skills-lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if strings.Contains(got, `"aaa"`) {
		t.Fatalf("remove kept aaa:\n%s", got)
	}
	if strings.Index(got, `"zzz"`) > strings.Index(got, `"mmm"`) {
		t.Fatalf("remove reordered the surviving entries:\n%s", got)
	}
}

func TestSkillsRemoveRefusesUnsafeRemovals(t *testing.T) {
	cases := []struct {
		name string
		args []string
		seed func(t *testing.T, lr lockRun)
		want string
	}{
		{"not locked", []string{"skills", "remove", "myskill"}, nil, "not in skills-lock.json"},
		{"invalid name", []string{"skills", "remove", "Bad_Name"}, nil, "invalid skill name"},
		{"body is a file", []string{"skills", "remove", "myskill"}, func(t *testing.T, lr lockRun) {
			seedLockedSkill(t, lr, "myskill")
			os.RemoveAll(filepath.Join(lr.root, "skills/g/myskill"))
			writeMode(t, filepath.Join(lr.root, "skills/g/myskill"), "x\n", 0o644)
		}, "refusing to remove non-directory skill path"},
		{"body is a symlink", []string{"skills", "remove", "myskill"}, func(t *testing.T, lr lockRun) {
			seedLockedSkill(t, lr, "myskill")
			target := filepath.Join(lr.root, "skills/g/myskill-real")
			os.MkdirAll(target, 0o755)
			os.RemoveAll(filepath.Join(lr.root, "skills/g/myskill"))
			os.Symlink(target, filepath.Join(lr.root, "skills/g/myskill"))
		}, "refusing to remove non-directory skill path"},
		{"link is a file", []string{"skills", "remove", "myskill"}, func(t *testing.T, lr lockRun) {
			seedLockedSkill(t, lr, "myskill")
			os.Remove(filepath.Join(lr.root, ".claude/skills/myskill"))
			writeMode(t, filepath.Join(lr.root, ".claude/skills/myskill"), "x\n", 0o644)
		}, "refusing to remove non-symlink Claude path"},
		{"link points elsewhere", []string{"skills", "remove", "myskill"}, func(t *testing.T, lr lockRun) {
			seedLockedSkill(t, lr, "myskill")
			os.Remove(filepath.Join(lr.root, ".claude/skills/myskill"))
			os.Symlink("../../.agents/skills/other", filepath.Join(lr.root, ".claude/skills/myskill"))
		}, "refusing to remove unexpected Claude link"},
		{"agents link points elsewhere", []string{"skills", "remove", "myskill"}, func(t *testing.T, lr lockRun) {
			seedLockedSkill(t, lr, "myskill")
			os.Remove(filepath.Join(lr.root, ".agents/skills/myskill"))
			os.Symlink("../../skills/g/other", filepath.Join(lr.root, ".agents/skills/myskill"))
		}, "refusing to remove unexpected Claude link"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lr := newLockRun(t)
			if c.seed != nil {
				c.seed(t, lr)
			}
			r := lr.harness(t, c.args...)
			if r.code == 0 || !strings.Contains(r.stderr, c.want) {
				t.Fatalf("%v exited %d with stderr %q, want %q", c.args, r.code, r.stderr, c.want)
			}
		})
	}
}

func seedLockedSkill(t *testing.T, lr lockRun, name string) {
	t.Helper()
	writeMode(t, filepath.Join(lr.root, "skills", "g", name, "SKILL.md"), "# "+name+"\n", 0o644)
	writeMode(t, filepath.Join(lr.root, "skills-lock.json"), `{
  "version": 1,
  "skills": {
    "`+name+`": {
      "source": "owner/repo",
      "sourceType": "github",
      "skillPath": "skills/`+name+`/SKILL.md",
      "computedHash": "`+strings.Repeat("0", 64)+`"
    }
  }
}
`, 0o644)
	os.MkdirAll(filepath.Join(lr.root, ".claude/skills"), 0o755)
	os.MkdirAll(filepath.Join(lr.root, ".agents/skills"), 0o755)
	if err := os.Symlink("../../skills/g/"+name, filepath.Join(lr.root, ".agents/skills", name)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../.agents/skills/"+name, filepath.Join(lr.root, ".claude/skills", name)); err != nil {
		t.Fatal(err)
	}
}

// TestSkillsVerifyMatchesThePinnedLock recomputes the real committed bodies against the lock skills.sh wrote, so a Go collation that diverged from localeCompare would show drift.
func TestSkillsVerifyMatchesThePinnedLock(t *testing.T) {
	lr := locksVerifySandbox(t)
	r := ok(t, lr.harness(t, "skills", "verify"), "skills verify")
	data, err := os.ReadFile(filepath.Join(lr.root, "skills-lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	var lock struct {
		Skills map[string]json.RawMessage `json:"skills"`
	}
	if err := json.Unmarshal(data, &lock); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(r.stdout, ": match"); got != len(lock.Skills) {
		t.Fatalf("verify matched %d skills, want all %d locked:\n%s", got, len(lock.Skills), r.stdout)
	}
}

func TestSkillsVerifyReportsDriftForTheEditedBodyOnly(t *testing.T) {
	lr := locksVerifySandbox(t)
	body := filepath.Join(lr.root, "skills/workflow/tdd/SKILL.md")
	data, err := os.ReadFile(body)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(body, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	r := lr.harness(t, "skills", "verify")
	if r.code == 0 {
		t.Fatal("verify exited 0 with a drifted body")
	}
	if !strings.Contains(r.stdout, "tdd: drift") {
		t.Fatalf("verify did not report tdd drift:\n%s", r.stdout)
	}
	if got := strings.Count(r.stdout, ": drift"); got != 1 {
		t.Fatalf("verify reported %d drifts, want 1:\n%s", got, r.stdout)
	}
}

func TestSkillsVerifyReportsUnverifiableNotDrift(t *testing.T) {
	lr := locksVerifySandbox(t)
	var lf map[string]any
	data, err := os.ReadFile(filepath.Join(lr.root, "skills-lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &lf); err != nil {
		t.Fatal(err)
	}
	skills := lf["skills"].(map[string]any)
	// A 40-hex git tree sha is a blob install's server snapshot, not a locally recomputed folder hash.
	skills["handoff"].(map[string]any)["computedHash"] = strings.Repeat("a", 40)
	// An entry whose body is gone cannot be recomputed either.
	skills["ponytail"].(map[string]any)["computedHash"] = strings.Repeat("b", 64)
	if err := os.RemoveAll(filepath.Join(lr.root, "skills/support/ponytail")); err != nil {
		t.Fatal(err)
	}
	out, err := json.MarshalIndent(lf, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lr.root, "skills-lock.json"), append(out, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	r := ok(t, lr.harness(t, "skills", "verify"), "skills verify")
	if got := strings.Count(r.stdout, ": unverifiable"); got != 2 {
		t.Fatalf("verify reported %d unverifiable, want 2:\n%s", got, r.stdout)
	}
	if strings.Contains(r.stdout, ": drift") {
		t.Fatalf("an unverifiable entry was called drift:\n%s", r.stdout)
	}
}

// TestSkillsVerifyRefusesAnUnguardedLocalSkill fails verify for a local skill missing a guard, while locked bodies, which carry none, still pass.
func TestSkillsVerifyRefusesAnUnguardedLocalSkill(t *testing.T) {
	lr := locksVerifySandbox(t)
	ok(t, lr.harness(t, "skills", "verify"), "skills verify with guarded local skills")
	writeMode(t, filepath.Join(lr.root, "skills", "workflow", "zz-bare", "SKILL.md"), "---\nname: zz-bare\ndisable-model-invocation: true\n---\nbody\n", 0o644)
	r := lr.harness(t, "skills", "verify")
	if r.code == 0 {
		t.Fatal("verify exited 0 with an unguarded local skill")
	}
	if !strings.Contains(r.stdout+r.stderr, "zz-bare: missing opencode/autoinvoke") {
		t.Fatalf("verify did not name the skill and key:\n%s%s", r.stdout, r.stderr)
	}
}

// TestAliasesShellSyntax keeps the shipped shell entry point parseable.
func TestAliasesShellSyntax(t *testing.T) {
	if out, err := exec.Command("bash", "-n", filepath.Join("..", "..", "aliases.sh")).CombinedOutput(); err != nil {
		t.Fatalf("bash -n aliases.sh: %v\n%s", err, out)
	}
}

func locksVerifySandbox(t *testing.T) lockRun {
	t.Helper()
	lr := newLockRun(t)
	repo := filepath.Join("..", "..")
	if err := copyTree(t, filepath.Join(repo, "skills"), filepath.Join(lr.root, "skills")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(repo, "skills-lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lr.root, "skills-lock.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return lr
}

func copyTree(t *testing.T, src, dst string) error {
	t.Helper()
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
