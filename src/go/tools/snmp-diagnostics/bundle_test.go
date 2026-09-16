// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const currentRun = "11111111-1111-4111-8111-111111111111"
const previousRun = "22222222-2222-4222-8222-222222222222"
const testBundleRoot = "netdata-support-bundle-fixture"

type testBundleMember struct {
	name string
	data []byte
	kind byte
}

func bundleFixture(t *testing.T) []testBundleMember {
	t.Helper()
	raw, err := os.ReadFile(replayableDiagnosticArchivePath())
	require.NoError(t, err)
	document, err := snmpdiag.Read(bytes.NewReader(raw), snmpdiag.DefaultReadLimits())
	require.NoError(t, err)
	members := []testBundleMember{
		{name: "MANIFEST.json", data: []byte(`{"schema":"netdata-support-bundle/v2"}`)},
		{name: bundleStatusPath, data: []byte("Result: complete; complete files copied: 6.\n")},
		{name: bundleDiagnosticPath + "/normal/runs.json", data: []byte(fmt.Sprintf(`{"current":%q,"previous":%q}`, currentRun, previousRun))},
	}
	for _, sequence := range []uint64{8, 10} {
		document.Checkpoint = sequence
		members = append(members, testBundleMember{name: fmt.Sprintf("%s/topology/checkpoint-%020d.zst", bundleDiagnosticPath, sequence), data: encodeDocument(t, document)})
	}
	document.Kind, document.Checkpoint = snmpdiag.KindLifecycle, 0
	document.Snapshot = snmpdiag.Snapshot{Lifecycle: document.Snapshot.Lifecycle}
	members = append(members, testBundleMember{name: bundleDiagnosticPath + "/lifecycle.zst", data: encodeDocument(t, document)})
	for _, run := range []string{currentRun, previousRun, "33333333-3333-4333-8333-333333333333"} {
		now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
		document := snmpdiag.Document{
			Format: snmpdiag.Format, Version: snmpdiag.Version, Kind: snmpdiag.KindNormal,
			Producer: snmpdiag.Producer{RunID: run},
			Normal: &snmpdiag.NormalDevice{RegistrationID: 7, RuntimeID: 1, Hostname: run + ".example", CapturedAt: now,
				Latest: &snmpdiag.NormalAttempt{ID: 2, Phase: "collect", StartedAt: now, CompletedAt: now, Samples: map[string]int64{"sample": 42}},
			},
		}
		members = append(members, testBundleMember{name: bundleDiagnosticPath + "/normal/" + run + "/device-00000000000000000007.zst", data: encodeDocument(t, document)})
	}
	return members
}

func encodeDocument(t *testing.T, document snmpdiag.Document) []byte {
	t.Helper()
	var out bytes.Buffer
	require.NoError(t, snmpdiag.Write(&out, document))
	return out.Bytes()
}

// Use real container writers and deliberately omit directory records. Ordering
// differs from producers so selection cannot rely on tar order or index position.
func writeBundle(t *testing.T, format, prefix string, members []testBundleMember) string {
	t.Helper()
	root := t.TempDir()
	if format == "directory" {
		for _, member := range members {
			filename := filepath.Join(root, member.name)
			require.NoError(t, os.MkdirAll(filepath.Dir(filename), 0700))
			require.NoError(t, os.WriteFile(filename, member.data, 0600))
		}
		return root
	}
	filename := filepath.Join(root, "support"+format)
	file, err := os.Create(filename)
	require.NoError(t, err)
	if format == ".zip" {
		writer := zip.NewWriter(file)
		for _, member := range members {
			header := &zip.FileHeader{Name: prefix + member.name, Method: zip.Deflate}
			if member.kind == tar.TypeSymlink {
				header.SetMode(os.ModeSymlink | 0600)
			}
			entry, err := writer.CreateHeader(header)
			require.NoError(t, err)
			_, err = entry.Write(member.data)
			require.NoError(t, err)
		}
		require.NoError(t, writer.Close())
	} else {
		var compressed io.WriteCloser
		if format == ".tar.gz" {
			compressed = gzip.NewWriter(file)
		} else {
			compressed, err = zstd.NewWriter(file, zstd.WithEncoderConcurrency(1))
			require.NoError(t, err)
		}
		writer := tar.NewWriter(compressed)
		for _, member := range members {
			kind := member.kind
			if kind == 0 {
				kind = tar.TypeReg
			}
			header := &tar.Header{Name: prefix + member.name, Mode: 0600, Size: int64(len(member.data)), Typeflag: kind}
			if kind == tar.TypeSymlink || kind == tar.TypeLink {
				header.Linkname = "outside"
				header.Size = 0
			}
			require.NoError(t, writer.WriteHeader(header))
			if header.Size > 0 {
				_, err := writer.Write(member.data)
				require.NoError(t, err)
			}
		}
		require.NoError(t, writer.Close())
		require.NoError(t, compressed.Close())
	}
	require.NoError(t, file.Close())
	return filename
}

