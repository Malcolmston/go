# `streamlit` parity coverage

| | |
| --- | --- |
| upstream oracle | `streamlit==1.61.1` (PyPI), installed into `python/.venv` — never globally |
| Go module | `github.com/malcolmston/streamlit v0.3.0` (resolved by `GOWORK=off go get …@latest`, no `replace`) |
| cases | 55 |
| case parity | **38 / 55 = 69.09 %** (see `parity.json`) |
| symbol parity | **29 / 44 = 65.91 %** of the symbols actually compared; 29 / 106 = 27.36 % of the whole upstream surface |

## What is compared, and why

Streamlit's real interface is not its Python signatures, it is the **protobuf
element tree** — the `ForwardMsg`/`Delta` stream a browser consumes. Streamlit
ships that tree as a first-class testing surface: `streamlit.testing.v1.AppTest`
runs a script headlessly and hands back the resulting elements, widget values
and session state. That is the oracle here. No browser is started and
`streamlit run` is never invoked.

The unit of comparison is therefore a **logical app**, defined twice:

* upstream as a Python script string in `python/apps.py`, run with
  `AppTest.from_string(...)`;
* the port as a `func(*st.Session)` in `go/apps.go`.

Each case runs its app, then walks a scripted list of interactions, and after
every interaction records one **snapshot**: the main body and the sidebar as a
canonical element description (each element's type, its salient properties, and
for containers its children, in document order) plus the app's own session-state
keys. Comparing snapshot *sequences* rather than a single tree is what makes
rerun semantics, form atomicity, caching and state persistence observable at
all.

### The canonical description

Both runners map their native element vocabulary onto one shared vocabulary
named after Streamlit's own element types (`title`, `alert`, `slider`,
`columns`, …). The properties compared by default are the ones both
implementations model: `label`, `value`, `options`, `min`, `max`, `step`,
`index`, plus the container-specific `labels`/`expanded`/`n`, table
`header`/`rows`, chart `kind`/`encoding`/`series`, and `metric`'s derived
`direction`/`color`.

Properties that only one side models are **opt-in per case**
(`{"extra": ["help", "disabled", …]}`). Without that split, a missing `help`
field would fail every case that merely happens to contain a widget and drown
out the cases about widget *values*; with it, each such gap gets exactly one
case of its own (`widgets-presentation`, `button-value-prop`,
`number-input-step`).

### Normalisation (identical on both sides)

* **Widget identity.** An explicitly-keyed widget reports its key. An unkeyed
  widget reports `#<n>`, its zero-based position among same-typed widgets in
  document order — Streamlit's real ids are content hashes
  (`$$ID-<sha>-<key>`) and the port's are `auto-<type>-<n>`, so neither is
  comparable verbatim. Form submit buttons are positional on both sides because
  neither implementation lets the app name one.
* **Numbers.** All JSON numbers are compared as `float64`, so `1` and `1.0`
  agree. Values the app *observed* are published through session state
  (`ss_*`) rather than rendered as text, so the two languages' float formatting
  never enters the comparison.
* **Dates and times** are ISO-8601 / `HH:MM` strings on both sides.
* **JSON payloads** (`st.json`) are re-parsed and re-emitted with sorted keys.
* **`st.latex`** upstream wraps its body in `$$\n…\n$$`; the wrapper is stripped.
* **Tables** are compared as a header row plus stringified cells, because the
  port's tabular input is `[][]string`/`[]struct` and has no dtypes.
* Map keys are sorted before emitting, on both sides.

### Interaction protocol

`run` (rerun with no event), `set` (change a widget, addressed by key or by
`(type, index)`), `click` (a button), `stage` (change a form widget *without*
reaching the server) and `submit` (commit every staged value with the submit
button). `stage` produces no snapshot on either side, because no request is
made.

## Usability findings from building the harness

1. **The port's own wire types are unexported.** Driving `st.Handler`
   in-process — the only way to reach the element tree, since `st.Session` is
   only constructible by the server — means POSTing to `/api/run`, whose request
   and response bodies (`st.runRequest`, `st.runResponse`, `st.event`) are
   unexported. `go/run.go` re-declares all three by hand. There is no exported
   `Run(app, event) *Element`, no exported session constructor, and no testing
   package, so every consumer that wants to test an app must re-derive the
   protocol from the source.
