// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"maps"
	"slices"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

// InstanceLabelPolicy is the normalized runtime interpretation of
// instances.by_labels and instances.optional_by_labels.
type InstanceLabelPolicy struct {
	RequiredKeys []string
	OptionalKeys []string
	ExcludedKeys []string
	IncludeAll   bool
}

// ResolveInstanceLabelPolicy compiles required and optional instance labels
// through the same parser and exclusion-precedence rules used by the production planner.
func ResolveInstanceLabelPolicy(instances *charttpl.Instances) (InstanceLabelPolicy, error) {
	labels, err := compileInstanceLabels(instances)
	if err != nil {
		return InstanceLabelPolicy{}, err
	}
	plan := compileInstanceLabelPlan(program.ChartIdentity{
		InstanceByLabels: labels.required,
		OptionalByLabels: labels.optional,
	})
	return InstanceLabelPolicy{
		RequiredKeys: slices.Clone(plan.explicitKeys),
		OptionalKeys: slices.Clone(plan.optionalKeys),
		ExcludedKeys: slices.Sorted(maps.Keys(plan.excludeSet)),
		IncludeAll:   plan.includeAll,
	}, nil
}
