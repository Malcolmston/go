// Package scaffold holds the file templates the CLI writes: new projects,
// generated components, and the one file the CLI owns inside a user's project.
//
// Everything here is plain text/template over small parameter structs. There is
// no embedded binary asset and no network fetch, so `react new` works offline
// and the generated code is reviewable in this file rather than downloaded from
// somewhere.
package scaffold

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

// Params are the values every project template is rendered against.
type Params struct {
	// Name is the project name, used in the page title and go.mod comment.
	Name string

	// Module is the Go module path of the generated project.
	Module string

	// ReactVersion is the version of the react library to require.
	ReactVersion string
}

// ComponentParams are the values the component generator is rendered against.
type ComponentParams struct {
	// Package is the Go package the component is generated into.
	Package string

	// Name is the exported component name, e.g. "UserCard".
	Name string

	// Props are the typed props the component accepts.
	Props []PropSpec

	// WithState adds a UseState example to the body.
	WithState bool
}

// PropSpec is one declared prop on a generated component.
type PropSpec struct {
	// Name is the props-map key, e.g. "title".
	Name string

	// Local is the Go variable the accessor binds the prop to.
	Local string

	// Field is the Go accessor used to read it, chosen from the prop's type.
	Field string

	// Type is the Go type the accessor yields.
	Type string
}

// File is one file a template set produces.
type File struct {
	// Path is relative to the destination directory, using forward slashes.
	Path string

	// Content is the fully rendered file body.
	Content string

	// Generated marks a file the CLI owns and may rewrite without asking. User
	// code is never marked, so a rewrite can never silently discard hand-edits.
	Generated bool
}

