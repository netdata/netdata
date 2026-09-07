// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
	"github.com/stretchr/testify/require"
)

type executionTestHandler struct {
	gosnmp.Handler
	walkRoots []string
	walkPDUs  []gosnmp.SnmpPDU
}

func (*executionTestHandler) Version() gosnmp.SnmpVersion { return gosnmp.Version2c }
func (*executionTestHandler) MaxOids() int                { return 10 }
func (*executionTestHandler) Get(oids []string) (*gosnmp.SnmpPacket, error) {
	packet := &gosnmp.SnmpPacket{}
	for _, oid := range oids {
		packet.Variables = append(
			packet.Variables,
			gosnmp.SnmpPDU{
				Name:  oid,
				Type:  gosnmp.OctetString,
				Value: []byte("synthetic"),
			},
		)
	}
	return packet, nil
}
func (h *executionTestHandler) BulkWalkAll(oid string) ([]gosnmp.SnmpPDU, error) {
	h.walkRoots = append(h.walkRoots, oid)
	return h.walkPDUs, errors.New("synthetic failure")
}

// Use the real collector/observer boundary, including a failed profile with no
// retained values; the archive fixture supplies only the surrounding sweep.
func collectExecutionTestCapture(tb testing.TB) *topologyAcquisitionCapture {
	tb.Helper()
	return collectSourceTestCapture(tb, nil)
}

func collectSourceTestCapture(tb testing.TB, pdus []gosnmp.SnmpPDU) *topologyAcquisitionCapture {
	tb.Helper()
	const root = "1.3.6.1.4.1.99999.1"
	recorder := newTopologyAcquisitionRecorder(topologyAcquisitionAttemptID{
		registrationID: 1,
		ordinal:        2,
	},
		topologySemanticDeviceInput{
			hostname: "192.0.2.1",
		},
		topologyTargetResolutionEvidence{
			outcome: topologyTargetResolutionEmpty,
		})
	handler := &executionTestHandler{walkPDUs: pdus}
	observer := recorder.beginContext(0, "", "")
	collector := ddsnmpcollector.New(ddsnmpcollector.Config{
		SnmpClient:                 recorder.sourceClient(0, handler),
		Log:                        logger.New(),
		InitialAcquisitionObserver: observer,
		Profiles: []*ddsnmp.Profile{{SourceFile: "synthetic.yaml", Definition: &ddprofiledefinition.ProfileDefinition{
			MetricTags: []ddprofiledefinition.GlobalMetricTagConfig{
				{MetricTagConfig: ddprofiledefinition.MetricTagConfig{
					Tag: "site",
					Symbol: ddprofiledefinition.SymbolConfigCompat{
						OID:  root + ".0",
						Name: "site",
					},
				}},
			},
			Topology: []ddprofiledefinition.TopologyConfig{
				{Kind: ddsnmp.KindIfName, MetricsConfig: ddprofiledefinition.MetricsConfig{
					Table: ddprofiledefinition.SymbolConfig{
						OID:  root,
						Name: "interfaces",
					},
					Symbols: []ddprofiledefinition.SymbolConfig{{OID: root + ".1", Name: "interface"}},
				}},
			},
		}}},
	})
	_, err := collector.Collect()
	require.Error(tb, err)
	require.Equal(tb, []string{root}, handler.walkRoots)
	recorder.completeContext(0, failedAcquisitionPhase(topologyAcquisitionFailureCollection))
	return recorder.finish()
}

