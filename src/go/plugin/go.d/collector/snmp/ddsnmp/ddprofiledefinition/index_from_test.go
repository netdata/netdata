// SPDX-License-Identifier: GPL-3.0-or-later

package ddprofiledefinition

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func indexFromTestTag() MetricTagConfig {
	return MetricTagConfig{
		Tag:   "interface",
		Table: "ifXTable",
		Symbol: SymbolConfigCompat{
			OID:  "1.3.6.1.2.1.31.1.1.1.1",
			Name: "ifName",
		},
		IndexFrom: []MetricTagIndexSource{{
			Table: "physicalTable",
			Symbol: SymbolConfig{
				OID:  "1.2.3.1",
				Name: "physicalIndex",
			},
		}},
	}
}

func TestMetricTagConfig_IndexFromDecode(t *testing.T) {
	want := indexFromTestTag()
	want.IndexFrom[0].Symbol.Mapping = NewExactMapping(map[string]string{"1": "2"})
	for name, tc := range map[string]struct {
		input  string
		decode func([]byte, any) error
	}{
		"yaml": {
			input: `tag: interface
table: ifXTable
symbol: {OID: 1.3.6.1.2.1.31.1.1.1.1, name: ifName}
index_from:
  - table: physicalTable
    symbol: {OID: 1.2.3.1, name: physicalIndex, mapping: {1: '2'}}
`,
			decode: yaml.Unmarshal,
		},
		"json": {
			input:  `{"tag":"interface","table":"ifXTable","symbol":{"OID":"1.3.6.1.2.1.31.1.1.1.1","name":"ifName"},"index_from":[{"table":"physicalTable","symbol":{"OID":"1.2.3.1","name":"physicalIndex","mapping":{"mode":"exact","items":{"1":"2"}}}}]}`,
			decode: json.Unmarshal,
		},
	} {
		t.Run(name, func(t *testing.T) {
			var got MetricTagConfig
			require.NoError(t, tc.decode([]byte(tc.input), &got))
			assert.Equal(t, want, got)
		})
	}
	for name, input := range map[string]string{
		"string symbol": `index_from: [{table: t, symbol: oldSymbol}]`,
		"string hop":    `index_from: [oldSource]`,
	} {
		t.Run(name, func(t *testing.T) {
			var got MetricTagConfig
			require.Error(t, yaml.Unmarshal([]byte(input), &got))
		})
	}
}

func TestMetricTagConfig_IndexFromClone(t *testing.T) {
	tag := indexFromTestTag()
	tag.IndexFrom[0].Symbol.Mapping = NewExactMapping(map[string]string{"1": "2"})
	profile := ProfileDefinition{
		MetricTags: []GlobalMetricTagConfig{{MetricTagConfig: tag}},
		Metrics:    []MetricsConfig{{MetricTags: []MetricTagConfig{tag}}},
		Topology: []TopologyConfig{{MetricsConfig: MetricsConfig{
			MetricTags: []MetricTagConfig{tag},
		}}},
		Licensing: []LicensingConfig{{MetricTags: []MetricTagConfig{tag}}},
		BGP:       []BGPConfig{{MetricTags: []MetricTagConfig{tag}}},
	}
	before, err := json.Marshal(profile)
	require.NoError(t, err)
	cloned := profile.Clone()
	assert.Equal(t, profile, *cloned)
	for _, clonedTag := range []*MetricTagConfig{
		&cloned.MetricTags[0].MetricTagConfig,
		&cloned.Metrics[0].MetricTags[0],
		&cloned.Topology[0].MetricTags[0],
		&cloned.Licensing[0].MetricTags[0],
		&cloned.BGP[0].MetricTags[0],
	} {
		clonedTag.IndexFrom[0].Table = "changed"
		clonedTag.IndexFrom[0].Symbol.Mapping.Items["1"] = "changed"
	}
	after, err := json.Marshal(profile)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
}

