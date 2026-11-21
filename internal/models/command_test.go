package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jvillegasd/harness/internal/ui"
)

const codexListing = `{"models":[` +
	`{"slug":"gpt-6-luna","visibility":"list","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"}]},` +
	`{"slug":"gpt-6.1-terra","visibility":"list","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"}]},` +
	`{"slug":"gpt-6-sol","visibility":"list","supported_reasoning_levels":[{"effort":"low"}]},` +
	`{"slug":"gpt-6.1-sol","visibility":"list","supported_reasoning_levels":[{"effort":"low"},{"effort":"high"},{"effort":"ultra"}]},` +
	`{"slug":"gpt-5.6-sol","visibility":"list","supported_reasoning_levels":[{"effort":"low"}]},` +
	`{"slug":"gpt-9-sol","visibility":"hide","supported_reasoning_levels":[{"effort":"low"}]}]}`

// listingPath puts stand-in opencode and codex binaries on PATH.
func listingPath(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", "/elsewhere")
	scripts := map[string]string{
		"opencode": "#!/bin/sh\ncase \"$1\" in models) printf 'opencode-go/glm-5.3\\nopenai/gpt-5.6-sol\\n' ;; esac\n",
		"codex":    "#!/bin/sh\n[ \"$CODEX_HOME\" = \"" + home + "/.codex\" ] || exit 9\n[ \"$1 $2\" = \"debug models\" ] && printf '%s' '" + codexListing + "'\n",
	}
	for name, script := range scripts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
}

