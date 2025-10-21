// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package ddprofiledefinition

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCloneSymbolConfig(t *testing.T) {
	s := SymbolConfig{
		OID:                  "1.2.3.4",
		Name:                 "foo",
		ExtractValue:         ".*",
		ExtractValueCompiled: regexp.MustCompile(".*"),
		MatchPattern:         ".*",
		MatchPatternCompiled: regexp.MustCompile(".*"),
		MatchValue:           "$1",
		ScaleFactor:          100,
		Format:               "mac_address",
		ConstantValueOne:     true,
		MetricType:           ProfileMetricTypeCounter,
	}
	s2 := s.Clone()
	assert.Equal(t, s, s2)
	// An issue with our previous deepcopy was that regexes were duplicated but
	// didn't keep their private internals, causing them to fail when used, so
	// double-check that the regex works fine.
	assert.True(t, s2.ExtractValueCompiled.MatchString("foo"))
}

func TestCloneSymbolConfigCompat(t *testing.T) {
	s := SymbolConfigCompat{
		OID:                  "1.2.3.4",
		Name:                 "foo",
		ExtractValue:         ".*",
		ExtractValueCompiled: regexp.MustCompile(".*"),
		MatchPattern:         ".*",
		MatchPatternCompiled: regexp.MustCompile(".*"),
		MatchValue:           "$1",
		ScaleFactor:          100,
		Format:               "mac_address",
		ConstantValueOne:     true,
		MetricType:           ProfileMetricTypeCounter,
	}
	s2 := s.Clone()
	assert.Equal(t, s, s2)
}

func TestCloneMetricTagConfig(t *testing.T) {
	c := MetricTagConfig{
		Tag:   "foo",
		Index: 10,
		Column: SymbolConfig{
			OID:                  "1.2.3.4",
			ExtractValue:         ".*",
			ExtractValueCompiled: regexp.MustCompile(".*"),
		},
		OID:    "2.4",
		Symbol: SymbolConfigCompat{},
		IndexTransform: []MetricIndexTransform{
			{
				Start: 0,
				End:   1,
			},
		},
		Mapping: map[string]string{
			"1": "bar",
			"2": "baz",
		},
		Match:   ".*",
		Pattern: regexp.MustCompile(".*"),
		Tags: map[string]string{
			"foo": "$1",
		},
		SymbolTag: "baz",
	}
	c2 := c.Clone()
	assert.Equal(t, c, c2)
	c2.Tags["bar"] = "$2"
	c2.IndexTransform = append(c2.IndexTransform, MetricIndexTransform{1, 3})
	c2.Mapping["3"] = "foo"
	c2.Tag = "bar"
	assert.NotEqual(t, c, c2)
	// Validate that c has not changed
	assert.Equal(t, c, MetricTagConfig{
		Tag:   "foo",
		Index: 10,
		Column: SymbolConfig{
			OID:                  "1.2.3.4",
			ExtractValue:         ".*",
			ExtractValueCompiled: regexp.MustCompile(".*"),
		},
		OID:    "2.4",
		Symbol: SymbolConfigCompat{},
		IndexTransform: []MetricIndexTransform{
			{
				Start: 0,
				End:   1,
			},
		},
		Mapping: map[string]string{
			"1": "bar",
			"2": "baz",
		},
		Match:   ".*",
		Pattern: regexp.MustCompile(".*"),
		Tags: map[string]string{
			"foo": "$1",
		},
		SymbolTag: "baz",
	})
}

func TestCloneMetricsConfig(t *testing.T) {
	buildConf := func() MetricsConfig {
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
			Options: MetricsConfigOption{
				Placement:    1,
				MetricSuffix: ".foo",
			},
		}
	}
	conf := buildConf()
	unchanged := buildConf()

	conf2 := conf.Clone()
	assert.Equal(t, conf, conf2)
	conf2.StaticTags[0] = StaticMetricTagConfig{Tag: "bar", Value: "baz"}
	conf2.MetricTags[0].IndexTransform = []MetricIndexTransform{{5, 7}}
	conf2.Options.Placement = 2
	conf2.Options.MetricSuffix = ".bar"
	assert.Equal(t, unchanged, conf)
	assert.NotEqual(t, conf, conf2)
}

func TestCloneEmpty(t *testing.T) {
	mc := MetricsConfig{}
	assert.Equal(t, mc, mc.Clone())
	sym := SymbolConfig{}
	assert.Equal(t, sym, sym.Clone())
	tag := MetricTagConfig{}
	assert.Equal(t, tag, tag.Clone())
}
