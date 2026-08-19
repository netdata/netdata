// SPDX-License-Identifier: GPL-3.0-or-later

package topologyenrich

import (
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

type topologyL3ActorRef struct {
	actorHandle   topologymodel.ActorHandle
	actorID       string
	actorOrder    int
	endpointMatch topologymodel.Match
}

func (r topologyL3ActorRef) valid() bool {
	return !r.actorHandle.IsZero()
}

type topologyL3ActorResolver struct {
	byActorID  map[string]topologyL3ActorRef
	byDeviceID map[string]topologyL3ActorRef
	byIP       map[string]topologyL3ActorRef
	byRouterID map[string]topologyL3ActorRef
}

type topologyL3ActorResolverProvider struct {
	data        *topologymodel.Data
	snapshots   []topologymodel.ObservationSnapshot
	resolver    topologyL3ActorResolver
	initialized bool
}

func newTopologyL3ActorResolverProvider(
	data *topologymodel.Data,
	snapshots []topologymodel.ObservationSnapshot,
) *topologyL3ActorResolverProvider {
	return &topologyL3ActorResolverProvider{data: data, snapshots: snapshots}
}

func (p *topologyL3ActorResolverProvider) resolve() topologyL3ActorResolver {
	if !p.initialized {
		p.resolver = newTopologyL3ActorResolver(p.data, p.snapshots)
		p.initialized = true
	}
	return p.resolver
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

	managedActors := make([]topologyL3ActorRef, 0, len(data.Actors))
	actorOrder := topologyL3ActorLexicalOrder(data.Actors)
	localMatchIndex := topologymodel.NewLocalActorMatchIndex()
	for _, actor := range data.Actors {
		if !topologymodel.IsManagedSNMPDeviceActor(actor) {
			continue
		}
		ref := topologyL3ActorRef{
			actorHandle:   actor.ActorHandle,
			actorID:       strings.TrimSpace(actor.ActorID),
			actorOrder:    actorOrder[actor.ActorHandle],
			endpointMatch: graph.LinkEndpointMatch(actor.Match, topologymodel.ActorDetailManagementIP(actor)),
		}
		managedActors = append(managedActors, ref)
		localMatchIndex.AddMatch(len(managedActors)-1, actor.Match)
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

	matches := make([]int, 0, 1)
	for _, snapshot := range snapshots {
		deviceID := strings.TrimSpace(snapshot.LocalDeviceID)
		if deviceID == "" {
			continue
		}
		matches = localMatchIndex.MatchIndexes(matches[:0], snapshot.LocalDevice)
		for _, actorIndex := range matches {
			ref := managedActors[actorIndex]
			resolver.addUniqueDeviceID(deviceID, ref)
			resolver.addUniqueRouterID(snapshot.LocalDevice.OSPFRouterID, ref)
		}
	}

	return resolver
}

func (r topologyL3ActorResolver) resolve(row topologymodel.L3Interface) (topologyL3ActorRef, bool) {
	if ref, ok := r.byDeviceID[strings.TrimSpace(row.DeviceID)]; ok && ref.valid() {
		return ref, true
	}
	if ref, ok := r.byActorID[strings.TrimSpace(row.DeviceID)]; ok && ref.valid() {
		return ref, true
	}
	return r.resolveIPAddress(row.IP)
}

func (r topologyL3ActorResolver) resolveDeviceID(deviceID string) (topologyL3ActorRef, bool) {
	if ref, ok := r.byDeviceID[strings.TrimSpace(deviceID)]; ok && ref.valid() {
		return ref, true
	}
	if ref, ok := r.byActorID[strings.TrimSpace(deviceID)]; ok && ref.valid() {
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
	if ref, ok := r.byRouterID[topologyutil.NormalizeTopologyRouterID(routerID)]; ok && ref.valid() {
		return ref, true
	}
	return topologyL3ActorRef{}, false
}

func (r topologyL3ActorResolver) resolveIPAddress(ip string) (topologyL3ActorRef, bool) {
	if ref, ok := r.byIP[topologyutil.NormalizeIPAddress(ip)]; ok && ref.valid() {
		return ref, true
	}
	return topologyL3ActorRef{}, false
}

func (r topologyL3ActorResolver) resolveNonUnspecifiedIPAddress(ip string) (topologyL3ActorRef, bool) {
	if ref, ok := r.byIP[topologyutil.NormalizeNonUnspecifiedIPAddress(ip)]; ok && ref.valid() {
		return ref, true
	}
	return topologyL3ActorRef{}, false
}

func (r topologyL3ActorResolver) addUniqueDeviceID(deviceID string, ref topologyL3ActorRef) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" || !ref.valid() {
		return
	}
	existing, ok := r.byDeviceID[deviceID]
	if !ok {
		r.byDeviceID[deviceID] = ref
		return
	}
	if existing.valid() && existing.actorHandle != ref.actorHandle {
		r.byDeviceID[deviceID] = topologyL3ActorRef{}
	}
}

func (r topologyL3ActorResolver) addUniqueIPAddress(ip string, ref topologyL3ActorRef) {
	ip = topologyutil.NormalizeIPAddress(ip)
	if ip == "" || !ref.valid() {
		return
	}
	existing, ok := r.byIP[ip]
	if !ok {
		r.byIP[ip] = ref
		return
	}
	if existing.valid() && existing.actorHandle != ref.actorHandle {
		r.byIP[ip] = topologyL3ActorRef{}
	}
}

func (r topologyL3ActorResolver) addUniqueRouterID(routerID string, ref topologyL3ActorRef) {
	routerID = topologyutil.NormalizeTopologyRouterID(routerID)
	if routerID == "" || !ref.valid() {
		return
	}
	existing, ok := r.byRouterID[routerID]
	if !ok {
		r.byRouterID[routerID] = ref
		return
	}
	if existing.valid() && existing.actorHandle != ref.actorHandle {
		r.byRouterID[routerID] = topologyL3ActorRef{}
	}
}

func topologyL3ActorLexicalOrder(actors []topologymodel.Actor) map[topologymodel.ActorHandle]int {
	type entry struct {
		handle  topologymodel.ActorHandle
		actorID string
	}
	entries := make([]entry, 0, len(actors))
	for _, actor := range actors {
		if actor.ActorHandle.IsZero() {
			continue
		}
		entries = append(entries, entry{handle: actor.ActorHandle, actorID: strings.TrimSpace(actor.ActorID)})
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].actorID < entries[j].actorID })
	order := make(map[topologymodel.ActorHandle]int, len(entries))
	for i, entry := range entries {
		order[entry.handle] = i
	}
	return order
}

func topologyL3ActorRouterIDs(actor topologymodel.Actor) []string {
	values := make([]string, 0, 2)
	if routerID := topologymodel.ActorDetailOSPFRouterID(actor); routerID != "" {
		values = append(values, routerID)
	}
	return values
}
