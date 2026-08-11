# force — Go port of d3-force — a velocity Verlet particle simulation for laying out graphs

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/d3/force.svg)](https://pkg.go.dev/github.com/malcolmston/d3/force)

Package force is a Go port of d3-force — a velocity Verlet particle
simulation for laying out graphs and packing points, built on the sibling
d3/quadtree package for its Barnes–Hut approximation and on d3/random for
reproducibility.

A simulation is a set of `Node` values carrying a position and a velocity, and
a set of `Force` values that adjust those velocities each step. Nothing here
draws anything: the output is the X and Y written onto the nodes.

## Install

`d3` lives inside the [aggregator repo](../..) as a plain
directory rather than its own GitHub repository, so it is **not fetchable with
`go get`** yet. Use it through the committed `go.work`, or add a `replace`:

```
require github.com/malcolmston/d3 v0.0.0
replace github.com/malcolmston/d3 => ../path/to/go/d3
```

```go
import "github.com/malcolmston/d3/force"
```

Local version: `0.1.0` (see [`VERSION`](../VERSION)).

## Exported surface

### Types

| Type | What it is |
| --- | --- |
| `Center` | Center keeps the layout's centre of mass at a point. |
| `Collide` | Collide treats each node as a circle and pushes overlapping circles apart. |
| `Force` | Force is something that adjusts node velocities once per tick. |
| `Link` | Link is one edge of the graph: two nodes that a `LinkForce` pulls toward a target distance. |
| `LinkForce` | LinkForce pulls linked nodes toward a fixed distance from each other, like a spring on every edge. |
| `ManyBody` | ManyBody is the n-body force: every node attracts or repels every other. |
| `Node` | Node is one particle: the caller's datum plus the state the simulation reads and writes. |
| `Radial` | Radial pulls every node toward a circle of a given radius around a centre. |
| `Simulation` | Simulation is a set of nodes and the forces acting on them. |
| `XForce` | XForce pulls every node toward a target x coordinate, leaving y alone. |
| `YForce` | YForce pulls every node toward a target y coordinate, leaving x alone. |

<details>
<summary><code>Center</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewCenter[T any](x, y float64) *Center[T]` | NewCenter returns a centring force targeting (x, y) with strength 1 — full correction every tick. |
| `func (f *Center[T]) Apply(float64)` | Apply implements `Force`. |
| `func (f *Center[T]) Initialize(nodes []*Node[T], _ *random.Rand)` | Initialize implements `Force`. |
| `func (f *Center[T]) SetStrength(s float64) *Center[T]` | SetStrength sets the fraction of the centring error corrected each tick and returns the receiver. |
| `func (f *Center[T]) SetX(x float64) *Center[T]` | SetX sets the target x and returns the receiver. |
| `func (f *Center[T]) SetY(y float64) *Center[T]` | SetY sets the target y and returns the receiver. |

</details>

<details>
<summary><code>Collide</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewCollide[T any]() *Collide[T]` | NewCollide returns a collision force with radius 1, strength 0.7 and one iteration. |
| `func (f *Collide[T]) Apply(float64)` | Apply implements `Force`. |
| `func (f *Collide[T]) Initialize(nodes []*Node[T], rnd *random.Rand)` | Initialize implements `Force`. |
| `func (f *Collide[T]) SetIterations(n int) *Collide[T]` | SetIterations sets how many resolution passes run per tick and returns the receiver. |
| `func (f *Collide[T]) SetRadius(r float64) *Collide[T]` | SetRadius sets a constant collision radius and returns the receiver. |
| `func (f *Collide[T]) SetRadiusFunc(fn func(*Node[T]) float64) *Collide[T]` | SetRadiusFunc sets a per-node collision radius and returns the receiver. |
| `func (f *Collide[T]) SetStrength(s float64) *Collide[T]` | SetStrength sets how much of each overlap is corrected per pass and returns the receiver. |

</details>

<details>
<summary><code>LinkForce</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewLink[T any](links []*Link[T]) *LinkForce[T]` | NewLink returns a link force over the given edges, with distance 30, one iteration and the degree-based default strength described on `LinkForce`. |
| `func (f *LinkForce[T]) Apply(alpha float64)` | Apply implements `Force`. |
| `func (f *LinkForce[T]) Initialize(nodes []*Node[T], rnd *random.Rand)` | Initialize implements `Force`. |
| `func (f *LinkForce[T]) Links() []*Link[T]` | Links returns the edges this force acts on. |
| `func (f *LinkForce[T]) SetDistance(d float64) *LinkForce[T]` | SetDistance sets a constant target length for every edge and returns the receiver. |
| `func (f *LinkForce[T]) SetDistanceFunc(fn func(*Link[T]) float64) *LinkForce[T]` | SetDistanceFunc sets a per-edge target length and returns the receiver. |
| `func (f *LinkForce[T]) SetIterations(n int) *LinkForce[T]` | SetIterations sets how many relaxation passes run per tick and returns the receiver. |
| `func (f *LinkForce[T]) SetLinks(links []*Link[T]) *LinkForce[T]` | SetLinks replaces the edges and returns the receiver, recomputing the degree counts and the per-link constants. |
| `func (f *LinkForce[T]) SetStrength(s float64) *LinkForce[T]` | SetStrength sets a constant spring stiffness for every edge and returns the receiver, replacing the degree-based default. |
| `func (f *LinkForce[T]) SetStrengthFunc(fn func(*Link[T]) float64) *LinkForce[T]` | SetStrengthFunc sets a per-edge stiffness and returns the receiver, replacing the degree-based default. |

</details>

<details>
<summary><code>ManyBody</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewManyBody[T any]() *ManyBody[T]` | NewManyBody returns an n-body force with d3's defaults: strength −30, theta 0.9, minimum distance 1 and no maximum. |
| `func (f *ManyBody[T]) Apply(alpha float64)` | Apply implements `Force`. |
| `func (f *ManyBody[T]) Initialize(nodes []*Node[T], rnd *random.Rand)` | Initialize implements `Force`. |
| `func (f *ManyBody[T]) SetDistanceMax(d float64) *ManyBody[T]` | SetDistanceMax sets the maximum interaction distance and returns the receiver. |
| `func (f *ManyBody[T]) SetDistanceMin(d float64) *ManyBody[T]` | SetDistanceMin sets the minimum interaction distance and returns the receiver. |
| `func (f *ManyBody[T]) SetStrength(v float64) *ManyBody[T]` | SetStrength sets a constant charge for every node and returns the receiver. |
| `func (f *ManyBody[T]) SetStrengthFunc(fn func(*Node[T]) float64) *ManyBody[T]` | SetStrengthFunc sets a per-node charge accessor and returns the receiver. |
| `func (f *ManyBody[T]) SetTheta(theta float64) *ManyBody[T]` | SetTheta sets the Barnes–Hut accuracy parameter and returns the receiver. |

</details>

<details>
<summary><code>Node</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewNode[T any](d T) *Node[T]` | NewNode returns a node holding d, marked for phyllotaxis placement: its position is NaN until the simulation assigns one. |
| `func (n *Node[T]) Fix(x, y float64) *Node[T]` | Fix pins the node at (x, y). |
| `func (n *Node[T]) FixX(x float64) *Node[T]` | FixX pins only the node's x coordinate, leaving y free. |
| `func (n *Node[T]) FixY(y float64) *Node[T]` | FixY pins only the node's y coordinate, leaving x free. |
| `func (n *Node[T]) Unfix() *Node[T]` | Unfix releases both pins. |

</details>

<details>
<summary><code>Radial</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewRadial[T any](radius, x, y float64) *Radial[T]` | NewRadial returns a force pulling every node toward the circle of the given radius centred at (x, y), with strength 0.1. |
| `func (f *Radial[T]) Apply(alpha float64)` | Apply implements `Force`. |
| `func (f *Radial[T]) Initialize(nodes []*Node[T], _ *random.Rand)` | Initialize implements `Force`. |
| `func (f *Radial[T]) SetRadius(r float64) *Radial[T]` | SetRadius sets a constant target radius and returns the receiver. |
| `func (f *Radial[T]) SetRadiusFunc(fn func(*Node[T]) float64) *Radial[T]` | SetRadiusFunc sets a per-node target radius and returns the receiver, evaluated once per node at initialization. |
| `func (f *Radial[T]) SetStrength(s float64) *Radial[T]` | SetStrength sets a constant pull and returns the receiver. |
| `func (f *Radial[T]) SetStrengthFunc(fn func(*Node[T]) float64) *Radial[T]` | SetStrengthFunc sets a per-node pull and returns the receiver. |
| `func (f *Radial[T]) SetX(x float64) *Radial[T]` | SetX sets the centre's x and returns the receiver. |
| `func (f *Radial[T]) SetY(y float64) *Radial[T]` | SetY sets the centre's y and returns the receiver. |

</details>

<details>
<summary><code>Simulation</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func New[T any](data []T) *Simulation[T]` | New returns a simulation over one node per datum, each placed on the phyllotaxis spiral. |
| `func NewWithNodes[T any](nodes []*Node[T]) *Simulation[T]` | NewWithNodes returns a simulation over nodes the caller built, which is how to start from known positions — reusing the result of an earlier run,… |
| `func (s *Simulation[T]) Alpha() float64` | Alpha returns the current alpha: the simulation's temperature, which scales every force and decays toward the alpha target each tick. |
| `func (s *Simulation[T]) AlphaDecay() float64` | AlphaDecay returns the per-tick decay rate: alpha moves this fraction of the way from its current value to the alpha target each tick. |
| `func (s *Simulation[T]) AlphaMin() float64` | AlphaMin returns the alpha below which the simulation is considered converged and `Simulation.Tick` becomes a no-op. |
| `func (s *Simulation[T]) AlphaTarget() float64` | AlphaTarget returns the value alpha decays toward, normally zero. |
| `func (s *Simulation[T]) Converged() bool` | Converged reports whether alpha has fallen below alphaMin, which is when `Simulation.Tick` stops doing anything. |
| `func (s *Simulation[T]) Find(x, y float64) (*Node[T], bool)` | Find returns the node closest to (x, y), and whether there was one. |
| `func (s *Simulation[T]) FindWithin(x, y, radius float64) (*Node[T], bool)` | FindWithin returns the node closest to (x, y) within radius, and false if there is none that close. |
| `func (s *Simulation[T]) Force(name string) Force[T]` | Force returns the force registered under name, or nil. |
| `func (s *Simulation[T]) Nodes() []*Node[T]` | Nodes returns the simulation's nodes — the same slice it is working on, not a copy, because the nodes are the output and copying the slice would… |
| `func (s *Simulation[T]) Random() *random.Rand` | Random returns the simulation's random source. |
| `func (s *Simulation[T]) RemoveForce(name string) *Simulation[T]` | RemoveForce unregisters a force and returns the receiver. |
| `func (s *Simulation[T]) Restart() *Simulation[T]` | Restart reheats the simulation and returns the receiver: alpha goes back to 1 and a previous `Simulation.Stop` is cleared. |
| `func (s *Simulation[T]) RunUntilStable() int` | RunUntilStable ticks until the simulation converges and returns how many ticks that took. |
| `func (s *Simulation[T]) SetAlpha(a float64) *Simulation[T]` | SetAlpha sets alpha and returns the receiver. |
| `func (s *Simulation[T]) SetAlphaDecay(d float64) *Simulation[T]` | SetAlphaDecay sets the decay rate and returns the receiver. |
| `func (s *Simulation[T]) SetAlphaMin(m float64) *Simulation[T]` | SetAlphaMin sets the convergence threshold and returns the receiver. |
| `func (s *Simulation[T]) SetAlphaTarget(t float64) *Simulation[T]` | SetAlphaTarget sets the value alpha decays toward and returns the receiver. |
| `func (s *Simulation[T]) SetForce(name string, f Force[T]) *Simulation[T]` | SetForce registers a force under a name and returns the receiver. |
| `func (s *Simulation[T]) SetNodes(nodes []*Node[T]) *Simulation[T]` | SetNodes replaces the node set and returns the receiver. |
| `func (s *Simulation[T]) SetRandom(r *random.Rand) *Simulation[T]` | SetRandom replaces the random source and returns the receiver, re-initializing every force so they pick it up. |
| `func (s *Simulation[T]) SetVelocityDecay(d float64) *Simulation[T]` | SetVelocityDecay sets the friction and returns the receiver. |
| `func (s *Simulation[T]) Stop() *Simulation[T]` | Stop halts the simulation and returns the receiver: `Simulation.Tick` becomes a no-op until `Simulation.Restart`. |
| `func (s *Simulation[T]) Tick() *Simulation[T]` | Tick advances the simulation one step and returns the receiver. |
| `func (s *Simulation[T]) TickN(n int) int` | TickN advances the simulation n steps, stopping early if it converges, and returns the number of steps actually taken. |
| `func (s *Simulation[T]) VelocityDecay() float64` | VelocityDecay returns the per-tick velocity damping, the simulation's friction. |

</details>

<details>
<summary><code>XForce</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewX[T any](x float64) *XForce[T]` | NewX returns a force pulling every node toward the given x, with strength 0.1. |
| `func (f *XForce[T]) Apply(alpha float64)` | Apply implements `Force`. |
| `func (f *XForce[T]) Initialize(nodes []*Node[T], _ *random.Rand)` | Initialize implements `Force`. |
| `func (f *XForce[T]) SetStrength(s float64) *XForce[T]` | SetStrength sets a constant pull and returns the receiver. |
| `func (f *XForce[T]) SetStrengthFunc(fn func(*Node[T]) float64) *XForce[T]` | SetStrengthFunc sets a per-node pull and returns the receiver, evaluated once per node at initialization. |
| `func (f *XForce[T]) SetX(x float64) *XForce[T]` | SetX sets a constant target and returns the receiver. |
| `func (f *XForce[T]) SetXFunc(fn func(*Node[T]) float64) *XForce[T]` | SetXFunc sets a per-node target and returns the receiver, evaluated once per node at initialization. |

</details>

<details>
<summary><code>YForce</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewY[T any](y float64) *YForce[T]` | NewY returns a force pulling every node toward the given y, with strength 0.1. |
| `func (f *YForce[T]) Apply(alpha float64)` | Apply implements `Force`. |
| `func (f *YForce[T]) Initialize(nodes []*Node[T], _ *random.Rand)` | Initialize implements `Force`. |
| `func (f *YForce[T]) SetStrength(s float64) *YForce[T]` | SetStrength sets a constant pull and returns the receiver. |
| `func (f *YForce[T]) SetStrengthFunc(fn func(*Node[T]) float64) *YForce[T]` | SetStrengthFunc sets a per-node pull and returns the receiver. |
| `func (f *YForce[T]) SetY(y float64) *YForce[T]` | SetY sets a constant target and returns the receiver. |
| `func (f *YForce[T]) SetYFunc(fn func(*Node[T]) float64) *YForce[T]` | SetYFunc sets a per-node target and returns the receiver, evaluated once per node at initialization. |

</details>

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/d3/force).

## Deviations from upstream

Deliberate differences for this package, where any exist, are recorded in the
module-wide [`API-DEVIATIONS.md`](../API-DEVIATIONS.md).

## License

MIT, as part of [`github.com/malcolmston/d3`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
