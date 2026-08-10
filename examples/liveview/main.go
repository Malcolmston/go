// Command liveview-example drives the github.com/malcolmston/liveview port
// entirely in-process: it defines a stateful view (plus a stateful live
// component), mounts it, renders the initial HTML, sends events that mutate
// server state, and asserts the resulting minimal diffs. It then exercises the
// HTTP transport (initial page + POST /event) and a real WebSocket round trip
// against an httptest server, with deadlines so nothing can block forever.
//
// The program prints labelled output, reports PASS/FAIL for every assertion,
// and terminates on its own.
package main

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/malcolmston/liveview"
)

// ---------------------------------------------------------------- assertions

var failures int

func section(name string) { fmt.Printf("\n=== %s ===\n", name) }

func show(label string, v any) { fmt.Printf("  %-26s %v\n", label+":", v) }

func assert(label string, cond bool, detail ...any) {
	if cond {
		fmt.Printf("  PASS  %s\n", label)
		return
	}
	failures++
	fmt.Printf("  FAIL  %s %v\n", label, detail)
}

func assertEq(label string, got, want any) {
	assert(label, fmt.Sprint(got) == fmt.Sprint(want), fmt.Sprintf("got %v want %v", got, want))
}

func jsonOf(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "MARSHAL ERROR: " + err.Error()
	}
	return string(b)
}

// ---------------------------------------------------------------- the view

// Toggle is a stateful live component embedded in the dashboard. Its state
// survives parent re-renders and its diff travels under the "c" key.
type Toggle struct{ id string }

var toggleTmpl = liveview.MustParse(
	`<button class="{{ cls }}" phx-click="toggle">{{ label }}: {{ state }}</button>`,
)

func (t *Toggle) ID() string { return t.id }

func (t *Toggle) Mount(s *liveview.Socket) error {
	s.Assign("on", false)
	s.Assign("label", "Notifications")
	return nil
}

func (t *Toggle) HandleEvent(event string, _ map[string]any, s *liveview.Socket) error {
	if event == "toggle" {
		v, _ := s.Get("on")
		on, _ := v.(bool)
		s.Assign("on", !on)
	}
	return nil
}

func (t *Toggle) Render(a map[string]any) *liveview.Rendered {
	on, _ := a["on"].(bool)
	state := "off"
	if on {
		state = "on"
	}
	return toggleTmpl.Render(map[string]any{
		"cls":   liveview.ClassList(map[string]bool{"btn": true, "btn-on": on, "btn-off": !on}),
		"label": a["label"],
		"state": state,
	})
}

// Dashboard is the top-level stateful view. It implements View plus the
// optional ParamsHandler and InfoHandler interfaces.
type Dashboard struct{}

var (
	headerTmpl = liveview.MustParse(`<header><h1>{{ title }}</h1><small>{{ filter }}</small></header>`)
	fullTmpl   = liveview.MustParse(
		`<div id="dash"{{ attrs }}>{{ header }}<p class="count">{{ count }}</p>` +
			`<div class="toggle">{{ toggle }}</div><p class="notice">{{ notice }}</p>` +
			`<p class="flash">{{ flash }}</p><nav>{{ link }}</nav><ul id="items" phx-update="stream"></ul></div>`,
	)
	compactTmpl = liveview.MustParse(`<div id="dash" class="compact">{{ count }}</div>`)
)

func (Dashboard) Mount(params map[string]any, s *liveview.Socket) error {
	s.Assign("title", "Dashboard")
	s.Assign("count", 0)
	s.Assign("notice", "")
	s.Assign("mode", "full")
	s.Assign("filter", "all")
	// Reference the stateful component; the returned ref lives in an assign.
	s.Assign("toggle", s.LiveComponent(&Toggle{id: "toggle-1"}))
	s.AllowUpload("avatar", liveview.UploadOptions{
		Accept:      []string{".png"},
		MaxEntries:  2,
		MaxFileSize: 1 << 20,
	})
	if v, ok := params["title"]; ok {
		s.Assign("title", v)
	}
	return nil
}

