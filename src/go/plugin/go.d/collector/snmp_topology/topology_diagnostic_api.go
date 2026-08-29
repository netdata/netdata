// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/internal/snmptopologydiagnostics"
	topologyapi "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

// DiagnosticArchive is one validated, owned SNMP topology diagnostic archive.
// It exposes only the read-only operations needed by the source-only maintainer tool.
type DiagnosticArchive struct {
	archive topologyDiagnosticArchive
}

// DefaultDiagnosticArchiveReadLimits returns the generous defaults measured for
// archives produced by the Agent.
func DefaultDiagnosticArchiveReadLimits() snmptopologydiagnostics.ReadLimits {
	limits := defaultTopologyDiagnosticArchiveReadLimits()
	return snmptopologydiagnostics.ReadLimits{
		MaxCompressedBytes: limits.maxCompressedBytes,
		MaxDecodedBytes:    limits.maxDecodedBytes,
	}
}

// DefaultDiagnosticQueryOptions returns the production topology query defaults.
func DefaultDiagnosticQueryOptions() snmptopologydiagnostics.QueryOptions {
	return diagnosticQueryOptionsFromInternal(topologyoptions.DefaultQueryOptions())
}

// ReadDiagnosticArchive validates and reconstructs one archive through the
// same reader used by all offline diagnostic operations.
func ReadDiagnosticArchive(
	r io.Reader,
	limits snmptopologydiagnostics.ReadLimits,
) (*DiagnosticArchive, error) {
	archive, err := readTopologyDiagnosticArchive(r, topologyDiagnosticArchiveReadLimits{
		maxCompressedBytes: limits.MaxCompressedBytes,
		maxDecodedBytes:    limits.MaxDecodedBytes,
	})
	if err != nil {
		return nil, err
	}
	return &DiagnosticArchive{archive: archive}, nil
}

func (a *DiagnosticArchive) Identity() snmptopologydiagnostics.ArchiveIdentity {
	if a == nil {
		return snmptopologydiagnostics.ArchiveIdentity{}
	}
	return snmptopologydiagnostics.ArchiveIdentity{
		Format:               topologyDiagnosticArchiveFormat,
		Version:              topologyDiagnosticArchiveVersion,
		ProducerAgentVersion: a.archive.producerVersion,
	}
}

func (a *DiagnosticArchive) Summary() (snmptopologydiagnostics.Summary, error) {
	if a == nil {
		return snmptopologydiagnostics.Summary{}, errors.New("summarize SNMP topology diagnostic archive: nil archive")
	}
	return newDiagnosticSummary(a.Identity(), a.archive.diagnostics)
}

func (a *DiagnosticArchive) Replay(
	options snmptopologydiagnostics.QueryOptions,
) (topologyapi.Data, error) {
	if a == nil {
		return topologyapi.Data{}, errors.New("replay SNMP topology diagnostic archive: nil archive")
	}
	query, err := diagnosticQueryOptionsToInternal(options)
	if err != nil {
		return topologyapi.Data{}, err
	}
	payload, ok, err := replayTopologyDiagnostics(a.archive.diagnostics, query)
	if err != nil {
		return topologyapi.Data{}, err
	}
	if !ok {
		return topologyapi.Data{}, errors.New("archive has no replayable topology generation")
	}
	return payload, nil
}

func (a *DiagnosticArchive) InspectDevice(
	options snmptopologydiagnostics.QueryOptions,
	registrationID uint64,
) (snmptopologydiagnostics.DeviceInspection, error) {
	if a == nil {
		return snmptopologydiagnostics.DeviceInspection{}, errors.New("inspect SNMP topology device: nil archive")
	}
	query, err := diagnosticQueryOptionsToInternal(options)
	if err != nil {
		return snmptopologydiagnostics.DeviceInspection{}, err
	}
	report, err := inspectTopologyDevice(a.archive.diagnostics, query, ddsnmp.DeviceRegistrationID(registrationID))
	if err != nil {
		return snmptopologydiagnostics.DeviceInspection{}, err
	}
	return newDiagnosticDeviceInspection(report)
}

