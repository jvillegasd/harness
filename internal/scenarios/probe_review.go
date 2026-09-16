package scenarios

import (
	"errors"
	"path/filepath"
	"regexp"
	"strings"
)

var reviewSkillPath = filepath.Join("skills", "adversarial-review", "SKILL.md")
var reviewRolesPath = filepath.Join("skills", "adversarial-review", "roles")

var reviewTriageKeys = []string{"severities", "second_round"}
var reviewInputsKeys = []string{"inputs", "loaded_prefix"}

// returned is the verdicts a refuter can return; missing, malformed, and unavailable are what the parent derives when it returns none of them.
var returnedVerdicts = []string{"corroborated", "refuted", "inconclusive"}

const (
	reviewFinding      = "x.py:L3: bug: `y` is None when the cache misses. Guard it."
	reviewCorroborated = "verdict: corroborated\nx.py:L3: `y` is read before `load` fills the cache."
	reviewRefuted      = "verdict: refuted\nx.py:L2: `load` raises before `y` can be None."
	reviewInconclusive = "verdict: inconclusive\nChecked `load`; whether the cache can miss decides it."
	reviewUnreadable   = "unreadable: /tmp/round1.diff"
)

var reviewMalformed = []string{"looks fine", "verdict: refuted", "verdict: maybe\nx.py:L2: `load` raises."}

func probeReviewFind(root string, keys []string, name string) (map[string]any, error) {
	text, err := readText(joinPath(root, reviewSkillPath))
	if err != nil {
		return nil, err
	}
	block, err := findBlock(text, keys)
	if err != nil {
		return nil, errors.New("adversarial-review carries no canonical " + name + " block")
	}
	return block, nil
}

func probeReviewBlock(root string) (map[string]any, error) {
	return probeReviewFind(root, reviewTriageKeys, "triage")
}

func probeReviewInputs(root string) (map[string]any, error) {
	return probeReviewFind(root, reviewInputsKeys, "input")
}

var reviewEvidenceLine = regexp.MustCompile(`^\S+:L\d+: .+$`)

// adjudicate returns the verdict and path:Lline: evidence for one contested finding from its refuter results, where nil is a refuter that could not run.
func adjudicate(block, inputs map[string]any, results []*string) (string, []any, error) {
	rules := dict(block, "refuter")
	if len(results) == 0 {
		return "", nil, errors.New("a contested finding is rejected only after the refuter runs")
	}
	// One leading `unreadable:` answer is the Diff delivery retry, so only the answer after it counts as the invocation.
	prefix := getString(inputs, "unreadable_prefix")
	var retries []int
	for index, result := range results {
		if result != nil && strings.HasPrefix(*result, prefix) {
			retries = append(retries, index)
		}
	}
	retriesValid := len(retries) == 0 || (len(retries) == 1 && retries[0] == 0)
	if !retriesValid {
		return "", nil, errors.New("an unreadable answer is retried once inline, before the answer")
	}
	answered := results[len(retries):]
	if len(answered) > int(floatOf(rules["invocations_per_finding"])) {
		return "", nil, errors.New("one refuter invocation per contested finding")
	}
	if len(answered) == 0 || answered[len(answered)-1] == nil {
		return "unavailable", []any{}, nil
	}
	lines := splitLines(strings.TrimSpace(*answered[len(answered)-1]))
	if len(lines) == 0 {
		return "missing", []any{}, nil
	}
	verdictPrefix := getString(inputs, "verdict_prefix")
	verdict := ""
	if strings.HasPrefix(lines[0], verdictPrefix) {
		verdict = strings.TrimPrefix(lines[0], verdictPrefix)
	}
	evidence := []any{}
	for _, line := range lines[1:] {
		if reviewEvidenceLine.MatchString(line) {
			evidence = append(evidence, line)
		}
	}
	if !containsString(returnedVerdicts, verdict) ||
		(containsString(stringList(rules["evidence_required"]), verdict) && len(evidence) == 0) {
		return "malformed", []any{}, nil
	}
	return verdict, evidence, nil
}

type decideOptions struct {
	fixSmaller bool
	defect     *string
	refuter    []*string
	human      *string
}

