// Command handlebars-example exercises github.com/malcolmston/handlebars:
// compilation, interpolation + escaping, conditionals, loops, block params,
// partials (incl. partial blocks / inline partials), custom helpers &
// decorators, subexpressions and compile options.
package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/malcolmston/handlebars"
)

type Product struct {
	Name  string  `json:"name"`
	Price float64 `json:"price"`
	Tags  []string
	Stock int `json:"stock"`
}

type Order struct {
	ID       string    `json:"id"`
	Customer string    `json:"customer"`
	Items    []Product `json:"items"`
	Note     string    `json:"note"`
}

func section(title string) {
	fmt.Printf("\n=== %s ===\n", title)
}

func main() {
	basicRendering()
	escaping()
	conditionals()
	loopsAndBlockParams()
	partials()
	customHelpers()
	decoratorsAndInlinePartials()
	compileOptions()
	utilities()
	errorHandling()
	fmt.Println("\nAll handlebars examples completed.")
}

// ---------------------------------------------------------------------------

func basicRendering() {
	section("1. One-shot Render + compiled reuse")

	out, err := handlebars.Render("Hello {{name}}, you are {{age}}!",
		map[string]any{"name": "Ada", "age": 36})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Render:      ", out)

	t := handlebars.MustParse("{{customer}} ordered {{items.0.name}} " +
		// `.length` on a slice works but is undocumented in the README.
		"(id={{id}}, len={{items.length}})")
	fmt.Println("MustParse:   ", t.MustRender(Order{
		ID:       "A-1",
		Customer: "Grace",
		Items:    []Product{{Name: "Keyboard", Price: 49.5}},
	}))

	// Parse returns (*Template, error); ParseString re-parses in place.
	t2, err := handlebars.Parse("path/parent: {{#with items.0}}{{name}} of {{../customer}}{{/with}}")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("parent path: ", t2.MustRender(Order{
		Customer: "Grace",
		Items:    []Product{{Name: "Keyboard"}},
	}))

	// @root reaches the top-level context from any depth.
	fmt.Println("@root:       ", handlebars.MustParse(
		"{{#each items}}{{@root.customer}}:{{name}} {{/each}}").
		MustRender(Order{Customer: "Grace", Items: []Product{{Name: "Mouse"}, {Name: "Pad"}}}))

	// Bracketed segments allow keys with spaces.
	fmt.Println("bracketed:   ", handlebars.MustParse("{{[odd key].[a b]}}").
		MustRender(map[string]any{"odd key": map[string]any{"a b": "works"}}))
}

func escaping() {
	section("2. HTML escaping vs raw output")

	data := map[string]any{"html": `<b>"bold" & 'brave'</b>`}
	fmt.Println("{{ }} escaped:  ", handlebars.MustParse("{{html}}").MustRender(data))
	fmt.Println("{{{ }}} raw:    ", handlebars.MustParse("{{{html}}}").MustRender(data))
	fmt.Println("{{& }} raw:     ", handlebars.MustParse("{{& html}}").MustRender(data))
	fmt.Println("EscapeHTML():   ", handlebars.EscapeHTML(`<a href="x">`))
	fmt.Println("EscapeExpr():   ", handlebars.EscapeExpression(handlebars.SafeString("<i>safe</i>")))
	fmt.Println("comments gone:  ",
		handlebars.MustParse("A{{! short }}B{{!-- long {{mustache}} --}}C").MustRender(nil))
}

func conditionals() {
	section("3. if / else if / else, unless, inverse sections")

	src := `{{#if inStock}}IN STOCK{{else if backorder}}BACKORDER{{else}}SOLD OUT{{/if}}`
	t := handlebars.MustParse(src)
	for _, d := range []map[string]any{
		{"inStock": true},
		{"inStock": false, "backorder": true},
		{"inStock": false, "backorder": false},
	} {
		fmt.Printf("  %-40v -> %s\n", d, t.MustRender(d))
	}

	fmt.Println("unless:  ", handlebars.MustParse(
		"{{#unless note}}(no note){{else}}note={{note}}{{/unless}}").
		MustRender(map[string]any{}))
	fmt.Println("inverse: ", handlebars.MustParse(
		"{{^items}}empty cart{{/items}}").MustRender(map[string]any{"items": []string{}}))

	// Built-in comparison helpers inside subexpressions.
	cmp := handlebars.MustParse(
		`{{#if (and (gt price 10) (lt price 100))}}mid-range{{else}}out of band{{/if}}` +
			` / eq:{{#if (eq name "Mouse")}}yes{{else}}no{{/if}}` +
			` / or:{{#if (or (not name) (gte stock 1))}}available{{/if}}`)
	fmt.Println("helpers: ", cmp.MustRender(Product{Name: "Mouse", Price: 25, Stock: 3}))
}

