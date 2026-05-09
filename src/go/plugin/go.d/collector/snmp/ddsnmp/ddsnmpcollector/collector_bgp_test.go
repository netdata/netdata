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

func TestCollector_Collect_BGPRowsFromScalarBGPConfig(t *testing.T) {
	tests := map[string]struct {
		statePDU  gosnmp.SnmpPDU
		uptimePDU gosnmp.SnmpPDU
	}{
		"scalar peer row with state and signal": {
			statePDU:  createIntegerPDU("1.3.6.1.4.1.99999.20.1.0", 6),
			uptimePDU: createGauge32PDU("1.3.6.1.4.1.99999.20.2.0", 3600),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl, mockHandler := setupMockHandler(t)
			defer ctrl.Finish()

			expectSNMPGet(mockHandler,
				[]string{
					"1.3.6.1.4.1.99999.20.1.0",
					"1.3.6.1.4.1.99999.20.2.0",
				},
				[]gosnmp.SnmpPDU{tc.statePDU, tc.uptimePDU},
			)

			collector := New(Config{
				SnmpClient: mockHandler,
				Profiles: []*ddsnmp.Profile{
					{
						SourceFile: "vendor-device.yaml",
						Definition: &ddprofiledefinition.ProfileDefinition{
							BGP: []ddprofiledefinition.BGPConfig{
								scalarBGPTestConfig(),
							},
						},
					},
				},
				Log: logger.New(),
			})

			results, err := collector.Collect()
			require.NoError(t, err)
			require.Len(t, results, 1)

			pm := results[0]
			require.Empty(t, pm.HiddenMetrics)
			require.Empty(t, pm.Metrics)
			require.Empty(t, pm.TopologyMetrics)
			require.Empty(t, pm.LicenseRows)
			require.Len(t, pm.BGPRows, 1)

			row := pm.BGPRows[0]
			assert.Equal(t, "_vendor-bgp.yaml", row.OriginProfileID)
			assert.Equal(t, ddprofiledefinition.BGPRowKindPeer, row.Kind)
			assert.Equal(t, "scalar-peer", row.RowKey)
			assert.Equal(t, lengthPrefixedKey("_vendor-bgp.yaml", "peer", "scalar", "scalar-peer"), row.StructuralID)
			assert.Equal(t, "default", row.Identity.RoutingInstance)
			assert.Equal(t, "192.0.2.1", row.Identity.Neighbor)
			assert.Equal(t, "65001", row.Identity.RemoteAS)
			assert.True(t, row.State.Has)
			assert.Equal(t, ddprofiledefinition.BGPPeerStateEstablished, row.State.State)
			assert.Equal(t, "6", row.State.Raw)
			assert.Equal(t, "1.3.6.1.4.1.99999.20.1.0", row.State.SourceOID)
			require.True(t, row.Connection.EstablishedUptime.Has)
			assert.EqualValues(t, 3600, row.Connection.EstablishedUptime.Value)
			assert.Equal(t, "1.3.6.1.4.1.99999.20.2.0", row.Connection.EstablishedUptime.SourceOID)
			assert.Equal(t, map[string]string{"routing_protocol": "bgp"}, row.Tags)
			assert.EqualValues(t, 1, pm.Stats.Metrics.BGP)
		})
	}
}