func (Dashboard) HandleEvent(event string, payload map[string]any, s *liveview.Socket) error {
	switch event {
	case "inc":
		s.Assign("count", s.GetInt("count")+1)
	case "dec":
		s.Assign("count", s.GetInt("count")-1)
	case "bump":
		by := 1
		if v, ok := payload["by"].(float64); ok {
			by = int(v)
		}
		s.Assign("count", s.GetInt("count")+by)
	case "noop":
		// deliberately changes nothing
	case "compact":
		s.Assign("mode", "compact")
	case "full":
		s.Assign("mode", "full")
	case "flash":
		liveview.PutFlash(s, "info", "saved <ok>")
	case "clear-flash":
		liveview.ClearFlash(s)
	case "ping-client":
		s.PushEvent("scroll-to-top", map[string]any{"smooth": true})
	case "goto-page-2":
		s.PushPatch("/dash?filter=archived")
	case "leave":
		s.PushNavigate("/other")
	case "self-message":
		s.Send("hello-from-self")
	case "add-item":
		name, _ := payload["name"].(string)
		s.StreamInsert("items", "item-"+name, -1, liveview.MustParse(`<li>{{ n }}</li>`).
			Render(map[string]any{"n": name}))
	case "drop-item":
		name, _ := payload["name"].(string)
		s.StreamDelete("items", "item-"+name)
	case "reset-items":
		s.Stream("items").Reset()
	}
	return nil
}

func (Dashboard) HandleParams(params map[string]any, uri string, s *liveview.Socket) error {
	if f, ok := params["filter"]; ok {
		s.Assign("filter", f)
	}
	return nil
}

func (Dashboard) HandleInfo(msg any, s *liveview.Socket) error {
	s.Assign("notice", fmt.Sprint(msg))
	return nil
}

func (Dashboard) Render(a map[string]any) *liveview.Rendered {
	if a["mode"] == "compact" {
		return compactTmpl.Render(a)
	}
	return fullTmpl.Render(map[string]any{
		"attrs":  liveview.AttrList(map[string]any{"data-count": a["count"], "hidden": false, "role": "main"}),
		"header": headerTmpl.Render(a),
		"count":  a["count"],
		"toggle": a["toggle"],
		"notice": a["notice"],
		"flash":  flashOf(a),
		"link":   liveview.LivePatch("Archived", "/dash?filter=archived"),
	})
}

// flashOf reads the flash map out of the assigns snapshot. There is no exported
// helper for this: Flash lives under a private assign key, and the exported
// accessors (GetFlash/SocketFlash) all take a *Socket, which Render never sees.
func flashOf(a map[string]any) string {
	for _, v := range a {
		if fl, ok := v.(liveview.Flash); ok {
			return strings.Join(func() []string {
				out := make([]string, 0, len(fl))
				for _, k := range fl.Kinds() {
					out = append(out, k+"="+fl.Get(k))
				}
				return out
			}(), ",")
		}
	}
	return ""
}

// ---------------------------------------------------------------- main

func main() {
	engineBasics()
	componentAndSideChannels()
	pubsubAndInfo()
	streamsAndUploads()
	formsAndHelpers()
	httpTransport()
	websocketTransport()

	fmt.Println()
	if failures > 0 {
		fmt.Printf("%d assertion(s) FAILED\n", failures)
		os.Exit(1)
	}
	fmt.Println("all assertions passed")
}

