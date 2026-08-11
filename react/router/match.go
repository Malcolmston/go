package router

import (
	"fmt"
	"sort"
	"strings"

	"github.com/malcolmston/react"
)

// This file is the matcher — the part of react-router that actually earns the
// name. Everything else in the package is plumbing around the two questions
// answered here: which routes *could* serve this URL, and which of them is the
// *best* one.
//
// # Why ranking, and not "first match wins"
//
// The single most common way to get a port of this library wrong is to walk the
// route table in declaration order and take the first pattern that matches.
// That is not what react-router does, and the difference is visible in the
// smallest realistic route table:
//
//	{Path: "/users/:id"}
//	{Path: "/users/new"}
//
// Under first-match-wins, "/users/new" renders the profile page for a user
// whose id is the literal string "new", and the only fix is to make the author
// order their routes by hand — a rule that is invisible in the source, easy to
// break by adding a route, and impossible to enforce.
//
// react-router instead scores every possible route path and tries them from the
// highest score down, so specificity beats declaration order: a static segment
// (10 points) outranks a dynamic one (3), which outranks the empty segment (1),
// and a splat is a penalty (-2) rather than a bonus, so "/*" is always the last
// resort. An index route gets a +2 bonus so it beats its own layout parent for
// the parent's exact path. Longer paths score higher simply because the segment
// count seeds the score. See [PatternScore] for the constants and
// [RankedPatterns] for the resulting order.
//
// The scoring constants and the tie-break are reproduced from upstream rather
// than invented, because the whole value of the rule is that it agrees with
// what a react-router user already expects.

// Route is one node of a route table — the Go form of react-router's route
// objects (`createBrowserRouter([{path, element, children}])`) rather than of
// its `<Route>` JSX element, since a Go tree cannot carry configuration in its
// children the way JSX does.
//
// A route table is ordinary data: build it once at package level and share it,
// or compute it per request. Nothing here holds runtime state.
type Route struct {
	// Path is the pattern this route matches, relative to its parent unless it
	// begins with "/" (in which case it must begin with the parent's full path,
	// exactly as upstream requires). It understands four kinds of segment:
	//
	//	users     a static segment, matched case-insensitively by default
	//	:id       a dynamic segment, captured into Params under "id"
	//	:id?      an optional segment: it may be absent entirely
	//	*         a splat, matching the whole remaining path into Params["*"]
	//
	// An empty Path means the route has no path of its own — a layout route
	// that exists only to wrap its children in a component. This is the one
	// place the Go shape is forced to differ from upstream: JavaScript
	// distinguishes an absent `path` from `path: ""`, and Go's zero string
	// cannot. Write "/" for a route that matches the site root.
	Path string

	// Component is rendered when this route matches. It is handed no props;
	// everything it needs — params, the location, its child route — comes from
	// the hooks in this package.
	//
	// A nil Component renders [Outlet] instead, which is upstream's behavior
	// for a route with no element: a layout route that only groups children
	// does not have to supply a component just to pass them through.
	Component react.Component

	// Children are nested routes whose Path is relative to this one. A route
	// with children is a layout route: it renders, and its matched child
	// renders wherever it puts an [Outlet].
	Children []Route

	// Index marks this route as its parent's index route — the child that
	// matches the parent's own path exactly, the way "/users" should show a
	// user list rather than an empty layout. An index route has no path of its
	// own and may not have children; both are checked when the table is
	// flattened.
	Index bool

	// CaseSensitive makes this route's static segments match exactly. The
	// default is upstream's: "/Users" and "/users" are the same route, because
	// case-sensitive URLs are a well-known source of duplicate content.
	CaseSensitive bool
}

