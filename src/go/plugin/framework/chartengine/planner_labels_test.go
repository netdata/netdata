// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"testing"

	program2 "github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
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
					"_collect_job":   "mysql-local",
					"env":            "prod",
					"instance":       "db1",
					"mode":           "read",
					"region":         "us",
					"selector_fixed": "x",
				},
				{
					"_collect_job":   "mysql-local",
					"env":            "prod",
					"instance":       "db1",
					"mode":           "write",
					"region":         "us",
					"selector_fixed": "x",
				},
				{
					"_collect_job":   "mysql-local",
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
			chart := program2.Chart{
				Identity: program2.ChartIdentity{
					InstanceByLabels: []program2.InstanceLabelSelector{
						{Key: "instance"},
					},
				},
				Labels: program2.LabelPolicy{
					Mode: program2.PromotionModeAutoIntersection,
					Exclusions: program2.LabelExclusions{
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
			assert.NotContains(t, got, "_collect_job")
		})
	}
}
