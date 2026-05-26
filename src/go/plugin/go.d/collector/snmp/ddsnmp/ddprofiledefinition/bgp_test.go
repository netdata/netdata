// SPDX-License-Identifier: GPL-3.0-or-later

package ddprofiledefinition

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestProfileDefinition_UnmarshalBGP(t *testing.T) {
	var profile ProfileDefinition

	err := yaml.Unmarshal([]byte(`
metric_tags:
  - tag: vendor
    consumers: [bgp]
    symbol:
      OID: 1.3.6.1.2.1.1.1.0
      name: sysDescr
bgp:
  - id: std-peer
    MIB: BGP4-MIB
    kind: peer
    table:
      OID: 1.3.6.1.2.1.15.3
      name: bgpPeerTable
    identity:
      neighbor:
        symbol: { OID: 1.3.6.1.2.1.15.3.1.7, name: bgpPeerRemoteAddr, format: ip_address }
      remote_as:
        symbol: { OID: 1.3.6.1.2.1.15.3.1.9, name: bgpPeerRemoteAs, format: uint32 }
    state:
      symbol:
        OID: 1.3.6.1.2.1.15.3.1.2
        name: bgpPeerState
        mapping:
          items: { 1: idle, 2: connect, 3: active, 4: opensent, 5: openconfirm, 6: established }
    connection:
      established_uptime:
        table: bgpPeerTimesTable
        lookup_symbol: { OID: 1.3.6.1.2.1.15.3.1.8, name: bgpPeerIndex }
        symbol: { OID: 1.3.6.1.2.1.15.3.1.16, name: bgpPeerFsmEstablishedTime }
`), &profile)

	require.NoError(t, err)
	require.Len(t, profile.BGP, 1)
	assert.Equal(t, "std-peer", profile.BGP[0].ID)
	assert.Equal(t, "BGP4-MIB", profile.BGP[0].MIB)
	assert.Equal(t, BGPRowKindPeer, profile.BGP[0].Kind)
	assert.Equal(t, "bgpPeerRemoteAddr", profile.BGP[0].Identity.Neighbor.Symbol.Name)
	assert.Equal(t, "bgpPeerFsmEstablishedTime", profile.BGP[0].Connection.EstablishedUptime.Symbol.Name)
	assert.Equal(t, "bgpPeerIndex", SymbolConfig(profile.BGP[0].Connection.EstablishedUptime.LookupSymbol).Name)
	require.Len(t, profile.MetricTags, 1)
	assert.Equal(t, ConsumerSet{ConsumerBGP}, profile.MetricTags[0].Consumers)
}

func TestProfileDefinition_CloneBGP(t *testing.T) {
	profile := &ProfileDefinition{
		BGP: []BGPConfig{
			{
				OriginProfileID: "_vendor-bgp.yaml",
				ID:              "peer",
				Kind:            BGPRowKindPeerFamily,
				Identity: BGPIdentityConfig{
					Neighbor:      BGPValueConfig{Value: "192.0.2.1"},
					RemoteAS:      BGPValueConfig{Value: "65001"},
					AddressFamily: BGPAddressFamilyValueConfig{BGPValueConfig: BGPValueConfig{IndexFromEnd: 2, Mapping: NewExactMapping(map[string]string{"1": "ipv4"})}},
					SubsequentAddressFamily: BGPSubsequentAddressFamilyValueConfig{
						BGPValueConfig: BGPValueConfig{Value: "unicast"},
					},
				},
				State: BGPStateConfig{
					BGPValueConfig: BGPValueConfig{
						Symbol: SymbolConfig{
							OID:  "1.2.3.1",
							Name: "bgpPeerState",
							Mapping: NewExactMapping(map[string]string{
								"1": "idle",
								"2": "connect",
								"3": "active",
								"4": "opensent",
								"5": "openconfirm",
								"6": "established",
							}),
						},
					},
				},
				Routes: BGPRoutesConfig{
					Current: BGPRouteCountersConfig{
						Accepted: BGPValueConfig{
							Table:        "peerTable",
							LookupSymbol: SymbolConfigCompat(SymbolConfig{OID: "1.2.3.14", Name: "peerIndex"}),
							Symbol:       SymbolConfig{OID: "1.2.4.1", Name: "acceptedPrefixes"},
						},
					},
				},
				MetricTags: MetricTagConfigList{
					{Tag: "routing_instance", IndexTransform: []MetricIndexTransform{{Start: 1}}},
				},
			},
		},
	}

	cloned := profile.Clone()
	require.Equal(t, profile, cloned)

	cloned.BGP[0].State.Symbol.Mapping.Items["1"] = "broken"
	cloned.BGP[0].Routes.Current.Accepted.Symbol.Name = "brokenPrefixes"
	cloned.BGP[0].Routes.Current.Accepted.LookupSymbol.Name = "brokenPeerIndex"
	cloned.BGP[0].MetricTags[0].IndexTransform[0].Start = 2
	cloned.BGP[0].Identity.AddressFamily.IndexFromEnd = 3

	assert.Equal(t, "idle", profile.BGP[0].State.Symbol.Mapping.Items["1"])
	assert.Equal(t, "acceptedPrefixes", profile.BGP[0].Routes.Current.Accepted.Symbol.Name)
	assert.Equal(t, "peerIndex", profile.BGP[0].Routes.Current.Accepted.LookupSymbol.Name)
	assert.Equal(t, uint(1), profile.BGP[0].MetricTags[0].IndexTransform[0].Start)
	assert.Equal(t, uint(2), profile.BGP[0].Identity.AddressFamily.IndexFromEnd)
}

