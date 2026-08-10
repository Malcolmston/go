// Example program exercising github.com/malcolmston/morgan.
//
// Everything runs in-process against an httptest server and the program exits on
// its own. Log output is captured into a bytes.Buffer per scenario so it can be
// printed under a label.
//
// Covered: all six predefined formats, custom format strings with every token
// family (:req/:res/:date/:response-time[n]/:host/:path/:query/:protocol/:incoming),
// custom tokens, named format + format-func registration, the Skip helpers,
// Immediate mode, buffered output, X-Forwarded-For handling, basic auth
// (:remote-user), streaming handlers, and the standalone helpers (Compile,
// FromRequest, Clfdate, ClientIP, RequestURL, RequestProtocol, StatusCategory,
// StatusColorCode, FormatDuration, Log.String, IP.String).
//
// Written against the PUBLISHED module github.com/malcolmston/morgan v1.2.0.
// Places where that version cannot do what is wanted are marked "HOLE:".
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/malcolmston/morgan"
)

// Normalise only the variable timing numbers, so runs are comparable while IPs,
// HTTP versions and body sizes stay intact.
var timingRes = []*regexp.Regexp{
	regexp.MustCompile(`\d+\.\d+ ms`),
	regexp.MustCompile(`(rt|tt)=\d+(\.\d+)?ms`),
	regexp.MustCompile(`"(responseTime|totalTime)":\d+(\.\d+)?`),
	regexp.MustCompile(`\bms=\d+(\.\d+)?`),
}

func norm(s string) string {
	for _, re := range timingRes {
		s = re.ReplaceAllStringFunc(s, func(m string) string {
			if i := strings.IndexAny(m, "=:"); i >= 0 {
				return m[:i+1] + "<ms>"
			}
			return "<ms> ms"
		})
	}
	return s
}

// syncBuf is a mutex-guarded bytes.Buffer used as morgan's Stream.
//
// HOLE: v1.2.0 does no synchronisation of its own. It calls fmt.Fprintln(out, …)
// directly from the request goroutine, and when Config.Buffer > 0 the delayed
// flush runs on a time.AfterFunc goroutine — with no Flush/Close on the returned
// handler to synchronise with. Passing a bare *bytes.Buffer (as the README's own
// examples effectively do) is therefore a data race under `go run -race .`, so
// this example locks around both its writes and its reads.
type syncBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func (s *syncBuf) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Len()
}

func (s *syncBuf) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf.Reset()
}

func banner(title string) {
	fmt.Printf("\n=== %s\n", title)
}

func show(buf *syncBuf) {
	out := strings.TrimRight(buf.String(), "\n")
	if out == "" {
		fmt.Println("  (no output)")
		return
	}
	for _, line := range strings.Split(out, "\n") {
		fmt.Println("  " + norm(line))
	}
	buf.Reset()
}

// app is the handler under observation. It covers several response shapes.
func app() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Api-Version", "2")
		_, _ = w.Write([]byte("hello world"))
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusFound)
	})
	mux.HandleFunc("/missing", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	})
	mux.HandleFunc("/boom", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "kaboom", http.StatusInternalServerError)
	})
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Api-Version", "2")
		b, _ := io.ReadAll(r.Body)
		_, _ = w.Write(b)
	})
	// A handler that wants to stream. HOLE: morgan v1.2.0's internal
	// responseRecorder embeds only http.ResponseWriter and forwards nothing else,
	// so the Flusher/Hijacker/Pusher/ReaderFrom capabilities of the real writer are
	// hidden from the handler. A plain `w.(http.Flusher).Flush()` — which works
	// without the middleware — panics once morgan wraps the handler, so this has to
	// be written defensively:
	mux.HandleFunc("/stream", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("chunk-1"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		} else {
			_, _ = w.Write([]byte("[no-flusher]"))
		}
		_, _ = w.Write([]byte("chunk-2"))
	})
	return mux
}

// drive spins up an httptest server wrapped in morgan, runs fn against it, and
// tears it down.
func drive(format morgan.Format, cfg morgan.Config, fn func(base string, c *http.Client)) {
	srv := httptest.NewServer(morgan.New(app(), format, cfg))
	defer srv.Close()
	fn(srv.URL, srv.Client())
}

