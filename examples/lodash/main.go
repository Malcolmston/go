// Command lodash-example exercises a broad slice of the github.com/malcolmston/lodash
// port: collection/array helpers, object and deep-path helpers, string case
// conversion and templating, lang predicates, function combinators, and the
// chain wrappers. It also probes generics behaviour and edge cases (empty
// slices, nil maps, nil interface values).
//
// Everything runs in-process and the program terminates on its own.
package main

import (
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	lodash "github.com/malcolmston/lodash"
)

type user struct {
	Name string
	Age  int
	Team string
}

func section(name string) {
	fmt.Printf("\n=== %s ===\n", name)
}

func show(label string, v any) {
	fmt.Printf("%-28s %v\n", label+":", v)
}

func main() {
	collections()
	arrays()
	setOps()
	mathAndNumbers()
	objects()
	deepPaths()
	stringsDemo()
	langDemo()
	combinators()
	functionUtils()
	chaining()
	edgeCases()

	fmt.Println("\nAll sections completed.")
}

func collections() {
	section("collections")

	nums := []int{1, 2, 3, 4, 5, 6, 7, 8}

	// Map changes the element type — generics infer both params from fn.
	labels := lodash.Map(nums, func(n int) string { return "n" + strconv.Itoa(n) })
	show("Map -> []string", labels)
	show("MapI (value*index)", lodash.MapI(nums, func(n, i int) int { return n * i }))

	evens := lodash.Filter(nums, func(n int) bool { return n%2 == 0 })
	show("Filter evens", evens)
	show("Reject evens", lodash.Reject(nums, func(n int) bool { return n%2 == 0 }))

	show("Reduce sum", lodash.Reduce(nums, func(acc, cur int) int { return acc + cur }, 0))
	show("ReduceRight concat", lodash.ReduceRight(labels, func(acc, cur string) string { return acc + cur }, ""))

	var seen []int
	lodash.ForEachI(nums[:3], func(n, i int) { seen = append(seen, i*100+n) })
	show("ForEachI", seen)

	var rev []int
	lodash.ForEachRight(nums[:4], func(n int) { rev = append(rev, n) })
	show("ForEachRight", rev)

	v, ok := lodash.Find(nums, func(n int) bool { return n > 5 })
	show("Find n>5", fmt.Sprintf("%d (found=%t)", v, ok))
	v, ok = lodash.FindLast(nums, func(n int) bool { return n < 4 })
	show("FindLast n<4", fmt.Sprintf("%d (found=%t)", v, ok))
	show("FindIndex n>5", lodash.FindIndex(nums, func(n int) bool { return n > 5 }))
	show("FindLastIndex n<4", lodash.FindLastIndex(nums, func(n int) bool { return n < 4 }))

	show("Every positive", lodash.Every(nums, func(n int) bool { return n > 0 }))
	show("Some >7", lodash.Some(nums, func(n int) bool { return n > 7 }))
	show("None >100", lodash.None(nums, func(n int) bool { return n > 100 }))
	show("Includes 3", lodash.Includes(nums, 3))
	show("IndexOf 3 / LastIndexOf 3", fmt.Sprintf("%d / %d", lodash.IndexOf(nums, 3), lodash.LastIndexOf(nums, 3)))
	show("IndexOfFrom 3 from 4", lodash.IndexOfFrom(nums, 3, 4))
	show("LastIndexOfFrom 8 from 3", lodash.LastIndexOfFrom(nums, 8, 3))

	people := []user{
		{"Ada", 36, "core"},
		{"Grace", 45, "core"},
		{"Linus", 28, "tools"},
		{"Rob", 51, "tools"},
		{"Ken", 45, "core"},
	}

	show("GroupBy team", lodash.GroupBy(people, func(u user) string { return u.Team }))
	show("KeyBy name", lodash.Keys(lodash.KeyBy(people, func(u user) string { return u.Name })))
	show("CountBy team", lodash.CountBy(people, func(u user) string { return u.Team }))
	young, old := lodash.Partition(people, func(u user) bool { return u.Age < 40 })
	show("Partition <40 names", fmt.Sprintf("%v | %v",
		lodash.Map(young, func(u user) string { return u.Name }),
		lodash.Map(old, func(u user) string { return u.Name })))

	show("SortBy age", lodash.Map(lodash.SortBy(people, func(u user) int { return u.Age }),
		func(u user) string { return u.Name }))

	// OrderBy takes comparator funcs + directions, not iteratees.
	byTeamThenAgeDesc := lodash.OrderBy(people,
		[]func(a, b user) int{
			func(a, b user) int { return strings.Compare(a.Team, b.Team) },
			func(a, b user) int { return a.Age - b.Age },
		},
		[]lodash.Order{lodash.Asc, lodash.Desc},
	)
	show("OrderBy team asc/age desc", lodash.Map(byTeamThenAgeDesc,
		func(u user) string { return fmt.Sprintf("%s/%s/%d", u.Team, u.Name, u.Age) }))

	// Map-oriented collection helpers.
	scores := map[string]int{"ada": 91, "grace": 99, "linus": 72}
	show("EveryMap >50", lodash.EveryMap(scores, func(v int, k string) bool { return v > 50 }))
	show("SomeMap >95", lodash.SomeMap(scores, func(v int, k string) bool { return v > 95 }))
	show("NoneMap <0", lodash.NoneMap(scores, func(v int, k string) bool { return v < 0 }))
	show("FilterMap >80 (sorted)", lodash.SortBy(lodash.FilterMap(scores, func(v int, k string) bool { return v > 80 }), lodash.Identity[int]))
	show("MapToSlice (sorted)", lodash.SortBy(lodash.MapToSlice(scores, func(v int, k string) string {
		return fmt.Sprintf("%s=%d", k, v)
	}), lodash.Identity[string]))
	show("ReduceMap total", lodash.ReduceMap(scores, func(acc, v int, k string) int { return acc + v }, 0))
	show("IncludesValue 99", lodash.IncludesValue(scores, 99))
	best, okBest := lodash.MaxByMap(scores, func(v int) int { return v })
	show("MaxByMap", fmt.Sprintf("%d (ok=%t)", best, okBest))
	worst, _ := lodash.MinByMap(scores, func(v int) int { return v })
	show("MinByMap", worst)
	show("GroupByMap parity", lodash.GroupByMap(scores, func(v int) bool { return v%2 == 0 }))
}

