// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"fmt"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func decodeContractProfile(t testing.TB, raw string) *ddprofiledefinition.ProfileDefinition {
	t.Helper()
	var def ddprofiledefinition.ProfileDefinition
	require.NoError(t, yaml.Unmarshal([]byte(raw), &def))
	require.NoError(t, ddprofiledefinition.ValidateEnrichProfile(&def))
	return &def
}

func TestDeviceMetadataCollector_SysobjectIDMetadataWithoutDeviceResource(t *testing.T) {
	for name, tc := range map[string]struct {
		oid, ordinary string
		want          string
	}{
		"no ordinary metadata":    {oid: "1.2.3", want: "example"},
		"empty ordinary metadata": {oid: "1.2.3", ordinary: "metadata: {device: {fields: {}}}\n", want: "example"},
		"nonmatching selector":    {oid: "1.2.4"},
		"missing sysobjectid":     {},
	} {
		t.Run(name, func(t *testing.T) {
			def := decodeContractProfile(t, tc.ordinary+`sysobjectid_metadata:
  - sysobjectid: "1.2.3"
    metadata:
      model: {value: example}
`)
			got, err := newDeviceMetadataCollector(nil, nil, logger.New(), tc.oid).collect(&ddsnmp.Profile{
				Definition: def,
			})
			require.NoError(t, err)
			assert.Equal(t, tc.want, got["model"].Value)
		})
	}
}

func TestTableRowProcessor_StockPowerSupplyMetrics(t *testing.T) {
	prof, err := ddsnmp.LoadProfileByName("_hp-compaq-health")
	require.NoError(t, err)
	var cfg ddprofiledefinition.MetricsConfig
	for _, metric := range prof.Definition.Metrics {
		if metric.Table.Name == "cpqHeFltTolPowerSupplyTable" {
			cfg = metric
		}
	}
	require.NotEmpty(t, cfg.Symbols)
	row := &tableRowData{
		index:     "1",
		pdus:      map[string]gosnmp.SnmpPDU{},
		tableName: cfg.Table.Name,
	}
	for _, sym := range cfg.Symbols {
		if sym.OID != "" {
			row.pdus[sym.OID] = gosnmp.SnmpPDU{
				Name:  sym.OID + ".1",
				Type:  gosnmp.Integer,
				Value: 1,
			}
		}
	}
	report := &AcquisitionRouteReport{}
	ctx := &tableRowProcessingContext{
		config:     cfg,
		columnOIDs: buildColumnOIDs(cfg),
		processing: processingFor(report, "1"),
	}
	got, err := newTableRowProcessor(logger.New()).processRowMetrics(row, ctx)
	require.NoError(t, err)
	assert.Len(t, got, 3)
	assert.Empty(t, report.Processing)
	require.NoError(t, ddprofiledefinition.ValidateEnrichProfile(prof.Definition))
}

func TestTableTagProcessor_TransformationChain(t *testing.T) {
	for name, tc := range map[string]struct {
		fields, input string
		want          map[string]string
	}{
		"extraction then mapping":              {fields: "  extract_value: 'value=([0-9]+)'\nmapping: {1: up}\n", input: "value=1", want: map[string]string{"state": "up"}},
		"replacement then mapping":             {fields: "  match_pattern: '^value=([0-9]+)$'\n  match_value: '$1'\nmapping: {1: up}\n", input: "value=1", want: map[string]string{"state": "up"}},
		"all transformations":                  {fields: "  extract_value: 'value=([0-9]+)'\n  match_pattern: '([0-9]+)'\n  match_value: 'code$1'\nmapping: {code1: up}\nmatch: '^(up)$'\ntags: {status: '$1'}\n", input: "value=1", want: map[string]string{"status": "up"}},
		"unmatched extraction retains input":   {fields: "  extract_value: 'value=([0-9]+)'\n", input: "unknown", want: map[string]string{"state": "unknown"}},
		"unmatched replacement omits tag":      {fields: "  match_pattern: '^value=([0-9]+)$'\n  match_value: '$1'\nmapping: {unknown: up}\n", input: "unknown", want: map[string]string{}},
		"unmatched final pattern omits tags":   {fields: "  extract_value: 'value=([0-9]+)'\nmapping: {1: down}\nmatch: '^(up)$'\ntags: {status: '$1'}\n", input: "value=1", want: map[string]string{}},
		"mapping miss retains extracted input": {fields: "  extract_value: 'value=([0-9]+)'\nmapping: {2: down}\n", input: "value=1", want: map[string]string{"state": "1"}},
	} {
		t.Run(name, func(t *testing.T) {
			raw := "tag: state\nsymbol:\n  OID: 1.2.3.0\n  name: state\n" + tc.fields
			var tag ddprofiledefinition.GlobalMetricTagConfig
			require.NoError(t, yaml.Unmarshal([]byte(raw), &tag))
			def := ddprofiledefinition.ProfileDefinition{
				MetricTags: []ddprofiledefinition.GlobalMetricTagConfig{tag},
			}
			require.NoError(t, ddprofiledefinition.ValidateEnrichProfile(&def))
			cfg := def.MetricTags[0].MetricTagConfig
			pdu := gosnmp.SnmpPDU{
				Name:  "1.2.3.0",
				Type:  gosnmp.OctetString,
				Value: []byte(tc.input),
			}
			for _, global := range []bool{false, true} {
				tags := map[string]string{}
				var err error
				if global {
					err = newGlobalTagProcessor().processTag(cfg, map[string]gosnmp.SnmpPDU{"1.2.3.0": pdu}, tagAdder{
						tags: tags,
					})
				} else {
					err = newTableTagProcessor().processTag(cfg, pdu, tagAdder{
						tags: tags,
					})
				}
				require.NoError(t, err)
				assert.Equal(t, tc.want, tags, "global=%v", global)
			}
		})
	}
}

