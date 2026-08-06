// SPDX-License-Identifier: GPL-3.0-or-later

package promprofileproof

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDescriptorRoundTripAndIntegrity(t *testing.T) {
	repoRoot := t.TempDir()
	testdataRoot := t.TempDir()
	proofDirectory := filepath.Join(repoRoot, filepath.FromSlash(ProofRoot), "app")
	mustMkdirAll(t, proofDirectory)

	localContents := map[string]string{
		"src/go/plugin/go.d/collector/prometheus/profile-proofs/app/EVIDENCE.md":         "evidence\n",
		"src/go/plugin/go.d/collector/prometheus/profile-proofs/app/OPERATOR-MODEL.md":   "model\n",
		"src/go/plugin/go.d/collector/prometheus/profile-proofs/app/VALIDATION-JOB.yaml": "app: app\n",
		"src/go/plugin/go.d/collector/prometheus/profile-proofs/app/VALIDATION.md":       "validation\n",
		"src/go/plugin/go.d/config/go.d/prometheus.profiles/default/app.yaml":            "match: app_*\n",
	}
	for path, content := range localContents {
		mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(path)), content)
	}

	externalRoot := filepath.Join(testdataRoot, "prometheus", "profiles", "app")
	mustWriteFile(t, filepath.Join(externalRoot, "SOURCE-INVENTORY.tsv"), validInventory)
	mustWriteFile(t, filepath.Join(externalRoot, "fixtures", "app.prom"), "# TYPE app_value gauge\napp_value 1\n")
	inventoryDigest, inventoryBytes, err := digestFile(filepath.Join(externalRoot, "SOURCE-INVENTORY.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	fixtureDigest, fixtureBytes, err := digestFile(filepath.Join(externalRoot, "fixtures", "app.prom"))
	if err != nil {
		t.Fatal(err)
	}
	manifestContent, err := yaml.Marshal(evidenceManifest{
		Version:       1,
		Profile:       "app",
		EvidenceClass: "source-derived-synthetic",
		Sanitized:     true,
		Files: []evidenceManifestFile{
			{Path: "SOURCE-INVENTORY.tsv", Kind: "source_inventory", SHA256: inventoryDigest, Bytes: inventoryBytes},
			{Path: "fixtures/app.prom", Kind: "prometheus_exposition", SHA256: fixtureDigest, Bytes: fixtureBytes},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(externalRoot, "manifest.yaml"), string(manifestContent))

	bundle := Bundle{
		Path: ProofRoot + "/app/" + DescriptorFilename,
		Descriptor: Descriptor{
			Version: 2,
			Profile: "app",
			External: ExternalEvidence{
				Revision: "app",
				Manifest: FileDigest{SHA256: strings.Repeat("0", 64)},
			},
			Inventory: SourceInventoryExpected{
				Rows: 2, SourceFamilies: 2, AuthoredSelectors: 1,
				Dispositions: InventoryDisposition{Chart: 1, JobExcluded: 1},
			},
			Validation: Validation{
				MetadataExample: MetadataExample{
					IntegrationID: "collector-app", ExampleName: "App", JobName: "app",
				},
				Cases: []ValidationCase{{
					Name: "source-complete", Kind: "source_complete", Fixture: "fixtures/app.prom", Job: "validation",
					Expected: ExpectedResult{
						Verdict: "PASS",
						Counts: ExpectedCounts{
							RawFamilies: 1, RawLogicalSeries: 1, WriterSeries: 1,
							SeriesScanned: 1, AuthoredCharts: 1, RuntimeCharts: 1, ChartDimensions: 1,
						},
						Errors:   map[string]int{},
						Warnings: map[string]int{},
					},
				}},
			},
			Integrity: Integrity{
				Evidence:          FileDigest{SHA256: strings.Repeat("0", 64)},
				OperatorModel:     FileDigest{SHA256: strings.Repeat("0", 64)},
				ValidationJob:     FileDigest{SHA256: strings.Repeat("0", 64)},
				ValidationSummary: FileDigest{SHA256: strings.Repeat("0", 64)},
				Profile:           FileDigest{SHA256: strings.Repeat("0", 64)},
			},
		},
	}
	if err := Write(repoRoot, bundle); err != nil {
		t.Fatal(err)
	}
	refreshed, err := Refresh(repoRoot, testdataRoot, bundle)
	if err != nil {
		t.Fatal(err)
	}
	if err := Write(repoRoot, refreshed); err != nil {
		t.Fatal(err)
	}

	bundles, err := Discover(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundles) != 1 {
		t.Fatalf("discovered %d bundles, want 1", len(bundles))
	}
	if err := Verify(repoRoot, testdataRoot, bundles[0]); err != nil {
		t.Fatal(err)
	}
	if got := EvidenceDirectories(bundles); len(got) != 1 || got[0] != "prometheus/profiles/app" {
		t.Fatalf("evidence directories: got %v", got)
	}

	extraPath := filepath.Join(proofDirectory, "EXTRA.md")
	mustWriteFile(t, extraPath, "extra\n")
	if err := VerifyLocal(repoRoot, bundles[0]); err == nil || !strings.Contains(err.Error(), "differ from descriptor") {
		t.Fatalf("VerifyLocal with undeclared proof file: got %v", err)
	}
	if err := os.Remove(extraPath); err != nil {
		t.Fatal(err)
	}

	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(bundle.EvidencePath())), "changed!\n")
	if err := VerifyLocal(repoRoot, bundles[0]); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("VerifyLocal after mutation: got %v, want SHA-256 error", err)
	}
}

