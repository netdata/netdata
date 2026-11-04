// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestCollector_Collect_StatsSnapshot(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	// --- SNMP expectations ---------------------------------------------------

	// Scalar: sysUpTime.0
	expectSNMPGet(mockHandler,
		[]string{"1.3.6.1.2.1.1.3.0"},
		[]gosnmp.SnmpPDU{
			createTimeTicksPDU("1.3.6.1.2.1.1.3.0", 123456),
		},
	)

	// Table: ifTable, we only care about ifInOctets with 2 rows
	expectSNMPWalk(mockHandler,
		gosnmp.Version2c,
		"1.3.6.1.2.1.2.2",
		[]gosnmp.SnmpPDU{
			// Row 1
			createCounter32PDU("1.3.6.1.2.1.2.2.1.10.1", 1000), // ifInOctets.1
			// Row 2
			createCounter32PDU("1.3.6.1.2.1.2.2.1.10.2", 2000), // ifInOctets.2
		},
	)

	// --- Profile definition --------------------------------------------------

	profile := &ddsnmp.Profile{
		SourceFile: "stats-toy-profile.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{
			Metrics: []ddprofiledefinition.MetricsConfig{
				// Simple scalar metric
				{
					Symbol: ddprofiledefinition.SymbolConfig{
						OID:  "1.3.6.1.2.1.1.3.0",
						Name: "sysUpTime",
					},
				},
				// Simple table metric: ifInOctets over ifTable
				{
					Table: ddprofiledefinition.SymbolConfig{
						OID:  "1.3.6.1.2.1.2.2",
						Name: "ifTable",
					},
					Symbols: []ddprofiledefinition.SymbolConfig{
						{
							OID:  "1.3.6.1.2.1.2.2.1.10",
							Name: "ifInOctets",
						},
					},
				},
			},
			// One virtual metric that sums ifInOctets across the table.
			VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
				{
					Name: "ifInOctets_total",
					Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
						{
							Metric: "ifInOctets",
							Table:  "ifTable",
						},
					},
				},
			},
		},
	}

	require.NoError(t, ddsnmp.CompileTransforms(profile))

	collector := New(Config{
		SnmpClient:  mockHandler,
		Profiles:    []*ddsnmp.Profile{profile},
		Log:         logger.New(),
		SysObjectID: "",
	})

	// --- Run collection ------------------------------------------------------

	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)

	pm := results[0]

	// --- Sanity check on actual metrics -------------------------------------

	// We expect:
	//   - 1 scalar metric (sysUpTime)
	//   - 2 table metrics (ifInOctets for 2 rows)
	//   - 1 virtual metric (ifInOctets_total)
	require.Len(t, pm.Metrics, 4, "total number of metrics")

	// --- Assert CollectionStats as a snapshot -------------------------------

	// Ignore timing (it's inherently variable).
	stats := pm.Stats
	stats.Timing = ddsnmp.TimingStats{}
	pm.Stats = stats

	expected := ddsnmp.CollectionStats{
		SNMP: ddsnmp.SNMPOperationStats{
			// Scalar: 1 GET with 1 OID
			GetRequests: 1,
			GetOIDs:     1,

			// Table: 1 WALK with 2 PDUs, 1 table walked, no cached tables
			WalkRequests: 1,
			WalkPDUs:     2,
			TablesWalked: 1,
			// TablesCached should be 0 on first run
		},
		Metrics: ddsnmp.MetricCountStats{
			Scalar:  1, // sysUpTime
			Table:   2, // ifInOctets.1, ifInOctets.2
			Virtual: 1, // ifInOctets_total
			Tables:  1, // ifTable
			Rows:    2, // 2 interfaces
		},
		TableCache: ddsnmp.TableCacheStats{
			Hits:   0, // first run → no cache hits
			Misses: 1, // one table config had to be walked
			// Expired intentionally ignored / omitted
		},
		Errors: ddsnmp.ErrorStats{
			SNMP:        0,
			MissingOIDs: 0,
		},
		// Timing left as zero-value for comparison
		Timing: ddsnmp.TimingStats{},
	}

	assert.Equal(t, expected, pm.Stats)
}

