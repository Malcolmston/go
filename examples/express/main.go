// Command express-example exercises the github.com/malcolmston/express port of
// Express.js: routing, route params, middleware, body parsing, error handling,
// content negotiation, sessions, SSE streaming, OpenAPI docs generation, and a
// handful of the utility/middleware subpackages.
//
// The whole program runs in-process against httptest and exits on its own.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/malcolmston/express"
	"github.com/malcolmston/express/bytes"
	"github.com/malcolmston/express/httperrors"
	"github.com/malcolmston/express/jsonwebtoken"
	"github.com/malcolmston/express/middleware/cors"
	"github.com/malcolmston/express/middleware/errorjson"
	"github.com/malcolmston/express/middleware/helmet"
	"github.com/malcolmston/express/middleware/pagination"
	"github.com/malcolmston/express/middleware/ratelimit"
	"github.com/malcolmston/express/middleware/requestid"
	"github.com/malcolmston/express/ms"
	"github.com/malcolmston/express/qs"
	"github.com/malcolmston/express/slugify"
	"github.com/malcolmston/express/statuses"
	"github.com/malcolmston/express/uuid"
)

// ---------------------------------------------------------------- test driver

type call struct {
	method  string
	path    string
	body    string
	headers map[string]string
}

// do drives the app with an in-process request and prints a one-line summary.
func do(app *express.Application, r call) *httptest.ResponseRecorder {
	var body io.Reader
	if r.body != "" {
		body = strings.NewReader(r.body)
	}
	httpReq := httptest.NewRequest(r.method, r.path, body)
	for k, v := range r.headers {
		httpReq.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httpReq)
	return rec
}

func show(label string, rec *httptest.ResponseRecorder, headers ...string) {
	out := strings.TrimRight(rec.Body.String(), "\n")
	if len(out) > 160 {
		out = out[:160] + "..."
	}
	fmt.Printf("  %-46s -> %d %s\n", label, rec.Code, out)
	for _, h := range headers {
		fmt.Printf("  %-46s    %s: %q\n", "", h, rec.Header().Get(h))
	}
}

func section(n int, title string) {
	fmt.Printf("\n=== %d. %s ===\n", n, title)
}

// -------------------------------------------------------------------- domain

type user struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

var users = map[string]user{
	"1": {ID: "1", Name: "Ada Lovelace", Email: "ada@example.com"},
	"2": {ID: "2", Name: "Grace Hopper", Email: "grace@example.com"},
}

const jwtSecretString = "s3cr3t-signing-key"

var jwtSecret = []byte(jwtSecretString)

func main() {
	app := buildApp()

	demoSettings(app)
	demoRouting(app)
	demoParamsAndRoute(app)
	demoMountedRouter(app)
	demoBodyAndErrors(app)
	demoMiddlewareSuite(app)
	demoNegotiation(app)
	demoCookiesAndSession(app)
	demoViewsAndFiles(app)
	demoStreaming(app)
	demoDocs(app)
	demoUtilityPackages()

	fmt.Println("\nAll sections completed; exiting.")
}

// ------------------------------------------------------------------ app setup

