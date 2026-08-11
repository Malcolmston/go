package cli

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/malcolmston/reactcli/internal/analyze"
	"github.com/malcolmston/reactcli/internal/ui"
)

func newDoctorCommand() *cobra.Command {
	var strict bool

	cmd := &cobra.Command{
		Use:   "doctor [dir]",
		Short: "Check for react mistakes the compiler cannot catch",
		Long: `Check for react mistakes the compiler cannot catch.

Every check here is for code that compiles cleanly and then misbehaves:

  conditional-hook       a hook behind an if, a loop or a switch. Hook state is
                         matched by call order, so a hook that does not run on
                         every render hands every later hook the wrong slot.

  hook-in-closure        a hook inside an effect body, callback or goroutine,
                         where there is no component to own it.

  unconverted-component  a function with a component's signature declared as a
                         plain func. Dispatch is by named type, so it renders as
                         nothing at all — no error, just a missing subtree.

  missing-key            an element built in a loop with no key. Keyless siblings
                         are matched by position, so inserting at the front moves
                         every item's state onto its neighbour.

Exits non-zero when any error-level finding is reported, so it drops into CI as
is. Pass --strict to fail on warnings too.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := ""
			if len(args) == 1 {
				dir = args[0]
			} else {
				m, err := requireProject()
				if err != nil {
					return err
				}
				dir = m.Root()
			}

			abs, err := filepath.Abs(dir)
			if err != nil {
				return err
			}

			printer.Banner("checking " + ui.Accent(filepath.Base(abs)))

			result, err := analyze.Run(abs)
			if err != nil {
				return err
			}

			for _, f := range result.Findings {
				rel, relErr := filepath.Rel(abs, f.Pos.Filename)
				if relErr != nil {
					rel = f.Pos.Filename
				}
				location := fmt.Sprintf("%s:%d:%d", rel, f.Pos.Line, f.Pos.Column)

				line := fmt.Sprintf("%s  %s  %s", location, ui.Dim(f.Check), f.Message)
				if f.Severity == analyze.Error {
					printer.Error("%s", line)
				} else {
					printer.Warn("%s", line)
				}
				printer.Note("%s", f.Hint)
			}

			printer.Blank()
			errCount := result.Errors()
			warnCount := len(result.Findings) - errCount

			switch {
			case len(result.Findings) == 0:
				printer.Success("no problems found in %d file(s)", result.Files)
			default:
				printer.Step("%d error(s), %d warning(s) across %d file(s)",
					errCount, warnCount, result.Files)
			}
			printer.Blank()

			if errCount > 0 || (strict && warnCount > 0) {
				// The findings above are the report; ErrReported asks for a
				// non-zero exit without printing anything further.
				return ErrReported
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&strict, "strict", false, "treat warnings as failures")
	return cmd
}

// ErrReported is returned by a command that has already printed its own
// diagnostics and only needs the process to exit non-zero. [Execute] recognises
// it and prints nothing more.
var ErrReported = errors.New("reported")
