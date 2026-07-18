package wireeval

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/oracle"
)

func TestCandidateProvenanceIsClosedAndBindsExecutable(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "candidate")
	if err := os.WriteFile(
		executable,
		[]byte("candidate executable"),
		0o700,
	); err != nil {
		t.Fatal(err)
	}
	checksum, err := fileSHA256(executable)
	if err != nil {
		t.Fatal(err)
	}
	valid := CandidateProvenance{
		SourceRevision:   strings.Repeat("a", 40),
		SourceTree:       strings.Repeat("b", 40),
		GoModSHA256:      strings.Repeat("c", 64),
		GoSumSHA256:      strings.Repeat("d", 64),
		ExecutableSHA256: checksum,
		GoVersion:        runtime.Version(),
		Package:          candidateAgentPackage,
		CGO:              "0",
	}
	if err := validateCandidateExecutable(valid, executable); err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*CandidateProvenance){
		"source revision": func(value *CandidateProvenance) {
			value.SourceRevision = "revision"
		},
		"source tree": func(value *CandidateProvenance) {
			value.SourceTree = "tree"
		},
		"Go module": func(value *CandidateProvenance) {
			value.GoModSHA256 = "checksum"
		},
		"Go sum": func(value *CandidateProvenance) {
			value.GoSumSHA256 = "checksum"
		},
		"executable": func(value *CandidateProvenance) {
			value.ExecutableSHA256 = strings.Repeat("e", 64)
		},
		"Go version": func(value *CandidateProvenance) {
			value.GoVersion = "unknown"
		},
		"package": func(value *CandidateProvenance) {
			value.Package = "example.test/candidate"
		},
		"CGO": func(value *CandidateProvenance) {
			value.CGO = "1"
		},
		"tags": func(value *CandidateProvenance) {
			value.Tags = "unexpected"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if err := validateCandidateExecutable(
				candidate,
				executable,
			); err == nil {
				t.Fatal("invalid Candidate provenance was accepted")
			}
		})
	}
}

func TestEvidenceManifestRetainsCandidateProvenance(t *testing.T) {
	candidate := CandidateProvenance{
		SourceRevision:   strings.Repeat("a", 40),
		SourceTree:       strings.Repeat("b", 40),
		GoModSHA256:      strings.Repeat("c", 64),
		GoSumSHA256:      strings.Repeat("d", 64),
		ExecutableSHA256: strings.Repeat("e", 64),
		GoVersion:        runtime.Version(),
		Package:          candidateAgentPackage,
		CGO:              "0",
	}
	directory := filepath.Join(t.TempDir(), "bundle")
	bundle, err := NewEvidenceBundle(directory)
	if err != nil {
		t.Fatal(err)
	}
	bundle.runs = make([]evidenceRun, expectedExperimentRunCount)
	summaries := make(
		[]oracle.RunSummary,
		expectedExperimentRunCount,
	)
	result := oracle.ExperimentResult{
		Gates: make([]oracle.Gate, 171),
		Pass:  true,
	}
	if err := bundle.Finalize(
		strings.Repeat("f", 64),
		summaries,
		result,
		&candidate,
	); err != nil {
		t.Fatal(err)
	}
	recorded, ok, err := EvidenceCandidateProvenance(directory)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || recorded != candidate {
		t.Fatalf(
			"Candidate provenance=(%+v,%v), want %+v",
			recorded,
			ok,
			candidate,
		)
	}
}