// engineBasics drives state -> render -> diff directly, with no transport.
func engineBasics() {
	section("engine: mount, render, event, diff")

	sess := liveview.NewSession(Dashboard{})
	initial, err := sess.Mount(map[string]any{"title": "Live & Well"})
	if err != nil {
		panic(err)
	}
	html := initial.HTML()
	show("initial HTML", html)
	assert("statics/dynamics invariant", len(initial.Statics) == len(initial.Dynamics)+1)
	assert("title interpolated & escaped", strings.Contains(html, "Live &amp; Well"))
	assert("component rendered inline", strings.Contains(html, "Notifications: off"))
	assert("AttrList emitted attrs", strings.Contains(html, `role="main"`))
	assert("false bool attr omitted", !strings.Contains(html, "hidden"))
	assert("count starts at 0", strings.Contains(html, `<p class="count">0</p>`))

	full := sess.InitialDiff()
	show("InitialDiff JSON", jsonOf(full))
	_, hasStatics := full["s"]
	assert("initial diff carries statics", hasStatics)
	assert("initial diff carries components", full["c"] != nil)

	d1, err := sess.Event("inc", nil)
	if err != nil {
		panic(err)
	}
	show("diff after inc", jsonOf(d1))
	_, resent := d1["s"]
	assert("inc diff omits statics", !resent)
	assert("inc diff is sparse (<=3 keys)", len(d1) <= 3, len(d1))
	assert("re-render shows count 1", strings.Contains(sess.Render().HTML(), `<p class="count">1</p>`))

	d2, _ := sess.Event("bump", map[string]any{"by": float64(9)})
	show("diff after bump by 9", jsonOf(d2))
	assertEq("count after bump", strings.Contains(sess.Render().HTML(), `<p class="count">10</p>`), true)

	d3, _ := sess.Event("noop", nil)
	show("diff after noop", jsonOf(d3))
	assert("no-op yields empty diff", d3.Empty())

	// A structural change (different template) must re-send statics.
	d4, _ := sess.Event("compact", nil)
	show("diff after mode switch", jsonOf(d4))
	_, structural := d4["s"]
	assert("template swap re-sends statics", structural)
	assertEq("compact HTML", sess.Render().HTML(), `<div id="dash" class="compact">10</div>`)
	_, _ = sess.Event("full", nil)

	// DiffRendered / FullDiff can be used standalone.
	prev := headerTmpl.Render(map[string]any{"title": "A", "filter": "all"})
	next := headerTmpl.Render(map[string]any{"title": "B", "filter": "all"})
	show("FullDiff(prev)", jsonOf(liveview.FullDiff(prev)))
	show("DiffRendered(prev,next)", jsonOf(liveview.DiffRendered(prev, next)))
	assertEq("standalone diff is one slot", len(liveview.DiffRendered(prev, next)), 1)
	assert("DiffRendered(nil,next) is full", liveview.DiffRendered(nil, next)["s"] != nil)
	assert("DiffRendered(prev,nil) empty", liveview.DiffRendered(prev, nil).Empty())

	// Socket change tracking.
	sock := liveview.NewSocket()
	sock.AssignAll(map[string]any{"a": 1, "b": "two"})
	assert("Changed(a)", sock.Changed("a"))
	assert("AnyChanged", sock.AnyChanged())
	sock.ResetChanges()
	assert("ResetChanges clears", !sock.AnyChanged())
	assertEq("GetInt/GetString", fmt.Sprintf("%d/%s", sock.GetInt("a"), sock.GetString("b")), "1/two")
	assertEq("GetInt on wrong type is 0", sock.GetInt("b"), 0)
	sock.Assign("a", 5)
	snap := sock.Assigns()
	snap["a"] = 999
	v, _ := sock.Get("a")
	assertEq("Assigns() is a copy", v, 5)

	// Safe opts out of escaping.
	esc := liveview.MustParse(`{{ a }}|{{ b }}`).Render(map[string]any{
		"a": `<b>&"'`,
		"b": liveview.Safe(`<b>bold</b>`),
	})
	assertEq("escape vs Safe", esc.HTML(), `&lt;b&gt;&amp;&quot;&#39;|<b>bold</b>`)

	// Parse errors.
	_, err = liveview.Parse(`{{ unterminated`)
	assert("Parse reports unterminated slot", err != nil, err)
	_, err = liveview.Parse(`{{ }}`)
	assert("Parse reports empty slot", err != nil, err)
	assertEq("{{{{ escapes braces", liveview.MustParse(`{{{{ x }}`).Render(nil).HTML(), `{{ x }}`)
	assertEq("missing assign renders empty", liveview.MustParse(`[{{ nope }}]`).Render(nil).HTML(), `[]`)

	// Verify the README's "Driving the engine directly" snippet verbatim.
	cs := liveview.NewSession(&liveview.Counter{Start: 0})
	cinit, err := cs.Mount(map[string]any{"label": "Clicks"})
	if err != nil {
		panic(err)
	}
	show("README Counter HTML", cinit.HTML())
	assertEq("README: Counter initial HTML", cinit.HTML(),
		`<div class="counter"><h1>Clicks</h1><span class="value">0</span></div>`)
	cdiff, _ := cs.Event("inc", nil)
	show("README Counter diff", jsonOf(cdiff))
	assertEq("README: inc diff is {\"1\":\"1\"}", jsonOf(cdiff), `{"1":"1"}`)
	// Counter's "set"/"label" events want typed payloads (int/string), which is
	// awkward from JSON where numbers decode as float64.
	_, _ = cs.Event("set", map[string]any{"value": 7})
	_, _ = cs.Event("label", map[string]any{"text": "Taps"})
	assertEq("Counter set+label", cs.Render().HTML(),
		`<div class="counter"><h1>Taps</h1><span class="value">7</span></div>`)
	_, _ = cs.Event("set", map[string]any{"value": float64(99)})
	assert("Counter \"set\" ignores JSON float64 payload",
		strings.Contains(cs.Render().HTML(), `>7<`))
}

