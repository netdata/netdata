// SPDX-License-Identifier: GPL-3.0-or-later

package projector

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

type bridgeDomainSegmentIndex struct {
	byPort   map[string]*bridgeDomainSegment
	position map[*bridgeDomainSegment]int
}

type bridgeBridgeLinkRecord struct {
	port           bridgePortRef
	designatedPort bridgePortRef
	method         string
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
	bblIndex := newBridgeDomainSegmentIndex()
	rootToNodes := make(map[string]bridgeNodeSet)

	for _, link := range bridgeLinks {
		designatedNodeID := strings.TrimSpace(link.designatedPort.deviceID)
		nodeID := strings.TrimSpace(link.port.deviceID)
		if designatedNodeID == "" || nodeID == "" {
			continue
		}

		if segment := bblIndex.segmentForPort(link.designatedPort); segment != nil {
			bblIndex.addPort(segment, link.port)
		} else {
			segment := newBridgeDomainSegment(link.designatedPort)
			segment.addPort(link.port)
			bblSegments = append(bblSegments, segment)
			bblIndex.addSegment(segment)
		}

		mergeRootDomainSets(rootToNodes, designatedNodeID, nodeID)
	}

	bmlSegments := make([]*bridgeDomainSegment, 0)
	bmlIndex := newBridgeDomainSegmentIndex()
	vlanAliases := buildBridgeVLANAliasIndex(macLinks)
	for _, link := range macLinks {
		if strings.TrimSpace(link.port.deviceID) == "" || strings.TrimSpace(link.endpointID) == "" {
			continue
		}

		if segment := bblIndex.segmentForPort(link.port); segment != nil {
			segment.addEndpoint(link.endpointID, link.method)
			continue
		}
		if segment := bblIndex.segmentForKey(vlanAliases.uniqueAliasKey(link.port)); segment != nil {
			segment.addEndpoint(link.endpointID, link.method)
			continue
		}
		if segment := bmlIndex.segmentForPort(link.port); segment != nil {
			segment.addEndpoint(link.endpointID, link.method)
			continue
		}

		segment := newBridgeDomainSegment(link.port)
		segment.addEndpoint(link.endpointID, link.method)
		bmlSegments = append(bmlSegments, segment)
		bmlIndex.addSegment(segment)
	}

	rootIDs := sortedStringKeys(rootToNodes)
	domainByBridge := make(map[string]*bridgeBroadcastDomain)
	domainPosition := make(map[*bridgeBroadcastDomain]int)
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
		domainPosition[domain] = len(model.domains) - 1
		indexBridgeDomain(domainByBridge, domain)
	}

	for _, segment := range bblSegments {
		if domain := bridgeDomainForSegment(domainByBridge, domainPosition, segment); domain != nil {
			domain.segments = append(domain.segments, segment)
		}
	}

	for _, segment := range bmlSegments {
		if domain := bridgeDomainForSegment(domainByBridge, domainPosition, segment); domain != nil {
			domain.segments = append(domain.segments, segment)
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
		domain.segments = append(domain.segments, segment)
		model.domains = append(model.domains, domain)
		domainPosition[domain] = len(model.domains) - 1
		indexBridgeDomain(domainByBridge, domain)
	}

	sort.SliceStable(model.domains, func(i, j int) bool {
		return model.domains[i].sortKey() < model.domains[j].sortKey()
	})
	for _, domain := range model.domains {
		domain.sortSegments()
	}

	return model
}

func indexBridgeDomain(index map[string]*bridgeBroadcastDomain, domain *bridgeBroadcastDomain) {
	if domain == nil {
		return
	}
	for deviceID := range domain.bridges {
		deviceID = strings.TrimSpace(deviceID)
		if deviceID == "" || index[deviceID] != nil {
			continue
		}
		index[deviceID] = domain
	}
}

