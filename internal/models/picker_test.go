package models

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

var (
	keyUp    = tea.KeyPressMsg{Code: tea.KeyUp}
	keyDown  = tea.KeyPressMsg{Code: tea.KeyDown}
	keyEnter = tea.KeyPressMsg{Code: tea.KeyEnter}
	keyEsc   = tea.KeyPressMsg{Code: tea.KeyEscape}
	keyQ     = tea.KeyPressMsg{Code: 'q', Text: "q"}
)

func drive(t *testing.T, p picker, keys ...tea.KeyPressMsg) picker {
	t.Helper()
	for _, k := range keys {
		next, _ := p.Update(k)
		p = next.(picker)
	}
	return p
}

func choose(t *testing.T, p picker, opt string) picker {
	t.Helper()
	i := slices.Index(p.options(), opt)
	if i < 0 {
		t.Fatalf("step %d does not offer %q: %v", p.step, opt, p.options())
	}
	for p.cursor < i {
		p = drive(t, p, keyDown)
	}
	return drive(t, p, keyEnter)
}

func show(t *testing.T, path string) string {
	t.Helper()
	return flat(t, path)
}

func catalogOf(opencode []string, err error) *Catalog {
	return &Catalog{opencode: opencode, opencodeErr: err, opencodeDone: true, codexErr: errors.New("codex missing"), codexDone: true}
}

func codexCatalogOf() *Catalog {
	return &Catalog{codex: []string{"gpt-6-luna", "gpt-6.1-sol"}, codexEfforts: map[string][]string{"gpt-6-luna": {"low", "medium"}, "gpt-6.1-sol": {"low", "high", "ultra"}}, codexDone: true, opencodeErr: errors.New("opencode missing"), opencodeDone: true}
}

func statePath(t *testing.T) string {
	t.Setenv("PATH", t.TempDir())
	return filepath.Join(t.TempDir(), "state.json")
}

func fresh(cat *Catalog) picker { return newPicker(cat, Choice{}) }

func TestPickerPresetStoresWhatSetStores(t *testing.T) {
	for name := range Presets {
		viaSet, viaPicker := statePath(t), statePath(t)
		if _, err := run(viaSet, []string{"tier", "preset", name}, &strings.Builder{}); err != nil {
			t.Fatal(err)
		}
		p := choose(t, fresh(catalogOf(nil, nil)), name)
		if !p.done {
			t.Fatalf("%s: choosing a preset did not finish the picker", name)
		}
		if _, err := p.save(viaPicker); err != nil {
			t.Fatal(err)
		}
		a, _ := os.ReadFile(viaSet)
		b, _ := os.ReadFile(viaPicker)
		if string(a) != string(b) {
			t.Errorf("%s: picker stored %s, set stored %s", name, b, a)
		}
		if show(t, viaSet) != show(t, viaPicker) {
			t.Errorf("%s: show differs between picker and set", name)
		}
	}
}

func TestPickerSetsARoleTierClientAndEffort(t *testing.T) {
	p := choose(t, fresh(catalogOf(nil, nil)), "custom")
	p = choose(t, p, "reviewer")
	p = choose(t, p, "standard")
	p = choose(t, p, "claude")
	if got := p.options(); !slices.Equal(got, claudeEfforts["sonnet"]) {
		t.Errorf("efforts offered for the standard tier on claude: %v", got)
	}
	p = choose(t, p, "high")
	p = choose(t, p, "save")
	path := statePath(t)
	if wrote, err := p.save(path); err != nil || !wrote {
		t.Fatalf("save wrote=%v err=%v", wrote, err)
	}
	out := show(t, path)
	if !strings.Contains(out, "reviewer tier=standard client=claude model=sonnet effort=high (override)") {
		t.Errorf("override not stored:\n%s", out)
	}
	if !strings.Contains(out, "code-search tier=fast client=claude model=haiku effort=inherit (preset:balanced)") {
		t.Errorf("override leaked into another role:\n%s", out)
	}
}

func TestPickerSetsARoleToTheSessionTierInOneStep(t *testing.T) {
	p := choose(t, fresh(catalogOf(nil, nil)), "custom")
	p = choose(t, p, "reviewer")
	p = choose(t, p, "session")
	p = choose(t, p, "save")
	path := statePath(t)
	if wrote, err := p.save(path); err != nil || !wrote {
		t.Fatalf("save wrote=%v err=%v", wrote, err)
	}
	if out := show(t, path); !strings.Contains(out, "reviewer tier=session client=session model=inherit effort=inherit (override)") {
		t.Errorf("session tier not stored:\n%s", out)
	}
}

