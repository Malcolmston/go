// Library + FAQ content, ported from the original static landing page.
export interface Lib {
  id: string; name: string; icon: string; accent: string; pkg: string; node: string;
  // source is the upstream ecosystem being ported ("Node.js" by default, "Python"
  // for the Python-port libraries). Drives the compare-column / heading labels.
  source?: string;
  repo: string; docs: string; tagline: string; blurb: string; tags: string[];
  features: string[]; node_code: string; go_code: string; integrate: string;
}
export const NODE_ACCENT = '#8cc84b';
export const LIBS: Lib[] = [
  {
    id:"express", name:"Express", icon:'<i class="fa-solid fa-route"></i>', accent:"#00add8",
    pkg:"github.com/malcolmston/express", node:"expressjs/express",
    repo:"https://github.com/malcolmston/express", docs:"https://malcolmston.github.io/express/",
    tagline:"Fast, unopinionated, minimalist web framework.",
    blurb:"Routing, middleware chains, views, content negotiation and streaming — the Express you know, in Go. "+
      "The same three-argument handler signature, path patterns (:param, :id?, :id(\\\\d+), *), and mountable routers.",
    tags:["routing","middleware","views","SSE / streaming","QUERY method","190+ packages"],
    features:[
      "Handlers with the classic <code>(req, res, next)</code> shape",
      "Path-to-regexp params: <code>:id</code>, optional <code>:id?</code>, regex <code>:id(\\d+)</code>, wildcard <code>*</code>",
      "Routers with <code>CaseSensitive</code> / <code>Strict</code> / <code>MergeParams</code> options",
      "Views via <code>html/template</code>, <code>res.Render</code> / <code>res.SendFile</code>",
      "Content negotiation, SSE &amp; chunked streaming, the new <code>QUERY</code> HTTP method",
      "Batteries: 100+ middleware + utility ports (<code>ms</code>, <code>bytes</code>, <code>cookie</code>, <code>qs</code>, <code>jsonwebtoken</code>, <code>uuid</code>, <code>lodash/*</code> …)",
      "<code>express.WrapHandler</code> mounts any <code>net/http</code> handler"
    ],
    node_code:
`const express = require('express')
const app = express()

app.get('/users/:id', (req, res) => {
  res.json({ id: req.params.id })
})

app.listen(3000)`,
    go_code:
`app := express.New()

app.Get("/users/:id", func(req *express.Request,
    res *express.Response, next express.Next) {
    res.JSON(map[string]any{"id": req.Params("id")})
})

app.Listen(":3000")`,
    integrate:
`<span class="tok-c">// Mount a router, add middleware, stream Server-Sent Events</span>
api := express.NewRouter(express.RouterOptions{MergeParams: true})
api.Use(func(req *express.Request, res *express.Response, next express.Next) {
    res.Set("X-App", "demo"); next()
})
api.Get("/events", func(req *express.Request, res *express.Response, next express.Next) {
    sse := res.SSE()                 <span class="tok-c">// sets SSE headers + flushes</span>
    sse.Send("tick", "hello")        <span class="tok-c">// event: tick\\ndata: hello</span>
})
app.Use("/api", api)`
  },
  {
    id:"passport", name:"Passport", icon:'<i class="fa-solid fa-shield-halved"></i>', accent:"#7ee787",
    pkg:"github.com/malcolmston/passport", node:"jaredhanson/passport",
    repo:"https://github.com/malcolmston/passport", docs:"https://malcolmston.github.io/passport/",
    tagline:"Simple, unobtrusive authentication.",
    blurb:"A strategy-based auth middleware for net/http. 100+ strategies — local, basic, bearer, JWT, OAuth2 with "+
      "60+ providers, WebAuthn passkeys, and OpenID Connect with RS256/JWKS — plus session serialize/deserialize.",
    tags:["100+ strategies","OAuth2","WebAuthn","JWT / JWKS","OIDC","sessions"],
    features:[
      "Strategy interface with <code>Success / Fail / Redirect / Error / Pass</code>",
      "Local, Basic, Bearer, API-key, HMAC, TOTP/HOTP, magic-link, client-cert …",
      "OAuth2 base + 60+ providers: Google, GitHub, Facebook, Slack, Discord, Apple …",
      "OpenID Connect id_tokens verified via <b>RS256/ES256 JWKS</b> (Google, Auth0, Okta, Azure)",
      "WebAuthn / passkeys (CBOR + COSE, ES256/RS256 assertions)",
      "Session <code>SerializeUser</code> / <code>DeserializeUser</code>, pluggable <code>Store</code>",
      "<code>RequireLogin</code> gate, custom callbacks, multi-strategy, <code>passReq</code>"
    ],
    node_code:
`const passport = require('passport')
const { Strategy } = require('passport-local')

passport.use(new Strategy((user, pw, done) => {
  if (user === 'alice' && pw === 'password123')
    return done(null, { id: '1', name: 'Alice' })
  return done(null, false)
}))`,
    go_code:
`p := passport.New()

p.Use(local.New(func(user, pw string) (any, error) {
    if user == "alice" && pw == "password123" {
        return User{ID: "1", Name: "Alice"}, nil
    }
    return nil, local.ErrInvalidCredentials
}))`,
    integrate:
`<span class="tok-c">// Sessions + a guarded route</span>
p.SerializeUser(func(u any) (string, error) { return u.(User).ID, nil })
p.DeserializeUser(func(id string, r *http.Request) (any, error) {
    return lookupUserByID(id), nil
})

mux := http.NewServeMux()
mux.Handle("/login", p.Authenticate("local")(welcomeHandler))
mux.Handle("/profile", p.RequireLogin("/login")(profileHandler))

handler := passport.Chain(mux, p.Initialize(), p.Session())
http.ListenAndServe(":3000", handler)`
  },
  {
    id:"socketio", name:"Socket.IO", icon:'<i class="fa-solid fa-bolt"></i>', accent:"#d2a8ff",
    pkg:"github.com/malcolmston/socketio", node:"socketio/socket.io",
    repo:"https://github.com/malcolmston/socket.io", docs:"https://malcolmston.github.io/socket.io/",
    tagline:"Bidirectional, low-latency, event-based communication.",
    blurb:"A dependency-free Socket.IO server (and Go client) speaking the real wire protocol — Engine.IO v4 "+
      "(polling + WebSocket + upgrade) and Socket.IO v5 (namespaces, rooms, acks, binary). The WebSocket layer is "+
      "RFC 6455 from scratch. Interoperates with socket.io-client@4.",
    tags:["Engine.IO v4","Socket.IO v5","RFC6455 WS","rooms","binary","Redis scale-out"],
    features:[
      "Both transports: HTTP long-polling and WebSocket, with the polling→WS upgrade",
      "Namespaces, rooms, broadcasting, acknowledgements (both directions)",
      "Per-socket data store — the equivalent of <code>socket.data</code>",
      "Connection middleware (<code>io.Use</code>), binary attachments",
      "Pluggable <code>Adapter</code> + a Redis <code>Broadcaster</code> for multi-node scale-out",
      "Mounts alongside Express via <code>io.Handler(app)</code>",
      "Ships a Go client with reconnection and <code>EmitWithAck</code>"
    ],
    node_code:
`const { Server } = require('socket.io')
const io = new Server(3000)

io.on('connection', (socket) => {
  socket.join('general')
  socket.on('chat', (msg) => {
    io.to('general').emit('chat', msg)
  })
})`,
    go_code:
`io := socketio.New()

io.OnConnection(func(s *socketio.Socket) {
    s.Join("general")
    s.On("chat", func(args []any) []any {
        io.To("general").Emit("chat", args...)
        return nil
    })
})
http.Handle(socketio.DefaultPath, io)`,
    integrate:
`<span class="tok-c">// Acks, per-socket data, and Redis scale-out</span>
io.OnConnection(func(s *socketio.Socket) {
    s.Set("user", currentUser)          <span class="tok-c">// socket.data</span>
    s.On("ping", func(args []any) []any {
        return []any{"pong"}            <span class="tok-c">// acknowledgement reply</span>
    })
})

bc, _ := redis.New(redis.Options{Addr: "localhost:6379"})
io.SetBroadcaster(bc)                    <span class="tok-c">// fan out across nodes</span>`
  },
  {
    id:"chalk", name:"chalk", icon:'<i class="fa-solid fa-palette"></i>', accent:"#ffa657",
    pkg:"github.com/malcolmston/chalk", node:"chalk/chalk",
    repo:"https://github.com/malcolmston/chalk", docs:"https://malcolmston.github.io/chalk/",
    tagline:"Terminal string styling done right — plus prompts &amp; figlet.",
    blurb:"Chainable, immutable ANSI styling (16 / 256 / truecolor) with automatic capability degradation and "+
      "NO_COLOR / FORCE_COLOR support. Includes chalk/prompts (inquirer-style interactive prompts) and "+
      "chalk/figlet (ASCII-art banners with bundled fonts, gradients and rainbow).",
    tags:["ANSI colors","truecolor","NO_COLOR","prompts","figlet","gradients"],
    features:[
      "Chainable styles: <code>chalk.New().Red().Bold().Sprint(\"x\")</code>",
      "Modifiers (bold, dim, italic, underline, inverse, strikethrough …) and bg colors",
      "Truecolor / 256-color: <code>RGB</code>, <code>Hex</code>, <code>Ansi256</code>, auto-degrading",
      "<code>chalk.Strip</code> and <code>chalk.VisibleLength</code> helpers",
      "<b>chalk/prompts</b> — input, confirm, select, multiselect, password (raw-mode TTY)",
      "<b>chalk/figlet</b> — FIGfont rendering, bundled distinct fonts, gradient &amp; rainbow coloring"
    ],
    node_code:
`const chalk = require('chalk')

console.log(chalk.red.bold('error!'))
console.log(chalk.green('ok'))
console.log(chalk.hex('#ff8800').underline('orange'))`,
    go_code:
`import "github.com/malcolmston/chalk"

fmt.Println(chalk.New().Red().Bold().Sprint("error!"))
fmt.Println(chalk.Green("ok"))
fmt.Println(chalk.New().Hex("#ff8800").Underline().Sprint("orange"))`,
    integrate:
`<span class="tok-c">// Prompts and figlet banners</span>
import (
    "github.com/malcolmston/chalk/prompts"
    "github.com/malcolmston/chalk/figlet"
)

name, _ := prompts.Input(prompts.InputConfig{Message: "Your name?"})
ok, _   := prompts.Confirm(prompts.ConfirmConfig{Message: "Proceed?"})

fmt.Println(figlet.Render("Hello"))                 <span class="tok-c">// default font</span>
banner, _ := figlet.RenderFont("banner", "GO")      <span class="tok-c">// bundled outline font</span>
fmt.Println(figlet.RenderRainbow(banner))`
  },
  {
    id:"morgan", name:"morgan", icon:'<i class="fa-solid fa-scroll"></i>', accent:"#f778ba",
    pkg:"github.com/malcolmston/morgan", node:"expressjs/morgan",
    repo:"https://github.com/malcolmston/morgan", docs:"https://malcolmston.github.io/morgan/",
    tagline:"HTTP request logger middleware.",
    blurb:"Wrap any http.Handler to log every request in a named format (combined, common, dev, short, tiny, json) "+
      "or your own :token string. Custom tokens, skip predicates, immediate mode and buffered output — just like the "+
      "Node original.",
    tags:["access logs","named formats","custom tokens","JSON","skip","buffering"],
    features:[
      "Named formats: <code>Combined</code>, <code>Common</code>, <code>Dev</code>, <code>Short</code>, <code>Tiny</code>, <code>JSON</code>",
      "Raw format strings with <code>:method :url :status :response-time</code> tokens",
      "Register custom tokens and named formats",
      "<code>Skip</code> predicate (e.g. only log errors), <code>Immediate</code> mode",
      "Buffered writes at an interval, any <code>io.Writer</code> stream",
      "Auto-detects a TTY to enable/disable dev colors"
    ],
    node_code:
`const morgan = require('morgan')
const express = require('express')
const app = express()

app.use(morgan('dev'))
app.listen(8080)`,
    go_code:
`import "github.com/malcolmston/morgan"

mux := http.NewServeMux()
mux.HandleFunc("/", handler)

http.ListenAndServe(":8080",
    morgan.New(mux, morgan.Dev, morgan.Config{}))`,
    integrate:
`<span class="tok-c">// Custom token, JSON output, only log errors</span>
morgan.Token("id", func(r *http.Request, log morgan.Log, args ...string) string {
    return r.Header.Get("X-Request-Id")
})

h := morgan.New(mux, morgan.JSON, morgan.Config{
    Skip: func(r *http.Request, status int) bool { return status < 400 },
})
http.ListenAndServe(":8080", h)`
  },
  {
    id:"fastmcp", name:"fastmcp", icon:'<i class="fa-solid fa-plug"></i>', accent:"#58a6ff",
    pkg:"github.com/malcolmston/fastmcp", node:"jlowin/fastmcp", source:"Python",
    repo:"https://github.com/malcolmston/fastmcp", docs:"https://malcolmston.github.io/fastmcp/",
    tagline:"Build Model Context Protocol servers, fast.",
    blurb:"A from-scratch, standard-library-only framework for building MCP (Model Context Protocol) servers in Go — "+
      "an idiomatic port of Python's FastMCP. Register tools, resources and prompts as ordinary Go functions; the "+
      "JSON-RPC 2.0 plumbing, reflected JSON schemas and transports (stdio + HTTP) are handled for you.",
    tags:["MCP","tools","resources","prompts","JSON-RPC","stdio + HTTP"],
    features:[
      "Register tools from plain Go funcs — args &amp; JSON schema reflected from struct tags",
      "Resources and prompts with the same ergonomic registration",
      "Newline-delimited <b>JSON-RPC 2.0</b> over stdio (the default transport)",
      "Streamable <b>HTTP</b> transport via <code>ServeHTTP</code> / <code>HTTPHandler</code>",
      "Server options: <code>WithVersion</code>, <code>WithInstructions</code>",
      "Zero third-party dependencies — Go standard library only"
    ],
    node_code:
`from fastmcp import FastMCP

mcp = FastMCP("demo")

@mcp.tool()
def add(a: int, b: int) -> int:
    return a + b

mcp.run()`,
    go_code:
`type AddArgs struct {
    A int \`json:"a"\`
    B int \`json:"b"\`
}

s := fastmcp.New("demo", fastmcp.WithVersion("1.0.0"))

s.Tool("add", "Add two integers together",
    func(ctx context.Context, args AddArgs) (any, error) {
        return args.A + args.B, nil
    })

s.Run(context.Background())`,
    integrate:
`<span class="tok-c">// Expose a resource, then serve over HTTP instead of stdio</span>
s.Resource("config://app", "config", "App configuration", "application/json",
    func(ctx context.Context) (string, error) {
        return \`{"theme":"dark"}\`, nil
    })

log.Fatal(s.ServeHTTP(":8080"))          <span class="tok-c">// streamable HTTP transport</span>`
  },
  {
    id:"streamlit", name:"Streamlit", icon:'<i class="fa-solid fa-sliders"></i>', accent:"#ff4b4b",
    pkg:"github.com/malcolmston/streamlit", node:"streamlit/streamlit", source:"Python",
    repo:"https://github.com/malcolmston/streamlit", docs:"https://malcolmston.github.io/streamlit/",
    tagline:"Turn a Go function into an interactive data app.",
    blurb:"A from-scratch, dependency-free port of Streamlit. Write an app function, call display and widget methods "+
      "top-to-bottom on a Session, and st.Run serves it over HTTP with a small embedded web UI — no JavaScript build "+
      "step, no external charting library, standard library only.",
    tags:["widgets","reactive re-run","charts","HTTP server","embedded UI","stdlib-only"],
    features:[
      "Write an app as <code>func(s *st.Session)</code>, run top-to-bottom",
      "Widgets: <code>Button</code>, <code>Slider</code>, <code>Checkbox</code>, <code>TextInput</code>, <code>SelectBox</code> …",
      "Display: <code>Title</code>, <code>Header</code>, <code>Markdown</code>, <code>Write</code>, <code>Table</code>, <code>Metric</code>",
      "Built-in <code>LineChart</code> — no external charting library",
      "Automatic re-run on interaction with per-session state",
      "<code>st.Run(app, \":8501\")</code> serves it with an embedded web UI"
    ],
    node_code:
`import streamlit as st

st.title("Hello, Streamlit")

if st.button("go"):
    st.write("clicked!")

n = st.slider("n", 0, 100, 50)
st.write(n)`,
    go_code:
`func app(s *st.Session) {
    s.Title("Hello, Streamlit-Go")

    if s.Button("go") {
        s.Write("clicked!")
    }

    n := s.Slider("n", 0, 100, 50, 1)
    s.Write(n)
}

st.Run(app, ":8501")`,
    integrate:
`<span class="tok-c">// A sidebar control, a metric and a live chart</span>
func app(s *st.Session) {
    s.Sidebar().Header("Controls")
    smooth := s.Sidebar().Checkbox("smooth", true)

    s.Metric("Users", "1,204", "+3.2%")
    s.LineChart(series(smooth))      <span class="tok-c">// []float64, no external chart lib</span>
}`
  },
  {
    id:"algebra", name:"algebra", icon:'<i class="fa-solid fa-square-root-variable"></i>', accent:"#f2cc60",
    pkg:"github.com/malcolmston/algebra", node:"sympy/sympy", source:"Python",
    repo:"https://github.com/malcolmston/algebra", docs:"https://malcolmston.github.io/algebra/",
    tagline:"Symbolic mathematics — a tiny computer-algebra system.",
    blurb:"A standard-library-only computer-algebra system, a spiritual port of a subset of Python's SymPy. Expressions "+
      "are immutable trees over symbols, big.Int / big.Rat numbers, named constants and elementary functions — build, "+
      "simplify, differentiate, integrate, expand, factor and solve, then print them back as readable infix.",
    tags:["symbolic math","differentiate","integrate","solve","simplify","math/big"],
    features:[
      "Immutable expression trees implementing an <code>Expr</code> interface",
      "Exact arithmetic via <code>math/big</code> integers &amp; rationals",
      "Calculus: <code>Diff</code> and <code>Integrate</code>",
      "<code>Simplify</code>, <code>Expand</code>, <code>Factor</code>, <code>Collect</code>, <code>Subs</code>",
      "Equation solving: <code>Solve</code> (linear &amp; quadratic)",
      "<code>Parse</code> / <code>MustParse</code> infix input, canonical printing"
    ],
    node_code:
`import sympy as sp

x = sp.symbols('x')
f = x**2 + 2*x + 1

print(sp.diff(f, x))    # 2*x + 2`,
    go_code:
`x := algebra.Sym("x")
f := algebra.MustParse("x^2 + 2*x + 1")

df := algebra.Diff(f, x)
fmt.Println(algebra.Simplify(df))   // 2*x + 2`,
    integrate:
`<span class="tok-c">// Solve, expand and integrate</span>
x := algebra.Sym("x")

roots, _ := algebra.Solve(algebra.MustParse("x^2 - 5*x + 6"), x)
fmt.Println(roots[0], roots[1])                              <span class="tok-c">// 2 3</span>

fmt.Println(algebra.Expand(algebra.MustParse("(x + 1)^2")))  <span class="tok-c">// x^2 + 2*x + 1</span>
fmt.Println(algebra.Integrate(algebra.MustParse("2*x"), x))  <span class="tok-c">// x^2</span>`
  },
  {
    id:"opencv", name:"opencv", icon:'<i class="fa-solid fa-eye"></i>', accent:"#56d364",
    pkg:"github.com/malcolmston/opencv", node:"opencv/opencv", source:"Python",
    repo:"https://github.com/malcolmston/opencv", docs:"https://malcolmston.github.io/opencv/",
    tagline:"Image processing &amp; computer vision, zero dependencies.",
    blurb:"A standard-library-only port of a useful subset of Python's OpenCV (cv2). The core type is Mat, a dense "+
      "row-major uint8 matrix; convert to/from image.Image, read and write PNG/JPEG, and run classic pipelines — "+
      "colour conversion, blurring, thresholding, morphology, edges and geometric transforms — with no cgo.",
    tags:["Mat","filters","Canny edges","morphology","PNG / JPEG","no cgo"],
    features:[
      "<code>Mat</code>: dense row-major 8-bit matrix (1- or 3-channel)",
      "I/O: <code>ImRead</code> / <code>ImWrite</code> (PNG &amp; JPEG), <code>FromImage</code> / <code>ToImage</code>",
      "Colour: <code>CvtColor</code> (e.g. <code>ColorRGB2Gray</code>)",
      "Filtering: <code>GaussianBlur</code>, <code>Blur</code>, <code>Threshold</code>, morphology",
      "Edges &amp; more: <code>Canny</code>, <code>Sobel</code>, geometric transforms, drawing",
      "Package name is <code>cv</code> — no cgo, standard library only"
    ],
    node_code:
`import cv2

img = cv2.imread("in.png")
gray = cv2.cvtColor(img, cv2.COLOR_RGB2GRAY)
blur = cv2.GaussianBlur(gray, (5, 5), 1.4)
edges = cv2.Canny(blur, 50, 150)

cv2.imwrite("edges.png", edges)`,
    go_code:
`img, _ := cv.ImRead("in.png")

gray := cv.CvtColor(img, cv.ColorRGB2Gray)
blur := cv.GaussianBlur(gray, 5, 1.4)
edges := cv.Canny(blur, 50, 150)

cv.ImWrite("edges.png", edges)`,
    integrate:
`<span class="tok-c">// Threshold, then clean up with a morphological open</span>
gray := cv.CvtColor(img, cv.ColorRGB2Gray)
bw, _ := cv.Threshold(gray, 127, 255, cv.ThreshBinary)

k := cv.GetStructuringElement(cv.MorphRect, 3, 3)
opened := cv.MorphologyEx(bw, k, cv.MorphOpen, 1)

cv.ImWrite("mask.png", opened)`
  },
  {
  id:"oban", name:"Oban", icon:'<i class="fa-solid fa-list-check"></i>', accent:"#a855f7",
  pkg:"github.com/malcolmston/oban", node:"sorentwo/oban",
  repo:"https://github.com/malcolmston/oban", docs:"https://malcolmston.github.io/oban/",
  tagline:"Background job processing for Go, standard library only.",
  blurb:"An Oban/Sidekiq-style background job system for Go built on nothing but the standard library. "+
    "An Oban engine runs named queues at configured concurrency, resolves workers by name from a Registry, "+
    "and executes each job with a per-attempt timeout. Failures retry with exponential backoff and jitter "+
    "until they succeed or exhaust their attempts, at which point they are discarded. The engine also "+
    "de-duplicates unique jobs, schedules periodic work with cron expressions, wraps every attempt in "+
    "middleware/telemetry, and shuts down gracefully by draining in-flight jobs. It is deterministic and "+
    "testable: time flows through an injectable clock, backoff jitter is seedable, and cron scheduling is "+
    "pure. A complete in-memory Store ships in the box, and the Store interface documents exactly what a "+
    "database-backed implementation must guarantee.",
  tags:["named queues","exponential backoff","jitter","retries","cron scheduling","unique jobs","graceful drain","middleware/telemetry","in-memory Store","zero dependencies"],
  features:[
    "<code>Oban</code> engine — polls named queues at configured concurrency, built with <code>New(Config{...})</code> and driven by <code>Start</code> / <code>Stop</code>",
    "Jobs as data — a <code>Job</code> carries queue, JSON <code>Args</code>, attempts, priority, schedule and error history; build one with <code>NewJob</code>",
    "Named workers — implement <code>Worker</code> (<code>Perform(ctx, *Job) error</code>) or register a func with <code>RegisterFunc</code> in a <code>Registry</code>",
    "Retries with backoff — the <code>Backoff</code> interface, with <code>ExponentialBackoff</code> growing the delay and adding seedable jitter up to a cap",
    "Discard on exhaustion — jobs that use up <code>MaxAttempts</code> transition to discarded and fire the <code>ErrorHandler</code>",
    "Cron scheduling — declare <code>Periodic</code> jobs against a parsed 5-field <code>Schedule</code>; <code>Schedule.Next</code> is a pure function",
    "Unique jobs — <code>WithUnique(key, period)</code> de-duplicates by queue + worker + key over a time window",
    "Middleware &amp; telemetry — wrap every attempt with a <code>Middleware</code> chain, with <code>Telemetry</code> installed innermost to time the worker",
    "Pluggable persistence — a complete <code>InMemoryStore</code> ships in the box; implement <code>Store</code> (SELECT ... FOR UPDATE SKIP LOCKED semantics) for a database",
    "Deterministic &amp; testable — the engine owns time through an injectable <code>Clock</code>, so scheduling, backoff and uniqueness test without real sleeps",
    "Graceful shutdown — <code>Stop</code> stops fetching and drains in-flight work, honouring the context deadline",
    "Zero dependencies — pure Go standard library, no cgo, nothing to audit but the toolchain"
  ],
  node_code:
`# Elixir Oban — the inspiration for this library.
defmodule MyApp.EmailWorker do
  use Oban.Worker, queue: :mailers, max_attempts: 5

  @impl Oban.Worker
  def perform(%Oban.Job{args: %{"to" => to}}) do
    MyApp.Mailer.deliver(to)
    :ok
  end
end

%{to: "ada@example.com"}
|> MyApp.EmailWorker.new()
|> Oban.insert()`,
  go_code:
`import "github.com/malcolmston/oban"

engine, _ := oban.New(oban.Config{
    Store:  oban.NewInMemoryStore(),
    Queues: map[string]int{"default": 5, "mailers": 2},
})

// Register a worker by name.
engine.RegisterFunc("email", func(ctx context.Context, job *oban.Job) error {
    var args struct{ To string ` + "`" + `json:"to"` + "`" + ` }
    if err := job.UnmarshalArgs(&args); err != nil {
        return err
    }
    return sendEmail(ctx, args.To)
})

_ = engine.Start(ctx)
job, _ := oban.NewJob("email", map[string]string{"to": "ada@example.com"},
    oban.WithQueue("mailers"), oban.WithMaxAttempts(5))
_, _, _ = engine.Enqueue(ctx, job)`,
  integrate:
`<span class="tok-c">// Retry failures with exponential backoff + jitter, capped at an hour.</span>
engine, _ := oban.New(oban.Config{
    Store:   oban.NewInMemoryStore(),
    Queues:  map[string]int{"default": 5},
    Backoff: oban.NewExponentialBackoff(time.Second, time.Hour, 0.2, 0),
})

<span class="tok-c">// Schedule a periodic job with a 5-field cron expression.</span>
engine, _ = oban.New(oban.Config{
    Store:  oban.NewInMemoryStore(),
    Queues: map[string]int{"default": 5},
    Periodic: []oban.Periodic{{
        Schedule: oban.MustParseCron("0 * * * *"), <span class="tok-c">// top of every hour</span>
        Worker:   "digest",
        Args:     map[string]string{"kind": "hourly"},
    }},
})

<span class="tok-c">// De-duplicate: skip an enqueue if an unfinished job with the same</span>
<span class="tok-c">// queue + worker + key was inserted within the window.</span>
job, _ := oban.NewJob("email", map[string]string{"to": "ada@example.com"},
    oban.WithUnique("ada@example.com", time.Minute),
    oban.WithPriority(0))
_, dup, _ := engine.Enqueue(ctx, job)

<span class="tok-c">// Graceful shutdown: stop fetching and drain in-flight jobs.</span>
shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
_ = engine.Stop(shutdown)`
  },
  {
  id:"gltf", name:"glTF", icon:'<i class="fa-solid fa-cube"></i>', accent:"#e8613c",
  pkg:"github.com/malcolmston/gltf", node:"KhronosGroup/glTF",
  repo:"https://github.com/malcolmston/gltf", docs:"https://malcolmston.github.io/gltf/",
  tagline:"Read, write, validate, and decode glTF 2.0 / GLB 3D assets in Go.",
  blurb:"A dependency-free, standard-library-only Go toolkit for reading, writing, and inspecting glTF 2.0 "+
    "assets in both the JSON (.gltf) and binary (.glb) container formats. The Document type mirrors the full "+
    "glTF 2.0 schema — scenes, nodes, meshes, accessors, buffer views, buffers, materials, textures, "+
    "animations, skins and cameras — as spec-indexed slices, with typed enumerations and vendor Extensions/"+
    "Extras preserved as raw JSON. Buffers resolve from GLB BIN chunks, base64 data URIs or external files; "+
    "typed accessor decoders read vertex data honoring byteOffset, byteStride, normalization and sparse "+
    "substitutions; Validate reports every structural problem with a descriptive path; and a triangle builder "+
    "emits a minimal valid asset. No cgo, no third-party dependencies.",
  tags:["Document model","glTF JSON I/O","GLB binary I/O","accessor decoding","sparse accessors","buffer resolution","validation","stdlib only"],
  features:[
    "<code>Document</code> model — the glTF 2.0 root, with every collection (scenes, nodes, meshes, accessors, buffer views, buffers, materials, textures, animations, skins, cameras) a spec-indexed slice; <code>Extensions</code> and <code>Extras</code> preserved as <code>json.RawMessage</code>",
    "JSON I/O — <code>Decode</code> / <code>Encode</code> plus the <code>Open</code> / <code>Save</code> file helpers for <code>.gltf</code> documents",
    "Binary GLB container — <code>ReadGLB</code> / <code>WriteGLB</code> and <code>OpenGLB</code> / <code>SaveGLB</code>, handling the JSON + BIN chunks, magic/version checks and 4-byte padding",
    "Buffer resolution from GLB BIN chunks, base64 <code>data:</code> URIs and external files via <code>Document.ResolveBuffers</code>, with <code>EncodeDataURI</code> for embedding",
    "Typed accessor decoders — <code>DecodeAccessorVec2</code>/<code>Vec3</code>/<code>Vec4</code>, <code>DecodeAccessorFloat32</code>, <code>DecodeAccessorUint32</code> and <code>DecodeIndices</code>, honoring <code>byteOffset</code>, <code>byteStride</code>, <code>normalized</code> and sparse substitutions",
    "Typed enumerations — <code>ComponentType</code>, <code>AccessorType</code>, <code>PrimitiveMode</code>, <code>Filter</code>, <code>WrapMode</code>, <code>Interpolation</code>, <code>CameraType</code> and <code>AlphaMode</code> carry the spec-defined constant values",
    "Structural validation — <code>Document.Validate</code> checks required fields and index ranges, returning <code>ValidationErrors</code> with paths like <code>meshes[0].primitives[0].attributes.POSITION</code> (<code>AsValidationErrors</code> unwraps them)",
    "Triangle builder — <code>Triangle</code> constructs a minimal valid document plus its BIN buffer, and <code>WriteTriangleGLB</code> / <code>WriteTriangleGLTF</code> emit it directly"
  ],
  node_code:
`// Load a .glb and read the first mesh's vertex positions (three.js GLTFLoader).
import { GLTFLoader } from "three/examples/jsm/loaders/GLTFLoader.js";

new GLTFLoader().load("model.glb", (gltf) => {
  const mesh = gltf.scene.getObjectByProperty("type", "Mesh");
  const positions = mesh.geometry.getAttribute("position");
  console.log(positions.count, "vertices");
});`,
  go_code:
`import "github.com/malcolmston/gltf"

// OpenGLB resolves the BIN chunk and external buffers automatically.
doc, _ := gltf.OpenGLB("model.glb")

prim := doc.Meshes[0].Primitives[0]
positions, _ := doc.DecodeAccessorVec3(prim.Attributes["POSITION"])
indices, _ := doc.DecodeIndices(&prim)
fmt.Println(len(positions), "vertices,", len(indices), "indices")`,
  integrate:
`<span class="tok-c">// Build a minimal triangle document plus its BIN buffer and write a binary GLB.</span>
doc, bin := gltf.Triangle()
if err := gltf.SaveGLB("triangle.glb", doc, bin); err != nil {
    log.Fatal(err)
}

<span class="tok-c">// Read it back; OpenGLB resolves the BIN chunk and any external buffers.</span>
doc, _ = gltf.OpenGLB("triangle.glb")

<span class="tok-c">// Validate structure (required fields + index ranges) before decoding.</span>
if err := doc.Validate(); err != nil {
    if verrs, ok := gltf.AsValidationErrors(err); ok {
        for _, v := range verrs {
            fmt.Printf("%s: %s\\n", v.Path, v.Message)
        }
    }
}

<span class="tok-c">// Decode typed vertex data — byteOffset, byteStride, normalization and sparse are all honored.</span>
prim := doc.Meshes[0].Primitives[0]
positions, _ := doc.DecodeAccessorVec3(prim.Attributes["POSITION"])
indices, _ := doc.DecodeIndices(&prim)

<span class="tok-c">// Or emit a self-contained .gltf with the buffer embedded as a base64 data URI.</span>
f, _ := os.Create("triangle.gltf")
defer f.Close()
gltf.WriteTriangleGLTF(f)`
  },
  {
  id:"sled", name:"sled", icon:'<i class="fa-solid fa-database"></i>', accent:"#e0803c",
  pkg:"github.com/malcolmston/sled", node:"spacejam/sled",
  repo:"https://github.com/malcolmston/sled", docs:"https://malcolmston.github.io/sled/",
  tagline:"An embedded, transactional, crash-safe key/value store in pure Go.",
  blurb:"A small embedded key/value store written in pure Go with no third-party dependencies and no cgo, "+
    "inspired by the Rust sled crate. All state lives in one append-only write-ahead log: every durable "+
    "mutation — a single Set/Delete, a Batch, or a transaction — is encoded as exactly one length-prefixed, "+
    "CRC-32 checksummed record and appended to the log, so groups of writes are applied all-or-nothing on "+
    "recovery. The in-memory index is an immutable, ordered persistent treap published through a single "+
    "atomic.Pointer store, giving lock-free snapshot reads that never race the writer. On Open the log is "+
    "replayed and stops at the first torn or corrupt record, so a crash mid-write loses only the in-flight "+
    "record and the partial tail is physically truncated. On top sit atomic Batch commits, serializable "+
    "Update/View transactions, ordered prefix and bounded Scan, and a rename-atomic Compact.",
  tags:["append-only WAL","CRC-32 records","persistent treap","atomic.Pointer","crash recovery","transactions","atomic batches","ordered scan","compaction","lock-free reads","fsync durability","zero deps"],
  features:[
    "Durable append-only WAL — every commit is one length-prefixed, CRC-32 checksummed record appended via <code>DB.Set</code> / <code>DB.Delete</code>",
    "Real crash recovery — <code>Open</code> replays the log, stops at the first torn or CRC-mismatched record, and truncates the partial tail",
    "Serializable transactions — <code>DB.Update</code> commits atomically or rolls back on error/panic; <code>DB.View</code> reads a stable snapshot",
    "Atomic batches — stage many writes with <code>DB.Batch</code> / <code>NewBatch</code> and land them as one all-or-nothing durable record",
    "Ordered range scans — <code>DB.Scan</code> over a <code>Range</code> (<code>Lower</code>/<code>Upper</code>/<code>Prefix</code>) yields keys in ascending order via <code>Iterator</code>",
    "Immutable persistent-treap index published with <code>atomic.Pointer</code>, so <code>DB.Get</code> / <code>DB.Has</code> take snapshots with no locks",
    "Lock-free concurrent readers — a single writer is serialized while any number of readers proceed race-free (verified under <code>-race</code>)",
    "Compaction — <code>DB.Compact</code> rewrites the log to the live key set and installs it atomically with a rename",
    "Tunable durability — fsync-per-commit by default, or <code>WithSyncWrites(false)</code> / <code>WithFileMode</code> at <code>Open</code>",
    "Zero dependencies — pure Go standard library, no cgo, nothing to audit but the toolchain"
  ],
  node_code:
`// The original sled is a Rust embedded database crate.
use sled::Db;

let db: Db = sled::open("data.sled")?;
db.insert(b"greeting", b"hello")?;

if let Some(v) = db.get(b"greeting")? {
    println!("{}", std::str::from_utf8(&v).unwrap());
}

db.remove(b"greeting")?;
db.flush()?;`,
  go_code:
`import "github.com/malcolmston/sled"

db, _ := sled.Open("data.sled")
defer db.Close()

db.Set([]byte("greeting"), []byte("hello"))

v, ok, _ := db.Get([]byte("greeting"))
if ok {
    fmt.Printf("%s\\n", v) // hello
}

db.Delete([]byte("greeting"))`,
  integrate:
`<span class="tok-c">// Atomic read/write transaction: both writes commit together, or</span>
<span class="tok-c">// neither does. Returning an error (or panicking) rolls it all back.</span>
err := db.Update(func(tx *sled.Tx) error {
	if err := tx.Set([]byte("a"), []byte("1")); err != nil {
		return err
	}
	return tx.Set([]byte("b"), []byte("2"))
})

<span class="tok-c">// Group many writes into one all-or-nothing durable record.</span>
_ = db.Batch(func(b *sled.Batch) error {
	b.Set([]byte("user:1"), []byte("ada"))
	b.Set([]byte("user:2"), []byte("alan"))
	b.Delete([]byte("user:0"))
	return nil
})

<span class="tok-c">// Ordered prefix scan over an immutable snapshot — never races a writer.</span>
it := db.Scan(sled.Range{Prefix: []byte("user:")})
for it.Valid() {
	fmt.Printf("%s = %s\\n", it.Key(), it.Value())
	it.Next()
}

<span class="tok-c">// Half-open bounded range [b, e), read inside a snapshot transaction.</span>
_ = db.View(func(tx *sled.Tx) error {
	r := tx.Scan(sled.Range{Lower: []byte("b"), Upper: []byte("e")})
	for r.Valid() {
		r.Next()
	}
	return nil
})

<span class="tok-c">// Reclaim space: rewrite the log to just the live keys, installed atomically.</span>
if err := db.Compact(); err != nil {
	log.Fatal(err)
}`
  },
  {
  id:"migrate", name:"Migrate", icon:'<i class="fa-solid fa-database"></i>', accent:"#8b5cf6",
  pkg:"github.com/malcolmston/migrate", node:"rails/rails",
  repo:"https://github.com/malcolmston/migrate", docs:"https://malcolmston.github.io/migrate/",
  tagline:"ActiveRecord-style schema migrations for Go.",
  blurb:"A small, dependency-free, ActiveRecord-flavoured schema-migration toolkit for Go built entirely on top of "+
    "the standard library's database/sql package — no third-party packages, no cgo. A Migration carries a uint64 "+
    "Version, a Name, and a pair of directions expressed either as a Go func(ctx, *sql.Tx) error or as raw SQL "+
    "text; a Migrator wraps a *sql.DB, maintains a schema_migrations bookkeeping table, and drives migrations "+
    "forward and backward with Migrate, Up, Down, Rollback, MigrateTo, Redo and Status. Every migration runs in "+
    "its own transaction, so a failure rolls back cleanly and re-running is idempotent. Migrations register "+
    "programmatically or load from a directory / io/fs.FS of <version>_<name>.up.sql / .down.sql pairs, and a "+
    "tiny schema DSL (CreateTable, AddColumn, AddIndex, typed column helpers, foreign keys) emits predictable, "+
    "greppable ANSI SQL.",
  tags:["database/sql","reversible","schema_migrations","transaction per migration","io/fs loading","schema DSL","zero deps","ANSI SQL"],
  features:[
    "Versioned, reversible migrations — a <code>Migration</code> with a <code>uint64</code> Version, applied ascending and rolled back descending",
    "Go <i>or</i> SQL directions — <code>Up</code>/<code>Down</code> as <code>func(ctx, *sql.Tx) error</code>, or <code>UpSQL</code>/<code>DownSQL</code> raw text",
    "Transaction per migration — a failure rolls back, does not record the version, and halts; re-running <code>Migrate</code> is idempotent",
    "Full command set — <code>Migrate</code>, <code>Up</code>, <code>Down</code>, <code>Rollback</code>, <code>MigrateTo</code>, <code>Redo</code> and <code>Status</code> on the <code>Migrator</code>",
    "File loading — <code>LoadDir</code> and <code>LoadFS</code> read <code>&lt;version&gt;_&lt;name&gt;.up.sql</code> / <code>.down.sql</code> pairs from any <code>io/fs.FS</code>",
    "Schema DSL — <code>CreateTable</code> with typed helpers (<code>String</code>, <code>Text</code>, <code>Integer</code>, <code>Boolean</code>, <code>Timestamps</code>) plus <code>AddColumn</code>, <code>AddIndex</code>, <code>DropTable</code>, <code>RenameColumn</code>",
    "Rails-style references &amp; foreign keys — <code>Table.References</code> with <code>WithForeignKey</code>, <code>ReferenceNotNull</code> and <code>ReferenceTable</code>",
    "Zero dependencies — pure Go standard library over <code>database/sql</code>, no cgo, nothing to audit but the toolchain"
  ],
  node_code:
`class CreateUsers < ActiveRecord::Migration[7.1]
  def change
    create_table :users do |t|
      t.string :email, null: false
      t.timestamps
    end
  end
end

# bin/rails db:migrate`,
  go_code:
`import "github.com/malcolmston/migrate"

mg := migrate.New(db) // any database/sql *sql.DB
mg.Register(migrate.Migration{
    Version: 20240101,
    Name:    "create_users",
    UpSQL: migrate.CreateTable("users", func(t *migrate.Table) {
        t.String("email", migrate.NotNull(), migrate.Unique())
        t.Timestamps()
    }),
    DownSQL: migrate.DropTable("users"),
})
if err := mg.Migrate(context.Background()); err != nil {
    log.Fatal(err)
}`,
  integrate:
`<span class="tok-c">// Load .up.sql/.down.sql pairs from a directory and register them.</span>
migs, err := migrate.LoadDir("migrations")
if err != nil {
    log.Fatal(err)
}
mg := migrate.New(db, migrate.WithTable("schema_migrations"))
mg.Register(migs...)

<span class="tok-c">// Apply everything pending, ascending — each in its own transaction.</span>
if err := mg.Migrate(ctx); err != nil {
    log.Fatal(err)
}

<span class="tok-c">// Report which versions are applied vs pending.</span>
statuses, _ := mg.Status(ctx)
for _, s := range statuses {
    log.Printf("%d %s applied=%v", s.Version, s.Name, s.Applied)
}

<span class="tok-c">// Undo the most recent migration, then move to an exact version.</span>
if err := mg.Rollback(ctx, 1); err != nil {
    log.Fatal(err)
}
if err := mg.MigrateTo(ctx, 20240101); err != nil {
    log.Fatal(err)
}`
  },
  {
  id:"lucene", name:"Lucene", icon:'<i class="fa-solid fa-magnifying-glass"></i>', accent:"#3fb6a8",
  pkg:"github.com/malcolmston/lucene", node:"apache/lucene",
  repo:"https://github.com/malcolmston/lucene", docs:"https://malcolmston.github.io/lucene/",
  tagline:"Embedded full-text search for Go, in the style of Apache Lucene.",
  blurb:"A small, dependency-free, in-memory full-text search engine written in pure Go, modelled on Apache "+
    "Lucene. A configurable analysis pipeline tokenizes, lowercases, drops stop words and stems your text; "+
    "an inverted index records term frequencies and positions for every field; and a rich query model — with "+
    "a query-string parser — is ranked by BM25 into deterministic top-N hits. Everything is built on the Go "+
    "standard library alone: no cgo, no third-party modules, nothing to audit but the toolchain.",
  tags:["analyzer pipeline","inverted index","BM25 ranking","phrase queries","query parser","prefix & range","highlighter","stdlib-only"],
  features:[
    "Analysis pipeline — <code>NewStandardAnalyzer</code> tokenizes, lowercases, drops stop words and stems, tunable via <code>WithStopWords</code> and <code>WithStemming</code>",
    "Inverted index — <code>NewIndex</code> with <code>Add</code> / <code>Delete</code> over <code>Document</code> values; postings carry term frequencies and positions, and it is safe for concurrent use",
    "Query model — <code>TermQuery</code>, <code>PhraseQuery</code>, <code>BooleanQuery</code> (<code>Must</code>/<code>Should</code>/<code>MustNot</code>), <code>PrefixQuery</code>, <code>RangeQuery</code> and <code>MatchAllQuery</code>",
    "Query-string parser — <code>NewParser</code> and <code>Parse</code> turn <code>title:go &quot;phrase&quot; +must -not net* [a TO z]</code> into a query tree",
    "BM25 relevance — <code>Search</code> returns a <code>Result</code> of top-ranked <code>Hit</code> values, ties broken by document ID for fully deterministic output",
    "One-call search — <code>SearchString</code> parses and executes a query string against the index's analyzer in a single step",
    "Highlighting — <code>NewHighlighter</code> and <code>Highlight</code> wrap matched (and stemmed) terms in custom markers while preserving the original text",
    "Zero dependencies — pure Go standard library, no cgo, no third-party modules"
  ],
  node_code:
`import org.apache.lucene.analysis.standard.StandardAnalyzer;
import org.apache.lucene.document.*;
import org.apache.lucene.index.*;
import org.apache.lucene.queryparser.classic.QueryParser;
import org.apache.lucene.search.*;
import org.apache.lucene.store.ByteBuffersDirectory;

Directory dir = new ByteBuffersDirectory();
IndexWriter w = new IndexWriter(dir, new IndexWriterConfig(new StandardAnalyzer()));

Document doc = new Document();
doc.add(new TextField("title", "The Go Programming Language", Field.Store.YES));
doc.add(new TextField("body", "Go is an open source programming language.", Field.Store.YES));
w.addDocument(doc);
w.close();

IndexSearcher searcher = new IndexSearcher(DirectoryReader.open(dir));
Query q = new QueryParser("body", new StandardAnalyzer()).parse("programming +go -rust");
TopDocs hits = searcher.search(q, 10);
System.out.println("matches: " + hits.totalHits.value);`,
  go_code:
`import "github.com/malcolmston/lucene"

idx := lucene.NewIndex(lucene.NewStandardAnalyzer())
_ = idx.Add(lucene.Document{ID: "1", Fields: map[string]string{
	"title": "The Go Programming Language",
	"body":  "Go is an open source programming language.",
}})

// Parse a query string and take the top 10 hits by BM25 score.
res, _ := idx.SearchString("body:programming +body:go -rust", 10)
fmt.Println("matches:", res.Total)
for _, hit := range res.Hits {
	fmt.Printf("  %s  %.3f\n", hit.ID, hit.Score)
}`,
  integrate:
`<span class="tok-c">// Build a boolean query by hand — the programmatic form of a</span>
<span class="tok-c">// "+must -not" query string, with clause-level Occur semantics.</span>
bq := lucene.NewBooleanQuery().
	Add(lucene.NewPhraseQuery("title", "programming", "language"), lucene.Must).
	Add(lucene.NewTermQuery("body", "google"), lucene.Should).
	Add(lucene.NewTermQuery("body", "rust"), lucene.MustNot)

res := idx.Search(bq, 10)
for _, hit := range res.Hits {
	fmt.Printf("%s  score=%.3f\n", hit.ID, hit.Score)
}

<span class="tok-c">// Prefix and range queries reach whole families of terms at once.</span>
res = idx.Search(lucene.NewPrefixQuery("body", "net"), 10)
res = idx.Search(lucene.NewRangeQuery("year", "2000", "2020", true, true), 10)

<span class="tok-c">// Highlight matched (and stemmed) words in a snippet with custom markers.</span>
h := lucene.NewHighlighter(idx.Analyzer(), "[", "]")
fmt.Println(h.Highlight("Go is a great programming language.",
	lucene.NewTermQuery("body", "programming")))`
  },
  {
  id:"quartz", name:"quartz", icon:'<i class="fa-solid fa-clock"></i>', accent:"#8b6df0",
  pkg:"github.com/malcolmston/quartz", node:"quartz-scheduler/quartz",
  repo:"https://github.com/malcolmston/quartz", docs:"https://malcolmston.github.io/quartz/",
  tagline:"A Quartz-style job scheduler for Go, stdlib only.",
  blurb:"A from-scratch, standard-library-only Go take on the classic Quartz job scheduler — no cgo, no "+
    "third-party dependencies. You get Jobs (any Execute(context.Context) error, plus a JobFunc adapter "+
    "and a JobDataMap for per-job state), two Trigger implementations, and a Scheduler that fires due "+
    "triggers on a bounded worker pool. SimpleTrigger covers start-time-plus-interval-plus-repeat "+
    "schedules; CronTrigger runs on a real cron parser supporting five- or six-field expressions with "+
    "ranges, steps, lists, names and the * / ? wildcards, evaluated in any *time.Location. The scheduler "+
    "adds graceful shutdown with drain, per-trigger and per-job pause/resume, configurable misfire "+
    "policies, and job/trigger listeners that can veto a fire. Storage sits behind a JobStore interface "+
    "with an in-memory implementation, and every fire-time decision flows through an injectable clock so "+
    "schedules are fully deterministic and testable without sleeping.",
  tags:["Job","JobDetail","SimpleTrigger","CronTrigger","ParseCron","Scheduler","worker pool","misfire","listeners","JobStore","injectable clock","stdlib-only"],
  features:[
    "Jobs — any <code>Job</code> (<code>Execute(context.Context) error</code>), a <code>JobFunc</code> adapter, and a <code>JobDataMap</code> of per-job state carried through each fire",
    "<code>JobDetail</code> — names a job via a <code>Key</code>, with <code>WithDescription</code>, <code>WithData</code> and <code>Durable</code> builder options",
    "<code>SimpleTrigger</code> — start time plus a fixed <code>Interval</code> and a repeat count (or <code>RepeatForever</code>), with an optional end time",
    "<code>CronTrigger</code> over a real parser — <code>ParseCron</code>/<code>CronExpression</code> handle 5/6 fields, ranges (<code>1-5</code>), steps (<code>*/15</code>), lists, names and <code>?</code>, evaluated in any <code>*time.Location</code> via <code>In</code>",
    "<code>Scheduler</code> — <code>NewScheduler</code> with a bounded <code>Options.Concurrency</code> worker pool, <code>ScheduleJob</code>, <code>Start</code>, and <code>Shutdown</code> with optional drain",
    "Pause / resume — <code>PauseTrigger</code>/<code>ResumeTrigger</code> and <code>PauseJob</code>/<code>ResumeJob</code>, plus out-of-band <code>TriggerJob</code>",
    "Misfire policies — <code>MisfireSmart</code>, <code>MisfireFireNow</code>, <code>MisfireIgnore</code> and <code>MisfireDoNothing</code> via <code>WithMisfirePolicy</code>",
    "Listeners — <code>JobListener</code> and <code>TriggerListener</code> hooks (embed <code>BaseJobListener</code>/<code>BaseTriggerListener</code>); a trigger listener can <code>VetoJobExecution</code>",
    "Pluggable storage — a <code>JobStore</code> interface with an in-memory <code>MemoryJobStore</code> (<code>NewMemoryJobStore</code>)",
    "Injectable clock — <code>Options.Clock</code> plus <code>ProcessDue</code> drive cron and next-fire logic deterministically in tests, without sleeping",
    "Zero dependencies — pure Go standard library, nothing to audit but the toolchain"
  ],
  node_code:
`// Java — quartz-scheduler/quartz
JobDetail job = JobBuilder.newJob(ReportJob.class)
    .withIdentity("daily-report")
    .build();

Trigger trigger = TriggerBuilder.newTrigger()
    .withIdentity("report-trigger")
    .withSchedule(CronScheduleBuilder.cronSchedule("0 0 10 * * ?"))
    .build();

Scheduler scheduler = StdSchedulerFactory.getDefaultScheduler();
scheduler.start();
scheduler.scheduleJob(job, trigger);`,
  go_code:
`import "github.com/malcolmston/quartz"

// A pool of 4 workers running in real time.
s := quartz.NewScheduler(quartz.Options{Concurrency: 4})

job := quartz.NewJobDetail(quartz.NewKey("daily-report"),
    quartz.JobFunc(func(ctx context.Context) error {
        fmt.Println("generating report")
        return nil
    }))

// Fire every day at 10:00:00.
trigger, _ := quartz.NewCronTriggerWithKeys(
    quartz.NewKey("report-trigger"), job.Key(), "0 0 10 * * *")

s.ScheduleJob(job, trigger)
s.Start()
defer s.Shutdown(true) // graceful drain`,
  integrate:
`<span class="tok-c">// A repeating interval trigger: fire every 30s, forever.</span>
job := quartz.NewJobDetail(quartz.NewKey("heartbeat"),
    quartz.JobFunc(func(ctx context.Context) error { return nil }))
beat := quartz.NewSimpleTrigger(quartz.NewKey("beat"), job.Key(),
    time.Now(), 30*time.Second, quartz.RepeatForever)
_ = s.ScheduleJob(job, beat)

<span class="tok-c">// A cron trigger in a chosen time zone, skipping missed fires.</span>
ny, _ := time.LoadLocation("America/New_York")
nightly, _ := quartz.NewCronTriggerWithKeys(
    quartz.NewKey("nightly"), job.Key(), "0 0 2 * * *")
nightly.In(ny).WithMisfirePolicy(quartz.MisfireDoNothing)

<span class="tok-c">// Pause and later resume a single trigger; misfires are reconciled.</span>
_ = s.PauseTrigger(beat.Key())
_ = s.ResumeTrigger(beat.Key())

<span class="tok-c">// Deterministic testing: inject a clock and call ProcessDue directly.</span>
now := time.Date(2026, 1, 1, 9, 59, 0, 0, time.UTC)
t := quartz.NewScheduler(quartz.Options{Clock: func() time.Time { return now }})
_ = t.ProcessDue()                                     // nothing due at 09:59
now = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)     // advance the clock
_ = t.ProcessDue()                                     // fires now`
  },
  {
  id:"sqlite", name:"SQLite", icon:'<i class="fa-solid fa-database"></i>', accent:"#f5a623",
  pkg:"github.com/malcolmston/sqlite", node:"sqlite/sqlite",
  repo:"https://github.com/malcolmston/sqlite", docs:"https://malcolmston.github.io/sqlite/",
  tagline:"A pure-Go embedded SQL engine with a database/sql driver.",
  blurb:"A small, dependency-free SQL database engine written in pure Go (standard library only, no cgo). It ships a "+
    "SQL tokenizer, a recursive-descent parser and a tree-walking executor over an in-memory, row-oriented store, "+
    "and plugs into the standard database/sql package through a registered driver named \"mstsqlite\". The engine "+
    "implements a genuinely useful subset of SQL — CREATE TABLE / INSERT / SELECT (WHERE, GROUP BY, HAVING, "+
    "ORDER BY, LIMIT, a two-table INNER JOIN and aggregates), UPDATE, DELETE and transactions — with SQLite-style "+
    "dynamic typing and three-valued NULL logic. Import path github.com/malcolmston/sqlite; package sqlite.",
  tags:["database/sql driver","pure Go","no cgo","in-memory","SQL parser","tree-walking executor","transactions","dynamic typing"],
  features:[
    "Registers the <code>&quot;mstsqlite&quot;</code> <code>database/sql</code> driver (see <code>DriverName</code>) — open with <code>sql.Open(&quot;mstsqlite&quot;, &quot;:memory:&quot;)</code>",
    "Full DDL/DML subset — <code>CREATE TABLE</code> (typed columns, <code>PRIMARY KEY</code>/<code>NOT NULL</code>, <code>IF NOT EXISTS</code>), <code>INSERT</code>, <code>UPDATE</code>, <code>DELETE</code>, <code>DROP TABLE</code>",
    "Rich <code>SELECT</code> — <code>WHERE</code>, <code>GROUP BY</code>, <code>HAVING</code>, <code>ORDER BY</code>, <code>LIMIT</code>/<code>OFFSET</code>, <code>DISTINCT</code>, <code>AS</code> aliases and a two-table <code>INNER JOIN</code>",
    "Aggregates — <code>COUNT</code> (incl. <code>COUNT(*)</code> / <code>COUNT(DISTINCT x)</code>), <code>SUM</code>, <code>AVG</code>, <code>MIN</code>, <code>MAX</code>",
    "Expressions — comparisons, <code>AND</code>/<code>OR</code>/<code>NOT</code>, <code>IN</code>, <code>LIKE</code> (<code>%</code>/<code>_</code>), <code>IS NULL</code>, arithmetic and <code>||</code> concatenation",
    "Transactions via <code>BEGIN</code>/<code>COMMIT</code>/<code>ROLLBACK</code> — snapshot rollback, serializable isolation (one writer at a time)",
    "SQLite-style dynamic typing — the <code>Value</code>/<code>ValueType</code> storage classes (NULL, INTEGER, REAL, TEXT, BLOB) with three-valued NULL logic",
    "Direct, non-<code>database/sql</code> API — <code>NewDatabase</code>, <code>Database.Exec</code>, <code>Database.Query</code> (returning <code>ResultSet</code>/<code>ExecResult</code>) and <code>Parse</code> for AST inspection"
  ],
  node_code:
`#include <sqlite3.h>
#include <stdio.h>

int main(void) {
    sqlite3 *db;
    sqlite3_open(":memory:", &db);
    sqlite3_exec(db, "CREATE TABLE fruit (name TEXT, qty INTEGER)", 0, 0, 0);
    sqlite3_exec(db,
        "INSERT INTO fruit VALUES ('apple', 3), ('banana', 7)", 0, 0, 0);

    sqlite3_stmt *st;
    sqlite3_prepare_v2(db,
        "SELECT name, qty FROM fruit WHERE qty >= 5 ORDER BY qty DESC",
        -1, &st, 0);
    while (sqlite3_step(st) == SQLITE_ROW)
        printf("%s: %d\\n", sqlite3_column_text(st, 0),
                            sqlite3_column_int(st, 1));
    sqlite3_finalize(st);
    sqlite3_close(db);
}`,
  go_code:
`import (
    "database/sql"
    "fmt"

    _ "github.com/malcolmston/sqlite" // registers the "mstsqlite" driver
)

db, _ := sql.Open("mstsqlite", ":memory:")
db.SetMaxOpenConns(1) // pin the anonymous in-memory DB to one connection

db.Exec(` + "`CREATE TABLE fruit (name TEXT, qty INTEGER)`" + `)
db.Exec(` + "`INSERT INTO fruit VALUES (?, ?), (?, ?)`" + `, "apple", 3, "banana", 7)

rows, _ := db.Query(` + "`SELECT name, qty FROM fruit WHERE qty >= ? ORDER BY qty DESC`" + `, 5)
for rows.Next() {
    var name string
    var qty int
    rows.Scan(&name, &qty)
    fmt.Printf("%s: %d\\n", name, qty)
}`,
  integrate:
`<span class="tok-c">// Aggregate with GROUP BY / HAVING / ORDER BY over the database/sql handle.</span>
rows, _ := db.Query(` + "`SELECT name, SUM(qty) AS total FROM fruit\n    GROUP BY name HAVING SUM(qty) > ? ORDER BY total DESC`" + `, 2)

<span class="tok-c">// Transactions use snapshot rollback and serializable isolation.</span>
tx, _ := db.Begin()
tx.Exec(` + "`UPDATE fruit SET qty = qty + 1 WHERE name = ?`" + `, "apple")
tx.Commit() <span class="tok-c">// or tx.Rollback() to discard</span>

<span class="tok-c">// A single two-table INNER JOIN ... ON is supported.</span>
db.Query(` + "`SELECT f.name, b.aisle FROM fruit f INNER JOIN bins b ON b.name = f.name`" + `)

<span class="tok-c">// Skip database/sql entirely and drive the engine in-process.</span>
store := sqlite.NewDatabase()
store.Exec(` + "`CREATE TABLE t (a INTEGER, b TEXT)`" + `)
rs, _ := store.Query(` + "`SELECT a, b FROM t WHERE a = ?`" + `, 1)
fmt.Println(rs.Columns, rs.Rows)

<span class="tok-c">// Inspect the parsed AST without executing anything.</span>
stmt, _ := sqlite.Parse(` + "`SELECT * FROM t WHERE a > 10`" + `)
_ = stmt`
  },
  {
  id:"redis", name:"Redis", icon:'<i class="fa-solid fa-database"></i>', accent:"#dc382d",
  pkg:"github.com/malcolmston/redis", node:"redis/redis",
  repo:"https://github.com/malcolmston/redis", docs:"https://malcolmston.github.io/redis/",
  tagline:"An embeddable, Redis-style in-memory data store in pure Go.",
  blurb:"A thread-safe, Redis-style keyspace built entirely on the Go standard library — no cgo and no third-party "+
    "dependencies. A single-mutex Store holds the core Redis data types (strings, lists, hashes, sets and "+
    "skiplist-backed sorted sets), exposed both as typed Go methods and through a dynamic Do(...) dispatcher that "+
    "mirrors sending a RESP command array. Expiration is lazy and driven by an injectable Clock so TTL behaviour is "+
    "fully deterministic in tests, and a RESP2 codec plus an optional TCP Server let real Redis clients speak to the "+
    "same store over the wire.",
  tags:["Store","strings","lists","hashes","sets","sorted sets","skiplist","lazy expiry","RESP2 codec","Do dispatcher","TCP server","stdlib-only"],
  features:[
    "<code>Store</code> — a thread-safe, single-mutex keyspace built with <code>New</code> or <code>NewWithClock</code>, plus generic <code>Del</code>, <code>Exists</code>, <code>Keys</code> (glob), <code>TypeOf</code>, <code>DBSize</code> and <code>FlushAll</code>",
    "Strings — <code>Set</code> (with EX/PX/NX/XX <code>SetOptions</code>), <code>Get</code>, <code>GetSet</code>, <code>Append</code>, <code>Strlen</code> and <code>Incr</code>/<code>Decr</code>/<code>IncrBy</code>/<code>DecrBy</code>",
    "Lists — <code>LPush</code>, <code>RPush</code>, <code>LPop</code>, <code>RPop</code>, <code>LRange</code>, <code>LLen</code> and <code>LIndex</code>",
    "Hashes — <code>HSet</code>, <code>HGet</code>, <code>HDel</code>, <code>HGetAll</code>, <code>HKeys</code>, <code>HVals</code>, <code>HLen</code> and <code>HExists</code>",
    "Sets — <code>SAdd</code>, <code>SRem</code>, <code>SMembers</code>, <code>SIsMember</code>, <code>SCard</code> plus <code>SInter</code>, <code>SUnion</code> and <code>SDiff</code>",
    "Sorted sets — <code>ZAdd</code>, <code>ZScore</code>, <code>ZRange</code>, <code>ZRevRange</code>, <code>ZRangeByScore</code>, <code>ZRank</code> and <code>ZRevRank</code>, backed by a <code>skiplist</code> ordered by (score, member) for O(log n) rank queries",
    "Lazy, deterministic expiry — <code>Expire</code>/<code>PExpire</code>/<code>TTL</code>/<code>PTTL</code>/<code>Persist</code> with an injectable <code>Clock</code> and a testable <code>ManualClock</code>",
    "Dynamic dispatch — <code>Store.Do</code> runs a command by name and returns RESP-friendly Go values (<code>SimpleString</code>, <code>int64</code>, <code>string</code>, <code>nil</code> or <code>[]any</code>)",
    "RESP2 codec — <code>Encoder</code> and <code>Decoder</code> (<code>NewEncoder</code>/<code>NewDecoder</code>) implement the RESP wire format, and an optional <code>Server</code> (<code>NewServer</code>/<code>ListenAndServe</code>) speaks it to real Redis clients over TCP",
    "Zero dependencies — pure Go standard library, no cgo, nothing to audit but the toolchain"
  ],
  node_code:
`# redis-cli speaking RESP to a real Redis server
SET name alice
GET name             # "alice"

RPUSH tasks a b c
LRANGE tasks 0 -1    # 1) "a" 2) "b" 3) "c"

ZADD board 25 bob
ZREVRANK board bob   # (integer) 0`,
  go_code:
`import "github.com/malcolmston/redis"

s := redis.New()

s.Set("name", "alice", redis.SetOptions{})
name, _, _ := s.Get("name")           // "alice"

s.RPush("tasks", "a", "b", "c")
items, _ := s.LRange("tasks", 0, -1)  // [a b c]

s.ZAdd("board", redis.ZMember{Member: "bob", Score: 25})
rank, _, _ := s.ZRevRank("board", "bob") // 0`,
  integrate:
`<span class="tok-c">// Every command is also a dynamic Do(...) call that mirrors sending a</span>
<span class="tok-c">// RESP array and returns RESP-friendly Go values.</span>
s := redis.New()
s.Do("SET", "n", "1")
n, _ := s.Do("INCR", "n")        <span class="tok-c">// int64(2)</span>
t, _ := s.Do("TYPE", "n")        <span class="tok-c">// redis.SimpleString("string")</span>

<span class="tok-c">// Inject a ManualClock so lazy, on-access expiry is deterministic.</span>
clk := redis.NewManualClock(time.Unix(0, 0))
s = redis.NewWithClock(clk)
s.Set("k", "v", redis.SetOptions{EX: 10 * time.Second})
clk.Advance(11 * time.Second)
_, ok, _ := s.Get("k")           <span class="tok-c">// ok == false: expired on access</span>

<span class="tok-c">// Serve RESP2 over TCP so a real redis-cli can connect to the store.</span>
srv := redis.NewServer(s)
go srv.ListenAndServe(":6379")
defer srv.Close()`
  },
  {
  id:"prisma", name:"Prisma", icon:'<i class="fa-solid fa-database"></i>', accent:"#5a67d8",
  pkg:"github.com/malcolmston/prisma", node:"prisma/prisma",
  repo:"https://github.com/malcolmston/prisma", docs:"https://malcolmston.github.io/prisma/",
  tagline:"Type-safe, stdlib-only ORM and query builder for Go.",
  blurb:"A small, type-safe query builder and lightweight ORM over the standard database/sql package, "+
    "inspired by the ergonomics of Prisma — with no dependencies beyond the Go standard library and no cgo. "+
    "Models are plain Go structs annotated with prisma:\"...\" struct tags and compiled into a Model by "+
    "reflection; a generic, chainable Query[T] builder emits parameterized SQL where literal values are always "+
    "bound as arguments and never concatenated into the statement. Every terminal operation has a matching "+
    "*SQL twin that returns the exact (sql, args) without touching the database, and a pluggable Dialect "+
    "switches placeholder style between ? (MySQL/SQLite) and $1, $2, … (PostgreSQL). The import path is "+
    "github.com/malcolmston/prisma and the package is named prisma.",
  tags:["type-safe","query builder","ORM","database/sql","generic Query[T]","parameterized SQL","struct tags","pluggable dialects"],
  features:[
    "Plain-struct models — annotate fields with <code>prisma:\"col=…,pk,auto\"</code> tags; reflection compiles them into a <code>Model</code> held by a <code>Registry</code>",
    "Generic chainable builder — <code>NewQuery[T]</code> with <code>Where</code>, <code>OrderBy</code>, <code>Take</code>, <code>Skip</code>, <code>Select</code> and <code>Include</code>",
    "Typed operators — <code>Equals</code>, <code>Not</code>, <code>In</code>, <code>NotIn</code>, <code>Lt</code>/<code>Lte</code>/<code>Gt</code>/<code>Gte</code>, <code>Contains</code>, <code>StartsWith</code>, <code>EndsWith</code>, combined with <code>And</code>, <code>Or</code> and <code>NotGroup</code>",
    "Always parameterized — literal values are bound as arguments, never concatenated into the SQL text",
    "<code>*SQL</code> twins — <code>FindManySQL</code>, <code>CountSQL</code>, <code>CreateSQL</code>, <code>UpdateSQL</code> and <code>DeleteSQL</code> return <code>(sql, args)</code> without hitting the database",
    "Terminal operations — <code>FindMany</code>, <code>FindFirst</code>, <code>FindUnique</code>, <code>Count</code>, <code>Create</code>, <code>CreateMany</code>, <code>Update</code> and <code>Delete</code>",
    "Eager loading — <code>Include</code> pulls a to-one relation via a LEFT JOIN and scans it into the struct",
    "Pluggable dialects — <code>Question</code> (<code>?</code>) and <code>Dollar</code> (<code>$1, $2, …</code>) selected with <code>WithDialect</code>",
    "Zero dependencies — pure Go standard library over <code>database/sql</code>, no cgo, nothing to audit but the toolchain"
  ],
  node_code:
`import { PrismaClient } from '@prisma/client'

const prisma = new PrismaClient()

const users = await prisma.user.findMany({
  where: { email: { startsWith: 'ada' }, id: { gt: 0 } },
  orderBy: { name: 'asc' },
  take: 10,
})

await prisma.user.create({
  data: { name: 'Ada', email: 'ada@example.com' },
})`,
  go_code:
`import "github.com/malcolmston/prisma"

client := prisma.NewClient(db) // any *sql.DB driver
client.Register(User{})

users, _ := prisma.NewQuery[User](client).
	Where(prisma.StartsWith("email", "ada"), prisma.Gt("id", 0)).
	OrderBy("name", prisma.Asc).
	Take(10).
	FindMany(ctx)

prisma.NewQuery[User](client).
	Create(ctx, User{Name: "Ada", Email: "ada@example.com"})

n, _ := prisma.NewQuery[User](client).Count(ctx)`,
  integrate:
`<span class="tok-c">// Every terminal op has a *SQL twin that returns the exact statement</span>
<span class="tok-c">// and argument slice without ever touching the database.</span>
sqlStr, args, _ := prisma.NewQuery[User](client).
	Where(prisma.Equals("email", "a@b.com"), prisma.Gt("id", 10)).
	OrderBy("name", prisma.Asc).
	Take(5).
	FindManySQL()
<span class="tok-c">// SELECT id, name, email FROM users WHERE email = ? AND id > ? ORDER BY name ASC LIMIT ?</span>

<span class="tok-c">// Eager-load a to-one relation via LEFT JOIN with Include.</span>
posts, _ := prisma.NewQuery[Post](client).
	Include("Author").
	Where(prisma.In("author_id", 1, 2, 3)).
	FindMany(ctx)

<span class="tok-c">// Switch to PostgreSQL $1,$2 placeholders with a pluggable dialect.</span>
pg := prisma.NewClient(db, prisma.WithDialect(prisma.Dollar))
prisma.NewQuery[User](pg).
	Where(prisma.Equals("id", 1)).
	Update(ctx, map[string]any{"name": "Ada Lovelace"})`
  },
  {
  id:"pandas", name:"pandas", icon:'<i class="fa-solid fa-table"></i>', accent:"#e86bb0",
  pkg:"github.com/malcolmston/pandas", node:"pandas-dev/pandas",
  repo:"https://github.com/malcolmston/pandas", docs:"https://malcolmston.github.io/pandas/",
  tagline:"pandas-style DataFrames and data analysis for Go.",
  blurb:"A from-scratch, standard-library-only Go take on pandas: a named, typed one-dimensional "+
    "Series with first-class missing-value (NA) support, and an ordered DataFrame of equal-length "+
    "columns built from column maps, slices of structs, or CSV. On top of those two types sit the "+
    "everyday analysis verbs — column and row selection (Select, Col, ILoc, Loc, FilterFunc), "+
    "transformation (WithColumn, SortBy, FillNA, DropNA, Describe), GroupBy aggregations (Sum, Mean, "+
    "Min, Max, Count, Std) and inner/left Merge. Everything is built on encoding/csv, sort, strconv "+
    "and reflect — no cgo, no third-party modules — and every operation produces a stable, "+
    "reproducible ordering with missing values sorted last.",
  tags:["Series","DataFrame","NA-aware","GroupBy","Merge","CSV I/O","Describe","stdlib-only"],
  features:[
    "<code>Series</code> — a named, typed 1-D column (<code>Float64</code>/<code>Int64</code>/<code>String</code>/<code>Bool</code> + <code>Object</code>) with an index and first-class <code>IsNA</code> missing-value support",
    "<code>DataFrame</code> construction from column maps (<code>FromMap</code>), slices of structs (<code>FromRecords</code>), Series (<code>NewDataFrame</code>) or CSV (<code>ReadCSV</code>/<code>ReadCSVFile</code>)",
    "Selection &amp; indexing — columns via <code>Select</code>/<code>Col</code>/<code>Drop</code>, rows via <code>ILoc</code>/<code>Head</code>/<code>Tail</code>, labels via <code>Loc</code>, masks via <code>Filter</code>/<code>FilterFunc</code>",
    "Transformation — <code>WithColumn</code>, <code>Rename</code>, <code>Apply</code>/<code>Map</code>, <code>SortBy</code> on one or more keys, and NA handling with <code>FillNA</code>/<code>DropNA</code>",
    "<code>GroupBy</code> partitioning with deterministic ordering and the <code>Sum</code>, <code>Mean</code>, <code>Min</code>, <code>Max</code>, <code>Count</code>, <code>Std</code> aggregations (or the general <code>Agg</code>)",
    "<code>Merge</code> — inner and left joins on a shared key (<code>InnerJoin</code>/<code>LeftJoin</code>), with <code>_left</code>/<code>_right</code> suffixes for colliding columns",
    "<code>Describe</code> — count / mean / std / min / max for every numeric column, plus <code>Unique</code> and <code>ValueCounts</code>",
    "Zero dependencies — pure Go standard library (<code>encoding/csv</code>, <code>sort</code>, <code>strconv</code>, <code>reflect</code>), with stable, reproducible ordering throughout"
  ],
  node_code:
`import pandas as pd

df = pd.DataFrame({
    "city":  ["NYC", "LA", "NYC", "LA"],
    "month": ["Jan", "Jan", "Feb", "Feb"],
    "sales": [100.0, 80.0, 120.0, 90.0],
})

hot = df[df["sales"] >= 100]
means = df.groupby("city")["sales"].mean()
print(df.describe())`,
  go_code:
`import "github.com/malcolmston/pandas"

df, _ := pandas.FromMap(map[string][]any{
    "city":  {"NYC", "LA", "NYC", "LA"},
    "month": {"Jan", "Jan", "Feb", "Feb"},
    "sales": {100.0, 80.0, 120.0, 90.0},
}, []string{"city", "month", "sales"})

hot := df.FilterFunc(func(r pandas.Row) bool {
    v, ok := r.Float("sales")
    return ok && v >= 100
})
gb, _ := df.GroupBy("city")
means, _ := gb.Mean("sales")
fmt.Print(hot, means, df.Describe())`,
  integrate:
`<span class="tok-c">// Build a DataFrame from column data; a nil cell becomes NA.</span>
df, _ := pandas.FromMap(map[string][]any{
    "city":  {"NYC", "LA", "NYC", "LA"},
    "units": {10.0, 8.0, 12.0, nil},
    "price": {9.99, 12.50, 9.99, 15.0},
}, []string{"city", "units", "price"})

<span class="tok-c">// Fill the missing unit count, then attach a derived revenue column.</span>
df = df.FillNA("units", 0.0)
rev := pandas.NewSeries("revenue", []any{99.9, 100.0, 119.88, 0.0})
df, _ = df.WithColumn(rev)

<span class="tok-c">// Sort by revenue descending — NA sorts last, ties break deterministically.</span>
df, _ = df.SortBy([]string{"revenue"}, []bool{false})

<span class="tok-c">// Group by city, total the revenue, then summarise every numeric column.</span>
gb, _ := df.GroupBy("city")
totals, _ := gb.Sum("revenue")
fmt.Print(totals)
fmt.Print(df.Describe())

<span class="tok-c">// Inner-join against a lookup table on the shared key column.</span>
joined, _ := df.Merge(regions, "city", pandas.InnerJoin)
fmt.Print(joined)`
  },
  {
  id:"numpy", name:"numpy", icon:'<i class="fa-solid fa-table-cells"></i>', accent:"#4dabcf",
  pkg:"github.com/malcolmston/numpy", node:"numpy/numpy",
  repo:"https://github.com/malcolmston/numpy", docs:"https://malcolmston.github.io/numpy/",
  tagline:"NumPy-style n-dimensional arrays in Go.",
  blurb:"A from-scratch, standard-library-only Go library modeled on the core of Python's NumPy. Everything is "+
    "built on a single dense NDArray of float64, backed by one flat slice with row-major shape and strides — "+
    "no cgo, no third-party dependencies. You get creation helpers, zero-copy views, NumPy-rule broadcasting "+
    "element-wise math, whole-array and per-axis reductions, basic linear algebra and boolean masking. "+
    "Transpose, Slice and Reshape return views that share the parent's buffer, while arithmetic and "+
    "reductions always return fresh contiguous arrays. All shape and dimension validation failures panic "+
    "with a message prefixed by \"numpy:\", keeping the arithmetic API free of error returns so operations "+
    "can be chained.",
  tags:["NDArray","float64","shape/strides","row-major","broadcasting","axis reductions","views","Reshape","MatMul","boolean masking","keepdims","stdlib-only"],
  features:[
    "<code>NDArray</code> core — a dense row-major array of <code>float64</code> with cached <code>Shape</code>, <code>Strides</code>, <code>Ndim</code> and <code>Size</code>",
    "Creation — <code>FromSlice</code>, <code>FromData</code>, <code>FromNested</code>, <code>Zeros</code>, <code>Ones</code>, <code>Full</code>, <code>Arange</code>, <code>Linspace</code>, <code>Eye</code>, <code>Identity</code>",
    "Zero-copy views — <code>Transpose</code>/<code>T</code> permute strides, <code>Slice</code> adjusts offset+shape, <code>Reshape</code> re-views the same buffer",
    "Broadcasting — NumPy's trailing-axis rules via <code>BroadcastTo</code>, with size-1 dimensions expanded through a zero stride (no copy)",
    "Element-wise math — <code>Add</code>, <code>Sub</code>, <code>Mul</code>, <code>Div</code>, <code>Pow</code> plus <code>*Scalar</code> variants, and <code>Neg</code>/<code>Abs</code>/<code>Sqrt</code>/<code>Exp</code>/<code>Log</code>/<code>Sin</code>/<code>Cos</code>",
    "Reductions — whole-array <code>Sum</code>, <code>Mean</code>, <code>Max</code>, <code>Min</code>, <code>Std</code>, <code>Var</code>, <code>Prod</code> and per-axis <code>SumAxis</code>/<code>MeanAxis</code>/<code>MaxAxis</code>/… with optional <code>keepdims</code>",
    "Linear algebra — <code>Dot</code> (1-D dot / 2-D matmul) and <code>MatMul</code> for matrix products",
    "Comparison &amp; masking — <code>Greater</code>, <code>Less</code>, <code>EqualMask</code> (and scalar variants), <code>MaskSelect</code>, <code>Where</code>, <code>Any</code>, <code>All</code>",
    "Indexing &amp; combining — <code>At</code>/<code>Set</code> by multi-index (negatives allowed), plus <code>Concatenate</code> and <code>Stack</code>",
    "Panic-based errors — every shape or dimension failure panics with a <code>numpy:</code> prefix, so the arithmetic API stays return-value clean and chainable",
    "Zero dependencies — pure Go standard library, nothing to audit but the toolchain"
  ],
  node_code:
`import numpy as np

a = np.arange(0, 6).reshape(2, 3)     # [[0 1 2] [3 4 5]]
b = np.array([10, 20, 30])            # broadcast (2,3) + (3,)
print(a + b)                          # [[10 21 32] [13 24 35]]
print(a.sum(axis=0))                  # [3 5 7]
print(a @ a.T)                        # [[ 5 14] [14 50]]
print(a[a > 2])                       # [3 4 5]`,
  go_code:
`import np "github.com/malcolmston/numpy"

a := np.Arange(0, 6, 1).Reshape(2, 3)      // [[0 1 2] [3 4 5]]
b := np.FromSlice([]float64{10, 20, 30})    // broadcast (2,3) + (3,)
fmt.Println(a.Add(b).Data())                // [10 21 32 13 24 35]
fmt.Println(a.SumAxis(0, false).Data())     // [3 5 7]
fmt.Println(a.MatMul(a.T()).Data())         // [5 14 14 50]
mask := a.GreaterScalar(2)
fmt.Println(a.MaskSelect(mask).Data())      // [3 4 5]`,
  integrate:
`<span class="tok-c">// Build a 2x3 grid and take a zero-copy transposed view — no data is</span>
<span class="tok-c">// copied, only the strides are permuted.</span>
a := np.Arange(0, 6, 1).Reshape(2, 3)
at := a.T() <span class="tok-c">// shape (3,2), shares a's buffer</span>

<span class="tok-c">// Broadcasting follows NumPy's trailing-axis rules: (3,1) + (1,2) -&gt; (3,2).</span>
col := np.FromData([]float64{1, 2, 3}, 3, 1)
row := np.FromData([]float64{10, 20}, 1, 2)
grid := col.Add(row)

<span class="tok-c">// Reduce along an axis with keepdims, then centre each column.</span>
means := a.MeanAxis(0, true)   <span class="tok-c">// shape (1,3)</span>
centered := a.Sub(means)       <span class="tok-c">// broadcast back to (2,3)</span>

<span class="tok-c">// Boolean masking: keep only the entries greater than the column mean.</span>
mask := centered.GreaterScalar(0)
kept := centered.MaskSelect(mask)

<span class="tok-c">// Matrix product of a (2x3) with its transpose (3x2) -&gt; (2x2) Gram matrix.</span>
gram := a.MatMul(at)`
  },
  {
  id:"matplotlib", name:"matplotlib", icon:'<i class="fa-solid fa-chart-line"></i>', accent:"#11557c",
  pkg:"github.com/malcolmston/matplotlib", node:"matplotlib/matplotlib",
  repo:"https://github.com/malcolmston/matplotlib", docs:"https://malcolmston.github.io/matplotlib/",
  tagline:"Matplotlib-style plotting and charting in pure Go.",
  blurb:"A small, dependency-free plotting library for Go, inspired by Python's matplotlib. It exposes a "+
    "Figure/Axes drawing model with a pyplot-style convenience API on top, so you can build a chart either "+
    "object-first (via NewFigure/AddAxes) or with package-level calls (Plot, Bar, Title, Save). Six chart "+
    "types cover the common cases — line plots, scatter, vertical and horizontal bars, histograms with "+
    "automatic binning, and pie — each returned as a typed value with fluent per-series styling for color, "+
    "line width, marker and label. Axes handle the rest automatically: data-range to pixel mapping, axis "+
    "lines, numeric ticks and labels, an optional legend and grid, and a tab10 color cycle. Every figure "+
    "renders to two real formats from the standard library alone — PNG (a raster image with a built-in "+
    "bitmap font) and vector SVG — and identical input always produces identical bytes. No cgo, no "+
    "third-party modules; the import path and package are both matplotlib.",
  tags:["Figure/Axes","pyplot API","line plot","scatter","bar / barh","histogram","pie","tab10 cycle","legend & grid","PNG output","SVG output","deterministic","zero deps"],
  features:[
    "<b>Figure + Axes model</b> — a <code>Figure</code> owns size, DPI and the color cycle; <code>Figure.AddAxes</code> / <code>Figure.Subplots</code> add coordinate systems",
    "Line plots via <code>Axes.Plot</code> returning a <code>LinePlot</code> — overlay multiple series with one call each",
    "Scatter, bars and more — <code>Axes.Scatter</code> (<code>ScatterPlot</code>), <code>Axes.Bar</code> / <code>Axes.BarH</code> (<code>BarChart</code>), <code>Axes.Hist</code> (<code>Histogram</code>), <code>Axes.Pie</code> (<code>PieChart</code>)",
    "Fluent per-series styling — <code>SetColor</code>, <code>SetLabel</code>, <code>SetLineWidth</code>, <code>SetMarker</code> and <code>SetSize</code> chain off each plot",
    "Marker styles — <code>MarkerCircle</code>, <code>MarkerSquare</code>, <code>MarkerTriangle</code>, <code>MarkerCross</code>, <code>MarkerPlus</code> (and <code>MarkerNone</code>)",
    "Automatic decorations — <code>SetTitle</code>, <code>SetXLabel</code>/<code>SetYLabel</code>, numeric ticks, <code>Legend</code> and <code>Grid</code>, with <code>SetXLim</code>/<code>SetYLim</code> overrides",
    "tab10 color cycle — new series draw successive colors from <code>DefaultColors</code>; build your own with <code>RGB</code>, <code>RGBA</code> or <code>Hex</code>",
    "pyplot-style convenience API — package-level <code>FigSize</code>, <code>Plot</code>, <code>Bar</code>, <code>Title</code>, <code>Legend</code>, <code>Grid</code>, <code>Save</code> over an implicit current figure (<code>Gcf</code>/<code>Gca</code>/<code>Clf</code>)",
    "PNG output — <code>SavePNG</code> / <code>WritePNG</code> / <code>PNGBytes</code> render a raster <code>*image.RGBA</code> with Bresenham lines, filled shapes and a built-in bitmap font",
    "Vector SVG output — <code>SaveSVG</code> / <code>WriteSVG</code> / <code>RenderSVG</code> emit clean <code>&lt;line&gt;</code>/<code>&lt;rect&gt;</code>/<code>&lt;polyline&gt;</code>/<code>&lt;polygon&gt;</code>/<code>&lt;text&gt;</code> markup",
    "Format-by-extension <code>Save</code> plus <code>RenderImage</code> for direct pixel access",
    "Deterministic — identical input always produces identical bytes",
    "Zero dependencies — pure Go standard library (<code>image</code>, <code>image/png</code>, <code>math</code>), no cgo, nothing to audit but the toolchain"
  ],
  node_code:
`import matplotlib.pyplot as plt

fig, ax = plt.subplots()
ax.plot([0, 1, 2, 3], [0, 1, 4, 9], marker="o", label="x^2")
ax.plot([0, 1, 2, 3], [0, 2, 4, 6], label="2x")
ax.set_title("Demo")
ax.set_xlabel("x"); ax.set_ylabel("y")
ax.legend(); ax.grid(True)
fig.savefig("demo.png")`,
  go_code:
`import "github.com/malcolmston/matplotlib"

fig := matplotlib.NewFigure(640, 480)
ax := fig.AddAxes()
ax.Plot([]float64{0, 1, 2, 3}, []float64{0, 1, 4, 9}).
	SetLabel("x^2").SetMarker(matplotlib.MarkerCircle)
ax.Plot([]float64{0, 1, 2, 3}, []float64{0, 2, 4, 6}).SetLabel("2x")
ax.SetTitle("Demo").SetXLabel("x").SetYLabel("y").Legend().Grid(true)
_ = fig.SavePNG("demo.png")
_ = fig.SaveSVG("demo.svg")`,
  integrate:
`<span class="tok-c">// Build a figure explicitly, then style two overlaid series. New series</span>
<span class="tok-c">// pull successive colors from the tab10 cycle unless you override them.</span>
fig := matplotlib.NewFigure(640, 480)
ax := fig.AddAxes()
ax.Plot(xs, ys).SetLabel("signal").SetLineWidth(2).SetMarker(matplotlib.MarkerCircle)
ax.Scatter(xs, noise).SetLabel("samples").SetColor(matplotlib.RGB(0xd6, 0x27, 0x28)).SetSize(6)
ax.SetTitle("Overlay").SetXLabel("t").SetYLabel("value").Legend().Grid(true)

<span class="tok-c">// A categorical bar chart with an explicit hex color.</span>
blue, _ := matplotlib.Hex("#11557c")
bx := matplotlib.NewFigure(500, 320).AddAxes()
bx.Bar([]string{"a", "b", "c"}, []float64{3, 7, 2}).SetColor(blue)

<span class="tok-c">// The pyplot-style API drives an implicit current figure, then Save</span>
<span class="tok-c">// picks PNG or SVG from the file extension.</span>
matplotlib.FigSize(480, 360)
matplotlib.Hist(data, 20)
matplotlib.Title("Distribution")
_ = matplotlib.Save("hist.svg")`
  },
  {
  id:"pdfkit", name:"PDFKit", icon:'<i class="fa-solid fa-file-pdf"></i>', accent:"#e5484d",
  pkg:"github.com/malcolmston/pdfkit", node:"foliojs/pdfkit",
  repo:"https://github.com/malcolmston/pdfkit", docs:"https://malcolmston.github.io/pdfkit/",
  tagline:"PDFKit-style PDF generation in pure Go.",
  blurb:"A from-scratch, standard-library-only Go take on Node's PDFKit: build a document, add pages, draw text, "+
    "vector graphics and images, and serialize valid %PDF-1.7 bytes. Everything sits on a real PDF object model "+
    "— indirect objects, a /Catalog, a /Pages tree, per-page content streams, an xref table, a trailer and %%EOF "+
    "— so the output is a conforming PDF that opens anywhere. It ships built-in Adobe Font Metrics for all 14 "+
    "standard Type1 fonts, so text is measured and word-wrapped accurately without embedding font files, plus "+
    "PNG and JPEG image embedding, vector paths with stroke/fill, standard and custom page sizes, and /Info "+
    "metadata. No cgo, no third-party modules — just the Go standard library.",
  tags:["PDF 1.7","object model","14 standard fonts","AFM widths","vector paths","stroke/fill","PNG + JPEG","word wrap","page sizes","metadata","xref/trailer","zero deps"],
  features:[
    "Document / Page model — <code>New</code>, <code>AddPage</code>, then <code>Save</code>, <code>Bytes</code> or <code>Write</code> emit a conforming PDF-1.7 catalog, page tree, xref and trailer",
    "The 14 standard Type1 fonts as package vars (<code>Helvetica</code>, <code>TimesRoman</code>, <code>Courier</code>, <code>Symbol</code>, <code>ZapfDingbats</code> …) or by name via <code>StandardFont</code>, with built-in AFM widths",
    "Text drawing &amp; layout — <code>SetFont</code>, <code>DrawText</code>, <code>DrawLines</code> and greedy word-wrapping <code>DrawParagraph</code>, with <code>Font.Width</code> / <code>TextWidth</code> for accurate measurement",
    "Vector paths — <code>MoveTo</code>, <code>LineTo</code>, <code>CurveTo</code> Bézier, <code>Rect</code>, <code>Circle</code>, <code>Ellipse</code> and <code>DrawLine</code>, painted with <code>Stroke</code>, <code>Fill</code> or <code>FillStroke</code>",
    "Colors &amp; state — RGB <code>Color</code> from <code>RGB</code> / <code>Gray</code>, applied via <code>SetFillColor</code>, <code>SetStrokeColor</code> and <code>SetLineWidth</code>, with <code>Save</code> / <code>Restore</code> graphics state",
    "Image embedding — <code>LoadImage</code> (auto-detect), <code>LoadPNG</code> (FlateDecode + soft mask) and <code>LoadJPEG</code> (DCTDecode) placed with <code>DrawImage</code> as image XObjects",
    "Page geometry — standard sizes <code>A4</code>, <code>Letter</code>, <code>Legal</code>, <code>Tabloid</code> …, plus <code>Custom</code> and <code>PageSize.Landscape</code> / <code>Portrait</code>",
    "Metadata &amp; read-back — <code>SetTitle</code>/<code>SetAuthor</code>/<code>SetSubject</code>/<code>SetKeywords</code> via <code>/Info</code>, plus a minimal xref <code>Reader</code>; zero dependencies, pure Go stdlib"
  ],
  node_code:
`const PDFDocument = require("pdfkit");
const fs = require("fs");

const doc = new PDFDocument({ size: "A4" });
doc.pipe(fs.createWriteStream("hello.pdf"));

doc.font("Helvetica").fontSize(24).text("Hello, PDF!", 72, 72);
doc.rect(72, 120, 200, 80).stroke();
doc.end();`,
  go_code:
`import "github.com/malcolmston/pdfkit"

doc := pdfkit.New()
page := doc.AddPage(pdfkit.A4)

page.SetFont(pdfkit.Helvetica, 24)
page.DrawText(72, 720, "Hello, PDF!")

page.Rect(72, 600, 200, 80)
page.Stroke()

doc.Save("hello.pdf")`,
  integrate:
`<span class="tok-c">// Start a document, set metadata, and add an A4 page.</span>
doc := pdfkit.New()
doc.SetTitle("Report")
doc.SetAuthor("malcolmston")
page := doc.AddPage(pdfkit.A4)

<span class="tok-c">// Draw a heading in blue, then a word-wrapped paragraph below it.</span>
page.SetFont(pdfkit.HelveticaBold, 24)
page.SetFillColor(pdfkit.Blue)
page.DrawText(72, 760, "Quarterly Report")

page.SetFont(pdfkit.TimesRoman, 12)
page.SetFillColor(pdfkit.Black)
nextY := page.DrawParagraph(72, 720, 400, 15,
	"A long run of text that is greedily wrapped to the given width in points.")

<span class="tok-c">// Vector graphics: a stroked rectangle and a filled circle.</span>
page.SetStrokeColor(pdfkit.RGB(200, 0, 0))
page.SetLineWidth(2)
page.Rect(72, nextY-120, 200, 80)
page.Stroke()

page.SetFillColor(pdfkit.RGB(0, 120, 200))
page.Circle(360, nextY-80, 40)
page.Fill()

<span class="tok-c">// Embed a PNG or JPEG (auto-detected) as an image XObject.</span>
if f, err := os.Open("logo.png"); err == nil {
	if img, err := pdfkit.LoadImage(f); err == nil {
		page.DrawImage(img, 72, 72, 120, 120)
	}
}

<span class="tok-c">// Serialize valid %PDF-1.7 bytes to a file (or Bytes / Write).</span>
if err := doc.Save("report.pdf"); err != nil {
	log.Fatal(err)
}`
  },
  {
  id:"sharp", name:"sharp", icon:'<i class="fa-solid fa-wand-magic-sparkles"></i>', accent:"#14b8a6",
  pkg:"github.com/malcolmston/sharp", node:"lovell/sharp",
  repo:"https://github.com/malcolmston/sharp", docs:"https://malcolmston.github.io/sharp/",
  tagline:"Fluent, chainable image processing for Go.",
  blurb:"A from-scratch, standard-library-only Go take on the ergonomics of Node's sharp: build a "+
    "Pipeline, chain operations, export to a format. Everything runs on the Go standard library "+
    "(image, image/color, image/png, image/jpeg and math) over an in-memory RGBA buffer — no cgo, "+
    "no third-party dependencies. Construct a pipeline from an image, file, byte slice or reader; the "+
    "source is copied on construction and never mutated. Operations apply eagerly and chain by "+
    "returning the same *Pipeline: resize with FitExact/FitContain/FitCover and Nearest/Bilinear "+
    "sampling, crop/extract, extend, rotate and flip, a full colour suite (grayscale, negate, tint, "+
    "brightness, contrast, gamma, saturation, threshold), separable-Gaussian blur, sharpen and generic "+
    "convolution, alpha-blended compositing and flatten, plus PNG/JPEG in and out. A deferred error "+
    "model retains the first failure and turns later steps into no-ops, so it surfaces once from the "+
    "terminal export or from Err.",
  tags:["fluent pipeline","resize","crop/extract","rotate/flip","colour ops","blur/sharpen","convolution","composite","PNG/JPEG I/O","deferred errors","zero deps","deterministic"],
  features:[
    "Fluent <code>Pipeline</code> built with <code>New</code>, <code>FromFile</code>, <code>FromBytes</code> or <code>FromReader</code> — copy-on-construct, so the source is never mutated",
    "Resize via <code>Resize(ResizeOptions{…})</code> and <code>ResizeTo</code> — <code>FitExact</code>/<code>FitContain</code>/<code>FitCover</code> with <code>Nearest</code> or <code>Bilinear</code> sampling",
    "Region &amp; layout — <code>Crop</code>/<code>Extract</code>, <code>Extend</code> (pad with a fill colour), <code>Rotate90</code>/<code>180</code>/<code>270</code>, arbitrary <code>Rotate</code>, <code>FlipVertical</code>/<code>FlipHorizontal</code> (aka <code>Flip</code>/<code>Flop</code>)",
    "Colour suite — <code>Grayscale</code>, <code>Negate</code>/<code>Invert</code>, <code>Tint</code>, <code>Brightness</code>, <code>Contrast</code>, <code>Gamma</code>, <code>Saturation</code>, <code>Threshold</code>",
    "Convolution — separable-Gaussian <code>Blur</code>, <code>Sharpen</code>, and a generic <code>Convolve</code> over <code>NewKernel</code>",
    "Composition — <code>Composite</code> with alpha blending plus <code>Gravity</code>/offset placement, and <code>Flatten</code> onto a solid background",
    "PNG + JPEG I/O — <code>ToPNG</code>, <code>ToJPEG</code>, <code>ToImage</code>, <code>ToFile</code> with <code>PNGOptions</code>/<code>JPEGOptions</code> quality control",
    "Deferred error model — the first failure is retained, later steps become no-ops, and it surfaces from the terminal call or <code>Err</code>",
    "Introspection — <code>Metadata</code> (width/height/format) and <code>Stats</code> (per-channel means), plus <code>Clone</code> to branch a pipeline",
    "Zero dependencies — pure Go standard library, nothing to audit but the toolchain"
  ],
  node_code:
`const sharp = require("sharp");

sharp("input.jpg")
  .resize(800, null, { fit: "inside" })
  .grayscale()
  .sharpen(1)
  .jpeg({ quality: 85 })
  .toFile("output.jpg");`,
  go_code:
`import "github.com/malcolmston/sharp"

buf, _ := sharp.FromFile("input.jpg").
	Resize(sharp.ResizeOptions{Width: 800, Fit: sharp.FitContain}).
	Grayscale().
	Blur(2).
	ToJPEG(85)
_ = sharp.FromFile("input.jpg").ToFile("output.jpg", sharp.FormatJPEG, 85)`,
  integrate:
`<span class="tok-c">// Build a pipeline from a file. The source image is copied on</span>
<span class="tok-c">// construction, so the file on disk is never touched.</span>
p := sharp.FromFile("photo.png").
	Resize(sharp.ResizeOptions{Width: 1200, Height: 630, Fit: sharp.FitCover, Interpolation: sharp.Bilinear}).
	Brightness(1.05).
	Contrast(1.1).
	Sharpen(0.8)

<span class="tok-c">// Branch the partially-built pipeline into an independent copy and</span>
<span class="tok-c">// produce a grayscale thumbnail without disturbing the original.</span>
thumb, _ := p.Clone().ResizeTo(320, 168).Grayscale().ToPNG()

<span class="tok-c">// Drop a watermark into the corner with alpha blending, flatten any</span>
<span class="tok-c">// transparency onto white, then encode to JPEG. The deferred error</span>
<span class="tok-c">// model means a single check at the end covers every step.</span>
logo, _ := sharp.FromFile("logo.png").ToImage()
buf, err := p.
	Composite(logo, sharp.CompositeOptions{UseGravity: true, Gravity: sharp.GravityBottomRight, Opacity: 0.8}).
	Flatten(sharp.White).
	ToJPEG(90)
if err != nil {
	log.Fatal(err)
}
_ = os.WriteFile("out.jpg", buf, 0o644)
_ = thumb`
  },
  {
  id:"puppeteer", name:"puppeteer", icon:'<i class="fa-solid fa-masks-theater"></i>', accent:"#40b5a4",
  pkg:"github.com/malcolmston/puppeteer", node:"puppeteer/puppeteer",
  repo:"https://github.com/malcolmston/puppeteer", docs:"https://malcolmston.github.io/puppeteer/",
  tagline:"Puppeteer-style page automation for Go, standard library only.",
  blurb:"A from-scratch, standard-library-only Go toolkit inspired by the Node.js Puppeteer API. It fetches "+
    "pages over net/http, parses the HTML with its own tokenizer and DOM builder, and queries the document "+
    "with a real CSS selector engine — no cgo, no third-party modules, not even golang.org/x/net. "+
    "Because a dependency-free Go library cannot embed a browser, it deliberately runs NO JavaScript and does "+
    "NO rendering: there is no script execution, no layout/geometry/screenshots, and no live DOM — the tree is "+
    "a static snapshot of the bytes the server sent. In practice it behaves like an HTTP client married to an "+
    "HTML parser and a selector engine, ideal for scraping and automating server-rendered pages, following "+
    "links, submitting forms and managing cookies and headers. Launch returns a Browser (cookie jar, shared "+
    "headers, user agent, per-navigation timeout); Browser.NewPage returns a Page whose Goto fetches and parses "+
    "a URL, and from which you select nodes, enumerate resolved Links and discover, fill and submit Forms. "+
    "The import path is github.com/malcolmston/puppeteer and the package is named puppeteer.",
  tags:["net/http","no JavaScript","no rendering","cookie jar","HTML tokenizer","DOM builder","CSS selectors",":nth-child(An+B)","Links","forms","zero deps"],
  features:[
    "<code>Launch</code> a <code>Browser</code> with a <code>LaunchOptions</code> cookie jar, shared headers, user agent, per-navigation timeout and custom transport",
    "<code>Browser.NewPage</code> returns a <code>Page</code>; <code>Page.Goto</code> / <code>GotoContext</code> fetch over <code>net/http</code>, follow redirects and update cookies",
    "Own HTML tokenizer + DOM builder — <code>Parse</code> never fails, recovering malformed input the way browsers do into a <code>Node</code> tree",
    "A real CSS selector engine — <code>QuerySelector</code> and <code>QuerySelectorAll</code> supporting type/<code>*</code>, <code>#id</code>, <code>.class</code>, attribute and combinator selectors",
    "Structural pseudo-classes including the <code>:nth-child()</code> <code>An+B</code> microsyntax (<code>odd</code>, <code>even</code>, <code>2n+1</code>, <code>-n+3</code>)",
    "<code>Element</code> handles — <code>TextContent</code>, <code>InnerHTML</code>, <code>OuterHTML</code>, <code>Attr</code>/<code>Attributes</code>, <code>ClassList</code>/<code>HasClass</code> and node-relative queries",
    "DOM traversal — <code>Children</code>, <code>Parent</code>, <code>Next</code>, <code>Prev</code>, <code>Closest</code> and <code>Matches</code>",
    "Resolved <code>Links</code> — every <code>a[href]</code> turned into an absolute URL against the page's location",
    "Form automation — <code>Forms</code>, <code>FormBySelector</code>, <code>FillForm</code>, then <code>BuildRequest</code> or <code>Submit</code> (GET query or POST body)",
    "Cookies &amp; headers — <code>Cookies</code>, <code>SetCookies</code>, <code>SetUserAgent</code>, <code>SetExtraHTTPHeaders</code> over a <code>net/http/cookiejar</code>",
    "<b>No JavaScript, no rendering</b> — no script execution, no layout/geometry/screenshots, no live DOM; a static snapshot of server-sent bytes",
    "Zero dependencies — pure Go standard library, nothing to audit but the toolchain"
  ],
  node_code:
`import puppeteer from "puppeteer";

const browser = await puppeteer.launch();
const page = await browser.newPage();
await page.goto("https://example.com");

console.log("title:", await page.title());

const links = await page.$$eval("a[href]", (as) =>
  as.map((a) => [a.getAttribute("href"), a.textContent]),
);
for (const [href, text] of links) console.log("link:", href, "->", text);

await browser.close();`,
  go_code:
`import "github.com/malcolmston/puppeteer"

browser, _ := puppeteer.Launch(nil)
defer browser.Close()

page := browser.NewPage()
if _, err := page.Goto("https://example.com"); err != nil {
	log.Fatal(err)
}

fmt.Println("title:", page.Title())

links, _ := page.QuerySelectorAll("a[href]")
for _, a := range links {
	href, _ := a.Attr("href")
	fmt.Println("link:", href, "->", a.TextContent())
}`,
  integrate:
`<span class="tok-c">// Launch a browser with a custom user agent and a shared header; the</span>
<span class="tok-c">// cookie jar stores server-set cookies and replays them automatically.</span>
browser, _ := puppeteer.Launch(&puppeteer.LaunchOptions{
	UserAgent: "my-scraper/1.0",
	Headers:   map[string]string{"Accept-Language": "en"},
})
defer browser.Close()

page := browser.NewPage()
page.Goto("https://example.com/login")

<span class="tok-c">// Discover a form by selector, fill named fields, and submit it —</span>
<span class="tok-c">// FillForm returns a Form you can Submit (GET query or POST body).</span>
form, _ := page.FillForm("#login", map[string]string{
	"username": "alice",
	"password": "hunter2",
})
resp, _ := form.Submit()
fmt.Println("status:", resp.StatusCode)

<span class="tok-c">// Query with the selector engine, including :nth-child(An+B), then</span>
<span class="tok-c">// traverse the static DOM snapshot and read attributes.</span>
rows, _ := page.QuerySelectorAll("table.results tr:nth-child(odd)")
for _, tr := range rows {
	if link, _ := tr.QuerySelector("a[href]"); link != nil {
		href, _ := link.Attr("href")
		fmt.Println(link.TextContent(), "->", href)
	}
}

<span class="tok-c">// Every href resolved to an absolute URL against the page location.</span>
fmt.Println(page.Links())`
  },
  {
  id:"liveview", name:"LiveView", icon:'<i class="fa-solid fa-bolt"></i>', accent:"#fd4f00",
  pkg:"github.com/malcolmston/liveview", node:"phoenixframework/phoenix_live_view",
  repo:"https://github.com/malcolmston/liveview", docs:"https://malcolmston.github.io/liveview/",
  tagline:"Phoenix LiveView-style reactive server-rendered UI for Go.",
  blurb:"A from-scratch, standard-library-only Go take on Phoenix LiveView: server-held state drives the "+
    "UI, the browser sends events, and the server ships back a minimal diff describing only what changed. "+
    "The core trick is LiveView's static/dynamic split — a template is compiled once into the literal "+
    "fragments that never change and the interpolated values that do, so unchanged HTML never travels "+
    "twice. You implement the View interface (Mount / HandleEvent / Render), keep per-connection state in "+
    "a Socket with per-key change tracking, and let a Session run the mount → render → diff cycle; a tiny "+
    "net/http Handler adds an optional HTTP + JSON transport on top. No cgo, no third-party dependencies — "+
    "the import path and package are both liveview, and diffs marshal to the same compact JSON shape "+
    "Phoenix LiveView uses.",
  tags:["View lifecycle","Socket assigns","change tracking","static/dynamic split","minimal diffs","Rendered tree","Session runtime","net/http Handler"],
  features:[
    "The <code>View</code> lifecycle — <code>Mount</code> seeds state, <code>HandleEvent</code> reacts to a client event, <code>Render</code> returns a static/dynamic tree",
    "Server-held state in a <code>Socket</code> — <code>Assign</code>/<code>AssignAll</code> writes, <code>GetInt</code>/<code>GetString</code>/<code>Get</code> reads, with per-key <code>Changed</code> tracking",
    "Tiny <code>{{ name }}</code> templates compiled once via <code>MustParse</code> / <code>Parse</code> into a fixed static/dynamic <code>Template</code>",
    "A <code>Rendered</code> tree, not a string — <code>Statics</code> + <code>Dynamics</code> with the <code>len(Statics) == len(Dynamics)+1</code> invariant, and <code>HTML()</code> to materialise it",
    "Minimal-patch diff engine — <code>DiffRendered</code> emits only the changed dynamic slots (recursing into nested components), <code>FullDiff</code> sends the first frame with statics under <code>\"s\"</code>",
    "Auto HTML-escaping by default, with <code>Safe</code> to opt trusted markup out of escaping",
    "A per-connection <code>Session</code> (<code>NewSession</code>) that drives mount → render → diff; <code>Session.Event</code> returns just the <code>Diff</code>",
    "An optional <code>net/http</code> transport — <code>NewHandler</code> serves the initial HTML page and accepts JSON events, so the state → render → diff core stays independent of the wire",
    "Zero dependencies — pure Go standard library, nothing to audit but the toolchain"
  ],
  node_code:
`defmodule CounterLive do
  use Phoenix.LiveView

  def mount(_params, _session, socket) do
    {:ok, assign(socket, count: 0, label: "Clicks")}
  end

  def handle_event("inc", _params, socket) do
    {:noreply, update(socket, :count, &(&1 + 1))}
  end

  def render(assigns) do
    ~H"""
    <div><h1><%= @label %></h1><span class="value"><%= @count %></span></div>
    """
  end
end`,
  go_code:
`import "github.com/malcolmston/liveview"

var tmpl = liveview.MustParse(
	` + "`<div><h1>{{ label }}</h1><span class=\"value\">{{ count }}</span></div>`" + `)

type Counter struct{}

func (Counter) Mount(_ map[string]any, s *liveview.Socket) error {
	s.Assign("count", 0)
	s.Assign("label", "Clicks")
	return nil
}

func (Counter) HandleEvent(e string, _ map[string]any, s *liveview.Socket) error {
	if e == "inc" {
		s.Assign("count", s.GetInt("count")+1)
	}
	return nil
}

func (Counter) Render(a map[string]any) *liveview.Rendered { return tmpl.Render(a) }`,
  integrate:
`<span class="tok-c">// The state → render → diff core stands alone — no HTTP required.</span>
sess := liveview.NewSession(&liveview.Counter{Start: 0})

<span class="tok-c">// Mount runs Mount + the first Render and caches it; HTML() is the full page.</span>
initial, _ := sess.Mount(map[string]any{"label": "Clicks"})
_ = initial.HTML() <span class="tok-c">// <div class="counter"><h1>Clicks</h1><span class="value">0</span></div></span>

<span class="tok-c">// An event re-renders, diffs against the previous frame, and returns only</span>
<span class="tok-c">// the changed dynamic slots. Here just the count slot moves.</span>
diff, _ := sess.Event("inc", nil)
<span class="tok-c">// diff marshals to {"1":"1"} — statics and the unchanged label are omitted.</span>

<span class="tok-c">// The very first frame is a FullDiff: every dynamic plus the statics under "s",</span>
<span class="tok-c">// so a fresh client can rebuild the document: {"s":[...],"0":"Clicks","1":"0"}.</span>
full := liveview.FullDiff(sess.Render())

<span class="tok-c">// Mount the same View over plain HTTP + JSON when you want a transport.</span>
h := liveview.NewHandler("/", func() liveview.View { return &liveview.Counter{} })
_ = diff
_ = full
_ = h`
  },
  {
  id:"cheerio", name:"cheerio", icon:'<i class="fa-solid fa-sitemap"></i>', accent:"#ff7a45",
  pkg:"github.com/malcolmston/cheerio", node:"cheeriojs/cheerio",
  repo:"https://github.com/malcolmston/cheerio", docs:"https://malcolmston.github.io/cheerio/",
  tagline:"HTML parsing and jQuery-style traversal for Go.",
  blurb:"A dependency-free, standard-library-only Go take on the Node.js cheerio package: tolerant HTML "+
    "parsing plus a chainable, jQuery-style traversal API. Everything is built from scratch — its own HTML "+
    "tokenizer and tree-construction parser, a CSS selector engine, and the Selection API — with no cgo and "+
    "no third-party code, not even golang.org/x/net. Load never fails: malformed markup is recovered the way "+
    "a browser would into a *Node tree, then Find returns an ordered, de-duplicated Selection you can filter, "+
    "traverse, read and lightly mutate. The import path is github.com/malcolmston/cheerio and the package is "+
    "named cheerio.",
  tags:["Load()","CSS selectors","jQuery-style API","own tokenizer","tree builder",":nth-child","combinators","zero deps"],
  features:[
    "Tolerant parsing — <code>Load</code> recovers malformed HTML into a <code>*Node</code> tree (<code>DocumentNode</code>/<code>ElementNode</code>/<code>TextNode</code>/<code>CommentNode</code>/<code>DoctypeNode</code>) using its own tokenizer + tree builder, no <code>golang.org/x/net</code>",
    "CSS selector engine — type/<code>*</code>/<code>#id</code>/<code>.class</code>, attribute matches (<code>[a^=v]</code>, <code>[a$=v]</code>, <code>[a*=v]</code>, <code>[a~=v]</code>, <code>[a|=v]</code>), combinators <code>&gt;</code> <code>+</code> <code>~</code>, and pseudo-classes <code>:nth-child(an+b)</code>/<code>:not</code>",
    "Chainable traversal — <code>Find</code>, <code>Children</code>, <code>Parent</code>, <code>Parents</code>, <code>Closest</code>, <code>Siblings</code>, <code>Next</code>/<code>Prev</code>, <code>Filter</code>, <code>Not</code>, <code>Is</code>, <code>Has</code>, <code>Eq</code>, <code>First</code>/<code>Last</code>, <code>Each</code>, <code>Map</code>",
    "Accessors — <code>Attr</code>/<code>AttrOr</code>, <code>Text</code>, <code>Html</code> (inner), <code>OuterHtml</code>, <code>HasClass</code>, <code>Val</code>, <code>TagName</code>",
    "Light mutation — <code>SetAttr</code>, <code>RemoveAttr</code>, <code>AddClass</code>, <code>RemoveClass</code>, <code>ToggleClass</code>, <code>SetText</code>",
    "Browser-like recovery — void elements take no children, self-closing syntax is honored, implicit-close rules fire for <code>&lt;p&gt;</code>/<code>&lt;li&gt;</code>/<code>&lt;tr&gt;</code>, raw-text <code>&lt;script&gt;</code>/<code>&lt;style&gt;</code>/<code>&lt;textarea&gt;</code> capture verbatim, and named/decimal/hex character references are decoded",
    "Deterministic — identical input yields identical output and Selections preserve document order; <code>Val</code> resolves <code>&lt;input&gt;</code>/<code>&lt;select&gt;</code>/<code>&lt;textarea&gt;</code> values",
    "Zero dependencies — pure Go standard library, no cgo, nothing to audit but the toolchain"
  ],
  node_code:
`import * as cheerio from 'cheerio';

const $ = cheerio.load(\`
  <ul id="fruit">
    <li class="a">Apple</li>
    <li class="b">Banana</li>
  </ul>
  <a href="/go">Go</a>\`);

// Text of the first list item.
console.log($('#fruit li').first().text()); // Apple

// Iterate over matched elements.
$('li').each((i, el) => console.log(i, $(el).text()));

// Attribute access.
console.log($('a').attr('href')); // /go`,
  go_code:
`import "github.com/malcolmston/cheerio"

doc := cheerio.Load(\`
  <ul id="fruit">
    <li class="a">Apple</li>
    <li class="b">Banana</li>
  </ul>
  <a href="/go">Go</a>\`)

// Text of the first list item.
fmt.Println(doc.Find("#fruit li").First().Text()) // Apple

// Iterate over matched elements.
doc.Find("li").Each(func(i int, n *cheerio.Node) {
    fmt.Println(i, n.Children[0].Data)
})

// Attribute access plus a pseudo-class.
href, _ := doc.Find("a").Attr("href")
fmt.Println(href, doc.Find("li:nth-child(odd)").Length()) // /go 1`,
  integrate:
`<span class="tok-c">// Load never fails: malformed markup is recovered the way a browser would.</span>
doc := cheerio.Load(page)

<span class="tok-c">// Combinators + attribute selectors — collect every off-site nav link.</span>
var external []string
doc.Find("nav a[href^=\\"http\\"]").Each(func(i int, n *cheerio.Node) {
    if href, ok := n.AttrValue("href"); ok {
        external = append(external, href)
    }
})

<span class="tok-c">// Pseudo-classes + light mutation: stripe the odd, non-header table rows.</span>
doc.Find("table tr:nth-child(odd):not(.head)").AddClass("zebra")

<span class="tok-c">// Traverse up the tree, then read text and serialized HTML back out.</span>
price := doc.Find(".product .price").First()
card := price.Closest(".product")
fmt.Println(card.Find("h2").Text(), price.Text(), card.OuterHtml())`
  },
  {
  id:"handlebars", name:"Handlebars", icon:'<i class="fa-solid fa-code"></i>', accent:"#f0772b",
  pkg:"github.com/malcolmston/handlebars", node:"handlebars-lang/handlebars.js",
  repo:"https://github.com/malcolmston/handlebars", docs:"https://malcolmston.github.io/handlebars/",
  tagline:"Logic-less Handlebars/Mustache templating in pure Go.",
  blurb:"A dependency-free Handlebars/Mustache templating engine implemented entirely in Go's standard library. "+
    "It ships its own lexer, parser and AST, then renders that tree against any value using reflection — it does "+
    "not wrap text/template, so the Handlebars semantics are implemented directly and match Handlebars.js. "+
    "You get escaped and raw interpolation, rich path expressions with ../parent and @root, the full set of "+
    "built-in block helpers, custom helpers with hash arguments and subexpressions, partials, comments and "+
    "whitespace control — over maps, structs, slices and pointers.",
  tags:["{{mustache}}","HTML escaping","path expressions","block helpers","{{#each}}","custom helpers","partials","zero deps"],
  features:[
    "Interpolation — <code>{{expr}}</code> HTML-escapes; <code>{{{expr}}}</code> and <code>{{&amp; expr}}</code> emit raw, matching the Handlebars.js escape set",
    "Path expressions — <code>foo.bar.baz</code>, index <code>foo.0</code>, <code>this</code>/<code>.</code>, parent <code>../foo</code>, <code>@root</code> and bracketed <code>[a b]</code> segments",
    "Block helpers — <code>{{#if}}</code>/<code>{{else}}</code>, <code>{{#unless}}</code>, <code>{{#each}}</code> and <code>{{#with}}</code>, plus inverse sections <code>{{^cond}}</code>",
    "<code>{{#each}}</code> over slices <b>and</b> maps, exposing <code>@index</code>, <code>@key</code>, <code>@first</code>, <code>@last</code> with map keys visited in sorted order",
    "Built-in inline helpers — <code>lookup</code>, <code>eq</code>, <code>ne</code>, <code>not</code>, <code>and</code>, <code>or</code>, <code>gt</code>, <code>lt</code>, <code>gte</code>, <code>lte</code> — usable in subexpressions like <code>{{#if (eq a b)}}</code>",
    "Custom helpers via <code>RegisterHelper</code> — positional <code>Options.Arg</code>, hash args <code>Options.HashStr</code>, block callbacks <code>Options.Fn</code>/<code>Options.Inverse</code>, and <code>SafeString</code> to bypass escaping",
    "Partials via <code>RegisterPartial</code> — <code>{{&gt; name}}</code>, explicit context <code>{{&gt; name ctx}}</code>, hash overlays and dynamic <code>{{&gt; (expr)}}</code>, with indentation preserved",
    "Comments <code>{{! ... }}</code>/<code>{{!-- ... --}}</code>, standalone-line stripping and explicit whitespace control with <code>{{~ ~}}</code> — all with zero third-party dependencies"
  ],
  node_code:
`const Handlebars = require("handlebars");

const tmpl = Handlebars.compile(
  "{{#each items}}{{@index}}:{{this}} {{/each}}"
);
console.log(tmpl({ items: ["a", "b"] }));
// 0:a 1:b`,
  go_code:
`import "github.com/malcolmston/handlebars"

// One-shot render, or compile once with Parse/MustParse and reuse.
t := handlebars.MustParse("{{#each items}}{{@index}}:{{this}} {{/each}}")
out := t.MustRender(map[string]any{"items": []string{"a", "b"}})
// out == "0:a 1:b "`,
  integrate:
`t := handlebars.New()

<span class="tok-c">// A custom inline helper returning a SafeString, so its markup</span>
<span class="tok-c">// is not re-escaped by the {{ }} interpolation.</span>
t.RegisterHelper("shout", func(o *handlebars.Options) any {
	return handlebars.SafeString("<em>" + fmt.Sprint(o.Arg(0)) + "!</em>")
})

<span class="tok-c">// A block helper wraps its rendered body via o.Fn.</span>
t.RegisterHelper("bold", func(o *handlebars.Options) any {
	return "<b>" + o.Fn(o.Arg(0)) + "</b>"
})

<span class="tok-c">// A partial reused inside an {{#each}} loop, with @index and</span>
<span class="tok-c">// a value pulled from the parent context via ../team.</span>
_ = t.RegisterPartial("row", "{{@index}}. {{name}} ({{team}})\\n")
_ = t.ParseString("{{shout title}}\\n{{#each people}}{{> row team=../team}}{{/each}}")

out, _ := t.Render(map[string]any{
	"title":  "team",
	"team":   "Blue",
	"people": []map[string]any{{"name": "Ada"}, {"name": "Lin"}},
})`
  },
  {
  id:"axios", name:"axios", icon:'<i class="fa-solid fa-arrow-right-arrow-left"></i>', accent:"#a78bfa",
  pkg:"github.com/malcolmston/axios", node:"axios/axios",
  repo:"https://github.com/malcolmston/axios", docs:"https://malcolmston.github.io/axios/",
  tagline:"An ergonomic, axios-style HTTP client for Go.",
  blurb:"A from-scratch, standard-library-only Go HTTP client that brings the ergonomics of JavaScript's axios "+
    "to net/http. Create a configured client with axios.New(Config{...}) — BaseURL, default Headers and query "+
    "Params, a Timeout, Basic or Bearer auth, retries, and request/response interceptors — or reach for the "+
    "package-level Get/Post helpers backed by a default client. Verb methods encode request bodies automatically "+
    "by their dynamic type (JSON for structs and maps, form-urlencoded for url.Values, raw for []byte/string/"+
    "io.Reader) and return a rich Response with JSON, Text, Bytes, OK and Header helpers. Non-2xx statuses "+
    "resolve to a typed *Error that still carries the parsed Response, ValidateStatus lets you redefine success, "+
    "and the generic GetJSON[T] fetches and decodes in a single call. No cgo, no third-party dependencies.",
  tags:["net/http","BaseURL","interceptors","retries","Bearer/Basic auth","auto body encoding","typed errors","GetJSON[T]","form + JSON","ValidateStatus","zero deps"],
  features:[
    "Configurable client via <code>New</code> and <code>Config</code> (<code>BaseURL</code>, <code>Headers</code>, <code>Params</code>, <code>Timeout</code>, <code>Context</code>) plus package-level <code>Get</code>/<code>Post</code>/… helpers backed by <code>Default</code>/<code>SetDefault</code>",
    "Full set of verb methods — <code>Get</code>, <code>Delete</code>, <code>Head</code>, <code>Options</code>, <code>Post</code>, <code>Put</code>, <code>Patch</code> — all built on the low-level <code>Request</code>",
    "Automatic body encoding by dynamic type via <code>EncodeBody</code>: JSON for structs/maps, form for <code>url.Values</code>, raw for <code>[]byte</code>/<code>string</code>/<code>io.Reader</code>",
    "Rich <code>Response</code> with <code>JSON</code>, <code>Text</code>, <code>Bytes</code>, <code>OK</code> and <code>Header</code> helpers over a fully-buffered body",
    "Authentication built in — <code>BearerToken</code> and <code>BasicAuth</code>, overridable per request through <code>RequestConfig</code>",
    "Ordered request &amp; response interceptors — <code>RequestInterceptor</code> and <code>ResponseInterceptor</code> — that mutate or transform in place",
    "Configurable retries via <code>RetryConfig</code> with <code>DefaultBackoff</code> (exponential) and <code>DefaultRetryOn</code> (transport errors + 5xx)",
    "Typed <code>*Error</code> that carries the parsed <code>Response</code> on rejected statuses, unwraps transport errors, and exposes <code>StatusCode</code>",
    "Redefine success with <code>ValidateStatus</code> (per client or per request), just like axios <code>validateStatus</code>",
    "Generic one-liner fetch + decode — <code>GetJSON[T]</code> and <code>GetJSONDefault[T]</code>",
    "Zero dependencies — pure Go standard library, nothing to audit but the toolchain"
  ],
  node_code:
`import axios from "axios";

const api = axios.create({
  baseURL: "https://api.example.com",
  timeout: 5000,
  headers: { Authorization: "Bearer secret-token" },
});

const { data } = await api.get("/users/1", { params: { expand: "profile" } });
console.log(data.name);

await api.post("/users", { name: "Ada", age: 36 });`,
  go_code:
`import "github.com/malcolmston/axios"

client := axios.New(axios.Config{
	BaseURL:     "https://api.example.com",
	Timeout:     5 * time.Second,
	BearerToken: "secret-token",
})

resp, _ := client.Get("/users/1", &axios.RequestConfig{
	Params: url.Values{"expand": {"profile"}},
})
var u User
_ = resp.JSON(&u)

// One-liner fetch + decode with generics.
u2, _ := axios.GetJSON[User](client, "/users/2")

client.Post("/users", User{Name: "Ada", Age: 36})`,
  integrate:
`<span class="tok-c">// Retry 5xx/network errors and tag every outgoing request via an interceptor.</span>
client := axios.New(axios.Config{
	BaseURL: "https://api.example.com",
	Retry:   &axios.RetryConfig{Retries: 3}, <span class="tok-c">// exponential backoff by default</span>
	RequestInterceptors: []axios.RequestInterceptor{
		func(req *http.Request) error { req.Header.Set("X-Trace", "abc"); return nil },
	},
})

<span class="tok-c">// POST a struct — it is JSON-encoded automatically by its dynamic type.</span>
resp, err := client.Post("/users", User{Name: "Ada", Age: 36})

<span class="tok-c">// Non-2xx becomes a typed *Error whose Response still holds the body.</span>
var aerr *axios.Error
if errors.As(err, &aerr) {
	fmt.Println(aerr.StatusCode(), aerr.Response.Text())
}

<span class="tok-c">// Redefine success: accept anything below 500.</span>
ok, _ := client.Get("/maybe", &axios.RequestConfig{
	ValidateStatus: func(status int) bool { return status < 500 },
})
fmt.Println(ok.OK(), ok.Header("Content-Type"))`
  },
  {
  id:"lodash", name:"lodash", icon:'<i class="fa-solid fa-toolbox"></i>', accent:"#f2a33c",
  pkg:"github.com/malcolmston/lodash", node:"lodash/lodash",
  repo:"https://github.com/malcolmston/lodash", docs:"https://malcolmston.github.io/lodash/",
  tagline:"Type-safe, lodash-style functional utilities for Go.",
  blurb:"A comprehensive, type-safe functional utility library for Go, inspired by JavaScript's lodash and built "+
    "entirely on the standard library and Go 1.24 generics — no third-party dependencies, no cgo, no reflection "+
    "on hot paths. Roughly 98 helpers span collections and slices (Map/Filter/Reduce/GroupBy/Uniq/Chunk/"+
    "Difference/Union/SortBy), math (Sum/Mean/Min/Max/Clamp/Range), objects and nested maps (Keys/Pick/Omit/"+
    "Invert/Merge/Get), string casing (CamelCase/SnakeCase/KebabCase/Deburr) and function wrappers (Once/Memoize/"+
    "Debouncer/Throttler). Every function is pure and non-mutating unless documented otherwise: helpers return new "+
    "slices or maps rather than modifying their arguments. Any randomness is injected through a *math/rand.Rand, so "+
    "Sample, SampleN and Shuffle stay fully deterministic and testable, while Debouncer and Throttler take "+
    "injectable timers and clocks. The import path is github.com/malcolmston/lodash and the package is named lodash.",
  tags:["generics","pure/non-mutating","zero dependencies","collections","math","objects","strings","functions"],
  features:[
    "Collections — <code>Map</code>, <code>Filter</code>, <code>Reduce</code>, <code>GroupBy</code>, <code>KeyBy</code>, <code>CountBy</code>, <code>Partition</code>, <code>ForEach</code>",
    "Querying — <code>Find</code>, <code>FindIndex</code>, <code>Every</code>, <code>Some</code>, <code>Includes</code>, <code>IndexOf</code>, <code>LastIndexOf</code>",
    "Set &amp; shape ops — <code>Uniq</code>, <code>UniqBy</code>, <code>Chunk</code>, <code>Flatten</code>, <code>FlattenDeep</code>, <code>Difference</code>, <code>Intersection</code>, <code>Union</code>, <code>Zip</code>, <code>Unzip</code>",
    "Ordering &amp; slicing — <code>SortBy</code>, <code>OrderBy</code>, <code>Reverse</code>, <code>Take</code>, <code>TakeRight</code>, <code>Drop</code>, <code>DropRight</code>",
    "Randomness (seeded) — <code>Sample</code>, <code>SampleN</code>, <code>Shuffle</code> over an injected <code>*math/rand.Rand</code>",
    "Math &amp; numbers — <code>Sum</code>, <code>SumBy</code>, <code>Mean</code>, <code>Min</code>, <code>Max</code>, <code>Clamp</code>, <code>Range</code>, <code>RangeStep</code>, <code>InRange</code>",
    "Objects &amp; maps — <code>Keys</code>, <code>Values</code>, <code>Entries</code>, <code>Pick</code>, <code>Omit</code>, <code>MapKeys</code>, <code>MapValues</code>, <code>Invert</code>, <code>Merge</code>, <code>Assign</code>",
    "Nested access — <code>Get</code> and <code>Has</code> resolve dot-separated paths into <code>map[string]any</code> trees",
    "String casing — <code>CamelCase</code>, <code>PascalCase</code>, <code>SnakeCase</code>, <code>KebabCase</code>, <code>StartCase</code>, <code>Capitalize</code>, <code>Deburr</code>",
    "String shaping — <code>Words</code>, <code>Pad</code>, <code>PadStart</code>, <code>PadEnd</code>, <code>Truncate</code>, <code>Repeat</code>, <code>Trim</code>",
    "Function wrappers — <code>Once</code>, <code>Memoize</code>, <code>MemoizeBy</code>, <code>After</code>, <code>Before</code>",
    "Rate control — <code>NewDebouncer</code> (<code>Debouncer</code>) and <code>NewThrottler</code> (<code>Throttler</code>) with injectable timers and clocks",
    "Type-safe via Go 1.24 generics — no reflection on hot paths, no <code>interface{}</code> juggling at call sites",
    "Zero dependencies — pure Go standard library, nothing to audit but the toolchain"
  ],
  node_code:
`const _ = require("lodash");

const nums = [1, 2, 3, 4, 5, 6];
const evens = _.filter(nums, n => n % 2 === 0);
const doubled = _.map(evens, n => n * 2);
const total = _.sum(doubled);   // 24

_.camelCase("Foo Bar-baz");     // "fooBarBaz"
_.snakeCase("fooBarBaz");       // "foo_bar_baz"`,
  go_code:
`import lodash "github.com/malcolmston/lodash"

nums := []int{1, 2, 3, 4, 5, 6}
evens := lodash.Filter(nums, func(n int) bool { return n%2 == 0 })
doubled := lodash.Map(evens, func(n int) int { return n * 2 })
total := lodash.Sum(doubled) // 24

lodash.CamelCase("Foo Bar-baz") // "fooBarBaz"
lodash.SnakeCase("fooBarBaz")   // "foo_bar_baz"`,
  integrate:
`<span class="tok-c">// Group a slice of records by a derived key, then reduce each bucket.</span>
type User struct{ Name, City string; Age int }
users := []User{{"Ada", "London", 36}, {"Linus", "Helsinki", 54}, {"Grace", "London", 45}}
byCity := lodash.GroupBy(users, func(u User) string { return u.City })
oldest, _ := lodash.MaxBy(byCity["London"], func(u User) int { return u.Age })

<span class="tok-c">// Sort and de-duplicate without mutating the input, then take the top two.</span>
ages := lodash.Map(users, func(u User) int { return u.Age })
ranked := lodash.Reverse(lodash.SortBy(lodash.Uniq(ages), func(a int) int { return a }))
top := lodash.Take(ranked, 2)

<span class="tok-c">// Reshape a map: keep only some keys, then invert it.</span>
scores := map[string]int{"ada": 36, "linus": 54, "grace": 45}
picked := lodash.Pick(scores, "ada", "grace")
byScore := lodash.Invert(picked)

<span class="tok-c">// Reach into a nested map[string]any config with a dotted path.</span>
cfg := map[string]any{"db": map[string]any{"host": "localhost", "port": 5432}}
host, ok := lodash.Get(cfg, "db.host")`
  },
  {
  id:"moment", name:"Moment", icon:'<i class="fa-solid fa-clock"></i>', accent:"#f59e0b",
  pkg:"github.com/malcolmston/moment", node:"moment/moment",
  repo:"https://github.com/malcolmston/moment", docs:"https://malcolmston.github.io/moment/",
  tagline:"moment.js-style dates and times in Go.",
  blurb:"A from-scratch, standard-library-only Go take on moment.js, layered directly on the time package with "+
    "no cgo and no third-party dependencies. A <code>Moment</code> is an immutable wrapper around "+
    "<code>time.Time</code>: every manipulation method returns a new value and never mutates the receiver, so "+
    "moments are safe to share. You get moment-token parsing and formatting (YYYY, MMMM, dddd, HH…), unit-based "+
    "Add/Subtract/StartOf/EndOf/Set arithmetic, float and integer Diff, comparison and query helpers, timezone "+
    "reinterpretation, and humanized relative time (FromNow, Calendar, Humanize) driven by an injectable Clock so "+
    "tests stay deterministic. The import path and package name are both moment.",
  tags:["immutable Moment","time.Time","moment tokens","Format/ParseFormat","Add/StartOf","Diff","FromNow","injectable Clock","zero deps"],
  features:[
    "Immutable <code>Moment</code> over <code>time.Time</code> — every method returns a new value; construct with <code>New</code>, <code>FromTime</code>, <code>Now</code>, <code>Unix</code>, <code>UnixMilli</code> or <code>DateTime</code>",
    "moment-token <code>Format</code> and <code>ParseFormat</code> (<code>YYYY</code>/<code>MMMM</code>/<code>dddd</code>/<code>HH</code>…) with <code>[literal]</code> escaping, plus raw-layout <code>FormatLayout</code>/<code>ParseLayout</code> and forgiving <code>Parse</code>",
    "Unit arithmetic — <code>Add</code>, <code>Subtract</code>, <code>AddDuration</code>, <code>StartOf</code>, <code>EndOf</code> and <code>Set</code> keyed by the <code>Unit</code> constants (<code>Year</code>…<code>Millisecond</code>) with moment.js aliases",
    "Comparison &amp; query — <code>IsBefore</code>, <code>IsAfter</code>, <code>IsSame</code>, <code>IsBetween</code>, <code>IsSameUnit</code> and getters like <code>Year</code>, <code>Weekday</code>, <code>DayOfYear</code>, <code>ISOWeek</code>",
    "Difference in any unit — <code>Diff</code> (float64), <code>DiffInt</code> (truncated) and <code>DiffDuration</code> (<code>time.Duration</code>), with moment.js-style fractional month math",
    "Humanized relative time — <code>FromNow</code>, <code>From</code>, <code>To</code>, <code>ToNow</code>, <code>Calendar</code> and package-level <code>Humanize</code> (\"in 3 days\", \"Today at 2:30 PM\")",
    "Deterministic clock — the injectable <code>Clock</code> interface with <code>FixedClock</code> and <code>WithClock</code> makes relative-time output reproducible in tests",
    "Time zones — <code>In</code>, <code>UTC</code> and <code>Local</code> reinterpret a moment in another <code>time.Location</code> without changing the instant",
    "Zero dependencies — pure Go standard library, no cgo, nothing to audit but the toolchain"
  ],
  node_code:
`const moment = require('moment');

const m = moment('14/07/2017 02:40', 'DD/MM/YYYY HH:mm');
console.log(m.format('dddd, MMMM D, YYYY [at] h:mm A'));
// Friday, July 14, 2017 at 2:40 AM

const later = m.clone().add(2, 'hours');
console.log(m.isBefore(later), later.diff(m, 'minutes'));
// true 120`,
  go_code:
`import "github.com/malcolmston/moment"

// The import path and the package name are both moment.
m, _ := moment.ParseFormat("14/07/2017 02:40", "DD/MM/YYYY HH:mm")
fmt.Println(m.Format("dddd, MMMM D, YYYY [at] h:mm A"))
// Friday, July 14, 2017 at 2:40 AM

later := m.Add(2, moment.Hour)
fmt.Println(m.IsBefore(later), later.DiffInt(m, moment.Minute))
// true 120`,
  integrate:
`<span class="tok-c">// Parse moment-style tokens, then manipulate immutably — every</span>
<span class="tok-c">// call returns a new Moment, so m is untouched.</span>
m, _ := moment.ParseFormat("14/07/2017 02:40", "DD/MM/YYYY HH:mm")
start := m.StartOf(moment.Day)
end := m.Add(3, moment.Day).EndOf(moment.Day)
fmt.Println(end.DiffInt(start, moment.Day)) // 3

<span class="tok-c">// Reinterpret the same instant in another time zone.</span>
ny, _ := time.LoadLocation("America/New_York")
fmt.Println(m.In(ny).Format("h:mm A Z"))

<span class="tok-c">// Inject a fixed clock so relative time is deterministic in tests.</span>
clock := moment.FixedClock(time.Date(2017, 7, 14, 2, 40, 0, 0, time.UTC))
soon := m.WithClock(clock).Add(2, moment.Hour)
fmt.Println(soon.FromNow())                       // in 2 hours
fmt.Println(soon.Calendar(moment.NowWith(clock))) // Today at 4:40 AM`
  },
  {
  id:"jest", name:"Jest", icon:'<i class="fa-solid fa-vial-circle-check"></i>', accent:"#e64b3c",
  pkg:"github.com/malcolmston/jest", node:"jestjs/jest",
  repo:"https://github.com/malcolmston/jest", docs:"https://malcolmston.github.io/jest/",
  tagline:"Jest-style fluent assertions and mocking for Go.",
  blurb:"A standard-library-only Go framework that brings the expressive, fluent feel of JavaScript's Jest to "+
    "Go tests, layered directly on top of the standard testing package. The entry point is a generic "+
    "Expect[T] that returns a fluent Matcher[T] with a full family of matchers — ToBe, ToEqual, ToContain, "+
    "ToHaveLen, ToMatch, ToBeCloseTo, numeric ordering and ToPanic — each invertible with .Not(). On top of "+
    "that sit configurable Mocks (Return/ReturnValues plus CallCount/CalledWith/LastCall inspection), "+
    "type-safe Fn0/Fn1/Fn2 and Spy helpers, and Describe/It/BeforeEach/AfterEach that run over t.Run through "+
    "a small TestReporter interface satisfied by *testing.T. No cgo, no third-party dependencies — assertions "+
    "plug straight into go test.",
  tags:["Expect[T]","Matcher[T]",".Not()","ToBe / ToEqual","ToContain","ToMatch","ToBeCloseTo","mocks","spies","Fn1 / Fn2","Describe / It","stdlib-only"],
  features:[
    "Generic fluent assertions — <code>Expect[T]</code> returns a <code>Matcher[T]</code> that reports through a <code>TestReporter</code> satisfied by <code>*testing.T</code>",
    "Equality &amp; nil — <code>ToBe</code> (shallow ==), <code>ToEqual</code> (deep equal with a diff), <code>ToBeNil</code>, <code>ToBeTrue</code>/<code>ToBeFalse</code>",
    "Containment &amp; shape — <code>ToContain</code> (substring / slice element / map key), <code>ToHaveLen</code>, <code>ToMatch</code> (regexp)",
    "Numbers — <code>ToBeCloseTo</code> with an epsilon plus <code>ToBeGreaterThan</code>/<code>ToBeGreaterThanOrEqual</code>/<code>ToBeLessThan</code>/<code>ToBeLessThanOrEqual</code>",
    "Panics &amp; negation — <code>ToPanic</code>/<code>ToThrow</code> assert a <code>func()</code> panics, and any matcher inverts with <code>.Not()</code>",
    "Mocks — <code>NewMock</code> with <code>Return</code>/<code>ReturnValues</code> and <code>CallCount</code>/<code>Called</code>/<code>CalledWith</code>/<code>LastCall</code>/<code>NthCall</code>/<code>Calls</code>/<code>Reset</code> inspection",
    "Type-safe function doubles — <code>Fn0</code>/<code>Fn1</code>/<code>Fn2</code> mock functions and <code>Spy0</code>/<code>Spy1</code>/<code>Spy2</code> that record while delegating to a real impl",
    "Test organization — <code>Describe</code>/<code>It</code> (alias <code>Test</code>) over <code>t.Run</code> with scoped, composable <code>BeforeEach</code>/<code>AfterEach</code> hooks",
    "Zero dependencies — pure Go standard library, fully integrated with <code>go test</code>, <code>-run</code> filtering and <code>-v</code> output"
  ],
  node_code:
`test('math and mocks', () => {
  expect(2 + 2).toBe(4);
  expect([1, 2, 3]).toContain(2);
  expect('hello world').toMatch(/world/);
  expect(5).not.toBe(6);

  const fn = jest.fn().mockReturnValue('hi');
  expect(fn(7)).toBe('hi');
  expect(fn).toHaveBeenCalledWith(7);
});`,
  go_code:
`import "github.com/malcolmston/jest"

func TestMathAndMocks(t *testing.T) {
	jest.Expect(t, 2+2).ToBe(4)
	jest.Expect(t, []int{1, 2, 3}).ToContain(2)
	jest.Expect(t, "hello world").ToMatch("world")
	jest.Expect(t, 5).Not().ToBe(6)

	fn, mock := jest.Fn1[int, string]("stringify")
	mock.Return("hi")
	jest.Expect(t, fn(7)).ToBe("hi")
	jest.Expect(t, mock.CalledWith(7)).ToBeTrue()
}`,
  integrate:
`func TestCounter(t *testing.T) {
	var counter int

	<span class="tok-c">// Describe/It run over t.Run, so subtests, -run and -v all work.</span>
	jest.Describe(t, "counter", func() {
		<span class="tok-c">// Hooks are scoped to this block and compose across nesting.</span>
		jest.BeforeEach(func() { counter = 0 })

		jest.It(t, "starts at zero", func(t *testing.T) {
			jest.Expect(t, counter).ToBe(0)
		})
		jest.It(t, "increments", func(t *testing.T) {
			counter++
			jest.Expect(t, counter).ToBe(1)
		})
	})

	<span class="tok-c">// Mocks record every call for later inspection.</span>
	m := jest.NewMock("adder")
	m.Return(42)
	m.Call(1, 2)

	jest.Expect(t, m.CallCount()).ToBe(1)
	jest.Expect(t, m.CalledWith(1, 2)).ToBeTrue()
	if last, ok := m.LastCall(); ok {
		jest.Expect(t, last.Args).ToEqual([]any{1, 2})
	}
}`
  },
  {
  id:"jwt", name:"JWT", icon:'<i class="fa-solid fa-key"></i>', accent:"#fb015b",
  pkg:"github.com/malcolmston/jwt", node:"auth0/node-jsonwebtoken",
  repo:"https://github.com/malcolmston/jwt", docs:"https://malcolmston.github.io/jwt/",
  tagline:"Standard-library-only JSON Web Tokens for Go.",
  blurb:"A from-scratch Go implementation of JSON Web Tokens (RFC 7519) on top of the JWS compact "+
    "serialization (RFC 7515), built entirely on the Go standard library — no third-party modules, "+
    "no cgo, no require directives. You sign with either <code>NewWithClaims(...).SignedString</code> or "+
    "the one-shot <code>Sign</code> helper, and verify with <code>Parse</code> / <code>ParseWithClaims</code> "+
    "driven by a <code>Keyfunc</code> for key selection. Every algorithm — HMAC-SHA (HS256/384/512), RSA "+
    "PKCS1v15 (RS*), RSA-PSS (PS*), ECDSA (ES*) and an opt-in unsecured none — implements a common "+
    "SigningMethod interface. RegisteredClaims models the IANA claim set with NumericDate encoding and a "+
    "string-or-array audience, MapClaims handles arbitrary payloads, and parser options give you method "+
    "allow-lists, audience/issuer/subject checks, configurable leeway and an injectable clock. Errors are "+
    "wrapped sentinels you match with errors.Is. The import path is github.com/malcolmston/jwt.",
  tags:["RFC 7519","JWS RFC 7515","HS256/384/512","RS* / PS*","ES256/384/512","Keyfunc","RegisteredClaims","MapClaims","errors.Is","PEM keys","opt-in none","zero deps"],
  features:[
    "One-call signing — <code>Sign(claims, SigningMethodHS256, []byte(secret))</code>, or build explicitly with <code>NewWithClaims(method, claims).SignedString(key)</code>",
    "Verification through <code>Parse</code> (into <code>MapClaims</code>) and <code>ParseWithClaims</code> (into your own <code>Claims</code>), resolving keys via a <code>Keyfunc</code> that sees the header for <code>kid</code> selection",
    "Every algorithm behind one <code>SigningMethod</code> interface — HMAC <code>SigningMethodHS256/384/512</code>, RSA <code>RS*</code>, RSA-PSS <code>PS*</code>, ECDSA <code>ES*</code> (fixed-width r&#124;&#124;s), plus opt-in <code>none</code>",
    "<code>RegisteredClaims</code> covers iss, sub, aud, exp, nbf, iat, jti with <code>NumericDate</code> epoch-seconds and string-or-array <code>ClaimStrings</code> audience",
    "<code>MapClaims</code> for arbitrary payloads, or any custom struct satisfying the one-method <code>Claims</code> interface (embed <code>RegisteredClaims</code>)",
    "Parser options — <code>WithValidMethods</code> (defeat alg-confusion), <code>WithAudience</code>, <code>WithIssuer</code>, <code>WithSubject</code>, <code>WithLeeway</code>, <code>WithExpirationRequired</code>, <code>WithIssuedAt</code>",
    "Deterministic time — <code>WithClock</code> / <code>WithTimeFunc</code> and the <code>ClockFunc</code> adapter make exp/nbf/iat validation reproducible in tests",
    "PEM key helpers — <code>ParseRSAPrivateKeyFromPEM</code>, <code>ParseRSAPublicKeyFromPEM</code>, <code>ParseECPrivateKeyFromPEM</code>, <code>ParseECPublicKeyFromPEM</code> (PKCS#1/SEC1/PKCS#8/PKIX)",
    "Wrapped sentinel errors — <code>ErrTokenExpired</code>, <code>ErrSignatureInvalid</code>, <code>ErrTokenInvalidAudience</code>, <code>ErrTokenNotValidYet</code> and more, all matchable with <code>errors.Is</code>",
    "Double opt-in <code>none</code> — the parser needs <code>WithAllowNone</code> <i>and</i> the <code>UnsafeAllowNoneSignatureType</code> sentinel key, so unsecured tokens never slip through by default",
    "Base64url (no padding) header/payload/signature encoding, tag headers with <code>SetKID</code>, expose the signing input via <code>SigningString</code>",
    "Zero dependencies — pure Go standard library (crypto/*, encoding/*, math/big), no cgo, nothing to audit but the toolchain"
  ],
  node_code:
`const jwt = require("jsonwebtoken");

const token = jwt.sign(
  { sub: "user-42", aud: "api.example.com" },
  "my-hmac-secret",
  { algorithm: "HS256", issuer: "auth.example.com", expiresIn: "1h" }
);

const claims = jwt.verify(token, "my-hmac-secret", {
  algorithms: ["HS256"],       // reject algorithm confusion
  audience: "api.example.com",
  issuer: "auth.example.com",
});
console.log(claims.sub);`,
  go_code:
`import "github.com/malcolmston/jwt"

claims := jwt.RegisteredClaims{
    Issuer:    "auth.example.com",
    Subject:   "user-42",
    Audience:  jwt.ClaimStrings{"api.example.com"},
    ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
}
signed, _ := jwt.Sign(claims, jwt.SigningMethodHS256, []byte("my-hmac-secret"))

var out jwt.RegisteredClaims
tok, _ := jwt.ParseWithClaims(signed, &out,
    func(*jwt.Token) (any, error) { return []byte("my-hmac-secret"), nil },
    jwt.WithValidMethods([]string{"HS256"}), // reject algorithm confusion
    jwt.WithAudience("api.example.com"),
    jwt.WithIssuer("auth.example.com"))
fmt.Println(tok.Valid, out.Subject)`,
  integrate:
`<span class="tok-c">// Sign an RS256 token from a PEM private key, tagging the JOSE</span>
<span class="tok-c">// header with a key id so verifiers can select the right key.</span>
priv, _ := jwt.ParseRSAPrivateKeyFromPEM(privPEM)
signed, _ := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SetKID("2026-07").SignedString(priv)

<span class="tok-c">// Verify with a Keyfunc that picks the key by kid, and pin the</span>
<span class="tok-c">// accepted method so an attacker cannot downgrade the algorithm.</span>
pub, _ := jwt.ParseRSAPublicKeyFromPEM(pubPEM)
tok, err := jwt.Parse(signed, func(t *jwt.Token) (any, error) {
    if t.Header["kid"] != "2026-07" {
        return nil, jwt.ErrTokenUnverifiable
    }
    return pub, nil
}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithLeeway(30*time.Second))

<span class="tok-c">// Every parser failure is a wrapped sentinel — match it with errors.Is.</span>
if errors.Is(err, jwt.ErrTokenExpired) {
    <span class="tok-c">// prompt a refresh...</span>
}
fmt.Println(tok.Valid)`
  },
  {
  id:"nodemailer", name:"Nodemailer", icon:'<i class="fa-solid fa-envelope"></i>', accent:"#34b3a0",
  pkg:"github.com/malcolmston/nodemailer", node:"nodemailer/nodemailer",
  repo:"https://github.com/malcolmston/nodemailer", docs:"https://malcolmston.github.io/nodemailer/",
  tagline:"Nodemailer-style email composition and SMTP sending for Go.",
  blurb:"A from-scratch, standard-library-only Go take on Node.js's Nodemailer: compose rich MIME email with a "+
    "fluent builder and deliver it through pluggable transports, with no cgo and no third-party dependencies. "+
    "A Message assembles From/To/Cc/Bcc/Reply-To, a subject, plain-text and HTML bodies, custom headers, file "+
    "attachments and inline (CID) images; addresses are parsed and validated through net/mail. Build emits "+
    "correct MIME — multipart/alternative for text+html, wrapped in multipart/related for inline resources and "+
    "multipart/mixed for attachments, with quoted-printable bodies, base64 attachments, RFC 2047 encoded words, "+
    "CRLF endings and header folding. A Transport interface carries the bytes: SMTPTransport speaks PLAIN auth "+
    "over STARTTLS or implicit TLS, while MemoryTransport and JSONTransport capture messages for tests. Set the "+
    "Date, Message-ID and boundary explicitly for byte-for-byte deterministic output.",
  tags:["Message builder","net/mail addresses","MIME encoder","multipart/alternative","multipart/related","multipart/mixed","quoted-printable","base64","RFC 2047","SMTP + STARTTLS","MemoryTransport","deterministic"],
  features:[
    "Fluent <code>Message</code> builder — <code>New()</code> plus <code>SetFrom</code>, <code>AddTo</code>, <code>AddCc</code>, <code>AddBcc</code>, <code>AddReplyTo</code>, <code>SetSubject</code>, <code>SetText</code>, <code>SetHTML</code>",
    "Attachments &amp; inline images — <code>Attach</code>, <code>AttachBytes</code> and <code>Embed</code> (cid: resources) over the <code>Attachment</code> struct",
    "Address parsing &amp; validation via <code>ParseAddress</code> / <code>ParseAddressList</code> on <code>net/mail</code>, with deferred errors surfaced by <code>Err</code>",
    "Automatic MIME structure from <code>Build</code> — single part, <code>multipart/alternative</code>, wrapped in <code>multipart/related</code> for inline and <code>multipart/mixed</code> for attachments",
    "Correct encoding — quoted-printable bodies, base64 attachments, RFC 2047 encoded words for non-ASCII subjects/names/filenames, CRLF endings and header folding",
    "Pluggable <code>Transport</code> interface — <code>Send(from, to, raw)</code> decouples composition from delivery",
    "<code>SMTPTransport</code> over <code>net/smtp</code> — PLAIN auth, implicit <code>TLS</code> or <code>STARTTLS</code>, custom <code>TLSConfig</code> and <code>LocalName</code>",
    "Test transports — <code>MemoryTransport</code> captures raw messages (<code>Last</code>) and <code>JSONTransport</code> records a JSON serialisation of each send",
    "<code>Transporter.SendMail</code> mirrors nodemailer's flow, deriving the <code>Envelope</code> and returning <code>Info</code> with <code>MessageID</code>, <code>Envelope</code> and <code>Raw</code>",
    "Deterministic output — <code>SetDate</code>, <code>SetMessageID</code> and <code>SetBoundary</code> make <code>Build</code> byte-for-byte stable for golden tests",
    "Zero dependencies — pure Go standard library, no cgo, nothing to audit but the toolchain"
  ],
  node_code:
`import nodemailer from "nodemailer";

const transporter = nodemailer.createTransport({
  host: "smtp.example.com",
  port: 587,
  secure: false,
  auth: { user: "ada", pass: "secret" },
});

const info = await transporter.sendMail({
  from: "Ada Lovelace <ada@example.com>",
  to: "grace@example.com",
  subject: "Progress report",
  text: "Plain-text version.",
  html: "<p>HTML version</p>",
});
console.log(info.messageId);`,
  go_code:
`import "github.com/malcolmston/nodemailer"

msg := nodemailer.New().
	SetFrom("Ada Lovelace <ada@example.com>").
	AddTo("grace@example.com").
	SetSubject("Progress report").
	SetText("Plain-text version.").
	SetHTML("<p>HTML version</p>")

tr := nodemailer.NewTransporter(&nodemailer.MemoryTransport{})
info, _ := tr.SendMail(msg)
fmt.Println(info.MessageID)`,
  integrate:
`<span class="tok-c">// Compose a rich message: text + HTML alternative, an inline logo</span>
<span class="tok-c">// referenced from the HTML via cid:logo, and a PDF attachment.</span>
msg := nodemailer.New().
	SetFrom("Ada Lovelace <ada@example.com>").
	AddTo("Grace Hopper <grace@example.com>", "team@example.com").
	AddCc("carl@example.com").
	SetSubject("Progress report").
	SetText("Plain-text version of the message.").
	SetHTML(\`<p>See the logo: <img src="cid:logo"></p>\`).
	Embed("logo", "logo.png", "image/png", pngBytes).
	AttachBytes("report.pdf", "application/pdf", pdfBytes)

<span class="tok-c">// Deliver over authenticated SMTP with STARTTLS; SendMail builds the</span>
<span class="tok-c">// MIME bytes, derives the envelope and returns Info.</span>
tr := nodemailer.NewTransporter(&nodemailer.SMTPTransport{
	Host: "smtp.example.com", Port: 587,
	Username: "ada", Password: "secret", STARTTLS: true,
})
info, err := tr.SendMail(msg)
if err != nil {
	log.Fatal(err)
}
log.Printf("sent %s to %v", info.MessageID, info.Envelope.To)

<span class="tok-c">// For golden tests, pin Date/Message-ID/Boundary and capture the raw</span>
<span class="tok-c">// bytes with an in-memory transport instead of a live server.</span>
mem := &nodemailer.MemoryTransport{}
_, _ = nodemailer.NewTransporter(mem).SendMail(
	msg.SetDate(time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)).
		SetMessageID("report-42@example.com").
		SetBoundary("BOUNDARY"))
captured, _ := mem.Last() <span class="tok-c">// captured.Raw holds the full RFC 5322 message</span>`
  },
];

