package react

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestFormDataAccessors(t *testing.T) {
	cases := []struct {
		name       string
		build      func() FormData
		key        string
		wantGet    string
		wantValues []string
	}{
		{
			name:       "absent key",
			build:      func() FormData { return FormData{} },
			key:        "q",
			wantGet:    "",
			wantValues: nil,
		},
		{
			name:       "nil map is readable",
			build:      func() FormData { return nil },
			key:        "q",
			wantGet:    "",
			wantValues: nil,
		},
		{
			name:       "single value",
			build:      func() FormData { return FormData{"q": {"go"}} },
			key:        "q",
			wantGet:    "go",
			wantValues: []string{"go"},
		},
		{
			name:       "Get returns the first of several",
			build:      func() FormData { return FormData{"tag": {"a", "b"}} },
			key:        "tag",
			wantGet:    "a",
			wantValues: []string{"a", "b"},
		},
		{
			name: "Add appends",
			build: func() FormData {
				f := FormData{}
				f.Add("tag", "a")
				f.Add("tag", "b")
				return f
			},
			key:        "tag",
			wantGet:    "a",
			wantValues: []string{"a", "b"},
		},
		{
			name: "Set replaces every value",
			build: func() FormData {
				f := FormData{"tag": {"a", "b"}}
				f.Set("tag", "c")
				return f
			},
			key:        "tag",
			wantGet:    "c",
			wantValues: []string{"c"},
		},
		{
			name: "an explicitly empty value list reads as absent",
			build: func() FormData {
				return FormData{"q": {}}
			},
			key:        "q",
			wantGet:    "",
			wantValues: nil,
		},
	}

	for _, tc := range cases {
		f := tc.build()
		if got := f.Get(tc.key); got != tc.wantGet {
			t.Errorf("%s: Get(%q) = %q, want %q", tc.name, tc.key, got, tc.wantGet)
		}
		got := f.Values(tc.key)
		if fmt.Sprint(got) != fmt.Sprint(tc.wantValues) {
			t.Errorf("%s: Values(%q) = %v, want %v", tc.name, tc.key, got, tc.wantValues)
		}
	}
}

func TestFormDataValuesReturnsACopy(t *testing.T) {
	f := FormData{"tag": {"a", "b"}}
	vs := f.Values("tag")
	vs[0] = "mutated"
	if got := f.Get("tag"); got != "a" {
		t.Errorf("Get after mutating the Values result = %q, want %q", got, "a")
	}
}

func TestUseOptimisticShowsValueThenResetsOnCommittedState(t *testing.T) {
	log := newConcurrentLog()

	var (
		capturedAdd    func(string)
		capturedCommit func(string)
	)
	var c Component = func(Props) Node {
		committed, commit := txState("base")
		shown, add := UseOptimistic(committed, func(current, action string) string {
			return current + "+" + action
		})
		capturedAdd, capturedCommit = add, commit
		log.add(shown)
		return concurrentHost("b", shown)
	}

	root := NewRoot(concurrentComponent(c, nil))
	defer root.Unmount()

	add, commit := capturedAdd, capturedCommit

	// The optimistic value is visible on the very next render.
	add("draft")
	if got, want := log.last(), "base+draft"; got != want {
		t.Fatalf("after add = %q, want %q", got, want)
	}

	// Optimistic updates compose while the committed state is unchanged.
	add("second")
	if got, want := log.last(), "base+draft+second"; got != want {
		t.Fatalf("after a second add = %q, want %q", got, want)
	}

	// New committed state discards the optimistic view — even though the real
	// value bears no resemblance to what was optimistically shown.
	commit("server")
	if got, want := log.last(), "server"; got != want {
		t.Fatalf("after commit = %q, want %q", got, want)
	}

	// …and the hook is usable again afterwards.
	add("again")
	if got, want := log.last(), "server+again"; got != want {
		t.Errorf("after add following a commit = %q, want %q", got, want)
	}

	want := []string{"base", "base+draft", "base+draft+second", "server", "server+again"}
	if got := log.all(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("renders = %v, want %v", got, want)
	}
}

