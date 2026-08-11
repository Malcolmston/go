// Command run is the Go side of the express/accepts nested parity harness.
//
// It speaks the same JSON-Lines contract as node/run.js (see
// parity/HARNESS.md): one request per line on stdin, exactly one reply per line
// on stdout, flushed immediately, never exiting on a failing case.
//
// Representation notes, applied identically by the Node runner:
//   - the Go API reports "nothing acceptable" with the "" sentinel where
//     upstream returns `false`; both are emitted as JSON null.
//   - an absent header is modelled by omitting the key from the case's headers
//     object, so http.Header.Values reports it as absent (which the port
//     distinguishes from a present-but-empty header, as upstream does).
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/malcolmston/express/accepts"
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

// headerFrom builds an http.Header from the case's headers object. Keys present
// with a null value are skipped so that "absent" is expressible.
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

func offersFrom(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("offers: %w", err)
	}
	return out, nil
}

// orNil maps the Go "" miss sentinel onto JSON null, matching the Node runner's
// treatment of upstream's `false`.
func orNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// list normalises a nil slice to an empty JSON array so that both runners agree
// on "no acceptable values".
func list(v []string) any {
	if v == nil {
		return []string{}
	}
	return v
}

func dispatch(req request) (any, error) {
	var headersRaw, offersRaw json.RawMessage
	if len(req.Args) > 0 {
		headersRaw = req.Args[0]
	}
	if len(req.Args) > 1 {
		offersRaw = req.Args[1]
	}
	hdr, err := headerFrom(headersRaw)
	if err != nil {
		return nil, err
	}
	offers, err := offersFrom(offersRaw)
	if err != nil {
		return nil, err
	}
	a := accepts.New(hdr)

	switch req.Fn {
	case "type":
		return orNil(a.Type(offers...)), nil
	case "types":
		return list(a.Types()), nil
	case "language":
		return orNil(a.Language(offers...)), nil
	case "languages":
		return list(a.Languages()), nil
	case "charset":
		return orNil(a.Charset(offers...)), nil
	case "charsets":
		return list(a.Charsets()), nil
	case "encoding":
		return orNil(a.Encoding(offers...)), nil
	case "encodings":
		return list(a.Encodings()), nil
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
		resp := answer(req)
		_ = enc.Encode(resp)
		// Flush per line or the harness deadlocks waiting for the reply.
		_ = out.Flush()
	}
}

// answer runs one case, converting a panic into an ok:false reply so that a
// single bad case never takes the runner down.
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
