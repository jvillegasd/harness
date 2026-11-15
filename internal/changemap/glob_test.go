package changemap

import "testing"

func TestMatch(t *testing.T) {
	cases := []struct {
		pattern, name string
		want          bool
	}{

		{"POLICY.md", "POLICY.md", true},
		{"POLICY.md", "docs/POLICY.md", false},
		{"clients/*.json", "clients/a.json", true},
		{"clients/*.json", "clients/sub/a.json", false},
		{"file?.txt", "file1.txt", true},
		{"file?.txt", "file12.txt", false},
		{"v[0-9].txt", "v7.txt", true},
		{"v[0-9].txt", "vx.txt", false},

		{"**/Dockerfile", "Dockerfile", true},
		{"**/Dockerfile", "a/b/Dockerfile", true},
		{"**/Dockerfile", "a/Dockerfile.dev", false},

		{"a/**/z.go", "a/z.go", true},
		{"a/**/z.go", "a/b/z.go", true},
		{"a/**/z.go", "a/b/c/z.go", true},
		{"a/**/z.go", "b/z.go", false},

		{".github/**", ".github/workflows/ci.yml", true},
		{".github/**", ".github/CODEOWNERS", true},
		{".github/**", ".githubx/ci.yml", false},
		{"**", "any/path/at/all", true},

		{"a**", "abc", true},
		{"a**", "a/bc", false},

		{"**/migrations/**/*.sql", "db/migrations/2020/up.sql", true},
		{"**/migrations/**/*.sql", "migrations/up.sql", true},
		{"**/migrations/**/*.sql", "db/migrations/up.txt", false},
	}
	for _, c := range cases {
		if got := Match(c.pattern, c.name); got != c.want {
			t.Errorf("Match(%q, %q) = %v, want %v", c.pattern, c.name, got, c.want)
		}
	}
}

func TestValidateGlob(t *testing.T) {
	good := []string{"POLICY.md", "clients/**", "**/*.sql", "a/[bc]/?.go", "**"}
	for _, p := range good {
		if err := ValidateGlob(p); err != nil {
			t.Errorf("ValidateGlob(%q) = %v, want nil", p, err)
		}
	}
	bad := map[string]string{
		"":          "empty",
		"/etc/x":    "absolute",
		"a/../b":    "..",
		"..":        "..",
		"a/[b":      "malformed",
		"a//b":      "empty segment",
		"clients/":  "empty segment",
		"dir/[a-/x": "malformed",
	}
	for p, wantText := range bad {
		err := ValidateGlob(p)
		if err == nil {
			t.Errorf("ValidateGlob(%q) = nil, want an error", p)
			continue
		}
		if !contains(err.Error(), wantText) {
			t.Errorf("ValidateGlob(%q) = %q, want it to mention %q", p, err, wantText)
		}
	}
}
