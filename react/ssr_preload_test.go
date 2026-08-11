package react

import (
	"strings"
	"testing"
)

// Every expectation in this file is a byte-for-byte copy of a probe render made
// with the pinned oracle, react-dom 19.2.7, renderToStaticMarkup. The probe
// output for the img is stripped off; only the hoisted link is asserted here,
// because where the link lands in the document is the hoistable buffer's
// concern and not this file's.
func TestSSRImagePreloadFor(t *testing.T) {
	tests := []struct {
		name       string
		props      Props
		suppressed bool
		want       string // "" means React emits no preload
		wantKey    string
		wantHigh   bool
	}{{
		name:    "src alone",
		props:   Props{"alt": "a", "src": "/a.png"},
		want:    `<link rel="preload" as="image" href="/a.png"/>`,
		wantKey: "/a.png",
	}, {
		name:    "srcSet replaces href",
		props:   Props{"src": "/a.png", "srcSet": "/a.png 1x"},
		want:    `<link rel="preload" as="image" imageSrcSet="/a.png 1x"/>`,
		wantKey: "/a.png 1x\n",
	}, {
		name:    "srcSet without src",
		props:   Props{"srcSet": "/a.png 1x"},
		want:    `<link rel="preload" as="image" imageSrcSet="/a.png 1x"/>`,
		wantKey: "/a.png 1x\n",
	}, {
		name:    "srcSet and sizes",
		props:   Props{"src": "/a.png", "srcSet": "/a.png 1x", "sizes": "100vw"},
		want:    `<link rel="preload" as="image" imageSrcSet="/a.png 1x" imageSizes="100vw"/>`,
		wantKey: "/a.png 1x\n100vw",
	}, {
		// sizes with no srcSet keeps href and still carries imageSizes.
		name:    "sizes without srcSet",
		props:   Props{"src": "/a.png", "sizes": "10px"},
		want:    `<link rel="preload" as="image" href="/a.png" imageSizes="10px"/>`,
		wantKey: "/a.png",
	}, {
		// An empty srcSet is falsy in React's ternary, so href comes back while
		// imageSrcSet is still written as an empty attribute.
		name:    "empty srcSet keeps href",
		props:   Props{"src": "/a.png", "srcSet": ""},
		want:    `<link rel="preload" as="image" href="/a.png" imageSrcSet=""/>`,
		wantKey: "/a.png",
	}, {
		name:    "empty sizes",
		props:   Props{"src": "/a.png", "sizes": ""},
		want:    `<link rel="preload" as="image" href="/a.png" imageSizes=""/>`,
		wantKey: "/a.png",
	}, {
		// A non-string sizes renders on the img but never reaches the link.
		name:    "numeric sizes ignored",
		props:   Props{"src": "/a.png", "sizes": 5},
		want:    `<link rel="preload" as="image" href="/a.png"/>`,
		wantKey: "/a.png",
	}, {
		name:    "crossOrigin anonymous collapses to empty",
		props:   Props{"src": "/a.png", "crossOrigin": "anonymous"},
		want:    `<link rel="preload" as="image" href="/a.png" crossorigin=""/>`,
		wantKey: "/a.png",
	}, {
		name:    "crossOrigin use-credentials survives",
		props:   Props{"src": "/a.png", "crossOrigin": "use-credentials"},
		want:    `<link rel="preload" as="image" href="/a.png" crossorigin="use-credentials"/>`,
		wantKey: "/a.png",
	}, {
		name:    "non-string crossOrigin dropped",
		props:   Props{"src": "/a.png", "crossOrigin": 5},
		want:    `<link rel="preload" as="image" href="/a.png"/>`,
		wantKey: "/a.png",
	}, {
		name:    "referrerPolicy keeps camelCase",
		props:   Props{"src": "/a.png", "referrerPolicy": "no-referrer"},
		want:    `<link rel="preload" as="image" href="/a.png" referrerPolicy="no-referrer"/>`,
		wantKey: "/a.png",
	}, {
		name:     "fetchPriority high",
		props:    Props{"src": "/a.png", "fetchPriority": "high"},
		want:     `<link rel="preload" as="image" href="/a.png" fetchPriority="high"/>`,
		wantKey:  "/a.png",
		wantHigh: true,
	}, {
		name:    "fetchPriority auto",
		props:   Props{"src": "/a.png", "fetchPriority": "auto"},
		want:    `<link rel="preload" as="image" href="/a.png" fetchPriority="auto"/>`,
		wantKey: "/a.png",
	}, {
		name: "every passthrough prop, in React's order",
		props: Props{
			"src":            "/a.png",
			"crossOrigin":    "anonymous",
			"integrity":      "sha",
			"type":           "image/png",
			"fetchPriority":  "high",
			"referrerPolicy": "no-referrer",
			"nonce":          "n", // header-only in React; never on the link
		},
		want: `<link rel="preload" as="image" href="/a.png" crossorigin="" ` +
			`integrity="sha" type="image/png" fetchPriority="high" referrerPolicy="no-referrer"/>`,
		wantKey:  "/a.png",
		wantHigh: true,
	}, {
		name:    "numeric integrity is stringified",
		props:   Props{"src": "/a.png", "integrity": 5},
		want:    `<link rel="preload" as="image" href="/a.png" integrity="5"/>`,
		wantKey: "/a.png",
	}, {
		name:    "href value is escaped",
		props:   Props{"src": `/a"b&c.png`},
		want:    `<link rel="preload" as="image" href="/a&quot;b&amp;c.png"/>`,
		wantKey: `/a"b&c.png`,
	}, {
		// media is not one of the props React copies onto the link.
		name:    "media is not copied",
		props:   Props{"src": "/a.png", "media": "print"},
		want:    `<link rel="preload" as="image" href="/a.png"/>`,
		wantKey: "/a.png",
	}, {
		name:  "loading lazy suppresses",
		props: Props{"src": "/a.png", "loading": "lazy"},
	}, {
		name:    "loading eager does not suppress",
		props:   Props{"src": "/a.png", "loading": "eager"},
		want:    `<link rel="preload" as="image" href="/a.png"/>`,
		wantKey: "/a.png",
	}, {
		name:    "non-string loading does not suppress",
		props:   Props{"src": "/a.png", "loading": 5},
		want:    `<link rel="preload" as="image" href="/a.png"/>`,
		wantKey: "/a.png",
	}, {
		name:  "fetchPriority low suppresses",
		props: Props{"src": "/a.png", "fetchPriority": "low"},
	}, {
		name:  "fetchPriority low suppresses a srcSet too",
		props: Props{"srcSet": "/x 1x", "fetchPriority": "low"},
	}, {
		name:  "no src and no srcSet",
		props: Props{"alt": "a"},
	}, {
		name:  "nil src",
		props: Props{"alt": "a", "src": nil},
	}, {
		name:  "empty src",
		props: Props{"src": ""},
	}, {
		name:  "non-string src",
		props: Props{"src": 5},
	}, {
		name:  "non-string src beside a usable srcSet",
		props: Props{"src": 5, "srcSet": "/x 1x"},
	}, {
		name:  "non-string srcSet",
		props: Props{"src": "/a.png", "srcSet": 5},
	}, {
		name:  "data URI src",
		props: Props{"src": "data:image/png;base64,AAA"},
	}, {
		name:  "data URI src, upper case scheme",
		props: Props{"src": "DATA:image/png;base64,AAA"},
	}, {
		name:  "data URI srcSet",
		props: Props{"srcSet": "data:image/png;base64,A 1x"},
	}, {
		// Only "data:" exactly; a scheme that merely starts with it preloads.
		name:    "datax scheme is not a data URI",
		props:   Props{"src": "datax:foo"},
		want:    `<link rel="preload" as="image" href="datax:foo"/>`,
		wantKey: "datax:foo",
	}, {
		name:       "suppressed by picture or noscript scope",
		props:      Props{"src": "/a.png"},
		suppressed: true,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ssrImagePreloadFor(tt.props, tt.suppressed)
			if tt.want == "" {
				if ok {
					t.Fatalf("got preload %q, want none", got.chunk)
				}
				return
			}
			if !ok {
				t.Fatalf("got no preload, want %q", tt.want)
			}
			if got.chunk != tt.want {
				t.Errorf("chunk:\n got %s\nwant %s", got.chunk, tt.want)
			}
			if got.key != tt.wantKey {
				t.Errorf("key: got %q, want %q", got.key, tt.wantKey)
			}
			if got.high != tt.wantHigh {
				t.Errorf("high: got %v, want %v", got.high, tt.wantHigh)
			}
		})
	}
}

