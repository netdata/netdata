// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologydiag"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyv1test"
	"github.com/stretchr/testify/require"
)

var updateTopologyDiagnosticReplayFixture = flag.Bool(
	"update-topology-diagnostic-replay-fixture",
	false,
	"update the replayable SNMP topology diagnostic archive fixture",
)

func TestTopologyDiagnosticArchiveRoundTripPreservesReplayAndInspection(t *testing.T) {
	scenario := newMixedL2L3ControlScenario()
	registry, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	completeTopologyDiagnosticArchiveFixture(&diagnostics)

	var encoded bytes.Buffer
	require.NoError(t, writeTopologyDiagnosticArchiveWithProducerVersion(
		&encoded,
		diagnostics,
		"v-test",
	))
	archive, err := readTestDiagnosticArchive(
		bytes.NewReader(encoded.Bytes()),
		snmpdiag.DefaultReadLimits(),
	)
	require.NoError(t, err)
	require.Equal(t, "v-test", archive.Identity().ProducerAgentVersion)

	live, ok, err := (funcDepsAdapter{registry: registry}).Snapshot(scenario.opts)
	require.NoError(t, err)
	require.True(t, ok)
	replayed, err := archive.Replay(testDiagnosticQuery(scenario.opts))
	require.NoError(t, err)
	require.Equal(t, topologyv1test.NormalizeData(t, live), topologyv1test.NormalizeData(t, replayed))

	beforeDevice, err := openTestDiagnosticCut(t, diagnostics).InspectDevice(testDiagnosticQuery(scenario.opts), 1)
	require.NoError(t, err)
	afterDevice, err := archive.InspectDevice(testDiagnosticQuery(scenario.opts), 1)
	require.NoError(t, err)
	require.Equal(t, beforeDevice, afterDevice)

	beforeLink, err := openTestDiagnosticCut(t, diagnostics).InspectLinkAt(testDiagnosticQuery(scenario.opts), 0)
	require.NoError(t, err)
	afterLink, err := archive.InspectLinkAt(testDiagnosticQuery(scenario.opts), 0)
	require.NoError(t, err)
	require.Equal(t, beforeLink, afterLink)
}

func TestTopologyDiagnosticArchiveReplayFixture(t *testing.T) {
	_, diagnostics := newTopologyScenarioReplayFixture(t, newLLDPDirectScenario())
	completeTopologyDiagnosticArchiveFixture(&diagnostics)

	var encoded bytes.Buffer
	require.NoError(t, writeTopologyDiagnosticArchiveWithProducerVersion(&encoded, diagnostics, "v-test"))
	path := filepath.Join("testdata", "topology-diagnostic-archive-replay-v1.zst")
	if *updateTopologyDiagnosticReplayFixture {
		require.NoError(t, os.WriteFile(path, encoded.Bytes(), 0o644))
	}
	fixture, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, encoded.Bytes(), fixture)
}

