package react

import (
	"fmt"
	"reflect"
	"sync"
)

// fiber is one node of the rendered tree — the mutable counterpart to the
// immutable [Element]. Elements describe what to render; fibers are what the
// runtime actually keeps, and they are where hook state lives. A fiber survives
// across renders as long as its (Type, Key) pair keeps matching in the same
// sibling position, which is exactly the rule that decides whether a component
// keeps its state or is remounted from scratch.
//
// Children are a linked list (child → sibling → …) rather than a slice, the
// same shape React uses: it makes insertion during reconciliation allocation
// free and gives every fiber a stable parent pointer for walking upward, which
// is how context lookup and boundary search work.
type fiber struct {
	// typ, key and props mirror the element this fiber was created from. props
	// is replaced on every re-render; typ and key never change for the life of
	// the fiber, because a change in either forces a remount.
	typ   any
	key   string
	props Props
	ref   any

	parent  *fiber
	child   *fiber
	sibling *fiber

	// hooks are this fiber's hook slots, in call order. Only function
	// components have them. hookIdx is the cursor used during a render pass and
	// is reset to zero each time the component function is entered — this is
	// what makes "hooks must be called in the same order every render" a real
	// constraint rather than a style rule.
	hooks   []*hook
	hookIdx int

	// root is the tree this fiber belongs to, so a hook holding only its fiber
	// can still schedule an update.
	root *Root

	// rendered is the node the component function returned on its last render.
	// Bailout paths reuse it instead of calling the component again.
	rendered Node

	// state is a scratch slot for element-type handlers registered through
	// [RegisterElementRenderer] — a Suspense boundary's resolved/fallback flag,
	// a Lazy component's loaded value. The runtime never inspects it.
	state any

	// needsRender is set when this fiber's own state changed; subtreeDirty is
	// set on every ancestor of such a fiber. Together they answer the question
	// a memoized component has to ask before bailing out: "did anything below
	// me request an update?" Without the ancestor flag, a memo boundary with
	// equal props would swallow its own child's state update.
	needsRender  bool
	subtreeDirty bool

	// wasDirty is the value of (needsRender || subtreeDirty) captured at the
	// start of this fiber's render, before the flags were cleared. Bailout
	// handlers read it; nothing else should.
	wasDirty bool

	// unmounted guards against a cleanup running twice when a subtree is
	// dropped while an effect from the same commit is still pending.
	unmounted bool
}

// markNeedsRender flags this fiber as having pending work and marks the path to
// the root, so a memoized ancestor knows not to bail out over the top of it.
func (f *fiber) markNeedsRender() {
	f.needsRender = true
	for p := f.parent; p != nil; p = p.parent {
		p.subtreeDirty = true
	}
}

// hook is a single hook slot. Every hook — state, effect, memo, ref — stores
// itself here; what value holds is private to the hook that owns the slot.
//
// deps is the dependency list from the previous render, kept by the hooks that
// take one so they can decide whether to recompute. cleanup is the teardown
// function an effect returned, run before the next effect fire and again when
// the fiber unmounts.
type hook struct {
	value   any
	deps    []any
	cleanup func()

	// kind names the hook that owns the slot ("state", "effect", …). It exists
	// so a mismatched hook order produces a message naming both hooks rather
	// than a confusing type assertion panic deep inside the runtime.
	kind string
}

// renderState is the ambient state of the render currently in progress. React
// calls this the dispatcher: hooks are plain functions with no receiver, so
// they need a way to find "the component being rendered right now". Renders are
// serialized by [Root.mu], so a single package-level pointer is sufficient and
// no goroutine ever observes another root's render.
var currentlyRendering *fiber

// currentFiber returns the fiber being rendered, panicking with React's own
// diagnostic when a hook is called outside a component body. Every hook must
// call this first.
func currentFiber() *fiber {
	if currentlyRendering == nil {
		panic("react: hook called outside of a component render; " +
			"hooks may only be called from the body of a function component")
	}
	return currentlyRendering
}

