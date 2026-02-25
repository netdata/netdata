// SPDX-License-Identifier: GPL-3.0-or-later

package charttpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
)

func TestSpecValidateScenarios(t *testing.T) {
	tests := map[string]struct {
		spec    Spec
		wantErr bool
		errLike string
	}{
		"valid nested groups with inherited metrics union": {
			spec: Spec{
				Version: VersionV1,
				Groups: []Group{
					{
						Family:  "Database",
						Metrics: []string{"mysql_queries_total"},
						Groups: []Group{
							{
								Family: "Throughput",
								Charts: []Chart{
									{
										Title:     "Queries",
										Context:   "queries_total",
										Units:     "queries/s",
										Algorithm: "incremental",
										Dimensions: []Dimension{
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
		},
		"requires groups in groups-only mode": {
			spec: Spec{
				Version: VersionV1,
			},
			wantErr: true,
			errLike: "groups[] is required",
		},
		"fails when selector metric not visible in scope": {
			spec: Spec{
				Version: VersionV1,
				Groups: []Group{
					{
						Family: "Database",
						Charts: []Chart{
							{
								Title:     "Queries",
								Context:   "queries_total",
								Units:     "queries/s",
								Algorithm: "incremental",
								Dimensions: []Dimension{
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
			wantErr: true,
			errLike: "not visible in current group scope",
		},
		"fails on duplicate by_labels tokens": {
			spec: Spec{
				Version: VersionV1,
				Groups: []Group{
					{
						Family:  "Database",
						Metrics: []string{"mysql_queries_total"},
						Charts: []Chart{
							{
								Title:     "Queries",
								Context:   "queries_total",
								Units:     "queries/s",
								Algorithm: "incremental",
								Instances: &Instances{
									ByLabels: []string{"*", "*"},
								},
								Dimensions: []Dimension{
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
			wantErr: true,
			errLike: "duplicate token",
		},
		"fails on empty metric name": {
			spec: Spec{
				Version: VersionV1,
				Groups: []Group{
					{
						Family:  "Database",
						Metrics: []string{"mysql_queries_total", " "},
						Charts: []Chart{
							{
								Title:   "Queries",
								Context: "queries_total",
								Units:   "queries/s",
								Dimensions: []Dimension{
									{Selector: "mysql_queries_total", Name: "total"},
								},
							},
						},
					},
				},
			},
			wantErr: true,
			errLike: "metric name must not be empty",
		},
		"fails on duplicate metric names in group": {
			spec: Spec{
				Version: VersionV1,
				Groups: []Group{
					{
						Family:  "Database",
						Metrics: []string{"mysql_queries_total", "mysql_queries_total"},
						Charts: []Chart{
							{
								Title:   "Queries",
								Context: "queries_total",
								Units:   "queries/s",
								Dimensions: []Dimension{
									{Selector: "mysql_queries_total", Name: "total"},
								},
							},
						},
					},
				},
			},
			wantErr: true,
			errLike: "duplicate metric",
		},
		"fails on empty group family": {
			spec: Spec{
				Version: VersionV1,
				Groups: []Group{
					{
						Family:  " ",
						Metrics: []string{"mysql_queries_total"},
						Charts: []Chart{
							{
								Title:   "Queries",
								Context: "queries_total",
								Units:   "queries/s",
								Dimensions: []Dimension{
									{Selector: "mysql_queries_total", Name: "total"},
								},
							},
						},
					},
				},
			},
			wantErr: true,
			errLike: "groups[0].family",
		},
		"fails on empty chart title": {
			spec: Spec{
				Version: VersionV1,
				Groups: []Group{
					{
						Family:  "Database",
						Metrics: []string{"mysql_queries_total"},
						Charts: []Chart{
							{
								Title:   " ",
								Context: "queries_total",
								Units:   "queries/s",
								Dimensions: []Dimension{
									{Selector: "mysql_queries_total", Name: "total"},
								},
							},
						},
					},
				},
			},
			wantErr: true,
			errLike: ".title",
		},
		"fails on empty chart context": {
			spec: Spec{
				Version: VersionV1,
				Groups: []Group{
					{
						Family:  "Database",
						Metrics: []string{"mysql_queries_total"},
						Charts: []Chart{
							{
								Title:   "Queries",
								Context: " ",
								Units:   "queries/s",
								Dimensions: []Dimension{
									{Selector: "mysql_queries_total", Name: "total"},
								},
							},
						},
					},
				},
			},
			wantErr: true,
			errLike: ".context",
		},
		"fails on empty chart units": {
			spec: Spec{
				Version: VersionV1,
				Groups: []Group{
					{
						Family:  "Database",
						Metrics: []string{"mysql_queries_total"},
						Charts: []Chart{
							{
								Title:   "Queries",
								Context: "queries_total",
								Units:   " ",
								Dimensions: []Dimension{
									{Selector: "mysql_queries_total", Name: "total"},
								},
							},
						},
					},
				},
			},
			wantErr: true,
			errLike: ".units",
		},
		"fails on invalid algorithm": {
			spec: Spec{
				Version: VersionV1,
				Groups: []Group{
					{
						Family:  "Database",
						Metrics: []string{"mysql_queries_total"},
						Charts: []Chart{
							{
								Title:     "Queries",
								Context:   "queries_total",
								Units:     "queries/s",
								Algorithm: "delta",
								Dimensions: []Dimension{
									{Selector: "mysql_queries_total", Name: "total"},
								},
							},
						},
					},
				},
			},
			wantErr: true,
			errLike: ".algorithm",
		},
		"fails on invalid chart type": {
			spec: Spec{
				Version: VersionV1,
				Groups: []Group{
					{
						Family:  "Database",
						Metrics: []string{"mysql_queries_total"},
						Charts: []Chart{
							{
								Title:   "Queries",
								Context: "queries_total",
								Units:   "queries/s",
								Type:    "bars",
								Dimensions: []Dimension{
									{Selector: "mysql_queries_total", Name: "total"},
								},
							},
						},
					},
				},
			},
			wantErr: true,
			errLike: ".type",
		},
		"fails on negative lifecycle max_instances": {
			spec: Spec{
				Version: VersionV1,
				Groups: []Group{
					{
						Family:  "Database",
						Metrics: []string{"mysql_queries_total"},
						Charts: []Chart{
							{
								Title:   "Queries",
								Context: "queries_total",
								Units:   "queries/s",
								Lifecycle: &Lifecycle{
									MaxInstances: -1,
								},
								Dimensions: []Dimension{
									{Selector: "mysql_queries_total", Name: "total"},
								},
							},
						},
					},
				},
			},
			wantErr: true,
			errLike: "lifecycle.max_instances",
		},
		"fails on negative lifecycle expire_after_cycles": {
			spec: Spec{
				Version: VersionV1,
				Groups: []Group{
					{
						Family:  "Database",
						Metrics: []string{"mysql_queries_total"},
						Charts: []Chart{
							{
								Title:   "Queries",
								Context: "queries_total",
								Units:   "queries/s",
								Lifecycle: &Lifecycle{
									ExpireAfterCycles: -1,
								},
								Dimensions: []Dimension{
									{Selector: "mysql_queries_total", Name: "total"},
								},
							},
						},
					},
				},
			},
			wantErr: true,
			errLike: "lifecycle.expire_after_cycles",
		},
		"fails on negative lifecycle dimensions max_dims": {
			spec: Spec{
				Version: VersionV1,
				Groups: []Group{
					{
						Family:  "Database",
						Metrics: []string{"mysql_queries_total"},
						Charts: []Chart{
							{
								Title:   "Queries",
								Context: "queries_total",
								Units:   "queries/s",
								Lifecycle: &Lifecycle{
									Dimensions: &DimensionLifecycle{
										MaxDims: -1,
									},
								},
								Dimensions: []Dimension{
									{Selector: "mysql_queries_total", Name: "total"},
								},
							},
						},
					},
				},
			},
			wantErr: true,
			errLike: "lifecycle.dimensions.max_dims",
		},
		"fails on negative lifecycle dimensions expire_after_cycles": {
			spec: Spec{
				Version: VersionV1,
				Groups: []Group{
					{
						Family:  "Database",
						Metrics: []string{"mysql_queries_total"},
						Charts: []Chart{
							{
								Title:   "Queries",
								Context: "queries_total",
								Units:   "queries/s",
								Lifecycle: &Lifecycle{
									Dimensions: &DimensionLifecycle{
										ExpireAfterCycles: -1,
									},
								},
								Dimensions: []Dimension{
									{Selector: "mysql_queries_total", Name: "total"},
								},
							},
						},
					},
				},
			},
			wantErr: true,
			errLike: "lifecycle.dimensions.expire_after_cycles",
		},
		"fails on empty dimensions": {
			spec: Spec{
				Version: VersionV1,
				Groups: []Group{
					{
						Family:  "Database",
						Metrics: []string{"mysql_queries_total"},
						Charts: []Chart{
							{
								Title:      "Queries",
								Context:    "queries_total",
								Units:      "queries/s",
								Dimensions: nil,
							},
						},
					},
				},
			},
			wantErr: true,
			errLike: ".dimensions",
		},
		"fails on empty selector": {
			spec: Spec{
				Version: VersionV1,
				Groups: []Group{
					{
						Family:  "Database",
						Metrics: []string{"mysql_queries_total"},
						Charts: []Chart{
							{
								Title:   "Queries",
								Context: "queries_total",
								Units:   "queries/s",
								Dimensions: []Dimension{
									{Selector: " ", Name: "total"},
								},
							},
						},
					},
				},
			},
			wantErr: true,
			errLike: ".selector",
		},
		"fails on selector without explicit metric": {
			spec: Spec{
				Version: VersionV1,
				Groups: []Group{
					{
						Family:  "Database",
						Metrics: []string{"mysql_queries_total"},
						Charts: []Chart{
							{
								Title:   "Queries",
								Context: "queries_total",
								Units:   "queries/s",
								Dimensions: []Dimension{
									{Selector: `{role="primary"}`, Name: "total"},
								},
							},
						},
					},
				},
			},
			wantErr: true,
			errLike: "selector must include explicit metric name",
		},
		"fails on name and name_from_label together": {
			spec: Spec{
				Version: VersionV1,
				Groups: []Group{
					{
						Family:  "Database",
						Metrics: []string{"mysql_queries_total"},
						Charts: []Chart{
							{
								Title:   "Queries",
								Context: "queries_total",
								Units:   "queries/s",
								Dimensions: []Dimension{
									{Selector: "mysql_queries_total", Name: "total", NameFromLabel: "mode"},
								},
							},
						},
					},
				},
			},
			wantErr: true,
			errLike: "use either name or name_from_label",
		},
		"fails on duplicate static dimension names": {
			spec: Spec{
				Version: VersionV1,
				Groups: []Group{
					{
						Family:  "Database",
						Metrics: []string{"mysql_queries_total", "mysql_questions_total"},
						Charts: []Chart{
							{
								Title:   "Queries",
								Context: "queries_total",
								Units:   "queries/s",
								Dimensions: []Dimension{
									{Selector: "mysql_queries_total", Name: "total"},
									{Selector: "mysql_questions_total", Name: "total"},
								},
							},
						},
					},
				},
			},
			wantErr: true,
			errLike: "duplicate dimension name",
		},
		"fails on instances empty token": {
			spec: Spec{
				Version: VersionV1,
				Groups: []Group{
					{
						Family:  "Database",
						Metrics: []string{"mysql_queries_total"},
						Charts: []Chart{
							{
								Title:   "Queries",
								Context: "queries_total",
								Units:   "queries/s",
								Instances: &Instances{
									ByLabels: []string{"* ", " "},
								},
								Dimensions: []Dimension{
									{Selector: "mysql_queries_total", Name: "total"},
								},
							},
						},
					},
				},
			},
			wantErr: true,
			errLike: "must not be empty",
		},
		"fails on instances bare exclude token": {
			spec: Spec{
				Version: VersionV1,
				Groups: []Group{
					{
						Family:  "Database",
						Metrics: []string{"mysql_queries_total"},
						Charts: []Chart{
							{
								Title:   "Queries",
								Context: "queries_total",
								Units:   "queries/s",
								Instances: &Instances{
									ByLabels: []string{"!"},
								},
								Dimensions: []Dimension{
									{Selector: "mysql_queries_total", Name: "total"},
								},
							},
						},
					},
				},
			},
			wantErr: true,
			errLike: "exclude token must include label key",
		},
		"fails on instances with empty by_labels list": {
			spec: Spec{
				Version: VersionV1,
				Groups: []Group{
					{
						Family:  "Database",
						Metrics: []string{"mysql_queries_total"},
						Charts: []Chart{
							{
								Title:   "Queries",
								Context: "queries_total",
								Units:   "queries/s",
								Instances: &Instances{
									ByLabels: []string{},
								},
								Dimensions: []Dimension{
									{Selector: "mysql_queries_total", Name: "total"},
								},
							},
						},
					},
				},
			},
			wantErr: true,
			errLike: "instances.by_labels",
		},
		"fails on whitespace-only dimension name": {
			spec: Spec{
				Version: VersionV1,
				Groups: []Group{
					{
						Family:  "Database",
						Metrics: []string{"mysql_queries_total"},
						Charts: []Chart{
							{
								Title:   "Queries",
								Context: "queries_total",
								Units:   "queries/s",
								Dimensions: []Dimension{
									{Selector: "mysql_queries_total", Name: " "},
								},
							},
						},
					},
				},
			},
			wantErr: true,
			errLike: "name",
		},
		"fails on whitespace-only name_from_label": {
			spec: Spec{
				Version: VersionV1,
				Groups: []Group{
					{
						Family:  "Database",
						Metrics: []string{"mysql_queries_total"},
						Charts: []Chart{
							{
								Title:   "Queries",
								Context: "queries_total",
								Units:   "queries/s",
								Dimensions: []Dimension{
									{Selector: "mysql_queries_total", NameFromLabel: " "},
								},
							},
						},
					},
				},
			},
			wantErr: true,
			errLike: "name_from_label",
		},
		"fails on empty engine selector entry": {
			spec: Spec{
				Version: VersionV1,
				Engine: &Engine{
					Selector: &metrixselector.Expr{
						Allow: []string{"  "},
					},
				},
				Groups: []Group{
					{
						Family:  "Database",
						Metrics: []string{"mysql_queries_total"},
						Charts: []Chart{
							{
								Title:   "Queries",
								Context: "queries_total",
								Units:   "queries/s",
								Dimensions: []Dimension{
									{Selector: "mysql_queries_total", Name: "total"},
								},
							},
						},
					},
				},
			},
			wantErr: true,
			errLike: "engine.selector.allow[0]",
		},
		"fails on invalid engine autogen max_type_id_len": {
			spec: Spec{
				Version: VersionV1,
				Engine: &Engine{
					Autogen: &EngineAutogen{
						Enabled:      true,
						MaxTypeIDLen: 3,
					},
				},
				Groups: []Group{
					{
						Family:  "Database",
						Metrics: []string{"mysql_queries_total"},
						Charts: []Chart{
							{
								Title:   "Queries",
								Context: "queries_total",
								Units:   "queries/s",
								Dimensions: []Dimension{
									{Selector: "mysql_queries_total", Name: "total"},
								},
							},
						},
					},
				},
			},
			wantErr: true,
			errLike: "engine.autogen.max_type_id_len",
		},
		"fails on empty engine deny selector entry": {
			spec: Spec{
				Version: VersionV1,
				Engine: &Engine{
					Selector: &metrixselector.Expr{
						Deny: []string{" "},
					},
				},
				Groups: []Group{
					{
						Family:  "Database",
						Metrics: []string{"mysql_queries_total"},
						Charts: []Chart{
							{
								Title:   "Queries",
								Context: "queries_total",
								Units:   "queries/s",
								Dimensions: []Dimension{
									{Selector: "mysql_queries_total", Name: "total"},
								},
							},
						},
					},
				},
			},
			wantErr: true,
			errLike: "engine.selector.deny[0]",
		},
		"fails on negative engine autogen max_type_id_len": {
			spec: Spec{
				Version: VersionV1,
				Engine: &Engine{
					Autogen: &EngineAutogen{
						MaxTypeIDLen: -1,
					},
				},
				Groups: []Group{
					{
						Family:  "Database",
						Metrics: []string{"mysql_queries_total"},
						Charts: []Chart{
							{
								Title:   "Queries",
								Context: "queries_total",
								Units:   "queries/s",
								Dimensions: []Dimension{
									{Selector: "mysql_queries_total", Name: "total"},
								},
							},
						},
					},
				},
			},
			wantErr: true,
			errLike: "engine.autogen.max_type_id_len",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := tc.spec.Validate()
			if tc.wantErr {
				require.Error(t, err)
				if tc.errLike != "" {
					assert.ErrorContains(t, err, tc.errLike)
				}
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestSpecValidateNilAndVersion(t *testing.T) {
	var spec *Spec
	err := spec.Validate()
	require.Error(t, err)
	assert.ErrorContains(t, err, "nil spec")

	spec = &Spec{
		Version: "v2",
		Groups: []Group{
			{
				Family:  "Database",
				Metrics: []string{"mysql_queries_total"},
				Charts: []Chart{
					{
						Title:   "Queries",
						Context: "queries_total",
						Units:   "queries/s",
						Dimensions: []Dimension{
							{Selector: "mysql_queries_total", Name: "total"},
						},
					},
				},
			},
		},
	}
	err = spec.Validate()
	require.Error(t, err)
	assert.ErrorContains(t, err, "expected \"v1\"")
}
