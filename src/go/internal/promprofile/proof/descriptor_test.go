// SPDX-License-Identifier: GPL-3.0-or-later

package promproof

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/profilecatalog"
)

func TestLoadDescriptor(t *testing.T) {
	repoRoot := t.TempDir()
	path := filepath.Join(repoRoot, filepath.FromSlash(ProofRoot), "app", DescriptorFilename)
	mustWriteFile(t, path, validDescriptor)

	bundle, err := Load(repoRoot, path)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Descriptor.Profile != "app" || len(bundle.Descriptor.Cases) != 2 {
		t.Fatalf("Load() = %#v", bundle)
	}
	if got := bundle.Descriptor.Cases["lifecycle"].Steps[1].Observations["requests#total"].Predicates.Membership; got != "removed" {
		t.Fatalf("membership = %q, want removed", got)
	}
}

func TestLoadDescriptorUsesProfileCatalogNameConstraint(t *testing.T) {
	for _, profile := range []string{"app", "app_1", "App", "1app", "app-name"} {
		t.Run(profile, func(t *testing.T) {
			repoRoot := t.TempDir()
			content := strings.Replace(validDescriptor, "profile: app", "profile: "+profile, 1)
			path := filepath.Join(repoRoot, filepath.FromSlash(ProofRoot), profile, DescriptorFilename)
			mustWriteFile(t, path, content)

			_, err := Load(repoRoot, path)
			if profilecatalog.DefaultValidName(profile) && err != nil {
				t.Fatalf("Load() rejected catalog-valid profile %q: %v", profile, err)
			}
			if !profilecatalog.DefaultValidName(profile) && err == nil {
				t.Fatalf("Load() accepted catalog-invalid profile %q", profile)
			}
		})
	}
}

func TestDiscoverLoadsEveryStrictBundle(t *testing.T) {
	repoRoot := t.TempDir()
	for _, profile := range []string{"app", "runtime"} {
		content := strings.Replace(validDescriptor, "profile: app", "profile: "+profile, 1)
		path := filepath.Join(repoRoot, filepath.FromSlash(ProofRoot), profile, DescriptorFilename)
		mustWriteFile(t, path, content)
	}
	bundles, err := Discover(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundles) != 2 || bundles[0].Descriptor.Profile != "app" || bundles[1].Descriptor.Profile != "runtime" {
		t.Fatalf("Discover() = %#v", bundles)
	}
}

func TestDiscoverRejectsUnsupportedDescriptorVersion(t *testing.T) {
	repoRoot := t.TempDir()
	mustWriteFile(t,
		filepath.Join(repoRoot, filepath.FromSlash(ProofRoot), "unsupported", DescriptorFilename),
		strings.Replace(
			strings.Replace(validDescriptor, "version: v1", "version: v2", 1),
			"profile: app", "profile: unsupported", 1,
		))

	_, err := Discover(repoRoot)
	if err == nil || !strings.Contains(err.Error(), `version: got "v2", want "v1"`) {
		t.Fatalf("Discover() error = %v, want unsupported-version rejection", err)
	}
}

