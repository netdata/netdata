// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	topologyv1renderer "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyv1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/snmptopologyfunc"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopologyFunctionAdapter_MethodParamsUsesRegistryFocusTargets(t *testing.T) {
	registry := newTopologyRegistry()
	registry.register(newTestTopologyCacheLLDP(
		"agent-test",
		time.Now().UTC(),
		"00:11:22:33:44:55",
		"sw-a",
		"10.0.0.1",
		"Gi0/1",
		"aa:bb:cc:dd:ee:ff",
		"sw-b",
		"10.0.0.2",
		"Gi0/2",
	))

	handler := snmptopologyfunc.NewHandler(funcDepsAdapter{registry: registry})

	params, err := handler.MethodParams(context.Background(), snmptopologyfunc.MethodID)
	require.NoError(t, err)
	require.Len(t, params, 5)
	assert.Equal(t, snmptopologyfunc.ParamNodesIdentity, params[0].ID)
	assert.Equal(t, snmptopologyfunc.ParamMapType, params[1].ID)
	assert.Equal(t, snmptopologyfunc.ParamInferenceStrategy, params[2].ID)
	assert.Equal(t, snmptopologyfunc.ParamManagedDeviceFocus, params[3].ID)
	assert.Equal(t, snmptopologyfunc.ParamDepth, params[4].ID)
	require.GreaterOrEqual(t, len(params[3].Options), 2)
	assert.Equal(t, snmptopologyfunc.ManagedFocusAllDevices, params[3].Options[0].ID)
	assert.Equal(t, "ip:10.0.0.1", params[3].Options[1].ID)

	params, err = handler.MethodParams(context.Background(), "unknown")
	require.NoError(t, err)
	assert.Nil(t, params)
}

func TestTopologyFunctionAdapter_HandleDefaultStrictL2(t *testing.T) {
	registry := newTopologyRegistry()
	registry.register(newTestTopologyCacheLLDP(
		"agent-test",
		time.Now().UTC(),
		"00:11:22:33:44:55",
		"sw-a",
		"10.0.0.1",
		"Gi0/1",
		"aa:bb:cc:dd:ee:ff",
		"sw-b",
		"10.0.0.2",
		"Gi0/2",
	))

	handler := snmptopologyfunc.NewHandler(funcDepsAdapter{registry: registry})
	resp := handler.Handle(context.Background(), snmptopologyfunc.MethodID, nil)
	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.Status)
	assert.Equal(t, "topology", resp.ResponseType)

	data, ok := resp.Data.(topologyv1.Data)
	require.True(t, ok)
	require.NoError(t, validateTopologyV1Data(data))
	assert.Equal(t, topologyv1.SchemaVersion, data.SchemaVersion)
	assert.Equal(t, "snmp-l2", data.Producer.Source)
	require.NotNil(t, data.View)
	assert.Equal(t, "summary", data.View.ID)
	assert.Equal(t, "network", data.View.Scope)
	assert.Equal(t, "detailed", data.View.Mode)
	assert.Greater(t, data.Actors.Rows, 0)
	assert.Greater(t, data.Links.Rows, 0)
}

func TestTopologyFunctionAdapter_HandleSelectorParams(t *testing.T) {
	registry := newTopologyRegistry()
	registry.register(newTestTopologyCacheLLDP(
		"agent-test",
		time.Now().UTC(),
		"00:11:22:33:44:55",
		"sw-a",
		"10.0.0.1",
		"Gi0/1",
		"aa:bb:cc:dd:ee:ff",
		"sw-b",
		"10.0.0.2",
		"Gi0/2",
	))

	handler := snmptopologyfunc.NewHandler(funcDepsAdapter{registry: registry})
	cfg := snmptopologyfunc.Methods()[0].RequiredParams

	params := funcapi.ResolveParams(cfg, map[string][]string{
		snmptopologyfunc.ParamNodesIdentity:      {snmptopologyfunc.NodesIdentityMAC},
		snmptopologyfunc.ParamMapType:            {snmptopologyfunc.MapTypeHighConfidenceInferred},
		snmptopologyfunc.ParamInferenceStrategy:  {snmptopologyfunc.InferenceStrategySTPFDBCorrelated},
		snmptopologyfunc.ParamManagedDeviceFocus: {"ip:10.0.0.1"},
		snmptopologyfunc.ParamDepth:              {"2"},
	})
	resp := handler.Handle(context.Background(), snmptopologyfunc.MethodID, params)
	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.Status)
	data, ok := resp.Data.(topologyv1.Data)
	require.True(t, ok)
	require.NoError(t, validateTopologyV1Data(data))
	require.NotNil(t, data.View)
	assert.Equal(t, "network", data.View.Scope)
}

func TestTopologyFunctionAdapter_HandleUnknownSelectorsFallbackToDefaults(t *testing.T) {
	registry := newTopologyRegistry()
	registry.register(newTestTopologyCacheLLDP(
		"agent-test",
		time.Now().UTC(),
		"00:11:22:33:44:55",
		"sw-a",
		"10.0.0.1",
		"Gi0/1",
		"aa:bb:cc:dd:ee:ff",
		"sw-b",
		"10.0.0.2",
		"Gi0/2",
	))

	handler := snmptopologyfunc.NewHandler(funcDepsAdapter{registry: registry})
	cfg := snmptopologyfunc.Methods()[0].RequiredParams

	defaultResp := handler.Handle(context.Background(), snmptopologyfunc.MethodID, nil)
	require.NotNil(t, defaultResp)
	require.Equal(t, 200, defaultResp.Status)
	defaultData, ok := defaultResp.Data.(topologyv1.Data)
	require.True(t, ok)

	invalidParams := funcapi.ResolveParams(cfg, map[string][]string{
		snmptopologyfunc.ParamNodesIdentity:      {"unknown"},
		snmptopologyfunc.ParamMapType:            {"invalid"},
		snmptopologyfunc.ParamInferenceStrategy:  {"invalid"},
		snmptopologyfunc.ParamManagedDeviceFocus: {"invalid"},
		snmptopologyfunc.ParamDepth:              {"invalid"},
	})
	invalidResp := handler.Handle(context.Background(), snmptopologyfunc.MethodID, invalidParams)
	require.NotNil(t, invalidResp)
	require.Equal(t, 200, invalidResp.Status)
	invalidData, ok := invalidResp.Data.(topologyv1.Data)
	require.True(t, ok)

	require.NoError(t, validateTopologyV1Data(invalidData))
	require.NotNil(t, defaultData.View)
	require.NotNil(t, invalidData.View)
	assert.Equal(t, defaultData.View.Scope, invalidData.View.Scope)
	assert.Equal(t, defaultData.View.ID, invalidData.View.ID)
}

