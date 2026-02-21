// SPDX-License-Identifier: GPL-3.0-or-later

package parity

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/topology/engine"
	"github.com/stretchr/testify/require"
)

func TestBuildL3ObservationFromWalk_EnLinkdSnmpIT_OspfGeneralNbrIfWalks(t *testing.T) {
	fixture := loadRoutingFixtureWalk(t,
		"switch1",
		"../testdata/enlinkd/upstream/linkd/nms17216/switch1-walk.txt",
	)

	observation, _, err := BuildL3ObservationFromWalk(fixture)
	require.NoError(t, err)

	require.NotNil(t, observation.OSPFElement)
	require.Equal(t, "192.168.100.246", observation.OSPFElement.RouterID)
	require.Equal(t, 1, observation.OSPFElement.AdminState)
	require.Equal(t, 2, observation.OSPFElement.VersionNumber)
	require.Equal(t, 2, observation.OSPFElement.AreaBorderRouter)
	require.Equal(t, 2, observation.OSPFElement.ASBorderRouter)

	ifByIP := make(map[string]engine.OSPFInterfaceObservation, len(observation.OSPFIfTable))
	for _, row := range observation.OSPFIfTable {
		ifByIP[row.IP] = row
	}
	require.Contains(t, ifByIP, "192.168.100.246")
	require.Equal(t, 0, ifByIP["192.168.100.246"].AddressLessIdx)
	require.Equal(t, "0.0.0.0", ifByIP["192.168.100.246"].AreaID)
	require.Equal(t, 10101, ifByIP["192.168.100.246"].IfIndex)
	require.Equal(t, "255.255.255.252", ifByIP["192.168.100.246"].Netmask)

	require.Len(t, observation.OSPFNbrTable, 1)
	require.Equal(t, "192.168.100.249", observation.OSPFNbrTable[0].RemoteRouterID)
	require.Equal(t, "192.168.100.245", observation.OSPFNbrTable[0].RemoteIP)
	require.Equal(t, 0, observation.OSPFNbrTable[0].RemoteAddressLessIdx)
}

func TestBuildL3ObservationFromWalk_EnLinkdSnmpIT_OspfTableTracker(t *testing.T) {
	fixture := loadRoutingFixtureWalk(t,
		"FireFly170",
		"../testdata/enlinkd/upstream/linkd/nms007/mib2_192.168.168.170.txt",
	)

	_, areas, err := BuildL3ObservationFromWalk(fixture)
	require.NoError(t, err)
	require.NotEmpty(t, areas)

	areaByID := make(map[string]OSPFAreaObservation, len(areas))
	for _, area := range areas {
		areaByID[area.AreaID] = area
	}
	require.Contains(t, areaByID, "0.0.0.0")
	require.Equal(t, 0, areaByID["0.0.0.0"].AuthType)
	require.Equal(t, 1, areaByID["0.0.0.0"].ImportAsExtern)
	require.Equal(t, 4, areaByID["0.0.0.0"].AreaBdrRtrCount)
	require.Equal(t, 2, areaByID["0.0.0.0"].ASBdrRtrCount)
	require.Equal(t, 43, areaByID["0.0.0.0"].AreaLsaCount)
}

