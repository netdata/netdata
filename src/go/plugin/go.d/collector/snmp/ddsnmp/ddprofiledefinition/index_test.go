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
		"scope":                   {mutate: func(c *MetricTagConfig) { c.LookupScope = &MetricLookupScope{} }, errorText: "cannot be combined"},
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

func TestIndexTransformMappingValidation(t *testing.T) {
	for name, tc := range map[string]struct {
		rule      MetricIndexTransform
		errorText string
	}{
		"legacy slice":  {rule: MetricIndexTransform{Start: 1}},
		"single arc":    {rule: MetricIndexTransform{Start: 1, End: 1, Mapping: map[string]string{"4": "1", "16": "2"}}},
		"zero arc":      {rule: MetricIndexTransform{Mapping: map[string]string{"0": "4294967295"}}},
		"slice mapping": {rule: MetricIndexTransform{Start: 1, End: 2, Mapping: map[string]string{"4": "1"}}, errorText: "single-arc"},
		"drop mapping":  {rule: MetricIndexTransform{DropRight: 1, Mapping: map[string]string{"4": "1"}}, errorText: "single-arc"},
		"reverse slice": {rule: MetricIndexTransform{Start: 3, End: 2}, errorText: "greater than start"},
		"end and drop":  {rule: MetricIndexTransform{End: 2, DropRight: 1}, errorText: "both end and drop_right"},
	} {
		t.Run(name, func(t *testing.T) {
			err := validateIndexTransforms([]MetricIndexTransform{tc.rule})
			if tc.errorText == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.errorText)
			}
		})
	}
	for _, invalid := range []string{"", "01", "-1", "+1", "1.0", " 1", "4294967296"} {
		t.Run("invalid arc "+invalid, func(t *testing.T) {
			require.Error(t, validateIndexTransforms([]MetricIndexTransform{{Mapping: map[string]string{invalid: "1"}}}))
			require.Error(t, validateIndexTransforms([]MetricIndexTransform{{Mapping: map[string]string{"1": invalid}}}))
		})
	}
}

