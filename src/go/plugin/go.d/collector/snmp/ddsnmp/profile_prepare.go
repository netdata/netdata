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
			for source := range tag.TableSources() {
				if !seenTableNames[source.Table] {
					if _, ok := dependencyKinds[source.Table]; !ok {
						dependencyKinds[source.Table] = topo.Kind
					}
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

func (d *crossTableDependency) add(oid, indexScope string) {
	d.oids = append(d.oids, oid)

	scopedWalkOID := ""
	if indexScope != "" {
		// A direct cross-table join already requires the dependency to use the
		// anchor's unchanged row-index suffix. Narrowing only preserves its fixed prefix.
		scopedWalkOID = trimProfileOID(oid) + "." + indexScope
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
	return ddprofiledefinition.CommonOIDPrefix(d.oids)
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
			scope := indexScope
			if tag.IndexDecode != nil || tag.Index != 0 || len(tag.IndexTransform) > 0 || len(tag.IndexFrom) > 0 || tag.LookupSymbol.OID != "" {
				scope = ""
			}
			for source := range tag.TableSources() {
				if seenTableNames[source.Table] {
					continue
				}
				dependency := tagCrossTableOnlyOIDs[source.Table]
				if dependency == nil {
					dependency = &crossTableDependency{}
					tagCrossTableOnlyOIDs[source.Table] = dependency
				}
				dependency.add(source.Symbol.OID, scope)
			}
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
