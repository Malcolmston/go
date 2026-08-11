// Package ui is the CLI's output layer: every byte the user sees goes through
// here, styled with chalk.
//
// It exists so that colour is decided in exactly one place. chalk already
// disables itself when stdout is not a terminal or NO_COLOR is set, so the
// helpers below are safe to use unconditionally — piping the CLI into a file
// yields clean, unstyled text with no flag to remember.
package ui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/malcolmston/chalk"
)

// Printer writes styled output to a pair of streams. Diagnostics go to stderr
// and results to stdout, so `react render > page.html` produces a usable file
// while progress messages still reach the terminal.
type Printer struct {
	Out io.Writer
	Err io.Writer

	// Quiet suppresses progress chatter — steps, notes and the banner — while
	// leaving warnings, errors and real results alone. Scripts want this.
	Quiet bool
}

// New returns a Printer wired to the process's standard streams.
func New() *Printer { return &Printer{Out: os.Stdout, Err: os.Stderr} }

// styles are built once. A chalk Style is immutable and safe to share, so
// package-level values cost nothing and keep the palette in one place where it
// can be judged as a whole rather than a colour at a time.
var (
	styleAccent  = chalk.New().Cyan().Bold()
	styleOK      = chalk.New().Green().Bold()
	styleWarn    = chalk.New().Yellow().Bold()
	styleErr     = chalk.New().Red().Bold()
	styleDim     = chalk.New().Dim()
	styleFile    = chalk.New().Cyan()
	styleCommand = chalk.New().Magenta()
)

// Banner prints the one-line header the long-running commands open with. It is
// deliberately not printed by short commands: a tool that announces itself
// before every trivial operation becomes noise.
func (p *Printer) Banner(text string) {
	if p.Quiet {
		return
	}
	fmt.Fprintf(p.Err, "\n%s %s\n\n", styleAccent.Sprint("react"), text)
}

// Step reports the start of a unit of work.
func (p *Printer) Step(format string, args ...any) {
	if p.Quiet {
		return
	}
	fmt.Fprintf(p.Err, "  %s %s\n", styleAccent.Sprint("→"), fmt.Sprintf(format, args...))
}

// Success reports a completed unit of work.
func (p *Printer) Success(format string, args ...any) {
	if p.Quiet {
		return
	}
	fmt.Fprintf(p.Err, "  %s %s\n", styleOK.Sprint("✓"), fmt.Sprintf(format, args...))
}

// Warn reports something the user should know about but that does not stop the
// command. It ignores Quiet: a suppressed warning is a bug report waiting to
// happen.
func (p *Printer) Warn(format string, args ...any) {
	fmt.Fprintf(p.Err, "  %s %s\n", styleWarn.Sprint("!"), fmt.Sprintf(format, args...))
}

// Error reports a failure. It ignores Quiet, for the same reason as Warn.
func (p *Printer) Error(format string, args ...any) {
	fmt.Fprintf(p.Err, "  %s %s\n", styleErr.Sprint("✗"), fmt.Sprintf(format, args...))
}

// Note prints an indented secondary line under whatever came before.
func (p *Printer) Note(format string, args ...any) {
	if p.Quiet {
		return
	}
	fmt.Fprintf(p.Err, "    %s\n", styleDim.Sprint(fmt.Sprintf(format, args...)))
}

// Blank prints an empty separator line.
func (p *Printer) Blank() {
	if p.Quiet {
		return
	}
	fmt.Fprintln(p.Err)
}

// Result writes a command's actual output to stdout, unstyled and unbuffered.
// Anything a user might redirect or pipe belongs here.
func (p *Printer) Result(s string) { fmt.Fprint(p.Out, s) }

// Resultln is [Printer.Result] with a trailing newline.
func (p *Printer) Resultln(s string) { fmt.Fprintln(p.Out, s) }

// File styles a path for inclusion in a message.
func File(path string) string { return styleFile.Sprint(path) }

// Command styles a shell command the user is being told to run.
func Command(cmd string) string { return styleCommand.Sprint(cmd) }

// Dim styles de-emphasised text.
func Dim(s string) string { return styleDim.Sprint(s) }

// Accent styles a term being introduced or emphasised.
func Accent(s string) string { return styleAccent.Sprint(s) }

// NextSteps prints the closing "here is what to do now" block that scaffolding
// commands end with. Each entry is printed as a shell command, because the one
// thing a user reliably wants after a scaffold is the next thing to type.
func (p *Printer) NextSteps(steps ...string) {
	if p.Quiet || len(steps) == 0 {
		return
	}
	fmt.Fprintf(p.Err, "\n  %s\n\n", styleDim.Sprint("Next:"))
	for _, s := range steps {
		fmt.Fprintf(p.Err, "    %s\n", styleCommand.Sprint(s))
	}
	fmt.Fprintln(p.Err)
}

// Table renders aligned two-column output — the shape `react add --list` and
// `react doctor` summaries need. Widths are measured with chalk's
// [chalk.VisibleWidth] rather than len, so styled cells and CJK or emoji
// content still line up: a coloured cell carries invisible escape bytes, and a
// wide rune occupies two terminal cells while counting as one rune.
func (p *Printer) Table(rows [][2]string) {
	width := 0
	for _, r := range rows {
		if w := chalk.VisibleWidth(r[0]); w > width {
			width = w
		}
	}
	for _, r := range rows {
		pad := strings.Repeat(" ", width-chalk.VisibleWidth(r[0]))
		fmt.Fprintf(p.Err, "    %s%s   %s\n", r[0], pad, styleDim.Sprint(r[1]))
	}
}
