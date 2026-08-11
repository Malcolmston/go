package router

import (
	"net/url"
	"strings"
)

// This file is the URL layer: the value type that stands in for the browser's
// `window.location`, and the pure string algebra react-router performs on
// paths — parsing, joining, and resolving a relative `to` against the route the
// link was written in.
//
// None of it touches the tree. Everything here is a pure function of strings,
// which is deliberate: path resolution is where routing bugs hide, and code
// that can be tested without rendering anything gets tested properly.

// Location is the address a tree is rendered for — the port's stand-in for
// react-router's `Location`, which is itself a subset of the DOM's
// `window.location`.
//
// It is a plain value rather than an object with an identity because there is
// no history stack here to give it one. Upstream's `key` and `state` fields are
// therefore absent: `key` exists to distinguish two visits to the same URL in a
// session history, and `state` is data the History API carries between them.
// A server render sees one URL and no session, so both would be permanently
// empty. Build one from an incoming request with [ParseLocation]:
//
//	loc := router.ParseLocation(r.URL.RequestURI())
//
// The three fields hold the URL exactly as it was written, including
// punctuation: Search keeps its leading "?" and Hash its leading "#", so that
// [Location.String] can rebuild the original and an empty field is
// unambiguously "absent" rather than "present but empty". Path is the only
// field the matcher looks at.
type Location struct {
	// Path is the URL path, always beginning with "/" for a location built by
	// [ParseLocation]. It is the only part of the URL that route patterns are
	// matched against.
	Path string

	// Search is the query string including its leading "?", or "" when there is
	// none. Read it through [Location.Query] rather than parsing it by hand.
	Search string

	// Hash is the fragment including its leading "#", or "" when there is none.
	// It is carried through links and never sent to a server, exactly as in a
	// browser; nothing in this package matches on it.
	Hash string
}

// ParseLocation splits a raw URL reference — "/users/7?tab=posts#top" — into a
// [Location]. It is the constructor a request handler wants:
//
//	page := router.Router(router.ParseLocation(r.URL.RequestURI()), routes)
//
// The split is textual and never fails: an input with no path yields Path "/",
// and a scheme-and-host URL keeps everything before the first "?" or "#" as the
// path, which for a routing decision is the wrong thing to hand it anyway. Use
// [LocationFromURL] when a parsed [net/url.URL] is already in hand.
func ParseLocation(raw string) Location {
	var loc Location

	if i := strings.IndexByte(raw, '#'); i >= 0 {
		loc.Hash = raw[i:]
		raw = raw[:i]
	}
	if i := strings.IndexByte(raw, '?'); i >= 0 {
		loc.Search = raw[i:]
		raw = raw[:i]
	}

	// A bare "?" or "#" carries no information and would make String round-trip
	// punctuation the caller never wrote.
	if loc.Search == "?" {
		loc.Search = ""
	}
	if loc.Hash == "#" {
		loc.Hash = ""
	}

	loc.Path = raw
	if loc.Path == "" {
		loc.Path = "/"
	}
	return loc
}

// LocationFromURL builds a [Location] from a parsed URL, taking the path in its
// encoded form so that a %2F in a path segment is still distinguishable from a
// separator when the matcher splits on "/". A nil URL yields the zero Location.
func LocationFromURL(u *url.URL) Location {
	if u == nil {
		return Location{}
	}
	loc := Location{Path: u.EscapedPath()}
	if loc.Path == "" {
		loc.Path = "/"
	}
	if u.RawQuery != "" {
		loc.Search = "?" + u.RawQuery
	}
	if u.Fragment != "" {
		loc.Hash = "#" + u.EscapedFragment()
	}
	return loc
}

// Query parses [Location.Search] into the standard library's query type. It
// replaces upstream's `URLSearchParams`, which has no Go counterpart;
// [net/url.Values] is the same idea with Go's naming.
//
// Parsing is lenient in the same direction the browser's is: a malformed pair
// is dropped and every pair that did parse is still returned, because a
// component reading `?page=2` should not lose it to an unrelated broken
// parameter elsewhere in the string. The result is never nil, so indexing and
// [net/url.Values.Get] are safe on a location with no query at all.
func (l Location) Query() url.Values {
	raw := strings.TrimPrefix(l.Search, "?")
	if raw == "" {
		return url.Values{}
	}
	// The error is deliberately discarded: ParseQuery returns every pair it
	// managed to decode alongside it, which is the lenient behavior described
	// above. A nil map cannot come back from a non-empty input.
	v, _ := url.ParseQuery(raw)
	if v == nil {
		return url.Values{}
	}
	return v
}

