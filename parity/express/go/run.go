// Command run is the Go-side runner for the express parity harness.
//
// Protocol: one JSON object per line on stdin, one JSON object per line on
// stdout (see ../../HARNESS.md).
//
//	in  {"id":"...","fn":"<appName>","args":[{"method":"GET","path":"/a"}]}
//	out {"id":"...","ok":true,"value":{"status":200,"headers":[[k,v],...],"body":"..."}}
//
// Each fn names an app fixture built below; the fixture must be the closest
// possible translation of the same fixture in ../node/run.js. Fixtures are
// created once, listened on a random loopback port, and every case for them
// replays one request. Nothing ever leaves localhost.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/malcolmston/express"
	"github.com/malcolmston/express/httperrors"
)

var pub = func() string {
	if v := os.Getenv("PARITY_FIXTURES"); v != "" {
		return filepath.Join(v, "pub")
	}
	return filepath.Join("..", "fixtures", "pub")
}()

// ---------------------------------------------------------------------------
// response normalisation (must match node/run.js exactly)
// ---------------------------------------------------------------------------

var drop = map[string]bool{
	"date":              true,
	"connection":        true,
	"keep-alive":        true,
	"transfer-encoding": true,
	"content-length":    true,
	"x-powered-by":      true,
}

// dropValidators lists headers whose values are implementation-defined (hash
// algorithm, file clock). They are dropped here; conditional-request behaviour
// is compared through status codes instead (see cases/conditional.json).
var dropValidators = map[string]bool{"etag": true, "last-modified": true}

var charsetRe = regexp.MustCompile(`(?i)charset=([^;\s]+)`)

var expiresRe = regexp.MustCompile(`(?i)Expires=[^;]*`)

// canonCookie hides the wall-clock Expires value while keeping the parameter.
func canonCookie(v string) string {
	return expiresRe.ReplaceAllString(v, "Expires=<opaque>")
}

// canonType lower-cases the charset token, which is case-insensitive.
func canonType(v string) string {
	return charsetRe.ReplaceAllStringFunc(v, strings.ToLower)
}

func normaliseHeaders(h http.Header) [][]string {
	out := [][]string{}
	for name, values := range h {
		ln := strings.ToLower(name)
		if drop[ln] || dropValidators[ln] {
			continue
		}
		for _, v := range values {
			if ln == "content-type" {
				v = canonType(v)
			}
			if ln == "set-cookie" {
				v = canonCookie(v)
			}
			out = append(out, []string{ln, v})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i][0] != out[j][0] {
			return out[i][0] < out[j][0]
		}
		return out[i][1] < out[j][1]
	})
	return out
}

// ---------------------------------------------------------------------------
// app fixtures
// ---------------------------------------------------------------------------

func send(s string) express.Handler {
	return func(req *express.Request, res *express.Response, next express.Next) { res.Send(s) }
}

func params(req *express.Request, res *express.Response, next express.Next) {
	res.JSON(req.AllParams())
}

var apps = map[string]func() http.Handler{}

