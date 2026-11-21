package models

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/jvillegasd/harness/internal/ui"
)

var presetNotes = map[string]string{
	"balanced": "Codex serves standard; Claude serves heavy, high, and fast (default)",
	"claude":   "Claude serves every tier: opus for heavy and high, sonnet, haiku",
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
	style              ui.Style
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
			return []string{newest, "fable", "opus", "sonnet", "haiku"}
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

// applied is the choice with every pending set applied.
func (p picker) applied() Choice {
	c := p.choice
	c.Roles = maps.Clone(p.choice.Roles)
	c.Tiers = maps.Clone(p.choice.Tiers)
	for _, set := range p.sets {
		if next, err := change(c, set, p.catalog); err == nil {
			c = next
		}
	}
	return c
}

// pending is the choice with the role being picked set to its tier and client
// so far, so the effort step lists what that model takes.
func (p picker) pending() Choice {
	c := p.applied()
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
			p.sets, p.done = [][]string{{"tier", "preset", opt}}, true
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
		set := []string{"tier", "set", p.name, "--client", p.client}
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
		set := []string{"role", "set", p.name}
		if p.tier != "" {
			set = append(set, "--tier", p.tier)
		}
		if p.client != "" {
			set = append(set, "--client", p.client)
		}
		if opt != "" {
			set = append(set, "--effort", opt)
		}
		if len(set) > 3 {
			p.sets = append(p.sets, set)
		}
		p.step = stepName
	}
	return p, nil
}

// stepTitles say what each step asks for, after the command's name.
var stepTitles = []string{
	"choose a preset, or custom to set tiers and roles one by one",
	"choose a tier or a role to set, or save",
	"client to serve %s",
	"model for %s on %s",
	"tier for %s",
	"client for %s, or keep the tier's",
	"effort for %s",
}

// View draws the step with the ui design system: a title, the options with
// the cursor's row accented and their details dim, then what the step needs
// beside them, and the keys.
func (p picker) View() tea.View {
	s := p.style
	var b strings.Builder
	title := stepTitles[p.step]
	switch strings.Count(title, "%s") {
	case 1:
		title = fmt.Sprintf(title, p.name)
	case 2:
		title = fmt.Sprintf(title, p.name, p.client)
	}
	b.WriteString(s.Title("harness models", title) + "\n\n")
	opts := p.options()
	first := min(max(p.cursor-visibleRows/2, 0), max(len(opts)-visibleRows, 0))
	var rows [][]string
	for i := first; i < min(first+visibleRows, len(opts)); i++ {
		mark, label := "  ", p.label(opts[i])
		if i == p.cursor {
			mark, label = s.Accent("›")+" ", s.Bold(label)
		}
		rows = append(rows, []string{mark + label, p.detail(opts[i])})
	}
	for _, line := range s.Table(nil, rows) {
		b.WriteString(line + "\n")
	}
	if len(opts) > visibleRows {
		b.WriteString("  " + s.Dim(fmt.Sprintf("%d of %d", p.cursor+1, len(opts))) + "\n")
	}
	if p.step == stepMenu && p.cursor < len(opts) && opts[p.cursor] != "custom" {
		b.WriteString("\n" + s.Title(opts[p.cursor], "runs each tier on") + "\n")
		for _, line := range p.presetPreview(opts[p.cursor]) {
			b.WriteString(line + "\n")
		}
	}
	if p.step == stepTierClient || p.step == stepRoleClient {
		if _, err := p.catalog.Codex(); err != nil {
			b.WriteString("\n" + s.Warn("codex unavailable: "+err.Error()) + "\n")
		}
		if _, err := p.catalog.OpenCode(); err != nil {
			b.WriteString("\n" + s.Warn("opencode unavailable: "+err.Error()) + "\n")
		}
	}
	if p.step == stepName && len(p.sets) > 0 {
		b.WriteString("\n" + s.Title("pending", "written on save") + "\n")
		for _, set := range p.sets {
			b.WriteString("  harness models " + strings.Join(set, " ") + "\n")
		}
	}
	b.WriteString("\n" + s.Dim("↑/↓ move  enter select  esc cancel without writing") + "\n")
	return tea.NewView(b.String())
}

// label is how an option reads: a tier in its hue, save accented, an
// inherited effort as default.
func (p picker) label(opt string) string {
	s := p.style
	switch {
	case p.step == stepRoleEffort:
		if opt == "" {
			return s.Dim("default")
		}
	case p.step == stepName && opt == "save":
		return s.Accent(opt)
	case (p.step == stepName || p.step == stepRoleTier) && slices.Contains(Tiers, opt):
		return s.Color(ui.Hue(slices.Index(Tiers, opt)), opt)
	}
	return opt
}

// detail is an option's dim note: what a preset does, or what a tier or role
// runs on now, pending sets included.
func (p picker) detail(opt string) string {
	s := p.style
	switch p.step {
	case stepMenu:
		note := presetNotes[opt]
		if opt == "custom" {
			note = "set tiers and roles one by one"
		}
		if opt == p.choice.preset() {
			note += "  " + s.Accent("current")
		}
		return s.Dim(note)
	case stepName:
		c := p.applied()
		switch {
		case slices.Contains(Tiers, opt):
			b, _ := c.binding(opt)
			model, _ := c.Model(opt, b.Client, p.catalog)
			return s.Dim(ui.Pad("", 10) + ui.Pad(b.Client, 6) + " › " + orUnavailable(model))
		case opt == "save":
			if len(p.sets) == 1 {
				return s.Dim("write 1 pending change")
			}
			return s.Dim(fmt.Sprintf("write %d pending changes", len(p.sets)))
		}
		r := c.Resolve(opt, Launch{}, p.catalog)
		return s.Dim(ui.Pad(r.Tier, 10) + ui.Pad(r.Client, 6) + " › " + orUnavailable(r.Model))
	}
	return ""
}

// presetPreview is what each tier would run on under a preset.
func (p picker) presetPreview(preset string) []string {
	s := p.style
	var pairs [][2]string
	for _, tier := range Tiers {
		m, alternatives := p.choice.underPreset(tier, preset, p.catalog)
		pairs = append(pairs, [2]string{tier, drawChoice(s, m, alternatives)})
	}
	return s.Fields(pairs...)
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
		if choice, err = change(choice, args, p.catalog); err != nil {
			return false, err
		}
	}
	err = Save(path, choice)
	return err == nil, err
}
