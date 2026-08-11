# react — Overview

`react` is a single Go module that ports **React 19's programming model** —
components, props, hooks, context, reconciliation, Suspense — onto Go's runtime.
It depends only on the standard library and compiles into whatever binary uses
it.

- `github.com/malcolmston/react` — the whole port; one package, no sub-packages.

The thing it deliberately does *not* port is the browser. There is no DOM, no
event system and no scheduler here, so the output of a render is HTML text
rather than a mutated document. Everything else follows from that one decision.

---

## What this is

React's real contribution is not the DOM diffing — it is a way of writing user
interfaces as a pure function of state, with local state that survives
re-renders and composition by ordinary function call. That model is not tied to
JavaScript or to a browser, and it is genuinely useful anywhere a program
repeatedly derives a tree from state: server-rendered pages, static site
generation, email templates, terminal layout, document generation.

This package takes that model at face value. Components are `func(Props) Node`.
State is hooks. A reconciler decides which parts of the previous tree survive.
And the terminal operation is `RenderToString` rather than a commit to a DOM.

---

## How it works

### Elements, fibers, and the render pass

Three layers, and keeping them straight explains almost everything else.

**Elements** are immutable, plain data descriptions of what to render. An
`Element` has a `Type` (a tag string, a `Component`, `Fragment`, or a type
registered by an internal feature), a `Key`, a `Props` map and a `Ref`. Building
one is pure construction — it touches no runtime state, schedules nothing, and
is safe from any goroutine. In React these come out of JSX; here they come out
of `CreateElement`, `H` and `Frag`, which is the same thing minus the compiler.

**Fibers** are the mutable counterpart. A fiber is a node of the *rendered* tree,
and it is where state actually lives: its hook slots, its effect cleanups, its
children. A fiber survives across renders as long as its `(Type, Key)` pair
keeps matching in the same sibling position. That single rule is what decides
whether a component keeps its state or is remounted from scratch.

Children are held as a linked list (`child` → `sibling` → …) rather than a
slice, the same shape React uses. It makes insertion during reconciliation
allocation-free, and it gives every fiber a stable `parent` pointer — which is
what context lookup and boundary search walk.

**A render pass** starts at a synthetic container fiber that owns the tree, and
recurses down. Having a real fiber above the user's element means the top-level
element is reconciled by exactly the same code path as every other child, with
no special case for "the root changed type".

The port re-renders from the root on every pass rather than restarting work at
the fiber that changed. Reconciliation preserves fibers either way, so state and
effects behave identically, and the simpler traversal is far easier to reason
about. `Memo` is where a re-render is actually skipped.

The flush loop is: render the tree, commit, run effects, and repeat while
anything left the tree dirty. Effects that set state are the normal reason for a
second pass — that is how a subscribe effect delivers its first value. A
component that sets state unconditionally in its own body never settles, so the
loop panics after a bounded number of passes with a diagnostic naming the likely
cause, because an infinite loop with no message is the worst possible failure
mode.

### Hook slots, and why hook order matters

Hooks are package-level functions with no receiver. `UseState` has no idea which
component called it — so how does it find the right state?

Through an ambient pointer. Before invoking a component the runtime points a
package-level `currentlyRendering` variable at that component's fiber; React
calls this the dispatcher. Every hook reads it first, and calling a hook outside
a component body panics with that diagnostic rather than misbehaving.

Each fiber holds `hooks []*hook` plus a cursor `hookIdx`, reset to zero every
time the component function is entered. Each hook call takes the next slot and
advances the cursor. **The slot index, and nothing else, is a hook's identity.**

That is why the Rules of Hooks are a hard constraint here and not a lint
preference: a hook behind an `if` shifts every subsequent hook onto the wrong
slot, and your `UseEffect` starts reading your `UseState`'s value. The port
refuses to let that be silent — each slot records the *kind* of hook that
created it, and a mismatch on a later render panics naming the slot number, the
old hook and the new one.

