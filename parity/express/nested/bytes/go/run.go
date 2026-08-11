// Command run is the Go side of the express/bytes nested parity harness. It
// speaks the same JSON Lines protocol as node/run.js (see parity/HARNESS.md)
// against github.com/malcolmston/express/bytes.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	gobytes "github.com/malcolmston/express/bytes"
)

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID string `json:"id"`
	OK bool   `json:"ok"`
	// Not omitempty: a legitimate zero value (0, "") must still be reported.
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

// argCount decodes a byte count. The port's currency is int64, so a fractional
// or out-of-range JSON number has no representation and is reported as a
// failure rather than silently truncated.
func argCount(args []json.RawMessage, i int) (int64, error) {
	if i >= len(args) {
		return 0, fmt.Errorf("missing argument %d", i)
	}
	var num json.Number
	if err := json.Unmarshal(args[i], &num); err != nil {
		return 0, fmt.Errorf("argument %d is not a number", i)
	}
	n, err := strconv.ParseInt(num.String(), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("argument %d (%s) is not an int64 byte count: %w", i, num.String(), err)
	}
	return n, nil
}

// jsonOptions mirrors the npm options object. DecimalPlaces is a pointer so an
// absent key keeps the port's default of 2, exactly as `options.decimalPlaces
// !== undefined` does upstream.
type jsonOptions struct {
	DecimalPlaces      *int   `json:"decimalPlaces"`
	FixedDecimals      bool   `json:"fixedDecimals"`
	ThousandsSeparator string `json:"thousandsSeparator"`
	UnitSeparator      string `json:"unitSeparator"`
	Unit               string `json:"unit"`
}

func argOptions(args []json.RawMessage, i int) (gobytes.FormatOptions, error) {
	if i >= len(args) {
		return gobytes.FormatOptions{}, nil
	}
	var o jsonOptions
	if err := json.Unmarshal(args[i], &o); err != nil {
		return gobytes.FormatOptions{}, fmt.Errorf("argument %d is not an options object: %w", i, err)
	}
	return gobytes.FormatOptions{
		DecimalPlaces:      o.DecimalPlaces,
		FixedDecimals:      o.FixedDecimals,
		ThousandsSeparator: o.ThousandsSeparator,
		UnitSeparator:      o.UnitSeparator,
		Unit:               o.Unit,
	}, nil
}

func dispatch(req request) (any, error) {
	switch req.Fn {
	// parse and the string arm of the dual-dispatch default export are one
	// function in Go: there is no runtime dispatch on the argument type.
	case "parse", "bytesString":
		s, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		n, err := gobytes.Parse(s)
		if err != nil {
			return nil, err
		}
		return n, nil

	// bytes.parse(number) is the identity; the Go port's Parse is string-typed
	// and has no such entry point at all.
	case "parseNum", "parseNaN", "bytesOther":
		return nil, fmt.Errorf("bytes.Parse takes a string: no Go entry point for fn %q", req.Fn)

	case "format", "bytesNumber":
		n, err := argCount(req.Args, 0)
		if err != nil {
			return nil, err
		}
		return gobytes.Format(n), nil

	case "formatOpts", "bytesNumberOpts":
		n, err := argCount(req.Args, 0)
		if err != nil {
			return nil, err
		}
		opts, err := argOptions(req.Args, 1)
		if err != nil {
			return nil, err
		}
		return gobytes.FormatOpts(n, opts), nil

	case "roundtrip":
		s, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		n, err := gobytes.Parse(s)
		if err != nil {
			return nil, err
		}
		return gobytes.Format(n), nil

	case "roundtripParse":
		n, err := argCount(req.Args, 0)
		if err != nil {
			return nil, err
		}
		got, err := gobytes.Parse(gobytes.Format(n))
		if err != nil {
			return nil, err
		}
		return got, nil

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
