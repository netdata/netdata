// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"

	topologyapi "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	topologyv1renderer "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyv1"
)

func replayTopologyDiagnostics(
	diagnostics topologyDiagnostics,
	options topologyoptions.QueryOptions,
) (topologyapi.Data, bool, error) {
	replay := replayTopologyDiagnosticStagesWithObservationScope(diagnostics, options, false)
	if replay.typed.state != topologyInspectionPresent {
		return topologyapi.Data{}, false, replay.err
	}
	return replay.payload, true, nil
}

type topologyDiagnosticReplayedDevice struct {
	registrationID  ddsnmp.DeviceRegistrationID
	latestAttempt   *topologyAcquisitionCapture
	retainedSuccess *topologyAcquisitionCapture
	renderable      bool
	observation     topologyInspectionState
	snapshot        *topologyDeviceSnapshot
}

type topologyDiagnosticReplayStages struct {
	devices []topologyDiagnosticReplayedDevice
	graph   topologyInspectionStage
	typed   topologyInspectionStage
	data    topologymodel.Data
	payload topologyapi.Data
	err     error
}

func replayTopologyDiagnosticStages(
	diagnostics topologyDiagnostics,
	options topologyoptions.QueryOptions,
) topologyDiagnosticReplayStages {
	return replayTopologyDiagnosticStagesWithObservationScope(diagnostics, options, true)
}

func replayTopologyDiagnosticStagesWithObservationScope(
	diagnostics topologyDiagnostics,
	options topologyoptions.QueryOptions,
	includeNonRenderable bool,
) topologyDiagnosticReplayStages {
	var replay topologyDiagnosticReplayStages
	cut := diagnostics.topology
	if cut == nil || cut.captureState != diagnosticCaptureAvailable {
		return replay
	}

	snapshots := make([]topologymodel.ObservationSnapshot, 0, len(cut.devices))
	for _, device := range cut.devices {
		replayed := topologyDiagnosticReplayedDevice{
			registrationID:  device.registrationID,
			latestAttempt:   device.latestAttempt,
			retainedSuccess: device.acquisition,
			renderable:      device.renderable,
		}
		if !device.renderable && !includeNonRenderable {
			replay.devices = append(replay.devices, replayed)
			continue
		}
		capture := device.acquisition
		if capture == nil || capture.state != diagnosticCaptureAvailable || capture.evidence == nil {
			replayed.observation = topologyInspectionUndetermined
			replay.devices = append(replay.devices, replayed)
			if device.renderable && replay.err == nil {
				replay.err = fmt.Errorf(
					"replay renderable device %s: acquisition evidence is unavailable",
					device.registrationID.String(),
				)
			}
			continue
		}
		snapshot, err := replayTopologyAcquisitionEvidence(capture.evidence)
		if err != nil {
			replayed.observation = topologyInspectionUndetermined
			replay.devices = append(replay.devices, replayed)
			if device.renderable && replay.err == nil {
				replay.err = fmt.Errorf("replay renderable device %s: %w", device.registrationID.String(), err)
			}
			continue
		}
		if snapshot == nil || !snapshot.hasObservation {
			replayed.observation = topologyInspectionAbsent
			replay.devices = append(replay.devices, replayed)
			if device.renderable && replay.err == nil {
				replay.err = fmt.Errorf(
					"replay renderable device %s: observation is unavailable",
					device.registrationID.String(),
				)
			}
			continue
		}
		replayed.observation = topologyInspectionPresent
		replayed.snapshot = snapshot
		replay.devices = append(replay.devices, replayed)
		if device.renderable {
			snapshots = append(snapshots, snapshot.observation)
		}
	}
	if replay.err != nil {
		return replay
	}
	if len(snapshots) == 0 {
		return replay
	}

	sortTopologyObservationSnapshots(snapshots)
	data, ok, err := buildTopologyObservationSnapshot(
		snapshots,
		diagnostics.producerScopeID,
		options,
		topologyGraphBuildEnvironment{},
	)
	if err != nil || !ok {
		replay.err = err
		return replay
	}
	replay.graph.state = topologyInspectionPresent
	replay.data = data
	payload, err := topologyv1renderer.Render(data)
	if err != nil {
		replay.err = err
		return replay
	}
	replay.typed.state = topologyInspectionPresent
	replay.payload = payload
	return replay
}
