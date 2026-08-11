package shape_test

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// The helpers below exist so that curves whose control points cannot be
// derived by hand can still be asserted rigorously: instead of pinning an
// opaque string, the tests parse the path and check properties of it — that it
// is well-formed, that it starts and ends where the data does, that it has the
// expected number of segments.

type cmd struct {
	op   byte
	args []float64
}

// argCount is the number of numbers each SVG path command takes. Only the
// commands this port can emit are listed; anything else is a bug.
var argCount = map[byte]int{'M': 2, 'L': 2, 'Q': 4, 'C': 6, 'A': 7, 'Z': 0, 'h': 1, 'v': 1}

// parsePath tokenizes path data and fails the test if it is not well-formed.
// It relies on this port's serialization never using spaces or exponents, so
// a command is a letter followed by comma-separated numbers.
func parsePath(t *testing.T, s string) []cmd {
	t.Helper()
	var cmds []cmd
	for i := 0; i < len(s); {
		op := s[i]
		n, ok := argCount[op]
		if !ok {
			t.Fatalf("path %q: unknown command %q at %d", s, op, i)
		}
		i++
		j := i
		for j < len(s) && !isCommand(s[j]) {
			j++
		}
		var args []float64
		if field := s[i:j]; field != "" {
			for _, f := range strings.Split(field, ",") {
				v, err := strconv.ParseFloat(f, 64)
				if err != nil {
					t.Fatalf("path %q: bad number %q: %v", s, f, err)
				}
				args = append(args, v)
			}
		}
		if len(args) != n {
			t.Fatalf("path %q: command %q wants %d args, got %d (%v)", s, op, n, len(args), args)
		}
		cmds = append(cmds, cmd{op: op, args: args})
		i = j
	}
	if len(cmds) > 0 && cmds[0].op != 'M' {
		t.Fatalf("path %q must begin with M", s)
	}
	return cmds
}

func isCommand(b byte) bool {
	_, ok := argCount[b]
	return ok
}

// endpoints walks the commands and returns the on-curve point each one lands
// on, which is what "starts at the first datum" and "ends at the last" mean.
func endpoints(cmds []cmd) [][2]float64 {
	var pts [][2]float64
	var cur, start [2]float64
	for _, c := range cmds {
		switch c.op {
		case 'M':
			cur = [2]float64{c.args[0], c.args[1]}
			start = cur
		case 'L':
			cur = [2]float64{c.args[0], c.args[1]}
		case 'Q':
			cur = [2]float64{c.args[2], c.args[3]}
		case 'C':
			cur = [2]float64{c.args[4], c.args[5]}
		case 'A':
			cur = [2]float64{c.args[5], c.args[6]}
		case 'h':
			cur[0] += c.args[0]
		case 'v':
			cur[1] += c.args[0]
		case 'Z':
			cur = start
		}
		pts = append(pts, cur)
	}
	return pts
}

// subpaths counts how many M commands the path has, i.e. how many disjoint
// pieces it draws — the observable consequence of a defined accessor.
func subpaths(cmds []cmd) int {
	n := 0
	for _, c := range cmds {
		if c.op == 'M' {
			n++
		}
	}
	return n
}

func countOp(cmds []cmd, op byte) int {
	n := 0
	for _, c := range cmds {
		if c.op == op {
			n++
		}
	}
	return n
}

func closeTo(t *testing.T, what string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %v, want %v (±%v)", what, got, want, tol)
	}
}

func pointCloseTo(t *testing.T, what string, got [2]float64, wx, wy, tol float64) {
	t.Helper()
	if math.Abs(got[0]-wx) > tol || math.Abs(got[1]-wy) > tol {
		t.Errorf("%s = (%v,%v), want (%v,%v) (±%v)", what, got[0], got[1], wx, wy, tol)
	}
}

// allNumbers returns every number in the path, for range assertions.
func allNumbers(cmds []cmd) []float64 {
	var out []float64
	for _, c := range cmds {
		out = append(out, c.args...)
	}
	return out
}

func assertFinite(t *testing.T, s string, cmds []cmd) {
	t.Helper()
	for _, v := range allNumbers(cmds) {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("path %q contains a non-finite number", s)
		}
	}
}
