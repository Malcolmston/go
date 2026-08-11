package react

import (
	"strconv"
	"strings"
)

// memoBox wraps a memoized value before it is stored in a [hook]'s untyped
// value slot. The wrapper exists because the slot is an `any`: storing a T
// directly and reading it back with a type assertion silently fails whenever T
// is an interface type holding nil, or whenever T is itself `any` and the value
// is nil — the assertion would report ok=false and the hook would recompute a
// value it had already cached. Boxing makes the assertion depend only on T,
// never on the dynamic value, so a memoized nil is cached like anything else.
type memoBox[T any] struct{ v T }

// memoCompute is the shared engine behind [UseMemo] and [UseCallback]. Both
// hooks are the same computation — "recompute only when deps change" — and
// differ only in the slot label they claim, which is what makes a hook-order
// violation report "memo" versus "callback" instead of a single generic name.
//
// kind must be a constant per call site, since [nextHook] uses it to detect
// conditional hook calls.
func memoCompute[T any](kind string, create func() T, deps []any) T {
	h, first := nextHook(kind)

	if !first && !DepsChanged(h.deps, deps) {
		if box, ok := h.value.(memoBox[T]); ok {
			return box.v
		}
	}

	v := create()
	h.value = memoBox[T]{v: v}
	h.deps = deps
	return v
}

// UseMemo returns the value produced by create, recomputing it only when deps
// differs from the previous render's list according to [DepsChanged]. It is the
// Go equivalent of React's useMemo, generic in the memoized type so that the
// result comes back typed rather than as an `any` the caller has to assert.
//
// The three shapes of deps behave as they do in React:
//
//   - nil deps — no dependency list was given, so create runs on every render.
//     Useful only for instrumentation; a nil list defeats the point of the hook.
//   - empty non-nil deps ([]any{}) — nothing can ever change, so create runs
//     exactly once for the life of the component instance.
//   - a non-empty list — create runs on the first render and again whenever any
//     entry stops being [ObjectsEqual] to the entry recorded last time.
//
// Unlike React, this is a real guarantee rather than a hint. React explicitly
// reserves the right to throw a memoized value away to reclaim memory and
// recompute it later, so useMemo may never be relied on for correctness. This
// port never discards a memo: the value lives in the fiber's hook slot and is
// released only when the component unmounts. Code may therefore depend on
// UseMemo for referential identity — memoizing a slice or a struct pointer that
// is then used as another hook's dependency is sound here.
//
// create must still be a pure computation. It runs during render, so a side
// effect in it fires at a moment the runtime makes no promises about; use
// UseEffect for that.
//
// See [UseCallback] for the function-valued special case and [UseRef] for
// values that must survive renders without being recomputed at all.
func UseMemo[T any](create func() T, deps []any) T {
	return memoCompute("memo", create, deps)
}

// UseCallback returns fn on the render that first sees a given deps list, and
// keeps returning that exact same function value on every later render until
// deps changes. It is React's useCallback, and it exists for the same reason:
// a func literal is a fresh value on every render, so passing one down as a
// prop defeats any memoization downstream that compares props by identity.
//
// The implementation really is [UseMemo] with a create function that returns
// fn — the semantics are identical, and the only reason UseCallback is not
// literally written as UseMemo(func() F { return fn }, deps) is that it claims a
// hook slot labelled "callback" instead of "memo", so a hook-order diagnostic
// names the hook the caller actually wrote. There is no behavioral difference
// beyond that label.
//
// F is unconstrained rather than restricted to a func type: Go cannot express
// "any function type" as a constraint, so a non-function F compiles and behaves
// exactly like UseMemo over a constant. The name documents the intent.
//
// The retained function is the closure captured on the render that last
// recomputed, so it sees that render's variables — the classic stale-closure
// hazard, unchanged from React. Every value the callback reads and that can
// change must appear in deps.
func UseCallback[F any](fn F, deps []any) F {
	return memoCompute("callback", func() F { return fn }, deps)
}

// UseDebugValue attaches a human-readable label to the component instance. In
// React this feeds React DevTools, which displays the label next to a custom
// hook's name in the inspector; there is no DevTools here, and no browser to
// host one, so the port does the only useful thing available: it records the
// label on the hook slot, where it survives until the next render replaces it or
// the component unmounts.
//
// The read path is [DebugValues], which collects every label in a [Root] in
// tree order. That makes the hook genuinely useful in tests — a custom hook can
// publish its internal state and a test can assert on it without exporting the
// state — and leaves the door open for a future inspector to read the same
// slots.
//
// Unlike React, the label is recorded unconditionally rather than only when a
// devtools hook is installed. Recording a string is cheap; use
// [UseDebugValueFn] when producing the label is not.
func UseDebugValue(label string) {
	h, _ := nextHook("debug")
	h.value = label
}

