// Package integration wires ecosystem tooling that the React world takes for
// granted — Tailwind first — into a Go react project.
//
// The guiding constraint is that a react project must stay a Go project. None
// of these integrations introduce package.json, node_modules or a JavaScript
// toolchain; each one is a self-contained binary the CLI fetches, pins and runs.
// That is only possible for tools that ship a standalone build, which is the
// honest boundary of what this package can offer: an npm-only tool cannot be
// wired up this way, and the registry says so rather than shelling out to npx
// behind the user's back.
package integration

import (
	"context"
	"fmt"
	"sort"

	"github.com/malcolmston/reactcli/internal/project"
	"github.com/malcolmston/reactcli/internal/ui"
)

// Env is what an integration is given to do its work.
type Env struct {
	Manifest *project.Manifest
	Printer  *ui.Printer
}

// Integration is one wireable ecosystem tool.
type Integration interface {
	// Name is the identifier used on the command line.
	Name() string

	// Summary is the one-line description shown by `react add --list`.
	Summary() string

	// Install fetches whatever tooling is needed, writes any source files the
	// tool expects, and returns the manifest entry recording what was done.
	// It must be safe to run twice.
	Install(ctx context.Context, env Env) (Entry, error)

	// Head returns Go expressions — each a react.Node — that this integration
	// contributes to the document head. They are written into the generated
	// app/integrations.go, so they must be valid Go referencing only the react
	// package.
	Head(entry Entry) []string

	// Build runs the tool as part of `react build` and `react dev`.
	Build(ctx context.Context, env Env, entry Entry) error
}

// Entry is an alias so integrations do not each import the project package for
// one type name.
type Entry = project.Integration

// registry holds every known integration, keyed by name.
var registry = map[string]Integration{}

// register adds an integration. Called from each implementation's init.
func register(i Integration) {
	if _, dup := registry[i.Name()]; dup {
		panic(fmt.Sprintf("integration %q registered twice", i.Name()))
	}
	registry[i.Name()] = i
}

// Lookup returns the integration with the given name.
func Lookup(name string) (Integration, bool) {
	i, ok := registry[name]
	return i, ok
}

// All returns every registered integration, ordered by name so listings and
// help output are stable.
func All() []Integration {
	out := make([]Integration, 0, len(registry))
	for _, i := range registry {
		out = append(out, i)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name() < out[b].Name() })
	return out
}

// Names returns the registered integration names, sorted.
func Names() []string {
	out := make([]string, 0, len(registry))
	for name := range registry {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
