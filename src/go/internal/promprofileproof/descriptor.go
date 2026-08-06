// SPDX-License-Identifier: GPL-3.0-or-later

package promprofileproof

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	DescriptorFilename = "proof.yaml"
	ProofRoot          = "src/go/plugin/go.d/collector/prometheus/profile-proofs"
	StockProfileRoot   = "src/go/plugin/go.d/config/go.d/prometheus.profiles/default"
)

type Descriptor struct {
	Version    int                     `yaml:"version"`
	Profile    string                  `yaml:"profile"`
	External   ExternalEvidence        `yaml:"external_evidence"`
	Inventory  SourceInventoryExpected `yaml:"source_inventory"`
	Validation Validation              `yaml:"validation"`
	Integrity  Integrity               `yaml:"integrity"`
}

type ExternalEvidence struct {
	Revision string     `yaml:"revision"`
	Manifest FileDigest `yaml:"manifest"`
}

type Validation struct {
	MetadataExample MetadataExample  `yaml:"metadata_example"`
	Cases           []ValidationCase `yaml:"cases"`
}

type MetadataExample struct {
	IntegrationID string `yaml:"integration_id"`
	ExampleName   string `yaml:"example_name"`
	JobName       string `yaml:"job_name"`
}

type ValidationCase struct {
	Name     string         `yaml:"name"`
	Kind     string         `yaml:"kind"`
	Fixture  string         `yaml:"fixture"`
	Job      string         `yaml:"job"`
	Expected ExpectedResult `yaml:"expected"`
}

type ExpectedResult struct {
	Verdict  string         `yaml:"verdict"`
	Counts   ExpectedCounts `yaml:"counts"`
	Errors   map[string]int `yaml:"errors"`
	Warnings map[string]int `yaml:"warnings"`
}

type ExpectedCounts struct {
	RawFamilies         int `yaml:"raw_families"`
	RawLogicalSeries    int `yaml:"raw_logical_series"`
	WriterSeries        int `yaml:"writer_series"`
	SeriesScanned       int `yaml:"series_scanned"`
	SeriesAutogen       int `yaml:"series_autogen"`
	SeriesUnmatched     int `yaml:"series_unmatched"`
	AuthoredCharts      int `yaml:"authored_charts"`
	RuntimeCharts       int `yaml:"runtime_charts"`
	AutogenCharts       int `yaml:"autogen_charts"`
	ChartDimensions     int `yaml:"chart_dimensions"`
	PipelineExcluded    int `yaml:"pipeline_excluded"`
	PipelineRenamed     int `yaml:"pipeline_renamed"`
	DeadCharts          int `yaml:"dead_charts"`
	DeadDimensions      int `yaml:"dead_dimensions"`
	DimensionLosses     int `yaml:"dimension_losses"`
	InstanceLosses      int `yaml:"instance_losses"`
	Collisions          int `yaml:"collisions"`
	ChartWireCollisions int `yaml:"chart_wire_collisions"`
	ContextCollisions   int `yaml:"context_wire_collisions"`
	DimensionCollisions int `yaml:"dimension_collisions"`
}

type SourceInventoryExpected struct {
	Rows              int                  `yaml:"rows"`
	SourceFamilies    int                  `yaml:"source_families"`
	AuthoredSelectors int                  `yaml:"authored_selectors"`
	Dispositions      InventoryDisposition `yaml:"dispositions"`
}

type InventoryDisposition struct {
	Chart            int `yaml:"chart"`
	JobExcluded      int `yaml:"job_excluded"`
	WriterIneligible int `yaml:"writer_ineligible"`
}

type FileDigest struct {
	SHA256 string `yaml:"sha256"`
	Bytes  int64  `yaml:"bytes"`
}

type Integrity struct {
	Evidence          FileDigest `yaml:"evidence"`
	OperatorModel     FileDigest `yaml:"operator_model"`
	ValidationJob     FileDigest `yaml:"validation_job"`
	ValidationSummary FileDigest `yaml:"validation_summary"`
	Profile           FileDigest `yaml:"profile"`
}

type FileIntegrity struct {
	Role string
	Path string
	FileDigest
}

type integrityTarget struct {
	role   string
	path   string
	digest *FileDigest
}

type Bundle struct {
	Path       string
	Descriptor Descriptor
}

func (b Bundle) ProofDirectory() string {
	return filepath.ToSlash(filepath.Dir(filepath.FromSlash(b.Path)))
}

func (b Bundle) ProfilePath() string {
	return StockProfileRoot + "/" + b.Descriptor.Profile + ".yaml"
}