func TestCollector_Collect_SharesCanonicalFreshTableViewAcrossProfiles(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	dependencyConfig, sourceConfig := crossTableDependencyTestConfigs("1.3.6.1.4.1.99999")
	sourceTableOID := sourceConfig.Table.OID
	sourceMetricOID := sourceConfig.Symbols[0].OID + ".1"
	dependencyTableOID := dependencyConfig.Table.OID
	dependencyMetricOID := dependencyConfig.Symbols[0].OID + ".1"
	dependencyProfile := &ddsnmp.Profile{
		SourceFile: "dependency-owner.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{Topology: []ddprofiledefinition.TopologyConfig{{
			Kind:          ddprofiledefinition.KindIfName,
			MetricsConfig: dependencyConfig,
		}}},
	}
	sourceProfile := &ddsnmp.Profile{
		SourceFile: "source-owner.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{Topology: []ddprofiledefinition.TopologyConfig{{
			Kind:          ddprofiledefinition.KindArpEntry,
			MetricsConfig: sourceConfig,
		}}},
	}
	ddsnmp.HandleCrossTableTagsWithoutMetrics(sourceProfile)

	expectSNMPWalk(mockHandler, gosnmp.Version2c, sourceTableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(sourceMetricOID, 10),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, dependencyTableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(dependencyMetricOID, 20),
	})

	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{dependencyProfile, sourceProfile},
		Log:        logger.New(),
	})
	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 2)

	var dependencyResult, sourceResult *ddsnmp.ProfileMetrics
	for _, result := range results {
		switch result.Source {
		case dependencyProfile.SourceFile:
			dependencyResult = result
		case sourceProfile.SourceFile:
			sourceResult = result
		}
	}
	require.NotNil(t, dependencyResult)
	require.NotNil(t, sourceResult)
	require.Len(t, sourceResult.TopologyMetrics, 1)
	sourceMetric := &sourceResult.TopologyMetrics[0]
	assert.EqualValues(t, 0, sourceMetric.Value)
	assert.Equal(t, map[string]string{"dependency_value": "20"}, sourceMetric.Tags)
	assert.Equal(t, ddsnmp.KindArpEntry, sourceMetric.TopologyKind)
	assert.Equal(t, int64(1), dependencyResult.Stats.SNMP.WalkRequests)
	assert.Equal(t, int64(1), dependencyResult.Stats.SNMP.TablesWalked)
	assert.Zero(t, dependencyResult.Stats.TableCache.Misses)
	assert.Equal(t, int64(1), sourceResult.Stats.SNMP.WalkRequests)
	assert.Equal(t, int64(1), sourceResult.Stats.SNMP.TablesWalked)
	assert.Zero(t, sourceResult.Stats.TableCache.Misses)

	var auxiliaryConfig *ddprofiledefinition.MetricsConfig
	for i := range sourceProfile.Definition.Topology {
		cfg := &sourceProfile.Definition.Topology[i].MetricsConfig
		if cfg.Table.Name == "dependencyTable" && len(cfg.Symbols) == 0 {
			auxiliaryConfig = cfg
			break
		}
	}
	require.NotNil(t, auxiliaryConfig)
	assert.False(t, collector.tableCache.isConfigCached(dependencyConfig))
	assert.False(t, collector.tableCache.isConfigCached(sourceConfig))
	assert.False(t, collector.tableCache.isConfigCached(*auxiliaryConfig))

	expectSNMPWalk(mockHandler, gosnmp.Version2c, sourceTableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(sourceMetricOID, 12),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, dependencyTableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(dependencyMetricOID, 30),
	})

	results, err = collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 2)
	var refreshedSourceResult *ddsnmp.ProfileMetrics
	for _, result := range results {
		if result.Source == sourceProfile.SourceFile {
			refreshedSourceResult = result
			break
		}
	}
	require.NotNil(t, refreshedSourceResult)
	require.Len(t, refreshedSourceResult.TopologyMetrics, 1)
	assert.Equal(t, map[string]string{"dependency_value": "30"}, refreshedSourceResult.TopologyMetrics[0].Tags)
	assert.Equal(t, int64(0), refreshedSourceResult.Stats.TableCache.Hits)
	assert.Equal(t, int64(0), refreshedSourceResult.Stats.TableCache.Misses)
	assert.Equal(t, int64(0), refreshedSourceResult.Stats.SNMP.GetRequests)
	assert.Equal(t, int64(1), refreshedSourceResult.Stats.SNMP.WalkRequests)
	assert.False(t, collector.tableCache.isConfigCached(dependencyConfig))
	assert.False(t, collector.tableCache.isConfigCached(sourceConfig))
	assert.False(t, collector.tableCache.isConfigCached(*auxiliaryConfig))

	expectSNMPWalk(mockHandler, gosnmp.Version2c, sourceTableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(sourceMetricOID, 14),
	})
	expectSNMPWalkError(mockHandler, gosnmp.Version2c, dependencyTableOID, errors.New("dependency timeout"))

	results, err = collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, sourceProfile.SourceFile, results[0].Source)
	require.Len(t, results[0].TopologyMetrics, 1)
	assert.Empty(t, results[0].TopologyMetrics[0].Tags)
	assert.Equal(t, int64(0), results[0].Stats.TableCache.Hits)
	assert.Equal(t, int64(0), results[0].Stats.TableCache.Misses)
	assert.Equal(t, int64(0), results[0].Stats.SNMP.GetRequests)
	assert.Equal(t, int64(1), results[0].Stats.SNMP.WalkRequests)
	assert.False(t, collector.tableCache.isConfigCached(dependencyConfig))
	assert.False(t, collector.tableCache.isConfigCached(sourceConfig))
	assert.False(t, collector.tableCache.isConfigCached(*auxiliaryConfig))
}

