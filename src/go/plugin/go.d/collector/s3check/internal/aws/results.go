// SPDX-License-Identifier: GPL-3.0-or-later

package aws

import (
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/contract"
)

func successProbe(owned *entry, writeObjective, deleteObjective time.Duration) *contract.ProbeResult {
	if owned.MarkerAt == nil {
		return contract.FailedProbe(contract.ReasonOwnership)
	}
	end := *owned.MarkerAt
	return &contract.ProbeResult{
		Status:           contract.StatusSuccess,
		Reason:           contract.ReasonNone,
		PayloadCompared:  owned.ObjectSeen,
		WriteVisibility:  writeResult(owned, end, writeObjective),
		DeleteVisibility: deleteResult(owned, end, deleteObjective),
	}
}

func waitingProbe(owned *entry, now time.Time, writeObjective, deleteObjective time.Duration) *contract.ProbeResult {
	return &contract.ProbeResult{
		Status:           contract.StatusWaiting,
		Reason:           contract.ReasonNone,
		PayloadCompared:  owned.ObjectSeen,
		WriteVisibility:  writeResult(owned, now, writeObjective),
		DeleteVisibility: deleteResult(owned, now, deleteObjective),
	}
}

func writeResult(owned *entry, now time.Time, objective time.Duration) contract.ObjectiveResult {
	if !owned.MeasureWrite || owned.PutAt == nil {
		return contract.ObjectiveResult{}
	}
	end := now
	if owned.VisibleAt != nil {
		end = *owned.VisibleAt
	}
	return contract.ObjectiveResultFor(end.Sub(*owned.PutAt), objective)
}

func deleteResult(owned *entry, now time.Time, objective time.Duration) contract.ObjectiveResult {
	if !owned.MeasureDelete || owned.DeleteAt == nil {
		return contract.ObjectiveResult{}
	}
	end := now
	if owned.MarkerAt != nil {
		end = *owned.MarkerAt
	}
	return contract.ObjectiveResultFor(end.Sub(*owned.DeleteAt), objective)
}

func withPayloadComparison(owned *entry, result *contract.ProbeResult) *contract.ProbeResult {
	if owned != nil && owned.ObjectSeen {
		result.PayloadCompared = true
	}
	return result
}
