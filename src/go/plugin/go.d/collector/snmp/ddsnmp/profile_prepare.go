// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"fmt"
	"sort"
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
// for such tables. The walk root is normally the longest common OID prefix of the
// referenced columns, including lookup columns used by value-based joins. A fixed
// structural index prefix on a single readable anchor is also propagated to simple
// same-index dependency columns so both sides retain the same narrowed row scope.
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

	tagCrossTableOnlyOIDs := crossTableOnlyTagOIDs(*metrics, seenTableNames, false)
	for _, tableName := range sortedDependencyNames(tagCrossTableOnlyOIDs) {
		dependency := tagCrossTableOnlyOIDs[tableName]
		*metrics = append(*metrics, syntheticCrossTableMetric(tableName, dependency.walkOID()))
	}
}

func handleCrossTableTagsWithoutMetricsForTopologyRows(topology *[]ddprofiledefinition.TopologyConfig) {
	seenTableNames := make(map[string]bool)
	for _, topo := range *topology {
		seenTableNames[topo.Table.Name] = true
	}

	metrics := make([]ddprofiledefinition.MetricsConfig, 0, len(*topology))
	dependencyKinds := make(map[string]ddprofiledefinition.TopologyKind)
	for _, topo := range *topology {
		metrics = append(metrics, topo.MetricsConfig)
		for _, tag := range topo.MetricTags {
			if tag.Table != "" && !seenTableNames[tag.Table] {
				if _, ok := dependencyKinds[tag.Table]; !ok {
					dependencyKinds[tag.Table] = topo.Kind
				}
			}
		}
	}

	tagCrossTableOnlyOIDs := crossTableOnlyTagOIDs(metrics, seenTableNames, true)
	for _, tableName := range sortedDependencyNames(tagCrossTableOnlyOIDs) {
		*topology = append(*topology, ddprofiledefinition.TopologyConfig{
			Kind:          dependencyKinds[tableName],
			MetricsConfig: syntheticCrossTableMetric(tableName, tagCrossTableOnlyOIDs[tableName].walkOID()),
		})
	}
}

type crossTableDependency struct {
	oids          []string
	scopedWalkOID string
	scopeConflict bool
	seen          bool
}

func (d *crossTableDependency) add(tag ddprofiledefinition.MetricTagConfig, indexScope string) {
	if tag.Symbol.OID != "" {
		d.oids = append(d.oids, tag.Symbol.OID)
	}
	if tag.LookupSymbol.OID != "" {
		d.oids = append(d.oids, tag.LookupSymbol.OID)
	}

	scopedWalkOID := ""
	if indexScope != "" && tag.Index == 0 && len(tag.IndexTransform) == 0 &&
		tag.Symbol.OID != "" && tag.LookupSymbol.OID == "" {
		// A direct cross-table join already requires the dependency to use the
		// anchor's unchanged row-index suffix. Narrowing only preserves its fixed prefix.
		scopedWalkOID = trimProfileOID(tag.Symbol.OID) + "." + indexScope
	}
	if d.seen && d.scopedWalkOID != scopedWalkOID {
		d.scopeConflict = true
	}
	d.seen = true
	d.scopedWalkOID = scopedWalkOID
}

func (d crossTableDependency) walkOID() string {
	if !d.scopeConflict && d.scopedWalkOID != "" {
		return d.scopedWalkOID
	}
	return longestCommonPrefix(d.oids)
}

func crossTableOnlyTagOIDs(
	metrics []ddprofiledefinition.MetricsConfig,
	seenTableNames map[string]bool,
	propagateIndexScope bool,
) map[string]*crossTableDependency {
	tagCrossTableOnlyOIDs := make(map[string]*crossTableDependency)
	for _, m := range metrics {
		if m.IsScalar() {
			continue
		}
		indexScope := ""
		if propagateIndexScope {
			indexScope = tableIndexScope(m)
		}
		for _, tag := range m.MetricTags {
			if tag.Table == "" || seenTableNames[tag.Table] {
				continue
			}
			dependency := tagCrossTableOnlyOIDs[tag.Table]
			if dependency == nil {
				dependency = &crossTableDependency{}
				tagCrossTableOnlyOIDs[tag.Table] = dependency
			}
			dependency.add(tag, indexScope)
		}
	}
	return tagCrossTableOnlyOIDs
}

func sortedDependencyNames(dependencies map[string]*crossTableDependency) []string {
	names := make([]string, 0, len(dependencies))
	for name := range dependencies {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func syntheticCrossTableMetric(tableName, walkOID string) ddprofiledefinition.MetricsConfig {
	return ddprofiledefinition.MetricsConfig{
		MIB: fmt.Sprintf("synthetic-%s-MIB", tableName),
		Table: ddprofiledefinition.SymbolConfig{
			OID:  walkOID,
			Name: tableName,
		},
	}
}

func tableIndexScope(metric ddprofiledefinition.MetricsConfig) string {
	if len(metric.Symbols) != 1 {
		return ""
	}
	tableOID := trimProfileOID(metric.Table.OID)
	columnOID := trimProfileOID(metric.Symbols[0].OID)
	scope, ok := strings.CutPrefix(tableOID, columnOID+".")
	if !ok {
		return ""
	}
	return scope
}

func trimProfileOID(oid string) string {
	return strings.Trim(oid, ".")
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
