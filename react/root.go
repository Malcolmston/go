package react

import (
	"fmt"
	"sync"
)

// Root is a mounted tree: an element, the fiber tree rendered from it, and the
// hook state hanging off that tree. It is the Go equivalent of the object
// `createRoot` returns, minus the DOM — there is no browser here, so a Root
// hands back a rendered tree that a renderer such as [RenderToString] turns
// into output.
//
// A Root is the unit of state. Two Roots share nothing; the same element
// rendered into two Roots has two independent sets of hooks.
//
// All methods are safe for concurrent use, but rendering itself is serialized
// process-wide (see renderMu), so a state update from a background goroutine
// blocks until any in-flight render finishes rather than interleaving with it.
type Root struct {
	mu sync.Mutex

	// container is a synthetic fragment fiber that owns the tree. Having a real
	// fiber above the user's element means the top-level element is reconciled
	// by exactly the same code path as every other child — no special case for
	// "the root changed type".
	container *fiber

	// element is the node most recently handed to [NewRoot] or [Root.Render].
	element Node

	// pendingEffects accumulates effects registered during the render pass in
	// progress; [Root.commit] drains it once the tree is complete.
	pendingEffects []*pendingEffect

	// flushing is true from the moment a flush loop starts until it settles,
	// covering the commit as well as the render. An update raised inside that
	// window must only mark the tree dirty and return: the loop re-checks dirty
	// after every commit and will pick the work up on its next pass.
	//
	// Covering the commit is not a detail. renderMu is held for the whole flush
	// and is not reentrant, so a setter called from an effect body — the
	// subscribe-then-deliver pattern, the single most common reason for a second
	// pass — would otherwise re-enter flush and deadlock against itself.
	flushing bool

	// pendingUpdates holds the fibers that asked for a re-render since the last
	// pass. Marking a fiber walks its ancestor chain, which only the rendering
	// goroutine may touch, so a setter called from a background goroutine parks
	// the fiber here under mu instead and the next pass does the marking. That
	// is what keeps a background setState off the renderer's tree pointers.
	pendingUpdates []*fiber

	// dirty means some fiber requested an update that has not been rendered
	// yet. The flush loop keeps going while it is set.
	dirty bool

	// unmounted makes [Root.Unmount] idempotent and turns a late state update
	// from a stale closure into a no-op rather than a panic — exactly the
	// "can't update an unmounted component" case, handled by ignoring it.
	unmounted bool
}

// pendingEffect is one effect waiting to fire after the current commit. layout
// effects run first and synchronously, before any passive effect, which is the
// ordering guarantee that lets a layout effect measure a tree that passive
// effects have not yet mutated.
type pendingEffect struct {
	h      *hook
	create func() func()
	layout bool
}

// maxRenderPasses bounds a single flush. A component that updates state
// unconditionally in its own body re-renders forever; React reports "Too many
// re-renders" after a fixed number of attempts and so does this, because an
// infinite loop with no diagnostic is the worst possible failure mode.
const maxRenderPasses = 50

// NewRoot mounts node and renders it, returning the live tree. Effects run
// before it returns, so the Root is fully settled by the time a caller sees it.
func NewRoot(node Node) *Root {
	r := &Root{}
	r.container = &fiber{typ: fragmentTag{}, props: Props{}, root: r}
	r.Render(node)
	return r
}

// Render replaces the root element and re-renders. Calling it with a
// structurally similar tree preserves state, the same way re-rendering any
// other subtree does; calling it with a different element type at the top
// remounts everything.
func (r *Root) Render(node Node) {
	r.mu.Lock()
	if r.unmounted {
		r.mu.Unlock()
		return
	}
	r.element = node
	r.dirty = true
	r.mu.Unlock()

	r.flush()
}

// Flush renders any pending updates and runs the effects they scheduled. State
// updates made from outside a render — from a goroutine, a timer, a test —
// flush automatically, so this is only needed when updates were batched with
// [Batch] or when a caller wants to settle effects explicitly before reading
// the tree.
func (r *Root) Flush() { r.flush() }

// flush is the render loop: render the tree, commit, run effects, and repeat
// while an effect or a render-phase update left the tree dirty. Effects that
// set state are the normal reason for a second pass — that is how a subscribe
// effect delivers its first value.
func (r *Root) flush() {
	if batching() {
		// Inside Batch, updates only mark the root dirty; the outermost Batch
		// flushes once at the end so N updates cost one render, not N.
		batchAdd(r)
		return
	}

	// Claim the flush. A second caller — another goroutine's setter, or an
	// effect body on this very goroutine — returns immediately; the work it
	// requested is already recorded in dirty and will be served by this loop.
	r.mu.Lock()
	if r.unmounted || r.flushing {
		r.mu.Unlock()
		return
	}
	r.flushing = true
	r.mu.Unlock()

	// owner is a goroutine-local guard so the deferred release only fires when
	// this call still holds the flush. The loop hands the claim back in the same
	// critical section that observes a settled tree, which closes the window
	// where an update could arrive, see flushing, and be dropped by a loop that
	// had already decided to stop. The defer is the panic path — without it a
	// component that panics mid-render would wedge the Root forever.
	owner := true
	defer func() {
		if owner {
			r.mu.Lock()
			r.flushing = false
			r.mu.Unlock()
		}
	}()

	renderMu.Lock()
	defer renderMu.Unlock()

	for pass := 0; ; pass++ {
		r.mu.Lock()
		if r.unmounted || !r.dirty {
			r.flushing = false
			owner = false
			r.mu.Unlock()
			return
		}
		if pass >= maxRenderPasses {
			r.dirty = false
			r.mu.Unlock()
			panic(fmt.Sprintf("react: too many re-renders (%d passes without settling); "+
				"a component is most likely calling a state setter unconditionally in its "+
				"body instead of inside an effect or an event handler", maxRenderPasses))
		}
		r.dirty = false
		element := r.element
		r.mu.Unlock()

		r.renderPass(element)

		r.mu.Lock()
		effects := r.pendingEffects
		r.pendingEffects = nil
		r.mu.Unlock()

		runEffects(effects)
	}
}