func TestValidateEnrichProfile_BGP(t *testing.T) {
	tests := map[string]struct {
		profile         ProfileDefinition
		wantErrContains []string
	}{
		"valid peer family": {
			profile: ProfileDefinition{
				BGP: []BGPConfig{validBGPPeerFamilyRow()},
			},
		},
		"valid peer family with cross-table value source": {
			profile: ProfileDefinition{
				BGP: []BGPConfig{validBGPPeerFamilyRowWithCrossTableSource()},
			},
		},
		"valid peer family with declared cross-table value source": {
			profile: ProfileDefinition{
				BGP: []BGPConfig{
					validBGPPeerFamilyRowWithCrossTableSource(),
					validBGPPeerRowForTable("bgpPeerTable", "1.2.4"),
				},
			},
		},
		"invalid kind": {
			profile: ProfileDefinition{
				BGP: []BGPConfig{
					{Kind: BGPRowKind("session")},
				},
			},
			wantErrContains: []string{`bgp[0].kind: invalid kind "session"`},
		},
		"peer requires neighbor and remote as": {
			profile: ProfileDefinition{
				BGP: []BGPConfig{
					{
						Kind: BGPRowKindPeer,
						State: BGPStateConfig{BGPValueConfig: BGPValueConfig{
							Symbol: bgpStateSymbol(),
						}},
					},
				},
			},
			wantErrContains: []string{
				"bgp[0].identity.neighbor: peer rows require neighbor identity",
				"bgp[0].identity.remote_as: peer rows require remote_as identity",
			},
		},
		"peer family requires afi and safi": {
			profile: ProfileDefinition{
				BGP: []BGPConfig{
					{
						Kind: BGPRowKindPeerFamily,
						Identity: BGPIdentityConfig{
							Neighbor: BGPValueConfig{Value: "192.0.2.1"},
							RemoteAS: BGPValueConfig{Value: "65001"},
						},
						State: BGPStateConfig{BGPValueConfig: BGPValueConfig{
							Symbol: bgpStateSymbol(),
						}},
					},
				},
			},
			wantErrContains: []string{
				"bgp[0].identity.address_family: peer_family rows require address_family identity",
				"bgp[0].identity.subsequent_address_family: peer_family rows require subsequent_address_family identity",
			},
		},
		"forbids cross-table value source on scalar row": {
			profile: ProfileDefinition{
				BGP: []BGPConfig{
					{
						ID:   "peer",
						Kind: BGPRowKindPeer,
						Identity: BGPIdentityConfig{
							Neighbor: BGPValueConfig{Value: "192.0.2.1"},
							RemoteAS: BGPValueConfig{
								Table: "peerTable",
								Symbol: SymbolConfig{
									OID:  "1.2.3.1",
									Name: "bgpPeerRemoteAs",
								},
							},
						},
						State: BGPStateConfig{BGPValueConfig: BGPValueConfig{
							Symbol: bgpStateSymbol(),
						}},
					},
				},
			},
			wantErrContains: []string{"bgp[0].identity.remote_as.table: scalar BGP values do not support `table` lookups"},
		},
		"forbids table value source without symbol oid": {
			profile: ProfileDefinition{
				BGP: []BGPConfig{
					{
						ID:   "peer-family",
						Kind: BGPRowKindPeerFamily,
						Table: SymbolConfig{
							OID:  "1.2.3",
							Name: "peerFamilyTable",
						},
						Identity: BGPIdentityConfig{
							Neighbor: BGPValueConfig{Value: "192.0.2.1"},
							RemoteAS: BGPValueConfig{
								Table:          "peerTable",
								IndexTransform: []MetricIndexTransform{{Start: 0, DropRight: 2}},
							},
							AddressFamily: BGPAddressFamilyValueConfig{BGPValueConfig: BGPValueConfig{Value: "ipv4"}},
							SubsequentAddressFamily: BGPSubsequentAddressFamilyValueConfig{
								BGPValueConfig: BGPValueConfig{Value: "unicast"},
							},
						},
						Routes: BGPRoutesConfig{
							Current: BGPRouteCountersConfig{
								Accepted: BGPValueConfig{Symbol: SymbolConfig{OID: "1.2.3.1", Name: "acceptedPrefixes"}},
							},
						},
					},
				},
			},
			wantErrContains: []string{"bgp[0].identity.remote_as.table: table lookups require symbol.OID, OID, or from"},
		},
		"forbids lookup symbol without table": {
			profile: ProfileDefinition{
				BGP: []BGPConfig{
					{
						ID:    "peer-family",
						Kind:  BGPRowKindPeerFamily,
						Table: SymbolConfig{OID: "1.2.3", Name: "peerFamilyTable"},
						Identity: BGPIdentityConfig{
							Neighbor: BGPValueConfig{Value: "192.0.2.1"},
							RemoteAS: BGPValueConfig{
								Symbol:       SymbolConfig{OID: "1.2.3.1", Name: "remoteAS"},
								LookupSymbol: SymbolConfigCompat(SymbolConfig{OID: "1.2.4.1", Name: "peerIndex"}),
							},
							AddressFamily: BGPAddressFamilyValueConfig{BGPValueConfig: BGPValueConfig{Value: "ipv4"}},
							SubsequentAddressFamily: BGPSubsequentAddressFamilyValueConfig{
								BGPValueConfig: BGPValueConfig{Value: "unicast"},
							},
						},
						Routes: BGPRoutesConfig{
							Current: BGPRouteCountersConfig{
								Accepted: BGPValueConfig{Symbol: SymbolConfig{OID: "1.2.3.2", Name: "acceptedPrefixes"}},
							},
						},
					},
				},
			},
			wantErrContains: []string{"bgp[0].identity.remote_as.lookup_symbol: lookup_symbol requires table"},
		},
		"forbids cross-table source outside declared referenced table": {
			profile: ProfileDefinition{
				BGP: []BGPConfig{
					func() BGPConfig {
						row := validBGPPeerFamilyRowWithCrossTableSource()
						row.Identity.RemoteAS.Symbol.OID = "1.9.9.1"
						return row
					}(),
					validBGPPeerRowForTable("bgpPeerTable", "1.2.4"),
				},
			},
			wantErrContains: []string{`bgp[0].identity.remote_as.table: referenced table "bgpPeerTable" uses OID "1.2.4"; source OID "1.9.9.1" is outside referenced table`},
		},
		"forbids lookup symbol outside declared referenced table": {
			profile: ProfileDefinition{
				BGP: []BGPConfig{
					func() BGPConfig {
						row := validBGPPeerFamilyRowWithCrossTableSource()
						row.Identity.RemoteAS.LookupSymbol.OID = "1.9.9.2"
						return row
					}(),
					validBGPPeerRowForTable("bgpPeerTable", "1.2.4"),
				},
			},
			wantErrContains: []string{`bgp[0].identity.remote_as.lookup_symbol: referenced table "bgpPeerTable" uses OID "1.2.4"; lookup OID "1.9.9.2" is outside referenced table`},
		},
		"forbids incomplete state mapping unless partial": {
			profile: ProfileDefinition{
				BGP: []BGPConfig{
					{
						ID:   "peer",
						Kind: BGPRowKindPeer,
						Identity: BGPIdentityConfig{
							Neighbor: BGPValueConfig{Value: "192.0.2.1"},
							RemoteAS: BGPValueConfig{Value: "65001"},
						},
						State: BGPStateConfig{BGPValueConfig: BGPValueConfig{
							Symbol: SymbolConfig{
								OID:     "1.2.3.1",
								Name:    "bgpPeerState",
								Mapping: NewExactMapping(map[string]string{"6": "established"}),
							},
						}},
					},
				},
			},
			wantErrContains: []string{`bgp[0].state.mapping: missing RFC 4271 state "idle"`},
		},
		"allows explicit partial state mapping": {
			profile: ProfileDefinition{
				BGP: []BGPConfig{
					{
						ID:   "peer",
						Kind: BGPRowKindPeer,
						Identity: BGPIdentityConfig{
							Neighbor: BGPValueConfig{Value: "192.0.2.1"},
							RemoteAS: BGPValueConfig{Value: "65001"},
						},
						State: BGPStateConfig{
							BGPValueConfig: BGPValueConfig{
								Symbol: SymbolConfig{
									OID:     "1.2.3.1",
									Name:    "bgpPeerState",
									Mapping: NewExactMapping(map[string]string{"6": "established"}),
								},
							},
							Partial:       true,
							PartialStates: []BGPPeerState{BGPPeerStateEstablished},
						},
					},
				},
			},
		},
		"forbids partial state mapping without mapping items": {
			profile: ProfileDefinition{
				BGP: []BGPConfig{
					{
						ID:   "peer",
						Kind: BGPRowKindPeer,
						Identity: BGPIdentityConfig{
							Neighbor: BGPValueConfig{Value: "192.0.2.1"},
							RemoteAS: BGPValueConfig{Value: "65001"},
						},
						State: BGPStateConfig{
							BGPValueConfig: BGPValueConfig{
								Symbol: SymbolConfig{OID: "1.2.3.1", Name: "bgpPeerState"},
							},
							Partial: true,
						},
					},
				},
			},
			wantErrContains: []string{"bgp[0].state.mapping: partial state mapping requires at least one RFC 4271 state"},
		},
		"forbids invalid state mapping value": {
			profile: ProfileDefinition{
				BGP: []BGPConfig{
					{
						ID:   "peer",
						Kind: BGPRowKindPeer,
						Identity: BGPIdentityConfig{
							Neighbor: BGPValueConfig{Value: "192.0.2.1"},
							RemoteAS: BGPValueConfig{Value: "65001"},
						},
						State: BGPStateConfig{BGPValueConfig: BGPValueConfig{
							Symbol: SymbolConfig{
								OID:     "1.2.3.1",
								Name:    "bgpPeerState",
								Mapping: NewExactMapping(map[string]string{"7": "almost_up"}),
							},
						}},
					},
				},
			},
			wantErrContains: []string{`bgp[0].state.mapping.items[7]: invalid BGP peer state "almost_up"`},
		},
		"forbids typed field without source": {
			profile: ProfileDefinition{
				BGP: []BGPConfig{
					{
						ID:   "peer",
						Kind: BGPRowKindPeer,
						Identity: BGPIdentityConfig{
							Neighbor: BGPValueConfig{Value: "192.0.2.1"},
							RemoteAS: BGPValueConfig{Value: "65001"},
						},
						State: BGPStateConfig{BGPValueConfig: BGPValueConfig{Symbol: bgpStateSymbol()}},
						Connection: BGPConnectionConfig{
							EstablishedUptime: BGPValueConfig{
								Mapping: NewExactMapping(map[string]string{"1": "1"}),
							},
						},
					},
				},
			},
			wantErrContains: []string{"bgp[0].connection.established_uptime: must define value, from, symbol.OID, OID, index, index_from_end, or index_transform"},
		},
		"forbids invalid address family": {
			profile: ProfileDefinition{
				BGP: []BGPConfig{
					{
						ID:   "peer-family",
						Kind: BGPRowKindPeerFamily,
						Identity: BGPIdentityConfig{
							Neighbor:      BGPValueConfig{Value: "192.0.2.1"},
							RemoteAS:      BGPValueConfig{Value: "65001"},
							AddressFamily: BGPAddressFamilyValueConfig{BGPValueConfig: BGPValueConfig{Value: "vpls"}},
							SubsequentAddressFamily: BGPSubsequentAddressFamilyValueConfig{
								BGPValueConfig: BGPValueConfig{Value: "unicast"},
							},
						},
						State: BGPStateConfig{BGPValueConfig: BGPValueConfig{Symbol: bgpStateSymbol()}},
					},
				},
			},
			wantErrContains: []string{`bgp[0].identity.address_family.value: invalid BGP address_family "vpls"`},
		},
		"allows explicit private family values": {
			profile: ProfileDefinition{
				BGP: []BGPConfig{
					{
						ID:   "peer-family",
						Kind: BGPRowKindPeerFamily,
						Identity: BGPIdentityConfig{
							Neighbor: BGPValueConfig{Value: "192.0.2.1"},
							RemoteAS: BGPValueConfig{Value: "65001"},
							AddressFamily: BGPAddressFamilyValueConfig{
								BGPValueConfig: BGPValueConfig{Value: "vendor_private"},
								AllowPrivate:   true,
							},
							SubsequentAddressFamily: BGPSubsequentAddressFamilyValueConfig{
								BGPValueConfig: BGPValueConfig{Value: "vendor_private"},
								AllowPrivate:   true,
							},
						},
						State: BGPStateConfig{BGPValueConfig: BGPValueConfig{Symbol: bgpStateSymbol()}},
					},
				},
			},
		},
		"forbids table from outside row table": {
			profile: ProfileDefinition{
				BGP: []BGPConfig{
					{
						ID:    "peer",
						Kind:  BGPRowKindPeer,
						Table: SymbolConfig{OID: "1.2.3", Name: "bgpPeerTable"},
						Identity: BGPIdentityConfig{
							Neighbor: BGPValueConfig{Value: "192.0.2.1"},
							RemoteAS: BGPValueConfig{Value: "65001"},
						},
						State: BGPStateConfig{BGPValueConfig: BGPValueConfig{From: "1.2.4.1"}},
					},
				},
			},
			wantErrContains: []string{`bgp[0].state.from: OID "1.2.4.1" is outside table "1.2.3"`},
		},
		"forbids duplicate typed fields for same identity": {
			profile: ProfileDefinition{
				BGP: []BGPConfig{
					{
						OriginProfileID: "_vendor-bgp.yaml",
						ID:              "peer",
						Kind:            BGPRowKindPeer,
						Identity: BGPIdentityConfig{
							Neighbor: BGPValueConfig{Value: "192.0.2.1"},
							RemoteAS: BGPValueConfig{Value: "65001"},
						},
						Connection: BGPConnectionConfig{
							EstablishedUptime: BGPValueConfig{Symbol: SymbolConfig{OID: "1.2.3.1", Name: "uptime"}},
						},
					},
					{
						OriginProfileID: "_vendor-bgp.yaml",
						ID:              "peer",
						Kind:            BGPRowKindPeer,
						Identity: BGPIdentityConfig{
							Neighbor: BGPValueConfig{Value: "192.0.2.1"},
							RemoteAS: BGPValueConfig{Value: "65001"},
						},
						Connection: BGPConnectionConfig{
							EstablishedUptime: BGPValueConfig{Symbol: SymbolConfig{OID: "1.2.3.2", Name: "uptime2"}},
						},
					},
				},
			},
			wantErrContains: []string{`duplicate BGP field for structural identity "_vendor-bgp.yaml|scalar-group|peer|peer"`},
		},
		"forbids scalar index lookups": {
			profile: ProfileDefinition{
				BGP: []BGPConfig{
					{
						ID:   "peer",
						Kind: BGPRowKindPeer,
						Identity: BGPIdentityConfig{
							Neighbor: BGPValueConfig{Value: "192.0.2.1"},
							RemoteAS: BGPValueConfig{Value: "65001"},
						},
						Connection: BGPConnectionConfig{
							EstablishedUptime: BGPValueConfig{Index: 1},
						},
					},
				},
			},
			wantErrContains: []string{"bgp[0].connection.established_uptime.index: scalar BGP values do not support `index` lookups"},
		},
		"forbids scalar index_from_end lookups": {
			profile: ProfileDefinition{
				BGP: []BGPConfig{
					{
						ID:   "peer",
						Kind: BGPRowKindPeer,
						Identity: BGPIdentityConfig{
							Neighbor: BGPValueConfig{Value: "192.0.2.1"},
							RemoteAS: BGPValueConfig{Value: "65001"},
						},
						Connection: BGPConnectionConfig{
							EstablishedUptime: BGPValueConfig{IndexFromEnd: 1},
						},
					},
				},
			},
			wantErrContains: []string{"bgp[0].connection.established_uptime.index_from_end: scalar BGP values do not support `index_from_end` lookups"},
		},
		"forbids multiple BGP row index selectors": {
			profile: ProfileDefinition{
				BGP: []BGPConfig{
					func() BGPConfig {
						row := validBGPPeerFamilyRow()
						row.Identity.Neighbor = BGPValueConfig{
							Index:        1,
							IndexFromEnd: 2,
						}
						return row
					}(),
				},
			},
			wantErrContains: []string{"bgp[0].identity.neighbor: index, index_from_end, and index_transform are mutually exclusive (set: index, index_from_end)"},
		},
		"forbids index and index_transform together": {
			profile: ProfileDefinition{
				BGP: []BGPConfig{
					func() BGPConfig {
						row := validBGPPeerFamilyRow()
						row.Identity.Neighbor = BGPValueConfig{
							Index:          1,
							IndexTransform: []MetricIndexTransform{{Start: 1}},
						}
						return row
					}(),
				},
			},
			wantErrContains: []string{"bgp[0].identity.neighbor: index, index_from_end, and index_transform are mutually exclusive (set: index, index_transform)"},
		},
		"forbids index_from_end and index_transform together": {
			profile: ProfileDefinition{
				BGP: []BGPConfig{
					func() BGPConfig {
						row := validBGPPeerFamilyRow()
						row.Identity.AddressFamily = BGPAddressFamilyValueConfig{BGPValueConfig: BGPValueConfig{
							IndexFromEnd:   2,
							IndexTransform: []MetricIndexTransform{{Start: 1}},
							Mapping:        NewExactMapping(map[string]string{"1": "ipv4"}),
						}}
						return row
					}(),
				},
			},
			wantErrContains: []string{"bgp[0].identity.address_family: index, index_from_end, and index_transform are mutually exclusive (set: index_from_end, index_transform)"},
		},
		"forbids all row index selectors together": {
			profile: ProfileDefinition{
				BGP: []BGPConfig{
					func() BGPConfig {
						row := validBGPPeerFamilyRow()
						row.Identity.Neighbor = BGPValueConfig{
							Index:          1,
							IndexFromEnd:   2,
							IndexTransform: []MetricIndexTransform{{Start: 1}},
						}
						return row
					}(),
				},
			},
			wantErrContains: []string{"bgp[0].identity.neighbor: index, index_from_end, and index_transform are mutually exclusive (set: index, index_from_end, index_transform)"},
		},
		"forbids non-device fields on device rows": {
			profile: ProfileDefinition{
				BGP: []BGPConfig{
					{
						Kind:  BGPRowKindDevice,
						State: BGPStateConfig{BGPValueConfig: BGPValueConfig{Symbol: bgpStateSymbol()}},
					},
				},
			},
			wantErrContains: []string{"bgp[0].state: device rows only support device_counts fields"},
		},
		"forbids device counts on peer rows": {
			profile: ProfileDefinition{
				BGP: []BGPConfig{
					{
						Kind: BGPRowKindPeer,
						Identity: BGPIdentityConfig{
							Neighbor: BGPValueConfig{Value: "192.0.2.1"},
							RemoteAS: BGPValueConfig{Value: "65001"},
						},
						Device: BGPDeviceCountsConfig{
							Peers: BGPValueConfig{Symbol: SymbolConfig{OID: "1.2.3.1", Name: "peerCount"}},
						},
					},
				},
			},
			wantErrContains: []string{"bgp[0].device_counts.peers: device_counts fields require kind=device"},
		},
		"forbids route fields on peer rows": {
			profile: ProfileDefinition{
				BGP: []BGPConfig{
					{
						Kind: BGPRowKindPeer,
						Identity: BGPIdentityConfig{
							Neighbor: BGPValueConfig{Value: "192.0.2.1"},
							RemoteAS: BGPValueConfig{Value: "65001"},
						},
						Routes: BGPRoutesConfig{
							Current: BGPRouteCountersConfig{
								Received: BGPValueConfig{Symbol: SymbolConfig{OID: "1.2.3.1", Name: "receivedRoutes"}},
							},
						},
					},
				},
			},
			wantErrContains: []string{"bgp[0].routes.current.received: route fields require kind=peer_family"},
		},
		"forbids unknown consumers": {
			profile: ProfileDefinition{
				MetricTags: []GlobalMetricTagConfig{
					{
						MetricTagConfig: MetricTagConfig{Tag: "vendor"},
						Consumers:       ConsumerSet{ConsumerBGP, ProfileConsumer("bgp_hidden")},
					},
				},
			},
			wantErrContains: []string{`metric_tags[0].consumers[1]: invalid consumer "bgp_hidden"`},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := ValidateEnrichProfile(&tc.profile)
			if len(tc.wantErrContains) == 0 {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			for _, want := range tc.wantErrContains {
				assert.Contains(t, err.Error(), want)
			}
		})
	}
}

