package react

import (
	"strings"
	"sync/atomic"
	"testing"
)

// treeTestHost builds a host element directly, so the tree-inspection tests do
// not depend on the element-construction layer. props may be nil.
func treeTestHost(tag, key string, props Props, children ...Node) *Element {
	p := Props{}
	for k, v := range props {
		p[k] = v
	}
	p[ChildrenKey] = children
	return &Element{Type: tag, Key: key, Props: p}
}

// treeTestApp is the fixture most tests below query: nested host elements, keys,
// props of two different types, and text at the leaves.
func treeTestApp() *Element {
	return treeTestHost("div", "", Props{"id": "app"},
		treeTestHost("h1", "", nil, "Title"),
		treeTestHost("ul", "list", Props{"role": "list"},
			treeTestHost("li", "a", Props{"value": 1}, "one"),
			treeTestHost("li", "b", Props{"value": 2}, "two"),
		),
	)
}

func TestRootTreeStringRendersSubtree(t *testing.T) {
	var comp Component = func(Props) Node {
		return treeTestHost("i", "", nil, "x")
	}

	cases := []struct {
		name string
		node Node
		want string
	}{
		{
			name: "host tree with props keys and text",
			node: treeTestApp(),
			want: `(div id="app" (h1 "Title") (ul#list role="list" (li#a value=1 "one") (li#b value=2 "two")))`,
		},
		{"bare text", "hi", `"hi"`},
		{"number becomes text", 42, `"42"`},
		{"fragment", &Element{Type: Fragment, Props: Props{ChildrenKey: []Node{
			treeTestHost("i", "", nil),
		}}}, `(<> (i))`},
		{"empty root", nil, ``},
	}
	for _, tc := range cases {
		root := NewRoot(tc.node)
		if got := root.Tree().String(); got != tc.want {
			t.Errorf("%s: String() = %s, want %s", tc.name, got, tc.want)
		}
	}

	// A component prints under its own Go function name, with its rendered
	// output nested beneath it — the exact name depends on where the literal was
	// written, so only the shape is asserted.
	root := NewRoot(&Element{Type: comp, Props: Props{}})
	got := root.Tree().String()
	if !strings.Contains(got, `(i "x")`) || !strings.HasPrefix(got, "(") {
		t.Errorf("component String() = %s, want a named wrapper around (i \"x\")", got)
	}
}

func TestInstanceFindByTagPropAndPredicate(t *testing.T) {
	root := NewRoot(treeTestApp())
	tree := root.Tree()

	if got, want := tree.FindByTag("h1").TextContent(), "Title"; got != want {
		t.Errorf("FindByTag(h1) text = %q, want %q", got, want)
	}
	if got, want := len(tree.FindAllByTag("li")), 2; got != want {
		t.Errorf("FindAllByTag(li) = %d nodes, want %d", got, want)
	}
	if got, want := tree.FindByProp("value", 2).Key(), "b"; got != want {
		t.Errorf("FindByProp(value,2) key = %q, want %q", got, want)
	}
	if tree.FindByProp("value", 99).Valid() {
		t.Error("FindByProp with no match returned a valid instance")
	}
	if got, want := tree.Find(func(i Instance) bool { return i.Key() == "a" }).TextContent(), "one"; got != want {
		t.Errorf("Find(key==a) text = %q, want %q", got, want)
	}
	if got := tree.FindAll(func(i Instance) bool { _, ok := i.Text(); return ok }); len(got) != 3 {
		t.Errorf("FindAll(text nodes) = %d, want 3", len(got))
	}

	// Find never matches the receiver, so narrowing twice reaches downward
	// rather than standing still.
	list := tree.FindByTag("ul")
	if !list.Valid() {
		t.Fatal("ul not found")
	}
	if got, want := list.FindByTag("li").Key(), "a"; got != want {
		t.Errorf("ul.FindByTag(li) key = %q, want %q", got, want)
	}
	if inner := list.FindByTag("ul"); inner.Valid() {
		t.Error("Find matched the receiver; it must search descendants only")
	}
}