func TestLookupScopeValidation(t *testing.T) {
	valid := MetricTagConfig{Tag: "description", Table: "peer", Symbol: SymbolConfigCompat{OID: "1.2.1", Name: "description"}, LookupSymbol: SymbolConfigCompat{OID: "1.2.2", Name: "address"}, LookupScope: &MetricLookupScope{SourceIndexTransform: []MetricIndexTransform{{Start: 1, End: 1}}, TargetIndexTransform: []MetricIndexTransform{{Start: 0, End: 0}}}}
	for name, tc := range map[string]struct {
		mutate    func(*MetricTagConfig)
		errorText string
	}{
		"valid":                 {},
		"no table":              {mutate: func(c *MetricTagConfig) { c.Table = "" }, errorText: "requires table"},
		"no lookup":             {mutate: func(c *MetricTagConfig) { c.LookupSymbol = SymbolConfigCompat{} }, errorText: "requires table"},
		"no source scope":       {mutate: func(c *MetricTagConfig) { c.LookupScope.SourceIndexTransform = nil }, errorText: "nonempty"},
		"no target scope":       {mutate: func(c *MetricTagConfig) { c.LookupScope.TargetIndexTransform = nil }, errorText: "nonempty"},
		"invalid scope mapping": {mutate: func(c *MetricTagConfig) { c.LookupScope.TargetIndexTransform[0].Mapping = map[string]string{"01": "1"} }, errorText: "canonical"},
		"index_from":            {mutate: func(c *MetricTagConfig) { c.IndexFrom = []MetricTagIndexSource{{Table: "link"}} }, errorText: "cannot be combined with index_from"},
	} {
		t.Run(name, func(t *testing.T) {
			tag := valid.Clone()
			if tc.mutate != nil {
				tc.mutate(&tag)
			}
			err := validateEnrichMetricTag("row.tag", &tag, false)
			if tc.errorText == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.errorText)
			}
		})
	}
	bgp := BGPValueConfig{Table: valid.Table, Symbol: SymbolConfig(valid.Symbol), LookupSymbol: valid.LookupSymbol, LookupScope: valid.LookupScope.Clone()}
	require.NoError(t, validateEnrichBGPValue("bgp.value", &bgp, true))
	bgp.LookupScope.TargetIndexTransform = nil
	require.ErrorContains(t, validateEnrichBGPValue("bgp.value", &bgp, true), "nonempty")
	for name, validate := range map[string]func([]MetricIndexTransform) error{
		"BGP": func(rules []MetricIndexTransform) error {
			return validateEnrichBGPValue("value", &BGPValueConfig{IndexTransform: rules}, true)
		},
		"licensing": func(rules []MetricIndexTransform) error {
			return validateEnrichLicenseValue("value", &LicenseValueConfig{IndexTransform: rules}, true)
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.ErrorContains(t, validate([]MetricIndexTransform{{Mapping: map[string]string{"bad": "1"}}}), "canonical")
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
      - tag: description
        table: peer
        symbol: {OID: 1.3.1, name: description}
        lookup_symbol: {OID: 1.3.2, name: address}
        index_transform: [{start: 1, end: 1, mapping: {4: 1, 16: 2}}]
        lookup_scope:
          source_index_transform: [{start: 1, end: 1, mapping: {4: 1}}]
          target_index_transform: [{start: 0, end: 0, mapping: {4: 1}}]
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
	cloned.Metrics[0].MetricTags[1].IndexTransform[0].Mapping["4"] = "9"
	cloned.Metrics[0].MetricTags[1].LookupScope.SourceIndexTransform[0].Mapping["4"] = "9"
	cloned.Metrics[0].MetricTags[1].LookupScope.TargetIndexTransform[0].Mapping["4"] = "9"
	assert.EqualValues(t, 48, *original.Metrics[0].MetricTags[0].IndexDecode.Components[1].Length)
	assert.Equal(t, "1", original.Metrics[0].MetricTags[1].IndexTransform[0].Mapping["4"])
	assert.Equal(t, "1", original.Metrics[0].MetricTags[1].LookupScope.SourceIndexTransform[0].Mapping["4"])
	assert.Equal(t, "1", original.Metrics[0].MetricTags[1].LookupScope.TargetIndexTransform[0].Mapping["4"])
	rules := []MetricIndexTransform{{Mapping: map[string]string{"4": "1"}}}
	bgp := BGPValueConfig{IndexTransform: rules, LookupScope: original.Metrics[0].MetricTags[1].LookupScope.Clone()}
	bgpClone := bgp.Clone()
	bgpClone.IndexTransform[0].Mapping["4"] = "9"
	bgpClone.LookupScope.SourceIndexTransform[0].Mapping["4"] = "9"
	assert.Equal(t, "1", bgp.IndexTransform[0].Mapping["4"])
	assert.Equal(t, "1", bgp.LookupScope.SourceIndexTransform[0].Mapping["4"])
	license := LicenseValueConfig{IndexTransform: rules}
	licenseClone := license.Clone()
	licenseClone.IndexTransform[0].Mapping["4"] = "9"
	assert.Equal(t, "1", license.IndexTransform[0].Mapping["4"])
	for _, data := range []string{"lookup_scope: {}", "lookup_scope: {source_index_transform: [{start: 1, end: 1}]}"} {
		var value LicenseValueConfig
		require.NoError(t, yaml.Unmarshal([]byte(data), &value))
		require.ErrorContains(t, validateEnrichLicenseValue("value", &value, true), "do not support lookup_scope")
	}
}

func TestLookupScopeRejectsBypassedSelectors(t *testing.T) {
	scope := &MetricLookupScope{SourceIndexTransform: []MetricIndexTransform{{Start: 0, End: 0}}, TargetIndexTransform: []MetricIndexTransform{{Start: 0, End: 0}}}
	tag := MetricTagConfig{Tag: "label", Table: "peer", Symbol: SymbolConfigCompat{OID: "1.2.1", Name: "value"}, LookupSymbol: SymbolConfigCompat{OID: "1.2.2", Name: "key"}, LookupScope: scope}
	t.Run("tag index", func(t *testing.T) {
		scoped := tag.Clone()
		scoped.Index = 1
		require.ErrorContains(t, validateEnrichMetricTag("tag", &scoped, false), "lookup_scope cannot be combined with index")
		scoped.LookupScope = nil
		require.NoError(t, validateEnrichMetricTag("tag", &scoped, false), "legacy index precedence stays supported")
	})
	for name, mutate := range map[string]func(*BGPValueConfig){
		"literal":        func(value *BGPValueConfig) { value.Value = "65001" },
		"index":          func(value *BGPValueConfig) { value.Index = 1 },
		"index from end": func(value *BGPValueConfig) { value.IndexFromEnd = 1 },
	} {
		t.Run("BGP "+name, func(t *testing.T) {
			value := BGPValueConfig{Table: tag.Table, Symbol: SymbolConfig(tag.Symbol), LookupSymbol: tag.LookupSymbol, LookupScope: scope.Clone()}
			mutate(&value)
			require.ErrorContains(t, validateEnrichBGPValue("value", &value, true), "lookup_scope cannot be combined")
			value.LookupScope = nil
			require.NoError(t, validateEnrichBGPValue("value", &value, true), "legacy value precedence stays supported")
		})
	}
}

func TestLookupScopeRejectsSameTable(t *testing.T) {
	scope := &MetricLookupScope{SourceIndexTransform: []MetricIndexTransform{{Start: 0, End: 0}}, TargetIndexTransform: []MetricIndexTransform{{Start: 0, End: 0}}}
	tag := MetricTagConfig{Tag: "label", Table: "peer", Symbol: SymbolConfigCompat{OID: "1.2.1", Name: "value"}, LookupSymbol: SymbolConfigCompat{OID: "1.2.2", Name: "key"}, LookupScope: scope}
	row := MetricsConfig{Table: SymbolConfig{OID: "1.2", Name: "peer"}, Symbols: []SymbolConfig{{OID: "1.2.3", Name: "metric"}}, MetricTags: []MetricTagConfig{tag}}
	require.ErrorContains(t, validateEnrichMetrics([]MetricsConfig{row}), "lookup_scope requires a different target table")
	bgp := validBGPPeerFamilyRow()
	bgp.Identity.RemoteAS = BGPValueConfig{Table: bgp.Table.Name, Symbol: SymbolConfig{OID: bgp.Table.OID + ".1", Name: "remoteAs"}, LookupSymbol: SymbolConfigCompat{OID: bgp.Table.OID + ".2", Name: "key"}, LookupScope: scope.Clone()}
	require.ErrorContains(t, validateEnrichBGP([]BGPConfig{bgp}), "lookup_scope requires a different target table")
}
