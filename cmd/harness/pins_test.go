package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func readRepo(t *testing.T, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

var jsonBlock = regexp.MustCompile("(?s)```json\n(.*?)```")

func jsonBlocksOf(text string) []map[string]any {
	var blocks []map[string]any
	for _, match := range jsonBlock.FindAllStringSubmatch(text, -1) {
		var object map[string]any
		if err := json.Unmarshal([]byte(match[1]), &object); err != nil {
			continue
		}
		blocks = append(blocks, object)
	}
	return blocks
}

// --- workflow-checkpoint skill: canonical block, no duplicated schema, and command wiring ---

func checkpointBlock(t *testing.T) map[string]any {
	t.Helper()
	for _, block := range jsonBlocksOf(readRepo(t, "skills/workflow-checkpoint/SKILL.md")) {
		if _, ok := block["fields"]; ok {
			if _, ok := block["states"]; ok {
				return block
			}
		}
	}
	t.Fatal("workflow-checkpoint carries no canonical protocol block")
	return nil
}

func TestCheckpointProtocolBlock(t *testing.T) {
	block := checkpointBlock(t)
	fields := block["fields"].(map[string]any)
	want := map[string]bool{"state": true, "branch_tip": true, "contract": true, "checks": true, "rationale": true, "handoff": true, "next_action": true}
	if len(fields) != len(want) {
		t.Fatalf("fields = %v", fields)
	}
	for field := range want {
		if _, ok := fields[field]; !ok {
			t.Errorf("missing field %s", field)
		}
	}
	if fields["contract"] != "array" || fields["checks"] != "array" {
		t.Errorf("contract and checks must be arrays: %v", fields)
	}
	assertStrings(t, block["states"], []string{"active", "completed", "abandoned", "cancelled"})
	assertStrings(t, block["terminal_states"], []string{"completed", "abandoned", "cancelled"})
	if block["active_state"] != "active" || block["topic_key"] != "checkpoint/<branch>" {
		t.Fatalf("active_state or topic_key changed: %v", block)
	}
	// The record table lists exactly the canonical fields.
	rows := map[string]bool{}
	for _, line := range strings.Split(readRepo(t, "skills/workflow-checkpoint/SKILL.md"), "\n") {
		if strings.HasPrefix(line, "| `") {
			if end := strings.Index(line[3:], "`"); end >= 0 {
				rows[line[3:3+end]] = true
			}
		}
	}
	if len(rows) != len(fields) {
		t.Fatalf("record table rows = %v, fields = %v", rows, fields)
	}
	for field := range fields {
		if !rows[field] {
			t.Errorf("record table does not list %s", field)
		}
	}
}

// duplicatesProtocol is true when a text restates the protocol block or the record schema.
func duplicatesProtocol(text string, block map[string]any) bool {
	if strings.Contains(text, "stale_invalidates") || strings.Contains(text, "require_non_empty") {
		return true
	}
	for _, candidate := range jsonBlocksOf(text) {
		keys := sortedStringKeys(candidate)
		if equalStringSlices(keys, sortedStringKeys(block)) || equalStringSlices(keys, sortedStringKeys(block["fields"].(map[string]any))) {
			return true
		}
	}
	return false
}

func sortedStringKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func TestCommandsDoNotDuplicateTheCheckpointSchema(t *testing.T) {
	block := checkpointBlock(t)
	commands, err := filepath.Glob(filepath.Join(repoRoot(t), "commands", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range commands {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if duplicatesProtocol(string(raw), block) {
			t.Errorf("%s restates the checkpoint schema", filepath.Base(path))
		}
	}
}

func TestDeliveryCommandsWireTheCheckpoint(t *testing.T) {
	for _, name := range []string{"proto", "ship", "feedback", "address-review", "land", "abandon"} {
		text := strings.ToLower(readRepo(t, "commands/"+name+".md"))
		if !strings.Contains(text, "read `workflow-checkpoint`") {
			t.Errorf("%s.md does not read the checkpoint skill at its boundary", name)
		}
		if !strings.Contains(text, "retrieve") {
			t.Errorf("%s.md does not retrieve the checkpoint", name)
		}
	}
	for name, boundary := range map[string]string{
		"proto":          "save the branch checkpoint",
		"ship":           "save the branch checkpoint",
		"feedback":       "refresh the branch checkpoint",
		"address-review": "refresh the branch checkpoint",
		"land":           "finalize the checkpoint as `completed`",
		"abandon":        "finalize the checkpoint as",
	} {
		if !strings.Contains(strings.ToLower(readRepo(t, "commands/"+name+".md")), boundary) {
			t.Errorf("%s.md does not save or finalize the checkpoint at its return", name)
		}
	}
	if !strings.Contains(readRepo(t, "commands/land.md"), "`completed`") {
		t.Error("land.md does not name its terminal state")
	}
	abandon := readRepo(t, "commands/abandon.md")
	if !strings.Contains(abandon, "`abandoned`") || !strings.Contains(abandon, "`cancelled`") {
		t.Error("abandon.md does not name its terminal states")
	}
}

// --- recon citations: one canonical section, pinned lines, steps that point at it ---

func reconSection(t *testing.T, rel string) string {
	t.Helper()
	text := readRepo(t, rel)
	start := strings.Index(text, "## Recon citations\n")
	if start < 0 {
		return ""
	}
	rest := text[start+len("## Recon citations\n"):]
	if end := strings.Index(rest, "\n## "); end >= 0 {
		return rest[:end]
	}
	return rest
}

func TestReconCommandsCarryTheSameSection(t *testing.T) {
	triage, research := reconSection(t, "commands/triage.md"), reconSection(t, "commands/research.md")
	if triage == "" || research == "" {
		t.Fatal("a command carries no recon citations section")
	}
	if triage != research {
		t.Fatal("triage.md and research.md carry different recon citation sections")
	}
	pins := []string{
		"A recon worker that searches the repository ends on a text message holding only its findings, one per line in the shape `path:line | symbol | mechanism`: the cited location, the named symbol, a bare identifier, that location carries, and the mechanism it is claimed to show. A trailing tool result is not the report: re-run that track once, and if it again ends without one, record it as no data, a `no data` row in `/triage`'s evidence ledger or an entry under `/research`'s `Contradictions and unknowns`, never as refuted or as a track that found nothing.",
		"Before reusing a citation, read only the cited line and confirm the path resolves inside the repository and that line carries the named symbol as a whole identifier. That check grounds the location only, never the claimed runtime mechanism: a load-bearing mechanism claim still goes through `evidence`.",
		"Drop a citation whose path is missing or outside the repository, whose line does not carry the named symbol, or that lacks any of the three fields. Record it as refuted, as a verdict row in `/triage`'s evidence ledger or an entry under `/research`'s `Contradictions and unknowns`, and never reuse it as grounded evidence.",
	}
	present := strings.Split(triage, "\n")
	for _, pin := range pins {
		found := false
		for _, line := range present {
			if line == pin {
				found = true
			}
		}
		if !found {
			t.Errorf("the recon section lost pinned line %q", pin[:60])
		}
	}
	for _, name := range []string{"triage", "research"} {
		text := readRepo(t, "commands/"+name+".md")
		steps := strings.SplitN(text, "## Steps\n", 2)
		if len(steps) < 2 {
			t.Fatalf("%s.md has no Steps section", name)
		}
		steps = strings.SplitN(steps[1], "\n## ", 2)
		if !strings.Contains(steps[0], "`## Recon citations`") {
			t.Errorf("%s.md delegation steps do not point at the recon citations section", name)
		}
	}
}

// --- review triage prose derived from the canonical block ---

func TestReviewProseAgreesWithTheBlock(t *testing.T) {
	skill := readRepo(t, "skills/adversarial-review/SKILL.md")
	var triage map[string]any
	for _, block := range jsonBlocksOf(skill) {
		if _, ok := block["severities"]; ok {
			triage = block
		}
	}
	if triage == nil {
		t.Fatal("skill carries no triage block")
	}
	tick := func(names []string) []string {
		var out []string
		for _, name := range names {
			out = append(out, "`"+name+":`")
		}
		return out
	}
	severities := triage["severities"].(map[string]any)
	var blocking []string
	for _, name := range []string{"bug", "risk", "nit", "q"} {
		if severities[name].(map[string]any)["blocking"] == true {
			blocking = append(blocking, name)
		}
	}
	word := map[float64]string{1: "one", 2: "two", 3: "three"}[triage["max_rounds"].(float64)]
	if word == "" {
		t.Fatal("the skill has no prose for this round cap")
	}
	reclassify := stringSlice(triage["reclassify_to"])
	sentences := []string{"Only " + strings.Join(tick(blocking), " and ") + " block."}
	sentences = append(sentences, "It becomes a "+strings.Join(tick(reclassify), " or ")+" only when evidence exposes a defect")
	nitActions := stringSlice(severities["nit"].(map[string]any)["actions"])
	if containsString(nitActions, "fix_when_smaller_than_argument") {
		sentences = append(sentences, "A `nit:` is fixed only when the fix is smaller than the argument, and otherwise dropped.")
	}
	secondRound := triage["second_round"].(map[string]any)
	if secondRound["trigger"] == "fixed_blocking_finding" && secondRound["scope"] == "repair_delta" {
		sentences = append(sentences, "A second round runs only after a fixed blocking finding, and it reviews the repair delta alone.")
	}
	if triage["conformance_repair"] == "counts_as_fixed_blocking_finding" {
		sentences = append(sentences, "A conformance repair counts as a fixed blocking finding for the second-round trigger.")
	}
	if triage["conformance_repair_after_cap"] == "recheck_and_disclose" {
		sentences = append(sentences, "After round two, a conformance repair is rechecked by the conformance pass alone and disclosed under `## Checks`, never given a third round.")
	}
	sentences = append(sentences, "At most "+word+" rounds.")
	refuter := triage["refuter"].(map[string]any)
	if refuter["required_before"] == "reject" && refuter["invocations_per_finding"].(float64) == 1 {
		sentences = append(sentences, "Before rejecting a contested "+strings.Join(tick(stringSlice(refuter["applies_to"])), " or ")+", launch the named `review-refuter` agent once for that finding")
	}
	if refuter["counts_as_round"] == false {
		sentences = append(sentences, "The refuter is not a reviewer and not a round")
	}
	if refuter["escalation_closes_on"] == "human_decision" {
		sentences = append(sentences, "A human's decision on an escalated finding closes it")
	}
	verdicts := refuter["verdicts"].(map[string]any)
	for _, action := range []string{"fix", "reject", "escalate"} {
		var names []string
		for _, verdict := range []string{"corroborated", "refuted", "inconclusive", "missing", "malformed", "unavailable"} {
			if verdicts[verdict] == action {
				names = append(names, "`"+verdict+"`")
			}
		}
		if len(names) == 0 {
			continue
		}
		phrase := map[string]string{"fix": "fix it", "reject": "reject it", "escalate": "escalate"}[action]
		joined := names[0]
		if len(names) > 1 {
			joined = strings.Join(names[:len(names)-1], ", ") + ", or " + names[len(names)-1]
		}
		sentences = append(sentences, "- "+joined+": "+phrase)
	}
	for _, sentence := range sentences {
		if !strings.Contains(skill, sentence) {
			t.Errorf("skill is missing prose sentence %q", sentence)
		}
	}
}

// --- role agents: standing instructions, orchestration boundaries, pinned lines ---

func TestRolesOwnTheirStandingInstructions(t *testing.T) {
	a := readRepo(t, "skills/adversarial-review/roles/reviewer-a.md")
	b := readRepo(t, "skills/adversarial-review/roles/reviewer-b.md")
	if !strings.Contains(a, "\nAssume this diff fails to do what the requirements say. Find where.\n") {
		t.Error("reviewer-a lost its lens")
	}
	if !strings.Contains(b, "\nAssume this diff is wrong. Find the bug.\n") {
		t.Error("reviewer-b lost its lens")
	}
	for _, text := range []string{a, b} {
		for _, heading := range []string{"## Inputs", "## Access", "## Lenses", "## Output"} {
			if !strings.Contains(text, "\n"+heading+"\n") {
				t.Errorf("a role is missing %s", heading)
			}
		}
	}
	if strings.SplitN(a, "## Output\n", 2)[1] != strings.SplitN(b, "## Output\n", 2)[1] {
		t.Error("reviewer-a and reviewer-b no longer share one output format")
	}
}

func TestSkillKeepsOrchestrationWithoutTheRoleText(t *testing.T) {
	skill := readRepo(t, "skills/adversarial-review/SKILL.md")
	for _, heading := range []string{"## Invocation", "## Diff delivery", "## Triage, by the implementer", "## Loop cap"} {
		if !strings.Contains(skill, "\n"+heading+"\n") {
			t.Errorf("the skill lost %s", heading)
		}
	}
	for _, phrase := range []string{"An unmet requirement", "Callers of every touched symbol", "a check that would still pass if the logic it covers were deleted", "dead code and leftovers", "Severity is one of"} {
		if strings.Contains(skill, phrase) {
			t.Errorf("the skill absorbed role text: %q", phrase)
		}
	}
}

func TestPinnedReviewProtocolLines(t *testing.T) {
	unreadable := "When you cannot read the whole diff file, return exactly `unreadable: <path>` and nothing else, so the parent can supply the same diff inline. Never review part of a diff."
	loaded := "Begin with one line naming every standards file you actually opened with a tool during this review, as `loaded: <absolute path>, <absolute path>`, or `loaded: none`. Name a standard whose full text the parent pasted into your prompt as `inline:<absolute path>`. A path you were given but did not open, or a standard you recall without opening, does not belong on it."
	deferLine := "Triage every finding by `adversarial-review`'s canonical triage block: only `bug:` and `risk:` block, a `nit:` is fixed only when the fix is smaller than the argument and is otherwise dropped, a `q:` is answered and blocks only once evidence reclassifies it, a contested `bug:` or `risk:` is rejected only on the refuter's `refuted` verdict or a human decision on its escalation, and a second adversarial round runs only after a fixed blocking finding, over the repair delta alone."
	access := "Read and search only. Read and search the repository to follow callers and context. Never edit a file, run a command, fetch from the web, or delegate to another agent, even where the client would let you."
	pins := map[string][]string{
		"POLICY.md": {
			"Use ephemeral specialist roles. Do not create persistent client-specific personas when a command, a skill, or an isolated review context is enough. Role definitions generated from one canonical source, such as the reviewer agents, are shared instructions and client capability configuration, not personas.",
			"Every delegated role runs through Orca, which owns the worker's placement, supervision, messaging, retries, and lifecycle; the harness contributes only the named role's launch and the reading of its result. A client-native subagent and a headless client run are not alternative backends for a role, whatever the role and whichever client the session itself runs.",
			"When a workflow chooses to delegate implementation, launch the named `executor` agent, whose model and effort come from the harness's central role assignments. Delegation stays optional and changes nothing else: the delegating workflow keeps the approved scope, the commits, the review passes, the conformance check, and every gate. When Orca or the named agent is unavailable, say so and do the work inline rather than hand it to an agent without that assignment. A required review is not inlined that way: it is run once the runtime is back, or escalated.",
			"Launch a named role agent without a model or effort override, so it runs on its central assignment.",
			"Before rejecting a contested `bug:` or `risk:`, every consumer launches `review-refuter` once with only that finding and the diff snapshot it was raised against: `corroborated` is fixed, `refuted` is rejected with the refuter's evidence recorded, and an inconclusive, missing, malformed, or unavailable result escalates with neither a dismissal nor a speculative fix until a human decision closes it, recorded where a rejection would be. The refuter adds no reviewer and no round.",
		},
		"skills/adversarial-review/SKILL.md": {
			"Launch each reviewer as its named agent, in parallel, each with a fresh context: `reviewer-a` and `reviewer-b`. The installer generates them from `roles/reviewer-a.md` and `roles/reviewer-b.md` beside this skill, which own each reviewer's standing instructions, lenses, and output format.",
			"Launch every role, reviewers, the refuter, and a delegated `executor` alike, by one rule: write the launch prompt to a file outside the worktree and run `harness dispatch <role> --prompt-file <path> --worktree <path>`, in the background or with the client's longest tool timeout, since a run can take up to an hour. Its stdout is the role's output, produced by the role's own agent on its assigned client, which `harness models resolve <role> --json` names. Orca is the only runtime, so a role assigned to this session's own client takes that same command; never launch a role as a client-native agent instead.",
			"- `agent_missing` or `stale_install`: run `harness install`, then dispatch again. Never substitute a general agent for the named one.",
			"- `orca_unavailable`: the runtime cannot launch anything. Follow the recovery the reason names and dispatch again. Where it cannot be followed, an `executor` slice may be done inline, as the policy says, after inspecting `git status` in the worktree and escalating rather than redoing when a failed run left changes; a required review may not. Escalate instead.",
			"Never skip, silently omit, or quietly run inline a review the blast radius requires. Disclose the dispatch reason under the pull request's `## Checks`, or in the requirements ledger under `/proto`.",
			"Give Reviewer A the requirements, the diff snapshot path, and the exact absolute path of the project's own instruction file as its standards, never the implementation rationale. Nothing else.",
			"Give Reviewer B only the diff snapshot path and the exact absolute paths of its review standards: the loaded `ponytail` body and the project's own instruction file. No requirements, no rationale, no ticket.",
			"Standards are paths, never requirements or rationale, so supplying them leaves each reviewer's information boundary where it was. Count a standards file as read only when the reviewer's `loaded:` line names it: supplying a path, or a reviewer recalling what a standard says, is not evidence that it was read.",
			"The project's own instruction file is `AGENTS.md` at the repository root, or `CLAUDE.md` there when no `AGENTS.md` exists, and is left out when neither does. Where the client is known to refuse a standards path, such as OpenCode for a file outside the project, paste that file's text into the first launch instead of its path. When a supplied standards file is missing from a reviewer's `loaded:` line, relaunch that reviewer once with that file's text pasted in; the relaunch replaces that reviewer's output for the same round and never counts toward the cap. A reviewer names a pasted standard as `inline:<absolute path>`. If a standard is still unread, disclose it under the pull request's `## Checks`, or in the requirements ledger under `/proto`.",
			"Before rejecting a contested `bug:` or `risk:`, launch the named `review-refuter` agent once for that finding, with a fresh context. The installer generates it from `roles/review-refuter.md` beside this skill, and it runs on its central assignment, dispatched by the same one rule. A refuter that cannot be launched is the `unavailable` verdict, which escalates.",
			"Give the refuter only the contested finding, verbatim, and the path of the diff snapshot that finding was raised against. Never the implementation rationale, the requirements, your reason for contesting it, or the conversation. A refuter that answers `unreadable: <path>` gets that snapshot inline under Diff delivery, and the relaunch completes the same invocation rather than adding one.",
			"Snapshot the complete diff once per round into a readable temporary file outside the worktree, and give each reviewer that path. A reviewer that answers `unreadable: <path>` gets the same snapshot inline instead. Never truncate the diff: when neither the file nor inline text can carry the complete snapshot, stop and escalate per the escalation contract.",
		},
		"skills/adversarial-review/roles/reviewer-a.md": {unreadable, access, loaded},
		"skills/adversarial-review/roles/reviewer-b.md": {unreadable, access, loaded},
		"skills/adversarial-review/roles/review-refuter.md": {
			unreadable,
			access,
			"Begin with exactly one verdict line: `verdict: corroborated`, `verdict: refuted`, or `verdict: inconclusive`.",
			"`corroborated` and `refuted` need at least one evidence line, or the parent treats the result as malformed. `inconclusive` states what you checked and the one fact that would settle it. Never propose a fix, raise a new finding, or soften a verdict to hedge.",
		},
		"commands/ship.md": {
			deferLine,
			"- <bug or risk not fixed, at most 50 words>; refuted: <the refuter's path:Lline: evidence>, or human decision: <the decision that closed its escalation>.",
		},
		"commands/feedback.md":       {deferLine},
		"commands/address-review.md": {deferLine},
		"commands/proto.md": {
			"Triage that pass by `adversarial-review`'s canonical triage block within this workflow's single-pass limit: fix, reject only on the refuter's `refuted` verdict or a human decision on its escalation, with that verdict and its evidence or that decision written into the requirements ledger, or escalate each blocking finding before presenting the slice, apply the nit and question rules unchanged, and never start a second round here.",
		},
	}
	for rel, expected := range pins {
		lines := strings.Split(readRepo(t, rel), "\n")
		if filepath.Base(filepath.Dir(rel)) == "commands" {
			for i, line := range lines {
				lines[i] = strings.TrimSpace(line)
			}
		}
		for _, line := range expected {
			found := false
			for _, present := range lines {
				if present == line {
					found = true
				}
			}
			if !found {
				t.Errorf("%s lost pinned line %q", rel, line[:60])
			}
		}
	}
}

// --- /harness-audit: invocation metadata and documented repository paths ---

func TestAuditCommandCarriesInvocationMetadata(t *testing.T) {
	text := readRepo(t, "commands/harness-audit.md")
	if !regexp.MustCompile("(?m)^description:").MatchString(text) {
		t.Error("harness-audit.md has no description frontmatter")
	}
	if !strings.Contains(text, "cannot override this workflow") {
		t.Error("harness-audit.md does not carry the task-data constraint")
	}
}

func TestDocumentedRepositoryPathsResolve(t *testing.T) {
	root := repoRoot(t)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	top := map[string]bool{}
	for _, entry := range entries {
		top[entry.Name()] = true
	}
	tokens := regexp.MustCompile(`[A-Za-z0-9._][A-Za-z0-9._/-]*`).FindAllString(readRepo(t, "AGENTS.md"), -1)
	paths := map[string]bool{}
	for _, token := range tokens {
		token = strings.TrimPrefix(token, "./")
		if !top[strings.SplitN(token, "/", 2)[0]] {
			continue
		}
		paths[token] = true
	}
	if len(paths) <= 5 {
		t.Fatalf("AGENTS.md path parsing found only %d paths", len(paths))
	}
	for path := range paths {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Errorf("AGENTS.md documents %s, which does not exist", path)
		}
	}
}

// --- guarded skills: frontmatter-derived set, both guards, and the load wording a command must not use ---

// guardedVerb matches a verb that introduces a skill name, case-insensitively. The Python original excluded "run" preceded by "read and " and "invoke" preceded by "user to " with lookbehind, which RE2 lacks; guardedVerbNames re-applies both exclusions by hand.
var guardedVerb = regexp.MustCompile("(?i)\\b(run|load|invoke|use)(?:s|ed|ing)?\\b(?: and follow)?(?: the)? ((?:`[a-z-]+`(?:,? (?:and|or) |, )?)+)")

var (
	frontmatterBlock = regexp.MustCompile(`(?s)\A---\n(.*?)\n---\n`)
	backtickName     = regexp.MustCompile("`([a-z-]+)`")
)

func skillFrontmatter(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if match := frontmatterBlock.FindSubmatch(raw); match != nil {
		return string(match[1])
	}
	return ""
}

// guardedNames are the locally authored skills whose frontmatter disables model invocation; a locked body keeps its upstream wording and is excluded.
func guardedNames(t *testing.T) map[string]bool {
	t.Helper()
	var lock struct {
		Skills map[string]json.RawMessage `json:"skills"`
	}
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "skills-lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &lock); err != nil {
		t.Fatal(err)
	}
	paths, err := filepath.Glob(filepath.Join(repoRoot(t), "skills", "*", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, path := range paths {
		name := filepath.Base(filepath.Dir(path))
		if _, locked := lock.Skills[name]; !locked && strings.Contains(skillFrontmatter(t, path), "disable-model-invocation: true") {
			names[name] = true
		}
	}
	return names
}

// guardedSkillsNamedByCommands is the subset of the derived guarded set that a command names; TestGuardedSetIsDerivedFromFrontmatter pins it separately from the full set.
func guardedSkillsNamedByCommands(t *testing.T, names map[string]bool) map[string]bool {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(repoRoot(t), "commands", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	var commands strings.Builder
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		commands.Write(raw)
		commands.WriteString("\n")
	}
	text := commands.String()
	named := map[string]bool{}
	for name := range names {
		if strings.Contains(text, "`"+name+"`") {
			named[name] = true
		}
	}
	return named
}

// guardedVerbNames reports whether a line loads a guarded skill by name through one of the verbs.
func guardedVerbNames(line string, names map[string]bool) bool {
	for _, match := range guardedVerb.FindAllStringSubmatchIndex(line, -1) {
		verb := strings.ToLower(line[match[2]:match[3]])
		before := strings.ToLower(line[:match[2]])
		if verb == "run" && strings.HasSuffix(before, "read and ") {
			continue
		}
		if verb == "invoke" && strings.HasSuffix(before, "user to ") {
			continue
		}
		for _, name := range backtickName.FindAllStringSubmatch(line[match[4]:match[5]], -1) {
			if names[name[1]] {
				return true
			}
		}
	}
	return false
}

// guardedViolations returns the lines that load a guarded skill by name, or that list one under "Skills this loads" without saying it is read from its path.
func guardedViolations(text string, names map[string]bool) []string {
	var found []string
	listing := false
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "## ") {
			listing = line == "## Skills this loads"
		}
		if guardedVerbNames(line, names) {
			found = append(found, line)
			continue
		}
		if listing {
			for name := range names {
				if strings.HasPrefix(line, "- `"+name+"`") && !strings.Contains(line, "read from its path") {
					found = append(found, line)
					break
				}
			}
		}
	}
	return found
}

// pinnedGuardedSkills is every locally authored skill that must carry both invocation guards. grill-with-docs carries them but no command names it, which is why the command-named set below is smaller.
var pinnedGuardedSkills = []string{"adversarial-review", "evidence", "grill-with-docs", "grilling", "local-tracker", "setup-harness", "workflow-checkpoint"}

var pinnedGuardedSkillsNamedByCommands = []string{"adversarial-review", "evidence", "grilling", "local-tracker", "setup-harness", "workflow-checkpoint"}

func TestGuardedSetIsDerivedFromFrontmatter(t *testing.T) {
	names := guardedNames(t)
	if got := sortedNameKeys(names); !equalStringSlices(got, pinnedGuardedSkills) {
		t.Fatalf("guarded set = %v, want %v", got, pinnedGuardedSkills)
	}
	if got := sortedNameKeys(guardedSkillsNamedByCommands(t, names)); !equalStringSlices(got, pinnedGuardedSkillsNamedByCommands) {
		t.Fatalf("command-named guarded set = %v, want %v", got, pinnedGuardedSkillsNamedByCommands)
	}
}

func TestEveryGuardedSkillKeepsBothGuards(t *testing.T) {
	for _, name := range pinnedGuardedSkills {
		head := skillFrontmatter(t, filepath.Join(repoRoot(t), "skills", name, "SKILL.md"))
		if !strings.Contains(head, "disable-model-invocation: true") {
			t.Errorf("%s lost disable-model-invocation", name)
		}
		if !strings.Contains(head, `opencode/autoinvoke: "false"`) {
			t.Errorf("%s lost the opencode autoinvoke guard", name)
		}
	}
}

func TestDuplicationDetectorCatchesACopiedBlock(t *testing.T) {
	block := checkpointBlock(t)
	// A marker-free block carrying exactly the record schema's keys is what only the JSON-key comparison catches.
	restated := "The command restates the schema:\n\n```json\n" +
		`{"state": "active", "branch_tip": "tip", "contract": [], "checks": [], "rationale": "", "handoff": "", "next_action": "next"}` +
		"\n```\n"
	if !duplicatesProtocol(restated, block) {
		t.Error("a restated record schema is not flagged")
	}
	unrelated := "The command carries an unrelated block:\n\n```json\n{\"other\": true}\n```\n"
	if duplicatesProtocol(unrelated, block) {
		t.Error("an unrelated JSON block is flagged as duplicating the schema")
	}
	if duplicatesProtocol("ordinary command prose", block) {
		t.Error("ordinary prose is flagged as duplicating the schema")
	}
}

func TestNoCommandOrPolicyLineLoadsAGuardedSkill(t *testing.T) {
	names := guardedNames(t)
	paths, err := filepath.Glob(filepath.Join(repoRoot(t), "commands", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	paths = append(paths, filepath.Join(repoRoot(t), "POLICY.md"))
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range guardedViolations(string(raw), names) {
			t.Errorf("%s loads a guarded skill: %s", filepath.Base(path), line)
		}
	}
}

func TestRevertingACommandLineToLoadWordingIsCaught(t *testing.T) {
	names := guardedNames(t)
	ship := readRepo(t, "commands/ship.md")
	for _, mutation := range []struct{ old, new string }{
		{"Read `workflow-checkpoint`", "Load `workflow-checkpoint`"},
		{"Read and run `adversarial-review` first", "Run `adversarial-review` first"},
	} {
		if !strings.Contains(ship, mutation.old) {
			t.Fatalf("ship.md lost %q", mutation.old)
		}
		if len(guardedViolations(strings.Replace(ship, mutation.old, mutation.new, 1), names)) == 0 {
			t.Errorf("mutation %q was not caught", mutation.new)
		}
	}
	for _, text := range []string{
		"Load and follow `evidence`: is it real.",
		"load the `local-tracker` skill first",
		"load `ponytail` and `grilling`",
		"use `ponytail`, `tdd`, and `evidence`",
		"read and load `evidence`",
		"invoke the `setup-harness` skill",
		"## Skills this loads\n\n- `adversarial-review`",
	} {
		if len(guardedViolations(text, names)) == 0 {
			t.Errorf("not caught: %q", text)
		}
	}
	clean := "Read and run `adversarial-review` first.\ntell the user to invoke the `setup-harness` skill\n## Skills this loads\n\n- `grilling`, read from its path\n- `ponytail`"
	if got := guardedViolations(clean, names); len(got) != 0 {
		t.Errorf("false positives: %v", got)
	}
}

func sortedNameKeys(m map[string]bool) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func assertStrings(t *testing.T, value any, want []string) {
	t.Helper()
	items, _ := value.([]any)
	var got []string
	for _, item := range items {
		text, _ := item.(string)
		got = append(got, text)
	}
	if !equalStringSlices(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func stringSlice(value any) []string {
	items, _ := value.([]any)
	var out []string
	for _, item := range items {
		if text, ok := item.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

func equalStringSlices(a, b []string) bool {
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

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

// TestPublicationAuthorityHasOneHome keeps the agreement test in one policy section that every publishing workflow defers to, so no workflow restates or weakens it.
func TestPublicationAuthorityHasOneHome(t *testing.T) {
	policy := readRepo(t, "POLICY.md")
	if n := strings.Count(policy, "\n## Publication authority\n"); n != 1 {
		t.Fatalf("POLICY.md has %d Publication authority sections, want 1", n)
	}
	test := "the purpose is settled, the whole scope is settled, the acceptance evidence is named, and an identifiable human message covers that scope"
	if n := strings.Count(policy, test); n != 1 {
		t.Errorf("POLICY.md states the agreement test %d times, want 1", n)
	}
	for _, sentence := range []string{
		"none of them supplies the agreement, alone or together, and an agent finalizing its own artifact never promotes it",
		"`/address-review` and `/feedback` are the deliberate exceptions to this blanket autonomy",
		"A commit, branch, or worktree that stays local on a task branch is not a publication",
		"publishes nothing from scope that arrived after the approval",
		"None of these authorizes an external write",
	} {
		if !strings.Contains(policy, sentence) {
			t.Errorf("POLICY.md lost the anchor %q", sentence)
		}
	}
	for rel, gate := range map[string]string{
		"commands/feedback.md": "Invoking `/feedback` is not that approval",
		"commands/proto.md":    "A ledger this workflow finalized on its own never promotes",
		"commands/land.md":     "confirm once before writing",
	} {
		if !strings.Contains(readRepo(t, rel), gate) {
			t.Errorf("%s lost its gate %q", rel, gate)
		}
	}
	for _, name := range []string{"ship", "feedback", "address-review", "proto", "land", "spec", "create-ticket"} {
		text := readRepo(t, "commands/"+name+".md")
		if !strings.Contains(text, "**Publication authority**") {
			t.Errorf("%s.md does not defer to the policy's **Publication authority**", name)
		}
		if strings.Contains(text, test) {
			t.Errorf("%s.md restates the agreement test instead of deferring to it", name)
		}
	}
}

// The /proto claim is only verifiable while every side keeps the clause that checks it.
func TestProtoClaimIsVerifiable(t *testing.T) {
	for rel, clause := range map[string]string{
		"commands/proto.md":             "the ledger's claim record must name this ticket and this branch",
		"commands/ship.md":              "whose ledger claim record names this ticket and this branch",
		"skills/local-tracker/SKILL.md": "verify it against the ledger's claim record, which must name this ticket and this branch",
		"POLICY.md":                     "the claim record in its ledger is what lets `/ship` verify it",
	} {
		if !strings.Contains(readRepo(t, rel), clause) {
			t.Errorf("%s lost the /proto claim check %q", rel, clause)
		}
	}
}
