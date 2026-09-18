package scenarios

import (
	"encoding/json"
	"fmt"
	"math"
)

// ---- matrix accessors ----

func getString(m map[string]any, key string) string {
	value, _ := m[key].(string)
	return value
}

func list(m map[string]any, key string) []any {
	value, _ := m[key].([]any)
	return value
}

func dict(m map[string]any, key string) map[string]any {
	value, _ := m[key].(map[string]any)
	return value
}

func truthy(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case bool:
		return v
	case string:
		return v != ""
	case []any:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	case json.Number:
		f, err := v.Float64()
		return err == nil && f != 0
	case float64:
		return v != 0
	default:
		return true
	}
}

// mode reports a scenario's execution mode, defaulting by whether it declares a check.
func mode(scenario map[string]any) string {
	if value := getString(scenario, "mode"); value != "" {
		return value
	}
	if truthy(scenario["check"]) {
		return "offline"
	}
	return "live"
}

// validateMatrix rejects a declaration before any session runs.
func validateMatrix(matrix map[string]any) []string {
	var errors []string
	scenarios := list(matrix, "scenarios")
	if len(scenarios) == 0 {
		return []string{"matrix declares no scenarios"}
	}
	seen := map[string]bool{}
	for index, raw := range scenarios {
		scenario, _ := raw.(map[string]any)
		where := getString(scenario, "id")
		if where == "" {
			where = fmt.Sprintf("scenario[%d]", index)
		}
		for _, field := range []string{"id", "command", "prompt"} {
			if !truthy(scenario[field]) {
				errors = append(errors, fmt.Sprintf("%s: missing %s", where, field))
			}
		}
		if seen[where] {
			errors = append(errors, fmt.Sprintf("%s: duplicate id", where))
		}
		seen[where] = true
		artifacts := dict(scenario, "artifacts")
		hasKey := false
		for _, key := range artifactKeys {
			if _, ok := artifacts[key]; ok {
				hasKey = true
				break
			}
		}
		if artifacts == nil || !hasKey {
			errors = append(errors, fmt.Sprintf("%s: declares no artifacts or assertions", where))
		} else {
			contains := artifacts["contains"]
			if contains != nil {
				if _, ok := contains.([]any); !ok {
					errors = append(errors, fmt.Sprintf("%s: contains must be a list", where))
				}
			}
			for _, item := range asList(contains) {
				entry, ok := item.(map[string]any)
				if !ok || !isString(entry["path"]) || !isString(entry["text"]) {
					errors = append(errors, fmt.Sprintf("%s: contains entry needs string path and text", where))
				}
			}
		}
		errors = append(errors, validatePublication(where, scenario, artifacts)...)
		if mode(scenario) == "offline" && !truthy(scenario["check"]) {
			errors = append(errors, fmt.Sprintf("%s: offline scenario needs a check command", where))
		}
	}
	return errors
}

// validatePublication checks the publication fixture and the side-effect assertions that only it can observe.
func validatePublication(where string, scenario, artifacts map[string]any) []string {
	var errors []string
	publication, declared := scenario["publication"]
	if _, ok := publication.(bool); declared && !ok {
		errors = append(errors, fmt.Sprintf("%s: publication must be a boolean", where))
	}
	for _, key := range sideEffectKeys {
		if _, ok := artifacts[key]; ok && !truthy(publication) {
			errors = append(errors, fmt.Sprintf("%s: side-effect assertions need a publication fixture", where))
			break
		}
	}
	for _, key := range []string{"commits", "pull_requests"} {
		if value, ok := artifacts[key]; ok {
			if f, isNumber := toNumber(value); !isNumber || f < 0 || f != math.Trunc(f) {
				errors = append(errors, fmt.Sprintf("%s: %s must be a non-negative integer", where, key))
			}
		}
	}
	if value, ok := artifacts["push"]; ok {
		if _, isBool := value.(bool); !isBool {
			errors = append(errors, fmt.Sprintf("%s: push must be a boolean", where))
		}
	}
	for _, key := range []string{"forge_allowed", "forge_forbidden"} {
		value, ok := artifacts[key]
		if !ok {
			continue
		}
		items, isList := value.([]any)
		for _, item := range items {
			if op, _ := item.(string); op == "" {
				isList = false
			}
		}
		if !isList {
			errors = append(errors, fmt.Sprintf("%s: %s must be a list of non-empty strings", where, key))
		}
	}
	return errors
}

func asList(value any) []any {
	items, _ := value.([]any)
	return items
}

func isString(value any) bool {
	_, ok := value.(string)
	return ok
}

func entry(name string, ok bool, detail string) map[string]any {
	return map[string]any{"name": name, "ok": ok, "detail": detail}
}
