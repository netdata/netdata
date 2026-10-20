// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
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

			expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.17.1.4", nil)
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

func TestTopologyProfile_BridgeFDBUsesMACFromIndex(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.17.1.4", nil)
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.17.4.3", []gosnmp.SnmpPDU{
		createIntegerPDU("1.3.6.1.2.1.17.4.3.1.2.120.133.23.132.11.153", 1),
		createIntegerPDU("1.3.6.1.2.1.17.4.3.1.3.120.133.23.132.11.153", 3),
	})

	actual := collectTopologyProfileTables(t, mockHandler, "_std-topology-fdb-mib")

	assertTableMetricsEqual(t, []ddsnmp.Metric{{
		Name:         "fdb_entry",
		Value:        0,
		Tags:         map[string]string{"fdb_mac": "78:85:17:84:0b:99", "fdb_bridge_port": "1", "fdb_status": "learned"},
		MetricType:   "gauge",
		IsTable:      true,
		Table:        "dot1dTpFdbTable",
		TopologyKind: ddsnmp.KindFdbEntry,
	}}, actual)
}

func TestTopologyProfile_InterfaceStateMappings(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.31.1.1", nil)
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.2.2", []gosnmp.SnmpPDU{
		createStringPDU("1.3.6.1.2.1.2.2.1.2.7", "Ethernet1"),
		createIntegerPDU("1.3.6.1.2.1.2.2.1.7.7", 1),
		createIntegerPDU("1.3.6.1.2.1.2.2.1.8.7", 6),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.10.7.2", []gosnmp.SnmpPDU{
		createIntegerPDU("1.3.6.1.2.1.10.7.2.1.19.7", 3),
	})

	actual := collectTopologyProfileTables(t, mockHandler, "_std-topology-interface-mib")

	assertTableMetricsEqual(t, []ddsnmp.Metric{
		{
			Name:         "if_status",
			Value:        0,
			Tags:         map[string]string{"topo_if_index": "7", "topo_if_descr": "Ethernet1", "topo_if_admin_status": "up", "topo_if_oper_status": "notPresent"},
			MetricType:   "gauge",
			IsTable:      true,
			Table:        "ifTable",
			TopologyKind: ddsnmp.KindIfStatus,
		},
		{
			Name:         "if_duplex",
			Value:        0,
			Tags:         map[string]string{"topo_if_index": "7", "topo_if_duplex": "full"},
			MetricType:   "gauge",
			IsTable:      true,
			Table:        "dot3StatsTable",
			TopologyKind: ddsnmp.KindIfDuplex,
		},
	}, actual)
}

func TestTopologyProfile_IPAddressUsesTableIndex(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.4.20", []gosnmp.SnmpPDU{
		createIntegerPDU("1.3.6.1.2.1.4.20.1.2.192.0.2.17", 7),
		createPDU("1.3.6.1.2.1.4.20.1.3.192.0.2.17", gosnmp.IPAddress, "255.255.255.248"),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.4.34.1.3.1.4", nil)

	actual := collectTopologyProfileTables(t, mockHandler, "_std-topology-ip-mib")

	assertTableMetricsEqual(t, []ddsnmp.Metric{{
		Name:         "ip_if_index",
		Value:        0,
		StaticTags:   map[string]string{"topo_ip_source": "legacy"},
		Tags:         map[string]string{"topo_ip_addr": "192.0.2.17", "topo_if_index": "7", "topo_ip_netmask": "255.255.255.248", "topo_ip_source": "legacy"},
		MetricType:   "gauge",
		IsTable:      true,
		Table:        "ipAddrTable",
		TopologyKind: ddsnmp.KindIpIfIndex,
	}}, actual)
}

func TestTopologyProfile_CiscoVTPUsesVLANIndexComponent(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.4.1.9.9.46.1.3.1.1", []gosnmp.SnmpPDU{
		createIntegerPDU("1.3.6.1.4.1.9.9.46.1.3.1.1.2.1.200", 1),
		createIntegerPDU("1.3.6.1.4.1.9.9.46.1.3.1.1.3.1.200", 1),
		createStringPDU("1.3.6.1.4.1.9.9.46.1.3.1.1.4.1.200", "servers"),
	})

	actual := collectTopologyProfileTables(t, mockHandler, "_std-topology-cisco-vtp-mib")

	assertTableMetricsEqual(t, []ddsnmp.Metric{{
		Name:         "vtp_vlan",
		Value:        0,
		Tags:         map[string]string{"vtp_vlan_index": "200", "vtp_vlan_state": "operational", "vtp_vlan_type": "1", "vtp_vlan_name": "servers"},
		MetricType:   "gauge",
		IsTable:      true,
		Table:        "vtpVlanTable",
		TopologyKind: ddsnmp.KindVtpVlan,
	}}, actual)
}

