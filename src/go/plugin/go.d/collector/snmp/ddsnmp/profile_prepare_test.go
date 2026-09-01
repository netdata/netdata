// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestLongestCommonPrefix(t *testing.T) {
	tests := map[string]struct {
		oids     []string
		expected string
	}{
		"if_x_table": {
			oids: []string{
				"1.3.6.1.2.1.31.1.1.1.1",
				"1.3.6.1.2.1.31.1.1.1.18",
			},
			expected: "1.3.6.1.2.1.31.1.1.1",
		},
		"juniper_table": {
			oids: []string{
				"1.3.6.1.4.1.2636.5.1.1.2.1.1.1.11",
				"1.3.6.1.4.1.2636.5.1.1.2.1.1.1.14",
			},
			expected: "1.3.6.1.4.1.2636.5.1.1.2.1.1.1",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, longestCommonPrefix(tc.oids))
		})
	}
}

func TestHandleCrossTableTagsWithoutMetrics(t *testing.T) {
	profile := &Profile{
		Definition: &ddprofiledefinition.ProfileDefinition{
			Metrics: []ddprofiledefinition.MetricsConfig{
				{
					Table: ddprofiledefinition.SymbolConfig{
						OID:  "1.3.6.1.2.1.2.2",
						Name: "ifTable",
					},
					Symbols: []ddprofiledefinition.SymbolConfig{
						{OID: "1.3.6.1.2.1.2.2.1.10", Name: "ifInOctets"},
					},
					MetricTags: ddprofiledefinition.MetricTagConfigList{
						{
							Tag:   "if_name",
							Table: "ifXTable",
							Symbol: ddprofiledefinition.SymbolConfigCompat{
								OID:  "1.3.6.1.2.1.31.1.1.1.1",
								Name: "ifName",
							},
						},
					},
				},
			},
			Topology: []ddprofiledefinition.TopologyConfig{
				{
					Kind: ddprofiledefinition.KindFdbEntry,
					MetricsConfig: ddprofiledefinition.MetricsConfig{
						Table: ddprofiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.17.4.3",
							Name: "dot1dTpFdbTable",
						},
						Symbols: []ddprofiledefinition.SymbolConfig{
							{OID: "1.3.6.1.2.1.17.4.3.1.2", Name: "dot1dTpFdbPort"},
						},
						MetricTags: ddprofiledefinition.MetricTagConfigList{
							{
								Tag:   "bridge_port_if_index",
								Table: "dot1dBasePortTable",
								Symbol: ddprofiledefinition.SymbolConfigCompat{
									OID:  "1.3.6.1.2.1.17.1.4.1.2",
									Name: "dot1dBasePortIfIndex",
								},
							},
						},
					},
				},
			},
		},
	}

	handleCrossTableTagsWithoutMetrics(profile)

	require.Len(t, profile.Definition.Metrics, 2)
	assert.Equal(t, "ifXTable", profile.Definition.Metrics[1].Table.Name)
	assert.Equal(t, "1.3.6.1.2.1.31.1.1.1.1", profile.Definition.Metrics[1].Table.OID)
	require.Len(t, profile.Definition.Topology, 2)
	assert.Equal(t, ddprofiledefinition.KindFdbEntry, profile.Definition.Topology[1].Kind)
	assert.Equal(t, "dot1dBasePortTable", profile.Definition.Topology[1].Table.Name)
	assert.Equal(t, "1.3.6.1.2.1.17.1.4.1.2", profile.Definition.Topology[1].Table.OID)
}