func TestTableRowProcessor_IndexTransformBounds(t *testing.T) {
	maxUint := ^uint(0)
	for name, tc := range map[string]struct {
		start, end, drop uint
		want             string
	}{
		"inclusive first":        {want: "1"},
		"inclusive range":        {start: 1, end: 2, want: "2.3"},
		"open ended":             {start: 1, want: "2.3"},
		"drop right":             {drop: 1, want: "1.2"},
		"too large end":          {end: 3},
		"overflowing end":        {end: maxUint - 1},
		"maximum end":            {end: maxUint},
		"overflowing start":      {start: maxUint - 1, end: maxUint},
		"overflowing drop right": {drop: maxUint - 1},
		"drop all":               {drop: 3},
	} {
		t.Run(name, func(t *testing.T) {
			def := decodeContractProfile(t, fmt.Sprintf(`metrics:
  - table: {OID: 1.2.3, name: example}
    symbols: [{OID: 1.2.3.1, name: example}]
    metric_tags:
      - tag: index
        index_transform: [{start: %d, end: %d, drop_right: %d}]
`, tc.start, tc.end, tc.drop))
			require.NotPanics(t, func() {
				_, got, err := newTableRowProcessor(logger.New()).processIndexTag(def.Metrics[0].MetricTags[0], "1.2.3")
				if tc.want == "" {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
					assert.Equal(t, tc.want, got)
				}
			})
		})
	}
}

func TestBGPState_EffectiveMapping(t *testing.T) {
	const canonical = `{1: idle, 2: connect, 3: active, 4: opensent, 5: openconfirm, 6: established}`
	const reversed = `{1: established, 2: connect, 3: active, 4: opensent, 5: openconfirm, 6: idle}`
	for name, tc := range map[string]struct {
		mapping, symbolMapping, want string
		wantErr                      bool
	}{
		"value mapping":                    {mapping: canonical, symbolMapping: "{}", want: "established"},
		"symbol mapping":                   {mapping: "{}", symbolMapping: canonical, want: "established"},
		"symbol precedence":                {mapping: canonical, symbolMapping: reversed, want: "idle"},
		"invalid ignored value mapping":    {mapping: "{6: invalid-state}", symbolMapping: canonical, want: "established"},
		"invalid effective symbol mapping": {mapping: canonical, symbolMapping: "{6: invalid-state}", wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			raw := fmt.Sprintf(`bgp:
  - kind: peer
    id: example
    identity:
      neighbor: {value: "192.0.2.1"}
      remote_as: {value: "65001"}
    state:
      value: "6"
      mapping: %s
      symbol:
        OID: 1.2.3.0
        name: bgpPeerState
        mapping: %s
`, tc.mapping, tc.symbolMapping)
			var def ddprofiledefinition.ProfileDefinition
			require.NoError(t, yaml.Unmarshal([]byte(raw), &def))
			err := ddprofiledefinition.ValidateEnrichProfile(&def)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid BGP peer state")
				return
			}
			require.NoError(t, err)
			var got ddsnmp.BGPState
			require.NoError(t, (&Collector{}).populateBGPStateValue(&got, def.BGP[0].State, bgpValueContext{}))
			assert.True(t, got.Has)
			assert.Equal(t, ddprofiledefinition.BGPPeerState(tc.want), got.State)
		})
	}
}

// These benchmarks exercise per-row work; configuration and regex compilation occur before timing.
func BenchmarkProfileTagProcessing(b *testing.B) {
	for name, fields := range map[string]string{"plain": "", "mapping": "mapping: {1: up}\n", "extract": "  extract_value: 'value=([0-9]+)'\n", "combined": "  extract_value: 'value=([0-9]+)'\nmapping: {1: up}\n"} {
		b.Run(name, func(b *testing.B) {
			var tag ddprofiledefinition.GlobalMetricTagConfig
			require.NoError(
				b,
				yaml.Unmarshal([]byte("tag: state\nsymbol:\n  OID: 1.2.3.0\n  name: state\n"+fields), &tag),
			)
			def := ddprofiledefinition.ProfileDefinition{
				MetricTags: []ddprofiledefinition.GlobalMetricTagConfig{tag},
			}
			require.NoError(b, ddprofiledefinition.ValidateEnrichProfile(&def))
			cfg := def.MetricTags[0].MetricTagConfig
			pdu := gosnmp.SnmpPDU{
				Name:  "1.2.3.0",
				Type:  gosnmp.OctetString,
				Value: []byte("value=1"),
			}
			p := newTableTagProcessor()
			tags := map[string]string{}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				clear(tags)
				if err := p.processTag(cfg, pdu, tagAdder{
					tags: tags,
				}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkProfileIndexTransform(b *testing.B) {
	p := newTableRowProcessor(logger.New())
	transforms := []ddprofiledefinition.MetricIndexTransform{{Start: 1, DropRight: 1}}
	b.ReportAllocs()
	for b.Loop() {
		p.crossTableResolver.applyIndexTransform("1.2.3.4", transforms)
	}
}

func BenchmarkProfileDeviceMetadata(b *testing.B) {
	def := decodeContractProfile(b, "metadata: {device: {fields: {model: {value: example}}}}")
	prof := &ddsnmp.Profile{
		Definition: def,
	}
	c := newDeviceMetadataCollector(nil, nil, logger.New(), "1.2.3")
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := c.collect(prof); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProfileBGPValueSymbol(b *testing.B) {
	cfg := scalarBGPTestConfig().State.BGPValueConfig
	b.ReportAllocs()
	for b.Loop() {
		cfg.EffectiveSymbol()
	}
}
