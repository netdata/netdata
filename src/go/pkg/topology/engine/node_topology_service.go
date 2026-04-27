// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import "sort"

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
