// SPDX-License-Identifier: GPL-3.0-or-later

package topologyenrich

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology/worklimit"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

type enrichmentWork struct {
	limiter worklimit.Limiter
	err     error
}

func (w *enrichmentWork) failure() error {
	if w == nil {
		return nil
	}
	return w.err
}

func (w *enrichmentWork) charge(units uint64) bool {
	if w == nil || w.err != nil {
		return w == nil
	}
	w.err = w.limiter.Charge(units)
	return w.err == nil
}

func (w *enrichmentWork) limiterValue() worklimit.Limiter {
	if w == nil {
		return nil
	}
	return w.limiter
}

func (w *enrichmentWork) fail(err error) {
	if w != nil && w.err == nil {
		w.err = err
	}
}

func (w *enrichmentWork) chargeStrings(values []string) bool {
	if w == nil || w.err != nil {
		return w == nil
	}
	w.err = worklimit.ChargeStrings(w.limiter, values)
	return w.err == nil
}

func sortEnrichmentSlice[S ~[]E, E any](w *enrichmentWork, values S, less func(i, j int) bool) bool {
	if w == nil {
		_ = worklimit.SortSlice(nil, values, less)
		return true
	}
	if w.err != nil {
		return false
	}
	w.err = worklimit.SortSlice(w.limiter, values, less)
	return w.err == nil
}

func sortEnrichmentSliceStableWithStringWork[S ~[]E, E any](
	w *enrichmentWork,
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

func sortEnrichmentStrings(w *enrichmentWork, values []string) bool {
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

// The trusted path receives caller-owned capacity so its backing array can remain
// local. The bounded path receives nil and charges before allocating.
func sortedEnrichmentKeys[V any](w *enrichmentWork, values map[string]V, keys []string) []string {
	if w == nil {
		for key := range values {
			keys = append(keys, key)
		}
		_ = worklimit.SortStrings(nil, keys)
		return keys
	}
	keys, err := worklimit.SortedStringKeys(func(units uint64) error {
		if w.err != nil {
			return w.err
		}
		w.err = w.limiter.Charge(units)
		return w.err
	}, values)
	if err != nil {
		return nil
	}
	return keys
}

func sortEnrichmentByPreparedStringKey[S ~[]E, E any](
	w *enrichmentWork,
	values S,
	prepare func(*enrichmentWork, E) (string, bool),
) bool {
	if w == nil {
		return worklimit.SortByPreparedStringKey(nil, values, func(value E) (string, error) {
			key, _ := prepare(nil, value)
			return key, nil
		}) == nil
	}
	if w.err != nil {
		return false
	}
	w.err = worklimit.SortByPreparedStringKey(w.limiter, values, func(value E) (string, error) {
		key, ok := prepare(w, value)
		if !ok {
			return "", w.err
		}
		return key, nil
	})
	return w.err == nil
}

func sortEnrichmentLinks(w *enrichmentWork, links []topologymodel.Link) bool {
	if w != nil && w.err != nil {
		return false
	}
	err := topologymodel.SortLinks(w.limiterValue(), links)
	if err != nil {
		w.fail(err)
		return false
	}
	return true
}

func sortEnrichmentActorsByID(w *enrichmentWork, actors []topologymodel.Actor) bool {
	return sortEnrichmentByPreparedStringKey(w, actors, func(work *enrichmentWork, actor topologymodel.Actor) (string, bool) {
		if work != nil && !work.chargeStrings([]string{actor.ActorID}) {
			return "", false
		}
		return strings.TrimSpace(actor.ActorID), true
	})
}
