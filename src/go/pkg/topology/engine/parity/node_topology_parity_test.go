// SPDX-License-Identifier: GPL-3.0-or-later

package parity

import (
	"encoding/json"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/topology/engine"
	"github.com/stretchr/testify/require"
)

type nodeTopologyFixtureDocument struct {
	SchemaVersion int                             `json:"schema_version"`
	Scenarios     map[string]nodeTopologyScenario `json:"scenarios"`
}

type nodeTopologyScenario struct {
	BuilderClass string                          `json:"builder_class"`
	Getters      []string                        `json:"getters"`
	Nodes        []nodeTopologyFixtureNode       `json:"nodes"`
	IPs          []nodeTopologyFixtureIP         `json:"ips"`
	SNMP         []nodeTopologyFixtureSNMP       `json:"snmp"`
	Expect       nodeTopologyFixtureExpectations `json:"expect"`
}

type nodeTopologyFixtureNode struct {
	ID        int    `json:"id"`
	Label     string `json:"label"`
	SysObject string `json:"sys_object"`
	SysName   string `json:"sys_name"`
	Address   string `json:"address"`
}

type nodeTopologyFixtureIP struct {
	ID              int    `json:"id"`
	NodeID          int    `json:"node_id"`
	IPAddress       string `json:"ip_address"`
	Netmask         string `json:"netmask"`
	IsManaged       bool   `json:"is_managed"`
	IsSnmpPrimary   bool   `json:"is_snmp_primary"`
	IfIndex         int    `json:"if_index"`
	SnmpInterfaceID int    `json:"snmp_interface_id"`
}

type nodeTopologyFixtureSNMP struct {
	ID      int    `json:"id"`
	NodeID  int    `json:"node_id"`
	IfIndex int    `json:"if_index"`
	IfName  string `json:"if_name"`
	IfDescr string `json:"if_descr"`
}

type nodeTopologyFixtureExpectations struct {
	Nodes            int                           `json:"nodes"`
	IPs              *int                          `json:"ips"`
	Subnets          *int                          `json:"subnets"`
	LegalSubnets     *int                          `json:"legal_subnets"`
	PTPSubnets       *int                          `json:"ptp_subnets"`
	LegalPTPSubnets  *int                          `json:"legal_ptp_subnets"`
	Loopbacks        *int                          `json:"loopbacks"`
	LegalLoopbacks   *int                          `json:"legal_loopbacks"`
	MultiBet         *int                          `json:"multibet"`
	PrioritySize     *int                          `json:"priority_size"`
	PriorityRule     nodeTopologyPriorityRule      `json:"priority_rule"`
	NLinksFinal      *int                          `json:"nlinks_final"`
	TopologyVertices *int                          `json:"topology_vertices"`
	TopologyEdges    *int                          `json:"topology_edges"`
	CIDRNodeSizes    []nodeTopologyCIDRExpectation `json:"cidr_node_sizes"`
}

type nodeTopologyPriorityRule struct {
	Kind  string `json:"kind"`
	Value *int   `json:"value"`
}

type nodeTopologyCIDRExpectation struct {
	CIDR    string `json:"cidr"`
	NodeIDs int    `json:"node_ids"`
}

