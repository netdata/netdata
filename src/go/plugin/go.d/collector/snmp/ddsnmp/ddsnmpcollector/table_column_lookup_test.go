// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestCrossTableResolver_IndexFrom(t *testing.T) {
	for name, tc := range map[string]struct {
		change      func(*ddprofiledefinition.MetricTagConfig, *crossTableContext)
		oid, reason string
	}{
		"complete chain": {},
		"first instance absent": {change: func(_ *ddprofiledefinition.MetricTagConfig, ctx *crossTableContext) {
			delete(ctx.walkedData["1.2"], "1.2.1.1.2")
		}, oid: "1.2.1.1.2", reason: "missing_dependency"},
		"second instance absent": {change: func(_ *ddprofiledefinition.MetricTagConfig, ctx *crossTableContext) {
			delete(ctx.walkedData["1.3"], "1.3.1.4.0")
		}, oid: "1.3.1.4.0", reason: "missing_dependency"},
		"hop route absent": {change: func(_ *ddprofiledefinition.MetricTagConfig, ctx *crossTableContext) {
			delete(ctx.tableNameToOID, "second")
		}, oid: "1.3.1", reason: "dependency_processing"},
		"hop data absent": {change: func(_ *ddprofiledefinition.MetricTagConfig, ctx *crossTableContext) {
			delete(ctx.walkedData, "1.3")
		}, oid: "1.3.1", reason: "dependency_processing"},
		"conversion fails": {change: func(_ *ddprofiledefinition.MetricTagConfig, ctx *crossTableContext) {
			ctx.walkedData["1.2"]["1.2.1.1.2"] = createIntegerPDU("1.2.1.1.2", 10)
		}, oid: "1.2.1.1.2", reason: "conversion"},
		"extraction does not match": {change: func(tag *ddprofiledefinition.MetricTagConfig, _ *crossTableContext) {
			tag.IndexFrom[0].Symbol.ExtractValue = "^(ff)$"
		}, oid: "1.2.1.1.2", reason: "extract_mismatch"},
		"empty capture": {change: func(tag *ddprofiledefinition.MetricTagConfig, _ *crossTableContext) {
			tag.IndexFrom[0].Symbol.ExtractValue = "^(z*)"
		}, oid: "1.2.1.1.2", reason: "extract_mismatch"},
		"match does not match": {change: func(tag *ddprofiledefinition.MetricTagConfig, _ *crossTableContext) {
			tag.IndexFrom[0].Symbol.MatchPattern = "^z$"
		}, oid: "1.2.1.1.2", reason: "pattern_mismatch"},
		"mapping produces empty key": {change: func(tag *ddprofiledefinition.MetricTagConfig, _ *crossTableContext) {
			tag.IndexFrom[0].Symbol.Mapping.Items["a.0"] = ""
		}, oid: "1.2.1.1.2", reason: "dependency_processing"},
		"final instance absent": {change: func(_ *ddprofiledefinition.MetricTagConfig, ctx *crossTableContext) {
			delete(ctx.walkedData["1.4"], "1.4.1.9")
		}, oid: "1.4.1.9", reason: "missing_dependency"},
		"final reverse lookup": {change: func(tag *ddprofiledefinition.MetricTagConfig, ctx *crossTableContext) {
			tag.LookupSymbol = ddprofiledefinition.SymbolConfigCompat{
				OID:  "1.4.2",
				Name: "targetKey",
			}
			delete(ctx.walkedData["1.4"], "1.4.1.9")
			ctx.walkedData["1.4"]["1.4.2.99"] = createIntegerPDU("1.4.2.99", 9)
			ctx.walkedData["1.4"]["1.4.1.99"] = createStringPDU("1.4.1.99", "expected")
		}},
	} {
		t.Run(name, func(t *testing.T) {
			tag := ddprofiledefinition.MetricTagConfig{
				Tag:   "_label",
				Table: "final",
				Symbol: ddprofiledefinition.SymbolConfigCompat{
					OID:  "1.4.1",
					Name: "label",
				},
				IndexTransform: []ddprofiledefinition.MetricIndexTransform{{Start: 1}},
				IndexFrom: []ddprofiledefinition.MetricTagIndexSource{
					{Table: "first", Symbol: ddprofiledefinition.SymbolConfig{
						OID:          "1.2.1",
						Name:         "firstKey",
						Format:       "hex",
						ExtractValue: "^0([a-f])$",
						MatchPattern: "^([a-f])$",
						MatchValue:   "$1.0",
						Mapping: ddprofiledefinition.MappingConfig{
							Items: map[string]string{"a.0": "4.0"},
						},
					}},
					{Table: "second", Symbol: ddprofiledefinition.SymbolConfig{
						OID:  "1.3.1",
						Name: "secondKey",
					}},
				},
			}
			ctx := newCrossTableContext(map[string]map[string]gosnmp.SnmpPDU{
				"1.2": {"1.2.1.1.2": {Name: "1.2.1.1.2", Type: gosnmp.OctetString, Value: []byte{0x0a}}},
				"1.3": {"1.3.1.4.0": createIntegerPDU("1.3.1.4.0", 9)},
				"1.4": {"1.4.1.9": createStringPDU("1.4.1.9", "expected")},
			}, map[string]string{"first": "1.2", "second": "1.3", "final": "1.4"})
			ctx.rowTags = make(map[string]string)
			if tc.change != nil {
				tc.change(&tag, ctx)
			}
			profile := &ddprofiledefinition.ProfileDefinition{
				Metrics: []ddprofiledefinition.MetricsConfig{{
					Table: ddprofiledefinition.SymbolConfig{
						OID:  "1.1",
						Name: "source",
					},
					Symbols:    []ddprofiledefinition.SymbolConfig{{OID: "1.1.1", Name: "value"}},
					MetricTags: []ddprofiledefinition.MetricTagConfig{tag},
				}},
			}
			require.NoError(t, ddprofiledefinition.ValidateEnrichProfile(profile))
			tag = profile.Metrics[0].MetricTags[0]
			var report AcquisitionRouteReport
			err := newCrossTableResolver(
				logger.New(),
			).resolveCrossTableTag(tag, "7.1.2", ctx, processingFor(&report, "7.1.2"))
			if tc.reason == "" {
				require.NoError(t, err)
				assert.Equal(t, map[string]string{"_label": "expected"}, ctx.rowTags)
				assert.Empty(t, report.Processing)
			} else {
				require.Error(t, err)
				assert.Empty(t, ctx.rowTags)
				assert.Equal(t, []ddsnmp.ProcessingEvent{{RowIndex: "7.1.2", Field: "_label", OID: tc.oid, Reason: tc.reason}}, report.Processing)
				require.NoError(t, ddsnmp.ValidateProcessingEvents(report.Processing))
			}
		})
	}
}

