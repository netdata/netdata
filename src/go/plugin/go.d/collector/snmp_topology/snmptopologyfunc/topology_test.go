// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopologyfunc

import (
	"context"
	"errors"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMethodsIncludesSelectors(t *testing.T) {
	methods := Methods()
	require.Len(t, methods, 1)

	cfg := methods[0]
	assert.Equal(t, MethodID, cfg.ID)
	assert.Equal(t, FunctionName, cfg.FunctionName)
	assert.Equal(t, []string{MethodID}, cfg.Aliases)
	assert.Equal(t, "topology", cfg.ResponseType)
	assert.True(t, cfg.AgentWide)
	require.Len(t, cfg.RequiredParams, 5)
	require.Nil(t, cfg.Presentation())

	identity := cfg.RequiredParams[0]
	assert.Equal(t, ParamNodesIdentity, identity.ID)
	assert.Equal(t, funcapi.ParamSelect, identity.Selection)
	require.Len(t, identity.Options, 2)
	assert.Equal(t, NodesIdentityIP, identity.Options[0].ID)
	assert.True(t, identity.Options[0].Default)
	assert.Equal(t, NodesIdentityMAC, identity.Options[1].ID)

	mapType := cfg.RequiredParams[1]
	assert.Equal(t, ParamMapType, mapType.ID)
	assert.Equal(t, "Map", mapType.Name)
	assert.Equal(t, funcapi.ParamSelect, mapType.Selection)
	require.Len(t, mapType.Options, 3)
	assert.Equal(t, MapTypeLLDPCDPManaged, mapType.Options[0].ID)
	assert.True(t, mapType.Options[0].Default)
	assert.Equal(t, MapTypeHighConfidenceInferred, mapType.Options[1].ID)
	assert.Equal(t, MapTypeAllDevicesLowConfidence, mapType.Options[2].ID)

	strategy := cfg.RequiredParams[2]
	assert.Equal(t, ParamInferenceStrategy, strategy.ID)
	assert.Equal(t, "Infer Strategy", strategy.Name)
	assert.Equal(t, funcapi.ParamSelect, strategy.Selection)
	require.Len(t, strategy.Options, 5)
	assert.Equal(t, InferenceStrategyFDBMinimumKnowledge, strategy.Options[0].ID)
	assert.True(t, strategy.Options[0].Default)
	assert.Equal(t, InferenceStrategySTPParentTree, strategy.Options[1].ID)
	assert.Equal(t, InferenceStrategyFDBPairwise, strategy.Options[2].ID)
	assert.Equal(t, InferenceStrategySTPFDBCorrelated, strategy.Options[3].ID)
	assert.Equal(t, InferenceStrategyCDPFDBHybrid, strategy.Options[4].ID)

	managedFocus := cfg.RequiredParams[3]
	assert.Equal(t, ParamManagedDeviceFocus, managedFocus.ID)
	assert.Equal(t, "Focus On", managedFocus.Name)
	assert.Equal(t, funcapi.ParamMultiSelect, managedFocus.Selection)
	require.Len(t, managedFocus.Options, 1)
	assert.Equal(t, ManagedFocusAllDevices, managedFocus.Options[0].ID)
	assert.True(t, managedFocus.Options[0].Default)

	depth := cfg.RequiredParams[4]
	assert.Equal(t, ParamDepth, depth.ID)
	assert.Equal(t, "Focus Depth", depth.Name)
	assert.Equal(t, funcapi.ParamSelect, depth.Selection)
	require.NotEmpty(t, depth.Options)
	assert.Equal(t, DepthAll, depth.Options[0].ID)
	assert.True(t, depth.Options[0].Default)
}

func TestTopologyHandlerMethodParams(t *testing.T) {
	deps := &fakeDeps{
		targets: []ManagedFocusTarget{
			{Value: "", Name: "skip"},
			{Value: "ip:10.0.0.1", Name: "sw-a (10.0.0.1)"},
		},
	}
	handler := NewHandler(deps)

	params, err := handler.MethodParams(context.Background(), MethodID)
	require.NoError(t, err)
	require.Len(t, params, 5)
	assert.Equal(t, ParamNodesIdentity, params[0].ID)
	assert.Equal(t, ParamMapType, params[1].ID)
	assert.Equal(t, ParamInferenceStrategy, params[2].ID)
	assert.Equal(t, ParamManagedDeviceFocus, params[3].ID)
	assert.Equal(t, ParamDepth, params[4].ID)
	require.Len(t, params[3].Options, 2)
	assert.Equal(t, ManagedFocusAllDevices, params[3].Options[0].ID)
	assert.Equal(t, "ip:10.0.0.1", params[3].Options[1].ID)

	params, err = handler.MethodParams(context.Background(), "unknown")
	require.NoError(t, err)
	require.Nil(t, params)
}

func TestTopologyHandlerHandleDefaultOptions(t *testing.T) {
	deps := &fakeDeps{data: topologyv1.Data{SchemaVersion: topologyv1.SchemaVersion}, ok: true}
	handler := NewHandler(deps)

	resp := handler.Handle(context.Background(), MethodID, nil)
	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.Status)
	assert.Equal(t, topologyv1.ResponseType, resp.ResponseType)
	assert.Equal(t, "SNMP topology and neighbor discovery data", resp.Help)
	assert.Equal(t, deps.data, resp.Data)

	require.Equal(t, 1, deps.snapshotCalls)
	assert.Equal(t, QueryOptions{
		CollapseActorsByIP:     true,
		EliminateNonIPInferred: true,
		MapType:                MapTypeLLDPCDPManaged,
		InferenceStrategy:      InferenceStrategyFDBMinimumKnowledge,
		ManagedDeviceFocus:     ManagedFocusAllDevices,
		Depth:                  DepthAllInternal,
	}, deps.lastOptions)
}

