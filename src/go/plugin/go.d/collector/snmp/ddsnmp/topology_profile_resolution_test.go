// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestTopologyProfiles_FormerUmbrellaCapabilityMatrix(t *testing.T) {
	tests := []struct {
		profile string
		oid     string
		want    string
	}{
		{profile: "alcatel-lucent-ent", oid: "1.3.6.1.4.1.6486.801.1.1.2.1.999", want: "I,S,A"},
		{profile: "alcatel-lucent-ind", oid: "1.3.6.1.4.1.6486.800.1.1.2.1.999", want: "I,A"},
		{profile: "alcatel-lucent-omni-access-wlc", oid: "1.3.6.1.4.1.6486.800.1.1.2.2.2.999", want: "I,A"},
		{profile: "arista", oid: "1.3.6.1.4.1.30065.1.999999", want: ""},
		{profile: "arista-switch", oid: "1.3.6.1.4.1.30065.1.3011.7010.427.48", want: "I,Q,A"},
		{profile: "aruba", oid: "1.3.6.1.4.1.14823.999", want: ""},
		{profile: "aruba-access-point", oid: "1.3.6.1.4.1.14823.1.2.71", want: "I"},
		{profile: "aruba-clearpass", oid: "1.3.6.1.4.1.14823.1.6.1", want: ""},
		{profile: "aruba-cx-switch", oid: "1.3.6.1.4.1.47196.4.1.1.1.1", want: "I,C,Q,S,A"},
		{profile: "aruba-mobility-controller", oid: "1.3.6.1.4.1.14823.1.1.50", want: "I,A"},
		{profile: "aruba-switch", oid: "1.3.6.1.4.1.14823.1.1.36", want: "I,C,Q,S,A"},
		{profile: "aruba-wireless-controller", oid: "1.3.6.1.4.1.14823.1.1.1", want: "I,A"},
		{profile: "cisco", oid: "1.3.6.1.4.1.9.1.1", want: ""},
		{profile: "cisco-3850", oid: "1.3.6.1.4.1.9.1.1745", want: "I,C,Q,S,A,V"},
		{profile: "cisco-access-point", oid: "1.3.6.1.4.1.9.1.525", want: "I"},
		{profile: "cisco-asa", oid: "1.3.6.1.4.1.9.1.669", want: "I,A"},
		{profile: "cisco-asr", oid: "1.3.6.1.4.1.9.1.403", want: "I,A"},
		{profile: "cisco-catalyst", oid: "1.3.6.1.4.1.9.1.2683", want: "I,C,S,A,V"},
		{profile: "cisco-catalyst-wlc", oid: "1.3.6.1.4.1.9.1.2025", want: "I,A,V"},
		{profile: "cisco-csr1000v", oid: "1.3.6.1.4.1.9.1.1537", want: "I,A"},
		{profile: "cisco-icm", oid: "1.3.6.1.4.1.9.1.693", want: ""},
		{profile: "cisco-isr", oid: "1.3.6.1.4.1.9.1.543", want: "I,A"},
		{profile: "cisco-isr-4431", oid: "1.3.6.1.4.1.9.1.1935", want: "I,A"},
		{profile: "cisco-load-balancer", oid: "1.3.6.1.4.1.9.1.824", want: "I,A"},
		{profile: "cisco-ncs", oid: "1.3.6.1.4.1.9.1.2571", want: "I,A"},
		{profile: "cisco-nexus", oid: "1.3.6.1.4.1.9.1.1216", want: "I,C,S,A,V"},
		{profile: "cisco-sb", oid: "1.3.6.1.4.1.9.6.1.1", want: "I,Q,S,A"},
		{profile: "cisco-uc-virtual-machine", oid: "1.3.6.1.4.1.9.1.1348", want: ""},
		{profile: "cisco-wan-optimizer", oid: "1.3.6.1.4.1.9.1.957", want: "I,A"},
		{profile: "dlink-dgs-switch", oid: "1.3.6.1.4.1.171.10.137.1.1", want: "I,Q,S,A"},
		{profile: "juniper", oid: "1.3.6.1.4.1.2636.1.1.1.2.999999", want: ""},
		{profile: "juniper-ex", oid: "1.3.6.1.4.1.2636.1.1.1.2.30", want: "I,Q,A"},
		{profile: "juniper-mx", oid: "1.3.6.1.4.1.2636.1.1.1.2.21", want: "I,A"},
		{profile: "juniper-pulse-secure", oid: "1.3.6.1.4.1.12532.256.1", want: "I,A"},
		{profile: "juniper-qfx", oid: "1.3.6.1.4.1.2636.1.1.1.2.82", want: "I,Q"},
		{profile: "juniper-srx", oid: "1.3.6.1.4.1.2636.1.1.1.2.26", want: "I,C,S,A"},
		{profile: "mikrotik-router", oid: "1.3.6.1.4.1.14988.1", want: "I,C,Q,S,A"},
		{profile: "mikrotik-swos", oid: "1.3.6.1.4.1.14988.2", want: "I,C,S"},
		{profile: "netgear-switch", oid: "1.3.6.1.4.1.4526.100.1.1", want: "I,C,Q,S,A"},
		{profile: "zyxel-switch", oid: "1.3.6.1.4.1.890.1.15", want: "I,Q,S,A"},
	}
	require.Len(t, tests, 40)

	counts := make(map[string]int)
	for _, tc := range tests {
		t.Run(tc.profile, func(t *testing.T) {
			names, kinds := resolvedTopologyProfileFacts(tc.oid, "")
			assert.Contains(t, names, tc.profile)
			assert.Equal(t, tc.want, topologyCapabilityString(kinds))
		})
		for capability := range strings.SplitSeq(tc.want, ",") {
			if capability != "" {
				counts[capability]++
			}
		}
	}

	assert.Equal(t, map[string]int{"I": 33, "C": 9, "Q": 11, "S": 13, "A": 29, "V": 4}, counts)
}

