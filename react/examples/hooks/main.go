// Command hooks is a tour of the hooks that mean something in a static render.
//
//	go run ./examples/hooks
//
// A word on what "state" means with no browser attached: this program renders
// once and exits, so a state update changes what the *build* produces, not what
// a visitor clicks. Hooks still earn their place — deriving a value once,
// loading data before the page is emitted, sharing configuration through a tree
// — they just are not reacting to events. For interactivity, see ../rsc-server.
package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/malcolmston/react"
)

// theme is context: a value any depth of the tree can read without every
// component in between having to pass it along.
var theme = react.CreateContext("light")

// Badge reads context. Nothing was threaded through its props to get here.
var Badge react.Component = func(props react.Props) react.Node {
	mode := react.UseContext(theme)
	return react.H("span", react.Props{"className": "badge badge--" + mode}, mode)
}

// Stats shows UseMemo and UseState working together.
var Stats react.Component = func(props react.Props) react.Node {
	words, _ := props.Get("words").([]string)

	// Recomputed only when the dependency changes. Unlike React, this is a
	// guarantee rather than a hint: the port never discards a memo.
	longest := react.UseMemo(func() string {
		best := ""
		for _, w := range words {
			if len(w) > len(best) {
				best = w
			}
		}
		return best
	}, []any{strings.Join(words, ",")})

	// A setter called from an effect settles before the render finishes: the
	// flush loop keeps rendering until the tree stops changing.
	total, setTotal := react.UseState(0)
	react.UseEffectFn(func() {
		if total != len(words) {
			setTotal(len(words))
		}
	}, []any{len(words), total})

	return react.H("ul", nil,
		react.H("li", nil, "count: ", total),
		react.H("li", nil, "longest: ", longest),
	)
}

var Page react.Component = func(props react.Props) react.Node {
	words := []string{"reconciliation", "hook", "fiber", "suspense"}

	return react.H("main", nil,
		react.H("h1", nil, "hooks"),
		theme.Provider("dark",
			react.H("p", nil, "current theme: ", react.CreateElement(Badge, nil)),
		),
		react.CreateElement(Stats, react.Props{"words": words}),
	)
}

func main() {
	html, err := react.RenderToString(react.CreateElement(Page, nil))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(html)
}
