// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestCollector_Collect_CiscoBgpPeer3Profile(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.4.1.9.9.187.1.2.9", []gosnmp.SnmpPDU{
		createIntegerPDU("1.3.6.1.4.1.9.9.187.1.2.9.1.6.7.1.4.192.0.2.1", 2),
		createIntegerPDU("1.3.6.1.4.1.9.9.187.1.2.9.1.5.7.1.4.192.0.2.1", 6),
		createCounter32PDU("1.3.6.1.4.1.9.9.187.1.2.9.1.15.7.1.4.192.0.2.1", 42),
		createCounter32PDU("1.3.6.1.4.1.9.9.187.1.2.9.1.18.7.1.4.192.0.2.1", 88),
		createPDU("1.3.6.1.4.1.9.9.187.1.2.9.1.19.7.1.4.192.0.2.1", gosnmp.OctetString, []byte{0x02, 0x03}),
		createCounter32PDU("1.3.6.1.4.1.9.9.187.1.2.9.1.20.7.1.4.192.0.2.1", 5),
		createGauge32PDU("1.3.6.1.4.1.9.9.187.1.2.9.1.21.7.1.4.192.0.2.1", 3600),
		createGauge32PDU("1.3.6.1.4.1.9.9.187.1.2.9.1.29.7.1.4.192.0.2.1", 30),
		createIntegerPDU("1.3.6.1.4.1.9.9.187.1.2.9.1.31.7.1.4.192.0.2.1", 5),
		createGauge32PDU("1.3.6.1.4.1.9.9.187.1.2.9.1.1.7.1.4.192.0.2.1", 7),
		createStringPDU("1.3.6.1.4.1.9.9.187.1.2.9.1.4.7.1.4.192.0.2.1", "blue"),
		createPDU("1.3.6.1.4.1.9.9.187.1.2.9.1.3.7.1.4.192.0.2.1", gosnmp.OctetString, []byte{192, 0, 2, 1}),
		createPDU("1.3.6.1.4.1.9.9.187.1.2.9.1.8.7.1.4.192.0.2.1", gosnmp.OctetString, []byte{198, 51, 100, 1}),
		createGauge32PDU("1.3.6.1.4.1.9.9.187.1.2.9.1.10.7.1.4.192.0.2.1", 65000),
		createGauge32PDU("1.3.6.1.4.1.9.9.187.1.2.9.1.13.7.1.4.192.0.2.1", 65001),
		createPDU("1.3.6.1.4.1.9.9.187.1.2.9.1.11.7.1.4.192.0.2.1", gosnmp.IPAddress, "198.51.100.10"),
		createPDU("1.3.6.1.4.1.9.9.187.1.2.9.1.14.7.1.4.192.0.2.1", gosnmp.IPAddress, "203.0.113.10"),
		createIntegerPDU("1.3.6.1.4.1.9.9.187.1.2.9.1.7.7.1.4.192.0.2.1", 4),
	})

	profile := &ddsnmp.Profile{
		SourceFile: "cisco-bgp4-mib.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{
			Metrics: []ddprofiledefinition.MetricsConfig{
				{
					Table: ddprofiledefinition.SymbolConfig{
						OID:  "1.3.6.1.4.1.9.9.187.1.2.9",
						Name: "cbgpPeer3Table",
					},
					Symbols: []ddprofiledefinition.SymbolConfig{
						{
							OID:  "1.3.6.1.4.1.9.9.187.1.2.9.1.6",
							Name: "bgpPeerAdminStatus",
							ChartMeta: ddprofiledefinition.ChartMeta{
								Description: "The desired state of the BGP connection",
								Family:      "Network/Routing/BGP/Peer/Admin/Status",
								Unit:        "{status}",
							},
						},
						{
							OID:  "1.3.6.1.4.1.9.9.187.1.2.9.1.5",
							Name: "bgpPeerState",
							ChartMeta: ddprofiledefinition.ChartMeta{
								Description: "The BGP peer connection state",
								Family:      "Network/Routing/BGP/Peer/Connection/Status",
								Unit:        "{status}",
							},
						},
						{
							OID:  "1.3.6.1.4.1.9.9.187.1.2.9.1.15",
							Name: "bgpPeerInUpdates",
							ChartMeta: ddprofiledefinition.ChartMeta{
								Description: "The number of BGP UPDATE messages received on this connection",
								Family:      "Network/Routing/BGP/Peer/Message/Update/In",
								Unit:        "{message}",
							},
						},
						{
							OID:  "1.3.6.1.4.1.9.9.187.1.2.9.1.18",
							Name: "bgpPeerOutTotalMessages",
							ChartMeta: ddprofiledefinition.ChartMeta{
								Description: "The total number of messages transmitted to the remote peer on this connection",
								Family:      "Network/Routing/BGP/Peer/Message/Total/Out",
								Unit:        "{message}",
							},
						},
						{
							OID:                  "1.3.6.1.4.1.9.9.187.1.2.9.1.19",
							Name:                 "bgpPeerLastErrorCode",
							Format:               "hex",
							ExtractValueCompiled: mustCompileRegex("^([0-9a-f]{2})"),
							ChartMeta: ddprofiledefinition.ChartMeta{
								Description: "The last BGP NOTIFICATION error code seen on this connection",
								Family:      "Network/Routing/BGP/Peer/Error/Last/Code",
								Unit:        "{code}",
							},
						},
						{
							OID:                  "1.3.6.1.4.1.9.9.187.1.2.9.1.19",
							Name:                 "bgpPeerLastErrorSubcode",
							Format:               "hex",
							ExtractValueCompiled: mustCompileRegex("^[0-9a-f]{2}([0-9a-f]{2})"),
							ChartMeta: ddprofiledefinition.ChartMeta{
								Description: "The last BGP NOTIFICATION error subcode seen on this connection",
								Family:      "Network/Routing/BGP/Peer/Error/Last/Subcode",
								Unit:        "{subcode}",
							},
						},
						{
							OID:  "1.3.6.1.4.1.9.9.187.1.2.9.1.20",
							Name: "bgpPeerFsmEstablishedTransitions",
							ChartMeta: ddprofiledefinition.ChartMeta{
								Description: "The total number of times the BGP FSM transitioned into the established state",
								Family:      "Network/Routing/BGP/Peer/FSM/Transition/Established/Count",
								Unit:        "{transition}",
							},
						},
						{
							OID:  "1.3.6.1.4.1.9.9.187.1.2.9.1.21",
							Name: "bgpPeerFsmEstablishedTime",
							ChartMeta: ddprofiledefinition.ChartMeta{
								Description: "Time in seconds this peer has been in the Established state or since last in Established state",
								Family:      "Network/Routing/BGP/Peer/FSM/Duration/Established",
								Unit:        "s",
							},
						},
						{
							OID:  "1.3.6.1.4.1.9.9.187.1.2.9.1.29",
							Name: "bgpPeerInUpdateElapsedTime",
							ChartMeta: ddprofiledefinition.ChartMeta{
								Description: "Time since the last BGP UPDATE message was received from the peer",
								Family:      "Network/Routing/BGP/Peer/Message/Update/In/Elapsed",
								Unit:        "s",
							},
						},
						{
							OID:  "1.3.6.1.4.1.9.9.187.1.2.9.1.31",
							Name: "bgpPeerPreviousState",
							ChartMeta: ddprofiledefinition.ChartMeta{
								Description: "The previous BGP peer connection state before the current state",
								Family:      "Network/Routing/BGP/Peer/Connection/Previous/Status",
								Unit:        "{status}",
							},
						},
					},
					MetricTags: []ddprofiledefinition.MetricTagConfig{
						{
							Tag: "routing_instance",
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								OID:  "1.3.6.1.4.1.9.9.187.1.2.9.1.4",
								Name: "cbgpPeer3VrfName",
							},
						},
						{
							Tag: "routing_instance_id",
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								OID:    "1.3.6.1.4.1.9.9.187.1.2.9.1.1",
								Name:   "cbgpPeer3VrfId",
								Format: "uint32",
							},
						},
						{
							Tag:   "neighbor_address_type",
							Index: 2,
							Mapping: ddprofiledefinition.NewExactMapping(map[string]string{
								"0":  "unknown",
								"1":  "ipv4",
								"2":  "ipv6",
								"3":  "ipv4z",
								"4":  "ipv6z",
								"16": "dns",
							}),
						},
						{
							Tag: "neighbor",
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								OID:    "1.3.6.1.4.1.9.9.187.1.2.9.1.3",
								Name:   "cbgpPeer3RemoteAddr",
								Format: "ip_address",
							},
						},
						{
							Tag: "local_address",
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								OID:    "1.3.6.1.4.1.9.9.187.1.2.9.1.8",
								Name:   "cbgpPeer3LocalAddr",
								Format: "ip_address",
							},
						},
						{
							Tag: "local_as",
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								OID:  "1.3.6.1.4.1.9.9.187.1.2.9.1.10",
								Name: "cbgpPeer3LocalAs",
							},
						},
						{
							Tag: "remote_as",
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								OID:  "1.3.6.1.4.1.9.9.187.1.2.9.1.13",
								Name: "cbgpPeer3RemoteAs",
							},
						},
						{
							Tag: "local_identifier",
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								OID:    "1.3.6.1.4.1.9.9.187.1.2.9.1.11",
								Name:   "cbgpPeer3LocalIdentifier",
								Format: "ip_address",
							},
						},
						{
							Tag: "peer_identifier",
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								OID:    "1.3.6.1.4.1.9.9.187.1.2.9.1.14",
								Name:   "cbgpPeer3RemoteIdentifier",
								Format: "ip_address",
							},
						},
						{
							Tag: "bgp_version",
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								OID:  "1.3.6.1.4.1.9.9.187.1.2.9.1.7",
								Name: "cbgpPeer3NegotiatedVersion",
							},
						},
					},
				},
			},
		},
	}

	ddsnmp.HandleCrossTableTagsWithoutMetrics(profile)
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

	for i := range results[0].Metrics {
		results[0].Metrics[i].Profile = nil
	}

	expectedTags := map[string]string{
		"routing_instance":      "blue",
		"routing_instance_id":   "7",
		"neighbor_address_type": "ipv4",
		"neighbor":              "192.0.2.1",
		"local_address":         "198.51.100.1",
		"local_as":              "65000",
		"remote_as":             "65001",
		"local_identifier":      "198.51.100.10",
		"peer_identifier":       "203.0.113.10",
		"bgp_version":           "4",
	}

	assert.ElementsMatch(t, []ddsnmp.Metric{
		{
			Name:        "bgpPeerAdminStatus",
			Description: "The desired state of the BGP connection",
			Family:      "Network/Routing/BGP/Peer/Admin/Status",
			Unit:        "{status}",
			Value:       2,
			Tags:        expectedTags,
			MetricType:  "gauge",
			IsTable:     true,
			Table:       "cbgpPeer3Table",
		},
		{
			Name:        "bgpPeerState",
			Description: "The BGP peer connection state",
			Family:      "Network/Routing/BGP/Peer/Connection/Status",
			Unit:        "{status}",
			Value:       6,
			Tags:        expectedTags,
			MetricType:  "gauge",
			IsTable:     true,
			Table:       "cbgpPeer3Table",
		},
		{
			Name:        "bgpPeerInUpdates",
			Description: "The number of BGP UPDATE messages received on this connection",
			Family:      "Network/Routing/BGP/Peer/Message/Update/In",
			Unit:        "{message}",
			Value:       42,
			Tags:        expectedTags,
			MetricType:  "rate",
			IsTable:     true,
			Table:       "cbgpPeer3Table",
		},
		{
			Name:        "bgpPeerOutTotalMessages",
			Description: "The total number of messages transmitted to the remote peer on this connection",
			Family:      "Network/Routing/BGP/Peer/Message/Total/Out",
			Unit:        "{message}",
			Value:       88,
			Tags:        expectedTags,
			MetricType:  "rate",
			IsTable:     true,
			Table:       "cbgpPeer3Table",
		},
		{
			Name:        "bgpPeerLastErrorCode",
			Description: "The last BGP NOTIFICATION error code seen on this connection",
			Family:      "Network/Routing/BGP/Peer/Error/Last/Code",
			Unit:        "{code}",
			Value:       2,
			Tags:        expectedTags,
			MetricType:  "gauge",
			IsTable:     true,
			Table:       "cbgpPeer3Table",
		},
		{
			Name:        "bgpPeerLastErrorSubcode",
			Description: "The last BGP NOTIFICATION error subcode seen on this connection",
			Family:      "Network/Routing/BGP/Peer/Error/Last/Subcode",
			Unit:        "{subcode}",
			Value:       3,
			Tags:        expectedTags,
			MetricType:  "gauge",
			IsTable:     true,
			Table:       "cbgpPeer3Table",
		},
		{
			Name:        "bgpPeerFsmEstablishedTransitions",
			Description: "The total number of times the BGP FSM transitioned into the established state",
			Family:      "Network/Routing/BGP/Peer/FSM/Transition/Established/Count",
			Unit:        "{transition}",
			Value:       5,
			Tags:        expectedTags,
			MetricType:  "rate",
			IsTable:     true,
			Table:       "cbgpPeer3Table",
		},
		{
			Name:        "bgpPeerFsmEstablishedTime",
			Description: "Time in seconds this peer has been in the Established state or since last in Established state",
			Family:      "Network/Routing/BGP/Peer/FSM/Duration/Established",
			Unit:        "s",
			Value:       3600,
			Tags:        expectedTags,
			MetricType:  "gauge",
			IsTable:     true,
			Table:       "cbgpPeer3Table",
		},
		{
			Name:        "bgpPeerInUpdateElapsedTime",
			Description: "Time since the last BGP UPDATE message was received from the peer",
			Family:      "Network/Routing/BGP/Peer/Message/Update/In/Elapsed",
			Unit:        "s",
			Value:       30,
			Tags:        expectedTags,
			MetricType:  "gauge",
			IsTable:     true,
			Table:       "cbgpPeer3Table",
		},
		{
			Name:        "bgpPeerPreviousState",
			Description: "The previous BGP peer connection state before the current state",
			Family:      "Network/Routing/BGP/Peer/Connection/Previous/Status",
			Unit:        "{status}",
			Value:       5,
			Tags:        expectedTags,
			MetricType:  "gauge",
			IsTable:     true,
			Table:       "cbgpPeer3Table",
		},
	}, results[0].Metrics)
}
