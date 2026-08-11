package react

import (
	"errors"
	"strings"
	"testing"
)

// ssrTag builds a host element from raw struct literals, deliberately avoiding
// CreateElement so these tests exercise the serializer rather than the element
// constructor. props may be nil.
func ssrTag(tag string, props Props, children ...Node) *Element {
	p := Props{}
	for k, v := range props {
		p[k] = v
	}
	if len(children) > 0 {
		p[ChildrenKey] = children
	}
	return &Element{Type: tag, Props: p}
}

// ssrFrag groups children with no wrapper element.
func ssrFrag(children ...Node) *Element {
	return &Element{Type: Fragment, Props: Props{ChildrenKey: children}}
}

// ssrComp wraps a component function in an element.
func ssrComp(c Component, props Props) *Element {
	if props == nil {
		props = Props{}
	}
	return &Element{Type: c, Props: props}
}

// ssrRender is the assertion workhorse: render and fail the test on error.
func ssrRender(t *testing.T, node Node) string {
	t.Helper()
	got, err := RenderToString(node)
	if err != nil {
		t.Fatalf("RenderToString: unexpected error: %v", err)
	}
	return got
}

func TestSSRRendersTreesAndText(t *testing.T) {
	var greeting Component = func(p Props) Node {
		return ssrTag("span", nil, "hello, ", p.String("who"))
	}

	cases := []struct {
		name string
		node Node
		want string
	}{
		{"nil tree", nil, ""},
		{"bare text", "hi", "hi"},
		{"number", 42, "42"},
		{"float uses shortest form", 1.5, "1.5"},
		{"empty element", ssrTag("div", nil), "<div></div>"},
		{
			"nested elements",
			ssrTag("div", nil, ssrTag("p", nil, "a"), ssrTag("p", nil, "b")),
			"<div><p>a</p><p>b</p></div>",
		},
		{
			"mixed text and elements",
			ssrTag("p", nil, "count: ", 7, ssrTag("b", nil, "!")),
			"<p>count: 7<b>!</b></p>",
		},
		{
			"nil and bool children vanish",
			ssrTag("ul", nil, nil, false, ssrTag("li", nil), true),
			"<ul><li></li></ul>",
		},
		{
			"slices are flattened",
			ssrTag("ul", nil, []Node{ssrTag("li", nil, "1"), ssrTag("li", nil, "2")}),
			"<ul><li>1</li><li>2</li></ul>",
		},
		{
			"fragments are transparent",
			ssrTag("div", nil, ssrFrag(ssrTag("i", nil), ssrTag("b", nil))),
			"<div><i></i><b></b></div>",
		},
		{
			"a bare fragment at the root emits only its children",
			ssrFrag(ssrTag("i", nil), "x"),
			"<i></i>x",
		},
		{
			"components are transparent",
			ssrTag("div", nil, ssrComp(greeting, Props{"who": "world"})),
			"<div><span>hello, world</span></div>",
		},
		{
			"a component rendering nothing emits nothing",
			ssrTag("div", nil, ssrComp(func(Props) Node { return nil }, nil)),
			"<div></div>",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ssrRender(t, c.node); got != c.want {
				t.Errorf("RenderToString = %q, want %q", got, c.want)
			}
		})
	}
}

func TestSSRTextEscaping(t *testing.T) {
	// Text and attribute values share one escaper, so both are checked here
	// against the same five characters React escapes.
	cases := []struct {
		name string
		node Node
		want string
	}{
		{
			"angle brackets and ampersands in text",
			ssrTag("p", nil, `<script>a && b</script>`),
			`<p>&lt;script&gt;a &amp;&amp; b&lt;/script&gt;</p>`,
		},
		{
			"quotes in text are escaped too",
			ssrTag("p", nil, `he said "it's fine"`),
			`<p>he said &quot;it&#39;s fine&quot;</p>`,
		},
		{
			"attribute values are escaped",
			ssrTag("a", Props{"title": `"x" & <y>`}),
			`<a title="&quot;x&quot; &amp; &lt;y&gt;"></a>`,
		},
		{
			"an already-escaped entity is escaped again, never decoded",
			ssrTag("p", nil, "&amp;"),
			"<p>&amp;amp;</p>",
		},
		{
			"non-ASCII passes through as UTF-8",
			ssrTag("p", nil, "héllo — ✓"),
			"<p>héllo — ✓</p>",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ssrRender(t, c.node); got != c.want {
				t.Errorf("RenderToString = %q, want %q", got, c.want)
			}
		})
	}
}

