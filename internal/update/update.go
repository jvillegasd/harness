// Package update coordinates explicit, channel-preserving updates of installed tools.
package update

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/jvillegasd/harness/internal/checkout"
	"github.com/jvillegasd/harness/internal/install"
	"github.com/jvillegasd/harness/internal/skills"
)

var groups = []string{"packages", "clients", "mcps", "skills", "ides"}

const usage = `Usage: harness update [clients|ides|packages|mcps|skills]

With no group, run packages, clients, mcps, skills, then ides, sequentially.
Only installed tools with recognized ownership are updated, through their existing
channel. Missing tools are skipped; ambiguous installations need manual updates.
Homebrew upgrades always name a formula or cask. IDEs are never restarted.
Failures do not stop unrelated updates; dependent actions are skipped. Exit 1
means an operation failed. Skips and manual notices do not fail the run.

  clients   Claude Code, Codex, OpenCode (Orca-managed copies are skipped)
  ides      Orca, Zed, preserving stable/preview; manual fallback when unsupported
  packages  Node, gh, uv
  mcps      Engram, agent-browser, browser runtime, existing client registrations
  skills    Update locked upstream bodies and lockfile in a clean Harness checkout

Machine groups work from any directory. A bare update outside a Harness checkout
skips skills; explicit update skills fails. Locally changed locked paths are
refused. Review skill changes before running harness install to deploy them.
This command neither installs missing tools nor updates Harness itself.
`

// Command validates the entire invocation before discovery or mutation.
func Command(args []string, stdout, stderr io.Writer) int {
	selected := groups
	if len(args) == 1 && slices.Contains([]string{"help", "--help", "-h"}, args[0]) || len(args) == 2 && slices.Contains(groups, args[0]) && slices.Contains([]string{"--help", "-h"}, args[1]) {
		fmt.Fprint(stdout, usage)
		return 0
	}
	if len(args) > 1 || len(args) == 1 && !slices.Contains(groups, args[0]) {
		fmt.Fprintln(stderr, "harness update: expected zero or one group; run harness update --help")
		return 2
	}
	if len(args) == 1 {
		selected = args
	}
	c, checkoutErr := checkout.Find(".")
	if len(args) == 1 && args[0] == "skills" && checkoutErr != nil {
		fmt.Fprintln(stderr, "harness update:", checkoutErr)
		return 1
	}
	var skillsErr error
	if slices.Contains(selected, "skills") && checkoutErr == nil {
		skillsErr = skills.CheckUpdate(c)
	}
	r := &runner{out: stdout, err: stderr}
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
						continue
					}
				}
				ok := r.update(target)
				if target.status == "" {
					outcomes[target.name] = ok
				}
				if group == "mcps" {
					r.reconcile(target, ok)
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
	clients  []string
	results  []string
}

func (r *runner) report(status, name, detail string) {
	r.results = append(r.results, fmt.Sprintf("%s: %s: %s", status, name, detail))
	if status == "failed" {
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
	cmd.Env = append(os.Environ(), "HOMEBREW_NO_AUTO_UPDATE=1")
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

func (r *runner) reconcile(t target, ready bool) {
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
	for _, client := range []string{"claude", "codex", "opencode"} {
		item := t.name + " registration in " + client
		if !slices.Contains(r.clients, client) {
			r.report("skipped", item, "client not installed at discovery")
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
