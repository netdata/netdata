// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"sort"
	"strconv"
	"strings"
)

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
			ID:         sourcePort.Vertex + keySep + sourcePort.ID + "->" + targetPort.Vertex + keySep + targetPort.ID,
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
				ID:         sourcePort.Vertex + keySep + sourcePort.ID + "->" + targetPort.Vertex + keySep + targetPort.ID,
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
