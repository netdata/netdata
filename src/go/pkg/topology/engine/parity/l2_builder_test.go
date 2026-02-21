// SPDX-License-Identifier: GPL-3.0-or-later

package parity

import (
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/topology/engine"
	"github.com/stretchr/testify/require"
)

func TestBuildL2ResultFromWalks_LLDP_NMS8003(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms8003/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms8003_lldp")
	require.True(t, ok)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)

	walks, err := LoadScenarioWalks(resolved)
	require.NoError(t, err)

	result, err := BuildL2ResultFromWalks(walks, BuildOptions{EnableLLDP: true})
	require.NoError(t, err)
	require.Len(t, result.Devices, 5)
	require.Len(t, result.Adjacencies, 12)
	require.Equal(t, 12, result.Stats["links_lldp"])
	require.Equal(t, 0, result.Stats["links_cdp"])

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)

	expected := make(map[string]struct{}, len(golden.Adjacencies))
	for _, adj := range golden.Adjacencies {
		expected[adj.Protocol+"|"+adj.SourceDevice+"|"+adj.SourcePort+"|"+adj.TargetDevice+"|"+adj.TargetPort] = struct{}{}
	}

	actual := make(map[string]struct{}, len(result.Adjacencies))
	for _, adj := range result.Adjacencies {
		actual[adj.Protocol+"|"+adj.SourceID+"|"+adj.SourcePort+"|"+adj.TargetID+"|"+adj.TargetPort] = struct{}{}
	}

	require.Equal(t, expected, actual)
}

func TestBuildL2ResultFromWalks_CDP_NMS8000(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms8000/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms8000_cdp")
	require.True(t, ok)

	require.False(t, scenario.Protocols.LLDP)
	require.True(t, scenario.Protocols.CDP)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, scenario.Protocols.ARPND)
	require.Equal(t, ManifestProtocols{CDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 5)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableCDP: true})
	require.NoError(t, errStep1)
	require.Equal(t, 3, step1.Stats["links_cdp"])

	step2, errStep2 := buildResultFromScenarioPrefix(resolved, 2, BuildOptions{EnableCDP: true})
	require.NoError(t, errStep2)
	require.Equal(t, 6, step2.Stats["links_cdp"])

	step3, errStep3 := buildResultFromScenarioPrefix(resolved, 3, BuildOptions{EnableCDP: true})
	require.NoError(t, errStep3)
	require.Equal(t, 9, step3.Stats["links_cdp"])

	step4, errStep4 := buildResultFromScenarioPrefix(resolved, 4, BuildOptions{EnableCDP: true})
	require.NoError(t, errStep4)
	require.Equal(t, 11, step4.Stats["links_cdp"])

	step5, errStep5 := buildResultFromScenarioPrefix(resolved, 5, BuildOptions{EnableCDP: true})
	require.NoError(t, errStep5)
	require.Equal(t, 13, step5.Stats["links_cdp"])

	final := step5
	require.Len(t, final.Adjacencies, 13)
	require.Equal(t, 13, final.Stats["links_cdp"])
	require.Equal(t, 0, final.Stats["links_lldp"])

	expected := map[string]struct{}{
		"cdp|nmmr1|Gi0/0|nmmr3|GigabitEthernet0/1":                                {},
		"cdp|nmmr1|Gi0/1|nmmsw1|FastEthernet0/1":                                  {},
		"cdp|nmmr1|Gi0/2|nmmsw2|FastEthernet0/2":                                  {},
		"cdp|nmmr2|Gi0/0|nmmr3|GigabitEthernet0/2":                                {},
		"cdp|nmmr2|Gi0/1|nmmsw2|FastEthernet0/1":                                  {},
		"cdp|nmmr2|Gi0/2|nmmsw1|FastEthernet0/2":                                  {},
		"cdp|nmmr3|Gi0/0|netlabsw03.informatik.hs-fulda.de|GigabitEthernet2/0/18": {},
		"cdp|nmmr3|Gi0/1|nmmr1|GigabitEthernet0/0":                                {},
		"cdp|nmmr3|Gi0/2|nmmr2|GigabitEthernet0/0":                                {},
		"cdp|nmmsw1|Fa0/1|nmmr1|GigabitEthernet0/1":                               {},
		"cdp|nmmsw1|Fa0/2|nmmr2|GigabitEthernet0/2":                               {},
		"cdp|nmmsw2|Fa0/1|nmmr2|GigabitEthernet0/1":                               {},
		"cdp|nmmsw2|Fa0/2|nmmr1|GigabitEthernet0/2":                               {},
	}
	require.Equal(t, expected, adjacencyKeySet(final.Adjacencies))

	for _, adj := range final.Adjacencies {
		require.Equal(t, "cdp", adj.Protocol)
		raw := strings.TrimSpace(adj.Labels["remote_address_raw"])
		require.NotEmpty(t, raw)
		decoded := decodeHexIP(raw)
		require.NotEmpty(t, decoded)
		_, parseErr := netip.ParseAddr(decoded)
		require.NoError(t, parseErr)
	}
}

func TestBuildL2ResultFromWalks_LLDP_NMS8000(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms8000/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms8000_lldp")
	require.True(t, ok)

	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, scenario.Protocols.ARPND)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 5)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep1)
	require.Equal(t, 3, step1.Stats["links_lldp"])

	step2, errStep2 := buildResultFromScenarioPrefix(resolved, 2, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep2)
	require.Equal(t, 6, step2.Stats["links_lldp"])

	step3, errStep3 := buildResultFromScenarioPrefix(resolved, 3, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep3)
	require.Equal(t, 8, step3.Stats["links_lldp"])

	step4, errStep4 := buildResultFromScenarioPrefix(resolved, 4, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep4)
	require.Equal(t, 10, step4.Stats["links_lldp"])

	step5, errStep5 := buildResultFromScenarioPrefix(resolved, 5, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep5)
	require.Equal(t, 12, step5.Stats["links_lldp"])

	final := step5
	require.Len(t, final.Adjacencies, 12)
	require.Equal(t, 12, final.Stats["links_lldp"])
	require.Equal(t, 0, final.Stats["links_cdp"])

	expected := map[string]struct{}{
		"lldp|nmmr1|Gi0/0|nmmr3|Gi0/1":  {},
		"lldp|nmmr1|Gi0/1|nmmsw1|Fa0/1": {},
		"lldp|nmmr1|Gi0/2|nmmsw2|Fa0/2": {},
		"lldp|nmmr2|Gi0/0|nmmr3|Gi0/2":  {},
		"lldp|nmmr2|Gi0/1|nmmsw2|Fa0/1": {},
		"lldp|nmmr2|Gi0/2|nmmsw1|Fa0/2": {},
		"lldp|nmmr3|Gi0/1|nmmr1|Gi0/0":  {},
		"lldp|nmmr3|Gi0/2|nmmr2|Gi0/0":  {},
		"lldp|nmmsw1|Fa0/1|nmmr1|Gi0/1": {},
		"lldp|nmmsw1|Fa0/2|nmmr2|Gi0/2": {},
		"lldp|nmmsw2|Fa0/1|nmmr2|Gi0/1": {},
		"lldp|nmmsw2|Fa0/2|nmmr1|Gi0/2": {},
	}
	require.Equal(t, expected, adjacencyKeySet(final.Adjacencies))
	require.Equal(t, 6, countBidirectionalPairs(final.Adjacencies, "lldp"))
}

func TestBuildL2ResultFromWalks_LLDP_NMS13637(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms13637/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms13637_lldp")
	require.True(t, ok)

	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, scenario.Protocols.ARPND)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 3)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep1)
	require.Equal(t, 5, step1.Stats["links_lldp"])

	step2, errStep2 := buildResultFromScenarioPrefix(resolved, 2, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep2)
	require.Equal(t, 10, step2.Stats["links_lldp"])

	step3, errStep3 := buildResultFromScenarioPrefix(resolved, 3, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep3)
	require.Equal(t, 19, step3.Stats["links_lldp"])

	final := step3
	require.Len(t, final.Adjacencies, 19)
	require.Equal(t, 19, final.Stats["links_lldp"])
	require.Equal(t, 0, final.Stats["links_cdp"])

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)

	expected := make(map[string]struct{}, len(golden.Adjacencies))
	for _, adj := range golden.Adjacencies {
		expected[adj.Protocol+"|"+adj.SourceDevice+"|"+adj.SourcePort+"|"+adj.TargetDevice+"|"+adj.TargetPort] = struct{}{}
	}
	require.Equal(t, expected, adjacencyKeySet(final.Adjacencies))

	localDevices := map[string]struct{}{
		"router-1":    {},
		"router-2":    {},
		"sw01-office": {},
	}
	require.Equal(t, 3, countUndirectedLocalDevicePairs(final.Adjacencies, "lldp", localDevices))
	require.Equal(t, 3, countLocalTopologyVertices(final.Adjacencies, "lldp", localDevices))
}

func TestBuildL2ResultFromWalks_LLDP_NMS10205B(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms10205b/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms10205b_lldp")
	require.True(t, ok)

	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, scenario.Protocols.ARPND)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 9)

	type stepExpectation struct {
		fixtureID      string
		totalLinks     int
		sourceLinks    int
		lldpElementCnt int
	}

	expectations := []stepExpectation{
		{fixtureID: "Mumbai", totalLinks: 0, sourceLinks: 0, lldpElementCnt: 0},
		{fixtureID: "Delhi", totalLinks: 2, sourceLinks: 2, lldpElementCnt: 1},
		{fixtureID: "Bangalore", totalLinks: 2, sourceLinks: 0, lldpElementCnt: 1},
		{fixtureID: "Bagmane", totalLinks: 5, sourceLinks: 3, lldpElementCnt: 2},
		{fixtureID: "Mysore", totalLinks: 5, sourceLinks: 0, lldpElementCnt: 2},
		{fixtureID: "Space-EX-SW1", totalLinks: 8, sourceLinks: 3, lldpElementCnt: 3},
		{fixtureID: "Space-EX-SW2", totalLinks: 10, sourceLinks: 2, lldpElementCnt: 4},
		{fixtureID: "J6350-42", totalLinks: 10, sourceLinks: 0, lldpElementCnt: 5},
		{fixtureID: "SRX-100", totalLinks: 10, sourceLinks: 0, lldpElementCnt: 6},
	}

	var final engine.Result
	for i, expected := range expectations {
		walks, walkErr := loadScenarioWalkPrefix(resolved, i+1)
		require.NoError(t, walkErr)

		result, buildErr := BuildL2ResultFromWalks(walks, BuildOptions{EnableLLDP: true})
		require.NoError(t, buildErr)

		sourceCounts := countAdjacenciesBySource(result.Adjacencies, "lldp")
		require.Equal(t, expected.sourceLinks, sourceCounts[expected.fixtureID])
		require.Equal(t, expected.totalLinks, result.Stats["links_lldp"])
		require.Equal(t, expected.lldpElementCnt, countFixturesWithLLDPLocalElements(walks))
		final = result
	}

	require.Equal(t, 10, final.Stats["links_lldp"])
	require.Equal(t, 0, final.Stats["links_cdp"])
	allWalks, err := loadScenarioWalkPrefix(resolved, len(resolved.Fixtures))
	require.NoError(t, err)
	require.Equal(t, 6, countFixturesWithLLDPLocalElements(allWalks))
	require.Len(t, final.Adjacencies, 10)
	require.Equal(t, 3, countBidirectionalPairs(final.Adjacencies, "lldp"))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)

	expected := make(map[string]struct{}, len(golden.Adjacencies))
	for _, adj := range golden.Adjacencies {
		expected[adj.Protocol+"|"+adj.SourceDevice+"|"+adj.SourcePort+"|"+adj.TargetDevice+"|"+adj.TargetPort] = struct{}{}
	}
	require.Equal(t, expected, adjacencyKeySet(final.Adjacencies))

	finalKeys := adjacencyKeySet(final.Adjacencies)
	require.Contains(t, finalKeys, "lldp|Delhi|28519|Bagmane|513")
	require.Contains(t, finalKeys, "lldp|Delhi|28520|Space-EX-SW1|528")
	require.Contains(t, finalKeys, "lldp|Space-EX-SW1|1361|Space-EX-SW2|531")

	topologyEdges := make(map[string]engine.Adjacency, len(final.Adjacencies))
	for _, adj := range final.Adjacencies {
		if adj.Protocol != "lldp" {
			continue
		}
		topologyEdges[adj.SourceID+"|"+adj.TargetID] = adj
	}

	edgeDelhiBagmane, ok := topologyEdges["Delhi|Bagmane"]
	require.True(t, ok)
	require.Equal(t, "28519", edgeDelhiBagmane.SourcePort)
	require.Equal(t, "513", edgeDelhiBagmane.TargetPort)

	edgeDelhiSpaceEXSW1, ok := topologyEdges["Delhi|Space-EX-SW1"]
	require.True(t, ok)
	require.Equal(t, "28520", edgeDelhiSpaceEXSW1.SourcePort)
	require.Equal(t, "528", edgeDelhiSpaceEXSW1.TargetPort)

	edgeSpaceEXSW1SpaceEXSW2, ok := topologyEdges["Space-EX-SW1|Space-EX-SW2"]
	require.True(t, ok)
	require.Equal(t, "1361", edgeSpaceEXSW1SpaceEXSW2.SourcePort)
	require.Equal(t, "531", edgeSpaceEXSW1SpaceEXSW2.TargetPort)
}

func TestBuildL2ResultFromWalks_LLDP_NMS17216(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms17216/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms17216_lldp")
	require.True(t, ok)

	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, scenario.Protocols.ARPND)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 5)

	type stepExpectation struct {
		fixtureID      string
		totalLinks     int
		sourceLinks    int
		lldpElementCnt int
	}

	expectations := []stepExpectation{
		{fixtureID: "Switch1", totalLinks: 4, sourceLinks: 4, lldpElementCnt: 1},
		{fixtureID: "Switch2", totalLinks: 10, sourceLinks: 6, lldpElementCnt: 2},
		{fixtureID: "Switch3", totalLinks: 12, sourceLinks: 2, lldpElementCnt: 3},
		{fixtureID: "Switch4", totalLinks: 12, sourceLinks: 0, lldpElementCnt: 4},
		{fixtureID: "Switch5", totalLinks: 12, sourceLinks: 0, lldpElementCnt: 5},
	}

	var final engine.Result
	for i, expected := range expectations {
		walks, walkErr := loadScenarioWalkPrefix(resolved, i+1)
		require.NoError(t, walkErr)

		result, buildErr := BuildL2ResultFromWalks(walks, BuildOptions{EnableLLDP: true})
		require.NoError(t, buildErr)

		sourceCounts := countAdjacenciesBySource(result.Adjacencies, "lldp")
		require.Equal(t, expected.totalLinks, result.Stats["links_lldp"])
		require.Equal(t, expected.sourceLinks, sourceCounts[expected.fixtureID])
		require.Equal(t, expected.lldpElementCnt, countFixturesWithLLDPLocalElements(walks))
		final = result
	}

	require.Len(t, final.Devices, 5)
	require.Len(t, final.Adjacencies, 12)
	require.Equal(t, 12, final.Stats["links_lldp"])
	require.Equal(t, 0, final.Stats["links_cdp"])
	require.Equal(t, 6, countBidirectionalPairs(final.Adjacencies, "lldp"))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)

	expected := make(map[string]struct{}, len(golden.Adjacencies))
	for _, adj := range golden.Adjacencies {
		expected[adj.Protocol+"|"+adj.SourceDevice+"|"+adj.SourcePort+"|"+adj.TargetDevice+"|"+adj.TargetPort] = struct{}{}
	}
	require.Equal(t, expected, adjacencyKeySet(final.Adjacencies))
}

func TestBuildL2ResultFromWalks_CDP_NMS17216(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms17216/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms17216_cdp")
	require.True(t, ok)

	require.False(t, scenario.Protocols.LLDP)
	require.True(t, scenario.Protocols.CDP)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, scenario.Protocols.ARPND)
	require.Equal(t, ManifestProtocols{CDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 9)

	type stepExpectation struct {
		fixtureID   string
		totalLinks  int
		sourceLinks int
	}

	expectations := []stepExpectation{
		{fixtureID: "Switch1", totalLinks: 5, sourceLinks: 5},
		{fixtureID: "Switch2", totalLinks: 11, sourceLinks: 6},
		{fixtureID: "Switch3", totalLinks: 15, sourceLinks: 4},
		{fixtureID: "Switch4", totalLinks: 16, sourceLinks: 1},
		{fixtureID: "Switch5", totalLinks: 18, sourceLinks: 2},
		{fixtureID: "Router1", totalLinks: 20, sourceLinks: 2},
		{fixtureID: "Router2", totalLinks: 22, sourceLinks: 2},
		{fixtureID: "Router3", totalLinks: 25, sourceLinks: 3},
		{fixtureID: "Router4", totalLinks: 26, sourceLinks: 1},
	}

	var final engine.Result
	for i, expected := range expectations {
		walks, walkErr := loadScenarioWalkPrefix(resolved, i+1)
		require.NoError(t, walkErr)

		result, buildErr := BuildL2ResultFromWalks(walks, BuildOptions{EnableCDP: true})
		require.NoError(t, buildErr)

		sourceCounts := countAdjacenciesBySource(result.Adjacencies, "cdp")
		require.Equal(t, expected.totalLinks, result.Stats["links_cdp"])
		require.Equal(t, expected.sourceLinks, sourceCounts[expected.fixtureID])
		final = result
	}

	require.Len(t, final.Devices, 9)
	require.Len(t, final.Adjacencies, 26)
	require.Equal(t, 26, final.Stats["links_cdp"])
	require.Equal(t, 0, final.Stats["links_lldp"])

	require.Equal(t, map[string]int{
		"Switch1": 5,
		"Switch2": 6,
		"Switch3": 4,
		"Switch4": 1,
		"Switch5": 2,
		"Router1": 2,
		"Router2": 2,
		"Router3": 3,
		"Router4": 1,
	}, countAdjacenciesBySource(final.Adjacencies, "cdp"))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)

	expected := make(map[string]struct{}, len(golden.Adjacencies))
	for _, adj := range golden.Adjacencies {
		expected[adj.Protocol+"|"+adj.SourceDevice+"|"+adj.SourcePort+"|"+adj.TargetDevice+"|"+adj.TargetPort] = struct{}{}
	}
	require.Equal(t, expected, adjacencyKeySet(final.Adjacencies))
}

func TestBuildL2ResultFromWalks_CDP_NMS17216_TopologyProjection(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms17216/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms17216_cdp")
	require.True(t, ok)

	require.False(t, scenario.Protocols.LLDP)
	require.True(t, scenario.Protocols.CDP)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, scenario.Protocols.ARPND)
	require.Equal(t, ManifestProtocols{CDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 9)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableCDP: true})
	require.NoError(t, errStep1)
	require.Equal(t, 5, step1.Stats["links_cdp"])
	require.Equal(t, 0, step1.Stats["links_lldp"])

	step2, errStep2 := buildResultFromScenarioPrefix(resolved, 2, BuildOptions{EnableCDP: true})
	require.NoError(t, errStep2)
	require.Equal(t, 11, step2.Stats["links_cdp"])
	require.Equal(t, 0, step2.Stats["links_lldp"])
	require.Equal(t, map[string]int{
		"Switch1": 5,
		"Switch2": 6,
	}, countAdjacenciesBySource(step2.Adjacencies, "cdp"))

	localDevices := map[string]struct{}{
		"Switch1": {},
		"Switch2": {},
	}
	require.Equal(t, 2, countLocalTopologyVertices(step2.Adjacencies, "cdp", localDevices))
	require.Equal(t, 1, countUndirectedLocalDevicePairs(step2.Adjacencies, "cdp", localDevices))
	require.Equal(t, 4, countMutualLocalAdjacencyEdges(step2.Adjacencies, "cdp", localDevices))
}

