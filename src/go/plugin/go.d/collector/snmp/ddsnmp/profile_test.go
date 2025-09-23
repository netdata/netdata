// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
)

func Test_loadDDSnmpProfiles(t *testing.T) {
	dir, _ := filepath.Abs("../../../config/go.d/snmp.profiles/default")

	f, err := os.Open(dir)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	profiles, err := loadProfilesFromDir(dir, multipath.New(dir))
	require.NoError(t, err)

	require.NotEmpty(t, profiles)

	names, err := f.Readdirnames(-1)
	require.NoError(t, err)

	var want int64
	for _, name := range names {
		want += metrix.Bool(!strings.HasPrefix(name, "_"))
	}

	require.Equal(t, want-1 /*README.md*/, int64(len(profiles)))
}

func Test_FindProfiles(t *testing.T) {
	test := map[string]struct {
		sysObjOId      string
		manualProfiles []string
		wanProfiles    []string
	}{
		"MikroTik": {
			sysObjOId:   "1.3.6.1.4.1.14988.1",
			wanProfiles: []string{"mikrotik-router", "generic-device"},
		},
		"net-snmp linux": {
			sysObjOId:   "1.3.6.1.4.1.8072.3.2.10",
			wanProfiles: []string{"net-snmp", "generic-device"},
		},
		"Kyocera printer": {
			sysObjOId:   "1.3.6.1.4.1.1347.41",
			wanProfiles: []string{"kyocera-printer", "generic-device"},
		},
		"APC pdu": {
			sysObjOId:   "1.3.6.1.4.1.318.1.3.4.5",
			wanProfiles: []string{"apc-pdu", "apc-ups", "apc", "generic-device"},
		},
		"IBM RackSwitch G8052-2 ": {
			sysObjOId:   "1.3.6.1.4.1.26543.1.7.7",
			wanProfiles: []string{"generic-device"},
		},
		"Aruba WLC 7210": {
			sysObjOId:   "1.3.6.1.4.1.14823.1.1.32",
			wanProfiles: []string{"aruba-wireless-controller", "aruba-switch", "aruba", "generic-device"},
		},
		"FortiGate 200": {
			sysObjOId:   "1.3.6.1.4.1.12356.15.200",
			wanProfiles: []string{"generic-device"},
		},
		"Cisco 7204 VXR": {
			sysObjOId:   "1.3.6.1.4.1.9.1.223",
			wanProfiles: []string{"cisco", "generic-device"},
		},
		"NetApp Cluster": {
			sysObjOId:   "1.3.6.1.4.1.789.2.5",
			wanProfiles: []string{"netapp", "generic-device"},
		},
		"Zyxel ZyAIR B-1000": {
			sysObjOId:   "1.3.6.1.4.1.890.1.2",
			wanProfiles: []string{"generic-device"},
		},
		"HP ProCurve 2650-PWR": {
			sysObjOId:   "1.3.6.1.4.1.11.2.3.7.11.35",
			wanProfiles: []string{"hp-icf-switch", "generic-device"},
		},
		"Aruba  2930F-8G-PoE+-2SFP+": {
			sysObjOId:   "1.3.6.1.4.1.11.2.3.7.11.181.16",
			wanProfiles: []string{"hp-icf-switch", "generic-device"},
		},
		"HP LaserJet 4050 Series": {
			sysObjOId:   "1.3.6.1.4.1.11.2.3.9.1",
			wanProfiles: []string{"generic-device"},
		},
		"Huawei H3C s2008-HI": {
			sysObjOId:   "1.3.6.1.4.1.2011.10.1.152",
			wanProfiles: []string{"generic-device"},
		},
		"Avaya AVX571040": {
			sysObjOId:   "1.3.6.1.4.1.6889.1.69.4",
			wanProfiles: []string{"generic-device"},
		},
		"Aruba IAP-225": {
			sysObjOId:   "1.3.6.1.4.1.14823.1.2.59",
			wanProfiles: []string{"aruba-access-point", "aruba", "generic-device"},
		},
		"Brocade MLXe-16 slot": {
			sysObjOId:   "1.3.6.1.4.1.1991.1.3.55.1.2",
			wanProfiles: []string{"generic-device"},
		},
		"FortiGate-100D": {
			sysObjOId:   "1.3.6.1.4.1.12356.101.1.1004",
			wanProfiles: []string{"fortinet-fortigate", "generic-device"},
		},
		"Summit X450-24x": {
			sysObjOId:   "1.3.6.1.4.1.1916.2.65",
			wanProfiles: []string{"extreme-switching", "generic-device"},
		},
		"Meraki MX84": {
			sysObjOId:   "1.3.6.1.4.1.29671.2.109",
			wanProfiles: []string{"meraki", "generic-device"},
		},
		"Palo Alto WF-500": {
			sysObjOId:   "1.3.6.1.4.1.25461.2.3.33",
			wanProfiles: []string{"palo-alto", "generic-device"},
		},
		"Cisco UCS 6120XP": {
			sysObjOId:   "1.3.6.1.4.1.9.12.3.1.3.847",
			wanProfiles: []string{"cisco-nexus", "cisco", "generic-device"},
		},
		"Cisco 5520 Wireless Controller": {
			sysObjOId:   "1.3.6.1.4.1.9.1.2170",
			wanProfiles: []string{"cisco-legacy-wlc", "cisco", "generic-device"},
		},
		"Linksys BEFSX41": {
			sysObjOId:   "1.3.6.1.4.1.3955.1.1",
			wanProfiles: []string{"linksys", "generic-device"},
		},
		"Juniper ERX-705": {
			sysObjOId:   "1.3.6.1.4.1.4874.1.1.1.1.4",
			wanProfiles: []string{"generic-device"},
		},

		"no match": {
			sysObjOId:   "0.1.2.3",
			wanProfiles: nil,
		},
		"no sysObjectID, manual profile applied": {
			sysObjOId:      "",
			manualProfiles: []string{"generic-device"},
			wanProfiles:    []string{"generic-device"},
		},
	}

	for name, test := range test {
		t.Run(name, func(t *testing.T) {
			profiles := FindProfiles(test.sysObjOId, test.manualProfiles)

			var names []string
			for _, p := range profiles {
				names = append(names, stripFileNameExt(p.SourceFile))
			}

			assert.Equal(t, test.wanProfiles, names)
		})
	}
}

