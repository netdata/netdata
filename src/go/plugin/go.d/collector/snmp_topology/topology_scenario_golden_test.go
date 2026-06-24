// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyv1test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const topologyScenarioGoldenDirEnv = "NETDATA_SNMP_TOPOLOGY_SCENARIO_GOLDEN_DIR"

var updateSNMPTopologyScenarioGoldens = flag.Bool("update-snmp-topology-scenario-goldens", false, "update SNMP topology scenario golden files")

type topologyScenarioCase struct {
	scenario *topologyScenario
	assert   func(testing.TB, topologyv1test.NormalizedData)
}

type topologyScenarioActorExpectation struct {
	Name         string
	Type         string
	ManagementIP string
}

type topologyScenarioLinkExpectation struct {
	Type      string
	Src       string
	Dst       string
	Direction string
	Protocol  string
	SrcPort   string
	DstPort   string
	State     string
}

func topologyScenarioCases() map[string]topologyScenarioCase {
	return map[string]topologyScenarioCase{
		"mixed_l2_l3_control": {
			scenario: newMixedL2L3ControlScenario(),
			assert:   assertMixedL2L3ControlScenario,
		},
		"probable_fdb_low_confidence": {
			scenario: newProbableFDBLowConfidenceScenario(),
			assert:   assertProbableFDBLowConfidenceScenario,
		},
		"focus_depth_l2": {
			scenario: newFocusDepthL2Scenario(),
			assert:   assertFocusDepthL2Scenario,
		},
		"cdp_direct": {
			scenario: newCDPDirectScenario(),
			assert:   assertCDPDirectScenario,
		},
		"stp_inferred": {
			scenario: newSTPInferredScenario(),
			assert:   assertSTPInferredScenario,
		},
		"l3_p2p_31": {
			scenario: newL3P2P31Scenario(),
			assert:   assertL3P2P31Scenario,
		},
	}
}

func topologyScenarioGoldenCases() map[string]topologyScenarioCase {
	all := topologyScenarioCases()
	return map[string]topologyScenarioCase{
		"mixed_l2_l3_control":         all["mixed_l2_l3_control"],
		"probable_fdb_low_confidence": all["probable_fdb_low_confidence"],
		"focus_depth_l2":              all["focus_depth_l2"],
	}
}

func TestSNMPTopologyScenarioSemantics(t *testing.T) {
	for name, tc := range topologyScenarioCases() {
		t.Run(name, func(t *testing.T) {
			payload := tc.scenario.render(t)
			normalized := topologyv1test.NormalizeData(t, payload)
			tc.assert(t, normalized)

			actual := topologyv1test.CanonicalJSON(t, normalized)
			second := topologyv1test.CanonicalJSON(t, topologyv1test.NormalizeData(t, tc.scenario.render(t)))
			require.Equal(t, actual, second, "scenario output must be deterministic")
		})
	}
}

func TestSNMPTopologyScenarioGoldens(t *testing.T) {
	dir, ok := topologyScenarioGoldenDir(t)
	if !ok {
		if *updateSNMPTopologyScenarioGoldens {
			t.Fatalf("SNMP topology scenario golden directory is missing; set %s or checkout netdata/testdata into src/go/testdata", topologyScenarioGoldenDirEnv)
		}
		t.Skipf("SNMP topology scenario golden directory is missing; set %s or checkout netdata/testdata into src/go/testdata", topologyScenarioGoldenDirEnv)
	}

	for name, tc := range topologyScenarioGoldenCases() {
		t.Run(name, func(t *testing.T) {
			payload := tc.scenario.render(t)
			normalized := topologyv1test.NormalizeData(t, payload)

			actual := topologyv1test.CanonicalJSON(t, normalized)
			if *updateSNMPTopologyScenarioGoldens {
				topologyScenarioWriteGolden(t, dir, name, actual)
			} else {
				require.Equal(t, topologyScenarioReadGolden(t, dir, name), actual)
			}
		})
	}
}

func topologyScenarioGoldenDir(t testing.TB) (string, bool) {
	t.Helper()

	if dir := strings.TrimSpace(os.Getenv(topologyScenarioGoldenDirEnv)); dir != "" {
		return dir, topologyScenarioDirExists(t, dir) || *updateSNMPTopologyScenarioGoldens
	}

	root, ok := topologyScenarioGoRoot(t)
	if !ok {
		return "", false
	}
	dir := filepath.Join(root, "testdata", "snmp", "topology-scenarios")
	return dir, topologyScenarioDirExists(t, dir)
}

