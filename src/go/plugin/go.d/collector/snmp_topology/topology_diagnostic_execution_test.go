// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"bytes"
	"errors"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologydiag"
	"github.com/stretchr/testify/require"
)

type executionTestHandler struct {
	gosnmp.Handler
	walkRoots []string
	walkPDUs  []gosnmp.SnmpPDU
}

func (*executionTestHandler) Version() gosnmp.SnmpVersion { return gosnmp.Version2c }

func (*executionTestHandler) MaxOids() int { return 10 }

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
func collectExecutionTestCapture(tb testing.TB) *topologydiag.AcquisitionCapture {
	tb.Helper()
	return collectSourceTestCapture(tb, nil)
}

func collectSourceTestCapture(tb testing.TB, pdus []gosnmp.SnmpPDU) *topologydiag.AcquisitionCapture {
	tb.Helper()
	const root = "1.3.6.1.4.1.99999.1"
	recorder := newTopologyAcquisitionRecorder(topologydiag.AcquisitionAttemptID{
		RegistrationID: 1,
		Ordinal:        2,
	},
		topologydiag.DeviceInput{
			Hostname: "192.0.2.1",
		},
		topologydiag.TargetResolutionEvidence{
			Outcome: topologydiag.TargetResolutionEmpty,
		})
	handler := &executionTestHandler{walkPDUs: pdus}
	observer := recorder.beginContext(0, "", "")
	collector := ddsnmpcollector.New(ddsnmpcollector.Config{
		SnmpClient:                 recorder.sourceClient(handler),
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
	recorder.completeContext(0, failedAcquisitionPhase(topologydiag.AcquisitionFailureCollection))
	return recorder.finish()
}

func TestTopologyExecutionAccountingRetentionArchiveInspection(t *testing.T) {
	capture := collectExecutionTestCapture(t)
	require.Equal(t, topologydiag.CaptureAvailable, capture.State)
	profile := capture.Evidence.CollectionContexts[0].Profiles[0]
	require.NotNil(t, profile.Execution)
	require.EqualValues(t, 1, profile.Execution.Preparation.GetRequests)
	require.Len(t, profile.Execution.WalkOperations, 1)
	require.NotEmpty(t, capture.Evidence.CollectionContexts[0].Sources[profile.Execution.WalkOperations[0]-1].Failure.Reason)
	require.Empty(t, profile.Values)

	scenario := newLLDPDirectScenario()
	_, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	before, err := openTestDiagnosticCut(t, diagnostics).Replay(testDiagnosticQuery(scenario.opts))
	require.NoError(t, err)
	diagnostics.Topology.Devices[0].LatestAttempt = capture
	var encoded bytes.Buffer
	require.NoError(t, writeTopologyDiagnosticArchiveWithProducerVersion(&encoded, diagnostics, "v-test"))
	archive, err := readTestDiagnosticArchive(
		bytes.NewReader(encoded.Bytes()),
		snmpdiag.DefaultReadLimits(),
	)
	require.NoError(t, err)
	public, err := archive.InspectDevice(testDiagnosticQuery(scenario.opts), 1)
	require.NoError(t, err)
	document, err := newTopologyDiagnosticArchiveDocumentV1(diagnostics, "v-test")
	require.NoError(t, err)
	expected := testLatestArchiveCapture(t, &document, 0).Evidence.CollectionContexts[0].Profiles[0]
	require.Equal(t, expected.Execution, public.LatestAttempt.CollectionContexts[0].Profiles[0].Execution)
	require.Equal(t, expected.Stats, public.LatestAttempt.CollectionContexts[0].Profiles[0].Stats)
	require.Nil(t, public.RetainedSuccess.CollectionContexts[0].Profiles[0].Execution)
	after, err := archive.Replay(testDiagnosticQuery(scenario.opts))
	require.NoError(t, err)
	require.Equal(t, before, after, "execution evidence must not change graph replay")

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

func BenchmarkTopologyExecutionAccounting(b *testing.B) {
	capture := collectExecutionTestCapture(b)
	_, diagnostics := newTopologyScenarioReplayFixture(b, newLLDPDirectScenario())
	diagnostics.Topology.Devices[0].LatestAttempt = capture
	b.Run("archive", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var encoded bytes.Buffer
			if err := writeTopologyDiagnosticArchiveWithProducerVersion(&encoded, diagnostics, "v-benchmark"); err != nil {
				b.Fatal(err)
			}
		}
	})
	archive := openTestDiagnosticCut(b, diagnostics)
	b.Run("device_inspection", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := archive.InspectDevice(testDiagnosticQuery(newLLDPDirectScenario().opts), 1); err != nil {
				b.Fatal(err)
			}
		}
	})
}
