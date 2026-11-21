package models

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
)

var presetNotes = map[string]string{
	"balanced": "Codex serves standard; Claude serves heavy, high, and fast (default)",
	"claude":   "Claude serves every tier: fable, opus, sonnet, haiku",
	"codex":    "Codex serves every tier: its newest astra, sol, terra, luna",
}

const (
	visibleRows = 15
	keep        = "keep"
	newest      = "newest"
)

const (
	stepMenu = iota
	stepName
	stepTierClient
	stepTierModel
	stepRoleTier
	stepRoleClient
	stepRoleEffort
)

type picker struct {
	step               int
	cursor             int
	catalog            *Catalog
	choice             Choice
	name, tier, client string
	sets               [][]string
	done               bool
}

func newPicker(catalog *Catalog, choice Choice) picker {
	return picker{catalog: catalog, choice: choice}
}

func (p picker) clients() []string {
	clients := []string{"claude"}
	if listed, _ := p.catalog.Codex(); len(listed) > 0 {
		clients = append(clients, "codex")
	}
	if listed, _ := p.catalog.OpenCode(); len(listed) > 0 {
		clients = append(clients, "opencode")
	}
	return clients
}

func (p picker) options() []string {
	switch p.step {
	case stepName:
		return slices.Concat(Tiers, AllRoles, []string{"save"})
	case stepTierClient:
		return p.clients()
	case stepTierModel:
		switch p.client {
		case "claude":
			return []string{newest}
		case "codex":
			listed, _ := p.catalog.Codex()
			return slices.Concat([]string{newest}, listed)
		}
		listed, _ := p.catalog.OpenCode()
		return listed
	case stepRoleTier:
		return slices.Concat([]string{keep}, Tiers)
	case stepRoleClient:
		return slices.Concat([]string{keep}, p.clients())
	case stepRoleEffort:
		r := p.pending().Resolve(p.name, Launch{}, p.catalog)
		switch r.Client {
		case "claude":
			if efforts, ok := claudeEfforts[r.Model]; ok {
				return efforts
			}
		case "codex":
			return p.catalog.CodexEfforts(r.Model)
		case "opencode":
			if strings.HasPrefix(r.Model, "openai/") {
				return openaiEfforts
			}
		}
		return []string{""}
	}
	return append(presetNames(), "custom")
}

// pending is the choice with the role being picked set to its tier and client
// so far, so the effort step lists what that model takes.
func (p picker) pending() Choice {
	c := p.choice
	c.Roles = maps.Clone(p.choice.Roles)
	c.Tiers = maps.Clone(p.choice.Tiers)
	for _, set := range p.sets {
		if next, err := apply(c, set, p.catalog); err == nil {
			c = next
		}
	}
	c.Roles = maps.Clone(c.Roles)
	if c.Roles == nil {
		c.Roles = map[string]Override{}
	}
	ov := c.Roles[p.name]
	if p.tier != "" {
		ov.Tier = p.tier
	}
	if p.client != "" {
		ov.Client = p.client
	}
	c.Roles[p.name] = ov
	return c
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
		p.step = stepName
	case stepName:
		if opt == "save" {
			p.done = true
			return p, tea.Quit
		}
		p.name, p.tier, p.client = opt, "", ""
		p.step = stepRoleTier
		if slices.Contains(Tiers, opt) {
			p.step = stepTierClient
		}
	case stepTierClient:
		p.client, p.step = opt, stepTierModel
	case stepTierModel:
		set := []string{p.name, "--client", p.client}
		if opt != newest {
			set = append(set, "--model", opt)
		}
		p.sets, p.step = append(p.sets, set), stepName
	case stepRoleTier:
		if opt != keep {
			p.tier = opt
		}
		p.step = stepRoleClient
	case stepRoleClient:
		if opt != keep {
			p.client = opt
		}
		p.step = stepRoleEffort
	case stepRoleEffort:
		set := []string{p.name}
		if p.tier != "" {
			set = append(set, "--tier", p.tier)
		}
		if p.client != "" {
			set = append(set, "--client", p.client)
		}
		if opt != "" {
			set = append(set, "--effort", opt)
		}
		if len(set) > 1 {
			p.sets = append(p.sets, set)
		}
		p.step = stepName
	}
	return p, nil
}

func (p picker) View() tea.View {
	var b strings.Builder
	b.WriteString([]string{
		"Choose a preset, or custom to set tiers and roles one by one",
		"Choose a tier or a role to set, or save",
		"Client to serve " + p.name,
		"Model for " + p.name + " on " + p.client,
		"Tier for " + p.name,
		"Client for " + p.name + ", or keep the tier's",
		"Effort for " + p.name,
	}[p.step] + "\n\n")
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
		case p.step == stepRoleEffort:
			line = effortOrInherit(opt)
		}
		b.WriteString(mark + line + "\n")
	}
	if len(opts) > visibleRows {
		fmt.Fprintf(&b, "  (%d of %d)\n", p.cursor+1, len(opts))
	}
	if p.step == stepTierClient || p.step == stepRoleClient {
		if _, err := p.catalog.Codex(); err != nil {
			b.WriteString("\ncodex unavailable: " + err.Error() + "\n")
		}
		if _, err := p.catalog.OpenCode(); err != nil {
			b.WriteString("\nopencode unavailable: " + err.Error() + "\n")
		}
	}
	if p.step == stepName {
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
		if choice, err = apply(choice, args, p.catalog); err != nil {
			return false, err
		}
	}
	err = Save(path, choice)
	return err == nil, err
}
