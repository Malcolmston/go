# dispatch — Go port of d3-dispatch: a small event bus with named event types and namespaced

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/d3/dispatch.svg)](https://pkg.go.dev/github.com/malcolmston/d3/dispatch)

Package dispatch is a Go port of d3-dispatch: a small event bus with named
event types and namespaced listener registration.

- **Named types on one object.** A simulation or a layout emits "start",
"tick" and "end". With channels that is three fields, three lifetimes and
three shutdown paths; here it is one value and one string. - **Namespaced
registration and, crucially, de-registration.** A listener registers as
"tick.legend", and later "tick.legend" replaces or removes exactly that
listener without disturbing "tick.axis". There is no channel equivalent —
you would have to invent a registry, which is this package. - **Synchronous,
ordered delivery.** Every listener runs to completion, in registration order,
on the caller's goroutine, before `Dispatch.Call` returns. That is what a
layout callback needs: "tick" means "the state is consistent right now, read
it". A channel hand-off makes the reader concurrent with the next mutation,
which is a different and usually wrong contract.

## Install

`d3` lives inside the [aggregator repo](../..) as a plain
directory rather than its own GitHub repository, so it is **not fetchable with
`go get`** yet. Use it through the committed `go.work`, or add a `replace`:

```
require github.com/malcolmston/d3 v0.0.0
replace github.com/malcolmston/d3 => ../path/to/go/d3
```

```go
import "github.com/malcolmston/d3/dispatch"
```

Local version: `0.1.0` (see [`VERSION`](../VERSION)).

## Exported surface

### Types

| Type | What it is |
| --- | --- |
| `Dispatch` | Dispatch is a bus over a fixed set of named event types. |
| `Listener` | Listener is a callback. |

<details>
<summary><code>Dispatch</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func MustNew[T any](types ...string) *Dispatch[T]` | MustNew is `New` for the usual case, where the type names are literals in the source and a bad one is a programmer error rather than bad input. |
| `func New[T any](types ...string) (*Dispatch[T], error)` | New returns a Dispatch for the given event types. |
| `func (d *Dispatch[T]) Apply(typ string, arg T) error` | Apply is `Dispatch.Call` with an unknown type reported as an error wrapping `ErrUnknownType` rather than as a panic. |
| `func (d *Dispatch[T]) Call(typ string, arg T)` | Call invokes every listener registered for the type, in registration order, synchronously on the calling goroutine, and returns when the last one has… |
| `func (d *Dispatch[T]) Copy() *Dispatch[T]` | Copy returns an independent Dispatch with the same types and the same listeners registered. |
| `func (d *Dispatch[T]) Listener(spec string) Listener[T]` | Listener returns the callback registered for a single "type.name" specifier, or nil if there is none. |
| `func (d *Dispatch[T]) On(typenames string, callback Listener[T]) error` | On registers, replaces or removes listeners. |
| `func (d *Dispatch[T]) Types() []string` | Types returns the declared event types in declaration order. |

</details>

### Variables

`ErrUnknownType`

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/d3/dispatch).

## Deviations from upstream

Deliberate differences for this package, where any exist, are recorded in the
module-wide [`API-DEVIATIONS.md`](../API-DEVIATIONS.md).

## License

MIT, as part of [`github.com/malcolmston/d3`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