func TestUseActionStateThreadsPreviousState(t *testing.T) {
	var prevSeen []string

	action := func(prev string, data FormData) string {
		prevSeen = append(prevSeen, prev)
		return prev + "," + data.Get("v")
	}

	var (
		capturedSubmit func(FormData)
		lastState      string
		lastPending    bool
	)
	var c Component = func(Props) Node {
		state, submit, pending := UseActionState(action, "init")
		capturedSubmit, lastState, lastPending = submit, state, pending
		return concurrentHost("b", state)
	}

	root := NewRoot(concurrentComponent(c, nil))
	defer root.Unmount()

	submit := capturedSubmit
	submit(FormData{"v": {"a"}})
	submit(FormData{"v": {"b"}})

	if got, want := lastState, "init,a,b"; got != want {
		t.Errorf("state = %q, want %q", got, want)
	}
	if lastPending {
		t.Errorf("isPending = true after both submissions finished, want false")
	}
	if got, want := fmt.Sprint(prevSeen), fmt.Sprint([]string{"init", "init,a"}); got != want {
		t.Errorf("prev values seen by the action = %v, want %v", got, want)
	}
	if got, want := dump(root.tree()), `(b "init,a,b")`; got != want {
		t.Errorf("tree = %s, want %s", got, want)
	}
}

func TestUseActionStateSnapshotsSubmittedData(t *testing.T) {
	var seen string
	action := func(prev string, data FormData) string {
		seen = data.Get("v")
		return prev
	}

	var capturedSubmit func(FormData)
	var c Component = func(Props) Node {
		_, submit, _ := UseActionState(action, "x")
		capturedSubmit = submit
		return nil
	}

	root := NewRoot(concurrentComponent(c, nil))
	defer root.Unmount()

	data := FormData{"v": {"original"}}
	capturedSubmit(data)
	data.Set("v", "mutated-after-submit")

	if seen != "original" {
		t.Errorf("action saw %q, want %q — data must be snapshotted", seen, "original")
	}
}

func TestUseActionStateIsPendingAcrossABlockingAction(t *testing.T) {
	log := newConcurrentLog()
	release := make(chan struct{})

	action := func(prev string, data FormData) string {
		<-release // blocks until the test releases it; never a sleep
		return prev + "/" + data.Get("v")
	}

	var capturedSubmit func(FormData)
	var c Component = func(Props) Node {
		state, submit, pending := UseActionState(action, "init")
		capturedSubmit = submit
		log.add(fmt.Sprintf("%s pending=%v", state, pending))
		return concurrentHost("b", state)
	}

	root := NewRoot(concurrentComponent(c, nil))
	defer root.Unmount()

	if got, want := log.last(), "init pending=false"; got != want {
		t.Fatalf("mount render = %q, want %q", got, want)
	}

	submit := capturedSubmit
	done := make(chan struct{})
	go func() {
		defer close(done)
		submit(FormData{"v": {"one"}})
	}()

	log.waitFor(t, "init pending=true")

	close(release)
	<-done

	log.waitFor(t, "init/one pending=false")
	if got, want := log.last(), "init/one pending=false"; got != want {
		t.Errorf("final render = %q, want %q", got, want)
	}
}

func TestUseActionStateSerializesConcurrentSubmissions(t *testing.T) {
	const submissions = 4

	gate := make(chan struct{})
	action := func(prev int, data FormData) int {
		<-gate // every action waits for the same release
		return prev + 1
	}

	var (
		capturedSubmit func(FormData)
		lastState      int
		lastPending    bool
	)
	var c Component = func(Props) Node {
		state, submit, pending := UseActionState(action, 0)
		capturedSubmit, lastState, lastPending = submit, state, pending
		return concurrentHost("b", state)
	}

	root := NewRoot(concurrentComponent(c, nil))
	defer root.Unmount()

	submit := capturedSubmit
	entered := make(chan struct{}, submissions)
	var wg sync.WaitGroup
	for i := range submissions {
		wg.Add(1)
		go func() {
			defer wg.Done()
			entered <- struct{}{}
			submit(FormData{"v": {fmt.Sprint(i)}})
		}()
	}
	for range submissions {
		<-entered
	}
	close(gate)
	wg.Wait()

	// Serialization is what makes this deterministic: every action saw the
	// previous one's result as prev, so nothing was lost to a last-writer-wins
	// race even though the submissions overlapped.
	if lastState != submissions {
		t.Errorf("state = %d, want %d (one increment per concurrent submission)", lastState, submissions)
	}
	if lastPending {
		t.Errorf("isPending = true after every submission finished, want false")
	}
}

