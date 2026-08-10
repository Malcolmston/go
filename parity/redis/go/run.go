// Go-side parity runner for github.com/malcolmston/redis.
//
// Speaks the same JSON-Lines protocol as parity/redis/c/run.py: one JSON object
// per line in, exactly one out, never exiting on a failing case.
//
// Every case is a SCRIPT of commands run against a fresh, empty in-process store
// (redis.New()). Commands are dispatched through Store.Do — the same surface a
// RESP client reaches through redis.NewServer — so the comparison is dispatcher
// vs real redis-server, not the typed Go API vs real redis-server.
//
// The emitted value is
//
//	{"steps": [<normalised reply per step>], "dump": [<canonical key dump>]}
//
// Reply -> JSON mapping (identical to the upstream runner):
//
//	int64                -> JSON number
//	string               -> JSON string  (RESP bulk string)
//	nil                  -> JSON null
//	redis.SimpleString   -> {"status": "OK"}
//	[]any                -> JSON array
//	error                -> {"error": true}   (message text is never compared)
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	redis "github.com/malcolmston/redis"
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

type step struct {
	Op    string   `json:"op"`
	Args  []string `json:"args"`
	Ms    int      `json:"ms"`
	Round int64    `json:"round"`
}

var errMarker = map[string]any{"error": true}

// ------------------------------------------------------- shared normalisation
// Kept byte-for-byte equivalent to the Python runner's rules.

// unordered lists the commands whose reply order Redis does not define.
var unordered = map[string]bool{
	"KEYS": true, "SMEMBERS": true, "SINTER": true, "SUNION": true,
	"SDIFF": true, "SRANDMEMBER": true, "SPOP": true, "HKEYS": true,
	"HVALS": true,
}

// paired lists the commands replying with a flat field/value list.
var paired = map[string]bool{"HGETALL": true, "CONFIG": true}

func scanLike(cmd string) bool {
	switch cmd {
	case "SCAN", "HSCAN", "SSCAN", "ZSCAN":
		return true
	}
	return false
}

