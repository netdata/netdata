// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"errors"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologydiag"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

type topologyAcquisitionRecorder struct {
	sourceRecorder *ddsnmpcollector.SourceRecorder
	attemptID      topologydiag.AcquisitionAttemptID

	state          topologydiag.CaptureState
	reason         topologydiag.CaptureReason
	recordCount    uint64
	logicalBytes   uint64
	evidence       *topologydiag.AcquisitionAttemptEvidence
	projectProfile func(
		topologySemanticEventKind,
		ddsnmpcollector.AcquisitionProfileReport,
		*ddsnmp.ProfileMetrics,
	) topologydiag.AcquisitionProfileValues
}

type topologyAcquisitionProfileObserver struct {
	recorder       *topologyAcquisitionRecorder
	contextOrdinal uint32
	eventKind      topologySemanticEventKind
}

func newTopologyAcquisitionRecorder(
	id topologydiag.AcquisitionAttemptID,
	device topologydiag.DeviceInput,
	target topologydiag.TargetResolutionEvidence,
) (recorder *topologyAcquisitionRecorder) {
	recorder = &topologyAcquisitionRecorder{
		attemptID:      id,
		state:          topologydiag.CaptureAvailable,
		projectProfile: projectTopologyAcquisitionProfileValues,
	}
	defer func() {
		if recover() != nil {
			recorder.fail(topologydiag.CaptureReasonProjectionPanic)
		}
	}()
	records, logicalBytes := topologyAcquisitionAttemptShape(device, target)
	recorder.recordCount += records
	recorder.logicalBytes += logicalBytes
	recorder.evidence = &topologydiag.AcquisitionAttemptEvidence{
		ID:     id,
		Device: cloneTopologyDeviceInput(device),
		Target: topologydiag.TargetResolutionEvidence{
			Outcome:   target.Outcome,
			Addresses: slices.Clone(target.Addresses),
		},
		Client:       notObservedAcquisitionPhase(),
		Connect:      notObservedAcquisitionPhase(),
		Profiles:     notObservedAcquisitionPhase(),
		Collection:   notObservedAcquisitionPhase(),
		SysUptime:    notObservedAcquisitionPhase(),
		VLANProfiles: notObservedAcquisitionPhase(),
	}
	return recorder
}

func topologyAcquisitionAttemptShape(device topologydiag.DeviceInput, target topologydiag.TargetResolutionEvidence) (uint64, uint64) {
	records := uint64(1 + len(target.Addresses) + len(device.VnodeLabels))
	logicalBytes := topologySemanticDeviceLogicalBytes(device) + 96 + 7*snmputils.FailureLogicalBytes + 64
	for _, address := range target.Addresses {
		logicalBytes += uint64(len(address.String()))
	}
	return records, logicalBytes
}

func topologyAcquisitionContextShape(vlanID, vlanName string) (uint64, uint64) {
	return 1, uint64(48+len(vlanID)+len(vlanName)) + 4*snmputils.FailureLogicalBytes + ddsnmp.CollectionFailuresLogicalBytes
}

func notObservedAcquisitionPhase() topologydiag.AcquisitionPhaseEvidence {
	return topologydiag.AcquisitionPhaseEvidence{Outcome: topologydiag.AcquisitionPhaseNotObserved}
}

func successfulAcquisitionPhase() topologydiag.AcquisitionPhaseEvidence {
	return topologydiag.AcquisitionPhaseEvidence{Outcome: topologydiag.AcquisitionPhaseSuccess}
}

func failedAcquisitionPhase(class topologydiag.AcquisitionFailureClass, errors ...error) topologydiag.AcquisitionPhaseEvidence {
	detail := snmputils.Failure{Reason: "unknown"}
	if len(errors) != 0 && errors[0] != nil {
		detail = snmputils.ClassifyFailure(errors[0])
	}
	switch class {
	case topologydiag.AcquisitionFailureClientConfiguration:
		detail.Operation = "client"
		if detail.Reason == "unknown" {
			detail.Reason = "invalid_configuration"
		}
	case topologydiag.AcquisitionFailureConnect:
		detail.Operation = "connect"
	case topologydiag.AcquisitionFailureCollection:
		if detail.Operation == "" {
			detail.Operation = "tables"
		}
	case topologydiag.AcquisitionFailureSysUptime:
		detail.Operation = "sys_uptime"
	case topologydiag.AcquisitionFailureVLANIdentifier:
		detail.Operation = "vlan_identifier"
		detail.Reason = "invalid_configuration"
	}
	return topologydiag.AcquisitionPhaseEvidence{Outcome: topologydiag.AcquisitionPhaseFailed, Failure: class, Detail: detail}
}

