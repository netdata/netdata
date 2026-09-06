// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
)

type topologyDiagnosticArchive struct {
	producerVersion string
	diagnostics     topologyDiagnostics
}

const (
	topologyDiagnosticArchiveCaptureRoleLatestAttempt   = "latest_attempt"
	topologyDiagnosticArchiveCaptureRoleRetainedSuccess = "retained_success"
)

func newTopologyDiagnosticArchiveSnapshotV1(
	diagnostics topologyDiagnostics,
) (snmpdiag.Snapshot, error) {
	lifecycle, err := newTopologyDiagnosticArchiveLifecycleV1(diagnostics.lifecycle)
	if err != nil {
		return snmpdiag.Snapshot{}, err
	}
	result := snmpdiag.Snapshot{
		Lifecycle:       lifecycle,
		ProducerScopeID: diagnostics.producerScopeID,
	}
	if diagnostics.topology != nil {
		cut, err := newTopologyDiagnosticArchiveSweepV1(diagnostics.topology)
		if err != nil {
			return snmpdiag.Snapshot{}, err
		}
		result.Topology = &cut
	}
	if diagnostics.lastAborted != nil {
		abort, err := newTopologyDiagnosticArchiveAbortV1(diagnostics.lastAborted)
		if err != nil {
			return snmpdiag.Snapshot{}, err
		}
		result.LastAborted = &abort
	}
	return result, nil
}

func newTopologyDiagnosticArchiveLifecycleV1(
	lifecycle topologyJobLifecycleDiagnosticCut,
) (snmpdiag.Lifecycle, error) {
	state, err := topologyDiagnosticArchiveCaptureStateName(lifecycle.state)
	if err != nil {
		return snmpdiag.Lifecycle{}, fmt.Errorf("job lifecycle capture state: %w", err)
	}
	reason, err := topologyDiagnosticArchiveCaptureReasonName(lifecycle.reason)
	if err != nil {
		return snmpdiag.Lifecycle{}, fmt.Errorf("job lifecycle capture reason: %w", err)
	}
	result, err := snmpdiag.NewLifecycle(lifecycle.cut)
	if err != nil {
		return snmpdiag.Lifecycle{}, err
	}
	result.State = state
	result.Reason = reason
	return result, nil
}

func newTopologyDiagnosticArchiveSweepV1(cut *topologySweepDiagnosticCut) (snmpdiag.Sweep, error) {
	state, err := topologyDiagnosticArchiveCaptureStateName(cut.captureState)
	if err != nil {
		return snmpdiag.Sweep{}, fmt.Errorf("topology sweep capture state: %w", err)
	}
	reason, err := topologyDiagnosticArchiveCaptureReasonName(cut.captureReason)
	if err != nil {
		return snmpdiag.Sweep{}, fmt.Errorf("topology sweep capture reason: %w", err)
	}
	result := snmpdiag.Sweep{
		Sequence:      cut.sequence,
		StartedAt:     cut.startedAt,
		PublishedAt:   cut.publishedAt,
		CaptureState:  state,
		CaptureReason: reason,
		RecordCount:   cut.recordCount,
		LogicalBytes:  cut.logicalBytes,
		Devices:       make([]snmpdiag.Device, 0, len(cut.devices)),
		Removed:       make([]snmpdiag.Removed, 0, len(cut.removed)),
	}
	for _, device := range cut.devices {
		row, err := newTopologyDiagnosticArchiveDeviceV1(device)
		if err != nil {
			return snmpdiag.Sweep{}, err
		}
		result.Devices = append(result.Devices, row)
	}
	for _, removed := range cut.removed {
		result.Removed = append(result.Removed, snmpdiag.Removed{
			RegistrationID:  uint64(removed.registrationID),
			RetainedSuccess: topologyDiagnosticArchiveEvidenceRef(removed.retainedSuccess, removed.hasRetainedSuccess),
		})
	}
	return result, nil
}

