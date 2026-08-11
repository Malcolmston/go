// Package router is a Go port of react-router v7's declarative routing, built
// on [github.com/malcolmston/react].
//
// A route table is data, a URL is matched against it by specificity, and the
// winning chain of routes renders as nested components:
//
//	var routes = []router.Route{{
//	    Path:      "/",
//	    Component: Layout,
//	    Children: []router.Route{
//	        {Index: true, Component: Home},
//	        {Path: "users", Component: UserList, Children: []router.Route{
//	            {Path: ":id", Component: UserDetail},
//	        }},
//	        {Path: "*", Component: NotFound},
//	    },
//	}}
//
//	func handler(w http.ResponseWriter, r *http.Request) {
//	    page := router.Router(router.ParseLocation(r.URL.RequestURI()), routes)
//	    if to, ok := router.NavigationOf(page).Pending(); ok {
//	        http.Redirect(w, r, to, http.StatusFound)
//	        return
//	    }
//	    _ = react.RenderToWriter(w, page)
//	}
//
// Layout renders whatever [Outlet] it contains; UserList's own [Outlet] is
// where UserDetail lands. Inside any of them, [UseParams], [UseLocation] and
// friends read the ambient match.
//
// # There is no browser
//
// This is the deviation that shapes everything else, and it is worth being
// blunt about. The react module has no DOM, no event system and no History API,
// so a `<Link>` cannot intercept a click and a `navigate()` cannot push a
// history entry. Nothing in this package pretends otherwise:
//
//   - [Link] and [NavLink] render a real <a href>, which is genuinely useful:
//     server-rendered navigation is what an anchor is for, and NavLink's active
//     class is computed from the location the tree was rendered for.
//   - [UseNavigate] records an *intent* on the [Navigation] value attached to
//     the [Router] element. After rendering, the caller reads it with
//     [NavigationOf] and acts — typically an HTTP redirect. It is not a silent
//     no-op, and it is not a lie about client-side routing; it is the server's
//     half of the same idea. [Navigate] is its declarative form.
//   - `BrowserRouter`, `HashRouter`, history objects, blockers, scroll
//     restoration, data loaders, actions and fetchers are absent, because every
//     one of them is defined in terms of a running browser session. See the
//     package's list of omissions in the [Router] doc comment.
//
// # Relationship to the react module's constraints
//
// Routing state travels through two contexts, both created once at package
// level, because a context created inside a component body allocates a fresh
// identity every render and orphans its readers. Everything a route renders is
// an ordinary component, so hook rules apply unchanged: call the hooks in this
// package unconditionally, from a component body, beneath a [Router].
package router

import (
	"strings"
	"sync"

	"github.com/malcolmston/react"
)

// routerValue is everything a subtree needs to know about routing that does not
// vary with position in the tree: where we are, what the route table is, and
// where to record a navigation intent.
//
// It travels by pointer so that a Router's [Navigation] is shared rather than
// copied — a copy would collect intents nobody could read.
type routerValue struct {
	location Location
	routes   []Route
	nav      *Navigation
}

// matchValue is the part of routing that *does* vary with position: the chain
// of matches being rendered and how deep into it this subtree is. [Outlet]
// renders depth+1; [UseParams] reads depth.
//
// It is a value rather than a pointer because each level publishes its own,
// and nesting is exactly what makes the depth correct.
type matchValue struct {
	matches []Match
	depth   int
}

// The two contexts, created once at package scope. A nil routerValue is the
// "no Router above you" sentinel every hook checks for; the zero matchValue is
// the legitimate "inside a Router but not inside a matched route" state, which
// is why the two are separate contexts rather than one.
var (
	routerCtx = react.CreateContext[*routerValue](nil)
	matchCtx  = react.CreateContext(matchValue{})
)

// Reserved props keys on the element [Router] builds. They are namespaced so
// they cannot collide with anything a caller passes, and unexported because
// [NavigationOf] is the supported way to reach the one that matters.
const (
	routerValueKey = "router:value"
	routesKey      = "router:routes"
)

// Navigation is where a navigation intent is recorded. It exists because there
// is no history stack to push onto: upstream's `navigate("/login")` mutates the
// browser's session and re-renders, and neither half of that is available here.
//
// So the port splits the operation in two. During render, [UseNavigate] or
// [Navigate] records the target here. After render, the caller reads it and
// decides what it means — for an HTTP handler that is almost always a redirect:
//
//	page := router.Router(loc, routes)
//	html, err := react.RenderToString(page)
//	if to, ok := router.NavigationOf(page).Pending(); ok {
//	    http.Redirect(w, r, to, http.StatusFound)
//	    return
//	}
//
// That is a deliberate design choice over the two alternatives. A no-op setter
// would make `navigate()` compile and do nothing, which is the worst outcome —
// working code that silently fails. Re-rendering the tree at the new location
// from inside a render would be a loop with no termination argument. Recording
// an intent is honest about what a server can do and is directly useful.
//
// The last recorder wins: several components asking to navigate in one render
// is not an error, and the deepest one to run has the most context. A
// Navigation is safe for concurrent use, so a tree rendered from several
// goroutines (which the react runtime serializes anyway) cannot corrupt it.
type Navigation struct {
	mu      sync.Mutex
	to      string
	replace bool
	set     bool
}