func topologyScenarioGoRoot(t testing.TB) (string, bool) {
	t.Helper()

	wd, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd, true
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			return "", false
		}
		wd = parent
	}
}

func topologyScenarioDirExists(t testing.TB, dir string) bool {
	t.Helper()

	info, err := os.Stat(dir)
	if err == nil {
		return info.IsDir()
	}
	if os.IsNotExist(err) {
		return false
	}
	require.NoError(t, err)
	return false
}

func topologyScenarioGoldenPath(dir, name string) string {
	return filepath.Join(dir, name+".golden.json")
}

func topologyScenarioReadGolden(t testing.TB, dir, name string) string {
	t.Helper()
	bs, err := os.ReadFile(topologyScenarioGoldenPath(dir, name))
	require.NoError(t, err)
	return string(bs)
}

func topologyScenarioWriteGolden(t testing.TB, dir, name, payload string) {
	t.Helper()
	path := topologyScenarioGoldenPath(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(payload), 0o644))
}

func newMixedL2L3ControlScenario() *topologyScenario {
	s := newTopologyScenario("mixed_l2_l3_control").WithOptions(func(options *topologyoptions.QueryOptions) {
		options.CollapseActorsByIP = false
	})
	routerA := s.Router("router-a", "192.0.2.1", "02:00:00:00:01:01", "192.0.2.101", "65001")
	routerB := s.Router("router-b", "192.0.2.2", "02:00:00:00:01:02", "192.0.2.102", "65002")
	switchA := s.Switch("switch-a", "192.0.2.11", "02:00:00:00:02:01")
	switchB := s.Switch("switch-b", "192.0.2.12", "02:00:00:00:02:02")

	routerAWAN := routerA.Port("wan0", 1).IPv4("198.51.100.1/30")
	routerBWAN := routerB.Port("wan0", 1).IPv4("198.51.100.2/30")
	routerALAN := routerA.Port("lan0", 2).IPv4("10.10.10.1/24")
	switchAUplink := switchA.Port("uplink-a", 1).IPv4("10.10.10.2/24")
	switchBUplink := switchB.Port("uplink-b", 1).IPv4("10.10.10.3/24")
	switchAHost := switchA.Port("host-a", 2)

	s.LLDP(routerALAN, switchAUplink)
	s.CDP(switchAUplink, switchBUplink)
	s.STP(switchAUplink, switchBUplink)
	s.OSPF(routerAWAN, routerBWAN)
	s.BGP(routerA, routerB, "default")
	s.FDBARP(switchAHost, "02:00:00:00:10:11", "10.10.10.50")
	return s
}

func newProbableFDBLowConfidenceScenario() *topologyScenario {
	s := newTopologyScenario("probable_fdb_low_confidence").WithOptions(func(options *topologyoptions.QueryOptions) {
		options.MapType = topologyoptions.MapTypeAllDevicesLowConfidence
		options.InferenceStrategy = topologyoptions.InferenceStrategyFDBPairwise
	})
	switchA := s.Switch("switch-a", "192.0.2.21", "02:00:00:00:03:01")
	switchB := s.Switch("switch-b", "192.0.2.22", "02:00:00:00:03:02")
	aPort := switchA.Port("uplink-a", 1)
	bPort := switchB.Port("uplink-b", 1)
	s.FDBARP(aPort, switchB.chassisMAC, switchB.mgmtIP)
	s.FDBARP(bPort, switchA.chassisMAC, switchA.mgmtIP)
	return s
}

func newFocusDepthL2Scenario() *topologyScenario {
	s := newTopologyScenario("focus_depth_l2").WithOptions(func(options *topologyoptions.QueryOptions) {
		options.CollapseActorsByIP = false
		options.ManagedDeviceFocus = topologyoptions.ManagedFocusIPPrefix + "192.0.2.31"
		options.Depth = 1
	})
	switchA := s.Switch("focus-switch-a", "192.0.2.31", "02:00:00:00:04:01")
	switchB := s.Switch("focus-switch-b", "192.0.2.32", "02:00:00:00:04:02")
	switchC := s.Switch("focus-switch-c", "192.0.2.33", "02:00:00:00:04:03")
	s.LLDP(switchA.Port("a-b", 1), switchB.Port("b-a", 1))
	s.LLDP(switchB.Port("b-c", 2), switchC.Port("c-b", 1))
	return s
}