func Test_Profile_merge(t *testing.T) {
	profiles := FindProfiles("1.3.6.1.4.1.9.1.1216", nil) // cisco-nexus

	i := slices.IndexFunc(profiles, func(p *Profile) bool {
		return strings.HasSuffix(p.SourceFile, "cisco-nexus.yaml")
	})

	require.GreaterOrEqual(t, 1, 0)

	for _, m := range profiles[i].Definition.Metrics {
		if m.IsColumn() && m.Table.Name == "ciscoMemoryPoolTable" {
			for _, s := range m.Symbols {
				assert.NotEqual(t, "memory.used", s.Name)
			}
		}
	}
}

func TestDeduplicateMetricsAcrossProfiles(t *testing.T) {
	tests := map[string]struct {
		profiles []*Profile
		expected []*Profile
	}{
		"no profiles": {
			profiles: []*Profile{},
			expected: []*Profile{},
		},
		"single profile no duplicates": {
			profiles: []*Profile{
				{
					SourceFile: "profile1.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.3.0",
									Name: "sysUpTime",
								},
							},
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.5.0",
									Name: "sysName",
								},
							},
						},
					},
				},
			},
			expected: []*Profile{
				{
					SourceFile: "profile1.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.3.0",
									Name: "sysUpTime",
								},
							},
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.5.0",
									Name: "sysName",
								},
							},
						},
					},
				},
			},
		},
		"duplicate scalar metrics across profiles - first wins": {
			profiles: []*Profile{
				{
					SourceFile: "specific-profile.yaml", // Most specific, comes first
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.3.0",
									Name: "sysUpTime",
								},
							},
						},
					},
				},
				{
					SourceFile: "generic-profile.yaml", // Less specific, comes second
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.3.0",
									Name: "sysUpTime",
								},
							},
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.5.0",
									Name: "sysName",
								},
							},
						},
					},
				},
			},
			expected: []*Profile{
				{
					SourceFile: "specific-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.3.0",
									Name: "sysUpTime",
								},
							},
						},
					},
				},
				{
					SourceFile: "generic-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.5.0",
									Name: "sysName",
								},
							},
						},
					},
				},
			},
		},
		"duplicate table metrics - exact same symbols": {
			profiles: []*Profile{
				{
					SourceFile: "specific-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.2.2",
									Name: "ifTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.2.1.2.2.1.10",
										Name: "ifInOctets",
									},
									{
										OID:  "1.3.6.1.2.1.2.2.1.16",
										Name: "ifOutOctets",
									},
								},
								MetricTags: []ddprofiledefinition.MetricTagConfig{
									{
										Tag: "interface",
										Symbol: ddprofiledefinition.SymbolConfigCompat{
											OID:  "1.3.6.1.2.1.2.2.1.2",
											Name: "ifDescr",
										},
									},
								},
							},
						},
					},
				},
				{
					SourceFile: "generic-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.2.2",
									Name: "ifTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.2.1.2.2.1.10",
										Name: "ifInOctets",
									},
									{
										OID:  "1.3.6.1.2.1.2.2.1.16",
										Name: "ifOutOctets",
									},
								},
								MetricTags: []ddprofiledefinition.MetricTagConfig{
									{
										Tag: "if_name", // Different tag, but ignored in key
										Symbol: ddprofiledefinition.SymbolConfigCompat{
											OID:  "1.3.6.1.2.1.31.1.1.1.1",
											Name: "ifName",
										},
										Table: "ifXTable",
									},
								},
							},
						},
					},
				},
			},
			expected: []*Profile{
				{
					SourceFile: "specific-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.2.2",
									Name: "ifTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.2.1.2.2.1.10",
										Name: "ifInOctets",
									},
									{
										OID:  "1.3.6.1.2.1.2.2.1.16",
										Name: "ifOutOctets",
									},
								},
								MetricTags: []ddprofiledefinition.MetricTagConfig{
									{
										Tag: "interface",
										Symbol: ddprofiledefinition.SymbolConfigCompat{
											OID:  "1.3.6.1.2.1.2.2.1.2",
											Name: "ifDescr",
										},
									},
								},
							},
						},
					},
				},
				{
					SourceFile: "generic-profile.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{}, // Empty because it's a duplicate
					},
				},
			},
		},
		"same table different symbols - keep both": {
			profiles: []*Profile{
				{
					SourceFile: "profile1.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.2.2",
									Name: "ifTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.2.1.2.2.1.10",
										Name: "ifInOctets",
									},
								},
							},
						},
					},
				},
				{
					SourceFile: "profile2.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.2.2",
									Name: "ifTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.2.1.2.2.1.16",
										Name: "ifOutOctets",
									},
								},
							},
						},
					},
				},
			},
			expected: []*Profile{
				{
					SourceFile: "profile1.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.2.2",
									Name: "ifTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.2.1.2.2.1.10",
										Name: "ifInOctets",
									},
								},
							},
						},
					},
				},
				{
					SourceFile: "profile2.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.2.2",
									Name: "ifTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.2.1.2.2.1.16",
										Name: "ifOutOctets",
									},
								},
							},
						},
					},
				},
			},
		},
		"symbols in different order - still duplicate": {
			profiles: []*Profile{
				{
					SourceFile: "profile1.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.2.2",
									Name: "ifTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.2.1.2.2.1.10",
										Name: "ifInOctets",
									},
									{
										OID:  "1.3.6.1.2.1.2.2.1.16",
										Name: "ifOutOctets",
									},
								},
							},
						},
					},
				},
				{
					SourceFile: "profile2.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.2.2",
									Name: "ifTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.2.1.2.2.1.16",
										Name: "ifOutOctets",
									},
									{
										OID:  "1.3.6.1.2.1.2.2.1.10",
										Name: "ifInOctets",
									},
								},
							},
						},
					},
				},
			},
			expected: []*Profile{
				{
					SourceFile: "profile1.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.2.2",
									Name: "ifTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.2.1.2.2.1.10",
										Name: "ifInOctets",
									},
									{
										OID:  "1.3.6.1.2.1.2.2.1.16",
										Name: "ifOutOctets",
									},
								},
							},
						},
					},
				},
				{
					SourceFile: "profile2.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{}, // Empty because symbols are sorted in key
					},
				},
			},
		},
		"order matters - specific profiles first": {
			// Profiles are already sorted by FindProfiles, most specific first
			profiles: []*Profile{
				{
					SourceFile: "cisco-nexus-9000.yaml", // Most specific
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.3.0",
									Name: "sysUpTime",
								},
							},
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.9.9.305.1.1.1.0",
									Name: "cempMemPoolUsed",
								},
							},
						},
					},
				},
				{
					SourceFile: "cisco-nexus.yaml", // Less specific
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.3.0",
									Name: "sysUpTime",
								},
							},
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.9.9.109.1.1.1.1.7",
									Name: "cpmCPUTotal5minRev",
								},
							},
						},
					},
				},
				{
					SourceFile: "generic-device.yaml", // Least specific
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.3.0",
									Name: "sysUpTime",
								},
							},
							{
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.2.2",
									Name: "ifTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.2.1.2.2.1.10",
										Name: "ifInOctets",
									},
								},
							},
						},
					},
				},
			},
			expected: []*Profile{
				{
					SourceFile: "cisco-nexus-9000.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.3.0",
									Name: "sysUpTime",
								},
							},
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.9.9.305.1.1.1.0",
									Name: "cempMemPoolUsed",
								},
							},
						},
					},
				},
				{
					SourceFile: "cisco-nexus.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.9.9.109.1.1.1.1.7",
									Name: "cpmCPUTotal5minRev",
								},
							},
						},
					},
				},
				{
					SourceFile: "generic-device.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.2.2",
									Name: "ifTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.2.1.2.2.1.10",
										Name: "ifInOctets",
									},
								},
							},
						},
					},
				},
			},
		},
		"empty metrics in some profiles": {
			profiles: []*Profile{
				{
					SourceFile: "profile1.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.3.0",
									Name: "sysUpTime",
								},
							},
						},
					},
				},
				{
					SourceFile: "profile2.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{},
					},
				},
				{
					SourceFile: "profile3.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.3.0",
									Name: "sysUpTime",
								},
							},
						},
					},
				},
			},
			expected: []*Profile{
				{
					SourceFile: "profile1.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.3.0",
									Name: "sysUpTime",
								},
							},
						},
					},
				},
				{
					SourceFile: "profile2.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{},
					},
				},
				{
					SourceFile: "profile3.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{}, // Removed as duplicate
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Make a deep copy of profiles to avoid modifying test data
			profiles := make([]*Profile, len(tc.profiles))
			for i, p := range tc.profiles {
				profiles[i] = p.clone()
			}

			deduplicateMetricsAcrossProfiles(profiles)

			require.Equal(t, len(tc.expected), len(profiles))

			for i, expectedProf := range tc.expected {
				assert.Equal(t, expectedProf.SourceFile, profiles[i].SourceFile)
				assert.Equal(t, len(expectedProf.Definition.Metrics), len(profiles[i].Definition.Metrics))

				// Compare metrics
				for j, expectedMetric := range expectedProf.Definition.Metrics {
					actualMetric := profiles[i].Definition.Metrics[j]

					if expectedMetric.IsScalar() {
						assert.Equal(t, expectedMetric.Symbol.OID, actualMetric.Symbol.OID)
						assert.Equal(t, expectedMetric.Symbol.Name, actualMetric.Symbol.Name)
					} else {
						assert.Equal(t, expectedMetric.Table.OID, actualMetric.Table.OID)
						assert.Equal(t, expectedMetric.Table.Name, actualMetric.Table.Name)
						assert.Equal(t, len(expectedMetric.Symbols), len(actualMetric.Symbols))
					}
				}
			}
		})
	}
}

