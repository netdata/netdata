package wireeval

import (
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/oracle"
)

const (
	evidenceVersion            = 1
	maximumRunEvidenceBytes    = 48 * 1024 * 1024
	maximumCompressedRunBytes  = 64 * 1024 * 1024
	maximumManifestBytes       = 1024 * 1024
	expectedExperimentRunCount = 270
)

type EvidenceBundle struct {
	directory string
	runs      []evidenceRun
	finalized bool
}

type evidenceRun struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
}

type evidenceManifest struct {
	Version           int                  `json:"version"`
	EnvironmentSHA256 string               `json:"environment_sha256"`
	Candidate         *CandidateProvenance `json:"candidate,omitempty"`
	Runs              []evidenceRun        `json:"runs"`
	SummariesFile     string               `json:"summaries_file"`
	SummariesSHA256   string               `json:"summaries_sha256"`
	GatesFile         string               `json:"gates_file"`
	GatesSHA256       string               `json:"gates_sha256"`
}

type runEvidence struct {
	Version     int                   `json:"version"`
	Observation oracle.RunObservation `json:"observation"`
}

func NewEvidenceBundle(directory string) (*EvidenceBundle, error) {
	if !filepath.IsAbs(directory) {
		return nil, errors.New("wire evaluator: evidence directory must be absolute")
	}
	if err := os.Mkdir(directory, 0o750); err != nil {
		return nil, fmt.Errorf("wire evaluator: create evidence directory: %w", err)
	}
	if err := os.Mkdir(filepath.Join(directory, "runs"), 0o750); err != nil {
		return nil, fmt.Errorf("wire evaluator: create evidence runs directory: %w", err)
	}
	return &EvidenceBundle{
		directory: directory,
		runs:      make([]evidenceRun, 0, expectedExperimentRunCount),
	}, nil
}

func (bundle *EvidenceBundle) Record(observation oracle.RunObservation) error {
	if bundle == nil || bundle.finalized {
		return errors.New("wire evaluator: evidence bundle is unavailable")
	}
	if len(bundle.runs) >= expectedExperimentRunCount {
		return errors.New("wire evaluator: evidence run count exceeded")
	}
	filename := fmt.Sprintf(
		"%03d-%s-p%d-pair%02d-%s.json.gz",
		len(bundle.runs),
		sanitizeEvidenceName(observation.WorkloadID),
		observation.Population,
		observation.PairIndex,
		observation.Side,
	)
	relative := filepath.Join("runs", filename)
	path := filepath.Join(bundle.directory, relative)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("wire evaluator: create run evidence: %w", err)
	}
	compressed := &boundedWriter{target: file, maximum: maximumCompressedRunBytes}
	zipper, err := gzip.NewWriterLevel(compressed, gzip.BestSpeed)
	if err != nil {
		_ = file.Close()
		return err
	}
	uncompressed := &boundedWriter{target: zipper, maximum: maximumRunEvidenceBytes}
	encodeErr := json.NewEncoder(uncompressed).Encode(runEvidence{
		Version: evidenceVersion, Observation: observation,
	})
	closeErr := errors.Join(zipper.Close(), file.Close())
	if err := errors.Join(encodeErr, closeErr); err != nil {
		return fmt.Errorf("wire evaluator: write run evidence: %w", err)
	}
	checksum, err := fileSHA256(path)
	if err != nil {
		return err
	}
	bundle.runs = append(bundle.runs, evidenceRun{
		File: filepath.ToSlash(relative), SHA256: checksum,
	})
	return nil
}

