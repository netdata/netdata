// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

// resolveTopologyReverseDNSNameNoop is the non-blocking DNS resolver hook used
// during function responses. SNMP topology has no reverse-DNS warmer today.
func resolveTopologyReverseDNSNameNoop(_ string) string {
	return ""
}
