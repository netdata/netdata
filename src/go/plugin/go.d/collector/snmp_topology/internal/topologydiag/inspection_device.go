// SPDX-License-Identifier: GPL-3.0-or-later

package topologydiag

import (
	"slices"
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

func inspectLifecycleRegistration(
	cut LifecycleCut,
	registrationID ddsnmp.DeviceRegistrationID,
) inspectionLifecycleResult {
	result := inspectionLifecycleResult{
		captureState:  cut.State,
		captureReason: cut.Reason,
		sequence:      cut.Cut.Sequence,
		capturedAt:    cut.Cut.CapturedAt,
	}
	if cut.State != CaptureAvailable {
		return result
	}
	for i := range cut.Cut.Entries {
		if cut.Cut.Entries[i].RegistrationID == registrationID {
			result.membership = inspectionStage{state: inspectionPresent, candidates: 1}
			result.entry = &cut.Cut.Entries[i]
			return result
		}
	}
	result.membership.state = inspectionAbsent
	return result
}

func inspectSweepRegistration(
	cut *SweepCut,
	registrationID ddsnmp.DeviceRegistrationID,
) inspectionSweepResult {
	var result inspectionSweepResult
	if cut == nil {
		return result
	}
	result.inspectionDiagnosticCutResult = inspectDiagnosticCut(cut)
	if cut.CaptureState != CaptureAvailable {
		return result
	}
	for i := range cut.Devices {
		if cut.Devices[i].RegistrationID == registrationID {
			result.membership = inspectionStage{state: inspectionPresent, candidates: 1}
			result.device = &cut.Devices[i]
			return result
		}
	}
	result.membership.state = inspectionAbsent
	return result
}

func inspectRemovedRegistration(
	cut *SweepCut,
	registrationID ddsnmp.DeviceRegistrationID,
) inspectionRemovedResult {
	var result inspectionRemovedResult
	if cut == nil || cut.CaptureState != CaptureAvailable {
		return result
	}
	for i := range cut.Removed {
		if cut.Removed[i].RegistrationID == registrationID {
			result.membership = inspectionStage{state: inspectionPresent, candidates: 1}
			result.device = &cut.Removed[i]
			return result
		}
	}
	result.membership.state = inspectionAbsent
	return result
}

func absentTopologyInspectionCapture() inspectionCaptureResult {
	return inspectionCaptureResult{
		membership: inspectionStage{state: inspectionAbsent},
		evidence:   inspectionStage{state: inspectionAbsent},
	}
}

func inspectCapture(capture *AcquisitionCapture) inspectionCaptureResult {
	if capture == nil {
		return absentTopologyInspectionCapture()
	}
	result := inspectionCaptureResult{
		membership: inspectionStage{state: inspectionPresent, candidates: 1},
		capture:    capture,
	}
	if capture.State == CaptureAvailable && capture.Evidence != nil {
		result.evidence = inspectionStage{state: inspectionPresent, candidates: 1}
	}
	return result
}

func findDiagnosticReplayedDevice(
	devices []diagnosticReplayedDevice,
	registrationID ddsnmp.DeviceRegistrationID,
) *diagnosticReplayedDevice {
	for i := range devices {
		if devices[i].registrationID == registrationID {
			return &devices[i]
		}
	}
	return nil
}

func inspectLocalDeviceIdentity(
	data topologymodel.Data,
	localDeviceID string,
	device topologymodel.Device,
) inspectionActorResult {
	index := topologymodel.NewLocalActorMatchIndex()
	for i := range data.Actors {
		if topologyengine.IsDeviceActorType(data.Actors[i].ActorType) {
			index.AddMatch(i, data.Actors[i].Match)
		}
	}
	if indexes := index.MatchIndexes(nil, device); len(indexes) > 0 {
		return inspectionActorsAt(data, indexes)
	}
	actorID := topologymodel.LocalActorID(localDeviceID, device)
	if actorID == "" {
		return inspectionActorsAt(data, nil)
	}
	return inspectActorIdentity(data, "actor:"+actorID)
}

func inspectActorIdentity(data topologymodel.Data, identityKey string) inspectionActorResult {
	identityKey = strings.TrimSpace(identityKey)
	indexes := make([]int, 0, 1)
	for i := range data.Actors {
		if inspectionActorHasIdentity(data.Actors[i], identityKey) {
			indexes = append(indexes, i)
		}
	}
	return inspectionActorsAt(data, indexes)
}

func inspectionActorsAt(data topologymodel.Data, indexes []int) inspectionActorResult {
	result := inspectionActorResult{indexes: indexes, index: -1}
	for _, index := range indexes {
		if index >= 0 && index < len(data.Actors) {
			result.actors = append(result.actors, data.Actors[index])
		}
	}
	result.membership.candidates = len(result.actors)
	switch len(result.actors) {
	case 0:
		result.membership.state = inspectionAbsent
	case 1:
		result.membership.state = inspectionPresent
		result.index = indexes[0]
	default:
		result.membership.state = inspectionUndetermined
	}
	return result
}

func inspectionActorHasIdentity(actor topologymodel.Actor, identityKey string) bool {
	if identityKey == "" {
		return false
	}
	if after, ok := strings.CutPrefix(identityKey, "actor:"); ok {
		return strings.TrimSpace(actor.ActorID) == after
	}
	return slices.Contains(topologymodel.MatchIdentityKeys(actor.Match), identityKey)
}

func inspectionPreferredActorIdentity(actor topologymodel.Actor) string {
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
