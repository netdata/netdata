// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"slices"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

const (
	topologyDiagnosticCutLogicalBytes = 32
	topologyDiagnosticRowLogicalBytes = 128
)

type deviceLifecycleSource interface {
	LifecycleCut() ddsnmp.DeviceLifecycleCut
}

type topologyJobLifecycleDiagnosticCut struct {
	state  diagnosticCaptureState
	reason diagnosticCaptureReason
	cut    ddsnmp.DeviceLifecycleCut
}

type topologySweepDeviceDiagnostic struct {
	registrationID ddsnmp.DeviceRegistrationID
	selected       bool
	outcome        deviceRefreshOutcome
	lastAttempt    time.Time
	lastSuccess    time.Time
	nextRetry      time.Time

	retainedSuccess    topologyEvidenceRef
	hasRetainedSuccess bool
	semantic           topologySemanticCapture
	hasObservation     bool
	expiresAt          time.Time
	renderable         bool
	expired            bool
}

type topologyRemovedDeviceDiagnostic struct {
	registrationID     ddsnmp.DeviceRegistrationID
	retainedSuccess    topologyEvidenceRef
	hasRetainedSuccess bool
}

type topologySweepDiagnosticCut struct {
	sequence      uint64
	startedAt     time.Time
	publishedAt   time.Time
	captureState  diagnosticCaptureState
	captureReason diagnosticCaptureReason
	recordCount   uint64
	logicalBytes  uint64
	devices       []topologySweepDeviceDiagnostic
	removed       []topologyRemovedDeviceDiagnostic
}

type topologyDiagnosticAbortReason uint8

const (
	topologyDiagnosticAbortUnknown topologyDiagnosticAbortReason = iota
	topologyDiagnosticAbortCanceled
	topologyDiagnosticAbortPanic
)

type topologyDiagnosticSweepPhase uint8

const (
	topologyDiagnosticSweepPhaseUnknown topologyDiagnosticSweepPhase = iota
	topologyDiagnosticSweepPhaseRegistrationCut
	topologyDiagnosticSweepPhaseTargetResolution
	topologyDiagnosticSweepPhaseDeviceRefresh
	topologyDiagnosticSweepPhaseCommit
)

type topologyAbortedSweepDiagnostic struct {
	sequence              uint64
	startedAt             time.Time
	abortedAt             time.Time
	reason                topologyDiagnosticAbortReason
	phase                 topologyDiagnosticSweepPhase
	activeRegistrationID  ddsnmp.DeviceRegistrationID
	hasActiveRegistration bool
	registrationCount     int
	selectedCount         int
}

type topologyDiagnostics struct {
	lifecycle   topologyJobLifecycleDiagnosticCut
	topology    *topologySweepDiagnosticCut
	lastAborted *topologyAbortedSweepDiagnostic
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
	limits         topologySemanticLimits
}

type topologyDiagnosticCutProjector func(topologyDiagnosticCutInput) (*topologySweepDiagnosticCut, error)

func (c *Collector) acquireTopologyDiagnostics() topologyDiagnostics {
	limits := c.currentTopologyDiagnosticGlobalLimits()
	diagnostics := topologyDiagnostics{lastAborted: c.lastAbortedTopologyDiagnostic.Load()}
	if generation := c.topologyRegistry.acquireGeneration(); generation != nil {
		diagnostics.topology = generation.diagnostic
	}
	if diagnostics.topology != nil {
		if diagnostics.topology.recordCount >= limits.maxRecords || diagnostics.topology.logicalBytes >= limits.maxLogicalBytes {
			limits = topologySemanticLimits{}
		} else {
			limits.maxRecords -= diagnostics.topology.recordCount
			limits.maxLogicalBytes -= diagnostics.topology.logicalBytes
		}
	}
	diagnostics.lifecycle = acquireTopologyJobLifecycleCut(c.deviceLifecycleSource, limits)
	return diagnostics
}

func acquireTopologyJobLifecycleCut(source deviceLifecycleSource, limits topologySemanticLimits) (result topologyJobLifecycleDiagnosticCut) {
	result.state = diagnosticCaptureAvailable
	defer func() {
		if recover() != nil {
			result = topologyJobLifecycleDiagnosticCut{
				state:  diagnosticCaptureUnavailable,
				reason: diagnosticCaptureReasonProjectionPanic,
			}
		}
	}()
	if source == nil {
		result.state = diagnosticCaptureUnavailable
		result.reason = diagnosticCaptureReasonProjectionError
		return result
	}
	result.cut = source.LifecycleCut()
	records := uint64(1 + len(result.cut.Entries))
	logicalBytes := uint64(topologyDiagnosticCutLogicalBytes)
	for _, entry := range result.cut.Entries {
		logicalBytes += uint64(64 + len(entry.Info.Hostname) + len(entry.Info.SNMPVersion))
	}
	if records > limits.maxRecords {
		result.state = diagnosticCaptureLimitExceeded
		result.reason = diagnosticCaptureReasonGlobalRecordLimit
		result.cut.Entries = nil
		return result
	}
	if logicalBytes > limits.maxLogicalBytes {
		result.state = diagnosticCaptureLimitExceeded
		result.reason = diagnosticCaptureReasonGlobalByteLimit
		result.cut.Entries = nil
	}
	return result
}

