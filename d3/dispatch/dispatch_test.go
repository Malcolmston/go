package dispatch

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestNewRejectsBadTypes(t *testing.T) {
	cases := map[string][]string{
		"empty":     {"start", ""},
		"dotted":    {"start.foo"},
		"duplicate": {"start", "end", "start"},
	}
	for name, types := range cases {
		if _, err := New[int](types...); err == nil {
			t.Errorf("New(%v) [%s] succeeded, want an error", types, name)
		}
	}
	if _, err := New[int](); err != nil {
		t.Errorf("New() with no types = %v, want nil", err)
	}
}

func TestMustNewPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("MustNew with a bad type did not panic")
		}
	}()
	MustNew[int]("a.b")
}

func TestCallInvokesListenersInRegistrationOrder(t *testing.T) {
	d := MustNew[int]("tick")
	var got []string
	must(t, d.On("tick.a", func(int) { got = append(got, "a") }))
	must(t, d.On("tick.b", func(int) { got = append(got, "b") }))
	must(t, d.On("tick", func(int) { got = append(got, "unnamed") }))
	d.Call("tick", 1)
	if strings.Join(got, ",") != "a,b,unnamed" {
		t.Errorf("order = %v, want a,b,unnamed", got)
	}
}

func TestPayloadIsForwarded(t *testing.T) {
	type ev struct {
		Alpha float64
		Nodes int
	}
	d := MustNew[ev]("tick")
	var seen ev
	must(t, d.On("tick", func(e ev) { seen = e }))
	d.Call("tick", ev{Alpha: 0.3, Nodes: 12})
	if seen != (ev{0.3, 12}) {
		t.Errorf("payload = %v, want {0.3 12}", seen)
	}
}

func TestNamespaceReplacesRatherThanAccumulates(t *testing.T) {
	d := MustNew[int]("tick")
	var n int
	must(t, d.On("tick.legend", func(int) { n++ }))
	must(t, d.On("tick.legend", func(int) { n += 10 }))
	d.Call("tick", 0)
	if n != 10 {
		t.Errorf("n = %d, want 10 (the second registration replaces the first)", n)
	}
}

func TestReplacementKeepsItsPosition(t *testing.T) {
	d := MustNew[int]("tick")
	var got []string
	must(t, d.On("tick.a", func(int) { got = append(got, "a") }))
	must(t, d.On("tick.b", func(int) { got = append(got, "b") }))
	must(t, d.On("tick.a", func(int) { got = append(got, "a2") }))
	d.Call("tick", 0)
	if strings.Join(got, ",") != "a2,b" {
		t.Errorf("order = %v, want a2,b", got)
	}
}

func TestRemoval(t *testing.T) {
	d := MustNew[int]("tick")
	var a, b int
	must(t, d.On("tick.a", func(int) { a++ }))
	must(t, d.On("tick.b", func(int) { b++ }))

	must(t, d.On("tick.a", nil))
	d.Call("tick", 0)
	if a != 0 || b != 1 {
		t.Errorf("after removing a: a=%d b=%d, want 0 and 1", a, b)
	}

	// Removing something that is not there is not an error.
	must(t, d.On("tick.nothing", nil))
}

func TestWildcardNamespaceRemovesAcrossTypes(t *testing.T) {
	d := MustNew[int]("start", "tick", "end")
	var n int
	inc := func(int) { n++ }
	must(t, d.On("start.legend tick.legend end.legend", inc))
	must(t, d.On("tick.axis", inc))

	d.Call("start", 0)
	d.Call("tick", 0)
	d.Call("end", 0)
	if n != 4 {
		t.Fatalf("n = %d, want 4", n)
	}

	n = 0
	must(t, d.On(".legend", nil))
	d.Call("start", 0)
	d.Call("tick", 0)
	d.Call("end", 0)
	if n != 1 {
		t.Errorf("n = %d after removing .legend, want 1 (tick.axis survives)", n)
	}
}

func TestWildcardNamespaceCannotRegister(t *testing.T) {
	d := MustNew[int]("tick")
	if err := d.On(".legend", func(int) {}); err == nil {
		t.Error("On(\".legend\", fn) succeeded, want an error")
	}
}

