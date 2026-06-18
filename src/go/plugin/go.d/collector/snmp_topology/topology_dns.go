// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

// resolveTopologyReverseDNSNameCached is the non-blocking DNS resolver hook used
// during function responses. SNMP topology has no reverse-DNS warmer today.
func resolveTopologyReverseDNSNameCached(_ string) string {
	return ""
}