Dependency lists go through `DepsChanged`, which implements React's rules: a nil
list means "always re-run", a length change always counts, and entries are
compared with `ObjectsEqual` — the port's `Object.is`. Go's `==` panics on
slices, maps and funcs, which a dependency list can easily contain, so those are
compared by pointer identity where possible and otherwise reported as *unequal*.
That is the conservative answer: reporting "changed" costs a re-render, while
reporting "same" would skip one that was needed.

Because the dispatcher is a single package-level pointer, rendering is
serialized process-wide by a mutex. Two roots cannot render in parallel. That is
a real cost, but the alternative — a dispatcher that concurrent renders
interleave on — is a class of bug that would be nearly impossible to diagnose.

### Reconciliation and keys

`reconcileChildren` diffs the new child list against the fiber's current
children and rebuilds the list, then renders each survivor.

The matching rule is React's. A child reuses an existing fiber — and therefore
its hooks, its state and its effect cleanups — when the old and new child agree
on **both element type and key**:

- **Keyed children** are matched by key across the whole sibling list, so
  reordering a keyed list moves state with the items instead of leaving it
  behind at a position.
- **Keyless children** are matched by index. This is why inserting an item at
  the front of a keyless list silently shifts everyone's state by one — the
  classic React bug, faithfully reproduced, because the alternative would be a
  different framework.

Element types are compared by Go type, and function types additionally by code
pointer: two references to the same function literal match, two structurally
identical but distinct functions do not. That is the right answer, since they
are different components.

Every old fiber that finds no match is unmounted depth-first, running effect
cleanups **from the leaves upward** so a child's cleanup never observes a parent
that has already been torn down. Unmounting is idempotent, which matters when a
subtree is dropped while an effect from the same commit is still pending.

Effects are collected during the render pass and drained after the tree is
complete — layout effects first and in tree order, then passive effects. That
ordering is the guarantee that lets a layout effect observe a tree that passive
effects have not yet mutated. Each effect's previous cleanup runs immediately
before its new body, which is what makes an effect with changing deps read as
"tear down the old subscription, set up the new one".

### Updates and batching

A state setter marks its fiber `needsRender` and sets `subtreeDirty` on every
ancestor up to the root. The ancestor flag is not redundant: it is what a
memoized component checks before bailing out. Without it, a `Memo` boundary with
equal props would swallow its own child's state update.

Then the setter marks the root dirty and either lets an in-flight render loop
pick the work up or starts one. An update raised *during* render does not
recurse — it sets the flag and the loop runs another pass, which is how the
"derive state from props" escape hatch settles without unbounded recursion.

There is no scheduler and no priority. React 19 assigns lanes, can interrupt a
render and can replay it at a different priority; none of that machinery exists
here. `UseTransition`, `StartTransition` and `UseDeferredValue` are present and
keep their API shape so code reads the same, but they cannot actually
deprioritise work.

Batching is explicit for the same reason. React batches automatically because it
owns the event loop that dispatches your handlers; Go has no such loop, so
`Batch(fn)` defers rendering across `fn` and flushes once per affected root at
the end. `Batch` calls nest and only the outermost flushes. Batch state is
package-level rather than per-root, because a single logical event may touch
several roots and should still produce one render each.

### Recursive rendering, and why Suspense is a panic

Rendering here is a recursive descent, not React's iterative work loop. That is
a deliberate trade: an interruptible work loop buys you time slicing, which this
port does not offer anyway, and a recursive descent buys you Go's
`panic`/`recover` — which is exactly the shape both Suspense and error
boundaries need.

`recover` only works within a call stack. Because a boundary's render is on the
stack below its whole subtree, a boundary can wrap its recursive descent in a
deferred `recover` and get the feature almost for free:

- **Suspense.** A component that cannot finish — because a value it needs is
  still being computed — calls `Suspend(ready)`, which panics a `SuspendSignal`
  carrying a channel. The nearest `Suspense` boundary above recovers it, renders
  its fallback instead, and re-renders the subtree once the channel closes. A
  `SuspendSignal` that reaches the root with no boundary to catch it is
  re-panicked as an ordinary error, matching React's "a component suspended
  while responding to synchronous input" failure.
