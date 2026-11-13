// SPDX-License-Identifier: GPL-3.0-or-later

package ddprofiledefinition

import (
	"fmt"
	"strings"
)

// OutOfTableOIDs reports same-table symbol and tag OIDs that lie outside their row's table.OID.
// Row acquisition walks only table.OID, so such OIDs are never collected. An OID may also be a
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
	for i := range p.Licensing {
		row := &p.Licensing[i]
		issues = append(issues, tagOutOfTableOIDs(fmt.Sprintf("licensing[%d]", i), row.Table, row.MetricTags)...)
	}
	for i := range p.BGP {
		row := &p.BGP[i]
		issues = append(issues, tagOutOfTableOIDs(fmt.Sprintf("bgp[%d]", i), row.Table, row.MetricTags)...)
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
	issues = append(issues, tagOutOfTableOIDs(path, row.Table, row.MetricTags)...)

	return issues
}

func tagOutOfTableOIDs(path string, table SymbolConfig, tags []MetricTagConfig) []string {
	if table.OID == "" {
		return nil
	}
	var issues []string
	for i, tag := range tags {
		check := func(oid string) {
			if oid != "" && !isWithinTable(strings.TrimPrefix(table.OID, "."), strings.TrimPrefix(oid, ".")) {
				issues = append(
					issues,
					fmt.Sprintf("%s.metric_tags[%d]: OID %s is outside table.OID %s and is never collected",
						path, i, oid, table.OID),
				)
			}
		}
		if tag.Table == "" {
			check(tag.Symbol.OID)
		}
		for source := range tag.TableSources() {
			if source.Table == table.Name {
				check(source.Symbol.OID)
			}
		}
	}
	return issues
}

func isWithinTable(table, oid string) bool {
	return oid == table || strings.HasPrefix(oid, table+".") || strings.HasPrefix(table, oid+".")
}