func TestTopologyProfile_IPNetToPhysicalUsesIndexFields(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.31.1.1.1.1", nil)
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.4.35.1", []gosnmp.SnmpPDU{
		createPDU("1.3.6.1.2.1.4.35.1.4.2.1.4.10.0.2.10", gosnmp.OctetString, []byte{0x00, 0x50, 0x56, 0xab, 0xcd, 0xef}),
		createIntegerPDU("1.3.6.1.2.1.4.35.1.6.2.1.4.10.0.2.10", 3),
		createIntegerPDU("1.3.6.1.2.1.4.35.1.7.2.1.4.10.0.2.10", 1),
		createPDU("1.3.6.1.2.1.4.35.1.4.3.2.16.254.128.0.0.0.0.0.0.0.0.0.0.0.0.0.1", gosnmp.OctetString, []byte{0x00, 0x50, 0x56, 0xab, 0xcd, 0xf0}),
		createIntegerPDU("1.3.6.1.2.1.4.35.1.6.3.2.16.254.128.0.0.0.0.0.0.0.0.0.0.0.0.0.1", 4),
		createIntegerPDU("1.3.6.1.2.1.4.35.1.7.3.2.16.254.128.0.0.0.0.0.0.0.0.0.0.0.0.0.1", 2),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.4.22", nil)

	actual := collectTopologyProfileTables(t, mockHandler, "_std-topology-arp-mib")

	assertTableMetricsEqual(t, []ddsnmp.Metric{
		{
			Name:         "arp_entry",
			Value:        0,
			Tags:         map[string]string{"arp_if_index": "2", "arp_addr_type": "ipv4", "arp_ip": "10.0.2.10", "arp_mac": "005056abcdef", "arp_type": "dynamic", "arp_state": "reachable"},
			MetricType:   "gauge",
			IsTable:      true,
			Table:        "ipNetToPhysicalTable",
			TopologyKind: ddsnmp.KindArpEntry,
		},
		{
			Name:         "arp_entry",
			Value:        0,
			Tags:         map[string]string{"arp_if_index": "3", "arp_addr_type": "ipv6", "arp_ip": "fe80::1", "arp_mac": "005056abcdf0", "arp_type": "static", "arp_state": "stale"},
			MetricType:   "gauge",
			IsTable:      true,
			Table:        "ipNetToPhysicalTable",
			TopologyKind: ddsnmp.KindArpEntry,
		},
	}, actual)
}

func TestTopologyProfile_LegacyARPUsesPhysicalAddressPresence(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.31.1.1.1.1", nil)
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.4.35.1", nil)
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.4.22", []gosnmp.SnmpPDU{
		createPDU("1.3.6.1.2.1.4.22.1.2.17.192.0.2.18", gosnmp.OctetString, []byte{0x00, 0x50, 0x56, 0xab, 0xcd, 0xef}),
		createIntegerPDU("1.3.6.1.2.1.4.22.1.4.17.192.0.2.18", 3),
	})

	actual := collectTopologyProfileTables(t, mockHandler, "_std-topology-arp-mib")

	assertTableMetricsEqual(t, []ddsnmp.Metric{{
		Name:         "arp_legacy_entry",
		Value:        0,
		Tags:         map[string]string{"arp_if_index": "17", "arp_ip": "192.0.2.18", "arp_mac": "005056abcdef", "arp_type": "dynamic"},
		MetricType:   "gauge",
		IsTable:      true,
		Table:        "ipNetToMediaTable",
		TopologyKind: ddsnmp.KindArpLegacyEntry,
	}}, actual)
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
			Value:        0,
			Tags:         map[string]string{"lldp_loc_mgmt_addr_subtype": "1", "lldp_loc_mgmt_addr": "0a000001", "lldp_loc_mgmt_addr_if_subtype": "2", "lldp_loc_mgmt_addr_if_id": "12", "lldp_loc_mgmt_addr_oid": "0.0"},
			MetricType:   "gauge",
			IsTable:      true,
			Table:        "lldpLocManAddrTable",
			TopologyKind: ddsnmp.KindLldpLocManAddr,
		},
		{
			Name:         "lldp_loc_man_addr",
			Value:        0,
			Tags:         map[string]string{"lldp_loc_mgmt_addr_subtype": "6", "lldp_loc_mgmt_addr": "005056abcdef", "lldp_loc_mgmt_addr_if_subtype": "2", "lldp_loc_mgmt_addr_if_id": "12", "lldp_loc_mgmt_addr_oid": "0.0"},
			MetricType:   "gauge",
			IsTable:      true,
			Table:        "lldpLocManAddrTable",
			TopologyKind: ddsnmp.KindLldpLocManAddr,
		},
	}, actual)
}

