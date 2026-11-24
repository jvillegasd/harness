package models

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jvillegasd/harness/internal/ui"
)

var presetNotes = map[string]string{
	"balanced": "Claude for heavy, high, fast; Codex for standard",
	"claude":   "Claude for every tier",
	"codex":    "Codex for every tier",
}

const (
	visibleRows = 15
	keep        = "keep"
	newest      = "newest"
	reset       = "reset"
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
	// refused is why the last pick could not be applied; the picker stays on its step.
	refused string
	// width is the terminal's, from its size message; 0 until one arrives.
	width int
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
		if _, set := p.applied().Tiers[p.name]; set {
			return slices.Concat([]string{reset}, p.clients())
		}
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
		if _, set := p.applied().Roles[p.name]; set {
			return slices.Concat([]string{reset, keep}, Tiers, []string{Session})
		}
		return slices.Concat([]string{keep}, Tiers, []string{Session})
	case stepRoleClient:
		// Only clients with a model at the role's tier, since role set refuses the others.
		out := []string{keep}
		for _, client := range p.clients() {
			c := p.pending()
			ov := c.Roles[p.name]
			ov.Client = client
			c.Roles[p.name] = ov
			if c.Resolve(p.name, Launch{}, p.catalog).Err == nil {
				out = append(out, client)
			}
		}
		return out
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
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		p.width = size.Width
		return p, nil
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return p, nil
	}
	p.refused = ""
	switch key.String() {
	case "q", "ctrl+c":
		return p, tea.Quit
	case "esc":
		// Back one step; on the first step there is nowhere back to, so quit.
		back, ok := backSteps[p.step]
		if !ok {
			return p, tea.Quit
		}
		p.step, p.cursor = back, 0
	case "up", "k":
		p.cursor = max(p.cursor-1, 0)
	case "down", "j":
		p.cursor = min(p.cursor+1, len(p.options())-1)
	case "enter":
		return p.pick(p.options()[p.cursor])
	}
	return p, nil
}

// backSteps is the step esc returns to from each step.
var backSteps = map[int]int{
	stepName:       stepMenu,
	stepTierClient: stepName,
	stepTierModel:  stepTierClient,
	stepRoleTier:   stepName,
	stepRoleClient: stepRoleTier,
	stepRoleEffort: stepRoleClient,
}

