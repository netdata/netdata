// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/netdata/netdata/go/plugins/internal/snmptopologydiagnostics"
	"github.com/netdata/netdata/go/plugins/pkg/topology/v1"
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
			},
			operation: "inspect-link",
			want:      `"family": "lldp"`,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fake := &fakeDiagnosticArchive{}
			openCalls := 0
			opener := func(_ io.Reader, limits snmptopologydiagnostics.ReadLimits) (diagnosticArchive, error) {
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
			if tc.operation == "inspect-device" || tc.operation == "inspect-link" {
				if !fake.query.CollapseActorsByIP || !fake.query.EliminateNonIPInferred ||
					fake.query.MapType != "managed_fabric" || fake.query.Depth != "all" {
					t.Fatalf("default query options were not forwarded: %+v", fake.query)
				}
			}
			if !strings.Contains(stdout.String(), tc.want) {
				t.Fatalf("stdout missing %q:\n%s", tc.want, stdout.String())
			}
		})
	}
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
	operation string
	limits    snmptopologydiagnostics.ReadLimits
	query     snmptopologydiagnostics.QueryOptions
}

func (a *fakeDiagnosticArchive) Identity() snmptopologydiagnostics.ArchiveIdentity {
	return snmptopologydiagnostics.ArchiveIdentity{}
}

func (a *fakeDiagnosticArchive) Summary() (snmptopologydiagnostics.Summary, error) {
	a.operation = "summary"
	return snmptopologydiagnostics.Summary{}, nil
}

func (a *fakeDiagnosticArchive) Replay(options snmptopologydiagnostics.QueryOptions) (topologyv1.Data, error) {
	a.operation = "replay"
	a.query = options
	return topologyv1.Data{SchemaVersion: "1"}, nil
}

func (a *fakeDiagnosticArchive) InspectDevice(
	options snmptopologydiagnostics.QueryOptions,
	registrationID uint64,
) (snmptopologydiagnostics.DeviceInspection, error) {
	a.operation = "inspect-device"
	a.query = options
	return snmptopologydiagnostics.DeviceInspection{RegistrationID: registrationID}, nil
}

func (a *fakeDiagnosticArchive) InspectLink(
	options snmptopologydiagnostics.QueryOptions,
	subject snmptopologydiagnostics.LinkSubject,
) (snmptopologydiagnostics.LinkInspection, error) {
	a.operation = "inspect-link"
	a.query = options
	return snmptopologydiagnostics.LinkInspection{Subject: subject}, nil
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
