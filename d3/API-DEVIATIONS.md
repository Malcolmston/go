# API deviations

This port mirrors the API of [d3](https://github.com/d3/d3) on purpose: names,
algorithms, tick selection, format grammar and path output are chosen to match
the original. Where behavior or shape differs, it is listed here with the
reason.

Nothing in this file is a bug report. These are deliberate choices, and each one
is either forced by the language or by the absence of a browser.

## Scope: no DOM

d3 is really two libraries wearing one name. One half computes — scales,
shapes, interpolators, hierarchies, formats. The other half manipulates a live
document — selections, transitions, drag, zoom, brush. This port is the first
half.

| Upstream | Here | Why |
| --- | --- | --- |
| `d3-selection` — `d3.select`, `selectAll`, `.data()`, the enter/update/exit join | Not ported | The whole module's purpose is binding data to DOM nodes. There are no nodes. In Go the equivalent job is done by rendering a tree from data, which is what [`react`](https://github.com/malcolmston/react) does. |
| `d3-transition` — `.transition().duration(750)` | Not ported | A transition mutates attributes on a node over time against a frame clock. Both halves of that are missing. `d3/interpolate` and `d3/ease` — the actual math a transition runs — *are* ported, so a caller with their own clock has everything they need. |
| `d3-drag`, `d3-zoom`, `d3-brush` | Not ported | Pointer-event behaviours. There is no event to listen for. |
| `d3-dispatch` | Ported, generic over one payload type | Channels do not give you *named, namespaced, multi-listener* callbacks (`"start.foo"`), which is what the module actually provides. `Dispatch[T]` replaces untyped variadics and `this` with one typed argument, and — unlike upstream — is goroutine-safe, because Go callers will assume it is. |
| `d3-timer` — `d3.timer`, `timeout`, `interval` | Ported as a **clock**, not a frame scheduler | There is no display, so nothing here can know when a frame would have been; a 16 ms ticker dressed as `requestAnimationFrame` would promise a budget it cannot observe. What survives is real: one shared "now" per pass (so several animated quantities stay in step) and d3's elapsed-time contract, so ported easing code runs unchanged. `NewClock()` is advanced only by `Advance`/`AdvanceTo`, which makes a render a pure function of its step sequence — reproducible frames being the thing a server actually wants. `NewWallClock()` reads the real clock for callers who want one. |
| `selection.attr("d", line(data))` | `react.H("path", react.Props{"d": line.Line(data)})` | The seam between the two libraries is a string of SVG path data. That is the whole integration story, and it is deliberately narrow. |

**Since ported.** `d3-quadtree`, `d3-force`, `d3-chord`, `d3-polygon`,
`d3-dispatch`, `d3-timer`, `d3-delaunay`, `d3-contour`, `d3-geo` and
`d3-scale-chromatic` have landed, completing the computational surface. Several
carry deviations worth knowing:

- **`polygon` area is positive for a counter-clockwise ring *on screen*** —
  origin top-left, y pointing down, as in SVG and canvas. d3's `polygonArea` is
  the negation of the textbook shoelace sum, and it trips people because
  flipping the y-axis reverses apparent handedness: screen and mathematical
  conventions disagree by a sign on identical numbers. d3 resolves it in favour
  of the screen, and so does this port.
- **`force` is explicitly stepped.** d3 drives a simulation from a
  `requestAnimationFrame` timer; `Tick()` and `RunUntilStable()` put the caller
  in charge instead. For a server-rendered layout that is strictly better — the
  caller decides when the answer is final, and a fixed seed makes it
  reproducible.

- **`delaunay` and `contour` never mutate the caller's input**, where d3 both
  reads and writes the coordinate array it was handed. The cost is a copy; the
  benefit is that a triangulation cannot change under a caller who kept a
  reference. `Delaunay.Update()` is consequently absent — re-running `New` is
  the equivalent when there is no live array to update.
- **`geo` uses d3's winding order, not RFC 7946's.** Exterior rings must wind
  *clockwise*. Data wound the RFC way is not rejected — it is silently
  interpreted as the complement, giving `4π − A` from `Area`, an inverted
  `Contains`, and a path smeared across the projection. This is the single
  most likely way to misuse the package.
- **`geo.Bounds` is loose for polygons enclosing the north pole**, returning the
  whole globe rather than a tight box. That is d3's behaviour, reproduced
  deliberately for output parity rather than quietly improved.
- **Configuration errors panic** in `delaunay` and `contour` — a size below 1,
  a mismatched `values` length, an inverted extent — because there is no
  meaningful geometry to return instead.

`d3-chord` has no `chordTransitive`; upstream's third constructor is
`chordTranspose`, which is what `chord.NewTranspose` ports.

## A hazard that recurred across four packages

Go and JavaScript disagree about arithmetic in ways that produce plausible,
wrong output rather than errors. Four packages hit it independently, so it is
recorded here once rather than four times:

| Where | What |
| --- | --- |
| `array`, `path` | JS `Math.round` rounds halves toward +∞; Go's `math.Round` rounds away from zero. Negative tick domains drift a whole tick. Both packages use an internal `floor(x+0.5)`. |
| `format` | ECMA-262 `toFixed` rounds halves up; Go's `strconv` rounds to even, so `.0f` of 2.5 differs. Rounding is done on the exact value via `math/big.Rat`. |
| `ease`, `delaunay`, `geo` | On arm64 the compiler contracts `a*b - c*d` into an FMA. In `geo` it left ~1e-18 where two identical degree values converted to radians should have cancelled to zero. In `ease` it made `ExpOut(0)` return 2⁻⁶⁰ instead of 0; in `delaunay` it made a degenerate triangle test non-zero, corrupting the triangulation. Both forbid fusion with an explicit `float64(...)` conversion. |

The FMA case is the one to watch: it is invisible on x86 and only appears on
Apple Silicon.

## Getter/setter: d3's overloaded method is split

This is the single most pervasive difference, and it touches every configurable
object in the port — scales, shape generators, hierarchy layouts, bin
generators, DSV writers.

d3 overloads one method name for both roles: `scale.domain()` reads, and
`scale.domain([0, 100])` writes and returns the scale for chaining. Go has
neither argument-count overloading nor optional parameters, so one identifier
cannot be both.

**Two resolutions are in use, and which one a package picks depends on whether
reading the property back is useful.**

### Where reading matters: `Name` / `SetName` (`scale`)

| Upstream | Here | Why |
| --- | --- | --- |
| `scale.domain()` reads; `scale.domain([0, 100])` writes | `scale.Domain()` reads; `scale.SetDomain(0, 100)` writes and returns the receiver | A scale's configuration is genuinely read back — an axis renderer needs the domain, a legend needs the range — so both roles have to exist. The bare noun is the getter; `SetX` is the setter; chaining is preserved because setters return the receiver. |
| `scale.rangeRound([0, w])` | `scale.SetRangeRound(0, w)` | `rangeRound` is a setter with a side effect on the interpolator, so it takes the `Set` prefix like any other. |
| `scale(x)` — the scale is itself callable | `scale.Scale(x)` | Go has no callable value that also carries methods. `Scale` is the name for "apply this scale", used consistently: `Continuous.Scale`, `Band.Scale`, `Ordinal.Scale`. |
| Methods that are already verbs: `.nice()`, `.copy()`, `.ticks()`, `.invert()` | Unchanged, and still chain | They were never dual-role, so there is nothing to split. |

### Where reading does not matter: configuration is write-only (`shape`, `hierarchy`)

| Upstream | Here | Why |
| --- | --- | --- |
| `line.x(fn)` writes; `line.x()` reads | `line.X(fn)` writes and returns the receiver; **there is no getter** | Nobody reads a shape generator's accessors back — the generator is configured once and then called. Dropping the getter lets the setter keep the short, d3-shaped name, so a chain reads almost exactly as it does in JavaScript: `NewArc().InnerRadius(40).OuterRadius(80).CornerRadius(4)`. Hold on to the values you set if you need them. |
| `line.x(d => d.t)` and `line.x(40)` are the same method | Two methods. The plain name takes whichever form dominates in practice, and a suffix supplies the other: `Line.X(fn)` / `Line.XConst(v)` because a positional channel is nearly always an accessor, but `Arc.InnerRadius(v)` / `Arc.InnerRadiusFunc(fn)` and `Treemap.Padding(v)` / `Treemap.PaddingFunc(fn)` because a radius and a padding are nearly always constants. | Go cannot accept both a `float64` and a `func(T) float64` through one parameter. Which form gets the short name is decided per property by which one the call sites actually use, so the common case never carries a suffix — at the cost of having to remember which suffix a given property takes. |
| `line(data)` — the generator is callable | `line.Generate(data)` | Go has no callable value carrying methods. One verb across every generator, rather than a different noun per type. |

The split is worth calling out as a genuine inconsistency across the port: a
`scale` is configured with `SetDomain`, a `shape` with `X`, and a `hierarchy`
layout with `Size`. It is not an oversight, but it does mean the rule to
remember is "does anything read this back?" rather than one uniform prefix.

The same question decides invocation. `scale.Scale(x)`, `shape` generators'
`Generate(data)`, and a hierarchy layout's `Layout(root)` are three different
verbs because they do three different things: a scale returns a value, a
generator returns a string, and a layout annotates the tree it is handed and
returns it.

Two further consequences worth stating plainly:

- **A getter that returns a slice returns a defensive copy.** In JavaScript
  `scale.domain()` hands back a live array whose mutation is a documented
  footgun. Here it cannot be, at the cost of an allocation on a call that is
  not in any hot path.
- **Setters mutate the receiver.** They do not return a new scale. A scale is
  configured once and then called thousands of times, so d3's mutate-and-chain
  shape is both more faithful and cheaper than the immutable alternative. The
  cost is that a configured scale is not safe for concurrent *mutation*: finish
  configuring before publishing the pointer, or hand each goroutine a `Copy()`.
  Note this is the opposite of the sibling `chalk` package, whose `Style` is
  immutable; the difference is intentional and follows from how each value is
  used.

## Generics and accessors instead of duck typing

| Upstream | Here | Why |
| --- | --- | --- |
| `d3.mean(data, d => d.price)` — `data` is any iterable of anything | `array.Mean(data []T, value func(T) float64) (float64, bool)` | Go generics express "any element type, plus a function that gets a number out of it" directly, and check it at compile time. No reflection, no `interface{}`. |
| The accessor is `(d, i, data) => …` | **`array`**: `func(T) float64`. **`shape`**: `func(d T, i int, data []T) float64` | The two packages answer this differently on purpose. In `array` the index is used vanishingly rarely and costs every call site two ignored parameters, so it is dropped — where it genuinely matters, index the slice yourself and use the `…Of` form. In `shape` it is not rare: an accessor that positions a point by its index (`x: (d, i) => xScale(i)`) or that looks at its neighbours is ordinary charting code, so the full d3 signature is kept. |
| `d3.mean([1, 2, 3])` — no accessor needed for plain numbers | `array.MeanOf([]float64{1, 2, 3})` | `Mean(xs, func(x float64) float64 { return x })` reads badly and slices of numbers are the common case, so every statistic that reduces to a number ships in both flavors: the generic accessor form and an `…Of` form specialized to `[]float64`. |
| `d3.group(data, d => d.type)` returns an `InternMap` keyed by anything | `array.Group` is generic over a `comparable` key | Go map keys must be comparable. d3's `InternMap` exists to make `Date` and object keys work by coercion; Go's type system makes the constraint explicit instead. |
| A shape generator reads `d.x` / `d[0]` / `d.length` depending on what you passed | The generator is generic over the datum type and reads it through the accessors you set | d3 infers structure at runtime from whatever showed up. Here the datum type is a type parameter, so a missing accessor is a compile error rather than a chart of `NaN`s. |
| `d3.ascending` compares with `<` across mixed types | `array.Ascending` constrains to `cmp.Ordered`, and comparator forms take `func(a, b T) int` | The `int`-returning comparator is the shape the standard library's `slices` package uses, so these compose with `slices.SortFunc` directly instead of needing an adapter. |

## Empty input, `undefined`, and NaN

JavaScript returns `undefined` for "there was no answer". Go has no
`undefined`, and returning a silent zero would be a bug factory — an empty
dataset would render as a bar of height zero rather than as no bar at all.

| Upstream | Here | Why |
| --- | --- | --- |
| `d3.min([])` → `undefined` | `array.MinOf(nil)` → `(0, false)` | The second result is `ok`. It is `false` when there was nothing to compute from, and the value must not be used. Applies to min, max, extent, mean, median, mode, quantile, variance and deviation. |
| `d3.sum([])` → `0` | `array.Sum` returns `0` with no `ok` | The sum of nothing is conventionally zero, upstream agrees, and there is no ambiguity to report. The shape difference is deliberate, not an oversight. |
| `d3.min` skips `null`, `undefined` and `NaN` | The statistics skip `NaN` | `NaN` is this port's stand-in for "no value". `MeanOf([]float64{1, NaN, 3})` is `2`, not `NaN`, and a slice of nothing but `NaN` is empty for statistical purposes — `ok` is `false`. |
| `d3.ascending` puts `undefined` last | `Ascending` and `Descending` both place `NaN` **last** | A sort needs a total order, and this matches the `ascendingDefined` comparator d3 itself sorts with. Ordering functions therefore take the opposite view from the statistics: they keep `NaN` rather than skipping it. |
| `scale(NaN)` → `NaN` | Same | Propagating is correct. Guarding `NaN` into `0` produces a chart that is quietly wrong instead of visibly broken. |
| A malformed color string yields `null` | `color.Parse` returns `(Color, error)`; `color.MustParse` panics | An unparseable color is a programmer error at a call site, and Go reports those with an error. `MustParse` exists for package-level constants where a panic at init is the right outcome. |
| A malformed format specifier throws at call time | Parsed and reported as a typed error | Same reasoning. A specifier is usually a literal, so the error surfaces immediately in a test. |
| A malformed CSV row is coerced or dropped | Reported as an error naming the row | Silent coercion is how bad data reaches a chart unnoticed. |

The general rule: **where d3 silently coerces, this port returns an error or an
`ok`; where d3 propagates `NaN` through arithmetic, so does this port.** The two
are different situations and get different treatment.

## Output: strings, not contexts

| Upstream | Here | Why |
| --- | --- | --- |
| `d3.path()` and a canvas `CanvasRenderingContext2D` are interchangeable sinks; `line.context(ctx)` draws to canvas instead of returning a string | `shape.Context` is a four-method interface (`MoveTo`, `LineTo`, `BezierCurveTo`, `ClosePath`) that a curve writes into, and `shape.PathContext` wraps a `*path.Path` as one. Generators return a `string`. | There is no canvas, so the pluggable-sink machinery upstream needs for it is reduced to what curves actually require. The interface survives because the package uses it internally to transform coordinates — `CurveRadial` converts (angle, radius) to (x, y) by wrapping a context — which is a better justification for it than canvas support was. |
| `line(data)` returns `null` when the data produces no segments | `line.Generate(data)` returns `""` | Go has no `null` for a `string`, and an empty path attribute is the correct SVG for "draw nothing". Check for the empty string before emitting a `<path>`, exactly where you would have checked for `null`. |
| Path number formatting is JavaScript's default `String(number)` | The shortest round-tripping decimal representation, matching JS output for the values a chart actually produces | `strconv.FormatFloat(v, 'f', -1, 64)`-style formatting. Go's `%v` would emit exponent notation where JavaScript does not, which is legal SVG but diffs badly against upstream fixtures. |
| `path.digits(n)` truncates coordinates | Supported where present, with the same rounding | Purely an output-size optimization; the semantics carry over unchanged. |
| `arc.centroid(d)` returns `[x, y]` | Returns two `float64`s | Go has multiple return values and no tuple type; a two-element slice would allocate for no reason. Applies wherever d3 returns a fixed-length coordinate array. |

## Colors

| Upstream | Here | Why |
| --- | --- | --- |
| `d3.color(s)` returns a `Color` object or `null`; `+color` and `color + ""` coerce | `color.Parse(s) (Color, error)`; `Color` is an interface with `RGBA()` and `String()` | Go has no coercion protocol. `String()` is the `toString()` equivalent and satisfies `fmt.Stringer`, so a color formats correctly anywhere `%v` appears. |
| Channel values live on a mutable object (`c.r = 255`) | Color values are structs, passed and returned by value | A color is a small value type. Mutating a shared color object is a class of bug that copies make impossible. `Brighter` / `Darker` return new values rather than mutating. |
| `color.rgb()` always succeeds, producing out-of-gamut channels freely | Same, with `Displayable()` and `Clamp()` to ask about and fix gamut explicitly | Matches upstream: intermediate results in Lab or HCL are legitimately out of gamut and clamping early loses information. The two methods make the decision the caller's. |
| Named CSS colors, hex, `rgb()`, `hsl()` all parse | Same | The parser follows CSS Color 3, as upstream does. CSS Color 4 function syntax is not yet accepted — see [BACKLOG.md](BACKLOG.md). |

## Interpolation

| Upstream | Here | Why |
| --- | --- | --- |
| `d3.interpolate(a, b)` sniffs the runtime types of `a` and `b` and picks an interpolator | Named constructors per type — `Number`, `Round`, `String`, `Numbers`, `Array`, `Object`, `Basis`, `Piecewise`, the color interpolators — plus `Value(a, b any) func(float64) any` as the direct port of the sniffing form | Choosing at the call site is faster, allocation-free and checked at compile time, so the named constructors are the ones to reach for. `Value` exists because d3 code being translated uses `d3.interpolate` directly and a port that omitted it would silently change behavior; it returns `any` and is the only place in the package where a type assertion is needed. |
| An interpolator is a function `t => value` | Same — a `func(float64) T` | No reason to change what already fits. |
| `interpolateString` finds embedded numbers with a regexp and interpolates them | Same behavior | Ported as-is, including the rule that non-numeric text comes from the *end* value. |
| `d3.quantize(interp, n)` | Same | Unchanged. |

## Scales

| Upstream | Here | Why |
| --- | --- | --- |
| `scaleLinear()` | `scale.NewLinear()` | Go's constructor convention. The upstream name is recoverable by reading `New` as the missing `scale` prefix. |
| A scale is a function with properties attached | A `*Continuous`, `*Band`, `*Ordinal`, … with a `Scale` method | See the getter/setter section. |
| `scale.unknown(v)` for undefined inputs | Present where upstream has it | Unchanged. |
| `scale.tickFormat(count, specifier)` returns a d3-format function | Returns a `func(float64) string` | Same contract in Go's vocabulary. |
| Ordinal scales grow their domain implicitly when handed an unknown value | Same | Faithfully reproduced, including the surprise: calling an ordinal scale can change it. This is why `Ordinal.Scale` is not safe to call concurrently. |
| `scaleTime` is local time; `scaleUtc` is UTC | `scale.NewTime()` and `scale.NewUTC()`, over `time.Time` | Go's `time.Time` carries a location, so the local/UTC distinction is about *which* intervals the ticks land on, exactly as upstream. `time.Time` is used throughout rather than a float epoch. |

## Hierarchy

| Upstream | Here | Why |
| --- | --- | --- |
| `d3.hierarchy(data, children)` walks arbitrary objects | Generic over the datum type, with a children accessor | Same reason as the shape generators: the structure is a type parameter rather than a runtime discovery. |
| `node.each`, `eachBefore`, `eachAfter` take callbacks | Same | Unchanged. Go 1.23 iterators would be an alternative surface, not a replacement. |
| `stratify()` throws on a cycle, multiple roots or a missing parent | Returns a typed error | Malformed input data is a runtime condition the caller can handle, not a programmer error. |
| Layouts mutate the nodes they are given (`node.x`, `node.y`, `node.x0`…) | Same | Faithfully reproduced. A layout is a pass over a tree that annotates it; copying the tree to avoid that would surprise anyone who knows d3. |

## Formatting and time

| Upstream | Here | Why |
| --- | --- | --- |
| `d3.format(".2f")` throws on an invalid specifier | Returns `(func(float64) string, error)`, with a `Must…` companion | Specifiers are usually literals; the `Must` form is for those, and the error form is for specifiers that came from configuration. |
| `d3.timeFormat("%Y-%m-%d")` uses strftime directives | Same directives, on `time.Time` | Go's reference-layout formatting (`2006-01-02`) is the idiomatic Go answer, but the whole point of the port is to accept a d3 specifier. Both remain available: use `time.Format` directly when the input is Go's, and this package when it is d3's. |
| `d3.timeDay.floor(date)`, `.range(a, b)`, `.every(n)` | The same interval surface over `time.Time` | Unchanged in shape. DST handling follows `time.Time`'s location rather than the JavaScript `Date`'s, which is the correct behavior and can differ from upstream at a DST boundary. |
| Locales are objects passed to `formatLocale` / `timeFormatLocale` | Built-in English defaults only, for now | The locale table is data, not algorithm, and belongs in a follow-up. Tracked in [BACKLOG.md](BACKLOG.md). |

## Random

| Upstream | Here | Why |
| --- | --- | --- |
| `d3.randomNormal(mu, sigma)` draws from `Math.random`; `.source(rng)` rebinds it | `random.Normal(mu, sigma)` draws from a package default; `random.Source(seed).Normal(mu, sigma)` binds an explicit one | Upstream's `.source()` returns a *new* generator function with the source curried in, which in Go is more naturally a method on the source. The package-level functions exist so the common case stays one call, and the default is `math/rand/v2`'s global generator — seeded unpredictably at startup, so package-level draws are **not** reproducible. Reach for `Source` when they must be. |
| `d3.randomLcg(seed)` | `random.Source(seed)` | Same LCG, same constants, so a seeded sequence matches upstream's — including d3's idiosyncratic seed handling, where a seed in `[0, 1)` is scaled by 2³² and any other seed has its absolute value taken, then truncated and reduced mod 2³² the way JavaScript's `| 0` does. |
| Any function can be passed to `.source()` | `random.SourceFunc(next func() float64)` | Same capability, named so that the contract (`next` must return values in `[0, 1)`) has somewhere to be documented. |

## DSV

| Upstream | Here | Why |
| --- | --- | --- |
| `d3.csvParse(text)` returns an array of objects carrying a `columns` property | `dsv.ParseCSV(text)` returns a `*Table` with `Columns` and `Rows` | Go has no way to hang a property off a slice. A struct says the same thing without the trick, and makes the column *order* — the thing you need to round-trip a file unchanged — a first-class field rather than an incidental one. |
| A parsed row is an object whose values are strings | `dsv.Row` is `map[string]string` | Same thing, named. |
| `d3.autoType(row)` mutates the row in place, converting to number, date, boolean or null | `dsv.AutoType(row) map[string]any` returns a new map; `AutoTypeValue(s) any` converts one field | Same rules and same order of attempts, but a Go map cannot change its value type in place, and mutating the caller's row is a surprise even where it can. |
| `d3.csvParseRows` for headerless data | `dsv.ParseCSVRows` / `ParseRows(delim, text)` | Unchanged. |
| A malformed file is parsed as best it can be | Returns an error | An unterminated quote is not a row of data. See *Empty input* above. |
| `d3.csvFormat(rows, columns)` | `dsv.FormatCSV(rows, columns)`, or `table.Format(delim)` | Unchanged; the method form exists because a `*Table` already knows its own column order. |

## Things that behave the same, and are easy to assume otherwise

- **`Ticks` is the shared foundation.** Every quantitative scale's `Nice` and
  `Ticks` route through `array.Ticks` / `TickStep` / `TickIncrement`, which is a
  literal port of d3-array's `tickSpec` — including the trick of tracking a
  negative increment to mean `1/n`, so ticks below 1 come out as exact
  divisions (`3/10 == 0.3`, not `0.1+0.1+0.1`). Do not reimplement it per
  scale.
- **JavaScript's `Math.round` rounds halves toward +∞; Go's `math.Round` rounds
  them away from zero.** The tick code uses `floor(x+0.5)` for this reason.
  Without it, negative domains drift by one tick. Anywhere else this port
  rounds, the same question has been asked.
- **Continuous scales extrapolate outside their domain** unless `SetClamp(true)`
  is set. This surprises people regularly and is upstream behavior.
- **Quantize, quantile and threshold differ in where their breakpoints come
  from**, not in what they do: quantize cuts the domain uniformly, quantile cuts
  the *data* so each bucket holds equally many observations, and threshold takes
  the cuts you supply.
- **`Pie` sorts by value by default** and returns angles, not a path. Feeding
  those angles to `Arc` is the second half of the operation, exactly as
  upstream.
- **Curves are stateful stream consumers**, ported as such — and that is exactly
  why a generator is configured with a curve *factory* rather than a curve.
  `Curve(shape.CurveMonotoneX)` takes the constructor; each `Generate` builds a
  fresh curve. The consequence is the one you want: a configured generator is a
  pure function from data to a string and is safe to call from any goroutine.
- **A configured `shape` generator is concurrency-safe; a configured `scale` is
  not.** `Ordinal.Scale` can extend its own domain when handed an unknown value
  (upstream behavior, faithfully reproduced), and every `Set…` mutates in place.
  Finish configuring before publishing the pointer, and give each goroutine a
  `Copy()` if it needs to reconfigure.