func init() {
	// --- routing: static paths, verbs, precedence, fallthrough -----------
	apps["route.basic"] = func() http.Handler {
		app := express.New()
		app.Get("/", send("root"))
		app.Get("/a", send("a"))
		app.Get("/a/b", send("a/b"))
		app.Get("/a/b/c", send("a/b/c"))
		app.Post("/a", send("post-a"))
		app.Put("/a", send("put-a"))
		app.Delete("/a", send("delete-a"))
		app.Patch("/a", send("patch-a"))
		app.Options("/opt", send("opt"))
		app.Head("/headonly", func(req *express.Request, res *express.Response, next express.Next) {
			res.Set("X-Head", "yes").End()
		})
		app.All("/all", func(req *express.Request, res *express.Response, next express.Next) {
			res.Send("all-" + req.Method())
		})
		app.Get("/dup", send("first"))
		app.Get("/dup", send("second"))
		app.Get("/order/:x", func(req *express.Request, res *express.Response, next express.Next) {
			res.Send("param-" + req.Params("x"))
		})
		app.Get("/order/static", send("static"))
		app.Get("/order2/static", send("static2"))
		app.Get("/order2/:x", func(req *express.Request, res *express.Response, next express.Next) {
			res.Send("param2-" + req.Params("x"))
		})
		app.Get("/chain", func(req *express.Request, res *express.Response, next express.Next) {
			res.Set("X-One", "1")
			next()
		}, send("chain-done"))
		app.Get("/fall", func(req *express.Request, res *express.Response, next express.Next) { next() })
		app.Get("/trail", send("trail"))
		app.Get("/Case", send("cased"))
		// isolate routing from the framework default 404 page (compared
		// separately in the route.default404 fixture)
		app.Use(func(req *express.Request, res *express.Response, next express.Next) {
			res.Status(404).Send("NOMATCH")
		})
		return app
	}

	// --- routing: parameters ---------------------------------------------
	apps["route.params"] = func() http.Handler {
		app := express.New()
		app.Get("/u/:id", params)
		app.Get("/u/:id/p/:pid", params)
		app.Get("/opt/:id?", params)
		app.Get(`/num/:id(\d+)`, params)
		app.Get(`/word/:w([a-z]+)`, params)
		app.Get("/mix/:from-:to", params)
		app.Get("/star/*", params)
		app.Get("/pre/*/post", params)
		app.Get("/many/:a/:b/:c", params)
		// isolate routing from the framework default 404 page (compared
		// separately in the route.default404 fixture)
		app.Use(func(req *express.Request, res *express.Response, next express.Next) {
			res.Status(404).Send("NOMATCH")
		})
		return app
	}

	// --- routing: mounted / nested routers -------------------------------
	apps["route.mount"] = func() http.Handler {
		app := express.New()

		r1 := express.NewRouter()
		r1.Get("/", send("r1-root"))
		r1.Get("/x", send("r1-x"))
		r1.Get("/p/:id", params)
		app.Use("/m", r1)

		merged := express.NewRouter(express.RouterOptions{MergeParams: true})
		merged.Get("/who", params)
		app.Use("/u/:uid", merged)

		unmerged := express.NewRouter(express.RouterOptions{MergeParams: false})
		unmerged.Get("/who", params)
		app.Use("/n/:nid", unmerged)

		inner := express.NewRouter()
		inner.Get("/leaf", send("leaf"))
		outer := express.NewRouter()
		outer.Use("/in", inner)
		outer.Get("/own", send("outer-own"))
		app.Use("/out", outer)

		mw := express.NewRouter()
		mw.Use(func(req *express.Request, res *express.Response, next express.Next) {
			res.Set("X-Router-MW", "1")
			next()
		})
		mw.Get("/hit", send("mw-hit"))
		app.Use("/mw", mw)

		app.Use("/pfx", func(req *express.Request, res *express.Response, next express.Next) {
			res.Send("pfx:" + req.Path())
		})

		loc := express.NewRouter()
		loc.Get("/where", func(req *express.Request, res *express.Response, next express.Next) {
			res.JSON(map[string]any{
				"baseUrl":     baseURLOf(req),
				"originalUrl": req.OriginalURL(),
				"path":        req.Path(),
				"url":         req.URL(),
			})
		})
		app.Use("/loc", loc)
		// isolate routing from the framework default 404 page (compared
		// separately in the route.default404 fixture)
		app.Use(func(req *express.Request, res *express.Response, next express.Next) {
			res.Status(404).Send("NOMATCH")
		})
		return app
	}

	// --- routing: strict / case-sensitive settings ------------------------
	// The Go port has no app.set("strict routing") / app.set("case sensitive
	// routing"); the nearest equivalent is a RouterOptions-configured router
	// mounted at the root. See COVERAGE.md.
	apps["route.strict"] = func() http.Handler {
		app := express.New()
		r := express.NewRouter(express.RouterOptions{Strict: true})
		r.Get("/s", send("s"))
		r.Get("/t/", send("t-slash"))
		app.Use("/", r)
		// isolate routing from the framework default 404 page (compared
		// separately in the route.default404 fixture)
		app.Use(func(req *express.Request, res *express.Response, next express.Next) {
			res.Status(404).Send("NOMATCH")
		})
		return app
	}
	apps["route.case"] = func() http.Handler {
		app := express.New()
		r := express.NewRouter(express.RouterOptions{CaseSensitive: true})
		r.Get("/Foo", send("Foo"))
		r.Get("/bar", send("bar"))
		app.Use("/", r)
		// isolate routing from the framework default 404 page (compared
		// separately in the route.default404 fixture)
		app.Use(func(req *express.Request, res *express.Response, next express.Next) {
			res.Status(404).Send("NOMATCH")
		})
		return app
	}

	// --- routing: app.param callbacks -------------------------------------
	apps["route.param"] = func() http.Handler {
		app := express.New()
		seen := func(req *express.Request) []any {
			if v, ok := req.Value("seen"); ok {
				return v.([]any)
			}
			return []any{}
		}
		app.Param("id", func(req *express.Request, res *express.Response, next express.Next, id string) {
			req.Set("seen", append(seen(req), "id:"+id))
			next()
		})
		app.Get("/p/:id", func(req *express.Request, res *express.Response, next express.Next) {
			res.JSON(map[string]any{"seen": seen(req), "id": req.Params("id")})
		})
		app.Get("/q/:other", func(req *express.Request, res *express.Response, next express.Next) {
			res.JSON(map[string]any{"seen": seen(req), "other": req.Params("other")})
		})
		app.Param("bad", func(req *express.Request, res *express.Response, next express.Next, v string) {
			next(fmt.Errorf("bad param"))
		})
		app.Get("/b/:bad", send("should-not-run"))
		app.Use(func(err error, req *express.Request, res *express.Response, next express.Next) {
			res.Status(500).Send("perr")
		})
		app.Use(func(req *express.Request, res *express.Response, next express.Next) {
			res.Status(404).Send("NOMATCH")
		})
		return app
	}

	// --- routing: strict / case sensitivity via app.set --------------------
	apps["route.appset"] = func() http.Handler {
		app := express.New()
		app.Set("strict routing", true)
		app.Set("case sensitive routing", true)
		app.Get("/s", send("s"))
		app.Get("/Foo", send("Foo"))
		app.Use(func(req *express.Request, res *express.Response, next express.Next) {
			res.Status(404).Send("NOMATCH")
		})
		return app
	}

	// the framework's own unmatched-route response, in isolation
	apps["route.default404"] = func() http.Handler {
		app := express.New()
		app.Get("/x", send("x"))
		return app
	}

	// --- body parsing -----------------------------------------------------
	apps["body"] = func() http.Handler {
		app := express.New()
		app.Use(express.JSON())
		app.Use(express.URLEncoded())
		app.Post("/echo", func(req *express.Request, res *express.Response, next express.Next) {
			res.JSON(map[string]any{"body": req.Body()})
		})
		app.Use(func(err error, req *express.Request, res *express.Response, next express.Next) {
			// Mirror node's `res.status(err.status || 500)`: a body-parser error
			// carries its own HTTP status (400 for malformed / non-strict JSON).
			status := 500
			if he, ok := err.(*httperrors.Error); ok && he.Status != 0 {
				status = he.Status
			}
			res.Status(status).JSON(map[string]any{"parseError": true})
		})
		return app
	}

	// --- response helpers -------------------------------------------------
	apps["res"] = func() http.Handler {
		app := express.New()
		app.Get("/send-string", send("plain string"))
		app.Get("/send-obj", func(req *express.Request, res *express.Response, next express.Next) {
			res.Send(map[string]any{"a": 1, "b": "two"})
		})
		app.Get("/send-buf", func(req *express.Request, res *express.Response, next express.Next) {
			res.Send([]byte("raw bytes"))
		})
		app.Get("/json", func(req *express.Request, res *express.Response, next express.Next) {
			res.JSON(map[string]any{"a": []any{1, 2, 3}, "z": 1})
		})
		app.Get("/json-key-order", func(req *express.Request, res *express.Response, next express.Next) {
			res.JSON(map[string]any{"z": 1, "m": 2, "a": 3})
		})
		app.Get("/json-null", func(req *express.Request, res *express.Response, next express.Next) {
			res.JSON(nil)
		})
		app.Get("/status", func(req *express.Request, res *express.Response, next express.Next) {
			res.Status(201).Send("created")
		})
		app.Get("/send-status", func(req *express.Request, res *express.Response, next express.Next) {
			res.SendStatus(404)
		})
		app.Get("/send-status-418", func(req *express.Request, res *express.Response, next express.Next) {
			res.SendStatus(418)
		})
		app.Get("/redirect", func(req *express.Request, res *express.Response, next express.Next) {
			res.Redirect("/target")
		})
		app.Get("/redirect-301", func(req *express.Request, res *express.Response, next express.Next) {
			res.Redirect(301, "/target")
		})
		app.Get("/type-json", func(req *express.Request, res *express.Response, next express.Next) {
			res.Type("json").Send(`{"x":1}`)
		})
		app.Get("/type-html", func(req *express.Request, res *express.Response, next express.Next) {
			res.Type("html").Send("<b>hi</b>")
		})
		app.Get("/type-full", func(req *express.Request, res *express.Response, next express.Next) {
			res.Type("application/xml").Send("<x/>")
		})
		app.Get("/set", func(req *express.Request, res *express.Response, next express.Next) {
			res.Set("X-Custom", "v1").Set("X-Other", "v2").Send("set")
		})
		app.Get("/append", func(req *express.Request, res *express.Response, next express.Next) {
			res.Append("X-Multi", "a")
			res.Append("X-Multi", "b")
			res.Send("append")
		})
		app.Get("/cookie", func(req *express.Request, res *express.Response, next express.Next) {
			res.Cookie("c", "v", &express.CookieOptions{Path: "/", HTTPOnly: true, MaxAge: 60}).Send("cookie")
		})
		app.Get("/cookie-plain", func(req *express.Request, res *express.Response, next express.Next) {
			res.Cookie("p", "q", nil).Send("cookie-plain")
		})
		app.Get("/clear-cookie", func(req *express.Request, res *express.Response, next express.Next) {
			res.ClearCookie("c").Send("cleared")
		})
		app.Get("/location", func(req *express.Request, res *express.Response, next express.Next) {
			res.Location("/somewhere").Send("loc")
		})
		app.Get("/links", func(req *express.Request, res *express.Response, next express.Next) {
			res.Links(map[string]string{"next": "http://x/?p=2"}).Send("links")
		})
		app.Get("/vary", func(req *express.Request, res *express.Response, next express.Next) {
			res.Vary("Accept-Encoding").Send("vary")
		})
		app.Get("/jsonp", func(req *express.Request, res *express.Response, next express.Next) {
			res.JSONP(map[string]any{"a": 1})
		})
		app.Get("/end", func(req *express.Request, res *express.Response, next express.Next) {
			res.Status(204).End()
		})
		app.Get("/read-cookie", func(req *express.Request, res *express.Response, next express.Next) {
			res.Send("cookie=" + req.Cookie("c"))
		})
		app.Get("/req-info", func(req *express.Request, res *express.Response, next express.Next) {
			res.JSON(map[string]any{
				"method":      req.Method(),
				"path":        req.Path(),
				"protocol":    req.Protocol(),
				"secure":      req.Secure(),
				"xhr":         req.Xhr(),
				"contentType": req.Get("content-type"),
				"hostSeen":    req.Hostname() != "",
			})
		})
		app.Get("/query", func(req *express.Request, res *express.Response, next express.Next) {
			q := req.QueryValues()
			get := func(k string) any {
				if _, ok := q[k]; !ok {
					return nil
				}
				return q.Get(k)
			}
			res.JSON(map[string]any{"a": get("a"), "b": get("b")})
		})
		return app
	}

	// --- content negotiation ---------------------------------------------
	apps["negotiate"] = func() http.Handler {
		app := express.New()
		app.Get("/format", func(req *express.Request, res *express.Response, next express.Next) {
			res.Format(map[string]func(){
				"text/plain":       func() { res.Send("as text") },
				"text/html":        func() { res.Send("<p>as html</p>") },
				"application/json": func() { res.JSON(map[string]any{"as": "json"}) },
			})
		})
		// A single-entry map so the "nothing matched" fallback is well defined on
		// both sides (this Go map is unordered, so a multi-entry fallback has no
		// defined winner here).
		app.Get("/format-one", func(req *express.Request, res *express.Response, next express.Next) {
			res.Format(map[string]func(){
				"text/plain": func() { res.Send("only text") },
			})
		})
		app.Get("/format-nodefault", func(req *express.Request, res *express.Response, next express.Next) {
			res.Format(map[string]func(){
				"application/json": func() { res.JSON(map[string]any{"only": "json"}) },
			})
		})
		app.Get("/accepts", func(req *express.Request, res *express.Response, next express.Next) {
			nz := func(s string) any {
				if s == "" {
					return nil
				}
				return s
			}
			res.JSON(map[string]any{
				"best":     nz(req.Accepts("html", "json")),
				"jsonOnly": nz(req.Accepts("json")),
				"lang":     nz(req.AcceptsLanguages("en", "fr")),
				"enc":      nz(req.AcceptsEncodings("gzip", "identity")),
				"charset":  nz(req.AcceptsCharsets("utf-8", "iso-8859-1")),
			})
		})
		// keep the 406 comparison portable: upstream's default error page embeds
		// absolute file paths and a stack trace
		app.Use(func(err error, req *express.Request, res *express.Response, next express.Next) {
			// Mirror node's `res.status(err.status || 500)`.
			status := 500
			if he, ok := err.(*httperrors.Error); ok && he.Status != 0 {
				status = he.Status
			}
			res.Status(status).Send("NOTACCEPTABLE")
		})
		return app
	}

	// --- conditional GET / etag ------------------------------------------
	apps["etag"] = func() http.Handler {
		app := express.New()
		app.Get("/body", send("etag me please"))
		app.Get("/json", func(req *express.Request, res *express.Response, next express.Next) {
			res.JSON(map[string]any{"etag": true})
		})
		app.Get("/manual", func(req *express.Request, res *express.Response, next express.Next) {
			res.Set("ETag", `"fixed-tag"`)
			if req.Get("if-none-match") == `"fixed-tag"` {
				res.Status(304).End()
				return
			}
			res.Send("manual etag")
		})
		return app
	}

	// --- static file serving ----------------------------------------------
	apps["static"] = func() http.Handler {
		app := express.New()
		app.Use(express.Static(pub))
		app.Use("/assets", express.Static(pub))
		app.Use(func(req *express.Request, res *express.Response, next express.Next) {
			res.Status(404).Send("no-file")
		})
		return app
	}

	// --- error handling ---------------------------------------------------
	apps["errors"] = func() http.Handler {
		app := express.New()

		app.Get("/next-err", func(req *express.Request, res *express.Response, next express.Next) {
			next(fmt.Errorf("boom"))
		})
		app.Get("/throw", func(req *express.Request, res *express.Response, next express.Next) {
			panic("thrown")
		})
		app.Get("/ok", send("ok"))

		sub := express.NewRouter()
		sub.Get("/inside", send("should-not-run"))
		app.Get("/pre-sub", func(req *express.Request, res *express.Response, next express.Next) {
			next(fmt.Errorf("before-sub"))
		})
		app.Use("/sub", func(req *express.Request, res *express.Response, next express.Next) {
			next(fmt.Errorf("sub-mw-error"))
		}, sub)

		local := express.NewRouter()
		local.Get("/fail", func(req *express.Request, res *express.Response, next express.Next) {
			next(fmt.Errorf("local"))
		})
		local.Use(func(err error, req *express.Request, res *express.Response, next express.Next) {
			res.Status(500).Send("local-handler:" + err.Error())
		})
		app.Use("/local", local)

		app.Use(func(req *express.Request, res *express.Response, next express.Next) {
			res.Status(404).Send("custom-404")
		})
		app.Use(func(err error, req *express.Request, res *express.Response, next express.Next) {
			res.Status(500).Send("app-handler:" + err.Error())
		})
		return app
	}
}

