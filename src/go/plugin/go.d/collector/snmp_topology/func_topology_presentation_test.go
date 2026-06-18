// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
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
	creator := newCreator(ddsnmp.NewDeviceStore(), NewTrapEnrichmentHandle())
	require.Nil(t, creator.Create)
	require.NotNil(t, creator.CreateV2)
	require.Equal(t, collectorapi.InstancePolicySingle, creator.InstancePolicy)
	require.False(t, creator.FunctionOnly)
	require.NotNil(t, creator.Methods)
	require.NotNil(t, creator.MethodHandler)
	require.Implements(t, (*collectorapi.CollectorV2Runner)(nil), creator.CreateV2())

	methods := creator.Methods()
	require.Len(t, methods, 1)
	require.Equal(t, topologyMethodID, methods[0].ID)
	require.Equal(t, topologyFunctionName, methods[0].FunctionName)
	require.True(t, methods[0].AgentWide)

	coll := newTestSNMPTopologyCollector()
	handler := creator.MethodHandler(&topologyRuntimeJobForTest{collector: coll})
	require.IsType(t, &funcTopology{}, handler)
	require.Same(t, coll.topologyRegistry, handler.(*funcTopology).registry)
	require.Nil(t, creator.MethodHandler(nil))
}

func TestSNMPTopologyCreatorRequiresSharedDependencies(t *testing.T) {
	require.PanicsWithValue(t, "snmp_topology Register requires a non-nil device store", func() {
		_ = newCreator(nil, NewTrapEnrichmentHandle())
	})
	require.PanicsWithValue(t, "snmp_topology Register requires a non-nil trap enrichment handle", func() {
		_ = newCreator(ddsnmp.NewDeviceStore(), nil)
	})
}

type topologyRuntimeJobForTest struct {
	collector *Collector
}

func (j *topologyRuntimeJobForTest) FullName() string   { return "snmp_topology" }
func (j *topologyRuntimeJobForTest) ModuleName() string { return "snmp_topology" }
func (j *topologyRuntimeJobForTest) Name() string       { return "snmp_topology" }
func (j *topologyRuntimeJobForTest) IsRunning() bool    { return true }
func (j *topologyRuntimeJobForTest) Collector() any     { return j.collector }
