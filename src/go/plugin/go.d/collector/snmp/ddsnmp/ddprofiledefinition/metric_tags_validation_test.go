// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ddprofiledefinition

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_validateEnrichMetricTag_MappingErrorUsesReadableFormat(t *testing.T) {
	tag := MetricTagConfig{
		Mapping: NewExactMapping(map[string]string{
			"1": "up",
		}),
	}

	err := validateEnrichMetricTag(&tag)

	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "map[1:up]")
		assert.NotContains(t, err.Error(), "%!s")
	}
}

func Test_validateEnrichMetricTag(t *testing.T) {
	tests := map[string]struct {
		metrics     []MetricTagConfig
		wantError   bool
		wantMetrics []MetricTagConfig
	}{
		"Move OID to Symbol": {
			wantError: false,
			metrics: []MetricTagConfig{
				{
					OID: "1.2.3.4",
					Symbol: SymbolConfigCompat{
						Name: "mySymbol",
					},
				},
			},
			wantMetrics: []MetricTagConfig{
				{
					Symbol: SymbolConfigCompat{
						OID:  "1.2.3.4",
						Name: "mySymbol",
					},
				},
			},
		},
		"Metric tag OID and symbol.OID cannot be both declared": {
			wantError: true,
			metrics: []MetricTagConfig{
				{
					OID: "1.2.3.4",
					Symbol: SymbolConfigCompat{
						OID:  "1.2.3.5",
						Name: "mySymbol",
					},
				},
			},
		},
		"metric tag symbol and column cannot be both declared 2": {
			wantError: true,
			metrics: []MetricTagConfig{
				{
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
		},
		"Missing OID": {
			wantError: true,
			metrics: []MetricTagConfig{
				{
					Symbol: SymbolConfigCompat{
						Name: "mySymbol",
					},
				},
			},
		},
		"raw index transform with drop_right": {
			wantError: false,
			metrics: []MetricTagConfig{
				{
					Tag: "remote_addr",
					IndexTransform: []MetricIndexTransform{
						{
							Start:     2,
							DropRight: 2,
						},
					},
				},
			},
		},
		"raw index transform to tail": {
			wantError: false,
			metrics: []MetricTagConfig{
				{
					Tag: "fdb_mac",
					IndexTransform: []MetricIndexTransform{
						{
							Start: 1,
						},
					},
				},
			},
		},
		"raw index transform cannot combine end and drop_right": {
			wantError: true,
			metrics: []MetricTagConfig{
				{
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
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if tc.wantError {
				var errs []error
				for i := range tc.metrics {
					errs = append(errs, validateEnrichMetricTag(&tc.metrics[i]))
				}
				assert.Error(t, errors.Join(errs...))
			} else {
				for i := range tc.metrics {
					assert.NoError(t, validateEnrichMetricTag(&tc.metrics[i]))
				}
			}
			if tc.wantMetrics != nil {
				assert.Equal(t, tc.wantMetrics, tc.metrics)
			}
		})
	}
}