// toJSON converts a Do reply into the JSON-comparable representation.
func toJSON(v any) (any, error) {
	switch t := v.(type) {
	case nil:
		return nil, nil
	case redis.SimpleString:
		return map[string]any{"status": string(t)}, nil
	case string:
		// The upstream runner refuses to decode a non-UTF-8 bulk string; mirror
		// that so a binary reply is an error on both sides rather than being
		// silently mangled by json.Marshal's U+FFFD substitution.
		if !utf8.ValidString(t) {
			return nil, fmt.Errorf("reply is not valid UTF-8")
		}
		return t, nil
	case int64:
		return t, nil
	case int:
		return int64(t), nil
	case bool:
		return t, nil
	case float64:
		return t, nil
	case []any:
		out := make([]any, 0, len(t))
		for _, e := range t {
			ev, err := toJSON(e)
			if err != nil {
				return nil, err
			}
			out = append(out, ev)
		}
		return out, nil
	case []string:
		out := make([]any, 0, len(t))
		for _, e := range t {
			out = append(out, e)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("cannot encode reply of type %T", v)
	}
}

func sortKey(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

func normalise(cmd string, v any, roundTo int64) (any, error) {
	j, err := toJSON(v)
	if err != nil {
		return nil, err
	}
	cmd = strings.ToUpper(cmd)
	if arr, ok := j.([]any); ok {
		switch {
		case unordered[cmd]:
			sorted := make([]any, len(arr))
			copy(sorted, arr)
			sort.SliceStable(sorted, func(i, k int) bool { return sortKey(sorted[i]) < sortKey(sorted[k]) })
			j = sorted
		case paired[cmd] && len(arr)%2 == 0:
			pairs := make([][]any, 0, len(arr)/2)
			for i := 0; i < len(arr); i += 2 {
				pairs = append(pairs, []any{arr[i], arr[i+1]})
			}
			sort.SliceStable(pairs, func(i, k int) bool { return sortKey(pairs[i]) < sortKey(pairs[k]) })
			flat := make([]any, 0, len(arr))
			for _, p := range pairs {
				flat = append(flat, p[0], p[1])
			}
			j = flat
		case scanLike(cmd) && len(arr) == 2:
			if inner, ok := arr[1].([]any); ok {
				s := make([]any, len(inner))
				copy(s, inner)
				sort.SliceStable(s, func(i, k int) bool { return sortKey(s[i]) < sortKey(s[k]) })
				j = []any{arr[0], s}
			}
		}
	}
	if roundTo > 0 {
		if n, ok := j.(int64); ok && n > 0 {
			j = ((n + roundTo - 1) / roundTo) * roundTo
		}
	}
	return j, nil
}

// ------------------------------------------------------------------ execution

// do runs one command through the dispatcher, turning both errors and panics
// into the error marker.
func do(s *redis.Store, args []string) (out any) {
	defer func() {
		if r := recover(); r != nil {
			out = errMarker
		}
	}()
	v, err := s.Do(args...)
	if err != nil {
		return errMarker
	}
	j, jerr := normalise(args[0], v, 0)
	if jerr != nil {
		return errMarker
	}
	return j
}

// doRaw is do() without normalisation collapsing, used where the caller needs
// the rounding hook.
func doRound(s *redis.Store, args []string, roundTo int64) (out any) {
	defer func() {
		if r := recover(); r != nil {
			out = errMarker
		}
	}()
	v, err := s.Do(args...)
	if err != nil {
		return errMarker
	}
	j, jerr := normalise(args[0], v, roundTo)
	if jerr != nil {
		return errMarker
	}
	return j
}

// statusOf returns the payload of a {"status": ...} reply.
func statusOf(v any) (string, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		return "", false
	}
	s, ok := m["status"].(string)
	return s, ok
}

// dump is the canonical whole-database dump: every key sorted, with type, TTL
// presence and value. It uses only dispatchable commands, exactly as the
// upstream runner does.
func dump(s *redis.Store) []any {
	keysRep := do(s, []string{"KEYS", "*"})
	arr, ok := keysRep.([]any)
	if !ok {
		return []any{errMarker}
	}
	keys := make([]string, 0, len(arr))
	for _, k := range arr {
		if ks, ok := k.(string); ok {
			keys = append(keys, ks)
		}
	}
	sort.Strings(keys)
	out := make([]any, 0, len(keys))
	for _, k := range keys {
		ent := map[string]any{"key": k}
		typ := do(s, []string{"TYPE", k})
		ent["type"] = typ
		switch t := do(s, []string{"TTL", k}).(type) {
		case int64:
			switch t {
			case -1:
				ent["ttl"] = "none"
			case -2:
				ent["ttl"] = "gone"
			default:
				ent["ttl"] = "set"
			}
		default:
			ent["ttl"] = errMarker
		}
		kind, _ := statusOf(typ)
		switch kind {
		case "string":
			ent["value"] = do(s, []string{"GET", k})
		case "list":
			ent["value"] = do(s, []string{"LRANGE", k, "0", "-1"})
		case "hash":
			ent["value"] = do(s, []string{"HGETALL", k})
		case "set":
			ent["value"] = do(s, []string{"SMEMBERS", k})
		case "zset":
			ent["value"] = do(s, []string{"ZRANGE", k, "0", "-1", "WITHSCORES"})
		default:
			ent["value"] = nil
		}
		out = append(out, ent)
	}
	return out
}

func runScript(steps []step) map[string]any {
	s := redis.New() // fresh, empty database per case
	results := make([]any, 0, len(steps))
	for _, st := range steps {
		op := st.Op
		if op == "" {
			op = "cmd"
		}
		switch op {
		case "sleep":
			time.Sleep(time.Duration(st.Ms) * time.Millisecond)
			results = append(results, nil)
		case "cmd":
			if len(st.Args) == 0 {
				results = append(results, errMarker)
				continue
			}
			results = append(results, doRound(s, st.Args, st.Round))
		default:
			results = append(results, errMarker)
		}
	}
	return map[string]any{"steps": results, "dump": dump(s)}
}

// probe reports which of the given command names the RESP dispatch table
// actually carries. It is mechanical: a name is "dispatchable" iff Store.Do does
// not answer with ErrUnknownCommand. This is how COVERAGE.md's reachable-command
// list is derived — never from reading the source or from memory.
func probe(names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		if dispatchable(n) {
			out = append(out, strings.ToUpper(n))
		}
	}
	sort.Strings(out)
	return out
}

func dispatchable(name string) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = true // it reached a handler, which is what "dispatchable" means
		}
	}()
	s := redis.New()
	_, err := s.Do(name)
	return !errors.Is(err, redis.ErrUnknownCommand)
}

// ---------------------------------------------------------------- main loop

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 1<<26)
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()

	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		resp := response{}
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			resp = response{OK: false, Error: "bad request: " + err.Error()}
		} else {
			resp.ID = req.ID
			resp = handle(req)
		}
		b, err := json.Marshal(resp)
		if err != nil {
			b, _ = json.Marshal(response{ID: req.ID, OK: false, Error: "marshal: " + err.Error()})
		}
		out.Write(b)
		out.WriteByte('\n')
		out.Flush()
	}
}

func handle(req request) (resp response) {
	resp.ID = req.ID
	defer func() {
		if r := recover(); r != nil {
			resp = response{ID: req.ID, OK: false, Error: fmt.Sprintf("panic: %v", r)}
		}
	}()
	if req.Fn == "probe" {
		if len(req.Args) != 1 {
			return response{ID: req.ID, OK: false, Error: "probe wants one array argument"}
		}
		var names []string
		if err := json.Unmarshal(req.Args[0], &names); err != nil {
			return response{ID: req.ID, OK: false, Error: "bad probe list: " + err.Error()}
		}
		return response{ID: req.ID, OK: true, Value: probe(names)}
	}
	if req.Fn != "script" || len(req.Args) != 1 {
		return response{ID: req.ID, OK: false, Error: "expected fn=script with one array argument"}
	}
	var steps []step
	if err := json.Unmarshal(req.Args[0], &steps); err != nil {
		return response{ID: req.ID, OK: false, Error: "bad script: " + err.Error()}
	}
	return response{ID: req.ID, OK: true, Value: runScript(steps)}
}
