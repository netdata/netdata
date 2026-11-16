// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopologyfunc

import (
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

func rootFocusConfig(prune bool) *funcapi.FocusConfig {
	traversal := &funcapi.FocusTraversal{
		ConnectorTypes: []string{"segment"}, BoundaryTypes: []string{"l3_subnet_segment"}, ConnectRoots: true,
	}
	if prune {
		traversal.PruneLeafTypes = []string{"segment", "l3_subnet_segment"}
		traversal.ProtectedMemberships = []funcapi.FocusProtectedMembership{{ActorType: "l3_subnet_segment", LinkType: "l3_subnet_membership"}}
	}
	return &funcapi.FocusConfig{
		Role: funcapi.FocusRoleRoots, UnfocusedOption: topologyoptions.ManagedFocusAllDevices,
		Bindings:  &funcapi.FocusBindings{Table: topologyoptions.ManagedFocusBindingsTable, OptionColumn: "option", ActorColumn: "actor"},
		Traversal: traversal,
	}
}
