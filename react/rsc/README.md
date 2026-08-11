# rsc — React Server Components from Go

Serializes a Go [react](..) tree into React's **Flight** wire format: the payload
a real React 19 client deserializes with `createFromFetch` /
`createFromReadableStream`.

This is the port's interoperability story. A Go server component is rendered
here and travels as data, so it needs no JavaScript counterpart. A client
component stays a real JS module, referenced by module id, and keeps its hooks,
its event handlers and its interactivity.

```go
Button := rsc.Client("./src/Button.js", "Button", "static/chunks/button.js")

tree := react.H("main", nil,
    react.H("h1", nil, "Dashboard"),
    Button.El(react.Props{"label": "Save"},
        react.H("span", nil, "rendered in Go")),
)

http.HandleFunc("/rsc", func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/x-component")
    _ = rsc.Render(w, tree)
})
```

The client resolves the reference through its bundler manifest, loads the real
`Button`, and renders it with the props Go sent — with the Go-rendered `<span>`
nested inside as `props.children`.

## Why this and not hydration

Classic SSR + `hydrateRoot` requires the same component tree to exist on **both**
sides, so a Go-authored component would need a JavaScript twin. RSC is the only
architecture in which a Go-authored component works with a real React client
without one: the server sends a serialized tree, not code to re-execute.

## Verified, not asserted

The format is undocumented, so nothing here is written from a spec.

- **Parity tests** (`parity_test.go`) compare this encoder's output against
  payloads captured from React itself. Always run; no toolchain needed.
- **Round-trip tests** (`roundtrip_test.go`) hand a Go-produced payload to
  React's *actual* client decoder, resolve a client reference through a real
  bundler manifest, and render the result with `react-dom`. Opt-in:

  ```sh
  cd testdata/roundtrip && npm install
  RSC_ROUNDTRIP=1 go test ./rsc/...
  ```

The second kind is the one that proves compatibility. Two encoders can agree
with each other and both be wrong about what the decoder accepts.

## What is faithfully reproduced

| | |
| --- | --- |
| Element tuples | `["$", type, key, props]`, keys as strings or `null` |
| Server components | evaluated before serialization, exactly as React does |
| Client references | `I` rows with hex chunk ids; one row per reference, reused |
| Element-valued props | rendered **in place**, with surrounding context intact |
| Authored children shape | `null`, `false`, `true`, `0`, `""` preserved; nested arrays stay nested; a lone child is collapsed, matching `createElement` |
| Numbers | stay numbers — the child `7` is not the child `"7"` |
| Special values | `$undefined`, `$Infinity`, `$-Infinity`, `$NaN`, `$-0`, `$D<iso>` with JS millisecond precision |
| `$` escaping | a literal `"$foo"` is sent as `"$$foo"` |
| JSON escaping | HTML characters left unescaped, matching React byte for byte |

## Limitations

Stated plainly, because a compatibility layer that overstates itself is worse
than none.

- **Prop order is sorted, not authored.** Go maps have no order, and `react.Props`
  is a map, so the information is gone before the encoder sees it. Sorting is
  deterministic and semantically identical — React matches attributes by name —
  but rendered HTML can differ from React's in attribute *order*.
- **No streaming.** The whole tree is rendered, then written. React's server can
  flush a shell and stream Suspense boundaries as data resolves; this cannot.
  Consequently a suspended subtree serializes its **fallback**, not its content.
- **No Suspense boundaries in the payload.** Boundaries resolve during the Go
  render and are not transmitted, so a client component that suspends on the
  client has no boundary above it.
- **No server actions / server references** (`$h` rows). Function props are
  rejected with an error rather than silently dropped.
- **No async Go components.** React server components may be `async`; Go
  components are synchronous. Resolve data before rendering.
- **A child cannot be `undefined`.** Go has one empty value. A `nil` child is
  `null`. `rsc.Undefined` covers the prop case, where the distinction is real.
- **Not emitted:** temporary references, blobs, typed arrays, `Map`/`Set`,
  async iterables, readable streams, hints (`H` rows).

## Stability

Flight is **internal, undocumented and version-coupled**. React makes no
compatibility promise about it across releases. `TargetVersion` records the
version this encoder was verified against; re-run the round trip on every React
upgrade. The tests exist so a mismatch fails loudly in CI rather than showing up
as a blank page in production.

This is a permanent maintenance tax, not a one-time cost. It is the price of the
only architecture that makes Go-authored components work with a real React
client.
