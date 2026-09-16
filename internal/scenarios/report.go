package scenarios

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func report(matrix map[string]any, outRoot, root string) string {
	errors := validateMatrix(matrix)
	var lines []string
	if len(errors) == 0 {
		lines = append(lines, "local-tooling: pass")
	} else {
		lines = append(lines, "local-tooling: fail")
	}
	for _, problem := range errors {
		lines = append(lines, "  "+problem)
	}
	lines = append(lines, "behavioral-acceptance:")
	for _, raw := range list(matrix, "scenarios") {
		scenario, _ := raw.(map[string]any)
		attempts := attemptPaths(outRoot, getString(scenario, "id"))
		if len(attempts) == 0 {
			lines = append(lines, "  "+getString(scenario, "id")+": incomplete (no attempt)")
			continue
		}
		state, reasons := evaluate(attempts[len(attempts)-1], scenario, scenarioDigest(scenario, root, filepath.Join(root, "harness", "scenarios", "fixtures")))
		detail := ""
		if len(reasons) > 0 {
			detail = " (" + strings.Join(reasons, "; ") + ")"
		}
		lines = append(lines, "  "+getString(scenario, "id")+": "+state+detail)
	}
	return strings.Join(lines, "\n")
}

var efficiencyFields = []string{"input_tokens", "output_tokens", "cached_input", "tool_calls", "command_output_bytes", "wall_seconds"}

// batchCounts tallies a batch by its own recorded states, not the current checkout.
func batchCounts(root string, scenario map[string]any, digest string) (map[string]int, map[string][]map[string]any) {
	attempts := attemptPaths(root, getString(scenario, "id"))
	counts := map[string]int{}
	passing := map[string][]map[string]any{}
	if len(attempts) == 0 {
		return map[string]int{"unavailable": 1}, passing
	}
	recorded := make([]string, len(attempts))
	tally := map[string]int{}
	for i, attempt := range attempts {
		value, ok := recordedDigest(attempt)
		if ok {
			recorded[i] = value
			tally[value]++
		}
	}
	reference := digest
	if _, ok := tally[digest]; ok {
		reference = digest
	} else if len(tally) > 0 {
		best, bestCount := "", -1
		for value, count := range tally {
			if count > bestCount || (count == bestCount && value < best) {
				best, bestCount = value, count
			}
		}
		reference = best
	}
	for i, attempt := range attempts {
		value := recorded[i]
		if value != "" && value != reference {
			counts["stale"]++
			continue
		}
		current := value
		if current == "" {
			current = digest
		}
		state, _ := evaluate(attempt, scenario, current)
		counts[state]++
		if state == "pass" {
			metrics := readMetrics(attempt)
			if model, ok := metrics["model"].(string); ok {
				passing[model] = append(passing[model], metrics)
			}
		}
	}
	return counts, passing
}

func rate(counts map[string]int) string {
	if len(counts) == 1 {
		if _, ok := counts["unavailable"]; ok {
			return "unavailable"
		}
	}
	total := 0
	for _, count := range counts {
		total += count
	}
	var details []string
	for _, state := range stateOrder {
		if counts[state] > 0 {
			details = append(details, fmt.Sprintf("%s=%d", state, counts[state]))
		}
	}
	return fmt.Sprintf("pass %d/%d (%s)", counts["pass"], total, strings.Join(details, " "))
}

func median(records []map[string]any, field string) (any, bool) {
	var values []float64
	for _, record := range records {
		if value, ok := toNumber(record[field]); ok {
			values = append(values, value)
		}
	}
	if len(values) == 0 {
		return nil, false
	}
	sort.Float64s(values)
	middle := len(values) / 2
	if len(values)%2 == 1 {
		return values[middle], true
	}
	return (values[middle-1] + values[middle]) / 2, true
}

func formatValue(value any) string {
	number, ok := toNumber(value)
	if !ok {
		return "-"
	}
	if number == math.Trunc(number) {
		return strconv.FormatInt(int64(number), 10)
	}
	return fmt.Sprintf("%.3f", number)
}

// compare puts two retained batches side by side, pass rates before efficiency.
func compare(beforeRoot, afterRoot string, matrix map[string]any, root string) string {
	lines := []string{"behavioral pass rates"}
	type pair struct {
		scenario, model string
		before, after   []map[string]any
	}
	var pairs []pair
	for _, raw := range list(matrix, "scenarios") {
		scenario, _ := raw.(map[string]any)
		digest := scenarioDigest(scenario, root, filepath.Join(root, "harness", "scenarios", "fixtures"))
		beforeCounts, beforePass := batchCounts(beforeRoot, scenario, digest)
		afterCounts, afterPass := batchCounts(afterRoot, scenario, digest)
		lines = append(lines, "  "+getString(scenario, "id"))
		lines = append(lines, "    before: "+rate(beforeCounts))
		lines = append(lines, "    after:  "+rate(afterCounts))
		for model := range beforePass {
			if _, ok := afterPass[model]; !ok {
				continue
			}
			pairs = append(pairs, pair{getString(scenario, "id"), model, beforePass[model], afterPass[model]})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].scenario != pairs[j].scenario {
			return pairs[i].scenario < pairs[j].scenario
		}
		return pairs[i].model < pairs[j].model
	})
	lines = append(lines, "", "efficiency (passing runs only)")
	if len(pairs) == 0 {
		lines = append(lines, "  no comparable pair")
		return strings.Join(lines, "\n")
	}
	if len(pairs) == 1 {
		lines = append(lines, fmt.Sprintf("  smoke: one successful pair (%s model=%s), report as smoke rather than a savings claim", pairs[0].scenario, pairs[0].model))
		return strings.Join(lines, "\n")
	}
	for _, item := range pairs {
		lines = append(lines, fmt.Sprintf("  %s model=%s", item.scenario, item.model))
		lines = append(lines, fmt.Sprintf("    %-20s %10s %10s %10s", "metric", "before", "after", "delta"))
		for _, field := range efficiencyFields {
			before, beforeOK := median(item.before, field)
			after, afterOK := median(item.after, field)
			delta := any(nil)
			if beforeOK && afterOK {
				delta = after.(float64) - before.(float64)
			}
			if !beforeOK {
				before = nil
			}
			if !afterOK {
				after = nil
			}
			lines = append(lines, fmt.Sprintf("    %-20s %10s %10s %10s", field, formatValue(before), formatValue(after), formatValue(delta)))
		}
	}
	return strings.Join(lines, "\n")
}
