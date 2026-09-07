// SPDX-License-Identifier: GPL-3.0-or-later

package topologydiag

import (
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
)

const (
	archiveCaptureRoleLatestAttempt   = "latest_attempt"
	archiveCaptureRoleRetainedSuccess = "retained_success"
)

func NewSnapshot(
	diagnostics Cut,
) (snmpdiag.Snapshot, error) {
	lifecycle, err := newArchiveLifecycleV1(diagnostics.Lifecycle)
	if err != nil {
		return snmpdiag.Snapshot{}, err
	}
	result := snmpdiag.Snapshot{
		Lifecycle:       lifecycle,
		ProducerScopeID: diagnostics.ProducerScopeID,
	}
	if diagnostics.Topology != nil {
		cut, err := newArchiveSweepV1(diagnostics.Topology)
		if err != nil {
			return snmpdiag.Snapshot{}, err
		}
		result.Topology = &cut
	}
	if diagnostics.LastAborted != nil {
		abort, err := newArchiveAbortV1(diagnostics.LastAborted)
		if err != nil {
			return snmpdiag.Snapshot{}, err
		}
		result.LastAborted = &abort
	}
	return result, nil
}

func newArchiveLifecycleV1(
	lifecycle LifecycleCut,
) (snmpdiag.Lifecycle, error) {
	state, err := archiveCaptureStateName(lifecycle.State)
	if err != nil {
		return snmpdiag.Lifecycle{}, fmt.Errorf("job lifecycle capture state: %w", err)
	}
	reason, err := archiveCaptureReasonName(lifecycle.Reason)
	if err != nil {
		return snmpdiag.Lifecycle{}, fmt.Errorf("job lifecycle capture reason: %w", err)
	}
	result, err := snmpdiag.NewLifecycle(lifecycle.Cut)
	if err != nil {
		return snmpdiag.Lifecycle{}, err
	}
	result.State = state
	result.Reason = reason
	return result, nil
}

func newArchiveSweepV1(cut *SweepCut) (snmpdiag.Sweep, error) {
	state, err := archiveCaptureStateName(cut.CaptureState)
	if err != nil {
		return snmpdiag.Sweep{}, fmt.Errorf("topology sweep capture state: %w", err)
	}
	reason, err := archiveCaptureReasonName(cut.CaptureReason)
	if err != nil {
		return snmpdiag.Sweep{}, fmt.Errorf("topology sweep capture reason: %w", err)
	}
	result := snmpdiag.Sweep{
		Sequence:      cut.Sequence,
		StartedAt:     cut.StartedAt,
		PublishedAt:   cut.PublishedAt,
		CaptureState:  state,
		CaptureReason: reason,
		RecordCount:   cut.RecordCount,
		LogicalBytes:  cut.LogicalBytes,
		Devices:       make([]snmpdiag.Device, 0, len(cut.Devices)),
		Removed:       make([]snmpdiag.Removed, 0, len(cut.Removed)),
	}
	for _, device := range cut.Devices {
		row, err := newArchiveDeviceV1(device)
		if err != nil {
			return snmpdiag.Sweep{}, err
		}
		result.Devices = append(result.Devices, row)
	}
	for _, removed := range cut.Removed {
		result.Removed = append(result.Removed, snmpdiag.Removed{
			RegistrationID:  uint64(removed.RegistrationID),
			RetainedSuccess: archiveEvidenceRef(removed.RetainedSuccess, removed.HasRetainedSuccess),
		})
	}
	return result, nil
}

