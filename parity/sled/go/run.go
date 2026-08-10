// Go-side parity runner for github.com/malcolmston/sled.
//
// Speaks the same JSON-Lines protocol as parity/sled/rust/src/main.rs: one JSON
// object per line in, exactly one out, never exiting on a failing case.
//
// Each case is a SCRIPT of operations against a freshly created database in a
// temp directory. The emitted value is
//
//	{"steps": [<result per step>], "trees": [{"name": hex, "entries": [[k,v]..]}]}
//
// All keys, values and tree names are lowercase-hex encoded in JSON.
package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	sled "github.com/malcolmston/sled"
)

// ------------------------------------------------------------------ protocol

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID    string      `json:"id"`
	OK    bool        `json:"ok"`
	Value interface{} `json:"value,omitempty"`
	Error string      `json:"error,omitempty"`
}

type step map[string]interface{}

// ------------------------------------------------------------------- helpers

func hexEnc(b []byte) string { return hex.EncodeToString(b) }

func optHex(b []byte, present bool) interface{} {
	if !present {
		return nil
	}
	return hexEnc(b)
}

func (s step) str(k string) (string, bool) {
	v, ok := s[k]
	if !ok || v == nil {
		return "", false
	}
	sv, ok := v.(string)
	return sv, ok
}

// bytes decodes an optional hex field. A missing/null field yields (nil, false):
// nil is meaningful for compare-and-swap ("absent") so it must be preserved.
func (s step) bytes(k string) ([]byte, bool, error) {
	v, ok := s[k]
	if !ok || v == nil {
		return nil, false, nil
	}
	sv, ok := v.(string)
	if !ok {
		return nil, false, fmt.Errorf("field %s must be a hex string", k)
	}
	b, err := hex.DecodeString(sv)
	if err != nil {
		return nil, false, err
	}
	if b == nil {
		b = make([]byte, 0)
	}
	return b, true, nil
}

func (s step) reqBytes(k string) ([]byte, error) {
	b, ok, err := s.bytes(k)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("missing field %s", k)
	}
	return b, nil
}

func (s step) boolean(k string, dflt bool) bool {
	v, ok := s[k]
	if !ok || v == nil {
		return dflt
	}
	bv, ok := v.(bool)
	if !ok {
		return dflt
	}
	return bv
}

func (s step) number(k string, dflt uint64) uint64 {
	v, ok := s[k]
	if !ok || v == nil {
		return dflt
	}
	fv, ok := v.(float64)
	if !ok {
		return dflt
	}
	return uint64(fv)
}

func pair(k, v []byte, ok bool) interface{} {
	if !ok {
		return nil
	}
	return []interface{}{hexEnc(k), hexEnc(v)}
}

// ------------------------------------------------------------ merge operators

// Must behave identically to merge_apply in the Rust runner.
func mergeApply(name string, old, merged []byte) []byte {
	switch name {
	case "concat":
		out := make([]byte, 0, len(old)+len(merged))
		out = append(out, old...)
		out = append(out, merged...)
		return out
	case "sum_u64":
		sum := be64(old) + be64(merged)
		out := make([]byte, 8)
		for i := 7; i >= 0; i-- {
			out[i] = byte(sum & 0xff)
			sum >>= 8
		}
		return out
	case "replace_or_delete":
		if len(merged) == 0 {
			return nil
		}
		return append([]byte(nil), merged...)
	default:
		return append([]byte(nil), merged...)
	}
}

func be64(b []byte) uint64 {
	n := len(b)
	if n > 8 {
		b = b[n-8:]
		n = 8
	}
	var out uint64
	for _, c := range b {
		out = out<<8 | uint64(c)
	}
	return out
}

// ---------------------------------------------------------------- runner state

type state struct {
	dir      string
	path     string
	db       *sled.DB
	mergeOps map[string]string
	lastID   uint64
	haveID   bool
}

func openState(dir string) (*state, error) {
	// NOTE: the Go port's Open takes a FILE path (it is an in-memory treap plus
	// a write-ahead log file). Upstream sled takes a DIRECTORY. Recorded as an
	// API divergence in COVERAGE.md.
	path := filepath.Join(dir, "db.sled")
	db, err := sled.Open(path)
	if err != nil {
		return nil, err
	}
	return &state{dir: dir, path: path, db: db, mergeOps: map[string]string{}}, nil
}

func (st *state) tree(name string) (*sled.Tree, error) {
	if name == "" {
		name = sled.DefaultTreeName
	}
	return st.db.OpenTree(name)
}

