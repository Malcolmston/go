package scale

// Ordinal maps a discrete domain to a discrete range — categories to colors,
// species to symbols, teams to stroke patterns. Values are matched by position:
// the i-th domain value maps to the i-th range value, and the range repeats
// cyclically if it is shorter than the domain, which is why a ten-color scheme
// silently reuses colors on the eleventh category.
//
// # The implicit domain
//
// The behavior that makes ordinal scales pleasant and occasionally surprising
// is the implicit domain. By default the domain does not have to be declared:
// scaling a value that has not been seen before APPENDS it to the domain and
// assigns it the next range value. So this works with no setup at all:
//
//	color := scale.NewOrdinal[string, string]("red", "green", "blue")
//	color.Scale("apples")  // "red",   domain is now [apples]
//	color.Scale("oranges") // "green", domain is now [apples oranges]
//	color.Scale("apples")  // "red" again — the assignment is stable
//
// The catch is that the assignment now depends on the order values are first
// scaled, so two charts drawn from the same data in different orders get
// different colors. Declaring the domain with [Ordinal.SetDomain] pins it.
//
// Calling [Ordinal.SetUnknown] switches the scale out of implicit mode: unseen
// values then return that sentinel and the domain stops growing. That is the
// right setting for a legend that must not invent entries, and it is what
// [Band] and [Point] use internally.
type Ordinal[K comparable, V any] struct {
	index     map[K]int
	domain    []K
	rangeVals []V
	unknown   V
	implicit  bool
}

// NewOrdinal returns an ordinal scale with an empty, implicit domain and the
// given range.
func NewOrdinal[K comparable, V any](rng ...V) *Ordinal[K, V] {
	return &Ordinal[K, V]{
		index:     make(map[K]int),
		rangeVals: append([]V(nil), rng...),
		implicit:  true,
	}
}

// Scale maps a domain value to a range value, extending the domain if the value
// is unseen and the scale is implicit.
func (o *Ordinal[K, V]) Scale(d K) V {
	i, ok := o.index[d]
	if !ok {
		if !o.implicit {
			return o.unknown
		}
		o.domain = append(o.domain, d)
		i = len(o.domain) - 1
		o.index[d] = i
	}
	if len(o.rangeVals) == 0 {
		return o.unknown
	}
	return o.rangeVals[i%len(o.rangeVals)]
}

// Domain returns a copy of the domain, in first-seen order.
func (o *Ordinal[K, V]) Domain() []K { return append([]K(nil), o.domain...) }

// SetDomain replaces the domain, dropping duplicates and keeping first
// occurrence order. Setting the domain explicitly is what makes an ordinal
// scale reproducible across renders; see the type documentation.
func (o *Ordinal[K, V]) SetDomain(d ...K) *Ordinal[K, V] {
	o.domain = o.domain[:0]
	o.index = make(map[K]int, len(d))
	for _, v := range d {
		if _, ok := o.index[v]; ok {
			continue
		}
		o.domain = append(o.domain, v)
		o.index[v] = len(o.domain) - 1
	}
	return o
}

// Range returns a copy of the range.
func (o *Ordinal[K, V]) Range() []V { return append([]V(nil), o.rangeVals...) }

// SetRange replaces the range. It does not touch the domain, so the mapping of
// existing domain values shifts with it.
func (o *Ordinal[K, V]) SetRange(r ...V) *Ordinal[K, V] {
	o.rangeVals = append([]V(nil), r...)
	return o
}

// Unknown returns the sentinel returned for values outside the domain, which is
// meaningful only when the scale is not implicit.
func (o *Ordinal[K, V]) Unknown() V { return o.unknown }

// SetUnknown sets the sentinel for unseen values AND turns off implicit domain
// extension. This is the single call that switches an ordinal scale from
// "learn as you go" to "closed set"; see the type documentation.
func (o *Ordinal[K, V]) SetUnknown(v V) *Ordinal[K, V] {
	o.unknown = v
	o.implicit = false
	return o
}

// SetImplicit re-enables implicit domain extension, undoing
// [Ordinal.SetUnknown].
func (o *Ordinal[K, V]) SetImplicit() *Ordinal[K, V] {
	o.implicit = true
	return o
}

// Implicit reports whether unseen values extend the domain.
func (o *Ordinal[K, V]) Implicit() bool { return o.implicit }

// Copy returns an independent copy of the scale, including the domain learned
// so far. Copying an implicit scale mid-flight freezes nothing — the copy keeps
// learning independently of the original.
func (o *Ordinal[K, V]) Copy() *Ordinal[K, V] {
	c := &Ordinal[K, V]{
		index:     make(map[K]int, len(o.index)),
		domain:    append([]K(nil), o.domain...),
		rangeVals: append([]V(nil), o.rangeVals...),
		unknown:   o.unknown,
		implicit:  o.implicit,
	}
	for k, v := range o.index {
		c.index[k] = v
	}
	return c
}
