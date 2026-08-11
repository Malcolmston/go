package router

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/malcolmston/react"
)

// renderIn renders body at path inside a Router over routes, and returns the
// HTML. It is the harness every hook test uses: hooks need a component to run
// in and a Router above it, and building both by hand in each test would bury
// the assertion.
func renderIn(t *testing.T, path string, routes []Route, body func() react.Node) string {
	t.Helper()
	var comp react.Component = func(react.Props) react.Node { return body() }

	// Static markup, not RenderToString: these tests assert on the markup a
	// routing hook produced, and RenderToString would additionally write the
	// "<!-- -->" hydration separator between adjacent text children (see
	// react/ssr_textsep.go), which says nothing about routing.
	html, err := react.RenderToStaticMarkup(
		Router(ParseLocation(path), routes, react.CreateElement(comp, nil)))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return html
}

// TestHooksRequireARouter is the diagnostic contract: every hook called outside
// a Router must fail loudly and say what to do about it, rather than returning
// a zero Location and sending the caller down the "we are at /" branch.
func TestHooksRequireARouter(t *testing.T) {
	tests := []struct {
		name string
		call func()
	}{
		{"UseLocation", func() { UseLocation() }},
		{"UseParams", func() { UseParams() }},
		{"UseSearchParams", func() { UseSearchParams() }},
		{"UseNavigate", func() { UseNavigate() }},
		{"UseMatch", func() { UseMatch("/x") }},
		{"UseOutlet", func() { UseOutlet() }},
		{"UseMatches", func() { UseMatches() }},
		{"UseResolvedPath", func() { UseResolvedPath("x") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var comp react.Component = func(react.Props) react.Node {
				tt.call()
				return nil
			}

			// The runtime recovers a panic during render and returns it as an
			// error, so the message is inspectable without a recover here.
			_, err := react.RenderToString(react.CreateElement(comp, nil))
			if err == nil {
				t.Fatalf("%s outside a Router did not fail", tt.name)
			}
			msg := err.Error()
			if !strings.Contains(msg, tt.name) {
				t.Errorf("message does not name the hook: %s", msg)
			}
			if !strings.Contains(msg, "outside a Router") {
				t.Errorf("message does not explain the problem: %s", msg)
			}
			if !strings.Contains(msg, "router.Router(") {
				t.Errorf("message does not say how to fix it: %s", msg)
			}
		})
	}
}

// TestUseLocation checks that the whole URL — not just the path — reaches a
// component.
func TestUseLocation(t *testing.T) {
	got := renderIn(t, "/users/7?tab=posts#bio", nil, func() react.Node {
		loc := UseLocation()
		return react.H("p", nil, loc.Path, "|", loc.Search, "|", loc.Hash)
	})
	if want := "<p>/users/7|?tab=posts|#bio</p>"; got != want {
		t.Errorf("render = %q, want %q", got, want)
	}
}

// TestUseSearchParams checks the query parse, and with it the decision to omit
// upstream's setter half.
func TestUseSearchParams(t *testing.T) {
	got := renderIn(t, "/?a=1&b=x%20y&a=2", nil, func() react.Node {
		q := UseSearchParams()
		return react.H("p", nil, strings.Join(q["a"], ","), "|", q.Get("b"))
	})
	if want := "<p>1,2|x y</p>"; got != want {
		t.Errorf("render = %q, want %q", got, want)
	}
}

// TestUseParamsOutsideARoute covers the one place a routing hook returns a zero
// value instead of failing: page chrome inside a Router but outside any matched
// route legitimately has no params.
func TestUseParamsOutsideARoute(t *testing.T) {
	got := renderIn(t, "/anything", nil, func() react.Node {
		p := UseParams()
		if p == nil {
			t.Error("UseParams returned nil; it must always return a usable map")
		}
		return react.H("p", nil, len(p))
	})
	if want := "<p>0</p>"; got != want {
		t.Errorf("render = %q, want %q", got, want)
	}
}

