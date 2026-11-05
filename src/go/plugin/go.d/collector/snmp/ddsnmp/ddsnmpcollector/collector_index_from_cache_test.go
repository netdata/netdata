// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

const (
	indexFromSourceRoot = "1.3.6.1.4.1.99999.880.1"
	indexFromFirstRoot  = "1.3.6.1.4.1.99999.880.2.1"
	indexFromSecondRoot = "1.3.6.1.4.1.99999.880.3.1"
	indexFromFinalRoot  = "1.3.6.1.4.1.99999.880.4"
)

func indexFromCacheTags() []ddprofiledefinition.MetricTagConfig {
	tag := ddprofiledefinition.MetricTagConfig{
		Tag:   "interface",
		Table: "interfaceTable",
		Symbol: ddprofiledefinition.SymbolConfigCompat{
			OID:  indexFromFinalRoot + ".1",
			Name: "ifName",
		},
		IndexFrom: []ddprofiledefinition.MetricTagIndexSource{
			{
				Table: "physicalTable",
				Symbol: ddprofiledefinition.SymbolConfig{
					OID:  indexFromFirstRoot,
					Name: "physicalIndex",
				},
			},
			{
				Table: "aliasTable",
				Symbol: ddprofiledefinition.SymbolConfig{
					OID:  indexFromSecondRoot,
					Name: "interfaceIndex",
				},
			},
		},
	}
	alias := tag.Clone()
	alias.Tag = "_if_alias"
	alias.Symbol.OID = indexFromFinalRoot + ".2"
	alias.Symbol.Name = "ifAlias"
	return []ddprofiledefinition.MetricTagConfig{tag, alias}
}

func indexFromCacheProfile() *ddsnmp.Profile {
	return createTestProfile("index-from-cache.yaml", []ddprofiledefinition.MetricsConfig{
		{
			Table: ddprofiledefinition.SymbolConfig{
				OID:  indexFromSourceRoot,
				Name: "sourceTable",
			},
			Symbols:    []ddprofiledefinition.SymbolConfig{{OID: indexFromSourceRoot + ".1", Name: "sourceValue"}},
			MetricTags: indexFromCacheTags(),
		},
		{Table: ddprofiledefinition.SymbolConfig{
			OID:  indexFromFirstRoot,
			Name: "physicalTable",
		}},
		{Table: ddprofiledefinition.SymbolConfig{
			OID:  indexFromSecondRoot,
			Name: "aliasTable",
		}},
		{Table: ddprofiledefinition.SymbolConfig{
			OID:  indexFromFinalRoot,
			Name: "interfaceTable",
		}},
	})
}

func indexFromCachePDUs(value uint, name string) map[string][]gosnmp.SnmpPDU {
	return map[string][]gosnmp.SnmpPDU{
		indexFromSourceRoot: {createGauge32PDU(indexFromSourceRoot+".1.1", value)},
		indexFromFirstRoot:  {createGauge32PDU(indexFromFirstRoot+".1", 10)},
		indexFromSecondRoot: {createGauge32PDU(indexFromSecondRoot+".10", 20)},
		indexFromFinalRoot: {
			createStringPDU(indexFromFinalRoot+".1.20", name),
			createStringPDU(indexFromFinalRoot+".2.20", "alias-"+name),
		},
	}
}

// Only time metadata changes: row instances, tags and dependency edges are all
// produced by earlier collections through the SNMP adapter.
func expireIndexFromHop(t *testing.T, cache *tableCache, root string) {
	t.Helper()
	cache.mu.Lock()
	defer cache.mu.Unlock()
	_, ok := cache.timestamps[root]
	require.True(t, ok)
	cache.timestamps[root] = time.Now().Add(-2 * time.Hour)
}

