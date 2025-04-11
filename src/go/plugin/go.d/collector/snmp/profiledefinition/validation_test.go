// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profiledefinition

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ValidateEnrichMetrics(t *testing.T) {
	tests := []struct {
		name            string
		metrics         []MetricsConfig
		expectedErrors  []string
		expectedMetrics []MetricsConfig
	}{
		{
			name: "either table symbol or scalar symbol must be provided",
			metrics: []MetricsConfig{
				{},
			},
			expectedErrors: []string{
				"either a table symbol or a scalar symbol must be provided",
			},
			expectedMetrics: []MetricsConfig{
				{},
			},
		},
		{
			name: "table column symbols and scalar symbol cannot be both provided",
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
			expectedErrors: []string{
				"table symbol and scalar symbol cannot be both provided",
			},
		},
		{
			name: "multiple errors",
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
			expectedErrors: []string{
				"either a table symbol or a scalar symbol must be provided",
				"table symbol and scalar symbol cannot be both provided",
			},
		},
		{
			name: "missing symbol name",
			metrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						OID: "1.2.3",
					},
				},
			},
			expectedErrors: []string{
				"either a table symbol or a scalar symbol must be provided",
			},
		},
		{
			name: "table column symbol name missing",
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
			expectedErrors: []string{
				"symbol name missing: name=`` oid=`1.2`",
				"symbol oid or send_as_one missing: name=`abc` oid=``",
			},
		},
		{
			name: "table external metric column tag symbol error",
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
			expectedErrors: []string{
				"symbol name missing: name=`` oid=`1.2.3`",
				"symbol oid missing: name=`abc` oid=``",
			},
		},
		{
			name: "missing MetricTags",
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
			expectedErrors: []string{
				"column symbols doesn't have a 'metric_tags' section",
			},
		},
		{
			name: "table external metric column tag MIB error",
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
			expectedErrors: []string{
				"symbol name missing: name=`` oid=`1.2.3`",
				"symbol oid missing: name=`abc` oid=``",
			},
		},
		{
			name: "missing match tags",
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
			expectedErrors: []string{
				"`tags` mapping must be provided if `match` (`([a-z])`) is defined",
			},
		},
		{
			name: "match cannot compile regex",
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
			expectedErrors: []string{
				"cannot compile `match` (`([a-z)`)",
			},
		},
		{
			name: "match cannot compile regex",
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
			expectedErrors: []string{
				"transform rule end should be greater than start. Invalid rule",
			},
		},
		{
			name: "compiling extract_value",
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
			expectedMetrics: []MetricsConfig{
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
			expectedErrors: []string{},
		},
		{
			name: "error compiling extract_value",
			metrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						OID:          "1.2.3",
						Name:         "myMetric",
						ExtractValue: "[{",
					},
				},
			},
			expectedErrors: []string{
				"cannot compile `extract_value`",
			},
		},
		{
			name: "constant_value_one usage in column symbol",
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
			expectedErrors: []string{},
		},
		{
			name: "constant_value_one usage in scalar symbol",
			metrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						Name:             "myMetric",
						ConstantValueOne: true,
					},
				},
			},
			expectedErrors: []string{
				"either a table symbol or a scalar symbol must be provided",
			},
		},
		{
			name: "constant_value_one usage in scalar symbol with OID",
			metrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						OID:              "1.2.3",
						Name:             "myMetric",
						ConstantValueOne: true,
					},
				},
			},
			expectedErrors: []string{
				"`constant_value_one` cannot be used outside of tables",
			},
		},
		{
			name: "constant_value_one usage in metric tags",
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
			expectedErrors: []string{
				"symbol oid missing",
				"`constant_value_one` cannot be used outside of tables",
			},
		},
		{
			name: "metric_type usage in column symbol",
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
			expectedErrors: []string{},
		},
		{
			name: "metric_type usage in scalar symbol",
			metrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{
						Name:       "abc",
						OID:        "1.2.3",
						MetricType: ProfileMetricTypeCounter,
					},
				},
			},
			expectedErrors: []string{},
		},
		{
			name: "ERROR metric_type usage in metric_tags",
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
			expectedErrors: []string{
				"`metric_type` cannot be used outside scalar/table metric symbols and metrics root",
			},
		},
		{
			name: "metric root forced_type converted to metric_type",
			metrics: []MetricsConfig{
				{
					ForcedType: ProfileMetricTypeCounter,
					Symbols: []SymbolConfig{
						{
							Name: "abc",
							OID:  "1.2.3",
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
			expectedMetrics: []MetricsConfig{
				{
					MetricType: ProfileMetricTypeCounter,
					Symbols: []SymbolConfig{
						{
							Name: "abc",
							OID:  "1.2.3",
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
		{
			name: "mapping used without tag",
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
			expectedErrors: []string{"`tag` must be provided if `mapping` (`map[1:abc 2:def]`) is defined"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := ValidateEnrichMetrics(tt.metrics)
			assert.Equal(t, len(tt.expectedErrors), len(errors), fmt.Sprintf("ERRORS: %v", errors))
			for i := range errors {
				assert.Contains(t, errors[i], tt.expectedErrors[i])
			}
			if tt.expectedMetrics != nil {
				assert.Equal(t, tt.expectedMetrics, tt.metrics)
			}
		})
	}
}

func Test_ValidateEnrichMetricTags(t *testing.T) {
	tests := []struct {
		name            string
		metrics         []MetricTagConfig
		expectedErrors  []string
		expectedMetrics []MetricTagConfig
	}{
		{
			name: "Move OID to Symbol",
			metrics: []MetricTagConfig{
				{
					OID: "1.2.3.4",
					Symbol: SymbolConfigCompat{
						Name: "mySymbol",
					},
				},
			},
			expectedMetrics: []MetricTagConfig{
				{
					Symbol: SymbolConfigCompat{
						OID:  "1.2.3.4",
						Name: "mySymbol",
					},
				},
			},
		},
		{
			name: "Metric tag OID and symbol.OID cannot be both declared",
			metrics: []MetricTagConfig{
				{
					OID: "1.2.3.4",
					Symbol: SymbolConfigCompat{
						OID:  "1.2.3.5",
						Name: "mySymbol",
					},
				},
			},
			expectedErrors: []string{
				"metric tag OID and symbol.OID cannot be both declared",
			},
		},
		{
			name: "metric tag symbol and column cannot be both declared",
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
			expectedErrors: []string{
				"metric tag symbol and column cannot be both declared",
			},
		},
		{
			name: "Missing OID",
			metrics: []MetricTagConfig{
				{
					Symbol: SymbolConfigCompat{
						Name: "mySymbol",
					},
				},
			},
			expectedErrors: []string{
				"symbol oid missing",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := ValidateEnrichMetricTags(tt.metrics)
			assert.Equal(t, len(tt.expectedErrors), len(errors), fmt.Sprintf("ERRORS: %v", errors))
			for i := range errors {
				assert.Contains(t, errors[i], tt.expectedErrors[i])
			}
			if tt.expectedMetrics != nil {
				assert.Equal(t, tt.expectedMetrics, tt.metrics)
			}
		})
	}
}

func Test_validateEnrichMetadata(t *testing.T) {
	tests := []struct {
		name             string
		metadata         MetadataConfig
		expectedErrors   []string
		expectedMetadata MetadataConfig
	}{
		{
			name: "both field symbol and value can be provided",
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
			expectedMetadata: MetadataConfig{
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
		{
			name: "invalid regex pattern for symbol",
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
			expectedErrors: []string{
				"cannot compile `extract_value`",
			},
		},
		{
			name: "invalid regex pattern for multiple symbols",
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
			expectedErrors: []string{
				"cannot compile `extract_value`",
			},
		},
		{
			name: "field regex pattern is compiled",
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
			expectedErrors: []string{},
			expectedMetadata: MetadataConfig{
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
		{
			name: "invalid resource",
			metadata: MetadataConfig{
				"invalid-res": MetadataResourceConfig{
					Fields: map[string]MetadataField{
						"name": {
							Value: "hey",
						},
					},
				},
			},
			expectedErrors: []string{
				"invalid resource: invalid-res",
			},
		},
		{
			name: "invalid field",
			metadata: MetadataConfig{
				"device": MetadataResourceConfig{
					Fields: map[string]MetadataField{
						"invalid-field": {
							Value: "hey",
						},
					},
				},
			},
			expectedErrors: []string{
				"invalid resource (device) field: invalid-field",
			},
		},
		{
			name: "invalid idtags",
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
			expectedErrors: []string{
				"invalid resource (interface) field: invalid-field",
				"cannot compile `match` (`([a-z)`)",
			},
		},
		{
			name: "device resource does not support id_tags",
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
			expectedErrors: []string{
				"device resource does not support custom id_tags",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := ValidateEnrichMetadata(tt.metadata)
			assert.Equal(t, len(tt.expectedErrors), len(errors), fmt.Sprintf("ERRORS: %v", errors))
			for i := range errors {
				assert.Contains(t, errors[i], tt.expectedErrors[i])
			}
			if tt.expectedMetadata != nil {
				assert.Equal(t, tt.expectedMetadata, tt.metadata)
			}
		})
	}
}

// TODO: Add test for ValidateEnrichMetricTags
