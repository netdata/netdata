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
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologydiag"
	"github.com/stretchr/testify/require"
)

func TestCollectorDiagnosticsReadsLifecycleIndependently(t *testing.T) {
	coll, store := newTestSNMPTopologyCollectorWithStore()
	store.RegisterJob("job-a", ddsnmp.DeviceLifecycleInfo{Hostname: "192.0.2.10", Port: 161})

	first, err := coll.diagnosticProvider.Capture()
	require.NoError(t, err)
	require.Equal(t, "available", first.Lifecycle.State)
	require.Len(t, first.Lifecycle.Cut.Entries, 1)
	require.Nil(t, first.Topology)

	store.RecordJobLifecycle("job-a", ddsnmp.DeviceLifecycleStatus{
		Phase:       ddsnmp.DeviceLifecyclePhaseInit,
		Outcome:     ddsnmp.DeviceLifecycleOutcomeFailed,
		CompletedAt: time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC),
	})
	second, err := coll.diagnosticProvider.Capture()
	require.NoError(t, err)
	require.Greater(t, second.Lifecycle.Cut.Sequence, first.Lifecycle.Cut.Sequence)
	require.Equal(t, "failed", second.Lifecycle.Cut.Entries[0].LastCompleted.Outcome)
	require.Equal(t, first.Topology, second.Topology)
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
	require.NotNil(t, diagnostics.Topology)
	require.Equal(t, topologydiag.CaptureAvailable, diagnostics.Topology.CaptureState)
	require.Len(t, diagnostics.Topology.Devices, 1)
	row := diagnostics.Topology.Devices[0]
	require.Equal(t, registrationID, row.RegistrationID)
	require.True(t, row.Selected)
	require.Equal(t, topologydiag.RefreshOutcomeSuccess, row.Outcome)
	require.Equal(t, coll.deviceStates[registrationID].nextRetry, row.NextRetry)
	require.True(t, row.HasRetainedSuccess)
	require.Equal(t, coll.deviceStates[registrationID].generation.evidenceRef, row.RetainedSuccess)
	require.Equal(t, topologydiag.CaptureAvailable, row.Acquisition.State)
	require.Same(t, row.Acquisition, row.LatestAttempt)
	require.True(t, row.HasObservation)
	require.True(t, row.Renderable)
	require.False(t, row.Expired)

	previousRef := row.RetainedSuccess
	base = base.Add(time.Minute)
	stats = coll.refreshTopology(context.Background())
	require.Zero(t, stats.errors)
	diagnostics = coll.acquireTopologyDiagnostics()
	require.Len(t, diagnostics.Topology.Devices, 1)
	row = diagnostics.Topology.Devices[0]
	require.False(t, row.Selected)
	require.True(t, row.HasRetainedSuccess)
	require.Equal(t, previousRef, row.RetainedSuccess)

	previousCut := diagnostics.Topology
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stats = coll.refreshTopology(ctx)
	require.Zero(t, stats.errors)
	diagnostics = coll.acquireTopologyDiagnostics()
	require.Same(t, previousCut, diagnostics.Topology)
	require.NotNil(t, diagnostics.LastAborted)
	require.Equal(t, topologydiag.DiagnosticAbortCanceled, diagnostics.LastAborted.Reason)
	require.Equal(t, topologydiag.DiagnosticSweepPhaseTargetResolution, diagnostics.LastAborted.Phase)
	require.False(t, diagnostics.LastAborted.HasActiveRegistration)
}

