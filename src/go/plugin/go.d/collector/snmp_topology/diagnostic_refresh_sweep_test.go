// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/diagnostic"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

func TestDiagnosticRefreshSweep_CancellationAndPanicCloseTheCompletePlanWithoutPublication(t *testing.T) {
	for _, tc := range []struct {
		name        string
		run         func(*Collector)
		publication string
		outcome     string
	}{
		{
			name: "canceled before first attempt",
			run: func(collector *Collector) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				collector.refreshTopology(ctx)
			},
			publication: diagnostic.RefreshPublicationCanceled,
			outcome:     diagnostic.RefreshOutcomeCanceledNotStarted,
		},
		{
			name: "panic in flight",
			run: func(collector *Collector) {
				collector.newSnmpClient = func() gosnmp.Handler { panic("test panic") }
				require.Panics(t, func() { collector.refreshTopology(context.Background()) })
			},
			publication: diagnostic.RefreshPublicationPanic,
			outcome:     diagnostic.RefreshOutcomePanicInFlight,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sink := &diagnostic.MemorySink{}
			recorder, err := diagnostic.NewRecorder(diagnostic.RecorderConfig{
				QueueCapacity: 2, MaxMembers: 32, MaxRetainedBytes: 1 << 20, Sink: sink,
			})
			require.NoError(t, err)
			collector, store := newTestSNMPTopologyCollectorWithStore()
			store.Register("device-a", ddsnmp.DeviceConnectionInfo{
				Hostname: "192.0.2.10", Port: 161, SNMPVersion: "2c",
			})
			collector.diagnosticRecorder = recorder
			tc.run(collector)
			recorder.Close()

			result := diagnosticResultForCapability(t, sink.Results(), diagnostic.RefreshCapabilityV1())
			registry := diagnostic.NewRegistry()
			require.NoError(t, registry.Register(diagnostic.RefreshCapabilityV1(), diagnostic.RefreshClosureV1()))
			report, err := registry.ValidateCapability(
				result.Manifest, sink.Source(), diagnostic.RefreshCapabilityV1(), topologyDiagnosticReaderLimits(),
			)
			require.NoError(t, err)
			assert.True(t, report.Completeness)
			assert.False(t, report.Replayable)
			assert.Equal(t, diagnostic.StateFailed, report.State)

			sweep, root := decodeDiagnosticRefreshSweep(t, result, sink.Source())
			assert.Equal(t, tc.publication, sweep.Publication.State)
			assert.Equal(t, diagnostic.GenerationReferenceNone, sweep.Publication.Generation.State)
			require.Len(t, sweep.Registrations, 1)
			assert.Equal(t, diagnostic.RefreshSelectionDue, sweep.Registrations[0].Selection)
			assert.Equal(t, tc.outcome, sweep.Registrations[0].Outcome)
			generationSection := diagnosticSection(root, diagnostic.RefreshSectionGeneration)
			assert.Equal(t, diagnostic.StateNotApplicable, generationSection.State)
			assert.Empty(t, generationSection.Members)
		})
	}
}

