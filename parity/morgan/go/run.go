// Command run is the Go-side runner for the morgan parity harness.
//
// Protocol: one JSON object per line on stdin, one JSON object per line on
// stdout, same order (see ../../HARNESS.md).
//
//	in : {"id":"...","fn":"log","args":[<spec>]}
//	out: {"id":"...","ok":true,"value":"<normalised log line>"}
//	     {"id":"...","ok":false,"error":"..."}
//
// <spec> drives a real request/response exchange through a real net/http server
// with github.com/malcolmston/morgan mounted on it; the emitted access-log line
// (normalised, see normalise) is the comparable artefact. The node runner in
// ../node/run.js performs the identical exchange against upstream morgan.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	morgan "github.com/malcolmston/morgan"
)

// ---------------------------------------------------------------------------
// wire types
// ---------------------------------------------------------------------------

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID string `json:"id"`
	OK bool   `json:"ok"`
	// Value is a pointer so that a legitimately empty log line (a skipped
	// request) still marshals as "value":"" rather than being elided.
	Value *string `json:"value,omitempty"`
	Error string  `json:"error,omitempty"`
}

type spec struct {
	Format   string       `json:"format"`
	Request  requestSpec  `json:"request"`
	Response responseSpec `json:"response"`
	Options  optionsSpec  `json:"options"`
}

type requestSpec struct {
	Method  string          `json:"method"`
	Path    string          `json:"path"`
	Headers json.RawMessage `json:"headers"`
	Body    *string         `json:"body"`
}

type responseSpec struct {
	Status        int               `json:"status"`
	Headers       map[string]string `json:"headers"`
	Body          string            `json:"body"`
	ContentLength string            `json:"contentLength"`
}

type optionsSpec struct {
	Immediate bool   `json:"immediate"`
	Skip      string `json:"skip"`
}

// ---------------------------------------------------------------------------
// custom tokens — registered identically in ../node/run.js
// ---------------------------------------------------------------------------

func registerCustomTokens() {
	morgan.Token("ctok-const", func(_ *http.Request, _ morgan.Log, _ ...string) string {
		return "CONST"
	})
	morgan.Token("ctok-method", func(r *http.Request, _ morgan.Log, _ ...string) string {
		return strings.ToLower(r.Method)
	})
	morgan.Token("ctok-hdr", func(r *http.Request, _ morgan.Log, args ...string) string {
		if len(args) == 0 {
			return ""
		}
		return r.Header.Get(args[0])
	})
	morgan.Token("ctok-empty", func(_ *http.Request, _ morgan.Log, _ ...string) string {
		return ""
	})
}

// ---------------------------------------------------------------------------
// named formats registered at runtime — registered identically in
// ../node/run.js
// ---------------------------------------------------------------------------

func registerCustomFormats() {
	morgan.RegisterFormat("parity-named", ":method :url :status len=:res[content-length]")
	morgan.RegisterFormatFunc("parity-fn", func(_ *http.Request, log morgan.Log) string {
		status := "-"
		if log.STATUS != 0 {
			status = strconv.Itoa(log.STATUS)
		}
		return "FN=" + log.METHOD + " " + status
	})
}

// ---------------------------------------------------------------------------
// named skip functions — registered identically in ../node/run.js
// ---------------------------------------------------------------------------

func skipFor(name string) (morgan.SkipFunc, error) {
	switch name {
	case "", "none":
		return nil, nil
	case "status-lt-400":
		return morgan.SkipStatusBelow(400), nil
	case "path-healthz":
		return morgan.SkipPaths("/healthz"), nil
	default:
		return nil, fmt.Errorf("unknown skip: %s", name)
	}
}

// ---------------------------------------------------------------------------
// normalisation — MUST stay byte-identical with ../node/run.js normalise()
// ---------------------------------------------------------------------------