func TestIndexFrom_StrictExtractionLeavesLegacyLookupUnchanged(t *testing.T) {
	r := newCrossTableResolver(logger.New())
	for _, pattern := range []string{"unmatched(.)", "^9$"} {
		sym := ddprofiledefinition.SymbolConfig{
			Name:                 "source",
			ExtractValueCompiled: regexp.MustCompile(pattern),
		}
		pdu := createIntegerPDU("1.2.1.1", 9)
		value, ok := r.processLookupSymbolValue(sym, pdu)
		require.True(t, ok)
		assert.Equal(t, "9", value, "legacy lookup extraction remains permissive")
		value, reason, err := r.processIndexSourceValue(sym, pdu)
		require.Error(t, err)
		assert.Empty(t, value)
		assert.Equal(t, "extract_mismatch", reason)
	}
}

func TestTableCollector_IndexFromReturnsToSourceTable(t *testing.T) {
	const root = "1.3.6.1.4.1.99999.80"
	profile := createTestProfile("same-table.yaml", []ddprofiledefinition.MetricsConfig{{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  root,
			Name: "source",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{{OID: root + ".1", Name: "value"}},
		MetricTags: []ddprofiledefinition.MetricTagConfig{{
			Tag: "_label", Table: "source", Symbol: ddprofiledefinition.SymbolConfigCompat{
				OID:  root + ".3",
				Name: "label",
			},
			IndexFrom: []ddprofiledefinition.MetricTagIndexSource{
				{Table: "source", Symbol: ddprofiledefinition.SymbolConfig{
					OID:  root + ".2",
					Name: "key",
				}},
			},
		}},
	}})
	require.NoError(t, ddprofiledefinition.ValidateEnrichProfile(profile.Definition))
	handler := &sourceTestHandler{
		walk: func(oid string) ([]gosnmp.SnmpPDU, error) {
			assert.Equal(t, root, oid)
			return []gosnmp.SnmpPDU{
				createIntegerPDU(root+".1.1", 7),
				createIntegerPDU(root+".2.1", 9),
				createStringPDU(root+".3.9", "remote row"),
			}, nil
		},
	}
	c := New(Config{
		SnmpClient: handler,
		Log:        logger.New(),
		Profiles:   []*ddsnmp.Profile{profile},
	})
	results, err := c.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Len(t, results[0].Metrics, 1)
	assert.Equal(t, int64(7), results[0].Metrics[0].Value)
	assert.Equal(t, map[string]string{"_label": "remote row"}, results[0].Metrics[0].Tags)
	assert.Equal(t, 1, handler.walks)
}

