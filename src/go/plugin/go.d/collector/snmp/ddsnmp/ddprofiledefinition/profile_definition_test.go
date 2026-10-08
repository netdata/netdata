// SPDX-License-Identifier: GPL-3.0-or-later
// Portions include software developed at Datadog (https://www.datadoghq.com/),
// licensed under the Apache License Version 2.0.

package ddprofiledefinition

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestProfileDefinition_UnmarshalYAML(t *testing.T) {
	tests := map[string]struct {
		input string
		want  ProfileDefinition
	}{
		"bgp": {
			input: `
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
`,
			want: ProfileDefinition{
				MetricTags: []GlobalMetricTagConfig{{
					MetricTagConfig: MetricTagConfig{
						Tag: "vendor",
						Symbol: SymbolConfigCompat{
							OID:  "1.3.6.1.2.1.1.1.0",
							Name: "sysDescr",
						},
					},
					Consumers: ConsumerSet{ConsumerBGP},
				}},
				BGP: []BGPConfig{{
					ID:   "std-peer",
					MIB:  "BGP4-MIB",
					Kind: BGPRowKindPeer,
					Table: SymbolConfig{
						OID:  "1.3.6.1.2.1.15.3",
						Name: "bgpPeerTable",
					},
					Identity: BGPIdentityConfig{
						Neighbor: BGPValueConfig{
							Symbol: SymbolConfig{
								OID:    "1.3.6.1.2.1.15.3.1.7",
								Name:   "bgpPeerRemoteAddr",
								Format: "ip_address",
							},
						},
						RemoteAS: BGPValueConfig{
							Symbol: SymbolConfig{
								OID:    "1.3.6.1.2.1.15.3.1.9",
								Name:   "bgpPeerRemoteAs",
								Format: "uint32",
							},
						},
					},
					State: BGPStateConfig{
						BGPValueConfig: BGPValueConfig{
							Symbol: SymbolConfig{
								OID:  "1.3.6.1.2.1.15.3.1.2",
								Name: "bgpPeerState",
								Mapping: NewExactMapping(
									map[string]string{
										"1": "idle",
										"2": "connect",
										"3": "active",
										"4": "opensent",
										"5": "openconfirm",
										"6": "established",
									},
								),
							},
						},
					},
					Connection: BGPConnectionConfig{
						EstablishedUptime: BGPValueConfig{
							Table: "bgpPeerTimesTable",
							LookupSymbol: SymbolConfigCompat{
								OID:  "1.3.6.1.2.1.15.3.1.8",
								Name: "bgpPeerIndex",
							},
							Symbol: SymbolConfig{
								OID:  "1.3.6.1.2.1.15.3.1.16",
								Name: "bgpPeerFsmEstablishedTime",
							},
						},
					},
				}},
			}},
		"licensing": {
			input: `
metric_tags:
  - tag: vendor
    consumers: [licensing]
    symbol:
      OID: 1.3.6.1.2.1.1.1.0
      name: sysDescr
licensing:
  - id: sophos-base-firewall
    MIB: SFOS-FIREWALL-MIB
    identity:
      id: { value: base_firewall }
      name: { value: Base Firewall }
    descriptors:
      type: { value: subscription }
    state:
      from: 1.3.6.1.4.1.2604.5.1.5.1.1.0
      policy: sophos
      mapping:
        items: { 0: ignored, 1: healthy, 2: broken }
    signals:
      expiry:
        from: 1.3.6.1.4.1.2604.5.1.5.1.2.0
        format: text_date
        sentinel: [timer_zero_or_negative]
`,
			want: ProfileDefinition{
				MetricTags: []GlobalMetricTagConfig{{
					MetricTagConfig: MetricTagConfig{
						Tag: "vendor",
						Symbol: SymbolConfigCompat{
							OID:  "1.3.6.1.2.1.1.1.0",
							Name: "sysDescr",
						},
					},
					Consumers: ConsumerSet{ConsumerLicensing},
				}},
				Licensing: []LicensingConfig{{
					ID:  "sophos-base-firewall",
					MIB: "SFOS-FIREWALL-MIB",
					Identity: LicenseIdentityConfig{
						ID: LicenseValueConfig{
							Value: "base_firewall",
						},
						Name: LicenseValueConfig{
							Value: "Base Firewall",
						},
					},
					Descriptors: LicenseDescriptorsConfig{
						Type: LicenseValueConfig{
							Value: "subscription",
						},
					},
					State: LicenseStateConfig{
						LicenseValueConfig: LicenseValueConfig{
							From:    "1.3.6.1.4.1.2604.5.1.5.1.1.0",
							Mapping: NewExactMapping(map[string]string{"0": "ignored", "1": "healthy", "2": "broken"}),
						},
						Policy: LicenseStatePolicySophos,
					},
					Signals: LicenseSignalsConfig{
						Expiry: LicenseTimerSignalsConfig{
							LicenseValueConfig: LicenseValueConfig{
								From:     "1.3.6.1.4.1.2604.5.1.5.1.2.0",
								Format:   "text_date",
								Sentinel: []LicenseSentinelPolicy{LicenseSentinelTimerZeroOrNegative},
							},
						},
					},
				}},
			}},
		"topology": {
			input: `
metadata:
  device:
    fields:
      lldp_loc_chassis_id:
        consumers: [topology]
        symbol:
          OID: 1.0.8802.1.1.2.1.3.2.0
          name: lldpLocChassisId
metric_tags:
  - tag: vendor
    consumers: [metrics, topology]
    symbol:
      OID: 1.3.6.1.2.1.1.1.0
      name: sysDescr
topology:
  - kind: lldp_rem
    MIB: LLDP-MIB
    table:
      OID: 1.0.8802.1.1.2.1.4.1
      name: lldpRemTable
    symbols:
      - OID: 1.0.8802.1.1.2.1.4.1.1.6
        name: lldpRemPortIdSubtype
    metric_tags:
      - tag: lldp_rem_index
        index: 1
`,
			want: ProfileDefinition{
				Metadata: MetadataConfig{
					"device": {Fields: map[string]MetadataField{
						"lldp_loc_chassis_id": {
							Consumers: ConsumerSet{ConsumerTopology},
							Symbol: SymbolConfig{
								OID:  "1.0.8802.1.1.2.1.3.2.0",
								Name: "lldpLocChassisId",
							},
						},
					}},
				},
				MetricTags: []GlobalMetricTagConfig{{
					MetricTagConfig: MetricTagConfig{
						Tag: "vendor",
						Symbol: SymbolConfigCompat{
							OID:  "1.3.6.1.2.1.1.1.0",
							Name: "sysDescr",
						},
					},
					Consumers: ConsumerSet{ConsumerMetrics, ConsumerTopology},
				}},
				Topology: []TopologyConfig{{Kind: KindLldpRem, MetricsConfig: MetricsConfig{
					MIB: "LLDP-MIB",
					Table: SymbolConfig{
						OID:  "1.0.8802.1.1.2.1.4.1",
						Name: "lldpRemTable",
					},
					Symbols:    []SymbolConfig{{OID: "1.0.8802.1.1.2.1.4.1.1.6", Name: "lldpRemPortIdSubtype"}},
					MetricTags: []MetricTagConfig{{Tag: "lldp_rem_index", Index: 1}},
				}}},
			}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var got ProfileDefinition
			require.NoError(t, yaml.Unmarshal([]byte(tc.input), &got))
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestProfileDefinition_Clone(t *testing.T) {
	tests := map[string]struct {
		newProfile func() *ProfileDefinition
		mutate     func(*ProfileDefinition)
	}{
		"nil":   {newProfile: func() *ProfileDefinition { return nil }},
		"empty": {newProfile: func() *ProfileDefinition { return &ProfileDefinition{} }},
		"bgp": {newProfile: func() *ProfileDefinition {
			return &ProfileDefinition{
				BGP: []BGPConfig{
					{
						OriginProfileID: "_vendor-bgp.yaml",
						ID:              "peer",
						Kind:            BGPRowKindPeerFamily,
						Identity: BGPIdentityConfig{
							Neighbor: BGPValueConfig{
								Value: "192.0.2.1",
							},
							RemoteAS: BGPValueConfig{
								Value: "65001",
							},
							AddressFamily: BGPAddressFamilyValueConfig{
								BGPValueConfig: BGPValueConfig{
									IndexFromEnd: 2,
									Mapping:      NewExactMapping(map[string]string{"1": "ipv4"}),
								},
							},
							SubsequentAddressFamily: BGPSubsequentAddressFamilyValueConfig{
								BGPValueConfig: BGPValueConfig{
									Value: "unicast",
								},
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
									Table: "peerTable",
									LookupSymbol: SymbolConfigCompat(SymbolConfig{
										OID:  "1.2.3.14",
										Name: "peerIndex",
									}),
									Symbol: SymbolConfig{
										OID:  "1.2.4.1",
										Name: "acceptedPrefixes",
									},
								},
							},
						},
						MetricTags: []MetricTagConfig{
							{Tag: "routing_instance", IndexTransform: []MetricIndexTransform{{Start: 1}}},
						},
					},
				},
			}
		},
			mutate: func(cloned *ProfileDefinition) {
				cloned.BGP[0].State.Symbol.Mapping.Items["1"] = "broken"
				cloned.BGP[0].Routes.Current.Accepted.Symbol.Name = "brokenPrefixes"
				cloned.BGP[0].Routes.Current.Accepted.LookupSymbol.Name = "brokenPeerIndex"
				cloned.BGP[0].MetricTags[0].IndexTransform[0].Start = 2
				cloned.BGP[0].Identity.AddressFamily.IndexFromEnd = 3
			}},
		"licensing": {newProfile: func() *ProfileDefinition {
			return &ProfileDefinition{
				Licensing: []LicensingConfig{
					{
						OriginProfileID: "_vendor-licensing.yaml",
						ID:              "row",
						Identity: LicenseIdentityConfig{
							ID: LicenseValueConfig{
								Value: "license-1",
							},
						},
						State: LicenseStateConfig{
							LicenseValueConfig: LicenseValueConfig{
								Symbol: SymbolConfig{
									OID:  "1.2.3.0",
									Name: "licenseState",
									Mapping: NewExactMapping(map[string]string{
										"1": "healthy",
									}),
								},
							},
							Policy: LicenseStatePolicyDefault,
						},
						Signals: LicenseSignalsConfig{
							Expiry: LicenseTimerSignalsConfig{
								LicenseValueConfig: LicenseValueConfig{
									From:     "1.2.4.0",
									Sentinel: []LicenseSentinelPolicy{LicenseSentinelTimerU32Max},
								},
							},
						},
						MetricTags: []MetricTagConfig{
							{Tag: "license_component", IndexTransform: []MetricIndexTransform{{Start: 1}}},
						},
					},
				},
			}
		},
			mutate: func(cloned *ProfileDefinition) {
				cloned.Licensing[0].State.Symbol.Mapping.Items["1"] = "broken"
				cloned.Licensing[0].Signals.Expiry.Sentinel[0] = LicenseSentinelTimerPre1971
				cloned.Licensing[0].MetricTags[0].IndexTransform[0].Start = 2
			}},
		"topology": {newProfile: func() *ProfileDefinition {
			return &ProfileDefinition{
				Metadata: MetadataConfig{
					"device": {
						Fields: map[string]MetadataField{
							"vendor": {
								Value:     "Cisco",
								Consumers: ConsumerSet{ConsumerMetrics, ConsumerTopology},
							},
						},
					},
				},
				MetricTags: []GlobalMetricTagConfig{
					{
						MetricTagConfig: MetricTagConfig{
							Tag: "vendor",
						},
						Consumers: ConsumerSet{ConsumerMetrics, ConsumerTopology},
					},
				},
				Topology: []TopologyConfig{
					{
						Kind: KindLldpRem,
						MetricsConfig: MetricsConfig{
							Table: SymbolConfig{
								OID:  "1.0.8802.1.1.2.1.4.1",
								Name: "lldpRemTable",
							},
							Symbols: []SymbolConfig{
								{OID: "1.0.8802.1.1.2.1.4.1.1.6", Name: "lldpRemPortIdSubtype"},
							},
							MetricTags: []MetricTagConfig{
								{
									Tag: "lldp_rem_index",
									IndexTransform: []MetricIndexTransform{
										{Start: 1},
									},
								},
							},
						},
					},
				},
			}
		},
			mutate: func(cloned *ProfileDefinition) {
				cloned.Metadata["device"].Fields["vendor"] = MetadataField{
					Value:     "Cisco",
					Consumers: ConsumerSet{ConsumerTopology},
				}
				cloned.MetricTags[0].Consumers[0] = ConsumerTopology
				cloned.Topology[0].MetricTags[0].IndexTransform[0].Start = 2
			}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			profile := tc.newProfile()
			// Build expectations independently so a shallow clone cannot corrupt the oracle.
			want := tc.newProfile()
			cloned := profile.Clone()
			require.Equal(t, want, cloned)
			if tc.mutate != nil {
				tc.mutate(cloned)
				assert.NotEqual(t, want, cloned)
			}
			assert.Equal(t, want, profile)
		})
	}
}
