package react

import (
	"strings"
	"testing"
)

// elDump renders a fiber subtree as a compact S-expression, the same idea as the
// engine tests' helper but declared locally so this file stays independent of
// theirs. Host elements print as (tag#key child…), text leaves as "value", and
// everything else (fragments, components) is transparent.
func elDump(f *fiber) string {
	var b strings.Builder
	var write func(*fiber)
	write = func(f *fiber) {
		switch t := f.typ.(type) {
		case string:
			b.WriteString("(" + t)
			if f.key != "" {
				b.WriteString("#" + f.key)
			}
			for c := f.child; c != nil; c = c.sibling {
				b.WriteString(" ")
				write(c)
			}
			b.WriteString(")")
		case textTag:
			s, _ := f.props[textValueKey].(string)
			b.WriteString(`"` + s + `"`)
		default:
			_ = t
			first := true
			for c := f.child; c != nil; c = c.sibling {
				if !first {
					b.WriteString(" ")
				}
				first = false
				write(c)
			}
		}
	}
	first := true
	for c := f.child; c != nil; c = c.sibling {
		if !first {
			b.WriteString(" ")
		}
		first = false
		write(c)
	}
	return b.String()
}

// elStringer exercises the fmt.Stringer branch of key formatting.
type elStringer struct{ s string }

func (v elStringer) String() string { return v.s }

func TestCreateElementLiftsReservedProps(t *testing.T) {
	ref := new(int)

	cases := []struct {
		name     string
		props    Props
		children []Node
		wantKey  string
		wantRef  any
		// wantProps is checked key by key; the reserved keys must be absent.
		wantProps map[string]any
	}{
		{
			name:      "nil props still yields a non-nil map",
			props:     nil,
			wantProps: map[string]any{},
		},
		{
			name:      "string key is lifted verbatim",
			props:     Props{KeyProp: "row-1", "id": "x"},
			wantKey:   "row-1",
			wantProps: map[string]any{"id": "x"},
		},
		{
			name:      "int key formats like a text child",
			props:     Props{KeyProp: 7},
			wantKey:   "7",
			wantProps: map[string]any{},
		},
		{
			name:      "float key uses the shortest round-tripping form",
			props:     Props{KeyProp: 1.5},
			wantKey:   "1.5",
			wantProps: map[string]any{},
		},
		{
			name:      "Stringer key",
			props:     Props{KeyProp: elStringer{"s"}},
			wantKey:   "s",
			wantProps: map[string]any{},
		},
		{
			name:      "nil key means unkeyed",
			props:     Props{KeyProp: nil},
			wantKey:   "",
			wantProps: map[string]any{},
		},
		{
			name:      "ref is lifted out of props",
			props:     Props{RefProp: ref, "class": "c"},
			wantRef:   ref,
			wantProps: map[string]any{"class": "c"},
		},
		{
			name:      "key and ref together",
			props:     Props{KeyProp: 2, RefProp: ref},
			wantKey:   "2",
			wantRef:   ref,
			wantProps: map[string]any{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			el := CreateElement("div", c.props, c.children...)

			if el.Props == nil {
				t.Fatal("Props is nil; CreateElement must always produce a map")
			}
			if el.Key != c.wantKey {
				t.Errorf("Key = %q, want %q", el.Key, c.wantKey)
			}
			if el.Ref != c.wantRef {
				t.Errorf("Ref = %v, want %v", el.Ref, c.wantRef)
			}
			if el.Props.Has(KeyProp) || el.Props.Has(RefProp) {
				t.Errorf("reserved props leaked into Props: %v", el.Props)
			}
			for k, want := range c.wantProps {
				if got := el.Props[k]; got != want {
					t.Errorf("Props[%q] = %v, want %v", k, got, want)
				}
			}
		})
	}
}

