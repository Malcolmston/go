# chord — Go port of d3-chord: the layout that turns a square matrix of flows into the angles

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/d3/chord.svg)](https://pkg.go.dev/github.com/malcolmston/d3/chord)

Package chord is a Go port of d3-chord: the layout that turns a square matrix
of flows into the angles of a chord diagram, and the ribbon generator that
draws one flow as an SVG path.

A chord diagram shows a square matrix — trade between countries, transitions
between states, migration between regions — as a circle. Each row/column
index becomes a *group*, an arc around the circumference whose angular size is
the group's total. Each matrix cell becomes a *chord*, a ribbon joining the
two groups it relates, whose width at each end is that cell's value.

## Install

`d3` lives inside the [aggregator repo](../..) as a plain
directory rather than its own GitHub repository, so it is **not fetchable with
`go get`** yet. Use it through the committed `go.work`, or add a `replace`:

```
require github.com/malcolmston/d3 v0.0.0
replace github.com/malcolmston/d3 => ../path/to/go/d3
```

```go
import "github.com/malcolmston/d3/chord"
```

Local version: `0.1.0` (see [`VERSION`](../VERSION)).

## Exported surface

### Types

| Type | What it is |
| --- | --- |
| `ChordAccessor` | ChordAccessor computes one property of a whole chord. |
| `ChordDatum` | ChordDatum is one ribbon: the two ends it joins. |
| `Chords` | Chords is what a layout produces. |
| `Group` | Group is one index's arc around the circumference. |
| `Layout` | Layout computes the angles of a chord diagram from a square matrix. |
| `Ribbon` | Ribbon generates the SVG path for one chord: the shape that leaves one group's arc, narrows through the centre of the circle and widens again onto… |
| `Subgroup` | Subgroup is one end of a chord: the slice of a group's arc that this particular flow occupies. |
| `SubgroupAccessor` | SubgroupAccessor computes one property of a chord end. |

<details>
<summary><code>Layout</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func New() *Layout` | New returns an undirected chord layout: no padding, and groups, subgroups and chords all left in matrix order. |
| `func NewDirected() *Layout` | NewDirected returns a directed chord layout, in which each cell is its own flow and a group's arc spans both its row and its column. |
| `func NewTranspose() *Layout` | NewTranspose returns an undirected chord layout over the transpose of the matrix — the same diagram with rows and columns exchanged. |
| `func (l *Layout) Generate(matrix [][]float64) Chords` | Generate computes the layout. |
| `func (l *Layout) PadAngle(v float64) *Layout` | PadAngle sets the angular gap between adjacent groups, in radians. |
| `func (l *Layout) SortChords(f func(a, b float64) int) *Layout` | SortChords orders the returned chords by comparing the sum of each chord's two end values. |
| `func (l *Layout) SortGroups(f func(a, b float64) int) *Layout` | SortGroups orders the groups around the circle by comparing their totals. |
| `func (l *Layout) SortSubgroups(f func(a, b float64) int) *Layout` | SortSubgroups orders the chord ends within each group by comparing their values. |

</details>

<details>
<summary><code>Ribbon</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewRibbon() *Ribbon` | NewRibbon returns a plain ribbon: both ends are arcs, and the two arcs are joined by curves through the centre. |
| `func NewRibbonArrow() *Ribbon` | NewRibbonArrow returns a ribbon whose target end is a triangular arrowhead rather than an arc, with a head radius of 10. |
| `func (r *Ribbon) Digits(n int) *Ribbon` | Digits rounds every emitted coordinate to n decimal places, the equivalent of d3.path(digits). |
| `func (r *Ribbon) EndAngle(v float64) *Ribbon` | EndAngle sets a constant end angle in radians for both ends. |
| `func (r *Ribbon) EndAngleFunc(f SubgroupAccessor) *Ribbon` | EndAngleFunc sets a per-end end angle. |
| `func (r *Ribbon) Generate(d ChordDatum) string` | Generate returns the SVG path data for one chord, or "" if nothing was drawn. |
| `func (r *Ribbon) HeadRadius(v float64) *Ribbon` | HeadRadius sets a constant arrowhead depth: the distance by which the two corners of the target end are pulled in from the radius, leaving the point… |
| `func (r *Ribbon) HeadRadiusFunc(f ChordAccessor) *Ribbon` | HeadRadiusFunc sets a per-chord arrowhead depth. |
| `func (r *Ribbon) PadAngle(v float64) *Ribbon` | PadAngle sets a constant angular gap, in radians, taken out of *each* end of the ribbon so that adjacent ribbons landing on the same group arc are… |
| `func (r *Ribbon) PadAngleFunc(f ChordAccessor) *Ribbon` | PadAngleFunc sets a per-chord pad angle. |
| `func (r *Ribbon) Radius(v float64) *Ribbon` | Radius sets a constant radius for both ends — the usual case, and normally the inner radius of the group arcs so the ribbons meet them flush. |
| `func (r *Ribbon) RadiusFunc(f SubgroupAccessor) *Ribbon` | RadiusFunc sets a per-end radius for both ends. |
| `func (r *Ribbon) Source(f func(d ChordDatum) Subgroup) *Ribbon` | Source sets the accessor for the chord's source end. |
| `func (r *Ribbon) SourceRadius(v float64) *Ribbon` | SourceRadius sets a constant radius for the source end only. |
| `func (r *Ribbon) SourceRadiusFunc(f SubgroupAccessor) *Ribbon` | SourceRadiusFunc sets a per-end radius for the source end only. |
| `func (r *Ribbon) StartAngle(v float64) *Ribbon` | StartAngle sets a constant start angle in radians for both ends. |
| `func (r *Ribbon) StartAngleFunc(f SubgroupAccessor) *Ribbon` | StartAngleFunc sets a per-end start angle. |
| `func (r *Ribbon) Target(f func(d ChordDatum) Subgroup) *Ribbon` | Target sets the accessor for the chord's target end. |
| `func (r *Ribbon) TargetRadius(v float64) *Ribbon` | TargetRadius sets a constant radius for the target end only. |
| `func (r *Ribbon) TargetRadiusFunc(f SubgroupAccessor) *Ribbon` | TargetRadiusFunc sets a per-end radius for the target end only. |

</details>

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/d3/chord).

## Deviations from upstream

Deliberate differences for this package, where any exist, are recorded in the
module-wide [`API-DEVIATIONS.md`](../API-DEVIATIONS.md).

## License

MIT, as part of [`github.com/malcolmston/d3`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
