// Package bench reports whether a harness change moved cost and friction by
// comparing the real session population before and after its commit date,
// reading each client's own session store. Cost and friction only; quality
// stays a human read of the transcripts.
package bench

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jvillegasd/harness/internal/checkout"
)

const usage = `Usage: harness bench --split <date> [--since <date>] [--client <name>] [--home <dir>] [--rows]

Compare the session population before and after a harness change.

  --split   Commit date of the change under test, YYYY-MM-DD. Sessions before
            it are before, on or after it after. Required.
  --since   Ignore sessions older than this date.
  --client  Limit to claude or opencode; repeatable (default: both).
  --home    Session-store root (default: the current home).
  --rows    Print one JSON object per session instead of the report.

Run it from inside the checkout, where the work patterns live in
bench/cohorts.json.
`

// Command runs `harness bench` and returns the process exit code.
func Command(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("bench", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {}
	var split, since, home string
	var rows bool
	var clients clientList
	flags.StringVar(&split, "split", "", "commit date of the harness change under test")
	flags.StringVar(&since, "since", "", "ignore sessions older than this date")
	flags.StringVar(&home, "home", homeDir(), "session-store root")
	flags.BoolVar(&rows, "rows", false, "print one JSON object per session instead of the report")
	flags.Var(&clients, "client", "limit to one client; repeatable")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage)
			return 0
		}
		fmt.Fprint(stderr, usage)
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "harness bench: unexpected argument %q; run harness bench --help\n", flags.Arg(0))
		return 2
	}
	if split == "" {
		fmt.Fprintln(stderr, "harness bench: --split is required")
		return 2
	}
	splitDate, err := parseDate(split)
	if err != nil {
		fmt.Fprintf(stderr, "harness bench: --split: %v\n", err)
		return 2
	}
	var sinceDate *time.Time
	if since != "" {
		date, err := parseDate(since)
		if err != nil {
			fmt.Fprintf(stderr, "harness bench: --since: %v\n", err)
			return 2
		}
		sinceDate = &date
	}
	if len(clients) == 0 {
		clients = clientList{"claude", "opencode"}
	}
	c, err := checkout.Find(".")
	if err != nil {
		fmt.Fprintf(stderr, "harness bench: %v\n", err)
		return 1
	}
	sessions, err := collect(home, clients, patterns(c, stderr))
	if err != nil {
		fmt.Fprintf(stderr, "harness bench: %v\n", err)
		return 1
	}
	if rows {
		if sinceDate != nil {
			sessions = filterSince(sessions, *sinceDate)
		}
		fmt.Fprint(stdout, renderRows(sessions))
		return 0
	}
	fmt.Fprintln(stdout, renderReport(sessions, splitDate, sinceDate))
	return 0
}

// clientList is the repeatable --client flag, restricted to the supported readers.
type clientList []string

func (c *clientList) String() string { return strings.Join(*c, ",") }

func (c *clientList) Set(value string) error {
	if value != "claude" && value != "opencode" {
		return fmt.Errorf("invalid client %q (want claude or opencode)", value)
	}
	*c = append(*c, value)
	return nil
}

func homeDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return os.Getenv("HOME")
}

func parseDate(value string) (time.Time, error) {
	day, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC), nil
}