func (bundle *EvidenceBundle) Finalize(
	environmentSHA256 string,
	summaries []oracle.RunSummary,
	result oracle.ExperimentResult,
	candidate *CandidateProvenance,
) error {
	if bundle == nil || bundle.finalized {
		return errors.New("wire evaluator: evidence bundle is unavailable")
	}
	if len(bundle.runs) != expectedExperimentRunCount ||
		len(summaries) != expectedExperimentRunCount ||
		len(result.Gates) != 171 {
		return fmt.Errorf(
			"wire evaluator: incomplete evidence runs=%d summaries=%d gates=%d",
			len(bundle.runs),
			len(summaries),
			len(result.Gates),
		)
	}
	summariesFile := "summaries.json.gz"
	if err := writeCompressedJSON(
		filepath.Join(bundle.directory, summariesFile),
		summaries,
		maximumRunEvidenceBytes,
	); err != nil {
		return err
	}
	gatesFile := "gates.json"
	if err := writeExclusiveJSON(filepath.Join(bundle.directory, gatesFile), result); err != nil {
		return err
	}
	summariesSHA256, err := fileSHA256(filepath.Join(bundle.directory, summariesFile))
	if err != nil {
		return err
	}
	gatesSHA256, err := fileSHA256(filepath.Join(bundle.directory, gatesFile))
	if err != nil {
		return err
	}
	var recordedCandidate *CandidateProvenance
	if candidate != nil {
		copied := *candidate
		recordedCandidate = &copied
	}
	manifest := evidenceManifest{
		Version:           evidenceVersion,
		EnvironmentSHA256: environmentSHA256,
		Candidate:         recordedCandidate,
		Runs:              append([]evidenceRun(nil), bundle.runs...),
		SummariesFile:     summariesFile,
		SummariesSHA256:   summariesSHA256,
		GatesFile:         gatesFile,
		GatesSHA256:       gatesSHA256,
	}
	if err := writeExclusiveJSON(
		filepath.Join(bundle.directory, "manifest.json"),
		manifest,
	); err != nil {
		return err
	}
	bundle.finalized = true
	return nil
}

