// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
)

// defaultChartLifecyclePolicy is the baseline lifecycle policy used when
// template lifecycle fields are omitted.
//
// Keep defaults centralized in this file so changes are easy to discover.
var defaultChartLifecyclePolicy = program.LifecyclePolicy{
	MaxInstances:      0,
	ExpireAfterCycles: 5,
	Dimensions: program.DimensionLifecyclePolicy{
		MaxDims:           0,
		ExpireAfterCycles: 0,
	},
}

func defaultChartLifecyclePolicyCopy() program.LifecyclePolicy {
	out := defaultChartLifecyclePolicy
	out.Dimensions = defaultChartLifecyclePolicy.Dimensions
	return out
}