func arrays() {
	section("arrays")

	nums := []int{1, 2, 3, 4, 5, 6, 7, 8, 9}

	show("Chunk 4", lodash.Chunk(nums, 4))
	show("Take 3 / TakeRight 3", fmt.Sprintf("%v / %v", lodash.Take(nums, 3), lodash.TakeRight(nums, 3)))
	show("Drop 6 / DropRight 6", fmt.Sprintf("%v / %v", lodash.Drop(nums, 6), lodash.DropRight(nums, 6)))
	show("TakeWhile <4", lodash.TakeWhile(nums, func(n int) bool { return n < 4 }))
	show("TakeRightWhile >7", lodash.TakeRightWhile(nums, func(n int) bool { return n > 7 }))
	show("DropWhile <4", lodash.DropWhile(nums, func(n int) bool { return n < 4 }))
	show("DropRightWhile >7", lodash.DropRightWhile(nums, func(n int) bool { return n > 7 }))

	h, _ := lodash.Head(nums)
	l, _ := lodash.Last(nums)
	n3, _ := lodash.Nth(nums, -2)
	show("Head/Last/Nth(-2)", fmt.Sprintf("%d/%d/%d", h, l, n3))
	show("Initial/Tail", fmt.Sprintf("%v / %v", lodash.Initial(nums), lodash.Tail(nums)))
	show("Slice(2,5) neg Slice(-3,-1)", fmt.Sprintf("%v / %v", lodash.Slice(nums, 2, 5), lodash.Slice(nums, -3, -1)))
	show("Join", lodash.Join(nums, "-"))
	show("Reverse (non-mutating)", fmt.Sprintf("%v, original %v", lodash.Reverse(nums), nums))

	show("Uniq", lodash.Uniq([]int{1, 1, 2, 3, 3, 3}))
	show("UniqBy len", lodash.UniqBy([]string{"a", "bb", "cc", "ddd"}, func(s string) int { return len(s) }))
	show("UniqWith approx eq", lodash.UniqWith([]float64{1.0, 1.05, 2.0}, func(a, b float64) bool {
		d := a - b
		return d < 0.1 && d > -0.1
	}))
	show("SortedUniq", lodash.SortedUniq([]int{1, 1, 2, 2, 3}))
	show("SortedUniqBy", lodash.SortedUniqBy([]int{10, 11, 20, 21}, func(n int) int { return n / 10 }))

	show("Compact", lodash.Compact([]int{0, 1, 0, 2}))
	show("Compact strings", lodash.Compact([]string{"", "a", "", "b"}))

	show("Flatten", lodash.Flatten([][]int{{1, 2}, {3}, nil, {4}}))
	nested := []any{1, []any{2, []any{3, []any{4}}}}
	show("FlattenDeep[int]", lodash.FlattenDeep[int](nested))
	show("FlattenDepth[int] d=1", lodash.FlattenDepth[int](nested, 1))
	show("FlatMap", lodash.FlatMap([]int{1, 2, 3}, func(n int) []int { return []int{n, n * 10} }))
	show("FlatMapDeep[int]", lodash.FlatMapDeep[int]([]any{1, 2}, func(v any) []any {
		return []any{v, []any{v}}
	}))
	show("FlatMapDepth[int] d=1", lodash.FlatMapDepth[int]([]any{1, 2}, func(v any) []any {
		return []any{v, []any{v}}
	}, 1))

	zipped := lodash.Zip([]string{"a", "b", "c"}, []int{1, 2})
	show("Zip (shorter wins)", zipped)
	ls, is := lodash.Unzip(zipped)
	show("Unzip", fmt.Sprintf("%v %v", ls, is))
	show("ZipWith", lodash.ZipWith([]string{"a", "b"}, []int{1, 2}, func(s string, n int) string {
		return s + strconv.Itoa(n)
	}))
	show("UnzipWith", lodash.UnzipWith(zipped, func(s string, n int) string { return fmt.Sprintf("%s:%d", s, n) }))
	show("ZipObject", lodash.ZipObject([]string{"x", "y"}, []int{7, 8}))
	show("ZipObjectDeep", lodash.ZipObjectDeep([]string{"a.b", "a.c[0]"}, []any{1, 2}))

	show("Fill(value,n)", lodash.Fill("x", 3))
	show("FillRange", lodash.FillRange([]int{1, 2, 3, 4, 5}, 0, 1, 4))
	show("Concat", lodash.Concat([]int{1}, []int{2, 3}, nil))

	show("Pull 2,4", lodash.Pull([]int{1, 2, 3, 4, 5}, 2, 4))
	show("PullAll", lodash.PullAll([]int{1, 2, 3}, []int{3}))
	show("PullAllBy mod3", lodash.PullAllBy([]int{1, 2, 3, 4}, []int{1}, func(n int) int { return n % 3 }))
	show("PullAllWith", lodash.PullAllWith([]string{"A", "b", "C"}, []string{"a"}, strings.EqualFold))
	show("PullAt 0,2", lodash.PullAt([]int{1, 2, 3, 4}, 0, 2))
	show("Remove evens", lodash.Remove([]int{1, 2, 3, 4}, func(n int) bool { return n%2 == 0 }))
	show("Without", lodash.Without([]int{1, 2, 3, 2}, 2))

	sorted := []int{10, 20, 30, 30, 40}
	show("SortedIndex 30", lodash.SortedIndex(sorted, 30))
	show("SortedLastIndex 30", lodash.SortedLastIndex(sorted, 30))
	show("SortedIndexOf 30", lodash.SortedIndexOf(sorted, 30))
	show("SortedLastIndexOf 30", lodash.SortedLastIndexOf(sorted, 30))
	show("SortedIndexBy", lodash.SortedIndexBy([]user{{Age: 20}, {Age: 40}}, user{Age: 30}, func(u user) int { return u.Age }))
	show("SortedLastIndexBy", lodash.SortedLastIndexBy([]user{{Age: 20}, {Age: 40}}, user{Age: 40}, func(u user) int { return u.Age }))

	// Randomness is injected, so results are reproducible.
	rng := rand.New(rand.NewSource(42))
	s1, _ := lodash.Sample(nums, rng)
	show("Sample (seed 42)", s1)
	show("SampleN 3", lodash.SampleN(nums, 3, rng))
	show("Shuffle", lodash.Shuffle(nums, rng))
	show("nums unchanged after ops", nums)
}

