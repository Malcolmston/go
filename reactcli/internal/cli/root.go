// Package cli assembles the command tree.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/malcolmston/reactcli/internal/project"
	"github.com/malcolmston/reactcli/internal/ui"
)

// version is stamped at build time with -ldflags "-X ...cli.version=...".
var version = "dev"

// printer is the process-wide output sink. Commands take it from here rather
// than constructing their own so that a global flag such as --quiet applies
// everywhere without being threaded through every function.
var printer = ui.New()

// Execute builds the command tree and runs it, returning the process exit code.
//
// It returns a code rather than calling os.Exit so that main stays the only
// place the process can end, and so the whole tree is testable in-process.
func Execute(ctx context.Context, args []string) int {
	root := newRootCommand()
	root.SetArgs(args)

	if err := root.ExecuteContext(ctx); err != nil {
		// Some commands are their own report: `react doctor` has already
		// printed every finding, and appending "doctor found problems" under it
		// would just be a second, less useful summary. They signal a non-zero
		// exit with ErrReported and print nothing more.
		if errors.Is(err, ErrReported) {
			return 1
		}

		// Cobra has already printed usage errors; everything else is ours to
		// report. Either way the error text itself has not been shown.
		printer.Blank()
		printer.Error("%v", err)

		if errors.Is(err, project.ErrNotFound) {
			printer.Note("run %s to create one, or cd into an existing project",
				ui.Command("react new <name>"))
		}
		printer.Blank()
		return 1
	}
	return 0
}

func newRootCommand() *cobra.Command {
	var quiet bool

	cmd := &cobra.Command{
		Use:   "react",
		Short: "Build sites with React's programming model, in Go",
		Long: fmt.Sprintf(`%s

React 19's programming model — components, hooks, context, suspense — as a Go
library, with a toolchain that keeps a project pure Go. There is no JSX, no
node_modules and no JavaScript runtime; ecosystem tools like Tailwind are wired
in as pinned standalone binaries.

The CLI orchestrates your program rather than owning it: a project is an
ordinary Go main package, so %s does everything %s does.`,
			ui.Accent("react"),
			ui.Command("go run ."),
			ui.Command("react build")),
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
	}

	cmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false,
		"suppress progress output; warnings and errors still print")
	cobra.OnInitialize(func() { printer.Quiet = quiet })

	cmd.AddCommand(
		newNewCommand(),
		newInitCommand(),
		newRSCCommand(),
		newAddCommand(),
		newInstallCommand(),
		newBuildCommand(),
		newDevCommand(),
		newDoctorCommand(),
		newGenerateCommand(),
	)
	return cmd
}

// requireProject loads the manifest for the working directory, or fails with an
// error the root handler turns into a "run react new" hint.
func requireProject() (*project.Manifest, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return project.Find(wd)
}