func runBundleCommand(t *testing.T, filename string, args ...string) (int, string, string) {
	t.Helper()
	arguments := append([]string{args[0], "--input", filename}, args[1:]...)
	var out, err bytes.Buffer
	code := run(arguments, &out, &err)
	return code, out.String(), err.String()
}

func TestSupportBundleCommands(t *testing.T) {
	for name, format := range map[string]struct{ suffix string }{
		"zstd tar": {".tar.zst"}, "gzip tar": {".tar.gz"}, "windows zip": {".zip"}, "extracted root": {"directory"},
	} {
		t.Run(name, func(t *testing.T) {
			members := bundleFixture(t)
			sort.Slice(members, func(i, j int) bool { return members[i].name > members[j].name })
			filename := writeBundle(t, format.suffix, testBundleRoot+"/", members)
			for name, tc := range map[string]struct {
				args []string
				want string
				code int
			}{
				"inventory":                        {[]string{"list"}, `"sequence": 10`, 0},
				"latest":                           {[]string{"validate"}, `"checkpoint": 10`, 0},
				"older":                            {[]string{"validate", "--checkpoint", "8"}, `"checkpoint": 8`, 0},
				"lifecycle":                        {[]string{"summary", "--lifecycle"}, `"kind": "lifecycle"`, 0},
				"replay":                           {[]string{"replay"}, `"schema_version": "netdata.topology.v1"`, 0},
				"topology device":                  {[]string{"inspect-device", "--registration-id", "7"}, `"registration_id": 7`, 0},
				"current device":                   {[]string{"summary", "--normal", "--registration-id", "7"}, currentRun + ".example", 0},
				"previous device":                  {[]string{"summary", "--normal", "--previous-run", "--registration-id", "7"}, previousRun + ".example", 0},
				"normal inspect":                   {[]string{"inspect-device", "--normal", "--registration-id", "7"}, `"sample": 42`, 0},
				"unretained":                       {[]string{"summary", "--normal", "--registration-id", "8"}, "not retained", 1},
				"normal needs id":                  {[]string{"summary", "--normal"}, "requires --registration-id", 1},
				"missing checkpoint":               {[]string{"summary", "--checkpoint", "9"}, "not retained", 1},
				"conflicting lifecycle normal":     {[]string{"summary", "--lifecycle", "--normal", "--registration-id", "7"}, "cannot be combined", 1},
				"conflicting lifecycle checkpoint": {[]string{"summary", "--lifecycle", "--checkpoint", "8"}, "cannot be combined", 1},
				"previous without normal":          {[]string{"summary", "--lifecycle", "--previous-run"}, "requires --normal", 1},
				"list lifecycle rejected":          {[]string{"list", "--lifecycle"}, "does not support", 1},
				"compressed limit":                 {[]string{"validate", "--max-compressed-size", "1B"}, "compressed", 1},
				"decoded limit":                    {[]string{"validate", "--max-decoded-size", "1B"}, "decoded", 1},
			} {
				t.Run(name, func(t *testing.T) {
					code, out, err := runBundleCommand(t, filename, tc.args...)
					require.Equal(t, tc.code, code, err)
					if code == 0 {
						assert.Contains(t, out, tc.want)
						assert.Empty(t, err)
					} else {
						assert.Contains(t, err, tc.want)
						assert.Empty(t, out)
					}
				})
			}
			code, out, err := runBundleCommand(t, filename, "list")
			require.Zero(t, code, err)
			var listing diagnosticListing
			require.NoError(t, json.Unmarshal([]byte(out), &listing))
			require.NotNil(t, listing.Bundle)
			require.NotNil(t, listing.Bundle.CollectionStatus)
			assert.Contains(t, *listing.Bundle.CollectionStatus, "Result: complete")
			assert.True(t, listing.Lifecycle)
			assert.Len(t, listing.Normal, 2)
			assert.Equal(t, currentRun, listing.Normal[0].RunID)
			assert.Equal(t, previousRun, listing.Normal[1].RunID)
			assert.Empty(t, listing.Errors)
		})
	}
}

