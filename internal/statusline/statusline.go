// Package statusline renders Claude Code's status payload into the line the client draws. It replaces the shell renderer that used to run on every draw, so the output is byte-identical: same segments, same colors, same rate-limit bars.
package statusline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const usageText = `Usage: harness statusline

Read a Claude Code status payload as JSON on stdin and print the rendered line.
An empty payload prints Claude alone.
`

const (
	blue    = "\x1b[38;2;0;153;255m"
	orange  = "\x1b[38;2;255;176;85m"
	green   = "\x1b[38;2;0;175;80m"
	cyan    = "\x1b[38;2;86;182;194m"
	red     = "\x1b[38;2;255;85;85m"
	yellow  = "\x1b[38;2;230;200;0m"
	white   = "\x1b[38;2;220;220;220m"
	magenta = "\x1b[38;2;180;140;255m"
	dim     = "\x1b[2m"
	reset   = "\x1b[0m"

	separator = " " + dim + "│" + reset + " "
)

// Command runs `harness statusline` and returns the exit code.
func Command(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	for _, arg := range args {
		switch arg {
		case "-h", "--help":
			fmt.Fprint(stdout, usageText)
			return 0
		default:
			fmt.Fprintf(stderr, "harness statusline: unknown argument %q; run harness statusline --help\n", arg)
			return 2
		}
	}
	input, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "harness statusline: %v\n", err)
		return 1
	}
	// The empty payload prints Claude alone before any cache, keychain, or network work, exactly as the shell exited on it.
	if isEmpty(input) {
		fmt.Fprint(stdout, "Claude")
		return 0
	}
	now := time.Now()
	fmt.Fprint(stdout, render(input, environment{ConfigDir: configDir(), Now: now, Usage: loadUsage(now)}))
	return 0
}

// environment carries everything render reads beyond the payload itself: the Claude config directory whose settings name the effort level, the current time for the session clock, and the fetched usage data, nil when none is available.
type environment struct {
	ConfigDir string
	Now       time.Time
	Usage     *usage
}

// isEmpty is the shell's `[ -z "$input" ]`, which saw the payload after command substitution stripped its trailing newlines.
func isEmpty(input []byte) bool {
	return len(bytes.TrimRight(input, "\n")) == 0
}

// render returns the status line for a Claude Code payload, rendering whatever segments the payload actually carries. Command handles the empty payload before calling it.
func render(input []byte, env environment) string {
	doc := map[string]any{}
	dec := json.NewDecoder(bytes.NewReader(input))
	dec.UseNumber()
	_ = dec.Decode(&doc)

	model := text(lookup(doc, "model", "display_name"))
	if !truthy(lookup(doc, "model", "display_name")) {
		model = "Claude"
	}

	size := 200000
	if v, ok := integer(lookup(doc, "context_window", "context_window_size")); ok && v != 0 {
		size = v
	}
	current := 0
	for _, key := range []string{"input_tokens", "cache_creation_input_tokens", "cache_read_input_tokens"} {
		if v, ok := integer(lookup(doc, "context_window", "current_usage", key)); ok {
			current += v
		}
	}
	pct := 0
	if size > 0 {
		pct = current * 100 / size
	}

	effort := "default"
	if data, err := os.ReadFile(filepath.Join(env.ConfigDir, "settings.json")); err == nil {
		var settings map[string]any
		if json.Unmarshal(data, &settings) == nil {
			if truthy(settings["effortLevel"]) {
				effort = text(settings["effortLevel"])
			}
		}
	}

	cwd := text(lookup(doc, "cwd"))
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	dirname := filepath.Base(cwd)

	var branch, dirty string
	if inside, err := git(cwd, "rev-parse", "--is-inside-work-tree"); err == nil && strings.TrimSpace(inside) == "true" {
		if out, err := git(cwd, "symbolic-ref", "--short", "HEAD"); err == nil {
			branch = strings.TrimSpace(out)
		}
		if out, err := git(cwd, "status", "--porcelain"); err == nil && strings.TrimSpace(out) != "" {
			dirty = "*"
		}
	}

	duration := ""
	if start := text(lookup(doc, "session", "start_time")); start != "" && start != "null" {
		if t, err := parseISO(start); err == nil {
			duration = formatDuration(env.Now.Sub(t))
		}
	}

	var b strings.Builder
	b.WriteString(blue + model + reset)
	b.WriteString(separator)
	b.WriteString("✍️ " + colorForPct(pct) + strconv.Itoa(pct) + "%" + reset)
	b.WriteString(separator)
	b.WriteString(cyan + dirname + reset)
	if branch != "" {
		b.WriteString(" " + green + "(" + branch + red + dirty + green + ")" + reset)
	}
	if duration != "" {
		b.WriteString(separator + dim + "⏱ " + reset + white + duration + reset)
	}
	b.WriteString(separator)
	b.WriteString(effortSegment(effort))
	if env.Usage != nil {
		if lines := rateLines(*env.Usage, env.Now); lines != "" {
			b.WriteString("\n\n" + lines)
		}
	}
	return b.String()
}

// effortSegment renders the reasoning-effort marker the way the shell did, defaulting to the half-filled dot.
func effortSegment(effort string) string {
	switch effort {
	case "high":
		return magenta + "● " + effort + reset
	case "low":
		return dim + "◔ " + effort + reset
	default:
		return dim + "◑ " + effort + reset
	}
}

// formatDuration renders a session clock the way the shell did: hours and minutes, minutes, or seconds, and always a segment once the payload carries a parseable start time.
func formatDuration(elapsed time.Duration) string {
	seconds := int(elapsed.Seconds())
	switch {
	case seconds >= 3600:
		return fmt.Sprintf("%dh%dm", seconds/3600, (seconds%3600)/60)
	case seconds >= 60:
		return fmt.Sprintf("%dm", seconds/60)
	default:
		return strconv.Itoa(seconds) + "s"
	}
}

// parseISO reads the timestamps Claude Code emits, with or without a zone or fractional seconds. A zone-less value is local, as `date` read it.
func parseISO(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, value); err == nil {
			return t, nil
		}
	}
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02T15:04:05.999999999"} {
		if t, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unparseable time %q", value)
}

// colorForPct is the threshold coloring shared by the context percentage and every rate bar.
func colorForPct(pct int) string {
	switch {
	case pct >= 90:
		return red
	case pct >= 70:
		return yellow
	case pct >= 50:
		return orange
	default:
		return green
	}
}

// buildBar draws a ten-cell bar, clamping the percentage to the 0-100 range that gives it cells.
func buildBar(pct, width int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := pct * width / 100
	return colorForPct(pct) + strings.Repeat("●", filled) + dim + strings.Repeat("○", width-filled) + reset
}

func git(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	return string(out), err
}

// lookup walks nested JSON objects and returns nil for a missing path, so every field reads as optional.
func lookup(doc map[string]any, keys ...string) any {
	var current any = doc
	for _, key := range keys {
		object, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = object[key]
	}
	return current
}

// truthy mirrors jq's `//`, which falls through only on null and false.
func truthy(v any) bool {
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return true
}

func text(v any) string {
	switch value := v.(type) {
	case string:
		return value
	case json.Number:
		return value.String()
	case bool:
		return strconv.FormatBool(value)
	default:
		return ""
	}
}

func integer(v any) (int, bool) {
	switch value := v.(type) {
	case json.Number:
		if n, err := value.Int64(); err == nil {
			return int(n), true
		}
		if f, err := value.Float64(); err == nil {
			return int(f), true
		}
	case float64:
		return int(value), true
	}
	return 0, false
}
