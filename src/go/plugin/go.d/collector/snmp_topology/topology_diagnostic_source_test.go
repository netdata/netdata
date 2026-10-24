// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologydiag"
	"github.com/stretchr/testify/require"
)

func TestDiagnosticCaptureKeepsLargeEvidence(t *testing.T) {
	for name, tc := range map[string]struct{ devices, descriptionBytes int }{
		"beyond former attempt bytes": {devices: 1, descriptionBytes: 32 << 20},
		"beyond former sweep bytes":   {devices: 2, descriptionBytes: 33 << 20},
	} {
		t.Run(name, func(t *testing.T) {
			description := strings.Repeat("x", tc.descriptionBytes)
			entries := make([]ddsnmp.DeviceEntry, 0, tc.devices)
			states := make(map[ddsnmp.DeviceRegistrationID]deviceRefreshState)
			for i := 0; i < tc.devices; i++ {
				id := ddsnmp.DeviceRegistrationID(i + 1)
				recorder := newTopologyAcquisitionRecorder(topologydiag.AcquisitionAttemptID{RegistrationID: id, Ordinal: 1}, topologydiag.DeviceInput{Hostname: "192.0.2.1", SysDescr: description}, testTopologyTarget())
				recorder.beginContext(0, "", "")
				capture := recorder.finish()
				require.Equal(t, topologydiag.CaptureAvailable, capture.State)
				require.Equal(t, description, capture.Evidence.Device.SysDescr)
				entries = append(entries, ddsnmp.DeviceEntry{RegistrationID: id})
				states[id] = deviceRefreshState{latestAttempt: capture, attemptOrdinal: 1, outcome: topologydiag.RefreshOutcomeFailed}
			}
			cut, err := projectTopologyDiagnosticCut(topologyDiagnosticCutInput{sequence: 1, startedAt: time.Now(), publishedAt: time.Now(), entries: entries, states: states})
			require.NoError(t, err)
			require.Equal(t, topologydiag.CaptureAvailable, cut.CaptureState)
			var archive bytes.Buffer
			require.NoError(t, writeTopologyDiagnosticArchiveWithProducerVersion(&archive, topologydiag.Cut{Topology: cut}, "test"))
			document, err := snmpdiag.Read(&archive, snmpdiag.DefaultReadLimits())
			require.NoError(t, err)
			restored, err := InspectDiagnosticDocument(document)
			require.NoError(t, err)
			summary, err := restored.Summary()
			require.NoError(t, err)
			require.Len(t, summary.Registrations, tc.devices)
			for _, device := range document.Snapshot.Topology.Devices {
				require.Equal(t, description, device.Captures[0].Evidence.Device.SysDescr)
			}
		})
	}
}

func TestSourceEvidenceArchiveAndInspection(t *testing.T) {
	pdus := []gosnmp.SnmpPDU{
		{Name: "1.3.6.1.4.1.99999.1.1.7", Type: gosnmp.OctetString, Value: []byte{0, 255}},
		{Name: "1.3.6.1.4.1.99999.1.1.7", Type: gosnmp.Counter64, Value: uint64(math.MaxUint64)},
		{Name: "1.3.6.1.4.1.99999.1.99", Type: gosnmp.NoSuchInstance},
	}
	capture := collectSourceTestCapture(t, pdus)
	scenario := newLLDPDirectScenario()
	_, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	diagnostics.Topology.Devices[0].LatestAttempt = capture
	var encoded bytes.Buffer
	require.NoError(t, writeTopologyDiagnosticArchiveWithProducerVersion(&encoded, diagnostics, "test"))
	archive, err := readTestDiagnosticArchive(bytes.NewReader(encoded.Bytes()), snmpdiag.DefaultReadLimits())
	require.NoError(t, err)
	inspected, err := archive.InspectDevice(testDiagnosticQuery(scenario.opts), 1)
	require.NoError(t, err)
	sources := inspected.LatestAttempt.CollectionContexts[0].Sources
	require.Equal(t, capture.Evidence.CollectionContexts[0].Sources, sources)
	require.Len(t, sources, 2)
	require.Len(t, sources[1].PDUs, 3, "partial duplicate and exceptional results survive downstream rejection")
	require.Equal(t, []uint64{2}, inspected.LatestAttempt.CollectionContexts[0].Profiles[0].Execution.WalkOperations)
}

func TestSourceEvidenceImportRejectsInvalidReferences(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(*snmpdiag.ContextEvidence)
	}{
		"duplicate execution charge":        {func(c *snmpdiag.ContextEvidence) { c.Profiles[0].Execution.WalkOperations = []uint64{2, 2} }},
		"execution outside context":         {func(c *snmpdiag.ContextEvidence) { c.Profiles[0].Execution.WalkOperations = []uint64{3} }},
		"GET charged as WALK":               {func(c *snmpdiag.ContextEvidence) { c.Profiles[0].Execution.WalkOperations = []uint64{1} }},
		"consumer outside context":          {func(c *snmpdiag.ContextEvidence) { c.Profiles[0].Routes[0].Sources[0].Operation = 3 }},
		"consumer references wrong request": {func(c *snmpdiag.ContextEvidence) { c.Profiles[0].Routes[0].Sources[0].OID = "1.9" }},
	} {
		t.Run(name, func(t *testing.T) {
			capture := collectExecutionTestCapture(t)
			_, cut := newTopologyScenarioReplayFixture(t, newLLDPDirectScenario())
			cut.Topology.Devices[0].LatestAttempt = capture
			document, err := newTopologyDiagnosticArchiveDocumentV1(cut, "v-test")
			require.NoError(t, err)
			// Decode a separate document, as an imported archive would, before corrupting it.
			data, err := json.Marshal(document)
			require.NoError(t, err)
			var imported snmpdiag.Document
			require.NoError(t, json.Unmarshal(data, &imported))
			tc.mutate(&testLatestArchiveCapture(t, &imported, 0).Evidence.CollectionContexts[0])
			_, err = InspectDiagnosticDocument(imported)
			require.Error(t, err)
		})
	}
}

