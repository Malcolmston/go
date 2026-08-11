package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/malcolmston/reactcli/internal/integration"
	"github.com/malcolmston/reactcli/internal/project"
	"github.com/malcolmston/reactcli/internal/ui"
)

func newBuildCommand() *cobra.Command {
	var clean bool

	cmd := &cobra.Command{
		Use:   "build",
		Short: "Render the project to static HTML",
		Long: `Render the project to static HTML.

The build runs your program with go run, so a compile error or a panic in a
component is reported by the Go toolchain with a real file and line — the CLI
adds no layer of its own between you and the error.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := requireProject()
			if err != nil {
				return err
			}

			printer.Banner(fmt.Sprintf("building %s", ui.Accent(m.Name)))
			started := time.Now()

			if clean {
				if err := os.RemoveAll(m.OutDir()); err != nil {
					return err
				}
				printer.Success("cleaned %s", ui.File(m.Out))
			}

			if err := runBuild(cmd.Context(), m); err != nil {
				return err
			}

			printer.Blank()
			printer.Success("built in %s → %s",
				time.Since(started).Round(time.Millisecond), ui.File(m.Out))
			printer.Blank()
			return nil
		},
	}

	cmd.Flags().BoolVar(&clean, "clean", false, "remove the output directory first")
	return cmd
}

// runBuild is the whole build, shared by `react build` and every rebuild the
// dev server triggers.
//
// The order matters. Static assets are copied first so a generated stylesheet
// cannot be clobbered by a stale copy of itself. The Go render runs next
// because it is where real mistakes live and failing fast on it keeps the
// feedback loop short. Integrations run last: Tailwind scans Go source, so its
// output depends on code that has just been proven to compile.
func runBuild(ctx context.Context, m *project.Manifest) error {
	if err := os.MkdirAll(m.OutDir(), 0o755); err != nil {
		return err
	}

	if m.Static != "" {
		src := m.Path(m.Static)
		if _, err := os.Stat(src); err == nil {
			if err := copyTree(src, m.OutDir()); err != nil {
				return fmt.Errorf("copying %s: %w", m.Static, err)
			}
			printer.Success("copied %s", ui.File(m.Static))
		}
	}

	// The client half is compiled before the render, because the render imports
	// the generated declarations: a build that regenerated them afterwards
	// would compile against the previous run's output.
	if m.Client != "" {
		if err := compileRSC(m, false); err != nil {
			return err
		}
	}

	if err := runRender(ctx, m); err != nil {
		return err
	}
	printer.Success("rendered %s", ui.File(filepath.Join(m.Out, "index.html")))

	names := make([]string, 0, len(m.Integrations))
	for name := range m.Integrations {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		target, ok := integration.Lookup(name)
		if !ok {
			printer.Warn("skipping unknown integration %q", name)
			continue
		}
		entry := m.Integrations[name]
		if err := target.Build(ctx, integration.Env{Manifest: m, Printer: printer}, entry); err != nil {
			return err
		}
		printer.Success("%s → %s", ui.Accent(name), ui.File(entry.Output))
	}

	return nil
}

// runRender executes the project's own program.
//
// go run is used rather than go build plus exec because it keeps the temporary
// binary out of the project and because its error output is what a Go developer
// already knows how to read. The cost is a rebuild on every dev-server cycle,
// which Go's build cache makes cheap enough not to matter at this scale.
func runRender(ctx context.Context, m *project.Manifest) error {
	entry := m.Entry
	if !strings.HasPrefix(entry, ".") {
		entry = "./" + entry
	}

	cmd := exec.CommandContext(ctx, "go", "run", entry, "-out", m.OutDir())
	cmd.Dir = m.Root()
	cmd.Env = os.Environ()

	out, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(out))
		if trimmed == "" {
			return fmt.Errorf("go run %s: %w", entry, err)
		}
		// The toolchain's own message is the useful part; wrapping it in more
		// prose would only push it further from the reader.
		return fmt.Errorf("build failed:\n\n%s", trimmed)
	}
	if trimmed := strings.TrimSpace(string(out)); trimmed != "" {
		// The program printed something on a successful run — logs from an
		// effect, say. Pass it through rather than swallowing it.
		printer.Note("%s", trimmed)
	}
	return nil
}

// copyTree copies src's contents into dst, creating directories as needed.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if d.Name() == ".gitkeep" {
			return nil
		}

		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		defer out.Close()

		_, err = io.Copy(out, in)
		return err
	})
}
