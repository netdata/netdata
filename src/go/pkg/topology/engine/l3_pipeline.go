// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"
)

// BuildL3ResultFromObservations converts normalized L3 observations into an
// engine Result using Enlinkd-style OSPF/ISIS pairing logic.
func BuildL3ResultFromObservations(observations []L3Observation) (Result, error) {
	if len(observations) == 0 {
		return Result{}, errors.New("topology engine: empty l3 observations")
	}

	deviceSeen := make(map[string]struct{}, len(observations))
	devices := make([]Device, 0, len(observations))
	ifaces := make([]Interface, 0, len(observations))
	ifaceSeen := make(map[string]struct{}, len(observations)*2)

	allOSPFLInks := make([]OSPFLinkObservation, 0)
	allISISLinks := make([]ISISLinkObservation, 0)
	isisElements := make([]ISISElementObservation, 0, len(observations))

	for _, obs := range observations {
		deviceID := strings.TrimSpace(obs.DeviceID)
		if deviceID == "" {
			continue
		}

		device, hasL3Data := l3ObservationToDevice(obs)
		if hasL3Data {
			if _, ok := deviceSeen[device.ID]; !ok {
				deviceSeen[device.ID] = struct{}{}
				devices = append(devices, device)
			}
		}

		if len(obs.Interfaces) > 0 {
			for _, observed := range obs.Interfaces {
				if observed.IfIndex <= 0 {
					continue
				}
				key := deviceIfIndexKey(deviceID, observed.IfIndex)
				if _, ok := ifaceSeen[key]; ok {
					continue
				}
				ifaceSeen[key] = struct{}{}
				ifaces = append(ifaces, Interface{
					DeviceID: deviceID,
					IfIndex:  observed.IfIndex,
					IfName:   strings.TrimSpace(observed.IfName),
					IfDescr:  strings.TrimSpace(observed.IfDescr),
				})
			}
		}

		ospfLinks := BuildOSPFLinksEnlinkd(deviceID, obs.OSPFNbrTable, obs.OSPFIfTable)
		if len(ospfLinks) > 0 {
			allOSPFLInks = append(allOSPFLInks, ospfLinks...)
		}

		if obs.ISISElement != nil {
			isisElements = append(isisElements, ISISElementObservation{
				DeviceID: deviceID,
				SysID:    strings.TrimSpace(obs.ISISElement.SysID),
			})
		}
		isisLinks := buildISISLinksEnlinkd(deviceID, obs.ISISCircTable, obs.ISISAdjTable)
		if len(isisLinks) > 0 {
			allISISLinks = append(allISISLinks, isisLinks...)
		}
	}

	adjacencies := make([]Adjacency, 0, len(allOSPFLInks)+len(allISISLinks))
	adjacencies = append(adjacencies, buildOSPFAdjacencies(MatchOSPFLinksEnlinkd(allOSPFLInks))...)
	adjacencies = append(adjacencies, buildISISAdjacencies(MatchISISLinksEnlinkd(isisElements, allISISLinks))...)
	sort.Slice(adjacencies, func(i, j int) bool {
		left, right := adjacencies[i], adjacencies[j]
		if left.Protocol != right.Protocol {
			return left.Protocol < right.Protocol
		}
		if left.SourceID != right.SourceID {
			return left.SourceID < right.SourceID
		}
		if left.TargetID != right.TargetID {
			return left.TargetID < right.TargetID
		}
		if left.SourcePort != right.SourcePort {
			return left.SourcePort < right.SourcePort
		}
		return left.TargetPort < right.TargetPort
	})

	sort.Slice(devices, func(i, j int) bool {
		return devices[i].ID < devices[j].ID
	})
	sort.Slice(ifaces, func(i, j int) bool {
		left, right := ifaces[i], ifaces[j]
		if left.DeviceID != right.DeviceID {
			return left.DeviceID < right.DeviceID
		}
		return left.IfIndex < right.IfIndex
	})

	return Result{
		CollectedAt: time.Now().UTC(),
		Devices:     devices,
		Interfaces:  ifaces,
		Adjacencies: adjacencies,
		Stats: map[string]any{
			"devices_total": len(devices),
			"links_total":   len(adjacencies),
		},
	}, nil
}

