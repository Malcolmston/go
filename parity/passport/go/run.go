// Command run is the Go-side runner for parity/passport.
//
// It speaks JSON Lines on stdio (see ../../HARNESS.md). For each case it
// rebuilds a fresh github.com/malcolmston/passport application (so no session or
// nonce state leaks between cases), replays the request(s) described by the case
// against it over loopback, and emits the authentication OUTCOME.
//
// Nothing here talks to a remote identity provider: every strategy compared is
// self-contained (local, http basic, http digest, http bearer).
//
// Determinism / normalisation, applied identically by the node runner:
//   - the digest challenge nonce is replaced with the literal <NONCE>
//   - session ids are never emitted, only setCookiePresent
//   - user is normalised to the principal's username (the Go digest port yields
//     a bare username string where upstream yields a user object)
//   - Date/Content-Length and every other volatile header are dropped
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/malcolmston/passport"
	"github.com/malcolmston/passport/strategies/basic"
	"github.com/malcolmston/passport/strategies/bearer"
	"github.com/malcolmston/passport/strategies/digest"
	"github.com/malcolmston/passport/strategies/local"
)

// ------------------------------------------------------------ fixed fixtures

const (
	realm        = "Parity"
	adminRealm   = "Admin"
	fixedNonce   = "deadbeefdeadbeefdeadbeefdeadbeef"
	sessionCooki = passport.DefaultCookieName // "passport.sid"
)

type user struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Password string `json:"-"`
}

// users is the fixed in-memory store. The node runner holds the same table.
var users = map[string]*user{
	"alice": {ID: "u-alice", Username: "alice", Password: "s3cr3t-alice"},
	"bob":   {ID: "u-bob", Username: "bob", Password: "hunter2"},
}

// tokens are the fixed opaque bearer tokens.
var tokens = map[string]string{"tok-alice": "alice", "tok-bob": "bob"}

func byID(id string) *user {
	for _, u := range users {
		if u.ID == id {
			return u
		}
	}
	return nil
}

// principal reduces whatever a strategy handed back to the authenticated
// username, so that the two ports' differing user shapes stay comparable.
func principal(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		return x
	case *user:
		return x.Username
	case user:
		return x.Username
	default:
		return fmt.Sprint(x)
	}
}

// ------------------------------------------------------------ app assembly

