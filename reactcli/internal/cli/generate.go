package cli

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/malcolmston/reactcli/internal/scaffold"
	"github.com/malcolmston/reactcli/internal/ui"
)

func newGenerateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "generate",
		Aliases: []string{"gen", "g"},
		Short:   "Generate components and other project code",
	}
	cmd.AddCommand(newGenerateComponentCommand())
	return cmd
}

func newGenerateComponentCommand() *cobra.Command {
	var (
		dir       string
		props     []string
		withState bool
	)

	cmd := &cobra.Command{
		Use:     "component <Name>",
		Aliases: []string{"c"},
		Short:   "Generate a component",
		Long: `Generate a component.

Props are declared as name:type pairs and become typed accessor lines in the
generated body, so the component starts out reading its props the way the Props
API intends rather than with untyped map lookups:

  react gen component UserCard --prop name:string --prop age:int --state

Supported prop types are string, int, bool and any (the default).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := requireProject()
			if err != nil {
				return err
			}

			name := goIdentifier(args[0])
			if name == "" {
				return fmt.Errorf("%q does not yield a usable Go identifier", args[0])
			}

			if dir == "" {
				dir = "app"
			}
			targetDir := m.Path(dir)
			if err := os.MkdirAll(targetDir, 0o755); err != nil {
				return err
			}

			specs, err := parsePropSpecs(props)
			if err != nil {
				return err
			}

			content, err := scaffold.Component(scaffold.ComponentParams{
				Package:   filepath.Base(targetDir),
				Name:      name,
				Props:     specs,
				WithState: withState,
			})
			if err != nil {
				return err
			}

			// Formatting the generated source rather than trusting the template
			// means the template can stay readable, and a template bug shows up
			// here as a parse error instead of as badly indented committed code.
			formatted, err := format.Source([]byte(content))
			if err != nil {
				return fmt.Errorf("generated code did not parse: %w", err)
			}

			path := filepath.Join(targetDir, strings.ToLower(name)+".go")
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("%s already exists", path)
			}
			if err := os.WriteFile(path, formatted, 0o644); err != nil {
				return err
			}

			rel, _ := filepath.Rel(m.Root(), path)
			printer.Banner("generated " + ui.Accent(name))
			printer.Success("%s", ui.File(rel))
			printer.Note("render it with %s",
				ui.Command(fmt.Sprintf("react.CreateElement(%s, react.Props{…})", name)))
			printer.Blank()
			return nil
		},
	}

	cmd.Flags().StringVar(&dir, "dir", "app", "directory to generate into")
	cmd.Flags().StringArrayVar(&props, "prop", nil,
		"a prop as name:type (string, int, bool, any); repeatable")
	cmd.Flags().BoolVar(&withState, "state", false, "include a UseState example")
	return cmd
}

// parsePropSpecs turns name:type pairs into the accessor lines the template
// emits. Each supported type maps to the Props accessor that reads it, which is
// the point of the flag: the generated component demonstrates the typed reads
// rather than leaving map indexing for the reader to get wrong.
func parsePropSpecs(props []string) ([]scaffold.PropSpec, error) {
	specs := make([]scaffold.PropSpec, 0, len(props))

	for _, raw := range props {
		name, typ, found := strings.Cut(raw, ":")
		if !found {
			typ = "any"
		}
		name = strings.TrimSpace(name)
		typ = strings.TrimSpace(typ)
		if name == "" {
			return nil, fmt.Errorf("prop %q has no name", raw)
		}

		local := lowerFirst(goIdentifier(name))
		var field string
		switch typ {
		case "string":
			field = fmt.Sprintf("%s := props.String(%q)", local, name)
		case "int":
			field = fmt.Sprintf("%s, _ := props.Get(%q).(int)", local, name)
		case "bool":
			field = fmt.Sprintf("%s, _ := props.Bool(%q)", local, name)
		case "", "any":
			typ = "any"
			field = fmt.Sprintf("%s := props.Get(%q)", local, name)
		default:
			return nil, fmt.Errorf("unsupported prop type %q for %q; use string, int, bool or any",
				typ, name)
		}

		specs = append(specs, scaffold.PropSpec{Name: name, Local: local, Field: field, Type: typ})
	}
	return specs, nil
}

// lowerFirst lowercases the first rune, turning an exported-looking identifier
// into a local variable name.
func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	if r[0] >= 'A' && r[0] <= 'Z' {
		r[0] = r[0] - 'A' + 'a'
	}
	return string(r)
}
