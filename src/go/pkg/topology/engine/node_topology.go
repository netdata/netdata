// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"
)

var (
	pointToPointMaskIPv4 = netip.MustParseAddr("255.255.255.252")
	pointToPointMaskIPv6 = netip.MustParseAddr("ffff:ffff:ffff:ffff:ffff:ffff:ffff:fffd")
	loopbackMaskIPv4     = netip.MustParseAddr("255.255.255.255")
	loopbackMaskIPv6     = netip.MustParseAddr("ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff")
	loopbackAddrIPv4     = netip.MustParseAddr("127.0.0.1")
)

// NodeTopologyEntity mirrors the minimum node fields used by Enlinkd node topology logic.
type NodeTopologyEntity struct {
	ID        int
	Label     string
	SysObject string
	SysName   string
	Address   netip.Addr
}

// IPInterfaceTopologyEntity mirrors the minimum IP interface fields used by Enlinkd node topology logic.
type IPInterfaceTopologyEntity struct {
	ID              int
	NodeID          int
	IPAddress       netip.Addr
	NetMask         netip.Addr
	IsManaged       bool
	IsSnmpPrimary   bool
	IfIndex         int
	SnmpInterfaceID int
}

// SnmpInterfaceTopologyEntity mirrors the minimum SNMP interface fields used by Enlinkd node topology logic.
type SnmpInterfaceTopologyEntity struct {
	ID      int
	NodeID  int
	IfIndex int
	IfName  string
	IfDescr string
}

// SubNetwork mirrors Enlinkd SubNetwork behavior for node/IP membership.
type SubNetwork struct {
	network          netip.Addr
	netmask          netip.Addr
	nodeInterfaceMap map[int]map[netip.Addr]struct{}
}

// NewSubNetwork creates a subnet from one managed IP interface.
func NewSubNetwork(nodeID int, ip, netmask netip.Addr) (*SubNetwork, error) {
	if nodeID <= 0 {
		return nil, fmt.Errorf("node id is required")
	}
	if !ip.IsValid() {
		return nil, fmt.Errorf("ip is required")
	}
	if !netmask.IsValid() {
		return nil, fmt.Errorf("netmask is required")
	}
	network, ok := NetworkAddress(ip, netmask)
	if !ok {
		return nil, fmt.Errorf("cannot build network from ip %q and netmask %q", ip, netmask)
	}
	s := &SubNetwork{
		network:          network,
		netmask:          netmask,
		nodeInterfaceMap: map[int]map[netip.Addr]struct{}{},
	}
	s.nodeInterfaceMap[nodeID] = map[netip.Addr]struct{}{ip.Unmap(): {}}
	return s, nil
}

// Network returns the network address.
func (s *SubNetwork) Network() netip.Addr {
	if s == nil {
		return netip.Addr{}
	}
	return s.network
}

// Netmask returns the subnet mask.
func (s *SubNetwork) Netmask() netip.Addr {
	if s == nil {
		return netip.Addr{}
	}
	return s.netmask
}

// CIDR returns network/prefix format.
func (s *SubNetwork) CIDR() string {
	if s == nil || !s.network.IsValid() || !s.netmask.IsValid() {
		return ""
	}
	prefix, err := MaskToCIDRPrefix(s.netmask)
	if err != nil {
		return ""
	}
	return s.network.String() + "/" + strconv.Itoa(prefix)
}

// NetworkPrefix returns the CIDR prefix for the mask.
func (s *SubNetwork) NetworkPrefix() int {
	if s == nil {
		return 0
	}
	prefix, err := MaskToCIDRPrefix(s.netmask)
	if err != nil {
		return 0
	}
	return prefix
}

// IsIPv4Subnetwork reports if the subnet uses IPv4.
func (s *SubNetwork) IsIPv4Subnetwork() bool {
	return s != nil && s.network.IsValid() && s.network.Is4()
}

// NodeIDs returns sorted node IDs in the subnet.
func (s *SubNetwork) NodeIDs() []int {
	if s == nil || len(s.nodeInterfaceMap) == 0 {
		return nil
	}
	ids := make([]int, 0, len(s.nodeInterfaceMap))
	for nodeID := range s.nodeInterfaceMap {
		ids = append(ids, nodeID)
	}
	sort.Ints(ids)
	return ids
}