// renderPass renders the whole tree from the container down. The port
// re-renders from the root on every pass rather than restarting work at the
// fiber that changed: reconciliation preserves fibers, so state and effects
// behave identically either way, and the simpler traversal is far easier to
// reason about. Memoized subtrees ([Memo]) are where a re-render is actually
// skipped.
func (r *Root) renderPass(element Node) {
	// Apply the update marks requested since the last pass. This happens here,
	// on the rendering goroutine, rather than in scheduleUpdate: marking walks
	// parent pointers that reconciliation rewrites, so doing it from a
	// background goroutine would race the renderer over the tree's own links.
	// Marking before the descent is what a memoized ancestor needs in order to
	// see that something beneath it wants to render.
	r.mu.Lock()
	pending := r.pendingUpdates
	r.pendingUpdates = nil
	r.mu.Unlock()
	for _, f := range pending {
		if !f.unmounted {
			f.markNeedsRender()
		}
	}

	// The container renders its single child through the normal child path, so
	// the top-level element gets keys, types and remounting like any other.
	r.container.props = Props{ChildrenKey: element}
	reconcileChildren(r.container, []Node{element})
}

// runEffects fires a commit's effects: layout effects first, in tree order,
// then passive effects. Each effect's previous cleanup runs immediately before
// its new body, which is what makes an effect with changing deps look like
// "tear down the old subscription, set up the new one".
func runEffects(effects []*pendingEffect) {
	for _, phase := range []bool{true, false} {
		for _, e := range effects {
			if e.layout != phase {
				continue
			}
			if e.h.cleanup != nil {
				cleanup := e.h.cleanup
				e.h.cleanup = nil
				cleanup()
			}
			e.h.cleanup = e.create()
		}
	}
}

// enqueueEffect registers an effect to fire after the current commit. Effect
// hooks call it during render; the effect body itself never runs during render,
// which is the whole point of the separation.
func enqueueEffect(h *hook, create func() func(), layout bool) {
	f := currentFiber()
	r := f.root
	if r == nil {
		return
	}
	r.mu.Lock()
	r.pendingEffects = append(r.pendingEffects, &pendingEffect{h: h, create: create, layout: layout})
	r.mu.Unlock()
}

// scheduleUpdate marks the tree as needing another render. Called from a state
// setter, it is the entire scheduling story: mark dirty, then either let the
// in-flight render loop pick it up or start one.
//
// An update raised while rendering does not recurse — it sets the flag and the
// flush loop runs another pass, which is how a render-phase update (the
// "derive state from props" escape hatch) settles without unbounded recursion.
func (f *fiber) scheduleUpdate() {
	r := f.root
	if r == nil {
		return
	}

	r.mu.Lock()
	if r.unmounted {
		r.mu.Unlock()
		return
	}
	r.dirty = true
	r.pendingUpdates = append(r.pendingUpdates, f)
	busy := r.flushing
	r.mu.Unlock()

	// A flush already in progress — whether this call came from an effect body
	// on the flushing goroutine or from a background goroutine mid-render — will
	// observe the dirty flag on its next iteration. Calling flush here would at
	// best duplicate work and at worst deadlock on renderMu.
	if busy {
		return
	}
	r.flush()
}

// Unmount tears the tree down, running every effect cleanup from the leaves
// upward. Later state updates from stale closures become no-ops. Unmount is
// idempotent.
func (r *Root) Unmount() {
	renderMu.Lock()
	defer renderMu.Unlock()

	r.mu.Lock()
	if r.unmounted {
		r.mu.Unlock()
		return
	}
	r.unmounted = true
	r.pendingEffects = nil
	container := r.container
	r.mu.Unlock()

	for c := container.child; c != nil; c = c.sibling {
		unmount(c)
	}
	container.child = nil
}

// tree returns the container fiber. Renderers walk from here; the container
// itself is synthetic and produces no output.
func (r *Root) tree() *fiber { return r.container }

// batchDepth and batchRoots implement [Batch]. Like the dispatcher, they are
// package-level because React's batching is a property of the runtime rather
// than of any one root — a single event handler may touch several roots and
// should still produce one render each.
var (
	batchMu    sync.Mutex
	batchDepth int
	batchRoots []*Root
)

func batching() bool {
	batchMu.Lock()
	defer batchMu.Unlock()
	return batchDepth > 0
}

func batchAdd(r *Root) {
	batchMu.Lock()
	defer batchMu.Unlock()
	for _, existing := range batchRoots {
		if existing == r {
			return
		}
	}
	batchRoots = append(batchRoots, r)
}

// Batch runs fn with rendering deferred, so any number of state updates inside
// it produce a single render per affected root once fn returns. React 19
// batches automatically around events and effects; Go has no event loop to hang
// that off, so batching is explicit here.
//
// Batch calls nest: only the outermost one flushes.
func Batch(fn func()) {
	batchMu.Lock()
	batchDepth++
	batchMu.Unlock()

	defer func() {
		batchMu.Lock()
		batchDepth--
		done := batchDepth == 0
		var roots []*Root
		if done {
			roots, batchRoots = batchRoots, nil
		}
		batchMu.Unlock()

		for _, r := range roots {
			r.flush()
		}
	}()

	fn()
}
