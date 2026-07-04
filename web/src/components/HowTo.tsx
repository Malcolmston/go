import { CodeBlock, hi } from 'go-ui';
import { SecH } from './SecH';

// HowTo is the "how to use this" getting-started tab.
export function HowTo() {
  return (
    <section className="view active" id="view-howto">
      <SecH h="h2">How to use this</SecH>
      <p className="muted">Every library is an ordinary Go module. Install what you need with <code>go get</code>, import it, and go.</p>

      <SecH>1 · Install</SecH>
      <CodeBlock lang="shell" html={hi(`# pick any subset — they're independent modules
go get github.com/malcolmston/express
go get github.com/malcolmston/passport
go get github.com/malcolmston/socketio
go get github.com/malcolmston/chalk
go get github.com/malcolmston/morgan`)} />

      <SecH>2 · A minimal Express server</SecH>
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
    log.Fatal(app.Listen(":3000"))
}`)} />

      <SecH>3 · The full stack together</SecH>
      <p className="muted">Express serves the API, morgan logs every request, and Socket.IO handles realtime — all on one <code>net/http</code> server. This is the runnable <code>examples/integration</code> in the repo.</p>
      <CodeBlock lang="go" html={hi(`package main

import (
    "net/http"
    "github.com/malcolmston/express"
    "github.com/malcolmston/morgan"
    socketio "github.com/malcolmston/socketio"
)

func main() {
    app := express.New()
    app.Get("/api/hello", func(req *express.Request, res *express.Response, next express.Next) {
        res.JSON(map[string]any{"msg": "hi"})
    })

    io := socketio.New()
    io.OnConnection(func(s *socketio.Socket) {
        s.On("chat", func(args []any) []any { io.Emit("chat", args...); return nil })
    })

    logged := morgan.New(io.Handler(app), morgan.Dev, morgan.Config{})
    http.ListenAndServe(":3000", logged)
}`)} />

      <div className="note">Because every library is an <code>http.Handler</code> (or produces one), they compose the same way any Go middleware does — no framework lock-in.</div>

      <SecH>4 · Where to go next</SecH>
      <ul className="clean">
        <li>Open a library tab above for its full API, feature list and a Node-vs-Go comparison.</li>
        <li>Each repo ships a live API reference on GitHub Pages (linked in every library tab).</li>
        <li>Runnable <code>examples/</code> live in each repository.</li>
      </ul>
    </section>
  );
}