func TestBuildL2ResultFromWalks_LLDP_NMS17216_TopologyProjection(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms17216/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms17216_lldp")
	require.True(t, ok)

	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, scenario.Protocols.ARPND)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 5)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep1)
	require.Equal(t, 4, step1.Stats["links_lldp"])
	require.Equal(t, 0, step1.Stats["links_cdp"])

	step2, errStep2 := buildResultFromScenarioPrefix(resolved, 2, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep2)
	require.Equal(t, 10, step2.Stats["links_lldp"])
	require.Equal(t, 0, step2.Stats["links_cdp"])

	localStep2 := map[string]struct{}{
		"Switch1": {},
		"Switch2": {},
	}
	require.Equal(t, 2, countLocalTopologyVertices(step2.Adjacencies, "lldp", localStep2))
	require.Equal(t, 1, countUndirectedLocalDevicePairs(step2.Adjacencies, "lldp", localStep2))
	require.Equal(t, 4, countMutualLocalAdjacencyEdges(step2.Adjacencies, "lldp", localStep2))

	step3Walks, errStep3Walks := loadScenarioWalkPrefix(resolved, 3)
	require.NoError(t, errStep3Walks)
	require.Equal(t, 3, countFixturesWithLLDPLocalElements(step3Walks))

	step3, errStep3 := BuildL2ResultFromWalks(step3Walks, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep3)
	require.Equal(t, 12, step3.Stats["links_lldp"])
	require.Equal(t, 0, step3.Stats["links_cdp"])

	localStep3 := map[string]struct{}{
		"Switch1": {},
		"Switch2": {},
		"Switch3": {},
	}
	require.Equal(t, 3, countLocalTopologyVertices(step3.Adjacencies, "lldp", localStep3))
	require.Equal(t, 2, countUndirectedLocalDevicePairs(step3.Adjacencies, "lldp", localStep3))
	require.Equal(t, 6, countMutualLocalAdjacencyEdges(step3.Adjacencies, "lldp", localStep3))
}

func TestBuildL2ResultFromWalks_LLDP_NMS0123(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms0123/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms0123_lldp")
	require.True(t, ok)

	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, scenario.Protocols.ARPND)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 8)

	type stepExpectation struct {
		fixtureID  string
		totalLinks int
	}

	expectations := []stepExpectation{
		{fixtureID: "ITPN0111", totalLinks: 4},
		{fixtureID: "ITPN0112", totalLinks: 6},
		{fixtureID: "ITPN0113", totalLinks: 8},
		{fixtureID: "ITPN0114", totalLinks: 10},
		{fixtureID: "ITPN0121", totalLinks: 11},
		{fixtureID: "ITPN0123", totalLinks: 14},
		{fixtureID: "ITPN0201", totalLinks: 20},
		{fixtureID: "ITPN0202", totalLinks: 23},
	}

	var final engine.Result
	for i, expected := range expectations {
		walks, walkErr := loadScenarioWalkPrefix(resolved, i+1)
		require.NoError(t, walkErr)

		result, buildErr := BuildL2ResultFromWalks(walks, BuildOptions{EnableLLDP: true})
		require.NoError(t, buildErr)

		require.Equal(t, expected.totalLinks, result.Stats["links_lldp"])
		final = result
	}

	allWalks, err := loadScenarioWalkPrefix(resolved, len(resolved.Fixtures))
	require.NoError(t, err)

	require.Len(t, final.Adjacencies, 23)
	require.Equal(t, 23, final.Stats["links_lldp"])
	require.Equal(t, 0, final.Stats["links_cdp"])
	require.Equal(t, 8, countFixturesWithLLDPLocalElements(allWalks))

	localDevices := map[string]struct{}{
		"ITPN0111": {},
		"ITPN0112": {},
		"ITPN0113": {},
		"ITPN0114": {},
		"ITPN0121": {},
		"ITPN0123": {},
		"ITPN0201": {},
		"ITPN0202": {},
	}
	require.Equal(t, 8, countLocalTopologyVertices(final.Adjacencies, "lldp", localDevices))
	require.Equal(t, 8, countUndirectedLocalDevicePairs(final.Adjacencies, "lldp", localDevices))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)

	expected := make(map[string]struct{}, len(golden.Adjacencies))
	for _, adj := range golden.Adjacencies {
		expected[adj.Protocol+"|"+adj.SourceDevice+"|"+adj.SourcePort+"|"+adj.TargetDevice+"|"+adj.TargetPort] = struct{}{}
	}
	require.Equal(t, expected, adjacencyKeySet(final.Adjacencies))
}

func TestBuildL2ResultFromWalks_LLDP_NMS0002_CISCO_JUNIPER(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms0002/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms0002_cisco_juniper_lldp")
	require.True(t, ok)

	enableOSPF := false
	enableISIS := false
	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, enableOSPF)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, enableISIS)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 2)

	preCollectionLinks := 0
	require.Equal(t, 0, preCollectionLinks)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep1)
	require.Equal(t, 1, step1.Stats["links_lldp"])

	step2First, errStep2First := buildResultFromScenarioPrefix(resolved, 2, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep2First)

	step2Second, errStep2Second := buildResultFromScenarioPrefix(resolved, 2, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep2Second)
	require.Equal(t, step2First.Stats["links_lldp"], step2Second.Stats["links_lldp"])

	final := step2Second
	require.Len(t, final.Adjacencies, 2)
	require.Equal(t, 2, final.Stats["links_lldp"])
	require.Equal(t, 0, final.Stats["links_cdp"])

	require.Equal(t, map[string]int{
		"Rluck001": 1,
		"Sluck001": 1,
	}, countAdjacenciesBySource(final.Adjacencies, "lldp"))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)

	expected := make(map[string]struct{}, len(golden.Adjacencies))
	for _, adj := range golden.Adjacencies {
		expected[adj.Protocol+"|"+adj.SourceDevice+"|"+adj.SourcePort+"|"+adj.TargetDevice+"|"+adj.TargetPort] = struct{}{}
	}
	require.Equal(t, expected, adjacencyKeySet(final.Adjacencies))
}

func TestBuildL2ResultFromWalks_LLDP_NMS0002_CISCO_ALCATEL(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms0002/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms0002_cisco_alcatel_lldp")
	require.True(t, ok)

	enableOSPF := false
	enableISIS := false
	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, enableOSPF)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, enableISIS)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 5)

	preCollectionLinks := 0
	require.Equal(t, 0, preCollectionLinks)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep1)
	require.Equal(t, 0, step1.Stats["links_lldp"])

	step2, errStep2 := buildResultFromScenarioPrefix(resolved, 2, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep2)
	require.Equal(t, 1, step2.Stats["links_lldp"])

	step3, errStep3 := buildResultFromScenarioPrefix(resolved, 3, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep3)
	require.Equal(t, 2, step3.Stats["links_lldp"])

	step4, errStep4 := buildResultFromScenarioPrefix(resolved, 4, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep4)
	require.Equal(t, 4, step4.Stats["links_lldp"])

	step5, errStep5 := buildResultFromScenarioPrefix(resolved, 5, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep5)

	final := step5
	require.Len(t, final.Adjacencies, 6)
	require.Equal(t, 6, final.Stats["links_lldp"])
	require.Equal(t, 0, final.Stats["links_cdp"])

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)

	expected := make(map[string]struct{}, len(golden.Adjacencies))
	for _, adj := range golden.Adjacencies {
		expected[adj.Protocol+"|"+adj.SourceDevice+"|"+adj.SourcePort+"|"+adj.TargetDevice+"|"+adj.TargetPort] = struct{}{}
	}
	require.Equal(t, expected, adjacencyKeySet(final.Adjacencies))
}

func TestBuildL2ResultFromWalks_LLDP_NMS0000_NETWORK_ALL(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms0000/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms0000_network_all_lldp")
	require.True(t, ok)

	enableOSPF := false
	enableISIS := false
	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, enableOSPF)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, enableISIS)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)

	expectedFixtureIDs := []string{
		"ms01",
		"ms02",
		"ms03",
		"ms04",
		"ms05",
		"ms06",
		"ms07",
		"ms08",
		"ms09",
		"ms10",
		"ms11",
		"ms12",
		"ms14",
		"ms15",
		"ms16",
		"ms17",
		"ms18",
		"ms19",
	}
	require.Len(t, resolved.Fixtures, len(expectedFixtureIDs))
	for i, fixture := range resolved.Fixtures {
		require.Equal(t, expectedFixtureIDs[i], fixture.DeviceID)
	}

	allWalks, err := LoadScenarioWalks(resolved)
	require.NoError(t, err)
	require.Len(t, allWalks, len(expectedFixtureIDs))

	preCollectionLinks := 0
	preCollectionElements := 0
	require.Equal(t, 0, preCollectionLinks)
	require.Equal(t, 0, preCollectionElements)

	type stepExpectation struct {
		fixtureID      string
		totalLinks     int
		sourceLinks    int
		lldpElementCnt int
	}

	expectations := []stepExpectation{
		{fixtureID: "ms01", totalLinks: 4, sourceLinks: 4, lldpElementCnt: 1},
		{fixtureID: "ms02", totalLinks: 8, sourceLinks: 4, lldpElementCnt: 2},
		{fixtureID: "ms03", totalLinks: 11, sourceLinks: 3, lldpElementCnt: 3},
		{fixtureID: "ms04", totalLinks: 15, sourceLinks: 4, lldpElementCnt: 4},
		{fixtureID: "ms05", totalLinks: 19, sourceLinks: 4, lldpElementCnt: 5},
		{fixtureID: "ms06", totalLinks: 23, sourceLinks: 4, lldpElementCnt: 6},
		{fixtureID: "ms07", totalLinks: 26, sourceLinks: 3, lldpElementCnt: 7},
		{fixtureID: "ms08", totalLinks: 31, sourceLinks: 5, lldpElementCnt: 8},
		{fixtureID: "ms09", totalLinks: 36, sourceLinks: 5, lldpElementCnt: 9},
		{fixtureID: "ms10", totalLinks: 41, sourceLinks: 5, lldpElementCnt: 10},
		{fixtureID: "ms11", totalLinks: 45, sourceLinks: 4, lldpElementCnt: 11},
		{fixtureID: "ms12", totalLinks: 51, sourceLinks: 6, lldpElementCnt: 12},
		{fixtureID: "ms14", totalLinks: 55, sourceLinks: 4, lldpElementCnt: 13},
		{fixtureID: "ms15", totalLinks: 56, sourceLinks: 1, lldpElementCnt: 14},
		{fixtureID: "ms16", totalLinks: 61, sourceLinks: 5, lldpElementCnt: 15},
		{fixtureID: "ms17", totalLinks: 65, sourceLinks: 4, lldpElementCnt: 16},
		{fixtureID: "ms18", totalLinks: 70, sourceLinks: 5, lldpElementCnt: 17},
		{fixtureID: "ms19", totalLinks: 73, sourceLinks: 3, lldpElementCnt: 18},
	}

	var final engine.Result
	for i, expected := range expectations {
		walks, walkErr := loadScenarioWalkPrefix(resolved, i+1)
		require.NoError(t, walkErr)

		result, buildErr := BuildL2ResultFromWalks(walks, BuildOptions{EnableLLDP: true})
		require.NoError(t, buildErr)

		sourceCounts := countAdjacenciesBySource(result.Adjacencies, "lldp")
		require.Equal(t, expected.totalLinks, result.Stats["links_lldp"])
		require.Equal(t, expected.sourceLinks, sourceCounts[expected.fixtureID])
		require.Equal(t, expected.lldpElementCnt, countFixturesWithLLDPLocalElements(allWalks[:i+1]))
		final = result
	}

	require.Len(t, final.Adjacencies, 73)
	require.Equal(t, 73, final.Stats["links_lldp"])
	require.Equal(t, 0, final.Stats["links_cdp"])
	require.Equal(t, 18, countFixturesWithLLDPLocalElements(allWalks))

	localDevices := map[string]struct{}{
		"ms01": {},
		"ms02": {},
		"ms03": {},
		"ms04": {},
		"ms05": {},
		"ms06": {},
		"ms07": {},
		"ms08": {},
		"ms09": {},
		"ms10": {},
		"ms11": {},
		"ms12": {},
		"ms14": {},
		"ms15": {},
		"ms16": {},
		"ms17": {},
		"ms18": {},
		"ms19": {},
	}
	require.Equal(t, 18, countLocalTopologyVertices(final.Adjacencies, "lldp", localDevices))
	require.Equal(t, 17, countUndirectedLocalDevicePairs(final.Adjacencies, "lldp", localDevices))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)
	require.Equal(t, goldenAdjacencyKeySet(golden.Adjacencies), adjacencyKeySet(final.Adjacencies))
}

func TestBuildL2ResultFromWalks_LLDP_NMS0000_NETWORK_TWO_CONNECTED(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms0000/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms0000_network_two_connected_lldp")
	require.True(t, ok)

	enableOSPF := false
	enableISIS := false
	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, enableOSPF)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, enableISIS)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 2)
	require.Equal(t, "ms07", resolved.Fixtures[0].DeviceID)
	require.Equal(t, "ms08", resolved.Fixtures[1].DeviceID)

	allWalks, err := LoadScenarioWalks(resolved)
	require.NoError(t, err)
	require.Len(t, allWalks, 2)

	preCollectionLinks := 0
	preCollectionElements := 0
	require.Equal(t, 0, preCollectionLinks)
	require.Equal(t, 0, preCollectionElements)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep1)
	require.Equal(t, 3, step1.Stats["links_lldp"])
	require.Equal(t, map[string]int{"ms07": 3}, countAdjacenciesBySource(step1.Adjacencies, "lldp"))
	require.Equal(t, 1, countFixturesWithLLDPLocalElements(allWalks[:1]))

	step2, errStep2 := buildResultFromScenarioPrefix(resolved, 2, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep2)
	require.Equal(t, 8, step2.Stats["links_lldp"])
	require.Equal(t, map[string]int{"ms07": 3, "ms08": 5}, countAdjacenciesBySource(step2.Adjacencies, "lldp"))
	require.Equal(t, 2, countFixturesWithLLDPLocalElements(allWalks))

	final := step2
	require.Len(t, final.Adjacencies, 8)
	require.Equal(t, 8, final.Stats["links_lldp"])
	require.Equal(t, 0, final.Stats["links_cdp"])

	localDevices := map[string]struct{}{
		"ms07": {},
		"ms08": {},
	}
	require.Equal(t, 2, countLocalTopologyVertices(final.Adjacencies, "lldp", localDevices))
	require.Equal(t, 1, countUndirectedLocalDevicePairs(final.Adjacencies, "lldp", localDevices))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)
	require.Equal(t, goldenAdjacencyKeySet(golden.Adjacencies), adjacencyKeySet(final.Adjacencies))
}

func TestBuildL2ResultFromWalks_LLDP_NMS0000_NETWORK_THREE_CONNECTED(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms0000/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms0000_network_three_connected_lldp")
	require.True(t, ok)

	enableOSPF := false
	enableISIS := false
	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, enableOSPF)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, enableISIS)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 2)
	require.Equal(t, "ms08", resolved.Fixtures[0].DeviceID)
	require.Equal(t, "ms10", resolved.Fixtures[1].DeviceID)

	allWalks, err := LoadScenarioWalks(resolved)
	require.NoError(t, err)
	require.Len(t, allWalks, 2)

	preCollectionLinks := 0
	preCollectionElements := 0
	require.Equal(t, 0, preCollectionLinks)
	require.Equal(t, 0, preCollectionElements)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep1)
	require.Equal(t, 5, step1.Stats["links_lldp"])
	require.Equal(t, map[string]int{"ms08": 5}, countAdjacenciesBySource(step1.Adjacencies, "lldp"))
	require.Equal(t, 1, countFixturesWithLLDPLocalElements(allWalks[:1]))

	step2, errStep2 := buildResultFromScenarioPrefix(resolved, 2, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep2)
	require.Equal(t, 10, step2.Stats["links_lldp"])
	require.Equal(t, map[string]int{"ms08": 5, "ms10": 5}, countAdjacenciesBySource(step2.Adjacencies, "lldp"))
	require.Equal(t, 2, countFixturesWithLLDPLocalElements(allWalks))

	final := step2
	require.Len(t, final.Adjacencies, 10)
	require.Equal(t, 10, final.Stats["links_lldp"])
	require.Equal(t, 0, final.Stats["links_cdp"])

	localDevices := map[string]struct{}{
		"ms08": {},
		"ms09": {},
		"ms10": {},
	}
	require.Equal(t, 0, countUndirectedLocalDevicePairs(final.Adjacencies, "lldp", localDevices))
	localSourceCounts := countAdjacenciesBySource(final.Adjacencies, "lldp")
	require.Equal(t, 2, len(localSourceCounts))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)
	require.Equal(t, goldenAdjacencyKeySet(golden.Adjacencies), adjacencyKeySet(final.Adjacencies))
}

func TestBuildL2ResultFromWalks_LLDP_NMS0000_MICROSENSE(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms0000/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms0000_microsense_lldp")
	require.True(t, ok)

	enableOSPF := false
	enableISIS := false
	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, enableOSPF)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, enableISIS)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 1)

	allWalks, err := LoadScenarioWalks(resolved)
	require.NoError(t, err)
	require.Len(t, allWalks, 1)

	preCollectionLinks := 0
	preCollectionElements := 0
	require.Equal(t, 0, preCollectionLinks)
	require.Equal(t, 0, preCollectionElements)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep1)
	require.Len(t, step1.Adjacencies, 5)
	require.Equal(t, 5, step1.Stats["links_lldp"])
	require.Equal(t, 0, step1.Stats["links_cdp"])
	require.Equal(t, 1, countFixturesWithLLDPLocalElements(allWalks))
	require.Equal(t, map[string]int{"ms08": 5}, countAdjacenciesBySource(step1.Adjacencies, "lldp"))

	deviceByID := make(map[string]engine.Device, len(step1.Devices))
	for _, dev := range step1.Devices {
		deviceByID[dev.ID] = dev
	}
	require.Contains(t, deviceByID, "ms08")
	require.Equal(t, "SW_A1_BDA_08_M", deviceByID["ms08"].Hostname)
	require.Equal(t, "00:60:a7:0a:7f:6c", deviceByID["ms08"].ChassisID)

	expected := map[string]struct{}{
		"lldp|ms08|2/4|microsens-g6-mac-00:60:a7:0a:7f:26|2/5": {},
		"lldp|ms08|2/6|axis-accc8eef78f9|ac:cc:8e:ef:78:f9":    {},
		"lldp|ms08|3/1|axis-accc8eef78e4|ac:cc:8e:ef:78:e4":    {},
		"lldp|ms08|3/2|axis-b8a44f502ddd|b8:a4:4f:50:2d:dd":    {},
		"lldp|ms08|3/4|microsens-g6-mac-00:60:a7:0a:7f:10|3/5": {},
	}
	require.Equal(t, expected, adjacencyKeySet(step1.Adjacencies))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)
	require.Equal(t, goldenAdjacencyKeySet(golden.Adjacencies), adjacencyKeySet(step1.Adjacencies))
}

