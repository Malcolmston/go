// Command cheerio-example exercises github.com/malcolmston/cheerio: parsing,
// CSS selectors, accessors, traversal, manipulation, forms and serialization.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/malcolmston/cheerio"
)

const page = `<!DOCTYPE html>
<html lang="en">
<head>
  <title>Fruit &amp; Veg</title>
  <meta charset="utf-8">
</head>
<body>
  <!-- navigation -->
  <nav class="site-nav">
    <ul class="menu">
      <li class="item active"><a href="/" data-nav-id="home" data-sort-order="1">Home</a></li>
      <li class="item"><a href="/shop" data-nav-id="shop" data-sort-order="2">Shop</a></li>
      <li class="item"><a href="https://external.example/blog" rel="nofollow">Blog</a></li>
    </ul>
  </nav>

  <h1 id="title">Produce list</h1>

  <table id="stock">
    <tr><th>Name</th><th>Price</th><th>Tags</th></tr>
    <tr class="row odd"><td class="name">Apple</td><td class="price">1.20</td><td>red,crisp</td></tr>
    <tr class="row even"><td class="name">Banana</td><td class="price">0.50</td><td>yellow</td></tr>
    <tr class="row odd"><td class="name">Cherry</td><td class="price">4.75</td><td>red,sweet</td></tr>
  <p class="note">Prices in EUR
  <p class="note">Updated daily

  <div id="empty"></div>
  <div id="styled" style="color: red; font-weight: bold">styled</div>

  <form id="order" action="/order">
    <input type="text" name="customer" value="Ada">
    <input type="checkbox" name="giftwrap" checked>
    <input type="checkbox" name="insured">
    <input type="radio" name="ship" value="slow">
    <input type="radio" name="ship" value="fast" checked>
    <input type="hidden" name="token" value="abc123">
    <input type="submit" name="go" value="Order">
    <input type="text" name="disabled-field" value="nope" disabled>
    <select name="size">
      <option value="s">Small</option>
      <option value="m" selected>Medium</option>
      <option value="l">Large</option>
    </select>
    <textarea name="comment">no rush</textarea>
  </form>

  <script>var raw = "<not a tag>";</script>
</body>
</html>`

func section(title string) {
	fmt.Printf("\n=== %s ===\n", title)
}

