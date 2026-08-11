package array

// The set operations take and return slices rather than map[T]struct{}. d3
// returns an InternSet, which in JavaScript iterates in insertion order; a Go
// map does not iterate in any order at all, and a set operation whose result
// reshuffles on every run is useless for anything you intend to render or
// compare in a test. So the results here are deduplicated slices in
// first-seen order: still sets in content, but deterministic. Inputs need not be
// deduplicated; outputs always are.

// dedupe returns the distinct elements of values in first-seen order.
func dedupe[T comparable](values []T) []T {
	seen := make(map[T]struct{}, len(values))
	out := make([]T, 0, len(values))
	for _, v := range values {
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// setOf returns a membership set for values.
func setOf[T comparable](values []T) map[T]struct{} {
	s := make(map[T]struct{}, len(values))
	for _, v := range values {
		s[v] = struct{}{}
	}
	return s
}

// Union returns the distinct elements of all the given slices, in the order they
// are first encountered.
func Union[T comparable](values ...[]T) []T {
	return dedupe(Merge(values...))
}

// Intersection returns the distinct elements of the first slice that also appear
// in every one of the others, in the order they appear in the first. Calling it
// with a single slice is just a deduplication; calling it with none returns an
// empty slice rather than the (infinite) universal set.
func Intersection[T comparable](values ...[]T) []T {
	if len(values) == 0 {
		return []T{}
	}
	others := make([]map[T]struct{}, len(values)-1)
	for i, v := range values[1:] {
		others[i] = setOf(v)
	}
	out := make([]T, 0, len(values[0]))
	seen := make(map[T]struct{}, len(values[0]))
	for _, v := range values[0] {
		if _, dup := seen[v]; dup {
			continue
		}
		in := true
		for _, o := range others {
			if _, ok := o[v]; !ok {
				in = false
				break
			}
		}
		if in {
			seen[v] = struct{}{}
			out = append(out, v)
		}
	}
	return out
}

// Difference returns the distinct elements of values that appear in none of the
// others, in the order they appear in values. It is asymmetric — the *relative*
// complement — so Difference(a, b) and Difference(b, a) are different questions.
func Difference[T comparable](values []T, others ...[]T) []T {
	sets := make([]map[T]struct{}, len(others))
	for i, o := range others {
		sets[i] = setOf(o)
	}
	out := make([]T, 0, len(values))
	seen := make(map[T]struct{}, len(values))
	for _, v := range values {
		if _, dup := seen[v]; dup {
			continue
		}
		excluded := false
		for _, s := range sets {
			if _, ok := s[v]; ok {
				excluded = true
				break
			}
		}
		if !excluded {
			seen[v] = struct{}{}
			out = append(out, v)
		}
	}
	return out
}

// Disjoint reports whether a and b share no elements. Two empty sets are
// disjoint, which is the vacuous but conventional answer.
func Disjoint[T comparable](a, b []T) bool {
	sa := setOf(a)
	for _, v := range b {
		if _, ok := sa[v]; ok {
			return false
		}
	}
	return true
}

// Superset reports whether every element of b is also in a. Note the argument
// order, which is d3's and reads as "a ⊇ b": the containing set comes first.
// Every set is a superset of the empty set.
func Superset[T comparable](a, b []T) bool {
	sa := setOf(a)
	for _, v := range b {
		if _, ok := sa[v]; !ok {
			return false
		}
	}
	return true
}

// Subset reports whether every element of a is also in b — Superset with its
// arguments swapped, spelled out because reading a nested Superset call
// backwards is how set-logic bugs happen.
func Subset[T comparable](a, b []T) bool { return Superset(b, a) }
