// Command run is the Go side of the express/ms nested parity harness. It speaks
// the same JSON Lines protocol as node/run.js (see parity/HARNESS.md) against
// github.com/malcolmston/express/ms.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/malcolmston/express/ms"
)

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	// Not omitempty: a legitimate zero value (0 ms, "") must still be reported.
	Value any    `json:"value"`
	Error string `json:"error,omitempty"`
}

func argString(args []json.RawMessage, i int) (string, error) {
	if i >= len(args) {
		return "", fmt.Errorf("missing argument %d", i)
	}
	var s string
	if err := json.Unmarshal(args[i], &s); err != nil {
		return "", fmt.Errorf("argument %d is not a string", i)
	}
	return s, nil
}

func argMillis(args []json.RawMessage, i int) (time.Duration, error) {
	if i >= len(args) {
		return 0, fmt.Errorf("missing argument %d", i)
	}
	var f float64
	if err := json.Unmarshal(args[i], &f); err != nil {
		return 0, fmt.Errorf("argument %d is not a number", i)
	}
	// Cases carry milliseconds; time.Duration is nanoseconds. Round so that a
	// value such as 100.5 ms lands on exactly 100500000 ns.
	return time.Duration(math.Round(f * float64(time.Millisecond))), nil
}

// millis renders a duration back into the milliseconds unit the cases use.
func millis(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

func dispatch(req request) (any, error) {
	switch req.Fn {
	// parse, parseStrict and the string arm of the dual-dispatch default export
	// are all one function in Go: there is no compile-time StringValue type to
	// distinguish, and no dynamic dispatch on the argument type.
	case "parse", "parseStrict", "msString":
		s, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		d, err := ms.Parse(s)
		if err != nil {
			return nil, err
		}
		return millis(d), nil

	case "format", "msNumber":
		d, err := argMillis(req.Args, 0)
		if err != nil {
			return nil, err
		}
		return ms.Format(d), nil

	case "formatLong", "msNumberLong":
		d, err := argMillis(req.Args, 0)
		if err != nil {
			return nil, err
		}
		return ms.FormatLong(d), nil

	case "roundtrip":
		s, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		d, err := ms.Parse(s)
		if err != nil {
			return nil, err
		}
		return ms.Format(d), nil

	case "roundtripLong":
		s, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		d, err := ms.Parse(s)
		if err != nil {
			return nil, err
		}
		return ms.FormatLong(d), nil

	default:
		return nil, fmt.Errorf("unknown fn: %s", req.Fn)
	}
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 1<<24)
	out := bufio.NewWriter(os.Stdout)
	enc := json.NewEncoder(out)

	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = enc.Encode(response{OK: false, Error: "malformed request: " + err.Error()})
			_ = out.Flush()
			continue
		}
		resp := response{ID: req.ID}
		if v, err := dispatch(req); err != nil {
			resp.OK = false
			resp.Error = err.Error()
		} else {
			resp.OK = true
			resp.Value = v
		}
		if err := enc.Encode(resp); err != nil {
			fmt.Fprintln(os.Stderr, "encode:", err)
		}
		// Flush after every line or the harness deadlocks waiting for a reply.
		if err := out.Flush(); err != nil {
			fmt.Fprintln(os.Stderr, "flush:", err)
		}
	}
	if err := in.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
	}
}