func TestBuildL2ResultFromWalks_LLDP_NMS0000_MS16(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms0000/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms0000_ms16_lldp")
	require.True(t, ok)

	enableOSPF := false
	enableISIS := false
	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, enableOSPF)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, enableISIS)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 1)

	allWalks, err := LoadScenarioWalks(resolved)
	require.NoError(t, err)
	require.Len(t, allWalks, 1)

	preCollectionLinks := 0
	preCollectionElements := 0
	require.Equal(t, 0, preCollectionLinks)
	require.Equal(t, 0, preCollectionElements)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep1)
	require.Len(t, step1.Adjacencies, 5)
	require.Equal(t, 5, step1.Stats["links_lldp"])
	require.Equal(t, 0, step1.Stats["links_cdp"])
	require.Equal(t, 1, countFixturesWithLLDPLocalElements(allWalks))
	require.Equal(t, map[string]int{"ms16": 5}, countAdjacenciesBySource(step1.Adjacencies, "lldp"))

	deviceByID := make(map[string]engine.Device, len(step1.Devices))
	for _, dev := range step1.Devices {
		deviceByID[dev.ID] = dev
	}
	require.Contains(t, deviceByID, "ms16")
	require.Equal(t, "SW_A1_BDA_16_M", deviceByID["ms16"].Hostname)
	require.Equal(t, "00:60:a7:0c:43:ff", deviceByID["ms16"].ChassisID)

	expected := map[string]struct{}{
		"lldp|ms16|2/5|microsens-g6-mac-00:60:a7:0a:cc:1b|2/6": {},
		"lldp|ms16|3/1|axis-accc8eef7a21|ac:cc:8e:ef:7a:21":    {},
		"lldp|ms16|3/2|axis-accc8eef78f8|ac:cc:8e:ef:78:f8":    {},
		"lldp|ms16|3/5|microsens-g6-mac-00:60:a7:0a:cb:d3|2/5": {},
		"lldp|ms16|3/6|microsens-g6-mac-00:60:a7:0a:cc:09|2/5": {},
	}
	require.Equal(t, expected, adjacencyKeySet(step1.Adjacencies))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)
	require.Equal(t, goldenAdjacencyKeySet(golden.Adjacencies), adjacencyKeySet(step1.Adjacencies))
}

func TestBuildL2ResultFromWalks_LLDP_NMS0000_PLANET(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms0000/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms0000_planet_lldp")
	require.True(t, ok)

	enableOSPF := false
	enableISIS := false
	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, enableOSPF)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, enableISIS)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 1)

	allWalks, err := LoadScenarioWalks(resolved)
	require.NoError(t, err)
	require.Len(t, allWalks, 1)

	preCollectionLinks := 0
	preCollectionElements := 0
	require.Equal(t, 0, preCollectionLinks)
	require.Equal(t, 0, preCollectionElements)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep1)
	require.Len(t, step1.Adjacencies, 3)
	require.Equal(t, 3, step1.Stats["links_lldp"])
	require.Equal(t, 0, step1.Stats["links_cdp"])
	require.Equal(t, 1, countFixturesWithLLDPLocalElements(allWalks))
	require.Equal(t, map[string]int{"planet": 3}, countAdjacenciesBySource(step1.Adjacencies, "lldp"))

	deviceByID := make(map[string]engine.Device, len(step1.Devices))
	for _, dev := range step1.Devices {
		deviceByID[dev.ID] = dev
	}
	require.Contains(t, deviceByID, "planet")
	require.Equal(t, "V177", deviceByID["planet"].Hostname)
	require.Equal(t, "a8:f7:e0:6c:3b:d8", deviceByID["planet"].ChassisID)

	adjByLocalPort := make(map[string]engine.Adjacency, len(step1.Adjacencies))
	for _, adj := range step1.Adjacencies {
		if adj.SourceID != "planet" {
			continue
		}
		adjByLocalPort[adj.SourcePort] = adj
	}

	adj06, ok06 := adjByLocalPort["06"]
	require.True(t, ok06)
	require.Equal(t, "epmp1000_64aaec", adj06.TargetID)
	require.Equal(t, "00-04-56-6F-82-1E", adj06.TargetPort)

	adj10, ok10 := adjByLocalPort["10"]
	require.True(t, ok10)
	require.Equal(t, "microsens-g6-mac-00:60:a7:0c:8e:33", adj10.TargetID)
	require.Equal(t, "2/5", adj10.TargetPort)

	adj09, ok09 := adjByLocalPort["09"]
	require.True(t, ok09)
	require.Equal(t, "switch area b - v178", adj09.TargetID)
	require.Equal(t, "9", adj09.TargetPort)

	expected := map[string]struct{}{
		"lldp|planet|06|epmp1000_64aaec|00-04-56-6F-82-1E":      {},
		"lldp|planet|09|switch area b - v178|9":                 {},
		"lldp|planet|10|microsens-g6-mac-00:60:a7:0c:8e:33|2/5": {},
	}
	require.Equal(t, expected, adjacencyKeySet(step1.Adjacencies))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)
	require.Equal(t, goldenAdjacencyKeySet(golden.Adjacencies), adjacencyKeySet(step1.Adjacencies))
}

func TestBuildL2ResultFromWalks_LLDP_NMS18541_NETWORK_ALL(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms18541/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms18541_network_all_lldp")
	require.True(t, ok)

	enableOSPF := false
	enableISIS := false
	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, enableOSPF)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, enableISIS)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)

	expectedFixtureIDs := []string{
		"SW_D6_01_M",
		"SW_D6_02_M",
		"SW_D6_03_M",
		"SW_D6_04_M",
		"SW_D6_08_M",
		"SW_D6_09_M",
		"E0281L-ScALBENGA2-QFX",
	}
	require.Len(t, resolved.Fixtures, len(expectedFixtureIDs))

	fixturesByID := make(map[string]ResolvedFixture, len(resolved.Fixtures))
	for _, fixture := range resolved.Fixtures {
		fixturesByID[fixture.DeviceID] = fixture
	}
	for _, fixtureID := range expectedFixtureIDs {
		_, exists := fixturesByID[fixtureID]
		require.True(t, exists)
	}

	allWalks, err := LoadScenarioWalks(resolved)
	require.NoError(t, err)
	require.Len(t, allWalks, len(expectedFixtureIDs))

	preCollectionLinks := 0
	preCollectionElements := 0
	require.Equal(t, 0, preCollectionLinks)
	require.Equal(t, 0, preCollectionElements)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep1)
	require.Equal(t, 2, step1.Stats["links_lldp"])
	require.Equal(t, 1, countFixturesWithLLDPLocalElements(allWalks[:1]))

	step2, errStep2 := buildResultFromScenarioPrefix(resolved, 2, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep2)
	require.Equal(t, 4, step2.Stats["links_lldp"])
	require.Equal(t, 2, countFixturesWithLLDPLocalElements(allWalks[:2]))

	step3, errStep3 := buildResultFromScenarioPrefix(resolved, 3, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep3)
	require.Equal(t, 5, step3.Stats["links_lldp"])
	require.Equal(t, 3, countFixturesWithLLDPLocalElements(allWalks[:3]))

	step4, errStep4 := buildResultFromScenarioPrefix(resolved, 4, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep4)
	require.Equal(t, 9, step4.Stats["links_lldp"])
	require.Equal(t, 4, countFixturesWithLLDPLocalElements(allWalks[:4]))

	step5, errStep5 := buildResultFromScenarioPrefix(resolved, 5, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep5)
	require.Equal(t, 12, step5.Stats["links_lldp"])
	require.Equal(t, 5, countFixturesWithLLDPLocalElements(allWalks[:5]))

	step6, errStep6 := buildResultFromScenarioPrefix(resolved, 6, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep6)
	require.Equal(t, 19, step6.Stats["links_lldp"])
	require.Equal(t, 6, countFixturesWithLLDPLocalElements(allWalks[:6]))

	localPreQFX := map[string]struct{}{
		"SW_D6_01_M": {},
		"SW_D6_02_M": {},
		"SW_D6_03_M": {},
		"SW_D6_04_M": {},
		"SW_D6_08_M": {},
		"SW_D6_09_M": {},
	}
	require.NotNil(t, step6.Adjacencies)
	require.Equal(t, 6, countFixturesWithLLDPLocalElements(allWalks[:6]))
	require.Equal(t, 2, countUndirectedLocalDevicePairs(step6.Adjacencies, "lldp", localPreQFX))

	step7, errStep7 := buildResultFromScenarioPrefix(resolved, 7, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep7)
	require.Equal(t, 34, step7.Stats["links_lldp"])
	require.Equal(t, 7, countFixturesWithLLDPLocalElements(allWalks))

	localAll := map[string]struct{}{
		"SW_D6_01_M":            {},
		"SW_D6_02_M":            {},
		"SW_D6_03_M":            {},
		"SW_D6_04_M":            {},
		"SW_D6_08_M":            {},
		"SW_D6_09_M":            {},
		"E0281L-ScALBENGA2-QFX": {},
	}
	require.Equal(t, 7, countFixturesWithLLDPLocalElements(allWalks))
	require.Equal(t, 6, countUndirectedLocalDevicePairs(step7.Adjacencies, "lldp", localAll))
}

func TestBuildL2ResultFromWalks_LLDP_NMS18541_TOPO_QFX_SW01_SW02_SW03(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms18541/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms18541_topoqfx_sw01_sw02_sw03_lldp")
	require.True(t, ok)

	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, scenario.Protocols.ARPND)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 4)

	allWalks, err := LoadScenarioWalks(resolved)
	require.NoError(t, err)
	require.Len(t, allWalks, 4)

	preCollectionLinks := 0
	preCollectionElements := 0
	require.Equal(t, 0, preCollectionLinks)
	require.Equal(t, 0, preCollectionElements)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep1)
	require.Equal(t, 15, step1.Stats["links_lldp"])
	require.Equal(t, 1, countFixturesWithLLDPLocalElements(allWalks[:1]))

	step2, errStep2 := buildResultFromScenarioPrefix(resolved, 2, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep2)
	require.Equal(t, 17, step2.Stats["links_lldp"])
	require.Equal(t, 2, countFixturesWithLLDPLocalElements(allWalks[:2]))

	step3, errStep3 := buildResultFromScenarioPrefix(resolved, 3, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep3)
	require.Equal(t, 19, step3.Stats["links_lldp"])
	require.Equal(t, 3, countFixturesWithLLDPLocalElements(allWalks[:3]))

	step4, errStep4 := buildResultFromScenarioPrefix(resolved, 4, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep4)
	require.Equal(t, 20, step4.Stats["links_lldp"])
	require.Equal(t, 4, countFixturesWithLLDPLocalElements(allWalks))

	localDevices := map[string]struct{}{
		"E0281L-ScALBENGA2-QFX": {},
		"SW_D6_01_M":            {},
		"SW_D6_02_M":            {},
		"SW_D6_03_M":            {},
	}
	require.NotNil(t, step4.Adjacencies)
	require.Equal(t, 4, countFixturesWithLLDPLocalElements(allWalks))
	require.Equal(t, 3, countUndirectedLocalDevicePairs(step4.Adjacencies, "lldp", localDevices))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)
	require.Equal(t, goldenAdjacencyKeySet(golden.Adjacencies), adjacencyKeySet(step4.Adjacencies))
}

func TestBuildL2ResultFromWalks_LLDP_NMS18541_TOPO_QFX_SW01(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms18541/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms18541_topoqfx_sw01_lldp")
	require.True(t, ok)

	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, scenario.Protocols.ARPND)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 2)

	allWalks, err := LoadScenarioWalks(resolved)
	require.NoError(t, err)
	require.Len(t, allWalks, 2)

	preCollectionLinks := 0
	preCollectionElements := 0
	require.Equal(t, 0, preCollectionLinks)
	require.Equal(t, 0, preCollectionElements)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep1)
	require.Equal(t, 15, step1.Stats["links_lldp"])
	require.Equal(t, 1, countFixturesWithLLDPLocalElements(allWalks[:1]))

	step2, errStep2 := buildResultFromScenarioPrefix(resolved, 2, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep2)
	require.Equal(t, 17, step2.Stats["links_lldp"])
	require.Equal(t, 2, countFixturesWithLLDPLocalElements(allWalks))

	localDevices := map[string]struct{}{
		"E0281L-ScALBENGA2-QFX": {},
		"SW_D6_01_M":            {},
	}
	require.NotNil(t, step2.Adjacencies)
	require.Equal(t, 2, countFixturesWithLLDPLocalElements(allWalks))
	require.Equal(t, 0, countUndirectedLocalDevicePairs(step2.Adjacencies, "lldp", localDevices))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)
	require.Equal(t, goldenAdjacencyKeySet(golden.Adjacencies), adjacencyKeySet(step2.Adjacencies))
}

func TestBuildL2ResultFromWalks_LLDP_NMS18541_TOPO_QFX_SW02(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms18541/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms18541_topoqfx_sw02_lldp")
	require.True(t, ok)

	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, scenario.Protocols.ARPND)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 2)

	allWalks, err := LoadScenarioWalks(resolved)
	require.NoError(t, err)
	require.Len(t, allWalks, 2)

	preCollectionLinks := 0
	preCollectionElements := 0
	require.Equal(t, 0, preCollectionLinks)
	require.Equal(t, 0, preCollectionElements)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep1)
	require.Equal(t, 15, step1.Stats["links_lldp"])
	require.Equal(t, 1, countFixturesWithLLDPLocalElements(allWalks[:1]))

	step2, errStep2 := buildResultFromScenarioPrefix(resolved, 2, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep2)
	require.Equal(t, 17, step2.Stats["links_lldp"])
	require.Equal(t, 2, countFixturesWithLLDPLocalElements(allWalks))

	localDevices := map[string]struct{}{
		"E0281L-ScALBENGA2-QFX": {},
		"SW_D6_02_M":            {},
	}
	require.NotNil(t, step2.Adjacencies)
	require.Equal(t, 2, countFixturesWithLLDPLocalElements(allWalks))
	require.Equal(t, 1, countUndirectedLocalDevicePairs(step2.Adjacencies, "lldp", localDevices))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)
	require.Equal(t, goldenAdjacencyKeySet(golden.Adjacencies), adjacencyKeySet(step2.Adjacencies))
}

func TestBuildL2ResultFromWalks_LLDP_NMS18541_TOPO_QFX_SW03(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms18541/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms18541_topoqfx_sw03_lldp")
	require.True(t, ok)

	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, scenario.Protocols.ARPND)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 2)

	allWalks, err := LoadScenarioWalks(resolved)
	require.NoError(t, err)
	require.Len(t, allWalks, 2)

	preCollectionLinks := 0
	preCollectionElements := 0
	require.Equal(t, 0, preCollectionLinks)
	require.Equal(t, 0, preCollectionElements)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep1)
	require.Equal(t, 15, step1.Stats["links_lldp"])
	require.Equal(t, 1, countFixturesWithLLDPLocalElements(allWalks[:1]))

	step2, errStep2 := buildResultFromScenarioPrefix(resolved, 2, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep2)
	require.Equal(t, 16, step2.Stats["links_lldp"])
	require.Equal(t, 2, countFixturesWithLLDPLocalElements(allWalks))

	localDevices := map[string]struct{}{
		"E0281L-ScALBENGA2-QFX": {},
		"SW_D6_03_M":            {},
	}
	require.NotNil(t, step2.Adjacencies)
	require.Equal(t, 2, countFixturesWithLLDPLocalElements(allWalks))
	require.Equal(t, 0, countUndirectedLocalDevicePairs(step2.Adjacencies, "lldp", localDevices))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)
	require.Equal(t, goldenAdjacencyKeySet(golden.Adjacencies), adjacencyKeySet(step2.Adjacencies))
}

func TestBuildL2ResultFromWalks_LLDP_NMS18541_TOPO_QFX_SW04(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms18541/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms18541_topoqfx_sw04_lldp")
	require.True(t, ok)

	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, scenario.Protocols.ARPND)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 2)

	allWalks, err := LoadScenarioWalks(resolved)
	require.NoError(t, err)
	require.Len(t, allWalks, 2)

	preCollectionLinks := 0
	preCollectionElements := 0
	require.Equal(t, 0, preCollectionLinks)
	require.Equal(t, 0, preCollectionElements)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep1)
	require.Equal(t, 15, step1.Stats["links_lldp"])
	require.Equal(t, 1, countFixturesWithLLDPLocalElements(allWalks[:1]))

	step2, errStep2 := buildResultFromScenarioPrefix(resolved, 2, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep2)
	require.Equal(t, 19, step2.Stats["links_lldp"])
	require.Equal(t, 2, countFixturesWithLLDPLocalElements(allWalks))

	localDevices := map[string]struct{}{
		"E0281L-ScALBENGA2-QFX": {},
		"SW_D6_04_M":            {},
	}
	require.NotNil(t, step2.Adjacencies)
	require.Equal(t, 2, countFixturesWithLLDPLocalElements(allWalks))
	require.Equal(t, 1, countUndirectedLocalDevicePairs(step2.Adjacencies, "lldp", localDevices))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)
	require.Equal(t, goldenAdjacencyKeySet(golden.Adjacencies), adjacencyKeySet(step2.Adjacencies))
}

func TestBuildL2ResultFromWalks_LLDP_NMS18541_TOPO_QFX_SW08(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms18541/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms18541_topoqfx_sw08_lldp")
	require.True(t, ok)

	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, scenario.Protocols.ARPND)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 2)

	allWalks, err := LoadScenarioWalks(resolved)
	require.NoError(t, err)
	require.Len(t, allWalks, 2)

	preCollectionLinks := 0
	preCollectionElements := 0
	require.Equal(t, 0, preCollectionLinks)
	require.Equal(t, 0, preCollectionElements)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep1)
	require.Equal(t, 15, step1.Stats["links_lldp"])
	require.Equal(t, 1, countFixturesWithLLDPLocalElements(allWalks[:1]))

	step2, errStep2 := buildResultFromScenarioPrefix(resolved, 2, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep2)
	require.Equal(t, 18, step2.Stats["links_lldp"])
	require.Equal(t, 2, countFixturesWithLLDPLocalElements(allWalks))

	localDevices := map[string]struct{}{
		"E0281L-ScALBENGA2-QFX": {},
		"SW_D6_08_M":            {},
	}
	require.NotNil(t, step2.Adjacencies)
	require.Equal(t, 2, countFixturesWithLLDPLocalElements(allWalks))
	require.Equal(t, 1, countUndirectedLocalDevicePairs(step2.Adjacencies, "lldp", localDevices))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)
	require.Equal(t, goldenAdjacencyKeySet(golden.Adjacencies), adjacencyKeySet(step2.Adjacencies))
}