func TestMikroTikProfiles_LLDPIsRouterOSOnlyAndDeviationIsRB750Gr3Only(t *testing.T) {
	tests := map[string]struct {
		sysObjectID  string
		sysDescr     string
		profile      string
		profileCount int
		anchorOID    string
	}{
		"generic RouterOS": {
			sysObjectID:  "1.3.6.1.4.1.14988.1",
			sysDescr:     "RouterOS CCR2004-1G-12S+2XS",
			profile:      "mikrotik-router",
			profileCount: 2,
			anchorOID:    "1.0.8802.1.1.2.1.4.2.1.3",
		},
		"RB750Gr3 RouterOS deviation": {
			sysObjectID:  "1.3.6.1.4.1.14988.1",
			sysDescr:     "RouterOS RB750Gr3",
			profile:      "topology-role-mikrotik-rb750gr3",
			profileCount: 3,
			anchorOID:    "1.0.8802.1.1.2.1.4.2.1.1",
		},
		"SwOS": {
			sysObjectID:  "1.3.6.1.4.1.14988.2",
			sysDescr:     "CSS610-8G-2S+ SwOS",
			profile:      "mikrotik-swos",
			profileCount: 2,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			profiles := DefaultCatalog().Resolve(ResolveRequest{
				SysObjectID: tc.sysObjectID,
				SysDescr:    tc.sysDescr,
			}).Profiles()
			require.Len(t, profiles, tc.profileCount)
			require.Equal(t, tc.profile, stripFileNameExt(profiles[0].SourceFile))

			var anchors []string
			for _, profile := range profiles {
				for _, row := range profile.Definition.Topology {
					if row.Kind == ddprofiledefinition.KindLldpRemManAddr {
						require.Len(t, row.Symbols, 1)
						anchors = append(anchors, row.Symbols[0].OID)
					}
				}
			}
			if tc.anchorOID == "" {
				require.Empty(t, anchors)
			} else {
				require.Equal(t, []string{tc.anchorOID}, anchors)
			}
		})
	}
}

func TestMikroTikProfiles_AreSplitByOS(t *testing.T) {
	routerOS, err := LoadProfileByName("mikrotik-router")
	require.NoError(t, err)
	assert.True(t, routerOS.HasExtension("_mikrotik-hardware.yaml"))
	assert.True(t, routerOS.HasExtension("_std-lldp-mib.yaml"))
	assert.True(t, routerOS.HasExtension("_std-topology-arp-mib.yaml"))
	assert.True(t, routerOS.HasExtension("_mikrotik-ipsec.yaml"))
	assert.True(t, routerOS.HasExtension("_mikrotik-routeros-licensing.yaml"))

	swOS, err := LoadProfileByName("mikrotik-swos")
	require.NoError(t, err)
	assert.True(t, swOS.HasExtension("_mikrotik-hardware.yaml"))
	assert.True(t, swOS.HasExtension("_std-topology-fdb-mib.yaml"))
	assert.True(t, swOS.HasExtension("_std-topology-stp-mib.yaml"))
	assert.False(t, swOS.HasExtension("_std-lldp-mib.yaml"))
	assert.False(t, swOS.HasExtension("_std-topology-arp-mib.yaml"))
	assert.False(t, swOS.HasExtension("_std-topology-q-bridge-mib.yaml"))
	assert.False(t, swOS.HasExtension("_mikrotik-ipsec.yaml"))
	assert.False(t, swOS.HasExtension("_mikrotik-routeros-licensing.yaml"))
}

