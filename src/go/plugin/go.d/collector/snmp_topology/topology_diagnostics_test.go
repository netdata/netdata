// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
)

func TestCollectorDiagnosticsReadsLifecycleIndependently(t *testing.T) {
	coll, store := newTestSNMPTopologyCollectorWithStore()
	store.RegisterJob("job-a", ddsnmp.DeviceLifecycleInfo{Hostname: "192.0.2.10", Port: 161})

	first := coll.acquireTopologyDiagnostics()
	require.Equal(t, diagnosticCaptureAvailable, first.lifecycle.state)
	require.Len(t, first.lifecycle.cut.Entries, 1)
	require.Nil(t, first.topology)

	store.RecordJobLifecycle("job-a", ddsnmp.DeviceLifecycleStatus{
		Phase:       ddsnmp.DeviceLifecyclePhaseInit,
		Outcome:     ddsnmp.DeviceLifecycleOutcomeFailed,
		CompletedAt: time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC),
	})
	second := coll.acquireTopologyDiagnostics()
	require.Greater(t, second.lifecycle.cut.Sequence, first.lifecycle.cut.Sequence)
	require.Equal(t, ddsnmp.DeviceLifecycleOutcomeFailed, second.lifecycle.cut.Entries[0].LastCompleted.Outcome)
	require.Same(t, first.topology, second.topology)
}

func TestCollectorDiagnosticsPublishesCommittedSweepCut(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dev := ddsnmp.DeviceConnectionInfo{
		Hostname:    "192.0.2.10",
		Port:        161,
		SNMPVersion: gosnmp.Version2c.String(),
	}
	mockHandler := snmpmock.NewMockHandler(ctrl)
	expectTopologyRefreshSNMPClient(mockHandler, dev)

	coll, store := newTestSNMPTopologyCollectorWithStore()
	store.Register("job-a", dev)
	registrationID := store.Entries()[0].RegistrationID
	coll.topologyProfiles = func(ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile { return []*ddsnmp.Profile{{}} }
	coll.newSnmpClient = func() gosnmp.Handler { return mockHandler }
	coll.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		return ddCollectorFunc(func() ([]*ddsnmp.ProfileMetrics, error) { return nil, nil })
	}
	base := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	coll.now = func() time.Time { return base }

	stats := coll.refreshTopology(context.Background())
	require.Zero(t, stats.errors)
	diagnostics := coll.acquireTopologyDiagnostics()
	require.NotNil(t, diagnostics.topology)
	require.Equal(t, diagnosticCaptureAvailable, diagnostics.topology.captureState)
	require.Len(t, diagnostics.topology.devices, 1)
	row := diagnostics.topology.devices[0]
	require.Equal(t, registrationID, row.registrationID)
	require.True(t, row.selected)
	require.Equal(t, deviceRefreshOutcomeSuccess, row.outcome)
	require.Equal(t, coll.deviceStates[registrationID].nextRetry, row.nextRetry)
	require.True(t, row.hasRetainedSuccess)
	require.Equal(t, coll.deviceStates[registrationID].generation.evidenceRef, row.retainedSuccess)
	require.Equal(t, diagnosticCaptureAvailable, row.semantic.state)
	require.True(t, row.hasObservation)
	require.True(t, row.renderable)
	require.False(t, row.expired)

	previousRef := row.retainedSuccess
	base = base.Add(time.Minute)
	stats = coll.refreshTopology(context.Background())
	require.Zero(t, stats.errors)
	diagnostics = coll.acquireTopologyDiagnostics()
	require.Len(t, diagnostics.topology.devices, 1)
	row = diagnostics.topology.devices[0]
	require.False(t, row.selected)
	require.True(t, row.hasRetainedSuccess)
	require.Equal(t, previousRef, row.retainedSuccess)

	previousCut := diagnostics.topology
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stats = coll.refreshTopology(ctx)
	require.Zero(t, stats.errors)
	diagnostics = coll.acquireTopologyDiagnostics()
	require.Same(t, previousCut, diagnostics.topology)
	require.NotNil(t, diagnostics.lastAborted)
	require.Equal(t, topologyDiagnosticAbortCanceled, diagnostics.lastAborted.reason)
	require.Equal(t, topologyDiagnosticSweepPhaseTargetResolution, diagnostics.lastAborted.phase)
	require.False(t, diagnostics.lastAborted.hasActiveRegistration)
}

