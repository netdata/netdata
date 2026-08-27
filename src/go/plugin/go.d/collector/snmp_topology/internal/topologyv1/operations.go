// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"github.com/netdata/netdata/go/plugins/pkg/topology/worklimit"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

type renderWork struct {
	limiter worklimit.Limiter
	err     error
}

func (w *renderWork) failure() error {
	if w == nil {
		return nil
	}
	return w.err
}

func (w *renderWork) charge(units uint64) bool {
	if w == nil || w.err != nil {
		return w == nil
	}
	w.err = w.limiter.Charge(units)
	return w.err == nil
}

func (w *renderWork) chargeMatch(match topologymodel.Match) bool {
	if w == nil || w.err != nil {
		return w == nil
	}
	w.err = topologymodel.ChargeMatch(w.limiter, match)
	return w.err == nil
}

// The trusted path receives caller-owned capacity so its backing array can remain
// local. The bounded path receives nil and charges before allocating.
func sortedRenderKeys[V any](w *renderWork, values map[string]V, keys []string) []string {
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
