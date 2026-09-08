// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ddprofiledefinition

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestValidateEnrichProfile_RetiredDatadogSyntax(t *testing.T) {
	tests := map[string]struct {
		input           string
		decodeError     bool
		validationError string
	}{
		"ignored metric options": {input: `metrics:
  - symbol: {OID: "1.2.3.0", name: value}
    options: {placement: 1, metric_suffix: unused}
`},
		"ignored topology options": {input: `topology:
  - kind: if_status
    symbol: {OID: "1.2.3.0", name: value}
    options: {placement: 1}
`},
		"ignored metadata id tags": {input: `metadata:
  device:
    fields:
      vendor: {value: example}
    id_tags: [{tag: unused, index: 1}]
`},
		"unsupported interface metadata": {input: `metadata:
  interface:
    fields:
      name: {value: unused}
`, validationError: "invalid resource: interface"},
		"unsupported string tag references": {input: `metrics:
  - table: {OID: "1.2.3", name: table}
    symbols: [{OID: "1.2.3.1", name: value}]
    metric_tags: [unused]
`, decodeError: true},
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
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateEnrichProfile_WorkingLegacyAliases(t *testing.T) {
	const input = `sysobjectid: "1.2.3"
metrics:
  - OID: "1.2.3.0"
    name: scalar
    metric_type: gauge
    metric_tags:
      - tag: column_tag
        column: {OID: "1.2.4.0", name: column}
      - OID: "1.2.5.0"
        symbol: fallback_name
`
	var profile ProfileDefinition
	require.NoError(t, yaml.Unmarshal([]byte(input), &profile))
	require.NoError(t, ValidateEnrichProfile(&profile))
	require.True(t, profile.Selector.HasExactOidMatch("1.2.3"))
	require.Equal(t, ProfileMetricTypeGauge, profile.Metrics[0].Symbol.MetricType)
	require.Equal(t, "scalar", profile.Metrics[0].Symbol.Name)
	require.Equal(t, "1.2.3.0", profile.Metrics[0].Symbol.OID)
	require.Equal(t, "1.2.4.0", profile.Metrics[0].MetricTags[0].Symbol.OID)
	require.Equal(t, "1.2.5.0", profile.Metrics[0].MetricTags[1].Symbol.OID)
	require.Equal(t, "fallback_name", profile.Metrics[0].MetricTags[1].Symbol.Name)
	before := profile.Clone()
	require.NoError(t, ValidateEnrichProfile(&profile))
	require.Equal(t, before, &profile)
}
