package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// This file is what each client can run on this machine: the models its
// catalog lists and the efforts each model takes.

var Clients = []string{"claude", "codex", "opencode"}

var claudeEfforts = map[string][]string{
	"fable":  {"", "low", "medium", "high", "xhigh", "max"},
	"opus":   {"", "low", "medium", "high", "xhigh", "max"},
	"sonnet": {"", "low", "medium", "high", "xhigh", "max"},
	"haiku":  {""},
}

var openaiEfforts = []string{"", "none", "minimal", "low", "medium", "high", "xhigh"}

var codexEfforts = []string{"", "low", "medium", "high", "xhigh", "max", "ultra"}

var (
	claudeModel = regexp.MustCompile(`^(fable|opus|sonnet|haiku|claude-[a-z0-9.-]+)$`)
	codexModel  = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)
)

var ErrNotListed = errors.New("not listed")

type Catalog struct {
	opencode, codex         []string
	codexEfforts            map[string][]string
	opencodeErr, codexErr   error
	opencodeDone, codexDone bool
}

func (c *Catalog) OpenCode() ([]string, error) {
	if !c.opencodeDone {
		c.opencodeDone = true
		out, err := exec.Command("opencode", "models").Output()
		if err != nil {
			c.opencodeErr = fmt.Errorf("listing OpenCode models failed: %v", err)
		} else {
			c.opencode = strings.Fields(string(out))
		}
	}
	return c.opencode, c.opencodeErr
}

func (c *Catalog) Codex() ([]string, error) {
	if !c.codexDone {
		c.codexDone = true
		c.codex, c.codexEfforts, c.codexErr = codexCatalog()
	}
	return c.codex, c.codexErr
}

func (c *Catalog) CodexEfforts(model string) []string {
	if _, err := c.Codex(); err != nil {
		return []string{""}
	}
	return append([]string{""}, c.codexEfforts[model]...)
}

func codexCatalog() ([]string, map[string][]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil, err
	}
	out, err := CodexCommand(home, "debug", "models").Output()
	if err != nil {
		return nil, nil, fmt.Errorf("listing Codex models failed: %v", err)
	}
	var catalog struct {
		Models []struct {
			Slug       string `json:"slug"`
			Visibility string `json:"visibility"`
			Levels     []struct {
				Effort string `json:"effort"`
			} `json:"supported_reasoning_levels"`
		} `json:"models"`
	}
	if err := json.Unmarshal(out, &catalog); err != nil {
		return nil, nil, fmt.Errorf("reading Codex's model catalog failed: %v", err)
	}
	var slugs []string
	efforts := map[string][]string{}
	for _, m := range catalog.Models {
		if m.Visibility != "list" {
			continue
		}
		slugs = append(slugs, m.Slug)
		for _, level := range m.Levels {
			efforts[m.Slug] = append(efforts[m.Slug], level.Effort)
		}
	}
	return slugs, efforts, nil
}

func CodexCommand(home string, args ...string) *exec.Cmd {
	cmd := exec.Command("codex", args...)
	cmd.Env = CodexEnv(home)
	return cmd
}

func CodexEnv(home string) []string {
	return append(os.Environ(), "CODEX_HOME="+filepath.Join(home, ".codex"))
}

// Runnable reports whether this machine lists the model, and the model the
// effort, when the client publishes a catalog.
func Runnable(client, model, effort string, cat *Catalog) error {
	switch client {
	case "claude":
		if efforts, ok := claudeEfforts[model]; ok && !slices.Contains(efforts, effort) {
			return fmt.Errorf("claude %s does not support effort %q", model, effort)
		}
	case "opencode":
		listed, err := cat.OpenCode()
		if err != nil {
			return err
		}
		if !slices.Contains(listed, model) {
			return fmt.Errorf("opencode has no model %q; run `opencode models`: %w", model, ErrNotListed)
		}
		if effort != "" && (!strings.HasPrefix(model, "openai/") || !slices.Contains(openaiEfforts, effort)) {
			return fmt.Errorf("opencode sends effort only as OpenAI's reasoningEffort, so %q is unsupported for %s", effort, model)
		}
	case "codex":
		listed, err := cat.Codex()
		if err != nil {
			return err
		}
		if !slices.Contains(listed, model) {
			return fmt.Errorf("codex has no model %q; run `codex debug models`: %w", model, ErrNotListed)
		}
		if effort != "" && !slices.Contains(cat.codexEfforts[model], effort) {
			return fmt.Errorf("codex %s does not support effort %q", model, effort)
		}
	}
	return nil
}

func modelShape(client, model string) error {
	switch client {
	case "claude":
		if !claudeModel.MatchString(model) {
			return fmt.Errorf("unknown Claude model %q; choose fable, opus, sonnet, haiku, or a claude- model id", model)
		}
	case "codex":
		if !codexModel.MatchString(model) {
			return fmt.Errorf("codex model %q is not a model slug; run `codex debug models`", model)
		}
	case "opencode":
		if !strings.Contains(model, "/") {
			return fmt.Errorf("opencode model %q is not a provider/model id", model)
		}
	default:
		return fmt.Errorf("unknown client %q; choose %s", client, strings.Join(Clients, ", "))
	}
	return nil
}
