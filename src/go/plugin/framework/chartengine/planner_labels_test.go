// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChartLabelAccumulatorIntersectsLabels(t *testing.T) {
	tests := map[string]struct {
		observed []map[string]string
		want     map[string]string
	}{
		"auto intersection keeps only common labels and excludes dimension key": {
			observed: []map[string]string{
				{
					collectJobLabel:  "mysql-local",
					"env":            "prod",
					"instance":       "db1",
					"mode":           "read",
					"region":         "us",
					"selector_fixed": "x",
				},
				{
					collectJobLabel:  "mysql-local",
					"env":            "prod",
					"instance":       "db1",
					"mode":           "write",
					"region":         "us",
					"selector_fixed": "x",
				},
				{
					collectJobLabel:  "mysql-local",
					"env":            "prod",
					"instance":       "db1",
					"mode":           "read",
					"region":         "eu",
					"selector_fixed": "x",
				},
			},
			want: map[string]string{
				"env":      "prod",
				"instance": "db1",
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			chart := program.Chart{
				Identity: program.ChartIdentity{
					InstanceByLabels: []program.InstanceLabelSelector{
						{Key: "instance"},
					},
				},
				Labels: program.LabelPolicy{
					Mode: program.PromotionModeAutoIntersection,
					Exclusions: program.LabelExclusions{
						SelectorConstrainedKeys: []string{"selector_fixed"},
					},
				},
			}

			acc := newChartLabelAccumulator(chart)
			require.NotNil(t, acc)

			for _, labels := range tc.observed {
				err := acc.observe(sortedLabelView(labels), "mode")
				require.NoError(t, err)
			}

			got, err := acc.materialize()
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			assert.NotContains(t, got, "mode")
			assert.NotContains(t, got, "selector_fixed")
			assert.NotContains(t, got, collectJobLabel)
		})
	}
}

func TestCompileInstanceLabelPlanExcludeWinsRegardlessOfTokenOrder(t *testing.T) {
	tests := map[string]struct {
		selectors []program.InstanceLabelSelector
	}{
		"exclude after explicit": {
			selectors: []program.InstanceLabelSelector{
				{Key: "host"},
				{Exclude: true, Key: "host"},
				{IncludeAll: true},
			},
		},
		"exclude before explicit": {
			selectors: []program.InstanceLabelSelector{
				{Exclude: true, Key: "host"},
				{Key: "host"},
				{IncludeAll: true},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			plan := compileInstanceLabelPlan(program.ChartIdentity{
				InstanceByLabels: tc.selectors,
			})
			assert.True(t, plan.includeAll)
			assert.Empty(t, plan.explicitKeys)
			assert.Empty(t, plan.explicitSet)
			assert.Contains(t, plan.excludeSet, "host")
		})
	}
}
