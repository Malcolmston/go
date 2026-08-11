package hierarchy

import (
	"errors"
	"strings"
	"testing"
)

// tdata is the test data type: a name, a weight and nested children. Using a
// concrete struct rather than `any` also exercises the generic parameter the way
// a caller would.
type tdata struct {
	name     string
	weight   float64
	children []*tdata
}

func kids(d *tdata) []*tdata { return d.children }

func leaf(name string, w float64) *tdata { return &tdata{name: name, weight: w} }

func branch(name string, cs ...*tdata) *tdata { return &tdata{name: name, children: cs} }

// sample is the fixture most tests use:
//
//	root
//	├── a
//	│   ├── a1 (1)
//	│   └── a2 (2)
//	├── b (3)
//	└── c
//	    └── c1
//	        └── c11 (4)
func sample() *Node[*tdata] {
	return Hierarchy(branch("root",
		branch("a", leaf("a1", 1), leaf("a2", 2)),
		leaf("b", 3),
		branch("c", branch("c1", leaf("c11", 4))),
	), kids)
}

func names[T any](nodes []*Node[T], name func(T) string) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = name(n.Data)
	}
	return out
}

func tname(d *tdata) string { return d.name }

func find(t *testing.T, root *Node[*tdata], name string) *Node[*tdata] {
	t.Helper()
	n := root.Find(func(n *Node[*tdata]) bool { return n.Data.name == name })
	if n == nil {
		t.Fatalf("node %q not found", name)
	}
	return n
}

func TestHierarchyStructure(t *testing.T) {
	root := sample()

	tests := []struct {
		name         string
		wantDepth    int
		wantHeight   int
		wantParent   string
		wantChildren int
	}{
		{"root", 0, 3, "", 3},
		{"a", 1, 1, "root", 2},
		{"a1", 2, 0, "a", 0},
		{"a2", 2, 0, "a", 0},
		{"b", 1, 0, "root", 0},
		{"c", 1, 2, "root", 1},
		{"c1", 2, 1, "c", 1},
		{"c11", 3, 0, "c1", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := find(t, root, tt.name)
			if n.Depth != tt.wantDepth {
				t.Errorf("Depth = %d, want %d", n.Depth, tt.wantDepth)
			}
			if n.Height != tt.wantHeight {
				t.Errorf("Height = %d, want %d", n.Height, tt.wantHeight)
			}
			got := ""
			if n.Parent != nil {
				got = n.Parent.Data.name
			}
			if got != tt.wantParent {
				t.Errorf("Parent = %q, want %q", got, tt.wantParent)
			}
			if len(n.Children) != tt.wantChildren {
				t.Errorf("len(Children) = %d, want %d", len(n.Children), tt.wantChildren)
			}
			// D3's convention: a leaf has nil children, not an empty slice.
			if tt.wantChildren == 0 && n.Children != nil {
				t.Errorf("Children = %v, want nil for a leaf", n.Children)
			}
		})
	}
}

func TestHierarchySingleNode(t *testing.T) {
	root := Hierarchy(leaf("only", 1), kids)
	if root.Depth != 0 || root.Height != 0 || root.Parent != nil || root.Children != nil {
		t.Errorf("single node = %+v, want depth 0 height 0 no parent no children", root)
	}
	if got := root.Descendants(); len(got) != 1 || got[0] != root {
		t.Errorf("Descendants() = %v, want [root]", got)
	}
	if got := root.Leaves(); len(got) != 1 || got[0] != root {
		t.Errorf("Leaves() = %v, want [root]", got)
	}
	if got := root.Links(); len(got) != 0 {
		t.Errorf("Links() = %v, want none", got)
	}
}

func TestTraversalOrders(t *testing.T) {
	root := sample()

	collect := func(f func(func(*Node[*tdata])) *Node[*tdata]) []string {
		var out []string
		f(func(n *Node[*tdata]) { out = append(out, n.Data.name) })
		return out
	}

	tests := []struct {
		name string
		got  []string
		want []string
	}{
		{
			// Breadth-first: level by level.
			name: "Each",
			got:  collect(root.Each),
			want: []string{"root", "a", "b", "c", "a1", "a2", "c1", "c11"},
		},
		{
			// Pre-order: a node immediately before its subtree.
			name: "EachBefore",
			got:  collect(root.EachBefore),
			want: []string{"root", "a", "a1", "a2", "b", "c", "c1", "c11"},
		},
		{
			// Post-order: a node after its whole subtree.
			name: "EachAfter",
			got:  collect(root.EachAfter),
			want: []string{"a1", "a2", "a", "b", "c11", "c1", "c", "root"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if strings.Join(tt.got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("order = %v, want %v", tt.got, tt.want)
			}
		})
	}
}