func newTopologyDiagnosticArchiveDeviceV1(
	device topologySweepDeviceDiagnostic,
) (snmpdiag.Device, error) {
	outcome, err := topologyDiagnosticArchiveDeviceOutcomeName(device.outcome)
	if err != nil {
		return snmpdiag.Device{}, fmt.Errorf("topology sweep registration %d outcome: %w", device.registrationID, err)
	}
	result := snmpdiag.Device{
		RegistrationID:  uint64(device.registrationID),
		Selected:        device.selected,
		Outcome:         outcome,
		LastAttempt:     device.lastAttempt,
		LastSuccess:     device.lastSuccess,
		NextRetry:       device.nextRetry,
		RetainedSuccess: topologyDiagnosticArchiveEvidenceRef(device.retainedSuccess, device.hasRetainedSuccess),
		HasObservation:  device.hasObservation,
		ExpiresAt:       device.expiresAt,
		Renderable:      device.renderable,
		Expired:         device.expired,
	}
	if device.acquisition != nil {
		capture, err := newTopologyDiagnosticArchiveCaptureV1(
			device.registrationID,
			device.acquisition,
			[]string{topologyDiagnosticArchiveCaptureRoleRetainedSuccess},
		)
		if err != nil {
			return snmpdiag.Device{}, err
		}
		result.Captures = append(result.Captures, capture)
	}
	if device.latestAttempt != nil {
		if device.latestAttempt == device.acquisition {
			result.Captures[0].Roles = append(
				[]string{topologyDiagnosticArchiveCaptureRoleLatestAttempt},
				result.Captures[0].Roles...,
			)
		} else {
			capture, err := newTopologyDiagnosticArchiveCaptureV1(
				device.registrationID,
				device.latestAttempt,
				[]string{topologyDiagnosticArchiveCaptureRoleLatestAttempt},
			)
			if err != nil {
				return snmpdiag.Device{}, err
			}
			result.Captures = append(result.Captures, capture)
		}
	}
	return result, nil
}

func newTopologyDiagnosticArchiveCaptureV1(
	registrationID ddsnmp.DeviceRegistrationID,
	capture *topologyAcquisitionCapture,
	roles []string,
) (snmpdiag.Capture, error) {
	state, err := topologyDiagnosticArchiveCaptureStateName(capture.state)
	if err != nil {
		return snmpdiag.Capture{}, fmt.Errorf("topology sweep registration %d capture state: %w", registrationID, err)
	}
	reason, err := topologyDiagnosticArchiveCaptureReasonName(capture.reason)
	if err != nil {
		return snmpdiag.Capture{}, fmt.Errorf("topology sweep registration %d capture reason: %w", registrationID, err)
	}
	if capture.attemptID.registrationID != registrationID {
		return snmpdiag.Capture{}, fmt.Errorf(
			"topology sweep registration %d capture belongs to registration %d",
			registrationID,
			capture.attemptID.registrationID,
		)
	}
	if capture.attemptID.ordinal == 0 {
		return snmpdiag.Capture{}, fmt.Errorf("topology sweep registration %d capture attempt ordinal is zero", registrationID)
	}
	if capture.state == diagnosticCaptureAvailable && capture.evidence == nil {
		return snmpdiag.Capture{}, fmt.Errorf("topology sweep registration %d available capture has no evidence", registrationID)
	}
	if capture.state != diagnosticCaptureAvailable && capture.evidence != nil {
		return snmpdiag.Capture{}, fmt.Errorf("topology sweep registration %d unavailable capture has evidence", registrationID)
	}
	result := snmpdiag.Capture{
		Roles:          roles,
		AttemptOrdinal: capture.attemptID.ordinal,
		State:          state,
		Reason:         reason,
		RecordCount:    capture.recordCount,
		LogicalBytes:   capture.logicalBytes,
	}
	if capture.evidence != nil {
		if capture.evidence.id != capture.attemptID {
			return snmpdiag.Capture{}, fmt.Errorf("topology sweep registration %d capture/evidence attempt mismatch", registrationID)
		}
		evidence, err := newTopologyDiagnosticArchiveAcquisitionEvidenceV1(capture.evidence)
		if err != nil {
			return snmpdiag.Capture{}, fmt.Errorf("topology sweep registration %d capture evidence: %w", registrationID, err)
		}
		result.Evidence = &evidence
	}
	return result, nil
}

