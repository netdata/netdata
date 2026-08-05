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
	Version    int              `yaml:"version"`
	Profile    Profile          `yaml:"profile"`
	Proof      ProofDocuments   `yaml:"proof"`
	External   ExternalEvidence `yaml:"external_evidence"`
	Validation Validation       `yaml:"validation"`
	Integrity  []FileIntegrity  `yaml:"integrity"`
}

type Profile struct {
	Name string `yaml:"name"`
	Path string `yaml:"path"`
}

type ProofDocuments struct {
	Evidence          string `yaml:"evidence"`
	OperatorModel     string `yaml:"operator_model"`
	ValidationSummary string `yaml:"validation_summary"`
}

type ExternalEvidence struct {
	Revision        string           `yaml:"revision"`
	Manifest        ExternalManifest `yaml:"manifest"`
	SourceInventory string           `yaml:"source_inventory"`
	Fixture         string           `yaml:"fixture"`
}

type ExternalManifest struct {
	Path   string `yaml:"path"`
	SHA256 string `yaml:"sha256"`
	Bytes  int64  `yaml:"bytes"`
}

type Validation struct {
	Job             string          `yaml:"job"`
	MetadataExample MetadataExample `yaml:"metadata_example"`
	Expected        ExpectedFacts   `yaml:"expected"`
}

type MetadataExample struct {
	IntegrationID string `yaml:"integration_id"`
	ExampleName   string `yaml:"example_name"`
	JobName       string `yaml:"job_name"`
}

type ExpectedFacts struct {
	Verdict          string `yaml:"verdict"`
	RawFamilies      int    `yaml:"raw_families"`
	RawLogicalSeries int    `yaml:"raw_logical_series"`
	WriterSeries     int    `yaml:"writer_series"`
	SeriesScanned    int    `yaml:"series_scanned"`
	SeriesAutogen    int    `yaml:"series_autogen"`
	SeriesUnmatched  int    `yaml:"series_unmatched"`
	AuthoredCharts   int    `yaml:"authored_charts"`
	RuntimeCharts    int    `yaml:"runtime_charts"`
	AutogenCharts    int    `yaml:"autogen_charts"`
	ChartDimensions  int    `yaml:"chart_dimensions"`
	PipelineExcluded int    `yaml:"pipeline_excluded"`
	PipelineRenamed  int    `yaml:"pipeline_renamed"`
}

type FileIntegrity struct {
	Path   string `yaml:"path"`
	SHA256 string `yaml:"sha256"`
	Bytes  int64  `yaml:"bytes"`
}