func newArchiveDeviceV1(
	device SweepDevice,
) (snmpdiag.Device, error) {
	outcome, err := archiveDeviceOutcomeName(device.Outcome)
	if err != nil {
		return snmpdiag.Device{}, fmt.Errorf("topology sweep registration %d outcome: %w", device.RegistrationID, err)
	}
	result := snmpdiag.Device{
		RegistrationID:  uint64(device.RegistrationID),
		Selected:        device.Selected,
		Outcome:         outcome,
		LastAttempt:     device.LastAttempt,
		LastSuccess:     device.LastSuccess,
		NextRetry:       device.NextRetry,
		RetainedSuccess: archiveEvidenceRef(device.RetainedSuccess, device.HasRetainedSuccess),
		HasObservation:  device.HasObservation,
		ExpiresAt:       device.ExpiresAt,
		Renderable:      device.Renderable,
		Expired:         device.Expired,
	}
	if device.Acquisition != nil {
		capture, err := newArchiveCaptureV1(
			device.RegistrationID,
			device.Acquisition,
			[]string{archiveCaptureRoleRetainedSuccess},
		)
		if err != nil {
			return snmpdiag.Device{}, err
		}
		result.Captures = append(result.Captures, capture)
	}
	if device.LatestAttempt != nil {
		if device.LatestAttempt == device.Acquisition {
			result.Captures[0].Roles = append(
				[]string{archiveCaptureRoleLatestAttempt},
				result.Captures[0].Roles...,
			)
		} else {
			capture, err := newArchiveCaptureV1(
				device.RegistrationID,
				device.LatestAttempt,
				[]string{archiveCaptureRoleLatestAttempt},
			)
			if err != nil {
				return snmpdiag.Device{}, err
			}
			result.Captures = append(result.Captures, capture)
		}
	}
	return result, nil
}

func newArchiveCaptureV1(
	registrationID ddsnmp.DeviceRegistrationID,
	capture *AcquisitionCapture,
	roles []string,
) (snmpdiag.Capture, error) {
	state, err := archiveCaptureStateName(capture.State)
	if err != nil {
		return snmpdiag.Capture{}, fmt.Errorf("topology sweep registration %d capture state: %w", registrationID, err)
	}
	reason, err := archiveCaptureReasonName(capture.Reason)
	if err != nil {
		return snmpdiag.Capture{}, fmt.Errorf("topology sweep registration %d capture reason: %w", registrationID, err)
	}
	if capture.AttemptID.RegistrationID != registrationID {
		return snmpdiag.Capture{}, fmt.Errorf(
			"topology sweep registration %d capture belongs to registration %d",
			registrationID,
			capture.AttemptID.RegistrationID,
		)
	}
	if capture.AttemptID.Ordinal == 0 {
		return snmpdiag.Capture{}, fmt.Errorf("topology sweep registration %d capture attempt ordinal is zero", registrationID)
	}
	if capture.State == CaptureAvailable && capture.Evidence == nil {
		return snmpdiag.Capture{}, fmt.Errorf("topology sweep registration %d available capture has no evidence", registrationID)
	}
	if capture.State != CaptureAvailable && capture.Evidence != nil {
		return snmpdiag.Capture{}, fmt.Errorf("topology sweep registration %d unavailable capture has evidence", registrationID)
	}
	result := snmpdiag.Capture{
		Roles:          roles,
		AttemptOrdinal: capture.AttemptID.Ordinal,
		State:          state,
		Reason:         reason,
		RecordCount:    capture.RecordCount,
		LogicalBytes:   capture.LogicalBytes,
	}
	if capture.Evidence != nil {
		if capture.Evidence.ID != capture.AttemptID {
			return snmpdiag.Capture{}, fmt.Errorf("topology sweep registration %d capture/evidence attempt mismatch", registrationID)
		}
		evidence, err := newArchiveAcquisitionEvidenceV1(capture.Evidence)
		if err != nil {
			return snmpdiag.Capture{}, fmt.Errorf("topology sweep registration %d capture evidence: %w", registrationID, err)
		}
		result.Evidence = &evidence
	}
	return result, nil
}

func archiveEvidenceRef(
	ref EvidenceRef,
	present bool,
) *snmpdiag.EvidenceRef {
	if !present {
		return nil
	}
	return &snmpdiag.EvidenceRef{
		RegistrationID: uint64(ref.RegistrationID),
		Generation:     ref.Generation,
	}
}

