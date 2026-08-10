package main

import (
	"strings"
	"testing"

	"github.com/malcolmston/jest"
)

// The Describe/It/BeforeEach organisation API, Each/DescribeEach and the
// snapshot matchers require a real *testing.T, so they are exercised here
// rather than from main().

func TestDescribeLifecycle(t *testing.T) {
	var counter int
	var order []string

	jest.Describe(t, "counter", func() {
		jest.BeforeAll(func() { order = append(order, "beforeAll") })
		jest.AfterAll(func() { order = append(order, "afterAll") })
		jest.BeforeEach(func() { counter = 0; order = append(order, "beforeEach") })
		jest.AfterEach(func() { order = append(order, "afterEach") })

		jest.It(t, "starts at zero", func(t *testing.T) {
			jest.Expect(t, counter).ToBe(0)
		})
		jest.It(t, "increments", func(t *testing.T) {
			counter++
			jest.Expect(t, counter).ToBe(1)
		})

		jest.Describe(t, "nested", func() {
			jest.BeforeEach(func() { counter = 100 })
			jest.It(t, "inherits outer hooks then applies its own", func(t *testing.T) {
				jest.Expect(t, counter).ToBe(100)
			})
		})

		jest.ItSkip(t, "skipped case", func(t *testing.T) { t.Fatal("must not run") })
		jest.ItTodo(t, "todo case")
	})

	jest.Test(t, "Test is an alias for It", func(t *testing.T) {
		jest.Expect(t, strings.ToUpper("go")).ToBe("GO")
	})

	if len(order) == 0 {
		t.Fatal("lifecycle hooks never ran")
	}
	t.Logf("hook order: %v", order)
}

type squareCase struct {
	In, Out int
}

func TestEach(t *testing.T) {
	jest.Each(t, "square of %v", []squareCase{{2, 4}, {3, 9}, {4, 16}},
		func(t *testing.T, tc squareCase) {
			jest.Expect(t, tc.In*tc.In).ToBe(tc.Out)
		})
}

func TestDescribeEach(t *testing.T) {
	jest.DescribeEach(t, "with %v", []string{"alpha", "beta"}, func(tc string) {
		jest.It(t, "is non-empty", func(t *testing.T) {
			jest.Expect(t, len(tc)).ToBeGreaterThan(0)
		})
		jest.It(t, "round-trips through upper", func(t *testing.T) {
			jest.Expect(t, strings.ToLower(strings.ToUpper(tc))).ToBe(tc)
		})
	})
}

func TestAssertionCounts(t *testing.T) {
	jest.Test(t, "exactly two assertions", func(t *testing.T) {
		jest.Assertions(t, 2)
		jest.Expect(t, 1).ToBe(1)
		jest.Expect(t, 2).ToBe(2)
	})
	jest.Test(t, "at least one assertion", func(t *testing.T) {
		jest.HasAssertions(t)
		jest.Expect(t, "x").ToHaveLen(1)
	})
}

func TestSnapshots(t *testing.T) {
	// Inline snapshots need no files on disk.
	// Note the trailing comma after the last entry -- the serializer emits it.
	jest.Expect[any](t, map[string]any{"b": 2, "a": 1}).
		ToMatchInlineSnapshot(`{
  "a": 1,
  "b": 2,
}`)

	// On-disk snapshots are created under __snapshots__/ on the first run and
	// compared afterwards. JEST_UPDATE_SNAPSHOTS=1 refreshes them.
	jest.Expect(t, renderReport()).ToMatchSnapshot("report")

	jest.Expect(t, func() { panic("kaboom") }).ToThrowMatchingSnapshot("panic-msg")
}

func renderReport() string {
	return strings.Join([]string{"line one", "line two", "line three"}, "\n")
}

func TestMocksInsideTests(t *testing.T) {
	t.Cleanup(jest.RestoreAllMocks)

	fetch, mock := jest.Fn1[int, string]("fetch")
	mock.ReturnValues([]any{"a"}, []any{"b"})

	jest.Expect(t, fetch(1)).ToBe("a")
	jest.Expect(t, fetch(2)).ToBe("b")
	jest.Expect(t, mock).ToHaveBeenCalledTimes(2)
	jest.Expect(t, mock).ToHaveBeenNthCalledWith(2, 2)
	jest.Expect(t, mock).ToHaveLastReturnedWith("b")
}
