// SPDX-License-Identifier: GPL-3.0-or-later

package promprofiles

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/netdata/netdata/go/plugins/internal/promtestdata"
)

type stockProfileProof struct {
	directory     string
	integrationID string
	exampleName   string
	jobName       string
	inputs        []string
}

func TestDefaultCatalog_StockProfileProofBundles(t *testing.T) {
	repoRoot, err := filepath.Abs("../../../../../../..")
	require.NoError(t, err)

	proofs := map[string]stockProfileProof{
		"ceph": {
			directory:     "ceph",
			integrationID: "collector-go.d.plugin-prometheus-ceph",
			exampleName:   "Ceph MGR and ceph-exporter",
			jobName:       "ceph-mgr",
			inputs: []string{
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/ceph/EVIDENCE.md",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/ceph/OPERATOR-MODEL.md",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/ceph/VALIDATION-JOB.yaml",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/ceph/VALIDATION.md",
				"src/go/plugin/go.d/config/go.d/prometheus.profiles/default/ceph.yaml",
				"src/go/testdata/prometheus/profiles/ceph/manifest.yaml",
			},
		},
		"litellm": {
			directory:     "litellm",
			integrationID: "collector-go.d.plugin-prometheus-litellm",
			exampleName:   "LiteLLM",
			jobName:       "litellm",
			inputs: []string{
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/litellm/EVIDENCE.md",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/litellm/OPERATOR-MODEL.md",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/litellm/VALIDATION-JOB.yaml",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/litellm/VALIDATION.md",
				"src/go/plugin/go.d/config/go.d/prometheus.profiles/default/litellm.yaml",
				"src/go/testdata/prometheus/profiles/litellm/manifest.yaml",
			},
		},
		"vllm": {
			directory:     "vllm",
			integrationID: "collector-go.d.plugin-prometheus-vllm",
			exampleName:   "Native vLLM",
			jobName:       "vllm",
			inputs: []string{
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/vllm/EVIDENCE.md",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/vllm/OPERATOR-MODEL.md",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/vllm/VALIDATION-JOB.yaml",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/vllm/VALIDATION.md",
				"src/go/plugin/go.d/config/go.d/prometheus.profiles/default/vllm.yaml",
				"src/go/testdata/prometheus/profiles/vllm/manifest.yaml",
			},
		},
		"vllm_ray": {
			directory:     "vllm_ray",
			integrationID: "collector-go.d.plugin-prometheus-vllm",
			exampleName:   "vLLM on Ray",
			jobName:       "vllm-ray",
			inputs: []string{
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/vllm_ray/EVIDENCE.md",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/vllm_ray/OPERATOR-MODEL.md",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/vllm_ray/VALIDATION-JOB.yaml",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/vllm_ray/VALIDATION.md",
				"src/go/plugin/go.d/config/go.d/prometheus.profiles/default/vllm_ray.yaml",
				"src/go/testdata/prometheus/profiles/vllm_ray/manifest.yaml",
			},
		},
	}

	metadataPath := filepath.Join(repoRoot, "src/go/plugin/go.d/collector/prometheus/metadata.yaml")
	for name, proof := range proofs {
		t.Run(name, func(t *testing.T) {
			slices.Sort(proof.inputs)
			manifestPath := filepath.Join(repoRoot, "src/go/plugin/go.d/collector/prometheus/profile-proofs",
				proof.directory, "SHA256SUMS.tsv")
			entries := readProofManifest(t, repoRoot, manifestPath)
			require.Equal(t, proof.inputs, proofManifestPaths(entries))
			verifyLocalProofInputs(t, repoRoot, entries)

			jobPath := filepath.Join(repoRoot, "src/go/plugin/go.d/collector/prometheus/profile-proofs",
				proof.directory, "VALIDATION-JOB.yaml")
			assertProofJobMatchesMetadata(t, metadataPath, jobPath, proof)

			t.Run("external evidence", func(t *testing.T) {
				relativePath := filepath.ToSlash(filepath.Join("prometheus/profiles", proof.directory, "manifest.yaml"))
				externalManifestPath := promtestdata.Require(t, relativePath)
				verifyExternalManifestPin(t, entries, externalManifestPath, relativePath)
				verifyExternalEvidenceManifest(t, externalManifestPath, proof.directory)
			})
		})
	}
}

