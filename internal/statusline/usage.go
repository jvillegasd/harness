package statusline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	usageURL      = "https://api.anthropic.com/api/oauth/usage"
	usageCacheAge = 60 * time.Second
	oauthBeta     = "oauth-2025-04-20"
	userAgent     = "claude-code/2.1.34"
)

type usage struct {
	fivePct    int
	fiveReset  string
	sevenPct   int
	sevenReset string
	extra      bool
	extraPct   int
	extraUsed  string
	extraLimit string
}

func loadUsage(now time.Time) *usage {
	path := cachePath()
	var data []byte
	if info, err := os.Stat(path); err == nil && now.Sub(info.ModTime()) < usageCacheAge {
		data, _ = os.ReadFile(path)
	} else {
		if token := oauthToken(); token != "" {
			if body, err := fetchUsage(token); err == nil && hasFiveHour(body) {
				data = body
				_ = os.MkdirAll(filepath.Dir(path), 0o755)
				_ = os.WriteFile(path, body, 0o644)
			}
		}
		if len(data) == 0 {
			data, _ = os.ReadFile(path)
		}
	}
	if len(data) == 0 {
		return nil
	}
	return parseUsage(data)
}

func cachePath() string {
	return filepath.Join(os.TempDir(), "claude", "statusline-usage-cache"+profileSuffix()+".json")
}

func configDir() string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

func profileSuffix() string {
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(dir))
	return "-" + hex.EncodeToString(sum[:])[:8]
}

func hasFiveHour(body []byte) bool {
	var doc map[string]any
	if json.Unmarshal(body, &doc) != nil {
		return false
	}
	return truthy(doc["five_hour"])
}

func parseUsage(data []byte) *usage {
	var doc map[string]any
	if json.Unmarshal(data, &doc) != nil {
		return nil
	}
	u := &usage{
		fivePct:    roundPct(lookup(doc, "five_hour", "utilization")),
		fiveReset:  resetString(lookup(doc, "five_hour", "resets_at"), "time"),
		sevenPct:   roundPct(lookup(doc, "seven_day", "utilization")),
		sevenReset: resetString(lookup(doc, "seven_day", "resets_at"), "datetime"),
	}
	if enabled, _ := lookup(doc, "extra_usage", "is_enabled").(bool); enabled {
		u.extra = true
		u.extraPct = roundPct(lookup(doc, "extra_usage", "utilization"))
		u.extraUsed = fmt.Sprintf("%.2f", number(lookup(doc, "extra_usage", "used_credits"))/100)
		u.extraLimit = fmt.Sprintf("%.2f", number(lookup(doc, "extra_usage", "monthly_limit"))/100)
	}
	return u
}

func rateLines(u usage, now time.Time) string {
	var b strings.Builder
	b.WriteString(white + "current" + reset + " " + buildBar(u.fivePct, 10) + " " + colorForPct(u.fivePct) + fmt.Sprintf("%3d", u.fivePct) + "%" + reset + " " + dim + "⟳" + reset + " " + white + u.fiveReset + reset)
	b.WriteString("\n" + white + "weekly" + reset + "  " + buildBar(u.sevenPct, 10) + " " + colorForPct(u.sevenPct) + fmt.Sprintf("%3d", u.sevenPct) + "%" + reset + " " + dim + "⟳" + reset + " " + white + u.sevenReset + reset)
	if u.extra {
		b.WriteString("\n" + white + "extra" + reset + "   " + buildBar(u.extraPct, 10) + " " + colorForPct(u.extraPct) + "$" + u.extraUsed + dim + "/" + reset + white + "$" + u.extraLimit + reset)
		b.WriteString("\n" + dim + "resets " + reset + white + nextMonth(now) + reset)
	}
	return b.String()
}

func roundPct(v any) int {
	rounded, _ := strconv.Atoi(strconv.FormatFloat(number(v), 'f', 0, 64))
	return rounded
}

func number(v any) float64 {
	switch value := v.(type) {
	case float64:
		return value
	case json.Number:
		f, _ := value.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(value, 64)
		return f
	}
	return 0
}

func resetString(v any, style string) string {
	s, _ := v.(string)
	if s == "" || s == "null" {
		return ""
	}
	t, err := parseISO(s)
	if err != nil {
		return ""
	}
	return formatResetTime(t.Local(), style)
}

func formatResetTime(t time.Time, style string) string {
	switch style {
	case "time":
		return strings.ToLower(t.Format("3:04pm"))
	case "datetime":
		return strings.ToLower(t.Format("Jan 2, 3:04pm"))
	default:
		return strings.ToLower(t.Format("Jan 2"))
	}
}

func nextMonth(now time.Time) string {
	local := now.Local()
	first := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	return strings.ToLower(first.AddDate(0, 1, 0).Format("Jan 2"))
}

func oauthToken() string {
	service := "Claude Code-credentials" + profileSuffix()
	if token := os.Getenv("CLAUDE_CODE_OAUTH_TOKEN"); token != "" && token != "null" {
		return token
	}
	if _, err := exec.LookPath("security"); err == nil {
		if out, err := exec.Command("security", "find-generic-password", "-s", service, "-w").Output(); err == nil {
			if token := accessToken(out); token != "" {
				return token
			}
		}
	}
	if data, err := os.ReadFile(filepath.Join(configDir(), ".credentials.json")); err == nil {
		if token := accessToken(data); token != "" {
			return token
		}
	}
	if _, err := exec.LookPath("secret-tool"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(ctx, "secret-tool", "lookup", "service", service).Output(); err == nil {
			if token := accessToken(out); token != "" {
				return token
			}
		}
	}
	return ""
}

func accessToken(blob []byte) string {
	var doc map[string]any
	if json.Unmarshal(blob, &doc) != nil {
		return ""
	}
	token := text(lookup(doc, "claudeAiOauth", "accessToken"))
	if token == "" || token == "null" {
		return ""
	}
	return token
}

func fetchUsage(token string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, usageURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("anthropic-beta", oauthBeta)
	request.Header.Set("User-Agent", userAgent)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	return io.ReadAll(io.LimitReader(response.Body, 1<<20))
}
