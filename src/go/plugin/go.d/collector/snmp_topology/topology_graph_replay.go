// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"

	topologyapi "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	topologyv1renderer "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyv1"
)

func replayTopologyDiagnostics(
	diagnostics topologyDiagnostics,
	options topologyoptions.QueryOptions,
) (topologyapi.Data, bool, error) {
	cut := diagnostics.topology
	if cut == nil || cut.captureState != diagnosticCaptureAvailable {
		return topologyapi.Data{}, false, nil
	}

	snapshots := make([]topologymodel.ObservationSnapshot, 0, len(cut.devices))
	for _, device := range cut.devices {
		if !device.renderable {
			continue
		}
		capture := device.acquisition
		if capture == nil || capture.state != diagnosticCaptureAvailable || capture.evidence == nil {
			return topologyapi.Data{}, false, fmt.Errorf(
				"replay renderable device %s: acquisition evidence is unavailable",
				device.registrationID.String(),
			)
		}
		snapshot, err := replayTopologyAcquisitionEvidence(capture.evidence)
		if err != nil {
			return topologyapi.Data{}, false, fmt.Errorf(
				"replay renderable device %s: %w",
				device.registrationID.String(),
				err,
			)
		}
		if snapshot == nil || !snapshot.hasObservation {
			return topologyapi.Data{}, false, fmt.Errorf(
				"replay renderable device %s: observation is unavailable",
				device.registrationID.String(),
			)
		}
		snapshots = append(snapshots, snapshot.observation)
	}
	if len(snapshots) == 0 {
		return topologyapi.Data{}, false, nil
	}

	sortTopologyObservationSnapshots(snapshots)
	data, ok, err := buildTopologyObservationSnapshot(
		snapshots,
		diagnostics.producerScopeID,
		options,
		topologyGraphBuildEnvironment{},
	)
	if err != nil || !ok {
		return topologyapi.Data{}, ok, err
	}
	payload, err := topologyv1renderer.Render(data)
	if err != nil {
		return topologyapi.Data{}, false, err
	}
	return payload, true, nil
}
