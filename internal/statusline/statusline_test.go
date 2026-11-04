package statusline

import (
	"strings"
	"testing"
	"time"
)

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		elapsed time.Duration
		want    string
	}{
		{0, "0s"},
		{5 * time.Second, "5s"},
		{59 * time.Second, "59s"},
		{60 * time.Second, "1m"},
		{5 * time.Minute, "5m"},
		{3599 * time.Second, "59m"},
		{3600 * time.Second, "1h0m"},
		{3660 * time.Second, "1h1m"},
		{25 * time.Hour, "25h0m"},
		{-100 * time.Second, "-100s"},
	}
	for _, c := range cases {
		if got := formatDuration(c.elapsed); got != c.want {
			t.Errorf("formatDuration(%v) = %q, want %q", c.elapsed, got, c.want)
		}
	}
}

func TestColorForPct(t *testing.T) {
	cases := map[int]string{-1: green, 0: green, 49: green, 50: orange, 69: orange, 70: yellow, 89: yellow, 90: red, 150: red}
	for pct, want := range cases {
		if got := colorForPct(pct); got != want {
			t.Errorf("colorForPct(%d) = %q, want %q", pct, got, want)
		}
	}
}

func TestBuildBarClampsAndColors(t *testing.T) {
	cases := []struct {
		pct, width int
		want       string
	}{
		{24, 10, green + "●●" + dim + "○○○○○○○○" + reset},
		{0, 10, green + dim + "○○○○○○○○○○" + reset},
		{51, 10, orange + "●●●●●" + dim + "○○○○○" + reset},
		{100, 10, red + "●●●●●●●●●●" + dim + reset},
		{150, 10, red + "●●●●●●●●●●" + dim + reset},
		{-5, 10, green + dim + "○○○○○○○○○○" + reset},
	}
	for _, c := range cases {
		if got := buildBar(c.pct, c.width); got != c.want {
			t.Errorf("buildBar(%d, %d) = %q, want %q", c.pct, c.width, got, c.want)
		}
	}
}

func TestFormatResetTimeMatchesTheShellStyles(t *testing.T) {
	cases := []struct {
		when  time.Time
		style string
		want  string
	}{
		{time.Date(2026, 9, 15, 21, 30, 0, 0, time.UTC), "time", "9:30pm"},
		{time.Date(2026, 9, 15, 0, 5, 0, 0, time.UTC), "time", "12:05am"},
		{time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC), "time", "12:00pm"},
		{time.Date(2026, 9, 15, 21, 30, 0, 0, time.UTC), "datetime", "sep 15, 9:30pm"},
		{time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC), "", "sep 5"},
	}
	for _, c := range cases {
		if got := formatResetTime(c.when, c.style); got != c.want {
			t.Errorf("formatResetTime(%v, %q) = %q, want %q", c.when, c.style, got, c.want)
		}
	}
}

func TestNextMonthIsTheFirstOfTheFollowingMonth(t *testing.T) {
	if got := nextMonth(time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)); got != "oct 1" {
		t.Errorf("nextMonth(sep) = %q", got)
	}
	if got := nextMonth(time.Date(2026, 12, 20, 0, 0, 0, 0, time.UTC)); got != "jan 1" {
		t.Errorf("nextMonth(dec) = %q", got)
	}
}

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

// TestRateLinesRenderTheWholeBlock pins the current, weekly, and extra-usage lines byte for byte, the extra reset included.
func TestRateLinesRenderTheWholeBlock(t *testing.T) {
	now := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	bar0 := green + dim + "○○○○○○○○○○" + reset
	bar12 := green + "●" + dim + "○○○○○○○○○" + reset
	want := white + "current" + reset + " " + bar0 + " " + green + "  0%" + reset + " " + dim + "⟳" + reset + " " + white + reset +
		"\n" + white + "weekly" + reset + "  " + bar0 + " " + green + "  0%" + reset + " " + dim + "⟳" + reset + " " + white + reset +
		"\n" + white + "extra" + reset + "   " + bar12 + " " + green + "$12.34" + dim + "/" + reset + white + "$50.00" + reset +
		"\n" + dim + "resets " + reset + white + "oct 1" + reset
	got := rateLines(usage{fivePct: 0, sevenPct: 0, extra: true, extraPct: 12, extraUsed: "12.34", extraLimit: "50.00"}, now)
	if got != want {
		t.Errorf("rateLines = %q, want %q", got, want)
	}
	if disabled := rateLines(usage{}, now); strings.Contains(disabled, "extra") {
		t.Errorf("disabled extra usage rendered %q", disabled)
	}
}

// TestRenderPinsTheSessionSegment builds the whole line for a payload with a session clock, because the fixtures cannot carry a start time without becoming time-dependent.
func TestRenderPinsTheSessionSegment(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	env := environment{ConfigDir: t.TempDir(), Now: now}
	got := render([]byte(`{"model":{"display_name":"X"},"cwd":"/nonexistent","session":{"start_time":"2026-09-15T10:45:00Z"}}`), env)
	want := blue + "X" + reset + separator +
		"✍️ " + green + "0%" + reset + separator +
		cyan + "nonexistent" + reset + separator +
		dim + "⏱ " + reset + white + "1h15m" + reset + separator +
		dim + "◑ default" + reset
	if got != want {
		t.Errorf("render = %q, want %q", got, want)
	}
	without := render([]byte(`{"model":{"display_name":"X"},"cwd":"/nonexistent","session":{"start_time":"nonsense"}}`), env)
	wantWithout := blue + "X" + reset + separator +
		"✍️ " + green + "0%" + reset + separator +
		cyan + "nonexistent" + reset + separator +
		dim + "◑ default" + reset
	if without != wantWithout {
		t.Errorf("an unparseable start time rendered %q, want %q", without, wantWithout)
	}
}

// TestProfileFollowsClaudeConfigDir keeps the keychain item and usage cache on the active profile, so switching accounts through CLAUDE_CONFIG_DIR never shows another account's bars.
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