- **Error boundaries.** `ErrorBoundary` recovers an ordinary panic from its
  subtree and renders a fallback from it. In React the equivalent is a thrown
  error caught by a class component's `componentDidCatch`; a panic is the
  faithful Go translation, since it is the only construct that unwinds an
  arbitrary depth of calls.

The two do not overlap: a `Suspense` boundary re-panics anything that is not a
`SuspendSignal`, so an ordinary failure passes through it to the nearest
`ErrorBoundary`. And an `ErrorBoundary` here does not latch. React keeps a
boundary in its error state until something resets it, because the error lives
in class state; here the boundary simply re-attempts its children on every
render and recovers on its own as soon as they stop panicking. That costs one
extra recover per render and never loops, because catching an error schedules no
update.

`Use`, `Lazy` and the `Async` helpers all sit on that one mechanism. `Go` starts
work and returns an `*Async[T]`; `Use` returns its value if it is ready,
suspends if it is not, and panics its error if it failed. `Resolved` and
`Failed` build already-settled values, which is what makes suspense behavior
testable without real concurrency. `Cache` / `NewCache` wrap a
`func(K) (V, error)` into a keyed, single-flight loader that returns the
*identical* `*Async` for a repeated key — so a component re-rendering does not
restart the work, and several components needing the same data suspend on one
channel and resume together.

The one thing to know as a consequence: a component that recovers panics
indiscriminately will swallow suspension and error propagation. Recover
selectively, or not at all, inside a component body.

### Serialising to HTML

A rendered tree is not the output; HTML is. `RenderToWriter` mounts a `Node`,
walks the resulting fiber tree emitting bytes as it goes, and unmounts it again
— so a large page starts reaching the client before the whole tree has been
serialized, and effect cleanups run exactly as they would for any other `Root`
lifecycle. `RenderToString` is the same walk accumulated into a string.

The emitter is where React's prop-to-HTML rules live: text and attribute values
are escaped, void elements are emitted without a closing tag, prop names are
mapped to attribute names (`className` → `class`, `htmlFor` → `for`, …), style
maps are serialised with camelCase-to-kebab conversion and `px` appended to
numeric values except for the unitless properties, and raw HTML is opt-in
through an explicit `DangerousHTML` wrapper rather than a bare string.

Two failure modes are treated as errors rather than as best-effort output: a
structurally invalid tree (children on a void element, raw HTML alongside
children, a style value of an unsupported type), and a panic from anywhere in
rendering. The second is deliberate — a template rendering deep inside a request
handler should fail that request, not the process — and it is why the render
functions return an error at all. An I/O failure is the writer's own error,
unwrappable; a tree problem is a `react:`-prefixed error naming the element at
fault.

`RenderToString` and `RenderToStaticMarkup` are currently the same function.
Upstream they differ by hydration metadata, and with no client to hydrate,
emitting markers nothing consumes would put noise in the output and imply a
capability that does not exist. The pair is kept because the names carry intent:
if hydration markers ever land, they land in one of them and not the other.

### Extension without a growing switch

The renderer's type switch handles four cases — host string, text, fragment,
`Component` — and then consults a registry keyed by Go type. Context providers,
`Memo`, `Lazy`, `Suspense`, `StrictMode` and `Profiler` each register their own
renderer from an `init` in their own file, so the core dispatch never grows a
case per feature. Registering the same type twice panics, because two features
silently fighting over one element type would be near-impossible to debug.

Context providers use this. `Context[T]` is generic — `UseContext` hands back a
`T` rather than an `any` to assert — but the provider's *element type* is a
single non-generic sentinel shared by every context and every value type, with
the context identity and the boxed value carried in props. A generic element
type could not be registered in a `map[reflect.Type]`, and a per-instantiation
type would fragment the registry. `UseContext` then walks up the parent chain
comparing a pointer identity, so nesting works and the nearest provider wins.

---

## How to use it