func TestCollectorPanicDiagnosticIdentifiesActiveDeviceRefresh(t *testing.T) {
	coll, store := newTestSNMPTopologyCollectorWithStore()
	store.Register("job-a", ddsnmp.DeviceConnectionInfo{Hostname: "192.0.2.10"})
	registrationID := store.Entries()[0].RegistrationID
	coll.newSnmpClient = func() gosnmp.Handler { panic("device refresh panic") }

	require.NotPanics(t, func() { coll.refreshTopologyRecovering(context.Background()) })

	diagnostics := coll.acquireTopologyDiagnostics()
	require.NotNil(t, diagnostics.lastAborted)
	require.Equal(t, topologyDiagnosticAbortPanic, diagnostics.lastAborted.reason)
	require.Equal(t, topologyDiagnosticSweepPhaseDeviceRefresh, diagnostics.lastAborted.phase)
	require.True(t, diagnostics.lastAborted.hasActiveRegistration)
	require.Equal(t, registrationID, diagnostics.lastAborted.activeRegistrationID)
	require.Equal(t, 1, diagnostics.lastAborted.registrationCount)
	require.Equal(t, 1, diagnostics.lastAborted.selectedCount)
}

func TestCollectorDiagnosticProjectionFailureDoesNotAffectTopologyCommit(t *testing.T) {
	for _, tc := range []struct {
		name      string
		projector topologyDiagnosticCutProjector
		reason    diagnosticCaptureReason
	}{
		{
			name: "error",
			projector: func(topologyDiagnosticCutInput) (*topologySweepDiagnosticCut, error) {
				return nil, errors.New("projection failed")
			},
			reason: diagnosticCaptureReasonProjectionError,
		},
		{
			name: "panic",
			projector: func(topologyDiagnosticCutInput) (*topologySweepDiagnosticCut, error) {
				panic("projection failed")
			},
			reason: diagnosticCaptureReasonProjectionPanic,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			dev := ddsnmp.DeviceConnectionInfo{Hostname: "192.0.2.10", Port: 161, SNMPVersion: gosnmp.Version2c.String()}
			mockHandler := snmpmock.NewMockHandler(ctrl)
			expectTopologyRefreshSNMPClient(mockHandler, dev)

			coll, store := newTestSNMPTopologyCollectorWithStore()
			store.Register("job-a", dev)
			coll.projectTopologyDiagnosticCut = tc.projector
			coll.topologyProfiles = func(ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile { return []*ddsnmp.Profile{{}} }
			coll.newSnmpClient = func() gosnmp.Handler { return mockHandler }
			coll.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
				return ddCollectorFunc(func() ([]*ddsnmp.ProfileMetrics, error) { return nil, nil })
			}

			require.NotPanics(t, func() {
				stats := coll.refreshTopology(context.Background())
				require.Zero(t, stats.errors)
			})
			require.Equal(t, 1, coll.topologyRegistry.acquireGeneration().deviceCount())
			diagnostics := coll.acquireTopologyDiagnostics()
			require.NotNil(t, diagnostics.topology)
			require.Equal(t, diagnosticCaptureUnavailable, diagnostics.topology.captureState)
			require.Equal(t, tc.reason, diagnostics.topology.captureReason)
		})
	}
}

