// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"

// Legacy tag columns can anchor typed rows. Chained targets may use a different
// row index, even in the source table, and must remain lookup-only.
func addTypedRowTagColumns(columns map[string]ddprofiledefinition.SymbolConfig, tags []ddprofiledefinition.MetricTagConfig) {
	for _, tag := range tags {
		if len(tag.IndexFrom) == 0 && tag.Symbol.OID != "" {
			columns[trimOID(tag.Symbol.OID)] = ddprofiledefinition.SymbolConfig(tag.Symbol)
		}
	}
}

// Declared table routes own their scope; only infer roots for missing dependencies.
func addReferencedTableOIDs(
	tag ddprofiledefinition.MetricTagConfig,
	known map[string]string,
	columns map[string][]string,
) {
	for source := range tag.TableSources() {
		if _, ok := known[source.Table]; !ok {
			columns[source.Table] = append(columns[source.Table], source.Symbol.OID)
		}
	}
}
