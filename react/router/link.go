package router

import (
	"strings"

	"github.com/malcolmston/react"
)

// This file holds the three elements that are about *going somewhere*: [Link],
// [NavLink] and [Navigate].
//
// Two of the three survive the absence of a browser essentially intact. An
// anchor with an href is the oldest navigation primitive there is, and it works
// perfectly in server-rendered HTML — upstream's `<Link>` only intercepts the
// click to avoid a page load, which is an optimization, not the feature. So
// Link here renders exactly the markup upstream renders, resolves relative
// targets by the same rules, and simply lets the browser handle the click.
// NavLink's active class is computed from the location the tree was rendered
// for, which is the same computation upstream performs.
//
// [Navigate] is the one that changes meaning, and it changes it in the same way
// [UseNavigate] does: it records an intent instead of performing a navigation.

// Reserved props keys read by [Link] and [NavLink]. They are namespaced so they
// cannot collide with an HTML attribute, and they are stripped from the props
// before the anchor is emitted so they never reach the output.
//
// They exist because upstream passes these as React props on a typed component
// and Go's [github.com/malcolmston/react.Props] is an untyped bag: an options
// struct would mean a second function per option combination, and a named key
// in the bag the caller is already building is the lighter of the two.
const (
	// ActiveClassProp overrides the class [NavLink] adds when it matches.
	// The value must be a string; the default is "active".
	ActiveClassProp = "router:activeClass"

	// EndProp, set to true, makes a [NavLink] active only on an exact path
	// match rather than for descendants too — upstream's `end` prop. It is what
	// keeps a "/" link from being active on every page of the site.
	EndProp = "router:end"

	// toKey carries the link target from the constructor into the component
	// that resolves it, and replaceKey carries [NavigateReplace]'s flag. Both
	// are unexported: they are plumbing, not options.
	toKey      = "router:to"
	replaceKey = "router:replace"
)

// Link renders an anchor to a route — upstream's `<Link to=…>`.
//
// The target is resolved against the route the Link is written in, so a
// relative `to` means what it means upstream: inside the route "/users/:id", a
// Link to "edit" points at "/users/7/edit" and a Link to ".." points at
// "/users". An absolute target is used as written. See [ResolvePath] for the
// full rules, and note that resolution is against the route hierarchy rather
// than against the URL's "directory", which is where it differs from a plain
// HTML relative href.
//
// props are passed through to the anchor unchanged, so class, id, target, rel
// and any aria attribute work as they would on a hand-written <a>. An href in
// props is ignored: the whole point of a Link is that the href is derived.
//
// Unlike upstream there is no click handling, no `replace`, no `state` and no
// `preventScrollReset`: all four describe what happens in a live browser
// session after the click, and there is no session here. The rendered markup is
// the same either way, which is why this is the one part of the library that
// survives the port unchanged.
func Link(to string, props react.Props, children ...react.Node) *react.Element {
	p := props.Clone()
	p[toKey] = to
	return react.CreateElement(linkComponent, p, children...)
}

// linkComponent resolves the target and emits the anchor. It has to be a
// component rather than a plain [react.H] call inside [Link], because
// resolution reads the enclosing route through context and context is only
// readable during a render.
var linkComponent react.Component = func(p react.Props) react.Node {
	to := p.String(toKey)
	resolved := UseResolvedPath(to)

	attrs := anchorProps(p)
	attrs["href"] = resolved.String()
	return react.CreateElement("a", attrs, p.Children()...)
}

// NavLink renders a [Link] that knows whether it points at the current page —
// upstream's `<NavLink>`, and the element a navigation bar is built from.
//
// When the link is active it gains a class (default "active", override it with
// [ActiveClassProp]) and the attribute aria-current="page", which is what makes
// the state perceivable to a screen reader rather than only visible. Both match
// upstream's defaults.
//
// "Active" means the current path is the target path or a descendant of it, so
// a link to "/users" stays highlighted on "/users/7". Set [EndProp] to true for
// an exact match — always do this for a link to "/", which is otherwise a
// prefix of every URL on the site. Comparison is case-insensitive and ignores
// trailing slashes, matching the matcher.
//
// Upstream's function-valued `className`/`style`/`children` props, which
// receive an `{isActive, isPending}` object, are not ported: `isPending`
// describes an in-flight client navigation that cannot exist here, and the
// function form buys nothing over the class this already adds. Use [UseMatch]
// when a component needs the boolean itself.
func NavLink(to string, props react.Props, children ...react.Node) *react.Element {
	p := props.Clone()
	p[toKey] = to
	return react.CreateElement(navLinkComponent, p, children...)
}

