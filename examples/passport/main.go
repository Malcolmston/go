// Command passport-example exercises github.com/malcolmston/passport v0.3.0 end
// to end against an in-process httptest server. It makes no outbound network
// calls and terminates on its own.
//
// Covered: strategy registration (Use / UseNamed), the local username/password
// strategy (success + failure), HTTP Basic and HTTP Digest (the latter driven by
// a hand-rolled client because the published httpauth package has no digest
// helpers), an opaque bearer token, a signed JWT (HS256) with forgery attempts,
// TOTP, the anonymous strategy, serialize/deserialize into a session, a route
// protected with RequireLogin, session regeneration on login, LogOut,
// AuthenticateCallback, AuthenticateAny, and the httpauth / pwhash / token
// helper packages.
package main

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/malcolmston/passport"
	"github.com/malcolmston/passport/httpauth"
	"github.com/malcolmston/passport/pwhash"
	"github.com/malcolmston/passport/strategies/anonymous"
	"github.com/malcolmston/passport/strategies/basic"
	"github.com/malcolmston/passport/strategies/bearer"
	"github.com/malcolmston/passport/strategies/digest"
	"github.com/malcolmston/passport/strategies/jwt"
	"github.com/malcolmston/passport/strategies/local"
	"github.com/malcolmston/passport/strategies/totp"
	"github.com/malcolmston/passport/token"
)

// ---------------------------------------------------------------------------
// application model
// ---------------------------------------------------------------------------

// user is this application's account model.
type user struct {
	ID         string
	Name       string
	PassHash   string // pwhash-encoded PBKDF2 hash
	APIToken   string // opaque bearer token
	TOTPSecret []byte
	Password   string // cleartext, only because HTTP Digest must recompute HA1
}

func (u *user) String() string { return "user(" + u.Name + ")" }

type store struct {
	byID   map[string]*user
	byName map[string]*user
}

func newStore() *store {
	s := &store{byID: map[string]*user{}, byName: map[string]*user{}}
	h, err := pwhash.Hash("s3cr3t-pw")
	must(err)
	u := &user{
		ID:         "1",
		Name:       "ada",
		PassHash:   h,
		Password:   "s3cr3t-pw",
		APIToken:   token.MustNew(),
		TOTPSecret: []byte("12345678901234567890"),
	}
	s.byID[u.ID] = u
	s.byName[u.Name] = u
	return s
}

// jwtSecret signs the HS256 tokens the jwt strategy verifies.
var jwtSecret = []byte("example-hs256-signing-key")

const digestRealm = "example"

// ---------------------------------------------------------------------------
// wiring
// ---------------------------------------------------------------------------

// sessionOpts is the options value used for session-creating routes.
//
// BUG (see README): Session must be set explicitly. passport.Options.Session is
// documented as "default true", but Authenticate replaces the whole default
// struct with the caller's literal, so any Options{...} that omits Session
// silently disables the login session.
var sessionOpts = passport.Options{Session: true, FailureMessage: true}

// statelessOpts authenticates without creating a session.
var statelessOpts = passport.Options{Session: false}

