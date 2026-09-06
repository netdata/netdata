// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/stretchr/testify/require"
)

func TestVLANPreClientCancellationDoesNotReportCollectionFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, progress, err := collectTopologyVLANContext(ctx, newTestSNMPTopologyCollector(), ddsnmp.DeviceConnectionInfo{}, "100", nil, nil)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, topologyAcquisitionPhaseNotObserved, progress.client.outcome)
	require.Equal(t, topologyAcquisitionPhaseNotObserved, progress.connect.outcome)
	require.Equal(t, topologyAcquisitionPhaseNotObserved, progress.collection.outcome)
	require.Equal(t, "cancelled", progress.interruption.Reason)
}

func TestSweepProfileContextYieldsToReplayWithoutMutatingOldCapture(t *testing.T) {
	profile, err := ddsnmp.RestoreProfileContext(
		ddsnmp.ProfileContextData{State: "available", ManualPolicy: "fallback", BGPMode: "absent", SysDescr: "synthetic device"},
		250000,
		64<<20,
	)
	require.NoError(t, err)
	firstRecorder := newTopologyAcquisitionRecorder(
		testTopologyAttemptID(1),
		topologySemanticDeviceInput{},
		testTopologyTarget(),
		defaultTopologyAcquisitionLimits,
	)
	firstRecorder.evidence.profileContext = profile
	first := firstRecorder.finish()
	second := newTopologyAcquisitionRecorder(
		testTopologyAttemptID(2),
		topologySemanticDeviceInput{},
		testTopologyTarget(),
		defaultTopologyAcquisitionLimits,
	).finish()
	contextRecords, contextBytes := profile.Shape()
	usage := topologyAcquisitionUsage{
		limits: topologyAcquisitionLimits{
			maxRecords:      first.recordCount + second.recordCount,
			maxLogicalBytes: first.logicalBytes + second.logicalBytes - contextBytes,
		},
	}
	admittedFirst := usage.include(first)
	require.Equal(t, "available", admittedFirst.evidence.profileContext.Snapshot().State)
	admittedSecond := usage.include(second)
	require.Equal(t, diagnosticCaptureAvailable, admittedFirst.state)
	require.Equal(t, diagnosticCaptureAvailable, admittedSecond.state)
	require.Equal(t, "limit_exceeded", admittedFirst.evidence.profileContext.Snapshot().State)
	require.Equal(t, "available", first.evidence.profileContext.Snapshot().State)
	require.Equal(t, first.recordCount+second.recordCount-contextRecords, usage.recordCount)
	require.Equal(t, first.logicalBytes+second.logicalBytes-contextBytes, usage.logicalBytes)
}

func TestProfileContextRestoreBudgetIsShared(t *testing.T) {
	data := ddsnmp.ProfileContextData{State: "available", ManualPolicy: "fallback", BGPMode: "absent"}
	context, err := ddsnmp.RestoreProfileContext(data, 250000, 64<<20)
	require.NoError(t, err)
	records, size := context.Shape()
	budget := profileContextRestoreBudget{records: 2 * records, bytes: 2 * size}
	_, err = budget.restore(data)
	require.NoError(t, err)
	_, err = budget.restore(data)
	require.NoError(t, err)
	_, err = budget.restore(data)
	require.ErrorContains(t, err, "exceeds limits")
	require.Zero(t, budget.bytes)
	_, err = budget.restore(ddsnmp.ProfileContextData{State: "limit_exceeded"})
	require.NoError(t, err, "unavailable slots are accounted by the enclosing record")
}