func newArchiveAbortV1(
	abort *AbortedSweep,
) (snmpdiag.Abort, error) {
	reason, err := archiveAbortReasonName(abort.Reason)
	if err != nil {
		return snmpdiag.Abort{}, err
	}
	phase, err := archiveSweepPhaseName(abort.Phase)
	if err != nil {
		return snmpdiag.Abort{}, err
	}
	return snmpdiag.Abort{
		Sequence:              abort.Sequence,
		StartedAt:             abort.StartedAt,
		AbortedAt:             abort.AbortedAt,
		Reason:                reason,
		Phase:                 phase,
		ActiveRegistrationID:  uint64(abort.ActiveRegistrationID),
		HasActiveRegistration: abort.HasActiveRegistration,
		RegistrationCount:     abort.RegistrationCount,
		SelectedCount:         abort.SelectedCount,
	}, nil
}

func restoreArchiveDocument(d snmpdiag.Document) (Cut, error) {
	if d.Format != snmpdiag.Format {
		return Cut{}, fmt.Errorf("unsupported format %q", d.Format)
	}
	if d.Version != snmpdiag.Version {
		return Cut{}, fmt.Errorf("unsupported version %d", d.Version)
	}
	return restoreArchiveSnapshot(d.Snapshot)
}

func restoreArchiveSnapshot(s snmpdiag.Snapshot) (Cut, error) {
	lifecycle, err := restoreArchiveLifecycle(s.Lifecycle)
	if err != nil {
		return Cut{}, err
	}
	result := Cut{
		Lifecycle:       lifecycle,
		ProducerScopeID: s.ProducerScopeID,
	}
	if s.Topology != nil {
		cut, err := restoreArchiveSweep(*s.Topology)
		if err != nil {
			return Cut{}, err
		}
		result.Topology = cut
	}
	if s.LastAborted != nil {
		abort, err := restoreArchiveAbort(*s.LastAborted)
		if err != nil {
			return Cut{}, err
		}
		result.LastAborted = abort
	}
	return result, nil
}

func restoreArchiveLifecycle(l snmpdiag.Lifecycle) (LifecycleCut, error) {
	state, err := archiveParseCaptureState(l.State)
	if err != nil {
		return LifecycleCut{}, fmt.Errorf("job lifecycle capture state: %w", err)
	}
	reason, err := archiveParseCaptureReason(l.Reason)
	if err != nil {
		return LifecycleCut{}, fmt.Errorf("job lifecycle capture reason: %w", err)
	}
	result := LifecycleCut{
		State:  state,
		Reason: reason,
		Cut: ddsnmp.DeviceLifecycleCut{
			Sequence:   l.Cut.Sequence,
			CapturedAt: l.Cut.CapturedAt,
			Entries:    make([]ddsnmp.DeviceLifecycleEntry, 0, len(l.Cut.Entries)),
		},
	}
	seen := make(map[ddsnmp.DeviceRegistrationID]struct{}, len(l.Cut.Entries))
	for _, entry := range l.Cut.Entries {
		registrationID := ddsnmp.DeviceRegistrationID(entry.RegistrationID)
		if registrationID == 0 {
			return LifecycleCut{}, errors.New("job lifecycle registration ID is zero")
		}
		if _, ok := seen[registrationID]; ok {
			return LifecycleCut{}, fmt.Errorf("duplicate job lifecycle registration ID %d", registrationID)
		}
		seen[registrationID] = struct{}{}
		phase, err := snmpdiag.ParseLifecyclePhase(entry.LastCompleted.Phase)
		if err != nil {
			return LifecycleCut{}, fmt.Errorf("job lifecycle registration %d phase: %w", registrationID, err)
		}
		outcome, err := snmpdiag.ParseLifecycleOutcome(entry.LastCompleted.Outcome)
		if err != nil {
			return LifecycleCut{}, fmt.Errorf("job lifecycle registration %d outcome: %w", registrationID, err)
		}
		if entry.LastCompleted.PreparationFailure != (collectorapi.JobConfigFailure{}) && !entry.LastCompleted.PreparationFailure.Valid() {
			return LifecycleCut{}, errors.New("invalid preparation failure")
		}
		if !entry.LastCompleted.CollectionFailures.Valid() {
			return LifecycleCut{}, errors.New("invalid collection failures")
		}
		if !entry.LastCompleted.Failure.Valid() {
			return LifecycleCut{}, errors.New("invalid lifecycle failure")
		}
		profileContext, err := ddsnmp.RestoreProfileContext(entry.Profiles)
		if err != nil {
			return LifecycleCut{}, fmt.Errorf("job lifecycle profile context: %w", err)
		}
		result.Cut.Entries = append(result.Cut.Entries, ddsnmp.DeviceLifecycleEntry{
			RegistrationID: registrationID,
			Info: ddsnmp.DeviceLifecycleInfo{
				Hostname:    entry.Hostname,
				Profiles:    profileContext,
				Port:        entry.Port,
				SNMPVersion: entry.SNMPVersion,
			},
			LastCompleted: ddsnmp.DeviceLifecycleStatus{
				Phase:              phase,
				Failure:            entry.LastCompleted.Failure,
				PreparationFailure: entry.LastCompleted.PreparationFailure,
				CollectionFailures: entry.LastCompleted.CollectionFailures,
				Outcome:            outcome,
				CompletedAt:        entry.LastCompleted.CompletedAt,
			},
			TopologyReady: entry.TopologyReady,
		})
	}
	return result, nil
}

