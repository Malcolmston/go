package array

import (
	"slices"
	"testing"
)

func TestUnion(t *testing.T) {
	tests := []struct {
		name string
		in   [][]int
		want []int
	}{
		{"nothing", nil, []int{}},
		{"one set", [][]int{{1, 2}}, []int{1, 2}},
		{"disjoint sets", [][]int{{1, 2}, {3}}, []int{1, 2, 3}},
		{"overlapping sets", [][]int{{1, 2}, {2, 3}}, []int{1, 2, 3}},
		{"duplicates within one input", [][]int{{1, 1, 2}}, []int{1, 2}},
		{"empty inputs", [][]int{{}, nil, {1}}, []int{1}},
		// Order is first-seen, not sorted and not random: a legend built from a
		// union must not reshuffle between runs.
		{"first-seen order", [][]int{{3, 1}, {2, 3}}, []int{3, 1, 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for trial := 0; trial < 10; trial++ {
				if got := Union(tt.in...); !slices.Equal(got, tt.want) {
					t.Fatalf("Union(%v) = %v, want %v", tt.in, got, tt.want)
				}
			}
		})
	}
}

func TestIntersection(t *testing.T) {
	tests := []struct {
		name string
		in   [][]int
		want []int
	}{
		{"nothing", nil, []int{}},
		{"one set is just a dedupe", [][]int{{1, 2, 2}}, []int{1, 2}},
		{"common elements", [][]int{{1, 2, 3}, {2, 3, 4}}, []int{2, 3}},
		{"three sets", [][]int{{1, 2, 3}, {2, 3}, {3, 9}}, []int{3}},
		{"no overlap", [][]int{{1}, {2}}, []int{}},
		{"with an empty set", [][]int{{1, 2}, {}}, []int{}},
		{"first set empty", [][]int{{}, {1, 2}}, []int{}},
		// The order and the deduplication both follow the first argument.
		{"order follows the first set", [][]int{{3, 1, 2, 1}, {1, 2, 3}}, []int{3, 1, 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for trial := 0; trial < 10; trial++ {
				if got := Intersection(tt.in...); !slices.Equal(got, tt.want) {
					t.Fatalf("Intersection(%v) = %v, want %v", tt.in, got, tt.want)
				}
			}
		})
	}
}

func TestDifference(t *testing.T) {
	tests := []struct {
		name   string
		a      []int
		others [][]int
		want   []int
	}{
		{"no others is a dedupe", []int{1, 2, 2}, nil, []int{1, 2}},
		{"remove some", []int{1, 2, 3}, [][]int{{2}}, []int{1, 3}},
		{"remove all", []int{1, 2}, [][]int{{1, 2}}, []int{}},
		{"remove nothing", []int{1, 2}, [][]int{{9}}, []int{1, 2}},
		{"several others", []int{1, 2, 3, 4}, [][]int{{1}, {3}}, []int{2, 4}},
		{"empty input", nil, [][]int{{1}}, []int{}},
		{"order follows the first set", []int{3, 1, 2}, [][]int{{1}}, []int{3, 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for trial := 0; trial < 10; trial++ {
				if got := Difference(tt.a, tt.others...); !slices.Equal(got, tt.want) {
					t.Fatalf("Difference(%v, %v) = %v, want %v", tt.a, tt.others, got, tt.want)
				}
			}
		})
	}
}

// TestDifferenceIsAsymmetric pins the direction, which is the easiest thing to
// get backwards.
func TestDifferenceIsAsymmetric(t *testing.T) {
	a, b := []int{1, 2}, []int{2, 3}
	if got := Difference(a, b); !slices.Equal(got, []int{1}) {
		t.Errorf("Difference(%v, %v) = %v, want [1]", a, b, got)
	}
	if got := Difference(b, a); !slices.Equal(got, []int{3}) {
		t.Errorf("Difference(%v, %v) = %v, want [3]", b, a, got)
	}
}