func TestSupportBundlePartialEvidence(t *testing.T) {
	for formatName, format := range map[string]struct{ suffix string }{
		"zstd": {".tar.zst"}, "gzip": {".tar.gz"}, "zip": {".zip"}, "directory": {"directory"},
	} {
		t.Run(formatName, func(t *testing.T) {
			for name, tc := range map[string]struct {
				index         string
				missingIndex  bool
				status        string
				missingStatus bool
				normalError   bool
			}{
				"partial":        {index: fmt.Sprintf(`{"current":%q}`, currentRun), status: "Result: partial; files withheld.\n"},
				"invalid index":  {index: `{"current":"../outside"}`, status: "Result: partial\n", normalError: true},
				"trailing index": {index: fmt.Sprintf(`{"current":%q} {}`, currentRun), status: "Result: partial\n", normalError: true},
				"missing index":  {missingIndex: true, status: "Result: partial\n"},
				"missing status": {index: fmt.Sprintf(`{"current":%q}`, currentRun), missingStatus: true},
			} {
				t.Run(name, func(t *testing.T) {
					var members []testBundleMember
					for _, member := range bundleFixture(t) {
						if strings.HasSuffix(member.name, "runs.json") {
							if tc.missingIndex {
								continue
							}
							member.data = []byte(tc.index)
						}
						if member.name == bundleStatusPath {
							if tc.missingStatus {
								continue
							}
							member.data = []byte(tc.status)
						}
						members = append(members, member)
					}
					filename := writeBundle(t, format.suffix, testBundleRoot+"/", members)
					code, out, err := runBundleCommand(t, filename, "list")
					require.Zero(t, code, err)
					var listing diagnosticListing
					require.NoError(t, json.Unmarshal([]byte(out), &listing))
					assert.Len(t, listing.Topology, 2)
					assert.True(t, listing.Lifecycle)
					assert.Equal(t, tc.normalError, listing.Errors["normal"] != "")
					assert.Equal(t, tc.missingStatus, listing.Errors["collection_status"] != "")
					if tc.missingIndex || tc.normalError {
						assert.Empty(t, listing.Normal)
					}
					for _, args := range [][]string{{"validate"}, {"validate", "--lifecycle"}} {
						code, _, err := runBundleCommand(t, filename, args...)
						require.Zero(t, code, err)
					}
					if tc.normalError {
						code, _, err := runBundleCommand(t, filename, "summary", "--normal", "--registration-id", "7")
						require.Equal(t, 1, code)
						assert.Contains(t, err, "normal evidence index")
					}
				})
			}
		})
	}
}

