package react

import (
	"strings"
	"testing"
)

// chDesc renders a node slice as a short, comparable string: elements as
// tag#key (key omitted when empty), text leaves quoted, everything else through
// the runtime's own text formatting. It keeps the table assertions below to one
// string comparison each.
func chDesc(nodes []Node) string {
	parts := make([]string, 0, len(nodes))
	for _, n := range nodes {
		switch v := n.(type) {
		case nil:
			parts = append(parts, "<nil>")
		case *Element:
			if v == nil {
				parts = append(parts, "<nil>")
				continue
			}
			tag, ok := HostTag(v)
			if !ok {
				tag = "?"
			}
			if v.Key != "" {
				tag += "#" + v.Key
			}
			parts = append(parts, tag)
		case string:
			parts = append(parts, `"`+v+`"`)
		default:
			parts = append(parts, textOf(v))
		}
	}
	return strings.Join(parts, " ")
}

// chFixture is the children value shared by several tables: a mixed list with a
// keyed element, a keyless element, a text leaf, a nested slice and values that
// must be dropped.
func chFixture() Node {
	return []Node{
		H("a", Props{KeyProp: "first"}),
		nil,
		H("b", nil),
		"text",
		false,
		[]Node{H("c", nil), true, H("d", Props{KeyProp: "last"})},
	}
}