func newCDPDirectScenario() *topologyScenario {
	s := newTopologyScenario("cdp_direct").WithOptions(func(options *topologyoptions.QueryOptions) {
		options.CollapseActorsByIP = false
	})
	switchA := s.Switch("cdp-switch-a", "192.0.2.41", "02:00:00:00:05:01")
	switchB := s.Switch("cdp-switch-b", "192.0.2.42", "02:00:00:00:05:02")
	s.CDP(switchA.Port("a-b", 1), switchB.Port("b-a", 1))
	return s
}

func newSTPInferredScenario() *topologyScenario {
	s := newTopologyScenario("stp_inferred").WithOptions(func(options *topologyoptions.QueryOptions) {
		options.CollapseActorsByIP = false
		options.MapType = topologyoptions.MapTypeHighConfidenceInferred
		options.InferenceStrategy = topologyoptions.InferenceStrategySTPParentTree
	})
	switchA := s.Switch("stp-switch-a", "192.0.2.51", "02:00:00:00:06:01")
	switchB := s.Switch("stp-switch-b", "192.0.2.52", "02:00:00:00:06:02")
	s.STP(switchA.Port("a-b", 1), switchB.Port("b-a", 1))
	return s
}

func newL3P2P31Scenario() *topologyScenario {
	s := newTopologyScenario("l3_p2p_31").WithOptions(func(options *topologyoptions.QueryOptions) {
		options.CollapseActorsByIP = false
	})
	routerA := s.Router("p2p-router-a", "192.0.2.61", "02:00:00:00:07:01", "192.0.2.161", "65061")
	routerB := s.Router("p2p-router-b", "192.0.2.62", "02:00:00:00:07:02", "192.0.2.162", "65062")
	routerA.Port("wan0", 1).IPv4("198.51.100.10/31")
	routerB.Port("wan0", 1).IPv4("198.51.100.11/31")
	return s
}

