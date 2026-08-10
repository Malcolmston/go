// Command axios-example exercises the github.com/malcolmston/axios HTTP client
// against an in-process httptest server. No outbound network calls are made.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/malcolmston/axios"
)

type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Age  int    `json:"age"`
}

// flaky counts hits to /flaky so it can fail twice before succeeding.
var flaky atomic.Int32

func newServer() *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/users/1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Echo-Query", r.URL.RawQuery)
		w.Header().Set("X-Echo-Auth", r.Header.Get("Authorization"))
		w.Header().Set("X-Echo-Trace", r.Header.Get("X-Trace"))
		w.Header().Set("X-Echo-App", r.Header.Get("X-App"))
		w.Header().Set("X-Echo-Accept", r.Header.Get("Accept"))
		_ = json.NewEncoder(w).Encode(User{ID: 1, Name: "Ada", Age: 36})
	})

	mux.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"method":       r.Method,
			"contentType":  r.Header.Get("Content-Type"),
			"receivedBody": string(body),
		})
	})

	mux.HandleFunc("/form", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "ct=%s body=%s", r.Header.Get("Content-Type"), string(body))
	})

	mux.HandleFunc("/missing", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"no such user"}`))
	})

	mux.HandleFunc("/teapot", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("short and stout"))
	})

	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
			_, _ = w.Write([]byte("finally"))
		case <-r.Context().Done():
		}
	})

	mux.HandleFunc("/flaky", func(w http.ResponseWriter, r *http.Request) {
		n := flaky.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("try again"))
			return
		}
		_, _ = fmt.Fprintf(w, "ok after %d attempts", n)
	})

	mux.HandleFunc("/large", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		for i := 0; i < 5; i++ {
			fmt.Fprintf(w, "chunk-%d\n", i)
		}
	})

	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		n, _ := io.Copy(io.Discard, r.Body)
		fmt.Fprintf(w, "received %d bytes (%s)", n, r.Header.Get("Content-Type"))
	})

	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("alpha"))
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("beta"))
	})

	mux.HandleFunc("/headers", func(w http.ResponseWriter, r *http.Request) {
		keys := []string{"X-Common", "X-Get-Only", "X-Post-Only"}
		var parts []string
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s=%q", k, r.Header.Get(k)))
		}
		fmt.Fprint(w, strings.Join(parts, " "))
	})

	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, r.URL.RawQuery)
	})

	mux.HandleFunc("/cookie", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "s3cret", Path: "/"})
		w.Header().Set("Retry-After", "7")
		w.Header().Set("Location", "/next")
		w.WriteHeader(http.StatusFound)
	})

	return httptest.NewServer(mux)
}

func section(title string) {
	fmt.Printf("\n=== %s ===\n", title)
}

func main() {
	srv := newServer()
	defer srv.Close()

	client := axios.New(axios.Config{
		BaseURL:     srv.URL,
		Timeout:     3 * time.Second,
		BearerToken: "secret-token",
		Headers:     http.Header{"X-App": {"demo"}},
		HeaderGroups: axios.HeaderDefaults{
			Common: http.Header{"X-Common": {"everywhere"}},
			Get:    http.Header{"X-Get-Only": {"get"}},
			Post:   http.Header{"X-Post-Only": {"post"}},
		},
		RequestInterceptors: []axios.RequestInterceptor{
			func(req *http.Request) error {
				req.Header.Set("X-Trace", "trace-123")
				return nil
			},
		},
		ResponseInterceptors: []axios.ResponseInterceptor{
			func(resp *axios.Response) error {
				fmt.Printf("  [interceptor] %s -> %d (%d bytes)\n",
					resp.Request.URL.Path, resp.Status, len(resp.Body))
				return nil
			},
		},
	})

	// ---------------------------------------------------------------- GET/JSON
	section("GET + query params + JSON decode")
	resp, err := client.Get("/users/1", &axios.RequestConfig{
		Params:  url.Values{"expand": {"profile"}},
		Headers: http.Header{"Accept": {"application/json"}},
	})
	if err != nil {
		fatal(err)
	}
	var u User
	if err := resp.JSON(&u); err != nil {
		fatal(err)
	}
	fmt.Printf("  status=%d (%s) ok=%v user=%+v\n", resp.Status, resp.StatusText, resp.OK(), u)
	fmt.Printf("  content-type=%q content-length=%d\n", resp.ContentType(), resp.ContentLength())
	fmt.Printf("  echoed query=%q auth=%q trace=%q app=%q accept=%q\n",
		resp.Header("X-Echo-Query"), resp.Header("X-Echo-Auth"),
		resp.Header("X-Echo-Trace"), resp.Header("X-Echo-App"),
		resp.Header("X-Echo-Accept"))

	// ------------------------------------------------------------ header group
	section("Method-scoped default header groups")
	hr, err := client.Get("/headers")
	if err != nil {
		fatal(err)
	}
	fmt.Println("  GET :", hr.Text())
	hr, err = client.Post("/headers", nil)
	if err != nil {
		fatal(err)
	}
	fmt.Println("  POST:", hr.Text())

	// ----------------------------------------------------------------- POST
	section("POST JSON body (auto-encoded from struct)")
	pr, err := client.Post("/users", User{ID: 2, Name: "Grace", Age: 45})
	if err != nil {
		fatal(err)
	}
	fmt.Printf("  status=%d body=%s", pr.Status, pr.Text())

	section("POST url.Values (auto form-urlencoded)")
	fr, err := client.Post("/form", url.Values{"a": {"1"}, "b": {"two"}})
	if err != nil {
		fatal(err)
	}
	fmt.Println("  ", fr.Text())

	section("PUT / PATCH / DELETE / HEAD / OPTIONS")
	for _, call := range []struct {
		name string
		fn   func() (*axios.Response, error)
	}{
		{"PUT", func() (*axios.Response, error) { return client.Put("/users", map[string]any{"n": 1}) }},
		{"PATCH", func() (*axios.Response, error) { return client.Patch("/users", map[string]any{"n": 2}) }},
		{"DELETE", func() (*axios.Response, error) { return client.Delete("/users") }},
		{"HEAD", func() (*axios.Response, error) { return client.Head("/users") }},
		{"OPTIONS", func() (*axios.Response, error) { return client.Options("/users") }},
	} {
		r, err := call.fn()
		if err != nil {
			fmt.Printf("  %-8s error: %v\n", call.name, err)
			continue
		}
		fmt.Printf("  %-8s status=%d bodyLen=%d\n", call.name, r.Status, len(r.Body))
	}

	// ------------------------------------------------------------ generics
	section("Generic helpers GetJSON / PostJSON")
	got, err := axios.GetJSON[User](client, "/users/1")
	if err != nil {
		fatal(err)
	}
	fmt.Printf("  GetJSON[User] -> %+v\n", got)
	echo, err := axios.PostJSON[map[string]any](client, "/users", User{Name: "Linus"})
	if err != nil {
		fatal(err)
	}
	fmt.Printf("  PostJSON[map] -> method=%v ct=%v\n", echo["method"], echo["contentType"])

	// -------------------------------------------------------- error handling
	section("Error handling: non-2xx yields *axios.Error carrying the Response")
	_, err = client.Get("/missing")
	var aerr *axios.Error
	if errors.As(err, &aerr) {
		fmt.Printf("  err=%v\n  code=%s status=%d body=%s\n",
			aerr, aerr.Code, aerr.StatusCode(), strings.TrimSpace(aerr.Response.Text()))
		fmt.Printf("  IsAxiosError=%v isClientError=%v\n",
			axios.IsAxiosError(err), aerr.Response.IsClientError())
		j, _ := json.Marshal(aerr.ToJSON())
		fmt.Printf("  ToJSON=%s\n", j)
	} else {
		fmt.Printf("  UNEXPECTED: err was %T %v\n", err, err)
	}

	section("Custom ValidateStatus accepts 418")
	tr, err := client.Get("/teapot", &axios.RequestConfig{
		ValidateStatus: func(status int) bool { return status == http.StatusTeapot },
	})
	if err != nil {
		fatal(err)
	}
	fmt.Printf("  status=%d body=%q (treated as success)\n", tr.Status, tr.Text())

	// ------------------------------------------------------------- timeouts
	section("Per-request timeout")
	start := time.Now()
	_, err = client.Get("/slow", &axios.RequestConfig{Timeout: 150 * time.Millisecond})
	fmt.Printf("  after %v err=%v\n", time.Since(start).Round(10*time.Millisecond), err)
	if e := axios.AsError(err); e != nil {
		fmt.Printf("  code=%s IsCancel=%v\n", e.Code, axios.IsCancel(err))
	}

	section("AbortController cancellation")
	ctrl := axios.NewAbortController()
	go func() {
		time.Sleep(100 * time.Millisecond)
		ctrl.Abort(nil)
	}()
	_, err = client.Get("/slow", &axios.RequestConfig{Signal: ctrl.Signal()})
	fmt.Printf("  err=%v IsCancel=%v aborted=%v\n", err, axios.IsCancel(err), ctrl.Signal().Aborted())

	section("Legacy CancelToken")
	tok, cancel := axios.NewCancelToken()
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel("user navigated away")
	}()
	_, err = client.Get("/slow", &axios.RequestConfig{CancelToken: tok})
	fmt.Printf("  err=%v IsCancel=%v\n", err, axios.IsCancel(err))

	// -------------------------------------------------------------- retries
	section("Automatic retries on 5xx")
	retrying := client.Create(axios.Config{
		Retry: &axios.RetryConfig{
			Retries: 3,
			Backoff: func(attempt int) time.Duration { return 10 * time.Millisecond },
		},
	})
	rr, err := retrying.Get("/flaky")
	if err != nil {
		fatal(err)
	}
	fmt.Printf("  status=%d body=%q\n", rr.Status, rr.Text())

	// ------------------------------------------------------------ transforms
	section("TransformRequest / TransformResponse")
	tres, err := client.Post("/form", "hello", &axios.RequestConfig{
		ContentType: "text/plain",
		TransformRequest: []axios.RequestTransform{
			func(body []byte, h http.Header) ([]byte, error) {
				h.Set("X-Transformed", "yes")
				return []byte(strings.ToUpper(string(body))), nil
			},
		},
		TransformResponse: []axios.ResponseTransform{
			func(body []byte, h http.Header) ([]byte, error) {
				return append([]byte("[transformed] "), body...), nil
			},
		},
	})
	if err != nil {
		fatal(err)
	}
	fmt.Println("  ", tres.Text())

	// ------------------------------------------------------- query serialization
	section("Query serialization: ArrayFormat + nested ParamsMap + GetUri")
	for name, f := range map[string]axios.ArrayFormat{
		"repeat":   axios.ArrayFormatRepeat,
		"brackets": axios.ArrayFormatBrackets,
		"indices":  axios.ArrayFormatIndices,
		"comma":    axios.ArrayFormatComma,
	} {
		format := f
		sr, err := client.Get("/search", &axios.RequestConfig{
			Params:      url.Values{"id": {"1", "2", "3"}},
			ArrayFormat: &format,
		})
		if err != nil {
			fatal(err)
		}
		fmt.Printf("  %-8s -> %s\n", name, sr.Text())
	}
	nested, err := client.Get("/search", &axios.RequestConfig{
		ParamsMap: map[string]any{"filter": map[string]any{"tag": "go", "n": 3}},
	})
	if err != nil {
		fatal(err)
	}
	fmt.Printf("  nested   -> %s\n", nested.Text())

	uri, err := client.GetUri("/search", &axios.RequestConfig{Params: url.Values{"q": {"a b"}}})
	if err != nil {
		fatal(err)
	}
	fmt.Printf("  GetUri   -> %s\n", uri)

	custom, err := client.Get("/search", &axios.RequestConfig{
		Params:           url.Values{"x": {"1"}, "y": {"2"}},
		ParamsSerializer: func(p url.Values) string { return "custom=" + p.Get("x") + p.Get("y") },
	})
	if err != nil {
		fatal(err)
	}
	fmt.Printf("  custom   -> %s\n", custom.Text())

	// --------------------------------------------------------------- progress
	section("Upload/download progress callbacks")
	payload := strings.Repeat("x", 64*1024)
	var upCalls, downCalls int
	up, err := client.Post("/upload", payload, &axios.RequestConfig{
		ContentType: "text/plain",
		OnUploadProgress: func(e axios.ProgressEvent) {
			upCalls++
			if upCalls <= 2 {
				fmt.Printf("  up   %d/%d (%.0f%%)\n", e.Loaded, e.Total, e.Progress()*100)
			}
		},
		OnDownloadProgress: func(e axios.ProgressEvent) {
			downCalls++
			fmt.Printf("  down %d/%d computable=%v\n", e.Loaded, e.Total, e.LengthComputable)
		},
	})
	if err != nil {
		fatal(err)
	}
	fmt.Printf("  server said: %s (upCalls=%d downCalls=%d)\n", up.Text(), upCalls, downCalls)

	// -------------------------------------------------------------- streaming
	section("Streaming response (ResponseStream)")
	st := axios.ResponseStream
	sresp, err := client.Get("/large", &axios.RequestConfig{ResponseType: &st})
	if err != nil {
		fatal(err)
	}
	n, err := io.Copy(prefixWriter{os.Stdout, "  | "}, sresp.Stream)
	if err != nil {
		fatal(err)
	}
	_ = sresp.Close()
	fmt.Printf("  streamed %d bytes\n", n)

	// ---------------------------------------------------- redirects & helpers
	section("Redirect control + response helpers (cookies, Location, Retry-After)")
	noFollow := client.Create(axios.Config{
		MaxRedirects:   -1,
		ValidateStatus: func(s int) bool { return s < 400 },
	})
	cr, err := noFollow.Get("/cookie")
	if err != nil {
		fatal(err)
	}
	loc, _ := cr.Location()
	ra, ok := cr.RetryAfter()
	fmt.Printf("  status=%d isRedirect=%v location=%v retryAfter=%v(%v)\n",
		cr.Status, cr.IsRedirect(), loc, ra, ok)
	for _, c := range cr.Cookies() {
		fmt.Printf("  cookie %s=%s\n", c.Name, c.Value)
	}

	// ------------------------------------------------------------- concurrency
	section("All / Spread")
	resps, err := axios.All(
		func() (*axios.Response, error) { return client.Get("/a") },
		func() (*axios.Response, error) { return client.Get("/b") },
	)
	if err != nil {
		fatal(err)
	}
	joined := axios.Spread(func(rs ...*axios.Response) string {
		parts := make([]string, len(rs))
		for i, r := range rs {
			parts[i] = r.Text()
		}
		return strings.Join(parts, "+")
	})(resps)
	fmt.Println("  joined:", joined)

	// -------------------------------------------------------------- FormData
	section("FormData multipart upload")
	fd := axios.NewFormData()
	fd.AddField("title", "report")
	fd.AddFileBytes("file", "notes.txt", []byte("line one\nline two\n"))
	fdBody, err := fd.Reader()
	if err != nil {
		fatal(err)
	}
	fdResp, err := client.Post("/upload", fdBody, &axios.RequestConfig{ContentType: fd.ContentType()})
	if err != nil {
		fatal(err)
	}
	fmt.Printf("  parts=%d boundary=%s\n  server said: %s\n", fd.Len(), fd.Boundary(), fdResp.Text())

	// --------------------------------------------------------------- misc utils
	section("Misc utilities")
	fmt.Printf("  IsAbsoluteURL(%q)=%v\n", "https://x/y", axios.IsAbsoluteURL("https://x/y"))
	fmt.Printf("  CombineURLs=%q\n", axios.CombineURLs("https://x/api/", "/users"))
	fmt.Printf("  SerializeParams comma=%q\n",
		axios.SerializeParams(url.Values{"a": {"1", "2"}}, axios.ArrayFormatComma))
	fmt.Printf("  FlattenParams=%v\n",
		axios.FlattenParams(map[string]any{"p": map[string]any{"q": 1}}))
	ftj := axios.FormToJSON(url.Values{"user[name]": {"Ada"}, "user[tags][0]": {"go"}})
	ftjJSON, _ := json.Marshal(ftj)
	fmt.Printf("  FormToJSON=%s\n", ftjJSON)
	fmt.Printf("  EncodeURIComponent(%q)=%q\n", "a b/c?", axios.EncodeURIComponent("a b/c?"))
	merged := axios.MergeConfig(
		axios.Config{BaseURL: "https://base", Headers: http.Header{"A": {"1"}}},
		axios.Config{Headers: http.Header{"B": {"2"}}},
	)
	fmt.Printf("  MergeConfig baseURL=%s headers=%v\n", merged.BaseURL, merged.Headers)

	section("Package-level default client")
	axios.SetDefault(client)
	dr, err := axios.Get("/a")
	if err != nil {
		fatal(err)
	}
	fmt.Printf("  axios.Get(\"/a\") -> %q (default==client: %v)\n", dr.Text(), axios.Default() == client)

	fmt.Println("\nAll axios examples completed.")
}

type prefixWriter struct {
	w      io.Writer
	prefix string
}

func (p prefixWriter) Write(b []byte) (int, error) {
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	for _, l := range lines {
		fmt.Fprintf(p.w, "%s%s\n", p.prefix, l)
	}
	return len(b), nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "FATAL:", err)
	os.Exit(1)
}
