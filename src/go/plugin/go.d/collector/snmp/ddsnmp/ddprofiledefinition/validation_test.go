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
					MetricTags: MetricTagConfigList{
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
					MetricTags: MetricTagConfigList{
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
					Symbols: []SymbolConfig{
						{
							OID: "1.2",
						},
						{
							Name: "abc",
						},
					},
					MetricTags: MetricTagConfigList{
						MetricTagConfig{},
					},
				},
			},
		},
		"table external metric column tag symbol error": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbols: []SymbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: MetricTagConfigList{
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
					Symbols: []SymbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: MetricTagConfigList{},
				},
			},
		},
		"table external metric column tag MIB error": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbols: []SymbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: MetricTagConfigList{
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
		"missing match tags": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbols: []SymbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: MetricTagConfigList{
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
					Symbols: []SymbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: MetricTagConfigList{
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
		"match cannot compile regex 2": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbols: []SymbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: MetricTagConfigList{
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
					Symbols: []SymbolConfig{
						{
							OID:          "1.2",
							Name:         "hey",
							ExtractValue: `(\d+)C`,
						},
					},
					MetricTags: MetricTagConfigList{
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
					Symbols: []SymbolConfig{
						{
							OID:                  "1.2",
							Name:                 "hey",
							ExtractValue:         `(\d+)C`,
							ExtractValueCompiled: regexp.MustCompile(`(\d+)C`),
						},
					},
					MetricTags: MetricTagConfigList{
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
		"constant_value_one usage in column symbol": {
			wantError: false,
			metrics: []MetricsConfig{
				{
					Symbols: []SymbolConfig{
						{
							Name:             "abc",
							ConstantValueOne: true,
						},
					},
					MetricTags: MetricTagConfigList{
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
		"constant_value_one usage in scalar symbol": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						Name:             "myMetric",
						ConstantValueOne: true,
					},
				},
			},
		},
		"constant_value_one usage in scalar symbol with OID": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						OID:              "1.2.3",
						Name:             "myMetric",
						ConstantValueOne: true,
					},
				},
			},
		},
		"constant_value_one usage in metric tags": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbols: []SymbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: MetricTagConfigList{
						MetricTagConfig{
							Symbol: SymbolConfigCompat{
								Name:             "abc",
								ConstantValueOne: true,
							},
							Tag: "hello",
						},
					},
				},
			},
		},
		"metric_type usage in column symbol": {
			wantError: false,
			metrics: []MetricsConfig{
				{
					Symbols: []SymbolConfig{
						{
							Name:       "abc",
							OID:        "1.2.3",
							MetricType: ProfileMetricTypeCounter,
						},
					},
					MetricTags: MetricTagConfigList{
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
						MetricType: ProfileMetricTypeCounter,
					},
				},
			},
		},
		"ERROR metric_type usage in metric_tags": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbols: []SymbolConfig{
						{
							Name: "abc",
							OID:  "1.2.3",
						},
					},
					MetricTags: MetricTagConfigList{
						MetricTagConfig{
							Symbol: SymbolConfigCompat{
								Name:       "abc",
								OID:        "1.2.3",
								MetricType: ProfileMetricTypeCounter,
							},
							Tag: "hello",
						},
					},
				},
			},
		},
		"mapping used without tag": {
			wantError: true,
			metrics: []MetricsConfig{
				{
					Symbols: []SymbolConfig{
						{
							OID:  "1.2",
							Name: "abc",
						},
					},
					MetricTags: MetricTagConfigList{
						MetricTagConfig{
							Symbol: SymbolConfigCompat{
								OID:  "1.2",
								Name: "abc",
							},
							Mapping: map[string]string{
								"1": "abc",
								"2": "def",
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

func Test_validateEnrichMetricTags(t *testing.T) {
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
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if tc.wantError {
				assert.Error(t, validateEnrichMetricTags(tc.metrics))
			} else {
				assert.NoError(t, validateEnrichMetricTags(tc.metrics))
			}
			if tc.wantMetrics != nil {
				assert.Equal(t, tc.wantMetrics, tc.metrics)
			}
		})
	}
}

func Test_validateEnrichMetadata(t *testing.T) {
	tests := map[string]struct {
		metadata     MetadataConfig
		wantError    bool
		wantMetadata MetadataConfig
	}{
		"both field symbol and value can be provided": {
			wantError: false,
			metadata: MetadataConfig{
				"device": MetadataResourceConfig{
					Fields: map[string]MetadataField{
						"name": {
							Value: "hey",
							Symbol: SymbolConfig{
								OID:  "1.2.3",
								Name: "someSymbol",
							},
						},
					},
				},
			},
			wantMetadata: MetadataConfig{
				"device": MetadataResourceConfig{
					Fields: map[string]MetadataField{
						"name": {
							Value: "hey",
							Symbol: SymbolConfig{
								OID:  "1.2.3",
								Name: "someSymbol",
							},
						},
					},
				},
			},
		},
		"invalid regex pattern for symbol": {
			wantError: true,
			metadata: MetadataConfig{
				"device": MetadataResourceConfig{
					Fields: map[string]MetadataField{
						"name": {
							Symbol: SymbolConfig{
								OID:          "1.2.3",
								Name:         "someSymbol",
								ExtractValue: "(\\w[)",
							},
						},
					},
				},
			},
		},
		"invalid regex pattern for multiple symbols": {
			wantError: true,
			metadata: MetadataConfig{
				"device": MetadataResourceConfig{
					Fields: map[string]MetadataField{
						"name": {
							Symbols: []SymbolConfig{
								{
									OID:          "1.2.3",
									Name:         "someSymbol",
									ExtractValue: "(\\w[)",
								},
							},
						},
					},
				},
			},
		},
		"field regex pattern is compiled": {
			wantError: false,
			metadata: MetadataConfig{
				"device": MetadataResourceConfig{
					Fields: map[string]MetadataField{
						"name": {
							Symbol: SymbolConfig{
								OID:          "1.2.3",
								Name:         "someSymbol",
								ExtractValue: "(\\w)",
							},
						},
					},
				},
			},
			wantMetadata: MetadataConfig{
				"device": MetadataResourceConfig{
					Fields: map[string]MetadataField{
						"name": {
							Symbol: SymbolConfig{
								OID:                  "1.2.3",
								Name:                 "someSymbol",
								ExtractValue:         "(\\w)",
								ExtractValueCompiled: regexp.MustCompile(`(\w)`),
							},
						},
					},
				},
			},
		},
		"invalid resource": {
			wantError: true,
			metadata: MetadataConfig{
				"invalid-res": MetadataResourceConfig{
					Fields: map[string]MetadataField{
						"name": {
							Value: "hey",
						},
					},
				},
			},
		},
		"invalid field": {
			wantError: true,
			metadata: MetadataConfig{
				"device": MetadataResourceConfig{
					Fields: map[string]MetadataField{
						"invalid-field": {
							Value: "hey",
						},
					},
				},
			},
		},
		"invalid idtags": {
			wantError: true,
			metadata: MetadataConfig{
				"interface": MetadataResourceConfig{
					Fields: map[string]MetadataField{
						"invalid-field": {
							Value: "hey",
						},
					},
					IDTags: MetricTagConfigList{
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
		"device resource does not support id_tags": {
			wantError: true,
			metadata: MetadataConfig{
				"device": MetadataResourceConfig{
					Fields: map[string]MetadataField{
						"name": {
							Value: "hey",
						},
					},
					IDTags: MetricTagConfigList{
						MetricTagConfig{
							Symbol: SymbolConfigCompat{
								OID:  "1.2.3",
								Name: "abc",
							},
							Tag: "abc",
						},
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if tc.wantError {
				assert.Error(t, validateEnrichMetadata(tc.metadata))
			} else {
				assert.NoError(t, validateEnrichMetadata(tc.metadata))
			}
			if tc.wantMetadata != nil {
				assert.Equal(t, tc.wantMetadata, tc.metadata)
			}
		})
	}
}