func TestCiscoProfiles_CDPIsTopologyOnly(t *testing.T) {
	for _, name := range []string{"cisco-catalyst", "cisco-sb"} {
		t.Run(name, func(t *testing.T) {
			profile, err := LoadProfileByName(name)
			require.NoError(t, err)

			assert.False(t, profile.HasExtension("_std-cdp-mib"))
			assert.True(t, profile.HasExtension("_std-topology-cdp-mib"))

			var ordinaryCDPMetrics []string
			for _, metric := range profile.Definition.Metrics {
				if metric.MIB != "CISCO-CDP-MIB" {
					continue
				}
				name := metric.Symbol.Name
				if name == "" {
					name = metric.Table.Name
				}
				ordinaryCDPMetrics = append(ordinaryCDPMetrics, name)
			}
			assert.Empty(t, ordinaryCDPMetrics)

			var kinds []ddprofiledefinition.TopologyKind
			for _, topology := range profile.Definition.Topology {
				kinds = append(kinds, topology.Kind)
			}
			assert.Contains(t, kinds, ddprofiledefinition.KindCdpCache)
		})
	}
}

func TestTopologyRoleProfiles_AllExactSelectorsResolve(t *testing.T) {
	tests := []struct {
		profile string
		count   int
		kinds   []ddprofiledefinition.TopologyKind
	}{
		{
			profile: "topology-role-classic-bridge",
			count:   6,
			kinds: []ddprofiledefinition.TopologyKind{
				ddprofiledefinition.KindIfName,
				ddprofiledefinition.KindIfStatus,
				ddprofiledefinition.KindIfDuplex,
				ddprofiledefinition.KindFdbEntry,
			},
		},
		{
			profile: "topology-role-qbridge",
			count:   48,
			kinds: []ddprofiledefinition.TopologyKind{
				ddprofiledefinition.KindIfName,
				ddprofiledefinition.KindIfStatus,
				ddprofiledefinition.KindIfDuplex,
				ddprofiledefinition.KindQbridgeFdbEntry,
				ddprofiledefinition.KindQbridgeVlanEntry,
			},
		},
		{
			profile: "topology-role-l3-neighbor",
			count:   54,
			kinds: []ddprofiledefinition.TopologyKind{
				ddprofiledefinition.KindArpEntry,
				ddprofiledefinition.KindArpLegacyEntry,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.profile, func(t *testing.T) {
			profile, err := LoadProfileByName(tc.profile)
			require.NoError(t, err)

			var exactOIDs []string
			for _, rule := range profile.Definition.Selector {
				if len(rule.SysDescr.Include) > 0 || len(rule.SysDescr.Exclude) > 0 {
					continue
				}
				for _, oid := range rule.SysObjectID.Include {
					if ddprofiledefinition.IsPlainOid(oid) {
						exactOIDs = append(exactOIDs, oid)
					}
				}
			}
			require.Len(t, exactOIDs, tc.count)

			for _, oid := range exactOIDs {
				t.Run(oid, func(t *testing.T) {
					names, kinds := resolvedTopologyProfileFacts(oid, "")
					assert.Contains(t, names, tc.profile)
					for _, kind := range tc.kinds {
						assert.Contains(t, kinds, kind)
					}
				})
			}
		})
	}
}

func TestTopologyRoleProfiles_QualifiedSelectors(t *testing.T) {
	tests := map[string]struct {
		oid            string
		sysDescr       string
		wantProfiles   []string
		absentProfiles []string
		wantKinds      []ddprofiledefinition.TopologyKind
		absentKinds    []ddprofiledefinition.TopologyKind
	}{
		"Ubiquiti enterprise-root Flex switch": {
			oid:          "1.3.6.1.4.1",
			sysDescr:     "Ubiquiti USW-Flex-XG",
			wantProfiles: []string{"topology-role-qbridge", "topology-role-l3-neighbor"},
			wantKinds:    []ddprofiledefinition.TopologyKind{ddprofiledefinition.KindQbridgeFdbEntry, ddprofiledefinition.KindArpEntry},
		},
		"unrelated enterprise-root device": {
			oid:            "1.3.6.1.4.1",
			sysDescr:       "unrelated enterprise device",
			absentProfiles: []string{"topology-role-qbridge", "topology-role-l3-neighbor"},
			absentKinds:    []ddprofiledefinition.TopologyKind{ddprofiledefinition.KindQbridgeFdbEntry, ddprofiledefinition.KindArpEntry},
		},
		"SONiC Net-SNMP": {
			oid:          "1.3.6.1.4.1.8072.3.2.10",
			sysDescr:     "SONiC Software Version",
			wantProfiles: []string{"topology-role-l3-neighbor"},
			wantKinds:    []ddprofiledefinition.TopologyKind{ddprofiledefinition.KindArpEntry},
		},
		"UniFi switch with generic Net-SNMP identity": {
			oid:          "1.3.6.1.4.1.8072.3.2.10",
			sysDescr:     "linux ubnt UniFi Switch",
			wantProfiles: []string{"topology-role-qbridge", "topology-role-l3-neighbor"},
			wantKinds: []ddprofiledefinition.TopologyKind{
				ddprofiledefinition.KindIfName,
				ddprofiledefinition.KindQbridgeFdbEntry,
				ddprofiledefinition.KindArpEntry,
			},
			absentKinds: []ddprofiledefinition.TopologyKind{
				ddprofiledefinition.KindFdbEntry,
				ddprofiledefinition.KindStpPort,
				ddprofiledefinition.KindLldpRem,
			},
		},
		"UniFi gateway with generic Net-SNMP identity": {
			oid:            "1.3.6.1.4.1.8072.3.2.10",
			sysDescr:       "Linux Router 6.6.43-ui-ipq9574",
			wantProfiles:   []string{"topology-role-l3-neighbor"},
			absentProfiles: []string{"topology-role-qbridge"},
			wantKinds:      []ddprofiledefinition.TopologyKind{ddprofiledefinition.KindArpEntry},
			absentKinds: []ddprofiledefinition.TopologyKind{
				ddprofiledefinition.KindFdbEntry,
				ddprofiledefinition.KindQbridgeFdbEntry,
				ddprofiledefinition.KindStpPort,
				ddprofiledefinition.KindLldpRem,
			},
		},
		"UniFi AP with exact enterprise identity": {
			oid:            "1.3.6.1.4.1.41112",
			sysDescr:       "Ubiquiti UniFi U6 Mesh",
			wantProfiles:   []string{"topology-role-l3-neighbor"},
			absentProfiles: []string{"topology-role-qbridge"},
			wantKinds:      []ddprofiledefinition.TopologyKind{ddprofiledefinition.KindArpLegacyEntry},
			absentKinds: []ddprofiledefinition.TopologyKind{
				ddprofiledefinition.KindFdbEntry,
				ddprofiledefinition.KindQbridgeFdbEntry,
				ddprofiledefinition.KindStpPort,
				ddprofiledefinition.KindLldpRem,
			},
		},
		"unrelated generic Net-SNMP agent": {
			oid:            "1.3.6.1.4.1.8072.3.2.10",
			sysDescr:       "Linux Net-SNMP agent",
			absentProfiles: []string{"topology-role-qbridge", "topology-role-l3-neighbor"},
			absentKinds:    []ddprofiledefinition.TopologyKind{ddprofiledefinition.KindQbridgeFdbEntry, ddprofiledefinition.KindArpEntry},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			names, kinds := resolvedTopologyProfileFacts(tc.oid, tc.sysDescr)
			for _, profile := range tc.wantProfiles {
				assert.Contains(t, names, profile)
			}
			for _, profile := range tc.absentProfiles {
				assert.NotContains(t, names, profile)
			}
			for _, kind := range tc.wantKinds {
				assert.Contains(t, kinds, kind)
			}
			for _, kind := range tc.absentKinds {
				assert.NotContains(t, kinds, kind)
			}
		})
	}
}

