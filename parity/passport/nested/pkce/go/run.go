// Command run is the Go half of parity/passport/nested/pkce: the same JSON Lines
// protocol as node/run.js, answered by github.com/malcolmston/passport/pkce.
//
// Conventions shared with the upstream runner: methods travel in their RFC 7636
// spelling, `verify` always answers a verdict (an unknown method cannot verify),
// and `generatedProperties` compares properties of a freshly generated verifier
// rather than the verifier itself. A failing case is reported as
// {ok:false,error} and never terminates the process.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/malcolmston/passport/pkce"
)

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID string `json:"id"`
	OK bool   `json:"ok"`
	// No omitempty: a legitimate answer may be false or "".
	Value any    `json:"value"`
	Error string `json:"error,omitempty"`
}

func args(a []json.RawMessage, n int) ([]string, error) {
	if len(a) < n {
		return nil, fmt.Errorf("expected %d arguments, got %d", n, len(a))
	}
	out := make([]string, n)
	for i := 0; i < n; i++ {
		if err := json.Unmarshal(a[i], &out[i]); err != nil {
			return nil, fmt.Errorf("argument %d is not a string: %w", i, err)
		}
	}
	return out, nil
}

func dispatch(req request) (any, error) {
	switch req.Fn {
	case "challenge":
		a, err := args(req.Args, 2)
		if err != nil {
			return nil, err
		}
		return pkce.ComputeChallenge(pkce.Method(a[0]), a[1])

	case "verify":
		a, err := args(req.Args, 3)
		if err != nil {
			return nil, err
		}
		return pkce.Verify(pkce.Method(a[0]), a[1], a[2]), nil

	case "generatedProperties":
		a, err := args(req.Args, 1)
		if err != nil {
			return nil, err
		}
		p, err := pkce.NewWithMethod(pkce.Method(a[0]))
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"verifierLength":          len(p.Verifier),
			"method":                  string(p.Method),
			"roundTrips":              pkce.Verify(p.Method, p.Verifier, p.Challenge),
			"challengeEqualsVerifier": p.Challenge == p.Verifier,
		}, nil

	default:
		return nil, fmt.Errorf("unknown fn %q", req.Fn)
	}
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 1<<24)
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			emit(response{OK: false, Error: "bad request: " + err.Error()})
			continue
		}
		v, err := dispatch(req)
		if err != nil {
			emit(response{ID: req.ID, OK: false, Error: err.Error()})
			continue
		}
		emit(response{ID: req.ID, OK: true, Value: v})
	}
}

func emit(r response) {
	b, err := json.Marshal(r)
	if err != nil {
		b, _ = json.Marshal(response{ID: r.ID, OK: false, Error: "marshal: " + err.Error()})
	}
	_, _ = os.Stdout.Write(append(b, '\n'))
	_ = os.Stdout.Sync()
}
