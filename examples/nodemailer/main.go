// Command nodemailer-example exercises the github.com/malcolmston/nodemailer
// port end to end without touching the public internet: messages are composed,
// rendered to MIME, re-parsed, signed with a locally generated DKIM key, and
// delivered through the in-memory / JSON / stream transports plus an SMTP
// transport pointed at an in-process listener on 127.0.0.1.
package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/malcolmston/nodemailer"
)

func section(title string) {
	fmt.Printf("\n=== %s %s\n", title, strings.Repeat("=", max(0, 62-len(title))))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// fixedDate keeps the MIME output byte-for-byte deterministic.
var fixedDate = time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)

func main() {
	log.SetFlags(0)

	tmp, err := os.MkdirTemp("", "nodemailer-example-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	logoPNG := makePNG()
	reportPath := filepath.Join(tmp, "report.csv")
	if err := os.WriteFile(reportPath, []byte("quarter,revenue\nQ1,1000\nQ2,1500\n"), 0o600); err != nil {
		log.Fatal(err)
	}

	// ------------------------------------------------------------------
	// 1. Address parsing and utilities
	// ------------------------------------------------------------------
	section("1. addresses")

	addr, err := nodemailer.ParseAddress("Ada Lovelace <ada@example.com>")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("ParseAddress   -> name=%q addr=%q local=%q domain=%q\n",
		addr.Name, addr.Address, addr.Local(), addr.Domain())

	list, err := nodemailer.ParseAddressList("grace@example.com, \"Hopper, Grace\" <gh@example.com>")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("ParseAddressList -> %s\n", nodemailer.FormatAddressList(list))

	norm, err := nodemailer.NormalizeAddress("  Ada <ADA@Example.COM>  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("NormalizeAddress -> %q (Equal to parsed? %v)\n", norm,
		addr.Equal(nodemailer.Address{Address: "ada@example.com"}))

	for _, p := range nodemailer.ParseAddresses("Team: a@x.com, b@x.com; solo@y.com") {
		fmt.Printf("ParsedAddr     -> %+v\n", p)
	}

	if _, err := nodemailer.ParseAddress("not-an-address"); err != nil {
		fmt.Printf("invalid address rejected: %v\n", err)
	}

	grp, err := nodemailer.ParseAddressGroup("Engineering: ada@example.com, grace@example.com;")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("ParseAddressGroup -> %s\n", grp.String())

	// ------------------------------------------------------------------
	// 2. Compose a rich message
	// ------------------------------------------------------------------
	section("2. compose")

	html := `<html><body><h1>Quarterly report</h1>` +
		`<p>Revenue is <b>up</b>. Logo: <img src="cid:logo" alt="logo"></p>` +
		`<p><a href="https://example.com/details">Details</a></p></body></html>`

	msg := nodemailer.New().
		SetFrom("Ada Lovelace <ada@example.com>").
		AddTo("Grace Hopper <grace@example.com>", "team@example.com").
		AddCc("Cc Person <cc@example.com>").
		AddBcc("bcc@example.com").
		AddReplyTo("no-reply@example.com").
		AddToGroup("Engineering", "eng1@example.com", "eng2@example.com").
		AddCcGroup("Undisclosed").
		SetSubject("Q1 résumé – naïve façade ✅").
		SetText("Revenue is up.\n\nPlain-text fallback body.\n").
		SetHTML(html).
		SetAMP(`<html amp4email><body><h1>AMP body</h1></body></html>`).
		SetWatchHTML(`<html><body>watch body</body></html>`).
		AddAlternative("text/x-markdown", "# Quarterly report\n\nRevenue is **up**.\n").
		SetPriority(nodemailer.PriorityHigh).
		SetInReplyTo("parent-thread@example.com").
		AddReferences("root@example.com", "parent-thread@example.com").
		AddListHeader("List-ID", "Reports <reports.example.com>").
		SetListUnsubscribe("https://example.com/unsub?u=1", "mailto:unsub@example.com").
		SetListUnsubscribePost().
		AddHeader("X-Campaign-ID", "2026-q1-report").
		AddHeader("X-Mailer", "nodemailer-go-example").
		Embed("logo", "logo.png", "image/png", logoPNG).
		AttachBytes("notes.txt", "text/plain", []byte("free-form notes\n")).
		AttachFile(reportPath).
		AttachReader("données.bin", "", bytes.NewReader([]byte{0x00, 0x01, 0x02, 0xff})).
		SetDate(fixedDate).
		SetMessageID("q1-report@example.com").
		SetBoundary("EXAMPLEBOUNDARY")

	msg.ICalEvent = &nodemailer.ICalEvent{
		Method:   "REQUEST",
		Filename: "review.ics",
		Content: "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\n" +
			"UID:review-1@example.com\r\nDTSTART:20260105T090000Z\r\n" +
			"SUMMARY:Quarterly review\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n",
	}

	if err := msg.Err(); err != nil {
		log.Fatalf("builder error: %v", err)
	}

	fmt.Printf("recipients (SMTP envelope): %v\n", msg.Recipients())

	raw, err := msg.Build()
	if err != nil {
		log.Fatalf("Build: %v", err)
	}
	fmt.Printf("built %d bytes; deterministic re-build identical? ", len(raw))
	raw2, _ := msg.Build()
	fmt.Println(bytes.Equal(raw, raw2))

	// ------------------------------------------------------------------
	// 3. Inspect the rendered MIME
	// ------------------------------------------------------------------
	section("3. rendered MIME (headers + structure)")
	printMIMESkeleton(raw)

	// ------------------------------------------------------------------
	// 4. Round-trip through the parser
	// ------------------------------------------------------------------
	section("4. ParseMIME round trip")

	parsed, err := nodemailer.ParseMIME(raw)
	if err != nil {
		log.Fatalf("ParseMIME: %v", err)
	}
	fmt.Printf("subject      : %s\n", parsed.Subject)
	fmt.Printf("from/to/cc   : %s | %s | %s\n",
		nodemailer.FormatAddressList(parsed.From),
		nodemailer.FormatAddressList(parsed.To),
		nodemailer.FormatAddressList(parsed.Cc))
	fmt.Printf("date         : %s\n", parsed.Date.Format(time.RFC1123Z))
	fmt.Printf("message-id   : %s\n", parsed.MessageID)
	fmt.Printf("text bytes   : %d\nhtml bytes   : %d\n", len(parsed.Text), len(parsed.HTML))
	fmt.Printf("X-Campaign-ID: %s\n", parsed.Get("X-Campaign-ID"))
	fmt.Printf("List-Unsub   : %s\n", parsed.Get("List-Unsubscribe"))
	fmt.Printf("Importance   : %s\n", parsed.Get("Importance"))
	for _, a := range parsed.Attachments {
		fmt.Printf("attachment   : name=%-14q type=%-24s cid=%-6q inline=%-5v bytes=%d\n",
			a.Filename, a.ContentType, a.ContentID, a.Inline, len(a.Content))
	}
	// The inline logo must survive base64 round tripping byte for byte.
	for _, a := range parsed.Attachments {
		if a.ContentID == "logo" {
			fmt.Printf("inline logo identical to source PNG? %v\n", bytes.Equal(a.Content, logoPNG))
		}
	}

	// HOLE: a non-ASCII attachment filename is emitted as a *bare* RFC 2047
	// encoded word ("filename==?utf-8?b?...?="). '?' and '=' are MIME
	// tspecials, so that parameter is not a valid token and mime.ParseMediaType
	// rejects the header — the library's own ParseMIME then silently drops the
	// whole part. Below, the "données.bin" attachment we added above is missing
	// from the 3 parsed attachments. A conforming encoder must either quote the
	// encoded word or use RFC 2231 (filename*=utf-8''...).
	section("4b. HOLE: non-ASCII attachment filename breaks the part")

	nonASCII, err := nodemailer.New().
		SetFrom("ada@example.com").AddTo("grace@example.com").
		SetSubject("filename").SetText("body").
		AttachBytes("données.bin", "application/octet-stream", []byte("payload")).
		SetDate(fixedDate).SetMessageID("f@example.com").SetBoundary("F").
		Build()
	if err != nil {
		log.Fatal(err)
	}
	for _, ln := range strings.Split(string(nonASCII), "\r\n") {
		if strings.HasPrefix(ln, "Content-Disposition") || strings.HasPrefix(ln, "Content-Type: application") {
			fmt.Printf("emitted  : %s\n", ln)
		}
	}
	pn, err := nodemailer.ParseMIME(nonASCII)
	if err != nil {
		fmt.Printf("ParseMIME: %v\n", err)
	} else {
		fmt.Printf("round trip recovered %d attachment(s) (expected 1)\n", len(pn.Attachments))
	}
	// An ASCII filename round-trips correctly, confirming the encoding is the cause.
	asciiOK, _ := nodemailer.New().
		SetFrom("ada@example.com").AddTo("grace@example.com").
		SetSubject("filename").SetText("body").
		AttachBytes("data.bin", "application/octet-stream", []byte("payload")).
		SetDate(fixedDate).SetMessageID("f2@example.com").SetBoundary("F").
		Build()
	pa, _ := nodemailer.ParseMIME(asciiOK)
	fmt.Printf("ASCII filename recovered %d attachment(s)\n", len(pa.Attachments))

	// ------------------------------------------------------------------
	// 5. HTML -> text helpers and low-level codecs
	// ------------------------------------------------------------------
	section("5. html-to-text and codecs")

	fmt.Printf("HTMLToText            : %q\n", nodemailer.HTMLToText(html))
	auto := nodemailer.New().
		SetFrom("ada@example.com").AddTo("grace@example.com").
		SetSubject("auto").SetHTML("<p>Hello <b>world</b></p>").
		GenerateTextFromHTML()
	fmt.Printf("GenerateTextFromHTML  : %q\n", auto.Text)
	fmt.Printf("QPEncode              : %q\n", nodemailer.QPEncode([]byte("naïve = façade")))
	fmt.Printf("Base64Encode          : %q\n", nodemailer.Base64Encode([]byte("hello world")))
	fmt.Printf("EncodeWord            : %s\n", nodemailer.EncodeWord("utf-8", "résumé"))
	dec, _ := nodemailer.DecodeHeaderWord(nodemailer.EncodeWord("utf-8", "résumé"))
	fmt.Printf("DecodeHeaderWord      : %s\n", dec)
	fmt.Printf("EncodeMimeWord(B)     : %s\n", nodemailer.EncodeMimeWord([]byte("héllo"), "B"))
	fmt.Printf("IsPlainText(\"héllo\")  : %v\n", nodemailer.IsPlainText("héllo"))
	fmt.Printf("HasLongerLines(80)    : %v\n", nodemailer.HasLongerLines(strings.Repeat("x", 100), 80))
	fmt.Printf("QPWrap                : %q\n", nodemailer.QPWrap(nodemailer.QPEncode([]byte(strings.Repeat("ü", 40))), 40))
	fmt.Printf("GenerateMessageID     : %s\n", nodemailer.GenerateMessageID("example.com"))

	// ------------------------------------------------------------------
	// 6. DKIM signing with a locally generated key
	// ------------------------------------------------------------------
	section("6. DKIM")

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	reparsed, err := nodemailer.ParseRSAPrivateKey(pemBytes)
	if err != nil {
		log.Fatalf("ParseRSAPrivateKey: %v", err)
	}

	dkim := &nodemailer.DKIM{
		Domain:      "example.com",
		Selector:    "mail",
		PrivateKey:  reparsed,
		HeaderCanon: nodemailer.CanonRelaxed,
		BodyCanon:   nodemailer.CanonRelaxed,
		Time:        fixedDate,
	}
	signed, err := nodemailer.New().
		SetFrom("ada@example.com").AddTo("grace@example.com").
		SetSubject("Signed").SetText("body").SetHTML("<p>body</p>").
		SetDate(fixedDate).SetMessageID("signed@example.com").SetBoundary("B").
		WithDKIM(dkim).
		Build()
	if err != nil {
		log.Fatalf("DKIM build: %v", err)
	}
	firstLine := strings.SplitN(string(signed), "\r\n", 2)[0]
	fmt.Printf("first header : %s...\n", truncate(firstLine, 90))

	sig, err := dkim.Sign(signed)
	if err != nil {
		log.Fatalf("DKIM Sign: %v", err)
	}
	fmt.Printf("Sign() tags  : %s\n", truncate(sig, 120))

	name, err := dkim.DNSRecordName()
	if err != nil {
		log.Fatal(err)
	}
	rec, err := dkim.DNSRecord()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("DNS name     : %s\n", name)
	fmt.Printf("DNS record   : %s\n", truncate(rec, 100))

	// A missing key must be reported, not silently skipped.
	if _, err := (&nodemailer.DKIM{Domain: "example.com"}).Sign(signed); err != nil {
		fmt.Printf("incomplete DKIM rejected: %v\n", err)
	}

	// ------------------------------------------------------------------
	// 7. Non-network transports
	// ------------------------------------------------------------------
	section("7. memory / JSON / stream transports")

	mem := &nodemailer.MemoryTransport{}
	info, err := nodemailer.NewTransporter(mem).SendMail(msg)
	if err != nil {
		log.Fatalf("memory send: %v", err)
	}
	captured, _ := mem.Last()
	fmt.Printf("MemoryTransport : message-id=%s envelope.from=%s rcpts=%d raw=%d bytes\n",
		info.MessageID, info.Envelope.From, len(info.Envelope.To), len(captured.Raw))

	jsonT := &nodemailer.JSONTransport{}
	if _, err := nodemailer.NewTransporter(jsonT).SendMail(msg); err != nil {
		log.Fatalf("json send: %v", err)
	}
	rec2, _ := jsonT.Last()
	var shape struct {
		From string   `json:"from"`
		To   []string `json:"to"`
		Raw  string   `json:"raw"`
	}
	if err := json.Unmarshal(rec2, &shape); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("JSONTransport   : from=%s to=%v base64-raw=%d chars\n", shape.From, shape.To, len(shape.Raw))

	var stream bytes.Buffer
	streamT := nodemailer.NewStreamTransport(&stream)
	for i := 0; i < 2; i++ {
		m := nodemailer.New().
			SetFrom("ada@example.com").
			AddTo(fmt.Sprintf("user%d@example.com", i)).
			SetSubject(fmt.Sprintf("stream %d", i)).
			SetText("hi").
			SetDate(fixedDate).SetMessageID(fmt.Sprintf("s%d@example.com", i))
		if _, err := nodemailer.NewTransporter(streamT).SendMail(m); err != nil {
			log.Fatalf("stream send: %v", err)
		}
	}
	fmt.Printf("StreamTransport : %d bytes containing %d Subject headers\n",
		stream.Len(), strings.Count(stream.String(), "Subject: stream"))

	// ------------------------------------------------------------------
	// 8. SMTP against an in-process listener (127.0.0.1, no external server)
	// ------------------------------------------------------------------
	section("8. SMTP transport, DSN and pooling (in-process server)")

	srv := startSMTPServer()
	defer srv.close()

	smtpT := &nodemailer.SMTPTransport{
		Host:      srv.host,
		Port:      srv.port,
		LocalName: "example.local",
		DSN: &nodemailer.DSNOptions{
			Return:        nodemailer.DSNReturnHeaders,
			EnvID:         "envid-42",
			Notify:        []nodemailer.DSNNotify{nodemailer.DSNNotifySuccess, nodemailer.DSNNotifyFailure},
			OrigRecipient: "grace@example.com",
		},
	}
	if err := smtpT.Verify(); err != nil {
		log.Fatalf("Verify: %v", err)
	}
	fmt.Println("SMTPTransport.Verify() ok (dial + EHLO, no send)")

	info, err = nodemailer.NewTransporter(smtpT).SendMail(msg)
	if err != nil {
		log.Fatalf("smtp send: %v", err)
	}
	fmt.Printf("delivered %d bytes to %d recipients\n", len(info.Raw), len(info.Envelope.To))
	fmt.Printf("server saw MAIL: %s\n", srv.lastMail())
	fmt.Printf("server saw RCPT: %s\n", srv.lastRcpt())

	pool := &nodemailer.Pool{Transport: smtpT, MaxConnections: 2, MaxMessages: 2}
	if err := pool.Verify(); err != nil {
		log.Fatalf("pool verify: %v", err)
	}
	for i := 0; i < 5; i++ {
		m := nodemailer.New().
			SetFrom("ada@example.com").AddTo("grace@example.com").
			SetSubject(fmt.Sprintf("pooled %d", i)).SetText("hi").
			SetDate(fixedDate).SetMessageID(fmt.Sprintf("p%d@example.com", i))
		if _, err := nodemailer.NewTransporter(pool).SendMail(m); err != nil {
			log.Fatalf("pooled send %d: %v", i, err)
		}
	}
	if err := pool.Close(); err != nil {
		log.Fatalf("pool close: %v", err)
	}
	fmt.Printf("pool sent 5 messages; server accepted %d total, over %d connections\n",
		srv.messageCount(), srv.connCount())
	if _, err := nodemailer.NewTransporter(pool).SendMail(msg); err != nil {
		fmt.Printf("send after Close rejected: %v\n", err)
	}

	// ------------------------------------------------------------------
	// 9. Well-known services and the sendmail transport
	// ------------------------------------------------------------------
	section("9. well-known services + sendmail")

	fmt.Printf("known services: %s\n", strings.Join(nodemailer.WellKnownServiceNames(), ", "))
	if svc, ok := nodemailer.WellKnownService("GMail"); ok {
		fmt.Printf("Gmail -> host=%s port=%d secure=%v\n", svc.Host, svc.Port, svc.Secure)
	}
	svcT, err := nodemailer.NewServiceSMTP("postmark", "token", "token")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("NewServiceSMTP(postmark) -> %s:%d TLS=%v STARTTLS=%v\n",
		svcT.Host, svcT.Port, svcT.TLS, svcT.STARTTLS)
	if _, err := nodemailer.NewServiceSMTP("nope", "u", "p"); err != nil {
		fmt.Printf("unknown service rejected: %v\n", err)
	}

	// XOAUTH2 credential encoding (no network: just the SASL payload).
	fmt.Printf("XOAuth2Token  : %s\n", truncate(nodemailer.XOAuth2Token("ada@example.com", "ya29.token"), 80))
	fmt.Printf("XOAuth2Auth   : %T\n", nodemailer.XOAuth2Auth("ada@example.com", "ya29.token"))

	// /usr/bin/true stands in for a local sendmail binary: it exercises the
	// exec + stdin plumbing without delivering anything anywhere.
	if _, err := os.Stat("/usr/bin/true"); err == nil {
		sm := &nodemailer.SendmailTransport{Path: "/usr/bin/true", UseHeaders: true}
		if _, err := nodemailer.NewTransporter(sm).SendMail(msg); err != nil {
			log.Fatalf("sendmail: %v", err)
		}
		fmt.Println("SendmailTransport piped the message to a local no-op binary ok")
	}
	bad := &nodemailer.SendmailTransport{Path: filepath.Join(tmp, "does-not-exist")}
	if _, err := nodemailer.NewTransporter(bad).SendMail(msg); err != nil {
		fmt.Printf("missing sendmail binary reported: %v\n", truncateErr(err))
	}

	// ------------------------------------------------------------------
	// 10. Error handling: deferred builder errors
	// ------------------------------------------------------------------
	section("10. deferred validation errors")

	broken := nodemailer.New().SetFrom("ada@example.com").AddTo("bad@@address").SetText("x")
	if _, err := broken.Build(); err != nil {
		fmt.Printf("Build surfaced the deferred setter error: %v\n", err)
	}
	if err := nodemailer.New().AddTo("who@example.com").Err(); err == nil {
		fmt.Println("NOTE: a message with no From is accepted by the builder (Err() == nil)")
	}

	fmt.Println("\nall nodemailer example steps completed")
}

// ----------------------------------------------------------------------
// helpers
// ----------------------------------------------------------------------

// printMIMESkeleton prints the top-level headers and then a one-line summary of
// every MIME boundary / Content-* header so the structure is visible at a glance.
func printMIMESkeleton(raw []byte) {
	lines := strings.Split(string(raw), "\r\n")
	inHeaders := true
	// stack holds the currently open boundaries so sibling delimiters of the
	// same boundary stay at the same indent level.
	var stack []string
	for _, ln := range lines {
		if inHeaders {
			if ln == "" {
				inHeaders = false
				fmt.Println("--- body ---")
				continue
			}
			fmt.Println(truncate(ln, 100))
			continue
		}
		switch {
		case strings.HasPrefix(ln, "--") && strings.HasSuffix(ln, "--") && len(ln) > 4:
			b := strings.TrimSuffix(strings.TrimPrefix(ln, "--"), "--")
			if len(stack) > 0 && stack[len(stack)-1] == b {
				fmt.Printf("%sclose    %s\n", indent(len(stack)), ln)
				stack = stack[:len(stack)-1]
			}
		case strings.HasPrefix(ln, "--"):
			b := strings.TrimPrefix(ln, "--")
			if len(stack) == 0 || stack[len(stack)-1] != b {
				stack = append(stack, b)
			}
			fmt.Printf("%spart in %s\n", indent(len(stack)), ln)
		case strings.HasPrefix(ln, "Content-"):
			fmt.Printf("%s  %s\n", indent(len(stack)), truncate(ln, 96))
		}
	}
}

func indent(n int) string { return strings.Repeat("  ", n) }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func truncateErr(err error) string {
	return truncate(err.Error(), 90)
}

// makePNG renders a tiny PNG in memory so the example needs no asset files.
func makePNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 32), G: uint8(y * 32), B: 0x80, A: 0xff})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		log.Fatal(err)
	}
	return buf.Bytes()
}
