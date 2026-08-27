// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"github.com/netdata/netdata/go/plugins/pkg/topology/worklimit"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

func collectTopologyFocusRoots(graph topologyFocusGraph, focusIPs []string) map[topologymodel.ActorHandle]struct{} {
	roots, _ := collectTopologyFocusRootsWithLimiter(graph, focusIPs, nil)
	return roots
}

func collectTopologyFocusRootsWithLimiter(graph topologyFocusGraph, focusIPs []string, limiter worklimit.Limiter) (map[topologymodel.ActorHandle]struct{}, error) {
	if err := worklimit.ChargeStrings(limiter, focusIPs); err != nil {
		return nil, err
	}
	roots := make(map[topologymodel.ActorHandle]struct{})
	normalizedFocusIPs := make(map[string]struct{}, len(focusIPs))
	for _, focusIP := range focusIPs {
		if ip := topologyutil.NormalizeIPAddress(focusIP); ip != "" {
			normalizedFocusIPs[ip] = struct{}{}
		}
	}
	if err := limiter.Charge(uint64(len(graph.actorByHandle))); err != nil {
		return nil, err
	}
	for actorHandle, actor := range graph.actorByHandle {
		if _, ok := graph.nonSegmentSet[actorHandle]; !ok {
			continue
		}
		if !topologymodel.IsManagedSNMPDeviceActor(actor) {
			continue
		}
		actorIPs, err := topologyActorIPsWithLimiter(actor, limiter)
		if err != nil {
			return nil, err
		}
		if err := limiter.Charge(uint64(len(normalizedFocusIPs))); err != nil {
			return nil, err
		}
		for focusIP := range normalizedFocusIPs {
			if _, ok := actorIPs[focusIP]; !ok {
				continue
			}
			roots[actorHandle] = struct{}{}
			break
		}
	}
	return roots, nil
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
	ips, _ := topologyActorIPsWithLimiter(actor, nil)
	return ips
}

func topologyActorIPsWithLimiter(actor topologymodel.Actor, limiter worklimit.Limiter) (map[string]struct{}, error) {
	matchIPs, err := topologymodel.NormalizedMatchIPsWithLimiter(actor.Match, limiter)
	if err != nil {
		return nil, err
	}
	managementIPs := topologymodel.ActorDetailManagementIPs(actor)
	if err := worklimit.ChargeStrings(limiter, managementIPs); err != nil {
		return nil, err
	}
	ips := make(map[string]struct{}, len(matchIPs)+len(managementIPs))
	for _, ip := range matchIPs {
		ips[ip] = struct{}{}
	}
	for _, ip := range managementIPs {
		ips[ip] = struct{}{}
	}
	return ips, nil
}
