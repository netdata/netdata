// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"errors"
	"net/netip"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

type topologyAcquisitionAttemptID struct {
	registrationID ddsnmp.DeviceRegistrationID
	ordinal        uint64
}

type topologyTargetResolutionOutcome uint8

const (
	topologyTargetResolutionUnknown topologyTargetResolutionOutcome = iota
	topologyTargetResolutionLiteral
	topologyTargetResolutionResolved
	topologyTargetResolutionEmpty
	topologyTargetResolutionUnavailable
	topologyTargetResolutionFailed
)

type topologyTargetResolutionEvidence struct {
	outcome   topologyTargetResolutionOutcome
	addresses []netip.Addr
}

type topologyAcquisitionPhaseOutcome uint8

const (
	topologyAcquisitionPhaseUnknown topologyAcquisitionPhaseOutcome = iota
	topologyAcquisitionPhaseSuccess
	topologyAcquisitionPhaseEmpty
	topologyAcquisitionPhaseFailed
	topologyAcquisitionPhaseNotObserved
)

type topologyAcquisitionFailureClass uint8

const (
	topologyAcquisitionFailureNone topologyAcquisitionFailureClass = iota
	topologyAcquisitionFailureClientConfiguration
	topologyAcquisitionFailureConnect
	topologyAcquisitionFailureCollection
	topologyAcquisitionFailureSysUptime
	topologyAcquisitionFailureVLANIdentifier
)

type topologyAcquisitionPhaseEvidence struct {
	detail  snmputils.Failure
	outcome topologyAcquisitionPhaseOutcome
	failure topologyAcquisitionFailureClass
}

type topologyAcquisitionAttemptEvidence struct {
	interruption       snmputils.Failure
	profileContext     *ddsnmp.ProfileContext
	vlanProfileContext *ddsnmp.ProfileContext
	id                 topologyAcquisitionAttemptID
	device             topologySemanticDeviceInput
	target             topologyTargetResolutionEvidence
	client             topologyAcquisitionPhaseEvidence
	connect            topologyAcquisitionPhaseEvidence
	profiles           topologyAcquisitionPhaseEvidence
	collection         topologyAcquisitionPhaseEvidence
	sysUptime          topologyAcquisitionPhaseEvidence
	vlanProfiles       topologyAcquisitionPhaseEvidence
	collectedAt        time.Time
	freshFor           time.Duration
	sysUptimeValue     int64
	collectionContexts []topologyAcquisitionContextEvidence
}

type topologyAcquisitionContextEvidence struct {
	sources        []ddsnmp.SourceOperation
	sourceRecorder *ddsnmpcollector.SourceRecorder
	interruption   snmputils.Failure
	failures       ddsnmp.CollectionFailures
	ordinal        uint32
	vlanID         string
	vlanName       string
	client         topologyAcquisitionPhaseEvidence
	connect        topologyAcquisitionPhaseEvidence
	collection     topologyAcquisitionPhaseEvidence
	profiles       []topologyAcquisitionProfileEvidence
}

type topologyAcquisitionProfileEvidence struct {
	identity     ddsnmpcollector.AcquisitionProfileIdentity
	outcome      ddsnmpcollector.AcquisitionProfileOutcome
	failurePhase ddsnmpcollector.AcquisitionFailurePhase
	stats        ddsnmp.CollectionStats
	execution    *ddsnmpcollector.AcquisitionExecutionReport
	routes       []ddsnmpcollector.AcquisitionRouteReport
	values       topologyAcquisitionProfileValues
}

type topologyAcquisitionCapture struct {
	attemptID    topologyAcquisitionAttemptID
	state        diagnosticCaptureState
	reason       diagnosticCaptureReason
	recordCount  uint64
	logicalBytes uint64
	evidence     *topologyAcquisitionAttemptEvidence
}

type topologyAcquisitionRecorder struct {
	attemptID topologyAcquisitionAttemptID

	state          diagnosticCaptureState
	reason         diagnosticCaptureReason
	recordCount    uint64
	logicalBytes   uint64
	evidence       *topologyAcquisitionAttemptEvidence
	projectProfile func(
		topologySemanticEventKind,
		ddsnmpcollector.AcquisitionProfileReport,
		*ddsnmp.ProfileMetrics,
	) topologyAcquisitionProfileValues
}

type topologyAcquisitionProfileObserver struct {
	recorder       *topologyAcquisitionRecorder
	contextOrdinal uint32
	eventKind      topologySemanticEventKind
}