func TestSSRAttributes(t *testing.T) {
	cases := []struct {
		name string
		node Node
		want string
	}{
		{
			"attributes are sorted for determinism",
			ssrTag("div", Props{"id": "b", "className": "a", "title": "c"}),
			`<div class="a" id="b" title="c"></div>`,
		},
		{"htmlFor", ssrTag("label", Props{"htmlFor": "x"}), `<label for="x"></label>`},
		{"defaultValue", ssrTag("input", Props{"defaultValue": "v"}), `<input value="v"/>`},
		{"defaultChecked", ssrTag("input", Props{"defaultChecked": true}), `<input checked=""/>`},
		{"tabIndex", ssrTag("div", Props{"tabIndex": 0}), `<div tabindex="0"></div>`},
		{"aria passes through", ssrTag("div", Props{"aria-label": "x"}), `<div aria-label="x"></div>`},
		{"data passes through", ssrTag("div", Props{"data-id": "7"}), `<div data-id="7"></div>`},
		{"viewBox keeps its case", ssrTag("svg", Props{"viewBox": "0 0 1 1"}), `<svg viewBox="0 0 1 1"></svg>`},
		{"strokeWidth hyphenates", ssrTag("path", Props{"strokeWidth": 2}), `<path stroke-width="2"></path>`},
		{"numbers format like text", ssrTag("div", Props{"data-n": 1.5}), `<div data-n="1.5"></div>`},

		{"true renders a flag attribute", ssrTag("button", Props{"disabled": true}), `<button disabled=""></button>`},
		{"false omits the attribute", ssrTag("button", Props{"disabled": false}), `<button></button>`},
		{"nil omits the attribute", ssrTag("button", Props{"disabled": nil}), `<button></button>`},

		{
			"event handlers are never emitted",
			ssrTag("button", Props{"onClick": func() {}, "onclick": "alert(1)", "id": "b"}),
			`<button id="b"></button>`,
		},
		{
			"key and ref are never emitted",
			ssrTag("div", Props{KeyProp: "k", RefProp: "r", "id": "d"}),
			`<div id="d"></div>`,
		},
		{
			"children is never emitted as an attribute",
			ssrTag("div", nil, "x"),
			`<div>x</div>`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ssrRender(t, c.node); got != c.want {
				t.Errorf("RenderToString = %q, want %q", got, c.want)
			}
		})
	}
}

func TestSSRVoidElements(t *testing.T) {
	for _, tag := range []string{"area", "base", "br", "col", "embed", "hr",
		"img", "input", "link", "meta", "source", "track", "wbr"} {
		t.Run(tag, func(t *testing.T) {
			got := ssrRender(t, ssrTag(tag, nil))
			if want := "<" + tag + "/>"; got != want {
				t.Errorf("RenderToString = %q, want %q", got, want)
			}
			if strings.Contains(got, "</") {
				t.Errorf("%q contains a closing tag", got)
			}
		})
	}

	t.Run("void element with attributes", func(t *testing.T) {
		got := ssrRender(t, ssrTag("img", Props{"src": "a.png", "alt": "a"}))
		if want := `<img alt="a" src="a.png"/>`; got != want {
			t.Errorf("RenderToString = %q, want %q", got, want)
		}
	})

	t.Run("void element with children is an error", func(t *testing.T) {
		_, err := RenderToString(ssrTag("br", nil, "nope"))
		if err == nil {
			t.Fatal("expected an error for a void element with children")
		}
		if !strings.Contains(err.Error(), "void element") {
			t.Errorf("error = %v, want a void-element diagnostic", err)
		}
	})

	t.Run("void element with raw HTML is an error", func(t *testing.T) {
		_, err := RenderToString(ssrTag("hr", Props{
			ssrDangerousProp: DangerousHTML{HTML: "<b>x</b>"},
		}))
		if err == nil {
			t.Fatal("expected an error for a void element with raw HTML")
		}
	})

	t.Run("case-insensitive tag matching", func(t *testing.T) {
		if got, want := ssrRender(t, ssrTag("BR", nil)), "<BR/>"; got != want {
			t.Errorf("RenderToString = %q, want %q", got, want)
		}
	})
}