func TestCollector_Collect_DiscardsPromotedSourceCacheAfterFreshWalkFailure(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	dependencyConfig, sourceConfig := crossTableDependencyTestConfigs("1.3.6.1.4.1.99999")
	sourceTableOID := sourceConfig.Table.OID
	sourceMetricOID := sourceConfig.Symbols[0].OID + ".1"
	dependencyTableOID := dependencyConfig.Table.OID
	dependencyMetricOID := dependencyConfig.Symbols[0].OID + ".1"
	profile := &ddsnmp.Profile{
		SourceFile: "promoted-source-cache.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{
			Metrics: []ddprofiledefinition.MetricsConfig{dependencyConfig, sourceConfig},
		},
	}
	ddsnmp.HandleCrossTableTagsWithoutMetrics(profile)

	expectSNMPWalk(mockHandler, gosnmp.Version2c, sourceTableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(sourceMetricOID, 10),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, dependencyTableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(dependencyMetricOID, 20),
	})

	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
	})
	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Len(t, results[0].Metrics, 2)
	assert.True(t, collector.tableCache.isConfigCached(sourceConfig))
	assert.True(t, collector.tableCache.isConfigCached(dependencyConfig))

	var sourceMetric *ddsnmp.Metric
	for i := range results[0].Metrics {
		if results[0].Metrics[i].Name == "sourceValue" {
			sourceMetric = &results[0].Metrics[i]
			break
		}
	}
	require.NotNil(t, sourceMetric)
	assert.Equal(t, map[string]string{"dependency_value": "20"}, sourceMetric.Tags)

	expectSNMPGet(mockHandler, []string{sourceMetricOID}, []gosnmp.SnmpPDU{
		createGauge32PDU(sourceMetricOID, 11),
	})
	expectSNMPGet(mockHandler, []string{dependencyMetricOID}, nil)
	expectSNMPWalk(mockHandler, gosnmp.Version2c, dependencyTableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(dependencyMetricOID, 30),
	})
	expectSNMPWalkError(mockHandler, gosnmp.Version2c, sourceTableOID, errors.New("source timeout"))

	results, err = collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Len(t, results[0].Metrics, 1)
	assert.Equal(t, "dependencyValue", results[0].Metrics[0].Name)
	assert.True(t, collector.tableCache.isConfigCached(dependencyConfig))
	require.False(t, collector.tableCache.isConfigCached(sourceConfig), "failed promoted route retained stale tags")

	expectSNMPWalk(mockHandler, gosnmp.Version2c, sourceTableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(sourceMetricOID, 12),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, dependencyTableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(dependencyMetricOID, 40),
	})

	results, err = collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Len(t, results[0].Metrics, 2)
	sourceMetric = nil
	for i := range results[0].Metrics {
		if results[0].Metrics[i].Name == "sourceValue" {
			sourceMetric = &results[0].Metrics[i]
			break
		}
	}
	require.NotNil(t, sourceMetric)
	assert.Equal(t, map[string]string{"dependency_value": "40"}, sourceMetric.Tags)
}