func TestSortProfilesBySpecificity(t *testing.T) {
	tests := map[string]struct {
		profiles    []string // Source file names
		matchedOIDs []string // OIDs in the same order as profiles
		expected    []string // Expected order of profile source files
	}{
		"empty profiles": {
			profiles:    []string{},
			matchedOIDs: []string{},
			expected:    []string{},
		},
		"single profile": {
			profiles:    []string{"cisco.yaml"},
			matchedOIDs: []string{"1.3.6.1.4.1.9.1.669"},
			expected:    []string{"cisco.yaml"},
		},
		"exact OID vs pattern - same length": {
			profiles:    []string{"pattern.yaml", "exact.yaml"},
			matchedOIDs: []string{"1.3.6.1.4.1.9.1.66*", "1.3.6.1.4.1.9.1.669"},
			expected:    []string{"exact.yaml", "pattern.yaml"},
		},
		"longer OID wins": {
			profiles:    []string{"short.yaml", "long.yaml", "medium.yaml"},
			matchedOIDs: []string{"1.3.6.1.4.1.9.*", "1.3.6.1.4.1.9.1.669", "1.3.6.1.4.1.9.1.*"},
			expected:    []string{"long.yaml", "medium.yaml", "short.yaml"},
		},
		"complex mix - length, exact, and patterns": {
			profiles: []string{
				"generic-wildcard.yaml",
				"specific-exact.yaml",
				"specific-pattern.yaml",
				"mid-exact.yaml",
				"mid-pattern.yaml",
			},
			matchedOIDs: []string{
				"1.3.6.1.4.1.9.*",
				"1.3.6.1.4.1.9.1.669",
				"1.3.6.1.4.1.9.1.66*",
				"1.3.6.1.4.1.9.1",
				"1.3.6.1.4.1.9.*",
			},
			expected: []string{
				"specific-exact.yaml",   // Longest + exact
				"specific-pattern.yaml", // Longest + pattern
				"mid-exact.yaml",        // Medium + exact
				"generic-wildcard.yaml", // Short + pattern (first in input order)
				"mid-pattern.yaml",      // Short + pattern (second in input order)
			},
		},
		"same OID different profiles - stable sort": {
			profiles:    []string{"profile-b.yaml", "profile-a.yaml", "profile-c.yaml"},
			matchedOIDs: []string{"1.3.6.1.4.1.9.1.669", "1.3.6.1.4.1.9.1.669", "1.3.6.1.4.1.9.1.669"},
			expected:    []string{"profile-b.yaml", "profile-a.yaml", "profile-c.yaml"}, // Maintains input order
		},
		"regex patterns with special characters": {
			profiles:    []string{"exact.yaml", "regex-dots.yaml", "regex-plus.yaml", "regex-question.yaml"},
			matchedOIDs: []string{"1.3.6.1.4.1.9.1.669", "1.3.6.1.4.1.9.1...", "1.3.6.1.4.1.9.1.6+", "1.3.6.1.4.1.9.1.66?"},
			expected: []string{
				"exact.yaml",          // Length 21, exact
				"regex-question.yaml", // Length 20, pattern
				"regex-dots.yaml",     // Length 19, pattern ("..." < ".6+" lexicographically)
				"regex-plus.yaml",     // Length 19, pattern
			},
		},
		"all exact OIDs - sort by OID value": {
			profiles:    []string{"oid-670.yaml", "oid-669.yaml", "oid-671.yaml"},
			matchedOIDs: []string{"1.3.6.1.4.1.9.1.670", "1.3.6.1.4.1.9.1.669", "1.3.6.1.4.1.9.1.671"},
			expected:    []string{"oid-669.yaml", "oid-670.yaml", "oid-671.yaml"},
		},
		"all patterns - sort by pattern value": {
			profiles:    []string{"pattern-b.yaml", "pattern-a.yaml", "pattern-c.yaml"},
			matchedOIDs: []string{"1.3.6.1.4.1.9.1.b*", "1.3.6.1.4.1.9.1.a*", "1.3.6.1.4.1.9.1.c*"},
			expected:    []string{"pattern-a.yaml", "pattern-b.yaml", "pattern-c.yaml"},
		},
		"real-world scenario": {
			profiles: []string{
				"generic-device.yaml",
				"cisco.yaml",
				"cisco-asa.yaml",
				"cisco-asa-5510.yaml",
			},
			matchedOIDs: []string{
				"1.3.6.1.4.1.*",
				"1.3.6.1.4.1.9.*",
				"1.3.6.1.4.1.9.1.*",
				"1.3.6.1.4.1.9.1.669",
			},
			expected: []string{
				"cisco-asa-5510.yaml",
				"cisco-asa.yaml",
				"cisco.yaml",
				"generic-device.yaml",
			},
		},
		"mixed vendor OIDs": {
			profiles: []string{
				"dell.yaml",
				"cisco.yaml",
				"hp.yaml",
				"generic.yaml",
			},
			matchedOIDs: []string{
				"1.3.6.1.4.1.674.10892.5",
				"1.3.6.1.4.1.9.1.669",
				"1.3.6.1.4.1.11.2.3.7.11",
				"1.3.6.1.4.1.*",
			},
			expected: []string{
				"hp.yaml",      // Longest OID (24 chars)
				"dell.yaml",    // Second longest (24 chars, but "11" < "674" lexicographically)
				"cisco.yaml",   // Third longest (21 chars)
				"generic.yaml", // Shortest (pattern, 13 chars)
			},
		},
		"edge case - empty OID": {
			profiles:    []string{"profile1.yaml", "profile2.yaml"},
			matchedOIDs: []string{"", "1.3.6.1.4.1.9.1.669"},
			expected:    []string{"profile2.yaml", "profile1.yaml"},
		},
		"patterns with same prefix": {
			profiles: []string{
				"generic.yaml",
				"specific.yaml",
				"more-specific.yaml",
			},
			matchedOIDs: []string{
				"1.3.6.1.4.1.9.*",
				"1.3.6.1.4.1.9.1.*",
				"1.3.6.1.4.1.9.1.6*",
			},
			expected: []string{
				"more-specific.yaml",
				"specific.yaml",
				"generic.yaml",
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Create profiles and matchedOIDs map
			profiles := make([]*Profile, len(tc.profiles))
			matchedOIDs := make(map[*Profile]string)

			for i, sourceFile := range tc.profiles {
				profiles[i] = &Profile{SourceFile: sourceFile}
				if i < len(tc.matchedOIDs) {
					matchedOIDs[profiles[i]] = tc.matchedOIDs[i]
				}
			}

			sortProfilesBySpecificity(profiles, matchedOIDs)

			actual := make([]string, len(profiles))
			for i, p := range profiles {
				actual[i] = p.SourceFile
			}

			assert.Equal(t, tc.expected, actual, "Profile order mismatch")
		})
	}
}