func TestUseFormStatusReportsTheNearestActionStateAncestor(t *testing.T) {
	log := newConcurrentLog()
	release := make(chan struct{})

	// The button is two levels below the "form", to show that no intermediate
	// component has to thread anything down.
	var button Component = func(Props) Node {
		status := UseFormStatus()
		log.add(fmt.Sprintf("pending=%v q=%q", status.Pending, status.Data.Get("q")))
		return concurrentHost("button", "submit")
	}
	var wrapper Component = func(Props) Node {
		return concurrentHost("div", concurrentComponent(button, nil))
	}

	action := func(prev string, data FormData) string {
		<-release
		return data.Get("q")
	}

	var (
		capturedSubmit func(FormData)
		formOwnStatus  FormStatus
	)
	var form Component = func(Props) Node {
		state, submit, _ := UseActionState(action, "")
		// The component that owns the action state is the form, not something
		// inside it, so its own UseFormStatus must report nothing.
		formOwnStatus = UseFormStatus()
		capturedSubmit = submit
		return concurrentHost("section", state, concurrentComponent(wrapper, nil))
	}

	root := NewRoot(concurrentComponent(form, nil))
	defer root.Unmount()

	log.waitFor(t, `pending=false q=""`)

	submit := capturedSubmit
	done := make(chan struct{})
	go func() {
		defer close(done)
		submit(FormData{"q": {"hello"}})
	}()

	// The descendant sees both the pending flag and the submitted data.
	log.waitFor(t, `pending=true q="hello"`)

	close(release)
	<-done

	log.waitFor(t, `pending=false q=""`)
	if got, want := log.last(), `pending=false q=""`; got != want {
		t.Errorf("final button render = %q, want %q", got, want)
	}
	// FormStatus holds a map, so it is not comparable with ==; check the fields.
	if formOwnStatus.Pending || formOwnStatus.Data != nil {
		t.Errorf("the form's own UseFormStatus = %+v, want the zero status", formOwnStatus)
	}
	if got, want := dump(root.tree()), `(section "hello" (div (button "submit")))`; got != want {
		t.Errorf("tree = %s, want %s", got, want)
	}
}

func TestUseFormStatusOutsideAFormIsZero(t *testing.T) {
	var got FormStatus
	var c Component = func(Props) Node {
		got = UseFormStatus()
		return nil
	}

	root := NewRoot(concurrentComponent(c, nil))
	defer root.Unmount()

	if got.Pending || got.Data != nil {
		t.Errorf("UseFormStatus outside a form = %+v, want the zero status", got)
	}
}

func TestUseFormStatusStopsReportingAfterTheFormUnmounts(t *testing.T) {
	// The scope registry is keyed by fiber, so it must be cleaned up on unmount
	// or a long-lived process would leak one entry per mounted form.
	var c Component = func(Props) Node {
		UseActionState(func(prev string, _ FormData) string { return prev }, "")
		return nil
	}

	formScopesMu.RLock()
	before := len(formScopes)
	formScopesMu.RUnlock()

	root := NewRoot(concurrentComponent(c, nil))

	formScopesMu.RLock()
	during := len(formScopes)
	formScopesMu.RUnlock()
	if during != before+1 {
		t.Fatalf("registered form scopes = %d, want %d", during, before+1)
	}

	root.Unmount()

	formScopesMu.RLock()
	after := len(formScopes)
	formScopesMu.RUnlock()
	if after != before {
		t.Errorf("form scopes after unmount = %d, want %d", after, before)
	}
}

func TestActionAndTransitionHooksComposeWithOptimisticState(t *testing.T) {
	// The intended end-to-end shape: an optimistic value is shown while a
	// transition runs the action, and is replaced by the committed result.
	log := newConcurrentLog()
	release := make(chan struct{})

	action := func(prev string, data FormData) string {
		<-release
		return "server:" + data.Get("v")
	}

	var (
		capturedStart  func(func())
		capturedSubmit func(FormData)
		capturedAdd    func(string)
	)
	var c Component = func(Props) Node {
		state, submit, _ := UseActionState(action, "empty")
		shown, add := UseOptimistic(state, func(current, a string) string { return "local:" + a })
		pending, start := UseTransition()
		capturedStart, capturedSubmit, capturedAdd = start, submit, add
		log.add(fmt.Sprintf("%s pending=%v", shown, pending))
		return concurrentHost("b", shown)
	}

	root := NewRoot(concurrentComponent(c, nil))
	defer root.Unmount()

	start, submit, add := capturedStart, capturedSubmit, capturedAdd
	done := make(chan struct{})
	go func() {
		defer close(done)
		add("draft")
		start(func() { submit(FormData{"v": {"draft"}}) })
	}()

	log.waitFor(t, "local:draft pending=true")

	close(release)
	<-done

	log.waitFor(t, "server:draft pending=false")
	if got, want := log.last(), "server:draft pending=false"; got != want {
		t.Errorf("final render = %q, want %q", got, want)
	}
	if strings.Contains(log.last(), "local:") {
		t.Errorf("optimistic value survived the committed state: %q", log.last())
	}
}
