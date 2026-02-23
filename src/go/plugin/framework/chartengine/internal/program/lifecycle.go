// SPDX-License-Identifier: GPL-3.0-or-later

package program

// LifecyclePolicy controls chart and dimension cardinality/expiry behavior.
//
// Counters are successful-cycle based by contract (failed collect attempts do not
// advance lifecycle clocks).
type LifecyclePolicy struct {
	MaxInstances      int
	ExpireAfterCycles int
	Dimensions        DimensionLifecyclePolicy
}

// DimensionLifecyclePolicy controls per-chart materialized dimension lifecycle.
type DimensionLifecyclePolicy struct {
	MaxDims           int
	ExpireAfterCycles int
}

func (p LifecyclePolicy) clone() LifecyclePolicy {
	out := p
	out.Dimensions = p.Dimensions
	return out
}
