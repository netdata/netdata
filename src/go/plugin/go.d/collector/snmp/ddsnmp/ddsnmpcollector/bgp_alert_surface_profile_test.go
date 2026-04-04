// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"strings"
	"testing"

	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestCollector_Collect_BGPAlertSurface_FromMatchedProfiles(t *testing.T) {
	type scenario struct {
		name        string
		sysObjectID string
		profileFile string
		tables      map[string][]string
		virtuals    []string
		walks       func(t *testing.T, p *ddsnmp.Profile) map[string][]gosnmp.SnmpPDU
		assertions  func(t *testing.T, metrics []ddsnmp.Metric)
	}

	tests := []scenario{
		{
			name:        "generic BGP4-MIB via NEC profile emits the stock contract",
			sysObjectID: "1.3.6.1.4.1.119.1.84.18",
			profileFile: "nec-univerge.yaml",
			tables: map[string][]string{
				"bgpPeerTable": {"bgpPeerAdminStatus", "bgpPeerState", "bgpPeerInUpdates", "bgpPeerOutUpdates", "bgpPeerFsmEstablishedTransitions"},
			},
			virtuals: []string{"bgpPeerAvailability", "bgpPeerUpdates"},
			walks: func(t *testing.T, p *ddsnmp.Profile) map[string][]gosnmp.SnmpPDU {
				idx := "192.0.2.10"
				return map[string][]gosnmp.SnmpPDU{
					"bgpPeerTable": {
						createIntegerPDU(oidWithIndex(requireSymbolOID(t, p, "bgpPeerTable", "bgpPeerAdminStatus"), idx), 2),
						createIntegerPDU(oidWithIndex(requireSymbolOID(t, p, "bgpPeerTable", "bgpPeerState"), idx), 6),
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "bgpPeerTable", "bgpPeerInUpdates"), idx), 11),
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "bgpPeerTable", "bgpPeerOutUpdates"), idx), 22),
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "bgpPeerTable", "bgpPeerFsmEstablishedTransitions"), idx), 3),
						createGauge32PDU(oidWithIndex(requireMetricTagOID(t, p, "bgpPeerTable", "remote_as"), idx), 64512),
						createPDU(oidWithIndex(requireMetricTagOID(t, p, "bgpPeerTable", "neighbor"), idx), gosnmp.IPAddress, "192.0.2.10"),
						createIntegerPDU(oidWithIndex(requireMetricTagOID(t, p, "bgpPeerTable", "bgp_version"), idx), 4),
						createPDU(oidWithIndex(requireMetricTagOID(t, p, "bgpPeerTable", "local_address"), idx), gosnmp.IPAddress, "198.51.100.1"),
						createPDU(oidWithIndex(requireMetricTagOID(t, p, "bgpPeerTable", "peer_identifier"), idx), gosnmp.IPAddress, "203.0.113.10"),
					},
				}
			},
			assertions: func(t *testing.T, metrics []ddsnmp.Metric) {
				availability := requireMetricWithTags(t, metrics, "bgpPeerAvailability", map[string]string{
					"neighbor":  "192.0.2.10",
					"remote_as": "64512",
				})
				assert.Equal(t, map[string]int64{"admin_enabled": 1, "established": 1}, availability.MultiValue)

				updates := requireMetricWithTags(t, metrics, "bgpPeerUpdates", map[string]string{
					"neighbor":  "192.0.2.10",
					"remote_as": "64512",
				})
				assert.Equal(t, map[string]int64{"received": 11, "sent": 22}, updates.MultiValue)

				transitions := requireMetricWithTags(t, metrics, "bgpPeerFsmEstablishedTransitions", map[string]string{
					"neighbor":  "192.0.2.10",
					"remote_as": "64512",
				})
				assert.EqualValues(t, 3, transitions.Value)
			},
		},
		{
			name:        "Cumulus switch emits the generic stock contract using standard BGP4-MIB rows",
			sysObjectID: "1.3.6.1.4.1.40310",
			profileFile: "nvidia-cumulus-linux-switch.yaml",
			tables: map[string][]string{
				"bgpPeerTable": {"bgpPeerAdminStatus", "bgpPeerState", "bgpPeerInUpdates", "bgpPeerOutUpdates", "bgpPeerFsmEstablishedTransitions"},
			},
			virtuals: []string{"bgpPeerAvailability", "bgpPeerUpdates"},
			walks: func(t *testing.T, p *ddsnmp.Profile) map[string][]gosnmp.SnmpPDU {
				// Adapted from LibreNMS tests/snmpsim/pfsense_frr-bgp.snmprec for the
				// generic BGP4-MIB row values. The Cumulus LibreNMS fixture carries the
				// Cumulus sysObjectID, but not peer-table rows.
				idx := "169.254.1.1"
				return map[string][]gosnmp.SnmpPDU{
					"bgpPeerTable": {
						createIntegerPDU(oidWithIndex(requireSymbolOID(t, p, "bgpPeerTable", "bgpPeerAdminStatus"), idx), 2),
						createIntegerPDU(oidWithIndex(requireSymbolOID(t, p, "bgpPeerTable", "bgpPeerState"), idx), 6),
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "bgpPeerTable", "bgpPeerInUpdates"), idx), 6),
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "bgpPeerTable", "bgpPeerOutUpdates"), idx), 14),
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "bgpPeerTable", "bgpPeerFsmEstablishedTransitions"), idx), 3),
						createGauge32PDU(oidWithIndex(requireMetricTagOID(t, p, "bgpPeerTable", "remote_as"), idx), 4200000000),
						createPDU(oidWithIndex(requireMetricTagOID(t, p, "bgpPeerTable", "neighbor"), idx), gosnmp.IPAddress, "169.254.1.1"),
						createIntegerPDU(oidWithIndex(requireMetricTagOID(t, p, "bgpPeerTable", "bgp_version"), idx), 4),
						createPDU(oidWithIndex(requireMetricTagOID(t, p, "bgpPeerTable", "local_address"), idx), gosnmp.IPAddress, "169.254.1.2"),
						createPDU(oidWithIndex(requireMetricTagOID(t, p, "bgpPeerTable", "peer_identifier"), idx), gosnmp.IPAddress, "169.254.1.1"),
					},
				}
			},
			assertions: func(t *testing.T, metrics []ddsnmp.Metric) {
				availability := requireMetricWithTags(t, metrics, "bgpPeerAvailability", map[string]string{
					"neighbor":  "169.254.1.1",
					"remote_as": "4200000000",
				})
				assert.Equal(t, map[string]int64{"admin_enabled": 1, "established": 1}, availability.MultiValue)

				updates := requireMetricWithTags(t, metrics, "bgpPeerUpdates", map[string]string{
					"neighbor":  "169.254.1.1",
					"remote_as": "4200000000",
				})
				assert.Equal(t, map[string]int64{"received": 6, "sent": 14}, updates.MultiValue)

				transitions := requireMetricWithTags(t, metrics, "bgpPeerFsmEstablishedTransitions", map[string]string{
					"neighbor":  "169.254.1.1",
					"remote_as": "4200000000",
				})
				assert.EqualValues(t, 3, transitions.Value)
			},
		},
		{
			name:        "Cisco ASR emits the common contract and gauge-style accepted prefixes",
			sysObjectID: "1.3.6.1.4.1.9.1.923",
			profileFile: "cisco-asr.yaml",
			tables: map[string][]string{
				"cbgpPeer3Table":                 {"bgpPeerAdminStatus", "bgpPeerState", "bgpPeerInUpdates", "bgpPeerOutUpdates", "bgpPeerFsmEstablishedTransitions"},
				"cbgpPeer2AddrFamilyPrefixTable": {"bgpPeerPrefixesAccepted"},
			},
			virtuals: []string{"bgpPeerAvailability", "bgpPeerUpdates"},
			walks: func(t *testing.T, p *ddsnmp.Profile) map[string][]gosnmp.SnmpPDU {
				peer3Idx := "7.1.4.192.0.2.1"
				peer2Idx := "1.4.192.0.2.1"
				prefixIdx := peer2Idx + ".1.128"
				return map[string][]gosnmp.SnmpPDU{
					"cbgpPeer3Table": {
						createIntegerPDU(oidWithIndex(requireSymbolOID(t, p, "cbgpPeer3Table", "bgpPeerAdminStatus"), peer3Idx), 2),
						createIntegerPDU(oidWithIndex(requireSymbolOID(t, p, "cbgpPeer3Table", "bgpPeerState"), peer3Idx), 6),
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "cbgpPeer3Table", "bgpPeerInUpdates"), peer3Idx), 42),
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "cbgpPeer3Table", "bgpPeerOutUpdates"), peer3Idx), 43),
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "cbgpPeer3Table", "bgpPeerFsmEstablishedTransitions"), peer3Idx), 5),
						createStringPDU(oidWithIndex(requireMetricTagOID(t, p, "cbgpPeer3Table", "routing_instance"), peer3Idx), "blue"),
						createPDU(oidWithIndex(requireMetricTagOID(t, p, "cbgpPeer3Table", "neighbor"), peer3Idx), gosnmp.OctetString, []byte{192, 0, 2, 1}),
						createPDU(oidWithIndex(requireMetricTagOID(t, p, "cbgpPeer3Table", "local_address"), peer3Idx), gosnmp.OctetString, []byte{198, 51, 100, 1}),
						createGauge32PDU(oidWithIndex(requireMetricTagOID(t, p, "cbgpPeer3Table", "local_as"), peer3Idx), 65000),
						createGauge32PDU(oidWithIndex(requireMetricTagOID(t, p, "cbgpPeer3Table", "remote_as"), peer3Idx), 65001),
						createPDU(oidWithIndex(requireMetricTagOID(t, p, "cbgpPeer3Table", "local_identifier"), peer3Idx), gosnmp.IPAddress, "198.51.100.10"),
						createPDU(oidWithIndex(requireMetricTagOID(t, p, "cbgpPeer3Table", "peer_identifier"), peer3Idx), gosnmp.IPAddress, "203.0.113.10"),
						createIntegerPDU(oidWithIndex(requireMetricTagOID(t, p, "cbgpPeer3Table", "bgp_version"), peer3Idx), 4),
					},
					"cbgpPeer2AddrFamilyPrefixTable": {
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "cbgpPeer2AddrFamilyPrefixTable", "bgpPeerPrefixesAccepted"), prefixIdx), 1200),
					},
					"cbgpPeer2Table": {
						createPDU(oidWithIndex(requireCrossTableTagOID(t, p, "cbgpPeer2AddrFamilyPrefixTable", "local_address"), peer2Idx), gosnmp.OctetString, []byte{198, 51, 100, 1}),
						createGauge32PDU(oidWithIndex(requireCrossTableTagOID(t, p, "cbgpPeer2AddrFamilyPrefixTable", "local_as"), peer2Idx), 65000),
						createPDU(oidWithIndex(requireCrossTableTagOID(t, p, "cbgpPeer2AddrFamilyPrefixTable", "local_identifier"), peer2Idx), gosnmp.IPAddress, "198.51.100.10"),
						createIntegerPDU(oidWithIndex(requireCrossTableTagOID(t, p, "cbgpPeer2AddrFamilyPrefixTable", "bgp_version"), peer2Idx), 4),
						createGauge32PDU(oidWithIndex(requireCrossTableTagOID(t, p, "cbgpPeer2AddrFamilyPrefixTable", "remote_as"), peer2Idx), 65001),
						createPDU(oidWithIndex(requireCrossTableTagOID(t, p, "cbgpPeer2AddrFamilyPrefixTable", "peer_identifier"), peer2Idx), gosnmp.IPAddress, "203.0.113.10"),
					},
					"cbgpPeer2AddrFamilyTable": {
						createStringPDU(oidWithIndex(requireCrossTableTagOID(t, p, "cbgpPeer2AddrFamilyPrefixTable", "address_family_name"), prefixIdx), "vpnv4 unicast"),
					},
				}
			},
			assertions: func(t *testing.T, metrics []ddsnmp.Metric) {
				availability := requireMetricWithTags(t, metrics, "bgpPeerAvailability", map[string]string{
					"routing_instance": "blue",
					"neighbor":         "192.0.2.1",
					"remote_as":        "65001",
				})
				assert.Equal(t, map[string]int64{"admin_enabled": 1, "established": 1}, availability.MultiValue)

				updates := requireMetricWithTags(t, metrics, "bgpPeerUpdates", map[string]string{
					"routing_instance": "blue",
					"neighbor":         "192.0.2.1",
				})
				assert.Equal(t, map[string]int64{"received": 42, "sent": 43}, updates.MultiValue)

				transitions := requireMetricWithTags(t, metrics, "bgpPeerFsmEstablishedTransitions", map[string]string{
					"routing_instance": "blue",
					"neighbor":         "192.0.2.1",
				})
				assert.EqualValues(t, 5, transitions.Value)

				accepted := requireMetricWithTags(t, metrics, "bgpPeerPrefixesAccepted", map[string]string{
					"neighbor":                  "192.0.2.1",
					"address_family":            "ipv4",
					"subsequent_address_family": "vpn",
				})
				assert.EqualValues(t, 1200, accepted.Value)
				assert.Equal(t, "vpnv4 unicast", accepted.Tags["address_family_name"])
			},
		},
		{
			name:        "Juniper MX emits the common contract and accepted prefixes",
			sysObjectID: "1.3.6.1.4.1.2636.1.1.1.2.21",
			profileFile: "juniper-mx.yaml",
			tables: map[string][]string{
				"jnxBgpM2PeerTable":           {"bgpPeerAdminStatus", "bgpPeerState"},
				"jnxBgpM2PeerCountersTable":   {"bgpPeerInUpdates", "bgpPeerOutUpdates", "bgpPeerFsmEstablishedTransitions"},
				"jnxBgpM2PrefixCountersTable": {"bgpPeerPrefixesAccepted"},
			},
			virtuals: []string{"bgpPeerAvailability", "bgpPeerUpdates"},
			walks: func(t *testing.T, p *ddsnmp.Profile) map[string][]gosnmp.SnmpPDU {
				peerIdx := "101"
				prefixIdx := "101.1.128"
				return map[string][]gosnmp.SnmpPDU{
					"jnxBgpM2PeerTable": {
						createIntegerPDU(oidWithIndex(requireSymbolOID(t, p, "jnxBgpM2PeerTable", "bgpPeerAdminStatus"), peerIdx), 2),
						createIntegerPDU(oidWithIndex(requireSymbolOID(t, p, "jnxBgpM2PeerTable", "bgpPeerState"), peerIdx), 6),
						createPDU(oidWithIndex(requireMetricTagOID(t, p, "jnxBgpM2PeerTable", "routing_instance"), peerIdx), gosnmp.OctetString, []byte("blue")),
						createIntegerPDU(oidWithIndex(requireMetricTagOID(t, p, "jnxBgpM2PeerTable", "neighbor_address_type"), peerIdx), 1),
						createPDU(oidWithIndex(requireMetricTagOID(t, p, "jnxBgpM2PeerTable", "neighbor"), peerIdx), gosnmp.OctetString, []byte{192, 0, 2, 11}),
						createPDU(oidWithIndex(requireMetricTagOID(t, p, "jnxBgpM2PeerTable", "local_address"), peerIdx), gosnmp.OctetString, []byte{198, 51, 100, 11}),
						createGauge32PDU(oidWithIndex(requireMetricTagOID(t, p, "jnxBgpM2PeerTable", "local_as"), peerIdx), 65000),
						createGauge32PDU(oidWithIndex(requireMetricTagOID(t, p, "jnxBgpM2PeerTable", "remote_as"), peerIdx), 65001),
						createPDU(oidWithIndex(requireMetricTagOID(t, p, "jnxBgpM2PeerTable", "peer_identifier"), peerIdx), gosnmp.OctetString, []byte{203, 0, 113, 11}),
						createIntegerPDU(oidWithIndex(requireMetricTagOID(t, p, "jnxBgpM2PeerTable", "bgp_version"), peerIdx), 4),
						createGauge32PDU(oidWithIndex(requireCrossTableLookupOID(t, p, "jnxBgpM2PrefixCountersTable", "routing_instance"), peerIdx), 101),
					},
					"jnxBgpM2PeerCountersTable": {
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "jnxBgpM2PeerCountersTable", "bgpPeerInUpdates"), peerIdx), 14),
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "jnxBgpM2PeerCountersTable", "bgpPeerOutUpdates"), peerIdx), 28),
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "jnxBgpM2PeerCountersTable", "bgpPeerFsmEstablishedTransitions"), peerIdx), 4),
					},
					"jnxBgpM2PrefixCountersTable": {
						createGauge32PDU(oidWithIndex(requireSymbolOID(t, p, "jnxBgpM2PrefixCountersTable", "bgpPeerPrefixesAccepted"), prefixIdx), 1200),
					},
				}
			},
			assertions: func(t *testing.T, metrics []ddsnmp.Metric) {
				availability := requireMetricWithTags(t, metrics, "bgpPeerAvailability", map[string]string{
					"routing_instance": "blue",
					"neighbor":         "192.0.2.11",
					"remote_as":        "65001",
				})
				assert.Equal(t, map[string]int64{"admin_enabled": 1, "established": 1}, availability.MultiValue)

				updates := requireMetricWithTags(t, metrics, "bgpPeerUpdates", map[string]string{
					"routing_instance": "blue",
					"neighbor":         "192.0.2.11",
				})
				assert.Equal(t, map[string]int64{"received": 14, "sent": 28}, updates.MultiValue)

				transitions := requireMetricWithTags(t, metrics, "bgpPeerFsmEstablishedTransitions", map[string]string{
					"routing_instance": "blue",
					"neighbor":         "192.0.2.11",
				})
				assert.EqualValues(t, 4, transitions.Value)

				accepted := requireMetricWithTags(t, metrics, "bgpPeerPrefixesAccepted", map[string]string{
					"neighbor":                  "192.0.2.11",
					"address_family":            "ipv4",
					"subsequent_address_family": "vpn",
				})
				assert.EqualValues(t, 1200, accepted.Value)
			},
		},
		{
			name:        "Nokia SR OS emits the common contract from TiMOS peer tables and keeps family gauges labeled",
			sysObjectID: "1.3.6.1.4.1.6527.1.3.17",
			profileFile: "nokia-service-router-os.yaml",
			tables: map[string][]string{
				"tBgpPeerNgTable":     {"bgpPeerAdminStatus", "bgpPeerState"},
				"tBgpPeerNgOperTable": {"bgpPeerInUpdates", "bgpPeerOutUpdates", "bgpPeerFsmEstablishedTime", "bgpPeerPrefixesReceivedTotal", "bgpPeerPrefixesActive", "bgpPeerPrefixesRejectedTotal"},
			},
			virtuals: []string{"bgpPeerAvailability", "bgpPeerUpdates"},
			walks: func(t *testing.T, p *ddsnmp.Profile) map[string][]gosnmp.SnmpPDU {
				idx := "2.1.4.192.0.2.21"
				return map[string][]gosnmp.SnmpPDU{
					"tBgpPeerNgTable": {
						createIntegerPDU(oidWithIndex(requireSymbolOID(t, p, "tBgpPeerNgTable", "bgpPeerAdminStatus"), idx), 2),
						createIntegerPDU(oidWithIndex(requireSymbolOID(t, p, "tBgpPeerNgTable", "bgpPeerState"), idx), 6),
						createPDU(oidWithIndex(requireMetricTagOID(t, p, "tBgpPeerNgTable", "local_address"), idx), gosnmp.OctetString, []byte{198, 51, 100, 21}),
						createGauge32PDU(oidWithIndex(requireMetricTagOID(t, p, "tBgpPeerNgTable", "local_as"), idx), 65000),
						createGauge32PDU(oidWithIndex(requireMetricTagOID(t, p, "tBgpPeerNgTable", "remote_as"), idx), 65001),
						createStringPDU(oidWithIndex(requireMetricTagOID(t, p, "tBgpPeerNgTable", "peer_description"), idx), "edge-peer"),
					},
					"tBgpPeerNgOperTable": {
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "tBgpPeerNgOperTable", "bgpPeerInUpdates"), idx), 42),
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "tBgpPeerNgOperTable", "bgpPeerOutUpdates"), idx), 43),
						createGauge32PDU(oidWithIndex(requireSymbolOID(t, p, "tBgpPeerNgOperTable", "bgpPeerFsmEstablishedTime"), idx), 3600),
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "tBgpPeerNgOperTable", "bgpPeerPrefixesReceivedTotal"), idx), 120),
						createGauge32PDU(oidWithIndex(requireSymbolOID(t, p, "tBgpPeerNgOperTable", "bgpPeerPrefixesActive"), idx), 110),
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "tBgpPeerNgOperTable", "bgpPeerPrefixesRejectedTotal"), idx), 2),
					},
					"vRtrConfTable": {
						createStringPDU(oidWithIndex(requireCrossTableTagOID(t, p, "tBgpPeerNgTable", "routing_instance"), "2"), "vprn111"),
					},
				}
			},
			assertions: func(t *testing.T, metrics []ddsnmp.Metric) {
				availability := requireMetricWithTags(t, metrics, "bgpPeerAvailability", map[string]string{
					"routing_instance": "vprn111",
					"neighbor":         "192.0.2.21",
					"remote_as":        "65001",
				})
				assert.Equal(t, map[string]int64{"admin_enabled": 1, "established": 1}, availability.MultiValue)
				assert.Equal(t, "edge-peer", availability.Tags["peer_description"])

				updates := requireMetricWithTags(t, metrics, "bgpPeerUpdates", map[string]string{
					"routing_instance": "vprn111",
					"neighbor":         "192.0.2.21",
				})
				assert.Equal(t, map[string]int64{"received": 42, "sent": 43}, updates.MultiValue)
				assert.Equal(t, "198.51.100.21", updates.Tags["local_address"])

				establishedTime := requireMetricWithTags(t, metrics, "bgpPeerFsmEstablishedTime", map[string]string{
					"routing_instance": "vprn111",
					"neighbor":         "192.0.2.21",
				})
				assert.EqualValues(t, 3600, establishedTime.Value)

				active := requireMetricWithTags(t, metrics, "bgpPeerPrefixesActive", map[string]string{
					"routing_instance":          "vprn111",
					"neighbor":                  "192.0.2.21",
					"address_family":            "ipv4",
					"subsequent_address_family": "unicast",
				})
				assert.EqualValues(t, 110, active.Value)
				assert.Equal(t, "edge-peer", active.Tags["peer_description"])

				rejected := requireMetricWithTags(t, metrics, "bgpPeerPrefixesRejectedTotal", map[string]string{
					"routing_instance":          "vprn111",
					"neighbor":                  "192.0.2.21",
					"address_family":            "ipv4",
					"subsequent_address_family": "unicast",
				})
				assert.EqualValues(t, 2, rejected.Value)
			},
		},
		{
			name:        "Arista switch emits the common contract and accepted prefixes with a neighbor label derived from the row index",
			sysObjectID: "1.3.6.1.4.1.30065.1.3011.7010.427.48",
			profileFile: "arista-switch.yaml",
			tables: map[string][]string{
				"aristaBgp4V2PeerTable":         {"aristaBgp4V2PeerAdminStatus", "aristaBgp4V2PeerState"},
				"aristaBgp4V2PeerCountersTable": {"bgpPeerInUpdates", "bgpPeerOutUpdates", "bgpPeerFsmEstablishedTransitions"},
				"aristaBgp4V2PrefixGaugesTable": {"bgpPeerPrefixesAccepted"},
			},
			virtuals: []string{"bgpPeerAvailability", "bgpPeerUpdates"},
			walks: func(t *testing.T, p *ddsnmp.Profile) map[string][]gosnmp.SnmpPDU {
				idx := "7.1.4.192.0.2.12"
				prefixIdx := idx + ".1.128"
				return map[string][]gosnmp.SnmpPDU{
					"aristaBgp4V2PeerTable": {
						createIntegerPDU(oidWithIndex(requireSymbolOID(t, p, "aristaBgp4V2PeerTable", "aristaBgp4V2PeerAdminStatus"), idx), 2),
						createIntegerPDU(oidWithIndex(requireSymbolOID(t, p, "aristaBgp4V2PeerTable", "aristaBgp4V2PeerState"), idx), 6),
						createGauge32PDU(oidWithIndex(requireMetricTagOID(t, p, "aristaBgp4V2PeerTable", "local_as"), idx), 65000),
						createGauge32PDU(oidWithIndex(requireMetricTagOID(t, p, "aristaBgp4V2PeerTable", "remote_as"), idx), 65001),
						createPDU(oidWithIndex(requireMetricTagOID(t, p, "aristaBgp4V2PeerTable", "local_identifier"), idx), gosnmp.IPAddress, "198.51.100.12"),
						createPDU(oidWithIndex(requireMetricTagOID(t, p, "aristaBgp4V2PeerTable", "peer_identifier"), idx), gosnmp.IPAddress, "203.0.113.12"),
						createStringPDU(oidWithIndex(requireMetricTagOID(t, p, "aristaBgp4V2PeerTable", "peer_description"), idx), "upstream-a"),
						createPDU(oidWithIndex(requireMetricTagOID(t, p, "aristaBgp4V2PeerTable", "local_address"), idx), gosnmp.OctetString, []byte{198, 51, 100, 12}),
					},
					"aristaBgp4V2PeerCountersTable": {
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "aristaBgp4V2PeerCountersTable", "bgpPeerInUpdates"), idx), 16),
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "aristaBgp4V2PeerCountersTable", "bgpPeerOutUpdates"), idx), 17),
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "aristaBgp4V2PeerCountersTable", "bgpPeerFsmEstablishedTransitions"), idx), 2),
					},
					"aristaBgp4V2PrefixGaugesTable": {
						createGauge32PDU(oidWithIndex(requireSymbolOID(t, p, "aristaBgp4V2PrefixGaugesTable", "bgpPeerPrefixesAccepted"), prefixIdx), 1300),
					},
				}
			},
			assertions: func(t *testing.T, metrics []ddsnmp.Metric) {
				availability := requireMetricWithTags(t, metrics, "bgpPeerAvailability", map[string]string{
					"routing_instance": "7",
					"neighbor":         "192.0.2.12",
					"remote_as":        "65001",
				})
				assert.Equal(t, map[string]int64{"admin_enabled": 1, "established": 1}, availability.MultiValue)

				updates := requireMetricWithTags(t, metrics, "bgpPeerUpdates", map[string]string{
					"neighbor": "192.0.2.12",
				})
				assert.Equal(t, map[string]int64{"received": 16, "sent": 17}, updates.MultiValue)

				transitions := requireMetricWithTags(t, metrics, "bgpPeerFsmEstablishedTransitions", map[string]string{
					"neighbor": "192.0.2.12",
				})
				assert.EqualValues(t, 2, transitions.Value)

				accepted := requireMetricWithTags(t, metrics, "bgpPeerPrefixesAccepted", map[string]string{
					"neighbor":                  "192.0.2.12",
					"address_family":            "ipv4",
					"subsequent_address_family": "vpn",
				})
				assert.EqualValues(t, 1300, accepted.Value)
			},
		},
		{
			name:        "Dell OS10 emits the common contract and accepted prefixes with a neighbor label derived from the row index",
			sysObjectID: "1.3.6.1.4.1.674.11000.5000.100.2.1.1",
			profileFile: "dell-os10.yaml",
			tables: map[string][]string{
				"dell.os10bgp4V2PeerTable":         {"dell.os10bgp4V2PeerAdminStatus", "dell.os10bgp4V2PeerState"},
				"dell.os10bgp4V2PeerCountersTable": {"bgpPeerInUpdates", "bgpPeerOutUpdates", "bgpPeerFsmEstablishedTransitions"},
				"dell.os10bgp4V2PrefixGaugesTable": {"bgpPeerPrefixesAccepted"},
			},
			virtuals: []string{"bgpPeerAvailability", "bgpPeerUpdates"},
			walks: func(t *testing.T, p *ddsnmp.Profile) map[string][]gosnmp.SnmpPDU {
				idx := "9.1.4.192.0.2.13"
				prefixIdx := idx + ".1.128"
				return map[string][]gosnmp.SnmpPDU{
					"dell.os10bgp4V2PeerTable": {
						createIntegerPDU(oidWithIndex(requireSymbolOID(t, p, "dell.os10bgp4V2PeerTable", "dell.os10bgp4V2PeerAdminStatus"), idx), 2),
						createIntegerPDU(oidWithIndex(requireSymbolOID(t, p, "dell.os10bgp4V2PeerTable", "dell.os10bgp4V2PeerState"), idx), 6),
						createGauge32PDU(oidWithIndex(requireMetricTagOID(t, p, "dell.os10bgp4V2PeerTable", "local_as"), idx), 65010),
						createGauge32PDU(oidWithIndex(requireMetricTagOID(t, p, "dell.os10bgp4V2PeerTable", "remote_as"), idx), 65011),
						createPDU(oidWithIndex(requireMetricTagOID(t, p, "dell.os10bgp4V2PeerTable", "local_identifier"), idx), gosnmp.IPAddress, "198.51.100.13"),
						createPDU(oidWithIndex(requireMetricTagOID(t, p, "dell.os10bgp4V2PeerTable", "peer_identifier"), idx), gosnmp.IPAddress, "203.0.113.13"),
						createStringPDU(oidWithIndex(requireMetricTagOID(t, p, "dell.os10bgp4V2PeerTable", "peer_description"), idx), "upstream-b"),
						createPDU(oidWithIndex(requireMetricTagOID(t, p, "dell.os10bgp4V2PeerTable", "local_address"), idx), gosnmp.OctetString, []byte{198, 51, 100, 13}),
					},
					"dell.os10bgp4V2PeerCountersTable": {
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "dell.os10bgp4V2PeerCountersTable", "bgpPeerInUpdates"), idx), 18),
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "dell.os10bgp4V2PeerCountersTable", "bgpPeerOutUpdates"), idx), 19),
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "dell.os10bgp4V2PeerCountersTable", "bgpPeerFsmEstablishedTransitions"), idx), 6),
					},
					"dell.os10bgp4V2PrefixGaugesTable": {
						createGauge32PDU(oidWithIndex(requireSymbolOID(t, p, "dell.os10bgp4V2PrefixGaugesTable", "bgpPeerPrefixesAccepted"), prefixIdx), 1400),
					},
				}
			},
			assertions: func(t *testing.T, metrics []ddsnmp.Metric) {
				availability := requireMetricWithTags(t, metrics, "bgpPeerAvailability", map[string]string{
					"routing_instance": "9",
					"neighbor":         "192.0.2.13",
					"remote_as":        "65011",
				})
				assert.Equal(t, map[string]int64{"admin_enabled": 1, "established": 1}, availability.MultiValue)

				updates := requireMetricWithTags(t, metrics, "bgpPeerUpdates", map[string]string{
					"neighbor": "192.0.2.13",
				})
				assert.Equal(t, map[string]int64{"received": 18, "sent": 19}, updates.MultiValue)

				transitions := requireMetricWithTags(t, metrics, "bgpPeerFsmEstablishedTransitions", map[string]string{
					"neighbor": "192.0.2.13",
				})
				assert.EqualValues(t, 6, transitions.Value)

				accepted := requireMetricWithTags(t, metrics, "bgpPeerPrefixesAccepted", map[string]string{
					"neighbor":                  "192.0.2.13",
					"address_family":            "ipv4",
					"subsequent_address_family": "vpn",
				})
				assert.EqualValues(t, 1400, accepted.Value)
			},
		},
		{
			name:        "Huawei routers emit standardized alert tags and keep AFI/SAFI rows separate",
			sysObjectID: "1.3.6.1.4.1.2011.2.224.279",
			profileFile: "huawei-routers.yaml",
			tables: map[string][]string{
				"hwBgpPeerTable":        {"huawei.hwBgpPeerAdminStatus", "huawei.hwBgpPeerState", "huawei.hwBgpPeerFsmEstablishedCounter"},
				"hwBgpPeerMessageTable": {"huawei.hwBgpPeerInUpdateMsgCounter", "huawei.hwBgpPeerOutUpdateMsgCounter"},
			},
			virtuals: []string{"bgpPeerAvailability", "bgpPeerFsmEstablishedTransitions", "bgpPeerUpdates"},
			walks: func(t *testing.T, p *ddsnmp.Profile) map[string][]gosnmp.SnmpPDU {
				ipv4Idx := "65001.1.1.1"
				ipv6Idx := "65001.2.1.2"
				return map[string][]gosnmp.SnmpPDU{
					"hwBgpPeerTable": {
						createIntegerPDU(oidWithIndex(requireSymbolOID(t, p, "hwBgpPeerTable", "huawei.hwBgpPeerAdminStatus"), ipv4Idx), 2),
						createIntegerPDU(oidWithIndex(requireSymbolOID(t, p, "hwBgpPeerTable", "huawei.hwBgpPeerState"), ipv4Idx), 6),
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "hwBgpPeerTable", "huawei.hwBgpPeerFsmEstablishedCounter"), ipv4Idx), 4),
						createPDU(oidWithIndex(requireMetricTagOID(t, p, "hwBgpPeerTable", "huawei_hw_bgp_peer_remote_addr"), ipv4Idx), gosnmp.OctetString, []byte{192, 0, 2, 14}),
						createGauge32PDU(oidWithIndex(requireMetricTagOID(t, p, "hwBgpPeerTable", "_remote_as"), ipv4Idx), 65020),
						createIntegerPDU(oidWithIndex(requireMetricTagOID(t, p, "hwBgpPeerTable", "_bgp_version"), ipv4Idx), 4),
						createStringPDU(oidWithIndex(requireMetricTagOID(t, p, "hwBgpPeerTable", "_peer_description"), ipv4Idx), "branch-v4"),

						createIntegerPDU(oidWithIndex(requireSymbolOID(t, p, "hwBgpPeerTable", "huawei.hwBgpPeerAdminStatus"), ipv6Idx), 2),
						createIntegerPDU(oidWithIndex(requireSymbolOID(t, p, "hwBgpPeerTable", "huawei.hwBgpPeerState"), ipv6Idx), 3),
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "hwBgpPeerTable", "huawei.hwBgpPeerFsmEstablishedCounter"), ipv6Idx), 9),
						createPDU(oidWithIndex(requireMetricTagOID(t, p, "hwBgpPeerTable", "huawei_hw_bgp_peer_remote_addr"), ipv6Idx), gosnmp.OctetString, []byte{
							0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x14,
						}),
						createGauge32PDU(oidWithIndex(requireMetricTagOID(t, p, "hwBgpPeerTable", "_remote_as"), ipv6Idx), 65020),
						createIntegerPDU(oidWithIndex(requireMetricTagOID(t, p, "hwBgpPeerTable", "_bgp_version"), ipv6Idx), 4),
						createStringPDU(oidWithIndex(requireMetricTagOID(t, p, "hwBgpPeerTable", "_peer_description"), ipv6Idx), "branch-v6"),
					},
					"hwBgpPeerMessageTable": {
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "hwBgpPeerMessageTable", "huawei.hwBgpPeerInUpdateMsgCounter"), ipv4Idx), 31),
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "hwBgpPeerMessageTable", "huawei.hwBgpPeerOutUpdateMsgCounter"), ipv4Idx), 41),
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "hwBgpPeerMessageTable", "huawei.hwBgpPeerInUpdateMsgCounter"), ipv6Idx), 51),
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "hwBgpPeerMessageTable", "huawei.hwBgpPeerOutUpdateMsgCounter"), ipv6Idx), 61),
					},
					"hwBgpPeerAddrFamilyTable": {
						createStringPDU(oidWithIndex(requireCrossTableTagOID(t, p, "hwBgpPeerTable", "huawei_hw_bgp_peer_vrf_name"), ipv4Idx), "blue"),
						createStringPDU(oidWithIndex(requireCrossTableTagOID(t, p, "hwBgpPeerTable", "huawei_hw_bgp_peer_vrf_name"), ipv6Idx), "blue"),
					},
				}
			},
			assertions: func(t *testing.T, metrics []ddsnmp.Metric) {
				availabilityV4 := requireMetricWithTags(t, metrics, "bgpPeerAvailability", map[string]string{
					"routing_instance":          "blue",
					"neighbor":                  "192.0.2.14",
					"address_family":            "ipv4",
					"subsequent_address_family": "unicast",
				})
				assert.Equal(t, map[string]int64{"admin_enabled": 1, "established": 1}, availabilityV4.MultiValue)
				assert.Equal(t, "65020", availabilityV4.Tags["remote_as"])

				availabilityV6 := requireMetricWithTags(t, metrics, "bgpPeerAvailability", map[string]string{
					"routing_instance":          "blue",
					"neighbor":                  "2001:0db8:0000:0000:0000:0000:0000:0014",
					"address_family":            "ipv6",
					"subsequent_address_family": "unicast",
				})
				assert.Equal(t, map[string]int64{"admin_enabled": 1, "established": 0}, availabilityV6.MultiValue)

				updatesV4 := requireMetricWithTags(t, metrics, "bgpPeerUpdates", map[string]string{
					"neighbor":       "192.0.2.14",
					"address_family": "ipv4",
				})
				assert.Equal(t, map[string]int64{"received": 31, "sent": 41}, updatesV4.MultiValue)

				updatesV6 := requireMetricWithTags(t, metrics, "bgpPeerUpdates", map[string]string{
					"neighbor":       "2001:0db8:0000:0000:0000:0000:0000:0014",
					"address_family": "ipv6",
				})
				assert.Equal(t, map[string]int64{"received": 51, "sent": 61}, updatesV6.MultiValue)

				transitionsV4 := requireMetricWithTags(t, metrics, "bgpPeerFsmEstablishedTransitions", map[string]string{
					"neighbor":       "192.0.2.14",
					"address_family": "ipv4",
				})
				assert.EqualValues(t, 4, transitionsV4.Value)

				transitionsV6 := requireMetricWithTags(t, metrics, "bgpPeerFsmEstablishedTransitions", map[string]string{
					"neighbor":       "2001:0db8:0000:0000:0000:0000:0000:0014",
					"address_family": "ipv6",
				})
				assert.EqualValues(t, 9, transitionsV6.Value)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			profile := matchedProfileByFile(t, tc.sysObjectID, tc.profileFile)
			filterProfileForAlertSurface(profile, tc.tables, tc.virtuals)

			ctrl, mockHandler := setupMockHandler(t)
			defer ctrl.Finish()

			setProfileWalkExpectations(t, mockHandler, profile, tc.walks(t, profile))

			collector := New(Config{
				SnmpClient:  mockHandler,
				Profiles:    []*ddsnmp.Profile{profile},
				Log:         logger.New(),
				SysObjectID: tc.sysObjectID,
			})

			results, err := collector.Collect()
			require.NoError(t, err)
			require.Len(t, results, 1)

			tc.assertions(t, stripProfilePointers(results[0].Metrics))
		})
	}
}

