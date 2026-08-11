# Examples

Each one runs from the `react` module root with no setup:

```sh
go run ./examples/hello
go run ./examples/hooks
go run ./examples/rsc-server      # then open http://localhost:8080
```

## `hello` — the shape of everything

Components, props, children, keys, and rendering to HTML. The only thing worth
staring at is the declaration:

```go
var Greeting react.Component = func(props react.Props) react.Node { … }
```

Not `func Greeting(props react.Props) react.Node`. The runtime dispatches on the
named `react.Component` type, so a bare function with the right signature
compiles, reads exactly like a component, and renders as **nothing at all** — no
error, just a missing subtree. `react doctor` catches it; this example avoids
teaching it wrong in the first place.

## `hooks` — state, effects, memo, context

A tour of the hooks that mean something in a static render, including an effect
that sets state and settles before the render finishes.

Read the package comment first. It is honest about what state means with no
browser attached: this program renders once and exits, so a state update changes
what the *build* produces, not what a visitor clicks. Hooks earn their place by
deriving values and loading data, not by reacting to events.

For events, you want the next one.

## `rsc-server` — the interactive one

A Go HTTP server rendering React Server Components that a real React 19 browser
client mounts. **The counters actually work**: clicking one runs JavaScript in
the browser, with React's own runtime and its own hooks, while everything around
it was rendered in Go and shipped as data.

```
GET /                 the HTML shell, rendered in Go
GET /rsc              the Flight payload — the component tree, as data
GET /client/*.js      client components, as plain ES modules
GET /rsc/entry.js     the browser entry that fetches /rsc and mounts it
```

Nothing is bundled. React is loaded from an ES module host, client components
are loaded by dynamic `import()`, and there is no build step — the only
requirement is a browser with import maps, which every current one has.

The division to understand: a **Go component is evaluated on the server** and
travels as its output; a **client component is not evaluated** — Go sends a
reference to the module and the browser loads the real thing. Go decides what
the tree is; JavaScript makes the interactive parts interactive.

Composition goes both ways. In this example a Go-rendered `<span>` is passed as
`children` into a JavaScript component and appears inside its markup.

In a real project the `Counter` declaration and `entry.js` are generated — `react
init` scans for `"use client"` and writes both, so a renamed file cannot leave a
stale module id behind. They are checked in here so the example runs with no
toolchain.

## `../../examples/d3-react-chart` — d3 and react together

Lives outside this directory, in its own module, for a reason worth knowing:
it needs both `malcolmston/react` and `malcolmston/d3`, and **neither library may
depend on the other**. Both are standard-library-only, and an example sitting
inside either one would quietly end that.

```sh
go run ./examples/d3-react-chart > chart.html && open chart.html
```

It renders a real SVG chart — time scale, monotone curve, area fill, axes with
SI-formatted labels. The point is how narrow the seam between the two libraries
turns out to be:

- **d3 does the arithmetic.** Scales map values to pixels, `shape.NewLine`
  turns points into a string of SVG path commands, `format` and `timefmt` turn
  tick values into labels.
- **react does the markup.** It renders the SVG element tree — including that
  path string — to HTML.

So the entire interface is a `d` attribute and some scaled numbers. No shared
runtime, no DOM in between, and neither library knows the other's types.

In browser d3 the same chart is a chain of `.append("path").attr("d", …)` calls
mutating a live document. Here the tree is a value and the chart is a pure
function of its data, which is why the identical code works in a static build,
an HTTP handler, or a server component.