func TestTopologyDiagnosticArchiveRejectsMalformedAndUnsupportedInput(t *testing.T) {
	_, diagnostics := newTopologyScenarioReplayFixture(t, newLLDPDirectScenario())
	completeTopologyDiagnosticArchiveFixture(&diagnostics)
	document, err := newTopologyDiagnosticArchiveDocumentV1(diagnostics, "v-test")
	require.NoError(t, err)

	tests := map[string]struct {
		mutate func(*snmpdiag.Document)
		want   string
	}{
		"format": {
			mutate: func(document *snmpdiag.Document) { document.Format = "other" },
			want:   "unsupported format",
		},
		"version": {
			mutate: func(document *snmpdiag.Document) { document.Version++ },
			want:   "unsupported version",
		},
		"retained reference": {
			mutate: func(document *snmpdiag.Document) {
				document.Snapshot.Topology.Devices[0].RetainedSuccess.RegistrationID++
			},
			want: "does not match owner",
		},
		"capture role": {
			mutate: func(document *snmpdiag.Document) {
				document.Snapshot.Topology.Devices[0].Captures[0].Roles[0] = "other"
			},
			want: "unknown capture role",
		},
		"route reference": {
			mutate: func(document *snmpdiag.Document) {
				profile := &document.Snapshot.Topology.Devices[0].Captures[0].Evidence.CollectionContexts[0].Profiles[0]
				require.NotEmpty(t, profile.Values.Metrics)
				profile.Values.Metrics[0].RouteOrdinal = ^uint32(0)
			},
			want: "references unknown route ordinal",
		},
		"target address": {
			mutate: func(document *snmpdiag.Document) {
				document.Snapshot.Topology.Devices[0].Captures[0].Evidence.Target.Addresses = []string{"not-an-address"}
			},
			want: "target address",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			encoded, err := json.Marshal(document)
			require.NoError(t, err)
			var mutated snmpdiag.Document
			require.NoError(t, json.Unmarshal(encoded, &mutated))
			tc.mutate(&mutated)

			_, err = readTestDiagnosticArchive(
				bytes.NewReader(compressArchiveJSON(t, archiveDocumentJSON(t, mutated))),
				snmpdiag.DefaultReadLimits(),
			)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestTopologyDiagnosticArchivePreservesOpenBGPPeerState(t *testing.T) {
	_, diagnostics := newTopologyScenarioReplayFixture(t, newMixedL2L3ControlScenario())
	document, err := newTopologyDiagnosticArchiveDocumentV1(diagnostics, "v-test")
	require.NoError(t, err)

	const state = "vendor-specific"
	row := firstTopologyDiagnosticArchiveBGPRow(t, &document)
	row.StateHas = true
	row.State = state

	archive, err := readTestDiagnosticArchive(
		bytes.NewReader(compressArchiveJSON(t, archiveDocumentJSON(t, document))),
		snmpdiag.DefaultReadLimits(),
	)
	require.NoError(t, err)
	report, err := archive.InspectLink(testDiagnosticQuery(newMixedL2L3ControlScenario().opts), DiagnosticLinkSubject{
		SourceIdentity: "actor:unknown-a", DestinationIdentity: "actor:unknown-b", Family: "bgp_adjacency", Protocol: "bgp", Direction: "bidirectional",
	})
	require.NoError(t, err)
	var found bool
	for _, context := range report.Source.Contexts {
		for _, capture := range context.Captures {
			for _, fact := range capture.Facts {
				if fact.BGP != nil && fact.BGP.State == state {
					found = true
				}
			}
		}
	}
	require.True(t, found, "open BGP state survives archive restoration and inspection")
}

func TestTopologyDiagnosticArchiveRejectsRetainedReferenceCaptureMismatch(t *testing.T) {
	_, diagnostics := newTopologyScenarioReplayFixture(t, newMixedL2L3ControlScenario())
	completeTopologyDiagnosticArchiveFixture(&diagnostics)
	document, err := newTopologyDiagnosticArchiveDocumentV1(diagnostics, "v-test")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(document.Snapshot.Topology.Devices), 2)

	tests := map[string]struct {
		mutate func(*snmpdiag.Device, uint64)
		want   string
	}{
		"reference without role": {
			mutate: func(device *snmpdiag.Device, _ uint64) {
				require.NotNil(t, device.RetainedSuccess)
				require.Len(t, device.Captures, 1)
				device.Captures[0].Roles = []string{"latest_attempt"}
			},
			want: "retained-success reference and capture role disagree",
		},
		"role without reference": {
			mutate: func(device *snmpdiag.Device, _ uint64) {
				require.NotNil(t, device.RetainedSuccess)
				device.RetainedSuccess = nil
			},
			want: "retained-success reference and capture role disagree",
		},
		"generation newer than sweep": {
			mutate: func(device *snmpdiag.Device, sequence uint64) {
				require.NotNil(t, device.RetainedSuccess)
				device.RetainedSuccess.Generation = sequence + 1
			},
			want: "retained-success generation",
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			mutated := document
			mutated.Snapshot.Topology = cloneTopologyDiagnosticArchiveSweepV1(t, document.Snapshot.Topology)
			device := &mutated.Snapshot.Topology.Devices[1]
			mutate.mutate(device, mutated.Snapshot.Topology.Sequence)

			_, err := readTestDiagnosticArchive(
				bytes.NewReader(compressArchiveJSON(t, archiveDocumentJSON(t, mutated))),
				snmpdiag.DefaultReadLimits(),
			)
			require.ErrorContains(t, err, mutate.want)
		})
	}
}

func TestTopologyDiagnosticArchiveRejectsDuplicateCaptureAttemptOrdinal(t *testing.T) {
	_, diagnostics := newTopologyScenarioReplayFixture(t, newMixedL2L3ControlScenario())
	completeTopologyDiagnosticArchiveFixture(&diagnostics)
	document, err := newTopologyDiagnosticArchiveDocumentV1(diagnostics, "v-test")
	require.NoError(t, err)
	device := &document.Snapshot.Topology.Devices[0]
	require.Len(t, device.Captures, 2)
	device.Captures[1].AttemptOrdinal = device.Captures[0].AttemptOrdinal

	_, err = readTestDiagnosticArchive(
		bytes.NewReader(compressArchiveJSON(t, archiveDocumentJSON(t, document))),
		snmpdiag.DefaultReadLimits(),
	)
	require.ErrorContains(t, err, "duplicate capture attempt ordinal")
}

func TestTopologyDiagnosticArchiveRejectsTrailingAndTruncatedContent(t *testing.T) {
	_, diagnostics := newTopologyScenarioReplayFixture(t, newLLDPDirectScenario())
	document, err := newTopologyDiagnosticArchiveDocumentV1(diagnostics, "v-test")
	require.NoError(t, err)
	validJSON := archiveDocumentJSON(t, document)

	_, err = readTestDiagnosticArchive(
		bytes.NewReader(compressArchiveJSON(t, validJSON+"{}")),
		snmpdiag.DefaultReadLimits(),
	)
	require.Error(t, err)

	encoded := compressArchiveJSON(t, validJSON)
	require.Greater(t, len(encoded), 8)
	_, err = readTestDiagnosticArchive(
		bytes.NewReader(encoded[:len(encoded)-4]),
		snmpdiag.DefaultReadLimits(),
	)
	require.Error(t, err)

	corrupt := append([]byte(nil), encoded...)
	corrupt[len(corrupt)-1] ^= 0xff
	_, err = readTestDiagnosticArchive(
		bytes.NewReader(corrupt),
		snmpdiag.DefaultReadLimits(),
	)
	require.Error(t, err)
}

func TestTopologyDiagnosticArchiveReadLimitsHaveGenerousDefaults(t *testing.T) {
	limits := snmpdiag.DefaultReadLimits()
	require.Equal(t, int64(128<<20), limits.MaxCompressedBytes)
	require.Equal(t, int64(512<<20), limits.MaxDecodedBytes)
}

func TestTopologyDiagnosticArchiveEnforcesCallerByteLimits(t *testing.T) {
	_, diagnostics := newTopologyScenarioReplayFixture(t, newLLDPDirectScenario())
	completeTopologyDiagnosticArchiveFixture(&diagnostics)
	document, err := newTopologyDiagnosticArchiveDocumentV1(diagnostics, "v-test")
	require.NoError(t, err)
	encoded := compressArchiveJSON(t, archiveDocumentJSON(t, document))

	for _, limit := range []int64{1, int64(len(encoded) - 1)} {
		limits := snmpdiag.DefaultReadLimits()
		limits.MaxCompressedBytes = limit
		_, err = readTestDiagnosticArchive(bytes.NewReader(encoded), limits)
		require.ErrorIs(t, err, snmpdiag.ErrCompressedLimit)
	}

	limits := snmpdiag.DefaultReadLimits()
	limits.MaxDecodedBytes = 64
	_, err = readTestDiagnosticArchive(bytes.NewReader(encoded), limits)
	require.ErrorIs(t, err, snmpdiag.ErrDecodedLimit)
}

func TestTopologyDiagnosticArchiveWriterPropagatesDestinationFailure(t *testing.T) {
	_, diagnostics := newTopologyScenarioReplayFixture(t, newLLDPDirectScenario())
	destinationErr := errors.New("destination failure")

	err := writeTopologyDiagnosticArchiveWithProducerVersion(
		topologyDiagnosticArchiveErrorWriter{err: destinationErr},
		diagnostics,
		"v-test",
	)
	require.ErrorIs(t, err, destinationErr)
}

func TestTopologyDiagnosticArchiveUsesStandardJSONFieldSemantics(t *testing.T) {
	_, diagnostics := newTopologyScenarioReplayFixture(t, newLLDPDirectScenario())
	document, err := newTopologyDiagnosticArchiveDocumentV1(diagnostics, "v-test")
	require.NoError(t, err)
	raw := archiveDocumentJSON(t, document)
	raw = strings.Replace(raw, `"version":1`, `"ignored":true,"version":1`, 1)
	raw = strings.Replace(raw, `"version":1`, `"version":99,"version":1`, 1)
	raw = strings.Replace(raw, `"format":`, `"FoRmAt":`, 1)

	_, err = readTestDiagnosticArchive(
		bytes.NewReader(compressArchiveJSON(t, raw)),
		snmpdiag.DefaultReadLimits(),
	)
	require.NoError(t, err)
}

func TestTopologyDiagnosticArchiveRejectsInvalidWireUTF8(t *testing.T) {
	_, diagnostics := newTopologyScenarioReplayFixture(t, newLLDPDirectScenario())
	document, err := newTopologyDiagnosticArchiveDocumentV1(diagnostics, "v-test")
	require.NoError(t, err)
	raw := archiveDocumentJSON(t, document)

	invalidUTF8 := strings.Replace(raw, `"v-test"`, `"`+string([]byte{0xff})+`"`, 1)
	_, err = readTestDiagnosticArchive(
		bytes.NewReader(compressArchiveJSON(t, invalidUTF8)),
		snmpdiag.DefaultReadLimits(),
	)
	require.ErrorContains(t, err, "invalid UTF-8")
}

func TestTopologyDiagnosticArchiveWriterPreservesV1StringSemantics(t *testing.T) {
	_, diagnostics := newTopologyScenarioReplayFixture(t, newLLDPDirectScenario())
	producerVersion := "v<&>" + string([]byte{0xff})
	var encoded bytes.Buffer
	require.NoError(t, writeTopologyDiagnosticArchiveWithProducerVersion(&encoded, diagnostics, producerVersion))

	raw := decompressArchiveJSON(t, encoded.Bytes())
	require.Contains(t, raw, `"agent_version":"v<&>`)
	require.NotContains(t, raw, `\u003c`)
	require.NotContains(t, raw, `\u003e`)
	require.NotContains(t, raw, `\u0026`)
	archive, err := readTestDiagnosticArchive(
		bytes.NewReader(encoded.Bytes()),
		snmpdiag.DefaultReadLimits(),
	)
	require.NoError(t, err)
	require.Equal(t, "v<&>\ufffd", archive.Identity().ProducerAgentVersion)
}

func FuzzReadTopologyDiagnosticArchive(f *testing.F) {
	seed := compressArchiveJSON(f, `{
		"format":"netdata.snmp_topology.diagnostics",
		"version":1,
		"producer":{"agent_version":"v-test"},
		"snapshot":{
			"job_lifecycle_cut":{"capture_state":"available","capture_reason":"none","cut":{"sequence":1,"captured_at":"2026-08-29T00:00:00Z"}},
			"producer_scope_id":"scope"
		}
	}`)
	f.Add(seed)
	f.Fuzz(func(t *testing.T, input []byte) {
		limits := snmpdiag.ReadLimits{
			MaxCompressedBytes: 1 << 20,
			MaxDecodedBytes:    4 << 20,
		}
		_, _ = readTestDiagnosticArchive(bytes.NewReader(input), limits)
	})
}

func completeTopologyDiagnosticArchiveFixture(diagnostics *topologydiag.Cut) {
	if diagnostics == nil || diagnostics.Topology == nil {
		return
	}
	capturedAt := diagnostics.Topology.PublishedAt.Add(time.Second)
	diagnostics.Lifecycle = topologydiag.LifecycleCut{
		State:  topologydiag.CaptureAvailable,
		Reason: topologydiag.CaptureReasonNone,
		Cut: ddsnmp.DeviceLifecycleCut{
			Sequence:   7,
			CapturedAt: capturedAt,
			Entries:    make([]ddsnmp.DeviceLifecycleEntry, 0, len(diagnostics.Topology.Devices)),
		},
	}
	for _, device := range diagnostics.Topology.Devices {
		diagnostics.Lifecycle.Cut.Entries = append(diagnostics.Lifecycle.Cut.Entries, ddsnmp.DeviceLifecycleEntry{
			RegistrationID: device.RegistrationID,
			Info: ddsnmp.DeviceLifecycleInfo{
				Hostname:    "192.0.2." + device.RegistrationID.String(),
				Port:        161,
				SNMPVersion: "2c",
			},
			LastCompleted: ddsnmp.DeviceLifecycleStatus{
				Phase:       ddsnmp.DeviceLifecyclePhaseCollect,
				Outcome:     ddsnmp.DeviceLifecycleOutcomeSuccess,
				CompletedAt: capturedAt.Add(-time.Second),
			},
			TopologyReady: true,
		})
	}
	if len(diagnostics.Topology.Devices) > 0 {
		device := &diagnostics.Topology.Devices[0]
		device.LatestAttempt = &topologydiag.AcquisitionCapture{
			AttemptID: topologydiag.AcquisitionAttemptID{
				RegistrationID: device.RegistrationID,
				Ordinal:        device.Acquisition.AttemptID.Ordinal + 1,
			},
			State:  topologydiag.CaptureUnavailable,
			Reason: topologydiag.CaptureReasonProjectionError,
		}
	}
	diagnostics.LastAborted = &topologydiag.AbortedSweep{
		Sequence:              3,
		StartedAt:             capturedAt.Add(time.Minute),
		AbortedAt:             capturedAt.Add(time.Minute + time.Second),
		Reason:                topologydiag.DiagnosticAbortCanceled,
		Phase:                 topologydiag.DiagnosticSweepPhaseDeviceRefresh,
		ActiveRegistrationID:  diagnostics.Topology.Devices[0].RegistrationID,
		HasActiveRegistration: true,
		RegistrationCount:     len(diagnostics.Topology.Devices),
		SelectedCount:         len(diagnostics.Topology.Devices),
	}
}

func archiveDocumentJSON(t testing.TB, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return string(encoded)
}

func compressArchiveJSON(t testing.TB, value string) []byte {
	t.Helper()
	return compressArchiveJSONWithWindow(t, value, 1<<20)
}

func compressArchiveJSONWithWindow(t testing.TB, value string, window int) []byte {
	t.Helper()
	var encoded bytes.Buffer
	encoder, err := zstd.NewWriter(
		&encoded,
		zstd.WithEncoderConcurrency(1),
		zstd.WithWindowSize(window),
		zstd.WithEncoderCRC(true),
	)
	require.NoError(t, err)
	_, err = encoder.Write([]byte(value))
	require.NoError(t, err)
	require.NoError(t, encoder.Close())
	return encoded.Bytes()
}

func decompressArchiveJSON(t testing.TB, encoded []byte) string {
	t.Helper()
	decoder, err := zstd.NewReader(bytes.NewReader(encoded), zstd.WithDecoderConcurrency(1))
	require.NoError(t, err)
	decompressed, err := io.ReadAll(decoder)
	decoder.Close()
	require.NoError(t, err)
	return string(decompressed)
}

func firstTopologyDiagnosticArchiveBGPRow(
	t testing.TB,
	document *snmpdiag.Document,
) *snmpdiag.BGPRowValue {
	t.Helper()
	require.NotNil(t, document)
	require.NotNil(t, document.Snapshot.Topology)
	for deviceIndex := range document.Snapshot.Topology.Devices {
		device := &document.Snapshot.Topology.Devices[deviceIndex]
		for captureIndex := range device.Captures {
			capture := &device.Captures[captureIndex]
			if capture.Evidence == nil {
				continue
			}
			for contextIndex := range capture.Evidence.CollectionContexts {
				context := &capture.Evidence.CollectionContexts[contextIndex]
				for profileIndex := range context.Profiles {
					profile := &context.Profiles[profileIndex]
					if len(profile.Values.BGPRows) > 0 {
						return &profile.Values.BGPRows[0]
					}
				}
			}
		}
	}
	t.Fatal("archive fixture has no BGP row")
	return nil
}

func cloneTopologyDiagnosticArchiveSweepV1(
	t testing.TB,
	sweep *snmpdiag.Sweep,
) *snmpdiag.Sweep {
	t.Helper()
	require.NotNil(t, sweep)
	raw, err := json.Marshal(sweep)
	require.NoError(t, err)
	var cloned snmpdiag.Sweep
	require.NoError(t, json.Unmarshal(raw, &cloned))
	return &cloned
}

type topologyDiagnosticArchiveErrorWriter struct {
	err error
}

func (w topologyDiagnosticArchiveErrorWriter) Write([]byte) (int, error) {
	return 0, w.err
}