func TestHandleCrossTableTagsWithoutMetrics_PropagatesNarrowAnchorIndexScope(t *testing.T) {
	const (
		anchorColumnOID     = "1.3.6.1.2.1.4.34.1.3"
		dependencyColumnOID = "1.3.6.1.2.1.4.34.1.4"
	)
	profile := &Profile{
		Definition: &ddprofiledefinition.ProfileDefinition{
			Topology: []ddprofiledefinition.TopologyConfig{{
				Kind: ddprofiledefinition.KindIpIfIndex,
				MetricsConfig: ddprofiledefinition.MetricsConfig{
					Table:   ddprofiledefinition.SymbolConfig{OID: anchorColumnOID + ".1", Name: "ipAddressIfIndexIPv4"},
					Symbols: []ddprofiledefinition.SymbolConfig{{OID: anchorColumnOID, Name: "ip_if_index"}},
					MetricTags: []ddprofiledefinition.MetricTagConfig{{
						Tag:   "ip_address_type",
						Table: "ipAddressTypeIPv4",
						Symbol: ddprofiledefinition.SymbolConfigCompat{
							OID:  dependencyColumnOID,
							Name: "ipAddressType",
						},
					}},
				},
			}},
		},
	}

	handleCrossTableTagsWithoutMetrics(profile)

	require.Len(t, profile.Definition.Topology, 2)
	assert.Equal(t, "ipAddressTypeIPv4", profile.Definition.Topology[1].Table.Name)
	assert.Equal(t, dependencyColumnOID+".1", profile.Definition.Topology[1].Table.OID)
}

func TestHandleCrossTableTagsWithoutMetrics_DoesNotScopeOrdinaryMetrics(t *testing.T) {
	const (
		anchorColumnOID     = "1.3.6.1.2.1.4.34.1.3"
		dependencyColumnOID = "1.3.6.1.2.1.4.34.1.4"
	)
	profile := &Profile{Definition: &ddprofiledefinition.ProfileDefinition{
		Metrics: []ddprofiledefinition.MetricsConfig{{
			Table:   ddprofiledefinition.SymbolConfig{OID: anchorColumnOID + ".1", Name: "scopedMetricTable"},
			Symbols: []ddprofiledefinition.SymbolConfig{{OID: anchorColumnOID, Name: "value"}},
			MetricTags: []ddprofiledefinition.MetricTagConfig{{
				Tag:    "dependency_value",
				Table:  "dependencyTable",
				Symbol: ddprofiledefinition.SymbolConfigCompat{OID: dependencyColumnOID, Name: "dependencyValue"},
			}},
		}},
	}}

	handleCrossTableTagsWithoutMetrics(profile)

	require.Len(t, profile.Definition.Metrics, 2)
	assert.Equal(t, dependencyColumnOID, profile.Definition.Metrics[1].Table.OID)
}

func TestHandleCrossTableTagsWithoutMetrics_DoesNotScopeTransformedTopologyJoin(t *testing.T) {
	const (
		anchorColumnOID     = "1.3.6.1.2.1.4.34.1.3"
		dependencyColumnOID = "1.3.6.1.2.1.4.34.1.4"
	)
	profile := &Profile{Definition: &ddprofiledefinition.ProfileDefinition{
		Topology: []ddprofiledefinition.TopologyConfig{{
			Kind: ddprofiledefinition.KindIpIfIndex,
			MetricsConfig: ddprofiledefinition.MetricsConfig{
				Table:   ddprofiledefinition.SymbolConfig{OID: anchorColumnOID + ".1", Name: "scopedTopologyTable"},
				Symbols: []ddprofiledefinition.SymbolConfig{{OID: anchorColumnOID, Name: "ip_if_index"}},
				MetricTags: []ddprofiledefinition.MetricTagConfig{{
					Tag:            "dependency_value",
					Table:          "dependencyTable",
					Symbol:         ddprofiledefinition.SymbolConfigCompat{OID: dependencyColumnOID, Name: "dependencyValue"},
					IndexTransform: []ddprofiledefinition.MetricIndexTransform{{Start: 1}},
				}},
			},
		}},
	}}

	handleCrossTableTagsWithoutMetrics(profile)

	require.Len(t, profile.Definition.Topology, 2)
	assert.Equal(t, dependencyColumnOID, profile.Definition.Topology[1].Table.OID)
}