type proofManifestEntry struct {
	sha256 string
	bytes  int64
	path   string
}

func readProofManifest(t *testing.T, repoRoot, manifestPath string) []proofManifestEntry {
	t.Helper()

	file, err := os.Open(manifestPath)
	require.NoError(t, err)
	defer func() { require.NoError(t, file.Close()) }()

	var entries []proofManifestEntry
	seen := make(map[string]bool)
	scanner := bufio.NewScanner(file)
	require.True(t, scanner.Scan(), "proof manifest must have a header")
	require.Equal(t, "sha256\tbytes\tpath", scanner.Text())
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		require.Len(t, fields, 3, "invalid proof manifest row %q", scanner.Text())
		expectedBytes, err := strconv.ParseInt(fields[1], 10, 64)
		require.NoErrorf(t, err, "invalid byte count for %q", fields[2])
		require.GreaterOrEqualf(t, expectedBytes, int64(0), "invalid byte count for %q", fields[2])
		digest, err := hex.DecodeString(fields[0])
		require.NoErrorf(t, err, "invalid SHA-256 for %q", fields[2])
		require.Lenf(t, digest, sha256.Size, "invalid SHA-256 for %q", fields[2])
		require.Equalf(t, strings.ToLower(fields[0]), fields[0], "SHA-256 must be lowercase for %q", fields[2])
		require.Falsef(t, seen[fields[2]], "duplicate proof manifest path %q", fields[2])
		seen[fields[2]] = true

		rawPath := filepath.FromSlash(fields[2])
		require.NotEmpty(t, fields[2], "proof manifest path must not be empty")
		require.Falsef(t, filepath.IsAbs(rawPath), "proof manifest path %q must be relative", fields[2])
		require.Equalf(t, fields[2], filepath.ToSlash(filepath.Clean(rawPath)),
			"proof manifest path %q must use canonical slash form", fields[2])
		path := filepath.Clean(filepath.Join(repoRoot, rawPath))
		rel, err := filepath.Rel(repoRoot, path)
		require.NoError(t, err)
		require.Falsef(t, rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)),
			"proof manifest path %q escapes the repository", fields[2])
		entries = append(entries, proofManifestEntry{sha256: fields[0], bytes: expectedBytes, path: filepath.ToSlash(rel)})
	}
	require.NoError(t, scanner.Err())
	return entries
}

func proofManifestPaths(entries []proofManifestEntry) []string {
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, entry.path)
	}
	slices.Sort(paths)
	return paths
}

func verifyLocalProofInputs(t *testing.T, repoRoot string, entries []proofManifestEntry) {
	t.Helper()

	for _, entry := range entries {
		if strings.HasPrefix(entry.path, "src/go/testdata/") {
			continue
		}
		verifyProofInput(t, filepath.Join(repoRoot, filepath.FromSlash(entry.path)), entry)
	}
}

func verifyProofInput(t *testing.T, path string, entry proofManifestEntry) {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoErrorf(t, err, "read proof input %q", entry.path)
	require.Equalf(t, entry.bytes, int64(len(content)), "proof input %q byte count", entry.path)
	require.Equalf(t, entry.sha256, fmt.Sprintf("%x", sha256.Sum256(content)), "proof input %q digest", entry.path)
}

func verifyExternalManifestPin(t *testing.T, entries []proofManifestEntry, manifestPath, relativePath string) {
	t.Helper()

	wantPath := "src/go/testdata/" + relativePath
	for _, entry := range entries {
		if entry.path == wantPath {
			verifyProofInput(t, manifestPath, entry)
			return
		}
	}
	t.Fatalf("proof manifest does not pin external evidence manifest %q", wantPath)
}

type externalEvidenceManifest struct {
	Version       int    `yaml:"version"`
	Profile       string `yaml:"profile"`
	EvidenceClass string `yaml:"evidence_class"`
	Sanitized     bool   `yaml:"sanitized"`
	Files         []struct {
		Path   string `yaml:"path"`
		Kind   string `yaml:"kind"`
		SHA256 string `yaml:"sha256"`
		Bytes  int64  `yaml:"bytes"`
	} `yaml:"files"`
}

