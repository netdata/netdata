// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import "strings"

const (
	lldpMatchPassDefault      = "default"
	lldpMatchPassPortDesc     = "port_description"
	lldpMatchPassSysName      = "sysname"
	lldpMatchPassChassisPort  = "chassis_port_id_subtype"
	lldpMatchPassChassisDescr = "chassis_port_descr"
	lldpMatchPassChassis      = "chassis"
)

type lldpMatchLink struct {
	index int

	sourceDeviceID string
	localChassisID string
	localSysName   string
	localMatchID   string

	localPortID        string
	localPortIDSubtype string
	localPortDescr     string

	remoteChassisID     string
	remoteSysName       string
	remoteMatchID       string
	remotePortID        string
	remotePortIDSubtype string
	remotePortDescr     string

	sourcePort       string
	targetPort       string
	remoteManagement string
	remoteFallbackID string
}

type lldpMatchedPair struct {
	sourceIndex int
	targetIndex int
	pass        string
}

func buildLLDPMatchLinks(observations []L2Observation) []lldpMatchLink {
	links := make([]lldpMatchLink, 0)
	for _, obs := range observations {
		sourceID := strings.TrimSpace(obs.DeviceID)
		if sourceID == "" {
			continue
		}
		localChassisID := strings.TrimSpace(obs.ChassisID)
		localSysName := strings.TrimSpace(obs.Hostname)

		remotes := sortedLLDPRemotes(obs.LLDPRemotes)
		for _, remote := range remotes {
			localPortID := strings.TrimSpace(remote.LocalPortID)
			localPortDescr := strings.TrimSpace(remote.LocalPortDesc)
			sourcePort := localPortID
			if sourcePort == "" {
				sourcePort = localPortDescr
			}
			if sourcePort == "" {
				sourcePort = strings.TrimSpace(remote.LocalPortNum)
			}

			targetPort := strings.TrimSpace(remote.PortID)
			if targetPort == "" {
				targetPort = strings.TrimSpace(remote.PortDesc)
			}

			links = append(links, lldpMatchLink{
				index: len(links),

				sourceDeviceID: sourceID,
				localChassisID: localChassisID,
				localSysName:   localSysName,
				localMatchID:   sourceID,

				localPortID:        localPortID,
				localPortIDSubtype: strings.TrimSpace(remote.LocalPortIDSubtype),
				localPortDescr:     localPortDescr,

				remoteChassisID:     strings.TrimSpace(remote.ChassisID),
				remoteSysName:       strings.TrimSpace(remote.SysName),
				remoteMatchID:       "",
				remotePortID:        strings.TrimSpace(remote.PortID),
				remotePortIDSubtype: strings.TrimSpace(remote.PortIDSubtype),
				remotePortDescr:     strings.TrimSpace(remote.PortDesc),

				sourcePort:       sourcePort,
				targetPort:       targetPort,
				remoteManagement: strings.TrimSpace(remote.ManagementIP),
				remoteFallbackID: strings.TrimSpace(remote.SysName),
			})
		}
	}
	return links
}

func annotateLLDPLinkMatchIdentities(
	links []lldpMatchLink,
	hostToID map[string]string,
	chassisToID map[string]string,
	ipToID map[string]string,
) {
	for i := range links {
		link := &links[i]
		if strings.TrimSpace(link.localMatchID) == "" {
			link.localMatchID = strings.TrimSpace(link.sourceDeviceID)
		}
		if strings.TrimSpace(link.remoteMatchID) != "" {
			continue
		}
		link.remoteMatchID = resolveKnownDeviceID(hostToID, chassisToID, ipToID, link.remoteSysName, link.remoteChassisID, link.remoteManagement)
	}
}

func resolveKnownDeviceID(
	hostToID map[string]string,
	chassisToID map[string]string,
	ipToID map[string]string,
	hostname, chassisID, managementIP string,
) string {
	if id := hostToID[canonicalHost(hostname)]; strings.TrimSpace(id) != "" {
		return strings.TrimSpace(id)
	}
	if id := chassisToID[canonicalToken(chassisID)]; strings.TrimSpace(id) != "" {
		return strings.TrimSpace(id)
	}
	if id := ipToID[canonicalIP(managementIP)]; strings.TrimSpace(id) != "" {
		return strings.TrimSpace(id)
	}
	return ""
}

func lldpIdentityTokenForMatch(matchID, chassisID string) string {
	matchID = strings.TrimSpace(matchID)
	if matchID != "" {
		return "device:" + matchID
	}
	return normalizeLLDPChassisForMatch(chassisID)
}