func TestSortProfilesBySpecificity_Stable(t *testing.T) {
	// Create many profiles with the same OID
	numProfiles := 100
	profiles := make([]*Profile, numProfiles)
	matchedOIDs := make(map[*Profile]string)

	for i := 0; i < numProfiles; i++ {
		profiles[i] = &Profile{
			SourceFile: fmt.Sprintf("profile-%03d.yaml", i),
		}
		matchedOIDs[profiles[i]] = "1.3.6.1.4.1.9.1.669"
	}

	sortProfilesBySpecificity(profiles, matchedOIDs)

	// Verify order is preserved (lexicographic due to same OID)
	for i := 0; i < numProfiles; i++ {
		expected := fmt.Sprintf("profile-%03d.yaml", i)
		assert.Equal(t, expected, profiles[i].SourceFile)
	}
}

func Test_ProfileExtends_CircularReference(t *testing.T) {
	tmp := t.TempDir()

	a := filepath.Join(tmp, "a.yaml")
	b := filepath.Join(tmp, "b.yaml")

	writeYAML(t, a, map[string]any{
		"extends": []string{"b.yaml"},
	})
	writeYAML(t, b, map[string]any{
		"extends": []string{"a.yaml"},
	})

	paths := multipath.New(tmp)
	_, err := loadProfile(a, paths)
	require.Error(t, err)
	require.Contains(t, err.Error(), "circular extends")
}