func main() {
	doc := cheerio.Load(page)

	// ------------------------------------------------------------- parsing
	section("Parsing")
	fmt.Printf("  root children: %d\n", doc.Root().Contents().Length())
	fmt.Printf("  <title> (entity decoded): %q\n", doc.Find("title").Text())
	fmt.Printf("  charset meta: %q (void element, children=%d)\n",
		doc.Find("meta").AttrOr("charset", "?"), doc.Find("meta").Contents().Length())
	fmt.Printf("  raw-text <script> preserved verbatim: %q\n", doc.Find("script").Text())
	fmt.Printf("  implicit <p> close recovered: %d sibling .note paragraphs\n",
		doc.Find("p.note").Length())
	comments := doc.Root().Find("*").Contents().FilterFunc(func(_ int, n *cheerio.Node) bool {
		return n.Type == cheerio.CommentNode
	})
	fmt.Printf("  comments preserved: %d -> %q\n", comments.Length(), strings.TrimSpace(comments.First().Nodes()[0].Data))
	fmt.Printf("  doctype node present: %v\n",
		doc.Root().Contents().FilterFunc(func(_ int, n *cheerio.Node) bool {
			return n.Type == cheerio.DoctypeNode
		}).Length() == 1)

	// ----------------------------------------------------------- selectors
	section("Selectors")
	cases := []string{
		"h1#title",
		".menu li",
		"li.item.active a",
		"a[href^='/']",
		"a[href*='example']",
		"a[rel~='nofollow']",
		"html[lang|='en']",
		"#stock tr.row > td.name",
		"tr.row:first-child",
		"tr:nth-child(odd)",
		"td:nth-of-type(2)",
		"tr.row:last-of-type td.name",
		"div:empty",
		"li:only-child",
		":root",
		"li.item:not(.active) a",
		"li:has(a[rel])",
		"td:contains('Cherry')",
		"tr.odd + tr",
		"tr.row ~ p",
		"h1, h2, h3",
		":header",
		":input",
		":checked",
		":selected",
		":disabled",
		":enabled",
	}
	for _, sel := range cases {
		s := doc.Find(sel)
		fmt.Printf("  %-28s n=%-2d first=%q\n", sel, s.Length(), truncate(s.First().Text()))
	}

	section("Precompiled Matcher")
	m, err := cheerio.Compile("td.price")
	if err != nil {
		fatal(err)
	}
	prices := doc.FindMatcher(m)
	fmt.Printf("  %s -> %v\n", m.String(), prices.Map(func(_ int, n *cheerio.Node) string {
		return n.Text()
	}))
	fmt.Printf("  MustCompile+IsMatcher on first row: %v\n",
		doc.Find("tr.row").First().IsMatcher(cheerio.MustCompile("tr.odd")))
	if _, err := cheerio.Compile("li:::bogus("); err != nil {
		fmt.Printf("  Compile(bad selector) -> error: %v\n", err)
	} else {
		fmt.Println("  NOTE: Compile accepted an invalid selector without error")
	}
	// HOLE (documented in README): Find has no error channel, so malformed
	// selectors are silently accepted. "nav.site-nav >" degrades to
	// "nav.site-nav" instead of failing.
	fmt.Printf("  Find(\"li:::bogus(\")=%d (silently empty)  Find(\"nav.site-nav >\")=%d (silently wrong)\n",
		doc.Find("li:::bogus(").Length(), doc.Find("nav.site-nav >").Length())

	// ----------------------------------------------------------- accessors
	section("Accessors: attributes, classes, text, html")
	home := doc.Find("li.active a")
	href, ok := home.Attr("href")
	fmt.Printf("  Attr(href)=%q ok=%v AttrOr(target)=%q\n", href, ok, home.AttrOr("target", "_self"))
	fmt.Printf("  Attrs()=%v\n", home.Attrs())
	fmt.Printf("  TagName=%q HasClass(active on <li>)=%v\n", home.TagName(), home.Parent().HasClass("active"))
	fmt.Printf("  Data(navId)=%q DataOr(missing)=%q DataMap=%v\n",
		home.DataOr("navId", ""), home.DataOr("nope", "fallback"), home.DataMap())
	fmt.Printf("  Prop(tagName)=%s Prop(checked on giftwrap)=%s\n",
		mustProp(home, "tagName"), mustProp(doc.Find("input[name=giftwrap]"), "checked"))
	fmt.Printf("  Text of table row 1: %q\n", truncate(doc.Find("#stock tr.row").First().Text()))
	fmt.Printf("  Html (inner) of nav li 1: %q\n", doc.Find(".menu li").First().Html())
	fmt.Printf("  OuterHtml of nav li 1:    %q\n", doc.Find(".menu li").First().OuterHtml())
	fmt.Printf("  Css(color)=%q Css(font-weight)=%q\n",
		doc.Find("#styled").Css("color"), doc.Find("#styled").Css("font-weight"))
	fmt.Printf("  Val: customer=%q giftwrap=%q size=%q comment=%q\n",
		doc.Find("input[name=customer]").Val(),
		doc.Find("input[name=giftwrap]").Val(),
		doc.Find("select[name=size]").Val(),
		doc.Find("textarea").Val())

	// ----------------------------------------------------------- traversal
	section("Traversal")
	rows := doc.Find("#stock tr.row")
	fmt.Printf("  rows=%d First=%q Last=%q Eq(1)=%q\n", rows.Length(),
		rows.First().Find("td.name").Text(), rows.Last().Find("td.name").Text(),
		rows.Eq(1).Find("td.name").Text())
	fmt.Printf("  Even=%v Odd=%v Slice(1,3)=%v\n",
		names(rows.Even()), names(rows.Odd()), names(rows.Slice(1, 3)))
	fmt.Printf("  Children of table: %v\n", rows.Parent().Children().Map(tag))
	fmt.Printf("  Parents of td.name: %v\n", rows.First().Find("td.name").Parents().Map(tag))
	fmt.Printf("  ParentsUntil(body): %v\n", rows.First().Find("td.name").ParentsUntil("body").Map(tag))
	fmt.Printf("  Closest(table) id=%q\n", rows.First().Closest("table").AttrOr("id", ""))
	fmt.Printf("  Next=%q Prev=%q NextAll=%d PrevAll=%d\n",
		rows.Eq(0).Next().Find("td.name").Text(), rows.Eq(2).Prev().Find("td.name").Text(),
		rows.Eq(0).NextAll().Length(), rows.Eq(2).PrevAll().Length())
	fmt.Printf("  Siblings of row 2: %d\n", rows.Eq(1).Siblings().Length())
	fmt.Printf("  NextUntil(p.note) from header row: %v\n",
		doc.Find("#stock tr").First().NextUntil("p.note").Map(tag))
	fmt.Printf("  Filter(.odd)=%d Not(.odd)=%d Is(tr)=%v Has(td.name)=%d\n",
		rows.Filter(".odd").Length(), rows.Not(".odd").Length(), rows.Is("tr"), rows.Has("td.name").Length())
	fmt.Printf("  FilterFunc(price>1)=%v\n", names(rows.FilterFunc(func(_ int, n *cheerio.Node) bool {
		return len(cheerio.TextOf(n)) > 0 && strings.HasPrefix(strings.TrimSpace(priceOf(n)), "4")
	})))
	fmt.Printf("  Index of row 2 among siblings=%d IndexSelector(tr.row)=%d\n",
		rows.Eq(1).Index(), rows.Eq(1).IndexSelector("tr.row"))
	fmt.Printf("  IndexOfNode(row3)=%d\n", rows.IndexOfNode(rows.Get(2)))
	fmt.Printf("  Add/AddSelection/AddNodes/AddBack: %d %d %d %d\n",
		rows.Add("h1#title").Length(),
		rows.AddSelection(doc.Find("h1#title")).Length(),
		rows.AddNodes(doc.Find("h1#title").Get(0)).Length(),
		doc.Find("#stock").Find("td.name").AddBack().Length())
	fmt.Printf("  End() after Find returns previous selection: %v\n",
		doc.Find("#stock").Find("td").End().Map(tag))
	fmt.Printf("  Each over names: ")
	rows.Find("td.name").Each(func(i int, n *cheerio.Node) {
		fmt.Printf("%d:%s ", i, n.Text())
	})
	fmt.Println()
	fmt.Printf("  Get(0) tag=%s ToArray len=%d Nodes len=%d\n",
		rows.Get(0).TagName(), len(rows.ToArray()), len(rows.Nodes()))

	// -------------------------------------------------------- node-level API
	section("Node-level API")
	n := doc.Find("li.item.active").Get(0)
	fmt.Printf("  TagName=%s Text=%q InnerHTML=%q\n", n.TagName(), n.Text(), n.InnerHTML())
	fmt.Printf("  OuterHTML=%q\n", n.OuterHTML())
	fmt.Printf("  FirstChild type=%v LastChild type=%v\n", n.FirstChild().Type, n.LastChild().Type)
	fmt.Printf("  NextElementSibling=%q PrevElementSibling=%v\n",
		n.NextElementSibling().Text(), n.PrevElementSibling())
	fmt.Printf("  Matches('li.active')=%v HasClass('item')=%v\n", n.Matches("li.active"), n.HasClass("item"))
	v, _ := n.AttrValue("class")
	fmt.Printf("  AttrValue(class)=%q\n", v)

	// -------------------------------------------------------- static helpers
	section("Static helpers")
	frag := cheerio.ParseHTML(`<span class="tag">new</span><b>bold</b>`)
	fmt.Printf("  ParseHTML -> %d nodes, RenderNodes=%q, TextOf=%q\n",
		len(frag), cheerio.RenderNodes(frag...), cheerio.TextOf(frag...))
	fmt.Printf("  Contains(table, td)=%v Contains(td, table)=%v\n",
		cheerio.Contains(doc.Find("#stock").Get(0), doc.Find("td.name").Get(0)),
		cheerio.Contains(doc.Find("td.name").Get(0), doc.Find("#stock").Get(0)))
	fmt.Printf("  Merge dedupes: %d\n",
		len(cheerio.Merge(doc.Find("td.name").ToArray(), doc.Find("td.name").Get(0))))

	// --------------------------------------------------------------- forms
	section("Forms")
	form := doc.Find("#order")
	for _, f := range form.SerializeArray() {
		fmt.Printf("  field %-15s = %q\n", f.Name, f.Value)
	}
	fmt.Printf("  Serialize() = %s\n", form.Serialize())

	// -------------------------------------------------------- manipulation
	section("Manipulation")
	work := cheerio.Load(`<div id="box"><p class="a">one</p><p class="b">two</p></div>`)
	box := work.Find("#box")

	box.Append(`<p class="c">three</p>`)
	box.Prepend(`<p class="z">zero</p>`)
	fmt.Printf("  after Append/Prepend: %s\n", box.Html())

	work.Find("p.a").Before(`<hr>`).After(`<br>`)
	fmt.Printf("  after Before/After:   %s\n", box.Html())

	work.Find("p.b").SetText("TWO").AddClass("bold big").RemoveClass("b").ToggleClass("bold")
	fmt.Printf("  after SetText/class ops: %s\n", box.Html())

	work.Find("p.c").SetHtml(`<em>3</em>`).SetAttr("data-n", "3").SetAttrs(map[string]string{"id": "third"})
	fmt.Printf("  after SetHtml/SetAttr(s): %s\n", box.Html())

	work.Find("#third").SetCss("color", "blue").SetCssMap(map[string]string{"margin": "0", "padding": "1px"})
	fmt.Printf("  after SetCss/SetCssMap: %s\n", work.Find("#third").OuterHtml())

	work.Find("#third").SetData("rowIndex", "7")
	fmt.Printf("  after SetData: %v ; RemoveData -> %v\n",
		work.Find("#third").DataMap(), work.Find("#third").RemoveData("rowIndex").DataMap())

	work.Find("p.z").ReplaceWith(`<h2>heading</h2>`)
	fmt.Printf("  after ReplaceWith: %s\n", box.Html())

	work.Find("br, hr").Remove()
	fmt.Printf("  after Remove(br,hr): %s\n", box.Html())

	work.Find("h2").Wrap(`<header class="hd"></header>`)
	fmt.Printf("  after Wrap: %s\n", box.Html())

	work.Find("p").WrapAll(`<section class="all"></section>`)
	fmt.Printf("  after WrapAll: %s\n", box.Html())

	work.Find("#third").WrapInner(`<strong></strong>`)
	fmt.Printf("  after WrapInner: %s\n", work.Find("#third").OuterHtml())

	work.Find("header.hd h2").Unwrap()
	fmt.Printf("  after Unwrap: %s\n", box.Html())

	clone := work.Find("section.all").Clone()
	clone.Find("p").SetText("cloned")
	fmt.Printf("  Clone is independent: clone=%q original still=%q\n",
		truncate(clone.Text()), truncate(work.Find("section.all").Text()))

	moved := cheerio.Load(`<div id="src"><i>x</i></div><div id="dst"></div>`)
	moved.Find("#src i").AppendTo("#dst")
	fmt.Printf("  AppendTo: %s\n", moved.Html())
	moved.Find("#dst i").PrependTo("#src")
	fmt.Printf("  PrependTo: %s\n", moved.Html())
	moved.Find("#src i").InsertAfter("#dst")
	fmt.Printf("  InsertAfter: %s\n", moved.Html())
	moved.Find("i").InsertBefore("#src")
	fmt.Printf("  InsertBefore: %s\n", moved.Html())
	moved.Find("#src").AppendNodes(cheerio.ParseHTML(`<u>u</u>`)...)
	moved.Find("#dst").PrependSelection(moved.Find("u"))
	fmt.Printf("  AppendNodes/PrependSelection: %s\n", moved.Html())

	work.Find("section.all").Empty()
	fmt.Printf("  after Empty: %q\n", box.Html())

	// --------------------------------------------------------------- output
	section("Output")
	out := cheerio.Load(`<p>a &amp; b <img src="x.png"> <span>c</span></p>`)
	fmt.Printf("  Document.Html: %s\n", out.Html())
	fmt.Printf("  Document.Text: %q\n", out.Text())
	fmt.Printf("  Selection.ToString: %s\n", out.Find("p").ToString())
	fmt.Print("  Selection.Render: ")
	if err := out.Find("span").Render(os.Stdout); err != nil {
		fatal(err)
	}
	fmt.Println()
	fmt.Print("  Document.Render: ")
	if err := out.Render(os.Stdout); err != nil {
		fatal(err)
	}
	fmt.Println()
	fmt.Printf("  MapNodes: %d nodes\n", len(doc.Find("td.name").MapNodes(func(_ int, n *cheerio.Node) *cheerio.Node {
		return n
	})))

	fmt.Println("\nAll cheerio examples completed.")
}

func tag(_ int, n *cheerio.Node) string { return n.TagName() }

func names(s *cheerio.Selection) []string {
	return s.Find("td.name").Map(func(_ int, n *cheerio.Node) string { return n.Text() })
}

func priceOf(row *cheerio.Node) string {
	for _, c := range row.Children {
		if c.Type == cheerio.ElementNode && c.HasClass("price") {
			return c.Text()
		}
	}
	return ""
}

func mustProp(s *cheerio.Selection, name string) string {
	v, ok := s.Prop(name)
	if !ok {
		return "<undefined>"
	}
	return v
}

func truncate(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 48 {
		return s[:48] + "..."
	}
	return s
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "FATAL:", err)
	os.Exit(1)
}