// nextHook advances the hook cursor and returns the slot for this call site,
// along with whether this is the component's first render (the slot was just
// created). Every hook is built on this: it is the whole reason hook order must
// be stable, because the slot is identified by call index and nothing else.
//
// kind is recorded on first render and checked on every later one, turning an
// order violation — a hook behind an if — into an immediate, explicit error
// instead of silently handing one hook another hook's state.
func nextHook(kind string) (h *hook, first bool) {
	f := currentFiber()
	i := f.hookIdx
	f.hookIdx++

	if i < len(f.hooks) {
		h = f.hooks[i]
		if h.kind != kind {
			panic(fmt.Sprintf("react: hook order changed between renders: "+
				"slot %d was %q, now %q; hooks must be called unconditionally, "+
				"in the same order, on every render", i, h.kind, kind))
		}
		return h, false
	}

	h = &hook{kind: kind}
	f.hooks = append(f.hooks, h)
	return h, true
}

// DepsChanged reports whether a hook's dependency list differs from the one
// recorded on the previous render, using React's comparison rules: a nil list
// means "no dependencies were given, always re-run", a length change always
// counts as changed, and individual entries are compared with Go's == via
// reflect.DeepEqual's shallow cousin ObjectsEqual.
//
// Hooks that take a deps argument should call this and then store the new list
// on the hook slot.
func DepsChanged(prev, next []any) bool {
	if prev == nil || next == nil {
		return true
	}
	if len(prev) != len(next) {
		return true
	}
	for i := range next {
		if !ObjectsEqual(prev[i], next[i]) {
			return true
		}
	}
	return false
}

// ObjectsEqual is the port's equivalent of React's Object.is, the comparison
// every dependency list and every state bailout uses. Go's == panics on
// uncomparable types (slices, maps, functions), which a dependency list can
// easily contain, so those are compared by identity where possible and
// otherwise reported as unequal — the conservative answer, since reporting
// "changed" only costs a re-render while reporting "same" would skip one that
// was needed.
func ObjectsEqual(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}

	ta, tb := reflect.TypeOf(a), reflect.TypeOf(b)
	if ta != tb {
		return false
	}
	if ta.Comparable() {
		return a == b
	}

	// Uncomparable kinds: slices, maps and funcs are equal only when they are
	// the same underlying object, which pointer identity captures.
	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	switch va.Kind() {
	case reflect.Slice, reflect.Map:
		return va.Pointer() == vb.Pointer() && va.Len() == vb.Len()
	case reflect.Func:
		// Funcs get their own case because reflect.Value.Len is not defined for
		// them and panics. The bug this avoids was subtle: && short-circuits, so
		// a shared Len call only ever reached the panic when the two pointers
		// were equal — meaning ObjectsEqual(f, f) blew up while comparing two
		// different funcs quietly worked.
		//
		// Pointer() is a func's *code* pointer, so two closures created from the
		// same literal with different captured variables compare equal. Go
		// offers nothing finer, and callers should not put closures in a
		// dependency list expecting per-instance identity.
		return va.Pointer() == vb.Pointer()
	}
	return false
}

// elementRenderers maps a Go type of [Element.Type] to the handler that renders
// it. It is the extension point that keeps [renderFiber] from growing a case
// per feature: context providers, Memo, Lazy, Suspense, StrictMode and Profiler
// each register their own renderer from an init function in their own file.
var elementRenderers = map[reflect.Type]func(*fiber){}

// RegisterElementRenderer registers fn as the renderer for every element whose
// Type has the same Go type as sample. Call it from an init function. The
// handler is responsible for calling [reconcileChildren] with whatever children
// its element type should produce; a handler that renders nothing should call
// it with an empty slice so any previous children are unmounted.
//
// Registering the same type twice panics: two features silently fighting over
// one element type would be near-impossible to debug.
func RegisterElementRenderer(sample any, fn func(f *fiber)) {
	t := reflect.TypeOf(sample)
	if t == nil {
		panic("react: RegisterElementRenderer needs a non-nil sample value")
	}
	if _, dup := elementRenderers[t]; dup {
		panic(fmt.Sprintf("react: element renderer for %s registered twice", t))
	}
	elementRenderers[t] = fn
}