var (
	reCLF      = regexp.MustCompile(`\d{2}/[A-Z][a-z]{2}/\d{4}:\d{2}:\d{2}:\d{2} [+-]\d{4}`)
	reISO      = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z`)
	reWEB      = regexp.MustCompile(`[A-Z][a-z]{2}, \d{2} [A-Z][a-z]{2} \d{4} \d{2}:\d{2}:\d{2} GMT`)
	reTimeDec  = regexp.MustCompile(`(\d+)\.(\d+) ms`)
	reTimeInt  = regexp.MustCompile(`(\d+) ms`)
	reJSONTime = regexp.MustCompile(`"(responseTime|totalTime)":(\d+)(?:\.(\d+))?`)
	reHostPort = regexp.MustCompile(`127\.0\.0\.1:\d+`)
	rePID      = regexp.MustCompile(`PID=\d+`)
)

func normalise(line string) string {
	line = strings.TrimRight(line, "\n")
	line = reCLF.ReplaceAllString(line, "<DATE-CLF>")
	line = reISO.ReplaceAllString(line, "<DATE-ISO>")
	line = reWEB.ReplaceAllString(line, "<DATE-WEB>")
	line = reTimeDec.ReplaceAllStringFunc(line, func(m string) string {
		g := reTimeDec.FindStringSubmatch(m)
		return "<T:" + strconv.Itoa(len(g[2])) + "> ms"
	})
	line = reTimeInt.ReplaceAllString(line, "<T:0> ms")
	line = reJSONTime.ReplaceAllStringFunc(line, func(m string) string {
		g := reJSONTime.FindStringSubmatch(m)
		return `"` + g[1] + `":<T:` + strconv.Itoa(len(g[3])) + ">"
	})
	line = reHostPort.ReplaceAllString(line, "127.0.0.1:<PORT>")
	line = rePID.ReplaceAllString(line, "PID=<PID>")
	return line
}

// ---------------------------------------------------------------------------
// log collector
// ---------------------------------------------------------------------------

type collector struct {
	mu    sync.Mutex
	lines []string
}

func (c *collector) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, l := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		c.lines = append(c.lines, l)
	}
	return len(p), nil
}

func (c *collector) first() (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.lines) == 0 {
		return "", false
	}
	return c.lines[0], true
}

// ---------------------------------------------------------------------------
// server
// ---------------------------------------------------------------------------

var (
	curMu sync.Mutex
	cur   http.Handler
)

func setCurrent(h http.Handler) {
	curMu.Lock()
	cur = h
	curMu.Unlock()
}

func dispatch(w http.ResponseWriter, r *http.Request) {
	curMu.Lock()
	h := cur
	curMu.Unlock()
	if h == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	h.ServeHTTP(w, r)
}

// responder applies the response half of a case spec, mirroring the express
// handler in ../node/run.js.
func responder(s spec) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body := s.Response.Body
		for k, v := range s.Response.Headers {
			w.Header().Set(k, v)
		}
		if s.Response.ContentLength == "explicit" {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		}
		status := s.Response.Status
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		if body != "" {
			_, _ = io.WriteString(w, body)
		}
	}
}

// ---------------------------------------------------------------------------
// case execution
// ---------------------------------------------------------------------------

func headerPairs(raw json.RawMessage) ([][2]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// deterministic order
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	var out [][2]string
	for _, k := range keys {
		switch v := m[k].(type) {
		case string:
			out = append(out, [2]string{k, v})
		case []any:
			for _, e := range v {
				out = append(out, [2]string{k, fmt.Sprint(e)})
			}
		default:
			out = append(out, [2]string{k, fmt.Sprint(v)})
		}
	}
	return out, nil
}

func runCase(client *http.Client, base string, s spec) (val string, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("panic: %v", p)
		}
	}()

	skip, err := skipFor(s.Options.Skip)
	if err != nil {
		return "", err
	}

	col := &collector{}
	cfg := morgan.Config{
		Stream:    col,
		Immediate: s.Options.Immediate,
		Skip:      skip,
	}
	setCurrent(morgan.New(responder(s), morgan.Format(s.Format), cfg))

	method := s.Request.Method
	if method == "" {
		method = http.MethodGet
	}
	path := s.Request.Path
	if path == "" {
		path = "/"
	}
	var body io.Reader
	if s.Request.Body != nil {
		body = strings.NewReader(*s.Request.Body)
	}
	req, err := http.NewRequest(method, base+path, body)
	if err != nil {
		return "", err
	}
	// Go's client invents a User-Agent; node's does not. Suppress it so that a
	// case without an explicit User-Agent tests the missing-header path on both
	// sides.
	req.Header["User-Agent"] = nil

	pairs, err := headerPairs(s.Request.Headers)
	if err != nil {
		return "", err
	}
	var haveCL bool
	for _, p := range pairs {
		if strings.EqualFold(p[0], "Content-Length") {
			haveCL = true
		}
		if strings.EqualFold(p[0], "Host") {
			req.Host = p[1]
			continue
		}
		req.Header.Add(p[0], p[1])
	}
	if s.Request.Body != nil {
		req.ContentLength = int64(len(*s.Request.Body))
		if !haveCL {
			req.Header.Set("Content-Length", strconv.Itoa(len(*s.Request.Body)))
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	deadline := time.Now().Add(400 * time.Millisecond)
	for {
		if l, ok := col.first(); ok {
			return normalise(l), nil
		}
		if time.Now().After(deadline) {
			return "", nil
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	registerCustomTokens()
	registerCustomFormats()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, "listen:", err)
		os.Exit(1)
	}
	srv := &http.Server{Handler: http.HandlerFunc(dispatch)}
	go func() { _ = srv.Serve(ln) }()
	base := "http://" + ln.Addr().String()

	client := &http.Client{
		Timeout: 5 * time.Second,
		// node's http.request never follows redirects; Go's client would, which
		// would turn a 3xx case into a second exchange.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{
			DisableCompression: true,
			DisableKeepAlives:  true,
		},
	}

	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	out := bufio.NewWriter(os.Stdout)

	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		res := response{OK: false}
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			res.Error = "bad request line: " + err.Error()
		} else {
			res.ID = req.ID
			if len(req.Args) == 0 {
				res.Error = "missing spec argument"
			} else {
				var s spec
				if err := json.Unmarshal(req.Args[0], &s); err != nil {
					res.Error = "bad spec: " + err.Error()
				} else if v, err := runCase(client, base, s); err != nil {
					res.Error = err.Error()
				} else {
					res.OK = true
					res.Value = &v
				}
			}
		}
		enc, err := json.Marshal(res)
		if err != nil {
			enc, _ = json.Marshal(response{ID: res.ID, OK: false, Error: "encode: " + err.Error()})
		}
		out.Write(enc)
		out.WriteByte('\n')
		_ = out.Flush()
	}
}