func TestCreateElementDoesNotMutateCallerProps(t *testing.T) {
	props := Props{KeyProp: 1, RefProp: "r", "id": "a", ChildrenKey: "old"}

	el := CreateElement("div", props, "new")

	if len(props) != 4 {
		t.Fatalf("caller props were resized: %v", props)
	}
	for _, k := range []string{KeyProp, RefProp, "id", ChildrenKey} {
		if !props.Has(k) {
			t.Errorf("caller props lost key %q: %v", k, props)
		}
	}
	if got := props[ChildrenKey]; got != "old" {
		t.Errorf("caller children mutated to %v, want %q", got, "old")
	}
	// And the element must be independent of the caller's map afterwards.
	el.Props["id"] = "changed"
	if props["id"] != "a" {
		t.Errorf("writing to the element's props reached the caller's map: %v", props)
	}
}

func TestCreateElementChildrenPrecedence(t *testing.T) {
	cases := []struct {
		name     string
		props    Props
		children []Node
		want     string
	}{
		{
			name:  "props children are used when no variadic children",
			props: Props{ChildrenKey: []Node{"a", "b"}},
			want:  "a,b",
		},
		{
			name:     "variadic children replace props children",
			props:    Props{ChildrenKey: []Node{"a", "b"}},
			children: []Node{"z"},
			want:     "z",
		},
		{
			name:     "a single explicit nil child still counts as supplied",
			props:    Props{ChildrenKey: []Node{"a"}},
			children: []Node{nil},
			want:     "",
		},
		{
			name:     "nested slices are flattened by the accessor",
			children: []Node{[]Node{"a", nil, "b"}, false, "c"},
			want:     "a,b,c",
		},
		{
			name: "no children at all",
			want: "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			el := CreateElement("div", c.props, c.children...)
			kids := el.Props.Children()
			parts := make([]string, 0, len(kids))
			for _, k := range kids {
				parts = append(parts, textOf(k))
			}
			if got := strings.Join(parts, ","); got != c.want {
				t.Errorf("children = %q, want %q", got, c.want)
			}
		})
	}
}

func TestHAndFragBuildRenderableTrees(t *testing.T) {
	tree := H("ul", Props{"class": "menu"},
		H("li", Props{KeyProp: 1}, "one"),
		H("li", Props{KeyProp: 2}, "two"),
		Frag(H("li", Props{KeyProp: 3}, "three"), nil, false),
	)

	if tag, ok := HostTag(tree); !ok || tag != "ul" {
		t.Fatalf("HostTag = %q, %v; want \"ul\", true", tag, ok)
	}
	if got := tree.Props.String("class"); got != "menu" {
		t.Errorf("class = %q, want %q", got, "menu")
	}

	root := NewRoot(tree)
	want := `(ul (li#1 "one") (li#2 "two") (li#3 "three"))`
	if got := elDump(root.tree()); got != want {
		t.Errorf("tree = %s, want %s", got, want)
	}
}

func TestFragIsAFragmentElement(t *testing.T) {
	f := Frag("a")
	if !IsFragment(f) {
		t.Errorf("Frag did not produce a fragment: %#v", f.Type)
	}
	if f.Props == nil {
		t.Error("Frag produced nil props")
	}
}