2. **`st.State` cannot be enumerated at v0.3.0.** It exposes
   `Get/Set/Delete/GetString/GetInt/GetFloat` and nothing else — no `Keys`,
   `Len`, `Has`, `SetDefault` or `Clear`. Where the Python side simply iterates
   `st.session_state`, each Go app has to name the keys it publishes
   explicitly (`appCtx.put`).
3. **`Session.Cache` takes a key, not arguments.** `@st.cache_data` hashes the
   call's arguments; the port makes the caller splice them into a string key
   (`cacheKey` in `go/apps.go`). Getting that wrong is a silent correctness
   bug, and there is no per-function `.clear()` — only the process-wide
   `st.CacheClear`.
4. **Buttons are not observable in the tree.** The port's `button` element
   carries only a label, so a test can learn whether a button fired only by
   having the app write it somewhere. Upstream's `Button.value` is part of the
   element.

## APIs expected of the tag but absent

The published `v0.3.0` tag lags the working tree. Everything below exists in the
local checkout and **not** in the module the harness resolved, so it is scored
`missing`:

`Session.Rerun`, `Session.CacheResource`, `Container.ColumnsWeighted`,
`Container.MarkdownUnsafe`, `Container.BorderedContainer`,
`State.GetBool/Has/Len/Keys/Clear/SetDefault`, `CacheDelete`, `CacheLen`,
`CacheResourceClear`, `CacheSetMaxEntries`, `HandlerWithOptions`,
`RunWithOptions`, `Options`.

Two behaviours in the released tag are also older than the tree: `Slider`
clamps an out-of-range value instead of widening its range, and the released
`Session` has **no stale-widget pruning** at all (no `seen` set, no
`pruneStaleWidgets`). Both show up as mismatches below.

## Divergences found

Ordered worst first: wrong widget values and state semantics before missing
element properties.

### 1. Out-of-range persisted widget values (`slider-above-max`, `slider-below-min`, `number-above-max`, `number-below-min`)

Upstream **rejects** a persisted value outside `[min, max]` and falls back to
the declared default; the port **clamps** it to the nearest bound.

| | upstream | Go v0.3.0 |
| --- | --- | --- |
| slider `0..10`, default `3`, stored `50` | `3` | `10` |
| slider `0..10`, default `3`, stored `-5` | `3` | `0` |
| number `0..10`, default `5`, stored `99` | `5` | `10` |
| number `0..10`, default `5`, stored `-3` | `5` | `0` |

The app therefore reads a value the user never chose. (The unreleased tree
changes this again — it widens the range to admit the value — which matches
neither.)

### 2. A widget hidden by a branch keeps its stale value (`widget-hidden-then-shown`)

Set `text_input` to `"grace"`, hide it, show it again. Upstream returns the
default `"ada"` — the widget's state was discarded when it stopped being drawn.
The port returns `"grace"`: `v0.3.0` never prunes widget state, so values
persist for the life of the session (and the per-session map grows without
bound, since keys arrive from the client).

### 3. Auto-generated widget keys are call-order dependent (`auto-key-shift`)

Confirmed. With two unkeyed sliders where the first is conditional:

| step | upstream | Go v0.3.0 |
| --- | --- | --- |
| set "Target" to `42` | Target = `42` | Target = `42` |
| reveal "Extra" | Extra = `0`, Target = `42` | **Extra = `10`**, **Target = `5`** |

The port's keys are `auto-<type>-<n>` off a single per-run counter, so revealing
a widget shifts every later key: "Extra" silently inherits Target's `42`
(clamped to its own `0..10` range) and Target reverts to its default. Streamlit
derives ids from the widget's own parameters, so identity survives a shape
change. This is a correctness class, not a cosmetic one — any conditional
widget can reassign a later widget's value.

### 4. `Session.Stop` is defeated by `recover()` in app code (`stop-swallowed-by-app`)

Confirmed. `st.stop()` asks the script runner to stop and yields; it raises
nothing catchable, so wrapping it in `except BaseException` changes nothing —
upstream renders `["before"]`. `Session.Stop` panics an unexported sentinel, so
an app with a `defer recover()` anywhere up the stack continues: the port
renders `["before", "swallowed", "after"]`. Because the sentinel type is
unexported, app code cannot even re-panic selectively; any `recover()` in an
app (a perfectly normal defensive idiom) silently disables `Stop`.

### 5. The port accepts widget values its own options exclude (`selectbox-unknown-value`)