func TestCollector_Collect_BGPRowsFromTableBGPConfig(t *testing.T) {
	tests := map[string]struct {
		pdus []gosnmp.SnmpPDU
	}{
		"table peer row with index identity": {
			pdus: []gosnmp.SnmpPDU{
				createIntegerPDU("1.3.6.1.4.1.99999.30.1.2.42", 6),
				createGauge32PDU("1.3.6.1.4.1.99999.30.1.3.42", 65001),
				createGauge32PDU("1.3.6.1.4.1.99999.30.1.4.42", 7200),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl, mockHandler := setupMockHandler(t)
			defer ctrl.Finish()

			expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.4.1.99999.30.1", tc.pdus)

			collector := New(Config{
				SnmpClient: mockHandler,
				Profiles: []*ddsnmp.Profile{
					{
						SourceFile: "vendor-device.yaml",
						Definition: &ddprofiledefinition.ProfileDefinition{
							BGP: []ddprofiledefinition.BGPConfig{
								tableBGPTestConfig(),
							},
						},
					},
				},
				Log: logger.New(),
			})

			results, err := collector.Collect()
			require.NoError(t, err)
			require.Len(t, results, 1)

			pm := results[0]
			require.Len(t, pm.BGPRows, 1)

			row := pm.BGPRows[0]
			assert.Equal(t, "_vendor-bgp.yaml", row.OriginProfileID)
			assert.Equal(t, ddprofiledefinition.BGPRowKindPeer, row.Kind)
			assert.Equal(t, "1.3.6.1.4.1.99999.30.1", row.TableOID)
			assert.Equal(t, "vendorBgpPeerTable", row.Table)
			assert.Equal(t, "42", row.RowKey)
			assert.Equal(t, lengthPrefixedKey("_vendor-bgp.yaml", "peer", "table", "table-peer", "1.3.6.1.4.1.99999.30.1", "42"), row.StructuralID)
			assert.Equal(t, "42", row.Identity.Neighbor)
			assert.Equal(t, "65001", row.Identity.RemoteAS)
			assert.Equal(t, "blue", row.Tags["routing_instance"])
			assert.True(t, row.State.Has)
			assert.Equal(t, ddprofiledefinition.BGPPeerStateEstablished, row.State.State)
			require.True(t, row.Connection.EstablishedUptime.Has)
			assert.EqualValues(t, 7200, row.Connection.EstablishedUptime.Value)
			assert.EqualValues(t, 1, pm.Stats.Metrics.BGP)
			assert.EqualValues(t, 1, pm.Stats.TableCache.Misses)
			assert.EqualValues(t, 1, pm.Stats.SNMP.TablesWalked)
		})
	}
}

func TestCollector_Collect_BGPRowsWithCrossTableBGPValues(t *testing.T) {
	tests := map[string]struct {
		mainPDUs         []gosnmp.SnmpPDU
		peerPDUs         []gosnmp.SnmpPDU
		expectedRowKey   string
		expectedNeighbor string
		expectedAFI      ddprofiledefinition.BGPAddressFamily
		expectedSAFI     ddprofiledefinition.BGPSubsequentAddressFamily
		expectedPeerType string
	}{
		"ipv4 peer family row resolves identity from peer table": {
			mainPDUs: []gosnmp.SnmpPDU{
				createGauge32PDU("1.3.6.1.4.1.99999.70.1.1.4.192.0.2.1.1.1", 42),
			},
			peerPDUs: []gosnmp.SnmpPDU{
				createGauge32PDU("1.3.6.1.4.1.99999.70.2.1.4.192.0.2.1", 65001),
				createStringPDU("1.3.6.1.4.1.99999.70.2.2.4.192.0.2.1", "blue"),
			},
			expectedRowKey:   "4.192.0.2.1.1.1",
			expectedNeighbor: "192.0.2.1",
			expectedAFI:      ddprofiledefinition.BGPAddressFamilyIPv4,
			expectedSAFI:     ddprofiledefinition.BGPSubsequentAddressFamilyUnicast,
			expectedPeerType: "01",
		},
		"ipv6 peer family row resolves AFI and SAFI from the tail of a variable-length row index": {
			mainPDUs: []gosnmp.SnmpPDU{
				createGauge32PDU("1.3.6.1.4.1.99999.70.1.1.16.32.1.13.184.0.0.0.0.0.0.0.0.0.0.0.1.2.1", 42),
			},
			peerPDUs: []gosnmp.SnmpPDU{
				createGauge32PDU("1.3.6.1.4.1.99999.70.2.1.16.32.1.13.184.0.0.0.0.0.0.0.0.0.0.0.1", 65001),
				createStringPDU("1.3.6.1.4.1.99999.70.2.2.16.32.1.13.184.0.0.0.0.0.0.0.0.0.0.0.1", "blue"),
			},
			expectedRowKey:   "16.32.1.13.184.0.0.0.0.0.0.0.0.0.0.0.1.2.1",
			expectedNeighbor: "2001:db8::1",
			expectedAFI:      ddprofiledefinition.BGPAddressFamilyIPv6,
			expectedSAFI:     ddprofiledefinition.BGPSubsequentAddressFamilyUnicast,
			expectedPeerType: "02",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl, mockHandler := setupMockHandler(t)
			defer ctrl.Finish()

			expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.4.1.99999.70.1", tc.mainPDUs)
			expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.4.1.99999.70.2", tc.peerPDUs)

			collector := New(Config{
				SnmpClient: mockHandler,
				Profiles: []*ddsnmp.Profile{
					{
						SourceFile: "vendor-device.yaml",
						Definition: &ddprofiledefinition.ProfileDefinition{
							BGP: []ddprofiledefinition.BGPConfig{
								crossTableBGPTestConfig(),
							},
						},
					},
				},
				Log: logger.New(),
			})

			results, err := collector.Collect()
			require.NoError(t, err)
			require.Len(t, results, 1)
			require.Len(t, results[0].BGPRows, 1)

			row := results[0].BGPRows[0]
			assert.Equal(t, ddprofiledefinition.BGPRowKindPeerFamily, row.Kind)
			assert.Equal(t, tc.expectedRowKey, row.RowKey)
			assert.Equal(t, tc.expectedNeighbor, row.Identity.Neighbor)
			assert.Equal(t, "65001", row.Identity.RemoteAS)
			assert.Equal(t, tc.expectedAFI, row.Identity.AddressFamily)
			assert.Equal(t, tc.expectedSAFI, row.Identity.SubsequentAddressFamily)
			assert.Equal(t, "blue", row.Identity.RoutingInstance)
			assert.Equal(t, tc.expectedPeerType, row.Descriptors.PeerType)
			require.True(t, row.Routes.Current.Accepted.Has)
			assert.EqualValues(t, 42, row.Routes.Current.Accepted.Value)
			assert.Equal(t, "1.3.6.1.4.1.99999.70.1.1", row.Routes.Current.Accepted.SourceOID)
			assert.EqualValues(t, 1, results[0].Stats.Metrics.BGP)
			assert.EqualValues(t, 2, results[0].Stats.SNMP.TablesWalked)
		})
	}
}

