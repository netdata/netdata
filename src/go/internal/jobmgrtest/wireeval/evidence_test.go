package wireeval

import (
	"bytes"
	"os"
	"path/filepath"
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
