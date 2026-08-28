// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/snmptopologyfunc"
	"github.com/stretchr/testify/require"
)

func TestSNMPTopologyCreatorOwnsTopologyFunction(t *testing.T) {
	creator := newCreator(ddsnmp.NewDeviceStore(), NewTrapEnrichmentHandle(), newTestReverseDNSResolver())
	require.Nil(t, creator.Create)
	require.NotNil(t, creator.CreateV2)
	require.Equal(t, collectorapi.InstancePolicySingle, creator.InstancePolicy)
	require.False(t, creator.FunctionOnly)
	require.NotNil(t, creator.SharedFunctions)
	require.NotNil(t, creator.MethodHandler)
	require.Implements(t, (*collectorapi.CollectorV2Runner)(nil), creator.CreateV2())

	methods := creator.SharedFunctions()
	require.Len(t, methods, 1)
	require.Equal(t, snmptopologyfunc.MethodID, methods[0].ID)
	require.Equal(t, snmptopologyfunc.FunctionName, methods[0].FunctionName)
	require.Nil(t, methods[0].Available)

	coll := newTestSNMPTopologyCollector()
	require.Implements(t, (*collectorapi.FunctionAvailability)(nil), coll)
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
				_ = newCreator(tc.store, tc.traps, newTestReverseDNSResolver())
			})
		})
	}
	require.PanicsWithValue(t, "snmp_topology Register requires a non-nil reverse DNS resolver", func() {
		_ = newCreator(ddsnmp.NewDeviceStore(), NewTrapEnrichmentHandle(), nil)
	})
}

func TestSNMPTopologyFunctionAvailabilityBecomesReadyAfterRenderableObservation(t *testing.T) {
	creator := newCreator(ddsnmp.NewDeviceStore(), NewTrapEnrichmentHandle(), newTestReverseDNSResolver())
	methods := creator.SharedFunctions()
	require.Len(t, methods, 1)
	require.Nil(t, methods[0].Available)

	coll, ok := creator.CreateV2().(*Collector)
	require.True(t, ok)
	require.False(t, coll.FunctionAvailable(snmptopologyfunc.MethodID))
	cache := newTopologyBuilder()
	seedPublishedEndpointSnapshot(cache)
	publishTestTopologyBuilder(coll.topologyRegistry, cache)

	require.True(t, coll.FunctionAvailable(snmptopologyfunc.MethodID))
}

func TestSNMPTopologyFunctionAvailabilityChangesOnlyWithPublishedGeneration(t *testing.T) {
	coll := newTestSNMPTopologyCollector()
	publishedAt := time.Now()
	builder := newTopologyBuilder()
	seedPublishedEndpointSnapshot(builder)
	builder.updateTime = publishedAt
	builder.lastUpdate = publishedAt
	builder.staleAfter = 20 * time.Millisecond
	const registrationID ddsnmp.DeviceRegistrationID = 1
	device := freezeTestTopologyBuilderAt(registrationID, publishedAt, builder)
	states := map[ddsnmp.DeviceRegistrationID]deviceRefreshState{registrationID: {generation: device}}
	coll.topologyRegistry.publishGeneration(newTopologyGeneration(1, publishedAt, coll.topologyRegistry.producerScope(), states))

	require.True(t, coll.FunctionAvailable(snmptopologyfunc.MethodID))
	require.True(t, device.freshAt(publishedAt.Add(10*time.Millisecond)))
	require.False(t, device.freshAt(publishedAt.Add(21*time.Millisecond)))
	require.True(t, coll.FunctionAvailable(snmptopologyfunc.MethodID),
		"one published generation must not decay between completed sweeps")

	coll.topologyRegistry.publishGeneration(newTopologyGeneration(
		2,
		publishedAt.Add(21*time.Millisecond),
		coll.topologyRegistry.producerScope(),
		states,
	))
	require.False(t, coll.FunctionAvailable(snmptopologyfunc.MethodID),
		"the next completed sweep must remove an expired retained generation from renderable membership")
}

func TestSNMPTopologyFunctionAvailabilityResetsWhenCollectorRuns(t *testing.T) {
	creator := newCreator(ddsnmp.NewDeviceStore(), NewTrapEnrichmentHandle(), newTestReverseDNSResolver())
	methods := creator.SharedFunctions()
	require.Len(t, methods, 1)
	require.Nil(t, methods[0].Available)

	coll, ok := creator.CreateV2().(*Collector)
	require.True(t, ok)
	builder := newTopologyBuilder()
	seedPublishedEndpointSnapshot(builder)
	publishTestTopologyBuilder(coll.topologyRegistry, builder)
	require.True(t, coll.FunctionAvailable(snmptopologyfunc.MethodID))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- coll.Run(ctx)
	}()

	require.Eventually(t, func() bool {
		return !coll.FunctionAvailable(snmptopologyfunc.MethodID)
	}, time.Second, 10*time.Millisecond)

	cancel()
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("collector did not stop")
	}
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
				_ = New(tc.store, tc.traps, newTestReverseDNSResolver())
			})
		})
	}
	require.PanicsWithValue(t, "snmp_topology New requires a non-nil reverse DNS resolver", func() {
		_ = New(ddsnmp.NewDeviceStore(), NewTrapEnrichmentHandle(), nil)
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