func TestTopologyHandlerHandleSelectorParams(t *testing.T) {
	deps := &fakeDeps{
		data: topologyv1.Data{SchemaVersion: topologyv1.SchemaVersion},
		ok:   true,
		targets: []ManagedFocusTarget{
			{Value: "ip:10.0.0.2", Name: "sw-b (10.0.0.2)"},
			{Value: "ip:10.0.0.1", Name: "sw-a (10.0.0.1)"},
		},
	}
	handler := NewHandler(deps)
	paramCfgs, err := handler.MethodParams(context.Background(), MethodID)
	require.NoError(t, err)

	params := funcapi.ResolveParams(paramCfgs, map[string][]string{
		ParamNodesIdentity:      {NodesIdentityMAC},
		ParamMapType:            {MapTypeHighConfidenceInferred},
		ParamInferenceStrategy:  {InferenceStrategySTPFDBCorrelated},
		ParamManagedDeviceFocus: {"ip:10.0.0.2", "ip:10.0.0.1"},
		ParamDepth:              {"2"},
	})

	resp := handler.Handle(context.Background(), MethodID, params)
	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.Status)
	require.Equal(t, 1, deps.snapshotCalls)
	assert.Equal(t, QueryOptions{
		CollapseActorsByIP:     false,
		EliminateNonIPInferred: false,
		MapType:                MapTypeHighConfidenceInferred,
		InferenceStrategy:      InferenceStrategySTPFDBCorrelated,
		ManagedDeviceFocus:     "ip:10.0.0.1,ip:10.0.0.2",
		Depth:                  2,
	}, deps.lastOptions)
}

func TestTopologyHandlerHandleUnknownSelectorsFallbackToDefaults(t *testing.T) {
	deps := &fakeDeps{data: topologyv1.Data{SchemaVersion: topologyv1.SchemaVersion}, ok: true}
	handler := NewHandler(deps)
	params := funcapi.ResolvedParams{
		ParamNodesIdentity:      {IDs: []string{"unknown"}},
		ParamMapType:            {IDs: []string{"invalid"}},
		ParamInferenceStrategy:  {IDs: []string{"invalid"}},
		ParamManagedDeviceFocus: {IDs: []string{"invalid"}},
		ParamDepth:              {IDs: []string{"invalid"}},
	}

	resp := handler.Handle(context.Background(), MethodID, params)
	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.Status)
	require.Equal(t, 1, deps.snapshotCalls)
	assert.Equal(t, QueryOptions{
		CollapseActorsByIP:     true,
		EliminateNonIPInferred: true,
		MapType:                MapTypeLLDPCDPManaged,
		InferenceStrategy:      InferenceStrategyFDBMinimumKnowledge,
		ManagedDeviceFocus:     ManagedFocusAllDevices,
		Depth:                  DepthAllInternal,
	}, deps.lastOptions)
}

func TestTopologyHandlerUnavailableAndErrors(t *testing.T) {
	t.Run("unknown method", func(t *testing.T) {
		resp := NewHandler(&fakeDeps{}).Handle(context.Background(), "unknown", nil)
		require.NotNil(t, resp)
		assert.Equal(t, 404, resp.Status)
	})

	t.Run("nil deps", func(t *testing.T) {
		resp := NewHandler(nil).Handle(context.Background(), MethodID, nil)
		require.NotNil(t, resp)
		assert.Equal(t, 503, resp.Status)
		assert.Contains(t, resp.Message, "topology data not available")
	})

	t.Run("snapshot unavailable", func(t *testing.T) {
		resp := NewHandler(&fakeDeps{}).Handle(context.Background(), MethodID, nil)
		require.NotNil(t, resp)
		assert.Equal(t, 503, resp.Status)
		assert.Contains(t, resp.Message, "topology data not available")
	})

	t.Run("snapshot error", func(t *testing.T) {
		resp := NewHandler(&fakeDeps{err: errors.New("boom")}).Handle(context.Background(), MethodID, nil)
		require.NotNil(t, resp)
		assert.Equal(t, 500, resp.Status)
		assert.Contains(t, resp.Message, "failed to build topology response")
	})
}

type fakeDeps struct {
	data topologyv1.Data
	ok   bool
	err  error

	targets []ManagedFocusTarget

	snapshotCalls int
	lastOptions   QueryOptions
}

func (d *fakeDeps) Snapshot(options QueryOptions) (topologyv1.Data, bool, error) {
	d.snapshotCalls++
	d.lastOptions = options
	return d.data, d.ok, d.err
}

func (d *fakeDeps) ManagedDeviceFocusTargets() []ManagedFocusTarget {
	return d.targets
}
