// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"maps"
	"slices"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

// InstanceLabelPolicy is the normalized runtime interpretation of
// instances.by_labels.
type InstanceLabelPolicy struct {
	ExplicitKeys []string
	ExcludedKeys []string
	IncludeAll   bool
}

// ResolveInstanceLabelPolicy compiles instances.by_labels through the same
// parser and exclusion-precedence rules used by the production planner.
func ResolveInstanceLabelPolicy(instances *charttpl.Instances) (InstanceLabelPolicy, error) {
	selectors, err := compileInstanceByLabels(instances)
	if err != nil {
		return InstanceLabelPolicy{}, err
	}
	plan := compileInstanceLabelPlan(program.ChartIdentity{InstanceByLabels: selectors})
	return InstanceLabelPolicy{
		ExplicitKeys: slices.Clone(plan.explicitKeys),
		ExcludedKeys: slices.Sorted(maps.Keys(plan.excludeSet)),
		IncludeAll:   plan.includeAll,
	}, nil
}
