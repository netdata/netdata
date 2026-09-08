// SPDX-License-Identifier: GPL-3.0-or-later

package topologydiag

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
	"github.com/stretchr/testify/require"
)

func TestTopologyDiagnosticArchiveEnumTablesAreCompleteAndRoundTrip(t *testing.T) {
	tables := map[string]struct {
		names []string
		last  uint8
	}{
		"capture state":   {archiveCaptureStateNames, uint8(CaptureUnavailable)},
		"capture reason":  {archiveCaptureReasonNames, uint8(CaptureReasonProjectionPanic)},
		"device outcome":  {archiveDeviceOutcomeNames, uint8(RefreshOutcomeFailed)},
		"abort reason":    {archiveAbortReasonNames, uint8(DiagnosticAbortPanic)},
		"sweep phase":     {archiveSweepPhaseNames, uint8(DiagnosticSweepPhaseCommit)},
		"target outcome":  {archiveTargetOutcomeNames, uint8(TargetResolutionFailed)},
		"phase outcome":   {archivePhaseOutcomeNames, uint8(AcquisitionPhaseNotObserved)},
		"phase failure":   {archivePhaseFailureNames, uint8(AcquisitionFailureVLANIdentifier)},
		"profile outcome": {archiveProfileOutcomeNames, uint8(ddsnmpcollector.AcquisitionProfileOutcomeFailed)},
		"profile failure": {archiveProfileFailurePhaseNames, uint8(ddsnmpcollector.AcquisitionFailurePhaseTables)},
		"route kind":      {archiveRouteKindNames, uint8(ddsnmpcollector.AcquisitionRouteKindBGPTable)},
		"route source":    {archiveRouteSourceNames, uint8(ddsnmpcollector.AcquisitionRouteSourceCache)},
		"route outcome":   {archiveRouteOutcomeNames, uint8(ddsnmpcollector.AcquisitionRouteOutcomePartial)},
		"route failure":   {archiveRouteFailureClassNames, uint8(ddsnmpcollector.AcquisitionFailureClassDependency)},
	}
	for name, table := range tables {
		t.Run(name, func(t *testing.T) {
			require.Len(t, table.names, int(table.last)+1)
			seen := make(map[string]struct{}, len(table.names))
			for index, name := range table.names {
				require.NotEmpty(t, name)
				_, duplicate := seen[name]
				require.False(t, duplicate)
				seen[name] = struct{}{}
				encoded, err := archiveEnumName(uint8(index), table.names)
				require.NoError(t, err)
				require.Equal(t, name, encoded)
				decoded, err := archiveParseEnum[uint8](encoded, table.names)
				require.NoError(t, err)
				require.Equal(t, uint8(index), decoded)
			}
		})
	}
}

func TestTopologyDiagnosticArchiveV1GoldenDocument(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/topology-diagnostic-archive-v1.json")
	require.NoError(t, err)

	var document snmpdiag.Document
	require.NoError(t, json.Unmarshal(raw, &document))
	restored, err := restoreArchiveDocument(document)
	require.NoError(t, err)
	snapshot, err := NewSnapshot(restored)
	require.NoError(t, err)
	document.Snapshot = snapshot
	encoded, err := json.Marshal(document)
	require.NoError(t, err)
	require.JSONEq(t, string(raw), string(encoded))
}

func TestTopologyExecutionAccountingPresence(t *testing.T) {
	for name, tc := range map[string]struct{ recorded bool }{"unobserved": {}, "observed": {true}} {
		t.Run(name, func(t *testing.T) {
			recorded := tc.recorded
			profile := AcquisitionProfileEvidence{
				Outcome: ddsnmpcollector.AcquisitionProfileOutcomeSuccess,
			}
			if recorded {
				profile.Execution = &ddsnmpcollector.AcquisitionExecutionReport{}
			}
			dto, err := newArchiveProfileEvidenceV1(profile)
			require.NoError(t, err)
			raw, err := json.Marshal(dto)
			require.NoError(t, err)
			var decoded snmpdiag.ProfileEvidence
			require.NoError(t, json.Unmarshal(raw, &decoded))
			restored, err := restoreArchiveProfileEvidence(decoded)
			require.NoError(t, err)
			require.Equal(t, recorded, restored.Execution != nil)
			again, err := newArchiveProfileEvidenceV1(restored)
			require.NoError(t, err)
			require.Equal(t, dto, again)
		})
	}
}

func TestTopologyExecutionAccountingRejectsInvalidMeasurements(t *testing.T) {
	for name, tc := range map[string]struct{ preparation snmpdiag.Preparation }{
		"negative duration":    {snmpdiag.Preparation{ElapsedNanos: -1}},
		"negative error count": {snmpdiag.Preparation{SNMPErrors: -1}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := restoreArchiveExecution(&snmpdiag.Execution{Preparation: tc.preparation})
			require.Error(t, err)
		})
	}
}

func TestArchiveRestorePreservesCaptureSharing(t *testing.T) {
	file, err := os.Open("../../testdata/topology-diagnostic-archive-replay-v1.zst")
	require.NoError(t, err)
	defer file.Close()
	document, err := snmpdiag.Read(file, snmpdiag.DefaultReadLimits())
	require.NoError(t, err)
	restored, err := restoreArchiveDocument(document)
	require.NoError(t, err)
	require.NotSame(t, restored.Topology.Devices[0].LatestAttempt, restored.Topology.Devices[0].Acquisition)
	require.Same(t, restored.Topology.Devices[1].LatestAttempt, restored.Topology.Devices[1].Acquisition)
	snapshot, err := NewSnapshot(restored)
	require.NoError(t, err)
	before, err := json.Marshal(document.Snapshot)
	require.NoError(t, err)
	after, err := json.Marshal(snapshot)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
}