func buildApp() *express.Application {
	app := express.New()

	app.Set("env", "production")
	app.Set("app name", "express-go-example")
	app.Enable("trust proxy")

	// App-wide middleware. Registration order is execution order.
	app.Use(express.Recover())
	app.Use(requestid.New())
	app.Use(helmet.New(helmet.Options{FrameguardAction: "DENY", HSTSIncludeSubDomains: true}))
	app.Use(express.JSON())
	app.Use(express.URLEncoded())
	app.Use(express.ResponseTime())
	app.Use(express.Session(express.SessionOptions{Name: "sid"}))

	// Path-scoped middleware: only requests under /api see CORS.
	app.Use("/api", cors.New(cors.Options{
		AllowedOrigins:   []string{"https://example.com"},
		AllowCredentials: true,
		MaxAge:           600,
	}))

	// --- plain routes ---------------------------------------------------
	app.Get("/", func(req *express.Request, res *express.Response, next express.Next) {
		res.Send("Hello from " + fmt.Sprint(app.GetSetting("app name")))
	})

	app.Get("/users/:id", func(req *express.Request, res *express.Response, next express.Next) {
		u, ok := users[req.Params("id")]
		if !ok {
			next(httperrors.NotFound("no such user: " + req.Params("id")))
			return
		}
		res.Status(200).JSON(u)
	})

	// Optional parameter.
	app.Get("/greet/:name?", func(req *express.Request, res *express.Response, next express.Next) {
		name := req.Params("name")
		if name == "" {
			name = "stranger"
		}
		res.JSON(map[string]string{"greeting": "hello " + name})
	})

	// Regex-constrained parameter.
	app.Get(`/items/:id(\d+)`, func(req *express.Request, res *express.Response, next express.Next) {
		res.JSON(map[string]string{"item": req.Params("id")})
	})

	// Wildcard, captured as the "*" param.
	app.Get("/files/*", func(req *express.Request, res *express.Response, next express.Next) {
		res.JSON(map[string]string{"rest": req.Params("*")})
	})

	// app.Route chaining + app.Param preprocessing.
	app.Param("pid", func(req *express.Request, res *express.Response, next express.Next, value string) {
		if _, ok := users[value]; !ok {
			next(httperrors.BadRequest("param handler rejected pid=" + value))
			return
		}
		req.Set("loadedUser", users[value])
		next()
	})
	app.Route("/profiles/:pid").
		Get(func(req *express.Request, res *express.Response, next express.Next) {
			v, _ := req.Value("loadedUser")
			res.JSON(map[string]any{"via": "Route().Get", "user": v})
		}).
		Delete(func(req *express.Request, res *express.Response, next express.Next) {
			res.SendStatus(http.StatusNoContent)
		})

	// --- mounted sub-router with MergeParams ----------------------------
	orders := express.NewRouter(express.RouterOptions{MergeParams: true})
	orders.Use(func(req *express.Request, res *express.Response, next express.Next) {
		res.Set("X-Sub-Router", "orders")
		next()
	})
	orders.Get("/", func(req *express.Request, res *express.Response, next express.Next) {
		res.JSON(map[string]any{
			"customer": req.Params("customerId"), // inherited from the mount path
			"orders":   []string{"o-1", "o-2"},
		})
	})
	orders.Get("/:orderId", func(req *express.Request, res *express.Response, next express.Next) {
		res.JSON(map[string]string{
			"customer": req.Params("customerId"),
			"order":    req.Params("orderId"),
		})
	})
	app.Use("/customers/:customerId/orders", orders)

	// --- the /api router ------------------------------------------------
	api := express.NewRouter()
	api.Post("/users", func(req *express.Request, res *express.Response, next express.Next) {
		// HOLE: req.BodyJSON(&in) cannot be used here. express.JSON() (registered
		// app-wide above) does io.ReadAll on req.Raw.Body and never restores it,
		// and BodyJSON reads from that same drained reader and returns nil for an
		// empty body -- so BodyJSON silently yields a zero-valued struct. The two
		// documented body-reading paths are mutually exclusive; see
		// /api/bodyjson-after-json below for the demonstration. Work around it by
		// re-marshalling the already-parsed req.Body().
		raw, ok := req.Body().(map[string]any)
		if !ok {
			next(httperrors.BadRequest("expected a JSON object body"))
			return
		}
		blob, err := json.Marshal(raw)
		if err != nil {
			next(err)
			return
		}
		var in user
		if err := json.Unmarshal(blob, &in); err != nil {
			next(httperrors.BadRequest("invalid JSON body: " + err.Error()))
			return
		}
		if in.Name == "" {
			next(httperrors.New(422, "name is required"))
			return
		}
		id, err := uuid.V4()
		if err != nil {
			next(err)
			return
		}
		in.ID = id[:8]
		res.Status(http.StatusCreated).
			Set("Location", "/api/users/"+in.ID).
			JSON(in)
	})
	api.Get("/echo-body", func(req *express.Request, res *express.Response, next express.Next) {
		res.JSON(map[string]any{"parsedBody": req.Body(), "isJSON": req.Is("json")})
	})
	// Demonstrates the drained-body hole described above.
	api.Post("/bodyjson-after-json", func(req *express.Request, res *express.Response, next express.Next) {
		var in user
		err := req.BodyJSON(&in)
		res.JSON(map[string]any{
			"bodyJSONErr":    fmt.Sprint(err),
			"bodyJSONResult": in,
			"reqBody":        req.Body(),
		})
	})
	api.Get("/panic", func(req *express.Request, res *express.Response, next express.Next) {
		panic("handler panicked on purpose")
	})
	app.Use("/api", api)

	// --- a router with its own errorjson error handler --------------------
	errs := express.NewRouter()
	errs.Get("/boom", func(req *express.Request, res *express.Response, next express.Next) {
		next(fmt.Errorf("something exploded deep in the handler"))
	})
	errs.Get("/teapot", func(req *express.Request, res *express.Response, next express.Next) {
		// HOLE: errorjson uses a single fixed Options.Status; it does not look at
		// *httperrors.Error, so a 418 arrives at the client as the configured 500.
		next(httperrors.New(http.StatusTeapot, "short and stout"))
	})
	// Router-local error handler (4-arg express.ErrorHandler).
	errs.Use(errorjson.New())
	app.Use("/errors", errs)

	// --- pagination + rate limiting on a dedicated prefix ---------------
	feed := express.NewRouter()
	feed.Use(pagination.New(pagination.Options{DefaultLimit: 5, MaxLimit: 10}))
	feed.Use(ratelimit.New(ratelimit.Options{Max: 2, Window: time.Minute}))
	feed.Get("/", func(req *express.Request, res *express.Response, next express.Next) {
		p := pagination.From(req)
		res.JSON(map[string]any{"page": p.Page, "limit": p.Limit, "offset": p.Offset})
	})
	app.Use("/feed", feed)

	// --- content negotiation --------------------------------------------
	app.Get("/negotiate", func(req *express.Request, res *express.Response, next express.Next) {
		res.Format(map[string]func(){
			"json": func() { res.JSON(map[string]string{"format": "json"}) },
			"html": func() { res.Type("html").Send("<p>html</p>") },
			"text": func() { res.Type("text").Send("plain text") },
		})
	})
	app.Get("/accepts", func(req *express.Request, res *express.Response, next express.Next) {
		res.JSON(map[string]any{
			"accepts":   req.Accepts("json", "html"),
			"languages": req.AcceptsLanguages("en", "fr"),
			"encodings": req.AcceptsEncodings("gzip", "identity"),
			"ip":        req.IP(),
			"protocol":  req.Protocol(),
			// HOLE: Hostname() reads only Host; it ignores X-Forwarded-Host even
			// with app.Enable("trust proxy"), unlike req.IP()/req.Protocol().
			"hostname": req.Hostname(),
			"xhr":      req.Xhr(),
			"baseURL":  req.BaseURL(),
		})
	})

	// --- cookies, sessions, JWT ------------------------------------------
	app.Get("/login", func(req *express.Request, res *express.Response, next express.Next) {
		sess := req.Session()
		sess.Set("user", "ada")
		token, err := jsonwebtoken.Sign(
			jsonwebtoken.Claims{"sub": "1", "role": "admin"},
			jwtSecret,
			&jsonwebtoken.SignOptions{Alg: "HS256", ExpiresIn: time.Hour, Issuer: "example"},
		)
		if err != nil {
			next(err)
			return
		}
		res.Cookie("theme", "dark", &express.CookieOptions{HTTPOnly: true, Path: "/"}).
			JSON(map[string]string{"token": token})
	})
	app.Get("/whoami", func(req *express.Request, res *express.Response, next express.Next) {
		res.JSON(map[string]any{
			"sessionUser": req.Session().GetString("user"),
			"themeCookie": req.Cookie("theme"),
		})
	})
	app.Get("/protected", func(req *express.Request, res *express.Response, next express.Next) {
		auth := strings.TrimPrefix(req.Get("Authorization"), "Bearer ")
		// HOLE: the published v0.4.0 Verify takes only (token, secret) -- there is
		// no VerifyOptions, so an algorithm allowlist and issuer/audience/subject
		// checks are not available. Those claims must be re-checked by hand.
		claims, err := jsonwebtoken.Verify(auth, jwtSecret)
		if err == nil && claims["iss"] != "example" {
			err = fmt.Errorf("unexpected issuer %v", claims["iss"])
		}
		if err != nil {
			next(httperrors.Unauthorized("bad token: " + err.Error()))
			return
		}
		res.JSON(map[string]any{"sub": claims["sub"], "role": claims["role"]})
	})

	// --- views, static files, file downloads ------------------------------
	app.Set("views", "views")
	app.Set("view engine", "html")
	app.Enable("view cache")
	// A custom engine registered for a new extension.
	app.Engine(".mustache", func(path string, data any) (string, error) {
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		out := string(raw)
		if m, ok := data.(map[string]any); ok {
			for k, v := range m {
				out = strings.ReplaceAll(out, "{{"+k+"}}", fmt.Sprint(v))
			}
		}
		return out, nil
	})
	// HOLE: a plain Handler mounted under a path prefix is NOT given the residual
	// path (only mounted *Routers are), so express.Static under "/static" looks
	// for public/static/<file> and always 404s. Mounting it at the root works.
	app.Use("/static", express.Static("public"))
	app.Use(express.Static("public"))
	app.Get("/render", func(req *express.Request, res *express.Response, next express.Next) {
		list := make([]user, 0, len(users))
		for _, id := range []string{"1", "2"} {
			list = append(list, users[id])
		}
		if err := res.Render("index", map[string]any{"Title": "Users", "Users": list}); err != nil {
			next(err)
		}
	})
	app.Get("/render-custom", func(req *express.Request, res *express.Response, next express.Next) {
		if err := res.Render("card.mustache", map[string]any{"name": "Ada", "role": "admin"}); err != nil {
			next(err)
		}
	})
	app.Get("/sendfile", func(req *express.Request, res *express.Response, next express.Next) {
		if err := res.SendFile("public/hello.txt"); err != nil {
			next(err)
		}
	})
	app.Get("/download", func(req *express.Request, res *express.Response, next express.Next) {
		if err := res.Download("public/hello.txt", "greeting.txt"); err != nil {
			next(err)
		}
	})
	app.Get("/etag", func(req *express.Request, res *express.Response, next express.Next) {
		res.ETag(`"v1"`).LastModified(time.Unix(1700000000, 0).UTC())
		if res.Fresh() {
			res.NotModified()
			return
		}
		res.Send("fresh body")
	})

	// --- streaming / SSE --------------------------------------------------
	app.Get("/chunked", func(req *express.Request, res *express.Response, next express.Next) {
		if err := res.SendChunked([]byte("abcdefghij"), 4); err != nil {
			next(err)
		}
	})
	app.Get("/events", func(req *express.Request, res *express.Response, next express.Next) {
		sse := res.SSE()
		_ = sse.Comment("stream open")
		_ = sse.Send("tick", "1")
		_ = sse.SendJSON("payload", map[string]int{"n": 2})
		_ = sse.SendID("3", "tick", "3")
	})

	// --- app-level error handler (runs for anything not handled above) ----
	app.Use(func(err error, req *express.Request, res *express.Response, next express.Next) {
		status := http.StatusInternalServerError
		msg := "internal error"
		if he, ok := err.(*httperrors.Error); ok {
			status = he.Status
			if he.Expose {
				msg = he.Message
			}
		} else {
			msg = err.Error()
		}
		res.Status(status).JSON(map[string]any{
			"error":  msg,
			"status": status,
			"reason": statuses.Message(status),
		})
	})

	// --- OpenAPI documentation metadata ----------------------------------
	app.Describe("GET", "/users/:id", express.RouteDoc{
		Summary:     "Fetch a user by id",
		Tags:        []string{"users"},
		OperationID: "getUser",
		Responses: map[string]express.ResponseDoc{
			"200": {Description: "the user", Schema: map[string]any{"type": "object"}},
			"404": {Description: "not found"},
		},
	})
	app.Describe("POST", "/api/users", express.RouteDoc{
		Summary: "Create a user",
		Tags:    []string{"users"},
		RequestBody: &express.BodyDoc{
			Required: true,
			Schema:   map[string]any{"type": "object", "required": []string{"name"}},
		},
	})
	app.Channel("user.created", express.ChannelDoc{
		Description: "emitted when a user is created",
		Subscribe:   &express.MessageDoc{Name: "UserCreated", ContentType: "application/json"},
	})

	return app
}