func TestSumAndCount(t *testing.T) {
	root := sample().Sum(func(d *tdata) float64 { return d.weight })

	sums := map[string]float64{
		"root": 10, "a": 3, "a1": 1, "a2": 2, "b": 3, "c": 4, "c1": 4, "c11": 4,
	}
	for name, want := range sums {
		if got := find(t, root, name).Value; got != want {
			t.Errorf("Sum: %s.Value = %v, want %v", name, got, want)
		}
	}

	root.Count()
	counts := map[string]float64{
		"root": 4, "a": 2, "a1": 1, "a2": 1, "b": 1, "c": 1, "c1": 1, "c11": 1,
	}
	for name, want := range counts {
		if got := find(t, root, name).Value; got != want {
			t.Errorf("Count: %s.Value = %v, want %v", name, got, want)
		}
	}
}

func TestSumClampsNegatives(t *testing.T) {
	root := Hierarchy(branch("r", leaf("x", -5), leaf("y", 2)), kids).
		Sum(func(d *tdata) float64 { return d.weight })
	if got := find(t, root, "x").Value; got != 0 {
		t.Errorf("negative leaf value = %v, want clamped to 0", got)
	}
	if got := root.Value; got != 2 {
		t.Errorf("root value = %v, want 2", got)
	}
}

func TestSortIsStableAndRecursive(t *testing.T) {
	root := Hierarchy(branch("r",
		branch("g", leaf("g1", 1), leaf("g2", 5), leaf("g3", 5)),
		leaf("h", 9),
	), kids).Sum(func(d *tdata) float64 { return d.weight })

	// Descending by value.
	root.Sort(func(a, b *Node[*tdata]) int {
		switch {
		case b.Value < a.Value:
			return -1
		case b.Value > a.Value:
			return 1
		default:
			return 0
		}
	})

	if got := names(root.Children, tname); got[0] != "g" || got[1] != "h" {
		t.Errorf("root children = %v, want [g h] (g=11 > h=9)", got)
	}
	g := find(t, root, "g")
	// g2 and g3 tie at 5; a stable sort must keep their input order.
	if got := names(g.Children, tname); strings.Join(got, ",") != "g2,g3,g1" {
		t.Errorf("g children = %v, want [g2 g3 g1]", got)
	}
}

