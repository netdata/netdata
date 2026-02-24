// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopologyMethodConfigIncludesIdentityAndConnectivitySelectors(t *testing.T) {
	cfg := topologyMethodConfig()
	assert.True(t, cfg.AgentWide)
	require.Len(t, cfg.RequiredParams, 2)

	identity := cfg.RequiredParams[0]
	assert.Equal(t, topologyParamNodesIdentity, identity.ID)
	assert.Equal(t, funcapi.ParamSelect, identity.Selection)
	require.Len(t, identity.Options, 2)
	assert.Equal(t, topologyNodesIdentityIP, identity.Options[0].ID)
	assert.True(t, identity.Options[0].Default)
	assert.Equal(t, topologyNodesIdentityMAC, identity.Options[1].ID)

	connectivity := cfg.RequiredParams[1]
	assert.Equal(t, topologyParamConnectivity, connectivity.ID)
	assert.Equal(t, funcapi.ParamSelect, connectivity.Selection)
	require.Len(t, connectivity.Options, 2)
	assert.Equal(t, topologyConnectivityStrict, connectivity.Options[0].ID)
	assert.Equal(t, topologyConnectivityProbable, connectivity.Options[1].ID)
	assert.True(t, connectivity.Options[1].Default)
}

func TestFuncTopology_MethodParams(t *testing.T) {
	f := newFuncTopology(&funcRouter{})

	params, err := f.MethodParams(context.Background(), topologyMethodID)
	require.NoError(t, err)
	require.Len(t, params, 2)
	assert.Equal(t, topologyParamNodesIdentity, params[0].ID)
	assert.Equal(t, topologyParamConnectivity, params[1].ID)

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

	f := newFuncTopology(&funcRouter{})
	resp := f.Handle(context.Background(), topologyMethodID, nil)
	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.Status)
	assert.Equal(t, "topology", resp.ResponseType)

	data, ok := resp.Data.(topologyData)
	require.True(t, ok)
	assert.Equal(t, "2", data.Layer)
	assert.Equal(t, "summary", data.View)
}

func TestFuncTopology_Handle_AcceptsIdentityAndConnectivityParams(t *testing.T) {
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

	f := newFuncTopology(&funcRouter{})
	cfg := []funcapi.ParamConfig{
		topologyNodesIdentityParamConfig(),
		topologyConnectivityParamConfig(),
	}

	params := funcapi.ResolveParams(cfg, map[string][]string{
		topologyParamNodesIdentity: {topologyNodesIdentityMAC},
		topologyParamConnectivity:  {topologyConnectivityStrict},
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

	f := newFuncTopology(&funcRouter{})
	cfg := []funcapi.ParamConfig{
		topologyNodesIdentityParamConfig(),
		topologyConnectivityParamConfig(),
	}

	defaultResp := f.Handle(context.Background(), topologyMethodID, nil)
	require.NotNil(t, defaultResp)
	require.Equal(t, 200, defaultResp.Status)
	defaultData, ok := defaultResp.Data.(topologyData)
	require.True(t, ok)

	invalidParams := funcapi.ResolveParams(cfg, map[string][]string{
		topologyParamNodesIdentity: {"unknown"},
		topologyParamConnectivity:  {"invalid"},
	})
	invalidResp := f.Handle(context.Background(), topologyMethodID, invalidParams)
	require.NotNil(t, invalidResp)
	require.Equal(t, 200, invalidResp.Status)
	invalidData, ok := invalidResp.Data.(topologyData)
	require.True(t, ok)

	assert.Equal(t, defaultData.Layer, invalidData.Layer)
	assert.Equal(t, defaultData.View, invalidData.View)
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
