// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	topologyapi "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	topologyv1renderer "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyv1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyv1test"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/snmptopologyfunc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopologyFocusBindingsGolden(t *testing.T) {
	bindings := make(map[string]any)
	for name, tc := range topologyScenarioGoldenCases() {
		payload := tc.scenario.render(t)
		normalized := topologyv1test.NormalizeData(t, payload)
		bindings[name] = normalized.Tables.Actor[topologyoptions.ManagedFocusBindingsTable]
	}
	actual := topologyv1test.CanonicalJSON(t, bindings)
	if *updateSNMPTopologyScenarioGoldens {
		topologyScenarioWriteGolden(t, "testdata", "topology_focus_bindings", actual)
	}
	require.Equal(t, topologyScenarioReadGolden(t, "testdata", "topology_focus_bindings"), actual)
}

func TestTopologyFocusKeepsAcquiredGenerationDuringPublication(t *testing.T) {
	registry := newTopologyRegistry()
	cache := newTopologyBuilder()
	seedPublishedEndpointSnapshot(cache)
	publishTestTopologyBuilder(registry, cache)
	options := topologyoptions.DefaultQueryOptions()
	expected, ok, err := registry.snapshotResultWithEnvironment(options, topologyGraphBuildEnvironment{})
	require.NoError(t, err)
	require.True(t, ok)
	require.NotEmpty(t, expected.FocusTargets)
	require.NotEmpty(t, expected.Data.FocusRootBindings)
	var callbacks int
	actual, ok, err := registry.snapshotResultWithEnvironment(options, topologyGraphBuildEnvironment{
		resolveDNSName: func(string) string {
			callbacks++
			registry.publishGeneration(&topologyGeneration{})
			return ""
		},
	})
	require.NoError(t, err)
	require.True(t, ok)
	require.Positive(t, callbacks, "publish a replacement while the acquired graph is being built")
	require.Empty(t, registry.managedDeviceFocusTargets(), "replacement has no choices")
	assert.Equal(t, expected.FocusTargets, actual.FocusTargets, "response choices must still describe the acquired generation")
	expectedPayload, err := topologyv1renderer.Render(expected.Data)
	require.NoError(t, err)
	actualPayload, err := topologyv1renderer.Render(actual.Data)
	require.NoError(t, err)
	assert.Equal(t, expectedPayload, actualPayload, "bindings and graph must use the same acquired observations")
}

func TestTopologyFocusProducedResponseValidates(t *testing.T) {
	for _, identity := range []string{snmptopologyfunc.NodesIdentityIP, snmptopologyfunc.NodesIdentityMAC} {
		t.Run(identity, func(t *testing.T) {
			registry, _ := newTopologyScenarioReplayFixture(t, newMixedL2L3ControlScenario())
			handler := snmptopologyfunc.NewHandler(funcDepsAdapter{registry: registry})
			params, err := handler.MethodParams(context.Background(), snmptopologyfunc.MethodID)
			require.NoError(t, err)
			resolved := funcapi.ResolveParams(params, map[string][]string{
				snmptopologyfunc.ParamNodesIdentity: {identity},
			})
			result := handler.Handle(context.Background(), snmptopologyfunc.MethodID, resolved)
			require.Equal(t, 200, result.Status)
			response := topologyapi.NewResponse(result.Data.(topologyapi.Data))
			for _, param := range result.RequiredParams {
				response.RequiredParams = append(response.RequiredParams, param.RequiredParam())
			}
			encoded, err := json.Marshal(response)
			require.NoError(t, err)
			var decoded any
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			require.NoError(t, topologyapi.ValidateDecodedResponse(decoded))
			require.Positive(t, response.Data.Tables.Actor[topologyoptions.ManagedFocusBindingsTable].Table.Rows)
		})
	}
}
