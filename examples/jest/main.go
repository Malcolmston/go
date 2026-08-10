// Command jest-example exercises github.com/malcolmston/jest outside of `go
// test`. jest.Expect accepts any jest.TestReporter (not just *testing.T), so
// this program plugs in a recording reporter and prints a PASS/FAIL line per
// assertion. Mocks, spies, fake timers, asymmetric matchers and custom
// matchers need no *testing.T at all.
//
// The Describe/It/Each/BeforeEach organisation API and snapshot matchers DO
// require *testing.T; those live in main_test.go (`GOWORK=off go test ./...`).
package main

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/malcolmston/jest"
)

// reporter implements jest.TestReporter and records failures instead of
// aborting, so this program can show both passing and failing assertions.
type reporter struct {
	label    string
	failures []string
}

func (r *reporter) Errorf(format string, args ...any) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
}
func (r *reporter) Fatalf(format string, args ...any) {
	r.failures = append(r.failures, "FATAL: "+fmt.Sprintf(format, args...))
}
func (r *reporter) Helper() {}

// check runs fn against a fresh reporter and prints whether it passed.
func check(desc string, fn func(t jest.TestReporter)) {
	r := &reporter{label: desc}
	fn(r)
	if len(r.failures) == 0 {
		fmt.Printf("  PASS  %s\n", desc)
		return
	}
	first := strings.SplitN(r.failures[0], "\n", 2)[0]
	fmt.Printf("  FAIL  %s\n          -> %s\n", desc, first)
}

func section(title string) { fmt.Printf("\n=== %s ===\n", title) }

type User struct {
	Name  string         `json:"name"`
	Age   int            `json:"age"`
	Tags  []string       `json:"tags"`
	Meta  map[string]any `json:"meta"`
	Email string         `json:"email"`
}

func main() {
	coreMatchers()
	deepAndObjectMatchers()
	panicMatchers()
	negation()
	asymmetricMatchers()
	customMatchers()
	mocks()
	typedMocksAndSpies()
	spyOn()
	mockMatchers()
	fakeTimers()
	deliberateFailures()
	fmt.Println("\nAll jest examples completed (see main_test.go for the",
		"Describe/It/snapshot side).")
}

// ---------------------------------------------------------------------------

func coreMatchers() {
	section("1. Core matchers")

	check("ToBe(4)", func(t jest.TestReporter) { jest.Expect(t, 2+2).ToBe(4) })
	check("ToEqual slice", func(t jest.TestReporter) {
		jest.Expect(t, []int{1, 2, 3}).ToEqual([]int{1, 2, 3})
	})
	check("ToBeNil", func(t jest.TestReporter) { jest.Expect[error](t, nil).ToBeNil() })
	check("ToBeTrue/ToBeFalse", func(t jest.TestReporter) {
		jest.Expect(t, true).ToBeTrue()
		jest.Expect(t, false).ToBeFalse()
	})
	check("numeric ordering", func(t jest.TestReporter) {
		jest.Expect(t, 10).ToBeGreaterThan(5)
		jest.Expect(t, 10).ToBeGreaterThanOrEqual(10)
		jest.Expect(t, 3).ToBeLessThan(4)
		jest.Expect(t, 3).ToBeLessThanOrEqual(3)
	})
	check("ToContain substring/element/key", func(t jest.TestReporter) {
		jest.Expect(t, "hello world").ToContain("world")
		jest.Expect(t, []string{"a", "b"}).ToContain("b")
		jest.Expect(t, map[string]int{"k": 1}).ToContain("k")
	})
	check("ToHaveLen / ToHaveLength", func(t jest.TestReporter) {
		jest.Expect(t, "abcd").ToHaveLen(4)
		jest.Expect(t, []int{1, 2}).ToHaveLength(2)
	})
	check("ToMatch regexp", func(t jest.TestReporter) {
		jest.Expect(t, "order-1234").ToMatch(`^order-\d+$`)
	})
	check("ToBeCloseTo", func(t jest.TestReporter) {
		jest.Expect(t, 3.14159).ToBeCloseTo(3.14, 0.01)
		jest.Expect(t, 0.1+0.2).ToBeCloseTo(0.3) // default epsilon 1e-9
	})
	check("truthy/falsy/defined/undefined/NaN/null", func(t jest.TestReporter) {
		jest.Expect[any](t, "x").ToBeTruthy()
		jest.Expect[any](t, 0).ToBeFalsy()
		jest.Expect[any](t, 1).ToBeDefined()
		jest.Expect[any](t, nil).ToBeUndefined()
		jest.Expect[any](t, nil).ToBeNull()
		jest.Expect(t, mathNaN()).ToBeNaN()
	})
	check("ToBeOneOf / ToContainEqual", func(t jest.TestReporter) {
		jest.Expect(t, "b").ToBeOneOf("a", "b", "c")
		jest.Expect(t, [][]int{{1}, {2}}).ToContainEqual([]int{2})
	})
	check("ToBeInstanceOf (concrete + interface)", func(t jest.TestReporter) {
		jest.Expect[any](t, User{}).ToBeInstanceOf(User{})
		jest.Expect[any](t, "hi").ToBeInstanceOf(reflect.TypeOf(""))
		// HOLE: the idiomatic Go way to name an interface type,
		// `(*error)(nil)`, does NOT work here -- Any()/ToBeInstanceOf calls
		// reflect.TypeOf on it and gets *error (a pointer), so the check
		// fails. You must spell it reflect.TypeOf((*error)(nil)).Elem().
		// jest.Expect[any](t, errors.New("boom")).ToBeInstanceOf((*error)(nil))
		jest.Expect[any](t, errors.New("boom")).
			ToBeInstanceOf(reflect.TypeOf((*error)(nil)).Elem())
	})
}