### Render a page to HTML

```go
package main

import (
	"fmt"

	"github.com/malcolmston/react"
)

var Item react.Component = func(p react.Props) react.Node {
	return react.H("li", nil, p.String("label"))
}

var List react.Component = func(p react.Props) react.Node {
	var items []react.Node
	for i, label := range []string{"alpha", "beta", "gamma"} {
		items = append(items, react.CreateElement(Item, react.Props{
			react.KeyProp: i,
			"label":       label,
		}))
	}
	return react.H("ul", react.Props{"class": "list"}, items)
}

func main() {
	fmt.Println(react.MustRenderToString(react.CreateElement(List, nil)))
}
```

The render functions take a `Node` and own the `Root`: they mount it, walk it
and unmount it — so effect cleanups run — before returning. Build a `Root` by
hand with `NewRoot` only when the tree needs to survive across renders, which
for a request handler it does not.

### State that survives a re-render

```go
var Counter react.Component = func(p react.Props) react.Node {
	n, setN := react.UseState(0)

	react.UseEffect(func() func() {
		t := time.NewTicker(time.Second)
		go func() {
			for range t.C {
				setN.Update(func(n int) int { return n + 1 })
			}
		}()
		return t.Stop
	}, []any{}) // empty deps: mount only

	return react.H("span", nil, "ticks: ", n)
}
```

The setter is callable for the replace form and carries `.Update` for the
functional form. `UseEffect`'s returned function is the cleanup; empty non-nil
deps mean "run once", `nil` deps mean "run after every render".

### Coalescing updates

```go
react.Batch(func() {
	setName("ada")
	setAge(36)
	setActive(true)
})
// one render, not three
```

### Suspense over async work

```go
user := react.Go(func() (User, error) { return fetchUser(ctx, id) })

var Profile react.Component = func(react.Props) react.Node {
	u := react.Use(user) // suspends until ready
	return react.H("h1", nil, u.Name)
}

page := react.Suspense(
	react.H("p", nil, "loading…"),
	react.CreateElement(Profile, nil),
)
```

---

## Why it's better than its predecessor

The predecessor is React itself, and React is excellent at the job it was built
for. This port is not an attempt to beat it at rendering a browser UI — it
cannot, because it has no browser. The honest case is about where the model is
useful *outside* one:

- **The model without the runtime.** You get components, hooks, context and
  reconciliation in a program that compiles to a single static binary. No Node,
  no `node_modules`, no bundler, no build step between source and output.
- **Type safety at the call site.** `UseState[T]`, `Context[T]`, `Ref[T]` and
  `Async[T]` are generic, so a context read is a `T` and not an `any` you assert
  or a `useContext` whose type you hope TypeScript inferred. Props remain an
  untyped map — they have to, since host elements take arbitrary attributes —
  but everything else is checked.
- **Loud failures instead of quiet corruption.** A changed hook order panics
  naming both hooks and the slot; an unrenderable child panics naming its Go
  type; a runaway re-render loop panics with the likely cause instead of
  hanging; registering a duplicate element renderer panics at init.
- **Server rendering as the first-class path, not an add-on.** React's SSR grew
  around a client-first design. Here it *is* the design, so there is no
  hydration-mismatch class of bug to reason about unless you go looking for one.
- **Go's tooling applies uniformly.** `gofmt`, `go vet`, the race detector, the
  testing package and the profiler all work on your components, because your
  components are just functions.

**Honest tradeoffs.** This is not a browser framework and will not become one:
no DOM, no events, no client-side hydration runtime. There is no scheduler, so
`UseTransition` and `UseDeferredValue` preserve the API shape without the
concurrency behavior, and a long render blocks its goroutine. Rendering is
serialized process-wide, so independent roots cannot render in parallel. And
writing trees as nested constructor calls is genuinely more verbose than JSX —
that is the price of Go having no macro system, and the reason `H` and `Frag`
exist at all. Reach for this when you want React's *way of thinking* on Go's
deployment story, not as a drop-in for a React application.
