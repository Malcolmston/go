package react

import (
	"reflect"
	"strings"
	"testing"
)

// TestVersion checks the constant is a plausible React 19 version rather than
// the module's own. The shape assertion is what would catch someone pasting the
// port's VERSION file in here by mistake.
func TestVersion(t *testing.T) {
	if Version != "19.2.8" {
		t.Errorf("Version = %q, want %q", Version, "19.2.8")
	}
	if !strings.HasPrefix(Version, "19.") {
		t.Errorf("Version = %q; this port targets React 19 semantics", Version)
	}
}

// TestForwardRefIsIdentity is the important assertion about ForwardRef: it must
// return the very same function value, because reconciliation compares component
// types by code pointer and a wrapper closure would remount the subtree on every
// render.
func TestForwardRefIsIdentity(t *testing.T) {
	var inner Component = func(p Props) Node { return H("i", nil, p.String("label")) }

	wrapped := ForwardRef(inner)

	if !sameType(wrapped, inner) {
		t.Fatal("ForwardRef returned a different element type; the subtree would remount")
	}
	if reflect.ValueOf(wrapped).Pointer() != reflect.ValueOf(inner).Pointer() {
		t.Error("ForwardRef returned a wrapper rather than the component itself")
	}
}

// TestForwardRefPreservesStateAcrossRerender is the consequence of the identity
// rule, tested through the runtime rather than through reflect.
func TestForwardRefPreservesStateAcrossRerender(t *testing.T) {
	var (
		setN   SetStateFunc[int]
		mounts int
	)
	var inner Component = func(p Props) Node {
		n, s := UseState(0)
		setN = s
		if n == 0 {
			mounts++
		}
		return H("i", nil, p.String("label"))
	}
	wrapped := ForwardRef(inner)

	root := NewRoot(CreateElement(wrapped, Props{"label": "a"}))
	defer root.Unmount()

	Act(func() { setN(1) })
	root.Render(CreateElement(wrapped, Props{"label": "b"}))

	if mounts != 1 {
		t.Errorf("component mounted %d times, want 1 — ForwardRef broke type identity", mounts)
	}
	if got := root.Tree().FindByTag("i").TextContent(); got != "b" {
		t.Errorf("label = %q, want %q", got, "b")
	}
}

// TestForwardRefNilPanics: the wrapper is normally built at package init, where a
// panic still points at the mistake.
func TestForwardRefNilPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("ForwardRef(nil) did not panic")
		}
	}()
	ForwardRef(nil)
}

// TestForwardRefRefIsAnOrdinaryProp documents the React 19 behaviour the no-op
// rests on: a ref reaches a host element through Element.Ref, and a component
// that wants one declares a prop.
func TestForwardRefRefIsAnOrdinaryProp(t *testing.T) {
	box := CreateRef[string]()

	var inner Component = func(p Props) Node {
		if r, ok := p.Get("handle").(*Ref[string]); ok {
			r.Current = "published"
		}
		return H("i", nil)
	}

	root := NewRoot(CreateElement(ForwardRef(inner), Props{"handle": box}))
	defer root.Unmount()

	if box.Current != "published" {
		t.Errorf("ref.Current = %q, want %q", box.Current, "published")
	}
}

// TestCreateRef covers the non-hook ref box.
func TestCreateRef(t *testing.T) {
	r := CreateRef[int]()
	if r == nil {
		t.Fatal("CreateRef returned nil")
	}
	if r.Current != 0 {
		t.Errorf("Current = %d, want the zero value", r.Current)
	}
	if other := CreateRef[int](); other == r {
		t.Error("CreateRef returned a shared box; each call must yield a fresh one")
	}
}