func TestTopologyProfile_LLDPRemoteManagementAddressUsesIndexFields(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.0.8802.1.1.2.1.3.7", nil)
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.0.8802.1.1.2.1.3.8", nil)
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.0.8802.1.1.2.1.4.1", nil)

	ipv4Index := "0.17.42.1.4.192.0.2.2"
	ipv6Index := "0.18.43.2.16.32.1.13.184.0.0.0.0.0.0.0.0.0.0.0.2"
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.0.8802.1.1.2.1.4.2", []gosnmp.SnmpPDU{
		createIntegerPDU("1.0.8802.1.1.2.1.4.2.1.3."+ipv4Index, 2),
		createIntegerPDU("1.0.8802.1.1.2.1.4.2.1.4."+ipv4Index, 12),
		createPDU("1.0.8802.1.1.2.1.4.2.1.5."+ipv4Index, gosnmp.ObjectIdentifier, "0.0"),
		createIntegerPDU("1.0.8802.1.1.2.1.4.2.1.3."+ipv6Index, 2),
		createIntegerPDU("1.0.8802.1.1.2.1.4.2.1.4."+ipv6Index, 13),
		createPDU("1.0.8802.1.1.2.1.4.2.1.5."+ipv6Index, gosnmp.ObjectIdentifier, "0.0"),
	})

	actual := collectTopologyProfileTables(t, mockHandler, "_std-topology-lldp-mib")

	assertTableMetricsEqual(t, []ddsnmp.Metric{
		{
			Name:         "lldp_rem_man_addr",
			Value:        0,
			Tags:         map[string]string{"lldp_loc_port_num": "17", "lldp_rem_index": "42", "lldp_rem_mgmt_addr_subtype": "1", "lldp_rem_mgmt_addr": "c0000202", "lldp_rem_mgmt_addr_len": "4", "lldp_rem_mgmt_addr_if_subtype": "2", "lldp_rem_mgmt_addr_if_id": "12", "lldp_rem_mgmt_addr_oid": "0.0"},
			MetricType:   "gauge",
			IsTable:      true,
			Table:        "lldpRemManAddrTable",
			TopologyKind: ddsnmp.KindLldpRemManAddr,
		},
		{
			Name:         "lldp_rem_man_addr",
			Value:        0,
			Tags:         map[string]string{"lldp_loc_port_num": "18", "lldp_rem_index": "43", "lldp_rem_mgmt_addr_subtype": "2", "lldp_rem_mgmt_addr": "20010db8000000000000000000000002", "lldp_rem_mgmt_addr_len": "16", "lldp_rem_mgmt_addr_if_subtype": "2", "lldp_rem_mgmt_addr_if_id": "13", "lldp_rem_mgmt_addr_oid": "0.0"},
			MetricType:   "gauge",
			IsTable:      true,
			Table:        "lldpRemManAddrTable",
			TopologyKind: ddsnmp.KindLldpRemManAddr,
		},
	}, actual)
}

func TestTopologyProfile_LLDPRemoteManagementAddressOmitsMalformedIndexTag(t *testing.T) {
	tests := map[string]struct {
		index string
	}{
		"missing address":       {index: "0.17.42.1.0"},
		"invalid address octet": {index: "0.17.42.1.1.999"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl, mockHandler := setupMockHandler(t)
			defer ctrl.Finish()

			expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.0.8802.1.1.2.1.3.7", nil)
			expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.0.8802.1.1.2.1.3.8", nil)
			expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.0.8802.1.1.2.1.4.1", nil)
			expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.0.8802.1.1.2.1.4.2", []gosnmp.SnmpPDU{
				createIntegerPDU("1.0.8802.1.1.2.1.4.2.1.3."+tc.index, 2),
			})

			actual := collectTopologyProfileTables(t, mockHandler, "_std-topology-lldp-mib")
			require.Len(t, actual, 1)
			require.NotContains(t, actual[0].Tags, "lldp_rem_mgmt_addr")
		})
	}
}