// ------------------------------------------------------------------- sections

func demoSettings(app *express.Application) {
	section(1, "Application settings")
	fmt.Printf("  env=%v  app name=%v\n", app.GetSetting("env"), app.GetSetting("app name"))
	fmt.Printf("  trust proxy enabled=%v  x-powered-by enabled=%v  view engine=%v\n",
		app.Enabled("trust proxy"), app.Enabled("x-powered-by"), app.GetSetting("view engine"))
	app.Locals()["buildID"] = "example-1"
	fmt.Printf("  app.Locals()=%v\n", app.Locals())
}

func demoRouting(app *express.Application) {
	section(2, "Routing, params, wildcards")
	show("GET /", do(app, call{method: "GET", path: "/"}), "X-Request-Id", "X-Frame-Options")
	show("GET /users/1", do(app, call{method: "GET", path: "/users/1"}))
	show("GET /users/99 (404 via httperrors)", do(app, call{method: "GET", path: "/users/99"}))
	show("GET /greet (optional param omitted)", do(app, call{method: "GET", path: "/greet"}))
	show("GET /greet/ada", do(app, call{method: "GET", path: "/greet/ada"}))
	show(`GET /items/42 (regex :id(\d+))`, do(app, call{method: "GET", path: "/items/42"}))
	show(`GET /items/abc (regex miss -> 404)`, do(app, call{method: "GET", path: "/items/abc"}))
	show("GET /files/a/b/c.txt (wildcard)", do(app, call{method: "GET", path: "/files/a/b/c.txt"}))
	show("GET /nope (unmatched)", do(app, call{method: "GET", path: "/nope"}))
	// HOLE: no HEAD -> GET fallback; Express answers HEAD with the GET headers.
	show("HEAD /users/1 (no HEAD->GET fallback)", do(app, call{method: "HEAD", path: "/users/1"}))
}

