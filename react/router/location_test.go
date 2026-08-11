package router

import (
	"net/url"
	"reflect"
	"testing"
)

// TestParseLocation covers the split, including the cases where punctuation is
// present but empty and the case where there is no path at all.
func TestParseLocation(t *testing.T) {
	tests := []struct {
		raw  string
		want Location
	}{
		{"/", Location{Path: "/"}},
		{"", Location{Path: "/"}},
		{"/users", Location{Path: "/users"}},
		{"/users?page=2", Location{Path: "/users", Search: "?page=2"}},
		{"/users#top", Location{Path: "/users", Hash: "#top"}},
		{"/users?page=2#top", Location{Path: "/users", Search: "?page=2", Hash: "#top"}},
		{"/users?", Location{Path: "/users"}},
		{"/users#", Location{Path: "/users"}},
		{"?page=2", Location{Path: "/", Search: "?page=2"}},
		// The hash is split first, so a "?" inside a fragment stays there.
		{"/a#b?c", Location{Path: "/a", Hash: "#b?c"}},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got := ParseLocation(tt.raw)
			if got != tt.want {
				t.Errorf("ParseLocation(%q) = %+v, want %+v", tt.raw, got, tt.want)
			}
			// Everything ParseLocation produces must round-trip, since String is
			// what a Link writes into an href.
			if want := tt.want.String(); ParseLocation(want).String() != want {
				t.Errorf("String() = %q did not round-trip", want)
			}
		})
	}
}

// TestLocationQuery checks the parse and the two leniency guarantees: a missing
// query yields an empty (never nil) Values, and a malformed pair does not take
// the well-formed ones down with it.
func TestLocationQuery(t *testing.T) {
	tests := []struct {
		name   string
		search string
		want   url.Values
	}{
		{"none", "", url.Values{}},
		{"one", "?a=1", url.Values{"a": {"1"}}},
		{"repeated", "?a=1&a=2", url.Values{"a": {"1", "2"}}},
		{"empty value", "?a=", url.Values{"a": {""}}},
		{"escaped", "?q=a%20b", url.Values{"q": {"a b"}}},
		{"malformed pair is dropped, the rest survives", "?a=%zz&b=2", url.Values{"b": {"2"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Location{Path: "/", Search: tt.search}.Query()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Query() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestLocationFromURL checks the bridge from net/url, which is how a real
// handler will build a Location most of the time.
func TestLocationFromURL(t *testing.T) {
	u, err := url.Parse("https://example.com/users/7?tab=posts#bio")
	if err != nil {
		t.Fatal(err)
	}
	want := Location{Path: "/users/7", Search: "?tab=posts", Hash: "#bio"}
	if got := LocationFromURL(u); got != want {
		t.Errorf("LocationFromURL = %+v, want %+v", got, want)
	}
	if got := LocationFromURL(nil); got != (Location{}) {
		t.Errorf("LocationFromURL(nil) = %+v, want the zero Location", got)
	}
}

// TestResolvePath is the relative-link algebra. The ".." cases matter most:
// they are what a link in a nested route uses to point at its parent, and
// popping past the root has to stop rather than produce nonsense.
func TestResolvePath(t *testing.T) {
	tests := []struct {
		name string
		to   string
		from string
		want Location
	}{
		{"absolute replaces everything", "/about", "/users/7", Location{Path: "/about"}},
		{"relative appends", "edit", "/users/7", Location{Path: "/users/7/edit"}},
		{"relative from the root", "about", "/", Location{Path: "/about"}},
		{"dot-dot pops one level", "..", "/users/7", Location{Path: "/users"}},
		{"dot-dot then a segment", "../new", "/users/7", Location{Path: "/users/new"}},
		{"dot-dot stops at the root", "../../../..", "/a/b", Location{Path: "/"}},
		{"single dot is ignored", "./edit", "/users/7", Location{Path: "/users/7/edit"}},
		{"trailing slash on the source is ignored", "edit", "/users/7/",
			Location{Path: "/users/7/edit"}},
		{"query only keeps the path", "?page=2", "/users",
			Location{Path: "/users", Search: "?page=2"}},
		{"hash only keeps the path", "#top", "/users",
			Location{Path: "/users", Hash: "#top"}},
		{"empty target keeps the path", "", "/users", Location{Path: "/users"}},
		{"search and hash ride along", "edit?x=1#y", "/users/7",
			Location{Path: "/users/7/edit", Search: "?x=1", Hash: "#y"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolvePath(tt.to, tt.from); got != tt.want {
				t.Errorf("ResolvePath(%q, %q) = %+v, want %+v", tt.to, tt.from, got, tt.want)
			}
		})
	}
}

// TestSplitSegments pins the normalization every comparison in the package sits
// on: leading and trailing slashes vanish, interior empties do not.
func TestSplitSegments(t *testing.T) {
	tests := []struct {
		path string
		want []string
	}{
		{"", nil},
		{"/", nil},
		{"/a", []string{"a"}},
		{"/a/", []string{"a"}},
		{"a/b", []string{"a", "b"}},
		{"/a//b", []string{"a", "", "b"}},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := splitSegments(tt.path); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitSegments(%q) = %#v, want %#v", tt.path, got, tt.want)
			}
		})
	}
}

// TestJoinPaths checks the slash collapsing that lets every caller be sloppy
// about which side of a join carries the separator.
func TestJoinPaths(t *testing.T) {
	tests := []struct {
		parts []string
		want  string
	}{
		{[]string{"", "/"}, "/"},
		{[]string{"/", "users"}, "/users"},
		{[]string{"/", "/users"}, "/users"},
		{[]string{"/users", "/7"}, "/users/7"},
		{[]string{"/users/", "/7"}, "/users/7"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := joinPaths(tt.parts...); got != tt.want {
				t.Errorf("joinPaths(%q) = %q, want %q", tt.parts, got, tt.want)
			}
		})
	}
}