func TestValidateEnrichMetricTag_IndexFrom(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate    func(*MetricTagConfig)
		scalar    bool
		wantError string
	}{
		"valid":                    {},
		"transformed source index": {mutate: func(tag *MetricTagConfig) { tag.IndexTransform = []MetricIndexTransform{{Start: 1}} }},
		"missing final table":      {mutate: func(tag *MetricTagConfig) { tag.Table = "" }, wantError: "requires final `table`"},
		"missing final oid":        {mutate: func(tag *MetricTagConfig) { tag.Symbol.OID = "" }, wantError: "requires final `symbol.OID`"},
		"missing final name":       {mutate: func(tag *MetricTagConfig) { tag.Symbol.Name = "" }, wantError: "requires final `symbol.OID`"},
		"index":                    {mutate: func(tag *MetricTagConfig) { tag.Index = 1 }, wantError: "cannot be combined with `index`"},
		"scalar":                   {scalar: true, wantError: "scalar metric_tags do not support `index_from`"},
		"hop table":                {mutate: func(tag *MetricTagConfig) { tag.IndexFrom[0].Table = "" }, wantError: "index_from[0]: requires `table`"},
		"hop oid":                  {mutate: func(tag *MetricTagConfig) { tag.IndexFrom[0].Symbol.OID = "" }, wantError: "index_from[0]: symbol oid missing"},
		"hop name":                 {mutate: func(tag *MetricTagConfig) { tag.IndexFrom[0].Symbol.Name = "" }, wantError: "index_from[0]: symbol name missing"},
		"metric type":              {mutate: func(tag *MetricTagConfig) { tag.IndexFrom[0].Symbol.MetricType = ProfileMetricTypeGauge }, wantError: "`metric_type` cannot be used"},
		"scale":                    {mutate: func(tag *MetricTagConfig) { tag.IndexFrom[0].Symbol.ScaleFactor = 2 }, wantError: "`scale_factor` is not supported"},
		"chart":                    {mutate: func(tag *MetricTagConfig) { tag.IndexFrom[0].Symbol.ChartMeta.Unit = "bytes" }, wantError: "`chart_meta` is not supported"},
		"transform":                {mutate: func(tag *MetricTagConfig) { tag.IndexFrom[0].Symbol.Transform = "{{ .Value }}" }, wantError: "`transform` is not supported"},
		"bitmask": {mutate: func(tag *MetricTagConfig) {
			tag.IndexFrom[0].Symbol.Mapping = MappingConfig{
				Mode:  MappingModeBitmask,
				Items: map[string]string{"1": "2"},
			}
		}, wantError: "bitmask` is only supported"},
		"invalid extraction":    {mutate: func(tag *MetricTagConfig) { tag.IndexFrom[0].Symbol.ExtractValue = "[" }, wantError: "cannot compile `extract_value`"},
		"no extraction capture": {mutate: func(tag *MetricTagConfig) { tag.IndexFrom[0].Symbol.ExtractValue = "^[0-9]+$" }, wantError: "`extract_value` requires a capture group"},
		"invalid match": {mutate: func(tag *MetricTagConfig) {
			tag.IndexFrom[0].Symbol.MatchPattern = "["
			tag.IndexFrom[0].Symbol.MatchValue = "$1"
		}, wantError: "cannot compile `match_pattern`"},
		"match without replacement": {mutate: func(tag *MetricTagConfig) { tag.IndexFrom[0].Symbol.MatchPattern = "(.*)" }, wantError: "`match_pattern` requires `match_value`"},
		"replacement without match": {mutate: func(tag *MetricTagConfig) { tag.IndexFrom[0].Symbol.MatchValue = "$1" }, wantError: "`match_value` requires `match_pattern`"},
	} {
		t.Run(name, func(t *testing.T) {
			tag := indexFromTestTag()
			if tc.mutate != nil {
				tc.mutate(&tag)
			}
			err := validateEnrichMetricTag("row.metric_tags[0]", &tag, tc.scalar)
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				return
			}
			require.NoError(t, err)
			assert.False(t, tag.IsIndexTag())
		})
	}
}