func restoreArchiveSweep(s snmpdiag.Sweep) (*SweepCut, error) {

	state, err := archiveParseCaptureState(s.CaptureState)
	if err != nil {
		return nil, fmt.Errorf("topology sweep capture state: %w", err)
	}
	reason, err := archiveParseCaptureReason(s.CaptureReason)
	if err != nil {
		return nil, fmt.Errorf("topology sweep capture reason: %w", err)
	}
	result := &SweepCut{
		Sequence:      s.Sequence,
		StartedAt:     s.StartedAt,
		PublishedAt:   s.PublishedAt,
		CaptureState:  state,
		CaptureReason: reason,
		RecordCount:   s.RecordCount,
		LogicalBytes:  s.LogicalBytes,
		Devices:       make([]SweepDevice, 0, len(s.Devices)),
		Removed:       make([]RemovedDevice, 0, len(s.Removed)),
	}
	seen := make(map[ddsnmp.DeviceRegistrationID]struct{}, len(s.Devices)+len(s.Removed))
	for _, device := range s.Devices {
		row, err := restoreArchiveDevice(device, s.Sequence)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[row.RegistrationID]; ok {
			return nil, fmt.Errorf("duplicate topology sweep registration ID %d", row.RegistrationID)
		}
		seen[row.RegistrationID] = struct{}{}
		result.Devices = append(result.Devices, row)
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
		if hasRef && ref.Generation > s.Sequence {
			return nil, fmt.Errorf(
				"removed topology registration %d retained-success generation %d exceeds sweep generation %d",
				registrationID,
				ref.Generation,
				s.Sequence,
			)
		}
		result.Removed = append(result.Removed, RemovedDevice{
			RegistrationID:     registrationID,
			RetainedSuccess:    ref,
			HasRetainedSuccess: hasRef,
		})
	}
	return result, nil
}