func demoParamsAndRoute(app *express.Application) {
	section(3, "app.Route chaining and app.Param preprocessing")
	show("GET /profiles/2", do(app, call{method: "GET", path: "/profiles/2"}))
	show("DELETE /profiles/1", do(app, call{method: "DELETE", path: "/profiles/1"}))
	show("GET /profiles/999 (param handler rejects)", do(app, call{method: "GET", path: "/profiles/999"}))
}

func demoMountedRouter(app *express.Application) {
	section(4, "Mounted router with MergeParams")
	show("GET /customers/77/orders",
		do(app, call{method: "GET", path: "/customers/77/orders"}), "X-Sub-Router")
	show("GET /customers/77/orders/o-1",
		do(app, call{method: "GET", path: "/customers/77/orders/o-1"}))
}

func demoBodyAndErrors(app *express.Application) {
	section(5, "Body parsing and error handling")
	jsonHdr := map[string]string{"Content-Type": "application/json"}
	show("POST /api/users (valid)", do(app, call{
		method: "POST", path: "/api/users",
		body: `{"name":"Alan Turing","email":"alan@example.com"}`, headers: jsonHdr,
	}), "Location")
	show("POST /api/users (missing name -> 422)", do(app, call{
		method: "POST", path: "/api/users", body: `{"email":"x@y.z"}`, headers: jsonHdr,
	}))
	// HOLE: express.JSON() calls next(err) on a malformed body, but Router.handle
	// does not skip mounted sub-routers while an error is propagating, so the
	// /api route handler still runs as if nothing had failed. The 400 below comes
	// from the handler's own guard, not from the body parser's error.
	show("POST /api/users (malformed JSON, error leaks)", do(app, call{
		method: "POST", path: "/api/users", body: `{not json`, headers: jsonHdr,
	}))
	show("GET /api/echo-body", do(app, call{
		method: "GET", path: "/api/echo-body", body: `{"a":1,"b":[2,3]}`, headers: jsonHdr,
	}))
	show("POST /api/bodyjson-after-json (HOLE)", do(app, call{
		method: "POST", path: "/api/bodyjson-after-json",
		body: `{"name":"Ada","email":"ada@x.z"}`, headers: jsonHdr,
	}))
	show("GET /errors/boom (errorjson handler)", do(app, call{method: "GET", path: "/errors/boom"}))
	show("GET /errors/teapot (418 flattened to 500)", do(app, call{method: "GET", path: "/errors/teapot"}))
	show("GET /api/panic (express.Recover)", do(app, call{method: "GET", path: "/api/panic"}))
	show("GET /protected (no token -> 401)", do(app, call{method: "GET", path: "/protected"}))
}

