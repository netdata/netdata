// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"encoding/hex"
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"
)

type enrichmentAccumulator struct {
	EndpointID string
	MAC        string
	IPs        map[string]netip.Addr
	Protocols  map[string]struct{}
	DeviceIDs  map[string]struct{}
	IfIndexes  map[string]struct{}
	IfNames    map[string]struct{}
	States     map[string]struct{}
	AddrTypes  map[string]struct{}
}

type fdbCandidate struct {
	mac        string
	bridgePort string
	ifIndex    int
	statusRaw  string
}

const (
	lldpMatchPassDefault      = "default"
	lldpMatchPassPortDesc     = "port_description"
	lldpMatchPassSysName      = "sysname"
	lldpMatchPassChassisPort  = "chassis_port_id_subtype"
	lldpMatchPassChassisDescr = "chassis_port_descr"
	lldpMatchPassChassis      = "chassis"

	cdpMatchPassDefault = "default"

	fdbStatusLearned = "learned"
	fdbStatusSelf    = "self"
	fdbStatusIgnored = "ignored"

	adjacencyLabelPairID   = "pair_id"
	adjacencyLabelPairSide = "pair_side"
	adjacencyLabelPairPass = "pair_pass"

	adjacencyPairSideSource = "source"
	adjacencyPairSideTarget = "target"
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

type cdpMatchLink struct {
	index int

	sourceDeviceID string
	sourceGlobalID string

	localInterfaceName string

	remoteDeviceID   string
	remoteDevicePort string
	remoteHost       string
	remoteAddressRaw string
}

type cdpMatchedPair struct {
	sourceIndex int
	targetIndex int
	pass        string
}

type matchedPairMetadata struct {
	id   string
	side string
	pass string
}

// BuildL2ResultFromObservations converts normalized L2 observations into a
// deterministic engine result.
func BuildL2ResultFromObservations(observations []L2Observation, opts DiscoverOptions) (Result, error) {
	if len(observations) == 0 {
		return Result{}, fmt.Errorf("at least one observation is required")
	}
	if !opts.EnableLLDP && !opts.EnableCDP && !opts.EnableBridge && !opts.EnableARP {
		opts.EnableLLDP = true
		opts.EnableCDP = true
	}

	devices := make(map[string]Device, len(observations))
	interfaces := make(map[string]Interface)
	adjacencies := make(map[string]Adjacency)
	attachments := make(map[string]Attachment)
	enrichments := make(map[string]*enrichmentAccumulator)
	ifNameByDeviceIfIndex := make(map[string]string)

	hostToID := make(map[string]string, len(observations))
	ipToID := make(map[string]string, len(observations))
	chassisToID := make(map[string]string, len(observations))

	for _, obs := range observations {
		deviceID := strings.TrimSpace(obs.DeviceID)
		if deviceID == "" {
			return Result{}, fmt.Errorf("observation with empty device id")
		}

		device := Device{
			ID:        deviceID,
			Hostname:  strings.TrimSpace(obs.Hostname),
			SysObject: strings.TrimSpace(obs.SysObjectID),
			ChassisID: strings.TrimSpace(obs.ChassisID),
		}
		if device.Hostname == "" {
			device.Hostname = device.ID
		}
		if addr := parseAddr(obs.ManagementIP); addr.IsValid() {
			device.Addresses = []netip.Addr{addr}
		}
		devices[device.ID] = device

		if host := canonicalHost(device.Hostname); host != "" {
			hostToID[host] = device.ID
		}
		if ip := canonicalIP(obs.ManagementIP); ip != "" {
			ipToID[ip] = device.ID
		}
		if chassis := canonicalToken(device.ChassisID); chassis != "" {
			chassisToID[chassis] = device.ID
		}

		for _, iface := range obs.Interfaces {
			if iface.IfIndex <= 0 {
				continue
			}
			ifName := strings.TrimSpace(iface.IfName)
			ifDescr := strings.TrimSpace(iface.IfDescr)
			if ifName == "" {
				ifName = ifDescr
			}
			if ifDescr == "" {
				ifDescr = ifName
			}
			if ifName == "" {
				continue
			}
			engIface := Interface{DeviceID: device.ID, IfIndex: iface.IfIndex, IfName: ifName, IfDescr: ifDescr}
			interfaces[ifaceKey(engIface)] = engIface
			ifNameByDeviceIfIndex[deviceIfIndexKey(device.ID, iface.IfIndex)] = ifName
		}
	}

	linksLLDP := 0
	linksCDP := 0
	attachmentsFDB := 0
	enrichmentsARPND := 0
	bridgeDomains := make(map[string]struct{})
	endpointIDs := make(map[string]struct{})

	resolveRemote := func(hostname, chassisID, mgmtIP, fallbackID string) string {
		if id := hostToID[canonicalHost(hostname)]; id != "" {
			return id
		}
		if id := chassisToID[canonicalToken(chassisID)]; id != "" {
			return id
		}
		if id := ipToID[canonicalIP(mgmtIP)]; id != "" {
			return id
		}

		generatedID := deriveRemoteDeviceID(hostname, chassisID, mgmtIP, fallbackID)
		if _, ok := devices[generatedID]; !ok {
			device := Device{
				ID:        generatedID,
				Hostname:  strings.TrimSpace(hostname),
				SysObject: "",
				ChassisID: strings.TrimSpace(chassisID),
			}
			if device.Hostname == "" {
				device.Hostname = generatedID
			}
			if ip := parseAddr(mgmtIP); ip.IsValid() {
				device.Addresses = []netip.Addr{ip}
			}
			devices[generatedID] = device
		}
		if host := canonicalHost(hostname); host != "" {
			hostToID[host] = generatedID
		}
		if chassis := canonicalToken(chassisID); chassis != "" {
			chassisToID[chassis] = generatedID
		}
		if ip := canonicalIP(mgmtIP); ip != "" {
			ipToID[ip] = generatedID
		}
		return generatedID
	}

	if opts.EnableLLDP {
		lldpLinks := buildLLDPMatchLinks(observations)
		annotateLLDPLinkMatchIdentities(lldpLinks, hostToID, chassisToID, ipToID)
		lldpPairs := matchLLDPLinksEnlinkdPassOrder(lldpLinks)
		lldpTargetOverrides := buildLLDPTargetOverrides(lldpLinks, lldpPairs)
		lldpPairMetadata := buildLLDPPairMetadata(lldpLinks, lldpPairs)

		for _, link := range lldpLinks {
			targetID := strings.TrimSpace(lldpTargetOverrides[link.index])
			if targetID == "" {
				targetID = resolveRemote(link.remoteSysName, link.remoteChassisID, link.remoteManagement, link.remoteFallbackID)
			}

			adj := Adjacency{
				Protocol:   "lldp",
				SourceID:   link.sourceDeviceID,
				SourcePort: link.sourcePort,
				TargetID:   targetID,
				TargetPort: link.targetPort,
			}
			applyAdjacencyPairMetadata(&adj, lldpPairMetadata[link.index])
			if addAdjacency(adjacencies, adj) {
				linksLLDP++
			}
		}
	}

	if opts.EnableCDP {
		cdpLinks := buildCDPMatchLinks(observations)
		cdpPairs := matchCDPLinksEnlinkdPassOrder(cdpLinks)
		cdpTargetOverrides := buildCDPTargetOverrides(cdpLinks, cdpPairs)
		cdpPairMetadata := buildCDPPairMetadata(cdpLinks, cdpPairs)

		for _, link := range cdpLinks {
			rawAddress := strings.TrimSpace(link.remoteAddressRaw)
			targetID := strings.TrimSpace(cdpTargetOverrides[link.index])
			if targetID == "" {
				targetIP := canonicalIP(rawAddress)
				targetID = resolveRemote(link.remoteHost, link.remoteDeviceID, targetIP, link.remoteDeviceID)
			}

			adj := Adjacency{
				Protocol:   "cdp",
				SourceID:   link.sourceDeviceID,
				SourcePort: link.localInterfaceName,
				TargetID:   targetID,
				TargetPort: link.remoteDevicePort,
			}
			if rawAddress != "" {
				adj.Labels = map[string]string{
					"remote_address_raw": strings.ToLower(rawAddress),
				}
			}
			applyAdjacencyPairMetadata(&adj, cdpPairMetadata[link.index])
			if addAdjacency(adjacencies, adj) {
				linksCDP++
			}
		}
	}

	if opts.EnableBridge {
		for _, obs := range observations {
			sourceID := strings.TrimSpace(obs.DeviceID)
			if sourceID == "" {
				continue
			}

			bridgePortToIfIndex := make(map[string]int, len(obs.BridgePorts))
			for _, bridgePort := range sortedBridgePorts(obs.BridgePorts) {
				basePort := strings.TrimSpace(bridgePort.BasePort)
				if basePort == "" || bridgePort.IfIndex <= 0 {
					continue
				}
				bridgePortToIfIndex[basePort] = bridgePort.IfIndex
			}

			for _, candidate := range buildFDBCandidates(obs.FDBEntries, bridgePortToIfIndex) {
				endpointID := "mac:" + candidate.mac
				attachment := Attachment{
					DeviceID:   sourceID,
					IfIndex:    candidate.ifIndex,
					EndpointID: endpointID,
					Method:     "fdb",
				}

				labels := make(map[string]string)
				if candidate.bridgePort != "" {
					labels["bridge_port"] = candidate.bridgePort
				}
				if status := strings.TrimSpace(candidate.statusRaw); status != "" {
					labels["fdb_status"] = status
				}
				if candidate.ifIndex > 0 {
					ifName := strings.TrimSpace(ifNameByDeviceIfIndex[deviceIfIndexKey(sourceID, candidate.ifIndex)])
					if ifName != "" {
						labels["if_name"] = ifName
					}
					labels["bridge_domain"] = deriveBridgeDomainFromIfIndex(sourceID, candidate.ifIndex)
				} else if candidate.bridgePort != "" {
					labels["bridge_domain"] = deriveBridgeDomainFromBridgePort(sourceID, candidate.bridgePort)
				}
				if len(labels) > 0 {
					attachment.Labels = labels
				}

				if addAttachment(attachments, attachment) {
					attachmentsFDB++
					endpointIDs[endpointID] = struct{}{}
					if domain := attachmentDomain(attachment); domain != "" {
						bridgeDomains[domain] = struct{}{}
					}
				}
			}
		}
	}

	if opts.EnableARP {
		for _, obs := range observations {
			sourceID := strings.TrimSpace(obs.DeviceID)
			if sourceID == "" {
				continue
			}
			for _, entry := range sortedARPNDEntries(obs.ARPNDEntries) {
				mac := normalizeMAC(entry.MAC)
				ip := canonicalIP(entry.IP)
				if mac == "" && ip == "" {
					continue
				}

				endpointID := ""
				if mac != "" {
					endpointID = "mac:" + mac
				} else {
					endpointID = "ip:" + ip
				}
				acc := ensureEnrichmentAccumulator(enrichments, endpointID)
				acc.EndpointID = endpointID
				if mac != "" {
					acc.MAC = mac
				}
				if ip != "" {
					addr := parseAddr(ip)
					if addr.IsValid() {
						acc.IPs[addr.String()] = addr
					}
				}

				protocol := canonicalProtocol(entry.Protocol)
				acc.Protocols[protocol] = struct{}{}
				acc.DeviceIDs[sourceID] = struct{}{}
				if entry.IfIndex > 0 {
					acc.IfIndexes[strconv.Itoa(entry.IfIndex)] = struct{}{}
				}
				ifName := strings.TrimSpace(entry.IfName)
				if ifName == "" && entry.IfIndex > 0 {
					ifName = strings.TrimSpace(ifNameByDeviceIfIndex[deviceIfIndexKey(sourceID, entry.IfIndex)])
				}
				if ifName != "" {
					acc.IfNames[ifName] = struct{}{}
				}
				if state := strings.TrimSpace(entry.State); state != "" {
					acc.States[state] = struct{}{}
				}
				if addrType := canonicalAddrType(entry.AddrType, ip); addrType != "" {
					acc.AddrTypes[addrType] = struct{}{}
				}
			}
		}
		mergeIPOnlyEnrichmentsIntoMAC(enrichments)
		for endpointID := range endpointIDs {
			delete(endpointIDs, endpointID)
		}
		for _, attachment := range attachments {
			endpointID := strings.TrimSpace(attachment.EndpointID)
			if endpointID == "" {
				continue
			}
			endpointIDs[endpointID] = struct{}{}
		}
		for _, enrichment := range enrichments {
			if enrichment == nil {
				continue
			}
			endpointID := strings.TrimSpace(enrichment.EndpointID)
			if endpointID == "" {
				continue
			}
			endpointIDs[endpointID] = struct{}{}
		}
		enrichmentsARPND = len(enrichments)
	}

	result := Result{
		CollectedAt: time.Now().UTC(),
		Devices:     sortedDevices(devices),
		Interfaces:  sortedInterfaces(interfaces),
		Adjacencies: sortedAdjacencies(adjacencies),
		Attachments: sortedAttachments(attachments),
		Enrichments: sortedEnrichments(enrichments),
		Stats: map[string]any{
			"devices_total":        len(devices),
			"links_total":          len(adjacencies),
			"links_lldp":           linksLLDP,
			"links_cdp":            linksCDP,
			"attachments_total":    len(attachments),
			"attachments_fdb":      attachmentsFDB,
			"enrichments_total":    len(enrichments),
			"enrichments_arp_nd":   enrichmentsARPND,
			"bridge_domains_total": len(bridgeDomains),
			"endpoints_total":      len(endpointIDs),
		},
	}
	return result, nil
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
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	for i, part := range parts {
		if i > 0 {
			b.WriteByte('|')
		}
		b.WriteString(strconv.Itoa(len(part)))
		b.WriteByte(':')
		b.WriteString(part)
	}
	return b.String()
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
			source.localSysName == source.remoteSysName {
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
		addPair(source.index, targetIndex, lldpMatchPassDefault)
	}

	for _, source := range links {
		if _, ok := parsed[source.index]; ok {
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

func buildCDPMatchLinks(observations []L2Observation) []cdpMatchLink {
	links := make([]cdpMatchLink, 0)
	for _, obs := range observations {
		sourceID := strings.TrimSpace(obs.DeviceID)
		if sourceID == "" {
			continue
		}

		sourceGlobalID := strings.TrimSpace(obs.Hostname)
		if sourceGlobalID == "" {
			sourceGlobalID = sourceID
		}

		remotes := sortedCDPRemotes(obs.CDPRemotes)
		for _, remote := range remotes {
			localInterfaceName := strings.TrimSpace(remote.LocalIfName)
			if localInterfaceName == "" && remote.LocalIfIndex > 0 {
				localInterfaceName = strconv.Itoa(remote.LocalIfIndex)
			}

			remoteDeviceID := strings.TrimSpace(remote.DeviceID)
			remoteHost := strings.TrimSpace(remote.SysName)
			if remoteHost == "" {
				remoteHost = remoteDeviceID
			}
			if remoteDeviceID == "" {
				remoteDeviceID = remoteHost
			}

			links = append(links, cdpMatchLink{
				index:              len(links),
				sourceDeviceID:     sourceID,
				sourceGlobalID:     sourceGlobalID,
				localInterfaceName: localInterfaceName,
				remoteDeviceID:     remoteDeviceID,
				remoteDevicePort:   strings.TrimSpace(remote.DevicePort),
				remoteHost:         remoteHost,
				remoteAddressRaw:   strings.TrimSpace(remote.Address),
			})
		}
	}
	return links
}

func buildCDPLookupMap(links []cdpMatchLink) map[string]int {
	lookup := make(map[string]int, len(links))
	for _, link := range links {
		key := lldpCompositeKey(
			link.remoteDevicePort,
			link.localInterfaceName,
			link.sourceGlobalID,
			link.remoteDeviceID,
		)
		lookup[key] = link.index
	}
	return lookup
}

func matchCDPLinksEnlinkdPassOrder(links []cdpMatchLink) []cdpMatchedPair {
	if len(links) == 0 {
		return nil
	}

	lookup := buildCDPLookupMap(links)
	parsed := make(map[int]struct{}, len(links))
	pairs := make([]cdpMatchedPair, 0, len(links)/2)

	for _, source := range links {
		if _, ok := parsed[source.index]; ok {
			continue
		}

		key := lldpCompositeKey(
			source.localInterfaceName,
			source.remoteDevicePort,
			source.remoteDeviceID,
			source.sourceGlobalID,
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

		parsed[source.index] = struct{}{}
		parsed[targetIndex] = struct{}{}
		pairs = append(pairs, cdpMatchedPair{
			sourceIndex: source.index,
			targetIndex: targetIndex,
			pass:        cdpMatchPassDefault,
		})
	}

	return pairs
}

func buildCDPTargetOverrides(links []cdpMatchLink, pairs []cdpMatchedPair) map[int]string {
	if len(pairs) == 0 {
		return nil
	}

	indexToLink := make(map[int]cdpMatchLink, len(links))
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

		overrides[source.index] = target.sourceDeviceID
		overrides[target.index] = source.sourceDeviceID
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
			side: adjacencyPairSideSource,
			pass: pair.pass,
		}
		metadata[targetLink.index] = matchedPairMetadata{
			id:   pairID,
			side: adjacencyPairSideTarget,
			pass: pair.pass,
		}
	}

	return metadata
}

func buildCDPPairMetadata(links []cdpMatchLink, pairs []cdpMatchedPair) map[int]matchedPairMetadata {
	if len(pairs) == 0 {
		return nil
	}

	indexToLink := make(map[int]cdpMatchLink, len(links))
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
			"cdp",
			sourceLink.sourceDeviceID,
			sourceLink.localInterfaceName,
			targetLink.sourceDeviceID,
			targetLink.localInterfaceName,
		)
		if pairID == "" {
			continue
		}

		metadata[sourceLink.index] = matchedPairMetadata{
			id:   pairID,
			side: adjacencyPairSideSource,
			pass: pair.pass,
		}
		metadata[targetLink.index] = matchedPairMetadata{
			id:   pairID,
			side: adjacencyPairSideTarget,
			pass: pair.pass,
		}
	}

	return metadata
}

func canonicalAdjacencyPairID(protocol, leftDeviceID, leftPort, rightDeviceID, rightPort string) string {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	leftKey := lldpCompositeKey(strings.TrimSpace(leftDeviceID), strings.TrimSpace(leftPort))
	rightKey := lldpCompositeKey(strings.TrimSpace(rightDeviceID), strings.TrimSpace(rightPort))
	if protocol == "" || leftKey == "" || rightKey == "" {
		return ""
	}
	if rightKey < leftKey {
		leftKey, rightKey = rightKey, leftKey
	}
	return protocol + ":" + leftKey + "<->" + rightKey
}

func applyAdjacencyPairMetadata(adj *Adjacency, metadata matchedPairMetadata) {
	if adj == nil || metadata.id == "" || metadata.side == "" {
		return
	}
	if adj.Labels == nil {
		adj.Labels = make(map[string]string)
	}
	adj.Labels[adjacencyLabelPairID] = metadata.id
	adj.Labels[adjacencyLabelPairSide] = metadata.side
	if metadata.pass != "" {
		adj.Labels[adjacencyLabelPairPass] = metadata.pass
	}
}

func addAdjacency(adjacencies map[string]Adjacency, adj Adjacency) bool {
	sourceID := strings.TrimSpace(adj.SourceID)
	targetID := strings.TrimSpace(adj.TargetID)
	if sourceID == "" || targetID == "" {
		return false
	}
	// Some agents can emit self-referential rows with identical local and
	// remote ports (or missing port information). These are not meaningful
	// topology links.
	if sourceID == targetID {
		sourcePort := strings.TrimSpace(adj.SourcePort)
		targetPort := strings.TrimSpace(adj.TargetPort)
		if sourcePort == "" || targetPort == "" || sourcePort == targetPort {
			return false
		}
	}
	key := adjacencyKey(adj)
	if _, ok := adjacencies[key]; ok {
		return false
	}
	adjacencies[key] = adj
	return true
}

func addAttachment(attachments map[string]Attachment, attachment Attachment) bool {
	if strings.TrimSpace(attachment.DeviceID) == "" || strings.TrimSpace(attachment.EndpointID) == "" {
		return false
	}
	key := attachmentKey(attachment)
	if _, ok := attachments[key]; ok {
		return false
	}
	attachments[key] = attachment
	return true
}

func sortedLLDPRemotes(in []LLDPRemoteObservation) []LLDPRemoteObservation {
	out := make([]LLDPRemoteObservation, 0, len(in))
	for _, remote := range in {
		if strings.TrimSpace(remote.ChassisID) == "" && strings.TrimSpace(remote.SysName) == "" {
			continue
		}
		out = append(out, remote)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.LocalPortNum != b.LocalPortNum {
			return a.LocalPortNum < b.LocalPortNum
		}
		if a.RemoteIndex != b.RemoteIndex {
			return a.RemoteIndex < b.RemoteIndex
		}
		if a.SysName != b.SysName {
			return a.SysName < b.SysName
		}
		if a.ChassisID != b.ChassisID {
			return a.ChassisID < b.ChassisID
		}
		if a.PortID != b.PortID {
			return a.PortID < b.PortID
		}
		if a.PortIDSubtype != b.PortIDSubtype {
			return a.PortIDSubtype < b.PortIDSubtype
		}
		if a.LocalPortIDSubtype != b.LocalPortIDSubtype {
			return a.LocalPortIDSubtype < b.LocalPortIDSubtype
		}
		if a.PortDesc != b.PortDesc {
			return a.PortDesc < b.PortDesc
		}
		if a.LocalPortDesc != b.LocalPortDesc {
			return a.LocalPortDesc < b.LocalPortDesc
		}
		return a.ManagementIP < b.ManagementIP
	})
	return out
}