func get(base, path string, c *http.Client, mutate ...func(*http.Request)) {
	req, err := http.NewRequest(http.MethodGet, base+path, nil)
	if err != nil {
		panic(err)
	}
	req.Header.Set("User-Agent", "morgan-example/1.0")
	req.Header.Set("Referer", "https://example.test/from")
	for _, m := range mutate {
		m(req)
	}
	// Do not follow redirects, so 302 shows up in the log.
	client := &http.Client{
		Transport: c.Transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

func post(base, path, body string, c *http.Client, mutate ...func(*http.Request)) {
	req, err := http.NewRequest(http.MethodPost, base+path, strings.NewReader(body))
	if err != nil {
		panic(err)
	}
	req.Header.Set("User-Agent", "morgan-example/1.0")
	for _, m := range mutate {
		m(req)
	}
	resp, err := c.Do(req)
	if err != nil {
		panic(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

func main() {
	buf := &syncBuf{}

	// ---------------------------------------------------------------- formats
	banner("1. Predefined formats")
	for _, f := range []struct {
		name string
		f    morgan.Format
	}{
		{"Combined", morgan.Combined},
		{"Common", morgan.Common},
		{"Dev", morgan.Dev},
		{"Short", morgan.Short},
		{"Tiny", morgan.Tiny},
		{"JSON", morgan.JSON},
	} {
		drive(f.f, morgan.Config{Stream: buf}, func(base string, c *http.Client) {
			get(base, "/?q=1", c)
		})
		fmt.Printf("  %-9s %s\n", f.name+":",
			norm(strings.TrimRight(buf.String(), "\n")))
		buf.Reset()
	}
	fmt.Println("  note: Dev is emitted uncoloured because Stream is not a terminal.")

	// The JSON format is supposed to emit one parseable object per line.
	drive(morgan.JSON, morgan.Config{Stream: buf}, func(base string, c *http.Client) {
		get(base, "/", c)
	})
	jsonLine := strings.TrimRight(buf.String(), "\n")
	buf.Reset()
	fmt.Printf("  json.Valid(JSON line) = %v\n", json.Valid([]byte(jsonLine)))
	// HOLE: it is not valid JSON. The "-" placeholders for status/contentLength are
	// interpolated unquoted (`"contentLength":-`), so any consumer piping this into
	// a JSON parser fails on ordinary requests.
	if !json.Valid([]byte(jsonLine)) {
		var v any
		fmt.Printf("  HOLE: unmarshal error = %v\n", json.Unmarshal([]byte(jsonLine), &v))
	}

	// ------------------------------------------------------- named lookup form
	banner("2. Named formats resolve as strings too")
	for _, name := range []string{"combined", "common", "short", "tiny", "dev", "json"} {
		drive(morgan.Format(name), morgan.Config{Stream: buf}, func(base string, c *http.Client) {
			get(base, "/", c)
		})
		fmt.Printf("  %-9s %s\n", name+":",
			norm(strings.TrimRight(buf.String(), "\n")))
		buf.Reset()
	}

	// ----------------------------------------------------------- token coverage
	banner("3. Custom format string, wide token coverage")
	tokenFmt := morgan.Format(strings.Join([]string{
		":method", ":url", ":path", ":query", ":status",
		"http/:http-version", ":protocol", "host=:host",
		"len=:res[content-length]", "ver=:res[x-api-version]",
		"accept=:req[accept]", "ua=:user-agent", "ref=:referrer",
		"in=:incoming", "rt=:response-time[1]ms", "tt=:total-time[2]ms",
		"pid=:pid", "iso=:date[iso]", "clf=:date[clf]", "web=:date[web]",
		"unknown=:not-a-real-token",
	}, " "))
	drive(tokenFmt, morgan.Config{Stream: buf}, func(base string, c *http.Client) {
		req, _ := http.NewRequest(http.MethodPost, base+"/echo?a=1&b=two", strings.NewReader("payload!"))
		// Two values for one header: :req[accept] joins them with ", " like JS.
		req.Header.Add("Accept", "application/json")
		req.Header.Add("Accept", "text/plain")
		req.Header.Set("User-Agent", "morgan-example/1.0")
		req.Header.Set("Referer", "https://example.test/from")
		resp, err := c.Do(req)
		if err != nil {
			panic(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	})
	show(buf)
	fmt.Println("  (unknown tokens render as \"-\")")

	banner("4. Status variety + streamed response")
	drive(":method :url :status cat=:status len=:res[content-length]",
		morgan.Config{Stream: buf}, func(base string, c *http.Client) {
			for _, p := range []string{"/", "/redirect", "/missing", "/boom", "/stream"} {
				get(base, p, c)
			}
		})
	show(buf)
	fmt.Println("  HOLE: len=- on every row. Go's net/http never puts Content-Length")
	fmt.Println("  into the header map, and v1.2.0 has no observed-body-size fallback,")
	fmt.Println("  so :res[content-length] — used by Combined/Common/Short/Tiny — is")
	fmt.Println("  always \"-\" for ordinary handlers.")
	fmt.Println("  HOLE: /stream could not flush; morgan's wrapper hides http.Flusher.")

	banner("5. Basic auth -> :remote-user, plus log injection")
	drive(":remote-addr user=:remote-user :method :url :status",
		morgan.Config{Stream: buf}, func(base string, c *http.Client) {
			get(base, "/", c, func(r *http.Request) { r.SetBasicAuth("alice", "s3cret") })
			// HOLE: no token sanitisation in v1.2.0. A newline in the Basic-auth
			// username is written verbatim, so a client can forge extra log lines.
			get(base, "/", c, func(r *http.Request) { r.SetBasicAuth("ev\nil 999 FORGED", "x") })
		})
	show(buf)
	fmt.Println("  ^ the second request produced TWO lines: unsanitised :remote-user.")

	// -------------------------------------------------------------- custom token
	banner("6. Custom token via morgan.Token")
	morgan.Token("request-id", func(r *http.Request, _ morgan.Log, _ ...string) string {
		return r.Header.Get("X-Request-ID")
	})
	// Tokens receive the [arg] from :name[arg]. HOLE: there is no Log field for the
	// observed response body size in v1.2.0 (no RESPONSE_SIZE), so a size-based
	// token has to fall back to the request's own Content-Length.
	morgan.Token("size-class", func(_ *http.Request, log morgan.Log, args ...string) string {
		limit := int64(4)
		if len(args) > 0 {
			fmt.Sscanf(args[0], "%d", &limit)
		}
		if log.INCOMING > limit {
			return "big-request"
		}
		return "small-request"
	})
	drive(":method :url :status id=:request-id class=:size-class[5]",
		morgan.Config{Stream: buf}, func(base string, c *http.Client) {
			get(base, "/", c, func(r *http.Request) { r.Header.Set("X-Request-ID", "req-abc-123") })
			post(base, "/echo", "a-longer-body", c,
				func(r *http.Request) { r.Header.Set("X-Request-ID", "req-def-456") })
		})
	show(buf)

	// ------------------------------------------------- registered named formats
	banner("7. RegisterFormat / RegisterFormatFunc")
	morgan.RegisterFormat("with-id", ":method :url :status :request-id")
	morgan.RegisterFormatFunc("kv", func(r *http.Request, log morgan.Log) string {
		return fmt.Sprintf("method=%s path=%s status=%d cat=%s color=%d ms=%s ua=%q",
			log.METHOD, log.URL, log.STATUS,
			morgan.StatusCategory(log.STATUS), morgan.StatusColorCode(log.STATUS),
			morgan.FormatDuration(log.TOTAL_TIME, 2), log.USER_AGENT)
	})
	drive("with-id", morgan.Config{Stream: buf}, func(base string, c *http.Client) {
		get(base, "/", c, func(r *http.Request) { r.Header.Set("X-Request-ID", "from-named-format") })
	})
	show(buf)
	drive("kv", morgan.Config{Stream: buf}, func(base string, c *http.Client) {
		get(base, "/boom", c)
	})
	show(buf)

	// --------------------------------------------------------------- skipping
	banner("8. Skip helpers")
	fmt.Println("  SkipPaths(\"/healthz\") — only \"/\" should appear:")
	drive(morgan.Tiny, morgan.Config{Stream: buf, Skip: morgan.SkipPaths("/healthz")},
		func(base string, c *http.Client) {
			get(base, "/healthz", c)
			get(base, "/", c)
		})
	show(buf)

	fmt.Println("  SkipStatusBelow(400) — only 404/500 should appear:")
	drive(morgan.Tiny, morgan.Config{Stream: buf, Skip: morgan.SkipStatusBelow(400)},
		func(base string, c *http.Client) {
			for _, p := range []string{"/", "/redirect", "/missing", "/boom"} {
				get(base, p, c)
			}
		})
	show(buf)

	fmt.Println("  SkipStatusBetween(300, 399) — the 302 should be dropped:")
	drive(morgan.Tiny, morgan.Config{Stream: buf, Skip: morgan.SkipStatusBetween(300, 399)},
		func(base string, c *http.Client) {
			get(base, "/", c)
			get(base, "/redirect", c)
		})
	show(buf)

	fmt.Println("  SkipUserAgents(\"kube-probe\") — probe request dropped:")
	drive(morgan.Tiny, morgan.Config{Stream: buf, Skip: morgan.SkipUserAgents("kube-probe")},
		func(base string, c *http.Client) {
			get(base, "/", c, func(r *http.Request) { r.Header.Set("User-Agent", "kube-probe/1.29") })
			get(base, "/", c)
		})
	show(buf)

	fmt.Println("  CombineSkips(SkipPaths(\"/healthz\"), SkipStatusBelow(400)) — only errors, never /healthz:")
	drive(morgan.Tiny, morgan.Config{Stream: buf, Skip: morgan.CombineSkips(
		morgan.SkipPaths("/healthz"), morgan.SkipStatusBelow(400))},
		func(base string, c *http.Client) {
			for _, p := range []string{"/healthz", "/", "/missing"} {
				get(base, p, c)
			}
		})
	show(buf)

	// -------------------------------------------------------------- immediate
	banner("9. Immediate mode (logged on arrival: no status/size/timings)")
	drive(":method :url status=:status len=:res[content-length] rt=:response-time tt=:total-time",
		morgan.Config{Stream: buf, Immediate: true}, func(base string, c *http.Client) {
			get(base, "/?immediate=1", c)
		})
	show(buf)
	fmt.Println("  HOLE: :status and :res[content-length] correctly render \"-\", but the")
	fmt.Println("  timings print 0.000 instead of \"-\": v1.2.0 passes zero durations")
	fmt.Println("  rather than a sentinel, so an unmeasured request looks instantaneous.")

	// ---------------------------------------------------------------- buffering
	banner("10. Buffered stream (Buffer: 50ms)")
	drive(morgan.Tiny, morgan.Config{Stream: buf, Buffer: 50 * time.Millisecond},
		func(base string, c *http.Client) {
			for i := 0; i < 3; i++ {
				get(base, "/", c)
			}
			fmt.Printf("  immediately after 3 requests, buffer holds %d bytes\n", buf.Len())
			time.Sleep(120 * time.Millisecond) // let the flush interval elapse
		})
	fmt.Println("  after the flush interval:")
	show(buf)

	// ------------------------------------------------------- forwarded headers
	banner("11. X-Forwarded-For handling")
	// HOLE: v1.2.0 has no Config.TrustProxy (and no ClientIPTrustProxy /
	// RequestProtocolTrustProxy / FromRequestTrustProxy). X-Forwarded-For is
	// trusted unconditionally, so any client on a direct connection can choose the
	// address recorded against its own requests. There is no way to turn that off:
	//   morgan.Config{TrustProxy: false}   // unknown field — does not compile
	fmt.Println("  single forged hop (spoofs :remote-addr, cannot be disabled):")
	drive(":remote-addr :protocol :method :url :status",
		morgan.Config{Stream: buf}, func(base string, c *http.Client) {
			get(base, "/", c, func(r *http.Request) {
				r.Header.Set("X-Forwarded-For", "203.0.113.7")
				r.Header.Set("X-Forwarded-Proto", "https")
			})
		})
	show(buf)
	// HOLE: FromRequest assigns the WHOLE X-Forwarded-For header to REMOTE_IP
	// without splitting on "," (unlike the exported ClientIP helper, which does
	// split). A normal two-hop chain therefore fails net.ParseIP and the token
	// renders the literal Go string "<nil>".
	fmt.Println("  two-hop chain \"203.0.113.7, 10.0.0.1\":")
	drive(":remote-addr :protocol :method :url :status",
		morgan.Config{Stream: buf}, func(base string, c *http.Client) {
			get(base, "/", c, func(r *http.Request) {
				r.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.1")
			})
		})
	show(buf)
	fmt.Println("  ^ HOLE: renders \"<nil>\", and :protocol stays http even with")
	fmt.Println("  X-Forwarded-Proto: https on a non-TLS connection.")

	// --------------------------------------------------- concurrency / interleave
	banner("12. Concurrent requests")
	drive(morgan.Tiny, morgan.Config{Stream: buf}, func(base string, c *http.Client) {
		done := make(chan struct{}, 20)
		for i := 0; i < 20; i++ {
			go func() { get(base, "/", c); done <- struct{}{} }()
		}
		for i := 0; i < 20; i++ {
			<-done
		}
	})
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	ok := 0
	for _, l := range lines {
		if strings.HasPrefix(l, "GET / 200 - - ") && strings.HasSuffix(l, " ms") {
			ok++
		}
	}
	fmt.Printf("  %d lines written, %d well-formed\n", len(lines), ok)
	fmt.Println("  note: v1.2.0 does NOT serialise writes to Stream — it calls")
	fmt.Println("  fmt.Fprintln(out, …) straight from the request goroutine. Lines stay")
	fmt.Println("  intact here only because this example's own Stream takes a mutex;")
	fmt.Println("  a bare *bytes.Buffer or *os.File would be raced on.")
	buf.Reset()

	// ------------------------------------------------------- standalone helpers
	banner("13. Standalone helpers (no middleware)")
	// Note the %0A in the path: r.URL.Path is the DECODED form, so it holds a real
	// newline.
	req := httptest.NewRequest(http.MethodGet, "/api/it%0Aems?page=2", nil)
	req.Header.Set("User-Agent", "helper-demo/1")
	req.Header.Set("X-Forwarded-For", "198.51.100.9")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.RemoteAddr = "192.0.2.10:54321"
	req.SetBasicAuth("bob", "pw")

	// HOLE: RequestURL builds from r.URL.Path (decoded), so a percent-encoded
	// newline in the request target becomes a real newline in the log line — a
	// second log-injection vector alongside the unsanitised header tokens.
	fmt.Printf("  RequestURL              = %q  <- contains a raw \\n\n", morgan.RequestURL(req))
	// ClientIP splits X-Forwarded-For on ","; FromRequest (used by the middleware)
	// does not. HOLE: the two disagree.
	fmt.Printf("  ClientIP                = %q\n", morgan.ClientIP(req))
	fmt.Printf("  RequestProtocol         = %q  (X-Forwarded-Proto honoured here)\n",
		morgan.RequestProtocol(req))
	// HOLE: no ClientIPTrustProxy / RequestProtocolTrustProxy in v1.2.0 — these
	// would not compile:
	//   morgan.ClientIPTrustProxy(req)
	//   morgan.RequestProtocolTrustProxy(req)
	fmt.Printf("  Clfdate(fixed time)     = %q\n",
		morgan.Clfdate(time.Date(2024, 11, 27, 6, 21, 42, 0, time.UTC)))
	for _, s := range []int{100, 204, 302, 404, 503, 0} {
		fmt.Printf("  status %-3d category=%-12s colorCode=%d\n", s, morgan.StatusCategory(s), morgan.StatusColorCode(s))
	}
	for _, d := range []int{0, 1, 3} {
		fmt.Printf("  FormatDuration(1234567ns, %d) = %s\n", d, morgan.FormatDuration(1234567, d))
	}
	fmt.Printf("  IP.String (empty)       = %q\n", morgan.IP(nil).String())
	fmt.Printf("  IP.String (v4)          = %q\n", morgan.IP([]byte{192, 0, 2, 10}).String())

	banner("14. FromRequest + Log.String + Compile (build a line by hand)")
	log := morgan.FromRequest(req, http.StatusCreated, 1500*time.Microsecond, 2200*time.Microsecond)
	log.RESPONSE_HEADERS = http.Header{"Content-Length": []string{"321"}}
	fmt.Printf("  Log.String():\n    %s\n", norm(strings.ReplaceAll(log.String(), "\n", "\\n")))
	// HOLE: v1.2.0's Log has no REMOTE_ADDR (string) and no PROTOCOL field — only
	// REMOTE_IP, which cannot represent a forwarded chain or a Unix-socket peer.
	// The following would not compile: log.REMOTE_ADDR, log.PROTOCOL,
	// log.RESPONSE_SIZE, log.RESPONSE_STREAMED.
	fmt.Printf("  FromRequest -> METHOD=%s URL=%q STATUS=%d REMOTE_IP=%q REMOTE_USER=%q INCOMING=%d\n",
		log.METHOD, strings.ReplaceAll(log.URL, "\n", "\\n"), log.STATUS,
		log.REMOTE_IP.String(), log.REMOTE_USER, log.INCOMING)
	// HOLE: no FromRequestTrustProxy in v1.2.0:
	//   morgan.FromRequestTrustProxy(req, 200, 0, 0)   // undefined

	line := morgan.Compile(`:remote-addr :remote-user ":method :url HTTP/:http-version" :status :res[content-length] rt=:response-time[1]ms`)
	fmt.Printf("  Compile()(req, log):\n    %s\n", strings.ReplaceAll(line(req, log), "\n", "\\n"))

	// Compile is also how you reuse a format without the middleware, e.g. to log
	// from a different transport.
	devish := morgan.Compile(":method :url :status - :response-time[0] ms")
	fmt.Printf("  reused compiled format:\n    %s\n", strings.ReplaceAll(devish(req, log), "\n", "\\n"))

	fmt.Println("\ndone.")
}
