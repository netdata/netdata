// SPDX-License-Identifier: GPL-3.0-or-later

package topologydiag

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	topologyapi "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

// DiagnosticArchive is one validated, owned SNMP topology diagnostic archive.
// It exposes only the read-only operations needed by the source-only maintainer tool.
type DiagnosticArchive struct {
	producerVersion string
	diagnostics     Cut
	semantics       Semantics
}

// DefaultDiagnosticQueryOptions returns the production topology query defaults.
func DefaultDiagnosticQueryOptions() DiagnosticQueryOptions {
	return diagnosticQueryOptionsFromInternal(topologyoptions.DefaultQueryOptions())
}

// InspectDiagnosticDocument validates the typed evidence before exposing
// lifecycle inspection or topology replay. The caller has decoded it once.
func InspectDiagnosticDocument(document snmpdiag.Document, semantics Semantics) (*DiagnosticArchive, error) {
	if semantics.ReplayAcquisition == nil || semantics.BuildGraph == nil {
		return nil, errors.New("inspect SNMP diagnostic archive: topology semantics are missing")
	}
	diagnostics, err := restoreArchiveDocument(document)
	if err != nil {
		return nil, fmt.Errorf("inspect SNMP diagnostic archive: %w", err)
	}
	return &DiagnosticArchive{
		semantics:       semantics,
		producerVersion: document.Producer.AgentVersion,
		diagnostics:     diagnostics,
	}, nil
}

func (a *DiagnosticArchive) Identity() DiagnosticArchiveIdentity {
	if a == nil {
		return DiagnosticArchiveIdentity{}
	}
	return DiagnosticArchiveIdentity{
		Format:               snmpdiag.Format,
		Version:              snmpdiag.Version,
		ProducerAgentVersion: a.producerVersion,
	}
}

func (a *DiagnosticArchive) Summary() (DiagnosticSummary, error) {
	if a == nil {
		return DiagnosticSummary{}, errors.New("summarize SNMP topology diagnostic archive: nil archive")
	}
	return newDiagnosticSummary(a.Identity(), a.diagnostics)
}

func (a *DiagnosticArchive) Replay(
	options DiagnosticQueryOptions,
) (topologyapi.Data, error) {
	if a == nil {
		return topologyapi.Data{}, errors.New("replay SNMP topology diagnostic archive: nil archive")
	}
	query, err := diagnosticQueryOptionsToInternal(options)
	if err != nil {
		return topologyapi.Data{}, err
	}
	payload, ok, err := a.semantics.replayDiagnostics(a.diagnostics, query)
	if err != nil {
		return topologyapi.Data{}, err
	}
	if !ok {
		return topologyapi.Data{}, errors.New("archive has no replayable topology generation")
	}
	return payload, nil
}

func (a *DiagnosticArchive) InspectDevice(
	options DiagnosticQueryOptions,
	registrationID uint64,
) (DiagnosticDeviceInspection, error) {
	if a == nil {
		return DiagnosticDeviceInspection{}, errors.New("inspect SNMP topology device: nil archive")
	}
	query, err := diagnosticQueryOptionsToInternal(options)
	if err != nil {
		return DiagnosticDeviceInspection{}, err
	}
	report, err := a.semantics.inspectDevice(a.diagnostics, query, ddsnmp.DeviceRegistrationID(registrationID))
	if err != nil {
		return DiagnosticDeviceInspection{}, err
	}
	return newDiagnosticDeviceInspection(report)
}

func (a *DiagnosticArchive) InspectLink(
	options DiagnosticQueryOptions,
	subject DiagnosticLinkSubject,
) (DiagnosticLinkInspection, error) {
	if a == nil {
		return DiagnosticLinkInspection{}, errors.New("inspect SNMP topology link: nil archive")
	}
	query, err := diagnosticQueryOptionsToInternal(options)
	if err != nil {
		return DiagnosticLinkInspection{}, err
	}
	internalSubject, err := diagnosticLinkSubjectToInternal(subject)
	if err != nil {
		return DiagnosticLinkInspection{}, err
	}
	report, err := a.semantics.inspectLink(a.diagnostics, query, internalSubject)
	if err != nil {
		return DiagnosticLinkInspection{}, err
	}
	return newDiagnosticLinkInspection(report)
}

