package react

// This file holds the component wrappers that are *element types* rather than
// hooks: [Memo], [StrictMode] and [Profiler]. In React each of these is a value
// you put in the `type` position of an element — `<Memo(C) …/>`, `<StrictMode>`,
// `<Profiler>` — and not something a component calls from its body. The port
// keeps that distinction: every wrapper here plugs into the runtime through
// [RegisterElementRenderer], so [renderFiber] never grows a case for them and
// each wrapper owns the decision of what its subtree reconciles to.
//
// The interesting one is [Memo]. Its whole job is to *not* reconcile — to leave
// the child fibers it already has exactly where they are — and the entire
// difficulty is knowing when that is safe. See renderMemoFiber for the rule.

import (
	"fmt"
	"reflect"
	"time"
)

// MemoType is the element type produced by [Memo] and [MemoWith]: a component
// wrapped in a props-equality bailout, the equivalent of React's
// `React.memo(C)`.
//
// It is a struct rather than a function because [Element.Type] is matched by Go
// type during reconciliation (see sameType), and a *MemoType pointer gives the
// wrapper a stable identity that survives re-renders while still carrying the
// wrapped component and its comparator. That stability is a requirement, not an
// implementation detail: as in React, a memo type must be created **once** and
// reused —
//
//	var Row = react.Memo(rowComponent) // package level, or built once in a hook
//
// — because calling Memo again on every render yields a different pointer, which
// the reconciler reads as a different element type and therefore remounts the
// subtree, destroying the state the memoization was supposed to protect.
//
// All fields are unexported: a MemoType is opaque, immutable and safe to share
// across goroutines and roots.
type MemoType struct {
	// component is the wrapped component. The memo fiber renders exactly one
	// child, an element of this type carrying the memo fiber's own props, so the
	// wrapped component sees the props the caller passed to the wrapper.
	component Component

	// areEqual decides whether a re-render may be skipped. It is never nil;
	// [Memo] fills in [ShallowEqualProps].
	areEqual func(prev, next Props) bool
}

// Memo wraps c so that re-rendering it with shallow-equal props is skipped
// entirely. It is React.memo with React's default comparison,
// [ShallowEqualProps].
//
// Memo is a *performance* tool and never a correctness one: a memoized subtree
// that bails out still receives its own state updates (see [MemoWith] for the
// rule), so the only thing wrapping a component in Memo can change is how often
// it runs — never what it eventually shows.
//
// Create the wrapper once and reuse it; see [MemoType] for why. Passing a nil
// component panics immediately rather than at the first render, because the
// wrapper is normally built at package initialisation where a stack trace still
// points at the mistake.
func Memo(c Component) *MemoType {
	return MemoWith(c, ShallowEqualProps)
}

// MemoWith is [Memo] with an explicit comparator, matching React.memo's optional
// second argument. areEqual receives the previous and next props and returns
// true when they are equivalent — note the direction, which is the opposite of a
// `shouldComponentUpdate`: returning true means "these are the same, skip the
// render".
//
// A custom comparator is the right answer whenever the default's identity
// comparison is too strict: props holding a freshly built slice, a closure, or
// children (see [ShallowEqualProps]) all defeat the default. It is also the only
// safe way to memoize over a prop whose identity changes but whose meaning does
// not — compare the fields that actually drive the output.
//
// A comparator must be a pure function of its two arguments and must not mutate
// either map. It is not consulted on the first render (there is no previous
// props to compare against) and, crucially, is not consulted when anything in
// the memoized subtree has requested an update: a state update raised by a
// descendant always wins over the comparator, so a memo boundary can never
// swallow its own child's update.
func MemoWith(c Component, areEqual func(prev, next Props) bool) *MemoType {
	if c == nil {
		panic("react: Memo called with a nil component")
	}
	if areEqual == nil {
		areEqual = ShallowEqualProps
	}
	return &MemoType{component: c, areEqual: areEqual}
}

// init registers the memo renderer. [RegisterElementRenderer] keys on the Go
// type of the sample, so one registration with a typed nil pointer covers every
// *MemoType value ever created.
func init() {
	RegisterElementRenderer((*MemoType)(nil), renderMemoFiber)
}