func setOps() {
	section("set operations")

	a := []int{1, 2, 3, 4}
	b := []int{3, 4, 5}

	show("Difference", lodash.Difference(a, b))
	// NOTE the argument order flip: the *By/*With variants take the iteratee first.
	show("DifferenceBy mod3", lodash.DifferenceBy(func(n int) int { return n % 3 }, a, b))
	show("DifferenceWith", lodash.DifferenceWith(func(x, y int) bool { return x == y }, a, b))
	show("Intersection", lodash.Intersection(a, b))
	show("IntersectionBy", lodash.IntersectionBy(func(n int) int { return n % 3 }, a, b))
	show("IntersectionWith", lodash.IntersectionWith(func(x, y int) bool { return x == y }, a, b))
	show("Union", lodash.Union(a, b))
	show("UnionBy mod3", lodash.UnionBy(func(n int) int { return n % 3 }, a, b))
	show("UnionWith", lodash.UnionWith(func(x, y int) bool { return x == y }, a, b))
	show("Xor", lodash.Xor(a, b))
	show("XorBy mod3", lodash.XorBy(func(n int) int { return n % 3 }, a, b))
	show("XorWith", lodash.XorWith(func(x, y int) bool { return x == y }, a, b))
	show("case-insensitive UnionWith", lodash.UnionWith(strings.EqualFold, []string{"A", "b"}, []string{"a", "C"}))
}

func mathAndNumbers() {
	section("math & numbers")

	nums := []float64{1.5, 2.5, 3.0}
	show("Sum", lodash.Sum(nums))
	show("Mean", lodash.Mean(nums))
	people := []user{{Age: 30}, {Age: 40}, {Age: 50}}
	show("SumBy age", lodash.SumBy(people, func(u user) int { return u.Age }))
	show("MeanBy age", lodash.MeanBy(people, func(u user) int { return u.Age }))

	mn, _ := lodash.Min([]int{4, 1, 9})
	mx, _ := lodash.Max([]int{4, 1, 9})
	show("Min/Max", fmt.Sprintf("%d/%d", mn, mx))
	youngest, _ := lodash.MinBy(people, func(u user) int { return u.Age })
	oldest, _ := lodash.MaxBy(people, func(u user) int { return u.Age })
	show("MinBy/MaxBy age", fmt.Sprintf("%d/%d", youngest.Age, oldest.Age))

	show("Add/Subtract/Multiply/Divide", fmt.Sprintf("%d %d %d %d",
		lodash.Add(6, 4), lodash.Subtract(6, 4), lodash.Multiply(6, 4), lodash.Divide(6, 4)))
	show("Clamp(15,0,10)", lodash.Clamp(15, 0, 10))
	show("Round(4.006,2)", lodash.Round(4.006, 2))
	show("Ceil(6.004,2)", lodash.Ceil(6.004, 2))
	show("Floor(4.006,2)", lodash.Floor(4.006, 2))
	show("Round(-4.5,0)", lodash.Round(-4.5, 0))

	show("Range(5)", lodash.Range(5))
	show("RangeStep(0,10,3)", lodash.RangeStep(0, 10, 3))
	show("RangeStep(4,0,-1)", lodash.RangeStep(4, 0, -1))
	show("RangeRight(5)", lodash.RangeRight(5))
	show("InRange(3,1,5)", lodash.InRange(3, 1, 5))
	show("InRange(7,1,5)", lodash.InRange(7, 1, 5))

	rng := rand.New(rand.NewSource(7))
	show("Random(1,6)", lodash.Random(1, 6, rng))
	show("RandomFloat(0,1)", lodash.RandomFloat(0, 1, rng))
}