func TestCollector_Collect_BGPRowsFromCategoricalBGPFields(t *testing.T) {
	tests := map[string]struct {
		reasonPDU gosnmp.SnmpPDU
	}{
		"mapped last down reason is emitted as text": {
			reasonPDU: createIntegerPDU("1.3.6.1.4.1.99999.60.1.0", 2),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl, mockHandler := setupMockHandler(t)
			defer ctrl.Finish()

			expectSNMPGet(mockHandler,
				[]string{"1.3.6.1.4.1.99999.60.1.0"},
				[]gosnmp.SnmpPDU{tc.reasonPDU},
			)

			collector := New(Config{
				SnmpClient: mockHandler,
				Profiles: []*ddsnmp.Profile{
					{
						SourceFile: "vendor-device.yaml",
						Definition: &ddprofiledefinition.ProfileDefinition{
							BGP: []ddprofiledefinition.BGPConfig{
								{
									OriginProfileID: "_vendor-bgp.yaml",
									ID:              "categorical-peer",
									Kind:            ddprofiledefinition.BGPRowKindPeer,
									Identity: ddprofiledefinition.BGPIdentityConfig{
										Neighbor: ddprofiledefinition.BGPValueConfig{Value: "192.0.2.1"},
										RemoteAS: ddprofiledefinition.BGPValueConfig{Value: "65001"},
									},
									Reasons: ddprofiledefinition.BGPReasonsConfig{
										LastDown: ddprofiledefinition.BGPValueConfig{
											Symbol: ddprofiledefinition.SymbolConfig{
												OID:  "1.3.6.1.4.1.99999.60.1.0",
												Name: "bgpPeerLastDownReason",
												Mapping: ddprofiledefinition.NewExactMapping(map[string]string{
													"2": "hold_timer_expired",
												}),
											},
										},
									},
								},
							},
						},
					},
				},
				Log: logger.New(),
			})

			results, err := collector.Collect()
			require.NoError(t, err)
			require.Len(t, results, 1)
			require.Len(t, results[0].BGPRows, 1)

			row := results[0].BGPRows[0]
			require.True(t, row.Reasons.LastDown.Has)
			assert.Equal(t, "hold_timer_expired", row.Reasons.LastDown.Value)
			assert.Equal(t, "2", row.Reasons.LastDown.Raw)
			assert.Equal(t, "1.3.6.1.4.1.99999.60.1.0", row.Reasons.LastDown.SourceOID)
		})
	}
}