func (r *topologyAcquisitionRecorder) recordInterruption(err error) {
	if r != nil && r.evidence != nil {
		r.evidence.Interruption = snmputils.ClassifyFailure(err)
	}
}

func (r *topologyAcquisitionRecorder) beginContext(ordinal uint32, vlanID, vlanName string) ddsnmpcollector.AcquisitionObserver {
	if r == nil || r.state != topologydiag.CaptureAvailable || r.evidence == nil {
		return nil
	}
	defer func() {
		if recover() != nil {
			r.fail(topologydiag.CaptureReasonProjectionPanic)
		}
	}()
	records, logicalBytes := topologyAcquisitionContextShape(vlanID, vlanName)
	r.recordCount += records
	r.logicalBytes += logicalBytes
	for _, context := range r.evidence.CollectionContexts {
		if context.Ordinal == ordinal {
			r.fail(topologydiag.CaptureReasonProjectionError)
			return nil
		}
	}
	r.evidence.CollectionContexts = append(r.evidence.CollectionContexts, topologydiag.AcquisitionContextEvidence{
		Ordinal:    ordinal,
		VLANID:     strings.Clone(vlanID),
		VLANName:   strings.Clone(vlanName),
		Client:     notObservedAcquisitionPhase(),
		Connect:    notObservedAcquisitionPhase(),
		Collection: notObservedAcquisitionPhase(),
	})
	eventKind := topologySemanticEventTopologyMetrics
	if ordinal != 0 {
		eventKind = topologySemanticEventVLANContext
	}
	return topologyAcquisitionProfileObserver{
		recorder:       r,
		contextOrdinal: ordinal,
		eventKind:      eventKind,
	}
}

func (o topologyAcquisitionProfileObserver) ObserveProfile(
	report ddsnmpcollector.AcquisitionProfileReport,
	metrics *ddsnmp.ProfileMetrics,
) {
	if o.recorder == nil || o.recorder.state != topologydiag.CaptureAvailable {
		return
	}
	defer func() {
		if recover() != nil {
			o.recorder.fail(topologydiag.CaptureReasonProjectionPanic)
		}
	}()
	context := o.recorder.contextByOrdinal(o.contextOrdinal)
	if context == nil {
		o.recorder.fail(topologydiag.CaptureReasonProjectionError)
		return
	}
	for _, profile := range context.Profiles {
		if profile.Identity.Ordinal == report.Identity.Ordinal {
			o.recorder.fail(topologydiag.CaptureReasonProjectionError)
			return
		}
	}
	records, logicalBytes, err := topologyAcquisitionProfileShape(o.eventKind, report, metrics)
	if err != nil {
		o.recorder.fail(topologydiag.CaptureReasonProjectionError)
		return
	}
	o.recorder.recordCount += records
	o.recorder.logicalBytes += logicalBytes
	if o.recorder.projectProfile == nil {
		o.recorder.fail(topologydiag.CaptureReasonProjectionError)
		return
	}
	for i := range report.Routes {
		report.Routes[i].RootOID = strings.Clone(report.Routes[i].RootOID)
	}
	context.Profiles = append(context.Profiles, topologydiag.AcquisitionProfileEvidence{
		Identity:     report.Identity,
		Outcome:      report.Outcome,
		FailurePhase: report.FailurePhase,
		Stats:        report.Stats,
		Execution:    report.Execution,
		Routes:       report.Routes,
		Values:       o.recorder.projectProfile(o.eventKind, report, metrics),
	})
}