type Bundle struct {
	Path       string
	Descriptor Descriptor
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
		return strings.Compare(left.Descriptor.Profile.Name, right.Descriptor.Profile.Name)
	})

	seenNames := make(map[string]string, len(bundles))
	seenProfiles := make(map[string]string, len(bundles))
	for _, bundle := range bundles {
		name := bundle.Descriptor.Profile.Name
		if previous := seenNames[name]; previous != "" {
			return nil, fmt.Errorf("duplicate profile name %q in %q and %q", name, previous, bundle.Path)
		}
		seenNames[name] = bundle.Path
		profilePath := bundle.Descriptor.Profile.Path
		if previous := seenProfiles[profilePath]; previous != "" {
			return nil, fmt.Errorf("duplicate profile path %q in %q and %q", profilePath, previous, bundle.Path)
		}
		seenProfiles[profilePath] = bundle.Path
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
	if descriptor.Version != 1 {
		return fmt.Errorf("version: got %d, want 1", descriptor.Version)
	}
	proofDirectory := filepath.ToSlash(filepath.Dir(b.Path))
	if filepath.ToSlash(filepath.Dir(proofDirectory)) != ProofRoot {
		return fmt.Errorf("descriptor path %q is not directly below %q", b.Path, ProofRoot)
	}
	if filepath.Base(b.Path) != DescriptorFilename {
		return fmt.Errorf("descriptor path %q must end in %q", b.Path, DescriptorFilename)
	}
	if descriptor.Profile.Name == "" {
		return errors.New("profile.name must not be empty")
	}
	if descriptor.Profile.Name != filepath.Base(filepath.FromSlash(proofDirectory)) {
		return fmt.Errorf("profile.name %q must match proof directory %q", descriptor.Profile.Name, proofDirectory)
	}

	localPaths := []struct {
		field string
		path  string
	}{
		{"profile.path", descriptor.Profile.Path},
		{"proof.evidence", descriptor.Proof.Evidence},
		{"proof.operator_model", descriptor.Proof.OperatorModel},
		{"proof.validation_summary", descriptor.Proof.ValidationSummary},
		{"validation.job", descriptor.Validation.Job},
	}
	for _, candidate := range localPaths {
		if err := validateRelativePath(candidate.field, candidate.path); err != nil {
			return err
		}
	}
	for _, candidate := range localPaths[1:] {
		if filepath.ToSlash(filepath.Dir(filepath.FromSlash(candidate.path))) != proofDirectory {
			return fmt.Errorf("%s %q must be in proof directory %q", candidate.field, candidate.path, proofDirectory)
		}
	}
	for _, item := range []struct {
		field string
		got   string
		want  string
	}{
		{"proof.evidence", descriptor.Proof.Evidence, proofDirectory + "/EVIDENCE.md"},
		{"proof.operator_model", descriptor.Proof.OperatorModel, proofDirectory + "/OPERATOR-MODEL.md"},
		{"proof.validation_summary", descriptor.Proof.ValidationSummary, proofDirectory + "/VALIDATION.md"},
		{"validation.job", descriptor.Validation.Job, proofDirectory + "/VALIDATION-JOB.yaml"},
	} {
		if item.got != item.want {
			return fmt.Errorf("%s %q must be %q", item.field, item.got, item.want)
		}
	}
	if filepath.Base(filepath.FromSlash(descriptor.Profile.Path)) != descriptor.Profile.Name+".yaml" {
		return fmt.Errorf("profile.path %q must name %q", descriptor.Profile.Path, descriptor.Profile.Name+".yaml")
	}
	if filepath.ToSlash(filepath.Dir(filepath.FromSlash(descriptor.Profile.Path))) != StockProfileRoot {
		return fmt.Errorf("profile.path %q must be directly below %q", descriptor.Profile.Path, StockProfileRoot)
	}

	if descriptor.External.Revision == "" || strings.ContainsAny(descriptor.External.Revision, "/\\") {
		return fmt.Errorf("external_evidence.revision %q must be one non-empty path segment", descriptor.External.Revision)
	}
	externalRoot := "prometheus/profiles/" + descriptor.External.Revision
	if descriptor.External.Manifest.Path != externalRoot+"/manifest.yaml" {
		return fmt.Errorf("external_evidence.manifest.path %q must be %q",
			descriptor.External.Manifest.Path, externalRoot+"/manifest.yaml")
	}
	if descriptor.External.SourceInventory != externalRoot+"/SOURCE-INVENTORY.tsv" {
		return fmt.Errorf("external_evidence.source_inventory %q must be %q",
			descriptor.External.SourceInventory, externalRoot+"/SOURCE-INVENTORY.tsv")
	}
	if !strings.HasPrefix(descriptor.External.Fixture, externalRoot+"/fixtures/") ||
		!strings.HasSuffix(descriptor.External.Fixture, ".prom") {
		return fmt.Errorf("external_evidence.fixture %q must be a .prom file under %q",
			descriptor.External.Fixture, externalRoot+"/fixtures")
	}
	for _, item := range []struct {
		field string
		path  string
	}{
		{"external_evidence.manifest.path", descriptor.External.Manifest.Path},
		{"external_evidence.source_inventory", descriptor.External.SourceInventory},
		{"external_evidence.fixture", descriptor.External.Fixture},
	} {
		if err := validateRelativePath(item.field, item.path); err != nil {
			return err
		}
	}
	if err := validateDigest("external_evidence.manifest", descriptor.External.Manifest.SHA256, descriptor.External.Manifest.Bytes); err != nil {
		return err
	}

	metadata := descriptor.Validation.MetadataExample
	if metadata.IntegrationID == "" || metadata.ExampleName == "" || metadata.JobName == "" {
		return errors.New("validation.metadata_example fields must not be empty")
	}
	expected := descriptor.Validation.Expected
	if expected.Verdict != "PASS" {
		return fmt.Errorf("validation.expected.verdict %q must be PASS", expected.Verdict)
	}
	for _, item := range []struct {
		field string
		value int
	}{
		{"raw_families", expected.RawFamilies},
		{"raw_logical_series", expected.RawLogicalSeries},
		{"writer_series", expected.WriterSeries},
		{"series_scanned", expected.SeriesScanned},
		{"authored_charts", expected.AuthoredCharts},
		{"runtime_charts", expected.RuntimeCharts},
		{"chart_dimensions", expected.ChartDimensions},
	} {
		if item.value <= 0 {
			return fmt.Errorf("validation.expected.%s must be positive", item.field)
		}
	}
	for _, item := range []struct {
		field string
		value int
	}{
		{"series_autogen", expected.SeriesAutogen},
		{"series_unmatched", expected.SeriesUnmatched},
		{"autogen_charts", expected.AutogenCharts},
		{"pipeline_excluded", expected.PipelineExcluded},
		{"pipeline_renamed", expected.PipelineRenamed},
	} {
		if item.value < 0 {
			return fmt.Errorf("validation.expected.%s must be non-negative", item.field)
		}
	}

	wantIntegrityPaths := make([]string, 0, len(localPaths))
	for _, candidate := range localPaths {
		wantIntegrityPaths = append(wantIntegrityPaths, candidate.path)
	}
	slices.Sort(wantIntegrityPaths)
	gotIntegrityPaths := make([]string, 0, len(descriptor.Integrity))
	seenIntegrity := make(map[string]bool, len(descriptor.Integrity))
	for index, entry := range descriptor.Integrity {
		if err := validateRelativePath(fmt.Sprintf("integrity[%d].path", index), entry.Path); err != nil {
			return err
		}
		if seenIntegrity[entry.Path] {
			return fmt.Errorf("duplicate integrity path %q", entry.Path)
		}
		seenIntegrity[entry.Path] = true
		if err := validateDigest(fmt.Sprintf("integrity[%d]", index), entry.SHA256, entry.Bytes); err != nil {
			return err
		}
		gotIntegrityPaths = append(gotIntegrityPaths, entry.Path)
	}
	if !slices.IsSorted(gotIntegrityPaths) {
		return errors.New("integrity entries must be sorted by path")
	}
	if !slices.Equal(gotIntegrityPaths, wantIntegrityPaths) {
		return fmt.Errorf("integrity paths differ from descriptor inputs: got %v, want %v",
			gotIntegrityPaths, wantIntegrityPaths)
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
