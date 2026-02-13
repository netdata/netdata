// SPDX-License-Identifier: GPL-3.0-or-later

package charttpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
