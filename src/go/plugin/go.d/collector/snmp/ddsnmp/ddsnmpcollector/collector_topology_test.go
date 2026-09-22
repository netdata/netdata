// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"slices"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestCollector_Collect_CiscoCDPTopologyWithoutOrdinaryMetrics(t *testing.T) {
	const (
		cdpCacheTableOID     = "1.3.6.1.4.1.9.9.23.1.2.1"
		cdpInterfaceTableOID = "1.3.6.1.4.1.9.9.23.1.1.1"
		ifNameColumnOID      = "1.3.6.1.2.1.31.1.1.1.1"
	)

	profile, err := ddsnmp.LoadProfileByName("cisco-catalyst")
	require.NoError(t, err)
	topology := slices.DeleteFunc(slices.Clone(profile.Definition.Topology), func(row ddprofiledefinition.TopologyConfig) bool {
		return row.Kind != ddsnmp.KindCdpCache
	})
	profile.Definition = &ddprofiledefinition.ProfileDefinition{Topology: topology}
	ddsnmp.HandleCrossTableTagsWithoutMetrics(profile)
	require.NotEmpty(t, profile.Definition.Topology)

	device := newStatefulSNMPDevice()
	device.set(createStringPDU(ifNameColumnOID+".5", "TenGigabitEthernet0/1/0"))
	device.set(createStringPDU(ifNameColumnOID+".6", "TenGigabitEthernet0/1/1"))
	device.set(createIntegerPDU(cdpCacheTableOID+".1.3.5.1", 1))
	device.set(createIntegerPDU(cdpCacheTableOID+".1.3.6.2", 1))

	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()
	device.install(mockHandler)
	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
	})

	for range 2 {
		results, err := collector.Collect()
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Empty(t, results[0].Metrics)
		require.Len(t, results[0].TopologyMetrics, 2)
		for _, metric := range results[0].TopologyMetrics {
			assert.Equal(t, "cdp_cache", metric.Name)
			assert.Equal(t, ddsnmp.KindCdpCache, metric.TopologyKind)
			assert.NotEmpty(t, metric.Tags["cdp_if_index"])
			assert.NotEmpty(t, metric.Tags["cdp_if_name"])
		}
	}

	assert.Equal(t, 2, device.walkCount[cdpCacheTableOID])
	assert.Equal(t, 2, device.walkCount[ifNameColumnOID])
	assert.Zero(t, device.walkCount[cdpInterfaceTableOID])
}

func TestCollector_CollectTopologyMetrics_TableSymbolUsesPDUPresenceWithoutStructureCache(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const (
		tableOID  = "1.3.6.1.2.1.4.22"
		columnOID = "1.3.6.1.2.1.4.22.1.2"
		rowOID    = columnOID + ".2.192.0.2.10"
	)

	pdu := createPDU(rowOID, gosnmp.OctetString, []byte{0x00, 0x50, 0x56, 0xab, 0xcd, 0xef})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, tableOID, []gosnmp.SnmpPDU{pdu})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, tableOID, []gosnmp.SnmpPDU{pdu})

	profile := &ddsnmp.Profile{
		SourceFile: "topology-presence-profile.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{
			Topology: []ddprofiledefinition.TopologyConfig{
				{
					Kind: ddsnmp.KindArpEntry,
					MetricsConfig: ddprofiledefinition.MetricsConfig{
						Table: ddprofiledefinition.SymbolConfig{OID: tableOID, Name: "ipNetToMediaTable"},
						Symbols: []ddprofiledefinition.SymbolConfig{
							{OID: columnOID, Name: "arp_entry"},
						},
						MetricTags: []ddprofiledefinition.MetricTagConfig{
							{Tag: "arp_if_index", Index: 1},
							{
								Tag: "arp_mac",
								Symbol: ddprofiledefinition.SymbolConfigCompat{
									OID:    columnOID,
									Name:   "ipNetToMediaPhysAddress",
									Format: "hex",
								},
							},
						},
					},
				},
			},
		},
	}

	cache := newTableCache(time.Hour, 0)
	collector := &Collector{
		scalarCollector: newScalarCollector(mockHandler, make(map[string]bool), logger.New()),
		tableCollector:  newTableCollector(mockHandler, make(map[string]bool), cache, logger.New(), false),
	}

	expected := []ddsnmp.Metric{
		{
			Name:         "arp_entry",
			Value:        0,
			Tags:         map[string]string{"arp_if_index": "2", "arp_mac": "005056abcdef"},
			MetricType:   ddprofiledefinition.ProfileMetricTypeGauge,
			IsTable:      true,
			Table:        "ipNetToMediaTable",
			TopologyKind: ddsnmp.KindArpEntry,
		},
	}

	var walkStats ddsnmp.CollectionStats
	walkMetrics, err := collector.collectTopologyMetrics(profile, &walkStats)
	require.NoError(t, err)
	require.Equal(t, expected, walkMetrics)
	require.Equal(t, int64(1), walkStats.SNMP.WalkRequests)
	require.Equal(t, int64(0), walkStats.SNMP.GetRequests)
	require.Equal(t, int64(0), walkStats.SNMP.TablesCached)

	var secondWalkStats ddsnmp.CollectionStats
	secondWalkMetrics, err := collector.collectTopologyMetrics(profile, &secondWalkStats)
	require.NoError(t, err)
	require.Equal(t, expected, secondWalkMetrics)
	require.Equal(t, int64(1), secondWalkStats.SNMP.WalkRequests)
	require.Equal(t, int64(0), secondWalkStats.SNMP.GetRequests)
	require.Equal(t, int64(0), secondWalkStats.SNMP.TablesCached)
	require.Zero(t, secondWalkStats.TableCache.Hits)
	require.Zero(t, secondWalkStats.TableCache.Misses)
}