func buildApp(db *store) (*passport.Passport, *jwt.Strategy, http.Handler) {
	p := passport.New()
	p.SetStore(passport.NewMemoryStore())
	p.SecureCookies(false) // httptest serves plain HTTP

	// --- local username/password -----------------------------------------
	p.Use(local.New(func(username, password string) (any, error) {
		u, ok := db.byName[username]
		if !ok {
			return nil, local.ErrInvalidCredentials
		}
		ok, err := pwhash.Verify(password, u.PassHash)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, local.ErrInvalidCredentials
		}
		return u, nil
	}))

	// UseNamed registers a second instance of the same strategy type under a
	// different name — here a variant reading email/pass instead of
	// username/password.
	emailLogin := local.New(func(email, password string) (any, error) {
		u, ok := db.byName[strings.TrimSuffix(email, "@example.com")]
		if !ok {
			return nil, local.ErrInvalidCredentials
		}
		if ok, _ := pwhash.Verify(password, u.PassHash); !ok {
			return nil, local.ErrInvalidCredentials
		}
		return u, nil
	})
	emailLogin.UsernameField = "email"
	emailLogin.PasswordField = "pass"
	p.UseNamed("local-email", emailLogin)

	// --- HTTP Basic -------------------------------------------------------
	p.Use(basic.New(func(username, password string) (any, error) {
		u, ok := db.byName[username]
		if !ok {
			return nil, basic.ErrInvalidCredentials
		}
		if ok, _ := pwhash.Verify(password, u.PassHash); !ok {
			return nil, basic.ErrInvalidCredentials
		}
		return u, nil
	}))

	// --- HTTP Digest ------------------------------------------------------
	// A fixed nonce keeps the example deterministic. v0.3.0 offers no hook to
	// remember issued nonces, so the strategy has no replay protection at all.
	p.Use(digest.New(digest.Options{
		Realm: digestRealm,
		Nonce: func() string { return "example-nonce" },
		Secret: func(name string) string {
			if u, ok := db.byName[name]; ok {
				return u.Password
			}
			return ""
		},
	}))

	// --- opaque bearer token ---------------------------------------------
	p.Use(bearer.New(func(tok string) (any, error) {
		for _, u := range db.byID {
			if token.Equal(tok, u.APIToken) {
				return u, nil
			}
		}
		return nil, bearer.ErrInvalidToken
	}))

	// --- JWT (HS256) ------------------------------------------------------
	jwtStrategy := jwt.New(jwtSecret, func(c jwt.Claims) (any, error) {
		u, ok := db.byID[c.Subject()]
		if !ok {
			return nil, nil // reject: unknown subject
		}
		return u, nil
	})
	jwtStrategy.Leeway = 5 * time.Second
	p.Use(jwtStrategy)

	// --- TOTP -------------------------------------------------------------
	p.Use(totp.New(totp.Options{
		Secret: func(name string) ([]byte, error) {
			u, ok := db.byName[name]
			if !ok {
				return nil, errors.New("unknown user")
			}
			return u.TOTPSecret, nil
		},
	}))

	// --- anonymous --------------------------------------------------------
	p.Use(anonymous.New())

	// --- session bridge ---------------------------------------------------
	p.SerializeUser(func(u any) (string, error) {
		uu, ok := u.(*user)
		if !ok {
			return "", fmt.Errorf("cannot serialize %T", u)
		}
		return uu.ID, nil
	})
	p.DeserializeUser(func(id string, _ *http.Request) (any, error) {
		if u, ok := db.byID[id]; ok {
			return u, nil
		}
		return nil, nil
	})

	// --- routes -----------------------------------------------------------
	mux := http.NewServeMux()
	whoami := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "authenticated=%t user=%v", passport.IsAuthenticated(r), passport.User(r))
	})

	// Session login: on success passport.LogIn runs and sets the cookie.
	mux.Handle("/login", p.Authenticate("local", sessionOpts)(whoami))
	mux.Handle("/login-email", p.Authenticate("local-email", sessionOpts)(whoami))
	// Same strategy, but with an Options literal that forgets Session: true —
	// used below to demonstrate the default-Session bug.
	mux.Handle("/login-nosession", p.Authenticate("local", passport.Options{FailureMessage: true})(whoami))

	// Protected routes: only a request carrying a logged-in session gets in.
	mux.Handle("/profile", p.RequireLogin("")(whoami))
	mux.Handle("/profile-redirect", p.RequireLogin("/login")(whoami))

	mux.HandleFunc("/logout", func(w http.ResponseWriter, r *http.Request) {
		must(p.LogOut(w, r))
		fmt.Fprint(w, "logged out")
	})

	// Stateless routes.
	mux.Handle("/api/basic", p.Authenticate("basic", statelessOpts)(whoami))
	mux.Handle("/api/digest", p.Authenticate("digest", statelessOpts)(whoami))
	mux.Handle("/api/token", p.Authenticate("bearer", statelessOpts)(whoami))
	mux.Handle("/api/jwt", p.Authenticate("jwt", statelessOpts)(whoami))
	mux.Handle("/2fa", p.Authenticate("totp", statelessOpts)(whoami))

	// AuthenticateAny: first strategy that authenticates wins.
	mux.Handle("/api/any", p.AuthenticateAny([]string{"bearer", "basic"}, statelessOpts)(whoami))
	// The README's "optional auth" pattern: a real strategy, then anonymous.
	mux.Handle("/api/optional", p.AuthenticateAny([]string{"jwt", "anonymous"}, statelessOpts)(whoami))
	// The same idea with the single-strategy form, which does honour Pass.
	mux.Handle("/api/optional-single", p.Authenticate("anonymous", statelessOpts)(whoami))

	// Custom callback form (Passport.js's authenticate(name, cb)).
	mux.Handle("/login-callback", p.AuthenticateCallback("local",
		func(w http.ResponseWriter, r *http.Request, err error, u any, info any) {
			switch {
			case err != nil:
				http.Error(w, "error: "+err.Error(), http.StatusInternalServerError)
			case u == nil:
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprintf(w, "rejected info=%v", info)
			default:
				must(p.LogIn(w, r, u))
				fmt.Fprintf(w, "callback login ok user=%v", u)
			}
		}))

	// An unregistered strategy name.
	mux.Handle("/api/missing", p.Authenticate("saml", statelessOpts)(whoami))

	return p, jwtStrategy, passport.Chain(mux, p.Initialize(), p.Session())
}

