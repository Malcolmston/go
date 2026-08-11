package hierarchy

import (
	"errors"
	"fmt"
)

// Errors reported by [Stratify]. They are returned wrapped with the offending
// id, so use [errors.Is] to classify and the message for the detail.
var (
	// ErrNoRoot means no row had an empty parent id — every row claims a
	// parent, which for a finite set of rows implies a cycle or a missing root.
	ErrNoRoot = errors.New("hierarchy: no root")

	// ErrMultipleRoots means more than one row had an empty parent id.
	ErrMultipleRoots = errors.New("hierarchy: multiple roots")

	// ErrAmbiguousID means two rows shared the same id.
	ErrAmbiguousID = errors.New("hierarchy: ambiguous id")

	// ErrMissingParent means a row named a parent id that no row defines.
	ErrMissingParent = errors.New("hierarchy: missing parent")

	// ErrCycle means the parent links form a loop, so some rows are
	// unreachable from the root.
	ErrCycle = errors.New("hierarchy: cycle")
)

// Stratify builds a tree from flat rows joined on id and parent id — the shape
// you get from a CSV export, a database query, or an org chart.
//
// id must return a unique, non-empty key for each row. parentID returns the id
// of the row's parent, or the empty string for the root; D3 uses null for this
// and the empty string is the natural Go stand-in, which does mean an id may not
// itself be empty.
//
// Unlike D3, which throws, Stratify returns an error, and it validates more
// thoroughly: as well as the "ambiguous", "missing", "no root" and "multiple
// roots" cases D3 detects, it explicitly reports [ErrCycle] when a group of rows
// points at each other and so never reaches the root. D3 surfaces that case as a
// confusing "no root" (or silently drops the rows when a separate root exists);
// naming it makes bad input far easier to debug.
//
// The resulting nodes' Data are the rows themselves, in the order children were
// encountered in the input. Depth and Height are computed; Value is not — call
// [Node.Sum] or [Node.Count] next.
func Stratify[T any](rows []T, id func(T) string, parentID func(T) string) (*Node[T], error) {
	if len(rows) == 0 {
		return nil, ErrNoRoot
	}

	nodes := make([]*Node[T], len(rows))
	byID := make(map[string]*Node[T], len(rows))
	for i, row := range rows {
		key := id(row)
		if key == "" {
			return nil, fmt.Errorf("%w: row %d has an empty id", ErrAmbiguousID, i)
		}
		if _, dup := byID[key]; dup {
			return nil, fmt.Errorf("%w: %q", ErrAmbiguousID, key)
		}
		n := &Node[T]{Data: row}
		nodes[i] = n
		byID[key] = n
	}

	var root *Node[T]
	for i, row := range rows {
		pid := parentID(row)
		if pid == "" {
			if root != nil {
				return nil, fmt.Errorf("%w: %q and %q", ErrMultipleRoots, id(root.Data), id(row))
			}
			root = nodes[i]
			continue
		}
		parent, ok := byID[pid]
		if !ok {
			return nil, fmt.Errorf("%w: %q (parent of %q)", ErrMissingParent, pid, id(row))
		}
		if parent == nodes[i] {
			return nil, fmt.Errorf("%w: %q is its own parent", ErrCycle, id(row))
		}
		nodes[i].Parent = parent
		parent.Children = append(parent.Children, nodes[i])
	}

	if root == nil {
		return nil, fmt.Errorf("%w: every row names a parent", ErrNoRoot)
	}

	// Anything not reachable from the root is in a cycle: it has a parent (or
	// it would be a second root, already rejected) yet no path upward ends at
	// the root. Counting reachable nodes is the cheapest way to detect that.
	reached := 0
	root.Each(func(*Node[T]) { reached++ })
	if reached != len(nodes) {
		for i, n := range nodes {
			if !reachesRoot(n, root) {
				return nil, fmt.Errorf("%w: %q is not reachable from the root", ErrCycle, id(rows[i]))
			}
		}
		return nil, ErrCycle
	}

	// Depth is assigned after linking rather than during, because rows may name
	// a parent that appears later in the input.
	root.EachBefore(func(node *Node[T]) {
		if node.Parent != nil {
			node.Depth = node.Parent.Depth + 1
		}
	})
	return root.computeHeights(), nil
}

// reachesRoot walks up from n, remembering where it has been so that a cycle
// terminates the walk instead of spinning forever.
func reachesRoot[T any](n, root *Node[T]) bool {
	seen := make(map[*Node[T]]bool)
	for ; n != nil; n = n.Parent {
		if n == root {
			return true
		}
		if seen[n] {
			return false
		}
		seen[n] = true
	}
	return false
}
