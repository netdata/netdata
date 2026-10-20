// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"net/netip"
	"sort"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
)

type builtEndpointActors struct {
	actors                []projectedActor
	count                 int
	matchByEndpointID     map[string]graph.Match
	linkMatchByEndpointID map[string]graph.Match
	labelsByEndpointID    map[string]map[string]string
}

func buildEndpointActors(
	attachments []model.Attachment,
	enrichments []model.Enrichment,
	ifaceByDeviceIndex map[string]model.Interface,
	source string,
	layer string,
	actorIndex map[string]struct{},
	actorMACIndex map[string]struct{},
) builtEndpointActors {
	accumulators := make(map[string]*endpointActorAccumulator)

	for _, attachment := range attachments {
		endpointID := strings.TrimSpace(attachment.EndpointID)
		if endpointID == "" {
			continue
		}
		acc := ensureEndpointActorAccumulator(accumulators, endpointID)
		addEndpointIDIdentity(acc, endpointID)
		if deviceID := strings.TrimSpace(attachment.DeviceID); deviceID != "" {
			acc.deviceIDs[deviceID] = struct{}{}
		}
		if method := strings.TrimSpace(attachment.Method); method != "" {
			acc.sources[strings.ToLower(method)] = struct{}{}
		}
		if attachment.IfIndex > 0 {
			acc.ifIndexes[strconv.Itoa(attachment.IfIndex)] = struct{}{}
			iface, ok := ifaceByDeviceIndex[deviceIfIndexKey(strings.TrimSpace(attachment.DeviceID), attachment.IfIndex)]
			if ok {
				if ifName := strings.TrimSpace(iface.IfName); ifName != "" {
					acc.ifNames[ifName] = struct{}{}
				}
			}
		}
		if ifName := strings.TrimSpace(attachment.Labels["if_name"]); ifName != "" {
			acc.ifNames[ifName] = struct{}{}
		}
	}

	for _, enrichment := range enrichments {
		endpointID := strings.TrimSpace(enrichment.EndpointID)
		if endpointID == "" {
			continue
		}
		acc := ensureEndpointActorAccumulator(accumulators, endpointID)
		addEndpointIDIdentity(acc, endpointID)

		if mac := normalizeMAC(enrichment.MAC); mac != "" {
			acc.mac = mac
		}
		for _, ip := range enrichment.IPs {
			if ip.IsValid() {
				acc.ips[ip.String()] = ip.Unmap()
			}
		}
		for _, sourceName := range csvToSet(enrichment.Labels["sources"]) {
			acc.sources[sourceName] = struct{}{}
		}
		for _, deviceID := range csvToSet(enrichment.Labels["device_ids"]) {
			deviceID = strings.TrimSpace(deviceID)
			if deviceID == "" {
				continue
			}
			acc.deviceIDs[deviceID] = struct{}{}
		}
		for _, ifIndex := range csvToSet(enrichment.Labels["if_indexes"]) {
			acc.ifIndexes[ifIndex] = struct{}{}
		}
		for _, ifName := range csvToSet(enrichment.Labels["if_names"]) {
			acc.ifNames[ifName] = struct{}{}
		}
	}

	if len(accumulators) == 0 {
		return builtEndpointActors{
			matchByEndpointID:     map[string]graph.Match{},
			linkMatchByEndpointID: map[string]graph.Match{},
			labelsByEndpointID:    map[string]map[string]string{},
		}
	}

	keys := make([]string, 0, len(accumulators))
	for endpointID := range accumulators {
		keys = append(keys, endpointID)
	}
	sort.Strings(keys)

	actors := make([]projectedActor, 0, len(keys))
	endpointCount := 0
	matchByEndpointID := make(map[string]graph.Match, len(keys))
	linkMatchByEndpointID := make(map[string]graph.Match, len(keys))
	labelsByEndpointID := make(map[string]map[string]string, len(keys))
	for _, endpointID := range keys {
		acc := accumulators[endpointID]
		if acc == nil {
			continue
		}

		match := graph.Match{}
		if acc.mac != "" {
			match.ChassisIDs = []string{acc.mac}
			match.MacAddresses = []string{acc.mac}
		}
		match.IPAddresses = sortedEndpointIPs(acc.ips)
		matchByEndpointID[endpointID] = match
		linkMatchByEndpointID[endpointID] = graph.LinkEndpointMatch(match, "")
		labelsByEndpointID[endpointID] = map[string]string{
			"learned_sources":    strings.Join(sortedTopologySet(acc.sources), ","),
			"learned_device_ids": strings.Join(sortedTopologySet(acc.deviceIDs), ","),
			"learned_if_indexes": strings.Join(sortedTopologySet(acc.ifIndexes), ","),
			"learned_if_names":   strings.Join(sortedTopologySet(acc.ifNames), ","),
		}

		detail := model.ProjectionEndpointActorDetail{
			Discovered:       true,
			LearnedSources:   sortedTopologySet(acc.sources),
			LearnedDeviceIDs: sortedTopologySet(acc.deviceIDs),
			LearnedIfIndexes: sortedTopologySet(acc.ifIndexes),
			LearnedIfNames:   sortedTopologySet(acc.ifNames),
		}
		derivedVendor, derivedPrefix := inferTopologyVendorFromMatch(match)
		if derivedVendor != "" {
			detail.Vendor = derivedVendor
			detail.VendorSource = "mac_oui"
			detail.VendorConfidence = "low"
			detail.VendorMatchPrefix = derivedPrefix
			detail.VendorDerived = derivedVendor
			detail.VendorDerivedSource = "mac_oui"
			detail.VendorDerivedConfidence = "low"
			detail.VendorDerivedMatchPrefix = derivedPrefix
		}
		actor := projectedActor{
			Actor: graph.Actor{
				ActorType: "endpoint",
				Layer:     layer,
				Source:    source,
				Match:     match,
			},
			Detail: model.ProjectionActorDetail{
				Endpoint: detail,
			},
		}

		keys := topologyMatchIdentityKeys(actor.Actor.Match)
		if len(keys) == 0 {
			continue
		}
		macKeys := topologyMatchHardwareIdentityKeys(actor.Actor.Match)
		if len(macKeys) > 0 {
			if topologyIdentityIndexOverlaps(actorMACIndex, macKeys) {
				continue
			}
			addTopologyIdentityKeys(actorMACIndex, macKeys)
		} else if topologyIdentityIndexOverlaps(actorIndex, keys) {
			continue
		}
		addTopologyIdentityKeys(actorIndex, keys)

		actors = append(actors, actor)
		endpointCount++
	}

	return builtEndpointActors{
		actors:                actors,
		count:                 endpointCount,
		matchByEndpointID:     matchByEndpointID,
		linkMatchByEndpointID: linkMatchByEndpointID,
		labelsByEndpointID:    labelsByEndpointID,
	}
}

