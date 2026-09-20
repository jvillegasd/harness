package scenarios

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// runProbe dispatches a scenario's internal probe, which is the Go replacement for the shell check that used to exec a Python probe. out receives any evidence a probe wants retained in the attempt transcript.
func runProbe(name, repo, root string, out io.Writer) error {
	switch name {
	case "landing-authorization":
		return probeLanding(repo, root)
	case "checkpoint-protocol":
		return probeCheckpoint(repo, root)
	case "checkpoint-two-session":
		return probeCheckpointTwoSession(repo, root)
	case "spec-slicing":
		return probeSpecSlicing(repo, root)
	case "harness-audit":
		return probeHarnessAudit(repo, root)
	case "recon-citations":
		return probeReconCitations(repo, root)
	case "review-triage":
		return probeReviewTriage(repo, root)
	case "gate-mixed-scope":
		return probeGateMixedScope(repo, root)
	case "gate-self-authored":
		return probeGateSelfAuthored(repo, root)
	case "route-open-shape":
		return probeRouteOpenShape(repo, root)
	case "feedback-local-first":
		return probeFeedbackLocalFirst(repo, root)
	case "feedback-approved-batch":
		return probeFeedbackApprovedBatch(repo, root)
	case "review-before-commit":
		return probeReviewBeforeCommit(repo, root)
	case "concurrent-claim":
		return probeConcurrentClaim(repo, root, out)
	case "recovery":
		return probeRecovery(repo, root)
	}
	return fmt.Errorf("unknown scenario probe %q", name)
}

func writeArtifact(repo, name string, value any) error {
	out := filepath.Join(repo, "artifacts", name)
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	return os.WriteFile(out, []byte(indentJSON(value)), 0o644)
}

var jsonBlockPattern = regexp.MustCompile("(?s)```json\n(.*?)```")

// jsonBlocks returns every fenced JSON object in a markdown document, ignoring the ones that do not parse.
func jsonBlocks(text string) []map[string]any {
	var blocks []map[string]any
	for _, match := range jsonBlockPattern.FindAllStringSubmatch(text, -1) {
		decoded, err := decodeJSON([]byte(match[1]))
		if err != nil {
			continue
		}
		if object, ok := decoded.(map[string]any); ok {
			blocks = append(blocks, object)
		}
	}
	return blocks
}

func hasAll(object map[string]any, keys []string) bool {
	for _, key := range keys {
		if _, ok := object[key]; !ok {
			return false
		}
	}
	return true
}

func readText(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// joinPath mirrors Python's Path / part: an absolute part replaces the base.
func joinPath(base, part string) string {
	if filepath.IsAbs(part) {
		return part
	}
	return filepath.Join(base, part)
}

// sortedKeys orders a string-keyed map for deterministic iteration.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func stringList(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		var out []string
		for _, item := range typed {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	}
	return nil
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

var errMissingBlock = errors.New("canonical block missing")

func findBlock(text string, keys []string) (map[string]any, error) {
	for _, block := range jsonBlocks(text) {
		if hasAll(block, keys) {
			return block, nil
		}
	}
	return nil, errMissingBlock
}

// hasDigit reports whether a string contains a decimal digit, the check RE2 cannot express as a lookahead.
func hasDigit(text string) bool {
	return strings.ContainsAny(text, "0123456789")
}

// splitLines matches Python's str.splitlines: it drops the empty tail a trailing newline would otherwise leave.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