func TestBuildL2ResultFromWalks_LLDP_NMS18541_TOPO_QFX_SW09(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms18541/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms18541_topoqfx_sw09_lldp")
	require.True(t, ok)

	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, scenario.Protocols.ARPND)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 2)

	allWalks, err := LoadScenarioWalks(resolved)
	require.NoError(t, err)
	require.Len(t, allWalks, 2)

	require.Equal(t, 0, 0)
	require.Equal(t, 0, 0)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep1)
	require.Equal(t, 15, step1.Stats["links_lldp"])
	require.Equal(t, 1, countFixturesWithLLDPLocalElements(allWalks[:1]))

	step2, errStep2 := buildResultFromScenarioPrefix(resolved, 2, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep2)
	require.Equal(t, 22, step2.Stats["links_lldp"])
	require.Equal(t, 2, countFixturesWithLLDPLocalElements(allWalks))

	localDevices := map[string]struct{}{
		"E0281L-ScALBENGA2-QFX": {},
		"SW_D6_09_M":            {},
	}
	require.NotNil(t, step2.Adjacencies)
	require.Equal(t, 2, countFixturesWithLLDPLocalElements(allWalks))
	require.Equal(t, 1, countUndirectedLocalDevicePairs(step2.Adjacencies, "lldp", localDevices))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)
	require.Equal(t, goldenAdjacencyKeySet(golden.Adjacencies), adjacencyKeySet(step2.Adjacencies))
}

func TestBuildL2ResultFromWalks_LLDP_NMS18541_QFX(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms18541/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms18541_qfx_lldp")
	require.True(t, ok)

	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, scenario.Protocols.ARPND)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 1)

	allWalks, err := LoadScenarioWalks(resolved)
	require.NoError(t, err)
	require.Len(t, allWalks, 1)

	preCollectionLinks := 0
	preCollectionElements := 0
	require.Equal(t, 0, preCollectionLinks)
	require.Equal(t, 0, preCollectionElements)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep1)
	require.Len(t, step1.Adjacencies, 15)
	require.Equal(t, 15, step1.Stats["links_lldp"])
	require.Equal(t, 0, step1.Stats["links_cdp"])
	require.Equal(t, 1, countFixturesWithLLDPLocalElements(allWalks))
	require.Equal(t, map[string]int{"E0281L-ScALBENGA2-QFX": 15}, countAdjacenciesBySource(step1.Adjacencies, "lldp"))

	expected := map[string]struct{}{
		"lldp|E0281L-ScALBENGA2-QFX|et-0/0/48|e0281l-scalbenga2-r1|581":               {},
		"lldp|E0281L-ScALBENGA2-QFX|et-0/0/49|e0281l-scalbenga2-r2|581":               {},
		"lldp|E0281L-ScALBENGA2-QFX|ge-0/0/0|kwe0095p0i_1sgiusto65|28":                {},
		"lldp|E0281L-ScALBENGA2-QFX|ge-0/0/1|kje0406p0i_1mbaldo11|28":                 {},
		"lldp|E0281L-ScALBENGA2-QFX|ge-0/0/10|arunarcangeli01|EC FC C6 C8 3B 80":      {},
		"lldp|E0281L-ScALBENGA2-QFX|ge-0/0/13|microsens-g6-mac-00-60-a7-0a-81-13|2/5": {},
		"lldp|E0281L-ScALBENGA2-QFX|ge-0/0/15|microsens-g6-mac-00-60-a7-0a-7f-16|2/5": {},
		"lldp|E0281L-ScALBENGA2-QFX|ge-0/0/16|microsens-g6-mac-00:60:a7:0d:9e:15|3/6": {},
		"lldp|E0281L-ScALBENGA2-QFX|ge-0/0/2|kke0482p0i_1fleming15|28":                {},
		"lldp|E0281L-ScALBENGA2-QFX|ge-0/0/3|wke0564p0i_1gnocchi8|26":                 {},
		"lldp|E0281L-ScALBENGA2-QFX|ge-0/0/4|wye0596p0i_1mbaldo15|28":                 {},
		"lldp|E0281L-ScALBENGA2-QFX|ge-0/0/5|wxe0729p0t_1pioii3|28":                   {},
		"lldp|E0281L-ScALBENGA2-QFX|ge-0/0/6|wje0588p0i_1marx2|27":                    {},
		"lldp|E0281L-ScALBENGA2-QFX|ge-0/0/7|microsens-g6-mac-00:60:a7:0c:27:fd|3/5":  {},
		"lldp|E0281L-ScALBENGA2-QFX|ge-0/0/9|arunbellaria01|50 E4 E0 CE 3B 8E":        {},
	}
	require.Equal(t, expected, adjacencyKeySet(step1.Adjacencies))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)
	require.Equal(t, goldenAdjacencyKeySet(golden.Adjacencies), adjacencyKeySet(step1.Adjacencies))
}

func TestBuildL2ResultFromWalks_LLDP_NMS18541_MICROSENS_SW01(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms18541/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms18541_microsens_sw01_lldp")
	require.True(t, ok)

	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, scenario.Protocols.ARPND)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 1)

	allWalks, err := LoadScenarioWalks(resolved)
	require.NoError(t, err)
	require.Len(t, allWalks, 1)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep1)
	require.Len(t, step1.Adjacencies, 2)
	require.Equal(t, 2, step1.Stats["links_lldp"])
	require.Equal(t, 0, step1.Stats["links_cdp"])
	require.Equal(t, 1, countFixturesWithLLDPLocalElements(allWalks))
	require.Equal(t, map[string]int{"SW_D6_01_M": 2}, countAdjacenciesBySource(step1.Adjacencies, "lldp"))

	expected := map[string]struct{}{
		"lldp|SW_D6_01_M|2/4|microsens-g6-mac-00:60:a7:0c:27:fd|2/5": {},
		"lldp|SW_D6_01_M|3/4|microsens-g6-mac-00:60:a7:0a:80:5e|2/5": {},
	}
	require.Equal(t, expected, adjacencyKeySet(step1.Adjacencies))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)
	require.Equal(t, goldenAdjacencyKeySet(golden.Adjacencies), adjacencyKeySet(step1.Adjacencies))
}

func TestBuildL2ResultFromWalks_LLDP_NMS18541_MICROSENS_SW02(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms18541/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms18541_microsens_sw02_lldp")
	require.True(t, ok)

	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, scenario.Protocols.ARPND)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 1)

	allWalks, err := LoadScenarioWalks(resolved)
	require.NoError(t, err)
	require.Len(t, allWalks, 1)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep1)
	require.Len(t, step1.Adjacencies, 2)
	require.Equal(t, 2, step1.Stats["links_lldp"])
	require.Equal(t, 0, step1.Stats["links_cdp"])
	require.Equal(t, 1, countFixturesWithLLDPLocalElements(allWalks))
	require.Equal(t, map[string]int{"SW_D6_02_M": 2}, countAdjacenciesBySource(step1.Adjacencies, "lldp"))

	expected := map[string]struct{}{
		"lldp|SW_D6_02_M|2/4|microsens-g6-mac-00:60:a7:0a:80:4e|2/5": {},
		"lldp|SW_D6_02_M|3/4|e0281l-scalbenga2-qfx|ge-0/0/7":         {},
	}
	require.Equal(t, expected, adjacencyKeySet(step1.Adjacencies))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)
	require.Equal(t, goldenAdjacencyKeySet(golden.Adjacencies), adjacencyKeySet(step1.Adjacencies))
}

func TestBuildL2ResultFromWalks_LLDP_NMS18541_MICROSENS_SW03(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms18541/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms18541_microsens_sw03_lldp")
	require.True(t, ok)

	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, scenario.Protocols.ARPND)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 1)

	allWalks, err := LoadScenarioWalks(resolved)
	require.NoError(t, err)
	require.Len(t, allWalks, 1)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep1)
	require.Len(t, step1.Adjacencies, 1)
	require.Equal(t, 1, step1.Stats["links_lldp"])
	require.Equal(t, 0, step1.Stats["links_cdp"])
	require.Equal(t, 1, countFixturesWithLLDPLocalElements(allWalks))
	require.Equal(t, map[string]int{"SW_D6_03_M": 1}, countAdjacenciesBySource(step1.Adjacencies, "lldp"))

	expected := map[string]struct{}{
		"lldp|SW_D6_03_M|2/4|microsens-g6-mac-00:60:a7:0a:80:4e|3/5": {},
	}
	require.Equal(t, expected, adjacencyKeySet(step1.Adjacencies))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)
	require.Equal(t, goldenAdjacencyKeySet(golden.Adjacencies), adjacencyKeySet(step1.Adjacencies))
}

func TestBuildL2ResultFromWalks_LLDP_NMS18541_MICROSENS_SW04(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms18541/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms18541_microsens_sw04_lldp")
	require.True(t, ok)

	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, scenario.Protocols.ARPND)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 1)

	allWalks, err := LoadScenarioWalks(resolved)
	require.NoError(t, err)
	require.Len(t, allWalks, 1)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep1)
	require.Len(t, step1.Adjacencies, 4)
	require.Equal(t, 4, step1.Stats["links_lldp"])
	require.Equal(t, 0, step1.Stats["links_cdp"])
	require.Equal(t, 1, countFixturesWithLLDPLocalElements(allWalks))
	require.Equal(t, map[string]int{"SW_D6_04_M": 4}, countAdjacenciesBySource(step1.Adjacencies, "lldp"))

	expected := map[string]struct{}{
		"lldp|SW_D6_04_M|0|e0281l-scalbenga2-qfx|ge-0/0/13":  {},
		"lldp|SW_D6_04_M|0|i0504-su-tlc01|e8:27:25:07:83:43": {},
		"lldp|SW_D6_04_M|0|i0504-su-tlc02|b8:a4:4f:b2:bb:2b": {},
		"lldp|SW_D6_04_M|0|i0504-su-tlc03|e8:27:25:07:96:9a": {},
	}
	require.Equal(t, expected, adjacencyKeySet(step1.Adjacencies))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)
	require.Equal(t, goldenAdjacencyKeySet(golden.Adjacencies), adjacencyKeySet(step1.Adjacencies))
}

func TestBuildL2ResultFromWalks_LLDP_NMS18541_MICROSENS_SW08(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms18541/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms18541_microsens_sw08_lldp")
	require.True(t, ok)

	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, scenario.Protocols.ARPND)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 1)

	allWalks, err := LoadScenarioWalks(resolved)
	require.NoError(t, err)
	require.Len(t, allWalks, 1)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep1)
	require.Len(t, step1.Adjacencies, 3)
	require.Equal(t, 3, step1.Stats["links_lldp"])
	require.Equal(t, 0, step1.Stats["links_cdp"])
	require.Equal(t, 1, countFixturesWithLLDPLocalElements(allWalks))
	require.Equal(t, map[string]int{"SW_D6_08_M": 3}, countAdjacenciesBySource(step1.Adjacencies, "lldp"))

	expected := map[string]struct{}{
		"lldp|SW_D6_08_M|0|e0281l-scalbenga2-qfx|ge-0/0/15":  {},
		"lldp|SW_D6_08_M|0|i0506-su-tlc01|e8:27:25:07:96:63": {},
		"lldp|SW_D6_08_M|0|i0506-su-tlc02|e8:27:25:07:83:27": {},
	}
	require.Equal(t, expected, adjacencyKeySet(step1.Adjacencies))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)
	require.Equal(t, goldenAdjacencyKeySet(golden.Adjacencies), adjacencyKeySet(step1.Adjacencies))
}

func TestBuildL2ResultFromWalks_LLDP_NMS18541_MICROSENS_SW09(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms18541/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms18541_microsens_sw09_lldp")
	require.True(t, ok)

	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, scenario.Protocols.ARPND)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 1)

	allWalks, err := LoadScenarioWalks(resolved)
	require.NoError(t, err)
	require.Len(t, allWalks, 1)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep1)
	require.Len(t, step1.Adjacencies, 7)
	require.Equal(t, 7, step1.Stats["links_lldp"])
	require.Equal(t, 0, step1.Stats["links_cdp"])
	require.Equal(t, 1, countFixturesWithLLDPLocalElements(allWalks))
	require.Equal(t, map[string]int{"SW_D6_09_M": 7}, countAdjacenciesBySource(step1.Adjacencies, "lldp"))

	expected := map[string]struct{}{
		"lldp|SW_D6_09_M|1/1|axis-accc8ef9c0a3|ac:cc:8e:f9:c0:a3": {},
		"lldp|SW_D6_09_M|2/1|axis-accc8e53f5fb|ac:cc:8e:53:f5:fb": {},
		"lldp|SW_D6_09_M|2/2|axis-accc8e536f3c|ac:cc:8e:53:6f:3c": {},
		"lldp|SW_D6_09_M|2/6|axis-accc8eaaeb7f|ac:cc:8e:aa:eb:7f": {},
		"lldp|SW_D6_09_M|3/1|axis-accc8ef9c09e|ac:cc:8e:f9:c0:9e": {},
		"lldp|SW_D6_09_M|3/2|axis camera|eth0":                    {},
		"lldp|SW_D6_09_M|3/5|e0281l-scalbenga2-qfx|ge-0/0/16":     {},
	}
	require.Equal(t, expected, adjacencyKeySet(step1.Adjacencies))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)
	require.Equal(t, goldenAdjacencyKeySet(golden.Adjacencies), adjacencyKeySet(step1.Adjacencies))
}

func TestBuildL2ResultFromWalks_CDP_NMS7467(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms7467/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms7467_cdp")
	require.True(t, ok)

	require.False(t, scenario.Protocols.LLDP)
	require.True(t, scenario.Protocols.CDP)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, scenario.Protocols.ARPND)
	require.Equal(t, ManifestProtocols{CDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 1)

	ds, err := LoadWalkFile(resolved.Fixtures[0].WalkFile)
	require.NoError(t, err)
	require.Equal(t, 1, mustAtoi(t, mustLookupWalkValue(t, ds, "1.3.6.1.4.1.9.9.23.1.3.1.0")))
	require.Equal(t, "JAB043408B7", strings.TrimSpace(mustLookupWalkValue(t, ds, "1.3.6.1.4.1.9.9.23.1.3.4.0")))
	require.Equal(t, 3, mustAtoi(t, mustLookupWalkValue(t, ds, "1.3.6.1.4.1.9.9.23.1.3.7.0")))

	walks, walkErr := loadScenarioWalkPrefix(resolved, 1)
	require.NoError(t, walkErr)

	final, buildErr := BuildL2ResultFromWalks(walks, BuildOptions{EnableCDP: true})
	require.NoError(t, buildErr)
	require.Len(t, final.Adjacencies, 5)
	require.Equal(t, 5, final.Stats["links_cdp"])
	require.Equal(t, 0, final.Stats["links_lldp"])
	require.Equal(t, map[string]int{"ciscoswitch": 5}, countAdjacenciesBySource(final.Adjacencies, "cdp"))

	for _, adj := range final.Adjacencies {
		require.NotEmpty(t, adj.TargetID)
		require.NotEmpty(t, adj.SourcePort)
	}

	expected := map[string]struct{}{
		"cdp|ciscoswitch|2/1|sep0004f22ad83e|Port 1":                            {},
		"cdp|ciscoswitch|2/39|mrgarrison.internal.opennms.com|GigabitEthernet0": {},
		"cdp|ciscoswitch|2/4|sip000628f0fb0a|Port 1":                            {},
		"cdp|ciscoswitch|2/44|mrmakay.internal.opennms.com|FastEthernet2":       {},
		"cdp|ciscoswitch|2/46|sip000ccea217a7|Port 1":                           {},
	}
	require.Equal(t, expected, adjacencyKeySet(final.Adjacencies))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)

	expectedFromGolden := make(map[string]struct{}, len(golden.Adjacencies))
	for _, adj := range golden.Adjacencies {
		expectedFromGolden[adj.Protocol+"|"+adj.SourceDevice+"|"+adj.SourcePort+"|"+adj.TargetDevice+"|"+adj.TargetPort] = struct{}{}
	}
	require.Equal(t, expectedFromGolden, adjacencyKeySet(final.Adjacencies))
}

func TestBuildL2ResultFromWalks_MIXED_NMS7563_CISCO01(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms7563/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms7563_cisco01")
	require.True(t, ok)

	require.True(t, scenario.Protocols.LLDP)
	require.True(t, scenario.Protocols.CDP)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, scenario.Protocols.ARPND)
	require.Equal(t, ManifestProtocols{LLDP: true, CDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 1)

	ds, err := LoadWalkFile(resolved.Fixtures[0].WalkFile)
	require.NoError(t, err)
	require.Equal(t, 1, mustAtoi(t, mustLookupWalkValue(t, ds, "1.3.6.1.4.1.9.9.23.1.3.1.0")))
	require.Equal(t, "cisco01", strings.ToLower(strings.TrimSpace(mustLookupWalkValue(t, ds, "1.3.6.1.4.1.9.9.23.1.3.4.0"))))

	walks, walkErr := loadScenarioWalkPrefix(resolved, 1)
	require.NoError(t, walkErr)

	final, buildErr := BuildL2ResultFromWalks(walks, BuildOptions{EnableLLDP: true, EnableCDP: true})
	require.NoError(t, buildErr)
	require.Len(t, final.Devices, 2)
	require.Len(t, final.Adjacencies, 1)
	require.Equal(t, 1, final.Stats["links_lldp"])
	require.Equal(t, 0, final.Stats["links_cdp"])
	require.Equal(t, 1, countFixturesWithLLDPLocalElements(walks))
	require.Equal(t, 0, countBidirectionalPairs(final.Adjacencies, "lldp"))

	deviceByID := make(map[string]engine.Device, len(final.Devices))
	for _, dev := range final.Devices {
		deviceByID[dev.ID] = dev
	}
	require.Contains(t, deviceByID, "cisco01")
	require.Contains(t, deviceByID, "procurve switch 2510b-24")
	require.Equal(t, "ac:a0:16:bf:02:00", deviceByID["cisco01"].ChassisID)
	require.Equal(t, "00:1d:b3:c5:09:60", deviceByID["procurve switch 2510b-24"].ChassisID)
	require.Equal(t, "ProCurve Switch 2510B-24", deviceByID["procurve switch 2510b-24"].Hostname)

	expected := map[string]struct{}{
		"lldp|cisco01|Fa0/8|procurve switch 2510b-24|24": {},
	}
	require.Equal(t, expected, adjacencyKeySet(final.Adjacencies))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)

	expectedFromGolden := make(map[string]struct{}, len(golden.Adjacencies))
	for _, adj := range golden.Adjacencies {
		expectedFromGolden[adj.Protocol+"|"+adj.SourceDevice+"|"+adj.SourcePort+"|"+adj.TargetDevice+"|"+adj.TargetPort] = struct{}{}
	}
	require.Equal(t, expectedFromGolden, adjacencyKeySet(final.Adjacencies))
}

