package wireeval

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/oracle"
)

func TestEvidenceRecordRoundTripsRawObservation(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "bundle")
	bundle, err := NewEvidenceBundle(directory)
	if err != nil {
		t.Fatal(err)
	}
	observation := oracle.RunObservation{
		WorkloadID: "B-WL-001-balanced",
		Population: 1,
		PairIndex:  0,
		Side:       oracle.SideBaseline,
	}
	if err := bundle.Record(observation); err != nil {
		t.Fatal(err)
	}
	if len(bundle.runs) != 1 {
		t.Fatalf("recorded runs=%d, want 1", len(bundle.runs))
	}
	path := filepath.Join(directory, filepath.FromSlash(bundle.runs[0].File))
	replayed, err := readRunEvidence(path)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Version != evidenceVersion ||
		replayed.Observation.WorkloadID != observation.WorkloadID ||
		replayed.Observation.Side != observation.Side {
		t.Fatalf("replayed evidence differs: %#v", replayed)
	}
	if err := verifyFileSHA256(path, bundle.runs[0].SHA256); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("mutation")); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := verifyFileSHA256(path, bundle.runs[0].SHA256); err == nil {
		t.Fatal("mutated run evidence retained checksum validity")
	}
}

func TestEvidenceBundleRefusesOverwriteAndOverflow(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "bundle")
	if _, err := NewEvidenceBundle(directory); err != nil {
		t.Fatal(err)
	}
	if _, err := NewEvidenceBundle(directory); err == nil {
		t.Fatal("existing evidence directory was overwritten")
	}
	var output bytes.Buffer
	writer := boundedWriter{target: &output, maximum: 4}
	if _, err := writer.Write([]byte("1234")); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("5")); err == nil {
		t.Fatal("evidence bound overflow was accepted")
	}
}

func TestEvidencePathsAreContainedRegularAndBounded(t *testing.T) {
	directory := t.TempDir()
	regular := filepath.Join(directory, "regular.json")
	if err := os.WriteFile(regular, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(directory, "symlink.json")
	if err := os.Symlink(outside, symlink); err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		name    string
		maximum int64
		wantErr bool
	}{
		"regular": {
			name: "regular.json", maximum: 2,
		},
		"absolute": {
			name: regular, maximum: 2, wantErr: true,
		},
		"parent traversal": {
			name: "../outside.json", maximum: 2, wantErr: true,
		},
		"non canonical": {
			name: "sub/../regular.json", maximum: 2, wantErr: true,
		},
		"backslash": {
			name: `runs\\regular.json`, maximum: 2, wantErr: true,
		},
		"symlink": {
			name: "symlink.json", maximum: 2, wantErr: true,
		},
		"oversized": {
			name: "regular.json", maximum: 1, wantErr: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := evidenceRegularFile(
				directory,
				test.name,
				test.maximum,
			)
			if (err != nil) != test.wantErr {
				t.Fatalf("error=%v, wantErr=%v", err, test.wantErr)
			}
		})
	}
}

func TestEvidenceReadersRejectTrailingAndOversizedContent(t *testing.T) {
	tests := map[string]struct {
		content  string
		maximum  int64
		wantPart string
	}{
		"exact JSON": {
			content: "{}", maximum: 2,
		},
		"trailing whitespace": {
			content: "{}\n\t", maximum: 4,
		},
		"trailing value": {
			content: "{}{}", maximum: 4, wantPart: "trailing value",
		},
		"trailing garbage": {
			content: "{}x", maximum: 3, wantPart: "invalid character",
		},
		"oversized": {
			content: "{}", maximum: 1, wantPart: "exceeds bound",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "value.json")
			if err := os.WriteFile(
				path,
				[]byte(test.content),
				0o600,
			); err != nil {
				t.Fatal(err)
			}
			var value map[string]any
			err := readJSONFileBounded(path, &value, test.maximum)
			if test.wantPart == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantPart) {
				t.Fatalf("error=%v, want containing %q", err, test.wantPart)
			}
		})
	}
}

func TestCompressedEvidenceRejectsTrailingMembersAndData(t *testing.T) {
	writeMember := func(buffer *bytes.Buffer, value string) {
		t.Helper()
		writer := gzip.NewWriter(buffer)
		if _, err := writer.Write([]byte(value)); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
	}
	tests := map[string]struct {
		build    func(*bytes.Buffer)
		wantPart string
	}{
		"one exact member": {
			build: func(buffer *bytes.Buffer) {
				writeMember(buffer, "{}")
			},
		},
		"second gzip member": {
			build: func(buffer *bytes.Buffer) {
				writeMember(buffer, "{}")
				writeMember(buffer, "{}")
			},
			wantPart: "trailing data",
		},
		"trailing bytes": {
			build: func(buffer *bytes.Buffer) {
				writeMember(buffer, "{}")
				buffer.WriteString("trailing")
			},
			wantPart: "trailing data",
		},
		"trailing JSON value": {
			build: func(buffer *bytes.Buffer) {
				writeMember(buffer, "{}{}")
			},
			wantPart: "trailing value",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var content bytes.Buffer
			test.build(&content)
			path := filepath.Join(t.TempDir(), "value.json.gz")
			if err := os.WriteFile(path, content.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}
			var value map[string]any
			err := readCompressedJSON(path, &value)
			if test.wantPart == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantPart) {
				t.Fatalf("error=%v, want containing %q", err, test.wantPart)
			}
		})
	}
}
