package scenarios

import (
	"os"
	"path/filepath"
	"testing"
)

// gateProbe runs a probe with its document path repointed at a mutated copy, and restores the original path.
func gateProbe(t *testing.T, target *string, rel, old, new string, run func(string) (map[string]any, error)) map[string]any {
	t.Helper()
	saved := *target
	t.Cleanup(func() { *target = saved })
	*target = writeTemp(t, patch(t, rel, old, new))
	outcomes, err := run(testRepoRoot())
	if err != nil {
		t.Fatal(err)
	}
	return outcomes
}

func requireProse(t *testing.T, rel string, phrases []string) {
	t.Helper()
	text := readRepoFile(t, rel)
	for _, phrase := range phrases {
		if !contains(text, phrase) {
			t.Errorf("%s lost the prose %q its block claims", rel, phrase)
		}
	}
}

func TestGateMixedScopeProbe(t *testing.T) {
	outcomes, err := probeGateMixedScopeRun(testRepoRoot())
	if err != nil {
		t.Fatal(err)
	}
	haveKeys(t, outcomes, []string{
		"mixed_scope_routes_to_proto", "open_part_named", "nothing_published",
		"unresolved_work_withheld", "settled_request_routes_to_ship",
	})
	allTrue(t, outcomes)
	requireProse(t, "POLICY.md", []string{
		"When part of a request is settled and part is not, route the whole request to `/proto`",
		"naming in one line which part is unsettled, and open no pull request from it",
		"A settled subtask never promotes the unresolved work travelling with it",
	})

	mutated := gateProbe(t, &policyGatePath, filepath.Join("POLICY.md"),
		`"mixed_scope_route": "/proto"`, `"mixed_scope_route": "/ship"`, probeGateMixedScopeRun)
	if mutated["mixed_scope_routes_to_proto"] != false {
		t.Fatal("the probe did not read the repointed mixed-scope route")
	}
}

func TestGateSelfAuthoredProbe(t *testing.T) {
	outcomes, err := probeGateSelfAuthoredRun(testRepoRoot())
	if err != nil {
		t.Fatal(err)
	}
	haveKeys(t, outcomes, []string{
		"agent_ledger_is_not_agreement", "human_message_covers_scope", "all_four_parts_required",
		"acceptance_requested", "nothing_published", "self_finalized_ledger_never_promotes",
	})
	allTrue(t, outcomes)
	requireProse(t, "POLICY.md", []string{
		"none of them supplies the agreement, alone or together, and an agent finalizing its own artifact never promotes it",
	})

	agentAuthored := gateProbe(t, &policyGatePath, filepath.Join("POLICY.md"),
		`"agent_artifacts_satisfy_agreement": false`, `"agent_artifacts_satisfy_agreement": true`, probeGateSelfAuthoredRun)
	if agentAuthored["agent_ledger_is_not_agreement"] != false || agentAuthored["self_finalized_ledger_never_promotes"] != false {
		t.Fatal("the probe did not read the repointed agent-authored rule")
	}

	action := gateProbe(t, &policyGatePath, filepath.Join("POLICY.md"),
		`"unsatisfied_agreement_action": "ask_for_acceptance"`, `"unsatisfied_agreement_action": "publish"`, probeGateSelfAuthoredRun)
	if action["acceptance_requested"] != false {
		t.Fatal("the probe did not read the repointed unsatisfied-agreement action")
	}

	// Dropping a middle part from the agreement test must fail every-part accounting, not only the last.
	part := gateProbe(t, &policyGatePath, filepath.Join("POLICY.md"),
		`"acceptance_evidence_named", `, ``, probeGateSelfAuthoredRun)
	if part["all_four_parts_required"] != false || part["human_message_covers_scope"] != false {
		t.Fatal("the probe did not require the named four agreement parts")
	}
}

func TestRouteOpenShapeProbe(t *testing.T) {
	outcomes, err := probeRouteOpenShapeRun(testRepoRoot())
	if err != nil {
		t.Fatal(err)
	}
	haveKeys(t, outcomes, []string{
		"open_implementation_routes_to_proto", "open_implementation_opens_no_pull_request",
		"bounded_scope_routes_to_ship", "design_question_stays_in_plan_mode", "mixed_scope_routes_to_proto",
	})
	allTrue(t, outcomes)
	requireProse(t, "POLICY.md", []string{
		`"implement this" with no approved contract and no bounded scope, a build request whose contract is still open | ` + "`/proto`",
		`"how should we do this", a design question with no build intent | native plan mode`,
	})

	mutated := gateProbe(t, &policyGatePath, filepath.Join("POLICY.md"),
		`"open_implementation_route": "/proto"`, `"open_implementation_route": "/ship"`, probeRouteOpenShapeRun)
	if mutated["open_implementation_routes_to_proto"] != false {
		t.Fatal("the probe did not read the repointed open-implementation route")
	}
}

