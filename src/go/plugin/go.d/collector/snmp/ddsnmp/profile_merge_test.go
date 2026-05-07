// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestProfile_MultipleExtends_TableSymbolLaterOverrideEarlierByNameWithinTable(t *testing.T) {
	tmp := t.TempDir()

	writeTableBase(t, filepath.Join(tmp, "_base1.yaml"), "1.3.6.1.2.1.2.2", "ifTable", "1.3.6.1.2.1.2.2.1.10", "ifInOctets", "base1")
	writeTableBase(t, filepath.Join(tmp, "_base2.yaml"), "1.3.6.1.2.1.2.2", "ifTable", "1.3.6.1.2.1.2.2.1.16", "ifInOctets", "base2")
	writeYAML(t, filepath.Join(tmp, "device.yaml"), ddprofiledefinition.ProfileDefinition{
		Extends: []string{"_base1.yaml", "_base2.yaml"},
	})

	prof, err := loadProfile(filepath.Join(tmp, "device.yaml"), multipath.New(tmp))
	require.NoError(t, err)
	require.Len(t, prof.Definition.Metrics, 1)
	require.Len(t, prof.Definition.Metrics[0].Symbols, 1)

	sym := prof.Definition.Metrics[0].Symbols[0]
	assert.Equal(t, "ifInOctets", sym.Name)
	assert.Equal(t, "1.3.6.1.2.1.2.2.1.16", sym.OID)
	assert.Equal(t, "base2", sym.ChartMeta.Description)
}

func TestProfile_MultipleExtends_TableSymbolLaterOverrideEarlierByTableNameWhenOIDDiffers(t *testing.T) {
	tmp := t.TempDir()

	writeTableBase(t, filepath.Join(tmp, "_base1.yaml"), "1.3.6.1.2.1.2.2", "ifTable", "1.3.6.1.2.1.2.2.1.10", "ifInOctets", "base1")
	writeTableBase(t, filepath.Join(tmp, "_base2.yaml"), "1.3.6.1.4.1.999.2", "ifTable", "1.3.6.1.4.1.999.2.1.10", "ifInOctets", "base2")
	writeYAML(t, filepath.Join(tmp, "device.yaml"), ddprofiledefinition.ProfileDefinition{
		Extends: []string{"_base1.yaml", "_base2.yaml"},
	})

	prof, err := loadProfile(filepath.Join(tmp, "device.yaml"), multipath.New(tmp))
	require.NoError(t, err)
	require.Len(t, prof.Definition.Metrics, 1)
	require.Len(t, prof.Definition.Metrics[0].Symbols, 1)

	assert.Equal(t, "1.3.6.1.4.1.999.2", prof.Definition.Metrics[0].Table.OID)
	assert.Equal(t, "ifTable", prof.Definition.Metrics[0].Table.Name)
	assert.Equal(t, "1.3.6.1.4.1.999.2.1.10", prof.Definition.Metrics[0].Symbols[0].OID)
	assert.Equal(t, "base2", prof.Definition.Metrics[0].Symbols[0].ChartMeta.Description)
}

func TestProfile_MultipleExtends_TableOIDOverrideDropsEarlierTableSymbols(t *testing.T) {
	tmp := t.TempDir()

	writeYAML(t, filepath.Join(tmp, "_base1.yaml"), ddprofiledefinition.ProfileDefinition{
		Metrics: []ddprofiledefinition.MetricsConfig{
			{
				Table: ddprofiledefinition.SymbolConfig{
					OID:  "1.3.6.1.2.1.2.2",
					Name: "ifTable",
				},
				Symbols: []ddprofiledefinition.SymbolConfig{
					{OID: "1.3.6.1.2.1.2.2.1.10", Name: "ifInOctets"},
					{OID: "1.3.6.1.2.1.2.2.1.16", Name: "ifOutOctets"},
				},
				MetricTags: []ddprofiledefinition.MetricTagConfig{
					{Tag: "row", IndexTransform: []ddprofiledefinition.MetricIndexTransform{{Start: 0, End: 0}}},
				},
			},
		},
	})
	writeTableBase(t, filepath.Join(tmp, "_base2.yaml"), "1.3.6.1.4.1.999.2", "ifTable", "1.3.6.1.4.1.999.2.1.10", "ifInOctets", "base2")
	writeYAML(t, filepath.Join(tmp, "device.yaml"), ddprofiledefinition.ProfileDefinition{
		Extends: []string{"_base1.yaml", "_base2.yaml"},
	})

	prof, err := loadProfile(filepath.Join(tmp, "device.yaml"), multipath.New(tmp))
	require.NoError(t, err)
	require.Len(t, prof.Definition.Metrics, 1)
	require.Len(t, prof.Definition.Metrics[0].Symbols, 1)

	assert.Equal(t, "1.3.6.1.4.1.999.2", prof.Definition.Metrics[0].Table.OID)
	assert.Equal(t, "ifTable", prof.Definition.Metrics[0].Table.Name)
	assert.Equal(t, "ifInOctets", prof.Definition.Metrics[0].Symbols[0].Name)
	assert.Equal(t, "1.3.6.1.4.1.999.2.1.10", prof.Definition.Metrics[0].Symbols[0].OID)
}