func TestCollector_Collect_KeepsTopologyCacheIneligibleWhenRegularScopeFails(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const (
		regularTableOID   = "1.3.6.1.4.1.99999.10"
		regularColumnOID  = regularTableOID + ".1"
		regularMetricOID  = regularColumnOID + ".1"
		topologyTableOID  = "1.3.6.1.4.1.99999.20"
		topologyColumnOID = topologyTableOID + ".1"
		topologyMetricOID = topologyColumnOID + ".1"
	)
	regularConfig := ddprofiledefinition.MetricsConfig{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  regularTableOID,
			Name: "regularTable",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{{
			OID:  regularColumnOID,
			Name: "regularValue",
		}},
	}
	topologyConfig := ddprofiledefinition.MetricsConfig{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  topologyTableOID,
			Name: "topologyTable",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{{
			OID:  topologyColumnOID,
			Name: "topologyValue",
		}},
	}
	profile := &ddsnmp.Profile{
		SourceFile: "fresh-unconsumed-topology.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{
			Metrics: []ddprofiledefinition.MetricsConfig{regularConfig},
			Topology: []ddprofiledefinition.TopologyConfig{{
				Kind:          ddprofiledefinition.KindIfName,
				MetricsConfig: topologyConfig,
			}},
		},
	}

	expectSNMPWalk(mockHandler, gosnmp.Version2c, regularTableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(regularMetricOID, 10),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, topologyTableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(topologyMetricOID, 20),
	})

	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
	})
	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Len(t, results[0].Metrics, 1)
	require.Len(t, results[0].TopologyMetrics, 1)
	assert.True(t, collector.tableCache.isConfigCached(regularConfig))
	assert.False(t, collector.tableCache.isConfigCached(topologyConfig))

	expectSNMPGet(mockHandler, []string{regularMetricOID}, nil)
	expectSNMPWalkError(mockHandler, gosnmp.Version2c, regularTableOID, errors.New("regular timeout"))
	expectSNMPWalk(mockHandler, gosnmp.Version2c, topologyTableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(topologyMetricOID, 30),
	})

	results, err = collector.Collect()
	require.Error(t, err)
	assert.Empty(t, results)
	require.False(t, collector.tableCache.isConfigCached(topologyConfig), "topology presence must remain cache-ineligible")
	assert.False(t, collector.tableCache.isConfigCached(regularConfig), "failed route retained old cache")
}

func TestCollector_Collect_DiscardsOmittedCachedDependentWhenDependencyRefreshes(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const scalarOID = "1.3.6.1.4.1.99999.0"
	dependencyConfig, sourceConfig := crossTableDependencyTestConfigs("1.3.6.1.4.1.99999")
	sourceTableOID := sourceConfig.Table.OID
	sourceMetricOID := sourceConfig.Symbols[0].OID + ".1"
	dependencyTableOID := dependencyConfig.Table.OID
	dependencyMetricOID := dependencyConfig.Symbols[0].OID + ".1"
	dependencyProfile := createTestProfile("dependency-owner.yaml", []ddprofiledefinition.MetricsConfig{dependencyConfig})
	sourceProfile := createTestProfile("source-owner.yaml", []ddprofiledefinition.MetricsConfig{
		createScalarMetric(scalarOID, "sourceScalar"),
		sourceConfig,
	})
	ddsnmp.HandleCrossTableTagsWithoutMetrics(sourceProfile)

	expectSNMPGet(mockHandler, []string{scalarOID}, []gosnmp.SnmpPDU{
		createGauge32PDU(scalarOID, 1),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, sourceTableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(sourceMetricOID, 10),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, dependencyTableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(dependencyMetricOID, 20),
	})

	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{dependencyProfile, sourceProfile},
		Log:        logger.New(),
	})
	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.True(t, collector.tableCache.isConfigCached(sourceConfig))
	assert.True(t, collector.tableCache.isConfigCached(dependencyConfig))

	findMetric := func(results []*ddsnmp.ProfileMetrics, source, name string) *ddsnmp.Metric {
		for _, result := range results {
			if result.Source != source {
				continue
			}
			for i := range result.Metrics {
				if result.Metrics[i].Name == name {
					return &result.Metrics[i]
				}
			}
		}
		return nil
	}
	sourceMetric := findMetric(results, sourceProfile.SourceFile, "sourceValue")
	require.NotNil(t, sourceMetric)
	assert.Equal(t, map[string]string{"dependency_value": "20"}, sourceMetric.Tags)

	expectSNMPGetError(mockHandler, []string{scalarOID}, errors.New("scalar timeout"))
	expectSNMPGet(mockHandler, []string{dependencyMetricOID}, nil)
	expectSNMPWalk(mockHandler, gosnmp.Version2c, dependencyTableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(dependencyMetricOID, 30),
	})

	results, err = collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, dependencyProfile.SourceFile, results[0].Source)
	assert.True(t, collector.tableCache.isConfigCached(dependencyConfig))
	require.False(t, collector.tableCache.isConfigCached(sourceConfig), "omitted reverse dependent retained stale tags")

	expectSNMPGet(mockHandler, []string{scalarOID}, []gosnmp.SnmpPDU{
		createGauge32PDU(scalarOID, 2),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, sourceTableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(sourceMetricOID, 12),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, dependencyTableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(dependencyMetricOID, 40),
	})

	results, err = collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 2)
	sourceMetric = findMetric(results, sourceProfile.SourceFile, "sourceValue")
	require.NotNil(t, sourceMetric)
	assert.Equal(t, map[string]string{"dependency_value": "40"}, sourceMetric.Tags)
}