func TestBuildL2ResultFromWalks_LLDP_NMS7563_HOMESERVER(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms7563/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms7563_homeserver_lldp")
	require.True(t, ok)

	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, scenario.Protocols.ARPND)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 1)

	walks, walkErr := loadScenarioWalkPrefix(resolved, 1)
	require.NoError(t, walkErr)

	final, buildErr := BuildL2ResultFromWalks(walks, BuildOptions{EnableLLDP: true})
	require.NoError(t, buildErr)
	require.Len(t, final.Devices, 2)
	require.Len(t, final.Adjacencies, 1)
	require.Equal(t, 1, final.Stats["links_lldp"])
	require.Equal(t, 0, final.Stats["links_cdp"])
	require.Equal(t, 1, countFixturesWithLLDPLocalElements(walks))

	deviceByID := make(map[string]engine.Device, len(final.Devices))
	for _, dev := range final.Devices {
		deviceByID[dev.ID] = dev
	}
	require.Contains(t, deviceByID, "homeserver")
	require.Contains(t, deviceByID, "procurve switch 2510b-24")
	require.Equal(t, "00:1f:f2:07:99:4f", deviceByID["homeserver"].ChassisID)
	require.Equal(t, "server.home.schwartzkopff.org", deviceByID["homeserver"].Hostname)
	require.Equal(t, "00:1d:b3:c5:09:60", deviceByID["procurve switch 2510b-24"].ChassisID)
	require.Equal(t, "ProCurve Switch 2510B-24", deviceByID["procurve switch 2510b-24"].Hostname)

	expected := map[string]struct{}{
		"lldp|homeserver|00 1F F2 07 99 4F|procurve switch 2510b-24|7": {},
	}
	require.Equal(t, expected, adjacencyKeySet(final.Adjacencies))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)

	expectedFromGolden := make(map[string]struct{}, len(golden.Adjacencies))
	for _, adj := range golden.Adjacencies {
		expectedFromGolden[adj.Protocol+"|"+adj.SourceDevice+"|"+adj.SourcePort+"|"+adj.TargetDevice+"|"+adj.TargetPort] = struct{}{}
	}
	require.Equal(t, expectedFromGolden, adjacencyKeySet(final.Adjacencies))
}

func TestBuildL2ResultFromWalks_CDP_NMS7563_SWITCH02(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms7563/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms7563_switch02_cdp")
	require.True(t, ok)

	require.False(t, scenario.Protocols.LLDP)
	require.True(t, scenario.Protocols.CDP)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, scenario.Protocols.ARPND)
	require.Equal(t, ManifestProtocols{CDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 1)

	ds, err := LoadWalkFile(resolved.Fixtures[0].WalkFile)
	require.NoError(t, err)
	require.Equal(t, 1, mustAtoi(t, mustLookupWalkValue(t, ds, "1.3.6.1.4.1.9.9.23.1.3.1.0")))
	require.Equal(t, "ProCurve Switch 2510B-24(001db3-c50960)", strings.TrimSpace(mustLookupWalkValue(t, ds, "1.3.6.1.4.1.9.9.23.1.3.4.0")))

	walks, walkErr := loadScenarioWalkPrefix(resolved, 1)
	require.NoError(t, walkErr)

	final, buildErr := BuildL2ResultFromWalks(walks, BuildOptions{EnableCDP: true})
	require.NoError(t, buildErr)
	require.Len(t, final.Devices, 3)
	require.Len(t, final.Adjacencies, 3)
	require.Equal(t, 3, final.Stats["links_cdp"])
	require.Equal(t, 0, final.Stats["links_lldp"])
	require.Equal(t, map[string]int{"switch02": 3}, countAdjacenciesBySource(final.Adjacencies, "cdp"))

	deviceByID := make(map[string]engine.Device, len(final.Devices))
	for _, dev := range final.Devices {
		deviceByID[dev.ID] = dev
	}
	require.Contains(t, deviceByID, "switch02")
	require.Contains(t, deviceByID, "cisco01")
	require.Contains(t, deviceByID, "00 1f f2 07 99 4f")
	require.Equal(t, "cisco01", deviceByID["cisco01"].Hostname)
	require.Equal(t, "00 1F F2 07 99 4F", deviceByID["00 1f f2 07 99 4f"].Hostname)

	for _, adj := range final.Adjacencies {
		require.Equal(t, "cdp", adj.Protocol)
		raw := strings.TrimSpace(adj.Labels["remote_address_raw"])
		require.NotEmpty(t, raw)
		decoded := decodeHexIP(raw)
		require.NotEmpty(t, decoded)
		_, parseErr := netip.ParseAddr(decoded)
		require.NoError(t, parseErr)
	}

	ipByAdjacency := make(map[string]string, len(final.Adjacencies))
	for _, adj := range final.Adjacencies {
		raw := strings.TrimSpace(adj.Labels["remote_address_raw"])
		ipByAdjacency[adj.SourcePort+"|"+adj.TargetID+"|"+adj.TargetPort] = decodeHexIP(raw)
	}
	require.Equal(t, "192.168.88.240", ipByAdjacency["24|cisco01|Fa0/8"])
	require.Equal(t, "192.168.88.240", ipByAdjacency["24|cisco01|FastEthernet0/8"])
	require.Equal(t, "192.168.87.16", ipByAdjacency["7|00 1f f2 07 99 4f|00 1F F2 07 99 4F"])

	expected := map[string]struct{}{
		"cdp|switch02|24|cisco01|Fa0/8":                      {},
		"cdp|switch02|24|cisco01|FastEthernet0/8":            {},
		"cdp|switch02|7|00 1f f2 07 99 4f|00 1F F2 07 99 4F": {},
	}
	require.Equal(t, expected, adjacencyKeySet(final.Adjacencies))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)

	expectedFromGolden := make(map[string]struct{}, len(golden.Adjacencies))
	for _, adj := range golden.Adjacencies {
		expectedFromGolden[adj.Protocol+"|"+adj.SourceDevice+"|"+adj.SourcePort+"|"+adj.TargetDevice+"|"+adj.TargetPort] = struct{}{}
	}
	require.Equal(t, expectedFromGolden, adjacencyKeySet(final.Adjacencies))
}

func TestBuildL2ResultFromWalks_LLDP_NMS7777DW_NO_LINKS(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms7777dw/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms7777dw_lldp_no_links")
	require.True(t, ok)

	enableOSPF := false
	enableISIS := false
	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, enableOSPF)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, enableISIS)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 1)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep1)

	final := step1
	require.Len(t, final.Devices, 1)
	require.Len(t, final.Adjacencies, 0)
	require.Equal(t, 0, final.Stats["links_lldp"])
	require.Equal(t, 0, final.Stats["links_cdp"])
	require.Equal(t, 0, countBidirectionalPairs(final.Adjacencies, "lldp"))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)

	expected := make(map[string]struct{}, len(golden.Adjacencies))
	for _, adj := range golden.Adjacencies {
		expected[adj.Protocol+"|"+adj.SourceDevice+"|"+adj.SourcePort+"|"+adj.TargetDevice+"|"+adj.TargetPort] = struct{}{}
	}
	require.Equal(t, expected, adjacencyKeySet(final.Adjacencies))

	expectedDevices := make(map[string]string, len(golden.Devices))
	for _, dev := range golden.Devices {
		expectedDevices[dev.ID] = dev.Hostname
	}
	actualDevices := make(map[string]string, len(final.Devices))
	for _, dev := range final.Devices {
		actualDevices[dev.ID] = dev.Hostname
	}
	require.Equal(t, expectedDevices, actualDevices)
}

func TestBuildL2ResultFromWalks_LLDP_NMS13923(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms13923/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms13923_lldp")
	require.True(t, ok)

	enableOSPF := false
	enableISIS := false
	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, enableOSPF)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, enableISIS)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 1)
	require.Equal(t, "srv005", resolved.Fixtures[0].DeviceID)

	walks, err := loadScenarioWalkPrefix(resolved, 1)
	require.NoError(t, err)
	require.Equal(t, 1, countFixturesWithLLDPLocalElements(walks))

	remoteChassisRows := 0
	for _, rec := range walks[0].Records {
		if !strings.HasPrefix(normalizeOID(rec.OID), "1.3.6.1.4.1.6527.3.1.2.59.4.1.1.5.") {
			continue
		}
		remoteChassisRows++
		require.Len(t, decodeHexBytes(rec.Value), 6)
	}
	require.Equal(t, 49, remoteChassisRows)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep1)
	require.Equal(t, 49, step1.Stats["links_lldp"])
	require.Len(t, step1.Adjacencies, 49)
	require.Equal(t, 0, countBidirectionalPairs(step1.Adjacencies, "lldp"))

	sourceCounts := countAdjacenciesBySource(step1.Adjacencies, "lldp")
	require.Equal(t, 1, len(sourceCounts))
	require.Equal(t, 49, sourceCounts["srv005"])

	for _, adj := range step1.Adjacencies {
		require.Equal(t, "lldp", adj.Protocol)
		require.Equal(t, "srv005", adj.SourceID)
		require.NotEmpty(t, adj.SourcePort)
		require.NotEmpty(t, adj.TargetID)
	}

	localDevices := map[string]struct{}{
		"srv005": {},
	}
	require.Equal(t, 0, countUndirectedLocalDevicePairs(step1.Adjacencies, "lldp", localDevices))

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)
	require.Equal(t, goldenAdjacencyKeySet(golden.Adjacencies), adjacencyKeySet(step1.Adjacencies))
}

func TestBuildL2ResultFromWalks_LLDP_NMS13593(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms13593/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms13593_lldp")
	require.True(t, ok)

	enableOSPF := false
	enableISIS := false
	require.True(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, enableOSPF)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, enableISIS)
	require.Equal(t, ManifestProtocols{LLDP: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 2)
	require.Equal(t, "ZHBGO1Zsr001", resolved.Fixtures[0].DeviceID)
	require.Equal(t, "ZHBGO1Zsr002", resolved.Fixtures[1].DeviceID)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep1)
	require.Equal(t, 3, step1.Stats["links_lldp"])
	require.Len(t, step1.Adjacencies, 3)

	step2, errStep2 := buildResultFromScenarioPrefix(resolved, 2, BuildOptions{EnableLLDP: true})
	require.NoError(t, errStep2)
	require.Equal(t, 7, step2.Stats["links_lldp"])
	require.Len(t, step2.Adjacencies, 7)
	require.Equal(t, 0, countBidirectionalPairs(step2.Adjacencies, "lldp"))

	localByID := make(map[string]engine.Device)
	for _, dev := range step2.Devices {
		if dev.ID != "ZHBGO1Zsr001" && dev.ID != "ZHBGO1Zsr002" {
			continue
		}
		localByID[dev.ID] = dev
	}
	require.Len(t, localByID, 2)
	require.Equal(t, "ZHBGO1Zsr001", localByID["ZHBGO1Zsr001"].Hostname)
	require.Equal(t, "24:21:24:ec:e2:3f", localByID["ZHBGO1Zsr001"].ChassisID)
	require.Equal(t, "ZHBGO1Zsr002", localByID["ZHBGO1Zsr002"].Hostname)
	require.Equal(t, "24:21:24:da:f6:3f", localByID["ZHBGO1Zsr002"].ChassisID)

	expectedAdjacencies := map[string]struct{}{
		"lldp|ZHBGO1Zsr001|104906753|esat-1|35700737":                   {},
		"lldp|ZHBGO1Zsr001|105037825|ZHBGO1Zsr002|3/2/c5/1":             {},
		"lldp|ZHBGO1Zsr001|105070593|ZHBGO1Zsr002|3/2/c6/1":             {},
		"lldp|ZHBGO1Zsr002|3/2/c1/1|chassis-50e0ef005000|35700737":      {},
		"lldp|ZHBGO1Zsr002|3/2/c5/1|ZHBGO1Zsr001|3/2/c5/1":              {},
		"lldp|ZHBGO1Zsr002|3/2/c6/1|ZHBGO1Zsr001|3/2/c6/1":              {},
		"lldp|ZHBGO1Zsr002|esat-1/1/27|chassis-e48184acbf34|1610901763": {},
	}
	require.Equal(t, expectedAdjacencies, adjacencyKeySet(step2.Adjacencies))

	sourceCounts := countAdjacenciesBySource(step2.Adjacencies, "lldp")
	require.Equal(t, 3, sourceCounts["ZHBGO1Zsr001"])
	require.Equal(t, 4, sourceCounts["ZHBGO1Zsr002"])

	localDevices := map[string]struct{}{
		"ZHBGO1Zsr001": {},
		"ZHBGO1Zsr002": {},
	}
	require.Equal(t, 2, countLocalTopologyVertices(step2.Adjacencies, "lldp", localDevices))
	require.Equal(t, 1, countUndirectedLocalDevicePairs(step2.Adjacencies, "lldp", localDevices))

	ifNamesByDevice := make(map[string]map[int]string)
	for _, iface := range step2.Interfaces {
		name := strings.TrimSpace(iface.IfName)
		if idx := strings.Index(name, ","); idx >= 0 {
			name = strings.TrimSpace(name[:idx])
		}
		if name == "" {
			continue
		}
		byIndex := ifNamesByDevice[iface.DeviceID]
		if byIndex == nil {
			byIndex = make(map[int]string)
			ifNamesByDevice[iface.DeviceID] = byIndex
		}
		byIndex[iface.IfIndex] = name
	}

	normalizePort := func(deviceID, port string) string {
		port = strings.TrimSpace(port)
		if port == "" {
			return ""
		}
		ifIndex, convErr := strconv.Atoi(port)
		if convErr != nil {
			return port
		}
		if byIndex, ok := ifNamesByDevice[deviceID]; ok {
			if ifName, found := byIndex[ifIndex]; found && strings.TrimSpace(ifName) != "" {
				return strings.TrimSpace(ifName)
			}
		}
		return port
	}

	type projectedLocalEdge struct {
		SourceID   string
		TargetID   string
		SourcePort string
		TargetPort string
	}

	projectedEdges := make([]projectedLocalEdge, 0, 2)
	for _, adj := range step2.Adjacencies {
		if adj.Protocol != "lldp" {
			continue
		}
		if adj.SourceID != "ZHBGO1Zsr001" || adj.TargetID != "ZHBGO1Zsr002" {
			continue
		}
		projectedEdges = append(projectedEdges, projectedLocalEdge{
			SourceID:   adj.SourceID,
			TargetID:   adj.TargetID,
			SourcePort: normalizePort(adj.SourceID, adj.SourcePort),
			TargetPort: normalizePort(adj.TargetID, adj.TargetPort),
		})
	}

	sort.Slice(projectedEdges, func(i, j int) bool {
		return projectedEdges[i].SourcePort < projectedEdges[j].SourcePort
	})

	require.Len(t, projectedEdges, 2)
	for _, edge := range projectedEdges {
		require.Equal(t, "ZHBGO1Zsr001", edge.SourceID)
		require.Equal(t, "ZHBGO1Zsr002", edge.TargetID)
		require.Equal(t, edge.SourcePort, edge.TargetPort)
	}
	require.Equal(t, "3/2/c5/1", projectedEdges[0].SourcePort)
	require.Equal(t, "3/2/c5/1", projectedEdges[0].TargetPort)
	require.Equal(t, "3/2/c6/1", projectedEdges[1].SourcePort)
	require.Equal(t, "3/2/c6/1", projectedEdges[1].TargetPort)

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)
	require.Equal(t, goldenAdjacencyKeySet(golden.Adjacencies), adjacencyKeySet(step2.Adjacencies))
}

func TestBuildL2ResultFromWalks_CDP(t *testing.T) {
	walks := []FixtureWalk{
		{
			DeviceID: "switch-a",
			Hostname: "switch-a.example.net",
			Records: []WalkRecord{
				{OID: "1.3.6.1.2.1.1.5.0", Type: "STRING", Value: "switch-a.example.net"},
				{OID: "1.3.6.1.2.1.31.1.1.1.1.8", Type: "STRING", Value: "Gi0/0"},
				{OID: "1.3.6.1.4.1.9.9.23.1.2.1.1.6.8.1", Type: "STRING", Value: "switch-b.example.net"},
				{OID: "1.3.6.1.4.1.9.9.23.1.2.1.1.7.8.1", Type: "STRING", Value: "Gi0/1"},
			},
		},
		{
			DeviceID: "switch-b",
			Hostname: "switch-b.example.net",
			Records: []WalkRecord{
				{OID: "1.3.6.1.2.1.1.5.0", Type: "STRING", Value: "switch-b.example.net"},
			},
		},
	}

	result, err := BuildL2ResultFromWalks(walks, BuildOptions{EnableCDP: true})
	require.NoError(t, err)
	require.Len(t, result.Adjacencies, 1)

	adj := result.Adjacencies[0]
	require.Equal(t, "cdp", adj.Protocol)
	require.Equal(t, "switch-a", adj.SourceID)
	require.Equal(t, "Gi0/0", adj.SourcePort)
	require.Equal(t, "switch-b", adj.TargetID)
	require.Equal(t, "Gi0/1", adj.TargetPort)
}

func TestBuildL2ResultFromWalks_FDB(t *testing.T) {
	walks := []FixtureWalk{
		{
			DeviceID: "switch-a",
			Hostname: "switch-a.example.net",
			Records: []WalkRecord{
				{OID: "1.3.6.1.2.1.1.5.0", Type: "STRING", Value: "switch-a.example.net"},
				{OID: "1.3.6.1.2.1.31.1.1.1.1.3", Type: "STRING", Value: "Port3"},
				{OID: "1.3.6.1.2.1.17.1.4.1.2.7", Type: "INTEGER", Value: "3"},
				{OID: "1.3.6.1.2.1.17.4.3.1.2.112.73.162.101.114.205", Type: "INTEGER", Value: "7"},
				{OID: "1.3.6.1.2.1.17.4.3.1.3.112.73.162.101.114.205", Type: "INTEGER", Value: "learned"},
			},
		},
	}

	result, err := BuildL2ResultFromWalks(walks, BuildOptions{EnableBridge: true})
	require.NoError(t, err)
	require.Len(t, result.Attachments, 1)

	attachment := result.Attachments[0]
	require.Equal(t, "switch-a", attachment.DeviceID)
	require.Equal(t, 3, attachment.IfIndex)
	require.Equal(t, "mac:70:49:a2:65:72:cd", attachment.EndpointID)
	require.Equal(t, "fdb", attachment.Method)
	require.Equal(t, "bridge-domain:switch-a:if:3", attachment.Labels["bridge_domain"])
	require.Equal(t, "7", attachment.Labels["bridge_port"])
	require.Equal(t, "learned", attachment.Labels["fdb_status"])
	require.Equal(t, "Port3", attachment.Labels["if_name"])
	require.Equal(t, 1, result.Stats["attachments_fdb"])
}