func TestPickerOffersEffortsOfTheResolvedModel(t *testing.T) {
	haiku := choose(t, choose(t, choose(t, choose(t, fresh(catalogOf(nil, nil)), "custom"), "executor"), "fast"), "claude")
	if got := haiku.options(); !slices.Equal(got, []string{""}) {
		t.Errorf("haiku efforts offered %v", got)
	}
	sol := choose(t, choose(t, choose(t, choose(t, fresh(codexCatalogOf()), "custom"), "reviewer"), keep), "codex")
	if got := sol.options(); !slices.Equal(got, []string{"", "low", "high", "ultra"}) {
		t.Errorf("efforts offered for the reviewer's tier on codex: %v", got)
	}
}

func TestPickerServesATierFromAClient(t *testing.T) {
	clients := choose(t, choose(t, fresh(codexCatalogOf()), "custom"), "high")
	if got := clients.options(); !slices.Equal(got, []string{"claude", "codex"}) {
		t.Errorf("clients offered %v", got)
	}
	models := choose(t, clients, "codex")
	if got := models.options(); !slices.Equal(got, []string{newest, "gpt-6-luna", "gpt-6.1-sol"}) {
		t.Errorf("codex models offered %v", got)
	}
	p := choose(t, choose(t, models, "gpt-6.1-sol"), "save")
	if !slices.Equal(p.sets[0], []string{"tier", "set", "high", "--client", "codex", "--model", "gpt-6.1-sol"}) {
		t.Errorf("tier set %v", p.sets)
	}
	opencode := choose(t, choose(t, choose(t, fresh(catalogOf([]string{"openai/gpt-5.6-sol"}, nil)), "custom"), "fast"), "opencode")
	if got := opencode.options(); !slices.Equal(got, []string{"openai/gpt-5.6-sol"}) {
		t.Errorf("opencode offers no newest, only listed models: %v", got)
	}
}

func TestPickerHidesOpenCodeWhenItListsNothing(t *testing.T) {
	p := choose(t, choose(t, fresh(catalogOf(nil, errors.New("opencode missing"))), "custom"), "high")
	if got := p.options(); !slices.Equal(got, []string{"claude"}) {
		t.Errorf("clients offered %v, want claude only", got)
	}
	if !strings.Contains(p.View().Content, "opencode missing") {
		t.Error("view does not say why opencode is unavailable")
	}
}

func TestPickerCancelWritesNothing(t *testing.T) {
	custom := choose(t, choose(t, choose(t, fresh(catalogOf(nil, nil)), "custom"), "executor"), "high")
	for name, p := range map[string]picker{
		"menu":          fresh(catalogOf(nil, nil)),
		"after an edit": choose(t, choose(t, custom, "claude"), "high"),
	} {
		p = drive(t, p, keyDown, keyUp)
		next, cmd := p.Update(keyQ)
		if cmd == nil {
			t.Fatalf("%s: q did not quit", name)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("%s: q did not quit", name)
		}
		p = next.(picker)
		path := statePath(t)
		if wrote, err := p.save(path); err != nil || wrote {
			t.Fatalf("%s: save wrote=%v err=%v", name, wrote, err)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s: cancelling wrote %s", name, path)
		}
	}
}

func TestPickerOffersEffortsAfterTheTiersItQueued(t *testing.T) {
	p := choose(t, choose(t, choose(t, fresh(codexCatalogOf()), "custom"), "high"), "codex")
	p = choose(t, p, newest)
	p = choose(t, choose(t, choose(t, p, "reviewer"), keep), keep)
	if got := p.options(); !slices.Equal(got, []string{"", "low", "high", "ultra"}) {
		t.Errorf("efforts ignore the queued codex tier: %v", got)
	}
}

