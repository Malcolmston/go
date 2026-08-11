package hierarchy

import (
	"cmp"
	"fmt"
	"math"
	"sort"
	"testing"
)

// randTree builds a deterministic pseudo-random tree. The generator is a plain
// LCG rather than math/rand so that a failure is reproducible from the seed
// alone, with no dependence on the Go version's shuffling of the global source.
func randTree(seed uint32, maxNodes int) *tdata {
	s := seed | 1
	next := func(n int) int {
		s = s*1664525 + 1013904223
		return int(s>>16) % n
	}
	count := 0
	var build func(depth int) *tdata
	build = func(depth int) *tdata {
		count++
		// Wide near the root, narrow further down, and always terminating.
		nkids := 0
		if depth < 5 && count < maxNodes {
			nkids = next(6 - depth)
		}
		if nkids == 0 {
			return leaf(fmt.Sprintf("l%d", count), float64(next(50)+1))
		}
		cs := make([]*tdata, nkids)
		for i := range cs {
			cs[i] = build(depth + 1)
		}
		return branch(fmt.Sprintf("b%d", count), cs...)
	}
	return build(0)
}

func randRoot(seed uint32) *Node[*tdata] {
	return Hierarchy(randTree(seed, 200), kids).
		Sum(func(d *tdata) float64 { return d.weight }).
		Sort(func(a, b *Node[*tdata]) int { return cmp.Compare(b.Value, a.Value) })
}

// TestLayoutInvariantsAcrossManyTrees is the broad safety net: every layout is
// run over 60 differently shaped trees and checked against the properties it
// promises. Exact coordinates are covered elsewhere by cases small enough to
// derive by hand; this test exists to catch the failure mode those cannot — a
// shape of input that makes an algorithm produce output that is merely wrong
// rather than obviously wrong.
func TestLayoutInvariantsAcrossManyTrees(t *testing.T) {
	for seed := uint32(1); seed <= 60; seed++ {
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			t.Run("tree", func(t *testing.T) {
				root := NewTree[*tdata]().NodeSize(1, 1).Layout(randRoot(seed))
				checkNoOverlapByDepth(t, root, 1)
				checkParentsCentred(t, root)
				checkYByDepth(t, root, 1)
			})

			t.Run("cluster", func(t *testing.T) {
				root := NewCluster[*tdata]().NodeSize(1, 1).Layout(randRoot(seed))
				leaves := root.Leaves()
				sort.Slice(leaves, func(i, j int) bool { return leaves[i].X < leaves[j].X })
				for i := 1; i < len(leaves); i++ {
					if gap := leaves[i].X - leaves[i-1].X; gap < 1-eps {
						t.Errorf("leaf gap %v < 1", gap)
					}
					if leaves[i].Y != leaves[0].Y {
						t.Errorf("leaves are not on one line")
					}
				}
				root.Each(func(n *Node[*tdata]) {
					if len(n.Children) == 0 {
						return
					}
					sum := 0.0
					for _, c := range n.Children {
						sum += c.X
					}
					if want := sum / float64(len(n.Children)); !closeTo(n.X, want) {
						t.Errorf("parent X %v, want the child mean %v", n.X, want)
					}
				})
			})

			for _, tc := range tilings() {
				t.Run("treemap/"+tc.name, func(t *testing.T) {
					root := NewTreemap[*tdata]().Size(960, 500).Tile(tc.tile).Layout(randRoot(seed))
					checkRectSanity(t, root)
					checkNoRectOverlap(t, root)
					checkExactTiling(t, root)
					checkAreaProportional(t, root)
				})
				t.Run("treemap padded/"+tc.name, func(t *testing.T) {
					root := NewTreemap[*tdata]().Size(960, 500).Tile(tc.tile).
						PaddingInner(2).PaddingTop(10).PaddingOuter(3).Layout(randRoot(seed))
					checkRectSanity(t, root)
					checkNoRectOverlap(t, root)
				})
			}

			t.Run("partition", func(t *testing.T) {
				root := NewPartition[*tdata]().Size(900, 400).Layout(randRoot(seed))
				checkPartitionSanity(t, root)
				checkNoRectOverlap(t, root)
				root.Each(func(n *Node[*tdata]) {
					if len(n.Children) == 0 {
						return
					}
					if !closeTo(n.Children[0].X0, n.X0) {
						t.Errorf("first child starts at %v, want %v", n.Children[0].X0, n.X0)
					}
					last := n.Children[len(n.Children)-1]
					if !closeTo(last.X1, n.X1) {
						t.Errorf("last child ends at %v, want %v", last.X1, n.X1)
					}
					for i := 1; i < len(n.Children); i++ {
						if !closeTo(n.Children[i].X0, n.Children[i-1].X1) {
							t.Errorf("children are not contiguous at %v/%v",
								n.Children[i-1].X1, n.Children[i].X0)
						}
					}
				})
			})

			t.Run("pack", func(t *testing.T) {
				root := NewPack[*tdata]().Size(900, 900).Layout(randRoot(seed))
				checkCirclesFinite(t, root)
				checkNoCircleOverlap(t, root)
				checkCirclesNested(t, root)
			})

			t.Run("pack padded", func(t *testing.T) {
				root := NewPack[*tdata]().Size(900, 900).Padding(3).Layout(randRoot(seed))
				checkCirclesFinite(t, root)
				checkNoCircleOverlap(t, root)
				checkCirclesNested(t, root)
			})
		})
	}
}

// TestPackPaddingSeparatesSiblings checks the thing padding is actually for:
// siblings end up visibly apart rather than tangent.
//
// The requested padding is only approximately honoured, and deliberately so.
// Pack applies padding by inflating radii in an *intermediate* coordinate space
// whose scale is only known after a first, unpadded packing pass; the second,
// padded pass then changes the overall radius slightly, so the final scale
// differs from the one the padding was converted with. D3 has exactly the same
// behaviour. The assertion here is therefore that the gap is in the right
// ballpark — at least half the requested padding — and never zero, not that it
// is exact.
func TestPackPaddingSeparatesSiblings(t *testing.T) {
	const p = 6
	for seed := uint32(1); seed <= 10; seed++ {
		root := NewPack[*tdata]().Size(900, 900).Padding(p).Layout(randRoot(seed))
		root.Each(func(n *Node[*tdata]) {
			cs := n.Children
			if len(cs) < 2 {
				return
			}
			for i := range cs {
				for j := i + 1; j < len(cs); j++ {
					gap := math.Hypot(cs[j].X-cs[i].X, cs[j].Y-cs[i].Y) - cs[i].R - cs[j].R
					if gap < p/2 {
						t.Fatalf("seed %d depth %d: siblings only %v apart, want about %v",
							seed, n.Depth, gap, p)
					}
				}
			}
		})
	}

	t.Run("more padding means more space", func(t *testing.T) {
		minGap := func(padding float64) float64 {
			best := math.Inf(1)
			for seed := uint32(1); seed <= 20; seed++ {
				root := NewPack[*tdata]().Size(900, 900).Padding(padding).Layout(randRoot(seed))
				root.Each(func(n *Node[*tdata]) {
					cs := n.Children
					for i := range cs {
						for j := i + 1; j < len(cs); j++ {
							best = math.Min(best, math.Hypot(cs[j].X-cs[i].X, cs[j].Y-cs[i].Y)-cs[i].R-cs[j].R)
						}
					}
				})
			}
			return best
		}
		if a, b := minGap(0), minGap(10); !(b > a+1) {
			t.Errorf("min gap with padding 10 = %v, with padding 0 = %v; want a clear increase", b, a)
		}
	})
}
