// Go runner for the express/filesize nested parity harness.
//
// Same JSON Lines protocol as ../node/run.js: one request per line in, one reply
// per line out, flushed each time, never exiting on a failing case.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/malcolmston/express/filesize"
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

// jsOptions is the presence-aware decode of a case's options object, mirroring
// filesize's destructuring defaults.
type jsOptions struct {
	Base     *int    `json:"base"`
	Round    *int    `json:"round"`
	Standard *string `json:"standard"`
}

// argNumber decodes the byte count. Cases pass non-finite values as the strings
// "NaN", "Infinity" and "-Infinity" because JSON has no literal for them;
// upstream rejects all three with a TypeError.
func argNumber(raw json.RawMessage) (float64, error) {
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return f, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		switch s {
		case "NaN":
			return math.NaN(), nil
		case "Infinity":
			return math.Inf(1), nil
		case "-Infinity":
			return math.Inf(-1), nil
		}
	}
	return 0, fmt.Errorf("Invalid number")
}

func toGoOptions(raw json.RawMessage) (filesize.Options, error) {
	var o filesize.Options
	if len(raw) == 0 || string(raw) == "null" {
		return o, nil
	}
	var js jsOptions
	if err := json.Unmarshal(raw, &js); err != nil {
		return o, fmt.Errorf("bad options: %w", err)
	}
	if js.Base != nil {
		o.Base = *js.Base
	}
	if js.Round != nil {
		r := *js.Round
		o.Round = &r
	}
	if js.Standard != nil {
		o.Standard = *js.Standard
	}
	return o, nil
}

// call applies the port, translating upstream's TypeError on a non-finite input
// into a failed case.
func call(n float64, o filesize.Options, withOpts bool) (any, error) {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return nil, fmt.Errorf("Invalid number")
	}
	if withOpts {
		return filesize.FileSizeOpts(n, o), nil
	}
	return filesize.FileSize(n), nil
}

func dispatch(fn string, args []json.RawMessage) (any, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("%s: missing argument", fn)
	}
	n, err := argNumber(args[0])
	if err != nil {
		return nil, err
	}
	switch fn {
	case "filesize":
		return call(n, filesize.Options{}, false)
	case "filesizeOpts":
		var raw json.RawMessage
		if len(args) > 1 {
			raw = args[1]
		}
		o, err := toGoOptions(raw)
		if err != nil {
			return nil, err
		}
		return call(n, o, true)
	default:
		return nil, fmt.Errorf("unknown fn: %s", fn)
	}
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	out := bufio.NewWriter(os.Stdout)
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			fmt.Fprintf(os.Stderr, "bad request line: %v\n", err)
			continue
		}
		resp := response{ID: req.ID}
		func() {
			defer func() {
				if r := recover(); r != nil {
					resp.OK = false
					resp.Error = fmt.Sprintf("panic: %v", r)
				}
			}()
			v, err := dispatch(req.Fn, req.Args)
			if err != nil {
				resp.OK = false
				resp.Error = err.Error()
				return
			}
			resp.OK = true
			resp.Value = v
		}()
		b, err := json.Marshal(resp)
		if err != nil {
			fmt.Fprintf(os.Stderr, "marshal reply: %v\n", err)
			continue
		}
		out.Write(b)
		out.WriteByte('\n')
		out.Flush()
	}
}