func mathNaN() float64 { z := 0.0; return z / z }

func deepAndObjectMatchers() {
	section("2. Deep / structural matchers")

	u := User{
		Name: "Ada", Age: 36,
		Tags:  []string{"admin", "dev"},
		Meta:  map[string]any{"ok": true, "score": 91},
		Email: "ada@example.com",
	}

	check("ToStrictEqual", func(t jest.TestReporter) {
		jest.Expect(t, u).ToStrictEqual(u)
	})
	check("ToMatchObject subset (struct)", func(t jest.TestReporter) {
		jest.Expect[any](t, u).ToMatchObject(map[string]any{"Name": "Ada", "Age": 36})
	})
	check("ToMatchObject subset (map, nested)", func(t jest.TestReporter) {
		jest.Expect[any](t, map[string]any{
			"a": 1, "b": map[string]any{"c": 2, "d": 3},
		}).ToMatchObject(map[string]any{"b": map[string]any{"c": 2}})
	})
	check("ToHaveProperty dotted + indexed", func(t jest.TestReporter) {
		jest.Expect[any](t, u).ToHaveProperty("Tags[0]", "admin")
		jest.Expect[any](t, u).ToHaveProperty("Meta.ok", true)
		jest.Expect[any](t, u).ToHaveProperty("Name") // existence only
	})
}

func panicMatchers() {
	section("3. Panic matchers")

	check("ToPanic with message", func(t jest.TestReporter) {
		jest.Expect(t, func() { panic("boom: bad input") }).ToPanic("bad input")
	})
	check("ToThrow (alias)", func(t jest.TestReporter) {
		jest.Expect(t, func() { panic(errors.New("kaput")) }).ToThrow("kaput")
	})
	check("Not().ToPanic", func(t jest.TestReporter) {
		jest.Expect(t, func() {}).Not().ToPanic()
	})
}

func negation() {
	section("4. Negation")

	check("Not() over several matchers", func(t jest.TestReporter) {
		jest.Expect(t, 5).Not().ToBe(6)
		jest.Expect(t, []int{1}).Not().ToEqual([]int{2})
		jest.Expect(t, "abc").Not().ToContain("z")
		jest.Expect(t, 1).Not().ToBeGreaterThan(9)
	})
}

