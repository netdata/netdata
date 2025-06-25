// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
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
		sysObjOId   string
		wanProfiles int
	}{
		"mikrotik": {
			sysObjOId:   "1.3.6.1.4.1.14988.1",
			wanProfiles: 2,
		},
		"no match": {
			sysObjOId:   "0.1.2.3",
			wanProfiles: 0,
		},
	}

	for name, test := range test {
		t.Run(name, func(t *testing.T) {
			profiles := FindProfiles(test.sysObjOId)

			require.Len(t, profiles, test.wanProfiles)
		})
	}
}

func Test_Profile_merge(t *testing.T) {
	profiles := FindProfiles("1.3.6.1.4.1.9.1.1216") // cisco-nexus

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
		"duplicate scalar metrics across profiles": {
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
						},
					},
				},
				{
					SourceFile: "profile2.yaml",
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
					SourceFile: "profile2.yaml",
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
		"multiple duplicates across three profiles": {
			profiles: []*Profile{
				{
					SourceFile: "generic-device.yaml",
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
				{
					SourceFile: "vendor-specific.yaml",
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
					SourceFile: "extended.yaml",
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
			expected: []*Profile{
				{
					SourceFile: "vendor-specific.yaml",
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
					SourceFile: "extended.yaml",
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
					SourceFile: "generic-device.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{}, // All removed as duplicates
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
						Metrics: []ddprofiledefinition.MetricsConfig{},
					},
				},
			},
		},
		"generic vs non-generic priority": {
			profiles: []*Profile{
				{
					SourceFile: "generic-device.yaml",
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
									{
										OID:  "1.3.6.1.2.1.2.2.1.16",
										Name: "ifOutOctets",
									},
								},
							},
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.1.0",
									Name: "sysDescr",
								},
							},
						},
					},
				},
				{
					SourceFile: "mikrotik-router.yaml",
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
									{
										OID:  "1.3.6.1.2.1.2.2.1.16",
										Name: "ifOutOctets",
									},
								},
							},
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.14988.1.1.1.3.0",
									Name: "mtxrHlCpuTemperature",
								},
							},
						},
					},
				},
				{
					SourceFile: "generic-if.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.1.0",
									Name: "sysDescr",
								},
							},
							{
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.31.1.1",
									Name: "ifXTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.2.1.31.1.1.1.6",
										Name: "ifHCInOctets",
									},
								},
							},
						},
					},
				},
			},
			expected: []*Profile{
				{
					SourceFile: "mikrotik-router.yaml",
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
									{
										OID:  "1.3.6.1.2.1.2.2.1.16",
										Name: "ifOutOctets",
									},
								},
							},
							{
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.14988.1.1.1.3.0",
									Name: "mtxrHlCpuTemperature",
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
								Symbol: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.1.0",
									Name: "sysDescr",
								},
							},
						},
					},
				},
				{
					SourceFile: "generic-if.yaml",
					Definition: &ddprofiledefinition.ProfileDefinition{
						Metrics: []ddprofiledefinition.MetricsConfig{
							{
								Table: ddprofiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.31.1.1",
									Name: "ifXTable",
								},
								Symbols: []ddprofiledefinition.SymbolConfig{
									{
										OID:  "1.3.6.1.2.1.31.1.1.1.6",
										Name: "ifHCInOctets",
									},
								},
							},
						},
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

func writeYAML(t *testing.T, path string, data any) {
	t.Helper()

	content, err := yaml.Marshal(data)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, content, 0600))
}
