// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ddprofiledefinition

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_validateEnrichMetrics(t *testing.T) {
	tests := map[string]struct {
		metrics     []MetricsConfig
		wantError   bool
		wantMetrics []MetricsConfig
	}{
		"either table symbol or scalar symbol must be provided": {
			wantError: true,
			metrics: []MetricsConfig{
				{},
			},
			wantMetrics: []MetricsConfig{
				{},
			},
		},
		"table column symbols and scalar symbol cannot be both provided": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Table: SymbolConfig{
						OID: "1.2",
					},
					Symbol: SymbolConfig{
						OID:  "1.2",
						Name: "abc",
					},
					Symbols: []SymbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: []MetricTagConfig{
						MetricTagConfig{},
					},
				},
			},
		},
		"multiple errors": {
			wantError: true,
			metrics: []MetricsConfig{
				{},
				{
					Table: SymbolConfig{
						OID: "1.2",
					},
					Symbol: SymbolConfig{
						OID:  "1.2",
						Name: "abc",
					},
					Symbols: []SymbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: []MetricTagConfig{
						MetricTagConfig{},
					},
				},
			},
		},
		"missing symbol name": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						OID: "1.2.3",
					},
				},
			},
		},
		"table column symbol name missing": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Table: SymbolConfig{
						OID: "1.2",
					},
					Symbols: []SymbolConfig{
						{
							OID: "1.2",
						},
						{
							Name: "abc",
						},
					},
					MetricTags: []MetricTagConfig{
						MetricTagConfig{},
					},
				},
			},
		},
		"table external metric column tag symbol error": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Table: SymbolConfig{
						OID: "1.2",
					},
					Symbols: []SymbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: []MetricTagConfig{
						MetricTagConfig{
							Symbol: SymbolConfigCompat{
								OID: "1.2.3",
							},
						},
						MetricTagConfig{
							Symbol: SymbolConfigCompat{
								Name: "abc",
							},
						},
					},
				},
			},
		},
		"missing MetricTags": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Table: SymbolConfig{
						OID: "1.2",
					},
					Symbols: []SymbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: []MetricTagConfig{},
				},
			},
		},
		"missing match tags": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Table: SymbolConfig{
						OID: "1.2",
					},
					Symbols: []SymbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: []MetricTagConfig{
						MetricTagConfig{
							Symbol: SymbolConfigCompat{
								OID:  "1.2.3",
								Name: "abc",
							},
							Match: "([a-z])",
						},
					},
				},
			},
		},
		"match cannot compile regex": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Table: SymbolConfig{
						OID: "1.2",
					},
					Symbols: []SymbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: []MetricTagConfig{
						MetricTagConfig{
							Symbol: SymbolConfigCompat{
								OID:  "1.2.3",
								Name: "abc",
							},
							Match: "([a-z)",
							Tags: map[string]string{
								"foo": "bar",
							},
						},
					},
				},
			},
		},
		"index transform end precedes start": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Table: SymbolConfig{
						OID: "1.2",
					},
					Symbols: []SymbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: []MetricTagConfig{
						MetricTagConfig{
							Symbol: SymbolConfigCompat{
								OID:  "1.2.3",
								Name: "abc",
							},
							Tag: "hello",
							IndexTransform: []MetricIndexTransform{
								{
									Start: 2,
									End:   1,
								},
							},
						},
					},
				},
			},
		},
		"compiling extract_value": {
			wantError: false,
			metrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						OID:          "1.2.3",
						Name:         "myMetric",
						ExtractValue: `(\d+)C`,
					},
				},
				{
					Table: SymbolConfig{
						OID: "1.2",
					},
					Symbols: []SymbolConfig{
						{
							OID:          "1.2",
							Name:         "hey",
							ExtractValue: `(\d+)C`,
						},
					},
					MetricTags: []MetricTagConfig{
						MetricTagConfig{
							Symbol: SymbolConfigCompat{
								OID:          "1.2.3",
								Name:         "abc",
								ExtractValue: `(\d+)C`,
							},
							Tag: "hello",
						},
					},
				},
			},
			wantMetrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						OID:                  "1.2.3",
						Name:                 "myMetric",
						ExtractValue:         `(\d+)C`,
						ExtractValueCompiled: regexp.MustCompile(`(\d+)C`),
					},
				},
				{
					Table: SymbolConfig{
						OID: "1.2",
					},
					Symbols: []SymbolConfig{
						{
							OID:                  "1.2",
							Name:                 "hey",
							ExtractValue:         `(\d+)C`,
							ExtractValueCompiled: regexp.MustCompile(`(\d+)C`),
						},
					},
					MetricTags: []MetricTagConfig{
						MetricTagConfig{
							Symbol: SymbolConfigCompat{
								OID:                  "1.2.3",
								Name:                 "abc",
								ExtractValue:         `(\d+)C`,
								ExtractValueCompiled: regexp.MustCompile(`(\d+)C`),
							},
							Tag: "hello",
						},
					},
				},
			},
		},
		"error compiling extract_value": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						OID:          "1.2.3",
						Name:         "myMetric",
						ExtractValue: "[{",
					},
				},
			},
		},
		"metric_type usage in column symbol": {
			wantError: false,
			metrics: []MetricsConfig{
				{
					Table: SymbolConfig{
						OID: "1.2",
					},
					Symbols: []SymbolConfig{
						{
							Name:       "abc",
							OID:        "1.2.3",
							MetricType: "counter",
						},
					},
					MetricTags: []MetricTagConfig{
						MetricTagConfig{
							Symbol: SymbolConfigCompat{
								Name: "abc",
								OID:  "1.2.3",
							},
							Tag: "hello",
						},
					},
				},
			},
		},
		"metric_type usage in scalar symbol": {
			wantError: false,
			metrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						Name:       "abc",
						OID:        "1.2.3",
						MetricType: "counter",
					},
				},
			},
		},
		"ERROR metric_type usage in metric_tags": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Table: SymbolConfig{
						OID: "1.2",
					},
					Symbols: []SymbolConfig{
						{
							Name: "abc",
							OID:  "1.2.3",
						},
					},
					MetricTags: []MetricTagConfig{
						MetricTagConfig{
							Symbol: SymbolConfigCompat{
								Name:       "abc",
								OID:        "1.2.3",
								MetricType: "counter",
							},
							Tag: "hello",
						},
					},
				},
			},
		},
		"mapping used with symbol.name and no explicit tag": {
			wantError: false,
			metrics: []MetricsConfig{
				{
					Table: SymbolConfig{
						OID: "1.2",
					},
					Symbols: []SymbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: []MetricTagConfig{
						MetricTagConfig{
							Symbol: SymbolConfigCompat{
								OID:  "1.2",
								Name: "abc",
							},
							Mapping: NewExactMapping(map[string]string{
								"1": "abc",
								"2": "def",
							}),
						},
					},
				},
			},
		},
		"bitmask mapping usage in scalar symbol": {
			wantError: false,
			metrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						OID:  "1.2.3",
						Name: "processorStatus",
						Mapping: MappingConfig{
							Mode: MappingModeBitmask,
							Items: map[string]string{
								"1":   "internalError",
								"128": "processorPresent",
							},
						},
					},
				},
			},
		},
		"ERROR bitmask mapping usage in metric_tags": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Table: SymbolConfig{
						OID: "1.2",
					},
					Symbols: []SymbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: []MetricTagConfig{
						MetricTagConfig{
							Tag: "state",
							Symbol: SymbolConfigCompat{
								OID:  "1.2",
								Name: "abc",
							},
							Mapping: MappingConfig{
								Mode: MappingModeBitmask,
								Items: map[string]string{
									"1": "internalError",
								},
							},
						},
					},
				},
			},
		},
		"scalar metric_tags with scalar OID symbol are supported": {
			wantError: false,
			metrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						OID:  "1.2.3",
						Name: "myMetric",
					},
					MetricTags: []MetricTagConfig{
						{
							OID: "1.2.4",
							Symbol: SymbolConfigCompat{
								Name: "stateSource",
							},
							Tag: "state",
						},
					},
				},
			},
			wantMetrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						OID:  "1.2.3",
						Name: "myMetric",
					},
					MetricTags: []MetricTagConfig{
						{
							Tag: "state",
							Symbol: SymbolConfigCompat{
								OID:  "1.2.4",
								Name: "stateSource",
							},
						},
					},
				},
			},
		},
		"scalar metric_tags do not support index lookups": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						OID:  "1.2.3",
						Name: "myMetric",
					},
					MetricTags: []MetricTagConfig{
						{
							Tag:   "idx",
							Index: 1,
						},
					},
				},
			},
		},
		"scalar metric_tags do not support table lookups": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						OID:  "1.2.3",
						Name: "myMetric",
					},
					MetricTags: []MetricTagConfig{
						{
							Tag:   "peer",
							Table: "ifTable",
							Symbol: SymbolConfigCompat{
								OID:  "1.2.4",
								Name: "ifDescr",
							},
						},
					},
				},
			},
		},
		"scalar metric_tags do not support index transforms": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						OID:  "1.2.3",
						Name: "myMetric",
					},
					MetricTags: []MetricTagConfig{
						{
							Tag: "peer",
							Symbol: SymbolConfigCompat{
								OID:  "1.2.4",
								Name: "peerState",
							},
							IndexTransform: []MetricIndexTransform{
								{
									Start: 1,
									End:   1,
								},
							},
						},
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if tc.wantError {
				assert.Error(t, validateEnrichMetrics(tc.metrics))
			} else {
				assert.NoError(t, validateEnrichMetrics(tc.metrics))
			}
			if tc.wantMetrics != nil {
				assert.Equal(t, tc.wantMetrics, tc.metrics)
			}
		})
	}
}