// renderMemoFiber is the bailout, and it is the reason this package has a memo
// at all. It answers one question: may this fiber keep the children it already
// has, without re-running anything?
//
// The answer is yes only when *both* of the following hold:
//
//  1. Nothing in this subtree asked to re-render. f.wasDirty is
//     (needsRender || subtreeDirty) as captured by [renderFiber] immediately
//     before it dispatched here — the flags themselves have already been
//     cleared, which is why the captured copy exists. It is true when this
//     fiber, or any fiber beneath it, called scheduleUpdate. Honouring it is
//     what stops a memo boundary from swallowing a descendant's own state
//     update: the parent re-renders the memo with the very same props map, the
//     comparator says "equal", and without this check the update that the child
//     scheduled would be silently dropped.
//
//  2. The comparator says the new props are equivalent to the ones stored on the
//     previous render.
//
// When both hold the handler does nothing at all — no reconcileChildren call, no
// traversal — so the existing child fibers, their hooks and their state stay
// exactly as they were. That "do nothing" is the entire feature; anything else
// here (reusing f.rendered, re-reconciling the old element) would still walk the
// subtree and defeat the purpose.
//
// Otherwise the new props are recorded and the wrapped component is reconciled
// as this fiber's single child. Rendering the wrapper and the wrapped component
// as two fibers rather than one keeps the hooks of the wrapped component in a
// fiber whose type is the plain [Component], so a memoized component's hooks
// behave identically to an unmemoized one's.
func renderMemoFiber(f *fiber) {
	m, ok := f.typ.(*MemoType)
	if !ok || m == nil {
		panic(fmt.Sprintf("react: memo renderer invoked for element type %T", f.typ))
	}

	next := f.props
	if prev, hadPrev := f.state.(Props); hadPrev && !f.wasDirty && m.areEqual(prev, next) {
		return
	}

	// f.state is the scratch slot every registered element type gets; for a memo
	// fiber it holds the props of the last render that was actually performed,
	// which is what a later bailout compares against. It deliberately is *not*
	// updated on a bailout — there was no render, so the recorded props are still
	// the ones on screen.
	f.state = next
	reconcileChildren(f, []Node{&Element{Type: m.component, Props: next}})
}

// ShallowEqualProps reports whether two props maps have the same keys and
// equivalent values, comparing each value with [ObjectsEqual] — React's
// `shallowEqual`, and the default comparator for [Memo].
//
// Key sets must match exactly: a key present with a nil value is different from
// an absent key, the same distinction [Props.Has] draws, because for host
// elements present-and-nil and absent render differently. A nil Props and an
// empty Props are equal, since neither carries any key.
//
// # Children participate
//
// [ChildrenKey] is compared like any other key. That is React's behaviour and it
// is the only sound choice: a memo bailout leaves the existing child fibers in
// place, so skipping a render after the children changed would leave the old
// children on screen. Excluding them would make memoization hit more often and
// be wrong when it did.
//
// The consequence is the same caveat React has, and it is worth stating plainly:
// children built inline are a fresh slice of fresh *Element pointers on every
// render, [ObjectsEqual] compares slices by identity, and so **a memoized
// component that takes children almost never bails out**. Memo pays off for leaf
// components with scalar props. To memoize a component that wraps children,
// either hoist the children so the same slice is passed each time, or supply a
// comparator to [MemoWith] that ignores [ChildrenKey] and accept responsibility
// for the staleness that then becomes possible.
//
// # Function-valued props
//
// Functions are compared by code pointer, which has a Go-specific sharp edge
// with no JavaScript equivalent: two closures produced from the *same* func
// literal share a code pointer even when they captured different variables, so
// they compare equal here while in JS they would be two distinct objects. A
// handler prop written inline therefore looks unchanged to this comparison even
// when the values it closed over changed. If a captured value matters, either
// pass it as its own prop (which is better style regardless) or use [MemoWith].
//
// This function deliberately does not call [ObjectsEqual] for function values:
// that helper compares funcs with `Pointer() == Pointer() && Len() == Len()`, and
// reflect.Value.Len panics on a func, so it panics on two identical function
// values. Handler props are far too common for Memo to inherit that. See the
// package notes on the foundation issue.
func ShallowEqualProps(a, b Props) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok {
			return false
		}
		if !shallowEqualPropValue(av, bv) {
			return false
		}
	}
	return true
}

// shallowEqualPropValue is [ObjectsEqual] with function values handled locally.
// Everything else is delegated, so props comparison and dependency-list
// comparison stay in agreement.
func shallowEqualPropValue(a, b any) bool {
	if a != nil && b != nil {
		ta, tb := reflect.TypeOf(a), reflect.TypeOf(b)
		if ta == tb && ta.Kind() == reflect.Func {
			return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
		}
	}
	return ObjectsEqual(a, b)
}

