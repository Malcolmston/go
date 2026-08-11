# router — Go port of react-router v7's declarative routing, built on github.com/malcolmston/react

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/react/router.svg)](https://pkg.go.dev/github.com/malcolmston/react/router)

Package router is a Go port of react-router v7's declarative routing, built on
`github.com/malcolmston/react`.

A route table is data, a URL is matched against it by specificity, and the
winning chain of routes renders as nested components:

```go
var routes = []router.Route{{
    Path:      "/",
    Component: Layout,
    Children: []router.Route{
        {Index: true, Component: Home},
        {Path: "users", Component: UserList, Children: []router.Route{
            {Path: ":id", Component: UserDetail},
        }},
        {Path: "*", Component: NotFound},
    },
}}
```

## Install

`react` lives inside the [aggregator repo](../..) as a plain
directory rather than its own GitHub repository, so it is **not fetchable with
`go get`** yet. Use it through the committed `go.work`, or add a `replace`:

```
require github.com/malcolmston/react v0.0.0
replace github.com/malcolmston/react => ../path/to/go/react
```

```go
import "github.com/malcolmston/react/router"
```

Local version: `0.1.0` (see [`VERSION`](../VERSION)).

## Exported surface

### Functions

| Function | What it does |
| --- | --- |
| `func Link(to string, props react.Props, children ...react.Node) *react.Element` | Link renders an anchor to a route — upstream's `<Link to=…>`. |
| `func MatchPath(pattern, path string) (map[string]string, bool)` | MatchPath matches a single pattern against a path and returns the captured params. |
| `func NavLink(to string, props react.Props, children ...react.Node) *react.Element` | NavLink renders a `Link` that knows whether it points at the current page — upstream's `<NavLink>`, and the element a navigation bar is built from. |
| `func Navigate(to string) *react.Element` | Navigate records a navigation intent when it renders — the declarative form of `UseNavigate`, and the port of upstream's `<Navigate to=…>`. |
| `func NavigateReplace(to string) *react.Element` | NavigateReplace is `Navigate` with the replace flag set, matching upstream's `<Navigate to=… replace />`. |
| `func Outlet() *react.Element` | Outlet marks the place where a layout route renders its matched child — the port of upstream's `<Outlet/>`, and the mechanism that makes nesting… |
| `func PatternScore(pattern string, index bool) int` | PatternScore returns the specificity score of a fully joined route pattern, the number `RankedPatterns` sorts on. |
| `func RankedPatterns(routes []Route) []string` | RankedPatterns returns the joined route patterns of a table in the order the matcher tries them, best-first. |
| `func Router(location Location, routes []Route, children ...react.Node) *react.Element` | Router establishes the routing context for a subtree: the location being rendered, the route table, and a fresh `Navigation` recorder reachable with… |
| `func RouterWith(nav *Navigation, location Location, routes []Route, children ...react.Node) *react.Element` | RouterWith is `Router` with a caller-supplied recorder, for the case where the `Navigation` must be reachable without holding on to the element — a… |
| `func Routes(routes []Route) *react.Element` | Routes matches the ambient location against a route table and renders the winning chain. |
| `func UseMatch(pattern string) (map[string]string, bool)` | UseMatch matches a pattern against the current location and reports the captured params — upstream's `useMatch`. |
| `func UseOutlet() react.Node` | UseOutlet returns the node `Outlet` would render at this position, or nil when the calling route has no matched child — upstream's `useOutlet`. |
| `func UseParams() map[string]string` | UseParams returns the dynamic segments captured by the route chain above and including the calling component — upstream's `useParams`. |
| `func UseSearchParams() url.Values` | UseSearchParams returns the parsed query string of the current location. |

### Types

| Type | What it is |
| --- | --- |
| `Location` | Location is the address a tree is rendered for — the port's stand-in for react-router's `Location`, which is itself a subset of the DOM's… |
| `Match` | Match is one route in the chain that matched a URL, from the outermost layout down to the leaf. |
| `NavigateFunc` | NavigateFunc is what `UseNavigate` returns: callable with a target for the ordinary push, with a `NavigateFunc.Replace` method for upstream's… |
| `Navigation` | Navigation is where a navigation intent is recorded. |
| `Route` | Route is one node of a route table — the Go form of react-router's route objects (`createBrowserRouter([{path, element, children}])`) rather than… |

<details>
<summary><code>Location</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func LocationFromURL(u *url.URL) Location` | LocationFromURL builds a `Location` from a parsed URL, taking the path in its encoded form so that a %2F in a path segment is still distinguishable… |
| `func ParseLocation(raw string) Location` | ParseLocation splits a raw URL reference — "/users/7?tab=posts#top" — into a `Location`. |
| `func ResolvePath(to string, fromPathname string) Location` | ResolvePath resolves a link target against the path it was written in, reproducing react-router's `resolvePath`. |
| `func UseLocation() Location` | UseLocation returns the location the tree is being rendered for — upstream's `useLocation`, and the hook every other one in this file is a… |
| `func UseResolvedPath(to string) Location` | UseResolvedPath resolves a link target against the route the calling component belongs to, returning the location it points at — upstream's… |
| `func (l Location) Query() url.Values` | Query parses `Location.Search` into the standard library's query type. |
| `func (l Location) String() string` | String rebuilds the URL reference the Location was parsed from. |

</details>

<details>
<summary><code>Match</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func MatchRoutes(routes []Route, path string) []Match` | MatchRoutes matches a URL path against a route table and returns the winning chain of matches, outermost first, or nil when nothing matched. |
| `func UseMatches() []Match` | UseMatches returns the whole chain of matches being rendered, outermost first, or nil outside a matched route. |

</details>

<details>
<summary><code>NavigateFunc</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func UseNavigate() NavigateFunc` | UseNavigate returns a function that records an intent to navigate. |
| `func (n NavigateFunc) Replace(to string)` | Replace records a navigation that should not leave the current URL behind it. |

</details>

<details>
<summary><code>Navigation</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NavigationOf(el *react.Element) *Navigation` | NavigationOf returns the `Navigation` recorder attached to an element built by `Router` or `RouterWith`, or nil for any other element. |
| `func NewNavigation() *Navigation` | NewNavigation returns an empty recorder. |
| `func (n *Navigation) IsReplace() bool` | IsReplace reports whether the recorded intent asked to replace the current entry rather than push a new one. |
| `func (n *Navigation) Pending() (to string, ok bool)` | Pending returns the recorded target and whether anything was recorded at all. |
| `func (n *Navigation) Push(to string)` | Push records an intent to navigate to a new URL, the equivalent of `navigate(to)`. |
| `func (n *Navigation) Replace(to string)` | Replace records an intent to navigate without leaving the current URL in the history — `navigate(to, {replace: true})`. |
| `func (n *Navigation) Reset()` | Reset clears the recorder so the same `Navigation` can be reused for another render. |

</details>

### Constants

`ActiveClassProp`, `EndProp`

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/react/router).

## Deviations from upstream

Deliberate differences for this package, where any exist, are recorded in the
module-wide [`API-DEVIATIONS.md`](../API-DEVIATIONS.md).

## License

MIT, as part of [`github.com/malcolmston/react`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