func TestSNMPTopologyToV1_BuildsTypedActorDetailTables(t *testing.T) {
	ts := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	data := topologyData{
		AgentID:     "agent-test",
		CollectedAt: ts,
		View:        "summary",
		Actors: []topologyActor{
			{
				ActorID:   "device-a",
				ActorType: "device",
				Match: topologyMatch{
					ChassisIDs:   []string{"00:11:22:33:44:55"},
					MacAddresses: []string{"00:11:22:33:44:55"},
					SysName:      "sw-a",
				},
				Labels: map[string]string{"site": "lab"},
				Detail: topologyActorDetail{
					L2: topologyengine.ProjectionActorDetail{
						Device: topologyengine.ProjectionDeviceActorDetail{
							PortsTotal: topologyengine.OptionalValue[int]{Value: 0, Has: true},
							Ports: []topologyengine.ProjectionPortDetail{
								{
									IfIndex:                topologyengine.OptionalValue[int]{Value: 1, Has: true},
									PortID:                 "1",
									Name:                   "Gi0/1",
									IfName:                 "Gi0/1",
									IfDescr:                "GigabitEthernet0/1",
									IfAlias:                "uplink to sw-b",
									MAC:                    "00:11:22:33:44:56",
									Speed:                  topologyengine.OptionalValue[int64]{Value: 1000000000, Has: true},
									VLANIDs:                []string{"10", "20"},
									NeighborCount:          topologyengine.OptionalValue[int]{Value: 0, Has: true},
									Duplex:                 "full",
									LinkModeConfidence:     "high",
									TopologyRoleConfidence: "medium",
									LinkModeSources:        []string{"lldp"},
									TopologyRoleSources:    []string{"bridge"},
									LastChange:             "2026-01-02T01:02:03Z",
									Neighbors: []topologyengine.ProjectionPortNeighbor{
										{
											Protocol:   "lldp",
											RemotePort: "Gi0/2",
										},
									},
									VLANs: []topologyengine.ProjectionPortVLAN{
										{
											VLANID:   "10",
											VLANName: "users",
										},
									},
								},
							},
						},
					},
				},
			},
			{
				ActorID:   "device-b",
				ActorType: "device",
				Match: topologyMatch{
					ChassisIDs:   []string{"aa:bb:cc:dd:ee:ff"},
					MacAddresses: []string{"aa:bb:cc:dd:ee:ff"},
					SysName:      "sw-b",
				},
				Detail: topologyActorDetail{
					L2: topologyengine.ProjectionActorDetail{
						Device: topologyengine.ProjectionDeviceActorDetail{
							CapabilitiesSupported: []string{"router"},
							CapabilitiesEnabled:   []string{"bridge"},
						},
						Endpoint: topologyengine.ProjectionEndpointActorDetail{
							LearnedSources: []string{"arp"},
						},
					},
				},
			},
		},
		Links: []topologyLink{
			{
				Protocol:   "lldp",
				LinkType:   "lldp",
				Direction:  "bidirectional",
				SrcActorID: "device-a",
				DstActorID: "device-b",
				Src: topologyLinkEndpoint{
					IfIndex: 1,
					IfName:  "Gi0/1",
					PortID:  "1",
				},
				Dst: topologyLinkEndpoint{
					IfIndex: 2,
					IfName:  "Gi0/2",
					PortID:  "2",
				},
				Inference: &graph.LinkInference{
					Confidence:     "high",
					Inference:      "observed",
					AttachmentMode: "lldp",
				},
			},
		},
	}

	payload, err := topologyv1renderer.Render(data)
	require.NoError(t, err)
	require.NoError(t, validateTopologyV1Data(payload))
	require.NotNil(t, payload.Tables)
	require.Contains(t, payload.Tables.Actor, "actor_ports")
	require.Contains(t, payload.Tables.Actor, "actor_port_links")
	require.Contains(t, payload.Tables.Actor, "actor_labels")
	require.Contains(t, payload.Tables.Actor, "actor_metadata")
	require.NotContains(t, payload.Tables.Actor, "actor_custom_labels")
	require.NotContains(t, payload.Tables.Actor, "actor_detail_custom_labels")
	require.NotContains(t, payload.Tables.Actor, "actor_custom_metadata")

	portTable := payload.Tables.Actor["actor_ports"].Table
	assert.Equal(t, 1, portTable.Rows)
	assert.Empty(t, topologyV1ColumnType(portTable, "port_number"))
	assert.Equal(t, "uint", topologyV1ColumnType(portTable, "if_index"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(portTable, "port_id"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(portTable, "if_name"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(portTable, "if_descr"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(portTable, "if_alias"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(portTable, "mac"))
	assert.Equal(t, "uint", topologyV1ColumnType(portTable, "speed"))
	assert.Equal(t, "actor_ref", topologyV1ColumnType(portTable, "neighbor_actor"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(portTable, "neighbor_port_name"))
	assert.Equal(t, "json", topologyV1ColumnType(portTable, "neighbors"))
	assert.Equal(t, "json", topologyV1ColumnType(portTable, "vlans"))
	assert.Empty(t, topologyV1ColumnType(portTable, "extra"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(portTable, "duplex"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(portTable, "link_mode_confidence"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(portTable, "topology_role_confidence"))
	assert.Equal(t, "array", topologyV1ColumnType(portTable, "link_mode_sources"))
	assert.Equal(t, "array", topologyV1ColumnType(portTable, "topology_role_sources"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(portTable, "last_change"))
	assert.Equal(t, []any{uint64(0), nil}, topologyV1ColumnValues(t, payload.Actors, "ports_total"))
	assert.Equal(t, []any{nil, []any{"arp"}}, topologyV1ColumnValues(t, payload.Actors, "protocols"))
	assert.Equal(t, []any{nil, nil}, topologyV1ColumnValues(t, payload.Actors, "capabilities"))
	assert.Equal(t, []any{uint64(1)}, topologyV1ColumnValues(t, portTable, "if_index"))
	assert.Equal(t, []string{"1"}, topologyV1StringColumnValues(t, payload, portTable, "port_id"))
	assert.Equal(t, []string{"Gi0/1"}, topologyV1StringColumnValues(t, payload, portTable, "if_name"))
	assert.Equal(t, []string{"GigabitEthernet0/1"}, topologyV1StringColumnValues(t, payload, portTable, "if_descr"))
	assert.Equal(t, []string{"uplink to sw-b"}, topologyV1StringColumnValues(t, payload, portTable, "if_alias"))
	assert.Equal(t, []string{"00:11:22:33:44:56"}, topologyV1StringColumnValues(t, payload, portTable, "mac"))
	assert.Equal(t, []any{uint64(1000000000)}, topologyV1ColumnValues(t, portTable, "speed"))
	assert.Equal(t, []any{[]any{"10", "20"}}, topologyV1ColumnValues(t, portTable, "vlan_ids"))
	assert.Equal(t, []any{uint64(0)}, topologyV1ColumnValues(t, portTable, "neighbor_count"))
	assert.Equal(t, []any{1}, topologyV1ColumnValues(t, portTable, "neighbor_actor"))
	assert.Equal(t, []string{"Gi0/2"}, topologyV1StringColumnValues(t, payload, portTable, "neighbor_port_name"))
	assert.Equal(t, []string{"full"}, topologyV1StringColumnValues(t, payload, portTable, "duplex"))
	assert.Equal(t, []string{"high"}, topologyV1StringColumnValues(t, payload, portTable, "link_mode_confidence"))
	assert.Equal(t, []string{"medium"}, topologyV1StringColumnValues(t, payload, portTable, "topology_role_confidence"))
	assert.Equal(t, []any{[]any{"lldp"}}, topologyV1ColumnValues(t, portTable, "link_mode_sources"))
	assert.Equal(t, []any{[]any{"bridge"}}, topologyV1ColumnValues(t, portTable, "topology_role_sources"))
	assert.Equal(t, []string{"2026-01-02T01:02:03Z"}, topologyV1StringColumnValues(t, payload, portTable, "last_change"))

	portLinksTable := payload.Tables.Actor["actor_port_links"].Table
	assert.Equal(t, 2, portLinksTable.Rows)
	assert.Equal(t, "actor_ref", topologyV1ColumnType(portLinksTable, "actor"))
	assert.Equal(t, "link_ref", topologyV1ColumnType(portLinksTable, "link"))
	assert.Equal(t, "actor_ref", topologyV1ColumnType(portLinksTable, "remote_actor"))
	assert.Equal(t, []any{0, 1}, topologyV1ColumnValues(t, portLinksTable, "actor"))
	assert.Equal(t, []any{0, 0}, topologyV1ColumnValues(t, portLinksTable, "link"))
	assert.Equal(t, []any{1, 0}, topologyV1ColumnValues(t, portLinksTable, "remote_actor"))
	assert.Equal(t, []any{uint64(1), uint64(2)}, topologyV1ColumnValues(t, portLinksTable, "if_index"))
	assert.Equal(t, []any{uint64(2), uint64(1)}, topologyV1ColumnValues(t, portLinksTable, "remote_if_index"))
	assert.Equal(t, []string{"Gi0/1", "Gi0/2"}, topologyV1StringColumnValues(t, payload, portLinksTable, "port_name"))
	assert.Equal(t, []string{"Gi0/2", "Gi0/1"}, topologyV1StringColumnValues(t, payload, portLinksTable, "remote_port_name"))
	assert.Equal(t, []string{"high", "high"}, topologyV1StringColumnValues(t, payload, portLinksTable, "confidence"))

	labelTable := payload.Tables.Actor["actor_labels"].Table
	assert.GreaterOrEqual(t, labelTable.Rows, 2)
	assert.Equal(t, "actor_ref", topologyV1ColumnType(labelTable, "actor"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(labelTable, "key"))
	assert.Equal(t, "attribute", topologyV1ColumnRole(labelTable, "key"))
	assert.Contains(t, topologyV1StringColumnValues(t, payload, labelTable, "key"), "capabilities_supported")
	assert.Contains(t, topologyV1StringColumnValues(t, payload, labelTable, "key"), "capabilities_enabled")
	assert.Contains(t, topologyV1StringColumnValues(t, payload, labelTable, "value"), "router")
	assert.Contains(t, topologyV1StringColumnValues(t, payload, labelTable, "value"), "bridge")

	require.Contains(t, payload.Evidence, "lldp")
	evidenceTable := payload.Evidence["lldp"].Table
	assert.Equal(t, "string_ref", topologyV1ColumnType(evidenceTable, "src_port_name"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(evidenceTable, "dst_port_name"))
	assert.Equal(t, "uint", topologyV1ColumnType(evidenceTable, "src_if_index"))
	assert.Equal(t, "uint", topologyV1ColumnType(evidenceTable, "dst_if_index"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(evidenceTable, "src_port_id"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(evidenceTable, "dst_port_id"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(evidenceTable, "src_management_ip"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(evidenceTable, "dst_management_ip"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(evidenceTable, "confidence"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(evidenceTable, "inference"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(evidenceTable, "attachment_mode"))
	assert.Empty(t, topologyV1ColumnType(evidenceTable, "src_endpoint"))
	assert.Empty(t, topologyV1ColumnType(evidenceTable, "dst_endpoint"))
	assert.Empty(t, topologyV1ColumnType(evidenceTable, "metrics"))
	assert.Equal(t, []any{uint64(1)}, topologyV1ColumnValues(t, evidenceTable, "src_if_index"))
	assert.Equal(t, []any{uint64(2)}, topologyV1ColumnValues(t, evidenceTable, "dst_if_index"))
	assert.Equal(t, []string{"1"}, topologyV1StringColumnValues(t, payload, evidenceTable, "src_port_id"))
	assert.Equal(t, []string{"2"}, topologyV1StringColumnValues(t, payload, evidenceTable, "dst_port_id"))

	deviceType := payload.Types.ActorTypes["device"]
	require.NotNil(t, deviceType.Presentation)
	require.NotNil(t, deviceType.Presentation.Modal)
	require.NotNil(t, deviceType.Presentation.Modal.Labels)
	require.NotNil(t, deviceType.Presentation.Modal.Labels.Identification)
	assert.Equal(t, "management_ip", deviceType.Presentation.Modal.Labels.Identification.Fields[1].Key)
	require.Len(t, deviceType.Presentation.Modal.Sections, 5)
	assert.Equal(t, "ports", deviceType.Presentation.Modal.Sections[0].ID)
	assert.Equal(t, "if_index", deviceType.Presentation.Modal.Sections[0].Columns[0].ID)
	assert.Equal(t, "Port ID", deviceType.Presentation.Modal.Sections[0].Columns[0].Label)
	assert.Equal(t, "neighbor_actor", deviceType.Presentation.Modal.Sections[0].Columns[11].ID)
	assert.Equal(t, "actor_link", deviceType.Presentation.Modal.Sections[0].Columns[11].Cell)
	assert.Equal(t, "expanded", deviceType.Presentation.Modal.Sections[0].Columns[11].Visibility)
	assert.Equal(t, "port_neighbors", deviceType.Presentation.Modal.Sections[1].ID)
	assert.Equal(t, "actor_table", deviceType.Presentation.Modal.Sections[1].Source.Kind)
	assert.Equal(t, "actor_port_links", deviceType.Presentation.Modal.Sections[1].Source.Table)
	assert.Equal(t, "l3_adjacencies", deviceType.Presentation.Modal.Sections[2].ID)
	assert.Equal(t, "evidence", deviceType.Presentation.Modal.Sections[2].Source.Kind)
	assert.Equal(t, topologymodel.L3SubnetLinkType, deviceType.Presentation.Modal.Sections[2].Source.Evidence)
	assert.Equal(t, "ospf_neighbors", deviceType.Presentation.Modal.Sections[3].ID)
	assert.Equal(t, "actor_table", deviceType.Presentation.Modal.Sections[3].Source.Kind)
	assert.Equal(t, "actor_ospf_neighbors", deviceType.Presentation.Modal.Sections[3].Source.Table)
	assert.Equal(t, "bgp_peers", deviceType.Presentation.Modal.Sections[4].ID)
	assert.Equal(t, "actor_table", deviceType.Presentation.Modal.Sections[4].Source.Kind)
	assert.Equal(t, "actor_bgp_peers", deviceType.Presentation.Modal.Sections[4].Source.Table)

	endpointType := payload.Types.ActorTypes["endpoint"]
	require.NotNil(t, endpointType.Presentation)
	require.NotNil(t, endpointType.Presentation.Modal)
	require.Len(t, endpointType.Presentation.Modal.Sections, 1)
	assert.Equal(t, "links", endpointType.Presentation.Modal.Sections[0].ID)
	assert.Equal(t, "selected_side_endpoint", endpointType.Presentation.Modal.Sections[0].Columns[1].Projection.Kind)
}

func TestSNMPTopologyToV1_PrefersSNMPActorDetailOverL2(t *testing.T) {
	data := topologyData{
		AgentID: "agent-test",
		View:    "summary",
		Actors: []topologyActor{
			{
				ActorID:   "device-a",
				ActorType: "device",
				Match: topologyMatch{
					ChassisIDs: []string{"00:11:22:33:44:55"},
					SysName:    "sw-a",
				},
				Detail: topologyActorDetail{
					L2: topologyengine.ProjectionActorDetail{
						Device: topologyengine.ProjectionDeviceActorDetail{
							ManagementIP: "10.0.0.1",
							Vendor:       "L2 Vendor",
							Capabilities: []string{"bridge"},
							PortsTotal:   topologyengine.OptionalValue[int]{Value: 24, Has: true},
						},
					},
					SNMP: topologySNMPActorDetail{
						ManagementIP: "10.0.0.2",
						Vendor:       "SNMP Vendor",
						Capabilities: []string{"router"},
					},
				},
			},
			{
				ActorID:   "device-b",
				ActorType: "device",
				Match: topologyMatch{
					ChassisIDs: []string{"aa:bb:cc:dd:ee:ff"},
					SysName:    "sw-b",
				},
				Detail: topologyActorDetail{
					SNMP: topologySNMPActorDetail{
						ManagementIP: "10.0.0.3",
						Vendor:       "Peer Vendor",
						Capabilities: []string{"bridge"},
					},
				},
			},
		},
		Links: []topologyLink{
			{
				Protocol:   "lldp",
				LinkType:   "lldp",
				SrcActorID: "device-a",
				DstActorID: "device-b",
			},
		},
	}

	payload, err := topologyv1renderer.Render(data)
	require.NoError(t, err)
	require.NoError(t, validateTopologyV1Data(payload))

	assert.Equal(t, "SNMP Vendor", topologyV1StringColumnValues(t, payload, payload.Actors, "vendor")[0])
	assert.Equal(t, "10.0.0.2", topologyV1StringColumnValues(t, payload, payload.Actors, "management_ip")[0])
	assert.Equal(t, []any{"router"}, topologyV1ColumnValues(t, payload.Actors, "capabilities")[0])
	assert.Equal(t, uint64(24), topologyV1ColumnValues(t, payload.Actors, "ports_total")[0])
}

func TestSNMPTopologyToV1_OmitsNeighborCountForEmptyNeighborList(t *testing.T) {
	data := topologyData{
		AgentID: "agent-test",
		View:    "summary",
		Actors: []topologyActor{
			{
				ActorID:   "device-a",
				ActorType: "device",
				Match: topologyMatch{
					SysName: "sw-a",
				},
				Detail: topologyActorDetail{
					L2: topologyengine.ProjectionActorDetail{
						Device: topologyengine.ProjectionDeviceActorDetail{
							Ports: []topologyengine.ProjectionPortDetail{
								{
									IfIndex:   topologyengine.OptionalValue[int]{Value: 42, Has: true},
									IfName:    "Gi0/42",
									Neighbors: []topologyengine.ProjectionPortNeighbor{},
								},
							},
						},
					},
				},
			},
			{
				ActorID:   "device-b",
				ActorType: "device",
				Match: topologyMatch{
					SysName: "sw-b",
				},
			},
		},
		Links: []topologyLink{
			{
				Protocol:   "lldp",
				LinkType:   "lldp",
				SrcActorID: "device-a",
				DstActorID: "device-b",
				Src: topologyLinkEndpoint{
					IfIndex: 42,
					IfName:  "Gi0/42",
				},
				Dst: topologyLinkEndpoint{
					IfName: "Gi0/1",
				},
			},
		},
	}

	payload, err := topologyv1renderer.Render(data)
	require.NoError(t, err)
	require.NoError(t, validateTopologyV1Data(payload))
	require.NotNil(t, payload.Tables)

	portTable := payload.Tables.Actor["actor_ports"].Table
	assert.Equal(t, []any{nil}, topologyV1ColumnValues(t, portTable, "neighbor_count"))
	assert.Equal(t, []any{nil}, topologyV1ColumnValues(t, portTable, "neighbors"))
}

func TestSNMPTopologyToV1_UsesIfIndexAsVisiblePortID(t *testing.T) {
	data := topologyData{
		AgentID: "agent-test",
		View:    "summary",
		Actors: []topologyActor{
			{
				ActorID:   "device-a",
				ActorType: "device",
				Match: topologyMatch{
					SysName: "sw-a",
				},
				Detail: topologyActorDetail{
					L2: topologyengine.ProjectionActorDetail{
						Device: topologyengine.ProjectionDeviceActorDetail{
							Ports: []topologyengine.ProjectionPortDetail{
								{
									IfIndex: topologyengine.OptionalValue[int]{Value: 42, Has: true},
									IfName:  "Gi0/42",
								},
							},
						},
					},
				},
			},
			{
				ActorID:   "device-b",
				ActorType: "device",
				Match: topologyMatch{
					SysName: "sw-b",
				},
			},
		},
		Links: []topologyLink{
			{
				Protocol:   "lldp",
				LinkType:   "lldp",
				SrcActorID: "device-a",
				DstActorID: "device-b",
				Src: topologyLinkEndpoint{
					IfIndex: 42,
					IfName:  "Gi0/42",
				},
				Dst: topologyLinkEndpoint{
					IfName: "Gi0/1",
				},
			},
		},
	}

	payload, err := topologyv1renderer.Render(data)
	require.NoError(t, err)
	require.NoError(t, validateTopologyV1Data(payload))
	require.NotNil(t, payload.Tables)

	portTable := payload.Tables.Actor["actor_ports"].Table
	assert.Empty(t, topologyV1ColumnType(portTable, "port_number"))
	assert.Equal(t, []any{uint64(42)}, topologyV1ColumnValues(t, portTable, "if_index"))

	deviceType := payload.Types.ActorTypes["device"]
	require.NotNil(t, deviceType.Presentation)
	require.NotNil(t, deviceType.Presentation.Modal)
	require.NotEmpty(t, deviceType.Presentation.Modal.Sections)
	assert.Equal(t, "if_index", deviceType.Presentation.Modal.Sections[0].Columns[0].ID)
	assert.Equal(t, "Port ID", deviceType.Presentation.Modal.Sections[0].Columns[0].Label)
}

func TestSNMPTopologyToV1_PortNamesOnlyUsePortFields(t *testing.T) {
	data := topologyData{
		AgentID: "agent-test",
		View:    "summary",
		Actors: []topologyActor{
			{
				ActorID:   "device-a",
				ActorType: "device",
				Match: topologyMatch{
					SysName: "sw-a",
				},
			},
			{
				ActorID:   "device-b",
				ActorType: "device",
				Match: topologyMatch{
					SysName: "sw-b",
				},
			},
		},
		Links: []topologyLink{
			{
				Protocol:   "lldp",
				LinkType:   "lldp",
				SrcActorID: "device-a",
				DstActorID: "device-b",
				Src: topologyLinkEndpoint{
					DisplayName: "10.0.0.10",
				},
				Dst: topologyLinkEndpoint{
					SysName: "sw-b",
				},
			},
		},
	}

	payload, err := topologyv1renderer.Render(data)
	require.NoError(t, err)
	require.NoError(t, validateTopologyV1Data(payload))

	assert.Equal(t, []any{nil}, topologyV1ColumnValues(t, payload.Links, "src_port_name"))
	assert.Equal(t, []any{nil}, topologyV1ColumnValues(t, payload.Links, "dst_port_name"))

	require.NotNil(t, payload.Tables)
	portLinksTable := payload.Tables.Actor["actor_port_links"].Table
	assert.Equal(t, []any{nil, nil}, topologyV1ColumnValues(t, portLinksTable, "port_name"))
	assert.Equal(t, []any{nil, nil}, topologyV1ColumnValues(t, portLinksTable, "remote_port_name"))
}

func TestSNMPTopologyToV1_PreservesL3SubnetPresentationAndEvidence(t *testing.T) {
	data := topologyData{
		AgentID: "agent-test",
		View:    "summary",
		Actors: []topologyActor{
			{
				ActorID:   "router-a",
				ActorType: "router",
				Match: topologyMatch{
					IPAddresses: []string{"192.0.2.1"},
					SysName:     "router-a",
				},
			},
			{
				ActorID:   "router-b",
				ActorType: "router",
				Match: topologyMatch{
					IPAddresses: []string{"192.0.2.2"},
					SysName:     "router-b",
				},
			},
		},
		Links: []topologyLink{
			{
				Protocol:   topologyL3SubnetLinkType,
				LinkType:   topologyL3SubnetLinkType,
				Direction:  "observed",
				SrcActorID: "router-a",
				DstActorID: "router-b",
				Src: topologyLinkEndpoint{
					IfIndex: 10,
					IfName:  "xe-0/0/0",
				},
				Dst: topologyLinkEndpoint{
					IfIndex: 20,
					IfName:  "xe-0/0/1",
				},
				Inference: &graph.LinkInference{
					Inference:      "shared_subnet",
					AttachmentMode: "logical_l3_subnet",
				},
				Detail: topologyLinkDetail{
					L3Subnet: &topologyL3SubnetLinkDetail{
						Source:  "ip_mib",
						SrcIP:   "192.0.2.1",
						DstIP:   "192.0.2.2",
						Subnet:  "192.0.2.0/30",
						Network: "192.0.2.0",
						Netmask: "255.255.255.252",
						Prefix:  30,
					},
				},
			},
		},
	}

	payload, err := topologyv1renderer.Render(data)
	require.NoError(t, err)
	require.NoError(t, validateTopologyV1Data(payload))

	assert.Contains(t, payload.Producer.Capabilities, "l3_subnet")
	require.NotNil(t, payload.Presentation)
	assert.Equal(t, "snmp-l2.v2", payload.Presentation.ProfileVersion)
	assert.Contains(t, topologyV1LegendLinkTypes(payload), topologymodel.L3SubnetLinkType)

	require.Contains(t, payload.Types.LinkTypes, topologymodel.L3SubnetLinkType)
	linkType := payload.Types.LinkTypes[topologymodel.L3SubnetLinkType]
	assert.Equal(t, "observed_bidirectional", linkType.Orientation)
	assert.Equal(t, "observation", linkType.DirectionRole)
	assert.Equal(t, "normal", linkType.SemanticRole)
	require.NotNil(t, linkType.Presentation)
	assert.Equal(t, "L3 subnet", linkType.Presentation.Label)
	assert.Equal(t, "info", linkType.Presentation.ColorSlot)
	assert.Equal(t, "dashed", linkType.Presentation.LineStyle)
	assert.Equal(t, "normal", linkType.Presentation.Width)

	require.Contains(t, payload.Types.EvidenceTypes, topologymodel.L3SubnetLinkType)
	evidenceType := payload.Types.EvidenceTypes[topologymodel.L3SubnetLinkType]
	assert.Equal(t, topologymodel.L3SubnetLinkType, evidenceType.LinkType)
	assert.Equal(t, []string{"src_actor", "dst_actor", "subnet", "src_ip", "dst_ip"}, evidenceType.MatchColumns)

	require.Contains(t, payload.Evidence, topologymodel.L3SubnetLinkType)
	evidenceTable := payload.Evidence[topologymodel.L3SubnetLinkType].Table
	assert.Equal(t, 1, evidenceTable.Rows)
	assert.Equal(t, "link_ref", topologyV1ColumnType(evidenceTable, "link"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(evidenceTable, "src_ip"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(evidenceTable, "dst_ip"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(evidenceTable, "subnet"))
	assert.Equal(t, "uint", topologyV1ColumnType(evidenceTable, "prefix"))
	assert.Equal(t, []any{0}, topologyV1ColumnValues(t, evidenceTable, "link"))
	assert.Equal(t, []string{"192.0.2.1"}, topologyV1StringColumnValues(t, payload, evidenceTable, "src_ip"))
	assert.Equal(t, []string{"192.0.2.2"}, topologyV1StringColumnValues(t, payload, evidenceTable, "dst_ip"))
	assert.Equal(t, []string{"192.0.2.0/30"}, topologyV1StringColumnValues(t, payload, evidenceTable, "subnet"))
	assert.Equal(t, []any{uint64(30)}, topologyV1ColumnValues(t, evidenceTable, "prefix"))
	assert.Equal(t, []string{"ip_mib"}, topologyV1StringColumnValues(t, payload, evidenceTable, "source"))

	assert.Equal(t, []string{topologymodel.L3SubnetLinkType}, topologyV1StringColumnValues(t, payload, payload.Links, "type"))
	assert.Equal(t, []string{topologymodel.L3SubnetLinkType}, topologyV1StringColumnValues(t, payload, payload.Links, "protocol"))

	deviceType := payload.Types.ActorTypes["router"]
	require.NotNil(t, deviceType.Presentation)
	require.NotNil(t, deviceType.Presentation.Modal)
	l3Section := requireTopologyV1ModalSection(t, deviceType.Presentation.Modal.Sections, "l3_adjacencies")
	assert.Equal(t, "evidence", l3Section.Source.Kind)
	assert.Equal(t, topologymodel.L3SubnetLinkType, l3Section.Source.Evidence)
	require.NotNil(t, l3Section.OwnerFilter)
	assert.Equal(t, "incident_evidence", l3Section.OwnerFilter.Mode)
	assert.Equal(t, "link", l3Section.OwnerFilter.LinkColumn)
	assert.Equal(t, "src_actor", l3Section.OwnerFilter.SrcActorColumn)
	assert.Equal(t, "dst_actor", l3Section.OwnerFilter.DstActorColumn)
	assert.Equal(t, "subnet", l3Section.Sort.Column)
	assert.Equal(t, "selected_side_endpoint", l3Section.Columns[1].Projection.Kind)
	assert.Equal(t, "endpoint", l3Section.Columns[1].Cell)

	if payload.Tables != nil {
		assert.NotContains(t, payload.Tables.Actor, "actor_port_links")
	}
}

func TestSNMPTopologyToV1_PreservesOSPFAdjacencyPresentationEvidenceAndNeighborRows(t *testing.T) {
	data := topologyData{
		AgentID: "agent-test",
		View:    "summary",
		Actors: []topologyActor{
			{
				ActorID:   "router-a",
				ActorType: "router",
				Source:    "snmp",
				Detail: topologyActorDetail{
					OSPF: []topologyOSPFNeighborDetailRow{
						{
							LocalRouterID:    "1.1.1.1",
							NeighborRouterID: "2.2.2.2",
							NeighborIP:       "192.0.2.2",
							State:            "full",
							LocalIP:          "192.0.2.1",
							Subnet:           "192.0.2.0/30",
							RemoteActorID:    "router-b",
						},
					},
				},
			},
			{
				ActorID:   "router-b",
				ActorType: "router",
				Source:    "snmp",
			},
		},
		Links: []topologyLink{
			{
				Protocol:   topologyOSPFAdjacencyLinkType,
				LinkType:   topologyOSPFAdjacencyLinkType,
				Direction:  "observed",
				State:      "full",
				SrcActorID: "router-a",
				DstActorID: "router-b",
				Src:        topologyLinkEndpoint{},
				Dst:        topologyLinkEndpoint{},
				Inference: &graph.LinkInference{
					Inference:      "ospf_full_adjacency",
					AttachmentMode: "logical_l3_ospf",
				},
				Detail: topologyLinkDetail{
					OSPF: &topologyOSPFAdjacencyLinkDetail{
						Source:           "ospf_mib",
						LocalRouterID:    "1.1.1.1",
						NeighborRouterID: "2.2.2.2",
						LocalIP:          "192.0.2.1",
						NeighborIP:       "192.0.2.2",
						Subnet:           "192.0.2.0/30",
						Network:          "192.0.2.0",
						Netmask:          "255.255.255.252",
						Prefix:           30,
					},
				},
			},
		},
	}

	payload, err := topologyv1renderer.Render(data)
	require.NoError(t, err)
	require.NoError(t, validateTopologyV1Data(payload))

	assert.Contains(t, payload.Producer.Capabilities, "ospf")
	require.Contains(t, payload.Types.LinkTypes, topologymodel.OSPFAdjacencyLinkType)
	linkType := payload.Types.LinkTypes[topologymodel.OSPFAdjacencyLinkType]
	assert.Equal(t, "observed_bidirectional", linkType.Orientation)
	assert.Equal(t, "observation", linkType.DirectionRole)
	assert.Equal(t, "control", linkType.SemanticRole)
	require.NotNil(t, linkType.Presentation)
	assert.Equal(t, "OSPF adjacency", linkType.Presentation.Label)
	assert.Equal(t, "purple", linkType.Presentation.ColorSlot)
	assert.Equal(t, "dashed", linkType.Presentation.LineStyle)
	assert.Contains(t, topologyV1LegendLinkTypes(payload), topologymodel.OSPFAdjacencyLinkType)

	require.Contains(t, payload.Types.EvidenceTypes, topologymodel.OSPFAdjacencyLinkType)
	evidenceType := payload.Types.EvidenceTypes[topologymodel.OSPFAdjacencyLinkType]
	assert.Equal(t, topologymodel.OSPFAdjacencyLinkType, evidenceType.LinkType)
	assert.Equal(t, []string{"src_actor", "dst_actor", "src_router_id", "dst_router_id", "src_ip", "dst_ip"}, evidenceType.MatchColumns)

	require.Contains(t, payload.Evidence, topologymodel.OSPFAdjacencyLinkType)
	evidenceTable := payload.Evidence[topologymodel.OSPFAdjacencyLinkType].Table
	assert.Equal(t, 1, evidenceTable.Rows)
	assert.Equal(t, "string_ref", topologyV1ColumnType(evidenceTable, "src_router_id"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(evidenceTable, "dst_router_id"))
	assert.Equal(t, []string{"1.1.1.1"}, topologyV1StringColumnValues(t, payload, evidenceTable, "src_router_id"))
	assert.Equal(t, []string{"2.2.2.2"}, topologyV1StringColumnValues(t, payload, evidenceTable, "dst_router_id"))
	assert.Equal(t, []string{"192.0.2.1"}, topologyV1StringColumnValues(t, payload, evidenceTable, "src_ip"))
	assert.Equal(t, []string{"192.0.2.2"}, topologyV1StringColumnValues(t, payload, evidenceTable, "dst_ip"))
	assert.Equal(t, []string{"192.0.2.0/30"}, topologyV1StringColumnValues(t, payload, evidenceTable, "subnet"))

	require.NotNil(t, payload.Tables)
	assert.NotContains(t, payload.Tables.Actor, "actor_port_links")
	require.Contains(t, payload.Tables.Actor, "actor_ospf_neighbors")
	neighborsTable := payload.Tables.Actor["actor_ospf_neighbors"].Table
	assert.Equal(t, 1, neighborsTable.Rows)
	assert.Equal(t, "actor_ref", topologyV1ColumnType(neighborsTable, "remote_actor"))
	assert.Equal(t, []any{0}, topologyV1ColumnValues(t, neighborsTable, "actor"))
	assert.Equal(t, []any{1}, topologyV1ColumnValues(t, neighborsTable, "remote_actor"))
	assert.Equal(t, []string{"1.1.1.1"}, topologyV1StringColumnValues(t, payload, neighborsTable, "local_router_id"))
	assert.Equal(t, []string{"2.2.2.2"}, topologyV1StringColumnValues(t, payload, neighborsTable, "neighbor_router_id"))
	assert.Equal(t, []string{"full"}, topologyV1StringColumnValues(t, payload, neighborsTable, "state"))

	deviceType := payload.Types.ActorTypes["router"]
	require.NotNil(t, deviceType.Presentation)
	require.NotNil(t, deviceType.Presentation.Modal)
	ospfSection := requireTopologyV1ModalSection(t, deviceType.Presentation.Modal.Sections, "ospf_neighbors")
	assert.Equal(t, "actor_table", ospfSection.Source.Kind)
	assert.Equal(t, "actor_ospf_neighbors", ospfSection.Source.Table)
	require.NotNil(t, ospfSection.OwnerFilter)
	assert.Equal(t, "actor_column", ospfSection.OwnerFilter.Mode)
	assert.Equal(t, "actor", ospfSection.OwnerFilter.ActorColumn)
	require.Len(t, ospfSection.Columns, 9)
	assert.Equal(t, "remote_actor", ospfSection.Columns[3].ID)
	assert.Equal(t, "actor_link", ospfSection.Columns[3].Cell)
	assert.Empty(t, ospfSection.Columns[3].Visibility)
}

func TestSNMPTopologyToV1_PreservesBGPAdjacencyPresentationEvidenceAndPeerRows(t *testing.T) {
	data := topologyData{
		AgentID: "agent-test",
		View:    "summary",
		Actors: []topologyActor{
			{
				ActorID:   "router-a",
				ActorType: "router",
				Source:    "snmp",
				Detail: topologyActorDetail{
					BGP: []topologyBGPPeerDetailRow{
						{
							RoutingInstance:       "default",
							NeighborIP:            "192.0.2.2",
							RemoteAS:              "65002",
							State:                 "established",
							AdminStatus:           "enabled",
							LocalIP:               "192.0.2.1",
							LocalAS:               "65001",
							LocalIdentifier:       "1.1.1.1",
							PeerIdentifier:        "2.2.2.2",
							PeerType:              "external",
							BGPVersion:            "4",
							Description:           "edge-peer",
							EstablishedUptime:     new(int64(300)),
							LastReceivedUpdateAge: new(int64(12)),
							RemoteActorID:         "router-b",
						},
					},
				},
			},
			{
				ActorID:   "router-b",
				ActorType: "router",
				Source:    "snmp",
			},
		},
		Links: []topologyLink{
			{
				Protocol:   topologyBGPAdjacencyLinkType,
				LinkType:   topologyBGPAdjacencyLinkType,
				Direction:  "observed",
				State:      "established",
				SrcActorID: "router-a",
				DstActorID: "router-b",
				Src:        topologyLinkEndpoint{},
				Dst:        topologyLinkEndpoint{},
				Inference: &graph.LinkInference{
					Inference:      "bgp_established_adjacency",
					AttachmentMode: "logical_l3_bgp",
				},
				Detail: topologyLinkDetail{
					BGP: &topologyBGPAdjacencyLinkDetail{
						Source:          "bgp_mib",
						RoutingInstance: "default",
						LocalIdentifier: "1.1.1.1",
						PeerIdentifier:  "2.2.2.2",
						LocalIP:         "192.0.2.1",
						NeighborIP:      "192.0.2.2",
						LocalAS:         "65001",
						RemoteAS:        "65002",
					},
				},
			},
		},
	}

	payload, err := topologyv1renderer.Render(data)
	require.NoError(t, err)
	require.NoError(t, validateTopologyV1Data(payload))

	assert.Contains(t, payload.Producer.Capabilities, "bgp")
	require.Contains(t, payload.Types.LinkTypes, topologymodel.BGPAdjacencyLinkType)
	linkType := payload.Types.LinkTypes[topologymodel.BGPAdjacencyLinkType]
	assert.Equal(t, "observed_bidirectional", linkType.Orientation)
	assert.Equal(t, "observation", linkType.DirectionRole)
	assert.Equal(t, "control", linkType.SemanticRole)
	require.NotNil(t, linkType.Presentation)
	assert.Equal(t, "BGP adjacency", linkType.Presentation.Label)
	assert.Equal(t, "accent", linkType.Presentation.ColorSlot)
	assert.Equal(t, "dashed", linkType.Presentation.LineStyle)
	assert.Contains(t, topologyV1LegendLinkTypes(payload), topologymodel.BGPAdjacencyLinkType)

	require.Contains(t, payload.Types.EvidenceTypes, topologymodel.BGPAdjacencyLinkType)
	evidenceType := payload.Types.EvidenceTypes[topologymodel.BGPAdjacencyLinkType]
	assert.Equal(t, topologymodel.BGPAdjacencyLinkType, evidenceType.LinkType)
	assert.Equal(t, []string{"src_actor", "dst_actor", "routing_instance"}, evidenceType.MatchColumns)

	require.Contains(t, payload.Evidence, topologymodel.BGPAdjacencyLinkType)
	evidenceTable := payload.Evidence[topologymodel.BGPAdjacencyLinkType].Table
	assert.Equal(t, 1, evidenceTable.Rows)
	assert.Equal(t, "string_ref", topologyV1ColumnType(evidenceTable, "routing_instance"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(evidenceTable, "local_identifier"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(evidenceTable, "peer_identifier"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(evidenceTable, "local_ip"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(evidenceTable, "neighbor_ip"))
	assert.Equal(t, []string{"default"}, topologyV1StringColumnValues(t, payload, evidenceTable, "routing_instance"))
	assert.Equal(t, []string{"1.1.1.1"}, topologyV1StringColumnValues(t, payload, evidenceTable, "local_identifier"))
	assert.Equal(t, []string{"2.2.2.2"}, topologyV1StringColumnValues(t, payload, evidenceTable, "peer_identifier"))
	assert.Equal(t, []string{"192.0.2.1"}, topologyV1StringColumnValues(t, payload, evidenceTable, "local_ip"))
	assert.Equal(t, []string{"192.0.2.2"}, topologyV1StringColumnValues(t, payload, evidenceTable, "neighbor_ip"))
	assert.Equal(t, []string{"65001"}, topologyV1StringColumnValues(t, payload, evidenceTable, "local_as"))
	assert.Equal(t, []string{"65002"}, topologyV1StringColumnValues(t, payload, evidenceTable, "remote_as"))

	require.NotNil(t, payload.Tables)
	require.Contains(t, payload.Tables.Actor, "actor_bgp_peers")
	peersTable := payload.Tables.Actor["actor_bgp_peers"].Table
	assert.Equal(t, 1, peersTable.Rows)
	assert.Equal(t, "actor_ref", topologyV1ColumnType(peersTable, "remote_actor"))
	assert.Equal(t, []any{0}, topologyV1ColumnValues(t, peersTable, "actor"))
	assert.Equal(t, []any{1}, topologyV1ColumnValues(t, peersTable, "remote_actor"))
	assert.Equal(t, []string{"default"}, topologyV1StringColumnValues(t, payload, peersTable, "routing_instance"))
	assert.Equal(t, []string{"192.0.2.2"}, topologyV1StringColumnValues(t, payload, peersTable, "neighbor_ip"))
	assert.Equal(t, []string{"65002"}, topologyV1StringColumnValues(t, payload, peersTable, "remote_as"))
	assert.Equal(t, []string{"established"}, topologyV1StringColumnValues(t, payload, peersTable, "state"))
	assert.Equal(t, []any{uint64(300)}, topologyV1ColumnValues(t, peersTable, "established_uptime"))

	deviceType := payload.Types.ActorTypes["router"]
	require.NotNil(t, deviceType.Presentation)
	require.NotNil(t, deviceType.Presentation.Modal)
	bgpSection := requireTopologyV1ModalSection(t, deviceType.Presentation.Modal.Sections, "bgp_peers")
	assert.Equal(t, "actor_table", bgpSection.Source.Kind)
	assert.Equal(t, "actor_bgp_peers", bgpSection.Source.Table)
	require.NotNil(t, bgpSection.OwnerFilter)
	assert.Equal(t, "actor_column", bgpSection.OwnerFilter.Mode)
	assert.Equal(t, "actor", bgpSection.OwnerFilter.ActorColumn)
	require.Len(t, bgpSection.Columns, 16)
	assert.Equal(t, "remote_actor", bgpSection.Columns[3].ID)
	assert.Equal(t, "actor_link", bgpSection.Columns[3].Cell)
	assert.Empty(t, bgpSection.Columns[3].Visibility)
}

func TestSNMPTopologyToV1_ReturnsErrorForL3SubnetWithoutSubnet(t *testing.T) {
	data := topologyData{
		AgentID: "agent-test",
		Actors: []topologyActor{
			{ActorID: "router-a", ActorType: "router"},
			{ActorID: "router-b", ActorType: "router"},
		},
		Links: []topologyLink{
			{
				Protocol:   topologyL3SubnetLinkType,
				LinkType:   topologyL3SubnetLinkType,
				SrcActorID: "router-a",
				DstActorID: "router-b",
				Detail: topologyLinkDetail{
					L3Subnet: &topologyL3SubnetLinkDetail{
						Prefix: 30,
					},
				},
			},
		},
	}

	_, err := topologyv1renderer.Render(data)

	require.Error(t, err)
	require.Contains(t, err.Error(), "l3_subnet link 0 is missing subnet")
}

func TestSNMPTopologyToV1_PreservesLinkPresentationTypes(t *testing.T) {
	ts := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	data := topologyData{
		AgentID:     "agent-test",
		CollectedAt: ts,
		View:        "summary",
		Actors: []topologyActor{
			{
				ActorID:   "device-a",
				ActorType: "device",
				Match: topologyMatch{
					ChassisIDs:   []string{"00:11:22:33:44:55"},
					MacAddresses: []string{"00:11:22:33:44:55"},
					SysName:      "sw-a",
				},
			},
			{
				ActorID:   "device-b",
				ActorType: "device",
				Match: topologyMatch{
					ChassisIDs:   []string{"aa:bb:cc:dd:ee:ff"},
					MacAddresses: []string{"aa:bb:cc:dd:ee:ff"},
					SysName:      "sw-b",
				},
			},
		},
		Links: []topologyLink{
			{
				Protocol:   "lldp",
				LinkType:   "lldp",
				Direction:  "bidirectional",
				SrcActorID: "device-a",
				DstActorID: "device-b",
			},
			{
				Protocol:   "bridge",
				LinkType:   "segment",
				Direction:  "bidirectional",
				State:      "probable",
				SrcActorID: "device-a",
				DstActorID: "device-b",
				Inference: &graph.LinkInference{
					AttachmentMode: "probable_bridge_anchor",
					Inference:      "probable",
				},
			},
		},
	}

	payload, err := topologyv1renderer.Render(data)
	require.NoError(t, err)
	require.NoError(t, validateTopologyV1Data(payload))

	assert.Equal(t, []string{"lldp", "probable"}, topologyV1StringColumnValues(t, payload, payload.Links, "type"))

	require.Contains(t, payload.Types.LinkTypes, "lldp")
	require.NotNil(t, payload.Types.LinkTypes["lldp"].Presentation)
	assert.Equal(t, "accent", payload.Types.LinkTypes["lldp"].Presentation.ColorSlot)
	assert.Equal(t, "thick", payload.Types.LinkTypes["lldp"].Presentation.Width)

	require.Contains(t, payload.Types.LinkTypes, "probable")
	require.NotNil(t, payload.Types.LinkTypes["probable"].Presentation)
	assert.Equal(t, "dim", payload.Types.LinkTypes["probable"].Presentation.ColorSlot)
	assert.Equal(t, "normal", payload.Types.LinkTypes["probable"].Presentation.Width)

	require.Contains(t, payload.Types.EvidenceTypes, "lldp")
	assert.Equal(t, "lldp", payload.Types.EvidenceTypes["lldp"].LinkType)
	require.Contains(t, payload.Types.EvidenceTypes, "probable")
	assert.Equal(t, "probable", payload.Types.EvidenceTypes["probable"].LinkType)
	require.Contains(t, payload.Evidence, "lldp")
	assert.Equal(t, 1, payload.Evidence["lldp"].Table.Rows)
	require.Contains(t, payload.Evidence, "probable")
	assert.Equal(t, 1, payload.Evidence["probable"].Table.Rows)

	assert.Contains(t, topologyV1LegendLinkTypes(payload), "lldp")
	assert.Contains(t, topologyV1LegendLinkTypes(payload), "probable")
}

func TestSNMPTopologyV1EvidenceMatchColumnsUseTypedL2EndpointFields(t *testing.T) {
	want := []string{
		"src_actor",
		"dst_actor",
		"protocol",
		"src_if_index",
		"src_port_name",
		"src_port_id",
		"dst_if_index",
		"dst_port_name",
		"dst_port_id",
	}
	tests := map[string]string{
		"lldp":     "lldp",
		"cdp":      "cdp",
		"bridge":   "bridge",
		"fdb":      "fdb",
		"stp":      "stp",
		"arp":      "arp",
		"snmp":     "snmp",
		"probable": "probable",
	}
	payload, err := topologyv1renderer.Render(topologyData{})
	require.NoError(t, err)

	for name, linkType := range tests {
		t.Run(name, func(t *testing.T) {
			evidenceType, ok := payload.Types.EvidenceTypes[linkType]
			require.True(t, ok)
			got := evidenceType.MatchColumns

			require.Equal(t, want, got)
			require.NotContains(t, got, "src_endpoint")
			require.NotContains(t, got, "dst_endpoint")
			require.NotContains(t, got, "metrics")
		})
	}
}

func TestSNMPTopologyToV1_PortlessFDBEvidenceUsesLinkRef(t *testing.T) {
	data := topologyData{
		AgentID:     "agent-test",
		CollectedAt: time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC),
		View:        "summary",
		Actors: []topologyActor{
			{
				ActorID:   "device-a",
				ActorType: "device",
				Source:    "snmp",
				Match:     topologyMatch{SysName: "switch-a"},
			},
			{
				ActorID:   "endpoint-a",
				ActorType: "endpoint",
				Source:    "fdb",
				Match:     topologyMatch{MacAddresses: []string{"00:11:22:33:44:55"}},
			},
		},
		Links: []topologyLink{
			{
				Protocol:   "fdb",
				LinkType:   "fdb",
				Direction:  "observed",
				SrcActorID: "device-a",
				DstActorID: "endpoint-a",
			},
		},
	}

	payload, err := topologyv1renderer.Render(data)
	require.NoError(t, err)
	require.NoError(t, validateTopologyV1Data(payload))
	require.Contains(t, payload.Evidence, "fdb")

	evidenceTable := payload.Evidence["fdb"].Table
	require.Equal(t, 1, evidenceTable.Rows)
	assert.Equal(t, []any{0}, topologyV1ColumnValues(t, evidenceTable, "link"))
	assert.Equal(t, []any{nil}, topologyV1ColumnValues(t, evidenceTable, "src_if_index"))
	assert.Equal(t, []any{nil}, topologyV1ColumnValues(t, evidenceTable, "dst_if_index"))
	assert.Equal(t, []any{nil}, topologyV1ColumnValues(t, evidenceTable, "src_port_name"))
	assert.Equal(t, []any{nil}, topologyV1ColumnValues(t, evidenceTable, "dst_port_name"))
	assert.Equal(t, []any{nil}, topologyV1ColumnValues(t, evidenceTable, "src_port_id"))
	assert.Equal(t, []any{nil}, topologyV1ColumnValues(t, evidenceTable, "dst_port_id"))
}

func TestSNMPTopologyToV1_L2EvidenceDistinguishesParallelLinksByTypedEndpoints(t *testing.T) {
	data := topologyData{
		AgentID:     "agent-test",
		CollectedAt: time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC),
		View:        "summary",
		Actors: []topologyActor{
			{
				ActorID:   "device-a",
				ActorType: "device",
				Source:    "snmp",
				Match:     topologyMatch{SysName: "switch-a"},
			},
			{
				ActorID:   "device-b",
				ActorType: "device",
				Source:    "snmp",
				Match:     topologyMatch{SysName: "switch-b"},
			},
		},
		Links: []topologyLink{
			{
				Protocol:   "lldp",
				LinkType:   "lldp",
				Direction:  "bidirectional",
				SrcActorID: "device-a",
				DstActorID: "device-b",
				Src: topologyLinkEndpoint{
					IfIndex:  1,
					IfName:   "Gi0/1",
					PortID:   "1",
					PortName: "Gi0/1",
				},
				Dst: topologyLinkEndpoint{
					IfIndex:  11,
					IfName:   "Eth1",
					PortID:   "11",
					PortName: "Eth1",
				},
			},
			{
				Protocol:   "lldp",
				LinkType:   "lldp",
				Direction:  "bidirectional",
				SrcActorID: "device-a",
				DstActorID: "device-b",
				Src: topologyLinkEndpoint{
					IfIndex:  2,
					IfName:   "Gi0/2",
					PortID:   "2",
					PortName: "Gi0/2",
				},
				Dst: topologyLinkEndpoint{
					IfIndex:  12,
					IfName:   "Eth2",
					PortID:   "12",
					PortName: "Eth2",
				},
			},
		},
	}

	payload, err := topologyv1renderer.Render(data)
	require.NoError(t, err)
	require.NoError(t, validateTopologyV1Data(payload))
	require.Contains(t, payload.Evidence, "lldp")

	evidenceTable := payload.Evidence["lldp"].Table
	require.Equal(t, 2, evidenceTable.Rows)
	assert.Equal(t, []any{0, 1}, topologyV1ColumnValues(t, evidenceTable, "link"))
	assert.Equal(t, []any{uint64(1), uint64(2)}, topologyV1ColumnValues(t, evidenceTable, "src_if_index"))
	assert.Equal(t, []string{"Gi0/1", "Gi0/2"}, topologyV1StringColumnValues(t, payload, evidenceTable, "src_port_name"))
	assert.Equal(t, []string{"1", "2"}, topologyV1StringColumnValues(t, payload, evidenceTable, "src_port_id"))
	assert.Equal(t, []any{uint64(11), uint64(12)}, topologyV1ColumnValues(t, evidenceTable, "dst_if_index"))
	assert.Equal(t, []string{"Eth1", "Eth2"}, topologyV1StringColumnValues(t, payload, evidenceTable, "dst_port_name"))
	assert.Equal(t, []string{"11", "12"}, topologyV1StringColumnValues(t, payload, evidenceTable, "dst_port_id"))
}

func TestNormalizeTopologyInferenceStrategy(t *testing.T) {
	tests := map[string]struct {
		in   string
		want string
	}{
		"default-empty":         {in: "", want: topologyInferenceStrategyFDBMinimumKnowledge},
		"fdb-minimum-knowledge": {in: topologyInferenceStrategyFDBMinimumKnowledge, want: topologyInferenceStrategyFDBMinimumKnowledge},
		"stp-parent-tree":       {in: topologyInferenceStrategySTPParentTree, want: topologyInferenceStrategySTPParentTree},
		"fdb-pairwise":          {in: topologyInferenceStrategyFDBPairwise, want: topologyInferenceStrategyFDBPairwise},
		"stp-fdb-correlated":    {in: topologyInferenceStrategySTPFDBCorrelated, want: topologyInferenceStrategySTPFDBCorrelated},
		"cdp-fdb-hybrid":        {in: topologyInferenceStrategyCDPFDBHybrid, want: topologyInferenceStrategyCDPFDBHybrid},
		"invalid":               {in: "invalid", want: ""},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, topologyoptions.NormalizeInferenceStrategy(tc.in))
		})
	}
}

func TestNormalizeTopologyManagedFocuses(t *testing.T) {
	tests := map[string]struct {
		in   []string
		want []string
	}{
		"nil":                 {in: nil, want: []string{topologyManagedFocusAllDevices}},
		"empty":               {in: []string{}, want: []string{topologyManagedFocusAllDevices}},
		"blank":               {in: []string{""}, want: []string{topologyManagedFocusAllDevices}},
		"comma-blanks":        {in: []string{" , , "}, want: []string{topologyManagedFocusAllDevices}},
		"invalid":             {in: []string{"invalid"}, want: []string{topologyManagedFocusAllDevices}},
		"deduplicated-ips":    {in: []string{"ip:10.0.0.2", "ip:10.0.0.1", "ip:10.0.0.2"}, want: []string{"ip:10.0.0.1", "ip:10.0.0.2"}},
		"comma-separated-ips": {in: []string{" ip:10.0.0.2 , ip:10.0.0.1 "}, want: []string{"ip:10.0.0.1", "ip:10.0.0.2"}},
		"all-devices-token":   {in: []string{"ip:10.0.0.1", topologyManagedFocusAllDevices}, want: []string{topologyManagedFocusAllDevices}},
		"all-devices-comma":   {in: []string{"ip:10.0.0.1,all_devices"}, want: []string{topologyManagedFocusAllDevices}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, topologyoptions.NormalizeManagedFocuses(tc.in))
		})
	}

	assert.Equal(
		t,
		"ip:10.0.0.1,ip:10.0.0.2",
		topologyoptions.FormatManagedFocuses([]string{"ip:10.0.0.2", "ip:10.0.0.1"}),
	)
	assert.Equal(t, []string{topologyManagedFocusAllDevices}, topologyoptions.ParseManagedFocuses(""))
	assert.Equal(t, []string{"10.0.0.1", "10.0.0.2"}, topologyoptions.ManagedFocusSelectedIPs("ip:10.0.0.2,ip:10.0.0.1"))
	assert.True(t, topologyoptions.IsManagedFocusAllDevices(topologyManagedFocusAllDevices))
	assert.False(t, topologyoptions.IsManagedFocusAllDevices("ip:10.0.0.1"))
}

func newTestTopologyCacheLLDP(
	agentID string,
	ts time.Time,
	localChassis, localSysName, localMgmtIP, localPortID string,
	remoteChassis, remoteSysName, remoteMgmtIP, remotePortID string,
) *topologyCache {
	cache := newTopologyCache()
	cache.updateTime = ts
	cache.lastUpdate = ts
	cache.agentID = agentID
	cache.localDevice = topologyDevice{
		ChassisID:     localChassis,
		ChassisIDType: "macAddress",
		SysName:       localSysName,
		ManagementIP:  localMgmtIP,
	}
	cache.lldpLocPorts["1"] = &lldpLocPort{
		portNum:       "1",
		portID:        localPortID,
		portIDSubtype: "interfaceName",
	}
	cache.lldpRemotes["1:1"] = &lldpRemote{
		localPortNum:     "1",
		remIndex:         "1",
		chassisID:        remoteChassis,
		chassisIDSubtype: "macAddress",
		portID:           remotePortID,
		portIDSubtype:    "interfaceName",
		sysName:          remoteSysName,
		managementAddr:   remoteMgmtIP,
	}
	return cache
}

func validateTopologyV1Data(data topologyv1.Data) error {
	bs, err := json.Marshal(data)
	if err != nil {
		return err
	}
	var decoded map[string]any
	if err := json.Unmarshal(bs, &decoded); err != nil {
		return err
	}
	if err := topologyv1.ValidateDecodedData(decoded); err != nil {
		return err
	}

	schemaPath := filepath.Clean(filepath.Join("..", "..", "..", "..", "..", "plugins.d", "FUNCTION_TOPOLOGY_SCHEMA.json"))
	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		return err
	}
	var schemaDoc any
	if err := json.Unmarshal(schemaBytes, &schemaDoc); err != nil {
		return err
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", schemaDoc); err != nil {
		return err
	}
	schema, err := compiler.Compile("schema.json")
	if err != nil {
		return err
	}
	var response any
	if err := json.Unmarshal([]byte(`{"status":200,"type":"topology","data":`+string(bs)+`}`), &response); err != nil {
		return err
	}
	return schema.Validate(response)
}

func topologyV1ColumnType(table topologyv1.Table, columnID string) string {
	for _, column := range table.Columns {
		if column.ID == columnID {
			return column.Type
		}
	}
	return ""
}

func topologyV1ColumnRole(table topologyv1.Table, columnID string) string {
	for _, column := range table.Columns {
		if column.ID == columnID {
			return column.Role
		}
	}
	return ""
}

func topologyV1ColumnValues(t *testing.T, table topologyv1.Table, columnID string) []any {
	t.Helper()

	for columnIndex, column := range table.Columns {
		if column.ID == columnID {
			return topologyV1DecodeColumnValues(t, table, columnIndex)
		}
	}

	require.Failf(t, "missing column", "column %q not found", columnID)
	return nil
}

func topologyV1StringColumnValues(t *testing.T, data topologyv1.Data, table topologyv1.Table, columnID string) []string {
	t.Helper()

	for columnIndex, column := range table.Columns {
		if column.ID != columnID {
			continue
		}
		require.Equal(t, "string_ref", column.Type)
		require.NotEmpty(t, column.Dictionary)
		dict := data.Dictionaries[column.Dictionary]
		require.NotNil(t, dict)

		values := topologyV1DecodeColumnValues(t, table, columnIndex)
		out := make([]string, 0, len(values))
		for _, value := range values {
			ref, ok := value.(int)
			require.Truef(t, ok, "expected integer dictionary reference for %q, got %T", columnID, value)
			require.GreaterOrEqual(t, ref, 0)
			require.Less(t, ref, len(dict))
			text, ok := dict[ref].(string)
			require.Truef(t, ok, "expected string dictionary value for %q, got %T", columnID, dict[ref])
			out = append(out, text)
		}
		return out
	}

	require.Failf(t, "missing column", "column %q not found", columnID)
	return nil
}

func topologyV1DecodeColumnValues(t *testing.T, table topologyv1.Table, columnIndex int) []any {
	t.Helper()

	switch encoding := table.Values[columnIndex].(type) {
	case topologyv1.ValuesEncoding:
		return encoding.Values
	case *topologyv1.ValuesEncoding:
		require.NotNil(t, encoding)
		return encoding.Values
	case topologyv1.ConstEncoding:
		values := make([]any, table.Rows)
		for i := range values {
			values[i] = encoding.Value
		}
		return values
	case *topologyv1.ConstEncoding:
		require.NotNil(t, encoding)
		values := make([]any, table.Rows)
		for i := range values {
			values[i] = encoding.Value
		}
		return values
	case topologyv1.DictEncoding:
		values := make([]any, 0, len(encoding.Indexes))
		for _, index := range encoding.Indexes {
			require.GreaterOrEqual(t, index, 0)
			require.Less(t, index, len(encoding.Values))
			values = append(values, encoding.Values[index])
		}
		return values
	case *topologyv1.DictEncoding:
		require.NotNil(t, encoding)
		values := make([]any, 0, len(encoding.Indexes))
		for _, index := range encoding.Indexes {
			require.GreaterOrEqual(t, index, 0)
			require.Less(t, index, len(encoding.Values))
			values = append(values, encoding.Values[index])
		}
		return values
	default:
		require.Failf(t, "unsupported encoding", "column %d has unsupported encoding %T", columnIndex, encoding)
		return nil
	}
}

func topologyV1LegendLinkTypes(data topologyv1.Data) []string {
	if data.Presentation == nil || data.Presentation.Legend == nil {
		return nil
	}
	out := make([]string, 0, len(data.Presentation.Legend.Links))
	for _, entry := range data.Presentation.Legend.Links {
		out = append(out, entry.Type)
	}
	return out
}

func requireTopologyV1ModalSection(t *testing.T, sections []topologyv1.ModalSection, id string) topologyv1.ModalSection {
	t.Helper()

	for _, section := range sections {
		if section.ID == id {
			return section
		}
	}
	require.Failf(t, "missing modal section", "section %q not found", id)
	return topologyv1.ModalSection{}
}