func assertMixedL2L3ControlScenario(t testing.TB, data topologyv1test.NormalizedData) {
	t.Helper()

	assertScenarioActors(t, data, []topologyScenarioActorExpectation{
		{Name: "router-a", Type: "router", ManagementIP: "192.0.2.1"},
		{Name: "router-b", Type: "router", ManagementIP: "192.0.2.2"},
		{Name: "switch-a", Type: "switch", ManagementIP: "192.0.2.11"},
		{Name: "switch-b", Type: "switch", ManagementIP: "192.0.2.12"},
		{Name: "10.10.10.0/24", Type: "l3_subnet_segment"},
	})
	assertScenarioLinks(t, data, []topologyScenarioLinkExpectation{
		{Type: "lldp", Src: "router-a", Dst: "switch-a", Direction: "bidirectional", Protocol: "lldp", SrcPort: "lan0", DstPort: "uplink-a"},
		{Type: "cdp", Src: "switch-a", Dst: "switch-b", Direction: "bidirectional", Protocol: "cdp", SrcPort: "uplink-a", DstPort: "uplink-b"},
		{Type: topologymodel.L3SubnetMembershipLinkType, Src: "router-a", Dst: "10.10.10.0/24", Direction: "observed", Protocol: topologymodel.L3SubnetMembershipLinkType, SrcPort: "lan0"},
		{Type: topologymodel.L3SubnetMembershipLinkType, Src: "switch-a", Dst: "10.10.10.0/24", Direction: "observed", Protocol: topologymodel.L3SubnetMembershipLinkType, SrcPort: "uplink-a"},
		{Type: topologymodel.L3SubnetMembershipLinkType, Src: "switch-b", Dst: "10.10.10.0/24", Direction: "observed", Protocol: topologymodel.L3SubnetMembershipLinkType, SrcPort: "uplink-b"},
		{Type: topologymodel.L3SubnetLinkType, Src: "router-a", Dst: "router-b", Direction: "observed", Protocol: topologymodel.L3SubnetLinkType, SrcPort: "wan0", DstPort: "wan0"},
		{Type: topologymodel.BGPAdjacencyLinkType, Src: "router-a", Dst: "router-b", Direction: "observed", Protocol: topologymodel.BGPAdjacencyLinkType, State: "established"},
		{Type: topologymodel.OSPFAdjacencyLinkType, Src: "router-a", Dst: "router-b", Direction: "observed", Protocol: topologymodel.OSPFAdjacencyLinkType, State: "full"},
	})
	assertScenarioTableRowsContain(t, data, "actor_ports", []map[string]any{
		{"actor": "router-a", "if_index": 2, "name": "lan0", "neighbor_actor": "switch-a", "neighbor_port_name": "uplink-a", "neighbor_count": 1, "link_count": 1, "topology_role": "switch_facing"},
		{"actor": "switch-a", "if_index": 1, "name": "uplink-a", "neighbor_count": 2, "link_count": 4, "stp_state": "forwarding", "topology_role": "switch_facing"},
		{"actor": "switch-a", "if_index": 2, "name": "host-a", "fdb_mac_count": 1, "topology_role": "host_facing"},
	})
	assertScenarioTableRowsExactly(t, data, "actor_port_links", []map[string]any{
		{"actor": "router-a", "remote_actor": "switch-a", "type": "lldp", "protocol": "lldp", "if_index": 2, "port_name": "lan0", "remote_if_index": 1, "remote_port_name": "uplink-a"},
		{"actor": "switch-a", "remote_actor": "router-a", "type": "lldp", "protocol": "lldp", "if_index": 1, "port_name": "uplink-a", "remote_if_index": 2, "remote_port_name": "lan0"},
		{"actor": "switch-a", "remote_actor": "switch-b", "type": "cdp", "protocol": "cdp", "if_index": 1, "port_name": "uplink-a", "remote_if_index": 1, "remote_port_name": "uplink-b"},
		{"actor": "switch-b", "remote_actor": "switch-a", "type": "cdp", "protocol": "cdp", "if_index": 1, "port_name": "uplink-b", "remote_if_index": 1, "remote_port_name": "uplink-a"},
	})
	assertScenarioTableRowsExactly(t, data, "actor_ospf_neighbors", []map[string]any{
		{"actor": "router-a", "remote_actor": "router-b", "local_ip": "198.51.100.1", "neighbor_ip": "198.51.100.2", "local_router_id": "192.0.2.101", "neighbor_router_id": "192.0.2.102", "state": "full", "subnet": "198.51.100.0/30"},
		{"actor": "router-b", "remote_actor": "router-a", "local_ip": "198.51.100.2", "neighbor_ip": "198.51.100.1", "local_router_id": "192.0.2.102", "neighbor_router_id": "192.0.2.101", "state": "full", "subnet": "198.51.100.0/30"},
	})
	assertScenarioTableRowsExactly(t, data, "actor_bgp_peers", []map[string]any{
		{"actor": "router-a", "remote_actor": "router-b", "local_ip": "198.51.100.1", "neighbor_ip": "198.51.100.2", "local_as": "65001", "remote_as": "65002", "local_identifier": "192.0.2.101", "peer_identifier": "192.0.2.102", "state": "established", "routing_instance": "default"},
		{"actor": "router-b", "remote_actor": "router-a", "local_ip": "198.51.100.2", "neighbor_ip": "198.51.100.1", "local_as": "65002", "remote_as": "65001", "local_identifier": "192.0.2.102", "peer_identifier": "192.0.2.101", "state": "established", "routing_instance": "default"},
	})
	assertScenarioEvidenceRowsExactly(t, data, "lldp", []map[string]any{
		{"src_actor": "router-a", "dst_actor": "switch-a", "src_if_index": 2, "dst_if_index": 1, "src_port_name": "lan0", "dst_port_name": "uplink-a", "protocol": "lldp"},
	})
	assertScenarioEvidenceRowsExactly(t, data, "cdp", []map[string]any{
		{"src_actor": "switch-a", "dst_actor": "switch-b", "src_if_index": 1, "dst_if_index": 1, "src_port_name": "uplink-a", "dst_port_name": "uplink-b", "protocol": "cdp"},
	})
	assertScenarioEvidenceRowsExactly(t, data, topologymodel.L3SubnetLinkType, []map[string]any{
		{"src_actor": "router-a", "dst_actor": "router-b", "src_ip": "198.51.100.1", "dst_ip": "198.51.100.2", "src_if_index": 1, "dst_if_index": 1, "src_port_name": "wan0", "dst_port_name": "wan0", "subnet": "198.51.100.0/30", "prefix": 30},
	})
	assertScenarioEvidenceRowsExactly(t, data, topologymodel.OSPFAdjacencyLinkType, []map[string]any{
		{"src_actor": "router-a", "dst_actor": "router-b", "src_ip": "198.51.100.1", "dst_ip": "198.51.100.2", "src_router_id": "192.0.2.101", "dst_router_id": "192.0.2.102", "state": "full", "subnet": "198.51.100.0/30"},
	})
	assertScenarioEvidenceRowsExactly(t, data, topologymodel.BGPAdjacencyLinkType, []map[string]any{
		{"src_actor": "router-a", "dst_actor": "router-b", "local_ip": "198.51.100.1", "neighbor_ip": "198.51.100.2", "local_as": "65001", "remote_as": "65002", "local_identifier": "192.0.2.101", "peer_identifier": "192.0.2.102", "state": "established", "routing_instance": "default"},
	})
	assertScenarioNoActorPortLinkType(t, data, topologymodel.L3SubnetLinkType)
	assertScenarioStatEquals(t, data, "links_cdp", 1)
	assertScenarioStatEquals(t, data, "links_stp", 2)
	assertScenarioStatEquals(t, data, "links_fdb", 1)
	assertScenarioStatEquals(t, data, "l3_subnet_segment_suppressed_no_producer_scope", 0)
}

