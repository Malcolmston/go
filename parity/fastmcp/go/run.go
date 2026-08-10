// Command run is the Go (github.com/malcolmston/fastmcp) side of the fastmcp
// parity harness.
//
// It reads one JSON object per line on stdin and writes exactly one JSON object
// per line on stdout. Each case builds the same MCP server the Python runner
// builds (../python/server_def.py) and answers protocol requests over an
// in-process pipe pair driven through Server.ServeStdio — no sockets, no model
// provider.
//
// Case shapes:
//
//	{"fn": "initialize", "args": [<protocolVersion|null>, "<path>"?]}
//	{"fn": "rpc",        "args": [<request>|[<request>,...], "<path>"?]}
//
// The value is a list of outcomes, one per request:
//
//	{"kind": "result", "result": <normalised JSON-RPC result>}
//	{"kind": "error",  "code": <JSON-RPC error code>}
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/malcolmston/fastmcp"
)

const (
	serverName    = "parity-server"
	serverVersion = "1.0.0"
	instructions  = "A fixed server used to compare MCP wire behaviour across ports."

	// handshakeVersion mirrors the Python runner's HANDSHAKE_VERSION
	// (mcp.types.LATEST_PROTOCOL_VERSION for the pinned upstream).
	handshakeVersion = "2025-11-25"

	caseTimeout = 20 * time.Second
)

// logoBytes is the deterministic 8-byte blob served by res://logo.png. It must
// match LOGO_BYTES in server_def.py exactly.
var logoBytes = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

// ----------------------------------------------------------------- the server

type addArgs struct {
	A int `json:"a" jsonschema:"description=the first addend"`
	B int `json:"b" jsonschema:"description=the second addend"`
}

type greetArgs struct {
	Name     string `json:"name" jsonschema:"description=who to greet"`
	Greeting string `json:"greeting" jsonschema:"description=the greeting word,default=Hello"`
}

type searchArgs struct {
	Query   string   `json:"query" jsonschema:"description=the search query"`
	Limit   int      `json:"limit" jsonschema:"description=maximum hits to return,default=10"`
	Verbose bool     `json:"verbose" jsonschema:"description=include debug detail,default=false"`
	Tags    []string `json:"tags" jsonschema:"description=restrict to these tags,default=[]"`
}

type nicknameArgs struct {
	Name     string  `json:"name" jsonschema:"description=the person's name"`
	Nickname *string `json:"nickname" jsonschema:"description=an optional nickname"`
}

type statsArgs struct {
	Values []float64 `json:"values" jsonschema:"description=the numbers to summarise"`
}

// Stats mirrors the Python pydantic model of the same name.
type Stats struct {
	Count int     `json:"count" jsonschema:"description=how many values were summarised"`
	Total float64 `json:"total" jsonschema:"description=the sum of the values"`
	Mean  float64 `json:"mean" jsonschema:"description=the arithmetic mean"`
}

type failArgs struct {
	Reason string `json:"reason" jsonschema:"description=why this call should fail"`
}