func topologyDiagnosticArchiveEvidenceRef(
	ref topologyEvidenceRef,
	present bool,
) *snmpdiag.EvidenceRef {
	if !present {
		return nil
	}
	return &snmpdiag.EvidenceRef{
		RegistrationID: uint64(ref.registrationID),
		Generation:     ref.generation,
	}
}

func newTopologyDiagnosticArchiveAbortV1(
	abort *topologyAbortedSweepDiagnostic,
) (snmpdiag.Abort, error) {
	reason, err := topologyDiagnosticArchiveAbortReasonName(abort.reason)
	if err != nil {
		return snmpdiag.Abort{}, err
	}
	phase, err := topologyDiagnosticArchiveSweepPhaseName(abort.phase)
	if err != nil {
		return snmpdiag.Abort{}, err
	}
	return snmpdiag.Abort{
		Sequence:              abort.sequence,
		StartedAt:             abort.startedAt,
		AbortedAt:             abort.abortedAt,
		Reason:                reason,
		Phase:                 phase,
		ActiveRegistrationID:  uint64(abort.activeRegistrationID),
		HasActiveRegistration: abort.hasActiveRegistration,
		RegistrationCount:     abort.registrationCount,
		SelectedCount:         abort.selectedCount,
	}, nil
}

func restoreArchiveDocument(d snmpdiag.Document) (topologyDiagnostics, error) {
	if d.Format != snmpdiag.Format {
		return topologyDiagnostics{}, fmt.Errorf("unsupported format %q", d.Format)
	}
	if d.Version != snmpdiag.Version {
		return topologyDiagnostics{}, fmt.Errorf("unsupported version %d", d.Version)
	}
	return restoreArchiveSnapshot(d.Snapshot)
}

func restoreArchiveSnapshot(s snmpdiag.Snapshot) (topologyDiagnostics, error) {
	lifecycle, err := restoreArchiveLifecycle(s.Lifecycle)
	if err != nil {
		return topologyDiagnostics{}, err
	}
	result := topologyDiagnostics{
		lifecycle:       lifecycle,
		producerScopeID: s.ProducerScopeID,
	}
	if s.Topology != nil {
		cut, err := restoreArchiveSweep(*s.Topology)
		if err != nil {
			return topologyDiagnostics{}, err
		}
		result.topology = cut
	}
	if s.LastAborted != nil {
		abort, err := restoreArchiveAbort(*s.LastAborted)
		if err != nil {
			return topologyDiagnostics{}, err
		}
		result.lastAborted = abort
	}
	return result, nil
}

