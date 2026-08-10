// Command puppeteer-example exercises github.com/malcolmston/puppeteer end to
// end without a real browser and without any outbound network access.
//
// Everything below runs against in-process net/http/httptest servers bound to
// loopback, so the program is fully hermetic and always terminates: every
// navigation is bounded by LaunchOptions.Timeout and every wait is bounded by
// WaitForSelectorOptions.Timeout.
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	"github.com/malcolmston/puppeteer"
)

const shortTimeout = 5 * time.Second

func main() {
	section("0. browser / CDP capability probe")
	probeBrowser()

	srv := newSite()
	defer srv.Close()

	browser := section1Launch(srv)
	defer func() { _ = browser.Close() }()

	page := section2Navigate(browser, srv)
	section3Selectors(page)
	section4Traversal(page)
	section5Extraction(page)
	section6Serialization(page)
	section7Cookies(browser, page, srv)
	section8History(browser, srv)
	section9Forms(browser, srv)
	section10WaitForSelector(browser, srv)
	section11Redirects(srv)
	section12Errors(browser)
	section13RawParse()

	section("done")
	fmt.Println("all sections completed")
}

// ---------------------------------------------------------------------------
// 0. Browser probe
// ---------------------------------------------------------------------------

// probeBrowser looks for a real Chrome/Chromium. The library turns out to have
// no Chrome DevTools Protocol layer at all (no websocket transport, no
// Launch(executablePath), no Page.Evaluate/Screenshot), so a browser would be
// unusable even if present; the probe documents that explicitly.
func probeBrowser() {
	candidates := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
		"google-chrome", "chromium", "chromium-browser",
	}
	found := ""
	for _, c := range candidates {
		if strings.Contains(c, "/") {
			if st, err := os.Stat(c); err == nil && !st.IsDir() {
				found = c
				break
			}
			continue
		}
		if p, err := exec.LookPath(c); err == nil {
			found = p
			break
		}
	}
	if found == "" {
		fmt.Println("no Chrome/Chromium executable found on this machine")
	} else {
		fmt.Println("Chrome/Chromium executable present at:", found)
	}
	fmt.Println("skipped: no browser available (and none usable) -- this port exposes")
	fmt.Println("  no CDP transport, no Evaluate, no Screenshot, no PDF, no viewport,")
	fmt.Println("  no click/type/hover; it is an HTTP client + HTML parser + CSS engine.")
	fmt.Println("  Everything that needs a live DOM is therefore skipped below.")
}

// ---------------------------------------------------------------------------
// The fixture site
// ---------------------------------------------------------------------------

var pollHits atomic.Int32

func newSite() *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc123", Path: "/"})
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, indexHTML)
	})

	mux.HandleFunc("/about", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `<!DOCTYPE html><html><head><title>About</title></head><body><h1>About</h1></body></html>`)
	})

	mux.HandleFunc("/contact", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `<!DOCTYPE html><html><head><title>Contact</title></head><body><h1>Contact</h1></body></html>`)
	})

	// Echoes back whatever the form submitted, so we can observe encoding.
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		var b strings.Builder
		b.WriteString("<!DOCTYPE html><html><head><title>Echo</title></head><body>")
		fmt.Fprintf(&b, "<p id=\"method\">%s</p>", r.Method)
		fmt.Fprintf(&b, "<p id=\"ctype\">%s</p>", r.Header.Get("Content-Type"))
		fmt.Fprintf(&b, "<p id=\"ua\">%s</p>", r.Header.Get("User-Agent"))
		fmt.Fprintf(&b, "<p id=\"lang\">%s</p>", r.Header.Get("Accept-Language"))
		fmt.Fprintf(&b, "<p id=\"trace\">%s</p>", r.Header.Get("X-Trace"))
		fmt.Fprintf(&b, "<p id=\"cookie\">%s</p>", r.Header.Get("Cookie"))
		fmt.Fprintf(&b, "<p id=\"values\">%s</p>", r.Form.Encode())
		b.WriteString("</body></html>")
		_, _ = io.WriteString(w, b.String())
	})

	// Only grows a #ready element after a few identical requests: the one thing
	// WaitForSelector (which re-fetches, rather than watching a live DOM) can do.
	mux.HandleFunc("/poll", func(w http.ResponseWriter, r *http.Request) {
		n := pollHits.Add(1)
		if n < 3 {
			fmt.Fprintf(w, `<html><body><p id="pending">attempt %d</p></body></html>`, n)
			return
		}
		fmt.Fprintf(w, `<html><body><p id="ready">ready after %d fetches</p></body></html>`, n)
	})

	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/about", http.StatusFound)
	})

	// A document with no <html>/<head>/<body>, to exercise NormalizedHTML.
	mux.HandleFunc("/fragment", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `<!DOCTYPE html><title>Frag</title><div class="c">hello</div>`)
	})

	return httptest.NewServer(mux)
}

const indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <title>  Fixture Shop  </title>
  <meta name="description" content="a fixture page">
  <meta property="og:title" content="Fixture Shop (OG)">
  <meta charset="utf-8">
  <link rel="stylesheet" href="/css/site.css">
  <script src="/js/app.js"></script>
  <script>var inline = 1;</script>
</head>
<body>
  <h1 id="headline" class="title big">Fixture&nbsp;Shop &amp; Co</h1>
  <!-- a comment -->
  <nav>
    <a href="/about" class="nav">About</a>
    <a href="/contact" class="nav" data-nav-role="secondary" data-testId="contactLink">Contact</a>
    <a href="https://example.com/external">External</a>
    <a href="#frag">Fragment</a>
  </nav>
  <ul id="products">
    <li class="item" data-sku="A-1" data-price-cents="1999">Widget<span class="badge">new</span>
    <li class="item sale" data-sku="B-2" data-price-cents="999">Gadget
    <li class="item" data-sku="C-3" data-price-cents="4999">Gizmo
    <li class="item empty"></li>
  </ul>
  <table>
    <tr><td>r1c1</td><td>r1c2</td></tr>
    <tr><td>r2c1</td><td>r2c2</td></tr>
  </table>
  <img src="/img/a.png"><img src="/img/b.png"><img src="/img/a.png">
  <form id="login" action="/login" method="post">
    <input type="text" name="username" value="default-user">
    <input type="password" name="password">
    <input type="checkbox" name="remember" value="yes" checked>
    <input type="checkbox" name="newsletter" value="yes">
    <input type="hidden" name="csrf" value="tok-42">
    <input type="text" name="ignored" value="x" disabled>
    <textarea name="bio">hello
bio</textarea>
    <select name="plan">
      <option value="free">Free</option>
      <option value="pro" selected>Pro</option>
    </select>
    <input type="submit" name="submitBtn" value="Log in">
  </form>
  <form id="search" action="/login" method="GET">
    <input type="text" name="q" value="">
  </form>
