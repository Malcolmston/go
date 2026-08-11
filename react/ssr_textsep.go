package react

// The hydration text separator: the one place where React's renderToString and
// renderToStaticMarkup produce different bytes for the same tree.
//
// Two adjacent text nodes serialize to one indistinguishable run of characters.
// "ab" could have come from one text node or from two, and a client that has to
// match its own tree against the server's markup cannot guess. So renderToString
// writes an empty comment between them — <div>a<!-- -->b</div> — which the DOM
// parses into a comment node splitting the two text nodes, and hydration walks
// straight through it. renderToStaticMarkup, whose output by definition nobody
// hydrates, writes <div>ab</div>.
//
// The rules below are React's, read out of react-dom-server-legacy's
// pushTextInstance and taken from probes against react-dom@19.2.7:
//
//   - Static markup never separates, and never has to track anything.
//   - An empty text node emits nothing and changes nothing: a text, an empty
//     text and a text still separate exactly once ("<div>a<!-- -->b</div>").
//   - Any other text emits the separator first if the bytes written immediately
//     before it were also text, then the escaped text.
//   - A tag ends a run of text. The flag is a single document-order boolean
//     reset by every start tag, every self-closing void tag and every end tag,
//     not a per-parent one, which is why <div>a<span>b<!-- -->c</span>d</div>
//     separates inside the span but not around it: the <span> and the </span>
//     each end the run. That reset lives in ssrEmitter.host, next to the tag it
//     is reset by.
//
// The flag is not maintained at all in static mode, so nothing depends on the
// resets being reached when the separator can never be written.

// ssrTextSeparator is React's hydration boundary between adjacent text nodes.
// The DOM parses it into a comment node, which is what actually splits the two
// text nodes apart; the bytes themselves are the shortest comment that cannot be
// confused with a directive or a conditional comment.
const ssrTextSeparator = "<!-- -->"

// text emits one text node, escaped, with the hydration separator in front of it
// when React would write one.
func (e *ssrEmitter) text(s string) {
	if e.staticMarkup {
		// No separator, and no bookkeeping either: nothing downstream can read
		// the flag in this mode.
		e.write(ssrEscapeText(s))
		return
	}
	if s == "" {
		// Emitting nothing must also mean deciding nothing: an empty text node
		// between two real ones is not a boundary, and treating it as one would
		// put a separator where React puts none.
		return
	}
	if e.lastText {
		e.write(ssrTextSeparator)
	}
	e.write(ssrEscapeText(s))
	e.lastText = true
}
