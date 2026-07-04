# malcolmston/go

**The Node.js ecosystem, reimagined in Go.**

A unified home for a family of Go libraries that recreate the most-loved
building blocks of the Node.js ecosystem — with the same ergonomics, on top of
Go's standard library.

🌐 **Unified site & docs:** [`index.html`](index.html) → published to GitHub
Pages (Home, per-library tabs, How-to, FAQ, AI, About, with Node-vs-Go code
comparisons).

## The libraries

Each library is an independent Go module. This repo vendors them as **git
submodules** and ties them together with a **Go workspace** (`go.work`) so
cross-module code builds against the local checkouts.

| Library | Ports | Module | Docs |
| ------- | ----- | ------ | ---- |
| [express](express) | expressjs/express | `github.com/malcolmston/express` | [pages](https://malcolmston.github.io/express/) |
| [passport](passport) | jaredhanson/passport | `github.com/malcolmston/passport` | [pages](https://malcolmston.github.io/passport/) |
| [socket.io](socket.io) | socketio/socket.io | `github.com/malcolmston/socketio` | [pages](https://malcolmston.github.io/socket.io/) |
| [chalk](chalk) | chalk/chalk (+inquirer, figlet) | `github.com/malcolmston/chalk` | [pages](https://malcolmston.github.io/chalk/) |
| [morgan](morgan) | expressjs/morgan | `github.com/malcolmston/morgan` | [pages](https://malcolmston.github.io/morgan/) |

## Clone (with submodules)

```sh
git clone --recurse-submodules https://github.com/malcolmston/go
# already cloned?
git submodule update --init --recursive
```

## Use a single library

The libraries are independent — you do **not** need this repo to use them:

```sh
go get github.com/malcolmston/express
go get github.com/malcolmston/passport
go get github.com/malcolmston/socketio
go get github.com/malcolmston/chalk
go get github.com/malcolmston/morgan
```

## Develop across libraries (workspace)

`go.work` lists every submodule plus the example, so code in this repo resolves
imports to the local checkouts — no publishing required:

```sh
go run ./examples/integration   # express + socket.io + morgan on one server
go build ./examples/integration
```

See [`examples/integration`](examples/integration) for a runnable server that
composes Express routing, a Socket.IO realtime endpoint, and morgan logging
through the standard `http.Handler` interface.

## Pipelines

- **[CI](.github/workflows/ci.yml)** — checks out every submodule, builds the
  cross-module workspace example, and runs each library's own test suite from
  one place.
- **[Pages](.github/workflows/pages.yml)** — publishes the unified site.

## License

MIT. Each library is an independent re-implementation and is **not** affiliated
with or endorsed by the original Node.js projects.
