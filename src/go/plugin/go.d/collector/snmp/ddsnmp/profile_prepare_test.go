// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestPrepareIndexFromDependencies(t *testing.T) {
	row := ddprofiledefinition.MetricsConfig{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  "1.1.1.7",
			Name: "source",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{{OID: "1.1.1", Name: "value"}},
		MetricTags: []ddprofiledefinition.MetricTagConfig{{
			Tag: "label", Table: "labels", Symbol: ddprofiledefinition.SymbolConfigCompat{
				OID:  "1.4.1",
				Name: "label",
			},
			IndexFrom: []ddprofiledefinition.MetricTagIndexSource{
				{Table: "link", Symbol: ddprofiledefinition.SymbolConfig{
					OID:  "1.2.1",
					Name: "link",
				}},
				{Table: "alias", Symbol: ddprofiledefinition.SymbolConfig{
					OID:  "1.3.1",
					Name: "alias",
				}},
			},
		}},
	}
	for _, topology := range []bool{false, true} {
		profile := &Profile{
			Definition: &ddprofiledefinition.ProfileDefinition{},
		}
		if topology {
			profile.Definition.Topology = []ddprofiledefinition.TopologyConfig{
				{Kind: ddprofiledefinition.KindIfName, MetricsConfig: row.Clone()},
			}
		} else {
			profile.Definition.Metrics = []ddprofiledefinition.MetricsConfig{row.Clone()}
		}
		handleCrossTableTagsWithoutMetrics(profile)
		got := make(map[string]string)
		for _, metric := range profile.Definition.Metrics {
			got[metric.Table.Name] = metric.Table.OID
		}
		for _, metric := range profile.Definition.Topology {
			assert.Equal(t, ddprofiledefinition.KindIfName, metric.Kind)
			got[metric.Table.Name] = metric.Table.OID
		}
		assert.Equal(
			t,
			map[string]string{"source": "1.1.1.7", "link": "1.2.1", "alias": "1.3.1", "labels": "1.4.1"},
			got,
			"value-derived keys must not inherit the source table's fixed index prefix",
		)
		before := profile.Definition.Clone()
		handleCrossTableTagsWithoutMetrics(profile)
		assert.Equal(t, before, profile.Definition, "preparation is idempotent")
	}
}