func demoMiddlewareSuite(app *express.Application) {
	section(6, "Middleware subpackages: cors, pagination, ratelimit, requestid, helmet")
	show("OPTIONS /api/users (CORS preflight)", do(app, call{
		method: "OPTIONS", path: "/api/users",
		headers: map[string]string{
			"Origin":                        "https://example.com",
			"Access-Control-Request-Method": "POST",
		},
	}), "Access-Control-Allow-Origin", "Access-Control-Allow-Credentials", "Access-Control-Max-Age")
	show("GET /feed?page=3&limit=4", do(app, call{method: "GET", path: "/feed?page=3&limit=4"}))
	show("GET /feed?limit=999 (clamped to MaxLimit)", do(app, call{method: "GET", path: "/feed?limit=999"}))
	show("GET /feed (3rd hit -> rate limited)", do(app, call{method: "GET", path: "/feed"}),
		"Retry-After", "X-RateLimit-Limit", "X-RateLimit-Remaining")

	rec := do(app, call{method: "GET", path: "/", headers: map[string]string{"X-Request-Id": "caller-supplied"}})
	fmt.Printf("  %-46s -> reused id %q\n", "GET / with inbound X-Request-Id", rec.Header().Get("X-Request-Id"))
	fmt.Printf("  %-46s -> HSTS %q, CSP present=%v\n", "helmet headers",
		rec.Header().Get("Strict-Transport-Security"),
		rec.Header().Get("Content-Security-Policy") != "")
	// HOLE: helmet documents that it strips X-Powered-By via an OnBeforeWrite
	// hook, but writeHeaderOnce runs the hooks *before* it re-adds the header,
	// so the hook is a no-op. app.Disable("x-powered-by") is the real fix.
	fmt.Printf("  %-46s -> %q (helmet hook did not strip it)\n",
		"X-Powered-By with helmet", rec.Header().Get("X-Powered-By"))
	app.Disable("x-powered-by")
	rec2 := do(app, call{method: "GET", path: "/"})
	fmt.Printf("  %-46s -> %q\n", "X-Powered-By after app.Disable", rec2.Header().Get("X-Powered-By"))
	app.Enable("x-powered-by")
	fmt.Printf("  %-46s -> %q\n", "express.ResponseTime()", rec.Header().Get("X-Response-Time"))
}

