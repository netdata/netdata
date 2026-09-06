// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	jsonv1 "encoding/json"
	jsonv2 "encoding/json/v2"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
	snmptopology "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology"
)

func TestRunValidatesAndSummarizesArchive(t *testing.T) {
	archivePath := writeGoldenDiagnosticArchive(t)

	tests := map[string]struct {
		args []string
		want []string
	}{
		"validate": {
			args: []string{"validate", "--archive", archivePath},
			want: []string{`"valid": true`, `"producer_agent_version": "v1.2.3"`},
		},
		"summary": {
			args: []string{"summary", "--archive", archivePath},
			want: []string{`"producer_agent_version": "v1.2.3"`, `"registration_id": 7`},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if exitCode := run(tc.args, &stdout, &stderr); exitCode != 0 {
				t.Fatalf("exit code=%d stderr=%s stdout=%s", exitCode, stderr.String(), stdout.String())
			}
			for _, want := range tc.want {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("stdout missing %q:\n%s", want, stdout.String())
				}
			}
			if stderr.Len() != 0 {
				t.Fatalf("unexpected stderr: %s", stderr.String())
			}
		})
	}
}

func TestRunDispatchesReplayAndInspectionOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archive.zst")
	if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := map[string]struct {
		args      []string
		operation string
		want      string
	}{
		"validate": {
			args:      []string{"validate", "--archive", path},
			operation: "validate",
			want:      `"valid": true`,
		},
		"summary": {
			args:      []string{"summary", "--archive", path},
			operation: "summary",
			want:      `"producer_agent_version": ""`,
		},
		"replay": {
			args: []string{
				"replay", "--archive", path, "--map-type", "lldp_cdp_managed", "--depth", "2",
				"--collapse-actors-by-ip=false", "--eliminate-non-ip-inferred=false",
				"--max-compressed-size", "1MiB", "--max-decoded-size", "2MiB",
			},
			operation: "replay",
			want:      `"schema_version": "1"`,
		},
		"inspect device": {
			args:      []string{"inspect-device", "--archive", path, "--registration-id", "7"},
			operation: "inspect-device",
			want:      `"registration_id": 7`,
		},
		"inspect link": {
			args: []string{
				"inspect-link", "--archive", path,
				"--source-identity", "actor:a", "--destination-identity", "actor:b", "--family", "lldp",
				"--direction", "bidirectional",
			},
			operation: "inspect-link",
			want:      `"family": "lldp"`,
		},
		"inspect link by index": {
			args:      []string{"inspect-link", "--archive", path, "--link-index", "3"},
			operation: "inspect-link-at",
			want:      `"selected_index": 0`,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fake := &fakeDiagnosticArchive{}
			openCalls := 0
			opener := func(_ io.Reader, limits snmpdiag.ReadLimits) (diagnosticArchive, error) {
				openCalls++
				fake.limits = limits
				return fake, nil
			}
			var stdout, stderr bytes.Buffer
			if exitCode := runWithOpener(tc.args, &stdout, &stderr, opener); exitCode != 0 {
				t.Fatalf("exit code=%d stderr=%s stdout=%s", exitCode, stderr.String(), stdout.String())
			}
			if openCalls != 1 {
				t.Fatalf("archive decode calls=%d, want 1", openCalls)
			}
			if fake.operationCalls != 1 {
				t.Fatalf("archive operation calls=%d, want 1", fake.operationCalls)
			}
			if fake.operation != tc.operation {
				t.Fatalf("operation=%q, want %q", fake.operation, tc.operation)
			}
			if tc.operation == "replay" {
				if fake.limits.MaxCompressedBytes != 1<<20 || fake.limits.MaxDecodedBytes != 2<<20 {
					t.Fatalf("human-readable limits were not forwarded: %+v", fake.limits)
				}
				if fake.query.MapType != "lldp_cdp_managed" || fake.query.Depth != "2" {
					t.Fatalf("explicit query options were not forwarded: %+v", fake.query)
				}
				if fake.query.CollapseActorsByIP || fake.query.EliminateNonIPInferred {
					t.Fatalf("explicit boolean query options were not forwarded: %+v", fake.query)
				}
			}
			if tc.operation == "inspect-device" || strings.HasPrefix(tc.operation, "inspect-link") {
				if !fake.query.CollapseActorsByIP || !fake.query.EliminateNonIPInferred ||
					fake.query.MapType != "managed_fabric" || fake.query.Depth != "all" {
					t.Fatalf("default query options were not forwarded: %+v", fake.query)
				}
			}
			if tc.operation == "inspect-link-at" && fake.linkIndex != 3 {
				t.Fatalf("link index=%d, want 3", fake.linkIndex)
			}
			if !strings.Contains(stdout.String(), tc.want) {
				t.Fatalf("stdout missing %q:\n%s", tc.want, stdout.String())
			}
		})
	}
}