func buildServer() *fastmcp.Server {
	s := fastmcp.New(serverName,
		fastmcp.WithVersion(serverVersion),
		fastmcp.WithInstructions(instructions),
	)

	s.Tool("add", "Add two integers.", func(ctx context.Context, a addArgs) (any, error) {
		return a.A + a.B, nil
	})

	s.Tool("greet", "Greet someone by name.", func(ctx context.Context, a greetArgs) (any, error) {
		return fmt.Sprintf("%s, %s!", a.Greeting, a.Name), nil
	})

	s.Tool("search", "Search with several defaulted options.", func(ctx context.Context, a searchArgs) (any, error) {
		return fmt.Sprintf("%s|%d|%t|%s", a.Query, a.Limit, a.Verbose, strings.Join(a.Tags, ",")), nil
	})

	// The dynamic (map) tool form: upstream declares a single `payload` object
	// argument, so the handler pulls that key out of the raw argument map.
	s.Tool("echo_map", "Echo a free-form JSON object.", func(ctx context.Context, a map[string]any) (any, error) {
		enc, err := json.Marshal(a["payload"])
		if err != nil {
			return nil, err
		}
		return string(enc), nil
	})

	s.Tool("nickname", "Build a label with an optional nickname.", func(ctx context.Context, a nicknameArgs) (any, error) {
		if a.Nickname == nil {
			return a.Name, nil
		}
		return fmt.Sprintf("%s (%s)", a.Name, *a.Nickname), nil
	})

	s.ToolWithOutput("stats", "Summarise a list of numbers.", func(ctx context.Context, a statsArgs) (Stats, error) {
		total := 0.0
		for _, v := range a.Values {
			total += v
		}
		mean := 0.0
		if len(a.Values) > 0 {
			mean = total / float64(len(a.Values))
		}
		return Stats{Count: len(a.Values), Total: total, Mean: mean}, nil
	})

	s.Tool("fail", "Always fail, to exercise tool errors.", func(ctx context.Context, a failArgs) (any, error) {
		return nil, fmt.Errorf("deliberate failure: %s", a.Reason)
	})

	s.Resource("res://greeting", "greeting", "A fixed greeting.", "text/plain",
		func(ctx context.Context) (string, error) { return "hello from the parity server", nil })
	s.Resource("res://data.json", "data", "A fixed JSON document.", "application/json",
		func(ctx context.Context) (string, error) { return `{"answer":42,"kind":"json"}`, nil })
	s.BinaryResource("res://logo.png", "logo", "A fixed binary blob.", "image/png",
		func(ctx context.Context) ([]byte, error) { return logoBytes, nil })
	s.ResourceTemplate("res://user/{id}", "user", "A templated user record.", "text/plain",
		func(ctx context.Context, p map[string]string) (string, error) { return "user:" + p["id"], nil })

	s.Prompt("summarize", "Summarise a block of text.",
		func(ctx context.Context, args map[string]string) ([]fastmcp.PromptMessage, error) {
			style := args["style"]
			if style == "" {
				style = "concise"
			}
			return []fastmcp.PromptMessage{
				fastmcp.NewUserMessage(fmt.Sprintf("Summarise the following in a %s style:\n%s", style, args["text"])),
			}, nil
		},
		fastmcp.PromptArgument{Name: "text", Description: "the text to summarise", Required: true},
		fastmcp.PromptArgument{Name: "style", Description: "the summary style", Required: false},
	)

	s.Prompt("review", "Review a snippet of code.",
		func(ctx context.Context, args map[string]string) ([]fastmcp.PromptMessage, error) {
			return []fastmcp.PromptMessage{
				fastmcp.NewUserMessage("Review this code:\n" + args["code"]),
			}, nil
		},
		fastmcp.PromptArgument{Name: "code", Description: "the code to review", Required: true},
	)

	return s
}

// ------------------------------------------------------------- normalisation

// dropKeys are volatile or implementation-private response keys, removed on both
// sides: pagination cursors, framework annotations and clocks. Keys beginning
// with "_" (e.g. "_meta") are dropped too.
var dropKeys = map[string]bool{
	"nextCursor": true, "meta": true, "lastModified": true, "timestamp": true,
}

var sortStringArrays = map[string]bool{"required": true, "values": true, "tags": true}

var sortByName = map[string]string{
	"tools":             "name",
	"prompts":           "name",
	"arguments":         "name",
	"resources":         "uri",
	"resourceTemplates": "uriTemplate",
}