func ensureEndpointActorAccumulator(accumulators map[string]*endpointActorAccumulator, endpointID string) *endpointActorAccumulator {
	acc := accumulators[endpointID]
	if acc != nil {
		return acc
	}
	acc = &endpointActorAccumulator{
		endpointID: endpointID,
		ips:        make(map[string]netip.Addr),
		sources:    make(map[string]struct{}),
		deviceIDs:  make(map[string]struct{}),
		ifIndexes:  make(map[string]struct{}),
		ifNames:    make(map[string]struct{}),
	}
	accumulators[endpointID] = acc
	return acc
}

func addEndpointIDIdentity(acc *endpointActorAccumulator, endpointID string) {
	if acc == nil {
		return
	}
	kind, value, ok := strings.Cut(strings.TrimSpace(endpointID), ":")
	if !ok {
		return
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "mac":
		if mac := normalizeMAC(value); mac != "" {
			acc.mac = mac
		}
	case "ip":
		if addr := parseAddr(value); addr.IsValid() {
			acc.ips[addr.String()] = addr.Unmap()
		}
	}
}

func discoveredDeviceCount(devices []model.Device, localDeviceID string) int {
	localDeviceID = strings.TrimSpace(localDeviceID)
	count := 0
	for _, dev := range devices {
		deviceID := strings.TrimSpace(dev.ID)
		if deviceID == "" {
			continue
		}
		if deviceID == localDeviceID {
			continue
		}
		count++
	}
	return count
}