func TestRunRealArchiveMatchesDirectOperations(t *testing.T) {
	archivePath := replayableDiagnosticArchivePath()
	query := snmptopology.DefaultDiagnosticQueryOptions()
	link := snmptopology.DiagnosticLinkSubject{
		SourceIdentity:      "ip:192.0.2.71",
		DestinationIdentity: "ip:192.0.2.72",
		Family:              "lldp",
		Direction:           "bidirectional",
	}
	tests := map[string]struct {
		args   []string
		direct func(*snmptopology.DiagnosticArchive) (any, error)
	}{
		"validate": {
			args: []string{"validate", "--archive", archivePath},
			direct: func(archive *snmptopology.DiagnosticArchive) (any, error) {
				return snmptopology.DiagnosticValidation{Valid: true, Archive: archive.Identity()}, nil
			},
		},
		"summary": {
			args:   []string{"summary", "--archive", archivePath},
			direct: func(archive *snmptopology.DiagnosticArchive) (any, error) { return archive.Summary() },
		},
		"replay": {
			args:   []string{"replay", "--archive", archivePath},
			direct: func(archive *snmptopology.DiagnosticArchive) (any, error) { return archive.Replay(query) },
		},
		"inspect device": {
			args: []string{"inspect-device", "--archive", archivePath, "--registration-id", "1"},
			direct: func(archive *snmptopology.DiagnosticArchive) (any, error) {
				return archive.InspectDevice(query, 1)
			},
		},
		"inspect link": {
			args: []string{
				"inspect-link", "--archive", archivePath,
				"--source-identity", link.SourceIdentity,
				"--destination-identity", link.DestinationIdentity,
				"--family", link.Family,
				"--direction", link.Direction,
			},
			direct: func(archive *snmptopology.DiagnosticArchive) (any, error) {
				return archive.InspectLink(query, link)
			},
		},
		"inspect link by index": {
			args: []string{"inspect-link", "--archive", archivePath, "--link-index", "0"},
			direct: func(archive *snmptopology.DiagnosticArchive) (any, error) {
				return archive.InspectLinkAt(query, 0)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			archive := readReplayableDiagnosticArchive(t)
			want, err := tc.direct(archive)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := jsonv2.Marshal(want, outputJSONOptions)
			if err != nil {
				t.Fatal(err)
			}
			encoded = append(encoded, '\n')

			var stdout, stderr bytes.Buffer
			if exitCode := run(tc.args, &stdout, &stderr); exitCode != 0 {
				t.Fatalf("exit code=%d stderr=%s stdout=%s", exitCode, stderr.String(), stdout.String())
			}
			if !jsonv1.Valid(stdout.Bytes()) {
				t.Fatalf("stdout is not valid JSON: %s", stdout.String())
			}
			if !bytes.Equal(encoded, stdout.Bytes()) {
				t.Fatalf(
					"command output differs from direct operation: %s",
					firstByteDifference(encoded, stdout.Bytes()),
				)
			}
			if stderr.Len() != 0 {
				t.Fatalf("unexpected stderr: %s", stderr.String())
			}
		})
	}
}

func firstByteDifference(want, got []byte) string {
	limit := min(len(want), len(got))
	index := 0
	for index < limit && want[index] == got[index] {
		index++
	}
	start := max(0, index-40)
	wantEnd := min(len(want), index+80)
	gotEnd := min(len(got), index+80)
	return fmt.Sprintf(
		"offset=%d want_len=%d got_len=%d want=%q got=%q",
		index,
		len(want),
		len(got),
		want[start:wantEnd],
		got[start:gotEnd],
	)
}

func BenchmarkRunSummary(b *testing.B) {
	archivePath := writeGoldenDiagnosticArchive(b)
	arguments := []string{"summary", "--archive", archivePath}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if exitCode := run(arguments, io.Discard, io.Discard); exitCode != 0 {
			b.Fatalf("exit code=%d", exitCode)
		}
	}
}

