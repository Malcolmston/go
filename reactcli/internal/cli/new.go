package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/malcolmston/reactcli/internal/project"
	"github.com/malcolmston/reactcli/internal/scaffold"
	"github.com/malcolmston/reactcli/internal/ui"
)

// defaultReactVersion is the version of the react library a new project
// requires. It is a variable so a build can stamp it.
var defaultReactVersion = "v0.1.0"

func newNewCommand() *cobra.Command {
	var (
		modulePath   string
		reactVersion string
	)

	cmd := &cobra.Command{
		Use:   "new <name>",
		Short: "Create a new react project",
		Long: `Create a new react project.

The generated project is a plain Go module with a main package that renders your
component tree to HTML. Nothing about it depends on this CLI — go run . works on
a fresh checkout with no tooling installed.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			dir, err := filepath.Abs(name)
			if err != nil {
				return err
			}

			// Refuse to scaffold into a directory that already has contents.
			// Writing a project over someone's files is not recoverable, and
			// the individual-file guard in scaffold.Write would leave a
			// half-created project behind on the first collision.
			if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
				return fmt.Errorf("%s already exists and is not empty", dir)
			} else if err != nil && !os.IsNotExist(err) {
				return err
			}

			if modulePath == "" {
				modulePath = filepath.Base(dir)
			}
			if reactVersion == "" {
				reactVersion = defaultReactVersion
			}

			printer.Banner(fmt.Sprintf("creating %s", ui.Accent(name)))

			files, err := scaffold.Project(scaffold.Params{
				Name:         filepath.Base(dir),
				Module:       modulePath,
				ReactVersion: reactVersion,
			})
			if err != nil {
				return err
			}

			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			if err := scaffold.Write(dir, files); err != nil {
				return err
			}
			for _, f := range files {
				printer.Success("%s", ui.File(filepath.Join(name, f.Path)))
			}

			m := project.New(dir, filepath.Base(dir))
			if err := m.Save(); err != nil {
				return err
			}
			printer.Success("%s", ui.File(filepath.Join(name, project.ManifestName)))

			printer.NextSteps(
				fmt.Sprintf("cd %s", name),
				"go mod tidy",
				"react add tailwind",
				"react dev",
			)
			return nil
		},
	}

	cmd.Flags().StringVar(&modulePath, "module", "",
		"Go module path (defaults to the project directory name)")
	cmd.Flags().StringVar(&reactVersion, "react-version", "",
		"version of github.com/malcolmston/react to require")
	return cmd
}

// goIdentifier turns an arbitrary name into something usable as a Go identifier,
// used by the component generator.
func goIdentifier(s string) string {
	var b strings.Builder
	upperNext := true
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			if upperNext {
				b.WriteRune(upper(r))
				upperNext = false
			} else {
				b.WriteRune(r)
			}
		case r >= '0' && r <= '9':
			if b.Len() == 0 {
				// A Go identifier cannot start with a digit.
				b.WriteRune('C')
			}
			b.WriteRune(r)
			upperNext = false
		default:
			upperNext = true
		}
	}
	return b.String()
}

func upper(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - 'a' + 'A'
	}
	return r
}
