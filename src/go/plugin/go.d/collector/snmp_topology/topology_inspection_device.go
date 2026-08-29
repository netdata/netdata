// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"slices"
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

func inspectTopologyLifecycleRegistration(
	cut topologyJobLifecycleDiagnosticCut,
	registrationID ddsnmp.DeviceRegistrationID,
) topologyInspectionLifecycleResult {
	result := topologyInspectionLifecycleResult{
		captureState:  cut.state,
		captureReason: cut.reason,
		sequence:      cut.cut.Sequence,
		capturedAt:    cut.cut.CapturedAt,
	}
	if cut.state != diagnosticCaptureAvailable {
		return result
	}
	for i := range cut.cut.Entries {
		if cut.cut.Entries[i].RegistrationID == registrationID {
			result.membership = topologyInspectionStage{state: topologyInspectionPresent, candidates: 1}
			result.entry = &cut.cut.Entries[i]
			return result
		}
	}
	result.membership.state = topologyInspectionAbsent
	return result
}

func inspectTopologySweepRegistration(
	cut *topologySweepDiagnosticCut,
	registrationID ddsnmp.DeviceRegistrationID,
) topologyInspectionSweepResult {
	var result topologyInspectionSweepResult
	if cut == nil {
		return result
	}
	result.topologyInspectionDiagnosticCutResult = inspectTopologyDiagnosticCut(cut)
	if cut.captureState != diagnosticCaptureAvailable {
		return result
	}
	for i := range cut.devices {
		if cut.devices[i].registrationID == registrationID {
			result.membership = topologyInspectionStage{state: topologyInspectionPresent, candidates: 1}
			result.device = &cut.devices[i]
			return result
		}
	}
	result.membership.state = topologyInspectionAbsent
	return result
}

func inspectTopologyRemovedRegistration(
	cut *topologySweepDiagnosticCut,
	registrationID ddsnmp.DeviceRegistrationID,
) topologyInspectionRemovedResult {
	var result topologyInspectionRemovedResult
	if cut == nil || cut.captureState != diagnosticCaptureAvailable {
		return result
	}
	for i := range cut.removed {
		if cut.removed[i].registrationID == registrationID {
			result.membership = topologyInspectionStage{state: topologyInspectionPresent, candidates: 1}
			result.device = &cut.removed[i]
			return result
		}
	}
	result.membership.state = topologyInspectionAbsent
	return result
}

func absentTopologyInspectionCapture() topologyInspectionCaptureResult {
	return topologyInspectionCaptureResult{
		membership: topologyInspectionStage{state: topologyInspectionAbsent},
		evidence:   topologyInspectionStage{state: topologyInspectionAbsent},
	}
}

func inspectTopologyCapture(capture *topologyAcquisitionCapture) topologyInspectionCaptureResult {
	if capture == nil {
		return absentTopologyInspectionCapture()
	}
	result := topologyInspectionCaptureResult{
		membership: topologyInspectionStage{state: topologyInspectionPresent, candidates: 1},
		capture:    capture,
	}
	if capture.state == diagnosticCaptureAvailable && capture.evidence != nil {
		result.evidence = topologyInspectionStage{state: topologyInspectionPresent, candidates: 1}
	}
	return result
}

func findTopologyDiagnosticReplayedDevice(
	devices []topologyDiagnosticReplayedDevice,
	registrationID ddsnmp.DeviceRegistrationID,
) *topologyDiagnosticReplayedDevice {
	for i := range devices {
		if devices[i].registrationID == registrationID {
			return &devices[i]
		}
	}
	return nil
}

func inspectTopologyLocalDeviceIdentity(
	data topologymodel.Data,
	localDeviceID string,
	device topologymodel.Device,
) topologyInspectionActorResult {
	index := topologymodel.NewLocalActorMatchIndex()
	for i := range data.Actors {
		if topologyengine.IsDeviceActorType(data.Actors[i].ActorType) {
			index.AddMatch(i, data.Actors[i].Match)
		}
	}
	if indexes := index.MatchIndexes(nil, device); len(indexes) > 0 {
		return topologyInspectionActorsAt(data, indexes)
	}
	actor, ok := topologyLocalActorFromCache(localDeviceID, device)
	if !ok {
		return topologyInspectionActorsAt(data, nil)
	}
	return inspectTopologyActorIdentity(data, "actor:"+actor.ActorID)
}

func inspectTopologyActorIdentity(data topologymodel.Data, identityKey string) topologyInspectionActorResult {
	identityKey = strings.TrimSpace(identityKey)
	indexes := make([]int, 0, 1)
	for i := range data.Actors {
		if topologyInspectionActorHasIdentity(data.Actors[i], identityKey) {
			indexes = append(indexes, i)
		}
	}
	return topologyInspectionActorsAt(data, indexes)
}

func topologyInspectionActorsAt(data topologymodel.Data, indexes []int) topologyInspectionActorResult {
	result := topologyInspectionActorResult{indexes: indexes, index: -1}
	for _, index := range indexes {
		if index >= 0 && index < len(data.Actors) {
			result.actors = append(result.actors, data.Actors[index])
		}
	}
	result.membership.candidates = len(result.actors)
	switch len(result.actors) {
	case 0:
		result.membership.state = topologyInspectionAbsent
	case 1:
		result.membership.state = topologyInspectionPresent
		result.index = indexes[0]
	default:
		result.membership.state = topologyInspectionUndetermined
	}
	return result
}

func topologyInspectionActorHasIdentity(actor topologymodel.Actor, identityKey string) bool {
	if identityKey == "" {
		return false
	}
	if after, ok := strings.CutPrefix(identityKey, "actor:"); ok {
		return strings.TrimSpace(actor.ActorID) == after
	}
	return slices.Contains(topologymodel.MatchIdentityKeys(actor.Match), identityKey)
}

func topologyInspectionPreferredActorIdentity(actor topologymodel.Actor) string {
	keys := topologymodel.MatchIdentityKeys(actor.Match)
	for _, prefix := range []string{"hw:", "chassis:", "ip:", "hostname:", "sysname:", "dns:"} {
		for _, key := range keys {
			if strings.HasPrefix(key, prefix) {
				return key
			}
		}
	}
	if actorID := strings.TrimSpace(actor.ActorID); actorID != "" {
		return "actor:" + actorID
	}
	return ""
}