func TestAncestorsDescendantsLeaves(t *testing.T) {
	root := sample()

	t.Run("Ancestors", func(t *testing.T) {
		got := names(find(t, root, "c11").Ancestors(), tname)
		want := "c11,c1,c,root"
		if strings.Join(got, ",") != want {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("Descendants of subtree", func(t *testing.T) {
		got := names(find(t, root, "a").Descendants(), tname)
		want := "a,a1,a2"
		if strings.Join(got, ",") != want {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("Leaves", func(t *testing.T) {
		got := names(root.Leaves(), tname)
		want := "b,a1,a2,c11"
		if strings.Join(got, ",") != want {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}

func TestFindReturnsShallowestMatch(t *testing.T) {
	// Two nodes named "dup" at different depths; breadth-first must find the
	// shallower one.
	root := Hierarchy(branch("r",
		branch("mid", leaf("dup", 1)),
		leaf("dup", 2),
	), kids)
	got := root.Find(func(n *Node[*tdata]) bool { return n.Data.name == "dup" })
	if got == nil || got.Depth != 1 {
		t.Errorf("Find returned depth %v, want the depth-1 match", got)
	}
	if root.Find(func(*Node[*tdata]) bool { return false }) != nil {
		t.Error("Find with no match should return nil")
	}
}

func TestPath(t *testing.T) {
	root := sample()
	tests := []struct {
		name, from, to, want string
	}{
		{"across the tree", "a1", "c11", "a1,a,root,c,c1,c11"},
		{"to self", "a1", "a1", "a1"},
		{"to sibling", "a1", "a2", "a1,a,a2"},
		{"up to ancestor", "c11", "root", "c11,c1,c,root"},
		{"down from ancestor", "root", "a2", "root,a,a2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := names(find(t, root, tt.from).Path(find(t, root, tt.to)), tname)
			if strings.Join(got, ",") != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("disjoint trees", func(t *testing.T) {
		other := sample()
		if got := root.Path(other); got != nil {
			t.Errorf("Path across trees = %v, want nil", got)
		}
	})
}

func TestLinks(t *testing.T) {
	root := sample()
	links := root.Links()
	if len(links) != 7 {
		t.Fatalf("len(Links()) = %d, want 7 (one per non-root node)", len(links))
	}
	seen := map[string]string{}
	for _, l := range links {
		if l.Target.Parent != l.Source {
			t.Errorf("link %v -> %v is not a parent/child edge", l.Source.Data.name, l.Target.Data.name)
		}
		seen[l.Target.Data.name] = l.Source.Data.name
	}
	want := map[string]string{"a": "root", "b": "root", "c": "root", "a1": "a", "a2": "a", "c1": "c", "c11": "c1"}
	for target, source := range want {
		if seen[target] != source {
			t.Errorf("link to %s came from %q, want %q", target, seen[target], source)
		}
	}
}

func TestCopy(t *testing.T) {
	root := sample().Sum(func(d *tdata) float64 { return d.weight })
	cp := root.Copy()

	if cp == root {
		t.Fatal("Copy returned the same node")
	}
	if strings.Join(names(cp.Descendants(), tname), ",") !=
		strings.Join(names(root.Descendants(), tname), ",") {
		t.Error("Copy changed the traversal order or shape")
	}
	// Data is shared, structure is not.
	if find(t, cp, "a1").Data != find(t, root, "a1").Data {
		t.Error("Copy should share Data, not clone it")
	}
	if find(t, cp, "a1") == find(t, root, "a1") {
		t.Error("Copy should allocate new nodes")
	}
	// Documented: Value is deliberately not carried across.
	if cp.Value != 0 {
		t.Errorf("copy root Value = %v, want 0", cp.Value)
	}
	// Heights and depths are recomputed.
	if cp.Height != root.Height {
		t.Errorf("copy Height = %d, want %d", cp.Height, root.Height)
	}

	t.Run("subtree copy rebases depth", func(t *testing.T) {
		sub := find(t, root, "c1").Copy()
		if sub.Depth != 0 {
			t.Errorf("subtree copy root Depth = %d, want 0", sub.Depth)
		}
		if sub.Children[0].Depth != 1 {
			t.Errorf("subtree copy child Depth = %d, want 1", sub.Children[0].Depth)
		}
	})
}

type row struct{ id, parent string }

func TestStratify(t *testing.T) {
	rows := []row{
		{"root", ""},
		{"a", "root"},
		{"b", "root"},
		// A child whose parent appears later in the input.
		{"a1", "a"},
		{"c", "b"},
	}
	root, err := Stratify(rows, func(r row) string { return r.id }, func(r row) string { return r.parent })
	if err != nil {
		t.Fatalf("Stratify: %v", err)
	}
	if root.Data.id != "root" {
		t.Errorf("root = %q, want root", root.Data.id)
	}
	got := map[string][2]int{}
	root.Each(func(n *Node[row]) { got[n.Data.id] = [2]int{n.Depth, n.Height} })
	want := map[string][2]int{
		"root": {0, 2}, "a": {1, 1}, "b": {1, 1}, "a1": {2, 0}, "c": {2, 0},
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("%s depth/height = %v, want %v", id, got[id], w)
		}
	}
}

func TestStratifyErrors(t *testing.T) {
	tests := []struct {
		name string
		rows []row
		want error
	}{
		{"no rows", nil, ErrNoRoot},
		{"no root", []row{{"a", "b"}, {"b", "a"}}, ErrNoRoot},
		{"multiple roots", []row{{"a", ""}, {"b", ""}}, ErrMultipleRoots},
		{"ambiguous id", []row{{"a", ""}, {"a", "a"}}, ErrAmbiguousID},
		{"empty id", []row{{"", ""}}, ErrAmbiguousID},
		{"missing parent", []row{{"a", ""}, {"b", "nope"}}, ErrMissingParent},
		{"self parent", []row{{"a", ""}, {"b", "b"}}, ErrCycle},
		{"cycle beside a root", []row{{"r", ""}, {"x", "y"}, {"y", "x"}}, ErrCycle},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Stratify(tt.rows, func(r row) string { return r.id }, func(r row) string { return r.parent })
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}
