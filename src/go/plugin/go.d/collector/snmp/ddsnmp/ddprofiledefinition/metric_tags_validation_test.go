// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ddprofiledefinition

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_validateEnrichMetricTag(t *testing.T) {
	tests := map[string]struct {
		tag                MetricTagConfig
		wantError          bool
		wantTag            *MetricTagConfig
		wantErrContains    string
		wantErrNotContains string
	}{
		"mapping error is readable": {
			tag: MetricTagConfig{
				Mapping: NewExactMapping(map[string]string{"1": "up"}),
			},
			wantError:          true,
			wantErrContains:    "map[1:up]",
			wantErrNotContains: "%!s",
		},
		"Move OID to Symbol": {
			wantError: false,
			tag: MetricTagConfig{
				OID: "1.2.3.4",
				Symbol: SymbolConfigCompat{
					Name: "mySymbol",
				},
			},
			wantTag: &MetricTagConfig{
				Symbol: SymbolConfigCompat{
					OID:  "1.2.3.4",
					Name: "mySymbol",
				},
			},
		},
		"Metric tag OID and symbol.OID cannot be both declared": {
			wantError: true,
			tag: MetricTagConfig{
				OID: "1.2.3.4",
				Symbol: SymbolConfigCompat{
					OID:  "1.2.3.5",
					Name: "mySymbol",
				},
			},
		},
		"metric tag symbol and column cannot be both declared 2": {
			wantError: true,
			tag: MetricTagConfig{
				Symbol: SymbolConfigCompat{
					OID:  "1.2.3.5",
					Name: "mySymbol",
				},
				Column: SymbolConfig{
					OID:  "1.2.3.5",
					Name: "mySymbol",
				},
			},
		},
		"Missing OID": {
			wantError: true,
			tag: MetricTagConfig{
				Symbol: SymbolConfigCompat{
					Name: "mySymbol",
				},
			},
		},
		"raw index transform with drop_right": {
			wantError: false,
			tag: MetricTagConfig{
				Tag: "remote_addr",
				IndexTransform: []MetricIndexTransform{
					{
						Start:     2,
						DropRight: 2,
					},
				},
			},
		},
		"raw index transform to tail": {
			wantError: false,
			tag: MetricTagConfig{
				Tag: "fdb_mac",
				IndexTransform: []MetricIndexTransform{
					{
						Start: 1,
					},
				},
			},
		},
		"raw index transform cannot combine end and drop_right": {
			wantError: true,
			tag: MetricTagConfig{
				Tag: "remote_addr",
				IndexTransform: []MetricIndexTransform{
					{
						Start:     2,
						End:       6,
						DropRight: 2,
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := validateEnrichMetricTag("", &tc.tag, false)
			if tc.wantError {
				require.Error(t, err)
				if tc.wantErrContains != "" {
					assert.ErrorContains(t, err, tc.wantErrContains)
				}
				if tc.wantErrNotContains != "" {
					assert.NotContains(t, err.Error(), tc.wantErrNotContains)
				}
			} else {
				require.NoError(t, err)
			}
			if tc.wantTag != nil {
				assert.Equal(t, *tc.wantTag, tc.tag)
			}
		})
	}
}
