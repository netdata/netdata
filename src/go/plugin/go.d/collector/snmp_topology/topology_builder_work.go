// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import "github.com/netdata/netdata/go/plugins/pkg/topology/worklimit"

func (c *topologyBuilder) chargeWork(units uint64) bool {
	if c == nil || c.workErr != nil {
		return c != nil && c.workErr == nil
	}
	c.workErr = c.workLimiter.Charge(units)
	return c.workErr == nil
}

func (c *topologyBuilder) sortStrings(values []string) bool {
	if c == nil || c.workErr != nil {
		return c != nil && c.workErr == nil
	}
	c.workErr = worklimit.SortStrings(c.workLimiter, values)
	return c.workErr == nil
}

func sortBuilderSlice[S ~[]E, E any](c *topologyBuilder, values S, less func(i, j int) bool) bool {
	if c == nil || c.workErr != nil {
		return c != nil && c.workErr == nil
	}
	c.workErr = worklimit.SortSlice(c.workLimiter, values, less)
	return c.workErr == nil
}

func sortBuilderFunc[S ~[]E, E any](c *topologyBuilder, values S, compare func(a, b E) int) bool {
	if c == nil || c.workErr != nil {
		return c != nil && c.workErr == nil
	}
	c.workErr = worklimit.SortFunc(c.workLimiter, values, compare)
	return c.workErr == nil
}

// The trusted path receives caller-owned capacity so its backing array can remain
// local. The bounded path receives nil and charges before allocating.
func sortedBuilderKeys[V any](c *topologyBuilder, values map[string]V, keys []string) []string {
	if c == nil || c.workErr != nil {
		return nil
	}
	if c.workLimiter == nil {
		for key := range values {
			keys = append(keys, key)
		}
		_ = worklimit.SortStrings(nil, keys)
		return keys
	}
	keys, err := worklimit.SortedStringKeys(c.workLimiter, values)
	c.workErr = err
	return keys
}