func seeded(t *testing.T, block string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.json")
	text := `{"version": 1, "installed": {}, "files": []}` + "\n"
	if block != "" {
		text = `{"version": 1, "installed": {}, "files": [], "models": ` + block + "}\n"
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func runOK(t *testing.T, path string, args ...string) string {
	t.Helper()
	var out strings.Builder
	if _, err := run(path, args, &out); err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return out.String()
}

func resolved(t *testing.T, path string, args ...string) map[string]string {
	t.Helper()
	var got map[string]string
	if err := json.Unmarshal([]byte(runOK(t, path, append(append([]string{"role", "resolve"}, args...), "--json")...)), &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestTiersResolveToEachClientsModel(t *testing.T) {
	listingPath(t)
	path := seeded(t, "")
	for tier, want := range map[string]string{"heavy": "opus", "high": "opus", "standard": "sonnet", "fast": "haiku"} {
		if got := resolved(t, path, "worker", "--tier", tier, "--client", "claude"); got["model"] != want {
			t.Errorf("claude %s = %s, want %s", tier, got["model"], want)
		}
	}
	if got := resolved(t, path, "worker", "--tier", "high", "--client", "codex"); got["model"] != "gpt-6.1-sol" {
		t.Errorf("codex high = %v, want the newest listed sol, not an older or hidden one", got)
	}
	if got := resolved(t, path, "worker", "--tier", "standard", "--client", "codex"); got["model"] != "gpt-6.1-terra" {
		t.Errorf("codex standard = %v", got)
	}
	var out strings.Builder
	if _, err := run(path, []string{"role", "resolve", "worker", "--client", "opencode"}, &out); err == nil || !strings.Contains(err.Error(), "--client opencode --model") {
		t.Errorf("an OpenCode tier with no named model resolved: %v", err)
	}
}

func TestHeavyFallsBackToHighWhenNoHeavyModelIsListed(t *testing.T) {
	listingPath(t)
	path := seeded(t, "")
	got := resolved(t, path, "worker", "--tier", "heavy", "--client", "codex")
	if got["tier"] != "high" || got["model"] != "gpt-6.1-sol" || !strings.Contains(got["note"], "heavy unavailable on codex") {
		t.Errorf("heavy on codex without astra = %v, want high with a note", got)
	}
	if got := resolved(t, path, "worker", "--tier", "fast", "--client", "codex"); got["tier"] != "fast" || got["model"] != "gpt-6-luna" {
		t.Errorf("fast does not fall back, so it resolves on its own: %v", got)
	}
}

func TestRolesRunAtTheirTierAndLensesFollowTheReviewer(t *testing.T) {
	listingPath(t)
	path := seeded(t, "")
	out := flat(t, path)
	for _, want := range []string{
		"heavy client=claude model=opus (preset:balanced)",
		"standard client=codex model=gpt-6.1-terra (preset:balanced)",
		"executor tier=standard client=codex model=gpt-6.1-terra effort=inherit (preset:balanced)",
		"code-search tier=fast client=claude model=haiku effort=inherit (preset:balanced)",
		"worker tier=standard client=codex model=gpt-6.1-terra effort=inherit (preset:balanced)",
		"reviewer tier=high client=claude model=opus effort=low (preset:balanced)",
		"review-risk tier=high client=claude model=opus effort=low (reviewer)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("show lacks %q:\n%s", want, out)
		}
	}
	runOK(t, path, "role", "set", "reviewer", "--tier", "heavy")
	runOK(t, path, "role", "set", "review-spec", "--tier", "fast")
	out = flat(t, path)
	for _, want := range []string{
		"review-risk tier=heavy client=claude model=opus effort=low (reviewer)",
		"review-spec tier=fast client=claude model=haiku effort=inherit (override)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("show lacks %q:\n%s", want, out)
		}
	}
	runOK(t, path, "role", "reset", "review-spec")
	if out := flat(t, path); !strings.Contains(out, "review-spec tier=heavy client=claude model=opus effort=low (reviewer)") {
		t.Errorf("a reset lens does not return to the reviewer:\n%s", out)
	}
}

func TestARoleOnAnotherClientTakesThatClientsModelForItsTier(t *testing.T) {
	listingPath(t)
	path := seeded(t, "")
	runOK(t, path, "role", "set", "reviewer", "--client", "codex", "--effort", "high")
	if got := resolved(t, path, "reviewer"); got["client"] != "codex" || got["model"] != "gpt-6.1-sol" || got["effort"] != "high" || got["source"] != "override" {
		t.Errorf("reviewer on codex = %v", got)
	}
	runOK(t, path, "tier", "set", "high", "--client", "claude", "--model", "claude-opus-5-5")
	if got := resolved(t, path, "review-bugs"); got["model"] != "gpt-6.1-sol" {
		t.Errorf("a model named for claude leaked into the codex reviewer: %v", got)
	}
	if got := resolved(t, path, "worker", "--tier", "high"); got["model"] != "claude-opus-5-5" || got["client"] != "claude" {
		t.Errorf("the named claude model is not used for the tier: %v", got)
	}
}

func TestSetRejectsInvalidValuesWithoutWriting(t *testing.T) {
	listingPath(t)
	const block = `{"preset": "claude"}`
	for name, args := range map[string][]string{
		"unknown preset":              {"tier", "preset", "lavish"},
		"two presets":                 {"tier", "preset", "claude", "codex"},
		"unknown tier":                {"role", "set", "reviewer", "--tier", "ultra"},
		"unknown client":              {"role", "set", "reviewer", "--client", "cursor"},
		"unknown role":                {"role", "set", "planner", "--tier", "high"},
		"a tier set as a role":        {"role", "set", "high", "--tier", "fast"},
		"a role set as a tier":        {"tier", "set", "reviewer", "--client", "claude"},
		"a role given a model":        {"role", "set", "reviewer", "--model", "opus"},
		"a role given nothing":        {"role", "set", "reviewer"},
		"effort the model lacks":      {"role", "set", "code-search", "--effort", "high"},
		"effort the codex model lack": {"role", "set", "executor", "--client", "codex", "--effort", "ultra"},
		"tier without a client":       {"tier", "set", "high", "--model", "opus"},
		"tier given an effort":        {"tier", "set", "high", "--client", "claude", "--effort", "high"},
		"unlisted codex model":        {"tier", "set", "high", "--client", "codex", "--model", "gpt-nope"},
		"hidden codex model":          {"tier", "set", "high", "--client", "codex", "--model", "gpt-9-sol"},
		"unlisted opencode model":     {"tier", "set", "high", "--client", "opencode", "--model", "openai/nope"},
		"opencode with no model":      {"tier", "set", "high", "--client", "opencode"},
		"unknown claude model":        {"tier", "set", "high", "--client", "claude", "--model", "claudius"},
		"opencode role, no mapping":   {"role", "set", "reviewer", "--client", "opencode"},
		"a tier command without one":  {"tier"},
		"an unknown role command":     {"role", "pin", "reviewer"},
		"reset given a name":          {"reset", "high"},
	} {
		path := seeded(t, block)
		before, _ := os.ReadFile(path)
		if _, err := run(path, args, &strings.Builder{}); err == nil {
			t.Errorf("%s: %v was accepted", name, args)
		}
		if after, _ := os.ReadFile(path); string(after) != string(before) {
			t.Errorf("%s: a rejected set rewrote the state file:\n%s", name, after)
		}
	}
}

func TestSetTierNamesOrClearsAModel(t *testing.T) {
	listingPath(t)
	path := seeded(t, "")
	runOK(t, path, "tier", "set", "high", "--client", "codex", "--model", "gpt-6-sol")
	if got := resolved(t, path, "reviewer"); got["model"] != "gpt-6-sol" || got["client"] != "codex" {
		t.Errorf("named model not used: %v", got)
	}
	if out := flat(t, path); !strings.Contains(out, "high client=codex model=gpt-6-sol (set)") {
		t.Errorf("show does not report the set tier:\n%s", out)
	}
	runOK(t, path, "tier", "set", "high", "--client", "codex")
	if got := resolved(t, path, "reviewer"); got["model"] != "gpt-6.1-sol" {
		t.Errorf("omitting --model does not return to the newest: %v", got)
	}
	runOK(t, path, "tier", "set", "high", "--client", "opencode", "--model", "openai/gpt-5.6-sol")
	if got := resolved(t, path, "reviewer"); got["client"] != "opencode" || got["model"] != "openai/gpt-5.6-sol" || got["effort"] != "low" {
		t.Errorf("opencode tier: %v", got)
	}
	runOK(t, path, "tier", "reset", "high")
	if got := resolved(t, path, "reviewer"); got["client"] != "claude" || got["source"] != "preset:balanced" {
		t.Errorf("reset tier did not return to the preset: %v", got)
	}
}

func TestResolveTakesLaunchChoicesOnlyForTheWorker(t *testing.T) {
	listingPath(t)
	path := seeded(t, "")
	if got := resolved(t, path, "worker", "--tier", "high", "--effort", "xhigh"); got["model"] != "opus" || got["effort"] != "xhigh" || got["source"] != "launch" {
		t.Errorf("worker launch: %v", got)
	}
	for name, args := range map[string][]string{
		"a role's tier at launch": {"executor", "--tier", "heavy"},
		"unknown launch tier":     {"worker", "--tier", "ultra"},
		"unsupported effort":      {"worker", "--tier", "fast", "--client", "claude", "--effort", "high"},
		"stray argument":          {"worker", "extra"},
	} {
		if _, err := run(path, append([]string{"role", "resolve"}, args...), &strings.Builder{}); err == nil {
			t.Errorf("%s: resolve was accepted", name)
		}
	}
}

func TestResolveFailsWhenTheTierCannotBeServed(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	path := seeded(t, "")
	_, err := run(path, []string{"role", "resolve", "executor"}, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "listing Codex models failed") {
		t.Errorf("resolve without a Codex catalog: %v", err)
	}
	if out := flat(t, path); !strings.Contains(out, "executor tier=standard client=codex model=unavailable effort=inherit (preset:balanced) [unverified: listing Codex models failed") {
		t.Errorf("show does not mark the unservable tier:\n%s", out)
	}
	if got := resolved(t, path, "code-search"); got["model"] != "haiku" {
		t.Errorf("a Claude tier needs no catalog: %v", got)
	}
}

func TestLoadMigratesAndRejects(t *testing.T) {
	listingPath(t)
	legacy := `{"preset": "economy", "roles": {"reviewer": {"client": "claude", "model": "opus", "effort": "high"}, "executor": {"client": "codex", "model": "gpt-5.6-luna"}, "code-search": {"client": "opencode", "model": "opencode-go/glm-5.3"}, "reviewer-b": {"client": "claude", "model": "opus"}}}`
	c, err := Load(seeded(t, legacy))
	if err != nil {
		t.Fatal(err)
	}
	if c.Preset != "" {
		t.Errorf("a retired preset was kept: %q", c.Preset)
	}
	if got := c.Roles["reviewer"]; got != (Override{Tier: "high", Client: "claude", Effort: "high"}) {
		t.Errorf("reviewer migrated to %+v", got)
	}
	if got := c.Roles["executor"]; got != (Override{Tier: "fast", Client: "codex"}) {
		t.Errorf("executor migrated to %+v", got)
	}
	if got := c.Roles["code-search"]; got != (Override{Client: "opencode"}) || c.Tiers["fast"].Models["opencode"] != "opencode-go/glm-5.3" {
		t.Errorf("an OpenCode override migrated to %+v with tiers %+v", got, c.Tiers)
	}
	if _, ok := c.Roles["reviewer-b"]; ok {
		t.Error("a retired role was kept")
	}
	for name, c := range map[string]struct {
		block string
		reset []string
	}{
		"unknown preset":   {`{"preset": "lavish"}`, []string{"tier", "reset"}},
		"unknown tier key": {`{"tiers": {"ultra": {"client": "claude"}}}`, []string{"tier", "reset"}},
		"bad tier client":  {`{"tiers": {"high": {"client": "cursor"}}}`, []string{"tier", "reset", "high"}},
		"bad role tier":    {`{"roles": {"executor": {"tier": "ultra"}}}`, []string{"role", "reset", "executor"}},
		"unknown role key": {`{"roles": {"planner": {"tier": "high"}}}`, []string{"role", "reset"}},
		"malformed block":  {`{"roles": []}`, []string{"reset"}},
	} {
		path, reset := seeded(t, c.block), c.reset
		_, err := run(path, []string{"show"}, &strings.Builder{})
		if err == nil || !strings.Contains(err.Error(), "run `harness models "+reset[0]) {
			t.Errorf("%s: show returned %v, want harness models %s named", name, err, strings.Join(reset, " "))
		}
		runOK(t, path, reset...)
		flat(t, path)
	}
}

func TestResetDropsOneSettingThenTheBlock(t *testing.T) {
	listingPath(t)
	path := seeded(t, "")
	runOK(t, path, "tier", "preset", "claude")
	runOK(t, path, "role", "set", "executor", "--tier", "high")
	runOK(t, path, "role", "reset", "executor")
	if got := resolved(t, path, "executor"); got["tier"] != "standard" || got["model"] != "sonnet" || got["source"] != "preset:claude" {
		t.Errorf("reset role did not fall back: %v", got)
	}
	runOK(t, path, "reset")
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), `"models"`) || !strings.Contains(string(raw), `"installed"`) {
		t.Errorf("reset left the models block or dropped another key:\n%s", raw)
	}
	if _, err := run(path, []string{"role", "reset", "planner"}, &strings.Builder{}); err == nil {
		t.Error("reset accepted an unknown name")
	}
}