// TestUseFormStateIsUseActionState checks the alias behaves identically, since
// its whole justification is that the signature did not change when React renamed
// the hook.
func TestUseFormStateIsUseActionState(t *testing.T) {
	var (
		submitOld func(FormData)
		submitNew func(FormData)
		oldState  string
		newState  string
	)

	appender := func(prev string, d FormData) string { return prev + d.Get("c") }

	var withOldName Component = func(p Props) Node {
		s, submit, _ := UseFormState(appender, "")
		oldState, submitOld = s, submit
		return H("i", nil, s)
	}
	var withNewName Component = func(p Props) Node {
		s, submit, _ := UseActionState(appender, "")
		newState, submitNew = s, submit
		return H("b", nil, s)
	}

	rootOld := NewRoot(CreateElement(withOldName, nil))
	defer rootOld.Unmount()
	rootNew := NewRoot(CreateElement(withNewName, nil))
	defer rootNew.Unmount()

	for _, c := range []string{"a", "b"} {
		data := FormData{}
		data.Set("c", c)
		Act(func() {
			submitOld(data)
			submitNew(data)
		})
	}

	if oldState != "ab" {
		t.Errorf("UseFormState state = %q, want %q", oldState, "ab")
	}
	if oldState != newState {
		t.Errorf("UseFormState = %q but UseActionState = %q; the alias diverged", oldState, newState)
	}
	if got := rootOld.Tree().FindByTag("i").TextContent(); got != "ab" {
		t.Errorf("rendered text = %q, want %q", got, "ab")
	}
}

// TestIsValidElementType is the table the function exists for: everything
// renderFiber can dispatch on is true, everything else is false.
func TestIsValidElementType(t *testing.T) {
	var component Component = func(p Props) Node { return nil }
	var nilComponent Component

	tests := []struct {
		name string
		typ  any
		want bool
	}{
		// Host strings.
		{"host tag", "div", true},
		{"custom element tag", "my-widget", true},
		{"portal tag", PortalTag, true},
		{"empty tag string", "", false},

		// Components.
		{"component", component, true},
		{"nil component", nilComponent, false},
		{"forwarded component", ForwardRef(component), true},

		// The sentinels.
		{"fragment", Fragment, true},

		// Registered wrapper types.
		{"memo", Memo(component), true},
		{"memo with comparator", MemoWith(component, ShallowEqualProps), true},
		{"nil memo pointer", (*MemoType)(nil), false},
		{"lazy", Lazy(func() (Component, error) { return component, nil }), true},
		{"nil lazy pointer", (*LazyType)(nil), false},
		{"suspense", Suspense(nil).Type, true},
		{"error boundary", ErrorBoundary(func(err any) Node { return nil }).Type, true},
		{"strict mode", StrictMode().Type, true},
		{"profiler", Profiler("p", func(ProfilerEvent) {}).Type, true},
		{"context provider", portalThemeContext.Provider("x").Type, true},

		// Not element types at all.
		{"nil", nil, false},
		{"int", 42, false},
		{"bool", true, false},
		{"unregistered struct", struct{ A int }{}, false},
		{"unregistered pointer", &struct{ A int }{}, false},
		{"an element, not a type", H("div", nil), false},
		{"a plain func of the wrong signature", func() {}, false},
		{"an unnamed func with the right signature", func(Props) Node { return nil }, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsValidElementType(tc.typ); got != tc.want {
				t.Errorf("IsValidElementType(%v) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// TestIsValidElementTypeAgreesWithTheRuntime is the assertion that keeps the
// table honest: a type reported valid must actually render, and one reported
// invalid must not. Anything else makes the predicate a second, drifting copy of
// renderFiber's switch.
func TestIsValidElementTypeAgreesWithTheRuntime(t *testing.T) {
	var component Component = func(p Props) Node { return H("i", nil, "ok") }

	valid := []any{"div", component, Fragment, Memo(component)}
	for _, typ := range valid {
		if !IsValidElementType(typ) {
			t.Fatalf("test setup: %T reported invalid", typ)
		}
		if _, err := RenderToString(CreateElement(typ, nil)); err != nil {
			t.Errorf("IsValidElementType said %T was valid but rendering it failed: %v", typ, err)
		}
	}

	invalid := []any{42, struct{ A int }{}, (*MemoType)(nil)}
	for _, typ := range invalid {
		if IsValidElementType(typ) {
			t.Fatalf("test setup: %T reported valid", typ)
		}
		if _, err := RenderToString(CreateElement(typ, nil)); err == nil {
			t.Errorf("IsValidElementType said %T was invalid but rendering it succeeded", typ)
		}
	}
}

// TestIsValidElementVsIsValidElementType keeps the two neighbouring predicates
// apart: one is about a value in the tree, the other about a value in the type
// position.
func TestIsValidElementVsIsValidElementType(t *testing.T) {
	el := H("div", nil)

	if !IsValidElement(el) || IsValidElementType(el) {
		t.Error("an element is a valid element but not a valid element type")
	}
	if IsValidElement("div") || !IsValidElementType("div") {
		t.Error("a tag string is a valid element type but not a valid element")
	}
}
