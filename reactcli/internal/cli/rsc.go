package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/malcolmston/reactcli/internal/project"
	"github.com/malcolmston/reactcli/internal/rsccompile"
	"github.com/malcolmston/reactcli/internal/scaffold"
	"github.com/malcolmston/reactcli/internal/ui"
)

func newRSCCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "rsc",
		Short: "Compile client components into Go references and a browser entry",
		Long: `Compile client components into Go references and a browser entry.

Scans the client directory for modules marked "use client", then writes two
files that have to agree about them:

  <entry package>/rsc_gen.go   Go declarations the server references them by
  <out>/rsc/entry.js           the browser module that loads and mounts them

Both are generated, so renaming a component cannot leave a stale string behind —
the failure mode this replaces is a blank component at runtime with no build
error anywhere.

This runs automatically as part of ` + "`react build`" + ` and ` + "`react dev`" + `.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := requireProject()
			if err != nil {
				return err
			}
			printer.Banner("compiling client components")
			if err := compileRSC(m, true); err != nil {
				return err
			}
			printer.Blank()
			return nil
		},
	}
}

// compileRSC scans for client components and regenerates the bridge.
//
// verbose distinguishes an explicit `react rsc` from the silent pass that a
// build makes: a build should not narrate work the user did not ask about, but
// it must still fail on a real problem.
func compileRSC(m *project.Manifest, verbose bool) error {
	clientDir := m.Path(m.Client)

	result, err := rsccompile.Scan(clientDir, "/"+m.Client)
	if err != nil {
		return err
	}

	for _, w := range result.Warnings {
		printer.Warn("%s", w)
	}

	opts := rsccompile.Options{
		Package:  filepath.Base(m.Path(m.Entry)),
		RSCPath:  m.RSCPath,
		MountID:  m.MountID,
		EntryURL: "/rsc/entry.js",
	}
	if opts.Package == "." || opts.Package == string(filepath.Separator) {
		opts.Package = "app"
	}

	exports := result.Exports()

	// The generated Go file lives beside the components that reference it. The
	// app package is the convention the scaffold sets up; a project that moved
	// it says so in the manifest.
	goPath := m.Path(filepath.Join("app", "rsc_gen.go"))
	opts.Package = "app"

	source, err := rsccompile.GenerateGo(exports, opts)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(goPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(goPath, []byte(source), 0o644); err != nil {
		return err
	}

	entry, err := rsccompile.GenerateEntry(opts)
	if err != nil {
		return err
	}
	entryPath := filepath.Join(m.OutDir(), "rsc", "entry.js")
	if err := os.MkdirAll(filepath.Dir(entryPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(entryPath, []byte(entry), 0o644); err != nil {
		return err
	}

	// Client modules are served as-is: they are plain ES modules, so copying is
	// the whole build step.
	if _, err := os.Stat(clientDir); err == nil {
		if err := copyTree(clientDir, filepath.Join(m.OutDir(), m.Client)); err != nil {
			return fmt.Errorf("copying client components: %w", err)
		}
	}

	if verbose {
		if len(exports) == 0 {
			printer.Note("no client components found in %s", ui.File(m.Client))
		}
		for _, e := range exports {
			printer.Success("%s %s %s", ui.Accent(e.GoName), ui.Dim("←"), ui.File(e.Module.Rel))
		}
		printer.Success("wrote %s", ui.File("app/rsc_gen.go"))
		printer.Success("wrote %s", ui.File(filepath.Join(m.Out, "rsc", "entry.js")))
	}
	return nil
}

func newInitCommand() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Add React Server Components to this project",
		Long: `Add React Server Components to this project.

Where ` + "`react new`" + ` creates a project that renders static HTML, this wires up the
interactive half: the browser loads a real React runtime, fetches the tree your
Go code renders, and mounts it — so client components keep their state and their
event handlers while everything else stays server-rendered Go.

It creates a client directory with a working component, records the RSC settings
in react.json, and runs the compiler once. Nothing is downloaded and no
JavaScript toolchain is required: client components are plain ES modules and
React is loaded from an ES module host.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := requireProject()
			if err != nil {
				return err
			}

			printer.Banner("adding server components to " + ui.Accent(m.Name))

			if m.Client == "" {
				m.Client = "client"
			}
			if m.RSCPath == "" {
				m.RSCPath = "/rsc"
			}
			if m.MountID == "" {
				m.MountID = "root"
			}
			if err := m.Save(); err != nil {
				return err
			}
			printer.Success("recorded RSC settings in %s", ui.File(project.ManifestName))

			sample := m.Path(filepath.Join(m.Client, "Counter.js"))
			if _, err := os.Stat(sample); os.IsNotExist(err) || force {
				if err := os.MkdirAll(filepath.Dir(sample), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(sample, []byte(scaffold.SampleClientComponent), 0o644); err != nil {
					return err
				}
				printer.Success("created %s", ui.File(filepath.Join(m.Client, "Counter.js")))
			} else {
				printer.Note("%s already exists; leaving it alone",
					ui.File(filepath.Join(m.Client, "Counter.js")))
			}

			if err := compileRSC(m, true); err != nil {
				return err
			}

			printer.Blank()
			printer.Step("wire it up:")
			printer.Note("1. serve the payload:  rsc.Render(w, tree)  with Content-Type: text/x-component")
			printer.Note("2. add to <head>:      react.Frag(app.ClientHead()...)")
			printer.Note("3. add to <body>:      react.H(\"div\", react.Props{\"id\": app.MountID})")
			printer.Note("4. render a component: app.Counter.El(react.Props{\"start\": 0})")
			printer.NextSteps("react dev")
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "overwrite the sample client component if it exists")
	return cmd
}