func TestSSRDangerouslySetInnerHTML(t *testing.T) {
	t.Run("raw content is not escaped", func(t *testing.T) {
		got := ssrRender(t, ssrTag("div", Props{
			ssrDangerousProp: DangerousHTML{HTML: `<b class="x">&nbsp;</b>`},
		}))
		if want := `<div><b class="x">&nbsp;</b></div>`; got != want {
			t.Errorf("RenderToString = %q, want %q", got, want)
		}
	})

	t.Run("a pointer works too", func(t *testing.T) {
		got := ssrRender(t, ssrTag("div", Props{
			ssrDangerousProp: &DangerousHTML{HTML: "<i/>"},
		}))
		if want := "<div><i/></div>"; got != want {
			t.Errorf("RenderToString = %q, want %q", got, want)
		}
	})

	t.Run("the prop itself is never an attribute", func(t *testing.T) {
		got := ssrRender(t, ssrTag("div", Props{
			ssrDangerousProp: DangerousHTML{HTML: "x"},
		}))
		if strings.Contains(got, ssrDangerousProp) {
			t.Errorf("RenderToString = %q, leaked the prop name", got)
		}
	})

	t.Run("children conflict is an error", func(t *testing.T) {
		_, err := RenderToString(ssrTag("div", Props{
			ssrDangerousProp: DangerousHTML{HTML: "raw"},
		}, "kid"))
		if err == nil {
			t.Fatal("expected an error for raw HTML alongside children")
		}
		if !strings.Contains(err.Error(), "both children") {
			t.Errorf("error = %v, want a conflict diagnostic", err)
		}
	})

	t.Run("a wrong value type is an error", func(t *testing.T) {
		_, err := RenderToString(ssrTag("div", Props{ssrDangerousProp: "<b>x</b>"}))
		if err == nil {
			t.Fatal("expected an error for a non-DangerousHTML value")
		}
		if !strings.Contains(err.Error(), "DangerousHTML") {
			t.Errorf("error = %v, want a type diagnostic", err)
		}
	})

	t.Run("nil value falls back to children", func(t *testing.T) {
		got := ssrRender(t, ssrTag("div", Props{ssrDangerousProp: nil}, "kid"))
		if want := "<div>kid</div>"; got != want {
			t.Errorf("RenderToString = %q, want %q", got, want)
		}
	})
}

func TestSSRStyleProp(t *testing.T) {
	cases := []struct {
		name string
		node Node
		want string
	}{
		{
			"string passthrough",
			ssrTag("div", Props{"style": "color:red"}),
			`<div style="color:red"></div>`,
		},
		{
			"map is kebab-cased and sorted",
			ssrTag("div", Props{"style": map[string]any{"zIndex": 3, "backgroundColor": "red"}}),
			`<div style="background-color:red;z-index:3"></div>`,
		},
		{
			"numbers gain px",
			ssrTag("div", Props{"style": map[string]any{"width": 10}}),
			`<div style="width:10px"></div>`,
		},
		{
			"map[string]string works",
			ssrTag("div", Props{"style": map[string]string{"color": "blue"}}),
			`<div style="color:blue"></div>`,
		},
		{
			"an empty map omits the attribute",
			ssrTag("div", Props{"style": map[string]any{}}),
			`<div></div>`,
		},
		{
			"style values are attribute-escaped",
			ssrTag("div", Props{"style": `font-family: "Fancy"`}),
			`<div style="font-family: &quot;Fancy&quot;"></div>`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ssrRender(t, c.node); got != c.want {
				t.Errorf("RenderToString = %q, want %q", got, c.want)
			}
		})
	}

	t.Run("an unsupported style type is an error", func(t *testing.T) {
		_, err := RenderToString(ssrTag("div", Props{"style": 42}))
		if err == nil {
			t.Fatal("expected an error for a numeric style prop")
		}
		if !strings.Contains(err.Error(), "style prop") {
			t.Errorf("error = %v, want a style diagnostic", err)
		}
	})
}

func TestSSRPanicBecomesError(t *testing.T) {
	cases := []struct {
		name string
		node Node
		want string
	}{
		{
			"a panicking component",
			ssrTag("div", nil, ssrComp(func(Props) Node { panic("boom") }, nil)),
			"boom",
		},
		{
			"an unrenderable child type",
			ssrTag("div", nil, struct{ A int }{1}),
			"cannot render value of type",
		},
		{
			"an unrenderable element type",
			&Element{Type: 3.14, Props: Props{}},
			"not renderable",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := RenderToString(c.node)
			if err == nil {
				t.Fatalf("expected an error, got output %q", got)
			}
			if got != "" {
				t.Errorf("output = %q, want empty on error", got)
			}
			if !strings.Contains(err.Error(), "panic while rendering") {
				t.Errorf("error = %v, want a panic-context prefix", err)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want it to mention %q", err, c.want)
			}
		})
	}
}

func TestSSRPanicErrorIsUnwrappable(t *testing.T) {
	sentinel := errors.New("sentinel")
	_, err := RenderToString(ssrComp(func(Props) Node { panic(sentinel) }, nil))
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("errors.Is(err, sentinel) = false; err = %v", err)
	}
}

// ssrFailingWriter fails after letting n bytes through, so a test can prove that
// an I/O failure is reported as itself rather than swallowed or misreported as a
// tree error.
type ssrFailingWriter struct {
	n   int
	err error
}

func (w *ssrFailingWriter) Write(p []byte) (int, error) {
	if w.n <= 0 {
		return 0, w.err
	}
	w.n -= len(p)
	return len(p), nil
}