func sortedCDPRemotes(in []CDPRemoteObservation) []CDPRemoteObservation {
	out := make([]CDPRemoteObservation, 0, len(in))
	for _, remote := range in {
		if strings.TrimSpace(remote.DeviceID) == "" && strings.TrimSpace(remote.Address) == "" {
			continue
		}
		out = append(out, remote)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.LocalIfIndex != b.LocalIfIndex {
			return a.LocalIfIndex < b.LocalIfIndex
		}
		if a.DeviceIndex != b.DeviceIndex {
			return a.DeviceIndex < b.DeviceIndex
		}
		if a.SysName != b.SysName {
			return a.SysName < b.SysName
		}
		if a.DeviceID != b.DeviceID {
			return a.DeviceID < b.DeviceID
		}
		return a.Address < b.Address
	})
	return out
}

func sortedBridgePorts(in []BridgePortObservation) []BridgePortObservation {
	out := make([]BridgePortObservation, 0, len(in))
	for _, bridgePort := range in {
		if strings.TrimSpace(bridgePort.BasePort) == "" || bridgePort.IfIndex <= 0 {
			continue
		}
		out = append(out, bridgePort)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.BasePort != b.BasePort {
			return a.BasePort < b.BasePort
		}
		return a.IfIndex < b.IfIndex
	})
	return out
}

