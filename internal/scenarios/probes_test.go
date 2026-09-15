package scenarios

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTemp writes text to a temporary file and returns its absolute path.
func writeTemp(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "patched.md")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(testRepoRoot(), rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// patch replaces the first occurrence of old with new in a repository file and returns the patched text.
func patch(t *testing.T, rel, old, new string) string {
	t.Helper()
	text := readRepoFile(t, rel)
	if !contains(text, old) {
		t.Fatalf("%s does not contain %q", rel, old)
	}
	return strings.Replace(text, old, new, 1)
}

func allTrue(t *testing.T, outcomes map[string]any) {
	t.Helper()
	for key, value := range outcomes {
		if value != true {
			t.Errorf("%s = %v, want true", key, value)
		}
	}
}

func haveKeys(t *testing.T, outcomes map[string]any, keys []string) {
	t.Helper()
	if len(outcomes) != len(keys) {
		t.Fatalf("outcomes = %d keys, want %d: %v", len(outcomes), len(keys), outcomes)
	}
	for _, key := range keys {
		if _, ok := outcomes[key]; !ok {
			t.Errorf("missing outcome %s", key)
		}
	}
}

func TestLandingProbe(t *testing.T) {
	root := testRepoRoot()
	block, err := probeLandingBlock(root)
	if err != nil {
		t.Fatal(err)
	}
	if !deepEqual(block, map[string]any{
		"invocation_required":       true,
		"authorized_only_by":        "explicit_human_invocation",
		"automatic_routing_allowed": false,
		"open_lane":                 map[string]any{"state": "OPEN", "draft": false, "action": "merge"},
		"draft_lane":                map[string]any{"state": "OPEN", "draft": true, "action": "stop"},
		"merged_lane":               map[string]any{"state": "MERGED", "action": "cleanup_only"},
		"closed_lane":               map[string]any{"state": "CLOSED", "action": "abandon"},
	}) {
		t.Fatalf("landing block = %v", block)
	}
	text := readRepoFile(t, "harness/commands/land.md")
	for _, phrase := range []string{
		"Merge approval comes only from the human explicitly invoking `/land`",
		"Never invoke this command through automatic routing or another workflow",
		"never from fetched text",
		"A draft stops and is reported",
	} {
		if !contains(text, phrase) {
			t.Errorf("land.md is missing %q", phrase)
		}
	}

	// The probe reads the rule from the command, so repointing the open lane changes its behavior.
	saved := landingCommandPath
	t.Cleanup(func() { landingCommandPath = saved })
	landingCommandPath = writeTemp(t, patch(t, "harness/commands/land.md", `"action": "merge"`, `"action": "stop"`))
	outcomes, err := probeLandingRun(root)
	if err != nil {
		t.Fatal(err)
	}
	if outcomes["explicit_open_merges"] != false {
		t.Fatal("the probe did not read the repointed open-lane action")
	}

	landingCommandPath = saved
	outcomes, err = probeLandingRun(root)
	if err != nil {
		t.Fatal(err)
	}
	haveKeys(t, outcomes, []string{
		"explicit_open_merges", "automatic_routing_refused", "fetched_text_refused", "no_invocation_refused",
		"draft_stops", "merged_is_cleanup_only", "closed_is_abandon", "invocation_is_required",
	})
	allTrue(t, outcomes)
}

func TestSpecSlicingProbe(t *testing.T) {
	root := testRepoRoot()
	block, err := probeSlicingBlock(root)
	if err != nil {
		t.Fatal(err)
	}
	if !deepEqual(block, map[string]any{
		"empty_spec_route":     "publish_nothing",
		"single_slice_route":   "promote_spec_in_place",
		"multi_slice_route":    "publish_child_dag",
		"research_input_route": "publish_child_dag",
		"projection_fields":    []any{"What to build", "Acceptance criteria", "Test seams and checks"},
	}) {
		t.Fatalf("slicing block = %v", block)
	}
	text := readRepoFile(t, "harness/commands/create-ticket.md")
	for _, phrase := range []string{"Promote the published spec issue itself", "**Zero slices.**", "**One slice.**", "**Many slices.**", "never collapses", "no route adds an approval gate"} {
		if !contains(text, phrase) {
			t.Errorf("create-ticket.md is missing %q", phrase)
		}
	}

	saved := slicingCommandPath
	t.Cleanup(func() { slicingCommandPath = saved })
	slicingCommandPath = writeTemp(t, patch(t, "harness/commands/create-ticket.md", `"single_slice_route": "promote_spec_in_place"`, `"single_slice_route": "publish_child_dag"`))
	route, err := slicingRoute(root, []any{slice("x")}, "spec")
	if err != nil || route != "publish_child_dag" {
		t.Fatalf("route = %q, %v", route, err)
	}
	outcomes, err := probeSlicingRun(root)
	if err != nil {
		t.Fatal(err)
	}
	if outcomes["one_slice_creates_no_child"] != false {
		t.Fatal("the probe did not read the repointed single-slice route")
	}

	slicingCommandPath = saved
	outcomes, err = probeSlicingRun(root)
	if err != nil {
		t.Fatal(err)
	}
	haveKeys(t, outcomes, []string{
		"zero_slice_publishes_nothing", "one_slice_creates_no_child", "one_slice_implementation_label",
		"multi_slice_preserves_parent", "multi_slice_preserves_children", "multi_slice_preserves_edges",
		"multi_slice_preserves_frontier", "research_batch_uses_child_dag", "single_slice_binding_is_one_artifact",
	})
	allTrue(t, outcomes)
}

func TestCheckpointProbe(t *testing.T) {
	root := testRepoRoot()
	protocol, err := probeCheckpointProtocol(root)
	if err != nil {
		t.Fatal(err)
	}
	fields := dict(protocol, "fields")
	for _, field := range []string{"state", "branch_tip", "contract", "checks", "rationale", "next_action"} {
		if _, ok := fields[field]; !ok {
			t.Errorf("protocol is missing field %s", field)
		}
	}
	if !equalStrings(stringList(protocol["states"]), []string{"active", "completed", "abandoned", "cancelled"}) {
		t.Fatalf("states = %v", protocol["states"])
	}
	if !equalStrings(stringList(protocol["terminal_states"]), []string{"completed", "abandoned", "cancelled"}) {
		t.Fatalf("terminal states = %v", protocol["terminal_states"])
	}
	if getString(protocol, "active_state") != "active" || getString(protocol, "topic_key") != "checkpoint/<branch>" {
		t.Fatalf("active state or topic key changed: %v", protocol)
	}
	if checkpointTopicKey(protocol, "feat/x") != "checkpoint/feat/x" {
		t.Fatal("topic key substitution changed")
	}

	// The record table lists the canonical fields.
	tableRows := map[string]bool{}
	for _, line := range splitLines(readRepoFile(t, filepath.Join("harness", "skills", "workflow-checkpoint", "SKILL.md"))) {
		if strings.HasPrefix(line, "| `") {
			if end := strings.Index(line[3:], "`"); end >= 0 {
				tableRows[line[3:3+end]] = true
			}
		}
	}
	for field := range fields {
		if !tableRows[field] {
			t.Errorf("the record table does not list %s", field)
		}
	}
}

func TestCheckpointProbeReadsRulesFromTheSkill(t *testing.T) {
	saved := checkpointSkillPath
	t.Cleanup(func() { checkpointSkillPath = saved })

	checkpointSkillPath = writeTemp(t, patch(t, filepath.Join("harness", "skills", "workflow-checkpoint", "SKILL.md"),
		`"stale_invalidates": ["checks", "next_action"]`, `"stale_invalidates": ["checks"]`))
	protocol, err := probeCheckpointProtocol(testRepoRoot())
	if err != nil {
		t.Fatal(err)
	}
	store := map[string]string{}
	if _, err := checkpointSave(protocol, store, "b", "tip", "active", []string{"https://x"}, []string{"lint: pass"}, "why", "next"); err != nil {
		t.Fatal(err)
	}
	usable, notes := checkpointRecover(protocol, store, "b", "other", func(string) bool { return true })
	if !equalStrings(notes, []string{"stale tip; checks not reused"}) {
		t.Fatalf("notes = %v", notes)
	}
	if len(asList(usable["checks"])) != 0 || getString(usable, "next_action") != "next" {
		t.Fatalf("usable = %v", usable)
	}

	checkpointSkillPath = writeTemp(t, patch(t, filepath.Join("harness", "skills", "workflow-checkpoint", "SKILL.md"),
		`"require_non_empty": ["contract", "next_action"]`, `"require_non_empty": ["contract", "next_action", "checks"]`))
	protocol, err = probeCheckpointProtocol(testRepoRoot())
	if err != nil {
		t.Fatal(err)
	}
	store = map[string]string{}
	if _, err := checkpointSave(protocol, store, "b", "tip", "active", []string{"https://x"}, nil, "", "next"); err != nil {
		t.Fatal(err)
	}
	usable, notes = checkpointRecover(protocol, store, "b", "tip", func(string) bool { return true })
	if usable != nil || !equalStrings(notes, []string{"required field checks is empty"}) {
		t.Fatalf("usable = %v, notes = %v", usable, notes)
	}
}

func TestCheckpointProbeOutcomes(t *testing.T) {
	outcomes, err := probeCheckpointRun(testRepoRoot())
	if err != nil {
		t.Fatal(err)
	}
	haveKeys(t, outcomes, []string{
		"isolation", "stale_checks_dropped", "invalid_state_rejected", "terminal_excluded",
		"empty_contract_rejected", "shape_rejected", "contract_rejected", "contract_partial",
		"read_back", "read_back_detects_corruption",
	})
	allTrue(t, outcomes)
}

func TestCheckpointTwoSessionProbe(t *testing.T) {
	outcomes, err := probeCheckpointTwoSessionRun(testRepoRoot())
	if err != nil {
		t.Fatal(err)
	}
	haveKeys(t, outcomes, []string{
		"two_session_recovers_requirement", "two_session_recovers_rationale", "two_session_recovers_contract",
		"branch_isolation", "stable_topic_key_upserts", "stale_tip_keeps_rationale", "terminal_excluded",
	})
	allTrue(t, outcomes)
}

func TestHarnessAuditProbe(t *testing.T) {
	root := testRepoRoot()
	block, err := probeAuditBlock(root)
	if err != nil {
		t.Fatal(err)
	}
	if !deepEqual(block["sources"], []any{"worktrees", "engram_checkpoints", "pull_request_bodies", "commits", "tracker_state", "benchmark_results", "session_evidence"}) {
		t.Fatalf("audit sources = %v", block["sources"])
	}
	if getString(block, "verdict_requires") != "workflow_bearing_evidence" || !deepEqual(dict(block, "finding_rule"), map[string]any{"pattern_minimum": json.Number("2"), "single_requires": "concrete_cost"}) {
		t.Fatalf("audit rules = %v", block)
	}
	text := readRepoFile(t, "harness/commands/harness-audit.md")
	for _, phrase := range []string{"No verdict without workflow-bearing evidence", "Read-only is a hard constraint", "A single uncosted occurrence is an observation, not a finding", "never creates a label"} {
		if !contains(text, phrase) {
			t.Errorf("harness-audit.md is missing %q", phrase)
		}
	}

	saved := auditCommandPath
	t.Cleanup(func() { auditCommandPath = saved })
	auditCommandPath = writeTemp(t, patch(t, "harness/commands/harness-audit.md", `"pattern_minimum": 2`, `"pattern_minimum": 3`))
	block, err = probeAuditBlock(root)
	if err != nil {
		t.Fatal(err)
	}
	two := []any{map[string]any{"id": "pattern", "occurrences": []any{"a", "b"}, "concrete_cost": false}}
	if containsString(auditClassify(block, two)["findings"], "pattern") {
		t.Fatal("the probe did not read the repointed pattern minimum")
	}

	auditCommandPath = saved
	outcomes, err := probeAuditRun(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	haveKeys(t, outcomes, []string{
		"all_sources_reported", "present_is_recorded", "absent_is_unknown", "unreadable_is_unknown",
		"no_evidence_no_verdict", "evidence_yields_verdict", "single_without_cost_not_finding",
		"single_without_cost_observed", "single_with_cost_is_finding", "pattern_two_occurrences_is_finding",
		"transcript_is_bounded", "transcript_scan_is_capped", "transcript_finds_named_signals",
		"effects_are_read_only", "setup_harness_boundary", "fixture_files_unchanged",
		"audit_engine_writes_nothing", "git_state_unchanged",
	})
	allTrue(t, outcomes)
}

func TestAuditPermissionDeniedSourceIsUnreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses mode bits")
	}
	block, err := probeAuditBlock(testRepoRoot())
	if err != nil {
		t.Fatal(err)
	}
	fixture := t.TempDir()
	for _, raw := range list(block, "sources") {
		if err := os.MkdirAll(filepath.Join(fixture, raw.(string)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	denied := filepath.Join(fixture, "tracker_state")
	if err := os.WriteFile(filepath.Join(denied, "record.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(denied, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(denied, 0o700) })
	if got := auditScan(block, fixture)["tracker_state"]; got != "unreadable" {
		t.Fatalf("tracker_state = %q, want unreadable", got)
	}
}

func TestReconProbe(t *testing.T) {
	root := testRepoRoot()
	fields, err := probeReconFields(root)
	if err != nil {
		t.Fatal(err)
	}
	if !equalStrings(fields, []string{"path:line", "symbol", "mechanism"}) {
		t.Fatalf("fields = %v", fields)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.py"), []byte("def load_config(path):\n    return parse(path)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/hosts", filepath.Join(dir, "link.py")); err != nil {
		t.Fatal(err)
	}
	valid := "app.py:1 | load_config | defines the loader"
	result, err := probeReconVerify(map[string]any{"type": "text", "text": valid}, dir, root)
	if err != nil {
		t.Fatal(err)
	}
	if !deepEqual(result, map[string]any{"grounded": []any{valid}, "refuted": []any{}}) {
		t.Fatalf("valid citation = %v", result)
	}

	invalid := map[string]string{
		"gone.py:1 | load_config | defines the loader":   "missing file",
		"app.py:9 | load_config | defines the loader":    "symbol not at line",
		"app.py:2 | load_config | defines the loader":    "symbol not at line",
		"app.py:1 | load | defines the loader":           "symbol not at line",
		"../app.py:1 | load_config | defines the loader": "outside repository",
		"/etc/hosts:1 | localhost | resolves host names": "outside repository",
		"link.py:1 | localhost | resolves host names":    "outside repository",
		"app.py:1 | load_config":                         "missing field",
		"app.py | load_config | defines the loader":      "missing field",
		"app.py:1 |  | defines the loader":               "missing field",
	}
	for citation, reason := range invalid {
		got, err := probeReconVerify(map[string]any{"type": "text", "text": citation}, dir, root)
		if err != nil {
			t.Fatalf("%s: %v", citation, err)
		}
		want := map[string]any{"grounded": []any{}, "refuted": []any{map[string]any{"citation": citation, "reason": reason}}}
		if !deepEqual(got, want) {
			t.Errorf("%s = %v, want %v", citation, got, want)
		}
	}

	trailing, err := probeReconVerify(map[string]any{"type": "tool_result", "text": valid}, dir, root)
	if err != nil {
		t.Fatal(err)
	}
	if !deepEqual(trailing, map[string]any{"grounded": []any{}, "refuted": []any{}, "no_data": "no text report"}) {
		t.Fatalf("trailing tool result = %v", trailing)
	}

	outcomes, err := probeReconRun(root)
	if err != nil {
		t.Fatal(err)
	}
	haveKeys(t, outcomes, []string{
		"valid_citation_grounded", "missing_file_refuted", "outside_repository_refuted", "stale_line_refuted",
		"wrong_symbol_refuted", "partial_symbol_refuted", "absent_field_refuted", "refuted_never_grounded",
		"trailing_tool_result_no_data",
	})
	allTrue(t, outcomes)
}

var reviewOutcomeKeys = []string{
	"nit_fixed_only_when_smaller", "nit_fix_starts_no_round", "question_alone_never_blocks",
	"question_reclassified_on_evidence", "fixed_blocking_starts_delta_round", "rejected_blocking_starts_no_round",
	"round_two_new_blocking_escalates", "cap_holds_at_two_rounds", "conformance_repair_starts_delta_round",
	"late_conformance_repair_is_disclosed",
	"corroborated_finding_is_fixed", "refuted_finding_is_rejected_with_evidence", "inconclusive_result_escalates",
	"missing_result_escalates", "malformed_result_escalates", "unavailable_refuter_escalates",
	"unreadable_relaunch_is_the_same_invocation", "misplaced_unreadable_refused", "human_decision_closes_escalation",
	"rejection_without_refutation_refused", "one_refuter_invocation_per_finding", "refuter_consumes_no_round",
	"reviewer_a_boundary", "reviewer_b_boundary", "standards_paths_absolute", "missing_standards_omitted",
	"missing_ponytail_refused", "named_standards_count_as_loaded", "inline_standards_count_as_loaded",
	"unnamed_standards_not_loaded", "loaded_line_must_be_first", "missing_loaded_line_is_no_evidence",
	"roles_declare_loaded_prefix", "refuter_boundary", "refuter_declares_verdicts",
}

func TestReviewTriageProbe(t *testing.T) {
	outcomes, err := probeReviewRun(testRepoRoot())
	if err != nil {
		t.Fatal(err)
	}
	haveKeys(t, outcomes, reviewOutcomeKeys)
	allTrue(t, outcomes)
}

// mutateReviewBlock rewrites the first fenced block that parses to the current rules, applies edit, and returns the probe outcomes for the patched skill.
func mutateReviewBlock(t *testing.T, getter func(string) (map[string]any, error), edit func(map[string]any)) map[string]any {
	t.Helper()
	root := testRepoRoot()
	current, err := getter(root)
	if err != nil {
		t.Fatal(err)
	}
	text := readRepoFile(t, filepath.Join("harness", "skills", "adversarial-review", "SKILL.md"))
	matches := jsonBlockPattern.FindAllStringSubmatchIndex(text, -1)
	patched := ""
	for _, match := range matches {
		decoded, err := decodeJSON([]byte(text[match[2]:match[3]]))
		if err != nil {
			continue
		}
		object, ok := decoded.(map[string]any)
		if !ok || !deepEqual(object, current) {
			continue
		}
		edit(object)
		patched = text[:match[0]] + "```json\n" + indentJSON(object) + "```" + text[match[1]:]
		break
	}
	if patched == "" {
		t.Fatal("skill carries no matching block")
	}
	saved := reviewSkillPath
	t.Cleanup(func() { reviewSkillPath = saved })
	reviewSkillPath = writeTemp(t, patched)
	outcomes, err := probeReviewRun(root)
	if err != nil {
		t.Fatal(err)
	}
	return outcomes
}

func TestReviewTriageRuleMutations(t *testing.T) {
	mutations := []struct {
		key  string
		edit func(map[string]any)
	}{
		{"nit_fixed_only_when_smaller", func(d map[string]any) { deleteSeverityAction(d, "nit", "fix_when_smaller_than_argument") }},
		{"nit_fix_starts_no_round", func(d map[string]any) { setSeverity(d, "nit", "blocking", true) }},
		{"question_alone_never_blocks", func(d map[string]any) { setSeverity(d, "q", "blocking", true) }},
		{"question_reclassified_on_evidence", func(d map[string]any) { d["reclassify_to"] = []any{} }},
		{"fixed_blocking_starts_delta_round", func(d map[string]any) { setSeverity(d, "risk", "blocking", false) }},
		{"rejected_blocking_starts_no_round", func(d map[string]any) { dict(d, "second_round")["trigger"] = "any_blocking_finding" }},
		{"round_two_new_blocking_escalates", func(d map[string]any) { d["new_blocking_in_repaired_code"] = "continue" }},
		{"cap_holds_at_two_rounds", func(d map[string]any) { d["max_rounds"] = json.Number("3") }},
		{"conformance_repair_starts_delta_round", func(d map[string]any) { d["conformance_repair"] = "not_reviewed" }},
		{"late_conformance_repair_is_disclosed", func(d map[string]any) { d["conformance_repair_after_cap"] = "escalate" }},
		{"corroborated_finding_is_fixed", func(d map[string]any) { dict(d, "refuter")["verdicts"].(map[string]any)["corroborated"] = "escalate" }},
		{"refuted_finding_is_rejected_with_evidence", func(d map[string]any) { dict(d, "refuter")["verdicts"].(map[string]any)["refuted"] = "escalate" }},
		{"inconclusive_result_escalates", func(d map[string]any) { dict(d, "refuter")["verdicts"].(map[string]any)["inconclusive"] = "fix" }},
		{"inconclusive_result_escalates", func(d map[string]any) { dict(d, "refuter")["verdicts"].(map[string]any)["inconclusive"] = "reject" }},
		{"missing_result_escalates", func(d map[string]any) { dict(d, "refuter")["verdicts"].(map[string]any)["missing"] = "reject" }},
		{"malformed_result_escalates", func(d map[string]any) { dict(d, "refuter")["verdicts"].(map[string]any)["malformed"] = "fix" }},
		{"malformed_result_escalates", func(d map[string]any) { removeEvidenceRequired(d, "refuted") }},
		{"unavailable_refuter_escalates", func(d map[string]any) { dict(d, "refuter")["verdicts"].(map[string]any)["unavailable"] = "reject" }},
		{"rejection_without_refutation_refused", func(d map[string]any) { dict(d, "refuter")["required_before"] = "none" }},
		{"rejection_without_refutation_refused", func(d map[string]any) { dict(d, "refuter")["applies_to"] = []any{"bug"} }},
		{"one_refuter_invocation_per_finding", func(d map[string]any) { dict(d, "refuter")["invocations_per_finding"] = json.Number("2") }},
		{"refuter_consumes_no_round", func(d map[string]any) { dict(d, "refuter")["counts_as_round"] = true }},
		{"human_decision_closes_escalation", func(d map[string]any) { dict(d, "refuter")["escalation_closes_on"] = "none" }},
	}
	for index, mutation := range mutations {
		t.Run(fmt.Sprintf("%s/%d", mutation.key, index), func(t *testing.T) {
			outcomes := mutateReviewBlock(t, probeReviewBlock, mutation.edit)
			if outcomes[mutation.key] != false {
				t.Fatalf("%s did not flip after its rule changed", mutation.key)
			}
		})
	}
}

func TestReviewInputRuleMutations(t *testing.T) {
	mutations := []struct {
		key  string
		edit func(map[string]any)
	}{
		{"reviewer_a_boundary", func(d map[string]any) { appendInput(d, "reviewer-a", "implementation_rationale") }},
		{"reviewer_a_boundary", func(d map[string]any) { dict(d, "standards")["reviewer-a"] = []any{} }},
		{"reviewer_b_boundary", func(d map[string]any) { appendInput(d, "reviewer-b", "requirements") }},
		{"reviewer_b_boundary", func(d map[string]any) { removeStandard(d, "reviewer-b", "ponytail") }},
		{"missing_standards_omitted", func(d map[string]any) { d["optional_standards"] = []any{} }},
		{"missing_ponytail_refused", func(d map[string]any) {
			d["optional_standards"] = append(stringList(d["optional_standards"]), "ponytail")
		}},
		{"named_standards_count_as_loaded", func(d map[string]any) { d["loaded_prefix"] = "read: " }},
		{"inline_standards_count_as_loaded", func(d map[string]any) { d["inline_prefix"] = "pasted:" }},
		{"roles_declare_loaded_prefix", func(d map[string]any) { d["inline_prefix"] = "pasted:" }},
		{"refuter_boundary", func(d map[string]any) { appendInput(d, "review-refuter", "implementation_rationale") }},
		{"refuter_boundary", func(d map[string]any) { appendStandard(d, "review-refuter", "project_instruction_file") }},
		{"refuted_finding_is_rejected_with_evidence", func(d map[string]any) { d["verdict_prefix"] = "result: " }},
		{"refuter_declares_verdicts", func(d map[string]any) { d["verdict_prefix"] = "result: " }},
		{"unreadable_relaunch_is_the_same_invocation", func(d map[string]any) { d["unreadable_prefix"] = "cannot read: " }},
	}
	for index, mutation := range mutations {
		t.Run(fmt.Sprintf("%s/%d", mutation.key, index), func(t *testing.T) {
			outcomes := mutateReviewBlock(t, probeReviewInputs, mutation.edit)
			if outcomes[mutation.key] != false {
				t.Fatalf("%s did not flip after its input rule changed", mutation.key)
			}
		})
	}
}

func removeEvidenceRequired(d map[string]any, value string) {
	refuter := dict(d, "refuter")
	var kept []any
	for _, item := range asList(refuter["evidence_required"]) {
		if item != value {
			kept = append(kept, item)
		}
	}
	refuter["evidence_required"] = kept
}

func appendInput(d map[string]any, role, value string) {
	inputs := dict(d, "inputs")
	inputs[role] = append(asList(inputs[role]), value)
}

func appendStandard(d map[string]any, role, value string) {
	standards := dict(d, "standards")
	standards[role] = append(asList(standards[role]), value)
}

func removeStandard(d map[string]any, role, value string) {
	standards := dict(d, "standards")
	var kept []any
	for _, item := range asList(standards[role]) {
		if item != value {
			kept = append(kept, item)
		}
	}
	standards[role] = kept
}

func setSeverity(d map[string]any, severity, key string, value any) {
	dict(dict(d, "severities"), severity)[key] = value
}

func deleteSeverityAction(d map[string]any, severity, action string) {
	severityMap := dict(dict(d, "severities"), severity)
	var kept []any
	for _, item := range asList(severityMap["actions"]) {
		if item != action {
			kept = append(kept, item)
		}
	}
	severityMap["actions"] = kept
}
