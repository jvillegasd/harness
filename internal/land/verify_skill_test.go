package land

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jvillegasd/harness/internal/run"
)

func TestVerifySkillFindsOnlyALoadableSkill(t *testing.T) {
	cases := map[string]struct {
		files map[string]string
		links map[string]string
		want  string
	}{
		"none":               {want: ""},
		"verify-work only":   {files: map[string]string{".agents/skills/verify-work/SKILL.md": "x"}, want: ""},
		"real directory":     {files: map[string]string{".claude/skills/verify-app/SKILL.md": "x"}, want: "verify-app"},
		"linked from group":  {files: map[string]string{"skills/assurance/verify-app/SKILL.md": "x"}, links: map[string]string{".agents/skills/verify-app": "../../skills/assurance/verify-app"}, want: "verify-app"},
		"link to link":       {files: map[string]string{".agents/skills/verify-app/SKILL.md": "x"}, links: map[string]string{".claude/skills/verify-app": "../../.agents/skills/verify-app"}, want: "verify-app"},
		"dangling link":      {links: map[string]string{".agents/skills/verify-app": "../../skills/gone/verify-app"}, want: ""},
		"dangling SKILL.md":  {links: map[string]string{".agents/skills/verify-app/SKILL.md": "../../../nowhere/SKILL.md"}, want: ""},
		"link out of repo":   {links: map[string]string{".agents/skills/verify-app": "../../../../outside"}, want: ""},
		"no SKILL.md":        {files: map[string]string{".agents/skills/verify-app/README.md": "x"}, want: ""},
		"linked skills root": {files: map[string]string{".agents/skills/verify-app/SKILL.md": "x"}, links: map[string]string{".claude/skills": "../.agents/skills"}, want: "verify-app"},
		"intermediate link":  {files: map[string]string{"real/assurance/verify-app/SKILL.md": "x"}, links: map[string]string{"skills": "real", ".agents/skills/verify-app": "../../skills/assurance/verify-app"}, want: "verify-app"},
		"absolute target":    {links: map[string]string{".agents/skills/verify-app": "/etc"}, want: ""},
		"cycle":              {links: map[string]string{".agents/skills/verify-app": "verify-loop", ".agents/skills/verify-loop": "verify-app"}, want: ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			git := func(args ...string) {
				t.Helper()
				if _, err := run.Git(dir, args...); err != nil {
					t.Fatalf("git %v: %v", args, err)
				}
			}
			git("init", "-q", "-b", "main")
			git("config", "user.email", "t@example.com")
			git("config", "user.name", "t")
			files := map[string]string{"README.md": "x"}
			for p, body := range tc.files {
				files[p] = body
			}
			for p, body := range files {
				full := filepath.Join(dir, p)
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			for p, target := range tc.links {
				full := filepath.Join(dir, p)
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, full); err != nil {
					t.Fatal(err)
				}
			}
			git("add", "-A")
			git("commit", "-q", "-m", "c")
			head, err := run.Git(dir, "rev-parse", "HEAD")
			if err != nil {
				t.Fatal(err)
			}
			got, err := verifySkill(dir, head)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("verifySkill = %q, want %q", got, tc.want)
			}
		})
	}
}