// Each hop is a map lookup, independent of unrelated rows in its table. Work and
// allocations grow with rows * (hops + 1); ns/op is a local trend, not a CI gate.
func BenchmarkCrossTableResolver_IndexFrom(b *testing.B) {
	for _, rows := range []int{32, 4096} {
		for _, hops := range []int{0, 2, 8} {
			b.Run(fmt.Sprintf("rows=%d/hops=%d", rows, hops), func(b *testing.B) {
				tag := ddprofiledefinition.MetricTagConfig{
					Tag:   "label",
					Table: "final",
					Symbol: ddprofiledefinition.SymbolConfigCompat{
						OID:  "1.2.1",
						Name: "label",
					},
				}
				ctx := newCrossTableContext(
					make(map[string]map[string]gosnmp.SnmpPDU),
					map[string]string{"final": "1.2"},
				)
				ctx.walkedData["1.2"] = make(map[string]gosnmp.SnmpPDU)
				indexes := make([]string, rows)
				for i := range rows {
					indexes[i] = strconv.Itoa(i + 1)
					ctx.walkedData["1.2"]["1.2.1."+indexes[i]] = createStringPDU("1.2.1."+indexes[i], "label")
				}
				for hop := range hops {
					table := fmt.Sprintf("hop%d", hop)
					root := fmt.Sprintf("1.3.%d", hop)
					ctx.tableNameToOID[table] = root
					pdus := make(map[string]gosnmp.SnmpPDU)
					for _, index := range indexes {
						pdus[root+".1."+index] = createStringPDU(root+".1."+index, index)
					}
					ctx.walkedData[root] = pdus
					tag.IndexFrom = append(
						tag.IndexFrom,
						ddprofiledefinition.MetricTagIndexSource{
							Table: table,
							Symbol: ddprofiledefinition.SymbolConfig{
								OID:  root + ".1",
								Name: table,
							},
						},
					)
				}
				r := newCrossTableResolver(logger.New())
				ctx.rowTags = make(map[string]string)
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					for _, index := range indexes {
						if err := r.resolveCrossTableTag(tag, index, ctx, nil); err != nil {
							b.Fatal(err)
						}
					}
				}
			})
		}
	}
}