func sortedFDBEntries(in []FDBObservation) []FDBObservation {
	out := make([]FDBObservation, 0, len(in))
	for _, entry := range in {
		if strings.TrimSpace(entry.MAC) == "" {
			continue
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.BridgePort != b.BridgePort {
			return a.BridgePort < b.BridgePort
		}
		if a.IfIndex != b.IfIndex {
			return a.IfIndex < b.IfIndex
		}
		if a.MAC != b.MAC {
			return a.MAC < b.MAC
		}
		return a.Status < b.Status
	})
	return out
}

func sortedARPNDEntries(in []ARPNDObservation) []ARPNDObservation {
	out := make([]ARPNDObservation, 0, len(in))
	for _, entry := range in {
		if strings.TrimSpace(entry.MAC) == "" && strings.TrimSpace(entry.IP) == "" {
			continue
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Protocol != b.Protocol {
			return a.Protocol < b.Protocol
		}
		if a.IfIndex != b.IfIndex {
			return a.IfIndex < b.IfIndex
		}
		if a.IP != b.IP {
			return a.IP < b.IP
		}
		if a.MAC != b.MAC {
			return a.MAC < b.MAC
		}
		if a.State != b.State {
			return a.State < b.State
		}
		return a.AddrType < b.AddrType
	})
	return out
}