Upstream cannot express the state at all — `AppTest` raises
`ValueError: 'delta' is not in list`, mirroring a frontend that can only send
one of the options. `POST /api/run` accepts any JSON value for any key; the port
then quietly substitutes the first option. Same story for `multiselect`, where
off-list entries are dropped (that case matches, because upstream drops them
too).

### 6. Charts carry no data (`chart-line`, `chart-area`, `chart-bar`, `chart-scatter`)

Upstream emits a `vega_lite_chart` element: a Vega-Lite spec (mark `line` /
`area` / `bar` / `circle`) plus the series arrow-encoded, so the data is
inspectable, themeable and interactive. The port renders the chart to an inline
SVG string on the server; the element has no mark kind and no series at all.
Nothing downstream can re-scale, re-theme, tooltip or export it. This is the
single largest structural gap in the port.

### 7. `st.metric` derives nothing (`metric`)

Upstream resolves the arrow direction and delta colour server-side
(`direction: UP`, `color: GREEN`). The `v0.3.0` element carries only
`label`/`value`/`delta`/`deltaColor`, leaving the sign interpretation to the
frontend.

### 8. `st.write`'s dispatch differs for non-strings (`write-dispatch`)

| value | upstream | Go v0.3.0 |
| --- | --- | --- |
| `"a string"` | markdown, body `a string` | markdown, body `a string` |
| `3` | markdown, body ``` `3` ``` | text, body `3` |
| `True` | markdown, body ``` `True` ``` | text, body `true` |

Upstream renders scalars as inline code; the port renders them as preformatted
text, and Go's `true` is not Python's `True`.

### 9. Widgets model none of the presentation parameters (`widgets-presentation`, `button-value-prop`, `number-input-step`)

The port's widget elements have no `help`, `disabled`, `placeholder` or
`format` property — the parameters do not exist on any signature. `max_chars`
exists only through the Go-only `TextInputMax`. A number input has no `step`
(upstream defaults to `0.01` for floats). A button element has no clicked state.

## What matched

Everything else, notably the parts most likely to be wrong:

* all text elements and all four alert kinds, in document order;
* the default value, label and options of the whole widget set, and the
  round-trip of a set value for checkbox, radio, selectbox, multiselect,
  slider, text input, text area, number input, date input and time input;
* dropping a multiselect entry that is no longer an option;
* `columns` / `tabs` / `expander` / `container` / `sidebar`, nested, and a
  widget inside a column still round-tripping its value;
* `dataframe`, `table` and `json`;
* forms: staged values stay invisible until submit, then commit atomically, and
  the submit button reads true for exactly one run;
* session state surviving reruns, and a button reading true for **exactly one
  run** (`button-one-run`, `session-state-counter`);
* caching: a hit does not recompute, a changed key does, and explicit
  invalidation forces exactly one recomputation;
* `st.stop` halting a run and the next run recovering.

## The upstream API inventory

Enumerated mechanically from the installed package, not from the docs:

```
$ python/.venv/bin/python -c "import streamlit as st; \
    print([n for n in dir(st) if not n.startswith('_') and callable(getattr(st, n))])"
```

