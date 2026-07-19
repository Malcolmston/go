import { CodeBlock, hi } from 'go-ui';
import { SecH } from './SecH';

// HowTo is the "how to use this" getting-started guide. It walks through
// install, then one worked example per family category (web, real-time, auth,
// math/data, stores, terminal, HTTP/HTML, testing/util) so a reader can get
// running with any part of the family without leaving the page. Each snippet is
// idiomatic; open a library tab for its full API reference.
export function HowTo() {
  return (
    <section className="view active" id="view-howto">
      <SecH h="h2">How to use this</SecH>
      <p className="muted">
        Every library is an ordinary, dependency-free Go module — install what you need with <code>go get</code>,
        import it, and go. Below is one worked example per category; open any library tab for its full API reference.
      </p>
      <div className="onthispage">
        <a href="#howto-install">Install</a>
        <a href="#howto-web">Web</a>
        <a href="#howto-realtime">Real-time stack</a>
        <a href="#howto-auth">Auth</a>
        <a href="#howto-math">Math &amp; data</a>
        <a href="#howto-stores">Data stores</a>
        <a href="#howto-cli">Terminal</a>
        <a href="#howto-http">HTTP &amp; HTML</a>
        <a href="#howto-util">Testing &amp; utils</a>
        <a href="#howto-next">Next</a>
      </div>

      <SecH id="howto-install">1 · Install</SecH>
      <p className="muted">Pick any subset — they're independent modules, all under <code>github.com/malcolmston/&lt;name&gt;</code>.</p>
      <CodeBlock lang="shell" html={hi(`go get github.com/malcolmston/express      # web framework
go get github.com/malcolmston/socketio     # real-time
go get github.com/malcolmston/passport     # auth
go get github.com/malcolmston/algebra      # symbolic + numeric math
go get github.com/malcolmston/sqlite       # embedded SQL
go get github.com/malcolmston/chalk        # terminal styling
# …33 libraries in total`)} />

      <SecH id="howto-web">2 · A minimal web server (express)</SecH>
      <CodeBlock lang="main.go" html={hi(`package main

import (
    "log"
    "github.com/malcolmston/express"
)

func main() {
    app := express.New()
    app.Get("/", func(req *express.Request, res *express.Response, next express.Next) {
        res.Send("Hello World")
    })
    app.Get("/users/:id", func(req *express.Request, res *express.Response, next express.Next) {
        res.JSON(map[string]any{"id": req.Param("id")})
    })
    log.Fatal(app.Listen(":3000"))
}`)} />

      <SecH id="howto-realtime">3 · The full real-time stack</SecH>
      <p className="muted">Express serves the API, morgan logs every request, and Socket.IO handles realtime — all on one <code>net/http</code> server. This is the runnable <code>examples/integration</code> in the repo.</p>
      <CodeBlock lang="go" html={hi(`app := express.New()
app.Get("/api/hello", func(req *express.Request, res *express.Response, next express.Next) {
    res.JSON(map[string]any{"msg": "hi"})
})

io := socketio.New()
io.OnConnection(func(s *socketio.Socket) {
    s.On("chat", func(args []any) []any { io.Emit("chat", args...); return nil })
})

logged := morgan.New(io.Handler(app), morgan.Dev, morgan.Config{})
http.ListenAndServe(":3000", logged)`)} />
      <div className="note">Because every library is an <code>http.Handler</code> (or produces one), they compose the same way any Go middleware does — no framework lock-in.</div>

      <SecH id="howto-auth">4 · Authentication (passport + jwt)</SecH>
      <p className="muted">Passport's strategies plug into Express; issue and verify tokens with the jwt library.</p>
      <CodeBlock lang="go" html={hi(`// Sign & verify a JSON Web Token
tok, _ := jwt.Sign(jwt.Claims{"sub": "alice"}, "secret", jwt.HS256)
claims, err := jwt.Verify(tok, "secret")

// Protect a route with a passport strategy
p := passport.New()
p.Use(bearer.New(func(token string) (any, error) {
    return lookupUser(token)
}))
app.Get("/me", p.Authenticate("bearer"), func(req *express.Request, res *express.Response, next express.Next) {
    res.JSON(req.User())
})`)} />

      <SecH id="howto-math">5 · Math &amp; data (algebra, numpy, pandas)</SecH>
      <p className="muted">algebra is 100+ stdlib-only subpackages spanning number theory, linear algebra, calculus, statistics and much more.</p>
      <CodeBlock lang="go" html={hi(`import (
    "github.com/malcolmston/algebra/ntheory"
    "github.com/malcolmston/algebra/matrix"
    "github.com/malcolmston/numpy"
)

func main() {
    fmt.Println(ntheory.Binomial(52, 5))          // 2598960
    fmt.Println(ntheory.Factorial(10))            // 3628800

    a := matrix.New([][]float64{{1, 2}, {3, 4}})
    det, _ := matrix.Det(a)                       // -2

    v := numpy.Arange(0, 10, 1)
    fmt.Println(numpy.Mean(v))                    // 4.5
}`)} />

      <SecH id="howto-stores">6 · Data stores (sqlite + redis)</SecH>
      <CodeBlock lang="go" html={hi(`db, _ := sqlite.Open("app.db")
db.Exec("CREATE TABLE users (id INTEGER, name TEXT)")
db.Exec("INSERT INTO users VALUES (?, ?)", 1, "alice")
rows, _ := db.Query("SELECT name FROM users WHERE id = ?", 1)

cache := redis.New()
cache.Set("greeting", "hi", 0)
val, _ := cache.Get("greeting")   // "hi"`)} />

      <SecH id="howto-cli">7 · Terminal styling (chalk)</SecH>
      <CodeBlock lang="go" html={hi(`c := chalk.New()
fmt.Println(c.Red().Bold().Sprint("error:"), "something broke")
fmt.Println(c.Green("ok"), c.Dim("(cached)"))
fmt.Println(c.Hex("#ffa657").Underline().Sprint("custom color"))`)} />

      <SecH id="howto-http">8 · HTTP client &amp; HTML (axios + cheerio)</SecH>
      <CodeBlock lang="go" html={hi(`client := axios.Create(axios.Config{BaseURL: "https://api.example.com"})
resp, _ := client.Get("/users", nil)

doc, _ := cheerio.LoadHTML(resp.Text())
doc.Find("h1").Each(func(i int, el *cheerio.Selection) {
    fmt.Println(el.Text())
})`)} />

      <SecH id="howto-util">9 · Testing &amp; utilities (jest + lodash)</SecH>
      <CodeBlock lang="go" html={hi(`// Jest-style expectations in a normal Go test
func TestSum(t *testing.T) {
    jest.Expect(t, sum(2, 3)).ToBe(5)
    jest.Expect(t, []int{1, 2, 3}).ToContain(2)
}

// lodash helpers, generic and type-safe
evens := lodash.Filter([]int{1, 2, 3, 4}, func(n int) bool { return n%2 == 0 })
groups := lodash.GroupBy(words, func(w string) int { return len(w) })`)} />

      <SecH id="howto-next">10 · Where to go next</SecH>
      <ul className="clean">
        <li>Open a library tab above for its <b>full inline API reference</b>, feature list and a Node-vs-Go comparison.</li>
        <li>Each library tab also shows its <b>live upstream-parity score</b> and the pipeline that produces it.</li>
        <li>Every repo ships the same API reference on its own GitHub Pages site (linked in each tab), and runnable <code>examples/</code> live in each repository.</li>
      </ul>
    </section>
  );
}
