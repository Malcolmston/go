package react

// Portals, without a DOM.
//
// React's createPortal answers a layout problem that only exists in a document:
// a modal, a tooltip or a toast must sit at the top of the DOM so that overflow
// and stacking contexts do not clip it, while staying *logically* inside the
// component that owns it — keeping its state, its context and its place in the
// event bubbling path. React resolves that by leaving the subtree where it is in
// the React tree and appending its host nodes to a different DOM container.
//
// This port has no DOM, so "a different container node" has no referent. What it
// does have is a tree and an output string, and the half of the portal contract
// that survives is exactly the half that matters: the subtree renders in place,
// so it keeps its fiber, its hooks and every context above it, while its *output*
// is collected under a name instead of being emitted inline. That is a named
// slot, and it is the honest analogue — a caller who would have appended to
// document.body appends the collected string wherever their layout wants it.
//
// # How the split is actually done
//
// A portal is an ordinary host element with a reserved tag, [PortalTag], and the
// container id in a reserved attribute, [PortalContainerProp]. Everything about
// reconciliation therefore comes for free: the portal fiber has a stable type, so
// its children keep their state across re-renders, and it lives inside the
// provider chain, so [UseContext] resolves through it.
//
// [RenderToString] treats the wrapper as what it is — an ordinary custom
// element — and emits it inline:
//
//	<main><react-portal data-react-portal="modal"><p>hi</p></react-portal></main>
//
// The reserved tag contains a hyphen, so that output is valid HTML and a browser
// handed it renders the portal contents in place: the most useful possible
// degradation for a caller who never asked for the split.
//
// [RenderPortalsToString] asks for the split. It renders once — one tree, one
// set of hooks, every context intact — and the *emitter* diverts each portal
// subtree into a buffer named by its container as the bytes are produced.
//
// Doing it during emission rather than by scanning the finished markup is not a
// stylistic preference. The emitter knows which output came from which fiber; a
// text scan can only pattern-match, and [DangerousHTML] content is written out
// byte for byte, so raw markup containing a literal "<react-portal" is
// indistinguishable from a real boundary once it is bytes. See
// renderSeparatingPortals in ssr.go.

// PortalTag is the host tag name a portal renders as. It is a custom element
// name (it contains a hyphen), which is deliberate: markup that has not been
// through [RenderPortalsToString] is still well-formed HTML rather than a broken
// document, and a browser handed it renders the portal contents in place — the
// most useful possible degradation.
//
// Do not use this tag for an ordinary host element. [RenderPortalsToString]
// treats every occurrence of it as a portal boundary and reports one without a
// container id as an error.
const PortalTag = "react-portal"

// PortalContainerProp is the prop, and therefore the HTML attribute, that names
// the container a portal's output is collected under. It is spelled as a data-*
// attribute so that [AttributeName] passes it through verbatim and so that the
// un-split markup stays valid.
const PortalContainerProp = "data-react-portal"

// CreatePortal renders children under containerID instead of inline, mirroring
// React's createPortal(children, container).
//
// The subtree stays exactly where it is in the element tree, which is the whole
// point of a portal: it keeps its hook state across re-renders, it reads every
// context provided above it, and an error it raises is caught by the nearest
// enclosing [ErrorBoundary] — not by whatever happens to surround the container.
// Only its *output* moves, and only when the caller asks for the split with
// [RenderPortalsToString] or [PortalContents].
//
// containerID replaces React's DOM node argument. React takes a live Element
// because it is going to call appendChild on it; there is nothing to append to
// here, so the container is a name the caller chooses and later looks up:
//
//	main, portals, err := react.RenderPortalsToString(page)
//	// main    — the page with the modal's markup removed
//	// portals — map[string]string{"modal": "<div class=\"modal\">…</div>"}
//
// children is a single [Node] rather than variadic, matching createPortal's
// signature; pass a slice or a [Frag] for several. An empty containerID panics,
// because an unnamed container can never be retrieved and the mistake is far
// cheaper to find at construction than in a missing subtree.
//
// Several portals may target the same container. Their output is concatenated in
// render order, the same order document.body would have ended up in.
func CreatePortal(children Node, containerID string) *Element {
	if containerID == "" {
		panic("react: CreatePortal needs a non-empty container id; " +
			"the id is how the rendered output is retrieved later")
	}
	return CreateElement(PortalTag, Props{PortalContainerProp: containerID}, children)
}

