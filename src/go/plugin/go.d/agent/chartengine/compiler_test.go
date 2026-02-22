// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/chartengine/internal/program"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/charttpl"
)

type mapLabelView map[string]string

func (m mapLabelView) Len() int { return len(m) }

func (m mapLabelView) Get(key string) (string, bool) {
	value, ok := m[key]
	return value, ok
}

func (m mapLabelView) Range(fn func(key, value string) bool) {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !fn(key, m[key]) {
			return
		}
	}
}

func TestCompileScenarios(t *testing.T) {
	tests := map[string]struct {
		spec    charttpl.Spec
		rev     uint64
		wantErr bool
		errLike string
		assert  func(t *testing.T, p *program.Program)
	}{
		"compiles dimension options under options block": {
			spec: charttpl.Spec{
				Version: charttpl.VersionV1,
				Groups: []charttpl.Group{
					{
						Family:  "Service",
						Metrics: []string{"svc_requests_total"},
						Charts: []charttpl.Chart{
							{
								Title:   "Requests",
								Context: "requests",
								Units:   "requests/s",
								Dimensions: []charttpl.Dimension{
									{
										Selector: "svc_requests_total",
										Name:     "total",
										Options: &charttpl.DimensionOptions{
											Hidden:     true,
											Multiplier: -8,
											Divisor:    1000,
										},
									},
								},
							},
						},
					},
				},
			},
			assert: func(t *testing.T, p *program.Program) {
				t.Helper()
				charts := p.Charts()
				require.Len(t, charts, 1)
				require.Len(t, charts[0].Dimensions, 1)
				assert.True(t, charts[0].Dimensions[0].Hidden)
				assert.Equal(t, -8, charts[0].Dimensions[0].Multiplier)
				assert.Equal(t, 1000, charts[0].Dimensions[0].Divisor)
			},
		},
		"applies default lifecycle when omitted": {
			spec: charttpl.Spec{
				Version: charttpl.VersionV1,
				Groups: []charttpl.Group{
					{
						Family:  "Service",
						Metrics: []string{"svc_requests_total"},
						Charts: []charttpl.Chart{
							{
								Title:   "Requests",
								Context: "requests",
								Units:   "requests/s",
								Dimensions: []charttpl.Dimension{
									{Selector: "svc_requests_total", Name: "total"},
								},
							},
						},
					},
				},
			},
			assert: func(t *testing.T, p *program.Program) {
				t.Helper()
				charts := p.Charts()
				require.Len(t, charts, 1)
				assert.Equal(t, 0, charts[0].Lifecycle.MaxInstances)
				assert.Equal(t, 5, charts[0].Lifecycle.ExpireAfterCycles)
				assert.Equal(t, 0, charts[0].Lifecycle.Dimensions.MaxDims)
				assert.Equal(t, 0, charts[0].Lifecycle.Dimensions.ExpireAfterCycles)
			},
		},
		"keeps default chart expiry when lifecycle is present without expire_after_cycles": {
			spec: charttpl.Spec{
				Version: charttpl.VersionV1,
				Groups: []charttpl.Group{
					{
						Family:  "Service",
						Metrics: []string{"svc_requests_total"},
						Charts: []charttpl.Chart{
							{
								Title:   "Requests",
								Context: "requests",
								Units:   "requests/s",
								Lifecycle: &charttpl.Lifecycle{
									MaxInstances: 32,
									Dimensions: &charttpl.DimensionLifecycle{
										MaxDims: 16,
									},
								},
								Dimensions: []charttpl.Dimension{
									{Selector: "svc_requests_total", Name: "total"},
								},
							},
						},
					},
				},
			},
			assert: func(t *testing.T, p *program.Program) {
				t.Helper()
				charts := p.Charts()
				require.Len(t, charts, 1)
				assert.Equal(t, 32, charts[0].Lifecycle.MaxInstances)
				assert.Equal(t, 5, charts[0].Lifecycle.ExpireAfterCycles)
				assert.Equal(t, 16, charts[0].Lifecycle.Dimensions.MaxDims)
				assert.Equal(t, 0, charts[0].Lifecycle.Dimensions.ExpireAfterCycles)
			},
		},
		"overrides default chart expiry when expire_after_cycles is set": {
			spec: charttpl.Spec{
				Version: charttpl.VersionV1,
				Groups: []charttpl.Group{
					{
						Family:  "Service",
						Metrics: []string{"svc_requests_total"},
						Charts: []charttpl.Chart{
							{
								Title:   "Requests",
								Context: "requests",
								Units:   "requests/s",
								Lifecycle: &charttpl.Lifecycle{
									ExpireAfterCycles: 9,
								},
								Dimensions: []charttpl.Dimension{
									{Selector: "svc_requests_total", Name: "total"},
								},
							},
						},
					},
				},
			},
			assert: func(t *testing.T, p *program.Program) {
				t.Helper()
				charts := p.Charts()
				require.Len(t, charts, 1)
				assert.Equal(t, 9, charts[0].Lifecycle.ExpireAfterCycles)
			},
		},
		"compiles nested groups with inherited metric visibility and inferred algorithm": {
			rev: 42,
			spec: charttpl.Spec{
				Version:          charttpl.VersionV1,
				ContextNamespace: "mysql",
				Groups: []charttpl.Group{
					{
						Family:           "Database",
						ContextNamespace: "database",
						Metrics:          []string{"mysql_queries_total"},
						Groups: []charttpl.Group{
							{
								Family: "Throughput",
								Charts: []charttpl.Chart{
									{
										Title:   "Queries",
										Context: "queries_total",
										Units:   "queries/s",
										Dimensions: []charttpl.Dimension{
											{
												Selector: "mysql_queries_total",
												Name:     "total",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			assert: func(t *testing.T, p *program.Program) {
				t.Helper()
				assert.Equal(t, charttpl.VersionV1, p.Version())
				assert.Equal(t, uint64(42), p.Revision())
				assert.Equal(t, []string{"mysql_queries_total"}, p.MetricNames())

				charts := p.Charts()
				require.Len(t, charts, 1)
				assert.Equal(t, "Database/Throughput", charts[0].Meta.Family)
				assert.Equal(t, "mysql.database.queries_total", charts[0].Meta.Context)
				assert.Equal(t, program.AlgorithmIncremental, charts[0].Meta.Algorithm)
				assert.Equal(t, program.ChartTypeLine, charts[0].Meta.Type)
				assert.True(t, charts[0].Identity.Static)

				assert.True(t, charts[0].Dimensions[0].Selector.Matcher.Matches("mysql_queries_total", mapLabelView{}))
				assert.False(t, charts[0].Dimensions[0].Selector.Matcher.Matches("mysql_queries", mapLabelView{}))
			},
		},
		"infer histogram dimension name_from_label and parse instance selectors": {
			rev: 7,
			spec: charttpl.Spec{
				Version: charttpl.VersionV1,
				Groups: []charttpl.Group{
					{
						Family:  "Latency",
						Metrics: []string{"mysql_query_duration_seconds_bucket"},
						Charts: []charttpl.Chart{
							{
								ID:      "mysql_query_duration_seconds_bucket",
								Title:   "Query duration buckets",
								Context: "query_duration_bucket",
								Units:   "observations",
								Instances: &charttpl.Instances{
									ByLabels: []string{"*", "!le"},
								},
								Dimensions: []charttpl.Dimension{
									{
										Selector: "mysql_query_duration_seconds_bucket",
									},
								},
							},
						},
					},
				},
			},
			assert: func(t *testing.T, p *program.Program) {
				t.Helper()
				charts := p.Charts()
				require.Len(t, charts, 1)
				assert.False(t, charts[0].Identity.Static)
				require.Len(t, charts[0].Identity.InstanceByLabels, 2)
				assert.True(t, charts[0].Identity.InstanceByLabels[0].IncludeAll)
				assert.True(t, charts[0].Identity.InstanceByLabels[1].Exclude)
				assert.Equal(t, "le", charts[0].Identity.InstanceByLabels[1].Key)

				require.Len(t, charts[0].Dimensions, 1)
				assert.Equal(t, "", charts[0].Dimensions[0].NameFromLabel)
				assert.True(t, charts[0].Dimensions[0].InferNameFromSeriesMeta)
				assert.True(t, charts[0].Dimensions[0].Dynamic)
				assert.Equal(t, program.AlgorithmIncremental, charts[0].Meta.Algorithm)
			},
		},
		"infer stateset dimension naming from runtime series metadata": {
			rev: 8,
			spec: charttpl.Spec{
				Version: charttpl.VersionV1,
				Groups: []charttpl.Group{
					{
						Family:  "Replication",
						Metrics: []string{"system_status"},
						Charts: []charttpl.Chart{
							{
								Title:   "Replication state",
								Context: "replication_state",
								Units:   "state",
								Dimensions: []charttpl.Dimension{
									{
										Selector: "system_status",
									},
								},
							},
						},
					},
				},
			},
			assert: func(t *testing.T, p *program.Program) {
				t.Helper()
				charts := p.Charts()
				require.Len(t, charts, 1)
				require.Len(t, charts[0].Dimensions, 1)
				assert.Equal(t, "", charts[0].Dimensions[0].NameFromLabel)
				assert.True(t, charts[0].Dimensions[0].InferNameFromSeriesMeta)
				assert.Equal(t, program.AlgorithmAbsolute, charts[0].Meta.Algorithm)
			},
		},
		"fails runtime inference for omitted naming on non-inferable selector": {
			spec: charttpl.Spec{
				Version: charttpl.VersionV1,
				Groups: []charttpl.Group{
					{
						Family:  "CPU",
						Metrics: []string{"windows_cpu_usage"},
						Charts: []charttpl.Chart{
							{
								Title:   "CPU usage",
								Context: "cpu_usage",
								Units:   "%",
								Dimensions: []charttpl.Dimension{
									{
										Selector: "windows_cpu_usage",
									},
								},
							},
						},
					},
				},
			},
			wantErr: true,
			errLike: "name inference requires inferable selector",
		},
		"compiles literal id with instances and label exclusion metadata": {
			spec: charttpl.Spec{
				Version: charttpl.VersionV1,
				Groups: []charttpl.Group{
					{
						Family:  "Disk",
						Metrics: []string{"ceph_osd_space_bytes"},
						Charts: []charttpl.Chart{
							{
								ID:            "osd_space_usage",
								Title:         "OSD space usage",
								Context:       "osd_space_usage",
								Units:         "bytes",
								LabelPromoted: []string{"cluster", "cluster", "host"},
								Instances: &charttpl.Instances{
									ByLabels: []string{"osd_uuid"},
								},
								Dimensions: []charttpl.Dimension{
									{
										Selector:      `ceph_osd_space_bytes{state="used"}`,
										NameFromLabel: "state",
									},
								},
							},
						},
					},
				},
			},
			assert: func(t *testing.T, p *program.Program) {
				t.Helper()
				charts := p.Charts()
				require.Len(t, charts, 1)
				require.Len(t, charts[0].Identity.InstanceByLabels, 1)
				assert.Equal(t, "osd_uuid", charts[0].Identity.InstanceByLabels[0].Key)
				assert.Equal(t, "Disk", charts[0].Meta.Family)
				assert.Equal(t, "osd_space_usage", charts[0].Meta.Context)

				assert.Equal(t, program.PromotionModeExplicitIntersection, charts[0].Labels.Mode)
				assert.Equal(t, []string{"cluster", "host"}, charts[0].Labels.PromoteKeys)
				assert.Equal(t, []string{"state"}, charts[0].Labels.Exclusions.SelectorConstrainedKeys)
				assert.Equal(t, []string{"state"}, charts[0].Labels.Exclusions.DimensionKeyLabels)
			},
		},
		"fails on mixed inferred metric kinds without explicit algorithm": {
			spec: charttpl.Spec{
				Version: charttpl.VersionV1,
				Groups: []charttpl.Group{
					{
						Family: "Mixed",
						Metrics: []string{
							"requests_total",
							"queue_depth",
						},
						Charts: []charttpl.Chart{
							{
								Title:   "Mixed chart",
								Context: "mixed",
								Units:   "value",
								Dimensions: []charttpl.Dimension{
									{Selector: "requests_total", Name: "req"},
									{Selector: "queue_depth", Name: "depth"},
								},
							},
						},
					},
				},
			},
			wantErr: true,
			errLike: "algorithm inference is ambiguous",
		},
		"treats braces as literal characters in chart id": {
			spec: charttpl.Spec{
				Version: charttpl.VersionV1,
				Groups: []charttpl.Group{
					{
						Family:  "Disk",
						Metrics: []string{"ceph_osd_space_bytes"},
						Charts: []charttpl.Chart{
							{
								ID:      "osd_{osd_uuid}_space_usage",
								Title:   "OSD space usage",
								Context: "osd_space_usage",
								Units:   "bytes",
								Dimensions: []charttpl.Dimension{
									{Selector: "ceph_osd_space_bytes", Name: "used"},
								},
							},
						},
					},
				},
			},
			assert: func(t *testing.T, p *program.Program) {
				t.Helper()
				charts := p.Charts()
				require.Len(t, charts, 1)
				assert.Equal(t, "osd_{osd_uuid}_space_usage", charts[0].Identity.IDTemplate.Raw)
			},
		},
		"treats braces as literal characters in dimension name": {
			spec: charttpl.Spec{
				Version: charttpl.VersionV1,
				Groups: []charttpl.Group{
					{
						Family:  "Disk",
						Metrics: []string{"ceph_osd_space_bytes"},
						Charts: []charttpl.Chart{
							{
								ID:      "osd_space_usage",
								Title:   "OSD space usage",
								Context: "osd_space_usage",
								Units:   "bytes",
								Dimensions: []charttpl.Dimension{
									{Selector: "ceph_osd_space_bytes", Name: "{state}"},
								},
							},
						},
					},
				},
			},
			assert: func(t *testing.T, p *program.Program) {
				t.Helper()
				charts := p.Charts()
				require.Len(t, charts, 1)
				require.Len(t, charts[0].Dimensions, 1)
				assert.Equal(t, "{state}", charts[0].Dimensions[0].NameTemplate.Raw)
			},
		},
		"fails on invalid selector syntax via compiler parse": {
			spec: charttpl.Spec{
				Version: charttpl.VersionV1,
				Groups: []charttpl.Group{
					{
						Family:  "Service",
						Metrics: []string{"svc_requests_total"},
						Charts: []charttpl.Chart{
							{
								Title:   "Requests",
								Context: "requests",
								Units:   "requests/s",
								Dimensions: []charttpl.Dimension{
									{Selector: `svc_requests_total{method="GET",}`},
								},
							},
						},
					},
				},
			},
			wantErr: true,
			errLike: "selector:",
		},
		"fails when selector metric is not visible in compile path": {
			spec: charttpl.Spec{
				Version: charttpl.VersionV1,
				Groups: []charttpl.Group{
					{
						Family:  "Service",
						Metrics: []string{"svc_requests_total"},
						Charts: []charttpl.Chart{
							{
								Title:   "Errors",
								Context: "errors",
								Units:   "errors/s",
								Dimensions: []charttpl.Dimension{
									{Selector: "svc_errors_total", Name: "total"},
								},
							},
						},
					},
				},
			},
			wantErr: true,
			errLike: "not visible in current group scope",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			compiled, err := Compile(&tc.spec, tc.rev)
			if tc.wantErr {
				require.Error(t, err)
				if tc.errLike != "" {
					assert.ErrorContains(t, err, tc.errLike)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, compiled)
			if tc.assert != nil {
				tc.assert(t, compiled)
			}
		})
	}
}