// Add adds one node/IP membership if the address is in range.
func (s *SubNetwork) Add(nodeID int, ip netip.Addr) bool {
	if s == nil || nodeID <= 0 || !ip.IsValid() || !s.IsInRange(ip) {
		return false
	}
	ip = ip.Unmap()
	if _, ok := s.nodeInterfaceMap[nodeID]; !ok {
		s.nodeInterfaceMap[nodeID] = map[netip.Addr]struct{}{}
	}
	if _, exists := s.nodeInterfaceMap[nodeID][ip]; exists {
		return false
	}
	s.nodeInterfaceMap[nodeID][ip] = struct{}{}
	return true
}

// Remove removes one node/IP membership.
func (s *SubNetwork) Remove(nodeID int, ip netip.Addr) bool {
	if s == nil || nodeID <= 0 || !ip.IsValid() {
		return false
	}
	ip = ip.Unmap()
	ips, ok := s.nodeInterfaceMap[nodeID]
	if !ok {
		return false
	}
	if _, exists := ips[ip]; !exists {
		return false
	}
	delete(ips, ip)
	if len(ips) == 0 {
		delete(s.nodeInterfaceMap, nodeID)
	}
	return true
}

// IsInRange reports if ip belongs to this subnet.
func (s *SubNetwork) IsInRange(ip netip.Addr) bool {
	if s == nil || !ip.IsValid() || !s.network.IsValid() || !s.netmask.IsValid() {
		return false
	}
	return InSameNetwork(ip.Unmap(), s.network, s.netmask)
}

// HasDuplicatedAddress reports true when the same address exists under multiple entries.
func (s *SubNetwork) HasDuplicatedAddress() bool {
	if s == nil {
		return false
	}
	seen := make(map[netip.Addr]struct{})
	for _, addresses := range s.nodeInterfaceMap {
		for addr := range addresses {
			if _, ok := seen[addr]; ok {
				return true
			}
			seen[addr] = struct{}{}
		}
	}
	return false
}

func (s *SubNetwork) clone() *SubNetwork {
	if s == nil {
		return nil
	}
	out := &SubNetwork{
		network:          s.network,
		netmask:          s.netmask,
		nodeInterfaceMap: make(map[int]map[netip.Addr]struct{}, len(s.nodeInterfaceMap)),
	}
	for nodeID, ips := range s.nodeInterfaceMap {
		copySet := make(map[netip.Addr]struct{}, len(ips))
		for ip := range ips {
			copySet[ip] = struct{}{}
		}
		out.nodeInterfaceMap[nodeID] = copySet
	}
	return out
}

func (s *SubNetwork) key() string {
	if s == nil {
		return ""
	}
	return subnetKey(s.network, s.netmask)
}

func subnetKey(network, netmask netip.Addr) string {
	if !network.IsValid() || !netmask.IsValid() {
		return ""
	}
	return network.String() + "|" + netmask.String()
}

