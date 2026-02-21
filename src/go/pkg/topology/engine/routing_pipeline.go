// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"encoding/hex"
	"net/netip"
	"sort"
	"strconv"
	"strings"
)

// OSPFNeighborObservation mirrors the OSPF neighbor row fields used by Enlinkd.
type OSPFNeighborObservation struct {
	RemoteRouterID       string
	RemoteIP             string
	RemoteAddressLessIdx int
}

// OSPFInterfaceObservation mirrors the OSPF interface row fields used by Enlinkd.
type OSPFInterfaceObservation struct {
	IP             string
	AddressLessIdx int
	AreaID         string
	IfIndex        int
	IfName         string
	Netmask        string
}

// OSPFLinkObservation is the normalized OSPF link used for topology matching.
type OSPFLinkObservation struct {
	ID           string
	DeviceID     string
	LocalIP      string
	LocalMask    string
	RemoteIP     string
	RemoteRouter string
	IfIndex      int
	IfName       string
	AreaID       string
}

// OSPFLinkPair is a matched bidirectional OSPF pair.
type OSPFLinkPair struct {
	Source OSPFLinkObservation
	Target OSPFLinkObservation
}

// BuildOSPFLinksEnlinkd applies Enlinkd's NodeDiscoveryOspf link-local association
// rules to neighbor + interface observations from one device.
func BuildOSPFLinksEnlinkd(deviceID string, neighbors []OSPFNeighborObservation, interfaces []OSPFInterfaceObservation) []OSPFLinkObservation {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" || len(neighbors) == 0 {
		return nil
	}

	out := make([]OSPFLinkObservation, 0, len(neighbors))
	for i, nbr := range neighbors {
		link := OSPFLinkObservation{
			ID:           deviceID + ":ospf:" + strconv.Itoa(i+1),
			DeviceID:     deviceID,
			RemoteIP:     strings.TrimSpace(nbr.RemoteIP),
			RemoteRouter: strings.TrimSpace(nbr.RemoteRouterID),
		}

		for _, local := range interfaces {
			// Enlinkd address-less fast path: if both sides are address-less, bind on first local address-less interface.
			if local.AddressLessIdx != 0 && nbr.RemoteAddressLessIdx != 0 {
				link.LocalIP = strings.TrimSpace(local.IP)
				link.IfIndex = local.AddressLessIdx
				link.IfName = strings.TrimSpace(local.IfName)
				link.AreaID = strings.TrimSpace(local.AreaID)
				link.LocalMask = strings.TrimSpace(local.Netmask)
				break
			}
			if local.AddressLessIdx == 0 && nbr.RemoteAddressLessIdx != 0 {
				continue
			}
			if local.AddressLessIdx != 0 && nbr.RemoteAddressLessIdx == 0 {
				continue
			}

			if inSameNetwork(local.IP, nbr.RemoteIP, local.Netmask) {
				link.LocalIP = strings.TrimSpace(local.IP)
				link.IfIndex = local.IfIndex
				link.IfName = strings.TrimSpace(local.IfName)
				link.AreaID = strings.TrimSpace(local.AreaID)
				link.LocalMask = strings.TrimSpace(local.Netmask)
				break
			}
		}

		out = append(out, link)
	}

	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.DeviceID != b.DeviceID {
			return a.DeviceID < b.DeviceID
		}
		if a.LocalIP != b.LocalIP {
			return a.LocalIP < b.LocalIP
		}
		if a.RemoteIP != b.RemoteIP {
			return a.RemoteIP < b.RemoteIP
		}
		if a.IfIndex != b.IfIndex {
			return a.IfIndex < b.IfIndex
		}
		return a.ID < b.ID
	})
	return out
}

// MatchOSPFLinksEnlinkd ports OspfTopologyServiceImpl.match(): two links match when
// their local/remote IP pair is reversed.
func MatchOSPFLinksEnlinkd(links []OSPFLinkObservation) []OSPFLinkPair {
	if len(links) == 0 {
		return nil
	}

	targetByKey := make(map[string]int, len(links))
	for i, link := range links {
		key := ospfPairKey(link.LocalIP, link.RemoteIP)
		if key == "" {
			continue
		}
		targetByKey[key] = i
	}

	parsed := make(map[int]struct{}, len(links))
	out := make([]OSPFLinkPair, 0, len(links)/2)
	for i, source := range links {
		if _, ok := parsed[i]; ok {
			continue
		}
		parsed[i] = struct{}{}

		targetIdx, ok := targetByKey[ospfPairKey(source.RemoteIP, source.LocalIP)]
		if !ok || targetIdx == i {
			continue
		}
		if _, done := parsed[targetIdx]; done {
			continue
		}

		target := links[targetIdx]
		parsed[targetIdx] = struct{}{}
		out = append(out, OSPFLinkPair{Source: source, Target: target})
	}

	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Source.DeviceID != b.Source.DeviceID {
			return a.Source.DeviceID < b.Source.DeviceID
		}
		if a.Target.DeviceID != b.Target.DeviceID {
			return a.Target.DeviceID < b.Target.DeviceID
		}
		if a.Source.LocalIP != b.Source.LocalIP {
			return a.Source.LocalIP < b.Source.LocalIP
		}
		if a.Target.LocalIP != b.Target.LocalIP {
			return a.Target.LocalIP < b.Target.LocalIP
		}
		return a.Source.ID < b.Source.ID
	})
	return out
}