func componentAndSideChannels() {
	section("components, flash, push events, navigation")

	sess := liveview.NewSession(Dashboard{})
	if _, err := sess.Mount(nil); err != nil {
		panic(err)
	}
	_ = sess.InitialDiff() // marks component renders as sent

	cid := sess.Socket().Components().CID("toggle-1")
	assertEq("component cid assigned", cid, 1)

	cd, err := sess.ComponentEvent(cid, "toggle", nil)
	if err != nil {
		panic(err)
	}
	show("diff after component event", jsonOf(cd))
	comps, ok := cd["c"].(map[string]any)
	assert("component diff under \"c\"", ok && len(comps) == 1, cd["c"])
	_, parentSlot := cd["3"]
	assert("parent slot omitted (stable cid)", !parentSlot)
	assert("component HTML flipped to on", strings.Contains(sess.Render().HTML(), "Notifications: on"))

	// Routing by string ID (test/driver convenience).
	if err := sess.Socket().Components().EventByID("toggle-1", "toggle", nil); err != nil {
		panic(err)
	}
	after, _ := sess.Event("noop", nil)
	show("diff after EventByID", jsonOf(after))
	assert("EventByID flipped back to off", strings.Contains(sess.Render().HTML(), "Notifications: off"))
	assertEq("CID of unknown component", sess.Socket().Components().CID("nope"), 0)

	// Flash.
	fd, _ := sess.Event("flash", nil)
	show("diff after flash", jsonOf(fd))
	assertEq("GetFlash", liveview.GetFlash(sess.Socket(), "info"), "saved <ok>")
	assert("flash rendered (escaped)", strings.Contains(sess.Render().HTML(), "info=saved &lt;ok&gt;"))
	_, _ = sess.Event("clear-flash", nil)
	assertEq("ClearFlash", liveview.GetFlash(sess.Socket(), "info"), "")

	fl := liveview.NewFlash()
	fl.Put("error", "boom")
	fl.Put("info", "hi")
	assertEq("Flash.Kinds sorted", fl.Kinds(), []string{"error", "info"})
	assert("Flash.Has/Get", fl.Has("error") && fl.Get("error") == "boom")
	fl.Delete("error")
	assert("Flash.Delete", !fl.Has("error"))
	assertEq("Flash.Merge", fl.Merge(liveview.Flash{"warn": "w"})["warn"], "w")

	// push_event side channel.
	pe, _ := sess.Event("ping-client", nil)
	show("diff with push events", jsonOf(pe))
	evs, ok := pe["e"].([]liveview.PushEvent)
	assert("push event in \"e\"", ok && len(evs) == 1 && evs[0].Name == "scroll-to-top", pe["e"])

	// push_patch / push_navigate side channel + HandleParams.
	np, _ := sess.Event("goto-page-2", nil)
	show("diff with nav", jsonOf(np))
	nav, ok := np["nav"].(*liveview.Nav)
	assert("nav patch in \"nav\"", ok && nav.Kind == "patch" && nav.To == "/dash?filter=archived", np["nav"])
	assert("nav is drained once", sess.Socket().Nav() == nil)

	pd, _ := sess.Params(nil, "/dash?filter=archived")
	show("diff after live patch", jsonOf(pd))
	assert("HandleParams applied filter", strings.Contains(sess.Render().HTML(), "<small>archived</small>"))

	nn, _ := sess.Event("leave", nil)
	navi, _ := nn["nav"].(*liveview.Nav)
	assertEq("push_navigate kind", navi.Kind, "navigate")

	// JS command builder.
	js := liveview.NewJS().AddClass("open", "#menu").Toggle("#panel").
		Transition("fade", "#panel", 300).Focus("#q").Push("refresh")
	show("JS command JSON", js.String())
	assertEq("JS command count", len(js.Commands()), 5)
	assert("JS Safe() is markup-ready", strings.HasPrefix(string(js.Safe()), "["))
	combined := liveview.NewJS().Hide("#a").Concat(liveview.NewJS().Show("#b"))
	assertEq("JS Concat", len(combined.Commands()), 2)
}

