// SPDX-License-Identifier: GPL-3.0-or-later

package topologydiag

import (
	"fmt"

	topologyapi "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	topologyv1renderer "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyv1"
)

func (s Semantics) replayDiagnostics(
	diagnostics Cut,
	options topologyoptions.QueryOptions,
) (topologyapi.Data, bool, error) {
	replay := s.replayDiagnosticStagesWithObservationScope(diagnostics, options, false)
	if replay.typed.state != inspectionPresent {
		return topologyapi.Data{}, false, replay.err
	}
	return replay.payload, true, nil
}

type diagnosticReplayedDevice struct {
	registrationID  ddsnmp.DeviceRegistrationID
	latestAttempt   *AcquisitionCapture
	retainedSuccess *AcquisitionCapture
	observation     inspectionState
	localDeviceID   string
	localDevice     topologymodel.Device
}

type diagnosticReplayStages struct {
	devices []diagnosticReplayedDevice
	graph   inspectionStage
	typed   inspectionStage
	data    topologymodel.Data
	payload topologyapi.Data
	err     error
}

func (s Semantics) replayDiagnosticStages(
	diagnostics Cut,
	options topologyoptions.QueryOptions,
) diagnosticReplayStages {
	return s.replayDiagnosticStagesWithObservationScope(diagnostics, options, true)
}

func (s Semantics) replayDiagnosticStagesWithObservationScope(
	diagnostics Cut,
	options topologyoptions.QueryOptions,
	includeNonRenderable bool,
) diagnosticReplayStages {
	var replay diagnosticReplayStages
	cut := diagnostics.Topology
	if cut == nil || cut.CaptureState != CaptureAvailable {
		return replay
	}

	replay.devices = make([]diagnosticReplayedDevice, 0, len(cut.Devices))
	snapshots := make([]topologymodel.ObservationSnapshot, 0, len(cut.Devices))
	for _, device := range cut.Devices {
		replayed := diagnosticReplayedDevice{
			registrationID:  device.RegistrationID,
			latestAttempt:   device.LatestAttempt,
			retainedSuccess: device.Acquisition,
		}
		if !device.Renderable && !includeNonRenderable {
			replay.devices = append(replay.devices, replayed)
			continue
		}
		capture := device.Acquisition
		if capture == nil || capture.State != CaptureAvailable || capture.Evidence == nil {
			replayed.observation = inspectionUndetermined
			replay.devices = append(replay.devices, replayed)
			if device.Renderable && replay.err == nil {
				replay.err = fmt.Errorf(
					"replay renderable device %s: acquisition evidence is unavailable",
					device.RegistrationID.String(),
				)
			}
			continue
		}
		err := validateAcquisitionEvidence(capture.Evidence)
		if err != nil {
			replayed.observation = inspectionUndetermined
			replay.devices = append(replay.devices, replayed)
			if device.Renderable && replay.err == nil {
				replay.err = fmt.Errorf("replay renderable device %s: %w", device.RegistrationID.String(), err)
			}
			continue
		}
		snapshot, hasObservation := s.ReplayAcquisition(capture.Evidence)
		if !hasObservation {
			replayed.observation = inspectionAbsent
			replay.devices = append(replay.devices, replayed)
			if device.Renderable && replay.err == nil {
				replay.err = fmt.Errorf(
					"replay renderable device %s: observation is unavailable",
					device.RegistrationID.String(),
				)
			}
			continue
		}
		replayed.observation = inspectionPresent
		replayed.localDeviceID = snapshot.LocalDeviceID
		replayed.localDevice = snapshot.LocalDevice
		replay.devices = append(replay.devices, replayed)
		if device.Renderable {
			snapshots = append(snapshots, snapshot)
		}
	}
	if replay.err != nil {
		return replay
	}
	if len(snapshots) == 0 {
		return replay
	}

	data, ok, err := s.BuildGraph(
		snapshots,
		diagnostics.ProducerScopeID,
		options,
	)
	if err != nil || !ok {
		replay.err = err
		return replay
	}
	replay.graph.state = inspectionPresent
	replay.data = data
	payload, err := topologyv1renderer.Render(data)
	if err != nil {
		replay.err = err
		return replay
	}
	replay.typed.state = inspectionPresent
	replay.payload = payload
	return replay
}