// strictModeTag is the element type [StrictMode] produces. It is an empty
// comparable struct — like fragmentTag — so that every StrictMode element has
// the same type and a StrictMode wrapper rebuilt on each render reuses its
// fiber instead of remounting the subtree beneath it.
type strictModeTag struct{}

// StrictModeDoubleRender controls whether a [StrictMode] subtree is reconciled
// twice per render pass. It defaults to true, matching React's development
// behaviour, and is a package-level variable rather than a build tag because Go
// has no dev/prod split for a library to key off.
//
// Double-rendering is a *diagnostic*: a pure component produces the same tree
// both times and nothing is observable except the wasted work, while an impure
// one — a component that mutates a captured variable, appends to a slice it
// closed over, or derives output from a counter — produces a different tree the
// second time, and the difference shows up immediately in the rendered output
// instead of days later as a mysterious desync.
//
// The trade-off, stated honestly:
//
//   - Every component under the boundary runs twice on every render pass. That
//     is real CPU cost in a program that is not being debugged, which is why
//     setting this to false is the right move for a production binary.
//   - Effects are *not* double-fired by this mechanism on their own account, but
//     an effect with no dependency list re-registers on every render and will
//     therefore fire twice per commit. That is genuinely React's
//     double-invocation check, and it is the intended way to notice an effect
//     that is missing its cleanup.
//   - A component that is impure will actually *behave* differently — the second
//     pass's output is what survives, and fibers produced only by the first pass
//     are unmounted, running their cleanups. Surfacing the bug means letting it
//     happen, not hiding it.
//
// Assign to this variable before rendering, from a single goroutine; it is read
// during a render and is not itself synchronized.
var StrictModeDoubleRender = true

// StrictMode wraps children in a boundary that renders them under extra checks,
// the equivalent of React's <StrictMode>. It produces no output of its own: the
// children appear exactly where the StrictMode element was, so inserting or
// removing one never changes the shape of the rendered tree.
//
// The only check this port implements is the double-render impurity check
// described on [StrictModeDoubleRender]. React's other StrictMode behaviours —
// warning on legacy lifecycle methods, on legacy string refs, on deprecated
// findDOMNode, and the mount/unmount/remount simulation for reusable state —
// have no counterpart here: the port has no legacy APIs to warn about, and
// remount simulation would require tearing down and rebuilding effect state in a
// way the single-pass reconciler cannot express safely.
//
// StrictMode is a boundary, not a global setting: nesting one inside another is
// harmless but nothing is inherited outward, and a subtree outside every
// StrictMode is never double-rendered.
func StrictMode(children ...Node) *Element {
	return &Element{Type: strictModeTag{}, Props: Props{ChildrenKey: children}}
}

func init() {
	RegisterElementRenderer(strictModeTag{}, renderStrictModeFiber)
}

// renderStrictModeFiber reconciles the boundary's children, twice when
// [StrictModeDoubleRender] is set.
//
// The second pass is safe on top of the reconciler precisely because
// reconciliation is idempotent for a pure subtree: the same child nodes match
// the same fibers by type and key, so the second pass reuses every fiber, resets
// each component's hook cursor and re-runs its body against the hook slots that
// already exist. Nothing is unmounted and no state is lost. When the subtree is
// *not* pure the second pass produces different children, the reconciler
// unmounts the ones that vanished, and the discrepancy becomes visible — which
// is the whole point.
func renderStrictModeFiber(f *fiber) {
	children := f.props.Children()
	reconcileChildren(f, children)
	if StrictModeDoubleRender {
		reconcileChildren(f, children)
	}
}

// ProfilerPhase distinguishes a [Profiler] subtree's first render from a
// subsequent one. React passes this as a string ("mount" / "update"); an int
// with named constants is the Go shape, so a switch over it is exhaustive and a
// typo is a compile error.
type ProfilerPhase int

const (
	// ProfilerMount is reported for the render that first created the subtree.
	// It is normally the slowest one, because every fiber and every hook slot
	// beneath the boundary is being allocated for the first time.
	ProfilerMount ProfilerPhase = iota

	// ProfilerUpdate is reported for every later render of the same subtree —
	// that is, every render in which the boundary's fiber was reused rather than
	// created. Changing a [Profiler]'s id remounts the boundary and therefore
	// produces a fresh ProfilerMount.
	ProfilerUpdate
)

