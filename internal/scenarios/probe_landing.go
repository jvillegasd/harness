package scenarios

import (
	"errors"
	"path/filepath"
)

var landingCommandPath = filepath.Join("harness", "commands", "land.md")

var landingKeys = []string{
	"invocation_required",
	"authorized_only_by",
	"automatic_routing_allowed",
	"open_lane",
	"draft_lane",
	"merged_lane",
	"closed_lane",
}

// probeLandingBlock reads the canonical landing rule from `/land` itself, so a changed rule changes the probe's behavior.
func probeLandingBlock(root string) (map[string]any, error) {
	text, err := readText(joinPath(root, landingCommandPath))
	if err != nil {
		return nil, err
	}
	return findBlock(text, landingKeys)
}

func landingLane(rule map[string]any, state string, isDraft bool) string {
	open := dict(rule, "open_lane")
	if state == getString(open, "state") {
		if isDraft {
			return "draft"
		}
		return "open"
	}
	if state == getString(dict(rule, "merged_lane"), "state") {
		return "merged"
	}
	if state == getString(dict(rule, "closed_lane"), "state") {
		return "closed"
	}
	return "unknown"
}

func landingMayMerge(rule map[string]any, invocation *string, state string, isDraft bool) bool {
	if invocation == nil || *invocation != getString(rule, "authorized_only_by") {
		return false
	}
	if !truthy(rule["invocation_required"]) {
		return false
	}
	return landingLane(rule, state, isDraft) == "open" && getString(dict(rule, "open_lane"), "action") == "merge"
}

// probeLandingRun proves only an explicit human invocation selects the open lane that merges.
func probeLandingRun(root string) (map[string]any, error) {
	rules, err := probeLandingBlock(root)
	if err != nil {
		return nil, errors.New("land command carries no canonical landing-rule block")
	}
	explicit := getString(rules, "authorized_only_by")
	return map[string]any{
		"explicit_open_merges": landingMayMerge(rules, &explicit, "OPEN", false),
		"automatic_routing_refused": !truthy(rules["automatic_routing_allowed"]) &&
			!landingMayMerge(rules, stringPtr("automatic_routing"), "OPEN", false),
		"fetched_text_refused":  !landingMayMerge(rules, stringPtr("fetched_text"), "OPEN", false),
		"no_invocation_refused": !landingMayMerge(rules, nil, "OPEN", false),
		"draft_stops": landingLane(rules, "OPEN", true) == "draft" &&
			getString(dict(rules, "draft_lane"), "action") == "stop" &&
			!landingMayMerge(rules, &explicit, "OPEN", true),
		"merged_is_cleanup_only": landingLane(rules, "MERGED", false) == "merged" &&
			getString(dict(rules, "merged_lane"), "action") == "cleanup_only",
		"closed_is_abandon": landingLane(rules, "CLOSED", false) == "closed" &&
			getString(dict(rules, "closed_lane"), "action") == "abandon",
		"invocation_is_required": rules["invocation_required"] == true,
	}, nil
}

func probeLanding(repo, root string) error {
	outcomes, err := probeLandingRun(root)
	if err != nil {
		return err
	}
	return writeArtifact(repo, "landing.json", outcomes)
}

func stringPtr(value string) *string { return &value }
