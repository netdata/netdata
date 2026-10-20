// SPDX-License-Identifier: GPL-3.0-or-later

package topologydiag

import (
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

type inspectionState uint8

const (
	inspectionUndetermined inspectionState = iota
	inspectionPresent
	inspectionAbsent
)

type inspectionStage struct {
	state      inspectionState
	candidates int
}

type inspectionLifecycleResult struct {
	membership    inspectionStage
	captureState  CaptureState
	captureReason CaptureReason
	sequence      uint64
	capturedAt    time.Time
	entry         *ddsnmp.DeviceLifecycleEntry
}

type inspectionDiagnosticCutResult struct {
	captureState  CaptureState
	captureReason CaptureReason
	sequence      uint64
	startedAt     time.Time
	publishedAt   time.Time
}

type inspectionSweepResult struct {
	inspectionDiagnosticCutResult
	membership inspectionStage
	device     *SweepDevice
}

type inspectionRemovedResult struct {
	membership inspectionStage
	device     *RemovedDevice
}

type inspectionCaptureResult struct {
	membership inspectionStage
	evidence   inspectionStage
	capture    *AcquisitionCapture
}

type inspectionActorResult struct {
	membership inspectionStage
	indexes    []int
	actors     []topologymodel.Actor
	index      int
}

type inspectionRowResult struct {
	inspectionStage
	row int
}

type deviceInspection struct {
	registrationID  ddsnmp.DeviceRegistrationID
	options         topologyoptions.QueryOptions
	lifecycle       inspectionLifecycleResult
	sweep           inspectionSweepResult
	removed         inspectionRemovedResult
	latestAttempt   inspectionCaptureResult
	retainedSuccess inspectionCaptureResult
	sameAttempt     bool
	observation     inspectionStage
	graphIdentity   inspectionActorResult
	typedIdentity   inspectionRowResult
	graphStats      topologymodel.Stats
	hasGraphStats   bool
	lastAborted     *AbortedSweep
}

type inspectionLinkSubject struct {
	srcIdentity string
	dstIdentity string
	family      string
	protocol    string
	direction   string
}

type inspectionSourceFact struct {
	registrationID ddsnmp.DeviceRegistrationID
	contextOrdinal uint32
	profileOrdinal uint32
	metric         *AcquisitionMetricValue
	bgp            *AcquisitionBGPRowValue
}

type inspectionSourceCaptureContext struct {
	latestAttempt   bool
	retainedSuccess bool
	capture         inspectionCaptureResult
	facts           []inspectionSourceFact
}

type inspectionSourceContext struct {
	registrationID  ddsnmp.DeviceRegistrationID
	latestAttempt   inspectionCaptureResult
	retainedSuccess inspectionCaptureResult
	sameAttempt     bool
	captures        []inspectionSourceCaptureContext
}

type inspectionSourceResult struct {
	contexts []inspectionSourceContext
}

type inspectionGraphLinkResult struct {
	membership inspectionStage
	srcActors  inspectionActorResult
	dstActors  inspectionActorResult
	links      []topologymodel.Link
	index      int
}

type linkInspection struct {
	subject       inspectionLinkSubject
	options       topologyoptions.QueryOptions
	diagnosticCut inspectionDiagnosticCutResult
	source        inspectionSourceResult
	graphLink     inspectionGraphLinkResult
	typedLink     inspectionRowResult
	graphStats    inspectionStage
	stats         topologymodel.Stats
	lastAborted   *AbortedSweep
}

func (s Semantics) inspectDevice(
	diagnostics Cut,
	options topologyoptions.QueryOptions,
	registrationID ddsnmp.DeviceRegistrationID,
) (deviceInspection, error) {
	report := deviceInspection{
		registrationID: registrationID,
		options:        topologyoptions.NormalizeQueryOptions(options),
		graphIdentity:  inspectionActorResult{index: -1},
		typedIdentity:  inspectionRowResult{row: -1},
		lastAborted:    diagnostics.LastAborted,
	}
	if registrationID == 0 {
		return report, fmt.Errorf("topology inspection registration ID is zero")
	}

	report.lifecycle = inspectLifecycleRegistration(diagnostics.Lifecycle, registrationID)
	report.sweep = inspectSweepRegistration(diagnostics.Topology, registrationID)
	report.removed = inspectRemovedRegistration(diagnostics.Topology, registrationID)
	if report.sweep.membership.state == inspectionUndetermined {
		return report, nil
	}
	if report.sweep.membership.state == inspectionAbsent {
		report.latestAttempt = absentTopologyInspectionCapture()
		report.retainedSuccess = absentTopologyInspectionCapture()
		report.observation.state = inspectionAbsent
		return report, nil
	}

	row := report.sweep.device
	report.latestAttempt = inspectCapture(row.LatestAttempt)
	report.retainedSuccess = inspectCapture(row.Acquisition)
	report.sameAttempt = row.LatestAttempt != nil && row.LatestAttempt == row.Acquisition
	if report.retainedSuccess.membership.state == inspectionAbsent {
		report.observation.state = inspectionAbsent
		return report, nil
	}
	if report.retainedSuccess.evidence.state != inspectionPresent {
		return report, nil
	}

	replay := s.replayDiagnosticStages(diagnostics, options)
	replayed := findDiagnosticReplayedDevice(replay.devices, registrationID)
	if replayed == nil {
		return report, nil
	}
	report.observation.state = replayed.observation
	if replayed.observation != inspectionPresent {
		return report, nil
	}
	if replay.graph.state != inspectionPresent {
		return report, nil
	}

	report.graphStats = replay.data.Stats
	report.hasGraphStats = true
	report.graphIdentity = inspectLocalDeviceIdentity(
		replay.data,
		replayed.localDeviceID,
		replayed.localDevice,
	)
	report.typedIdentity = inspectionRenderedRow(
		replay.typed.state,
		report.graphIdentity.membership,
		report.graphIdentity.index,
		replay.payload.Actors.Rows,
	)
	return report, nil
}

func (s Semantics) inspectLink(
	diagnostics Cut,
	options topologyoptions.QueryOptions,
	subject inspectionLinkSubject,
) (linkInspection, error) {
	subject = normalizeInspectionLinkSubject(subject)
	report := newLinkInspection(diagnostics, options, subject)
	if subject.srcIdentity == "" || subject.dstIdentity == "" || subject.family == "" {
		return report, fmt.Errorf("topology inspection link subject is incomplete")
	}

	replay := s.replayDiagnosticStages(diagnostics, options)
	if replay.graph.state != inspectionPresent {
		report.source = inspectLinkSourceContext(replay.devices, subject.family)
		return report, nil
	}
	return completeLinkInspection(report, replay, inspectGraphLink(replay.data, subject)), nil
}

func (s Semantics) inspectLinkAt(
	diagnostics Cut,
	options topologyoptions.QueryOptions,
	index int,
) (linkInspection, error) {
	report := newLinkInspection(diagnostics, options, inspectionLinkSubject{})
	if index < 0 {
		return report, fmt.Errorf("topology inspection link index must be zero or greater: %d", index)
	}

	replay := s.replayDiagnosticStages(diagnostics, options)
	if replay.graph.state != inspectionPresent {
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
	subject, ok := inspectionSubjectFromLink(replay.data, index)
	if !ok {
		return report, fmt.Errorf("topology inspection link index %d has no usable endpoint identity", index)
	}
	report.subject = subject
	return completeLinkInspection(report, replay, inspectGraphLinkAt(replay.data, index)), nil
}

func newLinkInspection(
	diagnostics Cut,
	options topologyoptions.QueryOptions,
	subject inspectionLinkSubject,
) linkInspection {
	return linkInspection{
		subject:       subject,
		options:       topologyoptions.NormalizeQueryOptions(options),
		diagnosticCut: inspectDiagnosticCut(diagnostics.Topology),
		graphLink:     inspectionGraphLinkResult{index: -1},
		typedLink:     inspectionRowResult{row: -1},
		lastAborted:   diagnostics.LastAborted,
	}
}

func completeLinkInspection(
	report linkInspection,
	replay diagnosticReplayStages,
	graphLink inspectionGraphLinkResult,
) linkInspection {
	report.source = inspectLinkSourceContext(replay.devices, report.subject.family)
	report.graphStats.state = inspectionPresent
	report.stats = replay.data.Stats
	report.graphLink = graphLink
	report.typedLink = inspectionRenderedRow(
		replay.typed.state,
		report.graphLink.membership,
		report.graphLink.index,
		replay.payload.Links.Rows,
	)
	return report
}
