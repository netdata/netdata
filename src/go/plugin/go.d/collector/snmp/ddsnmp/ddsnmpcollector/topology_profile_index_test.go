// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func TestTopologyProfile_QBridgeFDBUsesMACFromIndex(t *testing.T) {
	tests := map[string]struct {
		indexSuffix string
	}{
		"normal_mac_index":          {indexSuffix: "7.0.80.86.171.205.239"},
		"length_prefixed_mac_index": {indexSuffix: "7.6.0.80.86.171.205.239"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl, mockHandler := setupMockHandler(t)
			defer ctrl.Finish()

			expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.17.7.1.2.2.1", []gosnmp.SnmpPDU{
				createIntegerPDU("1.3.6.1.2.1.17.7.1.2.2.1.2."+tc.indexSuffix, 5),
				createIntegerPDU("1.3.6.1.2.1.17.7.1.2.2.1.3."+tc.indexSuffix, 3),
			})
			expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.17.7.1.4.2.1", nil)

			actual := collectTopologyProfileTables(t, mockHandler, "_std-topology-q-bridge-mib")

			assertTableMetricsEqual(t, []ddsnmp.Metric{qBridgeFDBMetric()}, actual)
		})
	}
}

func TestTopologyProfile_IPNetToPhysicalUsesIndexFields(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.31.1.1", nil)
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.2.2", nil)
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.10.7.2", nil)
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.4.20", nil)
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.17.1.4", nil)
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.17.4.3", nil)
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.4.35.1", []gosnmp.SnmpPDU{
		createPDU("1.3.6.1.2.1.4.35.1.4.2.1.4.10.0.2.10", gosnmp.OctetString, []byte{0x00, 0x50, 0x56, 0xab, 0xcd, 0xef}),
		createIntegerPDU("1.3.6.1.2.1.4.35.1.6.2.1.4.10.0.2.10", 1),
		createPDU("1.3.6.1.2.1.4.35.1.4.3.2.16.254.128.0.0.0.0.0.0.0.0.0.0.0.0.0.1", gosnmp.OctetString, []byte{0x00, 0x50, 0x56, 0xab, 0xcd, 0xf0}),
		createIntegerPDU("1.3.6.1.2.1.4.35.1.6.3.2.16.254.128.0.0.0.0.0.0.0.0.0.0.0.0.0.1", 2),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.4.22", nil)

	actual := collectTopologyProfileTables(t, mockHandler, "_std-topology-fdb-arp-mib")

	assertTableMetricsEqual(t, []ddsnmp.Metric{
		{
			Name:         "arp_entry",
			Value:        1,
			Tags:         map[string]string{"arp_if_index": "2", "arp_addr_type": "ipv4", "arp_ip": "10.0.2.10", "arp_mac": "005056abcdef", "arp_state": "1"},
			MetricType:   "gauge",
			IsTable:      true,
			Table:        "ipNetToPhysicalTable",
			TopologyKind: ddsnmp.KindArpEntry,
		},
		{
			Name:         "arp_entry",
			Value:        2,
			Tags:         map[string]string{"arp_if_index": "3", "arp_addr_type": "ipv6", "arp_ip": "fe80::1", "arp_mac": "005056abcdf0", "arp_state": "2"},
			MetricType:   "gauge",
			IsTable:      true,
			Table:        "ipNetToPhysicalTable",
			TopologyKind: ddsnmp.KindArpEntry,
		},
	}, actual)
}

func TestTopologyProfile_LLDPManagementAddressUsesIndexFields(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.0.8802.1.1.2.1.3.7", nil)
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.0.8802.1.1.2.1.3.8", []gosnmp.SnmpPDU{
		createIntegerPDU("1.0.8802.1.1.2.1.3.8.1.3.1.4.10.0.0.1", 4),
		createIntegerPDU("1.0.8802.1.1.2.1.3.8.1.4.1.4.10.0.0.1", 2),
		createIntegerPDU("1.0.8802.1.1.2.1.3.8.1.5.1.4.10.0.0.1", 12),
		createStringPDU("1.0.8802.1.1.2.1.3.8.1.6.1.4.10.0.0.1", "0.0"),
		createIntegerPDU("1.0.8802.1.1.2.1.3.8.1.3.6.6.0.80.86.171.205.239", 6),
		createIntegerPDU("1.0.8802.1.1.2.1.3.8.1.4.6.6.0.80.86.171.205.239", 2),
		createIntegerPDU("1.0.8802.1.1.2.1.3.8.1.5.6.6.0.80.86.171.205.239", 12),
		createStringPDU("1.0.8802.1.1.2.1.3.8.1.6.6.6.0.80.86.171.205.239", "0.0"),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.0.8802.1.1.2.1.4.1", nil)
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.0.8802.1.1.2.1.4.2", nil)

	actual := collectTopologyProfileTables(t, mockHandler, "_std-topology-lldp-mib")

	assertTableMetricsEqual(t, []ddsnmp.Metric{
		{
			Name:         "lldp_loc_man_addr",
			Value:        4,
			Tags:         map[string]string{"lldp_loc_mgmt_addr_subtype": "1", "lldp_loc_mgmt_addr": "0a000001", "lldp_loc_mgmt_addr_if_subtype": "2", "lldp_loc_mgmt_addr_if_id": "12", "lldp_loc_mgmt_addr_oid": "0.0"},
			MetricType:   "gauge",
			IsTable:      true,
			Table:        "lldpLocManAddrTable",
			TopologyKind: ddsnmp.KindLldpLocManAddr,
		},
		{
			Name:         "lldp_loc_man_addr",
			Value:        6,
			Tags:         map[string]string{"lldp_loc_mgmt_addr_subtype": "6", "lldp_loc_mgmt_addr": "005056abcdef", "lldp_loc_mgmt_addr_if_subtype": "2", "lldp_loc_mgmt_addr_if_id": "12", "lldp_loc_mgmt_addr_oid": "0.0"},
			MetricType:   "gauge",
			IsTable:      true,
			Table:        "lldpLocManAddrTable",
			TopologyKind: ddsnmp.KindLldpLocManAddr,
		},
	}, actual)
}

func collectTopologyProfileTables(t *testing.T, mockHandler gosnmp.Handler, profileName string) []ddsnmp.Metric {
	t.Helper()

	profile, err := ddsnmp.LoadProfileByName(profileName)
	require.NoError(t, err)

	missingOIDs := make(map[string]bool)
	tcache := newTableCache(0, 0)
	collector := &Collector{
		scalarCollector: newScalarCollector(mockHandler, missingOIDs, logger.New()),
		tableCollector:  newTableCollector(mockHandler, missingOIDs, tcache, logger.New(), false),
	}

	var stats ddsnmp.CollectionStats
	actual, err := collector.collectTopologyMetrics(profile, &stats)
	require.NoError(t, err)

	return actual
}

func qBridgeFDBMetric() ddsnmp.Metric {
	return ddsnmp.Metric{
		Name:         "qbridge_fdb_entry",
		Value:        5,
		Tags:         map[string]string{"dot1q_fdb_id": "7", "dot1q_fdb_mac": "00:50:56:ab:cd:ef", "dot1q_fdb_bridge_port": "5", "dot1q_fdb_status": "3"},
		MetricType:   "gauge",
		IsTable:      true,
		Table:        "dot1qTpFdbTable",
		TopologyKind: ddsnmp.KindQbridgeFdbEntry,
	}
}
