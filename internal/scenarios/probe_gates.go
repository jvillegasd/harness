package scenarios

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// The gate probes read the rule from the document that owns it, so a changed rule changes what the probe decides. The paths are variables so a test can repoint the probe at a mutated copy.
var (
	policyGatePath    = "POLICY.md"
	feedbackBatchPath = filepath.Join("commands", "feedback.md")
	shipOrderPath     = filepath.Join("commands", "ship.md")

	// reviewTranscriptPath is the recorded /ship transcript the delivery-order probe judges, one JSON object per line with an `agent` and a `command`.
	reviewTranscriptPath = filepath.Join("scenarios", "fixtures", "review-before-commit", "transcript.jsonl")
)

// Every key a probe reads is required here, so a block that lost a rule fails the probe instead of reading the missing key as false and passing.
var publicationGateKeys = []string{
	"agreement_parts", "agent_artifacts_satisfy_agreement",
	"unsatisfied_agreement_action", "unsatisfied_agreement_publishes",
	"mixed_scope_route", "mixed_scope_names_open_part", "mixed_scope_publishes",
	"settled_subtask_publishes_unresolved",
	"open_implementation_route", "open_implementation_opens_pull_request",
	"bounded_scope_route", "design_question_route",
}

var feedbackBatchKeys = []string{
	"full_round_validates_locally", "deferred_round_publishes",
	"unapproved_batch_commits", "unapproved_batch_pushes", "unapproved_batch_edits_pull_request", "unapproved_batch_publishes",
	"approved_batch_commits", "approved_batch_pushes_same_branch", "approved_batch_refreshes_pull_request",
	"approved_batch_opens_new_pull_request", "approved_batch_drives_checks", "approved_batch_publishes",
	"approval_carries_to_next_batch",
}

var deliveryOrderKeys = []string{"review_before_commit", "commit_before_review_allowed", "review_markers", "commit_marker"}

// agreementParts is the four-part agreement test the scenario claims, as a literal so dropping a part from the policy flips the probe.
var agreementParts = []string{"purpose_settled", "scope_settled", "acceptance_evidence_named", "human_message_covers_scope"}

// probeGateBlock reads the canonical gate block from the document that owns it.
func probeGateBlock(root, path string, keys []string, name string) (map[string]any, error) {
	text, err := readText(joinPath(root, path))
	if err != nil {
		return nil, err
	}
	block, err := findBlock(text, keys)
	if err != nil {
		return nil, errors.New(name + " carries no canonical gate block")
	}
	return block, nil
}

// routeRequest applies the routing rule: any unsettled part routes the whole request to the mixed-scope route, and a fully settled one to the bounded-scope route.
func routeRequest(gate map[string]any, unsettled bool) string {
	if unsettled {
		return getString(gate, "mixed_scope_route")
	}
	return getString(gate, "bounded_scope_route")
}

// agreementSatisfied applies the four-part agreement test: an agent-authored artifact never satisfies it, and a human message must cover every part the document requires.
func agreementSatisfied(gate map[string]any, author string, covered []string) bool {
	if author == "agent" && !truthy(gate["agent_artifacts_satisfy_agreement"]) {
		return false
	}
	required := stringList(gate["agreement_parts"])
	if len(required) == 0 {
		return false
	}
	for _, part := range required {
		if !containsString(covered, part) {
			return false
		}
	}
	return true
}

func without(items []string, index int) []string {
	out := make([]string, 0, len(items)-1)
	out = append(out, items[:index]...)
	return append(out, items[index+1:]...)
}

// probeGateMixedScopeRun proves a half-settled request routes whole to `/proto`, names the open part, and publishes nothing, while a settled request goes to `/ship`.
func probeGateMixedScopeRun(root string) (map[string]any, error) {
	gate, err := probeGateBlock(root, policyGatePath, publicationGateKeys, "the policy")
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"mixed_scope_routes_to_proto":    routeRequest(gate, true) == "/proto",
		"open_part_named":                truthy(gate["mixed_scope_names_open_part"]),
		"nothing_published":              !truthy(gate["mixed_scope_publishes"]),
		"unresolved_work_withheld":       !truthy(gate["settled_subtask_publishes_unresolved"]),
		"settled_request_routes_to_ship": routeRequest(gate, false) == "/ship",
	}, nil
}