func matchedProfileByFile(t *testing.T, sysObjectID, profileFile string) *ddsnmp.Profile {
	t.Helper()

	matched := ddsnmp.FindProfiles(sysObjectID, "", nil)
	for _, prof := range matched {
		if strings.HasSuffix(prof.SourceFile, profileFile) {
			return prof
		}
	}

	require.FailNowf(t, "missing profile", "expected %s for %s", profileFile, sysObjectID)
	return nil
}

func filterProfileForAlertSurface(profile *ddsnmp.Profile, keepTables map[string][]string, keepVirtuals []string) {
	profile.Definition.MetricTags = nil
	profile.Definition.StaticTags = nil
	profile.Definition.Metadata = nil
	profile.Definition.SysobjectIDMetadata = nil

	keepTableSymbols := make(map[string]map[string]struct{}, len(keepTables))
	for tableName, symbols := range keepTables {
		set := make(map[string]struct{}, len(symbols))
		for _, symbol := range symbols {
			set[symbol] = struct{}{}
		}
		keepTableSymbols[tableName] = set
	}

	filteredMetrics := make([]ddprofiledefinition.MetricsConfig, 0, len(keepTables))
	for _, metric := range profile.Definition.Metrics {
		if !metric.IsColumn() {
			continue
		}

		keepSymbols, ok := keepTableSymbols[metric.Table.Name]
		if !ok {
			continue
		}

		metric.Symbols = slicesDeleteFunc(metric.Symbols, func(sym ddprofiledefinition.SymbolConfig) bool {
			_, keep := keepSymbols[sym.Name]
			return !keep
		})
		if len(metric.Symbols) == 0 {
			continue
		}
		filteredMetrics = append(filteredMetrics, metric)
	}
	profile.Definition.Metrics = filteredMetrics

	profile.Definition.VirtualMetrics = slicesDeleteFunc(profile.Definition.VirtualMetrics, func(vm ddprofiledefinition.VirtualMetricConfig) bool {
		for _, keep := range keepVirtuals {
			if vm.Name == keep {
				return false
			}
		}
		return true
	})
}