// InspectLinkAt inspects one existing link by its zero-based index in the
// replay produced by the supplied query options.
func (a *DiagnosticArchive) InspectLinkAt(
	options DiagnosticQueryOptions,
	index int,
) (DiagnosticLinkInspection, error) {
	if a == nil {
		return DiagnosticLinkInspection{}, errors.New("inspect SNMP topology link: nil archive")
	}
	query, err := diagnosticQueryOptionsToInternal(options)
	if err != nil {
		return DiagnosticLinkInspection{}, err
	}
	report, err := a.semantics.inspectLinkAt(a.diagnostics, query, index)
	if err != nil {
		return DiagnosticLinkInspection{}, err
	}
	return newDiagnosticLinkInspection(report)
}

func diagnosticQueryOptionsFromInternal(options topologyoptions.QueryOptions) DiagnosticQueryOptions {
	depth := strconv.Itoa(options.Depth)
	if options.Depth == topologyoptions.DepthAllInternal {
		depth = topologyoptions.DepthAll
	}
	return DiagnosticQueryOptions{
		CollapseActorsByIP:     options.CollapseActorsByIP,
		EliminateNonIPInferred: options.EliminateNonIPInferred,
		MapType:                options.MapType,
		InferenceStrategy:      options.InferenceStrategy,
		ManagedDeviceFocus:     options.ManagedDeviceFocus,
		Depth:                  depth,
	}
}

func diagnosticQueryOptionsToInternal(
	options DiagnosticQueryOptions,
) (topologyoptions.QueryOptions, error) {
	mapType := strings.ToLower(strings.TrimSpace(options.MapType))
	if mapType == "" {
		mapType = topologyoptions.MapTypeManagedFabric
	}
	switch mapType {
	case topologyoptions.MapTypeManagedFabric,
		topologyoptions.MapTypeLLDPCDPManaged,
		topologyoptions.MapTypeHighConfidenceInferred,
		topologyoptions.MapTypeAllDevicesLowConfidence:
	default:
		return topologyoptions.QueryOptions{}, fmt.Errorf("unknown topology map type %q", options.MapType)
	}

	inference := strings.ToLower(strings.TrimSpace(options.InferenceStrategy))
	if inference == "" {
		inference = topologyoptions.InferenceStrategyFDBMinimumKnowledge
	}
	switch inference {
	case topologyoptions.InferenceStrategyFDBMinimumKnowledge,
		topologyoptions.InferenceStrategySTPParentTree,
		topologyoptions.InferenceStrategyFDBPairwise,
		topologyoptions.InferenceStrategySTPFDBCorrelated,
		topologyoptions.InferenceStrategyCDPFDBHybrid:
	default:
		return topologyoptions.QueryOptions{}, fmt.Errorf("unknown topology inference strategy %q", options.InferenceStrategy)
	}

	focus := strings.TrimSpace(options.ManagedDeviceFocus)
	if focus == "" {
		focus = topologyoptions.ManagedFocusAllDevices
	}
	focusValues := topologyoptions.SplitManagedFocusValues([]string{focus})
	if len(focusValues) == 0 {
		return topologyoptions.QueryOptions{}, fmt.Errorf("unknown managed device focus %q", options.ManagedDeviceFocus)
	}
	for _, value := range focusValues {
		if topologyoptions.NormalizeManagedFocusValue(value) == "" {
			return topologyoptions.QueryOptions{}, fmt.Errorf("unknown managed device focus %q", value)
		}
	}
	focus = topologyoptions.FormatManagedFocuses(focusValues)

	depthValue := strings.ToLower(strings.TrimSpace(options.Depth))
	depth := topologyoptions.DepthAllInternal
	if depthValue != "" && depthValue != topologyoptions.DepthAll {
		parsed, err := strconv.Atoi(depthValue)
		if err != nil || parsed < topologyoptions.DepthMin || parsed > topologyoptions.DepthMax {
			return topologyoptions.QueryOptions{}, fmt.Errorf("invalid topology depth %q", options.Depth)
		}
		depth = parsed
	}

	return topologyoptions.QueryOptions{
		CollapseActorsByIP:     options.CollapseActorsByIP,
		EliminateNonIPInferred: options.EliminateNonIPInferred,
		MapType:                mapType,
		InferenceStrategy:      inference,
		ManagedDeviceFocus:     focus,
		Depth:                  depth,
	}, nil
}