func asymmetricMatchers() {
	section("5. Asymmetric matchers")

	resp := map[string]any{
		"id":    7,
		"name":  "robot",
		"email": "bot@example.com",
		"roles": []any{"admin", "user"},
		"meta":  map[string]any{"ok": true},
		"extra": "anything",
	}

	check("nested asymmetrics in ToEqual", func(t jest.TestReporter) {
		jest.Expect[any](t, resp).ToEqual(map[string]any{
			"id":    jest.Any(0),
			"name":  jest.StringContaining("bo"),
			"email": jest.StringMatching(`@example\.com$`),
			"roles": jest.ArrayContaining("admin"),
			"meta":  jest.ObjectContaining(map[string]any{"ok": true}),
			"extra": jest.Anything(),
		})
	})
	check("CloseTo + negated asymmetrics", func(t jest.TestReporter) {
		jest.Expect[any](t, 3.14159).ToEqual(jest.CloseTo(3.14, 2))
		jest.Expect[any](t, "hello").ToEqual(jest.NotStringContaining("zzz"))
		jest.Expect[any](t, "hello").ToEqual(jest.NotStringMatching(`^\d+$`))
		jest.Expect[any](t, []any{1, 2}).ToEqual(jest.NotArrayContaining(9))
		jest.Expect[any](t, map[string]any{"a": 1}).
			ToEqual(jest.NotObjectContaining(map[string]any{"a": 2}))
	})
	check("asymmetric inside ToHaveProperty", func(t jest.TestReporter) {
		jest.Expect[any](t, resp).ToHaveProperty("name", jest.StringContaining("rob"))
	})
	check("Any(reflect-typed) matcher string form", func(t jest.TestReporter) {
		jest.Expect(t, jest.Any("").String()).ToContain("string")
	})
}

func customMatchers() {
	section("6. Custom matchers via Extend + To()")

	jest.Extend(map[string]jest.CustomMatcher{
		"ToBeEven": func(actual any, _ ...any) jest.MatcherResult {
			n, ok := actual.(int)
			return jest.MatcherResult{Pass: ok && n%2 == 0, Message: "expected an even int"}
		},
		"ToBeDivisibleBy": func(actual any, args ...any) jest.MatcherResult {
			n, ok1 := actual.(int)
			d, ok2 := args[0].(int)
			return jest.MatcherResult{
				Pass:    ok1 && ok2 && d != 0 && n%d == 0,
				Message: fmt.Sprintf("expected %v to be divisible by %v", actual, args[0]),
			}
		},
	})

	check("To(\"ToBeEven\")", func(t jest.TestReporter) { jest.Expect(t, 4).To("ToBeEven") })
	check("To(\"ToBeDivisibleBy\", 3)", func(t jest.TestReporter) {
		jest.Expect(t, 9).To("ToBeDivisibleBy", 3)
	})
	check("Not().To(\"ToBeEven\")", func(t jest.TestReporter) {
		jest.Expect(t, 5).Not().To("ToBeEven")
	})
	// An unregistered name is reported as a failure rather than panicking.
	unknown := &reporter{}
	jest.Expect(unknown, 1).To("NoSuchMatcher")
	fmt.Println("  unknown matcher name ->", unknown.failures[0])
}

func mocks() {
	section("7. Mocks: return values, sequences, implementations")

	m := jest.NewMock("adder")
	m.Return(42)
	fmt.Println("  Call(1,2) ->", m.Call(1, 2))
	check("call recording", func(t jest.TestReporter) {
		jest.Expect(t, m.CallCount()).ToBe(1)
		jest.Expect(t, m.Called()).ToBeTrue()
		jest.Expect(t, m.CalledWith(1, 2)).ToBeTrue()
		jest.Expect(t, m.Name()).ToBe("adder")
	})
	last, _ := m.LastCall()
	nth, _ := m.NthCall(0)
	fmt.Printf("  LastCall args=%v results=%v; NthCall(0) args=%v\n",
		last.Args, last.Results, nth.Args)
	fmt.Println("  Results():", m.Results(), "Calls():", len(m.Calls()))

	seq := jest.NewMock("seq").ReturnValues([]any{1}, []any{2}, []any{3})
	fmt.Println("  ReturnValues sequence:", seq.Call(), seq.Call(), seq.Call(), seq.Call())

	impl := jest.NewMock("impl")
	impl.MockImplementation(func(args ...any) []any {
		return []any{fmt.Sprint(args...) + "!"}
	})
	impl.MockImplementationOnce(func(args ...any) []any { return []any{"once"} })
	impl.MockReturnValueOnce("queued")
	fmt.Println("  once/queued/default:", impl.Call("a"), impl.Call("b"), impl.Call("c"))

	pr := jest.NewMock("promise").MockResolvedValue("value")
	rj := jest.NewMock("promise").MockRejectedValue(errors.New("nope"))
	fmt.Printf("  MockResolvedValue -> %v ; MockRejectedValue -> %v\n", pr.Call(), rj.Call())

	m.MockClear()
	fmt.Println("  after MockClear, CallCount:", m.CallCount())
	m.MockName("renamed")
	fmt.Println("  after MockName:", m.Name())
	m.Reset()
	jest.ClearAllMocks()
	jest.ResetAllMocks()
	fmt.Println("  ClearAllMocks/ResetAllMocks ran; CallCount:", m.CallCount())
}

