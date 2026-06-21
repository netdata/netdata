// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
	"slices"
)

func collectTopologyFocusRoots(graph topologyFocusGraph, focusIPs []string) map[string]struct{} {
	roots := make(map[string]struct{})
	for actorID, actor := range graph.actorByID {
		if _, ok := graph.nonSegmentSet[actorID]; !ok {
			continue
		}
		if !topologymodel.IsManagedSNMPDeviceActor(actor) {
			continue
		}
		for _, focusIP := range focusIPs {
			if !topologyActorHasIP(actor, focusIP) {
				continue
			}
			roots[actorID] = struct{}{}
			break
		}
	}
	return roots
}

func topologyActorHasIP(actor topologymodel.Actor, ip string) bool {
	ip = topologyutil.NormalizeIPAddress(ip)
	if ip == "" {
		return false
	}
	if slices.Contains(topologymodel.NormalizedMatchIPs(actor.Match), ip) {
		return true
	}
	return slices.Contains(topologymodel.ActorDetailManagementIPs(actor), ip)
}