func TestVLANSourceEvidenceUsesActualContextCollection(t *testing.T) {
	for name, tc := range map[string]struct {
		vlan   string
		failed bool
	}{"successful VLAN": {vlan: "100"}, "failed VLAN with partial data": {vlan: "200", failed: true}} {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			handler := snmpmock.NewMockHandler(ctrl)
			dev := ddsnmp.DeviceConnectionInfo{Hostname: "192.0.2.1", Port: 161, Community: "synthetic", SNMPVersion: gosnmp.Version2c.String()}
			expectTopologyVLANClient(handler, dev, tc.vlan, nil)
			handler.EXPECT().Version().Return(gosnmp.Version2c)
			const root = "1.3.6.1.4.1.99999.1"
			var walkErr error
			if tc.failed {
				walkErr = context.DeadlineExceeded
			}
			pdus := []gosnmp.SnmpPDU{{Name: root + ".1.7", Type: gosnmp.Integer, Value: 7}}
			handler.EXPECT().BulkWalkAll(root).Return(pdus, walkErr)
			handler.EXPECT().Close().Return(nil)
			collector := newTestSNMPTopologyCollector()
			collector.newSnmpClient = func() gosnmp.Handler { return handler }
			profiles := []*ddsnmp.Profile{{SourceFile: "synthetic.yaml", Definition: &ddprofiledefinition.ProfileDefinition{Topology: []ddprofiledefinition.TopologyConfig{{Kind: ddsnmp.KindIfName, MetricsConfig: ddprofiledefinition.MetricsConfig{Table: ddprofiledefinition.SymbolConfig{OID: root, Name: "table"}, Symbols: []ddprofiledefinition.SymbolConfig{{OID: root + ".1", Name: "value"}}}}}}}}
			recorder := newTopologyAcquisitionRecorder(testTopologyAttemptID(1), topologyDeviceInputFromConnection(dev), testTopologyTarget())
			observer := recorder.beginContext(1, tc.vlan, "synthetic")
			_, progress, err := collectTopologyVLANContext(context.Background(), collector, dev, tc.vlan, profiles, observer)
			require.Equal(t, tc.failed, err != nil)
			require.Len(t, progress.sources, 1)
			require.Equal(t, []string{root}, progress.sources[0].RequestedOIDs)
			require.Len(t, progress.sources[0].PDUs, 1)
			require.Equal(t, "7", progress.sources[0].PDUs[0].Value.Text)
			require.Equal(t, tc.failed, progress.sources[0].Failure.Reason != "")
			require.Equal(t, []uint64{1}, recorder.contextByOrdinal(1).Profiles[0].Execution.WalkOperations)
		})
	}
}

func BenchmarkTopologySourceEvidenceMemory(b *testing.B) {
	for name, tc := range map[string]struct{ pdus int }{"4096 PDUs": {4096}, "100000 PDUs": {100000}} {
		b.Run(name, func(b *testing.B) {
			pdus := make([]gosnmp.SnmpPDU, tc.pdus)
			for i := range pdus {
				pdus[i] = gosnmp.SnmpPDU{Name: fmt.Sprintf("1.3.6.1.4.1.99999.1.1.%d", i), Type: gosnmp.Counter64, Value: uint64(math.MaxUint64)}
			}
			for range b.N {
				runtime.GC()
				var before runtime.MemStats
				runtime.ReadMemStats(&before)
				var peak atomic.Uint64
				peak.Store(before.HeapAlloc)
				done, stopped := make(chan struct{}), make(chan struct{})
				stop := sync.OnceFunc(func() { close(done); <-stopped })
				b.Cleanup(stop)
				go func() {
					defer close(stopped)
					ticker := time.NewTicker(time.Millisecond)
					defer ticker.Stop()
					for {
						select {
						case <-done:
							return
						case <-ticker.C:
							var current runtime.MemStats
							runtime.ReadMemStats(&current)
							for old := peak.Load(); current.HeapAlloc > old; old = peak.Load() {
								if peak.CompareAndSwap(old, current.HeapAlloc) {
									break
								}
							}
						}
					}
				}()
				capture := collectSourceTestCapture(b, pdus)
				runtime.GC()
				var retained runtime.MemStats
				runtime.ReadMemStats(&retained)
				_, diagnostics := newTopologyScenarioReplayFixture(b, newLLDPDirectScenario())
				diagnostics.Topology.Devices[0].LatestAttempt = capture
				var archive bytes.Buffer
				if err := writeTopologyDiagnosticArchiveWithProducerVersion(&archive, diagnostics, "benchmark"); err != nil {
					b.Fatal(err)
				}
				restored, err := readTestDiagnosticArchive(bytes.NewReader(archive.Bytes()), snmpdiag.DefaultReadLimits())
				if err != nil {
					b.Fatal(err)
				}
				stop()
				b.ReportMetric(float64(int64(retained.HeapAlloc)-int64(before.HeapAlloc)), "retained-B")
				b.ReportMetric(float64(peak.Load()-before.HeapAlloc), "sampled-peak-B")
				b.ReportMetric(float64(archive.Len()), "archive-B")
				runtime.KeepAlive(capture)
				runtime.KeepAlive(restored)
				runtime.KeepAlive(pdus)
			}
		})
	}
}
