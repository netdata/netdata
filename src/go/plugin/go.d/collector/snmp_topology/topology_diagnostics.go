// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"slices"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologydiag"
)

const (
	topologyDiagnosticCutLogicalBytes = 32
	topologyDiagnosticRowLogicalBytes = 128
)

type deviceLifecycleSource interface {
	LifecycleCut() ddsnmp.DeviceLifecycleCut
}

type topologyDiagnosticCutInput struct {
	sequence       uint64
	startedAt      time.Time
	publishedAt    time.Time
	entries        []ddsnmp.DeviceEntry
	selected       map[ddsnmp.DeviceRegistrationID]bool
	seen           map[ddsnmp.DeviceRegistrationID]bool
	previousStates map[ddsnmp.DeviceRegistrationID]deviceRefreshState
	states         map[ddsnmp.DeviceRegistrationID]deviceRefreshState
}

type topologyDiagnosticCutProjector func(topologyDiagnosticCutInput) (*topologydiag.SweepCut, error)

func captureTopologyCut(registry *topologyRegistry, aborted *topologydiag.AbortedSweep) topologydiag.Cut {
	diagnostics := topologydiag.Cut{LastAborted: aborted}
	if generation := registry.acquireGeneration(); generation != nil {
		diagnostics.ProducerScopeID = generation.producerScopeID
		diagnostics.Topology = generation.diagnostic
	}

	return diagnostics
}

func (c *Collector) projectCommittedTopologyDiagnosticCut(input topologyDiagnosticCutInput) (cut *topologydiag.SweepCut) {
	cut = unavailableTopologyDiagnosticCut(input, topologydiag.CaptureReasonProjectionError)
	defer func() {
		if recover() != nil {
			cut = unavailableTopologyDiagnosticCut(input, topologydiag.CaptureReasonProjectionPanic)
			c.Limit("snmp_topology:diagnostic-cut", 1, topologyRefreshWarningEvery).
				Warningf("failed to project SNMP topology diagnostics")
		}
	}()
	if c.projectTopologyDiagnosticCut == nil {
		return cut
	}
	projected, err := c.projectTopologyDiagnosticCut(input)
	if err != nil || projected == nil {
		c.Limit("snmp_topology:diagnostic-cut", 1, topologyRefreshWarningEvery).
			Warningf("failed to project SNMP topology diagnostics")
		return cut
	}
	return projected
}