// PortalContainer reports whether el is a portal and, if so, the container id it
// targets. It is the [Element]-level counterpart of [IsFragment] and [HostTag],
// for code that inspects a tree before it is rendered.
func PortalContainer(el *Element) (containerID string, ok bool) {
	if el == nil {
		return "", false
	}
	if tag, isHost := HostTag(el); !isHost || tag != PortalTag {
		return "", false
	}
	id, _ := el.Props[PortalContainerProp].(string)
	return id, id != ""
}

// PortalContainer reports whether this rendered node is a portal and, if so, the
// container id it targets. It is the [Instance]-level counterpart of
// [Instance.Tag], and is defined here rather than in testing.go so that the whole
// portal feature lives in one file.
func (i Instance) PortalContainer() (containerID string, ok bool) {
	tag, isHost := i.Tag()
	if !isHost || tag != PortalTag {
		return "", false
	}
	id, _ := i.Props()[PortalContainerProp].(string)
	return id, id != ""
}

// PortalContents returns the live contents of every portal in r, keyed by
// container id. It is the [Instance] view of what [RenderPortalsToString] returns
// as strings — the answer to "what did this tree portal out, and where?" without
// serializing anything.
//
// The values are the portal wrapper's own children, in render order, with the
// wrapper itself omitted: a portal is a routing instruction, not a node the
// caller asked for. Several portals targeting one container contribute their
// children to the same slice, concatenated in render order.
//
// The returned Instances are live handles into r's tree, so they reflect later
// re-renders — but the *map* is a snapshot, so a portal that appears or
// disappears needs a fresh call. A nil or unmounted Root yields an empty,
// non-nil map, so ranging over the result is always safe.
//
// A portal nested inside another portal appears twice, as it must: once as a
// descendant of the outer portal's contents (that is where it lives in the tree)
// and once under its own container id. [RenderPortalsToString] resolves the
// same nesting by lifting the inner span out of the outer one's markup, so the
// string form never duplicates it.
func PortalContents(r *Root) map[string][]Instance {
	out := map[string][]Instance{}
	if r == nil {
		return out
	}
	tree := r.Tree()
	if !tree.Valid() {
		return out
	}
	for _, p := range tree.FindAllByTag(PortalTag) {
		id, ok := p.PortalContainer()
		if !ok {
			continue
		}
		out[id] = append(out[id], p.Children()...)
	}
	return out
}

// RenderPortalsToString renders node and returns the markup with every portal
// removed, plus one markup string per container id.
//
// It is [RenderToStaticMarkup] plus the split, and it renders exactly once: the
// tree is mounted, walked and unmounted the same way, so hooks run once, effects
// fire once and the portal subtrees see the same contexts they would have seen
// inline. Rendering the portals separately would have been the obvious
// implementation and is the wrong one — a second [Root] means a second set of
// hooks and no access to the providers above the portal.
//
// Static markup, despite the name, and the name is the older of the two: a
// document torn into a main string plus one string per container is not a tree
// any client can hydrate in one pass, so the "<!-- -->" separator
// [RenderToString] writes between adjacent text nodes would be noise in every
// piece. Adjacent text is therefore written as one run here; see ssr_textsep.go.
//
//	main, portals, err := react.RenderPortalsToString(page)
//	fmt.Fprint(w, main)
//	fmt.Fprint(w, portals["modal-root"])
//
// Ordering: several portals targeting one container are concatenated in render
// order. A portal nested inside another is lifted out of the outer one's markup
// and appended to its own container after the outer portal's, so no fragment of
// markup ever appears twice in the result.
//
// portals is always non-nil and contains only containers that actually rendered
// something — a container with no portal has no entry, which is different from an
// entry holding "". Errors are [RenderToStaticMarkup]'s, plus one of this function's
// own: a [PortalTag] element with no container id, which means the reserved tag
// was used as an ordinary host element.
func RenderPortalsToString(node Node) (main string, portals map[string]string, err error) {
	return renderSeparatingPortals(node)
}

// portalContainerOfFiber reports whether a rendered fiber is a portal wrapper,
// and the container it belongs to. It is the emitter's test, which is why it
// takes a fiber rather than an [Element] or an [Instance].
func portalContainerOfFiber(f *fiber) (string, bool) {
	if f == nil {
		return "", false
	}
	tag, ok := f.typ.(string)
	if !ok || tag != PortalTag {
		return "", false
	}
	id, _ := f.props[PortalContainerProp].(string)
	return id, true
}
