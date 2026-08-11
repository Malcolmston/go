// Go runner for the express/nanoid nested parity harness.
//
// Same JSON Lines protocol as ../node/run.js: one request per line in, one reply
// per line out, flushed each time, never exiting on a failing case.
//
// Generated ids are random, so nothing here returns one: every fn returns a
// deterministic property of generation (alphabet, length, character membership,
// observed character set, distinctness, rejection of an invalid size).
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/malcolmston/express/nanoid"
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

func argInt(raw json.RawMessage) (int, error) {
	var n int
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, fmt.Errorf("expected an integer: %w", err)
	}
	return n, nil
}

func argString(raw json.RawMessage) (string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", fmt.Errorf("expected a string: %w", err)
	}
	return s, nil
}

// inSet reports whether every character of id belongs to alphabet. Lengths and
// character sets are compared as code points, not UTF-8 bytes or UTF-16 units,
// so a non-ASCII alphabet means the same thing on both sides.
func inSet(id, alphabet string) bool {
	set := map[rune]bool{}
	for _, r := range alphabet {
		set[r] = true
	}
	for _, r := range id {
		if !set[r] {
			return false
		}
	}
	return true
}

// distinctChars returns the sorted, de-duplicated characters of s.
func distinctChars(s string) string {
	set := map[rune]bool{}
	for _, r := range s {
		set[r] = true
	}
	out := make([]rune, 0, len(set))
	for r := range set {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return string(out)
}

func dispatch(fn string, args []json.RawMessage) (any, error) {
	switch fn {
	case "urlAlphabet":
		return nanoid.DefaultAlphabet, nil
	case "urlAlphabetLength":
		return utf8.RuneCountInString(nanoid.DefaultAlphabet), nil
	case "defaultLength":
		id, err := nanoid.New()
		if err != nil {
			return nil, err
		}
		return utf8.RuneCountInString(id), nil
	case "length", "inAlphabet", "negative":
		size, err := argInt(args[0])
		if err != nil {
			return nil, err
		}
		id, err := nanoid.NewSize(size)
		if err != nil {
			return nil, err
		}
		switch fn {
		case "length":
			return utf8.RuneCountInString(id), nil
		case "inAlphabet":
			return inSet(id, nanoid.DefaultAlphabet), nil
		default: // negative
			return id, nil
		}
	case "zeroId":
		return nanoid.NewSize(0)
	case "distinct":
		size, err := argInt(args[0])
		if err != nil {
			return nil, err
		}
		count, err := argInt(args[1])
		if err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		for i := 0; i < count; i++ {
			id, err := nanoid.NewSize(size)
			if err != nil {
				return nil, err
			}
			seen[id] = true
		}
		return len(seen) == count, nil
	case "customLength", "customInAlphabet", "customNegative":
		alphabet, err := argString(args[0])
		if err != nil {
			return nil, err
		}
		size, err := argInt(args[1])
		if err != nil {
			return nil, err
		}
		id, err := nanoid.Custom(alphabet, size)
		if err != nil {
			return nil, err
		}
		switch fn {
		case "customLength":
			return utf8.RuneCountInString(id), nil
		case "customInAlphabet":
			return inSet(id, alphabet), nil
		default: // customNegative
			return id, nil
		}
	case "customZeroId":
		alphabet, err := argString(args[0])
		if err != nil {
			return nil, err
		}
		return nanoid.Custom(alphabet, 0)
	case "customCharset":
		alphabet, err := argString(args[0])
		if err != nil {
			return nil, err
		}
		size, err := argInt(args[1])
		if err != nil {
			return nil, err
		}
		samples, err := argInt(args[2])
		if err != nil {
			return nil, err
		}
		var seen strings.Builder
		for i := 0; i < samples; i++ {
			id, err := nanoid.Custom(alphabet, size)
			if err != nil {
				return nil, err
			}
			seen.WriteString(id)
		}
		return distinctChars(seen.String()), nil
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
