// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/enrichment"

var emptyTrapEnricher enrichment.Enricher

// enrichTrapEntry applies the collector's immutable enrichment dependencies.
// Reverse-DNS enablement remains job-local configuration.
func (c *Collector) enrichTrapEntry(entry *TrapEntry) {
	if c == nil {
		return
	}
	enricher := c.enricher
	if enricher == nil {
		enricher = &emptyTrapEnricher
	}
	enricher.Enrich(entry, c.reverseDNSEnabled)
}
