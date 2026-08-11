package react

import (
	"fmt"
	"strings"
	"testing"
)

// contextDump renders a fiber subtree as a compact S-expression, the same shape
// the engine tests use, so a context test can assert on the whole rendered tree
// in one comparison. It lives here under its own name because every test file
// in this package shares one namespace.
//
// Providers, consumers, components and fragments are all transparent in the
// output: that transparency is itself part of what these tests assert — a
// provider must contribute no output of its own.
// It returns a string rather than writing into a shared builder — unlike the
// engine's dump — because a transparent fiber that renders nothing must
// contribute no separator at all, and a provider with zero children is exactly
// that case.
func contextDump(f *fiber) string {
	var write func(*fiber) string
	children := func(f *fiber) string {
		var parts []string
		for c := f.child; c != nil; c = c.sibling {
			if s := write(c); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " ")
	}
	write = func(f *fiber) string {
		switch t := f.typ.(type) {
		case string:
			head := "(" + t
			if f.key != "" {
				head += "#" + f.key
			}
			if kids := children(f); kids != "" {
				return head + " " + kids + ")"
			}
			return head + ")"
		case textTag:
			s, _ := f.props[textValueKey].(string)
			return `"` + s + `"`
		default:
			_ = t
			return children(f)
		}
	}
	return children(f)
}

// contextHost builds a host element from raw fields, avoiding any dependency on
// the element-construction layer.
func contextHost(tag string, children ...Node) *Element {
	return &Element{Type: tag, Props: Props{ChildrenKey: children}}
}

// contextComponent wraps a render function as a component element. The
// component value is created per call, which is fine for these tests because
// each one is rendered at a stable position for the life of its root.
func contextComponent(fn func(Props) Node) *Element {
	var c Component = fn
	return &Element{Type: c, Props: Props{}}
}

// contextCounterHook is a minimal state hook built straight on the nextHook
// primitive, so these tests depend on nothing but the foundation.
func contextCounterHook(initial string) (string, func(string)) {
	h, first := nextHook("state")
	f := currentFiber()
	if first {
		h.value = initial
	}
	set := func(v string) {
		h.value = v
		f.scheduleUpdate()
	}
	return h.value.(string), set
}

// contextRecover runs fn and returns the value it panicked with, or nil.
func contextRecover(fn func()) (recovered any) {
	defer func() { recovered = recover() }()
	fn()
	return nil
}

func TestContextProvisionAndDefaults(t *testing.T) {
	// One context, exercised through every arrangement of providers that
	// changes the answer: absent, present, nested, and shadowed by a sibling.
	ctx := CreateContext("default")

	// reader renders the context value as a text child of <b>, so the dump
	// shows exactly what UseContext returned.
	reader := func() *Element {
		return contextComponent(func(Props) Node {
			return contextHost("b", UseContext(ctx))
		})
	}

	cases := []struct {
		name string
		tree func() Node
		want string
	}{
		{
			name: "no provider yields the default",
			tree: func() Node { return contextHost("div", reader()) },
			want: `(div (b "default"))`,
		},
		{
			name: "provider supplies its value",
			tree: func() Node {
				return contextHost("div", ctx.Provider("outer", reader()))
			},
			want: `(div (b "outer"))`,
		},
		{
			name: "provider adds no wrapper of its own",
			tree: func() Node {
				return contextHost("div", ctx.Provider("outer", contextHost("i")))
			},
			want: `(div (i))`,
		},
		{
			name: "nested providers: the nearest one wins",
			tree: func() Node {
				return contextHost("div",
					ctx.Provider("outer",
						reader(),
						ctx.Provider("inner", reader()),
					),
				)
			},
			want: `(div (b "outer") (b "inner"))`,
		},
		{
			name: "a provider does not leak to its siblings",
			tree: func() Node {
				return contextHost("div",
					ctx.Provider("scoped", reader()),
					reader(),
				)
			},
			want: `(div (b "scoped") (b "default"))`,
		},
		{
			name: "three levels deep still finds the nearest",
			tree: func() Node {
				return contextHost("div",
					ctx.Provider("a",
						ctx.Provider("b",
							ctx.Provider("c", contextHost("span", reader())),
						),
					),
				)
			},
			want: `(div (span (b "c")))`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := NewRoot(c.tree())
			defer root.Unmount()
			if got := contextDump(root.tree()); got != c.want {
				t.Errorf("tree = %s, want %s", got, c.want)
			}
		})
	}
}