// decide returns the triage decision for one finding under the block's rules.
func decide(block, inputs map[string]any, severity string, action *string, opts decideOptions) (map[string]any, error) {
	rules := block
	severities := dict(rules, "severities")
	actions := stringList(dict(severities, severity)["actions"])
	if containsString(actions, "reclassify_on_evidence") && opts.defect != nil && containsString(stringList(rules["reclassify_to"]), *opts.defect) {
		return decide(block, inputs, *opts.defect, action, decideOptions{refuter: opts.refuter, human: opts.human})
	}
	verdict := ""
	var evidence []any = []any{}
	closedBy := ""
	refuter := dict(rules, "refuter")
	switch {
	case containsString(actions, "answer"):
		action = stringPtr("answer")
	case containsString(actions, "drop"):
		if opts.fixSmaller && containsString(actions, "fix_when_smaller_than_argument") {
			action = stringPtr("fix")
		} else {
			action = stringPtr("drop")
		}
	case action == nil || !containsString(actions, *action):
		return nil, errors.New(severity + " does not permit " + deref(action))
	case containsString(stringList(refuter["applies_to"]), severity) && *action == getString(refuter, "required_before"):
		adjVerdict, adjEvidence, err := adjudicate(block, inputs, opts.refuter)
		if err != nil {
			return nil, err
		}
		verdict, evidence = adjVerdict, adjEvidence
		next := getString(dict(refuter, "verdicts"), verdict)
		action = &next
		if next == "escalate" && opts.human != nil {
			if getString(refuter, "escalation_closes_on") != "human_decision" || !containsString(actions, *opts.human) {
				return nil, errors.New("only a human decision the severity permits closes an escalated finding")
			}
			action, closedBy = opts.human, "human_decision"
		}
	}
	blocking := truthy(dict(severities, severity)["blocking"])
	trigger := getString(dict(rules, "second_round"), "trigger")
	var starts bool
	switch trigger {
	case "fixed_blocking_finding":
		starts = blocking && action != nil && *action == "fix"
	case "any_blocking_finding":
		starts = blocking
	default:
		return nil, errors.New("unknown second-round trigger: " + trigger)
	}
	var verdictValue any
	if verdict != "" {
		verdictValue = verdict
	}
	var closedValue any
	if closedBy != "" {
		closedValue = closedBy
	}
	var actionValue any
	if action != nil {
		actionValue = *action
	}
	return map[string]any{
		"severity":     severity,
		"action":       actionValue,
		"blocking":     blocking,
		"second_round": starts,
		"verdict":      verdictValue,
		"evidence":     evidence,
		"closed_by":    closedValue,
	}, nil
}

func deref(value *string) string {
	if value == nil {
		return "None"
	}
	return *value
}

// nextRound returns what follows a completed review round: none, the delta round's scope, or an action the cap names.
func nextRound(block map[string]any, completed int, decisions []map[string]any, newBlockingInRepaired, conformanceRepair bool, adjudications int) (string, error) {
	rules := block
	refuter := dict(rules, "refuter")
	if truthy(refuter["counts_as_round"]) {
		completed += adjudications
	}
	if completed >= int(floatOf(rules["max_rounds"])) {
		if newBlockingInRepaired {
			return getString(rules, "new_blocking_in_repaired_code"), nil
		}
		if conformanceRepair {
			return getString(rules, "conformance_repair_after_cap"), nil
		}
		return "none", nil
	}
	counts := conformanceRepair && getString(rules, "conformance_repair") == "counts_as_fixed_blocking_finding"
	anySecondRound := false
	for _, decision := range decisions {
		if truthy(decision["second_round"]) {
			anySecondRound = true
		}
	}
	if counts || anySecondRound {
		return getString(dict(rules, "second_round"), "scope"), nil
	}
	return "none", nil
}

// assemble returns exactly what the input block lets one reviewer receive: its listed inputs plus absolute standards paths.
func assemble(inputs map[string]any, role string, available, paths map[string]string) (map[string]any, error) {
	standardsMap := dict(inputs, "standards")
	roleStandards := stringList(standardsMap[role])
	optional := stringList(inputs["optional_standards"])
	var missing []string
	for _, name := range roleStandards {
		if _, ok := paths[name]; !ok {
			missing = append(missing, name)
		}
	}
	var required []string
	for _, name := range missing {
		if !containsString(optional, name) {
			required = append(required, name)
		}
	}
	if len(required) > 0 {
		return nil, errors.New("required standards missing: [" + strings.Join(required, " ") + "]")
	}
	standards := []string{}
	for _, name := range roleStandards {
		if path, ok := paths[name]; ok {
			standards = append(standards, path)
			if !filepath.IsAbs(path) {
				return nil, errors.New("standards paths must be absolute")
			}
		}
	}
	given := map[string]any{}
	for _, key := range stringList(dict(inputs, "inputs")[role]) {
		if value, ok := available[key]; ok {
			given[key] = value
		}
	}
	given["standards_paths"] = standards
	return given, nil
}