func (st *state) installMerge(treeName, op string) error {
	t, err := st.tree(treeName)
	if err != nil {
		return err
	}
	which := op
	t.SetMergeOperator(func(key, oldValue, mergeValue []byte) []byte {
		return mergeApply(which, oldValue, mergeValue)
	})
	return nil
}

func (st *state) reopen() error {
	if err := st.db.Close(); err != nil {
		return err
	}
	db, err := sled.Open(st.path)
	if err != nil {
		return err
	}
	st.db = db
	for name, op := range st.mergeOps {
		if err := st.installMerge(name, op); err != nil {
			return err
		}
	}
	return nil
}

func (st *state) dump() (interface{}, error) {
	names := st.db.TreeNames()
	sort.Slice(names, func(i, j int) bool { return hexEnc([]byte(names[i])) < hexEnc([]byte(names[j])) })
	trees := make([]interface{}, 0, len(names))
	for _, n := range names {
		t, err := st.tree(n)
		if err != nil {
			return nil, err
		}
		entries := []interface{}{}
		for it := t.Scan(sled.Range{}); it.Valid(); it.Next() {
			entries = append(entries, []interface{}{hexEnc(it.Key()), hexEnc(it.Value())})
		}
		trees = append(trees, map[string]interface{}{
			"name":    hexEnc([]byte(n)),
			"entries": entries,
		})
	}
	return trees, nil
}

// ---------------------------------------------------------------- range bounds

var errExclLower = errors.New("port cannot express an exclusive lower bound")
var errInclUpper = errors.New("port cannot express an inclusive upper bound")

func rangeOf(s step) (sled.Range, error) {
	var r sled.Range
	lo, haveLo, err := s.bytes("lower")
	if err != nil {
		return r, err
	}
	hi, haveHi, err := s.bytes("upper")
	if err != nil {
		return r, err
	}
	if haveLo {
		if !s.boolean("lowerIncl", true) {
			return r, errExclLower
		}
		r.Lower = lo
	}
	if haveHi {
		if s.boolean("upperIncl", false) {
			return r, errInclUpper
		}
		r.Upper = hi
	}
	return r, nil
}

func collect(it *sled.Iterator) interface{} {
	out := []interface{}{}
	for ; it.Valid(); it.Next() {
		out = append(out, []interface{}{hexEnc(it.Key()), hexEnc(it.Value())})
	}
	return out
}

// -------------------------------------------------------------------- one step