</body>
</html>`

// ---------------------------------------------------------------------------
// 1. Launch options
// ---------------------------------------------------------------------------

func section1Launch(srv *httptest.Server) *puppeteer.Browser {
	section("1. Launch / LaunchOptions")

	def, err := puppeteer.Launch(nil)
	check(err)
	fmt.Println("default user agent:", def.UserAgent())
	fmt.Println("DefaultUserAgent const matches:", def.UserAgent() == puppeteer.DefaultUserAgent)
	_ = def.Close()

	follow := true
	b, err := puppeteer.Launch(&puppeteer.LaunchOptions{
		UserAgent:       "go-example-scraper/1.0",
		Timeout:         shortTimeout,
		Headers:         map[string]string{"Accept-Language": "en-GB", "X-Trace": "sect1"},
		FollowRedirects: &follow,
		// Transport is the documented seam for pointing the browser elsewhere;
		// here it just reuses the httptest server's own transport.
		Transport: srv.Client().Transport,
	})
	check(err)
	fmt.Println("configured user agent:", b.UserAgent())
	b.SetUserAgent("go-example-scraper/1.1")
	fmt.Println("after SetUserAgent:", b.UserAgent())
	b.SetExtraHTTPHeaders(map[string]string{"Accept-Language": "en-GB", "X-Trace": "browser-level"})
	fmt.Println("browser-level headers installed (verified in section 9)")
	return b
}

// ---------------------------------------------------------------------------
// 2. Navigation
// ---------------------------------------------------------------------------

func section2Navigate(b *puppeteer.Browser, srv *httptest.Server) *puppeteer.Page {
	section("2. NewPage / Goto / GotoContext / Response")

	page := b.NewPage()
	resp, err := page.Goto(srv.URL + "/")
	check(err)
	fmt.Printf("status=%d ok=%v url=%s\n", resp.StatusCode, resp.OK(), resp.URL)
	fmt.Println("status text:", resp.Status)
	fmt.Println("content-type header:", resp.Header.Get("Content-Type"))
	fmt.Println("body bytes:", len(resp.Body))
	fmt.Println("page.URL():", page.URL())
	fmt.Printf("page.Title(): %q (leading/trailing space is trimmed by the lib)\n", page.Title())
	fmt.Println("raw Content() length:", len(page.Content()))

	// GotoContext honours cancellation; a relative URL resolves against the
	// current document URL.
	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()
	r2, err := page.GotoContext(ctx, "/about")
	check(err)
	fmt.Println("relative Goto resolved to:", r2.URL, "title:", page.Title())

	// An already-cancelled context must fail fast rather than hang.
	dead, cancelDead := context.WithCancel(context.Background())
	cancelDead()
	if _, err := page.GotoContext(dead, srv.URL+"/"); err != nil {
		fmt.Println("cancelled context rejected as expected:", firstLine(err.Error()))
	} else {
		fmt.Println("UNEXPECTED: cancelled context navigated anyway")
	}

	// Return to the index for the selector sections.
	_, err = page.Goto(srv.URL + "/")
	check(err)
	return page
}

// ---------------------------------------------------------------------------
// 3. Selector engine
// ---------------------------------------------------------------------------

func section3Selectors(page *puppeteer.Page) {
	section("3. CSS selector engine")

	selectors := []string{
		"h1",                   // type
		"*",                    // universal
		"#headline",            // id
		".item",                // class
		"li.item.sale",         // compound
		"a[href]",              // attribute presence
		`a[href="/about"]`,     // exact
		`a[href^="/"]`,         // prefix
		`img[src$=".png"]`,     // suffix
		`li[data-sku*="-"]`,    // substring
		`a[class~="nav"]`,      // whitespace list
		`html[lang|="en"]`,     // dash match
		"ul#products > li",     // child
		"ul li span",           // descendant
		"li + li",              // adjacent sibling
		"li ~ li",              // general sibling
		"h1, nav a",            // selector list
		"li:first-child",       // structural
		"li:last-child",        //
		"li:nth-child(2)",      //
		"li:nth-child(odd)",    //
		"li:nth-child(even)",   //
		"li:nth-child(2n+1)",   //
		"li:nth-child(-n+2)",   //
		"tr:nth-child(2) > td", // nested structural
		// Undocumented but implemented (README lists only :first-child,
		// :last-child and :nth-child):
		"li:nth-last-child(1)",
		"td:first-of-type",
		"td:last-of-type",
		"td:nth-of-type(2)",
		"td:nth-last-of-type(1)",
		"span:only-child",
		"span:only-of-type",
		"li:empty",
		"html:root",
		"li:not(.sale)",
		"li:not(:nth-child(2))", // nested functional pseudo
	}
	for _, s := range selectors {
		n, err := page.Count(s)
		if err != nil {
			fmt.Printf("  %-26s ERROR %v\n", s, err)
			continue
		}
		fmt.Printf("  %-26s -> %d match(es)\n", s, n)
	}

	first, err := page.QuerySelector("li.item")
	check(err)
	fmt.Println("first .item text:", quote(first.TextContent()))

	all, err := page.QuerySelectorAll("li.item")
	check(err)
	for i, el := range all {
		fmt.Printf("  item[%d] sku=%s price=%s text=%s\n",
			i, el.AttrOr("data-sku", "-"), el.AttrOr("data-price-cents", "-"), quote(el.TextContent()))
	}

	// A miss returns (nil, nil) -- not an error.
	miss, err := page.QuerySelector(".does-not-exist")
	fmt.Printf("miss: element=%v err=%v\n", miss, err)

	// Pseudo-classes the engine rejects (invalid selectors surface as errors,
	// never as silent empty results).
	for _, unsupported := range []string{"input:checked", "a:hover", "p::before", "div:has(> span)", "li:is(.sale)", "li[unclosed"} {
		if _, err := page.Count(unsupported); err != nil {
			fmt.Printf("  rejected %-18s -> %s\n", unsupported, firstLine(err.Error()))
		} else {
			fmt.Printf("  rejected %-18s -> accepted (surprising)\n", unsupported)
		}
	}
}

// ---------------------------------------------------------------------------
// 4. Element handles & traversal
// ---------------------------------------------------------------------------

func section4Traversal(page *puppeteer.Page) {
	section("4. Element handles, attributes, traversal")

	h1, err := page.QuerySelector("#headline")
	check(err)
	fmt.Println("TagName:      ", h1.TagName())
	fmt.Println("ID:           ", h1.ID())
	fmt.Println("ClassList:    ", h1.ClassList())
	fmt.Println("HasClass big: ", h1.HasClass("big"))
	fmt.Println("TextContent:  ", quote(h1.TextContent()), "(&nbsp; and &amp; decoded)")
	fmt.Println("InnerText:    ", quote(h1.InnerText()))
	fmt.Println("InnerHTML:    ", quote(h1.InnerHTML()))
	fmt.Println("OuterHTML:    ", quote(h1.OuterHTML()))
	fmt.Println("Attributes:   ", h1.Attributes())
	fmt.Println("AttributeNames:", h1.AttributeNames())
	fmt.Println("HasAttribute(id/foo):", h1.HasAttribute("id"), h1.HasAttribute("foo"))
	v, ok := h1.Attr("class")
	fmt.Printf("Attr(class): %q ok=%v\n", v, ok)
	fmt.Println("AttrOr(missing):", quote(h1.AttrOr("missing", "fallback")))
	fmt.Println("Node().Type is ElementNode:", h1.Node().Type == puppeteer.ElementNode)

	contact, err := page.QuerySelector(`a[data-nav-role]`)
	check(err)
	fmt.Println("Dataset:", contact.Dataset(), "(data-nav-role -> navRole; note lower-cased attrs)")

	second, err := page.QuerySelector("li.sale")
	check(err)
	fmt.Println("Parent:  ", second.Parent().TagName(), "#"+second.Parent().ID())
	fmt.Println("Prev:    ", quote(second.Prev().TextContent()))
	fmt.Println("Next:    ", quote(second.Next().TextContent()))
	fmt.Println("Siblings:", len(second.Siblings()), "PrevAll:", len(second.PrevAll()), "NextAll:", len(second.NextAll()))
	fmt.Println("Children of ul:", len(second.Parent().Children()))
	var anc []string
	for _, a := range second.Ancestors() {
		anc = append(anc, a.TagName())
	}
	fmt.Println("Ancestors:", anc)

	closest, err := second.Closest("ul")
	check(err)
	fmt.Println("Closest(ul):", "#"+closest.ID())
	matched, err := second.Matches("li.item.sale")
	check(err)
	fmt.Println("Matches(li.item.sale):", matched)

	badge, err := second.Parent().QuerySelector(".badge")
	check(err)
	fmt.Println("element-scoped QuerySelector(.badge):", quote(badge.TextContent()))
	tds, err := page.QuerySelectorAll("table")
	check(err)
	inner, err := tds[0].QuerySelectorAll("td")
	check(err)
	fmt.Println("element-scoped QuerySelectorAll(td):", len(inner))

	empty, err := page.QuerySelector("li.empty")
	check(err)
	fmt.Println("IsEmpty on <li class=empty>:", empty.IsEmpty(), "/ on <h1>:", h1.IsEmpty())

	// Element handles are read-only: no SetAttribute, no Click, no Type.
	fmt.Println("note: Element has no mutators (SetAttribute/Click/Type/Focus) -- skipped")
}

// ---------------------------------------------------------------------------
// 5. Page-level extraction
// ---------------------------------------------------------------------------

func section5Extraction(page *puppeteer.Page) {
	section("5. Page extraction helpers")

	fmt.Println("Links():", page.Links())
	fmt.Println("Images() (deduped, absolute):", page.Images())
	fmt.Println("Scripts() (external only):", page.Scripts())
	fmt.Println("Stylesheets():", page.Stylesheets())
	fmt.Println("Metas():", page.Metas())

	desc, ok := page.MetaContent("description")
	fmt.Printf("MetaContent(description)=%q ok=%v\n", desc, ok)
	og, ok := page.MetaContent("OG:TITLE")
	fmt.Printf("MetaContent(OG:TITLE) case-insensitive=%q ok=%v\n", og, ok)
	_, ok = page.MetaContent("nope")
	fmt.Println("MetaContent(nope) ok:", ok)

	txt, found, err := page.TextContent("li.sale")
	check(err)
	fmt.Printf("TextContent(li.sale)=%s found=%v\n", quote(txt), found)
	_, found, err = page.TextContent("li.nothing")
	check(err)
	fmt.Println("TextContent(miss) found:", found)

	attr, found, err := page.GetAttribute("li.sale", "data-sku")
	check(err)
	fmt.Printf("GetAttribute(li.sale,data-sku)=%q found=%v\n", attr, found)
	_, found, err = page.GetAttribute("li.sale", "nope")
	check(err)
	fmt.Println("GetAttribute(missing attr) found:", found)

	n, err := page.Count("li.item")
	check(err)
	fmt.Println("Count(li.item):", n)
}

// ---------------------------------------------------------------------------
// 6. DOM serialization
// ---------------------------------------------------------------------------

func section6Serialization(page *puppeteer.Page) {
	section("6. Document tree + serialization")

	doc := page.Document()
	fmt.Println("Document().Type is DocumentNode:", doc.Type == puppeteer.DocumentNode)
	fmt.Printf("node census: %v\n", census(doc))
	fmt.Println("implicit </li> recovery -> ul children:", len(mustEl(page, "ul#products").Children()))

	raw := page.Content()
	round := page.HTML()
	fmt.Printf("Content() %d bytes vs HTML() re-serialized %d bytes (equal=%v)\n",
		len(raw), len(round), raw == round)
	fmt.Println("HTML() keeps doctype:", strings.HasPrefix(strings.TrimSpace(round), "<!DOCTYPE"))
	fmt.Println("comment survives round trip:", strings.Contains(round, "<!-- a comment -->"))
}

func section6bNormalized(b *puppeteer.Browser, srv *httptest.Server) {
	frag := b.NewPage()
	_, err := frag.Goto(srv.URL + "/fragment")
	check(err)
	fmt.Println("NormalizedHTML of a head/body-less document:")
	fmt.Println("  ", frag.NormalizedHTML())
}

// ---------------------------------------------------------------------------
// 7. Cookies
// ---------------------------------------------------------------------------

func section7Cookies(b *puppeteer.Browser, page *puppeteer.Page, srv *httptest.Server) {
	section("7. Cookie jar")

	// Cookie set by the / handler was captured by the jar during section 2.
	jarCookies, err := b.Cookies(srv.URL + "/")
	check(err)
	for _, c := range jarCookies {
		fmt.Printf("  browser jar: %s=%s\n", c.Name, c.Value)
	}
	pageCookies, err := page.Cookies()
	check(err)
	fmt.Println("page.Cookies() count:", len(pageCookies))

	check(b.SetCookies(srv.URL+"/", []*http.Cookie{
		{Name: "manual", Value: "injected", Path: "/"},
	}))
	after, err := b.Cookies(srv.URL + "/")
	check(err)
	var names []string
	for _, c := range after {
		names = append(names, c.Name+"="+c.Value)
	}
	fmt.Println("after SetCookies:", names)
	fmt.Println("note: no DeleteCookie/ClearCookies API -- skipped")
}

// ---------------------------------------------------------------------------
// 8. Session history
// ---------------------------------------------------------------------------

func section8History(b *puppeteer.Browser, srv *httptest.Server) {
	section("8. Session history (Reload / GoBack / GoForward)")

	p := b.NewPage()
	for _, path := range []string{"/", "/about", "/contact"} {
		_, err := p.Goto(srv.URL + path)
		check(err)
	}
	fmt.Println("history:", trimHost(p.History(), srv.URL))
	fmt.Println("current:", trimOne(p.URL(), srv.URL), "canBack:", p.CanGoBack(), "canFwd:", p.CanGoForward())

	resp, err := p.GoBack()
	check(err)
	fmt.Println("GoBack ->", trimOne(resp.URL.String(), srv.URL), "title:", p.Title())
	resp, err = p.GoBack()
	check(err)
	fmt.Println("GoBack ->", trimOne(resp.URL.String(), srv.URL), "canBack:", p.CanGoBack())

	resp, err = p.GoBack()
	fmt.Printf("GoBack at start of history -> resp=%v err=%v (Puppeteer's null convention)\n", resp, err)

	resp, err = p.GoForward()
	check(err)
	fmt.Println("GoForward ->", trimOne(resp.URL.String(), srv.URL))

	before := len(p.History())
	resp, err = p.Reload()
	check(err)
	fmt.Printf("Reload -> %s ; history length unchanged: %v\n", trimOne(resp.URL.String(), srv.URL), before == len(p.History()))

	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()
	_, err = p.ReloadContext(ctx)
	check(err)
	fmt.Println("ReloadContext ok; history:", trimHost(p.History(), srv.URL))

	// The *Context variants exist for every navigation entry point.
	ctx2, cancel2 := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel2()
	if _, err := p.GoBackContext(ctx2); err != nil {
		check(err)
	}
	if _, err := p.GoForwardContext(ctx2); err != nil {
		check(err)
	}
	fmt.Println("GoBackContext/GoForwardContext ok, now at:", trimOne(p.URL(), srv.URL))

	section6bNormalized(b, srv)
}

// ---------------------------------------------------------------------------
// 9. Forms
// ---------------------------------------------------------------------------

func section9Forms(b *puppeteer.Browser, srv *httptest.Server) {
	section("9. Forms: discovery, filling, BuildRequest, Submit")

	p := b.NewPage()
	p.SetExtraHTTPHeaders(map[string]string{"X-Trace": "page-level"})
	p.SetUserAgent("go-example-page-ua/2.0")
	_, err := p.Goto(srv.URL + "/")
	check(err)

	forms, err := p.Forms()
	check(err)
	fmt.Println("forms discovered:", len(forms))
	for _, f := range forms {
		fmt.Printf("  action=%s method=%s enctype=%s fields=%v\n",
			trimOne(f.Action, srv.URL), f.Method, f.EncType, f.FieldNames())
	}

	login, err := p.FormBySelector("#login")
	check(err)
	fmt.Println("default Values() (unchecked boxes, disabled and submit inputs excluded):", login.Values().Encode())
	uname, _ := login.Get("username")
	bio, _ := login.Get("bio")
	plan, _ := login.Get("plan")
	fmt.Printf("defaults: username=%q bio=%s plan=%q (selected option wins)\n", uname, quoteBody(bio), plan)
	_, exists := login.Get("nope")
	fmt.Println("Get(nope) exists:", exists)

	filled, err := p.FillForm("#login", map[string]string{
		"username":   "alice",
		"password":   "hunter2",
		"newsletter": "yes",
		"extra":      "field-not-in-markup",
	})
	check(err)
	fmt.Println("after FillForm:", filled.Values().Encode())

	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()
	req, err := filled.BuildRequest(ctx)
	check(err)
	fmt.Printf("BuildRequest: %s %s content-type=%q (not sent)\n",
		req.Method, trimOne(req.URL.String(), srv.URL), req.Header.Get("Content-Type"))

	resp, err := filled.Submit()
	check(err)
	fmt.Println("Submit ->", resp.StatusCode, "loaded into page:", p.Title())
	for _, sel := range []string{"#method", "#ctype", "#values", "#ua", "#lang", "#trace", "#cookie"} {
		txt, _, err := p.TextContent(sel)
		check(err)
		fmt.Printf("  echo %-8s %s\n", sel, quote(txt))
	}

	// GET form: values go into the query string.
	_, err = p.Goto(srv.URL + "/")
	check(err)
	search, err := p.FillForm("#search", map[string]string{"q": "go & puppeteer"})
	check(err)
	getReq, err := search.BuildRequest(ctx)
	check(err)
	fmt.Println("GET form BuildRequest URL:", trimOne(getReq.URL.String(), srv.URL))
	gresp, err := search.SubmitContext(ctx)
	check(err)
	echo, _, _ := p.TextContent("#values")
	fmt.Println("GET form Submit status:", gresp.StatusCode, "echoed values:", quote(echo))

	if _, err := p.FormBySelector("#no-such-form"); err != nil {
		fmt.Println("FormBySelector miss:", firstLine(err.Error()))
	}
	fmt.Println("note: no multipart/file-upload support (EncType is reported but ignored) -- skipped")
}

// ---------------------------------------------------------------------------
// 10. WaitForSelector
// ---------------------------------------------------------------------------

func section10WaitForSelector(b *puppeteer.Browser, srv *httptest.Server) {
	section("10. WaitForSelector (poll by re-fetching)")

	p := b.NewPage()
	_, err := p.Goto(srv.URL + "/poll")
	check(err)
	pending, _, _ := p.TextContent("#pending")
	fmt.Println("first fetch:", quote(pending))

	start := time.Now()
	el, err := p.WaitForSelector("#ready", &puppeteer.WaitForSelectorOptions{
		Timeout:  2 * time.Second,
		Interval: 20 * time.Millisecond,
	})
	check(err)
	fmt.Printf("resolved in %v: %s\n", time.Since(start).Round(time.Millisecond), quote(el.TextContent()))
	// HOLE: each internal re-fetch goes through Goto, so every poll appends a
	// duplicate session-history entry (Reload avoids this with an internal lock).
	fmt.Println("HOLE: history entries after polling:", len(p.History()), "for 1 logical navigation")

	// Timeout path is bounded and returns an error rather than hanging.
	start = time.Now()
	_, err = p.WaitForSelector("#never", &puppeteer.WaitForSelectorOptions{
		Timeout:  150 * time.Millisecond,
		Interval: 20 * time.Millisecond,
	})
	fmt.Printf("timeout path after %v: %s\n", time.Since(start).Round(10*time.Millisecond), firstLine(fmt.Sprint(err)))
	fmt.Println("note: WaitForSelector takes no context.Context and has no 'hidden'/'visible' state -- skipped")
}

// ---------------------------------------------------------------------------
// 11. Redirect policy
// ---------------------------------------------------------------------------

func section11Redirects(srv *httptest.Server) {
	section("11. Redirect policy")

	followed, err := puppeteer.Launch(&puppeteer.LaunchOptions{Timeout: shortTimeout})
	check(err)
	defer func() { _ = followed.Close() }()
	fp := followed.NewPage()
	fr, err := fp.Goto(srv.URL + "/redirect")
	check(err)
	fmt.Printf("FollowRedirects default: status=%d final=%s title=%s\n", fr.StatusCode, trimOne(fr.URL.String(), srv.URL), fp.Title())

	no := false
	raw, err := puppeteer.Launch(&puppeteer.LaunchOptions{Timeout: shortTimeout, FollowRedirects: &no})
	check(err)
	defer func() { _ = raw.Close() }()
	rp := raw.NewPage()
	rr, err := rp.Goto(srv.URL + "/redirect")
	check(err)
	fmt.Printf("FollowRedirects=false: status=%d location=%s\n", rr.StatusCode, rr.Header.Get("Location"))
}

// ---------------------------------------------------------------------------
// 12. Error handling / timeouts
// ---------------------------------------------------------------------------

func section12Errors(b *puppeteer.Browser) {
	section("12. Errors, guards and timeouts")

	fresh := b.NewPage()
	if _, err := fresh.QuerySelector("h1"); err != nil {
		fmt.Println("QuerySelector before Goto:", firstLine(err.Error()))
	}
	if _, err := fresh.Reload(); err != nil {
		fmt.Println("Reload before Goto:      ", firstLine(err.Error()))
	}
	if _, err := fresh.Cookies(); err != nil {
		fmt.Println("Cookies before Goto:     ", firstLine(err.Error()))
	}
	if _, err := fresh.WaitForSelector("h1", &puppeteer.WaitForSelectorOptions{Timeout: 50 * time.Millisecond}); err != nil {
		fmt.Println("WaitForSelector before Goto:", firstLine(err.Error()))
	}
	fmt.Println("Title/URL/Content on empty page:", quote(fresh.Title()), quote(fresh.URL()), len(fresh.Content()))
	fmt.Println("NormalizedHTML on empty page:", quote(fresh.NormalizedHTML()))

	fmt.Println("Document() on empty page is nil:", fresh.Document() == nil)

	// A server that never answers: the navigation must be bounded by the
	// browser-level Timeout, so this returns quickly instead of hanging.
	block := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	defer func() { close(block); slow.Close() }()
	tb, err := puppeteer.Launch(&puppeteer.LaunchOptions{Timeout: 200 * time.Millisecond})
	check(err)
	defer func() { _ = tb.Close() }()
	start := time.Now()
	if _, err := tb.NewPage().Goto(slow.URL); err != nil {
		fmt.Printf("hung server timed out after %v: %s\n", time.Since(start).Round(50*time.Millisecond), firstLine(err.Error()))
	} else {
		fmt.Println("UNEXPECTED: hung server returned a response")
	}

	// An unreachable host: no DNS, no network needed (reserved-invalid port).
	ub, err := puppeteer.Launch(&puppeteer.LaunchOptions{Timeout: 500 * time.Millisecond})
	check(err)
	defer func() { _ = ub.Close() }()
	if _, err := ub.NewPage().Goto("http://127.0.0.1:1/"); err != nil {
		fmt.Println("unreachable host:", firstLine(err.Error()))
	}
	if _, err := ub.NewPage().Goto("not a url"); err != nil {
		fmt.Println("invalid URL:      ", firstLine(err.Error()))
	}
}

// ---------------------------------------------------------------------------
// 13. Standalone parsing
// ---------------------------------------------------------------------------

func section13RawParse() {
	section("13. Parse() and the raw Node API")

	doc := puppeteer.Parse(`<!DOCTYPE html><html><head><title>Raw</title></head>