func sortedDevices(in map[string]Device) []Device {
	out := make([]Device, 0, len(in))
	for _, dev := range in {
		out = append(out, dev)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Hostname < out[j].Hostname
	})
	return out
}

func sortedInterfaces(in map[string]Interface) []Interface {
	out := make([]Interface, 0, len(in))
	for _, iface := range in {
		out = append(out, iface)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.DeviceID != b.DeviceID {
			return a.DeviceID < b.DeviceID
		}
		if a.IfIndex != b.IfIndex {
			return a.IfIndex < b.IfIndex
		}
		return a.IfName < b.IfName
	})
	return out
}

func sortedAdjacencies(in map[string]Adjacency) []Adjacency {
	out := make([]Adjacency, 0, len(in))
	for _, adj := range in {
		out = append(out, adj)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Protocol != b.Protocol {
			return a.Protocol < b.Protocol
		}
		if a.SourceID != b.SourceID {
			return a.SourceID < b.SourceID
		}
		if a.SourcePort != b.SourcePort {
			return a.SourcePort < b.SourcePort
		}
		if a.TargetID != b.TargetID {
			return a.TargetID < b.TargetID
		}
		return a.TargetPort < b.TargetPort
	})
	return out
}

func sortedAttachments(in map[string]Attachment) []Attachment {
	out := make([]Attachment, 0, len(in))
	for _, attachment := range in {
		out = append(out, attachment)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.DeviceID != b.DeviceID {
			return a.DeviceID < b.DeviceID
		}
		if a.IfIndex != b.IfIndex {
			return a.IfIndex < b.IfIndex
		}
		if a.EndpointID != b.EndpointID {
			return a.EndpointID < b.EndpointID
		}
		return a.Method < b.Method
	})
	return out
}

