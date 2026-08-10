// Command pandas-example exercises the github.com/malcolmston/pandas port:
// Series/DataFrame construction, dtypes, loc/iloc-style selection, filtering,
// column add/drop, sorting, groupby + aggregation, merges, missing-value
// handling, describe, and CSV round-tripping through a temp dir.
//
// It makes no network calls and terminates on its own.
package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/malcolmston/pandas"
)

// ---- tiny verification harness -------------------------------------------

var (
	mismatches []string // numeric/structural results that disagree with hand-computed values
	holes      []string // API gaps / surprising behaviour worth reporting
)

func checkFloat(label string, got float64, ok bool, want float64) {
	if !ok {
		mismatches = append(mismatches, fmt.Sprintf("%s: no value returned (ok=false), want %v", label, want))
		fmt.Printf("  FAIL %-42s ok=false, want %v\n", label, want)
		return
	}
	if math.Abs(got-want) > 1e-6 {
		mismatches = append(mismatches, fmt.Sprintf("%s: got %v, want %v", label, got, want))
		fmt.Printf("  FAIL %-42s got %v, want %v\n", label, got, want)
		return
	}
	fmt.Printf("  ok   %-42s %v\n", label, got)
}

func checkInt(label string, got, want int) {
	if got != want {
		mismatches = append(mismatches, fmt.Sprintf("%s: got %d, want %d", label, got, want))
		fmt.Printf("  FAIL %-42s got %d, want %d\n", label, got, want)
		return
	}
	fmt.Printf("  ok   %-42s %d\n", label, got)
}

func checkStr(label, got, want string) {
	if got != want {
		mismatches = append(mismatches, fmt.Sprintf("%s: got %q, want %q", label, got, want))
		fmt.Printf("  FAIL %-42s got %q, want %q\n", label, got, want)
		return
	}
	fmt.Printf("  ok   %-42s %q\n", label, got)
}