func TestCollector_Collect_FreshAuxiliaryWalkMakesFailedSourcesPartialSuccess(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const (
		scalarOID       = "1.3.6.1.4.1.99999.0"
		sourceTableOID1 = "1.3.6.1.4.1.99999.1"
		sourceTableOID2 = "1.3.6.1.4.1.99999.2"
		dependencyOID   = "1.3.6.1.4.1.99999.3.1"
	)
	profile := profileWithTwoSourcesAndAuxiliary()
	profile.Definition.Metrics = append(
		[]ddprofiledefinition.MetricsConfig{createScalarMetric(scalarOID, "deviceScalar")},
		profile.Definition.Metrics...,
	)

	expectSNMPGet(mockHandler, []string{scalarOID}, []gosnmp.SnmpPDU{
		createGauge32PDU(scalarOID, 42),
	})
	expectSNMPWalkError(mockHandler, gosnmp.Version2c, sourceTableOID1, errors.New("source one timeout"))
	expectSNMPWalkError(mockHandler, gosnmp.Version2c, sourceTableOID2, errors.New("source two timeout"))
	expectSNMPWalk(mockHandler, gosnmp.Version2c, dependencyOID, []gosnmp.SnmpPDU{
		createStringPDU(dependencyOID+".1", "dependency"),
	})

	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
	})
	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Len(t, results[0].Metrics, 1)
	assert.Equal(t, "deviceScalar", results[0].Metrics[0].Name)
	assert.EqualValues(t, 42, results[0].Metrics[0].Value)
	assert.Equal(t, int64(3), results[0].Stats.TableCache.Misses)
	assert.Equal(t, int64(3), results[0].Stats.SNMP.WalkRequests)
	assert.Equal(t, int64(1), results[0].Stats.SNMP.TablesWalked)
	assert.Equal(t, int64(2), results[0].Stats.Errors.SNMP)
}

func TestCollector_Collect_PreservesHiddenMetrics(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSNMPWalk(mockHandler,
		gosnmp.Version2c,
		"1.3.6.1.4.1.99999.1",
		[]gosnmp.SnmpPDU{
			createCounter32PDU("1.3.6.1.4.1.99999.1.1.1", 100),
		},
	)

	profile := &ddsnmp.Profile{
		SourceFile: "hidden-metrics-profile.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{
			Metrics: []ddprofiledefinition.MetricsConfig{
				{
					Table: ddprofiledefinition.SymbolConfig{
						OID:  "1.3.6.1.4.1.99999.1",
						Name: "privateTable",
					},
					Symbols: []ddprofiledefinition.SymbolConfig{
						{
							OID:  "1.3.6.1.4.1.99999.1.1",
							Name: "_privateMetric",
						},
					},
				},
			},
			VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
				{
					Name: "privateMetric_total",
					Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
						{
							Metric: "_privateMetric",
							Table:  "privateTable",
						},
					},
				},
				{
					Name: "_privateMetric_total",
					Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
						{
							Metric: "_privateMetric",
							Table:  "privateTable",
						},
					},
				},
			},
		},
	}

	require.NoError(t, ddsnmp.CompileTransforms(profile))

	collector := New(Config{
		SnmpClient:  mockHandler,
		Profiles:    []*ddsnmp.Profile{profile},
		Log:         logger.New(),
		SysObjectID: "",
	})

	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)

	pm := results[0]
	require.Len(t, pm.HiddenMetrics, 2)
	assert.Equal(t, "_privateMetric", pm.HiddenMetrics[0].Name)
	assert.Equal(t, "_privateMetric_total", pm.HiddenMetrics[1].Name)
	require.Len(t, pm.Metrics, 1)
	assert.Equal(t, "privateMetric_total", pm.Metrics[0].Name)
}