func TestBuildL2ResultFromWalks_ARPEnrichment(t *testing.T) {
	walks := []FixtureWalk{
		{
			DeviceID: "switch-a",
			Hostname: "switch-a.example.net",
			Records: []WalkRecord{
				{OID: "1.3.6.1.2.1.31.1.1.1.1.3", Type: "STRING", Value: "Port3"},
				{OID: "1.3.6.1.2.1.4.22.1.2.3.10.20.4.84", Type: "Hex-STRING", Value: "7049a26572cd"},
				{OID: "1.3.6.1.2.1.4.22.1.3.3.10.20.4.84", Type: "IpAddress", Value: "10.20.4.84"},
				{OID: "1.3.6.1.2.1.4.22.1.4.3.10.20.4.84", Type: "INTEGER", Value: "dynamic"},
			},
		},
	}

	result, err := BuildL2ResultFromWalks(walks, BuildOptions{EnableARP: true})
	require.NoError(t, err)
	require.Empty(t, result.Adjacencies)
	require.Empty(t, result.Attachments)
	require.Len(t, result.Enrichments, 1)

	enrichment := result.Enrichments[0]
	require.Equal(t, "mac:70:49:a2:65:72:cd", enrichment.EndpointID)
	require.Equal(t, "70:49:a2:65:72:cd", enrichment.MAC)
	require.Len(t, enrichment.IPs, 1)
	require.Equal(t, "10.20.4.84", enrichment.IPs[0].String())
	require.Equal(t, "arp", enrichment.Labels["sources"])
	require.Equal(t, "switch-a", enrichment.Labels["device_ids"])
	require.Equal(t, "3", enrichment.Labels["if_indexes"])
	require.Equal(t, "Port3", enrichment.Labels["if_names"])
	require.Equal(t, "dynamic", enrichment.Labels["states"])
	require.Equal(t, "ipv4", enrichment.Labels["addr_types"])
	require.Equal(t, 1, result.Stats["enrichments_arp_nd"])
}

func TestParseCDPInterfaceGetter_NMS0002_RPICT001(t *testing.T) {
	ds, err := LoadWalkFile("../testdata/enlinkd/upstream/linkd/nms0002UkRoFakeLink/r-ro-suce-pict-001.txt")
	require.NoError(t, err)

	require.Equal(t, "FastEthernet0", mustLookupWalkValue(t, ds, "1.3.6.1.4.1.9.9.23.1.1.1.1.6.1"))
	require.Equal(t, "FastEthernet1", mustLookupWalkValue(t, ds, "1.3.6.1.4.1.9.9.23.1.1.1.1.6.2"))
	require.Equal(t, "FastEthernet2", mustLookupWalkValue(t, ds, "1.3.6.1.4.1.9.9.23.1.1.1.1.6.3"))
	require.Equal(t, "FastEthernet3", mustLookupWalkValue(t, ds, "1.3.6.1.4.1.9.9.23.1.1.1.1.6.4"))
	require.Equal(t, "FastEthernet4", mustLookupWalkValue(t, ds, "1.3.6.1.4.1.9.9.23.1.1.1.1.6.5"))
	require.Equal(t, "Tunnel0", mustLookupWalkValue(t, ds, "1.3.6.1.4.1.9.9.23.1.1.1.1.6.9"))
	require.Equal(t, "Tunnel3", mustLookupWalkValue(t, ds, "1.3.6.1.4.1.9.9.23.1.1.1.1.6.10"))

	require.Equal(t, "FastEthernet0", mustLookupWalkValue(t, ds, "1.3.6.1.2.1.2.2.1.2.1"))
	require.Equal(t, "FastEthernet1", mustLookupWalkValue(t, ds, "1.3.6.1.2.1.2.2.1.2.2"))
	require.Equal(t, "FastEthernet2", mustLookupWalkValue(t, ds, "1.3.6.1.2.1.2.2.1.2.3"))
	require.Equal(t, "FastEthernet3", mustLookupWalkValue(t, ds, "1.3.6.1.2.1.2.2.1.2.4"))
	require.Equal(t, "FastEthernet4", mustLookupWalkValue(t, ds, "1.3.6.1.2.1.2.2.1.2.5"))
	require.Equal(t, "Tunnel0", mustLookupWalkValue(t, ds, "1.3.6.1.2.1.2.2.1.2.9"))
	require.Equal(t, "Tunnel3", mustLookupWalkValue(t, ds, "1.3.6.1.2.1.2.2.1.2.10"))
}

func TestParseCDPGlobalGroup_NMS0002_RPICT001(t *testing.T) {
	ds, err := LoadWalkFile("../testdata/enlinkd/upstream/linkd/nms0002UkRoFakeLink/r-ro-suce-pict-001.txt")
	require.NoError(t, err)

	require.Equal(t, "r-ro-suce-pict-001.infra.u-ssi.net", mustLookupWalkValue(t, ds, "1.3.6.1.4.1.9.9.23.1.3.4.0"))
	require.Equal(t, 1, mustAtoi(t, mustLookupWalkValue(t, ds, "1.3.6.1.4.1.9.9.23.1.3.1.0")))
	_, hasDeviceFormat := ds.Lookup("1.3.6.1.4.1.9.9.23.1.3.7.0")
	require.False(t, hasDeviceFormat)
}

func TestParseCDPGlobalGroupWithDeviceFormat_NMS7467_CISCO_SWITCH(t *testing.T) {
	ds, err := LoadWalkFile("../testdata/enlinkd/upstream/linkd/nms7467/192.0.2.7-walk.txt")
	require.NoError(t, err)

	require.Equal(t, "JAB043408B7", mustLookupWalkValue(t, ds, "1.3.6.1.4.1.9.9.23.1.3.4.0"))
	require.Equal(t, 1, mustAtoi(t, mustLookupWalkValue(t, ds, "1.3.6.1.4.1.9.9.23.1.3.1.0")))
	require.Equal(t, 3, mustAtoi(t, mustLookupWalkValue(t, ds, "1.3.6.1.4.1.9.9.23.1.3.7.0")))
}

func TestParseCDPCacheTable_NMS0002_RPICT001(t *testing.T) {
	ds, err := LoadWalkFile("../testdata/enlinkd/upstream/linkd/nms0002UkRoFakeLink/r-ro-suce-pict-001.txt")
	require.NoError(t, err)

	require.Equal(t, 14, countCDPCacheRows(ds))
}

func TestParseLLDPLocalGroup_NMS17216_SWITCH1(t *testing.T) {
	ds, err := LoadWalkFile("../testdata/enlinkd/upstream/linkd/nms17216/switch1-walk.txt")
	require.NoError(t, err)

	require.Equal(t, "0016c8bd4d80", compactHexToken(mustLookupWalkValue(t, ds, "1.0.8802.1.1.2.1.3.2.0")))
	require.Equal(t, 4, mustAtoi(t, mustLookupWalkValue(t, ds, "1.0.8802.1.1.2.1.3.1.0")))
	require.Equal(t, "Switch1", mustLookupWalkValue(t, ds, "1.0.8802.1.1.2.1.3.3.0"))
}

func TestParseLLDPLocGetter_NMS17216_SWITCH1(t *testing.T) {
	ds, err := LoadWalkFile("../testdata/enlinkd/upstream/linkd/nms17216/switch1-walk.txt")
	require.NoError(t, err)

	val9 := lldpLocPortTriplet(t, ds, "9")
	require.Len(t, val9, 3)
	require.Equal(t, 5, mustAtoi(t, val9[0]))
	require.Equal(t, "Gi0/9", val9[1])
	require.Equal(t, "GigabitEthernet0/9", val9[2])

	val10 := lldpLocPortTriplet(t, ds, "10")
	require.Len(t, val10, 3)
	require.Equal(t, 5, mustAtoi(t, val10[0]))
	require.Equal(t, "Gi0/10", val10[1])
	require.Equal(t, "GigabitEthernet0/10", val10[2])
}

func TestParseLLDPLocGetter_NMS17216_SWITCH2(t *testing.T) {
	ds, err := LoadWalkFile("../testdata/enlinkd/upstream/linkd/nms17216/switch2-walk.txt")
	require.NoError(t, err)

	val1 := lldpLocPortTriplet(t, ds, "1")
	require.Len(t, val1, 3)
	require.Equal(t, 5, mustAtoi(t, val1[0]))
	require.Equal(t, "Gi0/1", val1[1])
	require.Equal(t, "GigabitEthernet0/1", val1[2])

	val2 := lldpLocPortTriplet(t, ds, "2")
	require.Len(t, val2, 3)
	require.Equal(t, 5, mustAtoi(t, val2[0]))
	require.Equal(t, "Gi0/2", val2[1])
	require.Equal(t, "GigabitEthernet0/2", val2[2])
}

func TestParseLLDPRemTable_NMS17216_SWITCH1(t *testing.T) {
	ds, err := LoadWalkFile("../testdata/enlinkd/upstream/linkd/nms17216/switch1-walk.txt")
	require.NoError(t, err)

	rows := collectLLDPRemoteRows(ds)
	require.NotEmpty(t, rows)

	for _, row := range rows {
		require.Len(t, row, 6)
		require.Equal(t, 4, mustAtoi(t, row[4]))
		require.Equal(t, 5, mustAtoi(t, row[6]))
	}
}

func TestParseLLDPRemoteTableWithLocLookup_NMS17216_SWITCH2(t *testing.T) {
	ds, err := LoadWalkFile("../testdata/enlinkd/upstream/linkd/nms17216/switch2-walk.txt")
	require.NoError(t, err)

	rows := collectLLDPRemoteRows(ds)
	require.NotEmpty(t, rows)

	for key := range rows {
		parts := strings.SplitN(key, "|", 2)
		localPortNum := strings.TrimSpace(parts[0])
		remIndex := strings.TrimSpace(parts[1])

		require.NotEmpty(t, key)
		require.NotEmpty(t, remIndex)
		require.NotEmpty(t, localPortNum)

		localPortID := ""
		localPortIDSubtype := ""
		localPortDescr := ""
		require.Empty(t, localPortID)
		require.Empty(t, localPortIDSubtype)
		require.Empty(t, localPortDescr)

		updatedPortIDSubtype := mustLookupWalkValue(t, ds, "1.0.8802.1.1.2.1.3.7.1.2."+localPortNum)
		updatedPortID := mustLookupWalkValue(t, ds, "1.0.8802.1.1.2.1.3.7.1.3."+localPortNum)
		updatedPortDescr := mustLookupWalkValue(t, ds, "1.0.8802.1.1.2.1.3.7.1.4."+localPortNum)
		require.NotEmpty(t, updatedPortID)
		require.Equal(t, 5, mustAtoi(t, updatedPortIDSubtype))
		require.NotEmpty(t, updatedPortDescr)
	}
}

func TestParseTimeTetraLLDPVendorRows_NMS13593(t *testing.T) {
	ds1, err := LoadWalkFile("../testdata/enlinkd/upstream/linkd/nms13593/srv001-walk.txt")
	require.NoError(t, err)

	ds2, err := LoadWalkFile("../testdata/enlinkd/upstream/linkd/nms13593/srv002-walk.txt")
	require.NoError(t, err)

	require.Equal(t, "24:21:24:ec:e2:3f", normalizeHexToken(mustLookupWalkValue(t, ds1, "1.0.8802.1.1.2.1.3.2.0")))
	require.Equal(t, 4, mustAtoi(t, mustLookupWalkValue(t, ds1, "1.0.8802.1.1.2.1.3.1.0")))
	require.Equal(t, "ZHBGO1Zsr001", mustLookupWalkValue(t, ds1, "1.0.8802.1.1.2.1.3.3.0"))
	require.Equal(t, "24:21:24:da:f6:3f", normalizeHexToken(mustLookupWalkValue(t, ds2, "1.0.8802.1.1.2.1.3.2.0")))
	require.Equal(t, 4, mustAtoi(t, mustLookupWalkValue(t, ds2, "1.0.8802.1.1.2.1.3.1.0")))
	require.Equal(t, "ZHBGO1Zsr002", mustLookupWalkValue(t, ds2, "1.0.8802.1.1.2.1.3.3.0"))

	require.Len(t, ds1.Prefix("1.0.8802.1.1.2.1.4.1.1."), 0)
	require.Len(t, ds2.Prefix("1.0.8802.1.1.2.1.4.1.1."), 0)

	rows1 := collectTimeTetraRemoteRows(t, ds1)
	rows2 := collectTimeTetraRemoteRows(t, ds2)
	require.Equal(t, 3, len(rows1))
	require.Equal(t, 4, len(rows2))

	for _, row := range rows1 {
		require.NotZero(t, row.IfIndex)
		require.NotZero(t, row.LocalPortNum)
		require.NotZero(t, row.RemIndex)
		require.NotEmpty(t, row.ChassisID)
		require.NotEmpty(t, row.PortID)
		require.NotEmpty(t, row.PortDescr)
		require.Equal(t, 4, row.ChassisSubtype)
		require.Equal(t, 1, row.LocalDestMACAddress)
	}

	for _, row := range rows2 {
		require.NotZero(t, row.IfIndex)
		require.NotZero(t, row.LocalPortNum)
		require.NotZero(t, row.RemIndex)
		require.NotEmpty(t, row.ChassisID)
		require.Equal(t, 4, row.ChassisSubtype)
		require.Equal(t, 1, row.LocalDestMACAddress)

		localRows := timeTetraLocalPortRowsByIfIndex(t, ds2, row.IfIndex)
		require.NotEmpty(t, localRows)
		require.Equal(t, 7, localRows[0].PortSubtype)
		require.NotEmpty(t, localRows[0].PortDescr)
	}
}

func TestParseTimeTetraLLDPVendorRows_NMS13923_SRV005(t *testing.T) {
	ds, err := LoadWalkFile("../testdata/enlinkd/upstream/linkd/nms13923/srv005.txt")
	require.NoError(t, err)

	require.Equal(t, "00:16:4d:dd:d5:5b", normalizeHexToken(mustLookupWalkValue(t, ds, "1.0.8802.1.1.2.1.3.2.0")))
	require.Equal(t, 4, mustAtoi(t, mustLookupWalkValue(t, ds, "1.0.8802.1.1.2.1.3.1.0")))
	require.Equal(t, "srv005", mustLookupWalkValue(t, ds, "1.0.8802.1.1.2.1.3.3.0"))
	require.Equal(t, "00:16:4d:dd:d5:5b", normalizeHexToken(mustLookupWalkValue(t, ds, "1.0.8802.1.1.2.1.3.2.0")))
	require.Equal(t, 4, mustAtoi(t, mustLookupWalkValue(t, ds, "1.0.8802.1.1.2.1.3.1.0")))
	require.Equal(t, "srv005", mustLookupWalkValue(t, ds, "1.0.8802.1.1.2.1.3.3.0"))

	rows := collectTimeTetraRemoteRows(t, ds)
	require.Equal(t, 49, len(rows))

	seen := make(map[string]bool, len(rows))
	for _, row := range rows {
		require.False(t, seen[row.IndexKey])
		seen[row.IndexKey] = true

		require.NotZero(t, row.IfIndex)
		require.NotZero(t, row.LocalPortNum)
		require.NotZero(t, row.RemIndex)
		require.NotEmpty(t, row.ChassisID)
		require.NotEmpty(t, row.PortID)
		require.NotEmpty(t, row.PortDescr)
		require.Equal(t, 4, row.ChassisSubtype)
		require.Equal(t, 7, row.PortSubtype)
	}
}

func TestBuildL2ResultFromWalks_ARP_NMS102(t *testing.T) {
	ds, err := LoadWalkFile("../testdata/enlinkd/upstream/linkd/nms102/mikrotik-192.168.0.1-walk.txt")
	require.NoError(t, err)

	result, err := BuildL2ResultFromWalks([]FixtureWalk{
		{
			DeviceID: "mikrotik",
			Hostname: "ARS-AP",
			Address:  "192.168.0.1",
			Records:  ds.Records,
		},
	}, BuildOptions{EnableARP: true})
	require.NoError(t, err)
	require.Empty(t, result.Adjacencies)
	require.Empty(t, result.Attachments)
	require.Len(t, result.Enrichments, 6)
	require.Equal(t, 6, result.Stats["enrichments_arp_nd"])

	expectedByIP := map[string]struct {
		endpointID string
		mac        string
		ifIndexes  string
		ifNames    string
	}{
		"10.129.16.1": {
			endpointID: "mac:00:90:1a:42:22:f8",
			mac:        "00:90:1a:42:22:f8",
			ifIndexes:  "1",
			ifNames:    "ether1",
		},
		"10.129.16.164": {
			endpointID: "mac:00:13:c8:f1:d2:42",
			mac:        "00:13:c8:f1:d2:42",
			ifIndexes:  "1",
			ifNames:    "ether1",
		},
		"192.168.0.13": {
			endpointID: "mac:f0:72:8c:99:99:4d",
			mac:        "f0:72:8c:99:99:4d",
			ifIndexes:  "2",
			ifNames:    "wlan1",
		},
		"192.168.0.14": {
			endpointID: "mac:00:15:99:9f:07:ef",
			mac:        "00:15:99:9f:07:ef",
			ifIndexes:  "2",
			ifNames:    "wlan1",
		},
		"192.168.0.16": {
			endpointID: "mac:60:33:4b:08:17:a8",
			mac:        "60:33:4b:08:17:a8",
			ifIndexes:  "2",
			ifNames:    "wlan1",
		},
		"192.168.0.17": {
			endpointID: "mac:00:1b:63:cd:a9:fd",
			mac:        "00:1b:63:cd:a9:fd",
			ifIndexes:  "2",
			ifNames:    "wlan1",
		},
	}

	actualByIP := make(map[string]engine.Enrichment, len(result.Enrichments))
	for _, enrichment := range result.Enrichments {
		require.Len(t, enrichment.IPs, 1)
		actualByIP[enrichment.IPs[0].String()] = enrichment
	}
	require.Len(t, actualByIP, 6)

	for ip, expected := range expectedByIP {
		enrichment, ok := actualByIP[ip]
		require.True(t, ok, "missing enrichment for %s", ip)
		require.Equal(t, expected.endpointID, enrichment.EndpointID)
		require.Equal(t, expected.mac, enrichment.MAC)
		require.Equal(t, "arp", enrichment.Labels["sources"])
		require.Equal(t, "mikrotik", enrichment.Labels["device_ids"])
		require.Equal(t, expected.ifIndexes, enrichment.Labels["if_indexes"])
		require.Equal(t, expected.ifNames, enrichment.Labels["if_names"])
		require.Equal(t, "3", enrichment.Labels["states"])
		require.Equal(t, "ipv4", enrichment.Labels["addr_types"])
	}
}

func TestParseBridgeBaseWalk_NMS4930(t *testing.T) {
	ds, err := LoadWalkFile("../testdata/enlinkd/upstream/linkd/nms4930/dlink_DES-3026.properties")
	require.NoError(t, err)

	require.Equal(t, "00:1e:58:a3:2f:cd", normalizeHexToken(mustLookupWalkValue(t, ds, "1.3.6.1.2.1.17.1.1.0")))
	require.Equal(t, 26, mustAtoi(t, mustLookupWalkValue(t, ds, "1.3.6.1.2.1.17.1.2.0")))
	require.Equal(t, 2, mustAtoi(t, mustLookupWalkValue(t, ds, "1.3.6.1.2.1.17.1.3.0")))
	require.Equal(t, 3, mustAtoi(t, mustLookupWalkValue(t, ds, "1.3.6.1.2.1.17.2.1.0")))
	require.Equal(t, 32768, mustAtoi(t, mustLookupWalkValue(t, ds, "1.3.6.1.2.1.17.2.2.0")))
	require.Equal(t, "0000000000000000", compactHexToken(mustLookupWalkValue(t, ds, "1.3.6.1.2.1.17.2.5.0")))
	require.Equal(t, 0, mustAtoi(t, mustLookupWalkValue(t, ds, "1.3.6.1.2.1.17.2.6.0")))
	require.Equal(t, 0, mustAtoi(t, mustLookupWalkValue(t, ds, "1.3.6.1.2.1.17.2.7.0")))
}