func TestProfile_MultipleExtends_TableSymbolsPreserveSameOIDDifferentNames(t *testing.T) {
	tmp := t.TempDir()

	writeTableBase(t, filepath.Join(tmp, "_base1.yaml"), "1.3.6.1.4.1.999.1", "eventTable", "1.3.6.1.4.1.999.1.1.5", "eventCode", "base1")
	writeTableBase(t, filepath.Join(tmp, "_base2.yaml"), "1.3.6.1.4.1.999.1", "eventTable", "1.3.6.1.4.1.999.1.1.5", "eventSubCode", "base2")
	writeYAML(t, filepath.Join(tmp, "device.yaml"), ddprofiledefinition.ProfileDefinition{
		Extends: []string{"_base1.yaml", "_base2.yaml"},
	})

	prof, err := loadProfile(filepath.Join(tmp, "device.yaml"), multipath.New(tmp))
	require.NoError(t, err)
	require.Len(t, prof.Definition.Metrics, 2)

	got := make(map[string]string)
	for _, metric := range prof.Definition.Metrics {
		require.Len(t, metric.Symbols, 1)
		got[metric.Symbols[0].Name] = metric.Symbols[0].OID
	}

	assert.Equal(t, map[string]string{
		"eventCode":    "1.3.6.1.4.1.999.1.1.5",
		"eventSubCode": "1.3.6.1.4.1.999.1.1.5",
	}, got)
}

func TestProfile_MergeMetrics_DoesNotMutateBaseColumnSymbols(t *testing.T) {
	target := &Profile{Definition: &ddprofiledefinition.ProfileDefinition{
		Metrics: []ddprofiledefinition.MetricsConfig{
			tableMetricConfig("1.3.6.1.2.1.2.2", "ifTable", "1.3.6.1.2.1.2.2.1.10", "ifInOctets", "target"),
		},
	}}
	base := &Profile{Definition: &ddprofiledefinition.ProfileDefinition{
		Metrics: []ddprofiledefinition.MetricsConfig{
			{
				Table: ddprofiledefinition.SymbolConfig{
					OID:  "1.3.6.1.2.1.2.2",
					Name: "ifTable",
				},
				Symbols: []ddprofiledefinition.SymbolConfig{
					{OID: "1.3.6.1.2.1.2.2.1.10", Name: "ifInOctets"},
					{OID: "1.3.6.1.2.1.2.2.1.16", Name: "ifOutOctets"},
				},
				MetricTags: []ddprofiledefinition.MetricTagConfig{
					{
						Tag: "row",
						IndexTransform: []ddprofiledefinition.MetricIndexTransform{
							{Start: 0, End: 0},
						},
					},
				},
			},
		},
	}}

	target.mergeMetrics(base)

	require.Len(t, base.Definition.Metrics[0].Symbols, 2)
	assert.Equal(t, "ifInOctets", base.Definition.Metrics[0].Symbols[0].Name)
	assert.Equal(t, "ifOutOctets", base.Definition.Metrics[0].Symbols[1].Name)
	require.Len(t, target.Definition.Metrics, 2)
	require.Len(t, target.Definition.Metrics[1].Symbols, 1)
	assert.Equal(t, "ifOutOctets", target.Definition.Metrics[1].Symbols[0].Name)
}