func projectTopologyDiagnosticCut(input topologyDiagnosticCutInput) (*topologydiag.SweepCut, error) {
	cut := &topologydiag.SweepCut{
		Sequence:      input.sequence,
		StartedAt:     input.startedAt,
		PublishedAt:   input.publishedAt,
		CaptureState:  topologydiag.CaptureAvailable,
		CaptureReason: topologydiag.CaptureReasonNone,
	}

	seen := input.seen
	if seen == nil {
		seen = make(map[ddsnmp.DeviceRegistrationID]bool, len(input.entries))
		for _, entry := range input.entries {
			seen[entry.RegistrationID] = true
		}
	}
	removedIDs := make([]ddsnmp.DeviceRegistrationID, 0)
	for registrationID := range input.previousStates {
		if !seen[registrationID] {
			removedIDs = append(removedIDs, registrationID)
		}
	}
	slices.Sort(removedIDs)

	rowCount := uint64(len(input.entries) + len(removedIDs))
	records := 1 + rowCount
	logicalBytes := uint64(topologyDiagnosticCutLogicalBytes) + rowCount*topologyDiagnosticRowLogicalBytes

	countedCaptures := make(map[*topologydiag.AcquisitionCapture]bool)
	for _, entry := range input.entries {
		state := input.states[entry.RegistrationID]
		for _, capture := range stateAcquisitionCaptures(state) {
			if capture == nil || capture.State != topologydiag.CaptureAvailable || countedCaptures[capture] {
				continue
			}
			countedCaptures[capture] = true

			records += capture.RecordCount
			logicalBytes += capture.LogicalBytes
		}
	}
	cut.RecordCount = records
	cut.LogicalBytes = logicalBytes

	cut.Devices = make([]topologydiag.SweepDevice, 0, len(input.entries))
	for _, entry := range input.entries {
		registrationID := entry.RegistrationID
		state := input.states[registrationID]
		row := topologydiag.SweepDevice{
			RegistrationID: registrationID,
			Selected:       input.selected[registrationID],
			Outcome:        state.outcome,
			LastAttempt:    state.lastAttempt,
			LastSuccess:    state.lastSuccess,
			NextRetry:      state.nextRetry,
		}
		if generation := state.generation; generation != nil {
			row.RetainedSuccess = generation.evidenceRef
			row.HasRetainedSuccess = true
			row.Acquisition = generation.acquisition
			row.HasObservation = generation.hasObservation
			row.ExpiresAt = generation.expiresAt
			row.Renderable = generation.hasObservation && generation.freshAt(input.publishedAt)
			row.Expired = !generation.expiresAt.IsZero() && input.publishedAt.After(generation.expiresAt)
		}
		row.LatestAttempt = state.latestAttempt
		cut.Devices = append(cut.Devices, row)
	}

	cut.Removed = make([]topologydiag.RemovedDevice, 0, len(removedIDs))
	for _, registrationID := range removedIDs {
		row := topologydiag.RemovedDevice{RegistrationID: registrationID}
		if generation := input.previousStates[registrationID].generation; generation != nil {
			row.RetainedSuccess = generation.evidenceRef
			row.HasRetainedSuccess = true
		}
		cut.Removed = append(cut.Removed, row)
	}
	return cut, nil
}

func stateAcquisitionCaptures(state deviceRefreshState) []*topologydiag.AcquisitionCapture {
	success := acquisitionCaptureFromGeneration(state.generation)
	if success == nil {
		if state.latestAttempt == nil {
			return nil
		}
		return []*topologydiag.AcquisitionCapture{state.latestAttempt}
	}
	if state.latestAttempt == nil || state.latestAttempt == success {
		return []*topologydiag.AcquisitionCapture{success}
	}
	return []*topologydiag.AcquisitionCapture{success, state.latestAttempt}
}

func unavailableTopologyDiagnosticCut(input topologyDiagnosticCutInput, reason topologydiag.CaptureReason) *topologydiag.SweepCut {
	return &topologydiag.SweepCut{
		Sequence:      input.sequence,
		StartedAt:     input.startedAt,
		PublishedAt:   input.publishedAt,
		CaptureState:  topologydiag.CaptureUnavailable,
		CaptureReason: reason,
		RecordCount:   1,
		LogicalBytes:  topologyDiagnosticCutLogicalBytes,
	}
}

func (c *Collector) publishAbortedTopologyDiagnostic(
	startedAt time.Time,
	reason topologydiag.DiagnosticAbortReason,
	phase topologydiag.DiagnosticSweepPhase,
	activeRegistrationID ddsnmp.DeviceRegistrationID,
	hasActiveRegistration bool,
	registrationCount int,
	selectedCount int,
) {
	if c == nil {
		return
	}
	c.lastAbortedTopologyDiagnostic.Store(&topologydiag.AbortedSweep{
		Sequence:              c.topologyDiagnosticAbortSequence.Add(1),
		StartedAt:             startedAt,
		AbortedAt:             safeTopologyDiagnosticTime(c),
		Reason:                reason,
		Phase:                 phase,
		ActiveRegistrationID:  activeRegistrationID,
		HasActiveRegistration: hasActiveRegistration,
		RegistrationCount:     registrationCount,
		SelectedCount:         selectedCount,
	})
	c.recordDiagnosticCheckpoint()
}

func safeTopologyDiagnosticTime(c *Collector) (now time.Time) {
	now = time.Now()
	defer func() { _ = recover() }()
	if c != nil && c.now != nil {
		now = c.now()
	}
	return now
}