func buildLLDPLookupMap(links []lldpMatchLink) map[string]int {
	lookup := make(map[string]int, len(links)*6)
	for _, link := range links {
		defaultKey := lldpCompositeKey(
			lldpIdentityTokenForMatch(link.remoteMatchID, link.remoteChassisID),
			lldpIdentityTokenForMatch(link.localMatchID, link.localChassisID),
			normalizeLLDPPortIDForMatch(link.localPortID, link.localPortIDSubtype),
			normalizeLLDPPortSubtypeForMatch(link.localPortIDSubtype),
			normalizeLLDPPortIDForMatch(link.remotePortID, link.remotePortIDSubtype),
			normalizeLLDPPortSubtypeForMatch(link.remotePortIDSubtype),
		)
		lookup[defaultKey] = link.index

		descrKey := lldpCompositeKey(
			lldpIdentityTokenForMatch(link.remoteMatchID, link.remoteChassisID),
			lldpIdentityTokenForMatch(link.localMatchID, link.localChassisID),
			link.localPortDescr,
			link.remotePortDescr,
		)
		lookup[descrKey] = link.index

		sysNameKey := lldpCompositeKey(
			link.remoteSysName,
			link.localSysName,
			normalizeLLDPPortIDForMatch(link.localPortID, link.localPortIDSubtype),
			normalizeLLDPPortSubtypeForMatch(link.localPortIDSubtype),
			normalizeLLDPPortIDForMatch(link.remotePortID, link.remotePortIDSubtype),
			normalizeLLDPPortSubtypeForMatch(link.remotePortIDSubtype),
		)
		lookup[sysNameKey] = link.index

		elementaryAKey := lldpCompositeKey(
			lldpIdentityTokenForMatch(link.remoteMatchID, link.remoteChassisID),
			lldpIdentityTokenForMatch(link.localMatchID, link.localChassisID),
			normalizeLLDPPortIDForMatch(link.remotePortID, link.remotePortIDSubtype),
			normalizeLLDPPortSubtypeForMatch(link.remotePortIDSubtype),
		)
		lookup[elementaryAKey] = link.index

		elementaryBKey := lldpCompositeKey(
			lldpIdentityTokenForMatch(link.remoteMatchID, link.remoteChassisID),
			lldpIdentityTokenForMatch(link.localMatchID, link.localChassisID),
			link.remotePortDescr,
		)
		lookup[elementaryBKey] = link.index

		elementaryCKey := lldpCompositeKey(
			lldpIdentityTokenForMatch(link.remoteMatchID, link.remoteChassisID),
			lldpIdentityTokenForMatch(link.localMatchID, link.localChassisID),
		)
		lookup[elementaryCKey] = link.index
	}
	return lookup
}

func lldpCompositeKey(parts ...string) string {
	return topologyMatchCompositeKey(parts...)
}