func TestLoadCompiledCatalogRejectsUnexpectedArtifacts(t *testing.T) {
	tests := map[string]struct {
		path    func(repoRoot, testdataRoot string) string
		wantErr string
	}{
		"retired local artifact": {
			path: func(repoRoot, _ string) string {
				return filepath.Join(repoRoot, filepath.FromSlash(ProofRoot), "app", "VALIDATION.md")
			},
			wantErr: "unexpected local proof artifact",
		},
		"retired external artifact": {
			path: func(_, testdataRoot string) string {
				return filepath.Join(testdataRoot, "prometheus", "profiles", "app", "SOURCE-INVENTORY.tsv")
			},
			wantErr: "unexpected external proof artifact",
		},
		"unreferenced fixture": {
			path: func(_, testdataRoot string) string {
				return filepath.Join(testdataRoot, "prometheus", "profiles", "app", "fixtures", "unused.prom")
			},
			wantErr: "unexpected external proof artifact",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			repoRoot, testdataRoot, bundles := writeCompleteTestBundle(t)
			mustWriteFile(t, tc.path(repoRoot, testdataRoot), "retired\n")
			_, err := LoadCompiledCatalog(context.Background(), repoRoot, testdataRoot, bundles)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("LoadCompiledCatalog() error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func writeCompleteTestBundle(t *testing.T) (string, string, []Bundle) {
	t.Helper()
	repoRoot := t.TempDir()
	testdataRoot := t.TempDir()
	proofDirectory := filepath.Join(repoRoot, filepath.FromSlash(ProofRoot), "app")
	externalDirectory := filepath.Join(testdataRoot, "prometheus", "profiles", "app")
	mustWriteFile(t, filepath.Join(proofDirectory, DescriptorFilename), replayDescriptor)
	mustWriteFile(t, filepath.Join(proofDirectory, "PROFILE-DESIGN.yaml"), replayProfileDesign)
	mustWriteFile(t, filepath.Join(proofDirectory, "OPERATOR-MODEL.md"), "# Operator model\n")
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(StockProfileRoot), "app.yaml"), "match: app_*\n")
	mustWriteFile(t, filepath.Join(externalDirectory, "SOURCE-SEMANTICS.yaml"), replaySourceSemantics)
	mustWriteFile(t, filepath.Join(externalDirectory, "fixtures", "present.prom"), "present\n")
	mustWriteFile(t, filepath.Join(externalDirectory, "fixtures", "absent.prom"), "absent\n")
	bundles, err := Discover(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	return repoRoot, testdataRoot, bundles
}

func TestLoadCompiledCatalogRequiresJoinedArtifactsAndRegistryPair(t *testing.T) {
	repoRoot := t.TempDir()
	testdataRoot := t.TempDir()
	path := filepath.Join(repoRoot, filepath.FromSlash(ProofRoot), "app", DescriptorFilename)
	mustWriteFile(t, path, validDescriptor)
	bundle, err := Load(repoRoot, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCompiledCatalog(context.Background(), repoRoot, testdataRoot, []Bundle{bundle}); err == nil || !strings.Contains(err.Error(), "OPERATOR-MODEL.md") {
		t.Fatalf("LoadCompiledCatalog() error = %v, want missing operator model", err)
	}

	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(bundle.OperatorModelPath())), "# Model\n")
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(bundle.ProfileDesignPath())), replayProfileDesign)
	mustWriteFile(t, filepath.Join(testdataRoot, filepath.FromSlash(bundle.SourceSemanticsPath())), replaySourceSemantics)
	mustWriteFile(t, filepath.Join(testdataRoot, filepath.FromSlash(bundle.FixturePath("fixtures/current.prom"))), "current\n")
	mustWriteFile(t, filepath.Join(testdataRoot, filepath.FromSlash(bundle.FixturePath("fixtures/present.prom"))), "present\n")
	mustWriteFile(t, filepath.Join(testdataRoot, filepath.FromSlash(bundle.FixturePath("fixtures/absent.prom"))), "absent\n")
	mustWriteFile(t, filepath.Join(testdataRoot, filepath.FromSlash(bundle.SourceRegistryPath())), "version: v1\n")
	if _, err := LoadCompiledCatalog(context.Background(), repoRoot, testdataRoot, []Bundle{bundle}); err == nil || !strings.Contains(err.Error(), "must be present together") {
		t.Fatalf("LoadCompiledCatalog() error = %v, want registry-pair failure", err)
	}
}

