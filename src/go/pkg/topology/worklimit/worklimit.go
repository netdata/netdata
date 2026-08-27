// SPDX-License-Identifier: GPL-3.0-or-later

// Package worklimit defines the optional work callback shared by topology
// build, shaping, enrichment, and rendering stages.
package worklimit

import (
	"errors"
	"math"
	"math/bits"
	"slices"
	"sort"
)

var ErrOverflow = errors.New("topology work estimate overflow")

// Limiter rejects work before a bounded stage executes. A nil limiter leaves
// trusted production callers on the unbounded path.
type Limiter func(units uint64) error

func (l Limiter) Charge(units uint64) error {
	if l == nil || units == 0 {
		return nil
	}
	return l(units)
}

func (l Limiter) ChargeProduct(factors ...uint64) error {
	if l == nil || len(factors) == 0 {
		return nil
	}
	product, err := Product(factors...)
	if err != nil {
		return err
	}
	return l.Charge(product)
}

// ChargeSort charges a conservative envelope for the standard-library sort
// operations used by topology code, including stable-sort merge movement.
func (l Limiter) ChargeSort(items uint64) error {
	if l == nil {
		return nil
	}
	units, err := SortEnvelope(items)
	if err != nil {
		return err
	}
	return l.Charge(units)
}

// ChargeStringComparisons charges the worst-case byte comparisons for a sort
// whose comparator can inspect at most maxItemBytes from either item.
func (l Limiter) ChargeStringComparisons(items, maxItemBytes uint64) error {
	if l == nil || items < 2 || maxItemBytes == 0 {
		return nil
	}
	envelope, err := SortEnvelope(items)
	if err != nil {
		return err
	}
	return l.ChargeProduct(envelope, 2, maxItemBytes)
}

// SortFunc charges the comparison envelope immediately before sorting.
func SortFunc[S ~[]E, E any](l Limiter, values S, compare func(a, b E) int) error {
	if err := l.ChargeSort(uint64(len(values))); err != nil {
		return err
	}
	slices.SortFunc(values, compare)
	return nil
}

// SortStableFunc charges the comparison envelope immediately before sorting.
func SortStableFunc[S ~[]E, E any](l Limiter, values S, compare func(a, b E) int) error {
	if err := l.ChargeSort(uint64(len(values))); err != nil {
		return err
	}
	slices.SortStableFunc(values, compare)
	return nil
}

// SortSlice charges the comparison envelope immediately before an index-based
// slice sort.
func SortSlice[S ~[]E, E any](l Limiter, values S, less func(i, j int) bool) error {
	if err := l.ChargeSort(uint64(len(values))); err != nil {
		return err
	}
	sort.Slice(values, less)
	return nil
}

// SortSliceStable charges the comparison envelope immediately before a stable
// index-based slice sort.
func SortSliceStable[S ~[]E, E any](l Limiter, values S, less func(i, j int) bool) error {
	if err := l.ChargeSort(uint64(len(values))); err != nil {
		return err
	}
	sort.SliceStable(values, less)
	return nil
}

// SortSliceWithStringWork charges repeated variable-width comparisons before
// executing an index-based sort. Source normalization/materialization must be
// charged by the caller before maxItemBytes is derived.
func SortSliceWithStringWork[S ~[]E, E any](
	l Limiter,
	values S,
	maxItemBytes uint64,
	less func(i, j int) bool,
) error {
	if err := l.ChargeStringComparisons(uint64(len(values)), maxItemBytes); err != nil {
		return err
	}
	return SortSlice(l, values, less)
}

// SortSliceStableWithStringWork is the stable form of
// SortSliceWithStringWork.
func SortSliceStableWithStringWork[S ~[]E, E any](
	l Limiter,
	values S,
	maxItemBytes uint64,
	less func(i, j int) bool,
) error {
	if err := l.ChargeStringComparisons(uint64(len(values)), maxItemBytes); err != nil {
		return err
	}
	return SortSliceStable(l, values, less)
}

// SortStableFuncWithStringWork charges repeated variable-width comparisons
// before executing a stable comparator sort.
func SortStableFuncWithStringWork[S ~[]E, E any](
	l Limiter,
	values S,
	maxItemBytes uint64,
	compare func(a, b E) int,
) error {
	if err := l.ChargeStringComparisons(uint64(len(values)), maxItemBytes); err != nil {
		return err
	}
	return SortStableFunc(l, values, compare)
}

type preparedStringKey[E any] struct {
	value E
	key   string
}

// SortByPreparedStringKey prepares each variable-cost key once for bounded
// callers. A nil limiter preserves the trusted path without a temporary vector.
func SortByPreparedStringKey[S ~[]E, E any](
	l Limiter,
	values S,
	prepare func(E) (string, error),
) error {
	return sortByPreparedStringKey(l, values, prepare, false)
}

// SortStableByPreparedStringKey is the stable form of SortByPreparedStringKey.
func SortStableByPreparedStringKey[S ~[]E, E any](
	l Limiter,
	values S,
	prepare func(E) (string, error),
) error {
	return sortByPreparedStringKey(l, values, prepare, true)
}