func TestChildrenCount(t *testing.T) {
	cases := []struct {
		name     string
		children Node
		want     int
	}{
		{"nil children", nil, 0},
		{"a bare false", false, 0},
		{"a single element", H("div", nil), 1},
		{"a single string", "hi", 1},
		{"a number", 0, 1},
		{"empty slice", []Node{}, 0},
		{"slice of only nils and bools", []Node{nil, false, true, nil}, 0},
		{"nested slices are flattened, not counted as one", []Node{[]Node{"a", "b"}, "c"}, 3},
		{"the mixed fixture", chFixture(), 5},
	}
	for _, c := range cases {
		if got := ChildrenCount(c.children); got != c.want {
			t.Errorf("%s: ChildrenCount = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestChildrenToSliceAssignsGeneratedKeys(t *testing.T) {
	cases := []struct {
		name     string
		children Node
		want     string
	}{
		{"nil yields an empty slice", nil, ""},
		{"keyless elements get positional dot keys",
			[]Node{H("a", nil), H("b", nil)}, "a#.0 b#.1"},
		{"author keys are preserved verbatim",
			[]Node{H("a", Props{KeyProp: "x"}), H("b", Props{KeyProp: 2})}, "a#x b#2"},
		{"text leaves pass through unkeyed",
			[]Node{"hi", 7}, `"hi" 7`},
		{"indexes count the flattened, filtered list",
			chFixture(), `a#first b#.1 "text" c#.3 d#last`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := chDesc(ChildrenToSlice(c.children)); got != c.want {
				t.Errorf("ChildrenToSlice = %q, want %q", got, c.want)
			}
		})
	}
}

func TestChildrenToSliceNeverMutatesInput(t *testing.T) {
	child := H("span", nil, "hi")
	out := ChildrenToSlice([]Node{child})

	if child.Key != "" {
		t.Errorf("input element was re-keyed in place: Key = %q", child.Key)
	}
	got, ok := out[0].(*Element)
	if !ok {
		t.Fatalf("output[0] = %T, want *Element", out[0])
	}
	if got == child {
		t.Error("output reuses the input pointer; re-keying must copy")
	}
	if got.Key != ".0" {
		t.Errorf("output key = %q, want %q", got.Key, ".0")
	}
	if len(got.Props.Children()) != 1 {
		t.Errorf("the copy lost its props: %v", got.Props)
	}
}

func TestChildrenToSliceIsAlwaysNonNil(t *testing.T) {
	if got := ChildrenToSlice(nil); got == nil {
		t.Error("ChildrenToSlice(nil) = nil, want an empty slice")
	}
	if got := ChildrenMap(nil, func(n Node, _ int) Node { return n }); got == nil {
		t.Error("ChildrenMap(nil, …) = nil, want an empty slice")
	}
}

func TestChildrenForEach(t *testing.T) {
	var seen []string
	var idx []int
	ChildrenForEach(chFixture(), func(c Node, i int) {
		seen = append(seen, chDesc([]Node{c}))
		idx = append(idx, i)
	})

	if got, want := strings.Join(seen, " "), `a#first b "text" c d#last`; got != want {
		t.Errorf("visited = %q, want %q", got, want)
	}
	for i, n := range idx {
		if n != i {
			t.Fatalf("indexes = %v, want 0..%d in order", idx, len(idx)-1)
		}
	}

	// A nil callback is a no-op rather than a panic, so an optional callback
	// argument does not need a guard at every call site.
	ChildrenForEach(chFixture(), nil)
	ChildrenForEach(nil, func(Node, int) {})
}

func TestChildrenMapKeyPropagation(t *testing.T) {
	wrap := func(c Node, _ int) Node { return H("li", nil, c) }
	wrapKeyed := func(c Node, _ int) Node { return H("li", Props{KeyProp: "w"}, c) }
	passthrough := func(c Node, _ int) Node { return c }
	toText := func(_ Node, i int) Node { return i }
	drop := func(Node, int) Node { return nil }

	cases := []struct {
		name     string
		children Node
		fn       func(Node, int) Node
		want     string
	}{
		{
			// The point of the whole exercise: wrapping keyed children must not
			// collapse them onto positional identity.
			name:     "wrapping inherits author keys",
			children: []Node{H("a", Props{KeyProp: "x"}), H("b", Props{KeyProp: "y"})},
			fn:       wrap,
			want:     "li#x li#y",
		},
		{
			name:     "wrapping keyless children inherits positional keys",
			children: []Node{H("a", nil), H("b", nil)},
			fn:       wrap,
			want:     "li#.0 li#.1",
		},
		{
			name:     "a mapper's own key is prefixed with the source key",
			children: []Node{H("a", Props{KeyProp: "x"}), H("b", nil)},
			fn:       wrapKeyed,
			want:     "li#x/w li#.1/w",
		},
		{
			name:     "text children give positional keys to their wrappers",
			children: []Node{"one", "two"},
			fn:       wrap,
			want:     "li#.0 li#.1",
		},
		{
			name:     "identity mapping keys the keyless the same way toSlice does",
			children: []Node{H("a", Props{KeyProp: "x"}), H("b", nil)},
			fn:       passthrough,
			want:     "a#x b#.1",
		},
		{
			name:     "non-element results pass through untouched",
			children: []Node{H("a", nil), H("b", nil)},
			fn:       toText,
			want:     "0 1",
		},
		{
			name:     "nil results are kept in place, not dropped",
			children: []Node{H("a", nil), H("b", nil)},
			fn:       drop,
			want:     "<nil> <nil>",
		},
		{
			name:     "nils, bools and nested slices are resolved before mapping",
			children: chFixture(),
			fn:       wrap,
			want:     "li#first li#.1 li#.2 li#.3 li#last",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := chDesc(ChildrenMap(c.children, c.fn)); got != c.want {
				t.Errorf("ChildrenMap = %q, want %q", got, c.want)
			}
		})
	}
}

func TestChildrenMapNeverMutatesInput(t *testing.T) {
	child := H("span", Props{KeyProp: "k"}, "hi")
	out := ChildrenMap([]Node{child}, func(c Node, _ int) Node { return c })

	if got := out[0].(*Element); got == child {
		t.Error("ChildrenMap returned the input pointer; re-keying must copy")
	}
	if child.Key != "k" {
		t.Errorf("input element key changed to %q", child.Key)
	}
}

// TestChildrenMapPreservesStateAcrossReorder is the end-to-end proof that the
// key scheme works: a keyed list is wrapped by a parent using ChildrenMap, then
// reordered. Each item's hook state must travel with its key.
func TestChildrenMapPreservesStateAcrossReorder(t *testing.T) {
	var item Component = func(p Props) Node {
		h, first := nextHook("state")
		if first {
			// Seeded once, at mount: if the fiber is remounted the seed changes
			// with it, which is exactly what a broken key scheme would show.
			h.value = p.String("id")
		}
		return H("i", nil, h.value.(string))
	}

	var list Component = func(p Props) Node {
		return H("ul", nil, ChildrenMap(p[ChildrenKey], func(c Node, _ int) Node {
			return H("li", nil, c)
		}))
	}

	mk := func(ids ...string) Node {
		kids := make([]Node, 0, len(ids))
		for _, id := range ids {
			kids = append(kids, CreateElement(item, Props{KeyProp: id, "id": id}))
		}
		return CreateElement(list, Props{ChildrenKey: kids})
	}

	root := NewRoot(mk("a", "b", "c"))
	want := `(ul (li#a (i "a")) (li#b (i "b")) (li#c (i "c")))`
	if got := elDump(root.tree()); got != want {
		t.Fatalf("initial = %s, want %s", got, want)
	}

	// After the reorder the rendered text still matches the key, which can only
	// happen if each item kept its own fiber.
	root.Render(mk("c", "a", "b"))
	want = `(ul (li#c (i "c")) (li#a (i "a")) (li#b (i "b")))`
	if got := elDump(root.tree()); got != want {
		t.Errorf("after reorder = %s, want %s", got, want)
	}
}

func TestChildrenOnly(t *testing.T) {
	el := H("div", nil)

	cases := []struct {
		name     string
		children Node
		want     *Element
		wantErr  string
	}{
		{"a single element", el, el, ""},
		{"a single element in a slice", []Node{el}, el, ""},
		{"a single element behind nils and bools", []Node{nil, el, false}, el, ""},
		{"a nested single element", []Node{[]Node{el}}, el, ""},
		{"no children", nil, nil, "exactly one child, got 0"},
		{"only dropped values", []Node{nil, false, true}, nil, "exactly one child, got 0"},
		{"two children", []Node{el, el}, nil, "exactly one child, got 2"},
		{"a lone string", "hi", nil, "to be an element, got string"},
		{"a lone number", 3, nil, "to be an element, got int"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ChildrenOnly(c.children)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != c.want {
					t.Errorf("element = %v, want the single child", got)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an error, got element %v", got)
			}
			if got != nil {
				t.Errorf("element = %v, want nil alongside an error", got)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("error = %q, want it to mention %q", err, c.wantErr)
			}
		})
	}
}

func TestChildrenSourceKeyScheme(t *testing.T) {
	// The generated-key namespace is documented as "." + index; the prefix is
	// what keeps it from colliding with an author key.
	cases := []struct {
		name  string
		child Node
		i     int
		want  string
	}{
		{"keyed element keeps its key", H("a", Props{KeyProp: "x"}), 3, "x"},
		{"keyless element is positional", H("a", nil), 3, ".3"},
		{"text leaf is positional", "hi", 0, ".0"},
		{"nil is positional", nil, 1, ".1"},
	}
	for _, c := range cases {
		if got := childrenSourceKey(c.child, c.i); got != c.want {
			t.Errorf("%s: childrenSourceKey = %q, want %q", c.name, got, c.want)
		}
	}
	if !strings.HasPrefix(childrenSourceKey(H("a", nil), 0), childrenKeyPrefix) {
		t.Errorf("generated keys must carry the %q prefix", childrenKeyPrefix)
	}
}