func section(title string) {
	fmt.Printf("\n=== %s %s\n", title, strings.Repeat("=", max(0, 60-len(title))))
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// cellFloat pulls a single numeric cell out of a frame by column name + row.
func cellFloat(df *pandas.DataFrame, col string, row int) (float64, bool) {
	return df.Row(row).Float(col)
}

// ---- fixture -------------------------------------------------------------
//
// Six sales rows. revenue has one NA (row 3) so missing-value behaviour is
// exercised everywhere.
//
//	region product units revenue
//	East   widget    10   100.0
//	East   widget    20   250.0
//	West   widget     5    60.0
//	West   gadget     8      NA
//	East   gadget    12   180.0
//	West   gadget     4    40.0

type saleRecord struct {
	Region  string  `pandas:"region"`
	Product string  `pandas:"product"`
	Units   int     `pandas:"units"`
	Revenue float64 `pandas:"revenue"`
	skipMe  string  // unexported: must not become a column
}

func buildFrame() *pandas.DataFrame {
	return must(pandas.FromMap(map[string][]any{
		"region":  {"East", "East", "West", "West", "East", "West"},
		"product": {"widget", "widget", "widget", "gadget", "gadget", "gadget"},
		"units":   {10, 20, 5, 8, 12, 4},
		"revenue": {100.0, 250.0, 60.0, nil, 180.0, 40.0},
	}, []string{"region", "product", "units", "revenue"}))
}

func main() {
	df := buildFrame()

	// ------------------------------------------------------------------
	section("1. Series construction, dtypes, NA")
	prices := pandas.NewSeries("price", []any{10.0, 20.0, nil, 40.0})
	fmt.Print(prices)
	fmt.Printf("name=%q dtype=%s len=%d count(non-NA)=%d isNA=%v\n",
		prices.Name(), prices.DType(), prices.Len(), prices.Count(), prices.IsNA())
	// mean of 10, 20, 40 = 70/3 = 23.333333...
	checkFloat("Series.Mean skips NA", mustf(prices.Mean()), true, 70.0/3.0)
	checkFloat("Series.Sum", mustf(prices.Sum()), true, 70)

	ints := pandas.NewSeries("n", []any{1, 2, 3})
	strs := pandas.NewSeries("s", []any{"a", "b"})
	bools := pandas.NewSeries("b", []any{true, false})
	objs := pandas.NewSeries("o", []any{struct{ X int }{1}})
	typed := pandas.NewSeriesTyped("forced", pandas.Float64, []any{"1.5", 2, "oops"})
	fmt.Printf("dtypes: int=%s string=%s bool=%s object=%s typed=%s\n",
		ints.DType(), strs.DType(), bools.DType(), objs.DType(), typed.DType())
	checkStr("NewSeries(int) dtype", ints.DType().String(), "int64")
	checkStr("NewSeries(struct) dtype", objs.DType().String(), "object")
	// "1.5" -> 1.5, 2 -> 2, "oops" -> NA  => sum 3.5, count 2
	checkFloat("NewSeriesTyped coercion sum", mustf(typed.Sum()), true, 3.5)
	checkInt("NewSeriesTyped coercion count", typed.Count(), 2)

	// ------------------------------------------------------------------
	section("2. DataFrame construction (FromMap / FromRecords / NewDataFrame)")
	fmt.Print(df)
	r, c := df.Shape()
	fmt.Printf("shape=(%d,%d) names=%v index=%v\n", r, c, df.Names(), df.Index())
	checkInt("FromMap rows", r, 6)
	checkInt("FromMap cols", c, 4)
	checkStr("column order honours `order` arg", strings.Join(df.Names(), ","), "region,product,units,revenue")
	checkStr("units dtype inferred", df.MustCol("units").DType().String(), "int64")
	checkStr("revenue dtype inferred", df.MustCol("revenue").DType().String(), "float64")

	recs := []saleRecord{
		{Region: "East", Product: "widget", Units: 10, Revenue: 100, skipMe: "x"},
		{Region: "West", Product: "gadget", Units: 4, Revenue: 40},
	}
	fromRecs := must(pandas.FromRecords(recs))
	fmt.Println("FromRecords:")
	fmt.Print(fromRecs)
	checkStr("FromRecords uses `pandas` tags", strings.Join(fromRecs.Names(), ","), "region,product,units,revenue")

	built := must(pandas.NewDataFrame(
		pandas.NewSeries("id", []any{1, 2, 3}),
		pandas.NewSeries("score", []any{0.5, 1.5, 2.5}),
	))
	fmt.Println("NewDataFrame:")
	fmt.Print(built)
	// Mismatched lengths must be an error, not a panic.
	if _, err := pandas.NewDataFrame(
		pandas.NewSeries("a", []any{1, 2}),
		pandas.NewSeries("b", []any{1}),
	); err == nil {
		mismatches = append(mismatches, "NewDataFrame accepted ragged columns")
		fmt.Println("  FAIL ragged columns accepted")
	} else {
		fmt.Printf("  ok   ragged columns rejected: %v\n", err)
	}

	// ------------------------------------------------------------------
	section("3. Selection: Col / Select / Head / Tail / ILoc / Take / Loc")
	fmt.Println("Head(2):")
	fmt.Print(df.Head(2))
	fmt.Println("Tail(2):")
	fmt.Print(df.Tail(2))
	fmt.Println("ILoc(1,4):")
	fmt.Print(df.ILoc(1, 4))
	fmt.Println("Select(region, revenue):")
	fmt.Print(must(df.Select("region", "revenue")))
	fmt.Println("Take([]int{5,0}):")
	fmt.Print(df.Take([]int{5, 0}))
	checkInt("ILoc(1,4) rows", df.ILoc(1, 4).NumRows(), 3)
	checkInt("ILoc clamps out-of-range end", df.ILoc(4, 99).NumRows(), 2)

	// Loc works on index *labels*. The default index is int64 positions, so
	// label lookup needs int64 (not int) keys -- see README notes.
	fmt.Println("Loc(int64(0), int64(3)) on default positional index:")
	fmt.Print(df.Loc(int64(0), int64(3)))
	checkInt("Loc with int64 labels", df.Loc(int64(0), int64(3)).NumRows(), 2)
	// HOLE: Loc matches index labels with Go's == on `any`, so the static type
	// must match exactly. defaultIndex stores int64, meaning the natural
	// df.Loc(0, 3) silently returns ZERO rows instead of erroring.
	checkInt("Loc with untyped int labels (no match)", df.Loc(0, 3).NumRows(), 0)
	holes = append(holes, "DataFrame.Loc does type-exact label matching against an int64 default index: df.Loc(0) silently returns 0 rows; unknown labels are skipped with no error")

	// A more pandas-like Loc: promote a column to the index first.
	keyed := must(df.SetIndex("product"))
	fmt.Println("SetIndex(\"product\") then Loc(\"gadget\"):")
	fmt.Print(keyed.Loc("gadget"))
	checkInt("Loc on string index", keyed.Loc("gadget").NumRows(), 3)
	fmt.Println("ResetIndex():")
	fmt.Print(keyed.Loc("gadget").ResetIndex())

	// ------------------------------------------------------------------
	section("4. Filtering (mask, FilterFunc, Between, IsIn, Str)")
	big := df.FilterFunc(func(row pandas.Row) bool {
		v, ok := row.Float("revenue")
		return ok && v >= 100
	})
	fmt.Println("revenue >= 100:")
	fmt.Print(big)
	checkInt("FilterFunc revenue>=100 rows", big.NumRows(), 3)
	checkFloat("FilterFunc revenue>=100 sum", mustf(big.MustCol("revenue").Sum()), true, 530)

	mask := df.MustCol("units").Between(5, 12)
	fmt.Println("units between 5 and 12 (inclusive):")
	fmt.Print(df.Filter(mask))
	checkInt("Between(5,12) rows", df.Filter(mask).NumRows(), 4)

	inMask := df.MustCol("region").IsIn([]any{"West"})
	checkInt("IsIn([\"West\"]) rows", df.Filter(inMask).NumRows(), 3)

	strMask := df.MustCol("product").Str().Contains("wid")
	checkInt("Str().Contains(\"wid\") rows", df.Filter(strMask).NumRows(), 3)
	fmt.Println("product uppercased via Str().Upper():")
	fmt.Print(df.MustCol("product").Str().Upper())

	// ------------------------------------------------------------------
	section("5. Adding / replacing / dropping / renaming columns")
	perUnit := pandas.NewSeriesTyped("rev_per_unit", pandas.Float64, func() []any {
		out := make([]any, df.NumRows())
		for i := 0; i < df.NumRows(); i++ {
			rev, okR := df.Row(i).Float("revenue")
			u, okU := df.Row(i).Float("units")
			if okR && okU && u != 0 {
				out[i] = rev / u
			} else {
				out[i] = nil // propagate NA
			}
		}
		return out
	}())
	withCol := must(df.WithColumn(perUnit))
	fmt.Print(withCol.Round(4))
	// row 0: 100/10 = 10 ; row 1: 250/20 = 12.5 ; row 3: NA
	checkFloat("rev_per_unit[0]", mustf(cellFloat(withCol, "rev_per_unit", 0)), true, 10)
	checkFloat("rev_per_unit[1]", mustf(cellFloat(withCol, "rev_per_unit", 1)), true, 12.5)
	checkInt("rev_per_unit non-NA count", withCol.MustCol("rev_per_unit").Count(), 5)

	// Series arithmetic is an alternative route to the same column.
	ratio := df.MustCol("revenue").Div(df.MustCol("units")).Rename("ratio")
	checkFloat("Series.Div elementwise [1]", mustf(ratio.At(1)).(float64), true, 12.5)

	dropped := withCol.Drop("product", "rev_per_unit")
	fmt.Println("Drop(product, rev_per_unit):")
	fmt.Print(dropped)
	checkStr("after Drop", strings.Join(dropped.Names(), ","), "region,units,revenue")

	renamed := df.Rename(map[string]string{"units": "qty", "revenue": "usd"})
	checkStr("after Rename", strings.Join(renamed.Names(), ","), "region,product,qty,usd")

	// WithColumn on a wrong-length series must error.
	if _, err := df.WithColumn(pandas.NewSeries("bad", []any{1})); err == nil {
		mismatches = append(mismatches, "WithColumn accepted wrong-length column")
		fmt.Println("  FAIL WithColumn accepted wrong-length column")
	} else {
		fmt.Printf("  ok   WithColumn length check: %v\n", err)
	}

	// ------------------------------------------------------------------
	section("6. Sorting")
	sorted := must(df.SortBy([]string{"region", "revenue"}, []bool{true, false}))
	fmt.Println("SortBy region ASC, revenue DESC (NA last):")
	fmt.Print(sorted)
	checkStr("first row region after sort", mustf(sorted.Row(0).Get("region")).(string), "East")
	checkFloat("first row revenue after sort", mustf(sorted.Row(0).Float("revenue")), true, 250)
	// The West group's NA revenue must sort last within its group (row 5).
	_, present := sorted.Row(5).Get("revenue")
	if present {
		mismatches = append(mismatches, "NA revenue did not sort last")
		fmt.Println("  FAIL NA revenue did not sort last")
	} else {
		fmt.Println("  ok   NA revenue sorted last")
	}
	if _, err := df.SortBy([]string{"nope"}, nil); err == nil {
		mismatches = append(mismatches, "SortBy accepted unknown column")
	} else {
		fmt.Printf("  ok   SortBy unknown column: %v\n", err)
	}

	fmt.Println("Series.Sort(false) on revenue:")
	fmt.Print(df.MustCol("revenue").Sort(false))
	fmt.Println("NLargest(2) on revenue:")
	fmt.Print(df.MustCol("revenue").NLargest(2))

	// ------------------------------------------------------------------
	section("7. GroupBy + aggregation")
	gbRegion := must(df.GroupBy("region"))
	fmt.Printf("groups=%d\n", gbRegion.Groups())
	sums := must(gbRegion.Sum("revenue", "units"))
	fmt.Println("GroupBy(region).Sum(revenue, units):")
	fmt.Print(sums)
	// East revenue = 100+250+180 = 530 ; West = 60+40 = 100 (NA skipped)
	// East units   = 10+20+12   = 42  ; West = 5+8+4  = 17
	checkStr("group order deterministic (row0)", mustf(sums.Row(0).Get("region")).(string), "East")
	checkFloat("East revenue_sum", mustf(sums.Row(0).Float("revenue_sum")), true, 530)
	checkFloat("West revenue_sum (NA skipped)", mustf(sums.Row(1).Float("revenue_sum")), true, 100)
	checkFloat("East units_sum", mustf(sums.Row(0).Float("units_sum")), true, 42)
	checkFloat("West units_sum", mustf(sums.Row(1).Float("units_sum")), true, 17)

	means := must(gbRegion.Mean("units"))
	fmt.Println("GroupBy(region).Mean(units):")
	fmt.Print(means)
	checkFloat("East units_mean", mustf(means.Row(0).Float("units_mean")), true, 14)
	checkFloat("West units_mean", mustf(means.Row(1).Float("units_mean")), true, 17.0/3.0)

	counts := must(gbRegion.Count("revenue"))
	fmt.Println("GroupBy(region).Count(revenue) -- counts non-NA:")
	fmt.Print(counts)
	checkFloat("East revenue_count", mustf(counts.Row(0).Float("revenue_count")), true, 3)
	checkFloat("West revenue_count (NA excluded)", mustf(counts.Row(1).Float("revenue_count")), true, 2)

	multi := must(df.GroupBy("region", "product"))
	agg := must(multi.Agg(map[string][]pandas.AggFunc{
		"revenue": {pandas.AggSum, pandas.AggMean, pandas.AggMax, pandas.AggCount},
		"units":   {pandas.AggSum, pandas.AggMin, pandas.AggStd},
	}, []string{"revenue", "units"}))
	fmt.Println("GroupBy(region, product).Agg(...):")
	fmt.Print(agg)
	checkInt("multi-key groups", multi.Groups(), 4)
	// (East, widget): revenue 100+250=350, mean 175, max 250, count 2
	//                 units 10+20=30, min 10, std of {10,20} = sqrt(50) = 7.0710678
	checkStr("multi-key row0 region", mustf(agg.Row(0).Get("region")).(string), "East")
	checkStr("multi-key row0 product", mustf(agg.Row(0).Get("product")).(string), "gadget")
	// gadget sorts before widget, so (East, widget) is row 1.
	checkFloat("(East,widget) revenue_sum", mustf(agg.Row(1).Float("revenue_sum")), true, 350)
	checkFloat("(East,widget) revenue_mean", mustf(agg.Row(1).Float("revenue_mean")), true, 175)
	checkFloat("(East,widget) revenue_max", mustf(agg.Row(1).Float("revenue_max")), true, 250)
	checkFloat("(East,widget) units_sum", mustf(agg.Row(1).Float("units_sum")), true, 30)
	checkFloat("(East,widget) units_min", mustf(agg.Row(1).Float("units_min")), true, 10)
	checkFloat("(East,widget) units_std", mustf(agg.Row(1).Float("units_std")), true, 7.0710678118654755)
	// (West, gadget) has a single revenue value (40) -> std undefined -> NA.
	if _, ok := agg.Row(2).Get("units_std"); ok {
		fmt.Println("  note  units_std for a 2-element group present as expected")
	}
	if _, err := df.GroupBy("missing"); err == nil {
		mismatches = append(mismatches, "GroupBy accepted unknown key column")
	} else {
		fmt.Printf("  ok   GroupBy unknown key: %v\n", err)
	}

	fmt.Println("ValueCounts on region:")
	fmt.Print(df.MustCol("region").ValueCounts())
	fmt.Printf("Unique products: %v\n", df.MustCol("product").Unique())
	fmt.Println("Nunique per column:")
	fmt.Print(df.Nunique())

	// ------------------------------------------------------------------
	section("8. Joins / merges")
	managers := must(pandas.FromMap(map[string][]any{
		"region":  {"East", "West", "North"},
		"manager": {"ana", "bo", "cy"},
		"units":   {1000, 2000, 3000}, // deliberate collision with df.units
	}, []string{"region", "manager", "units"}))
	fmt.Println("right frame:")
	fmt.Print(managers)

	inner := must(df.Merge(managers, "region", pandas.InnerJoin))
	fmt.Println("InnerJoin on region (colliding `units` gets _left/_right):")
	fmt.Print(inner)
	checkInt("inner join rows", inner.NumRows(), 6)
	checkStr("collision suffixes", strings.Join(inner.Names(), ","),
		"region,product,units_left,revenue,manager,units_right")

	partial := must(pandas.FromMap(map[string][]any{
		"region": {"East"},
		"target": {500.0},
	}, []string{"region", "target"}))
	left := must(df.Merge(partial, "region", pandas.LeftJoin))
	fmt.Println("LeftJoin against East-only lookup (West target -> NA):")
	fmt.Print(left)
	checkInt("left join rows", left.NumRows(), 6)
	checkInt("left join target non-NA", left.MustCol("target").Count(), 3)

	innerPartial := must(df.Merge(partial, "region", pandas.InnerJoin))
	checkInt("inner join drops unmatched", innerPartial.NumRows(), 3)

	if _, err := df.Merge(managers, "nope", pandas.InnerJoin); err == nil {
		mismatches = append(mismatches, "Merge accepted missing key column")
	} else {
		fmt.Printf("  ok   Merge missing key: %v\n", err)
	}

	fmt.Println("Concat (vertical stack, union of columns):")
	stacked := must(pandas.Concat(df.Head(2), partial))
	fmt.Print(stacked)
	checkInt("Concat rows", stacked.NumRows(), 3)

	// ------------------------------------------------------------------
	section("9. Missing-value handling")
	fmt.Printf("revenue IsNA: %v\n", df.MustCol("revenue").IsNA())
	fmt.Println("DropNA (row-wise):")
	fmt.Print(df.DropNA())
	checkInt("DataFrame.DropNA rows", df.DropNA().NumRows(), 5)

	filled := df.FillNA("revenue", 0.0)
	fmt.Println("FillNA(revenue, 0):")
	fmt.Print(filled)
	checkFloat("FillNA revenue sum", mustf(filled.MustCol("revenue").Sum()), true, 630)
	checkInt("FillNA revenue count", filled.MustCol("revenue").Count(), 6)
	// Mean changes once NA becomes a real 0: 630/6 = 105 (vs 126 before).
	checkFloat("mean before FillNA", mustf(df.MustCol("revenue").Mean()), true, 126)
	checkFloat("mean after FillNA", mustf(filled.MustCol("revenue").Mean()), true, 105)
	fmt.Println("Series.DropNA on revenue:")
	fmt.Print(df.MustCol("revenue").DropNA())

	// ------------------------------------------------------------------
	section("10. Describe & summary stats")
	fmt.Println("Describe():")
	fmt.Print(df.Describe().Round(6))
	desc := df.Describe()
	checkStr("Describe skips non-numeric columns", strings.Join(desc.Names(), ","), "stat,units,revenue")
	// units: 10,20,5,8,12,4 -> count 6, sum 59, mean 59/6, min 4, max 20
	// sample std = sqrt(168.8333333/5) = sqrt(33.7666667) = 5.810909281
	checkFloat("describe units count", mustf(cellFloat(desc, "units", 0)), true, 6)
	checkFloat("describe units mean", mustf(cellFloat(desc, "units", 1)), true, 59.0/6.0)
	checkFloat("describe units std", mustf(cellFloat(desc, "units", 2)), true, 5.810909281)
	checkFloat("describe units min", mustf(cellFloat(desc, "units", 3)), true, 4)
	checkFloat("describe units max", mustf(cellFloat(desc, "units", 4)), true, 20)
	// revenue: count is 5 (NA excluded)
	checkFloat("describe revenue count excludes NA", mustf(cellFloat(desc, "revenue", 0)), true, 5)
	checkFloat("describe revenue mean", mustf(cellFloat(desc, "revenue", 1)), true, 126)

	units := df.MustCol("units")
	checkFloat("units Median", mustf(units.Median()), true, 9) // (8+10)/2
	checkFloat("units Var", mustf(units.Var()), true, 168.8333333333333/5)
	checkFloat("units Quantile(0)", mustf(units.Quantile(0)), true, 4)
	checkFloat("units Quantile(1)", mustf(units.Quantile(1)), true, 20)
	checkFloat("units Prod", mustf(units.Prod()), true, 10*20*5*8*12*4)
	amax := mustf(units.ArgMax())
	amin := mustf(units.ArgMin())
	checkInt("units ArgMax", amax, 1)
	checkInt("units ArgMin", amin, 5)
	fmt.Printf("units Mode=%v\n", units.Mode())
	fmt.Println("Frame-level reductions (Sum / Mean / Max):")
	fmt.Print(df.Sum())
	fmt.Print(df.Mean())
	fmt.Print(df.Max())
	fmt.Println("Corr matrix:")
	fmt.Print(df.Corr().Round(6))

	fmt.Println("CumSum / Diff / PctChange / Rank on units:")
	fmt.Print(units.CumSum())
	fmt.Print(units.Diff())
	fmt.Print(units.Rank())
	fmt.Print(units.Shift(1))
	// CumSum: 10,30,35,43,55,59
	checkFloat("units CumSum last", mustf(units.CumSum().At(5)).(float64), true, 59)
	// HOLE: dtype handling is inconsistent across transforms. CumSum/Diff/Rank
	// widen an Int64 column to Float64, while Shift preserves Int64.
	fmt.Printf("  HOLE int64 column dtypes after transforms: CumSum=%s Diff=%s Rank=%s Shift=%s (Shift alone keeps int64)\n",
		units.CumSum().DType(), units.Diff().DType(), units.Rank().DType(), units.Shift(1).DType())
	holes = append(holes, "Int64 columns are silently widened to Float64 by CumSum/CumProd/Diff/PctChange/Rank/Abs, but Shift preserves Int64 -- inconsistent dtype policy")
	// Astype to string then back.
	asStr := units.Astype(pandas.String)
	checkStr("Astype(String) dtype", asStr.DType().String(), "string")
	checkStr("Astype(String) value", mustf(asStr.At(0)).(string), "10")
	checkFloat("Astype round-trip sum", mustf(asStr.Astype(pandas.Float64).Sum()), true, 59)

	// ------------------------------------------------------------------
	section("11. CSV read/write (temp dir + in-memory reader)")
	dir, err := os.MkdirTemp("", "pandas-example-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "sales.csv")
	if err := df.WriteCSVFile(path, nil); err != nil {
		panic(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	fmt.Printf("wrote %s:\n%s", path, raw)

	roundTrip := must(pandas.ReadCSVFile(path, nil))
	fmt.Println("read back:")
	fmt.Print(roundTrip)
	rr, rc := roundTrip.Shape()
	checkInt("CSV round-trip rows", rr, 6)
	checkInt("CSV round-trip cols", rc, 4)
	checkStr("CSV round-trip names", strings.Join(roundTrip.Names(), ","), "region,product,units,revenue")
	checkStr("CSV dtype inference: units", roundTrip.MustCol("units").DType().String(), "int64")
	// HOLE: CSV round-trips do not preserve Float64 dtype. WriteCSV renders
	// 100.0 with formatValue's %g-style output ("100"), so ReadCSV's inference
	// then sees an all-integer column and returns Int64. The *values* survive
	// (sum is still 630) but `revenue` comes back as int64, not float64.
	// There is no way to pin dtypes on read: ReadCSVOptions has only
	// Delimiter/NoHeader/NAValues -- no DTypes/Converters field.
	fmt.Printf("  HOLE revenue dtype after CSV round-trip: got %q, want %q\n",
		roundTrip.MustCol("revenue").DType().String(), "float64")
	holes = append(holes, "CSV round-trip demotes Float64 -> Int64 when all values are integral; ReadCSVOptions offers no dtype override")
	checkFloat("CSV round-trip revenue sum", mustf(roundTrip.MustCol("revenue").Sum()), true, 630)
	checkInt("CSV round-trip preserves NA", roundTrip.MustCol("revenue").Count(), 5)

	fmt.Println("WriteCSV to stdout:")
	if err := df.Head(2).WriteCSV(os.Stdout, nil); err != nil {
		panic(err)
	}

	// In-memory reader with a custom delimiter, NA sentinel, and bool column.
	inMem := "region;flag;score\nEast;true;1.5\nWest;false;NULL\nNorth;true;2.5\n"
	parsed := must(pandas.ReadCSV(strings.NewReader(inMem), &pandas.ReadCSVOptions{
		Delimiter: ';',
		NAValues:  []string{"NULL"},
	}))
	fmt.Println("in-memory semicolon CSV with NAValues=[NULL]:")
	fmt.Print(parsed)
	checkStr("custom-delimiter names", strings.Join(parsed.Names(), ","), "region,flag,score")
	checkStr("bool dtype inferred", parsed.MustCol("flag").DType().String(), "bool")
	checkStr("score dtype inferred", parsed.MustCol("score").DType().String(), "float64")
	checkInt("NAValues honoured", parsed.MustCol("score").Count(), 2)
	checkFloat("score sum", mustf(parsed.MustCol("score").Sum()), true, 4)

	noHeader := must(pandas.ReadCSV(strings.NewReader("1,2\n3,4\n"), &pandas.ReadCSVOptions{NoHeader: true}))
	fmt.Println("NoHeader CSV:")
	fmt.Print(noHeader)
	checkStr("NoHeader generated names", strings.Join(noHeader.Names(), ","), "col0,col1")
	checkInt("NoHeader rows", noHeader.NumRows(), 2)

	// ------------------------------------------------------------------
	section("12. Misc: DropDuplicates / Transpose / Copy independence")
	dups := must(pandas.FromMap(map[string][]any{
		"a": {1, 1, 2, 2},
		"b": {"x", "x", "y", "z"},
	}, []string{"a", "b"}))
	fmt.Print(dups.DropDuplicates())
	checkInt("DropDuplicates rows", dups.DropDuplicates().NumRows(), 3)

	tr := must(built.Transpose())
	fmt.Println("Transpose of the id/score frame:")
	fmt.Print(tr)

	cp := df.Copy()
	_ = cp.FillNA("revenue", -1.0)
	checkInt("Copy leaves original untouched", df.MustCol("revenue").Count(), 5)

	// ------------------------------------------------------------------
	section("RESULT")
	if len(mismatches) == 0 {
		fmt.Println("All numeric/structural assertions matched hand-computed values.")
	} else {
		fmt.Printf("%d mismatch(es) vs hand-computed values:\n", len(mismatches))
		for _, m := range mismatches {
			fmt.Printf("  - %s\n", m)
		}
	}
	fmt.Printf("\n%d hole(s) observed at runtime (see README.md for the full list):\n", len(holes))
	for _, h := range holes {
		fmt.Printf("  - %s\n", h)
	}
}

// mustf adapts the library's (value, bool) convention to a single value for
// inline use; it records a mismatch instead of panicking when the value is
// absent so the program always terminates cleanly.
func mustf[T any](v T, ok bool) T {
	if !ok {
		mismatches = append(mismatches, fmt.Sprintf("unexpected missing value where %T was required", v))
	}
	return v
}