func assertProbableFDBLowConfidenceScenario(t testing.TB, data topologyv1test.NormalizedData) {
	t.Helper()

	assertScenarioActors(t, data, []topologyScenarioActorExpectation{
		{Name: "switch-a", Type: "switch", ManagementIP: "192.0.2.21"},
		{Name: "switch-b", Type: "switch", ManagementIP: "192.0.2.22"},
		{Name: "switch-a.uplink-a.segment", Type: "segment"},
	})
	assertScenarioLinks(t, data, []topologyScenarioLinkExpectation{
		{Type: "bridge", Src: "switch-a", Dst: "switch-a.uplink-a.segment", Direction: "bidirectional", Protocol: "bridge", SrcPort: "uplink-a"},
		{Type: "bridge", Src: "switch-b", Dst: "switch-a.uplink-a.segment", Direction: "bidirectional", Protocol: "bridge", SrcPort: "uplink-b"},
	})
	assertScenarioTableRowsContain(t, data, "actor_ports", []map[string]any{
		{"actor": "switch-a", "if_index": 1, "name": "uplink-a", "neighbor_actor": "switch-a.uplink-a.segment", "link_count": 1, "fdb_mac_count": 1, "topology_role": "switch_facing"},
	})
	assertScenarioTableRowsExactly(t, data, "actor_port_links", []map[string]any{
		{"actor": "switch-a", "remote_actor": "switch-a.uplink-a.segment", "type": "bridge", "protocol": "bridge", "if_index": 1, "port_name": "uplink-a"},
		{"actor": "switch-b", "remote_actor": "switch-a.uplink-a.segment", "type": "bridge", "protocol": "bridge", "if_index": 1, "port_name": "uplink-b"},
		{"actor": "switch-a.uplink-a.segment", "remote_actor": "switch-a", "type": "bridge", "protocol": "bridge", "remote_if_index": 1, "remote_port_name": "uplink-a"},
		{"actor": "switch-a.uplink-a.segment", "remote_actor": "switch-b", "type": "bridge", "protocol": "bridge", "remote_if_index": 1, "remote_port_name": "uplink-b"},
	})
	assertScenarioEvidenceRowsExactly(t, data, "bridge", []map[string]any{
		{"src_actor": "switch-a", "dst_actor": "switch-a.uplink-a.segment", "src_if_index": 1, "src_port_name": "uplink-a", "protocol": "bridge"},
		{"src_actor": "switch-b", "dst_actor": "switch-a.uplink-a.segment", "src_if_index": 1, "src_port_name": "uplink-b", "protocol": "bridge"},
	})
	assertScenarioStatEquals(t, data, "attachments_fdb", 2)
	assertScenarioStatEquals(t, data, "enrichments_arp_nd", 2)
	assertScenarioStatEquals(t, data, "l3_subnet_segment_suppressed_no_producer_scope", 0)
}