func VerifyEvidenceBundle(directory string) (oracle.ExperimentResult, error) {
	if !filepath.IsAbs(directory) {
		return oracle.ExperimentResult{}, errors.New(
			"wire evaluator: evidence directory must be absolute",
		)
	}
	manifest, err := readEvidenceManifest(directory)
	if err != nil {
		return oracle.ExperimentResult{}, err
	}
	if len(manifest.Runs) != expectedExperimentRunCount {
		return oracle.ExperimentResult{}, errors.New("wire evaluator: invalid evidence manifest")
	}
	seenFiles := map[string]struct{}{"manifest.json": {}}
	artifacts := []struct {
		name, checksum string
		maximum        int64
	}{
		{
			name: manifest.SummariesFile, checksum: manifest.SummariesSHA256,
			maximum: maximumCompressedRunBytes,
		},
		{
			name: manifest.GatesFile, checksum: manifest.GatesSHA256,
			maximum: maximumRunEvidenceBytes,
		},
	}
	artifactPaths := make(map[string]string, len(artifacts))
	for _, file := range artifacts {
		path, err := evidenceRegularFile(
			directory,
			file.name,
			file.maximum,
		)
		if err != nil {
			return oracle.ExperimentResult{}, err
		}
		if _, exists := seenFiles[file.name]; exists {
			return oracle.ExperimentResult{}, errors.New(
				"wire evaluator: duplicate evidence path",
			)
		}
		seenFiles[file.name] = struct{}{}
		if err := verifyFileSHA256(path, file.checksum); err != nil {
			return oracle.ExperimentResult{}, err
		}
		artifactPaths[file.name] = path
	}
	summaries := make([]oracle.RunSummary, 0, len(manifest.Runs))
	for _, run := range manifest.Runs {
		if !strings.HasPrefix(run.File, "runs/") ||
			!strings.HasSuffix(run.File, ".json.gz") {
			return oracle.ExperimentResult{}, errors.New(
				"wire evaluator: invalid run evidence path",
			)
		}
		path, err := evidenceRegularFile(
			directory,
			run.File,
			maximumCompressedRunBytes,
		)
		if err != nil {
			return oracle.ExperimentResult{}, err
		}
		if _, exists := seenFiles[run.File]; exists {
			return oracle.ExperimentResult{}, errors.New(
				"wire evaluator: duplicate evidence path",
			)
		}
		seenFiles[run.File] = struct{}{}
		if err := verifyFileSHA256(path, run.SHA256); err != nil {
			return oracle.ExperimentResult{}, err
		}
		evidence, err := readRunEvidence(path)
		if err != nil {
			return oracle.ExperimentResult{}, err
		}
		if evidence.Version != evidenceVersion ||
			evidence.Observation.EnvironmentSHA256 != manifest.EnvironmentSHA256 {
			return oracle.ExperimentResult{}, errors.New("wire evaluator: run evidence identity differs")
		}
		summary, err := oracle.AnalyzeRun(evidence.Observation)
		if err != nil {
			return oracle.ExperimentResult{}, err
		}
		summaries = append(summaries, summary)
	}
	replayed, err := oracle.EvaluateExperiment(summaries)
	if err != nil {
		return oracle.ExperimentResult{}, err
	}
	var recordedSummaries []oracle.RunSummary
	if err := readCompressedJSON(
		artifactPaths[manifest.SummariesFile],
		&recordedSummaries,
	); err != nil {
		return oracle.ExperimentResult{}, err
	}
	var recordedResult oracle.ExperimentResult
	if err := readJSONFileBounded(
		artifactPaths[manifest.GatesFile],
		&recordedResult,
		maximumRunEvidenceBytes,
	); err != nil {
		return oracle.ExperimentResult{}, err
	}
	replayedJSON, err := json.Marshal(struct {
		Summaries []oracle.RunSummary
		Result    oracle.ExperimentResult
	}{Summaries: summaries, Result: replayed})
	if err != nil {
		return oracle.ExperimentResult{}, err
	}
	recordedJSON, err := json.Marshal(struct {
		Summaries []oracle.RunSummary
		Result    oracle.ExperimentResult
	}{Summaries: recordedSummaries, Result: recordedResult})
	if err != nil {
		return oracle.ExperimentResult{}, err
	}
	if string(replayedJSON) != string(recordedJSON) {
		return oracle.ExperimentResult{}, errors.New("wire evaluator: replay differs from recorded summaries or gates")
	}
	return replayed, nil
}

func EvidenceCandidateProvenance(
	directory string,
) (CandidateProvenance, bool, error) {
	if !filepath.IsAbs(directory) {
		return CandidateProvenance{}, false, errors.New(
			"wire evaluator: evidence directory must be absolute",
		)
	}
	manifest, err := readEvidenceManifest(directory)
	if err != nil {
		return CandidateProvenance{}, false, err
	}
	if manifest.Candidate == nil {
		return CandidateProvenance{}, false, nil
	}
	return *manifest.Candidate, true, nil
}

func readEvidenceManifest(directory string) (evidenceManifest, error) {
	var manifest evidenceManifest
	manifestPath, err := evidenceRegularFile(
		directory,
		"manifest.json",
		maximumManifestBytes,
	)
	if err != nil {
		return evidenceManifest{}, err
	}
	if err := readJSONFileBounded(
		manifestPath,
		&manifest,
		maximumManifestBytes,
	); err != nil {
		return evidenceManifest{}, err
	}
	if manifest.Version != evidenceVersion {
		return evidenceManifest{}, errors.New(
			"wire evaluator: invalid evidence manifest",
		)
	}
	if manifest.Candidate != nil {
		if err := manifest.Candidate.validate(); err != nil {
			return evidenceManifest{}, err
		}
	}
	return manifest, nil
}

func readRunEvidence(path string) (runEvidence, error) {
	var evidence runEvidence
	if err := readCompressedJSON(path, &evidence); err != nil {
		return runEvidence{}, err
	}
	return evidence, nil
}

