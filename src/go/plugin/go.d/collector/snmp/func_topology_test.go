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

func TestTopologyMethodConfigIncludesViewSelector(t *testing.T) {
	cfg := topologyMethodConfig()
	assert.True(t, cfg.AgentWide)
	require.Len(t, cfg.RequiredParams, 1)

	param := cfg.RequiredParams[0]
	assert.Equal(t, topologyParamView, param.ID)
	assert.Equal(t, funcapi.ParamSelect, param.Selection)
	require.Len(t, param.Options, 3)
	assert.Equal(t, topologyViewL2, param.Options[0].ID)
	assert.True(t, param.Options[0].Default)
	assert.Equal(t, topologyViewL3, param.Options[1].ID)
	assert.Equal(t, topologyViewMerged, param.Options[2].ID)
}

func TestFuncTopology_MethodParams(t *testing.T) {
	f := newFuncTopology(&funcRouter{})

	params, err := f.MethodParams(context.Background(), topologyMethodID)
	require.NoError(t, err)
	require.Len(t, params, 1)
	assert.Equal(t, topologyParamView, params[0].ID)

	params, err = f.MethodParams(context.Background(), "unknown")
	require.NoError(t, err)
	assert.Nil(t, params)
}

func TestFuncTopology_Handle_DefaultL2View(t *testing.T) {
	prev := snmpTopologyRegistry
	t.Cleanup(func() {
		snmpTopologyRegistry = prev
	})

	registry := newTopologyRegistry()
	snmpTopologyRegistry = registry

	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.lastUpdate = cache.updateTime
	cache.agentID = "agent-test"
	cache.localDevice = topologyDevice{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		SysName:       "sw-a",
		ManagementIP:  "10.0.0.1",
	}
	cache.lldpLocPorts["1"] = &lldpLocPort{
		portNum:       "1",
		portID:        "Gi0/1",
		portIDSubtype: "interfaceName",
	}
	cache.lldpRemotes["1:1"] = &lldpRemote{
		localPortNum:     "1",
		remIndex:         "1",
		chassisID:        "aa:bb:cc:dd:ee:ff",
		chassisIDSubtype: "macAddress",
		portID:           "Gi0/2",
		portIDSubtype:    "interfaceName",
		sysName:          "sw-b",
		managementAddr:   "10.0.0.2",
	}
	registry.register(cache)

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

func TestFuncTopology_Handle_L3AndMergedViewsUnavailable(t *testing.T) {
	f := newFuncTopology(&funcRouter{})
	cfg := []funcapi.ParamConfig{topologyViewParamConfig()}

	l3Params := funcapi.ResolveParams(cfg, map[string][]string{
		topologyParamView: {topologyViewL3},
	})
	resp := f.Handle(context.Background(), topologyMethodID, l3Params)
	require.NotNil(t, resp)
	assert.Equal(t, 503, resp.Status)
	assert.Contains(t, resp.Message, topologyViewL3)

	mergedParams := funcapi.ResolveParams(cfg, map[string][]string{
		topologyParamView: {topologyViewMerged},
	})
	resp = f.Handle(context.Background(), topologyMethodID, mergedParams)
	require.NotNil(t, resp)
	assert.Equal(t, 503, resp.Status)
	assert.Contains(t, resp.Message, topologyViewMerged)
}

func TestFuncTopology_Handle_MultiJobAggregationAndSelectorBehavior(t *testing.T) {
	prev := snmpTopologyRegistry
	t.Cleanup(func() {
		snmpTopologyRegistry = prev
	})

	registry := newTopologyRegistry()
	snmpTopologyRegistry = registry

	baseTS := time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC)
	cacheA := newTestTopologyCacheLLDP(
		"agent-test",
		baseTS,
		"00:11:22:33:44:55",
		"sw-a",
		"10.0.0.1",
		"Gi0/1",
		"aa:bb:cc:dd:ee:ff",
		"sw-b",
		"10.0.0.2",
		"Gi0/2",
	)
	cacheB := newTestTopologyCacheLLDP(
		"agent-test",
		baseTS.Add(time.Second),
		"aa:bb:cc:dd:ee:ff",
		"sw-b",
		"10.0.0.2",
		"Gi0/2",
		"00:11:22:33:44:55",
		"sw-a",
		"10.0.0.1",
		"Gi0/1",
	)
	registry.register(cacheA)
	registry.register(cacheB)

	f := newFuncTopology(&funcRouter{})
	cfg := []funcapi.ParamConfig{topologyViewParamConfig()}

	defaultResp := f.Handle(context.Background(), topologyMethodID, nil)
	require.NotNil(t, defaultResp)
	require.Equal(t, 200, defaultResp.Status)
	defaultData, ok := defaultResp.Data.(topologyData)
	require.True(t, ok)
	require.Equal(t, "2", defaultData.Layer)
	require.Equal(t, "summary", defaultData.View)
	require.GreaterOrEqual(t, countActorsByType(defaultData, "device"), 2)
	require.GreaterOrEqual(t, defaultData.Stats["links_total"].(int), 1)
	require.GreaterOrEqual(t, defaultData.Stats["links_lldp"].(int), 1)

	l2Params := funcapi.ResolveParams(cfg, map[string][]string{
		topologyParamView: {topologyViewL2},
	})
	l2Resp := f.Handle(context.Background(), topologyMethodID, l2Params)
	require.NotNil(t, l2Resp)
	require.Equal(t, 200, l2Resp.Status)
	l2Data, ok := l2Resp.Data.(topologyData)
	require.True(t, ok)
	assert.Equal(t, defaultData, l2Data)

	invalidParams := funcapi.ResolveParams(cfg, map[string][]string{
		topologyParamView: {"unknown-view"},
	})
	invalidResp := f.Handle(context.Background(), topologyMethodID, invalidParams)
	require.NotNil(t, invalidResp)
	require.Equal(t, 200, invalidResp.Status)
	invalidData, ok := invalidResp.Data.(topologyData)
	require.True(t, ok)
	assert.Equal(t, defaultData, invalidData)

	l3Params := funcapi.ResolveParams(cfg, map[string][]string{
		topologyParamView: {topologyViewL3},
	})
	l3Resp := f.Handle(context.Background(), topologyMethodID, l3Params)
	require.NotNil(t, l3Resp)
	assert.Equal(t, 503, l3Resp.Status)
	assert.Contains(t, l3Resp.Message, topologyViewL3)

	mergedParams := funcapi.ResolveParams(cfg, map[string][]string{
		topologyParamView: {topologyViewMerged},
	})
	mergedResp := f.Handle(context.Background(), topologyMethodID, mergedParams)
	require.NotNil(t, mergedResp)
	assert.Equal(t, 503, mergedResp.Status)
	assert.Contains(t, mergedResp.Message, topologyViewMerged)
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
