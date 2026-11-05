// SPDX-License-Identifier: GPL-3.0-or-later

package ddprofiledefinition

import (
	"fmt"
	"strings"
)

// OutOfTableOIDs reports same-table symbol and tag OIDs that lie outside their row's table.OID. Metric and topology
// rows walk only table.OID, so such OIDs are never collected and the collector reports nothing. An OID may also be a
// prefix of table.OID: that is an anchor column whose table.OID appends a fixed index prefix. Call it after
// ValidateEnrichProfile, which moves legacy tag OIDs into symbol.OID.
func OutOfTableOIDs(p *ProfileDefinition) []string {
	var issues []string
	for i := range p.Metrics {
		issues = append(issues, rowOutOfTableOIDs(fmt.Sprintf("metrics[%d]", i), &p.Metrics[i])...)
	}
	for i := range p.Topology {
		issues = append(issues, rowOutOfTableOIDs(fmt.Sprintf("topology[%d]", i), &p.Topology[i].MetricsConfig)...)
	}
	return issues
}

func rowOutOfTableOIDs(path string, row *MetricsConfig) []string {
	if !row.IsColumn() || row.Table.OID == "" {
		return nil
	}

	table := strings.TrimPrefix(row.Table.OID, ".")
	var issues []string
	check := func(field, oid string) {
		if oid == "" || isWithinTable(table, strings.TrimPrefix(oid, ".")) {
			return
		}
		issues = append(issues, fmt.Sprintf("%s.%s: OID %s is outside table.OID %s and is never collected",
			path, field, oid, row.Table.OID))
	}

	for i, sym := range row.Symbols {
		check(fmt.Sprintf("symbols[%d]", i), sym.OID)
	}
	for i, tag := range row.MetricTags {
		if tag.Table != "" && tag.Table != row.Table.Name {
			continue // cross-table tags read the referenced table
		}
		check(fmt.Sprintf("metric_tags[%d]", i), tag.Symbol.OID)
	}
	return issues
}

func isWithinTable(table, oid string) bool {
	return oid == table || strings.HasPrefix(oid, table+".") || strings.HasPrefix(table, oid+".")
}