func TestContextIndependentContextsDoNotInterfere(t *testing.T) {
	// Two contexts with the same value type and the same default: only identity
	// distinguishes them, which is exactly the property under test.
	a := CreateContext("a-default")
	b := CreateContext("a-default")
	n := CreateContext(0)

	// One component reads all three, proving a single fiber can consume several
	// contexts and that each walk finds its own provider.
	reader := contextComponent(func(Props) Node {
		return contextHost("b",
			UseContext(a), "/", UseContext(b), "/", UseContext(n))
	})

	cases := []struct {
		name string
		tree Node
		want string
	}{
		{
			name: "only a is provided",
			tree: a.Provider("A", reader),
			want: `(b "A" "/" "a-default" "/" "0")`,
		},
		{
			name: "b provided inside a",
			tree: a.Provider("A", b.Provider("B", reader)),
			want: `(b "A" "/" "B" "/" "0")`,
		},
		{
			name: "all three, interleaved",
			tree: n.Provider(7, a.Provider("A", b.Provider("B", reader))),
			want: `(b "A" "/" "B" "/" "7")`,
		},
		{
			name: "a provider for b does not answer a lookup for a",
			tree: b.Provider("B", reader),
			want: `(b "a-default" "/" "B" "/" "0")`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := NewRoot(c.tree)
			defer root.Unmount()
			if got := contextDump(root.tree()); got != c.want {
				t.Errorf("tree = %s, want %s", got, c.want)
			}
		})
	}
}

func TestContextValueChangeRerendersReaders(t *testing.T) {
	// A provider whose value comes from state above it. The runtime re-renders
	// from the root, so the new value must reach the reader without any
	// per-context subscription — asserted rather than assumed.
	ctx := CreateContext("default")

	readerRenders := 0
	reader := contextComponent(func(Props) Node {
		readerRenders++
		return contextHost("b", UseContext(ctx))
	})

	var set func(string)
	var app Component = func(Props) Node {
		v, s := contextCounterHook("first")
		set = s
		return ctx.Provider(v, reader)
	}

	root := NewRoot(&Element{Type: app, Props: Props{}})
	defer root.Unmount()

	if got, want := contextDump(root.tree()), `(b "first")`; got != want {
		t.Fatalf("initial tree = %s, want %s", got, want)
	}
	if readerRenders != 1 {
		t.Fatalf("reader renders = %d, want 1", readerRenders)
	}

	set("second")
	if got, want := contextDump(root.tree()), `(b "second")`; got != want {
		t.Errorf("after change tree = %s, want %s", got, want)
	}
	if readerRenders != 2 {
		t.Errorf("reader renders = %d, want 2 (one mount, one update)", readerRenders)
	}

	// An unchanged value must still be delivered: re-rendering with the same
	// value is not allowed to fall back to the default or to a stale read.
	set("second")
	if got, want := contextDump(root.tree()), `(b "second")`; got != want {
		t.Errorf("after unchanged value tree = %s, want %s", got, want)
	}

	// And the reader must keep its own hook identity across all of that: the
	// provider fiber is reused, so nothing below it remounted.
	if readerRenders != 3 {
		t.Errorf("reader renders = %d, want 3", readerRenders)
	}
}

func TestContextProviderValueSurvivesRootRerender(t *testing.T) {
	// Re-rendering the whole root with a structurally identical tree reuses the
	// provider fiber. The value must still be readable afterwards — a provider
	// that lost its props on reuse would silently fall back to the default.
	ctx := CreateContext("default")
	reader := contextComponent(func(Props) Node {
		return contextHost("b", UseContext(ctx))
	})
	tree := func(v string) Node { return contextHost("div", ctx.Provider(v, reader)) }

	root := NewRoot(tree("one"))
	defer root.Unmount()
	if got, want := contextDump(root.tree()), `(div (b "one"))`; got != want {
		t.Fatalf("initial = %s, want %s", got, want)
	}

	root.Render(tree("one"))
	if got, want := contextDump(root.tree()), `(div (b "one"))`; got != want {
		t.Errorf("after identical re-render = %s, want %s", got, want)
	}

	root.Render(tree("two"))
	if got, want := contextDump(root.tree()), `(div (b "two"))`; got != want {
		t.Errorf("after changed re-render = %s, want %s", got, want)
	}
}

func TestContextConsumerMatchesUseContext(t *testing.T) {
	ctx := CreateContext("default")

	// The consumer and the hook reader sit side by side under the same
	// provider; their outputs must agree in every arrangement.
	pair := func() Node {
		return []Node{
			ctx.Consumer(func(v string) Node { return contextHost("i", v) }),
			contextComponent(func(Props) Node { return contextHost("b", UseContext(ctx)) }),
		}
	}

	cases := []struct {
		name string
		tree Node
		want string
	}{
		{
			name: "default with no provider",
			tree: contextHost("div", pair()),
			want: `(div (i "default") (b "default"))`,
		},
		{
			name: "under a provider",
			tree: contextHost("div", ctx.Provider("v", pair())),
			want: `(div (i "v") (b "v"))`,
		},
		{
			name: "under nested providers",
			tree: contextHost("div", ctx.Provider("outer", ctx.Provider("inner", pair()))),
			want: `(div (i "inner") (b "inner"))`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := NewRoot(c.tree)
			defer root.Unmount()
			if got := contextDump(root.tree()); got != c.want {
				t.Errorf("tree = %s, want %s", got, c.want)
			}
		})
	}
}