// RegisterTransparentType registers an element type that renders its children
// and contributes nothing of its own to the output — the shape [Fragment] has,
// and the shape a marker element needs.
//
// It exists because [RegisterElementRenderer] takes a handler over the internal
// fiber type, which no package outside this one can name. A marker element — a
// reference to a component that will be rendered somewhere else entirely, as in
// [github.com/malcolmston/react/rsc] — needs its children reconciled normally
// while remaining identifiable by its own type to whoever walks the result.
// This is the supported way to declare one from another package.
func RegisterTransparentType(sample any) {
	RegisterElementRenderer(sample, func(f *fiber) {
		reconcileChildren(f, f.props.Children())
	})
}

// SuspendSignal is panicked by a component that cannot finish rendering until
// some asynchronous work completes — the mechanism behind [Use] and lazy
// components. The nearest Suspense boundary above the panicking component
// recovers it, renders its fallback instead, and re-renders the subtree once
// Ready is closed.
//
// A SuspendSignal that reaches the root with no boundary to catch it is
// re-panicked as an ordinary error, matching React's "a component suspended
// while responding to synchronous input" failure.
type SuspendSignal struct {
	// Ready is closed when the awaited work has finished. A boundary waits on
	// it before re-rendering.
	Ready <-chan struct{}
}

// Suspend aborts the current render and asks the nearest Suspense boundary to
// show its fallback until ready is closed. It never returns.
func Suspend(ready <-chan struct{}) {
	panic(SuspendSignal{Ready: ready})
}

// renderFiber renders one fiber and, recursively, its subtree. Rendering is
// recursive rather than a work loop because Go's panic/recover is the natural
// way to express Suspense and error boundaries, and recover only works within a
// call stack — a boundary handler wraps its recursive descent in a deferred
// recover and gets both features almost for free.
func renderFiber(f *fiber) {
	// Capture and clear the update flags before dispatching. A bailout handler
	// reads wasDirty to decide whether it may reuse its existing children; every
	// other path simply starts each render with a clean slate.
	f.wasDirty = f.needsRender || f.subtreeDirty
	f.needsRender, f.subtreeDirty = false, false

	switch t := f.typ.(type) {
	case string:
		// Host element: nothing to compute, just diff the declared children.
		reconcileChildren(f, f.props.Children())

	case textTag:
		// Text leaf: no children, and its content lives in props.
		reconcileChildren(f, nil)

	case fragmentTag:
		reconcileChildren(f, f.props.Children())

	case Component:
		renderComponent(f, t)

	default:
		if h, ok := elementRenderers[reflect.TypeOf(f.typ)]; ok {
			h(f)
			return
		}
		panic(fmt.Sprintf("react: element type %T is not renderable; "+
			"use a tag string, a react.Component, react.Fragment, or a type "+
			"registered with RegisterElementRenderer", f.typ))
	}
}

// renderComponent invokes a function component with the hook dispatcher pointed
// at its fiber, then reconciles whatever it returned. The dispatcher is saved
// and restored rather than cleared, because rendering is recursive and a parent
// component's render is still in progress on the stack below this one.
func renderComponent(f *fiber, c Component) {
	prev := currentlyRendering
	currentlyRendering = f
	f.hookIdx = 0

	// The dispatcher must be restored even when the component panics — a
	// suspending component or an error caught by a boundary would otherwise
	// leave every later hook call attributed to the wrong fiber.
	defer func() { currentlyRendering = prev }()

	out := c(f.props)

	// Restore before reconciling: children render with their own dispatcher,
	// and leaving this fiber current would let a child's hook write into it.
	currentlyRendering = prev

	f.rendered = out
	reconcileChildren(f, []Node{out})
}