func TestTableCollector_IndexFromHopRefresh(t *testing.T) {
	for name, tc := range map[string]struct {
		hop     string
		outcome string
	}{
		"first hop changed":  {hop: indexFromFirstRoot, outcome: "changed"},
		"second hop changed": {hop: indexFromSecondRoot, outcome: "changed"},
		"first hop missing":  {hop: indexFromFirstRoot, outcome: "missing"},
		"second hop missing": {hop: indexFromSecondRoot, outcome: "missing"},
		"first hop failed":   {hop: indexFromFirstRoot, outcome: "failed"},
		"second hop failed":  {hop: indexFromSecondRoot, outcome: "failed"},
	} {
		t.Run(name, func(t *testing.T) {
			ctrl, handler := setupMockHandler(t)
			defer ctrl.Finish()
			profile := indexFromCacheProfile()
			cache := newTableCache(time.Hour, 0)
			collector := newTableCollector(handler, make(map[string]bool), cache, logger.New(), false)

			for root, pdus := range indexFromCachePDUs(1, "old") {
				expectSNMPWalk(handler, gosnmp.Version2c, root, pdus)
			}
			var initial ddsnmp.CollectionStats
			metrics, err := collector.collect(profile, &initial)
			require.NoError(t, err)
			require.Len(t, metrics, 1)
			assert.Equal(t, map[string]string{"interface": "old", "_if_alias": "alias-old"}, metrics[0].Tags)
			require.True(t, cache.isConfigCached(profile.Definition.Metrics[0]))

			expireIndexFromHop(t, cache, tc.hop)
			pdus := indexFromCachePDUs(2, "new")
			if tc.outcome == "changed" {
				if tc.hop == indexFromFirstRoot {
					pdus[indexFromFirstRoot] = []gosnmp.SnmpPDU{createGauge32PDU(indexFromFirstRoot+".1", 11)}
					pdus[indexFromSecondRoot] = []gosnmp.SnmpPDU{createGauge32PDU(indexFromSecondRoot+".11", 21)}
				} else {
					pdus[indexFromSecondRoot] = []gosnmp.SnmpPDU{createGauge32PDU(indexFromSecondRoot+".10", 21)}
				}
				pdus[indexFromFinalRoot] = []gosnmp.SnmpPDU{
					createStringPDU(indexFromFinalRoot+".1.21", "new"),
					createStringPDU(indexFromFinalRoot+".2.21", "alias-new"),
				}
			}
			for root, rows := range pdus {
				if root == tc.hop && tc.outcome == "failed" {
					expectSNMPWalkError(handler, gosnmp.Version2c, root, errors.New("hop timeout"))
				} else if root == tc.hop && tc.outcome == "missing" {
					expectSNMPWalk(handler, gosnmp.Version2c, root, nil)
				} else {
					expectSNMPWalk(handler, gosnmp.Version2c, root, rows)
				}
			}
			var refreshed ddsnmp.CollectionStats
			metrics, err = collector.collect(profile, &refreshed)
			require.NoError(t, err)
			require.Len(t, metrics, 1)
			assert.EqualValues(t, 2, metrics[0].Value)
			if tc.outcome == "changed" {
				assert.Equal(t, map[string]string{"interface": "new", "_if_alias": "alias-new"}, metrics[0].Tags)
			} else {
				assert.Empty(t, metrics[0].Tags)
			}
			assert.EqualValues(t, 4, refreshed.SNMP.WalkRequests, "both tags share all hop and final walks")
			assert.Zero(t, refreshed.SNMP.GetRequests, "hop expiry settles the source before cached GET")
			assert.Zero(t, refreshed.TableCache.Hits)
			if tc.outcome == "failed" {
				assert.EqualValues(t, 1, refreshed.Errors.SNMP)
				assert.False(t, cache.isConfigCached(profile.Definition.Metrics[0]))
			}

			// Expiring the recovered hop advances the whole dependency group;
			// successful empty responses must not retain the former label either.
			if tc.outcome != "failed" {
				expireIndexFromHop(t, cache, tc.hop)
			}
			for root, rows := range indexFromCachePDUs(3, "recovered") {
				expectSNMPWalk(handler, gosnmp.Version2c, root, rows)
			}
			var recovered ddsnmp.CollectionStats
			metrics, err = collector.collect(profile, &recovered)
			require.NoError(t, err)
			require.Len(t, metrics, 1)
			assert.EqualValues(t, 3, metrics[0].Value)
			assert.Equal(
				t,
				map[string]string{"interface": "recovered", "_if_alias": "alias-recovered"},
				metrics[0].Tags,
			)
			assert.EqualValues(t, 4, recovered.SNMP.WalkRequests)
			require.True(t, cache.isConfigCached(profile.Definition.Metrics[0]))
		})
	}
}