func runStep(st *state, s step) (result interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			result, err = nil, fmt.Errorf("panic: %v", r)
		}
	}()

	op, _ := s.str("op")
	tname, _ := s.str("tree")

	switch op {
	// ---- single-key writes and reads --------------------------------
	case "insert":
		t, err := st.tree(tname)
		if err != nil {
			return nil, err
		}
		k, err := s.reqBytes("key")
		if err != nil {
			return nil, err
		}
		v, err := s.reqBytes("value")
		if err != nil {
			return nil, err
		}
		old, existed, err := t.GetAndSet(k, v)
		if err != nil {
			return nil, err
		}
		return optHex(old, existed), nil
	case "get":
		t, err := st.tree(tname)
		if err != nil {
			return nil, err
		}
		k, err := s.reqBytes("key")
		if err != nil {
			return nil, err
		}
		v, ok, err := t.Get(k)
		if err != nil {
			return nil, err
		}
		return optHex(v, ok), nil
	case "remove":
		t, err := st.tree(tname)
		if err != nil {
			return nil, err
		}
		k, err := s.reqBytes("key")
		if err != nil {
			return nil, err
		}
		old, existed, err := t.GetAndDelete(k)
		if err != nil {
			return nil, err
		}
		return optHex(old, existed), nil
	case "contains_key":
		t, err := st.tree(tname)
		if err != nil {
			return nil, err
		}
		k, err := s.reqBytes("key")
		if err != nil {
			return nil, err
		}
		return t.ContainsKey(k)
	case "len":
		t, err := st.tree(tname)
		if err != nil {
			return nil, err
		}
		return t.Len(), nil
	case "is_empty":
		t, err := st.tree(tname)
		if err != nil {
			return nil, err
		}
		return t.IsEmpty(), nil
	case "clear":
		t, err := st.tree(tname)
		if err != nil {
			return nil, err
		}
		return nil, t.Clear()

	// ---- compare_and_swap -------------------------------------------
	case "cas":
		t, err := st.tree(tname)
		if err != nil {
			return nil, err
		}
		k, err := s.reqBytes("key")
		if err != nil {
			return nil, err
		}
		old, _, err := s.bytes("old")
		if err != nil {
			return nil, err
		}
		nv, _, err := s.bytes("new")
		if err != nil {
			return nil, err
		}
		casErr := t.CompareAndSwapErr(k, old, nv)
		if casErr == nil {
			return map[string]interface{}{"swapped": true, "current": nil, "proposed": nil}, nil
		}
		var ce *sled.CompareAndSwapError
		if errors.As(casErr, &ce) {
			return map[string]interface{}{
				"swapped":  false,
				"current":  optHex(ce.Current, ce.Current != nil),
				"proposed": optHex(ce.Proposed, ce.Proposed != nil),
			}, nil
		}
		return nil, casErr

	// ---- read-modify-write -------------------------------------------
	case "update_and_fetch", "fetch_and_update":
		t, err := st.tree(tname)
		if err != nil {
			return nil, err
		}
		k, err := s.reqBytes("key")
		if err != nil {
			return nil, err
		}
		mode, ok := s.str("mut")
		if !ok {
			mode = "concat"
		}
		arg, _, err := s.bytes("value")
		if err != nil {
			return nil, err
		}
		f := func(old []byte) []byte { return mergeApply(mode, old, arg) }
		if op == "update_and_fetch" {
			nv, err := t.UpdateAndFetch(k, f)
			if err != nil {
				return nil, err
			}
			return optHex(nv, nv != nil), nil
		}
		ov, _, err := t.FetchAndUpdate(k, f)
		if err != nil {
			return nil, err
		}
		return optHex(ov, ov != nil), nil

	// ---- batches ------------------------------------------------------
	case "batch":
		t, err := st.tree(tname)
		if err != nil {
			return nil, err
		}
		raw, ok := s["ops"].([]interface{})
		if !ok {
			return nil, errors.New("batch step missing ops")
		}
		b := st.db.NewBatch()
		for _, sub := range raw {
			m, ok := sub.(map[string]interface{})
			if !ok {
				return nil, errors.New("batch op must be an object")
			}
			ss := step(m)
			sop, _ := ss.str("op")
			k, err := ss.reqBytes("key")
			if err != nil {
				return nil, err
			}
			switch sop {
			case "insert":
				v, err := ss.reqBytes("value")
				if err != nil {
					return nil, err
				}
				b.Set(k, v)
			case "remove":
				b.Delete(k)
			default:
				return nil, fmt.Errorf("unsupported batch op: %s", sop)
			}
		}
		return nil, t.ApplyBatch(b)

	// ---- iteration -----------------------------------------------------
	case "iter":
		t, err := st.tree(tname)
		if err != nil {
			return nil, err
		}
		return collect(t.Scan(sled.Range{Reverse: s.boolean("reverse", false)})), nil
	case "range":
		t, err := st.tree(tname)
		if err != nil {
			return nil, err
		}
		r, err := rangeOf(s)
		if err != nil {
			return nil, err
		}
		r.Reverse = s.boolean("reverse", false)
		return collect(t.Scan(r)), nil
	case "scan_prefix":
		t, err := st.tree(tname)
		if err != nil {
			return nil, err
		}
		p, _, err := s.bytes("prefix")
		if err != nil {
			return nil, err
		}
		if s.boolean("reverse", false) {
			r := sled.PrefixRange(p)
			r.Reverse = true
			return collect(t.Scan(r)), nil
		}
		return collect(t.ScanPrefix(p)), nil

	// ---- ends and neighbours --------------------------------------------
	case "first", "last":
		t, err := st.tree(tname)
		if err != nil {
			return nil, err
		}
		if op == "first" {
			k, v, ok := t.First()
			return pair(k, v, ok), nil
		}
		k, v, ok := t.Last()
		return pair(k, v, ok), nil
	case "get_gt", "get_gte", "get_lt", "get_lte":
		t, err := st.tree(tname)
		if err != nil {
			return nil, err
		}
		key, err := s.reqBytes("key")
		if err != nil {
			return nil, err
		}
		var k, v []byte
		var ok bool
		switch op {
		case "get_gt":
			k, v, ok = t.GetGt(key)
		case "get_gte":
			k, v, ok = t.GetGte(key)
		case "get_lt":
			k, v, ok = t.GetLt(key)
		case "get_lte":
			k, v, ok = t.GetLte(key)
		}
		return pair(k, v, ok), nil

	// ---- pop family ------------------------------------------------------
	case "pop_min", "pop_max":
		t, err := st.tree(tname)
		if err != nil {
			return nil, err
		}
		var k, v []byte
		var ok bool
		if op == "pop_min" {
			k, v, ok, err = t.PopMin()
		} else {
			k, v, ok, err = t.PopMax()
		}
		if err != nil {
			return nil, err
		}
		return pair(k, v, ok), nil
	case "pop_min_in_range", "pop_max_in_range":
		t, err := st.tree(tname)
		if err != nil {
			return nil, err
		}
		r, err := rangeOf(s)
		if err != nil {
			return nil, err
		}
		var k, v []byte
		var ok bool
		if op == "pop_min_in_range" {
			k, v, ok, err = t.PopMinInRange(r)
		} else {
			k, v, ok, err = t.PopMaxInRange(r)
		}
		if err != nil {
			return nil, err
		}
		return pair(k, v, ok), nil

	// ---- trees -------------------------------------------------------------
	case "open_tree":
		name, _ := s.str("name")
		t, err := st.db.OpenTree(name)
		if err != nil {
			return nil, err
		}
		return hexEnc([]byte(t.Name())), nil
	case "drop_tree":
		name, _ := s.str("name")
		return st.db.DropTree(name)
	case "tree_names":
		names := st.db.TreeNames()
		out := make([]string, 0, len(names))
		for _, n := range names {
			out = append(out, hexEnc([]byte(n)))
		}
		sort.Strings(out)
		return out, nil
	case "tree_name":
		t, err := st.tree(tname)
		if err != nil {
			return nil, err
		}
		return hexEnc([]byte(t.Name())), nil

	// ---- merge operators ---------------------------------------------------
	case "set_merge_operator":
		which, ok := s.str("merge")
		if !ok {
			return nil, errors.New("set_merge_operator missing merge")
		}
		if err := st.installMerge(tname, which); err != nil {
			return nil, err
		}
		st.mergeOps[tname] = which
		return nil, nil
	case "merge":
		t, err := st.tree(tname)
		if err != nil {
			return nil, err
		}
		k, err := s.reqBytes("key")
		if err != nil {
			return nil, err
		}
		v, err := s.reqBytes("value")
		if err != nil {
			return nil, err
		}
		out, err := t.Merge(k, v)
		if err != nil {
			return nil, err
		}
		return optHex(out, out != nil), nil

	// ---- ids, durability, lifecycle ----------------------------------------
	case "generate_id_monotonic":
		n := s.number("n", 5)
		for i := uint64(0); i < n; i++ {
			id, err := st.db.GenerateID()
			if err != nil {
				return nil, err
			}
			if st.haveID && id <= st.lastID {
				return false, nil
			}
			st.lastID, st.haveID = id, true
		}
		return true, nil
	case "flush":
		return nil, st.db.Flush()
	case "was_recovered":
		return st.db.WasRecovered(), nil
	case "reopen":
		return nil, st.reopen()
	case "compact":
		return nil, st.db.Compact()
	case "dump":
		return st.dump()
	}
	return nil, fmt.Errorf("unsupported op: %s", op)
}

