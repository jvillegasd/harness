// Package changemap classifies a branch's changed files into review tiers so a
// human knows what to read. It assigns only a floor from deterministic signals:
// a model may raise a tier but never lower it.
package changemap

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

const usage = `Usage: harness change-map [--base <ref>] [--json]

Classify the files this branch changes into review tiers, lowest to highest:
mechanical, tests-docs, structural, behavioral, critical. The tier is a floor
from deterministic signals; judgment may raise it, never lower it.

  --base  Ref to compare against (default: origin/HEAD if it resolves, else
          main). Files changed since the merge base with HEAD, including
          staged, unstaged, deleted, and untracked files, are classified.
  --json  Print one JSON object instead of tab-separated lines.

Add repo-relative globs under "criticalPaths" in docs/agents/harness.json to
mark files that always need a human.
`

// File is one classified changed file.
type File struct {
	Path   string `json:"path"`
	Tier   Tier   `json:"tier"`
	Reason string `json:"reason"`
}

// Report is the JSON shape of the command's output.
type Report struct {
	Base      string       `json:"base"`
	MergeBase string       `json:"mergeBase"`
	Files     []File       `json:"files"`
	Totals    map[Tier]int `json:"totals"`
}

// Command runs `harness change-map` and returns the process exit code.
func Command(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("change-map", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {}
	var base string
	var asJSON bool
	flags.StringVar(&base, "base", "", "ref to compare against")
	flags.BoolVar(&asJSON, "json", false, "print JSON")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage)
			return 0
		}
		fmt.Fprint(stderr, usage)
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "harness change-map: unexpected argument %q; run harness change-map --help\n", flags.Arg(0))
		return 2
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "harness change-map: %v\n", err)
		return 1
	}
	root, err := Root(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "harness change-map: %v; run it inside the repository to classify\n", err)
		return 2
	}
	cfg, err := LoadConfig(root)
	if err != nil {
		fmt.Fprintf(stderr, "harness change-map: %v\n", err)
		return 2
	}
	if base == "" {
		base = DefaultBase(root)
	}
	changes, err := Changed(root, base)
	if errors.Is(err, ErrUnknownBase) {
		fmt.Fprintf(stderr, "harness change-map: %v; pass --base with a ref that exists\n", err)
		return 2
	}
	if err != nil {
		fmt.Fprintf(stderr, "harness change-map: %v\n", err)
		return 1
	}

	report := Build(changes, cfg)
	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintf(stderr, "harness change-map: %v\n", err)
			return 1
		}
		return 0
	}
	fmt.Fprint(stdout, report.Text())
	return 0
}

// Build classifies every changed file and orders the result by tier, highest first, then path.
func Build(changes Changes, cfg Config) Report {
	report := Report{Base: changes.Base, MergeBase: changes.MergeBase, Files: []File{}, Totals: map[Tier]int{}}
	for _, tier := range Tiers {
		report.Totals[tier] = 0
	}
	for _, path := range changes.Files {
		result := Classify(path, cfg)
		report.Files = append(report.Files, File{path, result.Tier, result.Reason})
		report.Totals[result.Tier]++
	}
	sort.SliceStable(report.Files, func(i, j int) bool {
		a, b := report.Files[i], report.Files[j]
		if a.Tier != b.Tier {
			return a.Tier.Rank() > b.Tier.Rank()
		}
		return a.Path < b.Path
	})
	return report
}

// Text renders one tab-separated line per file, then one totals line per tier, highest first.
func (r Report) Text() string {
	var out strings.Builder
	for _, f := range r.Files {
		fmt.Fprintf(&out, "%s\t%s\t%s\n", f.Tier, f.Path, f.Reason)
	}
	for i := len(Tiers) - 1; i >= 0; i-- {
		fmt.Fprintf(&out, "total\t%s\t%d\n", Tiers[i], r.Totals[Tiers[i]])
	}
	return out.String()
}
