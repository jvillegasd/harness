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

// reviewAccepted reports whether the attempt carries an affirmative, evidence-bound human review, and why not when it does not. The current evidence digest is passed so evaluate can append the value a new review must bind to. A legacy boolean-only record, a rejected or malformed one, and one whose input or evidence digest no longer matches all require a new review rather than retaining acceptance.
func reviewAccepted(attempt, currentDigest, evidence string) (bool, string) {
	raw, err := os.ReadFile(filepath.Join(attempt, "review.json"))
	if err != nil {
		return false, "no transcript review record"
	}
	decoded, err := decodeJSON(raw)
	if err != nil {
		return false, "unreadable transcript review record"
	}
	object, ok := decoded.(map[string]any)
	if !ok {
		return false, "transcript review record is not an object"
	}
	approved, isBool := object["reviewed"].(bool)
	if !isBool || !approved {
		return false, "transcript review was not accepted"
	}
	if strings.TrimSpace(getString(object, "reviewer")) == "" {
		return false, "transcript review names no reviewer"
	}
	if strings.TrimSpace(getString(object, "observations")) == "" {
		return false, "transcript review records no observations"
	}
	if getString(object, "input_digest") != currentDigest {
		return false, "transcript review is not bound to the current inputs"
	}
	if getString(object, "evidence_digest") != evidence {
		return false, "transcript review does not match the retained evidence"
	}
	return true, ""
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
	evidence := evidenceDigest(attempt)
	if ok, reason := reviewAccepted(attempt, currentDigest, evidence); !ok {
		return "unreviewed", []string{reason + "; bind evidence_digest " + evidence}
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