That yields **104 public callables**. Two public non-callable attributes are
also core to the surface under test and are listed with them —
`st.session_state` and `st.sidebar` — for a denominator of **106**. Public
non-callables that are plainly internal plumbing (`st.proto`, `st.runtime`,
`st.delta_generator`, `st.type_util`, and 38 others) are excluded.

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `st.App` | — | missing | — | multipage apps not ported |
| `st.Page` | — | missing | — | multipage apps not ported |
| `st.altair_chart` | — | missing | — | no Vega/Altair spec support |
| `st.area_chart` | `st.Container.AreaChart` | differs | `chart-area` |  |
| `st.audio` | `st.Container.Audio` | untested | — |  |
| `st.audio_input` | `st.Container.AudioInput` | untested | — | bytes arrive over multipart /api/upload |
| `st.badge` | `st.Container.Badge` | untested | — |  |
| `st.balloons` | `st.Container.Balloons` | untested | — |  |
| `st.bar_chart` | `st.Container.BarChart` | differs | `chart-bar` |  |
| `st.button` | `st.Container.Button` | differs | `wd-button`, `widgets-defaults-all`, `button-value-prop`, `button-one-run`, `session-state-counter` | element carries no clicked state |
| `st.cache` | — | missing | — | removed upstream alias; nothing to port |
| `st.cache_data` | `st.Session.Cache` | match | `cache-hit-then-miss`, `cache-invalidate` | key must be derived by the caller; no arg hashing, no per-function .clear() |
| `st.cache_resource` | — | missing | — | Session.CacheResource exists only in the unreleased tree, not in v0.3.0 |
| `st.camera_input` | `st.Container.CameraInput` | untested | — |  |
| `st.caption` | `st.Container.Caption` | match | `text-elements` |  |
| `st.chat_input` | `st.Container.ChatInput` | untested | — |  |
| `st.chat_message` | `st.Container.ChatMessage` | untested | — |  |
| `st.checkbox` | `st.Container.Checkbox` | differs | `wd-checkbox`, `set-checkbox`, `widgets-presentation` | no help/disabled |
| `st.code` | `st.Container.Code` | match | `text-elements` |  |
| `st.color_picker` | `st.Container.ColorPicker` | untested | — |  |
| `st.columns` | `st.Container.Columns` | match | `layout-basic`, `layout-nested`, `layout-widget-in-column` | ColumnsWeighted (relative widths) exists only in the unreleased tree |
| `st.connection` | — | missing | — | no data-source connections |
| `st.container` | `st.Container.Container` | match | `layout-basic` | border=True only in the unreleased tree |
| `st.data_editor` | — | missing | — | no editable grid |
| `st.dataframe` | `st.Container.DataFrame` | match | `dataframe` | takes [][]string / []struct, not a DataFrame; cells are pre-stringified |
| `st.date_input` | `st.Container.DateInput` | match | `wd-date_input`, `set-date-input` | ISO strings, no min/max, no range form at this tag |
| `st.datetime_input` | — | missing | — |  |
| `st.dialog` | — | missing | — | no modal dialogs |
| `st.divider` | `st.Container.Divider` | match | `text-elements` |  |
| `st.download_button` | `st.Container.DownloadButton` | untested | — |  |
| `st.echo` | `st.Container.Echo` | untested | — | takes the source as a string; cannot read its own source |
| `st.empty` | `st.Container.Empty` | untested | — |  |
| `st.error` | `st.Container.Error` | match | `text-elements` |  |
| `st.exception` | `st.Container.Exception` | untested | — |  |
| `st.expander` | `st.Container.Expander` | match | `layout-basic`, `layout-nested` |  |
| `st.feedback` | `st.Container.Feedback` | untested | — |  |
| `st.file_uploader` | `st.Container.FileUploader` | match | `wd-file_uploader`, `widgets-defaults-all` | uploads only via multipart POST /api/upload; not driven by a case |
| `st.form` | `st.Container.Form` | match | `form-atomic-commit`, `form-submit-without-changes` |  |
| `st.form_submit_button` | `st.Container.FormSubmitButton` | match | `form-atomic-commit`, `form-submit-without-changes` | reuses the form key as its widget key |
| `st.fragment` | — | missing | — | no partial reruns |
| `st.get_option` | — | missing | — | no config system |
| `st.graphviz_chart` | — | missing | — |  |
| `st.header` | `st.Container.Header` | match | `text-elements` |  |
| `st.help` | `st.Container.Help` | untested | — |  |
| `st.html` | `st.Container.Html` | untested | — |  |
| `st.iframe` | — | missing | — |  |
| `st.image` | `st.Container.Image` | untested | — |  |
| `st.info` | `st.Container.Info` | match | `text-elements` |  |
| `st.json` | `st.Container.JSON` | match | `json-element` |  |
| `st.latex` | `st.Container.Latex` | match | `text-elements` |  |
| `st.line_chart` | `st.Container.LineChart` | differs | `chart-line` |  |
| `st.link_button` | `st.Container.LinkButton` | untested | — |  |
| `st.login` | — | missing | — | no OIDC auth |
| `st.logo` | `st.Container.Logo` | untested | — |  |
| `st.logout` | — | missing | — | no OIDC auth |
| `st.map` | `st.Container.Map` | untested | — |  |
| `st.markdown` | `st.Container.Markdown` | differs | `text-elements`, `write-dispatch` | unsafe_allow_html only in the unreleased tree |
| `st.menu_button` | — | missing | — |  |
| `st.mermaid_chart` | — | missing | — |  |
| `st.metric` | `st.Container.Metric` | differs | `metric` | element carries no derived direction/color at this tag |
| `st.multiselect` | `st.Container.MultiSelect` | match | `wd-multiselect`, `set-multiselect`, `multiselect-unknown-value` | no default= parameter: always starts empty |
| `st.navigation` | — | missing | — | multipage apps not ported |
| `st.number_input` | `st.Container.NumberInput` | differs | `wd-number_input`, `set-number-input`, `number-above-max`, `number-below-min`, `number-input-step` | no step; bounds only via the Go-only NumberInputRange |
| `st.page_link` | `st.Container.PageLink` | untested | — |  |
| `st.pagination` | — | missing | — |  |
| `st.pdf` | — | missing | — |  |
| `st.pills` | `st.Container.Pills` | untested | — |  |
| `st.plotly_chart` | — | missing | — |  |
| `st.popover` | `st.Container.Popover` | untested | — |  |
| `st.progress` | `st.Container.Progress` | untested | — |  |
| `st.pydeck_chart` | — | missing | — |  |
| `st.pyplot` | — | missing | — |  |
| `st.radio` | `st.Container.Radio` | match | `wd-radio`, `set-radio` | no index=/default parameter: always the first option |
| `st.rerun` | — | missing | — | Session.Rerun exists only in the unreleased tree, not in v0.3.0 |
| `st.scatter_chart` | `st.Container.ScatterChart` | differs | `chart-scatter` |  |
| `st.segmented_control` | `st.Container.SegmentedControl` | untested | — |  |
| `st.select_slider` | `st.Container.SelectSlider` | untested | — |  |
| `st.selectbox` | `st.Container.SelectBox` | differs | `wd-selectbox`, `set-selectbox`, `selectbox-unknown-value` | accepts an off-list value from the client where upstream cannot express one |
| `st.set_option` | — | missing | — | no config system |
| `st.set_page_config` | `st.Session.SetPageConfig` | untested | — | title and icon only |
| `st.skeleton` | — | missing | — |  |
| `st.slider` | `st.Container.Slider` | differs | `wd-slider`, `set-slider`, `slider-above-max`, `slider-below-min` | clamps an out-of-range persisted value; upstream reverts to the default |
| `st.snow` | `st.Container.Snow` | untested | — |  |
| `st.space` | — | missing | — |  |
| `st.spinner` | `st.Container.Spinner` | untested | — | decorative only: the tree ships after the run finishes |
| `st.status` | `st.Container.Status` | untested | — |  |
| `st.stop` | `st.Session.Stop` | differs | `stop-halts-run`, `stop-swallowed-by-app` | panics an unexported sentinel a recover() in app code swallows |
| `st.subheader` | `st.Container.Subheader` | match | `text-elements` |  |
| `st.success` | `st.Container.Success` | match | `text-elements` |  |
| `st.switch_page` | — | missing | — | multipage apps not ported |
| `st.table` | `st.Container.Table` | match | `table` |  |
| `st.tabs` | `st.Container.Tabs` | match | `layout-basic` |  |
| `st.text` | `st.Container.Text` | match | `text-elements`, `layout-basic` |  |
| `st.text_area` | `st.Container.TextArea` | match | `wd-text_area`, `set-text-area` | no max_chars/placeholder/height |
| `st.text_input` | `st.Container.TextInput` | differs | `wd-text_input`, `set-text-input`, `layout-widget-in-column`, `widgets-presentation` | no help/disabled/placeholder; max_chars only via the Go-only TextInputMax |
| `st.time_input` | `st.Container.TimeInput` | match | `wd-time_input`, `set-time-input` | "15:04" strings, no step |
| `st.title` | `st.Container.Title` | match | `text-elements` |  |
| `st.toast` | `st.Container.Toast` | untested | — |  |
| `st.toggle` | `st.Container.Toggle` | untested | — |  |
| `st.vega_lite_chart` | — | missing | — | charts are server-rendered SVG, not a Vega-Lite spec |
| `st.video` | `st.Container.Video` | untested | — |  |
| `st.warning` | `st.Container.Warning` | match | `text-elements` |  |
| `st.write` | `st.Container.Write` | differs | `write-dispatch` | int/bool render as plain text, upstream renders them as inline code |
| `st.write_stream` | — | missing | — | no streaming output |
| `st.session_state` | `st.Session.State` | differs | `session-state-counter`, `widget-hidden-then-shown`, `form-atomic-commit` | no iteration: no Keys/Len/Has/SetDefault/Clear at this tag |
| `st.sidebar` | `st.Session.Sidebar` | match | `layout-basic` |  |
| — | `st.Handler` / `st.Run` | extra | — | the HTTP surface; Streamlit has no library-level equivalent (`streamlit run` is a CLI) |
| — | `st.CacheClear` | extra | — | process-wide cache reset; upstream's is `st.cache_data.clear()` |
| — | `st.Container.PrimaryButton` | extra | — | upstream spells it `st.button(type="primary")` |
| — | `st.Container.PasswordInput` | extra | — | upstream spells it `st.text_input(type="password")` |
| — | `st.Container.TextInputMax` | extra | — | upstream spells it `st.text_input(max_chars=…)` |
| — | `st.Container.NumberInputRange` | extra | — | upstream spells it `st.number_input(min_value=, max_value=)` |
| — | `st.Container.SliderRange` | extra | — | upstream spells it `st.slider(value=(lo, hi))` |
| — | `st.Container.SelectSliderRange` | extra | — | upstream spells it `st.select_slider(value=(lo, hi))` |
| — | `st.Container.DateRangeInput` | extra | — | upstream spells it `st.date_input(value=(a, b))` |
| — | `st.Container.MetricColored` | extra | — | upstream spells it `st.metric(delta_color=…)` |
| — | `st.Container.PieChart` | extra | — | no upstream equivalent (`st.plotly_chart`/Vega only) |
| — | `st.Container.Histogram` | extra | — | no upstream equivalent |
| — | `st.Session.ID` | extra | — | opaque session id; upstream has no public equivalent |
| — | `st.State.GetString/GetInt/GetFloat` | extra | — | typed accessors; Python needs none |

