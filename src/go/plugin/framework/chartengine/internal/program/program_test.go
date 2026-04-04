// SPDX-License-Identifier: GPL-3.0-or-later

package program

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type trueMatcher struct{}

func (trueMatcher) Matches(_ string, _ SelectorLabels) bool { return true }

func TestNewProgramScenarios(t *testing.T) {
	tests := map[string]struct {
		version string
		metrics []string
		charts  []Chart
		wantErr bool
		assert  func(t *testing.T, p *Program)
	}{
		"builds immutable snapshot and deterministic metric ordering": {
			version: "v1",
			metrics: []string{"windows_tx_total", "windows_rx_total", "windows_tx_total"},
			charts: []Chart{
				sampleChart("win.nic.traffic"),
			},
			assert: func(t *testing.T, p *Program) {
				t.Helper()

				gotMetrics := p.MetricNames()
				require.Len(t, gotMetrics, 2)
				assert.Equal(t, []string{"windows_rx_total", "windows_tx_total"}, gotMetrics)

				gotCharts := p.Charts()
				require.Len(t, gotCharts, 1)

				// Mutate returned chart copy and ensure Program internals are unaffected.
				gotCharts[0].Meta.Title = "mutated-title"
				again, ok := p.Chart("win.nic.traffic")
				require.True(t, ok, "Chart() did not return existing template id")
				assert.Equal(t, "Network traffic", again.Meta.Title)
			},
		},
		"rejects duplicate chart template IDs": {
			version: "v1",
			metrics: []string{"windows_rx_total"},
			charts: []Chart{
				sampleChart("dup"),
				sampleChart("dup"),
			},
			wantErr: true,
		},
		"rejects empty metric name": {
			version: "v1",
			metrics: []string{"windows_rx_total", ""},
			charts: []Chart{
				sampleChart("win.nic.traffic"),
			},
			wantErr: true,
		},
		"rejects missing selector matcher": {
			version: "v1",
			metrics: []string{"windows_rx_total"},
			charts: []Chart{
				{
					TemplateID: "invalid",
					Meta: ChartMeta{
						Title:     "Invalid",
						Family:    "net",
						Context:   "win.invalid",
						Units:     "bytes/s",
						Algorithm: AlgorithmIncremental,
						Type:      ChartTypeLine,
					},
					Identity: ChartIdentity{
						IDTemplate: Template{Raw: "invalid"},
						Static:     true,
					},
					Labels: LabelPolicy{
						Mode:       PromotionModeAutoIntersection,
						Precedence: DefaultLabelPrecedence(),
					},
					Lifecycle:       LifecyclePolicy{},
					CollisionReduce: ReduceSum,
					Dimensions: []Dimension{
						{
							Selector: SelectorBinding{
								Expression: "windows_rx_total",
							},
							NameTemplate: Template{Raw: "received"},
						},
					},
				},
			},
			wantErr: true,
		},
		"rejects invalid chart type": {
			version: "v1",
			metrics: []string{"windows_rx_total"},
			charts: []Chart{
				func() Chart {
					chart := sampleChart("invalid-type")
					chart.Meta.Type = ChartType("bars")
					return chart
				}(),
			},
			wantErr: true,
		},
		"rejects missing label promotion mode": {
			version: "v1",
			metrics: []string{"windows_rx_total"},
			charts: []Chart{
				func() Chart {
					chart := sampleChart("missing-label-mode")
					chart.Labels.Mode = ""
					return chart
				}(),
			},
			wantErr: true,
		},
		"rejects negation-only instance selectors": {
			version: "v1",
			metrics: []string{"windows_rx_total"},
			charts: []Chart{
				func() Chart {
					chart := sampleChart("negation-only-instance-selectors")
					chart.Identity.InstanceByLabels = []InstanceLabelSelector{
						{Exclude: true, Key: "nic"},
					}
					return chart
				}(),
			},
			wantErr: true,
		},
		"rejects malformed instance selector keys": {
			version: "v1",
			metrics: []string{"windows_rx_total"},
			charts: []Chart{
				func() Chart {
					chart := sampleChart("malformed-instance-selector")
					chart.Identity.InstanceByLabels = []InstanceLabelSelector{
						{Key: "nic"},
						{Exclude: true, Key: " host"},
					}
					return chart
				}(),
			},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			p, err := New(tc.version, 42, tc.metrics, tc.charts)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tc.assert != nil {
				tc.assert(t, p)
			}
		})
	}
}

