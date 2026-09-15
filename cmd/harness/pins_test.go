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
	for _, block := range jsonBlocksOf(readRepo(t, "harness/skills/workflow-checkpoint/SKILL.md")) {
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
	want := map[string]bool{"state": true, "branch_tip": true, "contract": true, "checks": true, "rationale": true, "next_action": true}
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
	for _, line := range strings.Split(readRepo(t, "harness/skills/workflow-checkpoint/SKILL.md"), "\n") {
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
	commands, err := filepath.Glob(filepath.Join(repoRoot(t), "harness", "commands", "*.md"))
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
		text := strings.ToLower(readRepo(t, "harness/commands/"+name+".md"))
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
		if !strings.Contains(strings.ToLower(readRepo(t, "harness/commands/"+name+".md")), boundary) {
			t.Errorf("%s.md does not save or finalize the checkpoint at its return", name)
		}
	}
	if !strings.Contains(readRepo(t, "harness/commands/land.md"), "`completed`") {
		t.Error("land.md does not name its terminal state")
	}
	abandon := readRepo(t, "harness/commands/abandon.md")
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
	triage, research := reconSection(t, "harness/commands/triage.md"), reconSection(t, "harness/commands/research.md")
	if triage == "" || research == "" {
		t.Fatal("a command carries no recon citations section")
	}
	if triage != research {
		t.Fatal("triage.md and research.md carry different recon citation sections")
	}
	pins := []string{
		"A recon subagent that searches the repository ends on a text message holding only its findings, one per line in the shape `path:line | symbol | mechanism`: the cited location, the named symbol, a bare identifier, that location carries, and the mechanism it is claimed to show. A trailing tool result is not the report: re-run that track once, and if it again ends without one, record it as no data, a `no data` row in `/triage`'s evidence ledger or an entry under `/research`'s `Contradictions and unknowns`, never as refuted or as a track that found nothing.",
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
		text := readRepo(t, "harness/commands/"+name+".md")
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
	skill := readRepo(t, "harness/skills/adversarial-review/SKILL.md")
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
	a := readRepo(t, "harness/skills/adversarial-review/roles/reviewer-a.md")
	b := readRepo(t, "harness/skills/adversarial-review/roles/reviewer-b.md")
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
	skill := readRepo(t, "harness/skills/adversarial-review/SKILL.md")
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
		"harness/POLICY.md": {
			"Use ephemeral specialist roles. Do not create persistent client-specific personas when a command, a skill, or an isolated review context is enough. Role definitions generated from one canonical source, such as the reviewer agents, are shared instructions and client capability configuration, not personas.",
			"When a workflow chooses to delegate implementation, launch the named `executor` agent, whose model and effort come from the harness's central role assignments. Delegation stays optional and changes nothing else: the delegating workflow keeps the approved scope, the commits, the review passes, the conformance check, and every gate. When the named agent is unavailable, say so and do the work inline rather than hand it to an agent without that assignment.",
			"Launch a named role agent without a model or effort override, so it runs on its central assignment.",
			"Before rejecting a contested `bug:` or `risk:`, every consumer launches `review-refuter` once with only that finding and the diff snapshot it was raised against: `corroborated` is fixed, `refuted` is rejected with the refuter's evidence recorded, and an inconclusive, missing, malformed, or unavailable result escalates with neither a dismissal nor a speculative fix until a human decision closes it, recorded where a rejection would be. The refuter adds no reviewer and no round.",
		},
		"harness/skills/adversarial-review/SKILL.md": {
			"Launch each reviewer as its named agent, in parallel, each with a fresh context: `reviewer-a` and `reviewer-b` in Claude Code and OpenCode. The installer generates them from `roles/reviewer-a.md` and `roles/reviewer-b.md` beside this skill, which own each reviewer's standing instructions, lenses, and output format.",
			"When a named agent is unavailable, say so, launch a general agent with the matching role file's full text as its instructions, and report that this fallback enforces only the general agent's own permissions rather than the named agent's read-search restrictions. Never skip or silently omit a review because a named agent is missing.",
			"Give Reviewer A the requirements, the diff snapshot path, and the exact absolute path of the project's own instruction file as its standards, never the implementation rationale. Nothing else.",
			"Give Reviewer B only the diff snapshot path and the exact absolute paths of its review standards: the loaded `ponytail` body and the project's own instruction file. No requirements, no rationale, no ticket.",
			"Standards are paths, never requirements or rationale, so supplying them leaves each reviewer's information boundary where it was. Count a standards file as read only when the reviewer's `loaded:` line names it: supplying a path, or a reviewer recalling what a standard says, is not evidence that it was read.",
			"The project's own instruction file is `AGENTS.md` at the repository root, or `CLAUDE.md` there when no `AGENTS.md` exists, and is left out when neither does. Where the client is known to refuse a standards path, such as OpenCode for a file outside the project, paste that file's text into the first launch instead of its path. When a supplied standards file is missing from a reviewer's `loaded:` line, relaunch that reviewer once with that file's text pasted in; the relaunch replaces that reviewer's output for the same round and never counts toward the cap. A reviewer names a pasted standard as `inline:<absolute path>`. If a standard is still unread, disclose it under the pull request's `## Checks`, or in the requirements ledger under `/proto`.",
			"Before rejecting a contested `bug:` or `risk:`, launch the named `review-refuter` agent once for that finding, with a fresh context. The installer generates it from `roles/review-refuter.md` beside this skill, and it runs on its central assignment. When it is unavailable, the general-agent fallback above applies to it unchanged, with the same disclosure.",
			"Give the refuter only the contested finding, verbatim, and the path of the diff snapshot that finding was raised against. Never the implementation rationale, the requirements, your reason for contesting it, or the conversation. A refuter that answers `unreadable: <path>` gets that snapshot inline under Diff delivery, and the relaunch completes the same invocation rather than adding one.",
			"Snapshot the complete diff once per round into a readable temporary file outside the worktree, and give each reviewer that path. A reviewer that answers `unreadable: <path>` gets the same snapshot inline instead. Never truncate the diff: when neither the file nor inline text can carry the complete snapshot, stop and escalate per the escalation contract.",
		},
		"harness/skills/adversarial-review/roles/reviewer-a.md": {unreadable, access, loaded},
		"harness/skills/adversarial-review/roles/reviewer-b.md": {unreadable, access, loaded},
		"harness/skills/adversarial-review/roles/review-refuter.md": {
			unreadable,
			access,
			"Begin with exactly one verdict line: `verdict: corroborated`, `verdict: refuted`, or `verdict: inconclusive`.",
			"`corroborated` and `refuted` need at least one evidence line, or the parent treats the result as malformed. `inconclusive` states what you checked and the one fact that would settle it. Never propose a fix, raise a new finding, or soften a verdict to hedge.",
		},
		"harness/commands/ship.md": {
			deferLine,
			"- <bug or risk not fixed, at most 50 words>; refuted: <the refuter's path:Lline: evidence>, or human decision: <the decision that closed its escalation>.",
		},
		"harness/commands/feedback.md":       {deferLine},
		"harness/commands/address-review.md": {deferLine},
		"harness/commands/proto.md": {
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
	text := readRepo(t, "harness/commands/harness-audit.md")
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