func TestEveryRoleHasATierAndEveryPresetServesEveryTier(t *testing.T) {
	for _, name := range Roles {
		if role(name).Tier == "" {
			t.Errorf("role %s has no default tier", name)
		}
	}
	for preset, table := range Presets {
		for _, tier := range Tiers {
			if table[tier] == "" {
				t.Errorf("preset %s serves no client for %s", preset, tier)
			}
		}
	}
	if !errors.Is(new(Choice).Resolve("worker", Launch{Client: "opencode"}, &Catalog{opencodeDone: true}).Err, ErrNotListed) {
		t.Error("an OpenCode tier with no named model is not reported as not listed")
	}
}

func TestSetMergesIntoARolesOverride(t *testing.T) {
	listingPath(t)
	path := seeded(t, "")
	runOK(t, path, "role", "set", "executor", "--tier", "high")
	runOK(t, path, "role", "set", "executor", "--effort", "high")
	if got := resolved(t, path, "executor"); got["tier"] != "high" || got["effort"] != "high" {
		t.Errorf("a second set dropped the first one's tier: %v", got)
	}
}

func TestAnOverriddenLensInheritsWhatItDidNotSet(t *testing.T) {
	listingPath(t)
	path := seeded(t, "")
	runOK(t, path, "role", "set", "reviewer", "--client", "codex", "--effort", "high")
	runOK(t, path, "role", "set", "review-bugs", "--tier", "fast")
	if got := resolved(t, path, "review-bugs"); got["client"] != "codex" || got["model"] != "gpt-6-luna" || got["tier"] != "fast" {
		t.Errorf("the lens dropped the reviewer's client: %v", got)
	}
}

