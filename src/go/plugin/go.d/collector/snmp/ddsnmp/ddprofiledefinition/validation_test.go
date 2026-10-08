// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ddprofiledefinition

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestValidateEnrichProfile_YAML(t *testing.T) {
	tests := map[string]struct {
		input           string
		decodeError     bool
		validationError string
		want            ProfileDefinition
	}{
		"working legacy aliases": {
			input: `sysobjectid: "1.2.3"
metrics:
  - OID: "1.2.3.0"
    name: scalar
    metric_type: gauge
    metric_tags:
      - tag: column_tag
        column: {OID: "1.2.4.0", name: column}
      - OID: "1.2.5.0"
        symbol: fallback_name
`,
			want: ProfileDefinition{
				SysObjectIDs: StringArray{"1.2.3"},
				Selector: SelectorSpec{{SysObjectID: SelectorIncludeExclude{
					Include: []string{"1.2.3"},
				}}},
				Metrics: []MetricsConfig{{
					Symbol: SymbolConfig{
						OID:        "1.2.3.0",
						Name:       "scalar",
						MetricType: ProfileMetricTypeGauge,
					},
					MetricTags: []MetricTagConfig{
						{Tag: "column_tag", Symbol: SymbolConfigCompat{
							OID:  "1.2.4.0",
							Name: "column",
						}},
						{Symbol: SymbolConfigCompat{
							OID:  "1.2.5.0",
							Name: "fallback_name",
						}},
					},
				}},
			},
		},
		"ignored metric options": {
			input: `metrics:
  - symbol: {OID: "1.2.3.0", name: value}
    options: {placement: 1, metric_suffix: unused}
`,
			want: ProfileDefinition{
				Metrics: []MetricsConfig{{Symbol: SymbolConfig{
					OID:  "1.2.3.0",
					Name: "value",
				}}},
			}},
		"ignored topology options": {
			input: `topology:
  - kind: if_status
    symbol: {OID: "1.2.3.0", name: value}
    options: {placement: 1}
`,
			want: ProfileDefinition{
				Topology: []TopologyConfig{{Kind: KindIfStatus, MetricsConfig: MetricsConfig{
					Symbol: SymbolConfig{
						OID:  "1.2.3.0",
						Name: "value",
					},
				}}},
			}},
		"ignored metadata id tags": {
			input: `metadata:
  device:
    fields:
      vendor: {value: example}
    id_tags: [{tag: unused, index: 1}]
`,
			want: ProfileDefinition{
				Metadata: MetadataConfig{
					"device": {Fields: map[string]MetadataField{"vendor": {Value: "example"}}},
				},
			}},
		"unsupported interface metadata": {
			input: `metadata:
  interface:
    fields:
      name: {value: unused}
`,
			validationError: "invalid resource: interface"},
		"unsupported string tag references": {
			input: `metrics:
  - table: {OID: "1.2.3", name: table}
    symbols: [{OID: "1.2.3.1", name: value}]
    metric_tags: [unused]
`,
			decodeError: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var profile ProfileDefinition
			err := yaml.Unmarshal([]byte(tc.input), &profile)
			if tc.decodeError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			err = ValidateEnrichProfile(&profile)
			if tc.validationError != "" {
				require.ErrorContains(t, err, tc.validationError)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, profile)
			require.NoError(t, ValidateEnrichProfile(&profile))
			assert.Equal(t, tc.want, profile, "repeated validation must be idempotent")
		})
	}
}