func sortedEnrichments(in map[string]*enrichmentAccumulator) []Enrichment {
	out := make([]Enrichment, 0, len(in))
	for _, acc := range in {
		if acc == nil || strings.TrimSpace(acc.EndpointID) == "" {
			continue
		}
		enrichment := Enrichment{
			EndpointID: acc.EndpointID,
			MAC:        acc.MAC,
			IPs:        sortedAddrValues(acc.IPs),
			Labels: map[string]string{
				"sources":    setToCSV(acc.Protocols),
				"device_ids": setToCSV(acc.DeviceIDs),
				"if_indexes": setToCSV(acc.IfIndexes),
				"if_names":   setToCSV(acc.IfNames),
				"states":     setToCSV(acc.States),
				"addr_types": setToCSV(acc.AddrTypes),
			},
		}
		pruneEmptyLabels(enrichment.Labels)
		out = append(out, enrichment)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.EndpointID != b.EndpointID {
			return a.EndpointID < b.EndpointID
		}
		if a.MAC != b.MAC {
			return a.MAC < b.MAC
		}
		return len(a.IPs) < len(b.IPs)
	})
	return out
}

func ensureEnrichmentAccumulator(enrichments map[string]*enrichmentAccumulator, endpointID string) *enrichmentAccumulator {
	acc := enrichments[endpointID]
	if acc != nil {
		return acc
	}
	acc = &enrichmentAccumulator{
		EndpointID: endpointID,
		IPs:        make(map[string]netip.Addr),
		Protocols:  make(map[string]struct{}),
		DeviceIDs:  make(map[string]struct{}),
		IfIndexes:  make(map[string]struct{}),
		IfNames:    make(map[string]struct{}),
		States:     make(map[string]struct{}),
		AddrTypes:  make(map[string]struct{}),
	}
	enrichments[endpointID] = acc
	return acc
}

