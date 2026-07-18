package wireeval

import (
	"encoding/hex"
	"errors"
	"strings"
)

const candidateAgentPackage = "github.com/netdata/netdata/go/plugins/internal/jobmgrtest/cmd/agent"

type CandidateProvenance struct {
	SourceRevision   string `json:"source_revision"`
	SourceTree       string `json:"source_tree"`
	GoModSHA256      string `json:"go_mod_sha256"`
	GoSumSHA256      string `json:"go_sum_sha256"`
	ExecutableSHA256 string `json:"executable_sha256"`
	GoVersion        string `json:"go_version"`
	Package          string `json:"package"`
	CGO              string `json:"cgo"`
	Tags             string `json:"tags"`
}

func (provenance CandidateProvenance) validate() error {
	if !validGitObject(provenance.SourceRevision) ||
		!validGitObject(provenance.SourceTree) ||
		!validSHA256(provenance.GoModSHA256) ||
		!validSHA256(provenance.GoSumSHA256) ||
		!validSHA256(provenance.ExecutableSHA256) ||
		!strings.HasPrefix(provenance.GoVersion, "go1.") ||
		provenance.Package != candidateAgentPackage ||
		provenance.CGO != "0" ||
		provenance.Tags != "" {
		return errors.New("wire evaluator: invalid Candidate provenance")
	}
	return nil
}

func validateCandidateExecutable(
	provenance CandidateProvenance,
	executable string,
) error {
	if err := provenance.validate(); err != nil {
		return err
	}
	checksum, err := fileSHA256(executable)
	if err != nil {
		return err
	}
	if checksum != provenance.ExecutableSHA256 {
		return errors.New(
			"wire evaluator: Candidate executable differs from provenance",
		)
	}
	return nil
}

func validGitObject(value string) bool {
	return (len(value) == 40 || len(value) == 64) && validHex(value)
}

func validSHA256(value string) bool {
	return len(value) == 64 && validHex(value)
}

func validHex(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded)*2 == len(value)
}
