// Command run is the Go-side parity runner for github.com/malcolmston/sqlite,
// the pure-Go in-memory SQL engine that registers the "mstsqlite" database/sql
// driver.
//
// It speaks the harness JSON-Lines contract on stdio: one request object per
// line in, exactly one reply object per line out, in the same order, and it
// never exits because a case failed — errors and panics both become ok:false.
//
//	in   {"id":"...","fn":"script","args":[{"setup":[...],"query":"...","params":[...]}]}
//	out  {"id":"...","ok":true,"value":{"columns":[...],"rows":[[[tag,payload],...]]}}
//	out  {"id":"...","ok":false,"error":"..."}
//
// Every case runs against a brand-new anonymous ":memory:" database. The pool is
// pinned with SetMaxOpenConns(1): an anonymous in-memory database in this port
// is private per connection, so a pool of two would hand different statements
// different databases and make the whole comparison meaningless.
//
// Values are emitted as [tag, payload] with tag in {null,int,real,text,blob} so
// that the port's INTEGER-vs-REAL typing is compared explicitly rather than
// being hidden behind string coercion. The tag comes from the concrete Go type
// the driver yields (nil, int64, float64, string, []byte), which is exactly the
// engine's own value type. int payloads are decimal strings (exact for all of
// int64), real payloads are JSON numbers except non-finite ones ("inf", "-inf",
// "nan"), blob payloads are lowercase hex.
//
// A zero-row result reports columns as null, matching the upstream runner: the
// sqlite3 CLI emits no header row when there are no rows.
package main

import (
	"bufio"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	_ "github.com/malcolmston/sqlite" // registers the "mstsqlite" driver
)

// ------------------------------------------------------------------ protocol

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value any    `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
}

// step is one setup entry: either a bare SQL string, {"sql":…,"params":[…]}, or
// {"txn":"begin"|"commit"|"rollback"}.
type step struct {
	SQL    string
	Params []any
	Txn    string
}

func (s *step) UnmarshalJSON(raw []byte) error {
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		s.SQL = str
		return nil
	}
	var obj struct {
		SQL    string `json:"sql"`
		Params []any  `json:"params"`
		Txn    string `json:"txn"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return err
	}
	s.SQL, s.Params, s.Txn = obj.SQL, obj.Params, obj.Txn
	return nil
}

type spec struct {
	Setup  []step `json:"setup"`
	Query  string `json:"query"`
	Params []any  `json:"params"`
}

type result struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

// ------------------------------------------------------------------ execution

// execer is the common surface of *sql.DB and *sql.Tx used here, so setup
// statements run on whichever is currently active.
type execer interface {
	Exec(string, ...any) (sql.Result, error)
	Query(string, ...any) (*sql.Rows, error)
}