func TestTableCollector_IndexFromCachedRoutesReuseTags(t *testing.T) {
	ctrl, handler := setupMockHandler(t)
	defer ctrl.Finish()
	profile := indexFromCacheProfile()
	cache := newTableCache(time.Hour, 0)
	collector := newTableCollector(handler, make(map[string]bool), cache, logger.New(), false)
	for root, pdus := range indexFromCachePDUs(1, "port-20") {
		expectSNMPWalk(handler, gosnmp.Version2c, root, pdus)
	}
	var initial ddsnmp.CollectionStats
	metrics, err := collector.collect(profile, &initial)
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.EqualValues(t, 4, initial.SNMP.WalkRequests)

	expectSNMPGet(handler, []string{indexFromSourceRoot + ".1.1"}, []gosnmp.SnmpPDU{
		createGauge32PDU(indexFromSourceRoot+".1.1", 2),
	})
	var cached ddsnmp.CollectionStats
	metrics, err = collector.collect(profile, &cached)
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.EqualValues(t, 2, metrics[0].Value)
	assert.Equal(t, map[string]string{"interface": "port-20", "_if_alias": "alias-port-20"}, metrics[0].Tags)
	assert.EqualValues(t, 1, cached.SNMP.GetRequests)
	assert.Zero(t, cached.SNMP.WalkRequests)
	assert.EqualValues(t, 4, cached.TableCache.Hits)
}

func TestTableCollector_IndexFromHopRefreshInvalidatesOmittedConsumer(t *testing.T) {
	for name, root := range map[string]string{"first hop": indexFromFirstRoot, "second hop": indexFromSecondRoot, "final table": indexFromFinalRoot} {
		t.Run(name, func(t *testing.T) {
			ctrl, handler := setupMockHandler(t)
			defer ctrl.Finish()
			profile := indexFromCacheProfile()
			cache := newTableCache(time.Hour, 0)
			collector := newTableCollector(handler, make(map[string]bool), cache, logger.New(), false)
			for oid, pdus := range indexFromCachePDUs(1, "old") {
				expectSNMPWalk(handler, gosnmp.Version2c, oid, pdus)
			}
			var initial ddsnmp.CollectionStats
			_, err := collector.collect(profile, &initial)
			require.NoError(t, err)
			require.True(t, cache.isConfigCached(profile.Definition.Metrics[0]))

			var selected ddprofiledefinition.MetricsConfig
			for _, cfg := range profile.Definition.Metrics[1:] {
				if cfg.Table.OID == root {
					selected = cfg
				}
			}
			hopOnly := createTestProfile("hop-only.yaml", []ddprofiledefinition.MetricsConfig{selected})
			expireIndexFromHop(t, cache, root)
			expectSNMPWalk(handler, gosnmp.Version2c, root, indexFromCachePDUs(2, "new")[root])
			var refreshed ddsnmp.CollectionStats
			_, err = collector.collect(hopOnly, &refreshed)
			require.NoError(t, err)
			assert.False(
				t,
				cache.isConfigCached(profile.Definition.Metrics[0]),
				"omitted source cannot retain tags based on an older hop",
			)
			assert.EqualValues(t, 1, refreshed.SNMP.WalkRequests)

			for oid, pdus := range indexFromCachePDUs(3, "new") {
				expectSNMPWalk(handler, gosnmp.Version2c, oid, pdus)
			}
			var resumed ddsnmp.CollectionStats
			metrics, err := collector.collect(profile, &resumed)
			require.NoError(t, err)
			require.Len(t, metrics, 1)
			assert.EqualValues(t, 3, metrics[0].Value)
			assert.Equal(t, map[string]string{"interface": "new", "_if_alias": "alias-new"}, metrics[0].Tags)
			assert.EqualValues(t, 4, resumed.SNMP.WalkRequests)
		})
	}
}