func bridgeDomainForSegment(
	index map[string]*bridgeBroadcastDomain,
	position map[*bridgeBroadcastDomain]int,
	segment *bridgeDomainSegment,
) *bridgeBroadcastDomain {
	if segment == nil {
		return nil
	}
	var selected *bridgeBroadcastDomain
	for _, port := range segment.ports {
		domain := index[strings.TrimSpace(port.deviceID)]
		if domain == nil {
			continue
		}
		if selected == nil || position[domain] < position[selected] {
			selected = domain
		}
	}
	return selected
}

func mergeRootDomainSets(rootToNodes map[string]bridgeNodeSet, designatedNodeID, nodeID string) {
	designatedNodeID = strings.TrimSpace(designatedNodeID)
	nodeID = strings.TrimSpace(nodeID)
	if designatedNodeID == "" || nodeID == "" {
		return
	}

	targetRoot := findRootForNode(rootToNodes, designatedNodeID)
	if targetRoot == "" {
		targetRoot = designatedNodeID
	}
	targetSet := rootToNodes[targetRoot]
	if targetSet == nil {
		targetSet = make(bridgeNodeSet)
	}
	if designatedNodeID != targetRoot {
		targetSet.add(designatedNodeID)
	}

	sourceRoot := findRootForNode(rootToNodes, nodeID)
	if sourceRoot != "" && sourceRoot != targetRoot {
		if sourceSet, ok := rootToNodes[sourceRoot]; ok {
			for id := range sourceSet {
				targetSet.add(id)
			}
			delete(rootToNodes, sourceRoot)
		}
		targetSet.add(sourceRoot)
	}
	if nodeID != targetRoot {
		targetSet.add(nodeID)
	}

	rootToNodes[targetRoot] = targetSet
}

func findRootForNode(rootToNodes map[string]bridgeNodeSet, nodeID string) string {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return ""
	}
	if _, ok := rootToNodes[nodeID]; ok {
		return nodeID
	}
	if rootID := findRootContaining(rootToNodes, nodeID); rootID != "" {
		return rootID
	}
	return ""
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

func newBridgeDomainSegmentIndex() *bridgeDomainSegmentIndex {
	return &bridgeDomainSegmentIndex{
		byPort:   make(map[string]*bridgeDomainSegment),
		position: make(map[*bridgeDomainSegment]int),
	}
}

func (i *bridgeDomainSegmentIndex) addSegment(segment *bridgeDomainSegment) {
	if i == nil || segment == nil {
		return
	}
	if _, ok := i.position[segment]; !ok {
		i.position[segment] = len(i.position)
	}
	for _, port := range segment.ports {
		i.indexPort(segment, port)
	}
}

func (i *bridgeDomainSegmentIndex) addPort(segment *bridgeDomainSegment, port bridgePortRef) {
	if i == nil || segment == nil {
		return
	}
	segment.addPort(port)
	i.indexPort(segment, port)
}

func (i *bridgeDomainSegmentIndex) segmentForPort(port bridgePortRef) *bridgeDomainSegment {
	if i == nil {
		return nil
	}
	return i.segmentForKey(bridgeDomainPortIdentityKey(port))
}

func (i *bridgeDomainSegmentIndex) segmentForKey(key string) *bridgeDomainSegment {
	if i == nil || key == "" {
		return nil
	}
	return i.byPort[key]
}

func (i *bridgeDomainSegmentIndex) indexPort(segment *bridgeDomainSegment, port bridgePortRef) {
	key := bridgeDomainPortIdentityKey(port)
	if key == "" {
		return
	}
	existing := i.byPort[key]
	if existing == nil || i.position[segment] < i.position[existing] {
		i.byPort[key] = segment
	}
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
		if existing.fdbDomainID == "" {
			existing.fdbDomainID = port.fdbDomainID
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
	return bridgeDomainPortIdentityKey(port)
}

func bridgeDomainPortIdentityKey(port bridgePortRef) string {
	return bridgePortRefKey(port, false, true)
}

func (s *bridgeDomainSegment) sortKey() string {
	return portSortKey(s.designatedPort) + keySep + strings.Join(sortedBridgePortSet(s.ports), ",")
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
		bridgePortForwardingDomain(port),
	}, keySep)
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