func TestLoadMovesALegacyOpenCodeModelIntoItsTier(t *testing.T) {
	listingPath(t)
	path := seeded(t, `{"roles": {"code-search": {"client": "opencode", "model": "opencode-go/glm-5.3"}}}`)
	if got := resolved(t, path, "code-search"); got["client"] != "opencode" || got["model"] != "opencode-go/glm-5.3" || got["tier"] != "fast" {
		t.Errorf("a legacy OpenCode override was not kept: %v", got)
	}
}

func TestLoadKeepsALegacyFableOverrideOnFable(t *testing.T) {
	listingPath(t)
	path := seeded(t, `{"roles": {"reviewer": {"client": "claude", "model": "fable"}}}`)
	if got := resolved(t, path, "reviewer"); got["tier"] != "heavy" || got["model"] != "fable" {
		t.Errorf("a legacy fable override moved to another model: %v", got)
	}
}

func TestTierAndRoleCommandsTouchOnlyTheirHalf(t *testing.T) {
	listingPath(t)
	path := seeded(t, "")
	runOK(t, path, "tier", "preset", "claude")
	runOK(t, path, "tier", "set", "high", "--client", "codex")
	runOK(t, path, "role", "set", "executor", "--tier", "high")
	if out := runOK(t, path, "tier", "show"); !strings.Contains(out, "  runs on        codex  › gpt-6.1-sol  set\n") || strings.Contains(out, "executor") {
		t.Errorf("tier show prints more or less than the tiers:\n%s", out)
	}
	if out := runOK(t, path, "role", "show"); !strings.Contains(out, "executor      high      codex  › gpt-6.1-sol") || strings.Contains(out, " HIGH ") {
		t.Errorf("role show prints more or less than the roles:\n%s", out)
	}
	runOK(t, path, "tier", "reset")
	if got := resolved(t, path, "executor"); got["tier"] != "high" || got["client"] != "claude" || got["source"] != "override" {
		t.Errorf("tier reset dropped the role override or kept a tier setting: %v", got)
	}
	runOK(t, path, "tier", "preset", "codex")
	runOK(t, path, "role", "reset")
	if got := resolved(t, path, "executor"); got["tier"] != "standard" || got["source"] != "preset:codex" {
		t.Errorf("role reset kept an override or dropped the preset: %v", got)
	}
}