func newTopologyAcquisitionRecorder(
	id topologyAcquisitionAttemptID,
	device topologySemanticDeviceInput,
	target topologyTargetResolutionEvidence,
) (recorder *topologyAcquisitionRecorder) {
	recorder = &topologyAcquisitionRecorder{
		attemptID:      id,
		state:          diagnosticCaptureAvailable,
		projectProfile: projectTopologyAcquisitionProfileValues,
	}
	defer func() {
		if recover() != nil {
			recorder.fail(diagnosticCaptureReasonProjectionPanic)
		}
	}()
	records, logicalBytes := topologyAcquisitionAttemptShape(device, target)
	recorder.recordCount += records
	recorder.logicalBytes += logicalBytes
	recorder.evidence = &topologyAcquisitionAttemptEvidence{
		id:     id,
		device: cloneTopologySemanticDeviceInput(device),
		target: topologyTargetResolutionEvidence{
			outcome:   target.outcome,
			addresses: slices.Clone(target.addresses),
		},
		client:       notObservedAcquisitionPhase(),
		connect:      notObservedAcquisitionPhase(),
		profiles:     notObservedAcquisitionPhase(),
		collection:   notObservedAcquisitionPhase(),
		sysUptime:    notObservedAcquisitionPhase(),
		vlanProfiles: notObservedAcquisitionPhase(),
	}
	return recorder
}

func topologyAcquisitionAttemptShape(device topologySemanticDeviceInput, target topologyTargetResolutionEvidence) (uint64, uint64) {
	records := uint64(1 + len(target.addresses) + len(device.vnodeLabels))
	logicalBytes := topologySemanticDeviceLogicalBytes(device) + 96 + 7*snmputils.FailureLogicalBytes + 64
	for _, address := range target.addresses {
		logicalBytes += uint64(len(address.String()))
	}
	return records, logicalBytes
}

func topologyAcquisitionContextShape(vlanID, vlanName string) (uint64, uint64) {
	return 1, uint64(48+len(vlanID)+len(vlanName)) + 4*snmputils.FailureLogicalBytes + ddsnmp.CollectionFailuresLogicalBytes
}

func notObservedAcquisitionPhase() topologyAcquisitionPhaseEvidence {
	return topologyAcquisitionPhaseEvidence{outcome: topologyAcquisitionPhaseNotObserved}
}

func successfulAcquisitionPhase() topologyAcquisitionPhaseEvidence {
	return topologyAcquisitionPhaseEvidence{outcome: topologyAcquisitionPhaseSuccess}
}

func failedAcquisitionPhase(class topologyAcquisitionFailureClass, errors ...error) topologyAcquisitionPhaseEvidence {
	detail := snmputils.Failure{Reason: "unknown"}
	if len(errors) != 0 && errors[0] != nil {
		detail = snmputils.ClassifyFailure(errors[0])
	}
	switch class {
	case topologyAcquisitionFailureClientConfiguration:
		detail.Operation = "client"
		if detail.Reason == "unknown" {
			detail.Reason = "invalid_configuration"
		}
	case topologyAcquisitionFailureConnect:
		detail.Operation = "connect"
	case topologyAcquisitionFailureCollection:
		if detail.Operation == "" {
			detail.Operation = "tables"
		}
	case topologyAcquisitionFailureSysUptime:
		detail.Operation = "sys_uptime"
	case topologyAcquisitionFailureVLANIdentifier:
		detail.Operation = "vlan_identifier"
		detail.Reason = "invalid_configuration"
	}
	return topologyAcquisitionPhaseEvidence{outcome: topologyAcquisitionPhaseFailed, failure: class, detail: detail}
}

func (r *topologyAcquisitionRecorder) recordInterruption(err error) {
	if r != nil && r.evidence != nil {
		r.evidence.interruption = snmputils.ClassifyFailure(err)
	}
}