func TestCollector_Collect_SeparatesTopologyMetricsFromHiddenMetrics(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSNMPWalk(mockHandler,
		gosnmp.Version2c,
		"1.3.6.1.4.1.99999.1",
		[]gosnmp.SnmpPDU{
			createCounter32PDU("1.3.6.1.4.1.99999.1.1.1", 100),
		},
	)
	expectSNMPGet(mockHandler,
		[]string{"1.3.6.1.4.1.99999.2.0"},
		[]gosnmp.SnmpPDU{
			createIntegerPDU("1.3.6.1.4.1.99999.2.0", 1),
		},
	)

	profile := &ddsnmp.Profile{
		SourceFile: "topology-delivery-profile.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{
			Metrics: []ddprofiledefinition.MetricsConfig{
				{
					Table: ddprofiledefinition.SymbolConfig{
						OID:  "1.3.6.1.4.1.99999.1",
						Name: "privateTable",
					},
					Symbols: []ddprofiledefinition.SymbolConfig{
						{
							OID:  "1.3.6.1.4.1.99999.1.1",
							Name: "_privateMetric",
						},
					},
				},
			},
			Topology: []ddprofiledefinition.TopologyConfig{
				{
					Kind: ddprofiledefinition.KindIfStatus,
					MetricsConfig: ddprofiledefinition.MetricsConfig{
						Symbol: ddprofiledefinition.SymbolConfig{
							OID:  "1.3.6.1.4.1.99999.2.0",
							Name: "if_status",
						},
					},
				},
			},
		},
	}

	collector := New(Config{
		SnmpClient:  mockHandler,
		Profiles:    []*ddsnmp.Profile{profile},
		Log:         logger.New(),
		SysObjectID: "",
	})

	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)

	pm := results[0]
	require.Len(t, pm.HiddenMetrics, 1)
	assert.Equal(t, "_privateMetric", pm.HiddenMetrics[0].Name)
	require.Len(t, pm.TopologyMetrics, 1)
	assert.Equal(t, "if_status", pm.TopologyMetrics[0].Name)
	assert.Equal(t, int64(1), pm.TopologyMetrics[0].Value)
	assert.Equal(t, ddsnmp.KindIfStatus, pm.TopologyMetrics[0].TopologyKind)
	require.Empty(t, pm.Metrics)
}

func crossTableDependencyTestConfigs(oidBase string) (ddprofiledefinition.MetricsConfig, ddprofiledefinition.MetricsConfig) {
	dependencyColumnOID := oidBase + ".2.1"
	dependency := ddprofiledefinition.MetricsConfig{
		Table:   ddprofiledefinition.SymbolConfig{OID: oidBase + ".2", Name: "dependencyTable"},
		Symbols: []ddprofiledefinition.SymbolConfig{{OID: dependencyColumnOID, Name: "dependencyValue"}},
	}
	source := ddprofiledefinition.MetricsConfig{
		Table:   ddprofiledefinition.SymbolConfig{OID: oidBase + ".1", Name: "sourceTable"},
		Symbols: []ddprofiledefinition.SymbolConfig{{OID: oidBase + ".1.1", Name: "sourceValue"}},
		MetricTags: []ddprofiledefinition.MetricTagConfig{{
			Tag:   "dependency_value",
			Table: "dependencyTable",
			Symbol: ddprofiledefinition.SymbolConfigCompat{
				OID:  dependencyColumnOID,
				Name: "dependencyValue",
			},
		}},
	}
	return dependency, source
}
