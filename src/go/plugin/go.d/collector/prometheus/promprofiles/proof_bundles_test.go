// SPDX-License-Identifier: GPL-3.0-or-later

package promprofiles

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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
				"src/go/plugin/go.d/collector/prometheus/metadata.yaml",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/ceph/EVIDENCE.md",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/ceph/OPERATOR-MODEL.md",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/ceph/SOURCE-INVENTORY.tsv",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/ceph/VALIDATION-JOB.yaml",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/ceph/VALIDATION.md",
				"src/go/plugin/go.d/collector/prometheus/testdata/ceph_all_metrics.prom",
				"src/go/plugin/go.d/collector/prometheus/testdata/ceph_nvmeof_all_metrics.prom",
				"src/go/plugin/go.d/collector/prometheus/testdata/ceph_reef_exporter_prio0_all_metrics.prom",
				"src/go/plugin/go.d/collector/prometheus/testdata/ceph_reef_mgr_perf_all_metrics.prom",
				"src/go/plugin/go.d/collector/prometheus/testdata/ceph_squid_exporter_prio0_all_metrics.prom",
				"src/go/plugin/go.d/collector/prometheus/testdata/ceph_squid_mgr_perf_all_metrics.prom",
				"src/go/plugin/go.d/collector/prometheus/testdata/ceph_tentacle_exporter_prio0_all_metrics.prom",
				"src/go/plugin/go.d/collector/prometheus/testdata/ceph_tentacle_mgr_perf_all_metrics.prom",
				"src/go/plugin/go.d/config/go.d/prometheus.profiles/default/ceph.yaml",
			},
		},
		"litellm": {
			directory:     "litellm",
			integrationID: "collector-go.d.plugin-prometheus-litellm",
			exampleName:   "LiteLLM",
			jobName:       "litellm",
			inputs: []string{
				"src/go/plugin/go.d/collector/prometheus/metadata.yaml",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/litellm/EVIDENCE.md",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/litellm/OPERATOR-MODEL.md",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/litellm/SOURCE-INVENTORY.tsv",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/litellm/VALIDATION-JOB.yaml",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/litellm/VALIDATION.md",
				"src/go/plugin/go.d/collector/prometheus/testdata/litellm_all_metrics.prom",
				"src/go/plugin/go.d/config/go.d/prometheus.profiles/default/litellm.yaml",
			},
		},
		"vllm": {
			directory:     "vllm",
			integrationID: "collector-go.d.plugin-prometheus-vllm",
			exampleName:   "Native vLLM",
			jobName:       "vllm",
			inputs: []string{
				"src/go/plugin/go.d/collector/prometheus/metadata.yaml",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/vllm/EVIDENCE.md",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/vllm/OPERATOR-MODEL.md",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/vllm/SOURCE-INVENTORY.tsv",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/vllm/VALIDATION-JOB.yaml",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/vllm/VALIDATION.md",
				"src/go/plugin/go.d/collector/prometheus/testdata/vllm_all_metrics.prom",
				"src/go/plugin/go.d/config/go.d/prometheus.profiles/default/vllm.yaml",
			},
		},
		"vllm_ray": {
			directory:     "vllm_ray",
			integrationID: "collector-go.d.plugin-prometheus-vllm",
			exampleName:   "vLLM on Ray",
			jobName:       "vllm-ray",
			inputs: []string{
				"src/go/plugin/go.d/collector/prometheus/metadata.yaml",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/vllm_ray/EVIDENCE.md",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/vllm_ray/OPERATOR-MODEL.md",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/vllm_ray/SOURCE-INVENTORY.tsv",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/vllm_ray/VALIDATION-JOB.yaml",
				"src/go/plugin/go.d/collector/prometheus/profile-proofs/vllm_ray/VALIDATION.md",
				"src/go/plugin/go.d/collector/prometheus/testdata/vllm_ray_all_metrics.prom",
				"src/go/plugin/go.d/config/go.d/prometheus.profiles/default/vllm_ray.yaml",
			},
		},
	}

	metadataPath := filepath.Join(repoRoot, "src/go/plugin/go.d/collector/prometheus/metadata.yaml")
	for name, proof := range proofs {
		t.Run(name, func(t *testing.T) {
			slices.Sort(proof.inputs)
			manifestPath := filepath.Join(repoRoot, "src/go/plugin/go.d/collector/prometheus/profile-proofs",
				proof.directory, "SHA256SUMS.tsv")
			require.Equal(t, proof.inputs, verifyProofManifest(t, repoRoot, manifestPath))

			jobPath := filepath.Join(repoRoot, "src/go/plugin/go.d/collector/prometheus/profile-proofs",
				proof.directory, "VALIDATION-JOB.yaml")
			assertProofJobMatchesMetadata(t, metadataPath, jobPath, proof)
		})
	}
}

func verifyProofManifest(t *testing.T, repoRoot, manifestPath string) []string {
	t.Helper()

	file, err := os.Open(manifestPath)
	require.NoError(t, err)
	defer func() { require.NoError(t, file.Close()) }()

	var paths []string
	seen := make(map[string]bool)
	scanner := bufio.NewScanner(file)
	require.True(t, scanner.Scan(), "proof manifest must have a header")
	require.Equal(t, "sha256\tbytes\tpath", scanner.Text())
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		require.Len(t, fields, 3, "invalid proof manifest row %q", scanner.Text())
		expectedBytes, err := strconv.ParseInt(fields[1], 10, 64)
		require.NoErrorf(t, err, "invalid byte count for %q", fields[2])
		require.Falsef(t, seen[fields[2]], "duplicate proof manifest path %q", fields[2])
		seen[fields[2]] = true

		path := filepath.Clean(filepath.Join(repoRoot, fields[2]))
		rel, err := filepath.Rel(repoRoot, path)
		require.NoError(t, err)
		require.Falsef(t, rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)),
			"proof manifest path %q escapes the repository", fields[2])
		content, err := os.ReadFile(path)
		require.NoErrorf(t, err, "read proof input %q", fields[2])
		require.Equalf(t, expectedBytes, int64(len(content)), "proof input %q byte count", fields[2])
		require.Equalf(t, fields[0], fmt.Sprintf("%x", sha256.Sum256(content)), "proof input %q digest", fields[2])
		paths = append(paths, filepath.ToSlash(rel))
	}
	require.NoError(t, scanner.Err())
	slices.Sort(paths)
	return paths
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
