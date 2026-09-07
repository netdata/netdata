// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"bytes"
	"net/netip"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
	"github.com/stretchr/testify/require"
)

func TestArchiveAcquisitionSharesProfileBudget(t *testing.T) {
	for name, tc := range map[string]struct {
		recordShortfall, byteShortfall uint64
		wantError                      bool
	}{
		"exact fit":       {},
		"one record over": {recordShortfall: 1, wantError: true},
		"one byte over":   {byteShortfall: 1, wantError: true},
	} {
		t.Run(name, func(t *testing.T) {
			limits := topologyAcquisitionLimits{
				maxRecords:      snmpdiag.MaxRecords,
				maxLogicalBytes: snmpdiag.MaxLogicalBytes,
			}
			recorder := newTopologyAcquisitionRecorder(
				topologyAcquisitionAttemptID{
					registrationID: 1,
					ordinal:        1,
				},
				topologySemanticDeviceInput{
					hostname:    "192.0.2.1",
					vnodeLabels: map[string]string{"site": "lab"},
				},
				topologyTargetResolutionEvidence{
					outcome:   topologyTargetResolutionLiteral,
					addresses: []netip.Addr{netip.MustParseAddr("192.0.2.1")},
				},
				limits,
			)
			for ordinal, context := range []struct{ vlanID, vlanName string }{{}, {"100", "lab"}} {
				observer := recorder.beginContext(uint32(ordinal), context.vlanID, context.vlanName)
				require.NotNil(t, observer)
				for _, pm := range benchmarkTopologyAcquisitionMetrics(2) {
					pm.DeviceMetadata = map[string]ddsnmp.MetaTag{"vendor": {Value: "synthetic"}}
					pm.Tags = map[string]string{tagLldpLocSysName: "synthetic"}
					pm.BGPRows = []ddsnmp.BGPRow{{
						OriginProfileID: "synthetic.yaml", Kind: ddprofiledefinition.BGPRowKindPeer,
						Identity: ddsnmp.BGPIdentity{
							Neighbor: "192.0.2.2",
						},
						State: ddsnmp.BGPState{
							Has:   true,
							State: ddprofiledefinition.BGPPeerStateEstablished,
							Raw:   "6",
						},
						Tags: map[string]string{"neighbor": "192.0.2.2"},
					}}
					report := acquisitionReportForMetrics(0, ddsnmpcollector.AcquisitionProfileOutcomeSuccess, pm)
					report.Execution = &ddsnmpcollector.AcquisitionExecutionReport{
						Walks: []ddsnmpcollector.AcquisitionWalkReport{{RootOID: "1.3.6.1.2.1"}},
					}
					observer.ObserveProfile(report, pm)
				}
				recorder.completeContext(uint32(ordinal), successfulAcquisitionPhase())
			}
			context, err := ddsnmp.RestoreProfileContext(ddsnmp.ProfileContextData{
				State:        "available",
				ManualPolicy: "fallback",
				BGPMode:      "absent",
				SysDescr:     "synthetic device",
			}, limits.maxRecords, limits.maxLogicalBytes)
			require.NoError(t, err)
			recorder.evidence.profileContext, recorder.evidence.vlanProfileContext = context, context
			capture := recorder.finish()
			require.Equal(t, diagnosticCaptureAvailable, capture.state)
			wire, err := newTopologyDiagnosticArchiveAcquisitionEvidenceV1(capture.evidence)
			require.NoError(t, err)
			budget := diagnosticRestoreBudget{
				records: capture.recordCount - tc.recordShortfall,
				bytes:   capture.logicalBytes - tc.byteShortfall,
			}
			restored, err := restoreArchiveAcquisitionEvidence(wire, capture.attemptID, &budget)
			if tc.wantError {
				require.ErrorContains(t, err, "limit")
				return
			}
			require.NoError(t, err)
			require.Zero(t, budget.records)
			require.Zero(t, budget.bytes)
			require.Equal(t, context.Snapshot(), restored.profileContext.Snapshot())
			require.Equal(t, context.Snapshot(), restored.vlanProfileContext.Snapshot())
		})
	}
}

func TestArchiveSweepRejectsProfileBudgetOverflow(t *testing.T) {
	for name, tc := range map[string]struct{ forgedCounters bool }{
		"reported usage":    {},
		"forged zero usage": {forgedCounters: true},
	} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile("testdata/topology-diagnostic-archive-replay-v1.zst")
			require.NoError(t, err)
			document, err := snmpdiag.Read(bytes.NewReader(raw), snmpdiag.DefaultReadLimits())
			require.NoError(t, err)
			_, err = InspectDiagnosticDocument(document)
			require.NoError(t, err)
			context := ddsnmp.ProfileContextData{
				State:          "available",
				ManualPolicy:   "fallback",
				BGPMode:        "absent",
				ManualProfiles: make([]string, snmpdiag.MaxRecords-1),
			}
			for i := range context.ManualProfiles {
				context.ManualProfiles[i] = "synthetic"
			}
			document.Snapshot.Topology.Devices[0].Captures[0].Evidence.ProfileContext = context
			document.Snapshot.Topology.RecordCount += snmpdiag.MaxRecords
			if tc.forgedCounters {
				document.Snapshot.Topology.RecordCount, document.Snapshot.Topology.LogicalBytes = 0, 0
				for i := range document.Snapshot.Topology.Devices {
					for j := range document.Snapshot.Topology.Devices[i].Captures {
						capture := &document.Snapshot.Topology.Devices[i].Captures[j]
						capture.RecordCount, capture.LogicalBytes = 0, 0
					}
				}
			}
			var encoded bytes.Buffer
			require.NoError(t, snmpdiag.Write(&encoded, document))
			decoded, err := snmpdiag.Read(&encoded, snmpdiag.DefaultReadLimits())
			require.NoError(t, err)
			_, err = InspectDiagnosticDocument(decoded)
			require.ErrorContains(t, err, "limit")
		})
	}
}