func matchLLDPLinksEnlinkdPassOrder(links []lldpMatchLink) []lldpMatchedPair {
	if len(links) == 0 {
		return nil
	}

	lookup := buildLLDPLookupMap(links)
	parsed := make(map[int]struct{}, len(links))
	pairs := make([]lldpMatchedPair, 0, len(links)/2)

	addPair := func(sourceIndex, targetIndex int, pass string) {
		parsed[sourceIndex] = struct{}{}
		parsed[targetIndex] = struct{}{}
		pairs = append(pairs, lldpMatchedPair{
			sourceIndex: sourceIndex,
			targetIndex: targetIndex,
			pass:        pass,
		})
	}

	for _, source := range links {
		if _, ok := parsed[source.index]; ok {
			continue
		}
		if lldpIdentityTokenForMatch(source.localMatchID, source.localChassisID) == lldpIdentityTokenForMatch(source.remoteMatchID, source.remoteChassisID) ||
			(source.localSysName != "" && source.localSysName == source.remoteSysName) {
			parsed[source.index] = struct{}{}
			continue
		}

		key := lldpCompositeKey(
			lldpIdentityTokenForMatch(source.localMatchID, source.localChassisID),
			lldpIdentityTokenForMatch(source.remoteMatchID, source.remoteChassisID),
			normalizeLLDPPortIDForMatch(source.remotePortID, source.remotePortIDSubtype),
			normalizeLLDPPortSubtypeForMatch(source.remotePortIDSubtype),
			normalizeLLDPPortIDForMatch(source.localPortID, source.localPortIDSubtype),
			normalizeLLDPPortSubtypeForMatch(source.localPortIDSubtype),
		)
		targetIndex, ok := lookup[key]
		if !ok {
			continue
		}
		if source.index == targetIndex {
			continue
		}
		if _, targetParsed := parsed[targetIndex]; targetParsed {
			continue
		}
		addPair(source.index, targetIndex, lldpMatchPassDefault)
	}

	for _, source := range links {
		if _, ok := parsed[source.index]; ok {
			continue
		}
		if strings.TrimSpace(source.remotePortDescr) == "" || strings.TrimSpace(source.localPortDescr) == "" {
			continue
		}
		key := lldpCompositeKey(
			lldpIdentityTokenForMatch(source.localMatchID, source.localChassisID),
			lldpIdentityTokenForMatch(source.remoteMatchID, source.remoteChassisID),
			source.remotePortDescr,
			source.localPortDescr,
		)
		targetIndex, ok := lookup[key]
		if !ok {
			continue
		}
		if source.index == targetIndex {
			continue
		}
		if _, targetParsed := parsed[targetIndex]; targetParsed {
			continue
		}
		addPair(source.index, targetIndex, lldpMatchPassPortDesc)
	}

	for _, source := range links {
		if _, ok := parsed[source.index]; ok {
			continue
		}
		key := lldpCompositeKey(
			source.localSysName,
			source.remoteSysName,
			normalizeLLDPPortIDForMatch(source.remotePortID, source.remotePortIDSubtype),
			normalizeLLDPPortSubtypeForMatch(source.remotePortIDSubtype),
			normalizeLLDPPortIDForMatch(source.localPortID, source.localPortIDSubtype),
			normalizeLLDPPortSubtypeForMatch(source.localPortIDSubtype),
		)
		targetIndex, ok := lookup[key]
		if !ok {
			continue
		}
		if source.index == targetIndex {
			continue
		}
		if _, targetParsed := parsed[targetIndex]; targetParsed {
			continue
		}
		addPair(source.index, targetIndex, lldpMatchPassSysName)
	}

	for _, source := range links {
		if _, ok := parsed[source.index]; ok {
			continue
		}
		key := lldpCompositeKey(
			lldpIdentityTokenForMatch(source.localMatchID, source.localChassisID),
			lldpIdentityTokenForMatch(source.remoteMatchID, source.remoteChassisID),
			normalizeLLDPPortIDForMatch(source.localPortID, source.localPortIDSubtype),
			normalizeLLDPPortSubtypeForMatch(source.localPortIDSubtype),
		)
		targetIndex, ok := lookup[key]
		if !ok {
			continue
		}
		if source.index == targetIndex {
			continue
		}
		if _, targetParsed := parsed[targetIndex]; targetParsed {
			continue
		}
		addPair(source.index, targetIndex, lldpMatchPassChassisPort)
	}

	for _, source := range links {
		if _, ok := parsed[source.index]; ok {
			continue
		}
		key := lldpCompositeKey(
			lldpIdentityTokenForMatch(source.localMatchID, source.localChassisID),
			lldpIdentityTokenForMatch(source.remoteMatchID, source.remoteChassisID),
			source.localPortDescr,
		)
		targetIndex, ok := lookup[key]
		if !ok {
			continue
		}
		if source.index == targetIndex {
			continue
		}
		if _, targetParsed := parsed[targetIndex]; targetParsed {
			continue
		}
		addPair(source.index, targetIndex, lldpMatchPassChassisDescr)
	}

	for _, source := range links {
		if _, ok := parsed[source.index]; ok {
			continue
		}
		key := lldpCompositeKey(
			lldpIdentityTokenForMatch(source.localMatchID, source.localChassisID),
			lldpIdentityTokenForMatch(source.remoteMatchID, source.remoteChassisID),
		)
		targetIndex, ok := lookup[key]
		if !ok {
			continue
		}
		if source.index == targetIndex {
			continue
		}
		if _, targetParsed := parsed[targetIndex]; targetParsed {
			continue
		}
		addPair(source.index, targetIndex, lldpMatchPassChassis)
	}

	return pairs
}

func buildLLDPTargetOverrides(links []lldpMatchLink, pairs []lldpMatchedPair) map[int]string {
	if len(pairs) == 0 {
		return nil
	}

	indexToLink := make(map[int]lldpMatchLink, len(links))
	for _, link := range links {
		indexToLink[link.index] = link
	}

	overrides := make(map[int]string, len(pairs)*2)
	for _, pair := range pairs {
		source, sourceOK := indexToLink[pair.sourceIndex]
		target, targetOK := indexToLink[pair.targetIndex]
		if !sourceOK || !targetOK {
			continue
		}

		if _, exists := overrides[source.index]; !exists {
			overrides[source.index] = target.sourceDeviceID
		}
		if _, exists := overrides[target.index]; !exists {
			overrides[target.index] = source.sourceDeviceID
		}
	}

	return overrides
}

func buildLLDPPairMetadata(links []lldpMatchLink, pairs []lldpMatchedPair) map[int]matchedPairMetadata {
	if len(pairs) == 0 {
		return nil
	}

	indexToLink := make(map[int]lldpMatchLink, len(links))
	for _, link := range links {
		indexToLink[link.index] = link
	}

	metadata := make(map[int]matchedPairMetadata, len(pairs)*2)
	for _, pair := range pairs {
		sourceLink, sourceOK := indexToLink[pair.sourceIndex]
		targetLink, targetOK := indexToLink[pair.targetIndex]
		if !sourceOK || !targetOK {
			continue
		}

		pairID := canonicalAdjacencyPairID(
			"lldp",
			sourceLink.sourceDeviceID,
			sourceLink.sourcePort,
			targetLink.sourceDeviceID,
			targetLink.sourcePort,
		)
		if pairID == "" {
			continue
		}

		metadata[sourceLink.index] = matchedPairMetadata{
			id:   pairID,
			pass: pair.pass,
		}
		metadata[targetLink.index] = matchedPairMetadata{
			id:   pairID,
			pass: pair.pass,
		}
	}

	return metadata
}