// TestUseParamsInRoute checks accumulation down the chain and the copy: a
// component sees its ancestors' captures as well as its own, and writing to the
// result cannot reach the tree.
func TestUseParamsInRoute(t *testing.T) {
	var leaf react.Component = func(react.Props) react.Node {
		params := UseParams()
		params["injected"] = "no"

		keys := make([]string, 0, len(params))
		for k := range params {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return react.H("p", nil, strings.Join(keys, ","))
	}

	routes := []Route{{Path: "/:org", Children: []Route{
		{Path: "u/:id", Component: leaf},
	}}}

	html, err := react.RenderToString(Router(ParseLocation("/acme/u/7"), routes))
	if err != nil {
		t.Fatal(err)
	}
	if want := "<p>id,injected,org</p>"; html != want {
		t.Errorf("render = %q, want %q", html, want)
	}
}

// TestUseMatch checks the standalone pattern test, which is independent of the
// route table and therefore of where the calling component sits.
func TestUseMatch(t *testing.T) {
	tests := []struct {
		path       string
		pattern    string
		wantOK     bool
		wantParams map[string]string
	}{
		{"/users/7", "/users/:id", true, map[string]string{"id": "7"}},
		{"/users/7", "/users", false, nil},
		{"/users", "/users", true, map[string]string{}},
		{"/a/b/c", "/a/*", true, map[string]string{"*": "b/c"}},
	}

	for _, tt := range tests {
		t.Run(tt.path+" vs "+tt.pattern, func(t *testing.T) {
			var gotParams map[string]string
			var gotOK bool

			renderIn(t, tt.path, nil, func() react.Node {
				gotParams, gotOK = UseMatch(tt.pattern)
				return nil
			})

			if gotOK != tt.wantOK {
				t.Fatalf("ok = %v, want %v", gotOK, tt.wantOK)
			}
			if tt.wantOK && !reflect.DeepEqual(gotParams, tt.wantParams) {
				t.Errorf("params = %v, want %v", gotParams, tt.wantParams)
			}
		})
	}
}

// TestUseOutlet covers the reason the hook exists alongside the element: the
// node can be tested for emptiness before it is placed.
func TestUseOutlet(t *testing.T) {
	var layout react.Component = func(react.Props) react.Node {
		if child := UseOutlet(); child != nil {
			return react.H("div", nil, child)
		}
		return react.H("p", nil, "nothing selected")
	}
	var child react.Component = func(react.Props) react.Node {
		return react.H("span", nil, "child")
	}

	routes := []Route{{Path: "/", Component: layout, Children: []Route{
		{Path: "c", Component: child},
	}}}

	for path, want := range map[string]string{
		"/":  "<p>nothing selected</p>",
		"/c": "<div><span>child</span></div>",
	} {
		html, err := react.RenderToString(Router(ParseLocation(path), routes))
		if err != nil {
			t.Fatal(err)
		}
		if html != want {
			t.Errorf("render(%q) = %q, want %q", path, html, want)
		}
	}
}

// TestUseMatches checks the breadcrumb source: the whole chain, outermost
// first, each level carrying the pathname it owns.
func TestUseMatches(t *testing.T) {
	var leaf react.Component = func(react.Props) react.Node {
		var crumbs []string
		for _, m := range UseMatches() {
			crumbs = append(crumbs, m.Pathname)
		}
		return react.H("p", nil, strings.Join(crumbs, " > "))
	}

	routes := []Route{{Path: "/", Children: []Route{
		{Path: "users", Children: []Route{{Path: ":id", Component: leaf}}},
	}}}

	html, err := react.RenderToString(Router(ParseLocation("/users/7"), routes))
	if err != nil {
		t.Fatal(err)
	}
	if want := "<p>/ &gt; /users &gt; /users/7</p>"; html != want {
		t.Errorf("render = %q, want %q", html, want)
	}
}

// TestUseNavigateRecordsIntent is the navigation contract in full: calling the
// function changes nothing about the render, and the intent is readable
// afterwards through the element the caller already holds.
func TestUseNavigateRecordsIntent(t *testing.T) {
	tests := []struct {
		name        string
		call        func(NavigateFunc)
		wantTo      string
		wantReplace bool
	}{
		{"push", func(n NavigateFunc) { n("/next") }, "/next", false},
		{"replace", func(n NavigateFunc) { n.Replace("/next") }, "/next", true},
		{"last writer wins", func(n NavigateFunc) { n("/a"); n("/b") }, "/b", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var comp react.Component = func(react.Props) react.Node {
				tt.call(UseNavigate())
				return react.H("p", nil, "still rendered")
			}

			page := Router(ParseLocation("/"), nil, react.CreateElement(comp, nil))
			html, err := react.RenderToString(page)
			if err != nil {
				t.Fatal(err)
			}
			if want := "<p>still rendered</p>"; html != want {
				t.Errorf("navigating changed the output: %q, want %q", html, want)
			}

			nav := NavigationOf(page)
			to, ok := nav.Pending()
			if !ok {
				t.Fatal("nothing was recorded")
			}
			if to != tt.wantTo {
				t.Errorf("target = %q, want %q", to, tt.wantTo)
			}
			if nav.IsReplace() != tt.wantReplace {
				t.Errorf("IsReplace = %v, want %v", nav.IsReplace(), tt.wantReplace)
			}
		})
	}
}

