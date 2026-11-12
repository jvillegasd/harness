// Package update coordinates explicit, channel-preserving updates of installed tools.
package update

import (
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strings"

	"github.com/jvillegasd/harness/internal/checkout"
	"github.com/jvillegasd/harness/internal/install"
	"github.com/jvillegasd/harness/internal/skills"
)

// components are the ones that carry a tool or an upstream source to update, in the order a bare update runs them.
var components = []string{"packages", "clients", "mcps", "skills"}

// configOnly are the install components with nothing to update: managed config a harness install reconciles.
var configOnly = []string{"agents", "floor", "settings", "aliases"}

var allClients = []string{"claude", "codex", "opencode"}

const usage = `Usage: harness update [<component>...] [--client <name>]...

Update installed tools. Omit the components to update all of them.

Components:
  clients   Update Claude Code, Codex, and OpenCode
  packages  Update Node, gh, and uv
  mcps      Update Engram, agent-browser, and client registrations
  skills    Update locked skills in this checkout

  --client  Limit clients and mcps registrations to claude, codex, or
            opencode; repeatable. packages and skills serve every client.

Details: docs/cli.md#update-installed-tools
`

// Command validates the entire invocation before discovery or mutation.
func Command(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "help" {
		fmt.Fprint(stdout, usage)
		return 0
	}
	var selected, only []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case (arg == "--help" || arg == "-h") && i == len(args)-1:
			fmt.Fprint(stdout, usage)
			return 0
		case arg == "--client":
			if i+1 == len(args) || !slices.Contains(allClients, args[i+1]) {
				fmt.Fprintf(stderr, "harness update: --client takes one of %s; run harness update --help\n", strings.Join(allClients, ", "))
				return 2
			}
			i++
			only = append(only, args[i])
		case slices.Contains(components, arg):
			selected = append(selected, arg)
		case slices.Contains(configOnly, arg):
			fmt.Fprintf(stderr, "harness update: %s has no tool to update; run harness install %s to reconcile it\n", arg, arg)
			return 2
		default:
			fmt.Fprintf(stderr, "harness update: unknown component %q; run harness update --help\n", arg)
			return 2
		}
	}
	if selected == nil {
		selected = components
	}
	selected = slices.DeleteFunc(slices.Clone(components), func(c string) bool { return !slices.Contains(selected, c) })
	if only == nil {
		only = allClients
	}
	c, checkoutErr := checkout.Find(".")
	if len(selected) == 1 && selected[0] == "skills" && checkoutErr != nil {
		fmt.Fprintln(stderr, "harness update:", checkoutErr)
		return 1
	}
	var skillsErr error
	if slices.Contains(selected, "skills") && checkoutErr == nil {
		skillsErr = skills.CheckUpdate(c)
	}
	r := &runner{out: stdout, err: stderr, only: only}
	defer r.summary()
	// Finish discovery for every selected target before the first upgrade.
	targets := r.discover(selected)
	outcomes := map[string]bool{}
	for _, group := range selected {
		if group == "skills" {
			if checkoutErr != nil {
				r.report("skipped", "skills", checkoutErr.Error())
				continue
			}
			if ok, present := outcomes["node"]; present && !ok {
				r.report("skipped", "skills", "Node prerequisite failed")
				continue
			}
			if skillsErr != nil {
				r.report("failed", "skills", skillsErr.Error())
				continue
			}
			if skills.Command([]string{"update"}, stdout, stderr) != 0 {
				r.report("failed", "skills", "upstream update failed")
			} else {
				r.report("checked", "skills", "review reported paths before harness install")
			}
			continue
		}
		for _, target := range targets {
			if target.group == group {
				if target.name == "codex" && len(target.command) > 1 && target.command[1] == "install" {
					if ok, present := outcomes["node"]; present && !ok {
						r.report("skipped", target.name, "Node prerequisite failed")
						outcomes[target.name] = false
						continue
					}
				}
				ok := r.update(target)
				if target.status == "" {
					outcomes[target.name] = ok
				}
				if group == "mcps" {
					r.reconcile(target, ok, outcomes)
				}
			}
		}
	}
	if r.failed {
		return 1
	}
	return 0
}

type target struct {
	group, name, path, notice, status string
	command, versionCommand           []string
}
type runner struct {
	out, err io.Writer
	failed   bool
	// only are the clients --client names, or every client.
	only    []string
	clients []string
	results []string
}

func (r *runner) report(status, name, detail string) {
	r.results = append(r.results, fmt.Sprintf("%s: %s: %s", status, name, detail))
	if status == "failed" {
		fmt.Fprintf(r.err, "harness update: %s: %s\n", name, detail)
		r.failed = true
	}
}
func version(path string) string {
	if path == "" {
		return ""
	}
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
func (r *runner) update(t target) bool {
	if t.status != "" {
		r.report(t.status, t.name, t.notice)
		return false
	}
	before := t.version()
	cmd := exec.Command(t.command[0], t.command[1:]...)
	cmd.Stdout, cmd.Stderr = r.out, r.err
	if err := cmd.Run(); err != nil {
		r.report("failed", t.name, err.Error())
		return false
	}
	after := t.version()
	switch {
	case before != "" && after != "" && before != after:
		r.report("updated", t.name, before+" -> "+after)
	case before != "" && before == after:
		r.report("unchanged", t.name, after)
	default:
		r.report("checked", t.name, "command succeeded; version change unavailable")
	}
	return true
}

func (t target) version() string {
	if len(t.versionCommand) == 0 {
		return version(t.path)
	}
	out, err := exec.Command(t.versionCommand[0], t.versionCommand[1:]...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (r *runner) reconcile(t target, ready bool, outcomes map[string]bool) {
	if !ready {
		if t.name == "agent-browser" {
			r.report("skipped", "agent-browser runtime", "package update prerequisite unavailable or failed")
		}
		r.report("skipped", t.name+" registrations", "package update prerequisite unavailable or failed")
		return
	}
	if t.name == "agent-browser" {
		cmd := exec.Command(t.path, "install")
		cmd.Stdout, cmd.Stderr = r.out, r.err
		if err := cmd.Run(); err != nil {
			r.report("failed", "agent-browser runtime", err.Error())
			r.report("skipped", t.name+" registrations", "browser runtime refresh failed")
			return
		}
		r.report("checked", "agent-browser runtime", "browser runtime refreshed")
	}
	binary, err := exec.LookPath(t.name)
	if err != nil {
		r.report("failed", t.name+" registrations", "updated executable missing")
		return
	}
	for _, client := range r.only {
		item := t.name + " registration in " + client
		if !slices.Contains(r.clients, client) {
			r.report("skipped", item, "client not installed at discovery")
			continue
		}
		if ok, present := outcomes[client]; present && !ok {
			r.report("skipped", item, "client update failed or its prerequisite failed")
			continue
		}
		if err := install.ReconcileMCP(client, t.name, binary, r.out, r.err); err != nil {
			r.report("failed", item, err.Error())
		} else {
			r.report("checked", item, "registration reconciled")
		}
	}
}

func (r *runner) summary() {
	fmt.Fprintln(r.out, "Update summary:")
	for _, result := range r.results {
		fmt.Fprintln(r.out, result)
	}
}