func demoNegotiation(app *express.Application) {
	section(7, "Content negotiation")
	for _, accept := range []string{"application/json", "text/html", "text/plain"} {
		show("GET /negotiate Accept: "+accept, do(app, call{
			method: "GET", path: "/negotiate", headers: map[string]string{"Accept": accept},
		}), "Content-Type")
	}
	show("GET /accepts", do(app, call{
		method: "GET", path: "/accepts",
		headers: map[string]string{
			"Accept":           "text/html;q=0.9, application/json",
			"Accept-Language":  "fr-CH, fr;q=0.9, en;q=0.5",
			"Accept-Encoding":  "gzip, deflate",
			"X-Requested-With": "XMLHttpRequest",
			"X-Forwarded-For":  "203.0.113.7",
			"X-Forwarded-Host": "api.example.com",
		},
	}))
}

func demoCookiesAndSession(app *express.Application) {
	section(8, "Cookies, sessions, JWT round-trip")
	login := do(app, call{method: "GET", path: "/login"})
	cookies := login.Result().Cookies()
	var jar []string
	for _, c := range cookies {
		jar = append(jar, c.Name+"="+c.Value)
	}
	fmt.Printf("  %-46s -> %d cookie(s): %s\n", "GET /login", len(cookies), strings.Join(jar, "; "))

	var payload map[string]string
	_ = json.Unmarshal(login.Body.Bytes(), &payload)
	token := payload["token"]
	fmt.Printf("  %-46s -> %s...\n", "signed JWT", token[:min(len(token), 40)])

	show("GET /whoami (with cookie jar)", do(app, call{
		method: "GET", path: "/whoami",
		headers: map[string]string{"Cookie": strings.Join(jar, "; ")},
	}))
	show("GET /protected (valid Bearer token)", do(app, call{
		method: "GET", path: "/protected",
		headers: map[string]string{"Authorization": "Bearer " + token},
	}))
	show("GET /protected (tampered token)", do(app, call{
		method: "GET", path: "/protected",
		headers: map[string]string{"Authorization": "Bearer " + token + "x"},
	}))
}