func TestDiscoverRejectsMissingAndUnknownDescriptors(t *testing.T) {
	t.Run("missing descriptor", func(t *testing.T) {
		repoRoot := t.TempDir()
		mustMkdirAll(t, filepath.Join(repoRoot, filepath.FromSlash(ProofRoot), "app"))
		if _, err := Discover(repoRoot); err == nil || !strings.Contains(err.Error(), DescriptorFilename) {
			t.Fatalf("Discover: got %v, want missing descriptor error", err)
		}
	})

	t.Run("unknown field", func(t *testing.T) {
		repoRoot := t.TempDir()
		path := filepath.Join(repoRoot, filepath.FromSlash(ProofRoot), "app", DescriptorFilename)
		mustWriteFile(t, path, "version: 1\nunknown: true\n")
		if _, err := Load(repoRoot, path); err == nil || !strings.Contains(err.Error(), "field unknown not found") {
			t.Fatalf("Load: got %v, want strict field error", err)
		}
	})
}

func TestLoadDescriptorV2NamedCasesAndInventory(t *testing.T) {
	repoRoot := t.TempDir()
	path := filepath.Join(repoRoot, filepath.FromSlash(ProofRoot), "app", DescriptorFilename)
	mustWriteFile(t, path, `version: 2
profile: app
external_evidence:
  revision: app
  manifest:
    sha256: 0000000000000000000000000000000000000000000000000000000000000000
    bytes: 0
source_inventory:
  rows: 1
  source_families: 1
  authored_selectors: 1
  dispositions:
    chart: 1
    job_excluded: 0
    writer_ineligible: 0
validation:
  metadata_example:
    integration_id: collector-app
    example_name: App
    job_name: app
  cases:
    - name: source-complete
      kind: source_complete
      fixture: fixtures/app.prom
      job: validation
      expected:
        verdict: PASS
        counts:
          raw_families: 1
          raw_logical_series: 1
          writer_series: 1
          series_scanned: 1
          series_autogen: 0
          series_unmatched: 0
          authored_charts: 1
          runtime_charts: 1
          autogen_charts: 0
          chart_dimensions: 1
          pipeline_excluded: 0
          pipeline_renamed: 0
          dead_charts: 0
          dead_dimensions: 0
          dimension_losses: 0
          instance_losses: 0
          collisions: 0
          chart_wire_collisions: 0
          context_wire_collisions: 0
          dimension_collisions: 0
        errors: {}
        warnings: {}
integrity:
  evidence: {sha256: 0000000000000000000000000000000000000000000000000000000000000000, bytes: 0}
  operator_model: {sha256: 0000000000000000000000000000000000000000000000000000000000000000, bytes: 0}
  validation_job: {sha256: 0000000000000000000000000000000000000000000000000000000000000000, bytes: 0}
  validation_summary: {sha256: 0000000000000000000000000000000000000000000000000000000000000000, bytes: 0}
  profile: {sha256: 0000000000000000000000000000000000000000000000000000000000000000, bytes: 0}
`)

	bundle, err := Load(repoRoot, path)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Descriptor.Version != 2 {
		t.Fatalf("version: got %d, want 2", bundle.Descriptor.Version)
	}
}

func TestValidateInventoryExpectedRejectsImpossibleCardinalities(t *testing.T) {
	tests := map[string]SourceInventoryExpected{
		"source families exceed rows": {
			Rows: 1, SourceFamilies: 2, AuthoredSelectors: 1,
			Dispositions: InventoryDisposition{Chart: 1},
		},
		"selectors exceed chart dispositions": {
			Rows: 2, SourceFamilies: 2, AuthoredSelectors: 2,
			Dispositions: InventoryDisposition{Chart: 1, JobExcluded: 1},
		},
	}
	for name, expected := range tests {
		t.Run(name, func(t *testing.T) {
			if err := validateInventoryExpected(expected); err == nil || !strings.Contains(err.Error(), "exceeds") {
				t.Fatalf("validateInventoryExpected: got %v, want impossible-cardinality error", err)
			}
		})
	}
}

func TestValidateExpectedResultRejectsInvalidFindingCounts(t *testing.T) {
	expected := ExpectedResult{
		Verdict: "PASS",
		Counts: ExpectedCounts{
			RawFamilies:    1,
			AuthoredCharts: 1,
		},
		Warnings: map[string]int{"invalid": 0},
	}
	if err := validateExpectedResult("expected", expected); err == nil || !strings.Contains(err.Error(), "positive counts") {
		t.Fatalf("validateExpectedResult: got %v, want invalid-finding-count error", err)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	mustMkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
