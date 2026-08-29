// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyv1test"
	"github.com/stretchr/testify/require"
)

func TestTopologyDiagnosticArchiveRoundTripPreservesReplayAndInspection(t *testing.T) {
	scenario := newMixedL2L3ControlScenario()
	registry, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	completeTopologyDiagnosticArchiveFixture(&diagnostics)

	var encoded bytes.Buffer
	require.NoError(t, writeTopologyDiagnosticArchiveWithLimits(
		&encoded,
		diagnostics,
		"v-test",
		defaultTopologyDiagnosticArchiveLimits,
	))
	archive, err := readTopologyDiagnosticArchive(bytes.NewReader(encoded.Bytes()))
	require.NoError(t, err)
	require.Equal(t, "v-test", archive.producerVersion)

	beforeDocument, err := newTopologyDiagnosticArchiveDocumentV1(diagnostics, "v-test")
	require.NoError(t, err)
	afterDocument, err := newTopologyDiagnosticArchiveDocumentV1(archive.diagnostics, "v-test")
	require.NoError(t, err)
	require.JSONEq(t, archiveDocumentJSON(t, beforeDocument), archiveDocumentJSON(t, afterDocument))

	require.NotSame(t,
		archive.diagnostics.topology.devices[0].latestAttempt,
		archive.diagnostics.topology.devices[0].acquisition,
	)
	require.Same(t,
		archive.diagnostics.topology.devices[1].latestAttempt,
		archive.diagnostics.topology.devices[1].acquisition,
	)

	live, ok, err := (funcDepsAdapter{registry: registry}).Snapshot(scenario.opts)
	require.NoError(t, err)
	require.True(t, ok)
	replayed, ok, err := replayTopologyDiagnostics(archive.diagnostics, scenario.opts)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, topologyv1test.NormalizeData(t, live), topologyv1test.NormalizeData(t, replayed))

	beforeDevice, err := inspectTopologyDevice(diagnostics, scenario.opts, 1)
	require.NoError(t, err)
	afterDevice, err := inspectTopologyDevice(archive.diagnostics, scenario.opts, 1)
	require.NoError(t, err)
	require.Equal(t, beforeDevice, afterDevice)

	stages := replayTopologyDiagnosticStages(diagnostics, scenario.opts)
	require.NotEmpty(t, stages.data.Links)
	subject, ok := topologyInspectionSubjectFromLink(stages.data, 0)
	require.True(t, ok)
	beforeLink, err := inspectTopologyLink(diagnostics, scenario.opts, subject)
	require.NoError(t, err)
	afterLink, err := inspectTopologyLink(archive.diagnostics, scenario.opts, subject)
	require.NoError(t, err)
	require.Equal(t, beforeLink, afterLink)
}

func TestTopologyDiagnosticArchiveV1GoldenDocument(t *testing.T) {
	raw, err := os.ReadFile("testdata/topology-diagnostic-archive-v1.json")
	require.NoError(t, err)

	archive, err := readTopologyDiagnosticArchive(bytes.NewReader(compressArchiveJSON(t, string(raw))))
	require.NoError(t, err)
	require.Equal(t, "v1.2.3", archive.producerVersion)

	document, err := newTopologyDiagnosticArchiveDocumentV1(archive.diagnostics, archive.producerVersion)
	require.NoError(t, err)
	require.JSONEq(t, string(raw), archiveDocumentJSON(t, document))
}