func sortedSubnetworkKeys(subnets map[string]*SubNetwork) []string {
	keys := make([]string, 0, len(subnets))
	for key := range subnets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// NodeTopologyService ports Enlinkd NodeTopologyServiceImpl logic.
type NodeTopologyService struct {
	nodes []NodeTopologyEntity
	ips   []IPInterfaceTopologyEntity
	snmp  []SnmpInterfaceTopologyEntity
}

// NewNodeTopologyService builds a deterministic node topology service snapshot.
func NewNodeTopologyService(nodes []NodeTopologyEntity, ips []IPInterfaceTopologyEntity, snmp []SnmpInterfaceTopologyEntity) *NodeTopologyService {
	out := &NodeTopologyService{
		nodes: append([]NodeTopologyEntity(nil), nodes...),
		ips:   append([]IPInterfaceTopologyEntity(nil), ips...),
		snmp:  append([]SnmpInterfaceTopologyEntity(nil), snmp...),
	}

	sort.Slice(out.nodes, func(i, j int) bool {
		if out.nodes[i].ID != out.nodes[j].ID {
			return out.nodes[i].ID < out.nodes[j].ID
		}
		return out.nodes[i].Label < out.nodes[j].Label
	})
	sort.Slice(out.ips, func(i, j int) bool {
		if out.ips[i].ID != out.ips[j].ID {
			return out.ips[i].ID < out.ips[j].ID
		}
		if out.ips[i].NodeID != out.ips[j].NodeID {
			return out.ips[i].NodeID < out.ips[j].NodeID
		}
		return out.ips[i].IPAddress.String() < out.ips[j].IPAddress.String()
	})
	sort.Slice(out.snmp, func(i, j int) bool {
		if out.snmp[i].ID != out.snmp[j].ID {
			return out.snmp[i].ID < out.snmp[j].ID
		}
		if out.snmp[i].NodeID != out.snmp[j].NodeID {
			return out.snmp[i].NodeID < out.snmp[j].NodeID
		}
		return out.snmp[i].IfIndex < out.snmp[j].IfIndex
	})
	return out
}

// FindAllNode returns all node entities.
func (s *NodeTopologyService) FindAllNode() []NodeTopologyEntity {
	if s == nil {
		return nil
	}
	return append([]NodeTopologyEntity(nil), s.nodes...)
}

// FindAllIP returns all IP interface entities.
func (s *NodeTopologyService) FindAllIP() []IPInterfaceTopologyEntity {
	if s == nil {
		return nil
	}
	return append([]IPInterfaceTopologyEntity(nil), s.ips...)
}

// FindAllSnmp returns all SNMP interface entities.
func (s *NodeTopologyService) FindAllSnmp() []SnmpInterfaceTopologyEntity {
	if s == nil {
		return nil
	}
	return append([]SnmpInterfaceTopologyEntity(nil), s.snmp...)
}

// FindAllSubNetwork ports NodeTopologyServiceImpl.findAllSubNetwork().
func (s *NodeTopologyService) FindAllSubNetwork() []*SubNetwork {
	if s == nil {
		return nil
	}
	byKey := make(map[string]*SubNetwork)
	keys := make([]string, 0)

	for _, ip := range s.ips {
		if !ip.IsManaged || !ip.IPAddress.IsValid() || !ip.NetMask.IsValid() {
			continue
		}
		network, ok := NetworkAddress(ip.IPAddress, ip.NetMask)
		if !ok {
			continue
		}
		key := subnetKey(network, ip.NetMask)
		subnet := byKey[key]
		if subnet == nil {
			created, err := NewSubNetwork(ip.NodeID, ip.IPAddress, ip.NetMask)
			if err != nil {
				continue
			}
			byKey[key] = created
			keys = append(keys, key)
			continue
		}
		subnet.Add(ip.NodeID, ip.IPAddress)
	}

	for _, ip := range s.ips {
		if !ip.IsManaged || !ip.IPAddress.IsValid() || ip.NetMask.IsValid() {
			continue
		}
		for _, key := range keys {
			subnet := byKey[key]
			if subnet == nil {
				continue
			}
			subnet.Add(ip.NodeID, ip.IPAddress)
		}
	}

	sorted := sortedSubnetworkKeys(byKey)
	result := make([]*SubNetwork, 0, len(sorted))
	for _, key := range sorted {
		subnet := byKey[key]
		if subnet == nil {
			continue
		}
		result = append(result, subnet.clone())
	}
	return result
}

// FindAllLegalSubNetwork ports NodeTopologyServiceImpl.findAllLegalSubNetwork().
func (s *NodeTopologyService) FindAllLegalSubNetwork() []*SubNetwork {
	all := s.FindAllSubNetwork()
	if len(all) == 0 {
		return nil
	}
	result := make([]*SubNetwork, 0, len(all))
	for _, subnet := range all {
		if subnet == nil || subnet.HasDuplicatedAddress() {
			continue
		}
		if InSameNetwork(subnet.Network(), loopbackAddrIPv4, subnet.Netmask()) {
			continue
		}
		result = append(result, subnet)
	}
	return result
}

// FindSubNetworkByNetworkPrefixLessThen ports NodeTopologyServiceImpl.findSubNetworkByNetworkPrefixLessThen().
func (s *NodeTopologyService) FindSubNetworkByNetworkPrefixLessThen(ipv4prefix, ipv6prefix int) []*SubNetwork {
	legal := s.FindAllLegalSubNetwork()
	if len(legal) == 0 {
		return nil
	}
	result := make([]*SubNetwork, 0, len(legal))
	for _, subnet := range legal {
		if subnet == nil {
			continue
		}
		prefix := subnet.NetworkPrefix()
		if subnet.IsIPv4Subnetwork() {
			if prefix < ipv4prefix {
				result = append(result, subnet)
			}
			continue
		}
		if prefix < ipv6prefix {
			result = append(result, subnet)
		}
	}
	return result
}

// FindAllPointToPointSubNetwork ports NodeTopologyServiceImpl.findAllPointToPointSubNetwork().
func (s *NodeTopologyService) FindAllPointToPointSubNetwork() []*SubNetwork {
	all := s.FindAllSubNetwork()
	if len(all) == 0 {
		return nil
	}
	result := make([]*SubNetwork, 0, len(all))
	for _, subnet := range all {
		if subnet == nil {
			continue
		}
		if IsPointToPointMask(subnet.Netmask()) {
			result = append(result, subnet)
		}
	}
	return result
}

// FindAllLegalPointToPointSubNetwork ports NodeTopologyServiceImpl.findAllLegalPointToPointSubNetwork().
func (s *NodeTopologyService) FindAllLegalPointToPointSubNetwork() []*SubNetwork {
	legal := s.FindAllLegalSubNetwork()
	if len(legal) == 0 {
		return nil
	}
	result := make([]*SubNetwork, 0, len(legal))
	for _, subnet := range legal {
		if subnet == nil {
			continue
		}
		if IsPointToPointMask(subnet.Netmask()) && len(subnet.NodeIDs()) == 2 {
			result = append(result, subnet)
		}
	}
	return result
}

// FindAllLoopbacks ports NodeTopologyServiceImpl.findAllLoopbacks().
func (s *NodeTopologyService) FindAllLoopbacks() []*SubNetwork {
	all := s.FindAllSubNetwork()
	if len(all) == 0 {
		return nil
	}
	result := make([]*SubNetwork, 0, len(all))
	for _, subnet := range all {
		if subnet == nil {
			continue
		}
		if IsLoopbackMask(subnet.Netmask()) {
			result = append(result, subnet)
		}
	}
	return result
}

// FindAllLegalLoopbacks ports NodeTopologyServiceImpl.findAllLegalLoopbacks().
func (s *NodeTopologyService) FindAllLegalLoopbacks() []*SubNetwork {
	all := s.FindAllSubNetwork()
	if len(all) == 0 {
		return nil
	}
	result := make([]*SubNetwork, 0, len(all))
	for _, subnet := range all {
		if subnet == nil {
			continue
		}
		if IsLoopbackMask(subnet.Netmask()) && len(subnet.NodeIDs()) == 1 {
			result = append(result, subnet)
		}
	}
	return result
}

// GetNodeIDPriorityMap ports NodeTopologyServiceImpl.getNodeidPriorityMap().
func (s *NodeTopologyService) GetNodeIDPriorityMap() map[int]int {
	priorityMap := make(map[int]int)
	legal := s.FindAllLegalSubNetwork()
	remaining := make(map[string]*SubNetwork)
	for _, subnet := range legal {
		if subnet == nil || len(subnet.NodeIDs()) <= 1 {
			continue
		}
		remaining[subnet.key()] = subnet
	}

	priority := 0
	for len(remaining) > 0 {
		start := getNextSubnetwork(remaining)
		if start == nil {
			break
		}
		delete(remaining, start.key())
		for _, nodeID := range start.NodeIDs() {
			priorityMap[nodeID] = priority
		}
		priority = getConnectedSubnets(start, remaining, priorityMap, priority+1)
	}
	return priorityMap
}

func getNextSubnetwork(subnets map[string]*SubNetwork) *SubNetwork {
	var selected *SubNetwork
	for _, subnet := range subnets {
		if subnet == nil {
			continue
		}
		if selected == nil {
			selected = subnet
			continue
		}
		selectedSize := len(selected.NodeIDs())
		subnetSize := len(subnet.NodeIDs())
		if selectedSize < subnetSize {
			selected = subnet
			continue
		}
		if selectedSize != subnetSize {
			continue
		}
		if compareAddr(selected.Network(), subnet.Network()) > 0 {
			selected = subnet
		}
	}
	return selected
}

func getConnectedSubnets(starting *SubNetwork, subnetworks map[string]*SubNetwork, priorityMap map[int]int, priority int) int {
	if starting == nil || len(subnetworks) == 0 {
		return priority
	}

	downlevels := make([]*SubNetwork, 0)
	for _, subnet := range subnetworks {
		if subnet == nil {
			continue
		}
		if hasNodeIntersection(starting, subnet) {
			downlevels = append(downlevels, subnet)
		}
	}
	for _, subnet := range downlevels {
		delete(subnetworks, subnet.key())
	}

	for _, subnet := range downlevels {
		if subnet == nil {
			continue
		}
		addingNodes := make([]int, 0)
		for _, nodeID := range subnet.NodeIDs() {
			if _, exists := priorityMap[nodeID]; exists {
				continue
			}
			addingNodes = append(addingNodes, nodeID)
		}
		if len(addingNodes) == 0 {
			continue
		}
		for _, nodeID := range addingNodes {
			priorityMap[nodeID] = priority
		}
		priority++
	}

	if len(downlevels) > 0 && len(subnetworks) > 0 {
		for _, level := range downlevels {
			priority = getConnectedSubnets(level, subnetworks, priorityMap, priority)
		}
	}
	return priority
}

func hasNodeIntersection(left, right *SubNetwork) bool {
	if left == nil || right == nil {
		return false
	}
	rightIDs := make(map[int]struct{}, len(right.nodeInterfaceMap))
	for nodeID := range right.nodeInterfaceMap {
		rightIDs[nodeID] = struct{}{}
	}
	for nodeID := range left.nodeInterfaceMap {
		if _, ok := rightIDs[nodeID]; ok {
			return true
		}
	}
	return false
}

func compareAddr(a, b netip.Addr) int {
	ab := addrBytes(a)
	bb := addrBytes(b)
	if len(ab) != len(bb) {
		if len(ab) < len(bb) {
			return -1
		}
		return 1
	}
	for i := range ab {
		if ab[i] < bb[i] {
			return -1
		}
		if ab[i] > bb[i] {
			return 1
		}
	}
	return 0
}

// IsPointToPointMask ports InetAddressUtils.isPointToPointMask().
func IsPointToPointMask(mask netip.Addr) bool {
	mask = mask.Unmap()
	return mask == pointToPointMaskIPv4 || mask == pointToPointMaskIPv6
}

// IsLoopbackMask ports InetAddressUtils.isLoopbackMask().
func IsLoopbackMask(mask netip.Addr) bool {
	mask = mask.Unmap()
	return mask == loopbackMaskIPv4 || mask == loopbackMaskIPv6
}

// InSameNetwork ports InetAddressUtils.inSameNetwork().
func InSameNetwork(addr1, addr2, mask netip.Addr) bool {
	addr1 = addr1.Unmap()
	addr2 = addr2.Unmap()
	mask = mask.Unmap()
	if !addr1.IsValid() || !addr2.IsValid() || !mask.IsValid() {
		return false
	}
	if addr1.Is4() != addr2.Is4() || addr1.Is4() != mask.Is4() {
		return false
	}

	ab := addrBytes(addr1)
	bb := addrBytes(addr2)
	mb := addrBytes(mask)
	if len(ab) != len(bb) || len(ab) != len(mb) {
		return false
	}
	for i := range ab {
		if (ab[i] & mb[i]) != (bb[i] & mb[i]) {
			return false
		}
	}
	return true
}

// NetworkAddress returns ip&mask for matching IP families.
func NetworkAddress(ip, mask netip.Addr) (netip.Addr, bool) {
	ip = ip.Unmap()
	mask = mask.Unmap()
	if !ip.IsValid() || !mask.IsValid() || ip.Is4() != mask.Is4() {
		return netip.Addr{}, false
	}
	ib := addrBytes(ip)
	mb := addrBytes(mask)
	if len(ib) != len(mb) {
		return netip.Addr{}, false
	}
	out := make([]byte, len(ib))
	for i := range ib {
		out[i] = ib[i] & mb[i]
	}
	addr, ok := netip.AddrFromSlice(out)
	if !ok {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

// MaskToCIDRPrefix ports InetAddressUtils.convertInetAddressMaskToCidr().
func MaskToCIDRPrefix(mask netip.Addr) (int, error) {
	mask = mask.Unmap()
	if !mask.IsValid() {
		return 0, fmt.Errorf("invalid mask")
	}
	foundZero := false
	cidr := 0
	for _, value := range addrBytes(mask) {
		k := int(value)
		if foundZero && k != 0 {
			return 0, fmt.Errorf("invalid mask %q", mask)
		}
		switch k {
		case 255:
			cidr += 8
		case 254:
			cidr += 7
			foundZero = true
		case 252:
			cidr += 6
			foundZero = true
		case 248:
			cidr += 5
			foundZero = true
		case 240:
			cidr += 4
			foundZero = true
		case 224:
			cidr += 3
			foundZero = true
		case 192:
			cidr += 2
			foundZero = true
		case 128:
			cidr += 1
			foundZero = true
		case 0:
			foundZero = true
		default:
			return 0, fmt.Errorf("invalid mask %q", mask)
		}
	}
	return cidr, nil
}

func addrBytes(addr netip.Addr) []byte {
	if !addr.IsValid() {
		return nil
	}
	if addr.Is4() {
		a := addr.As4()
		return a[:]
	}
	a := addr.As16()
	return a[:]
}

// NetworkRouterTopology mirrors Enlinkd NetworkRouterTopologyUpdater graph payload.
type NetworkRouterTopology struct {
	Vertices      []NetworkRouterVertex
	Edges         []NetworkRouterEdge
	DefaultVertex string
}

// NetworkRouterVertex is one node or subnet vertex in the network-router topology.
type NetworkRouterVertex struct {
	ID       string
	Label    string
	Address  string
	IconKey  string
	NodeID   int
	ToolTip  string
	IsSubnet bool
}

// NetworkRouterPort is one endpoint port in the network-router topology.
type NetworkRouterPort struct {
	ID      string
	Vertex  string
	IPID    int
	IfIndex int
	IfName  string
	Addr    string
	ToolTip string
}

// NetworkRouterEdge is one link in the network-router topology.
type NetworkRouterEdge struct {
	ID         string
	SourcePort NetworkRouterPort
	TargetPort NetworkRouterPort
}

// BuildNetworkRouterTopology ports NetworkRouterTopologyUpdater.buildTopology().
func BuildNetworkRouterTopology(service *NodeTopologyService, ipv4prefix, ipv6prefix int) NetworkRouterTopology {
	if service == nil {
		return NetworkRouterTopology{}
	}

	result := NetworkRouterTopology{}
	vertexByID := make(map[string]NetworkRouterVertex)
	edgeByID := make(map[string]NetworkRouterEdge)

	nodeMap := make(map[int]NodeTopologyEntity)
	for _, node := range service.FindAllNode() {
		nodeMap[node.ID] = node
	}
	ipPrimaryMap := getIPPrimaryMap(service.FindAllIP())
	ipTable := getIPInterfaceTable(service.FindAllIP())
	snmpByID := make(map[int]SnmpInterfaceTopologyEntity)
	for _, snmp := range service.FindAllSnmp() {
		snmpByID[snmp.ID] = snmp
	}

	addVertex := func(v NetworkRouterVertex) {
		if strings.TrimSpace(v.ID) == "" {
			return
		}
		if _, exists := vertexByID[v.ID]; exists {
			return
		}
		vertexByID[v.ID] = v
	}
	addEdge := func(e NetworkRouterEdge) {
		if strings.TrimSpace(e.ID) == "" {
			return
		}
		if _, exists := edgeByID[e.ID]; exists {
			return
		}
		edgeByID[e.ID] = e
	}

	for _, node := range service.FindAllNode() {
		primary := ipPrimaryMap[node.ID]
		addVertex(createNodeVertex(node, primary))
	}

	for _, subnet := range service.FindAllLegalPointToPointSubNetwork() {
		if subnet == nil {
			continue
		}
		nodeIDs := subnet.NodeIDs()
		if len(nodeIDs) < 2 {
			continue
		}
		sourceNodeID := nodeIDs[0]
		targetNodeID := nodeIDs[1]

		source, sourceOK := nodeMap[sourceNodeID]
		target, targetOK := nodeMap[targetNodeID]
		if !sourceOK || !targetOK {
			continue
		}

		sourceIP, sourceIPOK := firstIPInSubnet(ipTable[sourceNodeID], subnet)
		targetIP, targetIPOK := firstIPInSubnet(ipTable[targetNodeID], subnet)
		if !sourceIPOK || !targetIPOK {
			continue
		}

		sourcePort := createNodePort(createNodeVertex(source, ipPrimaryMap[source.ID]), sourceIP, snmpByID[sourceIP.SnmpInterfaceID])
		targetPort := createNodePort(createNodeVertex(target, ipPrimaryMap[target.ID]), targetIP, snmpByID[targetIP.SnmpInterfaceID])
		addEdge(NetworkRouterEdge{
			ID:         strconv.Itoa(sourceIP.ID),
			SourcePort: sourcePort,
			TargetPort: targetPort,
		})
	}

	for _, subnet := range service.FindSubNetworkByNetworkPrefixLessThen(ipv4prefix, ipv6prefix) {
		if subnet == nil {
			continue
		}
		networkVertex := createNetworkVertex(subnet)
		addVertex(networkVertex)
		for _, targetNodeID := range subnet.NodeIDs() {
			targetNode, ok := nodeMap[targetNodeID]
			if !ok {
				continue
			}
			targetIP, found := firstIPInSubnet(ipTable[targetNodeID], subnet)
			if !found {
				continue
			}
			targetVertex := createNodeVertex(targetNode, ipPrimaryMap[targetNodeID])
			sourcePort := createNetworkPort(networkVertex, targetIP)
			targetPort := createNodePort(targetVertex, targetIP, snmpByID[targetIP.SnmpInterfaceID])
			addEdge(NetworkRouterEdge{
				ID:         strconv.Itoa(targetIP.ID),
				SourcePort: sourcePort,
				TargetPort: targetPort,
			})
		}
	}

	if len(ipPrimaryMap) > 0 {
		nodeIDs := make([]int, 0, len(ipPrimaryMap))
		for nodeID := range ipPrimaryMap {
			nodeIDs = append(nodeIDs, nodeID)
		}
		sort.Ints(nodeIDs)
		for _, nodeID := range nodeIDs {
			if nodeID <= 0 {
				continue
			}
			result.DefaultVertex = strconv.Itoa(nodeID)
			break
		}
	}

	result.Vertices = sortedNetworkRouterVertices(vertexByID)
	result.Edges = sortedNetworkRouterEdges(edgeByID)
	return result
}

func createNodeVertex(node NodeTopologyEntity, primary IPInterfaceTopologyEntity) NetworkRouterVertex {
	address := ""
	if primary.IPAddress.IsValid() {
		address = primary.IPAddress.String()
	} else if node.Address.IsValid() {
		address = node.Address.String()
	}
	label := strings.TrimSpace(node.Label)
	if label == "" {
		label = strconv.Itoa(node.ID)
	}
	vertex := NetworkRouterVertex{
		ID:      strconv.Itoa(node.ID),
		Label:   label,
		Address: address,
		IconKey: "node",
		NodeID:  node.ID,
	}
	vertex.ToolTip = "Node: " + label
	if address != "" {
		vertex.ToolTip += " (" + address + ")"
	}
	return vertex
}

func createNodePort(vertex NetworkRouterVertex, ip IPInterfaceTopologyEntity, snmp SnmpInterfaceTopologyEntity) NetworkRouterPort {
	port := NetworkRouterPort{
		ID:     ip.IPAddress.String(),
		Vertex: vertex.ID,
		IPID:   ip.ID,
		Addr:   ip.IPAddress.String(),
	}
	if snmp.ID > 0 {
		port.IfIndex = snmp.IfIndex
		port.IfName = snmp.IfName
	}
	port.ToolTip = "Port " + port.Addr
	if port.IfName != "" {
		port.ToolTip += " (" + port.IfName + ")"
	}
	return port
}

func createNetworkPort(vertex NetworkRouterVertex, target IPInterfaceTopologyEntity) NetworkRouterPort {
	addr := "to: " + target.IPAddress.String()
	return NetworkRouterPort{
		ID:      vertex.ID + "to:" + target.IPAddress.String(),
		Vertex:  vertex.ID,
		IPID:    target.ID,
		Addr:    addr,
		ToolTip: "Port " + addr,
	}
}

func createNetworkVertex(network *SubNetwork) NetworkRouterVertex {
	cidr := ""
	nodeIDs := ""
	if network != nil {
		cidr = network.CIDR()
		nodeIDValues := network.NodeIDs()
		parts := make([]string, 0, len(nodeIDValues))
		for _, nodeID := range nodeIDValues {
			parts = append(parts, strconv.Itoa(nodeID))
		}
		nodeIDs = strings.Join(parts, ",")
	}
	return NetworkRouterVertex{
		ID:       cidr,
		Label:    cidr,
		Address:  cidr,
		IconKey:  "cloud",
		ToolTip:  "SubNetwork: " + cidr + ", Nodeids:[" + nodeIDs + "]",
		IsSubnet: true,
	}
}

func getIPPrimaryMap(ips []IPInterfaceTopologyEntity) map[int]IPInterfaceTopologyEntity {
	primary := make(map[int]IPInterfaceTopologyEntity)
	for _, ip := range ips {
		if ip.NodeID <= 0 || !ip.IPAddress.IsValid() {
			continue
		}
		current, exists := primary[ip.NodeID]
		if !exists {
			primary[ip.NodeID] = ip
			continue
		}
		if ip.IsSnmpPrimary {
			primary[ip.NodeID] = ip
			continue
		}
		primary[ip.NodeID] = current
	}
	return primary
}

func getIPInterfaceTable(ips []IPInterfaceTopologyEntity) map[int][]IPInterfaceTopologyEntity {
	table := make(map[int][]IPInterfaceTopologyEntity)
	for _, ip := range ips {
		if ip.NodeID <= 0 || !ip.IPAddress.IsValid() {
			continue
		}
		table[ip.NodeID] = append(table[ip.NodeID], ip)
	}
	for nodeID := range table {
		sort.Slice(table[nodeID], func(i, j int) bool {
			if table[nodeID][i].ID != table[nodeID][j].ID {
				return table[nodeID][i].ID < table[nodeID][j].ID
			}
			return compareAddr(table[nodeID][i].IPAddress, table[nodeID][j].IPAddress) < 0
		})
	}
	return table
}

func firstIPInSubnet(ips []IPInterfaceTopologyEntity, subnet *SubNetwork) (IPInterfaceTopologyEntity, bool) {
	for _, ip := range ips {
		if subnet.IsInRange(ip.IPAddress) {
			return ip, true
		}
	}
	return IPInterfaceTopologyEntity{}, false
}

func sortedNetworkRouterVertices(values map[string]NetworkRouterVertex) []NetworkRouterVertex {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]NetworkRouterVertex, 0, len(keys))
	for _, key := range keys {
		result = append(result, values[key])
	}
	return result
}

func sortedNetworkRouterEdges(values map[string]NetworkRouterEdge) []NetworkRouterEdge {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]NetworkRouterEdge, 0, len(keys))
	for _, key := range keys {
		result = append(result, values[key])
	}
	return result
}
