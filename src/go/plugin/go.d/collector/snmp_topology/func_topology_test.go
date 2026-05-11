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
	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopologyMethodConfigIncludesSelectors(t *testing.T) {
	cfg := topologyMethodConfig()
	assert.True(t, cfg.AgentWide)
	require.Len(t, cfg.RequiredParams, 5)

	identity := cfg.RequiredParams[0]
	assert.Equal(t, topologyParamNodesIdentity, identity.ID)
	assert.Equal(t, funcapi.ParamSelect, identity.Selection)
	require.Len(t, identity.Options, 2)
	assert.Equal(t, topologyNodesIdentityIP, identity.Options[0].ID)
	assert.True(t, identity.Options[0].Default)
	assert.Equal(t, topologyNodesIdentityMAC, identity.Options[1].ID)

	mapType := cfg.RequiredParams[1]
	assert.Equal(t, topologyParamMapType, mapType.ID)
	assert.Equal(t, "Map", mapType.Name)
	assert.Equal(t, funcapi.ParamSelect, mapType.Selection)
	require.Len(t, mapType.Options, 3)
	assert.Equal(t, topologyMapTypeLLDPCDPManaged, mapType.Options[0].ID)
	assert.True(t, mapType.Options[0].Default)
	assert.Equal(t, topologyMapTypeHighConfidenceInferred, mapType.Options[1].ID)
	assert.Equal(t, topologyMapTypeAllDevicesLowConfidence, mapType.Options[2].ID)

	strategy := cfg.RequiredParams[2]
	assert.Equal(t, topologyParamInferenceStrategy, strategy.ID)
	assert.Equal(t, "Infer Strategy", strategy.Name)
	assert.Equal(t, funcapi.ParamSelect, strategy.Selection)
	require.Len(t, strategy.Options, 5)
	assert.Equal(t, topologyInferenceStrategyFDBMinimumKnowledge, strategy.Options[0].ID)
	assert.True(t, strategy.Options[0].Default)
	assert.Equal(t, topologyInferenceStrategySTPParentTree, strategy.Options[1].ID)
	assert.Equal(t, topologyInferenceStrategyFDBPairwise, strategy.Options[2].ID)
	assert.Equal(t, topologyInferenceStrategySTPFDBCorrelated, strategy.Options[3].ID)
	assert.Equal(t, topologyInferenceStrategyCDPFDBHybrid, strategy.Options[4].ID)

	managedFocus := cfg.RequiredParams[3]
	assert.Equal(t, topologyParamManagedDeviceFocus, managedFocus.ID)
	assert.Equal(t, "Focus On", managedFocus.Name)
	assert.Equal(t, funcapi.ParamMultiSelect, managedFocus.Selection)
	require.Len(t, managedFocus.Options, 1)
	assert.Equal(t, topologyManagedFocusAllDevices, managedFocus.Options[0].ID)
	assert.True(t, managedFocus.Options[0].Default)

	depth := cfg.RequiredParams[4]
	assert.Equal(t, topologyParamDepth, depth.ID)
	assert.Equal(t, "Focus Depth", depth.Name)
	assert.Equal(t, funcapi.ParamSelect, depth.Selection)
	require.NotEmpty(t, depth.Options)
	assert.Equal(t, topologyDepthAll, depth.Options[0].ID)
	assert.True(t, depth.Options[0].Default)
}

func TestFuncTopology_MethodParams(t *testing.T) {
	prev := snmpTopologyRegistry
	t.Cleanup(func() {
		snmpTopologyRegistry = prev
	})

	registry := newTopologyRegistry()
	snmpTopologyRegistry = registry
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

	f := &funcTopology{}

	params, err := f.MethodParams(context.Background(), topologyMethodID)
	require.NoError(t, err)
	require.Len(t, params, 5)
	assert.Equal(t, topologyParamNodesIdentity, params[0].ID)
	assert.Equal(t, topologyParamMapType, params[1].ID)
	assert.Equal(t, topologyParamInferenceStrategy, params[2].ID)
	assert.Equal(t, topologyParamManagedDeviceFocus, params[3].ID)
	assert.Equal(t, topologyParamDepth, params[4].ID)
	require.GreaterOrEqual(t, len(params[3].Options), 2)
	assert.Equal(t, topologyManagedFocusAllDevices, params[3].Options[0].ID)
	assert.Equal(t, "ip:10.0.0.1", params[3].Options[1].ID)

	params, err = f.MethodParams(context.Background(), "unknown")
	require.NoError(t, err)
	assert.Nil(t, params)
}