func TestContextConsumerFollowsValueChanges(t *testing.T) {
	// The render-prop form must also see updates, and the thunk it renders
	// through must be refreshed on every pass rather than captured once.
	ctx := CreateContext(0)
	tree := func(v int) Node {
		return ctx.Provider(v, ctx.Consumer(func(n int) Node {
			return contextHost("b", n*2)
		}))
	}

	root := NewRoot(tree(1))
	defer root.Unmount()
	if got, want := contextDump(root.tree()), `(b "2")`; got != want {
		t.Fatalf("initial = %s, want %s", got, want)
	}

	root.Render(tree(5))
	if got, want := contextDump(root.tree()), `(b "10")`; got != want {
		t.Errorf("after update = %s, want %s", got, want)
	}
}

func TestContextProviderWithNoChildrenRendersNothing(t *testing.T) {
	ctx := CreateContext("v")

	root := NewRoot(contextHost("div", ctx.Provider("v")))
	defer root.Unmount()
	if got, want := contextDump(root.tree()), `(div)`; got != want {
		t.Errorf("tree = %s, want %s", got, want)
	}

	// Going from children to none must tear the subtree down rather than leave
	// stale fibers behind, and must not panic.
	root2 := NewRoot(contextHost("div", ctx.Provider("v", contextHost("i"))))
	defer root2.Unmount()
	root2.Render(contextHost("div", ctx.Provider("v")))
	if got, want := contextDump(root2.tree()), `(div)`; got != want {
		t.Errorf("after emptying = %s, want %s", got, want)
	}
}

func TestContextZeroAndNilValuesAreProvidedNotDefaulted(t *testing.T) {
	// React's rule: the default applies only when there is no provider. A
	// provider supplying the zero value must shadow the default.
	strCtx := CreateContext("fallback")
	numCtx := CreateContext(99)
	anyCtx := CreateContext[any]("fallback")

	tree := contextHost("div",
		strCtx.Provider("",
			numCtx.Provider(0,
				anyCtx.Provider(nil,
					contextComponent(func(Props) Node {
						return contextHost("b",
							fmt.Sprintf("[%v]/%v/%v",
								UseContext(strCtx), UseContext(numCtx), UseContext(anyCtx)))
					}),
				),
			),
		),
	)

	root := NewRoot(tree)
	defer root.Unmount()
	if got, want := contextDump(root.tree()), `(div (b "[]/0/<nil>"))`; got != want {
		t.Errorf("tree = %s, want %s", got, want)
	}
}

func TestContextUseContextOutsideComponentPanics(t *testing.T) {
	ctx := CreateContext("v")

	r := contextRecover(func() { UseContext(ctx) })
	if r == nil {
		t.Fatal("expected a panic when UseContext is called outside a render")
	}
	if !strings.Contains(fmt.Sprint(r), "hook called outside of a component render") {
		t.Errorf("panic = %v, want the runtime's hook diagnostic", r)
	}
}

func TestContextMismatchedValueTypePanics(t *testing.T) {
	// Only reachable by hand-building a provider element with a value of the
	// wrong type. It is a programming error, and the diagnostic must name both
	// the type found and the type expected.
	ctx := CreateContext(0)

	bad := &Element{
		Type: contextProviderTag{},
		Props: Props{
			contextIdentityKey: ctx.id,
			contextValueKey:    any("not an int"),
			ChildrenKey: []Node{
				contextComponent(func(Props) Node { return UseContext(ctx) }),
			},
		},
	}

	r := contextRecover(func() { NewRoot(bad) })
	if r == nil {
		t.Fatal("expected a panic for a mismatched context value type")
	}
	msg := fmt.Sprint(r)
	for _, want := range []string{"context value has Go type", "string", "int"} {
		if !strings.Contains(msg, want) {
			t.Errorf("panic %q does not mention %q", msg, want)
		}
	}
}

func TestContextIdentitiesAreDistinct(t *testing.T) {
	// Contexts created with identical defaults must never share identity; the
	// non-empty contextID struct is what guarantees distinct addresses.
	seen := map[*contextID]bool{}
	for range 100 {
		c := CreateContext(struct{}{})
		if c.id == nil {
			t.Fatal("CreateContext produced a nil identity")
		}
		if seen[c.id] {
			t.Fatalf("duplicate context identity %p", c.id)
		}
		seen[c.id] = true
	}
}

func TestContextConsumerRejectsHandBuiltElement(t *testing.T) {
	// The consumer component is package-level and reads its render function
	// from props; an element built without one must fail loudly rather than
	// render nothing.
	bad := &Element{Type: contextConsumerComponent, Props: Props{}}

	r := contextRecover(func() { NewRoot(bad) })
	if r == nil {
		t.Fatal("expected a panic for a consumer element with no render function")
	}
	if !strings.Contains(fmt.Sprint(r), "missing its render function") {
		t.Errorf("panic = %v, want a missing-render-function diagnostic", r)
	}
}
