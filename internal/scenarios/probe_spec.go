package scenarios

import (
	"errors"
	"path/filepath"
)

var slicingCommandPath = filepath.Join("harness", "commands", "create-ticket.md")
var slicingManifestPath = filepath.Join("docs", "agents", "harness.json")

var routeKeys = []string{
	"empty_spec_route",
	"single_slice_route",
	"multi_slice_route",
	"research_input_route",
	"projection_fields",
}

// probeSlicingBlock reads the canonical route table from `/create-ticket`, so a change to the table changes the probe's behavior.
func probeSlicingBlock(root string) (map[string]any, error) {
	text, err := readText(joinPath(root, slicingCommandPath))
	if err != nil {
		return nil, err
	}
	return findBlock(text, routeKeys)
}

// probeSlicingLabels reads the tracker label mapping from the repository manifest.
func probeSlicingLabels(root string) (map[string]any, error) {
	raw, err := readText(joinPath(root, slicingManifestPath))
	if err != nil {
		return nil, err
	}
	decoded, err := decodeJSON([]byte(raw))
	if err != nil {
		return nil, err
	}
	manifest, _ := decoded.(map[string]any)
	labels := dict(manifest, "labels")
	if labels == nil {
		return nil, errors.New("docs/agents/harness.json carries no usable label mapping")
	}
	for _, key := range []string{"blocked", "ready_for_agent", "in_progress"} {
		if _, ok := labels[key]; !ok {
			return nil, errors.New("docs/agents/harness.json carries no usable label mapping")
		}
	}
	return labels, nil
}

func slicingRoute(root string, slices []any, kind string) (string, error) {
	rules, err := probeSlicingBlock(root)
	if err != nil {
		return "", err
	}
	if kind == "research" {
		return getString(rules, "research_input_route"), nil
	}
	if len(slices) == 0 {
		return getString(rules, "empty_spec_route"), nil
	}
	if len(slices) == 1 {
		return getString(rules, "single_slice_route"), nil
	}
	return getString(rules, "multi_slice_route"), nil
}

type slicingPlan struct {
	route      string
	artifacts  []string
	parent     string
	children   []string
	edges      [][2]string
	frontier   []string
	labels     map[string]string
	projection map[string]bool
}

func buildSlicingPlan(root string, parent map[string]any, slices []any, kind, binding string) (slicingPlan, error) {
	mapped, err := probeSlicingLabels(root)
	if err != nil {
		return slicingPlan{}, err
	}
	rules, err := probeSlicingBlock(root)
	if err != nil {
		return slicingPlan{}, err
	}
	chosen, err := slicingRoute(root, slices, kind)
	if err != nil {
		return slicingPlan{}, err
	}
	address := getString(parent, "id")
	if binding != "github" {
		address = getString(parent, "path")
	}
	result := slicingPlan{route: chosen, labels: map[string]string{}, projection: map[string]bool{}}
	if chosen == "publish_nothing" {
		return result, nil
	}
	if chosen == "promote_spec_in_place" {
		blocked := false
		for _, raw := range slices {
			slice, _ := raw.(map[string]any)
			if truthy(slice["blocked_by"]) {
				blocked = true
			}
		}
		result.artifacts = []string{address}
		result.labels[address] = getString(mapped, "ready_for_agent")
		if blocked {
			result.labels[address] = getString(mapped, "blocked")
		}
		for _, field := range stringList(rules["projection_fields"]) {
			result.projection[field] = true
		}
		return result, nil
	}
	result.parent = address
	result.artifacts = append(result.artifacts, address)
	result.labels[address] = getString(mapped, "in_progress")
	for _, raw := range slices {
		slice, _ := raw.(map[string]any)
		id := getString(slice, "id")
		result.artifacts = append(result.artifacts, id)
		result.children = append(result.children, id)
		blockers := stringList(slice["blocked_by"])
		for _, blocker := range blockers {
			result.edges = append(result.edges, [2]string{id, blocker})
		}
		if len(blockers) == 0 {
			result.frontier = append(result.frontier, id)
			result.labels[id] = getString(mapped, "ready_for_agent")
		} else {
			result.labels[id] = getString(mapped, "blocked")
		}
	}
	return result, nil
}

func slice(id string, blockers ...string) map[string]any {
	blocked := []any{}
	for _, blocker := range blockers {
		blocked = append(blocked, blocker)
	}
	return map[string]any{"id": id, "blocked_by": blocked}
}

// probeSlicingRun applies the route table to zero-, one-, and many-slice spec fixtures.
func probeSlicingRun(root string) (map[string]any, error) {
	mapped, err := probeSlicingLabels(root)
	if err != nil {
		return nil, err
	}
	spec := map[string]any{"id": "issue-74", "path": "docs/specs/continuity/spec.md"}
	researchParent := map[string]any{"id": "issue-70", "path": "docs/specs/research/spec.md"}
	empty := []any{}
	one := []any{slice("issue-74")}
	many := []any{slice("issue-101"), slice("issue-102", "issue-101"), slice("issue-103")}
	research := []any{slice("issue-104")}

	zeroPlan, err := buildSlicingPlan(root, spec, empty, "spec", "github")
	if err != nil {
		return nil, err
	}
	onePlan, err := buildSlicingPlan(root, spec, one, "spec", "github")
	if err != nil {
		return nil, err
	}
	manyPlan, err := buildSlicingPlan(root, spec, many, "spec", "github")
	if err != nil {
		return nil, err
	}
	researchPlan, err := buildSlicingPlan(root, researchParent, research, "research", "github")
	if err != nil {
		return nil, err
	}
	localPlan, err := buildSlicingPlan(root, spec, one, "spec", "local-markdown")
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"zero_slice_publishes_nothing":   len(zeroPlan.artifacts) == 0,
		"one_slice_creates_no_child":     len(onePlan.children) == 0 && len(onePlan.artifacts) == 1 && onePlan.artifacts[0] == "issue-74",
		"one_slice_implementation_label": onePlan.labels["issue-74"] == getString(mapped, "ready_for_agent"),
		"multi_slice_preserves_parent":   manyPlan.parent == "issue-74",
		"multi_slice_preserves_children": equalStrings(manyPlan.children, []string{"issue-101", "issue-102", "issue-103"}),
		"multi_slice_preserves_edges":    len(manyPlan.edges) == 1 && manyPlan.edges[0] == [2]string{"issue-102", "issue-101"},
		"multi_slice_preserves_frontier": equalStrings(manyPlan.frontier, []string{"issue-101", "issue-103"}),
		"research_batch_uses_child_dag": researchPlan.parent == "issue-70" &&
			len(researchPlan.artifacts) > 0 && researchPlan.artifacts[0] == "issue-70" &&
			equalStrings(researchPlan.children, []string{"issue-104"}),
		"single_slice_binding_is_one_artifact": len(localPlan.artifacts) == 1 &&
			localPlan.artifacts[0] == "docs/specs/continuity/spec.md" &&
			len(localPlan.children) == 0 &&
			localPlan.labels["docs/specs/continuity/spec.md"] == getString(mapped, "ready_for_agent"),
	}, nil
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func probeSpecSlicing(repo, root string) error {
	outcomes, err := probeSlicingRun(root)
	if err != nil {
		return err
	}
	return writeArtifact(repo, "slicing.json", outcomes)
}
