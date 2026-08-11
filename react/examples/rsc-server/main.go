// Command rsc-server is the interactive example: a Go server rendering React
// Server Components that a real React 19 browser client mounts.
//
//	go run ./examples/rsc-server
//	open http://localhost:8080
//
// The counters are real. Clicking one runs JavaScript, in the browser, with
// React's own runtime and its own hooks — while everything around it was
// rendered in Go and shipped as data.
//
// # What is where
//
//	GET /                 the HTML shell, rendered in Go
//	GET /rsc              the Flight payload — the component tree, as data
//	GET /client/*.js      client components, served as plain ES modules
//	GET /rsc/entry.js     the browser entry that fetches /rsc and mounts it
//
// Nothing here is bundled. React comes from an ES module host, client
// components are loaded by dynamic import, and there is no build step: the only
// requirement is a browser with import maps, which every current one has.
//
// # The one thing worth understanding
//
// A Go component is *evaluated here* and travels as its output. A client
// component is *not* evaluated — Go sends a reference to the module, and the
// browser loads the real thing. That is the whole division: Go decides what the
// tree is, JavaScript makes the interactive parts interactive.
package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/malcolmston/react"
	"github.com/malcolmston/react/rsc"
)

//go:embed client/*.js entry.js
var assets embed.FS

// Counter references the JavaScript module. In a real project `react rsc`
// generates this line by scanning for "use client", so the module id and the
// export name can never drift from the file they name.
var Counter = rsc.Client("/client/Counter.js", "Counter", "/client/Counter.js")

// Row is an ordinary Go server component. It runs here, once, and only its
// output crosses the wire.
var Row react.Component = func(props react.Props) react.Node {
	return react.H("li", nil,
		Counter.El(
			react.Props{"start": props.Get("start"), "label": props.String("label")},
			// Children are rendered in Go and handed to the JavaScript
			// component as props.children.
			react.H("span", react.Props{"className": "hint"},
				"rendered in Go at ", props.String("at"), " — "),
		),
	)
}

// App is the tree sent to the browser. Note that it is not a full document:
// the shell is separate, and this is only what mounts inside it.
var App react.Component = func(props react.Props) react.Node {
	at := props.String("at")

	rows := []react.Node{}
	for i, label := range []string{"apples", "pears", "plums"} {
		rows = append(rows, react.CreateElement(Row, react.Props{
			"key": label, "label": label, "start": i * 10, "at": at,
		}))
	}

	return react.H("div", nil,
		react.H("h1", nil, "Server components, in Go"),
		react.H("p", nil,
			"Everything you see was rendered in Go. The buttons are JavaScript ",
			"client components, so their state lives in your browser."),
		react.H("ul", nil, rows),
	)
}

// shell is the HTML document the payload mounts into. It is rendered in Go too,
// with the same API — there is no separate templating layer.
var shell react.Component = func(props react.Props) react.Node {
	return react.H("html", react.Props{"lang": "en"},
		react.H("head", nil,
			react.H("meta", react.Props{"charSet": "utf-8"}),
			react.H("title", nil, "react — server components"),
			// The import map is what lets client components write
			// `import React from "react"` with no bundler to resolve it.
			react.H("script", react.Props{
				"type": "importmap",
				"dangerouslySetInnerHTML": react.DangerousHTML{HTML: `{"imports":{
					"react": "https://esm.sh/react@19.2.8",
					"react/": "https://esm.sh/react@19.2.8/",
					"react-dom": "https://esm.sh/react-dom@19.2.8",
					"react-dom/": "https://esm.sh/react-dom@19.2.8/",
					"react-server-dom-webpack/client.browser": "https://esm.sh/react-server-dom-webpack@19.2.8/client.browser"
				}}`},
			}),
			react.H("script", react.Props{"type": "module", "src": "/rsc/entry.js"}),
			react.H("style", react.Props{
				"dangerouslySetInnerHTML": react.DangerousHTML{HTML: css},
			}),
		),
		react.H("body", nil,
			react.H("div", react.Props{"id": "root"},
				react.H("p", react.Props{"className": "hint"}, "loading…")),
		),
	)
}

func main() {
	addr := flag.String("addr", ":8080", "address to listen on")
	flag.Parse()

	mux := http.NewServeMux()

	// The shell. Static, and deliberately so: it exists to load the entry.
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		html, err := react.RenderToString(react.CreateElement(shell, nil))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<!doctype html>\n", html)
	})

	// The payload. text/x-component is the content type React's client expects.
	mux.HandleFunc("GET /rsc", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/x-component; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")

		tree := react.CreateElement(App, react.Props{
			"at": time.Now().Format(time.Kitchen),
		})
		if err := rsc.Render(w, tree); err != nil {
			// The header is already written, so this can only be logged. A
			// production server would render into a buffer first.
			log.Printf("rendering the payload: %v", err)
		}
	})

	// Client modules and the entry, served straight from the embedded files.
	// They are plain ES modules, so there is nothing to compile.
	js := http.FileServerFS(assets)
	mux.Handle("GET /client/", jsHandler(js))
	mux.HandleFunc("GET /rsc/entry.js", func(w http.ResponseWriter, r *http.Request) {
		serveJS(w, "entry.js")
	})

	log.Printf("listening on http://localhost%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

// jsHandler forces the JavaScript content type. A module served as text/plain
// is rejected by the browser's module loader, and the resulting error names the
// MIME type rather than the cause.
func jsHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		next.ServeHTTP(w, r)
	})
}

func serveJS(w http.ResponseWriter, name string) {
	data, err := fs.ReadFile(assets, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	_, _ = w.Write(data)
}

const css = `
  body { font: 16px/1.5 system-ui, sans-serif; margin: 3rem auto; max-width: 42rem; padding: 0 1rem; }
  h1 { font-size: 1.5rem; }
  ul { list-style: none; padding: 0; }
  li { margin: .5rem 0; }
  .counter { display: flex; align-items: center; gap: .5rem; }
  .hint { color: #666; font-size: .875rem; }
  button { font: inherit; padding: .25rem .75rem; border-radius: .375rem;
           border: 1px solid #ccc; background: #fff; cursor: pointer; }
  button:hover { background: #f4f4f4; }
`