func TestHandleCrossTableTagsWithoutMetrics_ConflictingTopologyScopesUseFullDependencyColumn(t *testing.T) {
	const (
		anchorColumnOID     = "1.3.6.1.2.1.4.34.1.3"
		dependencyColumnOID = "1.3.6.1.2.1.4.34.1.4"
	)
	row := func(scope string) ddprofiledefinition.TopologyConfig {
		return ddprofiledefinition.TopologyConfig{
			Kind: ddprofiledefinition.KindIpIfIndex,
			MetricsConfig: ddprofiledefinition.MetricsConfig{
				Table:   ddprofiledefinition.SymbolConfig{OID: anchorColumnOID + "." + scope, Name: "anchor" + scope},
				Symbols: []ddprofiledefinition.SymbolConfig{{OID: anchorColumnOID, Name: "ip_if_index"}},
				MetricTags: []ddprofiledefinition.MetricTagConfig{{
					Tag:    "dependency_value",
					Table:  "dependencyTable",
					Symbol: ddprofiledefinition.SymbolConfigCompat{OID: dependencyColumnOID, Name: "dependencyValue"},
				}},
			},
		}
	}

	for _, scopes := range [][]string{{"1", "2"}, {"2", "1"}} {
		t.Run(scopes[0]+"_then_"+scopes[1], func(t *testing.T) {
			profile := &Profile{Definition: &ddprofiledefinition.ProfileDefinition{
				Topology: []ddprofiledefinition.TopologyConfig{row(scopes[0]), row(scopes[1])},
			}}

			handleCrossTableTagsWithoutMetrics(profile)

			require.Len(t, profile.Definition.Topology, 3)
			assert.Equal(t, "dependencyTable", profile.Definition.Topology[2].Table.Name)
			assert.Equal(t, dependencyColumnOID, profile.Definition.Topology[2].Table.OID)
		})
	}
}

func TestHandleCrossTableTagsWithoutMetrics_AppendsDependenciesDeterministically(t *testing.T) {
	const anchorOID = "1.3.6.1.4.1.99999.1"
	wantNames := []string{"alphaTable", "middleTable", "zuluTable"}
	for range 32 {
		profile := &Profile{Definition: &ddprofiledefinition.ProfileDefinition{
			Topology: []ddprofiledefinition.TopologyConfig{{
				Kind: ddprofiledefinition.KindIpIfIndex,
				MetricsConfig: ddprofiledefinition.MetricsConfig{
					Table:   ddprofiledefinition.SymbolConfig{OID: anchorOID, Name: "anchorTable"},
					Symbols: []ddprofiledefinition.SymbolConfig{{OID: anchorOID + ".1", Name: "value"}},
					MetricTags: []ddprofiledefinition.MetricTagConfig{
						{Tag: "zulu", Table: "zuluTable", Symbol: ddprofiledefinition.SymbolConfigCompat{OID: "1.3.6.1.4.1.99999.4.1"}},
						{Tag: "alpha", Table: "alphaTable", Symbol: ddprofiledefinition.SymbolConfigCompat{OID: "1.3.6.1.4.1.99999.2.1"}},
						{Tag: "middle", Table: "middleTable", Symbol: ddprofiledefinition.SymbolConfigCompat{OID: "1.3.6.1.4.1.99999.3.1"}},
					},
				},
			}},
		}}

		handleCrossTableTagsWithoutMetrics(profile)

		require.Len(t, profile.Definition.Topology, 4)
		gotNames := []string{
			profile.Definition.Topology[1].Table.Name,
			profile.Definition.Topology[2].Table.Name,
			profile.Definition.Topology[3].Table.Name,
		}
		assert.Equal(t, wantNames, gotNames)
	}
}

func TestPrepareLoadedProfile_EnrichesTopologyMappingRefs(t *testing.T) {
	profile := &Profile{
		Definition: &ddprofiledefinition.ProfileDefinition{
			Topology: []ddprofiledefinition.TopologyConfig{
				{
					Kind: ddprofiledefinition.KindIfName,
					MetricsConfig: ddprofiledefinition.MetricsConfig{
						Table: ddprofiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.2.2",
							Name: "ifTable",
						},
						Symbols: []ddprofiledefinition.SymbolConfig{
							{OID: "1.3.6.1.2.1.2.2.1.2", Name: "ifDescr"},
						},
						MetricTags: ddprofiledefinition.MetricTagConfigList{
							{
								Tag:        "if_type",
								Index:      1,
								MappingRef: "ifType",
							},
						},
					},
				},
			},
		},
	}

	require.NoError(t, prepareLoadedProfile(profile))

	require.Len(t, profile.Definition.Topology, 1)
	tag := profile.Definition.Topology[0].MetricTags[0]
	assert.True(t, tag.Mapping.HasItems())
	assert.Equal(t, "ethernetCsmacd", tag.Mapping.Items["6"])
}