func TestUnknownTypeIsReported(t *testing.T) {
	d := MustNew[int]("tick")

	err := d.On("tock", func(int) {})
	if !errors.Is(err, ErrUnknownType) {
		t.Errorf("On error = %v, want ErrUnknownType", err)
	}
	if err := d.Apply("tock", 0); !errors.Is(err, ErrUnknownType) {
		t.Errorf("Apply error = %v, want ErrUnknownType", err)
	}
	if err := d.Apply("tick", 0); err != nil {
		t.Errorf("Apply on a known type = %v, want nil", err)
	}

	func() {
		defer func() {
			if recover() == nil {
				t.Error("Call with an unknown type did not panic")
			}
		}()
		d.Call("tock", 0)
	}()
}

func TestOnRequiresATypename(t *testing.T) {
	d := MustNew[int]("tick")
	if err := d.On("   ", func(int) {}); err == nil {
		t.Error("On with no typename succeeded, want an error")
	}
}

func TestListenerReadsBack(t *testing.T) {
	d := MustNew[int]("tick")
	if d.Listener("tick") != nil {
		t.Error("unregistered listener is not nil")
	}
	var n int
	must(t, d.On("tick.a", func(int) { n++ }))
	fn := d.Listener("tick.a")
	if fn == nil {
		t.Fatal("Listener(\"tick.a\") = nil")
	}
	fn(0)
	if n != 1 {
		t.Errorf("the returned listener did not run")
	}
	if d.Listener("tick") != nil {
		t.Error("Listener(\"tick\") should not find the namespaced listener")
	}
}

func TestCopyIsIndependent(t *testing.T) {
	d := MustNew[int]("tick")
	var orig, copied int
	must(t, d.On("tick.shared", func(int) { orig++ }))

	c := d.Copy()
	// The inherited listener is present.
	c.Call("tick", 0)
	if orig != 1 {
		t.Fatalf("copied dispatch did not inherit the listener")
	}

	// Adding to the copy does not touch the original.
	must(t, c.On("tick.extra", func(int) { copied++ }))
	d.Call("tick", 0)
	if copied != 0 {
		t.Error("a listener added to the copy ran on the original")
	}

	// Removing from the original does not touch the copy.
	must(t, d.On("tick.shared", nil))
	orig = 0
	c.Call("tick", 0)
	if orig != 1 {
		t.Error("removing from the original also removed from the copy")
	}

	if got := c.Types(); strings.Join(got, ",") != "tick" {
		t.Errorf("Copy().Types() = %v, want [tick]", got)
	}
}

func TestListenersMayMutateDuringDispatch(t *testing.T) {
	d := MustNew[int]("tick")
	var runs int
	must(t, d.On("tick.a", func(int) {
		runs++
		// Registering, removing and dispatching from inside a listener must
		// not deadlock, and must not affect the pass already in flight.
		_ = d.On("tick.b", func(int) { runs += 100 })
		_ = d.On("tick.a", nil)
	}))

	d.Call("tick", 0)
	if runs != 1 {
		t.Fatalf("runs = %d after the first call, want 1", runs)
	}
	d.Call("tick", 0)
	if runs != 101 {
		t.Errorf("runs = %d after the second call, want 101", runs)
	}
}

func TestListenerPanicPropagates(t *testing.T) {
	d := MustNew[int]("tick")
	must(t, d.On("tick", func(int) { panic("boom") }))
	defer func() {
		if r := recover(); r != "boom" {
			t.Errorf("recovered %v, want boom", r)
		}
	}()
	d.Call("tick", 0)
}

// Run with -race: concurrent registration, removal, copying and dispatch must
// not race and must not deadlock.
func TestConcurrentRegistrationAndDispatch(t *testing.T) {
	d := MustNew[int]("start", "tick", "end")
	var calls atomic.Int64

	const goroutines = 8
	const iterations = 200

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			name := string(rune('a' + g))
			for i := 0; i < iterations; i++ {
				if err := d.On("tick."+name, func(int) { calls.Add(1) }); err != nil {
					t.Error(err)
					return
				}
				d.Call("tick", i)
				if err := d.On("tick."+name, nil); err != nil {
					t.Error(err)
					return
				}
			}
		}(g)
	}
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				d.Call("tick", i)
				d.Call("start", i)
				_ = d.Copy()
				_ = d.Listener("tick.a")
				_ = d.Types()
			}
		}()
	}
	wg.Wait()

	if calls.Load() == 0 {
		t.Error("no listener ever ran; the test proved nothing")
	}
	// Every listener was removed by its own goroutine, so the bus is empty.
	var after int
	must(t, d.On("tick.probe", func(int) { after++ }))
	d.Call("tick", 0)
	if after != 1 {
		t.Errorf("probe ran %d times, want 1 (listeners leaked)", after)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
