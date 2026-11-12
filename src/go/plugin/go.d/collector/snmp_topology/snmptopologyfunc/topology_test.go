// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopologyfunc

import (
	"context"
	"errors"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
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
	assert.True(t, cfg.DynamicInfo)
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
	require.Len(t, mapType.Options, 4)
	assert.Equal(t, topologyoptions.MapTypeManagedFabric, mapType.Options[0].ID)
	assert.True(t, mapType.Options[0].Default)
	assert.Equal(t, topologyoptions.MapTypeLLDPCDPManaged, mapType.Options[1].ID)
	assert.Equal(t, topologyoptions.MapTypeHighConfidenceInferred, mapType.Options[2].ID)
	assert.Equal(t, topologyoptions.MapTypeAllDevicesLowConfidence, mapType.Options[3].ID)

	strategy := cfg.RequiredParams[2]
	assert.Equal(t, ParamInferenceStrategy, strategy.ID)
	assert.Equal(t, "Infer Strategy", strategy.Name)
	assert.Equal(t, funcapi.ParamSelect, strategy.Selection)
	require.Len(t, strategy.Options, 5)
	assert.Equal(t, topologyoptions.InferenceStrategyFDBMinimumKnowledge, strategy.Options[0].ID)
	assert.True(t, strategy.Options[0].Default)
	assert.Equal(t, topologyoptions.InferenceStrategySTPParentTree, strategy.Options[1].ID)
	assert.Equal(t, topologyoptions.InferenceStrategyFDBPairwise, strategy.Options[2].ID)
	assert.Equal(t, topologyoptions.InferenceStrategySTPFDBCorrelated, strategy.Options[3].ID)
	assert.Equal(t, topologyoptions.InferenceStrategyCDPFDBHybrid, strategy.Options[4].ID)

	managedFocus := cfg.RequiredParams[3]
	assert.Equal(t, ParamManagedDeviceFocus, managedFocus.ID)
	assert.Equal(t, "Focus On", managedFocus.Name)
	assert.Equal(t, funcapi.ParamMultiSelect, managedFocus.Selection)
	require.Len(t, managedFocus.Options, 1)
	assert.Equal(t, topologyoptions.ManagedFocusAllDevices, managedFocus.Options[0].ID)
	assert.True(t, managedFocus.Options[0].Default)

	depth := cfg.RequiredParams[4]
	assert.Equal(t, ParamDepth, depth.ID)
	assert.Equal(t, "Focus Depth", depth.Name)
	assert.Equal(t, funcapi.ParamSelect, depth.Selection)
	require.NotEmpty(t, depth.Options)
	assert.Equal(t, topologyoptions.DepthAll, depth.Options[0].ID)
	assert.True(t, depth.Options[0].Default)
}