func TestCollector_Collect_BGPRowsBestEffortForRegularMetrics(t *testing.T) {
	tests := map[string]struct {
		bgpPDU gosnmp.SnmpPDU
	}{
		"invalid BGP signal does not drop scalar metric": {
			bgpPDU: createStringPDU("1.3.6.1.4.1.99999.40.1.0", "not-a-number"),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl, mockHandler := setupMockHandler(t)
			defer ctrl.Finish()

			expectSNMPGet(mockHandler,
				[]string{"1.3.6.1.2.1.1.3.0"},
				[]gosnmp.SnmpPDU{createTimeTicksPDU("1.3.6.1.2.1.1.3.0", 123456)},
			)
			expectSNMPGet(mockHandler,
				[]string{"1.3.6.1.4.1.99999.40.1.0"},
				[]gosnmp.SnmpPDU{tc.bgpPDU},
			)

			collector := New(Config{
				SnmpClient: mockHandler,
				Profiles: []*ddsnmp.Profile{
					{
						SourceFile: "vendor-device.yaml",
						Definition: &ddprofiledefinition.ProfileDefinition{
							Metrics: []ddprofiledefinition.MetricsConfig{
								{
									Symbol: ddprofiledefinition.SymbolConfig{
										OID:  "1.3.6.1.2.1.1.3.0",
										Name: "sysUpTime",
									},
								},
							},
							BGP: []ddprofiledefinition.BGPConfig{
								{
									OriginProfileID: "_vendor-bgp.yaml",
									ID:              "bad-bgp-row",
									Kind:            ddprofiledefinition.BGPRowKindPeer,
									Identity: ddprofiledefinition.BGPIdentityConfig{
										Neighbor: ddprofiledefinition.BGPValueConfig{Value: "192.0.2.1"},
										RemoteAS: ddprofiledefinition.BGPValueConfig{Value: "65001"},
									},
									Connection: ddprofiledefinition.BGPConnectionConfig{
										EstablishedUptime: ddprofiledefinition.BGPValueConfig{
											Symbol: ddprofiledefinition.SymbolConfig{
												OID:  "1.3.6.1.4.1.99999.40.1.0",
												Name: "bgpPeerEstablishedTime",
											},
										},
									},
								},
							},
						},
					},
				},
				Log: logger.New(),
			})

			results, err := collector.Collect()
			require.NoError(t, err)
			require.Len(t, results, 1)
			require.Len(t, results[0].Metrics, 1)
			assert.Equal(t, "sysUpTime", results[0].Metrics[0].Name)
			assert.Empty(t, results[0].BGPRows)
			assert.EqualValues(t, 1, results[0].Stats.Errors.Processing.BGP)
		})
	}
}