func TestSSRRenderToWriterReportsIOErrors(t *testing.T) {
	boom := errors.New("disk on fire")
	w := &ssrFailingWriter{n: 2, err: boom}

	err := RenderToWriter(w, ssrTag("div", nil, ssrTag("span", nil, "hello")))
	if err == nil {
		t.Fatal("expected the writer's error to surface")
	}
	if !errors.Is(err, boom) {
		t.Errorf("errors.Is(err, boom) = false; err = %v", err)
	}
}

func TestSSRRenderToWriterStreamsIncrementally(t *testing.T) {
	// Each write is recorded separately: proving there is more than one shows
	// markup leaves the renderer before the tree is finished.
	var chunks []string
	w := ssrWriterFunc(func(p []byte) (int, error) {
		chunks = append(chunks, string(p))
		return len(p), nil
	})

	if err := RenderToWriter(w, ssrTag("div", nil, ssrTag("b", nil, "x"))); err != nil {
		t.Fatalf("RenderToWriter: %v", err)
	}
	if len(chunks) < 2 {
		t.Errorf("got %d writes, want several (output should stream)", len(chunks))
	}
	if got, want := strings.Join(chunks, ""), "<div><b>x</b></div>"; got != want {
		t.Errorf("joined output = %q, want %q", got, want)
	}
}

// ssrWriterFunc adapts a function to io.Writer.
type ssrWriterFunc func(p []byte) (int, error)

func (f ssrWriterFunc) Write(p []byte) (int, error) { return f(p) }

func TestSSRStaticMarkupMatchesRenderToString(t *testing.T) {
	// The port emits no hydration markers, so the two entry points are
	// documented as identical. This test is the executable form of that promise:
	// if markers are ever added to RenderToString, it fails and forces the
	// documentation to be updated with them.
	node := ssrTag("div", Props{"id": "x"}, ssrTag("span", nil, "hi"), 3)

	a, err := RenderToString(node)
	if err != nil {
		t.Fatalf("RenderToString: %v", err)
	}
	b, err := RenderToStaticMarkup(node)
	if err != nil {
		t.Fatalf("RenderToStaticMarkup: %v", err)
	}
	if a != b {
		t.Errorf("RenderToString = %q, RenderToStaticMarkup = %q; want identical", a, b)
	}
	if want := `<div id="x"><span>hi</span>3</div>`; a != want {
		t.Errorf("output = %q, want %q", a, want)
	}
}

func TestSSRMustRenderToString(t *testing.T) {
	if got, want := MustRenderToString(ssrTag("i", nil, "x")), "<i>x</i>"; got != want {
		t.Errorf("MustRenderToString = %q, want %q", got, want)
	}

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected MustRenderToString to panic on a bad tree")
		}
		if _, ok := r.(error); !ok {
			t.Errorf("panic value = %T, want an error", r)
		}
	}()
	MustRenderToString(ssrTag("br", nil, "content"))
}

func TestSSRUnmountsTheTreeBeforeReturning(t *testing.T) {
	// Rendering to a string is a complete Root lifecycle, so effect cleanups
	// must have run by the time the string is handed back — otherwise every
	// server-rendered page would leak whatever its effects acquired.
	cleaned := false
	var c Component = func(Props) Node {
		h, first := nextHook("effect")
		if first {
			enqueueEffect(h, func() func() {
				return func() { cleaned = true }
			}, false)
		}
		return ssrTag("div", nil, "x")
	}

	if got, want := ssrRender(t, ssrComp(c, nil)), "<div>x</div>"; got != want {
		t.Errorf("RenderToString = %q, want %q", got, want)
	}
	if !cleaned {
		t.Error("effect cleanup did not run; the Root was not unmounted")
	}
}

func TestSSRDeepTree(t *testing.T) {
	// A component-and-fragment sandwich around host elements, to confirm that
	// transparency composes at depth rather than only one level down.
	var row Component = func(p Props) Node {
		return ssrFrag(
			ssrTag("td", nil, p.String("k")),
			ssrTag("td", nil, p.Get("v")),
		)
	}
	var table Component = func(Props) Node {
		return ssrTag("table", Props{"className": "t"},
			ssrTag("tbody", nil,
				ssrTag("tr", nil, ssrComp(row, Props{"k": "a", "v": 1})),
				ssrTag("tr", nil, ssrComp(row, Props{"k": "b", "v": 2})),
			),
		)
	}

	want := `<table class="t"><tbody>` +
		`<tr><td>a</td><td>1</td></tr>` +
		`<tr><td>b</td><td>2</td></tr>` +
		`</tbody></table>`
	if got := ssrRender(t, ssrComp(table, nil)); got != want {
		t.Errorf("RenderToString = %q, want %q", got, want)
	}
}