func buildApp() http.Handler {
	p := passport.New()

	p.SerializeUser(func(u any) (string, error) {
		if uu, ok := u.(*user); ok {
			return uu.ID, nil
		}
		return "", errors.New("cannot serialize user")
	})
	p.DeserializeUser(func(id string, _ *http.Request) (any, error) {
		if u := byID(id); u != nil {
			return u, nil
		}
		return nil, errors.New("unknown session user")
	})

	verify := func(username, password string) (any, error) {
		u, ok := users[username]
		if !ok || u.Password != password {
			return nil, nil
		}
		return u, nil
	}

	p.Use(local.New(verify))

	b := basic.New(verify)
	b.Realm = realm
	p.UseNamed("basic", b)

	ba := basic.New(verify)
	ba.Realm = adminRealm
	p.UseNamed("basic-admin", ba)

	// A fresh (nonce, nc) replay ledger per app, mirroring the node runner's
	// `validate` callback: a spent pair re-issues the challenge. buildApp() is
	// called once per case, so no replay state leaks across cases.
	spent := map[string]bool{}
	p.UseNamed("digest", digest.New(digest.Options{
		Realm: realm,
		Secret: func(name string) string {
			if u, ok := users[name]; ok {
				return u.Password
			}
			return ""
		},
		Nonce: func() string { return fixedNonce },
		ValidateNonce: func(nonce, nc string) bool {
			key := nonce + ":" + nc
			if spent[key] {
				return false
			}
			spent[key] = true
			return true
		},
	}))

	be := bearer.New(func(token string) (any, error) {
		name, ok := tokens[token]
		if !ok {
			return nil, nil
		}
		return users[name], nil
	})
	be.Realm = realm
	p.UseNamed("bearer", be)

	// Terminal handler: reports the authentication outcome of the request.
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"__parity":      true,
			"authenticated": passport.IsAuthenticated(r),
			"user":          principal(passport.User(r)),
		})
	})

	mux := http.NewServeMux()
	route := func(path string, mw ...passport.Middleware) {
		mux.Handle(path, passport.Chain(ok, mw...))
	}

	// --- local ---------------------------------------------------------------
	route("/login/local", p.Authenticate("local", passport.Options{Session: passport.WithSession()}))
	route("/login/local-nosession", p.Authenticate("local", passport.Options{Session: passport.NoSession()}))
	// Options with no explicit Session field: Options.Session is a *bool, so the
	// zero value is nil, which the port treats as the Passport.js default
	// (session:true) — matching upstream's authenticate('local', {successRedirect}).
	route("/login/local-defaults", p.Authenticate("local", passport.Options{SuccessRedirect: "/ok"}))
	route("/login/local-failredirect", p.Authenticate("local", passport.Options{Session: passport.WithSession(), FailureRedirect: "/denied"}))

	// --- http basic ----------------------------------------------------------
	route("/basic", p.Authenticate("basic", passport.Options{Session: passport.NoSession()}))
	route("/basic-admin", p.Authenticate("basic-admin", passport.Options{Session: passport.NoSession()}))

	// --- http digest ---------------------------------------------------------
	route("/digest", p.Authenticate("digest", passport.Options{Session: passport.NoSession()}))
	route("/digest/other", p.Authenticate("digest", passport.Options{Session: passport.NoSession()}))

	// --- bearer --------------------------------------------------------------
	route("/bearer", p.Authenticate("bearer", passport.Options{Session: passport.NoSession()}))

	// --- session / route protection -----------------------------------------
	route("/whoami")
	route("/protected", p.RequireLogin(""))
	mux.Handle("/logout", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := p.LogOut(w, r); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		ok.ServeHTTP(w, r)
	}))
	route("/ok")
	mux.Handle("/denied", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		ok.ServeHTTP(w, r)
	}))

	return passport.Chain(mux, p.Initialize(), p.Session())
}

// ------------------------------------------------------------ request replay

type reqSpec struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
	Form    map[string]string `json:"form"`
	JSON    map[string]any    `json:"json"`
	Raw     *string           `json:"raw"`
	Session string            `json:"session"`
}

type outcome struct {
	Status           int  `json:"status"`
	Authenticated    bool `json:"authenticated"`
	User             any  `json:"user"`
	WWWAuthenticate  any  `json:"wwwAuthenticate"`
	SetCookiePresent bool `json:"setCookiePresent"`
	Location         any  `json:"location"`
}

var nonceRe = regexp.MustCompile(`nonce="[^"]*"`)

func normaliseChallenge(v string, present bool) any {
	if !present {
		return nil
	}
	return nonceRe.ReplaceAllString(v, `nonce="<NONCE>"`)
}