func (c *Collector) projectCommittedTopologyDiagnosticCut(input topologyDiagnosticCutInput) (cut *topologySweepDiagnosticCut) {
	cut = unavailableTopologyDiagnosticCut(input, diagnosticCaptureReasonProjectionError)
	defer func() {
		if recover() != nil {
			cut = unavailableTopologyDiagnosticCut(input, diagnosticCaptureReasonProjectionPanic)
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

func projectTopologyDiagnosticCut(input topologyDiagnosticCutInput) (*topologySweepDiagnosticCut, error) {
	cut := &topologySweepDiagnosticCut{
		sequence:      input.sequence,
		startedAt:     input.startedAt,
		publishedAt:   input.publishedAt,
		captureState:  diagnosticCaptureAvailable,
		captureReason: diagnosticCaptureReasonNone,
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
	if records > input.limits.maxRecords {
		cut.captureState = diagnosticCaptureLimitExceeded
		cut.captureReason = diagnosticCaptureReasonRecordLimit
		cut.recordCount = 1
		cut.logicalBytes = topologyDiagnosticCutLogicalBytes
		return cut, nil
	}
	if logicalBytes > input.limits.maxLogicalBytes {
		cut.captureState = diagnosticCaptureLimitExceeded
		cut.captureReason = diagnosticCaptureReasonByteLimit
		cut.recordCount = 1
		cut.logicalBytes = topologyDiagnosticCutLogicalBytes
		return cut, nil
	}
	for _, entry := range input.entries {
		generation := input.states[entry.RegistrationID].generation
		if generation == nil || generation.semantic.state != diagnosticCaptureAvailable {
			continue
		}
		if generation.semantic.recordCount > input.limits.maxRecords-records {
			cut.captureState = diagnosticCaptureLimitExceeded
			cut.captureReason = diagnosticCaptureReasonGlobalRecordLimit
			cut.recordCount = 1
			cut.logicalBytes = topologyDiagnosticCutLogicalBytes
			return cut, nil
		}
		if generation.semantic.logicalBytes > input.limits.maxLogicalBytes-logicalBytes {
			cut.captureState = diagnosticCaptureLimitExceeded
			cut.captureReason = diagnosticCaptureReasonGlobalByteLimit
			cut.recordCount = 1
			cut.logicalBytes = topologyDiagnosticCutLogicalBytes
			return cut, nil
		}
		records += generation.semantic.recordCount
		logicalBytes += generation.semantic.logicalBytes
	}
	cut.recordCount = records
	cut.logicalBytes = logicalBytes

	cut.devices = make([]topologySweepDeviceDiagnostic, 0, len(input.entries))
	for _, entry := range input.entries {
		registrationID := entry.RegistrationID
		state := input.states[registrationID]
		row := topologySweepDeviceDiagnostic{
			registrationID: registrationID,
			selected:       input.selected[registrationID],
			outcome:        state.outcome,
			lastAttempt:    state.lastAttempt,
			lastSuccess:    state.lastSuccess,
			nextRetry:      state.nextRetry,
		}
		if generation := state.generation; generation != nil {
			row.retainedSuccess = generation.evidenceRef
			row.hasRetainedSuccess = true
			row.semantic = generation.semantic
			row.hasObservation = generation.hasObservation
			row.expiresAt = generation.expiresAt
			row.renderable = generation.hasObservation && generation.freshAt(input.publishedAt)
			row.expired = !generation.expiresAt.IsZero() && input.publishedAt.After(generation.expiresAt)
		}
		cut.devices = append(cut.devices, row)
	}

	cut.removed = make([]topologyRemovedDeviceDiagnostic, 0, len(removedIDs))
	for _, registrationID := range removedIDs {
		row := topologyRemovedDeviceDiagnostic{registrationID: registrationID}
		if generation := input.previousStates[registrationID].generation; generation != nil {
			row.retainedSuccess = generation.evidenceRef
			row.hasRetainedSuccess = true
		}
		cut.removed = append(cut.removed, row)
	}
	return cut, nil
}

func unavailableTopologyDiagnosticCut(input topologyDiagnosticCutInput, reason diagnosticCaptureReason) *topologySweepDiagnosticCut {
	return &topologySweepDiagnosticCut{
		sequence:      input.sequence,
		startedAt:     input.startedAt,
		publishedAt:   input.publishedAt,
		captureState:  diagnosticCaptureUnavailable,
		captureReason: reason,
		recordCount:   1,
		logicalBytes:  topologyDiagnosticCutLogicalBytes,
	}
}

func (c *Collector) publishAbortedTopologyDiagnostic(
	startedAt time.Time,
	reason topologyDiagnosticAbortReason,
	phase topologyDiagnosticSweepPhase,
	activeRegistrationID ddsnmp.DeviceRegistrationID,
	hasActiveRegistration bool,
	registrationCount int,
	selectedCount int,
) {
	if c == nil {
		return
	}
	c.lastAbortedTopologyDiagnostic.Store(&topologyAbortedSweepDiagnostic{
		sequence:              c.topologyDiagnosticAbortSequence.Add(1),
		startedAt:             startedAt,
		abortedAt:             safeTopologyDiagnosticTime(c),
		reason:                reason,
		phase:                 phase,
		activeRegistrationID:  activeRegistrationID,
		hasActiveRegistration: hasActiveRegistration,
		registrationCount:     registrationCount,
		selectedCount:         selectedCount,
	})
}

func safeTopologyDiagnosticTime(c *Collector) (now time.Time) {
	now = time.Now()
	defer func() { _ = recover() }()
	if c != nil && c.now != nil {
		now = c.now()
	}
	return now
}