// UseDebugValueFn is the deferred-formatting form of [UseDebugValue], matching
// React's optional second argument to useDebugValue. format is stored, not
// called: it runs only when [DebugValues] actually reads the slot, so an
// expensive label — serializing a large structure, walking a cache — costs
// nothing on renders nobody inspects.
//
// Because format is invoked later, it must not capture anything it is not
// prepared to see at inspection time. It is called with no render in progress,
// so it must not call hooks.
//
// A nil format records no label and is skipped by [DebugValues].
func UseDebugValueFn(format func() string) {
	h, _ := nextHook("debug")
	h.value = format
}

// DebugValues returns every label published by [UseDebugValue] and
// [UseDebugValueFn] in r, in render order (parents before children, siblings
// left to right). Deferred labels are formatted here, at read time.
//
// This is the port's stand-in for opening the DevTools inspector. It is a
// diagnostic: the order and content of the result track the tree's shape and
// are not part of any stability promise.
//
// A nil Root, or one with no debug labels anywhere, yields a nil slice.
func DebugValues(r *Root) []string {
	if r == nil {
		return nil
	}

	var out []string
	walk(r.tree(), func(f *fiber) {
		for _, h := range f.hooks {
			if h.kind != "debug" {
				continue
			}
			switch v := h.value.(type) {
			case string:
				out = append(out, v)
			case func() string:
				if v != nil {
					out = append(out, v())
				}
			}
		}
	})
	return out
}

// UseId returns a stable identifier for this component instance: the same
// string on every render of that instance, and different from the id of every
// other instance in the same [Root]. It is React's useId, and it exists for the
// same reason — generating an id with a counter or a random source produces a
// different value on the server than on the client, which breaks hydration and
// breaks any test that renders the same tree twice and diffs the output.
//
// # Scheme
//
// An id has the form
//
//	":r" <segment> ("_" <segment>)* ":"
//
// where the leading segments are the component's path from the root of the
// tree — the fiber's index among its siblings at each level, outermost first —
// and the final segment is the index of this UseId call among the component's
// hooks. Each segment is written in base 32, the same radix React uses, purely
// for compactness. The surrounding ":" characters are React's: they make an id
// invalid as a CSS selector, which is deliberate, because these ids are for
// matching markup across renders and not for styling.
//
// So the second child of the first child of the root, calling UseId once, gets
// ":r0_1_0:".
//
// # Determinism and collisions
//
// The id is derived from tree position and hook index only — never from a
// pointer, a counter or the clock — so rendering the same tree into two
// different Roots produces the same ids in the same places. That is the entire
// point of the hook: it is what lets server-generated and client-generated
// markup agree.
//
// Within one Root the scheme cannot collide. Two ids are equal only when their
// segment sequences are equal, which forces both the path and the hook index to
// match, which means they came from the same UseId call on the same fiber.
// Across Roots ids collide by construction, as described above; when two
// independently rendered trees must coexist in one document, prefix the id.
//
// The id is computed on the component's first render and stored in the hook
// slot, so it stays fixed even if the instance is later moved to a different
// position by a keyed reorder. A remount — a change of element type, or a
// keyless insertion that shifts the instance — creates a new fiber and
// therefore a new id, exactly as in React.
func UseId() string {
	h, first := nextHook("id")
	if !first {
		if s, ok := h.value.(string); ok {
			return s
		}
	}

	f := currentFiber()
	// hookIdx has already been advanced past this slot by nextHook, so the
	// index of this call is one less.
	id := memoFormatID(memoFiberPath(f), f.hookIdx-1)
	h.value = id
	return id
}

// memoFiberPath returns f's position in its tree as the list of sibling indices
// from the outermost ancestor down to f. The Root's container fiber is
// synthetic and contributes no segment, so the top-level element is path [0].
//
// Walking parent pointers is safe during render: reconcileChildren links the
// whole sibling list and every parent pointer before it renders any child, so a
// fiber's ancestry is fully built by the time its component body runs.
func memoFiberPath(f *fiber) []int {
	var reversed []int
	for cur := f; cur != nil && cur.parent != nil; cur = cur.parent {
		i := 0
		for c := cur.parent.child; c != nil && c != cur; c = c.sibling {
			i++
		}
		reversed = append(reversed, i)
	}

	path := make([]int, len(reversed))
	for i, v := range reversed {
		path[len(reversed)-1-i] = v
	}
	return path
}

// memoFormatID renders a path and hook index in the ":r0_1_0:" form documented
// on [UseId]. Appending the hook index as a final segment is what keeps two
// UseId calls in one component distinct, and it cannot be confused with a
// deeper path because the two encode different segment sequences.
func memoFormatID(path []int, hookIndex int) string {
	var b strings.Builder
	b.WriteString(":r")
	for _, seg := range path {
		b.WriteString(strconv.FormatInt(int64(seg), 32))
		b.WriteByte('_')
	}
	b.WriteString(strconv.FormatInt(int64(hookIndex), 32))
	b.WriteByte(':')
	return b.String()
}