func loadNodeTopologyFixtureDocument(t *testing.T) nodeTopologyFixtureDocument {
	t.Helper()
	path := filepath.Join("..", "testdata", "enlinkd", "node_topology", "scenarios.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var doc nodeTopologyFixtureDocument
	require.NoError(t, json.Unmarshal(data, &doc))
	require.Equal(t, 1, doc.SchemaVersion)
	require.NotEmpty(t, doc.Scenarios)
	return doc
}

func scenarioToService(t *testing.T, scenario nodeTopologyScenario) *engine.NodeTopologyService {
	t.Helper()

	nodes := make([]engine.NodeTopologyEntity, 0, len(scenario.Nodes))
	for _, node := range scenario.Nodes {
		entry := engine.NodeTopologyEntity{
			ID:        node.ID,
			Label:     node.Label,
			SysObject: node.SysObject,
			SysName:   node.SysName,
		}
		if ip, ok := parseOptionalAddr(node.Address); ok {
			entry.Address = ip
		}
		nodes = append(nodes, entry)
	}

	ips := make([]engine.IPInterfaceTopologyEntity, 0, len(scenario.IPs))
	for _, ip := range scenario.IPs {
		entry := engine.IPInterfaceTopologyEntity{
			ID:              ip.ID,
			NodeID:          ip.NodeID,
			IsManaged:       ip.IsManaged,
			IsSnmpPrimary:   ip.IsSnmpPrimary,
			IfIndex:         ip.IfIndex,
			SnmpInterfaceID: ip.SnmpInterfaceID,
		}
		parsedIP, ok := parseOptionalAddr(ip.IPAddress)
		require.Truef(t, ok, "invalid fixture IP address %q", ip.IPAddress)
		entry.IPAddress = parsedIP
		if mask, ok := parseOptionalAddr(ip.Netmask); ok {
			entry.NetMask = mask
		}
		ips = append(ips, entry)
	}

	snmp := make([]engine.SnmpInterfaceTopologyEntity, 0, len(scenario.SNMP))
	for _, iface := range scenario.SNMP {
		snmp = append(snmp, engine.SnmpInterfaceTopologyEntity{
			ID:      iface.ID,
			NodeID:  iface.NodeID,
			IfIndex: iface.IfIndex,
			IfName:  iface.IfName,
			IfDescr: iface.IfDescr,
		})
	}

	return engine.NewNodeTopologyService(nodes, ips, snmp)
}

func parseOptionalAddr(value string) (netip.Addr, bool) {
	value = stringsTrimSpace(value)
	if value == "" {
		return netip.Addr{}, false
	}
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

func runNodeTopologyScenarioParity(t *testing.T, scenarioName string) {
	t.Helper()
	doc := loadNodeTopologyFixtureDocument(t)
	scenario, ok := doc.Scenarios[scenarioName]
	require.Truef(t, ok, "missing scenario %q", scenarioName)

	service := scenarioToService(t, scenario)
	expect := scenario.Expect

	require.Len(t, service.FindAllNode(), expect.Nodes)
	if expect.IPs != nil {
		require.Len(t, service.FindAllIP(), *expect.IPs)
	}

	subnets := service.FindAllSubNetwork()
	if expect.Subnets != nil {
		require.Len(t, subnets, *expect.Subnets)
	}

	legalSubnets := service.FindAllLegalSubNetwork()
	if expect.LegalSubnets != nil {
		require.Len(t, legalSubnets, *expect.LegalSubnets)
	}

	ptpSubnets := service.FindAllPointToPointSubNetwork()
	if expect.PTPSubnets != nil {
		require.Len(t, ptpSubnets, *expect.PTPSubnets)
	}

	legalPTPSubnets := service.FindAllLegalPointToPointSubNetwork()
	if expect.LegalPTPSubnets != nil {
		require.Len(t, legalPTPSubnets, *expect.LegalPTPSubnets)
	}

	loopbacks := service.FindAllLoopbacks()
	if expect.Loopbacks != nil {
		require.Len(t, loopbacks, *expect.Loopbacks)
	}

	legalLoopbacks := service.FindAllLegalLoopbacks()
	if expect.LegalLoopbacks != nil {
		require.Len(t, legalLoopbacks, *expect.LegalLoopbacks)
	}

	multibet := service.FindSubNetworkByNetworkPrefixLessThen(30, 126)
	if expect.MultiBet != nil {
		require.Len(t, multibet, *expect.MultiBet)
	}

	priority := service.GetNodeIDPriorityMap()
	if expect.PrioritySize != nil {
		require.Len(t, priority, *expect.PrioritySize)
	}
	switch expect.PriorityRule.Kind {
	case "all_zero":
		for _, value := range priority {
			require.Equal(t, 0, value)
		}
	case "lt":
		require.NotNil(t, expect.PriorityRule.Value)
		for _, value := range priority {
			require.Less(t, value, *expect.PriorityRule.Value)
		}
	}

	if len(expect.CIDRNodeSizes) > 0 {
		actual := make(map[string]int)
		for _, subnet := range legalSubnets {
			actual[subnet.CIDR()] = len(subnet.NodeIDs())
		}
		for _, expectedCIDR := range expect.CIDRNodeSizes {
			require.Equalf(t, expectedCIDR.NodeIDs, actual[expectedCIDR.CIDR], "cidr %s", expectedCIDR.CIDR)
		}
	}

	topology := engine.BuildNetworkRouterTopology(service, 30, 126)
	if expect.TopologyVertices != nil {
		require.Len(t, topology.Vertices, *expect.TopologyVertices)
	} else if expect.MultiBet != nil {
		require.Len(t, topology.Vertices, expect.Nodes+*expect.MultiBet)
	}

	if expect.TopologyEdges != nil {
		require.Len(t, topology.Edges, *expect.TopologyEdges)
	} else if expect.NLinksFinal != nil {
		require.Len(t, topology.Edges, *expect.NLinksFinal)
	}
}

func TestNodeTopologyServiceIT_nms0001SubnetworksTest(t *testing.T) {
	runNodeTopologyScenarioParity(t, "nms0001SubnetworksTest")
}
func TestNodeTopologyServiceIT_nms0002SubnetworksTest(t *testing.T) {
	runNodeTopologyScenarioParity(t, "nms0002SubnetworksTest")
}
func TestNodeTopologyServiceIT_nms003SubnetworkTests(t *testing.T) {
	runNodeTopologyScenarioParity(t, "nms003SubnetworkTests")
}
func TestNodeTopologyServiceIT_nms007SubnetworkTest(t *testing.T) {
	runNodeTopologyScenarioParity(t, "nms007SubnetworkTest")
}
func TestNodeTopologyServiceIT_nms101SubnetworksTest(t *testing.T) {
	runNodeTopologyScenarioParity(t, "nms101SubnetworksTest")
}
func TestNodeTopologyServiceIT_nms102SubnetworksTest(t *testing.T) {
	runNodeTopologyScenarioParity(t, "nms102SubnetworksTest")
}
func TestNodeTopologyServiceIT_nms0123SubnetworksTest(t *testing.T) {
	runNodeTopologyScenarioParity(t, "nms0123SubnetworksTest")
}
func TestNodeTopologyServiceIT_nms1055SubnetworksTest(t *testing.T) {
	runNodeTopologyScenarioParity(t, "nms1055SubnetworksTest")
}
func TestNodeTopologyServiceIT_nms4005SubnetworksTest(t *testing.T) {
	runNodeTopologyScenarioParity(t, "nms4005SubnetworksTest")
}
func TestNodeTopologyServiceIT_nms4930SubnetworksTest(t *testing.T) {
	runNodeTopologyScenarioParity(t, "nms4930SubnetworksTest")
}
func TestNodeTopologyServiceIT_nms6802SubnetworksTest(t *testing.T) {
	runNodeTopologyScenarioParity(t, "nms6802SubnetworksTest")
}
func TestNodeTopologyServiceIT_nms7467SubnetworksTest(t *testing.T) {
	runNodeTopologyScenarioParity(t, "nms7467SubnetworksTest")
}
func TestNodeTopologyServiceIT_nms7563SubnetworksTest(t *testing.T) {
	runNodeTopologyScenarioParity(t, "nms7563SubnetworksTest")
}
func TestNodeTopologyServiceIT_nms7777DWSubnetworksTest(t *testing.T) {
	runNodeTopologyScenarioParity(t, "nms7777DWSubnetworksTest")
}
func TestNodeTopologyServiceIT_nms7918SubnetworksTest(t *testing.T) {
	runNodeTopologyScenarioParity(t, "nms7918SubnetworksTest")
}
func TestNodeTopologyServiceIT_nms8000SubnetworksTest(t *testing.T) {
	runNodeTopologyScenarioParity(t, "nms8000SubnetworksTest")
}
func TestNodeTopologyServiceIT_nms10205aSubnetworksTest(t *testing.T) {
	runNodeTopologyScenarioParity(t, "nms10205aSubnetworksTest")
}
func TestNodeTopologyServiceIT_nms10205bSubnetworksTest(t *testing.T) {
	runNodeTopologyScenarioParity(t, "nms10205bSubnetworksTest")
}
func TestNodeTopologyServiceIT_nms13593SubnetworksTest(t *testing.T) {
	runNodeTopologyScenarioParity(t, "nms13593SubnetworksTest")
}
func TestNodeTopologyServiceIT_nms13637SubnetworksTest(t *testing.T) {
	runNodeTopologyScenarioParity(t, "nms13637SubnetworksTest")
}
func TestNodeTopologyServiceIT_nms13923SubnetworksTest(t *testing.T) {
	runNodeTopologyScenarioParity(t, "nms13923SubnetworksTest")
}
func TestNodeTopologyServiceIT_nms17216SubnetworksTest(t *testing.T) {
	runNodeTopologyScenarioParity(t, "nms17216SubnetworksTest")
}

func TestNms0001EnIT_testLinkdNetworkTopologyUpdater(t *testing.T) {
	doc := loadNodeTopologyFixtureDocument(t)
	scenario, ok := doc.Scenarios["nms0001EnIT_testLinkdNetworkTopologyUpdater"]
	require.True(t, ok)

	service := scenarioToService(t, scenario)
	topology := engine.BuildNetworkRouterTopology(service, 30, 126)
	require.NotEmpty(t, topology.Vertices)
	require.Len(t, topology.Vertices, *scenario.Expect.TopologyVertices)
	require.Len(t, topology.Edges, *scenario.Expect.TopologyEdges)
}

func TestNodeTopologyFixtureCoverage(t *testing.T) {
	doc := loadNodeTopologyFixtureDocument(t)
	scenarios := make([]string, 0, len(doc.Scenarios))
	for name := range doc.Scenarios {
		scenarios = append(scenarios, name)
	}
	sort.Strings(scenarios)

	require.Contains(t, scenarios, "nms0001SubnetworksTest")
	require.Contains(t, scenarios, "nms17216SubnetworksTest")
	require.Contains(t, scenarios, "nms0001EnIT_testLinkdNetworkTopologyUpdater")
}

func stringsTrimSpace(value string) string { return strings.TrimSpace(value) }
