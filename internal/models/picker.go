package models

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// presetNotes are the one-line descriptions the picker shows beside each preset.
var presetNotes = map[string]string{
	"balanced":    "OpenCode GLM implements; reviews stay on the session's Claude model (default)",
	"performance": "Opus implements; Fable reviews; all at high effort",
	"economy":     "OpenCode DeepSeek Flash implements; Sonnet reviews",
	"diversity":   "Sonnet implements; review spans Claude, OpenAI, and GLM",
}

// claudeModels is the picker's order for the aliases claudeEfforts supports.
var claudeModels = []string{"inherit", "fable", "opus", "sonnet", "haiku"}

const (
	stepMenu = iota
	stepRole
	stepClient
	stepModel
	stepEffort
)

// picker is the interactive form of `harness models set`. It collects set arguments and writes nothing itself; save applies them.
type picker struct {
	step         int
	cursor       int
	opencode     []string
	opencodeErr  string
	role, client string
	model        string
	sets         [][]string
	done         bool
}

// newPicker takes `opencode models`' result, so tests supply the machine's list.
func newPicker(opencode []string, err error) picker {
	p := picker{opencode: opencode}
	if err != nil {
		p.opencodeErr = err.Error()
	}
	return p
}

// options lists what the current step offers. An effort of "" inherits the session's.
func (p picker) options() []string {
	switch p.step {
	case stepRole:
		return append(slices.Clone(Roles), "save")
	case stepClient:
		if len(p.opencode) == 0 {
			return []string{"claude"}
		}
		return []string{"claude", "opencode"}
	case stepModel:
		if p.client == "claude" {
			return claudeModels
		}
		return p.opencode
	case stepEffort:
		if p.client == "claude" {
			return claudeEfforts[p.model]
		}
		if strings.HasPrefix(p.model, "openai/") {
			return openaiEfforts
		}
		return []string{""}
	}
	return append(presetNames(), "custom")
}

func (p picker) Init() tea.Cmd { return nil }

func (p picker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return p, nil
	}
	switch key.String() {
	case "q", "esc", "ctrl+c":
		return p, tea.Quit
	case "up", "k":
		p.cursor = max(p.cursor-1, 0)
	case "down", "j":
		p.cursor = min(p.cursor+1, len(p.options())-1)
	case "enter":
		return p.pick(p.options()[p.cursor])
	}
	return p, nil
}

func (p picker) pick(opt string) (tea.Model, tea.Cmd) {
	p.cursor = 0
	switch p.step {
	case stepMenu:
		if opt != "custom" {
			p.sets, p.done = [][]string{{"--preset", opt}}, true
			return p, tea.Quit
		}
		p.step = stepRole
	case stepRole:
		if opt == "save" {
			p.done = true
			return p, tea.Quit
		}
		p.role, p.step = opt, stepClient
	case stepClient:
		p.client, p.step = opt, stepModel
	case stepModel:
		p.model, p.step = opt, stepEffort
	case stepEffort:
		p.sets = append(p.sets, []string{p.role, "--client", p.client, "--model", p.model, "--effort", opt})
		p.step = stepRole
	}
	return p, nil
}

func (p picker) View() tea.View {
	var b strings.Builder
	b.WriteString([]string{"Choose a preset, or custom to set roles one by one", "Choose a role to override, or save", "Client for " + p.role, "Model for " + p.role, "Effort for " + p.role + " on " + p.model}[p.step] + "\n\n")
	for i, opt := range p.options() {
		mark := "  "
		if i == p.cursor {
			mark = "> "
		}
		line := opt
		switch {
		case p.step == stepMenu && opt != "custom":
			line = fmt.Sprintf("%-12s %s", opt, presetNotes[opt])
		case p.step == stepEffort:
			line = effortOrInherit(opt)
		}
		b.WriteString(mark + line + "\n")
	}
	if p.step == stepClient && p.opencodeErr != "" {
		b.WriteString("\nopencode unavailable: " + p.opencodeErr + "\n")
	}
	if p.step == stepRole {
		for _, s := range p.sets {
			b.WriteString("\npending: " + strings.Join(s, " "))
		}
	}
	b.WriteString("\n\n↑/↓ move, enter select, esc cancel without writing\n")
	return tea.NewView(b.String())
}

// save applies the collected set arguments through apply and writes once, so a cancelled picker or one rejected value writes nothing.
func (p picker) save(path string) error {
	if !p.done || len(p.sets) == 0 {
		return nil
	}
	choice, err := Load(path)
	if err != nil {
		return err
	}
	for _, args := range p.sets {
		if choice, err = apply(choice, args); err != nil {
			return err
		}
	}
	return Save(path, choice)
}