func TestTopologyProfile_LLDPV2UsesAccessibleColumnsAndStableIndexes(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.111.2.802.1.1.13.1.3.7", []gosnmp.SnmpPDU{
		createIntegerPDU("1.3.111.2.802.1.1.13.1.3.7.1.2.17", 5),
		createStringPDU("1.3.111.2.802.1.1.13.1.3.7.1.3.17", "ethernet1/1"),
		createStringPDU("1.3.111.2.802.1.1.13.1.3.7.1.4.17", "fixture uplink"),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.111.2.802.1.1.13.1.3.8", []gosnmp.SnmpPDU{
		createIntegerPDU("1.3.111.2.802.1.1.13.1.3.8.1.3.1.4.192.0.2.17", 5),
		createIntegerPDU("1.3.111.2.802.1.1.13.1.3.8.1.4.1.4.192.0.2.17", 2),
		createIntegerPDU("1.3.111.2.802.1.1.13.1.3.8.1.5.1.4.192.0.2.17", 17),
		createPDU("1.3.111.2.802.1.1.13.1.3.8.1.6.1.4.192.0.2.17", gosnmp.ObjectIdentifier, "0.0"),
		createIntegerPDU("1.3.111.2.802.1.1.13.1.3.8.1.3.2.16.32.1.13.184.0.0.0.0.0.0.0.0.0.0.0.17", 17),
		createIntegerPDU("1.3.111.2.802.1.1.13.1.3.8.1.4.2.16.32.1.13.184.0.0.0.0.0.0.0.0.0.0.0.17", 2),
		createIntegerPDU("1.3.111.2.802.1.1.13.1.3.8.1.5.2.16.32.1.13.184.0.0.0.0.0.0.0.0.0.0.0.17", 18),
		createPDU("1.3.111.2.802.1.1.13.1.3.8.1.6.2.16.32.1.13.184.0.0.0.0.0.0.0.0.0.0.0.17", gosnmp.ObjectIdentifier, "0.0"),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.111.2.802.1.1.13.1.4.1", []gosnmp.SnmpPDU{
		createIntegerPDU("1.3.111.2.802.1.1.13.1.4.1.1.5.0.17.1.1", 4),
		createStringPDU("1.3.111.2.802.1.1.13.1.4.1.1.6.0.17.1.1", "02:00:00:00:00:01"),
		createIntegerPDU("1.3.111.2.802.1.1.13.1.4.1.1.7.0.17.1.1", 5),
		createStringPDU("1.3.111.2.802.1.1.13.1.4.1.1.8.0.17.1.1", "ethernet1/1"),
		createStringPDU("1.3.111.2.802.1.1.13.1.4.1.1.10.0.17.1.1", "fixture-neighbor-a"),
		createIntegerPDU("1.3.111.2.802.1.1.13.1.4.1.1.5.0.17.2.1", 4),
		createStringPDU("1.3.111.2.802.1.1.13.1.4.1.1.6.0.17.2.1", "02:00:00:00:00:02"),
		createIntegerPDU("1.3.111.2.802.1.1.13.1.4.1.1.7.0.17.2.1", 5),
		createStringPDU("1.3.111.2.802.1.1.13.1.4.1.1.8.0.17.2.1", "ethernet1/2"),
		createStringPDU("1.3.111.2.802.1.1.13.1.4.1.1.10.0.17.2.1", "fixture-neighbor-b"),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.111.2.802.1.1.13.1.4.2", []gosnmp.SnmpPDU{
		createIntegerPDU("1.3.111.2.802.1.1.13.1.4.2.1.3.0.17.1.1.1.4.192.0.2.21", 2),
		createIntegerPDU("1.3.111.2.802.1.1.13.1.4.2.1.4.0.17.1.1.1.4.192.0.2.21", 17),
		createPDU("1.3.111.2.802.1.1.13.1.4.2.1.5.0.17.1.1.1.4.192.0.2.21", gosnmp.ObjectIdentifier, "0.0"),
		createIntegerPDU("1.3.111.2.802.1.1.13.1.4.2.1.3.0.17.2.1.2.16.32.1.13.184.0.0.0.0.0.0.0.0.0.0.0.33", 2),
		createIntegerPDU("1.3.111.2.802.1.1.13.1.4.2.1.4.0.17.2.1.2.16.32.1.13.184.0.0.0.0.0.0.0.0.0.0.0.33", 18),
		createPDU("1.3.111.2.802.1.1.13.1.4.2.1.5.0.17.2.1.2.16.32.1.13.184.0.0.0.0.0.0.0.0.0.0.0.33", gosnmp.ObjectIdentifier, "0.0"),
	})

	actual := collectTopologyProfileTables(t, mockHandler, "_std-topology-lldp-v2-mib")

	assertTableMetricsEqual(t, []ddsnmp.Metric{
		{
			Name:         "lldp_loc_port",
			Tags:         map[string]string{"lldp_loc_port_num": "17", "lldp_loc_port_id": "ethernet1/1", "lldp_loc_port_id_subtype": "interfaceName", "lldp_loc_port_desc": "fixture uplink"},
			MetricType:   "gauge",
			IsTable:      true,
			Table:        "lldpV2LocPortTable",
			TopologyKind: ddsnmp.KindLldpLocPort,
		},
		{
			Name:         "lldp_loc_man_addr",
			Tags:         map[string]string{"lldp_loc_mgmt_addr_subtype": "1", "lldp_loc_mgmt_addr": "c0000211", "lldp_loc_mgmt_addr_if_subtype": "2", "lldp_loc_mgmt_addr_if_id": "17", "lldp_loc_mgmt_addr_oid": "0.0"},
			MetricType:   "gauge",
			IsTable:      true,
			Table:        "lldpV2LocManAddrTable",
			TopologyKind: ddsnmp.KindLldpLocManAddr,
		},
		{
			Name:         "lldp_loc_man_addr",
			Tags:         map[string]string{"lldp_loc_mgmt_addr_subtype": "2", "lldp_loc_mgmt_addr": "20010db8000000000000000000000011", "lldp_loc_mgmt_addr_if_subtype": "2", "lldp_loc_mgmt_addr_if_id": "18", "lldp_loc_mgmt_addr_oid": "0.0"},
			MetricType:   "gauge",
			IsTable:      true,
			Table:        "lldpV2LocManAddrTable",
			TopologyKind: ddsnmp.KindLldpLocManAddr,
		},
		{
			Name:         "lldp_rem",
			Tags:         map[string]string{"lldp_loc_port_num": "17", "lldp_rem_index": "1.1", "lldp_rem_chassis_id_subtype": "macAddress", "lldp_rem_chassis_id": "02:00:00:00:00:01", "lldp_rem_port_id_subtype": "interfaceName", "lldp_rem_port_id": "ethernet1/1", "lldp_rem_sys_name": "fixture-neighbor-a"},
			MetricType:   "gauge",
			IsTable:      true,
			Table:        "lldpV2RemTable",
			TopologyKind: ddsnmp.KindLldpRem,
		},
		{
			Name:         "lldp_rem",
			Tags:         map[string]string{"lldp_loc_port_num": "17", "lldp_rem_index": "2.1", "lldp_rem_chassis_id_subtype": "macAddress", "lldp_rem_chassis_id": "02:00:00:00:00:02", "lldp_rem_port_id_subtype": "interfaceName", "lldp_rem_port_id": "ethernet1/2", "lldp_rem_sys_name": "fixture-neighbor-b"},
			MetricType:   "gauge",
			IsTable:      true,
			Table:        "lldpV2RemTable",
			TopologyKind: ddsnmp.KindLldpRem,
		},
		{
			Name:         "lldp_rem_man_addr",
			Tags:         map[string]string{"lldp_loc_port_num": "17", "lldp_rem_index": "1.1", "lldp_rem_mgmt_addr_subtype": "1", "lldp_rem_mgmt_addr_len": "4", "lldp_rem_mgmt_addr": "c0000215", "lldp_rem_mgmt_addr_if_subtype": "2", "lldp_rem_mgmt_addr_if_id": "17", "lldp_rem_mgmt_addr_oid": "0.0"},
			MetricType:   "gauge",
			IsTable:      true,
			Table:        "lldpV2RemManAddrTable",
			TopologyKind: ddsnmp.KindLldpRemManAddr,
		},
		{
			Name:         "lldp_rem_man_addr",
			Tags:         map[string]string{"lldp_loc_port_num": "17", "lldp_rem_index": "2.1", "lldp_rem_mgmt_addr_subtype": "2", "lldp_rem_mgmt_addr_len": "16", "lldp_rem_mgmt_addr": "20010db8000000000000000000000021", "lldp_rem_mgmt_addr_if_subtype": "2", "lldp_rem_mgmt_addr_if_id": "18", "lldp_rem_mgmt_addr_oid": "0.0"},
			MetricType:   "gauge",
			IsTable:      true,
			Table:        "lldpV2RemManAddrTable",
			TopologyKind: ddsnmp.KindLldpRemManAddr,
		},
	}, actual)
}