// ISISElementObservation mirrors the IS-IS element row fields used by Enlinkd.
type ISISElementObservation struct {
	DeviceID   string
	SysID      string
	AdminState int
}

// ISISLinkObservation mirrors the IS-IS adjacency row fields used by Enlinkd.
type ISISLinkObservation struct {
	ID             string
	DeviceID       string
	AdjIndex       int
	CircIfIndex    int
	NeighborSysID  string
	NeighborSNPA   string
	NeighborSysTyp string
}

// ISISLinkPair is a matched bidirectional IS-IS pair.
type ISISLinkPair struct {
	Source ISISLinkObservation
	Target ISISLinkObservation
}

// MatchISISLinksEnlinkd ports IsisTopologyServiceImpl.match().
func MatchISISLinksEnlinkd(elements []ISISElementObservation, links []ISISLinkObservation) []ISISLinkPair {
	if len(elements) == 0 || len(links) == 0 {
		return nil
	}

	sysIDByDevice := make(map[string]string, len(elements))
	for _, element := range elements {
		deviceID := strings.TrimSpace(element.DeviceID)
		sysID := normalizeHexToken(element.SysID)
		if deviceID == "" || sysID == "" {
			continue
		}
		sysIDByDevice[deviceID] = sysID
	}

	targetByKey := make(map[string]int, len(links))
	for i, link := range links {
		sourceSysID := sysIDByDevice[strings.TrimSpace(link.DeviceID)]
		if sourceSysID == "" {
			continue
		}
		key := isisPairKey(link.AdjIndex, sourceSysID, link.NeighborSysID)
		if key == "" {
			continue
		}
		targetByKey[key] = i
	}

	parsed := make(map[int]struct{}, len(links))
	out := make([]ISISLinkPair, 0, len(links)/2)
	for i, source := range links {
		if _, ok := parsed[i]; ok {
			continue
		}
		sourceSysID := sysIDByDevice[strings.TrimSpace(source.DeviceID)]
		if sourceSysID == "" {
			continue
		}

		key := isisPairKey(source.AdjIndex, source.NeighborSysID, sourceSysID)
		targetIdx, ok := targetByKey[key]
		if !ok || targetIdx == i {
			continue
		}
		if _, done := parsed[targetIdx]; done {
			continue
		}

		target := links[targetIdx]
		out = append(out, ISISLinkPair{Source: source, Target: target})
		parsed[i] = struct{}{}
		parsed[targetIdx] = struct{}{}
	}

	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Source.DeviceID != b.Source.DeviceID {
			return a.Source.DeviceID < b.Source.DeviceID
		}
		if a.Target.DeviceID != b.Target.DeviceID {
			return a.Target.DeviceID < b.Target.DeviceID
		}
		if a.Source.AdjIndex != b.Source.AdjIndex {
			return a.Source.AdjIndex < b.Source.AdjIndex
		}
		return a.Source.ID < b.Source.ID
	})
	return out
}

func ospfPairKey(localIP, remoteIP string) string {
	local := canonicalRoutingIP(localIP)
	remote := canonicalRoutingIP(remoteIP)
	if local == "" || remote == "" {
		return ""
	}
	return local + "|" + remote
}

func isisPairKey(adjIndex int, sourceSysID, neighSysID string) string {
	if adjIndex <= 0 {
		return ""
	}
	source := normalizeHexToken(sourceSysID)
	neigh := normalizeHexToken(neighSysID)
	if source == "" || neigh == "" {
		return ""
	}
	return strconv.Itoa(adjIndex) + "|" + source + "|" + neigh
}

func canonicalRoutingIP(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil || !addr.IsValid() {
		return ""
	}
	return addr.Unmap().String()
}

func normalizeHexToken(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if asIP := decodeHexIP(v); asIP != "" {
		return asIP
	}

	clean := strings.NewReplacer(":", "", "-", "", ".", "", " ", "", "\t", "").Replace(strings.ToLower(v))
	if clean == "" {
		return ""
	}
	if len(clean)%2 == 1 {
		clean = "0" + clean
	}
	if _, err := hex.DecodeString(clean); err == nil {
		return clean
	}
	return v
}

func inSameNetwork(localIP, remoteIP, mask string) bool {
	local, err := netip.ParseAddr(strings.TrimSpace(localIP))
	if err != nil {
		return false
	}
	remote, err := netip.ParseAddr(strings.TrimSpace(remoteIP))
	if err != nil {
		return false
	}
	netmask, err := netip.ParseAddr(strings.TrimSpace(mask))
	if err != nil {
		return false
	}
	return InSameNetwork(local.Unmap(), remote.Unmap(), netmask.Unmap())
}