func TestCollector_Collect_BGPRowsSkipsKnownMissingScalarOIDs(t *testing.T) {
	tests := map[string]struct {
		missingPDU gosnmp.SnmpPDU
	}{
		"no such object cached after first BGP poll": {
			missingPDU: createNoSuchObjectPDU("1.3.6.1.4.1.99999.50.1.0"),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl, mockHandler := setupMockHandler(t)
			defer ctrl.Finish()

			expectSNMPGet(mockHandler,
				[]string{"1.3.6.1.2.1.1.3.0"},
				[]gosnmp.SnmpPDU{createTimeTicksPDU("1.3.6.1.2.1.1.3.0", 123456)},
			)
			expectSNMPGet(mockHandler,
				[]string{"1.3.6.1.4.1.99999.50.1.0"},
				[]gosnmp.SnmpPDU{tc.missingPDU},
			)
			expectSNMPGet(mockHandler,
				[]string{"1.3.6.1.2.1.1.3.0"},
				[]gosnmp.SnmpPDU{createTimeTicksPDU("1.3.6.1.2.1.1.3.0", 223456)},
			)

			collector := New(Config{
				SnmpClient: mockHandler,
				Profiles: []*ddsnmp.Profile{
					{
						SourceFile: "vendor-device.yaml",
						Definition: &ddprofiledefinition.ProfileDefinition{
							Metrics: []ddprofiledefinition.MetricsConfig{
								{
									Symbol: ddprofiledefinition.SymbolConfig{
										OID:  "1.3.6.1.2.1.1.3.0",
										Name: "sysUpTime",
									},
								},
							},
							BGP: []ddprofiledefinition.BGPConfig{
								{
									OriginProfileID: "_vendor-bgp.yaml",
									ID:              "missing-bgp-row",
									Kind:            ddprofiledefinition.BGPRowKindPeer,
									Identity: ddprofiledefinition.BGPIdentityConfig{
										Neighbor: ddprofiledefinition.BGPValueConfig{Value: "192.0.2.1"},
										RemoteAS: ddprofiledefinition.BGPValueConfig{Value: "65001"},
									},
									Connection: ddprofiledefinition.BGPConnectionConfig{
										EstablishedUptime: ddprofiledefinition.BGPValueConfig{
											Symbol: ddprofiledefinition.SymbolConfig{
												OID:  "1.3.6.1.4.1.99999.50.1.0",
												Name: "bgpPeerEstablishedTime",
											},
										},
									},
								},
							},
						},
					},
				},
				Log: logger.New(),
			})

			first, err := collector.Collect()
			require.NoError(t, err)
			require.Len(t, first, 1)
			assert.Empty(t, first[0].BGPRows)
			assert.EqualValues(t, 1, first[0].Stats.Errors.MissingOIDs)

			second, err := collector.Collect()
			require.NoError(t, err)
			require.Len(t, second, 1)
			assert.Empty(t, second[0].BGPRows)
			assert.EqualValues(t, 1, second[0].Stats.Errors.MissingOIDs)
			assert.EqualValues(t, 1, second[0].Stats.SNMP.GetRequests)
			assert.EqualValues(t, 1, second[0].Stats.SNMP.GetOIDs)
		})
	}
}

func scalarBGPTestConfig() ddprofiledefinition.BGPConfig {
	return ddprofiledefinition.BGPConfig{
		OriginProfileID: "_vendor-bgp.yaml",
		ID:              "scalar-peer",
		Kind:            ddprofiledefinition.BGPRowKindPeer,
		Identity: ddprofiledefinition.BGPIdentityConfig{
			RoutingInstance: ddprofiledefinition.BGPValueConfig{Value: "default"},
			Neighbor:        ddprofiledefinition.BGPValueConfig{Value: "192.0.2.1"},
			RemoteAS:        ddprofiledefinition.BGPValueConfig{Value: "65001"},
		},
		State: ddprofiledefinition.BGPStateConfig{
			BGPValueConfig: ddprofiledefinition.BGPValueConfig{
				Symbol: ddprofiledefinition.SymbolConfig{
					OID:     "1.3.6.1.4.1.99999.20.1.0",
					Name:    "bgpPeerState",
					Mapping: bgpPeerStateMapping(),
				},
			},
		},
		Connection: ddprofiledefinition.BGPConnectionConfig{
			EstablishedUptime: ddprofiledefinition.BGPValueConfig{
				Symbol: ddprofiledefinition.SymbolConfig{
					OID:  "1.3.6.1.4.1.99999.20.2.0",
					Name: "bgpPeerEstablishedTime",
				},
			},
		},
		StaticTags: []ddprofiledefinition.StaticMetricTagConfig{
			{Tag: "routing_protocol", Value: "bgp"},
		},
	}
}