func TestTopologyHandlerMethodParams(t *testing.T) {
	deps := &fakeDeps{
		targets: []topologyoptions.ManagedFocusTarget{
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
	assert.Equal(t, topologyoptions.ManagedFocusAllDevices, params[3].Options[0].ID)
	assert.Equal(t, "ip:10.0.0.1", params[3].Options[1].ID)
	assert.Zero(t, deps.snapshotCalls, "metadata discovery must not render or collect a topology")

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
	assert.Equal(t, topologyoptions.QueryOptions{
		CollapseActorsByIP:     true,
		EliminateNonIPInferred: true,
		MapType:                topologyoptions.MapTypeManagedFabric,
		InferenceStrategy:      topologyoptions.InferenceStrategyFDBMinimumKnowledge,
		ManagedDeviceFocus:     topologyoptions.ManagedFocusAllDevices,
		Depth:                  topologyoptions.DepthAllInternal,
	}, deps.lastOptions)
}

func TestTopologyHandlerHandleSelectorParams(t *testing.T) {
	deps := &fakeDeps{
		data: topologyv1.Data{SchemaVersion: topologyv1.SchemaVersion},
		ok:   true,
		targets: []topologyoptions.ManagedFocusTarget{
			{Value: "ip:10.0.0.2", Name: "sw-b (10.0.0.2)"},
			{Value: "ip:10.0.0.1", Name: "sw-a (10.0.0.1)"},
		},
	}
	handler := NewHandler(deps)
	paramCfgs, err := handler.MethodParams(context.Background(), MethodID)
	require.NoError(t, err)

	params := funcapi.ResolveParams(paramCfgs, map[string][]string{
		ParamNodesIdentity:      {NodesIdentityMAC},
		ParamMapType:            {topologyoptions.MapTypeHighConfidenceInferred},
		ParamInferenceStrategy:  {topologyoptions.InferenceStrategySTPFDBCorrelated},
		ParamManagedDeviceFocus: {"ip:10.0.0.2", "ip:10.0.0.1"},
		ParamDepth:              {"2"},
	})

	resp := handler.Handle(context.Background(), MethodID, params)
	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.Status)
	require.Equal(t, 1, deps.snapshotCalls)
	assert.Equal(t, topologyoptions.QueryOptions{
		CollapseActorsByIP:     false,
		EliminateNonIPInferred: false,
		MapType:                topologyoptions.MapTypeHighConfidenceInferred,
		InferenceStrategy:      topologyoptions.InferenceStrategySTPFDBCorrelated,
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
	assert.Equal(t, topologyoptions.QueryOptions{
		CollapseActorsByIP:     true,
		EliminateNonIPInferred: true,
		MapType:                topologyoptions.MapTypeManagedFabric,
		InferenceStrategy:      topologyoptions.InferenceStrategyFDBMinimumKnowledge,
		ManagedDeviceFocus:     topologyoptions.ManagedFocusAllDevices,
		Depth:                  topologyoptions.DepthAllInternal,
	}, deps.lastOptions)
}

func TestTopologyHandlerUnavailableAndErrors(t *testing.T) {
	tests := map[string]struct {
		deps        Deps
		method      string
		wantStatus  int
		wantMessage string
	}{
		"unknown-method": {
			deps:       &fakeDeps{},
			method:     "unknown",
			wantStatus: 404,
		},
		"nil-deps": {
			method:      MethodID,
			wantStatus:  503,
			wantMessage: "topology data not available",
		},
		"snapshot-unavailable": {
			deps:        &fakeDeps{},
			method:      MethodID,
			wantStatus:  503,
			wantMessage: "topology data not available",
		},
		"snapshot-error": {
			deps:        &fakeDeps{err: errors.New("boom")},
			method:      MethodID,
			wantStatus:  500,
			wantMessage: "failed to build topology response",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			resp := NewHandler(tc.deps).Handle(context.Background(), tc.method, nil)
			require.NotNil(t, resp)
			assert.Equal(t, tc.wantStatus, resp.Status)
			if tc.wantMessage != "" {
				assert.Contains(t, resp.Message, tc.wantMessage)
			}
		})
	}
}

type fakeDeps struct {
	data topologyv1.Data
	ok   bool
	err  error

	targets         []topologyoptions.ManagedFocusTarget
	snapshotTargets []topologyoptions.ManagedFocusTarget
	targetCalls     int

	snapshotCalls int
	lastOptions   topologyoptions.QueryOptions
}

func (d *fakeDeps) Snapshot(options topologyoptions.QueryOptions) (SnapshotResult, bool, error) {
	d.snapshotCalls++
	d.lastOptions = options
	targets := d.snapshotTargets
	if targets == nil {
		targets = d.targets
	}
	return SnapshotResult{Data: d.data, FocusTargets: targets}, d.ok, d.err
}

func (d *fakeDeps) ManagedDeviceFocusTargets() []topologyoptions.ManagedFocusTarget {
	d.targetCalls++
	return d.targets
}

func TestMethodsRequirePaidPlan(t *testing.T) {
	// Viewing SNMP topology requires a paid plan (access 0x0017).
	for _, method := range Methods() {
		assert.True(t, method.RequireCloud, method.ID)
		assert.True(t, method.RequireCommercial, method.ID)
	}
}

func TestTopologyHandlerFocusUsesSnapshotChoicesAndResolvedPolicy(t *testing.T) {
	for _, identity := range []string{NodesIdentityIP, NodesIdentityMAC} {
		t.Run(identity, func(t *testing.T) {
			deps := &fakeDeps{ok: true, targets: []topologyoptions.ManagedFocusTarget{{Value: "ip:192.0.2.1", Name: "Old"}},
				snapshotTargets: []topologyoptions.ManagedFocusTarget{{Value: "ip:192.0.2.2", Name: "Current"}}}
			handler := NewHandler(deps)
			metadata, err := handler.MethodParams(context.Background(), MethodID)
			require.NoError(t, err)
			assert.Equal(t, "ip:192.0.2.1", metadata[3].Options[1].ID)
			response := handler.Handle(context.Background(), MethodID, funcapi.ResolvedParams{
				ParamNodesIdentity: {IDs: []string{identity}},
			})
			require.Equal(t, 200, response.Status)
			require.NoError(t, funcapi.ValidateFocusParams(response.RequiredParams))
			assert.Equal(t, 1, deps.targetCalls, "data response must not reacquire discovery metadata")
			assert.Equal(t, "ip:192.0.2.2", response.RequiredParams[3].Options[1].ID)
			focus := response.RequiredParams[3].Focus
			require.NotNil(t, focus)
			assert.Equal(t, funcapi.FocusRoleRoots, focus.Role)
			assert.Equal(t, topologyoptions.ManagedFocusAllDevices, focus.UnfocusedOption)
			assert.Equal(t, &funcapi.FocusBindings{Table: topologyoptions.ManagedFocusBindingsTable, OptionColumn: "option", ActorColumn: "actor"}, focus.Bindings)
			assert.Equal(t, []string{"segment"}, focus.Traversal.ConnectorTypes)
			assert.Equal(t, []string{"l3_subnet_segment"}, focus.Traversal.BoundaryTypes)
			assert.True(t, focus.Traversal.ConnectRoots)
			if identity == NodesIdentityIP {
				assert.Equal(t, []string{"segment", "l3_subnet_segment"}, focus.Traversal.PruneLeafTypes)
				assert.Equal(t, []funcapi.FocusProtectedMembership{{ActorType: "l3_subnet_segment", LinkType: "l3_subnet_membership"}}, focus.Traversal.ProtectedMemberships)
			} else {
				assert.Empty(t, focus.Traversal.PruneLeafTypes)
				assert.Empty(t, focus.Traversal.ProtectedMemberships)
			}
			assert.Equal(t, &funcapi.FocusConfig{Role: funcapi.FocusRoleDepth, UnlimitedOption: topologyoptions.DepthAll}, response.RequiredParams[4].Focus)
		})
	}
}
