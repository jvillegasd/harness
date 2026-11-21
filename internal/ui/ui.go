// Package ui is the harness CLI's design system. Every command that prints
// for a person draws with it, so the commands look like one tool:
//
//   - A title line: the command in bold, then its context dim (Title).
//   - A group is a colored pill (Pill), one color per group in a fixed order
//     (Hue), followed by what the group resolves to: `client › model`.
//   - A group's members hang under it as a tree (Branch), name first, then
//     their settings, then dim annotations saying where a value came from.
//   - Metadata is dim. A value the human set is accented ("set"). Problems
//     are a `!` line in the warning color under what they affect (Warn);
//     a finished action is an `ok` line (OK).
//   - A flat list of records is a Table: aligned columns, a dim header,
//     the first column as the record's name. A record's details are Fields.
//   - A failed check is an `x` line in red (Fail); a passed one is `ok`.
//   - Color and weight appear only on a terminal and never under NO_COLOR;
//     the layout is the same either way. Scripts read `--json`, never this.
//
// What a program or agent reads as a value stays out of the design: JSON, a
// printed path or digest, a forge comment, a written file, and hook output.
package ui

import (
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
)

// Style draws text for one writer, in color only when it is a terminal.
type Style struct{ color bool }

// For returns the style for w: color when w is a terminal and NO_COLOR is unset.
func For(w io.Writer) Style {
	f, ok := w.(*os.File)
	return Style{color: ok && os.Getenv("NO_COLOR") == "" && term.IsTerminal(f.Fd())}
}

// Plain is the style without color, for tests and non-terminal output.
var Plain = Style{}

// Colored is the style with color, for tests of the colored form.
var Colored = Style{color: true}

const (
	blue    = "34"
	cyan    = "36"
	yellow  = "33"
	magenta = "35"
	green   = "32"
)

// hues is the fixed order of group colors.
var hues = []string{blue, cyan, yellow, magenta, green}

// Hue is the color of the i-th group, cycling through the order.
type Hue int

func (s Style) sgr(code, text string) string {
	if !s.color || text == "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func (s Style) Bold(text string) string   { return s.sgr("1", text) }
func (s Style) Dim(text string) string    { return s.sgr("2", text) }
func (s Style) Accent(text string) string { return s.sgr("1;34", text) }

// Color draws text in the group's hue.
func (s Style) Color(h Hue, text string) string { return s.sgr(hues[int(h)%len(hues)], text) }

// Pill draws a group's label, padded to width, on its hue.
func (s Style) Pill(h Hue, label string, width int) string {
	text := " " + Pad(strings.ToUpper(label), width) + " "
	return s.sgr("1;30;4"+hues[int(h)%len(hues)][1:], text)
}

// Title is a command's title line: the name in bold and its context dim.
func (s Style) Title(name string, context ...string) string {
	line := s.Bold(name)
	for _, c := range context {
		line += "  " + s.Dim(c)
	}
	return line
}

// Resolved draws what a group resolves to: `client › model`.
func (s Style) Resolved(client, model string) string {
	return Pad(client, 6) + " " + s.Dim("›") + " " + s.Bold(model)
}

// Branch is the tree prefix of a member: the last one closes the branch.
func (s Style) Branch(last bool) string {
	if last {
		return "  " + s.Dim("└─") + " "
	}
	return "  " + s.Dim("├─") + " "
}

// Stem is the prefix of a line under a member, keeping the branch open.
func (s Style) Stem(last bool) string {
	if last {
		return "     "
	}
	return "  " + s.Dim("│") + "  "
}

// Warn is a problem line.
func (s Style) Warn(text string) string { return s.sgr("33", "!") + " " + text }

// OK is a finished-action line.
func (s Style) OK(text string) string { return s.sgr("32", "ok") + " " + s.Dim(text) }

// Fail is a failed-check line.
func (s Style) Fail(text string) string { return s.sgr("31", "x") + " " + text }

// Bad draws text in the failure color, for a value that is a failure.
func (s Style) Bad(text string) string { return s.sgr("31", text) }

// Good draws text in the success color, for a value that is a success.
func (s Style) Good(text string) string { return s.sgr("32", text) }

// Caution draws text in the warning color, for a value that needs attention.
func (s Style) Caution(text string) string { return s.sgr("33", text) }

// Table aligns rows into columns two spaces apart, the header dim when given.
// Cells may already be styled; widths count display columns only.
func (s Style) Table(header []string, rows [][]string) []string {
	all := rows
	if header != nil {
		dimmed := make([]string, len(header))
		for i, h := range header {
			dimmed[i] = s.Dim(strings.ToUpper(h))
		}
		all = append([][]string{dimmed}, rows...)
	}
	var widths []int
	for _, row := range all {
		for i, cell := range row {
			if i >= len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], ansi.StringWidth(cell))
		}
	}
	out := make([]string, 0, len(all))
	for _, row := range all {
		var b strings.Builder
		for i, cell := range row {
			if i < len(row)-1 {
				cell = Pad(cell, widths[i]) + "  "
			}
			b.WriteString(cell)
		}
		out = append(out, strings.TrimRight(b.String(), " "))
	}
	return out
}

// Fields are a record's details: dim labels aligned, values after them.
func (s Style) Fields(pairs ...[2]string) []string {
	width := 0
	for _, p := range pairs {
		width = max(width, ansi.StringWidth(p[0]))
	}
	out := make([]string, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, "  "+s.Dim(Pad(p[0], width))+"  "+p[1])
	}
	return out
}

// Pad pads text with spaces to width display columns; styled text counts
// only what shows.
func Pad(text string, width int) string {
	if n := ansi.StringWidth(text); n < width {
		return text + strings.Repeat(" ", width-n)
	}
	return text
}