func buildFDBCandidates(entries []FDBObservation, bridgePortToIfIndex map[string]int) []fdbCandidate {
	if len(entries) == 0 {
		return nil
	}

	sorted := sortedFDBEntries(entries)
	selfMACs := make(map[string]struct{}, len(sorted))
	for _, entry := range sorted {
		if canonicalFDBStatus(entry.Status) != fdbStatusSelf {
			continue
		}
		mac := normalizeMAC(entry.MAC)
		if mac == "" {
			continue
		}
		selfMACs[mac] = struct{}{}
	}

	candidatesByMAC := make(map[string]fdbCandidate, len(sorted))
	duplicates := make(map[string]struct{})
	for _, entry := range sorted {
		mac := normalizeMAC(entry.MAC)
		if mac == "" {
			continue
		}
		if _, isSelf := selfMACs[mac]; isSelf {
			continue
		}
		if canonicalFDBStatus(entry.Status) != fdbStatusLearned {
			continue
		}

		bridgePort := strings.TrimSpace(entry.BridgePort)
		ifIndex := entry.IfIndex
		if ifIndex <= 0 && bridgePort != "" {
			if mappedIfIndex, ok := bridgePortToIfIndex[bridgePort]; ok {
				ifIndex = mappedIfIndex
			}
		}

		candidate := fdbCandidate{
			mac:        mac,
			bridgePort: bridgePort,
			ifIndex:    ifIndex,
			statusRaw:  strings.TrimSpace(entry.Status),
		}

		existing, exists := candidatesByMAC[mac]
		if !exists {
			candidatesByMAC[mac] = candidate
			continue
		}

		if sameFDBDestination(existing, candidate) {
			updated := existing
			if candidate.statusRaw != "" {
				updated.statusRaw = candidate.statusRaw
			}
			candidatesByMAC[mac] = updated
			continue
		}

		delete(candidatesByMAC, mac)
		duplicates[mac] = struct{}{}
	}

	out := make([]fdbCandidate, 0, len(candidatesByMAC))
	for mac, candidate := range candidatesByMAC {
		if _, duplicated := duplicates[mac]; duplicated {
			continue
		}
		out = append(out, candidate)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].mac != out[j].mac {
			return out[i].mac < out[j].mac
		}
		if out[i].ifIndex != out[j].ifIndex {
			return out[i].ifIndex < out[j].ifIndex
		}
		return out[i].bridgePort < out[j].bridgePort
	})
	return out
}

func canonicalFDBStatus(status string) string {
	normalized := strings.ToLower(strings.TrimSpace(status))
	switch normalized {
	case "", "3", "learned", "dot1d_tp_fdb_status_learned", "dot1dtpfdbstatuslearned":
		return fdbStatusLearned
	case "4", "self", "dot1d_tp_fdb_status_self", "dot1dtpfdbstatusself":
		return fdbStatusSelf
	default:
		if strings.Contains(normalized, "learned") {
			return fdbStatusLearned
		}
		if strings.Contains(normalized, "self") {
			return fdbStatusSelf
		}
		return fdbStatusIgnored
	}
}

