package router

import (
	"testing"

	"github.com/malcolmston/react"
)

// renderLink renders one element inside a Router at path and returns the HTML.
// The Router carries no routes, so resolution is against the location itself —
// which is the right baseline for the tests that are about anchor markup rather
// than about route-relative resolution.
func renderLink(t *testing.T, path string, el react.Node) string {
	t.Helper()
	html, err := react.RenderToString(Router(ParseLocation(path), nil, el))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return html
}

// TestLink covers the markup: the derived href, pass-through props, and the
// reserved keys that must not leak into the output.
func TestLink(t *testing.T) {
	tests := []struct {
		name string
		path string
		el   react.Node
		want string
	}{
		{
			name: "absolute target",
			path: "/",
			el:   Link("/about", nil, "About"),
			want: `<a href="/about">About</a>`,
		},
		{
			name: "relative target resolves against the location",
			path: "/users/7",
			el:   Link("edit", nil, "Edit"),
			want: `<a href="/users/7/edit">Edit</a>`,
		},
		{
			name: "dot-dot climbs",
			path: "/users/7",
			el:   Link("..", nil, "Up"),
			want: `<a href="/users">Up</a>`,
		},
		{
			name: "query and hash ride along",
			path: "/",
			el:   Link("/search?q=go#top", nil, "Search"),
			want: `<a href="/search?q=go#top">Search</a>`,
		},
		{
			name: "props pass through",
			path: "/",
			el:   Link("/a", react.Props{"className": "btn", "rel": "nofollow"}, "A"),
			want: `<a class="btn" href="/a" rel="nofollow">A</a>`,
		},
		{
			name: "an href in props is ignored",
			path: "/",
			el:   Link("/a", react.Props{"href": "/hijacked"}, "A"),
			want: `<a href="/a">A</a>`,
		},
		{
			name: "several children",
			path: "/",
			el:   Link("/a", nil, "one ", "two"),
			want: `<a href="/a">one two</a>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := renderLink(t, tt.path, tt.el); got != tt.want {
				t.Errorf("render =\n  %s\nwant\n  %s", got, tt.want)
			}
		})
	}
}

// TestLinkIsRouteRelative is the case that distinguishes route-relative
// resolution from URL-relative resolution: inside a splat route, "edit" means
// "next to the route", not "next to the deepest URL segment".
func TestLinkIsRouteRelative(t *testing.T) {
	var page react.Component = func(react.Props) react.Node {
		return Link("edit", nil, "Edit")
	}
	routes := []Route{{Path: "/docs/*", Component: page}}

	html, err := react.RenderToString(Router(ParseLocation("/docs/a/b"), routes))
	if err != nil {
		t.Fatal(err)
	}
	if want := `<a href="/docs/edit">Edit</a>`; html != want {
		t.Errorf("render = %q, want %q", html, want)
	}
}

// TestNavLink covers the active computation: the prefix rule, the exact-match
// opt-out, the class merge and the aria attribute.
func TestNavLink(t *testing.T) {
	tests := []struct {
		name string
		path string
		el   react.Node
		want string
	}{
		{
			name: "exact match is active",
			path: "/users",
			el:   NavLink("/users", nil, "Users"),
			want: `<a aria-current="page" class="active" href="/users">Users</a>`,
		},
		{
			name: "a descendant is active too",
			path: "/users/7",
			el:   NavLink("/users", nil, "Users"),
			want: `<a aria-current="page" class="active" href="/users">Users</a>`,
		},
		{
			name: "end restricts to an exact match",
			path: "/users/7",
			el:   NavLink("/users", react.Props{EndProp: true}, "Users"),
			want: `<a href="/users">Users</a>`,
		},
		{
			name: "root without end would match everything",
			path: "/users",
			el:   NavLink("/", react.Props{EndProp: true}, "Home"),
			want: `<a href="/">Home</a>`,
		},
		{
			name: "an unrelated path is inactive",
			path: "/posts",
			el:   NavLink("/users", nil, "Users"),
			want: `<a href="/users">Users</a>`,
		},
		{
			name: "a string prefix that is not an ancestor is inactive",
			path: "/userstuff",
			el:   NavLink("/users", nil, "Users"),
			want: `<a href="/users">Users</a>`,
		},
		{
			name: "the active class is appended, not replaced",
			path: "/users",
			el:   NavLink("/users", react.Props{"class": "nav"}, "Users"),
			want: `<a aria-current="page" class="nav active" href="/users">Users</a>`,
		},
		{
			name: "className is the key that gets written back",
			path: "/users",
			el:   NavLink("/users", react.Props{"className": "nav"}, "Users"),
			want: `<a aria-current="page" class="nav active" href="/users">Users</a>`,
		},
		{
			name: "the active class is overridable",
			path: "/users",
			el:   NavLink("/users", react.Props{ActiveClassProp: "current"}, "Users"),
			want: `<a aria-current="page" class="current" href="/users">Users</a>`,
		},
		{
			name: "matching ignores case and trailing slashes",
			path: "/Users/",
			el:   NavLink("/users", nil, "Users"),
			want: `<a aria-current="page" class="active" href="/users">Users</a>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := renderLink(t, tt.path, tt.el); got != tt.want {
				t.Errorf("render =\n  %s\nwant\n  %s", got, tt.want)
			}
		})
	}
}

// TestNavigateElement covers the declarative redirect, including the fact that
// it renders nothing of its own and that the replace variant is carried
// through.
func TestNavigateElement(t *testing.T) {
	tests := []struct {
		name        string
		el          react.Node
		wantTo      string
		wantReplace bool
	}{
		{"push", Navigate("/login"), "/login", false},
		{"replace", NavigateReplace("/login"), "/login", true},
		{"relative targets are resolved", Navigate("sibling"), "/here/sibling", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := Router(ParseLocation("/here"), nil, tt.el)
			html, err := react.RenderToString(page)
			if err != nil {
				t.Fatal(err)
			}
			if html != "" {
				t.Errorf("Navigate rendered %q, want nothing", html)
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

// TestPathIsActive tables the comparison on its own, away from any rendering,
// because it is the part of NavLink most likely to be quietly wrong.
func TestPathIsActive(t *testing.T) {
	tests := []struct {
		current string
		target  string
		end     bool
		want    bool
	}{
		{"/users", "/users", false, true},
		{"/users", "/users", true, true},
		{"/users/7", "/users", false, true},
		{"/users/7", "/users", true, false},
		{"/users", "/users/7", false, false},
		{"/userstuff", "/users", false, false},
		{"/Users", "/users", false, true},
		{"/users/", "/users", false, true},
		{"/anything", "/", false, true},
		{"/anything", "/", true, false},
		{"/", "/", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.current+" vs "+tt.target, func(t *testing.T) {
			if got := pathIsActive(tt.current, tt.target, tt.end); got != tt.want {
				t.Errorf("pathIsActive(%q, %q, end=%v) = %v, want %v",
					tt.current, tt.target, tt.end, got, tt.want)
			}
		})
	}
}