func TestCollectorPanicDiagnosticIdentifiesActiveDeviceRefresh(t *testing.T) {
	coll, store := newTestSNMPTopologyCollectorWithStore()
	store.Register("job-a", ddsnmp.DeviceConnectionInfo{Hostname: "192.0.2.10"})
	registrationID := store.Entries()[0].RegistrationID
	coll.newSnmpClient = func() gosnmp.Handler { panic("device refresh panic") }

	require.NotPanics(t, func() { coll.refreshTopologyRecovering(context.Background()) })

	diagnostics := coll.acquireTopologyDiagnostics()
	require.NotNil(t, diagnostics.LastAborted)
	require.Equal(t, topologydiag.DiagnosticAbortPanic, diagnostics.LastAborted.Reason)
	require.Equal(t, topologydiag.DiagnosticSweepPhaseDeviceRefresh, diagnostics.LastAborted.Phase)
	require.True(t, diagnostics.LastAborted.HasActiveRegistration)
	require.Equal(t, registrationID, diagnostics.LastAborted.ActiveRegistrationID)
	require.Equal(t, 1, diagnostics.LastAborted.RegistrationCount)
	require.Equal(t, 1, diagnostics.LastAborted.SelectedCount)
}

func TestCollectorDiagnosticProjectionFailureDoesNotAffectTopologyCommit(t *testing.T) {
	for name, tc := range map[string]struct {
		projector topologyDiagnosticCutProjector
		reason    topologydiag.CaptureReason
	}{
		"error": {
			projector: func(topologyDiagnosticCutInput) (*topologydiag.SweepCut, error) {
				return nil, errors.New("projection failed")
			},
			reason: topologydiag.CaptureReasonProjectionError,
		},
		"panic": {
			projector: func(topologyDiagnosticCutInput) (*topologydiag.SweepCut, error) {
				panic("projection failed")
			},
			reason: topologydiag.CaptureReasonProjectionPanic,
		},
	} {
		t.Run(name, func(t *testing.T) {
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
			require.NotNil(t, diagnostics.Topology)
			require.Equal(t, topologydiag.CaptureUnavailable, diagnostics.Topology.CaptureState)
			require.Equal(t, tc.reason, diagnostics.Topology.CaptureReason)
		})
	}
}

func TestProjectTopologyDiagnosticCutMarksExpiredRetainedGeneration(t *testing.T) {
	base := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	registrationID := ddsnmp.DeviceRegistrationID(1)
	generation := &topologyDeviceGeneration{
		registrationID: registrationID,
		evidenceRef:    topologydiag.EvidenceRef{RegistrationID: registrationID, Generation: 1},
		collectedAt:    base.Add(-time.Hour),
		expiresAt:      base.Add(-time.Minute),
		hasObservation: true,
		acquisition:    &topologydiag.AcquisitionCapture{State: topologydiag.CaptureAvailable},
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
	})
	require.NoError(t, err)
	require.Len(t, cut.Devices, 1)
	require.True(t, cut.Devices[0].Expired)
	require.False(t, cut.Devices[0].Renderable)
	require.True(t, cut.Devices[0].HasRetainedSuccess)
}

func TestAcquireTopologyDiagnosticsContainsLifecyclePanic(t *testing.T) {
	coll := newTestSNMPTopologyCollector()
	coll.diagnosticProvider.source = panickingTopologyLifecycleSource{}

	diagnostics, err := coll.diagnosticProvider.Capture()
	require.NoError(t, err)
	require.Equal(t, "unavailable", diagnostics.Lifecycle.State)
	require.Equal(t, "projection_panic", diagnostics.Lifecycle.Reason)
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
			cut := &topologydiag.SweepCut{
				Sequence:     uint64(i + 1),
				CaptureState: topologydiag.CaptureAvailable,
				RecordCount:  1,
				LogicalBytes: 32,
			}
			coll.topologyRegistry.publishGeneration(&topologyGeneration{sequence: uint64(i + 1), diagnostic: cut})
		}
	}()
	go func() {
		defer wg.Done()
		for range iterations {
			diagnostics, err := coll.diagnosticProvider.Capture()
			if err != nil {
				t.Error(err)
				return
			}
			if diagnostics.Lifecycle.State != "available" {
				t.Errorf("lifecycle capture state = %s", diagnostics.Lifecycle.State)
				return
			}
			if diagnostics.Topology != nil {
				if diagnostics.Topology.Sequence == 0 || diagnostics.Topology.CaptureState != "available" {
					t.Errorf("invalid topology cut: sequence=%d state=%s", diagnostics.Topology.Sequence, diagnostics.Topology.CaptureState)
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
