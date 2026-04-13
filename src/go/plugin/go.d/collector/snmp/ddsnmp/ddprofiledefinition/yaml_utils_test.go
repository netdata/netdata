// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ddprofiledefinition

import (
	"testing"

	"gopkg.in/yaml.v2"

	"github.com/stretchr/testify/assert"
)

type MyStringArray struct {
	SomeIDs StringArray `yaml:"my_field"`
}

type MySymbolStruct struct {
	SymbolField SymbolConfigCompat `yaml:"my_symbol_field"`
}

type MyMappingStruct struct {
	Mapping MappingConfig `yaml:"mapping"`
}

func Test_metricTagConfig_UnmarshalYAML(t *testing.T) {
	myStruct := MetricsConfig{}
	expected := MetricsConfig{MetricTags: []MetricTagConfig{{Index: 3}}}

	yaml.Unmarshal([]byte(`
metric_tags:
- index: 3
`), &myStruct)

	assert.Equal(t, expected, myStruct)
}

func Test_metricTagConfig_onlyTags(t *testing.T) {
	myStruct := MetricsConfig{}
	expected := MetricsConfig{MetricTags: []MetricTagConfig{{SymbolTag: "aaa"}}}

	yaml.Unmarshal([]byte(`
metric_tags:
- aaa
`), &myStruct)

	assert.Equal(t, expected, myStruct)
}

func TestStringArray_UnmarshalYAML_array(t *testing.T) {
	myStruct := MyStringArray{}
	expected := MyStringArray{SomeIDs: StringArray{"aaa", "bbb"}}

	yaml.Unmarshal([]byte(`
my_field:
 - aaa
 - bbb
`), &myStruct)

	assert.Equal(t, expected, myStruct)
}

func TestStringArray_UnmarshalYAML_string(t *testing.T) {
	myStruct := MyStringArray{}
	expected := MyStringArray{SomeIDs: StringArray{"aaa"}}

	yaml.Unmarshal([]byte(`
my_field: aaa
`), &myStruct)

	assert.Equal(t, expected, myStruct)
}

func TestSymbolConfig_UnmarshalYAML_symbolObject(t *testing.T) {
	myStruct := MySymbolStruct{}
	expected := MySymbolStruct{SymbolField: SymbolConfigCompat{OID: "1.2.3", Name: "aSymbol"}}

	yaml.Unmarshal([]byte(`
my_symbol_field:
  name: aSymbol
  OID: 1.2.3
`), &myStruct)

	assert.Equal(t, expected, myStruct)
}

func TestSymbolConfig_UnmarshalYAML_symbolString(t *testing.T) {
	myStruct := MySymbolStruct{}
	expected := MySymbolStruct{SymbolField: SymbolConfigCompat{Name: "aSymbol"}}

	yaml.Unmarshal([]byte(`
my_symbol_field: aSymbol
`), &myStruct)

	assert.Equal(t, expected, myStruct)
}

func TestMappingConfig_UnmarshalYAML_legacyMap(t *testing.T) {
	myStruct := MyMappingStruct{}
	expected := MyMappingStruct{
		Mapping: NewExactMapping(map[string]string{
			"1": "up",
			"2": "down",
		}),
	}

	yaml.Unmarshal([]byte(`
mapping:
  1: up
  2: down
`), &myStruct)

	assert.Equal(t, expected, myStruct)
}

func TestMappingConfig_UnmarshalYAML_structuredExact(t *testing.T) {
	myStruct := MyMappingStruct{}
	expected := MyMappingStruct{
		Mapping: NewExactMapping(map[string]string{
			"1": "up",
			"2": "down",
		}),
	}

	yaml.Unmarshal([]byte(`
mapping:
  items:
    1: up
    2: down
`), &myStruct)

	assert.Equal(t, expected, myStruct)
}

func TestMappingConfig_UnmarshalYAML_structuredBitmask(t *testing.T) {
	myStruct := MyMappingStruct{}
	expected := MyMappingStruct{
		Mapping: NewBitmaskMapping(map[string]string{
			"1":   "internalError",
			"128": "processorPresent",
		}),
	}

	yaml.Unmarshal([]byte(`
mapping:
  mode: bitmask
  items:
    1: internalError
    128: processorPresent
`), &myStruct)

	assert.Equal(t, expected, myStruct)
}

func TestMappingConfig_UnmarshalYAML_legacyMapWithLiteralModeAndItemsKeys(t *testing.T) {
	myStruct := MyMappingStruct{}
	expected := MyMappingStruct{
		Mapping: NewExactMapping(map[string]string{
			"mode":  "active",
			"items": "present",
		}),
	}

	err := yaml.Unmarshal([]byte(`
mapping:
  mode: active
  items: present
`), &myStruct)

	assert.NoError(t, err)
	assert.Equal(t, expected, myStruct)
}

func TestMappingConfig_UnmarshalYAML_emptyLegacyMapNormalizesToZeroValue(t *testing.T) {
	myStruct := MyMappingStruct{}
	expected := MyMappingStruct{}

	err := yaml.Unmarshal([]byte(`
mapping: {}
`), &myStruct)

	assert.NoError(t, err)
	assert.Equal(t, expected, myStruct)
}

func TestMappingConfig_UnmarshalYAML_emptyStructuredItemsNormalizesToZeroValue(t *testing.T) {
	myStruct := MyMappingStruct{}
	expected := MyMappingStruct{}

	err := yaml.Unmarshal([]byte(`
mapping:
  items: {}
`), &myStruct)

	assert.NoError(t, err)
	assert.Equal(t, expected, myStruct)
}
