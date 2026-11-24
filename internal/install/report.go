package install

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jvillegasd/harness/internal/models"
	"github.com/jvillegasd/harness/internal/ui"
)

// This file is the install and uninstall log, drawn with the ui design
// system: one section per phase (a pill and what it covers), its steps as a
// tree while they run, file changes counted per component and listed one by
// one only with --verbose, warnings as `!` lines on stderr under the section
// they belong to, and an `ok` line closing each section.

// fileVerbs is the order a section's file counts are reported in.
var fileVerbs = []string{"installed", "removed", "kept", "unchanged"}

type logbook struct {
	section string
	hue     int
	// files counts each component's file changes by verb.
	files map[string]map[string]int
	// deferring holds output until its component section opens.
	deferring bool
	component string
	deferred  []heldLine
	warnings  int
}

type heldLine struct {
	component, text string
	warning         bool
}

func (in *installer) style() ui.Style { return ui.For(in.stdout) }

// begin closes the open section and opens the next one.
func (in *installer) begin(name, detail string) {
	in.end("done")
	s := in.style()
	fmt.Fprintln(in.stdout)
	fmt.Fprintln(in.stdout, strings.TrimRight(s.Pill(ui.Hue(in.book.hue), name, 8)+"  "+s.Dim(detail), " "))
	in.book.hue++
	in.book.section = name
}

// end closes the open section with an ok line, if one is open.
func (in *installer) end(result string) {
	if in.book.section == "" {
		return
	}
	s := in.style()
	fmt.Fprintln(in.stdout, s.Branch(true)+s.OK(result))
	in.book.section = ""
}

// log is one step: a tree line inside a section, an ok line outside one.
func (in *installer) log(format string, args ...any) {
	if in.book.deferring {
		in.hold(in.book.component, false, fmt.Sprintf(format, args...))
		return
	}
	s, text := in.style(), in.tilde(fmt.Sprintf(format, args...))
	if in.book.section == "" {
		fmt.Fprintln(in.stdout, s.OK(text))
		return
	}
	fmt.Fprintln(in.stdout, s.Branch(false)+text)
}

// warn is a problem, under the open section when there is one.
func (in *installer) warn(format string, args ...any) {
	if in.book.deferring {
		in.hold(in.book.component, true, fmt.Sprintf(format, args...))
		return
	}
	in.book.warnings++
	s, prefix := ui.For(in.stderr), ""
	if in.book.section != "" {
		prefix = s.Branch(false)
	}
	fmt.Fprintln(in.stderr, prefix+s.Warn(fmt.Sprintf(format, args...)))
}

// tilde shortens paths under the home directory to ~/ in progress lines;
// warnings keep full paths, since the human acts on them.
func (in *installer) tilde(text string) string {
	if in.home == "" {
		return text
	}
	return strings.ReplaceAll(text, in.home+"/", "~/")
}

// warnFor is a problem with one component's files: held until that
// component's section is drawn while files are being written.
func (in *installer) warnFor(component, format string, args ...any) {
	if in.book.deferring {
		in.hold(component, true, fmt.Sprintf(format, args...))
		return
	}
	in.warn(format, args...)
}

func (in *installer) hold(component string, warning bool, text string) {
	in.book.deferred = append(in.book.deferred, heldLine{component, text, warning})
}

// flush draws matching lines in their original order.
func (in *installer) flush(components ...string) {
	var remaining []heldLine
	for _, line := range in.book.deferred {
		if !slices.Contains(components, line.component) {
			remaining = append(remaining, line)
			continue
		}
		if line.warning {
			in.warn("%s", line.text)
		} else {
			in.log("%s", line.text)
		}
	}
	in.book.deferred = remaining
}

// flushOnPanic preserves output already produced before a fatal error.
func (in *installer) flushOnPanic() {
	if r := recover(); r != nil {
		in.book.deferring = false
		in.book.section = ""
		for _, line := range in.book.deferred {
			if line.warning {
				in.warn("%s", line.text)
			} else {
				in.log("%s", line.text)
			}
		}
		in.book.deferred = nil
		panic(r)
	}
}

// file counts one file change, and with --verbose shows it.
func (in *installer) file(component, verb, path string) {
	if in.book.files == nil {
		in.book.files = map[string]map[string]int{}
	}
	if in.book.files[component] == nil {
		in.book.files[component] = map[string]int{}
	}
	in.book.files[component][verb]++
	if in.verbose && verb != "unchanged" {
		if in.book.deferring {
			in.hold(component, false, fmt.Sprintf("%s %s", verb, path))
		} else {
			in.log("%s %s", verb, path)
		}
	}
}

// counted is the components' file counts, such as "3 installed, 64 unchanged".
func (in *installer) counted(components ...string) string {
	var parts []string
	for _, verb := range fileVerbs {
		n := 0
		for _, c := range components {
			n += in.book.files[c][verb]
		}
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, verb))
		}
	}
	if len(parts) == 0 {
		return "no files"
	}
	return strings.Join(parts, ", ")
}

// drawAssignments is one line per client: the roles assigned to it, grouped
// by model and effort. A role assigned elsewhere has no agent on this client.
func (in *installer) drawAssignments() {
	s := in.style()
	assigned := in.assigned()
	for _, client := range in.clients {
		var order []string
		groups := map[string][]string{}
		for _, role := range models.AllRoles {
			a, ok := assigned[role]
			if !ok || a.Client != client || a.Model == "" {
				continue
			}
			setting := a.Model
			if a.Effort != "" {
				setting += " " + s.Dim("@") + " " + a.Effort
			}
			if groups[setting] == nil {
				order = append(order, setting)
			}
			groups[setting] = append(groups[setting], role)
		}
		line := s.Dim("no roles assigned")
		if len(order) > 0 {
			var parts []string
			for _, setting := range order {
				parts = append(parts, strings.Join(groups[setting], ", ")+" "+s.Dim("on")+" "+setting)
			}
			line = strings.Join(parts, s.Dim("  ·  "))
		}
		in.log("%s  %s", ui.Pad(client, 8), line)
	}
}

// summarize closes a run with one ok line naming the warnings, if any.
func (in *installer) summarize(text string) {
	in.end("done")
	if n := in.book.warnings; n > 0 {
		text += fmt.Sprintf(" · %d warning", n)
		if n > 1 {
			text += "s"
		}
	}
	fmt.Fprintln(in.stdout)
	fmt.Fprintln(in.stdout, in.style().OK(text))
}

// drawList draws a titled list, such as the next steps after an install.
func (in *installer) drawList(name string, items []string) {
	if len(items) == 0 {
		return
	}
	s := in.style()
	fmt.Fprintln(in.stdout)
	fmt.Fprintln(in.stdout, s.Bold(name))
	for i, item := range items {
		fmt.Fprintln(in.stdout, s.Branch(i == len(items)-1)+item)
	}
}

func numbered(items []string) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = fmt.Sprintf("%d. %s", i+1, item)
	}
	return out
}