func TestBuildL3ObservationFromWalk_EnLinkdSnmpIT_IsisTrackerWalks(t *testing.T) {
	fixture := loadRoutingFixtureWalk(t,
		"siegfrie",
		"../testdata/enlinkd/upstream/linkd/nms0001/siegfrie-192.168.239.54-walk.txt",
	)

	observation, _, err := BuildL3ObservationFromWalk(fixture)
	require.NoError(t, err)

	require.NotNil(t, observation.ISISElement)
	require.Equal(t, "000110255054", observation.ISISElement.SysID)
	require.Equal(t, 1, observation.ISISElement.AdminState)

	require.Len(t, observation.ISISCircTable, 12)
	circByIndex := make(map[int]engine.ISISCircuitObservation, len(observation.ISISCircTable))
	for _, row := range observation.ISISCircTable {
		circByIndex[row.CircIndex] = row
	}
	require.Contains(t, circByIndex, 533)
	require.Equal(t, 533, circByIndex[533].IfIndex)
	require.Equal(t, 1, circByIndex[533].AdminState)
	require.Contains(t, circByIndex, 552)
	require.Equal(t, 552, circByIndex[552].IfIndex)
	require.Equal(t, 1, circByIndex[552].AdminState)
	require.Contains(t, circByIndex, 13)
	require.Equal(t, 13, circByIndex[13].IfIndex)
	require.Equal(t, 2, circByIndex[13].AdminState)

	require.Len(t, observation.ISISAdjTable, 2)
	adjByCirc := make(map[int]engine.ISISAdjacencyObservation, len(observation.ISISAdjTable))
	for _, row := range observation.ISISAdjTable {
		adjByCirc[row.CircIndex] = row
	}
	require.Contains(t, adjByCirc, 533)
	require.Equal(t, 1, adjByCirc[533].AdjIndex)
	require.Equal(t, 3, adjByCirc[533].State)
	require.Equal(t, 1, adjByCirc[533].NeighborSysType)
	require.Equal(t, 0, adjByCirc[533].NeighborExtendedID)
	require.Equal(t, "001f12accbf0", adjByCirc[533].NeighborSNPA)
	require.Equal(t, "000110255062", adjByCirc[533].NeighborSysID)

	require.Contains(t, adjByCirc, 552)
	require.Equal(t, 1, adjByCirc[552].AdjIndex)
	require.Equal(t, 3, adjByCirc[552].State)
	require.Equal(t, 1, adjByCirc[552].NeighborSysType)
	require.Equal(t, 0, adjByCirc[552].NeighborExtendedID)
	require.Equal(t, "0021590e47c2", adjByCirc[552].NeighborSNPA)
	require.Equal(t, "000110088500", adjByCirc[552].NeighborSysID)
}

func TestBuildL3ObservationFromWalk_Nms6802_IsisLinks(t *testing.T) {
	fixture := loadRoutingFixtureWalk(t,
		"cisco-ios-xr",
		"../testdata/enlinkd/upstream/linkd/nms6802/cisco-ios-xr-walk.txt",
	)

	observation, _, err := BuildL3ObservationFromWalk(fixture)
	require.NoError(t, err)
	require.NotNil(t, observation.ISISElement)
	require.Equal(t, "093176090107", observation.ISISElement.SysID)
	require.Equal(t, 1, observation.ISISElement.AdminState)

	require.Len(t, observation.ISISAdjTable, 4)
	circByIndex := make(map[int]engine.ISISCircuitObservation, len(observation.ISISCircTable))
	for _, row := range observation.ISISCircTable {
		circByIndex[row.CircIndex] = row
	}

	type expected struct {
		adjIndex        int
		ifIndex         int
		neighborSysID   string
		neighborExtCirc int
	}
	expectedByCirc := map[int]expected{
		19: {adjIndex: 5, ifIndex: 19, neighborSysID: "093176092059", neighborExtCirc: 234881856},
		20: {adjIndex: 5, ifIndex: 20, neighborSysID: "093176092059", neighborExtCirc: 234881920},
		27: {adjIndex: 3, ifIndex: 27, neighborSysID: "093176090003", neighborExtCirc: 33554880},
		28: {adjIndex: 3, ifIndex: 28, neighborSysID: "093176090003", neighborExtCirc: 33554944},
	}

	for _, adj := range observation.ISISAdjTable {
		exp, ok := expectedByCirc[adj.CircIndex]
		require.Truef(t, ok, "unexpected ISIS circ index %d", adj.CircIndex)
		require.Equal(t, 3, adj.State)
		require.Equal(t, 2, adj.NeighborSysType)
		require.Equal(t, "000000000000", adj.NeighborSNPA)
		require.Equal(t, exp.adjIndex, adj.AdjIndex)
		require.Equal(t, exp.neighborSysID, adj.NeighborSysID)
		require.Equal(t, exp.neighborExtCirc, adj.NeighborExtendedID)
		require.Contains(t, circByIndex, adj.CircIndex)
		require.Equal(t, exp.ifIndex, circByIndex[adj.CircIndex].IfIndex)
		require.Equal(t, 1, circByIndex[adj.CircIndex].AdminState)
	}
}