func TestTopologyProfile_MikroTikRB750Gr3OverridesLLDPRemoteManagementAddressRow(t *testing.T) {
	profile, err := ddsnmp.LoadProfileByName("topology-role-mikrotik-rb750gr3")
	require.NoError(t, err)

	var rows []ddprofiledefinition.TopologyConfig
	for _, row := range profile.Definition.Topology {
		if row.Table.Name == "lldpRemManAddrTable" {
			rows = append(rows, row)
		}
	}
	require.Len(t, rows, 1)

	row := rows[0]
	require.Equal(t, ddsnmp.KindLldpRemManAddr, row.Kind)
	require.Len(t, row.Symbols, 1)
	require.Equal(t, "lldp_rem_man_addr", row.Symbols[0].Name)
	require.Equal(t, "1.0.8802.1.1.2.1.4.2.1.1", row.Symbols[0].OID)

	tags := make(map[string]ddprofiledefinition.MetricTagConfig, len(row.MetricTags))
	for _, tag := range row.MetricTags {
		tags[tag.Tag] = tag
	}
	require.Equal(t, "1.0.8802.1.1.2.1.4.2.1.1", tags["lldp_rem_mgmt_addr_subtype"].Symbol.OID)
	require.Equal(t, "1.0.8802.1.1.2.1.4.2.1.2", tags["lldp_rem_mgmt_addr"].Symbol.OID)
	require.Equal(t, uint(5), tags["lldp_rem_mgmt_addr_len"].Index)
	require.Empty(t, tags["lldp_rem_mgmt_addr"].IndexTransform)
}