func (a *DiagnosticArchive) InspectLink(
	options snmptopologydiagnostics.QueryOptions,
	subject snmptopologydiagnostics.LinkSubject,
) (snmptopologydiagnostics.LinkInspection, error) {
	if a == nil {
		return snmptopologydiagnostics.LinkInspection{}, errors.New("inspect SNMP topology link: nil archive")
	}
	query, err := diagnosticQueryOptionsToInternal(options)
	if err != nil {
		return snmptopologydiagnostics.LinkInspection{}, err
	}
	internalSubject, err := diagnosticLinkSubjectToInternal(subject)
	if err != nil {
		return snmptopologydiagnostics.LinkInspection{}, err
	}
	report, err := inspectTopologyLink(a.archive.diagnostics, query, internalSubject)
	if err != nil {
		return snmptopologydiagnostics.LinkInspection{}, err
	}
	return newDiagnosticLinkInspection(report)
}

func diagnosticQueryOptionsFromInternal(options topologyoptions.QueryOptions) snmptopologydiagnostics.QueryOptions {
	depth := strconv.Itoa(options.Depth)
	if options.Depth == topologyoptions.DepthAllInternal {
		depth = topologyoptions.DepthAll
	}
	return snmptopologydiagnostics.QueryOptions{
		CollapseActorsByIP:     options.CollapseActorsByIP,
		EliminateNonIPInferred: options.EliminateNonIPInferred,
		MapType:                options.MapType,
		InferenceStrategy:      options.InferenceStrategy,
		ManagedDeviceFocus:     options.ManagedDeviceFocus,
		Depth:                  depth,
	}
}

func diagnosticQueryOptionsToInternal(
	options snmptopologydiagnostics.QueryOptions,
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
	subject snmptopologydiagnostics.LinkSubject,
) (topologyInspectionLinkSubject, error) {
	family := normalizeTopologyInspectionToken(subject.Family)
	switch family {
	case "lldp", "cdp", "bridge", "fdb", "stp", "arp",
		topologymodel.L3SubnetLinkType,
		topologymodel.L3SubnetMembershipLinkType,
		topologymodel.OSPFAdjacencyLinkType,
		topologymodel.BGPAdjacencyLinkType:
	default:
		return topologyInspectionLinkSubject{}, fmt.Errorf("unknown topology link family %q", subject.Family)
	}
	return normalizeTopologyInspectionLinkSubject(topologyInspectionLinkSubject{
		srcIdentity: subject.SourceIdentity,
		dstIdentity: subject.DestinationIdentity,
		family:      family,
		protocol:    subject.Protocol,
		direction:   subject.Direction,
	}), nil
}