func TestBuildL3ResultFromWalks_Nms0001_IsisPairing(t *testing.T) {
	fixtures := []FixtureWalk{
		loadRoutingFixtureWalk(t, "froh", "../testdata/enlinkd/upstream/linkd/nms0001/froh-192.168.239.51-walk.txt"),
		loadRoutingFixtureWalk(t, "oedipus", "../testdata/enlinkd/upstream/linkd/nms0001/oedipus-192.168.239.62-walk.txt"),
		loadRoutingFixtureWalk(t, "siegfrie", "../testdata/enlinkd/upstream/linkd/nms0001/siegfrie-192.168.239.54-walk.txt"),
	}

	result, err := BuildL3ResultFromWalks(fixtures)
	require.NoError(t, err)

	isisAdjacencies := make([]engine.Adjacency, 0)
	for _, adj := range result.Adjacencies {
		if adj.Protocol == "isis" {
			isisAdjacencies = append(isisAdjacencies, adj)
		}
	}
	require.Len(t, isisAdjacencies, 6)
	require.Equal(t, 3, countUndirectedPairIDs(isisAdjacencies))
}

func TestBuildL3ObservationFromWalk_Nms0001_IsisLinksExecSingleDevice(t *testing.T) {
	fixture := loadRoutingFixtureWalk(t,
		"froh",
		"../testdata/enlinkd/upstream/linkd/nms0001/froh-192.168.239.51-walk.txt",
	)

	observation, _, err := BuildL3ObservationFromWalk(fixture)
	require.NoError(t, err)
	require.NotNil(t, observation.ISISElement)
	require.Equal(t, 2, len(observation.ISISAdjTable))
}

func TestBuildL3ResultFromWalks_Nms10205b_OspfTopology(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms10205b/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms10205b_lldp")
	require.True(t, ok)
	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 9)

	walks, err := LoadScenarioWalks(resolved)
	require.NoError(t, err)

	result, err := BuildL3ResultFromWalks(walks)
	require.NoError(t, err)

	ospfAdjacencies := make([]engine.Adjacency, 0)
	for _, adj := range result.Adjacencies {
		if adj.Protocol == "ospf" {
			ospfAdjacencies = append(ospfAdjacencies, adj)
		}
	}
	require.Len(t, ospfAdjacencies, 22)
	require.Equal(t, 11, countUndirectedPairIDs(ospfAdjacencies))

	check := []ospfEdgeExpectation{
		{src: "Mumbai", dst: "Mysore", srcPort: "ge-0/1/1.0", dstPort: "ge-0/0/1.0", srcIP: "192.168.5.21", dstIP: "192.168.5.22", srcIf: 978, dstIf: 508},
		{src: "Mumbai", dst: "Bagmane", srcPort: "ge-0/0/2.0", dstPort: "ge-1/0/0.0", srcIP: "192.168.5.17", dstIP: "192.168.5.18", srcIf: 977, dstIf: 534},
		{src: "Mumbai", dst: "Delhi", srcPort: "ge-0/1/2.0", dstPort: "ge-1/0/2.0", srcIP: "192.168.5.9", dstIP: "192.168.5.10", srcIf: 519, dstIf: 28503},
		{src: "Mumbai", dst: "Bangalore", srcPort: "ge-0/0/1.0", dstPort: "ge-0/0/0.0", srcIP: "192.168.5.13", dstIP: "192.168.5.14", srcIf: 507, dstIf: 2401},
		{src: "Bangalore", dst: "Bagmane", srcPort: "ge-0/1/0.0", dstPort: "ge-1/0/4.0", srcIP: "192.168.1.9", dstIP: "192.168.1.10", srcIf: 2396, dstIf: 1732},
		{src: "Bangalore", dst: "Space-EX-SW2", srcPort: "ge-0/0/3.0", dstPort: "ge-0/0/3.0", srcIP: "172.16.9.1", dstIP: "172.16.9.2", srcIf: 2398, dstIf: 551},
		{src: "Space-EX-SW1", dst: "Space-EX-SW2", srcPort: "ge-0/0/0.0", dstPort: "ge-0/0/0.0", srcIP: "172.16.10.1", dstIP: "172.16.10.2", srcIf: 1361, dstIf: 531},
		{src: "Bagmane", dst: "Mysore", srcPort: "ge-1/0/5.0", dstPort: "ge-0/1/1.0", srcIP: "192.168.1.13", dstIP: "192.168.1.14", srcIf: 654, dstIf: 520},
		{src: "Bagmane", dst: "J6350-42", srcPort: "ge-1/0/2.0", dstPort: "ge-0/0/2.0", srcIP: "172.16.20.1", dstIP: "172.16.20.2", srcIf: 540, dstIf: 549},
		{src: "Delhi", dst: "Space-EX-SW1", srcPort: "ge-1/1/6.0", dstPort: "ge-0/0/6.0", srcIP: "172.16.7.1", dstIP: "172.16.7.2", srcIf: 17619, dstIf: 528},
		{src: "Delhi", dst: "Bangalore", srcPort: "ge-1/0/1.0", dstPort: "ge-0/0/1.0", srcIP: "192.168.1.5", dstIP: "192.168.1.6", srcIf: 3674, dstIf: 2397},
	}

	for _, edge := range check {
		requireOSPFDirectedEdge(t, ospfAdjacencies, edge.src, edge.dst, edge.srcPort, edge.dstPort, edge.srcIP, edge.dstIP, edge.srcIf)
		requireOSPFDirectedEdge(t, ospfAdjacencies, edge.dst, edge.src, edge.dstPort, edge.srcPort, edge.dstIP, edge.srcIP, edge.dstIf)
	}
}

