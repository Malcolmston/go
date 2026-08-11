package react

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// TestCreatePortalElement covers the element CreatePortal builds, before any
// rendering happens: it is an ordinary host element, which is what makes every
// reconciliation guarantee in the package apply to it unchanged.
func TestCreatePortalElement(t *testing.T) {
	el := CreatePortal(H("p", nil, "hi"), "modal")

	if tag, ok := HostTag(el); !ok || tag != PortalTag {
		t.Fatalf("HostTag = (%q, %v), want (%q, true)", tag, ok, PortalTag)
	}
	if id, ok := PortalContainer(el); !ok || id != "modal" {
		t.Errorf("PortalContainer = (%q, %v), want (\"modal\", true)", id, ok)
	}
	if got := len(el.Props.Children()); got != 1 {
		t.Errorf("children = %d, want 1", got)
	}
}

// TestPortalContainerRejects checks the negative cases of both PortalContainer
// forms, since a wrapper component will call them on arbitrary nodes.
func TestPortalContainerRejects(t *testing.T) {
	tests := []struct {
		name string
		el   *Element
	}{
		{"nil element", nil},
		{"host element", H("div", nil)},
		{"fragment", Frag()},
		{"portal tag without container prop", H(PortalTag, nil, "x")},
		{"portal tag with empty container prop", H(PortalTag, Props{PortalContainerProp: ""})},
		{"portal tag with non-string container prop", H(PortalTag, Props{PortalContainerProp: 7})},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if id, ok := PortalContainer(tc.el); ok {
				t.Errorf("PortalContainer = (%q, true), want ok=false", id)
			}
		})
	}
}

// TestCreatePortalEmptyContainerPanics: an unnamed container can never be
// retrieved, so the mistake is caught at construction.
func TestCreatePortalEmptyContainerPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("CreatePortal with an empty container id did not panic")
		}
	}()
	CreatePortal(H("p", nil), "")
}

// TestRenderPortalsToString is the core table: what stays in the main markup and
// what is lifted out, across the shapes a page actually produces.
func TestRenderPortalsToString(t *testing.T) {
	tests := []struct {
		name    string
		node    Node
		main    string
		portals map[string]string
	}{
		{
			name:    "no portal at all",
			node:    H("main", nil, H("p", nil, "body")),
			main:    "<main><p>body</p></main>",
			portals: map[string]string{},
		},
		{
			name: "single portal is removed from main",
			node: H("main", nil,
				H("p", nil, "body"),
				CreatePortal(H("dialog", nil, "modal"), "overlay"),
			),
			main:    "<main><p>body</p></main>",
			portals: map[string]string{"overlay": "<dialog>modal</dialog>"},
		},
		{
			name: "two portals to one container concatenate in render order",
			node: H("main", nil,
				CreatePortal(H("i", nil, "first"), "toasts"),
				H("hr", nil),
				CreatePortal(H("i", nil, "second"), "toasts"),
			),
			main:    "<main><hr/></main>",
			portals: map[string]string{"toasts": "<i>first</i><i>second</i>"},
		},
		{
			name: "portals to different containers",
			node: Frag(
				CreatePortal("a", "one"),
				CreatePortal("b", "two"),
			),
			main:    "",
			portals: map[string]string{"one": "a", "two": "b"},
		},
		{
			name: "nested portal is lifted out of its parent's markup",
			node: H("main", nil,
				CreatePortal(H("div", nil,
					"outer",
					CreatePortal(H("span", nil, "inner"), "deep"),
				), "shallow"),
			),
			main: "<main></main>",
			portals: map[string]string{
				"shallow": "<div>outer</div>",
				"deep":    "<span>inner</span>",
			},
		},
		{
			name:    "empty portal still registers its container",
			node:    H("main", nil, CreatePortal(nil, "empty")),
			main:    "<main></main>",
			portals: map[string]string{"empty": ""},
		},
		{
			name: "container id needing escaping round-trips",
			node: CreatePortal("x", `a&b"<c>`),
			main: "",
			portals: map[string]string{
				`a&b"<c>`: "x",
			},
		},
		{
			name:    "portal content keeps its own escaping",
			node:    CreatePortal(H("p", nil, "5 < 6 & \"quoted\""), "c"),
			main:    "",
			portals: map[string]string{"c": "<p>5 &lt; 6 &amp; &quot;quoted&quot;</p>"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			main, portals, err := RenderPortalsToString(tc.node)
			if err != nil {
				t.Fatalf("RenderPortalsToString: %v", err)
			}
			if main != tc.main {
				t.Errorf("main = %q, want %q", main, tc.main)
			}
			if !reflect.DeepEqual(portals, tc.portals) {
				t.Errorf("portals = %#v, want %#v", portals, tc.portals)
			}
		})
	}
}