func sameFDBDestination(left, right fdbCandidate) bool {
	if left.ifIndex > 0 && right.ifIndex > 0 {
		return left.ifIndex == right.ifIndex
	}
	return left.bridgePort != "" && left.bridgePort == right.bridgePort
}

func mergeIPOnlyEnrichmentsIntoMAC(enrichments map[string]*enrichmentAccumulator) {
	if len(enrichments) == 0 {
		return
	}

	keys := make([]string, 0, len(enrichments))
	for endpointID := range enrichments {
		keys = append(keys, endpointID)
	}
	sort.Strings(keys)

	ipToMACEndpoint := make(map[string]string, len(enrichments))
	for _, endpointID := range keys {
		acc := enrichments[endpointID]
		if acc == nil || strings.TrimSpace(acc.MAC) == "" {
			continue
		}
		for _, ip := range sortedIPKeys(acc.IPs) {
			if _, exists := ipToMACEndpoint[ip]; exists {
				continue
			}
			ipToMACEndpoint[ip] = endpointID
		}
	}

	for _, endpointID := range keys {
		acc := enrichments[endpointID]
		if acc == nil || strings.TrimSpace(acc.MAC) != "" || !strings.HasPrefix(endpointID, "ip:") {
			continue
		}
		targetEndpointID := ""
		for _, ip := range sortedIPKeys(acc.IPs) {
			if target := ipToMACEndpoint[ip]; target != "" {
				targetEndpointID = target
				break
			}
		}
		if targetEndpointID == "" {
			continue
		}
		target := ensureEnrichmentAccumulator(enrichments, targetEndpointID)
		mergeEnrichmentAccumulator(target, acc)
		delete(enrichments, endpointID)
	}
}

func mergeEnrichmentAccumulator(target, source *enrichmentAccumulator) {
	if target == nil || source == nil || target == source {
		return
	}
	if target.MAC == "" {
		target.MAC = source.MAC
	}
	for key, addr := range source.IPs {
		target.IPs[key] = addr
	}
	for key := range source.Protocols {
		target.Protocols[key] = struct{}{}
	}
	for key := range source.DeviceIDs {
		target.DeviceIDs[key] = struct{}{}
	}
	for key := range source.IfIndexes {
		target.IfIndexes[key] = struct{}{}
	}
	for key := range source.IfNames {
		target.IfNames[key] = struct{}{}
	}
	for key := range source.States {
		target.States[key] = struct{}{}
	}
	for key := range source.AddrTypes {
		target.AddrTypes[key] = struct{}{}
	}
}

func sortedIPKeys(in map[string]netip.Addr) []string {
	if len(in) == 0 {
		return nil
	}
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func adjacencyKey(adj Adjacency) string {
	return strings.Join([]string{adj.Protocol, adj.SourceID, adj.SourcePort, adj.TargetID, adj.TargetPort}, "|")
}

func attachmentKey(attachment Attachment) string {
	return strings.Join([]string{
		attachment.DeviceID,
		strconv.Itoa(attachment.IfIndex),
		attachment.EndpointID,
		attachment.Method,
	}, "|")
}

func ifaceKey(iface Interface) string {
	return fmt.Sprintf("%s|%d|%s", iface.DeviceID, iface.IfIndex, iface.IfName)
}

func deviceIfIndexKey(deviceID string, ifIndex int) string {
	return fmt.Sprintf("%s|%d", deviceID, ifIndex)
}

func deriveBridgeDomainFromIfIndex(deviceID string, ifIndex int) string {
	return fmt.Sprintf("bridge-domain:%s:if:%d", deviceID, ifIndex)
}

func deriveBridgeDomainFromBridgePort(deviceID, bridgePort string) string {
	return fmt.Sprintf("bridge-domain:%s:bp:%s", deviceID, bridgePort)
}

func attachmentDomain(attachment Attachment) string {
	if len(attachment.Labels) == 0 {
		return ""
	}
	return strings.TrimSpace(attachment.Labels["bridge_domain"])
}

func canonicalProtocol(protocol string) string {
	protocol = strings.TrimSpace(strings.ToLower(protocol))
	if protocol == "" {
		return "arp"
	}
	return protocol
}

func canonicalAddrType(addrType, ip string) string {
	addrType = strings.TrimSpace(strings.ToLower(addrType))
	if ipAddr := parseAddr(ip); ipAddr.IsValid() {
		if ipAddr.Is4() {
			return "ipv4"
		}
		return "ipv6"
	}
	if addrType == "" {
		return ""
	}
	return addrType
}

func sortedAddrValues(in map[string]netip.Addr) []netip.Addr {
	if len(in) == 0 {
		return nil
	}
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]netip.Addr, 0, len(keys))
	for _, key := range keys {
		if addr, ok := in[key]; ok && addr.IsValid() {
			out = append(out, addr)
		}
	}
	return out
}