func TestDiagnosticRefreshSweep_GenerationInventoriesUnavailableAndNonRenderableState(t *testing.T) {
	publishedAt := time.Date(2026, time.August, 27, 16, 0, 0, 0, time.UTC)
	sink := &diagnostic.MemorySink{}
	recorder, err := diagnostic.NewRecorder(diagnostic.RecorderConfig{
		QueueCapacity: 2, MaxMembers: 64, MaxRetainedBytes: 4 << 20, Sink: sink,
	})
	require.NoError(t, err)

	semanticTxn, err := recorder.Begin(diagnostic.CapabilityKey{Name: "test_observation", Revision: 1})
	require.NoError(t, err)
	require.NoError(t, semanticTxn.DefineSection(diagnostic.SemanticSectionObservation, diagnostic.StateSuccess, 1))
	observationHandle, err := semanticTxn.AddOwned(
		diagnostic.SemanticSectionObservation,
		diagnostic.MemberType{Kind: diagnostic.KindObservation, Schema: diagnostic.SchemaV1},
		diagnostic.ObservationV1{
			CaptureID: semanticTxn.CaptureID(), Registration: 1,
			LocalDeviceID: "device-a", CollectedAt: canonicalDiagnosticTime(publishedAt.Add(-time.Minute)),
		},
		512,
	)
	require.NoError(t, err)
	require.NoError(t, semanticTxn.Commit(diagnostic.StateSuccess))

	entries := []ddsnmp.DeviceEntry{
		{RegistrationID: 1, Info: ddsnmp.DeviceConnectionInfo{Hostname: "192.0.2.1", Port: 161, SNMPVersion: "2c"}},
		{RegistrationID: 2, Info: ddsnmp.DeviceConnectionInfo{Hostname: "192.0.2.2", Port: 161, SNMPVersion: "2c"}},
		{RegistrationID: 3, Info: ddsnmp.DeviceConnectionInfo{Hostname: "192.0.2.3", Port: 161, SNMPVersion: "2c"}},
		{RegistrationID: 4, Info: ddsnmp.DeviceConnectionInfo{Hostname: "192.0.2.4", Port: 161, SNMPVersion: "2c"}},
	}
	sweep := newTopologyDiagnosticRefreshSweep(recorder, publishedAt.Add(-time.Second), entries, nil)
	sweep.markDue(0)
	sweep.setTargetResolution(0, diagnostic.TargetResolutionLiteral)
	sweep.complete(0, diagnostic.RefreshOutcomeSuccess)

	deviceA := &topologyDeviceGeneration{
		registrationID: 1, collectedAt: publishedAt.Add(-time.Minute), expiresAt: publishedAt.Add(time.Hour),
		hasObservation: true,
		observation: topologymodel.ObservationSnapshot{
			LocalDeviceID: "device-a", CollectedAt: publishedAt.Add(-time.Minute),
		},
		diagnosticObservation: observationHandle,
	}
	deviceB := &topologyDeviceGeneration{
		registrationID: 2, collectedAt: publishedAt.Add(-time.Minute), expiresAt: publishedAt.Add(time.Hour),
		hasObservation: true,
		observation: topologymodel.ObservationSnapshot{
			LocalDeviceID: "device-b", CollectedAt: publishedAt.Add(-time.Minute),
		},
	}
	deviceC := &topologyDeviceGeneration{
		registrationID: 3, collectedAt: publishedAt.Add(-2 * time.Hour), expiresAt: publishedAt.Add(-time.Hour),
		hasObservation: true,
	}
	states := map[ddsnmp.DeviceRegistrationID]deviceRefreshState{
		1: {generation: deviceA},
		2: {generation: deviceB},
		3: {generation: deviceC},
		4: {},
	}
	generation := &topologyGeneration{
		sequence: 1, publishedAt: publishedAt,
		devices:           []*topologyDeviceGeneration{deviceA, deviceB, deviceC},
		renderableDevices: []*topologyDeviceGeneration{deviceA, deviceB},
	}
	collector := newTestSNMPTopologyCollector()
	generation.diagnosticMember = sweep.finishPublished(
		collector,
		publishedAt,
		generation,
		states,
		map[ddsnmp.DeviceRegistrationID]*topologyDeviceSnapshot{1: {}},
	)
	require.NotZero(t, generation.diagnosticMember.ID())
	recorder.Close()

	result := diagnosticResultForCapability(t, sink.Results(), diagnostic.RefreshCapabilityV1())
	registry := diagnostic.NewRegistry()
	require.NoError(t, registry.Register(diagnostic.RefreshCapabilityV1(), diagnostic.RefreshClosureV1()))
	report, err := registry.ValidateCapability(
		result.Manifest, sink.Source(), diagnostic.RefreshCapabilityV1(), topologyDiagnosticReaderLimits(),
	)
	require.NoError(t, err)
	assert.True(t, report.Completeness)

	_, root := decodeDiagnosticRefreshSweep(t, result, sink.Source())
	section := diagnosticSection(root, diagnostic.RefreshSectionGeneration)
	var captured diagnostic.GenerationV1
	require.NoError(t, diagnostic.DecodeReferenced(
		sink.Source(), section.Members[0], topologyDiagnosticReaderLimits(), &captured,
	))
	require.Len(t, captured.Devices, 4)
	assert.Equal(t, diagnostic.GenerationStateRefreshed, captured.Devices[0].State)
	assert.Equal(t, diagnostic.ObservationStateAvailable, captured.Devices[0].ObservationState)
	assert.Equal(t, diagnostic.GenerationStateRetained, captured.Devices[1].State)
	assert.Equal(t, diagnostic.ObservationStateUnavailable, captured.Devices[1].ObservationState)
	assert.Equal(t, diagnostic.GenerationStateExpired, captured.Devices[2].State)
	assert.Equal(t, diagnostic.ObservationStateNotApplicable, captured.Devices[2].ObservationState)
	assert.Equal(t, diagnostic.GenerationStateAbsent, captured.Devices[3].State)
	assert.False(t, captured.Replayable())
}

