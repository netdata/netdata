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