func sortByPreparedStringKey[S ~[]E, E any](
	l Limiter,
	values S,
	prepare func(E) (string, error),
	stable bool,
) error {
	if len(values) < 2 {
		return nil
	}
	if l == nil {
		var prepareErr error
		less := func(i, j int) bool {
			if prepareErr != nil {
				return false
			}
			left, err := prepare(values[i])
			if err != nil {
				prepareErr = err
				return false
			}
			right, err := prepare(values[j])
			if err != nil {
				prepareErr = err
				return false
			}
			return left < right
		}
		if stable {
			sort.SliceStable(values, less)
		} else {
			sort.Slice(values, less)
		}
		return prepareErr
	}

	if err := l.Charge(uint64(len(values))); err != nil {
		return err
	}
	prepared := make([]preparedStringKey[E], len(values))
	var maxKeyBytes uint64
	for i, value := range values {
		key, err := prepare(value)
		if err != nil {
			return err
		}
		prepared[i] = preparedStringKey[E]{value: value, key: key}
		if bytes := uint64(len(key)); bytes > maxKeyBytes {
			maxKeyBytes = bytes
		}
	}
	less := func(i, j int) bool { return prepared[i].key < prepared[j].key }
	if stable {
		if err := SortSliceStableWithStringWork(l, prepared, maxKeyBytes, less); err != nil {
			return err
		}
	} else if err := SortSliceWithStringWork(l, prepared, maxKeyBytes, less); err != nil {
		return err
	}
	if err := l.Charge(uint64(len(values))); err != nil {
		return err
	}
	for i := range prepared {
		values[i] = prepared[i].value
	}
	return nil
}

// ChargeStrings charges the string scan and key bytes immediately before the
// caller performs variable-cost string work.
func ChargeStrings[S ~[]string](l Limiter, values S) error {
	if l != nil {
		if err := l.Charge(uint64(len(values))); err != nil {
			return err
		}
		var bytes uint64
		for _, value := range values {
			var err error
			bytes, err = Sum(bytes, uint64(len(value)))
			if err != nil {
				return err
			}
		}
		if err := l.Charge(bytes); err != nil {
			return err
		}
	}
	return nil
}

// ChargeStringValues charges a fixed-cost scan and the source bytes of a
// collection before variable string work begins. It returns the maximum bytes
// one item can expose to a later comparator.
func ChargeStringValues[S ~[]E, E any](
	l Limiter,
	values S,
	itemBytes func(E) (uint64, error),
) (uint64, error) {
	if l == nil || len(values) == 0 {
		return 0, nil
	}
	if err := l.Charge(uint64(len(values))); err != nil {
		return 0, err
	}
	var total, maximum uint64
	for _, value := range values {
		bytes, err := itemBytes(value)
		if err != nil {
			return 0, err
		}
		total, err = Sum(total, bytes)
		if err != nil {
			return 0, err
		}
		if bytes > maximum {
			maximum = bytes
		}
	}
	if err := l.Charge(total); err != nil {
		return 0, err
	}
	return maximum, nil
}

// StringBytes returns the checked byte sum for a fixed tuple of strings.
func StringBytes(values ...string) (uint64, error) {
	var total uint64
	for _, value := range values {
		var err error
		total, err = Sum(total, uint64(len(value)))
		if err != nil {
			return 0, err
		}
	}
	return total, nil
}

// SortStrings charges the string scan, key bytes, and comparison envelope
// immediately before their respective operations.
func SortStrings[S ~[]string](l Limiter, values S) error {
	maximum, err := ChargeStringValues(l, values, func(value string) (uint64, error) {
		return uint64(len(value)), nil
	})
	if err != nil {
		return err
	}
	if err := l.ChargeStringComparisons(uint64(len(values)), maximum); err != nil {
		return err
	}
	if err := l.ChargeSort(uint64(len(values))); err != nil {
		return err
	}
	slices.Sort(values)
	return nil
}

// SortedStringKeys charges map materialization before allocating and fills a
// deterministic key vector whose scan, bytes, and sort are charged by the same
// limiter.
func SortedStringKeys[V any](l Limiter, values map[string]V) ([]string, error) {
	if err := l.Charge(uint64(len(values))); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	if err := SortStrings(l, keys); err != nil {
		return nil, err
	}
	return keys, nil
}

// SortEnvelope returns a conservative n*ceil(log2(n))^2 operation envelope.
// The extra logarithmic factor covers the movement performed by Go's stable
// merge sort and safely dominates the small insertion-sort blocks.
func SortEnvelope(items uint64) (uint64, error) {
	if items < 2 {
		return 0, nil
	}
	levels := uint64(bits.Len64(items - 1))
	return Product(items, levels, levels)
}

func Sum(values ...uint64) (uint64, error) {
	var sum uint64
	for _, value := range values {
		if sum > math.MaxUint64-value {
			return 0, ErrOverflow
		}
		sum += value
	}
	return sum, nil
}

func Product(factors ...uint64) (uint64, error) {
	if len(factors) == 0 {
		return 0, nil
	}
	product := uint64(1)
	for _, factor := range factors {
		if factor == 0 {
			return 0, nil
		}
		if product > math.MaxUint64/factor {
			return 0, ErrOverflow
		}
		product *= factor
	}
	return product, nil
}
