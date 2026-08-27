// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import "github.com/netdata/netdata/go/plugins/pkg/topology/worklimit"

type projectionWork struct {
	limiter worklimit.Limiter
	err     error
}

func (w *projectionWork) failure() error {
	if w == nil {
		return nil
	}
	return w.err
}

func (w *projectionWork) charge(units uint64) bool {
	if w == nil || w.err != nil {
		return w == nil
	}
	w.err = w.limiter.Charge(units)
	return w.err == nil
}

func (w *projectionWork) chargeProduct(factors ...uint64) bool {
	if w == nil || w.err != nil {
		return w == nil
	}
	w.err = w.limiter.ChargeProduct(factors...)
	return w.err == nil
}

func (w *projectionWork) chargeStrings(values []string) bool {
	if w == nil {
		return true
	}
	if w.err != nil {
		return false
	}
	w.err = worklimit.ChargeStrings(w.limiter, values)
	return w.err == nil
}

func chargeProjectionStringValues[S ~[]E, E any](
	w *projectionWork,
	values S,
	itemBytes func(E) (uint64, error),
) (uint64, bool) {
	if w == nil {
		return 0, true
	}
	if w.err != nil {
		return 0, false
	}
	maximum, err := worklimit.ChargeStringValues(w.limiter, values, itemBytes)
	w.err = err
	return maximum, err == nil
}

func sortProjectionSliceStableWithStringWork[S ~[]E, E any](
	w *projectionWork,
	values S,
	maxItemBytes uint64,
	less func(i, j int) bool,
) bool {
	if w == nil {
		_ = worklimit.SortSliceStableWithStringWork(nil, values, maxItemBytes, less)
		return true
	}
	if w.err != nil {
		return false
	}
	w.err = worklimit.SortSliceStableWithStringWork(w.limiter, values, maxItemBytes, less)
	return w.err == nil
}

func sortProjectionStrings[S ~[]string](w *projectionWork, values S) bool {
	if w == nil {
		_ = worklimit.SortStrings(nil, values)
		return true
	}
	if w.err != nil {
		return false
	}
	w.err = worklimit.SortStrings(w.limiter, values)
	return w.err == nil
}

func sortedProjectionKeys[V any](w *projectionWork, values map[string]V, keys []string) []string {
	if w == nil {
		for key := range values {
			keys = append(keys, key)
		}
		_ = worklimit.SortStrings(nil, keys)
		return keys
	}
	if w.err != nil {
		return nil
	}
	keys, err := worklimit.SortedStringKeys(w.limiter, values)
	w.err = err
	return keys
}

func sortProjectionByPreparedStringKeyStable[S ~[]E, E any](
	w *projectionWork,
	values S,
	prepare func(*projectionWork, E) (string, bool),
) bool {
	if w == nil {
		return worklimit.SortStableByPreparedStringKey(nil, values, func(value E) (string, error) {
			key, _ := prepare(nil, value)
			return key, nil
		}) == nil
	}
	if w.err != nil {
		return false
	}
	w.err = worklimit.SortStableByPreparedStringKey(w.limiter, values, func(value E) (string, error) {
		key, ok := prepare(w, value)
		if !ok {
			return "", w.err
		}
		return key, nil
	})
	return w.err == nil
}