func (p picker) pick(opt string) (tea.Model, tea.Cmd) {
	at := p.cursor
	p.cursor = 0
	switch p.step {
	case stepMenu:
		if opt != "custom" {
			p.sets, p.done = append(p.sets, []string{"tier", "preset", opt}), true
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
		if opt == reset {
			return p.queue(stepTierClient, at, []string{"tier", "reset", p.name})
		}
		p.client, p.step = opt, stepTierModel
	case stepTierModel:
		set := []string{"tier", "set", p.name, "--client", p.client}
		if opt != newest {
			set = append(set, "--model", opt)
		}
		return p.queue(stepTierModel, at, set)
	case stepRoleTier:
		if opt == reset {
			return p.queue(stepRoleTier, at, []string{"role", "reset", p.name})
		}
		// The session tier takes no client or effort, so it is set here.
		if opt == Session {
			return p.queue(stepRoleTier, at, []string{"role", "set", p.name, "--tier", Session})
		}
		// keep clears an earlier pick of this step, such as one made before esc.
		p.tier = ""
		if opt != keep {
			p.tier = opt
		}
		p.step = stepRoleClient
	case stepRoleClient:
		p.client = ""
		if opt != keep {
			p.client = opt
		}
		p.step = stepRoleEffort
	case stepRoleEffort:
		return p.queue(stepRoleEffort, at, p.roleSets(opt)...)
	}
	return p, nil
}

// roleSets are the changes a role pick makes. Default clears an effort the
// role was set to, which role set cannot do alone, so it resets the role and
// sets again what it keeps.
func (p picker) roleSets(effort string) [][]string {
	ov := p.applied().Roles[p.name]
	tier, client := cmp.Or(p.tier, ov.Tier), cmp.Or(p.client, ov.Client)
	set := []string{"role", "set", p.name}
	if effort == "" && ov.Effort != "" {
		var out [][]string
		out = append(out, []string{"role", "reset", p.name})
		if tier != "" {
			set = append(set, "--tier", tier)
		}
		if client != "" {
			set = append(set, "--client", client)
		}
		if len(set) > 3 {
			out = append(out, set)
		}
		return out
	}
	if p.tier != "" {
		set = append(set, "--tier", p.tier)
	}
	if p.client != "" {
		set = append(set, "--client", p.client)
	}
	if effort != "" {
		set = append(set, "--effort", effort)
	}
	if len(set) == 3 {
		return nil
	}
	return [][]string{set}
}

// queue applies sets to the pending choice as save will. When one is
// refused, it stays on step with the reason and queues nothing, so the
// pending list only holds changes that save writes.
func (p picker) queue(step, cursor int, sets ...[]string) (tea.Model, tea.Cmd) {
	c := p.applied()
	for _, set := range sets {
		next, err := change(c, set, p.catalog)
		if err != nil {
			p.step, p.cursor, p.refused = step, cursor, err.Error()
			return p, nil
		}
		c = next
	}
	p.sets, p.step = append(p.sets, sets...), stepName
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
	if p.refused != "" {
		b.WriteString("\n" + s.Warn(p.refused) + "\n")
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
	back := "esc back"
	if p.step == stepMenu {
		back = "esc quit"
	}
	b.WriteString("\n" + s.Dim("↑/↓ move  enter select  "+back+"  q quit without writing") + "\n")
	return tea.NewView(p.fit(b.String()))
}

// fit cuts each line to the terminal's width, so a long model id or command
// cannot wrap and break the layout.
func (p picker) fit(view string) string {
	if p.width <= 0 {
		return view
	}
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, p.width, "…")
	}
	return strings.Join(lines, "\n")
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
	case opt == reset:
		return s.Caution(opt)
	case (p.step == stepName || p.step == stepRoleTier) && slices.Contains(Tiers, opt),
		p.step == stepRoleTier && opt == Session:
		return s.Color(tierHue(opt), opt)
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
	case stepTierClient, stepRoleTier:
		if p.step == stepRoleTier && opt == Session {
			return s.Dim("run on the session's own client, model, and effort")
		}
		if opt == reset {
			c := p.applied()
			if p.step == stepTierClient {
				delete(c.Tiers, p.name)
				b, _ := c.binding(p.name)
				model, _ := c.Model(p.name, b.Client, p.catalog)
				return s.Dim("back to preset " + c.preset() + ": " + b.Client + " › " + cmp.Or(model, "unavailable"))
			}
			delete(c.Roles, p.name)
			r := c.Resolve(p.name, Launch{}, p.catalog)
			return s.Dim("back to its default: " + r.Tier + ", " + r.Client + " › " + cmp.Or(r.Model, "unavailable"))
		}
	case stepName:
		c := p.applied()
		_, tierSet := c.Tiers[opt]
		_, roleSet := c.Roles[opt]
		mark := ""
		if tierSet || roleSet {
			mark = "  " + s.Accent("set")
		}
		switch {
		case slices.Contains(Tiers, opt):
			b, _ := c.binding(opt)
			model, _ := c.Model(opt, b.Client, p.catalog)
			return s.Dim(ui.Pad("", 10)+ui.Pad(b.Client, 6)+" › "+cmp.Or(model, "unavailable")) + mark
		case opt == "save":
			if len(p.sets) == 1 {
				return s.Dim("write 1 pending change")
			}
			return s.Dim(fmt.Sprintf("write %d pending changes", len(p.sets)))
		}
		r := c.Resolve(opt, Launch{}, p.catalog)
		return s.Dim(ui.Pad(r.Tier, 10)+ui.Pad(r.Client, 6)+" › "+cmp.Or(r.Model, "unavailable")) + mark
	case stepTierModel:
		if opt == newest {
			return p.newestDetail()
		}
	}
	return ""
}

// newestDetail says what a role at the tier runs on once it is served by the
// client's newest model: that model, the fallback when the client lists none
// for the tier, or that nothing can run.
func (p picker) newestDetail() string {
	s := p.style
	next, err := change(p.applied(), []string{"tier", "set", p.name, "--client", p.client}, p.catalog)
	if err != nil {
		return s.Caution(err.Error())
	}
	m, _ := next.underPreset(p.name, next.preset(), p.catalog)
	switch {
	case m.Problem != "":
		return s.Caution("none listed for " + p.name)
	case m.Tier != "":
		return s.Dim("falls back to " + m.Tier + ": " + m.Client + " › " + m.Model)
	}
	return s.Dim("now " + m.Model)
}

// presetPreview is what each tier would run on under a preset.
func (p picker) presetPreview(preset string) []string {
	s := p.style
	var pairs [][2]string
	for _, tier := range Tiers {
		m, alternatives := p.applied().underPreset(tier, preset, p.catalog)
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