func loopsAndBlockParams() {
	section("4. each over slices and maps, @index/@key/@first/@last, block params")

	order := Order{
		ID:       "A-42",
		Customer: "Grace",
		Items: []Product{
			{Name: "Keyboard", Price: 49.5, Tags: []string{"input", "usb"}, Stock: 4},
			{Name: "Monitor", Price: 219, Tags: []string{"display"}, Stock: 0},
			{Name: "Cable", Price: 7.25, Stock: 12},
		},
	}

	t := handlebars.MustParse(
		"{{#each items}}{{@index}}. {{name}} (${{price}}){{#if @first}} <-first{{/if}}" +
			"{{#if @last}} <-last{{/if}}\n" +
			"    tags: {{#each Tags}}{{this}}{{#unless @last}}, {{/unless}}{{/each}}\n" +
			"{{/each}}")
	fmt.Print(t.MustRender(order))

	fmt.Println("map (sorted keys):",
		handlebars.MustParse("{{#each m}}{{@key}}={{this}} {{/each}}").
			MustRender(map[string]any{"m": map[string]int{"b": 2, "a": 1, "c": 3}}))

	fmt.Println("nested + @../index:",
		handlebars.MustParse(
			"{{#each items}}{{#each Tags}}{{@../index}}/{{@index}}:{{this}} {{/each}}{{/each}}").
			MustRender(order))

	fmt.Println("block params:      ",
		handlebars.MustParse(
			"{{#each items as |item i|}}{{i}}:{{item.name}} {{/each}}").MustRender(order))

	fmt.Println("with as |o|:       ",
		handlebars.MustParse("{{#with items.1 as |p|}}{{p.name}} costs {{p.price}}{{/with}}").
			MustRender(order))

	fmt.Println("lookup helper:     ",
		handlebars.MustParse(`{{lookup items 2}}-> {{#with (lookup items 2)}}{{name}}{{/with}}`).
			MustRender(order))
}