func l3ObservationToDevice(obs L3Observation) (Device, bool) {
	deviceID := strings.TrimSpace(obs.DeviceID)
	if deviceID == "" {
		return Device{}, false
	}

	hasL3Data := obs.OSPFElement != nil || len(obs.OSPFNbrTable) > 0 || len(obs.OSPFIfTable) > 0 || obs.ISISElement != nil || len(obs.ISISAdjTable) > 0 || len(obs.ISISCircTable) > 0
	if !hasL3Data {
		return Device{}, false
	}

	device := Device{
		ID:        deviceID,
		Hostname:  strings.TrimSpace(obs.Hostname),
		SysObject: strings.TrimSpace(obs.SysObjectID),
		ChassisID: strings.TrimSpace(obs.ChassisID),
	}
	if ip := canonicalRoutingIP(obs.ManagementIP); ip != "" {
		device.Addresses = append(device.Addresses, parseAddr(ip))
	}

	labels := make(map[string]string)
	if obs.OSPFElement != nil {
		if routerID := canonicalRoutingIP(obs.OSPFElement.RouterID); routerID != "" {
			labels["ospf_router_id"] = routerID
		}
		labels["ospf_admin_state"] = strconv.Itoa(obs.OSPFElement.AdminState)
		labels["ospf_version"] = strconv.Itoa(obs.OSPFElement.VersionNumber)
	}
	if obs.ISISElement != nil {
		if sysID := normalizeHexToken(obs.ISISElement.SysID); sysID != "" {
			labels["isis_sys_id"] = sysID
		}
		labels["isis_admin_state"] = strconv.Itoa(obs.ISISElement.AdminState)
	}
	if len(labels) > 0 {
		device.Labels = labels
	}
	return device, true
}

func buildISISLinksEnlinkd(deviceID string, circuits []ISISCircuitObservation, adjs []ISISAdjacencyObservation) []ISISLinkObservation {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" || len(adjs) == 0 {
		return nil
	}

	circuitByIndex := make(map[int]ISISCircuitObservation, len(circuits))
	for _, circ := range circuits {
		if circ.CircIndex <= 0 {
			continue
		}
		circuitByIndex[circ.CircIndex] = circ
	}

	out := make([]ISISLinkObservation, 0, len(adjs))
	for _, adj := range adjs {
		if adj.AdjIndex <= 0 {
			continue
		}

		circ := circuitByIndex[adj.CircIndex]
		link := ISISLinkObservation{
			ID:             buildISISLinkID(deviceID, adj.CircIndex, adj.AdjIndex),
			DeviceID:       deviceID,
			AdjIndex:       adj.AdjIndex,
			CircIfIndex:    circ.IfIndex,
			NeighborSysID:  strings.TrimSpace(adj.NeighborSysID),
			NeighborSNPA:   strings.TrimSpace(adj.NeighborSNPA),
			NeighborSysTyp: strconv.Itoa(adj.NeighborSysType),
		}
		out = append(out, link)
	}

	sort.Slice(out, func(i, j int) bool {
		left, right := out[i], out[j]
		if left.DeviceID != right.DeviceID {
			return left.DeviceID < right.DeviceID
		}
		if left.AdjIndex != right.AdjIndex {
			return left.AdjIndex < right.AdjIndex
		}
		return left.ID < right.ID
	})
	return out
}

func buildISISLinkID(deviceID string, circIndex, adjIndex int) string {
	parts := []string{strings.TrimSpace(deviceID), "isis"}
	if circIndex > 0 {
		parts = append(parts, strconv.Itoa(circIndex))
	}
	parts = append(parts, strconv.Itoa(adjIndex))
	return strings.Join(parts, ":")
}

func buildOSPFAdjacencies(pairs []OSPFLinkPair) []Adjacency {
	if len(pairs) == 0 {
		return nil
	}

	out := make([]Adjacency, 0, len(pairs)*2)
	for _, pair := range pairs {
		pairID := canonicalPairID("ospf", pair.Source.ID, pair.Target.ID)
		sourcePort := pair.Source.IfName
		if strings.TrimSpace(sourcePort) == "" && pair.Source.IfIndex > 0 {
			sourcePort = strconv.Itoa(pair.Source.IfIndex)
		}
		targetPort := pair.Target.IfName
		if strings.TrimSpace(targetPort) == "" && pair.Target.IfIndex > 0 {
			targetPort = strconv.Itoa(pair.Target.IfIndex)
		}

		sourceLabels := map[string]string{
			adjacencyLabelPairID:   pairID,
			adjacencyLabelPairSide: adjacencyPairSideSource,
			adjacencyLabelPairPass: "ospf:reverse-ip",
			"ospf_ip_addr":         canonicalRoutingIP(pair.Source.LocalIP),
			"ospf_rem_ip_addr":     canonicalRoutingIP(pair.Source.RemoteIP),
			"ospf_rem_router_id":   canonicalRoutingIP(pair.Source.RemoteRouter),
			"ospf_if_index":        strconv.Itoa(pair.Source.IfIndex),
			"ospf_if_name":         strings.TrimSpace(pair.Source.IfName),
			"ospf_if_area_id":      canonicalRoutingIP(pair.Source.AreaID),
		}
		targetLabels := map[string]string{
			adjacencyLabelPairID:   pairID,
			adjacencyLabelPairSide: adjacencyPairSideTarget,
			adjacencyLabelPairPass: "ospf:reverse-ip",
			"ospf_ip_addr":         canonicalRoutingIP(pair.Target.LocalIP),
			"ospf_rem_ip_addr":     canonicalRoutingIP(pair.Target.RemoteIP),
			"ospf_rem_router_id":   canonicalRoutingIP(pair.Target.RemoteRouter),
			"ospf_if_index":        strconv.Itoa(pair.Target.IfIndex),
			"ospf_if_name":         strings.TrimSpace(pair.Target.IfName),
			"ospf_if_area_id":      canonicalRoutingIP(pair.Target.AreaID),
		}
		out = append(out, Adjacency{
			Protocol:   "ospf",
			SourceID:   strings.TrimSpace(pair.Source.DeviceID),
			SourcePort: strings.TrimSpace(sourcePort),
			TargetID:   strings.TrimSpace(pair.Target.DeviceID),
			TargetPort: strings.TrimSpace(targetPort),
			Labels:     compactLabels(sourceLabels),
		})
		out = append(out, Adjacency{
			Protocol:   "ospf",
			SourceID:   strings.TrimSpace(pair.Target.DeviceID),
			SourcePort: strings.TrimSpace(targetPort),
			TargetID:   strings.TrimSpace(pair.Source.DeviceID),
			TargetPort: strings.TrimSpace(sourcePort),
			Labels:     compactLabels(targetLabels),
		})
	}
	return out
}