func typedMocksAndSpies() {
	section("8. Typed function mocks (Fn0/Fn1/Fn2) and spies (Spy0/Spy1/Spy2)")

	greet, gm := jest.Fn0[string]("greet")
	gm.Return("hi")
	fmt.Println("  Fn0:", greet())

	stringify, sm := jest.Fn1[int, string]("stringify")
	sm.Return("seven")
	fmt.Println("  Fn1:", stringify(7))

	add, am := jest.Fn2[int, int, int]("add")
	am.Return(99)
	fmt.Println("  Fn2:", add(1, 2))

	check("typed mocks record args", func(t jest.TestReporter) {
		jest.Expect(t, sm.CalledWith(7)).ToBeTrue()
		jest.Expect(t, am.CalledWith(1, 2)).ToBeTrue()
		jest.Expect(t, gm.CallCount()).ToBe(1)
	})

	// Spies wrap a real implementation and still record.
	now, nowSpy := jest.Spy0("now", func() int { return 5 })
	double, dSpy := jest.Spy1("double", func(x int) int { return x * 2 })
	concat, cSpy := jest.Spy2("concat", func(a, b string) string { return a + b })
	fmt.Println("  Spy0/1/2 real results:", now(), double(21), concat("go", "lang"))
	check("spies pass through and record", func(t jest.TestReporter) {
		jest.Expect(t, nowSpy.CallCount()).ToBe(1)
		jest.Expect(t, dSpy.CalledWith(21)).ToBeTrue()
		jest.Expect(t, cSpy.CalledWith("go", "lang")).ToBeTrue()
	})
}

type client struct {
	Fetch func(id int) (string, error)
}

func spyOn() {
	section("9. SpyOn: replace a func field in place, then restore")

	c := &client{Fetch: func(id int) (string, error) { return fmt.Sprintf("real-%d", id), nil }}

	spy := jest.SpyOn(&c.Fetch)
	got, _ := c.Fetch(7) // delegates to the original by default
	fmt.Println("  through spy (delegating):", got)
	check("SpyOn records the call", func(t jest.TestReporter) {
		jest.Expect(t, spy).ToHaveBeenCalledWith(7)
		jest.Expect(t, spy.Mock.CallCount()).ToBe(1)
	})

	// Override the behaviour, then restore the original function.
	spy.Mock.MockImplementation(func(args ...any) []any { return []any{"stubbed", nil} })
	stubbed, _ := c.Fetch(7)
	fmt.Println("  stubbed:", stubbed)

	spy.Restore()
	restored, _ := c.Fetch(7)
	fmt.Println("  after Restore():", restored)

	// RestoreAllMocks() undoes every outstanding SpyOn.
	s2 := jest.SpyOn(&c.Fetch)
	s2.Mock.MockImplementation(func(args ...any) []any { return []any{"again", nil} })
	jest.RestoreAllMocks()
	afterAll, _ := c.Fetch(1)
	fmt.Println("  after RestoreAllMocks():", afterAll)
}