func setProfileWalkExpectations(t *testing.T, mockHandler *snmpmock.MockHandler, profile *ddsnmp.Profile, walks map[string][]gosnmp.SnmpPDU) {
	t.Helper()

	handleCrossTableTagsWithoutMetrics(profile)
	mockHandler.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()

	seen := make(map[string]bool)
	for _, metric := range profile.Definition.Metrics {
		if metric.Table.OID == "" || metric.Table.Name == "" || seen[metric.Table.Name] {
			continue
		}
		seen[metric.Table.Name] = true

		pdus, ok := walks[metric.Table.Name]
		require.Truef(t, ok, "missing mocked walk for table %s", metric.Table.Name)
		mockHandler.EXPECT().BulkWalkAll(metric.Table.OID).Return(pdus, nil)
	}
}

func requireSymbolOID(t *testing.T, profile *ddsnmp.Profile, tableName, symbolName string) string {
	t.Helper()

	for _, metric := range profile.Definition.Metrics {
		if metric.Table.Name != tableName {
			continue
		}
		for _, sym := range metric.Symbols {
			if sym.Name == symbolName {
				return strings.Trim(sym.OID, ".")
			}
		}
	}

	require.FailNowf(t, "missing symbol", "missing symbol %s on table %s", symbolName, tableName)
	return ""
}

