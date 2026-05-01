// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

// Topology profile selection is declarative: vendor/root profiles extend these
// mixins directly, and snmp_topology relies on the normal FindProfiles path.

const (
	topologyLldpProfileName = "_std-topology-lldp-mib.yaml"
	cdpProfileName          = "_std-cdp-mib.yaml"
	fdbArpProfileName       = "_std-topology-fdb-arp-mib.yaml"
	qBridgeProfileName      = "_std-topology-q-bridge-mib.yaml"
	stpProfileName          = "_std-topology-stp-mib.yaml"
	vtpProfileName          = "_std-topology-cisco-vtp-mib.yaml"
)