func TestTopologyExecutionAccountingRetentionArchiveInspection(t *testing.T) {
	capture := collectExecutionTestCapture(t)
	require.Equal(t, diagnosticCaptureAvailable, capture.state)
	profile := capture.evidence.collectionContexts[0].profiles[0]
	require.NotNil(t, profile.execution)
	require.EqualValues(t, 1, profile.execution.Preparation.GetRequests)
	require.Len(t, profile.execution.WalkOperations, 1)
	require.NotEmpty(t, capture.evidence.collectionContexts[0].sources[profile.execution.WalkOperations[0]-1].Failure.Reason)
	require.Empty(t, profile.values)

	scenario := newLLDPDirectScenario()
	_, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	before, ok, err := replayTopologyDiagnostics(diagnostics, scenario.opts)
	require.NoError(t, err)
	require.True(t, ok)
	diagnostics.topology.devices[0].latestAttempt = capture
	var encoded bytes.Buffer
	require.NoError(t, writeTopologyDiagnosticArchiveWithProducerVersion(&encoded, diagnostics, "v-test"))
	archive, err := readTopologyDiagnosticArchive(
		bytes.NewReader(encoded.Bytes()),
		snmpdiag.DefaultReadLimits(),
	)
	require.NoError(t, err)
	restored := archive.diagnostics.topology.devices[0].latestAttempt.evidence.collectionContexts[0].profiles[0]
	require.Equal(t, profile.execution, restored.execution)
	require.Equal(t, profile.stats, restored.stats)
	require.Nil(
		t,
		archive.diagnostics.topology.devices[0].acquisition.evidence.collectionContexts[0].profiles[0].execution,
		"historical retained success must not acquire fabricated measurements",
	)
	inspection, err := inspectTopologyDevice(archive.diagnostics, scenario.opts, 1)
	require.NoError(t, err)
	public, err := newDiagnosticDeviceInspection(inspection)
	require.NoError(t, err)
	require.Equal(
		t,
		newTopologyDiagnosticArchiveExecutionV1(profile.execution),
		public.LatestAttempt.CollectionContexts[0].Profiles[0].Execution,
	)
	require.Nil(t, public.RetainedSuccess.CollectionContexts[0].Profiles[0].Execution)
	after, ok, err := replayTopologyDiagnostics(archive.diagnostics, scenario.opts)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, before, after, "execution evidence must not change graph replay")

	var reencoded bytes.Buffer
	require.NoError(t, writeTopologyDiagnosticArchiveWithProducerVersion(&reencoded, archive.diagnostics, "v-test"))
	require.Equal(t, encoded.Bytes(), reencoded.Bytes())
}

func TestTopologyExecutionAccountingShapeIncludesOperationReferences(t *testing.T) {
	report := ddsnmpcollector.AcquisitionProfileReport{
		Outcome:   ddsnmpcollector.AcquisitionProfileOutcomeFailed,
		Execution: &ddsnmpcollector.AcquisitionExecutionReport{WalkOperations: []uint64{1, 2}},
	}
	records, logicalBytes, err := topologyAcquisitionProfileShape(topologySemanticEventTopologyMetrics, report, nil)
	require.NoError(t, err)
	require.EqualValues(t, 4, records)
	require.EqualValues(t, 96+88+2*8, logicalBytes)
}

func TestTopologyExecutionAccountingPresence(t *testing.T) {
	for name, tc := range map[string]struct{ recorded bool }{"unobserved": {}, "observed": {true}} {
		t.Run(name, func(t *testing.T) {
			recorded := tc.recorded
			profile := topologyAcquisitionProfileEvidence{
				outcome: ddsnmpcollector.AcquisitionProfileOutcomeSuccess,
			}
			if recorded {
				profile.execution = &ddsnmpcollector.AcquisitionExecutionReport{}
			}
			dto, err := newTopologyDiagnosticArchiveProfileEvidenceV1(profile)
			require.NoError(t, err)
			raw, err := json.Marshal(dto)
			require.NoError(t, err)
			var decoded snmpdiag.ProfileEvidence
			require.NoError(t, json.Unmarshal(raw, &decoded))
			restored, err := restoreArchiveProfileEvidence(decoded)
			require.NoError(t, err)
			require.Equal(t, recorded, restored.execution != nil)
			again, err := newTopologyDiagnosticArchiveProfileEvidenceV1(restored)
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

func BenchmarkTopologyExecutionAccounting(b *testing.B) {
	capture := collectExecutionTestCapture(b)
	_, diagnostics := newTopologyScenarioReplayFixture(b, newLLDPDirectScenario())
	diagnostics.topology.devices[0].latestAttempt = capture
	b.Run("archive", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var encoded bytes.Buffer
			if err := writeTopologyDiagnosticArchiveWithProducerVersion(&encoded, diagnostics, "v-benchmark"); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("device_accounting", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := newDiagnosticDeviceCaptureInspection(topologyInspectionCaptureResult{
				capture: capture,
			}); err != nil {
				b.Fatal(err)
			}
		}
	})
}
