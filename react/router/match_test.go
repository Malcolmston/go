package router

import (
	"reflect"
	"strings"
	"testing"
)

// TestMatchPath exercises the segment walker directly: the four segment kinds,
// the anchoring rule, trailing-slash tolerance and percent-decoding. Everything
// higher up in the package funnels through this function, so a table here is
// the cheapest place to pin the semantics down.
func TestMatchPath(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		want    map[string]string
		wantOK  bool
	}{
		{"static exact", "/users", "/users", map[string]string{}, true},
		{"static case-insensitive", "/Users", "/users", map[string]string{}, true},
		{"static mismatch", "/users", "/posts", nil, false},
		{"anchored at the end", "/users", "/users/7", nil, false},
		{"anchored at the start", "/users", "/admin/users", nil, false},

		{"root", "/", "/", map[string]string{}, true},
		{"empty pattern matches root", "", "/", map[string]string{}, true},

		{"trailing slash on the path", "/users", "/users/", map[string]string{}, true},
		{"trailing slash on the pattern", "/users/", "/users", map[string]string{}, true},

		{"one param", "/users/:id", "/users/7", map[string]string{"id": "7"}, true},
		{"two params", "/u/:id/p/:post", "/u/7/p/9",
			map[string]string{"id": "7", "post": "9"}, true},
		{"param does not span segments", "/users/:id", "/users/7/edit", nil, false},
		{"param is decoded", "/hello/:who", "/hello/a%20b",
			map[string]string{"who": "a b"}, true},

		{"splat captures the tail", "/files/*", "/files/a/b/c.txt",
			map[string]string{"*": "a/b/c.txt"}, true},
		{"splat captures nothing", "/files/*", "/files", map[string]string{"*": ""}, true},
		{"bare splat", "*", "/a/b", map[string]string{"*": "a/b"}, true},

		{"optional param present", "/:lang?/docs", "/fr/docs",
			map[string]string{"lang": "fr"}, true},
		{"optional param absent", "/:lang?/docs", "/docs", map[string]string{}, true},
		{"optional static present", "/docs/latest?", "/docs/latest", map[string]string{}, true},
		{"optional static absent", "/docs/latest?", "/docs", map[string]string{}, true},
		{"two optionals, none taken", "/:a?/:b?/end", "/end", map[string]string{}, true},
		{"two optionals, second taken", "/:a?/:b?/end", "/x/end",
			map[string]string{"a": "x"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := MatchPath(tt.pattern, tt.path)
			if ok != tt.wantOK {
				t.Fatalf("MatchPath(%q, %q) ok = %v, want %v", tt.pattern, tt.path, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MatchPath(%q, %q) params = %v, want %v",
					tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}

// TestMatchPathBacktracks is the case a greedy walker gets wrong: the optional
// segment can be consumed, but only a walk that undoes that choice discovers
// that skipping it is what makes the rest of the pattern fit.
func TestMatchPathBacktracks(t *testing.T) {
	params, ok := MatchPath("/:a?/fixed", "/fixed")
	if !ok {
		t.Fatal("pattern with a skippable optional segment failed to match")
	}
	if _, captured := params["a"]; captured {
		t.Errorf("optional segment left a capture behind after backtracking: %v", params)
	}
}

// TestPatternScore pins the ranking arithmetic itself, so a change to the
// weights shows up here rather than as a mysteriously reordered route table.
func TestPatternScore(t *testing.T) {
	tests := []struct {
		pattern string
		index   bool
		want    int
	}{
		{"/", false, 4},           // 2 segments + two empties
		{"/", true, 6},            // …plus the index bonus
		{"/users", false, 13},     // 2 segments + empty + static
		{"/users/new", false, 24}, // 3 segments + empty + static + static
		{"/users/:id", false, 17}, // 3 segments + empty + static + dynamic
		{"/users/*", false, 12},   // 3 segments - 2 splat + empty + static
		{"/*", false, 1},          // 2 segments - 2 splat + empty
		{"/:a/:b/:c", false, 14},  // 4 segments + empty + 3 dynamic
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			if got := PatternScore(tt.pattern, tt.index); got != tt.want {
				t.Errorf("PatternScore(%q, %v) = %d, want %d",
					tt.pattern, tt.index, got, tt.want)
			}
		})
	}
}

// TestRankedPatterns is the heart of the "not first match wins" contract: the
// order the matcher tries branches in is a function of specificity, and the
// declaration order in each of these tables is deliberately adversarial.
func TestRankedPatterns(t *testing.T) {
	tests := []struct {
		name   string
		routes []Route
		want   []string
	}{
		{
			name: "static beats dynamic regardless of declaration order",
			routes: []Route{
				{Path: "/users/:id"},
				{Path: "/users/new"},
			},
			want: []string{"/users/new", "/users/:id"},
		},
		{
			name: "dynamic beats splat",
			routes: []Route{
				{Path: "/files/*"},
				{Path: "/files/:name"},
			},
			want: []string{"/files/:name", "/files/*"},
		},
		{
			name: "a splat is always the last resort",
			routes: []Route{
				{Path: "*"},
				{Path: "/"},
				{Path: "/about"},
			},
			want: []string{"/about", "/", "/*"},
		},
		{
			name: "longer paths outrank shorter ones",
			routes: []Route{
				{Path: "/a"},
				{Path: "/a/b/c"},
				{Path: "/a/b"},
			},
			want: []string{"/a/b/c", "/a/b", "/a"},
		},
		{
			name: "an index route outranks its own layout",
			routes: []Route{{Path: "/", Children: []Route{
				{Path: "about"},
				{Index: true},
			}}},
			want: []string{"/about", "/", "/"},
		},
		{
			name: "nested routes are ranked by their joined path",
			routes: []Route{{Path: "/", Children: []Route{
				{Path: "*"},
				{Path: "users", Children: []Route{
					{Path: ":id"},
					{Path: "new"},
				}},
			}}},
			want: []string{"/users/new", "/users/:id", "/users", "/", "/*"},
		},
		{
			name: "equal-scoring siblings keep declaration order",
			routes: []Route{
				{Path: "/:a"},
				{Path: "/:b"},
			},
			want: []string{"/:a", "/:b"},
		},
		{
			name: "a pathless layout contributes no branch of its own",
			routes: []Route{{Component: nil, Children: []Route{
				{Path: "/one"},
				{Path: "/two"},
			}}},
			want: []string{"/one", "/two"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RankedPatterns(tt.routes)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("RankedPatterns = %v, want %v", got, tt.want)
			}
		})
	}
}

// nestedRoutes is the table the matching tests share: a root layout with an
// index, a two-level users section, a static route that collides with a dynamic
// sibling, and a catch-all.
var nestedRoutes = []Route{{
	Path: "/",
	Children: []Route{
		{Index: true},
		{Path: "users", Children: []Route{
			{Index: true},
			{Path: "new"},
			{Path: ":id", Children: []Route{
				{Path: "posts/:postID"},
			}},
		}},
		{Path: "files/*"},
		{Path: "*"},
	},
}}

// TestMatchRoutes checks the whole decision: which chain wins, what each level
// claims as its pathname, and what ends up in the params.
func TestMatchRoutes(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantChain  []string // Pathname of each match, outermost first
		wantParams map[string]string
	}{
		{"root renders the index", "/", []string{"/", "/"}, map[string]string{}},
		{"static wins over dynamic", "/users/new",
			[]string{"/", "/users", "/users/new"}, map[string]string{}},
		{"dynamic segment", "/users/7",
			[]string{"/", "/users", "/users/7"}, map[string]string{"id": "7"}},
		{"section index", "/users",
			[]string{"/", "/users", "/users"}, map[string]string{}},
		{"trailing slash is insignificant", "/users/",
			[]string{"/", "/users", "/users"}, map[string]string{}},
		{"deeply nested params", "/users/7/posts/9",
			[]string{"/", "/users", "/users/7", "/users/7/posts/9"},
			map[string]string{"id": "7", "postID": "9"}},
		{"splat captures the tail", "/files/a/b.txt",
			[]string{"/", "/files/a/b.txt"}, map[string]string{"*": "a/b.txt"}},
		{"catch-all is the last resort", "/nope",
			[]string{"/", "/nope"}, map[string]string{"*": "nope"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := MatchRoutes(nestedRoutes, tt.path)
			if matches == nil {
				t.Fatalf("MatchRoutes(%q) found no match", tt.path)
			}

			var chain []string
			for _, m := range matches {
				chain = append(chain, m.Pathname)
			}
			if !reflect.DeepEqual(chain, tt.wantChain) {
				t.Errorf("pathname chain = %v, want %v", chain, tt.wantChain)
			}

			leaf := matches[len(matches)-1]
			if !reflect.DeepEqual(leaf.Params, tt.wantParams) {
				t.Errorf("leaf params = %v, want %v", leaf.Params, tt.wantParams)
			}
		})
	}
}