func normalise(v any, key string) any {
	switch val := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, item := range val {
			if dropKeys[k] || strings.HasPrefix(k, "_") {
				continue
			}
			out[k] = normalise(item, k)
		}
		return out
	case []any:
		items := make([]any, len(val))
		allStr, allMap := true, true
		for i, item := range val {
			items[i] = normalise(item, "")
			if _, ok := items[i].(string); !ok {
				allStr = false
			}
			if _, ok := items[i].(map[string]any); !ok {
				allMap = false
			}
		}
		if sortStringArrays[key] && allStr {
			sort.Slice(items, func(i, j int) bool { return items[i].(string) < items[j].(string) })
		} else if field, ok := sortByName[key]; ok && allMap {
			sort.SliceStable(items, func(i, j int) bool {
				return fmt.Sprint(items[i].(map[string]any)[field]) < fmt.Sprint(items[j].(map[string]any)[field])
			})
		}
		return items
	default:
		return v
	}
}

// selectPath follows a "/"-separated path. A "#field=value" segment picks the
// element of the current array whose field equals value.
//
// A path that cannot be followed yields JSON null rather than an error, so that
// "field absent on one side, present on the other" scores as a value mismatch
// instead of both sides agreeing that they failed.
func selectPath(v any, path string) any {
	if path == "" {
		return v
	}
	for _, seg := range strings.Split(path, "/") {
		if strings.HasPrefix(seg, "#") {
			field, want, _ := strings.Cut(seg[1:], "=")
			arr, ok := v.([]any)
			if !ok {
				return nil
			}
			found := false
			for _, item := range arr {
				if m, ok := item.(map[string]any); ok && fmt.Sprint(m[field]) == want {
					v, found = item, true
					break
				}
			}
			if !found {
				return nil
			}
			continue
		}
		if n, err := strconv.Atoi(seg); err == nil {
			arr, ok := v.([]any)
			if !ok || n >= len(arr) {
				return nil
			}
			v = arr[n]
			continue
		}
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		item, present := m[seg]
		if !present {
			return nil
		}
		v = item
	}
	return v
}

// ------------------------------------------------------------------- session

// session drives one freshly built server over an in-process pipe pair. A new
// server per case keeps cases independent, exactly as the Python runner does.
type session struct {
	in     *io.PipeWriter
	out    *bufio.Scanner
	cancel context.CancelFunc
	done   chan struct{}
	nextID int
}

func newSession() *session {
	srv := buildServer()

	clientToServer, serverIn := io.Pipe()
	serverOut, srvWriter := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.ServeStdio(ctx, clientToServer, srvWriter)
		_ = srvWriter.Close()
	}()

	sc := bufio.NewScanner(serverOut)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	return &session{in: serverIn, out: sc, cancel: cancel, done: done}
}

func (s *session) close() {
	_ = s.in.Close()
	s.cancel()
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
	}
}

type rpcError struct {
	Code int `json:"code"`
}

type rpcResponse struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
	Method string          `json:"method"`
}

// request sends one JSON-RPC request and returns its outcome, skipping any
// server-initiated notifications that arrive first.
func (s *session) request(method string, params json.RawMessage) (map[string]any, error) {
	s.nextID++
	id := s.nextID
	msg := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if len(params) > 0 {
		msg["params"] = params
	}
	enc, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	if _, err := s.in.Write(append(enc, '\n')); err != nil {
		return nil, err
	}

	type readResult struct {
		out map[string]any
		err error
	}
	ch := make(chan readResult, 1)
	go func() {
		for s.out.Scan() {
			var resp rpcResponse
			if err := json.Unmarshal(s.out.Bytes(), &resp); err != nil {
				ch <- readResult{err: fmt.Errorf("unparseable server line %q: %w", s.out.Text(), err)}
				return
			}
			if resp.Method != "" || len(resp.ID) == 0 { // notification
				continue
			}
			var gotID int
			if err := json.Unmarshal(resp.ID, &gotID); err != nil || gotID != id {
				continue
			}
			if resp.Error != nil {
				ch <- readResult{out: map[string]any{"kind": "error", "code": resp.Error.Code}}
				return
			}
			var result any
			if len(resp.Result) > 0 {
				if err := json.Unmarshal(resp.Result, &result); err != nil {
					ch <- readResult{err: err}
					return
				}
			}
			ch <- readResult{out: map[string]any{"kind": "result", "result": result}}
			return
		}
		err := s.out.Err()
		if err == nil {
			err = fmt.Errorf("server closed its output before answering %s", method)
		}
		ch <- readResult{err: err}
	}()

	select {
	case r := <-ch:
		return r.out, r.err
	case <-time.After(caseTimeout):
		return nil, fmt.Errorf("timed out awaiting %s", method)
	}
}