func Test_ProfileExtends_RecursiveChain(t *testing.T) {
	tmp := t.TempDir()

	base := filepath.Join(tmp, "base.yaml")
	mid := filepath.Join(tmp, "mid.yaml")
	top := filepath.Join(tmp, "top.yaml")

	writeYAML(t, base, ddprofiledefinition.ProfileDefinition{
		Metrics: []ddprofiledefinition.MetricsConfig{
			{
				Symbol: ddprofiledefinition.SymbolConfig{
					OID:  "1.3.6.1.2.1.1.3.0",
					Name: "sysUpTime",
				},
			},
		},
	})
	writeYAML(t, mid, map[string]any{
		"extends": []string{"base.yaml"},
	})
	writeYAML(t, top, map[string]any{
		"extends": []string{"mid.yaml"},
	})

	paths := multipath.New(tmp)
	prof, err := loadProfile(top, paths)
	require.NoError(t, err)
	require.Len(t, prof.Definition.Metrics, 1)
	require.Equal(t, "sysUpTime", prof.Definition.Metrics[0].Symbol.Name)
}

func Test_ProfileExtends_MultipleBases(t *testing.T) {
	tmp := t.TempDir()

	base1 := filepath.Join(tmp, "base1.yaml")
	base2 := filepath.Join(tmp, "base2.yaml")
	main := filepath.Join(tmp, "main.yaml")

	writeYAML(t, base1, ddprofiledefinition.ProfileDefinition{
		Metrics: []ddprofiledefinition.MetricsConfig{
			{Symbol: ddprofiledefinition.SymbolConfig{
				OID: "1.3.6.1.2.1.1.1.0", Name: "sysDescr",
			}},
		},
	})
	writeYAML(t, base2, ddprofiledefinition.ProfileDefinition{
		Metrics: []ddprofiledefinition.MetricsConfig{
			{Symbol: ddprofiledefinition.SymbolConfig{
				OID: "1.3.6.1.2.1.1.5.0", Name: "sysName",
			}},
		},
	})
	writeYAML(t, main, map[string]any{
		"extends": []string{"base1.yaml", "base2.yaml"},
	})

	paths := multipath.New(tmp)
	prof, err := loadProfile(main, paths)
	require.NoError(t, err)
	require.Len(t, prof.Definition.Metrics, 2)
}

func Test_ProfileExtends_NonexistentFile(t *testing.T) {
	tmp := t.TempDir()
	main := filepath.Join(tmp, "main.yaml")

	writeYAML(t, main, map[string]any{
		"extends": []string{"missing.yaml"},
	})

	paths := multipath.New(tmp)
	_, err := loadProfile(main, paths)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing.yaml")
}

func Test_ProfileExtends_SharedBase(t *testing.T) {
	tmp := t.TempDir()

	base := filepath.Join(tmp, "base.yaml")
	a := filepath.Join(tmp, "a.yaml")
	b := filepath.Join(tmp, "b.yaml")

	writeYAML(t, base, ddprofiledefinition.ProfileDefinition{
		Metrics: []ddprofiledefinition.MetricsConfig{
			{Symbol: ddprofiledefinition.SymbolConfig{
				OID: "1.3.6.1.2.1.1.1.0", Name: "sysDescr",
			}},
		},
	})
	writeYAML(t, a, map[string]any{"extends": []string{"base.yaml"}})
	writeYAML(t, b, map[string]any{"extends": []string{"base.yaml"}})

	paths := multipath.New(tmp)

	profA, err := loadProfile(a, paths)
	require.NoError(t, err)
	require.Len(t, profA.Definition.Metrics, 1)

	profB, err := loadProfile(b, paths)
	require.NoError(t, err)
	require.Len(t, profB.Definition.Metrics, 1)
}

func Test_ProfileExtends_OverrideIgnored(t *testing.T) {
	tmp := t.TempDir()

	base := filepath.Join(tmp, "base.yaml")
	main := filepath.Join(tmp, "main.yaml")

	writeYAML(t, base, ddprofiledefinition.ProfileDefinition{
		Metrics: []ddprofiledefinition.MetricsConfig{
			{Symbol: ddprofiledefinition.SymbolConfig{
				OID: "1.3.6.1.2.1.1.3.0", Name: "sysUpTime",
			}},
		},
	})
	writeYAML(t, main, map[string]any{
		"extends": []string{"base.yaml"},
		"metrics": []map[string]any{
			{
				"symbol": map[string]string{
					"OID":  "1.3.6.1.2.1.1.3.0",
					"name": "sysUpTime",
				},
			},
		},
	})

	paths := multipath.New(tmp)
	prof, err := loadProfile(main, paths)
	require.NoError(t, err)

	deduplicateMetricsAcrossProfiles([]*Profile{prof})

	// Should not duplicate
	require.Len(t, prof.Definition.Metrics, 1)
}