func TestTopologyRoleProfiles_ArubaGuard(t *testing.T) {
	roleProfiles := []string{
		"topology-role-classic-bridge",
		"topology-role-qbridge",
		"topology-role-l3-neighbor",
		"topology-role-stp",
	}

	names, kinds := resolvedTopologyProfileFacts("1.3.6.1.4.1.14823.1.1.36", "Aruba switch")
	for _, name := range roleProfiles {
		assert.Contains(t, names, name)
	}
	for _, kind := range []ddprofiledefinition.TopologyKind{
		ddprofiledefinition.KindIfName,
		ddprofiledefinition.KindFdbEntry,
		ddprofiledefinition.KindQbridgeFdbEntry,
		ddprofiledefinition.KindStpPort,
		ddprofiledefinition.KindArpEntry,
	} {
		assert.Contains(t, kinds, kind)
	}

	controllerSuffixes := []string{
		"1", "2", "3", "4", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20",
		"23", "24", "25", "26", "27", "28", "29", "30", "31", "32", "33", "34", "35", "39", "40", "41", "42",
		"47", "48", "50", "53", "54", "55", "56",
	}
	require.Len(t, controllerSuffixes, 42)
	for _, suffix := range controllerSuffixes {
		t.Run(suffix, func(t *testing.T) {
			oid := "1.3.6.1.4.1.14823.1.1." + suffix
			names, kinds := resolvedTopologyProfileFacts(oid, "Aruba controller")
			for _, name := range roleProfiles {
				assert.NotContains(t, names, name)
			}
			assert.Contains(t, kinds, ddprofiledefinition.KindIfName)
			assert.Contains(t, kinds, ddprofiledefinition.KindArpEntry)
			assert.NotContains(t, kinds, ddprofiledefinition.KindFdbEntry)
			assert.NotContains(t, kinds, ddprofiledefinition.KindQbridgeFdbEntry)
			assert.NotContains(t, kinds, ddprofiledefinition.KindStpPort)
		})
	}
}