func buildISISAdjacencies(pairs []ISISLinkPair) []Adjacency {
	if len(pairs) == 0 {
		return nil
	}

	out := make([]Adjacency, 0, len(pairs)*2)
	for _, pair := range pairs {
		pairID := canonicalPairID("isis", pair.Source.ID, pair.Target.ID)
		sourcePort := ""
		if pair.Source.CircIfIndex > 0 {
			sourcePort = strconv.Itoa(pair.Source.CircIfIndex)
		}
		targetPort := ""
		if pair.Target.CircIfIndex > 0 {
			targetPort = strconv.Itoa(pair.Target.CircIfIndex)
		}

		sourceLabels := map[string]string{
			adjacencyLabelPairID:   pairID,
			adjacencyLabelPairSide: adjacencyPairSideSource,
			adjacencyLabelPairPass: "isis:adjindex-sysid",
			"isis_adj_index":       strconv.Itoa(pair.Source.AdjIndex),
			"isis_neigh_sys_id":    normalizeHexToken(pair.Source.NeighborSysID),
			"isis_neigh_snpa":      strings.TrimSpace(pair.Source.NeighborSNPA),
			"isis_neigh_sys_type":  strings.TrimSpace(pair.Source.NeighborSysTyp),
			"isis_if_index":        strconv.Itoa(pair.Source.CircIfIndex),
		}
		targetLabels := map[string]string{
			adjacencyLabelPairID:   pairID,
			adjacencyLabelPairSide: adjacencyPairSideTarget,
			adjacencyLabelPairPass: "isis:adjindex-sysid",
			"isis_adj_index":       strconv.Itoa(pair.Target.AdjIndex),
			"isis_neigh_sys_id":    normalizeHexToken(pair.Target.NeighborSysID),
			"isis_neigh_snpa":      strings.TrimSpace(pair.Target.NeighborSNPA),
			"isis_neigh_sys_type":  strings.TrimSpace(pair.Target.NeighborSysTyp),
			"isis_if_index":        strconv.Itoa(pair.Target.CircIfIndex),
		}
		out = append(out, Adjacency{
			Protocol:   "isis",
			SourceID:   strings.TrimSpace(pair.Source.DeviceID),
			SourcePort: sourcePort,
			TargetID:   strings.TrimSpace(pair.Target.DeviceID),
			TargetPort: targetPort,
			Labels:     compactLabels(sourceLabels),
		})
		out = append(out, Adjacency{
			Protocol:   "isis",
			SourceID:   strings.TrimSpace(pair.Target.DeviceID),
			SourcePort: targetPort,
			TargetID:   strings.TrimSpace(pair.Source.DeviceID),
			TargetPort: sourcePort,
			Labels:     compactLabels(targetLabels),
		})
	}
	return out
}

func canonicalPairID(protocol, leftID, rightID string) string {
	a := strings.TrimSpace(leftID)
	b := strings.TrimSpace(rightID)
	if a > b {
		a, b = b, a
	}
	return strings.TrimSpace(protocol) + ":" + a + "|" + b
}

func compactLabels(input map[string]string) map[string]string {
	out := make(map[string]string, len(input))
	for key, value := range input {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