// TestNavigationEmpty checks the untouched recorder, which is what a handler
// branches on for the overwhelmingly common "no redirect" case.
func TestNavigationEmpty(t *testing.T) {
	page := Router(ParseLocation("/"), nil)
	if _, err := react.RenderToString(page); err != nil {
		t.Fatal(err)
	}
	if to, ok := NavigationOf(page).Pending(); ok {
		t.Errorf("an idle render recorded a navigation to %q", to)
	}
	if NavigationOf(nil) != nil {
		t.Error("NavigationOf(nil) must be nil")
	}
	if NavigationOf(react.H("div", nil)) != nil {
		t.Error("NavigationOf on a non-Router element must be nil")
	}
}

// TestNavigationReset covers reusing one recorder across renders, which is the
// only way a shared Navigation is safe.
func TestNavigationReset(t *testing.T) {
	nav := NewNavigation()
	nav.Push("/somewhere")
	if _, ok := nav.Pending(); !ok {
		t.Fatal("Push did not record")
	}

	nav.Reset()
	if to, ok := nav.Pending(); ok {
		t.Errorf("Reset left %q behind", to)
	}
	if nav.IsReplace() {
		t.Error("Reset left the replace flag set")
	}
}

// TestRouterWithSharedNavigation covers the constructor that exists for the
// handler which builds a tree in one place and reads the outcome in another.
func TestRouterWithSharedNavigation(t *testing.T) {
	nav := NewNavigation()
	var comp react.Component = func(react.Props) react.Node {
		UseNavigate()("/elsewhere")
		return nil
	}

	if _, err := react.RenderToString(
		RouterWith(nav, ParseLocation("/"), nil, react.CreateElement(comp, nil))); err != nil {
		t.Fatal(err)
	}
	if to, _ := nav.Pending(); to != "/elsewhere" {
		t.Errorf("shared recorder holds %q, want %q", to, "/elsewhere")
	}
}

// TestUseResolvedPath checks that relative resolution is against the route's
// own base rather than the raw URL — the distinction that makes a relative Link
// inside a splat route point somewhere sensible.
func TestUseResolvedPath(t *testing.T) {
	var leaf react.Component = func(react.Props) react.Node {
		return react.H("p", nil,
			UseResolvedPath("edit").Path, "|",
			UseResolvedPath("..").Path, "|",
			UseResolvedPath("/top").Path)
	}

	routes := []Route{{Path: "/docs/*", Component: leaf}}
	// Static markup keeps the three resolved paths adjacent; see renderIn.
	html, err := react.RenderToStaticMarkup(Router(ParseLocation("/docs/a/b"), routes))
	if err != nil {
		t.Fatal(err)
	}
	// The route's base is "/docs" — the splat tail is not part of what it owns.
	if want := "<p>/docs/edit|/|/top</p>"; html != want {
		t.Errorf("render = %q, want %q", html, want)
	}
}