func objects() {
	section("objects & maps")

	m := map[string]int{"a": 1, "b": 2, "c": 3}

	show("SortedKeys", lodash.SortedKeys(m))
	show("Values (sorted)", lodash.SortBy(lodash.Values(m), lodash.Identity[int]))
	entries := lodash.Entries(m)
	show("Entries len", len(entries))
	show("FromEntries roundtrip", lodash.SortedKeys(lodash.FromEntries(entries)))
	pairs := lodash.ToPairs(m)
	show("FromPairs roundtrip", lodash.SortedKeys(lodash.FromPairs(pairs)))

	show("Pick a,c", lodash.Pick(m, "a", "c"))
	show("PickBy even", lodash.PickBy(m, func(k string, v int) bool { return v%2 == 0 }))
	show("Omit b", lodash.Omit(m, "b"))
	show("OmitBy even", lodash.OmitBy(m, func(k string, v int) bool { return v%2 == 0 }))
	show("MapKeys upper", lodash.MapKeys(m, func(k string, v int) string { return strings.ToUpper(k) }))
	show("MapValues to string", lodash.MapValues(m, func(k string, v int) string { return strconv.Itoa(v * 10) }))
	show("Invert", lodash.Invert(m))
	show("InvertBy parity", lodash.InvertBy(m, func(v int) bool { return v%2 == 0 }))

	show("Merge", lodash.Merge(map[string]int{"a": 1}, map[string]int{"a": 9, "b": 2}))
	show("MergeWith sum", lodash.MergeWith(func(existing, incoming int) int { return existing + incoming },
		map[string]int{"a": 1}, map[string]int{"a": 9, "b": 2}))
	show("Assign", lodash.Assign(map[string]int{"a": 1}, map[string]int{"b": 2}))
	show("AssignWith max", lodash.AssignWith(map[string]int{"a": 5}, func(dstV, srcV int) int {
		if srcV > dstV {
			return srcV
		}
		return dstV
	}, map[string]int{"a": 3, "b": 4}))
	show("Defaults", lodash.Defaults(map[string]int{"a": 1}, map[string]int{"a": 9, "z": 26}))
	show("DefaultsDeep", lodash.DefaultsDeep(
		map[string]any{"cfg": map[string]any{"host": "local"}},
		map[string]any{"cfg": map[string]any{"host": "remote", "port": 8080}},
	))

	show("Transform to string", lodash.Transform(m, func(acc int, v int, k string) int { return acc + v }, 100))
	var owned []string
	lodash.ForOwn(m, func(k string, v int) { owned = append(owned, k) })
	show("ForOwn key count", len(owned))
	key, okKey := lodash.FindKey(m, func(k string, v int) bool { return v == 2 })
	show("FindKey v==2", fmt.Sprintf("%s (ok=%t)", key, okKey))
	show("Size(map/slice/string)", fmt.Sprintf("%d %d %d",
		lodash.Size(m), lodash.Size([]int{1, 2}), lodash.Size("héllo")))
}

func deepPaths() {
	section("deep paths")

	obj := map[string]any{
		"user": map[string]any{
			"name": "Ada",
			"tags": []any{"math", "eng"},
			"addr": map[string]any{"city": "London"},
		},
	}

	show("ToPath a.b[0].c", lodash.ToPath("a.b[0].c"))
	show("ToPath a['b.c']", lodash.ToPath("a['b.c']"))

	name, ok := lodash.Get(obj, "user.name")
	show("Get user.name", fmt.Sprintf("%v (ok=%t)", name, ok))
	tag, ok := lodash.Get(obj, "user.tags[1]")
	show("Get user.tags[1]", fmt.Sprintf("%v (ok=%t)", tag, ok))
	missing, ok := lodash.Get(obj, "user.missing.deep")
	show("Get missing", fmt.Sprintf("%v (ok=%t)", missing, ok))
	show("GetOr fallback", lodash.GetOr(obj, "user.nope", "n/a"))
	show("Has user.addr.city", lodash.Has(obj, "user.addr.city"))
	show("At two paths", lodash.At(obj, "user.name", "user.addr.city"))

	obj = lodash.Set(obj, "user.addr.zip", "E1")
	show("Set user.addr.zip", lodash.GetOr(obj, "user.addr.zip", nil))
	// HOLE: Set/setPath has no []any case — writing an index into an existing
	// slice replaces the whole slice with a map keyed by the index string,
	// destroying the previous elements. Get() *does* traverse []any, so the
	// read and write paths are asymmetric.
	obj = lodash.Set(obj, "user.tags[2]", "cs")
	show("Set into slice (BROKEN)", lodash.GetOr(obj, "user.tags", nil))
	obj = lodash.Update(obj, "user.name", func(old any) any {
		return strings.ToUpper(fmt.Sprint(old))
	})
	show("Update user.name", lodash.GetOr(obj, "user.name", nil))
	obj, removed := lodash.Unset(obj, "user.addr.zip")
	show("Unset zip", fmt.Sprintf("removed=%t has=%t", removed, lodash.Has(obj, "user.addr.zip")))

	withCustom := lodash.SetWith(map[string]any{}, "a.b.c", 1, func(key string) any {
		return map[string]any{"created_for": key}
	})
	show("SetWith customizer", withCustom)
	updated := lodash.UpdateWith(map[string]any{"n": 1}, "n", func(old any) any {
		return lodash.ToInteger(old) + 41
	}, nil)
	show("UpdateWith", updated)

	// Result invokes a func value stored at the path; plain values pass through.
	withFn := map[string]any{"lazy": func() any { return "computed" }, "plain": 5}
	r1, _ := lodash.Result(withFn, "lazy")
	r2, _ := lodash.Result(withFn, "plain")
	show("Result func/plain", fmt.Sprintf("%v / %v", r1, r2))

	show("Property('user.name')", func() any {
		get := lodash.Property("user.name")
		v, _ := get(obj)
		return v
	}())
	show("PropertyOf(obj)", func() any {
		of := lodash.PropertyOf(obj)
		v, _ := of("user.addr.city")
		return v
	}())
	show("Matches", lodash.Matches(map[string]any{"plain": 5})(withFn))
	show("MatchesProperty", lodash.MatchesProperty("plain", 5)(withFn))
}

