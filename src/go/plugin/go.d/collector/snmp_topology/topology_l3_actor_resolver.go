// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"
)

type topologyL3ActorRef struct {
	actorID string
	match   topologyMatch
}

type topologyL3ActorResolver struct {
	byActorID  map[string]topologyL3ActorRef
	byDeviceID map[string]topologyL3ActorRef
	byIP       map[string]topologyL3ActorRef
	byRouterID map[string]topologyL3ActorRef
}

func newTopologyL3ActorResolver(data *topologyData, snapshots []topologyObservationSnapshot) topologyL3ActorResolver {
	resolver := topologyL3ActorResolver{
		byActorID:  make(map[string]topologyL3ActorRef),
		byDeviceID: make(map[string]topologyL3ActorRef),
		byIP:       make(map[string]topologyL3ActorRef),
		byRouterID: make(map[string]topologyL3ActorRef),
	}
	if data == nil || len(data.Actors) == 0 {
		return resolver
	}

	managedActors := make([]topologyActor, 0, len(data.Actors))
	for _, actor := range data.Actors {
		if !isManagedSNMPDeviceActor(actor) {
			continue
		}
		managedActors = append(managedActors, actor)
		ref := topologyL3ActorRef{
			actorID: strings.TrimSpace(actor.ActorID),
			match:   actor.Match,
		}
		if ref.actorID != "" {
			resolver.byActorID[ref.actorID] = ref
		}
		if deviceID := topologyMetricValueString(actor.Attributes, "device_id"); deviceID != "" {
			resolver.addUniqueDeviceID(deviceID, ref)
		}
		for _, ip := range normalizedMatchIPs(actor.Match) {
			resolver.addUniqueIP(ip, ref)
		}
		for _, routerID := range topologyL3ActorRouterIDs(actor) {
			resolver.addUniqueRouterID(routerID, ref)
		}
	}

	for _, snapshot := range snapshots {
		deviceID := strings.TrimSpace(snapshot.localDeviceID)
		if deviceID == "" {
			continue
		}
		for _, actor := range managedActors {
			if !matchLocalTopologyActor(actor.Match, snapshot.localDevice) {
				continue
			}
			resolver.addUniqueDeviceID(deviceID, topologyL3ActorRef{
				actorID: strings.TrimSpace(actor.ActorID),
				match:   actor.Match,
			})
			resolver.addUniqueRouterID(snapshot.localDevice.OSPFRouterID, topologyL3ActorRef{
				actorID: strings.TrimSpace(actor.ActorID),
				match:   actor.Match,
			})
		}
	}

	return resolver
}

func (r topologyL3ActorResolver) resolve(row topologyL3Interface) (topologyL3ActorRef, bool) {
	if ref, ok := r.byDeviceID[strings.TrimSpace(row.DeviceID)]; ok && ref.actorID != "" {
		return ref, true
	}
	if ref, ok := r.byActorID[strings.TrimSpace(row.DeviceID)]; ok && ref.actorID != "" {
		return ref, true
	}
	if ref, ok := r.byIP[normalizeIPAddress(row.IP)]; ok && ref.actorID != "" {
		return ref, true
	}
	return topologyL3ActorRef{}, false
}

func (r topologyL3ActorResolver) resolveDeviceID(deviceID string) (topologyL3ActorRef, bool) {
	if ref, ok := r.byDeviceID[strings.TrimSpace(deviceID)]; ok && ref.actorID != "" {
		return ref, true
	}
	if ref, ok := r.byActorID[strings.TrimSpace(deviceID)]; ok && ref.actorID != "" {
		return ref, true
	}
	return topologyL3ActorRef{}, false
}

func (r topologyL3ActorResolver) resolveOSPFNeighbor(row topologyOSPFNeighbor) (topologyL3ActorRef, bool) {
	if ref, ok := r.byRouterID[normalizeOSPFRouterID(row.NeighborRouterID)]; ok && ref.actorID != "" {
		return ref, true
	}
	if ref, ok := r.byIP[normalizeIPAddress(row.NeighborIP)]; ok && ref.actorID != "" {
		return ref, true
	}
	return topologyL3ActorRef{}, false
}

func (r topologyL3ActorResolver) addUniqueDeviceID(deviceID string, ref topologyL3ActorRef) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" || ref.actorID == "" {
		return
	}
	existing, ok := r.byDeviceID[deviceID]
	if !ok {
		r.byDeviceID[deviceID] = ref
		return
	}
	if existing.actorID != "" && existing.actorID != ref.actorID {
		r.byDeviceID[deviceID] = topologyL3ActorRef{}
	}
}

func (r topologyL3ActorResolver) addUniqueIP(ip string, ref topologyL3ActorRef) {
	ip = normalizeIPAddress(ip)
	if ip == "" || ref.actorID == "" {
		return
	}
	existing, ok := r.byIP[ip]
	if !ok {
		r.byIP[ip] = ref
		return
	}
	if existing.actorID != "" && existing.actorID != ref.actorID {
		r.byIP[ip] = topologyL3ActorRef{}
	}
}

func (r topologyL3ActorResolver) addUniqueRouterID(routerID string, ref topologyL3ActorRef) {
	routerID = normalizeOSPFRouterID(routerID)
	if routerID == "" || ref.actorID == "" {
		return
	}
	existing, ok := r.byRouterID[routerID]
	if !ok {
		r.byRouterID[routerID] = ref
		return
	}
	if existing.actorID != "" && existing.actorID != ref.actorID {
		r.byRouterID[routerID] = topologyL3ActorRef{}
	}
}

func topologyL3ActorRouterIDs(actor topologyActor) []string {
	values := make([]string, 0, 2)
	if routerID := normalizeOSPFRouterID(anyStringValue(actor.Attributes[tagOSPFRouterID])); routerID != "" {
		values = append(values, routerID)
	}
	if routerID := normalizeOSPFRouterID(actor.Labels[tagOSPFRouterID]); routerID != "" {
		values = append(values, routerID)
	}
	return values
}