func (r *topologyAcquisitionRecorder) beginContext(ordinal uint32, vlanID, vlanName string) ddsnmpcollector.AcquisitionObserver {
	if r == nil || r.state != diagnosticCaptureAvailable || r.evidence == nil {
		return nil
	}
	defer func() {
		if recover() != nil {
			r.fail(diagnosticCaptureReasonProjectionPanic)
		}
	}()
	records, logicalBytes := topologyAcquisitionContextShape(vlanID, vlanName)
	r.recordCount += records
	r.logicalBytes += logicalBytes
	for _, context := range r.evidence.collectionContexts {
		if context.ordinal == ordinal {
			r.fail(diagnosticCaptureReasonProjectionError)
			return nil
		}
	}
	r.evidence.collectionContexts = append(r.evidence.collectionContexts, topologyAcquisitionContextEvidence{
		ordinal:    ordinal,
		vlanID:     strings.Clone(vlanID),
		vlanName:   strings.Clone(vlanName),
		client:     notObservedAcquisitionPhase(),
		connect:    notObservedAcquisitionPhase(),
		collection: notObservedAcquisitionPhase(),
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
	if o.recorder == nil || o.recorder.state != diagnosticCaptureAvailable {
		return
	}
	defer func() {
		if recover() != nil {
			o.recorder.fail(diagnosticCaptureReasonProjectionPanic)
		}
	}()
	context := o.recorder.contextByOrdinal(o.contextOrdinal)
	if context == nil {
		o.recorder.fail(diagnosticCaptureReasonProjectionError)
		return
	}
	for _, profile := range context.profiles {
		if profile.identity.Ordinal == report.Identity.Ordinal {
			o.recorder.fail(diagnosticCaptureReasonProjectionError)
			return
		}
	}
	records, logicalBytes, err := topologyAcquisitionProfileShape(o.eventKind, report, metrics)
	if err != nil {
		o.recorder.fail(diagnosticCaptureReasonProjectionError)
		return
	}
	o.recorder.recordCount += records
	o.recorder.logicalBytes += logicalBytes
	if o.recorder.projectProfile == nil {
		o.recorder.fail(diagnosticCaptureReasonProjectionError)
		return
	}
	for i := range report.Routes {
		report.Routes[i].RootOID = strings.Clone(report.Routes[i].RootOID)
	}
	context.profiles = append(context.profiles, topologyAcquisitionProfileEvidence{
		identity:     report.Identity,
		outcome:      report.Outcome,
		failurePhase: report.FailurePhase,
		stats:        report.Stats,
		execution:    report.Execution,
		routes:       report.Routes,
		values:       o.recorder.projectProfile(o.eventKind, report, metrics),
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
) topologyAcquisitionProfileValues {
	if profile == nil || report.Outcome == ddsnmpcollector.AcquisitionProfileOutcomeFailed {
		return topologyAcquisitionProfileValues{}
	}
	result := topologyAcquisitionProfileValues{
		metadata: cloneTopologySemanticMetaTags(profile.DeviceMetadata, topologySemanticProfileMetadataAllowed),
		tags:     cloneTopologySemanticStringTags(profile.Tags, topologySemanticProfileTagAllowed),
		metrics: projectTopologyAcquisitionMetrics(
			eventKind,
			profile.TopologyMetrics,
			report.TopologyValueReferences,
		),
	}
	if eventKind == topologySemanticEventVLANContext {
		return result
	}
	result.bgpFailed = profile.BGPCollectError != nil
	if !result.bgpFailed {
		result.bgpRows = projectTopologyAcquisitionBGPRows(profile.BGPRows, report.BGPValueReferences)
	}
	return result
}

func (r *topologyAcquisitionRecorder) contextByOrdinal(ordinal uint32) *topologyAcquisitionContextEvidence {
	if r == nil || r.evidence == nil {
		return nil
	}
	for i := range r.evidence.collectionContexts {
		if r.evidence.collectionContexts[i].ordinal == ordinal {
			return &r.evidence.collectionContexts[i]
		}
	}
	return nil
}

func (r *topologyAcquisitionRecorder) completeContext(
	ordinal uint32,
	phase topologyAcquisitionPhaseEvidence,
) {
	if r == nil || r.state != diagnosticCaptureAvailable {
		return
	}
	context := r.contextByOrdinal(ordinal)
	if context == nil {
		r.fail(diagnosticCaptureReasonProjectionError)
		return
	}
	context.collection = phase
	sort.Slice(context.profiles, func(i, j int) bool {
		return context.profiles[i].identity.Ordinal < context.profiles[j].identity.Ordinal
	})
}

func (r *topologyAcquisitionRecorder) setCollectedShape(collectedAt time.Time, freshFor time.Duration, sysUptime int64) {
	if r == nil || r.evidence == nil || r.state != diagnosticCaptureAvailable {
		return
	}
	r.evidence.collectedAt = collectedAt
	r.evidence.freshFor = freshFor
	r.evidence.sysUptimeValue = sysUptime
}

func (r *topologyAcquisitionRecorder) finish() *topologyAcquisitionCapture {
	if r == nil {
		return &topologyAcquisitionCapture{state: diagnosticCaptureUnavailable, reason: diagnosticCaptureReasonProjectionError}
	}
	if r.evidence != nil {
		for i := range r.evidence.collectionContexts {
			context := &r.evidence.collectionContexts[i]
			if context.sourceRecorder != nil {
				context.sources = context.sourceRecorder.Finish()
				context.sourceRecorder = nil
			}
		}
	}
	records, logicalBytes := r.recordCount, r.logicalBytes
	if r.evidence != nil {
		for _, context := range r.evidence.collectionContexts {
			cr, cb := ddsnmp.SourceOperationsShape(context.sources)
			records += cr
			logicalBytes += cb
		}
		for _, context := range []*ddsnmp.ProfileContext{r.evidence.profileContext, r.evidence.vlanProfileContext} {
			if context == nil {
				continue
			}
			cr, cb := context.Shape()
			records += cr
			logicalBytes += cb
		}
	}
	return &topologyAcquisitionCapture{
		attemptID:    r.attemptID,
		state:        r.state,
		reason:       r.reason,
		recordCount:  records,
		logicalBytes: logicalBytes,
		evidence:     r.evidence,
	}
}

func acquisitionCaptureFromGeneration(generation *topologyDeviceGeneration) *topologyAcquisitionCapture {
	if generation == nil {
		return nil
	}
	return generation.acquisition
}

func (r *topologyAcquisitionRecorder) fail(reason diagnosticCaptureReason) {
	if r == nil {
		return
	}
	r.state = diagnosticCaptureUnavailable
	r.reason = reason
	r.evidence = nil
}

func (r *topologyAcquisitionRecorder) sourceClient(ordinal uint32, client gosnmp.Handler) gosnmp.Handler {
	context := r.contextByOrdinal(ordinal)
	if context == nil {
		return client
	}
	context.sourceRecorder = &ddsnmpcollector.SourceRecorder{}
	return context.sourceRecorder.Wrap(client)
}
