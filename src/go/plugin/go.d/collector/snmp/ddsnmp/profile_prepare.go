// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"fmt"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// HandleCrossTableTagsWithoutMetrics is the exported entry point for callers outside
// this package (notably tests in ddsnmpcollector) that need to prepare a profile in
// isolation, without the additional enrichment performed by FinalizeProfiles.
func HandleCrossTableTagsWithoutMetrics(prof *Profile) {
	handleCrossTableTagsWithoutMetrics(prof)
}

// handleCrossTableTagsWithoutMetrics ensures tables referenced only by cross-table tags
// are still walked during collection. Without this, if a table like ifXTable is used
// only for cross-table tags (e.g., getting interface names) but has no metrics defined,
// it won't be walked and the tags will be missing. This creates synthetic metric entries
// for such tables using the longest common OID prefix of the referenced columns, including
// lookup columns used by value-based joins.
func handleCrossTableTagsWithoutMetrics(prof *Profile) {
	if prof.Definition == nil {
		return
	}

	handleCrossTableTagsWithoutMetricsForRows(&prof.Definition.Metrics)
	handleCrossTableTagsWithoutMetricsForTopologyRows(&prof.Definition.Topology)
}

func handleCrossTableTagsWithoutMetricsForRows(metrics *[]ddprofiledefinition.MetricsConfig) {
	seenTableNames := make(map[string]bool)
	for _, m := range *metrics {
		seenTableNames[m.Table.Name] = true
	}

	tagCrossTableOnlyOIDs := crossTableOnlyTagOIDs(*metrics, seenTableNames)
	for tableName, oids := range tagCrossTableOnlyOIDs {
		*metrics = append(*metrics, syntheticCrossTableMetric(tableName, oids))
	}
}

func handleCrossTableTagsWithoutMetricsForTopologyRows(topology *[]ddprofiledefinition.TopologyConfig) {
	seenTableNames := make(map[string]bool)
	for _, topo := range *topology {
		seenTableNames[topo.Table.Name] = true
	}

	for i := range *topology {
		topo := &(*topology)[i]
		tagCrossTableOnlyOIDs := crossTableOnlyTagOIDs([]ddprofiledefinition.MetricsConfig{topo.MetricsConfig}, seenTableNames)
		for tableName, oids := range tagCrossTableOnlyOIDs {
			*topology = append(*topology, ddprofiledefinition.TopologyConfig{
				Kind:          topo.Kind,
				MetricsConfig: syntheticCrossTableMetric(tableName, oids),
			})
			seenTableNames[tableName] = true
		}
	}
}

func crossTableOnlyTagOIDs(metrics []ddprofiledefinition.MetricsConfig, seenTableNames map[string]bool) map[string][]string {
	tagCrossTableOnlyOIDs := make(map[string][]string)
	for _, m := range metrics {
		if m.IsScalar() {
			continue
		}
		for _, tag := range m.MetricTags {
			if tag.Table == "" || seenTableNames[tag.Table] {
				continue
			}
			if tag.Symbol.OID != "" {
				tagCrossTableOnlyOIDs[tag.Table] = append(tagCrossTableOnlyOIDs[tag.Table], tag.Symbol.OID)
			}
			if tag.LookupSymbol.OID != "" {
				tagCrossTableOnlyOIDs[tag.Table] = append(tagCrossTableOnlyOIDs[tag.Table], tag.LookupSymbol.OID)
			}
		}
	}
	return tagCrossTableOnlyOIDs
}

func syntheticCrossTableMetric(tableName string, oids []string) ddprofiledefinition.MetricsConfig {
	slices.Sort(oids)
	oids = slices.Compact(oids)

	return ddprofiledefinition.MetricsConfig{
		MIB: fmt.Sprintf("synthetic-%s-MIB", tableName),
		Table: ddprofiledefinition.SymbolConfig{
			OID:  longestCommonPrefix(oids),
			Name: tableName,
		},
	}
}

func longestCommonPrefix(oids []string) string {
	if len(oids) == 0 {
		return ""
	}

	prefixParts := splitOIDParts(oids[0])
	for i := 1; i < len(oids); i++ {
		parts := splitOIDParts(oids[i])
		n := min(len(parts), len(prefixParts))

		j := 0
		for j < n && prefixParts[j] == parts[j] {
			j++
		}
		prefixParts = prefixParts[:j]
		if len(prefixParts) == 0 {
			return ""
		}
	}

	return strings.Join(prefixParts, ".")
}

func splitOIDParts(oid string) []string {
	parts := strings.Split(strings.Trim(oid, "."), ".")
	if len(parts) == 1 && parts[0] == "" {
		return nil
	}
	return parts
}