func assertFocusDepthL2Scenario(t testing.TB, data topologyv1test.NormalizedData) {
	t.Helper()

	assertScenarioActors(t, data, []topologyScenarioActorExpectation{
		{Name: "focus-switch-a", Type: "switch", ManagementIP: "192.0.2.31"},
		{Name: "focus-switch-b", Type: "switch", ManagementIP: "192.0.2.32"},
	})
	assertScenarioLinks(t, data, []topologyScenarioLinkExpectation{
		{Type: "lldp", Src: "focus-switch-a", Dst: "focus-switch-b", Direction: "bidirectional", Protocol: "lldp", SrcPort: "a-b", DstPort: "b-a"},
	})
	assertScenarioTableRowsContain(t, data, "actor_ports", []map[string]any{
		{"actor": "focus-switch-a", "if_index": 1, "name": "a-b", "neighbor_count": 1, "link_count": 1, "topology_role": "switch_facing"},
	})
	assertScenarioTableRowsExactly(t, data, "actor_port_links", []map[string]any{
		{"actor": "focus-switch-a", "remote_actor": "focus-switch-b", "type": "lldp", "protocol": "lldp", "if_index": 1, "port_name": "a-b", "remote_if_index": 1, "remote_port_name": "b-a"},
		{"actor": "focus-switch-b", "remote_actor": "focus-switch-a", "type": "lldp", "protocol": "lldp", "if_index": 1, "port_name": "b-a", "remote_if_index": 1, "remote_port_name": "a-b"},
	})
	assertScenarioEvidenceRowsExactly(t, data, "lldp", []map[string]any{
		{"src_actor": "focus-switch-a", "dst_actor": "focus-switch-b", "src_if_index": 1, "dst_if_index": 1, "src_port_name": "a-b", "dst_port_name": "b-a", "protocol": "lldp"},
	})
	assertScenarioStatEquals(t, data, "actors_focus_depth_filtered", 1)
	assertScenarioStatEquals(t, data, "links_total", 1)
	assertScenarioStatEquals(t, data, "links_lldp", 2)
	assertScenarioNoActor(t, data, "focus-switch-c")
}

func assertCDPDirectScenario(t testing.TB, data topologyv1test.NormalizedData) {
	t.Helper()

	assertScenarioActors(t, data, []topologyScenarioActorExpectation{
		{Name: "cdp-switch-a", Type: "switch", ManagementIP: "192.0.2.41"},
		{Name: "cdp-switch-b", Type: "switch", ManagementIP: "192.0.2.42"},
	})
	assertScenarioLinks(t, data, []topologyScenarioLinkExpectation{
		{Type: "cdp", Src: "cdp-switch-a", Dst: "cdp-switch-b", Direction: "bidirectional", Protocol: "cdp", SrcPort: "a-b", DstPort: "b-a"},
	})
	assertScenarioTableRowsExactly(t, data, "actor_port_links", []map[string]any{
		{"actor": "cdp-switch-a", "remote_actor": "cdp-switch-b", "type": "cdp", "protocol": "cdp", "if_index": 1, "port_name": "a-b", "remote_if_index": 1, "remote_port_name": "b-a"},
		{"actor": "cdp-switch-b", "remote_actor": "cdp-switch-a", "type": "cdp", "protocol": "cdp", "if_index": 1, "port_name": "b-a", "remote_if_index": 1, "remote_port_name": "a-b"},
	})
	assertScenarioEvidenceRowsExactly(t, data, "cdp", []map[string]any{
		{"src_actor": "cdp-switch-a", "dst_actor": "cdp-switch-b", "src_if_index": 1, "dst_if_index": 1, "src_port_name": "a-b", "dst_port_name": "b-a", "protocol": "cdp"},
	})
	assertScenarioStatEquals(t, data, "links_cdp", 1)
}

func assertSTPInferredScenario(t testing.TB, data topologyv1test.NormalizedData) {
	t.Helper()

	assertScenarioActors(t, data, []topologyScenarioActorExpectation{
		{Name: "stp-switch-a", Type: "switch", ManagementIP: "192.0.2.51"},
		{Name: "stp-switch-b", Type: "switch", ManagementIP: "192.0.2.52"},
	})
	assertScenarioLinks(t, data, []topologyScenarioLinkExpectation{
		{Type: "stp", Src: "stp-switch-a", Dst: "stp-switch-b", Direction: "unidirectional", Protocol: "stp", SrcPort: "a-b", DstPort: "b-a"},
		{Type: "stp", Src: "stp-switch-b", Dst: "stp-switch-a", Direction: "unidirectional", Protocol: "stp", SrcPort: "b-a", DstPort: "a-b"},
	})
	assertScenarioEvidenceRowsExactly(t, data, "stp", []map[string]any{
		{"src_actor": "stp-switch-a", "dst_actor": "stp-switch-b", "src_if_index": 1, "dst_if_index": 1, "src_port_name": "a-b", "dst_port_name": "b-a", "protocol": "stp"},
		{"src_actor": "stp-switch-b", "dst_actor": "stp-switch-a", "src_if_index": 1, "dst_if_index": 1, "src_port_name": "b-a", "dst_port_name": "a-b", "protocol": "stp"},
	})
	assertScenarioStatEquals(t, data, "links_stp", 2)
}