func pubsubAndInfo() {
	section("pubsub & handle_info")

	hub := liveview.NewPubSub()
	sess := liveview.NewSession(Dashboard{})
	sess.AttachPubSub(hub)
	if _, err := sess.Mount(nil); err != nil {
		panic(err)
	}
	sess.SetConnected(true)
	assert("Socket.Connected", sess.Socket().Connected())

	sess.Socket().Subscribe("updates")
	assertEq("SubscriberCount", hub.SubscriberCount("updates"), 1)
	assertEq("Broadcast delivered", hub.Broadcast("updates", "deploy finished"), 1)

	msg := <-sess.Inbox()
	id, err := sess.Info(msg)
	if err != nil {
		panic(err)
	}
	show("diff after handle_info", jsonOf(id))
	assert("notice rendered", strings.Contains(sess.Render().HTML(), "deploy finished"))

	// Socket.Send routes into the same mailbox.
	_, _ = sess.Event("self-message", nil)
	self := <-sess.Inbox()
	_, _ = sess.Info(self)
	assert("Socket.Send delivered", strings.Contains(sess.Render().HTML(), "hello-from-self"))

	assertEq("Broadcast to empty topic", hub.Broadcast("nobody", 1), 0)
	sess.Close()
	assertEq("Close unsubscribes", hub.SubscriberCount("updates"), 0)
}

func streamsAndUploads() {
	section("streams & uploads")

	sess := liveview.NewSession(Dashboard{})
	if _, err := sess.Mount(nil); err != nil {
		panic(err)
	}
	_ = sess.InitialDiff()

	sd, _ := sess.Event("add-item", map[string]any{"name": "alpha"})
	show("diff with stream insert", jsonOf(sd))
	ops, ok := sd["stream"].(map[string][]liveview.StreamOp)
	assert("stream ops under \"stream\"", ok && len(ops["items"]) == 1, sd["stream"])
	assertEq("insert op shape", fmt.Sprintf("%s/%s/%s", ops["items"][0].Action, ops["items"][0].DOM, ops["items"][0].HTML),
		"insert/item-alpha/<li>alpha</li>")

	dd, _ := sess.Event("drop-item", map[string]any{"name": "alpha"})
	dops, _ := dd["stream"].(map[string][]liveview.StreamOp)
	assertEq("delete op", dops["items"][0].Action, "delete")
	rd, _ := sess.Event("reset-items", nil)
	rops, _ := rd["stream"].(map[string][]liveview.StreamOp)
	assertEq("reset op", rops["items"][0].Action, "reset")
	empty, _ := sess.Event("noop", nil)
	assert("stream queue drained", empty["stream"] == nil)

	st := sess.Socket().Stream("items")
	st.Append("x", liveview.MustParse(`<li>x</li>`).Render(nil))
	st.Prepend("y", liveview.MustParse(`<li>y</li>`).Render(nil))
	assertEq("Stream.Pending", st.Pending(), 2)
	_, _ = sess.Event("noop", nil)
	assertEq("Pending after drain", st.Pending(), 0)

	// Uploads: register metadata, stream chunks, then consume.
	cfg := sess.Socket().Upload("avatar")
	assert("AllowUpload registered slot", cfg != nil && cfg.Name == "avatar")
	assertEq("MaxEntries", cfg.MaxEntries, 2)
	if err := sess.RegisterUploadEntry("avatar", "ref-1", "face.png", 10, "image/png"); err != nil {
		panic(err)
	}
	if _, err := sess.UploadChunk("avatar", "ref-1", []byte("12345"), false); err != nil {
		panic(err)
	}
	entries := sess.Socket().UploadEntries("avatar")
	assertEq("progress at half", entries[0].Progress, 50)
	assert("not done yet", !entries[0].Done)
	if _, err := sess.UploadChunk("avatar", "ref-1", []byte("67890"), true); err != nil {
		panic(err)
	}
	entries = sess.Socket().UploadEntries("avatar")
	assert("entry done at 100%", entries[0].Done && entries[0].Progress == 100)
	assertEq("assembled bytes", string(entries[0].Bytes()), "1234567890")

	consumed := liveview.ConsumeUploadedEntries(sess.Socket(), "avatar", func(e *liveview.UploadEntry) any {
		return e.Name + ":" + string(e.Bytes())
	})
	assertEq("ConsumeUploadedEntries", consumed, []any{"face.png:1234567890"})
	assertEq("entries removed after consume", len(sess.Socket().UploadEntries("avatar")), 0)

	// Oversized entry is rejected.
	sock := liveview.NewSocket()
	sock.AllowUpload("small", liveview.UploadOptions{MaxFileSize: 4})
	tiny := liveview.NewSession(Dashboard{})
	_, _ = tiny.Mount(nil)
	err := tiny.RegisterUploadEntry("avatar", "big", "big.png", 1<<30, "image/png")
	assert("oversized upload rejected", err != nil, err)
	assert("unknown slot is a no-op", tiny.RegisterUploadEntry("nope", "r", "f", 1, "") == nil)
	d, err := tiny.UploadChunk("nope", "r", []byte("x"), true)
	assert("chunk to unknown slot is empty diff", err == nil && d.Empty())
}

