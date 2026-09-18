package scenarios

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// publication is a local bare origin seeded with one commit, which the scenario's repository pushes to instead of a forge.
type publication struct {
	origin, seed, refsBefore string
}

// sideEffects is what a publication attempt did to git and the forge, measured after the run rather than taken from the transcript.
type sideEffects struct {
	commits int
	pushed  bool
}

func git(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// seedPublication creates the bare origin, commits a seed to the repository, and pushes it, so commits and pushes are measured against a known baseline.
func seedPublication(workspace, repo string) (*publication, error) {
	origin := filepath.Join(workspace, "origin.git")
	steps := [][]string{
		{workspace, "init", "-q", "--bare", origin},
		{repo, "-c", "user.name=scenario", "-c", "user.email=scenario@invalid", "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "seed"},
		{repo, "remote", "add", "origin", origin},
		{repo, "push", "-q", "-u", "origin", "HEAD"},
	}
	for _, step := range steps {
		if _, err := git(step[0], step[1:]...); err != nil {
			return nil, err
		}
	}
	seed, err := git(repo, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	refs, err := git(origin, "for-each-ref")
	if err != nil {
		return nil, err
	}
	return &publication{origin: origin, seed: seed, refsBefore: refs}, nil
}

// measure counts commits made after the seed, in any ref or worktree, and reports whether any origin ref moved.
func (p *publication) measure(repo string) (*sideEffects, error) {
	count, err := git(repo, "rev-list", "--count", "--all", "^"+p.seed)
	if err != nil {
		return nil, err
	}
	commits, err := strconv.Atoi(count)
	if err != nil {
		return nil, err
	}
	refs, err := git(p.origin, "for-each-ref")
	if err != nil {
		return nil, err
	}
	return &sideEffects{commits: commits, pushed: refs != p.refsBefore}, nil
}

// foreignRemotes reports every remote URL that is not the seeded origin, so a publication attempt cannot point elsewhere.
func (p *publication) foreignRemotes(repo string) []string {
	out, _ := git(repo, "config", "--get-regexp", `^remote\..*\.(push)?url$`)
	var foreign []string
	for _, line := range strings.Split(out, "\n") {
		if key, url, ok := strings.Cut(line, " "); ok && url != p.origin {
			foreign = append(foreign, key+"="+url)
		}
	}
	return foreign
}

// forgeOp reports whether a logged `gh` call is the operation, matched on whole words so `pr create` does not match `pr created`.
func forgeOp(call, op string) bool {
	return call == op || strings.HasPrefix(call, op+" ")
}

// sideEffectEntries judges the declared side-effect assertions against what the attempt measurably did.
func sideEffectEntries(artifacts map[string]any, effects *sideEffects, forgeCalls []string) []map[string]any {
	var entries []map[string]any
	if value, ok := artifacts["commits"]; ok {
		entries = append(entries, entry("commits", effects.commits == intOf(value), fmt.Sprintf("%d commits", effects.commits)))
	}
	if value, ok := artifacts["push"]; ok {
		entries = append(entries, entry("push", effects.pushed == truthy(value), fmt.Sprintf("pushed: %t", effects.pushed)))
	}
	if value, ok := artifacts["pull_requests"]; ok {
		opened := 0
		for _, call := range forgeCalls {
			if forgeOp(call, "pr create") {
				opened++
			}
		}
		entries = append(entries, entry("pull_requests", opened == intOf(value), fmt.Sprintf("%d opened", opened)))
	}
	for _, raw := range asList(artifacts["forge_forbidden"]) {
		op := raw.(string)
		var hits []string
		for _, call := range forgeCalls {
			if forgeOp(call, op) {
				hits = append(hits, call)
			}
		}
		entries = append(entries, entry("forge_forbidden:"+op, len(hits) == 0, strings.Join(hits, "; ")))
	}
	if allowed, ok := artifacts["forge_allowed"]; ok {
		var outside []string
		for _, call := range forgeCalls {
			permitted := false
			for _, raw := range asList(allowed) {
				permitted = permitted || forgeOp(call, raw.(string))
			}
			if !permitted {
				outside = append(outside, call)
			}
		}
		entries = append(entries, entry("forge_allowed", len(outside) == 0, strings.Join(outside, "; ")))
	}
	return entries
}