func topologyAcquisitionProfileShape(
	eventKind topologySemanticEventKind,
	report ddsnmpcollector.AcquisitionProfileReport,
	profile *ddsnmp.ProfileMetrics,
) (uint64, uint64, error) {
	if report.Outcome == ddsnmpcollector.AcquisitionProfileOutcomeUnknown {
		return 0, 0, errors.New("unknown acquisition profile outcome")
	}
	records, logicalBytes := topologyAcquisitionReportShape(report.Routes, report.Execution)
	if profile == nil || report.Outcome == ddsnmpcollector.AcquisitionProfileOutcomeFailed {
		return records, logicalBytes, nil
	}
	if eventKind != topologySemanticEventVLANContext && eventKind != topologySemanticEventTopologyMetrics {
		return 0, 0, errors.New("unsupported acquisition semantic event")
	}
	if len(report.TopologyValueReferences) != len(profile.TopologyMetrics) {
		return 0, 0, errors.New("topology acquisition value-reference count mismatch")
	}
	for i, metric := range profile.TopologyMetrics {
		if !topologySemanticMetricConsumed(eventKind, metric.TopologyKind) {
			continue
		}
		records++
		ref := report.TopologyValueReferences[i]
		logicalBytes += uint64(len(ref.RowIndex) + len(ref.Field))
		logicalBytes += uint64(len(metric.TopologyKind)) + topologySemanticFilteredStringMapBytes(
			metric.Tags,
			func(key string) bool { return topologySemanticMetricTagAllowed(metric.TopologyKind, key) },
		)
	}
	logicalBytes += topologySemanticFilteredMetaTagMapBytes(profile.DeviceMetadata, topologySemanticProfileMetadataAllowed)
	logicalBytes += topologySemanticFilteredStringMapBytes(profile.Tags, topologySemanticProfileTagAllowed)
	if eventKind == topologySemanticEventVLANContext {
		return records, logicalBytes, nil
	}
	if len(report.BGPValueReferences) != len(profile.BGPRows) {
		return 0, 0, errors.New("BGP acquisition value-reference count mismatch")
	}
	if profile.BGPCollectError == nil {
		records += uint64(len(profile.BGPRows))
		for _, row := range profile.BGPRows {
			if !portableTopologySemanticOrigin(row.OriginProfileID) {
				return 0, 0, errors.New("non-portable BGP origin profile ID")
			}
			logicalBytes += topologySemanticBGPRowLogicalBytes(row)
		}
	}
	return records, logicalBytes, nil
}

func topologyAcquisitionReportShape(routes []ddsnmpcollector.AcquisitionRouteReport, execution *ddsnmpcollector.AcquisitionExecutionReport) (uint64, uint64) {
	records := uint64(1 + len(routes))
	logicalBytes := uint64(96)
	for _, route := range routes {
		logicalBytes += uint64(64 + len(route.RootOID))
		records += uint64(len(route.Sources) + len(route.Processing))
		for _, binding := range route.Sources {
			logicalBytes += uint64(40 + len(binding.OID) + len(binding.Role))
		}
		for _, event := range route.Processing {
			logicalBytes += uint64(80 + len(event.RowIndex) + len(event.Field) + len(event.OID) + len(event.Reason) + len(event.Stage))
		}
	}
	if execution != nil {
		records += uint64(1 + len(execution.WalkOperations))
		// Execution header, preparation measurements and operation references.
		logicalBytes += 88
		logicalBytes += 8 * uint64(len(execution.WalkOperations))
	}
	return records, logicalBytes
}