func diagnosticLinkSubjectToInternal(
	subject DiagnosticLinkSubject,
) (inspectionLinkSubject, error) {
	family := normalizeInspectionToken(subject.Family)
	switch family {
	case "lldp", "cdp", "bridge", "fdb", "stp", "arp",
		topologymodel.L3SubnetLinkType,
		topologymodel.L3SubnetMembershipLinkType,
		topologymodel.OSPFAdjacencyLinkType,
		topologymodel.BGPAdjacencyLinkType:
	default:
		return inspectionLinkSubject{}, fmt.Errorf("unknown topology link family %q", subject.Family)
	}
	direction := normalizeInspectionToken(subject.Direction)
	switch direction {
	case "observed", "unidirectional", "bidirectional":
	default:
		return inspectionLinkSubject{}, fmt.Errorf("unknown topology link direction %q", subject.Direction)
	}
	return normalizeInspectionLinkSubject(inspectionLinkSubject{
		srcIdentity: subject.SourceIdentity,
		dstIdentity: subject.DestinationIdentity,
		family:      family,
		protocol:    subject.Protocol,
		direction:   direction,
	}), nil
}

func newDiagnosticSummary(
	identity DiagnosticArchiveIdentity,
	diagnostics Cut,
) (DiagnosticSummary, error) {
	lifecycleStatus, err := newDiagnosticCaptureStatus(diagnostics.Lifecycle.State, diagnostics.Lifecycle.Reason)
	if err != nil {
		return DiagnosticSummary{}, fmt.Errorf("summarize lifecycle cut: %w", err)
	}
	result := DiagnosticSummary{
		Archive:         identity,
		ProducerScopeID: diagnostics.ProducerScopeID,
		Lifecycle: diagnosticLifecycleCutSummary{
			Capture:       lifecycleStatus,
			Sequence:      diagnostics.Lifecycle.Cut.Sequence,
			CapturedAt:    diagnostics.Lifecycle.Cut.CapturedAt,
			Registrations: len(diagnostics.Lifecycle.Cut.Entries),
		},
	}
	registrations := make(map[uint64]*diagnosticRegistrationSummary)
	registration := func(id uint64) *diagnosticRegistrationSummary {
		if registrations[id] == nil {
			registrations[id] = &diagnosticRegistrationSummary{RegistrationID: id}
		}
		return registrations[id]
	}
	for _, entry := range diagnostics.Lifecycle.Cut.Entries {
		converted, err := newDiagnosticLifecycleRegistration(entry)
		if err != nil {
			return DiagnosticSummary{}, err
		}
		registration(uint64(entry.RegistrationID)).Lifecycle = &converted
	}
	if diagnostics.Topology != nil {
		cut, err := newDiagnosticTopologyCutSummary(diagnostics.Topology)
		if err != nil {
			return DiagnosticSummary{}, err
		}
		result.Topology = &cut
		for i := range diagnostics.Topology.Devices {
			device, err := newDiagnosticSweepRegistration(&diagnostics.Topology.Devices[i])
			if err != nil {
				return DiagnosticSummary{}, err
			}
			registration(uint64(diagnostics.Topology.Devices[i].RegistrationID)).Sweep = &device
		}
		for i := range diagnostics.Topology.Removed {
			removed := newDiagnosticRemovedRegistration(&diagnostics.Topology.Removed[i])
			registration(uint64(diagnostics.Topology.Removed[i].RegistrationID)).Removed = &removed
		}
	}
	result.LastAborted, err = newDiagnosticAbortedSweep(diagnostics.LastAborted)
	if err != nil {
		return DiagnosticSummary{}, err
	}
	ids := make([]uint64, 0, len(registrations))
	for id := range registrations {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	result.Registrations = make([]diagnosticRegistrationSummary, 0, len(ids))
	for _, id := range ids {
		result.Registrations = append(result.Registrations, *registrations[id])
	}
	return result, nil
}

func newDiagnosticTopologyCutSummary(
	cut *SweepCut,
) (diagnosticTopologyCutSummary, error) {
	capture, err := newDiagnosticCaptureStatus(cut.CaptureState, cut.CaptureReason)
	if err != nil {
		return diagnosticTopologyCutSummary{}, fmt.Errorf("summarize topology cut: %w", err)
	}
	result := diagnosticTopologyCutSummary{
		Capture:      capture,
		Sequence:     cut.Sequence,
		StartedAt:    cut.StartedAt,
		PublishedAt:  cut.PublishedAt,
		RecordCount:  cut.RecordCount,
		LogicalBytes: cut.LogicalBytes,
		Devices:      len(cut.Devices),
		Removed:      len(cut.Removed),
	}
	for _, device := range cut.Devices {
		if device.Selected {
			result.Selected++
		}
		if device.Renderable {
			result.Renderable++
		}
		if device.Expired {
			result.Expired++
		}
	}
	return result, nil
}

func newDiagnosticLifecycleRegistration(
	entry ddsnmp.DeviceLifecycleEntry,
) (diagnosticLifecycleRegistration, error) {
	phase, err := snmpdiag.LifecyclePhaseName(entry.LastCompleted.Phase)
	if err != nil {
		return diagnosticLifecycleRegistration{}, fmt.Errorf("lifecycle phase: %w", err)
	}
	outcome, err := snmpdiag.LifecycleOutcomeName(entry.LastCompleted.Outcome)
	if err != nil {
		return diagnosticLifecycleRegistration{}, fmt.Errorf("lifecycle outcome: %w", err)
	}
	return diagnosticLifecycleRegistration{
		Hostname:           entry.Info.Hostname,
		Profiles:           entry.Info.Profiles.Snapshot(),
		Port:               entry.Info.Port,
		SNMPVersion:        entry.Info.SNMPVersion,
		Phase:              phase,
		Failure:            entry.LastCompleted.Failure,
		PreparationFailure: entry.LastCompleted.PreparationFailure,
		CollectionFailures: entry.LastCompleted.CollectionFailures,
		Outcome:            outcome,
		CompletedAt:        entry.LastCompleted.CompletedAt,
		TopologyReady:      entry.TopologyReady,
	}, nil
}

func newDiagnosticSweepRegistration(
	device *SweepDevice,
) (diagnosticSweepRegistration, error) {
	outcome, err := archiveDeviceOutcomeName(device.Outcome)
	if err != nil {
		return diagnosticSweepRegistration{}, fmt.Errorf("device outcome: %w", err)
	}
	latest, err := newDiagnosticCaptureSummary(device.LatestAttempt)
	if err != nil {
		return diagnosticSweepRegistration{}, fmt.Errorf("latest attempt: %w", err)
	}
	retained, err := newDiagnosticCaptureSummary(device.Acquisition)
	if err != nil {
		return diagnosticSweepRegistration{}, fmt.Errorf("retained success: %w", err)
	}
	return diagnosticSweepRegistration{
		Selected:           device.Selected,
		Outcome:            outcome,
		LastAttempt:        device.LastAttempt,
		LastSuccess:        device.LastSuccess,
		NextRetry:          device.NextRetry,
		RetainedSuccessRef: newDiagnosticEvidenceReference(device.RetainedSuccess, device.HasRetainedSuccess),
		LatestAttempt:      latest,
		RetainedSuccess:    retained,
		SameAttempt:        device.LatestAttempt != nil && device.LatestAttempt == device.Acquisition,
		HasObservation:     device.HasObservation,
		ExpiresAt:          device.ExpiresAt,
		Renderable:         device.Renderable,
		Expired:            device.Expired,
	}, nil
}

func newDiagnosticRemovedRegistration(device *RemovedDevice) diagnosticRemovedRegistration {
	return diagnosticRemovedRegistration{
		RetainedSuccessRef: newDiagnosticEvidenceReference(device.RetainedSuccess, device.HasRetainedSuccess),
	}
}

func newDiagnosticEvidenceReference(ref EvidenceRef, present bool) *diagnosticEvidenceReference {
	if !present {
		return nil
	}
	return &diagnosticEvidenceReference{
		RegistrationID: uint64(ref.RegistrationID),
		Generation:     ref.Generation,
	}
}

func newDiagnosticCaptureSummary(capture *AcquisitionCapture) (*diagnosticCaptureSummary, error) {
	if capture == nil {
		return nil, nil
	}
	status, err := newDiagnosticCaptureStatus(capture.State, capture.Reason)
	if err != nil {
		return nil, err
	}
	result := &diagnosticCaptureSummary{
		AttemptOrdinal: capture.AttemptID.Ordinal,
		Capture:        status,
		RecordCount:    capture.RecordCount,
		LogicalBytes:   capture.LogicalBytes,
	}
	if capture.Evidence != nil {
		evidence, err := newDiagnosticAcquisitionEvidenceSummary(capture.Evidence)
		if err != nil {
			return nil, err
		}
		result.Evidence = &evidence
	}
	return result, nil
}

func newDiagnosticAcquisitionEvidenceSummary(
	evidence *AcquisitionAttemptEvidence,
) (diagnosticAcquisitionEvidenceSummary, error) {
	target, err := archiveTargetOutcomeName(evidence.Target.Outcome)
	if err != nil {
		return diagnosticAcquisitionEvidenceSummary{}, fmt.Errorf("target outcome: %w", err)
	}
	client, err := newDiagnosticPhaseStatus(evidence.Client)
	if err != nil {
		return diagnosticAcquisitionEvidenceSummary{}, fmt.Errorf("client phase: %w", err)
	}
	connect, err := newDiagnosticPhaseStatus(evidence.Connect)
	if err != nil {
		return diagnosticAcquisitionEvidenceSummary{}, fmt.Errorf("connect phase: %w", err)
	}
	profiles, err := newDiagnosticPhaseStatus(evidence.Profiles)
	if err != nil {
		return diagnosticAcquisitionEvidenceSummary{}, fmt.Errorf("profiles phase: %w", err)
	}
	collection, err := newDiagnosticPhaseStatus(evidence.Collection)
	if err != nil {
		return diagnosticAcquisitionEvidenceSummary{}, fmt.Errorf("collection phase: %w", err)
	}
	sysUptime, err := newDiagnosticPhaseStatus(evidence.SysUptime)
	if err != nil {
		return diagnosticAcquisitionEvidenceSummary{}, fmt.Errorf("sys_uptime phase: %w", err)
	}
	vlanProfiles, err := newDiagnosticPhaseStatus(evidence.VLANProfiles)
	if err != nil {
		return diagnosticAcquisitionEvidenceSummary{}, fmt.Errorf("VLAN profiles phase: %w", err)
	}
	result := diagnosticAcquisitionEvidenceSummary{
		Interruption:       evidence.Interruption,
		ProfileContext:     evidence.ProfileContext.Snapshot(),
		VLANProfileContext: evidence.VLANProfileContext.Snapshot(),
		Hostname:           evidence.Device.Hostname,
		SysObjectID:        evidence.Device.SysObjectID,
		SysName:            evidence.Device.SysName,
		Vendor:             evidence.Device.Vendor,
		Model:              evidence.Device.Model,
		TargetOutcome:      target,
		CollectedAt:        evidence.CollectedAt,
		FreshForNanos:      int64(evidence.FreshFor),
		Client:             client,
		Connect:            connect,
		Profiles:           profiles,
		Collection:         collection,
		SysUptime:          sysUptime,
		VLANProfiles:       vlanProfiles,
		Contexts:           len(evidence.CollectionContexts),
	}
	for _, address := range evidence.Target.Addresses {
		result.TargetAddresses = append(result.TargetAddresses, address.String())
	}
	for _, context := range evidence.CollectionContexts {
		result.ProfileRuns += len(context.Profiles)
	}
	return result, nil
}

func newDiagnosticCaptureStatus(
	state CaptureState,
	reason CaptureReason,
) (diagnosticCaptureStatus, error) {
	stateName, err := archiveCaptureStateName(state)
	if err != nil {
		return diagnosticCaptureStatus{}, err
	}
	reasonName, err := archiveCaptureReasonName(reason)
	if err != nil {
		return diagnosticCaptureStatus{}, err
	}
	return diagnosticCaptureStatus{State: stateName, Reason: reasonName}, nil
}

func newDiagnosticPhaseStatus(
	phase AcquisitionPhaseEvidence,
) (diagnosticPhaseStatus, error) {
	outcome, err := archivePhaseOutcomeName(phase.Outcome)
	if err != nil {
		return diagnosticPhaseStatus{}, err
	}
	failure, err := archivePhaseFailureName(phase.Failure)
	if err != nil {
		return diagnosticPhaseStatus{}, err
	}
	return diagnosticPhaseStatus{Outcome: outcome, Failure: failure, Detail: phase.Detail}, nil
}

func newDiagnosticAbortedSweep(
	aborted *AbortedSweep,
) (*diagnosticAbortedSweep, error) {
	if aborted == nil {
		return nil, nil
	}
	reason, err := archiveAbortReasonName(aborted.Reason)
	if err != nil {
		return nil, err
	}
	phase, err := archiveSweepPhaseName(aborted.Phase)
	if err != nil {
		return nil, err
	}
	return &diagnosticAbortedSweep{
		Sequence:              aborted.Sequence,
		StartedAt:             aborted.StartedAt,
		AbortedAt:             aborted.AbortedAt,
		Reason:                reason,
		Phase:                 phase,
		ActiveRegistrationID:  uint64(aborted.ActiveRegistrationID),
		HasActiveRegistration: aborted.HasActiveRegistration,
		RegistrationCount:     aborted.RegistrationCount,
		SelectedCount:         aborted.SelectedCount,
	}, nil
}

func diagnosticStage(stage inspectionStage) diagnosticStageReport {
	state := diagnosticStateUndetermined
	switch stage.state {
	case inspectionPresent:
		state = diagnosticStatePresent
	case inspectionAbsent:
		state = diagnosticStateAbsent
	}
	return diagnosticStageReport{State: state, Candidates: stage.candidates}
}

func cloneStringMap(values map[string]string) map[string]string {
	return maps.Clone(values)
}

func cloneStrings(values []string) []string {
	return slices.Clone(values)
}
