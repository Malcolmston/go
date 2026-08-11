package react

// React's DOM server does not serialize every element with one generic prop
// loop. input, button, form and option each have a hand-written serializer that
// pulls a fixed set of props out of the loop and appends them after everything
// else, in a fixed order of its own. The result is observable in the bytes:
// <input id="q" name="q" required=""/> is what a generic sorted pass produces,
// while React writes <input id="q" required="" type="text" name="q"/> because
// name is deferred to the end.
//
// This file reproduces only that deferral. It is a reordering layer over the
// name slice ssrEmitter.attributes has already sorted: the props React defers
// are lifted out of their sorted position and re-appended in React's order.
// Everything else -- how a prop name maps to an attribute name, how its value
// is serialized, whether it is emitted at all -- stays in ssr_attr.go.
//
// Sources (react-dom 19.2, renderToStaticMarkup / renderToString path in
// react-dom-server-legacy.node.development.js):
//
//	pushInput  -- defers name, formAction, formEncType, formMethod, formTarget,
//	              then checked (or defaultChecked), then value (or defaultValue)
//	pushButton -- defers name, formAction, formEncType, formMethod, formTarget
//	pushForm   -- defers action, encType, method, target
//	option     -- defers selected only; value falls through and stays in place
//
// The keys below are RAW prop names, not attribute names: the port aliases
// defaultValue to value and defaultChecked to checked inside ssr_attr.go, and
// that renaming keeps happening untouched.
var ssrTrailingProps = map[string][]string{
	"input": {
		"name",
		"formAction",
		"formEncType",
		"formMethod",
		"formTarget",
		"checked",
		"defaultChecked",
		"value",
		"defaultValue",
	},
	"button": {
		"name",
		"formAction",
		"formEncType",
		"formMethod",
		"formTarget",
	},
	"form": {
		"action",
		"encType",
		"method",
		"target",
	},
	"option": {
		"selected",
	},
}

// ssrControlledPairs maps a controlled prop to the uncontrolled prop it wins
// over. React reads both out of the loop and then writes only one attribute:
// checked if present, otherwise defaultChecked. Since the port renames the
// default form to the same attribute name, keeping both in the list would emit
// the attribute twice, so the loser is dropped here.
var ssrControlledPairs = map[string]string{
	"checked": "defaultChecked",
	"value":   "defaultValue",
}

// ssrOrderHasProp reports whether props carries prop with a value React would
// act on. React tests `null != propValue` before its switch, so a nil value is
// invisible to the serializer: <input value={null} defaultValue="x"/> writes
// value="x". A false checked, by contrast, is present -- it wins over
// defaultChecked and then renders as no attribute at all.
func ssrOrderHasProp(props Props, prop string) bool {
	value, ok := props[prop]
	return ok && value != nil
}

// ssrAttributeOrder returns names with the props tag's React serializer defers
// moved to the end, in React's order. Props the element does not have are
// skipped, and the relative order of every other prop is preserved, so a tag
// with no special serializer comes back unchanged.
func ssrAttributeOrder(tag string, props Props, names []string) []string {
	trailing, ok := ssrTrailingProps[tag]
	if !ok || len(names) == 0 {
		return names
	}

	// Which of the deferred props this element actually carries, and which of
	// them lose a controlled/uncontrolled fight and must not be emitted.
	deferred := make(map[string]bool, len(trailing))
	dropped := make(map[string]bool, 2)
	for _, prop := range trailing {
		if !ssrOrderHasProp(props, prop) {
			continue
		}
		deferred[prop] = true
		if loser, paired := ssrControlledPairs[prop]; paired {
			if ssrOrderHasProp(props, loser) {
				dropped[loser] = true
			}
		}
	}
	if len(deferred) == 0 {
		return names
	}

	out := make([]string, 0, len(names))
	for _, name := range names {
		if deferred[name] || dropped[name] {
			continue
		}
		out = append(out, name)
	}
	for _, prop := range trailing {
		if deferred[prop] && !dropped[prop] {
			out = append(out, prop)
		}
	}
	return out
}