// TestRenderToStringEmitsPortalWrapper documents the seam: without the split,
// the port's own serializer emits the portal as a custom element in place. The
// assertion exists so that a change to that behaviour is a deliberate one.
func TestRenderToStringEmitsPortalWrapper(t *testing.T) {
	markup := MustRenderToString(H("main", nil, CreatePortal(H("p", nil, "hi"), "modal")))

	want := `<main><` + PortalTag + ` ` + PortalContainerProp + `="modal"><p>hi</p></` + PortalTag + `></main>`
	if markup != want {
		t.Errorf("RenderToString = %q, want %q", markup, want)
	}
}

// TestRenderPortalsToStringErrors covers the two error sources: a bad tree, and
// the reserved tag used as an ordinary host element.
func TestRenderPortalsToStringErrors(t *testing.T) {
	tests := []struct {
		name    string
		node    Node
		wantSub string
	}{
		{
			name:    "reserved tag without a container id",
			node:    H("main", nil, H(PortalTag, nil, "orphan")),
			wantSub: PortalContainerProp,
		},
		{
			name:    "render error propagates",
			node:    H("br", nil, "void elements take no children"),
			wantSub: "void element",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			main, portals, err := RenderPortalsToString(tc.node)
			if err == nil {
				t.Fatalf("want an error, got main=%q portals=%v", main, portals)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error = %q, want it to mention %q", err, tc.wantSub)
			}
		})
	}
}

// portalThemeContext is package-level because a context created inside a
// component allocates a fresh identity every render, as CreateContext documents.
var portalThemeContext = CreateContext("light")

// TestPortalKeepsStateAndContextAcrossRerender is the test the whole design is
// for. A portal that lost its state or its providers when the tree re-rendered
// would be a re-mount with extra steps, and the named-slot output split would not
// be worth having.
func TestPortalKeepsStateAndContextAcrossRerender(t *testing.T) {
	var (
		setCount SetStateFunc[int]
		renders  int
	)

	// Declared once, so its code pointer is stable and reconciliation reuses the
	// fiber rather than remounting.
	var modal Component = func(p Props) Node {
		count, set := UseState(0)
		setCount = set
		renders++
		theme := UseContext(portalThemeContext)
		return H("dialog", nil, theme+":"+strconv.Itoa(count))
	}

	var app Component = func(p Props) Node {
		return portalThemeContext.Provider("dark",
			H("main", nil,
				H("h1", nil, p.String("title")),
				CreatePortal(CreateElement(modal, nil), "overlay"),
			),
		)
	}

	root := NewRoot(CreateElement(app, Props{"title": "one"}))
	defer root.Unmount()

	contents := PortalContents(root)["overlay"]
	if len(contents) != 1 {
		t.Fatalf("PortalContents[overlay] = %d instances, want 1", len(contents))
	}
	if got := contents[0].TextContent(); got != "dark:0" {
		t.Fatalf("initial portal text = %q, want %q", got, "dark:0")
	}

	// State set from outside a render must survive into the portal's output.
	Act(func() { setCount(5) })
	if got := PortalContents(root)["overlay"][0].TextContent(); got != "dark:5" {
		t.Fatalf("portal text after update = %q, want %q", got, "dark:5")
	}

	// A re-render of the whole root with different props must not remount the
	// portal's subtree: same fiber, same hooks, same state, same providers.
	before := renders
	root.Render(CreateElement(app, Props{"title": "two"}))
	if renders <= before {
		t.Fatalf("the portal's component did not re-render (%d passes)", renders-before)
	}
	if got := PortalContents(root)["overlay"][0].TextContent(); got != "dark:5" {
		t.Errorf("portal text after re-render = %q, want %q — state was lost", got, "dark:5")
	}

	// And the surrounding tree really did change, so the assertion above is not
	// passing because nothing happened.
	if got := root.Tree().FindByTag("h1").TextContent(); got != "two" {
		t.Errorf("h1 = %q, want %q", got, "two")
	}
}