func demoViewsAndFiles(app *express.Application) {
	section(9, "Views, static files, downloads, conditional GET")
	show("GET /render (built-in html/template)", do(app, call{method: "GET", path: "/render"}), "Content-Type")
	show("GET /render-custom (app.Engine)", do(app, call{method: "GET", path: "/render-custom"}))
	show("GET /static/hello.txt (express.Static)", do(app, call{method: "GET", path: "/static/hello.txt"}), "Content-Type")
	show("GET /hello.txt (Static mounted at root)", do(app, call{method: "GET", path: "/hello.txt"}), "Content-Type")
	show("GET /missing.txt (falls through to 404)", do(app, call{method: "GET", path: "/missing.txt"}))
	show("GET /sendfile (res.SendFile)", do(app, call{method: "GET", path: "/sendfile"}), "Content-Type")
	show("GET /download (res.Download)", do(app, call{method: "GET", path: "/download"}), "Content-Disposition")
	show("GET /etag (first hit)", do(app, call{method: "GET", path: "/etag"}), "ETag", "Last-Modified")
	show("GET /etag (If-None-Match -> 304)", do(app, call{
		method: "GET", path: "/etag", headers: map[string]string{"If-None-Match": `"v1"`},
	}))
}

func demoStreaming(app *express.Application) {
	section(10, "Streaming and Server-Sent Events")
	show("GET /chunked", do(app, call{method: "GET", path: "/chunked"}))
	rec := do(app, call{method: "GET", path: "/events"})
	fmt.Printf("  %-46s -> %d, Content-Type %q\n", "GET /events", rec.Code, rec.Header().Get("Content-Type"))
	for _, line := range strings.Split(strings.TrimRight(rec.Body.String(), "\n"), "\n") {
		fmt.Printf("      %s\n", line)
	}
}