func TestDisjoint(t *testing.T) {
	tests := []struct {
		name string
		a, b []int
		want bool
	}{
		{"no overlap", []int{1, 2}, []int{3, 4}, true},
		{"one shared element", []int{1, 2}, []int{2, 3}, false},
		{"identical", []int{1}, []int{1}, false},
		{"both empty", nil, nil, true},
		{"one empty", []int{1}, nil, true},
		{"other empty", nil, []int{1}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Disjoint(tt.a, tt.b); got != tt.want {
				t.Errorf("Disjoint(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
			// Disjointness is symmetric, unlike Difference.
			if got := Disjoint(tt.b, tt.a); got != tt.want {
				t.Errorf("Disjoint(%v, %v) = %v, want %v (symmetry)", tt.b, tt.a, got, tt.want)
			}
		})
	}
}

func TestSupersetSubset(t *testing.T) {
	tests := []struct {
		name         string
		a, b         []int
		wantSuperset bool // a ⊇ b
	}{
		{"strict superset", []int{1, 2, 3}, []int{1, 2}, true},
		{"equal sets", []int{1, 2}, []int{1, 2}, true},
		{"missing an element", []int{1, 2}, []int{1, 3}, false},
		{"strict subset the wrong way", []int{1}, []int{1, 2}, false},
		// Every set contains the empty set, and only the empty set is contained
		// by the empty set.
		{"everything contains the empty set", []int{1}, nil, true},
		{"the empty set contains only itself", nil, []int{1}, false},
		{"empty and empty", nil, nil, true},
		{"duplicates do not matter", []int{1, 1, 2}, []int{2, 2}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Superset(tt.a, tt.b); got != tt.wantSuperset {
				t.Errorf("Superset(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.wantSuperset)
			}
			// Subset is Superset with the arguments swapped, by definition.
			if got := Subset(tt.b, tt.a); got != tt.wantSuperset {
				t.Errorf("Subset(%v, %v) = %v, want %v", tt.b, tt.a, got, tt.wantSuperset)
			}
		})
	}
}

func TestSetsOfStrings(t *testing.T) {
	// The set operations are generic over any comparable type, not just numbers.
	a := []string{"a", "b", "c"}
	b := []string{"b", "d"}
	if got := Union(a, b); !slices.Equal(got, []string{"a", "b", "c", "d"}) {
		t.Errorf("Union = %v", got)
	}
	if got := Intersection(a, b); !slices.Equal(got, []string{"b"}) {
		t.Errorf("Intersection = %v", got)
	}
	if got := Difference(a, b); !slices.Equal(got, []string{"a", "c"}) {
		t.Errorf("Difference = %v", got)
	}
}

// TestSetIdentities checks the algebra rather than more examples: union and
// intersection must agree with difference and containment on the same inputs.
func TestSetIdentities(t *testing.T) {
	cases := [][2][]int{
		{{1, 2, 3}, {2, 3, 4}},
		{{1}, {1}},
		{{}, {1, 2}},
		{{1, 2}, {}},
		{{5, 5, 5}, {5}},
		{{1, 2, 3}, {4, 5, 6}},
	}
	for _, c := range cases {
		a, b := c[0], c[1]
		union := Union(a, b)
		inter := Intersection(a, b)
		diffA := Difference(a, b)
		diffB := Difference(b, a)

		// |A ∪ B| = |A \ B| + |A ∩ B| + |B \ A|
		if len(union) != len(diffA)+len(inter)+len(diffB) {
			t.Errorf("inclusion-exclusion fails for %v, %v: |union|=%d, parts=%d+%d+%d",
				a, b, len(union), len(diffA), len(inter), len(diffB))
		}
		// The intersection is a subset of both; each difference is disjoint
		// from the other set.
		if !Subset(inter, a) || !Subset(inter, b) {
			t.Errorf("intersection of %v, %v is not a subset of both: %v", a, b, inter)
		}
		if !Disjoint(diffA, b) {
			t.Errorf("Difference(%v, %v) = %v is not disjoint from %v", a, b, diffA, b)
		}
		// The union is a superset of both.
		if !Superset(union, a) || !Superset(union, b) {
			t.Errorf("union of %v, %v is not a superset of both: %v", a, b, union)
		}
	}
}