func mockMatchers() {
	section("10. Mock-oriented matchers")

	m := jest.NewMock("api")
	m.Return(42)
	m.Call("a", 1)
	m.Call("b", 2)

	check("call-count / call-args matchers", func(t jest.TestReporter) {
		jest.Expect(t, m).ToHaveBeenCalled()
		jest.Expect(t, m).ToHaveBeenCalledTimes(2)
		jest.Expect(t, m).ToHaveBeenCalledWith(jest.Any(""), 1)
		jest.Expect(t, m).ToHaveBeenNthCalledWith(2, "b", 2)
		jest.Expect(t, m).ToHaveBeenLastCalledWith("b", 2)
	})
	check("return matchers", func(t jest.TestReporter) {
		jest.Expect(t, m).ToHaveReturned()
		jest.Expect(t, m).ToHaveReturnedTimes(2)
		jest.Expect(t, m).ToHaveReturnedWith(42)
		jest.Expect(t, m).ToHaveLastReturnedWith(42)
		jest.Expect(t, m).ToHaveNthReturnedWith(1, 42)
	})
	check("Jest legacy aliases", func(t jest.TestReporter) {
		jest.Expect(t, m).ToBeCalled()
		jest.Expect(t, m).ToBeCalledTimes(2)
		jest.Expect(t, m).ToBeCalledWith("a", 1)
		jest.Expect(t, m).LastCalledWith("b", 2)
		jest.Expect(t, m).NthCalledWith(1, "a", 1)
		jest.Expect(t, m).ToReturn()
		jest.Expect(t, m).ToReturnWith(42)
	})
	check("Not() on a mock matcher", func(t jest.TestReporter) {
		jest.Expect(t, m).Not().ToHaveBeenCalledTimes(5)
	})
}

func fakeTimers() {
	section("11. Fake timers (jest.Clock)")

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := jest.NewClockAt(start)
	var log []string

	c.SetTimeout(time.Second, func() { log = append(log, "t+1s") })
	c.SetTimeout(3*time.Second, func() { log = append(log, "t+3s") })
	tick := c.SetInterval(time.Second, func() { log = append(log, "tick@"+c.Now().Format("15:04:05")) })
	cancelled := c.SetTimeout(10*time.Second, func() { log = append(log, "never") })

	fmt.Println("  pending:", c.PendingCount(), "GetTimerCount:", c.GetTimerCount())
	fmt.Println("  ClearTimer(cancelled):", c.ClearTimer(cancelled))

	fired := c.AdvanceTimersByTime(2500 * time.Millisecond)
	fmt.Printf("  AdvanceTimersByTime(2.5s) fired %d; now=%s\n", fired, c.Now().Format("15:04:05.000"))
	fmt.Println("  log:", log)

	fmt.Println("  AdvanceTimersToNextTimer fired:", c.AdvanceTimersToNextTimer())
	c.ClearTimer(tick)
	fmt.Println("  RunAllTimers fired:", c.RunAllTimers(), "now=", c.Now().Format("15:04:05"))

	// After() gives a channel that is delivered when time is advanced.
	c2 := jest.NewClock()
	ch := c2.After(5 * time.Second)
	c2.AdvanceTimersByTime(5 * time.Second)
	select {
	case at := <-ch:
		fmt.Println("  After() channel delivered at offset:", at.Sub(c2.Now()) == 0)
	case <-time.After(time.Second):
		fmt.Println("  HOLE: After() channel never delivered")
	}

	c3 := jest.NewClock()
	c3.SetTimeout(time.Second, func() {})
	c3.SetTimeout(2*time.Second, func() {})
	fmt.Println("  RunOnlyPendingTimers fired:", c3.RunOnlyPendingTimers())
	c3.SetTimeout(time.Second, func() {})
	fmt.Println("  ClearAllTimers removed:", c3.ClearAllTimers())
	c3.SetSystemTime(start)
	fmt.Println("  SetSystemTime ->", c3.Now().Format(time.RFC3339))

	check("timers ran in order", func(t jest.TestReporter) {
		jest.Expect(t, log).ToContain("t+1s")
		jest.Expect(t, len(log)).ToBeGreaterThan(2)
	})
}

func deliberateFailures() {
	section("12. Failure messages (these are SUPPOSED to fail)")

	r := &reporter{}
	jest.Expect(r, 1).ToBe(2)
	jest.Expect(r, []int{1, 2}).ToEqual([]int{1, 3})
	jest.Expect(r, "abc").ToMatch(`^\d+$`)
	jest.Expect(r, func() {}).ToPanic("boom")
	jest.Expect[any](r, map[string]any{"a": 1}).
		ToMatchObject(map[string]any{"a": 2})
	for i, f := range r.failures {
		fmt.Printf("  [%d] %s\n", i+1, strings.ReplaceAll(f, "\n", "\n        "))
	}
	fmt.Println("  captured", len(r.failures), "failures without aborting")
}
