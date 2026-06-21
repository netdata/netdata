// SPDX-License-Identifier: GPL-3.0-or-later

package topologyenrich

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

type topologyL3ActorRef struct {
	actorID string
	match   topologymodel.Match
}

type topologyL3ActorResolver struct {
	byActorID  map[string]topologyL3ActorRef
	byDeviceID map[string]topologyL3ActorRef
	byIP       map[string]topologyL3ActorRef
	byRouterID map[string]topologyL3ActorRef
}

func newTopologyL3ActorResolver(data *topologymodel.Data, snapshots []topologymodel.ObservationSnapshot) topologyL3ActorResolver {
	resolver := topologyL3ActorResolver{
		byActorID:  make(map[string]topologyL3ActorRef),
		byDeviceID: make(map[string]topologyL3ActorRef),
		byIP:       make(map[string]topologyL3ActorRef),
		byRouterID: make(map[string]topologyL3ActorRef),
	}
	if data == nil || len(data.Actors) == 0 {
		return resolver
	}

	managedActors := make([]topologymodel.Actor, 0, len(data.Actors))
	for _, actor := range data.Actors {
		if !topologymodel.IsManagedSNMPDeviceActor(actor) {
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
		if deviceID := topologymodel.ActorDetailDeviceID(actor); deviceID != "" {
			resolver.addUniqueDeviceID(deviceID, ref)
		}
		for _, ip := range topologymodel.NormalizedMatchIPs(actor.Match) {
			resolver.addUniqueIPAddress(ip, ref)
		}
		for _, routerID := range topologyL3ActorRouterIDs(actor) {
			resolver.addUniqueRouterID(routerID, ref)
		}
	}

	for _, snapshot := range snapshots {
		deviceID := strings.TrimSpace(snapshot.LocalDeviceID)
		if deviceID == "" {
			continue
		}
		for _, actor := range managedActors {
			if !topologymodel.MatchLocalActor(actor.Match, snapshot.LocalDevice) {
				continue
			}
			resolver.addUniqueDeviceID(deviceID, topologyL3ActorRef{
				actorID: strings.TrimSpace(actor.ActorID),
				match:   actor.Match,
			})
			resolver.addUniqueRouterID(snapshot.LocalDevice.OSPFRouterID, topologyL3ActorRef{
				actorID: strings.TrimSpace(actor.ActorID),
				match:   actor.Match,
			})
		}
	}

	return resolver
}

func (r topologyL3ActorResolver) resolve(row topologymodel.L3Interface) (topologyL3ActorRef, bool) {
	if ref, ok := r.byDeviceID[strings.TrimSpace(row.DeviceID)]; ok && ref.actorID != "" {
		return ref, true
	}
	if ref, ok := r.byActorID[strings.TrimSpace(row.DeviceID)]; ok && ref.actorID != "" {
		return ref, true
	}
	return r.resolveIPAddress(row.IP)
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

func (r topologyL3ActorResolver) resolveRouterEndpoint(routerID, ip string) (topologyL3ActorRef, bool) {
	if ref, ok := r.resolveRouterID(routerID); ok {
		return ref, true
	}
	return r.resolveNonUnspecifiedIPAddress(ip)
}

func (r topologyL3ActorResolver) resolveRouterID(routerID string) (topologyL3ActorRef, bool) {
	if ref, ok := r.byRouterID[topologyutil.NormalizeTopologyRouterID(routerID)]; ok && ref.actorID != "" {
		return ref, true
	}
	return topologyL3ActorRef{}, false
}

func (r topologyL3ActorResolver) resolveIPAddress(ip string) (topologyL3ActorRef, bool) {
	if ref, ok := r.byIP[topologyutil.NormalizeIPAddress(ip)]; ok && ref.actorID != "" {
		return ref, true
	}
	return topologyL3ActorRef{}, false
}

func (r topologyL3ActorResolver) resolveNonUnspecifiedIPAddress(ip string) (topologyL3ActorRef, bool) {
	if ref, ok := r.byIP[topologyutil.NormalizeNonUnspecifiedIPAddress(ip)]; ok && ref.actorID != "" {
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

func (r topologyL3ActorResolver) addUniqueIPAddress(ip string, ref topologyL3ActorRef) {
	ip = topologyutil.NormalizeIPAddress(ip)
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
	routerID = topologyutil.NormalizeTopologyRouterID(routerID)
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

func topologyL3ActorRouterIDs(actor topologymodel.Actor) []string {
	values := make([]string, 0, 2)
	if routerID := topologymodel.ActorDetailOSPFRouterID(actor); routerID != "" {
		values = append(values, routerID)
	}
	return values
}