func stringsDemo() {
	section("strings")

	show("Words", lodash.Words("fooBar-baz qux_quux XMLHttpRequest"))
	show("CamelCase", lodash.CamelCase("Foo Bar-baz"))
	show("PascalCase", lodash.PascalCase("foo bar baz"))
	show("SnakeCase", lodash.SnakeCase("fooBarBaz"))
	show("KebabCase", lodash.KebabCase("FooBarBaz"))
	show("StartCase", lodash.StartCase("--foo-bar--"))
	show("LowerCase/UpperCase", fmt.Sprintf("%q / %q", lodash.LowerCase("--Foo-Bar--"), lodash.UpperCase("fooBar")))
	show("Capitalize", lodash.Capitalize("FRED"))
	show("UpperFirst/LowerFirst", fmt.Sprintf("%s / %s", lodash.UpperFirst("fred"), lodash.LowerFirst("FRED")))

	show("Pad", fmt.Sprintf("%q", lodash.Pad("abc", 9, "_-")))
	show("PadStart/PadEnd", fmt.Sprintf("%q / %q", lodash.PadStart("7", 3, "0"), lodash.PadEnd("7", 3, ".")))
	show("Truncate", lodash.Truncate("hi-diddly-ho there neighbour", 20, "..."))
	show("Repeat", lodash.Repeat("ab", 3))
	show("Deburr", lodash.Deburr("déjà vu – Œuvre ﬁ"))
	show("Trim/TrimStart/TrimEnd", fmt.Sprintf("%q %q %q",
		lodash.Trim("-_-abc-_-", "-_"), lodash.TrimStart("-_-abc-_-", "-_"), lodash.TrimEnd("-_-abc-_-", "-_")))
	show("Escape", lodash.Escape(`<a href="x">Tom & Jerry</a>`))
	show("Unescape", lodash.Unescape("&lt;p&gt;Tom &amp; Jerry&lt;/p&gt;"))
	show("EscapeRegExp", lodash.EscapeRegExp("[lodash](https://lodash.com/)"))
	show("Replace (first only)", lodash.Replace("a-a-a", "-", "+"))
	show("Split limit 2", lodash.Split("a,b,c", ",", 2))
	show("StartsWith/EndsWith", fmt.Sprintf("%t/%t", lodash.StartsWith("abc", "ab"), lodash.EndsWith("abc", "bc")))

	render := lodash.Template("Hello <%= user.name %>! Raw: <%- user.html %> <% ignored %>done")
	show("Template", render(map[string]any{
		"user": map[string]any{"name": "Ada", "html": "<b>x</b>"},
	}))
	// NOTE: custom delimiters replace only "<%"/"%>", so the "=" interpolation
	// marker is still required — a bare "{{ n }}" is parsed as an (ignored)
	// evaluate block and renders as "".
	custom := lodash.Template("{{= n }}", lodash.TemplateOptions{Open: "{{", Close: "}}"})
	show("Template custom delims", custom(map[string]any{"n": 42}))
	bare := lodash.Template("{{ n }}", lodash.TemplateOptions{Open: "{{", Close: "}}"})
	show("Template bare custom delims", fmt.Sprintf("%q", bare(map[string]any{"n": 42})))

	i64, err := lodash.ParseInt("ff", 16)
	show("ParseInt hex", fmt.Sprintf("%d err=%v", i64, err))
	f, err := lodash.ParseFloat("3.5")
	show("ParseFloat", fmt.Sprintf("%v err=%v", f, err))
}

func langDemo() {
	section("lang & value")

	type nested struct {
		Tags []string
	}
	orig := nested{Tags: []string{"a", "b"}}
	shallow := lodash.Clone(orig)
	deep := lodash.CloneDeep(orig)
	shallow.Tags[0] = "MUTATED"
	show("Clone shares backing array", fmt.Sprintf("orig=%v deep=%v", orig.Tags, deep.Tags))

	show("IsEqual maps", lodash.IsEqual(map[string]int{"a": 1}, map[string]int{"a": 1}))
	show("Eq typed", lodash.Eq(3, 3))
	show("IsEmpty \"\"/0/[]/nil", fmt.Sprintf("%t %t %t %t",
		lodash.IsEmpty(""), lodash.IsEmpty(0), lodash.IsEmpty([]int{}), lodash.IsEmpty(nil)))
	show("IsNil nil/typed-nil/0", fmt.Sprintf("%t %t %t",
		lodash.IsNil(nil), lodash.IsNil((*user)(nil)), lodash.IsNil(0)))
	show("IsMatch", lodash.IsMatch(
		map[string]any{"a": 1, "b": map[string]any{"c": 2, "d": 3}},
		map[string]any{"b": map[string]any{"c": 2}}))
	show("IsPlainObject", fmt.Sprintf("%t %t", lodash.IsPlainObject(map[string]any{}), lodash.IsPlainObject([]int{})))
	show("IsString/IsBool/IsNumber", fmt.Sprintf("%t %t %t",
		lodash.IsString("x"), lodash.IsBool(true), lodash.IsNumber(1.5)))
	show("IsInteger 3 / 3.5", fmt.Sprintf("%t %t", lodash.IsInteger(3), lodash.IsInteger(3.5)))
	show("IsSlice/IsMap", fmt.Sprintf("%t %t", lodash.IsSlice([]int{}), lodash.IsMap(map[string]int{})))
	show("IsError", lodash.IsError(errors.New("boom")))
	show("IsObjectLike", fmt.Sprintf("%t %t", lodash.IsObjectLike(map[string]int{}), lodash.IsObjectLike(3)))
	show("IsNaN/IsFinite", fmt.Sprintf("%t %t", lodash.IsNaN(0.0/floatZero()), lodash.IsFinite(1.0)))
	show("IsFunction/IsRegExp/IsDate", fmt.Sprintf("%t %t %t",
		lodash.IsFunction(lodash.Noop), lodash.IsRegExp("x"), lodash.IsDate(time.Now())))
	show("IsBuffer/IsSafeInteger", fmt.Sprintf("%t %t", lodash.IsBuffer([]byte("x")), lodash.IsSafeInteger(9)))
	show("IsLength/IsNull/IsUndefined", fmt.Sprintf("%t %t %t",
		lodash.IsLength(3), lodash.IsNull(nil), lodash.IsUndefined(nil)))
	show("IsObject/IsArrayLike", fmt.Sprintf("%t %t", lodash.IsObject(map[string]int{}), lodash.IsArrayLike([]int{1})))
	show("IsArrayLikeObject", lodash.IsArrayLikeObject("abc"))
	show("ToLength", lodash.ToLength(-5))

	show("CastArray", lodash.CastArray(1, 2, 3))
	show("ToArray copy", lodash.ToArray([]int{1, 2}))
	show("DefaultTo", fmt.Sprintf("%d %d", lodash.DefaultTo(0, 42), lodash.DefaultTo(7, 42)))
	show("Gt/Gte/Lt/Lte", fmt.Sprintf("%t %t %t %t",
		lodash.Gt(2, 1), lodash.Gte(2, 2), lodash.Lt(1, 2), lodash.Lte(2, 2)))
	show("ToNumber \"3.5\"/true/nil", fmt.Sprintf("%v %v %v",
		lodash.ToNumber("3.5"), lodash.ToNumber(true), lodash.ToNumber(nil)))
	show("ToInteger \"3.9\"", lodash.ToInteger("3.9"))
	show("ToFinite +Inf", lodash.ToFinite(inf()))
	show("ToSafeInteger", lodash.ToSafeInteger(3.99))
	show("ToString slice", lodash.ToString([]int{1, 2}))

	spec := map[string]func(any) bool{
		"age": func(v any) bool { return lodash.ToInteger(v) >= 18 },
	}
	show("Conforms", lodash.Conforms(spec)(map[string]any{"age": 21}))
	show("ConformsTo", lodash.ConformsTo(map[string]any{"age": 4}, spec))
	show("IsEqualWith (len eq)", lodash.IsEqualWith("ab", "cd", func(a, b any) (bool, bool) {
		as, aok := a.(string)
		bs, bok := b.(string)
		if aok && bok {
			return len(as) == len(bs), true
		}
		return false, false
	}))
	show("IsMatchWith", lodash.IsMatchWith(
		map[string]any{"n": 10}, map[string]any{"n": 2},
		func(objValue, srcValue any) bool { return lodash.ToInteger(objValue) > lodash.ToInteger(srcValue) }))
}

