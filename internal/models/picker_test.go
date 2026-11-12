package models

import (
	"errors"
	"fmt"
	"maps"
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
)

// drive feeds key messages through Update, as the running program would.
func drive(t *testing.T, p picker, keys ...tea.KeyPressMsg) picker {
	t.Helper()
	for _, k := range keys {
		next, _ := p.Update(k)
		p = next.(picker)
	}
	return p
}

// choose moves the cursor onto opt and presses enter, failing when the current step does not offer it.
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
	var out strings.Builder
	if err := run(path, []string{"show"}, &out); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

// catalogOf is a catalog listing the given OpenCode models and no Codex, so a picker test never asks this machine's clients.
func catalogOf(opencode []string, err error) *Catalog {
	return &Catalog{opencode: opencode, opencodeErr: err, opencodeDone: true, codexErr: errors.New("codex missing"), codexDone: true}
}

// statePath is a fresh state file, with PATH emptied so show and Validate never reach this machine's opencode.
func statePath(t *testing.T) string {
	t.Setenv("PATH", t.TempDir())
	return filepath.Join(t.TempDir(), "state.json")
}

func TestPickerPresetStoresWhatSetStores(t *testing.T) {
	for name := range Presets {
		viaSet, viaPicker := statePath(t), statePath(t)
		if err := run(viaSet, []string{"set", "--preset", name}, &strings.Builder{}); err != nil {
			t.Fatal(err)
		}
		p := choose(t, newPicker(catalogOf(nil, nil)), name)
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

func TestPickerCustomOverridesRole(t *testing.T) {
	p := choose(t, newPicker(catalogOf(nil, nil)), "custom")
	p = choose(t, p, "reviewer")
	p = choose(t, p, "claude")
	p = choose(t, p, "sonnet")
	if got := p.options(); !slices.Equal(got, claudeEfforts["sonnet"]) {
		t.Errorf("sonnet efforts offered %v", got)
	}
	p = choose(t, p, "high")
	p = choose(t, p, "save")
	path := statePath(t)
	if wrote, err := p.save(path); err != nil || !wrote {
		t.Fatalf("save wrote=%v err=%v", wrote, err)
	}
	out := show(t, path)
	if !strings.Contains(out, "reviewer client=claude model=sonnet effort=high (override)") {
		t.Errorf("override not stored:\n%s", out)
	}
	if !strings.Contains(out, "code-search client=claude model=haiku effort=inherit (preset:balanced)") {
		t.Errorf("override leaked into another role:\n%s", out)
	}
}

func TestPickerOffersOnlyListedModelsAndValidEfforts(t *testing.T) {
	listed := []string{"opencode-go/glm-5.3", "openai/gpt-5.6-sol"}
	base := choose(t, choose(t, choose(t, newPicker(catalogOf(listed, nil)), "custom"), "executor"), "opencode")
	if got := base.options(); !slices.Equal(got, listed) {
		t.Errorf("opencode models offered %v, want %v", got, listed)
	}
	if got := choose(t, base, "openai/gpt-5.6-sol").options(); !slices.Equal(got, openaiEfforts) {
		t.Errorf("openai efforts offered %v", got)
	}
	if got := choose(t, base, "opencode-go/glm-5.3").options(); !slices.Equal(got, []string{""}) {
		t.Errorf("non-OpenAI efforts offered %v, want inherit only", got)
	}
	haiku := choose(t, choose(t, choose(t, choose(t, newPicker(catalogOf(listed, nil)), "custom"), "executor"), "claude"), "haiku")
	if got := haiku.options(); !slices.Equal(got, []string{""}) {
		t.Errorf("haiku efforts offered %v", got)
	}
}

func TestPickerHidesOpenCodeWhenItListsNothing(t *testing.T) {
	p := choose(t, choose(t, newPicker(catalogOf(nil, errors.New("opencode missing"))), "custom"), "executor")
	if got := p.options(); !slices.Equal(got, []string{"claude"}) {
		t.Errorf("clients offered %v, want claude only", got)
	}
	if !strings.Contains(p.View().Content, "opencode missing") {
		t.Error("view does not say why opencode is unavailable")
	}
}

func TestPickerOffersCodexModelsAndTheirEfforts(t *testing.T) {
	catalog := &Catalog{codex: []string{"gpt-6-luna", "gpt-6.1-sol"}, codexEfforts: map[string][]string{"gpt-6-luna": {"low", "medium"}, "gpt-6.1-sol": {"low", "high", "ultra"}}, codexDone: true, opencodeErr: errors.New("opencode missing"), opencodeDone: true}
	clients := choose(t, choose(t, newPicker(catalog), "custom"), "reviewer")
	if got := clients.options(); !slices.Equal(got, []string{"claude", "codex"}) {
		t.Errorf("clients offered %v", got)
	}
	base := choose(t, clients, "codex")
	if got := base.options(); !slices.Equal(got, []string{"inherit", "gpt-6-luna", "gpt-6.1-sol"}) {
		t.Errorf("codex models offered %v", got)
	}
	if got := choose(t, base, "gpt-6.1-sol").options(); !slices.Equal(got, []string{"", "low", "high", "ultra"}) {
		t.Errorf("sol efforts offered %v", got)
	}
	if got := choose(t, base, "inherit").options(); !slices.Equal(got, []string{""}) {
		t.Errorf("inherit efforts offered %v", got)
	}
}

func TestPickerCancelWritesNothing(t *testing.T) {
	custom := choose(t, choose(t, choose(t, newPicker(catalogOf(nil, nil)), "custom"), "executor"), "claude")
	for name, p := range map[string]picker{
		"menu":          newPicker(catalogOf(nil, nil)),
		"after an edit": choose(t, choose(t, custom, "opus"), "high"),
	} {
		p = drive(t, p, keyDown, keyUp)
		next, cmd := p.Update(keyEsc)
		if cmd == nil {
			t.Fatalf("%s: esc did not quit", name)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("%s: esc did not quit", name)
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

func TestPickerViewKeepsCursorOnScreen(t *testing.T) {
	var listed []string
	for i := range 49 {
		listed = append(listed, fmt.Sprintf("p/m%02d", i))
	}
	p := choose(t, choose(t, choose(t, newPicker(catalogOf(listed, nil)), "custom"), "executor"), "opencode")
	for range 40 {
		p = drive(t, p, keyDown)
	}
	view := p.View().Content
	if !strings.Contains(view, "> p/m40") || strings.Contains(view, "p/m00") {
		t.Errorf("view does not follow the cursor:\n%s", view)
	}
	if got := strings.Count(view, "p/m"); got != visibleRows {
		t.Errorf("view renders %d ids, want %d", got, visibleRows)
	}
	for range 20 {
		p = drive(t, p, keyDown)
	}
	if view := p.View().Content; !strings.Contains(view, "> p/m48") || strings.Count(view, "p/m") != visibleRows {
		t.Errorf("view at the end of the list:\n%s", view)
	}
}

// TestPickerTablesMatch fails when a preset lacks its note or a Claude alias is missing from either table, which would leave the picker a blank line or an empty effort step.
func TestPickerTablesMatch(t *testing.T) {
	for name := range Presets {
		if presetNotes[name] == "" {
			t.Errorf("preset %s has no picker note", name)
		}
	}
	if len(presetNotes) != len(Presets) {
		t.Errorf("presetNotes has %d entries for %d presets", len(presetNotes), len(Presets))
	}
	aliases := slices.Sorted(maps.Keys(claudeEfforts))
	if !slices.Equal(aliases, slices.Sorted(slices.Values(claudeModels))) {
		t.Errorf("claudeModels %v does not match claudeEfforts %v", claudeModels, aliases)
	}
}