func formsAndHelpers() {
	section("forms & HTML helpers")

	decoded, err := liveview.DecodeFormString("user[name]=Ada&user[address][city]=Bath&tags[]=a&tags[]=b&flat=1")
	if err != nil {
		panic(err)
	}
	show("DecodeFormString", jsonOf(decoded))
	user, _ := decoded["user"].(map[string]any)
	assertEq("nested scalar", user["name"], "Ada")
	addr, _ := user["address"].(map[string]any)
	assertEq("deeply nested scalar", addr["city"], "Bath")
	assertEq("list accumulation", decoded["tags"], []string{"a", "b"})
	assertEq("flat key", decoded["flat"], "1")

	vals := url.Values{"a": {"1", "2"}}
	assertEq("repeated scalar: last wins", liveview.DecodeForm(vals)["a"], "2")
	_, err = liveview.DecodeFormString("%zz")
	assert("DecodeFormString rejects bad query", err != nil, err)

	form := liveview.NewForm("user", user)
	assertEq("Form.GetString", form.GetString("name"), "Ada")
	assert("Form.Get non-string", form.Get("address") != nil)
	assertEq("Form.InputName", form.InputName("email"), "user[email]")
	assertEq("bare InputName", liveview.NewForm("", nil).InputName("q"), "q")
	assert("valid before errors", form.Valid())
	form.AddError("email", "can't be blank")
	form.AddError("email", "must be an email")
	assertEq("Errors in insertion order", form.Errors("email"), []string{"can't be blank", "must be an email"})
	assert("HasErrors / !Valid", form.HasErrors() && !form.Valid())
	assert("Errors for clean field is nil", form.Errors("name") == nil)

	assertEq("ClassList sorted, only true", liveview.ClassList(map[string]bool{"z": true, "a": true, "no": false, "": true}), "a z")
	assertEq("AttrList", string(liveview.AttrList(map[string]any{"b": 2, "a": "x<y", "on": true, "off": false})),
		` a="x&lt;y" b="2" on`)
	assertEq("HiddenInputs", string(liveview.HiddenInputs(map[string]string{"_csrf": "t&k"})),
		`<input type="hidden" name="_csrf" value="t&amp;k">`)
	show("LivePatch", string(liveview.LivePatch("Next", "/p?x=1&y=2")))
	assert("LivePatch marks link kind", strings.Contains(string(liveview.LivePatch("Next", "/p")), `data-phx-link="patch"`))
	assert("LiveNavigate marks redirect", strings.Contains(string(liveview.LiveNavigate("Away", "/o")), `data-phx-link="redirect"`))
}