func restoreArchiveLifecycle(l snmpdiag.Lifecycle) (topologyJobLifecycleDiagnosticCut, error) {
	state, err := topologyDiagnosticArchiveParseCaptureState(l.State)
	if err != nil {
		return topologyJobLifecycleDiagnosticCut{}, fmt.Errorf("job lifecycle capture state: %w", err)
	}
	reason, err := topologyDiagnosticArchiveParseCaptureReason(l.Reason)
	if err != nil {
		return topologyJobLifecycleDiagnosticCut{}, fmt.Errorf("job lifecycle capture reason: %w", err)
	}
	result := topologyJobLifecycleDiagnosticCut{
		state:  state,
		reason: reason,
		cut: ddsnmp.DeviceLifecycleCut{
			Sequence:   l.Cut.Sequence,
			CapturedAt: l.Cut.CapturedAt,
			Entries:    make([]ddsnmp.DeviceLifecycleEntry, 0, len(l.Cut.Entries)),
		},
	}
	seen := make(map[ddsnmp.DeviceRegistrationID]struct{}, len(l.Cut.Entries))
	for _, entry := range l.Cut.Entries {
		registrationID := ddsnmp.DeviceRegistrationID(entry.RegistrationID)
		if registrationID == 0 {
			return topologyJobLifecycleDiagnosticCut{}, errors.New("job lifecycle registration ID is zero")
		}
		if _, ok := seen[registrationID]; ok {
			return topologyJobLifecycleDiagnosticCut{}, fmt.Errorf("duplicate job lifecycle registration ID %d", registrationID)
		}
		seen[registrationID] = struct{}{}
		phase, err := snmpdiag.ParseLifecyclePhase(entry.LastCompleted.Phase)
		if err != nil {
			return topologyJobLifecycleDiagnosticCut{}, fmt.Errorf("job lifecycle registration %d phase: %w", registrationID, err)
		}
		outcome, err := snmpdiag.ParseLifecycleOutcome(entry.LastCompleted.Outcome)
		if err != nil {
			return topologyJobLifecycleDiagnosticCut{}, fmt.Errorf("job lifecycle registration %d outcome: %w", registrationID, err)
		}
		result.cut.Entries = append(result.cut.Entries, ddsnmp.DeviceLifecycleEntry{
			RegistrationID: registrationID,
			Info: ddsnmp.DeviceLifecycleInfo{
				Hostname:    entry.Hostname,
				Port:        entry.Port,
				SNMPVersion: entry.SNMPVersion,
			},
			LastCompleted: ddsnmp.DeviceLifecycleStatus{
				Phase:       phase,
				Outcome:     outcome,
				CompletedAt: entry.LastCompleted.CompletedAt,
			},
			TopologyReady: entry.TopologyReady,
		})
	}
	return result, nil
}

func restoreArchiveSweep(s snmpdiag.Sweep) (*topologySweepDiagnosticCut, error) {
	state, err := topologyDiagnosticArchiveParseCaptureState(s.CaptureState)
	if err != nil {
		return nil, fmt.Errorf("topology sweep capture state: %w", err)
	}
	reason, err := topologyDiagnosticArchiveParseCaptureReason(s.CaptureReason)
	if err != nil {
		return nil, fmt.Errorf("topology sweep capture reason: %w", err)
	}
	result := &topologySweepDiagnosticCut{
		sequence:      s.Sequence,
		startedAt:     s.StartedAt,
		publishedAt:   s.PublishedAt,
		captureState:  state,
		captureReason: reason,
		recordCount:   s.RecordCount,
		logicalBytes:  s.LogicalBytes,
		devices:       make([]topologySweepDeviceDiagnostic, 0, len(s.Devices)),
		removed:       make([]topologyRemovedDeviceDiagnostic, 0, len(s.Removed)),
	}
	seen := make(map[ddsnmp.DeviceRegistrationID]struct{}, len(s.Devices)+len(s.Removed))
	for _, device := range s.Devices {
		row, err := restoreArchiveDevice(device, s.Sequence)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[row.registrationID]; ok {
			return nil, fmt.Errorf("duplicate topology sweep registration ID %d", row.registrationID)
		}
		seen[row.registrationID] = struct{}{}
		result.devices = append(result.devices, row)
	}
	for _, removed := range s.Removed {
		registrationID := ddsnmp.DeviceRegistrationID(removed.RegistrationID)
		if registrationID == 0 {
			return nil, errors.New("removed topology registration ID is zero")
		}
		if _, ok := seen[registrationID]; ok {
			return nil, fmt.Errorf("duplicate topology sweep registration ID %d", registrationID)
		}
		seen[registrationID] = struct{}{}
		ref, hasRef, err := restoreArchiveEvidenceRef(removed.RetainedSuccess, registrationID)
		if err != nil {
			return nil, fmt.Errorf("removed topology registration %d: %w", registrationID, err)
		}
		if hasRef && ref.generation > s.Sequence {
			return nil, fmt.Errorf(
				"removed topology registration %d retained-success generation %d exceeds sweep generation %d",
				registrationID,
				ref.generation,
				s.Sequence,
			)
		}
		result.removed = append(result.removed, topologyRemovedDeviceDiagnostic{
			registrationID:     registrationID,
			retainedSuccess:    ref,
			hasRetainedSuccess: hasRef,
		})
	}
	return result, nil
}

