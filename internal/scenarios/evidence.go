package scenarios

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// reviewed reports whether the attempt carries a transcript review record.
func reviewed(attempt string) bool {
	raw, err := os.ReadFile(filepath.Join(attempt, "review.json"))
	if err != nil {
		return false
	}
	data, err := decodeJSON(raw)
	if err != nil {
		return false
	}
	object, ok := data.(map[string]any)
	if !ok {
		return false
	}
	return truthy(object["reviewed"])
}

// evaluate returns the state of one retained attempt and the reasons it did not pass.
func evaluate(attempt string, scenario map[string]any, currentDigest string) (string, []string) {
	var missing []string
	for _, name := range requiredEvidence {
		if !pathExists(filepath.Join(attempt, name)) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return "incomplete", []string{"missing evidence: " + strings.Join(missing, ", ")}
	}
	provenance, err := readObject(filepath.Join(attempt, "provenance.json"))
	if err != nil {
		return "incomplete", []string{"unreadable evidence: " + err.Error()}
	}
	artifacts, err := readObject(filepath.Join(attempt, "artifacts.json"))
	if err != nil {
		return "incomplete", []string{"unreadable evidence: " + err.Error()}
	}
	metrics, err := readObject(filepath.Join(attempt, "metrics.json"))
	if err != nil {
		return "incomplete", []string{"unreadable evidence: metrics is not an object"}
	}
	if getString(provenance, "input_digest") != currentDigest {
		return "stale", []string{"inputs changed since " + getString(provenance, "recorded_at")}
	}
	if getString(metrics, "terminal_status") == "interrupted" {
		return "interrupted", []string{"session interrupted"}
	}
	if !truthy(artifacts["ok"]) {
		var failed []string
		for _, raw := range asList(artifacts["results"]) {
			result, _ := raw.(map[string]any)
			if !truthy(result["ok"]) {
				failed = append(failed, getString(result, "name"))
			}
		}
		detail := "none recorded"
		if len(failed) > 0 {
			detail = strings.Join(failed, ", ")
		}
		return "fail", []string{"artifact checks failed: " + detail}
	}
	if !reviewed(attempt) {
		return "unreviewed", []string{"no transcript review record"}
	}
	return "pass", nil
}

func readObject(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decoded, err := decodeJSON(raw)
	if err != nil {
		return nil, err
	}
	object, ok := decoded.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s is not an object", filepath.Base(path))
	}
	return object, nil
}

func recordedDigest(attempt string) (string, bool) {
	provenance, err := readObject(filepath.Join(attempt, "provenance.json"))
	if err != nil {
		return "", false
	}
	value, ok := provenance["input_digest"].(string)
	return value, ok
}

func readMetrics(attempt string) map[string]any {
	metrics, err := readObject(filepath.Join(attempt, "metrics.json"))
	if err != nil {
		return map[string]any{}
	}
	return metrics
}

func decodeJSON(data []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("trailing data after the JSON document")
	}
	return value, nil
}
