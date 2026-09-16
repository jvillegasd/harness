package scenarios

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var reconCommandPath = filepath.Join("commands", "triage.md")

var reconShapePattern = regexp.MustCompile("(?ms)^## Recon citations\n.*?in the shape `([^`]+)`")

var reconLocation = regexp.MustCompile(`^(.+):(\d+)$`)

// probeReconFields reads the finding shape from `/triage`'s copy of the canonical section.
func probeReconFields(root string) ([]string, error) {
	text, err := readText(joinPath(root, reconCommandPath))
	if err != nil {
		return nil, err
	}
	match := reconShapePattern.FindStringSubmatch(text)
	if match == nil {
		return nil, errors.New("triage command carries no recon citation shape")
	}
	return strings.Split(match[1], " | "), nil
}

// probeReconVerify splits a recon subagent's final message into grounded and refuted citations, checking only the cited line.
func probeReconVerify(report map[string]any, citeRoot, harnessRoot string) (map[string]any, error) {
	if getString(report, "type") != "text" {
		return map[string]any{"grounded": []any{}, "refuted": []any{}, "no_data": "no text report"}, nil
	}
	fields, err := probeReconFields(harnessRoot)
	if err != nil {
		return nil, err
	}
	root := resolveExisting(citeRoot)
	grounded := []any{}
	refuted := []any{}
	for _, citation := range splitLines(getString(report, "text")) {
		if strings.TrimSpace(citation) == "" {
			continue
		}
		parts := strings.SplitN(citation, " | ", len(fields))
		location := []string(nil)
		if len(parts) > 0 {
			location = reconLocation.FindStringSubmatch(parts[0])
		}
		if len(parts) < len(fields) || anyEmpty(parts) || location == nil {
			refuted = append(refuted, map[string]any{"citation": citation, "reason": "missing field"})
			continue
		}
		path := resolveExisting(joinPath(root, location[1]))
		reason := ""
		if !isWithin(root, path) {
			reason = "outside repository"
		} else if info, err := os.Stat(path); err != nil || info.IsDir() {
			reason = "missing file"
		} else {
			raw, err := os.ReadFile(path)
			if err != nil {
				reason = "missing file"
			} else {
				lines := strings.Split(string(raw), "\n")
				number, _ := strconv.Atoi(location[2])
				if number > 0 && number <= len(lines) && wholeWordMatch(lines[number-1], parts[1]) {
					grounded = append(grounded, citation)
					continue
				}
				reason = "symbol not at line"
			}
		}
		refuted = append(refuted, map[string]any{"citation": citation, "reason": reason})
	}
	return map[string]any{"grounded": grounded, "refuted": refuted}, nil
}

// wholeWordMatch matches Python's (?<!\\w)symbol(?!\\w): the symbol stands alone, with a non-word byte or an edge on each side.
func wholeWordMatch(line, symbol string) bool {
	for start := 0; ; {
		index := strings.Index(line[start:], symbol)
		if index < 0 {
			return false
		}
		index += start
		before := true
		if index > 0 {
			r, _ := utf8.DecodeLastRuneInString(line[:index])
			before = !isWordRune(r)
		}
		after := true
		if end := index + len(symbol); end < len(line) {
			r, _ := utf8.DecodeRuneInString(line[end:])
			after = !isWordRune(r)
		}
		if before && after {
			return true
		}
		start = index + 1
	}
}

// isWordRune matches Python's Unicode-aware \w for the identifier symbols the protocol allows.
func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func anyEmpty(parts []string) bool {
	for _, part := range parts {
		if part == "" {
			return true
		}
	}
	return false
}

type reconCase struct {
	key      string
	citation string
	reason   string
}

// probeReconRun applies the citation check to a valid citation and to every way a citation can be refuted.
func probeReconRun(harnessRoot string) (map[string]any, error) {
	directory, err := os.MkdirTemp("", "recon-citations-probe-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(directory)
	if err := os.WriteFile(filepath.Join(directory, "app.py"), []byte("def load_config(path):\n    return parse(path)\n"), 0o644); err != nil {
		return nil, err
	}
	valid := "app.py:1 | load_config | defines the loader"
	cases := []reconCase{
		{"missing_file_refuted", "gone.py:1 | load_config | defines the loader", "missing file"},
		{"outside_repository_refuted", "../app.py:1 | load_config | defines the loader", "outside repository"},
		{"stale_line_refuted", "app.py:9 | load_config | defines the loader", "symbol not at line"},
		{"wrong_symbol_refuted", "app.py:2 | load_config | defines the loader", "symbol not at line"},
		{"partial_symbol_refuted", "app.py:1 | load | defines the loader", "symbol not at line"},
		{"absent_field_refuted", "app.py:1 | load_config", "missing field"},
	}
	refutedMatches := func(citation, reason string) bool {
		result, err := probeReconVerify(map[string]any{"type": "text", "text": citation}, directory, harnessRoot)
		if err != nil {
			return false
		}
		refuted, _ := result["refuted"].([]any)
		if len(refuted) != 1 || len(asList(result["grounded"])) != 0 {
			return false
		}
		first, _ := refuted[0].(map[string]any)
		return getString(first, "citation") == citation && getString(first, "reason") == reason
	}
	validResult, err := probeReconVerify(map[string]any{"type": "text", "text": valid}, directory, harnessRoot)
	if err != nil {
		return nil, err
	}
	outcomes := map[string]any{
		"valid_citation_grounded": len(asList(validResult["grounded"])) == 1 &&
			asList(validResult["grounded"])[0] == valid &&
			len(asList(validResult["refuted"])) == 0,
	}
	var lines []string
	lines = append(lines, valid)
	for _, item := range cases {
		outcomes[item.key] = refutedMatches(item.citation, item.reason)
		lines = append(lines, item.citation)
	}
	mixed, err := probeReconVerify(map[string]any{"type": "text", "text": strings.Join(lines, "\n")}, directory, harnessRoot)
	if err != nil {
		return nil, err
	}
	var mixedCitations []string
	for _, item := range asList(mixed["refuted"]) {
		object, _ := item.(map[string]any)
		mixedCitations = append(mixedCitations, getString(object, "citation"))
	}
	var expectedCitations []string
	for _, item := range cases {
		expectedCitations = append(expectedCitations, item.citation)
	}
	outcomes["refuted_never_grounded"] = len(asList(mixed["grounded"])) == 1 &&
		asList(mixed["grounded"])[0] == valid &&
		equalStrings(mixedCitations, expectedCitations)
	trailing, err := probeReconVerify(map[string]any{"type": "tool_result", "text": valid}, directory, harnessRoot)
	if err != nil {
		return nil, err
	}
	outcomes["trailing_tool_result_no_data"] = getString(trailing, "no_data") == "no text report" &&
		len(asList(trailing["grounded"])) == 0 && len(asList(trailing["refuted"])) == 0
	return outcomes, nil
}

func probeReconCitations(repo, root string) error {
	outcomes, err := probeReconRun(root)
	if err != nil {
		return err
	}
	return writeArtifact(repo, "recon.json", outcomes)
}