// TestMatchRoutesNoMatch is the case a table without a catch-all has to handle.
func TestMatchRoutesNoMatch(t *testing.T) {
	if m := MatchRoutes([]Route{{Path: "/a"}}, "/b"); m != nil {
		t.Errorf("MatchRoutes matched an unrelated path: %v", m)
	}
}

// TestMatchPathnameBase pins the distinction between what a route matched and
// what it owns, which is what relative links and nested Routes both depend on.
func TestMatchPathnameBase(t *testing.T) {
	matches := MatchRoutes([]Route{{Path: "/docs/*"}}, "/docs/a/b")
	if len(matches) != 1 {
		t.Fatalf("got %d matches, want 1", len(matches))
	}
	if got, want := matches[0].Pathname, "/docs/a/b"; got != want {
		t.Errorf("Pathname = %q, want %q", got, want)
	}
	if got, want := matches[0].PathnameBase, "/docs"; got != want {
		t.Errorf("PathnameBase = %q, want %q", got, want)
	}
}

// TestMatchParamsAreIndependent guards the per-level copy: a component holding
// an ancestor's params must not see a descendant's captures appear in it.
func TestMatchParamsAreIndependent(t *testing.T) {
	matches := MatchRoutes(nestedRoutes, "/users/7/posts/9")
	for _, m := range matches[:len(matches)-1] {
		if _, ok := m.Params["postID"]; ok {
			t.Fatalf("ancestor at %q leaked the leaf's param: %v", m.Pathname, m.Params)
		}
	}
	if _, ok := matches[len(matches)-1].Params["postID"]; !ok {
		t.Error("leaf is missing its own param")
	}
}

// TestFlattenPanics covers the two structural mistakes that are rejected rather
// than silently producing a table that never matches.
func TestFlattenPanics(t *testing.T) {
	tests := []struct {
		name   string
		routes []Route
		want   string
	}{
		{
			name:   "index route with children",
			routes: []Route{{Path: "/", Children: []Route{{Index: true, Children: []Route{{Path: "x"}}}}}},
			want:   "may not have children",
		},
		{
			name:   "absolute child that does not extend its parent",
			routes: []Route{{Path: "/users", Children: []Route{{Path: "/posts"}}}},
			want:   "absolute route path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("expected a panic, got none")
				}
				if msg, _ := r.(string); !strings.Contains(msg, tt.want) {
					t.Errorf("panic = %v, want it to mention %q", r, tt.want)
				}
			}()
			RankedPatterns(tt.routes)
		})
	}
}

// TestSplatMustBeLast rejects a pattern whose splat is followed by a segment
// that could never be reached.
func TestSplatMustBeLast(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected a panic, got none")
		}
		if msg, _ := r.(string); !strings.Contains(msg, "must be last") {
			t.Errorf("panic = %v, want it to mention the splat position", r)
		}
	}()
	MatchPath("/a/*/b", "/a/x/b")
}