func TestProfile_MergeMetrics_PreservesRepeatedBaseColumnSymbols(t *testing.T) {
	target := &Profile{Definition: &ddprofiledefinition.ProfileDefinition{}}
	base := &Profile{Definition: &ddprofiledefinition.ProfileDefinition{
		Metrics: []ddprofiledefinition.MetricsConfig{
			{
				Table: ddprofiledefinition.SymbolConfig{
					OID:  "1.3.6.1.4.1.999.1",
					Name: "eventTable",
				},
				Symbols: []ddprofiledefinition.SymbolConfig{
					{OID: "1.3.6.1.4.1.999.1.1.5", Name: "eventCode"},
					{OID: "1.3.6.1.4.1.999.1.1.6", Name: "eventCode"},
				},
			},
		},
	}}

	target.mergeMetrics(base)

	require.Len(t, target.Definition.Metrics, 1)
	require.Len(t, target.Definition.Metrics[0].Symbols, 2)
	assert.Equal(t, "1.3.6.1.4.1.999.1.1.5", target.Definition.Metrics[0].Symbols[0].OID)
	assert.Equal(t, "1.3.6.1.4.1.999.1.1.6", target.Definition.Metrics[0].Symbols[1].OID)
}

func TestProfile_ExtendsTopologyMixinInheritsTopologyRows(t *testing.T) {
	tmp := t.TempDir()

	writeYAML(t, filepath.Join(tmp, "_topology-base.yaml"), ddprofiledefinition.ProfileDefinition{
		Topology: []ddprofiledefinition.TopologyConfig{
			topologyTableConfig(
				ddprofiledefinition.KindLldpRem,
				"1.0.8802.1.1.2.1.4.1",
				"lldpRemTable",
				"1.0.8802.1.1.2.1.4.1.1.6",
				"lldpRemPortIdSubtype",
			),
		},
	})
	writeYAML(t, filepath.Join(tmp, "device.yaml"), ddprofiledefinition.ProfileDefinition{
		Extends: []string{"_topology-base.yaml"},
	})

	prof, err := loadProfile(filepath.Join(tmp, "device.yaml"), multipath.New(tmp))
	require.NoError(t, err)
	require.Len(t, prof.Definition.Topology, 1)
	assert.Equal(t, ddprofiledefinition.KindLldpRem, prof.Definition.Topology[0].Kind)
	assert.Equal(t, "lldpRemTable", prof.Definition.Topology[0].Table.Name)
	require.Len(t, prof.Definition.Topology[0].Symbols, 1)
	assert.Equal(t, "lldpRemPortIdSubtype", prof.Definition.Topology[0].Symbols[0].Name)
}

func TestProfile_MergeTopology_DerivedOverridesDuplicateAndPreservesBaseMissingSymbols(t *testing.T) {
	target := &Profile{Definition: &ddprofiledefinition.ProfileDefinition{
		Topology: []ddprofiledefinition.TopologyConfig{
			topologyTableConfig(
				ddprofiledefinition.KindLldpRem,
				"1.0.8802.1.1.2.1.4.1",
				"lldpRemTable",
				"1.0.8802.1.1.2.1.4.1.1.99",
				"lldpRemPortIdSubtype",
			),
		},
	}}
	base := &Profile{Definition: &ddprofiledefinition.ProfileDefinition{
		Topology: []ddprofiledefinition.TopologyConfig{
			{
				Kind: ddprofiledefinition.KindLldpRem,
				MetricsConfig: ddprofiledefinition.MetricsConfig{
					Table: ddprofiledefinition.SymbolConfig{
						OID:  "1.0.8802.1.1.2.1.4.1",
						Name: "lldpRemTable",
					},
					Symbols: []ddprofiledefinition.SymbolConfig{
						{OID: "1.0.8802.1.1.2.1.4.1.1.6", Name: "lldpRemPortIdSubtype"},
						{OID: "1.0.8802.1.1.2.1.4.1.1.7", Name: "lldpRemPortId"},
					},
				},
			},
		},
	}}

	require.NoError(t, target.mergeTopology(base))

	require.Len(t, target.Definition.Topology, 2)
	require.Len(t, target.Definition.Topology[0].Symbols, 1)
	assert.Equal(t, "1.0.8802.1.1.2.1.4.1.1.99", target.Definition.Topology[0].Symbols[0].OID)
	require.Len(t, target.Definition.Topology[1].Symbols, 1)
	assert.Equal(t, "lldpRemPortId", target.Definition.Topology[1].Symbols[0].Name)
}