// navLinkComponent is [NavLink]'s body: resolve, compare, decorate, emit. It
// deliberately does not delegate to [linkComponent] — doing so would put a
// second component between the NavLink and its anchor for no gain, and the
// shared work is one line.
var navLinkComponent react.Component = func(p react.Props) react.Node {
	to := p.String(toKey)
	resolved := UseResolvedPath(to)
	current := UseLocation().Path

	end, _ := p.Bool(EndProp)
	active := pathIsActive(current, resolved.Path, end)

	attrs := anchorProps(p)
	attrs["href"] = resolved.String()

	if active {
		activeClass := "active"
		if s := p.String(ActiveClassProp); s != "" {
			activeClass = s
		}
		key, existing := existingClass(attrs)
		attrs[key] = strings.TrimSpace(existing + " " + activeClass)
		attrs["aria-current"] = "page"
	}

	return react.CreateElement("a", attrs, p.Children()...)
}

// pathIsActive implements NavLink's match test. Both paths are normalized
// through [splitSegments] first, so "/users", "/users/" and "/Users" are the
// same target — the same tolerance the route matcher has, which matters because
// a highlight that disagrees with the page that rendered is worse than no
// highlight.
//
// The descendant test compares segment lists rather than string prefixes on
// purpose: "/user" is a string prefix of "/users" but not an ancestor of it,
// and a prefix test would light up the wrong link.
func pathIsActive(current, target string, end bool) bool {
	cur := splitSegments(current)
	tgt := splitSegments(target)

	if end && len(cur) != len(tgt) {
		return false
	}
	if len(tgt) > len(cur) {
		return false
	}
	for i, seg := range tgt {
		if !strings.EqualFold(seg, cur[i]) {
			return false
		}
	}
	return true
}

// anchorProps copies a link's props into the map that becomes the anchor's
// attributes, dropping the keys this package reserved for itself and the href
// the caller is not allowed to set. Children are dropped too: they are passed
// to CreateElement variadically instead, so leaving them in the map would
// render them twice.
func anchorProps(p react.Props) react.Props {
	out := make(react.Props, len(p))
	for k, v := range p {
		switch k {
		case toKey, replaceKey, ActiveClassProp, EndProp, react.ChildrenKey, "href":
			continue
		}
		out[k] = v
	}
	return out
}

// existingClass finds where a caller's class list lives in the props, so a
// NavLink appends to it instead of replacing it. React accepts both spellings
// and the SSR emitter maps "className" to "class"; whichever the caller used is
// the one written back, because mixing the two in one element would emit two
// class attributes.
func existingClass(p react.Props) (key, value string) {
	for _, k := range []string{"className", "class"} {
		if p.Has(k) {
			return k, p.String(k)
		}
	}
	return "class", ""
}

// Navigate records a navigation intent when it renders — the declarative form
// of [UseNavigate], and the port of upstream's `<Navigate to=…>`.
//
// It is how a route redirects: render it from a route component that has
// decided the user does not belong here, and the handler that rendered the tree
// finds the target on the [Navigation] recorder afterwards.
//
//	if !session.Valid() {
//	    return router.Navigate("/login")
//	}
//
// It renders no output of its own. Upstream unmounts the tree and swaps in the
// new route; here the surrounding markup is still produced, which is harmless
// precisely because the caller is expected to discard it and redirect. Nothing
// stops a caller from rendering it anyway — a rendered page plus a recorded
// intent is a perfectly reasonable "here is the content, and by the way you
// should be somewhere else".
func Navigate(to string) *react.Element {
	return react.CreateElement(navigateComponent, react.Props{toKey: to})
}

// NavigateReplace is [Navigate] with the replace flag set, matching upstream's
// `<Navigate to=… replace />`. It is a separate function rather than a prop
// because a single boolean option does not justify making every caller build a
// props map.
func NavigateReplace(to string) *react.Element {
	return react.CreateElement(navigateComponent, react.Props{toKey: to, replaceKey: true})
}

// navigateComponent records the intent. Recording during render is a side
// effect in a component body, which the react module warns against, and the
// warning does not apply here for a specific reason: the write is idempotent
// and the last writer wins, so running it twice — which the runtime may do —
// leaves the recorder in exactly the state one run would have. There is no
// alternative placement, either: an effect would fire after the render the
// caller is about to read.
var navigateComponent react.Component = func(p react.Props) react.Node {
	navigate := UseNavigate()
	to := UseResolvedPath(p.String(toKey)).String()

	if replace, _ := p.Bool(replaceKey); replace {
		navigate.Replace(to)
	} else {
		navigate(to)
	}
	return nil
}