func assertL3P2P31Scenario(t testing.TB, data topologyv1test.NormalizedData) {
	t.Helper()

	assertScenarioActors(t, data, []topologyScenarioActorExpectation{
		{Name: "p2p-router-a", Type: "router", ManagementIP: "192.0.2.61"},
		{Name: "p2p-router-b", Type: "router", ManagementIP: "192.0.2.62"},
	})
	assertScenarioLinks(t, data, []topologyScenarioLinkExpectation{
		{Type: topologymodel.L3SubnetLinkType, Src: "p2p-router-a", Dst: "p2p-router-b", Direction: "observed", Protocol: topologymodel.L3SubnetLinkType, SrcPort: "wan0", DstPort: "wan0"},
	})
	assertScenarioEvidenceRowsExactly(t, data, topologymodel.L3SubnetLinkType, []map[string]any{
		{"src_actor": "p2p-router-a", "dst_actor": "p2p-router-b", "src_ip": "198.51.100.10", "dst_ip": "198.51.100.11", "src_if_index": 1, "dst_if_index": 1, "src_port_name": "wan0", "dst_port_name": "wan0", "subnet": "198.51.100.10/31", "prefix": 31},
	})
	assertScenarioNoActor(t, data, "198.51.100.10/31")
	assertScenarioStatEquals(t, data, "l3_subnet_visible_links", 1)
	assertScenarioStatEquals(t, data, "l3_subnet_segment_emitted_segments", 0)
}

func assertScenarioActors(t testing.TB, data topologyv1test.NormalizedData, want []topologyScenarioActorExpectation) {
	t.Helper()

	require.Len(t, data.Actors.Rows, len(want), "unexpected actor set: %v", data.Actors.Rows)
	for _, expected := range want {
		row := scenarioActorByName(t, data, expected.Name)
		assert.Equal(t, expected.Type, row["type"], "actor %q type", expected.Name)
		if expected.ManagementIP != "" {
			assert.Equal(t, expected.ManagementIP, row["management_ip"], "actor %q management_ip", expected.Name)
		}
	}
}

func assertScenarioNoActor(t testing.TB, data topologyv1test.NormalizedData, displayName string) {
	t.Helper()

	for _, row := range data.Actors.Rows {
		require.NotEqual(t, displayName, row["display_name"])
		require.NotEqual(t, displayName, row["sys_name"])
	}
}

func assertScenarioLinks(t testing.TB, data topologyv1test.NormalizedData, want []topologyScenarioLinkExpectation) {
	t.Helper()

	expectedRows := make([]map[string]any, 0, len(want))
	for _, expected := range want {
		row := map[string]any{
			"type":      expected.Type,
			"src_actor": expected.Src,
			"dst_actor": expected.Dst,
		}
		if expected.Direction != "" {
			row["direction"] = expected.Direction
		}
		if expected.Protocol != "" {
			row["protocol"] = expected.Protocol
		}
		if expected.SrcPort != "" {
			row["src_port_name"] = expected.SrcPort
		}
		if expected.DstPort != "" {
			row["dst_port_name"] = expected.DstPort
		}
		if expected.State != "" {
			row["state"] = expected.State
		}
		expectedRows = append(expectedRows, row)
	}
	assertScenarioRowsExactly(t, data, "links", data.Links.Rows, expectedRows)
}

func assertScenarioTableRowsContain(t testing.TB, data topologyv1test.NormalizedData, table string, want []map[string]any) {
	t.Helper()

	rows, ok := scenarioActorTableRows(data, table)
	require.Truef(t, ok, "missing actor table %q", table)
	assertScenarioRowsContain(t, data, "actor table "+table, rows, want)
}

