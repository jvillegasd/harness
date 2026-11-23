package statusline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseUsageReducesTheResponse(t *testing.T) {
	u := parseUsage([]byte(`{
		"five_hour": {"utilization": 24.5, "resets_at": "2026-09-15T21:30:00Z"},
		"seven_day": {"utilization": 51, "resets_at": "2026-09-20T09:00:00Z"},
		"extra_usage": {"is_enabled": true, "utilization": 12, "used_credits": 1234, "monthly_limit": 5000}
	}`))
	if u == nil {
		t.Fatal("a valid response parsed to nil")
	}
	if u.fivePct != 24 || u.sevenPct != 51 || !u.extra || u.extraPct != 12 {
		t.Fatalf("percentages = %d, %d, extra %v at %d", u.fivePct, u.sevenPct, u.extra, u.extraPct)
	}
	if u.extraUsed != "12.34" || u.extraLimit != "50.00" {
		t.Fatalf("extra credits = %s/%s", u.extraUsed, u.extraLimit)
	}
	if parseUsage([]byte("not json")) != nil {
		t.Error("invalid JSON should parse to nil")
	}
}

func TestProfileFollowsClaudeConfigDir(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	if got := profileSuffix(); got != "" {
		t.Errorf("default profile suffix = %q, want none", got)
	}
	defaultCache := cachePath()
	t.Setenv("CLAUDE_CONFIG_DIR", "/profiles/work")
	if got := profileSuffix(); got != "-2ea9e238" {
		t.Errorf("profile suffix = %q, want -2ea9e238", got)
	}
	if got := configDir(); got != "/profiles/work" {
		t.Errorf("configDir = %q, want /profiles/work", got)
	}
	if cachePath() == defaultCache {
		t.Error("profile shares the default usage cache")
	}
}

func TestProfileReadsItsOwnLogin(t *testing.T) {
	profile := t.TempDir()
	shims := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", profile)
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", shims)
	service := "Claude Code-credentials" + profileSuffix()
	security := "#!/bin/sh\nif [ \"$3\" = '" + service + "' ]; then echo '{\"claudeAiOauth\":{\"accessToken\":\"profile-keychain\"}}'; exit 0; fi\necho '{\"claudeAiOauth\":{\"accessToken\":\"default-keychain\"}}'\n"
	if err := os.WriteFile(filepath.Join(shims, "security"), []byte(security), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := oauthToken(); got != "profile-keychain" {
		t.Errorf("keychain token = %q, want profile-keychain", got)
	}

	if err := os.WriteFile(filepath.Join(shims, "security"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, ".credentials.json"), []byte(`{"claudeAiOauth":{"accessToken":"profile-file"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := oauthToken(); got != "profile-file" {
		t.Errorf("credentials-file token = %q, want profile-file", got)
	}

	if err := os.WriteFile(filepath.Join(profile, "settings.json"), []byte(`{"effortLevel":"high"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := render([]byte(`{"model":{"display_name":"X"},"cwd":"/nonexistent"}`), environment{ConfigDir: configDir(), Now: time.Now()})
	if !strings.Contains(got, "high") {
		t.Errorf("render ignored the profile's effort level: %q", got)
	}
}
