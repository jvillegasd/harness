package models

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/jvillegasd/harness/internal/ui"
)

// This file is what `show`, `tier show`, `role show`, and `role resolve`
// print: one report, drawn with the ui design system or encoded as JSON.

type tierView struct {
	Tier    string `json:"tier"`
	Client  string `json:"client"`
	Model   string `json:"model"`
	Source  string `json:"source"`
	Problem string `json:"problem,omitempty"`
	// Fallback is the tier its roles fall back to when its model is not listed.
	Fallback string `json:"fallback,omitempty"`
	// Others is what each other client runs at this tier, for a role set to
	// that client; a client with no model for the tier is left out.
	Others []clientModel `json:"others"`
	// Presets is what serves this tier under each preset.
	Presets map[string]clientModel `json:"presets"`
	// alternatives is each preset's other models on the same client, such as fable beside opus.
	alternatives map[string][]string
}

type clientModel struct {
	Client string `json:"client"`
	Model  string `json:"model"`
	// Tier is the tier it falls back to, when the client lists no model for this one.
	Tier string `json:"tier,omitempty"`
	// Problem says why the model cannot run, as role resolve would.
	Problem string `json:"problem,omitempty"`
}

// others lists every model a role set to a tier may run on besides the one
// that serves it: each client's model for the tier, falling back as Resolve
// does, and each other Claude alias made for the tier, such as fable for heavy.
func (c Choice) others(tier string, served clientModel, cat *Catalog) []clientModel {
	out := []clientModel{}
	add := func(m clientModel) {
		if (m.Client != served.Client || m.Model != served.Model) && !slices.Contains(out, m) {
			out = append(out, m)
		}
	}
	for _, client := range Clients {
		at := tier
		model, err := c.Model(at, client, cat)
		for next, ok := fallback[at]; ok && errors.Is(err, ErrNotListed); next, ok = fallback[at] {
			at = next
			model, err = c.Model(at, client, cat)
		}
		if err != nil {
			continue
		}
		m := clientModel{Client: client, Model: model}
		if at != tier {
			m.Tier = at
		}
		add(m)
		if client == "claude" {
			for _, alias := range slices.Sorted(maps.Keys(claudeTiers)) {
				if claudeTiers[alias] == tier {
					add(clientModel{Client: client, Model: alias})
				}
			}
		}
	}
	return out
}

type roleView struct {
	Role    string `json:"role"`
	Tier    string `json:"tier"`
	Client  string `json:"client"`
	Model   string `json:"model"`
	Effort  string `json:"effort"`
	Source  string `json:"source"`
	Note    string `json:"note"`
	Problem string `json:"problem,omitempty"`
	generic bool
}

type report struct {
	Preset string     `json:"preset"`
	Tiers  []tierView `json:"tiers"`
	Roles  []roleView `json:"roles"`
}

func (c Choice) report(cat *Catalog) report {
	out := report{Preset: c.preset()}
	for _, tier := range Tiers {
		b, source := c.binding(tier)
		model, err := c.Model(tier, b.Client, cat)
		v := tierView{Tier: tier, Client: b.Client, Model: cmp.Or(model, "unavailable"), Source: source, Problem: problem(err)}
		if next, ok := fallback[tier]; ok && errors.Is(err, ErrNotListed) {
			v.Fallback = next
		}
		v.Others = c.others(tier, clientModel{Client: b.Client, Model: model}, cat)
		v.Presets, v.alternatives = map[string]clientModel{}, map[string][]string{}
		for _, name := range presetNames() {
			v.Presets[name], v.alternatives[name] = c.underPreset(tier, name, cat)
		}
		out.Tiers = append(out.Tiers, v)
	}
	for _, name := range AllRoles {
		out.Roles = append(out.Roles, view(c.Resolve(name, Launch{}, cat)))
	}
	return out
}

// underPreset is what a role at the tier runs on under a preset, resolved as
// Resolve does: when the client lists no model it falls back to the next
// tier, served by that tier's own binding. A direct model also returns its
// client's other models for the tier; a fallback runs at another tier, so it
// returns none.
func (c Choice) underPreset(tier, preset string, cat *Catalog) (clientModel, []string) {
	under := c
	under.Preset = preset
	at := tier
	b, _ := under.binding(at)
	model, err := under.Model(at, b.Client, cat)
	for next, ok := fallback[at]; ok && errors.Is(err, ErrNotListed); next, ok = fallback[at] {
		at = next
		b, _ = under.binding(at)
		model, err = under.Model(at, b.Client, cat)
	}
	m := clientModel{Client: b.Client, Model: cmp.Or(model, "unavailable"), Problem: problem(err)}
	if at != tier {
		m.Tier = at
		return m, nil
	}
	return m, sameClient(under.others(tier, clientModel{Client: b.Client, Model: model}, cat), b.Client)
}

// sameClient is the models in others that run on client.
func sameClient(others []clientModel, client string) []string {
	var out []string
	for _, o := range others {
		if o.Client == client && o.Tier == "" {
			out = append(out, o.Model)
		}
	}
	return out
}

func view(r Resolved) roleView {
	return roleView{Role: r.Role, Tier: r.Tier, Client: r.Client, Model: cmp.Or(r.Model, "unavailable"), Effort: cmp.Or(r.Effort, "inherit"), Source: r.Source, Note: r.Note, Problem: problem(r.Err), generic: role(r.Role).Generic}
}

