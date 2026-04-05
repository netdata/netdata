// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
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

	data, ok := resp.Data.(topologyData)
	require.True(t, ok)
	assert.Equal(t, "2", data.Layer)
	assert.Equal(t, "summary", data.View)
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
	data, ok := resp.Data.(topologyData)
	require.True(t, ok)
	assert.Equal(t, "2", data.Layer)
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
	defaultData, ok := defaultResp.Data.(topologyData)
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
	invalidData, ok := invalidResp.Data.(topologyData)
	require.True(t, ok)

	assert.Equal(t, defaultData.Layer, invalidData.Layer)
	assert.Equal(t, defaultData.View, invalidData.View)
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
