// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ddprofiledefinition

import (
	"testing"

	"gopkg.in/yaml.v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricTagConfig_UnmarshalYAML(t *testing.T) {
	tests := map[string]struct {
		input     string
		want      MetricsConfig
		wantError bool
	}{
		"index tag": {
			input: `
metric_tags:
- index: 3
`,
			want: MetricsConfig{
				MetricTags: []MetricTagConfig{{Index: 3}},
			}},
		"unsupported string reference": {input: "metric_tags: [aaa]", wantError: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var got MetricsConfig
			err := yaml.Unmarshal([]byte(tc.input), &got)
			if tc.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestStringArray_UnmarshalYAML(t *testing.T) {
	type config struct {
		SomeIDs StringArray `yaml:"my_field"`
	}
	tests := map[string]struct {
		input string
		want  config
	}{
		"array": {
			input: `
my_field:
 - aaa
 - bbb
`,
			want: config{
				SomeIDs: StringArray{"aaa", "bbb"},
			}},
		"string": {
			input: `
my_field: aaa
`,
			want: config{
				SomeIDs: StringArray{"aaa"},
			}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var got config
			err := yaml.Unmarshal([]byte(tc.input), &got)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestSymbolConfigCompat_UnmarshalYAML(t *testing.T) {
	type config struct {
		SymbolField SymbolConfigCompat `yaml:"my_symbol_field"`
	}
	tests := map[string]struct {
		input string
		want  config
	}{
		"object": {
			input: `
my_symbol_field:
  name: aSymbol
  OID: 1.2.3
`,
			want: config{
				SymbolField: SymbolConfigCompat{
					OID:  "1.2.3",
					Name: "aSymbol",
				},
			}},
		"legacy string": {
			input: `
my_symbol_field: aSymbol
`,
			want: config{
				SymbolField: SymbolConfigCompat{
					Name: "aSymbol",
				},
			}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var got config
			err := yaml.Unmarshal([]byte(tc.input), &got)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestMappingConfig_UnmarshalYAML(t *testing.T) {
	type config struct {
		Mapping MappingConfig `yaml:"mapping"`
	}
	tests := map[string]struct {
		input string
		want  config
	}{
		"legacy map": {
			input: `
mapping:
  1: up
  2: down
`,
			want: config{
				Mapping: NewExactMapping(map[string]string{
					"1": "up",
					"2": "down",
				}),
			}},
		"structured exact": {
			input: `
mapping:
  items:
    1: up
    2: down
`,
			want: config{
				Mapping: NewExactMapping(map[string]string{
					"1": "up",
					"2": "down",
				}),
			}},
		"structured bitmask": {
			input: `
mapping:
  mode: bitmask
  items:
    1: internalError
    128: processorPresent
`,
			want: config{
				Mapping: MappingConfig{
					Mode: MappingModeBitmask,
					Items: map[string]string{
						"1":   "internalError",
						"128": "processorPresent",
					},
				},
			}},
		"literal mode and items keys": {
			input: `
mapping:
  mode: active
  items: present
`,
			want: config{
				Mapping: NewExactMapping(map[string]string{
					"mode":  "active",
					"items": "present",
				}),
			}},
		"empty legacy map": {
			input: `
mapping: {}
`,
			want: config{}},
		"empty structured items": {
			input: `
mapping:
  items: {}
`,
			want: config{}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var got config
			err := yaml.Unmarshal([]byte(tc.input), &got)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
