package router

import (
	"net/url"

	"github.com/malcolmston/react"
)

// This file is the read side of the package: the hooks a route component uses
// to find out where it is.
//
// None of them consume a hook slot — they are all [github.com/malcolmston/react.UseContext]
// underneath, which is itself slot-free — so unlike UseState they may legally
// be called conditionally. They are named Use* anyway, for the reason the react
// module names UseContext that way: they need a component fiber to read from,
// and the rule that matters ("call these from a component body") is the same.
//
// Every one of them panics when there is no [Router] above the caller. That is
// a deliberate choice over returning a zero value: a component that reads the
// location outside a Router is not in a degraded state it can cope with, it is
// mounted in the wrong place, and a zero Location would send it down the "we
// are at /" branch and produce a wrong page instead of an error.

// useRouterValue fetches the ambient routing context, or panics naming the hook
// the caller actually wrote. The hook name is threaded through rather than
// derived at runtime because a stack-walking diagnostic would be both slower
// and less reliable than the caller simply saying who it is.
func useRouterValue(hook string) *routerValue {
	v := react.UseContext(routerCtx)
	if v == nil {
		panic("router: " + hook + " was called outside a Router; " +
			"wrap the tree in router.Router(location, routes, …) — every routing " +
			"hook reads the location and route table from that context")
	}
	return v
}

// UseLocation returns the location the tree is being rendered for — upstream's
// `useLocation`, and the hook every other one in this file is a convenience
// over.
//
// The value is fixed for the whole render: there is no history to change
// underneath it, so unlike upstream this can never return a different location
// on a later render of the same [Router] element. That makes it safe to use in
// a dependency list without the usual "did the location object's identity
// change" worry, since a [Location] is a comparable value rather than an
// object.
func UseLocation() Location {
	return useRouterValue("UseLocation").location
}

// UseParams returns the dynamic segments captured by the route chain above and
// including the calling component — upstream's `useParams`.
//
// Params accumulate down the chain, so a component under "/users/:id/posts/:postID"
// sees both keys regardless of which of the two routes it belongs to. A splat
// is under "*". Values are URL-decoded.
//
// The result is never nil, and it is a per-match copy: writing to it cannot
// affect another component or a later render. Calling this inside a [Router]
// but outside any matched route — in page chrome, say — returns an empty map
// rather than panicking, because "no params here" is a real and unremarkable
// state.
func UseParams() map[string]string {
	useRouterValue("UseParams")

	mv := react.UseContext(matchCtx)
	if mv.depth < len(mv.matches) {
		params := mv.matches[mv.depth].Params
		out := make(map[string]string, len(params))
		for k, v := range params {
			out[k] = v
		}
		return out
	}
	return map[string]string{}
}

// UseSearchParams returns the parsed query string of the current location.
//
// Upstream returns a `[searchParams, setSearchParams]` pair, and the setter
// half is deliberately absent here: setting search params is a navigation, and
// navigation on a server is [UseNavigate] recording an intent, not a mutation
// that re-renders. Writing the pair would imply a round trip this package
// cannot make. To change a query parameter, build the new URL and navigate:
//
//	q := router.UseSearchParams()
//	q.Set("page", "2")
//	navigate(router.UseLocation().Path + "?" + q.Encode())
//
// The returned [net/url.Values] is freshly parsed on every call, so mutating it
// is safe and affects nothing else.
func UseSearchParams() url.Values {
	return useRouterValue("UseSearchParams").location.Query()
}

// NavigateFunc is what [UseNavigate] returns: callable with a target for the
// ordinary push, with a [NavigateFunc.Replace] method for upstream's
// `navigate(to, {replace: true})`.
//
// The shape follows the react module's SetStateFunc, for the same reason and
// with the same trick. Go has no overloading and no optional parameters, so an
// options argument has to become either a second function or a variadic tail;
// a named func type with a method keeps the common call reading exactly like
// upstream's while leaving the variant discoverable from the type.
//
// The variadic parameter is plumbing for [NavigateFunc.Replace], not part of
// the calling convention: call a NavigateFunc with exactly one argument.
type NavigateFunc func(to string, replace ...bool)