func TestTopologyProfile_OSPFNeighborUsesNeighborTags(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	index := "198.51.100.2.0"
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.14.10", []gosnmp.SnmpPDU{
		createPDU("1.3.6.1.2.1.14.10.1.1."+index, gosnmp.IPAddress, "198.51.100.2"),
		createIntegerPDU("1.3.6.1.2.1.14.10.1.2."+index, 0),
		createPDU("1.3.6.1.2.1.14.10.1.3."+index, gosnmp.IPAddress, "2.2.2.2"),
		createIntegerPDU("1.3.6.1.2.1.14.10.1.6."+index, 8),
	})

	actual := collectTopologyProfileTables(t, mockHandler, "_std-ospf-mib")

	assertTableMetricsEqual(t, []ddsnmp.Metric{
		{
			Name:         "ospf_neighbor",
			Value:        0,
			Tags:         map[string]string{"ospf_neighbor_ip": "198.51.100.2", "ospf_neighbor_addressless_index": "0", "ospf_neighbor_router_id": "2.2.2.2", "ospf_neighbor_state": "full"},
			MetricType:   "gauge",
			IsTable:      true,
			Table:        "ospfNbrTable",
			TopologyKind: ddsnmp.KindOSPFNeighbor,
		},
	}, actual)
}

func TestTopologyProfile_OSPFTopologyExcludesVirtualNeighborTable(t *testing.T) {
	profile, err := ddsnmp.LoadProfileByName("_std-ospf-mib")
	require.NoError(t, err)
	require.NotNil(t, profile.Definition)
	require.Len(t, profile.Definition.Topology, 1)
	require.Equal(t, ddsnmp.KindOSPFNeighbor, profile.Definition.Topology[0].Kind)
	require.Equal(t, "ospfNbrTable", profile.Definition.Topology[0].Table.Name)
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
		Value:        0,
		Tags:         map[string]string{"dot1q_fdb_id": "7", "dot1q_fdb_mac": "00:50:56:ab:cd:ef", "dot1q_fdb_bridge_port": "5", "dot1q_fdb_status": "learned"},
		MetricType:   "gauge",
		IsTable:      true,
		Table:        "dot1qTpFdbTable",
		TopologyKind: ddsnmp.KindQbridgeFdbEntry,
	}
}
