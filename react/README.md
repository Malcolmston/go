# react

[![Go Test](https://github.com/Malcolmston/react/actions/workflows/go-test.yml/badge.svg)](https://github.com/Malcolmston/react/actions/workflows/go-test.yml)
[![Go Lint](https://github.com/Malcolmston/react/actions/workflows/go-lint.yml/badge.svg)](https://github.com/Malcolmston/react/actions/workflows/go-lint.yml)
[![Go Vuln](https://github.com/Malcolmston/react/actions/workflows/go-vuln.yml/badge.svg)](https://github.com/Malcolmston/react/actions/workflows/go-vuln.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/react.svg)](https://pkg.go.dev/github.com/malcolmston/react)
[![Go Report Card](https://goreportcard.com/badge/github.com/malcolmston/react)](https://goreportcard.com/report/github.com/malcolmston/react)
[![Go Version](https://img.shields.io/github/go-mod/go-version/Malcolmston/react)](go.mod)
[![Release](https://img.shields.io/github/v/release/Malcolmston/react?sort=semver)](https://github.com/Malcolmston/react/releases)
[![Last Commit](https://img.shields.io/github/last-commit/Malcolmston/react)](https://github.com/Malcolmston/react/commits)
[![Code Size](https://img.shields.io/github/languages/code-size/Malcolmston/react)](https://github.com/Malcolmston/react)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)
[![Docs](https://img.shields.io/badge/docs-pages-2f9bff)](https://malcolmston.github.io/react/)

**React 19's component and hooks model — for Go.**

`react` brings the declarative programming model of React to Go: components are
plain functions, state lives in hooks, and a reconciler diffs the tree between
renders so state survives where the output shape does not change. There is no
browser, so the output is HTML — the package renders trees to strings.

- **Elements** — `CreateElement`, `H`, `Frag`, `CloneElement`, `Fragment`.
- **Hooks** — state, reducer, effects, memo, callback, ref, context, id,
  transitions, external stores.
- **Boundaries** — `Suspense`, `ErrorBoundary`, `Lazy`.
- **SSR** — `RenderToString`, `RenderToStaticMarkup`, `RenderToWriter`.

Standard library only.

## Install

```sh
go get github.com/malcolmston/react
```

Requires **Go 1.24+** (the package uses generics and range-over-int).

## Hello, world

```go
package main

import (
	"fmt"

	"github.com/malcolmston/react"
)

// Components are a named function type, so a func literal must be given that
// type before it can be used as an element type.
var Greeting react.Component = func(p react.Props) react.Node {
	return react.H("section", react.Props{"class": "greeting"},
		react.H("h1", nil, "Hello, ", p.String("name"), "!"),
		react.H("ul", nil,
			react.H("li", react.Props{react.KeyProp: "a"}, "first"),
			react.H("li", react.Props{react.KeyProp: "b"}, "second"),
		),
	)
}

func main() {
	html, err := react.RenderToString(react.CreateElement(Greeting, react.Props{
		"name": "Ada",
	}))
	if err != nil {
		panic(err)
	}
	fmt.Println(html)
	// <section class="greeting"><h1>Hello, Ada!</h1><ul><li>first</li><li>second</li></ul></section>
}
```

## Building trees

There is no JSX in Go, so a tree is nested constructor calls. `H` is the
workhorse for host (tag) elements, `CreateElement` is the general form used for
components, and `Frag` groups siblings without a wrapper:

```go
react.H("div", react.Props{"id": "app"}, children...)        // <div id="app">…</div>
react.CreateElement(Greeting, react.Props{"name": "Ada"})    // a component
react.Frag(a, b, c)                                          // <>…</> — no wrapper
react.CloneElement(el, react.Props{"class": "active"})       // inject a prop
```

`Props` is a `map[string]any`. Three keys are reserved and behave exactly as in
React: `react.ChildrenKey` (`"children"`) holds the child nodes, and
`react.KeyProp` / `react.RefProp` are lifted out of the map into `Element.Key`
and `Element.Ref` and never reach a component.

Anything that can appear as a child is a `Node`:

| Node value | Renders as |
| --- | --- |
| `nil`, `false`, `true` | nothing (both booleans, so `cond && node` works) |
| `string` | a text node |
| any int / uint / float | a text node, formatted the way JavaScript would |
| `*react.Element` | the element |
| a slice of any of the above | spliced in place |

Anything else panics at render time naming the offending Go type.

Read props through the nil-safe accessors — `Get`, `Has`, `String`, `Bool`,
`Children`, `Clone`. Never mutate the map a component was handed: the runtime
reuses props across bailouts, so a mutation is visible to the previous render.

The `Children` helpers mirror `React.Children`: `ChildrenToSlice`,
`ChildrenCount`, `ChildrenForEach`, `ChildrenMap`, `ChildrenOnly`.

## The hooks tour

### State

```go
count, setCount := react.UseState(0)

setCount(count + 1)                     // replace
setCount.Update(func(n int) int { return n + 1 }) // functional form
```

`UseState` is generic, so `count` is an `int`, not an `any` to assert. The
setter is a `SetStateFunc[T]` — callable like React's setter, with an `.Update`
method for the functional form, because Go cannot overload one parameter to
accept both a value and an updater.

`UseStateLazy` takes a `func() T` for expensive initial values.
`UseReducer` and `UseReducerInit` are the reducer form.

### Effects

```go
react.UseEffect(func() func() {
	tick := time.NewTicker(time.Second)
	go pump(tick.C)
	return tick.Stop           // cleanup
}, []any{interval})

react.UseEffectFn(func() { log.Println("rendered") }, nil) // no cleanup
```

Dependency rules follow React's exactly:

- **`nil` deps** — run after every render.
- **empty non-nil deps** (`[]any{}`) — run once, on mount.
- **populated deps** — re-run when any entry changes.

`UseLayoutEffect` / `UseLayoutEffectFn` run synchronously before any passive
effect; `UseInsertionEffect` runs earlier still. Every effect's previous cleanup
runs immediately before its new body, and unmounting runs cleanups from the
leaves upward.

### Memo, refs and identity

```go
total := react.UseMemo(func() int { return sum(items) }, []any{items})
onClick := react.UseCallback(func() { setOpen(true) }, []any{})
node := react.UseRef[*Widget](nil)   // node.Current
id := react.UseId()                  // stable, unique per component instance
```

`UseImperativeHandle` publishes a value through a `*Ref[T]`; `UseDebugValue`
labels a hook for inspection.

### Context

```go
var ThemeContext = react.CreateContext("light")   // package-level, once

// provide
ThemeContext.Provider("dark", children...)

// read
theme := react.UseContext(ThemeContext)           // a string, not an any

// or, where there is no component to hang a hook on
ThemeContext.Consumer(func(theme string) react.Node { return react.H("i", nil, theme) })
```

Contexts are generic, so `UseContext` returns a `T`. Create them once, into a
package-level var — creating one inside a component body allocates a fresh
identity every render and orphans every consumer.

### Memoized components

```go
// Build the wrapper once, at package level — a fresh one each render is a new
// element type and would remount the subtree.
var Row = react.Memo(rowComponent)
var Row2 = react.MemoWith(rowComponent, myPropsEqual)

// Use it as an element type, exactly like a component:
react.CreateElement(Row, react.Props{"id": id})
```

`Memo` returns a `*MemoType` — an element type, not a `Component` — and bails
out of re-rendering when props are shallow-equal (`ShallowEqualProps`).
`MemoWith` takes a custom comparison; it returns `true` for "these are the same,
skip the render", the opposite direction from `shouldComponentUpdate`. A memo
boundary still re-renders when a component *below* it has pending state, so
memoizing can never swallow a descendant's update.

`StrictMode` and `Profiler` wrap a subtree the way their React counterparts do.

### Concurrency helpers

```go
pending, startTransition := react.UseTransition()
startTransition(func() { setQuery(next) })

deferred := react.UseDeferredValue(query)
optimistic, addOptimistic := react.UseOptimistic(messages)
state, action, isPending := react.UseActionState(submit, initial)
```

`StartTransition` is the standalone form, `UseDeferredValueInitial` seeds a
deferred value for the first render, and `UseFormStatus` reports the enclosing
action's in-flight state. These keep React's API shape, but note that this port
has no scheduler — see [how this differs](#how-this-differs-from-react).

### External stores

```go
value := react.UseSyncExternalStore(store.Subscribe, store.Get)
```

`Store[T]` is a batteries-included implementation of that contract —
`NewStore`, `Get`, `Set`, `Update`, `Subscribe` — for the common case where you
just need shared state outside the tree.

### Async and Suspense

```go
// A keyed, single-flight loader: the same id returns the identical *Async[User].
var loadUser = react.Cache(func(id string) (User, error) { return fetchUser(id) })

var Profile react.Component = func(p react.Props) react.Node {
	u := react.Use(loadUser(p.String("id"))) // value, or suspend, or panic the error
	return react.H("h1", nil, u.Name)
}

react.Suspense(
	react.H("p", nil, "loading…"),                       // fallback
	react.CreateElement(Profile, react.Props{"id": id}), // children
)

react.ErrorBoundary(
	func(err any) react.Node { return react.H("p", nil, "failed") },
	subtree,
)
```

`Go` starts one-off work directly; `Resolved` and `Failed` build already-settled
`Async` values, which is what makes suspense behavior testable without real
concurrency. `NewCache` is `Cache` with the `*Cached` handle kept, so
`Invalidate` and `Clear` stay reachable — usually what you want in a
long-running Go process. `Lazy` defers loading a component until it is first
rendered.

Note that a `Suspense` boundary is *not* an error boundary: any panic that is
not a suspension passes straight through it to an `ErrorBoundary`. And unlike
React, an `ErrorBoundary` here re-attempts its children on every render rather
than latching into an error state — to keep a failed subtree down, stop
rendering it, or change the boundary's `Key` to force a remount.

## Rendering to HTML

Go has no DOM, so server rendering is the port's primary output path. All four
entry points take a `Node` and manage the `Root` themselves — mounting it,
walking it, and unmounting it (so effect cleanups run) before returning:

```go
html, err := react.RenderToString(app)        // accumulate into a string
html, err  = react.RenderToStaticMarkup(app)  // use it when the output is final
err = react.RenderToWriter(w, app)            // stream to an io.Writer

fmt.Fprint(w, react.MustRenderToString(app))  // panics on error; tests and init only
```

`RenderToString` and `RenderToStaticMarkup` are **not** aliases. As upstream,
they differ by hydration metadata, and the one piece of that metadata which
reaches the bytes is the separator between adjacent text nodes:

```go
el := react.H("div", nil, "a", "b")

react.RenderToString(el)       // <div>a<!-- -->b</div>
react.RenderToStaticMarkup(el) // <div>ab</div>
```

`ab` is ambiguous — one text node or two? — and a hydrating client cannot guess,
so React writes an empty comment the parser turns into a splitting comment node.
Static markup is never hydrated and never needs it. `RenderToWriter` renders in
`RenderToString` mode, separator included. What the port still does not have is
the rest of hydration: no boundary comments, no bootstrap scripts, no
`hydrateRoot`. See [API-DEVIATIONS.md](API-DEVIATIONS.md) for the exact rules and
for the attribute-order deviation that cannot be fixed here.

Wired into `net/http`:

```go
http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
	page := react.CreateElement(Page, react.Props{"path": r.URL.Path})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := react.RenderToWriter(w, page); err != nil {
		http.Error(w, "render failed", http.StatusInternalServerError)
	}
})
```

What the emitter handles:

- **Escaping.** Text children and attribute values are HTML-escaped.
- **Void elements** (`<br>`, `<img>`, …) are emitted without a closing tag;
  giving one children is an error, not malformed output.
- **Attribute names.** React's prop names map to HTML attributes —
  `className` → `class`, `htmlFor` → `for`, and the rest. `AttributeName`
  exposes the mapping.
- **Styles.** A `style` prop holding a map is serialised with React's rules:
  camelCase → kebab-case, and `px` appended to numeric values except for the
  unitless properties. `FormatStyle` exposes it.
- **Raw HTML** is opt-in through `DangerousHTML`, and cannot be combined with
  children.
- **Document metadata is hoisted**, as React 19's Float does: a `<title>`,
  `<meta>`, `<link>` or async `<script src>` written in the body is lifted to the
  front of the document — into the `<head>` when the tree has one, which is
  synthesized if an `<html>` lacks it. A tree that can hoist is buffered rather
  than streamed, since the last element of a document can still belong at its
  front. `<base>`, an unowned stylesheet, and anything carrying `itemProp` stay
  put; see [API-DEVIATIONS.md](API-DEVIATIONS.md) for the full rules and the four
  limits, including the image preloads that are decided but not yet flushed.
- **Attribute order** is sorted by prop name. React follows the props object's
  insertion order, which a Go map does not have; sorting is what makes one tree
  render to the same bytes twice. React's own hand-written orderings — `input`,
  `button`, `form` and `option` defer a fixed set of props to the end of the tag
  — *are* reproduced on top of that pass.
- **Panics** anywhere in rendering — an unrenderable child, a panicking
  component, a hook-order violation — are recovered and returned as an error, so
  a bad template fails the request rather than the process.

## Interoperating with real React

The [`rsc`](rsc) subpackage serializes a tree into React's **Flight** wire
format, so a real React 19 client can render it. Go server components are
evaluated here and travel as data — no JavaScript twin needed — while
interactive components stay real JS modules referenced by module id:

```go
Button := rsc.Client("./src/Button.js", "Button", "static/chunks/button.js")

tree := react.H("main", nil,
    react.H("h1", nil, "Dashboard"),
    Button.El(react.Props{"label": "Save"}, react.H("span", nil, "rendered in Go")),
)
_ = rsc.Render(w, tree)
```

The encoder is verified by handing its output to React's own decoder, not by
following a spec — Flight is undocumented and version-coupled. See
[rsc/README.md](rsc/README.md) for what is reproduced, what is not, and the
maintenance cost that comes with it.

## Testing a tree

`Root.Tree()` returns an `Instance` — a queryable view of the rendered tree,
this port's stand-in for the DOM node a browser test would assert against.
`Act(fn)` runs `fn` and then settles every tree it touched: the updates it
scheduled render, their effects fire, updates *those* scheduled render, and so
on until nothing is pending. Assertions never race a pending effect:

```go
root := react.NewRoot(app)
defer root.Unmount()

react.Act(func() { setCount(3) })

// The tree does not change while fn runs — read it after Act returns.
got := root.Tree().FindByTag("span").TextContent()
```

`Instance` offers `Find`, `FindAll`, `FindByTag`, `FindAllByTag`, `FindByProp`,
`Children`, `Parent`, `Props`, `Text`, `TextContent` and a `String` dump.

`Act` is `Batch` with a name that says what it is for, so it settles synchronous
work only. If `fn` starts a goroutine, wait for it with a channel — never a
sleep — and then call `Act` again.

## How this differs from React

The programming model is faithful; the runtime underneath it is not the same
machine. The differences that will actually change how you write code:

- **No JSX.** Trees are built by calling `H` / `CreateElement` / `Frag`. This is
  the single biggest ergonomic difference. Let indentation carry the structure,
  and factor sub-trees into helper functions more aggressively than you would in
  JSX — a Go tree gets unreadable at a shallower nesting depth than markup does.
- **No DOM, no events.** There is no `document`, no synthetic event system, no
  `onClick` that fires. Handlers passed as props are inert data as far as the
  renderer is concerned. Output is HTML via `RenderToString`; inspection is via
  `Instance` rather than a DOM node.
- **No scheduler, no priority.** React 19 splits work into lanes and can
  interrupt a render. Here a state update marks the tree dirty and a render loop
  runs passes until it settles, synchronously. `UseTransition`,
  `StartTransition` and `UseDeferredValue` keep their shape but cannot actually
  deprioritise work.
- **Explicit batching.** React batches automatically around events and effects.
  Go has no event loop to hang that off, so wrap updates in `react.Batch` to
  coalesce them into one render. Outside a `Batch`, each update flushes.
- **Generics instead of dynamic typing.** `UseState[T]`, `Context[T]`, `Ref[T]`
  and `Async[T]` are typed, so no assertions are needed at the call site — but a
  setter cannot accept both a value and an updater function, hence
  `setCount.Update(...)`.
- **Panic/recover instead of `throw`.** Suspense is a panicked `SuspendSignal`
  recovered by the nearest boundary; `ErrorBoundary` recovers ordinary panics.
  A component that recovers panics indiscriminately will swallow both.
- **Function components only.** React 19 removed class components from the
  recommended surface and Go has no analogue for `this.setState`, so hooks are
  the single state mechanism.
- **Components are a named type.** `react.Component` is `func(Props) Node`, and
  the runtime dispatches on it by type switch — declare components as
  `var Foo react.Component = func(...) {...}` rather than as a bare `func`.
- **Rendering is serialized process-wide.** The hook dispatcher is a single
  ambient pointer, so two roots cannot render in parallel. All `Root` methods are
  safe to call from any goroutine; they just take a turn.

Every deviation, including the smaller ones, is itemised with its reason in
[API-DEVIATIONS.md](API-DEVIATIONS.md). The architecture is walked through in
[OVERVIEW.md](OVERVIEW.md).

## License

[MIT](LICENSE). This is an independent re-implementation and is **not**
affiliated with or endorsed by Meta or the React project.
