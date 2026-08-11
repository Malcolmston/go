// Package scale is a Go port of d3-scale — the mappings that turn abstract
// data into visual variables. A scale is a function from a *domain* (the units
// your data is in: dollars, seconds, categories) to a *range* (the units your
// display is in: pixels, colors, radii). Almost every chart is, at bottom, two
// or three scales plus some drawing:
//
//	x := scale.NewTime().SetDomain(t0, t1).SetRange(0, 640).Nice(10)
//	y := scale.NewLinear().SetDomain(0, maxValue).SetRange(480, 0).Nice(10)
//	for _, d := range data {
//		plot(x.Scale(d.When), y.Scale(d.Value))
//	}
//
// This package is the computational half of d3: it has no DOM, emits no SVG and
// imports nothing outside the standard library except its sibling packages
// [github.com/malcolmston/d3/array] (tick generation),
// [github.com/malcolmston/d3/color] and
// [github.com/malcolmston/d3/interpolate]. Pair it with a renderer such as
// github.com/malcolmston/react to actually draw something.
//
// # Getter/setter convention
//
// This is the one place where the Go API deliberately diverges from d3, so it
// is worth stating once and clearly. In JavaScript, d3 overloads a single
// method as both accessor and mutator: `scale.domain()` reads, and
// `scale.domain([0, 100])` writes and returns the scale for chaining. Go has no
// argument-count overloading and no optional arguments, so every d3 dual-role
// method is split in two:
//
//   - The bare noun is the GETTER and takes no arguments: [Continuous.Domain],
//     [Continuous.Range], [Continuous.Clamp], [Band.Padding], [Band.Bandwidth].
//     Getters that return a slice always return a defensive copy, so callers
//     cannot mutate a scale's internals by accident.
//   - `SetX` is the SETTER and returns the receiver, so chaining works exactly
//     as it does in d3: [Continuous.SetDomain], [Continuous.SetRange],
//     [Continuous.SetClamp], [Band.SetPadding].
//   - Methods that are already verbs in d3 and are not accessors keep their
//     names and their chaining behavior: [Continuous.Nice], [Continuous.Copy],
//     [Continuous.Ticks], [Continuous.TickFormat], [Continuous.Invert].
//   - d3's `rangeRound(r)` is [Continuous.SetRangeRound]: it sets the range and
//     switches the interpolator to a rounding one, which is what you want when
//     the output is device pixels.
//   - The callable scale itself — `scale(x)` in JavaScript — is the `Scale`
//     method: [Continuous.Scale], [Band.Scale], [Ordinal.Scale].
//
// Setters mutate the receiver in place and return it. They do NOT return a new
// scale. That means a scale value is cheap to configure but is not safe for
// concurrent mutation; share a [Continuous.Copy] across goroutines instead, or
// finish configuring before publishing the pointer. This is the opposite of
// this repo's chalk package (whose Style is immutable), and the difference is
// intentional: chalk values are combined ad hoc in expressions, whereas a scale
// is built once and then called thousands of times, so d3's mutate-and-chain
// shape is both more faithful and cheaper.
//
// # The families of scales
//
// Continuous scales ([NewLinear], [NewPow], [NewSqrt], [NewLog], [NewSymlog],
// [NewIdentity], [NewRadial], [NewTime], [NewUTC]) map a continuous, quantitative
// domain to a continuous range by *normalizing* the input to [0, 1] within the
// domain and then *interpolating* the range at that position. They are
// invertible ([Continuous.Invert]) whenever the range is numeric, and they
// extrapolate outside the domain unless [Continuous.SetClamp] is set — a point
// that surprises people, and one this package tests explicitly.
//
// Sequential ([NewSequential] and friends) and diverging ([NewDiverging])
// scales are continuous scales whose range is fixed to an interpolator rather
// than to a pair of endpoints; they exist mainly for color ramps, where the
// range is a curve through a color space rather than two stops.
//
// Quantize, quantile and threshold ([NewQuantize], [NewQuantile],
// [NewThreshold]) all map a continuous domain to a *discrete* range, and are
// routinely confused with each other. The distinction is where the breakpoints
// come from: quantize cuts the domain into uniform slices, quantile cuts the
// *data* so each bucket holds the same number of observations, and threshold
// takes the breakpoints you hand it. See the package tests, which compare all
// three on the same input.
//
// Ordinal scales ([NewOrdinal]) map a discrete domain to a discrete range, and
// [NewBand]/[NewPoint] specialize that for positional encoding: a band scale
// gives every category a slice of the range with configurable padding
// ([Band.Bandwidth], [Band.Step]) and is what every bar chart is built on.
//
// # Fidelity notes
//
// This package deliberately owns none of the shared numeric machinery. Tick
// generation delegates to array.Ticks, array.TickIncrement and array.TickStep,
// and [Continuous.Nice] is defined in terms of array.TickIncrement, so that the
// nicened endpoints are guaranteed to be ticks. Number formatting goes through
// github.com/malcolmston/d3/format, so [Continuous.TickFormatSpecifier] accepts
// the same specifier strings as d3 ("$,.2f", ".1%", "s"). Calendar arithmetic —
// the interval ladder, boundary snapping and the strftime-style label formats —
// comes from github.com/malcolmston/d3/timefmt, so [Time] is a thin adapter
// rather than a second implementation of d3-time.
//
// The one visible gap against upstream is d3's `scheme*`/`interpolate*` color
// ramps: those live in d3-scale-chromatic, not d3-scale, and are not part of
// this package. Supply your own interpolator to [NewSequential] or
// [NewDiverging].
package scale
