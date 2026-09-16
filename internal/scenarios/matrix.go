package scenarios

import (
	"encoding/json"
	"fmt"
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
		if mode(scenario) == "offline" && !truthy(scenario["check"]) {
			errors = append(errors, fmt.Sprintf("%s: offline scenario needs a check command", where))
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
