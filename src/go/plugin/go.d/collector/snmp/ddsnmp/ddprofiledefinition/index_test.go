// SPDX-License-Identifier: GPL-3.0-or-later

package ddprofiledefinition

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestIndexDecodeValidation(t *testing.T) {
	length := uint(48)
	valid := MetricTagConfig{Tag: "name", IndexDecode: &MetricIndexDecode{Components: []MetricIndexComponent{{Type: "integer"}, {Type: "octet_string"}, {Type: "octet_string", Length: &length}}, Component: 2}}
	for name, tc := range map[string]struct {
		mutate    func(*MetricTagConfig)
		scalar    bool
		errorText string
	}{
		"valid":                   {},
		"scalar":                  {scalar: true, errorText: "do not support `index_decode`"},
		"empty":                   {mutate: func(c *MetricTagConfig) { c.IndexDecode.Components = nil }, errorText: "nonempty"},
		"zero component":          {mutate: func(c *MetricTagConfig) { c.IndexDecode.Component = 0 }, errorText: "one-based"},
		"component beyond layout": {mutate: func(c *MetricTagConfig) { c.IndexDecode.Component = 4 }, errorText: "one-based"},
		"unknown type":            {mutate: func(c *MetricTagConfig) { c.IndexDecode.Components[0].Type = "implied" }, errorText: "unsupported type"},
		"integer length":          {mutate: func(c *MetricTagConfig) { c.IndexDecode.Components[0].Length = &length }, errorText: "integer does not support"},
		"zero fixed length":       {mutate: func(c *MetricTagConfig) { zero := uint(0); c.IndexDecode.Components[2].Length = &zero }, errorText: "positive"},
		"index":                   {mutate: func(c *MetricTagConfig) { c.Index = 1 }, errorText: "cannot be combined"},
		"transform":               {mutate: func(c *MetricTagConfig) { c.IndexTransform = []MetricIndexTransform{{Start: 1}} }, errorText: "cannot be combined"},
		"table":                   {mutate: func(c *MetricTagConfig) { c.Table = "other" }, errorText: "cannot be combined"},
		"OID":                     {mutate: func(c *MetricTagConfig) { c.OID = "1.2.3" }, errorText: "cannot be combined"},
		"symbol OID":              {mutate: func(c *MetricTagConfig) { c.Symbol.OID = "1.2.3" }, errorText: "cannot be combined"},
		"column OID":              {mutate: func(c *MetricTagConfig) { c.Column.OID = "1.2.3" }, errorText: "cannot be combined"},
		"index_from":              {mutate: func(c *MetricTagConfig) { c.IndexFrom = []MetricTagIndexSource{{Table: "other"}} }, errorText: "cannot be combined"},
		"lookup":                  {mutate: func(c *MetricTagConfig) { c.LookupSymbol.Name = "lookup" }, errorText: "cannot be combined"},
	} {
		t.Run(name, func(t *testing.T) {
			tag := valid.Clone()
			if tc.mutate != nil {
				tc.mutate(&tag)
			}
			err := validateEnrichMetricTag("row.tag", &tag, tc.scalar)
			if tc.errorText != "" {
				require.ErrorContains(t, err, tc.errorText)
			} else {
				require.NoError(t, err)
				assert.True(t, tag.IsIndexTag())
			}
		})
	}
}

func TestIndexSchemaYAMLRoundTripAndClone(t *testing.T) {
	const data = `metrics:
  - table: {OID: 1.2, name: rows}
    symbols: [{OID: 1.2.1, name: value}]
    metric_tags:
      - tag: name
        index_decode:
          components: [{type: integer}, {type: octet_string, length: 48}]
          component: 2
`
	var original ProfileDefinition
	require.NoError(t, yaml.Unmarshal([]byte(data), &original))
	require.NoError(t, ValidateEnrichProfile(&original))
	encoded, err := yaml.Marshal(original)
	require.NoError(t, err)
	var decoded ProfileDefinition
	require.NoError(t, yaml.Unmarshal(encoded, &decoded))
	require.NoError(t, ValidateEnrichProfile(&decoded))
	assert.Equal(t, original, decoded)
	cloned := original.Clone()
	*cloned.Metrics[0].MetricTags[0].IndexDecode.Components[1].Length = 99
	assert.EqualValues(t, 48, *original.Metrics[0].MetricTags[0].IndexDecode.Components[1].Length)
}
