// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/snmptopologyfunc"
	"github.com/stretchr/testify/require"
)

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
	require.Equal(t, snmptopologyfunc.MethodID, methods[0].ID)
	require.Equal(t, snmptopologyfunc.FunctionName, methods[0].FunctionName)
	require.True(t, methods[0].AgentWide)
	require.NotNil(t, methods[0].Available)
	require.False(t, methods[0].Available())

	coll := newTestSNMPTopologyCollector()
	handler := creator.MethodHandler(&topologyRuntimeJobForTest{collector: coll})
	require.Implements(t, (*funcapi.MethodHandler)(nil), handler)
	require.Nil(t, creator.MethodHandler(nil))
	require.Nil(t, topologyFunctionHandler(nil))
}

func TestSNMPTopologyCreatorRequiresSharedDependencies(t *testing.T) {
	tests := map[string]struct {
		store     *ddsnmp.DeviceStore
		traps     *TrapEnrichmentHandle
		wantPanic string
	}{
		"nil-device-store": {
			store:     nil,
			traps:     NewTrapEnrichmentHandle(),
			wantPanic: "snmp_topology Register requires a non-nil device store",
		},
		"nil-trap-handle": {
			store:     ddsnmp.NewDeviceStore(),
			traps:     nil,
			wantPanic: "snmp_topology Register requires a non-nil trap enrichment handle",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.PanicsWithValue(t, tc.wantPanic, func() {
				_ = newCreator(tc.store, tc.traps)
			})
		})
	}
}

func TestSNMPTopologyFunctionAvailabilityBecomesReadyAfterRenderableSnapshot(t *testing.T) {
	creator := newCreator(ddsnmp.NewDeviceStore(), NewTrapEnrichmentHandle())
	methods := creator.Methods()
	require.Len(t, methods, 1)
	require.NotNil(t, methods[0].Available)
	require.False(t, methods[0].Available())

	coll, ok := creator.CreateV2().(*Collector)
	require.True(t, ok)
	cache := newTopologyCache()
	seedPublishedEndpointSnapshot(cache)
	coll.topologyRegistry.register(cache)

	coll.updateFunctionAvailability()

	require.True(t, methods[0].Available())
	require.True(t, creator.Methods()[0].Available())
}

func TestSNMPTopologyNewRequiresSharedDependencies(t *testing.T) {
	tests := map[string]struct {
		store     *ddsnmp.DeviceStore
		traps     *TrapEnrichmentHandle
		wantPanic string
	}{
		"nil-device-store": {
			store:     nil,
			traps:     NewTrapEnrichmentHandle(),
			wantPanic: "snmp_topology New requires a non-nil device store",
		},
		"nil-trap-handle": {
			store:     ddsnmp.NewDeviceStore(),
			traps:     nil,
			wantPanic: "snmp_topology New requires a non-nil trap enrichment handle",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.PanicsWithValue(t, tc.wantPanic, func() {
				_ = New(tc.store, tc.traps)
			})
		})
	}
}

type topologyRuntimeJobForTest struct {
	collector *Collector
}

func (j *topologyRuntimeJobForTest) FullName() string   { return "snmp_topology" }
func (j *topologyRuntimeJobForTest) ModuleName() string { return "snmp_topology" }
func (j *topologyRuntimeJobForTest) Name() string       { return "snmp_topology" }
func (j *topologyRuntimeJobForTest) IsRunning() bool    { return true }
func (j *topologyRuntimeJobForTest) Collector() any     { return j.collector }
