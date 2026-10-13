// SPDX-License-Identifier: GPL-3.0-or-later

// Package l2topology converts normalized layer-2 observations into a generic
// topology graph projection.
//
// The package is collection-method neutral: callers provide already-normalized
// LLDP, CDP, bridge/FDB, STP, and ARP/ND observations. The current production
// caller is the SNMP topology collector.
package l2topology