func (b Bundle) EvidencePath() string {
	return b.ProofDirectory() + "/EVIDENCE.md"
}

func (b Bundle) OperatorModelPath() string {
	return b.ProofDirectory() + "/OPERATOR-MODEL.md"
}

func (b Bundle) ValidationJobPath() string {
	return b.ProofDirectory() + "/VALIDATION-JOB.yaml"
}

func (b Bundle) ValidationSummaryPath() string {
	return b.ProofDirectory() + "/VALIDATION.md"
}

func (b Bundle) ExternalRoot() string {
	return "prometheus/profiles/" + b.Descriptor.External.Revision
}

func (b Bundle) ExternalManifestPath() string {
	return b.ExternalRoot() + "/manifest.yaml"
}

func (b Bundle) SourceInventoryPath() string {
	return b.ExternalRoot() + "/SOURCE-INVENTORY.tsv"
}

func (b Bundle) FixturePath(validationCase ValidationCase) string {
	return b.ExternalRoot() + "/" + validationCase.Fixture
}

func (b Bundle) IntegrityEntries() []FileIntegrity {
	targets := (&b).integrityTargets()
	entries := make([]FileIntegrity, 0, len(targets))
	for _, target := range targets {
		entries = append(entries, FileIntegrity{Role: target.role, Path: target.path, FileDigest: *target.digest})
	}
	return entries
}

func (b *Bundle) integrityTargets() []integrityTarget {
	return []integrityTarget{
		{role: "evidence", path: b.EvidencePath(), digest: &b.Descriptor.Integrity.Evidence},
		{role: "operator_model", path: b.OperatorModelPath(), digest: &b.Descriptor.Integrity.OperatorModel},
		{role: "validation_job", path: b.ValidationJobPath(), digest: &b.Descriptor.Integrity.ValidationJob},
		{role: "validation_summary", path: b.ValidationSummaryPath(), digest: &b.Descriptor.Integrity.ValidationSummary},
		{role: "profile", path: b.ProfilePath(), digest: &b.Descriptor.Integrity.Profile},
	}
}

func Discover(repoRoot string) ([]Bundle, error) {
	proofRoot := filepath.Join(repoRoot, filepath.FromSlash(ProofRoot))
	entries, err := os.ReadDir(proofRoot)
	if err != nil {
		return nil, fmt.Errorf("read proof root: %w", err)
	}

	var bundles []Bundle
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(proofRoot, entry.Name(), DescriptorFilename)
		bundle, err := Load(repoRoot, path)
		if err != nil {
			return nil, fmt.Errorf("proof directory %q: %w", entry.Name(), err)
		}
		bundles = append(bundles, bundle)
	}
	if len(bundles) == 0 {
		return nil, errors.New("no Prometheus profile proof descriptors found")
	}
	slices.SortFunc(bundles, func(left, right Bundle) int {
		return strings.Compare(left.Descriptor.Profile, right.Descriptor.Profile)
	})

	seenNames := make(map[string]string, len(bundles))
	for _, bundle := range bundles {
		name := bundle.Descriptor.Profile
		if previous := seenNames[name]; previous != "" {
			return nil, fmt.Errorf("duplicate profile name %q in %q and %q", name, previous, bundle.Path)
		}
		seenNames[name] = bundle.Path
	}
	return bundles, nil
}

func Load(repoRoot, path string) (Bundle, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Bundle{}, fmt.Errorf("read descriptor: %w", err)
	}
	var descriptor Descriptor
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	if err := decoder.Decode(&descriptor); err != nil {
		return Bundle{}, fmt.Errorf("decode descriptor: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Bundle{}, errors.New("descriptor must contain exactly one YAML document")
		}
		return Bundle{}, fmt.Errorf("decode trailing descriptor content: %w", err)
	}

	relativePath, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return Bundle{}, fmt.Errorf("resolve descriptor path: %w", err)
	}
	bundle := Bundle{Path: filepath.ToSlash(relativePath), Descriptor: descriptor}
	if err := bundle.validate(); err != nil {
		return Bundle{}, err
	}
	return bundle, nil
}

