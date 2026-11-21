package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
}

type clientModel struct {
	Client string `json:"client"`
	Model  string `json:"model"`
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
		v := tierView{Tier: tier, Client: b.Client, Model: orUnavailable(model), Source: source, Problem: problem(err)}
		if next, ok := fallback[tier]; ok && errors.Is(err, ErrNotListed) {
			v.Fallback = next
		}
		v.Others = []clientModel{}
		for _, client := range Clients {
			if client == b.Client {
				continue
			}
			if other, err := c.Model(tier, client, cat); err == nil {
				v.Others = append(v.Others, clientModel{client, other})
			}
		}
		out.Tiers = append(out.Tiers, v)
	}
	for _, name := range AllRoles {
		out.Roles = append(out.Roles, view(c.Resolve(name, Launch{}, cat)))
	}
	return out
}

func view(r Resolved) roleView {
	return roleView{Role: r.Role, Tier: r.Tier, Client: r.Client, Model: orUnavailable(r.Model), Effort: effortOrInherit(r.Effort), Source: r.Source, Note: r.Note, Problem: problem(r.Err), generic: role(r.Role).Generic}
}

func orUnavailable(model string) string {
	if model == "" {
		return "unavailable"
	}
	return model
}

func effortOrInherit(e string) string {
	if e == "" {
		return "inherit"
	}
	return e
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

// drawTree draws every tier with the roles that run at it hanging under it.
func drawTree(s ui.Style, rep report, w io.Writer) {
	fmt.Fprintln(w, s.Title("harness models", "preset "+rep.Preset, "~/.harness/state.json"))
	for i, t := range rep.Tiers {
		fmt.Fprintln(w)
		drawTierLines(s, i, t, w)
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
			fmt.Fprintln(w, strings.TrimRight(s.Branch(last)+drawMember(s, r, t), " "))
			for _, p := range memberProblems(r) {
				if p != t.Problem {
					fmt.Fprintln(w, s.Stem(last)+s.Warn(p))
				}
			}
		}
	}
}

func drawTier(s ui.Style, i int, t tierView) string {
	return s.Pill(ui.Hue(i), t.Tier, 8) + "  " + s.Resolved(t.Client, ui.Pad(t.Model, 14)) + "  " + drawSource(s, t.Source)
}

// drawOthers lists what a role set to another client would run at the tier.
func drawOthers(s ui.Style, t tierView) string {
	if len(t.Others) == 0 {
		return ""
	}
	var parts []string
	for _, o := range t.Others {
		parts = append(parts, o.Client+" "+s.Dim("›")+" "+o.Model)
	}
	return "  " + s.Dim("also "+strings.Join(parts, s.Dim(" · ")))
}

func tierProblem(t tierView) string {
	if t.Fallback != "" {
		return strings.TrimSuffix(t.Problem, "; install renders the client default") + "; roles at " + t.Tier + " fall back to " + t.Fallback
	}
	return t.Problem
}

// drawSource says where a tier's binding came from: the human, or the preset.
func drawSource(s ui.Style, source string) string {
	if source == "set" {
		return s.Accent("set")
	}
	preset, _, _ := strings.Cut(source, ":")
	return s.Dim(preset)
}

// drawMember is a role under its tier: its name, effort, and where it came
// from, plus the client and model when they differ from the tier's.
func drawMember(s ui.Style, r roleView, t tierView) string {
	line := ui.Pad(r.Role, 13) + " " + s.Dim("effort") + " " + drawEffort(s, r.Effort)
	if r.Client != t.Client || r.Model != t.Model {
		line += "  " + s.Dim("on") + " " + s.Resolved(r.Client, r.Model)
	}
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

// drawTiers draws the tiers alone, for `tier show`.
func drawTiers(s ui.Style, rep report, w io.Writer) {
	fmt.Fprintln(w, s.Title("harness models tier", "preset "+rep.Preset))
	fmt.Fprintln(w)
	for i, t := range rep.Tiers {
		drawTierLines(s, i, t, w)
	}
}

// drawTierLines draws a tier, its problem, and what other clients run at it.
func drawTierLines(s ui.Style, i int, t tierView, w io.Writer) {
	fmt.Fprintln(w, drawTier(s, i, t))
	if t.Problem != "" {
		fmt.Fprintln(w, "  "+s.Warn(tierProblem(t)))
	}
	if others := drawOthers(s, t); others != "" {
		fmt.Fprintln(w, others)
	}
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
