// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

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