func readCompressedJSON(path string, value any) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("wire evaluator: evidence file is not regular")
	}
	if info.Size() > maximumCompressedRunBytes {
		return errors.New("wire evaluator: compressed evidence exceeds bound")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	buffered := bufio.NewReader(file)
	zipper, err := gzip.NewReader(buffered)
	if err != nil {
		return err
	}
	zipper.Multistream(false)
	defer zipper.Close()
	limited := &io.LimitedReader{R: zipper, N: maximumRunEvidenceBytes + 1}
	if err := decodeExactJSON(limited, value); err != nil {
		return err
	}
	if limited.N <= 0 {
		return errors.New("wire evaluator: uncompressed evidence exceeds bound")
	}
	if err := zipper.Close(); err != nil {
		return err
	}
	if _, err := buffered.ReadByte(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New(
				"wire evaluator: compressed evidence has trailing data",
			)
		}
		return err
	}
	return nil
}

func writeCompressedJSON(path string, value any, maximum int64) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	compressed := &boundedWriter{target: file, maximum: maximumCompressedRunBytes}
	zipper, err := gzip.NewWriterLevel(compressed, gzip.BestSpeed)
	if err != nil {
		_ = file.Close()
		return err
	}
	uncompressed := &boundedWriter{target: zipper, maximum: maximum}
	return errors.Join(
		json.NewEncoder(uncompressed).Encode(value),
		zipper.Close(),
		file.Close(),
	)
}

func writeExclusiveJSON(path string, value any) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	return errors.Join(json.NewEncoder(file).Encode(value), file.Close())
}

func readJSONFile(path string, value any) error {
	return readJSONFileBounded(path, value, maximumRunEvidenceBytes)
}

func readJSONFileBounded(
	path string,
	value any,
	maximum int64,
) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("wire evaluator: evidence file is not regular")
	}
	if info.Size() > maximum {
		return errors.New("wire evaluator: JSON evidence exceeds bound")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return decodeExactJSON(io.LimitReader(file, maximum+1), value)
}

func decodeExactJSON(reader io.Reader, value any) error {
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("wire evaluator: JSON evidence has trailing value")
		}
		return err
	}
	return nil
}

func evidenceRegularFile(
	directory string,
	name string,
	maximum int64,
) (string, error) {
	if name == "" ||
		strings.Contains(name, "\\") ||
		filepath.IsAbs(name) ||
		filepath.Clean(name) == "." ||
		filepath.Clean(name) == ".." ||
		strings.HasPrefix(filepath.Clean(name), ".."+string(filepath.Separator)) ||
		filepath.ToSlash(filepath.Clean(name)) != name {
		return "", errors.New("wire evaluator: unsafe evidence path")
	}
	path := filepath.Join(directory, filepath.FromSlash(name))
	relative, err := filepath.Rel(directory, path)
	if err != nil ||
		relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("wire evaluator: evidence path escapes bundle")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("wire evaluator: evidence path is not a regular file")
	}
	if info.Size() > maximum {
		return "", errors.New("wire evaluator: evidence file exceeds bound")
	}
	return path, nil
}

func verifyFileSHA256(path, want string) error {
	got, err := fileSHA256(path)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("wire evaluator: checksum differs for %s", filepath.Base(path))
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func sanitizeEvidenceName(value string) string {
	return strings.NewReplacer("/", "-", "\\", "-", ":", "-").Replace(value)
}

type boundedWriter struct {
	target  io.Writer
	written int64
	maximum int64
}

func (writer *boundedWriter) Write(payload []byte) (int, error) {
	if int64(len(payload)) > writer.maximum-writer.written {
		return 0, errors.New("wire evaluator: evidence exceeds bound")
	}
	count, err := writer.target.Write(payload)
	writer.written += int64(count)
	if err == nil && count != len(payload) {
		err = io.ErrShortWrite
	}
	return count, err
}