func TestValidateEnrichMetricTag_IndexFromPatterns(t *testing.T) {
	for name, replacement := range map[string]string{"dollar": "$1.0", "backslash": `\1.0`, "constant": "1.0"} {
		t.Run(name, func(t *testing.T) {
			tag := indexFromTestTag()
			sym := &tag.IndexFrom[0].Symbol
			sym.Format = "hex"
			sym.ExtractValue = "^([0-9]+)$"
			sym.MatchPattern = "^([0-9]+)$"
			sym.MatchValue = replacement
			sym.Mapping = NewExactMapping(map[string]string{"1.0": "2.0"})
			require.NoError(t, validateEnrichMetricTag("", &tag, false))
			require.NotNil(t, sym.ExtractValueCompiled)
			require.NotNil(t, sym.MatchPatternCompiled)
			assert.Equal(t, []string{"12", "12"}, sym.ExtractValueCompiled.FindStringSubmatch("12"))
			assert.Equal(t, []string{"12", "12"}, sym.MatchPatternCompiled.FindStringSubmatch("12"))
		})
	}
}

func TestMetricTagConfig_TableSources(t *testing.T) {
	tag := indexFromTestTag()
	tag.IndexFrom = append(tag.IndexFrom, MetricTagIndexSource{
		Table: "aliasTable",
		Symbol: SymbolConfig{
			OID:  "1.2.4.1",
			Name: "aliasPointer",
		},
	})
	tag.LookupSymbol = SymbolConfigCompat{
		OID:  "1.3.6.1.2.1.31.1.1.1.2",
		Name: "ifKey",
	}
	want := []MetricTagIndexSource{
		tag.IndexFrom[0],
		tag.IndexFrom[1],
		{Table: tag.Table, Symbol: SymbolConfig(tag.Symbol)},
		{Table: tag.Table, Symbol: SymbolConfig(tag.LookupSymbol)},
	}
	assert.Equal(t, want, slices.Collect(tag.TableSources()))
	var first []MetricTagIndexSource
	for source := range tag.TableSources() {
		first = append(first, source)
		break
	}
	assert.Equal(t, want[:1], first)
	assert.Nil(t, slices.Collect((MetricTagConfig{
		Symbol: tag.Symbol,
	}).TableSources()))
	tag.IndexFrom = append(tag.IndexFrom, MetricTagIndexSource{
		Table: "missingOID",
	}, MetricTagIndexSource{
		Symbol: SymbolConfig{
			OID: "1.2",
		},
	})
	assert.Equal(t, want, slices.Collect(tag.TableSources()))
}

func TestValidateEnrichProfile_IndexFromContexts(t *testing.T) {
	for name, profile := range map[string]ProfileDefinition{
		"global": {MetricTags: []GlobalMetricTagConfig{{MetricTagConfig: indexFromTestTag()}}},
		"metric": {Metrics: []MetricsConfig{{MetricTags: []MetricTagConfig{indexFromTestTag()}}}},
		"topology": {Topology: []TopologyConfig{{Kind: KindIfName, MetricsConfig: MetricsConfig{
			MetricTags: []MetricTagConfig{indexFromTestTag()},
		}}}},
		"licensing": {Licensing: []LicensingConfig{{MetricTags: []MetricTagConfig{indexFromTestTag()}}}},
		"bgp":       {BGP: []BGPConfig{{Kind: BGPRowKindDevice, MetricTags: []MetricTagConfig{indexFromTestTag()}}}},
	} {
		t.Run(name, func(t *testing.T) {
			err := ValidateEnrichProfile(&profile)
			require.ErrorContains(t, err, "scalar metric_tags do not support `index_from`")
		})
	}
}

func TestMetricTagConfig_EmptyIndexFromPreservesIndexTag(t *testing.T) {
	for name, chain := range map[string][]MetricTagIndexSource{"absent": nil, "empty": {}} {
		t.Run(name, func(t *testing.T) {
			tag := MetricTagConfig{
				Tag:       "index",
				Index:     1,
				IndexFrom: chain,
			}
			require.NoError(t, validateEnrichMetricTag("", &tag, false))
			assert.True(t, tag.IsIndexTag())
		})
	}
}