func TestSupportBundleMemberValidation(t *testing.T) {
	for formatName, format := range map[string]struct{ suffix string }{"zstd": {".tar.zst"}, "gzip": {".tar.gz"}, "zip": {".zip"}} {
		t.Run(formatName, func(t *testing.T) {
			for name, tc := range map[string]struct {
				members []testBundleMember
				want    string
				success bool
			}{
				"duplicate":               {members: []testBundleMember{{name: bundleStatusPath}, {name: bundleStatusPath}}, want: "duplicate"},
				"dot alias":               {members: []testBundleMember{{name: bundleStatusPath}, {name: "./" + bundleStatusPath}}, want: "duplicate"},
				"traversal":               {members: []testBundleMember{{name: "../" + bundleStatusPath}}, want: "unsafe"},
				"absolute":                {members: []testBundleMember{{name: "/" + bundleStatusPath}}, want: "unsafe"},
				"backslash":               {members: []testBundleMember{{name: `06-state\snmp-diagnostics-status.txt`}}, want: "unsafe"},
				"two roots":               {members: []testBundleMember{{name: "one/" + bundleStatusPath}, {name: "two/" + bundleStatusPath}}, want: "multiple"},
				"file directory conflict": {members: []testBundleMember{{name: bundleDiagnosticPath}, {name: bundleDiagnosticPath + "/lifecycle.zst"}}, want: "also a directory"},
				"symlink":                 {members: []testBundleMember{{name: bundleDiagnosticPath + "/lifecycle.zst", kind: tar.TypeSymlink}}, want: "not a regular file"},
				"no bundle":               {members: []testBundleMember{{name: "notes.txt"}}, want: "no SNMP support-bundle root"},
				"nested bundle":           {members: []testBundleMember{{name: "one/two/" + bundleStatusPath}}, want: "no SNMP support-bundle root"},
				"not requested":           {members: []testBundleMember{{name: bundleStatusPath, data: []byte("Not requested. Use --include-snmp-diagnostics.")}}, success: true, want: "Not requested"},
				"unavailable":             {members: []testBundleMember{{name: bundleStatusPath, data: []byte("Result: unavailable; complete files copied: 0.")}}, success: true, want: "unavailable"},
			} {
				t.Run(name, func(t *testing.T) {
					filename := writeBundle(t, format.suffix, "", tc.members)
					code, out, err := runBundleCommand(t, filename, "list")
					if tc.success {
						require.Zero(t, code, err)
						assert.Contains(t, out, tc.want)
					} else {
						require.Equal(t, 1, code)
						assert.Contains(t, err, tc.want)
					}
				})
			}
		})
	}
}

func TestSupportBundleUnselectedPayloads(t *testing.T) {
	for name, tc := range map[string]struct{ format string }{"tar": {".tar.zst"}, "gzip": {".tar.gz"}, "zip": {".zip"}} {
		t.Run(name, func(t *testing.T) {
			members := bundleFixture(t)
			// These are intentionally not diagnostic documents. Listing and selecting
			// another member must neither decode nor retain their payloads.
			for _, name := range []string{"logs/large.log", bundleDiagnosticPath + "/topology/checkpoint-00000000000000000009.zst"} {
				members = append(members, testBundleMember{name: name, data: bytes.Repeat([]byte("unselected"), 1<<20)})
			}
			filename := writeBundle(t, tc.format, "./"+testBundleRoot+"/", members)
			input, err := openInput(filename)
			require.NoError(t, err)
			defer input.Close()
			for name, entry := range input.bundle.entries {
				if strings.HasSuffix(name, ".zst") {
					assert.Empty(t, entry.metadata, name)
				}
			}
			code, _, message := runBundleCommand(t, filename, "list")
			require.Zero(t, code, message)
			code, _, message = runBundleCommand(t, filename, "validate")
			require.Zero(t, code, message)
			code, _, message = runBundleCommand(t, filename, "validate", "--checkpoint", "9")
			require.Equal(t, 1, code)
			assert.Contains(t, message, "read archive")
		})
	}
}

