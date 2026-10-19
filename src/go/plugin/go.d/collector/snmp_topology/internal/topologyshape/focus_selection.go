// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
	"strings"
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
		if !managedFocusEligible(actor) {
			continue
		}
		actorIPs := topologyActorIPs(actor)
		for actorIP := range actorIPs {
			if _, ok := normalizedFocusIPs[actorIP]; !ok {
				continue
			}
			roots[actorHandle] = struct{}{}
			break
		}
	}
	return roots
}

func managedFocusEligible(actor topologymodel.Actor) bool {
	return !actor.ActorHandle.IsZero() && !topologymodel.ActorIsSegment(actor) && topologymodel.IsManagedSNMPDeviceActor(actor)
}

// BuildManagedFocusBindings uses the same reconciled address authority as local focus.
func BuildManagedFocusBindings(actors []topologymodel.Actor, targets []topologyoptions.ManagedFocusTarget) []topologymodel.FocusRootBinding {
	byIP := make(map[string][]topologymodel.ActorHandle)
	for _, actor := range actors {
		if !managedFocusEligible(actor) {
			continue
		}
		for ip := range topologyActorIPs(actor) {
			byIP[ip] = append(byIP[ip], actor.ActorHandle)
		}
	}
	seen := make(map[string]struct{}, len(targets))
	var bindings []topologymodel.FocusRootBinding
	for _, target := range targets {
		if _, ok := seen[target.Value]; ok {
			continue
		}
		seen[target.Value] = struct{}{}
		if !strings.HasPrefix(target.Value, topologyoptions.ManagedFocusIPPrefix) {
			continue
		}
		ip := topologyutil.NormalizeIPAddress(strings.TrimPrefix(target.Value, topologyoptions.ManagedFocusIPPrefix))
		for _, handle := range byIP[ip] {
			bindings = append(bindings, topologymodel.FocusRootBinding{OptionID: target.Value, ActorHandle: handle})
		}
	}
	return bindings
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