func TestDiagnosticGenerationBaseOrdersEqualObservationKeysByRegistration(t *testing.T) {
	collectedAt := time.Date(2026, time.August, 27, 16, 0, 0, 0, time.UTC)
	sink := &diagnostic.MemorySink{}
	recorder, err := diagnostic.NewRecorder(diagnostic.RecorderConfig{
		QueueCapacity: 1, MaxMembers: 8, MaxRetainedBytes: 1 << 20, Sink: sink,
	})
	require.NoError(t, err)
	txn, err := recorder.Begin(diagnostic.CapabilityKey{Name: "test_observations", Revision: 1})
	require.NoError(t, err)
	require.NoError(t, txn.DefineSection("observations", diagnostic.StateSuccess, 2))
	first, err := txn.AddOwned("observations", diagnostic.MemberType{Kind: diagnostic.KindObservation, Schema: diagnostic.SchemaV1},
		diagnostic.ObservationV1{CaptureID: txn.CaptureID(), Registration: 1, LocalDeviceID: "same", CollectedAt: canonicalDiagnosticTime(collectedAt)}, 128)
	require.NoError(t, err)
	second, err := txn.AddOwned("observations", diagnostic.MemberType{Kind: diagnostic.KindObservation, Schema: diagnostic.SchemaV1},
		diagnostic.ObservationV1{CaptureID: txn.CaptureID(), Registration: 2, LocalDeviceID: "same", CollectedAt: canonicalDiagnosticTime(collectedAt)}, 128)
	require.NoError(t, err)
	require.NoError(t, txn.Commit(diagnostic.StateSuccess))

	entries := []ddsnmp.DeviceEntry{{RegistrationID: 1}, {RegistrationID: 2}}
	sweep := newTopologyDiagnosticRefreshSweep(recorder, collectedAt, entries, nil)
	device1 := &topologyDeviceGeneration{
		registrationID: 1, hasObservation: true, diagnosticObservation: first,
		observation: topologymodel.ObservationSnapshot{LocalDeviceID: "same", CollectedAt: collectedAt},
	}
	device2 := &topologyDeviceGeneration{
		registrationID: 2, hasObservation: true, diagnosticObservation: second,
		observation: topologymodel.ObservationSnapshot{LocalDeviceID: "same", CollectedAt: collectedAt},
	}
	generation := &topologyGeneration{
		sequence: 1, publishedAt: collectedAt,
		renderableDevices: []*topologyDeviceGeneration{device2, device1},
	}
	states := map[ddsnmp.DeviceRegistrationID]deviceRefreshState{
		1: {generation: device1},
		2: {generation: device2},
	}
	_, dependencies, rows := sweep.generationBase(newTestSNMPTopologyCollector(), generation, states, nil)
	require.Equal(t, []uint64{first.ID(), second.ID()}, []uint64{dependencies[0].ID(), dependencies[1].ID()})
	require.Equal(t, []int{0, 1}, rows)
	recorder.Close()
}

func decodeDiagnosticRefreshSweep(
	t *testing.T,
	result diagnostic.CaptureResult,
	source diagnostic.MemberSource,
) (diagnostic.RefreshSweepV1, diagnostic.CapabilityRootV1) {
	t.Helper()
	var root diagnostic.CapabilityRootV1
	require.NoError(t, diagnostic.DecodeReferenced(
		source, result.Manifest.Roots[0].Root, topologyDiagnosticReaderLimits(), &root,
	))
	section := diagnosticSection(root, diagnostic.RefreshSectionSweep)
	var sweep diagnostic.RefreshSweepV1
	require.NoError(t, diagnostic.DecodeReferenced(source, section.Members[0], topologyDiagnosticReaderLimits(), &sweep))
	return sweep, root
}
