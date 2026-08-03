// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

// enrichTrapEntry applies the collector's immutable enrichment dependencies.
// Reverse-DNS enablement remains job-local configuration.
func (c *Collector) enrichTrapEntry(entry *TrapEntry) {
	if c == nil {
		return
	}
	c.enricher.Enrich(entry, c.reverseDNSEnabled)
}