// Match is one route in the chain that matched a URL, from the outermost layout
// down to the leaf. It is react-router's `RouteMatch`, and a full match of a
// nested table produces one Match per level — which is precisely what lets each
// level render its own component and hand the next one to [Outlet].
type Match struct {
	// Route is the route that matched. It is a copy, so mutating it cannot
	// disturb the table it came from.
	Route Route

	// Params are every dynamic segment captured so far, this route's and every
	// ancestor's, with values URL-decoded. A splat is stored under "*". The map
	// is freshly allocated per Match and is safe to keep.
	Params map[string]string

	// Pathname is the portion of the URL this route and its ancestors consumed,
	// always starting with "/". For the leaf of a complete match it is the whole
	// URL path.
	Pathname string

	// PathnameBase is Pathname with any splat-matched tail removed. It is the
	// value relative links resolve against ([ResolvePath]) and the prefix a
	// nested [Routes] strips before matching, because both mean "the path this
	// route owns" rather than "the path it happened to swallow".
	PathnameBase string
}

// The scoring weights, reproduced from react-router's computeScore. They are
// exported through [PatternScore] rather than as constants because their
// absolute values carry no meaning — only their order does — and pinning them
// as API would freeze an implementation detail of the ranking.
const (
	staticSegmentScore  = 10
	dynamicSegmentScore = 3
	indexRouteScore     = 2
	emptySegmentScore   = 1
	splatPenalty        = -2
)

// PatternScore returns the specificity score of a fully joined route pattern,
// the number [RankedPatterns] sorts on. It is exported because a surprising
// match is nearly always a surprising ranking, and being able to print the two
// scores turns "why did that route win?" into a one-line answer.
//
// The rules: every segment contributes to a base score equal to the segment
// count, then a static segment adds 10, a dynamic (":x") segment adds 3, and an
// empty segment adds 1; a pattern containing a splat takes a flat -2 penalty
// and the splat segment itself scores nothing; an index route adds 2.
//
// The pattern must be the *joined* path — "/users/:id", not the ":id" fragment
// a nested route declares — because specificity is a property of the whole URL
// a route claims, not of one level of it.
func PatternScore(pattern string, index bool) int {
	segments := strings.Split(pattern, "/")

	score := len(segments)
	for _, s := range segments {
		if s == "*" {
			score += splatPenalty
			break
		}
	}
	if index {
		score += indexRouteScore
	}

	for _, s := range segments {
		switch {
		case s == "*":
			// Already accounted for by the penalty above; scoring it again
			// would let a splat pay for itself.
		case isDynamicSegment(s):
			score += dynamicSegmentScore
		case s == "":
			score += emptySegmentScore
		default:
			score += staticSegmentScore
		}
	}
	return score
}

// isDynamicSegment reports whether a pattern segment captures a parameter.
// Upstream's test is the regexp /^:[\w-]+$/; the equivalent here is "starts
// with a colon and has at least one character after it", with a trailing "?"
// (the optional marker) not counting toward that character.
func isDynamicSegment(s string) bool {
	s = strings.TrimSuffix(s, "?")
	return len(s) > 1 && s[0] == ':'
}

// routeMeta is one level of a flattened branch: a route plus the fragment of
// the pattern it contributed and its position among its siblings.
//
// relativePath is stored separately from Route.Path because an absolute child
// path ("/users/:id" declared under a parent at "/users") is rewritten to the
// fragment the child actually consumes, and the matcher walks fragments.
type routeMeta struct {
	relativePath  string
	caseSensitive bool
	childIndex    int
	route         Route
}

// branch is one complete root-to-leaf path through the route table: the joined
// pattern, its score, and the chain of routes that produced it. Matching is
// performed against branches, never against the tree, which is what makes
// ranking possible at all — a tree has no total order, a flat list of branches
// does.
type branch struct {
	path  string
	score int
	metas []routeMeta
}