func setToCSV(in map[string]struct{}) string {
	if len(in) == 0 {
		return ""
	}
	out := make([]string, 0, len(in))
	for value := range in {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return ""
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func pruneEmptyLabels(labels map[string]string) {
	for key, value := range labels {
		if strings.TrimSpace(value) == "" {
			delete(labels, key)
		}
	}
}

func deriveRemoteDeviceID(hostname, chassisID, mgmtIP, fallback string) string {
	if host := canonicalHost(hostname); host != "" {
		return host
	}
	if ch := canonicalToken(chassisID); ch != "" {
		return "chassis-" + ch
	}
	if ip := canonicalIP(mgmtIP); ip != "" {
		return "ip-" + strings.ReplaceAll(ip, ":", "-")
	}
	if fb := canonicalHost(fallback); fb != "" {
		return fb
	}
	return "discovered-unknown"
}

func canonicalHost(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	v = strings.TrimSuffix(v, ".")
	return v
}

func normalizeLLDPPortIDForMatch(portID, subtype string) string {
	portID = strings.TrimSpace(portID)
	if portID == "" {
		return ""
	}

	switch normalizeLLDPPortSubtypeForMatch(subtype) {
	case "mac":
		if mac := canonicalLLDPMACToken(portID); mac != "" {
			return mac
		}
	case "network":
		if ip := canonicalIP(portID); ip != "" {
			return ip
		}
	}

	return portID
}

func normalizeLLDPPortSubtypeForMatch(subtype string) string {
	switch strings.ToLower(strings.TrimSpace(subtype)) {
	case "3", "macaddress":
		return "mac"
	case "4", "networkaddress":
		return "network"
	default:
		return strings.ToLower(strings.TrimSpace(subtype))
	}
}

func normalizeLLDPChassisForMatch(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if ip := canonicalIP(v); ip != "" {
		return ip
	}
	if mac := canonicalLLDPMACToken(v); mac != "" {
		return mac
	}
	return v
}

func canonicalLLDPMACToken(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" {
		return ""
	}

	clean := strings.TrimPrefix(v, "0x")
	clean = strings.NewReplacer(":", "", "-", "", ".", "", " ", "").Replace(clean)
	if len(clean) != 12 {
		return ""
	}
	if _, err := hex.DecodeString(clean); err != nil {
		return ""
	}
	return clean
}

func canonicalToken(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	v = strings.ReplaceAll(v, ":", "")
	v = strings.ReplaceAll(v, "-", "")
	v = strings.ReplaceAll(v, ".", "")
	v = strings.ReplaceAll(v, " ", "")
	return v
}

func canonicalIP(v string) string {
	if ip := parseAddr(v); ip.IsValid() {
		return ip.String()
	}
	if ip := parseAddr(decodeHexIP(v)); ip.IsValid() {
		return ip.String()
	}
	return ""
}

func parseAddr(v string) netip.Addr {
	addr, err := netip.ParseAddr(strings.TrimSpace(v))
	if err != nil {
		return netip.Addr{}
	}
	return addr.Unmap()
}

func decodeHexIP(v string) string {
	bs := decodeHexBytes(v)
	if len(bs) == 4 {
		addr, ok := netip.AddrFromSlice(bs)
		if ok {
			return addr.Unmap().String()
		}
	}
	if len(bs) == 16 {
		addr, ok := netip.AddrFromSlice(bs)
		if ok {
			return addr.String()
		}
	}
	return ""
}

func decodeHexBytes(v string) []byte {
	clean := strings.ToLower(strings.TrimSpace(v))
	clean = strings.TrimPrefix(clean, "0x")
	if clean == "" {
		return nil
	}

	if strings.ContainsAny(clean, ":-. \t") {
		parts := strings.FieldsFunc(clean, func(r rune) bool {
			return r == ':' || r == '-' || r == '.' || r == ' ' || r == '\t'
		})
		if len(parts) == 0 {
			return nil
		}

		out := make([]byte, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if len(part) > 2 {
				return nil
			}
			if len(part) == 1 {
				part = "0" + part
			}
			b, err := hex.DecodeString(part)
			if err != nil || len(b) != 1 {
				return nil
			}
			out = append(out, b[0])
		}
		if len(out) == 0 {
			return nil
		}
		return out
	}

	if len(clean)%2 == 1 {
		clean = "0" + clean
	}
	bs, err := hex.DecodeString(clean)
	if err != nil {
		return nil
	}
	return bs
}

func normalizeMAC(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}

	if bs := parseDottedDecimalBytes(v); len(bs) == 6 {
		return formatMAC(bs)
	}
	if bs := decodeHexBytes(v); len(bs) == 6 {
		return formatMAC(bs)
	}
	return ""
}

func parseDottedDecimalBytes(v string) []byte {
	parts := strings.Split(strings.TrimSpace(v), ".")
	if len(parts) != 6 {
		return nil
	}
	out := make([]byte, 0, 6)
	for _, part := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 0 || n > 255 {
			return nil
		}
		out = append(out, byte(n))
	}
	return out
}

func formatMAC(bs []byte) string {
	if len(bs) != 6 {
		return ""
	}
	parts := make([]string, 0, 6)
	for _, b := range bs {
		parts = append(parts, fmt.Sprintf("%02x", b))
	}
	return strings.Join(parts, ":")
}