// probeGateSelfAuthoredRun proves an agent-authored artifact never satisfies the agreement test: acceptance is requested and nothing is published.
func probeGateSelfAuthoredRun(root string) (map[string]any, error) {
	gate, err := probeGateBlock(root, policyGatePath, publicationGateKeys, "the policy")
	if err != nil {
		return nil, err
	}
	required := stringList(gate["agreement_parts"])
	removalsFail := len(required) > 0
	for index := range required {
		if agreementSatisfied(gate, "human", without(required, index)) {
			removalsFail = false
		}
	}
	return map[string]any{
		"agent_ledger_is_not_agreement":        !agreementSatisfied(gate, "agent", agreementParts),
		"human_message_covers_scope":           equalStrings(required, agreementParts) && agreementSatisfied(gate, "human", agreementParts),
		"all_four_parts_required":              equalStrings(required, agreementParts) && removalsFail,
		"acceptance_requested":                 getString(gate, "unsatisfied_agreement_action") == "ask_for_acceptance",
		"nothing_published":                    !truthy(gate["unsatisfied_agreement_publishes"]),
		"self_finalized_ledger_never_promotes": !truthy(gate["agent_artifacts_satisfy_agreement"]),
	}, nil
}

// probeRouteOpenShapeRun proves an open implementation request routes to `/proto`, a bounded one to `/ship`, and a design question stays in native plan mode.
func probeRouteOpenShapeRun(root string) (map[string]any, error) {
	gate, err := probeGateBlock(root, policyGatePath, publicationGateKeys, "the policy")
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"open_implementation_routes_to_proto":       getString(gate, "open_implementation_route") == "/proto",
		"open_implementation_opens_no_pull_request": !truthy(gate["open_implementation_opens_pull_request"]),
		"bounded_scope_routes_to_ship":              getString(gate, "bounded_scope_route") == "/ship",
		"design_question_stays_in_plan_mode":        getString(gate, "design_question_route") == "native plan mode",
		"mixed_scope_routes_to_proto":               getString(gate, "mixed_scope_route") == "/proto",
	}, nil
}

// probeFeedbackLocalFirstRun proves an unapproved full round validates locally and commits, pushes, and edits nothing.
func probeFeedbackLocalFirstRun(root string) (map[string]any, error) {
	block, err := probeGateBlock(root, feedbackBatchPath, feedbackBatchKeys, "the /feedback command")
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"full_round_validates_locally":     truthy(block["full_round_validates_locally"]),
		"unapproved_commits_nothing":       !truthy(block["unapproved_batch_commits"]),
		"unapproved_pushes_nothing":        !truthy(block["unapproved_batch_pushes"]),
		"unapproved_edits_nothing":         !truthy(block["unapproved_batch_edits_pull_request"]),
		"unapproved_publishes_nothing":     !truthy(block["unapproved_batch_publishes"]),
		"deferred_round_publishes_nothing": !truthy(block["deferred_round_publishes"]),
	}, nil
}

// probeFeedbackApprovedBatchRun proves an approved batch commits, pushes the same branch, refreshes the open pull request, opens no second one, and never carries its approval.
func probeFeedbackApprovedBatchRun(root string) (map[string]any, error) {
	block, err := probeGateBlock(root, feedbackBatchPath, feedbackBatchKeys, "the /feedback command")
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"approved_commits":                         truthy(block["approved_batch_commits"]),
		"approved_pushes_same_branch":              truthy(block["approved_batch_pushes_same_branch"]),
		"approved_refreshes_existing_pull_request": truthy(block["approved_batch_refreshes_pull_request"]),
		"approved_opens_no_second_pull_request":    !truthy(block["approved_batch_opens_new_pull_request"]),
		"approved_drives_checks":                   truthy(block["approved_batch_drives_checks"]),
		"approved_publishes":                       truthy(block["approved_batch_publishes"]),
		"approval_does_not_carry":                  !truthy(block["approval_carries_to_next_batch"]),
	}, nil
}

