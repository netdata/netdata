// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"fmt"
	"math"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestProfileIndexTagSources(t *testing.T) {
	for name, tc := range map[string]struct {
		tag  string
		want map[string]string
	}{
		"bare index": {
			tag:  `index: 1`,
			want: map[string]string{"index1": "7"},
		},
		"symbol name": {
			tag:  `index: 1, symbol: {name: row}`,
			want: map[string]string{"row": "7"},
		},
		"tag name": {
			tag:  `index: 1, tag: row`,
			want: map[string]string{"row": "7"},
		},
		"index retains precedence over OID": {
			tag:  `index: 1, symbol: {OID: 1.2.3.9, name: row}`,
			want: map[string]string{"row": "7"},
		},
		"index retains precedence over table": {
			tag:  `index: 1, table: other, symbol: {name: row}`,
			want: map[string]string{"row": "7"},
		},
		"index mapping": {
			tag:  `index: 1, tag: row, mapping: {7: seven}`,
			want: map[string]string{"row": "seven"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			def := decodeContractProfile(
				t,
				`metrics: [{table: {OID: 1.2.3, name: example}, symbols: [{OID: 1.2.3.1, name: value}], metric_tags: [{`+tc.tag+`}]}]`,
			)
			cfg := def.Metrics[0]
			row := &tableRowData{
				index:     "7.8",
				tableName: "example",
				tags:      map[string]string{},
				pdus:      map[string]gosnmp.SnmpPDU{},
			}
			p := newTableRowProcessor(logger.New())
			ordered := buildOrderedTags(cfg)
			require.Len(t, ordered, 1)
			require.Equal(t, tagTypeIndex, ordered[0].tagType)
			require.NoError(t, p.processSingleIndexTag(row, ordered[0].config))
			assert.Equal(t, tc.want, row.tags)
		})
	}
}

func TestProfileTypedSourcePrecedence(t *testing.T) {
	for name, tc := range map[string]struct{ fields, oid string }{
		"from": {
			fields: `from: 1.2.3.0`,
			oid:    "1.2.3.0",
		},
		"symbol": {
			fields: `symbol: {OID: 1.2.3.0, name: state}`,
			oid:    "1.2.3.0",
		},
		"legacy": {
			fields: `OID: 1.2.3.0, name: state`,
			oid:    "1.2.3.0",
		},
		"symbol over from": {
			fields: `symbol: {OID: 1.2.3.0, name: state}, from: 9.9.0`,
			oid:    "1.2.3.0",
		},
		"from over legacy": {
			fields: `from: 1.2.3.0, OID: 9.9.0, name: state`,
			oid:    "1.2.3.0",
		},
	} {
		t.Run(name, func(t *testing.T) {
			for kind, input := range map[string]string{
				"licensing": `licensing: [{state: {` + tc.fields + `}}]`,
				"bgp":       `bgp: [{kind: peer, identity: {neighbor: {value: "192.0.2.1"}, remote_as: {value: "65001"}}, state: {mapping: {1: idle, 2: connect, 3: active, 4: opensent, 5: openconfirm, 6: established}, ` + tc.fields + `}}]`,
			} {
				t.Run(kind, func(t *testing.T) {
					var def ddprofiledefinition.ProfileDefinition
					require.NoError(t, yaml.Unmarshal([]byte(input), &def))
					for _, validate := range []bool{false, true} {
						if validate {
							require.NoError(t, ddprofiledefinition.ValidateEnrichProfile(&def))
						}
						type observation struct{ source, collected, identity string }
						var got observation
						if kind == "licensing" {
							cfg := def.Licensing[0]
							got = observation{
								cfg.State.LicenseValueConfig.SourceOID(),
								cfg.State.LicenseValueConfig.EffectiveSymbol().OID,
								ddprofiledefinition.LicenseMergeIdentity(cfg),
							}
						} else {
							cfg := def.BGP[0]
							got = observation{cfg.State.BGPValueConfig.SourceOID(), cfg.State.BGPValueConfig.EffectiveSymbol().OID, ddprofiledefinition.BGPMergeIdentity(cfg)}
						}
						identity := "scalar|" + tc.oid
						if kind == "bgp" {
							identity = "scalar|peer|" + tc.oid
						}
						assert.Equal(t, observation{tc.oid, tc.oid, identity}, got, "validated=%v", validate)
					}
				})
			}
		})
	}
}

