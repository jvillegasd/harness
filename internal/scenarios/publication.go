package scenarios

import (
	"fmt"
	"os"
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

// remoteHelper serves the `scenario` protocol. It ignores the URL and always connects to the seeded origin, so a session allowed only this protocol can publish nowhere else: every other transport, local paths included, stays blocked.
const remoteHelper = `#!/bin/sh
read _
printf 'connect\n\n'
read _ service
echo
exec git "${service#git-}" "%s"
`

// seedPublication creates the bare origin, commits a seed to the repository, pushes it, and points origin at the scenario protocol, so commits and pushes are measured against a known baseline. The helper lands in the shim directory the forge guard puts on PATH.
func seedPublication(workspace, repo string) (*publication, error) {
	origin := filepath.Join(workspace, "origin.git")
	shim := filepath.Join(workspace, "shim")
	if err := os.MkdirAll(shim, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(shim, "git-remote-scenario"), []byte(fmt.Sprintf(remoteHelper, origin)), 0o755); err != nil {
		return nil, err
	}
	steps := [][]string{
		{workspace, "init", "-q", "--bare", origin},
		{repo, "-c", "user.name=scenario", "-c", "user.email=scenario@invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "-q", "--allow-empty", "-m", "seed"},
		{repo, "remote", "add", "origin", origin},
		{repo, "-c", "core.hooksPath=/dev/null", "push", "-q", "-u", "origin", "HEAD"},
		{repo, "remote", "set-url", "origin", "scenario::origin"},
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

// measure counts commits made after the seed, in any ref or worktree except the stash, and reports whether any origin ref moved.
func (p *publication) measure(repo string) (*sideEffects, error) {
	count, err := git(repo, "rev-list", "--count", "--exclude=refs/stash", "--all", "^"+p.seed)
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
