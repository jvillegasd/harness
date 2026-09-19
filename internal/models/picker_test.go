package models

import (
	"errors"
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
		p := choose(t, newPicker(nil, nil), name)
		if !p.done {
			t.Fatalf("%s: choosing a preset did not finish the picker", name)
		}
		if err := p.save(viaPicker); err != nil {
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
	p := choose(t, newPicker(nil, nil), "custom")
	p = choose(t, p, "reviewer-b")
	p = choose(t, p, "claude")
	p = choose(t, p, "sonnet")
	if got := p.options(); !slices.Equal(got, claudeEfforts["sonnet"]) {
		t.Errorf("sonnet efforts offered %v", got)
	}
	p = choose(t, p, "high")
	p = choose(t, p, "save")
	path := statePath(t)
	if err := p.save(path); err != nil {
		t.Fatal(err)
	}
	out := show(t, path)
	if !strings.Contains(out, "reviewer-b client=claude model=sonnet effort=high (override)") {
		t.Errorf("override not stored:\n%s", out)
	}
	if !strings.Contains(out, "reviewer-a client=claude model=inherit effort=inherit (preset:balanced)") {
		t.Errorf("override leaked into another role:\n%s", out)
	}
}

func TestPickerOffersOnlyListedModelsAndValidEfforts(t *testing.T) {
	listed := []string{"opencode-go/glm-5.3", "openai/gpt-5.6-sol"}
	base := choose(t, choose(t, choose(t, newPicker(listed, nil), "custom"), "executor"), "opencode")
	if got := base.options(); !slices.Equal(got, listed) {
		t.Errorf("opencode models offered %v, want %v", got, listed)
	}
	if got := choose(t, base, "openai/gpt-5.6-sol").options(); !slices.Equal(got, openaiEfforts) {
		t.Errorf("openai efforts offered %v", got)
	}
	if got := choose(t, base, "opencode-go/glm-5.3").options(); !slices.Equal(got, []string{""}) {
		t.Errorf("non-OpenAI efforts offered %v, want inherit only", got)
	}
	haiku := choose(t, choose(t, choose(t, choose(t, newPicker(listed, nil), "custom"), "executor"), "claude"), "haiku")
	if got := haiku.options(); !slices.Equal(got, []string{""}) {
		t.Errorf("haiku efforts offered %v", got)
	}
}

func TestPickerHidesOpenCodeWhenItListsNothing(t *testing.T) {
	p := choose(t, choose(t, newPicker(nil, errors.New("opencode missing")), "custom"), "executor")
	if got := p.options(); !slices.Equal(got, []string{"claude"}) {
		t.Errorf("clients offered %v, want claude only", got)
	}
	if !strings.Contains(p.View().Content, "opencode missing") {
		t.Error("view does not say why opencode is unavailable")
	}
}

func TestPickerCancelWritesNothing(t *testing.T) {
	custom := choose(t, choose(t, choose(t, newPicker(nil, nil), "custom"), "executor"), "claude")
	for name, p := range map[string]picker{
		"menu":          newPicker(nil, nil),
		"after an edit": choose(t, choose(t, custom, "opus"), "high"),
	} {
		p = drive(t, p, keyDown, keyUp, keyEsc)
		if p.done {
			t.Errorf("%s: esc finished the picker", name)
		}
		path := statePath(t)
		if err := p.save(path); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s: cancelling wrote %s", name, path)
		}
	}
}