// The wiring in ssr.go: walking a tree collects one preload per distinct image
// and none for an img anywhere inside a picture or a noscript. The chunks are
// collected, not yet emitted — flushing them is the hoistable buffer's job — so
// this asserts the collector's contents rather than the markup.
func TestSSREmitterCollectsImagePreloads(t *testing.T) {
	tree := CreateElement("div", nil,
		CreateElement("img", Props{"src": "/a.png"}),
		CreateElement("img", Props{"src": "/a.png"}),
		CreateElement("img", Props{"src": "/b.png"}),
		CreateElement("img", Props{"src": "/c.png", "loading": "lazy"}),
		CreateElement("picture", nil,
			CreateElement("span", nil, CreateElement("img", Props{"src": "/d.png"})),
		),
		CreateElement("noscript", nil, CreateElement("img", Props{"src": "/e.png"})),
		// Back outside the suppressing subtrees, preloads resume.
		CreateElement("img", Props{"src": "/f.png"}),
	)

	root := NewRoot(tree)
	defer root.Unmount()
	var b strings.Builder
	e := &ssrEmitter{w: &b}
	e.children(root.tree())
	if e.err != nil {
		t.Fatalf("render failed: %v", e.err)
	}

	want := []string{
		`<link rel="preload" as="image" href="/a.png"/>`,
		`<link rel="preload" as="image" href="/b.png"/>`,
		`<link rel="preload" as="image" href="/f.png"/>`,
	}
	got := e.preloads.highChunks()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("chunk %d:\n got %s\nwant %s", i, got[i], want[i])
		}
	}
	if bulk := e.preloads.bulkChunks(); len(bulk) != 0 {
		t.Errorf("bulk bucket: got %v, want none", bulk)
	}
	if e.noPreload {
		t.Error("the picture/noscript suppression flag leaked past the subtree")
	}
}