func restoreArchiveDevice(d snmpdiag.Device, sweepGeneration uint64) (topologySweepDeviceDiagnostic, error) {
	registrationID := ddsnmp.DeviceRegistrationID(d.RegistrationID)
	if registrationID == 0 {
		return topologySweepDeviceDiagnostic{}, errors.New("topology sweep registration ID is zero")
	}
	outcome, err := topologyDiagnosticArchiveParseDeviceOutcome(d.Outcome)
	if err != nil {
		return topologySweepDeviceDiagnostic{}, fmt.Errorf("topology sweep registration %d outcome: %w", registrationID, err)
	}
	ref, hasRef, err := restoreArchiveEvidenceRef(d.RetainedSuccess, registrationID)
	if err != nil {
		return topologySweepDeviceDiagnostic{}, fmt.Errorf("topology sweep registration %d: %w", registrationID, err)
	}
	result := topologySweepDeviceDiagnostic{
		registrationID:     registrationID,
		selected:           d.Selected,
		outcome:            outcome,
		lastAttempt:        d.LastAttempt,
		lastSuccess:        d.LastSuccess,
		nextRetry:          d.NextRetry,
		retainedSuccess:    ref,
		hasRetainedSuccess: hasRef,
		hasObservation:     d.HasObservation,
		expiresAt:          d.ExpiresAt,
		renderable:         d.Renderable,
		expired:            d.Expired,
	}
	seenRoles := make(map[string]struct{}, 2)
	seenAttemptOrdinals := make(map[uint64]struct{}, len(d.Captures))
	for _, archivedCapture := range d.Captures {
		capture, err := restoreArchiveCapture(archivedCapture, registrationID)
		if err != nil {
			return topologySweepDeviceDiagnostic{}, err
		}
		if _, ok := seenAttemptOrdinals[capture.attemptID.ordinal]; ok {
			return topologySweepDeviceDiagnostic{}, fmt.Errorf(
				"topology sweep registration %d duplicate capture attempt ordinal %d",
				registrationID,
				capture.attemptID.ordinal,
			)
		}
		seenAttemptOrdinals[capture.attemptID.ordinal] = struct{}{}
		if len(archivedCapture.Roles) == 0 {
			return topologySweepDeviceDiagnostic{}, fmt.Errorf("topology sweep registration %d capture has no role", registrationID)
		}
		for _, role := range archivedCapture.Roles {
			if _, ok := seenRoles[role]; ok {
				return topologySweepDeviceDiagnostic{}, fmt.Errorf("topology sweep registration %d duplicate capture role %q", registrationID, role)
			}
			seenRoles[role] = struct{}{}
			switch role {
			case topologyDiagnosticArchiveCaptureRoleLatestAttempt:
				result.latestAttempt = capture
			case topologyDiagnosticArchiveCaptureRoleRetainedSuccess:
				result.acquisition = capture
			default:
				return topologySweepDeviceDiagnostic{}, fmt.Errorf("topology sweep registration %d unknown capture role %q", registrationID, role)
			}
		}
	}
	if hasRef != (result.acquisition != nil) {
		return topologySweepDeviceDiagnostic{}, fmt.Errorf(
			"topology sweep registration %d retained-success reference and capture role disagree",
			registrationID,
		)
	}
	if hasRef && ref.generation > sweepGeneration {
		return topologySweepDeviceDiagnostic{}, fmt.Errorf(
			"topology sweep registration %d retained-success generation %d exceeds sweep generation %d",
			registrationID,
			ref.generation,
			sweepGeneration,
		)
	}
	if result.acquisition != nil && result.acquisition.state == diagnosticCaptureAvailable {
		if err := validateTopologyAcquisitionEvidence(result.acquisition.evidence); err != nil {
			return topologySweepDeviceDiagnostic{}, fmt.Errorf(
				"topology sweep registration %d retained-success evidence: %w",
				registrationID,
				err,
			)
		}
	}
	return result, nil
}