// flattenRoutes walks the route table depth-first and produces one branch per
// matchable route, carrying the parent's joined path and meta chain down.
//
// Two structural mistakes are rejected loudly rather than producing a table
// that quietly never matches: an index route with children (an index route is
// by definition a leaf), and an absolute child path that does not extend its
// parent's path (which would claim a URL its parent could never be reached
// through). Both are upstream invariants.
func flattenRoutes(routes []Route, parents []routeMeta, parentPath string) []branch {
	var out []branch

	for i, route := range routes {
		relativePath := route.Path
		if strings.HasPrefix(relativePath, "/") {
			if !strings.HasPrefix(relativePath, parentPath) {
				panic(fmt.Sprintf("router: absolute route path %q nested under %q; "+
					"an absolute child path must begin with its parent's full path, "+
					"or be written relative to it", relativePath, parentPath))
			}
			relativePath = strings.TrimPrefix(relativePath, parentPath)
		}
		if route.Index && len(route.Children) > 0 {
			panic(fmt.Sprintf("router: index route %q may not have children; "+
				"an index route is what renders when the parent's own path matches "+
				"exactly, so it is always a leaf", joinPaths(parentPath, relativePath)))
		}

		path := joinPaths(parentPath, relativePath)
		metas := append(append([]routeMeta(nil), parents...), routeMeta{
			relativePath:  relativePath,
			caseSensitive: route.CaseSensitive,
			childIndex:    i,
			route:         route,
		})

		if len(route.Children) > 0 {
			out = append(out, flattenRoutes(route.Children, metas, path)...)
		}

		// A pathless layout route contributes no branch of its own: it exists
		// only to wrap whichever child matched. Upstream spells this
		// `route.path == null && !route.index`; here an empty Path is the only
		// way to say "no path", so it reads the same.
		if route.Path == "" && !route.Index {
			continue
		}

		out = append(out, branch{
			path:  path,
			score: PatternScore(path, route.Index),
			metas: metas,
		})
	}

	return out
}

// rankBranches orders branches best-first, reproducing react-router's
// rankRouteBranches: descending score, and for equal scores a sibling
// tie-break.
//
// The tie-break only fires between branches that differ solely in their last
// child index — genuine siblings — where declaration order is the author's own
// statement of preference. Branches that are not siblings compare equal and the
// sort is stable, so their relative order is left exactly as flattening
// produced it. Using an unstable sort here would make an equal-score table
// order-dependent between runs, which is precisely the bug ranking exists to
// remove.
func rankBranches(branches []branch) {
	sort.SliceStable(branches, func(a, b int) bool {
		if branches[a].score != branches[b].score {
			return branches[a].score > branches[b].score
		}
		return siblingLess(branches[a].metas, branches[b].metas)
	})
}

// siblingLess implements the sibling tie-break: two branches are siblings when
// their child-index chains have the same length and agree everywhere but the
// last position, in which case the earlier-declared one wins.
func siblingLess(a, b []routeMeta) bool {
	if len(a) != len(b) || len(a) == 0 {
		return false
	}
	for i := range len(a) - 1 {
		if a[i].childIndex != b[i].childIndex {
			return false
		}
	}
	return a[len(a)-1].childIndex < b[len(b)-1].childIndex
}

// RankedPatterns returns the joined route patterns of a table in the order the
// matcher tries them, best-first. It is a diagnostic, not a routing entry
// point: printing it is the fastest way to see why one route beat another, and
// asserting on it is how this package's own ranking tests are written.
//
// Pathless layout routes do not appear, because they are never matched against
// on their own — only the branches that pass through them are.
func RankedPatterns(routes []Route) []string {
	branches := flattenRoutes(routes, nil, "")
	rankBranches(branches)

	out := make([]string, len(branches))
	for i, b := range branches {
		out[i] = b.path
	}
	return out
}