func TestSupportBundleCorruption(t *testing.T) {
	for name, tc := range map[string]struct {
		format  string
		corrupt func([]byte) []byte
	}{
		"truncated zstd": {".tar.zst", func(b []byte) []byte { return b[:len(b)/2] }},
		"truncated gzip": {".tar.gz", func(b []byte) []byte { return b[:len(b)/2] }},
		"truncated zip":  {".zip", func(b []byte) []byte { return b[:len(b)/2] }},
		"zstd trailer":   {".tar.zst", func(b []byte) []byte { b[len(b)-1] ^= 0xff; return b }},
		"gzip trailer":   {".tar.gz", func(b []byte) []byte { b[len(b)-1] ^= 0xff; return b }},
	} {
		t.Run(name, func(t *testing.T) {
			filename := writeBundle(t, tc.format, testBundleRoot+"/", bundleFixture(t))
			data, err := os.ReadFile(filename)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filename, tc.corrupt(data), 0600))
			code, out, message := runBundleCommand(t, filename, "validate")
			require.Equal(t, 1, code)
			assert.Empty(t, out)
			assert.Contains(t, message, "open input")
		})
	}
}

func TestTarBundleHardLinks(t *testing.T) {
	for name, tc := range map[string]struct{ format string }{"zstd": {".tar.zst"}, "gzip": {".tar.gz"}} {
		t.Run(name, func(t *testing.T) {
			filename := writeBundle(t, tc.format, "", []testBundleMember{{name: bundleDiagnosticPath + "/lifecycle.zst", kind: tar.TypeLink}})
			code, _, message := runBundleCommand(t, filename, "list")
			require.Equal(t, 1, code)
			assert.Contains(t, message, "not a regular file")
		})
	}
}

func TestZIPMetadataReadFailureIsolation(t *testing.T) {
	for name, tc := range map[string]struct {
		member    string
		component string
	}{
		"status checksum":    {bundleStatusPath, "collection_status"},
		"run index checksum": {bundleDiagnosticPath + "/normal/runs.json", "normal"},
	} {
		t.Run(name, func(t *testing.T) {
			filename := writeBundle(t, ".zip", testBundleRoot+"/", bundleFixture(t))
			data, err := os.ReadFile(filename)
			require.NoError(t, err)
			original, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
			require.NoError(t, err)
			var corrupt bytes.Buffer
			writer := zip.NewWriter(&corrupt)
			for _, member := range original.File {
				header := member.FileHeader
				if header.Name == testBundleRoot+"/"+tc.member {
					header.CRC32 ^= 1
				}
				destination, err := writer.CreateRaw(&header)
				require.NoError(t, err)
				source, err := member.OpenRaw()
				require.NoError(t, err)
				_, err = io.Copy(destination, source)
				require.NoError(t, err)
			}
			require.NoError(t, writer.Close())
			require.NoError(t, os.WriteFile(filename, corrupt.Bytes(), 0600))
			code, out, message := runBundleCommand(t, filename, "list")
			require.Zero(t, code, message)
			var listing diagnosticListing
			require.NoError(t, json.Unmarshal([]byte(out), &listing))
			assert.NotEmpty(t, listing.Errors[tc.component])
			assert.True(t, listing.Lifecycle)
			assert.Len(t, listing.Topology, 2)
			for _, args := range [][]string{{"validate"}, {"validate", "--lifecycle"}} {
				code, _, message := runBundleCommand(t, filename, args...)
				require.Zero(t, code, message)
			}
			if tc.component == "normal" {
				code, _, message := runBundleCommand(t, filename, "summary", "--normal", "--registration-id", "7")
				require.Equal(t, 1, code)
				assert.Contains(t, message, "normal evidence index")
			}
		})
	}
}

func TestInvalidSelectionDoesNotOpenInput(t *testing.T) {
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"list selector":           {[]string{"list", "--normal"}, "list does not support"},
		"lifecycle normal":        {[]string{"summary", "--lifecycle", "--normal"}, "--lifecycle cannot"},
		"normal missing id":       {[]string{"summary", "--normal"}, "--normal requires"},
		"normal checkpoint":       {[]string{"summary", "--normal", "--registration-id", "7", "--checkpoint", "8"}, "cannot select a topology"},
		"previous without normal": {[]string{"summary", "--previous-run"}, "--previous-run requires"},
	} {
		t.Run(name, func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), "missing.tar.zst")
			code, _, message := runBundleCommand(t, filename, tc.args...)
			require.Equal(t, 1, code)
			assert.Contains(t, message, tc.want)
			assert.NotContains(t, message, "open input")
		})
	}
}