// ---------------------------------------------------------------------------
// scenarios
// ---------------------------------------------------------------------------

func main() {
	db := newStore()
	_, jwtStrategy, handler := buildApp(db)

	srv := httptest.NewServer(handler)
	defer srv.Close()

	noRedirect := func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	jar, err := cookiejar.New(nil)
	must(err)
	client := &http.Client{Jar: jar, CheckRedirect: noRedirect}
	anon := &http.Client{CheckRedirect: noRedirect}

	section("1. a protected route rejects an unauthenticated request")
	status, body, hdr := do(anon, get(srv.URL+"/profile"))
	check(status == http.StatusUnauthorized, "GET /profile without a session => %d %q", status, strings.TrimSpace(body))
	status, _, hdr = do(anon, get(srv.URL+"/profile-redirect"))
	check(status == http.StatusFound && hdr.Get("Location") == "/login",
		`RequireLogin("/login") redirects => %d Location=%q`, status, hdr.Get("Location"))

	section("2. local strategy: bad credentials are rejected")
	status, body, _ = do(anon, form(srv.URL+"/login", url.Values{"username": {"ada"}, "password": {"wrong"}}))
	check(status == http.StatusUnauthorized, "wrong password => %d %q", status, strings.TrimSpace(body))
	status, body, _ = do(anon, form(srv.URL+"/login", url.Values{"username": {"nobody"}, "password": {"s3cr3t-pw"}}))
	check(status == http.StatusUnauthorized, "unknown user => %d %q", status, strings.TrimSpace(body))
	status, body, _ = do(anon, form(srv.URL+"/login", url.Values{"username": {"ada"}}))
	check(status == http.StatusBadRequest, "missing password => %d %q", status, strings.TrimSpace(body))

	section("3. local strategy: correct credentials log in and set a session")
	status, body, hdr = do(client, form(srv.URL+"/login", url.Values{"username": {"ada"}, "password": {"s3cr3t-pw"}}))
	check(status == http.StatusOK && strings.Contains(body, "user=user(ada)"), "login => %d %q", status, body)
	check(strings.Contains(hdr.Get("Set-Cookie"), passport.DefaultCookieName+"="),
		"session cookie => %q", hdr.Get("Set-Cookie"))
	check(strings.Contains(hdr.Get("Set-Cookie"), "HttpOnly"), "cookie is HttpOnly => %q", hdr.Get("Set-Cookie"))

	section("4. the session cookie authenticates a later request (Session middleware)")
	status, body, _ = do(client, get(srv.URL+"/profile"))
	check(status == http.StatusOK && strings.Contains(body, "authenticated=true"),
		"GET /profile with the session => %d %q", status, body)

	section("5. a forged session id does not authenticate")
	forged := get(srv.URL + "/profile")
	forged.AddCookie(&http.Cookie{Name: passport.DefaultCookieName, Value: "not-a-real-session-id"})
	status, body, _ = do(anon, forged)
	check(status == http.StatusUnauthorized, "unknown session id => %d %q", status, strings.TrimSpace(body))

	section("6. LogOut destroys the session")
	status, body, _ = do(client, get(srv.URL+"/logout"))
	check(status == http.StatusOK, "logout => %d %q", status, body)
	status, body, _ = do(client, get(srv.URL+"/profile"))
	check(status == http.StatusUnauthorized, "GET /profile after logout => %d %q", status, strings.TrimSpace(body))

	section("7. session fixation: the session id is regenerated on login")
	fixJar, err := cookiejar.New(nil)
	must(err)
	fixClient := &http.Client{Jar: fixJar, CheckRedirect: noRedirect}
	do(fixClient, get(srv.URL+"/profile")) // a pre-login visit
	before := cookieValue(fixJar, srv.URL, passport.DefaultCookieName)
	do(fixClient, form(srv.URL+"/login", url.Values{"username": {"ada"}, "password": {"s3cr3t-pw"}}))
	first := cookieValue(fixJar, srv.URL, passport.DefaultCookieName)
	do(fixClient, form(srv.URL+"/login", url.Values{"username": {"ada"}, "password": {"s3cr3t-pw"}}))
	second := cookieValue(fixJar, srv.URL, passport.DefaultCookieName)
	check(first != "" && first != before, "sid after first login differs from the pre-login sid (%q -> %q)", before, first)
	check(second != first, "re-login rotates the sid again (%q -> %q)", first, second)

	section("8. UseNamed: a second local strategy with custom field names")
	status, body, _ = do(anon, form(srv.URL+"/login-email", url.Values{"email": {"ada@example.com"}, "pass": {"s3cr3t-pw"}}))
	check(status == http.StatusOK && strings.Contains(body, "user(ada)"), "email login => %d %q", status, body)
	status, body, _ = do(anon, form(srv.URL+"/login-email", url.Values{"email": {"ada@example.com"}, "pass": {"nope"}}))
	check(status == http.StatusUnauthorized, "email login, wrong password => %d %q", status, strings.TrimSpace(body))

	section("9. local strategy also reads JSON bodies")
	jsonReq, err := http.NewRequest(http.MethodPost, srv.URL+"/login", strings.NewReader(`{"username":"ada","password":"s3cr3t-pw"}`))
	must(err)
	jsonReq.Header.Set("Content-Type", "application/json")
	status, body, _ = do(anon, jsonReq)
	check(status == http.StatusOK && strings.Contains(body, "user(ada)"), "JSON login => %d %q", status, body)

	section("10. HTTP Basic")
	req := get(srv.URL + "/api/basic")
	req.Header.Set("Authorization", httpauth.EncodeBasic("ada", "s3cr3t-pw"))
	status, body, _ = do(anon, req)
	check(status == http.StatusOK && strings.Contains(body, "user(ada)"), "basic ok => %d %q", status, body)

	req = get(srv.URL + "/api/basic")
	req.Header.Set("Authorization", httpauth.EncodeBasic("ada", "nope"))
	status, body, hdr = do(anon, req)
	check(status == http.StatusUnauthorized, "basic wrong password => %d %q", status, strings.TrimSpace(body))
	check(hdr.Get("WWW-Authenticate") == httpauth.BasicChallenge("Users"),
		"basic challenge => %q", hdr.Get("WWW-Authenticate"))

	status, body, _ = do(anon, get(srv.URL+"/api/basic"))
	check(status == http.StatusUnauthorized, "basic, no header => %d %q", status, strings.TrimSpace(body))

	req = get(srv.URL + "/api/basic")
	req.Header.Set("Authorization", "Basic !!!not-base64!!!")
	status, body, _ = do(anon, req)
	check(status == http.StatusUnauthorized, "basic, malformed header => %d %q", status, strings.TrimSpace(body))

	// httpauth round-trip, used directly.
	creds, err := httpauth.ParseBasic(httpauth.EncodeBasic("ada", "pa:ss:word"))
	must(err)
	check(creds.Username == "ada" && creds.Password == "pa:ss:word",
		"httpauth.ParseBasic round-trip => %q / %q", creds.Username, creds.Password)
	_, err = httpauth.ParseBasic(httpauth.EncodeBearer("abc"))
	check(errors.Is(err, httpauth.ErrWrongScheme), "ParseBasic on a Bearer header => %v", err)
	check(httpauth.SchemeOf(httpauth.EncodeBearer("abc")) == "bearer" &&
		httpauth.HasScheme(httpauth.EncodeBasic("a", "b"), "basic"),
		"httpauth.SchemeOf / HasScheme agree on the scheme")

	section("11. HTTP Digest (challenge / response)")
	status, _, hdr = do(anon, get(srv.URL+"/api/digest"))
	challenge := hdr.Get("WWW-Authenticate")
	check(status == http.StatusUnauthorized && strings.HasPrefix(challenge, "Digest "),
		"digest challenge => %d %q", status, challenge)
	params := parseChallenge(strings.TrimPrefix(challenge, "Digest "))

	req = get(srv.URL + "/api/digest")
	req.Header.Set("Authorization", digestHeader(params, "ada", "s3cr3t-pw", "GET", "/api/digest"))
	status, body, _ = do(anon, req)
	check(status == http.StatusOK && strings.Contains(body, "user=ada"), "digest ok => %d %q", status, body)

	req = get(srv.URL + "/api/digest")
	req.Header.Set("Authorization", digestHeader(params, "ada", "wrong-password", "GET", "/api/digest"))
	status, body, _ = do(anon, req)
	check(status == http.StatusUnauthorized, "digest wrong password => %d %q", status, strings.TrimSpace(body))

	req = get(srv.URL + "/api/digest")
	req.Header.Set("Authorization", digestHeader(params, "nobody", "s3cr3t-pw", "GET", "/api/digest"))
	status, body, _ = do(anon, req)
	check(status == http.StatusUnauthorized, "digest unknown user => %d %q", status, strings.TrimSpace(body))

	// Replay the very same captured Authorization header.
	replay := get(srv.URL + "/api/digest")
	replay.Header.Set("Authorization", digestHeader(params, "ada", "s3cr3t-pw", "GET", "/api/digest"))
	status, _, _ = do(anon, replay)
	bug("a captured digest Authorization header replays successfully (status %d): "+
		"v0.3.0 has no hook to remember issued nonces or (nonce, nc) pairs, so "+
		"anyone who observes one request can repeat it indefinitely", status)

	bug("digest c.Success(username) reports a plain string, not the app's user " +
		"object: body above says user=ada, not user=user(ada) — the strategy has " +
		"no verify callback, unlike every other credential strategy")

	section("12. opaque bearer token")
	req = get(srv.URL + "/api/token")
	req.Header.Set("Authorization", httpauth.EncodeBearer(db.byID["1"].APIToken))
	status, body, hdr = do(anon, req)
	check(status == http.StatusOK && strings.Contains(body, "user(ada)"), "bearer ok => %d %q", status, body)
	check(hdr.Get("Set-Cookie") == "", "Options{Session:false} sets no cookie => %q", hdr.Get("Set-Cookie"))

	// The published bearer strategy also accepts ?access_token=.
	status, body, _ = do(anon, get(srv.URL+"/api/token?access_token="+url.QueryEscape(db.byID["1"].APIToken)))
	check(status == http.StatusOK && strings.Contains(body, "user(ada)"), "bearer via query param => %d %q", status, body)

	req = get(srv.URL + "/api/token")
	req.Header.Set("Authorization", httpauth.EncodeBearer("not-a-real-token"))
	status, body, hdr = do(anon, req)
	check(status == http.StatusUnauthorized, "bearer bad token => %d %q", status, strings.TrimSpace(body))
	check(strings.Contains(hdr.Get("WWW-Authenticate"), `error="invalid_token"`),
		"bearer challenge => %q", hdr.Get("WWW-Authenticate"))

	section("13. JWT (HS256) issued with jwt.Sign")
	good := signJWT(jwt.Claims{"sub": "1", "exp": exp(time.Hour)})
	req = get(srv.URL + "/api/jwt")
	req.Header.Set("Authorization", "Bearer "+good)
	status, body, _ = do(anon, req)
	check(status == http.StatusOK && strings.Contains(body, "user(ada)"), "jwt ok => %d %q", status, body)

	rejected := map[string]string{
		"expired":        signJWT(jwt.Claims{"sub": "1", "exp": exp(-time.Hour)}),
		"not yet valid":  signJWT(jwt.Claims{"sub": "1", "nbf": exp(time.Hour), "exp": exp(2 * time.Hour)}),
		"unknown sub":    signJWT(jwt.Claims{"sub": "999", "exp": exp(time.Hour)}),
		"attacker's key": signWith([]byte("attacker-key"), jwt.Claims{"sub": "1", "exp": exp(time.Hour)}),
		"tampered body":  tamper(good),
		"alg=none":       algNoneToken(jwt.Claims{"sub": "1", "exp": exp(time.Hour)}),
		"garbage":        "not.a.jwt",
	}
	for name, tok := range rejected {
		req = get(srv.URL + "/api/jwt")
		req.Header.Set("Authorization", "Bearer "+tok)
		status, body, _ = do(anon, req)
		check(status == http.StatusUnauthorized, "jwt %s rejected => %d %q", name, status, strings.TrimSpace(body))
	}
	status, body, _ = do(anon, get(srv.URL+"/api/jwt"))
	check(status == http.StatusUnauthorized, "jwt, no token => %d %q", status, strings.TrimSpace(body))

	// jwt.Strategy.Parse verifies a token outside the middleware.
	claims, err := jwtStrategy.Parse(good)
	must(err)
	check(claims.Subject() == "1", "Parse => sub=%q", claims.Subject())
	_, err = jwtStrategy.Parse(tamper(good))
	check(errors.Is(err, jwt.ErrSignature), "Parse(tampered) => %v", err)

	// A token with no exp claim is accepted forever: v0.3.0 has no
	// RequireExpiry, issuer or audience checks.
	noExp := signJWT(jwt.Claims{"sub": "1"})
	req = get(srv.URL + "/api/jwt")
	req.Header.Set("Authorization", "Bearer "+noExp)
	status, _, _ = do(anon, req)
	bug("a JWT with no exp claim is accepted (status %d): v0.3.0 exposes no "+
		"RequireExpiry / Issuer / Audience option, so an unbounded, unrevocable "+
		"token and a token minted for another service both pass", status)

	section("14. TOTP second factor")
	code := totp.Generate(db.byName["ada"].TOTPSecret, time.Now())
	status, body, _ = do(anon, form(srv.URL+"/2fa", url.Values{"user": {"ada"}, "code": {code}}))
	check(status == http.StatusOK, "totp current code => %d %q", status, body)
	prev := totp.Generate(db.byName["ada"].TOTPSecret, time.Now().Add(-totp.Step))
	status, body, _ = do(anon, form(srv.URL+"/2fa", url.Values{"user": {"ada"}, "code": {prev}}))
	check(status == http.StatusOK, "totp previous window accepted (default skew 1) => %d %q", status, body)
	status, body, _ = do(anon, form(srv.URL+"/2fa", url.Values{"user": {"ada"}, "code": {"000000"}}))
	check(status == http.StatusUnauthorized, "totp wrong code => %d %q", status, strings.TrimSpace(body))
	stale := totp.Generate(db.byName["ada"].TOTPSecret, time.Now().Add(-10*totp.Step))
	status, body, _ = do(anon, form(srv.URL+"/2fa", url.Values{"user": {"ada"}, "code": {stale}}))
	check(status == http.StatusUnauthorized, "totp far-past code => %d %q", status, strings.TrimSpace(body))

	section("15. AuthenticateAny and the anonymous strategy")
	req = get(srv.URL + "/api/any")
	req.Header.Set("Authorization", httpauth.EncodeBasic("ada", "s3cr3t-pw"))
	status, body, _ = do(anon, req)
	check(status == http.StatusOK && strings.Contains(body, "user(ada)"), "any (basic leg) => %d %q", status, body)
	req = get(srv.URL + "/api/any")
	req.Header.Set("Authorization", httpauth.EncodeBearer(db.byID["1"].APIToken))
	status, body, _ = do(anon, req)
	check(status == http.StatusOK && strings.Contains(body, "user(ada)"), "any (bearer leg) => %d %q", status, body)
	status, body, _ = do(anon, get(srv.URL+"/api/any"))
	check(status == http.StatusUnauthorized, "any, no credentials => %d %q", status, strings.TrimSpace(body))

	// Single-strategy form: Pass falls through to the next handler.
	status, body, _ = do(anon, get(srv.URL+"/api/optional-single"))
	check(status == http.StatusOK && strings.Contains(body, "authenticated=false"),
		"Authenticate(\"anonymous\") passes through => %d %q", status, body)
	// AuthenticateAny form: Pass is treated as a decline and the request 401s.
	status, _, _ = do(anon, get(srv.URL+"/api/optional"))
	bug("AuthenticateAny([\"jwt\",\"anonymous\"]) returns %d, not 200: a Pass "+
		"outcome is counted as a decline and the final fallthrough always fails, "+
		"so the README's \"fall back to anonymous\" optional-auth pattern cannot "+
		"be built with AuthenticateAny", status)

	section("16. an unregistered strategy name")
	status, body, _ = do(anon, get(srv.URL+"/api/missing"))
	check(status == http.StatusInternalServerError && strings.Contains(body, "unknown strategy"),
		"unknown strategy => %d %q", status, strings.TrimSpace(body))

	section("17. AuthenticateCallback (custom callback form)")
	cbJar, err := cookiejar.New(nil)
	must(err)
	cbClient := &http.Client{Jar: cbJar, CheckRedirect: noRedirect}
	status, body, _ = do(cbClient, form(srv.URL+"/login-callback", url.Values{"username": {"ada"}, "password": {"s3cr3t-pw"}}))
	check(status == http.StatusOK && strings.Contains(body, "callback login ok"), "callback success => %d %q", status, body)
	status, body, _ = do(cbClient, get(srv.URL+"/profile"))
	check(status == http.StatusOK, "session established by the callback's LogIn => %d %q", status, body)
	status, body, _ = do(anon, form(srv.URL+"/login-callback", url.Values{"username": {"ada"}, "password": {"bad"}}))
	check(status == http.StatusForbidden && strings.Contains(body, "Invalid credentials"),
		"callback failure carries the challenge => %d %q", status, strings.TrimSpace(body))

	section("18. Options.Session does NOT default to true")
	nsJar, err := cookiejar.New(nil)
	must(err)
	nsClient := &http.Client{Jar: nsJar, CheckRedirect: noRedirect}
	_, _, hdr = do(nsClient, form(srv.URL+"/login-nosession", url.Values{"username": {"ada"}, "password": {"s3cr3t-pw"}}))
	status, _, _ = do(nsClient, get(srv.URL+"/profile"))
	bug("login through Authenticate(\"local\", Options{FailureMessage:true}) set "+
		"Set-Cookie=%q and a later GET /profile returned %d: because the caller's "+
		"Options literal left Session at its zero value, no session was created "+
		"even though the field is documented as defaulting to true. Every route "+
		"in this example that needs a session must say Session: true explicitly, "+
		"and the library's own doc.go/example_test.go login route (which passes "+
		"only SuccessRedirect) silently never logs the user in",
		hdr.Get("Set-Cookie"), status)

	section("19. helper packages: pwhash and token")
	h, err := pwhash.Hash("hunter2")
	must(err)
	ok1, err := pwhash.Verify("hunter2", h)
	must(err)
	ok2, err := pwhash.Verify("hunter3", h)
	must(err)
	check(ok1 && !ok2, "pwhash verify right=%t wrong=%t", ok1, ok2)
	h2, err := pwhash.Hash("hunter2")
	must(err)
	check(h != h2, "pwhash salts each hash (%s… vs %s…)", h[:24], h2[:24])
	alg, iter, salt, dk, err := pwhash.Decode(h)
	must(err)
	check(alg == pwhash.SHA256 && iter > 100_000 && len(salt) >= 16 && len(dk) >= 32,
		"pwhash.Decode => alg=%s iterations=%d salt=%dB key=%dB", alg, iter, len(salt), len(dk))
	_, err = pwhash.Verify("x", "not-an-encoded-hash")
	check(err != nil, "pwhash.Verify rejects a malformed encoding => %v", err)

	t1, err := token.New()
	must(err)
	t2, err := token.New()
	must(err)
	check(len(t1) >= 43 && t1 != t2, "token.New is unique and %d chars", len(t1))
	check(token.Equal(t1, t1) && !token.Equal(t1, t2), "token.Equal compares in constant time")
	n, err := token.Numeric(6)
	must(err)
	check(len(n) == 6, "token.Numeric(6) => %s", n)
	hx, err := token.Hex(8)
	must(err)
	check(len(hx) == 16, "token.Hex(8) => %s", hx)

	// HOLE: no OAuth 1.0a / OAuth 2.0 / OpenID Connect / SAML / WebAuthn
	// strategy is exercised here. Those flows need a real remote identity
	// provider (authorization and token endpoints, IdP metadata) or a browser
	// authenticator, which this offline example cannot supply. See README.md.

	fmt.Println()
	fmt.Printf("%d checks, %d failed, %d library bugs recorded\n", checks, failures, bugs)
	if failures > 0 {
		os.Exit(1)
	}
}