<body><p class="a">one<p class="a">two<br><ul><li>x<li>y</ul>
<!--c--><script>if (a<b) {}</script><input value='unquoted' disabled></body></html>`)
	fmt.Println("Parse never fails; root type is DocumentNode:", doc.Type == puppeteer.DocumentNode)
	fmt.Println("node census:", census(doc))
	fmt.Println("tag outline:", strings.Join(outline(doc, 0), " "))

	// Attribute values and node data are directly readable.
	var walkInput func(*puppeteer.Node)
	walkInput = func(n *puppeteer.Node) {
		if n.Type == puppeteer.ElementNode && n.Data == "input" {
			for _, a := range n.Attr {
				fmt.Printf("  input attr %s=%q\n", a.Name, a.Value)
			}
		}
		for _, c := range n.Children {
			walkInput(c)
		}
	}
	walkInput(doc)

	// AppendChild is the only exported mutator on Node.
	extra := &puppeteer.Node{Type: puppeteer.ElementNode, Data: "section",
		Attr: []puppeteer.Attribute{{Name: "id", Value: "added"}}}
	doc.AppendChild(extra)
	fmt.Println("AppendChild set parent link:", extra.Parent == doc, "root children:", len(doc.Children))

	// HOLE: there is no exported way to run the CSS selector engine, or to get
	// an *Element handle, over a tree produced by Parse. compileSelector,
	// wrapElement and (*selector).queryAll are unexported, Element has no
	// exported constructor, and Page has no SetContent/SetDocument. So the
	// selector engine is reachable only through a Page that has completed a
	// real HTTP navigation. The commented-out call below is what one would want:
	//
	//   el, err := puppeteer.QuerySelector(doc, "p.a")   // does not exist
	//   el, err := puppeteer.NewElement(doc).QuerySelector("p.a") // does not exist
	fmt.Println("HOLE: Parse()'d trees cannot be queried with selectors (no exported entry point)")
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func section(title string) {
	fmt.Printf("\n=== %s ===\n", title)
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func mustEl(p *puppeteer.Page, sel string) *puppeteer.Element {
	el, err := p.QuerySelector(sel)
	check(err)
	if el == nil {
		check(fmt.Errorf("no element for %q", sel))
	}
	return el
}

func quote(s string) string {
	return fmt.Sprintf("%q", strings.Join(strings.Fields(s), " "))
}

func quoteBody(s string) string { return strings.ReplaceAll(s, "\n", `\n`) }

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func census(n *puppeteer.Node) map[string]int {
	out := map[string]int{}
	var walk func(*puppeteer.Node)
	walk = func(n *puppeteer.Node) {
		switch n.Type {
		case puppeteer.DocumentNode:
			out["document"]++
		case puppeteer.ElementNode:
			out["element"]++
		case puppeteer.TextNode:
			if strings.TrimSpace(n.Data) != "" {
				out["text"]++
			}
		case puppeteer.CommentNode:
			out["comment"]++
		case puppeteer.DoctypeNode:
			out["doctype"]++
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(n)
	return out
}

func outline(n *puppeteer.Node, depth int) []string {
	var out []string
	if n.Type == puppeteer.ElementNode {
		out = append(out, strings.Repeat(".", depth)+n.Data)
	}
	for _, c := range n.Children {
		out = append(out, outline(c, depth+1)...)
	}
	return out
}

func trimOne(s, host string) string {
	if s == "" {
		return "(empty)"
	}
	return strings.TrimPrefix(s, host)
}

func trimHost(ss []string, host string) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		out = append(out, trimOne(s, host))
	}
	return out
}