// TestPortalContentsShape covers the map PortalContents returns: keyed by
// container, holding the wrapper's children rather than the wrapper, empty and
// non-nil for the degenerate inputs.
func TestPortalContentsShape(t *testing.T) {
	t.Run("nil root", func(t *testing.T) {
		if got := PortalContents(nil); got == nil || len(got) != 0 {
			t.Errorf("PortalContents(nil) = %v, want an empty non-nil map", got)
		}
	})

	t.Run("unmounted root", func(t *testing.T) {
		root := NewRoot(CreatePortal(H("p", nil, "x"), "c"))
		root.Unmount()
		if got := PortalContents(root); len(got) != 0 {
			t.Errorf("PortalContents after Unmount = %v, want empty", got)
		}
	})

	t.Run("children not the wrapper", func(t *testing.T) {
		root := NewRoot(H("main", nil,
			CreatePortal(Frag(H("i", nil, "a"), H("b", nil, "c")), "slot"),
		))
		defer root.Unmount()

		got := PortalContents(root)["slot"]
		// The single child is the fragment the portal was given; the wrapper
		// itself must not appear.
		for _, inst := range got {
			if _, isPortal := inst.PortalContainer(); isPortal {
				t.Errorf("PortalContents included the wrapper itself: %s", inst)
			}
		}
		text := ""
		for _, inst := range got {
			text += inst.TextContent()
		}
		if text != "ac" {
			t.Errorf("portal text = %q, want %q", text, "ac")
		}
	})

	t.Run("two portals share a container", func(t *testing.T) {
		root := NewRoot(Frag(
			CreatePortal(H("i", nil, "one"), "shared"),
			CreatePortal(H("i", nil, "two"), "shared"),
		))
		defer root.Unmount()

		if got := len(PortalContents(root)["shared"]); got != 2 {
			t.Errorf("shared container holds %d instances, want 2", got)
		}
	})
}

// TestInstancePortalContainer checks the Instance-side accessor against nodes
// that are not portals, since it is written to be chain-safe.
func TestInstancePortalContainer(t *testing.T) {
	root := NewRoot(H("main", nil, CreatePortal(H("p", nil, "x"), "c")))
	defer root.Unmount()

	if id, ok := root.Tree().FindByTag(PortalTag).PortalContainer(); !ok || id != "c" {
		t.Errorf("portal instance = (%q, %v), want (\"c\", true)", id, ok)
	}
	if _, ok := root.Tree().FindByTag("main").PortalContainer(); ok {
		t.Error("a <main> reported itself as a portal")
	}
	if _, ok := (Instance{}).PortalContainer(); ok {
		t.Error("the zero Instance reported itself as a portal")
	}
}

// TestPortalTagIsNotMistakenForAPrefix guards the open-tag scanner: a custom
// element whose name merely starts with the reserved tag must be left alone.
func TestPortalTagIsNotMistakenForAPrefix(t *testing.T) {
	main, portals, err := RenderPortalsToString(H(PortalTag+"-host", nil, "not a portal"))
	if err != nil {
		t.Fatalf("RenderPortalsToString: %v", err)
	}
	if len(portals) != 0 {
		t.Errorf("portals = %v, want none", portals)
	}
	if want := "<" + PortalTag + "-host>not a portal</" + PortalTag + "-host>"; main != want {
		t.Errorf("main = %q, want %q", main, want)
	}
}