func (b Bundle) validate() error {
	descriptor := b.Descriptor
	if descriptor.Version != 2 {
		return fmt.Errorf("version: got %d, want 2", descriptor.Version)
	}
	proofDirectory := b.ProofDirectory()
	if filepath.ToSlash(filepath.Dir(proofDirectory)) != ProofRoot {
		return fmt.Errorf("descriptor path %q is not directly below %q", b.Path, ProofRoot)
	}
	if filepath.Base(b.Path) != DescriptorFilename {
		return fmt.Errorf("descriptor path %q must end in %q", b.Path, DescriptorFilename)
	}
	if descriptor.Profile == "" {
		return errors.New("profile must not be empty")
	}
	if descriptor.Profile != filepath.Base(filepath.FromSlash(proofDirectory)) {
		return fmt.Errorf("profile %q must match proof directory %q", descriptor.Profile, proofDirectory)
	}
	for _, entry := range b.IntegrityEntries() {
		if err := validateRelativePath(entry.Role+" path", entry.Path); err != nil {
			return err
		}
	}

	if descriptor.External.Revision == "" || strings.ContainsAny(descriptor.External.Revision, "/\\") {
		return fmt.Errorf("external_evidence.revision %q must be one non-empty path segment", descriptor.External.Revision)
	}
	for _, item := range []struct {
		field string
		path  string
	}{
		{"external manifest path", b.ExternalManifestPath()},
		{"source inventory path", b.SourceInventoryPath()},
	} {
		if err := validateRelativePath(item.field, item.path); err != nil {
			return err
		}
	}
	if err := validateDigest("external_evidence.manifest", descriptor.External.Manifest.SHA256, descriptor.External.Manifest.Bytes); err != nil {
		return err
	}
	if err := validateInventoryExpected(descriptor.Inventory); err != nil {
		return err
	}

	metadata := descriptor.Validation.MetadataExample
	if metadata.IntegrationID == "" || metadata.ExampleName == "" || metadata.JobName == "" {
		return errors.New("validation.metadata_example fields must not be empty")
	}
	if len(descriptor.Validation.Cases) == 0 {
		return errors.New("validation.cases must not be empty")
	}
	seenCases := make(map[string]bool, len(descriptor.Validation.Cases))
	sourceComplete := 0
	for index, validationCase := range descriptor.Validation.Cases {
		field := fmt.Sprintf("validation.cases[%d]", index)
		if validationCase.Name == "" {
			return fmt.Errorf("%s.name must not be empty", field)
		}
		if seenCases[validationCase.Name] {
			return fmt.Errorf("duplicate validation case name %q", validationCase.Name)
		}
		seenCases[validationCase.Name] = true
		switch validationCase.Kind {
		case "source_complete":
			sourceComplete++
			if validationCase.Expected.Verdict != "PASS" {
				return fmt.Errorf("%s source_complete case must expect PASS", field)
			}
		case "supplemental":
		default:
			return fmt.Errorf("%s.kind %q must be source_complete or supplemental", field, validationCase.Kind)
		}
		switch validationCase.Job {
		case "validation", "none":
		default:
			return fmt.Errorf("%s.job %q must be validation or none", field, validationCase.Job)
		}
		if err := validateRelativePath(field+".fixture", validationCase.Fixture); err != nil {
			return err
		}
		if !strings.HasPrefix(validationCase.Fixture, "fixtures/") || !strings.HasSuffix(validationCase.Fixture, ".prom") {
			return fmt.Errorf("%s.fixture %q must be a fixtures/*.prom path", field, validationCase.Fixture)
		}
		if err := validateExpectedResult(field+".expected", validationCase.Expected); err != nil {
			return err
		}
	}
	if sourceComplete != 1 {
		return fmt.Errorf("validation.cases must contain exactly one source_complete case, got %d", sourceComplete)
	}

	for _, entry := range b.IntegrityEntries() {
		if err := validateDigest("integrity."+entry.Role, entry.SHA256, entry.Bytes); err != nil {
			return err
		}
	}
	return nil
}

func validateInventoryExpected(expected SourceInventoryExpected) error {
	for _, item := range []struct {
		field string
		value int
	}{
		{"rows", expected.Rows},
		{"source_families", expected.SourceFamilies},
		{"authored_selectors", expected.AuthoredSelectors},
		{"dispositions.chart", expected.Dispositions.Chart},
	} {
		if item.value <= 0 {
			return fmt.Errorf("source_inventory.%s must be positive", item.field)
		}
	}
	for _, item := range []struct {
		field string
		value int
	}{
		{"dispositions.job_excluded", expected.Dispositions.JobExcluded},
		{"dispositions.writer_ineligible", expected.Dispositions.WriterIneligible},
	} {
		if item.value < 0 {
			return fmt.Errorf("source_inventory.%s must be non-negative", item.field)
		}
	}
	if got := expected.Dispositions.Chart + expected.Dispositions.JobExcluded + expected.Dispositions.WriterIneligible; got != expected.Rows {
		return fmt.Errorf("source_inventory disposition total %d differs from rows %d", got, expected.Rows)
	}
	if expected.SourceFamilies > expected.Rows {
		return fmt.Errorf("source_inventory.source_families %d exceeds rows %d", expected.SourceFamilies, expected.Rows)
	}
	if expected.AuthoredSelectors > expected.Dispositions.Chart {
		return fmt.Errorf("source_inventory.authored_selectors %d exceeds chart dispositions %d",
			expected.AuthoredSelectors, expected.Dispositions.Chart)
	}
	return nil
}

