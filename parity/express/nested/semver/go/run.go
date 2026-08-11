// Command run is the Go side of the express/semver nested parity harness. It
// speaks the same JSON Lines protocol as node/run.js (see parity/HARNESS.md)
// against github.com/malcolmston/express/semver.
//
// The port signals "not a version / no answer" with an error (or a false ok
// return, or a panic from the Must* helpers). node-semver signals it either by
// throwing or by returning null. node/run.js funnels its nulls into throws, and
// this runner reports every one of those situations as ok:false, so the two
// sides agree on what "no answer" means. See the NULL MAPPING note in run.js.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/malcolmston/express/semver"
)

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID string `json:"id"`
	OK bool   `json:"ok"`
	// Not omitempty: a legitimate zero value (0, "", false) must still be reported.
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

func argStrings(args []json.RawMessage, i int) ([]string, error) {
	if i >= len(args) {
		return nil, fmt.Errorf("missing argument %d", i)
	}
	var ss []string
	if err := json.Unmarshal(args[i], &ss); err != nil {
		return nil, fmt.Errorf("argument %d is not an array of strings", i)
	}
	return ss, nil
}

// str1 adapts a one-string-argument function.
func str1[T any](args []json.RawMessage, f func(string) (T, error)) (any, error) {
	s, err := argString(args, 0)
	if err != nil {
		return nil, err
	}
	v, err := f(s)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// str2 adapts a two-string-argument function.
func str2[T any](args []json.RawMessage, f func(string, string) (T, error)) (any, error) {
	a, err := argString(args, 0)
	if err != nil {
		return nil, err
	}
	b, err := argString(args, 1)
	if err != nil {
		return nil, err
	}
	v, err := f(a, b)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// cmp2 adapts the free comparison helpers, which panic on an invalid version.
func cmp2[T any](args []json.RawMessage, f func(string, string) T) (any, error) {
	return str2(args, func(a, b string) (T, error) {
		// Validate first: Compare and its boolean wrappers panic on bad input,
		// and a panic is recovered in dispatch anyway, but checking here keeps
		// the reported error message useful.
		if _, err := semver.Parse(a); err != nil {
			return *new(T), err
		}
		if _, err := semver.Parse(b); err != nil {
			return *new(T), err
		}
		return f(a, b), nil
	})
}

// method2 adapts a *Version method taking another version.
func method2[T any](args []json.RawMessage, f func(*semver.Version, *semver.Version) T) (any, error) {
	return str2(args, func(a, b string) (T, error) {
		av, err := semver.Parse(a)
		if err != nil {
			return *new(T), err
		}
		bv, err := semver.Parse(b)
		if err != nil {
			return *new(T), err
		}
		return f(av, bv), nil
	})
}

func dispatch(req request) (any, error) {
	switch req.Fn {
	// ---------------------------------------------------------------- parsing
	case "valid", "versionString":
		return str1(req.Args, func(s string) (string, error) {
			v, err := semver.Parse(s)
			if err != nil {
				return "", err
			}
			return v.String(), nil
		})

	case "isValid":
		s, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		return semver.Valid(s), nil

	case "clean":
		return str1(req.Args, func(s string) (string, error) {
			c := semver.Clean(s)
			if c == "" {
				return "", fmt.Errorf("semver: Clean(%q) returned no version", s)
			}
			return c, nil
		})

	case "core":
		return str1(req.Args, func(s string) (string, error) {
			v, err := semver.Parse(s)
			if err != nil {
				return "", err
			}
			return v.Core().String(), nil
		})

	// -------------------------------------------------------------- accessors
	case "major":
		return str1(req.Args, semver.Major)
	case "minor":
		return str1(req.Args, semver.Minor)
	case "patch":
		return str1(req.Args, semver.Patch)
	case "prerelease":
		return str1(req.Args, func(s string) (string, error) {
			p, err := semver.Prerelease(s)
			if err != nil {
				return "", err
			}
			if p == "" {
				return "", fmt.Errorf("semver: %q has no prerelease", s)
			}
			return p, nil
		})

	// ------------------------------------------------------------- comparison
	case "compare":
		return cmp2(req.Args, semver.Compare)
	case "rcompare":
		return cmp2(req.Args, func(a, b string) int { return -semver.Compare(a, b) })
	case "compareMethod":
		return method2(req.Args, func(a, b *semver.Version) int { return a.Compare(b) })
	case "gt":
		return cmp2(req.Args, semver.GT)
	case "gte":
		return cmp2(req.Args, semver.GTE)
	case "lt":
		return cmp2(req.Args, semver.LT)
	case "lte":
		return cmp2(req.Args, semver.LTE)
	case "eq":
		return cmp2(req.Args, semver.EQ)
	case "neq":
		return cmp2(req.Args, semver.NEQ)
	case "lessThan":
		return method2(req.Args, func(a, b *semver.Version) bool { return a.LessThan(b) })
	case "greaterThan":
		return method2(req.Args, func(a, b *semver.Version) bool { return a.GreaterThan(b) })
	case "equalVersion":
		return method2(req.Args, func(a, b *semver.Version) bool { return a.Equal(b) })

	case "sort":
		list, err := argStrings(req.Args, 0)
		if err != nil {
			return nil, err
		}
		// Sort panics on an invalid version, exactly as upstream sort() throws;
		// validate up front so the error message names the offender.
		for _, s := range list {
			if _, err := semver.Parse(s); err != nil {
				return nil, err
			}
		}
		// make, not append-to-nil: an empty result must marshal as [] and not
		// as null, so it compares against upstream's empty array.
		out := make([]string, len(list))
		copy(out, list)
		semver.Sort(out)
		return out, nil

	// ------------------------------------------------------------ incrementing
	case "inc":
		return str2(req.Args, semver.Inc)
	case "incId":
		a, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		rel, err := argString(req.Args, 1)
		if err != nil {
			return nil, err
		}
		id, err := argString(req.Args, 2)
		if err != nil {
			return nil, err
		}
		return semver.IncWith(a, rel, id)
	case "incMajorMethod":
		return str1(req.Args, func(s string) (string, error) {
			v, err := semver.Parse(s)
			if err != nil {
				return "", err
			}
			return v.IncMajor().String(), nil
		})
	case "incMinorMethod":
		return str1(req.Args, func(s string) (string, error) {
			v, err := semver.Parse(s)
			if err != nil {
				return "", err
			}
			return v.IncMinor().String(), nil
		})
	case "incPatchMethod":
		return str1(req.Args, func(s string) (string, error) {
			v, err := semver.Parse(s)
			if err != nil {
				return "", err
			}
			return v.IncPatch().String(), nil
		})

	// ---------------------------------------------------------------- coercion
	case "coerce":
		return str1(req.Args, semver.Coerce)

	// ------------------------------------------------------------------ ranges
	case "satisfies":
		return str2(req.Args, func(v, r string) (bool, error) { return semver.Satisfies(v, r), nil })

	case "rangeTest":
		return str2(req.Args, func(vs, rs string) (bool, error) {
			r, err := semver.ParseRange(rs)
			if err != nil {
				return false, err
			}
			// Range.Test takes a *Version, so an unparseable version cannot
			// reach it at all; upstream's Range#test catches the same condition
			// internally and answers false. Report false to compare like for
			// like — an invalid RANGE still fails on both sides.
			v, err := semver.Parse(vs)
			if err != nil {
				return false, nil
			}
			return r.Test(v), nil
		})

	case "rangeValid":
		s, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		_, perr := semver.ParseRange(s)
		return perr == nil, nil

	case "rangeString":
		return str1(req.Args, func(s string) (string, error) {
			r, err := semver.ParseRange(s)
			if err != nil {
				return "", err
			}
			return r.String(), nil
		})

	case "maxSatisfying", "minSatisfying":
		list, err := argStrings(req.Args, 0)
		if err != nil {
			return nil, err
		}
		r, err := argString(req.Args, 1)
		if err != nil {
			return nil, err
		}
		pick := semver.MaxSatisfying
		if req.Fn == "minSatisfying" {
			pick = semver.MinSatisfying
		}
		v, ok := pick(list, r)
		if !ok {
			return nil, fmt.Errorf("semver: %s: nothing in the list satisfies %q", req.Fn, r)
		}
		return v, nil

	default:
		return nil, fmt.Errorf("unknown fn: %s", req.Fn)
	}
}

// safeDispatch turns a panic (MustParse, Compare, Sort on invalid input) into an
// ordinary failure, so one bad case never takes the runner down.
func safeDispatch(req request) (v any, err error) {
	defer func() {
		if r := recover(); r != nil {
			v, err = nil, fmt.Errorf("panic: %v", r)
		}
	}()
	return dispatch(req)
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
		if v, err := safeDispatch(req); err != nil {
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