// runCase executes one script against a fresh database and returns the final
// SELECT's rows. All errors are returned, never fatal.
func runCase(sp spec) (res *result, err error) {
	defer func() {
		if r := recover(); r != nil {
			res, err = nil, fmt.Errorf("panic: %v", r)
		}
	}()

	db, err := sql.Open("mstsqlite", ":memory:")
	if err != nil {
		return nil, err
	}
	// Required: an anonymous in-memory database is private to each connection.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	defer db.Close()

	var tx *sql.Tx
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()
	active := func() execer {
		if tx != nil {
			return tx
		}
		return db
	}

	for i, st := range sp.Setup {
		switch st.Txn {
		case "":
			args, aerr := bindArgs(st.Params)
			if aerr != nil {
				return nil, fmt.Errorf("setup[%d]: %w", i, aerr)
			}
			if _, eerr := active().Exec(st.SQL, args...); eerr != nil {
				return nil, fmt.Errorf("setup[%d]: %w", i, eerr)
			}
		case "begin":
			if tx != nil {
				return nil, fmt.Errorf("setup[%d]: transaction already open", i)
			}
			t, berr := db.Begin()
			if berr != nil {
				return nil, fmt.Errorf("setup[%d]: %w", i, berr)
			}
			tx = t
		case "commit":
			if tx == nil {
				return nil, fmt.Errorf("setup[%d]: no transaction", i)
			}
			cerr := tx.Commit()
			tx = nil
			if cerr != nil {
				return nil, fmt.Errorf("setup[%d]: %w", i, cerr)
			}
		case "rollback":
			if tx == nil {
				return nil, fmt.Errorf("setup[%d]: no transaction", i)
			}
			rerr := tx.Rollback()
			tx = nil
			if rerr != nil {
				return nil, fmt.Errorf("setup[%d]: %w", i, rerr)
			}
		default:
			return nil, fmt.Errorf("setup[%d]: unknown txn %q", i, st.Txn)
		}
	}

	args, err := bindArgs(sp.Params)
	if err != nil {
		return nil, err
	}
	rows, err := active().Query(sp.Query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	out := &result{Rows: [][]any{}}
	for rows.Next() {
		cells := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range cells {
			ptrs[i] = &cells[i]
		}
		if serr := rows.Scan(ptrs...); serr != nil {
			return nil, serr
		}
		tagged := make([]any, len(cells))
		for i, c := range cells {
			tagged[i] = tag(c)
		}
		out.Rows = append(out.Rows, tagged)
	}
	if rerr := rows.Err(); rerr != nil {
		return nil, rerr
	}
	// The sqlite3 CLI prints no header row for an empty result, so neither do we.
	if len(out.Rows) > 0 {
		out.Columns = cols
	}
	return out, nil
}

// bindArgs converts JSON case parameters into driver arguments. JSON has one
// number type, so an integral float is passed as an int64 — matching how the
// upstream runner renders the same literal.
func bindArgs(params []any) ([]any, error) {
	if len(params) == 0 {
		return nil, nil
	}
	out := make([]any, len(params))
	for i, p := range params {
		switch v := p.(type) {
		case nil:
			out[i] = nil
		case bool:
			if v {
				out[i] = int64(1)
			} else {
				out[i] = int64(0)
			}
		case float64:
			if v == math.Trunc(v) && !math.IsInf(v, 0) && math.Abs(v) < 1e15 {
				out[i] = int64(v)
			} else {
				out[i] = v
			}
		case string:
			out[i] = v
		case map[string]any:
			h, ok := v["blob"].(string)
			if !ok {
				return nil, fmt.Errorf("unsupported parameter %v", v)
			}
			b, err := hex.DecodeString(h)
			if err != nil {
				return nil, fmt.Errorf("bad blob parameter %q: %w", h, err)
			}
			out[i] = b
		default:
			return nil, fmt.Errorf("unsupported parameter %T", p)
		}
	}
	return out, nil
}

// tag renders one scanned cell as [tag, payload].
func tag(c any) []any {
	switch v := c.(type) {
	case nil:
		return []any{"null", nil}
	case int64:
		return []any{"int", strconv.FormatInt(v, 10)}
	case int:
		return []any{"int", strconv.Itoa(v)}
	case float64:
		switch {
		case math.IsInf(v, 1):
			return []any{"real", "inf"}
		case math.IsInf(v, -1):
			return []any{"real", "-inf"}
		case math.IsNaN(v):
			return []any{"real", "nan"}
		}
		return []any{"real", v}
	case string:
		return []any{"text", v}
	case []byte:
		return []any{"blob", hex.EncodeToString(v)}
	case bool:
		if v {
			return []any{"int", "1"}
		}
		return []any{"int", "0"}
	default:
		return []any{"text", fmt.Sprintf("%v", v)}
	}
}

// ---------------------------------------------------------------------- main

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	out := bufio.NewWriter(os.Stdout)

	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			emit(out, response{ID: "", OK: false, Error: "bad request: " + err.Error()})
			continue
		}
		emit(out, handle(req))
	}
}

func handle(req request) response {
	if req.Fn != "script" {
		return response{ID: req.ID, OK: false, Error: "unknown fn " + req.Fn}
	}
	if len(req.Args) != 1 {
		return response{ID: req.ID, OK: false, Error: "script takes exactly one argument"}
	}
	var sp spec
	if err := json.Unmarshal(req.Args[0], &sp); err != nil {
		return response{ID: req.ID, OK: false, Error: "bad spec: " + err.Error()}
	}
	res, err := runCase(sp)
	if err != nil {
		return response{ID: req.ID, OK: false, Error: err.Error()}
	}
	return response{ID: req.ID, OK: true, Value: res}
}

func emit(w *bufio.Writer, r response) {
	enc, err := json.Marshal(r)
	if err != nil {
		enc, _ = json.Marshal(response{ID: r.ID, OK: false, Error: "encode: " + err.Error()})
	}
	w.Write(enc)
	w.WriteByte('\n')
	w.Flush()
}