func demoDocs(app *express.Application) {
	section(11, "Route introspection and OpenAPI/AsyncAPI generation")
	routes := app.Routes()
	fmt.Printf("  app.Routes() reported %d routes; first 8:\n", len(routes))
	for i, r := range routes {
		if i >= 8 {
			break
		}
		fmt.Printf("      %-7s %-42s openapi=%s\n", r.Method, r.Path, r.OpenAPIPath())
	}

	doc := app.OpenAPI()
	fmt.Printf("  OpenAPI: version=%s title=%q paths=%d tags=%d\n",
		doc.OpenAPI, doc.Info.Title, len(doc.Paths), len(doc.Tags))
	if op, ok := doc.Paths["/users/{id}"]; ok {
		if get, ok := op["get"]; ok {
			fmt.Printf("  GET /users/{id}: summary=%q operationId=%q responses=%v\n",
				get.Summary, get.OperationID, sortedKeys(get.Responses))
		}
	}
	yaml := app.OpenAPIYAML()
	fmt.Printf("  OpenAPIYAML() produced %d bytes, first line: %q\n",
		len(yaml), strings.SplitN(yaml, "\n", 2)[0])

	async := app.AsyncAPI()
	fmt.Printf("  AsyncAPI: version=%s channels=%v\n", async.AsyncAPI, app.ChannelNames())
}

func demoUtilityPackages() {
	section(12, "Utility subpackages")
	d, err := ms.Parse("1.5h")
	fmt.Printf("  ms.Parse(\"1.5h\")            = %v (err=%v)\n", d, err)
	fmt.Printf("  ms.FormatLong(90*time.Minute) = %q\n", ms.FormatLong(90*time.Minute))

	n, err := bytes.Parse("1.5mb")
	fmt.Printf("  bytes.Parse(\"1.5mb\")         = %d (err=%v)\n", n, err)
	fmt.Printf("  bytes.Format(1048576)         = %q\n", bytes.Format(1048576))

	fmt.Printf("  slugify(\"Héllo, Wörld! 2024\") = %q\n",
		slugify.Slugify("Héllo, Wörld! 2024", slugify.Options{Lower: true, Strict: true}))

	id, _ := uuid.V4()
	fmt.Printf("  uuid.V4()                     = %s (valid=%v)\n", id, uuid.Validate(id))
	v5, _ := uuid.V5("6ba7b810-9dad-11d1-80b4-00c04fd430c8", "example.com")
	fmt.Printf("  uuid.V5(dns, example.com)     = %s\n", v5)

	parsed := qs.Parse("a[b][c]=1&list[]=x&list[]=y")
	fmt.Printf("  qs.Parse nested/array         = %v\n", parsed)
	fmt.Printf("  qs.Stringify round-trip       = %q\n", qs.Stringify(parsed))

	fmt.Printf("  statuses.Message(418)         = %q\n", statuses.Message(418))
	code, err := statuses.Code("Not Found")
	fmt.Printf("  statuses.Code(\"Not Found\")    = %d (err=%v)\n", code, err)
	fmt.Printf("  statuses.IsRedirect(302)=%v IsRetry(503)=%v IsEmpty(204)=%v\n",
		statuses.IsRedirect(302), statuses.IsRetry(503), statuses.IsEmpty(204))

	e := httperrors.Forbidden("")
	fmt.Printf("  httperrors.Forbidden(\"\")      = status=%d msg=%q expose=%v isHTTPError=%v\n",
		e.Status, e.Message, e.Expose, httperrors.IsHTTPError(e))

	tok, _ := jsonwebtoken.Sign(jsonwebtoken.Claims{"sub": "42"}, jwtSecret,
		&jsonwebtoken.SignOptions{ExpiresIn: time.Minute})
	decoded, _ := jsonwebtoken.Decode(tok)
	fmt.Printf("  jsonwebtoken.Decode           = sub=%v exp set=%v\n", decoded["sub"], decoded["exp"] != nil)
	if _, err := jsonwebtoken.Verify(tok, []byte("wrong-secret")); err != nil {
		fmt.Printf("  Verify with wrong secret      = rejected (%v)\n", err)
	}
}

func sortedKeys(m map[string]express.OpenAPIResponse) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