func TestTopologyRoleProfiles_CollisionGuards(t *testing.T) {
	t.Run("Nexus excludes UCS", func(t *testing.T) {
		names, kinds := resolvedTopologyProfileFacts("1.3.6.1.4.1.9.12.3.1.3.1062", "Cisco UCS")
		assert.NotContains(t, names, "cisco-nexus")
		assert.NotContains(t, kinds, ddprofiledefinition.KindFdbEntry)
		assert.NotContains(t, kinds, ddprofiledefinition.KindStpPort)
		assert.NotContains(t, kinds, ddprofiledefinition.KindArpEntry)

		names, kinds = resolvedTopologyProfileFacts("1.3.6.1.4.1.9.12.3.1.3.1063", "Cisco Nexus")
		assert.Contains(t, names, "cisco-nexus")
		assert.Contains(t, kinds, ddprofiledefinition.KindFdbEntry)
		assert.Contains(t, kinds, ddprofiledefinition.KindStpPort)
		assert.Contains(t, kinds, ddprofiledefinition.KindArpEntry)
	})

	t.Run("Netgear switch excludes ReadyNAS", func(t *testing.T) {
		names, kinds := resolvedTopologyProfileFacts("1.3.6.1.4.1.4526.100.16.1", "NETGEAR ReadyNAS")
		assert.NotContains(t, names, "netgear-switch")
		assert.NotContains(t, kinds, ddprofiledefinition.KindFdbEntry)
		assert.NotContains(t, kinds, ddprofiledefinition.KindArpEntry)

		names, kinds = resolvedTopologyProfileFacts("1.3.6.1.4.1.4526.100.1.1", "NETGEAR switch")
		assert.Contains(t, names, "netgear-switch")
		assert.Contains(t, kinds, ddprofiledefinition.KindFdbEntry)
		assert.Contains(t, kinds, ddprofiledefinition.KindArpEntry)
	})
}