func TestValidateInstanceLabelSelectorsReportsJoinedErrors(t *testing.T) {
	err := validateInstanceLabelSelectors([]InstanceLabelSelector{
		{Exclude: true, Key: " host"},
		{},
	})

	require.Error(t, err)
	assert.ErrorContains(t, err, "instance selector[0]: exclude selector key must be trimmed")
	assert.ErrorContains(t, err, "instance selector[1]: selector is empty")
	assert.ErrorContains(t, err, "instance selectors must include at least one positive selector")
}

func TestValidateDimensionReportsJoinedErrors(t *testing.T) {
	err := validateDimension(Dimension{})

	require.Error(t, err)
	assert.ErrorContains(t, err, "selector expression is required")
	assert.ErrorContains(t, err, "selector matcher is required")
	assert.ErrorContains(t, err, "dimension name is required")
}

func TestNewProgramReportsJoinedChartErrors(t *testing.T) {
	chart := sampleChart("invalid-multi")
	chart.Meta.Context = ""
	chart.Meta.Units = ""
	chart.Identity.InstanceByLabels = []InstanceLabelSelector{
		{Exclude: true, Key: "nic"},
	}
	chart.CollisionReduce = ""
	chart.Dimensions = []Dimension{{}}

	_, err := New("v1", 42, []string{"windows_rx_total"}, []Chart{chart})
	require.Error(t, err)
	assert.ErrorContains(t, err, "context is required")
	assert.ErrorContains(t, err, "units is required")
	assert.ErrorContains(t, err, "identity: instance selectors must include at least one positive selector")
	assert.ErrorContains(t, err, "collision reduce op is required")
	assert.ErrorContains(t, err, "dimension[0]: selector expression is required")
	assert.ErrorContains(t, err, "selector matcher is required")
	assert.ErrorContains(t, err, "dimension name is required")
}

func sampleChart(templateID string) Chart {
	return Chart{
		TemplateID: templateID,
		Meta: ChartMeta{
			Title:     "Network traffic",
			Family:    "net",
			Context:   "win.nic_traffic",
			Units:     "bytes/s",
			Algorithm: AlgorithmIncremental,
			Type:      ChartTypeLine,
			Priority:  1000,
		},
		Identity: ChartIdentity{
			IDTemplate: Template{Raw: "win_nic_{nic}_traffic"},
			InstanceByLabels: []InstanceLabelSelector{
				{Key: "nic"},
			},
		},
		Labels: LabelPolicy{
			Mode:        PromotionModeAutoIntersection,
			PromoteKeys: []string{"interface_type"},
			Exclusions: LabelExclusions{
				SelectorConstrainedKeys: []string{"direction"},
			},
			Precedence: DefaultLabelPrecedence(),
		},
		Lifecycle: LifecyclePolicy{
			MaxInstances:      512,
			ExpireAfterCycles: 10,
			Dimensions: DimensionLifecyclePolicy{
				MaxDims:           16,
				ExpireAfterCycles: 10,
			},
		},
		CollisionReduce: ReduceSum,
		Dimensions: []Dimension{
			{
				Selector: SelectorBinding{
					Expression:           "windows_rx_total",
					Matcher:              trueMatcher{},
					MetricNames:          []string{"windows_rx_total"},
					ConstrainedLabelKeys: []string{"direction"},
				},
				NameTemplate: Template{
					Raw: "received",
				},
			},
			{
				Selector: SelectorBinding{
					Expression:           "windows_tx_total",
					Matcher:              trueMatcher{},
					MetricNames:          []string{"windows_tx_total"},
					ConstrainedLabelKeys: []string{"direction"},
				},
				NameTemplate: Template{
					Raw: "sent",
				},
			},
		},
	}
}
