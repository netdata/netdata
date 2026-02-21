// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"sort"
	"strconv"
	"strings"
)

// bridgeDomainModel is the persisted bridge-domain assembly equivalent that
// topology projection can consume directly.
type bridgeDomainModel struct {
	domains []*bridgeBroadcastDomain
}

type bridgeBroadcastDomain struct {
	bridges  map[string]*bridgeDomainBridge
	segments []*bridgeDomainSegment
}

type bridgeDomainBridge struct {
	nodeID string
	root   bool
}

type bridgeDomainSegment struct {
	designatedPort bridgePortRef
	ports          map[string]bridgePortRef
	endpointIDs    map[string]struct{}
	methods        map[string]struct{}
}

type bridgeBridgeLinkRecord struct {
	port           bridgePortRef
	designatedPort bridgePortRef
}

type bridgeMacLinkRecord struct {
	port       bridgePortRef
	endpointID string
	method     string
}

type bridgeNodeSet map[string]struct{}

func (s bridgeNodeSet) add(v string) {
	v = strings.TrimSpace(v)
	if v == "" {
		return
	}
	s[v] = struct{}{}
}

func buildBridgeDomainModel(
	bridgeLinks []bridgeBridgeLinkRecord,
	macLinks []bridgeMacLinkRecord,
) bridgeDomainModel {
	model := bridgeDomainModel{domains: make([]*bridgeBroadcastDomain, 0)}
	if len(bridgeLinks) == 0 && len(macLinks) == 0 {
		return model
	}

	bblSegments := make([]*bridgeDomainSegment, 0)
	rootToNodes := make(map[string]bridgeNodeSet)

	for _, link := range bridgeLinks {
		designatedNodeID := strings.TrimSpace(link.designatedPort.deviceID)
		nodeID := strings.TrimSpace(link.port.deviceID)
		if designatedNodeID == "" || nodeID == "" {
			continue
		}

		added := false
		for _, segment := range bblSegments {
			if segment.containsPort(link.designatedPort) {
				segment.addPort(link.port)
				added = true
				break
			}
		}
		if !added {
			segment := newBridgeDomainSegment(link.designatedPort)
			segment.addPort(link.port)
			bblSegments = append(bblSegments, segment)
		}

		mergeRootDomainSets(rootToNodes, designatedNodeID, nodeID)
	}

	bmlSegments := make([]*bridgeDomainSegment, 0)
	for _, link := range macLinks {
		if strings.TrimSpace(link.port.deviceID) == "" || strings.TrimSpace(link.endpointID) == "" {
			continue
		}

		added := false
		for _, segment := range bblSegments {
			if segment.containsPort(link.port) {
				segment.addEndpoint(link.endpointID, link.method)
				added = true
				break
			}
		}
		if added {
			continue
		}
		for _, segment := range bmlSegments {
			if segment.containsPort(link.port) {
				segment.addEndpoint(link.endpointID, link.method)
				added = true
				break
			}
		}
		if added {
			continue
		}

		segment := newBridgeDomainSegment(link.port)
		segment.addEndpoint(link.endpointID, link.method)
		bmlSegments = append(bmlSegments, segment)
	}

	rootIDs := sortedStringKeys(rootToNodes)
	for _, rootID := range rootIDs {
		domain := &bridgeBroadcastDomain{
			bridges:  make(map[string]*bridgeDomainBridge),
			segments: make([]*bridgeDomainSegment, 0),
		}
		domain.bridges[rootID] = &bridgeDomainBridge{nodeID: rootID, root: true}
		for nodeID := range rootToNodes[rootID] {
			domain.bridges[nodeID] = &bridgeDomainBridge{nodeID: nodeID, root: false}
		}
		model.domains = append(model.domains, domain)
	}

	for _, segment := range bblSegments {
		for _, domain := range model.domains {
			if domain.loadSegment(segment) {
				break
			}
		}
	}

	for _, segment := range bmlSegments {
		inserted := false
		for _, domain := range model.domains {
			if domain.loadSegment(segment) {
				inserted = true
				break
			}
		}
		if inserted {
			continue
		}

		rootID := strings.TrimSpace(segment.designatedPort.deviceID)
		if rootID == "" {
			continue
		}
		domain := &bridgeBroadcastDomain{
			bridges: map[string]*bridgeDomainBridge{
				rootID: {nodeID: rootID, root: true},
			},
			segments: make([]*bridgeDomainSegment, 0, 1),
		}
		domain.loadSegment(segment)
		model.domains = append(model.domains, domain)
	}

	sort.SliceStable(model.domains, func(i, j int) bool {
		return model.domains[i].sortKey() < model.domains[j].sortKey()
	})
	for _, domain := range model.domains {
		domain.sortSegments()
	}

	return model
}