func validBGPPeerFamilyRow() BGPConfig {
	return BGPConfig{
		ID:    "peer-family",
		Kind:  BGPRowKindPeerFamily,
		Table: SymbolConfig{OID: "1.2.3", Name: "bgpPeerFamilyTable"},
		Identity: BGPIdentityConfig{
			Neighbor:      BGPValueConfig{Index: 1},
			RemoteAS:      BGPValueConfig{Symbol: SymbolConfig{OID: "1.2.3.1.2", Name: "remoteAS"}},
			AddressFamily: BGPAddressFamilyValueConfig{BGPValueConfig: BGPValueConfig{Value: "ipv4"}},
			SubsequentAddressFamily: BGPSubsequentAddressFamilyValueConfig{
				BGPValueConfig: BGPValueConfig{Value: "unicast"},
			},
		},
		State: BGPStateConfig{BGPValueConfig: BGPValueConfig{Symbol: bgpStateSymbol()}},
		Routes: BGPRoutesConfig{
			Current: BGPRouteCountersConfig{
				Accepted: BGPValueConfig{Symbol: SymbolConfig{OID: "1.2.3.1.3", Name: "acceptedRoutes"}},
			},
		},
	}
}

func validBGPPeerFamilyRowWithCrossTableSource() BGPConfig {
	row := validBGPPeerFamilyRow()
	row.Identity.RemoteAS = BGPValueConfig{
		Table:          "bgpPeerTable",
		IndexTransform: []MetricIndexTransform{{Start: 0, DropRight: 2}},
		LookupSymbol:   SymbolConfigCompat(SymbolConfig{OID: "1.2.4.1.1", Name: "peerIndex"}),
		Symbol: SymbolConfig{
			OID:  "1.2.4.1.2",
			Name: "remoteAS",
		},
	}
	return row
}

func validBGPPeerRowForTable(name, oid string) BGPConfig {
	return BGPConfig{
		ID:    name + "-peer",
		Kind:  BGPRowKindPeer,
		Table: SymbolConfig{Name: name, OID: oid},
		Identity: BGPIdentityConfig{
			Neighbor: BGPValueConfig{Symbol: SymbolConfig{OID: oid + ".1.1", Name: name + "RemoteAddr", Format: "ip_address"}},
			RemoteAS: BGPValueConfig{Symbol: SymbolConfig{OID: oid + ".1.2", Name: name + "RemoteAs", Format: "uint32"}},
		},
		State: BGPStateConfig{BGPValueConfig: BGPValueConfig{Symbol: bgpStateSymbolWithOID(oid + ".1.3")}},
	}
}

func bgpStateSymbol() SymbolConfig {
	return bgpStateSymbolWithOID("1.2.3.1.1")
}

func bgpStateSymbolWithOID(oid string) SymbolConfig {
	return SymbolConfig{
		OID:  oid,
		Name: "bgpPeerState",
		Mapping: NewExactMapping(map[string]string{
			"1": "idle",
			"2": "connect",
			"3": "active",
			"4": "opensent",
			"5": "openconfirm",
			"6": "established",
		}),
	}
}
