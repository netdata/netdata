// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"

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
