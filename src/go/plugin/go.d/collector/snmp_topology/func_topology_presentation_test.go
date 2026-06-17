// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/require"
)

func TestSNMPTopologyMethodConfigDoesNotUseLegacyPresentation(t *testing.T) {
	method := topologyMethodConfig()

	require.Equal(t, topologyMethodID, method.ID)
	require.Equal(t, topologyFunctionName, method.FunctionName)
	require.Equal(t, []string{topologyMethodID}, method.Aliases)
	require.Equal(t, "topology", method.ResponseType)
	require.Nil(t, method.Presentation())
}

func TestSNMPTopologyCreatorOwnsTopologyFunction(t *testing.T) {
	creator, ok := collectorapi.DefaultRegistry.Lookup("snmp_topology")
	require.True(t, ok)
	require.NotNil(t, creator.Methods)
	require.NotNil(t, creator.MethodHandler)

	methods := creator.Methods()
	require.Len(t, methods, 1)
	require.Equal(t, topologyMethodID, methods[0].ID)
	require.Equal(t, topologyFunctionName, methods[0].FunctionName)
	require.True(t, methods[0].AgentWide)
	require.IsType(t, &funcTopology{}, creator.MethodHandler(nil))
}