func TestFeedbackLocalFirstProbe(t *testing.T) {
	outcomes, err := probeFeedbackLocalFirstRun(testRepoRoot())
	if err != nil {
		t.Fatal(err)
	}
	haveKeys(t, outcomes, []string{
		"full_round_validates_locally", "unapproved_commits_nothing", "unapproved_pushes_nothing",
		"unapproved_edits_nothing", "unapproved_publishes_nothing", "deferred_round_publishes_nothing",
	})
	allTrue(t, outcomes)
	requireProse(t, "commands/feedback.md", []string{
		"It validates locally first and publishes only an approved batch.",
		"No ladder, no review, no commit, no push.",
	})

	mutated := gateProbe(t, &feedbackBatchPath, filepath.Join("commands", "feedback.md"),
		`"unapproved_batch_publishes": false`, `"unapproved_batch_publishes": true`, probeFeedbackLocalFirstRun)
	if mutated["unapproved_publishes_nothing"] != false {
		t.Fatal("the probe did not read the repointed unapproved-batch rule")
	}
}

func TestFeedbackApprovedBatchProbe(t *testing.T) {
	outcomes, err := probeFeedbackApprovedBatchRun(testRepoRoot())
	if err != nil {
		t.Fatal(err)
	}
	haveKeys(t, outcomes, []string{
		"approved_commits", "approved_pushes_same_branch", "approved_refreshes_existing_pull_request",
		"approved_opens_no_second_pull_request", "approved_drives_checks", "approved_publishes", "approval_does_not_carry",
	})
	allTrue(t, outcomes)
	requireProse(t, "commands/feedback.md", []string{
		"On approval, make the fewest coherent Conventional Commits and push, with no further confirmation for this batch.",
		"an earlier batch's approval does not carry",
	})

	mutated := gateProbe(t, &feedbackBatchPath, filepath.Join("commands", "feedback.md"),
		`"approved_batch_opens_new_pull_request": false`, `"approved_batch_opens_new_pull_request": true`, probeFeedbackApprovedBatchRun)
	if mutated["approved_opens_no_second_pull_request"] != false {
		t.Fatal("the probe did not read the repointed approved-batch rule")
	}
}

// violatingTranscript writes a commit-first transcript and returns its path, so the delivery-order probe can be pointed at it.
func violatingTranscript(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	text := "{\"agent\": \"\", \"command\": \"git commit -m work\"}\n{\"agent\": \"reviewer-a\", \"command\": \"\"}\n"
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReviewBeforeCommitProbe(t *testing.T) {
	outcomes, err := probeReviewBeforeCommitRun(testRepoRoot())
	if err != nil {
		t.Fatal(err)
	}
	haveKeys(t, outcomes, []string{"review_before_commit", "commit_first_refused", "transcript_ordered", "no_review_refused"})
	allTrue(t, outcomes)
	requireProse(t, "commands/ship.md", []string{
		"Review the uncommitted diff before committing anything",
	})

	// The compliant recording orders the first review before the first commit; the violating one must flip.
	saved := reviewTranscriptPath
	t.Cleanup(func() { reviewTranscriptPath = saved })
	reviewTranscriptPath = violatingTranscript(t)
	violating, err := probeReviewBeforeCommitRun(testRepoRoot())
	if err != nil {
		t.Fatal(err)
	}
	if violating["transcript_ordered"] != false {
		t.Fatal("a commit-first transcript was not refused")
	}

	reviewTranscriptPath = saved
	mutated := gateProbe(t, &shipOrderPath, filepath.Join("commands", "ship.md"),
		`"review_before_commit": true`, `"review_before_commit": false`, probeReviewBeforeCommitRun)
	if mutated["review_before_commit"] != false {
		t.Fatal("the probe did not read the repointed review-before-commit rule")
	}

	markers := gateProbe(t, &shipOrderPath, filepath.Join("commands", "ship.md"),
		`"review_markers": ["reviewer-a", "reviewer-b", "review-refuter"]`, `"review_markers": []`, probeReviewBeforeCommitRun)
	if markers["transcript_ordered"] != false || markers["no_review_refused"] != true {
		t.Fatal("the probe did not derive the reviewer markers from the document")
	}
}