type ospfEdgeExpectation struct {
	src     string
	dst     string
	srcPort string
	dstPort string
	srcIP   string
	dstIP   string
	srcIf   int
	dstIf   int
}

func loadRoutingFixtureWalk(t *testing.T, deviceID, walkPath string) FixtureWalk {
	t.Helper()
	ds, err := LoadWalkFile(walkPath)
	require.NoError(t, err)
	return FixtureWalk{
		DeviceID: deviceID,
		Hostname: deviceID,
		Records:  ds.Records,
	}
}

func countUndirectedPairIDs(adjacencies []engine.Adjacency) int {
	unique := make(map[string]struct{})
	for _, adj := range adjacencies {
		pairID := strings.TrimSpace(adj.Labels["pair_id"])
		if pairID == "" {
			continue
		}
		unique[pairID] = struct{}{}
	}
	return len(unique)
}

func requireOSPFDirectedEdge(
	t *testing.T,
	adjacencies []engine.Adjacency,
	sourceID, targetID, sourcePort, targetPort, sourceIP, remoteIP string,
	sourceIfIndex int,
) {
	t.Helper()
	adj := findOSPFAdjacency(adjacencies, sourceID, targetID, sourcePort, targetPort)
	require.NotNilf(t, adj, "missing ospf adjacency %s/%s -> %s/%s", sourceID, sourcePort, targetID, targetPort)
	require.Equal(t, "ospf", adj.Protocol)
	require.Equal(t, sourceIP, adj.Labels["ospf_ip_addr"])
	require.Equal(t, remoteIP, adj.Labels["ospf_rem_ip_addr"])
	require.Equal(t, strconv.Itoa(sourceIfIndex), adj.Labels["ospf_if_index"])
	require.Equal(t, sourcePort, adj.Labels["ospf_if_name"])
	require.NotEmpty(t, adj.Labels["pair_id"])
}

func findOSPFAdjacency(adjacencies []engine.Adjacency, sourceID, targetID, sourcePort, targetPort string) *engine.Adjacency {
	for i := range adjacencies {
		adj := &adjacencies[i]
		if adj.Protocol != "ospf" {
			continue
		}
		if adj.SourceID != sourceID || adj.TargetID != targetID {
			continue
		}
		if adj.SourcePort != sourcePort || adj.TargetPort != targetPort {
			continue
		}
		return adj
	}
	return nil
}

func TestParseNumeric(t *testing.T) {
	cases := map[string]int{
		"1":          1,
		"Gauge32: 4": 4,
		"up(3)":      3,
		"down(2)":    2,
		"":           0,
	}
	keys := make([]string, 0, len(cases))
	for key := range cases {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, input := range keys {
		require.Equal(t, cases[input], parseNumeric(input), fmt.Sprintf("parseNumeric(%q)", input))
	}
}