func floatZero() float64 { return 0 }

func inf() float64 {
	x := 1.0
	return x / floatZero()
}

func combinators() {
	section("function combinators")

	add := func(a, b int) int { return a + b }
	show("Curry", lodash.Curry(add)(2)(3))
	show("CurryRight", lodash.CurryRight(func(a, b string) string { return a + b })("B")("A"))
	show("Curry3", lodash.Curry3(func(a, b, c int) int { return a * b * c })(2)(3)(4))
	show("CurryRight3", lodash.CurryRight3(func(a, b, c string) string { return a + b + c })("C")("B")("A"))
	show("Curry4", lodash.Curry4(func(a, b, c, d int) int { return a + b + c + d })(1)(2)(3)(4))
	show("Partial", lodash.Partial(add, 10)(5))
	show("PartialRight", lodash.PartialRight(func(a, b string) string { return a + "/" + b }, "suffix")("prefix"))
	show("Flip", lodash.Flip(func(a, b string) string { return a + b })("second", "first"))

	inc := func(n int) int { return n + 1 }
	dbl := func(n int) int { return n * 2 }
	show("Flow inc->dbl", lodash.Flow(inc, dbl)(3))
	show("FlowRight inc<-dbl", lodash.FlowRight(inc, dbl)(3))
	show("Compose", lodash.Compose(inc, dbl)(3))

	classify := lodash.Cond(
		lodash.CondPair[int, string]{Pred: func(n int) bool { return n < 0 }, Result: func(int) string { return "negative" }},
		lodash.CondPair[int, string]{Pred: func(n int) bool { return n == 0 }, Result: func(int) string { return "zero" }},
		lodash.CondPair[int, string]{Pred: func(n int) bool { return n > 0 }, Result: func(int) string { return "positive" }},
	)
	c1, _ := classify(-4)
	c2, _ := classify(0)
	show("Cond", fmt.Sprintf("%s / %s", c1, c2))
	unmatched, matched := lodash.Cond(lodash.CondPair[int, string]{
		Pred: func(n int) bool { return false }, Result: func(int) string { return "never" },
	})(1)
	show("Cond no match", fmt.Sprintf("%q matched=%t", unmatched, matched))

	show("Over", lodash.Over(inc, dbl)(5))
	show("OverEvery", lodash.OverEvery(func(n int) bool { return n > 0 }, func(n int) bool { return n%2 == 0 })(4))
	show("OverSome", lodash.OverSome(func(n int) bool { return n < 0 }, func(n int) bool { return n%2 == 0 })(4))
	show("Negate", lodash.Negate(func(n int) bool { return n%2 == 0 })(3))

	sumAll := func(ns ...int) int { return lodash.Sum(ns) }
	show("OverArgs (double each)", lodash.OverArgs(sumAll, dbl, dbl)(1, 2))
	show("Ary 2", lodash.Ary(sumAll, 2)(1, 2, 3, 4))
	show("Unary", lodash.Unary(sumAll)(9))
	joinAll := func(ss ...string) string { return strings.Join(ss, "") }
	show("Rearg 2,0,1", lodash.Rearg(joinAll, 2, 0, 1)("a", "b", "c"))
	show("Spread", lodash.Spread(sumAll)([]int{1, 2, 3}))
	show("NthArg 1", lodash.NthArg[string](1)("a", "b", "c"))
	show("Wrap", lodash.Wrap("Ada", func(v string, greeting string) string { return greeting + ", " + v })("Hi"))
	show("Rest", lodash.Rest(func(ns []int) int { return len(ns) })(1, 2, 3))
}

