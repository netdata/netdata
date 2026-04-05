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

func TestCollector_Collect_CiscoBgpPeer2PrefixProfile(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	prefixIndex := "1.4.192.0.2.1.1.128"
	peerIndex := "1.4.192.0.2.1"

	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.4.1.9.9.187.1.2.8", []gosnmp.SnmpPDU{
		createCounter32PDU("1.3.6.1.4.1.9.9.187.1.2.8.1.1."+prefixIndex, 1200),
		createGauge32PDU("1.3.6.1.4.1.9.9.187.1.2.8.1.2."+prefixIndex, 7),
		createGauge32PDU("1.3.6.1.4.1.9.9.187.1.2.8.1.3."+prefixIndex, 2000),
		createGauge32PDU("1.3.6.1.4.1.9.9.187.1.2.8.1.4."+prefixIndex, 80),
		createGauge32PDU("1.3.6.1.4.1.9.9.187.1.2.8.1.5."+prefixIndex, 70),
		createGauge32PDU("1.3.6.1.4.1.9.9.187.1.2.8.1.6."+prefixIndex, 1100),
		createGauge32PDU("1.3.6.1.4.1.9.9.187.1.2.8.1.7."+prefixIndex, 3),
		createGauge32PDU("1.3.6.1.4.1.9.9.187.1.2.8.1.8."+prefixIndex, 25),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.4.1.9.9.187.1.2.5.1", []gosnmp.SnmpPDU{
		createPDU("1.3.6.1.4.1.9.9.187.1.2.5.1.6."+peerIndex, gosnmp.OctetString, []byte{198, 51, 100, 1}),
		createGauge32PDU("1.3.6.1.4.1.9.9.187.1.2.5.1.8."+peerIndex, 65000),
		createPDU("1.3.6.1.4.1.9.9.187.1.2.5.1.9."+peerIndex, gosnmp.IPAddress, "198.51.100.10"),
		createIntegerPDU("1.3.6.1.4.1.9.9.187.1.2.5.1.5."+peerIndex, 4),
		createGauge32PDU("1.3.6.1.4.1.9.9.187.1.2.5.1.11."+peerIndex, 65001),
		createPDU("1.3.6.1.4.1.9.9.187.1.2.5.1.12."+peerIndex, gosnmp.IPAddress, "203.0.113.10"),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.4.1.9.9.187.1.2.7.1.3", []gosnmp.SnmpPDU{
		createStringPDU("1.3.6.1.4.1.9.9.187.1.2.7.1.3."+prefixIndex, "vpnv4 unicast"),
	})

	profile := &ddsnmp.Profile{
		SourceFile: "cisco-bgp4-mib.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{
			Metrics: []ddprofiledefinition.MetricsConfig{
				{
					Table: ddprofiledefinition.SymbolConfig{
						OID:  "1.3.6.1.4.1.9.9.187.1.2.8",
						Name: "cbgpPeer2AddrFamilyPrefixTable",
					},
					Symbols: []ddprofiledefinition.SymbolConfig{
						{
							OID:        "1.3.6.1.4.1.9.9.187.1.2.8.1.1",
							Name:       "bgpPeerPrefixesAccepted",
							MetricType: ddprofiledefinition.ProfileMetricTypeGauge,
							ChartMeta: ddprofiledefinition.ChartMeta{
								Description: "The number of accepted route prefixes on this connection for the address family",
								Family:      "Network/Routing/BGP/Peer/Prefix/Accepted",
								Unit:        "{prefix}",
							},
						},
						{
							OID:        "1.3.6.1.4.1.9.9.187.1.2.8.1.2",
							Name:       "bgpPeerPrefixesRejected",
							MetricType: ddprofiledefinition.ProfileMetricTypeGauge,
							ChartMeta: ddprofiledefinition.ChartMeta{
								Description: "The number of route prefixes denied on this connection for the address family",
								Family:      "Network/Routing/BGP/Peer/Prefix/Rejected",
								Unit:        "{prefix}",
							},
						},
						{
							OID:        "1.3.6.1.4.1.9.9.187.1.2.8.1.3",
							Name:       "bgpPeerPrefixAdminLimit",
							MetricType: ddprofiledefinition.ProfileMetricTypeGauge,
							ChartMeta: ddprofiledefinition.ChartMeta{
								Description: "The administrative maximum number of accepted route prefixes for the address family",
								Family:      "Network/Routing/BGP/Peer/Prefix/AdminLimit",
								Unit:        "{prefix}",
							},
						},
						{
							OID:        "1.3.6.1.4.1.9.9.187.1.2.8.1.4",
							Name:       "bgpPeerPrefixThreshold",
							MetricType: ddprofiledefinition.ProfileMetricTypeGauge,
							ChartMeta: ddprofiledefinition.ChartMeta{
								Description: "The warning threshold percentage for accepted route prefixes for the address family",
								Family:      "Network/Routing/BGP/Peer/Prefix/Threshold",
								Unit:        "%",
							},
						},
						{
							OID:        "1.3.6.1.4.1.9.9.187.1.2.8.1.5",
							Name:       "bgpPeerPrefixClearThreshold",
							MetricType: ddprofiledefinition.ProfileMetricTypeGauge,
							ChartMeta: ddprofiledefinition.ChartMeta{
								Description: "The clear threshold percentage for accepted route prefixes for the address family",
								Family:      "Network/Routing/BGP/Peer/Prefix/ClearThreshold",
								Unit:        "%",
							},
						},
						{
							OID:        "1.3.6.1.4.1.9.9.187.1.2.8.1.6",
							Name:       "bgpPeerPrefixesAdvertised",
							MetricType: ddprofiledefinition.ProfileMetricTypeGauge,
							ChartMeta: ddprofiledefinition.ChartMeta{
								Description: "The number of route prefixes advertised on this connection for the address family",
								Family:      "Network/Routing/BGP/Peer/Prefix/Advertised",
								Unit:        "{prefix}",
							},
						},
						{
							OID:        "1.3.6.1.4.1.9.9.187.1.2.8.1.7",
							Name:       "bgpPeerPrefixesSuppressed",
							MetricType: ddprofiledefinition.ProfileMetricTypeGauge,
							ChartMeta: ddprofiledefinition.ChartMeta{
								Description: "The number of route prefixes suppressed from being sent on this connection for the address family",
								Family:      "Network/Routing/BGP/Peer/Prefix/Suppressed",
								Unit:        "{prefix}",
							},
						},
						{
							OID:        "1.3.6.1.4.1.9.9.187.1.2.8.1.8",
							Name:       "bgpPeerPrefixesWithdrawn",
							MetricType: ddprofiledefinition.ProfileMetricTypeGauge,
							ChartMeta: ddprofiledefinition.ChartMeta{
								Description: "The number of route prefixes withdrawn on this connection for the address family",
								Family:      "Network/Routing/BGP/Peer/Prefix/Withdrawn",
								Unit:        "{prefix}",
							},
						},
					},
					MetricTags: []ddprofiledefinition.MetricTagConfig{
						{
							Tag:   "neighbor_address_type",
							Index: 1,
							Mapping: map[string]string{
								"0":  "unknown",
								"1":  "ipv4",
								"2":  "ipv6",
								"3":  "ipv4z",
								"4":  "ipv6z",
								"16": "dns",
							},
						},
						{
							Tag: "neighbor",
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								Name:   "cbgpPeer2RemoteAddrIndex",
								Format: "ip_address",
							},
							IndexTransform: []ddprofiledefinition.MetricIndexTransform{
								{
									Start:     2,
									DropRight: 2,
								},
							},
						},
						{
							Tag: "address_family",
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								Name:                 "cbgpPeer2AddrFamilyAfiIndex",
								ExtractValueCompiled: mustCompileRegex(`^(?:\d+\.)+(\d+)\.\d+$`),
							},
							Mapping: map[string]string{
								"1": "ipv4",
							},
						},
						{
							Tag: "subsequent_address_family",
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								Name:                 "cbgpPeer2AddrFamilySafiIndex",
								ExtractValueCompiled: mustCompileRegex(`^(?:\d+\.)+\d+\.(\d+)$`),
							},
							Mapping: map[string]string{
								"128": "vpn",
							},
						},
						{
							Tag:   "address_family_name",
							Table: "cbgpPeer2AddrFamilyTable",
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								OID:  "1.3.6.1.4.1.9.9.187.1.2.7.1.3",
								Name: "cbgpPeer2AddrFamilyName",
							},
						},
						{
							Tag:   "local_address",
							Table: "cbgpPeer2Table",
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								OID:    "1.3.6.1.4.1.9.9.187.1.2.5.1.6",
								Name:   "cbgpPeer2LocalAddr",
								Format: "ip_address",
							},
							IndexTransform: []ddprofiledefinition.MetricIndexTransform{
								{
									Start:     0,
									DropRight: 2,
								},
							},
						},
						{
							Tag:   "local_as",
							Table: "cbgpPeer2Table",
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								OID:  "1.3.6.1.4.1.9.9.187.1.2.5.1.8",
								Name: "cbgpPeer2LocalAs",
							},
							IndexTransform: []ddprofiledefinition.MetricIndexTransform{
								{
									Start:     0,
									DropRight: 2,
								},
							},
						},
						{
							Tag:   "remote_as",
							Table: "cbgpPeer2Table",
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								OID:  "1.3.6.1.4.1.9.9.187.1.2.5.1.11",
								Name: "cbgpPeer2RemoteAs",
							},
							IndexTransform: []ddprofiledefinition.MetricIndexTransform{
								{
									Start:     0,
									DropRight: 2,
								},
							},
						},
						{
							Tag:   "local_identifier",
							Table: "cbgpPeer2Table",
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								OID:    "1.3.6.1.4.1.9.9.187.1.2.5.1.9",
								Name:   "cbgpPeer2LocalIdentifier",
								Format: "ip_address",
							},
							IndexTransform: []ddprofiledefinition.MetricIndexTransform{
								{
									Start:     0,
									DropRight: 2,
								},
							},
						},
						{
							Tag:   "peer_identifier",
							Table: "cbgpPeer2Table",
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								OID:    "1.3.6.1.4.1.9.9.187.1.2.5.1.12",
								Name:   "cbgpPeer2RemoteIdentifier",
								Format: "ip_address",
							},
							IndexTransform: []ddprofiledefinition.MetricIndexTransform{
								{
									Start:     0,
									DropRight: 2,
								},
							},
						},
						{
							Tag:   "bgp_version",
							Table: "cbgpPeer2Table",
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								OID:  "1.3.6.1.4.1.9.9.187.1.2.5.1.5",
								Name: "cbgpPeer2NegotiatedVersion",
							},
							IndexTransform: []ddprofiledefinition.MetricIndexTransform{
								{
									Start:     0,
									DropRight: 2,
								},
							},
						},
					},
				},
			},
		},
	}

	handleCrossTableTagsWithoutMetrics(profile)
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
		"neighbor_address_type":     "ipv4",
		"neighbor":                  "192.0.2.1",
		"address_family":            "ipv4",
		"subsequent_address_family": "vpn",
		"address_family_name":       "vpnv4 unicast",
		"local_address":             "198.51.100.1",
		"local_as":                  "65000",
		"remote_as":                 "65001",
		"local_identifier":          "198.51.100.10",
		"peer_identifier":           "203.0.113.10",
		"bgp_version":               "4",
	}

	assert.ElementsMatch(t, []ddsnmp.Metric{
		{
			Name:        "bgpPeerPrefixesAccepted",
			Description: "The number of accepted route prefixes on this connection for the address family",
			Family:      "Network/Routing/BGP/Peer/Prefix/Accepted",
			Unit:        "{prefix}",
			Value:       1200,
			Tags:        expectedTags,
			MetricType:  "gauge",
			IsTable:     true,
			Table:       "cbgpPeer2AddrFamilyPrefixTable",
		},
		{
			Name:        "bgpPeerPrefixesRejected",
			Description: "The number of route prefixes denied on this connection for the address family",
			Family:      "Network/Routing/BGP/Peer/Prefix/Rejected",
			Unit:        "{prefix}",
			Value:       7,
			Tags:        expectedTags,
			MetricType:  "gauge",
			IsTable:     true,
			Table:       "cbgpPeer2AddrFamilyPrefixTable",
		},
		{
			Name:        "bgpPeerPrefixAdminLimit",
			Description: "The administrative maximum number of accepted route prefixes for the address family",
			Family:      "Network/Routing/BGP/Peer/Prefix/AdminLimit",
			Unit:        "{prefix}",
			Value:       2000,
			Tags:        expectedTags,
			MetricType:  "gauge",
			IsTable:     true,
			Table:       "cbgpPeer2AddrFamilyPrefixTable",
		},
		{
			Name:        "bgpPeerPrefixThreshold",
			Description: "The warning threshold percentage for accepted route prefixes for the address family",
			Family:      "Network/Routing/BGP/Peer/Prefix/Threshold",
			Unit:        "%",
			Value:       80,
			Tags:        expectedTags,
			MetricType:  "gauge",
			IsTable:     true,
			Table:       "cbgpPeer2AddrFamilyPrefixTable",
		},
		{
			Name:        "bgpPeerPrefixClearThreshold",
			Description: "The clear threshold percentage for accepted route prefixes for the address family",
			Family:      "Network/Routing/BGP/Peer/Prefix/ClearThreshold",
			Unit:        "%",
			Value:       70,
			Tags:        expectedTags,
			MetricType:  "gauge",
			IsTable:     true,
			Table:       "cbgpPeer2AddrFamilyPrefixTable",
		},
		{
			Name:        "bgpPeerPrefixesAdvertised",
			Description: "The number of route prefixes advertised on this connection for the address family",
			Family:      "Network/Routing/BGP/Peer/Prefix/Advertised",
			Unit:        "{prefix}",
			Value:       1100,
			Tags:        expectedTags,
			MetricType:  "gauge",
			IsTable:     true,
			Table:       "cbgpPeer2AddrFamilyPrefixTable",
		},
		{
			Name:        "bgpPeerPrefixesSuppressed",
			Description: "The number of route prefixes suppressed from being sent on this connection for the address family",
			Family:      "Network/Routing/BGP/Peer/Prefix/Suppressed",
			Unit:        "{prefix}",
			Value:       3,
			Tags:        expectedTags,
			MetricType:  "gauge",
			IsTable:     true,
			Table:       "cbgpPeer2AddrFamilyPrefixTable",
		},
		{
			Name:        "bgpPeerPrefixesWithdrawn",
			Description: "The number of route prefixes withdrawn on this connection for the address family",
			Family:      "Network/Routing/BGP/Peer/Prefix/Withdrawn",
			Unit:        "{prefix}",
			Value:       25,
			Tags:        expectedTags,
			MetricType:  "gauge",
			IsTable:     true,
			Table:       "cbgpPeer2AddrFamilyPrefixTable",
		},
	}, results[0].Metrics)
}
