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

func TestMetadataConfig_Clone(t *testing.T) {
	tests := map[string]struct {
		newMetadata func() MetadataConfig
		mutate      func(MetadataConfig)
	}{
		"nil":   {newMetadata: func() MetadataConfig { return nil }},
		"empty": {newMetadata: func() MetadataConfig { return MetadataConfig{} }},
		"populated": {
			// Clone fixtures exercise every field, regardless of schema validation rules.
			newMetadata: func() MetadataConfig {
				return MetadataConfig{
					"device": MetadataResourceConfig{
						Fields: map[string]MetadataField{
							"name": {
								Value:     "hey",
								Consumers: ConsumerSet{ConsumerMetrics, ConsumerTopology},
								Symbol: SymbolConfig{
									OID:                  "1.2.3",
									Name:                 "someSymbol",
									ExtractValue:         ".*",
									ExtractValueCompiled: regexp.MustCompile(".*"),
									MatchPattern:         ".*",
									MatchPatternCompiled: regexp.MustCompile(".*"),
									MatchValue:           "$1",
									ScaleFactor:          100,
									Format:               "mac_address",
									MetricType:           "gauge",
								},
							},
						},
					},
				}
			},
			mutate: func(cloned MetadataConfig) {
				delete(cloned["device"].Fields, "name")
				cloned["device"].Fields["foo"] = MetadataField{
					Value: "foo",
				}
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			original, want := tc.newMetadata(), tc.newMetadata()
			cloned := original.Clone()
			require.Equal(t, want, cloned)
			if tc.mutate != nil {
				tc.mutate(cloned)
				assert.NotEqual(t, want, cloned)
			}
			assert.Equal(t, want, original)
		})
	}
}