func restoreArchiveCapture(c snmpdiag.Capture,
	registrationID ddsnmp.DeviceRegistrationID,
) (*topologyAcquisitionCapture, error) {
	state, err := topologyDiagnosticArchiveParseCaptureState(c.State)
	if err != nil {
		return nil, fmt.Errorf("topology sweep registration %d capture state: %w", registrationID, err)
	}
	reason, err := topologyDiagnosticArchiveParseCaptureReason(c.Reason)
	if err != nil {
		return nil, fmt.Errorf("topology sweep registration %d capture reason: %w", registrationID, err)
	}
	attemptID := topologyAcquisitionAttemptID{registrationID: registrationID, ordinal: c.AttemptOrdinal}
	if attemptID.ordinal == 0 {
		return nil, fmt.Errorf("topology sweep registration %d capture attempt ordinal is zero", registrationID)
	}
	result := &topologyAcquisitionCapture{
		attemptID:    attemptID,
		state:        state,
		reason:       reason,
		recordCount:  c.RecordCount,
		logicalBytes: c.LogicalBytes,
	}
	if c.Evidence != nil {
		evidence, err := restoreArchiveAcquisitionEvidence(*c.Evidence, attemptID)
		if err != nil {
			return nil, fmt.Errorf("topology sweep registration %d capture evidence: %w", registrationID, err)
		}
		result.evidence = evidence
	}
	if state == diagnosticCaptureAvailable && result.evidence == nil {
		return nil, fmt.Errorf("topology sweep registration %d available capture has no evidence", registrationID)
	}
	if state != diagnosticCaptureAvailable && result.evidence != nil {
		return nil, fmt.Errorf("topology sweep registration %d unavailable capture has evidence", registrationID)
	}
	return result, nil
}

func restoreArchiveEvidenceRef(r *snmpdiag.EvidenceRef,
	owner ddsnmp.DeviceRegistrationID,
) (topologyEvidenceRef, bool, error) {
	if r == nil {
		return topologyEvidenceRef{}, false, nil
	}
	registrationID := ddsnmp.DeviceRegistrationID(r.RegistrationID)
	if registrationID == 0 || registrationID != owner {
		return topologyEvidenceRef{}, false, fmt.Errorf("retained-success registration reference %d does not match owner %d", registrationID, owner)
	}
	if r.Generation == 0 {
		return topologyEvidenceRef{}, false, errors.New("retained-success generation is zero")
	}
	return topologyEvidenceRef{registrationID: registrationID, generation: r.Generation}, true, nil
}

func restoreArchiveAbort(a snmpdiag.Abort) (*topologyAbortedSweepDiagnostic, error) {
	reason, err := topologyDiagnosticArchiveParseAbortReason(a.Reason)
	if err != nil {
		return nil, err
	}
	phase, err := topologyDiagnosticArchiveParseSweepPhase(a.Phase)
	if err != nil {
		return nil, err
	}
	registrationID := ddsnmp.DeviceRegistrationID(a.ActiveRegistrationID)
	if a.HasActiveRegistration && registrationID == 0 {
		return nil, errors.New("aborted sweep active registration ID is zero")
	}
	if !a.HasActiveRegistration && registrationID != 0 {
		return nil, errors.New("aborted sweep has an inactive registration reference")
	}
	if a.RegistrationCount < 0 || a.SelectedCount < 0 || a.SelectedCount > a.RegistrationCount {
		return nil, errors.New("aborted sweep registration counts are invalid")
	}
	return &topologyAbortedSweepDiagnostic{
		sequence:              a.Sequence,
		startedAt:             a.StartedAt,
		abortedAt:             a.AbortedAt,
		reason:                reason,
		phase:                 phase,
		activeRegistrationID:  registrationID,
		hasActiveRegistration: a.HasActiveRegistration,
		registrationCount:     a.RegistrationCount,
		selectedCount:         a.SelectedCount,
	}, nil
}