func TestFuncTopology_Handle_DefaultStrictL2(t *testing.T) {
	prev := snmpTopologyRegistry
	t.Cleanup(func() {
		snmpTopologyRegistry = prev
	})

	registry := newTopologyRegistry()
	snmpTopologyRegistry = registry
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

	f := &funcTopology{}
	resp := f.Handle(context.Background(), topologyMethodID, nil)
	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.Status)
	assert.Equal(t, "topology", resp.ResponseType)

	data, ok := resp.Data.(topologyv1.Data)
	require.True(t, ok)
	require.NoError(t, validateTopologyV1Data(data))
	assert.Equal(t, topologyv1.SchemaVersion, data.SchemaVersion)
	assert.Equal(t, snmpTopologyV1ProducerSource, data.Producer.Source)
	require.NotNil(t, data.View)
	assert.Equal(t, "summary", data.View.ID)
	assert.Equal(t, "network", data.View.Scope)
	assert.Equal(t, "detailed", data.View.Mode)
	assert.Greater(t, data.Actors.Rows, 0)
	assert.Greater(t, data.Links.Rows, 0)
}

func TestFuncTopology_Handle_AcceptsSelectorParams(t *testing.T) {
	prev := snmpTopologyRegistry
	t.Cleanup(func() {
		snmpTopologyRegistry = prev
	})

	registry := newTopologyRegistry()
	snmpTopologyRegistry = registry
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

	f := &funcTopology{}
	cfg := []funcapi.ParamConfig{
		topologyNodesIdentityParamConfig(),
		topologyMapTypeParamConfig(),
		topologyInferenceStrategyParamConfig(),
		topologyManagedFocusParamConfig(nil),
		topologyDepthParamConfig(),
	}

	params := funcapi.ResolveParams(cfg, map[string][]string{
		topologyParamNodesIdentity:      {topologyNodesIdentityMAC},
		topologyParamMapType:            {topologyMapTypeHighConfidenceInferred},
		topologyParamInferenceStrategy:  {topologyInferenceStrategySTPFDBCorrelated},
		topologyParamManagedDeviceFocus: {"ip:10.0.0.1"},
		topologyParamDepth:              {"2"},
	})
	resp := f.Handle(context.Background(), topologyMethodID, params)
	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.Status)
	data, ok := resp.Data.(topologyv1.Data)
	require.True(t, ok)
	require.NoError(t, validateTopologyV1Data(data))
	require.NotNil(t, data.View)
	assert.Equal(t, "network", data.View.Scope)
}