func verifyExternalEvidenceManifest(t *testing.T, manifestPath, profileName string) {
	t.Helper()

	content, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	var manifest externalEvidenceManifest
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	require.NoError(t, decoder.Decode(&manifest))
	var extra any
	require.ErrorIs(t, decoder.Decode(&extra), io.EOF, "external evidence manifest must contain one YAML document")

	require.Equal(t, 1, manifest.Version)
	require.Equal(t, profileName, manifest.Profile)
	require.Equal(t, "source-derived-synthetic", manifest.EvidenceClass)
	require.True(t, manifest.Sanitized)
	require.NotEmpty(t, manifest.Files)

	root := filepath.Dir(manifestPath)
	seen := make(map[string]bool)
	var inventories, fixtures int
	for _, file := range manifest.Files {
		clean := filepath.Clean(filepath.FromSlash(file.Path))
		require.Falsef(t, clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)),
			"external evidence path %q escapes its profile directory", file.Path)
		require.Equalf(t, file.Path, filepath.ToSlash(clean),
			"external evidence path %q must use canonical slash form", file.Path)
		require.Falsef(t, seen[file.Path], "duplicate external evidence path %q", file.Path)
		seen[file.Path] = true

		switch file.Kind {
		case "source_inventory":
			require.Equal(t, "SOURCE-INVENTORY.tsv", filepath.ToSlash(clean))
			inventories++
		case "prometheus_exposition":
			require.Truef(t, strings.HasPrefix(filepath.ToSlash(clean), "fixtures/") && strings.HasSuffix(clean, ".prom"),
				"Prometheus exposition path %q must be a fixtures/*.prom file", file.Path)
			fixtures++
		default:
			t.Fatalf("unsupported external evidence kind %q for %q", file.Kind, file.Path)
		}

		entry := proofManifestEntry{sha256: file.SHA256, bytes: file.Bytes, path: file.Path}
		decoded, err := hex.DecodeString(entry.sha256)
		require.NoErrorf(t, err, "invalid SHA-256 for external evidence %q", file.Path)
		require.Lenf(t, decoded, sha256.Size, "invalid SHA-256 for external evidence %q", file.Path)
		require.Equalf(t, strings.ToLower(entry.sha256), entry.sha256,
			"SHA-256 must be lowercase for external evidence %q", file.Path)
		require.GreaterOrEqualf(t, entry.bytes, int64(0), "invalid byte count for external evidence %q", file.Path)
		verifyProofInput(t, filepath.Join(root, clean), entry)
	}
	require.Equal(t, 1, inventories, "external evidence manifest must contain one source inventory")
	require.Positive(t, fixtures, "external evidence manifest must contain at least one Prometheus fixture")
}

func assertProofJobMatchesMetadata(t *testing.T, metadataPath, jobPath string, proof stockProfileProof) {
	t.Helper()

	type example struct {
		Name   string `yaml:"name"`
		Config string `yaml:"config"`
	}
	type module struct {
		Meta struct {
			ID string `yaml:"id"`
		} `yaml:"meta"`
		Setup struct {
			Configuration struct {
				Examples struct {
					List []example `yaml:"list"`
				} `yaml:"examples"`
			} `yaml:"configuration"`
		} `yaml:"setup"`
	}
	var metadata struct {
		Modules []module `yaml:"modules"`
	}
	content, err := os.ReadFile(metadataPath)
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(content, &metadata))

	var config string
	for _, mod := range metadata.Modules {
		if mod.Meta.ID != proof.integrationID {
			continue
		}
		for _, example := range mod.Setup.Configuration.Examples.List {
			if example.Name == proof.exampleName {
				config = example.Config
			}
		}
	}
	require.NotEmptyf(t, config, "metadata example %q for %q", proof.exampleName, proof.integrationID)

	var exampleConfig struct {
		Jobs []map[string]any `yaml:"jobs"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(config), &exampleConfig))
	var metadataJob map[string]any
	for _, job := range exampleConfig.Jobs {
		if job["name"] == proof.jobName {
			metadataJob = job
		}
	}
	require.NotNilf(t, metadataJob, "metadata job %q in example %q", proof.jobName, proof.exampleName)
	delete(metadataJob, "url")
	delete(metadataJob, "profiles")

	var validationJob map[string]any
	content, err = os.ReadFile(jobPath)
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(content, &validationJob))
	require.Equal(t, metadataJob, validationJob,
		"proof validation job must match metadata after removing endpoint and profile-selection fields")
}
