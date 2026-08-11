package react

import (
	"strings"
	"sync"
	"testing"
)

// TestRegressEffectSettingStateDoesNotDeadlock is the repro from the docs pass:
// a setter called from an effect body used to re-enter flush and deadlock on the
// non-reentrant renderMu.
func TestRegressEffectSettingStateDoesNotDeadlock(t *testing.T) {
	var c Component = func(Props) Node {
		n, setN := UseState(0)
		UseEffectFn(func() {
			if n == 0 {
				setN(1)
			}
		}, []any{n})
		return H("b", nil, n)
	}
	root := NewRoot(&Element{Type: c, Props: Props{}})
	if got := root.Tree().TextContent(); got != "1" {
		t.Errorf("text = %q, want %q", got, "1")
	}
}

// TestRegressObjectsEqualOnIdenticalFuncs used to panic inside reflect.Value.Len.
func TestRegressObjectsEqualOnIdenticalFuncs(t *testing.T) {
	f := func() {}
	if !ObjectsEqual(f, f) {
		t.Error("ObjectsEqual(f, f) = false, want true")
	}
}

// TestRegressBackgroundSetStateIsRaceFree drives setters from many goroutines;
// it is meaningful only under -race.
func TestRegressBackgroundSetStateIsRaceFree(t *testing.T) {
	var set SetStateFunc[int]
	var mu sync.Mutex
	var c Component = func(Props) Node {
		n, s := UseState(0)
		mu.Lock()
		set = s
		mu.Unlock()
		return H("b", nil, n)
	}
	root := NewRoot(&Element{Type: c, Props: Props{}})
	defer root.Unmount()

	var wg sync.WaitGroup
	for i := 1; i <= 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			mu.Lock()
			s := set
			mu.Unlock()
			s(i)
		}(i)
	}
	wg.Wait()
}

// TestRegressPortalSeparationSeesThroughRawHTML pins the reason portals are
// separated by the emitter rather than by scanning the finished markup.
//
// DangerousHTML content is written to the output byte for byte, so a raw string
// containing the literal portal tag is indistinguishable from a real boundary
// once it is bytes. The emitter never has that problem: it knows which fiber
// produced which output.
func TestRegressPortalSeparationSeesThroughRawHTML(t *testing.T) {
	decoy := `<` + PortalTag + ` ` + PortalContainerProp + `="decoy">stolen</` + PortalTag + `>`

	page := H("main", nil,
		H("div", Props{"dangerouslySetInnerHTML": DangerousHTML{HTML: decoy}}),
		CreatePortal(H("p", nil, "real"), "modal"),
	)

	main, portals, err := RenderPortalsToString(page)
	if err != nil {
		t.Fatal(err)
	}

	// The decoy is raw markup and must survive untouched, in place.
	if !strings.Contains(main, "stolen") {
		t.Errorf("raw HTML was consumed as a portal boundary; main = %q", main)
	}
	if _, claimed := portals["decoy"]; claimed {
		t.Error(`a container named "decoy" was collected from raw HTML`)
	}

	// The real portal still moves.
	if got, want := portals["modal"], "<p>real</p>"; got != want {
		t.Errorf("portals[modal] = %q, want %q", got, want)
	}
	if strings.Contains(main, "<p>real</p>") {
		t.Errorf("the portal's output stayed inline; main = %q", main)
	}
}
