// Command run is the Go-side parity runner for github.com/malcolmston/pandas.
//
// It speaks JSON Lines on stdio: one request object per line in, exactly one
// response object per line out, in the same order. See ../../HARNESS.md.
//
// The canonical result encoding is identical to python/run.py:
//
//	frame  {"kind":"frame","columns":[names],"dtypes":[canon],"index":[labels],"values":[[row],...]}
//	series {"kind":"series","name":str,"dtype":canon,"index":[labels],"values":[...]}
//	mask   {"kind":"mask","values":[bool,...]}
//	list   {"kind":"list","values":[...]}
//	scalar {"kind":"scalar","value":...}
//	text   {"kind":"text","value":str}
//
// Missing values encode as "__NA__"; non-finite floats as "__INF__",
// "__-INF__" and "__NAN__".
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"

	pd "github.com/malcolmston/pandas"
)

const na = "__NA__"

type request struct {
	ID   string          `json:"id"`
	Fn   string          `json:"fn"`
	Args json.RawMessage `json:"args"`
}

type response struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value any    `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
}

// ---------------------------------------------------------------------------
// encoding
// ---------------------------------------------------------------------------

func canonDType(d pd.DType) string {
	switch d {
	case pd.Float64:
		return "float64"
	case pd.Int64:
		return "int64"
	case pd.String:
		return "string"
	case pd.Bool:
		return "bool"
	default:
		return "object"
	}
}

func canonScalar(v any, present bool) any {
	if !present || v == nil {
		return na
	}
	switch x := v.(type) {
	case float64:
		if math.IsNaN(x) {
			return "__NAN__"
		}
		if math.IsInf(x, 1) {
			return "__INF__"
		}
		if math.IsInf(x, -1) {
			return "__-INF__"
		}
		return x
	case float32:
		return canonScalar(float64(x), true)
	case int:
		return int64(x)
	case int64:
		return x
	case bool:
		return x
	case string:
		return x
	default:
		return fmt.Sprint(x)
	}
}

func canonIndex(idx []any) []any {
	out := make([]any, len(idx))
	for i, v := range idx {
		out[i] = canonScalar(v, v != nil)
	}
	return out
}

func canonSeries(s *pd.Series) map[string]any {
	vals := make([]any, s.Len())
	for i := 0; i < s.Len(); i++ {
		v, ok := s.At(i)
		vals[i] = canonScalar(v, ok)
	}
	return map[string]any{
		"kind":   "series",
		"name":   s.Name(),
		"dtype":  canonDType(s.DType()),
		"index":  canonIndex(s.Index()),
		"values": vals,
	}
}

func canonFrame(df *pd.DataFrame) map[string]any {
	names := df.Names()
	dtypes := make([]any, len(names))
	cols := make([]*pd.Series, len(names))
	for i, n := range names {
		c, ok := df.Col(n)
		if !ok {
			panic("missing column " + n)
		}
		cols[i] = c
		dtypes[i] = canonDType(c.DType())
	}
	rows := make([]any, df.NumRows())
	for r := 0; r < df.NumRows(); r++ {
		row := make([]any, len(cols))
		for c, col := range cols {
			v, ok := col.At(r)
			row[c] = canonScalar(v, ok)
		}
		rows[r] = row
	}
	ns := make([]any, len(names))
	for i, n := range names {
		ns[i] = n
	}
	return map[string]any{
		"kind":    "frame",
		"columns": ns,
		"dtypes":  dtypes,
		"index":   canonIndex(df.Index()),
		"values":  rows,
	}
}

func canonMask(m []bool) map[string]any {
	vals := make([]any, len(m))
	for i, b := range m {
		vals[i] = b
	}
	return map[string]any{"kind": "mask", "values": vals}
}

func canonList(vs []any) map[string]any {
	out := make([]any, len(vs))
	for i, v := range vs {
		out[i] = canonScalar(v, v != nil)
	}
	return map[string]any{"kind": "list", "values": out}
}

func canonScalarResult(v any, present bool) map[string]any {
	return map[string]any{"kind": "scalar", "value": canonScalar(v, present)}
}

func canonText(s string) map[string]any {
	return map[string]any{"kind": "text", "value": s}
}

// ---------------------------------------------------------------------------
// input construction
// ---------------------------------------------------------------------------

type colSpec struct {
	Name   string `json:"name"`
	DType  string `json:"dtype"`
	Values []any  `json:"values"`
}

type tableSpec struct {
	Columns []colSpec `json:"columns"`
}

func dtypeFor(name string) (pd.DType, error) {
	switch name {
	case "int64":
		return pd.Int64, nil
	case "float64":
		return pd.Float64, nil
	case "string":
		return pd.String, nil
	case "bool":
		return pd.Bool, nil
	case "object":
		return pd.Object, nil
	}
	return pd.Object, fmt.Errorf("unknown dtype %q", name)
}

// coerce turns a JSON-decoded value into the Go type the port expects for the
// target dtype, so that both runners are fed the same logical input.
func coerce(dt pd.DType, v any) any {
	if v == nil {
		return nil
	}
	switch dt {
	case pd.Int64:
		if f, ok := v.(float64); ok {
			return int64(f)
		}
	case pd.Float64:
		if f, ok := v.(float64); ok {
			return f
		}
	case pd.String:
		if s, ok := v.(string); ok {
			return s
		}
	case pd.Bool:
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return v
}

func buildSeries(name, dtype string, values []any) (*pd.Series, error) {
	dt, err := dtypeFor(dtype)
	if err != nil {
		return nil, err
	}
	vs := make([]any, len(values))
	for i, v := range values {
		vs[i] = coerce(dt, v)
	}
	return pd.NewSeriesTyped(name, dt, vs), nil
}

func buildFrame(t tableSpec) (*pd.DataFrame, error) {
	cols := make([]*pd.Series, 0, len(t.Columns))
	for _, c := range t.Columns {
		s, err := buildSeries(c.Name, c.DType, c.Values)
		if err != nil {
			return nil, err
		}
		cols = append(cols, s)
	}
	return pd.NewDataFrame(cols...)
}

// coerceJSON narrows a JSON number to int64 when it is integral, so a fill
// value of 0 reaches the port as an integer rather than a float64.
func coerceJSON(v any) any {
	if f, ok := v.(float64); ok && f == math.Trunc(f) && math.Abs(f) < 1<<53 {
		return int64(f)
	}
	return v
}

// ---------------------------------------------------------------------------
// argument shapes
// ---------------------------------------------------------------------------

type args struct {
	// data
	Table tableSpec `json:"table"`
	Right tableSpec `json:"right"`
	Name  string    `json:"name"`
	DType string    `json:"dtype"`
	Vals  []any     `json:"values"`
	Kinds []string  `json:"kinds"`

	// generic
	Op        string            `json:"op"`
	Arg       json.RawMessage   `json:"arg"`
	To        string            `json:"to"`
	Stat      string            `json:"stat"`
	Q         float64           `json:"q"`
	Column    string            `json:"column"`
	Columns   []string          `json:"columns"`
	Order     []string          `json:"order"`
	Keys      []string          `json:"keys"`
	Ascending []bool            `json:"ascending"`
	Mapping   map[string]string `json:"mapping"`
	Mask      []bool            `json:"mask"`
	Positions []int             `json:"positions"`
	Start     int               `json:"start"`
	End       int               `json:"end"`
	N         int               `json:"n"`
	Labels    []any             `json:"labels"`
	LabelKind string            `json:"label_kind"`
	SetIndex  string            `json:"set_index"`
	SortFirst []string          `json:"sort_first"`
	Value     any               `json:"value"`
	Spec      [][2]string       `json:"spec"`
	On        string            `json:"on"`
	How       string            `json:"how"`
	Agg       string            `json:"agg"`
	CSV       string            `json:"csv"`
	Delimiter string            `json:"delimiter"`
	NAValues  []string          `json:"na_values"`
	NoHeader  bool              `json:"no_header"`
}

// leftRight decodes the "left"/"right" arrays used by the arithmetic cases,
// where "right" is an array of values rather than a table.
type arithArgs struct {
	DType string `json:"dtype"`
	Left  []any  `json:"left"`
	Right []any  `json:"right"`
	Op    string `json:"op"`
}

func argInt(raw json.RawMessage) (int, error) {
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil {
		return 0, err
	}
	return int(f), nil
}

func argString(raw json.RawMessage) (string, error) {
	var s string
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	err := json.Unmarshal(raw, &s)
	return s, err
}

func argBool(raw json.RawMessage) (bool, error) {
	var b bool
	err := json.Unmarshal(raw, &b)
	return b, err
}

func argFloats(raw json.RawMessage) ([]float64, error) {
	var f []float64
	err := json.Unmarshal(raw, &f)
	return f, err
}

func argStrings(raw json.RawMessage) ([]string, error) {
	var s []string
	err := json.Unmarshal(raw, &s)
	return s, err
}

func argAnys(raw json.RawMessage) ([]any, error) {
	var v []any
	err := json.Unmarshal(raw, &v)
	return v, err
}

// ---------------------------------------------------------------------------
// dispatch
// ---------------------------------------------------------------------------

var aggByName = map[string]pd.AggFunc{
	"sum":   pd.AggSum,
	"mean":  pd.AggMean,
	"min":   pd.AggMin,
	"max":   pd.AggMax,
	"count": pd.AggCount,
	"std":   pd.AggStd,
}

func dispatch(fn string, raw json.RawMessage) (any, error) {
	if fn == "series_arith" || fn == "series_pair_stat" {
		var a arithArgs
		if err := json.Unmarshal(raw, &a); err != nil {
			return nil, err
		}
		return arith(fn, a)
	}
	var a args
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &a); err != nil {
			return nil, err
		}
	}
	switch fn {

	// ---- construction -------------------------------------------------
	case "series_new":
		s, err := buildSeries(a.Name, a.DType, a.Vals)
		if err != nil {
			return nil, err
		}
		return canonSeries(s), nil

	case "series_infer":
		// "kinds" pins the intended Go type of each element, because JSON
		// cannot distinguish 1 from 1.0 and the two runners must infer from
		// the same logical input.
		vals := make([]any, len(a.Vals))
		for i, v := range a.Vals {
			vals[i] = v
			if i < len(a.Kinds) && v != nil {
				switch a.Kinds[i] {
				case "int":
					vals[i] = int64(v.(float64))
				case "float":
					vals[i] = v.(float64)
				case "str":
					vals[i] = fmt.Sprint(v)
				case "bool":
					vals[i] = v.(bool)
				}
			}
		}
		return canonSeries(pd.NewSeries(a.Name, vals)), nil

	case "series_astype":
		s, err := buildSeries(a.Name, a.DType, a.Vals)
		if err != nil {
			return nil, err
		}
		to, err := dtypeFor(a.To)
		if err != nil {
			return nil, err
		}
		return canonSeries(s.Astype(to)), nil

	case "frame":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		return canonFrame(df), nil

	case "frame_shape":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		r, c := df.Shape()
		return canonList([]any{int64(r), int64(c)}), nil

	case "frame_names":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		vs := make([]any, 0, df.NumCols())
		for _, n := range df.Names() {
			vs = append(vs, n)
		}
		return canonList(vs), nil

	case "frame_index":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		return canonList(df.Index()), nil

	case "frame_dtypes":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		vs := make([]any, 0, df.NumCols())
		for _, n := range df.Names() {
			c, _ := df.Col(n)
			vs = append(vs, canonDType(c.DType()))
		}
		return canonList(vs), nil

	case "frame_from_map":
		data := map[string][]any{}
		for _, c := range a.Table.Columns {
			dt, err := dtypeFor(c.DType)
			if err != nil {
				return nil, err
			}
			vs := make([]any, len(c.Values))
			for i, v := range c.Values {
				vs[i] = coerce(dt, v)
			}
			data[c.Name] = vs
		}
		df, err := pd.FromMap(data, a.Order)
		if err != nil {
			return nil, err
		}
		return canonFrame(df), nil

	// ---- selection ----------------------------------------------------
	case "select":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		out, err := df.Select(a.Columns...)
		if err != nil {
			return nil, err
		}
		return canonFrame(out), nil

	case "col":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		c, ok := df.Col(a.Column)
		if !ok {
			return nil, fmt.Errorf("pandas: no column %q", a.Column)
		}
		return canonSeries(c), nil

	case "iloc":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		return canonFrame(df.ILoc(a.Start, a.End)), nil

	case "loc":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		if a.SetIndex != "" {
			df, err = df.SetIndex(a.SetIndex)
			if err != nil {
				return nil, err
			}
		}
		labels := make([]any, len(a.Labels))
		for i, l := range a.Labels {
			switch a.LabelKind {
			case "int64":
				labels[i] = int64(l.(float64))
			case "int":
				labels[i] = int(l.(float64))
			default:
				labels[i] = l
			}
		}
		return canonFrame(df.Loc(labels...)), nil

	case "head":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		return canonFrame(df.Head(a.N)), nil

	case "tail":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		return canonFrame(df.Tail(a.N)), nil

	case "take":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		return canonFrame(df.Take(a.Positions)), nil

	case "reset_index":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		if len(a.SortFirst) > 0 {
			df, err = df.SortBy(a.SortFirst, nil)
			if err != nil {
				return nil, err
			}
		}
		return canonFrame(df.ResetIndex()), nil

	case "set_index":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		out, err := df.SetIndex(a.Column)
		if err != nil {
			return nil, err
		}
		return canonFrame(out), nil

	// ---- filtering / mutation ----------------------------------------
	case "filter_mask":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		if len(a.Mask) != df.NumRows() {
			return nil, fmt.Errorf("pandas: mask length %d != %d rows", len(a.Mask), df.NumRows())
		}
		return canonFrame(df.Filter(a.Mask)), nil

	case "filter_cmp":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		col := a.Column
		op := a.Op
		want := a.Value
		out := df.FilterFunc(func(r pd.Row) bool {
			v, ok := r.Get(col)
			if !ok {
				return false
			}
			return compare(v, op, want)
		})
		return canonFrame(out), nil

	case "with_column":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		s, err := buildSeries(a.Column, a.DType, a.Vals)
		if err != nil {
			return nil, err
		}
		out, err := df.WithColumn(s)
		if err != nil {
			return nil, err
		}
		return canonFrame(out), nil

	case "drop":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		return canonFrame(df.Drop(a.Columns...)), nil

	case "rename":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		return canonFrame(df.Rename(a.Mapping)), nil

	case "sort_by":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		out, err := df.SortBy(a.Keys, a.Ascending)
		if err != nil {
			return nil, err
		}
		return canonFrame(out), nil

	case "drop_duplicates":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		return canonFrame(df.DropDuplicates()), nil

	case "concat":
		l, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		r, err := buildFrame(a.Right)
		if err != nil {
			return nil, err
		}
		out, err := pd.Concat(l, r)
		if err != nil {
			return nil, err
		}
		return canonFrame(out), nil

	// ---- groupby ------------------------------------------------------
	case "groupby_groups":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		gb, err := df.GroupBy(a.Keys...)
		if err != nil {
			return nil, err
		}
		return canonScalarResult(int64(gb.Groups()), true), nil

	case "groupby_agg":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		gb, err := df.GroupBy(a.Keys...)
		if err != nil {
			return nil, err
		}
		spec := map[string][]pd.AggFunc{}
		var order []string
		for _, p := range a.Spec {
			col, name := p[0], p[1]
			f, ok := aggByName[name]
			if !ok {
				return nil, fmt.Errorf("unknown aggregation %q", name)
			}
			if _, seen := spec[col]; !seen {
				order = append(order, col)
			}
			spec[col] = append(spec[col], f)
		}
		out, err := gb.Agg(spec, order)
		if err != nil {
			return nil, err
		}
		return canonFrame(out), nil

	case "groupby_plain":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		gb, err := df.GroupBy(a.Keys...)
		if err != nil {
			return nil, err
		}
		var out *pd.DataFrame
		switch a.Agg {
		case "sum":
			out, err = gb.Sum(a.Column)
		case "mean":
			out, err = gb.Mean(a.Column)
		case "min":
			out, err = gb.Min(a.Column)
		case "max":
			out, err = gb.Max(a.Column)
		case "count":
			out, err = gb.Count(a.Column)
		case "std":
			out, err = gb.Std(a.Column)
		default:
			return nil, fmt.Errorf("unknown aggregation %q", a.Agg)
		}
		if err != nil {
			return nil, err
		}
		return canonFrame(out), nil

	// ---- merge --------------------------------------------------------
	case "merge":
		l, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		r, err := buildFrame(a.Right)
		if err != nil {
			return nil, err
		}
		var how pd.JoinType
		switch a.How {
		case "inner":
			how = pd.InnerJoin
		case "left":
			how = pd.LeftJoin
		default:
			return nil, fmt.Errorf("pandas: unsupported join type %q (port supports inner and left only)", a.How)
		}
		out, err := l.Merge(r, a.On, how)
		if err != nil {
			return nil, err
		}
		return canonFrame(out), nil

	// ---- missing values ----------------------------------------------
	case "dropna":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		return canonFrame(df.DropNA()), nil

	case "fillna":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		// No guard here on purpose: FillNA cannot report an unknown column,
		// and inventing an error would hide that fact.
		return canonFrame(df.FillNA(a.Column, coerceJSON(a.Value))), nil

	case "series_mask":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		c, ok := df.Col(a.Column)
		if !ok {
			return nil, fmt.Errorf("pandas: no column %q", a.Column)
		}
		switch a.Op {
		case "isna":
			return canonMask(c.IsNA()), nil
		case "notna":
			m := c.IsNA()
			out := make([]bool, len(m))
			for i, b := range m {
				out[i] = !b
			}
			return canonMask(out), nil
		}
		return nil, fmt.Errorf("unknown mask op %q", a.Op)

	case "series_mask_values":
		s, err := buildSeries(a.Name, a.DType, a.Vals)
		if err != nil {
			return nil, err
		}
		switch a.Op {
		case "isin":
			vs, err := argAnys(a.Arg)
			if err != nil {
				return nil, err
			}
			for i, v := range vs {
				vs[i] = coerce(s.DType(), v)
			}
			return canonMask(s.IsIn(vs)), nil
		case "between":
			fs, err := argFloats(a.Arg)
			if err != nil {
				return nil, err
			}
			return canonMask(s.Between(fs[0], fs[1])), nil
		}
		return nil, fmt.Errorf("unknown mask op %q", a.Op)

	// ---- statistics ---------------------------------------------------
	case "describe":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		return canonFrame(df.Describe()), nil

	case "frame_stat":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		var s *pd.Series
		switch a.Stat {
		case "sum":
			s = df.Sum()
		case "mean":
			s = df.Mean()
		case "min":
			s = df.Min()
		case "max":
			s = df.Max()
		case "std":
			s = df.Std()
		case "var":
			s = df.Var()
		case "median":
			s = df.Median()
		case "nunique":
			s = df.Nunique()
		default:
			return nil, fmt.Errorf("unknown frame stat %q", a.Stat)
		}
		return canonSeries(s.Rename("")), nil

	case "frame_stat_name":
		// The name the port gives a column-wise reduction result.
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		var s *pd.Series
		switch a.Stat {
		case "sum":
			s = df.Sum()
		case "mean":
			s = df.Mean()
		default:
			return nil, fmt.Errorf("unknown frame stat %q", a.Stat)
		}
		return canonScalarResult(s.Name(), true), nil

	case "frame_corr":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		return canonFrame(df.Corr()), nil

	case "frame_op":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		switch a.Op {
		case "abs":
			return canonFrame(df.Abs()), nil
		case "round":
			n, err := argInt(a.Arg)
			if err != nil {
				return nil, err
			}
			return canonFrame(df.Round(n)), nil
		case "transpose":
			out, err := df.Transpose()
			if err != nil {
				return nil, err
			}
			return canonFrame(out), nil
		}
		return nil, fmt.Errorf("unknown frame op %q", a.Op)

	case "series_stat":
		s, err := buildSeries(a.Name, a.DType, a.Vals)
		if err != nil {
			return nil, err
		}
		return seriesStat(s, a)

	case "series_op":
		s, err := buildSeries(a.Name, a.DType, a.Vals)
		if err != nil {
			return nil, err
		}
		return seriesOp(s, a)

	case "str_op":
		s, err := buildSeries(a.Name, a.DType, a.Vals)
		if err != nil {
			return nil, err
		}
		return strOp(s, a)

	// ---- csv ----------------------------------------------------------
	case "to_csv":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		var sb strings.Builder
		if err := df.WriteCSV(&sb, nil); err != nil {
			return nil, err
		}
		return canonText(sb.String()), nil

	case "csv_roundtrip":
		df, err := buildFrame(a.Table)
		if err != nil {
			return nil, err
		}
		var sb strings.Builder
		if err := df.WriteCSV(&sb, nil); err != nil {
			return nil, err
		}
		out, err := pd.ReadCSV(strings.NewReader(sb.String()), nil)
		if err != nil {
			return nil, err
		}
		return canonFrame(out), nil

	case "read_csv":
		opts := &pd.ReadCSVOptions{Delimiter: ',', NoHeader: a.NoHeader, NAValues: a.NAValues}
		if a.Delimiter != "" {
			opts.Delimiter = []rune(a.Delimiter)[0]
		}
		out, err := pd.ReadCSV(strings.NewReader(a.CSV), opts)
		if err != nil {
			return nil, err
		}
		return canonFrame(out), nil
	}
	return nil, fmt.Errorf("unknown fn %q", fn)
}

func compare(v any, op string, want any) bool {
	switch x := v.(type) {
	case float64, int64:
		f := toFloat(x)
		w, ok := want.(float64)
		if !ok {
			return false
		}
		switch op {
		case ">":
			return f > w
		case ">=":
			return f >= w
		case "<":
			return f < w
		case "<=":
			return f <= w
		case "==":
			return f == w
		case "!=":
			return f != w
		}
	case string:
		w, ok := want.(string)
		if !ok {
			return false
		}
		switch op {
		case ">":
			return x > w
		case ">=":
			return x >= w
		case "<":
			return x < w
		case "<=":
			return x <= w
		case "==":
			return x == w
		case "!=":
			return x != w
		}
	case bool:
		w, ok := want.(bool)
		if !ok {
			return false
		}
		switch op {
		case "==":
			return x == w
		case "!=":
			return x != w
		}
	}
	return false
}

func toFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int64:
		return float64(x)
	case int:
		return float64(x)
	}
	return math.NaN()
}

func seriesStat(s *pd.Series, a args) (any, error) {
	switch a.Stat {
	case "count":
		return canonScalarResult(int64(s.Count()), true), nil
	case "nunique":
		return canonScalarResult(int64(len(s.Unique())), true), nil
	case "quantile":
		v, ok := s.Quantile(a.Q)
		return canonScalarResult(v, ok), nil
	case "sum":
		v, ok := s.Sum()
		return canonScalarResult(v, ok), nil
	case "mean":
		v, ok := s.Mean()
		return canonScalarResult(v, ok), nil
	case "min":
		v, ok := s.Min()
		return canonScalarResult(v, ok), nil
	case "max":
		v, ok := s.Max()
		return canonScalarResult(v, ok), nil
	case "std":
		v, ok := s.Std()
		return canonScalarResult(v, ok), nil
	case "var":
		v, ok := s.Var()
		return canonScalarResult(v, ok), nil
	case "median":
		v, ok := s.Median()
		return canonScalarResult(v, ok), nil
	case "prod":
		v, ok := s.Prod()
		return canonScalarResult(v, ok), nil
	}
	return nil, fmt.Errorf("unknown stat %q", a.Stat)
}

func seriesOp(s *pd.Series, a args) (any, error) {
	switch a.Op {
	case "dropna":
		return canonSeries(s.DropNA()), nil
	case "fillna":
		var v any
		if err := json.Unmarshal(a.Arg, &v); err != nil {
			return nil, err
		}
		return canonSeries(s.FillNA(coerce(s.DType(), v))), nil
	case "cumsum":
		return canonSeries(s.CumSum()), nil
	case "cummax":
		return canonSeries(s.CumMax()), nil
	case "cummin":
		return canonSeries(s.CumMin()), nil
	case "cumprod":
		return canonSeries(s.CumProd()), nil
	case "diff":
		return canonSeries(s.Diff()), nil
	case "pct_change":
		return canonSeries(s.PctChange()), nil
	case "shift":
		n, err := argInt(a.Arg)
		if err != nil {
			return nil, err
		}
		return canonSeries(s.Shift(n)), nil
	case "rank":
		m, err := argString(a.Arg)
		if err != nil {
			return nil, err
		}
		if m == "" {
			return canonSeries(s.Rank()), nil
		}
		return canonSeries(s.RankBy(m)), nil
	case "round":
		n, err := argInt(a.Arg)
		if err != nil {
			return nil, err
		}
		return canonSeries(s.Round(n)), nil
	case "abs":
		return canonSeries(s.Abs()), nil
	case "clip":
		fs, err := argFloats(a.Arg)
		if err != nil {
			return nil, err
		}
		return canonSeries(s.Clip(fs[0], fs[1])), nil
	case "sort":
		asc, err := argBool(a.Arg)
		if err != nil {
			return nil, err
		}
		return canonSeries(s.Sort(asc)), nil
	case "unique":
		return canonList(s.Unique()), nil
	case "value_counts":
		return canonSeries(s.ValueCounts()), nil
	case "mode":
		m := s.Mode()
		sort.Slice(m, func(i, j int) bool { return fmt.Sprint(m[i]) < fmt.Sprint(m[j]) })
		return canonList(m), nil
	case "nlargest":
		n, err := argInt(a.Arg)
		if err != nil {
			return nil, err
		}
		return canonSeries(s.NLargest(n)), nil
	case "nsmallest":
		n, err := argInt(a.Arg)
		if err != nil {
			return nil, err
		}
		return canonSeries(s.NSmallest(n)), nil
	case "head":
		n, err := argInt(a.Arg)
		if err != nil {
			return nil, err
		}
		return canonSeries(s.Head(n)), nil
	case "tail":
		n, err := argInt(a.Arg)
		if err != nil {
			return nil, err
		}
		return canonSeries(s.Tail(n)), nil
	}
	return nil, fmt.Errorf("unknown series op %q", a.Op)
}

func strOp(s *pd.Series, a args) (any, error) {
	acc := s.Str()
	switch a.Op {
	case "upper":
		return canonSeries(acc.Upper()), nil
	case "lower":
		return canonSeries(acc.Lower()), nil
	case "title":
		return canonSeries(acc.Title()), nil
	case "strip":
		return canonSeries(acc.Strip()), nil
	case "len":
		return canonSeries(acc.Len()), nil
	case "replace":
		ss, err := argStrings(a.Arg)
		if err != nil {
			return nil, err
		}
		return canonSeries(acc.Replace(ss[0], ss[1])), nil
	case "contains":
		v, err := argString(a.Arg)
		if err != nil {
			return nil, err
		}
		return canonMask(acc.Contains(v)), nil
	case "startswith":
		v, err := argString(a.Arg)
		if err != nil {
			return nil, err
		}
		return canonMask(acc.StartsWith(v)), nil
	case "endswith":
		v, err := argString(a.Arg)
		if err != nil {
			return nil, err
		}
		return canonMask(acc.EndsWith(v)), nil
	}
	return nil, fmt.Errorf("unknown str op %q", a.Op)
}

func arith(fn string, a arithArgs) (any, error) {
	l, err := buildSeries("l", a.DType, a.Left)
	if err != nil {
		return nil, err
	}
	r, err := buildSeries("r", a.DType, a.Right)
	if err != nil {
		return nil, err
	}
	if fn == "series_pair_stat" {
		switch a.Op {
		case "corr":
			v, ok := l.Corr(r)
			return canonScalarResult(v, ok), nil
		case "cov":
			v, ok := l.Cov(r)
			return canonScalarResult(v, ok), nil
		}
		return nil, fmt.Errorf("unknown pair stat %q", a.Op)
	}
	var out *pd.Series
	switch a.Op {
	case "add":
		out = l.Add(r)
	case "sub":
		out = l.Sub(r)
	case "mul":
		out = l.Mul(r)
	case "div":
		out = l.Div(r)
	default:
		return nil, fmt.Errorf("unknown arith op %q", a.Op)
	}
	return canonSeries(out.Rename("")), nil
}

// ---------------------------------------------------------------------------
// main loop
// ---------------------------------------------------------------------------

func safeDispatch(fn string, raw json.RawMessage) (v any, err error) {
	defer func() {
		if r := recover(); r != nil {
			v = nil
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return dispatch(fn, raw)
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 1<<24)
	out := bufio.NewWriter(os.Stdout)
	enc := json.NewEncoder(out)
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = enc.Encode(response{ID: "", OK: false, Error: "bad request: " + err.Error()})
			_ = out.Flush()
			continue
		}
		v, err := safeDispatch(req.Fn, req.Args)
		resp := response{ID: req.ID}
		if err != nil {
			resp.OK = false
			resp.Error = err.Error()
			fmt.Fprintf(os.Stderr, "case %s: %v\n", req.ID, err)
		} else {
			resp.OK = true
			resp.Value = v
		}
		if err := enc.Encode(resp); err != nil {
			fmt.Fprintf(os.Stderr, "case %s: encode: %v\n", req.ID, err)
			_ = enc.Encode(response{ID: req.ID, OK: false, Error: "encode: " + err.Error()})
		}
		_ = out.Flush()
	}
}