func cookieHeader(mode string, jar map[string]string) (string, bool) {
	if mode == "" || mode == "none" {
		return "", false
	}
	names := make([]string, 0, len(jar))
	for k := range jar {
		names = append(names, k)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, k := range names {
		v := jar[k]
		if mode == "tamper" && k == sessionCooki {
			last := v[len(v)-1:]
			repl := "a"
			if last == "a" {
				repl = "b"
			}
			v = v[:len(v)-1] + repl
		}
		parts = append(parts, k+"="+v)
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, "; "), true
}

func replay(client *http.Client, base string, spec reqSpec, jar map[string]string) (outcome, error) {
	headers := http.Header{}
	for k, v := range spec.Headers {
		headers.Set(k, v)
	}
	var body io.Reader
	switch {
	case spec.Form != nil:
		vals := url.Values{}
		keys := make([]string, 0, len(spec.Form))
		for k := range spec.Form {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			vals.Set(k, spec.Form[k])
		}
		headers.Set("content-type", "application/x-www-form-urlencoded")
		body = strings.NewReader(vals.Encode())
	case spec.JSON != nil:
		b, err := json.Marshal(spec.JSON)
		if err != nil {
			return outcome{}, err
		}
		headers.Set("content-type", "application/json")
		body = strings.NewReader(string(b))
	case spec.Raw != nil:
		body = strings.NewReader(*spec.Raw)
	}

	method := spec.Method
	if method == "" {
		method = http.MethodGet
	}
	path := spec.Path
	if path == "" {
		path = "/"
	}
	req, err := http.NewRequest(method, base+path, body)
	if err != nil {
		return outcome{}, err
	}
	req.Header = headers
	if ck, present := cookieHeader(spec.Session, jar); present {
		req.Header.Set("cookie", ck)
	}

	resp, err := client.Do(req)
	if err != nil {
		return outcome{}, err
	}
	defer resp.Body.Close()

	setCookiePresent := false
	for _, c := range resp.Cookies() {
		if c.Value != "" {
			jar[c.Name] = c.Value
		} else {
			delete(jar, c.Name)
		}
		if c.Name == sessionCooki && c.Value != "" {
			setCookiePresent = true
		}
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return outcome{}, err
	}
	out := outcome{Status: resp.StatusCode, SetCookiePresent: setCookiePresent}
	var parsed struct {
		Parity        bool `json:"__parity"`
		Authenticated bool `json:"authenticated"`
		User          any  `json:"user"`
	}
	if json.Unmarshal(raw, &parsed) == nil && parsed.Parity {
		out.Authenticated = parsed.Authenticated
		out.User = parsed.User
	}
	wa, waOK := resp.Header["Www-Authenticate"]
	if waOK && len(wa) > 0 {
		out.WWWAuthenticate = normaliseChallenge(strings.Join(wa, ", "), true)
	}
	if loc := resp.Header.Get("Location"); loc != "" {
		out.Location = loc
	}
	return out, nil
}

// ------------------------------------------------------------ JSON Lines loop

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

func main() {
	var mu sync.Mutex
	current := buildApp()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		h := current
		mu.Unlock()
		h.ServeHTTP(w, r)
	}))
	defer srv.Close()

	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	stdout := bufio.NewWriter(os.Stdout)
	defer stdout.Flush()
	emit := func(v any) {
		b, err := json.Marshal(v)
		if err != nil {
			b = []byte(`{"ok":false,"error":"marshal failure"}`)
		}
		stdout.Write(b)
		stdout.WriteByte('\n')
		stdout.Flush()
	}

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<24)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			emit(map[string]any{"ok": false, "error": "bad request line: " + err.Error()})
			continue
		}
		value, err := func() (v any, err error) {
			// Never exit on a failing case: a panic in the port becomes ok:false.
			defer func() {
				if p := recover(); p != nil {
					err = fmt.Errorf("panic: %v", p)
				}
			}()
			mu.Lock()
			current = buildApp() // fresh state per case
			mu.Unlock()
			jar := map[string]string{}
			switch req.Fn {
			case "request":
				if len(req.Args) < 1 {
					return nil, errors.New("request: missing spec")
				}
				var spec reqSpec
				if err := json.Unmarshal(req.Args[0], &spec); err != nil {
					return nil, err
				}
				return replay(client, srv.URL, spec, jar)
			case "sequence":
				if len(req.Args) < 1 {
					return nil, errors.New("sequence: missing specs")
				}
				var specs []reqSpec
				if err := json.Unmarshal(req.Args[0], &specs); err != nil {
					return nil, err
				}
				outs := make([]outcome, 0, len(specs))
				for _, spec := range specs {
					o, err := replay(client, srv.URL, spec, jar)
					if err != nil {
						return nil, err
					}
					outs = append(outs, o)
				}
				return outs, nil
			default:
				return nil, errors.New("unknown fn: " + req.Fn)
			}
		}()
		if err != nil {
			emit(map[string]any{"id": req.ID, "ok": false, "error": err.Error()})
			continue
		}
		emit(map[string]any{"id": req.ID, "ok": true, "value": value})
	}
}