func TestInstanceTextContentConcatenatesSubtree(t *testing.T) {
	root := NewRoot(treeTestHost("p", "", nil,
		"Hello, ", "world", "!",
		treeTestHost("em", "", nil, " (really)"),
	))
	if got, want := root.Tree().TextContent(), "Hello, world! (really)"; got != want {
		t.Errorf("TextContent = %q, want %q", got, want)
	}
	if got, want := root.Tree().FindByTag("em").TextContent(), " (really)"; got != want {
		t.Errorf("em TextContent = %q, want %q", got, want)
	}
}

func TestInstanceChildrenAndParentNavigation(t *testing.T) {
	root := NewRoot(treeTestApp())
	tree := root.Tree()

	top := tree.Children()
	if len(top) != 1 {
		t.Fatalf("root children = %d, want 1", len(top))
	}
	div := top[0]
	if tag, ok := div.Tag(); !ok || tag != "div" {
		t.Fatalf("top child tag = %q (host=%v), want div", tag, ok)
	}
	if got, want := len(div.Children()), 2; got != want {
		t.Errorf("div children = %d, want %d", got, want)
	}

	li := tree.FindByProp("value", 1)
	if got, want := li.Parent().Key(), "list"; got != want {
		t.Errorf("li parent key = %q, want %q", got, want)
	}
	if got, want := li.Parent().Parent().Type(), any("div"); got != want {
		t.Errorf("li grandparent type = %v, want %v", got, want)
	}
	if tree.Parent().Valid() {
		t.Error("the root instance must have no parent")
	}

	// Component and fragment nodes are part of the fiber tree, so navigation
	// passes through them rather than around them.
	var comp Component = func(Props) Node { return treeTestHost("span", "", nil, "deep") }
	root2 := NewRoot(treeTestHost("div", "", nil, &Element{Type: comp, Props: Props{}}))
	span := root2.Tree().FindByTag("span")
	if span.Parent().Type() == any("div") {
		t.Error("span's parent should be the component fiber, not the div")
	}
	if got, want := span.Parent().Parent().Type(), any("div"); got != want {
		t.Errorf("span grandparent = %v, want the div", got)
	}
}

func TestInstanceAccessorsAreSafeWhenInvalid(t *testing.T) {
	root := NewRoot(treeTestApp())
	live := root.Tree()
	root.Unmount()

	cases := []struct {
		name string
		i    Instance
	}{
		{"zero value", Instance{}},
		{"nil root", (*Root)(nil).Tree()},
		{"after unmount", live},
		{"missing node", root.Tree().FindByTag("nope")},
	}
	for _, tc := range cases {
		i := tc.i
		if i.Valid() {
			t.Errorf("%s: Valid() = true, want false", tc.name)
		}
		if got := i.Type(); got != nil {
			t.Errorf("%s: Type() = %v, want nil", tc.name, got)
		}
		if tag, ok := i.Tag(); ok || tag != "" {
			t.Errorf("%s: Tag() = %q,%v, want \"\",false", tc.name, tag, ok)
		}
		if txt, ok := i.Text(); ok || txt != "" {
			t.Errorf("%s: Text() = %q,%v, want \"\",false", tc.name, txt, ok)
		}
		if got := i.Key(); got != "" {
			t.Errorf("%s: Key() = %q, want empty", tc.name, got)
		}
		if got := i.Props(); got != nil {
			t.Errorf("%s: Props() = %v, want nil", tc.name, got)
		}
		if got := i.Children(); got != nil {
			t.Errorf("%s: Children() = %v, want nil", tc.name, got)
		}
		if i.Parent().Valid() {
			t.Errorf("%s: Parent() reported valid", tc.name)
		}
		if got := i.TextContent(); got != "" {
			t.Errorf("%s: TextContent() = %q, want empty", tc.name, got)
		}
		if got, want := i.String(), "<invalid>"; got != want {
			t.Errorf("%s: String() = %q, want %q", tc.name, got, want)
		}
		// The whole point: a chain through a missing node must not panic.
		if i.FindByTag("ul").FindByTag("li").FindByProp("value", 1).Valid() {
			t.Errorf("%s: chained lookup found something in an invalid tree", tc.name)
		}
		if got := i.FindAllByTag("li"); got != nil {
			t.Errorf("%s: FindAllByTag = %v, want nil", tc.name, got)
		}
	}
}