func httpTransport() {
	section("HTTP transport (httptest)")

	h := liveview.NewHandler("/", func() liveview.View { return Dashboard{} })
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/?title=FromQuery")
	if err != nil {
		panic(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	page := string(body)
	assertEq("GET status", resp.StatusCode, 200)
	assert("page has lv-root", strings.Contains(page, `id="lv-root"`))
	assert("query params reached Mount", strings.Contains(page, "FromQuery"))
	assert("inline JS client served", strings.Contains(page, "<script>") && strings.Contains(page, "WS_PATH"))

	sid := regexp.MustCompile(`data-session="([^"]+)"`).FindStringSubmatch(page)
	assert("session id embedded in page", len(sid) == 2)
	sessionID := sid[1]
	assert("Handler.Session lookup", h.Session(sessionID) != nil)
	assert("unknown session is nil", h.Session("nope") == nil)

	postEvent := func(id, event string, payload map[string]any) (int, string) {
		reqBody, _ := json.Marshal(map[string]any{"session": id, "event": event, "payload": payload})
		r, err := http.Post(srv.URL+"/event", "application/json", strings.NewReader(string(reqBody)))
		if err != nil {
			panic(err)
		}
		b, _ := io.ReadAll(r.Body)
		r.Body.Close()
		return r.StatusCode, strings.TrimSpace(string(b))
	}

	code, diff := postEvent(sessionID, "inc", nil)
	show("POST /event -> diff", diff)
	assertEq("POST status", code, 200)
	assert("diff mentions new count", strings.Contains(diff, `"1"`) || strings.Contains(diff, `1`))
	assert("server-side state advanced", strings.Contains(h.Session(sessionID).Render().HTML(), `<p class="count">1</p>`))
	// HOLE (observed): the GET page path never marks component renders as
	// "sent" (only Session.InitialDiff does), so the first diff over the HTTP
	// event route redundantly re-ships every component's statics even though
	// nothing about the component changed.
	assert("first POST re-sends component statics (bug)", strings.Contains(diff, `"c":{"1":{`) && strings.Contains(diff, `"s":[`))
	_, diff2 := postEvent(sessionID, "inc", nil)
	show("second POST /event -> diff", diff2)
	assert("second POST diff is clean", !strings.Contains(diff2, `"c":`))

	code, _ = postEvent("bogus-session", "inc", nil)
	assertEq("unknown session -> 404", code, 404)

	r, err := http.Post(srv.URL+"/event", "application/json", strings.NewReader("{not json"))
	if err != nil {
		panic(err)
	}
	r.Body.Close()
	assertEq("bad JSON -> 400", r.StatusCode, 400)

	// PubSub reaches every session mounted by the handler.
	assertEq("Handler.PubSub non-nil", h.PubSub() != nil, true)
	assertEq("AcceptKey (RFC 6455 example)",
		liveview.AcceptKey("dGhlIHNhbXBsZSBub25jZQ=="), "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=")
}

func websocketTransport() {
	section("WebSocket transport (hand-rolled client)")

	h := liveview.NewHandler("/", func() liveview.View { return Dashboard{} })
	srv := httptest.NewServer(h)
	defer srv.Close()

	conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	// Deadlines guarantee this section cannot block forever.
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	var nonce [16]byte
	_, _ = rand.Read(nonce[:])
	key := base64.StdEncoding.EncodeToString(nonce[:])
	handshake := "GET / HTTP/1.1\r\nHost: " + strings.TrimPrefix(srv.URL, "http://") +
		"\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: " +
		key + "\r\n\r\n"
	if _, err := conn.Write([]byte(handshake)); err != nil {
		panic(err)
	}

	br := bufio.NewReader(conn)
	res, err := http.ReadResponse(br, nil)
	if err != nil {
		panic(err)
	}
	assertEq("upgrade status", res.StatusCode, 101)
	assertEq("Sec-WebSocket-Accept matches AcceptKey",
		res.Header.Get("Sec-WebSocket-Accept"), liveview.AcceptKey(key))

	mount := readWSText(br)
	show("ws mount frame", mount)
	assert("mount frame typed", strings.Contains(mount, `"type":"mount"`))
	assert("mount frame carries statics", strings.Contains(mount, `"s":[`))

	writeWSText(conn, `{"type":"event","event":"inc"}`)
	show("ws diff frame", readWSText(br))

	// Component-targeted event by cid.
	writeWSText(conn, `{"type":"event","cid":1,"event":"toggle"}`)
	cdiff := readWSText(br)
	show("ws component diff", cdiff)
	assert("component diff over socket", strings.Contains(cdiff, `"c":{"1"`))

	// Live patch over the socket.
	writeWSText(conn, `{"type":"patch","uri":"/dash?filter=starred"}`)
	pdiff := readWSText(br)
	show("ws patch diff", pdiff)
	assert("patch diff contains new filter", strings.Contains(pdiff, "starred"))

	// Upload over the socket.
	writeWSText(conn, `{"type":"upload_start","name":"avatar","ref":"r1","file_name":"a.png","size":4,"mime":"image/png"}`)
	show("ws upload_start reply", readWSText(br))
	writeWSText(conn, `{"type":"upload_chunk","name":"avatar","ref":"r1","data":"`+
		base64.StdEncoding.EncodeToString([]byte("abcd"))+`","last":true}`)
	show("ws upload_chunk reply", readWSText(br))

	// An unrecognised message type gets no reply, so probe with a valid one after.
	writeWSText(conn, `{"type":"totally-unknown"}`)
	writeWSText(conn, `{"type":"event","event":"inc"}`)
	assert("unknown type ignored, stream still sane",
		strings.Contains(readWSText(br), `"type":"diff"`))

	assert("client-side ping answered", pingPong(conn, br))
	writeWSClose(conn)
	fmt.Println("  (socket closed cleanly)")
}

// ---------------------------------------------------------- ws client frames

func writeWSFrame(w io.Writer, opcode byte, payload []byte) {
	var mask [4]byte
	_, _ = rand.Read(mask[:])
	hdr := []byte{0x80 | opcode}
	n := len(payload)
	switch {
	case n <= 125:
		hdr = append(hdr, byte(0x80|n))
	case n <= 0xFFFF:
		ext := make([]byte, 2)
		binary.BigEndian.PutUint16(ext, uint16(n))
		hdr = append(hdr, 0x80|126)
		hdr = append(hdr, ext...)
	default:
		ext := make([]byte, 8)
		binary.BigEndian.PutUint64(ext, uint64(n))
		hdr = append(hdr, 0x80|127)
		hdr = append(hdr, ext...)
	}
	hdr = append(hdr, mask[:]...)
	masked := make([]byte, n)
	for i := range payload {
		masked[i] = payload[i] ^ mask[i%4]
	}
	if _, err := w.Write(append(hdr, masked...)); err != nil {
		panic(err)
	}
}

func writeWSText(w io.Writer, s string) { writeWSFrame(w, 0x1, []byte(s)) }
func writeWSClose(w io.Writer)          { writeWSFrame(w, 0x8, []byte{0x03, 0xE8}) }
func writeWSPing(w io.Writer, p []byte) { writeWSFrame(w, 0x9, p) }

// readWSFrame reads one server frame (server frames are never masked).
func readWSFrame(br *bufio.Reader) (opcode byte, payload []byte) {
	var hdr [2]byte
	if _, err := io.ReadFull(br, hdr[:]); err != nil {
		panic(err)
	}
	opcode = hdr[0] & 0x0F
	length := uint64(hdr[1] & 0x7F)
	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(br, ext[:]); err != nil {
			panic(err)
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(br, ext[:]); err != nil {
			panic(err)
		}
		length = binary.BigEndian.Uint64(ext[:])
	}
	payload = make([]byte, length)
	if _, err := io.ReadFull(br, payload); err != nil {
		panic(err)
	}
	return opcode, payload
}

func readWSText(br *bufio.Reader) string {
	for {
		op, payload := readWSFrame(br)
		if op == 0x1 || op == 0x2 {
			return string(payload)
		}
	}
}

func pingPong(conn net.Conn, br *bufio.Reader) bool {
	writeWSPing(conn, []byte("hi"))
	for i := 0; i < 4; i++ {
		op, payload := readWSFrame(br)
		if op == 0xA {
			return string(payload) == "hi"
		}
	}
	return false
}
