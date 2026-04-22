// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import "github.com/netdata/netdata/go/plugins/pkg/funcapi"

// TopologyHandler is set by the snmp_topology module at init time.
// The snmp module's function router delegates topology:snmp requests to it.
// This avoids circular imports: snmp -> ddsnmp <- snmp_topology.
var TopologyHandler funcapi.MethodHandler

// TopologyMethodConfig is set by the snmp_topology module at init time.
// The snmp module includes it in its method list so the function appears
// as snmp:topology:snmp.
var TopologyMethodConfig *funcapi.MethodConfig