func mergeRootDomainSets(rootToNodes map[string]bridgeNodeSet, designatedNodeID, nodeID string) {
	designatedSet, designatedExists := rootToNodes[designatedNodeID]
	if designatedExists {
		designatedSet.add(nodeID)
		if nodeSet, ok := rootToNodes[nodeID]; ok {
			for id := range nodeSet {
				designatedSet.add(id)
			}
			delete(rootToNodes, nodeID)
		}
		rootToNodes[designatedNodeID] = designatedSet
		return
	}

	if nodeSet, nodeExists := rootToNodes[nodeID]; nodeExists {
		delete(rootToNodes, nodeID)
		nodeSet.add(nodeID)
		rootOfDesignated := findRootContaining(rootToNodes, designatedNodeID)
		if rootOfDesignated != "" {
			nodeSet.add(designatedNodeID)
			for id := range nodeSet {
				rootToNodes[rootOfDesignated].add(id)
			}
			return
		}
		rootToNodes[designatedNodeID] = nodeSet
		return
	}

	rootOfDesignated := findRootContaining(rootToNodes, designatedNodeID)
	if rootOfDesignated != "" {
		rootToNodes[rootOfDesignated].add(nodeID)
		return
	}

	set := make(bridgeNodeSet)
	set.add(nodeID)
	rootToNodes[designatedNodeID] = set
}

func findRootContaining(rootToNodes map[string]bridgeNodeSet, nodeID string) string {
	for rootID, set := range rootToNodes {
		if _, ok := set[nodeID]; ok {
			return rootID
		}
	}
	return ""
}

func newBridgeDomainSegment(designatedPort bridgePortRef) *bridgeDomainSegment {
	seg := &bridgeDomainSegment{
		designatedPort: designatedPort,
		ports:          make(map[string]bridgePortRef),
		endpointIDs:    make(map[string]struct{}),
		methods:        make(map[string]struct{}),
	}
	seg.addPort(designatedPort)
	return seg
}

func (s *bridgeDomainSegment) containsPort(port bridgePortRef) bool {
	if s == nil {
		return false
	}
	_, ok := s.ports[s.portIdentityKey(port)]
	return ok
}

func (s *bridgeDomainSegment) addPort(port bridgePortRef) {
	if s == nil {
		return
	}
	key := s.portIdentityKey(port)
	if key == "" {
		return
	}
	if existing, ok := s.ports[key]; ok {
		if existing.ifName == "" {
			existing.ifName = port.ifName
		}
		if existing.ifIndex == 0 && port.ifIndex > 0 {
			existing.ifIndex = port.ifIndex
		}
		if existing.bridgePort == "" {
			existing.bridgePort = port.bridgePort
		}
		if existing.vlanID == "" {
			existing.vlanID = port.vlanID
		}
		port = existing
	}
	s.ports[key] = port
}

func (s *bridgeDomainSegment) addEndpoint(endpointID, method string) {
	if s == nil {
		return
	}
	endpointID = strings.TrimSpace(endpointID)
	if endpointID == "" {
		return
	}
	s.endpointIDs[endpointID] = struct{}{}
	method = strings.ToLower(strings.TrimSpace(method))
	if method == "" {
		method = "fdb"
	}
	s.methods[method] = struct{}{}
}

func (s *bridgeDomainSegment) portIdentityKey(port bridgePortRef) string {
	nodeID := strings.TrimSpace(port.deviceID)
	bridgePort := strings.TrimSpace(port.bridgePort)
	if bridgePort == "" {
		if port.ifIndex > 0 {
			bridgePort = strconvItoa(port.ifIndex)
		} else {
			bridgePort = strings.TrimSpace(port.ifName)
		}
	}
	if nodeID == "" || bridgePort == "" {
		return ""
	}
	return nodeID + "|" + strings.ToLower(bridgePort)
}

func (s *bridgeDomainSegment) sortKey() string {
	return portSortKey(s.designatedPort) + "|" + strings.Join(sortedBridgePortSet(s.ports), ",")
}

func (d *bridgeBroadcastDomain) loadSegment(segment *bridgeDomainSegment) bool {
	if d == nil || segment == nil {
		return false
	}
	for _, port := range segment.ports {
		if _, ok := d.bridges[strings.TrimSpace(port.deviceID)]; ok {
			d.segments = append(d.segments, segment)
			return true
		}
	}
	return false
}

func (d *bridgeBroadcastDomain) sortKey() string {
	if d == nil {
		return ""
	}
	ids := make([]string, 0, len(d.bridges))
	for id := range d.bridges {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return strings.Join(ids, ",")
}

func (d *bridgeBroadcastDomain) sortSegments() {
	if d == nil {
		return
	}
	sort.SliceStable(d.segments, func(i, j int) bool {
		return d.segments[i].sortKey() < d.segments[j].sortKey()
	})
}

func sortedBridgePortSet(m map[string]bridgePortRef) []string {
	out := make([]string, 0, len(m))
	for _, port := range m {
		out = append(out, portSortKey(port))
	}
	sort.Strings(out)
	return out
}

func portSortKey(port bridgePortRef) string {
	return strings.Join([]string{
		strings.TrimSpace(port.deviceID),
		strings.ToLower(strings.TrimSpace(port.bridgePort)),
		strings.TrimSpace(port.ifName),
		strconvItoa(port.ifIndex),
		strings.TrimSpace(port.vlanID),
	}, "|")
}

func strconvItoa(v int) string {
	if v <= 0 {
		return ""
	}
	return strconv.Itoa(v)
}

func sortedStringKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