func TestCollector_IndexFromTypedTableTags(t *testing.T) {
	for _, consumer := range []string{"topology", "licensing", "bgp"} {
		t.Run(consumer, func(t *testing.T) {
			ctrl, handler := setupMockHandler(t)
			defer ctrl.Finish()
			profile := &ddsnmp.Profile{
				SourceFile: consumer + "-index-from.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{},
			}
			table := ddprofiledefinition.SymbolConfig{
				OID:  indexFromSourceRoot,
				Name: "sourceTable",
			}
			symbol := ddprofiledefinition.SymbolConfig{
				OID:  indexFromSourceRoot + ".1",
				Name: "sourceValue",
			}
			switch consumer {
			case "topology":
				profile.Definition.Topology = []ddprofiledefinition.TopologyConfig{{
					Kind: ddprofiledefinition.KindIfName,
					MetricsConfig: ddprofiledefinition.MetricsConfig{
						Table:      table,
						Symbols:    []ddprofiledefinition.SymbolConfig{symbol},
						MetricTags: indexFromCacheTags(),
					},
				}}
			case "licensing":
				profile.Definition.Licensing = []ddprofiledefinition.LicensingConfig{{
					ID:    "licenses",
					Table: table,
					Identity: ddprofiledefinition.LicenseIdentityConfig{
						ID: ddprofiledefinition.LicenseValueConfig{
							Index: 1,
						},
					},
					State: ddprofiledefinition.LicenseStateConfig{
						LicenseValueConfig: ddprofiledefinition.LicenseValueConfig{
							Symbol: symbol,
						},
					},
					MetricTags: indexFromCacheTags(),
				}}
			case "bgp":
				symbol.Mapping = ddprofiledefinition.NewExactMapping(map[string]string{
					"1": "idle", "2": "connect", "3": "active", "4": "opensent", "5": "openconfirm", "6": "established",
				})
				profile.Definition.BGP = []ddprofiledefinition.BGPConfig{{
					ID:    "peers",
					Kind:  ddprofiledefinition.BGPRowKindPeer,
					Table: table,
					Identity: ddprofiledefinition.BGPIdentityConfig{
						Neighbor: ddprofiledefinition.BGPValueConfig{
							Index: 1,
						},
						RemoteAS: ddprofiledefinition.BGPValueConfig{
							Value: "65001",
						},
					},
					State: ddprofiledefinition.BGPStateConfig{
						BGPValueConfig: ddprofiledefinition.BGPValueConfig{
							Symbol: symbol,
						},
					},
					MetricTags: indexFromCacheTags(),
				}}
			}
			require.NoError(t, ddprofiledefinition.ValidateEnrichProfile(profile.Definition))
			ddsnmp.HandleCrossTableTagsWithoutMetrics(profile)
			collector := New(Config{
				SnmpClient: handler,
				Profiles:   []*ddsnmp.Profile{profile},
				Log:        logger.New(),
			})
			for phase, label := range []string{"initial", "", "recovered"} {
				pdus := indexFromCachePDUs(6, label)
				pdus[indexFromSourceRoot] = append(
					pdus[indexFromSourceRoot],
					createGauge32PDU(indexFromSourceRoot+".1.2", 6),
				)
				pdus[indexFromFirstRoot] = append(
					pdus[indexFromFirstRoot],
					createGauge32PDU(indexFromFirstRoot+".2", 10),
				)
				if label == "" {
					pdus[indexFromSecondRoot] = nil
				}
				for root, rows := range pdus {
					expectSNMPWalk(handler, gosnmp.Version2c, root, rows)
				}
				results, err := collector.Collect()
				require.NoError(t, err)
				require.Len(t, results, 1)
				pm := results[0]
				var tags []map[string]string
				switch consumer {
				case "topology":
					for _, metric := range pm.TopologyMetrics {
						tags = append(tags, metric.Tags)
					}
				case "licensing":
					for _, row := range pm.LicenseRows {
						tags = append(tags, row.Tags)
					}
				case "bgp":
					for _, row := range pm.BGPRows {
						tags = append(tags, row.Tags)
					}
				}
				require.Len(t, tags, 2, "phase %d keeps source rows when optional lookup tags are missing", phase)
				wantTags := map[string]string{}
				if label != "" {
					wantTags = map[string]string{"interface": label, "_if_alias": "alias-" + label}
				}
				for _, gotTags := range tags {
					if label == "" {
						assert.Empty(t, gotTags, "phase %d", phase)
					} else {
						assert.Equal(t, wantTags, gotTags, "phase %d", phase)
					}
				}
				assert.EqualValues(t, 4, pm.Stats.SNMP.WalkRequests, "rows and both tags reuse each source walk")
				assert.Zero(t, pm.Stats.SNMP.GetRequests)
			}

			expectSNMPWalk(handler, gosnmp.Version2c, indexFromSourceRoot, nil)
			results, err := collector.Collect()
			require.NoError(t, err)
			require.Len(t, results, 1)
			assert.Empty(t, results[0].TopologyMetrics)
			assert.Empty(t, results[0].LicenseRows)
			assert.Empty(t, results[0].BGPRows)
			assert.EqualValues(
				t,
				1,
				results[0].Stats.SNMP.WalkRequests,
				"unused chains do not activate dependency walks",
			)
		})
	}
}

