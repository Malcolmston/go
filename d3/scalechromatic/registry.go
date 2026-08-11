package scalechromatic

import (
	"sort"
	"strings"

	"github.com/malcolmston/d3/color"
)

// The registries exist for one reason: a scheme name is very often configuration
// rather than code. It arrives from a YAML file, a URL query parameter, a
// dashboard's dropdown. Without a lookup, every such call site grows a switch
// over forty names that has to be edited whenever this package gains one.
//
// Lookup is case-insensitive because configuration is written by people, and
// "viridis" is what they type. The canonical spelling — the one this package
// exports and [SchemeNames], [SchemeFamilyNames] and [InterpolatorNames] return
// — is d3's, minus its Scheme/Interpolate prefix:
// "Category10", "RdYlBu", "Viridis".
//
// A name can legitimately appear in more than one registry: "Blues" is both a
// [Family] and an interpolator, because a ColorBrewer family is published in
// both forms. Only the categorical schemes are exclusive to [Scheme], and only
// the computed ramps are exclusive to [Interpolator].

// Scheme returns the categorical scheme with the given name.
//
// Only the categorical schemes are here — Category10, Accent, Dark2, Paired,
// Pastel1, Pastel2, Set1, Set2, Set3, Tableau10, Observable10 — because they are
// the ones that are a single fixed list. A ColorBrewer family is not one list
// but seven or nine, and is looked up through [SchemeFamily] with the class
// count you want.
//
// The returned slice is the package's own; treat it as read-only, or copy it.
func Scheme(name string) ([]color.Color, bool) {
	cs, ok := schemeTable[strings.ToLower(strings.TrimSpace(name))]
	return cs, ok
}

// SchemeFamily returns the ColorBrewer family with the given name, indexable by
// class count.
//
//	if f, ok := scalechromatic.SchemeFamily(cfg.Palette); ok {
//		swatches = f.K(len(buckets))
//	}
//
// Note that [Family.K] can still return nil for a family that exists, when the
// class count is outside what ColorBrewer publishes. The two failures are worth
// distinguishing in an error message: an unknown palette name is a typo, and an
// unavailable k is a chart with too many or too few classes.
func SchemeFamily(name string) (Family, bool) {
	f, ok := familyTable[strings.ToLower(strings.TrimSpace(name))]
	return f, ok
}

// Interpolator returns the continuous ramp with the given name.
//
//	f, ok := scalechromatic.Interpolator(cfg.Ramp)
//	if !ok {
//		f = scalechromatic.InterpolateViridis
//	}
//	heat := scale.NewSequential(f).SetDomain(0, max)
//
// Every ColorBrewer family has an entry, as do the matplotlib ramps, Cividis,
// Turbo, the three cubehelix ramps, Rainbow and Sinebow.
func Interpolator(name string) (func(float64) color.Color, bool) {
	f, ok := interpolatorTable[strings.ToLower(strings.TrimSpace(name))]
	return f, ok
}

// Schemes returns every categorical scheme, keyed by its canonical name.
//
// The map is freshly built on each call and the caller may keep it, but the
// slices inside it are the package's own — copy one before mutating it.
func Schemes() map[string][]color.Color {
	out := make(map[string][]color.Color, len(schemeTable))
	for k, v := range schemeTable {
		out[canonicalNames[k]] = v
	}
	return out
}

// SchemeFamilies returns every ColorBrewer family, keyed by its canonical name.
// See [Schemes] for what is and is not copied.
func SchemeFamilies() map[string]Family {
	out := make(map[string]Family, len(familyTable))
	for k, v := range familyTable {
		out[canonicalNames[k]] = v
	}
	return out
}

// Interpolators returns every continuous ramp, keyed by its canonical name.
func Interpolators() map[string]func(float64) color.Color {
	out := make(map[string]func(float64) color.Color, len(interpolatorTable))
	for k, v := range interpolatorTable {
		out[canonicalNames[k]] = v
	}
	return out
}

// SchemeNames returns the canonical names of the categorical schemes, sorted.
func SchemeNames() []string { return names(schemeTable) }

// SchemeFamilyNames returns the canonical names of the ColorBrewer families,
// sorted.
func SchemeFamilyNames() []string { return names(familyTable) }

// InterpolatorNames returns the canonical names of the continuous ramps, sorted.
//
// Sorted, rather than in d3's declaration order, because the only thing this
// list is for is showing a human a choice, and a Go map has no order to preserve
// anyway.
func InterpolatorNames() []string { return names(interpolatorTable) }

func names[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, canonicalNames[k])
	}
	sort.Strings(out)
	return out
}