export const FAQS: [string, string][] = [
  ["Are these official ports of the Node.js libraries?",
   "No. They're independent re-implementations that mirror the original APIs and behaviour. They're not affiliated with or endorsed by the upstream Node.js projects. Each is MIT-licensed."],
  ["How compatible are they really?",
   "Where a wire format exists, compatibility is verified against the real clients: Socket.IO interoperates with socket.io-client@4, Passport's JWTs verify against jsonwebtoken, and the WebSocket layer follows RFC 6455. API-level ports (Express, morgan, chalk) mirror the original semantics and are covered by tests."],
  ["What are the dependencies?",
   "Almost none. Express, Passport, Socket.IO and morgan use only the Go standard library. chalk pulls in golang.org/x/term (for raw-mode prompts). There are no replace directives, so a plain go get works."],
  ["Which Go version do I need?",
   "Go 1.23 or newer. CI tests each library on Go 1.23 and 1.24."],
  ["Can I use just one library?",
   "Yes — each is a standalone Go module. Use Express on its own, or wire several together. They compose through the standard http.Handler interface."],
  ["Is it production-ready?",
   "Treat them like any dependency: read the docs, run the tests, and pin your versions. Every package ships tests and CI enforces build + vet + test. The libraries prioritise faithful behaviour and a small, auditable codebase."],
  ["Where are the full API docs?",
   "Each repository publishes a generated API reference on GitHub Pages (linked in every library tab here). The generator is a dependency-free go/doc tool committed in each repo under docs/gen."],
  ["How do I install a specific version?",
   "Every library publishes semver releases automatically — the live version badge on each tab (and the home cards) reads the latest tag straight from GitHub. Pin to one with a version suffix, e.g. go get github.com/malcolmston/express@v0.1.0, or track the moving stable tag with @stable. Plain @latest always resolves to the newest release."]
];

// ---------- highlight ----------