func tableBGPTestConfig() ddprofiledefinition.BGPConfig {
	return ddprofiledefinition.BGPConfig{
		OriginProfileID: "_vendor-bgp.yaml",
		ID:              "table-peer",
		Kind:            ddprofiledefinition.BGPRowKindPeer,
		Table: ddprofiledefinition.SymbolConfig{
			OID:  "1.3.6.1.4.1.99999.30.1",
			Name: "vendorBgpPeerTable",
		},
		Identity: ddprofiledefinition.BGPIdentityConfig{
			Neighbor: ddprofiledefinition.BGPValueConfig{Index: 1},
			RemoteAS: ddprofiledefinition.BGPValueConfig{
				Symbol: ddprofiledefinition.SymbolConfig{
					OID:  "1.3.6.1.4.1.99999.30.1.3",
					Name: "vendorBgpPeerRemoteAs",
				},
			},
		},
		State: ddprofiledefinition.BGPStateConfig{
			BGPValueConfig: ddprofiledefinition.BGPValueConfig{
				Symbol: ddprofiledefinition.SymbolConfig{
					OID:     "1.3.6.1.4.1.99999.30.1.2",
					Name:    "vendorBgpPeerState",
					Mapping: bgpPeerStateMapping(),
				},
			},
		},
		Connection: ddprofiledefinition.BGPConnectionConfig{
			EstablishedUptime: ddprofiledefinition.BGPValueConfig{
				Symbol: ddprofiledefinition.SymbolConfig{
					OID:  "1.3.6.1.4.1.99999.30.1.4",
					Name: "vendorBgpPeerEstablishedTime",
				},
			},
		},
		StaticTags: []ddprofiledefinition.StaticMetricTagConfig{
			{Tag: "routing_instance", Value: "blue"},
		},
	}
}

func crossTableBGPTestConfig() ddprofiledefinition.BGPConfig {
	peerIndex := []ddprofiledefinition.MetricIndexTransform{{Start: 0, DropRight: 2}}

	return ddprofiledefinition.BGPConfig{
		OriginProfileID: "_vendor-bgp.yaml",
		ID:              "table-peer-family",
		Kind:            ddprofiledefinition.BGPRowKindPeerFamily,
		Table: ddprofiledefinition.SymbolConfig{
			OID:  "1.3.6.1.4.1.99999.70.1",
			Name: "vendorBgpPeerFamilyTable",
		},
		Identity: ddprofiledefinition.BGPIdentityConfig{
			RoutingInstance: ddprofiledefinition.BGPValueConfig{
				Table:          "vendorBgpPeerTable",
				IndexTransform: peerIndex,
				Symbol: ddprofiledefinition.SymbolConfig{
					OID:  "1.3.6.1.4.1.99999.70.2.2",
					Name: "vendorBgpPeerRoutingInstance",
				},
			},
			Neighbor: ddprofiledefinition.BGPValueConfig{
				IndexTransform: []ddprofiledefinition.MetricIndexTransform{{Start: 1, DropRight: 2}},
				Format:         "ip_address",
			},
			RemoteAS: ddprofiledefinition.BGPValueConfig{
				Table:          "vendorBgpPeerTable",
				IndexTransform: peerIndex,
				Symbol: ddprofiledefinition.SymbolConfig{
					OID:  "1.3.6.1.4.1.99999.70.2.1",
					Name: "vendorBgpPeerRemoteAs",
				},
			},
			AddressFamily: ddprofiledefinition.BGPAddressFamilyValueConfig{
				BGPValueConfig: ddprofiledefinition.BGPValueConfig{
					IndexFromEnd: 2,
					Mapping: ddprofiledefinition.NewExactMapping(map[string]string{
						"1": "ipv4",
						"2": "ipv6",
					}),
				},
			},
			SubsequentAddressFamily: ddprofiledefinition.BGPSubsequentAddressFamilyValueConfig{
				BGPValueConfig: ddprofiledefinition.BGPValueConfig{
					IndexFromEnd: 1,
					Mapping: ddprofiledefinition.NewExactMapping(map[string]string{
						"1": "unicast",
					}),
				},
			},
		},
		Descriptors: ddprofiledefinition.BGPDescriptorsConfig{
			PeerType: ddprofiledefinition.BGPValueConfig{
				IndexFromEnd: 2,
				Format:       "hex",
			},
		},
		Routes: ddprofiledefinition.BGPRoutesConfig{
			Current: ddprofiledefinition.BGPRouteCountersConfig{
				Accepted: ddprofiledefinition.BGPValueConfig{
					Symbol: ddprofiledefinition.SymbolConfig{
						OID:  "1.3.6.1.4.1.99999.70.1.1",
						Name: "vendorBgpPrefixesAccepted",
					},
				},
			},
		},
	}
}

func bgpPeerStateMapping() ddprofiledefinition.MappingConfig {
	return ddprofiledefinition.NewExactMapping(map[string]string{
		"1": "idle",
		"2": "connect",
		"3": "active",
		"4": "opensent",
		"5": "openconfirm",
		"6": "established",
	})
}