// String renders the phase using React's own vocabulary, so log lines and test
// failures read the way the React DevTools profiler does.
func (p ProfilerPhase) String() string {
	switch p {
	case ProfilerMount:
		return "mount"
	case ProfilerUpdate:
		return "update"
	default:
		return fmt.Sprintf("ProfilerPhase(%d)", int(p))
	}
}

// ProfilerEvent is one measurement delivered to a [Profiler]'s callback. React
// passes six positional arguments to onRender; a struct is the Go shape, and it
// carries only the three that mean anything without a DOM commit phase to
// measure.
type ProfilerEvent struct {
	// ID is the id given to [Profiler], echoed back so one callback can serve
	// several boundaries.
	ID string

	// Phase distinguishes the initial render from a re-render.
	Phase ProfilerPhase

	// Duration is the wall-clock time the subtree's reconciliation took,
	// including every descendant component's body and every nested Profiler.
	// It is React's `actualDuration`. There is no separate `baseDuration`: with
	// no commit phase and no host mutation, "how long would it take without
	// memoization" is not something this runtime can observe.
	//
	// The measurement is a plain time.Since over the recursive descent, so it
	// includes anything a component body did — including blocking I/O a
	// component had no business performing. A surprisingly large Duration is
	// usually that, and finding it is what a profiler is for.
	Duration time.Duration
}

// profilerTag is the element type [Profiler] produces. The id is part of the Go
// value — and therefore part of reconciliation identity — while the callback
// lives in props, for two reasons: a struct containing a func would be
// uncomparable and would break sameType's `==` path, and a callback rebuilt as a
// closure on every render must not force a remount. Keeping the id in the type
// means changing it deliberately does remount the boundary, which is the right
// semantics — a different id is a different thing being measured.
type profilerTag struct{ id string }

// profilerCallbackKey is the reserved props key holding a [Profiler]'s callback.
// It is namespaced so it can never collide with a user prop.
const profilerCallbackKey = "react:profiler.onRender"

// Profiler measures how long its subtree takes to render and reports each
// measurement to onRender, mirroring React's <Profiler id onRender>. Like
// [StrictMode] it is transparent: the children render exactly where the Profiler
// element sat, so wrapping a subtree in one never changes the output.
//
// onRender is called synchronously at the end of the subtree's reconciliation,
// while the render pass is still in progress. It must therefore be cheap and
// must not touch the tree — scheduling a state update from it turns every render
// into another render, which the root's pass limit will eventually catch. Record
// the event and get out. onRender may be nil, which makes the boundary a pure
// no-op wrapper and is useful for leaving a Profiler in place with its callback
// wired up conditionally.
//
// Nested Profilers each report their own subtree, and an outer one's Duration
// includes the inner ones — the same containment React reports.
func Profiler(id string, onRender func(ProfilerEvent), children ...Node) *Element {
	return &Element{
		Type: profilerTag{id: id},
		Props: Props{
			ChildrenKey:         children,
			profilerCallbackKey: onRender,
		},
	}
}

func init() {
	RegisterElementRenderer(profilerTag{}, renderProfilerFiber)
}

// renderProfilerFiber times the subtree's reconciliation and reports it.
//
// The mount/update distinction comes from the fiber's own scratch state rather
// than from any flag the runtime keeps, because "mount" here means precisely
// "this fiber had not rendered before" — and the fiber is the only thing that
// knows. A boundary whose id changed is a different element type, gets a new
// fiber, and correctly reports a mount.
//
// The callback fires after the timing is taken so that a slow callback cannot
// inflate the number it is being handed.
func renderProfilerFiber(f *fiber) {
	tag, ok := f.typ.(profilerTag)
	if !ok {
		panic(fmt.Sprintf("react: profiler renderer invoked for element type %T", f.typ))
	}

	start := time.Now()
	reconcileChildren(f, f.props.Children())
	elapsed := time.Since(start)

	phase := ProfilerUpdate
	if rendered, _ := f.state.(bool); !rendered {
		phase = ProfilerMount
		f.state = true
	}

	// A nil callback is stored as a typed nil func, so the assertion succeeds and
	// yields a nil value; both the missing-key and explicit-nil cases land here.
	onRender, _ := f.props[profilerCallbackKey].(func(ProfilerEvent))
	if onRender == nil {
		return
	}
	onRender(ProfilerEvent{ID: tag.id, Phase: phase, Duration: elapsed})
}
