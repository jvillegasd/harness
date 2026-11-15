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

Classify changed files into review tiers: mechanical, tests-docs, structural,
behavioral, or critical. Deterministic signals set the minimum tier; judgment
can raise it.

  --base  Ref to compare against (default: origin/HEAD, or main if unavailable).
          Classifies staged, unstaged, deleted, and untracked files since the
          merge base with HEAD.
  --json  Print one JSON object instead of tab-separated lines.

Set repo-relative globs in "criticalPaths" in docs/agents/harness.json for
files that always need human review.
`

type File struct {
	Path   string `json:"path"`
	Tier   Tier   `json:"tier"`
	Reason string `json:"reason"`

	Files int `json:"files,omitempty"`
}

type Report struct {
	Base      string       `json:"base"`
	MergeBase string       `json:"mergeBase"`
	Files     []File       `json:"files"`
	Totals    map[Tier]int `json:"totals"`
}

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

	cfg, err = EffectiveConfig(root, changes.MergeBase)
	if err != nil {
		fmt.Fprintf(stderr, "harness change-map: %v\n", err)
		return 2
	}
	if cfg.Locked, err = LoadLocks(root); err != nil {
		fmt.Fprintf(stderr, "harness change-map: %v\n", err)
		return 2
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

func Build(changes Changes, cfg Config) Report {
	report := Report{Base: changes.Base, MergeBase: changes.MergeBase, Files: []File{}, Totals: map[Tier]int{}}
	for _, tier := range Tiers {
		report.Totals[tier] = 0
	}
	groups := map[string]int{}
	for _, path := range changes.Files {
		result := Classify(path, cfg)
		if result.Group != "" {
			if i, ok := groups[result.Group]; ok {
				report.Files[i].Files++
				continue
			}
			groups[result.Group] = len(report.Files)
			report.Files = append(report.Files, File{result.Group, result.Tier, result.Reason, 1})
			report.Totals[result.Tier]++
			continue
		}
		report.Files = append(report.Files, File{Path: path, Tier: result.Tier, Reason: result.Reason})
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

func (r Report) Text() string {
	var out strings.Builder
	for _, f := range r.Files {
		reason := f.Reason
		if f.Files > 0 {
			reason = fmt.Sprintf("%d files: %s", f.Files, reason)
		}
		fmt.Fprintf(&out, "%s\t%s\t%s\n", f.Tier, f.Path, reason)
	}
	for i := len(Tiers) - 1; i >= 0; i-- {
		fmt.Fprintf(&out, "total\t%s\t%d\n", Tiers[i], r.Totals[Tiers[i]])
	}
	return out.String()
}