func projectTopologyAcquisitionProfileValues(
	eventKind topologySemanticEventKind,
	report ddsnmpcollector.AcquisitionProfileReport,
	profile *ddsnmp.ProfileMetrics,
) topologydiag.AcquisitionProfileValues {
	if profile == nil || report.Outcome == ddsnmpcollector.AcquisitionProfileOutcomeFailed {
		return topologydiag.AcquisitionProfileValues{}
	}
	result := topologydiag.AcquisitionProfileValues{
		Metadata: cloneTopologySemanticMetaTags(profile.DeviceMetadata, topologySemanticProfileMetadataAllowed),
		Tags:     cloneTopologySemanticStringTags(profile.Tags, topologySemanticProfileTagAllowed),
		TopologyMetrics: projectTopologyAcquisitionMetrics(
			eventKind,
			profile.TopologyMetrics,
			report.TopologyValueReferences,
		),
	}
	if eventKind == topologySemanticEventVLANContext {
		return result
	}
	result.BGPFailed = profile.BGPCollectError != nil
	if !result.BGPFailed {
		result.BGPRows = projectTopologyAcquisitionBGPRows(profile.BGPRows, report.BGPValueReferences)
	}
	return result
}

func (r *topologyAcquisitionRecorder) contextByOrdinal(ordinal uint32) *topologydiag.AcquisitionContextEvidence {
	if r == nil || r.evidence == nil {
		return nil
	}
	for i := range r.evidence.CollectionContexts {
		if r.evidence.CollectionContexts[i].Ordinal == ordinal {
			return &r.evidence.CollectionContexts[i]
		}
	}
	return nil
}

func (r *topologyAcquisitionRecorder) completeContext(
	ordinal uint32,
	phase topologydiag.AcquisitionPhaseEvidence,
) {
	if r == nil || r.state != topologydiag.CaptureAvailable {
		return
	}
	context := r.contextByOrdinal(ordinal)
	if context == nil {
		r.fail(topologydiag.CaptureReasonProjectionError)
		return
	}
	context.Collection = phase
	sort.Slice(context.Profiles, func(i, j int) bool {
		return context.Profiles[i].Identity.Ordinal < context.Profiles[j].Identity.Ordinal
	})
}

func (r *topologyAcquisitionRecorder) setCollectedShape(collectedAt time.Time, freshFor time.Duration, sysUptime int64) {
	if r == nil || r.evidence == nil || r.state != topologydiag.CaptureAvailable {
		return
	}
	r.evidence.CollectedAt = collectedAt
	r.evidence.FreshFor = freshFor
	r.evidence.SysUptimeValue = sysUptime
}

func (r *topologyAcquisitionRecorder) finish() *topologydiag.AcquisitionCapture {
	if r == nil {
		return &topologydiag.AcquisitionCapture{State: topologydiag.CaptureUnavailable, Reason: topologydiag.CaptureReasonProjectionError}
	}
	if r.sourceRecorder != nil {
		if context := r.contextByOrdinal(0); context != nil {
			context.Sources = r.sourceRecorder.Finish()
		}
		r.sourceRecorder = nil
	}

	records, logicalBytes := r.recordCount, r.logicalBytes
	if r.evidence != nil {
		for _, context := range r.evidence.CollectionContexts {
			cr, cb := ddsnmp.SourceOperationsShape(context.Sources)
			records += cr
			logicalBytes += cb
		}
		for _, context := range []*ddsnmp.ProfileContext{r.evidence.ProfileContext, r.evidence.VLANProfileContext} {
			if context == nil {
				continue
			}
			cr, cb := context.Shape()
			records += cr
			logicalBytes += cb
		}
	}
	return &topologydiag.AcquisitionCapture{
		AttemptID:    r.attemptID,
		State:        r.state,
		Reason:       r.reason,
		RecordCount:  records,
		LogicalBytes: logicalBytes,
		Evidence:     r.evidence,
	}
}

func acquisitionCaptureFromGeneration(generation *topologyDeviceGeneration) *topologydiag.AcquisitionCapture {
	if generation == nil {
		return nil
	}
	return generation.acquisition
}

func (r *topologyAcquisitionRecorder) fail(reason topologydiag.CaptureReason) {
	if r == nil {
		return
	}
	r.state = topologydiag.CaptureUnavailable
	r.reason = reason
	r.evidence = nil
}

func (r *topologyAcquisitionRecorder) sourceClient(client gosnmp.Handler) gosnmp.Handler {
	context := r.contextByOrdinal(0)
	if context == nil {
		return client
	}
	r.sourceRecorder = &ddsnmpcollector.SourceRecorder{}
	return r.sourceRecorder.Wrap(client)
}
