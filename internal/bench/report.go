package bench

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// metrics is the report's row order: three cost, then the two the harness
// claims to move, then three friction.
var metrics = []string{"turns", "tool_calls", "minutes", "commands", "delegation", "interrupts", "denials", "errors"}

const thin = 20

func filterSince(sessions []*session, since time.Time) []*session {
	var out []*session
	for _, s := range sessions {
		if !s.date.Before(since) {
			out = append(out, s)
		}
	}
	return out
}

func median(sessions []*session, metric string) *float64 {
	var values []float64
	for _, s := range sessions {
		if value, ok := s.metric(metric); ok {
			values = append(values, value)
		}
	}
	if len(values) == 0 {
		return nil
	}
	sort.Float64s(values)
	n := len(values)
	if n%2 == 1 {
		middle := values[n/2]
		return &middle
	}
	middle := (values[n/2-1] + values[n/2]) / 2
	return &middle
}

func cell(value *float64) string {
	if value == nil {
		return fmt.Sprintf("%10s", "-")
	}
	return fmt.Sprintf("%10.1f", *value)
}

func deltaCell(before, after *float64) string {
	if before == nil || after == nil {
		return fmt.Sprintf("%10s", "-")
	}
	return fmt.Sprintf("%+10.1f", *after-*before)
}

func block(label string, cohort []*session, split time.Time) []string {
	var before, after []*session
	for _, s := range cohort {
		if s.date.Before(split) {
			before = append(before, s)
		} else {
			after = append(after, s)
		}
	}
	lines := []string{
		"",
		label,
		fmt.Sprintf("  %-12s%10s%10s%10s", "metric", "before", "after", "delta"),
		fmt.Sprintf("  %-12s%10d%10d%10s", "sessions", len(before), len(after), ""),
	}
	for _, metric := range metrics {
		b, a := median(before, metric), median(after, metric)
		lines = append(lines, fmt.Sprintf("  %-12s%s%s%s", metric, cell(b), cell(a), deltaCell(b, a)))
	}
	if len(before) < thin || len(after) < thin {
		lines = append(lines, fmt.Sprintf("  warning: a cohort has fewer than %d sessions; noise, not a result.", thin))
	}
	return lines
}

func renderReport(sessions []*session, split time.Time, since *time.Time) string {
	if since != nil {
		sessions = filterSince(sessions, *since)
	}
	header := fmt.Sprintf("split %s", split.Format("2006-01-02"))
	if since != nil {
		header += fmt.Sprintf("  since %s", since.Format("2006-01-02"))
	}
	lines := []string{header}
	// Split by cohort only once one is configured. Unconfigured, every session
	// is personal, and a "personal" heading would draw a distinction nobody made.
	groups := []string{""}
	for _, s := range sessions {
		if s.cohort == "work" {
			groups = []string{"work", "personal"}
			break
		}
	}
	for _, client := range []string{"claude", "opencode"} {
		for _, group := range groups {
			var selected []*session
			for _, s := range sessions {
				if s.client == client && (group == "" || s.cohort == group) {
					selected = append(selected, s)
				}
			}
			if len(selected) == 0 {
				continue
			}
			label := client
			if group != "" {
				label = client + "  " + group
			}
			lines = append(lines, block(label, selected, split)...)
		}
	}
	if len(lines) == 1 {
		lines = append(lines, "no sessions found")
	}
	return strings.Join(lines, "\n")
}

func renderRows(sessions []*session) string {
	sort.SliceStable(sessions, func(i, j int) bool {
		if !sessions[i].date.Equal(sessions[j].date) {
			return sessions[i].date.Before(sessions[j].date)
		}
		return sessions[i].client < sessions[j].client
	})
	var b strings.Builder
	for _, s := range sessions {
		b.WriteString(rowJSON(s))
		b.WriteByte('\n')
	}
	return b.String()
}

func rowJSON(s *session) string {
	parts := []string{
		jsonField("client", quote(s.client)),
		jsonField("date", quote(s.date.Format("2006-01-02"))),
		jsonField("project", nullableString(s.project)),
	}
	for _, metric := range metrics {
		parts = append(parts, jsonField(metric, valueJSON(s.values[metric])))
	}
	parts = append(parts, jsonField("cohort", quote(s.cohort)))
	return "{" + strings.Join(parts, ", ") + "}"
}

func jsonField(key, value string) string { return quote(key) + ": " + value }

func quote(value string) string {
	data, err := json.Marshal(value)
	if err != nil {
		return strconv.Quote(value)
	}
	return string(data)
}

func nullableString(value *string) string {
	if value == nil {
		return "null"
	}
	return quote(*value)
}

func valueJSON(value any) string {
	switch v := value.(type) {
	case nil:
		return "null"
	case int:
		return strconv.Itoa(v)
	case float64:
		return floatJSON(v)
	}
	return "null"
}

// floatJSON matches Python's json.dumps, which keeps a decimal point on an
// integer-valued float (9.0, not 9).
func floatJSON(value float64) string {
	text := strconv.FormatFloat(value, 'g', -1, 64)
	if !strings.ContainsAny(text, ".eE") {
		text += ".0"
	}
	return text
}