func TestSSRPreloadSuppressingTag(t *testing.T) {
	for _, tag := range []string{"picture", "noscript"} {
		if !ssrPreloadSuppressingTag(tag) {
			t.Errorf("%s should suppress image preloads", tag)
		}
	}
	for _, tag := range []string{"div", "span", "img", "figure", "script"} {
		if ssrPreloadSuppressingTag(tag) {
			t.Errorf("%s should not suppress image preloads", tag)
		}
	}
}

// The bucket rules, from probe renders: the first ten preloads flush early, the
// rest flush late, and fetchPriority="high" jumps the queue.
func TestSSRImagePreloadsBuckets(t *testing.T) {
	var p ssrImagePreloads
	for i := 0; i < 12; i++ {
		src := "/i" + string(rune('0'+i))
		preload, ok := ssrImagePreloadFor(Props{"src": src}, false)
		if !ok {
			t.Fatalf("no preload for %s", src)
		}
		p.add(preload)
	}
	if got := len(p.highChunks()); got != ssrImagePreloadHighBucketLimit {
		t.Errorf("high bucket: got %d entries, want %d", got, ssrImagePreloadHighBucketLimit)
	}
	if got := len(p.bulkChunks()); got != 2 {
		t.Errorf("bulk bucket: got %d entries, want 2", got)
	}
	if first := p.highChunks()[0]; !strings.Contains(first, `href="/i0"`) {
		t.Errorf("high bucket should start with the first img, got %s", first)
	}
	if last := p.bulkChunks()[1]; !strings.Contains(last, `href="/i;"`) {
		// '0'+11 is ';': the twelfth src is "/i;".
		t.Errorf("bulk bucket should end with the twelfth img, got %s", last)
	}
}

