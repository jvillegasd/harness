package models

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	out := runOK(t, path, "show")
	for _, want := range []string{
		"heavy client=claude model=opus (preset:balanced)",
		"standard client=codex model=gpt-6.1-terra (preset:balanced)",
		"executor tier=standard client=codex model=gpt-6.1-terra effort=inherit (preset:balanced)",
		"code-search tier=fast client=claude model=haiku effort=inherit (preset:balanced)",
		"worker tier=standard client=codex model=gpt-6.1-terra effort=inherit (preset:balanced)",
		"reviewer tier=high client=claude model=opus effort=medium (preset:balanced)",
		"review-risk tier=high client=claude model=opus effort=medium (reviewer)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("show lacks %q:\n%s", want, out)
		}
	}
	runOK(t, path, "role", "set", "reviewer", "--tier", "heavy")
	runOK(t, path, "role", "set", "review-spec", "--tier", "fast")
	out = runOK(t, path, "show")
	for _, want := range []string{
		"review-risk tier=heavy client=claude model=opus effort=medium (reviewer)",
		"review-spec tier=fast client=claude model=haiku effort=inherit (override)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("show lacks %q:\n%s", want, out)
		}
	}
	runOK(t, path, "role", "reset", "review-spec")
	if out := runOK(t, path, "show"); !strings.Contains(out, "review-spec tier=heavy client=claude model=opus effort=medium (reviewer)") {
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
	if out := runOK(t, path, "show"); !strings.Contains(out, "high client=codex model=gpt-6-sol (set)") {
		t.Errorf("show does not report the set tier:\n%s", out)
	}
	runOK(t, path, "tier", "set", "high", "--client", "codex")
	if got := resolved(t, path, "reviewer"); got["model"] != "gpt-6.1-sol" {
		t.Errorf("omitting --model does not return to the newest: %v", got)
	}
	runOK(t, path, "tier", "set", "high", "--client", "opencode", "--model", "openai/gpt-5.6-sol")
	if got := resolved(t, path, "reviewer"); got["client"] != "opencode" || got["model"] != "openai/gpt-5.6-sol" || got["effort"] != "medium" {
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
	if out := runOK(t, path, "show"); !strings.Contains(out, "executor tier=standard client=codex model=unavailable effort=inherit (preset:balanced) [unverified: listing Codex models failed") {
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
		runOK(t, path, "show")
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
	if out := runOK(t, path, "tier", "show"); !strings.Contains(out, "high client=codex") || strings.Contains(out, "executor") {
		t.Errorf("tier show prints more or less than the tiers:\n%s", out)
	}
	if out := runOK(t, path, "role", "show"); !strings.Contains(out, "executor tier=high client=codex") || strings.Contains(out, "\nhigh client=") {
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