func restoreArchiveDevice(
	d snmpdiag.Device,
	sweepGeneration uint64,
) (SweepDevice, error) {
	registrationID := ddsnmp.DeviceRegistrationID(d.RegistrationID)
	if registrationID == 0 {
		return SweepDevice{}, errors.New("topology sweep registration ID is zero")
	}
	outcome, err := archiveParseDeviceOutcome(d.Outcome)
	if err != nil {
		return SweepDevice{}, fmt.Errorf("topology sweep registration %d outcome: %w", registrationID, err)
	}
	ref, hasRef, err := restoreArchiveEvidenceRef(d.RetainedSuccess, registrationID)
	if err != nil {
		return SweepDevice{}, fmt.Errorf("topology sweep registration %d: %w", registrationID, err)
	}
	result := SweepDevice{
		RegistrationID:     registrationID,
		Selected:           d.Selected,
		Outcome:            outcome,
		LastAttempt:        d.LastAttempt,
		LastSuccess:        d.LastSuccess,
		NextRetry:          d.NextRetry,
		RetainedSuccess:    ref,
		HasRetainedSuccess: hasRef,
		HasObservation:     d.HasObservation,
		ExpiresAt:          d.ExpiresAt,
		Renderable:         d.Renderable,
		Expired:            d.Expired,
	}
	seenRoles := make(map[string]struct{}, 2)
	seenAttemptOrdinals := make(map[uint64]struct{}, len(d.Captures))
	for _, archivedCapture := range d.Captures {
		capture, err := restoreArchiveCapture(archivedCapture, registrationID)
		if err != nil {
			return SweepDevice{}, err
		}
		if _, ok := seenAttemptOrdinals[capture.AttemptID.Ordinal]; ok {
			return SweepDevice{}, fmt.Errorf(
				"topology sweep registration %d duplicate capture attempt ordinal %d",
				registrationID,
				capture.AttemptID.Ordinal,
			)
		}
		seenAttemptOrdinals[capture.AttemptID.Ordinal] = struct{}{}
		if len(archivedCapture.Roles) == 0 {
			return SweepDevice{}, fmt.Errorf("topology sweep registration %d capture has no role", registrationID)
		}
		for _, role := range archivedCapture.Roles {
			if _, ok := seenRoles[role]; ok {
				return SweepDevice{}, fmt.Errorf(
					"topology sweep registration %d duplicate capture role %q",
					registrationID,
					role,
				)
			}
			seenRoles[role] = struct{}{}
			switch role {
			case archiveCaptureRoleLatestAttempt:
				result.LatestAttempt = capture
			case archiveCaptureRoleRetainedSuccess:
				result.Acquisition = capture
			default:
				return SweepDevice{}, fmt.Errorf(
					"topology sweep registration %d unknown capture role %q",
					registrationID,
					role,
				)
			}
		}
	}
	if hasRef != (result.Acquisition != nil) {
		return SweepDevice{}, fmt.Errorf(
			"topology sweep registration %d retained-success reference and capture role disagree",
			registrationID,
		)
	}
	if hasRef && ref.Generation > sweepGeneration {
		return SweepDevice{}, fmt.Errorf(
			"topology sweep registration %d retained-success generation %d exceeds sweep generation %d",
			registrationID,
			ref.Generation,
			sweepGeneration,
		)
	}
	if result.Acquisition != nil && result.Acquisition.State == CaptureAvailable {
		if err := validateAcquisitionEvidence(result.Acquisition.Evidence); err != nil {
			return SweepDevice{}, fmt.Errorf(
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
) (*AcquisitionCapture, error) {
	state, err := archiveParseCaptureState(c.State)
	if err != nil {
		return nil, fmt.Errorf("topology sweep registration %d capture state: %w", registrationID, err)
	}
	reason, err := archiveParseCaptureReason(c.Reason)
	if err != nil {
		return nil, fmt.Errorf("topology sweep registration %d capture reason: %w", registrationID, err)
	}
	attemptID := AcquisitionAttemptID{RegistrationID: registrationID, Ordinal: c.AttemptOrdinal}
	if attemptID.Ordinal == 0 {
		return nil, fmt.Errorf("topology sweep registration %d capture attempt ordinal is zero", registrationID)
	}
	result := &AcquisitionCapture{
		AttemptID:    attemptID,
		State:        state,
		Reason:       reason,
		RecordCount:  c.RecordCount,
		LogicalBytes: c.LogicalBytes,
	}
	if c.Evidence != nil {
		evidence, err := restoreArchiveAcquisitionEvidence(*c.Evidence, attemptID)
		if err != nil {
			return nil, fmt.Errorf("topology sweep registration %d capture evidence: %w", registrationID, err)
		}
		result.Evidence = evidence
	}
	if state == CaptureAvailable && result.Evidence == nil {
		return nil, fmt.Errorf("topology sweep registration %d available capture has no evidence", registrationID)
	}
	if state != CaptureAvailable && result.Evidence != nil {
		return nil, fmt.Errorf("topology sweep registration %d unavailable capture has evidence", registrationID)
	}
	return result, nil
}

func restoreArchiveEvidenceRef(r *snmpdiag.EvidenceRef,
	owner ddsnmp.DeviceRegistrationID,
) (EvidenceRef, bool, error) {
	if r == nil {
		return EvidenceRef{}, false, nil
	}
	registrationID := ddsnmp.DeviceRegistrationID(r.RegistrationID)
	if registrationID == 0 || registrationID != owner {
		return EvidenceRef{}, false, fmt.Errorf(
			"retained-success registration reference %d does not match owner %d",
			registrationID,
			owner,
		)
	}
	if r.Generation == 0 {
		return EvidenceRef{}, false, errors.New("retained-success generation is zero")
	}
	return EvidenceRef{RegistrationID: registrationID, Generation: r.Generation}, true, nil
}

func restoreArchiveAbort(a snmpdiag.Abort) (*AbortedSweep, error) {
	reason, err := archiveParseAbortReason(a.Reason)
	if err != nil {
		return nil, err
	}
	phase, err := archiveParseSweepPhase(a.Phase)
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
	return &AbortedSweep{
		Sequence:              a.Sequence,
		StartedAt:             a.StartedAt,
		AbortedAt:             a.AbortedAt,
		Reason:                reason,
		Phase:                 phase,
		ActiveRegistrationID:  registrationID,
		HasActiveRegistration: a.HasActiveRegistration,
		RegistrationCount:     a.RegistrationCount,
		SelectedCount:         a.SelectedCount,
	}, nil
}

var (
	archiveCaptureStateNames = []string{
		"unknown", "available", "unavailable",
	}
	archiveCaptureReasonNames = []string{
		"none", "projection_error", "projection_panic",
	}
	archiveDeviceOutcomeNames = []string{
		"unknown", "success", "no_profiles", "failed",
	}
	archiveAbortReasonNames = []string{
		"unknown", "canceled", "panic",
	}
	archiveSweepPhaseNames = []string{
		"unknown", "registration_cut", "target_resolution", "device_refresh", "commit",
	}
)

func archiveEnumName[T ~uint8](value T, names []string) (string, error) {
	index := int(value)
	if index < 0 || index >= len(names) {
		return "", fmt.Errorf("unknown value %d", value)
	}
	return names[index], nil
}

func archiveParseEnum[T ~uint8](value string, names []string) (T, error) {
	for index, name := range names {
		if value == name {
			return T(index), nil
		}
	}
	return 0, fmt.Errorf("unknown value %q", value)
}

func archiveCaptureStateName(value CaptureState) (string, error) {
	return archiveEnumName(value, archiveCaptureStateNames)
}

func archiveParseCaptureState(value string) (CaptureState, error) {
	return archiveParseEnum[CaptureState](value, archiveCaptureStateNames)
}

func archiveCaptureReasonName(value CaptureReason) (string, error) {
	return archiveEnumName(value, archiveCaptureReasonNames)
}

func archiveParseCaptureReason(value string) (CaptureReason, error) {
	return archiveParseEnum[CaptureReason](value, archiveCaptureReasonNames)
}

func archiveDeviceOutcomeName(value RefreshOutcome) (string, error) {
	return archiveEnumName(value, archiveDeviceOutcomeNames)
}

func archiveParseDeviceOutcome(value string) (RefreshOutcome, error) {
	return archiveParseEnum[RefreshOutcome](value, archiveDeviceOutcomeNames)
}

func archiveAbortReasonName(value DiagnosticAbortReason) (string, error) {
	return archiveEnumName(value, archiveAbortReasonNames)
}

func archiveParseAbortReason(value string) (DiagnosticAbortReason, error) {
	return archiveParseEnum[DiagnosticAbortReason](value, archiveAbortReasonNames)
}

func archiveSweepPhaseName(value DiagnosticSweepPhase) (string, error) {
	return archiveEnumName(value, archiveSweepPhaseNames)
}

func archiveParseSweepPhase(value string) (DiagnosticSweepPhase, error) {
	return archiveParseEnum[DiagnosticSweepPhase](value, archiveSweepPhaseNames)
}