func Test_ProfileExtends_UserOverride(t *testing.T) {
	stockDir := filepath.Join(t.TempDir(), "stock")
	userDir := filepath.Join(t.TempDir(), "user")

	require.NoError(t, os.MkdirAll(stockDir, 0755))
	require.NoError(t, os.MkdirAll(userDir, 0755))

	// Stock base profile
	writeYAML(t, filepath.Join(stockDir, "_base.yaml"), ddprofiledefinition.ProfileDefinition{
		Metrics: []ddprofiledefinition.MetricsConfig{
			{Symbol: ddprofiledefinition.SymbolConfig{
				OID: "1.3.6.1.2.1.1.3.0", Name: "sysUpTime",
			}},
		},
	})

	// User override of base profile
	writeYAML(t, filepath.Join(userDir, "_base.yaml"), ddprofiledefinition.ProfileDefinition{
		Metrics: []ddprofiledefinition.MetricsConfig{
			{Symbol: ddprofiledefinition.SymbolConfig{
				OID: "1.3.6.1.2.1.1.3.0", Name: "sysUpTime",
			}},
			{Symbol: ddprofiledefinition.SymbolConfig{
				OID: "1.3.6.1.2.1.1.5.0", Name: "sysName",
			}},
		},
	})

	// Main profile in stock
	writeYAML(t, filepath.Join(stockDir, "device.yaml"), map[string]any{
		"extends": []string{"_base.yaml"},
	})

	paths := multipath.New(userDir, stockDir)
	prof, err := loadProfile(filepath.Join(stockDir, "device.yaml"), paths)
	require.NoError(t, err)

	// Should use user's _base.yaml, so should have 2 metrics
	require.Len(t, prof.Definition.Metrics, 2)
	assert.Equal(t, "sysName", prof.Definition.Metrics[1].Symbol.Name)
}

func TestProfile_ExtensionHierarchy(t *testing.T) {
	tmp := t.TempDir()

	// Create a base profile
	base := filepath.Join(tmp, "_base.yaml")
	writeYAML(t, base, ddprofiledefinition.ProfileDefinition{
		Metrics: []ddprofiledefinition.MetricsConfig{
			{
				Symbol: ddprofiledefinition.SymbolConfig{
					OID:  "1.3.6.1.2.1.1.3.0",
					Name: "sysUpTime",
				},
			},
		},
	})

	// Create an intermediate profile
	intermediate := filepath.Join(tmp, "_intermediate.yaml")
	writeYAML(t, intermediate, map[string]any{
		"extends": []string{"_base.yaml"},
		"metrics": []map[string]any{
			{
				"symbol": map[string]string{
					"OID":  "1.3.6.1.2.1.1.5.0",
					"name": "sysName",
				},
			},
		},
	})

	// Create the main profile
	main := filepath.Join(tmp, "device.yaml")
	writeYAML(t, main, map[string]any{
		"extends": []string{"_intermediate.yaml"},
		"metrics": []map[string]any{
			{
				"symbol": map[string]string{
					"OID":  "1.3.6.1.2.1.1.1.0",
					"name": "sysDescr",
				},
			},
		},
	})

	paths := multipath.New(tmp)
	prof, err := loadProfile(main, paths)
	require.NoError(t, err)

	// Check that we have the extension hierarchy
	require.Len(t, prof.extensionHierarchy, 1)
	assert.Equal(t, "_intermediate.yaml", prof.extensionHierarchy[0].name)

	// Check nested extensions
	require.Len(t, prof.extensionHierarchy[0].extensions, 1)
	assert.Equal(t, "_base.yaml", prof.extensionHierarchy[0].extensions[0].name)

	// Check that all metrics were merged
	require.Len(t, prof.Definition.Metrics, 3)

	// Test helper methods
	allFiles := prof.getAllExtendedFiles()
	fmt.Println(allFiles)
	assert.Len(t, allFiles, 2)

	depth := prof.getExtensionDepth()
	assert.Equal(t, 2, depth)

	assert.False(t, prof.hasCircularDependency())
}

func TestProfile_MultipleExtends(t *testing.T) {
	tmp := t.TempDir()

	// Create base profiles
	base1 := filepath.Join(tmp, "_base1.yaml")
	writeYAML(t, base1, ddprofiledefinition.ProfileDefinition{
		Metrics: []ddprofiledefinition.MetricsConfig{
			{
				Symbol: ddprofiledefinition.SymbolConfig{
					OID:  "1.3.6.1.2.1.1.1.0",
					Name: "sysDescr",
				},
			},
		},
	})

	base2 := filepath.Join(tmp, "_base2.yaml")
	writeYAML(t, base2, ddprofiledefinition.ProfileDefinition{
		Metrics: []ddprofiledefinition.MetricsConfig{
			{
				Symbol: ddprofiledefinition.SymbolConfig{
					OID:  "1.3.6.1.2.1.1.5.0",
					Name: "sysName",
				},
			},
		},
	})

	// Main profile extending both
	main := filepath.Join(tmp, "device.yaml")
	writeYAML(t, main, map[string]any{
		"extends": []string{"_base1.yaml", "_base2.yaml"},
	})

	paths := multipath.New(tmp)
	prof, err := loadProfile(main, paths)
	require.NoError(t, err)

	// Check that we have both extensions
	require.Len(t, prof.extensionHierarchy, 2)
	assert.Equal(t, "_base1.yaml", prof.extensionHierarchy[0].name)
	assert.Equal(t, "_base2.yaml", prof.extensionHierarchy[1].name)

	// Both should have no nested extensions
	assert.Len(t, prof.extensionHierarchy[0].extensions, 0)
	assert.Len(t, prof.extensionHierarchy[1].extensions, 0)

	// Check all files
	allFiles := prof.getAllExtendedFiles()
	assert.Len(t, allFiles, 2)
}