func TestTopologyDiagnosticArchiveRejectsMalformedAndUnsupportedInput(t *testing.T) {
	_, diagnostics := newTopologyScenarioReplayFixture(t, newLLDPDirectScenario())
	completeTopologyDiagnosticArchiveFixture(&diagnostics)
	document, err := newTopologyDiagnosticArchiveDocumentV1(diagnostics, "v-test")
	require.NoError(t, err)

	tests := map[string]struct {
		mutate func(*topologyDiagnosticArchiveDocumentV1)
		want   string
	}{
		"format": {
			mutate: func(document *topologyDiagnosticArchiveDocumentV1) { document.Format = "other" },
			want:   "unsupported format",
		},
		"version": {
			mutate: func(document *topologyDiagnosticArchiveDocumentV1) { document.Version++ },
			want:   "unsupported version",
		},
		"retained reference": {
			mutate: func(document *topologyDiagnosticArchiveDocumentV1) {
				document.Snapshot.Topology.Devices[0].RetainedSuccess.RegistrationID++
			},
			want: "does not match owner",
		},
		"capture role": {
			mutate: func(document *topologyDiagnosticArchiveDocumentV1) {
				document.Snapshot.Topology.Devices[0].Captures[0].Roles[0] = "other"
			},
			want: "unknown capture role",
		},
		"route reference": {
			mutate: func(document *topologyDiagnosticArchiveDocumentV1) {
				profile := &document.Snapshot.Topology.Devices[0].Captures[0].Evidence.CollectionContexts[0].Profiles[0]
				require.NotEmpty(t, profile.Values.Metrics)
				profile.Values.Metrics[0].RouteOrdinal = ^uint32(0)
			},
			want: "references unknown route ordinal",
		},
		"target address": {
			mutate: func(document *topologyDiagnosticArchiveDocumentV1) {
				document.Snapshot.Topology.Devices[0].Captures[0].Evidence.Target.Addresses = []string{"not-an-address"}
			},
			want: "target address",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			encoded, err := json.Marshal(document)
			require.NoError(t, err)
			var mutated topologyDiagnosticArchiveDocumentV1
			require.NoError(t, json.Unmarshal(encoded, &mutated))
			tc.mutate(&mutated)

			_, err = readTopologyDiagnosticArchive(bytes.NewReader(compressArchiveJSON(t, archiveDocumentJSON(t, mutated))))
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestTopologyDiagnosticArchiveRejectsTrailingAndTruncatedContent(t *testing.T) {
	_, diagnostics := newTopologyScenarioReplayFixture(t, newLLDPDirectScenario())
	document, err := newTopologyDiagnosticArchiveDocumentV1(diagnostics, "v-test")
	require.NoError(t, err)
	validJSON := archiveDocumentJSON(t, document)

	_, err = readTopologyDiagnosticArchive(bytes.NewReader(compressArchiveJSON(t, validJSON+"{}")))
	require.ErrorContains(t, err, "trailing JSON value")

	encoded := compressArchiveJSON(t, validJSON)
	require.Greater(t, len(encoded), 8)
	_, err = readTopologyDiagnosticArchive(bytes.NewReader(encoded[:len(encoded)-4]))
	require.Error(t, err)

	corrupt := append([]byte(nil), encoded...)
	corrupt[len(corrupt)-1] ^= 0xff
	_, err = readTopologyDiagnosticArchive(bytes.NewReader(corrupt))
	require.Error(t, err)
}

func TestTopologyDiagnosticArchiveEnforcesOnlyTheThreeResourceBounds(t *testing.T) {
	_, diagnostics := newTopologyScenarioReplayFixture(t, newLLDPDirectScenario())
	document, err := newTopologyDiagnosticArchiveDocumentV1(diagnostics, "v-test")
	require.NoError(t, err)
	encoded := compressArchiveJSON(t, archiveDocumentJSON(t, document))

	limits := defaultTopologyDiagnosticArchiveLimits
	limits.maxCompressedBytes = int64(len(encoded) - 1)
	_, err = readTopologyDiagnosticArchiveWithLimits(bytes.NewReader(encoded), limits)
	require.ErrorIs(t, err, errTopologyDiagnosticArchiveCompressedLimit)

	limits = defaultTopologyDiagnosticArchiveLimits
	limits.maxDecodedBytes = 64
	_, err = readTopologyDiagnosticArchiveWithLimits(bytes.NewReader(encoded), limits)
	require.ErrorIs(t, err, errTopologyDiagnosticArchiveDecodedLimit)

	windowed := compressArchiveJSONWithWindow(t, strings.Repeat(" ", 8<<10)+archiveDocumentJSON(t, document), 8<<10)
	limits = defaultTopologyDiagnosticArchiveLimits
	limits.maxWindowBytes = 1 << 10
	_, err = readTopologyDiagnosticArchiveWithLimits(bytes.NewReader(windowed), limits)
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "window")
}

func TestTopologyDiagnosticArchiveWriterEnforcesEncodedAndDecodedBounds(t *testing.T) {
	_, diagnostics := newTopologyScenarioReplayFixture(t, newLLDPDirectScenario())

	limits := defaultTopologyDiagnosticArchiveLimits
	limits.maxDecodedBytes = 64
	err := writeTopologyDiagnosticArchiveWithLimits(io.Discard, diagnostics, "v-test", limits)
	require.ErrorIs(t, err, errTopologyDiagnosticArchiveDecodedLimit)

	limits = defaultTopologyDiagnosticArchiveLimits
	limits.maxCompressedBytes = 16
	err = writeTopologyDiagnosticArchiveWithLimits(io.Discard, diagnostics, "v-test", limits)
	require.ErrorIs(t, err, errTopologyDiagnosticArchiveCompressedLimit)
}

func TestTopologyDiagnosticArchiveUsesStandardJSONFieldSemantics(t *testing.T) {
	_, diagnostics := newTopologyScenarioReplayFixture(t, newLLDPDirectScenario())
	document, err := newTopologyDiagnosticArchiveDocumentV1(diagnostics, "v-test")
	require.NoError(t, err)
	raw := archiveDocumentJSON(t, document)
	raw = strings.Replace(raw, `"version":1`, `"ignored":true,"version":1`, 1)
	raw = strings.Replace(raw, `"version":1`, `"version":99,"version":1`, 1)

	_, err = readTopologyDiagnosticArchive(bytes.NewReader(compressArchiveJSON(t, raw)))
	require.NoError(t, err)
}

func TestTopologyDiagnosticArchiveEnumTablesAreCompleteAndRoundTrip(t *testing.T) {
	tables := []struct {
		name  string
		names []string
		last  uint8
	}{
		{"capture state", topologyDiagnosticArchiveCaptureStateNames, uint8(diagnosticCaptureUnavailable)},
		{"capture reason", topologyDiagnosticArchiveCaptureReasonNames, uint8(diagnosticCaptureReasonGlobalByteLimit)},
		{"lifecycle phase", topologyDiagnosticArchiveLifecyclePhaseNames, uint8(ddsnmp.DeviceLifecyclePhaseCollect)},
		{"lifecycle outcome", topologyDiagnosticArchiveLifecycleOutcomeNames, uint8(ddsnmp.DeviceLifecycleOutcomeFailed)},
		{"device outcome", topologyDiagnosticArchiveDeviceOutcomeNames, uint8(deviceRefreshOutcomeFailed)},
		{"abort reason", topologyDiagnosticArchiveAbortReasonNames, uint8(topologyDiagnosticAbortPanic)},
		{"sweep phase", topologyDiagnosticArchiveSweepPhaseNames, uint8(topologyDiagnosticSweepPhaseCommit)},
		{"target outcome", topologyDiagnosticArchiveTargetOutcomeNames, uint8(topologyTargetResolutionFailed)},
		{"phase outcome", topologyDiagnosticArchivePhaseOutcomeNames, uint8(topologyAcquisitionPhaseNotObserved)},
		{"phase failure", topologyDiagnosticArchivePhaseFailureNames, uint8(topologyAcquisitionFailureVLANIdentifier)},
		{"profile outcome", topologyDiagnosticArchiveProfileOutcomeNames, uint8(ddsnmpcollector.AcquisitionProfileOutcomeFailed)},
		{"profile failure", topologyDiagnosticArchiveProfileFailurePhaseNames, uint8(ddsnmpcollector.AcquisitionFailurePhaseTables)},
		{"route kind", topologyDiagnosticArchiveRouteKindNames, uint8(ddsnmpcollector.AcquisitionRouteKindBGPTable)},
		{"route source", topologyDiagnosticArchiveRouteSourceNames, uint8(ddsnmpcollector.AcquisitionRouteSourceCache)},
		{"route outcome", topologyDiagnosticArchiveRouteOutcomeNames, uint8(ddsnmpcollector.AcquisitionRouteOutcomePartial)},
		{"route failure", topologyDiagnosticArchiveRouteFailureClassNames, uint8(ddsnmpcollector.AcquisitionFailureClassDependency)},
	}
	for _, table := range tables {
		t.Run(table.name, func(t *testing.T) {
			require.Len(t, table.names, int(table.last)+1)
			seen := make(map[string]struct{}, len(table.names))
			for index, name := range table.names {
				require.NotEmpty(t, name)
				_, duplicate := seen[name]
				require.False(t, duplicate)
				seen[name] = struct{}{}
				encoded, err := topologyDiagnosticArchiveEnumName(uint8(index), table.names)
				require.NoError(t, err)
				require.Equal(t, name, encoded)
				decoded, err := topologyDiagnosticArchiveParseEnum[uint8](encoded, table.names)
				require.NoError(t, err)
				require.Equal(t, uint8(index), decoded)
			}
		})
	}
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
		limits := topologyDiagnosticArchiveLimits{
			maxCompressedBytes: 1 << 20,
			maxDecodedBytes:    4 << 20,
			maxWindowBytes:     1 << 20,
		}
		_, _ = readTopologyDiagnosticArchiveWithLimits(bytes.NewReader(input), limits)
	})
}

