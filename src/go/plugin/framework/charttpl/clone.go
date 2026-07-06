// SPDX-License-Identifier: GPL-3.0-or-later

package charttpl

import "slices"

// Clone returns a typed deep copy of the group. Mutating the clone — including
// nested groups, charts, dimensions, and any options/defaults pointers — never
// affects the receiver, so a collector can clone a shared catalog template and
// inject per-job fields without corrupting the catalog.
func (g Group) Clone() Group {
	out := g
	out.Metrics = slices.Clone(g.Metrics)
	out.ChartDefaults = cloneChartDefaults(g.ChartDefaults)
	out.Groups = cloneSlice(g.Groups, Group.Clone)
	out.Charts = cloneSlice(g.Charts, cloneChart)
	return out
}

func cloneChartDefaults(in *ChartDefaults) *ChartDefaults {
	if in == nil {
		return nil
	}
	return &ChartDefaults{
		LabelPromoted: slices.Clone(in.LabelPromoted),
		Instances:     cloneInstances(in.Instances),
	}
}

func cloneChart(in Chart) Chart {
	out := in
	out.LabelPromoted = slices.Clone(in.LabelPromoted)
	out.Instances = cloneInstances(in.Instances)
	out.Lifecycle = cloneLifecycle(in.Lifecycle)
	out.Dimensions = cloneSlice(in.Dimensions, cloneDimension)
	return out
}

func cloneLifecycle(in *Lifecycle) *Lifecycle {
	if in == nil {
		return nil
	}
	out := *in
	if in.Dimensions != nil {
		dims := *in.Dimensions
		out.Dimensions = &dims
	}
	return &out
}

func cloneDimension(in Dimension) Dimension {
	out := in
	if in.Options != nil {
		opts := *in.Options
		out.Options = &opts
	}
	return out
}

// cloneSlice deep-copies a slice by cloning each element, preserving a nil
// input as a nil output so a marshaled clone is byte-identical to the original.
func cloneSlice[T any](in []T, clone func(T) T) []T {
	if in == nil {
		return nil
	}
	out := make([]T, len(in))
	for i := range in {
		out[i] = clone(in[i])
	}
	return out
}