func TestCollector_IndexFromTypedLookupTargetsDoNotCreateRows(t *testing.T) {
	const root = "1.3.6.1.4.1.99999.999"
	for _, consumer := range []string{"licensing", "bgp"} {
		t.Run(consumer, func(t *testing.T) {
			for name, tc := range map[string]struct{ missingLink, legacy bool }{
				"complete chain":          {},
				"missing link":            {missingLink: true},
				"legacy tag anchors rows": {legacy: true},
			} {
				t.Run(name, func(t *testing.T) {
					profile := &ddsnmp.Profile{
						SourceFile: "same-table-chain.yaml",
						Definition: &ddprofiledefinition.ProfileDefinition{},
					}
					table := ddprofiledefinition.SymbolConfig{
						OID:  root,
						Name: "source",
					}
					anchor := ddprofiledefinition.SymbolConfig{
						OID:  root + ".1",
						Name: "description",
					}
					tags := []ddprofiledefinition.MetricTagConfig{{
						Tag: "_label", Table: "source",
						Symbol: ddprofiledefinition.SymbolConfigCompat{
							OID:  root + ".3",
							Name: "label",
						},
						IndexFrom: []ddprofiledefinition.MetricTagIndexSource{{
							Table: "source", Symbol: ddprofiledefinition.SymbolConfig{
								OID:  root + ".2",
								Name: "key",
							},
						}},
					}}
					if tc.legacy {
						tags[0].IndexFrom = nil
					}
					switch consumer {
					case "licensing":
						profile.Definition.Licensing = []ddprofiledefinition.LicensingConfig{{
							ID: "licenses", Table: table, MetricTags: tags,
							Identity: ddprofiledefinition.LicenseIdentityConfig{
								ID: ddprofiledefinition.LicenseValueConfig{
									Index: 1,
								},
								Name: ddprofiledefinition.LicenseValueConfig{
									Symbol: anchor,
								},
							},
							State: ddprofiledefinition.LicenseStateConfig{
								LicenseValueConfig: ddprofiledefinition.LicenseValueConfig{
									Value: "0",
								},
							},
						}}
					case "bgp":
						profile.Definition.BGP = []ddprofiledefinition.BGPConfig{{
							ID: "peers", Kind: ddprofiledefinition.BGPRowKindPeer, Table: table, MetricTags: tags,
							Identity: ddprofiledefinition.BGPIdentityConfig{
								Neighbor: ddprofiledefinition.BGPValueConfig{
									Index: 1,
								},
								RemoteAS: ddprofiledefinition.BGPValueConfig{
									Value: "65001",
								},
							},
							Descriptors: ddprofiledefinition.BGPDescriptorsConfig{
								Description: ddprofiledefinition.BGPValueConfig{
									Symbol: anchor,
								},
							},
							State: ddprofiledefinition.BGPStateConfig{
								BGPValueConfig: ddprofiledefinition.BGPValueConfig{
									Value: "1",
									Mapping: ddprofiledefinition.NewExactMapping(map[string]string{
										"1": "idle", "2": "connect", "3": "active", "4": "opensent", "5": "openconfirm", "6": "established",
									}),
								},
							},
						}}
					}
					require.NoError(t, ddprofiledefinition.ValidateEnrichProfile(profile.Definition))
					handler := &sourceTestHandler{
						walk: func(oid string) ([]gosnmp.SnmpPDU, error) {
							assert.Equal(t, root, oid)
							pdus := []gosnmp.SnmpPDU{createStringPDU(root+".1.1", "source-row"), createStringPDU(root+".3.9", "target-label")}
							if !tc.missingLink {
								pdus = append(pdus, createIntegerPDU(root+".2.1", 9))
							}
							return pdus, nil
						},
					}
					collector := New(Config{
						SnmpClient: handler,
						Profiles:   []*ddsnmp.Profile{profile},
						Log:        logger.New(),
					})
					results, err := collector.Collect()
					require.NoError(t, err)
					require.Len(t, results, 1)
					rows := make(map[string]map[string]string)
					for _, row := range results[0].LicenseRows {
						rows[row.RowKey] = row.Tags
					}
					for _, row := range results[0].BGPRows {
						rows[row.RowKey] = row.Tags
					}
					wantTags := map[string]string{}
					if !tc.missingLink {
						wantTags["_label"] = "target-label"
					}
					wantRows := map[string]map[string]string{"1": wantTags}
					if tc.legacy {
						wantRows = map[string]map[string]string{"1": {}, "9": {"_label": "target-label"}}
					}
					assert.Equal(t, wantRows, rows)
					assert.Equal(t, 1, handler.walks)
				})
			}
		})
	}
}

// Typed row preparation is O(fields + tags), with no cost proportional to table
// rows or chain length. ns/op is a local trend; allocations are the regression gate.
func BenchmarkTypedRowConfig(b *testing.B) {
	for _, chained := range []bool{false, true} {
		tags := indexFromCacheTags()
		if !chained {
			for i := range tags {
				tags[i].IndexFrom = nil
			}
		}
		mode := "legacy"
		if chained {
			mode = "chain"
		}
		license := ddprofiledefinition.LicensingConfig{
			MetricTags: tags,
		}
		bgp := ddprofiledefinition.BGPConfig{
			MetricTags: tags,
		}
		for consumer, prepare := range map[string]func() ddprofiledefinition.MetricsConfig{
			"licensing": func() ddprofiledefinition.MetricsConfig { return licensingConfigAsMetricsConfig(license) },
			"bgp":       func() ddprofiledefinition.MetricsConfig { return bgpConfigAsMetricsConfig(bgp) },
		} {
			b.Run(consumer+"/"+mode, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					prepare()
				}
			})
		}
	}
}