func assertScenarioTableRowsExactly(t testing.TB, data topologyv1test.NormalizedData, table string, want []map[string]any) {
	t.Helper()

	rows, ok := scenarioActorTableRows(data, table)
	if len(want) == 0 && !ok {
		return
	}
	require.Truef(t, ok, "missing actor table %q", table)
	assertScenarioRowsExactly(t, data, "actor table "+table, rows, want)
}

func assertScenarioEvidenceRowsExactly(t testing.TB, data topologyv1test.NormalizedData, table string, want []map[string]any) {
	t.Helper()

	section, ok := data.Evidence[table]
	if len(want) == 0 && !ok {
		return
	}
	require.Truef(t, ok, "missing evidence table %q", table)
	assertScenarioRowsExactly(t, data, "evidence table "+table, section.Table.Rows, want)
}

func scenarioActorTableRows(data topologyv1test.NormalizedData, table string) ([]map[string]any, bool) {
	if data.Tables == nil || data.Tables.Actor == nil {
		return nil, false
	}
	section, ok := data.Tables.Actor[table]
	if !ok {
		return nil, false
	}
	return section.Table.Rows, true
}

func assertScenarioRowsExactly(t testing.TB, data topologyv1test.NormalizedData, label string, rows []map[string]any, want []map[string]any) {
	t.Helper()

	require.Lenf(t, rows, len(want), "%s rows differ: %v", label, rows)
	assertScenarioRowsContain(t, data, label, rows, want)
}

func assertScenarioRowsContain(t testing.TB, data topologyv1test.NormalizedData, label string, rows []map[string]any, want []map[string]any) {
	t.Helper()

	matched := make([]bool, len(rows))
	for _, expected := range want {
		expected = scenarioResolveExpectedRow(t, data, expected)
		match := -1
		for i, row := range rows {
			if matched[i] || !scenarioRowContains(row, expected) {
				continue
			}
			match = i
			break
		}
		require.NotEqualf(t, -1, match, "%s missing expected row %v in rows %v", label, expected, rows)
		matched[match] = true
	}
}

func scenarioResolveExpectedRow(t testing.TB, data topologyv1test.NormalizedData, row map[string]any) map[string]any {
	t.Helper()

	resolved := make(map[string]any, len(row))
	for key, value := range row {
		if scenarioFieldIsActorRef(key) {
			if name, ok := value.(string); ok && name != "" {
				resolved[key] = scenarioActorIDByDisplayName(t, data, name)
				continue
			}
		}
		resolved[key] = value
	}
	return resolved
}

func scenarioFieldIsActorRef(key string) bool {
	return key == "actor" || strings.HasSuffix(key, "_actor")
}

func scenarioRowContains(row map[string]any, expected map[string]any) bool {
	for key, want := range expected {
		got, ok := row[key]
		if !ok || !assert.ObjectsAreEqualValues(want, got) {
			return false
		}
	}
	return true
}

func assertScenarioNoActorPortLinkType(t testing.TB, data topologyv1test.NormalizedData, linkType string) {
	t.Helper()

	require.NotNil(t, data.Tables)
	table, ok := data.Tables.Actor["actor_port_links"]
	require.True(t, ok)
	for _, row := range table.Table.Rows {
		require.NotEqual(t, linkType, row["type"])
	}
}

func assertScenarioNoVisibleLinks(t testing.TB, data topologyv1test.NormalizedData) {
	t.Helper()

	require.Empty(t, data.Links.Rows)
}

func assertScenarioStatEquals(t testing.TB, data topologyv1test.NormalizedData, stat string, want any) {
	t.Helper()

	stats, ok := data.Stats.(map[string]any)
	require.True(t, ok)
	require.EqualValues(t, want, stats[stat])
}

func scenarioActorIDByDisplayName(t testing.TB, data topologyv1test.NormalizedData, displayName string) string {
	t.Helper()

	row := scenarioActorByName(t, data, displayName)
	id, ok := row["id"].(string)
	require.Truef(t, ok && id != "", "actor %q has invalid id %v", displayName, row["id"])
	return id
}

func scenarioActorByName(t testing.TB, data topologyv1test.NormalizedData, name string) map[string]any {
	t.Helper()

	for _, row := range data.Actors.Rows {
		if row["display_name"] == name || row["sys_name"] == name {
			return row
		}
	}
	require.Failf(t, "missing scenario actor", "name=%s actors=%v", name, data.Actors.Rows)
	return nil
}