func TestCollector_CollectTopologyMetrics_DependencyOnlyRouteWaitsForAnchorRows(t *testing.T) {
	const (
		anchorTableOID      = "1.3.6.1.4.1.99999.1"
		anchorColumnOID     = anchorTableOID + ".1"
		dependencyColumnOID = "1.3.6.1.4.1.99999.2.1"
	)

	profile := &ddsnmp.Profile{
		SourceFile: "topology-lazy-dependency.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{
			Topology: []ddprofiledefinition.TopologyConfig{{
				Kind: ddsnmp.KindIpIfIndex,
				MetricsConfig: ddprofiledefinition.MetricsConfig{
					Table:   ddprofiledefinition.SymbolConfig{OID: anchorTableOID, Name: "anchorTable"},
					Symbols: []ddprofiledefinition.SymbolConfig{{OID: anchorColumnOID, Name: "ip_if_index"}},
					MetricTags: []ddprofiledefinition.MetricTagConfig{{
						Tag:    "dependency_value",
						Table:  "dependencyTable",
						Symbol: ddprofiledefinition.SymbolConfigCompat{OID: dependencyColumnOID, Name: "dependencyValue"},
					}},
				},
			}},
		},
	}
	ddsnmp.HandleCrossTableTagsWithoutMetrics(profile)

	device := newStatefulSNMPDevice()
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()
	device.install(mockHandler)
	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
	})

	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Empty(t, results[0].TopologyMetrics)
	assert.Equal(t, 1, device.walkCount[anchorTableOID])
	assert.Zero(t, device.walkCount[dependencyColumnOID])
}

func TestCollector_CollectTopologyMetrics_DependencyWithSymbolOwnerRemainsEager(t *testing.T) {
	const (
		anchorTableOID      = "1.3.6.1.4.1.99999.1"
		anchorColumnOID     = anchorTableOID + ".1"
		dependencyColumnOID = "1.3.6.1.4.1.99999.2.1"
	)

	profile := &ddsnmp.Profile{
		SourceFile: "topology-owned-dependency.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{
			Topology: []ddprofiledefinition.TopologyConfig{
				{
					Kind: ddsnmp.KindIpIfIndex,
					MetricsConfig: ddprofiledefinition.MetricsConfig{
						Table:   ddprofiledefinition.SymbolConfig{OID: anchorTableOID, Name: "anchorTable"},
						Symbols: []ddprofiledefinition.SymbolConfig{{OID: anchorColumnOID, Name: "anchor"}},
						MetricTags: []ddprofiledefinition.MetricTagConfig{{
							Tag:    "dependency_value",
							Table:  "dependencyTable",
							Symbol: ddprofiledefinition.SymbolConfigCompat{OID: dependencyColumnOID, Name: "dependencyValue"},
						}},
					},
				},
				{
					Kind: ddsnmp.KindIpIfIndex,
					MetricsConfig: ddprofiledefinition.MetricsConfig{
						Table:   ddprofiledefinition.SymbolConfig{OID: dependencyColumnOID, Name: "dependencyTable"},
						Symbols: []ddprofiledefinition.SymbolConfig{{OID: dependencyColumnOID, Name: "dependencyValue"}},
					},
				},
			},
		},
	}

	device := newStatefulSNMPDevice()
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()
	device.install(mockHandler)
	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
	})

	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Empty(t, results[0].TopologyMetrics)
	assert.Equal(t, 1, device.walkCount[anchorTableOID])
	assert.Equal(t, 1, device.walkCount[dependencyColumnOID])
}