func TestRunRejectsUsageArchiveAndSelectors(t *testing.T) {
	validArchive := writeGoldenDiagnosticArchive(t)
	invalidArchive := filepath.Join(t.TempDir(), "invalid.zst")
	if err := os.WriteFile(invalidArchive, []byte("not zstd"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := map[string]struct {
		args []string
		code int
		want string
	}{
		"missing operation": {code: 2, want: "usage:"},
		"unknown operation": {args: []string{"other"}, code: 2, want: "unknown operation"},
		"missing archive":   {args: []string{"validate"}, code: 2, want: "--archive is required"},
		"invalid archive": {
			args: []string{"validate", "--archive", invalidArchive},
			code: 1,
			want: "read archive",
		},
		"invalid size": {
			args: []string{"validate", "--archive", validArchive, "--max-compressed-size", "many"},
			code: 2,
			want: "compressed size",
		},
		"query flag on validate": {
			args: []string{"validate", "--archive", validArchive, "--map-type", "managed_fabric"},
			code: 2,
			want: "flag provided but not defined",
		},
		"compressed limit": {
			args: []string{"validate", "--archive", validArchive, "--max-compressed-size", "1B"},
			code: 1,
			want: "compressed-byte limit",
		},
		"invalid selector": {
			args: []string{"replay", "--archive", validArchive, "--map-type", "other"},
			code: 1,
			want: "map type",
		},
		"missing link direction": {
			args: []string{
				"inspect-link", "--archive", validArchive,
				"--source-identity", "actor:a", "--destination-identity", "actor:b", "--family", "lldp",
			},
			code: 2,
			want: "--direction",
		},
		"invalid link direction": {
			args: []string{
				"inspect-link", "--archive", validArchive,
				"--source-identity", "actor:a", "--destination-identity", "actor:b", "--family", "lldp",
				"--direction", "sideways",
			},
			code: 1,
			want: "link direction",
		},
		"missing link selector": {
			args: []string{"inspect-link", "--archive", validArchive},
			code: 2,
			want: "link selector",
		},
		"negative link index": {
			args: []string{"inspect-link", "--archive", validArchive, "--link-index", "-1"},
			code: 2,
			want: "--link-index must be zero or greater",
		},
		"mixed link selectors": {
			args: []string{
				"inspect-link", "--archive", validArchive, "--link-index", "0",
				"--source-identity", "actor:a", "--destination-identity", "actor:b",
				"--family", "lldp", "--direction", "bidirectional",
			},
			code: 2,
			want: "mutually exclusive",
		},
		"link index out of range": {
			args: []string{"inspect-link", "--archive", validArchive, "--link-index", "1000000"},
			code: 1,
			want: "link index",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if exitCode := run(tc.args, &stdout, &stderr); exitCode != tc.code {
				t.Fatalf("exit code=%d, want %d; stderr=%s stdout=%s", exitCode, tc.code, stderr.String(), stdout.String())
			}
			if !strings.Contains(strings.ToLower(stderr.String()), strings.ToLower(tc.want)) {
				t.Fatalf("stderr missing %q: %s", tc.want, stderr.String())
			}
		})
	}
}

type fakeDiagnosticArchive struct {
	operation      string
	operationCalls int
	limits         snmpdiag.ReadLimits
	query          snmptopology.DiagnosticQueryOptions
	linkIndex      int
}

func (a *fakeDiagnosticArchive) Identity() snmptopology.DiagnosticArchiveIdentity {
	a.operation = "validate"
	a.operationCalls++
	return snmptopology.DiagnosticArchiveIdentity{}
}

func (a *fakeDiagnosticArchive) Summary() (snmptopology.DiagnosticSummary, error) {
	a.operation = "summary"
	a.operationCalls++
	return snmptopology.DiagnosticSummary{}, nil
}

func (a *fakeDiagnosticArchive) Replay(options snmptopology.DiagnosticQueryOptions) (topologyv1.Data, error) {
	a.operation = "replay"
	a.operationCalls++
	a.query = options
	return topologyv1.Data{SchemaVersion: "1"}, nil
}

func (a *fakeDiagnosticArchive) InspectDevice(
	options snmptopology.DiagnosticQueryOptions,
	registrationID uint64,
) (snmptopology.DiagnosticDeviceInspection, error) {
	a.operation = "inspect-device"
	a.operationCalls++
	a.query = options
	return snmptopology.DiagnosticDeviceInspection{RegistrationID: registrationID}, nil
}

func (a *fakeDiagnosticArchive) InspectLink(
	options snmptopology.DiagnosticQueryOptions,
	subject snmptopology.DiagnosticLinkSubject,
) (snmptopology.DiagnosticLinkInspection, error) {
	a.operation = "inspect-link"
	a.operationCalls++
	a.query = options
	return snmptopology.DiagnosticLinkInspection{Subject: subject}, nil
}

func (a *fakeDiagnosticArchive) InspectLinkAt(
	options snmptopology.DiagnosticQueryOptions,
	index int,
) (snmptopology.DiagnosticLinkInspection, error) {
	a.operation = "inspect-link-at"
	a.operationCalls++
	a.query = options
	a.linkIndex = index
	return snmptopology.DiagnosticLinkInspection{}, nil
}

func writeGoldenDiagnosticArchive(t testing.TB) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(
		"..", "..", "plugin", "go.d", "collector", "snmp_topology", "testdata", "topology-diagnostic-archive-v1.json",
	))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "diagnostics.zst")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	encoder, err := zstd.NewWriter(file, zstd.WithEncoderConcurrency(1))
	if err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if _, err := encoder.Write(raw); err != nil {
		encoder.Close()
		_ = file.Close()
		t.Fatal(err)
	}
	if err := encoder.Close(); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func replayableDiagnosticArchivePath() string {
	return filepath.Join(
		"..", "..", "plugin", "go.d", "collector", "snmp_topology", "testdata",
		"topology-diagnostic-archive-replay-v1.zst",
	)
}

func readReplayableDiagnosticArchive(t testing.TB) *snmptopology.DiagnosticArchive {
	t.Helper()
	fixture, err := os.ReadFile(replayableDiagnosticArchivePath())
	if err != nil {
		t.Fatal(err)
	}
	archive, err := openDiagnosticArchive(
		bytes.NewReader(fixture),
		snmpdiag.DefaultReadLimits(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return archive
}