// baseURLOf approximates Express's req.baseUrl, which the Go port does not
// expose. See COVERAGE.md.
func baseURLOf(req *express.Request) string {
	orig := req.OriginalURL()
	if i := strings.IndexByte(orig, '?'); i >= 0 {
		orig = orig[:i]
	}
	p := req.Path()
	if strings.HasSuffix(orig, p) {
		return strings.TrimSuffix(orig, p)
	}
	return ""
}

// ---------------------------------------------------------------------------
// server management + request replay
// ---------------------------------------------------------------------------

type running struct {
	port int
	err  error
}

var (
	mu      sync.Mutex
	servers = map[string]*running{}
)

func serverFor(name string) (int, error) {
	mu.Lock()
	defer mu.Unlock()
	if r, ok := servers[name]; ok {
		return r.port, r.err
	}
	r := &running{}
	servers[name] = r
	factory, ok := apps[name]
	if !ok {
		r.err = fmt.Errorf("unknown app fixture: %s", name)
		return 0, r.err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		r.err = err
		return 0, err
	}
	srv := &http.Server{Handler: factory()}
	go func() { _ = srv.Serve(ln) }()
	r.port = ln.Addr().(*net.TCPAddr).Port
	return r.port, nil
}

type reqSpec struct {
	Method     string            `json:"method"`
	Path       string            `json:"path"`
	Headers    map[string]string `json:"headers"`
	Body       *string           `json:"body"`
	Revalidate string            `json:"revalidate"`
}