func TestProfile_MergeTopology_RejectsConflictingKindForSameRow(t *testing.T) {
	target := &Profile{Definition: &ddprofiledefinition.ProfileDefinition{
		Topology: []ddprofiledefinition.TopologyConfig{
			topologyTableConfig(
				ddprofiledefinition.KindLldpRem,
				"1.0.8802.1.1.2.1.4.1",
				"lldpRemTable",
				"1.0.8802.1.1.2.1.4.1.1.6",
				"lldpRemPortIdSubtype",
			),
		},
	}}
	base := &Profile{Definition: &ddprofiledefinition.ProfileDefinition{
		Topology: []ddprofiledefinition.TopologyConfig{
			topologyTableConfig(
				ddprofiledefinition.KindLldpLocPort,
				"1.0.8802.1.1.2.1.4.1",
				"lldpRemTable",
				"1.0.8802.1.1.2.1.4.1.1.6",
				"lldpRemPortIdSubtype",
			),
		},
	}}

	err := target.mergeTopology(base)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflicting topology kinds")
}

func TestDeduplicateMetricsAcrossProfiles_TopologyUsesKindTableAndSymbol(t *testing.T) {
	profiles := []*Profile{
		{
			SourceFile: "specific.yaml",
			Definition: &ddprofiledefinition.ProfileDefinition{
				Topology: []ddprofiledefinition.TopologyConfig{
					topologyTableConfig(
						ddprofiledefinition.KindLldpRem,
						"1.0.8802.1.1.2.1.4.1",
						"lldpRemTable",
						"1.0.8802.1.1.2.1.4.1.1.6",
						"lldpRemPortIdSubtype",
					),
				},
			},
		},
		{
			SourceFile: "generic.yaml",
			Definition: &ddprofiledefinition.ProfileDefinition{
				Topology: []ddprofiledefinition.TopologyConfig{
					{
						Kind: ddprofiledefinition.KindLldpRem,
						MetricsConfig: ddprofiledefinition.MetricsConfig{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  "1.0.8802.1.1.2.1.4.1",
								Name: "lldpRemTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{
								{OID: "1.0.8802.1.1.2.1.4.1.1.6", Name: "lldpRemPortIdSubtype"},
								{OID: "1.0.8802.1.1.2.1.4.1.1.7", Name: "lldpRemPortId"},
							},
						},
					},
				},
			},
		},
	}

	deduplicateMetricsAcrossProfiles(profiles)

	require.Len(t, profiles[0].Definition.Topology, 1)
	require.Len(t, profiles[1].Definition.Topology, 1)
	require.Len(t, profiles[1].Definition.Topology[0].Symbols, 1)
	assert.Equal(t, "lldpRemPortId", profiles[1].Definition.Topology[0].Symbols[0].Name)
}

func writeTableBase(t *testing.T, path, tableOID, tableName, symbolOID, symbolName, description string) {
	t.Helper()

	writeYAML(t, path, ddprofiledefinition.ProfileDefinition{
		Metrics: []ddprofiledefinition.MetricsConfig{
			tableMetricConfig(tableOID, tableName, symbolOID, symbolName, description),
		},
	})
}

func topologyTableConfig(kind ddprofiledefinition.TopologyKind, tableOID, tableName, symbolOID, symbolName string) ddprofiledefinition.TopologyConfig {
	return ddprofiledefinition.TopologyConfig{
		Kind: kind,
		MetricsConfig: ddprofiledefinition.MetricsConfig{
			Table: ddprofiledefinition.SymbolConfig{
				OID:  tableOID,
				Name: tableName,
			},
			Symbols: []ddprofiledefinition.SymbolConfig{
				{OID: symbolOID, Name: symbolName},
			},
			MetricTags: []ddprofiledefinition.MetricTagConfig{
				{Tag: "row", Index: 1},
			},
		},
	}
}

func tableMetricConfig(tableOID, tableName, symbolOID, symbolName, description string) ddprofiledefinition.MetricsConfig {
	return ddprofiledefinition.MetricsConfig{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  tableOID,
			Name: tableName,
		},
		Symbols: []ddprofiledefinition.SymbolConfig{
			{
				OID:  symbolOID,
				Name: symbolName,
				ChartMeta: ddprofiledefinition.ChartMeta{
					Description: description,
				},
			},
		},
		MetricTags: []ddprofiledefinition.MetricTagConfig{
			{
				Tag: "row",
				IndexTransform: []ddprofiledefinition.MetricIndexTransform{
					{Start: 0, End: 0},
				},
			},
		},
	}
}