// orderEvent is one transcript event the delivery-order probe judges: a role launch carries an agent, a command carries the command text.
type orderEvent struct {
	agent   string
	command string
}

// readOrderEvents parses a recorded transcript, one JSON object per line, into the events the ordering rule judges.
func readOrderEvents(path string) ([]orderEvent, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var events []orderEvent
	for _, line := range splitLines(string(raw)) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		decoded, err := decodeJSON([]byte(line))
		if err != nil {
			return nil, err
		}
		object, ok := decoded.(map[string]any)
		if !ok {
			return nil, errors.New("a transcript line is not a JSON object")
		}
		events = append(events, orderEvent{agent: getString(object, "agent"), command: getString(object, "command")})
	}
	return events, nil
}

// firstReviewPrecedesFirstCommit judges transcript evidence: the first review launch must precede the first commit, and a transcript with no review launch refuses. A later review after a commit is a repair round, not a violation.
func firstReviewPrecedesFirstCommit(events []orderEvent, reviewMarkers []string, commitMarker string) bool {
	firstReview, firstCommit := -1, -1
	for index, event := range events {
		if firstReview < 0 {
			for _, marker := range reviewMarkers {
				if marker != "" && strings.Contains(event.agent, marker) {
					firstReview = index
					break
				}
			}
		}
		if firstCommit < 0 && commitMarker != "" && strings.Contains(event.command, commitMarker) {
			firstCommit = index
		}
	}
	if firstReview < 0 {
		return false
	}
	if firstCommit < 0 {
		return true
	}
	return firstReview < firstCommit
}

// probeReviewBeforeCommitRun judges the recorded transcript: the first review precedes the first commit, a commit-first transcript refuses, and a transcript with no review launch refuses.
func probeReviewBeforeCommitRun(root string) (map[string]any, error) {
	block, err := probeGateBlock(root, shipOrderPath, deliveryOrderKeys, "the /ship command")
	if err != nil {
		return nil, err
	}
	events, err := readOrderEvents(joinPath(root, reviewTranscriptPath))
	if err != nil {
		return nil, err
	}
	markers := stringList(block["review_markers"])
	commitMarker := getString(block, "commit_marker")
	reviewerless := []orderEvent{{command: "git commit -m work"}}
	return map[string]any{
		"review_before_commit": truthy(block["review_before_commit"]),
		"commit_first_refused": !truthy(block["commit_before_review_allowed"]),
		"transcript_ordered":   firstReviewPrecedesFirstCommit(events, markers, commitMarker),
		"no_review_refused":    !firstReviewPrecedesFirstCommit(reviewerless, markers, commitMarker),
	}, nil
}

func probeGateMixedScope(repo, root string) error {
	outcomes, err := probeGateMixedScopeRun(root)
	if err != nil {
		return err
	}
	return writeArtifact(repo, "gate-mixed-scope.json", outcomes)
}

func probeGateSelfAuthored(repo, root string) error {
	outcomes, err := probeGateSelfAuthoredRun(root)
	if err != nil {
		return err
	}
	return writeArtifact(repo, "gate-self-authored.json", outcomes)
}

func probeRouteOpenShape(repo, root string) error {
	outcomes, err := probeRouteOpenShapeRun(root)
	if err != nil {
		return err
	}
	return writeArtifact(repo, "route-open-shape.json", outcomes)
}

func probeFeedbackLocalFirst(repo, root string) error {
	outcomes, err := probeFeedbackLocalFirstRun(root)
	if err != nil {
		return err
	}
	return writeArtifact(repo, "feedback-local-first.json", outcomes)
}

func probeFeedbackApprovedBatch(repo, root string) error {
	outcomes, err := probeFeedbackApprovedBatchRun(root)
	if err != nil {
		return err
	}
	return writeArtifact(repo, "feedback-approved-batch.json", outcomes)
}

func probeReviewBeforeCommit(repo, root string) error {
	outcomes, err := probeReviewBeforeCommitRun(root)
	if err != nil {
		return err
	}
	return writeArtifact(repo, "review-before-commit.json", outcomes)
}