func TestTopologySemanticUsageBoundsRetainedEvidenceDeterministically(t *testing.T) {
	first := &topologyDeviceGeneration{semantic: topologySemanticCapture{
		state: diagnosticCaptureAvailable, recordCount: 6, logicalBytes: 60, evidence: &topologySemanticEvidence{},
	}}
	second := &topologyDeviceGeneration{semantic: topologySemanticCapture{
		state: diagnosticCaptureAvailable, recordCount: 6, logicalBytes: 60, evidence: &topologySemanticEvidence{},
	}}
	states := map[ddsnmp.DeviceRegistrationID]deviceRefreshState{
		2: {generation: second},
		1: {generation: first},
	}
	entries := []ddsnmp.DeviceEntry{{RegistrationID: 1}, {RegistrationID: 2}}
	seen := map[ddsnmp.DeviceRegistrationID]bool{1: true, 2: true}

	usage := newTopologySemanticUsage(entries, seen, nil, states, states, topologySemanticLimits{
		maxRecords:      10,
		maxLogicalBytes: 1000,
	})
	require.Equal(t, diagnosticCaptureAvailable, states[1].generation.semantic.state)
	require.Equal(t, diagnosticCaptureLimitExceeded, states[2].generation.semantic.state)
	require.Equal(t, diagnosticCaptureReasonGlobalRecordLimit, states[2].generation.semantic.reason)
	require.Nil(t, states[2].generation.semantic.evidence)
	require.NotSame(t, second, states[2].generation)

	require.Equal(t, topologySemanticLimits{maxRecords: 1, maxLogicalBytes: 652},
		usage.availableLimits(defaultTopologySemanticLimits))
}

func TestProjectTopologyDiagnosticCutMarksExpiredRetainedGeneration(t *testing.T) {
	base := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	registrationID := ddsnmp.DeviceRegistrationID(1)
	generation := &topologyDeviceGeneration{
		registrationID: registrationID,
		evidenceRef:    topologyEvidenceRef{registrationID: registrationID, generation: 1},
		collectedAt:    base.Add(-time.Hour),
		expiresAt:      base.Add(-time.Minute),
		hasObservation: true,
		semantic:       topologySemanticCapture{state: diagnosticCaptureAvailable},
	}
	cut, err := projectTopologyDiagnosticCut(topologyDiagnosticCutInput{
		sequence:    2,
		startedAt:   base,
		publishedAt: base,
		entries:     []ddsnmp.DeviceEntry{{RegistrationID: registrationID}},
		selected:    map[ddsnmp.DeviceRegistrationID]bool{},
		states: map[ddsnmp.DeviceRegistrationID]deviceRefreshState{
			registrationID: {generation: generation},
		},
		limits: defaultTopologyDiagnosticGlobalLimits,
	})
	require.NoError(t, err)
	require.Len(t, cut.devices, 1)
	require.True(t, cut.devices[0].expired)
	require.False(t, cut.devices[0].renderable)
	require.True(t, cut.devices[0].hasRetainedSuccess)
}

func TestAcquireTopologyDiagnosticsContainsLifecyclePanic(t *testing.T) {
	coll := newTestSNMPTopologyCollector()
	coll.deviceLifecycleSource = panickingTopologyLifecycleSource{}

	diagnostics := coll.acquireTopologyDiagnostics()
	require.Equal(t, diagnosticCaptureUnavailable, diagnostics.lifecycle.state)
	require.Equal(t, diagnosticCaptureReasonProjectionPanic, diagnostics.lifecycle.reason)
}