func TestIndexFromSurvivesInheritanceAndConsumerProjection(t *testing.T) {
	base := &Profile{
		Definition: &ddprofiledefinition.ProfileDefinition{
			Metrics: []ddprofiledefinition.MetricsConfig{{
				Table: ddprofiledefinition.SymbolConfig{
					OID:  "1.1",
					Name: "source",
				},
				Symbols: []ddprofiledefinition.SymbolConfig{{OID: "1.1.1", Name: "value"}},
				MetricTags: []ddprofiledefinition.MetricTagConfig{{
					Tag: "label", Table: "labels", Symbol: ddprofiledefinition.SymbolConfigCompat{
						OID:  "1.3.1",
						Name: "label",
					},
					IndexFrom: []ddprofiledefinition.MetricTagIndexSource{
						{Table: "link", Symbol: ddprofiledefinition.SymbolConfig{
							OID:  "1.2.1",
							Name: "link",
							Mapping: ddprofiledefinition.MappingConfig{
								Items: map[string]string{"1": "2"},
							},
						}},
					},
				}},
			}},
		},
	}
	child := &Profile{
		Definition: &ddprofiledefinition.ProfileDefinition{},
	}
	require.NoError(t, child.merge(base))
	resolved := &ResolvedProfileSet{
		profiles: []*Profile{child},
	}
	first := resolved.Project(ConsumerMetrics).Profiles()
	second := resolved.Project(ConsumerMetrics).Profiles()
	require.Len(t, first, 1)
	require.Len(t, second, 1)
	first[0].Definition.Metrics[0].MetricTags[0].IndexFrom[0].Symbol.Mapping.Items["1"] = "changed"
	assert.Equal(t, "2", second[0].Definition.Metrics[0].MetricTags[0].IndexFrom[0].Symbol.Mapping.Items["1"])
	handleCrossTableTagsWithoutMetrics(second[0])
	require.Len(t, second[0].Definition.Metrics, 3)
	assert.Equal(t, "labels", second[0].Definition.Metrics[1].Table.Name)
	assert.Equal(t, "link", second[0].Definition.Metrics[2].Table.Name)
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
					MetricTags: []ddprofiledefinition.MetricTagConfig{
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
						MetricTags: []ddprofiledefinition.MetricTagConfig{
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
					Table: ddprofiledefinition.SymbolConfig{
						OID:  anchorColumnOID + ".1",
						Name: "ipAddressIfIndexIPv4",
					},
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
	profile := &Profile{
		Definition: &ddprofiledefinition.ProfileDefinition{
			Metrics: []ddprofiledefinition.MetricsConfig{{
				Table: ddprofiledefinition.SymbolConfig{
					OID:  anchorColumnOID + ".1",
					Name: "scopedMetricTable",
				},
				Symbols: []ddprofiledefinition.SymbolConfig{{OID: anchorColumnOID, Name: "value"}},
				MetricTags: []ddprofiledefinition.MetricTagConfig{{
					Tag:   "dependency_value",
					Table: "dependencyTable",
					Symbol: ddprofiledefinition.SymbolConfigCompat{
						OID:  dependencyColumnOID,
						Name: "dependencyValue",
					},
				}},
			}},
		},
	}

	handleCrossTableTagsWithoutMetrics(profile)

	require.Len(t, profile.Definition.Metrics, 2)
	assert.Equal(t, dependencyColumnOID, profile.Definition.Metrics[1].Table.OID)
}

func TestHandleCrossTableTagsWithoutMetrics_DoesNotScopeTransformedTopologyJoin(t *testing.T) {
	const (
		anchorColumnOID     = "1.3.6.1.2.1.4.34.1.3"
		dependencyColumnOID = "1.3.6.1.2.1.4.34.1.4"
	)
	profile := &Profile{
		Definition: &ddprofiledefinition.ProfileDefinition{
			Topology: []ddprofiledefinition.TopologyConfig{{
				Kind: ddprofiledefinition.KindIpIfIndex,
				MetricsConfig: ddprofiledefinition.MetricsConfig{
					Table: ddprofiledefinition.SymbolConfig{
						OID:  anchorColumnOID + ".1",
						Name: "scopedTopologyTable",
					},
					Symbols: []ddprofiledefinition.SymbolConfig{{OID: anchorColumnOID, Name: "ip_if_index"}},
					MetricTags: []ddprofiledefinition.MetricTagConfig{{
						Tag:   "dependency_value",
						Table: "dependencyTable",
						Symbol: ddprofiledefinition.SymbolConfigCompat{
							OID:  dependencyColumnOID,
							Name: "dependencyValue",
						},
						IndexTransform: []ddprofiledefinition.MetricIndexTransform{{Start: 1}},
					}},
				},
			}},
		},
	}

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
				Table: ddprofiledefinition.SymbolConfig{
					OID:  anchorColumnOID + "." + scope,
					Name: "anchor" + scope,
				},
				Symbols: []ddprofiledefinition.SymbolConfig{{OID: anchorColumnOID, Name: "ip_if_index"}},
				MetricTags: []ddprofiledefinition.MetricTagConfig{{
					Tag:   "dependency_value",
					Table: "dependencyTable",
					Symbol: ddprofiledefinition.SymbolConfigCompat{
						OID:  dependencyColumnOID,
						Name: "dependencyValue",
					},
				}},
			},
		}
	}

	for _, scopes := range [][]string{{"1", "2"}, {"2", "1"}} {
		t.Run(scopes[0]+"_then_"+scopes[1], func(t *testing.T) {
			profile := &Profile{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Topology: []ddprofiledefinition.TopologyConfig{row(scopes[0]), row(scopes[1])},
				},
			}

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
		profile := &Profile{
			Definition: &ddprofiledefinition.ProfileDefinition{
				Topology: []ddprofiledefinition.TopologyConfig{{
					Kind: ddprofiledefinition.KindIpIfIndex,
					MetricsConfig: ddprofiledefinition.MetricsConfig{
						Table: ddprofiledefinition.SymbolConfig{
							OID:  anchorOID,
							Name: "anchorTable",
						},
						Symbols: []ddprofiledefinition.SymbolConfig{{OID: anchorOID + ".1", Name: "value"}},
						MetricTags: []ddprofiledefinition.MetricTagConfig{
							{
								Tag:   "zulu",
								Table: "zuluTable",
								Symbol: ddprofiledefinition.SymbolConfigCompat{
									OID: "1.3.6.1.4.1.99999.4.1",
								},
							},
							{
								Tag:   "alpha",
								Table: "alphaTable",
								Symbol: ddprofiledefinition.SymbolConfigCompat{
									OID: "1.3.6.1.4.1.99999.2.1",
								},
							},
							{
								Tag:   "middle",
								Table: "middleTable",
								Symbol: ddprofiledefinition.SymbolConfigCompat{
									OID: "1.3.6.1.4.1.99999.3.1",
								},
							},
						},
					},
				}},
			},
		}

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
						MetricTags: []ddprofiledefinition.MetricTagConfig{
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

func TestLoadProfileByName_LegacyTableMetricTypes(t *testing.T) {
	tests := map[string]struct {
		profile string
		symbol  string
		want    ddprofiledefinition.ProfileMetricType
	}{
		"inherited gauge": {
			profile: "cisco-catalyst-wlc",
			symbol:  "bsnApIfNoOfUsers",
			want:    ddprofiledefinition.ProfileMetricTypeGauge,
		},
		"monotonic count": {
			profile: "_arista-queue",
			symbol:  "aristaIngressQueuePktsDropped",
			want:    ddprofiledefinition.ProfileMetricTypeMonotonicCount,
		},
		"legacy percent string": {profile: "f5-big-ip", symbol: "sysMultiHostCpuUser", want: "percent"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			profile, err := LoadProfileByName(tc.profile)
			require.NoError(t, err)
			var found bool
			for _, row := range profile.Definition.Metrics {
				for _, symbol := range row.Symbols {
					if symbol.Name == tc.symbol {
						found = true
						assert.Equal(t, tc.want, symbol.MetricType)
					}
				}
			}
			require.True(t, found, "missing stock symbol %s", tc.symbol)
		})
	}
}

func TestPreparePreservesDecodedIndexTag(t *testing.T) {
	length := uint(48)
	tag := ddprofiledefinition.MetricTagConfig{
		Tag:    "row_name",
		Symbol: ddprofiledefinition.SymbolConfigCompat{Format: "string"},
		IndexDecode: &ddprofiledefinition.MetricIndexDecode{
			Components: []ddprofiledefinition.MetricIndexComponent{
				{Type: "integer"},
				{Type: "octet_string", Length: &length},
			},
			Component: 2,
		},
	}
	profile := &Profile{Definition: &ddprofiledefinition.ProfileDefinition{
		Metrics: []ddprofiledefinition.MetricsConfig{{
			Table:      ddprofiledefinition.SymbolConfig{OID: "1.2", Name: "rows"},
			Symbols:    []ddprofiledefinition.SymbolConfig{{OID: "1.2.1", Name: "value"}},
			MetricTags: []ddprofiledefinition.MetricTagConfig{tag},
		}},
	}}
	require.NoError(t, ddprofiledefinition.ValidateEnrichProfile(profile.Definition))
	handleCrossTableTagsWithoutMetrics(profile)
	require.Len(t, profile.Definition.Metrics, 1)
	assert.Equal(t, tag, profile.Definition.Metrics[0].MetricTags[0])
	assert.True(t, profile.Definition.Metrics[0].MetricTags[0].IsIndexTag())
}
