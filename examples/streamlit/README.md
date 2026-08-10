# streamlit example

A runnable validation program for the published Go module
[`github.com/malcolmston/streamlit`](https://github.com/Malcolmston/streamlit) —
a Streamlit-style data-app framework whose public API lives in the `st`
subpackage.

- **Resolved module version: `github.com/malcolmston/streamlit v0.3.0`**
- Consumed as a published module: no `replace` directive, no reference to the
  sibling `../../streamlit` working tree.

## What it demonstrates

The program declares one realistic app function (`func(*st.Session)`) that uses
the library's primary features, mounts it with `st.Handler` on an
`httptest.NewServer` bound to a random localhost port, drives it over the real
`POST /api/run` JSON protocol, asserts on the returned `*st.Element` trees, and
shuts the server down. It always terminates and exits non-zero if any check
fails.

Features exercised:

| Area | API |
| --- | --- |
| Page config / regions | `SetPageConfig`, `Sidebar`, root `sidebar`/`main` split |
| Text & markdown | `Title`, `Subheader`, `Header`, `Markdown`, `Caption`, `Latex`, `Code`, `Text`, `Info`/`Success`/`Warning` |
| Charts (server-rendered SVG) | `LineChart`, `AreaChart`, `BarChart`, `PieChart`, `ScatterChart`, `Histogram`, `Map` + `st.MapPoint` |
| Dataframes | `DataFrame` (struct-slice reflection, sortable), `Table` (`[][]string`), `JSON` |
| Layout | `Columns`, `Tabs`, `Expander`, `BorderedContainer`, `Status` |
| Widgets | `SelectBox`, `Slider`, `Checkbox`, `NumberInput`, `SegmentedControl`, `MultiSelect`, `Radio`, `Pills`, `Toggle`, `TextInput`, `TextArea`, `ColorPicker`, `DateInput`, `SelectSlider`, `Feedback`, `Button`, `PrimaryButton`, `DownloadButton`, `LinkButton` |
| Forms | `Form`, `FormSubmitButton` (staged values committed atomically) |
| Metrics & effects | `Metric`, `MetricColored`, `Progress`, `Badge`, `Help`, `Toast`, `Balloons` |
| Caching | `Session.Cache` with TTL, `st.CacheClear` |
| Session state | `s.State.Set/GetInt`, cross-rerun persistence, per-session isolation |
| Control flow | `Session.Stop` truncating a run without leaking a panic |
| Server | `st.Handler`, `/api/run`, `/api/upload`, embedded `/`, `/app.js`, `/style.css` |

Verified behaviours include: widget values persisting across reruns, slider
range clamping, buttons reading `true` for exactly one run, forms committing all
staged values at once, dataframe struct-field reflection into headers, inline
SVG generation for every chart kind, process-wide cache memoisation surviving
reruns and being invalidated by `CacheClear`, and two sessions being isolated.

## How to run

```sh
cd examples/streamlit
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

`GOWORK=off` is required so the repo's `go.work` does not silently substitute
the local library checkout for the published module.

Status: **builds and runs clean; all 26 checks pass.** No outbound network
beyond the initial module download.

## Holes and rough edges found in v0.3.0

Nothing was missing that stopped the example compiling — every API this program
needed exists in the published tag, and no HOLE had to be commented out of the
app itself. The problems are ergonomic:

1. **No exported in-process render entry point.** The element tree is the
   library's whole value proposition, but the only way to produce one is to go
   through HTTP. `Session.run`, `newSession`, and `Session.reset` are all
   unexported, and there is no exported `Render(app) *st.Element`. The package's
   own `ExampleSession` in `example_test.go` calls `newSession()` and
   `s.run(app)` — an in-package example that an outside caller cannot reproduce.
   This example therefore has to stand up an `httptest` server just to snapshot
   a tree, and unit-testing a `func(*st.Session)` from another package is
   impossible without one.

2. **The wire protocol types are unexported.** `runRequest`, `runResponse`, and
   `event` are private, so any external driver, integration test, or
   alternative frontend must hand-redeclare all three structs (this example
   does, in `main.go`) by reading the server source. Only `st.Element` is
   exported. `Handler`'s doc comment explicitly advertises being "driven in
   tests", which those unexported types make awkward.

3. **`go doc` hides ~90 methods.** `Session` embeds `*ctr`, an unexported alias
   of `Container`. `go doc st.Session` consequently lists only `Cache`, `ID`,
   `SetPageConfig`, `Sidebar`, and `Stop` — none of `Title`, `Slider`,
   `LineChart`, `DataFrame`, etc., even though they are the API you actually
   call on the session. The doc comment explains the trick but discoverability
   is genuinely worse for it; the methods are only findable under `Container`.

4. **The multipart upload format is undocumented.** `FileUploader`,
   `CameraInput`, and `AudioInput` return `[]UploadedFile` only after bytes
   arrive at `POST /api/upload`, whose contract (`session`, `key` form fields
   plus `files` parts) appears nowhere in the README or godoc — only in the
   handler source. This example can only assert that the endpoint rejects a
   non-multipart body; see the `// HOLE:` comment in section 11 of `main.go`.

5. **`Session.Stop` halts via `panic`.** It panics an unexported `stopSignal`
   recovered by `runApp`. It works, and a non-sentinel panic propagates
   correctly, but any `recover()` in app code (or in a library the app calls)
   will swallow it and silently change control flow. There is no documented
   guidance about that.

6. **`Cache` is untyped and globally keyed.** It returns `any`, forcing a type
   assertion at every call site (`cached.([]Row)`), and the key namespace is
   process-wide with no per-key invalidation — only `CacheClear()`, which nukes
   everything. A generics-based `Cache[T]` would be the idiomatic Go shape here.

7. **`Container.Error(string)` vs `Container.Exception(error)`.** `Error` takes a
   string and renders a red alert; it does not satisfy any error-related
   convention and reads like it should accept an `error`. Minor, but a name that
   invites the wrong call.

8. **Auto-generated widget keys are call-order-dependent.** `key()` falls back to
   `auto-<type>-<ordinal>`, so any conditional widget shifts every subsequent
   auto key and silently reassigns persisted values. Documented as requiring
   "stable structure", but there is no detection or warning. This example passes
   explicit keys to every widget for that reason.

No compile failures, no runtime panics, no rendering defects, and no dependency
problems were observed: the module has zero third-party dependencies and builds
against Go 1.24.7.