func TestAcquireTopologyDiagnosticsBoundsLifecycleCut(t *testing.T) {
	coll, store := newTestSNMPTopologyCollectorWithStore()
	coll.diagnosticGlobalLimits = topologySemanticLimits{maxRecords: 2, maxLogicalBytes: 1 << 20}
	store.RegisterJob("job-a", ddsnmp.DeviceLifecycleInfo{Hostname: "192.0.2.10"})
	store.RegisterJob("job-b", ddsnmp.DeviceLifecycleInfo{Hostname: "192.0.2.20"})

	diagnostics := coll.acquireTopologyDiagnostics()
	require.Equal(t, diagnosticCaptureLimitExceeded, diagnostics.lifecycle.state)
	require.Equal(t, diagnosticCaptureReasonGlobalRecordLimit, diagnostics.lifecycle.reason)
	require.NotZero(t, diagnostics.lifecycle.cut.Sequence)
	require.Empty(t, diagnostics.lifecycle.cut.Entries)
}

func TestCollectorDiagnosticCutLimitDoesNotAffectTopologyCommit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dev := ddsnmp.DeviceConnectionInfo{Hostname: "192.0.2.10", Port: 161, SNMPVersion: gosnmp.Version2c.String()}
	mockHandler := snmpmock.NewMockHandler(ctrl)
	expectTopologyRefreshSNMPClient(mockHandler, dev)

	coll, store := newTestSNMPTopologyCollectorWithStore()
	store.Register("job-a", dev)
	coll.diagnosticGlobalLimits = topologySemanticLimits{maxRecords: 10, maxLogicalBytes: 64}
	coll.topologyProfiles = func(ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile { return []*ddsnmp.Profile{{}} }
	coll.newSnmpClient = func() gosnmp.Handler { return mockHandler }
	coll.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		return ddCollectorFunc(func() ([]*ddsnmp.ProfileMetrics, error) { return nil, nil })
	}

	stats := coll.refreshTopology(context.Background())
	require.Zero(t, stats.errors)
	require.Equal(t, 1, coll.topologyRegistry.acquireGeneration().deviceCount())
	diagnostics := coll.acquireTopologyDiagnostics()
	require.NotNil(t, diagnostics.topology)
	require.Equal(t, diagnosticCaptureLimitExceeded, diagnostics.topology.captureState)
	require.Equal(t, diagnosticCaptureReasonByteLimit, diagnostics.topology.captureReason)
	require.Empty(t, diagnostics.topology.devices)
}

func TestCollectorDiagnosticsConcurrentLifecycleAndGenerationReads(t *testing.T) {
	coll, store := newTestSNMPTopologyCollectorWithStore()
	store.RegisterJob("job-a", ddsnmp.DeviceLifecycleInfo{Hostname: "192.0.2.10"})

	const iterations = 500
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range iterations {
			store.RecordJobLifecycle("job-a", ddsnmp.DeviceLifecycleStatus{
				Phase:       ddsnmp.DeviceLifecyclePhaseCollect,
				Outcome:     ddsnmp.DeviceLifecycleOutcomeSuccess,
				CompletedAt: time.Unix(int64(i+1), 0),
			})
			cut := &topologySweepDiagnosticCut{
				sequence:     uint64(i + 1),
				captureState: diagnosticCaptureAvailable,
				recordCount:  1,
				logicalBytes: 32,
			}
			coll.topologyRegistry.publishGeneration(&topologyGeneration{sequence: uint64(i + 1), diagnostic: cut})
		}
	}()
	go func() {
		defer wg.Done()
		for range iterations {
			diagnostics := coll.acquireTopologyDiagnostics()
			if diagnostics.lifecycle.state != diagnosticCaptureAvailable {
				t.Errorf("lifecycle capture state = %d", diagnostics.lifecycle.state)
				return
			}
			if diagnostics.topology != nil {
				if diagnostics.topology.sequence == 0 || diagnostics.topology.captureState != diagnosticCaptureAvailable {
					t.Errorf("invalid topology cut: sequence=%d state=%d", diagnostics.topology.sequence, diagnostics.topology.captureState)
					return
				}
			}
		}
	}()
	wg.Wait()
}

type panickingTopologyLifecycleSource struct{}

func (panickingTopologyLifecycleSource) LifecycleCut() ddsnmp.DeviceLifecycleCut {
	panic("lifecycle cut")
}