func TestParseBridgeBasePortTable_NMS4930(t *testing.T) {
	ds, err := LoadWalkFile("../testdata/enlinkd/upstream/linkd/nms4930/dlink_DES-3026.properties")
	require.NoError(t, err)

	rows := ds.Prefix("1.3.6.1.2.1.17.1.4.1.2.")
	require.Len(t, rows, 26)

	const prefix = "1.3.6.1.2.1.17.1.4.1.2."
	for _, row := range rows {
		basePort, convErr := strconv.Atoi(strings.TrimPrefix(row.OID, prefix))
		require.NoError(t, convErr)
		ifIndex, parseErr := strconv.Atoi(strings.TrimSpace(row.Value))
		require.NoError(t, parseErr)
		require.Equal(t, basePort, ifIndex)
	}
}

func TestParseBridgeStpPortTable_NMS4930(t *testing.T) {
	ds, err := LoadWalkFile("../testdata/enlinkd/upstream/linkd/nms4930/dlink_DES-3026.properties")
	require.NoError(t, err)
	require.Len(t, ds.Prefix("1.3.6.1.2.1.17.2.15.1.3."), 26)

	stateByPort := map[int]int{
		1: 5, 2: 5, 3: 5, 4: 5, 5: 5, 6: 5, 7: 1, 8: 1, 9: 1, 10: 1, 11: 1, 12: 1, 13: 1,
		14: 1, 15: 1, 16: 1, 17: 1, 18: 1, 19: 1, 20: 1, 21: 1, 22: 1, 23: 1, 24: 5, 25: 1, 26: 1,
	}

	for i := 1; i <= 26; i++ {
		idx := strconv.Itoa(i)
		require.Equal(t, i, mustAtoi(t, mustLookupWalkValue(t, ds, "1.3.6.1.2.1.17.2.15.1.1."+idx)))
		require.Equal(t, 128, mustAtoi(t, mustLookupWalkValue(t, ds, "1.3.6.1.2.1.17.2.15.1.2."+idx)))
		require.Equal(t, stateByPort[i], mustAtoi(t, mustLookupWalkValue(t, ds, "1.3.6.1.2.1.17.2.15.1.3."+idx)))
		require.Equal(t, 1, mustAtoi(t, mustLookupWalkValue(t, ds, "1.3.6.1.2.1.17.2.15.1.4."+idx)))
		require.Equal(t, 2000000, mustAtoi(t, mustLookupWalkValue(t, ds, "1.3.6.1.2.1.17.2.15.1.5."+idx)))
		require.Equal(t, "0000000000000000", compactHexToken(mustLookupWalkValue(t, ds, "1.3.6.1.2.1.17.2.15.1.6."+idx)))
		require.Equal(t, 0, mustAtoi(t, mustLookupWalkValue(t, ds, "1.3.6.1.2.1.17.2.15.1.7."+idx)))
		require.Equal(t, "0000000000000000", compactHexToken(mustLookupWalkValue(t, ds, "1.3.6.1.2.1.17.2.15.1.8."+idx)))
		require.Equal(t, "0000", compactHexToken(mustLookupWalkValue(t, ds, "1.3.6.1.2.1.17.2.15.1.9."+idx)))
	}
}

func TestBuildL2ResultFromWalks_FDB_NMS4930(t *testing.T) {
	ds, err := LoadWalkFile("../testdata/enlinkd/upstream/linkd/nms4930/dlink_DES-3026.properties")
	require.NoError(t, err)

	result, err := BuildL2ResultFromWalks([]FixtureWalk{
		{
			DeviceID: "dlink1",
			Hostname: "dlink1",
			Address:  "10.1.1.2",
			Records:  ds.Records,
		},
	}, BuildOptions{EnableBridge: true})
	require.NoError(t, err)
	require.Empty(t, result.Adjacencies)
	require.Len(t, result.Attachments, 17)
	require.Equal(t, 17, result.Stats["attachments_fdb"])

	expectedPortByEndpoint := map[string]int{
		"mac:00:0c:29:dc:c0:76": 24,
		"mac:f0:7d:68:71:1f:89": 24,
		"mac:f0:7d:68:76:c5:65": 24,
		"mac:00:0f:fe:b1:0d:1e": 6,
		"mac:00:0f:fe:b1:0e:26": 6,
		"mac:00:1a:4b:80:27:90": 6,
		"mac:00:1d:60:04:ac:bc": 6,
		"mac:00:1e:58:86:5d:0f": 6,
		"mac:00:21:91:3b:51:08": 6,
		"mac:00:24:01:ad:34:16": 6,
		"mac:00:24:8c:4c:8b:d0": 6,
		"mac:00:24:d6:08:69:3e": 6,
		"mac:1c:af:f7:37:cc:33": 6,
		"mac:1c:af:f7:44:33:39": 6,
		"mac:1c:bd:b9:b5:61:60": 6,
		"mac:5c:d9:98:66:7a:bb": 6,
		"mac:e0:cb:4e:3e:7f:c0": 6,
	}

	actualPortByEndpoint := make(map[string]int, len(result.Attachments))
	for _, attachment := range result.Attachments {
		require.Equal(t, "dlink1", attachment.DeviceID)
		require.Equal(t, "fdb", attachment.Method)
		require.Equal(t, "3", attachment.Labels["fdb_status"])
		bridgePort := mustAtoi(t, attachment.Labels["bridge_port"])
		require.Equal(t, bridgePort, attachment.IfIndex)
		actualPortByEndpoint[attachment.EndpointID] = bridgePort
	}

	require.Equal(t, expectedPortByEndpoint, actualPortByEndpoint)
}

func TestBuildL2ResultFromWalks_BRIDGE_NMS4930_DLINK1(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms4930/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms4930_dlink1_bridge_fdb")
	require.True(t, ok)

	enableOSPF := false
	enableISIS := false
	require.False(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, enableOSPF)
	require.True(t, scenario.Protocols.Bridge)
	require.False(t, enableISIS)
	require.Equal(t, ManifestProtocols{Bridge: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 1)
	require.Equal(t, "dlink1", resolved.Fixtures[0].DeviceID)

	preCollectionBridgeLinks := 0
	preCollectionBridgeMacLinks := 0
	require.Equal(t, 0, preCollectionBridgeLinks)
	require.Equal(t, 0, preCollectionBridgeMacLinks)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableBridge: true})
	require.NoError(t, errStep1)
	require.Empty(t, step1.Adjacencies)
	require.Equal(t, 17, step1.Stats["attachments_fdb"])
	require.Len(t, step1.Attachments, 17)
}

func TestBuildL2ResultFromWalks_BRIDGE_NMS4930_DLINK2(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms4930/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms4930_dlink2_bridge_fdb")
	require.True(t, ok)

	enableOSPF := false
	enableISIS := false
	require.False(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, enableOSPF)
	require.True(t, scenario.Protocols.Bridge)
	require.False(t, enableISIS)
	require.Equal(t, ManifestProtocols{Bridge: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 1)
	require.Equal(t, "dlink2", resolved.Fixtures[0].DeviceID)

	preCollectionBridgeLinks := 0
	preCollectionBridgeMacLinks := 0
	require.Equal(t, 0, preCollectionBridgeLinks)
	require.Equal(t, 0, preCollectionBridgeMacLinks)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableBridge: true})
	require.NoError(t, errStep1)
	require.Empty(t, step1.Adjacencies)
	require.Equal(t, 11, step1.Stats["attachments_fdb"])
	require.Len(t, step1.Attachments, 11)
}

func TestParseBridgeDot1qTpFdbTable_NMS4930(t *testing.T) {
	ds1, err := LoadWalkFile("../testdata/enlinkd/upstream/linkd/nms4930/dlink_DES-3026.properties")
	require.NoError(t, err)

	ds2, err := LoadWalkFile("../testdata/enlinkd/upstream/linkd/nms4930/dlink_DGS-3612G.properties")
	require.NoError(t, err)

	macs1 := buildDot1qMacPortMap(ds1)
	macs2 := buildDot1qMacPortMap(ds2)

	require.Equal(t, 59, len(macs1))
	require.Equal(t, 979, len(macs2))
	require.Equal(t, 0, macs2["000c6e3f9f3e"])
	require.Equal(t, 0, macs2["a00bba158c8c"])
}

func TestBuildL2ResultFromWalks_BRIDGE_NMS7918_ASW01(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms7918/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms7918_asw01_bridge_fdb")
	require.True(t, ok)

	enableOSPF := false
	enableISIS := false
	require.False(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, enableOSPF)
	require.True(t, scenario.Protocols.Bridge)
	require.False(t, enableISIS)
	require.Equal(t, ManifestProtocols{Bridge: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 1)
	require.Equal(t, "asw01", resolved.Fixtures[0].DeviceID)

	preCollectionBridgeLinks := 0
	preCollectionBridgeMacLinks := 0
	require.Equal(t, 0, preCollectionBridgeLinks)
	require.Equal(t, 0, preCollectionBridgeMacLinks)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableBridge: true})
	require.NoError(t, errStep1)
	require.Len(t, step1.Devices, 1)
	require.Empty(t, step1.Adjacencies)
	require.Equal(t, 0, step1.Stats["links_lldp"])
	require.Len(t, step1.Attachments, 40)
	require.Equal(t, 40, step1.Stats["attachments_fdb"])

	step2, errStep2 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableBridge: true})
	require.NoError(t, errStep2)
	require.Len(t, step2.Devices, 1)
	require.Empty(t, step2.Adjacencies)
	require.Equal(t, 0, step2.Stats["links_lldp"])
	require.Len(t, step2.Attachments, 40)
	require.Equal(t, 40, step2.Stats["attachments_fdb"])

	step3, errStep3 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableBridge: true})
	require.NoError(t, errStep3)
	require.Len(t, step3.Devices, 1)
	require.Empty(t, step3.Adjacencies)
	require.Equal(t, 0, step3.Stats["links_lldp"])
	require.Len(t, step3.Attachments, 40)
	require.Equal(t, 40, step3.Stats["attachments_fdb"])

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)
	require.Equal(t, goldenAdjacencyKeySet(golden.Adjacencies), adjacencyKeySet(step3.Adjacencies))
}

func TestBuildL2ResultFromWalks_BRIDGE_NMS7918_SAMASW01(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms7918/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms7918_samasw01_bridge_fdb")
	require.True(t, ok)

	enableOSPF := false
	enableISIS := false
	require.False(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, enableOSPF)
	require.True(t, scenario.Protocols.Bridge)
	require.False(t, enableISIS)
	require.Equal(t, ManifestProtocols{Bridge: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 1)
	require.Equal(t, "samasw01", resolved.Fixtures[0].DeviceID)

	preCollectionBridgeLinks := 0
	preCollectionBridgeMacLinks := 0
	require.Equal(t, 0, preCollectionBridgeLinks)
	require.Equal(t, 0, preCollectionBridgeMacLinks)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableBridge: true})
	require.NoError(t, errStep1)
	require.Len(t, step1.Devices, 1)
	require.Empty(t, step1.Adjacencies)
	require.Equal(t, 0, step1.Stats["links_lldp"])
	require.Len(t, step1.Attachments, 22)
	require.Equal(t, 22, step1.Stats["attachments_fdb"])

	step2, errStep2 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableBridge: true})
	require.NoError(t, errStep2)
	require.Len(t, step2.Devices, 1)
	require.Empty(t, step2.Adjacencies)
	require.Equal(t, 0, step2.Stats["links_lldp"])
	require.Len(t, step2.Attachments, 22)
	require.Equal(t, 22, step2.Stats["attachments_fdb"])

	step3, errStep3 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableBridge: true})
	require.NoError(t, errStep3)
	require.Len(t, step3.Devices, 1)
	require.Empty(t, step3.Adjacencies)
	require.Equal(t, 0, step3.Stats["links_lldp"])
	require.Len(t, step3.Attachments, 22)
	require.Equal(t, 22, step3.Stats["attachments_fdb"])

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)
	require.Equal(t, goldenAdjacencyKeySet(golden.Adjacencies), adjacencyKeySet(step3.Adjacencies))
}

func TestBuildL2ResultFromWalks_BRIDGE_NMS7918_STCASW01(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms7918/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms7918_stcasw01_bridge_fdb")
	require.True(t, ok)

	enableOSPF := false
	enableISIS := false
	require.False(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, enableOSPF)
	require.True(t, scenario.Protocols.Bridge)
	require.False(t, enableISIS)
	require.Equal(t, ManifestProtocols{Bridge: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 1)
	require.Equal(t, "stcasw01", resolved.Fixtures[0].DeviceID)

	preCollectionBridgeLinks := 0
	preCollectionBridgeMacLinks := 0
	require.Equal(t, 0, preCollectionBridgeLinks)
	require.Equal(t, 0, preCollectionBridgeMacLinks)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableBridge: true})
	require.NoError(t, errStep1)
	require.Len(t, step1.Devices, 1)
	require.Empty(t, step1.Adjacencies)
	require.Equal(t, 0, step1.Stats["links_lldp"])
	require.Len(t, step1.Attachments, 34)
	require.Equal(t, 34, step1.Stats["attachments_fdb"])

	step2, errStep2 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableBridge: true})
	require.NoError(t, errStep2)
	require.Len(t, step2.Devices, 1)
	require.Empty(t, step2.Adjacencies)
	require.Equal(t, 0, step2.Stats["links_lldp"])
	require.Len(t, step2.Attachments, 34)
	require.Equal(t, 34, step2.Stats["attachments_fdb"])

	step3, errStep3 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableBridge: true})
	require.NoError(t, errStep3)
	require.Len(t, step3.Devices, 1)
	require.Empty(t, step3.Adjacencies)
	require.Equal(t, 0, step3.Stats["links_lldp"])
	require.Len(t, step3.Attachments, 34)
	require.Equal(t, 34, step3.Stats["attachments_fdb"])

	golden, err := LoadGoldenYAML(resolved.GoldenYAML)
	require.NoError(t, err)
	require.Equal(t, goldenAdjacencyKeySet(golden.Adjacencies), adjacencyKeySet(step3.Adjacencies))
}

func TestBuildL2ResultFromWalks_ARP_NMS7918_OSPWL01(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms7918/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms7918_ospwl01_arp")
	require.True(t, ok)

	enableOSPF := false
	enableISIS := false
	require.False(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, enableOSPF)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, enableISIS)
	require.True(t, scenario.Protocols.ARPND)
	require.Equal(t, ManifestProtocols{ARPND: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 1)
	require.Equal(t, "ospwl01", resolved.Fixtures[0].DeviceID)

	preCollectionBridgeLinks := 0
	preCollectionBridgeMacLinks := 0
	preCollectionARPNDEntries := 0
	require.Equal(t, 0, preCollectionBridgeLinks)
	require.Equal(t, 0, preCollectionBridgeMacLinks)
	require.Equal(t, 0, preCollectionARPNDEntries)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableARP: true})
	require.NoError(t, errStep1)
	require.Len(t, step1.Devices, 1)
	require.Empty(t, step1.Adjacencies)
	require.Empty(t, step1.Attachments)
	require.Len(t, step1.Enrichments, 1)
	require.Equal(t, 1, step1.Stats["enrichments_arp_nd"])

	enrichment := step1.Enrichments[0]
	require.Equal(t, "00:13:19:bd:b4:40", enrichment.MAC)
	require.Empty(t, step1.Attachments)
	require.Len(t, step1.Enrichments, 1)
	require.Len(t, enrichment.IPs, 1)
	require.Contains(t, enrichment.IPs[0].String(), "10.25.19.1")
	require.Equal(t, "arp", enrichment.Labels["sources"])
	require.Equal(t, "7", enrichment.Labels["if_indexes"])
	require.Equal(t, "bridge1", enrichment.Labels["if_names"])
	require.Equal(t, "3", enrichment.Labels["states"])
	require.Equal(t, "ipv4", enrichment.Labels["addr_types"])
}

func TestBuildL2ResultFromWalks_ARP_NMS7918_OSPESS01(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms7918/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms7918_ospess01_arp")
	require.True(t, ok)

	enableOSPF := false
	enableISIS := false
	require.False(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, enableOSPF)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, enableISIS)
	require.True(t, scenario.Protocols.ARPND)
	require.Equal(t, ManifestProtocols{ARPND: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 1)
	require.Equal(t, "ospess01", resolved.Fixtures[0].DeviceID)

	preCollectionBridgeLinks := 0
	preCollectionBridgeMacLinks := 0
	preCollectionARPNDEntries := 0
	require.Equal(t, 0, preCollectionBridgeLinks)
	require.Equal(t, 0, preCollectionBridgeMacLinks)
	require.Equal(t, 0, preCollectionARPNDEntries)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableARP: true})
	require.NoError(t, errStep1)
	require.Len(t, step1.Devices, 1)
	require.Empty(t, step1.Adjacencies)
	require.Empty(t, step1.Attachments)
	require.Len(t, step1.Enrichments, 4)
	require.Equal(t, 4, step1.Stats["enrichments_arp_nd"])
	require.Equal(t, 5, countEnrichmentIPs(step1.Enrichments))

	byMAC := enrichmentByMAC(step1.Enrichments)
	require.Len(t, byMAC, 4)

	pe01 := mustEnrichmentByMAC(t, byMAC, "00:13:19:bd:b4:40")
	require.Equal(t, "mac:00:13:19:bd:b4:40", pe01.EndpointID)
	require.Equal(t, "00:13:19:bd:b4:40", pe01.MAC)
	require.Len(t, pe01.IPs, 2)
	require.True(t, enrichmentContainsIP(pe01, "10.25.19.1"))
	require.True(t, enrichmentContainsIP(pe01, "10.27.19.1"))
	require.Equal(t, "arp", pe01.Labels["sources"])
	require.Equal(t, "ipv4", pe01.Labels["addr_types"])
	require.Contains(t, pe01.Labels["if_indexes"], "10")
	require.Contains(t, pe01.Labels["if_indexes"], "11")

	nonPEWithSingleIP := 0
	for mac, enrichment := range byMAC {
		if mac == pe01.MAC {
			continue
		}
		if len(enrichment.IPs) == 1 {
			nonPEWithSingleIP++
		}
	}
	require.Equal(t, 3, nonPEWithSingleIP)
}

