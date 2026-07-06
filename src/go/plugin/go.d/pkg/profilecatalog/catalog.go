// SPDX-License-Identifier: GPL-3.0-or-later

package profilecatalog

import (
	"sort"
)

// Named pairs a profile with its identity (the file basename).
type Named[P any] struct {
	Name    string
	Profile P
}

// entry is one resolved catalog slot.
type entry[P any] struct {
	profile  P
	baseName string // exact basename (identity)
	isStock  bool   // effective origin: a user override of a stock profile is NOT stock
}

// Catalog is the resolved set of profiles keyed by normalized name and kept in
// discovery order. It is produced by Load and is safe for concurrent reads; a
// lazy profile type P is responsible for its own hydration safety. Lookups
// (Get/Has/HasStock/EffectiveIsStock) normalize their argument with the same
// function Load used, so a case-insensitive catalog resolves "HAProxy" to
// "haproxy" transparently.
type Catalog[P any] struct {
	byKey       map[string]entry[P]
	orderedKeys []string            // normalized keys, in discovery order
	hadStock    map[string]struct{} // normalized keys that had a stock profile (even if later overridden by a user profile)
	normalize   func(string) string
}

func newCatalog[P any](normalize func(string) string) Catalog[P] {
	return Catalog[P]{
		byKey:     make(map[string]entry[P]),
		hadStock:  make(map[string]struct{}),
		normalize: normalize,
	}
}

// Len returns the number of profiles.
func (c Catalog[P]) Len() int { return len(c.byKey) }

// Empty reports whether the catalog holds no profiles.
func (c Catalog[P]) Empty() bool { return len(c.byKey) == 0 }

// Get returns the profile for the given name (matched via the catalog's
// normalize function) and whether it exists.
func (c Catalog[P]) Get(name string) (P, bool) {
	e, ok := c.byKey[c.key(name)]
	return e.profile, ok
}

// Has reports whether a profile with the given name exists.
func (c Catalog[P]) Has(name string) bool {
	_, ok := c.byKey[c.key(name)]
	return ok
}

// InOrder returns all profiles in deterministic discovery order.
func (c Catalog[P]) InOrder() []Named[P] {
	if len(c.orderedKeys) == 0 {
		return nil
	}
	out := make([]Named[P], 0, len(c.orderedKeys))
	for _, k := range c.orderedKeys {
		e := c.byKey[k]
		out = append(out, Named[P]{Name: e.baseName, Profile: e.profile})
	}
	return out
}

// Sorted returns all profiles sorted by basename.
func (c Catalog[P]) Sorted() []Named[P] {
	if len(c.byKey) == 0 {
		return nil
	}
	out := make([]Named[P], 0, len(c.byKey))
	for _, e := range c.byKey {
		out = append(out, Named[P]{Name: e.baseName, Profile: e.profile})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// HasStock reports whether a stock profile with the given name was found during
// loading, even if a user profile later overrode it.
func (c Catalog[P]) HasStock(name string) bool {
	_, ok := c.hadStock[c.key(name)]
	return ok
}

// StockNames returns, sorted, the basenames that had a stock profile (even if a
// user profile overrode them).
func (c Catalog[P]) StockNames() []string {
	if len(c.hadStock) == 0 {
		return nil
	}
	out := make([]string, 0, len(c.hadStock))
	for k := range c.hadStock {
		out = append(out, c.byKey[k].baseName)
	}
	sort.Strings(out)
	return out
}

// EffectiveIsStock reports whether the profile currently held for the given name
// came from a stock directory. A user override of a stock profile is NOT stock.
func (c Catalog[P]) EffectiveIsStock(name string) bool {
	e, ok := c.byKey[c.key(name)]
	return ok && e.isStock
}

// key normalizes a lookup name the same way Load normalized stored keys. A
// zero-value catalog (no normalize set) falls back to identity.
func (c Catalog[P]) key(name string) string {
	if c.normalize == nil {
		return name
	}
	return c.normalize(name)
}
