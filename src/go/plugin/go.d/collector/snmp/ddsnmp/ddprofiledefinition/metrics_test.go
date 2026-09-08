// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package ddprofiledefinition

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSymbolConfig_Clone(t *testing.T) {
	tests := map[string]struct{ symbol SymbolConfig }{
		"empty": {},
		"populated": {symbol: SymbolConfig{
			OID:                  "1.2.3.4",
			Name:                 "foo",
			ExtractValue:         ".*",
			ExtractValueCompiled: regexp.MustCompile(".*"),
			MatchPattern:         ".*",
			MatchPatternCompiled: regexp.MustCompile(".*"),
			MatchValue:           "$1",
			ScaleFactor:          100,
			Format:               "mac_address",
			MetricType:           "counter",
		}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			variants := map[string]struct {
				clone func(SymbolConfig) SymbolConfig
			}{
				"symbol": {clone: func(s SymbolConfig) SymbolConfig { return s.Clone() }},
				"compatibility symbol": {
					clone: func(s SymbolConfig) SymbolConfig { return SymbolConfig(SymbolConfigCompat(s).Clone()) },
				},
			}
			for name, variant := range variants {
				t.Run(name, func(t *testing.T) {
					got := variant.clone(tc.symbol)
					require.Equal(t, tc.symbol, got)
					// Executing regexes catches copies that lose regexp's private state.
					if got.ExtractValueCompiled != nil {
						assert.True(t, got.ExtractValueCompiled.MatchString("foo"))
					}
					if got.MatchPatternCompiled != nil {
						assert.True(t, got.MatchPatternCompiled.MatchString("foo"))
					}
				})
			}
		})
	}
}

func TestMetricTagConfig_Clone(t *testing.T) {
	tests := map[string]struct {
		newTag func() MetricTagConfig
		mutate func(*MetricTagConfig)
	}{
		"empty": {newTag: func() MetricTagConfig { return MetricTagConfig{} }},
		"populated": {
			newTag: func() MetricTagConfig {
				return MetricTagConfig{
					Tag:   "foo",
					Index: 10,
					Column: SymbolConfig{
						OID:                  "1.2.3.4",
						ExtractValue:         ".*",
						ExtractValueCompiled: regexp.MustCompile(".*"),
					},
					OID:    "2.4",
					Symbol: SymbolConfigCompat{},
					LookupSymbol: SymbolConfigCompat{
						OID:                  "2.4.6.8",
						ExtractValue:         ".*",
						ExtractValueCompiled: regexp.MustCompile(".*"),
					},
					IndexTransform: []MetricIndexTransform{
						{
							Start: 0,
							End:   1,
						},
					},
					Mapping: NewExactMapping(map[string]string{
						"1": "bar",
						"2": "baz",
					}),
					Match:   ".*",
					Pattern: regexp.MustCompile(".*"),
					Tags: map[string]string{
						"foo": "$1",
					},
				}
			},
			mutate: func(cloned *MetricTagConfig) {
				cloned.Tags["bar"] = "$2"
				cloned.IndexTransform[0].Start = 2
				cloned.IndexTransform = append(cloned.IndexTransform, MetricIndexTransform{
					Start: 1,
					End:   3,
				})
				cloned.Mapping.Items["3"] = "foo"
				cloned.Tag = "bar"
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			original, want := tc.newTag(), tc.newTag()
			cloned := original.Clone()
			require.Equal(t, want, cloned)
			if tc.mutate != nil {
				tc.mutate(&cloned)
				assert.NotEqual(t, want, cloned)
			}
			assert.Equal(t, want, original)
		})
	}
}

func TestMetricsConfig_Clone(t *testing.T) {
	tests := map[string]struct {
		newMetrics func() MetricsConfig
		mutate     func(*MetricsConfig)
	}{
		"empty": {newMetrics: func() MetricsConfig { return MetricsConfig{} }},
		"populated": {
			newMetrics: func() MetricsConfig {
				return MetricsConfig{
					MIB: "FOO-MIB",
					Table: SymbolConfig{
						OID:                  "1.2.3.4",
						ExtractValue:         ".*",
						ExtractValueCompiled: regexp.MustCompile(".*"),
					},
					Symbol: SymbolConfig{
						OID:                  "1.2.3.4",
						ExtractValue:         ".*",
						ExtractValueCompiled: regexp.MustCompile(".*"),
					},
					OID:  "1.2.3.4",
					Name: "foo",
					Symbols: []SymbolConfig{
						{
							OID:                  "1.2.3.4",
							ExtractValue:         ".*",
							ExtractValueCompiled: regexp.MustCompile(".*"),
						},
					},
					StaticTags: []StaticMetricTagConfig{
						{Tag: "foo", Value: "bar"},
					},
					MetricTags: []MetricTagConfig{
						{
							IndexTransform: make([]MetricIndexTransform, 0),
						},
					},
					MetricType: ProfileMetricTypeGauge,
				}
			},
			mutate: func(cloned *MetricsConfig) {
				cloned.StaticTags[0] = StaticMetricTagConfig{
					Tag:   "bar",
					Value: "baz",
				}
				cloned.MetricTags[0].IndexTransform = []MetricIndexTransform{{Start: 5, End: 7}}
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			original, want := tc.newMetrics(), tc.newMetrics()
			cloned := original.Clone()
			require.Equal(t, want, cloned)
			if tc.mutate != nil {
				tc.mutate(&cloned)
				assert.NotEqual(t, want, cloned)
			}
			assert.Equal(t, want, original)
		})
	}
}
