// Command react is the toolchain for the Go port of React 19.
//
// It scaffolds projects, wires in ecosystem tooling as pinned standalone
// binaries, rebuilds and serves on save, generates components, and checks for
// the react-specific mistakes the Go compiler cannot catch.
//
//	react new blog
//	react add tailwind
//	react dev
//	react doctor
//
// A project is an ordinary Go module with a main package, so the CLI is a
// convenience rather than a dependency: go run . produces the same output that
// react build does.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/malcolmston/reactcli/internal/cli"
)

func main() {
	// Ctrl-C cancels the context rather than killing the process outright, so
	// the dev server drains its connections and an in-flight download removes
	// its temporary file instead of leaving it behind.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	os.Exit(cli.Execute(ctx, os.Args[1:]))
}
