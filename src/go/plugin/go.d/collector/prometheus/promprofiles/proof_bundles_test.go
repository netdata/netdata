// SPDX-License-Identifier: GPL-3.0-or-later

package promprofiles

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/netdata/netdata/go/plugins/internal/promprofileproof"
	"github.com/netdata/netdata/go/plugins/internal/promtestdata"
)

func TestDefaultCatalog_StockProfileProofBundles(t *testing.T) {
	repoRoot, err := filepath.Abs("../../../../../../..")
	require.NoError(t, err)
	bundles, err := promprofileproof.Discover(repoRoot)
	require.NoError(t, err)

	metadataPath := filepath.Join(repoRoot, "src/go/plugin/go.d/collector/prometheus/metadata.yaml")
	for _, bundle := range bundles {
		t.Run(bundle.Descriptor.Profile, func(t *testing.T) {
			require.NoError(t, promprofileproof.VerifyLocal(repoRoot, bundle))
			assertProofInputLFAttributes(t, repoRoot, bundle)
			assertProofJobMatchesMetadata(t, repoRoot, metadataPath, bundle)

			t.Run("external evidence", func(t *testing.T) {
				manifestPath := promtestdata.Require(t, bundle.ExternalManifestPath())
				testdataRoot := resolvedRoot(manifestPath, bundle.ExternalManifestPath())
				require.NoError(t, promprofileproof.VerifyExternal(testdataRoot, bundle))
			})
		})
	}
}

func resolvedRoot(resolvedPath, relativePath string) string {
	root := resolvedPath
	for range strings.Split(filepath.ToSlash(relativePath), "/") {
		root = filepath.Dir(root)
	}
	return root
}

func assertProofInputLFAttributes(t *testing.T, repoRoot string, bundle promprofileproof.Bundle) {
	t.Helper()

	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Log("git is unavailable; skipping effective EOL attribute assertion")
		return
	}
	worktree, err := exec.Command(gitPath, "-C", repoRoot, "rev-parse", "--is-inside-work-tree").Output()
	if err != nil || strings.TrimSpace(string(worktree)) != "true" {
		t.Log("Git worktree metadata is unavailable; skipping effective EOL attribute assertion")
		return
	}

	paths := []string{".gitattributes", bundle.Path}
	for _, entry := range bundle.IntegrityEntries() {
		paths = append(paths, entry.Path)
	}
	args := []string{"-C", repoRoot, "check-attr", "-z", "eol", "--"}
	output, err := exec.Command(gitPath, append(args, paths...)...).Output()
	require.NoError(t, err)
	fields := bytes.Split(output, []byte{0})
	require.NotEmpty(t, fields)
	require.Empty(t, fields[len(fields)-1], "git check-attr output must end with NUL")
	fields = fields[:len(fields)-1]
	require.Len(t, fields, len(paths)*3)
	for index, path := range paths {
		offset := index * 3
		require.Equal(t, path, string(fields[offset]))
		require.Equal(t, "eol", string(fields[offset+1]))
		require.Equal(t, "lf", string(fields[offset+2]), "proof input %q must retain LF checkout bytes", path)
	}
}

func assertProofJobMatchesMetadata(
	t *testing.T,
	repoRoot string,
	metadataPath string,
	bundle promprofileproof.Bundle,
) {
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

	identity := bundle.Descriptor.Validation.MetadataExample
	var config string
	for _, mod := range metadata.Modules {
		if mod.Meta.ID != identity.IntegrationID {
			continue
		}
		for _, example := range mod.Setup.Configuration.Examples.List {
			if example.Name == identity.ExampleName {
				config = example.Config
			}
		}
	}
	require.NotEmptyf(t, config, "metadata example %q for %q", identity.ExampleName, identity.IntegrationID)

	var exampleConfig struct {
		Jobs []map[string]any `yaml:"jobs"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(config), &exampleConfig))
	var metadataJob map[string]any
	for _, job := range exampleConfig.Jobs {
		if job["name"] == identity.JobName {
			metadataJob = job
		}
	}
	require.NotNilf(t, metadataJob, "metadata job %q in example %q", identity.JobName, identity.ExampleName)
	delete(metadataJob, "url")
	delete(metadataJob, "profiles")

	var validationJob map[string]any
	jobPath := filepath.Join(repoRoot, filepath.FromSlash(bundle.ValidationJobPath()))
	content, err = os.ReadFile(jobPath)
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(content, &validationJob))
	delete(validationJob, "future_inputs")
	require.Equal(t, metadataJob, validationJob,
		"proof validation job must match metadata after removing endpoint/profile selection and validation-only inputs")
}
