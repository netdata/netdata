// SPDX-License-Identifier: GPL-3.0-or-later

package engine

func newL2ResultStats() map[string]any {
	return map[string]any{
		"devices_total":                          0,
		"links_total":                            0,
		"links_lldp":                             0,
		"links_cdp":                              0,
		"links_stp":                              0,
		"attachments_total":                      0,
		"attachments_fdb":                        0,
		"enrichments_total":                      0,
		"enrichments_arp_nd":                     0,
		"bridge_domains_total":                   0,
		"endpoints_total":                        0,
		"identity_alias_endpoints_mapped":        0,
		"identity_alias_endpoints_ambiguous_mac": 0,
		"identity_alias_ips_merged":              0,
		"identity_alias_ips_conflict_skipped":    0,
	}
}
