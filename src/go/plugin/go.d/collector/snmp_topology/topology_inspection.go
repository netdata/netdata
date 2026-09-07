// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

type topologyInspectionState uint8

const (
	topologyInspectionUndetermined topologyInspectionState = iota
	topologyInspectionPresent
	topologyInspectionAbsent
)

type topologyInspectionStage struct {
	state      topologyInspectionState
	candidates int
}

type topologyInspectionLifecycleResult struct {
	membership    topologyInspectionStage
	captureState  diagnosticCaptureState
	captureReason diagnosticCaptureReason
	sequence      uint64
	capturedAt    time.Time
	entry         *ddsnmp.DeviceLifecycleEntry
}

type topologyInspectionDiagnosticCutResult struct {
	captureState  diagnosticCaptureState
	captureReason diagnosticCaptureReason
	sequence      uint64
	startedAt     time.Time
	publishedAt   time.Time
}

type topologyInspectionSweepResult struct {
	topologyInspectionDiagnosticCutResult
	membership topologyInspectionStage
	device     *topologySweepDeviceDiagnostic
}

type topologyInspectionRemovedResult struct {
	membership topologyInspectionStage
	device     *topologyRemovedDeviceDiagnostic
}

type topologyInspectionCaptureResult struct {
	membership topologyInspectionStage
	evidence   topologyInspectionStage
	capture    *topologyAcquisitionCapture
}

type topologyInspectionActorResult struct {
	membership topologyInspectionStage
	indexes    []int
	actors     []topologymodel.Actor
	index      int
}

type topologyInspectionRowResult struct {
	topologyInspectionStage
	row int
}

type topologyDeviceInspection struct {
	registrationID  ddsnmp.DeviceRegistrationID
	options         topologyoptions.QueryOptions
	lifecycle       topologyInspectionLifecycleResult
	sweep           topologyInspectionSweepResult
	removed         topologyInspectionRemovedResult
	latestAttempt   topologyInspectionCaptureResult
	retainedSuccess topologyInspectionCaptureResult
	sameAttempt     bool
	observation     topologyInspectionStage
	graphIdentity   topologyInspectionActorResult
	typedIdentity   topologyInspectionRowResult
	graphStats      topologymodel.Stats
	hasGraphStats   bool
	lastAborted     *topologyAbortedSweepDiagnostic
}

type topologyInspectionLinkSubject struct {
	srcIdentity string
	dstIdentity string
	family      string
	protocol    string
	direction   string
}

type topologyInspectionSourceFact struct {
	registrationID ddsnmp.DeviceRegistrationID
	contextOrdinal uint32
	profileOrdinal uint32
	metric         *topologyAcquisitionMetricValue
	bgp            *topologyAcquisitionBGPRowValue
}

type topologyInspectionSourceCaptureContext struct {
	latestAttempt   bool
	retainedSuccess bool
	capture         topologyInspectionCaptureResult
	facts           []topologyInspectionSourceFact
}

type topologyInspectionSourceContext struct {
	registrationID  ddsnmp.DeviceRegistrationID
	latestAttempt   topologyInspectionCaptureResult
	retainedSuccess topologyInspectionCaptureResult
	sameAttempt     bool
	captures        []topologyInspectionSourceCaptureContext
}

type topologyInspectionSourceResult struct {
	contexts []topologyInspectionSourceContext
}

type topologyInspectionGraphLinkResult struct {
	membership topologyInspectionStage
	srcActors  topologyInspectionActorResult
	dstActors  topologyInspectionActorResult
	links      []topologymodel.Link
	index      int
}

type topologyLinkInspection struct {
	subject       topologyInspectionLinkSubject
	options       topologyoptions.QueryOptions
	diagnosticCut topologyInspectionDiagnosticCutResult
	source        topologyInspectionSourceResult
	graphLink     topologyInspectionGraphLinkResult
	typedLink     topologyInspectionRowResult
	graphStats    topologyInspectionStage
	stats         topologymodel.Stats
	lastAborted   *topologyAbortedSweepDiagnostic
}

