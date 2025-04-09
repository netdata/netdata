// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package profiledefinition

import (
	"github.com/stretchr/testify/assert"
	"regexp"
	"testing"
)

func makeMetadata() MetadataConfig {
	// This is not actually a valid config, since e.g. it has ExtractValue and
	// MatchPattern both set; this is just to check that every field gets copied
	// properly.
	return MetadataConfig{
		"device": MetadataResourceConfig{
			Fields: map[string]MetadataField{
				"name": {
					Value: "hey",
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
						ConstantValueOne:     true,
						MetricType:           "gauge",
					},
				},
			},
			IDTags: []MetricTagConfig{
				{
					Tag:   "foo",
					Index: 1,
					Column: SymbolConfig{
						Name:                 "bar",
						OID:                  "1.2.3",
						ExtractValue:         ".*",
						ExtractValueCompiled: regexp.MustCompile(".*"),
					},
					OID: "2.3.4",
					Symbol: SymbolConfigCompat{
						OID:                  "1.2.3",
						Name:                 "someSymbol",
						ExtractValue:         ".*",
						ExtractValueCompiled: regexp.MustCompile(".*"),
					},
					IndexTransform: []MetricIndexTransform{
						{
							Start: 1,
							End:   5,
						},
					},
					Mapping: map[string]string{
						"1": "on",
						"2": "off",
					},
					Match:   ".*",
					Pattern: regexp.MustCompile(".*"),
					Tags: map[string]string{
						"foo": "bar",
					},
					SymbolTag: "ok",
				},
			},
		},
	}
}

func TestCloneMetadata(t *testing.T) {
	metadata := makeMetadata()
	metaCopy := metadata.Clone()
	assert.Equal(t, metadata, metaCopy)
	// Modify the copy in place
	metaCopy["interface"] = MetadataResourceConfig{}
	metaCopy["device"].Fields["foo"] = MetadataField{
		Value: "foo",
	}
	// Original has not changed
	assert.Equal(t, makeMetadata(), metadata)
	// New one is different
	assert.NotEqual(t, metadata, metaCopy)
}
