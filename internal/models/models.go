// Package models holds the human's model choice: which client and model
// serve each tier (tiers.go), and which tier, client, and effort each role
// runs at (roles.go), stored under the "models" key of ~/.harness/state.json.
package models

import (
	"encoding/json"
	"fmt"

	"github.com/jvillegasd/harness/internal/statefile"
)

// Choice is the stored choice: a preset and tier bindings, and role overrides.
type Choice struct {
	Preset string              `json:"preset,omitempty"`
	Tiers  map[string]Binding  `json:"tiers,omitempty"`
	Roles  map[string]Override `json:"roles,omitempty"`
}

func StatePath(home string) string { return statefile.Path(home) }

// stored reads the choice as saved, without validating or migrating it.
func stored(path string) (Choice, error) {
	doc, err := statefile.Read(path)
	if err != nil {
		return Choice{}, err
	}
	var c Choice
	if raw, ok := doc["models"]; ok {
		if err := json.Unmarshal(raw, &c); err != nil {
			return Choice{}, fmt.Errorf("%s: models: %w; run `harness models reset`", path, err)
		}
	}
	return c, nil
}

// forReset reads the choice a reset starts from: the loaded one, so a reset
// clears what a migration made, or the stored block when Load refuses it, so
// a reset can still clear a bad one.
func forReset(path string) (Choice, error) {
	if c, err := Load(path); err == nil {
		return c, nil
	}
	return stored(path)
}

// Load reads the choice, migrating what older versions saved and refusing
// what this version cannot run, each refusal naming the reset that clears it.
func Load(path string) (Choice, error) {
	c, err := stored(path)
	if err != nil {
		return Choice{}, err
	}
	if retiredPresets[c.Preset] {
		c.Preset = ""
	}
	if _, ok := Presets[c.Preset]; c.Preset != "" && !ok {
		return Choice{}, fmt.Errorf("%s: unknown preset %q; run `harness models tier reset`", path, c.Preset)
	}
	for tier, b := range c.Tiers {
		if err := checkBinding(tier, b); err != nil {
			if knownTier(tier) == nil {
				return Choice{}, fmt.Errorf("%s: %v; run `harness models tier reset %s`", path, err, tier)
			}
			return Choice{}, fmt.Errorf("%s: %v; run `harness models tier reset`", path, err)
		}
	}
	for name, ov := range c.Roles {
		if retired[name] {
			delete(c.Roles, name)
			continue
		}
		if err := knownRole(name); err != nil {
			return Choice{}, fmt.Errorf("%s: %v; run `harness models role reset`", path, err)
		}
		if ov.Model != "" {
			if !c.migrate(name, &ov) {
				delete(c.Roles, name)
				continue
			}
			c.Roles[name] = ov
		}
		if err := checkOverride(ov); err != nil {
			return Choice{}, fmt.Errorf("%s: %s: %v; run `harness models role reset %s`", path, name, err, name)
		}
	}
	return c, nil
}

func Save(path string, c Choice) error {
	doc, err := statefile.Read(path)
	if err != nil {
		return err
	}
	if c.Preset == "" && len(c.Tiers) == 0 && len(c.Roles) == 0 {
		delete(doc, "models")
	} else {
		raw, err := json.Marshal(c)
		if err != nil {
			return err
		}
		doc["models"] = raw
	}
	return statefile.Write(path, doc)
}

func first(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