type rawResp struct {
	status  int
	headers http.Header
	body    string
}

var client = &http.Client{
	Timeout: 10 * time.Second,
	// Never follow redirects: the redirect response itself is the artefact.
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	Transport:     &http.Transport{DisableCompression: true, DisableKeepAlives: true},
}

func doRequest(port int, spec reqSpec, extra map[string]string) (*rawResp, error) {
	method := spec.Method
	if method == "" {
		method = "GET"
	}
	var body io.Reader
	if spec.Body != nil {
		body = strings.NewReader(*spec.Body)
	}
	url := fmt.Sprintf("http://127.0.0.1:%d%s", port, spec.Path)
	r, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	for k, v := range spec.Headers {
		r.Header.Set(k, v)
	}
	for k, v := range extra {
		r.Header.Set(k, v)
	}
	resp, err := client.Do(r)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return &rawResp{status: resp.StatusCode, headers: resp.Header, body: string(b)}, nil
}

func replay(name string, spec reqSpec) (any, error) {
	port, err := serverFor(name)
	if err != nil {
		return nil, err
	}
	raw, err := doRequest(port, spec, nil)
	if err != nil {
		return nil, err
	}
	if spec.Revalidate != "" {
		src, dst := "ETag", "If-None-Match"
		if spec.Revalidate == "lastModified" {
			src, dst = "Last-Modified", "If-Modified-Since"
		}
		v := raw.headers.Get(src)
		if v == "" {
			return map[string]any{"status": 0, "headers": [][]string{}, "body": "", "validator": "absent"}, nil
		}
		raw, err = doRequest(port, spec, map[string]string{dst: v})
		if err != nil {
			return nil, err
		}
	}
	return map[string]any{
		"status":  raw.status,
		"headers": normaliseHeaders(raw.headers),
		"body":    raw.body,
	}, nil
}

// ---------------------------------------------------------------------------
// main loop: strictly sequential, never exits on a failing case
// ---------------------------------------------------------------------------

type inCase struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

func main() {
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	out := os.Stdout
	emit := func(v any) {
		b, err := json.Marshal(v)
		if err != nil {
			b = []byte(`{"id":"?","ok":false,"error":"marshal failure"}`)
		}
		out.Write(append(b, '\n'))
		out.Sync()
	}
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var c inCase
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			emit(map[string]any{"id": "?", "ok": false, "error": "bad input: " + err.Error()})
			continue
		}
		var spec reqSpec
		if len(c.Args) > 0 {
			if err := json.Unmarshal(c.Args[0], &spec); err != nil {
				emit(map[string]any{"id": c.ID, "ok": false, "error": "bad spec: " + err.Error()})
				continue
			}
		}
		value, err := replay(c.Fn, spec)
		if err != nil {
			emit(map[string]any{"id": c.ID, "ok": false, "error": err.Error()})
			continue
		}
		emit(map[string]any{"id": c.ID, "ok": true, "value": value})
	}
}