func partials() {
	section("5. Partials: plain, context arg, hash overlay, dynamic, partial blocks")

	t := handlebars.New()
	if err := t.RegisterPartials(map[string]string{
		"row":    "  - {{name}} @ {{price}}{{#if badge}} [{{badge}}]{{/if}}\n",
		"header": "Order {{id}} for {{customer}}\n",
		"layout": "<<<\n{{> @partial-block}}>>>\n",
		"en":     "Hello {{customer}}\n",
		"fr":     "Bonjour {{customer}}\n",
	}); err != nil {
		log.Fatal(err)
	}
	if err := t.ParseString(
		"{{> header}}" +
			"{{#each items}}{{> row badge=\"sale\"}}{{/each}}" +
			"one item: {{> row items.0}}" +
			"{{#> layout}}wrapped {{customer}}\n{{/layout}}" +
			"{{> (lang)}}"); err != nil {
		log.Fatal(err)
	}
	t.RegisterHelper("lang", func(o *handlebars.Options) any { return "fr" })

	out, err := t.Render(Order{
		ID: "A-42", Customer: "Grace",
		Items: []Product{{Name: "Keyboard", Price: 49.5}, {Name: "Cable", Price: 7.25}},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(out)

	fmt.Println("HasPartial(row):", t.HasPartial("row"), "PartialNames:", t.PartialNames())
	fmt.Println("HasHelper(lang):", t.HasHelper("lang"))

	// Fallback body when the partial is not registered.
	fb := handlebars.New()
	_ = fb.ParseString("{{#> missing}}fallback body{{/missing}}")
	fmt.Println("partial-block fallback:", fb.MustRender(nil))
}

func customHelpers() {
	section("6. Custom helpers: inline, SafeString, hash args, block helper, else")

	t := handlebars.New()
	t.RegisterHelpers(map[string]handlebars.Helper{
		"upper": func(o *handlebars.Options) any {
			return strings.ToUpper(handlebars.Stringify(o.Arg(0)))
		},
		"em": func(o *handlebars.Options) any { // opts out of escaping
			return handlebars.SafeString("<em>" + handlebars.EscapeExpression(o.Arg(0)) + "</em>")
		},
		"money": func(o *handlebars.Options) any { // hash arguments
			cur := o.HashStr("currency", "$")
			return fmt.Sprintf("%s%.2f (%s)", cur, o.Arg(0), o.HashStr("note", "none"))
		},
		"list": func(o *handlebars.Options) any { // block helper w/ block params
			items, _ := o.Arg(0).([]string)
			var b strings.Builder
			for i, it := range items {
				b.WriteString(o.FnWithBlockParams(it, it, i))
			}
			if b.Len() == 0 {
				return o.Inverse(nil) // else section
			}
			return b.String()
		},
	})
	t.RegisterHelper("helperMissing", func(o *handlebars.Options) any {
		return "<?" + o.Name + "?>"
	})

	_ = t.ParseString(
		"upper:   {{upper name}}\n" +
			"em:      {{em name}}\n" +
			"money:   {{money price currency=\"EUR \" note=\"tax incl\"}}\n" +
			"list:    {{#list tags as |tag i|}}{{i}}:{{tag}} {{else}}(none){{/list}}\n" +
			"empty:   {{#list empty}}x{{else}}(none){{/list}}\n" +
			"missing: {{nosuchhelper 1 2}}\n")

	out, err := t.Render(map[string]any{
		"name":  `Ada & <friends>`,
		"price": 49.5,
		"tags":  []string{"a", "b"},
		"empty": []string{},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(out)
	fmt.Println("HelperNames (subset):", t.HelperNames()[:4], "...")

	t.UnregisterHelper("upper")
	fmt.Println("after UnregisterHelper('upper'):", t.HasHelper("upper"))

	// Clone gives an independent registry.
	c := t.Clone()
	c.RegisterHelper("only-on-clone", func(o *handlebars.Options) any { return "" })
	fmt.Println("clone isolation:", c.HasHelper("only-on-clone"), t.HasHelper("only-on-clone"))
}

func decoratorsAndInlinePartials() {
	section("7. Inline partials and custom decorators")

	t := handlebars.New()
	_ = t.ParseString(
		`{{#*inline "chip"}}[{{name}}]{{/inline}}` +
			"{{#each items}}{{> chip}}{{/each}}\n")
	fmt.Println("inline partial:", t.MustRender(Order{
		Items: []Product{{Name: "Keyboard"}, {Name: "Cable"}}}))

	d := handlebars.New()
	d.RegisterDecorator("banner", func(o *handlebars.DecoratorOptions) {
		// Register a partial from the decorator's own program.
		o.RegisterPartial(fmt.Sprint(o.Arg(0)), o.Program)
	})
	if err := d.ParseString(
		`{{#*banner "note"}}** {{customer}} **{{/banner}}{{> note}}`); err != nil {
		log.Fatal(err)
	}
	out, err := d.Render(Order{Customer: "Grace"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("decorator:     ", out)
}

func compileOptions() {
	section("8. Compile options: NoEscape, Strict, NoData, KnownHelpersOnly, SetLogger")

	data := map[string]any{"html": "<b>hi</b>"}
	ne, _ := handlebars.Compile("{{html}}", handlebars.NoEscape())
	fmt.Println("NoEscape:", ne.MustRender(data))

	st, _ := handlebars.Compile("{{missing}}", handlebars.Strict())
	if _, err := st.Render(map[string]any{}); err != nil {
		fmt.Println("Strict error:", err)
	} else {
		fmt.Println("Strict: no error (unexpected)")
	}

	nd, _ := handlebars.Compile("{{#each xs}}[{{@index}}]{{/each}}", handlebars.NoData())
	fmt.Println("NoData:", nd.MustRender(map[string]any{"xs": []int{1, 2}}))

	kh, _ := handlebars.Compile("{{unknownHelper 1}}", handlebars.KnownHelpersOnly())
	if _, err := kh.Render(nil); err != nil {
		fmt.Println("KnownHelpersOnly error:", err)
	}
	kh2, _ := handlebars.Compile("{{eq 1 1}}", handlebars.KnownHelpers("eq"), handlebars.KnownHelpersOnly())
	fmt.Println("KnownHelpers allow-listed:", kh2.MustRender(nil))

	lg := handlebars.MustParse(`{{log "logged from template" level="info"}}ok`)
	lg.SetLogger(log.New(os.Stdout, "hbs-log: ", 0))
	fmt.Println("log helper output ->", lg.MustRender(nil))
}

func utilities() {
	section("9. Exported utilities")

	fmt.Println("Stringify(3.50):  ", handlebars.Stringify(3.50))
	fmt.Println("Stringify(nil):   ", fmt.Sprintf("%q", handlebars.Stringify(nil)))
	fmt.Println("IsEmpty([]):      ", handlebars.IsEmpty([]int{}), "IsEmpty(0):", handlebars.IsEmpty(0))
	fmt.Println("IsArray([1]):     ", handlebars.IsArray([]int{1}), "IsArray(\"s\"):", handlebars.IsArray("s"))
	fmt.Println("Extend:           ", handlebars.Extend(map[string]any{"a": 1}, map[string]any{"b": 2}))
	fmt.Println("CreateFrame:      ", handlebars.CreateFrame(map[string]any{"index": 3}))
}

func errorHandling() {
	section("10. Error handling")

	if _, err := handlebars.Parse("{{#if x}}unclosed"); err != nil {
		fmt.Println("unclosed block:  ", err)
	}
	if _, err := handlebars.Parse("{{/if}}"); err != nil {
		fmt.Println("stray close:     ", err)
	}
	if _, err := handlebars.Parse("{{ unterminated"); err != nil {
		fmt.Println("unterminated:    ", err)
	}
	func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Println("MustParse panics:", r)
			}
		}()
		handlebars.MustParse("{{#each}}")
	}()
	t := handlebars.New()
	if err := t.RegisterPartial("bad", "{{#if}}"); err != nil {
		fmt.Println("bad partial:     ", err)
	}
}
