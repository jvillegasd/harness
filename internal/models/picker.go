package models

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
)

var presetNotes = map[string]string{
	"balanced":    "Codex GPT-6 Luna implements; Haiku searches; Sonnet reviews at medium (default)",
	"performance": "Opus implements at medium; Sonnet searches; Fable reviews at medium",
	"economy":     "Codex GPT-5.6 Luna implements; Haiku searches; Sonnet reviews at low",
	"diversity":   "Sonnet implements at medium; Haiku searches; Codex GPT-6.1 Sol reviews at low",
}

var claudeModels = []string{"inherit", "fable", "opus", "sonnet", "haiku"}

const visibleRows = 15

const (
	stepMenu = iota
	stepRole
	stepClient
	stepModel
	stepEffort
)

type picker struct {
	step         int
	cursor       int
	catalog      *Catalog
	role, client string
	model        string
	sets         [][]string
	done         bool
}

func newPicker(catalog *Catalog) picker {
	return picker{catalog: catalog}
}

func (p picker) options() []string {
	switch p.step {
	case stepRole:
		return append(slices.Clone(AllRoles), "save")
	case stepClient:
		clients := []string{"claude"}
		if listed, _ := p.catalog.Codex(); len(listed) > 0 {
			clients = append(clients, "codex")
		}
		if listed, _ := p.catalog.OpenCode(); len(listed) > 0 {
			clients = append(clients, "opencode")
		}
		return clients
	case stepModel:
		switch p.client {
		case "claude":
			return claudeModels
		case "codex":
			listed, _ := p.catalog.Codex()
			return slices.Concat([]string{"inherit"}, listed)
		}
		listed, _ := p.catalog.OpenCode()
		return listed
	case stepEffort:
		switch p.client {
		case "claude":
			return claudeEfforts[p.model]
		case "codex":
			return p.catalog.CodexEfforts(p.model)
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
	opts := p.options()
	first := min(max(p.cursor-visibleRows/2, 0), max(len(opts)-visibleRows, 0))
	for i := first; i < min(first+visibleRows, len(opts)); i++ {
		opt := opts[i]
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
	if len(opts) > visibleRows {
		fmt.Fprintf(&b, "  (%d of %d)\n", p.cursor+1, len(opts))
	}
	if p.step == stepClient {
		if _, err := p.catalog.Codex(); err != nil {
			b.WriteString("\ncodex unavailable: " + err.Error() + "\n")
		}
		if _, err := p.catalog.OpenCode(); err != nil {
			b.WriteString("\nopencode unavailable: " + err.Error() + "\n")
		}
	}
	if p.step == stepRole {
		for _, s := range p.sets {
			b.WriteString("\npending: " + strings.Join(s, " "))
		}
	}
	b.WriteString("\n\n↑/↓ move, enter select, esc cancel without writing\n")
	return tea.NewView(b.String())
}

func (p picker) save(path string) (bool, error) {
	if !p.done || len(p.sets) == 0 {
		return false, nil
	}
	choice, err := Load(path)
	if err != nil {
		return false, err
	}
	for _, args := range p.sets {
		if choice, err = apply(choice, args); err != nil {
			return false, err
		}
	}
	err = Save(path, choice)
	return err == nil, err
}
