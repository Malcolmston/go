package router

import (
	"strings"
	"testing"

	"github.com/malcolmston/react"
)

// The components the rendering tests share. They are package-level vars rather
// than literals inside each test because a react.Component's identity is its
// code pointer: a fresh closure per test would be a different element type and
// would defeat the reconciler's reuse, which is exactly what these tests are
// looking at.
var (
	// layoutComp is a layout route: chrome plus an Outlet for whichever child
	// matched.
	layoutComp react.Component = func(react.Props) react.Node {
		return react.H("div", react.Props{"id": "layout"},
			react.H("h1", nil, "site"),
			Outlet(),
		)
	}

	homeComp react.Component = func(react.Props) react.Node {
		return react.H("p", react.Props{"id": "home"}, "home")
	}

	// userListComp is a layout *and* a page: it renders its own content and
	// nests a child route beneath it.
	userListComp react.Component = func(react.Props) react.Node {
		return react.H("section", react.Props{"id": "users"},
			react.H("h2", nil, "users"),
			Outlet(),
		)
	}

	userDetailComp react.Component = func(react.Props) react.Node {
		return react.H("p", react.Props{"id": "user"}, "user ", UseParams()["id"])
	}

	notFoundComp react.Component = func(react.Props) react.Node {
		return react.H("p", react.Props{"id": "404"}, "no such page: ", UseParams()["*"])
	}
)

// appRoutes is the table the rendering tests render. It is the shape a real
// application has: a root layout, an index, a nested section with its own index
// and a dynamic child, and a catch-all.
var appRoutes = []Route{{
	Path:      "/",
	Component: layoutComp,
	Children: []Route{
		{Index: true, Component: homeComp},
		{Path: "users", Component: userListComp, Children: []Route{
			{Path: ":id", Component: userDetailComp},
		}},
		{Path: "*", Component: notFoundComp},
	},
}}

// renderPath renders the app at a path and returns the HTML, failing the test
// on any render error.
func renderPath(t *testing.T, path string) string {
	t.Helper()
	html, err := react.RenderToString(Router(ParseLocation(path), appRoutes))
	if err != nil {
		t.Fatalf("RenderToString(%q): %v", path, err)
	}
	return html
}