func requireMetricTagOID(t *testing.T, profile *ddsnmp.Profile, tableName, tagName string) string {
	t.Helper()

	for _, metric := range profile.Definition.Metrics {
		if metric.Table.Name != tableName {
			continue
		}
		for _, tag := range metric.MetricTags {
			if tag.Tag == tagName {
				return strings.Trim(tag.Symbol.OID, ".")
			}
		}
	}

	require.FailNowf(t, "missing tag", "missing tag %s on table %s", tagName, tableName)
	return ""
}

func requireCrossTableTagOID(t *testing.T, profile *ddsnmp.Profile, tableName, tagName string) string {
	t.Helper()

	for _, metric := range profile.Definition.Metrics {
		if metric.Table.Name != tableName {
			continue
		}
		for _, tag := range metric.MetricTags {
			if tag.Tag == tagName {
				return strings.Trim(tag.Symbol.OID, ".")
			}
		}
	}

	require.FailNowf(t, "missing cross-table tag", "missing cross-table tag %s on table %s", tagName, tableName)
	return ""
}

func requireCrossTableLookupOID(t *testing.T, profile *ddsnmp.Profile, tableName, tagName string) string {
	t.Helper()

	for _, metric := range profile.Definition.Metrics {
		if metric.Table.Name != tableName {
			continue
		}
		for _, tag := range metric.MetricTags {
			if tag.Tag == tagName {
				return strings.Trim(tag.LookupSymbol.OID, ".")
			}
		}
	}

	require.FailNowf(t, "missing lookup tag", "missing lookup tag %s on table %s", tagName, tableName)
	return ""
}