func newDiagnosticSummary(
	identity snmptopologydiagnostics.ArchiveIdentity,
	diagnostics topologyDiagnostics,
) (snmptopologydiagnostics.Summary, error) {
	lifecycleStatus, err := newDiagnosticCaptureStatus(diagnostics.lifecycle.state, diagnostics.lifecycle.reason)
	if err != nil {
		return snmptopologydiagnostics.Summary{}, fmt.Errorf("summarize lifecycle cut: %w", err)
	}
	result := snmptopologydiagnostics.Summary{
		Archive:         identity,
		ProducerScopeID: diagnostics.producerScopeID,
		Lifecycle: snmptopologydiagnostics.LifecycleCutSummary{
			Capture:       lifecycleStatus,
			Sequence:      diagnostics.lifecycle.cut.Sequence,
			CapturedAt:    diagnostics.lifecycle.cut.CapturedAt,
			Registrations: len(diagnostics.lifecycle.cut.Entries),
		},
	}
	registrations := make(map[uint64]*snmptopologydiagnostics.RegistrationSummary)
	registration := func(id uint64) *snmptopologydiagnostics.RegistrationSummary {
		if registrations[id] == nil {
			registrations[id] = &snmptopologydiagnostics.RegistrationSummary{RegistrationID: id}
		}
		return registrations[id]
	}
	for _, entry := range diagnostics.lifecycle.cut.Entries {
		converted, err := newDiagnosticLifecycleRegistration(entry)
		if err != nil {
			return snmptopologydiagnostics.Summary{}, err
		}
		registration(uint64(entry.RegistrationID)).Lifecycle = &converted
	}
	if diagnostics.topology != nil {
		cut, err := newDiagnosticTopologyCutSummary(diagnostics.topology)
		if err != nil {
			return snmptopologydiagnostics.Summary{}, err
		}
		result.Topology = &cut
		for i := range diagnostics.topology.devices {
			device, err := newDiagnosticSweepRegistration(&diagnostics.topology.devices[i])
			if err != nil {
				return snmptopologydiagnostics.Summary{}, err
			}
			registration(uint64(diagnostics.topology.devices[i].registrationID)).Sweep = &device
		}
		for i := range diagnostics.topology.removed {
			removed := newDiagnosticRemovedRegistration(&diagnostics.topology.removed[i])
			registration(uint64(diagnostics.topology.removed[i].registrationID)).Removed = &removed
		}
	}
	result.LastAborted, err = newDiagnosticAbortedSweep(diagnostics.lastAborted)
	if err != nil {
		return snmptopologydiagnostics.Summary{}, err
	}
	ids := make([]uint64, 0, len(registrations))
	for id := range registrations {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	result.Registrations = make([]snmptopologydiagnostics.RegistrationSummary, 0, len(ids))
	for _, id := range ids {
		result.Registrations = append(result.Registrations, *registrations[id])
	}
	return result, nil
}

func newDiagnosticTopologyCutSummary(
	cut *topologySweepDiagnosticCut,
) (snmptopologydiagnostics.TopologyCutSummary, error) {
	capture, err := newDiagnosticCaptureStatus(cut.captureState, cut.captureReason)
	if err != nil {
		return snmptopologydiagnostics.TopologyCutSummary{}, fmt.Errorf("summarize topology cut: %w", err)
	}
	result := snmptopologydiagnostics.TopologyCutSummary{
		Capture:      capture,
		Sequence:     cut.sequence,
		StartedAt:    cut.startedAt,
		PublishedAt:  cut.publishedAt,
		RecordCount:  cut.recordCount,
		LogicalBytes: cut.logicalBytes,
		Devices:      len(cut.devices),
		Removed:      len(cut.removed),
	}
	for _, device := range cut.devices {
		if device.selected {
			result.Selected++
		}
		if device.renderable {
			result.Renderable++
		}
		if device.expired {
			result.Expired++
		}
	}
	return result, nil
}

func newDiagnosticLifecycleRegistration(
	entry ddsnmp.DeviceLifecycleEntry,
) (snmptopologydiagnostics.LifecycleRegistration, error) {
	phase, err := topologyDiagnosticArchiveLifecyclePhaseName(entry.LastCompleted.Phase)
	if err != nil {
		return snmptopologydiagnostics.LifecycleRegistration{}, fmt.Errorf("lifecycle phase: %w", err)
	}
	outcome, err := topologyDiagnosticArchiveLifecycleOutcomeName(entry.LastCompleted.Outcome)
	if err != nil {
		return snmptopologydiagnostics.LifecycleRegistration{}, fmt.Errorf("lifecycle outcome: %w", err)
	}
	return snmptopologydiagnostics.LifecycleRegistration{
		Hostname:      entry.Info.Hostname,
		Port:          entry.Info.Port,
		SNMPVersion:   entry.Info.SNMPVersion,
		Phase:         phase,
		Outcome:       outcome,
		CompletedAt:   entry.LastCompleted.CompletedAt,
		TopologyReady: entry.TopologyReady,
	}, nil
}

func newDiagnosticSweepRegistration(
	device *topologySweepDeviceDiagnostic,
) (snmptopologydiagnostics.SweepRegistration, error) {
	outcome, err := topologyDiagnosticArchiveDeviceOutcomeName(device.outcome)
	if err != nil {
		return snmptopologydiagnostics.SweepRegistration{}, fmt.Errorf("device outcome: %w", err)
	}
	latest, err := newDiagnosticCaptureSummary(device.latestAttempt)
	if err != nil {
		return snmptopologydiagnostics.SweepRegistration{}, fmt.Errorf("latest attempt: %w", err)
	}
	retained, err := newDiagnosticCaptureSummary(device.acquisition)
	if err != nil {
		return snmptopologydiagnostics.SweepRegistration{}, fmt.Errorf("retained success: %w", err)
	}
	return snmptopologydiagnostics.SweepRegistration{
		Selected:           device.selected,
		Outcome:            outcome,
		LastAttempt:        device.lastAttempt,
		LastSuccess:        device.lastSuccess,
		NextRetry:          device.nextRetry,
		RetainedSuccessRef: newDiagnosticEvidenceReference(device.retainedSuccess, device.hasRetainedSuccess),
		LatestAttempt:      latest,
		RetainedSuccess:    retained,
		SameAttempt:        device.latestAttempt != nil && device.latestAttempt == device.acquisition,
		HasObservation:     device.hasObservation,
		ExpiresAt:          device.expiresAt,
		Renderable:         device.renderable,
		Expired:            device.expired,
	}, nil
}

func newDiagnosticRemovedRegistration(device *topologyRemovedDeviceDiagnostic) snmptopologydiagnostics.RemovedRegistration {
	return snmptopologydiagnostics.RemovedRegistration{
		RetainedSuccessRef: newDiagnosticEvidenceReference(device.retainedSuccess, device.hasRetainedSuccess),
	}
}

func newDiagnosticEvidenceReference(ref topologyEvidenceRef, present bool) *snmptopologydiagnostics.EvidenceReference {
	if !present {
		return nil
	}
	return &snmptopologydiagnostics.EvidenceReference{
		RegistrationID: uint64(ref.registrationID),
		Generation:     ref.generation,
	}
}

func newDiagnosticCaptureSummary(capture *topologyAcquisitionCapture) (*snmptopologydiagnostics.CaptureSummary, error) {
	if capture == nil {
		return nil, nil
	}
	status, err := newDiagnosticCaptureStatus(capture.state, capture.reason)
	if err != nil {
		return nil, err
	}
	result := &snmptopologydiagnostics.CaptureSummary{
		AttemptOrdinal: capture.attemptID.ordinal,
		Capture:        status,
		RecordCount:    capture.recordCount,
		LogicalBytes:   capture.logicalBytes,
	}
	if capture.evidence != nil {
		evidence, err := newDiagnosticAcquisitionEvidenceSummary(capture.evidence)
		if err != nil {
			return nil, err
		}
		result.Evidence = &evidence
	}
	return result, nil
}

func newDiagnosticAcquisitionEvidenceSummary(
	evidence *topologyAcquisitionAttemptEvidence,
) (snmptopologydiagnostics.AcquisitionEvidenceSummary, error) {
	target, err := topologyDiagnosticArchiveTargetOutcomeName(evidence.target.outcome)
	if err != nil {
		return snmptopologydiagnostics.AcquisitionEvidenceSummary{}, fmt.Errorf("target outcome: %w", err)
	}
	client, err := newDiagnosticPhaseStatus(evidence.client)
	if err != nil {
		return snmptopologydiagnostics.AcquisitionEvidenceSummary{}, fmt.Errorf("client phase: %w", err)
	}
	connect, err := newDiagnosticPhaseStatus(evidence.connect)
	if err != nil {
		return snmptopologydiagnostics.AcquisitionEvidenceSummary{}, fmt.Errorf("connect phase: %w", err)
	}
	profiles, err := newDiagnosticPhaseStatus(evidence.profiles)
	if err != nil {
		return snmptopologydiagnostics.AcquisitionEvidenceSummary{}, fmt.Errorf("profiles phase: %w", err)
	}
	collection, err := newDiagnosticPhaseStatus(evidence.collection)
	if err != nil {
		return snmptopologydiagnostics.AcquisitionEvidenceSummary{}, fmt.Errorf("collection phase: %w", err)
	}
	sysUptime, err := newDiagnosticPhaseStatus(evidence.sysUptime)
	if err != nil {
		return snmptopologydiagnostics.AcquisitionEvidenceSummary{}, fmt.Errorf("sys_uptime phase: %w", err)
	}
	vlanProfiles, err := newDiagnosticPhaseStatus(evidence.vlanProfiles)
	if err != nil {
		return snmptopologydiagnostics.AcquisitionEvidenceSummary{}, fmt.Errorf("VLAN profiles phase: %w", err)
	}
	result := snmptopologydiagnostics.AcquisitionEvidenceSummary{
		Hostname:      evidence.device.hostname,
		SysObjectID:   evidence.device.sysObjectID,
		SysName:       evidence.device.sysName,
		Vendor:        evidence.device.vendor,
		Model:         evidence.device.model,
		TargetOutcome: target,
		CollectedAt:   evidence.collectedAt,
		FreshForNanos: int64(evidence.freshFor),
		Client:        client,
		Connect:       connect,
		Profiles:      profiles,
		Collection:    collection,
		SysUptime:     sysUptime,
		VLANProfiles:  vlanProfiles,
		Contexts:      len(evidence.collectionContexts),
	}
	for _, address := range evidence.target.addresses {
		result.TargetAddresses = append(result.TargetAddresses, address.String())
	}
	for _, context := range evidence.collectionContexts {
		result.ProfileRuns += len(context.profiles)
	}
	return result, nil
}

func newDiagnosticCaptureStatus(
	state diagnosticCaptureState,
	reason diagnosticCaptureReason,
) (snmptopologydiagnostics.CaptureStatus, error) {
	stateName, err := topologyDiagnosticArchiveCaptureStateName(state)
	if err != nil {
		return snmptopologydiagnostics.CaptureStatus{}, err
	}
	reasonName, err := topologyDiagnosticArchiveCaptureReasonName(reason)
	if err != nil {
		return snmptopologydiagnostics.CaptureStatus{}, err
	}
	return snmptopologydiagnostics.CaptureStatus{State: stateName, Reason: reasonName}, nil
}

func newDiagnosticPhaseStatus(
	phase topologyAcquisitionPhaseEvidence,
) (snmptopologydiagnostics.PhaseStatus, error) {
	outcome, err := topologyDiagnosticArchivePhaseOutcomeName(phase.outcome)
	if err != nil {
		return snmptopologydiagnostics.PhaseStatus{}, err
	}
	failure, err := topologyDiagnosticArchivePhaseFailureName(phase.failure)
	if err != nil {
		return snmptopologydiagnostics.PhaseStatus{}, err
	}
	return snmptopologydiagnostics.PhaseStatus{Outcome: outcome, Failure: failure}, nil
}

func newDiagnosticAbortedSweep(
	aborted *topologyAbortedSweepDiagnostic,
) (*snmptopologydiagnostics.AbortedSweep, error) {
	if aborted == nil {
		return nil, nil
	}
	reason, err := topologyDiagnosticArchiveAbortReasonName(aborted.reason)
	if err != nil {
		return nil, err
	}
	phase, err := topologyDiagnosticArchiveSweepPhaseName(aborted.phase)
	if err != nil {
		return nil, err
	}
	return &snmptopologydiagnostics.AbortedSweep{
		Sequence:              aborted.sequence,
		StartedAt:             aborted.startedAt,
		AbortedAt:             aborted.abortedAt,
		Reason:                reason,
		Phase:                 phase,
		ActiveRegistrationID:  uint64(aborted.activeRegistrationID),
		HasActiveRegistration: aborted.hasActiveRegistration,
		RegistrationCount:     aborted.registrationCount,
		SelectedCount:         aborted.selectedCount,
	}, nil
}

func diagnosticStage(stage topologyInspectionStage) snmptopologydiagnostics.Stage {
	state := snmptopologydiagnostics.StateUndetermined
	switch stage.state {
	case topologyInspectionPresent:
		state = snmptopologydiagnostics.StatePresent
	case topologyInspectionAbsent:
		state = snmptopologydiagnostics.StateAbsent
	}
	return snmptopologydiagnostics.Stage{State: state, Candidates: stage.candidates}
}

func cloneStringMap(values map[string]string) map[string]string {
	return maps.Clone(values)
}

func cloneStrings(values []string) []string {
	return slices.Clone(values)
}
