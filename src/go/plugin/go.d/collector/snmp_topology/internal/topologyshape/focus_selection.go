// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

func collectTopologyFocusRoots(graph topologyFocusGraph, focusIPs []string) map[topologymodel.ActorHandle]struct{} {
	roots := make(map[topologymodel.ActorHandle]struct{})
	normalizedFocusIPs := make(map[string]struct{}, len(focusIPs))
	for _, focusIP := range focusIPs {
		if ip := topologyutil.NormalizeIPAddress(focusIP); ip != "" {
			normalizedFocusIPs[ip] = struct{}{}
		}
	}
	for actorHandle, actor := range graph.actorByHandle {
		if _, ok := graph.nonSegmentSet[actorHandle]; !ok {
			continue
		}
		if !topologymodel.IsManagedSNMPDeviceActor(actor) {
			continue
		}
		actorIPs := topologyActorIPs(actor)
		for focusIP := range normalizedFocusIPs {
			if _, ok := actorIPs[focusIP]; !ok {
				continue
			}
			roots[actorHandle] = struct{}{}
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
	_, ok := topologyActorIPs(actor)[ip]
	return ok
}

func topologyActorIPs(actor topologymodel.Actor) map[string]struct{} {
	matchIPs := topologymodel.NormalizedMatchIPs(actor.Match)
	managementIPs := topologymodel.ActorDetailManagementIPs(actor)
	ips := make(map[string]struct{}, len(matchIPs)+len(managementIPs))
	for _, ip := range matchIPs {
		ips[ip] = struct{}{}
	}
	for _, ip := range managementIPs {
		ips[ip] = struct{}{}
	}
	return ips
}