// Replace records a navigation that should not leave the current URL behind
// it. Whether that distinction means anything is the caller's decision — there
// is no history stack to replace an entry in, so the flag is carried through to
// [Navigation.IsReplace] rather than acted on.
func (n NavigateFunc) Replace(to string) {
	if n == nil {
		return
	}
	n(to, true)
}

// UseNavigate returns a function that records an intent to navigate.
//
// This is where the absence of a browser is most visible, so read
// [Navigation] before using it. In short: calling the returned function does
// not change the tree, does not re-render, and does not move the location. It
// writes the target onto the [Router]'s recorder, which the code that rendered
// the tree reads afterwards with [NavigationOf] and turns into whatever
// navigation means in its context — an HTTP redirect, in almost every case.
//
// Because the effect is out-of-band, calling it during render is meaningful
// rather than forbidden: a route component that discovers it should not be here
// can record a redirect while rendering, and the handler acts on it once the
// render finishes. [Navigate] is that pattern as an element.
func UseNavigate() NavigateFunc {
	nav := useRouterValue("UseNavigate").nav
	return func(to string, replace ...bool) {
		if len(replace) > 0 && replace[0] {
			nav.Replace(to)
			return
		}
		nav.Push(to)
	}
}

// UseMatch matches a pattern against the current location and reports the
// captured params — upstream's `useMatch`.
//
// The pattern is absolute and anchored at both ends, independent of where the
// calling component sits in the route tree: it answers "is the browser on this
// URL shape?", which is what a navigation highlight or a conditional banner
// wants. It has nothing to do with the route table, so a pattern that is not in
// it is a perfectly reasonable question to ask.
//
// The params map is non-nil whenever ok is true.
func UseMatch(pattern string) (map[string]string, bool) {
	loc := useRouterValue("UseMatch").location
	return MatchPath(pattern, loc.Path)
}

// UseOutlet returns the node [Outlet] would render at this position, or nil
// when the calling route has no matched child — upstream's `useOutlet`.
//
// It is the hook form of the element, and the reason to reach for it is that a
// node can be tested before it is rendered:
//
//	child := router.UseOutlet()
//	if child == nil {
//	    return react.H("p", nil, "Pick something from the list.")
//	}
//	return react.H("section", nil, child)
//
// Writing that with [Outlet] alone is not possible, because an Outlet element
// is opaque until it renders.
func UseOutlet() react.Node {
	useRouterValue("UseOutlet")
	return outletNode()
}

// UseMatches returns the whole chain of matches being rendered, outermost
// first, or nil outside a matched route. It is upstream's `useMatches`, and it
// is what a breadcrumb trail is built from: each Match carries the route that
// produced it and the pathname it owns, so a crumb needs nothing else.
//
// The slice is a copy; the Params maps inside it are the live per-match ones
// and must not be mutated.
func UseMatches() []Match {
	useRouterValue("UseMatches")

	mv := react.UseContext(matchCtx)
	if len(mv.matches) == 0 {
		return nil
	}
	return append([]Match(nil), mv.matches...)
}

// UseResolvedPath resolves a link target against the route the calling
// component belongs to, returning the location it points at — upstream's
// `useResolvedPath`. It is what [Link] uses to turn a relative `to` into an
// href, exposed because a component that builds a URL for something other than
// an anchor needs the same resolution.
//
// Resolution is against the current match's [Match.PathnameBase] — the path the
// route owns, with any splat tail removed — falling back to the location's own
// path outside a matched route. See [ResolvePath] for the rules.
func UseResolvedPath(to string) Location {
	r := useRouterValue("UseResolvedPath")

	from := r.location.Path
	mv := react.UseContext(matchCtx)
	if mv.depth < len(mv.matches) {
		from = mv.matches[mv.depth].PathnameBase
	}
	return ResolvePath(to, from)
}