// -------------------------------------------------------------------- one case

func runCase(steps []step) (interface{}, error) {
	dir, err := os.MkdirTemp("", "sled-parity-go-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	st, err := openState(dir)
	if err != nil {
		return nil, err
	}
	out := make([]interface{}, 0, len(steps))
	for _, s := range steps {
		v, err := runStep(st, s)
		if err != nil {
			out = append(out, map[string]interface{}{"error": true})
			continue
		}
		out = append(out, v)
	}
	trees, err := st.dump()
	if err != nil {
		return nil, err
	}
	_ = st.db.Close()
	return map[string]interface{}{"steps": out, "trees": trees}, nil
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 1<<24)
	out := bufio.NewWriter(os.Stdout)

	emit := func(r response) {
		b, err := json.Marshal(r)
		if err != nil {
			b, _ = json.Marshal(response{ID: r.ID, OK: false, Error: err.Error()})
		}
		out.Write(b)
		out.WriteByte('\n')
		out.Flush()
	}

	for in.Scan() {
		line := in.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			emit(response{ID: "?", OK: false, Error: err.Error()})
			continue
		}
		var steps []step
		if len(req.Args) > 0 {
			if err := json.Unmarshal(req.Args[0], &steps); err != nil {
				emit(response{ID: req.ID, OK: false, Error: err.Error()})
				continue
			}
		}
		v, err := func() (v interface{}, err error) {
			defer func() {
				if r := recover(); r != nil {
					v, err = nil, fmt.Errorf("panic: %v", r)
				}
			}()
			return runCase(steps)
		}()
		if err != nil {
			emit(response{ID: req.ID, OK: false, Error: err.Error()})
			continue
		}
		emit(response{ID: req.ID, OK: true, Value: v})
	}
}