func functionUtils() {
	section("function utilities")

	calls := 0
	once := lodash.Once(func() int { calls++; return calls })
	once()
	once()
	show("Once (calls==1)", fmt.Sprintf("value=%d calls=%d", once(), calls))

	memoCalls := 0
	square := lodash.Memoize(func(n int) int { memoCalls++; return n * n })
	square(4)
	square(4)
	show("Memoize", fmt.Sprintf("value=%d underlying calls=%d", square(4), memoCalls))

	byLen := lodash.MemoizeBy(func(s string) int { return len(s) }, func(s string) int { return len(s) })
	show("MemoizeBy", fmt.Sprintf("%d %d", byLen("abc"), byLen("xyz")))

	afterFn := lodash.After(3, func() string { return "fired" })
	var afterResults []string
	for i := 0; i < 4; i++ {
		v, ran := afterFn()
		afterResults = append(afterResults, fmt.Sprintf("%q/%t", v, ran))
	}
	show("After(3)", afterResults)

	beforeFn := lodash.Before(3, func() int { return 1 })
	var beforeResults []int
	for i := 0; i < 4; i++ {
		beforeResults = append(beforeResults, beforeFn())
	}
	show("Before(3)", beforeResults)

	// Debouncer: coalesce a burst into one invocation.
	done := make(chan struct{}, 1)
	d := lodash.NewDebouncer(20*time.Millisecond, func() { done <- struct{}{} })
	for i := 0; i < 5; i++ {
		d.Call()
	}
	select {
	case <-done:
		show("Debouncer fired once", true)
	case <-time.After(time.Second):
		show("Debouncer fired once", "TIMEOUT")
	}
	d2 := lodash.NewDebouncer(time.Hour, func() {})
	show("Debouncer Cancel pending", func() bool { d2.Call(); return d2.Cancel() }())

	throttleRuns := 0
	th := lodash.NewThrottler(time.Hour, func() { throttleRuns++ })
	first := th.Call()
	second := th.Call()
	show("Throttler first/second", fmt.Sprintf("%t/%t runs=%d", first, second, throttleRuns))

	timer := lodash.Delay(func() {}, time.Millisecond)
	show("Delay returns timer", timer != nil)
	deferred := make(chan struct{}, 1)
	lodash.Defer(func() { deferred <- struct{}{} })
	select {
	case <-deferred:
		show("Defer ran", true)
	case <-time.After(time.Second):
		show("Defer ran", "TIMEOUT")
	}

	show("Times", lodash.Times(4, func(i int) int { return i * i }))
	show("Identity/Constant", fmt.Sprintf("%d %d", lodash.Identity(9), lodash.Constant(7)()))
	show("UniqueID", fmt.Sprintf("%s %s", lodash.UniqueID("row_"), lodash.UniqueID("row_")))
	show("Stub*", fmt.Sprintf("%v %v %q %t %t",
		lodash.StubArray[int](), lodash.StubObject(), lodash.StubString(), lodash.StubTrue(), lodash.StubFalse()))

	okVal, err := lodash.Attempt(func() int { return 5 })
	show("Attempt ok", fmt.Sprintf("%d err=%v", okVal, err))
	badVal, err := lodash.Attempt(func() int { panic("kaboom") })
	show("Attempt panic recovered", fmt.Sprintf("%d err=%v", badVal, err))
}

func chaining() {
	section("chaining")

	// Seq: single-value chain.
	res := lodash.Chain(5).Thru(func(n int) int { return n * 2 }).Tap(func(n int) {
		fmt.Printf("%-28s %v\n", "Chain Tap saw:", n)
	}).Value()
	show("Chain Value", res)
	// Type-changing steps are package-level functions.
	asText := lodash.Thru(lodash.Chain(21), func(n int) string { return "n=" + strconv.Itoa(n*2) })
	show("Thru (type change)", asText.Value())
	show("Tap (package-level)", lodash.Tap(lodash.Chain("x"), func(string) {}).Value())

	// SliceSeq: fluent slice pipeline.
	sq := lodash.ChainSlice([]int{5, 1, 4, 2, 3, 9, 9}).
		Filter(func(n int) bool { return n < 9 }).
		Reverse().
		Take(4).
		Concat([]int{100})
	show("SliceSeq Value", sq.Value())
	show("SliceSeq Size/IsEmpty", fmt.Sprintf("%d/%t", sq.Size(), sq.IsEmpty()))
	head, _ := sq.Head()
	last, _ := sq.Last()
	nth, _ := sq.Nth(1)
	show("SliceSeq Head/Last/Nth", fmt.Sprintf("%d/%d/%d", head, last, nth))
	show("SliceSeq Join/Chunk", fmt.Sprintf("%s | %v", sq.Join(","), sq.Chunk(2)))
	show("SliceSeq Drop/Tail/Initial", fmt.Sprintf("%v %v %v",
		sq.Drop(2).Value(), sq.Tail().Value(), sq.Initial().Value()))
	show("SliceSeq Slice(1,3)", sq.Slice(1, 3).Value())
	show("SliceSeq TakeWhile", lodash.ChainSlice([]int{1, 2, 9, 1}).TakeWhile(func(n int) bool { return n < 5 }).Value())
	show("SliceSeq DropRight/While", fmt.Sprintf("%v %v",
		sq.DropRight(1).Value(), sq.DropWhile(func(n int) bool { return n > 2 }).Value()))
	var tapped int
	sq.ForEach(func(int) { tapped++ }).Tap(func(s []int) { tapped += len(s) })
	show("SliceSeq ForEach/Tap", tapped)
	sampled, _ := sq.Shuffle(rand.New(rand.NewSource(1))).Sample(rand.New(rand.NewSource(2)))
	show("SliceSeq Shuffle+Sample", sampled)
	show("SliceSeq Reject/TakeRight", fmt.Sprintf("%v %v",
		sq.Reject(func(n int) bool { return n > 50 }).Value(), sq.TakeRight(2).Value()))

	// SetSeq: set-flavoured pipeline.
	ss := lodash.ChainSet([]int{1, 1, 0, 2, 3}).
		Compact().
		Uniq().
		Union([]int{4}).
		Without(3).
		Filter(func(n int) bool { return n != 99 }).
		Reverse()
	show("SetSeq Value", ss.Value())
	show("SetSeq Includes/IndexOf/Size", fmt.Sprintf("%t/%d/%d", ss.Includes(4), ss.IndexOf(2), ss.Size()))
	show("SetSeq Intersection", ss.Intersection([]int{2, 4}).Value())
	show("SetSeq Difference", ss.Difference([]int{2}).Value())
}