func inspectTopologyDevice(
	diagnostics topologyDiagnostics,
	options topologyoptions.QueryOptions,
	registrationID ddsnmp.DeviceRegistrationID,
) (topologyDeviceInspection, error) {
	report := topologyDeviceInspection{
		registrationID: registrationID,
		options:        topologyoptions.NormalizeQueryOptions(options),
		graphIdentity:  topologyInspectionActorResult{index: -1},
		typedIdentity:  topologyInspectionRowResult{row: -1},
		lastAborted:    diagnostics.lastAborted,
	}
	if registrationID == 0 {
		return report, fmt.Errorf("topology inspection registration ID is zero")
	}

	report.lifecycle = inspectTopologyLifecycleRegistration(diagnostics.lifecycle, registrationID)
	report.sweep = inspectTopologySweepRegistration(diagnostics.topology, registrationID)
	report.removed = inspectTopologyRemovedRegistration(diagnostics.topology, registrationID)
	if report.sweep.membership.state == topologyInspectionUndetermined {
		return report, nil
	}
	if report.sweep.membership.state == topologyInspectionAbsent {
		report.latestAttempt = absentTopologyInspectionCapture()
		report.retainedSuccess = absentTopologyInspectionCapture()
		report.observation.state = topologyInspectionAbsent
		return report, nil
	}

	row := report.sweep.device
	report.latestAttempt = inspectTopologyCapture(row.latestAttempt)
	report.retainedSuccess = inspectTopologyCapture(row.acquisition)
	report.sameAttempt = row.latestAttempt != nil && row.latestAttempt == row.acquisition
	if report.retainedSuccess.membership.state == topologyInspectionAbsent {
		report.observation.state = topologyInspectionAbsent
		return report, nil
	}
	if report.retainedSuccess.evidence.state != topologyInspectionPresent {
		return report, nil
	}

	replay := replayTopologyDiagnosticStages(diagnostics, options)
	replayed := findTopologyDiagnosticReplayedDevice(replay.devices, registrationID)
	if replayed == nil {
		return report, nil
	}
	report.observation.state = replayed.observation
	if replayed.observation != topologyInspectionPresent || replayed.snapshot == nil {
		return report, nil
	}
	if replay.graph.state != topologyInspectionPresent {
		return report, nil
	}

	report.graphStats = replay.data.Stats
	report.hasGraphStats = true
	report.graphIdentity = inspectTopologyLocalDeviceIdentity(
		replay.data,
		replayed.snapshot.observation.LocalDeviceID,
		replayed.snapshot.observation.LocalDevice,
	)
	report.typedIdentity = topologyInspectionRenderedRow(
		replay.typed.state,
		report.graphIdentity.membership,
		report.graphIdentity.index,
		replay.payload.Actors.Rows,
	)
	return report, nil
}

func inspectTopologyLink(
	diagnostics topologyDiagnostics,
	options topologyoptions.QueryOptions,
	subject topologyInspectionLinkSubject,
) (topologyLinkInspection, error) {
	subject = normalizeTopologyInspectionLinkSubject(subject)
	report := newTopologyLinkInspection(diagnostics, options, subject)
	if subject.srcIdentity == "" || subject.dstIdentity == "" || subject.family == "" {
		return report, fmt.Errorf("topology inspection link subject is incomplete")
	}

	replay := replayTopologyDiagnosticStages(diagnostics, options)
	if replay.graph.state != topologyInspectionPresent {
		report.source = inspectTopologyLinkSourceContext(replay.devices, subject.family)
		return report, nil
	}
	return completeTopologyLinkInspection(report, replay, inspectTopologyGraphLink(replay.data, subject)), nil
}

func inspectTopologyLinkAt(
	diagnostics topologyDiagnostics,
	options topologyoptions.QueryOptions,
	index int,
) (topologyLinkInspection, error) {
	report := newTopologyLinkInspection(diagnostics, options, topologyInspectionLinkSubject{})
	if index < 0 {
		return report, fmt.Errorf("topology inspection link index must be zero or greater: %d", index)
	}

	replay := replayTopologyDiagnosticStages(diagnostics, options)
	if replay.graph.state != topologyInspectionPresent {
		if replay.err != nil {
			return report, fmt.Errorf("topology inspection link index %d: replay graph: %w", index, replay.err)
		}
		return report, fmt.Errorf("topology inspection link index %d out of range [0,0)", index)
	}
	if index >= len(replay.data.Links) {
		return report, fmt.Errorf(
			"topology inspection link index %d out of range [0,%d)",
			index,
			len(replay.data.Links),
		)
	}
	subject, ok := topologyInspectionSubjectFromLink(replay.data, index)
	if !ok {
		return report, fmt.Errorf("topology inspection link index %d has no usable endpoint identity", index)
	}
	report.subject = subject
	return completeTopologyLinkInspection(report, replay, inspectTopologyGraphLinkAt(replay.data, index)), nil
}

func newTopologyLinkInspection(
	diagnostics topologyDiagnostics,
	options topologyoptions.QueryOptions,
	subject topologyInspectionLinkSubject,
) topologyLinkInspection {
	return topologyLinkInspection{
		subject:       subject,
		options:       topologyoptions.NormalizeQueryOptions(options),
		diagnosticCut: inspectTopologyDiagnosticCut(diagnostics.topology),
		graphLink:     topologyInspectionGraphLinkResult{index: -1},
		typedLink:     topologyInspectionRowResult{row: -1},
		lastAborted:   diagnostics.lastAborted,
	}
}

func completeTopologyLinkInspection(
	report topologyLinkInspection,
	replay topologyDiagnosticReplayStages,
	graphLink topologyInspectionGraphLinkResult,
) topologyLinkInspection {
	report.source = inspectTopologyLinkSourceContext(replay.devices, report.subject.family)
	report.graphStats.state = topologyInspectionPresent
	report.stats = replay.data.Stats
	report.graphLink = graphLink
	report.typedLink = topologyInspectionRenderedRow(
		replay.typed.state,
		report.graphLink.membership,
		report.graphLink.index,
		replay.payload.Links.Rows,
	)
	return report
}