// MatchRoutes matches a URL path against a route table and returns the winning
// chain of matches, outermost first, or nil when nothing matched.
//
// This is the whole routing decision in one function, and it is deliberately
// usable without rendering anything: a handler that only needs to know *which*
// route a URL belongs to — for logging, for an access check, for building a
// sitemap — should call this rather than render a tree and inspect it.
//
// Every branch is tried in ranked order and the first one that matches wins,
// which is not the same as first-match-wins over the table: the ordering is by
// specificity, not by declaration. See the file comment for why.
//
// path is a URL path, not a full URL; strip the query and hash first (or use
// [Location.Path], which already has). Trailing slashes are insignificant.
func MatchRoutes(routes []Route, path string) []Match {
	branches := flattenRoutes(routes, nil, "")
	rankBranches(branches)

	for _, b := range branches {
		if m := matchBranch(b, path); m != nil {
			return m
		}
	}
	return nil
}

// matchBranch tries one root-to-leaf branch against a path, matching each
// level's fragment against what the previous levels left over.
//
// Matching level by level rather than against the joined pattern in one go is
// what produces a per-level Pathname and PathnameBase, and those are not
// bookkeeping: PathnameBase is what a relative [Link] resolves against and what
// a nested [Routes] strips, so a whole-pattern match would leave both features
// with no way to know where one route ends and the next begins.
//
// Only the last level is matched with end=true. Every level above it is a
// prefix match by construction — a layout route consumes its own segments and
// hands the rest down.
func matchBranch(b branch, path string) []Match {
	var (
		matches         []Match
		params          = map[string]string{}
		matchedPathname = "/"
	)

	for i, meta := range b.metas {
		// What is left for this level to consume. Slicing rather than
		// re-splitting keeps the original text — and therefore any percent
		// escapes — intact.
		remaining := path
		if matchedPathname != "/" {
			remaining = path[len(matchedPathname):]
			if remaining == "" {
				remaining = "/"
			}
		}

		res, ok := matchPattern(meta.relativePath, remaining, i == len(b.metas)-1, meta.caseSensitive)
		if !ok {
			return nil
		}
		for k, v := range res.params {
			params[k] = v
		}

		// Params are copied per level so that a component reading them through
		// UseParams cannot reach a later level's captures by holding the map.
		levelParams := make(map[string]string, len(params))
		for k, v := range params {
			levelParams[k] = v
		}

		matches = append(matches, Match{
			Route:        meta.route,
			Params:       levelParams,
			Pathname:     normalizePathname(joinPaths(matchedPathname, res.pathname)),
			PathnameBase: normalizePathname(joinPaths(matchedPathname, res.pathnameBase)),
		})

		// A base of "/" means this level consumed nothing (a pathless layout,
		// or a splat that matched at the root), so the cursor must not move.
		if res.pathnameBase != "/" {
			matchedPathname = normalizePathname(joinPaths(matchedPathname, res.pathnameBase))
		}
	}

	return matches
}

// normalizePathname strips a trailing slash from a joined pathname, leaving the
// root alone.
//
// Upstream normalizes only PathnameBase and lets Pathname keep the slash an
// index route contributes, so a match on "/users" reports a Pathname of
// "/users/". That is an artifact of building the string by concatenation rather
// than a decision, and it leaks: a component comparing Match.Pathname against
// Location.Path would find them unequal for exactly the URLs an index route
// serves. Since the matcher already treats trailing slashes as insignificant,
// normalizing both fields is the consistent choice.
func normalizePathname(p string) string {
	if p == "/" {
		return p
	}
	return strings.TrimRight(p, "/")
}

// patternMatch is what matching one pattern fragment against one path fragment
// produces: the captures, the text consumed, and the text consumed before any
// splat began.
type patternMatch struct {
	params       map[string]string
	pathname     string
	pathnameBase string
}

