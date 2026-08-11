package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/malcolmston/reactcli/internal/integration"
	"github.com/malcolmston/reactcli/internal/project"
	"github.com/malcolmston/reactcli/internal/scaffold"
	"github.com/malcolmston/reactcli/internal/ui"
)

func newAddCommand() *cobra.Command {
	var (
		list    bool
		version string
	)

	cmd := &cobra.Command{
		Use:   "add [integration]",
		Short: "Wire an ecosystem tool into this project",
		Long: `Wire an ecosystem tool into this project.

Integrations are the React ecosystem's tooling, adapted to a Go project. Each one
installs a pinned, self-contained binary under .react/bin — no package.json, no
node_modules, no JavaScript runtime.

The resolved version is written to react.json, so a clone of this project builds
with exactly the tool version it was developed against.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if list || len(args) == 0 {
				printer.Banner("available integrations")
				rows := make([][2]string, 0)
				for _, i := range integration.All() {
					rows = append(rows, [2]string{ui.Accent(i.Name()), i.Summary()})
				}
				printer.Table(rows)
				printer.Blank()
				printer.Note("add one with %s", ui.Command("react add <name>"))
				printer.Blank()
				return nil
			}

			name := args[0]
			target, ok := integration.Lookup(name)
			if !ok {
				return fmt.Errorf("unknown integration %q; run `react add --list` to see what is available", name)
			}

			m, err := requireProject()
			if err != nil {
				return err
			}

			printer.Banner(fmt.Sprintf("adding %s", ui.Accent(name)))

			if name == "tailwind" {
				integration.TailwindVersion = version
			}

			entry, err := target.Install(cmd.Context(), integration.Env{Manifest: m, Printer: printer})
			if err != nil {
				return err
			}

			m.Integrations[name] = entry
			if err := m.Save(); err != nil {
				return err
			}
			printer.Success("recorded in %s", ui.File(project.ManifestName))

			if err := regenerateIntegrations(m); err != nil {
				return err
			}

			printer.NextSteps("react dev")
			return nil
		},
	}

	cmd.Flags().BoolVar(&list, "list", false, "list available integrations")
	cmd.Flags().StringVar(&version, "version", "",
		"exact version to install (default: resolve the latest release and pin it)")
	return cmd
}

func newInstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Restore the integrations recorded in react.json",
		Long: `Restore the integrations recorded in react.json.

This is the command to run after cloning a project: it reinstalls every tool at
the exact version the manifest pins, which is why .react/ is gitignored.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := requireProject()
			if err != nil {
				return err
			}

			if len(m.Integrations) == 0 {
				printer.Banner("nothing to install")
				printer.Note("no integrations are recorded in %s", ui.File(project.ManifestName))
				printer.Blank()
				return nil
			}

			printer.Banner("restoring integrations")

			names := make([]string, 0, len(m.Integrations))
			for name := range m.Integrations {
				names = append(names, name)
			}
			sort.Strings(names)

			for _, name := range names {
				target, ok := integration.Lookup(name)
				if !ok {
					printer.Warn("%s is recorded in the manifest but this CLI does not know it", name)
					continue
				}
				if name == "tailwind" {
					// Pin to the recorded version rather than resolving latest:
					// restoring a project must not silently upgrade it.
					integration.TailwindVersion = m.Integrations[name].Version
				}
				entry, err := target.Install(cmd.Context(), integration.Env{Manifest: m, Printer: printer})
				if err != nil {
					return err
				}
				m.Integrations[name] = entry
			}

			if err := m.Save(); err != nil {
				return err
			}
			return regenerateIntegrations(m)
		},
	}
}

// regenerateIntegrations rewrites the CLI-owned app/integrations.go from the
// manifest.
//
// The file is regenerated wholesale from the recorded integration set rather
// than patched, so adding and removing tools cannot corrupt it and its contents
// are always exactly a function of react.json. If a project has no such file —
// because it predates the convention or was laid out by hand — the command says
// so instead of creating one in a package it knows nothing about.
func regenerateIntegrations(m *project.Manifest) error {
	path := findIntegrationsFile(m)
	if path == "" {
		printer.Warn("no generated integrations file found; add the head elements yourself")
		for name, entry := range m.Integrations {
			if target, ok := integration.Lookup(name); ok {
				for _, head := range target.Head(entry) {
					printer.Note("%s", head)
				}
			}
		}
		return nil
	}

	var heads []string
	names := make([]string, 0, len(m.Integrations))
	for name := range m.Integrations {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if target, ok := integration.Lookup(name); ok {
			heads = append(heads, target.Head(m.Integrations[name])...)
		}
	}

	content, err := scaffold.Integrations(heads)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return err
	}

	rel, _ := filepath.Rel(m.Root(), path)
	printer.Success("regenerated %s", ui.File(rel))
	return nil
}

// findIntegrationsFile locates the generated file. It looks in the conventional
// place first and then anywhere in the tree, so a project that renamed app/ to
// something else still works.
func findIntegrationsFile(m *project.Manifest) string {
	conventional := m.Path("app/integrations.go")
	if _, err := os.Stat(conventional); err == nil {
		return conventional
	}

	var found string
	_ = filepath.WalkDir(m.Root(), func(path string, d os.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".react", "dist", "node_modules", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == "integrations.go" {
			found = path
		}
		return nil
	})
	return found
}