// render executes a template and returns its output.
func render(name, text string, data any) (string, error) {
	t, err := template.New(name).Parse(text)
	if err != nil {
		return "", fmt.Errorf("parsing template %s: %w", name, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("rendering template %s: %w", name, err)
	}
	return buf.String(), nil
}

// Project returns every file a new project is made of.
func Project(p Params) ([]File, error) {
	specs := []struct {
		path      string
		text      string
		generated bool
	}{
		{"go.mod", goModTemplate, false},
		{"main.go", mainTemplate, false},
		{"app/app.go", appTemplate, false},
		{"app/counter.go", counterTemplate, false},
		{"app/integrations.go", integrationsTemplate, true},
		{"public/.gitkeep", "", false},
		{".gitignore", gitignoreTemplate, false},
		{"README.md", readmeTemplate, false},
	}

	files := make([]File, 0, len(specs))
	for _, s := range specs {
		content, err := render(s.path, s.text, p)
		if err != nil {
			return nil, err
		}
		files = append(files, File{Path: s.path, Content: content, Generated: s.generated})
	}
	return files, nil
}

// Component renders a single generated component file.
func Component(p ComponentParams) (string, error) {
	return render("component", componentTemplate, p)
}

// Integrations renders the CLI-owned file that injects installed integrations'
// elements into the document head.
//
// It is a whole generated file rather than a patch applied to the user's
// app.go, which is the important design choice here: `react add` and `react
// remove` rewrite it wholesale and can never mangle hand-written code, and a
// user who wants to see what an integration did can read one short file.
func Integrations(heads []string) (string, error) {
	return render("integrations", integrationsGenTemplate, struct{ Heads []string }{heads})
}

// Write creates files under dir. It refuses to overwrite an existing file
// unless that file is one the CLI generated, which is what keeps `react add`
// safe to re-run while `react new` still declines to clobber a real project.
func Write(dir string, files []File) error {
	for _, f := range files {
		full := filepath.Join(dir, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if !f.Generated {
			if _, err := os.Stat(full); err == nil {
				return fmt.Errorf("refusing to overwrite existing file %s", full)
			}
		}
		if err := os.WriteFile(full, []byte(f.Content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

const goModTemplate = `module {{.Module}}

go 1.24

require github.com/malcolmston/react {{.ReactVersion}}
`

const mainTemplate = `// Command {{.Name}} renders this react project to static HTML.
//
// The CLI does not load your code as a plugin — it runs this program. That
// means ` + "`go run .`" + ` works with no CLI involved at all, and anything you
// can do in a Go main function (read a database, walk a content directory,
// generate a page per record) is available to a build.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/malcolmston/react"

	"{{.Module}}/app"
)

func main() {
	out := flag.String("out", "dist", "directory to write rendered HTML into")
	flag.Parse()

	// RenderToString mounts the tree, renders it, runs effect cleanups and
	// unmounts — a complete lifecycle per call. Errors come back as values
	// rather than panics, so a broken component fails the build cleanly.
	html, err := react.RenderToString(react.CreateElement(app.App, nil))
	if err != nil {
		fmt.Fprintf(os.Stderr, "render failed: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	page := "<!doctype html>\n" + html + "\n"
	if err := os.WriteFile(filepath.Join(*out, "index.html"), []byte(page), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
`

const appTemplate = `package app

import "github.com/malcolmston/react"

// App is the root component. Note the type: it is declared as react.Component,
// not as a bare func(react.Props) react.Node. The runtime dispatches on the
// named type, so an unconverted function literal will not be recognised as a
// component — this is the single most common mistake when starting out.
var App react.Component = func(props react.Props) react.Node {
	return react.H("html", react.Props{"lang": "en"},
		react.H("head", nil,
			react.H("meta", react.Props{"charSet": "utf-8"}),
			react.H("meta", react.Props{
				"name":    "viewport",
				"content": "width=device-width, initial-scale=1",
			}),
			react.H("title", nil, "{{.Name}}"),
			// Elements contributed by installed integrations. Managed by
			// ` + "`react add`" + `; see app/integrations.go.
			react.Frag(integrationHead()...),
		),
		react.H("body", nil,
			react.H("main", nil,
				react.H("h1", nil, "{{.Name}}"),
				react.CreateElement(Counter, react.Props{"start": 3, "label": "Items"}),
			),
		),
	)
}
`

const counterTemplate = `package app

import "github.com/malcolmston/react"

// Counter shows the shape of a component that takes props and holds state.
//
// A word about state in a static render: this project renders to HTML once and
// exits, so a state update here changes what the *build* produces, not what a
// visitor clicks. Hooks earn their place when state is derived during rendering
// — a value computed once and reused, an effect that loads data before the page
// is emitted — rather than as a response to browser events, which this port has
// no way to receive.
var Counter react.Component = func(props react.Props) react.Node {
	label := props.String("label")
	start, _ := props.Get("start").(int)

	count, setCount := react.UseState(start)

	// Runs after the render commits. Cleanup is returned, and the runtime calls
	// it when the component unmounts — which RenderToString does for you.
	react.UseEffectFn(func() {
		if count < 5 {
			setCount(5)
		}
	}, []any{count})

	return react.H("section", react.Props{"className": "counter"},
		react.H("h2", nil, label),
		react.H("p", nil, "count: ", count),
	)
}
`

const integrationsTemplate = `// Code generated by ` + "`react add`" + `. DO NOT EDIT.

package app

import "github.com/malcolmston/react"

// integrationHead returns the elements installed integrations contribute to
// the document head. It is empty until you run ` + "`react add`" + `.
func integrationHead() []react.Node {
	return nil
}
`

const integrationsGenTemplate = `// Code generated by ` + "`react add`" + `. DO NOT EDIT.

package app

import "github.com/malcolmston/react"

// integrationHead returns the elements installed integrations contribute to
// the document head.
func integrationHead() []react.Node {
{{- if .Heads}}
	return []react.Node{
{{- range .Heads}}
		{{.}},
{{- end}}
	}
{{- else}}
	return nil
{{- end}}
}
`

const gitignoreTemplate = `dist/
.react/
`

const readmeTemplate = `# {{.Name}}

A [react](https://github.com/malcolmston/react) project — React 19's programming
model in Go, rendered to static HTML.

## Develop

` + "```sh" + `
react dev       # rebuild and reload on save
react build     # write the site to dist/
react doctor    # check for hook-order and component-type mistakes
` + "```" + `

There is no CLI lock-in: this project is an ordinary Go program, so
` + "`go run . -out dist`" + ` produces the same output that ` + "`react build`" + ` does.

## Add ecosystem tooling

` + "```sh" + `
react add tailwind
` + "```" + `

## Layout

| Path | What it is |
| ---- | ---------- |
| ` + "`main.go`" + ` | Renders the tree and writes ` + "`dist/index.html`" + ` |
| ` + "`app/app.go`" + ` | The root component |
| ` + "`app/integrations.go`" + ` | Generated by ` + "`react add`" + ` — do not edit |
| ` + "`react.json`" + ` | Project manifest: entry, output, installed integrations |
| ` + "`public/`" + ` | Copied verbatim into the build output |
`

const componentTemplate = `package {{.Package}}

import "github.com/malcolmston/react"

// {{.Name}} is a component.
//
// It is declared as react.Component rather than as a bare
// func(react.Props) react.Node so the runtime recognises it: dispatch is by
// named type, and an unconverted function literal renders as nothing.
var {{.Name}} react.Component = func(props react.Props) react.Node {
{{- range .Props}}
	{{.Field}}
{{- end}}
{{- if .WithState}}

	// Hooks must be called unconditionally and in the same order on every
	// render — never inside an if, a loop or after an early return. The runtime
	// identifies a hook by its call index, so a conditional hook hands the next
	// hook someone else's state. ` + "`react doctor`" + ` checks this for you.
	value, setValue := react.UseState(0)
	_ = setValue
{{- end}}

	return react.H("div", react.Props{"className": "{{.Name}}"},
{{- range .Props}}
		react.H("span", react.Props{"className": "{{.Name}}"}, {{.Local}}),
{{- end}}
{{- if .WithState}}
		react.H("span", nil, value),
{{- end}}
	)
}
`

// SampleClientComponent is the component `react init` writes to show what a
// client component looks like.
//
// It is deliberately plain ES-module JavaScript rather than JSX: a browser can
// load it with no transform, which is what keeps a react project free of a
// JavaScript build step. The trade-off is React.createElement instead of JSX —
// the same trade-off the Go side makes, for the same reason.
const SampleClientComponent = `'use client';

// The "use client" directive above is what makes this a client component. The
// Go server never runs this file; it sends a reference to it, and the browser
// loads and renders it. That is why the state below actually works: this code
// runs in the browser, with a real React runtime.

import React, { useState } from 'react';

export function Counter({ start = 0, children }) {
  const [count, setCount] = useState(start);

  return React.createElement(
    'div',
    { className: 'counter' },
    children,
    React.createElement(
      'button',
      { type: 'button', onClick: () => setCount((n) => n + 1) },
      'clicked ', count, ' times'
    )
  );
}
`