func TestInstanceGoesInvalidWhenNodeIsRemoved(t *testing.T) {
	show := func(withItem bool) Node {
		if withItem {
			return treeTestHost("ul", "", nil, treeTestHost("li", "a", nil, "one"))
		}
		return treeTestHost("ul", "", nil)
	}

	root := NewRoot(show(true))
	item := root.Tree().FindByTag("li")
	list := root.Tree().FindByTag("ul")
	if !item.Valid() {
		t.Fatal("li not found")
	}

	root.Render(show(false))

	if item.Valid() {
		t.Error("a removed node's handle must report invalid")
	}
	// The surviving node keeps its identity across the re-render, and the handle
	// is a live view rather than a snapshot.
	if !list.Valid() {
		t.Error("the ul kept its fiber and its handle must stay valid")
	}
	if got := list.TextContent(); got != "" {
		t.Errorf("ul text after removal = %q, want empty", got)
	}
}

func TestInstancePropsAreACopy(t *testing.T) {
	root := NewRoot(treeTestApp())
	li := root.Tree().FindByProp("value", 1)
	if !li.Valid() {
		t.Fatal("li not found")
	}

	props := li.Props()
	if got, want := props["value"], 1; got != want {
		t.Fatalf("props[value] = %v, want %v", got, want)
	}

	props["value"] = 999
	delete(props, ChildrenKey)

	if got := li.Props()["value"]; got != 1 {
		t.Errorf("mutating the returned props reached the tree: value = %v, want 1", got)
	}
	if !root.Tree().FindByProp("value", 1).Valid() {
		t.Error("the node became unfindable after its props copy was mutated")
	}
	if got, want := li.TextContent(), "one"; got != want {
		t.Errorf("children survived the copy's deletion? text = %q, want %q", got, want)
	}
}

func TestActCoalescesUpdatesAndSettlesTheTree(t *testing.T) {
	st := NewStore(0)
	var renders atomic.Int32
	var c Component = func(Props) Node {
		renders.Add(1)
		return treeTestHost("b", "", nil, UseSyncExternalStore(st.Subscribe, st.Get))
	}

	root := NewRoot(&Element{Type: c, Props: Props{}})
	renders.Store(0)

	Act(func() {
		st.Set(1)
		st.Set(2)
		st.Set(3)
		// Updates are held until Act returns, so the tree is still the old one.
		if got, want := root.Tree().TextContent(), "0"; got != want {
			t.Errorf("inside Act, text = %q, want %q (updates must be deferred)", got, want)
		}
	})

	if got := renders.Load(); got != 1 {
		t.Errorf("renders = %d, want 1 (three writes coalesced)", got)
	}
	if got, want := root.Tree().TextContent(), "3"; got != want {
		t.Errorf("after Act, text = %q, want %q", got, want)
	}

	Act(nil) // must be a harmless no-op
}

func TestActSettlesAnEffectTriggeredUpdate(t *testing.T) {
	st := NewStore(1)
	var mutated, renders atomic.Int32

	// This component leaves the commit's layout-phase effect with work to do: it
	// moves the store after its snapshot was read, so settling the tree requires
	// a second render pass that an effect — not the caller — asked for.
	var c Component = func(Props) Node {
		renders.Add(1)
		n := UseSyncExternalStore(st.Subscribe, st.Get)
		if mutated.CompareAndSwap(0, 1) {
			done := make(chan struct{})
			go func() {
				st.Set(2)
				close(done)
			}()
			<-done
		}
		return treeTestHost("b", "", nil, n)
	}

	var root *Root
	Act(func() {
		root = NewRoot(&Element{Type: c, Props: Props{}})
		// A Root created inside Act does not render until Act returns.
		if got := root.Tree().Children(); got != nil {
			t.Errorf("inside Act, root already rendered %v", got)
		}
	})

	if got, want := root.Tree().FindByTag("b").String(), `(b "2")`; got != want {
		t.Errorf("after Act, tree = %s, want %s", got, want)
	}
	if got := renders.Load(); got != 2 {
		t.Errorf("renders = %d, want 2 (the effect-triggered pass must have run)", got)
	}
}