func completeTopologyDiagnosticArchiveFixture(diagnostics *topologyDiagnostics) {
	if diagnostics == nil || diagnostics.topology == nil {
		return
	}
	capturedAt := diagnostics.topology.publishedAt.Add(time.Second)
	diagnostics.lifecycle = topologyJobLifecycleDiagnosticCut{
		state:  diagnosticCaptureAvailable,
		reason: diagnosticCaptureReasonNone,
		cut: ddsnmp.DeviceLifecycleCut{
			Sequence:   7,
			CapturedAt: capturedAt,
			Entries:    make([]ddsnmp.DeviceLifecycleEntry, 0, len(diagnostics.topology.devices)),
		},
	}
	for _, device := range diagnostics.topology.devices {
		diagnostics.lifecycle.cut.Entries = append(diagnostics.lifecycle.cut.Entries, ddsnmp.DeviceLifecycleEntry{
			RegistrationID: device.registrationID,
			Info: ddsnmp.DeviceLifecycleInfo{
				Hostname:    "192.0.2." + device.registrationID.String(),
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
	if len(diagnostics.topology.devices) > 0 {
		device := &diagnostics.topology.devices[0]
		device.latestAttempt = &topologyAcquisitionCapture{
			attemptID: topologyAcquisitionAttemptID{
				registrationID: device.registrationID,
				ordinal:        device.acquisition.attemptID.ordinal + 1,
			},
			state:  diagnosticCaptureUnavailable,
			reason: diagnosticCaptureReasonProjectionError,
		}
	}
	diagnostics.lastAborted = &topologyAbortedSweepDiagnostic{
		sequence:              3,
		startedAt:             capturedAt.Add(time.Minute),
		abortedAt:             capturedAt.Add(time.Minute + time.Second),
		reason:                topologyDiagnosticAbortCanceled,
		phase:                 topologyDiagnosticSweepPhaseDeviceRefresh,
		activeRegistrationID:  diagnostics.topology.devices[0].registrationID,
		hasActiveRegistration: true,
		registrationCount:     len(diagnostics.topology.devices),
		selectedCount:         len(diagnostics.topology.devices),
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