// TestRenderRoutes is the end-to-end assertion: a URL in, HTML out, with the
// layout wrapping whichever page the ranking chose.
func TestRenderRoutes(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "index route renders inside its layout",
			path: "/",
			want: `<div id="layout"><h1>site</h1><p id="home">home</p></div>`,
		},
		{
			name: "nested route renders through two outlets",
			path: "/users/7",
			want: `<div id="layout"><h1>site</h1><section id="users"><h2>users</h2>` +
				`<p id="user">user 7</p></section></div>`,
		},
		{
			name: "a layout with no matched child renders an empty outlet",
			path: "/users",
			want: `<div id="layout"><h1>site</h1><section id="users"><h2>users</h2></section></div>`,
		},
		{
			name: "the splat route catches everything else",
			path: "/nope/deeper",
			want: `<div id="layout"><h1>site</h1><p id="404">no such page: nope/deeper</p></div>`,
		},
		{
			name: "a query string does not affect matching",
			path: "/users/7?tab=posts",
			want: `<div id="layout"><h1>site</h1><section id="users"><h2>users</h2>` +
				`<p id="user">user 7</p></section></div>`,
		},
		{
			name: "a trailing slash does not affect matching",
			path: "/users/7/",
			want: `<div id="layout"><h1>site</h1><section id="users"><h2>users</h2>` +
				`<p id="user">user 7</p></section></div>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := renderPath(t, tt.path); got != tt.want {
				t.Errorf("render(%q) =\n  %s\nwant\n  %s", tt.path, got, tt.want)
			}
		})
	}
}

// TestRenderNoMatch checks that a table with no catch-all renders nothing
// rather than panicking. A 404 is the caller's decision, not the router's.
func TestRenderNoMatch(t *testing.T) {
	html, err := react.RenderToString(Router(ParseLocation("/nope"),
		[]Route{{Path: "/", Component: homeComp}}))
	if err != nil {
		t.Fatal(err)
	}
	if html != "" {
		t.Errorf("unmatched location rendered %q, want nothing", html)
	}
}

// TestRenderPathlessLayout covers the layout route with no path and no
// component: it must contribute a level to the tree structure without
// contributing a level to the URL or an element to the output.
func TestRenderPathlessLayout(t *testing.T) {
	routes := []Route{{
		Component: nil, // no component: renders its matched child directly
		Children: []Route{
			{Path: "/a", Component: homeComp},
		},
	}}

	html, err := react.RenderToString(Router(ParseLocation("/a"), routes))
	if err != nil {
		t.Fatal(err)
	}
	if want := `<p id="home">home</p>`; html != want {
		t.Errorf("render = %q, want %q", html, want)
	}
}

// TestRouterWithChildren covers the second Router form: the caller supplies the
// tree and places the routed content with a Routes element.
func TestRouterWithChildren(t *testing.T) {
	page := Router(ParseLocation("/"), appRoutes,
		react.H("header", nil, "top"),
		react.H("main", nil, Routes(nil)),
		react.H("footer", nil, "bottom"),
	)

	html, err := react.RenderToString(page)
	if err != nil {
		t.Fatal(err)
	}
	want := `<header>top</header><main><div id="layout"><h1>site</h1>` +
		`<p id="home">home</p></div></main><footer>bottom</footer>`
	if html != want {
		t.Errorf("render =\n  %s\nwant\n  %s", html, want)
	}
}

// TestNestedRoutesElement covers descendant routing: a route table rendered
// from inside an already-matched route matches the part of the URL its parent
// did not consume, and inherits the parent's params.
func TestNestedRoutesElement(t *testing.T) {
	var sectionComp react.Component = func(react.Props) react.Node {
		return react.H("section", nil, Routes([]Route{
			{Path: "one", Component: react.Component(func(react.Props) react.Node {
				return react.H("p", nil, "one of ", UseParams()["org"])
			})},
			{Path: "two", Component: react.Component(func(react.Props) react.Node {
				return react.H("p", nil, "two")
			})},
		}))
	}

	routes := []Route{{Path: "/:org", Component: sectionComp, Children: []Route{
		{Path: "*"},
	}}}

	html, err := react.RenderToString(Router(ParseLocation("/acme/one"), routes))
	if err != nil {
		t.Fatal(err)
	}
	if want := `<section><p>one of acme</p></section>`; html != want {
		t.Errorf("render = %q, want %q", html, want)
	}
}

// TestTreeShape asserts on the rendered tree rather than the HTML, which is
// what a test cares about when the question is "did the right components run"
// rather than "what markup came out".
func TestTreeShape(t *testing.T) {
	root := react.NewRoot(Router(ParseLocation("/users/7"), appRoutes))
	defer root.Unmount()

	tree := root.Tree()
	if got := tree.FindByTag("section").FindByTag("p").TextContent(); got != "user 7" {
		t.Errorf("nested page text = %q, want %q", got, "user 7")
	}
	if n := len(tree.FindAllByTag("div")); n != 1 {
		t.Errorf("rendered %d layout divs, want 1", n)
	}
}

// TestRerenderPreservesState checks that routing composes with the runtime's
// reconciliation instead of fighting it: re-rendering the same route with a new
// param keeps the component instance, so its hook state survives.
func TestRerenderPreservesState(t *testing.T) {
	renders := 0
	var counter react.Component = func(react.Props) react.Node {
		n, _ := react.UseState(0)
		renders++
		return react.H("p", nil, UseParams()["id"], "/", n)
	}

	routes := []Route{{Path: "/u/:id", Component: counter}}

	root := react.NewRoot(Router(ParseLocation("/u/1"), routes))
	defer root.Unmount()
	if got := root.Tree().TextContent(); got != "1/0" {
		t.Fatalf("first render = %q, want %q", got, "1/0")
	}

	react.Act(func() { root.Render(Router(ParseLocation("/u/2"), routes)) })
	if got := root.Tree().TextContent(); got != "2/0" {
		t.Errorf("after navigation = %q, want %q", got, "2/0")
	}
	if renders != 2 {
		t.Errorf("component rendered %d times, want 2 (it should be reused, not remounted)", renders)
	}
}

// TestRenderToWriterIntegration is the shape a request handler actually uses,
// including the render-then-read check for a recorded redirect.
func TestRenderToWriterIntegration(t *testing.T) {
	var gate react.Component = func(react.Props) react.Node {
		return Navigate("/login")
	}
	routes := []Route{
		{Path: "/admin", Component: gate},
		{Path: "/login", Component: homeComp},
	}

	page := Router(ParseLocation("/admin"), routes)

	var b strings.Builder
	if err := react.RenderToWriter(&b, page); err != nil {
		t.Fatal(err)
	}
	to, ok := NavigationOf(page).Pending()
	if !ok {
		t.Fatal("no navigation was recorded")
	}
	if to != "/login" {
		t.Errorf("recorded target = %q, want %q", to, "/login")
	}
	if b.String() != "" {
		t.Errorf("redirecting route rendered %q, want nothing", b.String())
	}
}