func TestTierResetClearsAPinALegacyOverrideMade(t *testing.T) {
	listingPath(t)
	for _, reset := range [][]string{{"tier", "reset", "heavy"}, {"tier", "reset"}} {
		path := seeded(t, `{"roles": {"reviewer": {"client": "claude", "model": "fable"}}}`)
		runOK(t, path, reset...)
		if out := flat(t, path); !strings.Contains(out, "heavy client=claude model=opus (preset:balanced)") {
			t.Errorf("%v left the migrated pin:\n%s", reset, out)
		}
	}
}

func TestAnInheritedEffortTheModelLacksIsDroppedWithANote(t *testing.T) {
	listingPath(t)
	path := seeded(t, "")
	runOK(t, path, "role", "set", "reviewer", "--tier", "fast", "--client", "claude")
	if out := flat(t, path); !strings.Contains(out, `reviewer tier=fast client=claude model=haiku effort=inherit (override) [claude haiku does not support effort "low", so it runs at the client default effort]`) {
		t.Errorf("the reviewer's default effort on haiku was not dropped with a note:\n%s", out)
	}
}

// flat reads `show --json` and prints one line per tier and role, so tests
// assert on the report rather than on how it is drawn.
func flat(t *testing.T, path string) string {
	t.Helper()
	var rep report
	if err := json.Unmarshal([]byte(runOK(t, path, "show", "--json")), &rep); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, v := range rep.Tiers {
		fmt.Fprintf(&b, "%s client=%s model=%s (%s)", v.Tier, v.Client, v.Model, v.Source)
		if v.Problem != "" {
			b.WriteString(" [" + tierProblem(v) + "]")
		}
		b.WriteString("\n")
	}
	for _, r := range rep.Roles {
		fmt.Fprintf(&b, "%s tier=%s client=%s model=%s effort=%s (%s)", r.Role, r.Tier, r.Client, r.Model, r.Effort, r.Source)
		for _, p := range []string{r.Note, r.Problem} {
			if p != "" {
				b.WriteString(" [" + p + "]")
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

func TestShowDrawsRolesUnderTheirTier(t *testing.T) {
	listingPath(t)
	path := seeded(t, "")
	runOK(t, path, "role", "set", "review-spec", "--tier", "fast")
	out := runOK(t, path, "show")
	for _, want := range []string{
		"harness models  preset balanced",
		"tier defaults  preset balanced\n\n HEAVY      system design, architecture\n  runs on      claude › opus or fable\n  with claude  claude › opus or fable\n  with codex   codex  › gpt-6.1-sol at high\n",
		" STANDARD   scoped work with tests\n  runs on      codex  › gpt-6.1-terra\n  with claude  claude › sonnet\n  with codex   codex  › gpt-6.1-terra\n",
		"roles  the model each role runs on\n\n HEAVY    \n  └─ no roles",
		" HIGH     \n  ├─ reviewer      effort low      on claude › opus\n  ├─ review-bugs   effort low      on claude › opus  via reviewer\n  └─ review-risk   effort low      on claude › opus  via reviewer\n",
		" STANDARD \n  ├─ executor      effort default  on codex  › gpt-6.1-terra\n  └─ worker        effort default  on codex  › gpt-6.1-terra  tier chosen at launch\n",
		"  └─ review-spec   effort default  on claude › haiku  set\n     ! claude haiku does not support effort \"low\"",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("show lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("show wrote color to a non-terminal:\n%q", out)
	}
	var tree strings.Builder
	choice, _ := Load(path)
	drawTree(ui.Colored, choice.report(new(Catalog)), &tree)
	if !strings.Contains(tree.String(), "\x1b[1;30;46m HIGH     \x1b[0m") {
		t.Errorf("the colored tree does not draw the high tier's pill:\n%q", tree.String())
	}
}

func TestEachTierListsWhatOtherClientsAndPresetsRunItOn(t *testing.T) {
	listingPath(t)
	path := seeded(t, "")
	runOK(t, path, "tier", "set", "high", "--client", "claude", "--model", "fable")
	runOK(t, path, "tier", "set", "fast", "--client", "opencode", "--model", "opencode-go/glm-5.3")
	runOK(t, path, "tier", "set", "fast", "--client", "claude")
	var rep report
	if err := json.Unmarshal([]byte(runOK(t, path, "tier", "show", "--json")), &rep); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, v := range rep.Tiers {
		var parts []string
		for _, o := range v.Others {
			part := o.Client + "=" + o.Model
			if o.Tier != "" {
				part += "@" + o.Tier
			}
			parts = append(parts, part)
		}
		got[v.Tier] = strings.Join(parts, " ")
	}
	want := map[string]string{
		"heavy":    "claude=fable codex=gpt-6.1-sol@high",
		"high":     "claude=opus codex=gpt-6.1-sol",
		"standard": "claude=sonnet",
		"fast":     "codex=gpt-6-luna opencode=opencode-go/glm-5.3",
	}
	for tier, w := range want {
		if got[tier] != w {
			t.Errorf("%s lists %q from other clients, want %q", tier, got[tier], w)
		}
	}
	if got := resolved(t, path, "worker", "--tier", "heavy", "--client", "codex"); got["model"] != "gpt-6.1-sol" || got["tier"] != "high" {
		t.Errorf("the listing disagrees with what a role on codex at heavy resolves to: %v", got)
	}
	presets := map[string]string{}
	for _, v := range rep.Tiers {
		for _, name := range presetNames() {
			m := v.Presets[name]
			presets[v.Tier+" "+name] = m.Client + "=" + m.Model + strings.TrimSuffix("@"+m.Tier, "@")
		}
	}
	for key, w := range map[string]string{
		"heavy balanced":    "claude=opus",
		"heavy codex":       "claude=fable@high",
		"high codex":        "claude=fable",
		"standard balanced": "codex=gpt-6.1-terra",
		"standard claude":   "claude=sonnet",
		"fast codex":        "claude=haiku",
	} {
		if presets[key] != w {
			t.Errorf("under preset %s serves %q, want %q (a tier set by hand keeps its client)", key, presets[key], w)
		}
	}
	if out := runOK(t, path, "tier", "show"); !strings.Contains(out, " HEAVY      system design, architecture\n  runs on      claude › opus or fable\n  with claude  claude › opus or fable\n  with codex   claude › fable at high\n") {
		t.Errorf("tier show does not draw heavy's card with its fallback:\n%s", out)
	}
	// The preview is what a role at heavy runs on once the preset is chosen.
	runOK(t, path, "tier", "preset", "codex")
	if got := resolved(t, path, "worker", "--tier", "heavy"); got["client"] != "claude" || got["model"] != "fable" || got["tier"] != "high" {
		t.Errorf("under preset codex a role at heavy resolves to %v, not what the card showed", got)
	}
}

func TestACardRunsOnTheFallbackAndMarksAModelThatCannotRun(t *testing.T) {
	listingPath(t)
	path := seeded(t, `{"preset": "codex", "tiers": {"fast": {"client": "codex", "models": {"codex": "gpt-6.0-luna"}}}}`)
	out := runOK(t, path, "tier", "show")
	if !strings.Contains(out, " HEAVY      system design, architecture\n  runs on        codex  › gpt-6.1-sol at high\n  ! codex lists no heavy (astra) model") {
		t.Errorf("heavy under codex without astra does not run on its fallback:\n%s", out)
	}
	if !strings.Contains(out, "  with claude    codex  › gpt-6.0-luna not runnable\n") {
		t.Errorf("an unlisted pinned model is not marked:\n%s", out)
	}
	var rep report
	if err := json.Unmarshal([]byte(runOK(t, path, "tier", "show", "--json")), &rep); err != nil {
		t.Fatal(err)
	}
	if p := rep.Tiers[3].Presets["claude"].Problem; !strings.Contains(p, `codex has no model "gpt-6.0-luna"`) {
		t.Errorf("the JSON preset entry carries no problem: %q", p)
	}
}