## Counts

| status | symbols |
| --- | --- |
| `match` | 29 |
| `differs` | 15 |
| `missing` | 30 |
| `untested` | 32 |
| `extra` | 14 (Go-only, outside the denominator) |
| **denominator** | **106** (104 public callables + `st.session_state` + `st.sidebar`) |

* **Symbol parity over what was compared: 29 / 44 = 65.91 %.**
* Symbol parity over the whole upstream surface: 29 / 106 = 27.36 %.
* **Case parity: 38 / 55 = 69.09 %**, 0 deliberate deviations.

A symbol with no case is `untested`, never `match`. The 32 untested symbols are
ported entry points nobody drove: the media elements (`image`, `audio`,
`video`, `logo`, `map`), the effect/annotation elements (`balloons`, `snow`,
`toast`, `badge`, `html`, `help`, `exception`, `echo`, `empty`, `spinner`,
`progress`), the chat elements, the newer selection widgets (`pills`,
`segmented_control`, `select_slider`, `color_picker`, `toggle`, `feedback`,
`camera_input`, `audio_input`, `download_button`), and `popover`, `status`,
`link_button`, `page_link`, `set_page_config`.

## Scale of what is not ported

30 of 106 upstream symbols have no Go counterpart at all. They cluster:

* **multipage apps** — `st.App`, `st.Page`, `st.navigation`, `st.switch_page`,
  `st.pagination`;
* **the whole third-party chart surface** — `altair_chart`,
  `vega_lite_chart`, `plotly_chart`, `pydeck_chart`, `graphviz_chart`,
  `pyplot`, `mermaid_chart`;
* **execution model** — `st.rerun` (absent from the tag), `st.fragment`
  (partial reruns), `st.dialog`, `st.write_stream`, `st.cache_resource`
  (absent from the tag);
* **data** — `st.data_editor`, `st.connection`, `st.column_config`;
* **auth and config** — `st.login`, `st.logout`, `st.get_option`,
  `st.set_option`, `st.secrets`, `st.context`, `st.query_params`;
* **misc elements** — `st.iframe`, `st.pdf`, `st.menu_button`, `st.skeleton`,
  `st.space`, `st.datetime_input`.

## Reproducing

```
python3 -m venv parity/streamlit/python/.venv
parity/streamlit/python/.venv/bin/pip install 'streamlit==1.61.1'
GOWORK=off go test ./parity/streamlit/
```

The suite `t.Skip`s (never fails) when `python3`, the venv, or the pinned
`streamlit` install is missing, and refuses to score at all if the installed
version is not the pin. A partial run (`-run` filter) leaves `parity.json`
untouched.