func TestProfile_ComplexHierarchy(t *testing.T) {
	tmp := t.TempDir()

	// Create a complex hierarchy:
	// device.yaml -> [_vendor.yaml, _generic.yaml]
	// _vendor.yaml -> _base.yaml
	// _generic.yaml -> _base.yaml

	base := filepath.Join(tmp, "_base.yaml")
	writeYAML(t, base, ddprofiledefinition.ProfileDefinition{
		Metrics: []ddprofiledefinition.MetricsConfig{
			{
				Symbol: ddprofiledefinition.SymbolConfig{
					OID:  "1.3.6.1.2.1.1.3.0",
					Name: "sysUpTime",
				},
			},
		},
	})

	vendor := filepath.Join(tmp, "_vendor.yaml")
	writeYAML(t, vendor, map[string]any{
		"extends": []string{"_base.yaml"},
		"metrics": []map[string]any{
			{
				"symbol": map[string]string{
					"OID":  "1.3.6.1.4.1.9.9.109.1.1.1.1.7",
					"name": "cpmCPUTotal5minRev",
				},
			},
		},
	})

	generic := filepath.Join(tmp, "_generic.yaml")
	writeYAML(t, generic, map[string]any{
		"extends": []string{"_base.yaml"},
		"metrics": []map[string]any{
			{
				"table": map[string]string{
					"OID":  "1.3.6.1.2.1.2.2",
					"name": "ifTable",
				},
				"symbols": []map[string]string{
					{
						"OID":  "1.3.6.1.2.1.2.2.1.10",
						"name": "ifInOctets",
					},
				},
			},
		},
	})

	device := filepath.Join(tmp, "device.yaml")
	writeYAML(t, device, map[string]any{
		"extends": []string{"_vendor.yaml", "_generic.yaml"},
		"metrics": []map[string]any{
			{
				"symbol": map[string]string{
					"OID":  "1.3.6.1.2.1.1.1.0",
					"name": "sysDescr",
				},
			},
		},
	})

	paths := multipath.New(tmp)
	prof, err := loadProfile(device, paths)
	require.NoError(t, err)

	// Check hierarchy
	require.Len(t, prof.extensionHierarchy, 2)

	// Both vendor and generic should have base as their extension
	require.Len(t, prof.extensionHierarchy[0].extensions, 1)
	require.Len(t, prof.extensionHierarchy[1].extensions, 1)
	assert.Equal(t, "_base.yaml", prof.extensionHierarchy[0].extensions[0].name)
	assert.Equal(t, "_base.yaml", prof.extensionHierarchy[1].extensions[0].name)

	// Check depth
	assert.Equal(t, 2, prof.getExtensionDepth())

	// Check that base.yaml appears only once in the flat list
	allFiles := prof.getAllExtendedFiles()
	baseCount := 0
	for _, f := range allFiles {
		if filepath.Base(f) == "_base.yaml" {
			baseCount++
		}
	}
	assert.Equal(t, 1, baseCount, "base.yaml should appear only once in the flat list")
}

func TestProfile_Clone(t *testing.T) {
	tmp := t.TempDir()

	base := filepath.Join(tmp, "_base.yaml")
	writeYAML(t, base, ddprofiledefinition.ProfileDefinition{
		Metrics: []ddprofiledefinition.MetricsConfig{
			{
				Symbol: ddprofiledefinition.SymbolConfig{
					OID:  "1.3.6.1.2.1.1.3.0",
					Name: "sysUpTime",
				},
			},
		},
	})

	main := filepath.Join(tmp, "device.yaml")
	writeYAML(t, main, map[string]any{
		"extends": []string{"_base.yaml"},
	})

	paths := multipath.New(tmp)
	original, err := loadProfile(main, paths)
	require.NoError(t, err)

	// Clone the profile
	cloned := original.clone()

	// Verify the clone is independent
	assert.Equal(t, original.SourceFile, cloned.SourceFile)
	assert.Len(t, cloned.extensionHierarchy, 1)

	// Modify the original
	original.extensionHierarchy[0].name = "modified"

	// Check that clone wasn't affected
	assert.Equal(t, "_base.yaml", cloned.extensionHierarchy[0].name)
}

