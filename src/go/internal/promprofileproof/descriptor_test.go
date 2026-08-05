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
	mustWriteFile(t, filepath.Join(externalRoot, "SOURCE-INVENTORY.tsv"), "source\n")
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

	paths := []string{
		"src/go/plugin/go.d/collector/prometheus/profile-proofs/app/EVIDENCE.md",
		"src/go/plugin/go.d/collector/prometheus/profile-proofs/app/OPERATOR-MODEL.md",
		"src/go/plugin/go.d/collector/prometheus/profile-proofs/app/VALIDATION-JOB.yaml",
		"src/go/plugin/go.d/collector/prometheus/profile-proofs/app/VALIDATION.md",
		"src/go/plugin/go.d/config/go.d/prometheus.profiles/default/app.yaml",
	}
	integrity := make([]FileIntegrity, 0, len(paths))
	for _, path := range paths {
		integrity = append(integrity, FileIntegrity{Path: path, SHA256: strings.Repeat("0", 64)})
	}
	bundle := Bundle{
		Path: ProofRoot + "/app/" + DescriptorFilename,
		Descriptor: Descriptor{
			Version: 1,
			Profile: Profile{Name: "app", Path: paths[4]},
			Proof: ProofDocuments{
				Evidence:          paths[0],
				OperatorModel:     paths[1],
				ValidationSummary: paths[3],
			},
			External: ExternalEvidence{
				Revision: "app",
				Manifest: ExternalManifest{
					Path: "prometheus/profiles/app/manifest.yaml", SHA256: strings.Repeat("0", 64),
				},
				SourceInventory: "prometheus/profiles/app/SOURCE-INVENTORY.tsv",
				Fixture:         "prometheus/profiles/app/fixtures/app.prom",
			},
			Validation: Validation{
				Job: paths[2],
				MetadataExample: MetadataExample{
					IntegrationID: "collector-app", ExampleName: "App", JobName: "app",
				},
				Expected: ExpectedFacts{
					Verdict:     "PASS",
					RawFamilies: 1, RawLogicalSeries: 1, WriterSeries: 1,
					SeriesScanned: 1, AuthoredCharts: 1, RuntimeCharts: 1, ChartDimensions: 1,
				},
			},
			Integrity: integrity,
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

	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(paths[0])), "changed!\n")
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
