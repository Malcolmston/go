// Command hello is the smallest possible react program: build a tree, render
// it, print the HTML.
//
//	go run ./examples/hello
package main

import (
	"fmt"
	"log"

	"github.com/malcolmston/react"
)

// Greeting is a component. Note the type: react.Component, not a bare
// func(react.Props) react.Node. The runtime dispatches on the named type, so an
// unconverted function renders as nothing at all — no error, just a gap.
var Greeting react.Component = func(props react.Props) react.Node {
	return react.H("p", react.Props{"className": "greeting"},
		"Hello, ", props.String("name"), "!")
}

// Page composes components the way JSX would nest them. Without JSX, nesting is
// function calls and children are variadic arguments.
var Page react.Component = func(props react.Props) react.Node {
	names := []react.Node{}
	for _, name := range []string{"Ada", "Grace", "Katherine"} {
		// A keyed child keeps its identity across reorders. In a static render
		// it changes nothing; the habit is what matters.
		names = append(names, react.CreateElement(Greeting,
			react.Props{"key": name, "name": name}))
	}

	return react.H("html", react.Props{"lang": "en"},
		react.H("head", nil,
			react.H("meta", react.Props{"charSet": "utf-8"}),
			react.H("title", nil, "hello"),
		),
		react.H("body", nil,
			react.H("h1", nil, "react, in Go"),
			react.H("section", nil, names),
		),
	)
}

func main() {
	html, err := react.RenderToString(react.CreateElement(Page, nil))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("<!doctype html>")
	fmt.Println(html)
}