func TestPickerKeepLeavesAnExistingOverride(t *testing.T) {
	listingPath(t)
	path := filepath.Join(t.TempDir(), "state.json")
	if _, err := run(path, []string{"role", "set", "executor", "--tier", "high"}, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	choice, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	p := choose(t, choose(t, choose(t, choose(t, newPicker(new(Catalog), choice), "custom"), "executor"), keep), keep)
	p = choose(t, choose(t, p, "xhigh"), "save")
	if _, err := p.save(path); err != nil {
		t.Fatal(err)
	}
	if out := show(t, path); !strings.Contains(out, "executor tier=high client=claude model=opus effort=xhigh (override)") {
		t.Errorf("keep did not keep the tier:\n%s", out)
	}
}

func TestPickerMapsAClaudeTierToAnAlias(t *testing.T) {
	p := choose(t, choose(t, choose(t, fresh(catalogOf(nil, nil)), "custom"), "heavy"), "claude")
	p = choose(t, choose(t, p, "fable"), "save")
	if !slices.Equal(p.sets[0], []string{"tier", "set", "heavy", "--client", "claude", "--model", "fable"}) {
		t.Errorf("tier set %v", p.sets)
	}
}

func TestPickerOffersOnlyClientsWithAModelAtTheRolesTier(t *testing.T) {
	listingPath(t)
	p := choose(t, choose(t, choose(t, newPicker(new(Catalog), Choice{}), "custom"), "reviewer"), keep)
	if got := p.options(); slices.Contains(got, "opencode") || !slices.Contains(got, "codex") {
		t.Errorf("clients offered for the reviewer at high: %v, want codex and not opencode", got)
	}
}

func TestPickerRefusesAChangeSaveWouldRefuse(t *testing.T) {
	listingPath(t)
	p := choose(t, newPicker(new(Catalog), Choice{}), "custom")
	p = choose(t, choose(t, choose(t, p, "high"), "codex"), newest)
	p.name, p.tier, p.client, p.step = "reviewer", "", "opencode", stepRoleEffort
	p = choose(t, p, "")
	if p.step != stepRoleEffort || !strings.Contains(p.View().Content, "! opencode has no model for the high tier") {
		t.Errorf("a refused pick left step %d:\n%s", p.step, p.View().Content)
	}
	if len(p.sets) != 1 {
		t.Fatalf("pending = %v, want only the tier change", p.sets)
	}
	p.step, p.cursor = stepName, len(p.options())-1
	p = choose(t, p, "save")
	path := statePath(t)
	if wrote, err := p.save(path); err != nil || !wrote {
		t.Fatalf("the valid change was not saved: wrote=%v err=%v", wrote, err)
	}
}

func TestPickerDefaultEffortClearsAnEffortTheNewModelLacks(t *testing.T) {
	listingPath(t)
	p := choose(t, newPicker(new(Catalog), Choice{}), "custom")
	p = choose(t, choose(t, choose(t, choose(t, p, "reviewer"), keep), keep), "high")
	p = choose(t, choose(t, choose(t, choose(t, p, "reviewer"), "fast"), keep), "")
	p = choose(t, p, "save")
	path := statePath(t)
	if wrote, err := p.save(path); err != nil || !wrote {
		t.Fatalf("moving the reviewer to haiku was not saved: wrote=%v err=%v", wrote, err)
	}
	choice, _ := Load(path)
	if ov := choice.Roles["reviewer"]; ov.Tier != "fast" || ov.Effort != "" {
		t.Errorf("reviewer override = %+v, want tier fast and no effort", ov)
	}
}

func TestPickerResetsATierAndARole(t *testing.T) {
	listingPath(t)
	choice := Choice{Tiers: map[string]Binding{"high": {Client: "codex"}}, Roles: map[string]Override{"reviewer": {Client: "codex"}}}
	p := choose(t, newPicker(new(Catalog), choice), "custom")
	p = choose(t, choose(t, p, "high"), reset)
	p = choose(t, choose(t, p, "reviewer"), reset)
	if got := fmt.Sprint(p.sets); got != "[[tier reset high] [role reset reviewer]]" {
		t.Fatalf("pending = %s", got)
	}
	if opts := choose(t, p, "executor").options(); slices.Contains(opts, reset) {
		t.Errorf("reset offered for a role with nothing set: %v", opts)
	}
}

func TestPickerKeepClearsAPickMadeBeforeEsc(t *testing.T) {
	listingPath(t)
	back := func(p picker) picker { next, _ := p.Update(keyEsc); return next.(picker) }
	p := choose(t, choose(t, choose(t, newPicker(new(Catalog), Choice{}), "custom"), "reviewer"), "fast")
	p = choose(t, choose(t, choose(t, back(p), keep), keep), "")
	q := choose(t, choose(t, choose(t, choose(t, newPicker(new(Catalog), Choice{}), "custom"), "executor"), keep), "codex")
	q = choose(t, choose(t, back(q), keep), "")
	if len(p.sets)+len(q.sets) != 0 {
		t.Errorf("keep after esc still queued the earlier pick: %v %v", p.sets, q.sets)
	}
}

func TestPickerOffersNoClientToARoleKeptAtTheSessionTier(t *testing.T) {
	p := newPicker(catalogOf(nil, nil), Choice{Roles: map[string]Override{"reviewer": {Tier: Session}}})
	p = choose(t, choose(t, choose(t, p, "custom"), "reviewer"), keep)
	if got := p.options(); !slices.Equal(got, []string{keep}) {
		t.Errorf("clients offered to a role at the session tier, which role set refuses: %v", got)
	}
}