func TestSSRImagePreloadsHighPriorityJumpsFullBucket(t *testing.T) {
	var p ssrImagePreloads
	for i := 0; i < 10; i++ {
		preload, _ := ssrImagePreloadFor(Props{"src": "/i" + string(rune('0'+i))}, false)
		p.add(preload)
	}
	preload, _ := ssrImagePreloadFor(Props{"src": "/high", "fetchPriority": "high"}, false)
	p.add(preload)

	high := p.highChunks()
	if len(high) != 11 {
		t.Fatalf("high bucket: got %d entries, want 11", len(high))
	}
	if !strings.Contains(high[10], `href="/high"`) {
		t.Errorf("high-priority preload should be appended to the early bucket, got %s", high[10])
	}
	if len(p.bulkChunks()) != 0 {
		t.Errorf("bulk bucket: got %d entries, want 0", len(p.bulkChunks()))
	}
}

func TestSSRImagePreloadsDedupe(t *testing.T) {
	var p ssrImagePreloads
	for i := 0; i < 3; i++ {
		preload, _ := ssrImagePreloadFor(Props{"src": "/a.png"}, false)
		p.add(preload)
	}
	// The second and third img share the first one's key, and even the crossOrigin
	// of a later img is ignored: one link, from the first occurrence.
	later, _ := ssrImagePreloadFor(Props{"src": "/a.png", "crossOrigin": "anonymous"}, false)
	p.add(later)

	want := []string{`<link rel="preload" as="image" href="/a.png"/>`}
	if got := p.highChunks(); len(got) != 1 || got[0] != want[0] {
		t.Errorf("got %v, want %v", got, want)
	}

	// An img with a srcSet keys on the srcSet, not the src, so it is a distinct
	// resource even when the src matches an earlier one.
	withSrcSet, _ := ssrImagePreloadFor(Props{"src": "/a.png", "srcSet": "/a.png 1x"}, false)
	p.add(withSrcSet)
	if got := len(p.highChunks()); got != 2 {
		t.Errorf("srcSet img should add a second preload, got %d", got)
	}

	// Same srcSet, different sizes: also a distinct key.
	a, _ := ssrImagePreloadFor(Props{"srcSet": "/x 1x", "sizes": "1px"}, false)
	b, _ := ssrImagePreloadFor(Props{"srcSet": "/x 1x", "sizes": "2px"}, false)
	p.add(a)
	p.add(b)
	if got := len(p.highChunks()); got != 4 {
		t.Errorf("srcSet with differing sizes should add two preloads, got %d", got)
	}
}

// A repeat of an image that landed in the bulk bucket promotes it to the end of
// the early bucket, if the early bucket has room by then.
func TestSSRImagePreloadsPromotion(t *testing.T) {
	var p ssrImagePreloads
	for i := 0; i < 10; i++ {
		preload, _ := ssrImagePreloadFor(Props{"src": "/i" + string(rune('0'+i))}, false)
		p.add(preload)
	}
	bulk, _ := ssrImagePreloadFor(Props{"src": "/late"}, false)
	p.add(bulk)
	if len(p.bulkChunks()) != 1 {
		t.Fatalf("expected /late in the bulk bucket, got %v", p.bulkChunks())
	}

	// Nothing can leave the early bucket, so a repeat while it is full stays put.
	p.add(bulk)
	if len(p.bulkChunks()) != 1 || len(p.highChunks()) != 10 {
		t.Fatalf("repeat with a full early bucket should change nothing, got high=%d bulk=%d",
			len(p.highChunks()), len(p.bulkChunks()))
	}

	// With fetchPriority="high" the repeat promotes it regardless of the limit.
	promoted, _ := ssrImagePreloadFor(Props{"src": "/late", "fetchPriority": "high"}, false)
	p.add(promoted)
	high := p.highChunks()
	if len(high) != 11 || len(p.bulkChunks()) != 0 {
		t.Fatalf("promotion should move the entry, got high=%d bulk=%d", len(high), len(p.bulkChunks()))
	}
	if !strings.Contains(high[10], `href="/late"`) {
		t.Errorf("promoted entry should be last in the early bucket, got %s", high[10])
	}
	// Promotion moves the chunk that was already built; the repeat's own
	// fetchPriority does not rewrite it.
	if strings.Contains(high[10], "fetchPriority") {
		t.Errorf("promoted chunk should be the original markup, got %s", high[10])
	}
}