func TestProfileBitmaskValues(t *testing.T) {
	for name, tc := range map[string]struct {
		value     any
		pduType   gosnmp.Asn1BER
		format    string
		raw       int64
		dims      map[string]int64
		wantError bool
	}{
		"zero": {
			value:     uint64(0),
			pduType:   gosnmp.Counter64,
			format:    "",
			raw:       0,
			dims:      map[string]int64{"zero": 1, "low": 0, "high": 0},
			wantError: false,
		},
		"numeric high bit": {
			value:     uint64(1) << 63,
			pduType:   gosnmp.Counter64,
			format:    "",
			raw:       math.MinInt64,
			dims:      map[string]int64{"zero": 0, "low": 0, "high": 1},
			wantError: false,
		},
		"numeric all bits": {
			value:     uint64(math.MaxUint64),
			pduType:   gosnmp.Counter64,
			format:    "",
			raw:       -1,
			dims:      map[string]int64{"zero": 0, "low": 1, "high": 1},
			wantError: false,
		},
		"decimal high and low": {
			value:     []byte("9223372036854775809"),
			pduType:   gosnmp.OctetString,
			format:    "",
			raw:       math.MinInt64 + 1,
			dims:      map[string]int64{"zero": 0, "low": 1, "high": 1},
			wantError: false,
		},
		"hex high and low": {
			value:     []byte{0x80, 0, 0, 0, 0, 0, 0, 1},
			pduType:   gosnmp.OctetString,
			format:    "hex",
			raw:       math.MinInt64 + 1,
			dims:      map[string]int64{"zero": 0, "low": 1, "high": 1},
			wantError: false,
		},
		"decimal overflow rejected": {
			value:     []byte("18446744073709551616"),
			pduType:   gosnmp.OctetString,
			format:    "",
			raw:       0,
			dims:      nil,
			wantError: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			def := decodeContractProfile(
				t,
				fmt.Sprintf(
					`metrics: [{symbol: {OID: 1.2.3.0, name: flags, format: %q, mapping: {mode: bitmask, items: {0: zero, 1: low, 9223372036854775808: high}}}}]`,
					tc.format,
				),
			)
			sym := def.Metrics[0].Symbol
			pdu := gosnmp.SnmpPDU{
				Name:  sym.OID,
				Type:  tc.pduType,
				Value: tc.value,
			}
			value, err := newValueProcessor().processValue(sym, pdu)
			if tc.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			got, err := buildScalarMetric(sym, pdu, value, nil, nil)
			require.NoError(t, err)
			assert.Equal(t, &ddsnmp.Metric{
				Name:       "flags",
				Value:      tc.raw,
				MultiValue: tc.dims,
				MetricType: getMetricTypeFromPDUType(pdu),
			}, got)
		})
	}
}

func TestProfileVirtualDimensionsCollectFromMatchingSamples(t *testing.T) {
	for name, fields := range map[string][]string{"forward": {"first", "second"}, "reverse": {"second", "first"}} {
		t.Run(name, func(t *testing.T) {
			def := decodeContractProfile(t, fmt.Sprintf(`metrics:
 - table: {OID: 1.2.3, name: example}
   symbols:
    - {OID: 1.2.3.1, name: _shared, mapping: {1: %s}}
    - {OID: 1.2.3.2, name: _shared, mapping: {1: %s}}
   metric_tags: [{index: 1}]
virtual_metrics:
 - name: result
   per_row: true
   sources: [{metric: _shared, table: example, dim: first}]
`, fields[0], fields[1]))
			cfg := def.Metrics[0]
			row := &tableRowData{
				index:     "1",
				tableName: "example",
				pdus: map[string]gosnmp.SnmpPDU{
					"1.2.3.1": {Name: "1.2.3.1.1", Type: gosnmp.Integer, Value: 1},
					"1.2.3.2": {Name: "1.2.3.2.1", Type: gosnmp.Integer, Value: 1},
				},
			}
			metrics, err := newTableRowProcessor(logger.New()).processRowMetrics(row, &tableRowProcessingContext{
				config:     cfg,
				columnOIDs: buildColumnOIDs(cfg),
			})
			require.NoError(t, err)
			var values []int64
			for _, metric := range metrics {
				if value, _, ok := vmResolveSourceValue(metric, vmetricsSink{
					sourceDim: "first",
				}); ok {
					values = append(values, value)
				}
			}
			assert.Equal(t, []int64{1}, values)
		})
	}
}

func BenchmarkProfileBitmaskDecoding(b *testing.B) {
	sym := ddprofiledefinition.SymbolConfig{
		Mapping: ddprofiledefinition.MappingConfig{
			Mode:  ddprofiledefinition.MappingModeBitmask,
			Items: map[string]string{"0": "zero", "1": "low", "128": "high"},
		},
	}
	pdu := gosnmp.SnmpPDU{
		Type:  gosnmp.OctetString,
		Value: []byte("129"),
	}
	p := newValueProcessor()
	b.ReportAllocs()
	for b.Loop() {
		v, err := p.processValue(sym, pdu)
		if err != nil {
			b.Fatal(err)
		}
		buildMultiValue(v, sym.Mapping)
	}
}