var (
	topologyDiagnosticArchiveCaptureStateNames = []string{
		"unknown", "available", "limit_exceeded", "unavailable",
	}
	topologyDiagnosticArchiveCaptureReasonNames = []string{
		"none", "record_limit", "byte_limit", "projection_error", "projection_panic",
		"global_record_limit", "global_byte_limit",
	}
	topologyDiagnosticArchiveDeviceOutcomeNames = []string{
		"unknown", "success", "no_profiles", "failed",
	}
	topologyDiagnosticArchiveAbortReasonNames = []string{
		"unknown", "canceled", "panic",
	}
	topologyDiagnosticArchiveSweepPhaseNames = []string{
		"unknown", "registration_cut", "target_resolution", "device_refresh", "commit",
	}
)

func topologyDiagnosticArchiveEnumName[T ~uint8](value T, names []string) (string, error) {
	index := int(value)
	if index < 0 || index >= len(names) {
		return "", fmt.Errorf("unknown value %d", value)
	}
	return names[index], nil
}

func topologyDiagnosticArchiveParseEnum[T ~uint8](value string, names []string) (T, error) {
	for index, name := range names {
		if value == name {
			return T(index), nil
		}
	}
	return 0, fmt.Errorf("unknown value %q", value)
}

func topologyDiagnosticArchiveCaptureStateName(value diagnosticCaptureState) (string, error) {
	return topologyDiagnosticArchiveEnumName(value, topologyDiagnosticArchiveCaptureStateNames)
}

func topologyDiagnosticArchiveParseCaptureState(value string) (diagnosticCaptureState, error) {
	return topologyDiagnosticArchiveParseEnum[diagnosticCaptureState](value, topologyDiagnosticArchiveCaptureStateNames)
}

func topologyDiagnosticArchiveCaptureReasonName(value diagnosticCaptureReason) (string, error) {
	return topologyDiagnosticArchiveEnumName(value, topologyDiagnosticArchiveCaptureReasonNames)
}

func topologyDiagnosticArchiveParseCaptureReason(value string) (diagnosticCaptureReason, error) {
	return topologyDiagnosticArchiveParseEnum[diagnosticCaptureReason](value, topologyDiagnosticArchiveCaptureReasonNames)
}

func topologyDiagnosticArchiveDeviceOutcomeName(value deviceRefreshOutcome) (string, error) {
	return topologyDiagnosticArchiveEnumName(value, topologyDiagnosticArchiveDeviceOutcomeNames)
}

func topologyDiagnosticArchiveParseDeviceOutcome(value string) (deviceRefreshOutcome, error) {
	return topologyDiagnosticArchiveParseEnum[deviceRefreshOutcome](value, topologyDiagnosticArchiveDeviceOutcomeNames)
}

func topologyDiagnosticArchiveAbortReasonName(value topologyDiagnosticAbortReason) (string, error) {
	return topologyDiagnosticArchiveEnumName(value, topologyDiagnosticArchiveAbortReasonNames)
}

func topologyDiagnosticArchiveParseAbortReason(value string) (topologyDiagnosticAbortReason, error) {
	return topologyDiagnosticArchiveParseEnum[topologyDiagnosticAbortReason](value, topologyDiagnosticArchiveAbortReasonNames)
}

func topologyDiagnosticArchiveSweepPhaseName(value topologyDiagnosticSweepPhase) (string, error) {
	return topologyDiagnosticArchiveEnumName(value, topologyDiagnosticArchiveSweepPhaseNames)
}

func topologyDiagnosticArchiveParseSweepPhase(value string) (topologyDiagnosticSweepPhase, error) {
	return topologyDiagnosticArchiveParseEnum[topologyDiagnosticSweepPhase](value, topologyDiagnosticArchiveSweepPhaseNames)
}