func edgeCases() {
	section("edge cases: empty, nil, single-element")

	var nilSlice []int
	var nilMap map[string]int

	show("Map(nil)", fmt.Sprintf("%v (nil=%t)", lodash.Map(nilSlice, func(n int) int { return n }), lodash.Map(nilSlice, func(n int) int { return n }) == nil))
	show("Filter(nil)", lodash.Filter(nilSlice, func(int) bool { return true }))
	show("Reduce(nil)", lodash.Reduce(nilSlice, func(a, c int) int { return a + c }, 99))
	show("Sum(nil)/Mean(nil)", fmt.Sprintf("%d / %v", lodash.Sum(nilSlice), lodash.Mean(nilSlice)))

	_, okMin := lodash.Min(nilSlice)
	_, okHead := lodash.Head(nilSlice)
	_, okLast := lodash.Last(nilSlice)
	_, okFind := lodash.Find(nilSlice, func(int) bool { return true })
	show("Min/Head/Last/Find on nil", fmt.Sprintf("%t %t %t %t", okMin, okHead, okLast, okFind))

	_, okSample := lodash.Sample(nilSlice, rand.New(rand.NewSource(1)))
	show("Sample(nil) ok", okSample)
	show("SampleN(nil,3)", lodash.SampleN(nilSlice, 3, rand.New(rand.NewSource(1))))
	show("Shuffle(nil)", lodash.Shuffle(nilSlice, rand.New(rand.NewSource(1))))

	show("Chunk(nil,3)", lodash.Chunk(nilSlice, 3))
	// Chunk documents a panic for size < 1 (rather than returning [] as JS lodash does).
	show("Chunk(x,0) panics", func() (msg string) {
		defer func() { msg = fmt.Sprint(recover()) }()
		lodash.Chunk([]int{1, 2}, 0)
		return "no panic"
	}())
	show("Take(nil,5)/Drop(nil,5)", fmt.Sprintf("%v %v", lodash.Take(nilSlice, 5), lodash.Drop(nilSlice, 5)))
	show("Slice out of range", lodash.Slice([]int{1, 2, 3}, 5, 99))
	show("Nth out of range ok", func() bool { _, ok := lodash.Nth([]int{1}, 9); return ok }())
	show("Initial/Tail single", fmt.Sprintf("%v %v", lodash.Initial([]int{1}), lodash.Tail([]int{1})))
	show("Reverse single", lodash.Reverse([]int{1}))
	show("Uniq(nil)/Compact(nil)", fmt.Sprintf("%v %v", lodash.Uniq(nilSlice), lodash.Compact(nilSlice)))
	show("Union() no args", lodash.Union[int]())
	show("Intersection() no args", lodash.Intersection[int]())
	show("Difference(nil,nil)", lodash.Difference(nilSlice, nilSlice))
	show("Xor(single)", lodash.Xor([]int{1, 2}))
	show("Zip(nil,nil)", lodash.Zip(nilSlice, nilSlice))
	show("Join(nil)", fmt.Sprintf("%q", lodash.Join(nilSlice, ",")))
	show("Fill(x,-1)", lodash.Fill(1, -1))
	show("Range(0)", lodash.Range(0))
	// RangeStep also panics on step == 0 (JS lodash returns a filled array).
	show("RangeStep step 0 panics", func() (msg string) {
		defer func() { msg = fmt.Sprint(recover()) }()
		lodash.RangeStep(0, 5, 0)
		return "no panic"
	}())

	show("Keys(nilMap)", lodash.Keys(nilMap))
	show("SortedKeys(nilMap)", lodash.SortedKeys(nilMap))
	show("Pick(nilMap)", lodash.Pick(nilMap, "a"))
	show("Merge(nilMap)", lodash.Merge(nilMap, map[string]int{"a": 1}))
	show("Size(nilMap)/Size(nil)", fmt.Sprintf("%d %d", lodash.Size(nilMap), lodash.Size(nil)))
	show("Get on nil map", func() string {
		v, ok := lodash.Get(nil, "a.b")
		return fmt.Sprintf("%v ok=%t", v, ok)
	}())
	show("Set on nil map", lodash.Set(nil, "a.b", 1))
	show("Has on empty path", lodash.Has(map[string]any{"a": 1}, ""))
	show("ToPath(\"\")", fmt.Sprintf("%v (len=%d)", lodash.ToPath(""), len(lodash.ToPath(""))))

	show("Words(\"\")", fmt.Sprintf("%v (len=%d)", lodash.Words(""), len(lodash.Words(""))))
	show("CamelCase(\"\")", fmt.Sprintf("%q", lodash.CamelCase("")))
	show("Capitalize(\"\")", fmt.Sprintf("%q", lodash.Capitalize("")))
	show("Truncate shorter than len", lodash.Truncate("ab", 10, "..."))
	show("Repeat(x,0)/Repeat(x,-1)", fmt.Sprintf("%q %q", lodash.Repeat("x", 0), lodash.Repeat("x", -1)))
	show("PadStart already long", lodash.PadStart("abcd", 2, "0"))
	show("Split limit 0", fmt.Sprintf("%v (len=%d)", lodash.Split("a,b", ",", 0), len(lodash.Split("a,b", ",", 0))))
	show("Template missing key", fmt.Sprintf("%q", lodash.Template("<%= nope %>")(nil)))

	show("IsEqual(nil,nil)", lodash.IsEqual(nil, nil))
	show("IsString(nil) no panic", lodash.IsString(nil))
	show("ToNumber(\"abc\")", lodash.ToNumber("abc"))
	show("ToString(nil)", fmt.Sprintf("%q", lodash.ToString(nil)))
	show("Clone(nilSlice)", lodash.Clone(nilSlice))
	show("CloneDeep(nilMap)", lodash.CloneDeep(nilMap))
	show("Flow() identity", lodash.Flow[int]()(7))
	show("Over() no fns", lodash.Over[int, int]()(1))
	show("CastArray()", lodash.CastArray[int]())

	// HOLE: lodash.Sample / SampleN / Shuffle dereference the *rand.Rand
	// argument unconditionally, so passing nil panics instead of falling back
	// to the global source (JS lodash has no such parameter at all).
	//   lodash.Sample([]int{1, 2}, nil) // panics: nil pointer dereference
}