// MatchPath matches a single pattern against a path and returns the captured
// params. It is react-router's `matchPath` narrowed to the question callers
// actually ask — "does this URL look like this pattern, and what is in it?" —
// and it is what [UseMatch] is built on.
//
// The match is anchored at both ends: "/users" does not match "/users/7" unless
// the pattern says so with a splat ("/users/*"). Static segments are matched
// case-insensitively, matching the default for a [Route]; there is no
// case-sensitive variant here because a caller who needs one can compare the
// captured values directly.
//
// The returned map is non-nil whenever ok is true, even for a pattern with no
// dynamic segments, so a caller may index it without checking.
func MatchPath(pattern, path string) (map[string]string, bool) {
	res, ok := matchPattern(pattern, path, true, false)
	if !ok {
		return nil, false
	}
	return res.params, true
}

// matchPattern is the segment walker every match in the package goes through.
//
// end distinguishes a leaf match, which must consume the whole path, from a
// layout match, which consumes a prefix and hands the rest to its children.
//
// The walk backtracks. That is required rather than defensive: an optional
// segment ("/:lang?/docs") creates a genuine choice at each position — consume
// the segment or skip it — and a greedy walk that guessed wrong would reject a
// path the pattern does describe. The recursion depth is bounded by the number
// of pattern segments, and only optional segments branch at all, so the cost is
// linear for every pattern that does not use them.
func matchPattern(pattern, path string, end, caseSensitive bool) (patternMatch, bool) {
	var (
		patSegs  = splitSegments(pattern)
		pathSegs = splitSegments(path)
		params   = map[string]string{}
	)

	// splatAt records where a splat began consuming, so pathnameBase can stop
	// there. -1 means the pattern had no splat.
	splatAt := -1

	var walk func(pi, si int) (int, bool)
	walk = func(pi, si int) (int, bool) {
		if pi == len(patSegs) {
			if end && si != len(pathSegs) {
				return 0, false
			}
			return si, true
		}

		seg := patSegs[pi]
		if seg == "*" {
			// A splat swallows everything left, including nothing at all, and
			// is necessarily the last thing in the pattern — anything after it
			// could never be reached.
			if pi != len(patSegs)-1 {
				panic(fmt.Sprintf("router: splat segment must be last in pattern %q; "+
					"a segment after * could never match", pattern))
			}
			splatAt = si
			params["*"] = decodeSegment(strings.Join(pathSegs[si:], "/"))
			return len(pathSegs), true
		}

		optional := strings.HasSuffix(seg, "?")
		core := strings.TrimSuffix(seg, "?")

		if si < len(pathSegs) {
			if isDynamicSegment(core) {
				// A dynamic segment never matches an empty path segment: an
				// empty capture would make "/users//edit" match "/users/:id/edit"
				// with an id of "".
				if pathSegs[si] != "" {
					name := core[1:]
					prev, had := params[name]
					params[name] = decodeSegment(pathSegs[si])
					if n, ok := walk(pi+1, si+1); ok {
						return n, true
					}
					// Undo the capture before trying the skip branch, so a
					// failed attempt leaves no trace in the params.
					if had {
						params[name] = prev
					} else {
						delete(params, name)
					}
				}
			} else if segmentsEqual(core, pathSegs[si], caseSensitive) {
				if n, ok := walk(pi+1, si+1); ok {
					return n, true
				}
			}
		}

		if optional {
			return walk(pi+1, si)
		}
		return 0, false
	}

	consumed, ok := walk(0, 0)
	if !ok {
		return patternMatch{}, false
	}

	base := consumed
	if splatAt >= 0 {
		base = splatAt
	}
	return patternMatch{
		params:       params,
		pathname:     "/" + strings.Join(pathSegs[:consumed], "/"),
		pathnameBase: "/" + strings.Join(pathSegs[:base], "/"),
	}, true
}

// segmentsEqual compares a static pattern segment with a path segment, folding
// case unless the route asked not to. Percent-escapes are decoded first so that
// a pattern segment written in plain text still matches a URL that escaped a
// character in it.
func segmentsEqual(pattern, actual string, caseSensitive bool) bool {
	actual = decodeSegment(actual)
	if caseSensitive {
		return pattern == actual
	}
	return strings.EqualFold(pattern, actual)
}