// reconcileChildren diffs nodes against f's current children and rebuilds the
// child list, then renders each surviving child.
//
// The matching rule is React's: a child keeps its fiber — and therefore its
// hooks, its state and its DOM identity — when the old and new child at the
// same slot agree on both element type and key. Keyed children are matched by
// key across the whole sibling list, so reordering a keyed list moves state
// with the items instead of leaving it behind at a position. Keyless children
// match by index, which is why inserting at the front of a keyless list
// silently shifts everyone's state by one.
//
// Every old fiber that finds no match is unmounted depth-first, running its
// effect cleanups from the leaves upward.
func reconcileChildren(f *fiber, nodes []Node) {
	// Index the existing children so a keyed match can find its partner
	// regardless of how far it moved.
	var (
		byKey    map[string]*fiber
		byIndex  []*fiber
		existing = map[*fiber]bool{}
	)
	for c := f.child; c != nil; c = c.sibling {
		byIndex = append(byIndex, c)
		existing[c] = true
		if c.key != "" {
			if byKey == nil {
				byKey = map[string]*fiber{}
			}
			byKey[c.key] = c
		}
	}

	var (
		head, tail *fiber
		matched    = map[*fiber]bool{}
	)

	for i, n := range NormalizeChildren(nodes) {
		el := nodeToElement(n)
		if el == nil {
			continue
		}

		// Find the fiber this element should reuse.
		var reuse *fiber
		if el.Key != "" {
			if c, ok := byKey[el.Key]; ok && !matched[c] && sameType(c.typ, el.Type) {
				reuse = c
			}
		} else if i < len(byIndex) {
			if c := byIndex[i]; !matched[c] && c.key == "" && sameType(c.typ, el.Type) {
				reuse = c
			}
		}

		var child *fiber
		if reuse != nil {
			matched[reuse] = true
			child = reuse
			child.props = el.Props
			child.ref = el.Ref
		} else {
			child = &fiber{typ: el.Type, key: el.Key, props: el.Props, ref: el.Ref, root: f.root}
		}
		child.parent = f
		child.sibling = nil

		if head == nil {
			head = child
		} else {
			tail.sibling = child
		}
		tail = child
	}

	// Anything left over disappeared from the output and must be torn down.
	for c := range existing {
		if !matched[c] {
			unmount(c)
		}
	}

	f.child = head

	for c := head; c != nil; c = c.sibling {
		renderFiber(c)
	}
}

// sameType reports whether two element types are the same for reconciliation.
// Component values are functions and therefore uncomparable with ==, so they
// are compared by code pointer: two references to the same function literal
// match, two structurally identical but distinct functions do not — which is
// the right answer, since they are different components.
func sameType(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ta, tb := reflect.TypeOf(a), reflect.TypeOf(b)
	if ta != tb {
		return false
	}
	if ta.Kind() == reflect.Func {
		return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
	}
	if ta.Comparable() {
		return a == b
	}
	return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
}

// unmount tears down a fiber and its subtree, running effect cleanups from the
// leaves upward so a child's cleanup never observes a parent that has already
// been torn down. It is idempotent.
func unmount(f *fiber) {
	if f == nil || f.unmounted {
		return
	}
	for c := f.child; c != nil; c = c.sibling {
		unmount(c)
	}
	f.unmounted = true
	for _, h := range f.hooks {
		if h.cleanup != nil {
			cleanup := h.cleanup
			h.cleanup = nil
			cleanup()
		}
	}
	f.child = nil
}

// walk visits f and every fiber beneath it in render order, calling fn on each.
// It is the traversal every consumer of a rendered tree is built on — HTML
// rendering, test queries, effect collection.
func walk(f *fiber, fn func(*fiber)) {
	if f == nil {
		return
	}
	fn(f)
	for c := f.child; c != nil; c = c.sibling {
		walk(c, fn)
	}
}

// findAncestor walks up from f — excluding f itself — and returns the first
// fiber for which pred is true, or nil. Context lookup and boundary search are
// both this function with a different predicate.
func findAncestor(f *fiber, pred func(*fiber) bool) *fiber {
	for p := f.parent; p != nil; p = p.parent {
		if pred(p) {
			return p
		}
	}
	return nil
}

// renderMu serializes rendering across all roots. React's runtime is
// single-threaded; the dispatcher that hooks read is a package-level pointer,
// so concurrent renders would interleave and hand hooks the wrong fiber. This
// mutex makes that impossible rather than merely undocumented, at the cost of
// disallowing genuinely parallel renders of independent roots.
var renderMu sync.Mutex