// loaded returns the supplied standards paths the first-line `loaded:` report names, opened or pasted; every other path counts as unread.
func loaded(inputs map[string]any, output string, supplied []string) []string {
	lines := splitLines(output)
	if len(lines) == 0 || !strings.HasPrefix(lines[0], getString(inputs, "loaded_prefix")) {
		return nil
	}
	prefix := getString(inputs, "loaded_prefix")
	inline := getString(inputs, "inline_prefix")
	named := map[string]bool{}
	for _, name := range strings.Split(lines[0][len(prefix):], ",") {
		name = strings.TrimSpace(name)
		name = strings.TrimPrefix(name, inline)
		named[name] = true
	}
	out := []string{}
	for _, path := range supplied {
		if named[path] {
			out = append(out, path)
		}
	}
	return out
}

func refused(call func() error) bool {
	return call() != nil
}

// probeReviewRun applies the triage, refuter, and input rules declared by adversarial-review.
func probeReviewRun(root string) (map[string]any, error) {
	block, err := probeReviewBlock(root)
	if err != nil {
		return nil, err
	}
	inputs, err := probeReviewInputs(root)
	if err != nil {
		return nil, err
	}
	nitFix, err := decide(block, inputs, "nit", nil, decideOptions{fixSmaller: true})
	if err != nil {
		return nil, err
	}
	nitDrop, err := decide(block, inputs, "nit", nil, decideOptions{})
	if err != nil {
		return nil, err
	}
	question, err := decide(block, inputs, "q", nil, decideOptions{})
	if err != nil {
		return nil, err
	}
	bugFix, err := decide(block, inputs, "bug", stringPtr("fix"), decideOptions{})
	if err != nil {
		return nil, err
	}
	riskFix, err := decide(block, inputs, "risk", stringPtr("fix"), decideOptions{})
	if err != nil {
		return nil, err
	}
	questionAsBug, err := decide(block, inputs, "q", stringPtr("fix"), decideOptions{defect: stringPtr("bug")})
	if err != nil {
		return nil, err
	}
	questionPlain, err := decide(block, inputs, "q", stringPtr("fix"), decideOptions{})
	if err != nil {
		return nil, err
	}
	rejectedRisk, err := decide(block, inputs, "risk", stringPtr("reject"), decideOptions{refuter: []*string{stringPtr(reviewRefuted)}})
	if err != nil {
		return nil, err
	}
	lateConformance, err := nextRound(block, 2, nil, false, true, 0)
	if err != nil {
		return nil, err
	}
	capTwo, err := nextRound(block, 2, []map[string]any{bugFix}, false, false, 0)
	if err != nil {
		return nil, err
	}
	roundTwoEscalate, err := nextRound(block, 2, []map[string]any{bugFix}, true, false, 0)
	if err != nil {
		return nil, err
	}
	conformanceDelta, err := nextRound(block, 1, nil, false, true, 0)
	if err != nil {
		return nil, err
	}
	rejectedNoRound, err := nextRound(block, 1, []map[string]any{rejectedRisk}, false, false, 0)
	if err != nil {
		return nil, err
	}
	outcomes := map[string]any{
		"nit_fixed_only_when_smaller": getString(nitFix, "action") == "fix" && getString(nitDrop, "action") == "drop",
		"nit_fix_starts_no_round":     !truthy(nitFix["blocking"]) && mustNextRound(block, 1, []map[string]any{nitFix}) == "none",
		"question_alone_never_blocks": !truthy(question["blocking"]) &&
			getString(question, "action") == "answer" &&
			mustNextRound(block, 1, []map[string]any{question}) == "none",
		"question_reclassified_on_evidence": deepEqual(questionAsBug, bugFix) && getString(questionPlain, "severity") == "q",
		"fixed_blocking_starts_delta_round": truthy(bugFix["blocking"]) && truthy(riskFix["blocking"]) &&
			mustNextRound(block, 1, []map[string]any{bugFix}) == "repair_delta" &&
			mustNextRound(block, 1, []map[string]any{riskFix}) == "repair_delta",
		"rejected_blocking_starts_no_round":     rejectedNoRound == "none",
		"round_two_new_blocking_escalates":      roundTwoEscalate == "escalate",
		"cap_holds_at_two_rounds":               capTwo == "none",
		"conformance_repair_starts_delta_round": conformanceDelta == "repair_delta",
		"late_conformance_repair_is_disclosed":  lateConformance == "recheck_and_disclose",
	}

	corroborated, err := decide(block, inputs, "bug", stringPtr("reject"), decideOptions{refuter: []*string{stringPtr(reviewCorroborated)}})
	if err != nil {
		return nil, err
	}
	refuted, err := decide(block, inputs, "risk", stringPtr("reject"), decideOptions{refuter: []*string{stringPtr(reviewRefuted)}})
	if err != nil {
		return nil, err
	}
	refutedLine := []string{strings.Split(reviewRefuted, "\n")[1]}
	refutedEvidence := stringList(refuted["evidence"])
	outcomes["corroborated_finding_is_fixed"] = getString(corroborated, "verdict") == "corroborated" &&
		getString(corroborated, "action") == "fix" &&
		mustNextRound(block, 1, []map[string]any{corroborated}) == "repair_delta"
	outcomes["refuted_finding_is_rejected_with_evidence"] = getString(refuted, "action") == "reject" &&
		equalStrings(refutedEvidence, refutedLine) &&
		mustNextRound(block, 1, []map[string]any{refuted}) == "none"
	outcomes["inconclusive_result_escalates"] = escalates(block, inputs, []*string{stringPtr(reviewInconclusive)}, "inconclusive")
	outcomes["missing_result_escalates"] = escalates(block, inputs, []*string{stringPtr("")}, "missing")
	malformedAll := true
	for _, text := range reviewMalformed {
		if !escalates(block, inputs, []*string{stringPtr(text)}, "malformed") {
			malformedAll = false
		}
	}
	outcomes["malformed_result_escalates"] = malformedAll
	outcomes["unavailable_refuter_escalates"] = escalates(block, inputs, []*string{nil}, "unavailable") &&
		escalates(block, inputs, []*string{stringPtr(reviewUnreadable)}, "unavailable")
	relaunch, relaunchErr := decide(block, inputs, "bug", stringPtr("reject"), decideOptions{refuter: []*string{stringPtr(reviewUnreadable), stringPtr(reviewRefuted)}})
	outcomes["unreadable_relaunch_is_the_same_invocation"] = relaunchErr == nil && getString(relaunch, "action") == "reject"
	outcomes["misplaced_unreadable_refused"] = refused(func() error {
		_, err := decide(block, inputs, "bug", stringPtr("reject"), decideOptions{refuter: []*string{stringPtr(reviewRefuted), stringPtr(reviewUnreadable)}})
		return err
	}) && refused(func() error {
		_, err := decide(block, inputs, "bug", stringPtr("reject"), decideOptions{refuter: []*string{stringPtr(reviewUnreadable), stringPtr(reviewUnreadable), stringPtr(reviewRefuted)}})
		return err
	})
	humanReject, humanErr := decide(block, inputs, "bug", stringPtr("reject"), decideOptions{refuter: []*string{stringPtr(reviewInconclusive)}, human: stringPtr("reject")})
	humanFix, humanFixErr := decide(block, inputs, "risk", stringPtr("reject"), decideOptions{refuter: []*string{nil}, human: stringPtr("fix")})
	outcomes["human_decision_closes_escalation"] = humanErr == nil && getString(humanReject, "closed_by") == "human_decision" &&
		humanFixErr == nil && getString(humanFix, "action") == "fix" &&
		escalates(block, inputs, []*string{stringPtr(reviewInconclusive)}, "inconclusive")
	outcomes["rejection_without_refutation_refused"] = refused(func() error {
		_, err := decide(block, inputs, "bug", stringPtr("reject"), decideOptions{})
		return err
	}) && refused(func() error {
		_, err := decide(block, inputs, "risk", stringPtr("reject"), decideOptions{refuter: []*string{}})
		return err
	})
	outcomes["one_refuter_invocation_per_finding"] = refused(func() error {
		_, err := decide(block, inputs, "bug", stringPtr("reject"), decideOptions{refuter: []*string{stringPtr(reviewInconclusive), stringPtr(reviewRefuted)}})
		return err
	})
	outcomes["refuter_consumes_no_round"] = mustNextRoundAdjudicated(block, 1, []map[string]any{corroborated}, 1) == "repair_delta" &&
		mustNextRoundAdjudicated(block, 2, []map[string]any{corroborated}, 1) == "none"

	available := map[string]string{
		"requirements":             "the acceptance criteria",
		"diff_snapshot_path":       "/tmp/round1.diff",
		"implementation_rationale": "why the implementer thought it was right",
		"ticket":                   "#1",
		"contested_finding":        reviewFinding,
	}
	paths := map[string]string{
		"ponytail":                 "/home/u/.claude/skills/ponytail/SKILL.md",
		"project_instruction_file": "/repo/AGENTS.md",
	}
	a, err := assemble(inputs, "reviewer-a", available, paths)
	if err != nil {
		return nil, err
	}
	b, err := assemble(inputs, "reviewer-b", available, paths)
	if err != nil {
		return nil, err
	}
	supplied := stringList(b["standards_paths"])
	finding := "x.py:L1: nit: `x` is unused. Delete it."
	outcomes["reviewer_a_boundary"] = deepEqual(a, map[string]any{
		"requirements":       available["requirements"],
		"diff_snapshot_path": available["diff_snapshot_path"],
		"standards_paths":    []string{paths["project_instruction_file"]},
	})
	outcomes["reviewer_b_boundary"] = deepEqual(b, map[string]any{
		"diff_snapshot_path": available["diff_snapshot_path"],
		"standards_paths":    []string{paths["ponytail"], paths["project_instruction_file"]},
	})
	outcomes["standards_paths_absolute"] = refused(func() error {
		_, err := assemble(inputs, "reviewer-a", available, map[string]string{"ponytail": paths["ponytail"], "project_instruction_file": "AGENTS.md"})
		return err
	})
	missingStandards, missingErr := assemble(inputs, "reviewer-a", available, map[string]string{"ponytail": paths["ponytail"]})
	outcomes["missing_standards_omitted"] = missingErr == nil && len(stringList(missingStandards["standards_paths"])) == 0
	outcomes["missing_ponytail_refused"] = refused(func() error {
		_, err := assemble(inputs, "reviewer-b", available, map[string]string{"project_instruction_file": paths["project_instruction_file"]})
		return err
	})
	outcomes["named_standards_count_as_loaded"] = equalStrings(loaded(inputs, "loaded: /repo/AGENTS.md\n"+finding, supplied), []string{"/repo/AGENTS.md"})
	outcomes["inline_standards_count_as_loaded"] = equalStrings(loaded(inputs, "loaded: /repo/AGENTS.md, inline:"+paths["ponytail"], supplied), supplied)
	outcomes["unnamed_standards_not_loaded"] = !containsString(loaded(inputs, "loaded: /repo/AGENTS.md", supplied), paths["ponytail"]) &&
		len(loaded(inputs, "loaded: none", supplied)) == 0
	outcomes["loaded_line_must_be_first"] = len(loaded(inputs, finding+"\nloaded: /repo/AGENTS.md", supplied)) == 0
	outcomes["missing_loaded_line_is_no_evidence"] = len(loaded(inputs, finding, supplied)) == 0
	refuterAssembly, refuterErr := assemble(inputs, "review-refuter", available, paths)
	outcomes["refuter_boundary"] = refuterErr == nil && deepEqual(refuterAssembly, map[string]any{
		"contested_finding":  reviewFinding,
		"diff_snapshot_path": available["diff_snapshot_path"],
		"standards_paths":    []string{},
	})
	rolesDir := joinPath(root, reviewRolesPath)
	refuterText, err := readText(filepath.Join(rolesDir, "review-refuter.md"))
	if err != nil {
		return nil, err
	}
	declaresVerdicts := true
	for _, verdict := range returnedVerdicts {
		if !strings.Contains(refuterText, "`"+getString(inputs, "verdict_prefix")+verdict+"`") {
			declaresVerdicts = false
		}
	}
	outcomes["refuter_declares_verdicts"] = declaresVerdicts
	declaresPrefix := true
	for _, name := range []string{"reviewer-a", "reviewer-b"} {
		text, err := readText(filepath.Join(rolesDir, name+".md"))
		if err != nil {
			return nil, err
		}
		if !strings.Contains(text, "`"+getString(inputs, "loaded_prefix")) || !strings.Contains(text, "`"+getString(inputs, "inline_prefix")) {
			declaresPrefix = false
		}
	}
	outcomes["roles_declare_loaded_prefix"] = declaresPrefix
	return outcomes, nil
}

func mustNextRound(block map[string]any, completed int, decisions []map[string]any) string {
	scope, err := nextRound(block, completed, decisions, false, false, 0)
	if err != nil {
		return "error"
	}
	return scope
}

func mustNextRoundAdjudicated(block map[string]any, completed int, decisions []map[string]any, adjudications int) string {
	scope, err := nextRound(block, completed, decisions, false, false, adjudications)
	if err != nil {
		return "error"
	}
	return scope
}

func escalates(block, inputs map[string]any, results []*string, verdict string) bool {
	decision, err := decide(block, inputs, "bug", stringPtr("reject"), decideOptions{refuter: results})
	if err != nil {
		return false
	}
	return getString(decision, "verdict") == verdict &&
		getString(decision, "action") == "escalate" &&
		!truthy(decision["second_round"])
}

func probeReviewTriage(repo, root string) error {
	outcomes, err := probeReviewRun(root)
	if err != nil {
		return err
	}
	return writeArtifact(repo, "review.json", outcomes)
}
