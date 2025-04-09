// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package profiledefinition

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSplitOIDs(t *testing.T) {
	type testCase struct {
		name            string
		metrics         []MetricsConfig
		tags            []MetricTagConfig
		metadata        MetadataConfig
		expectedScalars []string
		expectedColumns []string
	}
	testCases := []testCase{
		{
			name: "scalar metric",
			metrics: []MetricsConfig{{
				Symbol: SymbolConfig{
					OID: "1.2.3.4",
				},
			}},
			expectedScalars: []string{"1.2.3.4"},
		}, {
			name: "tabular metric",
			metrics: []MetricsConfig{{
				Symbols: []SymbolConfig{{OID: "1.2.3.4"}},
				MetricTags: []MetricTagConfig{
					{Symbol: SymbolConfigCompat{
						OID: "2.3.4.5",
					}},
				},
			}},
			expectedColumns: []string{"1.2.3.4", "2.3.4.5"},
		}, {
			name: "tags",
			tags: []MetricTagConfig{
				{Symbol: SymbolConfigCompat{
					OID: "2.3.4.5",
				},
				},
			},
			expectedScalars: []string{"2.3.4.5"},
		}, {
			name: "metadata",
			metadata: map[string]MetadataResourceConfig{
				"device": {
					Fields: map[string]MetadataField{
						"vendor": {Value: "static"},
						"name": {Symbol: SymbolConfig{
							OID: "1.1",
						}},
						"os_name": {Symbols: []SymbolConfig{
							{
								OID: "1.2",
							}, {
								OID: "1.3",
							},
						}},
					},
					IDTags: []MetricTagConfig{
						{Symbol: SymbolConfigCompat{
							OID: "1.4",
						}},
					},
				},
				"not_device": {
					Fields: map[string]MetadataField{
						"vendor": {Value: "static"},
						"name": {Symbol: SymbolConfig{
							OID: "2.1",
						}},
						"os_name": {Symbols: []SymbolConfig{
							{
								OID: "2.2",
							}, {
								OID: "2.3",
							},
						}},
					},
					IDTags: []MetricTagConfig{
						{Symbol: SymbolConfigCompat{
							OID: "2.4",
						}},
					},
				},
			},
			expectedScalars: []string{"1.1", "1.2", "1.3", "1.4"},
			expectedColumns: []string{"2.1", "2.2", "2.3", "2.4"},
		}, {
			name: "duplicates",
			metrics: []MetricsConfig{
				{
					Symbol: SymbolConfig{OID: "1.1"},
				}, {
					Symbols: []SymbolConfig{
						{OID: "1.1"},
					},
					MetricTags: []MetricTagConfig{
						{Symbol: SymbolConfigCompat{OID: "1.1"}},
					},
				}},
			metadata: map[string]MetadataResourceConfig{
				"device": {
					Fields: map[string]MetadataField{
						"name": {Symbol: SymbolConfig{
							OID: "1.1",
						}},
					},
				},
			},
			expectedScalars: []string{"1.1"},
			expectedColumns: []string{"1.1"},
		}, {
			name: "sorting",
			metrics: []MetricsConfig{
				{Symbol: SymbolConfig{OID: "1.2"}},
				{Symbol: SymbolConfig{OID: "1.1"}},
				{
					Symbols: []SymbolConfig{
						{OID: "2.4"},
						{OID: "2.3"},
					},
					MetricTags: []MetricTagConfig{
						{Symbol: SymbolConfigCompat{OID: "2.2"}},
						{Symbol: SymbolConfigCompat{OID: "2.1"}},
					},
				}},
			expectedScalars: []string{"1.1", "1.2"},
			expectedColumns: []string{"2.1", "2.2", "2.3", "2.4"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			scalars, columns := splitOIDs(tc.metrics, tc.tags, tc.metadata)
			expectedScalars := tc.expectedScalars
			if expectedScalars == nil {
				expectedScalars = []string{}
			}
			assert.Equal(t, expectedScalars, scalars)
			expectedColumns := tc.expectedColumns
			if expectedColumns == nil {
				expectedColumns = []string{}
			}
			assert.Equal(t, expectedColumns, columns)
		})
	}
}

func TestProfileSplitOIDs(t *testing.T) {
	p := ProfileDefinition{
		Metrics: []MetricsConfig{
			{Symbol: SymbolConfig{OID: "1.2"}},
			{Symbol: SymbolConfig{OID: "1.1"}},
			{
				Symbols: []SymbolConfig{
					{OID: "2.4"},
					{OID: "2.3"},
				},
				MetricTags: []MetricTagConfig{
					{Symbol: SymbolConfigCompat{OID: "2.2"}},
					{Symbol: SymbolConfigCompat{OID: "2.1"}},
				},
			},
		},
		MetricTags: []MetricTagConfig{
			{Symbol: SymbolConfigCompat{OID: "1.4"}},
			{Symbol: SymbolConfigCompat{OID: "1.3"}},
		},
		Metadata: map[string]MetadataResourceConfig{
			"device": {
				Fields: map[string]MetadataField{
					"vendor": {Value: "static"},
					"name": {Symbol: SymbolConfig{
						OID: "3.4",
					}},
					"os_name": {Symbols: []SymbolConfig{
						{
							OID: "3.3",
						}, {
							OID: "3.2",
						},
					}},
				},
				IDTags: []MetricTagConfig{
					{Symbol: SymbolConfigCompat{
						OID: "3.1",
					}},
				},
			},
			"not_device": {
				Fields: map[string]MetadataField{
					"vendor": {Value: "static"},
					"name": {Symbol: SymbolConfig{
						OID: "4.4",
					}},
					"os_name": {Symbols: []SymbolConfig{
						{
							OID: "4.3",
						}, {
							OID: "4.2",
						},
					}},
				},
				IDTags: []MetricTagConfig{
					{Symbol: SymbolConfigCompat{
						OID: "4.1",
					}},
				},
			},
		},
	}
	scalars, columns := p.SplitOIDs(true)
	assert.Equal(t, []string{"1.1", "1.2", "1.3", "1.4", "3.1", "3.2", "3.3", "3.4"}, scalars)
	assert.Equal(t, []string{"2.1", "2.2", "2.3", "2.4", "4.1", "4.2", "4.3", "4.4"}, columns)

	scalars, columns = p.SplitOIDs(false)
	assert.Equal(t, []string{"1.1", "1.2", "1.3", "1.4"}, scalars)
	assert.Equal(t, []string{"2.1", "2.2", "2.3", "2.4"}, columns)
}