// String rebuilds the URL reference the Location was parsed from. It is the
// inverse of [ParseLocation] for any input that function produced, and it is
// what [Link] writes into an href.
func (l Location) String() string {
	return l.Path + l.Search + l.Hash
}

// ResolvePath resolves a link target against the path it was written in,
// reproducing react-router's `resolvePath`. It is the reason a `Link` deep
// inside a nested route can say `to: "edit"` and mean "one level down from
// here" rather than "/edit".
//
// The rules, in order:
//
//   - An empty to keeps fromPathname and only replaces the search and hash, so
//     to "?page=2" is "stay here, change the query".
//   - A to beginning with "/" is absolute and replaces the path outright.
//   - Anything else is relative: its segments are appended to fromPathname,
//     with "." ignored and ".." popping one segment. Popping stops at the root,
//     so a surplus of ".." resolves to "/" rather than escaping above it.
//
// Note the deviation worth knowing about, which is upstream's too: resolution
// is against a *path*, not against a route hierarchy, and a trailing slash on
// fromPathname is stripped first. "b" resolved from "/a" is "/a/b", not "/b" —
// unlike an HTML relative href, which would resolve against the directory and
// give "/b". [Link] passes the matched route's pathname base as fromPathname,
// which is what makes the route-relative reading the correct one.
func ResolvePath(to string, fromPathname string) Location {
	target := ParseLocation(to)
	if fromPathname == "" {
		fromPathname = "/"
	}

	switch {
	case to == "" || strings.HasPrefix(to, "?") || strings.HasPrefix(to, "#"):
		// ParseLocation defaulted the empty path to "/"; a search- or hash-only
		// target means "this page", so put the original path back.
		target.Path = fromPathname
	case strings.HasPrefix(target.Path, "/"):
		// Already absolute; nothing to resolve.
	default:
		target.Path = resolvePathname(target.Path, fromPathname)
	}
	return target
}

// resolvePathname is [ResolvePath]'s relative-segment walker, kept separate
// because it is the only part with a loop in it and the only part that can be
// got subtly wrong.
//
// The leading empty segment produced by splitting "/a/b" is retained and acts
// as the root sentinel: a segment list of length one is exactly "at the root",
// which is what stops ".." from popping past it and what makes the final
// re-join produce "/" instead of "".
func resolvePathname(relative, from string) string {
	segments := strings.Split(strings.TrimRight(from, "/"), "/")

	for _, seg := range strings.Split(relative, "/") {
		switch seg {
		case "..":
			if len(segments) > 1 {
				segments = segments[:len(segments)-1]
			}
		case ".":
			// A no-op segment; upstream drops it and so do we.
		default:
			segments = append(segments, seg)
		}
	}

	if len(segments) > 1 {
		return strings.Join(segments, "/")
	}
	return "/"
}

// joinPaths concatenates path fragments the way react-router's `joinPaths`
// does: join on "/", then collapse every run of slashes back to one. The
// collapse is what makes the parent path "/" and the child path "users" produce
// "/users" rather than "//users", and it is why callers may be sloppy about
// whether their fragments carry leading or trailing slashes.
func joinPaths(parts ...string) string {
	joined := strings.Join(parts, "/")
	for strings.Contains(joined, "//") {
		joined = strings.ReplaceAll(joined, "//", "/")
	}
	return joined
}

// splitSegments turns a path into the segment list the matcher walks: leading
// and trailing slashes are dropped, so "/", "" and "/users/" and "/users" all
// reduce to the same shape.
//
// Collapsing the trailing slash here is how the whole package gets its
// trailing-slash tolerance for free — "/users/" and "/users" are the same
// route, which is upstream's behavior and almost always what a caller means.
// Interior empty segments (from a "//" in the URL) are preserved, because those
// really are distinct paths and silently merging them would let two different
// URLs match one pattern.
func splitSegments(path string) []string {
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

// decodeSegment URL-decodes a captured parameter value, which is what makes
// `:name` on the URL "/hello%20world" yield "hello world" rather than the raw
// escape. A value that does not decode is handed back untouched: a param that
// happens to contain a stray "%" is far more useful to a component than an
// error it has no way to report from inside a match.
func decodeSegment(s string) string {
	if !strings.ContainsRune(s, '%') {
		return s
	}
	if decoded, err := url.PathUnescape(s); err == nil {
		return decoded
	}
	return s
}