func TestLoadDescriptorRejectsStrictAndSemanticShapeErrors(t *testing.T) {
	tests := map[string]struct {
		mutate  func(string) string
		wantErr string
	}{
		"unknown nested field": {
			mutate: func(value string) string {
				return strings.Replace(value, "coverage: true", "coverage: true\n    typo: rejected", 1)
			},
			wantErr: "field typo not found",
		},
		"missing environment": {
			mutate: func(value string) string {
				return strings.Replace(value, "    environment:\n      app: {mode: single}\n", "", 1)
			},
			wantErr: "environment must be present",
		},
		"fixture and steps": {
			mutate: func(value string) string {
				return strings.Replace(value, "    steps:\n", "    fixture: fixtures/current.prom\n    steps:\n", 1)
			},
			wantErr: "exactly one of fixture or steps",
		},
		"pass findings": {
			mutate: func(value string) string {
				return strings.Replace(value, "expected: {verdict: PASS}", "expected: {verdict: PASS, findings: [unexpected]}", 1)
			},
			wantErr: "PASS result must not declare findings",
		},
		"fail without findings": {
			mutate: func(value string) string {
				value = strings.Replace(value, "coverage: true", "coverage: false", 1)
				return strings.Replace(value, "expected: {verdict: PASS}", "expected: {verdict: FAIL}", 1)
			},
			wantErr: "FAIL result must declare findings",
		},
		"failed fixture contributes coverage": {
			mutate: func(value string) string {
				return strings.Replace(value, "expected: {verdict: PASS}",
					"expected: {verdict: FAIL, findings: [source_mismatch]}", 1)
			},
			wantErr: "FAIL result must set coverage: false",
		},
		"lifecycle step expects failure": {
			mutate: func(value string) string {
				return strings.Replace(value, "        expected: {verdict: PASS}",
					"        expected: {verdict: FAIL, findings: [source_mismatch]}", 1)
			},
			wantErr: "lifecycle step must expect PASS",
		},
		"arbitrary job": {
			mutate: func(value string) string {
				return strings.Replace(value, "job: minimal", "job: validation", 1)
			},
			wantErr: "job must be minimal",
		},
		"invalid observation target": {
			mutate: func(value string) string {
				return strings.Replace(value, "requests#total:", "requests:", 1)
			},
			wantErr: "must be <view>#<input>",
		},
		"noncanonical fixture path": {
			mutate: func(value string) string {
				return strings.Replace(value, "fixtures/current.prom", `fixtures/dir\current.prom`, 1)
			},
			wantErr: "must use canonical slash form",
		},
		"implicit future type": {
			mutate: func(value string) string {
				return strings.Replace(value, "    type: counter\n", "", 1)
			},
			wantErr: "type must be explicit",
		},
		"case input duplicates profile input": {
			mutate: func(value string) string {
				return strings.Replace(value, "    job: minimal\n", `    job: minimal
    future_inputs:
      future_namespace:
        name: app_other_future_total
        type: counter
`, 1)
			},
			wantErr: "duplicates profile-level input",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			repoRoot := t.TempDir()
			path := filepath.Join(repoRoot, filepath.FromSlash(ProofRoot), "app", DescriptorFilename)
			mustWriteFile(t, path, tc.mutate(validDescriptor))
			_, err := Load(repoRoot, path)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Load() error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

const validDescriptor = `version: v1
profile: app
metadata_example:
  integration_id: collector-go.d.plugin-prometheus-app
  example_name: app
  job_name: local
future_inputs:
  future_namespace:
    name: app_future_total
    type: counter
    labels: {mode: future}
cases:
  default:
    environment:
      app: {mode: single}
    fixture: fixtures/current.prom
    coverage: true
    expected: {verdict: PASS}
    job: minimal
  lifecycle:
    environment:
      app: {mode: multiprocess}
    coverage: true
    steps:
      - fixture: fixtures/present.prom
        expected: {verdict: PASS}
        observations:
          requests#total:
            state: current
            predicates:
              membership: establish
              aggregate: matches_reducer
              identity: establish
      - fixture: fixtures/absent.prom
        expected: {verdict: PASS}
        observations:
          requests#total:
            state: stale
            predicates:
              membership: removed
              aggregate: decreased
              identity: unchanged
            limitation: requests#total
`