func TestOutOfTableOIDs_IndexFromSources(t *testing.T) {
	for name, mutate := range map[string]func(*MetricTagConfig){
		"hop": func(tag *MetricTagConfig) {
			tag.IndexFrom[0] = MetricTagIndexSource{
				Table: "source",
				Symbol: SymbolConfig{
					OID:  "1.2.4.1",
					Name: "bad",
				},
			}
		},
		"final": func(tag *MetricTagConfig) { tag.Table = "source"; tag.Symbol.OID = "1.2.4.1" },
		"lookup": func(tag *MetricTagConfig) {
			tag.Table = "source"
			tag.Symbol.OID = "1.2.3.1"
			tag.LookupSymbol = SymbolConfigCompat{
				OID:  "1.2.4.1",
				Name: "bad",
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			tag := indexFromTestTag()
			mutate(&tag)
			table := SymbolConfig{
				OID:  "1.2.3",
				Name: "source",
			}
			row := MetricsConfig{
				Table:      table,
				Symbols:    []SymbolConfig{{OID: "1.2.3.1", Name: "value"}},
				MetricTags: []MetricTagConfig{tag},
			}
			profile := ProfileDefinition{
				Metrics:   []MetricsConfig{row},
				Topology:  []TopologyConfig{{MetricsConfig: row}},
				Licensing: []LicensingConfig{{Table: table, MetricTags: []MetricTagConfig{tag}}},
				BGP:       []BGPConfig{{Table: table, MetricTags: []MetricTagConfig{tag}}},
			}
			assert.Equal(t, []string{
				"metrics[0].metric_tags[0]: OID 1.2.4.1 is outside table.OID 1.2.3 and is never collected",
				"topology[0].metric_tags[0]: OID 1.2.4.1 is outside table.OID 1.2.3 and is never collected",
				"licensing[0].metric_tags[0]: OID 1.2.4.1 is outside table.OID 1.2.3 and is never collected",
				"bgp[0].metric_tags[0]: OID 1.2.4.1 is outside table.OID 1.2.3 and is never collected",
			}, OutOfTableOIDs(&profile))
		})
	}
}

func TestValidateEnrichProfile_IndexFromTableContexts(t *testing.T) {
	tag := indexFromTestTag()
	tag.IndexFrom[0].Symbol.ExtractValue = "^([0-9]+)$"
	table := SymbolConfig{
		OID:  "1.2.3",
		Name: "source",
	}
	row := MetricsConfig{
		Table:      table,
		Symbols:    []SymbolConfig{{OID: "1.2.3.1", Name: "value"}},
		MetricTags: []MetricTagConfig{tag},
	}
	bgp := validBGPPeerFamilyRow()
	bgp.MetricTags = []MetricTagConfig{tag}
	for name, profile := range map[string]ProfileDefinition{
		"metrics":  {Metrics: []MetricsConfig{row}},
		"topology": {Topology: []TopologyConfig{{Kind: KindIfName, MetricsConfig: row}}},
		"licensing": {Licensing: []LicensingConfig{{
			Table: table,
			State: LicenseStateConfig{
				LicenseValueConfig: LicenseValueConfig{
					Symbol: SymbolConfig{
						OID:  "1.2.3.1",
						Name: "licenseState",
					},
				},
			},
			MetricTags: []MetricTagConfig{tag},
		}}},
		"bgp": {BGP: []BGPConfig{bgp}},
	} {
		t.Run(name, func(t *testing.T) {
			cloned := profile.Clone()
			require.NoError(t, ValidateEnrichProfile(cloned))
			var got MetricTagConfig
			switch name {
			case "metrics":
				got = cloned.Metrics[0].MetricTags[0]
			case "topology":
				got = cloned.Topology[0].MetricTags[0]
			case "licensing":
				got = cloned.Licensing[0].MetricTags[0]
			case "bgp":
				got = cloned.BGP[0].MetricTags[0]
			}
			require.NotNil(t, got.IndexFrom[0].Symbol.ExtractValueCompiled)
			want := tag.Clone()
			want.IndexFrom[0].Symbol.ExtractValueCompiled = got.IndexFrom[0].Symbol.ExtractValueCompiled
			assert.Equal(t, want, got)
		})
	}
}
