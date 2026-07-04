// Library + FAQ content, ported from the original static landing page.
export interface Lib {
  id: string; name: string; icon: string; accent: string; pkg: string; node: string;
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
  }
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
