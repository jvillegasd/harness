package scenarios

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
)

var metricFields = []string{
	"scenario",
	"model",
	"input_tokens",
	"output_tokens",
	"cached_input",
	"tool_calls",
	"command_output_bytes",
	"wall_seconds",
	"terminal_status",
}

// toNumber normalizes a JSON number to a float64, rejecting booleans as Python's _number does.
func toNumber(value any) (float64, bool) {
	switch v := value.(type) {
	case bool:
		return 0, false
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	}
	return 0, false
}

func number(value any) any {
	f, ok := toNumber(value)
	if !ok {
		return nil
	}
	return f
}

func floatOf(value any) float64 {
	f, _ := toNumber(value)
	return f
}

func intOf(value any) int {
	return int(floatOf(value))
}

func formatNumber(value any) string {
	f, ok := toNumber(value)
	if !ok {
		return fmt.Sprint(value)
	}
	if f == math.Trunc(f) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func usageFields(usage map[string]any) map[string]any {
	details := dict(usage, "input_tokens_details")
	promptDetails := dict(usage, "prompt_tokens_details")
	cached := usage["cached_input"]
	candidates := []any{cached, usage["cache_read_input_tokens"], details["cached_tokens"], promptDetails["cached_tokens"]}
	for _, candidate := range candidates {
		if candidate != nil {
			cached = candidate
			break
		}
	}
	input := usage["input_tokens"]
	if _, ok := usage["input_tokens"]; !ok {
		input = usage["prompt_tokens"]
	}
	output := usage["output_tokens"]
	if _, ok := usage["output_tokens"]; !ok {
		output = usage["completion_tokens"]
	}
	return map[string]any{
		"input_tokens":  number(input),
		"output_tokens": number(output),
		"cached_input":  number(cached),
	}
}

// parseUsage normalizes token usage, keeping an absent measurement unknown: a summary `result` event wins, otherwise per-message usages are summed.
func parseUsage(events []any) map[string]any {
	found := map[string][]float64{"input_tokens": nil, "output_tokens": nil, "cached_input": nil}
	for _, raw := range events {
		event, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		var usage map[string]any
		if nested, ok := event["message"].(map[string]any); ok {
			usage = dict(nested, "usage")
		}
		if direct := dict(event, "usage"); direct != nil {
			usage = direct
		}
		if usage == nil {
			continue
		}
		values := usageFields(usage)
		if getString(event, "type") == "result" {
			return values
		}
		for key, value := range values {
			if f, ok := toNumber(value); ok {
				found[key] = append(found[key], f)
			}
		}
	}
	out := map[string]any{"input_tokens": nil, "output_tokens": nil, "cached_input": nil}
	for key, values := range found {
		if len(values) > 0 {
			sum := 0.0
			for _, value := range values {
				sum += value
			}
			out[key] = sum
		}
	}
	return out
}

func modelFromEvents(events []any) *string {
	for _, raw := range events {
		event, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if model, ok := event["model"].(string); ok {
			return &model
		}
		if message, ok := event["message"].(map[string]any); ok {
			if model, ok := message["model"].(string); ok {
				return &model
			}
		}
	}
	return nil
}

func countToolCalls(events []any) int {
	count := 0
	for _, raw := range events {
		event, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		switch getString(event, "type") {
		case "tool_use", "tool_call", "function_call":
			count++
		}
		if message, ok := event["message"].(map[string]any); ok {
			if content, ok := message["content"].([]any); ok {
				for _, part := range content {
					if object, ok := part.(map[string]any); ok && getString(object, "type") == "tool_use" {
						count++
					}
				}
			}
		}
	}
	return count
}

func contentBytes(content any) int {
	if list, ok := content.([]any); ok {
		total := 0
		for _, part := range list {
			if object, ok := part.(map[string]any); ok {
				text, _ := object["text"].(string)
				total += len(text)
				continue
			}
			total += len(fmt.Sprint(part))
		}
		return total
	}
	if content == nil {
		return 0
	}
	return len(fmt.Sprint(content))
}

// commandOutputBytes counts the bytes the run's tools returned across the recorded client shapes.
func commandOutputBytes(events []any) int {
	total := 0
	for _, raw := range events {
		event, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if getString(event, "type") == "tool_result" {
			content := event["content"]
			if content == nil {
				content = event["text"]
			}
			total += contentBytes(content)
		}
		if message, ok := event["message"].(map[string]any); ok {
			if content, ok := message["content"].([]any); ok {
				for _, part := range content {
					object, ok := part.(map[string]any)
					if !ok || getString(object, "type") != "tool_result" {
						continue
					}
					inner := object["content"]
					if inner == nil {
						inner = object["text"]
					}
					total += contentBytes(inner)
				}
			}
		}
	}
	return total
}

func terminalStatus(code *int) string {
	if code == nil {
		return "unknown"
	}
	if *code == 0 {
		return "ok"
	}
	if *code < 0 || *code >= 128 {
		return "interrupted"
	}
	return "failed"
}

func buildMetrics(scenario map[string]any, events []any, wallSeconds float64, status string) map[string]any {
	usage := parseUsage(events)
	model := modelFromEvents(events)
	values := map[string]any{
		"scenario":             getString(scenario, "id"),
		"model":                nil,
		"input_tokens":         usage["input_tokens"],
		"output_tokens":        usage["output_tokens"],
		"cached_input":         usage["cached_input"],
		"tool_calls":           countToolCalls(events),
		"command_output_bytes": commandOutputBytes(events),
		"wall_seconds":         round6(wallSeconds),
		"terminal_status":      status,
	}
	if model != nil {
		values["model"] = *model
	}
	out := map[string]any{}
	for _, field := range metricFields {
		out[field] = values[field]
	}
	return out
}

func round6(value float64) float64 {
	return math.Round(value*1e6) / 1e6
}