func validateExpectedResult(field string, expected ExpectedResult) error {
	if expected.Verdict != "PASS" && expected.Verdict != "FAIL" {
		return fmt.Errorf("%s.verdict %q must be PASS or FAIL", field, expected.Verdict)
	}
	values := []struct {
		name  string
		value int
	}{
		{"raw_families", expected.Counts.RawFamilies},
		{"raw_logical_series", expected.Counts.RawLogicalSeries},
		{"writer_series", expected.Counts.WriterSeries},
		{"series_scanned", expected.Counts.SeriesScanned},
		{"series_autogen", expected.Counts.SeriesAutogen},
		{"series_unmatched", expected.Counts.SeriesUnmatched},
		{"authored_charts", expected.Counts.AuthoredCharts},
		{"runtime_charts", expected.Counts.RuntimeCharts},
		{"autogen_charts", expected.Counts.AutogenCharts},
		{"chart_dimensions", expected.Counts.ChartDimensions},
		{"pipeline_excluded", expected.Counts.PipelineExcluded},
		{"pipeline_renamed", expected.Counts.PipelineRenamed},
		{"dead_charts", expected.Counts.DeadCharts},
		{"dead_dimensions", expected.Counts.DeadDimensions},
		{"dimension_losses", expected.Counts.DimensionLosses},
		{"instance_losses", expected.Counts.InstanceLosses},
		{"collisions", expected.Counts.Collisions},
		{"chart_wire_collisions", expected.Counts.ChartWireCollisions},
		{"context_wire_collisions", expected.Counts.ContextCollisions},
		{"dimension_collisions", expected.Counts.DimensionCollisions},
	}
	for _, item := range values {
		if item.value < 0 {
			return fmt.Errorf("%s.counts.%s must be non-negative", field, item.name)
		}
	}
	if expected.Counts.RawFamilies == 0 || expected.Counts.AuthoredCharts == 0 {
		return fmt.Errorf("%s counts must include positive raw_families and authored_charts", field)
	}
	if err := validateFindingCounts(field+".errors", expected.Errors); err != nil {
		return err
	}
	if err := validateFindingCounts(field+".warnings", expected.Warnings); err != nil {
		return err
	}
	if expected.Verdict == "PASS" && len(expected.Errors) != 0 {
		return fmt.Errorf("%s PASS case must not expect errors", field)
	}
	if expected.Verdict == "FAIL" && len(expected.Errors) == 0 {
		return fmt.Errorf("%s FAIL case must expect at least one error", field)
	}
	return nil
}

func validateFindingCounts(field string, findings map[string]int) error {
	codes := make([]string, 0, len(findings))
	for code := range findings {
		codes = append(codes, code)
	}
	slices.Sort(codes)
	for _, code := range codes {
		if code == "" || findings[code] <= 0 {
			return fmt.Errorf("%s must contain non-empty codes with positive counts", field)
		}
	}
	return nil
}

func validateRelativePath(field, path string) error {
	if path == "" {
		return fmt.Errorf("%s must not be empty", field)
	}
	native := filepath.FromSlash(path)
	if filepath.IsAbs(native) {
		return fmt.Errorf("%s %q must be relative", field, path)
	}
	clean := filepath.Clean(native)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s %q escapes its root", field, path)
	}
	if filepath.ToSlash(clean) != path {
		return fmt.Errorf("%s %q must use canonical slash form", field, path)
	}
	return nil
}

func validateDigest(field, digest string, size int64) error {
	decoded, err := hex.DecodeString(digest)
	if err != nil || len(decoded) != sha256.Size || digest != strings.ToLower(digest) {
		return fmt.Errorf("%s.sha256 %q must be a lowercase SHA-256", field, digest)
	}
	if size < 0 {
		return fmt.Errorf("%s.bytes must be non-negative", field)
	}
	return nil
}