func TestTopologyRoleProfiles_RefinedProductSelectors(t *testing.T) {
	tests := []struct {
		oid         string
		profile     string
		wantKinds   []ddprofiledefinition.TopologyKind
		absentKinds []ddprofiledefinition.TopologyKind
	}{
		{
			oid:       "1.3.6.1.4.1.9.1.2683",
			profile:   "cisco-catalyst",
			wantKinds: []ddprofiledefinition.TopologyKind{ddprofiledefinition.KindFdbEntry, ddprofiledefinition.KindStpPort, ddprofiledefinition.KindArpEntry},
		},
		{
			oid:       "1.3.6.1.4.1.9.1.3155",
			profile:   "cisco-catalyst",
			wantKinds: []ddprofiledefinition.TopologyKind{ddprofiledefinition.KindFdbEntry, ddprofiledefinition.KindStpPort, ddprofiledefinition.KindArpEntry},
		},
		{
			oid:         "1.3.6.1.4.1.9.1.2571",
			profile:     "cisco-ncs",
			wantKinds:   []ddprofiledefinition.TopologyKind{ddprofiledefinition.KindIfName, ddprofiledefinition.KindArpEntry},
			absentKinds: []ddprofiledefinition.TopologyKind{ddprofiledefinition.KindFdbEntry},
		},
		{
			oid:         "1.3.6.1.4.1.9.1.1899",
			profile:     "topology-role-qbridge",
			wantKinds:   []ddprofiledefinition.TopologyKind{ddprofiledefinition.KindQbridgeFdbEntry, ddprofiledefinition.KindArpEntry},
			absentKinds: []ddprofiledefinition.TopologyKind{ddprofiledefinition.KindFdbEntry},
		},
		{
			oid:         "1.3.6.1.4.1.41112.1",
			profile:     "topology-role-qbridge",
			wantKinds:   []ddprofiledefinition.TopologyKind{ddprofiledefinition.KindQbridgeFdbEntry},
			absentKinds: []ddprofiledefinition.TopologyKind{ddprofiledefinition.KindFdbEntry, ddprofiledefinition.KindArpEntry},
		},
	}

	for _, tc := range tests {
		t.Run(tc.oid, func(t *testing.T) {
			names, kinds := resolvedTopologyProfileFacts(tc.oid, "")
			assert.Contains(t, names, tc.profile)
			for _, kind := range tc.wantKinds {
				assert.Contains(t, kinds, kind)
			}
			for _, kind := range tc.absentKinds {
				assert.NotContains(t, kinds, kind)
			}
		})
	}
}

func resolvedTopologyProfileFacts(sysObjectID, sysDescr string) (map[string]struct{}, map[ddprofiledefinition.TopologyKind]struct{}) {
	names := make(map[string]struct{})
	kinds := make(map[ddprofiledefinition.TopologyKind]struct{})
	for _, profile := range FindProfiles(sysObjectID, sysDescr, nil) {
		names[stripFileNameExt(profile.SourceFile)] = struct{}{}
		for _, topology := range profile.Definition.Topology {
			kinds[topology.Kind] = struct{}{}
		}
	}
	return names, kinds
}

func topologyCapabilityString(kinds map[ddprofiledefinition.TopologyKind]struct{}) string {
	var capabilities []string

	interfaceKinds := []ddprofiledefinition.TopologyKind{
		ddprofiledefinition.KindIfName,
		ddprofiledefinition.KindIfStatus,
		ddprofiledefinition.KindIfDuplex,
	}
	if countTopologyKinds(kinds, interfaceKinds) == len(interfaceKinds) {
		capabilities = append(capabilities, "I")
	} else if countTopologyKinds(kinds, interfaceKinds) != 0 {
		capabilities = append(capabilities, "I(partial)")
	}
	if _, ok := kinds[ddprofiledefinition.KindFdbEntry]; ok {
		capabilities = append(capabilities, "C")
	}
	qbridgeKinds := []ddprofiledefinition.TopologyKind{
		ddprofiledefinition.KindQbridgeFdbEntry,
		ddprofiledefinition.KindQbridgeVlanEntry,
	}
	if countTopologyKinds(kinds, qbridgeKinds) == len(qbridgeKinds) {
		capabilities = append(capabilities, "Q")
	} else if countTopologyKinds(kinds, qbridgeKinds) != 0 {
		capabilities = append(capabilities, "Q(partial)")
	}
	if _, ok := kinds[ddprofiledefinition.KindStpPort]; ok {
		capabilities = append(capabilities, "S")
	}
	arpKinds := []ddprofiledefinition.TopologyKind{
		ddprofiledefinition.KindArpEntry,
		ddprofiledefinition.KindArpLegacyEntry,
	}
	if countTopologyKinds(kinds, arpKinds) == len(arpKinds) {
		capabilities = append(capabilities, "A")
	} else if countTopologyKinds(kinds, arpKinds) != 0 {
		capabilities = append(capabilities, "A(partial)")
	}
	if _, ok := kinds[ddprofiledefinition.KindVtpVlan]; ok {
		capabilities = append(capabilities, "V")
	}
	return strings.Join(capabilities, ",")
}

func countTopologyKinds(kinds map[ddprofiledefinition.TopologyKind]struct{}, candidates []ddprofiledefinition.TopologyKind) int {
	var count int
	for _, kind := range candidates {
		if _, ok := kinds[kind]; ok {
			count++
		}
	}
	return count
}
