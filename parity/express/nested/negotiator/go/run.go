// Command run is the Go side of the express/negotiator nested parity harness.
//
// Same JSON-Lines contract as node/run.js (see parity/HARNESS.md): one request
// per line on stdin, one reply per line on stdout, flushed per line, never
// exiting on a failing case.
//
// Representation notes, applied identically by the Node runner:
//   - the singular accessors report "nothing acceptable" with "" where upstream
//     returns undefined; both are emitted as JSON null.
//   - a null `available` argument means "call with no argument" (the full sorted
//     list); the variadic Go form receives a nil slice.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/malcolmston/express/negotiator"
)

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value any    `json:"value"`
	Error string `json:"error,omitempty"`
}

func headerFrom(raw json.RawMessage) (http.Header, error) {
	h := http.Header{}
	if len(raw) == 0 || string(raw) == "null" {
		return h, nil
	}
	var m map[string]*string
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("headers: %w", err)
	}
	for k, v := range m {
		if v == nil {
			continue
		}
		h.Set(k, *v)
	}
	return h, nil
}

func availFrom(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("available: %w", err)
	}
	return out, nil
}

func orNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func list(v []string) any {
	if v == nil {
		return []string{}
	}
	return v
}

func dispatch(req request) (any, error) {
	var headersRaw, availRaw json.RawMessage
	if len(req.Args) > 0 {
		headersRaw = req.Args[0]
	}
	if len(req.Args) > 1 {
		availRaw = req.Args[1]
	}
	hdr, err := headerFrom(headersRaw)
	if err != nil {
		return nil, err
	}
	available, err := availFrom(availRaw)
	if err != nil {
		return nil, err
	}
	n := negotiator.New(hdr)

	switch req.Fn {
	case "mediaType", "preferredMediaType":
		return orNil(n.MediaType(available...)), nil
	case "mediaTypes", "preferredMediaTypes":
		return list(n.MediaTypes(available...)), nil
	case "language", "preferredLanguage":
		return orNil(n.Language(available...)), nil
	case "languages", "preferredLanguages":
		return list(n.Languages(available...)), nil
	case "charset", "preferredCharset":
		return orNil(n.Charset(available...)), nil
	case "charsets", "preferredCharsets":
		return list(n.Charsets(available...)), nil
	case "encoding", "preferredEncoding":
		return orNil(n.Encoding(available...)), nil
	case "encodings", "preferredEncodings":
		return list(n.Encodings(available...)), nil
	}
	return nil, fmt.Errorf("unknown fn: %s", req.Fn)
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	out := bufio.NewWriter(os.Stdout)
	enc := json.NewEncoder(out)

	for in.Scan() {
		line := in.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			_ = enc.Encode(response{OK: false, Error: "bad request line: " + err.Error()})
			_ = out.Flush()
			continue
		}
		_ = enc.Encode(answer(req))
		// Flush per line or the harness deadlocks waiting for the reply.
		_ = out.Flush()
	}
}

func answer(req request) (resp response) {
	defer func() {
		if r := recover(); r != nil {
			resp = response{ID: req.ID, OK: false, Error: fmt.Sprintf("panic: %v", r)}
		}
	}()
	v, err := dispatch(req)
	if err != nil {
		return response{ID: req.ID, OK: false, Error: err.Error()}
	}
	return response{ID: req.ID, OK: true, Value: v}
}