func oidWithIndex(baseOID, index string) string {
	return strings.Trim(baseOID, ".") + "." + strings.Trim(index, ".")
}

func stripProfilePointers(metrics []ddsnmp.Metric) []ddsnmp.Metric {
	out := make([]ddsnmp.Metric, len(metrics))
	for i, metric := range metrics {
		metric.Profile = nil
		out[i] = metric
	}
	return out
}

func requireMetricWithTags(t *testing.T, metrics []ddsnmp.Metric, name string, subset map[string]string) ddsnmp.Metric {
	t.Helper()

	var candidates []map[string]string
	for _, metric := range metrics {
		if metric.Name != name {
			continue
		}
		candidates = append(candidates, metric.Tags)
		if tagsContain(metric.Tags, subset) {
			return metric
		}
	}

	require.FailNowf(t, "missing metric", "missing metric %s with tags %v; candidates: %v", name, subset, candidates)
	return ddsnmp.Metric{}
}

func tagsContain(tags map[string]string, subset map[string]string) bool {
	for k, v := range subset {
		if tags[k] != v {
			return false
		}
	}
	return true
}

func slicesDeleteFunc[T any](s []T, del func(T) bool) []T {
	out := s[:0]
	for _, v := range s {
		if del(v) {
			continue
		}
		out = append(out, v)
	}
	return out
}