// ---------------------------------------------------------------------------
// JWT helpers
// ---------------------------------------------------------------------------

func exp(d time.Duration) float64 { return float64(time.Now().Add(d).Unix()) }

func signJWT(c jwt.Claims) string { return signWith(jwtSecret, c) }

func signWith(secret []byte, c jwt.Claims) string {
	s, err := jwt.Sign(secret, c)
	must(err)
	return s
}

// tamper flips a character in a JWT's payload, invalidating the signature.
func tamper(tok string) string {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 || parts[1] == "" {
		return tok
	}
	b := []byte(parts[1])
	if b[1] == 'X' {
		b[1] = 'Y'
	} else {
		b[1] = 'X'
	}
	return parts[0] + "." + string(b) + "." + parts[2]
}

// algNoneToken forges an unsigned token, the classic JWT downgrade attack.
func algNoneToken(c jwt.Claims) string {
	seg := func(v any) string {
		b, err := json.Marshal(v)
		must(err)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	return seg(map[string]string{"alg": "none", "typ": "JWT"}) + "." + seg(c) + "."
}

// ---------------------------------------------------------------------------
// Digest client (hand-rolled: v0.3.0's httpauth has no digest helpers)
// ---------------------------------------------------------------------------

func md5hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func parseChallenge(rest string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(rest, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"`)
	}
	return out
}

func digestHeader(challenge map[string]string, username, password, method, uri string) string {
	const nc, cnonce, qop = "00000001", "0a4f113b", "auth"
	realm, nonce := challenge["realm"], challenge["nonce"]
	ha1 := md5hex(username + ":" + realm + ":" + password)
	ha2 := md5hex(method + ":" + uri)
	resp := md5hex(strings.Join([]string{ha1, nonce, nc, cnonce, qop, ha2}, ":"))
	return fmt.Sprintf(`Digest username=%q, realm=%q, nonce=%q, uri=%q, qop=%s, nc=%s, cnonce=%q, response=%q`,
		username, realm, nonce, uri, qop, nc, cnonce, resp)
}

// ---------------------------------------------------------------------------
// HTTP + reporting helpers
// ---------------------------------------------------------------------------

func get(u string) *http.Request {
	r, err := http.NewRequest(http.MethodGet, u, nil)
	must(err)
	return r
}

func form(u string, v url.Values) *http.Request {
	r, err := http.NewRequest(http.MethodPost, u, strings.NewReader(v.Encode()))
	must(err)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return r
}

func do(c *http.Client, r *http.Request) (int, string, http.Header) {
	resp, err := c.Do(r)
	must(err)
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	must(err)
	return resp.StatusCode, string(b), resp.Header
}

func cookieValue(jar http.CookieJar, rawURL, name string) string {
	u, err := url.Parse(rawURL)
	must(err)
	for _, c := range jar.Cookies(u) {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

var (
	checks   int
	failures int
	bugs     int
)

func section(title string) { fmt.Printf("\n== %s\n", title) }

func check(cond bool, format string, args ...any) {
	checks++
	tag := "ok  "
	if !cond {
		tag = "FAIL"
		failures++
	}
	fmt.Printf("  %s %s\n", tag, fmt.Sprintf(format, args...))
}

// bug records an observed library defect. It is not a test failure: the example
// documents the behaviour rather than asserting the behaviour it wanted.
func bug(format string, args ...any) {
	bugs++
	fmt.Printf("  BUG  %s\n", fmt.Sprintf(format, args...))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