func TestCloneElement(t *testing.T) {
	refA, refB := new(int), new(int)
	orig := CreateElement("div", Props{KeyProp: "k", RefProp: refA, "id": "a", "class": "c"}, "old")

	t.Run("merges props and keeps key, ref and children", func(t *testing.T) {
		got := CloneElement(orig, Props{"id": "b", "title": "t"})

		if got.Key != "k" {
			t.Errorf("Key = %q, want %q", got.Key, "k")
		}
		if got.Ref != refA {
			t.Errorf("Ref = %v, want the original ref", got.Ref)
		}
		if got.Props.String("id") != "b" {
			t.Errorf("id = %q, want overridden %q", got.Props.String("id"), "b")
		}
		if got.Props.String("class") != "c" {
			t.Errorf("class = %q, want inherited %q", got.Props.String("class"), "c")
		}
		if got.Props.String("title") != "t" {
			t.Errorf("title = %q, want added %q", got.Props.String("title"), "t")
		}
		if kids := got.Props.Children(); len(kids) != 1 || kids[0] != "old" {
			t.Errorf("children = %v, want the original [old]", kids)
		}
	})

	t.Run("key and ref overrides win", func(t *testing.T) {
		got := CloneElement(orig, Props{KeyProp: 9, RefProp: refB})
		if got.Key != "9" {
			t.Errorf("Key = %q, want %q", got.Key, "9")
		}
		if got.Ref != refB {
			t.Errorf("Ref = %v, want the overriding ref", got.Ref)
		}
		if got.Props.Has(KeyProp) || got.Props.Has(RefProp) {
			t.Errorf("reserved props leaked into the clone: %v", got.Props)
		}
	})

	t.Run("variadic children replace the original's", func(t *testing.T) {
		got := CloneElement(orig, nil, "new", "er")
		kids := got.Props.Children()
		if len(kids) != 2 || kids[0] != "new" || kids[1] != "er" {
			t.Errorf("children = %v, want [new er]", kids)
		}
	})

	t.Run("the original is untouched", func(t *testing.T) {
		if orig.Props.String("id") != "a" || orig.Props.Has("title") {
			t.Errorf("original props were mutated: %v", orig.Props)
		}
		if kids := orig.Props.Children(); len(kids) != 1 || kids[0] != "old" {
			t.Errorf("original children were mutated: %v", kids)
		}
	})

	t.Run("nil element panics", func(t *testing.T) {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected a panic cloning a nil element")
			}
			if !strings.Contains(textOf(r), "nil element") {
				t.Errorf("panic = %v, want a nil-element diagnostic", r)
			}
		}()
		CloneElement(nil, nil)
	})
}

// TestCloneElementInjectsPropsIntoChildren is the wrapper-component use case
// clone exists for: a parent adds a prop to children it did not build.
func TestCloneElementInjectsPropsIntoChildren(t *testing.T) {
	var wrapper Component = func(p Props) Node {
		out := ChildrenMap(p[ChildrenKey], func(c Node, _ int) Node {
			el, ok := c.(*Element)
			if !ok {
				return c
			}
			return CloneElement(el, Props{"data-wrapped": "yes"})
		})
		return H("div", nil, out)
	}

	child := H("span", nil, "hi")
	root := NewRoot(CreateElement(wrapper, Props{ChildrenKey: []Node{child}}))

	if got, want := elDump(root.tree()), `(div (span#.0 "hi"))`; got != want {
		t.Errorf("tree = %s, want %s", got, want)
	}
	if child.Props.Has("data-wrapped") {
		t.Errorf("cloning mutated the caller's element: %v", child.Props)
	}
}

func TestIsValidElement(t *testing.T) {
	var nilEl *Element
	cases := []struct {
		name string
		node Node
		want bool
	}{
		{"host element", H("div", nil), true},
		{"fragment", Frag(), true},
		{"text element", newTextElement("x", "x"), true},
		{"typed nil element", nilEl, false},
		{"nil", nil, false},
		{"string", "hi", false},
		{"number", 4, false},
		{"slice", []Node{H("div", nil)}, false},
	}
	for _, c := range cases {
		if got := IsValidElement(c.node); got != c.want {
			t.Errorf("%s: IsValidElement = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestCreateElementKeysDriveReconciliation checks that a key lifted from props
// by CreateElement really reaches the reconciler, not just the struct field.
func TestCreateElementKeysDriveReconciliation(t *testing.T) {
	mk := func(ids ...int) Node {
		kids := make([]Node, 0, len(ids))
		for _, id := range ids {
			kids = append(kids, H("li", Props{KeyProp: id}, id))
		}
		return H("ul", nil, kids)
	}

	root := NewRoot(mk(1, 2, 3))
	if got, want := elDump(root.tree()), `(ul (li#1 "1") (li#2 "2") (li#3 "3"))`; got != want {
		t.Fatalf("initial = %s, want %s", got, want)
	}
	root.Render(mk(3, 1))
	if got, want := elDump(root.tree()), `(ul (li#3 "3") (li#1 "1"))`; got != want {
		t.Errorf("after reorder = %s, want %s", got, want)
	}
}