func TestFuncTopology_Handle_UnknownSelectorsFallbackToDefaults(t *testing.T) {
	prev := snmpTopologyRegistry
	t.Cleanup(func() {
		snmpTopologyRegistry = prev
	})

	registry := newTopologyRegistry()
	snmpTopologyRegistry = registry
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

	f := &funcTopology{}
	cfg := []funcapi.ParamConfig{
		topologyNodesIdentityParamConfig(),
		topologyMapTypeParamConfig(),
		topologyInferenceStrategyParamConfig(),
		topologyManagedFocusParamConfig(nil),
		topologyDepthParamConfig(),
	}

	defaultResp := f.Handle(context.Background(), topologyMethodID, nil)
	require.NotNil(t, defaultResp)
	require.Equal(t, 200, defaultResp.Status)
	defaultData, ok := defaultResp.Data.(topologyv1.Data)
	require.True(t, ok)

	invalidParams := funcapi.ResolveParams(cfg, map[string][]string{
		topologyParamNodesIdentity:      {"unknown"},
		topologyParamMapType:            {"invalid"},
		topologyParamInferenceStrategy:  {"invalid"},
		topologyParamManagedDeviceFocus: {"invalid"},
		topologyParamDepth:              {"invalid"},
	})
	invalidResp := f.Handle(context.Background(), topologyMethodID, invalidParams)
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

func TestSNMPTopologyToV1_PreservesActorCustomTables(t *testing.T) {
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
				Attributes: map[string]any{
					"ports_total":         uint64(0),
					"lldp_neighbor_count": uint64(0),
				},
				Tables: map[string][]map[string]any{
					"ports": {
						{
							"name":           "Gi0/1",
							"neighbor_count": uint64(0),
							"vlan_ids":       []int{10, 20},
							"vendor_note":    "uplink",
							"neighbors": []map[string]any{
								{
									"protocol":    "lldp",
									"remote_port": "Gi0/2",
								},
							},
							"vlans": []map[string]any{
								{
									"id":   "10",
									"name": "users",
								},
							},
						},
					},
					"labels": {
						{"name": "custom-label-row"},
					},
					"custom_labels": {
						{"name": "secondary-custom-label-row"},
					},
					"metadata": {
						{"name": "custom-metadata-row"},
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
				Attributes: map[string]any{
					"learned_sources": []string{"arp"},
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
					Attributes: map[string]any{"if_name": "Gi0/1", "if_index": uint64(0)},
				},
				Dst: topologyLinkEndpoint{
					Attributes: map[string]any{"if_name": "Gi0/2", "if_index": uint64(0)},
				},
			},
		},
	}

	payload, err := snmpTopologyToV1(data)
	require.NoError(t, err)
	require.NoError(t, validateTopologyV1Data(payload))
	require.NotNil(t, payload.Tables)
	require.Contains(t, payload.Tables.Actor, "actor_ports")
	require.Contains(t, payload.Tables.Actor, "actor_labels")
	require.Contains(t, payload.Tables.Actor, "actor_metadata")
	require.Contains(t, payload.Tables.Actor, "actor_custom_labels")
	require.Contains(t, payload.Tables.Actor, "actor_detail_custom_labels")
	require.Contains(t, payload.Tables.Actor, "actor_custom_metadata")

	portTable := payload.Tables.Actor["actor_ports"].Table
	assert.Equal(t, 1, portTable.Rows)
	assert.Equal(t, "json", topologyV1ColumnType(portTable, "neighbors"))
	assert.Equal(t, "json", topologyV1ColumnType(portTable, "vlans"))
	assert.Equal(t, "json", topologyV1ColumnType(portTable, "extra"))
	assert.Equal(t, []any{uint64(0), nil}, topologyV1ColumnValues(t, payload.Actors, "ports_total"))
	assert.Equal(t, []any{nil, []any{"arp"}}, topologyV1ColumnValues(t, payload.Actors, "protocols"))
	assert.Equal(t, []any{nil, nil}, topologyV1ColumnValues(t, payload.Actors, "capabilities"))
	assert.Equal(t, []any{[]any{"10", "20"}}, topologyV1ColumnValues(t, portTable, "vlan_ids"))
	assert.Equal(t, []any{uint64(0)}, topologyV1ColumnValues(t, portTable, "neighbor_count"))
	assert.Equal(t, []any{map[string]any{"vendor_note": "uplink"}}, topologyV1ColumnValues(t, portTable, "extra"))

	labelTable := payload.Tables.Actor["actor_labels"].Table
	assert.GreaterOrEqual(t, labelTable.Rows, 2)
	assert.Equal(t, "actor_ref", topologyV1ColumnType(labelTable, "actor"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(labelTable, "key"))
	assert.Equal(t, "attribute", topologyV1ColumnRole(labelTable, "key"))

	require.Contains(t, payload.Evidence, "lldp")
	evidenceTable := payload.Evidence["lldp"].Table
	assert.Equal(t, "string_ref", topologyV1ColumnType(evidenceTable, "src_port_name"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(evidenceTable, "dst_port_name"))
	assert.Equal(t, "uint", topologyV1ColumnType(evidenceTable, "src_if_index"))
	assert.Equal(t, "uint", topologyV1ColumnType(evidenceTable, "dst_if_index"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(evidenceTable, "src_management_ip"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(evidenceTable, "dst_management_ip"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(evidenceTable, "confidence"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(evidenceTable, "inference"))
	assert.Equal(t, "string_ref", topologyV1ColumnType(evidenceTable, "attachment_mode"))
	assert.Equal(t, []any{uint64(0)}, topologyV1ColumnValues(t, evidenceTable, "src_if_index"))
	assert.Equal(t, []any{uint64(0)}, topologyV1ColumnValues(t, evidenceTable, "dst_if_index"))

	deviceType := payload.Types.ActorTypes["device"]
	require.NotNil(t, deviceType.Presentation)
	require.NotNil(t, deviceType.Presentation.Modal)
	assert.NotEmpty(t, deviceType.Presentation.Modal.Sections)

	linksSection := deviceType.Presentation.Modal.Sections[1]
	assert.Equal(t, "links", linksSection.ID)
	require.Len(t, linksSection.Columns, 7)
	assert.Equal(t, "local_port", linksSection.Columns[1].ID)
	assert.Equal(t, "selected_side_endpoint", linksSection.Columns[1].Projection.Kind)
	assert.Equal(t, "src_actor", linksSection.Columns[1].Projection.SrcActorColumn)
	assert.Equal(t, "dst_actor", linksSection.Columns[1].Projection.DstActorColumn)
	assert.Equal(t, "src_port_name", linksSection.Columns[1].Projection.LocalPortColumn)
	assert.Equal(t, "dst_port_name", linksSection.Columns[1].Projection.RemotePortColumn)
	assert.Equal(t, "remote_port", linksSection.Columns[2].ID)
	assert.Equal(t, "dst_port_name", linksSection.Columns[2].Projection.LocalPortColumn)
	assert.Equal(t, "src_port_name", linksSection.Columns[2].Projection.RemotePortColumn)
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
				Metrics: map[string]any{
					"attachment_mode": "probable_bridge_anchor",
					"inference":       "probable",
				},
			},
		},
	}

	payload, err := snmpTopologyToV1(data)
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

func TestNormalizeTopologyInferenceStrategy(t *testing.T) {
	assert.Equal(t, topologyInferenceStrategyFDBMinimumKnowledge, normalizeTopologyInferenceStrategy(""))
	assert.Equal(t, topologyInferenceStrategyFDBMinimumKnowledge, normalizeTopologyInferenceStrategy(topologyInferenceStrategyFDBMinimumKnowledge))
	assert.Equal(t, topologyInferenceStrategySTPParentTree, normalizeTopologyInferenceStrategy(topologyInferenceStrategySTPParentTree))
	assert.Equal(t, topologyInferenceStrategyFDBPairwise, normalizeTopologyInferenceStrategy(topologyInferenceStrategyFDBPairwise))
	assert.Equal(t, topologyInferenceStrategySTPFDBCorrelated, normalizeTopologyInferenceStrategy(topologyInferenceStrategySTPFDBCorrelated))
	assert.Equal(t, topologyInferenceStrategyCDPFDBHybrid, normalizeTopologyInferenceStrategy(topologyInferenceStrategyCDPFDBHybrid))
	assert.Equal(t, "", normalizeTopologyInferenceStrategy("invalid"))
}

func TestNormalizeTopologyManagedFocuses(t *testing.T) {
	assert.Equal(t, topologyManagedFocusAllDevices, normalizeTopologyManagedFocus(""))
	assert.Equal(t, "ip:10.0.0.1", normalizeTopologyManagedFocus(" ip:10.0.0.1 "))
	assert.Equal(t, []string{topologyManagedFocusAllDevices}, normalizeTopologyManagedFocuses(nil))
	assert.Equal(t, []string{topologyManagedFocusAllDevices}, normalizeTopologyManagedFocuses([]string{}))
	assert.Equal(t, []string{topologyManagedFocusAllDevices}, normalizeTopologyManagedFocuses([]string{""}))
	assert.Equal(t, []string{topologyManagedFocusAllDevices}, normalizeTopologyManagedFocuses([]string{" , , "}))
	assert.Equal(
		t,
		[]string{topologyManagedFocusAllDevices},
		normalizeTopologyManagedFocuses([]string{"invalid"}),
	)
	assert.Equal(
		t,
		[]string{"ip:10.0.0.1", "ip:10.0.0.2"},
		normalizeTopologyManagedFocuses([]string{"ip:10.0.0.2", "ip:10.0.0.1", "ip:10.0.0.2"}),
	)
	assert.Equal(
		t,
		[]string{"ip:10.0.0.1", "ip:10.0.0.2"},
		normalizeTopologyManagedFocuses([]string{" ip:10.0.0.2 , ip:10.0.0.1 "}),
	)
	assert.Equal(
		t,
		[]string{topologyManagedFocusAllDevices},
		normalizeTopologyManagedFocuses([]string{"ip:10.0.0.1", topologyManagedFocusAllDevices}),
	)
	assert.Equal(
		t,
		[]string{topologyManagedFocusAllDevices},
		normalizeTopologyManagedFocuses([]string{"ip:10.0.0.1,all_devices"}),
	)
	assert.Equal(
		t,
		"ip:10.0.0.1,ip:10.0.0.2",
		formatTopologyManagedFocuses([]string{"ip:10.0.0.2", "ip:10.0.0.1"}),
	)
	assert.Equal(t, []string{topologyManagedFocusAllDevices}, parseTopologyManagedFocuses(""))
	assert.Equal(t, "10.0.0.1", topologyManagedFocusSelectedIP("ip:10.0.0.2,ip:10.0.0.1"))
	assert.Equal(t, []string{"10.0.0.1", "10.0.0.2"}, topologyManagedFocusSelectedIPs("ip:10.0.0.2,ip:10.0.0.1"))
	assert.True(t, isTopologyManagedFocusAllDevices(topologyManagedFocusAllDevices))
	assert.False(t, isTopologyManagedFocusAllDevices("ip:10.0.0.1"))
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