// NewNavigation returns an empty recorder. Callers rarely need it: [Router]
// allocates one and [NavigationOf] hands it back. Build one explicitly to share
// a single recorder across several independently rendered trees, or to reuse
// one across requests after calling [Navigation.Reset].
func NewNavigation() *Navigation { return &Navigation{} }

// Push records an intent to navigate to a new URL, the equivalent of
// `navigate(to)`. Recording is idempotent in the sense that matters: re-running
// the same render records the same intent again rather than accumulating.
func (n *Navigation) Push(to string) { n.record(to, false) }

// Replace records an intent to navigate without leaving the current URL in the
// history — `navigate(to, {replace: true})`. There is no history here, so the
// distinction is carried through to the caller rather than acted on: an HTTP
// handler typically maps it to a 303 rather than a 302, or ignores it.
func (n *Navigation) Replace(to string) { n.record(to, true) }

func (n *Navigation) record(to string, replace bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.to, n.replace, n.set = to, replace, true
}

// Pending returns the recorded target and whether anything was recorded at all.
// The boolean is what a caller branches on; an empty target with ok true means
// something genuinely asked to navigate to "".
func (n *Navigation) Pending() (to string, ok bool) {
	if n == nil {
		return "", false
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.to, n.set
}

// IsReplace reports whether the recorded intent asked to replace the current
// entry rather than push a new one. It is false when nothing was recorded.
func (n *Navigation) IsReplace() bool {
	if n == nil {
		return false
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.set && n.replace
}

// Reset clears the recorder so the same [Navigation] can be reused for another
// render. A recorder that is not reset keeps reporting the previous render's
// intent, which is the right default for the render-then-read flow and the
// wrong one for a recorder shared across requests.
func (n *Navigation) Reset() {
	if n == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.to, n.replace, n.set = "", false, false
}

// Router establishes the routing context for a subtree: the location being
// rendered, the route table, and a fresh [Navigation] recorder reachable with
// [NavigationOf]. It is the equivalent of upstream's `<RouterProvider>` /
// `<MemoryRouter>`, minus the history object neither can exist without here.
//
// Children are optional, and the two forms mean different things:
//
//	router.Router(loc, routes)                    // render the matched route chain
//	router.Router(loc, routes, page(...))         // render your own tree
//
// With no children the matched chain renders directly, which is the common
// case. With children, the subtree renders as written and the matched chain
// appears wherever it puts a [Routes] element — the shape to reach for when
// page chrome that is not itself a route has to wrap the routed content.
//
// A location whose Path is empty is treated as "/", so the zero [Location]
// renders the site root rather than matching nothing.
//
// What upstream has and this does not: data routers (`loader`, `action`,
// `useLoaderData`, fetchers, `defer`/`Await`), navigation blocking, scroll
// restoration, lazy route modules, and the `BrowserRouter`/`HashRouter`
// distinction. The first group is a data-fetching framework layered on routing
// and belongs above this package; the rest require a browser session.
func Router(location Location, routes []Route, children ...react.Node) *react.Element {
	return RouterWith(NewNavigation(), location, routes, children...)
}

// RouterWith is [Router] with a caller-supplied recorder, for the case where
// the [Navigation] must be reachable without holding on to the element — a
// handler that builds its tree in one function and inspects the outcome in
// another, or a test that wants to assert on navigation without threading the
// element through.
func RouterWith(nav *Navigation, location Location, routes []Route, children ...react.Node) *react.Element {
	if nav == nil {
		nav = NewNavigation()
	}
	if location.Path == "" {
		location.Path = "/"
	}
	return react.CreateElement(routerComponent, react.Props{
		routerValueKey: &routerValue{location: location, routes: routes, nav: nav},
	}, children...)
}

// NavigationOf returns the [Navigation] recorder attached to an element built
// by [Router] or [RouterWith], or nil for any other element.
//
// This is how the render-then-read flow closes: the element is the handle, so a
// caller that has the tree also has the answer, with no extra plumbing to set
// up and nothing to forget to wire.
func NavigationOf(el *react.Element) *Navigation {
	if el == nil {
		return nil
	}
	v, _ := el.Props.Get(routerValueKey).(*routerValue)
	if v == nil {
		return nil
	}
	return v.nav
}

// routerComponent publishes the routing context. It is a component rather than
// a bare provider element so that [NavigationOf] has a props map of its own to
// read from, and so that a tree dump names it.
var routerComponent react.Component = func(p react.Props) react.Node {
	v, ok := p.Get(routerValueKey).(*routerValue)
	if !ok || v == nil {
		panic("router: Router element is missing its routing value; " +
			"build routers with router.Router, never by hand")
	}

	children := p.Children()
	if len(children) == 0 {
		// No children means "just render the routes", the common form.
		children = []react.Node{Routes(nil)}
	}
	return routerCtx.Provider(v, children...)
}

// Routes matches the ambient location against a route table and renders the
// winning chain. It is upstream's `<Routes>` element, with the table passed as
// data because a Go tree cannot carry `<Route>` children as configuration.
//
// Passing nil uses the table the enclosing [Router] was given, which is what
// makes the children form of Router readable:
//
//	router.Router(loc, routes,
//	    react.H("header", nil, nav()),
//	    react.H("main", nil, router.Routes(nil)),
//	    react.H("footer", nil, "…"),
//	)
//
// Routes nests. A Routes element rendered inside a matched route matches
// against the part of the URL its parent route did not consume — that is,
// against the path relative to the parent's [Match.PathnameBase] — and the
// parent's params are merged into the nested matches. That reproduces
// upstream's descendant routes, and it is how a self-contained section of a
// site ships its own route table.
//
// When nothing matches, Routes renders nothing. A route table that wants a
// 404 page should end with a splat route, which ranking guarantees is tried
// last.
func Routes(routes []Route) *react.Element {
	return react.CreateElement(routesComponent, react.Props{routesKey: routes})
}

// routesComponent does the matching. Splitting it out of [Routes] is what lets
// the matching happen during render, where the location and the enclosing match
// are readable through context, rather than at element-construction time when
// neither exists.
var routesComponent react.Component = func(p react.Props) react.Node {
	r := useRouterValue("Routes")

	routes, _ := p.Get(routesKey).([]Route)
	if routes == nil {
		routes = r.routes
	}

	// A Routes nested inside a matched route matches the remainder of the URL,
	// with the parent's params carried in. At the top level parent is the zero
	// matchValue and both of these are no-ops.
	var (
		parent       = react.UseContext(matchCtx)
		parentBase   = "/"
		parentParams map[string]string
	)
	if parent.depth < len(parent.matches) {
		m := parent.matches[parent.depth]
		parentBase, parentParams = m.PathnameBase, m.Params
	}

	remaining := r.location.Path
	if parentBase != "/" {
		remaining = strings.TrimPrefix(remaining, parentBase)
		if remaining == "" {
			remaining = "/"
		}
	}

	matches := MatchRoutes(routes, remaining)
	if matches == nil {
		return nil
	}
	if parentBase != "/" || len(parentParams) > 0 {
		matches = rebaseMatches(matches, parentBase, parentParams)
	}
	return renderMatch(matches, 0)
}

// rebaseMatches shifts a nested table's matches back into whole-URL terms:
// their pathnames are prefixed with the parent's base and the parent's params
// are merged underneath their own.
//
// Merging underneath rather than over is the meaningful choice: a nested route
// that declares ":id" shadows an ancestor's ":id", which is the same rule
// upstream applies and the same one lexical scoping would.
func rebaseMatches(matches []Match, base string, parentParams map[string]string) []Match {
	out := make([]Match, len(matches))
	for i, m := range matches {
		params := make(map[string]string, len(parentParams)+len(m.Params))
		for k, v := range parentParams {
			params[k] = v
		}
		for k, v := range m.Params {
			params[k] = v
		}
		out[i] = Match{
			Route:        m.Route,
			Params:       params,
			Pathname:     joinPaths(base, m.Pathname),
			PathnameBase: joinPaths(base, m.PathnameBase),
		}
	}
	return out
}

// renderMatch renders one level of a match chain, publishing the chain and this
// level's depth so that the level's [Outlet] can render the next one.
//
// A route with no Component renders its child directly, which is what makes a
// pure layout route — a grouping node with a shared path prefix and nothing to
// draw — legal without a pass-through component. Upstream words this as "the
// default element is an <Outlet/>"; skipping straight to the next level is the
// same thing observably, since there is no component at this level for the
// intervening context to serve.
func renderMatch(matches []Match, depth int) react.Node {
	if depth >= len(matches) {
		return nil
	}

	c := matches[depth].Route.Component
	if c == nil {
		return renderMatch(matches, depth+1)
	}
	return matchCtx.Provider(matchValue{matches: matches, depth: depth},
		react.CreateElement(c, nil))
}

// Outlet marks the place where a layout route renders its matched child — the
// port of upstream's `<Outlet/>`, and the mechanism that makes nesting mean
// anything.
//
// A route component that omits an Outlet renders nothing for its children, and
// that is not an error: it is how a leaf route with an accidental child table,
// or a layout that deliberately suppresses a section, behaves.
//
// Upstream's `context` prop (`<Outlet context={…}>` with `useOutletContext`) is
// not ported. It exists to pass a value down without prop drilling, which is
// what [github.com/malcolmston/react.CreateContext] already does, with types.
func Outlet() *react.Element {
	return react.CreateElement(outletComponent, nil)
}

// outletComponent is the one component behind every Outlet element. Keeping it
// a single package-level value means every Outlet has the same element type, so
// two Outlets reconcile against each other by position like any other
// component.
var outletComponent react.Component = func(react.Props) react.Node {
	return outletNode()
}

// outletNode computes what an Outlet renders at the current position. It is
// shared with [UseOutlet], which is the same question asked as a value rather
// than as an element.
func outletNode() react.Node {
	mv := react.UseContext(matchCtx)
	return renderMatch(mv.matches, mv.depth+1)
}