func TestProfile_SourceTree(t *testing.T) {
	tests := map[string]struct {
		setup    func(t *testing.T, tmp string) string
		expected string
	}{
		"no extensions": {
			setup: func(t *testing.T, tmp string) string {
				device := filepath.Join(tmp, "device.yaml")
				writeYAML(t, device, ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{{
						Symbol: ddprofiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.1.1.0",
							Name: "sysDescr",
						},
					}},
				})
				return device
			},
			expected: "device",
		},
		"single extension": {
			setup: func(t *testing.T, tmp string) string {
				base := filepath.Join(tmp, "_base.yaml")
				writeYAML(t, base, ddprofiledefinition.ProfileDefinition{})

				device := filepath.Join(tmp, "device.yaml")
				writeYAML(t, device, map[string]any{
					"extends": []string{"_base.yaml"},
				})
				return device
			},
			expected: "device: [_base]",
		},
		"two direct extensions": {
			setup: func(t *testing.T, tmp string) string {
				base1 := filepath.Join(tmp, "_base1.yaml")
				writeYAML(t, base1, ddprofiledefinition.ProfileDefinition{})

				base2 := filepath.Join(tmp, "_base2.yaml")
				writeYAML(t, base2, ddprofiledefinition.ProfileDefinition{})

				device := filepath.Join(tmp, "device.yaml")
				writeYAML(t, device, map[string]any{
					"extends": []string{"_base1.yaml", "_base2.yaml"},
				})
				return device
			},
			expected: "device: [_base1, _base2]",
		},
		"nested chain": {
			setup: func(t *testing.T, tmp string) string {
				base := filepath.Join(tmp, "_base.yaml")
				writeYAML(t, base, ddprofiledefinition.ProfileDefinition{})

				intermediate := filepath.Join(tmp, "_intermediate.yaml")
				writeYAML(t, intermediate, map[string]any{
					"extends": []string{"_base.yaml"},
				})

				device := filepath.Join(tmp, "device.yaml")
				writeYAML(t, device, map[string]any{
					"extends": []string{"_intermediate.yaml"},
				})
				return device
			},
			expected: "device: [_intermediate: [_base]]",
		},
		"complex diamond pattern": {
			setup: func(t *testing.T, tmp string) string {
				base := filepath.Join(tmp, "_base.yaml")
				writeYAML(t, base, ddprofiledefinition.ProfileDefinition{})

				vendor := filepath.Join(tmp, "_vendor.yaml")
				writeYAML(t, vendor, map[string]any{
					"extends": []string{"_base.yaml"},
				})

				generic := filepath.Join(tmp, "_generic.yaml")
				writeYAML(t, generic, map[string]any{
					"extends": []string{"_base.yaml"},
				})

				device := filepath.Join(tmp, "cisco-nexus.yaml")
				writeYAML(t, device, map[string]any{
					"extends": []string{"_vendor.yaml", "_generic.yaml"},
				})
				return device
			},
			expected: "cisco-nexus: [_vendor: [_base], _generic: [_base]]",
		},
		"mixed depths": {
			setup: func(t *testing.T, tmp string) string {
				base := filepath.Join(tmp, "_base.yaml")
				writeYAML(t, base, ddprofiledefinition.ProfileDefinition{})

				intermediate := filepath.Join(tmp, "_intermediate.yaml")
				writeYAML(t, intermediate, map[string]any{
					"extends": []string{"_base.yaml"},
				})

				standalone := filepath.Join(tmp, "_standalone.yaml")
				writeYAML(t, standalone, ddprofiledefinition.ProfileDefinition{})

				device := filepath.Join(tmp, "device.yaml")
				writeYAML(t, device, map[string]any{
					"extends": []string{"_intermediate.yaml", "_standalone.yaml"},
				})
				return device
			},
			expected: "device: [_intermediate: [_base], _standalone]",
		},
		"three level nesting": {
			setup: func(t *testing.T, tmp string) string {
				base := filepath.Join(tmp, "_base.yaml")
				writeYAML(t, base, ddprofiledefinition.ProfileDefinition{})

				level1 := filepath.Join(tmp, "_level1.yaml")
				writeYAML(t, level1, map[string]any{
					"extends": []string{"_base.yaml"},
				})

				level2 := filepath.Join(tmp, "_level2.yaml")
				writeYAML(t, level2, map[string]any{
					"extends": []string{"_level1.yaml"},
				})

				device := filepath.Join(tmp, "device.yaml")
				writeYAML(t, device, map[string]any{
					"extends": []string{"_level2.yaml"},
				})
				return device
			},
			expected: "device: [_level2: [_level1: [_base]]]",
		},
		"yml extension stripped": {
			setup: func(t *testing.T, tmp string) string {
				base := filepath.Join(tmp, "_base.yml")
				writeYAML(t, base, ddprofiledefinition.ProfileDefinition{})

				device := filepath.Join(tmp, "device.yml")
				writeYAML(t, device, map[string]any{
					"extends": []string{"_base.yml"},
				})
				return device
			},
			expected: "device: [_base]",
		},
		"complex real-world example": {
			setup: func(t *testing.T, tmp string) string {
				base := filepath.Join(tmp, "_base.yaml")
				writeYAML(t, base, ddprofiledefinition.ProfileDefinition{})

				genericDevice := filepath.Join(tmp, "_generic-device.yaml")
				writeYAML(t, genericDevice, map[string]any{
					"extends": []string{"_base.yaml"},
				})

				genericIf := filepath.Join(tmp, "_generic-if.yaml")
				writeYAML(t, genericIf, map[string]any{
					"extends": []string{"_base.yaml"},
				})

				cisco := filepath.Join(tmp, "_cisco.yaml")
				writeYAML(t, cisco, map[string]any{
					"extends": []string{"_generic-device.yaml"},
				})

				ciscoNexus := filepath.Join(tmp, "cisco-nexus.yaml")
				writeYAML(t, ciscoNexus, map[string]any{
					"extends": []string{"_cisco.yaml", "_generic-if.yaml"},
				})
				return ciscoNexus
			},
			expected: "cisco-nexus: [_cisco: [_generic-device: [_base]], _generic-if: [_base]]",
		},
		"empty extends list": {
			setup: func(t *testing.T, tmp string) string {
				device := filepath.Join(tmp, "device.yaml")
				writeYAML(t, device, map[string]any{
					"extends": []string{},
					"metrics": []map[string]any{{
						"symbol": map[string]string{
							"OID":  "1.3.6.1.2.1.1.1.0",
							"name": "sysDescr",
						},
					}},
				})
				return device
			},
			expected: "device",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tmp := t.TempDir()
			mainFile := tc.setup(t, tmp)

			paths := multipath.New(tmp)
			prof, err := loadProfile(mainFile, paths)
			require.NoError(t, err)

			assert.Equal(t, tc.expected, prof.SourceTree())
		})
	}
}

// getAllExtendedFiles returns a flat list of all files in the extension hierarchy
func (p *Profile) getAllExtendedFiles() []string {
	var files []string
	seen := make(map[string]bool)

	var collect func([]*extensionInfo)
	collect = func(extensions []*extensionInfo) {
		for _, ext := range extensions {
			if !seen[ext.sourceFile] {
				seen[ext.sourceFile] = true
				files = append(files, ext.sourceFile)
			}
			collect(ext.extensions)
		}
	}

	collect(p.extensionHierarchy)
	return files
}

// getExtensionDepth returns the maximum depth of the extension hierarchy
func (p *Profile) getExtensionDepth() int {
	var maxDepth func([]*extensionInfo, int) int
	maxDepth = func(extensions []*extensionInfo, depth int) int {
		if len(extensions) == 0 {
			return depth
		}

		maximum := depth + 1
		for _, ext := range extensions {
			d := maxDepth(ext.extensions, depth+1)
			if d > maximum {
				maximum = d
			}
		}
		return maximum
	}

	return maxDepth(p.extensionHierarchy, 0)
}

// hasCircularDependency checks if there's a circular dependency in the extension hierarchy
func (p *Profile) hasCircularDependency() bool {
	visited := make(map[string]bool)

	var hasCircle func([]*extensionInfo) bool
	hasCircle = func(extensions []*extensionInfo) bool {
		for _, ext := range extensions {
			if visited[ext.sourceFile] {
				return true
			}
			visited[ext.sourceFile] = true
			if hasCircle(ext.extensions) {
				return true
			}
			delete(visited, ext.sourceFile)
		}
		return false
	}

	return hasCircle(p.extensionHierarchy)
}

func writeYAML(t *testing.T, path string, data any) {
	t.Helper()

	content, err := yaml.Marshal(data)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, content, 0600))
}