func (s *session) notify(method string) error {
	enc, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method, "params": map[string]any{}})
	if err != nil {
		return err
	}
	_, err = s.in.Write(append(enc, '\n'))
	return err
}

func initParams(version string) json.RawMessage {
	enc, _ := json.Marshal(map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "parity-client", "version": "1.0.0"},
	})
	return enc
}

func project(outcome map[string]any, path string) map[string]any {
	if outcome["kind"] != "result" {
		return outcome
	}
	return map[string]any{
		"kind":   "result",
		"result": selectPath(normalise(outcome["result"], ""), path),
	}
}

// ------------------------------------------------------------------- the cases

type rpcSpec struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func argString(args []json.RawMessage, i int) string {
	if i >= len(args) {
		return ""
	}
	var s string
	if err := json.Unmarshal(args[i], &s); err != nil {
		return ""
	}
	return s
}

func doInitialize(args []json.RawMessage) (any, error) {
	version := argString(args, 0)
	if version == "" {
		version = handshakeVersion
	}
	path := argString(args, 1)

	s := newSession()
	defer s.close()

	out, err := s.request("initialize", initParams(version))
	if err != nil {
		return nil, err
	}
	return []any{project(out, path)}, nil
}

func doRPC(args []json.RawMessage) (any, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("rpc needs a request argument")
	}
	var specs []rpcSpec
	if err := json.Unmarshal(args[0], &specs); err != nil {
		var one rpcSpec
		if err2 := json.Unmarshal(args[0], &one); err2 != nil {
			return nil, fmt.Errorf("bad rpc argument: %v / %v", err, err2)
		}
		specs = []rpcSpec{one}
	}
	path := argString(args, 1)

	s := newSession()
	defer s.close()

	hand, err := s.request("initialize", initParams(handshakeVersion))
	if err != nil {
		return nil, err
	}
	if hand["kind"] != "result" {
		return nil, fmt.Errorf("handshake failed: %v", hand)
	}
	if err := s.notify("notifications/initialized"); err != nil {
		return nil, err
	}

	outs := make([]any, 0, len(specs))
	for i, spec := range specs {
		out, err := s.request(spec.Method, spec.Params)
		if err != nil {
			return nil, err
		}
		step := ""
		if i == len(specs)-1 {
			step = path
		}
		outs = append(outs, project(out, step))
	}
	return outs, nil
}

// ------------------------------------------------------------------- the loop

type caseReq struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type caseRes struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value any    `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
}

func handle(c caseReq) (value any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	switch c.Fn {
	case "initialize":
		return doInitialize(c.Args)
	case "rpc":
		return doRPC(c.Args)
	default:
		return nil, fmt.Errorf("unknown fn: %s", c.Fn)
	}
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	out := bufio.NewWriter(os.Stdout)
	enc := json.NewEncoder(out)

	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var c caseReq
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			_ = enc.Encode(caseRes{OK: false, Error: "bad request: " + err.Error()})
			_ = out.Flush()
			continue
		}
		value, err := handle(c)
		res := caseRes{ID: c.ID, OK: err == nil, Value: value}
		if err != nil {
			res.Value = nil
			res.Error = err.Error()
		}
		_ = enc.Encode(res)
		_ = out.Flush()
	}
}