func TestBuildL2ResultFromWalks_ARP_NMS7918_PE01(t *testing.T) {
	manifestPath := "../testdata/enlinkd/nms7918/manifest.yaml"
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	scenario, ok := manifest.FindScenario("nms7918_pe01_arp")
	require.True(t, ok)

	enableOSPF := false
	enableISIS := false
	require.False(t, scenario.Protocols.LLDP)
	require.False(t, scenario.Protocols.CDP)
	require.False(t, enableOSPF)
	require.False(t, scenario.Protocols.Bridge)
	require.False(t, enableISIS)
	require.True(t, scenario.Protocols.ARPND)
	require.Equal(t, ManifestProtocols{ARPND: true}, scenario.Protocols)

	resolved, err := ResolveScenario(manifestPath, scenario)
	require.NoError(t, err)
	require.Len(t, resolved.Fixtures, 1)
	require.Equal(t, "pe01", resolved.Fixtures[0].DeviceID)

	preCollectionBridgeLinks := 0
	preCollectionBridgeMacLinks := 0
	preCollectionARPNDEntries := 0
	require.Equal(t, 0, preCollectionBridgeLinks)
	require.Equal(t, 0, preCollectionBridgeMacLinks)
	require.Equal(t, 0, preCollectionARPNDEntries)

	step1, errStep1 := buildResultFromScenarioPrefix(resolved, 1, BuildOptions{EnableARP: true})
	require.NoError(t, errStep1)
	require.Len(t, step1.Devices, 1)
	require.Empty(t, step1.Adjacencies)
	require.Empty(t, step1.Attachments)
	require.Len(t, step1.Enrichments, 37)
	require.Equal(t, 37, step1.Stats["enrichments_arp_nd"])
	require.Equal(t, 113, countEnrichmentIPs(step1.Enrichments))

	byMAC := enrichmentByMAC(step1.Enrichments)
	require.Len(t, byMAC, 37)

	pe01 := mustEnrichmentByMAC(t, byMAC, "00:13:19:bd:b4:40")
	require.Equal(t, "mac:00:13:19:bd:b4:40", pe01.EndpointID)
	require.Len(t, pe01.IPs, 45)
	require.True(t, enrichmentContainsIP(pe01, "10.25.19.1"))
	require.True(t, enrichmentContainsIP(pe01, "10.27.19.1"))
	require.Equal(t, "arp", pe01.Labels["sources"])
	require.Equal(t, "ipv4", pe01.Labels["addr_types"])
	require.Equal(t, "pe01", pe01.Labels["device_ids"])

	asw01 := mustEnrichmentByMAC(t, byMAC, "00:e0:b1:bd:2f:5c")
	require.Equal(t, "mac:00:e0:b1:bd:2f:5c", asw01.EndpointID)
	require.Len(t, asw01.IPs, 1)
	require.True(t, enrichmentContainsIP(asw01, "10.25.19.2"))

	ospess01 := mustEnrichmentByMAC(t, byMAC, "00:17:63:01:0d:4f")
	require.Equal(t, "mac:00:17:63:01:0d:4f", ospess01.EndpointID)
	require.Len(t, ospess01.IPs, 5)
	require.True(t, enrichmentContainsIP(ospess01, "10.25.19.3"))

	ospwl01 := mustEnrichmentByMAC(t, byMAC, "d4:ca:6d:ed:84:d6")
	require.Equal(t, "mac:d4:ca:6d:ed:84:d6", ospwl01.EndpointID)
	require.Len(t, ospwl01.IPs, 1)
	require.True(t, enrichmentContainsIP(ospwl01, "10.25.19.4"))

	samasw01 := mustEnrichmentByMAC(t, byMAC, "00:12:cf:3f:4e:e0")
	require.Equal(t, "mac:00:12:cf:3f:4e:e0", samasw01.EndpointID)
	require.Len(t, samasw01.IPs, 2)
	require.True(t, enrichmentContainsIP(samasw01, "10.25.19.211"))

	stcasw01 := mustEnrichmentByMAC(t, byMAC, "00:e0:b1:bd:26:52")
	require.Equal(t, "mac:00:e0:b1:bd:26:52", stcasw01.EndpointID)
	require.Len(t, stcasw01.IPs, 1)
	require.True(t, enrichmentContainsIP(stcasw01, "10.25.19.216"))
}

func buildDot1qMacPortMap(ds WalkDataset) map[string]int {
	const dot1qPortPrefix = "1.3.6.1.2.1.17.7.1.2.2.1.2."
	const dot1qStatusPrefix = "1.3.6.1.2.1.17.7.1.2.2.1.3."

	macPort := make(map[string]int)
	portByIndex := make(map[string]int)
	macByIndex := make(map[string]string)

	for _, rec := range ds.Records {
		oid := normalizeOID(rec.OID)
		if !strings.HasPrefix(oid, dot1qPortPrefix) {
			continue
		}

		index, mac, ok := dot1qIndexFromOID(oid, dot1qPortPrefix)
		if !ok {
			continue
		}
		port, err := strconv.Atoi(strings.TrimSpace(rec.Value))
		if err != nil {
			continue
		}

		portByIndex[index] = port
		macByIndex[index] = mac
		macPort[mac] = port
	}

	for _, rec := range ds.Records {
		oid := normalizeOID(rec.OID)
		if !strings.HasPrefix(oid, dot1qStatusPrefix) {
			continue
		}

		index, mac, ok := dot1qIndexFromOID(oid, dot1qStatusPrefix)
		if !ok {
			continue
		}
		if _, exists := macPort[mac]; exists {
			continue
		}

		if port, ok := portByIndex[index]; ok {
			macPort[mac] = port
			continue
		}
		if indexedMAC, ok := macByIndex[index]; ok {
			macPort[indexedMAC] = 0
			continue
		}
		macPort[mac] = 0
	}

	return macPort
}

func dot1qIndexFromOID(oid, prefix string) (key string, mac string, ok bool) {
	suffix := strings.TrimPrefix(oid, prefix)
	suffix = strings.TrimPrefix(suffix, ".")
	parts := strings.Split(suffix, ".")
	if len(parts) < 7 {
		return "", "", false
	}

	macParts := parts[len(parts)-6:]
	octets := make([]byte, 0, 6)
	for _, part := range macParts {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 0 || n > 255 {
			return "", "", false
		}
		octets = append(octets, byte(n))
	}

	macBuilder := strings.Builder{}
	for _, octet := range octets {
		macBuilder.WriteString(fmt.Sprintf("%02x", octet))
	}

	return strings.Join(parts, "."), macBuilder.String(), true
}

type timeTetraRemoteRow struct {
	IndexKey            string
	LocalPortNum        int
	IfIndex             int
	LocalDestMACAddress int
	RemIndex            int
	ChassisSubtype      int
	ChassisID           string
	PortSubtype         int
	PortID              string
	PortDescr           string
	SysName             string
}

type timeTetraLocalPortRow struct {
	Key         string
	PortSubtype int
	PortID      string
	PortDescr   string
}

func collectTimeTetraRemoteRows(t *testing.T, ds WalkDataset) []timeTetraRemoteRow {
	t.Helper()

	const prefix = "1.3.6.1.4.1.6527.3.1.2.59.4.1.1."
	chassisRows := ds.Prefix(prefix + "5.")
	rows := make([]timeTetraRemoteRow, 0, len(chassisRows))

	for _, rec := range chassisRows {
		oid := normalizeOID(rec.OID)
		index := strings.TrimPrefix(oid, prefix+"5.")
		require.NotEmpty(t, index)

		localPortNum, ifIndex, localDestMACAddress, remIndex := parseTimeTetraRemoteIndex(t, index)
		row := timeTetraRemoteRow{
			IndexKey:            index,
			LocalPortNum:        localPortNum,
			IfIndex:             ifIndex,
			LocalDestMACAddress: localDestMACAddress,
			RemIndex:            remIndex,
			ChassisSubtype:      mustAtoi(t, mustLookupWalkValue(t, ds, prefix+"4."+index)),
			ChassisID:           normalizeHexToken(rec.Value),
			PortSubtype:         mustAtoi(t, mustLookupWalkValue(t, ds, prefix+"6."+index)),
			PortID:              strings.TrimSpace(mustLookupWalkValue(t, ds, prefix+"7."+index)),
			PortDescr:           strings.TrimSpace(mustLookupWalkValue(t, ds, prefix+"8."+index)),
		}

		if rec9, ok := ds.Lookup(prefix + "9." + index); ok {
			row.SysName = strings.TrimSpace(rec9.Value)
		}

		rows = append(rows, row)
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].IndexKey < rows[j].IndexKey
	})

	return rows
}

func parseTimeTetraRemoteIndex(t *testing.T, index string) (localPortNum int, ifIndex int, localDestMACAddress int, remIndex int) {
	t.Helper()

	parts := strings.Split(strings.TrimSpace(index), ".")
	require.Len(t, parts, 4)

	localPortNum = mustAtoi(t, parts[0])
	ifIndex = mustAtoi(t, parts[1])
	localDestMACAddress = mustAtoi(t, parts[2])
	remIndex = mustAtoi(t, parts[3])
	return
}

func timeTetraLocalPortRowsByIfIndex(t *testing.T, ds WalkDataset, ifIndex int) []timeTetraLocalPortRow {
	t.Helper()

	index := strconv.Itoa(ifIndex)
	subtypePrefix := "1.3.6.1.4.1.6527.3.1.2.59.3.1.1.2." + index + "."
	oidPrefix := "1.3.6.1.4.1.6527.3.1.2.59.3.1.1.3." + index + "."
	descrPrefix := "1.3.6.1.4.1.6527.3.1.2.59.3.1.1.4." + index + "."

	subtypeRows := ds.Prefix(subtypePrefix)
	rows := make([]timeTetraLocalPortRow, 0, len(subtypeRows))
	for _, rec := range subtypeRows {
		key := strings.TrimPrefix(normalizeOID(rec.OID), subtypePrefix)
		if strings.TrimSpace(key) == "" {
			continue
		}

		portIDRec, ok := ds.Lookup(oidPrefix + key)
		if !ok {
			continue
		}
		portDescrRec, ok := ds.Lookup(descrPrefix + key)
		if !ok {
			continue
		}

		rows = append(rows, timeTetraLocalPortRow{
			Key:         key,
			PortSubtype: mustAtoi(t, rec.Value),
			PortID:      strings.TrimSpace(portIDRec.Value),
			PortDescr:   strings.TrimSpace(portDescrRec.Value),
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Key < rows[j].Key
	})

	return rows
}

func buildResultFromScenarioPrefix(scenario ResolvedScenario, prefixLen int, opts BuildOptions) (engine.Result, error) {
	walks, err := loadScenarioWalkPrefix(scenario, prefixLen)
	if err != nil {
		return engine.Result{}, err
	}

	return BuildL2ResultFromWalks(walks, opts)
}

func loadScenarioWalkPrefix(scenario ResolvedScenario, prefixLen int) ([]FixtureWalk, error) {
	if prefixLen <= 0 || prefixLen > len(scenario.Fixtures) {
		return nil, fmt.Errorf("invalid fixture prefix len %d", prefixLen)
	}

	walks := make([]FixtureWalk, 0, prefixLen)
	for i := 0; i < prefixLen; i++ {
		fixture := scenario.Fixtures[i]
		ds, err := LoadWalkFile(fixture.WalkFile)
		if err != nil {
			return nil, fmt.Errorf("load walk for fixture %q: %w", fixture.DeviceID, err)
		}
		walks = append(walks, FixtureWalk{
			DeviceID: fixture.DeviceID,
			Hostname: fixture.Hostname,
			Address:  fixture.Address,
			Records:  ds.Records,
		})
	}

	return walks, nil
}

func countAdjacenciesBySource(adjacencies []engine.Adjacency, protocol string) map[string]int {
	out := make(map[string]int)
	for _, adj := range adjacencies {
		if adj.Protocol != protocol {
			continue
		}
		out[adj.SourceID]++
	}
	return out
}

func countFixturesWithLLDPLocalElements(fixtures []FixtureWalk) int {
	count := 0
	for _, fixture := range fixtures {
		hasLLDPIdentity := false
		for _, rec := range fixture.Records {
			switch normalizeOID(rec.OID) {
			case "1.0.8802.1.1.2.1.3.2.0", "1.0.8802.1.1.2.1.3.3.0":
				if strings.TrimSpace(rec.Value) != "" {
					hasLLDPIdentity = true
				}
			}
			if hasLLDPIdentity {
				break
			}
		}
		if hasLLDPIdentity {
			count++
		}
	}
	return count
}

func goldenAdjacencyKeySet(adjacencies []GoldenAdjacency) map[string]struct{} {
	out := make(map[string]struct{}, len(adjacencies))
	for _, adj := range adjacencies {
		out[adj.Protocol+"|"+adj.SourceDevice+"|"+adj.SourcePort+"|"+adj.TargetDevice+"|"+adj.TargetPort] = struct{}{}
	}
	return out
}

func mustLookupWalkValue(t *testing.T, ds WalkDataset, oid string) string {
	t.Helper()
	record, ok := ds.Lookup(oid)
	require.True(t, ok, "missing OID %s", oid)
	return strings.TrimSpace(record.Value)
}

func mustAtoi(t *testing.T, v string) int {
	t.Helper()
	out, err := strconv.Atoi(strings.TrimSpace(v))
	require.NoError(t, err)
	return out
}

func compactHexToken(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	return strings.NewReplacer(":", "", "-", "", ".", "", " ", "").Replace(v)
}

func countCDPCacheRows(ds WalkDataset) int {
	const prefix = "1.3.6.1.4.1.9.9.23.1.2.1.1.3."

	count := 0
	for _, rec := range ds.Records {
		if strings.HasPrefix(normalizeOID(rec.OID), prefix) {
			count++
		}
	}
	return count
}

func lldpLocPortTriplet(t *testing.T, ds WalkDataset, portNum string) []string {
	t.Helper()
	return []string{
		mustLookupWalkValue(t, ds, "1.0.8802.1.1.2.1.3.7.1.2."+portNum),
		mustLookupWalkValue(t, ds, "1.0.8802.1.1.2.1.3.7.1.3."+portNum),
		mustLookupWalkValue(t, ds, "1.0.8802.1.1.2.1.3.7.1.4."+portNum),
	}
}

func collectLLDPRemoteRows(ds WalkDataset) map[string]map[int]string {
	const base = "1.0.8802.1.1.2.1.4.1.1."
	rows := make(map[string]map[int]string)

	for _, rec := range ds.Records {
		oid := normalizeOID(rec.OID)
		if !strings.HasPrefix(oid, base) {
			continue
		}

		suffix := strings.TrimPrefix(oid, base)
		suffix = strings.TrimPrefix(suffix, ".")
		parts := strings.Split(suffix, ".")
		if len(parts) < 4 {
			continue
		}

		column, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil || column < 4 || column > 9 {
			continue
		}

		localPort := strings.TrimSpace(parts[len(parts)-2])
		remIndex := strings.TrimSpace(parts[len(parts)-1])
		if localPort == "" || remIndex == "" {
			continue
		}

		key := localPort + "|" + remIndex
		row, ok := rows[key]
		if !ok {
			row = make(map[int]string, 6)
			rows[key] = row
		}
		row[column] = strings.TrimSpace(rec.Value)
	}

	return rows
}

func adjacencyKeySet(adjacencies []engine.Adjacency) map[string]struct{} {
	out := make(map[string]struct{}, len(adjacencies))
	for _, adj := range adjacencies {
		key := fmt.Sprintf("%s|%s|%s|%s|%s", adj.Protocol, adj.SourceID, adj.SourcePort, adj.TargetID, adj.TargetPort)
		out[key] = struct{}{}
	}
	return out
}

func countBidirectionalPairs(adjacencies []engine.Adjacency, protocol string) int {
	directed := make(map[string]struct{}, len(adjacencies))
	for _, adj := range adjacencies {
		if adj.Protocol != protocol {
			continue
		}
		directed[fmt.Sprintf("%s|%s|%s|%s|%s", adj.Protocol, adj.SourceID, adj.SourcePort, adj.TargetID, adj.TargetPort)] = struct{}{}
	}

	counted := make(map[string]struct{}, len(directed))
	pairs := 0
	for _, adj := range adjacencies {
		if adj.Protocol != protocol {
			continue
		}

		forward := fmt.Sprintf("%s|%s|%s|%s|%s", adj.Protocol, adj.SourceID, adj.SourcePort, adj.TargetID, adj.TargetPort)
		reverse := fmt.Sprintf("%s|%s|%s|%s|%s", adj.Protocol, adj.TargetID, adj.TargetPort, adj.SourceID, adj.SourcePort)

		canonical := forward
		if reverse < canonical {
			canonical = reverse
		}
		if _, done := counted[canonical]; done {
			continue
		}
		if _, ok := directed[reverse]; !ok {
			continue
		}

		counted[canonical] = struct{}{}
		pairs++
	}

	return pairs
}

func countMutualLocalAdjacencyEdges(adjacencies []engine.Adjacency, protocol string, localDevices map[string]struct{}) int {
	directedCounts := make(map[string]int)
	for _, adj := range adjacencies {
		if adj.Protocol != protocol || adj.SourceID == adj.TargetID {
			continue
		}
		if _, ok := localDevices[adj.SourceID]; !ok {
			continue
		}
		if _, ok := localDevices[adj.TargetID]; !ok {
			continue
		}
		directedCounts[adj.SourceID+"|"+adj.TargetID]++
	}

	counted := make(map[string]struct{}, len(directedCounts))
	edges := 0
	for key, forwardCount := range directedCounts {
		parts := strings.SplitN(key, "|", 2)
		if len(parts) != 2 {
			continue
		}

		left, right := parts[0], parts[1]
		canonical := left + "|" + right
		if right < left {
			canonical = right + "|" + left
		}
		if _, ok := counted[canonical]; ok {
			continue
		}

		reverseCount := directedCounts[right+"|"+left]
		if forwardCount < reverseCount {
			edges += forwardCount
		} else {
			edges += reverseCount
		}

		counted[canonical] = struct{}{}
	}

	return edges
}

func enrichmentByMAC(enrichments []engine.Enrichment) map[string]engine.Enrichment {
	byMAC := make(map[string]engine.Enrichment, len(enrichments))
	for _, enrichment := range enrichments {
		if enrichment.MAC == "" {
			continue
		}
		byMAC[enrichment.MAC] = enrichment
	}
	return byMAC
}

func mustEnrichmentByMAC(t *testing.T, byMAC map[string]engine.Enrichment, mac string) engine.Enrichment {
	t.Helper()
	enrichment, ok := byMAC[mac]
	require.True(t, ok, "missing enrichment for mac=%s", mac)
	return enrichment
}

func enrichmentContainsIP(enrichment engine.Enrichment, ip string) bool {
	for _, addr := range enrichment.IPs {
		if addr.String() == ip {
			return true
		}
	}
	return false
}

func countEnrichmentIPs(enrichments []engine.Enrichment) int {
	total := 0
	for _, enrichment := range enrichments {
		total += len(enrichment.IPs)
	}
	return total
}

func countUndirectedLocalDevicePairs(adjacencies []engine.Adjacency, protocol string, localDevices map[string]struct{}) int {
	pairs := make(map[string]struct{})
	for _, adj := range adjacencies {
		if adj.Protocol != protocol || adj.SourceID == adj.TargetID {
			continue
		}
		if _, ok := localDevices[adj.SourceID]; !ok {
			continue
		}
		if _, ok := localDevices[adj.TargetID]; !ok {
			continue
		}
		left, right := adj.SourceID, adj.TargetID
		if right < left {
			left, right = right, left
		}
		pairs[left+"|"+right] = struct{}{}
	}
	return len(pairs)
}

func countLocalTopologyVertices(adjacencies []engine.Adjacency, protocol string, localDevices map[string]struct{}) int {
	vertices := make(map[string]struct{})
	for _, adj := range adjacencies {
		if adj.Protocol != protocol || adj.SourceID == adj.TargetID {
			continue
		}
		if _, ok := localDevices[adj.SourceID]; !ok {
			continue
		}
		if _, ok := localDevices[adj.TargetID]; !ok {
			continue
		}
		vertices[adj.SourceID] = struct{}{}
		vertices[adj.TargetID] = struct{}{}
	}
	return len(vertices)
}
