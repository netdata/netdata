// SPDX-License-Identifier: GPL-3.0-or-later

package charttpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gopkg.in/yaml.v2"
)

func TestGroupClone(t *testing.T) {
	orig := richGroup()
	origBefore := mustMarshalV2(t, orig)

	clone := orig.Clone()

	// every deep-copied pointer must be a distinct object
	require.NotSame(t, orig.ChartDefaults, clone.ChartDefaults)
	require.NotSame(t, orig.ChartDefaults.Instances, clone.ChartDefaults.Instances)
	require.NotSame(t, orig.Charts[0].Instances, clone.Charts[0].Instances)
	require.NotSame(t, orig.Charts[0].Lifecycle, clone.Charts[0].Lifecycle)
	require.NotSame(t, orig.Charts[0].Lifecycle.Dimensions, clone.Charts[0].Lifecycle.Dimensions)
	require.NotSame(t, orig.Charts[0].Dimensions[0].Options, clone.Charts[0].Dimensions[0].Options)
	require.NotSame(t, orig.Groups[0].Charts[0].Dimensions[0].Options, clone.Groups[0].Charts[0].Dimensions[0].Options)

	// aggressively mutate every reference-typed field reachable from the clone
	clone.Family = "MUTATED"
	clone.Metrics[0] = "mutated"
	clone.Metrics = append(clone.Metrics, "extra")
	clone.ChartDefaults.LabelPromoted[0] = "mutated"
	clone.ChartDefaults.Instances.ByLabels[0] = "mutated"
	clone.Charts[0].Title = "MUTATED"
	clone.Charts[0].LabelPromoted[0] = "mutated"
	clone.Charts[0].Instances.ByLabels[0] = "mutated"
	clone.Charts[0].Lifecycle.MaxInstances = 999
	clone.Charts[0].Lifecycle.Dimensions.MaxDims = 999
	clone.Charts[0].Dimensions[0].Name = "MUTATED"
	clone.Charts[0].Dimensions[0].Options.Multiplier = 999
	clone.Groups[0].Family = "MUTATED"
	clone.Groups[0].Metrics[0] = "mutated"
	clone.Groups[0].Charts[0].Dimensions[0].Options.Divisor = 999

	// the clone must have actually changed, otherwise the isolation check is vacuous
	assert.NotEqual(t, origBefore, mustMarshalV2(t, clone))
	// the original must be untouched by any mutation of the clone
	assert.Equal(t, origBefore, mustMarshalV2(t, orig))
}

func TestGroupCloneMarshalsIdentically(t *testing.T) {
	g := richGroup()
	assert.Equal(t, mustMarshalV2(t, g), mustMarshalV2(t, g.Clone()))
}

// richGroup builds a fully populated, validation-clean group exercising every
// reference-typed field: metrics, chart_defaults (+instances), nested groups,
// charts with label_promotion/instances/lifecycle, and dimension options.
func richGroup() Group {
	return Group{
		Family:           "Root",
		ContextNamespace: "ns",
		Metrics:          []string{"metric_a"},
		ChartDefaults: &ChartDefaults{
			LabelPromoted: []string{"region"},
			Instances:     &Instances{ByLabels: []string{"resource_uid"}},
		},
		Charts: []Chart{
			{
				Title:         "Chart A",
				Context:       "chart_a",
				Units:         "units/s",
				Type:          "line",
				LabelPromoted: []string{"zone"},
				Instances:     &Instances{ByLabels: []string{"id"}},
				Lifecycle: &Lifecycle{
					MaxInstances:      10,
					ExpireAfterCycles: 5,
					Dimensions:        &DimensionLifecycle{MaxDims: 100, ExpireAfterCycles: 3},
				},
				Dimensions: []Dimension{
					{
						Selector: "metric_a",
						Name:     "a",
						Options:  &DimensionOptions{Multiplier: 2, Divisor: 1000, Hidden: true, Float: true},
					},
				},
			},
		},
		Groups: []Group{
			{
				Family:  "Child",
				Metrics: []string{"metric_c"},
				Charts: []Chart{
					{
						Title:   "Chart B",
						Context: "chart_b",
						Units:   "units",
						Type:    "area",
						Dimensions: []Dimension{
							{
								Selector: "metric_c",
								Name:     "b",
								Options:  &DimensionOptions{Divisor: 10},
							},
						},
					},
				},
			},
		},
	}
}

func mustMarshalV2(t *testing.T, v any) string {
	t.Helper()
	data, err := yaml.Marshal(v)
	require.NoError(t, err)
	return string(data)
}