// problem says why a model is missing: not listed, or a catalog that could not be read.
func problem(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrNotListed):
		return err.Error() + "; install renders the client default"
	}
	return "unverified: " + err.Error()
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// drawTree draws a card per tier, then each tier's roles hanging under its pill.
func drawTree(s ui.Style, rep report, w io.Writer) {
	fmt.Fprintln(w, s.Title("harness models", "preset "+rep.Preset, "~/.harness/state.json"))
	fmt.Fprintln(w)
	fmt.Fprintln(w, s.Title("tier defaults", "preset "+rep.Preset))
	drawCards(s, rep, w)
	fmt.Fprintln(w)
	fmt.Fprintln(w, s.Title("roles", "the model each role runs on"))
	for i, t := range rep.Tiers {
		fmt.Fprintln(w)
		fmt.Fprintln(w, s.Pill(ui.Hue(i), t.Tier, 8))
		var members []roleView
		for _, r := range rep.Roles {
			if r.Tier == t.Tier {
				members = append(members, r)
			}
		}
		if len(members) == 0 {
			fmt.Fprintln(w, s.Branch(true)+s.Dim("no roles; the worker on request"))
		}
		for j, r := range members {
			last := j == len(members)-1
			fmt.Fprintln(w, strings.TrimRight(s.Branch(last)+drawMember(s, r), " "))
			for _, p := range memberProblems(r) {
				if p != t.Problem {
					fmt.Fprintln(w, s.Stem(last)+s.Warn(p))
				}
			}
		}
	}
}

// tierWork is what each tier's card says the tier is for.
var tierWork = map[string]string{
	"heavy":    "system design, architecture",
	"high":     "defects, engineering, analysis",
	"standard": "scoped work with tests",
	"fast":     "classification, bulk reads, boilerplate",
}

// drawCards draws one card per tier: what it is for, what runs it, and what
// each other preset would run it on.
func drawCards(s ui.Style, rep report, w io.Writer) {
	for i, t := range rep.Tiers {
		fmt.Fprintln(w)
		fmt.Fprintln(w, s.Pill(ui.Hue(i), t.Tier, 8)+"  "+s.Dim(tierWork[t.Tier]))
		// The tier's own problem is its warning line below, so runs on does not mark it again.
		active := t.Presets[rep.Preset]
		active.Problem = ""
		runs := drawChoice(s, active, t.alternatives[rep.Preset])
		if t.Source == "set" {
			runs += "  " + s.Accent("set")
		}
		pairs := [][2]string{{"runs on", runs}}
		for _, name := range presetNames() {
			if name != rep.Preset {
				pairs = append(pairs, [2]string{"with " + name, drawChoice(s, t.Presets[name], t.alternatives[name])})
			}
		}
		lines := s.Fields(pairs...)
		fmt.Fprintln(w, lines[0])
		if t.Problem != "" {
			fmt.Fprintln(w, "  "+s.Warn(tierProblem(t)))
		}
		for _, line := range lines[1:] {
			fmt.Fprintln(w, line)
		}
	}
}

// drawChoice is `client › model`, with the client's other models for the tier dim after it.
func drawChoice(s ui.Style, m clientModel, alternatives []string) string {
	text := s.Resolved(m.Client, m.Model)
	if m.Tier != "" {
		text += " " + s.Dim("at "+m.Tier)
	}
	if m.Problem != "" {
		text += " " + s.Caution("not runnable")
	}
	for _, alt := range alternatives {
		text += s.Dim(" or " + alt)
	}
	return text
}

func tierProblem(t tierView) string {
	if t.Fallback != "" {
		return strings.TrimSuffix(t.Problem, "; install renders the client default") + "; roles at " + t.Tier + " fall back to " + t.Fallback
	}
	return t.Problem
}

// drawMember is a role under its tier: its name, effort, the model it runs
// on, and where it came from.
func drawMember(s ui.Style, r roleView) string {
	line := ui.Pad(r.Role, 13) + " " + s.Dim("effort") + " " + drawEffort(s, r.Effort)
	line += "  " + s.Resolved(r.Client, r.Model)
	if a := annotation(s, r); a != "" {
		line += "  " + a
	}
	return line
}

func drawEffort(s ui.Style, effort string) string {
	if effort == "inherit" {
		return s.Dim(ui.Pad("default", 7))
	}
	return ui.Pad(effort, 7)
}

func annotation(s ui.Style, r roleView) string {
	switch {
	case r.Source == "override":
		return s.Accent("set")
	case r.Source == "reviewer":
		return s.Dim("via reviewer")
	case r.Source == "launch":
		return s.Dim("at launch")
	case r.generic:
		return s.Dim("tier chosen at launch")
	}
	return ""
}

func memberProblems(r roleView) []string {
	var out []string
	if r.Note != "" {
		out = append(out, r.Note)
	}
	if r.Problem != "" {
		out = append(out, r.Problem)
	}
	return out
}

// drawTiers draws the tier cards alone, for `tier show`.
func drawTiers(s ui.Style, rep report, w io.Writer) {
	fmt.Fprintln(w, s.Title("harness models tier", "preset "+rep.Preset))
	drawCards(s, rep, w)
}

// drawRoles draws one row per role, for `role show` and `role resolve`.
func drawRoles(s ui.Style, roles []roleView, w io.Writer) {
	for _, r := range roles {
		i := slices.Index(Tiers, r.Tier)
		line := ui.Pad(r.Role, 13) + " " + s.Color(ui.Hue(i), ui.Pad(r.Tier, 9)) + " " + s.Resolved(r.Client, ui.Pad(r.Model, 14)) + "  " + s.Dim("effort") + " " + drawEffort(s, r.Effort)
		if a := annotation(s, r); a != "" {
			line += "  " + a
		}
		fmt.Fprintln(w, strings.TrimRight(line, " "))
		for _, p := range memberProblems(r) {
			fmt.Fprintln(w, "  "+s.Warn(p))
		}
	}
}
